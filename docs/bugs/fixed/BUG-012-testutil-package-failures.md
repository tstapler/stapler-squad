# BUG-012: Testutil Package Failures [SEVERITY: Medium]

**Status**: ✅ Fixed (2026-04-20)
**Discovered**: 2025-12-05 during test stabilization work
**Impact**: Test infrastructure broken, blocks test development

## Problem Description

The `testutil` package has 29 failing tests across `tmux_integration_test.go`, `tmux_polling_test.go`, and `tmux_test.go`. All failures share the same error: `failed to start tmux session: error starting tmux session: exit status 1`.

**Root cause identified (2026-04-20)**: Stale tmux socket accumulation. Previous test runs leave hundreds of dead socket files in `/run/user/<uid>/` (or `/tmp/tmux-*/`). When the OS socket/file descriptor limit is reached, new tmux servers fail to start immediately with `server exited unexpectedly`. The isolation mechanism (`tmux -L <socket-name>`) is correct; the socket files simply never get cleaned up after test failures or crashes.

**Evidence**: `ls /run/user/$(id -u)/` shows 200+ stale test sockets: `test_TestRealTmuxSessionCreation_1`, `test_ensure_noop_*`, `test_keepalive_*`, `test_recovery_*`, etc.

**Implications**:
- Tests fail non-deterministically based on accumulated socket count
- Each failed cleanup (t.Cleanup or server.Cleanup) leaves a socket behind
- Tests pass individually (`-run TestTmuxTestServer_SessionExists` passes), fail in suite runs
- Stale sockets also consume file descriptors, eventually exhausting system limits

## Reproduction

```bash
# Run testutil package tests
go test ./testutil/... -v --timeout=60s

# Actual failures (29 failed):
# failed to start tmux session: error starting tmux session: exit status 1
# (across TestRealTmuxSessionCreation, TestRealTmuxSessionLifecycle,
#  TestRealTmuxMultipleServers, TestTmuxPollingDoesNotHang, etc.)

# Individual tests pass in isolation:
go test ./testutil/... -run TestTmuxTestServer_SessionExists -v  # PASSES

# Root cause: stale socket accumulation
ls /run/user/$(id -u)/  # Shows 200+ stale test_* sockets
```

**Expected**: All testutil tests pass
**Actual**: 29 tests fail with `exit status 1` from tmux server startup

## Root Cause Analysis

**Identified (2026-04-20)**: Stale tmux socket file accumulation.

### Mechanism

`CreateIsolatedTmuxServer` in `testutil/tmux.go` generates per-test socket names like `test_TestRealTmuxSessionCreation_1`. These socket files live in `/run/user/<uid>/` (or `/tmp/tmux-<uid>/` on some systems). When a test crashes or `t.Cleanup` fails before `Cleanup()` runs, the socket file persists.

After enough accumulated runs, the socket directory fills up with 200+ stale entries:
- `test_ensure_noop_*` (30+ entries)
- `test_ensure_start_*` (25+ entries)
- `test_exit_empty_*` (20+ entries)
- `test_keepalive_*` (30+ entries)
- `test_recovery_*` (30+ entries)
- ... and more

When a new test tries `tmux -L <socket-name> new-session`, tmux tries to create a new server. If the system's open file limit is approaching, the tmux daemon starts and immediately exits, returning `exit status 1` with the message `server exited unexpectedly`.

### Why Individual Tests Pass

Individual tests succeed because they acquire a socket before the limit is hit or use a lower counter that doesn't collide. The full suite runs all tests in parallel, simultaneously trying to create many tmux servers, guaranteeing failures.

### Files Affected

- `testutil/tmux.go` - `TmuxTestServer.Cleanup()` and `CreateIsolatedTmuxServer` (lines ~155-195)
- `testutil/tmux.go` - `KillServer()` method (line ~285): server kill may leave socket file even after killing tmux process

## Files Affected (3 files)

- `testutil/tmux.go` - `TmuxTestServer.Cleanup()` and `KillServer()` must remove socket files
- `testutil/tmux.go` - `sanitizeTestName()` — long socket names may exceed OS limits on some distros
- `testutil/tmux_integration_test.go`, `testutil/tmux_polling_test.go`, `testutil/tmux_test.go` — 29 failing tests (consumers, not root cause)

**Context boundary**: 1-2 files, well within scope.

## Fix Approach

### Immediate: Clean stale sockets (1 minute, manual)

```bash
# Clean all stale test sockets to unblock test runs immediately
ls /run/user/$(id -u)/test_* | wc -l   # Count stale sockets
rm /run/user/$(id -u)/test_*           # Remove all test sockets
go test ./testutil/... --timeout=60s   # Should now pass
```

### Permanent fix: Socket file cleanup in `TmuxTestServer.Cleanup()` (2h, Small task)

In `testutil/tmux.go`, `KillServer()` method currently runs `tmux -L <socket> kill-server` but does not remove the socket file afterwards. Add OS-level socket removal:

```go
func (s *TmuxTestServer) KillServer() error {
    // ... existing kill-server command ...

    // Remove the socket file to prevent stale accumulation
    socketPath := tmuxSocketPath(s.socketName)
    if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
        s.t.Logf("Warning: could not remove socket file %s: %v", socketPath, err)
    }
    return nil
}

// tmuxSocketPath returns the filesystem path for a named tmux socket.
// Tmux uses /run/user/<uid>/<name> on systemd systems, /tmp/tmux-<uid>/<name> on others.
func tmuxSocketPath(socketName string) string {
    if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
        return filepath.Join(dir, socketName)
    }
    return filepath.Join(fmt.Sprintf("/tmp/tmux-%d", os.Getuid()), socketName)
}
```

Additionally truncate socket names longer than 60 chars in `sanitizeTestName()` to stay well under OS path limits.

## Verification

```bash
# Before fix: count stale sockets
ls /run/user/$(id -u)/test_* 2>/dev/null | wc -l

# Apply fix and run tests
go test ./testutil/... --timeout=60s

# After multiple runs: verify sockets are cleaned up
ls /run/user/$(id -u)/test_* 2>/dev/null | wc -l  # Should stay near 0
```

## Expected Fix Outcomes

After investigation and fixes:
- All testutil tests pass ✅
- Test helpers work correctly ✅
- Mocks match current interfaces ✅
- Fixtures have valid data ✅
- No resource leaks in test utilities ✅
- Other test packages can use utilities safely ✅

## Impact Assessment

**Severity**: **Medium**
- **User-Facing**: No (test infrastructure only)
- **Data Loss**: No
- **Workaround**: Don't use broken utilities (hard to know which)
- **Frequency**: Every test run using testutil
- **Scope**: Test infrastructure, affects all test development

**Priority**: P2 - Important for test development, doesn't affect production

**Timeline**:
- Phase 1 (Capture output): 15 minutes
- Phase 2 (Identify broken): 30 minutes
- Phase 3 (Check impact): 30 minutes
- Phase 4 (Fix): 2-4 hours
- Phase 5 (Verify): 1 hour
- **Total**: 4-6 hours

## Prevention Strategy

**Test infrastructure maintenance**:
1. Test the test utilities (meta-testing)
2. Run testutil tests in CI
3. Update mocks when interfaces change
4. Regenerate fixtures when schemas change
5. Document utility usage and assumptions

**Code generation**:
```bash
# Use go generate for mocks
//go:generate mockgen -source=session.go -destination=testutil/mock_session.go

# Run before tests in CI
go generate ./...
go test ./...
```

**Test isolation**:
```go
// Use t.Cleanup() for proper teardown
func TestSomething(t *testing.T) {
    helper := testutil.NewHelper(t)
    t.Cleanup(func() {
        helper.Cleanup()
    })
    // Test code
}
```

**Fixture validation**:
```go
// Add schema validation for fixtures
func LoadFixture(path string) (*Fixture, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var f Fixture
    if err := json.Unmarshal(data, &f); err != nil {
        return nil, err
    }

    // Validate against current schema
    if err := f.Validate(); err != nil {
        return nil, fmt.Errorf("fixture %s invalid: %w", path, err)
    }

    return &f, nil
}
```

## Related Issues

- **BUG-008**: Category rendering in tests (CRITICAL, open) - May be using broken testutil
- **BUG-009**: Session package test failures (high, open) - May be due to broken mocks
- **BUG-010**: tmux banner detection (high, open) - May be in testutil/tmux helpers
- **Test Stabilization Epic**: See `docs/tasks/test-stabilization-and-teatest-integration.md`

## Additional Notes

**Why this matters**:

Broken test infrastructure is **more dangerous than broken tests**:

1. **False confidence**: Tests pass but are using broken utilities
2. **Hidden bugs**: Utilities mask real issues in production code
3. **Cascading failures**: One broken utility breaks many tests
4. **Test debt accumulation**: Developers work around broken utilities

**Priority justification**:

This is **P2 (not P1)** because:
- Doesn't directly affect production code
- Other tests may not depend on broken utilities
- Can work around by not using testutil

But should be **fixed before adding new tests** to avoid building on broken foundation.

**Recommendation**:

1. **Investigate quickly** (1 hour) to assess impact
2. **Fix critical utilities first** (those blocking other tests)
3. **File separate bugs** for each broken utility found
4. **Update fixtures** as part of fix (don't defer)
5. **Add meta-tests** to catch future breakage

**Don't ignore test infrastructure problems** - They compound exponentially.

---

**Bug Tracking ID**: BUG-012
**Related Feature**: Test Infrastructure (testutil/ package)
**Fix Complexity**: Medium (multiple utilities, mocks, fixtures)
**Fix Risk**: Low-Medium (test code only, but affects all tests)
**Blocked By**: Investigation needed (Phase 1-3)
**Blocks**: Test development, may contribute to other test failures
**Related To**: All test-related bugs (BUG-008, BUG-009, BUG-010, BUG-011)
