// End-to-end for billing: the webhook over real HTTP, and the owner-only
// dashboard endpoints.
import { createHmac } from 'node:crypto';

const API = 'http://127.0.0.1:8090';
const SECRET = 'whsec_test_secret';
const API_VERSION = '2026-07-29.dahlia';
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

const envelope = (id, type, object) =>
  JSON.stringify({
    id, object: 'event', api_version: API_VERSION, type,
    created: Math.floor(Date.now() / 1000), data: { object },
  });

const sign = (payload, at = Date.now()) => {
  const ts = Math.floor(at / 1000);
  const mac = createHmac('sha256', SECRET).update(`${ts}.${payload}`).digest('hex');
  return `t=${ts},v1=${mac}`;
};

const postWebhook = async (payload, signature) => {
  const res = await fetch(`${API}/api/anis/stripe/webhook`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', 'stripe-signature': signature },
    body: payload,
  });
  return { status: res.status, body: await res.json().catch(() => ({})) };
};

const stamp = Date.now();
const pw = 'correct-horse-1';
const email = `bill${stamp}@example.com`;

await j('/api/collections/users/records', {
  method: 'POST', body: JSON.stringify({ email, password: pw, passwordConfirm: pw }),
});
const token = (await j('/api/collections/users/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: email, password: pw }),
})).body.token;
const ws = (await j('/api/collections/workspaces/records', { headers: { authorization: token } })).body.items[0];
const accountId = ws.account;

// --- summary ------------------------------------------------------------------
let summary = await j('/api/anis/billing', { headers: { authorization: token } });
check(summary.status === 200, `billing summary -> ${summary.status}`);
check(summary.body.plan === 'free', `starts on free (${summary.body.plan})`);
check(summary.body.repliesLimit === 50, `free limit reported (${summary.body.repliesLimit})`);
check(summary.body.hardCapUsd > 0, `spending cap present (${summary.body.hardCapUsd})`);
check(summary.body.billingConfigured === false, 'reports billing is not configured (no STRIPE_SECRET_KEY)');

// --- webhook rejects bad signatures ------------------------------------------
const payload = envelope('evt_reject', 'ping', {});
check((await postWebhook(payload, 'garbage')).status === 400, 'malformed signature rejected');
check((await postWebhook(payload, '')).status === 400, 'missing signature rejected');
check(
  (await postWebhook(payload, sign(payload, Date.now() - 24 * 3600 * 1000))).status === 400,
  'day-old signature rejected (replay protection)',
);

// A body altered after signing must not verify.
const tampered = envelope('evt_reject', 'customer.subscription.deleted', {});
check((await postWebhook(tampered, sign(payload))).status === 400, 'tampered body rejected');

// --- link the customer via checkout.session.completed ------------------------
const checkout = envelope('evt_co_' + stamp, 'checkout.session.completed', {
  id: 'cs_1', customer: { id: 'cus_' + stamp }, subscription: { id: 'sub_' + stamp },
  metadata: { account_id: accountId },
});
let r = await postWebhook(checkout, sign(checkout));
check(r.status === 200, `checkout.session.completed -> ${r.status}`);

// --- upgrade via subscription event -------------------------------------------
const periodEnd = Math.floor(Date.now() / 1000) + 30 * 24 * 3600;
const subActive = envelope('evt_sub_' + stamp, 'customer.subscription.created', {
  id: 'sub_' + stamp, status: 'active', cancel_at_period_end: false,
  customer: { id: 'cus_' + stamp },
  items: { data: [{ price: { id: 'price_growth_test' }, current_period_end: periodEnd }] },
});
r = await postWebhook(subActive, sign(subActive));
check(r.status === 200, `subscription.created -> ${r.status}`);

summary = await j('/api/anis/billing', { headers: { authorization: token } });
check(summary.body.plan === 'growth', `upgraded to growth (got ${summary.body.plan})`);
check(summary.body.repliesLimit === 4000, `limit follows the plan (${summary.body.repliesLimit})`);
check(summary.body.subscriptionStatus === 'active', `status recorded (${summary.body.subscriptionStatus})`);

// --- idempotency: the same event again must change nothing --------------------
r = await postWebhook(subActive, sign(subActive));
check(r.status === 200, `replay -> ${r.status} (must be 2xx or Stripe keeps retrying)`);
check(r.body.duplicate === true, 'replay reported as a duplicate');

// --- past_due keeps the plan ---------------------------------------------------
const pastDue = envelope('evt_pd_' + stamp, 'customer.subscription.updated', {
  id: 'sub_' + stamp, status: 'past_due', cancel_at_period_end: false,
  customer: { id: 'cus_' + stamp },
  items: { data: [{ price: { id: 'price_growth_test' }, current_period_end: periodEnd }] },
});
await postWebhook(pastDue, sign(pastDue));
summary = await j('/api/anis/billing', { headers: { authorization: token } });
check(summary.body.plan === 'growth', `past_due keeps the plan (got ${summary.body.plan})`);
check(summary.body.subscriptionStatus === 'past_due', 'past_due status recorded');

// --- spending cap ---------------------------------------------------------------
r = await j('/api/anis/billing/cap', {
  method: 'POST', headers: { authorization: token }, body: JSON.stringify({ hardCapUsd: 25 }),
});
check(r.status === 200, `owner can set the cap -> ${r.status}`);
summary = await j('/api/anis/billing', { headers: { authorization: token } });
check(summary.body.hardCapUsd === 25, `cap saved (${summary.body.hardCapUsd})`);

// Zero must be saveable: "never bill me overage" is not "no limit".
r = await j('/api/anis/billing/cap', {
  method: 'POST', headers: { authorization: token }, body: JSON.stringify({ hardCapUsd: 0 }),
});
check(r.status === 200, `cap of 0 accepted -> ${r.status}`);
summary = await j('/api/anis/billing', { headers: { authorization: token } });
check(summary.body.hardCapUsd === 0, `cap of 0 persisted (${summary.body.hardCapUsd})`);

r = await j('/api/anis/billing/cap', {
  method: 'POST', headers: { authorization: token }, body: JSON.stringify({ hardCapUsd: -5 }),
});
check(r.status === 400, `negative cap rejected -> ${r.status}`);

// --- cancellation downgrades ----------------------------------------------------
const cancelled = envelope('evt_del_' + stamp, 'customer.subscription.deleted', {
  id: 'sub_' + stamp, status: 'canceled', customer: { id: 'cus_' + stamp },
  items: { data: [{ price: { id: 'price_growth_test' }, current_period_end: periodEnd }] },
});
await postWebhook(cancelled, sign(cancelled));
summary = await j('/api/anis/billing', { headers: { authorization: token } });
check(summary.body.plan === 'free', `cancellation downgrades to free (got ${summary.body.plan})`);

// --- authorisation ---------------------------------------------------------------
const anon = await j('/api/anis/billing');
check(anon.status === 401 || anon.status === 403, `anonymous summary refused -> ${anon.status}`);

const anonCap = await j('/api/anis/billing/cap', {
  method: 'POST', body: JSON.stringify({ hardCapUsd: 9999 }),
});
check(anonCap.status === 401 || anonCap.status === 403, `anonymous cap change refused -> ${anonCap.status}`);

// An agent must not be able to commit the business to a charge.
const agentEmail = `agent${stamp}@example.com`;
await j('/api/collections/users/records', {
  method: 'POST', body: JSON.stringify({ email: agentEmail, password: pw, passwordConfirm: pw }),
});
const agentAuth = await j('/api/collections/users/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: agentEmail, password: pw }),
});
const agentId = agentAuth.body.record.id;
const agentToken = agentAuth.body.token;

const su = await j('/api/collections/_superusers/auth-with-password', {
  method: 'POST', body: JSON.stringify({ identity: 'test@anis.chat', password: 'testtesttest' }),
});
// Add the agent to the OWNER's workspace with role=agent.
await j('/api/collections/memberships/records', {
  method: 'POST', headers: { authorization: su.body.token },
  body: JSON.stringify({ workspace: ws.id, user: agentId, role: 'agent' }),
});

// Signup gives every user their own account, so an invited agent is an owner
// elsewhere. The property that matters is therefore not "an agent gets 403"
// but "an agent cannot spend ANOTHER account's money": billing writes resolve
// the account the caller OWNS, never one they merely have access to.
const agentCap = await j('/api/anis/billing/cap', {
  method: 'POST', headers: { authorization: agentToken }, body: JSON.stringify({ hardCapUsd: 9999 }),
});
check(agentCap.status === 200, `agent cap change applies to their own account -> ${agentCap.status}`);

summary = await j('/api/anis/billing', { headers: { authorization: token } });
check(
  summary.body.hardCapUsd === 0,
  `the owner cap is untouched by the agent (${summary.body.hardCapUsd})`,
);

// And the agent's own account did change, proving the write landed on theirs.
const agentSummary = await j('/api/anis/billing', { headers: { authorization: agentToken } });
check(
  agentSummary.body.hardCapUsd === 9999,
  `the agent changed their OWN cap (${agentSummary.body.hardCapUsd})`,
);

// Checkout resolves the same way: the caller's own account, not the one they
// are an agent in.
const agentCheckout = await j('/api/anis/billing/checkout', {
  method: 'POST', headers: { authorization: agentToken }, body: JSON.stringify({ plan: 'growth' }),
});
check(
  agentCheckout.status === 503 || agentCheckout.status === 200,
  `agent checkout targets their own account -> ${agentCheckout.status}`,
);
const ownerAfter = await j('/api/anis/billing', { headers: { authorization: token } });
check(ownerAfter.body.plan === 'free', `the owner plan is untouched (${ownerAfter.body.plan})`);

console.log(fail === 0 ? '\nALL PASS' : `\n${fail} FAILURE(S)`);
process.exit(fail === 0 ? 0 : 1);
