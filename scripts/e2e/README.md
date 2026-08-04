# End-to-end checks

These drive a **running** Anis against real HTTP. They are the counterpart to
the Go and Vitest suites, which test units in isolation: everything here
exercises the paths where the interesting bugs actually turned up — raw request
bodies, tenancy on custom routes, streaming, and webhook signatures.

Several bugs in this repo were caught only by these and would have shipped:
a silently-failing usage counter, orphaned vectors surviving a delete, a
handoff button that vanished after a reconnect, and billing writes resolving
the wrong account.

## Running them

Start the backend with the development flags. Without them the app refuses to
run on fake credentials, which is the correct production behaviour and makes
these unrunnable:

```bash
cd pocketbase && ANIS_DEV_FAKE_EMBEDDINGS=1 ANIS_DEV_ECHO_LLM=1 ANIS_ALLOW_PRIVATE_CRAWL=1 STRIPE_WEBHOOK_SECRET=whsec_test_secret STRIPE_PRICE_GROWTH=price_growth_test go run -tags no_default_driver . serve --http=127.0.0.1:8090
```

Some scripts need a superuser (they change an account's plan, which is
deliberately not something a customer can do):

```bash
cd pocketbase && go run -tags no_default_driver . superuser upsert test@anis.chat testtesttest
```

Then, from the repo root:

```bash
node scripts/e2e/signup.mjs
```

Each script is self-contained — it creates its own users and workspaces with a
timestamped email — so they can run in any order against the same database, and
re-running them does not require a reset.

| Script         | Covers                                                                                              |
| -------------- | --------------------------------------------------------------------------------------------------- |
| `signup.mjs`   | Signup provisioning, Arabic names round-tripping, tenant isolation                                  |
| `sources.mjs`  | Text and FAQ ingestion, refusal of unbuilt source types, vector cleanup on delete                   |
| `crawl.mjs`    | Website crawling, robots.txt, boilerplate stripping, refresh replacing rather than duplicating      |
| `chat.mjs`     | Origin allow-list, refusals verbatim in both languages, SSE framing, metering only answered replies |
| `escalate.mjs` | Lead capture, the assistant going silent under human control, message-role forgery refused          |
| `stream.mjs`   | History replay, agent replies reaching the visitor, four authorisation refusals                     |
| `billing.mjs`  | Webhook signatures, idempotent replays, `past_due` keeping the plan, owner-scoped writes            |
| `analytics.mjs`| Ratings resolving (or not) a conversation, the summary's figures, answering a knowledge gap         |

`crawl.mjs` starts its own fake customer site on a random port, which is why
the backend needs `ANIS_ALLOW_PRIVATE_CRAWL=1` — the SSRF guard would otherwise
correctly refuse to fetch loopback.

## What they do NOT cover

Retrieval **quality**. Every one of these passes against a hash-based embedder
with no semantic understanding and a provider that generates no language. They
prove the plumbing is right; they say nothing about whether Anis answers an
Arabic support question well. That needs real API keys and a labelled
question set.

No call is made to Stripe's API either. `billing.mjs` signs its own payloads
with a local secret, which covers the webhook path — where the correctness
actually lives — but not session creation.
