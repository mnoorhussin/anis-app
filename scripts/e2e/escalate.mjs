// End-to-end for escalation + inbox.
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

const ask = async (key, text, conversationId = null) => {
  const res = await fetch(`${API}/api/anis/widget/${key}/message`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', origin: ORIGIN },
    body: JSON.stringify({ text, conversationId, visitor: 'v-esc' }),
  });
  const raw = await res.text();
  let reply = '', done = null;
  for (const block of raw.split('\n\n')) {
    const ev = /^event: (.+)$/m.exec(block)?.[1];
    const data = /^data: (.+)$/m.exec(block)?.[1];
    if (!ev || !data) continue;
    const p = JSON.parse(data);
    if (ev === 'token') reply += p.text;
    if (ev === 'done') done = p;
  }
  return { reply, done };
};

const stamp = Date.now();
const pw = 'correct-horse-1';

// --- a Growth account (human takeover) ---------------------------------------
const email = `esc${stamp}@example.com`;
await j('/api/collections/users/records', {
  method: 'POST', body: JSON.stringify({ email, password: pw, passwordConfirm: pw }),
});
const auth = await j('/api/collections/users/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: email, password: pw }),
});
const token = auth.body.token;
const ws = (await j('/api/collections/workspaces/records', { headers: { authorization: token } })).body.items[0];
const key = ws.widget_key;

await j(`/api/collections/workspaces/records/${ws.id}`, {
  method: 'PATCH', headers: { authorization: token },
  body: JSON.stringify({ allowed_domains: ['shop.example.com'] }),
});

// --- Free plan: handoff must NOT be offered or accepted ---------------------
let cfg = await j(`/api/anis/widget/${key}/config`, { headers: { origin: ORIGIN } });
check(cfg.body.handoffEnabled === false, `free plan does not offer handoff (got ${cfg.body.handoffEnabled})`);

const freeAttempt = await j(`/api/anis/widget/${key}/escalate`, {
  method: 'POST', headers: { origin: ORIGIN },
  body: JSON.stringify({ conversationId: 'x', contact: 'a@b.com' }),
});
check(freeAttempt.status === 404, `free plan refuses a direct escalate call -> ${freeAttempt.status}`);

// Upgrade the account to Growth.
const accountId = ws.account;
// Accounts are superuser-only for writes, so go through the DB via a
// superuser token is not available here — instead use the admin-less path:
// the test harness seeds plan directly through the collections API is blocked,
// so we assert the gate above and switch plans via the usage of a fresh
// account is impractical. Use PocketBase's superuser API instead.
const su = await j('/api/collections/_superusers/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: 'test@anis.chat', password: 'testtesttest' }),
});
if (su.status !== 200) {
  console.log('note: no superuser available; skipping the Growth-plan half of this test');
  console.log(fail === 0 ? '\nALL PASS (partial)' : `\n${fail} FAILURE(S)`);
  process.exit(fail === 0 ? 0 : 1);
}
await j(`/api/collections/accounts/records/${accountId}`, {
  method: 'PATCH', headers: { authorization: su.body.token },
  body: JSON.stringify({ plan: 'growth' }),
});

cfg = await j(`/api/anis/widget/${key}/config`, { headers: { origin: ORIGIN } });
check(cfg.body.handoffEnabled === true, `growth plan offers handoff (got ${cfg.body.handoffEnabled})`);

// --- a refusal, then a handoff ----------------------------------------------
const refused = await ask(key, 'zxqv plorbin frundle wexit');
check(refused.done?.outcome === 'refused', `question refused (${refused.done?.outcome})`);
const convo = refused.done.conversationId;

const esc = await j(`/api/anis/widget/${key}/escalate`, {
  method: 'POST', headers: { origin: ORIGIN },
  body: JSON.stringify({
    conversationId: convo, name: 'ليلى حداد', contact: '+966501234567',
    question: 'zxqv plorbin frundle wexit',
  }),
});
check(esc.status === 200, `escalate -> ${esc.status}`);

// Contact must be required.
const noContact = await j(`/api/anis/widget/${key}/escalate`, {
  method: 'POST', headers: { origin: ORIGIN },
  body: JSON.stringify({ conversationId: convo, name: 'x', contact: '  ' }),
});
check(noContact.status === 400, `escalate without contact -> ${noContact.status}`);

// --- the lead and escalation are visible to the owner ------------------------
const leads = await j('/api/collections/leads/records', { headers: { authorization: token } });
check(leads.body.totalItems === 1, `lead recorded (${leads.body.totalItems})`);
check(leads.body.items[0]?.name === 'ليلى حداد', 'Arabic name stored intact');
check(leads.body.items[0]?.contact_kind === 'phone', `phone classified (got ${leads.body.items[0]?.contact_kind})`);

const escs = await j('/api/collections/escalations/records', { headers: { authorization: token } });
check(escs.body.totalItems === 1, `escalation raised (${escs.body.totalItems})`);

// Pressing the button again must not create a second escalation.
await j(`/api/anis/widget/${key}/escalate`, {
  method: 'POST', headers: { origin: ORIGIN },
  body: JSON.stringify({ conversationId: convo, contact: 'layla@example.com' }),
});
const escs2 = await j('/api/collections/escalations/records', { headers: { authorization: token } });
check(escs2.body.totalItems === 1, `a second press does not duplicate the escalation (${escs2.body.totalItems})`);

// --- the conversation is marked, and the AI goes silent ----------------------
const conv = await j(`/api/collections/conversations/records/${convo}`, { headers: { authorization: token } });
check(conv.body.status === 'escalated', `conversation marked escalated (got ${conv.body.status})`);

const silent = await ask(key, 'هل لديكم شحن مجاني؟', convo);
check(silent.reply === '', `assistant stayed silent after escalation (said: ${JSON.stringify(silent.reply.slice(0, 40))})`);
check(silent.done?.withHuman === true, 'widget is told a person has the conversation');

// The visitor's message is still recorded — the agent needs to see it.
const msgs = await j(`/api/collections/messages/records?filter=${encodeURIComponent(`conversation="${convo}"`)}&perPage=100`, {
  headers: { authorization: token },
});
const userMsgs = msgs.body.items.filter((m) => m.role === 'user');
check(userMsgs.length === 2, `both visitor messages saved (${userMsgs.length})`);
check(
  !msgs.body.items.some((m) => m.role === 'assistant' && m.created > conv.body.updated && m.text.length > 0 && m.outcome === 'answered'),
  'no assistant reply was generated after escalation',
);

// --- usage: an escalated conversation costs nothing --------------------------
const usage = await j('/api/collections/usage/records', { headers: { authorization: token } });
check((usage.body.items[0]?.ai_replies_used ?? 0) === 0, `no replies metered (${usage.body.items[0]?.ai_replies_used ?? 0})`);

// --- agent takes over and replies -------------------------------------------
await j(`/api/collections/conversations/records/${convo}`, {
  method: 'PATCH', headers: { authorization: token },
  body: JSON.stringify({ status: 'human' }),
});
const humanReply = await j('/api/collections/messages/records', {
  method: 'POST', headers: { authorization: token },
  body: JSON.stringify({ workspace: ws.id, conversation: convo, role: 'human', text: 'أهلاً ليلى، سأساعدك.' }),
});
check(humanReply.status === 200, `agent can post a human reply -> ${humanReply.status}`);

// An agent must NOT be able to forge a visitor or assistant message.
for (const role of ['user', 'assistant']) {
  const forged = await j('/api/collections/messages/records', {
    method: 'POST', headers: { authorization: token },
    body: JSON.stringify({ workspace: ws.id, conversation: convo, role, text: 'forged' }),
  });
  check(forged.status >= 400, `agent cannot forge a "${role}" message -> ${forged.status}`);
}

// --- return to auto ----------------------------------------------------------
await j(`/api/collections/conversations/records/${convo}`, {
  method: 'PATCH', headers: { authorization: token },
  body: JSON.stringify({ status: 'active' }),
});
const backToAuto = await ask(key, 'zxqv plorbin frundle wexit', convo);
check(backToAuto.reply.length > 0, 'assistant answers again after returning to auto');

// --- tenancy ------------------------------------------------------------------
const other = `oth${stamp}@example.com`;
await j('/api/collections/users/records', {
  method: 'POST', body: JSON.stringify({ email: other, password: pw, passwordConfirm: pw }),
});
const ot = (await j('/api/collections/users/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: other, password: pw }),
})).body.token;
for (const col of ['leads', 'escalations']) {
  const r = await j(`/api/collections/${col}/records`, { headers: { authorization: ot } });
  check(r.body.totalItems === 0, `another business sees 0 ${col} (got ${r.body.totalItems})`);
}

console.log(fail === 0 ? '\nALL PASS' : `\n${fail} FAILURE(S)`);
process.exit(fail === 0 ? 0 : 1);
