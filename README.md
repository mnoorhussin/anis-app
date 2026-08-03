# anis-app

The product behind [anis.chat](https://anis.chat) — **Anis (أنيس)**, Arabic-first AI customer support.

The marketing site lives in a separate repo (`anis-chat`). This one is the
thing it sells: the dashboard at `app.anis.chat`, the embeddable widget at
`cdn.anis.chat`, and the backend at `api.anis.chat`.

> **Status: scaffold.** The structure, tooling, schema and deployment are real
> and verified. No product feature is implemented yet. Endpoints that do not
> exist return `501` rather than pretending.

## The rule that governs this repo

Every marketing claim must map to a working feature. When something is not
real, the site hides it or marks it `Soon` — and the product refuses to enable
it, rather than half-working.

Concretely, and these are not negotiable:

- The assistant answers **only** from the business's approved sources, and says
  so plainly when it cannot. A refusal is a success state, not an error.
- "Auto-resolved" requires a defensible signal — a thumbs-up, a completed
  action, an explicit confirmation. Never inferred from a visitor closing the
  tab. See `RESOLUTION_SIGNALS` in `packages/types`.
- No "no hallucinations", no "<1s", no hard "40+ languages", no resolution
  percentage until it is measured.
- No "GDPR compliant" badge until the technical, contractual and operational
  work is actually done. Data is _stored_ in France; message text is
  _processed_ by the model provider, currently in the United States. Both facts
  get stated.
- Features sold on a plan but not yet built are listed in `NOT_YET_SHIPPED` and
  `featureIsAvailable()` returns false for them — an entitlement alone cannot
  switch one on.

## Layout

```
apps/
  dashboard/    Vite + React SPA          → app.anis.chat
  widget/       Preact in a shadow root   → cdn.anis.chat
pocketbase/     Go-extended PocketBase    → api.anis.chat
packages/
  tokens/       design tokens + fonts + logo (from the marketing site)
  types/        domain vocabulary, plan catalogue, language detection
  ui/           React components (dashboard only)
  config/       shared ESLint / Prettier / TypeScript config
deploy/         Caddyfile, systemd units, Litestream, deploy + restore scripts
```

## Getting started

Requires Node 22+, pnpm 10+, and Go 1.24+.

```bash
pnpm install
pnpm dev
```

That starts three processes: the dashboard on `:5173`, the widget harness on
`:5174`, and PocketBase on `:8090`. Open `http://127.0.0.1:8090/_/` to create
the first superuser; migrations run automatically on first boot.

```bash
pnpm check    # format:check + lint + typecheck + test, everything
pnpm build
```

The widget dev page (`:5174`) is deliberately a hostile fake customer site —
Comic Sans, aggressive resets, a high-`z-index` banner. If the widget survives
it, it will survive a real storefront.

## Decisions worth knowing before you change something

### Everything is one SQLite file

PocketBase is used as a Go framework, not as a binary. It provides the database,
auth, file storage, realtime and admin UI; `pocketbase/routes` adds what needs a
secret or a model call. One process, one file, one backup story.

### sqlite-vec, and a load-bearing version pin

Vector search runs inside the same SQLite database, so retrieval and tenancy
live in one query and one transaction.

Getting there requires an exact set of pins, recorded in `pocketbase/go.mod`:

```
github.com/ncruces/go-sqlite3 v0.20.0    // DO NOT BUMP
github.com/tetratelabs/wazero v1.8.1
github.com/asg017/sqlite-vec-go-bindings v0.1.6
```

`ncruces/go-sqlite3` v0.22–v0.32 compile and then **panic at runtime on the
first query**. v0.33+ does not compile at all: it dropped the `sqlite3.Binary`
extension point that every sqlite-vec binding depends on. The bindings' WebAssembly
build has not been rebuilt since early 2025 and there is no upstream fix. This
was verified here, not assumed.

That is a real liability — an old SQLite build (3.47.0) pinned indefinitely — and
it should be revisited. The migration target is `ncruces/go-sqlite3/ext/vec1`,
SQLite's own vector extension: current, maintained in lockstep with the driver,
CGO-free. It is a schema and query rewrite rather than a dependency swap, and
its KNN is global-top-k with no partition key, so tenant scoping becomes
over-fetch-and-filter — measurably worse for multi-tenancy, which is why
sqlite-vec won for now.

The build must use `-tags no_default_driver`. Without it PocketBase also links
`modernc.org/sqlite`, and a mistake in wiring `DBConnect` would silently fall
back to a SQLite with no vector support. With the tag it panics instead.
`db.AssertVecAvailable` re-checks at startup, because a stock PocketBase binary
opening the same `pb_data` produces exactly this failure — the app boots fine
and every vector query fails, which looks like a bad knowledge base rather than
a misconfiguration.

### Tenant isolation is enforced twice

Collections carry API rules scoped through `memberships` — see
`pocketbase/migrations`. But **custom Go routes bypass collection rules
entirely**: inside a handler you are a superuser. So `rag.Retriever` takes a
workspace ID it will not run without, and the vec0 table uses
`workspace_id TEXT PARTITION KEY` so the tenant filter is a genuine pre-filter
rather than a filter applied after global nearest-neighbour search.

A note on the rules themselves: they use `?=` ("any of"), not `=`. With `=`,
access is denied as soon as a workspace has a second member — it fails in the
safe direction, so it survives testing by one developer and breaks for the
first real team.

### TypeScript 6, not 7

TypeScript 7 is `latest`, but `typescript-eslint` requires `<6.1.0`. Lint
working matters more than the newest major. Revisit when typescript-eslint
ships TS 7 support.

### Internal packages are consumed as source

`@anis/tokens`, `@anis/types` and `@anis/ui` have no build step — their
`exports` point straight at `.ts`, and the apps' bundlers compile them. A token
change shows up in `pnpm dev` without rebuilding anything first.

Two consequences. `packages/config/tsconfig/base.json` is self-contained rather
than extending a root file: it is reached through a pnpm symlink, and a
relative `extends` out of the package resolves against the symlink path — `tsc`
copes, Vite's oxc transform does not. And `apps/dashboard/src/app.css` needs an
explicit `@source` pointing at `packages/ui/src`, because Tailwind does not
scan into workspace dependencies; without it, utilities used only inside `ui`
are missing from the production stylesheet.

`turbo.json` also lists `globalPassThroughEnv` for `LOCALAPPDATA`, `GOCACHE`
and friends. Turborepo runs tasks in a stripped environment, and without those
`go build` fails with "build cache is required, but could not be located".
They are passthrough rather than `globalEnv` because they are machine-specific
paths that must not contribute to the cache hash.

### The widget shares tokens with the dashboard, not components

It runs Preact in a shadow root with a hard size budget (`pnpm --filter
@anis/widget size`; currently ~35 KB raw, ~10 KB brotli). It does not import
`@anis/ui`, and it ships **no webfonts** — partly because 350 KB on someone
else's storefront is unacceptable, and partly because `@font-face` is
document-scoped and simply does not apply inside a shadow root.

Its stylesheet is imported with Vite's `?inline` so it can be injected _into_
the shadow root. A CSS-injection plugin would put it in `document.head`, which
both misses the shadow tree and leaks onto the host page. And because a shadow
root inherits none of the page's custom properties, `:host` variables are
stamped at runtime from `@anis/tokens` — which is also what makes each
workspace's accent colour work.

### Arabic is handled per message, not per conversation

Gulf and Levantine customers code-switch mid-sentence ("عندكم iPhone 15 Pro
بالمخزون؟"), so direction and font are decided per message. The Latin font
stacks list the Arabic face as a fallback, so a mixed string resolves per-glyph
to the right face.

Detection is implemented twice — `packages/types/src/language.ts` and
`pocketbase/internal/lang` — against a shared fixture table, because a message
the widget lays out RTL and the retriever treats as English is a bug that only
shows up in production, in a language most of the team cannot read.

One Unicode trap, hit in both languages and documented in both: Arabic
diacritics are `Script=Inherited`, **not** `Script=Arabic`, so the obvious
spelling of the normalisation regex matches nothing and silently does nothing.
The obvious fix — strip all `Mn` — turns a decomposed "café" into "cafe" and
breaks retrieval for French sources, which a France-hosted product will have.

## Deployment

One OVH box in France. See [`deploy/README.md`](deploy/README.md) — including
the part about backups needing **two** mechanisms, since Litestream replicates
SQLite and does not touch a single uploaded file.

## Reference

- `docs/PRODUCT-REQUIREMENTS.md` in the `anis-chat` repo — the full spec and the
  claim→feature map.
- `packages/types/src/plans.ts` — plan limits and prices, which must stay in
  step with the marketing site's `src/i18n/en.ts`.
