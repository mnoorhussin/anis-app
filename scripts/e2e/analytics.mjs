// End-to-end for ratings, analytics, and answering a knowledge gap.
//
// The metric this exercises is the one most easily faked: "auto-resolved".
// The assertions below exist to prove it is NOT faked — a thumbs down resolves
// nothing, an abandoned conversation resolves nothing, and every resolution
// carries the signal that justified it.
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

const ask = async (key, text, visitor, conversationId = null) => {
  const res = await fetch(`${API}/api/anis/widget/${key}/message`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', origin: ORIGIN },
    body: JSON.stringify({ text, conversationId, visitor }),
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

const rate = (key, body) =>
  j(`/api/anis/widget/${key}/rate`, {
    method: 'POST', headers: { origin: ORIGIN }, body: JSON.stringify(body),
  });

const stamp = Date.now();
const pw = 'correct-horse-1';
const email = `ana${stamp}@example.com`;

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

await j('/api/anis/sources', {
  method: 'POST', headers: { authorization: token },
  body: JSON.stringify({
    workspace: ws.id, type: 'faq', title: 'الشحن',
    pairs: [{ question: 'هل لديكم شحن مجاني؟', answer: 'نعم، فوق ٢٠٠ ريال.' }],
  }),
});

// --- an answered reply is rateable; a refusal is not -------------------------
const answered = await ask(key, 'هل لديكم شحن مجاني؟', 'v-up');
check(answered.done?.outcome === 'answered', `question answered (${answered.done?.outcome})`);
check(!!answered.done?.messageId, 'the done event carries a message id to rate');

const refused = await ask(key, 'zxqv plorbin frundle wexit', 'v-gap');
check(refused.done?.outcome === 'refused', `unknown question refused (${refused.done?.outcome})`);
check(!refused.done?.messageId, 'a refusal carries no message id — there is nothing to rate');

// --- a thumbs up resolves the conversation, and says why ---------------------
const up = await rate(key, {
  conversationId: answered.done.conversationId,
  messageId: answered.done.messageId,
  rating: 'up',
  visitor: 'v-up',
});
check(up.status === 200, `thumbs up -> ${up.status}`);

let conv = (await j(`/api/collections/conversations/records/${answered.done.conversationId}`, {
  headers: { authorization: token },
})).body;
check(conv.status === 'auto_resolved', `thumbs up resolves the conversation (got ${conv.status})`);
check(conv.resolution_signal === 'rated_helpful', `signal recorded (got ${conv.resolution_signal})`);

// --- a thumbs down resolves NOTHING ------------------------------------------
const second = await ask(key, 'هل لديكم شحن مجاني؟', 'v-down');
const down = await rate(key, {
  conversationId: second.done.conversationId,
  messageId: second.done.messageId,
  rating: 'down',
  visitor: 'v-down',
});
check(down.status === 200, `thumbs down -> ${down.status}`);
const conv2 = (await j(`/api/collections/conversations/records/${second.done.conversationId}`, {
  headers: { authorization: token },
})).body;
check(conv2.status === 'active', `thumbs down leaves the conversation unresolved (got ${conv2.status})`);
check(conv2.resolution_signal === '', `thumbs down invents no signal (got "${conv2.resolution_signal}")`);

// --- an abandoned conversation is never counted ------------------------------
const abandoned = await ask(key, 'هل لديكم شحن مجاني؟', 'v-ghost');
const convGhost = (await j(`/api/collections/conversations/records/${abandoned.done.conversationId}`, {
  headers: { authorization: token },
})).body;
check(convGhost.status === 'active', 'a conversation with no rating stays unresolved');

// --- ratings are authorised by visitor, not just by the public key ----------
const impostor = await rate(key, {
  conversationId: abandoned.done.conversationId,
  messageId: abandoned.done.messageId,
  rating: 'up',
  visitor: 'someone-else',
});
check(impostor.status === 404, `another visitor cannot rate this conversation -> ${impostor.status}`);

const userMsgId = (await j(
  `/api/collections/messages/records?filter=${encodeURIComponent(`conversation="${abandoned.done.conversationId}" && role="user"`)}`,
  { headers: { authorization: token } },
)).body.items[0]?.id;
const rateUser = await rate(key, {
  conversationId: abandoned.done.conversationId,
  messageId: userMsgId,
  rating: 'up',
  visitor: 'v-ghost',
});
check(rateUser.status === 400, `a visitor's own message cannot be rated -> ${rateUser.status}`);

const badRating = await rate(key, {
  conversationId: abandoned.done.conversationId,
  messageId: abandoned.done.messageId,
  rating: 'meh',
  visitor: 'v-ghost',
});
check(badRating.status === 400, `an invented rating value is refused -> ${badRating.status}`);

// --- the summary -------------------------------------------------------------
const a = (await j('/api/anis/analytics?days=30', { headers: { authorization: token } })).body;
check(a.conversations === 4, `4 conversations counted (got ${a.conversations})`);
check(a.autoResolved === 1, `exactly one resolved (got ${a.autoResolved})`);
check(a.resolutionBreakdown?.rated_helpful === 1, `breakdown attributes it to the rating (${JSON.stringify(a.resolutionBreakdown)})`);
check(a.ratedHelpful === 1 && a.ratedUnhelpful === 1, `ratings counted (${a.ratedHelpful}/${a.ratedUnhelpful})`);
check(a.answered === 3 && a.unanswered === 1, `outcomes counted (${a.answered} answered, ${a.unanswered} unanswered)`);
// Three Arabic conversations and one English: the nonsense question is Latin
// script, and the detector is right to say so rather than inheriting the
// workspace's language.
check(a.languages?.ar === 3 && a.languages?.en === 1, `languages detected per conversation (${JSON.stringify(a.languages)})`);
check(typeof a.firstResponseMedianMs === 'number', `median first response measured (${a.firstResponseMedianMs}ms)`);
check(a.knowledgeGaps === 1, `one knowledge gap (got ${a.knowledgeGaps})`);
check(!('topTopics' in a), 'no fabricated "top topics" field');

// A window is a window: nothing that predates it may appear.
const zero = (await j('/api/anis/analytics?days=0', { headers: { authorization: token } })).body;
check(zero.days === 30, `an invalid window falls back to 30 rather than erroring (got ${zero.days})`);

// --- tenancy ------------------------------------------------------------------
const other = `oth${stamp}@example.com`;
await j('/api/collections/users/records', {
  method: 'POST', body: JSON.stringify({ email: other, password: pw, passwordConfirm: pw }),
});
const ot = (await j('/api/collections/users/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: other, password: pw }),
})).body.token;

const stolen = await j(`/api/anis/analytics?workspace=${ws.id}`, { headers: { authorization: ot } });
check(stolen.status === 404 || stolen.body?.conversations === 0,
  `another business cannot read these numbers -> ${stolen.status} ${JSON.stringify(stolen.body).slice(0, 80)}`);

const anon = await j('/api/anis/analytics');
check(anon.status === 401, `analytics requires a sign-in -> ${anon.status}`);

// --- answering the gap turns it into knowledge -------------------------------
const gaps = (await j('/api/collections/knowledge_gaps/records', { headers: { authorization: token } })).body;
check(gaps.totalItems === 1, `the refused question was recorded as a gap (${gaps.totalItems})`);
const gap = gaps.items[0];
check(gap.question === 'zxqv plorbin frundle wexit', `the gap holds the question (${gap.question})`);
check(gap.answered === false, 'the gap starts unanswered');

const theft = await j('/api/anis/gaps/answer', {
  method: 'POST', headers: { authorization: ot },
  body: JSON.stringify({ gapId: gap.id, answer: 'nope' }),
});
check(theft.status === 404, `another business cannot answer this gap -> ${theft.status}`);

const empty = await j('/api/anis/gaps/answer', {
  method: 'POST', headers: { authorization: token },
  body: JSON.stringify({ gapId: gap.id, answer: '' }),
});
check(empty.status === 400, `an empty answer is refused -> ${empty.status}`);

const solved = await j('/api/anis/gaps/answer', {
  method: 'POST', headers: { authorization: token },
  body: JSON.stringify({ gapId: gap.id, answer: 'A frundle wexit takes about ten minutes.' }),
});
check(solved.status === 200, `answering the gap -> ${solved.status} ${JSON.stringify(solved.body).slice(0, 120)}`);
check(solved.body.source?.status === 'ready', `the answer was indexed (${solved.body.source?.status})`);

const gapAfter = (await j(`/api/collections/knowledge_gaps/records/${gap.id}`, { headers: { authorization: token } })).body;
check(gapAfter.answered === true, 'the gap is cleared only after ingestion succeeded');

// The point of the whole loop: the question is now answerable.
const nowAnswered = await ask(key, 'zxqv plorbin frundle wexit', 'v-again');
check(nowAnswered.done?.outcome === 'answered', `the previously-refused question is now answered (${nowAnswered.done?.outcome})`);

console.log(fail === 0 ? '\nALL PASS' : `\n${fail} FAILURE(S)`);
process.exit(fail === 0 ? 0 : 1);
