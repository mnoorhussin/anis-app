// End-to-end: does an agent's reply actually reach the visitor?
const API = 'http://127.0.0.1:8090';
const ORIGIN = 'https://shop.example.com';
let fail = 0;
const check = (ok, msg) => { console.log(`${ok ? 'ok  ' : 'FAIL'} ${msg}`); if (!ok) fail++; };

const j = async (path, opts = {}) => {
  const res = await fetch(API + path, {
    ...opts,
    headers: { 'content-type': 'application/json', ...(opts.headers || {}) },
  });
  const text = await res.text();
  let body;
  try { body = JSON.parse(text); } catch { body = text; }
  return { status: res.status, body };
};

const ask = async (key, text, conversationId, visitor) => {
  const res = await fetch(`${API}/api/anis/widget/${key}/message`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', origin: ORIGIN },
    body: JSON.stringify({ text, conversationId, visitor }),
  });
  const raw = await res.text();
  let done = null;
  for (const block of raw.split('\n\n')) {
    const ev = /^event: (.+)$/m.exec(block)?.[1];
    const data = /^data: (.+)$/m.exec(block)?.[1];
    if (ev === 'done' && data) done = JSON.parse(data);
  }
  return done;
};

const stamp = Date.now();
const pw = 'correct-horse-1';
const email = `str${stamp}@example.com`;
const VISITOR = `v-${stamp}`;

await j('/api/collections/users/records', {
  method: 'POST', body: JSON.stringify({ email, password: pw, passwordConfirm: pw }),
});
const token = (await j('/api/collections/users/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: email, password: pw }),
})).body.token;
const ws = (await j('/api/collections/workspaces/records', { headers: { authorization: token } })).body.items[0];
const key = ws.widget_key;

await j(`/api/collections/workspaces/records/${ws.id}`, {
  method: 'PATCH', headers: { authorization: token },
  body: JSON.stringify({ allowed_domains: ['shop.example.com'] }),
});

// Growth plan, so takeover is allowed.
const su = await j('/api/collections/_superusers/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: 'test@anis.chat', password: 'testtesttest' }),
});
await j(`/api/collections/accounts/records/${ws.account}`, {
  method: 'PATCH', headers: { authorization: su.body.token },
  body: JSON.stringify({ plan: 'growth' }),
});

// Start a conversation.
const done = await ask(key, 'zxqv plorbin frundle wexit', null, VISITOR);
const convo = done.conversationId;
check(!!convo, 'conversation started');

// --- connect as the widget would ---------------------------------------------
const streamURL = `${API}/api/anis/widget/${key}/stream?conversation=${convo}&visitor=${encodeURIComponent(VISITOR)}`;
const received = { history: null, messages: [], statuses: [] };

// Consume the stream with fetch rather than EventSource: Node's EventSource
// cannot set an Origin header, and the endpoint requires one. A browser sets
// it automatically for a cross-origin EventSource, so this exercises the same
// server path.
const controller = new AbortController();
const streamRes = await fetch(streamURL, {
  headers: { origin: ORIGIN, accept: 'text/event-stream' },
  signal: controller.signal,
});
check(streamRes.status === 200, `stream opened -> ${streamRes.status}`);

(async () => {
  const reader = streamRes.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      let i;
      while ((i = buf.indexOf('\n\n')) >= 0) {
        const block = buf.slice(0, i);
        buf = buf.slice(i + 2);
        const ev = /^event: (.+)$/m.exec(block)?.[1];
        const data = /^data: (.+)$/m.exec(block)?.[1];
        if (!ev || !data) continue;
        const parsed = JSON.parse(data);
        if (ev === 'history') received.history = parsed.messages;
        if (ev === 'message') received.messages.push(parsed);
        if (ev === 'status') received.statuses.push(parsed.status);
      }
    }
  } catch { /* aborted */ }
})();

const waitFor = async (pred, ms = 6000) => {
  const t0 = Date.now();
  while (Date.now() - t0 < ms) {
    if (pred()) return true;
    await new Promise((r) => setTimeout(r, 100));
  }
  return false;
};

check(await waitFor(() => received.history !== null), 'history replayed on connect');
check(
  received.history?.some((m) => m.role === 'user' && m.text.includes('zxqv')),
  'history contains the visitor question',
);
check(
  received.history?.some((m) => m.role === 'assistant'),
  'history contains the assistant refusal — a reload no longer shows an empty panel',
);

// --- agent takes over --------------------------------------------------------
await j(`/api/collections/conversations/records/${convo}`, {
  method: 'PATCH', headers: { authorization: token },
  body: JSON.stringify({ status: 'human' }),
});
check(await waitFor(() => received.statuses.includes('human')), 'visitor is told a person took over');

// --- the reply that this whole step exists for -------------------------------
const AGENT_TEXT = 'أهلاً، أنا سارة من فريق الدعم. سأساعدك الآن.';
await j('/api/collections/messages/records', {
  method: 'POST', headers: { authorization: token },
  body: JSON.stringify({ workspace: ws.id, conversation: convo, role: 'human', text: AGENT_TEXT }),
});

check(
  await waitFor(() => received.messages.some((m) => m.text === AGENT_TEXT)),
  "the agent's reply reached the visitor",
);
const agentMsg = received.messages.find((m) => m.text === AGENT_TEXT);
check(agentMsg?.role === 'human', `reply carries its role (${agentMsg?.role})`);
check(agentMsg?.text === AGENT_TEXT, 'Arabic reply arrived intact');

// --- the assistant's own reply must NOT be pushed (it streams instead) -------
await j(`/api/collections/conversations/records/${convo}`, {
  method: 'PATCH', headers: { authorization: token },
  body: JSON.stringify({ status: 'active' }),
});
await waitFor(() => received.statuses.includes('active'));

const before = received.messages.length;
await ask(key, 'zxqv plorbin frundle wexit', convo, VISITOR);
await new Promise((r) => setTimeout(r, 1200));
check(
  received.messages.length === before,
  `assistant reply not double-pushed (${received.messages.length - before} extra events)`,
);

controller.abort();

// --- authorisation ------------------------------------------------------------
// The widget key is public, so it alone must not open a conversation.
const wrongVisitor = await fetch(
  `${API}/api/anis/widget/${key}/stream?conversation=${convo}&visitor=someone-else`,
  { headers: { origin: ORIGIN } },
);
check(wrongVisitor.status === 404, `another visitor id is refused -> ${wrongVisitor.status}`);

const noVisitor = await fetch(`${API}/api/anis/widget/${key}/stream?conversation=${convo}`, {
  headers: { origin: ORIGIN },
});
check(noVisitor.status === 400, `missing visitor is refused -> ${noVisitor.status}`);

const wrongOrigin = await fetch(streamURL, { headers: { origin: 'https://evil.example' } });
check(wrongOrigin.status === 404, `disallowed origin is refused -> ${wrongOrigin.status}`);

// A conversation belonging to another workspace must be invisible even with a
// valid key and visitor.
const otherEmail = `oth${stamp}@example.com`;
await j('/api/collections/users/records', {
  method: 'POST', body: JSON.stringify({ email: otherEmail, password: pw, passwordConfirm: pw }),
});
const otherToken = (await j('/api/collections/users/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: otherEmail, password: pw }),
})).body.token;
const otherWs = (await j('/api/collections/workspaces/records', { headers: { authorization: otherToken } })).body.items[0];
await j(`/api/collections/workspaces/records/${otherWs.id}`, {
  method: 'PATCH', headers: { authorization: otherToken },
  body: JSON.stringify({ allowed_domains: ['shop.example.com'] }),
});
const crossTenant = await fetch(
  `${API}/api/anis/widget/${otherWs.widget_key}/stream?conversation=${convo}&visitor=${encodeURIComponent(VISITOR)}`,
  { headers: { origin: ORIGIN } },
);
check(crossTenant.status === 404, `another workspace's key cannot open this conversation -> ${crossTenant.status}`);

console.log(fail === 0 ? '\nALL PASS' : `\n${fail} FAILURE(S)`);
process.exit(fail === 0 ? 0 : 1);
