// End-to-end check of signup provisioning + tenant isolation, over the real
// HTTP API with proper UTF-8 (bash/curl on Windows mangles Arabic on the way in).
const API = 'http://127.0.0.1:8090';

const j = async (path, opts = {}) => {
  const res = await fetch(API + path, {
    ...opts,
    headers: { 'content-type': 'application/json', ...(opts.headers || {}) },
  });
  const text = await res.text();
  let body;
  try {
    body = JSON.parse(text);
  } catch {
    body = text;
  }
  return { status: res.status, body };
};

const ARABIC_NAME = 'ليلى حدّاد';
let fail = 0;
const check = (ok, msg) => {
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${msg}`);
  if (!ok) fail++;
};

const stamp = Date.now();
const userA = `a${stamp}@example.com`;
const userB = `b${stamp}@example.com`;

// --- signup A, with an Arabic name -----------------------------------------
const createA = await j('/api/collections/users/records', {
  method: 'POST',
  body: JSON.stringify({
    email: userA,
    password: 'correct-horse-1',
    passwordConfirm: 'correct-horse-1',
    name: ARABIC_NAME,
  }),
});
check(createA.status === 200, `signup A -> ${createA.status}`);
check(
  createA.body.name === ARABIC_NAME,
  `Arabic name round-trips intact: ${JSON.stringify(createA.body.name)}`,
);

const authA = await j('/api/collections/users/auth-with-password', {
  method: 'POST',
  body: JSON.stringify({ identity: userA, password: 'correct-horse-1' }),
});
check(authA.status === 200, `auth A -> ${authA.status}`);
const tokenA = authA.body.token;

const wsA = await j('/api/collections/workspaces/records?expand=account', {
  headers: { authorization: tokenA },
});
check(wsA.body.totalItems === 1, `A sees exactly 1 workspace (saw ${wsA.body.totalItems})`);
const workspaceA = wsA.body.items[0];
check(
  workspaceA.name === ARABIC_NAME,
  `workspace named from the Arabic profile name: ${JSON.stringify(workspaceA.name)}`,
);
check(/^wk_[0-9A-HJKMNP-TV-Z]{26}$/.test(workspaceA.widget_key), `widget key well-formed: ${workspaceA.widget_key}`);
check(Array.isArray(workspaceA.allowed_domains) && workspaceA.allowed_domains.length === 0, 'allow-list empty by default');
check(workspaceA.expand?.account?.plan === 'free', 'account on free plan');
check(workspaceA.expand?.account?.hard_cap_usd > 0, 'account has a spending cap');

// --- signup B ---------------------------------------------------------------
await j('/api/collections/users/records', {
  method: 'POST',
  body: JSON.stringify({
    email: userB,
    password: 'correct-horse-2',
    passwordConfirm: 'correct-horse-2',
    name: 'Bob',
  }),
});
const authB = await j('/api/collections/users/auth-with-password', {
  method: 'POST',
  body: JSON.stringify({ identity: userB, password: 'correct-horse-2' }),
});
const tokenB = authB.body.token;

const wsB = await j('/api/collections/workspaces/records', { headers: { authorization: tokenB } });
check(wsB.body.totalItems === 1, `B sees exactly 1 workspace (saw ${wsB.body.totalItems})`);
check(wsB.body.items[0].id !== workspaceA.id, 'A and B have different workspaces');
check(
  wsB.body.items[0].widget_key !== workspaceA.widget_key,
  'A and B have different widget keys',
);

// --- tenant isolation -------------------------------------------------------
const steal = await j(`/api/collections/workspaces/records/${workspaceA.id}`, {
  headers: { authorization: tokenB },
});
check(steal.status === 404, `B fetching A's workspace by id -> ${steal.status} (want 404)`);

const stealAcct = await j(`/api/collections/accounts/records/${workspaceA.account}`, {
  headers: { authorization: tokenB },
});
check(stealAcct.status === 404, `B fetching A's account by id -> ${stealAcct.status} (want 404)`);

const anon = await j(`/api/collections/workspaces/records/${workspaceA.id}`);
check(anon.status === 404 || anon.status === 403, `anonymous fetch -> ${anon.status} (want 404/403)`);

const anonList = await j('/api/collections/workspaces/records');
check(
  anonList.status === 400 || anonList.status === 403 || anonList.body.totalItems === 0,
  `anonymous list -> ${anonList.status} / ${anonList.body.totalItems ?? '-'} items`,
);

// --- the client cannot widen its own scope ---------------------------------
const filtered = await j(
  `/api/collections/workspaces/records?filter=${encodeURIComponent(`id="${workspaceA.id}"`)}`,
  { headers: { authorization: tokenB } },
);
check(
  filtered.body.totalItems === 0,
  `B filtering for A's workspace id returns nothing (got ${filtered.body.totalItems})`,
);

console.log(fail === 0 ? '\nALL PASS' : `\n${fail} FAILURE(S)`);
process.exit(fail === 0 ? 0 : 1);
