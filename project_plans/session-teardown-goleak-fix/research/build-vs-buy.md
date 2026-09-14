# Build vs. Buy: bounded wait for N session-component goroutines in a test

## Question

The fix needs to block `TestSessionService_CreateThenImmediateDelete_NoDataRace`
(or its shared teardown helper) until ~5-6 session component goroutines have
actually exited, with a bounded timeout, before the test returns. Is there any
justification for reaching outside the standard library / already-vendored
deps for this?

## What's already available

- **`sync.WaitGroup` + `time.After`/`context.WithTimeout`** — stdlib, zero cost.
- **`golang.org/x/sync/errgroup`** — already a direct dependency
  (`go.mod:207`, `golang.org/x/sync v0.21.0`; resolved in `go.sum:449-450`).
  Not currently imported for this kind of teardown-join anywhere in
  `session/` or `server/` (spot-checked via grep on `errgroup` in module
  root — no hits outside `go.sum`/`go.mod`), so adopting it here would be a
  first use of the package for this purpose, not an extension of an existing
  pattern.
- **`go.uber.org/goleak` v1.3.0** (`go.mod:55`) — already a dependency, and
  it's the library the *target* test (`TestServer_Shutdown_JoinsBackgroundTickers`)
  uses to detect the leak in the first place. It is a leak-detector, not a
  wait-for-exit primitive — it can assert "no unexpected goroutines exist
  right now," but has no API for "block until goroutine X's shutdown work
  has finished." It's the symptom-detector here, not a candidate fix.
- **Existing in-repo convention**: `Stopped() <-chan struct{}` — a field
  `stopped chan struct{}` closed via `defer close(w.stopped)` at the top of
  the goroutine's run loop, exposed through a `Stopped()` accessor
  (`session/history_watcher.go:73-76`, mirrored in
  `session/detection/plugin_watcher.go:43` and
  `server/services/claude_settings_watcher.go:320`). This is exactly
  stdlib channels — no library involved.

## Comparing the options

**(a) Stdlib `Stopped() <-chan struct{}` per component + a small join helper.**
Each of the 5-6 components (`ResponseStream`, `CommandExecutor`,
`ClaudeController`, `PTYConsumer`, `MangleCorrelator`'s eviction goroutine,
and the diagnostic `RestoreWithWorkDir` `Cmd.Wait` goroutine) already runs a
single dedicated goroutine per responsibility, so each one fits the existing
one-channel-per-watcher pattern directly — no new primitive needed, just
exposing `Stopped()` (or reusing an existing one) on each and `select`-ing
across all of them with a `time.After` bound, or funneling them into one
`sync.WaitGroup.Wait()` guarded by a timeout goroutine. Either sub-approach is
10-20 lines, matches existing style byte-for-byte, and requires zero new
imports.

**(b) `errgroup.Group`.** Built for waiting on N goroutines you *launch* and
propagating the first error / supporting cancellation — not for waiting on
goroutines that already exist and are owned by other components with their
own lifecycles. Using it here would mean wrapping each `Stopped()` channel in
an adapter goroutine just to feed `errgroup`, which is more code and an extra
goroutine hop than just `select`ing on the channels or a `WaitGroup`
directly. It's available (already vendored), but it doesn't fit this
particular shape of problem — waiting on existing shutdown signals, not
orchestrating new work.

**(c) A third-party "await multiple conditions with timeout" library.**
Searched for anything already vendored that does this (e.g. via `go.sum` for
condition/barrier libraries) — nothing found, and there is no gap that would
justify adding one. The problem — join N already-existing done-channels with
a bound — is exactly what `select` + `time.After` (or `sync.WaitGroup` +
a timeout goroutine) does in the small number of lines this needs. No
third-party library adds meaningful value over ~15 lines of stdlib for a
fixed, small N known at the call site.

## Verdict

| Option | Verdict |
|---|---|
| (a) Stdlib channels, following the repo's existing `Stopped()` pattern | **Recommended** |
| (b) `errgroup.Group` (already a dependency) | **Not recommended** — wrong shape for joining pre-existing shutdown signals; would add an adapter layer with no benefit over (a) |
| (c) New third-party dependency | **Not recommended** — no gap stdlib doesn't already close; this is a test-infra fix in a codebase whose engineering discipline (`.claude/rules/prefer-go-git-over-subshells.md`, the interface-pollution checklist) already favors minimal, idiomatic Go over added abstraction/dependencies |

This is squarely a "few lines of stdlib" problem. The repo already has a
working, three-times-precedented convention for exactly this need
(`Stopped() <-chan struct{}`); the correct move is to apply it to the
remaining components that lack it (or aggregate their existing
signals), not to introduce `errgroup` or any external package.
