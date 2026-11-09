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
| `streamhub` activity sink (`activity.go`) | Reports changes to the backend, coalesced, off the input path; re-sends a non-normal hint every minute; sends `normal` on hub teardown. |
| Background hubs | Skip opportunistic flushes and use a 60ms batch ceiling (`BackgroundBatchWindow`) while another session is interactive. |
| Review-queue, PR status, worktree PR and unfinished-scanner pollers | Skip a tick while anyone is typing, at most `qos.BackgroundMaxDeferral` (30s) in a row. |
| Attach gate (`server/services/attach_gate.go`) | At most 3 terminal attaches run at once, most recently used session first. |

## tymuxd integration point

`session/tymux/pane_activity.go` sends `SetPaneActivity` when the daemon advertises
`pane.activity_hint` in `GetServerInfo`. It is a logged-once no-op when the daemon lacks the
capability or the client cannot send the RPC. Panes are reset to normal when adopted and on
`Server.Shutdown` (`tymux.ResetPaneActivities`), because the daemon keeps hints until it restarts.

The pinned tymux Go client predates the RPC, so the real sender
(`pane_activity_adapter.go`) sits behind the `tymux_pane_activity` build tag. To enable it:

1. tymux must expose `SetPaneActivity` on `selecting.Transport` and `selecting.Client`
   (tymux#86 regenerates the connect client but does not touch `selecting`).
2. Bump the `github.com/tstapler/tymux/clients/go` pin, then delete the build tag line from
   `pane_activity_adapter.go`.

To compile-check the adapter before the pin moves, build with a local `-modfile` whose `replace`
points `github.com/tstapler/tymux/clients/go` at a checkout of the tymux branch, plus
`-tags tymux_pane_activity`. Never commit that replace.
