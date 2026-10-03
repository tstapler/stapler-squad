# Research: Stack/Library Patterns — session-teardown-goleak-fix

## 1. goleak (go.uber.org/goleak v1.3.0, confirmed `go.mod:55`/`go.sum:405-406`)

The failing assertion (`server/server_test.go:257`) uses per-test `goleak.VerifyNone`, not a
`TestMain`-level `goleak.VerifyTestMain`. Neither `goleak.IgnoreTopFunction` nor a retry/backoff
option is the right tool here: those mask a goroutine stack pattern, but the leaked goroutines
here are real, live, session-specific goroutines from the *previous* test, not runtime-internal
noise — masking them would hide a genuine future leak in the same code paths. goleak itself needs
no change; the fix belongs entirely on the producer side (make the preceding test's teardown
actually synchronous before the next test's snapshot), which is what the acceptance criteria
already say.

## 2. The `Stopped() <-chan struct{}` precedent (read from source, all three)

All three cited examples use the identical, minimal shape — not `sync.Once`-guarded, because each
has exactly one producer goroutine:

```go
type X struct{ stopped chan struct{} /* created via make(chan struct{}) in the constructor */ }
func (x *X) Stopped() <-chan struct{} { return x.stopped }
func (x *X) run(ctx context.Context) {
    defer close(x.stopped)   // single defer in the single loop goroutine — closes exactly once
    ...
}
```
(`session/history_watcher.go:73-79`, `session/detection/plugin_watcher.go:41-45,120`,
`server/services/claude_settings_watcher.go:317-320,224`). `HistoryFileWatcher.Start` also closes
`stopped` immediately on early-return paths that never spawn the goroutine (lines 42/48/55), so
`Stopped()` is safe to select on even if `Start` bailed out before `go w.run(ctx)`.

## 3. What's already joined vs. what's genuinely a gap (verified by reading each Stop())

Two of the six listed leak sources are *already* correctly joined and not actually part of the
bug: `session/response_stream.go`'s `ResponseStream.Stop()` (line 391-405) and
`session/command_executor.go`'s `CommandExecutor.Stop()` (line 158-172) both call their own
`wg.Wait()` synchronously, and `ClaudeController.Stop()` (`session/claude_controller.go:397-458`)
calls both synchronously. The real gaps, confirmed by reading each `Stop`/`Start` pair directly:

- `session/detection/ratelimit/integration.go`'s `PTYConsumer.Stop()` (line 128-141) only calls
  `cancelFn()` — no `WaitGroup`, no done-channel, no join on `pollLoop` (line 143).
- `session/claude_controller.go`'s `runStatusChangeLoop` goroutine (started bare at line 365,
  `go cc.runStatusChangeLoop(innerCtx)`) is never tracked by any WaitGroup — `Stop()` cancels the
  shared context but joins nothing for this specific goroutine.
- `pkg/analytics/mangle_correlator.go`'s `StartEviction` (line 180-191) has no `Stop`/`Stopped`
  method at all; its only caller, `pkg/analytics/escape_code_parser.go:147`, doesn't wrap it in a
  WaitGroup either — it's launched and forgotten.
- `session/tmux/tmux.go:1730`'s `RestoreWithWorkDir` diagnostic goroutine
  (`go func(cmd, name, once) { once.Do(func(){ err = cmd.Wait() }) ...}`) is completely untracked
  — this is the literal BUG-084 recurrence goroutine, fire-and-forget by construction.

These four need a join added; the fix should not touch `ResponseStream`/`CommandExecutor`, which
are already correct.

## 4. Existing WaitGroup-join idiom to copy, and the bounded-timeout helper

`session/session_driver.go`'s `StopSessionDriver` (line 207-221) is the best in-repo template for
"signal + bounded join": it does `stopper.once.Do(func() { close(stopper.stop) })` then
`syncutil.WaitWithTimeout(&inst.driverWG, driverStopTimeout)`, logging a warning (not failing) on
timeout. `internal/syncutil.WaitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool`
(`internal/syncutil/syncutil.go:14-26`) is the one shared bounded-wait primitive in the repo —
it races `wg.Wait()` against `time.After(timeout)` in a helper goroutine and returns a bool rather
than failing the caller. This directly satisfies acceptance criterion 5 ("any new wait has a
bounded timeout") without inventing a new primitive; `session/instance.go`'s `driverWG`/
`hibernateWG` fields (lines 462, 466) are the matching per-`Instance` WaitGroup pattern to extend.

## 5. Existing aggregate join at the service level (partial, and in the wrong package)

`server/services/session_service.go` already has a service-level join:
`deleteCleanupWG sync.WaitGroup` (line 248) is `Add`ed via `trackCleanup` (line 316-329), which
wraps every `liveInst.Destroy` call made from `DeleteSession` (line 3148-3149). Both
`Shutdown()` (line 1068) and an already-written-but-currently-unused-by-tests private helper,
`waitForPendingCleanup()` (line 1081-1083 — its doc comment describes exactly this "a test must
block before trusting cleanup finished" scenario), call `s.deleteCleanupWG.Wait()`. Two caveats
for using this in the fix:

- It's unexported and lives in package `server/services`; the flaky test lives in package
  `server` (`server/server_integration_test.go`), so either an exported wrapper is needed or the
  test needs a different accessor for the same `SessionService`.
- Waiting for `Destroy()` to return is necessary but **not sufficient** — `Destroy()`'s internal
  `destroyChain()` currently has no join for the four gaps in §3, so even a correctly-wired
  `waitForPendingCleanup()` call from the test would still race those four goroutines. The
  `Stopped()`/WaitGroup+`syncutil.WaitWithTimeout` joins need to be added to those four components
  first (or to `Instance.Destroy`/`StopController` directly) before an aggregate wait at the
  service or test level can be relied on to prove the goroutines are actually gone.
