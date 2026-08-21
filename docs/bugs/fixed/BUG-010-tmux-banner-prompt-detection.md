# BUG-010: Tmux Test Failures — Global Registry Contamination [SEVERITY: High]

**Status**: ✅ Fixed (2026-04-20)
**Discovered**: 2025-12-05 during test stabilization work
**Fixed**: 2026-04-20 — commit `250a0bf`
**Impact**: 8 tests in `session/tmux` failed when run as part of the full suite

## Problem Description

Eight tests in `session/tmux` passed in isolation but failed when run as part of the full test
suite. The original hypothesis (shell banners, ANSI codes, prompt regex mismatch) was incorrect.
Investigation revealed two distinct root causes, both related to the global `TmuxServerRegistry`
leaking real state into mock-based tests.

**Failing tests** (actual):
1. `TestSessionResumption/ExistingSession_ShouldReuse_NotFail`
2. `TestSessionResumption/ExistingSession_WithCleanup_ShouldSetupProperly`
3. `TestSessionResumption/NewSession_ShouldCreateNormally`
4. `TestSessionResumptionBehaviorComparison/NEW_Behavior_Succeeds`
5. `TestSessionResumptionIntegration`
6. `TestStartTmuxSession`
7. (+ 2 parent suite entries)

**Actual error** (from JSON test output):
```
timed out waiting for tmux session staplersquad_<name>: <nil>
```

The mock executor returned the session as existing, but `DoesSessionExist()` was routing through
the real global `TmuxServerRegistry` (which had no knowledge of mock sessions), so `Start()`
timed out waiting.

## Root Causes

### Root Cause 1: Global registry bypassed mock executor

`DoesSessionExist()` has a fast path: when `t.registry != nil && t.registry.IsHealthy()`, it
queries the in-memory registry map instead of calling the mock's `CombinedOutput`. When another
test in the suite started first and warmed up the global registry for the default socket, all
subsequent mock tests routed through real tmux — finding nothing — and timed out.

**Fix**: All mock-based test constructors now pass `WithRegistry(nil)` to force the fallback path:

```go
// Before
session := newTmuxSession("test-session", "echo", ptyFactory, cmdExec, TmuxPrefix)

// After
session := newTmuxSessionWithSocket("test-session", "echo", ptyFactory, cmdExec, TmuxPrefix, "", WithRegistry(nil))
```

Files changed:
- `session/tmux/tmux_test.go`: `TestStartTmuxSession`
- `session/tmux/session_resumption_test.go`: 5 test helpers

### Root Cause 2: Keepalive session injected into isolated test servers

`startControlMode()` unconditionally created `staplersquad_keepalive` on every tmux server it
connected to — including isolated test servers started with `-L <socket>`. This caused
`TmuxTestServer.ListSessions()` to return 3 sessions instead of 2, breaking session-count
assertions in integration tests.

**Fix** in `session/tmux/server_registry.go`:

```go
// Only create the keepalive sentinel on the default server (empty socket).
if r.serverSocket == "" {
    createArgs := []string{"new-session", "-d", "-s", keepaliveName}
    _ = exec.Command("tmux", createArgs...).Run()
}
```

## Original Hypothesis (Incorrect)

The original doc theorized failures were due to:
- Shell initialization banners (MOTD, .bashrc output)
- Prompt regex mismatches across shells (bash/zsh/fish)
- ANSI escape code interference
- PTY configuration issues

None of these were present in the actual failing tests. The tests use mock PTY factories and
mock command executors — no real shell is involved. The failures were purely a test isolation
issue.

## Verification

After all fixes:

```
session/tmux: 173/173 passing
testutil:      52/52  passing
```

The 12 pre-existing failures in `session/` (TestComprehensiveSessionCreation,
TestSessionRestartWithConversationContinuity) were confirmed pre-existing via `git stash` and are
unrelated to this bug.

## Impact Assessment

**Severity**: High (before fix)
- **User-Facing**: No direct user impact; affected test reliability only
- **Data Loss**: No
- **Frequency**: Every full suite run

## Prevention

- Mock-based tests that construct `TmuxSession` directly must always pass `WithRegistry(nil)`.
- Integration tests that need a real registry should use `testutil.CreateIsolatedTmuxServer()`.
- The keepalive guard (`serverSocket == ""`) ensures production-only behavior stays out of
  isolated test environments.

## Related

- **BUG-012**: Stale tmux socket accumulation (fixed same session, commit `7a7bd5b`)
- **commit 250a0bf**: "fix(tmux): prevent global registry from contaminating isolated test servers"
