# Terminal latency telemetry

Per-stage latency for the terminal input and output path, measured in the browser and on the
server, with no metrics backend needed to read it.

```
curl -s localhost:8543/actuator/terminal-latency?format=text
curl -s localhost:8543/actuator/terminal-latency | jq '.alerting, .stages'
```

The same observations feed the OTel histogram `terminal_latency.stage_duration`
(VictoriaMetrics: `terminal_latency_stage_duration_{bucket,sum,count}`, no unit suffix; verified against the live store) with labels `stage`,
`backend` (`tmux`, `tymux`, `native`, `unknown`) and `listener` (`local`, `remote`, `unknown`).
Every label is a fixed enum (`internal/termlat`); the browser cannot add a series.

The actuator shows p50/p95/p99 over the last 5 minutes per stage x backend x listener, `max`
since process start, and an `alerting` list.

## Stages

| Stage | Where | Meaning |
|---|---|---|
| `input_total` | server | keystroke frame handled -> backend send returned |
| `input_send` | server | the backend send alone (daemon ack for tymux, control-mode write for tmux) |
| `input_bookkeep` | server | per-keystroke bookkeeping done before the send; the number that must stay near 0 |
| `echo_server` | server | keystroke received -> next output write to the socket (heuristic, see below) |
| `ws_write_wait` | server | waiting for the per-stream socket write lock |
| `ws_write` | server | one socket write (mux queue or TCP) returning |
| `output_send` | server | hub batch handed to the transport -> write returned (hub path only) |
| `output_queue_depth` | server | frames already queued for a subscriber at each enqueue (a count, hub path only) |
| `echo_arrive` | browser | keystroke sent -> next output frame reached the component |
| `echo_paint` | browser | keystroke sent -> echo painted (two animation frames after arrival): input-to-paint |
| `ttfp_connect`, `ttfp_first_data`, `ttfp_first_paint` | browser | terminal mounted -> stream open / first output / first paint |

Browser stages arrive as `cm.term_latency` analytics events (`lib/telemetry/terminalLatency.ts`),
at most one echo sample per 250 ms and 300 events per minute per mounted terminal (the event-loop sampler reports at most one event per 5 s and is outside that cap), carrying only a stage name,
a duration and `listener`. The browser does not know the session backend, so its rows are under
`backend="unknown"`; use `echo_server` for a tymux-vs-tmux echo comparison. Long main-thread
tasks while typing are the existing `client_long_task_duration_*` metric.

Echo stages pair a keystroke with the next output after it. That is an upper bound when other
output is streaming, which is exactly the "input behind bulk output" case worth seeing. A reply
over 2 s (`echo_server`) or 3 s (browser) is dropped as unrelated.

## Decomposition stages

These split one slow call or frame into the places it can wait. All are in the same histogram.

| Stage | Where | Meaning |
|---|---|---|
| `output_queue_wait` | server (hub) | a frame sat in a subscriber's 100-slot outbound queue until its writer took it |
| `tymux_frame_handle` | squad<->tymux | daemon frame received -> fanout broadcast done |
| `tymux_fanout_wait` | squad<->tymux | broadcast -> the PTY adapter read it from its 64-slot fanout queue |
| `tymux_fanout_depth` | squad<->tymux | queue depth at each broadcast (a count) |
| `tymux_pty_write` | squad<->tymux | the adapter's write of one frame into the PTY socketpair |
| `capture_render` | tymux | `CellsToSGR` / `RenderSnapshotRedraw` after the capture RPC returned |
| `capture_revive` | tymux | dead-pane revive plus the retried capture (only when a capture hit a dead pane) |
| `stream_lock_wait` | tymux | waiting for the Attach stream's input or frame lock before a send (keystroke, resize, paste chunk) |
| `stream_send` | tymux | one Attach stream `Send`; gRPC flow-control stalls land here |
| `pm_lock_wait` | server | a contended wait for `Instance.pmMu`. An uncontended call records nothing, so the sample count is how often a call met the lock |
| `echo_daemon` | server (tymux) | browser keystroke handed to the tymux session -> the daemon stamped the first output frame after it. Includes `stream_lock_wait`, `stream_send`, squad -> daemon transit, pty write, app/shell echo and the daemon's send queue. Needs capability `output.sent_at` (daemon v1.17.0+); no samples otherwise |
| `output_transit` | server (tymux) | daemon stamped the first frame after a browser keystroke -> squad handled it (recorded even when the gap exceeds the 2 s `echo_daemon` bound): gRPC/HTTP2 transit and the client receive queue. An upper bound (the daemon may pull a frame before flow control releases it) |
| `input_queue` | browser | keystroke entered the send queue -> handed to the socket |
| `ws_buffered_kib` | browser | KiB the socket still had buffered when the keystroke was sent, rounded up so any backlog is at least 1 (a count, not ms; the histogram tops out at 5000) |
| `echo_render` | browser | echo frame arrived -> painted |
| `eventloop_lag` | browser | worst drift of a 100 ms timer in a 5 s window, reported when over 16 ms; ticks while the tab is hidden, and the first one after it returns, are skipped |

The time a frame spends inside the gRPC client's flow-control window before `Receive` returns is
`output_transit` on daemons that stamp frames (capability `output.sent_at`, v1.17.0+). On older
daemons it is not observable: `tymux_frame_handle` starts when `Receive` returns, and a stall before
that shows up as late output at the browser, not as a stage. Daemon and squad must share a host
(the stamp is wall-clock); a stamp from the future drops both samples.

Residuals are computed on the dashboard, not recorded: `capture_pane` squad-side time is the
`session.backend.capture_pane` duration minus the RPC round trip (`tymux_rpc_duration_ms`),
`capture_render` and `capture_revive`; echo "network + return" is `echo_arrive` minus
`echo_server` minus `input_queue`. Both use means (they add; percentiles do not).

On tymux, `echo_server` splits into `input_bookkeep` (squad pre-write), `echo_daemon` (squad send,
daemon, app echo; it contains `stream_lock_wait` and `stream_send`), `output_transit`, `tymux_fanout_wait`, `output_queue_wait` (hub
batch) and `ws_write_wait` + `ws_write`; the dashboard stacks them. The pieces are an
approximation: `echo_daemon` and `output_transit` attribute the first stamped frame after a
keystroke, which under bulk output need not be its echo, and a second keystroke typed before the
first echo arrives replaces the first (as in `echo_server`). Only browser keystrokes start the
clock; agent prompts and scroll forwarding do not, and a send that fails is forgotten. The daemon
and squad must share a host: a daemon clock that runs behind inflates both stages by the offset.

## Dropped frames

`terminal_frame_drops_total{cause,backend}` and the `drops` section of the actuator count output
frames lost to a full queue: `hub_queue_full` (a hub subscriber's outbound queue) and
`tymux_fanout_full` (a tymux fanout subscriber). Each drop makes that subscriber repaint.

## Slow calls

A `capture_pane`, `resize`, `write_stdin` or `send_keys` call slower than
`STAPLER_SQUAD_SLOW_CALL_MS` (default 250 ms) increments
`terminal_slow_call_total{op,backend,session}` and writes a Tempo span after the fact, with the
start time backdated: name `session.backend.<op>`, attributes `backend`, `session`,
`duration_ms`. A keystroke slower than the threshold also gets a `terminal.input` span with
`terminal.input.bookkeep` and `terminal.input.send` children. Fast calls never touch the tracer.
Spans parent to the caller's context when it has a span (the hub resync and `write_stdin` paths
pass one); the per-keystroke path has no request span, so its spans are roots, matched to the
backend span by `session` and time overlap.

The `session` label keeps only the 20 busiest sessions of each 10 minute window (their id,
truncated to 24 chars); every other session is `other`, and at most 100 distinct values are ever
created per process, so the series count is bounded.

## Alerts

A rolling p95 over its limit with at least 20 samples logs `terminal latency p95 over threshold`
at most once a minute and appears in `alerting`. Limits: `echo_paint` and `echo_server` 150 ms
(`STAPLER_SQUAD_TERMINAL_ECHO_P95_MS`), `input_total` 100 ms.

## Resync counters

`terminal_resync_total{reason,backend}` and the `resync` section of `/actuator/terminal-latency`
count resyncs (each costs the server a full-pane capture):

| Reason | Meaning |
|---|---|
| `visibility` | the browser sent a resync on tab/window focus or visibility |
| `skipped_fresh` | focus/visibility, but the size is unchanged and the last sync is under 60 s old (`lib/terminal/resyncPolicy.ts`) |
| `skipped_unavailable` | focus/visibility, but the terminal could not be measured |
| `deferred_hidden` | focus/visibility reached a pane that was hidden: it is marked stale and resyncs when shown |
| `stale_shown` | a stale pane was shown and resynced (subject to the same skip rules) |
| `skipped_output` | focus/visibility, but output arrived within 5 s on a connected stream |
| `watchdog_retry` | a resync got no reply within the stall budget and was re-sent once |
| `watchdog_disconnect` | the retry also stalled and the stream was dropped and re-attached (spaced 500 ms apart tab-wide) |
| `server_request` | the server received a mid-stream CurrentPaneRequest (resize, visibility, or manual) |
| `server_stale_dims` | the same, flagged by the client as a same-size visibility resync: the avoidable kind |

Browser reasons arrive as `cm.term_resync` events with `backend="unknown"`; server reasons carry the real backend.

At most two resyncs run in flight across the tab (`lib/terminal/resyncGate.ts`); the rest wait, foreground first, with
up to 200 ms of jitter. `resync_queue_depth` (a count) is the queue length each time a resync had to wait. The stall
budget is the larger of 10 s and 3x the recent p95 resync round trip for that terminal.
