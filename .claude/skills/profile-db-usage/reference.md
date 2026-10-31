# profile-db-usage: PromQL reference

Every query here was run against the live VictoriaMetrics on 2026-10-31 and returned series. `[W]` is the window; use `5m` for live panels, `6h` for a shift, or a window starting after the last restart.

```promql
# queries per second per pool, and per call site
sum by (db_name)(rate(db_query_duration_milliseconds_count[6h]))
topk(5, sum by (db_caller)(rate(db_query_duration_milliseconds_count{db_name="ent-read"}[6h])))

# execution seconds per second per pool (utilisation of a single-writer pool when db_name is ent or analytics)
sum by (db_name)(rate(db_query_duration_milliseconds_sum[6h])) / 1000

# p99 wait and p99 exec per pool
histogram_quantile(0.99, sum by (le, db_name)(rate(db_query_pool_wait_milliseconds_bucket[6h])))
histogram_quantile(0.99, sum by (le, db_name)(rate(db_query_duration_milliseconds_bucket[6h])))

# share of queries that waited at all
sum by (db_name)(rate(db_query_pool_wait_milliseconds_count[6h])) / sum by (db_name)(rate(db_query_duration_milliseconds_count[6h]))

# slowest callers by p95 in one pool
topk(5, histogram_quantile(0.95, sum by (le, db_caller)(rate(db_query_duration_milliseconds_bucket{db_name="analytics"}[6h]))))

# in-use connections: mean, and share of time at the pool ceiling for the single-writer pools
avg_over_time(db_pool_in_use[6h])
avg_over_time((db_pool_in_use{db_name=~"ent|analytics"} >= bool 1)[6h:15s])

# queries per request, by RPC (p95, and mean)
histogram_quantile(0.95, sum by (le, rpc_method)(rate(db_queries_per_request_bucket[6h])))
topk(5, sum by (rpc_method)(rate(db_queries_per_request_sum[6h])) / sum by (rpc_method)(rate(db_queries_per_request_count[6h])))

# pool wait over time (look for restarts: startup bursts are not steady state)
sum by (db_name)(rate(db_pool_wait_duration_ms[5m]))

# configuration drift: 1 = WAL / synchronous NORMAL per pool
db_journal_mode
db_synchronous

# maintenance
db_maint_free_pages / db_maint_page_count
db_maint_last_error
```

## Metric inventory (live names, labels)

| Metric | Labels | Source |
|---|---|---|
| `db_query_duration_milliseconds_{bucket,sum,count}` | `db_name`, `db_caller` | `session/dbobs/stats.go` |
| `db_query_pool_wait_milliseconds_{bucket,sum,count}` | `db_name`, `db_caller` | `session/dbobs/stats.go` |
| `db_pool_in_use`, `db_pool_open`, `db_pool_wait_count`, `db_pool_wait_duration_ms` | `db_name` | `session/dbobs/otel_gauges.go` |
| `db_slow_queries_total`, `db_busy_errors_total` | `db_name` | `session/dbobs/stats.go` |
| `db_queries_per_request_{bucket,sum,count}` | `rpc_method` | `server/interceptors/db_query_count.go` |
| `db_maint_*` | `db_name` (`sessions`, `analytics`) | `session/dbmaint` |
| `db_journal_mode`, `db_synchronous` | `db_name`, `db_class`, `db_logical` | `session/dbpool` |

Not exported (gaps): WAL size/checkpoint, write-lock hold time, `max_open`, host CPU and fsync latency.
