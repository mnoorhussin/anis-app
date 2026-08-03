#!/usr/bin/env bash
#
# Deploy a new backend build. Run on the OVH box as root.
#
#   ./deploy.sh <git-sha>
#
# Releases are staged into /opt/anis/releases/<sha> and made live by swapping a
# symlink, so a rollback is another symlink swap rather than a rebuild.
#
# True blue/green is not possible here: two PocketBase processes must not share
# one pb_data. Atomic symlink swap plus a fast restart is the correct pattern on
# a single box. Expect a sub-second gap — Caddy's lb_try_duration absorbs it,
# and browsers reconnect their own EventSource streams.
set -euo pipefail

SHA="${1:?usage: deploy.sh <git-sha>}"
REL="/opt/anis/releases/${SHA}"

# 1. Stage the new binary out of band.
install -D -m0755 ./anis "${REL}/anis"

# 2. Flush replication before touching the app. Needs socket.enabled in
#    litestream.yml, or this cannot reach the daemon.
litestream sync -config /etc/litestream.yml || true

# 3. Full consistent snapshot, including uploaded files, which Litestream does
#    not cover. Uses PocketBase's own backup rather than cp: a copy of a live
#    WAL database is silently corrupt.
#
#    Note this BLOCKS WRITES for its duration and needs 2x pb_data free disk.
#    On a large pb_data it is a visible stall, not a free operation.
curl -fsS -X POST http://127.0.0.1:8090/api/backups \
  -H "Authorization: ${PB_SUPERUSER_TOKEN:?set PB_SUPERUSER_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"predeploy_${SHA}.zip\"}" || true

# 4. Remember the current release, then swap atomically. `mv -T` over a symlink
#    is atomic; `ln -sfn` directly onto an existing symlink is not.
readlink -f /opt/anis/current > /opt/anis/.previous || true
ln -sfn "${REL}" /opt/anis/.current.tmp
mv -Tf /opt/anis/.current.tmp /opt/anis/current

# 5. Restart. systemd re-resolves ExecStart, so it picks up the new target.
systemctl restart anis

# 6. Health gate, with rollback.
for _ in $(seq 1 40); do
  if curl -fsS --max-time 2 http://127.0.0.1:8090/api/health >/dev/null; then
    echo "deploy ${SHA} ok"
    exit 0
  fi
  sleep 0.5
done

echo "health check FAILED after 20s — rolling back" >&2
if [ -s /opt/anis/.previous ]; then
  ln -sfn "$(cat /opt/anis/.previous)" /opt/anis/.current.tmp
  mv -Tf /opt/anis/.current.tmp /opt/anis/current
  systemctl restart anis
  echo "rolled back to $(cat /opt/anis/.previous)" >&2
else
  echo "no previous release recorded — manual intervention needed" >&2
fi
exit 1
