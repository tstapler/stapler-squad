# profile-blocking-and-locking reference

Every query below was run against the live service, VictoriaMetrics (`:8428`) and Pyroscope (`:4040`) on 2026-10-31 and returned data unless marked GAP or EMPTY-WHEN-IDLE (counters with no recent events return nothing for a short window: widen to `[1h]` or use `increase(...[1h])`/`last_over_time(...[1h])`). Run PromQL with `python3 -I scripts/pq.py '<expr>' ...` (a plain `curl | python` pipe is blocked by the repo's hook). Metric names are as stored in VictoriaMetrics (suffixes appended).

## Scripts

| Script | Use |
|---|---|
| `scripts/pq.py EXPR...` | Instant PromQL against `localhost:8428`, prints label sets and values, `EMPTY` when no series |
| `scripts/goroutine-states.py [FILE\|URL] [--top N] [--find FUNC] [--module PATH]` | Group a `goroutine?debug=2` dump by state and by top app frame; list lock/WaitGroup/chan-send waits over 1 minute; `--find` lists every stack containing FUNC |
| `scripts/pyro-top.py cpu\|mutex\|mutex-count\|block\|block-count\|goroutine\|alloc [--minutes 60] [--match SUBSTR]` | Top self frames from Pyroscope over a window (Pyroscope `SelectMergeStacktraces`); `--match stapler-squad` charges each leaf (for mutex, `sync.Unlock`) to its nearest app caller, i.e. the lock holder. Block ranking is dominated by idle loops; use pprof `-ignore` for that |

`goroutine-states.py` defaults to the live URL; pass a saved file to compare two dumps.

## PromQL

Runtime (always on, no `--profile` needed):
```
rate(go_runtime_mutex_wait_seconds_total[5m])                       # goroutine-seconds blocked on Mutex/RWMutex per second
avg_over_time(rate(go_runtime_mutex_wait_seconds_total[5m])[1h:1m])  # 1h average
max_over_time(rate(go_runtime_mutex_wait_seconds_total[5m])[1h:1m])
go_runtime_sched_latency_seconds                                     # quantile=0.5|0.99 over last interval (runnable-wait)
max_over_time(go_runtime_sched_latency_seconds{quantile="0.99"}[1h])
go_runtime_goroutines   go_runtime_gomaxprocs
go_runtime_gc_pause_seconds                                          # quantile label
# runtime.unlock / _LostContendedRuntimeLock lead an unfiltered mutex ranking: use --match or -hide
process_open_fds / process_fd_limit
```
`process_start_time_seconds` is GAP (empty). Get the restart time from `ps -o lstart,etime -p $(pgrep -x stapler-squad)`.

tmux exec gate (`session/tmux/exec_gate_observability.go`, `docs/observability/README.md`):
```
histogram_quantile(0.99, sum by (le,pool)(rate(tmux_exec_gate_wait_duration_ms_milliseconds_bucket[15m])))   # wait for a slot
histogram_quantile(0.99, sum by (le,pool)(rate(tmux_exec_gate_exec_duration_ms_milliseconds_bucket[15m])))   # time holding it
tmux_exec_gate_in_flight                       # by pool; only pools with a live series appear (default, resync seen)
sum by (pool)(rate(tmux_exec_gate_acquired_total[15m]))
sum by (pool)(increase(tmux_exec_gate_timeouts_total[1h]))
sum by (subcommand)(rate(tmux_exec_total[15m]))
topk(5, sum by (caller)(rate(tmux_exec_total[15m])))   # logical owner of the spawns
increase(tmux_control_mode_timeouts_total[1h])   histogram_quantile(0.99,sum by (le)(rate(tmux_control_mode_pending_depth_bucket[15m])))
```

Network, mux and request fan-out:
```
histogram_quantile(0.99, sum by (le)(rate(terminal_mux_hol_wait_milliseconds_bucket[15m])))
terminal_mux_sockets_open   terminal_mux_streams_open   rate(terminal_mux_write_errors_total[1h])
histogram_quantile(0.99, sum by (le)(rate(client_ws_send_queue_delay_milliseconds_bucket[15m])))
histogram_quantile(0.99, sum by (le)(rate(client_rpc_network_queue_delay_milliseconds_bucket[1h])))
sum(rate(client_rpc_coalesce_total[15m]))   rate(client_ws_unary_fallback_total[15m])
topk(5, rpc_server_inflight)                 # busiest rpc_method (pq.py prints at most 12 rows)
http_server_connections_open
vcs_status_in_flight
histogram_quantile(0.99, sum by (le)(rate(vcs_status_queue_wait_duration_milliseconds_bucket[15m])))
histogram_quantile(0.5,  sum by (le)(rate(vcs_status_compute_duration_milliseconds_bucket[1h])))
```
`http_server_request_duration_seconds` includes long-lived streams, so its p99 is not a stall signal.

DB (owned by `profile-db-usage`): `rate(db_pool_wait_duration_ms[15m])` by `db_name`, `histogram_quantile(0.99,sum by (le)(rate(db_query_pool_wait_milliseconds_bucket[15m])))`, `curl -s localhost:8543/actuator/metrics | jq '.db'`.

## Endpoints

| URL | Gives |
|---|---|
| `localhost:6060/debug/pprof/{mutex,block,goroutine,heap,allocs,threadcreate}` | pprof; `?debug=2` on goroutine gives the full dump |
| `localhost:6060/debug/pprof/trace?seconds=5` | execution trace; `go tool trace -pprof={sync,sched,syscall,net} t.out` writes a pprof profile (the `Main binary filename not available` warning is harmless) |
| `localhost:6060/debug/pprof/delta_{mutex,block,heap}` | per-interval profile; do not call by hand (steals Alloy's interval) |
| `localhost:6060/debug/fork-pressure`, `/debug/blob-cache` | spawn/zombie counters; blob cache hit rate |
| `localhost:8543/actuator/health` | runtime goroutine status (`degraded` when over the goroutine threshold) |
| `localhost:8543/actuator/ws-unary` | `ws_unary_inflight`, `ws_unary_zombies`, per-method call counts (also Prometheus series `ws_unary_inflight`, `ws_unary_zombies`) |
| `localhost:8543/actuator/terminal-mux` | `poolSize`, `sockets`, `streams` |
| `localhost:8543/actuator/metrics` | blob cache and DB pool stats with top statements and wait time |
| `localhost:4040/querier.v1.QuerierService/{ProfileTypes,LabelValues,SelectMergeStacktraces}` | Pyroscope (Connect JSON POST) |

The `:6060` server only exists when started with `--profile`; `/actuator/*` is on the app port.

## Other pprof recipes

```bash
# Block profile, real synchronization only, delta over a window
curl -s -o ba.pb.gz localhost:6060/debug/pprof/block; sleep 60; curl -s -o bb.pb.gz localhost:6060/debug/pprof/block
go tool pprof -sample_index=delay -top -nodecount=15 -hide='^runtime\.' -ignore='selectgo|chanrecv' -base ba.pb.gz bb.pb.gz
# Rank by event count instead of time
go tool pprof -sample_index=contentions -top -nodecount=10 -focus='github.com/tstapler/stapler-squad' -base a.pb.gz b.pb.gz
# Flamegraph in a browser (cumulative profile, fine for one-off shape)
go tool pprof -http=:8081 -sample_index=delay localhost:6060/debug/pprof/mutex
# Pyroscope, last hour (ranking only for mutex/block, see SKILL.md Step 1b)
python3 -I scripts/pyro-top.py mutex --minutes 60 --match stapler-squad
```

## Metric and tooling gaps (not available today)

- No per-lock or per-call-site lock-wait metric: only the process-wide `go_runtime_mutex_wait_seconds_total`; attribution needs the pprof mutex profile.
- `vcsFlights.saturated()` (`server/services/vcs_status_flight.go:167`) and the slot capacity are not exported; the exec gate exports `in_flight` but no capacity gauge, so saturation is inferred from wait p99.
- Alloy scrapes cumulative mutex/block, so Pyroscope lock values are lifetime totals times scrapes (`docs/observability/README.md`, "Per-interval lock profiles").
- No goroutine-by-state metric; use `goroutine-states.py` on a dump.
- No `process_start_time_seconds`.
- HTTP/1.1 per-origin connection cap is client-side: only `http_server_connections_open` is visible server-side.
- `deadlock` library output sink on the live service is UNVERIFIED.
