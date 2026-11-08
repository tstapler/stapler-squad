# Escape events: batched writes, short raw window, rollup history

Status: accepted. Applies to `db/escape_events.db` (package `server/analytics/escapedb`).

## Problem

Measured on a copy of the live database (`sqlite3 .backup`, 2026-11-08):

| Fact | Value |
|---|---|
| File size | 406 MB (104,003 pages, 6,065 free) |
| Rows | 728,739 over 117 sessions, 2026-11-01 to 2026-11-08 |
| Distinct (session, project, type, stage, mangled, mangle_type) groups | 1,168 |
| Distinct groups per UTC hour | 2,567 |
| Global summary (`GROUP BY` over raw rows) | 4.2 s |

Rows arrived at about 20k per hour, but the writer flushed every 500 ms or 100 rows, so each
flush carried a handful of events. Every flush was a separate `INSERT`, then (for sessions at their
10,000-row cap) a `COUNT` and a `DELETE`. The live log shows `INSERT`, `SELECT COUNT` and `DELETE`
from `flushBatch` as slow statements with p95 pool waits of 1.1 to 4.1 s and a worst case of 73 s
in the sampled window. The database has a single writer connection, so every slow statement
queues the next one, and the `dbmaint` page-stats query queues behind them.

Raw rows were doing two jobs: a per-session debugging timeline, and the only source for the
Escape Analytics summaries. The second job forced a 10,000-row cap (history was lost by eviction)
and a full scan per summary.

## Decision

1. **One transaction per flush, flushed on a 5 s timer or 500 rows.** The flush is a multi-row
   `INSERT` of the raw events plus an additive upsert of their hourly rollups, in the same
   transaction. The in-memory queue holds 10,000 events; beyond that events are dropped and
   counted (`DroppedCount`, `Stats()`, OTel gauges `escape_events.*`). `WriteEscapeEvent` never blocks.
2. **Counts are written when events arrive.** `escape_event_rollups` is keyed by
   `(bucket_secs, bucket_start, session_id, project_path, sequence_type, stage, mangled, mangle_type)`
   with `event_count`, `byte_total`, `first_seen_ms`, `last_seen_ms`. Buckets are UTC hours, then UTC days.
   Because the writer counts, deleting a raw row never loses its count.
3. **Raw rows become a short window.** A separate job (`Retention`, every 10 minutes, off the
   write path) deletes raw rows older than `escapeAnalyticsRetentionDays` (default 2, was 7) in
   250-row transactions, and trims sessions above `escapeAnalyticsMaxRowsPerSession`. The per-flush
   `COUNT`/`DELETE` is gone, so a session can exceed its cap by up to ten minutes of its own events.
4. **Hourly rollups older than 14 days merge into daily rollups** (one transaction per day,
   insert then delete); daily rollups older than 730 days are deleted.
5. **Readers use `escapedb.Reader`.** The global and per-session summaries read whole rollup
   buckets inside the requested range and raw rows for the partial hours at its ends. The
   per-session timeline (`QueryEscapeAnalytics`) stays on raw rows, since it shows individual events.

## Backfill and the live_since cut

Rollups only count events written after the feature starts, so existing rows need a one-time
backfill. `escape_event_rollup_state` holds `live_since_ms` (stamped by the first writer or
retention start) and a backfill cursor.

- Backfill reads raw rows in `id` order, 5,000 per chunk, and adds only rows with
  `wall_time < live_since`; later rows were already counted by the writer.
- Each chunk's upsert and the cursor advance are one transaction, so a crash resumes without
  double counting.
- **Until the backfill is marked done, no raw row is deleted and readers fall back to raw rows**, so a
  half-built rollup table never reads as lost history.

Real-copy result: the backfill of 728,739 rows finished inside the first 10 s retention cycle
(chunk reads and writes only, no long transaction).

## Range edges

Rollup buckets are whole UTC hours or days, while requests can start at any instant.

- In the hourly tier, partial hours at each end come from raw rows, so results match a raw count
  as long as those raw rows still exist (property-tested against a brute-force count).
- Where a range reaches into the daily tier, whole UTC days that overlap it are included: the
  edge snaps outward to day boundaries.
- `wall_time` is stored as local-zone text (`Time.String()`) and compared as text. Raw bounds are
  converted to the local zone before binding. Before this change the summaries bound UTC times
  against local text, which shifts ranges by the zone offset.

## Consequences

- Real-copy numbers: rollups hold 2,567 rows for all 728,739 events; the global summary takes 7.8 ms
  (was 4.2 s) with 0 of 1,168 group counts differing from the raw count; raw rows drop to
  271,450 and the file from 406 MB to 146 MB after `incremental_vacuum`. The flush of 500 rows on the 406 MB database averaged 45 ms (20 runs).
- A raw row that is dropped before a flush (queue full) or lost to a failed flush is missing from raw
  and rollups alike; `Stats().FailedRows` and `Dropped` count both.
- Rows added to `escape_events` by the legacy `analytics.db` migration after a backfill has
  finished are not rolled up. The migration is long finished on existing installs.
- The owner's config stores `escapeAnalyticsRetentionDays: 7` explicitly, so the new default of 2 only
  applies to fresh configs; lower it to shrink the raw window.

## Alternatives considered

- **Keep per-flush cap enforcement with bigger batches.** Cheaper than today, but still bounds
  history by the cap, so it cannot answer "what happened last week".
- **Compact raw rows into rollups on a schedule instead of counting at write time.** Needs a
  watermark and loses counts for rows the cap evicts before the compaction runs.
- **Store `wall_time` as UTC.** Fixes the text comparison but mixes two formats in one column
  during the transition; left for a separate change.

## Rollback

See `docs/how-to/operate-escape-event-rollups.md`.
