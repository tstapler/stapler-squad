# Instance God Object Decomposition

**Date**: 2026-03-18
**Status**: Draft
**Scope**: Decompose `session/instance.go` (2,077 lines, 70+ methods) into 5 focused structs
**Prerequisite**: None -- three manager structs already extracted, this completes the pattern
**Reference**: [Architecture Refactor Plan](architecture-refactor.md) ADR-009

---

## Table of Contents

- [Executive Summary](#executive-summary)
- [ADR-011: Decomposition Strategy -- Embedded Structs with Delegation](#adr-011-decomposition-strategy----embedded-structs-with-delegation)
- [ADR-012: Backward Compatibility -- Stable Public API via Delegation](#adr-012-backward-compatibility----stable-public-api-via-delegation)
- [ADR-013: Thread Safety -- Shared stateMutex, No Sub-Manager Locks](#adr-013-thread-safety----shared-statemutex-no-sub-manager-locks)
- [Story 1: Extract ReviewStateManager](#story-1-extract-reviewstatemanager)
- [Story 2: Extract TagManager](#story-2-extract-tagmanager)
- [Story 3: Extract VCSInterface](#story-3-extract-vcsinterface)
- [Story 4: Extract TerminalInterface](#story-4-extract-terminalinterface)
- [Story 5: Extract GitHubMetadata as Value Object](#story-5-extract-githubmetadata-as-value-object)
- [Story 6: Slim Down Instance and Clean Up](#story-6-slim-down-instance-and-clean-up)
- [Known Issues](#known-issues)
- [Dependency Graph](#dependency-graph)
- [Verification Checklist](#verification-checklist)

---

## Executive Summary

`session/instance.go` is the largest file in the codebase at 2,077 lines with 70+ methods spanning
6 unrelated domains. Three manager structs have already been extracted following a delegation pattern:

| Already Extracted | File | Lines |
|-------------------|------|-------|
| `TmuxProcessManager` | `session/tmux_process_manager.go` | 235 |
| `GitWorktreeManager` | `session/git_worktree_manager.go` | 182 |
| `ControllerManager` | `session/controller_manager.go` | 71 |

Additionally, `ReviewState` exists as an embedded struct in `session/review_state.go` (198 lines)
but its methods are still accessed via field promotion on `Instance` rather than through explicit
delegation.

This plan completes the decomposition by extracting 5 remaining domain clusters into focused structs,
reducing `instance.go` from ~2,077 lines to ~1,150 lines (lifecycle + metadata + delegation wrappers).

**What stays on Instance**: Title, Path, WorkingDir, Branch, Status, Program, CreatedAt, UpdatedAt,
SessionType, TmuxPrefix, TmuxServerSocket, InstanceType, IsManaged, Permissions, ExternalMetadata,
plus lifecycle methods (Start, Pause, Resume, Kill, Destroy, Restart) that coordinate sub-managers.

**What gets extracted**: ReviewState delegation wrappers, Tag CRUD, GitHub field accessors,
VCS/diff delegation, and terminal I/O delegation.

---

## ADR-011: Decomposition Strategy -- Embedded Structs with Delegation

**Status**: Proposed

**Context**: The existing extractions (`TmuxProcessManager`, `GitWorktreeManager`,
`ControllerManager`) use composition with unexported fields on `Instance`:

```go
type Instance struct {
    tmuxManager       TmuxProcessManager
    gitManager        GitWorktreeManager
    controllerManager ControllerManager
    ReviewState                         // embedded (promoted fields)
}
```

`ReviewState` is the outlier: it uses Go struct embedding, which promotes its fields directly
onto `Instance`. This means `inst.LastMeaningfulOutput` compiles and is used in 20+ call sites
across `review_queue_poller.go`, `server/dependencies.go`, `server/adapters/instance_adapter.go`,
and `server/review_queue_manager.go`.

**Decision**: Use **composition with delegation wrappers** for all new extractions, matching the
pattern of `TmuxProcessManager`, `GitWorktreeManager`, and `ControllerManager`. For `ReviewState`,
keep the existing embedding but add explicit delegation methods where callers currently reach
through the promoted fields, as a first step toward full encapsulation in a future pass.

**Rationale**:
- Composition with delegation is the established pattern in this codebase (3 prior extractions).
- Go embedding with promotion is convenient but leaks internal structure -- callers depend on
  the struct layout. However, changing 20+ `inst.LastMeaningfulOutput` references across 5 files
  in one shot risks merge conflicts and review burden.
- For new extractions (Tags, GitHub, VCS, Terminal), delegation wrappers are clean because
  the public method signatures already exist on `Instance` -- we just move the implementation.

**Consequences**:
- `TagManager`, `VCSInterface`, `TerminalInterface` become composed structs (unexported fields).
- `GitHubMetadata` becomes a value object (exported struct, no methods on Instance needed).
- `ReviewState` embedding is preserved but wrapper methods are added for the most commonly
  accessed fields, enabling future encapsulation.
- New code should use delegation methods, not promoted fields.

---

## ADR-012: Backward Compatibility -- Stable Public API via Delegation

**Status**: Proposed

**Context**: `Instance` methods are called from:
- `server/adapters/instance_adapter.go` -- Reads 25+ fields for proto conversion
- `server/services/session_service.go` -- Session CRUD lifecycle
- `server/dependencies.go` -- Review queue item construction
- `session/review_queue_poller.go` -- Timestamp reads, acknowledgment checks
- `server/review_queue_manager.go` -- Direct field writes (`inst.LastUserResponse`)
- `session/storage.go` -- Serialization via `ToInstanceData()`/`FromInstanceData()`

**Decision**: Every extracted method retains its existing signature on `Instance` as a
one-line delegation wrapper. No caller outside `session/` needs to change.

**Example**:
```go
// Before (in instance.go, 10 lines):
func (i *Instance) AddTag(tag string) {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    for _, existingTag := range i.Tags {
        if existingTag == tag { return }
    }
    i.Tags = append(i.Tags, tag)
}

// After (in instance.go, 3 lines):
func (i *Instance) AddTag(tag string) {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    i.tagManager.Add(tag)
}
```

**Rationale**:
- Zero breaking changes to callers.
- Each story can be merged independently without coordinating cross-package changes.
- Delegation wrappers can be removed in a future pass when callers migrate to sub-managers.

**Consequences**:
- `instance.go` retains ~30-40 thin wrapper methods (3 lines each = ~120 lines).
- Callers can optionally access sub-managers directly for new code: `inst.TagManager()`.
- The adapter (`instance_adapter.go`) continues reading exported fields directly for now.

---

## ADR-013: Thread Safety -- Shared stateMutex, No Sub-Manager Locks

**Status**: Proposed

**Context**: The existing architecture uses a single `sync.RWMutex` (`stateMutex`) on `Instance`
to protect all mutable state. The already-extracted managers (`TmuxProcessManager`,
`GitWorktreeManager`, `ControllerManager`) do NOT have their own mutexes -- they are always
accessed under `Instance.stateMutex` or from single-threaded lifecycle methods.

The architecture-refactor.md ADR-009 originally proposed giving each sub-manager its own lock.
However, the actual implementation diverged: no sub-manager has a lock, and this has worked
correctly in production.

**Decision**: Continue using the shared `Instance.stateMutex` for all sub-manager state. New
extractions (`TagManager`, `VCSInterface`, `TerminalInterface`, `GitHubMetadata`) will NOT
have their own mutexes. The delegation wrappers on `Instance` acquire `stateMutex` as needed.

**Rationale**:
- Consistent with the 3 existing extractions (no sub-manager locks).
- Eliminates lock-ordering deadlock risk entirely (Risk C1 from architecture-refactor.md).
- The `ReviewState` comment explicitly states: "Fields are protected by Instance.stateMutex;
  do not lock ReviewState independently."
- Performance is not a concern: lock contention is minimal because operations are short-lived.

**Consequences**:
- Sub-manager structs are pure data + logic, no synchronization primitives.
- All concurrency control remains on `Instance` delegation wrappers.
- Future optimization (per-domain locks) remains possible but is not needed now.
- `ReviewState` methods that say "caller must hold stateMutex" continue to work unchanged.

---

## Story 1: Extract ReviewStateManager

**Goal**: Formalize the `ReviewState` extraction by adding proper delegation methods to
`Instance` and moving the remaining review-related methods off `Instance` into `ReviewState`.

**Rationale**: `ReviewState` already exists as an embedded struct. Two methods
(`UpdateTerminalTimestamps`, `detectAndTrackPrompt`) still live on `Instance` because they
bridge tmux content filtering with review state updates. This story moves those methods
into `ReviewState` and adds delegation wrappers on `Instance`.

### Task 1.1: Move `UpdateTerminalTimestamps` logic into ReviewState

**Current state** (instance.go lines 1891-1912):
```go
func (i *Instance) UpdateTerminalTimestamps(content string, forceUpdate bool) {
    filteredContent := content
    shouldUpdateMeaningful := false
    if i.tmuxManager.HasSession() {
        // ... banner filtering via tmuxManager
    }
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    i.ReviewState.UpdateTimestamps(content, filteredContent, shouldUpdateMeaningful, i.Title)
}
```

**Change**: The tmux-dependent pre-processing (banner filtering, meaningful content detection)
is already delegated through `tmuxManager`. Keep the `Instance` method as a thin coordinator
that:
1. Calls `tmuxManager.FilterBanners()`/`HasMeaningfulContent()` (no lock needed, read-only)
2. Acquires `stateMutex`
3. Calls `ReviewState.UpdateTimestamps()` (already exists)

This is effectively what it does today. The task is to verify the boundary is clean and
document it. No functional change needed -- the method stays on `Instance` as a coordinator.

**Files modified**: `session/instance.go` (add comment clarifying this is a coordinator method)

**Tests**: Existing `session/instance_timestamp_test.go` and
`session/instance_timestamp_signature_test.go` continue to pass.

### Task 1.2: Move `GetTimeSince*` methods to pure delegation

**Current state** (instance.go lines 1916-1928):
```go
func (i *Instance) GetTimeSinceLastMeaningfulOutput() time.Duration {
    i.stateMutex.RLock()
    defer i.stateMutex.RUnlock()
    return i.ReviewState.TimeSinceLastMeaningfulOutput(i.CreatedAt)
}
```

**Change**: These are already delegation wrappers. Verify they are the ONLY callers of the
underlying `ReviewState` methods from outside the `session` package. Add doc comments
noting the delegation pattern.

**Files modified**: `session/instance.go` (documentation only)

### Task 1.3: Add accessor methods for direct field access patterns

**Problem**: `review_queue_poller.go` accesses `inst.LastMeaningfulOutput`,
`inst.LastAcknowledged`, `inst.LastAddedToQueue`, `inst.ProcessingGraceUntil` directly
through Go field promotion. `server/review_queue_manager.go` writes
`inst.LastUserResponse = time.Now()` directly. `server/adapters/instance_adapter.go`
reads `inst.LastTerminalUpdate` and `inst.LastMeaningfulOutput`.

**Change**: For this story, do NOT break these access patterns. Instead, add a comment block
at the top of `ReviewState` documenting which callers access promoted fields directly and
why this is acceptable (all within `session` package or through `Instance.stateMutex`-protected
code paths). Add a `// TODO: Migrate to accessor methods` comment for the cross-package
accesses in `server/`.

**Files modified**: `session/review_state.go` (documentation)

**Acceptance criteria**:
- `go build ./...` passes
- `go test ./...` passes
- No functional changes -- this story is about documenting and verifying the existing boundary

---

## Story 2: Extract TagManager

**Goal**: Extract the 5 tag methods (`AddTag`, `RemoveTag`, `HasTag`, `GetTags`, `SetTags`)
into a `TagManager` struct in a new file.

**Rationale**: Pure data manipulation with no I/O, no tmux, no git dependencies. The simplest
extraction target. Tags are stored as `[]string` on Instance and serialized in `ToInstanceData()`.

### Task 2.1: Create `session/tag_manager.go`

**Change**: Create a new file with the `TagManager` struct:

```go
// session/tag_manager.go
package session

// TagManager provides CRUD operations for session tags.
// It is a pure data structure with no I/O or external dependencies.
// Thread safety is provided by Instance.stateMutex -- callers must hold
// the lock when calling TagManager methods.
type TagManager struct {
    tags *[]string // points to Instance.Tags for zero-sync compatibility
}

// NewTagManager creates a TagManager backed by the given slice pointer.
func NewTagManager(tags *[]string) TagManager {
    return TagManager{tags: tags}
}

// Add adds a tag if it does not already exist.
func (tm *TagManager) Add(tag string) {
    for _, existing := range *tm.tags {
        if existing == tag {
            return
        }
    }
    *tm.tags = append(*tm.tags, tag)
}

// Remove removes a tag by value.
func (tm *TagManager) Remove(tag string) {
    newTags := make([]string, 0, len(*tm.tags))
    for _, existing := range *tm.tags {
        if existing != tag {
            newTags = append(newTags, existing)
        }
    }
    *tm.tags = newTags
}

// Has returns true if the tag exists.
func (tm *TagManager) Has(tag string) bool {
    for _, existing := range *tm.tags {
        if existing == tag {
            return true
        }
    }
    return false
}

// All returns a copy of the tag slice.
func (tm *TagManager) All() []string {
    result := make([]string, len(*tm.tags))
    copy(result, *tm.tags)
    return result
}

// Set replaces all tags with a new set.
func (tm *TagManager) Set(tags []string) {
    newTags := make([]string, len(tags))
    copy(newTags, tags)
    *tm.tags = newTags
}
```

**Files created**: `session/tag_manager.go`

### Task 2.2: Create `session/tag_manager_test.go`

**Change**: Unit tests for all `TagManager` methods in isolation (no `Instance` needed):
- `TestTagManager_Add` -- add new tag, add duplicate tag (idempotent)
- `TestTagManager_Remove` -- remove existing, remove non-existent
- `TestTagManager_Has` -- positive and negative
- `TestTagManager_All` -- returns copy, not reference (mutation safety)
- `TestTagManager_Set` -- replaces all, empty set
- Verify that mutations through `TagManager` are visible via the backing `*[]string`

**Files created**: `session/tag_manager_test.go`

### Task 2.3: Wire `TagManager` into `Instance`

**Change**: Add `tagManager TagManager` field to `Instance`. Initialize in `NewInstance()` and
`FromInstanceData()` using `NewTagManager(&instance.Tags)`.

The `Tags []string` field on `Instance` remains as a public field for backward compatibility
with `instance_adapter.go` and `review_queue_poller.go` which read `inst.Tags` directly.
Because `TagManager` stores a `*[]string` pointer to `Instance.Tags`, mutations through
`TagManager` are automatically visible via `inst.Tags`.

**Files modified**: `session/instance.go` (add field, update `NewInstance`, `FromInstanceData`)

### Task 2.4: Update delegation wrappers on Instance

**Change**: Replace the 5 tag method implementations with delegation:

```go
func (i *Instance) AddTag(tag string) {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    i.tagManager.Add(tag)
}

func (i *Instance) RemoveTag(tag string) {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    i.tagManager.Remove(tag)
}

func (i *Instance) HasTag(tag string) bool {
    i.stateMutex.RLock()
    defer i.stateMutex.RUnlock()
    return i.tagManager.Has(tag)
}

func (i *Instance) GetTags() []string {
    i.stateMutex.RLock()
    defer i.stateMutex.RUnlock()
    return i.tagManager.All()
}

func (i *Instance) SetTags(tags []string) {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    i.tagManager.Set(tags)
}
```

**Files modified**: `session/instance.go`

**Acceptance criteria**:
- All 5 Instance tag methods delegate to TagManager
- `inst.Tags` remains accessible for serialization and adapter code
- `go test ./...` passes with no changes to callers

**Net line reduction**: ~35 lines removed from `instance.go`, ~80 lines added in
`tag_manager.go` + test file. Total codebase impact: +45 lines (investment in testability).

---

## Story 3: Extract VCSInterface

**Goal**: Extract the git diff stats update logic into `GitWorktreeManager` and simplify
the complex lock-upgrade pattern in `Instance.UpdateDiffStats()`.

**Rationale**: `UpdateDiffStats()` is the most complex remaining method on `Instance` (60 lines)
with a read-lock-to-write-lock upgrade pattern. Moving the I/O-heavy logic into
`GitWorktreeManager` simplifies `instance.go` and makes the lock upgrade pattern testable.

### Task 3.1: Move diff computation I/O into GitWorktreeManager

**Change**: Add `ComputeDiffIfReady()` to `GitWorktreeManager`:

```go
// session/git_worktree_manager.go

// ComputeDiffIfReady checks if the worktree path exists and computes a new diff.
// Returns (stats, needsPause) where needsPause is true if the worktree dir is missing.
// This method performs I/O and should be called WITHOUT holding Instance.stateMutex.
func (gm *GitWorktreeManager) ComputeDiffIfReady() (stats *git.DiffStats, needsPause bool) {
    if gm.worktree == nil {
        return nil, false
    }
    worktreePath := gm.worktree.GetWorktreePath()
    if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
        return nil, true
    }
    result := gm.worktree.Diff()
    return result, false
}
```

**Files modified**: `session/git_worktree_manager.go`

### Task 3.2: Simplify `Instance.UpdateDiffStats()` to coordinator

**Change**: Rewrite `UpdateDiffStats` to use the new method:

```go
func (i *Instance) UpdateDiffStats() error {
    i.stateMutex.RLock()
    if !i.started {
        i.gitManager.ClearDiffStats()
        i.stateMutex.RUnlock()
        return nil
    }
    if i.Status == Paused {
        i.stateMutex.RUnlock()
        return nil
    }
    if !i.gitManager.HasWorktree() {
        i.gitManager.ClearDiffStats()
        i.stateMutex.RUnlock()
        return nil
    }
    i.stateMutex.RUnlock()

    // I/O outside lock
    stats, needsPause := i.gitManager.ComputeDiffIfReady()

    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()

    if needsPause {
        if i.Status != Paused {
            log.WarningLog.Printf("Worktree directory for '%s' doesn't exist, marking as paused", i.Title)
            i.Status = Paused
        }
        i.gitManager.ClearDiffStats()
        return nil
    }
    if stats != nil && stats.Error != nil {
        if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
            i.gitManager.ClearDiffStats()
            return nil
        }
        return fmt.Errorf("failed to get diff stats: %w", stats.Error)
    }
    i.gitManager.SetDiffStats(stats)
    return nil
}
```

**Files modified**: `session/instance.go`

**Net line reduction**: ~20 lines moved to `git_worktree_manager.go`.

### Task 3.3: Add tests for `ComputeDiffIfReady`

**Change**: Test the missing-directory case and the happy path:
- Create temp directory, set up GitWorktreeManager with mock worktree
- Call `ComputeDiffIfReady()` -- verify stats returned, `needsPause=false`
- Remove directory, call again -- verify `needsPause=true`

**Files modified**: `session/git_worktree_manager_test.go` (add tests)

### Task 3.4: Document remaining VCS delegation wrappers

**Change**: Add section comment in `instance.go` for VCS delegation methods. These are
already 3-5 line wrappers and do not need further extraction:
- `RepoName()`, `HasGitWorktree()`, `GetGitWorktree()`, `GetWorkingDirectory()`,
  `GetEffectiveRootDir()`, `GetDiffStats()`

**Files modified**: `session/instance.go` (documentation only)

---

## Story 4: Extract TerminalInterface

**Goal**: Move non-trivial terminal logic from `Instance` into `TmuxProcessManager` and
document the remaining delegation wrappers.

**Rationale**: Terminal I/O methods are the largest cluster (~200 lines) remaining on `Instance`.
Most are 3-5 line delegation wrappers. Two contain non-trivial logic that belongs in
`TmuxProcessManager`.

### Task 4.1: Move viewport capture logic into TmuxProcessManager

**Current state**: `GetCurrentPaneContent` (instance.go lines 1345-1366) computes line count
from pane dimensions, then delegates.

**Change**: Add `CaptureViewport(lines int)` to `TmuxProcessManager`:

```go
// session/tmux_process_manager.go

// CaptureViewport captures the last N lines of the pane.
// If lines <= 0, captures the current viewport height.
func (tm *TmuxProcessManager) CaptureViewport(lines int) (string, error) {
    if tm.session == nil {
        return "", fmt.Errorf("tmux session not initialized")
    }
    if lines <= 0 {
        _, height, err := tm.session.GetPaneDimensions()
        if err != nil {
            lines = 40 // Fallback
        } else {
            lines = height
        }
    }
    startLine := fmt.Sprintf("-%d", lines)
    return tm.session.CapturePaneContentWithOptions(startLine, "-")
}
```

Then `Instance.GetCurrentPaneContent` becomes a 4-line delegation wrapper.

**Files modified**: `session/tmux_process_manager.go`, `session/instance.go`

### Task 4.2: Move SendPrompt multi-step logic into TmuxProcessManager

**Current state**: `SendPrompt` (instance.go lines 1464-1482) sends keys, sleeps 100ms,
then taps enter.

**Change**: Add `SendPromptWithEnter(prompt string) error` to `TmuxProcessManager`:

```go
// session/tmux_process_manager.go

// SendPromptWithEnter sends text to the session followed by Enter key.
// Includes a brief pause between text and Enter to prevent interpretation issues.
func (tm *TmuxProcessManager) SendPromptWithEnter(prompt string) error {
    if tm.session == nil {
        return fmt.Errorf("tmux session not initialized")
    }
    if _, err := tm.session.SendKeys(prompt); err != nil {
        return fmt.Errorf("error sending keys to tmux session: %w", err)
    }
    time.Sleep(100 * time.Millisecond)
    if err := tm.session.TapEnter(); err != nil {
        return fmt.Errorf("error tapping enter: %w", err)
    }
    return nil
}
```

Then `Instance.SendPrompt` becomes a 5-line delegation wrapper.

**Files modified**: `session/tmux_process_manager.go`, `session/instance.go`

### Task 4.3: Document remaining terminal delegation wrappers

**Change**: Add section header in `instance.go`:

```go
// ---- Terminal I/O delegation ------------------------------------------------
// The following methods delegate to TmuxProcessManager with started/paused
// guards and stateMutex protection. They preserve the public Instance API
// while keeping terminal logic in TmuxProcessManager.
```

Annotate each method with `// Delegates to tmuxManager.XYZ`.

**Files modified**: `session/instance.go` (documentation only)

**Net line reduction**: ~27 lines moved from `instance.go` to `tmux_process_manager.go`.

---

## Story 5: Extract GitHubMetadata as Value Object

**Goal**: Extract the 6 GitHub fields and 5 GitHub helper methods into a `GitHubMetadata`
value object struct.

**Rationale**: The GitHub fields (`GitHubPRNumber`, `GitHubPRURL`, `GitHubOwner`, `GitHubRepo`,
`GitHubSourceRef`, `ClonedRepoPath`) form a cohesive group with dedicated accessor methods.
They are purely metadata with no I/O, no mutex needs, and no external dependencies.

### Task 5.1: Create `session/github_metadata.go`

**Change**: Create value object struct:

```go
// session/github_metadata.go
package session

import "fmt"

// GitHubMetadataView is a read-only value object for GitHub session metadata.
// Constructed by Instance.GitHub() from the underlying fields.
// This is intentionally a value type (not a pointer) for safe concurrent reads.
type GitHubMetadataView struct {
    PRNumber       int
    PRURL          string
    Owner          string
    Repo           string
    SourceRef      string
    ClonedRepoPath string
}

// IsPRSession returns true if this metadata represents a PR-based session.
func (gh GitHubMetadataView) IsPRSession() bool {
    return gh.PRNumber > 0
}

// RepoFullName returns "owner/repo" format, or empty string.
func (gh GitHubMetadataView) RepoFullName() string {
    if gh.Owner == "" || gh.Repo == "" {
        return ""
    }
    return fmt.Sprintf("%s/%s", gh.Owner, gh.Repo)
}

// PRDisplayInfo returns human-readable PR description for UI display.
func (gh GitHubMetadataView) PRDisplayInfo() string {
    if !gh.IsPRSession() {
        return ""
    }
    return fmt.Sprintf("PR #%d on %s", gh.PRNumber, gh.RepoFullName())
}

// IsGitHubSession returns true if owner and repo are both set.
func (gh GitHubMetadataView) IsGitHubSession() bool {
    return gh.Owner != "" && gh.Repo != ""
}

// IsEmpty returns true if no GitHub metadata is set.
func (gh GitHubMetadataView) IsEmpty() bool {
    return gh.PRNumber == 0 && gh.PRURL == "" && gh.Owner == "" && gh.Repo == ""
}
```

**Files created**: `session/github_metadata.go`

### Task 5.2: Create `session/github_metadata_test.go`

**Tests**:
- `TestGitHubMetadataView_IsPRSession` -- PR number > 0 vs 0
- `TestGitHubMetadataView_RepoFullName` -- Both set, one missing, both missing
- `TestGitHubMetadataView_PRDisplayInfo` -- PR session vs non-PR
- `TestGitHubMetadataView_IsGitHubSession` -- Both set vs partial
- `TestGitHubMetadataView_IsEmpty` -- All empty vs any set

**Files created**: `session/github_metadata_test.go`

### Task 5.3: Wire `GitHubMetadataView` into Instance

**Change**: Keep the 6 individual fields on `Instance` with their original names for backward
compatibility. Add a `GitHub()` method that returns a populated value object:

```go
// GitHub returns a read-only view of the GitHub metadata for this instance.
func (i *Instance) GitHub() GitHubMetadataView {
    return GitHubMetadataView{
        PRNumber:       i.GitHubPRNumber,
        PRURL:          i.GitHubPRURL,
        Owner:          i.GitHubOwner,
        Repo:           i.GitHubRepo,
        SourceRef:      i.GitHubSourceRef,
        ClonedRepoPath: i.ClonedRepoPath,
    }
}
```

Replace the 4 helper method bodies with delegation:

```go
func (i *Instance) IsPRSession() bool             { return i.GitHub().IsPRSession() }
func (i *Instance) GetGitHubRepoFullName() string  { return i.GitHub().RepoFullName() }
func (i *Instance) GetPRDisplayInfo() string       { return i.GitHub().PRDisplayInfo() }
func (i *Instance) IsGitHubSession() bool          { return i.GitHub().IsGitHubSession() }
```

**Files modified**: `session/instance.go`

### Task 5.4: Document `DetectAndPopulateWorktreeInfo` field writes

**Change**: `DetectAndPopulateWorktreeInfo` writes to `i.GitHubOwner` and `i.GitHubRepo`
directly. Add a comment noting the write path and that a future pass could route writes
through a setter method.

**Files modified**: `session/instance.go` (documentation only)

**Net line reduction**: ~30 lines moved from `instance.go` to `github_metadata.go`.

---

## Story 6: Slim Down Instance and Clean Up

**Goal**: Final cleanup pass to organize `instance.go` into clear sections and verify
the overall line count reduction.

### Task 6.1: Organize `instance.go` into commented sections

**Change**: Add section headers to group the remaining code:

```
// ==== Instance -- Core Fields and Construction ====
// (Instance struct, NewInstance, NewInstanceWithCleanup, FromInstanceData, ToInstanceData)

// ==== Lifecycle Methods ====
// (Start, Pause, Resume, Kill, Destroy, Restart, and their internal helpers)

// ==== Terminal I/O Delegation ====
// (delegates to TmuxProcessManager)

// ==== VCS/Git Delegation ====
// (delegates to GitWorktreeManager)

// ==== Review State Delegation ====
// (delegates to embedded ReviewState)

// ==== Tag Management Delegation ====
// (delegates to TagManager)

// ==== GitHub Metadata ====
// (delegates to GitHubMetadataView value object)

// ==== Controller Delegation ====
// (delegates to ControllerManager)

// ==== Claude Session Management ====
// (handleClaudeSessionReattachment and related helpers)
```

**Files modified**: `session/instance.go`

### Task 6.2: Verify line count targets

**Expected outcome** after all stories:

| Section | Approx Lines | Notes |
|---------|-------------|-------|
| Struct + construction | 350 | Instance struct, NewInstance, FromInstanceData, ToInstanceData |
| Lifecycle | 350 | Start, Pause, Resume, Kill, Destroy, Restart (complex coordination) |
| Terminal delegation | 120 | ~20 thin wrapper methods, 6 lines each avg |
| VCS delegation | 80 | UpdateDiffStats (simplified), 5 other wrappers |
| Review state | 30 | 2 coordinator methods + 2 time-since wrappers |
| Tag delegation | 25 | 5 wrapper methods, 4 lines each |
| GitHub delegation | 25 | 5 wrapper methods + GitHub() constructor |
| Controller delegation | 50 | StartController, StopController + wrappers |
| Claude session mgmt | 120 | handleClaudeSessionReattachment and helpers |
| **Total** | **~1,150** | **Down from 2,077 (~44% reduction)** |

New files added:

| File | Lines | Tests |
|------|-------|-------|
| `session/tag_manager.go` | ~70 | `session/tag_manager_test.go` (~80 lines) |
| `session/github_metadata.go` | ~55 | `session/github_metadata_test.go` (~70 lines) |
| Additions to `tmux_process_manager.go` | ~30 | Via existing test file |
| Additions to `git_worktree_manager.go` | ~20 | Via existing test file |

### Task 6.3: Run full validation

```bash
go build ./...
go test ./...
go vet ./...
make quick-check
make restart-web  # Manual smoke test
```

---

## Known Issues

### Bug 1: Direct Field Access to ReviewState Promoted Fields [SEVERITY: Medium]

**Description**: 20+ call sites across `review_queue_poller.go`, `server/dependencies.go`,
`server/adapters/instance_adapter.go`, and `server/review_queue_manager.go` access ReviewState
fields through Go embedding promotion (e.g., `inst.LastMeaningfulOutput` instead of
`inst.ReviewState.LastMeaningfulOutput`). This tight coupling makes it impossible to replace
the embedding with a composed field without updating all callers.

**Files Affected**:
- `session/review_queue_poller.go` (lines 229, 272, 304, 313, 324, 327, 343, 345, 347, 351, 362, 364, 634, 640, 679, 687, 689, 713, 722, 758, 810, 812, 820)
- `server/dependencies.go` (lines 195, 316, 385, 386)
- `server/adapters/instance_adapter.go` (lines 35, 36)
- `server/review_queue_manager.go` (lines 171, 173, 177)

**Mitigation**: Story 1 documents these access patterns without changing them. A future pass
can introduce accessor methods and update callers in a single batch. This is intentionally
deferred to keep each story independently mergeable.

**Prevention**: New code should use `Instance.ReviewState.XYZ()` method calls, not direct
field access.

### Bug 2: Unsynchronized ReviewState Field Writes from server/ [SEVERITY: Medium]

**Description**: `server/review_queue_manager.go` line 171 writes `inst.LastUserResponse = time.Now()`
directly without holding `Instance.stateMutex`. This is a data race if any other goroutine reads
`LastUserResponse` concurrently (which `review_queue_poller.go` does via `UserRespondedAfterPrompt()`).

**Files Affected**:
- `server/review_queue_manager.go` (line 171)

**Mitigation**: Add a `SetLastUserResponse(t time.Time)` method on `Instance` that acquires
`stateMutex`. Update the call site in `review_queue_manager.go`. This fix is independent of
the decomposition and can be done in any story.

**Prevention**: Enforce that all field mutations go through `Instance` methods. Consider making
`ReviewState` fields unexported in a future pass.

### Bug 3: TagManager Pointer vs Value Semantics [SEVERITY: Low]

**Description**: If `TagManager` stores its own `[]string` (value semantics), it must be synced
with `Instance.Tags` which is read directly by `instance_adapter.go` and
`review_queue_poller.go`. If `TagManager` stores a `*[]string` pointer to `Instance.Tags`,
the sync is automatic but the pointer indirection is unusual for Go.

**Files Affected**:
- `session/instance.go`
- `server/adapters/instance_adapter.go` (line 33)
- `session/review_queue_poller.go` (line 776)
- `server/dependencies.go` (lines 192, 313, 382)

**Mitigation**: Story 2 uses the pointer approach (`*[]string`), which eliminates sync issues.
Document the pointer semantics in `TagManager` struct comments. Callers that read `inst.Tags`
directly see the same data as `inst.GetTags()`.

**Prevention**: Future pass can make `Tags` unexported and route all access through `TagManager`.

### Bug 4: ToInstanceData/FromInstanceData Serialization Drift [SEVERITY: Medium]

**Description**: `ToInstanceData()` and `FromInstanceData()` are the serialization boundary.
As fields move into sub-managers, these methods must continue to read/write all fields correctly.
If a new field is added to `GitHubMetadata` but not to `ToInstanceData`, it silently drops data.

**Files Affected**:
- `session/instance.go` (ToInstanceData, FromInstanceData)
- `session/storage.go`

**Mitigation**: After each story, run the existing `session/storage_test.go` round-trip test.
Add a test that serializes an Instance with all fields set and verifies none are lost:

```go
func TestToInstanceDataRoundTrip(t *testing.T) {
    // Create instance with ALL fields populated
    // Serialize via ToInstanceData
    // Reconstruct via FromInstanceData
    // Assert field-by-field equality
}
```

**Prevention**: Add a compile-time struct size assertion or use `reflect.DeepEqual` in a
test to catch new fields that lack serialization code.

### Bug 5: Race in HasUpdated UpdateTerminalTimestamps Path [SEVERITY: Low]

**Description**: `Instance.HasUpdated()` calls `tmuxManager.HasUpdated()` without holding
`stateMutex`, then calls `UpdateTerminalTimestamps()` which acquires `stateMutex`. Between
the two calls, another goroutine could modify the review state. This is a pre-existing condition
that the decomposition does not worsen but should be documented.

**Files Affected**:
- `session/instance.go` (lines 897-920)

**Mitigation**: Document the race window in a code comment. The practical impact is minimal
because the worst case is a slightly stale timestamp, not data corruption.

---

## Dependency Graph

```
Story 1: ReviewStateManager (documentation + boundary verification)
    |
    v (can proceed in parallel with Story 2)
Story 2: TagManager (pure data, no dependencies)
    |
    v (can proceed in parallel with Story 3)
Story 3: VCSInterface (depends on GitWorktreeManager understanding from Story 1/2)
    |
    v
Story 4: TerminalInterface (depends on TmuxProcessManager understanding from Story 3)
    |
    v (can proceed in parallel with Story 5)
Story 5: GitHubMetadata (pure data, no dependencies on Stories 3-4)
    |
    v
Story 6: Final cleanup (depends on all above)
```

**Recommended execution order**: Stories 1 and 2 are independent and can be done in parallel.
Stories 3 and 4 should be sequential (both modify the same files). Story 5 is independent.
Story 6 must be last.

**Minimum viable merge order**: 2 -> 5 -> 3 -> 4 -> 1 -> 6 (sorted by risk, lowest first)

---

## Verification Checklist (per story)

After completing each story, verify:

- [ ] `go build ./...` succeeds with no errors
- [ ] `go test ./...` passes (including flaky tests -- rerun 3 times)
- [ ] `go vet ./...` reports no new issues
- [ ] `make quick-check` passes
- [ ] No new linter warnings from `make lint` (if available)
- [ ] `instance.go` line count has decreased (or remained stable for doc-only stories)
- [ ] New test file exists for extracted struct (Stories 2 and 5)
- [ ] No callers outside `session/` package have been modified
- [ ] `ToInstanceData()` / `FromInstanceData()` round-trip test passes
- [ ] `make restart-web` starts successfully
- [ ] Create session, add tags, view diff stats, view terminal -- all work via web UI