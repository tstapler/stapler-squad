# BUG-074: `Close()`'s kill-session call bypassed the injected executor, leaving mock-backed liveness checks stale [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-08-14
**Fixed**: 2026-08-15
**Impact**: `session/session_restart_test.go`'s `TestSessionRestartWithConversationContinuity/HealthCheckerAutoRestart`, `TestClaudeCommandBuilderIntegration/MultipleRestartCycles`, and `TestClaudeCommandBuilderIntegration/SessionDataPersistence` flaked in CI with `assert.False(t, instance.TmuxAlive(), "Tmux session should be dead after kill")` failing — `TmuxAlive()` returned `true` immediately after `KillSession()` returned `nil`.

## Actual Root Cause

All three failing tests build their instance via `NewTestInstance(t, ...).Build()`, which resolves to `buildWithMockTmux()` (`session/comprehensive_session_creation_test.go:212-227`). That helper injects a fully in-memory `mockTmuxExecutor` (`session/comprehensive_session_creation_test.go:26-129`) as the session's `t.cmdExec` — it never shells out to a real tmux binary; it just tracks session names in an in-memory `sessionsCreated` map, adding on `new-session` and deleting on `kill-session`.

`TmuxSession.Close()` (`session/tmux/tmux.go:1919-1983`, as it stood before this fix) correctly used `t.cmdExec` for every other operation, but its kill-session subprocess call did not:

```go
killExec := executor.MakeTimeoutExecutor(killSessionTimeout)
gatedErr := runGatedErr(context.Background(), t.serverSocket, func() error {
    return killExec.Run(cmd)
})
```

`executor.MakeTimeoutExecutor` constructs a brand-new `TimeoutExecutor` that always executes a real OS subprocess (`executor/timeout_executor.go`'s `Run`/`Output`/`CombinedOutput` build and run `exec.CommandContext` directly; the struct's `delegate` field is declared but never read). It has no relationship to whatever executor was injected into the `TmuxSession` via `NewTmuxSessionWithDeps`.

So in a mock-backed test:

1. `KillSession()` → `Close()` builds a `kill-session` command and runs it through `killExec` — a REAL executor — against `t.serverSocket`, a socket with no real tmux server behind it. This fails (exit code 1 / "no such file or directory"), which `Close()`'s error handling treats as "already killed," logs, and swallows.
2. The mock's `Run()` — the thing that actually deletes the session name from `sessionsCreated` — is never invoked, because the kill-session call went through `killExec`, not `t.cmdExec`.
3. `Close()` returns `nil` (the swallowed real-executor error looks like "already gone").
4. `TmuxAlive()` → `DoesSessionExist()` → `t.cmdExec` (still the mock) reports the session present, because its in-memory map was never updated.
5. `assert.False(t, instance.TmuxAlive(), ...)` fails.

This diagnosis **supersedes** the bug's originally-filed theory (an async push-based `TmuxServerRegistry` race between `Close()`'s synchronous cache invalidation and a `%session-closed` control-mode event). That theory doesn't apply to these three tests at all: `buildWithMockTmux()` never touches a real tmux control-mode connection or the live `TmuxServerRegistry` — the entire failure is local to the mock executor being bypassed for one call.

## Fix

`TmuxSession.Close()` (`session/tmux/tmux.go`) now binds the kill-session command to a `killSessionTimeout`-bound `context.Context` (via a new `buildTmuxCommandContext(ctx, args...)` helper factored out of `buildTmuxCommand`) and runs it through `t.cmdExec.Run(cmd)` — the same injectable executor every other tmux operation in the file already uses — instead of constructing a separate, always-real `executor.MakeTimeoutExecutor`. This preserves the original 5s timeout protection (previously the whole reason a `TimeoutExecutor` was introduced here) while making the call properly respect dependency injection.

Verified via `go test -run TestSessionRestartWithConversationContinuity -count=8 ./session/...` and `go test -run 'TestClaudeCommandBuilderIntegration/MultipleRestartCycles|TestClaudeCommandBuilderIntegration/SessionDataPersistence' -count=8 ./session/...` — all reps pass.

## Related Tasks

Originally found while investigating CI flakiness on PR #503 per `.claude/rules/fix-flaky-tests-dont-defer.md`; re-investigated and root-caused correctly in a later session after the registry-race theory failed to explain why the mock-backed tests (which never touch a real registry) were affected.
