// End-to-end for step 3: website crawling through the real route.
// Serves a fake customer site on a public-looking port and points Anis at it.
import { createServer } from 'node:http';

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
  return { status: res.status, body };
};

const filler =
  'الشحن مجاني للطلبات فوق مئتي ريال ونحن نوصل إلى جميع مدن المملكة خلال ثلاثة أيام عمل من تاريخ الطلب. ';

const pages = {
  '/robots.txt': { type: 'text/plain', body: 'User-agent: *\nDisallow: /admin\n' },
  '/': {
    type: 'text/html; charset=utf-8',
    body: `<html><head><title>متجر النخبة</title></head><body>
      <nav><a href="/">الرئيسية</a></nav>
      <main><h1>مرحباً بكم</h1><p>${filler.repeat(3)}</p></main>
      <a href="/shipping">الشحن</a><a href="/admin/secret">admin</a>
      <footer>جميع الحقوق محفوظة</footer></body></html>`,
  },
  '/shipping': {
    type: 'text/html; charset=utf-8',
    body: `<html><head><title>سياسة الشحن</title></head><body>
      <main><h1>الشحن</h1><p>${filler.repeat(3)}</p></main></body></html>`,
  },
};

let adminHits = 0;
const site = createServer((req, res) => {
  const path = req.url.split('?')[0];
  if (path.startsWith('/admin')) {
    adminHits++;
    res.writeHead(200, { 'content-type': 'text/html' });
    res.end('<html><body><main><p>SECRET ADMIN CONTENT</p></main></body></html>');
    return;
  }
  const p = pages[path];
  if (!p) { res.writeHead(404); res.end('nope'); return; }
  res.writeHead(200, { 'content-type': p.type });
  res.end(p.body);
});

await new Promise((r) => site.listen(0, '127.0.0.1', r));
const sitePort = site.address().port;
const siteURL = `http://127.0.0.1:${sitePort}`;
console.log(`fake customer site on ${siteURL}\n`);

const stamp = Date.now();
const email = `crawl${stamp}@example.com`;
const pw = 'correct-horse-1';
await j('/api/collections/users/records', {
  method: 'POST',
  body: JSON.stringify({ email, password: pw, passwordConfirm: pw }),
});
const auth = await j('/api/collections/users/auth-with-password', {
  method: 'POST',
  body: JSON.stringify({ identity: email, password: pw }),
});
const token = auth.body.token;
const ws = await j('/api/collections/workspaces/records', { headers: { authorization: token } });
const workspace = ws.body.items[0].id;

// --- SSRF ------------------------------------------------------------------
// Private-address refusal is NOT asserted here: this server runs with
// ANIS_ALLOW_PRIVATE_CRAWL=1 so the fixture site on 127.0.0.1 is reachable, and
// the flag is exactly what disables that guard. It is covered by the Go tests
// (TestDialerRefusesPrivateAddressesAtConnectTime, TestCheckURLRejectsPrivateLiterals,
// TestIsPublicBlocksTheAddressesThatMatter). Scheme rejection still applies with
// the flag on, so that is checked.
for (const [url, what] of [
  ['file:///etc/passwd', 'local file'],
  ['gopher://example.com/', 'gopher'],
]) {
  const r = await j('/api/anis/sources', {
    method: 'POST',
    headers: { authorization: token },
    body: JSON.stringify({ workspace, type: 'website', url }),
  });
  check(r.status >= 400, `refuses ${what} -> ${r.status}`);
}

// --- crawl the fake site ----------------------------------------------------
// The dev server runs with ANIS_ALLOW_PRIVATE_CRAWL=1 so loopback is reachable
// for this test only; see the flag's documentation.
const created = await j('/api/anis/sources', {
  method: 'POST',
  headers: { authorization: token },
  body: JSON.stringify({ workspace, type: 'website', url: siteURL }),
});
check(created.status === 200, `start crawl -> ${created.status} ${JSON.stringify(created.body).slice(0, 140)}`);
check(created.body.status === 'queued', `source starts queued (got ${created.body.status})`);

// --- poll until it finishes -------------------------------------------------
let source = null;
for (let i = 0; i < 60; i++) {
  await new Promise((r) => setTimeout(r, 500));
  const list = await j('/api/collections/sources/records', { headers: { authorization: token } });
  source = list.body.items.find((s) => s.id === created.body.id);
  if (source && (source.status === 'ready' || source.status === 'failed')) break;
}
check(!!source, 'source is visible to its owner');
check(source?.status === 'ready', `crawl finished ready (got ${source?.status}: ${source?.error})`);
check(source?.pages >= 2, `indexed ${source?.pages} pages (want >= 2)`);
check(source?.title === '127.0.0.1', `title derived from the host: ${source?.title}`);

// --- robots.txt was obeyed ---------------------------------------------------
check(adminHits === 0, `disallowed /admin fetched ${adminHits} times (want 0)`);

const chunks = await j('/api/collections/chunks/records?perPage=200', {
  headers: { authorization: token },
});
const all = chunks.body.items.map((c) => c.text).join('\n');
check(!all.includes('SECRET ADMIN'), 'disallowed content never reached the index');
check(!all.includes('جميع الحقوق محفوظة'), 'footer boilerplate stripped');
check(all.includes('الشحن مجاني'), 'page content indexed');
check(all.includes('سياسة الشحن'), 'page title prepended to its chunks');

// --- refresh -----------------------------------------------------------------
const before = chunks.body.totalItems;
const refresh = await j(`/api/anis/sources/${created.body.id}/refresh`, {
  method: 'POST',
  headers: { authorization: token },
});
check(refresh.status === 200, `refresh -> ${refresh.status}`);

let after = null;
for (let i = 0; i < 60; i++) {
  await new Promise((r) => setTimeout(r, 500));
  const list = await j('/api/collections/sources/records', { headers: { authorization: token } });
  after = list.body.items.find((s) => s.id === created.body.id);
  if (after && (after.status === 'ready' || after.status === 'failed')) break;
}
check(after?.status === 'ready', `refresh finished ready (got ${after?.status}: ${after?.error})`);

const chunksAfter = await j('/api/collections/chunks/records?perPage=200', {
  headers: { authorization: token },
});
check(
  chunksAfter.body.totalItems === before,
  `refresh replaced rather than duplicated: ${before} -> ${chunksAfter.body.totalItems}`,
);

site.close();
console.log(fail === 0 ? '\nALL PASS' : `\n${fail} FAILURE(S)`);
process.exit(fail === 0 ? 0 : 1);
