# Reclaim SQLite free space (analytics.db, escape_events.db, sessions.db)

SQLite never shrinks a file when rows are deleted; freed pages go on a freelist.
`analytics.db` once reached 2.9 GB with 71% free pages (505702 of 709053 pages at
4096 bytes) because `auto_vacuum` was `NONE`, and shrank to 675 MB (measured by the author, not reproducible from the repo) after an offline
`PRAGMA auto_vacuum=INCREMENTAL; VACUUM;`.

## What the app does

- **New databases** are created with `auto_vacuum=INCREMENTAL`: `session/ent_repository.go`
  (`sessions.db`) and `server/analytics/db.go` (`analytics.db`) set it in the DSN.
  The pragma only takes effect before the first table exists.
- **A background loop** (`session/dbmaint`, started from `server/db_maintenance.go`) runs per
  database every 15 minutes. It reads `freelist_count` and `page_count`. When free pages exceed
  both 20% of all pages and 5000 pages, it runs `PRAGMA incremental_vacuum(2000)` in chunks,
  pausing 250 ms between chunks, then a `wal_checkpoint(PASSIVE)`. Each chunk is a short write
  transaction on the single writer connection, and the pause lets queued writers run. Ticks never
  overlap, and the loop stops when the server context is cancelled.
- **Databases still at `auto_vacuum=NONE`** are never vacuumed by the app (a full `VACUUM`
  blocks the writer for the whole rewrite). The app logs one WARN per database per process with the
  remedy below and reports `needs_offline_vacuum: true`.

The statements run through the same `dbobs`-wrapped driver as normal queries, so slow-query
logging and `/actuator/db` pool stats cover them.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `STAPLER_SQUAD_DB_MAINT_DISABLE` | unset | Any `strconv.ParseBool` true value (`1`, `true`) turns the loop off |
| `STAPLER_SQUAD_DB_MAINT_INTERVAL` | `15m` | Go duration between ticks |
| `STAPLER_SQUAD_DB_MAINT_FREE_RATIO` | `0.20` | Reclaim above this free/total ratio (0 to 1) |
| `STAPLER_SQUAD_DB_MAINT_MIN_FREE_PAGES` | `5000` | ...and above this many free pages |
| `STAPLER_SQUAD_DB_MAINT_CHUNK_PAGES` | `2000` | Pages per `incremental_vacuum` call |
| `STAPLER_SQUAD_DB_MAINT_CHUNK_PAUSE` | `250ms` | Pause between chunks |

Invalid values fall back to the default.

## Observe it

`GET /actuator/db` returns a `maintenance` array (and `/actuator/metrics` a `db_maintenance` array), one
entry per database (`sessions`, `analytics`):

```json
{"name":"analytics","auto_vacuum_mode":"incremental","auto_vacuum_mode_value":2,"needs_offline_vacuum":false,
 "page_count":170000,"free_pages":120,"page_size":4096,"reclaimed_pages_total":505000,
 "last_reclaimed_pages":0,"runs_total":3,"errors_total":0,"last_run":"..."}
```

OpenTelemetry gauges (label `db.name`): `db.maint.free_pages`, `db.maint.page_count`,
`db.maint.reclaimed_pages_total`, `db.maint.last_run_timestamp_seconds`, `db.maint.last_error`
(1 on failure) and `db.maint.auto_vacuum_mode` (0 none, 1 full, 2 incremental). No Grafana dashboard
under `docs/observability/grafana/dashboards/` covers database metrics yet, so there is no panel
for these.

## Convert a database at `auto_vacuum=NONE` (offline)

Check the mode and free space:

```bash
sqlite3 ~/.stapler-squad/analytics.db 'PRAGMA auto_vacuum; PRAGMA freelist_count; PRAGMA page_count;'
```

`0` means `NONE`. Stop the service, then convert. This rewrites the file and needs free disk
roughly equal to the database size:

```bash
sqlite3 <db> 'PRAGMA auto_vacuum=INCREMENTAL; VACUUM;'
```

Restart the service. The in-app loop takes over from there. The directory follows `config.GetConfigDir`
(`workspaces/<hash-or-name>/` or `instances/<name>/` under `~/.stapler-squad/`); see
`docs/reference/state-isolation.md`.

## escape_events moved to its own database

Escape events live in `<config dir>/db/escape_events.db` (pool `escape-events`, listed in
`/actuator/db` under `databases` and `maintenance`), with their own writer, retention and
incremental vacuum. A background copy (feature flag `escape_events_db_migrate`, default on, starts
one minute after boot) fills it from the legacy `escape_events` table in `analytics.db` in
adaptive batches of about 40 ms writer hold; the ledger is the table `escape_events_migration` in
the new file (`phase`, `cursor`, `scanned`, `inserted`). It resumes after a restart and ends
`done` only when the source row count equals the rows scanned.

The legacy table is **not dropped by default**. Dropping it is destructive, so it is opt-in through
`STAPLER_SQUAD_ESCAPE_LEGACY_DROP` (read at boot; unset, `off` or any unrecognised value does nothing):

| Value | Effect |
|---|---|
| unset / `off` | Nothing runs (default). |
| `dry-run` | Evaluates the guards and logs both row counts and what it would do; changes nothing. |
| `on` | Runs the drop below, once the copy above has finished in the same boot (or an earlier one). |

Guards, all required, evaluated on read-only connections before any delete (neither writer is held):

1. `escape_events_migration.phase` in `escape_events.db` is `done` (a missing ledger, `copying` or `failed` refuses).
2. Every legacy row **id** exists in `escape_events.db`, or the row's `wall_time` is older than the
   age-retention cutoff (`EscapeAnalyticsRetentionDays`, the same cutoff `runEscapeEventRetention`
   uses), so the new database may have evicted it. This checks ids, **not** row content: a copied row
   whose content differs is not detected. A missing row newer than the cutoff refuses with
   `refused_uncopied`, logging the counts and a sample of ids. That is what a legacy write after the
   ledger turned `done` (for example an older binary) looks like. With age retention off, no row may
   be missing. Rows the per-session cap evicted from the new database while still young also refuse;
   that is conservative.

The check repeats per chunk, and each chunk deletes only the ids it just verified, so a row written to
the legacy table mid-run survives; the final `DROP TABLE` runs in a transaction that re-counts the table
and keeps it if any row is left.

When the guards pass, the drop deletes the verified legacy rows in short chunks on the analytics
writer (adaptive to about 40 ms per chunk, with a pause between), then drops the empty table, then
returns freed pages with up to 64 bounded `PRAGMA incremental_vacuum(1024)` steps if `analytics.db` is
`auto_vacuum=INCREMENTAL` (the periodic maintenance loop reclaims any remainder; see above for
converting a legacy file). A crash or shutdown mid-way leaves a smaller table; the next boot checks
again and continues. A table that is empty on the next boot (ent recreates it) is dropped quietly,
without a log line or metric.

The ledger is read once at the start of a run, so do not run a second process or reset the ledger
while a drop is in progress.

To copy rows a `refused_uncopied` run found (catch-up), reset the ledger and restart; the copy
resumes from the start, skips ids already present (`ON CONFLICT DO NOTHING`) and marks `done` again
when the source count equals the rows scanned:

```bash
sqlite3 <escape_events.db> "UPDATE escape_events_migration SET phase='copying', cursor='', scanned=0, inserted=0, pass=0 WHERE id=1"
```

Verify:

```bash
grep -E 'escape legacy' ~/.stapler-squad/logs/staplersquad.log   # guards passed / refused (both counts), dropped
sqlite3 -readonly <analytics.db> "SELECT name FROM sqlite_master WHERE name='escape_events'"   # empty once dropped
```

Metrics: `escape_events.legacy_drop.runs{outcome=...}` (`dropped`, `dry_run`, `migration_incomplete`,
`refused_uncopied`, `error`) and `escape_events.legacy_drop.rows_deleted`.

To reclaim the space without the in-app drop, use the offline path, with the service stopped and
free disk about the size of `analytics.db`:

```bash
sqlite3 <analytics.db> 'PRAGMA auto_vacuum=INCREMENTAL; DROP TABLE escape_events; VACUUM;'
```

Run it only after `SELECT phase FROM escape_events_migration` reads `done` in `escape_events.db`. The
offline `VACUUM` is also the only way to shrink a file that is not at `auto_vacuum=INCREMENTAL`.
`OpenAnalyticsDB` recreates an empty `escape_events` table on every start (`client.Schema.Create`
creates every ent table; a subset create like `escapedb.Open` uses would avoid it, not done here), and
an older binary does too; that is harmless, and the next `on` run drops the empty table again.

## Fallback external job

`scripts/analytics-db-maintenance.sh` runs the same incremental vacuum from outside the app, for
`analytics.db` files found under `$STAPLER_SQUAD_HOME` (default `~/.stapler-squad`) and
`workspaces/*/`, or for paths passed as arguments. It skips databases not at `auto_vacuum=2`.
It does not scan `instances/<name>/`; pass those paths explicitly.

On the author's machine the launchd job `com.stapler-squad.analytics-maintenance`
(`~/Library/LaunchAgents`, daily 04:15) runs it. **Once the in-app loop ships that job is
redundant**; keep the script for databases not managed by a running app.
