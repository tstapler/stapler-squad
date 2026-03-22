# Circuit Breaker for Git/Tmux Subprocess Calls

## Epic Overview

### Problem Statement

Stapler Squad executes `git` and `tmux` subprocesses on nearly every API request cycle — `git diff`, `git status`, `tmux capture-pane`, `tmux list-sessions`, etc. The `TimeoutExecutor` provides a per-call timeout ceiling, but when an underlying resource is persistently degraded (locked git index, NFS stall, corrupted repository, tmux server overload), every request for that session still blocks for the full timeout duration before failing. With N concurrent API calls per session (WebSocket streaming, diff polling, status checks), a single degraded git worktree can consume N*timeout worth of goroutine-seconds every polling cycle.

A circuit breaker would detect repeated failures for a specific command class on a specific session and short-circuit subsequent calls immediately, returning a cached or error response without spawning the subprocess at all.

### User Value Statement

Protect Stapler Squad's responsiveness and resource usage when external subprocesses become persistently degraded, ensuring that one bad session does not drag down the entire server.

### Success Metrics

- **Fail-fast latency**: When circuit is OPEN, calls return in <1ms (no subprocess spawn)
- **Recovery detection**: Circuit transitions from OPEN to HALF-OPEN to CLOSED within 60s of underlying issue resolution
- **No false positives**: Transient single failures do not trip the breaker (threshold >= 3 consecutive)
- **Zero behavioral change in happy path**: No measurable latency increase when circuit is CLOSED

### Scope

- **In Scope**: `CircuitBreakerExecutor` implementing `Executor` interface, per-command-class keying, integration with git and tmux operations, circuit state observability via health endpoint, comprehensive unit tests
- **Out of Scope**: Automatic remediation of degraded resources, UI indicators for circuit state, persistent circuit state across server restarts, rate limiting

## Architecture Decisions

### ADR-001: Per-Command-Class + Per-Session Circuit Breakers (Not Global)

**Status**: Proposed

**Context**: The circuit breaker must be scoped so that a degraded `git diff` on Session A does not prevent `git diff` on Session B, and a degraded `git diff` does not prevent `tmux capture-pane` on the same session. Global breakers would cause cascading false failures across unrelated sessions.

**Decision**: Circuit breakers are keyed by a composite key of `(session-identifier, command-class)`. Command class is derived from the first argument of the subprocess (e.g., `"git-diff"`, `"git-status"`, `"tmux-list-sessions"`, `"tmux-capture-pane"`). Each unique key gets its own independent circuit breaker instance.

**Consequences**:
- Correct isolation: Session A's git failures do not affect Session B
- Correct granularity: git diff failures do not block tmux operations on the same session
- Memory overhead: One breaker per (session, command-class) pair; ~200 bytes per breaker, ~10 command classes per session, ~50 sessions = ~100KB total — negligible
- Breakers are created lazily on first use and garbage collected when sessions are destroyed

**Alternatives Considered**:
1. **Global per-command-class**: Single breaker for all `git diff` calls across all sessions
   - Rejected: One bad repo breaks diffs for all sessions
2. **Per-session only**: Single breaker per session covering all commands
   - Rejected: A git hang would also block tmux operations
3. **Per-command invocation**: Every unique command string gets a breaker
   - Rejected: Too fine-grained; `git diff abc123` and `git diff def456` should share a breaker

### ADR-002: Custom Implementation Over External Library (sony/gobreaker)

**Status**: Proposed

**Context**: `sony/gobreaker` is the most popular Go circuit breaker library (~4.5K stars). It provides a well-tested implementation with configurable thresholds and callbacks. However, our requirements are narrow: we need a simple consecutive-failure counter with three states, and the `Executor` interface requires wrapping `*exec.Cmd` (not arbitrary function calls).

**Decision**: Implement a custom circuit breaker within the `executor` package. The implementation is approximately 120 lines of code and avoids adding an external dependency for a pattern this simple.

**Rationale**:
- The `Executor` interface wraps `*exec.Cmd`, which requires extracting command-class keys from the cmd before execution — this integration logic would exist regardless of whether we use a library
- `sony/gobreaker` uses a sliding window with configurable ReadyToTrip function, which is more complex than our needs (simple consecutive failure count)
- No new dependency in `go.mod` for ~120 lines of straightforward code
- Full control over error classification (timeout vs exit-code-1 vs signal) and what counts as a "failure"
- The project already has zero circuit breaker dependencies; adding one for a single use creates maintenance burden

**Consequences**:
- We own the implementation and tests — no upstream breaking changes
- Slightly more code to maintain (~120 lines + ~200 lines tests)
- No sliding window or advanced features unless we add them

**Alternatives Considered**:
1. **sony/gobreaker**: Full-featured, well-tested
   - Rejected: Over-engineered for consecutive-failure-count with 3 states
   - Would still need adapter code to integrate with `Executor` interface
2. **rubyist/circuitbreaker**: Simpler API
   - Rejected: Less maintained, still an unnecessary dependency
3. **Wrap both**: Use library internally, expose `Executor`
   - Rejected: Dual abstraction layers for a 120-line pattern

### ADR-003: Consecutive Failure Counting (Not Sliding Window)

**Status**: Proposed

**Context**: Two common strategies exist for determining when to open a circuit: consecutive failure count and sliding-window error rate. Sliding windows require tracking timestamps of recent calls and computing ratios, adding complexity and memory.

**Decision**: Use consecutive failure counting. The circuit opens after `N` consecutive failures (default 3). Any single success resets the counter to zero and closes the circuit.

**Rationale**:
- Subprocess failures are typically modal — either the underlying issue is present (100% failure) or it is not (0% failure). Intermittent failures are rare for git/tmux operations.
- Simpler implementation with `atomic.Int64` counter, no time-windowed bookkeeping
- Easier to reason about: "3 failures in a row" is intuitive for operators
- Aligns with the failure patterns we're protecting against (locked index file, NFS mount, corrupted repo)

**Consequences**:
- A single transient failure followed by recovery will not trip the breaker (correct behavior)
- Three consecutive transient failures will trip the breaker even if the underlying issue resolves immediately — the 30s recovery timeout is the cost
- No protection against high error rates that are non-consecutive (e.g., 50% failures intermixed with successes) — acceptable for our use case

### ADR-004: Circuit State Observability via Health/Debug Endpoint

**Status**: Proposed

**Context**: When a circuit breaker trips, operators need visibility into which breakers are open and why. The project already has a `/health` endpoint and `/api/debug/escape-codes/*` debug endpoints.

**Decision**: Add a `/api/debug/circuit-breakers` endpoint that returns JSON listing all circuit breaker instances, their current state, failure counts, and last state transition time. This follows the existing debug endpoint pattern.

**Consequences**:
- Operators can diagnose "why is session X showing stale diffs" by checking circuit state
- No UI integration needed initially — raw JSON is sufficient for debugging
- Endpoint is behind existing auth middleware
- Minimal implementation cost: iterate breaker registry, serialize to JSON

## User Stories

### Story 1: Implement CircuitBreakerExecutor

**As a** developer integrating the circuit breaker
**I want** a `CircuitBreakerExecutor` that wraps any `Executor` and implements the circuit breaker pattern
**So that** persistently failing subprocess calls fail fast instead of blocking for the timeout duration

**Acceptance Criteria** (Given-When-Then):

1. **Given** a `CircuitBreakerExecutor` wrapping a `TimeoutExecutor`
   **When** the circuit is CLOSED and a command succeeds
   **Then** the result is returned normally with no additional latency

2. **Given** a `CircuitBreakerExecutor` in CLOSED state
   **When** 3 consecutive calls to the same command-class fail
   **Then** the circuit transitions to OPEN state

3. **Given** a `CircuitBreakerExecutor` in OPEN state
   **When** a call is made to the same command-class
   **Then** the call returns immediately with `ErrCircuitOpen` without executing the subprocess

4. **Given** a `CircuitBreakerExecutor` in OPEN state
   **When** the recovery timeout (30s) elapses
   **Then** the circuit transitions to HALF-OPEN and allows one probe call

5. **Given** a `CircuitBreakerExecutor` in HALF-OPEN state
   **When** the probe call succeeds
   **Then** the circuit transitions to CLOSED and resets the failure counter

6. **Given** a `CircuitBreakerExecutor` in HALF-OPEN state
   **When** the probe call fails
   **Then** the circuit transitions back to OPEN and restarts the recovery timeout

**Technical Design**:

```go
// executor/circuit_breaker.go

package executor

import (
    "fmt"
    "os/exec"
    "strings"
    "sync"
    "time"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
    CircuitClosed   CircuitState = iota // Normal operation
    CircuitOpen                         // Fail-fast mode
    CircuitHalfOpen                     // Probing for recovery
)

func (s CircuitState) String() string {
    switch s {
    case CircuitClosed:
        return "CLOSED"
    case CircuitOpen:
        return "OPEN"
    case CircuitHalfOpen:
        return "HALF-OPEN"
    default:
        return "UNKNOWN"
    }
}

// ErrCircuitOpen is returned when a call is rejected because the circuit is open.
var ErrCircuitOpen = fmt.Errorf("circuit breaker is open")

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
    FailureThreshold int           // Number of consecutive failures to trip the breaker
    RecoveryTimeout  time.Duration // Time to wait before probing in HALF-OPEN
}

// DefaultCircuitBreakerConfig returns the default configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
    return CircuitBreakerConfig{
        FailureThreshold: 3,
        RecoveryTimeout:  30 * time.Second,
    }
}

// circuitBreaker tracks the state for a single (session, command-class) pair.
type circuitBreaker struct {
    mu                sync.Mutex
    state             CircuitState
    consecutiveFailures int
    lastFailureTime   time.Time
    lastStateChange   time.Time
    config            CircuitBreakerConfig
}

// CircuitBreakerExecutor wraps an Executor with per-command-class circuit breakers.
type CircuitBreakerExecutor struct {
    delegate Executor
    config   CircuitBreakerConfig

    mu       sync.RWMutex
    breakers map[string]*circuitBreaker // keyed by command-class
}

// NewCircuitBreakerExecutor creates a new circuit breaker executor wrapping the delegate.
func NewCircuitBreakerExecutor(delegate Executor, config CircuitBreakerConfig) *CircuitBreakerExecutor {
    return &CircuitBreakerExecutor{
        delegate: delegate,
        config:   config,
        breakers: make(map[string]*circuitBreaker),
    }
}
```

**Command-class extraction**:

```go
// commandClass extracts a stable key from the command for circuit breaker grouping.
// Examples: "git diff ..." -> "git-diff", "tmux capture-pane ..." -> "tmux-capture-pane"
func commandClass(cmd *exec.Cmd) string {
    if cmd == nil || len(cmd.Args) == 0 {
        return "unknown"
    }
    // Use the binary name and first subcommand/flag
    parts := []string{filepath.Base(cmd.Args[0])}
    if len(cmd.Args) > 1 {
        // Take the first non-flag argument as the subcommand
        for _, arg := range cmd.Args[1:] {
            if !strings.HasPrefix(arg, "-") {
                parts = append(parts, arg)
                break
            }
        }
    }
    return strings.Join(parts, "-")
}
```

**Files to create**:
- `executor/circuit_breaker.go` — CircuitBreakerExecutor implementation (~150 lines)
- `executor/circuit_breaker_test.go` — Comprehensive tests (~300 lines)

**Dependencies**: None (Story 1 is self-contained within the `executor` package)

---

### Story 2: Table-Driven Unit Tests for All Circuit States

**As a** developer maintaining the circuit breaker
**I want** comprehensive table-driven tests covering every state transition
**So that** I can confidently refactor the implementation without regressions

**Acceptance Criteria**:

1. **Closed -> Open**: Verify N consecutive failures trip the breaker
2. **Open -> rejection**: Verify calls are rejected immediately with `ErrCircuitOpen`
3. **Open -> Half-Open**: Verify recovery timeout allows one probe
4. **Half-Open -> Closed**: Verify successful probe resets the breaker
5. **Half-Open -> Open**: Verify failed probe re-opens the breaker
6. **Closed stays Closed**: Verify intermittent failures (success between failures) do not trip the breaker
7. **Concurrent access**: Verify thread safety with parallel goroutines
8. **Command-class isolation**: Verify independent breakers for different command classes
9. **Config validation**: Verify custom thresholds and timeouts are respected
10. **Edge cases**: Zero threshold, very short recovery timeout, nil command

**Technical Design**:

```go
func TestCircuitBreakerStateTransitions(t *testing.T) {
    tests := []struct {
        name           string
        sequence       []callResult // sequence of success/failure outcomes
        expectedState  CircuitState
        expectedReject bool         // whether final call should be rejected
    }{
        {
            name:          "stays closed on successes",
            sequence:      []callResult{success, success, success},
            expectedState: CircuitClosed,
        },
        {
            name:          "opens after 3 consecutive failures",
            sequence:      []callResult{failure, failure, failure},
            expectedState: CircuitOpen,
            expectedReject: true,
        },
        {
            name:          "resets on success between failures",
            sequence:      []callResult{failure, failure, success, failure, failure},
            expectedState: CircuitClosed,
        },
        // ... more cases
    }
}
```

**Files to create/modify**:
- `executor/circuit_breaker_test.go` — Table-driven tests with mock executor

**Dependencies**: Story 1 (needs the implementation to test)

---

### Story 3: Integrate into Git Worktree Operations

**As a** user running sessions against repositories with degraded git state
**I want** git operations to fail fast after repeated failures
**So that** the web UI remains responsive even when a repository has a locked index or NFS issue

**Acceptance Criteria**:

1. **Given** a session with a corrupted git repository
   **When** `UpdateDiffStats()` is called repeatedly
   **Then** the first 3 calls execute normally (and fail via timeout), and subsequent calls return immediately with `ErrCircuitOpen`

2. **Given** a session whose git repository recovers from a lock
   **When** the recovery timeout elapses
   **Then** the circuit breaker allows a probe call, and on success, resumes normal operation

3. **Given** two sessions A and B
   **When** Session A's git repository is degraded
   **Then** Session B's git operations are unaffected

**Technical Design**:

The key insight is that `GitWorktree.runGitCommand()` currently calls `exec.Command("git", ...)` and `cmd.CombinedOutput()` directly — it does not use the `Executor` interface at all. To integrate the circuit breaker, we need to:

1. Add an `Executor` field to `GitWorktree` (injected via constructor)
2. Modify `runGitCommand()` to use the executor instead of direct `cmd.CombinedOutput()`
3. Wrap the executor with `CircuitBreakerExecutor` at the session level

The circuit breaker executor would be created per-session (in `Instance.Start()` or `GitWorktreeManager`) so that breaker state is scoped to the session.

```go
// session/git/worktree.go — Modified struct
type GitWorktree struct {
    repoPath      string
    worktreePath  string
    sessionName   string
    branchName    string
    baseCommitSHA string
    cmdExec       executor.Executor // NEW: injected executor for subprocess calls
}

// session/git/worktree_git.go — Modified runGitCommand
func (g *GitWorktree) runGitCommand(path string, args ...string) (string, error) {
    baseArgs := []string{"-C", path}
    cmd := exec.Command("git", append(baseArgs, args...)...)
    output, err := g.cmdExec.Output(cmd)
    if err != nil {
        return "", fmt.Errorf("git command failed: %s (%w)", output, err)
    }
    return string(output), nil
}
```

**Migration strategy**: The existing `NewGitWorktree` and `NewGitWorktreeFromStorage` constructors will accept an optional `Executor` parameter (or a functional option). When nil, they fall back to `executor.MakeExecutor()` for backward compatibility.

**Files to modify**:
- `session/git/worktree.go` — Add `cmdExec` field, update constructors
- `session/git/worktree_git.go` — Use `g.cmdExec` instead of direct `cmd.CombinedOutput()`
- `session/git/worktree_ops.go` — Update any direct exec calls
- `session/git/worktree_branch.go` — Update any direct exec calls
- `session/git_worktree_manager.go` — Create and inject executor
- `session/instance.go` — Wire executor creation in `start()`

**Dependencies**: Story 1 (needs `CircuitBreakerExecutor`)

---

### Story 4: Integrate into Tmux Operations and Add Debug Endpoint

**As an** operator debugging why a session shows stale terminal output
**I want** to see which circuit breakers are open and inspect their state
**So that** I can diagnose and resolve the underlying subprocess issue

**Acceptance Criteria**:

1. **Given** tmux subprocess calls use the circuit breaker executor
   **When** `tmux list-sessions` starts hanging
   **Then** after 3 failures the circuit opens and `DoesSessionExist()` returns false immediately

2. **Given** `tmux capture-pane` is circuit-broken for a session
   **When** I GET `/api/debug/circuit-breakers`
   **Then** I see the breaker state including command-class, state, failure count, and last transition time

3. **Given** a circuit is OPEN for tmux operations on session A
   **When** the tmux server recovers
   **Then** within 30 seconds, the circuit transitions to HALF-OPEN and successfully probes

**Technical Design**:

Tmux already injects `executor.Executor` via `NewTmuxSessionWithDeps()`. The integration is to wrap the injected executor with `CircuitBreakerExecutor` at construction time.

```go
// session/tmux/tmux.go — Update constructors to wrap executor
func NewTmuxSession(name string, program string) *TmuxSession {
    baseExec := executor.MakeExecutor()
    cbExec := executor.NewCircuitBreakerExecutor(baseExec, executor.DefaultCircuitBreakerConfig())
    return newTmuxSession(name, program, MakePtyFactory(), cbExec, TmuxPrefix)
}
```

**Debug endpoint** (following existing `/api/debug/escape-codes/*` pattern):

```go
// server/services/circuit_breaker_handler.go
// GET /api/debug/circuit-breakers
// Response:
{
  "breakers": [
    {
      "key": "session-A:git-diff",
      "state": "OPEN",
      "consecutive_failures": 5,
      "last_failure": "2026-03-18T10:30:00Z",
      "last_state_change": "2026-03-18T10:30:00Z",
      "config": {
        "failure_threshold": 3,
        "recovery_timeout_seconds": 30
      }
    }
  ]
}
```

**Registry pattern**: A global `CircuitBreakerRegistry` allows the debug endpoint to iterate all breakers. Each `CircuitBreakerExecutor` registers itself on creation and deregisters on garbage collection (via `Close()` method called during session cleanup).

**Files to create**:
- `server/services/circuit_breaker_handler.go` — Debug endpoint (~80 lines)
- `executor/registry.go` — Global breaker registry (~50 lines)

**Files to modify**:
- `session/tmux/tmux.go` — Wrap executor in constructors
- `server/server.go` — Register debug endpoint

**Dependencies**: Story 1 (needs `CircuitBreakerExecutor`), can run in parallel with Story 3

---

## Dependency Visualization

```
Story 1: CircuitBreakerExecutor
    |
    +---> Story 2: Unit Tests (sequential after Story 1)
    |
    +---> Story 3: Git Integration (parallel with Story 4, after Story 1)
    |
    +---> Story 4: Tmux Integration + Debug Endpoint (parallel with Story 3, after Story 1)
```

**Critical path**: Story 1 -> Story 2 -> (Story 3 || Story 4)

**Recommended implementation order**:
1. Story 1 + Story 2 (single PR — implementation + tests together)
2. Story 3 (PR: git integration)
3. Story 4 (PR: tmux integration + debug endpoint)

Stories 3 and 4 can be developed in parallel by different developers after Story 1+2 merges.

## Known Issues

### BUG-001: Circuit Breakers Must Be Per-Instance, Not Shared Globally [SEVERITY: High]

**Description**: If circuit breakers are accidentally shared across sessions (e.g., via a global map keyed only by command-class without session qualifier), one session's degraded git repository will trip the breaker for all sessions' git operations. This is the single most critical correctness requirement.

**Mitigation**:
- Key breakers by composite key `(session-title, command-class)` — never by command-class alone
- Enforce via constructor: `CircuitBreakerExecutor` is created per-session, not as a singleton
- Add explicit test: "Session A failure does not affect Session B"
- Code review checklist item: verify no global breaker map without session scoping

**Files Likely Affected**:
- `executor/circuit_breaker.go` — Breaker map keying
- `session/instance.go` — Executor creation site

**Prevention Strategy**:
- Create `CircuitBreakerExecutor` in `Instance.start()` with session title injected
- Document in code comments that breakers MUST be per-instance

### BUG-002: Circuit State Must Not Persist Across Server Restart [SEVERITY: Medium]

**Description**: If circuit breaker state were serialized to disk (accidentally via session persistence), a breaker that was OPEN at shutdown could remain OPEN after restart even though the underlying issue resolved. Circuit state must be ephemeral — reconstructed fresh on each server start.

**Mitigation**:
- Circuit breaker state is stored only in memory (Go struct fields)
- `CircuitBreakerExecutor` is never serialized — it is not part of `InstanceData`
- Executors are created fresh in `Instance.start()` and `FromInstanceData()`
- Add a comment in `session/storage.go` noting that executor state is intentionally excluded

**Files Likely Affected**:
- `session/instance.go` — `ToInstanceData()` must not include executor
- `executor/circuit_breaker.go` — No `json` struct tags

### BUG-003: Race Condition in HALF-OPEN Probe Admission [SEVERITY: Medium]

**Description**: When the circuit transitions to HALF-OPEN, exactly one probe call should be admitted. If multiple goroutines check `state == CircuitHalfOpen` simultaneously, multiple probes could execute concurrently, leading to inconsistent state transitions.

**Mitigation**:
- Use `sync.Mutex` around the entire check-and-execute path in HALF-OPEN state
- The mutex ensures only one goroutine enters the probe path; others see OPEN and get rejected
- Add concurrent test: 100 goroutines hitting a HALF-OPEN breaker, verify exactly 1 probe executes

**Files Likely Affected**:
- `executor/circuit_breaker.go` — `allowRequest()` and `recordResult()` methods

**Prevention Strategy**:
- Hold mutex through the entire allow-execute-record cycle for HALF-OPEN probes
- Use `sync.Mutex` (not `RWMutex`) for state transitions to prevent read-during-write

### BUG-004: `runGitCommand()` Uses `CombinedOutput()` Directly, Bypassing Executor [SEVERITY: High]

**Description**: Currently `GitWorktree.runGitCommand()` calls `exec.Command(...).CombinedOutput()` directly — it does not use any `Executor` interface. Simply adding a `CircuitBreakerExecutor` to the struct is insufficient; the actual subprocess invocation site must be changed to call `g.cmdExec.Output(cmd)` instead.

**Mitigation**:
- Story 3 explicitly modifies `runGitCommand()` to use the injected executor
- Additionally, `worktree.go:findExistingWorktreeForBranch()` and `worktree_branch.go` have direct `exec.Command` calls that should be audited
- Add a grep-based CI check or linter annotation to flag direct `exec.Command` usage in `session/git/` package

**Files Likely Affected**:
- `session/git/worktree_git.go` — `runGitCommand()`
- `session/git/worktree.go` — `findExistingWorktreeForBranch()`
- `session/git/worktree_ops.go` — Any direct exec calls
- `session/git/worktree_branch.go` — Any direct exec calls

**Prevention Strategy**:
- Audit all `exec.Command` calls in `session/git/` package before implementation
- Route all subprocess calls through the injected executor
- Consider a package-level convention: "no direct exec.Command in this package"

### BUG-005: Executor.Output() Return Value Mismatch with CombinedOutput() [SEVERITY: Medium]

**Description**: `GitWorktree.runGitCommand()` currently uses `cmd.CombinedOutput()` which returns stdout+stderr combined. The `Executor.Output()` method uses `cmd.Output()` which returns only stdout (stderr goes to `ExitError.Stderr`). This semantic difference could cause git error messages to be lost when switching to the executor interface.

**Mitigation**:
- Use `TimeoutExecutor.OutputWithPipes()` which captures both stdout and stderr
- Or modify the `Executor` interface to add a `CombinedOutput()` method
- Or set `cmd.Stderr = &bytes.Buffer{}` before passing to `executor.Output()` and combine manually
- Test that git error messages are still propagated correctly after the switch

**Files Likely Affected**:
- `executor/executor.go` — Potentially add `CombinedOutput()` to interface
- `session/git/worktree_git.go` — `runGitCommand()` output handling

### BUG-006: tmux DoesSessionExist() Has Its Own Timeout Context [SEVERITY: Low]

**Description**: `DoesSessionExist()` in `tmux.go` creates its own `context.WithTimeout` (3 seconds) and calls `t.cmdExec.Output(cmd)`. If the executor is a `TimeoutExecutor` wrapping a `CircuitBreakerExecutor`, there are now two timeout layers. The innermost timeout wins, but the outer one wastes a goroutine waiting. More importantly, the `context.WithTimeout` in `DoesSessionExist()` creates the `exec.Cmd` with `exec.CommandContext`, which uses process killing — this may interfere with the executor's own timeout mechanism.

**Mitigation**:
- `DoesSessionExist()` should delegate timeout responsibility to the executor rather than managing its own context
- Or accept the duplication since the inner timeout (3s) is shorter than the executor timeout and will win
- Document this interaction in code comments
- Monitor for goroutine leaks under load

**Files Likely Affected**:
- `session/tmux/tmux.go` — `DoesSessionExist()`, `DoesSessionExistNoCache()`

### BUG-007: Breaker Registry Memory Leak on Session Destruction [SEVERITY: Medium]

**Description**: If breakers are registered in a global registry (for the debug endpoint) but not deregistered when a session is destroyed, the registry will grow unboundedly as sessions are created and destroyed over the server lifetime.

**Mitigation**:
- Add a `Close()` method to `CircuitBreakerExecutor` that deregisters from the global registry
- Call `Close()` during session cleanup (`Instance.Destroy()`, `Instance.KillSession()`)
- Add a test that creates and destroys 100 sessions and verifies the registry returns to zero entries
- Consider using `sync.Map` with weak-reference semantics or explicit cleanup

**Files Likely Affected**:
- `executor/circuit_breaker.go` — `Close()` method
- `executor/registry.go` — Deregistration
- `session/instance.go` — Wire `Close()` into cleanup path

## Testing Strategy

### Unit Tests (Story 2)

- Table-driven state transition tests (10+ cases)
- Concurrent access tests (100 goroutines)
- Command-class extraction tests
- Config validation tests
- Timer-based recovery tests (using injectable clock)
- Mock executor for deterministic failure injection

### Integration Tests (Stories 3-4)

- `GitWorktree` with failing executor verifies circuit opens
- `TmuxSession` with failing executor verifies fast rejection
- Cross-session isolation test (two instances, one degraded)
- Recovery test (circuit opens, underlying issue resolves, circuit closes)

### Testing Considerations

- **Time dependency**: Circuit breaker recovery relies on `time.Now()`. Inject a `Clock` interface for deterministic testing without `time.Sleep()`.
- **Executor mocking**: Create a `MockExecutor` that returns configurable success/failure sequences for table-driven tests.

## Implementation Tasks

### Phase 1: Core Implementation (Story 1 + Story 2)

| # | Task | Effort | Dependencies |
|---|------|--------|-------------|
| 1.1 | Define `CircuitState`, `CircuitBreakerConfig`, `ErrCircuitOpen` types | S | None |
| 1.2 | Implement `circuitBreaker` struct with state machine logic | M | 1.1 |
| 1.3 | Implement `commandClass()` key extraction function | S | None |
| 1.4 | Implement `CircuitBreakerExecutor.Run()` and `Output()` | M | 1.2, 1.3 |
| 1.5 | Add injectable `Clock` interface for testability | S | None |
| 1.6 | Write table-driven state transition tests | M | 1.4 |
| 1.7 | Write concurrent access tests | S | 1.4 |
| 1.8 | Write command-class extraction tests | S | 1.3 |

### Phase 2: Git Integration (Story 3)

| # | Task | Effort | Dependencies |
|---|------|--------|-------------|
| 2.1 | Add `cmdExec executor.Executor` field to `GitWorktree` | S | Phase 1 |
| 2.2 | Update `GitWorktree` constructors to accept optional executor | S | 2.1 |
| 2.3 | Modify `runGitCommand()` to use injected executor | M | 2.2 |
| 2.4 | Audit and convert all direct `exec.Command` calls in `session/git/` | M | 2.3 |
| 2.5 | Wire executor creation in `GitWorktreeManager` / `Instance.start()` | S | 2.3 |
| 2.6 | Add integration test: degraded git repo triggers circuit open | M | 2.5 |

### Phase 3: Tmux Integration + Debug Endpoint (Story 4)

| # | Task | Effort | Dependencies |
|---|------|--------|-------------|
| 3.1 | Update tmux constructors to wrap executor with circuit breaker | S | Phase 1 |
| 3.2 | Implement `CircuitBreakerRegistry` for global observability | S | Phase 1 |
| 3.3 | Add `Close()` method and cleanup lifecycle | S | 3.2 |
| 3.4 | Implement `/api/debug/circuit-breakers` HTTP handler | M | 3.2 |
| 3.5 | Register debug endpoint in `server.go` | S | 3.4 |
| 3.6 | Add integration test: tmux degradation triggers circuit open | M | 3.1 |
| 3.7 | Add registry cleanup test (create/destroy sessions) | S | 3.3 |

**Effort key**: S = Small (< 1 hour), M = Medium (1-3 hours)