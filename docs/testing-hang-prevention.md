# Testing Hang Prevention and Diagnostics

## Overview

This document describes the comprehensive hang prevention infrastructure implemented in Stapler Squad's testing harness to prevent and diagnose test hangs.

## Problem Statement

The original testing infrastructure had several issues:
1. **Mocked tmux backend** - Couldn't detect real-world hangs
2. **No timeout protection** - External commands could block indefinitely
3. **Polling loops** - `DoesSessionExist()` could hang waiting for tmux
4. **Cleanup hangs** - Test cleanup could block on PTY/console operations

## Solutions Implemented

### 1. Timeout Executor (Story 1)

**Location**: `executor/timeout_executor.go`

**Purpose**: Prevents any command execution from hanging indefinitely.

**Key Features**:
- Context-based cancellation with configurable timeout (default: 5s)
- Automatic process cleanup on timeout
- Clear error messages indicating which command timed out
- Works for `Run()`, `Output()`, and `OutputWithPipes()` operations

**Example**:
```go
executor := executor.NewTimeoutExecutor(5 * time.Second, executor.MakeExecutor())
cmd := exec.Command("slow-command")
err := executor.Run(cmd) // Times out after 5 seconds
```

**Integration**:
- Config initialization uses `timeoutCommandExecutor` wrapping
- Prevents `which claude` and other external commands from blocking
- All command execution throughout the application is protected

### 2. Isolated Tmux Testing (Story 2)

**Location**: `testutil/tmux.go`

**Purpose**: Real tmux integration testing without interference between tests.

**Key Features**:
- Unique tmux socket per test server using `-L` flag
- Atomic counter ensures no socket name collisions
- Automatic cleanup via `t.Cleanup()`
- Idempotent cleanup operations (safe to call multiple times)

**Example**:
```go
server := CreateIsolatedTmuxServer(t)
session, err := server.CreateSession("test-session", "sleep 5")
// Automatic cleanup when test completes
```

**Hang Prevention**:
- Tests can't interfere with each other's tmux sessions
- Each test gets a clean tmux environment
- Failed cleanup doesn't block other tests

### 3. Expect-Based TUI Testing (Story 3)

**Location**: `testutil/expect.go`

**Purpose**: Interactive TUI testing with real terminal sessions.

**Key Features**:
- PTY-based terminal allocation using `github.com/creack/pty`
- Expect pattern matching using `github.com/Netflix/go-expect`
- Automatic process cleanup via `t.Cleanup()`
- Forceful process termination to prevent cleanup hangs

**Example**:
```go
session, err := StartExpectSession(t, DefaultExpectConfig())
session.SendKeys("n")                      // Navigate to new session
session.ExpectString("Session name:", 2*time.Second)
session.Close()                            // Immediate process kill
```

**Hang Prevention**:
- `Close()` kills process immediately (no graceful shutdown in tests)
- PTY read operations have timeouts
- Console operations are non-blocking
- Test cleanup cannot hang indefinitely

### 4. Test Timeout Configuration

All tests have explicit timeouts configured:

```go
go test ./testutil -timeout=30s  // Short tests
go test ./testutil -timeout=60s  // Medium tests
go test ./testutil -timeout=90s  // Long tests
```

**Benefits**:
- Tests fail fast instead of hanging indefinitely
- Clear indication when a test is hanging
- Prevents CI/CD pipeline hangs

## Diagnostic Tools

### Test Timing Logs

All tests log timing information:

```go
startTime := time.Now()
// ... perform operation ...
elapsed := time.Since(startTime)
t.Logf("Operation took %v", elapsed)
```

### Hang Detection Patterns

Tests validate operations complete within reasonable time:

```go
startTime := time.Now()
err := waiter.WaitForSessionExists()
elapsed := time.Since(startTime)
assert.Less(t, elapsed, 10*time.Second, "Should not hang")
```

### Error Message Clarity

Timeout errors clearly indicate what failed:

```
timeout waiting for "expected string": context deadline exceeded
session never became available: timeout after 5s
TUI did not exit after 3s
```

## Best Practices for Writing Non-Hanging Tests

### 1. Always Use Timeouts

```go
// ✅ Good - explicit timeout
config := WaitConfig{
    Timeout: 5 * time.Second,
    PollInterval: 100 * time.Millisecond,
}

// ❌ Bad - no timeout protection
for {
    if condition() { break }
    time.Sleep(100 * time.Millisecond)
}
```

### 2. Use Isolated Test Resources

```go
// ✅ Good - isolated tmux server
server := CreateIsolatedTmuxServer(t)

// ❌ Bad - shared default tmux server
// Tests can interfere with each other
```

### 3. Implement Proper Cleanup

```go
// ✅ Good - automatic cleanup
t.Cleanup(func() {
    session.Close()
})

// ❌ Bad - manual cleanup that might not run
defer session.Close() // Might not execute if test panics
```

### 4. Validate Timing Expectations

```go
// ✅ Good - measure and validate timing
startTime := time.Now()
operation()
elapsed := time.Since(startTime)
assert.Less(t, elapsed, expectedMax)

// ❌ Bad - no timing validation
operation() // How do we know it didn't hang?
```

### 5. Use Context-Based Cancellation

```go
// ✅ Good - context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// ❌ Bad - no cancellation mechanism
```

## Debugging Hanging Tests

### 1. Check Test Logs

Test logs show timing information:
```
Operation took 8.5s
Exit took 5.001s
Session became ready in 2.3s
```

### 2. Identify Blocking Operations

Common blocking operations:
- External command execution (e.g., `which`, `tmux list-sessions`)
- PTY read operations without timeouts
- Console write operations to closed PTY
- Waiting for processes that never exit

### 3. Add Diagnostic Logging

```go
t.Logf("Before potentially blocking operation")
err := potentiallyBlockingOp()
t.Logf("After operation: %v", err)
```

### 4. Use Shorter Timeouts During Development

```go
// Development: Short timeout to detect hangs quickly
config.Timeout = 2 * time.Second

// Production: Longer timeout for reliability
config.Timeout = 10 * time.Second
```

### 5. Check goroutine Stack Traces

When a test hangs, Go's test runner prints goroutine stack traces showing where each goroutine is blocked:

```
goroutine 36 [IO wait]:
internal/poll.(*FD).Read(...)
os.(*File).Read(...)
github.com/Netflix/go-expect.(*Console).ExpectString(...)
```

## Test Coverage

### Timeout Protection Tests
- `executor/timeout_executor_test.go` - 16 tests
- Validates timeout behavior, process cleanup, error messages

### Isolated Tmux Tests
- `testutil/tmux_test.go` - 10 tests
- `testutil/tmux_integration_test.go` - 5 test suites
- `testutil/tmux_polling_test.go` - 3 test suites
- Validates isolation, creation, lifecycle, polling

### TUI Interaction Tests
- `testutil/expect_test.go` - 5 tests
- `testutil/tui_session_creation_test.go` - 2 test suites
- `testutil/tui_exit_test.go` - 3 test suites
- Validates TUI startup, navigation, exit, hang prevention

## Performance Characteristics

### Test Execution Times

```
Short tests (infrastructure):  0.5s - 2s
Medium tests (TUI operations): 3s - 10s
Long tests (lifecycle):        10s - 20s
```

### Timeout Recommendations

| Operation Type | Recommended Timeout |
|----------------|---------------------|
| Command execution | 5s |
| Session creation | 10s |
| TUI operations | 10-15s |
| Full lifecycle tests | 30s |
| Stress tests | 60s+ |

## Continuous Integration

### CI/CD Configuration

```yaml
# GitHub Actions example
- name: Run Tests
  run: go test ./... -timeout=120s -v
  timeout-minutes: 5
```

### Monitoring for Hangs

1. **Test timeout**: Go test runner timeout
2. **CI job timeout**: GitHub Actions/CI timeout
3. **Both should be configured** to catch different hang scenarios

## Summary

The testing infrastructure prevents hangs through:

1. ✅ **Timeout executor** - All commands have timeout protection
2. ✅ **Isolated resources** - Tests don't interfere with each other
3. ✅ **Explicit timeouts** - Every test has a timeout configured
4. ✅ **Forceful cleanup** - Cleanup operations don't block
5. ✅ **Diagnostic logging** - Timing information for debugging
6. ✅ **Clear error messages** - Easy to identify what timed out

## References

- Timeout Executor Implementation: `executor/timeout_executor.go`
- Isolated Tmux Infrastructure: `testutil/tmux.go`
- Expect Testing Framework: `testutil/expect.go`
- Task Breakdown: `docs/tasks/testing-harness-real-tmux.md`
