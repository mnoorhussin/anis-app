// Seeds a known account with enough traffic to make every screen show something.
//
// For local development only. It talks to 127.0.0.1 and creates data that is
// meaningless without the fake embedder, so these credentials are not secrets
// and must never exist on a real deployment.
//
// Re-running adds more traffic rather than replacing it — the account and its
// sources are reused, the conversations accumulate. Delete pb_data for a clean
// slate.
const API = 'http://127.0.0.1:8090';
const ORIGIN = 'https://shop.example.com';
const EMAIL = 'demo@anis.test';
const PASSWORD = 'demo-anis-1234';

const j = async (path, opts = {}) => {
  const res = await fetch(API + path, {
    ...opts,
    headers: { 'content-type': 'application/json', ...(opts.headers || {}) },
  });
  const text = await res.text();
  try { return JSON.parse(text); } catch { return text; }
};

await j('/api/collections/users/records', {
  method: 'POST',
  body: JSON.stringify({
    email: EMAIL, password: PASSWORD, passwordConfirm: PASSWORD, name: 'متجر النخبة',
  }),
});
const auth = await j('/api/collections/users/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: EMAIL, password: PASSWORD }),
});
if (!auth.token) {
  console.error('could not sign in — is the backend running with the dev flags?');
  process.exit(1);
}
const token = auth.token;
const ws = (await j('/api/collections/workspaces/records', { headers: { authorization: token } })).items[0];
const key = ws.widget_key;

await j(`/api/collections/workspaces/records/${ws.id}`, {
  method: 'PATCH', headers: { authorization: token },
  body: JSON.stringify({ allowed_domains: ['shop.example.com'] }),
});

// Human takeover is a Growth entitlement, and a plan change is deliberately
// something a customer cannot make. Without a superuser the account stays Free
// and the inbox stays empty — which is correct, just a duller demo.
const su = await j('/api/collections/_superusers/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: 'test@anis.chat', password: 'testtesttest' }),
});
if (su.token) {
  await j(`/api/collections/accounts/records/${ws.account}`, {
    method: 'PATCH', headers: { authorization: su.token },
    body: JSON.stringify({ plan: 'growth' }),
  });
} else {
  console.log('note: no superuser, so the account stays on Free and handoff is off');
  console.log('      create one with: go run -tags no_default_driver . superuser upsert test@anis.chat testtesttest');
}

// A source left in `failed` — the usual cause is a backend started without the
// dev flags, so ingestion asked for a real VOYAGE_API_KEY — is cleared rather
// than kept. Otherwise the seed looks like it worked while every question gets
// refused.
const existing = await j('/api/collections/sources/records', { headers: { authorization: token } });
for (const s of existing.items ?? []) {
  if (s.status !== 'ready') {
    await j(`/api/collections/sources/records/${s.id}`, {
      method: 'DELETE', headers: { authorization: token },
    });
  }
}
const ready = (existing.items ?? []).filter((s) => s.status === 'ready');
if (ready.length === 0) {
  await j('/api/anis/sources', {
    method: 'POST', headers: { authorization: token },
    body: JSON.stringify({
      workspace: ws.id, type: 'text', title: 'سياسة الشحن والإرجاع',
      body:
        'الشحن مجاني للطلبات فوق ٢٠٠ ريال.\n\n' +
        'مدة التوصيل إلى الرياض ثلاثة أيام عمل، وإلى بقية المدن خمسة أيام.\n\n' +
        'يمكنك إرجاع المنتج خلال ٣٠ يوماً من تاريخ الاستلام بشرط أن يكون بحالته الأصلية.',
    }),
  });
  await j('/api/anis/sources', {
    method: 'POST', headers: { authorization: token },
    body: JSON.stringify({
      workspace: ws.id, type: 'faq', title: 'Common questions',
      pairs: [
        { question: 'هل لديكم شحن مجاني؟', answer: 'نعم، الشحن مجاني للطلبات فوق ٢٠٠ ريال.' },
        { question: 'كيف أتتبع طلبي؟', answer: 'ستصلك رسالة برقم التتبع فور شحن الطلب.' },
        { question: 'Do you ship to France?', answer: 'Yes, within five working days.' },
      ],
    }),
  });
}

const ask = async (text, visitor) => {
  const res = await fetch(`${API}/api/anis/widget/${key}/message`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', origin: ORIGIN },
    body: JSON.stringify({ text, conversationId: null, visitor }),
  });
  const raw = await res.text();
  for (const block of raw.split('\n\n')) {
    if (/^event: done$/m.test(block)) {
      const d = /^data: (.+)$/m.exec(block)?.[1];
      if (d) return JSON.parse(d);
    }
  }
  return null;
};

const rate = (body) =>
  j(`/api/anis/widget/${key}/rate`, {
    method: 'POST', headers: { origin: ORIGIN }, body: JSON.stringify(body),
  });

const stamp = Date.now();

// Answered, and mostly rated — so the analytics view has a real breakdown
// rather than a column of zeroes.
const answerable = [
  'هل لديكم شحن مجاني؟',
  'كيف أتتبع طلبي؟',
  'كم مدة التوصيل إلى الرياض؟',
  'Do you ship to France?',
  'هل يمكنني إرجاع المنتج؟',
  'ما هي مدة الإرجاع؟',
];
const replied = [];
for (const [i, q] of answerable.entries()) {
  const visitor = `seed-${stamp}-${i}`;
  const done = await ask(q, visitor);
  // Under the fake embedder some of these come back refused — it has no
  // semantic understanding — so the ratings are attached to whatever actually
  // got answered rather than to fixed positions.
  if (done?.messageId) replied.push({ done, visitor });
}
for (const [i, r] of replied.entries()) {
  const body = { conversationId: r.done.conversationId, messageId: r.done.messageId, visitor: r.visitor };
  // All but the last two helpful, one unhelpful, one left unrated. The unrated
  // one is the point: it must not appear as resolved.
  if (i < replied.length - 2) await rate({ ...body, rating: 'up' });
  else if (i === replied.length - 2) await rate({ ...body, rating: 'down' });
}

// Refused — these become the knowledge-gap queue.
for (const [i, q] of [
  'هل لديكم فرع في جدة؟',
  'هل لديكم فرع في جدة؟',
  'do you offer gift wrapping',
  'ما هي طرق الدفع بالتقسيط؟',
].entries()) {
  await ask(q, `gap-${stamp}-${i}`);
}

// One escalation, so the inbox and the leads figure are not empty.
const esc = await ask('أريد التحدث مع موظف بخصوص طلب خاص', `esc-${stamp}`);
if (esc) {
  await j(`/api/anis/widget/${key}/escalate`, {
    method: 'POST', headers: { origin: ORIGIN },
    body: JSON.stringify({
      conversationId: esc.conversationId,
      name: 'ليلى حداد',
      contact: '+966501234567',
      question: 'أريد التحدث مع موظف بخصوص طلب خاص',
    }),
  });
}

console.log(`email     ${EMAIL}`);
console.log(`password  ${PASSWORD}`);
console.log(`workspace ${ws.id}`);
console.log(`widget    ${key}`);
console.log(`origin    ${ORIGIN}  (the only domain the widget will load on)`);
