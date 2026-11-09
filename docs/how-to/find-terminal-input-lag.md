# Find where terminal input lag comes from

Open Grafana (`http://localhost:48300`) -> `stapler-squad: terminal experience`
(`ssq-terminal-experience`). Read it top to bottom; stop at the first panel that is bad.
Stage definitions are in `docs/reference/terminal-latency-telemetry.md`.

Without Grafana: `curl -s localhost:8543/actuator/terminal-latency?format=text` shows the same
stages, drops and resyncs over the last 5 minutes.

## 1. Alerts row

Red tiles say the user is feeling it. Echo paint p95 over 150 ms is the headline. If everything
is green but typing still feels slow, the lag is outside what is measured (the terminal
renderer's own work, or the remote host).

## 2. Keystroke to echo

- **Echo breakdown (mean, stacked)** splits the round trip. The biggest band is the culprit:
  - *send queue (browser)* large: the browser held the keystroke. Check `eventloop_lag` and
    *Browser WebSocket backlog*; a busy main thread or a full uplink.
  - *server* large: go to section 3.
  - *network + return* large: the path between squad and the browser (remote listener, VPN),
    or the browser was slow to receive. Compare `listener` in *Echo painted*.
  - *render* large: the browser took long to paint; compare *Browser: long task p95*.
- **Server input path p95** shows `stream_lock_wait` (typing queued behind a paste or resize on
  the same Attach stream), `stream_send` (gRPC Send is slow), `input_bookkeep` (should be ~0).

## 3. Backend calls

- **tymux capture_pane decomposition**: total minus `rpc` minus `render` minus `revive` is the
  `squad-side residual`. Large `rpc` means the daemon (or the squad<->tymux connection) is slow;
  large residual means squad was busy before the call (locks, scheduling); large `render` means
  big screens.
- **tmux capture_pane decomposition**: exec-gate wait vs subprocess exec vs control-mode command.
- **Slow calls by session** names the sessions behind slow calls (20 busiest, rest `other`). Open
  Tempo from the dashboard link for the span: name `session.backend.<op>` or `terminal.input`,
  attribute `session`.
- **Contended Instance lock waits**: typing met a lock held by something else.

### Splitting `echo_server` on tymux

Panel *tymux echo_server decomposition* stacks the mean time of each piece against the
`echo_server` line. Read the tallest block:

| Block | Means |
|---|---|
| `input_bookkeep` | squad work before the write |
| `echo_daemon` | everything from handing the keystroke to the tymux session to the daemon stamping the echo: `stream_lock_wait` and `stream_send` (check *Server input path*), squad -> daemon transit, pty write, the app producing its echo, the daemon's send queue. Large with quiet *Server input path* = slow app or loaded host |
| `output_transit` | daemon -> squad gRPC transit and receive queue |
| `tymux_fanout_wait`, `output_queue_wait` | squad draining the daemon stream, then the hub batch |
| `ws_write_wait`, `ws_write` | the browser socket |

`echo_daemon` and `output_transit` need a daemon with capability `output.sent_at` (v1.17.0+); the
panel shows nothing for them on older daemons. Check with `curl -s localhost:8543/actuator/terminal-latency`
for stage `echo_daemon`.

## 4. Head-of-line waits per hop

Walk the path output takes: tymuxd -> squad -> browser.

| Hop | Panel | Healthy | Bad means |
|---|---|---|---|
| tymux -> squad | `tymux_frame_handle`, `tymux_fanout_wait`, `tymux_pty_write` | p95 under a few ms | squad is slow to drain the daemon stream |
| hub -> subscriber | *Hub subscriber queue wait* | p95 under a few ms | the subscriber's writer is slower than output |
| squad -> browser socket | `ws_write_wait`, `ws_write` | p95 under a few ms | slow client or a blocked socket |
| browser mux | *HOL wait* | p99 under ~10 ms | one stream's burst delays others |

## 5. Queues, drops and resyncs

Depth near the cap (100 hub, 64 fanout) precedes drops. Any sustained drop rate is a visible
glitch and forces repaints. A resync spike right after tab focus is `server_stale_dims`, the
avoidable kind.

## 6. Runtime and host

Scheduler latency p99 over 100 ms, GC pauses or goroutine growth mean the server process itself
is starved, which shows up as the squad-side residual and `pm_lock_wait`. DB pool wait matters
only for paths that touch the database. Host pressure panels need Linux cgroup data and are
empty on macOS.

## Tuning

| Knob | Effect |
|---|---|
| `STAPLER_SQUAD_SLOW_CALL_MS` (default 250) | threshold for slow-call counter and spans |
| `STAPLER_SQUAD_SLOW_OP_MS` (default 2000) | threshold for the still-running goroutine dump |
| `STAPLER_SQUAD_TERMINAL_ECHO_P95_MS` (default 150) | echo alert limit |

Panels for stages added in this change stay empty until a build containing it runs; a panel
that is empty after that is a real "nothing recorded", not a typo (a test checks every stage
name the dashboard queries).
