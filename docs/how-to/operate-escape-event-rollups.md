# Operate the escape-event rollups

Design and rationale: `docs/explanation/escape-events-rollups.md`.

## What runs

- The writer flushes every 5 s or 500 queued events, one transaction each (raw rows + hourly rollups).
- `Retention` runs at startup and every 10 minutes: finish the backfill, merge hourly rollups older
  than 14 days into days, delete raw rows older than `escapeAnalyticsRetentionDays` (default 2) and
  trim sessions above `escapeAnalyticsMaxRowsPerSession` (default 10,000). Age eviction deletes at most
  200 chunks of 250 rows per cycle, so a large backlog drains over several cycles.
- `dbmaint` (see `docs/how-to/reclaim-sqlite-free-space.md`) returns the freed pages with `incremental_vacuum`.

## Check it

```bash
DB=~/.stapler-squad/workspaces/<hash>/db/escape_events.db   # or <config dir>/db/escape_events.db
sqlite3 "file:$DB?mode=ro" "SELECT backfill_done, backfilled_rows, datetime(live_since_ms/1000,'unixepoch') FROM escape_event_rollup_state;"
sqlite3 "file:$DB?mode=ro" "SELECT bucket_secs, COUNT(*), SUM(event_count) FROM escape_event_rollups GROUP BY 1;"
```

`backfill_done = 0` means rollups are incomplete: raw rows are not deleted and summaries read raw rows.

Writer metrics (OTel gauges): `escape_events.queue_depth`, `dropped_total`, `rows_written_total`,
`failed_rows_total`, `flushes_total`, `last_flush_seconds`, `max_flush_seconds`. A rising
`dropped_total` means the queue (10,000) filled; the log then shows `events dropped under backpressure`.

## Back up before an upgrade

`.backup` is safe against a live database:

```bash
sqlite3 "file:$DB?mode=ro" ".backup /path/to/escape_events.backup.db"
```

## Roll back

1. Stop the service and install the previous binary. It ignores the two rollup tables and keeps
   writing raw rows, so nothing needs to be dropped for it to run.
2. The rollup tables go stale while the old binary runs. To return to this version later, reset them
   so the next start rebuilds from the raw rows that still exist (history older than the raw window is lost):

   ```bash
   sqlite3 "$DB" "DELETE FROM escape_event_rollups; DROP TABLE escape_event_rollup_state;"
   ```
3. To discard rollups for good: `DROP TABLE escape_event_rollups; DROP TABLE escape_event_rollup_state;`.
4. Raw rows deleted by the age window are not recoverable from rollups; restore the `.backup` copy if you need them.
