---
name: profile-db-usage
description: Use when profiling stapler-squad database usage from Grafana/VictoriaMetrics — DB pool wait, high pool wait duration, SQLite contention, SQLITE_BUSY, slow queries, "database is locked", ent-read needs more connections, queries per request, N+1 or polled-handler DB load, ssq-database dashboard, /actuator/db, WAL size or free pages. Gives the metric names, verified PromQL, a signal-to-fix decision tree, and a before/after protocol.
---

# Profile DB Usage (pool wait, contention, slow queries)

Find which pool is starved, which call site holds it, and fix demand before raising limits. Read-only on the live DB.

Related: `sqlite-optimizer` (PRAGMAs, indexes, EXPLAIN), `go:optimize` / `golang-profiling` (CPU, mutex), `observability-victoriametrics-tempo` (querying the stack), `observability-grafana-dashboards` (editing panels). Metric reference: `docs/reference/database-observability.md`. Pool layout: `docs/adr/ADR-036-database-pool-registry.md`.

## 1. Find the data

| Source | How |
|---|---|
| VictoriaMetrics | `docker ps --format '{{.Names}} {{.Ports}}' \| grep victoria` (usually `localhost:8428`). Confirm names: `curl -s localhost:8428/api/v1/label/__name__/values \| tr ',' '\n' \| grep -iE 'db_'` |
| Grafana | `http://localhost:48300`, dashboard uid `ssq-database` (`docs/observability/grafana/dashboards/database.json`; pool wait, exec p50/p95/p99, top-10 by call site, slow, busy, free pages) |
| `/actuator/db` | `curl -s localhost:8543/actuator/db > /tmp/adb.json` (never pipe curl into an interpreter; the hook blocks it). Keys: `databases[]` (pool, use_case, path, file_bytes, wal_bytes, writer_conns, reader_conns), `pools[]` (name, max_open, in_use, idle, wait_count, wait_duration_ms, queries_total, slow_queries_total, busy_errors_total, errors_total, max_duration_ms, `top_statements[]` with statement, caller chain, count, total_ms, max_ms, pool_wait_ms), `maintenance[]` (page_count, free_pages, auto_vacuum_mode, runs_total) |
| Restart time | `launchctl list \| grep stapler-squad` for the PID, then `ps -o lstart= -p <pid>`. `/actuator/db` counters are since that start; VM series survive restarts |

Pools (`db_name`): `ent` (writer, max_open 1), `ent-read` (reader, 4; knob `STAPLER_SQUAD_DB_READ_CONNS`, `session/ent_client_instrumented.go:19-37`, default via `session/dbpool/profile.go` `Readers: 4`), `analytics` (escape events + analytics events, writer 1, no reader). `db_maint_*` uses `db_name="sessions"` for the ent DB, not `ent`.

Gaps (no metric exists): WAL size and checkpoint stalls (only `wal_bytes` in `/actuator/db`), write-lock hold time (approximate with the `BEGIN`/`COMMIT` statements in `top_statements` and `db_query_duration` by caller), per-pool `max_open` (only in `/actuator/db`; use the constant 1/4 in PromQL), CPU/fsync pressure (use `uptime`, `vm_stat`, `iostat`).

## 2. Run these first (ready-to-paste; more in `reference.md`)

Helper: `curl -s -G localhost:8428/api/v1/query --data-urlencode 'query=<PROMQL>'`. Use `curl -G --data-urlencode`, never hand-escaped URLs. Swap `[6h]` for a window that starts after the last restart when judging a deploy.

```promql
# wait time per second per pool (>1000 = on average more than one caller blocked)
sum by (db_name)(rate(db_pool_wait_duration_ms[6h]))
# wait events per second, and mean wait per event
sum by (db_name)(rate(db_pool_wait_count[6h]))
sum by (db_name)(increase(db_pool_wait_duration_ms[6h])) / sum by (db_name)(increase(db_pool_wait_count[6h]))
# p95 / p99 wait of the queries that waited (a flat 10000 is the top histogram bucket: ">= 10 s")
histogram_quantile(0.95, sum by (le, db_name)(rate(db_query_pool_wait_milliseconds_bucket[6h])))
# exec p95 without wait: if wait p95 >> exec p95 callers queue; if exec is high, SQL or host is slow
histogram_quantile(0.95, sum by (le, db_name)(rate(db_query_duration_milliseconds_bucket[6h])))
# saturation: share of samples with every connection in use (use 1 for ent/analytics, 4 for ent-read)
avg_over_time((db_pool_in_use{db_name="ent-read"} >= bool 4)[6h:15s])
# who uses the DB: total execution ms per second by call site, and who waits
topk(8, sum by (db_name, db_caller)(rate(db_query_duration_milliseconds_sum[6h])))
topk(8, sum by (db_name, db_caller)(rate(db_query_pool_wait_milliseconds_sum[6h])))
# busy errors, slow queries, free pages
sum by (db_name)(increase(db_busy_errors_total[6h]))
sum by (db_name)(increase(db_slow_queries_total[6h]))
db_maint_free_pages / db_maint_page_count
```

`db_pool_wait_*` are cumulative gauges; `rate`/`increase` handle restarts. `db_caller` is capped at 256 values then `other`; `unknown` means the statement ran without a caller frame (a gap in attribution; find it by the statement text in `top_statements`).

Then in `/actuator/db`: per pool, the top statements by `total_ms` and by `pool_wait_ms`. The holder of a single-writer pool is the statement with the largest `total_ms` in that pool, not the one with the largest `pool_wait_ms` (that is a victim).

## 3. Decision tree: signal to fix

First check the confounder: `uptime` load average vs `sysctl -n hw.ncpu`, swap, disk. A load several times the core count makes every statement slow and mimics pool saturation; fixing demand still helps, raising limits does not.

1. **Writer pools (`ent`, `analytics`, max_open 1) wait.** Find the holder (largest `total_ms`, `max_ms`). Fix order:
   1. Batch many writes into one transaction (pattern: `server/analytics` `RecordBatch`, #1307).
   2. Move high-rate writers (analytics events, escape events, context samples) to their own DB via the dbpool registry (ADR-036, `session/dbpool`) so they stop contending with core state.
   3. Async/batched writer with a bounded queue and drop policy for loss-tolerant data (telemetry class).
   4. Retention and maintenance deletes: evict with slack (delete once over cap by ~10%, in big chunks) rather than every flush; run `COUNT(*)` scans off the writer's hot path.
   5. Startup/maintenance statements (`dbmaint` `PRAGMA page_count`) must not queue behind the hot writer or run in the first minute.
   6. Indexes: every index is written on every insert/delete; drop unused ones (#1272, #1284) and add only with EXPLAIN evidence (below).
2. **`ent-read` waits (max_open 4).** Decide with the rule below; then reduce demand: N+1 and back-to-back selects (the `NextTurnIndex` pattern, `session/contexthistory/store.go:133`), polled handlers re-scanning (#1291), repeated `FindByIDWithOptions` for the same id in one request (`db_queries_per_request` per `rpc_method`). Cache constant lookups, batch ids with `IDIn`, publish snapshots (#1306) or a materialized view (ADR-034).
3. **`ent` high exec p95 but low wait.** The SQL or the host is slow: EXPLAIN QUERY PLAN on a copy, fsync/disk pressure, `wal_autocheckpoint`, `synchronous` (`db_synchronous` should be 1 = NORMAL).
4. **`db_busy_errors_total` above 0.** Writer outside the pool, long transaction, or busy_timeout too low; find the transaction owner via `top_statements` (`BEGIN`/`COMMIT` rows with large `total_ms`).
5. **Free pages / size.** `db_maint_free_pages / db_maint_page_count` above 0.2 and not shrinking: `docs/how-to/reclaim-sqlite-free-space.md`. Never `VACUUM` the live file.

### Does ent-read need more than 4 connections?

Raise `STAPLER_SQUAD_DB_READ_CONNS` only if all hold over a window that excludes restarts and a loaded host:
- saturation: `avg_over_time((db_pool_in_use{db_name="ent-read"} >= bool 4)[6h:15s])` above 0.2;
- waiting share: wait events / queries above 0.05 and mean wait above exec p95;
- queueing is spread over many callers (the by-caller wait query), not one slow statement;
- host has CPU headroom (load average below core count). WAL readers do not block the writer, but each connection adds page cache and modernc per-connection overhead, and readers share the same cores.

If one statement dominates exec time or waits, reduce that demand instead. A/B check: copy the DB with `sqlite3 -readonly <live.db> ".backup /tmp/x/copy.db"`, replay the read mix at 4, 8, 16 connections on a throwaway binary (own ports from CLAUDE.md "Manual dev port block", `STAPLER_SQUAD_INSTANCE`, `STAPLER_SQUAD_TMUX_SOCKET`), compare wait count and p95/p99, three interleaved repetitions. On the 2026-10-31 run more connections lowered p95 only when concurrency exceeded 4 and raised p99 and did not raise throughput under load average ~70 on 18 cores.

## 4. Attribute and confirm

- Attribution: `db_caller` is the first frame of the caller chain; the full chain is in `/actuator/db` `top_statements[].caller`. Per-pool `wait` over-attributes under concurrency (pool-wide delta), so rank callers by `total_ms` first.
- Index work: `sqlite3 <copy.db> "EXPLAIN QUERY PLAN <statement with literals>"` on a `.backup` copy. Look for `SCAN`, `USE TEMP B-TREE`; confirm `SEARCH ... USING INDEX`.
- Before/after: record the section 2 queries for a matched window before the deploy (same hours of day, same weekday type, excluding the 10 minutes after restart) and after; compare wait events, mean wait, p95, queries/s by caller, and `db_queries_per_request` for the touched `rpc_method`. Say which window each number covers. `WatchBacklogItems` is a stream: its queries-per-request is per stream, not per message.

## 5. Safety

- Read-only against the live DB: only `/actuator/db`, VictoriaMetrics, and `.backup` copies. Copies of WAL-mode files need to be opened without `-readonly`.
- Never `VACUUM`, `PRAGMA wal_checkpoint(TRUNCATE)`, writes, or schema changes on the live file.
- Do not deploy to test a hypothesis with `make install-service`; use an isolated instance (CLAUDE.md "Manual/interactive testing").
- Report VERIFIED (query ran, output shown) vs INFERRED; a metric that does not exist is a gap, not zero.
