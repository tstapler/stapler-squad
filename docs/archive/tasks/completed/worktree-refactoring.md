# Git Worktree Operations Refactoring

## Epic Overview

**Goal**: Refactor git worktree operations to be robust, maintainable, and thread-safe while following SOLID principles and Go best practices.

**Value Proposition**:
- Eliminate race conditions and resource leaks in session management
- Improve error handling and recovery from partial failures
- Reduce code duplication and maintenance burden
- Enable better testing and observability

**Success Metrics**:
- Zero race conditions detected by `go test -race`
- 100% idempotent cleanup operations (can be called multiple times safely)
- Reduced code duplication by >60% in worktree operations
- All operations properly logged with context
- Comprehensive error wrapping with actionable messages

**Current Pain Points**:
- Race condition in `Setup()` parallel operations
- Non-idempotent cleanup causing "directory not empty" errors
- Extensive code duplication in removal logic
- Silent error swallowing in cleanup paths
- No concurrency control for concurrent session operations

---

## Story 1: Fix Critical Race Conditions and Concurrency Issues (1 week)

**Objective**: Eliminate race conditions and add proper concurrency control to prevent data corruption and undefined behavior.

**Value**: Prevents session creation failures and data corruption in concurrent scenarios.

**Dependencies**: None (foundational)

### Task 1.1: Add Mutex-Based Concurrency Control (3h) - Medium

**Scope**: Add per-worktree mutex locking to prevent concurrent operations on the same worktree.

**Files**:
- `session/git/worktree.go` - Add mutex field and locking methods
- `session/git/worktree_ops.go` - Wrap operations with lock/unlock
- `session/git/worktree_ops_test.go` - Add concurrency tests

**Context Needed**:
- Understanding of `GitWorktree` struct lifecycle
- Go sync.Mutex patterns and defer unlock
- Current setup/cleanup operation flows

**Implementation**:
```go
type GitWorktree struct {
    mu           sync.Mutex
    repoPath     string
    branchName   string
    worktreePath string
    // ... existing fields
}

func (g *GitWorktree) Setup() error {
    g.mu.Lock()
    defer g.mu.Unlock()
    // ... existing setup logic
}
```

**Success Criteria**:
- `go test -race` passes on all worktree operations
- Concurrent Setup() calls serialize properly
- Lock is always released (defer pattern)

**Testing**:
```go
func TestConcurrentSetup(t *testing.T) {
    wg := sync.WaitGroup{}
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // Test concurrent setup
        }()
    }
    wg.Wait()
}
```

**Dependencies**: None

---

### Task 1.2: Fix Race Condition in Setup() Parallel Operations (2h) - Small

**Scope**: Replace unsafe parallel goroutines in `Setup()` with sequential operations or proper channel-based synchronization.

**Files**:
- `session/git/worktree_ops.go` - Remove parallel goroutines or add proper sync
- `session/git/worktree_ops_test.go` - Add race detection test

**Context Needed**:
- Lines 23-52 in `worktree_ops.go` (parallel directory creation + branch check)
- Understanding why optimization was attempted
- Performance implications of sequential approach

**Current Issue**:
```go
// UNSAFE: branchExists is accessed from goroutine without sync
go func() {
    if _, err := repo.Reference(branchRef, false); err == nil {
        branchExists = true  // RACE CONDITION
    }
    errChan <- nil
}()
```

**Fixed Approach**:
```go
// Safe sequential approach
if err := os.MkdirAll(worktreesDir, 0755); err != nil {
    return fmt.Errorf("failed to create worktrees directory: %w", err)
}

repo, err := git.PlainOpen(g.repoPath)
if err != nil {
    return fmt.Errorf("failed to open repository: %w", err)
}

branchRef := plumbing.NewBranchReferenceName(g.branchName)
branchExists := false
if _, err := repo.Reference(branchRef, false); err == nil {
    branchExists = true
}
```

**Success Criteria**:
- No race conditions detected by race detector
- Setup() performance degradation <10ms
- All error paths properly handled

**Testing**:
```bash
go test -race -run TestSetup ./session/git/
```

**Dependencies**: Task 1.1 (mutex in place)

---

### Task 1.3: Add Global Worktree Registry for Conflict Detection (4h) - Large

**Scope**: Create a global registry to track active worktrees and prevent conflicts across goroutines/sessions.

**Files**:
- `session/git/registry.go` - New file for global registry
- `session/git/worktree.go` - Integrate with registry
- `session/git/worktree_ops.go` - Check registry before operations
- `session/git/registry_test.go` - New test file

**Context Needed**:
- How sessions are created concurrently
- Worktree path generation logic
- Potential conflicts between sessions

**Implementation**:
```go
// registry.go
package git

import "sync"

type worktreeRegistry struct {
    mu       sync.RWMutex
    active   map[string]*GitWorktree
}

var globalRegistry = &worktreeRegistry{
    active: make(map[string]*GitWorktree),
}

func (r *worktreeRegistry) Register(path string, wt *GitWorktree) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if _, exists := r.active[path]; exists {
        return fmt.Errorf("worktree already active at %s", path)
    }
    r.active[path] = wt
    return nil
}

func (r *worktreeRegistry) Unregister(path string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    delete(r.active, path)
}
```

**Success Criteria**:
- Concurrent session creation prevented on same path
- Registry automatically cleaned on worktree cleanup
- No memory leaks (entries removed on cleanup)

**Testing**:
- Test concurrent registration on same path (should fail)
- Test cleanup removes registry entry
- Test registry survives process-level operations

**Dependencies**: Task 1.1, Task 1.2

---

## Story 2: Make Cleanup Operations Idempotent (4 days)

**Objective**: Ensure all cleanup operations can be safely retried and handle partial failure states.

**Value**: Eliminates "directory not empty" errors and allows safe retry on failures.

**Dependencies**: Story 1 (concurrency control)

### Task 2.1: Extract Common Cleanup Logic (2h) - Small

**Scope**: Extract duplicated worktree removal code into a single reusable function.

**Files**:
- `session/git/worktree_ops.go` - Extract `removeWorktreeDirectory()` helper
- Refactor: `Cleanup()`, `Remove()`, `setupFromExistingBranch()`, `setupNewWorktree()`

**Context Needed**:
- Lines 192-222 in `Cleanup()` - worktree removal logic
- Lines 280-313 in `Remove()` - similar worktree removal
- Duplication patterns across cleanup methods

**Implementation**:
```go
// removeWorktreeDirectory safely removes a worktree directory with fallbacks
func (g *GitWorktree) removeWorktreeDirectory() error {
    // Check if directory exists
    if _, err := os.Stat(g.worktreePath); os.IsNotExist(err) {
        log.InfoLog.Printf("Worktree directory already removed: %s", g.worktreePath)
        return nil // Idempotent
    }

    // Try git worktree remove first
    if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
        log.WarningLog.Printf("Git worktree remove failed, trying manual removal: %v", err)

        // Fallback to manual removal
        if rmErr := os.RemoveAll(g.worktreePath); rmErr != nil {
            return fmt.Errorf("failed to remove worktree directory: git failed (%w), manual failed (%w)", err, rmErr)
        }
    }

    log.InfoLog.Printf("Successfully removed worktree directory: %s", g.worktreePath)
    return nil
}
```

**Success Criteria**:
- All cleanup methods use shared `removeWorktreeDirectory()`
- Code duplication reduced by >60%
- Operation is idempotent (can call multiple times)

**Testing**:
```go
func TestRemoveWorktreeDirectory_Idempotent(t *testing.T) {
    // First call removes
    err := wt.removeWorktreeDirectory()
    require.NoError(t, err)

    // Second call succeeds (idempotent)
    err = wt.removeWorktreeDirectory()
    require.NoError(t, err)
}
```

**Dependencies**: Story 1 complete

---

### Task 2.2: Implement Idempotent Branch Cleanup (3h) - Medium

**Scope**: Make `cleanupExistingBranch()` safe to call multiple times and handle missing branches gracefully.

**Files**:
- `session/git/worktree_branch.go` - Refactor `cleanupExistingBranch()`
- `session/git/worktree_branch_test.go` - Add idempotency tests

**Context Needed**:
- Lines 11-44 in `worktree_branch.go` - current cleanup logic
- go-git `RemoveReference()` behavior on missing refs
- Relationship between branches and worktrees

**Current Issue**:
```go
// Not idempotent - fails if reference doesn't exist
if err := repo.Storer.RemoveReference(branchRef); err != nil && err != plumbing.ErrReferenceNotFound {
    return fmt.Errorf("failed to remove branch reference %s: %w", g.branchName, err)
}
```

**Improved Implementation**:
```go
func (g *GitWorktree) cleanupExistingBranch(repo *git.Repository) error {
    branchRef := plumbing.NewBranchReferenceName(g.branchName)

    // Check if branch exists first (idempotent)
    if _, err := repo.Reference(branchRef, false); err == plumbing.ErrReferenceNotFound {
        log.InfoLog.Printf("Branch %s already removed, skipping cleanup", g.branchName)
        return nil // Idempotent
    }

    // Try to remove the branch reference
    if err := repo.Storer.RemoveReference(branchRef); err != nil {
        // Only fail on non-NotFound errors
        if err != plumbing.ErrReferenceNotFound {
            return fmt.Errorf("failed to remove branch reference %s: %w", g.branchName, err)
        }
    }

    log.InfoLog.Printf("Successfully cleaned up branch %s", g.branchName)
    return nil
}
```

**Success Criteria**:
- Operation succeeds on already-cleaned branches
- No "directory not empty" errors
- Proper logging of idempotent skips

**Testing**:
```go
func TestCleanupExistingBranch_Idempotent(t *testing.T) {
    // First cleanup
    err := wt.cleanupExistingBranch(repo)
    require.NoError(t, err)

    // Second cleanup should succeed
    err = wt.cleanupExistingBranch(repo)
    require.NoError(t, err)
}
```

**Dependencies**: Task 2.1

---

### Task 2.3: Add Transaction-Like Setup with Rollback (4h) - Large

**Scope**: Implement setup operations with automatic rollback on failure to prevent partial states.

**Files**:
- `session/git/worktree_ops.go` - Add `setupWithRollback()` wrapper
- `session/git/worktree_ops_test.go` - Test rollback scenarios

**Context Needed**:
- Current setup flow in `Setup()`, `setupNewWorktree()`, `setupFromExistingBranch()`
- Partial failure scenarios (branch created but worktree failed)
- Go defer patterns for cleanup

**Implementation**:
```go
func (g *GitWorktree) Setup() error {
    g.mu.Lock()
    defer g.mu.Unlock()

    // Track what we've created for rollback
    var branchCreated, worktreeCreated bool

    // Rollback on error
    defer func() {
        if r := recover(); r != nil || (err != nil && (branchCreated || worktreeCreated)) {
            log.WarningLog.Printf("Setup failed, rolling back changes")
            g.rollbackSetup(branchCreated, worktreeCreated)
        }
    }()

    // ... setup logic with tracking

    return nil
}

func (g *GitWorktree) rollbackSetup(branchCreated, worktreeCreated bool) {
    if worktreeCreated {
        _ = g.removeWorktreeDirectory()
    }
    if branchCreated {
        repo, _ := git.PlainOpen(g.repoPath)
        if repo != nil {
            _ = g.cleanupExistingBranch(repo)
        }
    }
}
```

**Success Criteria**:
- Failed setup leaves no orphaned resources
- Rollback is logged for debugging
- Partial states are prevented

**Testing**:
- Test setup failure at each stage (directory, branch, worktree)
- Verify no orphaned branches/directories after failure
- Test recovery from transient failures

**Dependencies**: Task 2.1, Task 2.2

---

## Story 3: Improve Error Handling and Observability (3 days)

**Objective**: Add comprehensive error wrapping, context, and logging for debugging and monitoring.

**Value**: Easier troubleshooting of session failures and better operational visibility.

**Dependencies**: Story 1, Story 2

### Task 3.1: Add Structured Error Types (2h) - Small

**Scope**: Create typed errors for common failure scenarios with context.

**Files**:
- `session/git/errors.go` - New file with error types
- `session/git/worktree_ops.go` - Use typed errors
- `session/git/worktree_branch.go` - Use typed errors

**Implementation**:
```go
// errors.go
package git

import "fmt"

type WorktreeError struct {
    Op      string // Operation that failed
    Path    string
    Branch  string
    Err     error
}

func (e *WorktreeError) Error() string {
    return fmt.Sprintf("worktree %s failed for branch %s at %s: %v", e.Op, e.Branch, e.Path, e.Err)
}

func (e *WorktreeError) Unwrap() error {
    return e.Err
}

// Typed error constructors
func SetupError(path, branch string, err error) *WorktreeError {
    return &WorktreeError{Op: "setup", Path: path, Branch: branch, Err: err}
}

func CleanupError(path, branch string, err error) *WorktreeError {
    return &WorktreeError{Op: "cleanup", Path: path, Branch: branch, Err: err}
}
```

**Success Criteria**:
- All operations return typed errors
- Errors include operation context
- Unwrap chain works with errors.Is/As

**Testing**:
```go
func TestErrorTypes(t *testing.T) {
    err := SetupError("/path", "branch", io.EOF)
    require.ErrorIs(t, err, io.EOF)
    require.Contains(t, err.Error(), "setup")
}
```

**Dependencies**: Story 2 complete

---

### Task 3.2: Add Comprehensive Logging with Context (3h) - Medium

**Scope**: Add structured logging throughout worktree operations with consistent context fields.

**Files**:
- `session/git/worktree_ops.go` - Add logging to all operations
- `session/git/worktree_branch.go` - Add logging to cleanup
- Update existing log statements with context

**Context Needed**:
- Current logging patterns in codebase
- Log levels and when to use each
- Performance implications of verbose logging

**Implementation**:
```go
func (g *GitWorktree) Setup() error {
    log.InfoLog.Printf("[worktree] Starting setup: branch=%s path=%s", g.branchName, g.worktreePath)

    // ... operations with logging

    log.InfoLog.Printf("[worktree] Setup completed successfully: branch=%s path=%s", g.branchName, g.worktreePath)
    return nil
}

func (g *GitWorktree) logOperation(op string, fields map[string]interface{}) {
    log.InfoLog.Printf("[worktree] %s: branch=%s path=%s %v", op, g.branchName, g.worktreePath, fields)
}
```

**Success Criteria**:
- Every operation entry/exit logged
- All errors logged before returning
- Context fields consistent across logs

**Testing**:
- Manually verify log output for setup/cleanup flows
- Check logs contain all required context

**Dependencies**: Task 3.1

---

### Task 3.3: Add Operation Metrics and Timing (2h) - Small

**Scope**: Add timing metrics for operations to identify performance bottlenecks.

**Files**:
- `session/git/worktree_ops.go` - Add timing wrappers
- `session/git/metrics.go` - New file for metrics collection

**Implementation**:
```go
func (g *GitWorktree) Setup() error {
    start := time.Now()
    defer func() {
        duration := time.Since(start)
        log.InfoLog.Printf("[worktree] Setup completed in %v: branch=%s", duration, g.branchName)
    }()

    // ... existing setup logic
}
```

**Success Criteria**:
- All major operations log duration
- Slow operations (>5s) logged as warnings
- Metrics available for monitoring

**Dependencies**: Task 3.2

---

## Story 4: Architectural Refactoring (1-2 weeks) - Optional/Future

**Objective**: Implement clean architecture with layered separation of concerns.

**Value**: Long-term maintainability, testability, and extensibility.

**Dependencies**: Stories 1-3 complete

**Note**: This is a strategic refactoring that can be done incrementally over time. Not required for immediate stability improvements.

### Task 4.1: Create Git Repository Abstraction Layer (4h) - Large

**Scope**: Extract git operations behind an interface for better testing.

**Files**:
- `session/git/repository.go` - New repository interface
- `session/git/repository_impl.go` - go-git implementation
- `session/git/repository_mock.go` - Mock for testing
- `session/git/worktree_ops.go` - Use interface instead of direct go-git

**Dependencies**: Stories 1-3

---

### Task 4.2: Implement Service Layer Pattern (4h) - Large

**Scope**: Create a service layer that coordinates worktree lifecycle operations.

**Files**:
- `session/git/service.go` - New service layer
- `session/git/worktree_ops.go` - Delegate to service
- `session/git/service_test.go` - Service layer tests

**Dependencies**: Task 4.1

---

## Dependency Visualization

```
Story 1 (Concurrency)
├─ Task 1.1: Mutex Control [START] ────┐
├─ Task 1.2: Fix Race in Setup    <────┤
└─ Task 1.3: Global Registry      <────┴─ (after 1.1 & 1.2)

Story 2 (Idempotency)                    <─ (after Story 1)
├─ Task 2.1: Extract Cleanup Logic [START]
├─ Task 2.2: Idempotent Branch    <────┤
└─ Task 2.3: Transaction/Rollback <────┴─ (after 2.1 & 2.2)

Story 3 (Observability)                  <─ (after Story 1 & 2)
├─ Task 3.1: Error Types [START]
├─ Task 3.2: Structured Logging   <────┤
└─ Task 3.3: Metrics/Timing       <────┘

Story 4 (Architecture)                   <─ (optional, after Story 1-3)
├─ Task 4.1: Repository Interface
└─ Task 4.2: Service Layer
```

### Parallel Execution Opportunities

**Phase 1** (Can run in parallel after Story 1):
- Story 2 (Idempotency fixes)
- Story 3 (Observability improvements)

**Phase 2** (Sequential):
- Story 4 requires completion of all previous stories

---

## Context Preparation Guide

### For Story 1 Tasks (Concurrency):
**Required Reading**:
1. `session/git/worktree.go` - Struct definition
2. `session/git/worktree_ops.go` - Setup() method (lines 15-58)
3. Go sync package documentation
4. Understanding of defer and panic/recover

**Required Understanding**:
- How GitWorktree is instantiated
- Current setup flow and parallel operations
- Go race detector basics

### For Story 2 Tasks (Idempotency):
**Required Reading**:
1. `session/git/worktree_ops.go` - Cleanup methods
2. `session/git/worktree_branch.go` - Branch cleanup
3. Current error when resuming paused sessions

**Required Understanding**:
- Worktree lifecycle (create → use → cleanup)
- Branch lifecycle and relationship to worktrees
- Idempotency patterns in Go

### For Story 3 Tasks (Observability):
**Required Reading**:
1. `log/log.go` - Current logging infrastructure
2. Error handling patterns in existing codebase
3. Performance considerations for logging

**Required Understanding**:
- When to use Info vs Warning vs Error logs
- How to structure error types in Go
- Context propagation patterns

---

## Integration Checkpoints

### Checkpoint 1: After Story 1
**Validation**:
- Run `go test -race ./session/git/` - must pass
- Test concurrent session creation - no crashes
- Verify mutex overhead <1ms per operation

### Checkpoint 2: After Story 2
**Validation**:
- Test cleanup multiple times on same worktree - all succeed
- Test resume on paused session - no errors
- Verify no orphaned branches or directories after failures

### Checkpoint 3: After Story 3
**Validation**:
- Review logs for setup/cleanup flow - complete context present
- Check error messages are actionable
- Verify metrics show operation durations

### Final Integration Test
**Validation**:
- Create 100 sessions concurrently - all succeed
- Pause and resume sessions - all succeed
- Force-kill during setup - cleanup handles partial state
- Run for 1 hour - no memory leaks or resource exhaustion

---

## Success Criteria (Overall)

- [ ] Zero race conditions (`go test -race` passes)
- [ ] All cleanup operations idempotent
- [ ] Code duplication reduced >60%
- [ ] All operations have structured logging
- [ ] Error messages include full context
- [ ] Operation timing metrics available
- [ ] Comprehensive test coverage (>80%)
- [ ] Documentation updated for new patterns

---

## Implementation Order Recommendation

**Week 1 - Critical Fixes**:
1. Task 1.1: Mutex control (3h)
2. Task 1.2: Fix race in Setup (2h)
3. Task 2.1: Extract cleanup logic (2h)
4. Task 2.2: Idempotent branch cleanup (3h)

**Week 2 - Stability & Observability**:
5. Task 1.3: Global registry (4h)
6. Task 2.3: Transaction/rollback (4h)
7. Task 3.1: Error types (2h)
8. Task 3.2: Structured logging (3h)

**Week 3 - Polish & Optional**:
9. Task 3.3: Metrics (2h)
10. Story 4 tasks (optional, if time permits)

**Total Effort**: ~29 hours (2-3 weeks with testing and review)
