#!/usr/bin/env bash
# Online space reclaim for analytics.db while the service runs.
# Needs auto_vacuum=INCREMENTAL (set once by an offline `VACUUM`); other DBs are skipped.
# Frees pages in small chunks so the single writer is never blocked for long.
set -euo pipefail

CHUNK_PAGES="${CHUNK_PAGES:-2000}"
MAX_CHUNKS="${MAX_CHUNKS:-500}"
ROOT="${STAPLER_SQUAD_HOME:-$HOME/.stapler-squad}"

shopt -s nullglob
dbs=("$@")
[ ${#dbs[@]} -eq 0 ] && dbs=("$ROOT"/workspaces/*/analytics.db "$ROOT"/analytics.db)

for db in "${dbs[@]}"; do
  [ -f "$db" ] || continue
  mode=$(sqlite3 "$db" "PRAGMA auto_vacuum;")
  if [ "$mode" != "2" ]; then
    echo "skip $db: auto_vacuum=$mode (run an offline VACUUM after PRAGMA auto_vacuum=INCREMENTAL)"
    continue
  fi
  before=$(sqlite3 "$db" "PRAGMA freelist_count;")
  for ((i = 0; i < MAX_CHUNKS; i++)); do
    left=$(sqlite3 "$db" ".timeout 5000" "PRAGMA incremental_vacuum($CHUNK_PAGES); PRAGMA freelist_count;" | tail -n 1)
    [ "$left" -eq 0 ] && break
    sleep 1
  done
  sqlite3 "$db" ".timeout 5000" "PRAGMA wal_checkpoint(PASSIVE);" >/dev/null
  echo "$(date -u +%FT%TZ) $db freelist $before -> $(sqlite3 "$db" 'PRAGMA freelist_count;')"
done
