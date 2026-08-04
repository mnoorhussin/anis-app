# Status — 4 August 2026

Where the product actually is, what is verified, and what is not. Written to be
read first by anyone (or any session) picking this up cold.

## Built and tested

| Area                                                      | State                                                    |
| --------------------------------------------------------- | -------------------------------------------------------- |
| Monorepo, tokens, CI tasks                                | Done — `pnpm check` runs 21 tasks green                  |
| Signup → account, workspace, owner membership, widget key | Done, atomic with the user row                           |
| Sources: text, FAQ                                        | Done                                                     |
| Sources: website crawl (robots, boilerplate, refresh)     | Done                                                     |
| Sources: PDF                                              | Extraction done; **Arabic PDFs are refused** — see below |
| Retrieval, confidence floor, refusal                      | Done, floor is a parameter                               |
| Widget: chat, streaming, RTL per message                  | Done                                                     |
| Escalation: lead capture, notification                    | Done                                                     |
| Inbox: conversations, takeover, human reply               | Done                                                     |
| Live: agent replies pushed to the visitor                 | Done                                                     |
| Usage metering + hard spending cap                        | Done                                                     |
| Stripe: checkout, webhooks, portal, billing screen        | Done — webhook path verified, API calls not              |

Not started: analytics and knowledge-gap **views** (the data is already being
recorded), WhatsApp/Messenger, advanced actions, agency multi-workspace,
instant-demo generator.

## The one thing to understand before continuing

**Nothing here has run on a real embedding model or a real LLM.** Every test
passes against a hash-based embedder with no semantic understanding and a
provider that emits no language.

A concrete illustration from testing: an off-topic Arabic question was
_answered_ rather than refused, because it happened to share the token "كم"
with an FAQ entry. That is the fake embedder working as designed, not a bug.

So `answer.DefaultFloor = 0.35` is a **guess**. It is labelled unvalidated in
the code, and every decision logs the similarity it saw so it can be fitted to
real traffic later. When `VOYAGE_API_KEY` and `ANTHROPIC_API_KEY` arrive, the
first job is measuring that floor against a labelled Arabic + English question
set — not building the next feature.

## Known gaps, stated plainly

- **Arabic PDFs are refused.** PDFs store glyphs in visual order; extraction
  returns reversed text that indexes cleanly and matches nothing. Reversing the
  runs does not reliably repair it, so the product refuses with an actionable
  message rather than indexing garbage. Latin PDFs work.
- **Stripe's API is uncalled.** Webhook handling — where correctness lives — is
  fully tested against locally-signed payloads. Session creation needs live
  credentials and one rehearsal via `stripe listen`.
- **The live hub is single-process.** It fits the one-binary OVH deployment
  exactly, but a second process would mean a visitor connected to one never
  sees a reply written on the other, and it would fail silently. Documented at
  the top of `internal/live`.
- **Litestream against OVH Object Storage is unverified.** OVH is not in
  Litestream's compatibility docs. Confirm LTX objects appear and run a
  `restore -dry-run` before trusting it. And the restore path itself has never
  been rehearsed.
- **Backups need two mechanisms.** Litestream covers SQLite only; uploaded
  files need PocketBase's own backup. Neither alone is sufficient.

## Traps that will cost hours if rediscovered

Recorded in code comments at each site, and worth knowing up front:

- `ncruces/go-sqlite3` **must stay at v0.20.0**. Newer versions compile and then
  panic, or do not compile. `go get -u` breaks vector search.
- Build with **`-tags no_default_driver`** or PocketBase can silently fall back
  to a SQLite without vector support.
- **PocketBase `Required` on a number field means "non-zero"** — a counter
  starting at 0 cannot be saved. This silently broke usage metering, and would
  have made a spending cap of 0 ("never bill me overage") unsaveable.
- **PocketBase returns a JSON field as `types.JSONRaw`**, not a map. Asserting
  `.(map[string]any)` compiles, always fails, and silently discards a
  workspace's entire widget configuration.
- **Arabic tashkeel is `Script=Inherited`**, not `Script=Arabic`. The obvious
  normalisation regex matches nothing.
- **Stripe signatures cover the RAW body.** Any decode/re-encode before
  verification fails every event and looks like a wrong secret.
- **stripe-go v86 pins API version `2026-07-29.dahlia`** and refuses events from
  another release train.

## Verifying a change

```bash
pnpm check                 # format, lint, typecheck, unit tests — all packages
```

For anything touching HTTP, tenancy, streaming or billing, also run the
end-to-end suite in [`scripts/e2e/`](scripts/e2e/README.md). Several bugs in
this repo were caught only there and would otherwise have shipped: a silently
failing usage counter, orphaned vectors surviving a delete, a handoff button
vanishing after a reconnect, and billing writes resolving the wrong account.

## Suggested next steps

1. **Get the two API keys**, then measure the confidence floor. Everything
   downstream of retrieval is guesswork until this happens.
2. Analytics and knowledge-gap views — presentation over data already recorded
   honestly (`RESOLUTION_SIGNALS` has no "abandoned" member by design).
3. Rehearse a Stripe checkout end to end with `stripe listen`.
4. Rehearse a Litestream restore on a scratch box.
