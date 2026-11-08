# Terminal latency telemetry

Per-stage latency for the terminal input and output path, measured in the browser and on the
server, with no metrics backend needed to read it.

```
curl -s localhost:8543/actuator/terminal-latency?format=text
curl -s localhost:8543/actuator/terminal-latency | jq '.alerting, .stages'
```

The same observations feed the OTel histogram `terminal_latency.stage_duration`
(VictoriaMetrics: `terminal_latency_stage_duration_milliseconds_*`) with labels `stage`,
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
at most one echo sample per 250 ms and 60 events per minute per tab, carrying only a stage name,
a duration and `listener`. The browser does not know the session backend, so its rows are under
`backend="unknown"`; use `echo_server` for a tymux-vs-tmux echo comparison. Long main-thread
tasks while typing are the existing `client_long_task_duration_*` metric.

Echo stages pair a keystroke with the next output after it. That is an upper bound when other
output is streaming, which is exactly the "input behind bulk output" case worth seeing. A reply
over 2 s (`echo_server`) or 3 s (browser) is dropped as unrelated.

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
