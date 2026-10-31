# Grafana Dashboards

`grafana/dashboards/*.json` are this project's Grafana dashboards, migrated
here from `~/dotfiles/stapler-scripts/observability/grafana/dashboards/stapler-squad/`
(2026-09-12) so they version alongside the metrics they visualize instead of
living in an unrelated repo.

| Dashboard | uid | Covers |
|---|---|---|
| `cgroup-memory.json` | `ssq-cgroup-memory` | USE-method cgroup memory (`cgroup_memory_*`, see `docs/how-to/enable-opentelemetry.md`) |
| `go-runtime-request-path.json` | `ssq-go-runtime` | Go scheduler/GC/memory (`go.runtime.*`), in-flight unary RPCs, DB statements per request (`db.queries_per_request`), HTTP-vs-handler p95 |
| `http-rpc-connections.json` | `ssq-http-rpc-connections` | RED-method HTTP/RPC connection saturation (`http.server.connections_*`, `rpc.server_streams.open`) |
| `pprof-profiles.json` | `ssq-pprof-profiles` | Pyroscope flamegraphs (CPU/heap/goroutines), scraped from the pprof server |
| `tmux-control-mode.json` | `ssq-tmux-control-mode` | RED-method tmux control-mode command latency/throughput |
| `tymux-backend.json` | `ssq-tymux-backend` | tymux vs tmux operation latency (RED), attach stream health, backend mix, tymuxd supervision |
| `client-network.json` | `ssq-client-network` | Browser RPC rate/latency/errors by method and transport, duplicates and coalescing, batch effectiveness, WebSocket lifecycle/RTT (client and `ws_unary_*` server side), web vitals, long tasks; see `docs/reference/client-network-telemetry.md` |
| `database.json` | `ssq-database` | SQLite pool saturation vs execution time per pool and per query call site (`db_*`); see `docs/reference/database-observability.md` |

## tymux backend metrics

OTel names below are as written in code; VictoriaMetrics appends the unit and
`_total`/`_bucket` suffixes (`tymux_daemon_start_duration_ms` is queried as
`tymux_daemon_start_duration_ms_milliseconds_bucket`). No metric carries a
session or pane id as a label.

| Metric | Type | Labels | Source |
|---|---|---|---|
| `session_backend_operation_duration_ms` | histogram | `backend`, `operation` | `session/backend_observability.go`; lifecycle ops (`start`, `restore`, `attach`) plus per-call `send_keys`, `write_stdin`, `capture_pane`, `resize` on both backends. The hot-path four are metric-only (no span of their own); the unary `capture_pane` RPC still gets an otelconnect client span, subject to the trace sampler. |
| `session_backend_info` | gauge (always 1) | `backend`, `source` (`global`/`session_override`) | `session/backend_info_metric.go`; refreshed on backend resolution and the rollout status RPC |
| `tymux_attach_stream_reconnects_total` | counter | `cause` (`error`, `output_gap`) | `session/tymux/stream.go` |
| `tymux_attach_events_total` | counter | `kind` (`output_gap`, `gap_exceeded`, `snapshot_redraw`) | `session/tymux/stream.go`, `resync.go` |
| `tymux_attach_heartbeat_age_seconds` | gauge | none | max seconds since any event on a live stream; 0 when none are live |
| `tymux_daemon_restarts_total` | counter | `reason` (`cold_start`, `replace`, `detected_restart`) | `session/tymux/supervise.go`, `version.go`, `stream.go`; `detected_restart` counts once per affected stream |
| `tymux_daemon_start_duration_ms` | histogram | `outcome` (`ok`, `failed`) | spawn until healthy |
| `tymux_daemon_version_mismatch` | gauge | none | 1 when the running tymuxd differs from the installed binary; updated by `DetectDaemonVersions` |

stapler-squad's calls to tymuxd carry a W3C `traceparent` header (an
`otelconnect` client interceptor in `session/tymux/transport.go`), so a trace
that starts in stapler-squad continues into tymuxd once the daemon extracts
it. tymuxd does not export spans yet.

## Viewing them

These dashboards don't run their own Grafana — they're provisioned into the
single machine-wide observability stack defined in
`~/dotfiles/stapler-scripts/observability/` (Grafana + VictoriaMetrics +
Tempo + Pyroscope + an OTel Collector, shared across every local project).
That stack's `docker-compose.yml` bind-mounts this directory directly
(`docs/observability/grafana/dashboards` → `/var/lib/grafana/dashboards/stapler-squad`
in the `grafana` container), so editing a file here and running
`docker compose restart grafana` in the observability stack picks it up —
there is no separate copy to keep in sync.

```bash
cd ~/dotfiles/stapler-scripts/observability && make up   # if not already running
open http://localhost:48300   # admin / admin
```

`Dashboards → stapler-squad` in the Grafana UI. `allowUiUpdates: true` is set
on this provider (see that repo's `grafana/provisioning/dashboards/dashboards.yml`),
so an edit made in the Grafana UI writes back to the JSON file here directly
— `git diff` after using the UI to see what changed, same "dashboards as
code" workflow as editing the JSON by hand.

Point stapler-squad's own OTel exporter at this stack per
`docs/how-to/enable-opentelemetry.md` — it defaults to `localhost:4317`
(`telemetry.DefaultOTLPEndpoint`), which is exactly where the stack's
collector listens, so there's usually nothing to configure beyond enabling
the app's own OTel flag.

## Traces on the dashboards

All six dashboards carry a "Traces in Tempo (Explore)" dashboard link (top bar) that opens Explore on the
Tempo datasource with the dashboard's time range. `cgroup-memory` and `pprof-profiles` have only that
link, because no spans correspond to memory or profile data. The other four also have the following. The overview link on `cgroup-memory` and `pprof-profiles` is pre-filtered to server spans slower than 500ms:

| Dashboard | Trace panels (TraceQL, `resource.service.name = "stapler-squad"`) | Metric-panel data links (click a series) |
|---|---|---|
| `http-rpc-connections` | slow (> 500ms) and errored Connect RPC server spans; `Watch*` streams are excluded | streams-by-method panel opens that method's spans |
| `tmux-control-mode` | slow and errored/timed-out `tmux.control_mode.command` spans | rate, timeout, latency panels open spans for that `command` |
| `tymux-backend` | slow and errored `session.backend.*` / `tymux.v1.*` spans (`Attach` excluded) | latency and rate panels open spans for that `backend` |
| `client-network` | slow and errored server RPC spans for the selected `$method` (`Watch*` streams excluded) | error/timeout/p95 stats and per-method panels open server spans for the method(s); the timeout links filter on `error_code = "deadline_exceeded"` (server spans only, which are rare). Latency links filter to spans slower than 500ms, so they can open an empty result when the panel value is lower. Browser-side series are linked to the matching server spans |

Notes:

- Grafana 11.5's Tempo plugin runs TraceQL searches in the browser, so the tables cannot be checked with
  `POST /api/ds/query` (it returns a 500; Grafana logs "backend TraceQL search queries are not supported"). Check a query with
  `GET /api/datasources/proxy/uid/tempo/api/search?q=<traceql>&start=<s>&end=<s>` instead.
- Span attributes used: `rpc.system`, `rpc.method`, `rpc.connect_rpc.error_code`, `command`, `timed_out`,
  `backend`. Hot-path backend operations (`send_keys`, `write_stdin`, `capture_pane`, `resize`)
  have no `session.backend.*` span, so a per-backend link from those series can open an empty result. The tymux
  backend's gRPC client spans (`tymux.v1.*`) do appear in the tables but carry no `backend` attribute.
- Not available: metric-to-trace exemplars. VictoriaMetrics returned no exemplars from
  `/api/v1/query_exemplars` when checked (whether it can store them was not confirmed), and the provisioned Tempo datasource has no `tracesToMetrics`/`tracesToLogs`
  settings (they live in the dotfiles stack, not this repo), so trace-to-metrics and trace-to-logs links
  are not part of these dashboards.
- The existing "Slow browser spans" table on `client-network` needs a web build with
  `NEXT_PUBLIC_OTEL_ENABLED=true`; without it, the panel shows "No data".
