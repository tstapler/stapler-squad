# Requirements: tmux-ptmx-race-fix

## Source

Backlog item `c42de545-ee23-420f-950b-d7635ab6ae27`: "bug: data race on TmuxSession.ptmx
between GetPTY() and closePTYAndAttachCmd()". Filed per
`.claude/rules/fix-flaky-tests-dont-defer.md` after `go test -race` intermittently failed
`TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` in
`server/server_integration_test.go` with a real (not test-infra) data race, surfaced by
unrelated GitHub CLI-import work.

## Problem statement

`TmuxSession.ptmx` (`*os.File`, `session/tmux/tmux.go:61`) is read and written from many
methods with zero synchronization:

- Writers/nil-ers: `AttachToExisting` (tmux.go:864), `RestoreWithWorkDir` retry loop
  (tmux.go:1265), `closePTYAndAttachCmd` (tmux.go:1689-1696, both the `Close()` call and the
  `t.ptmx = nil` reset)
- Readers: `GetPTY` (tmux.go:1333-1336), `TapEnter`/`TapDAndEnter`/`SendKeys` (tmux.go:1309,
  1318, 1326), `Attach`'s two goroutines (`io.Copy(os.Stdout, t.ptmx)` at tmux.go:1484 and
  `t.ptmx.Write(buf[:nr])` at tmux.go:1544), `updateWindowSize`/resize path (tmux.go:1781,
  1786, 1800)

Confirmed race (from `-race` output in the item description):
- Read: `TmuxSession.GetPTY` (tmux.go:1333) via `TmuxProcessManager.GetPTY` →
  `TmuxBackend.GetPTY` → `Instance.GetPTYReader`, called from `SessionService.CreateSession`'s
  async controller-start goroutine.
- Write: `TmuxSession.closePTYAndAttachCmd` (tmux.go:1696, `t.ptmx = nil`) via
  `TmuxSession.Close()` → ... → `Instance.Destroy()`, called from `SessionService.DeleteSession`'s
  cleanup goroutine.

These two goroutines run concurrently when a caller deletes a session while its async
controller-start goroutine is still wiring up the PTY (a real, not merely theoretical,
interleaving — `CreateSession` starts controller wiring asynchronously and returns before it
completes).

## Why not fixed inline (context, not a requirement)

Guarding every access site (10+ call sites across `session/tmux/tmux.go`) is a larger blast
radius than the CLI-import change that surfaced it. This item exists to scope and fix it on
its own.

## Non-goals

- Not fixing every latent race in `session/tmux/tmux.go` — scope is strictly `t.ptmx` and the
  fields that must move in lockstep with it for correctness (`t.attachCmd`,
  `t.attachCmdWaitOnce`, all reassigned together at every write site above).
  `t.attachCmd`/`t.attachCmdWaitOnce` races are explicitly in scope because they are written
  under the exact same critical sections as `t.ptmx` at every call site (tmux.go:864-866,
  1265-1268, 1689-1696) — guarding `t.ptmx` alone while leaving these two unguarded would just
  relocate the same class of race one field over.
- No behavior change to PTY lifecycle semantics (retry logic, EIO handling, orphan-process
  killing) — this is a concurrency-safety fix, not a functional change.
- No change to the `detachMutex`, `controlModeSubMu`, `controlModeStartMu`, `cmdSendMu`, or
  `recoveryMu` locks already in the file — new/reused locking must not create a new deadlock
  ordering against these.

## Acceptance criteria

1. `t.ptmx`, `t.attachCmd`, and `t.attachCmdWaitOnce` are never read or written outside a
   consistent synchronization mechanism (mutex or equivalent) in `session/tmux/tmux.go`.
2. `go test -race ./session/... ./server/...` passes with no data race reported on these
   fields, run at least 10x consecutively
   (`go test -race ./session/... ./server/... -count=10`) to catch intermittency.
3. `go test -race ./server/... -run TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort -count=20`
   (the originally-flaky test) passes cleanly, including under concurrent load matching the
   original repro conditions (concurrent `CreateSession`/`DeleteSession`).
4. No new deadlocks introduced: existing `session/tmux` test suite
   (`go test -race ./session/tmux/...`) passes, and lock acquisition order is documented if a
   new mutex is added alongside existing ones (`detachMutex`, `controlModeSubMu`, etc.) that
   could nest.
5. `make quick-check` passes (build + test + lint) with no regressions attributable to this
   change.
6. Fix does not alter observable PTY lifecycle behavior (existing retry/backoff, EIO
   handling, orphan-process cleanup in `closePTYAndAttachCmd`) — verified by existing
   `session/tmux` tests continuing to pass unmodified in intent (only synchronization is
   added).
