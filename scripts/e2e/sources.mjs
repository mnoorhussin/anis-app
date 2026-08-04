// End-to-end for step 2: source ingestion over the real HTTP route.
const API = 'http://127.0.0.1:8090';

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

let fail = 0;
const check = (ok, msg) => { console.log(`${ok ? 'ok  ' : 'FAIL'} ${msg}`); if (!ok) fail++; };

const stamp = Date.now();
const signup = async (email, pw) => {
  await j('/api/collections/users/records', {
    method: 'POST',
    body: JSON.stringify({ email, password: pw, passwordConfirm: pw }),
  });
  const auth = await j('/api/collections/users/auth-with-password', {
    method: 'POST',
    body: JSON.stringify({ identity: email, password: pw }),
  });
  const ws = await j('/api/collections/workspaces/records', {
    headers: { authorization: auth.body.token },
  });
  return { token: auth.body.token, workspace: ws.body.items[0] };
};

const alice = await signup(`alice${stamp}@example.com`, 'correct-horse-1');
const mallory = await signup(`mallory${stamp}@example.com`, 'correct-horse-2');
check(!!alice.token && !!alice.workspace, 'alice signed up with a workspace');

// --- add a text source ------------------------------------------------------
const text = await j('/api/anis/sources', {
  method: 'POST',
  headers: { authorization: alice.token },
  body: JSON.stringify({
    workspace: alice.workspace.id,
    type: 'text',
    title: 'سياسة الشحن',
    body:
      'الشحن مجاني للطلبات فوق ٢٠٠ ريال.\n\n' +
      'مدة التوصيل إلى الرياض ثلاثة أيام عمل.\n\n' +
      'يمكنك إرجاع المنتج خلال ٣٠ يوماً من تاريخ الاستلام.',
  }),
});
check(text.status === 200, `create Arabic text source -> ${text.status} ${JSON.stringify(text.body).slice(0, 160)}`);
check(text.body.status === 'ready', `source status = ${text.body.status}`);
check(text.body.pages > 0, `indexed ${text.body.pages} passages`);

// --- add an FAQ source ------------------------------------------------------
const faq = await j('/api/anis/sources', {
  method: 'POST',
  headers: { authorization: alice.token },
  body: JSON.stringify({
    workspace: alice.workspace.id,
    type: 'faq',
    title: 'Common questions',
    pairs: [
      { question: 'Do you ship to France?', answer: 'Yes, within five working days.' },
      { question: 'هل لديكم شحن مجاني؟', answer: 'نعم، فوق ٢٠٠ ريال.' },
    ],
  }),
});
check(faq.status === 200, `create FAQ source -> ${faq.status}`);
check(faq.body.pages === 2, `FAQ produced ${faq.body.pages} passages (want exactly one per pair)`);

// --- unimplemented types are refused ---------------------------------------
for (const type of ['website', 'pdf']) {
  const r = await j('/api/anis/sources', {
    method: 'POST',
    headers: { authorization: alice.token },
    body: JSON.stringify({ workspace: alice.workspace.id, type, title: 'x', body: 'y' }),
  });
  check(r.status >= 400, `${type} source refused -> ${r.status}`);
}

// --- tenancy: custom routes bypass collection rules, so this is the real test
const steal = await j('/api/anis/sources', {
  method: 'POST',
  headers: { authorization: mallory.token },
  body: JSON.stringify({
    workspace: alice.workspace.id,
    type: 'text',
    title: 'injected',
    body: 'Mallory was here.',
  }),
});
check(steal.status === 404, `mallory writing into alice's workspace -> ${steal.status} (want 404)`);

const anon = await j('/api/anis/sources', {
  method: 'POST',
  body: JSON.stringify({ workspace: alice.workspace.id, type: 'text', title: 'x', body: 'y' }),
});
check(anon.status === 401 || anon.status === 403, `anonymous create -> ${anon.status}`);

// --- mallory cannot read alice's sources ------------------------------------
const list = await j('/api/collections/sources/records', {
  headers: { authorization: mallory.token },
});
check(list.body.totalItems === 0, `mallory sees ${list.body.totalItems} of alice's sources (want 0)`);

const aliceList = await j('/api/collections/sources/records', {
  headers: { authorization: alice.token },
});
check(aliceList.body.totalItems === 2, `alice sees her own ${aliceList.body.totalItems} sources`);

// --- chunks are scoped too --------------------------------------------------
const mChunks = await j('/api/collections/chunks/records', {
  headers: { authorization: mallory.token },
});
check(mChunks.body.totalItems === 0, `mallory sees ${mChunks.body.totalItems} of alice's chunks (want 0)`);

const aChunks = await j('/api/collections/chunks/records?perPage=100', {
  headers: { authorization: alice.token },
});
check(aChunks.body.totalItems > 0, `alice sees her ${aChunks.body.totalItems} chunks`);
const normalised = aChunks.body.items.find((c) => c.text.includes('٣٠'));
check(!!normalised, 'original Arabic text preserved verbatim (Arabic-Indic digits intact)');

// --- deleting a source must remove its vectors, not just its rows -----------
const del = await j(`/api/collections/sources/records/${text.body.id}`, {
  method: 'DELETE',
  headers: { authorization: alice.token },
});
check(del.status === 204, `delete source -> ${del.status}`);

const afterChunks = await j('/api/collections/chunks/records?perPage=100', {
  headers: { authorization: alice.token },
});
check(
  afterChunks.body.totalItems === faq.body.pages,
  `after delete, ${afterChunks.body.totalItems} chunks remain (want ${faq.body.pages} from the FAQ)`,
);

console.log(fail === 0 ? '\nALL PASS' : `\n${fail} FAILURE(S)`);
process.exit(fail === 0 ? 0 : 1);
