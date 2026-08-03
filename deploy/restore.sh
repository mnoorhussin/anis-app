#!/usr/bin/env bash
#
# Disaster recovery. Run on the OVH box as root.
#
# Test this on a scratch box BEFORE you need it. An untested restore is not a
# backup; it is a hope.
set -euo pipefail

PB_DATA=/var/lib/anis/pb_data

systemctl stop anis
# Never restore underneath a running replicator.
systemctl stop litestream

# Restore both databases. PocketBase has exactly two; there is no logs.db.
# -force also clears stale -wal/-shm sidecars.
for db in data.db auxiliary.db; do
  litestream restore -config /etc/litestream.yml -force \
    -integrity-check quick \
    "${PB_DATA}/${db}"
done

# Point in time instead of latest:
#   litestream restore -config /etc/litestream.yml -force \
#     -timestamp 2026-08-02T13:45:00Z -o /tmp/data.db "${PB_DATA}/data.db"
#
# Inspect the plan without writing anything:
#   litestream restore -config /etc/litestream.yml -dry-run "${PB_DATA}/data.db"
#
# From a bare machine with no config file:
#   litestream restore -o "${PB_DATA}/data.db" \
#     "s3://anis-pb-backups/data.db?endpoint=https://s3.gra.io.cloud.ovh.net&region=gra&force-path-style=true"

chown -R anis:anis "${PB_DATA}"

cat >&2 <<'NOTE'

  This restored the DATABASES ONLY.

  pb_data/storage — every uploaded PDF and logo — is not replicated by
  Litestream. Restore it from the most recent PocketBase backup zip, or the
  database will reference files that do not exist.

NOTE

systemctl start litestream
systemctl start anis
