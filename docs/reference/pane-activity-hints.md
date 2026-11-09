# Pane activity hints (squad-side QoS)

Squad classifies every session as interactive, background or normal and sends that to tymuxd so
the daemon can favor the pane someone is typing into. The same classification makes squad's own
background work yield.

## Classification

`internal/qos.Tracker` (`qos.Default`) records input per session. A session is **interactive** for
`qos.InteractiveWindow` (5s) after its last keystroke, **background** while another session is
interactive and it is not, **normal** otherwise. The hub notes input in
`StreamHub.NoteInteractive`. There is no server-side focus or visibility signal yet, so "focused
pane" means "typed into recently".

## What reacts

| Consumer | Behavior |
|---|---|
| `streamhub` activity sink (`activity.go`) | Reports changes to the backend, coalesced, off the input path; sends an initial hint when created; re-sends a non-normal hint every minute; retries an undelivered hint every 2s (a failed final `normal` at teardown at most 5 times); sends `normal` on hub teardown. |
| Background hubs | Skip opportunistic flushes and use a 60ms batch ceiling (`BackgroundBatchWindow`) while another session is interactive. |
| Review-queue, PR status, worktree PR and unfinished-scanner pollers | Skip a tick while anyone is typing, at most `qos.BackgroundMaxDeferral` (30s) in a row. |
| Attach gate (`server/services/attach_gate.go`) | At most 3 terminal attaches run at once, most recently used session first. |

## tymuxd integration point

`session/tymux/pane_activity.go` sends `SetPaneActivity` when the daemon advertises
`pane.activity_hint` in `GetServerInfo`. It is a logged-once no-op when the daemon lacks the
capability or the client cannot send the RPC. The hub only records a hint as sent once the backend confirms it (`SetPaneActivity` returns nil when delivered or permanently impossible, non-nil when a retry may help, e.g. no pane yet). tymuxd drops a hint that has no open Attach stream after 5s, so the session re-sends the last non-normal hint whenever its Attach stream opens or reconnects. Panes are reset to normal when adopted (unless a hub hint was requested since) and on
`Server.Shutdown` (`tymux.ResetPaneActivities`), because the daemon keeps hints until it restarts.

The real sender (`pane_activity_adapter.go`) is compiled in unconditionally; the pinned tymux client carries
the RPC on the production wrapper (`splitTransport`) and `selecting.Client` (compile-time asserted). It only fires when the live daemon advertises
`pane.activity_hint` (tymuxd >= v1.17.0).
