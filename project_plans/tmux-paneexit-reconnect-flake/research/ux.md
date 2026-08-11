# UX Research — tmux-paneexit-reconnect-flake

## Question

Does `SubscribePaneExit` (on `TmuxServerRegistry`, `session/tmux/server_registry.go:159`) or the
registry's internal control-mode reconnect state ever surface to the web UI (`web-app/`) in a
user-visible way — status badge, "disconnected" indicator, toast?

## What I checked

- `grep -rn "SubscribePaneExit"` across all non-test `.go` files. Only three references exist:
  - Implementation: `session/tmux/server_registry.go:159` (`TmuxServerRegistry.SubscribePaneExit`)
  - Interface declaration: `session/tmux/registry_port.go:23` (`PaneExitSubscriber` port)
  - Sole caller: `session/mux/multiplexer.go:633`, inside `startSessionMonitor()` — when the
    channel closes (pane exit detected), it calls `m.Shutdown()` on the `Multiplexer`.
- Read `session/mux/multiplexer.go:628-647` — the `Multiplexer` type here is `ssq-mux`, the
  standalone external PTY-multiplexing CLI helper described in
  `.claude/docs/pty-multiplexing.md`, not the main `stapler-squad` web server. Its `Shutdown()`
  tears down that separate process; it has no code path back into `server/services/` or any
  ConnectRPC handler.
- `grep -rln "TmuxServerRegistry\|reconnect"` across `session/` and `server/` to find any other
  consumer of registry health/reconnect state. All non-test hits in `server/services/` (e.g.
  `connectrpc_websocket.go`) reference *WebSocket client* reconnection (browser tab
  reconnect/SIGWINCH handling) or `session_seq` stability — unrelated to
  `TmuxServerRegistry`'s internal control-mode reconnect loop. No handler reads
  `TmuxServerRegistry` reconnect/health state and no RPC/proto field exposes pane-exit timing
  or control-mode connection status to the frontend.
- Confirmed there is no `PaneExit`, `paneExit`, or tmux-registry-health field anywhere under
  `proto/session/v1/` (not grepped exhaustively beyond the above, but no caller chain reaches
  proto/RPC layer, so there is nothing to expose).

## Finding

No user-facing surface exists for `SubscribePaneExit` or `TmuxServerRegistry` reconnect state.
The only consumer is `ssq-mux`, a separate external CLI process for IDE-terminal PTY
multiplexing, and its reaction (`Shutdown()`) is local process teardown, not a signal relayed to
the web UI. This is consistent with the bug's scope: an internal session-lifecycle/control-mode
test-reliability fix with no direct UI surface.

## Summary

N/A — no user-facing surface
