# Feature Plan: Automated Test Coverage for Core Business Logic

**Date**: 2026-03-18
**Status**: Draft
**Scope**: Add unit test infrastructure and coverage for the two highest-risk untested areas: Go backend `Instance` state transitions and frontend React hooks (`useSessionService`, `useReviewQueue`, `useTerminalStream`)

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [Research Findings: Current Test Landscape](#research-findings-current-test-landscape)
- [ADR-015: Mock Strategy for Instance Dependencies](#adr-015-mock-strategy-for-instance-dependencies)
- [ADR-016: Frontend Test Framework Selection](#adr-016-frontend-test-framework-selection)
- [ADR-017: Coverage Targets and Quality Gates](#adr-017-coverage-targets-and-quality-gates)
- [Story 1: Go Backend Mock Infrastructure and Instance State Transition Tests](#story-1-go-backend-mock-infrastructure-and-instance-state-transition-tests)
- [Story 2: Frontend Test Setup and useSessionService Tests](#story-2-frontend-test-setup-and-usesessionservice-tests)
- [Story 3: Frontend useReviewQueue and useTerminalStream Tests](#story-3-frontend-usereviewqueue-and-useterminalstream-tests)
- [Known Issues and Bug Risks](#known-issues-and-bug-risks)
- [Testing Strategy](#testing-strategy)
- [Dependency Graph](#dependency-graph)

---

## Problem Statement

The two most critical codepaths in stapler-squad have zero or near-zero unit test coverage:

1. **`session/instance.go`** (2,077 lines, 70+ methods): The core session entity that owns
   all state transitions (Creating to Running, Running to Paused, Paused to Running,
   Running to NeedsApproval, NeedsApproval to Running). The existing `instance_test.go`
   covers only static data operations (tilde expansion, corrupted path migration, worktree
   existence checking). No tests exercise `Start()`, `Pause()`, `Resume()`, `Destroy()`,
   `Restart()`, or `StartController()` because these methods directly construct real
   `tmux.TmuxSession` and `git.GitWorktree` objects -- there are no interfaces to mock.

2. **Frontend hooks** (`useSessionService.ts` at 440 lines, `useReviewQueue.ts` at 434 lines,
   `useTerminalStream.ts` at 968 lines): These contain all client-side business logic for
   session management, optimistic updates, WebSocket reconnection, and terminal I/O. Zero
   test files exist for any hook. The project has Jest + Testing Library configured but only
   six source-level test files (terminal parsers, URL parser, websocket transport envelope).

**Impact of the gap:**
- State transition bugs (e.g., pausing an already-paused session, resuming without a worktree)
  are only caught by manual testing or production incidents.
- Optimistic update logic in `useSessionService` (duplicate prevention in `createSession`,
  local state removal in `deleteSession`) has no regression safety net.
- WebSocket reconnection and event handling in `useReviewQueue` (itemAdded deduplication,
  optimistic acknowledge with rollback) cannot be validated without end-to-end tests.

---

## Research Findings: Current Test Landscape

### Go Backend Tests

**Existing test files in `session/`**: 63 test files covering subpackages (`detection/`,
`search/`, `scrollback/`, `tmux/`, `git/`, `vc/`, `workspace/`, `mux/`, `framebuffer/`).

**`session/instance_test.go`**: 4 test functions:
- `TestFromInstanceDataWithMissingWorktree` -- manually constructs `Instance` struct, checks
  status after worktree deletion
- `TestStatusEnumValues` -- verifies iota ordering
- `TestTildeExpansionInNewInstance` -- calls `NewInstance()` with tilde paths
- `TestMigrationOfCorruptedPaths` -- calls `FromInstanceData()` with corrupted paths

**Key structural observation**: `TmuxProcessManager`, `GitWorktreeManager`, and
`ControllerManager` are concrete structs, not interfaces. `Instance` embeds them as value
fields (`tmuxManager TmuxProcessManager`, `gitManager GitWorktreeManager`,
`controllerManager ControllerManager`). Each manager wraps a concrete pointer
(`*tmux.TmuxSession`, `*git.GitWorktree`, `*ClaudeController`).

**Existing mock infrastructure**: `testutil/mocks.go` defines a `CommandExecutor` interface
with `MockCommandExecutor`. This pattern demonstrates the project's preferred mock approach:
interface extraction with function-field mocks.

**`session/test_helpers_test.go`**: Package-internal helper `createTestSession(title)` that
returns an `InstanceData` struct for use in table-driven tests.

### Frontend Tests

**Jest configuration exists** (`web-app/jest.config.js`): ts-jest preset, jsdom environment,
`@/` path alias mapped, `jest.setup.js` imports `@testing-library/jest-dom` and polyfills
`TextEncoder`/`TextDecoder`.

**Existing source-level test files** (6 files, none for hooks):
- `src/lib/terminal/__tests__/EscapeSequenceParser.test.ts`
- `src/lib/terminal/__tests__/flow-control-stress.test.ts`
- `src/lib/terminal/StateApplicator.test.ts`
- `src/lib/terminal/CircularBuffer.test.ts`
- `src/lib/github/urlParser.test.ts`
- `src/lib/transport/websocket-transport.test.ts`

**Dependencies already installed**: `@testing-library/react@16.3.0`,
`@testing-library/jest-dom@6.9.1`, `jest@30.2.0`, `jest-environment-jsdom@30.2.0`,
`ts-jest@29.4.4`.

**Package.json test scripts**: `"test": "jest"`, `"test:watch": "jest --watch"`.

**No Vitest present.** The project already uses Jest 30 with a working configuration. There
is no reason to introduce a second framework.

---

## ADR-015: Mock Strategy for Instance Dependencies

**Status**: Proposed

**Context**: `Instance.Start()`, `Instance.Pause()`, and `Instance.Resume()` call methods on
`TmuxProcessManager` and `GitWorktreeManager`, which in turn delegate to concrete
`*tmux.TmuxSession` and `*git.GitWorktree`. Testing state transitions requires controlling
the behavior of these dependencies without real tmux sessions or git repositories.

Three approaches were considered:

1. **Interface extraction** (chosen): Define `TmuxManager` and `GitManager` interfaces at the
   `session` package level. `TmuxProcessManager` and `GitWorktreeManager` already satisfy the
   shape. Change `Instance` fields from concrete structs to interface types. Provide mock
   implementations in test files.

2. **Monkey-patching with function fields**: Add function-field overrides (like
   `testutil.MockCommandExecutor`) to each manager. This avoids interface changes but pollutes
   production code with test hooks.

3. **Integration-only testing**: Use real tmux sessions with `TmuxServerSocket` isolation (the
   pattern in `testutil/tmux.go`). This tests real behavior but is slow, flaky, and
   OS-dependent.

**Decision**: Interface extraction (option 1).

**Rationale**:
- Follows the Dependency Inversion Principle (SOLID "D"): `Instance` depends on abstract
  behavior, not concrete implementations.
- Consistent with existing pattern in `testutil/mocks.go` (`CommandExecutor` interface).
- Enables table-driven tests with precise control over success/failure paths.
- Zero runtime cost (Go interfaces are structurally typed; no registration needed).
- Does not require tmux or git to be installed in CI environments.

**Consequences**:
- `Instance.tmuxManager` field type changes from `TmuxProcessManager` to `TmuxManager`
  (interface). `Instance.gitManager` changes from `GitWorktreeManager` to `GitManager`.
- Existing code that accesses `instance.tmuxManager.session` directly must use interface
  methods instead. A survey of `instance.go` shows all access goes through manager methods,
  except two locations in `FromInstanceData` and `Pause` that access `gitManager.worktree`
  and `gitManager.diffStats` directly -- these need accessor methods.
- `ControllerManager` is NOT extracted to an interface in this phase because controller
  testing requires a running PTY reader, which is out of scope. The controller interactions
  are tested indirectly through state assertions.
- Mock structs are placed in `session/mock_managers_test.go` (test-only, not exported).

**Migration Strategy**:
1. Define `TmuxManager` and `GitManager` interfaces in `session/interfaces.go`
2. Verify `TmuxProcessManager` and `GitWorktreeManager` satisfy the interfaces (compile check)
3. Change `Instance` field types to the interfaces
4. Fix any direct field access that bypasses the interface (add accessor methods)
5. Compilation gate: `go build .` must pass before proceeding

---

## ADR-016: Frontend Test Framework Selection

**Status**: Proposed

**Context**: The project needs tests for React hooks that use ConnectRPC clients, streaming
RPCs, and React state management. A test framework must support React 19, TypeScript, async
testing, and mock injection for ConnectRPC transports.

**Decision**: Keep Jest 30 (already configured and working).

**Rationale**:
- Jest 30, ts-jest, `@testing-library/react`, and `jest-environment-jsdom` are already
  installed and configured in `package.json`.
- Six existing test files use Jest. Introducing Vitest would create framework fragmentation.
- `@testing-library/react` `renderHook` is framework-agnostic and works with Jest.
- ConnectRPC mocking is transport-level: create a mock transport that returns canned
  responses, inject it via the `baseUrl` option that all hooks accept.

**Consequences**:
- No new dependencies needed for the test framework itself.
- A `createMockTransport()` utility will be created in `web-app/src/lib/test-utils/` that
  wraps `createRouterTransport` from `@connectrpc/connect` to provide canned RPC responses.
  If `createRouterTransport` is not available in the installed version, implement a manual
  mock transport.
- Tests use `renderHook` from `@testing-library/react` for hook testing.
- `act()` wrapper is required for all state-changing operations in hook tests.

---

## ADR-017: Coverage Targets and Quality Gates

**Status**: Proposed

**Context**: Need to establish measurable coverage targets for the new tests to ensure
meaningful protection without over-testing stable code.

**Decision**: Set the following coverage targets for the code touched by this feature:

| Area | Metric | Target |
|------|--------|--------|
| `Instance` state transitions | Branch coverage of `Start`, `Pause`, `Resume`, `Destroy`, `Restart` | 80% |
| `Instance` guard conditions | All error paths tested (not started, already paused, wrong status) | 100% |
| `useSessionService` | Line coverage of hook body | 70% |
| `useReviewQueue` | Line coverage of event handlers | 70% |
| `useTerminalStream` | Line coverage of connect/disconnect/sendInput | 50% (lower due to binary protocol complexity) |

**Rationale**:
- State transition guard conditions are the highest-value tests (prevent invalid transitions
  that corrupt session state). These MUST be 100%.
- Hook line coverage at 70% captures CRUD operations, error handling, and optimistic updates
  while acknowledging that some streaming edge cases are hard to unit test.
- Terminal stream at 50% reflects that the core connect/disconnect/input paths are testable
  but the binary WebSocket protocol, LZMA decompression, and xterm integration are better
  served by E2E tests.

**Consequences**:
- CI can enforce coverage minimums per-package once infrastructure exists.
- Coverage is measured with `go test -cover` (Go) and `jest --coverage` (frontend).

---

## Story 1: Go Backend Mock Infrastructure and Instance State Transition Tests

**Goal**: Enable isolated unit testing of `Instance` state machine by introducing interfaces
for `TmuxProcessManager` and `GitWorktreeManager`, then writing table-driven tests for all
major state transitions.

### Task 1.1: Define TmuxManager and GitManager Interfaces

**File**: `session/interfaces.go` (new)

Define two interfaces that capture the methods called by `Instance`:

```go
// TmuxManager defines the tmux session operations that Instance depends on.
type TmuxManager interface {
    HasSession() bool
    Session() *tmux.TmuxSession
    SetSession(s *tmux.TmuxSession)
    IsAlive() bool
    Close() error
    DetachSafely() error
    DoesSessionExist() bool
    Start(dir string) error
    RestoreWithWorkDir(workDir string) error
    SetDetachedSize(width, height int, instanceTitle string) error
    Attach() (chan struct{}, error)
    CapturePaneContent() (string, error)
    CapturePaneContentRaw() (string, error)
    CapturePaneContentWithOptions(startLine, endLine string) (string, error)
    GetPaneDimensions() (width, height int, err error)
    GetCursorPosition() (x, y int, err error)
    GetPTY() (*os.File, error)
    SendKeys(keys string) (int, error)
    SetWindowSize(cols, rows int) error
    RefreshClient() error
    TapEnter() error
    HasUpdated() (updated bool, hasPrompt bool)
    FilterBanners(content string) (string, int)
    HasMeaningfulContent(content string) bool
}

// GitManager defines the git worktree operations that Instance depends on.
type GitManager interface {
    HasWorktree() bool
    GetWorktree() *git.GitWorktree
    SetWorktree(wt *git.GitWorktree)
    GetWorktreePath() string
    GetRepoPath() string
    GetRepoName() string
    GetBranchName() string
    GetBaseCommitSHA() string
    Setup() error
    Cleanup() error
    Remove() error
    Prune() error
    IsDirty() (bool, error)
    CommitChanges(commitMsg string) error
    PushChanges(commitMsg string, open bool) error
    IsBranchCheckedOut() (bool, error)
    OpenBranchURL() error
    ComputeDiff() *git.DiffStats
    UpdateDiffStats()
    GetDiffStats() *git.DiffStats
    SetDiffStats(stats *git.DiffStats)
    ClearDiffStats()
}
```

**Acceptance Criteria**:
- `TmuxProcessManager` satisfies `TmuxManager` (verified by compile-time check:
  `var _ TmuxManager = (*TmuxProcessManager)(nil)`)
- `GitWorktreeManager` satisfies `GitManager` (same pattern)
- No behavioral changes to production code

### Task 1.2: Refactor Instance to Use Interfaces

**Files**: `session/instance.go`, `session/interfaces.go`

Change the `Instance` struct field types:
```go
type Instance struct {
    // ...
    tmuxManager TmuxManager          // was: TmuxProcessManager
    gitManager  GitManager           // was: GitWorktreeManager
    // ...
}
```

Fix direct field access patterns found in the codebase:
- `instance.gitManager.worktree` in `FromInstanceData()` (line ~344) must use
  `instance.gitManager.GetWorktree()` or `instance.gitManager.HasWorktree()`
- `i.gitManager.diffStats` in `ToInstanceData()` (line ~208) must use
  `i.gitManager.GetDiffStats()`
- `i.gitManager.worktree.GetWorktreePath()` in `Pause()` (line ~1023) must use
  `i.gitManager.GetWorktreePath()`

Update all construction sites:
- `NewInstance()` continues to use `GitWorktreeManager{}` zero-value (satisfies interface)
- `FromInstanceData()` continues to construct concrete managers, assigned to interface fields

**Acceptance Criteria**:
- `go build .` passes with zero errors
- `go test ./...` passes (all existing tests still green)
- No changes to serialization behavior (`ToInstanceData` / `FromInstanceData`)

### Task 1.3: Create Mock Implementations

**File**: `session/mock_managers_test.go` (new, test-only)

```go
type mockTmuxManager struct {
    hasSession       bool
    isAlive          bool
    doesSessionExist bool
    startErr         error
    closeErr         error
    detachErr        error
    restoreErr       error
    captureContent   string
    captureErr       error
    startCalled      bool
    closeCalled      bool
    detachCalled     bool
    restoreCalled    bool
    restoreDir       string
    // Function overrides for custom behavior per-test
    StartFunc        func(dir string) error
    CloseFunc        func() error
}

type mockGitManager struct {
    hasWorktree        bool
    worktreePath       string
    repoPath           string
    branchName         string
    isDirty            bool
    isDirtyErr         error
    setupErr           error
    removeErr          error
    pruneErr           error
    commitErr          error
    cleanupErr         error
    branchCheckedOut   bool
    branchCheckedOutErr error
    diffStats          *git.DiffStats
    setupCalled        bool
    removeCalled       bool
    pruneCalled        bool
    commitCalled       bool
    cleanupCalled      bool
    commitMsg          string
    // Function overrides for custom behavior per-test
    SetupFunc          func() error
    RemoveFunc         func() error
}
```

Each method returns the configured value, or delegates to the `XxxFunc` override if set.
Call tracking fields (e.g., `startCalled`, `restoreDir`) enable assertions about which
methods were invoked and with what arguments.

**Acceptance Criteria**:
- Both mocks satisfy their respective interfaces (compile-time assertion)
- Helper constructors: `newMockTmuxManager()`, `newMockGitManager()` with sensible defaults
- Defaults: `hasSession=true`, `isAlive=true`, `hasWorktree=true`, all errors nil

### Task 1.4: Write State Transition Tests (Happy Paths)

**File**: `session/instance_state_transition_test.go` (new)

Table-driven tests covering the primary state machine transitions:

| Test Name | Initial State | Action | Expected Final Status | Key Assertions |
|-----------|---------------|--------|----------------------|----------------|
| `TestStart_FirstTimeSetup_NewWorktree` | `Ready` (not started) | `Start(true)` | `Running` | `started==true`, `SetStatus(Running)` called, git `Setup()` called, tmux `Start()` called |
| `TestStart_FirstTimeSetup_DirectorySession` | `Ready` (not started) | `Start(true)` | `Running` | `started==true`, git `Setup()` NOT called, tmux `Start()` called |
| `TestStart_Restore_ExistingSession` | `Running` (persisted) | `Start(false)` | `Running` | tmux `RestoreWithWorkDir()` called with correct path |
| `TestPause_RunningToPaused` | `Running` (started) | `Pause()` | `Paused` | tmux `DetachSafely()` called, git `Remove()` + `Prune()` called |
| `TestPause_WithDirtyWorktree` | `Running` (started, dirty) | `Pause()` | `Paused` | git `CommitChanges()` called before `Remove()` |
| `TestResume_PausedToRunning_SessionExists` | `Paused` (started) | `Resume()` | `Running` | tmux `DoesSessionExist()==true`, tmux `RestoreWithWorkDir()` called |
| `TestResume_PausedToRunning_SessionGone` | `Paused` (started) | `Resume()` | `Running` | tmux `DoesSessionExist()==false`, tmux `Start()` called |
| `TestDestroy_RunningInstance` | `Running` (started) | `Destroy()` | - | tmux `Close()` called, git `Cleanup()` called |
| `TestDestroy_NotStarted` | `Ready` (not started) | `Destroy()` | - | Returns nil immediately, no manager methods called |
| `TestRestart_RunningInstance` | `Running` (started) | `Restart(false)` | `Running` | tmux `Close()` called then `Start()` called with correct path |

**Acceptance Criteria**:
- All 10 tests pass
- Each test constructs `Instance` with mock managers, calls the action, asserts final state
- No real tmux or git processes spawned
- Tests run in < 1 second total

### Task 1.5: Write State Transition Tests (Error and Guard Paths)

**File**: `session/instance_state_transition_test.go` (append to same file)

Table-driven tests for invalid transitions and error propagation:

| Test Name | Setup | Action | Expected Error |
|-----------|-------|--------|----------------|
| `TestStart_EmptyTitle` | `Title=""` | `Start(true)` | "instance title cannot be empty" |
| `TestPause_NotStarted` | `started=false` | `Pause()` | "cannot pause instance that has not been started" |
| `TestPause_AlreadyPaused` | `Status=Paused` | `Pause()` | "instance is already paused" |
| `TestPause_CommitFails` | dirty=true, commitErr set | `Pause()` | wraps commit error, does NOT proceed to Remove |
| `TestPause_RemoveFails` | clean worktree, removeErr set | `Pause()` | wraps remove error |
| `TestResume_NotStarted` | `started=false` | `Resume()` | "cannot resume instance that has not been started" |
| `TestResume_NotPaused` | `Status=Running` | `Resume()` | "can only resume paused instances" |
| `TestResume_BranchCheckedOut` | branchCheckedOut=true | `Resume()` | "branch is checked out" |
| `TestResume_GitSetupFails` | setupErr set | `Resume()` | wraps setup error |
| `TestResume_TmuxStartFails_CleansUpGit` | startErr set, doesSessionExist=false | `Resume()` | wraps start error, git `Cleanup()` called |
| `TestRestart_NotStarted` | `started=false` | `Restart(false)` | `ErrCannotRestart` |
| `TestRestart_Paused` | `Status=Paused` | `Restart(false)` | "session is paused" |

**Acceptance Criteria**:
- All 12 tests pass
- Error messages match production code exactly (prevents message drift)
- Cleanup actions verified (e.g., git cleanup after tmux start failure in Resume)

### Task 1.6: Write SetStatus Concurrency Test

**File**: `session/instance_state_transition_test.go` (append)

Verify thread-safe status updates under concurrent access:

```go
func TestSetStatus_ConcurrentAccess(t *testing.T) {
    inst := &Instance{started: true}
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(status Status) {
            defer wg.Done()
            inst.SetStatus(status)
        }(Status(i % 5))
    }
    wg.Wait()
    // No race detector failures = pass
}
```

**Acceptance Criteria**:
- Passes with `-race` flag: `go test ./session -run TestSetStatus_ConcurrentAccess -race`
- Validates `stateMutex` protection in `SetStatus()`

---

## Story 2: Frontend Test Setup and useSessionService Tests

**Goal**: Write comprehensive tests for the `useSessionService` hook covering session CRUD,
optimistic updates, error handling, and streaming event handling.

### Task 2.1: Create ConnectRPC Mock Transport Utility

**File**: `web-app/src/lib/test-utils/mock-transport.ts` (new)

Create a reusable mock transport for ConnectRPC that allows tests to define canned responses
per RPC method:

```typescript
import { createRouterTransport } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";

export function createMockSessionTransport(handlers: {
  listSessions?: (req: any) => any;
  getSession?: (req: any) => any;
  createSession?: (req: any) => any;
  updateSession?: (req: any) => any;
  deleteSession?: (req: any) => any;
  renameSession?: (req: any) => any;
  restartSession?: (req: any) => any;
  acknowledgeSession?: (req: any) => any;
  watchSessions?: (req: any) => AsyncGenerator<any>;
  // Review queue methods
  getReviewQueue?: (req: any) => any;
  watchReviewQueue?: (req: any) => AsyncGenerator<any>;
}) {
  return createRouterTransport(({ service }) => {
    service(SessionService, handlers);
  });
}
```

If `createRouterTransport` is not available in the installed `@connectrpc/connect@1.6.0`,
implement a manual mock by creating a transport object that intercepts `unary` and
`serverStream` calls and routes to handler functions based on method name.

**Acceptance Criteria**:
- `createMockSessionTransport({})` returns a valid transport
- Handlers can return both success responses and thrown errors
- Streaming handlers can yield multiple events
- Transport is compatible with `createPromiseClient(SessionService, transport)`

### Task 2.2: Create renderHook Test Helper

**File**: `web-app/src/lib/test-utils/render-hook-helpers.ts` (new)

Wrapper around `@testing-library/react` `renderHook` that provides common setup:

```typescript
import { renderHook, act, waitFor } from "@testing-library/react";

export { renderHook, act, waitFor };

// Helper for waiting for async operations in hooks
export async function flushPromises(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}
```

**Acceptance Criteria**:
- Exports work correctly with Jest + jsdom
- `act()` correctly batches React state updates
- `flushPromises()` allows async operations to settle

### Task 2.3: Write useSessionService CRUD Tests

**File**: `web-app/src/lib/hooks/__tests__/useSessionService.test.ts` (new)

Test cases:

| Test Name | What It Validates |
|-----------|-------------------|
| `listSessions populates sessions array` | Calls `listSessions()`, asserts `sessions` state matches response |
| `listSessions sets loading state` | Asserts `loading=true` during call, `loading=false` after |
| `listSessions handles error` | Mock throws, asserts `error` state is set, `sessions` unchanged |
| `getSession returns session by ID` | Calls `getSession('id')`, asserts return value |
| `getSession returns null on error` | Mock throws, asserts null returned, `error` set |
| `createSession adds to local state` | Calls `createSession(...)`, asserts session appended to `sessions` |
| `createSession prevents duplicate in local state` | Session already exists with same ID, asserts no duplicate added |
| `createSession throws on error` | Mock throws, asserts error propagated to caller |
| `deleteSession removes from local state` | Calls `deleteSession('id')`, asserts filtered from `sessions` |
| `deleteSession returns false on error` | Mock throws, asserts `false` returned |
| `updateSession updates local state` | Calls `updateSession('id', {...})`, asserts mapped in `sessions` |
| `pauseSession calls updateSession with PAUSED` | Asserts correct status passed to server |
| `resumeSession calls updateSession with RUNNING` | Asserts correct status passed to server |

**Acceptance Criteria**:
- All 13 tests pass
- Tests do not make real HTTP requests
- Each test is independent (no shared state between tests)

### Task 2.4: Write useSessionService Streaming Event Tests

**File**: `web-app/src/lib/hooks/__tests__/useSessionService.test.ts` (append)

Test the `handleSessionEvent` logic:

| Test Name | What It Validates |
|-----------|-------------------|
| `sessionCreated event adds session` | Streaming yields `sessionCreated`, asserts added to `sessions` |
| `sessionCreated event deduplicates` | Same ID already in `sessions`, asserts no duplicate |
| `sessionUpdated event updates session` | Streaming yields `sessionUpdated`, asserts session replaced |
| `sessionDeleted event removes session` | Streaming yields `sessionDeleted`, asserts filtered out |
| `statusChanged event updates status` | Streaming yields `statusChanged`, asserts status field updated |
| `notification event routes to callback` | Streaming yields `notification`, asserts `onNotification` called |
| `watchSessions handles abort cleanly` | Abort controller fired, asserts no error state |

**Acceptance Criteria**:
- All 7 tests pass
- Streaming mock correctly simulates async iteration
- Tests verify state AFTER React re-renders (using `waitFor` or `act`)

### Task 2.5: Write useSessionService enabled/disabled Gate Tests

**File**: `web-app/src/lib/hooks/__tests__/useSessionService.test.ts` (append)

| Test Name | What It Validates |
|-----------|-------------------|
| `enabled=false suppresses initial load` | No `listSessions` RPC call made |
| `enabled=false suppresses autoWatch` | No stream opened |
| `enabled transitions from false to true` | Initial load triggers on re-enable |

**Acceptance Criteria**:
- All 3 tests pass
- Validates the auth-gate pattern used in production

---

## Story 3: Frontend useReviewQueue and useTerminalStream Tests

**Goal**: Test the review queue optimistic update and reconnection logic, plus the terminal
stream connect/disconnect lifecycle.

### Task 3.1: Write useReviewQueue Core Tests

**File**: `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` (new)

| Test Name | What It Validates |
|-----------|-------------------|
| `refresh fetches review queue` | Calls `refresh()`, asserts `reviewQueue` populated |
| `refresh sets loading state` | Loading transitions correctly |
| `refresh handles error` | Mock throws, asserts `error` set |
| `getByPriority fetches filtered queue` | Correct filter param passed to server |
| `getByReason fetches filtered queue` | Correct filter param passed to server |
| `acknowledgeSession optimistic remove` | Item removed from `items` before server responds |
| `acknowledgeSession rollback on error` | Server fails, `refresh()` called to restore state |
| `statistics derived correctly` | `totalItems`, `byPriority`, `byReason` computed from queue |

**Acceptance Criteria**:
- All 8 tests pass
- Optimistic update test verifies item removed BEFORE async call completes
- Rollback test verifies `refresh()` called after failure

### Task 3.2: Write useReviewQueue Event Stream Tests

**File**: `web-app/src/lib/hooks/__tests__/useReviewQueue.test.ts` (append)

| Test Name | What It Validates |
|-----------|-------------------|
| `itemAdded event adds to queue` | New item appended to items array |
| `itemAdded event deduplicates by sessionId` | Same sessionId already present, not duplicated |
| `itemRemoved event removes from queue` | Item filtered out by sessionId |
| `itemUpdated event replaces item` | Matching item replaced in-place |
| `statistics event updates counts` | totalItems and priority/reason maps updated |
| `WebSocket stream error does not set error state` | Stream error logged but not surfaced |

**Acceptance Criteria**:
- All 6 tests pass
- Event handling matches the `handleReviewQueueEventRef` switch cases exactly

### Task 3.3: Write useTerminalStream Connect/Disconnect Tests

**File**: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` (new)

Focus on the lifecycle methods, NOT the binary protocol:

| Test Name | What It Validates |
|-----------|-------------------|
| `connect sets isConnected` | After `connect()`, `isConnected=true` |
| `disconnect sets isConnected false` | After `disconnect()`, `isConnected=false` |
| `connect with no sessionId is noop` | Empty sessionId, no connection attempt |
| `sendInput pushes to message queue` | Input message enqueued after connect |
| `resize pushes resize message` | Resize dimensions enqueued |
| `autoConnect=false requires manual connect` | Not connected on mount |
| `autoConnect=true connects on mount` | Connected after mount |
| `error callback invoked on stream failure` | `onError` called with error |

**Acceptance Criteria**:
- All 8 tests pass
- Terminal binary protocol is NOT tested (out of scope for unit tests)
- Tests mock the ConnectRPC bidi streaming at the transport level
- `createWebsocketBasedTransport` may need to be module-mocked if it eagerly opens connections

### Task 3.4: Write MessageQueue Unit Tests

**File**: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` (append or separate)

The `MessageQueue` class in `useTerminalStream.ts` is a standalone async iterator with
specific behavior worth testing independently:

| Test Name | What It Validates |
|-----------|-------------------|
| `push delivers to waiting consumer` | Consumer awaiting iterator gets message immediately |
| `push queues when no consumer` | Messages buffered until consumed |
| `close unblocks waiting consumer` | Iterator completes after close |
| `close stops accepting new messages` | Push after close is dropped silently |
| `sentinel messages are filtered out` | Empty sessionId messages not yielded to consumer |

**Acceptance Criteria**:
- All 5 tests pass
- `MessageQueue` may need to be exported or extracted to a separate file for direct testing;
  alternatively, test through the hook's `sendInput` behavior

---

## Known Issues and Bug Risks

### Potential Bug: Race Condition in Instance.StartController [SEVERITY: Medium]

**Description**: `StartController()` drops and re-acquires `stateMutex` between checking
preconditions and storing the controller (lines 1773-1829 of `instance.go`). The double-check
pattern on line 1819 mitigates this, but a concurrent `StopController()` call between the
`Unlock()` on line 1800 and `Lock()` on line 1815 could cause the newly created controller
to be registered after a stop was requested.

**Mitigation in this plan**:
- Task 1.6 adds a concurrency test for `SetStatus` to validate mutex usage
- The StartController race is documented but NOT fixed in this plan (requires architectural
  change to controller lifecycle management)
- Recommend adding a `controllerStarting` flag in a future PR

**Files affected**: `/Users/tylerstapler/IdeaProjects/stapler-squad/session/instance.go` lines 1773-1829

### Potential Bug: Direct Field Access Bypasses Interface [SEVERITY: High]

**Description**: Three locations in `instance.go` access `gitManager` struct fields directly
rather than through methods:
- `FromInstanceData()` line ~344: `instance.gitManager.worktree != nil`
- `ToInstanceData()` line ~208: `i.gitManager.diffStats != nil`
- `Pause()` line ~1023: `i.gitManager.worktree.GetWorktreePath()`

After Task 1.2 changes the field type to an interface, these direct accesses will cause
compile errors (interfaces do not expose struct fields). This is actually beneficial -- the
compiler will force us to fix them to use interface methods.

**Mitigation**: Task 1.2 explicitly calls out these locations and requires using existing
interface methods (`HasWorktree()`, `GetDiffStats()`, `GetWorktreePath()`).

**Files affected**: `/Users/tylerstapler/IdeaProjects/stapler-squad/session/instance.go`

### Potential Bug: Jest jsdom Missing WebSocket [SEVERITY: Medium]

**Description**: `useTerminalStream` creates a WebSocket-based transport via
`createWebsocketBasedTransport`. The `jsdom` environment does not provide a real WebSocket
implementation. If the transport constructor eagerly opens a connection, tests will fail
with `WebSocket is not defined`.

**Mitigation**:
- Mock the transport at the ConnectRPC level (not the WebSocket level)
- Add `global.WebSocket = jest.fn()` to `jest.setup.js` if needed
- Module-mock `@/lib/transport/websocket-transport` in terminal stream tests

**Files affected**: `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

### Potential Bug: Flaky Tests from Timing-Dependent Terminal Output [SEVERITY: Low]

**Description**: If future tests attempt to verify terminal output content, they may be
flaky due to the adaptive batching in `useTerminalStream` (requestAnimationFrame-based
flushing with 4KB threshold). This plan intentionally avoids testing terminal output
content in unit tests.

**Mitigation**: Terminal output correctness is covered by existing E2E Playwright tests
in `web-app/tests/e2e/`. Unit tests focus on connection lifecycle only.

### Potential Bug: ConnectRPC createRouterTransport Availability [SEVERITY: Medium]

**Description**: The `createRouterTransport` utility for testing may not be available in
`@connectrpc/connect@1.6.0`. The API was introduced in later versions of the library.

**Mitigation**:
- Task 2.1 includes a fallback: implement a manual mock transport if
  `createRouterTransport` is not available
- The manual mock intercepts the transport's `unary()` and `serverStream()` methods
  and routes to test handlers based on method name
- Check availability before implementing: `npm ls @connectrpc/connect`

**Files affected**: `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/test-utils/mock-transport.ts`

### Potential Bug: Interface Change Breaks Existing Tests [SEVERITY: Low]

**Description**: Existing tests in `session/instance_test.go` directly construct `Instance`
structs with `gitManager: GitWorktreeManager{...}` literal syntax. After changing the field
type to an interface, these tests still compile because `GitWorktreeManager` satisfies the
`GitManager` interface. However, any test accessing the concrete struct fields through the
interface will fail.

**Mitigation**: Review all existing test files that construct `Instance` structs and verify
they compile after the interface change. The existing tests in `instance_test.go` use
`gitManager: GitWorktreeManager{worktree: ...}` which assigns a concrete struct to an
interface field -- this is valid Go and will continue to work.

**Files affected**: `/Users/tylerstapler/IdeaProjects/stapler-squad/session/instance_test.go`

---

## Testing Strategy

### Go Backend

**Test pyramid position**: Unit tests (base of pyramid)

**Test execution**:
```bash
# Run only state transition tests
go test ./session -run TestStart_ -v
go test ./session -run TestPause_ -v
go test ./session -run TestResume_ -v
go test ./session -run TestDestroy_ -v
go test ./session -run TestRestart_ -v

# Run with race detector
go test ./session -run "TestSetStatus_ConcurrentAccess" -race -v

# Run with coverage
go test ./session -cover -coverprofile=coverage.out
go tool cover -func=coverage.out | grep instance.go
```

**Test pattern**: Table-driven tests with sub-tests (`t.Run`), following the established
pattern in `session/status_mapping_test.go` and `session/test_helpers_test.go`.

### Frontend

**Test pyramid position**: Unit tests (base of pyramid)

**Test execution**:
```bash
cd web-app

# Run all hook tests
npx jest src/lib/hooks/__tests__/ --verbose

# Run with coverage
npx jest src/lib/hooks/__tests__/ --coverage

# Watch mode during development
npx jest src/lib/hooks/__tests__/ --watch
```

**Test pattern**: `renderHook` + `act` + `waitFor` from `@testing-library/react`, following
the established pattern of isolated tests with mock transports.

---

## Dependency Graph

```
Story 1 (Go Backend)
  Task 1.1: Define interfaces ───────────────┐
  Task 1.2: Refactor Instance fields ────────┤ (depends on 1.1)
  Task 1.3: Create mock implementations ─────┤ (depends on 1.1)
  Task 1.4: Happy path tests ───────────────┤ (depends on 1.2 + 1.3)
  Task 1.5: Error/guard path tests ──────────┤ (depends on 1.2 + 1.3)
  Task 1.6: Concurrency test ───────────────┘ (depends on 1.2)

Story 2 (Frontend: useSessionService)         [independent of Story 1]
  Task 2.1: Mock transport utility ──────────┐
  Task 2.2: renderHook helpers ──────────────┤
  Task 2.3: CRUD tests ─────────────────────┤ (depends on 2.1 + 2.2)
  Task 2.4: Streaming event tests ───────────┤ (depends on 2.1 + 2.2)
  Task 2.5: enabled/disabled gate tests ─────┘ (depends on 2.1 + 2.2)

Story 3 (Frontend: useReviewQueue + useTerminalStream)
  Task 3.1: useReviewQueue core tests ───────┐ (depends on 2.1)
  Task 3.2: useReviewQueue event tests ──────┤ (depends on 2.1)
  Task 3.3: useTerminalStream lifecycle ─────┤ (depends on 2.1)
  Task 3.4: MessageQueue unit tests ─────────┘ (independent)
```

**Parallelism**: Story 1 and Story 2 can be worked in parallel by different developers.
Story 3 depends on Story 2's mock transport utility (Task 2.1) but can begin Task 3.4
(MessageQueue tests) independently.

**Critical path**: Task 1.1 -> Task 1.2 -> Task 1.4 (Go backend) is the longest dependency
chain because the interface refactor touches production code and must be validated before
mocks and tests can be written.
