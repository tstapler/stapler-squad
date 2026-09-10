# 2026-09-06: `readAttachLoop`/`ReconnectLoop` incident history

Relocated from `session/tymux/stream.go`'s doc comments during code review (trimmed there
per CLAUDE.md's comment-proportionality rule) — kept here so the history isn't lost, just
moved off the hot path a reviewer of an unrelated diff would otherwise have to read past.

## Incident #1 — clean exit misclassified as transport drop (commit `477eacd6a`)

Before `classifyStreamEnd`'s clean-exit check existed, a clean pane exit was
misclassified as `ReasonTransportDrop`. `ReconnectLoop`'s reattach dial then failed with
`errPaneDead` (the pane really was dead, just not because tymuxd restarted), and
`daemonRestarted`'s `errors.Is` check couldn't tell that apart from an actual daemon
restart. Consequence: a needless replacement process spawned via `ReviveSession`, and
`BackendRestarted()` falsely reported `true`.

## Incident #2 — reopen misclassified as transport drop, wedging teardown

Before `classifyStreamEnd`'s `ctx.Err()` check existed, `openStandingStream`'s own
tear-down-before-reopen (`teardownStandingStream`, not a `Close()`/`DetachSafely()`) was
misclassified the same way as incident #1: the old reader fell through to
`ReconnectLoop` instead of exiting. Since nothing ever closes `abortReconnect` for a mere
reopen, that dial blocked indefinitely — wedging `teardownStandingStream`'s wait on
`done`.

## Incident #3 — reader stuck in `Receive()` ignoring ctx cancellation

Referenced by `maxTeardownWait`'s doc comment: a reader stuck in a `Receive()` call that
doesn't honor ctx cancellation could stall a reopen/`RestoreWithWorkDir` call
indefinitely. `maxTeardownWait` (5s) bounds `teardownStandingStream`'s wait so an
abandoned reader doesn't stall the caller forever, at the cost of the old goroutine
possibly outliving the wait in that specific failure mode.

## Fix (2026-09) — `ReconnectLoop`'s give-up `Reason` returned, not re-derived

`readAttachLoop` used to call `s.reasonForReconnectFailure()` after `ReconnectLoop`
returned, which re-checked `s.closing.Load()` a second time to decide between
`ReasonDeliberateClose` and `ReasonReconnectExhausted` — duplicating a check
`ReconnectLoop` had already made internally, a few instructions earlier in the same
goroutine, to decide the same thing for its own `lifecycle.RecordEnd` call. Since
`s.closing` is set asynchronously by `Close()`/`DetachSafely()` from a different
goroutine, the two checks could theoretically observe different values in a narrow
window, making `tymux_reconnect`'s `RecordEnd` and `tymux_stream`'s `EndGeneration`
disagree about why the same logical event happened. Fixed by having `ReconnectLoop`
return the `lifecycle.Reason` it already computed (a fourth return value alongside its
existing `bool`), which `readAttachLoop` now reuses directly instead of re-deriving it.
