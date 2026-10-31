---
name: profile-db-usage
description: Use when diagnosing stapler-squad database load from metrics — DB pool wait is high, pool saturation, "are we saturating SQLite", SQLite contention, SQLITE_BUSY, "database is locked", which query is slow, slow queries, ent-read needs more connections, queries per request, N+1 or polled-handler DB load, ssq-database dashboard, /actuator/db. Not for schema/DDL/PRAGMA design (use sqlite-optimizer).
---

# Profile DB Usage (pool wait, contention, slow queries)

Find which pool is starved, which call site holds it, and fix demand before raising limits. Read-only on the live DB.

Related: `sqlite-optimizer` (PRAGMAs, indexes, EXPLAIN), `go:optimize` / `golang-profiling` (CPU, mutex), `observability-victoriametrics-tempo` (querying the stack), `observability-grafana-dashboards` (editing panels). Metric reference: `docs/reference/database-observability.md`. Pool layout: `docs/adr/ADR-036-database-pool-registry.md`.

## 0. Quick triage

| Owner symptom | Start at |
|---|---|
| "pool waits are high", "saturating" | section 2 first four queries, then 3.1 / 3.2 |
| "database is locked" / SQLITE_BUSY | `db_busy_errors_total` (section 2), then `BEGIN`/`COMMIT` rows in `/actuator/db` `top_statements` (3.4) |
| "which query is slow" | by-caller `topk` queries (section 2), `top_statements` by `total_ms - pool_wait_ms` |
| "need more ent-read connections" | "Does ent-read need more than 4" in section 3 |

Step 0 for every case: rule out the host. `uptime` load average vs `sysctl -n hw.ncpu` (or `nproc`), swap and disk. A load several times the core count slows every statement and looks like pool saturation; also exclude the first minutes after a restart (startup maintenance and backfills burst). Raising limits does not fix a loaded host; reducing demand still helps.

## 1. Find the data

| Source | How |
|---|---|
| VictoriaMetrics | `docker ps --format '{{.Names}} {{.Ports}}' \| grep victoria` (usually `localhost:8428`; if nothing answers the stack is down: `cd ~/dotfiles/stapler-scripts/observability && docker compose up -d`). Confirm names: `curl -s localhost:8428/api/v1/label/__name__/values \| tr ',' '\n' \| grep -iE 'db_'` |
| Grafana | `http://localhost:48300`, dashboard uid `ssq-database` (`docs/observability/grafana/dashboards/database.json`; pool wait, exec p50/p95/p99, top-10 by call site, slow, busy, free pages) |
| `/actuator/db` | `curl -s localhost:8543/actuator/db > /tmp/adb.json` (download first, then parse: a PreToolUse hook blocks piping curl into an interpreter). Keys: `databases[]` (pool, use_case, path, file_bytes, wal_bytes, writer_conns, reader_conns), `pools[]` (name, max_open, in_use, idle, wait_count, wait_duration_ms, queries_total, slow_queries_total, busy_errors_total, errors_total, max_duration_ms, `top_statements[]` with statement, caller chain, count, total_ms, max_ms, pool_wait_ms), `maintenance[]` (page_count, free_pages, auto_vacuum_mode, runs_total) |
| Restart time | macOS `launchctl list \| grep 'com.stapler-squad$'`, Linux `systemctl --user show stapler-squad -p MainPID`, for the PID, then `ps -o lstart= -p <pid>`. `/actuator/db` counters are since that start; VM series survive restarts |

Pools (`db_name`): `ent` (writer, max_open 1), `ent-read` (reader, 4 for the `state` class; knob `STAPLER_SQUAD_DB_READ_CONNS`, `session/ent_client_instrumented.go:19-37`; built-in default `defaultReadPoolSize = 4` there; `0` disables the reader/writer split), `analytics` (escape events + analytics events, writer 1, no reader). `db_maint_*` uses `db_name="sessions"` for the ent DB, not `ent`.

Gaps (no metric exists): WAL size and checkpoint stalls (only `wal_bytes` in `/actuator/db`), write-lock hold time (approximate with the `BEGIN`/`COMMIT` statements in `top_statements` and `db_query_duration` by caller), per-pool `max_open` (only `pools[].max_open` in `/actuator/db`: read it and substitute it in the saturation query, do not assume 1/4), CPU/fsync pressure (use `uptime`, `vm_stat`, `iostat`).

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
# latency p50/p95 (INCLUDES pool wait; p50 is the typical exec cost when few queries wait): high p95 with low p50 = queueing or outliers
histogram_quantile(0.5, sum by (le, db_name)(rate(db_query_duration_milliseconds_bucket[6h])))
histogram_quantile(0.95, sum by (le, db_name)(rate(db_query_duration_milliseconds_bucket[6h])))
# saturation: share of samples with every connection in use (replace 4 with pools[].max_open from /actuator/db; 1 for ent/analytics)
avg_over_time((db_pool_in_use{db_name="ent-read"} >= bool 4)[6h:15s])
# who uses the DB: total execution ms per second by call site, and who waits
topk(8, sum by (db_name, db_caller)(rate(db_query_duration_milliseconds_sum[6h])))
topk(8, sum by (db_name, db_caller)(rate(db_query_pool_wait_milliseconds_sum[6h])))
# busy errors, slow queries, free pages
sum by (db_name)(increase(db_busy_errors_total[6h]))
sum by (db_name)(increase(db_slow_queries_total[6h]))
db_maint_free_pages / db_maint_page_count
```

`db_pool_wait_*` are cumulative gauges; `rate`/`increase` handle restarts. `db_query_duration_*` includes pool wait (`session/dbobs/driver.go` times the whole call), and `db_query_pool_wait_*` records only queries that waited using the pool-wide wait delta, so it over-attributes under concurrency and its sum can exceed the duration sum. Treat per-caller wait as a hint; `db_pool_wait_*` (from `sql.DB.Stats`) is the exact pool total. `db_caller` is capped at 256 values then `other`; `unknown` means the statement ran without a caller frame (a gap in attribution; find it by the statement text in `top_statements`).

Then in `/actuator/db`: per pool, the top statements by `total_ms` and by `pool_wait_ms`. The holder of a single-writer pool is the statement with the largest `total_ms - pool_wait_ms` (execution time) in that pool; one with a large `pool_wait_ms` is a victim.

## 3. Decision tree: signal to fix

1. **Writer pools (`ent`, `analytics`, max_open 1) wait.** Find the holder (largest `total_ms - pool_wait_ms`, then `max_ms`). Fix order:
   1. Batch many writes into one transaction (pattern: `server/analytics` `RecordBatch`, #1307).
   2. Move high-rate writers (analytics events, escape events, context samples) to their own DB via the dbpool registry (ADR-036, `session/dbpool`) so they stop contending with core state.
   3. Async/batched writer with a bounded queue and drop policy for loss-tolerant data (telemetry class).
   4. Retention and maintenance deletes: evict with slack (delete once over cap by ~10%, in big chunks) rather than every flush; run `COUNT(*)` scans off the writer's hot path.
   5. Indexes: every index is written on every insert/delete; drop ones no statement in `top_statements` uses (precedents #1272, #1284) and add only with EXPLAIN evidence (section 4). Details: `sqlite-optimizer`.
   6. Startup and maintenance statements (`dbmaint` samples, backfills) must not run behind the hot writer at boot; stagger or pace them.
2. **`ent-read` waits (max_open 4).** Decide with the rule below; then reduce demand: N+1 and back-to-back selects (the `NextTurnIndex` pattern, `session/contexthistory/store.go:133`), polled handlers re-scanning (#1291), repeated `FindByIDWithOptions` for the same id in one request (`db_queries_per_request` per `rpc_method`). Cache constant lookups, batch ids with `IDIn`, publish snapshots (#1306) or a materialized view (ADR-034).
3. **`ent` high exec p95 but low wait.** The SQL or the host is slow: EXPLAIN QUERY PLAN on a copy, fsync/disk pressure, `wal_autocheckpoint`, `synchronous` (`db_synchronous` should be 1 = NORMAL).
4. **`db_busy_errors_total` above 0.** Writer outside the pool, long transaction, or busy_timeout too low; find the transaction owner via `top_statements` (`BEGIN`/`COMMIT` rows with large `total_ms`).
5. **Free pages / size.** `db_maint_free_pages / db_maint_page_count` above 0.2 and not shrinking: `docs/how-to/reclaim-sqlite-free-space.md`. Never `VACUUM` the live file.

### Does ent-read need more than 4 connections?

Raise `STAPLER_SQUAD_DB_READ_CONNS` only if checks 1-4 all pass over a window that excludes restarts (thresholds are starting heuristics, not measured constants; record the numbers you used):

1. Saturation: `avg_over_time((db_pool_in_use{db_name="ent-read"} >= bool <max_open>)[6h:15s])` is above 0.2.
2. Waiting share: `sum(rate(db_query_pool_wait_milliseconds_count{db_name="ent-read"}[6h])) / sum(rate(db_query_duration_milliseconds_count{db_name="ent-read"}[6h]))` is above 0.05.
3. Waits are not exec time: mean wait per wait event, `increase(db_pool_wait_duration_ms) / increase(db_pool_wait_count)` for `ent-read`, exceeds the pool's latency p50 (`db_query_duration_milliseconds_bucket`, section 2). Short, frequent waits point at pool size; a few long waits behind slow statements point at demand or the host.
4. Spread: the top caller in `topk(8, sum by (db_caller)(rate(db_query_pool_wait_milliseconds_sum{db_name="ent-read"}[6h])))` holds under 50% of total wait. A single dominant caller is a demand problem.
5. Headroom (outside VictoriaMetrics): `uptime` load average below `hw.ncpu`. WAL readers do not block the writer, but each connection adds page cache and modernc per-connection overhead and shares the same cores.

If any of 1-4 fails, do not raise the limit: reduce demand (3.2). Suggested size when all pass: 8 (about half the cores on an 18-core host; larger sizes only moved wait around in the test below). Confirm with an A/B: `sqlite3 -readonly <live.db> ".backup /tmp/x/copy.db"` (live source opened read-only; open the copy normally), replay the read mix against the copy at 4, 8, 16 connections (about 100 lines of Go: N `query_only` reader connections, one writer connection updating a row every 20 ms, a weighted SELECT mix taken from `top_statements`, 8 and 32 client goroutines, 6 s runs, 3 interleaved repetitions), compare wait count and p95/p99. Example (2026-10-31, load average ~70 on 18 cores, a pessimistic host): with 8 clients, p95 fell from 3.3-6.4 ms to about 2.1 ms going 4 to 8 connections, p99 rose from 10-18 ms to 23-28 ms, and with 32 clients throughput did not improve at 8 or 16 connections.

## 4. Attribute and confirm

- Attribution: `db_caller` is the first frame of the caller chain; the full chain is in `/actuator/db` `top_statements[].caller`. Per-pool `wait` over-attributes under concurrency (pool-wide delta), so rank callers by execution time (`total_ms - pool_wait_ms`) first.
- Index work: `sqlite3 <copy.db> "EXPLAIN QUERY PLAN <statement with literals>"` on a `.backup` copy. Look for `SCAN`, `USE TEMP B-TREE`; confirm `SEARCH ... USING INDEX`.
- Before/after: record the section 2 queries for a matched window before the deploy (same hours of day, same weekday type, excluding the 10 minutes after restart) and after; compare wait events, mean wait, p95, queries/s by caller, and `db_queries_per_request` for the touched `rpc_method`. Say which window each number covers. `WatchBacklogItems` is a stream: its queries-per-request is per stream, not per message.

## 5. Safety

- Read-only against the live DB: only `/actuator/db`, VictoriaMetrics, and `.backup` copies. Copies of WAL-mode files need to be opened without `-readonly`.
- Never `VACUUM`, `PRAGMA wal_checkpoint(TRUNCATE)`, writes, or schema changes on the live file.
- Do not deploy to test a hypothesis with `make install-service`; use an isolated instance (CLAUDE.md "Manual/interactive testing").
- Report VERIFIED (query ran, output shown) vs INFERRED; a metric that does not exist is a gap, not zero.
