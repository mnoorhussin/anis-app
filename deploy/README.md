# Deploying Anis

One OVH box in France. One Go binary, one SQLite database, Caddy in front.

| Host            | Serves                                              | From                              |
| --------------- | --------------------------------------------------- | --------------------------------- |
| `api.anis.chat` | PocketBase — API, auth, realtime, our custom routes | `127.0.0.1:8090`, reverse-proxied |
| `app.anis.chat` | Dashboard SPA                                       | `/srv/anis/app` (static)          |
| `cdn.anis.chat` | Embeddable widget                                   | `/srv/anis/cdn` (static)          |

Data residency: customer data is stored in France, on hardware we control.
Message text is _processed_ by the model provider — Anthropic, in the United
States, by default. Those are different claims and the privacy page must make
both, plainly. See `internal/llm.Provider.ProcessingRegion`.

## Files here

| File                 | Goes to                                  |
| -------------------- | ---------------------------------------- |
| `anis.service`       | `/etc/systemd/system/anis.service`       |
| `litestream.service` | `/etc/systemd/system/litestream.service` |
| `litestream.yml`     | `/etc/litestream.yml`                    |
| `Caddyfile`          | `/etc/caddy/Caddyfile`                   |
| `deploy.sh`          | `/opt/anis/deploy.sh`                    |
| `restore.sh`         | `/opt/anis/restore.sh`                   |

Secrets are **not** here. `/etc/anis/anis.env` and `/etc/litestream.env` are
created by hand, mode 0600. `.env.example` at the repo root lists what goes in
them.

## First-time setup

```bash
adduser --system --group --home /var/lib/anis anis
mkdir -p /opt/anis/releases /srv/anis/app /srv/anis/cdn /etc/anis
```

Then install the unit files, write the two env files, and:

```bash
systemctl daemon-reload
systemctl enable --now anis litestream caddy
```

Create the first superuser once the service is up:

```bash
/opt/anis/current/anis superuser upsert you@anis.chat 'a-long-password' --dir=/var/lib/anis/pb_data
```

**Set `Settings.TrustedProxy.headers = ["X-Forwarded-For"]` in the admin UI.**
Until you do, PocketBase sees every request as coming from `127.0.0.1`, which
means its rate limiter is limiting Caddy rather than any actual visitor. Leave
`useLeftmostIP` off — a client can prepend its own value to that header.

## Releasing

```bash
# on a build machine
cd pocketbase && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -tags no_default_driver -o anis .

# on the box
./deploy.sh "$(git rev-parse --short HEAD)"
```

`deploy.sh` stages the binary, snapshots, swaps a symlink atomically, restarts,
and rolls back if `/api/health` does not come up within 20 seconds.

Frontends are plain file copies:

```bash
rsync -a --delete apps/dashboard/dist/ root@box:/srv/anis/app/
rsync -a --delete apps/widget/dist/    root@box:/srv/anis/cdn/
```

`CGO_ENABLED=0` is not optional — the whole point of the sqlite-vec setup is
that it needs no C toolchain. `-tags no_default_driver` is also not optional:
without it the binary links a second SQLite build with no vector support and
could silently fall back to it.

## Backups — read this part

**Two mechanisms, both required.** Each covers what the other misses.

**Litestream** replicates the two SQLite databases continuously. Recovery point
is seconds, and you can restore to any point in time. It does **not** touch
`pb_data/storage`, so it does not back up a single uploaded PDF or logo.
Restoring from Litestream alone gives you a database whose file references all
dangle.

**PocketBase's own backup** (`/api/backups`, or scheduled in the admin UI to
OVH Object Storage) produces one self-contained zip _including_ uploaded files.
But it is a full copy every time, its recovery point is the cron interval, and
it blocks all writes while it runs and needs twice `pb_data` free on disk.
Schedule it off-peak.

Run Litestream continuously and the native backup nightly.

Never `cp`, `tar` or `rsync` a live `pb_data`. In WAL mode that produces a torn
database that looks fine until it doesn't. Use `VACUUM INTO`, the backup
endpoint, or stop the service first.

One benign interaction worth recognising: PocketBase's backup issues
`wal_checkpoint(TRUNCATE)`, while Litestream deliberately holds a read
transaction to stop the WAL being checkpointed past its replicated position. So
the checkpoint is partial and the WAL does not shrink as much as it otherwise
would. PocketBase ignores the error by design. This is not corruption.

**Unverified:** OVH is not listed in Litestream's provider-compatibility docs.
Its S3 endpoints accept path-style addressing, but whether `sign-payload: true`
is needed — as it is for Backblaze B2 and Hetzner — has not been confirmed
against a primary source. Before trusting this: start replication, confirm LTX
objects appear in the bucket, and run `litestream restore -dry-run`. If you see
`InvalidArgument` or `MissingContentLength`, uncomment `sign-payload` first.

**Also unverified until someone does it:** the restore path itself. Run
`restore.sh` against a scratch box and confirm the app comes up with real data
before treating any of this as a backup strategy.

## Things that will bite you

- **Never pass a domain to `anis serve`.** With no domain argument it binds
  `127.0.0.1:8090`. Pass one and it switches to `0.0.0.0:80/:443` and starts its
  own Let's Encrypt client, which then fights Caddy for those ports.
- **`--publicDir` and `--indexFallback` do not exist on this binary.** They are
  flags declared by PocketBase's own example `main.go`, not by the framework.
  Ours serves `pb_public` from a fixed path; in production Caddy serves the
  static files anyway.
- **`ProtectSystem=strict` makes the filesystem read-only.** `StateDirectory=`
  is what keeps `/var/lib/anis` writable. Remove it and PocketBase fails at
  startup creating a temp directory.
- **Zero-downtime is not available.** PocketBase's graceful shutdown budget is
  about one second and it cancels the base context first, so in-flight requests
  are severed and every SSE stream drops on restart. Caddy's `lb_try_duration`
  turns that into a retry rather than a 502.
- **Compression on the API host is allowlisted by content type** so it cannot
  buffer `text/event-stream`. Do not simplify it to a bare `encode zstd gzip`.
