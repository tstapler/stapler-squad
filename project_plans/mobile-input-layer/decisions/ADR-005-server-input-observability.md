# ADR-005: Server input observability: class-only metrics, bounded actuator tally

## Status
Accepted (2026-11-06)

## Context
There is no signal showing whether a phone's keystrokes reach the server. Input content must never be logged. Per-session metric labels are unbounded (`docs/reference/client-network-telemetry.md` label-bounds policy).

## Decision
- New `server/services/terminal_input_metrics.go` with `recordTerminalInput(path, listener, data, stage)` with `stage` of `received|forwarded`: `received` is recorded before permission/read-only checks and `forwarded` only where the writer actually forwards, at: `dispatchInputReadLoopFrame` (`connectrpc_websocket.go:3129`, covers the :1653 and :2489 closures), the capture-pane call (:3849), `connectrpc_websocket_shell.go:265`, and `session_service_stream_terminal.go:439`. `forwarded` is recorded at EVERY site: after the site's own gate passes and the write is issued (the shared loop via the closure result; the other three directly after their gate or, where there is no gate, right after the write, documented in the site table), with one test per site (four `path` labels `cm|shell|capture|stream`; the `:1653`/`:2489` closures share `cm`; no `hub` path exists), so `received - forwarded` never conflates denial with a path that does not record `forwarded`. `ws_terminal_mux_bridge.go` is confirmed by test to reach one of these.
- Classifier is O(1) on the first byte plus a bounded scan of at most the first 64 bytes: `escape` (first byte 0x1b), `text` (printable UTF-8 plus `\r \t \n \x7f`), `other`.
- OTel counters `terminal_input_frames_total` and `terminal_input_bytes_total`, labels `kind`, `listener`, `path`, `stage` only (no `accepted` label: the entry-side call cannot know the permission result; the `onInput` closures at :1653/:2489 return the forwarded result so permission denials show as `received` without `forwarded`). No session id/title label. Created with the existing `counterNamed` helpers (`ws_unary_metrics.go:30-50`).
- Per-session detail: bounded LRU tally (256 sessions) behind `GET /actuator/terminal-input` (JSON, `?format=text`), registered beside `/actuator/ws-unary` (`server/actuator.go:45`).
- Logs: Info on first input per stream, throttled summary at most 1/min/stream; never content. Tests inject a logger (no global slog swap; BUG-087).
- Client events `cm.input` and `cm.ime_anomaly` (mapping to the requirements names `input_dropped_disconnected_total` and `ime_event_sequence_unrecognized_total` is documented in the plan and the telemetry reference) added to the allow-list in `server/analytics/client_metrics.go` with fixed-enum fields.

## Alternatives rejected
Prometheus per-session labels (cardinality); a new beacon RPC (sinkguard write classification, registry churn).

## Consequences
"Phone typed but nothing arrived" = client `cm.input emitted` > 0 while `terminal_input_frames_total{kind="text",listener="remote"}` is flat; `received` positive with `forwarded` flat means read-only or permission denial. Counter cost is an atomic add per frame.
