# Architecture Refactor Plan

**Date**: 2026-02-26
**Score Baseline**: 5.5/10 (from architecture review)
**Target Score**: 8.0/10
**Scope**: Incremental refactoring across 6 stories, each leaving the app in a working state

---

## Table of Contents

- [ADR-008: Storage System Consolidation](#adr-008-storage-system-consolidation)
- [ADR-009: Instance Decomposition Strategy](#adr-009-instance-decomposition-strategy)
- [ADR-010: SessionService Splitting Strategy](#adr-010-sessionservice-splitting-strategy)
- [Story 1: Quick Wins](#story-1-quick-wins)
- [Story 2: Storage Consolidation](#story-2-storage-consolidation)
- [Story 3: Instance Decomposition](#story-3-instance-decomposition)
- [Story 4: SessionService Splitting](#story-4-sessionservice-splitting)
- [Story 5: Package Structure](#story-5-package-structure)
- [Story 6: Type Safety](#story-6-type-safety)
- [Known Issues and Bug Risks](#known-issues-and-bug-risks)
- [Dependency Graph](#dependency-graph)

---

## ADR-008: Storage System Consolidation

**Status**: Proposed
**Context**: Three parallel storage systems are active simultaneously:

1. `config/state.go` -- JSON file-based legacy storage (`state.json`, `instances.json`).
   Used by `Storage` via `config.InstanceStorage` interface. All runtime reads/writes
   go through this path today.
2. `session/sqlite_repository.go` -- Raw SQL against SQLite with hand-written schema
   migrations (`sqlite_schema.go`, `MigrateV1ToV2`, `MigrateV2ToV3`).
3. `session/ent_repository.go` -- Ent ORM with auto-migration, type-safe queries,
   and a normalized schema (`session/ent/schema/`).

Both `SQLiteRepository` and `EntRepository` implement the `Repository` interface,
which itself defines two parallel method families: `InstanceData`-based methods
(`Create`, `Update`, `Get`, `List`) and `Session`-based methods (`CreateSession`,
`UpdateSession`, `GetSession`, `ListSessions`). Neither repository is wired into
the production hot path today -- `Storage` still delegates to `config.InstanceStorage`
(JSON).

The `InstanceData` and `Session` types duplicate most of the same domain information.
Adapter functions (`InstanceToSession`, `SessionToInstance`) exist in `session/session.go`
but add a maintenance burden and risk data loss on round-trip (e.g., `CloudContext`
has no `Instance` equivalent).

**Decision**: Consolidate on `EntRepository` as the single storage backend and
`Session` as the canonical domain model.

**Rationale**:
- Ent provides type-safe queries, automatic migrations, and a normalized schema
  that already matches the `Session` context model (`GitContext`, `FilesystemContext`,
  etc.).
- The raw `SQLiteRepository` duplicates Ent's purpose with hand-maintained SQL
  that is harder to evolve.
- The JSON backend (`config/state.go` `InstanceStorage`) was appropriate for
  early prototyping but does not support indexed queries, concurrent access, or
  selective loading.
- `Session` with optional contexts (`ContextOptions`) is a cleaner domain model
  than the flat `InstanceData` struct. The context-based loading is already
  implemented in `context_options.go`.

**Consequences**:
- `sqlite_repository.go`, `sqlite_schema.go`, `sqlite_state.go`,
  `sqlite_diagnostics.go` will be deleted.
- JSON-to-Ent migration path (`migrate.go`) will be preserved as a one-time
  upgrade tool, then deprecated.
- `config/state.go` `InstanceStorage` interface will be narrowed to UI state only
  (help screens, category expansion, search mode). Session data moves to Ent.
- `InstanceData` will become a read-only backward-compatibility alias, eventually
  removed. New code must use `Session`.
- `Storage` struct will be rewritten to wrap `EntRepository` instead of
  `config.InstanceStorage`.

**Migration Strategy**:
1. Wire `EntRepository` as the backing store for `Storage` behind a feature flag
   (`STAPLER_SQUAD_USE_ENT=true`, default false).
2. Implement dual-write: JSON + Ent, read from Ent when flag is on.
3. Validate with integration tests that round-trip `Session` data through Ent.
4. Flip the default to true. Deprecate JSON path.
5. Delete `SQLiteRepository`, `sqlite_schema.go`, `sqlite_state.go`,
   `sqlite_diagnostics.go`.
6. Delete `InstanceData`-based `Repository` methods once all callers migrated.

---

## ADR-009: Instance Decomposition Strategy

**Status**: Proposed
**Context**: `session/instance.go` is 2,342 lines with 35+ fields and 80+ methods
spanning five unrelated concerns:

| Concern | Fields | Methods |
|---------|--------|---------|
| Session metadata | Title, Path, Branch, Status, Program, Tags, Category, CreatedAt/UpdatedAt, GitHub fields | ToInstanceData, SetTitle, SetStatus, RepoName, IsPRSession, GetGitHubRepoFullName |
| Git worktree management | gitWorktree, diffStats, IsWorktree, MainRepoPath | Start (worktree setup), CleanupWorktree, UpdateDiffStats, GetDiffStats, DetectAndPopulateWorktreeInfo |
| Tmux process management | tmuxSession, started, Height/Width | Start (tmux setup), Kill, Destroy, Pause, Resume, Attach, Preview, SetPreviewSize, SendKeys, CapturePaneContent, GetPTYReader, WriteToPTY, ResizePTY |
| Review queue state | reviewQueue, LastAcknowledged, LastAddedToQueue, LastTerminalUpdate, LastMeaningfulOutput, LastOutputSignature, LastViewed, LastPromptDetected, LastPromptSignature, LastUserResponse, ProcessingGraceUntil | NeedsReview, GetReviewItem, UpdateTerminalTimestamps, detectAndTrackPrompt, computePromptSignature |
| Controller/status | statusManager, claudeController, claudeSession | StartController, StopController, GetController, handleClaudeSessionReattachment |

This violates the Single Responsibility Principle. Changes to review queue logic
require touching the same file as changes to tmux integration. The large surface
area makes testing slow and reasoning about correctness difficult.

**Decision**: Extract four focused structs that `Instance` delegates to:

1. **`GitWorktreeManager`** -- Owns `gitWorktree`, `diffStats`, worktree lifecycle
   (create, cleanup, detect), and diff operations.
2. **`TmuxProcessManager`** -- Owns `tmuxSession`, `started`, terminal dimensions,
   PTY access, preview, attach/detach, pause/resume, send keys.
3. **`ReviewState`** -- Owns all review queue timestamps, prompt detection/tracking,
   output signatures, and the `NeedsReview` predicate.
4. **`ControllerManager`** -- Owns `claudeController`, `statusManager`,
   `claudeSession`, controller lifecycle.

`Instance` retains session metadata fields (Title, Path, Branch, Status, etc.)
and embeds or composes the four extracted structs. Public methods on `Instance`
delegate to the appropriate manager, preserving the existing API surface during
migration.

**Rationale**:
- Composition over inheritance. Each extracted struct can be tested independently.
- The delegation pattern preserves backward compatibility: callers still call
  `instance.Preview()`, which delegates to `instance.tmux.Preview()`.
- Future work can replace the delegation with direct access to sub-managers
  where appropriate.

**Consequences**:
- New files: `session/git_worktree_manager.go`, `session/tmux_process_manager.go`,
  `session/review_state.go`, `session/controller_manager.go`.
- `instance.go` shrinks from ~2,342 to ~600 lines (metadata + delegation).
- Tests for extracted structs can be written and run independently.
- Serialization (`ToInstanceData`) remains on `Instance` but reads from sub-managers.
- The `stateMutex` on `Instance` will need careful handling: each sub-manager
  may need its own lock for its state, while `Instance`-level operations that
  span multiple managers need the existing lock or a coordinator pattern.

---

## ADR-010: SessionService Splitting Strategy

**Status**: Proposed
**Context**: `server/services/session_service.go` is 2,921 lines with 40+ RPC
handler methods covering:

| Concern | Methods | Line Range |
|---------|---------|------------|
| Session CRUD | ListSessions, GetSession, CreateSession, UpdateSession, DeleteSession, RenameSession, RestartSession | 162-535, 2381-2620 |
| Terminal streaming | StreamTerminal (stub, actual impl in connectrpc_websocket.go) | 612-905 |
| Review queue | GetReviewQueue, AcknowledgeSession, WatchReviewQueue, LogUserInteraction | 965-1440 |
| Search & history | ListClaudeHistory, GetClaudeHistoryDetail, GetClaudeHistoryMessages, SearchClaudeHistory | 1539-1873 |
| GitHub/PR operations | GetPRInfo, GetPRComments, PostPRComment, MergePR, ClosePR | 1874-2140 |
| Notifications | SendNotification, FocusWindow | 2141-2380 |
| Config | GetClaudeConfig, ListClaudeConfigs, UpdateClaudeConfig | 1441-1538 |
| Workspace | GetVCSStatus, GetWorkspaceInfo, ListWorkspaceTargets, SwitchWorkspace | 2519-2921 |

**Decision**: Extract domain-focused service objects that `SessionService`
delegates to. Do NOT split the ConnectRPC handler registration -- keep a single
`SessionService` that implements the generated `SessionServiceHandler` interface.
Instead, create internal service objects:

1. **`TerminalStreamService`** -- Terminal streaming logic currently in
   `connectrpc_websocket.go`. Already partially separated.
2. **`ReviewQueueService`** -- Review queue CRUD, watch streams, acknowledgment,
   and user interaction logging.
3. **`SearchService`** -- Claude history listing, detail, messages, search.
4. **`GitHubService`** -- PR info, comments, merge, close.
5. **`WorkspaceService`** -- VCS status, workspace info, targets, switch.

`SessionService` becomes a thin facade: each RPC method delegates to the
appropriate internal service with 1-3 lines of glue code.

**Rationale**:
- ConnectRPC generates a single handler interface (`SessionServiceHandler`).
  Splitting into multiple ConnectRPC services would require proto changes and
  client-side migration. That is out of scope for this refactor.
- Internal service objects can be independently tested with mock dependencies.
- The facade pattern keeps the public API unchanged while reducing per-file
  complexity.

**Consequences**:
- New files under `server/services/`: `review_queue_service.go`,
  `search_service.go`, `github_service.go`, `workspace_service.go`.
- `session_service.go` shrinks from ~2,921 to ~400-500 lines (CRUD + delegation).
- `connectrpc_websocket.go` may be refactored but stays in `server/services/`.
- Each internal service receives only the dependencies it needs (not the full
  `SessionService` dependency bag).

---

## Story 1: Quick Wins

**Goal**: Fix P2 issues and low-risk improvements that build confidence in the
refactoring process. Each task is independently mergeable.

**Estimated effort**: 1 week

### Task 1.1: Remove debug `fmt.Printf` in production adapter

**File**: `server/adapters/review_queue_adapter.go:94`

**Change**: Replace the `fmt.Printf` on line 94 with a structured log call using
the project's `log` package, or remove it entirely if the debug information is
no longer needed.

```go
// Before (line 94):
fmt.Printf("[ReviewQueueAdapter] Serializing statistics: ...")

// After:
log.DebugLog.Printf("[ReviewQueueAdapter] Serializing statistics: ...")
```

**Tests**: Run `go test ./server/adapters/...` -- no behavior change expected.

**Risk**: Minimal. Pure logging change.

### Task 1.2: Add `fmt.Printf` linter rule to CI

**Change**: Add a `go vet` check or `staticcheck` rule that flags `fmt.Print*`
calls in non-test, non-main files. This prevents future production debug prints.

Options:
- Add `forbidigo` linter to `golangci-lint` config with rule: `fmt\\.Print.*`
- Or add a Makefile target: `make no-print-in-prod`

**Files**: `.golangci.yml` (create or update), `Makefile`

**Risk**: May flag intentional `fmt.Print` in CLI output paths. Needs allowlist
for `main.go` and test files.

### Task 1.3: Document initialization order in `server/server.go`

**Change**: Replace the 5 "CRITICAL ORDER" comments with a single block comment
at the top of `NewServer` that documents the initialization dependency graph:

```
// Initialization Order (dependencies flow downward):
//
//   1. SessionService (creates Storage, EventBus, ReviewQueue)
//   2. StatusManager, ReviewQueuePoller (depend on ReviewQueue, Storage)
//   3. Storage.Start() (loads instances from disk, requires StatusManager wired)
//   4. Instance wiring (SetReviewQueue, SetStatusManager on each instance)
//   5. Instance.Start() (starts tmux sessions, requires wired dependencies)
//   6. Controller startup (requires started instances + StatusManager)
//   7. ReactiveQueueManager (requires ReviewQueue, Poller, EventBus, StatusManager)
//   8. ScrollbackManager, WebSocket handler, External discovery
//
// Violating this order causes nil pointer panics or silent failures.
// See ADR-XXX for the plan to replace this with a dependency injection container.
```

**Risk**: Documentation-only. No behavior change.

### Task 1.4: Extract `NewServer` initialization into a `ServerDependencies` struct

**Change**: Create a struct that holds all the wired dependencies, and a builder
function that constructs them in the correct order. This replaces the imperative
initialization sequence with a declarative one.

```go
// server/dependencies.go
type ServerDependencies struct {
    SessionService      *services.SessionService
    Storage             *session.Storage
    StatusManager       *session.InstanceStatusManager
    ReviewQueue         *session.ReviewQueue
    ReviewQueuePoller   *session.ReviewQueuePoller
    ReactiveQueueMgr    *ReactiveQueueManager
    ScrollbackManager   *scrollback.ScrollbackManager
    TmuxStreamerManager *session.ExternalTmuxStreamerManager
    ExternalDiscovery   *session.ExternalSessionDiscovery
}

func BuildDependencies() (*ServerDependencies, error) {
    // Steps 1-8 from the comment above, with error returns instead of log-and-continue
}
```

**Files**: New `server/dependencies.go`, modified `server/server.go`

**Tests**: Existing `server/` tests must still pass. Add a unit test for
`BuildDependencies` that verifies the returned struct has all fields non-nil.

**Risk**: Medium. Must preserve exact initialization order. Test by running
`make restart-web` and verifying all session operations work.

### Task 1.5: Audit and fix `connectrpc_websocket.go` TODOs

**File**: `server/services/connectrpc_websocket.go`

**Change**: Address the `TODO: Restrict origins in production` on line 30.
Replace the `CheckOrigin` that allows all origins with one that checks for
`localhost` and `127.0.0.1` origins (matching the existing
`validateLocalhostOrigin` pattern in `session_service.go`).

**Risk**: Low. The app only binds to localhost, but restricting WebSocket origins
adds defense-in-depth.

---

## Story 2: Storage Consolidation

**Goal**: Eliminate the raw `SQLiteRepository`, wire `EntRepository` as the
backing store, and begin migrating from `InstanceData` to `Session` as the
canonical domain model.

**Estimated effort**: 1-2 weeks

**Prerequisite**: Story 1 (initialization refactor in Task 1.4 simplifies wiring)

### Task 2.1: Wire `EntRepository` into `Storage` behind feature flag

**Change**: Modify `Storage` to accept a `Repository` interface (instead of
`config.InstanceStorage`) and add a constructor that creates an `EntRepository`.
Gate with `STAPLER_SQUAD_USE_ENT` environment variable (default: false).

```go
// session/storage.go
func NewStorageWithRepository(repo Repository) (*Storage, error) { ... }
```

When the flag is on, `Storage.SaveInstances` writes to both JSON (for rollback)
and Ent. `Storage.LoadInstances` reads from Ent.

**Files**: `session/storage.go`, `server/server.go` (or `server/dependencies.go`)

**Tests**: Add integration test that creates instances via `Storage`, then reads
them back via `EntRepository` directly to verify data fidelity.

**Bug risk**: `EntRepository` auto-migration may alter schema in ways incompatible
with existing data. Run `ent` auto-migration on a copy of the production database
first.

### Task 2.2: Implement data migration from JSON to Ent

**Change**: Extend `migrate.go` to support JSON-to-Ent migration (currently it
does JSON-to-SQLite). Reuse `MigrateJSONToSQLite` structure but target
`EntRepository.CreateSession`.

Add a startup migration that runs once: if Ent database is empty and JSON state
file exists, migrate all sessions.

**Files**: `session/migrate.go`, `session/migrate_test.go`

**Tests**: Test with fixture JSON files containing edge cases:
- Sessions with empty tags
- Sessions with GitHub PR fields
- Sessions with zero-value timestamps
- Sessions with nested category paths

**Bug risk**: Round-trip data loss through `InstanceData -> Session -> Ent -> Session`.
Specifically watch for:
- `ClaudeSessionData` nested struct serialization
- `DiffStatsData.Content` field (excluded from JSON via `json:"-"`)
- `SessionType` string enum values

### Task 2.3: Delete `SQLiteRepository` and raw SQL code

**Change**: Remove files:
- `session/sqlite_repository.go`
- `session/sqlite_repository_test.go`
- `session/sqlite_schema.go`
- `session/sqlite_state.go`
- `session/sqlite_diagnostics.go`

Update `session/repository.go`:
- Remove `WithMigrationMode` option (only used by `SQLiteRepository`)
- Remove `SQLiteRepository` type assertions in `WithDatabasePath`

**Tests**: `go test ./session/...` must still pass. Fix any tests that directly
instantiate `SQLiteRepository`.

**Bug risk**: Search the codebase for all `NewSQLiteRepository` and
`SQLiteRepository` references. Ensure nothing in production code path depends
on them.

### Task 2.4: Narrow `config.InstanceStorage` to UI-only state

**Change**: Remove `SaveInstances` and `GetInstances` from the
`config.InstanceStorage` interface. Session persistence is now handled by
`EntRepository`. Retain `config.StateManager` for UI state (help screens,
category expansion, search mode, selected index).

This is a breaking interface change. Update all implementations:
- `config/state.go` `State` struct: remove instance methods
- Any test mocks that implement `InstanceStorage`

**Files**: `config/state.go`, `session/storage.go`, mock files in tests

**Tests**: Full test suite. Pay attention to `session/storage_test.go` which
likely tests the JSON-based save/load path.

**Bug risk**: `Storage.SaveInstances` currently loads existing instances from
JSON for merging with in-memory state. After this change, merging logic moves
to `EntRepository` (which handles it via SQL `INSERT OR REPLACE`). Verify that
the merge behavior is preserved: instances from other processes must not be
silently dropped.

### Task 2.5: Add `Session`-first convenience methods to `Storage`

**Change**: Add methods to `Storage` that accept/return `*Session` directly,
without going through `InstanceData`:

```go
func (s *Storage) GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error)
func (s *Storage) ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error)
func (s *Storage) SaveSession(ctx context.Context, session *Session) error
```

These delegate to `EntRepository`'s Session-based methods. Existing
`InstanceData`-based methods (`SaveInstances`, `LoadInstances`) remain but are
marked deprecated.

**Files**: `session/storage.go`

**Tests**: Unit tests for new methods. Integration test verifying
`SaveSession -> GetSession` round-trip preserves all context fields.

---

## Story 3: Instance Decomposition

**Goal**: Break the `Instance` god class into focused sub-components while
preserving the external API.

**Estimated effort**: 2 weeks

**Prerequisite**: Story 2 (storage consolidation gives us a cleaner persistence
layer to serialize decomposed state)

### Task 3.1: Extract `ReviewState` struct

**Change**: Create `session/review_state.go` containing:

```go
type ReviewState struct {
    LastAcknowledged     time.Time
    LastAddedToQueue     time.Time
    LastTerminalUpdate   time.Time
    LastMeaningfulOutput time.Time
    LastOutputSignature  string
    LastViewed           time.Time
    LastPromptDetected   time.Time
    LastPromptSignature  string
    LastUserResponse     time.Time
    ProcessingGraceUntil time.Time

    mu sync.RWMutex
}

func (rs *ReviewState) NeedsReview() bool { ... }
func (rs *ReviewState) UpdateTerminalTimestamps(content string, forceUpdate bool) { ... }
func (rs *ReviewState) DetectAndTrackPrompt(content string, statusInfo InstanceStatusInfo) bool { ... }
func (rs *ReviewState) ComputePromptSignature(content string) string { ... }
func (rs *ReviewState) GetTimeSinceLastMeaningfulOutput() time.Duration { ... }
func (rs *ReviewState) GetTimeSinceLastTerminalUpdate() time.Duration { ... }
```

`Instance` gets a `reviewState *ReviewState` field. Existing methods on
`Instance` delegate:

```go
func (i *Instance) NeedsReview() bool {
    return i.reviewState.NeedsReview()
}
```

Start with `ReviewState` because it has the cleanest boundary: no dependencies
on tmux, git, or controller state.

**Files**: New `session/review_state.go`, new `session/review_state_test.go`,
modified `session/instance.go`

**Tests**: Move review-queue-related tests from `instance_test.go` and
`instance_last_acknowledged_test.go` to `review_state_test.go`. Verify
`ReviewState` can be tested without instantiating a full `Instance`.

**Bug risk**: The `stateMutex` on `Instance` currently protects review state
fields. After extraction, `ReviewState` has its own `mu`. Callers that hold
`Instance.stateMutex` and then call review methods must not deadlock. Audit
all `stateMutex.Lock()` sites in `instance.go` to verify they do not also
lock `ReviewState.mu`.

### Task 3.2: Extract `TmuxProcessManager` struct

**Change**: Create `session/tmux_process_manager.go` containing:

```go
type TmuxProcessManager struct {
    session   *tmux.TmuxSession
    started   bool
    // Preview size tracking
    lastPreviewWidth  int
    lastPreviewHeight int
    lastPTYWarningTime time.Time

    mu sync.RWMutex
}

func (tm *TmuxProcessManager) Start(name, program, path string, opts TmuxStartOptions) error { ... }
func (tm *TmuxProcessManager) Kill() error { ... }
func (tm *TmuxProcessManager) Pause() error { ... }
func (tm *TmuxProcessManager) Resume() error { ... }
func (tm *TmuxProcessManager) Attach() (chan struct{}, error) { ... }
func (tm *TmuxProcessManager) Preview() (string, error) { ... }
func (tm *TmuxProcessManager) SetPreviewSize(width, height int) error { ... }
func (tm *TmuxProcessManager) CapturePaneContent() (string, error) { ... }
func (tm *TmuxProcessManager) CapturePaneContentRaw() (string, error) { ... }
func (tm *TmuxProcessManager) GetPTYReader() (*os.File, error) { ... }
func (tm *TmuxProcessManager) WriteToPTY(data []byte) (int, error) { ... }
func (tm *TmuxProcessManager) ResizePTY(cols, rows int) error { ... }
func (tm *TmuxProcessManager) SendKeys(keys string) error { ... }
func (tm *TmuxProcessManager) Alive() bool { ... }
func (tm *TmuxProcessManager) GetSession() *tmux.TmuxSession { ... }
```

`Instance` delegates all tmux operations to `TmuxProcessManager`.

**Files**: New `session/tmux_process_manager.go`, new
`session/tmux_process_manager_test.go`, modified `session/instance.go`

**Tests**: Test `TmuxProcessManager` in isolation with a real tmux session
(integration test) or with a mock `tmux.TmuxSession`.

**Bug risk**: The `Instance.Start()` method currently interleaves tmux setup
with git worktree setup. After extraction, `Instance.Start()` must coordinate
both managers in the correct order (git worktree first, then tmux session
pointing at the worktree path). A sequencing error here will cause tmux to
start in the wrong directory.

### Task 3.3: Extract `GitWorktreeManager` struct

**Change**: Create `session/git_worktree_manager.go` containing:

```go
type GitWorktreeManager struct {
    worktree  *git.GitWorktree
    diffStats *git.DiffStats

    mu sync.RWMutex
}

func (gm *GitWorktreeManager) Setup(repoPath, branch, sessionName string, opts GitSetupOptions) error { ... }
func (gm *GitWorktreeManager) Cleanup() error { ... }
func (gm *GitWorktreeManager) UpdateDiffStats() error { ... }
func (gm *GitWorktreeManager) GetDiffStats() *git.DiffStats { ... }
func (gm *GitWorktreeManager) GetWorktree() *git.GitWorktree { ... }
func (gm *GitWorktreeManager) HasWorktree() bool { ... }
func (gm *GitWorktreeManager) DetectAndPopulateWorktreeInfo(instance *Instance) error { ... }
```

**Files**: New `session/git_worktree_manager.go`, new
`session/git_worktree_manager_test.go`, modified `session/instance.go`

**Tests**: Test worktree creation and cleanup in isolation using a temporary
git repository.

**Bug risk**: `DetectAndPopulateWorktreeInfo` currently modifies `Instance`
fields directly (`IsWorktree`, `MainRepoPath`, etc.). After extraction, it
needs a way to write back to `Instance` metadata. Pass `Instance` as a
parameter or return a struct that the caller applies.

### Task 3.4: Extract `ControllerManager` struct

**Change**: Create `session/controller_manager.go` containing:

```go
type ControllerManager struct {
    controller     *ClaudeController
    statusManager  *InstanceStatusManager
    claudeSession  *ClaudeSessionData

    mu sync.RWMutex
}

func (cm *ControllerManager) StartController(instance *Instance) error { ... }
func (cm *ControllerManager) StopController() { ... }
func (cm *ControllerManager) GetController() *ClaudeController { ... }
func (cm *ControllerManager) HandleSessionReattachment(instance *Instance) error { ... }
func (cm *ControllerManager) SetStatusManager(manager *InstanceStatusManager) { ... }
func (cm *ControllerManager) GetStatusManager() *InstanceStatusManager { ... }
```

**Files**: New `session/controller_manager.go`, new
`session/controller_manager_test.go`, modified `session/instance.go`

**Bug risk**: `StartController` currently needs access to `Instance` for the
title, program, tmux session, and review queue. Either pass these as parameters
or pass `Instance` itself. The latter creates a circular dependency risk if
`ControllerManager` stores an `Instance` reference -- prefer passing an
interface.

### Task 3.5: Update `Instance.Start()` to coordinate sub-managers

**Change**: Refactor `Instance.start()` (lines 619-825) to delegate to the
three managers in sequence:

```go
func (i *Instance) start(firstTimeSetup bool, ...) error {
    // 1. Git worktree setup (if applicable)
    if i.SessionType != SessionTypeDirectory {
        if err := i.gitManager.Setup(...); err != nil {
            return err
        }
    }

    // 2. Tmux session start
    if err := i.tmuxManager.Start(...); err != nil {
        i.gitManager.Cleanup() // rollback
        return err
    }

    // 3. Controller start (depends on tmux being alive)
    if err := i.controllerManager.StartController(i); err != nil {
        log.WarningLog.Printf("Controller start failed: %v", err)
        // Non-fatal: session works without controller
    }

    return nil
}
```

This replaces the current 200+ line method with ~30 lines of coordination logic.

**Files**: `session/instance.go`

**Tests**: Integration test that starts an instance and verifies all three
managers are initialized. Test the rollback path (git succeeds, tmux fails).

**Bug risk**: The current `start()` has subtle cleanup logic (`tmux.CleanupFunc`)
that must be preserved. Verify that cleanup on partial failure still removes
the worktree and kills the tmux session.

---

## Story 4: SessionService Splitting

**Goal**: Extract domain-focused internal service objects from the monolithic
`SessionService`, turning it into a thin facade.

**Estimated effort**: 2 weeks

**Prerequisite**: Story 3 (decomposed `Instance` makes it clearer which
service owns which operations)

### Task 4.1: Extract `ReviewQueueService`

**Change**: Create `server/services/review_queue_service.go` containing:

```go
type ReviewQueueService struct {
    reviewQueue       *session.ReviewQueue
    statusManager     *session.InstanceStatusManager
    reviewQueuePoller *session.ReviewQueuePoller
    reactiveQueueMgr  ReactiveQueueManager
    storage           *session.Storage
    eventBus          *events.EventBus
}

func (rqs *ReviewQueueService) GetReviewQueue(ctx context.Context) (*sessionv1.ReviewQueue, error) { ... }
func (rqs *ReviewQueueService) AcknowledgeSession(ctx context.Context, sessionID string) error { ... }
func (rqs *ReviewQueueService) WatchReviewQueue(ctx context.Context, filters interface{}) (<-chan *sessionv1.ReviewQueueEvent, string) { ... }
func (rqs *ReviewQueueService) LogUserInteraction(ctx context.Context, sessionID string, interactionType string) error { ... }
```

Move lines 965-1440 from `session_service.go` into this new file.
`SessionService` delegates:

```go
func (s *SessionService) GetReviewQueue(ctx context.Context, req *connect.Request[...]) (*connect.Response[...], error) {
    return s.reviewQueueService.GetReviewQueue(ctx, req)
}
```

**Files**: New `server/services/review_queue_service.go`, modified
`server/services/session_service.go`

**Tests**: Move or duplicate existing tests for review queue RPC methods.
Verify delegation preserves request/response types.

**Bug risk**: The review queue methods access `s.storage`, `s.reviewQueue`,
`s.statusManager`, and `s.reactiveQueueMgr`. All must be passed to the new
service. Missing a dependency causes nil panics at runtime -- not caught by
compilation.

### Task 4.2: Extract `SearchService`

**Change**: Create `server/services/search_service.go` containing:

```go
type SearchService struct {
    searchEngine     *session.SearchEngine
    snippetGenerator *session.SnippetGenerator
    historyCache     *session.ClaudeSessionHistory
    historyCacheTime time.Time
    historyCacheTTL  time.Duration
}

func (ss *SearchService) ListClaudeHistory(ctx context.Context, ...) { ... }
func (ss *SearchService) GetClaudeHistoryDetail(ctx context.Context, ...) { ... }
func (ss *SearchService) GetClaudeHistoryMessages(ctx context.Context, ...) { ... }
func (ss *SearchService) SearchClaudeHistory(ctx context.Context, ...) { ... }
```

Move lines 1539-1873 from `session_service.go`.

**Files**: New `server/services/search_service.go`, modified
`server/services/session_service.go`

**Bug risk**: The history cache (`historyCache`, `historyCacheTime`) is currently
accessed without explicit synchronization in `SessionService`. After extraction
into `SearchService`, add a `sync.RWMutex` to protect cache access. Without
this, concurrent `ListClaudeHistory` calls may race on cache refresh.

### Task 4.3: Extract `GitHubService`

**Change**: Create `server/services/github_service.go` containing:

```go
type GitHubService struct {
    storage *session.Storage
}

func (gs *GitHubService) GetPRInfo(ctx context.Context, ...) { ... }
func (gs *GitHubService) GetPRComments(ctx context.Context, ...) { ... }
func (gs *GitHubService) PostPRComment(ctx context.Context, ...) { ... }
func (gs *GitHubService) MergePR(ctx context.Context, ...) { ... }
func (gs *GitHubService) ClosePR(ctx context.Context, ...) { ... }
```

Move lines 1874-2140 from `session_service.go`. These methods shell out to
`gh` CLI -- they have no dependency on review queue, terminal, or search.

**Files**: New `server/services/github_service.go`, modified
`server/services/session_service.go`

**Bug risk**: These methods call `exec.Command("gh", ...)`. Ensure the
extraction does not alter error handling or context cancellation propagation.

### Task 4.4: Extract `WorkspaceService`

**Change**: Create `server/services/workspace_service.go` containing:

```go
type WorkspaceService struct {
    storage *session.Storage
}

func (ws *WorkspaceService) GetVCSStatus(ctx context.Context, ...) { ... }
func (ws *WorkspaceService) GetWorkspaceInfo(ctx context.Context, ...) { ... }
func (ws *WorkspaceService) ListWorkspaceTargets(ctx context.Context, ...) { ... }
func (ws *WorkspaceService) SwitchWorkspace(ctx context.Context, ...) { ... }
```

Move lines 2519-2921 from `session_service.go`.

**Files**: New `server/services/workspace_service.go`, modified
`server/services/session_service.go`

**Bug risk**: `SwitchWorkspace` modifies session state and may trigger
re-initialization. Verify that the extracted service has access to the same
`Storage` instance and that state mutations are visible to other services.

### Task 4.5: Wire extracted services into `SessionService`

**Change**: Update `SessionService` struct to hold references to all extracted
services:

```go
type SessionService struct {
    // Core dependencies
    storage   *session.Storage
    eventBus  *events.EventBus

    // Extracted domain services
    reviewQueueService *ReviewQueueService
    searchService      *SearchService
    githubService      *GitHubService
    workspaceService   *WorkspaceService

    // Notification rate limiter
    notificationRateLimiter *NotificationRateLimiter
}
```

Update `NewSessionService` and the dependency builder from Story 1 to construct
and wire all sub-services.

**Files**: `server/services/session_service.go`, `server/dependencies.go`

**Tests**: Run the full test suite. Verify all RPC endpoints return the same
responses as before extraction.

---

## Story 5: Package Structure

**Goal**: Organize the 95-file `session/` package into sub-packages with clear
module boundaries.

**Estimated effort**: 1 week

**Prerequisite**: Story 3 (extracted structs become natural package boundaries)

### Task 5.1: Create `session/queue/` sub-package

**Change**: Move review queue types and logic into `session/queue/`:
- `review_queue.go` -> `session/queue/queue.go`
- `review_queue_poller.go` -> `session/queue/poller.go`
- `review_queue_test.go` -> `session/queue/queue_test.go`
- `review_queue_poller_test.go` -> `session/queue/poller_test.go`
- `review_queue_example_test.go` -> `session/queue/example_test.go`
- `review_queue_uncommitted_changes_test.go` -> `session/queue/uncommitted_changes_test.go`
- `review_state.go` (from Story 3) -> `session/queue/review_state.go`

Package name: `queue`

Update all import paths. The types `ReviewQueue`, `ReviewItem`, `Priority`,
`AttentionReason` become `queue.Queue`, `queue.Item`, `queue.Priority`,
`queue.AttentionReason`.

**Bug risk**: Circular dependency. `ReviewQueuePoller` references `*Instance`
and `*Storage`. Define an interface in `session/queue/` that `Instance`
satisfies:

```go
// session/queue/session.go
type MonitoredSession interface {
    GetTitle() string
    GetStatus() Status
    GetLastMeaningfulOutput() time.Time
    GetReviewState() *ReviewState
    // ... minimal interface
}
```

This breaks the circular dependency `queue -> session -> queue`.

### Task 5.2: Create `session/detection/` sub-package

**Change**: Move status detection logic into `session/detection/`:
- `status_detector.go` -> `session/detection/detector.go`
- `status_detector_test.go` -> `session/detection/detector_test.go`
- `status_detector_input_required_test.go` -> `session/detection/input_required_test.go`
- `status_detector_tests_failing_test.go` -> `session/detection/tests_failing_test.go`
- `status_patterns.yaml` -> `session/detection/patterns.yaml`
- `claude_status_patterns.yaml` -> `session/detection/claude_patterns.yaml`
- `idle_detector.go` -> `session/detection/idle.go`
- `idle_detector_test.go` -> `session/detection/idle_test.go`
- `approval_detector.go` -> `session/detection/approval.go`
- `approval_detector_test.go` -> `session/detection/approval_test.go`

Package name: `detection`

The `DetectedStatus` type moves from `session` to `detection`. Add a type alias
in `session/` for backward compatibility during migration:

```go
// session/status_compat.go
type DetectedStatus = detection.DetectedStatus
```

**Bug risk**: YAML file embedding. The `status_detector.go` embeds YAML files
using `go:embed` or reads from disk. After moving to a sub-package, the embed
paths must be updated. Verify the embedded files are accessible from the new
package location.

### Task 5.3: Create `session/search/` sub-package

**Change**: Move search and indexing into `session/search/`:
- `search_engine.go` -> `session/search/engine.go`
- `search_engine_test.go` -> `session/search/engine_test.go`
- `bm25.go` -> `session/search/bm25.go`
- `bm25_test.go` -> `session/search/bm25_test.go`
- `inverted_index.go` -> `session/search/inverted_index.go`
- `inverted_index_test.go` -> `session/search/inverted_index_test.go`
- `tokenizer.go` -> `session/search/tokenizer.go`
- `tokenizer_test.go` -> `session/search/tokenizer_test.go`
- `snippet_generator.go` -> `session/search/snippet.go`
- `snippet_generator_test.go` -> `session/search/snippet_test.go`
- `index_store.go` -> `session/search/index_store.go`
- `index_store_test.go` -> `session/search/index_store_test.go`
- `document_store.go` -> `session/search/document_store.go`

Package name: `search`

**Bug risk**: The search engine indexes `Instance` fields directly. After
extraction, define a `search.Indexable` interface that `Instance`/`Session`
implements, avoiding a direct dependency on the `session` package.

### Task 5.4: Update import paths and verify build

**Change**: Comprehensive find-and-replace of import paths across the codebase.
Use `goimports` to fix imports automatically where possible.

Run:
```bash
go build ./...
go test ./...
go vet ./...
```

**Risk**: This is the highest-risk task in Story 5. A single missed import
breaks compilation. Use `go build ./...` iteratively during the migration.

---

## Story 6: Type Safety

**Goal**: Replace bare numeric types with proper domain types and consolidate
overlapping status enumerations.

**Estimated effort**: 1 week

**Prerequisite**: Story 5 (package structure provides clear homes for types)

### Task 6.1: Create `Priority` as a proper type with validation

**Change**: Replace the bare `int` priority constants with a type that
validates values:

```go
// session/queue/priority.go
type Priority int

const (
    PriorityUrgent Priority = 1
    PriorityHigh   Priority = 2
    PriorityMedium Priority = 3
    PriorityLow    Priority = 4
)

func (p Priority) IsValid() bool {
    return p >= PriorityUrgent && p <= PriorityLow
}

func (p Priority) String() string {
    switch p {
    case PriorityUrgent: return "urgent"
    case PriorityHigh:   return "high"
    case PriorityMedium: return "medium"
    case PriorityLow:    return "low"
    default:             return fmt.Sprintf("invalid(%d)", p)
    }
}

// IsHigherThan returns true if p is higher priority than other.
// Note: LOWER numeric value = HIGHER priority.
func (p Priority) IsHigherThan(other Priority) bool {
    return p < other
}
```

The `IsHigherThan` method eliminates the confusing "lower int = higher priority"
comparison that is currently scattered through the codebase.

**Files**: `session/queue/priority.go` (or `session/review_queue.go` if Story 5
is not yet complete), all files that compare priorities.

**Tests**: Unit tests for `IsValid`, `String`, `IsHigherThan`. Test with
boundary values (0, 5, -1).

**Bug risk**: Any code that does `if item.Priority < other.Priority` must be
updated to `if item.Priority.IsHigherThan(other.Priority)`. A missed site
silently inverts priority ordering.

### Task 6.2: Consolidate `Status`, `DetectedStatus`, and `AttentionReason`

**Change**: Document the relationship between the three status types and create
mapping functions with exhaustive `switch` statements:

```go
// session/status_mapping.go

// StatusFromDetected maps a DetectedStatus to the corresponding lifecycle Status.
// This is used when the review queue poller updates Instance.Status based on detection.
func StatusFromDetected(detected DetectedStatus) Status {
    switch detected {
    case StatusReady, StatusIdle, StatusSuccess:
        return Ready
    case StatusProcessing, StatusActive:
        return Running
    case StatusNeedsApproval, StatusInputRequired:
        return NeedsApproval
    case StatusError, StatusTestsFailing:
        return Running // Error state still "running" at lifecycle level
    default:
        return Running
    }
}

// AttentionReasonFromDetected maps a DetectedStatus to the appropriate AttentionReason
// for review queue items. Returns empty string if no attention needed.
func AttentionReasonFromDetected(detected DetectedStatus) AttentionReason {
    switch detected {
    case StatusNeedsApproval:
        return ReasonApprovalPending
    case StatusInputRequired:
        return ReasonInputRequired
    case StatusError:
        return ReasonErrorState
    case StatusTestsFailing:
        return ReasonTestsFailing
    case StatusIdle:
        return ReasonIdle
    case StatusSuccess:
        return ReasonTaskComplete
    default:
        return "" // No attention needed
    }
}
```

Add exhaustiveness checking: the `switch` must cover all `DetectedStatus` values.
Use a linter rule or a compile-time assertion pattern.

**Files**: New `session/status_mapping.go`, new `session/status_mapping_test.go`

**Tests**: Table-driven tests covering every `DetectedStatus` value. Verify
that the mapping is consistent with the current behavior (grep for existing
status-to-reason mapping code to extract the implicit rules).

**Bug risk**: The current mapping is implicit and spread across
`review_queue_poller.go`, `instance_status.go`, and `claude_controller.go`.
Centralizing it may surface inconsistencies where different code paths map
the same `DetectedStatus` to different `AttentionReason` values. These must
be resolved, not papered over.

### Task 6.3: Add `SessionType` validation

**Change**: `SessionType` is currently `type SessionType string` with no
validation. Add a `IsValid()` method and use it at session creation time:

```go
func (st SessionType) IsValid() bool {
    switch st {
    case SessionTypeDirectory, SessionTypeNewWorktree, SessionTypeExistingWorktree:
        return true
    default:
        return false
    }
}
```

Call `IsValid()` in `CreateSession` RPC handler. Return a `connect.CodeInvalidArgument`
error for invalid session types.

**Files**: `session/instance.go` (or the file where `SessionType` is defined),
`server/services/session_service.go` CreateSession handler

**Bug risk**: Low. Additive validation. Existing valid session types are not
affected.

---

## Known Issues and Bug Risks

### Migration-Related Risks

**Risk M1: Data loss during JSON-to-Ent migration** [SEVERITY: High]

During Story 2, converting `InstanceData` to `Session` and storing via
`EntRepository` involves multiple data transformations. The `InstanceToSession`
adapter in `session/session.go` does not preserve all fields:
- `CloudContext` has no `Instance` equivalent (data loss on `SessionToInstance`)
- `DiffStatsData.Content` is excluded from JSON serialization (`json:"-"`)
- `ClaudeSessionData` nested struct has optional fields that may serialize as
  zero values

**Mitigation**: Write a comprehensive round-trip test:
`InstanceData -> Instance -> Session -> Ent -> Session -> Instance -> InstanceData`
and assert field-by-field equality. Run this test against production data
snapshots.

**Risk M2: Ent auto-migration alters existing SQLite schema** [SEVERITY: Medium]

`EntRepository` calls `client.Schema.Create(context.Background())` on startup,
which auto-migrates the schema. If the Ent schema diverges from the existing
SQLite schema (created by `sqlite_schema.go`), auto-migration may fail or
silently drop columns.

**Mitigation**: Before wiring Ent in production, run auto-migration against a
copy of a production database and diff the schema. Use `ent`'s `WithDropColumn`
and `WithDropIndex` options cautiously.

### Concurrency Risks

**Risk C1: Lock ordering between `Instance.stateMutex` and sub-manager locks** [SEVERITY: High]

After Story 3, `Instance` has `stateMutex` and each sub-manager has its own
`mu`. If code acquires `Instance.stateMutex` then calls a sub-manager method
that acquires `manager.mu`, and other code acquires `manager.mu` then calls
back to `Instance` (acquiring `stateMutex`), a deadlock occurs.

**Mitigation**: Establish a strict lock ordering: `Instance.stateMutex` is
always acquired before any sub-manager lock. Sub-manager methods that are called
while `stateMutex` is held must not attempt to acquire `stateMutex` again.
Document this in code comments.

**Risk C2: Race condition on `SearchService` history cache** [SEVERITY: Medium]

The history cache in `SessionService` (`historyCache`, `historyCacheTime`) is
accessed without synchronization. After extraction to `SearchService` (Story 4,
Task 4.2), concurrent `ListClaudeHistory` calls may see inconsistent cache state.

**Mitigation**: Add `sync.RWMutex` to `SearchService` and use `RLock` for
cache reads, `Lock` for cache refreshes. This is a pre-existing bug that
the refactor surfaces.

### Import/Compilation Risks

**Risk I1: Circular dependencies after package extraction** [SEVERITY: High]

Story 5 moves types into sub-packages. The `session/queue/` package needs to
reference `Instance` (to read status, timestamps), but `Instance` references
`ReviewQueue`. This is a classic circular dependency.

**Mitigation**: Define interfaces in the sub-package that the parent package
types implement. Example: `queue.MonitoredSession` interface satisfied by
`*Instance`. The sub-package never imports the parent.

**Risk I2: Embedded file path breakage** [SEVERITY: Medium]

`status_detector.go` loads YAML pattern files. After moving to
`session/detection/`, the relative paths to embedded files change.

**Mitigation**: Use `go:embed` with paths relative to the new package directory.
Verify with `go build` immediately after the move.

### Behavioral Risks

**Risk B1: Initialization order regression** [SEVERITY: High]

Story 1 Task 1.4 refactors the fragile initialization in `server/server.go`.
The 5 "CRITICAL ORDER" comments encode hard-won knowledge about dependency
ordering. Refactoring this incorrectly causes nil pointer panics on startup
that only manifest when sessions exist in the database.

**Mitigation**: Write an integration test that:
1. Creates a state file with 2-3 sessions
2. Starts the server via `BuildDependencies`
3. Verifies all sessions are running and have controllers
4. Verifies the review queue poller has all sessions registered

**Risk B2: `Storage.SaveInstances` merge behavior change** [SEVERITY: Medium]

The current `Storage.SaveInstances` loads existing instances from JSON, merges
with in-memory instances, and writes back. After switching to `EntRepository`,
this merge happens at the database level (`INSERT OR REPLACE`). The semantics
differ: JSON merge preserves instances from other processes, while `INSERT OR
REPLACE` may overwrite concurrent writes.

**Mitigation**: Use SQLite transactions with proper isolation. Add a
`last_updated` timestamp check (optimistic locking) to detect concurrent
modifications.

---

## Dependency Graph

```
Story 1: Quick Wins (no prerequisites)
    |
    v
Story 2: Storage Consolidation (depends on Story 1, Task 1.4)
    |
    v
Story 3: Instance Decomposition (depends on Story 2)
    |
    +---> Story 4: SessionService Splitting (depends on Story 3)
    |
    +---> Story 5: Package Structure (depends on Story 3)
              |
              v
          Story 6: Type Safety (depends on Story 5)
```

Stories 4 and 5 can run in parallel after Story 3 completes. Story 6 depends
on Story 5 because the type improvements land in the new sub-packages.

---

## Verification Checklist (per story)

After completing each story, verify:

- [ ] `go build ./...` succeeds with no errors
- [ ] `go test ./...` passes (all existing tests, including flaky ones)
- [ ] `go vet ./...` reports no new issues
- [ ] `make restart-web` starts the server and web UI loads
- [ ] Session create, pause, resume, delete work via web UI
- [ ] Terminal streaming works for at least one session
- [ ] Review queue populates when a session reaches approval state
- [ ] Search returns results matching session titles and tags
