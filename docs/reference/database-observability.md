# Database metrics

Dashboard: `ssq-database` (`docs/observability/grafana/dashboards/database.json`). Names are as written in code; VictoriaMetrics rewrites dots to underscores and adds `_bucket`/`_sum`/`_count` and the unit (`db_query_duration_milliseconds_bucket`).

## Reading it

1. Compare **Pool wait** with **Query duration**. Wait much larger than execution time means callers queue for a connection (the `ent` writer has `max_open=1`, `ent-read` has 4), not that SQL is slow.
2. Open **By query call site**. `db_caller` is the repository method that issued the query (the first frame of the caller chain `/actuator/db` shows; capped at 256 distinct values, then `other`). A caller with hundreds of calls per second and sub-millisecond queries is a sweep or N+1 loop; this is how the 2026-10-30 terminal-item sweeps (88% of read queries) were found.
3. `GET /actuator/db` has the same data with normalized statement text (`top_statements`).

## Metrics

| Metric | Type | Labels | Saturation looks like |
|---|---|---|---|
| `db_query_duration_milliseconds` | histogram | `db_name`, `db_caller` | a caller dominating Top-10 total time or p95 |
| `db_query_pool_wait_milliseconds` | histogram | `db_name`, `db_caller` | p95 wait above exec p95; over-attributed under concurrency (pool-wide delta) |
| `db_pool_in_use`, `db_pool_open` | gauge | `db_name` | in-use equals max_open for long stretches |
| `db_pool_wait_duration_ms` | cumulative gauge | `db_name` | rate above 1000 ms/s: more than one caller waiting on average |
| `db_pool_wait_count` | cumulative gauge | `db_name` | rising wait event rate |
| `db_slow_queries_total` | counter | `db_name` | any sustained rate (threshold `STAPLER_SQUAD_SLOW_QUERY_MS`, default 100) |
| `db_busy_errors_total` | counter | `db_name` | any non-zero rate: SQLITE_BUSY despite busy_timeout |
| `db_maint_free_pages`, `db_maint_page_count` | gauge | `db_name` | free/total above 0.2 and not shrinking |

`db_caller` series appear only after this ships; the pool and total-duration panels work on existing data.
