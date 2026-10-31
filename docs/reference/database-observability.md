# Database and saturation metrics

Dashboards: `ssq-database` (`docs/observability/grafana/dashboards/database.json`) and the "Saturation" row of `ssq-http-rpc-connections`. Names are as written in code; VictoriaMetrics rewrites dots to underscores and adds `_bucket`/`_sum`/`_count`/`_total` (the new instruments carry no OTel unit, so the names below are what to query; confirm after first deploy with `curl 'localhost:8428/api/v1/label/__name__/values'`).

## Diagnosing "everything is slow"

1. `go_sched_latency_p99_seconds` > 10 ms: the host is CPU-starved (load average far above core count). Every handler inflates by this; fix the host first.
2. `db_pool_wait_duration_ms` rate (per pool) > 1000 ms/s while `db_query_duration` p95 is small: callers queue for a connection. Open the "By query call site" row: a `db_caller` with hundreds of calls per second is a sweep or N+1 loop. `GET /actuator/db` shows the same data with full statement text (`top_statements`).
3. `rpc_server_inflight{kind="unary"}` rising with a flat request rate: handlers are stuck behind a saturated resource (DB pool, mutex: `go_sync_mutex_wait_seconds_total`).

## Metrics added

| Metric | Type | Labels | Saturation looks like |
|---|---|---|---|
| `db_query_duration_milliseconds`, `db_query_pool_wait_milliseconds` | histogram | `db_name`, `db_caller` (repository method; capped at 256 values, then `other`) | caller p95 or rate dominating the Top-10 panels |
| `http_listener_accepts_total` | counter | `listener` (`main`, `remote`) | rate collapse with clients still connecting |
| `http_listener_accept_errors_total` | counter | `listener`, `kind` (`emfile`, `timeout`, `other`) | any `emfile`: out of file descriptors |
| `http_listener_conns_open` | gauge | `listener` | monotonic growth = connection leak; includes hijacked WebSockets, unlike `http_server_connections_open` |
| `http_listener_conn_age_seconds` | histogram | `listener` | short-lived p99 where WebSockets are expected = churn |
| `http_listener_tls_handshake_errors_total` | counter | `listener` | sustained rate on `remote` = bad certs or plain HTTP to :8444 |
| `net_tcp_listen_overflows_total`, `net_tcp_listen_drops_total` | counter | none | any sustained non-zero rate (Linux only) |
| `net_tcp_somaxconn` | gauge | none | ceiling on the accept backlog (macOS default 128) |
| `process_open_fds`, `process_fd_limit` | gauge | none | open/limit > 0.8 |
| `go_sched_goroutines`, `go_heap_objects_bytes` | gauge | none | unbounded growth |
| `go_sched_latency_p50_seconds`, `go_sched_latency_p99_seconds`, `go_gc_pause_p99_seconds` | gauge | none | p99 > 10 ms (orange) / 100 ms (red); computed over the last scrape interval |
| `go_sync_mutex_wait_seconds_total` | counter | none | rate > 1 s/s |
| `rpc_server_inflight` | up-down counter | `kind` (`unary`, `stream`) | see step 3 above |
| `rpc_server_client_aborted_total` | counter | `method` | rate relative to the method's request rate; a method with a high abort ratio is slower than the client's timeout |

## Not covered

- macOS kernel listen overflow: no readable source without parsing `net.inet.tcp.stats` or shelling out (raw `os/exec` is forbidden by the `norawexec` lint). Check manually with `netstat -s -p tcp | grep -i listen`; only `net_tcp_somaxconn` is exported there.
- Per-route queue time and an accept-loop gap metric are not implemented; `rpc_server_inflight` and the accept/open-connection counters are the proxies.
- The TLS-mode main listener (`ListenAndServeTLS`) is not wrapped; the default plain-HTTP main listener and the remote listener are.
- `http.Server.ReadTimeout` (15 s) is verified not to apply to hijacked WebSockets: `TestWebSocketUpgrade_should_ClearHTTPServerReadDeadline` shows gorilla's `Upgrade` clears the read deadline.
