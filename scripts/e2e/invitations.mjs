// End-to-end check of team invitations and the roles they create.
//
// An agency invites its client into the client's workspace as an admin; the
// client invites an agent. Then every boundary is pushed on: the link only works
// for the invited email and only once, seats are counted across the account and
// re-checked at acceptance, an expired link is dead, an agent works the inbox
// but cannot touch knowledge or settings, nobody can re-point a workspace at
// another account, and none of it is reachable from a different tenant.
//
// Needs a superuser (changing plans, back-dating an expiry). See README.md.
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
const pass = 'correct-horse-1';

const signUp = async (email, name) => {
  await j('/api/collections/users/records', {
    method: 'POST',
    body: JSON.stringify({
      email,
      password: pass,
      passwordConfirm: pass,
      ...(name ? { name } : {}),
    }),
  });
  const a = await j('/api/collections/users/auth-with-password', {
    method: 'POST',
    body: JSON.stringify({ identity: email, password: pass }),
  });
  return { authorization: a.body.token };
};
const tokenOf = (link) => link?.split('/invite/')[1] ?? '';

const su = await j('/api/collections/_superusers/auth-with-password', {
  method: 'POST',
  body: JSON.stringify(SUPERUSER),
});
check(su.status === 200, `superuser auth -> ${su.status}`);
const asSu = { authorization: su.body.token };
const setPlan = (accountId, plan) =>
  j(`/api/collections/accounts/records/${accountId}`, {
    method: 'PATCH',
    headers: asSu,
    body: JSON.stringify({ plan }),
  });

// --- the agency, on free: inviting is not on the plan ----------------------
const owner = await signUp(`agency${stamp}@example.com`, 'وكالة الضوء');
let ov = await j('/api/anis/workspaces', { headers: owner });
const accountId = ov.body.account.id;
const ownWs = ov.body.workspaces[0].id;

let inv = await j(`/api/anis/workspaces/${ownWs}/invitations`, {
  method: 'POST',
  headers: owner,
  body: JSON.stringify({ email: `nobody${stamp}@example.com`, role: 'agent' }),
});
check(
  inv.status === 402 && inv.body.feature === 'multipleMembers',
  `free plan: inviting starts at Growth -> ${inv.status}`,
);

// --- agency plan, a client workspace, invite the client as its admin --------
check((await setPlan(accountId, 'agency')).status === 200, 'raise plan to agency');
const created = await j('/api/anis/workspaces', {
  method: 'POST',
  headers: owner,
  body: JSON.stringify({ name: 'متجر نور' }),
});
const clientWs = created.body.id;
const clientKey = created.body.widget_key;

const clientEmail = `client${stamp}@example.com`;
inv = await j(`/api/anis/workspaces/${clientWs}/invitations`, {
  method: 'POST',
  headers: owner,
  body: JSON.stringify({ email: `  ${clientEmail.toUpperCase()} `, role: 'admin' }),
});
check(inv.status === 200, `owner invites the client as admin -> ${inv.status}`);
check(inv.body.invitation?.email === clientEmail, `the address is stored normalised`);
check(typeof inv.body.emailed === 'boolean', `the response says whether mail went out`);
const clientToken = tokenOf(inv.body.link);
check(clientToken.startsWith('inv_'), `the inviter gets a working link to share`);

for (const [body, want, why] of [
  [{ email: 'not-an-email', role: 'agent' }, 400, 'an invalid address'],
  [{ email: `x${stamp}@example.com`, role: 'owner' }, 403, 'inviting someone as owner'],
  [{ email: `agency${stamp}@example.com`, role: 'agent' }, 409, 'inviting an existing member'],
]) {
  const r = await j(`/api/anis/workspaces/${clientWs}/invitations`, {
    method: 'POST',
    headers: owner,
    body: JSON.stringify(body),
  });
  check(r.status === want, `${why} is refused -> ${r.status} (want ${want})`);
}

let team = await j(`/api/anis/workspaces/${clientWs}/team`, { headers: owner });
check(team.status === 200, `owner sees the team -> ${team.status}`);
check(team.body.invitations?.length === 1, `one pending invitation`);
check(
  team.body.seats?.used === 2 && team.body.seats?.limit === 25,
  `a pending invitation holds a seat (used ${team.body.seats?.used}/${team.body.seats?.limit})`,
);

// --- the link: preview, wrong person, right person, single use --------------
let pv = await j(`/api/anis/invitations/preview/${clientToken}`);
check(pv.status === 200, `preview without signing in -> ${pv.status}`);
check(pv.body.workspace_name === 'متجر نور', `preview names the workspace (Arabic intact)`);
check(pv.body.email === clientEmail && pv.body.role === 'admin', `preview says who and as what`);
pv = await j(`/api/anis/invitations/preview/inv_${'A'.repeat(52)}`);
check(pv.status === 404, `an unknown token is a 404 -> ${pv.status}`);

const mallory = await signUp(`mallory${stamp}@example.com`);
let acc = await j('/api/anis/invitations/accept', {
  method: 'POST',
  headers: mallory,
  body: JSON.stringify({ token: clientToken }),
});
check(acc.status === 403, `someone else holding the link cannot accept it -> ${acc.status}`);
check(acc.body.email === clientEmail, `and is told which address it is for`);

// Signed up with different casing: the address still matches.
const client = await signUp(`Client${stamp}@Example.com`, 'نور');
acc = await j('/api/anis/invitations/accept', {
  method: 'POST',
  headers: client,
  body: JSON.stringify({ token: clientToken }),
});
check(acc.status === 200 && acc.body.role === 'admin', `the client accepts -> ${acc.status}`);
acc = await j('/api/anis/invitations/accept', {
  method: 'POST',
  headers: client,
  body: JSON.stringify({ token: clientToken }),
});
check(acc.status === 404, `the link is single-use -> ${acc.status}`);

ov = await j('/api/anis/workspaces', { headers: client });
const shared = ov.body.shared?.find((w) => w.id === clientWs);
check(!!shared && shared.role === 'admin', `the client's switcher lists the workspace they joined`);
check(shared?.account_name === 'وكالة الضوء', `labelled with the agency's name`);
check(
  !ov.body.workspaces.some((w) => w.id === clientWs),
  `but it is not counted as theirs to bill`,
);

// --- the client, as admin, runs their workspace and invites an agent --------
let r = await j('/api/anis/sources', {
  method: 'POST',
  headers: client,
  body: JSON.stringify({
    workspace: clientWs,
    type: 'text',
    title: 'الأسعار',
    body: 'التوصيل مجاني فوق ٢٠٠ ريال.',
  }),
});
check(r.status === 200, `an admin can add knowledge -> ${r.status}`);
const sourceId = r.body.id;

r = await j(`/api/collections/workspaces/records/${clientWs}`, {
  method: 'PATCH',
  headers: client,
  body: JSON.stringify({ allowed_domains: ['noor.example.com'] }),
});
check(r.status === 200, `an admin can change settings -> ${r.status}`);

// The hole this migration closes: re-pointing a workspace at another account.
const clientOwnAccount = ov.body.account.id;
r = await j(`/api/collections/workspaces/records/${clientWs}`, {
  method: 'PATCH',
  headers: client,
  body: JSON.stringify({ account: clientOwnAccount }),
});
const after = await j(`/api/collections/workspaces/records/${clientWs}`, { headers: asSu });
check(
  r.status !== 200 && after.body.account === accountId,
  `nobody can move a workspace to another account (got ${r.status}, still under the agency: ${after.body.account === accountId})`,
);
r = await j(`/api/collections/workspaces/records/${ownWs}`, {
  method: 'PATCH',
  headers: owner,
  body: JSON.stringify({ widget_key: 'wk_CHOSENBYTHECALLER000000000' }),
});
check(r.status !== 200, `not even the owner can choose a widget key -> ${r.status}`);

r = await j(`/api/anis/workspaces/${clientWs}/invitations`, {
  method: 'POST',
  headers: client,
  body: JSON.stringify({ email: `other-admin${stamp}@example.com`, role: 'admin' }),
});
check(r.status === 403, `an admin cannot create another admin -> ${r.status}`);

const agentEmail = `agent${stamp}@example.com`;
inv = await j(`/api/anis/workspaces/${clientWs}/invitations`, {
  method: 'POST',
  headers: client,
  body: JSON.stringify({ email: agentEmail, role: 'agent' }),
});
check(inv.status === 200, `an admin invites an agent -> ${inv.status}`);
const agent = await signUp(agentEmail);
acc = await j('/api/anis/invitations/accept', {
  method: 'POST',
  headers: agent,
  body: JSON.stringify({ token: tokenOf(inv.body.link) }),
});
check(acc.status === 200 && acc.body.role === 'agent', `the agent accepts -> ${acc.status}`);

// --- what an agent can and cannot do ----------------------------------------
r = await j('/api/anis/sources', {
  method: 'POST',
  headers: agent,
  body: JSON.stringify({ workspace: clientWs, type: 'text', title: 't', body: 'x' }),
});
check(r.status === 403, `an agent cannot add knowledge -> ${r.status}`);
r = await j(`/api/collections/sources/records/${sourceId}`, { method: 'DELETE', headers: agent });
const stillThere = await j(`/api/collections/sources/records/${sourceId}`, { headers: asSu });
check(
  r.status !== 204 && stillThere.status === 200,
  `an agent cannot delete knowledge -> ${r.status}`,
);
r = await j(`/api/collections/workspaces/records/${clientWs}`, {
  method: 'PATCH',
  headers: agent,
  body: JSON.stringify({ name: 'hijacked' }),
});
check(r.status !== 200, `an agent cannot change settings -> ${r.status}`);
r = await j(`/api/anis/workspaces/${clientWs}/team`, { headers: agent });
check(r.status === 403, `an agent does not see the team list -> ${r.status}`);
r = await j(`/api/anis/analytics?workspace=${clientWs}`, { headers: agent });
check(r.status === 200, `an agent sees the analytics -> ${r.status}`);

// A visitor arrives; the agent takes over and replies.
const res = await fetch(`${API}/api/anis/widget/${clientKey}/message`, {
  method: 'POST',
  headers: { 'content-type': 'application/json', origin: 'https://noor.example.com' },
  body: JSON.stringify({ text: 'هل التوصيل مجاني؟', visitor: 'v-inv' }),
});
const raw = await res.text();
const done = raw
  .split('\n\n')
  .filter((b) => b.startsWith('event: done'))
  .map((b) => JSON.parse(/^data: (.+)$/m.exec(b)[1]))[0];
const conv = done?.conversationId;
check(res.status === 200 && !!conv, `a visitor starts a conversation -> ${res.status}`);
r = await j(`/api/collections/conversations/records/${conv}`, {
  method: 'PATCH',
  headers: agent,
  body: JSON.stringify({ status: 'human' }),
});
check(r.status === 200, `the agent takes it over -> ${r.status}`);
r = await j('/api/collections/messages/records', {
  method: 'POST',
  headers: agent,
  body: JSON.stringify({
    workspace: clientWs,
    conversation: conv,
    role: 'human',
    text: 'نعم، فوق ٢٠٠ ريال.',
  }),
});
check(r.status === 200, `the agent replies to the visitor -> ${r.status}`);

// --- the plan: invitations start at Growth, re-checked at acceptance --------
const late = `late${stamp}@example.com`;
inv = await j(`/api/anis/workspaces/${clientWs}/invitations`, {
  method: 'POST',
  headers: owner,
  body: JSON.stringify({ email: late, role: 'agent' }),
});
check(inv.status === 200, `an invitation sent while on agency -> ${inv.status}`);
const lateLink = inv.body.link;

check((await setPlan(accountId, 'starter')).status === 200, 'downgrade to starter');
const invite = (email) =>
  j(`/api/anis/workspaces/${ownWs}/invitations`, {
    method: 'POST',
    headers: owner,
    body: JSON.stringify({ email, role: 'agent' }),
  });
r = await invite(`more${stamp}@example.com`);
check(
  r.status === 402 && r.body.feature === 'multipleMembers',
  `starter cannot invite at all — it is a Growth feature -> ${r.status} (${r.body.feature})`,
);
const lateUser = await signUp(late);
acc = await j('/api/anis/invitations/accept', {
  method: 'POST',
  headers: lateUser,
  body: JSON.stringify({ token: tokenOf(lateLink) }),
});
check(acc.status === 402, `a pending invitation stops working below Growth -> ${acc.status}`);

// --- seats: Growth has 5, counted across the account ------------------------
// The owner, the client and the agent are members; `late` holds the fourth.
check((await setPlan(accountId, 'growth')).status === 200, 'move to growth (5 seats, 4 taken)');
r = await invite(`fifth${stamp}@example.com`);
check(r.status === 200, `growth: the fifth seat can be filled -> ${r.status}`);
r = await invite(`sixth${stamp}@example.com`);
check(
  r.status === 402 && r.body.max_members === 5,
  `growth: no sixth seat (a seat limit, not the feature) -> ${r.status}`,
);
await setPlan(accountId, 'agency');

// --- expiry and revocation --------------------------------------------------
const exp = await j(`/api/anis/workspaces/${clientWs}/invitations`, {
  method: 'POST',
  headers: owner,
  body: JSON.stringify({ email: `slow${stamp}@example.com`, role: 'agent' }),
});
await j(`/api/collections/invitations/records/${exp.body.invitation.id}`, {
  method: 'PATCH',
  headers: asSu,
  body: JSON.stringify({ expires: '2020-01-01 00:00:00.000Z' }),
});
pv = await j(`/api/anis/invitations/preview/${tokenOf(exp.body.link)}`);
check(pv.status === 410, `an expired link says so -> ${pv.status}`);
const slow = await signUp(`slow${stamp}@example.com`);
acc = await j('/api/anis/invitations/accept', {
  method: 'POST',
  headers: slow,
  body: JSON.stringify({ token: tokenOf(exp.body.link) }),
});
check(acc.status === 410, `and cannot be accepted -> ${acc.status}`);

r = await j(`/api/anis/invitations/${exp.body.invitation.id}`, {
  method: 'DELETE',
  headers: agent,
});
check(r.status === 403, `an agent cannot revoke invitations -> ${r.status}`);
r = await j(`/api/anis/invitations/${exp.body.invitation.id}`, {
  method: 'DELETE',
  headers: owner,
});
check(r.status === 204, `the owner revokes it -> ${r.status}`);
pv = await j(`/api/anis/invitations/preview/${tokenOf(exp.body.link)}`);
check(pv.status === 404, `a revoked link is dead -> ${pv.status}`);

// --- tenancy: another account reaches none of this --------------------------
r = await j(`/api/anis/workspaces/${clientWs}/team`, { headers: mallory });
check(r.status === 404, `another tenant cannot see the team -> ${r.status}`);
r = await j(`/api/anis/workspaces/${clientWs}/invitations`, {
  method: 'POST',
  headers: mallory,
  body: JSON.stringify({ email: `mallory${stamp}@example.com`, role: 'agent' }),
});
check(r.status === 404, `or invite themselves in -> ${r.status}`);

// --- removing people --------------------------------------------------------
team = await j(`/api/anis/workspaces/${clientWs}/team`, { headers: owner });
const ownerId = team.body.members.find((m) => m.role === 'owner').user_id;
const agentId = team.body.members.find((m) => m.role === 'agent').user_id;
r = await j(`/api/anis/workspaces/${clientWs}/members/${ownerId}`, {
  method: 'DELETE',
  headers: client,
});
check(r.status === 403, `an admin cannot remove the owner -> ${r.status}`);
r = await j(`/api/anis/workspaces/${clientWs}/members/${agentId}`, {
  method: 'DELETE',
  headers: client,
});
check(r.status === 204, `an admin removes the agent -> ${r.status}`);
ov = await j('/api/anis/workspaces', { headers: agent });
check(
  !ov.body.shared?.some((w) => w.id === clientWs),
  `the agent has lost access to the workspace`,
);
r = await j(`/api/anis/analytics?workspace=${clientWs}`, { headers: agent });
check(r.status === 404, `and its data -> ${r.status}`);

console.log(fail ? `\n${fail} check(s) FAILED` : `\nall invitation checks passed`);
process.exit(fail ? 1 : 0);
