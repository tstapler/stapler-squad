# BUG-074: `TmuxAlive()` can read a stale control-mode registry entry immediately after `KillSession()` [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-14
**Impact**: `session/session_restart_test.go`'s `TestSessionRestartWithConversationContinuity/HealthCheckerAutoRestart` (and two others sharing the same kill-then-check pattern) flake in CI with `assert.False(t, instance.TmuxAlive(), "Tmux session should be dead after kill")` failing — `TmuxAlive()` returns `true` immediately after `KillSession()` returned `nil`. Any production caller that kills a session and immediately re-checks liveness (health-check auto-restart, restart cycles) is subject to the same race, not just the tests.

## Problem Description

`Instance.KillSession()` (`session/instance_tmux.go:399-406`) calls `i.pm().Close()`, which for the tmux backend resolves to `TmuxProcessManager.Close()` (`session/tmux_process_manager.go:102-111`) → `TmuxSession.Close()` (`session/tmux/tmux.go:1891-1945`). `Close()` synchronously runs `kill-session` and then calls `t.invalidateExistsCache()` (`session/tmux/tmux.go:1922`) before returning `nil` to the caller.

`invalidateExistsCache()` (`session/tmux/tmux.go:2152-2155`) only resets `t.existsCache` — the local TTL cache. It does **not** touch `t.registry`, the push-based `TmuxServerRegistry` fed by tmux control-mode `%session-closed` notifications (`session/tmux/server_registry.go`). `TmuxAlive()` (`session/instance_tmux.go:530-535`) calls `pm().IsAlive()` → `TmuxProcessManager.IsAlive()` (`session/tmux_process_manager.go:81-87`) → `TmuxSession.DoesSessionExist()` (`session/tmux/tmux.go:2091-2135`), whose **first** check is:

```go
if t.registry != nil && t.registry.IsHealthy() {
    if t.registry.SessionExists(t.sanitizedName) {
        return true
    }
    // Registry returned false — do not trust it blindly; fall through.
}
```
(`session/tmux/tmux.go:2100-2105`)

`TmuxServerRegistry.SessionExists` (`session/tmux/server_registry.go:138-142`) is a plain `RLock`-guarded map read with no relation to the synchronous `kill-session` subprocess `Close()` just ran — it only updates when the registry's own control-mode reconnect/event-processing goroutine receives and processes the `%session-closed` notification for that session name. That delivery is asynchronous and has no guaranteed ordering with respect to `Close()`'s return. So the sequence in the failing tests is:

1. `KillSession()` synchronously runs `tmux kill-session` (succeeds) and invalidates only the local TTL cache.
2. `KillSession()` returns `nil` to the test.
3. The registry's control-mode event goroutine has not yet processed `%session-closed` for this session — `r.sessions[name]` is still `true`.
4. `TmuxAlive()` → `DoesSessionExist()` hits the registry fast path first, sees `SessionExists(name) == true`, and returns `true` without ever falling through to the (now-correctly-invalidated) TTL cache or a fresh subprocess check.

This is a genuine TOCTOU race between a synchronous kill (which invalidates the wrong cache) and an asynchronous push-based registry (which isn't invalidated at all), not a symptom of any change in PR #503 — `session_restart_test.go` is untouched by that PR (confirmed via `gh pr diff 503 --repo tstapler/stapler-squad --name-only`).

`TmuxSession.DoesSessionExistNoCache()` (`session/tmux/tmux.go:2168-2206`) is unaffected — it has no registry fast path and always does a fresh `list-sessions` subprocess check — which is why it's documented as the authoritative check for "critical validation before session creation."

## Confirmed Failure Sites (same root cause)

All three follow the identical `KillSession()` → immediate `TmuxAlive()` assertion pattern in `session/session_restart_test.go`:
- `TestSessionRestartWithConversationContinuity/HealthCheckerAutoRestart`, line 258 (`instance.KillSession()` at line 254)
- `TestClaudeCommandBuilderIntegration/MultipleRestartCycles` (`testMultipleRestartCycles`), line 488 (`instance.KillSession()` at line 484)
- `TestClaudeCommandBuilderIntegration/SessionDataPersistence` (`testSessionDataPersistence`) uses the same `KillSession()` call at line 556, though without an immediately-following `TmuxAlive()` assertion in the excerpt reviewed — grep confirms the shared call site pattern; flakiness there likely stems from a later liveness re-check further down the same test.

## Fix Approach

`Close()` should invalidate (or synchronously update) the registry entry for the session it just killed, not just the TTL cache — e.g. have `Close()` call a `registry.MarkSessionClosed(name)`/`Invalidate(name)` method after a successful `kill-session`, so `SessionExists()` reflects the kill immediately rather than waiting on the async control-mode notification. Alternatively, `DoesSessionExist()` could fall through to the TTL-cache/subprocess path when the *session being checked* was locally killed within some short window (a "just closed this myself" flag on `TmuxSession`), without weakening the registry fast path for all other callers.

## Related Tasks

Found while investigating CI flakiness on PR #503 (`fix(services): eliminate deleteCleanupWG Add/Wait race, widen cleanup timeout`) per `.claude/rules/fix-flaky-tests-dont-defer.md`. `session_restart_test.go` was not modified by that PR, so this is filed as a standalone bug rather than fixed inline — the fix requires threading state between `TmuxSession.Close()` and `TmuxServerRegistry`, which is shared infrastructure well beyond that PR's scope.
