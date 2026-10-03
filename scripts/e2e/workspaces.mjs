// End-to-end check of the agency workspace roster: the plan ceiling, creating a
// client workspace, and that creation lands under the caller's own account and
// never leaks across tenants. Then deleting one: refused across tenants, its
// content and widget gone, and the owner's last workspace kept.
//
// Over real HTTP with proper UTF-8, because that is where tenancy-on-custom-
// routes bugs actually surface — a create that resolves the wrong account would
// pass every unit test and bill the wrong customer.
//
// Needs a superuser to raise the plan, because changing a plan is deliberately
// not something a customer can do. See scripts/e2e/README.md.
const API = 'http://127.0.0.1:8090';
const SUPERUSER = { identity: 'test@anis.chat', password: 'testtesttest' };

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

let fail = 0;
const check = (ok, msg) => {
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${msg}`);
  if (!ok) fail++;
};

const stamp = Date.now();
const owner = `ws${stamp}@example.com`;
const other = `wsb${stamp}@example.com`;
const pass = 'correct-horse-1';

// --- an account starts on free: one workspace, no room for another ---------
await j('/api/collections/users/records', {
  method: 'POST',
  body: JSON.stringify({
    email: owner,
    password: pass,
    passwordConfirm: pass,
    name: 'وكالة الاختبار',
  }),
});
const auth = await j('/api/collections/users/auth-with-password', {
  method: 'POST',
  body: JSON.stringify({ identity: owner, password: pass }),
});
check(auth.status === 200, `owner auth -> ${auth.status}`);
const token = auth.body.token;

let ov = await j('/api/anis/workspaces', { headers: { authorization: token } });
check(ov.status === 200, `overview -> ${ov.status}`);
check(ov.body.account?.plan === 'free', `starts on the free plan`);
check(
  ov.body.workspaces?.length === 1,
  `one workspace to start (saw ${ov.body.workspaces?.length})`,
);
check(ov.body.account?.slots_left === 0, `free: no workspace slots left`);
check(ov.body.account?.client_workspaces === false, `free: agency UX is off`);
const accountId = ov.body.account.id;

// Per-workspace usage fields are present (the "separate usage tracking" the
// agency screen shows), even if zero for a fresh workspace.
const first = ov.body.workspaces[0];
check(
  typeof first.replies === 'number' && typeof first.conversations === 'number',
  `per-workspace usage is reported`,
);

// --- free plan refuses a second workspace ----------------------------------
let c = await j('/api/anis/workspaces', {
  method: 'POST',
  headers: { authorization: token },
  body: JSON.stringify({ name: 'عميل' }),
});
check(c.status === 402, `free create is blocked with a plan ceiling -> ${c.status}`);

// --- raise the plan to agency (superuser only) -----------------------------
const su = await j('/api/collections/_superusers/auth-with-password', {
  method: 'POST',
  body: JSON.stringify(SUPERUSER),
});
check(su.status === 200, `superuser auth -> ${su.status}`);
const up = await j(`/api/collections/accounts/records/${accountId}`, {
  method: 'PATCH',
  headers: { authorization: su.body.token },
  body: JSON.stringify({ plan: 'agency' }),
});
check(up.status === 200, `raise plan to agency -> ${up.status}`);

ov = await j('/api/anis/workspaces', { headers: { authorization: token } });
check(ov.body.account?.plan === 'agency', `now on agency`);
check(ov.body.account?.client_workspaces === true, `agency: client UX is on`);
check(
  ov.body.account?.slots_left === 19,
  `agency: 19 slots left (saw ${ov.body.account?.slots_left})`,
);

// --- create a client workspace ---------------------------------------------
c = await j('/api/anis/workspaces', {
  method: 'POST',
  headers: { authorization: token },
  body: JSON.stringify({ name: 'متجر نور' }),
});
check(c.status === 200, `agency create -> ${c.status}`);
check(c.body.role === 'owner', `the creator is the owner of the new workspace`);
check(
  typeof c.body.widget_key === 'string' && c.body.widget_key.length > 0,
  `new workspace has a widget key`,
);
const newWsId = c.body.id;
const newWsKey = c.body.widget_key;

ov = await j('/api/anis/workspaces', { headers: { authorization: token } });
check(
  ov.body.workspaces?.length === 2,
  `two workspaces after create (saw ${ov.body.workspaces?.length})`,
);
check(
  ov.body.workspaces.every((w) => w.role === 'owner'),
  `owner of both workspaces`,
);

const wrec = await j(`/api/collections/workspaces/records/${newWsId}?expand=account`, {
  headers: { authorization: token },
});
check(wrec.body.account === accountId, `the new workspace is under the same account`);

// --- a blank name is rejected ----------------------------------------------
const blank = await j('/api/anis/workspaces', {
  method: 'POST',
  headers: { authorization: token },
  body: JSON.stringify({ name: '   ' }),
});
check(blank.status === 400, `a blank name is a 400 -> ${blank.status}`);

// --- tenancy: a different account never sees or touches this one -----------
await j('/api/collections/users/records', {
  method: 'POST',
  body: JSON.stringify({ email: other, password: pass, passwordConfirm: pass }),
});
const authB = await j('/api/collections/users/auth-with-password', {
  method: 'POST',
  body: JSON.stringify({ identity: other, password: pass }),
});
const ovB = await j('/api/anis/workspaces', { headers: { authorization: authB.body.token } });
check(
  ovB.body.workspaces?.length === 1,
  `B sees only their own workspace (saw ${ovB.body.workspaces?.length})`,
);
check(ovB.body.account?.id !== accountId, `B has a different account`);
check(!ovB.body.workspaces.some((w) => w.id === newWsId), `B cannot see A's client workspace`);

// --- delete -----------------------------------------------------------------
// Give the client workspace indexed content and a live widget first, so the
// delete is checked against something real to lose — a fresh workspace's widget
// already 404s (no allowed domains), which would make "it stops working" vacuous.
const src = await j('/api/anis/sources', {
  method: 'POST',
  headers: { authorization: token },
  body: JSON.stringify({
    workspace: newWsId,
    type: 'text',
    title: 'ساعات العمل',
    body: 'نفتح من التاسعة صباحاً حتى الخامسة مساءً.\n\nنغلق يوم الجمعة.',
  }),
});
check(src.status === 200, `content added to the client workspace -> ${src.status}`);

const ORIGIN = 'https://noor.example.com';
const dom = await j(`/api/collections/workspaces/records/${newWsId}`, {
  method: 'PATCH',
  headers: { authorization: token },
  body: JSON.stringify({ allowed_domains: ['noor.example.com'] }),
});
check(dom.status === 200, `client domain allowed -> ${dom.status}`);
let wcfg = await j(`/api/anis/widget/${newWsKey}/config`, { headers: { origin: ORIGIN } });
check(wcfg.status === 200, `the client's widget is live before the delete -> ${wcfg.status}`);

// Counted as superuser: after the delete the owner's own reads are filtered by
// membership, which is also gone, so an empty result from them would prove
// nothing about whether the rows were actually removed.
const suAuth = { authorization: su.body.token };
const countIn = async (collection, ws) => {
  const filter = encodeURIComponent(`workspace='${ws}'`);
  const r = await j(`/api/collections/${collection}/records?filter=${filter}`, { headers: suAuth });
  return r.body.totalItems;
};
check((await countIn('chunks', newWsId)) > 0, `the client workspace has indexed chunks`);

let d = await j(`/api/anis/workspaces/${newWsId}`, {
  method: 'DELETE',
  headers: { authorization: authB.body.token },
});
check(d.status === 404, `another account cannot delete it (and gets a plain 404) -> ${d.status}`);

d = await j(`/api/anis/workspaces/${newWsId}`, {
  method: 'DELETE',
  headers: { authorization: token },
});
check(d.status === 204, `the owner deletes the client workspace -> ${d.status}`);

ov = await j('/api/anis/workspaces', { headers: { authorization: token } });
check(
  ov.body.workspaces?.length === 1 && !ov.body.workspaces.some((w) => w.id === newWsId),
  `the roster no longer lists it (saw ${ov.body.workspaces?.length})`,
);
check(ov.body.account?.slots_left === 19, `the freed slot is available again`);
for (const col of ['sources', 'chunks', 'memberships']) {
  const n = await countIn(col, newWsId);
  check(n === 0, `its ${col} went with it (saw ${n})`);
}

wcfg = await j(`/api/anis/widget/${newWsKey}/config`, { headers: { origin: ORIGIN } });
check(wcfg.status === 404, `its widget stops working on the client's site -> ${wcfg.status}`);

d = await j(`/api/anis/workspaces/${newWsId}`, {
  method: 'DELETE',
  headers: { authorization: token },
});
check(d.status === 404, `deleting it again is a 404 -> ${d.status}`);

// --- the last workspace the owner owns cannot go ----------------------------
d = await j(`/api/anis/workspaces/${first.id}`, {
  method: 'DELETE',
  headers: { authorization: token },
});
check(d.status === 409, `the owner's only workspace cannot be deleted -> ${d.status}`);
check(typeof d.body.error === 'string' && d.body.error.length > 0, `and the reason is given`);
ov = await j('/api/anis/workspaces', { headers: { authorization: token } });
check(ov.body.workspaces?.length === 1, `it is still there`);

console.log(fail ? `\n${fail} check(s) FAILED` : `\nall workspace checks passed`);
process.exit(fail ? 1 : 0);
