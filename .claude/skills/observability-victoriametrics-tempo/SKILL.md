---
name: observability-victoriametrics-tempo
description: Use when investigating a live stapler-squad performance/latency/error question ("why is X slow", "is Y timing out", "find the outlier") — query the local VictoriaMetrics + Tempo + Pyroscope stack directly instead of guessing or re-instrumenting from scratch. Covers exact endpoints, the OTel-to-Prometheus metric-name mangling gotcha, PromQL patterns for this repo's existing `tmux_control_mode_*` and `http.server.*`/`rpc.server_streams.*` metrics, the Tempo search/trace API, and pulling CPU/heap/goroutine flamegraphs from Pyroscope. For building or editing the Grafana dashboards themselves, see `observability-grafana-dashboards`.
---

# Reading stapler-squad's Observability Stack (VictoriaMetrics + Tempo)

This repo's OpenTelemetry pipeline (see `docs/how-to/enable-opentelemetry.md`) fans out locally to a
docker-compose stack (`observability-*` containers) instead of only Datadog:

```
stapler-squad (OTEL_ENABLED=true)
  --OTLP grpc:4317/http:4318-->  otel-collector-contrib
       traces  --otlp exporter-->      Tempo        (localhost:3200, no auth)
       metrics --prometheus_remote_write--> VictoriaMetrics (localhost:8428, no auth)
       logs    --> debug exporter only (no log backend wired up — check the app's own log file instead)

stapler-squad --profile (pprof HTTP endpoint, :6060, independent of OTEL_ENABLED)
  <--pull scrape-- Grafana Alloy (alloy-config.alloy) --> Pyroscope (localhost:4040, no auth)
```

Collector config (if you need to check the pipeline itself, not just query it):
`~/dotfiles/stapler-scripts/observability/otel-collector-config.yaml`; profiling scrape config:
`~/dotfiles/stapler-scripts/observability/alloy-config.alloy`. Grafana itself runs at
`localhost:48300` (container's internal `:3000` mapped out) with VictoriaMetrics + Tempo + Pyroscope
as datasources and a provisioned `stapler-squad` dashboard folder, but for agent use, query the
backends directly — faster, no auth, no UI to screenshot. For building/editing the dashboards
themselves (not just querying live data), see the `observability-grafana-dashboards` skill.

**Before querying anything**: confirm the live service actually has `OTEL_ENABLED=true` —
check `~/.stapler-squad/service.env`. If it's not set, these backends will be empty/stale for
recent activity regardless of how correct your query is.

## Querying metrics (VictoriaMetrics)

Use `promql-cli` (`~/go/bin/promql-cli` if not on PATH — `go install github.com/nalbury/promql-cli@latest`
if missing), pointed at `--host http://localhost:8428`. Follow the `promql-cli` skill's own query-cost
rules (aggregate in PromQL, don't dump raw series, check cardinality first) — they apply here unchanged.

**The metric-name mangling gotcha**: OTel metrics reach VictoriaMetrics via Prometheus remote-write,
which mangles names. A histogram registered in Go as `tmux_control_mode_command_duration_ms` (already
including a `_ms` unit in its own name) becomes, in VictoriaMetrics:

```
tmux_control_mode_command_duration_ms_milliseconds_bucket
tmux_control_mode_command_duration_ms_milliseconds_count
tmux_control_mode_command_duration_ms_milliseconds_sum
```

— the OTel unit (`ms`, from `metric.WithUnit("ms")` at registration) gets APPENDED as `_milliseconds`
even though the Go-side name already spelled out the unit. **Never guess the exact metric name** — list
what actually exists first:

```bash
promql-cli --host http://localhost:8428 'count({__name__=~"tmux_control_mode.*"}) by (__name__)'
```

A bare `promql-cli --host http://localhost:8428 metrics` (list everything) will usually fail with a
`422 ... exceeds 30000` cardinality error on this instance — always scope with a `{__name__=~"prefix.*"}`
regex filter instead of listing the whole namespace.

### Known instruments already registered in this codebase

| Metric (VictoriaMetrics name) | Type | Labels | Source | What it answers |
|---|---|---|---|---|
| `tmux_control_mode_command_duration_ms_milliseconds_{bucket,count,sum}` | Histogram | `command` (`send-keys`, `capture-pane`, `resize-window`, `display-message`) | `session/tmux/control_mode_observability.go` | End-to-end latency of one tmux control-mode command round trip (enqueue + ack). **No `session` label** — this is a process-wide aggregate across every concurrent tmux session; it cannot tell you if one session is a pathological outlier vs. a systemic issue. For per-session breakdown you need Tempo (below), not this metric. |
| `tmux_control_mode_timeouts_total` | Counter | `command` | same file | Count of control-mode commands whose ctx expired before a response arrived. Always pair with the duration histogram's count for that command to get a real failure *rate*, not just a raw count. |
| `tmux_control_mode_pending_depth` | Histogram | (none) | same file | FIFO backlog depth at the moment a new command was enqueued. Normal is single digits; a documented prior incident (see `controlModeQueueBackpressureThreshold`'s doc comment, `session/tmux/tmux.go`) saw 107-115. |
| `http.server.connections_open` | Observable gauge | — | `server/http_connection_metrics.go` | Live HTTP/1.1 connections (excludes hijacked/WebSocket). |
| `http.server.connections_hijacked_total` | Observable counter | — | same file | Cumulative WebSocket upgrades. |
| `rpc.server_streams.open` | Observable gauge | `method` | `server/services/watch_stream_metrics.go` | Long-lived stream count per RPC method — graph summed by `method` to see how many streams one browser tab holds open. |
| `cgroup_memory_*` | Gauges/counters (Linux only) | — | `telemetry/cgroup_linux.go` | Live cgroup v2 memory pressure/limits. |
| `safeexec.sigkill_escalations` | Counter | — | `executor/safeexec` | SIGKILL escalations after a SIGTERM was ignored. |

### Useful query patterns

```bash
# p99 latency by command, last 10 minutes
promql-cli --host http://localhost:8428 \
  'histogram_quantile(0.99, sum(rate(tmux_control_mode_command_duration_ms_milliseconds_bucket[10m])) by (le, command))'

# Timeout counts by command, last 30 minutes (raw increase, not a rate — small absolute numbers are more readable)
promql-cli --host http://localhost:8428 \
  'sum(increase(tmux_control_mode_timeouts_total[30m])) by (command)'

# Timeout RATE as a fraction of attempts — always compute this, a raw timeout count means nothing without
# the attempt count next to it (this repo has been burned by exactly that: "132 timeouts" sounds bad in
# isolation, but is a genuinely different finding if the attempt count was 200 vs. 20000)
promql-cli --host http://localhost:8428 \
  'sum(increase(tmux_control_mode_timeouts_total{command="send-keys"}[30m])) / sum(increase(tmux_control_mode_command_duration_ms_milliseconds_count{command="send-keys"}[30m]))'

# FIFO backlog depth p99
promql-cli --host http://localhost:8428 \
  'histogram_quantile(0.99, sum(rate(tmux_control_mode_pending_depth_bucket[10m])) by (le))'
```

**Round-number durations are a tell.** If a histogram's p99/p50 lands on a suspiciously exact value
(2000ms, 3000ms, 5000ms), check the code for a `context.WithTimeout` constant matching it — you are very
likely looking at ctx-deadline timeouts, not organically varying processing time. `cmCtx()` in
`session/tmux/tmux.go` is a 3-second constant; `fastLaneCMAttemptTimeout` is 300ms; the three
`SendInputViaControlMode` call sites in `server/services/connectrpc_websocket.go` currently build a
2-second ctx. Match observed latencies against these before theorizing anything more exotic.

## Querying traces (Tempo)

No CLI wrapper exists for Tempo in this repo — use `curl` directly against its HTTP API (this is the one
place raw `curl` is fine; Tempo's search API returns compact trace summaries, not a metrics firehose).

```bash
# Find slow spans by name, minimum duration, recent window (defaults to a reasonable lookback)
curl -s "http://localhost:3200/api/search?tags=name%3Dtmux.control_mode.command&minDuration=2s&limit=20"

# Pull one full trace by ID (from a search result's "traceID" field) to see every child span and its timing
curl -s "http://localhost:3200/api/traces/<traceID>"
```

**Check `rootServiceName`/`rootTraceName` in search results before anything else.** If a span you expect
to be a *child* of a request/connection shows up as its own trace root (root name == the span's own
name), it has NO parent — the code that started it built its context from `context.Background()` instead
of propagating the caller's ctx. This is a real, previously-confirmed gap in this codebase: all three
`SendInputViaControlMode` call sites did exactly this, silently orphaning every `tmux.control_mode.command`
span for user keystrokes from any request/session/WebSocket-connection trace. Don't assume spans are
linked just because the code has tracing calls in it — verify via a live search result.

## Cross-referencing with backend logs

The live service's structured JSON logs are NOT centralized in this stack (no Loki backend wired up —
see the collector config's `logs` pipeline, which only has a `debug` exporter). Find the current log file
directly:

```bash
find ~/.stapler-squad -iname "*.log" -newermt "-30 minutes"
```

The path depends on which state-isolation mode is active (see `docs/reference/state-isolation.md`) — it
moves under `workspaces/<hash>/logs/` or `instances/<name>/logs/` depending on config, not always the
top-level `~/.stapler-squad/logs/staplersquad.log`. Don't assume the top-level path is current; the
`-newermt` filter finds whichever file is actually being written to right now.

## Investigation order

1. **Metrics first** — cheap, aggregate, tells you *whether* there's a problem and roughly *how bad*
   (histogram_quantile, timeout rates). Confirms or kills a hypothesis in one query.
2. **Traces next** — once metrics show an outlier, Tempo tells you *which* specific operation/session and
   *what else was happening* during that span (child spans, gaps, whether parent linkage exists at all).
3. **Logs last** — for the specific timestamp/session a trace already pointed you at, not as a first-pass
   grep across everything.

Querying metrics/traces beats guessing from code reading alone or re-running the live app to "see if it's
slow" — the data already exists and is free to query.
