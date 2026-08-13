# Testing Harness: Real Tmux Integration and UI Hang Detection

## Epic Overview

**Goal**: Create comprehensive multi-layer testing infrastructure that catches real-world UI hangs, tmux session creation issues, and external command blocking that current mock-based tests cannot detect.

**Value Proposition**:
- Reproduce production hangs in test environment
- Detect session creation timeouts and polling loops
- Verify UI exit/quit functionality works correctly
- Catch external command blocking (e.g., `which claude`)

**Success Metrics**:
- ✅ Integration tests catch real tmux hangs
- ✅ TUI interaction tests verify exit/quit works
- ✅ No test runs longer than 30 seconds without clear failure
- ✅ Timeout protection on all external commands
- ✅ Can reproduce production hangs in controlled test environment

**Current Gap**: Mock-based tests (`mockTmuxExecutor`) bypass real tmux, hiding:
- Real tmux command execution issues
- PTY allocation problems
- `DoesSessionExist()` polling loop hangs
- UI event loop blocking behavior

---

## Story Breakdown

### Story 1: Timeout Executor Infrastructure (Foundation)
**Objective**: Wrap all external command execution with context-based timeouts to prevent indefinite blocking.

**Value**: Prevents tests and production code from hanging on external commands like `which claude`.

**Scope**:
- Create timeout-aware command executor
- Replace blocking command execution in config
- Add configurable timeout durations
- Provide clear timeout error messages

**Estimated Duration**: 2-3 hours (2 atomic tasks)

---

### Story 2: Real Tmux Integration Testing (Core)
**Objective**: Test actual tmux session creation, existence checking, and cleanup with isolated tmux servers.

**Value**: Catches real tmux behavior issues that mocks cannot detect.

**Scope**:
- Isolated tmux server helpers (per-test isolation)
- Real session creation and lifecycle tests
- Timeout scenario validation
- Cleanup verification

**Estimated Duration**: 3-4 hours (3 atomic tasks)

---

### Story 3: TUI Interaction Testing (End-to-End)
**Objective**: Use expect-like testing to interact with real TUI, send keypresses, verify responses.

**Value**: Catches UI hang scenarios including exit/quit issues user reported.

**Scope**:
- Expect-based test infrastructure
- Session creation flow testing
- Navigation and exit handling
- Hang detection with force-kill

**Estimated Duration**: 4-5 hours (3 atomic tasks)

---

### Story 4: Test Hang Detection (Quality)
**Objective**: Add test-level timeout monitoring with goroutine stack dumps on hang.

**Value**: Clear diagnostic information when tests hang, preventing indefinite CI/CD blocking.

**Scope**:
- Test timeout wrapper utilities
- Goroutine stack dump on timeout
- Integration with existing tests
- Documentation

**Estimated Duration**: 1-2 hours (1 atomic task)

---

## Atomic Task Specifications

### Task 1.1: Create Timeout Executor Infrastructure (2h) [MEDIUM]

**Scope**: Implement context-based timeout wrapper for command execution to prevent indefinite blocking.

**Files** (4 files):
- `executor/timeout_executor.go` (create) - Core timeout executor implementation
- `executor/executor.go` (modify) - Add timeout executor constructor
- `executor/timeout_executor_test.go` (create) - Comprehensive timeout tests
- `config/config.go` (modify) - Replace globalCommandExecutor usage

**Context Required**:
- Current `executor.Executor` interface pattern
- How `config.GetClaudeCommand()` executes shell commands
- Context-based timeout patterns in Go
- Error wrapping for timeout scenarios

**Success Criteria**:
- `TimeoutExecutor` implements `Executor` interface
- Commands timeout after configurable duration (default 5s)
- Clear error messages indicate timeout occurred
- Tests verify timeout behavior and cleanup
- Config commands use timeout executor

**Testing**:
```go
// Test timeout behavior
func TestTimeoutExecutorTimesOut(t *testing.T)
// Test successful command execution
func TestTimeoutExecutorSuccess(t *testing.T)
// Test error propagation
func TestTimeoutExecutorError(t *testing.T)
```

**Dependencies**: None (foundation task)

**Implementation Hints**:
- Use `context.WithTimeout()` for clean cancellation
- Ensure proper process cleanup on timeout
- Handle both stdout and stderr in timeout scenarios
- Make timeout duration configurable via constructor

---

### Task 1.2: Integrate Timeout Executor in Config Package (1h) [MICRO]

**Scope**: Replace blocking command execution in config package with timeout-aware version.

**Files** (2 files):
- `config/config.go` (modify) - Use TimeoutExecutor for GetClaudeCommand
- `config/config_test.go` (modify) - Verify timeout behavior in config

**Context Required**:
- `config.GetClaudeCommand()` implementation (line 162)
- How `globalCommandExecutor` is used
- Test patterns for config validation

**Success Criteria**:
- `GetClaudeCommand()` uses TimeoutExecutor
- Config initialization never hangs indefinitely
- Tests verify timeout on missing claude command
- Default timeout set to 5 seconds

**Testing**:
```go
func TestGetClaudeCommandTimeout(t *testing.T)
```

**Dependencies**: Task 1.1 (requires TimeoutExecutor)

---

### Task 2.1: Create Isolated Tmux Test Helpers (2h) [SMALL]

**Scope**: Build infrastructure for running real tmux with per-test server isolation.

**Files** (3 files):
- `testutil/tmux.go` (create) - Isolated tmux server utilities
- `testutil/tmux_test.go` (create) - Test helper validation
- `testutil/cleanup.go` (modify) - Enhanced cleanup for tmux servers

**Context Required**:
- Tmux `-L` flag for server isolation
- Session naming and cleanup patterns
- Test cleanup best practices
- Current `session/tmux/tmux.go` server socket usage

**Success Criteria**:
- `CreateIsolatedTmuxServer(t *testing.T) *TmuxTestServer` helper
- Each test gets unique tmux server socket
- Automatic cleanup after test completion
- No interference between concurrent tests
- Cleanup handles failure scenarios gracefully

**Testing**:
```go
func TestIsolatedTmuxServerCreation(t *testing.T)
func TestConcurrentTmuxServers(t *testing.T)
func TestTmuxServerCleanup(t *testing.T)
```

**Dependencies**: None (can run in parallel with Task 1.1)

**Implementation Hints**:
- Use `t.Name()` for unique server socket names
- Register cleanup with `t.Cleanup()`
- Check for tmux availability with `which tmux`
- Handle tmux not installed gracefully

---

### Task 2.2: Write Real Tmux Session Creation Tests (2h) [SMALL]

**Scope**: Test actual tmux session lifecycle with real command execution.

**Files** (3 files):
- `session/tmux_integration_test.go` (create) - Core integration tests
- `session/tmux/tmux_test.go` (modify) - Enhanced with real tmux cases
- `testutil/tmux.go` (modify) - Add session validation helpers

**Context Required**:
- `session/tmux/tmux.go` Start() implementation
- `DoesSessionExist()` polling loop (line 335)
- Session creation timeout logic
- PTY allocation patterns

**Success Criteria**:
- Test real session creation completes successfully
- Test session existence checking works correctly
- Test timeout scenarios (2-second timeout) trigger properly
- Test cleanup kills real tmux sessions
- Tests use isolated tmux servers

**Testing**:
```go
func TestRealTmuxSessionCreation(t *testing.T)
func TestRealTmuxSessionExistence(t *testing.T)
func TestRealTmuxSessionTimeout(t *testing.T)
func TestRealTmuxSessionCleanup(t *testing.T)
```

**Dependencies**: Task 2.1 (requires TmuxTestServer)

---

### Task 2.3: Test Tmux Polling and Hang Scenarios (2h) [MEDIUM]

**Scope**: Specifically test the `DoesSessionExist()` polling loop and hang detection.

**Files** (3 files):
- `session/tmux_hang_test.go` (create) - Hang scenario tests
- `session/tmux/tmux.go` (analyze) - Understand polling loop
- `testutil/wait.go` (modify) - Add hang detection utilities

**Context Required**:
- `DoesSessionExist()` implementation and caching
- Exponential backoff pattern in session creation
- Timeout handling in Start() method
- How sessions can get stuck in polling loop

**Success Criteria**:
- Test simulates slow session creation (polls multiple times)
- Test verifies exponential backoff behavior
- Test ensures timeout protection works (2s max)
- Test validates cache invalidation on session kill
- Clear error messages when polling times out

**Testing**:
```go
func TestSessionExistencePollingLoop(t *testing.T)
func TestSessionCreationWithDelay(t *testing.T)
func TestExponentialBackoffBehavior(t *testing.T)
func TestPollingTimeoutProtection(t *testing.T)
```

**Dependencies**: Task 2.2 (builds on session creation tests)

---

### Task 3.1: Add Expect Testing Infrastructure (2h) [MEDIUM]

**Scope**: Integrate `github.com/Netflix/go-expect` for TUI interaction testing.

**Files** (4 files):
- `go.mod` (modify) - Add go-expect dependency
- `testutil/expect.go` (create) - Expect test helpers
- `testutil/expect_test.go` (create) - Validate expect infrastructure
- `testutil/tui.go` (create) - TUI test utilities

**Context Required**:
- How BubbleTea programs initialize and run
- PTY requirements for expect testing
- Timeout patterns for expect operations
- Clean shutdown of TUI programs

**Success Criteria**:
- `github.com/Netflix/go-expect` added to dependencies
- `CreateTUIExpectSession(t *testing.T, program tea.Model)` helper
- Can send keypresses and read output
- Timeout protection on expect operations (5s default)
- Clean cleanup of PTY and program

**Testing**:
```go
func TestExpectInfrastructure(t *testing.T)
func TestTUISendKeys(t *testing.T)
func TestTUIReadOutput(t *testing.T)
func TestTUITimeout(t *testing.T)
```

**Dependencies**: None (infrastructure task, can run parallel)

**Implementation Hints**:
- Use `pty.Start()` for PTY allocation
- Wrap expect operations with timeouts
- Handle terminal size properly (80x24 default)
- Register cleanup for PTY and process

---

### Task 3.2: Test Session Creation Flow with Real TUI (2h) [MEDIUM]

**Scope**: End-to-end test of session creation through actual TUI interaction.

**Files** (3 files):
- `app/tui_session_creation_test.go` (create) - Session creation E2E tests
- `testutil/expect.go` (modify) - Add session creation helpers
- `app/app.go` (analyze) - Understand session creation flow

**Context Required**:
- Session creation key bindings (n key)
- SessionSetupOverlay interaction flow
- How `sessionCreationResultMsg` is sent
- Session creation timeout (60s in app.go)

**Success Criteria**:
- Test starts real TUI with expect
- Test sends 'n' key and enters session details
- Test verifies session appears in list
- Test completes within timeout (10s)
- Test cleanup kills TUI and tmux sessions

**Testing**:
```go
func TestTUISessionCreationFlow(t *testing.T)
func TestTUISessionCreationTimeout(t *testing.T)
func TestTUISessionCreationCancellation(t *testing.T)
```

**Dependencies**: Task 3.1, Task 2.1 (requires expect and tmux infrastructure)

---

### Task 3.3: Test UI Exit and Quit Handling (2h) [MEDIUM]

**Scope**: Test the "inability to exit" issue user reported with real TUI interaction.

**Files** (3 files):
- `app/tui_exit_test.go` (create) - Exit/quit behavior tests
- `testutil/expect.go` (modify) - Add exit verification helpers
- `app/app.go` (analyze) - Understand quit key handling

**Context Required**:
- Quit key bindings (q, Ctrl+C)
- How `tea.Quit` command works
- Session cleanup on exit
- Potential blocking points in exit flow

**Success Criteria**:
- Test sends 'q' key and verifies TUI exits
- Test sends Ctrl+C and verifies clean shutdown
- Test ensures exit completes within 2 seconds
- Test force-kills TUI if exit hangs (diagnostic)
- Clear error message if exit hangs

**Testing**:
```go
func TestTUIExitWithQKey(t *testing.T)
func TestTUIExitWithCtrlC(t *testing.T)
func TestTUIExitWithRunningSession(t *testing.T)
func TestTUIExitHangDetection(t *testing.T)
```

**Dependencies**: Task 3.1 (requires expect infrastructure)

---

### Task 4.1: Add Test Hang Detection and Diagnostics (1h) [SMALL]

**Scope**: Wrapper utilities for test-level timeout monitoring with goroutine stack dumps.

**Files** (3 files):
- `testutil/timeout.go` (create) - Test timeout wrappers
- `testutil/timeout_test.go` (create) - Validate timeout behavior
- `CLAUDE.md` (modify) - Document testing timeout strategy

**Context Required**:
- Goroutine stack dump patterns (`runtime.Stack()`)
- Test timeout best practices
- How to wrap test functions
- Timeout duration recommendations

**Success Criteria**:
- `RunWithTimeout(t, duration, testFunc)` wrapper
- Goroutine stack dump on timeout
- Clear indication of where test is stuck
- Integration example in existing test
- Documentation in CLAUDE.md

**Testing**:
```go
func TestTimeoutWrapperDetectsHang(t *testing.T)
func TestTimeoutWrapperAllowsCompletion(t *testing.T)
func TestStackDumpOnTimeout(t *testing.T)
```

**Dependencies**: None (utility task)

**Implementation Hints**:
- Use `runtime.Stack(buf, true)` for all goroutines
- Print stack to test output for debugging
- Mark test as failed with clear message
- Make timeout duration configurable

---

## Dependency Visualization

```
Story 1: Timeout Executor (Foundation)
  Task 1.1: Create TimeoutExecutor [2h]
    ↓
  Task 1.2: Integrate in Config [1h]

Story 2: Real Tmux Integration (Core)
  Task 2.1: Isolated Tmux Helpers [2h]
    ↓
  Task 2.2: Session Creation Tests [2h]
    ↓
  Task 2.3: Polling and Hangs [2h]

Story 3: TUI Interaction (End-to-End)
  Task 3.1: Expect Infrastructure [2h]
    ├→ Task 3.2: Session Creation E2E [2h]
    └→ Task 3.3: Exit/Quit Testing [2h]

Story 4: Test Hang Detection (Quality)
  Task 4.1: Timeout Wrappers [1h]

Parallel Opportunities:
- Task 1.1 || Task 2.1 || Task 3.1 || Task 4.1 (all foundation)
- Task 3.2 || Task 3.3 (after 3.1, independent scenarios)
```

**Critical Path**:
1. Task 1.1 → Task 1.2 (3h total)
2. Task 2.1 → Task 2.2 → Task 2.3 (6h total)
3. Task 3.1 → Task 3.2 (4h total)

**Total Estimated Effort**: 14 hours
**Parallelizable**: ~6 hours can be done concurrently
**Sequential Critical Path**: ~8 hours

---

## Context Preparation Guide

### For Task 1.1 (Timeout Executor):
**Files to Review**:
- `executor/executor.go` - Understand interface
- `config/config.go:162` - See GetClaudeCommand usage
- `docs/tasks/emergency-test-timeouts.md` - Problem context

**Concepts to Understand**:
- Go context cancellation patterns
- Process cleanup on timeout
- Command execution with exec.Cmd

---

### For Task 2.1 (Tmux Helpers):
**Files to Review**:
- `session/tmux/tmux.go:82` - TmuxPrefix and server socket
- `session/comprehensive_session_creation_test.go:19` - Current test isolation
- `testutil/teatest_helpers.go:148` - Cleanup patterns

**Concepts to Understand**:
- Tmux `-L` flag for server isolation
- Test cleanup with `t.Cleanup()`
- Concurrent test execution safety

---

### For Task 2.2 (Real Session Tests):
**Files to Review**:
- `session/tmux/tmux.go:207` - Start() implementation
- `session/tmux/tmux.go:335` - DoesSessionExist() polling
- `testutil/wait.go` - WaitForCondition patterns

**Concepts to Understand**:
- Real PTY allocation with `github.com/creack/pty`
- Session creation timeout logic
- Exponential backoff patterns

---

### For Task 2.3 (Polling Tests):
**Files to Review**:
- `session/tmux/tmux.go:556` - DoesSessionExist() implementation
- `session/tmux/tmux.go:335` - Polling loop in Start()
- `docs/adr/003-no-static-sleeps-in-tests.md` - Testing philosophy

**Concepts to Understand**:
- Session existence caching (70-75)
- Timeout protection mechanisms
- Cache invalidation scenarios

---

### For Task 3.1 (Expect Infrastructure):
**Files to Review**:
- `testutil/teatest_helpers.go` - Current TUI test patterns
- `app/app.go:28` - Run() entrypoint
- `github.com/Netflix/go-expect` - Library documentation

**Concepts to Understand**:
- BubbleTea program lifecycle
- PTY-based testing requirements
- Expect pattern matching

---

### For Task 3.2 (Session Creation E2E):
**Files to Review**:
- `app/app.go` - Session creation flow
- `ui/overlay/sessionSetup.go` - SessionSetupOverlay
- `app/handleAdvancedSessionSetup.go` - Advanced setup flow

**Concepts to Understand**:
- Key binding for session creation (n key)
- Overlay interaction patterns
- sessionCreationResultMsg handling

---

### For Task 3.3 (Exit Testing):
**Files to Review**:
- `app/app.go` - Update() method, quit handling
- `keys/keys.go` - Quit key bindings
- User report about "inability to exit"

**Concepts to Understand**:
- tea.Quit command
- Session cleanup on exit
- Potential blocking points in shutdown

---

### For Task 4.1 (Hang Detection):
**Files to Review**:
- `docs/adr/003-no-static-sleeps-in-tests.md` - Testing strategy
- Go runtime package - Stack dump APIs

**Concepts to Understand**:
- Goroutine stack dump format
- Test timeout best practices
- Diagnostic output patterns

---

## INVEST Validation Matrix

| Task | Independent | Negotiable | Valuable | Estimable | Small | Testable |
|------|-------------|------------|----------|-----------|-------|----------|
| 1.1  | ✅ No deps | ✅ Interface design flexible | ✅ Prevents hangs | ✅ 2h confident | ✅ Single executor | ✅ Timeout tests |
| 1.2  | ✅ Uses 1.1 only | ✅ Integration approach | ✅ Config never hangs | ✅ 1h confident | ✅ Config focused | ✅ Config tests |
| 2.1  | ✅ Standalone helpers | ✅ Isolation approach | ✅ Test foundation | ✅ 2h confident | ✅ Helper utilities | ✅ Helper tests |
| 2.2  | ✅ Uses 2.1 only | ✅ Test scenarios | ✅ Catches real bugs | ✅ 2h confident | ✅ Session tests | ✅ Integration tests |
| 2.3  | ✅ Uses 2.2 only | ✅ Hang scenarios | ✅ Validates timeouts | ✅ 2h confident | ✅ Polling focused | ✅ Hang tests |
| 3.1  | ✅ No deps | ✅ Expect framework | ✅ TUI test foundation | ✅ 2h confident | ✅ Infrastructure | ✅ Expect tests |
| 3.2  | ✅ Uses 3.1, 2.1 | ✅ E2E approach | ✅ Session creation | ✅ 2h confident | ✅ Creation flow | ✅ E2E tests |
| 3.3  | ✅ Uses 3.1 only | ✅ Exit scenarios | ✅ Solves user issue | ✅ 2h confident | ✅ Exit handling | ✅ Exit tests |
| 4.1  | ✅ No deps | ✅ Timeout approach | ✅ Prevents CI hangs | ✅ 1h confident | ✅ Single utility | ✅ Timeout tests |

All tasks meet INVEST criteria with proper context boundaries (3-5 files max).

---

## Integration Checkpoints and Testing Milestones

### Checkpoint 1: Timeout Infrastructure Complete
**After**: Task 1.2
**Validation**:
- `go test ./executor -v` passes
- `go test ./config -v` passes
- Config initialization completes within 5 seconds
- Timeout errors have clear messages

---

### Checkpoint 2: Real Tmux Integration Working
**After**: Task 2.3
**Validation**:
- `go test ./session -run Integration -v` passes
- Tests use isolated tmux servers
- Session creation timeout scenarios detected
- No interference between concurrent tests

---

### Checkpoint 3: TUI Interaction Testing Functional
**After**: Task 3.3
**Validation**:
- `go test ./app -run TUI -v` passes
- Session creation flow works end-to-end
- Exit/quit handling verified
- User's "inability to exit" issue reproduced/fixed

---

### Checkpoint 4: Complete Testing Infrastructure
**After**: Task 4.1
**Validation**:
- All tests complete within 30 seconds or fail clearly
- Hang detection catches indefinite blocking
- Goroutine dumps provide diagnostic information
- Documentation updated in CLAUDE.md

---

## Migration Strategy

**Phase 1**: Foundation (Tasks 1.1, 1.2, 4.1) - 4 hours
- Build timeout executor infrastructure
- Add hang detection utilities
- No disruption to existing tests

**Phase 2**: Real Tmux Integration (Tasks 2.1, 2.2, 2.3) - 6 hours
- Add alongside existing mock tests
- Validate real tmux behavior
- Keep mocks for fast unit testing

**Phase 3**: TUI Interaction (Tasks 3.1, 3.2, 3.3) - 6 hours
- Add expect-based E2E tests
- Verify user-reported issues
- Document TUI testing patterns

**Phase 4**: Documentation and Refinement - 2 hours
- Update CLAUDE.md with testing strategy
- Add troubleshooting guide
- Document expected test execution times

**Total Effort**: ~18 hours (14h tasks + 4h refinement)

---

## Success Criteria Summary

**Technical Success**:
- ✅ Integration tests catch real tmux hangs
- ✅ TUI interaction tests verify exit/quit works
- ✅ No test runs longer than 30 seconds without clear failure
- ✅ Timeout protection on all external commands
- ✅ Goroutine stack dumps on hang detection

**Quality Success**:
- ✅ Tests provide reliable feedback for development
- ✅ Can reproduce production hangs in test environment
- ✅ Clear diagnostic information on failures
- ✅ No test flakiness from timing issues

**Developer Experience Success**:
- ✅ Tests run fast in normal case (mocks still used)
- ✅ Integration tests catch real issues
- ✅ Documentation enables test maintenance
- ✅ CI/CD pipeline provides clear feedback

**Total Estimated Effort**: 14 hours across 9 atomic tasks (4 stories)
**Risk**: LOW-MEDIUM - Well-defined scope, clear testing patterns
**Impact**: HIGH - Solves critical testing gap, prevents production issues
