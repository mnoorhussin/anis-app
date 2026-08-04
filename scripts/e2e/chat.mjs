// End-to-end for step 4: the widget chat endpoint.
const API = 'http://127.0.0.1:8090';
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
  return { status: res.status, body, headers: res.headers };
};

// Reads an SSE response into its token stream and final event.
const ask = async (key, text, origin, conversationId = null) => {
  const res = await fetch(`${API}/api/anis/widget/${key}/message`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', ...(origin ? { origin } : {}) },
    body: JSON.stringify({ text, conversationId, visitor: 'v-test' }),
  });
  if (res.status !== 200) return { status: res.status, reply: '', done: null };

  const raw = await res.text();
  let reply = '';
  let done = null;
  let events = 0;
  for (const block of raw.split('\n\n')) {
    const ev = /^event: (.+)$/m.exec(block)?.[1];
    const data = /^data: (.+)$/m.exec(block)?.[1];
    if (!ev || !data) continue;
    events++;
    const parsed = JSON.parse(data);
    if (ev === 'token') reply += parsed.text;
    if (ev === 'done') done = parsed;
  }
  return { status: res.status, reply, done, events, contentType: res.headers.get('content-type') };
};

const ORIGIN = 'https://shop.example.com';
const stamp = Date.now();
const email = `chat${stamp}@example.com`;
const pw = 'correct-horse-1';

await j('/api/collections/users/records', {
  method: 'POST',
  body: JSON.stringify({ email, password: pw, passwordConfirm: pw, name: 'متجر النخبة' }),
});
const auth = await j('/api/collections/users/auth-with-password', {
  method: 'POST',
  body: JSON.stringify({ identity: email, password: pw }),
});
const token = auth.body.token;
const wsRes = await j('/api/collections/workspaces/records', { headers: { authorization: token } });
const workspace = wsRes.body.items[0];
const key = workspace.widget_key;

// --- config: allow-list is deny-by-default ----------------------------------
let cfg = await j(`/api/anis/widget/${key}/config`, { headers: { origin: ORIGIN } });
check(cfg.status === 404, `unconfigured workspace refuses any origin -> ${cfg.status}`);

// Add the domain (superuser-only via collection rules, so do it directly with
// the owner's token through the workspaces update rule).
const upd = await j(`/api/collections/workspaces/records/${workspace.id}`, {
  method: 'PATCH',
  headers: { authorization: token },
  body: JSON.stringify({ allowed_domains: ['shop.example.com'] }),
});
check(upd.status === 200, `owner can set allowed domains -> ${upd.status}`);

cfg = await j(`/api/anis/widget/${key}/config`, { headers: { origin: ORIGIN } });
check(cfg.status === 200, `config served to an allowed origin -> ${cfg.status}`);
check(cfg.headers.get('access-control-allow-origin') === ORIGIN, 'CORS echoes the allowed origin');
check(!!cfg.body.greeting?.ar && !!cfg.body.greeting?.en, 'config carries both greetings');
check(cfg.body.accentColor === '#5a5af0', 'config carries the accent colour');
// The config must never leak the knowledge base or internals.
for (const leak of ['account', 'allowed_domains', 'widget_key', 'sources']) {
  check(!(leak in cfg.body), `config does not leak "${leak}"`);
}

// --- wrong origin -----------------------------------------------------------
const wrong = await j(`/api/anis/widget/${key}/config`, { headers: { origin: 'https://evil.example' } });
check(wrong.status === 404, `disallowed origin refused -> ${wrong.status}`);
const noOrigin = await j(`/api/anis/widget/${key}/config`);
check(noOrigin.status === 404, `missing Origin refused -> ${noOrigin.status}`);
const badKey = await j('/api/anis/widget/wk_NOTAREALKEY0000000000000/config', { headers: { origin: ORIGIN } });
check(badKey.status === 404, `unknown key refused -> ${badKey.status}`);

// --- with no sources at all, every question must be refused -----------------
const empty = await ask(key, 'هل لديكم شحن مجاني؟', ORIGIN);
check(empty.status === 200, `ask with no sources -> ${empty.status}`);
check(empty.done?.outcome === 'refused', `outcome = ${empty.done?.outcome}, want refused`);
check(
  empty.reply.includes('لم أجد هذه المعلومة'),
  `Arabic question gets the Arabic refusal verbatim: ${JSON.stringify(empty.reply.slice(0, 60))}`,
);
check(empty.done?.offerHandoff === true, 'refusal offers a human handoff');
check(empty.contentType?.includes('text/event-stream'), `streamed as SSE (${empty.contentType})`);

const emptyEn = await ask(key, 'Do you ship to France?', ORIGIN);
check(emptyEn.reply.includes("couldn't find that"), 'English question gets the English refusal');

// --- add a source, then the same question should answer ---------------------
await j('/api/anis/sources', {
  method: 'POST',
  headers: { authorization: token },
  body: JSON.stringify({
    workspace: workspace.id,
    type: 'faq',
    title: 'الأسئلة الشائعة',
    pairs: [
      { question: 'هل لديكم شحن مجاني؟', answer: 'نعم، الشحن مجاني للطلبات فوق مئتي ريال.' },
      { question: 'كم مدة التوصيل؟', answer: 'ثلاثة أيام عمل داخل المملكة.' },
    ],
  }),
});

const answered = await ask(key, 'هل لديكم شحن مجاني؟', ORIGIN);
check(answered.done?.outcome === 'answered', `outcome = ${answered.done?.outcome}, want answered`);
check(answered.events > 2, `streamed in ${answered.events} events (not one lump)`);
// The dev provider echoes retrieved passages, which proves grounding rather
// than fluency — a fluent fake would hide a broken knowledge base.
check(answered.reply.includes('DEV ECHO'), 'dev provider is unmistakably marked');
check(answered.reply.includes('مئتي ريال'), 'the retrieved passage reached the model');
check(answered.done?.offerHandoff === false, 'an answered question does not offer handoff');

// --- a question the sources cannot answer must still refuse -----------------
const offTopic = await ask(key, 'ما هو رأس مال الشركة وكم عدد الموظفين في فرع دبي؟', ORIGIN);
check(offTopic.done?.outcome === 'refused', `off-topic outcome = ${offTopic.done?.outcome}, want refused`);
check(!offTopic.reply.includes('DEV ECHO'), 'a refusal never calls the model');

// --- conversation continuity ------------------------------------------------
const first = await ask(key, 'كم مدة التوصيل؟', ORIGIN);
const convo = first.done?.conversationId;
check(!!convo, 'a conversation id is returned');
const second = await ask(key, 'وهل الشحن مجاني؟', ORIGIN, convo);
check(second.done?.conversationId === convo, 'follow-up stays in the same conversation');

// --- knowledge gaps recorded for refusals only ------------------------------
const gaps = await j('/api/collections/knowledge_gaps/records', { headers: { authorization: token } });
check(gaps.body.totalItems > 0, `knowledge gaps recorded: ${gaps.body.totalItems}`);
const answeredGap = gaps.body.items.find((g) => g.question.includes('شحن مجاني') && g.count > 0);
const offTopicGap = gaps.body.items.find((g) => g.question.includes('رأس مال'));
check(!!offTopicGap, 'the unanswerable question was recorded as a gap');
check(
  typeof offTopicGap?.best_similarity === 'number',
  'the gap records the best similarity, so the floor can be tuned from it',
);

// --- usage metered for answers, NOT for refusals ----------------------------
const usage = await j('/api/collections/usage/records', { headers: { authorization: token } });
const used = usage.body.items[0]?.ai_replies_used ?? 0;
// Two questions were answered (free-shipping, delivery-time); the rest refused.
check(used === 2, `metered ${used} replies — refusals must not be billable (want 2)`);

// --- tenancy: another business cannot read any of this ----------------------
const other = `other${stamp}@example.com`;
await j('/api/collections/users/records', {
  method: 'POST',
  body: JSON.stringify({ email: other, password: pw, passwordConfirm: pw }),
});
const otherAuth = await j('/api/collections/users/auth-with-password', {
  method: 'POST',
  body: JSON.stringify({ identity: other, password: pw }),
});
const ot = otherAuth.body.token;
for (const col of ['conversations', 'messages', 'knowledge_gaps', 'usage']) {
  const r = await j(`/api/collections/${col}/records`, { headers: { authorization: ot } });
  check(r.body.totalItems === 0, `another business sees 0 of our ${col} (got ${r.body.totalItems})`);
}

console.log(fail === 0 ? '\nALL PASS' : `\n${fail} FAILURE(S)`);
process.exit(fail === 0 ? 0 : 1);
