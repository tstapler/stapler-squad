# Architecture Research: session-teardown-goleak-fix

## Existing orchestration point

`Instance.Destroy()` (`session/instance.go:1561`) is already the single "stop
everything" entry point — no new top-level owner is needed. Its call order is
strictly sequential in one goroutine:

1. `i.StopController()` (`session/instance_controller.go:148`) — runs
   **synchronously**, before anything else.
2. `destroyChain()` (VNC/CDP/`KillSession`/`CleanupWorktree`) — runs in a
   background goroutine bounded by `destroyChainTimeout` (5s); `KillSession`
   is what flips `TmuxSessionExists()` to false.

`StopController` → `ControllerManager.stopAndClear()`
(`session/controller_manager.go:56`) → `ClaudeController.Stop()`
(`session/claude_controller.go:397`), which already does two real
`wg.Wait()`-backed joins: `CommandExecutor.Stop()` (`session/command_executor.go:158`)
and `ResponseStream.Stop()` (`session/response_stream.go:391`). `ResponseStream`
also already tracks the `MangleCorrelator.StartEviction` goroutine in its own
`wg` (`session/response_stream.go:172-180`, fixed under BUG-025 — confirmed via
`docs/bugs/fixed/BUG-025-escape-analytics-session-id-and-mangle-detection.md`).
So 3 of the 6 goroutines named in requirements.md are **already correctly
joined** by existing `Stop()` methods; only two are not:

- `ClaudeController.runStatusChangeLoop` (`session/claude_controller.go:1178`) —
  started with a bare `go cc.runStatusChangeLoop(innerCtx)`
  (`claude_controller.go:365`), no `WaitGroup` field on `ClaudeController` at
  all, `Stop()` cancels the context but never waits for this goroutine to
  return.
- `ratelimit.PTYConsumer.pollLoop` (`session/detection/ratelimit/integration.go:143`) —
  `PTYConsumer.Stop()` (`integration.go:128`) cancels via `cancelFn()` and
  clears `running`, but has no `sync.WaitGroup`/done-channel, so it returns
  before `pollLoop` has actually exited. `ClaudeController.Stop()` calls this
  `Stop()` at phase 3 (`claude_controller.go:441`, `rlh.Stop()`) but, same
  gap, doesn't block on it either.

The sixth item, the tmux diagnostic `RestoreWithWorkDir`'s `os/exec.(*Cmd).Wait`
goroutine, is unrelated to `Instance`/`ClaudeController` ownership — it's
internal to `session/tmux` and out of scope for this fix per requirements.md's
"not a rewrite" framing.

## Integration point: DeleteSession → Instance.Destroy

`SessionService.DeleteSession` (`server/services/session_service.go:3092`)
calls `liveInst.Destroy` inside `waitForDestroyLoggingSlowCleanup`
(`session_service.go:3149`), which runs `Destroy()` in a goroutine and is
itself only bounded by `deleteSessionCleanupTimeout` — on timeout it logs a
warning and returns, but the goroutine (and `Destroy()` inside it) keeps
running to completion. This means `Destroy()`'s synchronous `StopController()`
prefix is already tolerated as a slow/blocking call by the surrounding
architecture; nothing here needs a new timeout wrapper.

Because `StopController()` runs strictly *before* `destroyChain()`'s
`KillSession()` call in the same goroutine (program order, no concurrency
between them), fixing the two joins above means: **by the time
`TmuxSessionExists()` goes false, `ClaudeController`'s and `PTYConsumer`'s
goroutines are already guaranteed to have exited.** The existing test helper
`waitForTmuxTeardown` (`server/server_integration_test.go:628`, which only
polls `TmuxSessionExists()`) becomes sufficient automatically once the two
component fixes land — no new `Instance.Stopped()` channel is required to
satisfy the acceptance criteria.

## Stopped() precedent confirmed

Read `server/services/claude_settings_watcher.go`'s `Stopped()`
(`claude_settings_watcher.go:320`): it's a `sync.Once`-guarded `close(w.stopped)`
on a `chan struct{}` field, returned by `Stopped() <-chan struct{}`, closed
once from the watcher's own exit path — the same shape as
`session/history_watcher.go:74` and `session/detection/plugin_watcher.go:43`.
This is the pattern to reach for *if* a channel-based signal is ever needed,
but per the ordering argument above it isn't needed here — a plain
`sync.WaitGroup` (matching `CommandExecutor`/`ResponseStream`'s own idiom in
the same package) is the more local, more consistent fix for the two gaps.

## Disposition: minimal seam, not a refactor

This is not a hotspot needing restructuring. `Instance`/`ClaudeController`'s
shutdown chain is already correctly designed (synchronous `StopController()`
before the async tmux/git chain, `wg.Wait()`-joined sub-components) — two
components just don't participate in the pattern the rest of the file already
uses. Recommended fix, entirely inside existing files:

1. Add a `wg sync.WaitGroup` field to `ClaudeController`, `Add(1)`/`defer Done()`
   around the `go cc.runStatusChangeLoop(innerCtx)` launch site, and
   `cc.wg.Wait()` in `Stop()` after `cancelFn()` (mirroring
   `ResponseStream.Stop()`'s own comment: "blocking call that waits for the
   streaming goroutine to finish").
2. Add the same `wg sync.WaitGroup` + `Add`/`Done`/`Wait` to `PTYConsumer`
   (`session/detection/ratelimit/integration.go`), wrapping `pollLoop`'s
   launch in `Start()` and waited on in `Stop()`.
3. No change needed to `waitForTmuxTeardown`, `Instance`, or `DeleteSession` —
   the existing poll-`TmuxSessionExists()` helper becomes correct as a side
   effect of (1) and (2), given the program-order guarantee above.

One risk to verify during implementation (not blocking the disposition, but
worth a targeted check before merging): `StopController()` holds `i.mu`
across the entirety of `ClaudeController.Stop()`, including the new
`wg.Wait()` calls. This is already true today for the existing
`executor.Stop()`/`rs.Stop()` joins, so it's not a new class of risk, but
confirm no `StatusChangeListener` callback invoked from `runStatusChangeLoop`
can re-enter any `Instance` method that also takes `i.mu` — that would turn a
"wait a bit longer" into an actual deadlock. A quick grep of registered
listeners should settle it; if one is found, run the listener callbacks
outside the lock (e.g. copy-then-call, which `runStatusChangeLoop` already
does for the listener slice itself) rather than restructuring `Stop()`.
