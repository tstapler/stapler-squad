# Workspace Status Visualization Feature Plan

## Executive Summary

This feature introduces a comprehensive workspace status visualization system for claude-squad, enabling users to see at a glance the state of all directories that sessions have interacted with, their associated git worktrees, and pending git/jj work (uncommitted changes, untracked files, staged files). This dashboard-style view helps users understand where they have pending work across all their sessions, reducing the cognitive load of managing multiple parallel development tasks.

## Problem Statement

### Current Limitations

1. **No Global Workspace View**: Users can only see git status for one session at a time in the VcsPanel
2. **Hidden Pending Work**: Uncommitted changes in paused or inactive sessions are not visible at a glance
3. **Scattered Worktrees**: Users lose track of which directories have active claude-squad worktrees
4. **No Directory Discovery**: No way to see all directories that sessions have touched across time
5. **Manual Status Checking**: Users must attach to each session individually to check git status

### User Impact

- Developers forget about uncommitted work in paused sessions
- Work gets stranded in worktrees when sessions are deleted
- No visibility into potential merge conflicts across sessions
- Difficult to understand the overall state of development across repositories
- Risk of losing work that was never committed

## Requirements Analysis

### Functional Requirements (IEEE 830 Format)

#### FR-1: Workspace Discovery

- **FR-1.1**: System SHALL discover all directories that have active claude-squad sessions
- **FR-1.2**: System SHALL identify git/jj worktrees associated with each session
- **FR-1.3**: System SHALL track the main repository path for each worktree
- **FR-1.4**: System SHALL detect orphaned worktrees (no active session)
- **FR-1.5**: System SHALL persist workspace directory history for quick reference

#### FR-2: Git/JJ Status Collection

- **FR-2.1**: System SHALL retrieve git status for all tracked directories
- **FR-2.2**: System SHALL identify uncommitted changes (staged and unstaged)
- **FR-2.3**: System SHALL identify untracked files
- **FR-2.4**: System SHALL detect ahead/behind status relative to remote
- **FR-2.5**: System SHALL support both Git and Jujutsu version control systems

#### FR-3: TUI Visualization

- **FR-3.1**: System SHALL provide a dedicated workspace status overlay accessible via keyboard shortcut
- **FR-3.2**: System SHALL display hierarchical view of repositories and their worktrees
- **FR-3.3**: System SHALL show aggregated change counts (files modified, staged, untracked)
- **FR-3.4**: System SHALL allow navigation to individual session from workspace view
- **FR-3.5**: System SHALL highlight workspaces with uncommitted changes using visual indicators

#### FR-4: Web UI Dashboard

- **FR-4.1**: System SHALL provide a workspace status dashboard page
- **FR-4.2**: System SHALL display repository cards with worktree and change information
- **FR-4.3**: System SHALL support filtering by repository, status, or session
- **FR-4.4**: System SHALL provide drill-down navigation to session details
- **FR-4.5**: System SHALL support real-time status updates via WebSocket

#### FR-5: Status Aggregation

- **FR-5.1**: System SHALL aggregate workspace status across all sessions
- **FR-5.2**: System SHALL provide summary statistics (total uncommitted files, total repositories)
- **FR-5.3**: System SHALL group workspaces by repository root
- **FR-5.4**: System SHALL identify workspaces that need attention (conflicts, stale branches)

### Non-Functional Requirements (ISO/IEC 25010)

#### Performance

- **NFR-P1**: Workspace status collection SHALL complete within 2 seconds for 50 repositories
- **NFR-P2**: Status updates SHALL propagate to UI within 500ms
- **NFR-P3**: Background status refresh SHALL not exceed 5% CPU utilization
- **NFR-P4**: Memory usage for workspace cache SHALL not exceed 50MB

#### Reliability

- **NFR-R1**: System SHALL handle git command failures gracefully with clear error messages
- **NFR-R2**: System SHALL maintain status cache across application restarts
- **NFR-R3**: System SHALL recover from network failures when checking remote status
- **NFR-R4**: System SHALL handle concurrent git operations safely

#### Security

- **NFR-S1**: System SHALL only access directories authorized by the user
- **NFR-S2**: System SHALL not expose file contents, only metadata
- **NFR-S3**: System SHALL sanitize all paths for display

#### Usability

- **NFR-U1**: Workspace status SHALL be accessible with maximum 2 keystrokes
- **NFR-U2**: Visual indicators SHALL be colorblind-accessible
- **NFR-U3**: Status information SHALL be scannable in under 5 seconds

#### Concurrency (Multi-Pod Deployment)

- **NFR-C1**: System SHALL support concurrent access from multiple pods/replicas
- **NFR-C2**: Workspace status updates SHALL be coordinated to prevent race conditions
- **NFR-C3**: Cache invalidation SHALL propagate across all replicas within 5 seconds
- **NFR-C4**: Lock acquisition SHALL timeout after 10 seconds to prevent deadlocks
- **NFR-C5**: System SHALL gracefully degrade if distributed lock service is unavailable

## Architecture & Design

### Bounded Contexts (Domain-Driven Design)

#### Workspace Context

- **Aggregate Root**: `WorkspaceRegistry`
- **Entities**: `TrackedWorkspace`, `WorkspaceStatus`
- **Value Objects**: `WorkspacePath`, `RepositoryRoot`, `ChangesSummary`
- **Domain Events**: `WorkspaceDiscovered`, `StatusChanged`, `WorkspaceOrphaned`

#### Session Integration Context

- **Integration Point**: `Instance.GetWorkingDirectory()`, `Instance.GetGitWorktree()`
- **Anti-Corruption Layer**: `WorkspaceSessionAdapter`
- **Shared Kernel**: `VCSStatus` types from `session/vc`

### Architecture Decision Records

#### ADR-001: Distributed Locking Strategy for Multi-Pod Deployment

**Context**: The current architecture uses SQLite with in-memory `sync.RWMutex` for concurrency control. This works for single-process deployments but fails when multiple pods access shared workspace state. Multi-pod deployments require coordinated access to prevent race conditions in status cache updates and orphan detection.

**Decision**: Implement a tiered locking strategy with database-level coordination:

1. **PostgreSQL Advisory Locks** (Primary for production multi-pod):
   - Use `pg_advisory_lock(key)` for exclusive access to workspace registry operations
   - Use `pg_try_advisory_lock(key)` with timeout for non-blocking attempts
   - Lock keys derived from workspace path hash for fine-grained locking
   - Reentrant within same database connection (supports nested operations)

2. **SQLite with WAL + Application-Level Coordination** (Development/single-pod):
   - Keep existing SQLite repository for local development
   - Use database transactions with `IMMEDIATE` mode for write coordination
   - Accept that multi-pod SQLite deployments require shared filesystem (NFS/EFS)

3. **Cache Invalidation via Database Triggers + Notifications**:
   - PostgreSQL: Use `LISTEN/NOTIFY` for cross-pod cache invalidation
   - SQLite: Poll-based invalidation with short TTL (5s)

**Rationale**:
- PostgreSQL advisory locks are built-in, no external dependencies (vs Redis Redlock)
- Advisory locks are reentrant, avoiding deadlock in nested operations
- `LISTEN/NOTIFY` provides efficient pub/sub without external message queue
- Maintains SQLite compatibility for development simplicity
- No additional infrastructure required (vs etcd/Consul)

**Consequences**:
- Production multi-pod deployments MUST use PostgreSQL
- SQLite remains viable for single-pod/development scenarios
- Need to implement `DistributedLock` interface with two backends
- Cache TTL must be shorter than lock timeout to prevent stale reads
- Database connection pool must support long-lived connections for LISTEN

**Patterns Applied**:
- Strategy Pattern: `DistributedLock` interface with SQLite/PostgreSQL implementations
- Circuit Breaker: Degrade to local-only mode if lock service unavailable
- Event-Driven: Use database notifications for cache invalidation

#### ADR-002: Workspace State Storage Location

**Context**: Workspace status needs to be persisted and shared across pods. Options include:
1. Store in existing sessions database
2. Store in separate workspace database
3. Store in external cache (Redis)

**Decision**: Extend existing session database with workspace tables.

**Rationale**:
- Maintains transactional consistency with session data
- No additional infrastructure dependencies
- Workspace data is tightly coupled to session lifecycle
- Simplifies backup and migration

**Consequences**:
- Schema migration required for new tables
- Larger database size
- Workspace cleanup tied to database maintenance

### Component Architecture

```
+------------------+     +-------------------+     +------------------+
|  WorkspacePanel  |---->| WorkspaceService  |---->|  VCSProvider     |
|  (TUI/Web)       |     |                   |     |  (Git/JJ)        |
+------------------+     +-------------------+     +------------------+
                                  |
                                  v
                         +-------------------+
                         | WorkspaceRegistry |
                         | (State Manager)   |
                         +-------------------+
                                  |
                                  v
                         +-------------------+
                         |  SessionStorage   |
                         |  (Persistence)    |
                         +-------------------+
```

### Data Model

#### WorkspaceRegistry

```go
// Location: session/workspace/registry.go

// WorkspaceRegistry tracks all known workspaces and their status
type WorkspaceRegistry struct {
    workspaces    map[string]*TrackedWorkspace  // keyed by absolute path
    byRepository  map[string][]*TrackedWorkspace // grouped by repo root
    mu            sync.RWMutex                   // local mutex for in-memory cache
    distLock      DistributedLock                // distributed lock for multi-pod coordination
    statusCache   *WorkspaceStatusCache
    pollInterval  time.Duration
    notifier      CacheInvalidationNotifier      // cross-pod cache invalidation
}

// DistributedLock provides coordination across multiple pods/processes
// Implementations: PostgresAdvisoryLock, SQLiteTransactionLock, NoOpLock
type DistributedLock interface {
    // Acquire obtains an exclusive lock for the given resource
    // Returns context that should be used for operations and released when done
    Acquire(ctx context.Context, resource string, timeout time.Duration) (LockHandle, error)

    // TryAcquire attempts to acquire lock without blocking
    // Returns (handle, true) if acquired, (nil, false) if unavailable
    TryAcquire(ctx context.Context, resource string) (LockHandle, bool, error)
}

// LockHandle represents an acquired distributed lock
type LockHandle interface {
    // Release releases the lock - must be called when done
    Release() error

    // Extend extends the lock TTL (for long-running operations)
    Extend(duration time.Duration) error

    // IsValid checks if lock is still held
    IsValid() bool
}

// CacheInvalidationNotifier handles cross-pod cache invalidation
// Implementations: PostgresListenNotify, PollingNotifier
type CacheInvalidationNotifier interface {
    // Subscribe listens for invalidation events
    Subscribe(ctx context.Context, handler func(workspacePath string)) error

    // Publish broadcasts invalidation to all pods
    Publish(ctx context.Context, workspacePath string) error
}

// TrackedWorkspace represents a directory tracked by claude-squad
type TrackedWorkspace struct {
    Path            string             // Absolute path to workspace
    RepositoryRoot  string             // Root of the git repository
    WorktreePath    string             // Path if this is a worktree
    MainRepoPath    string             // Main repo if this is a worktree
    IsWorktree      bool               // True if this is a git worktree
    SessionTitle    string             // Title of associated session (if any)
    SessionStatus   session.Status     // Status of associated session
    VCSType         vc.VCSType         // Git or Jujutsu
    LastChecked     time.Time          // When status was last refreshed
    IsOrphaned      bool               // No active session
}

// WorkspaceStatus extends VCSStatus with workspace-specific info
type WorkspaceStatus struct {
    vc.VCSStatus                        // Embedded VCS status
    WorkspacePath     string            // Path to this workspace
    SessionTitle      string            // Associated session (if any)
    SessionStatus     session.Status    // Session state
    IsOrphaned        bool              // No active session
    LastActivity      time.Time         // Last file modification
    NeedsAttention    bool              // Has issues requiring action
    AttentionReason   string            // Why attention is needed
}

// ChangesSummary provides aggregated statistics
type ChangesSummary struct {
    TotalRepositories   int
    TotalWorkspaces     int
    TotalUncommitted    int
    TotalUntracked      int
    TotalStaged         int
    TotalConflicts      int
    WorkspacesWithWork  int             // Workspaces with any pending changes
    OrphanedWorkspaces  int             // Workspaces without active sessions
}
```

### API Design

#### gRPC Service

```protobuf
// Add to proto/session/v1/session.proto

message GetWorkspaceStatusRequest {
    // Optional filter by repository path
    string repository_path = 1;
    // Include orphaned workspaces (no active session)
    bool include_orphaned = 2;
}

message GetWorkspaceStatusResponse {
    repeated WorkspaceStatusInfo workspaces = 1;
    WorkspaceSummary summary = 2;
}

message WorkspaceStatusInfo {
    string path = 1;
    string repository_root = 2;
    string worktree_path = 3;
    bool is_worktree = 4;
    string session_title = 5;
    string session_status = 6;
    bool is_orphaned = 7;
    VCSStatus vcs_status = 8;
    string last_checked = 9;
    bool needs_attention = 10;
    string attention_reason = 11;
}

message WorkspaceSummary {
    int32 total_repositories = 1;
    int32 total_workspaces = 2;
    int32 total_uncommitted = 3;
    int32 total_untracked = 4;
    int32 total_staged = 5;
    int32 total_conflicts = 6;
    int32 workspaces_with_work = 7;
    int32 orphaned_workspaces = 8;
}

// Add to SessionService
service SessionService {
    // ... existing methods ...
    rpc GetWorkspaceStatus(GetWorkspaceStatusRequest) returns (GetWorkspaceStatusResponse);
    rpc RefreshWorkspaceStatus(RefreshWorkspaceStatusRequest) returns (RefreshWorkspaceStatusResponse);
}
```

#### TUI Key Bindings

```go
// Location: keys/keys.go

// Add new key binding
const (
    // ... existing keys ...
    KeyWorkspaceStatus KeyName = "workspace_status"
)

// Add to GlobalKeyStringsMap
var GlobalKeyStringsMap = map[string]KeyName{
    // ... existing mappings ...
    "W": KeyWorkspaceStatus, // Shift+W for workspace status
}
```

### UI Design

#### TUI Workspace Status Overlay

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Workspace Status                                            [?] Help   │
├─────────────────────────────────────────────────────────────────────────┤
│ Summary: 3 repos, 5 workspaces, 12 uncommitted, 3 untracked            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ ▼ ~/projects/claude-squad (main)                          2 sessions   │
│   ├─ main                           ✓ clean                            │
│   │  └─ Session: "main-session"     ● running                          │
│   └─ feature-branch                 ● 5 staged, 3 modified             │
│      └─ Session: "feature-work"     ⏸ paused                           │
│                                                                         │
│ ▼ ~/projects/webapp (develop)                             1 session    │
│   └─ ~/.claude-squad/worktrees/api-refactor                            │
│      ├─ Branch: cs/api-refactor     ○ 4 untracked, 2 modified          │
│      └─ Session: "api-refactor"     ● running                          │
│                                                                         │
│ ▶ ~/projects/utils (main)           ⚠ 1 orphaned worktree              │
│                                                                         │
├─────────────────────────────────────────────────────────────────────────┤
│ [j/k] Navigate  [Enter] Go to Session  [r] Refresh  [ESC] Close        │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Web UI Dashboard

```tsx
// Location: web-app/src/app/workspaces/page.tsx

interface WorkspaceDashboardProps {
    workspaces: WorkspaceStatusInfo[];
    summary: WorkspaceSummary;
    onRefresh: () => void;
    onNavigateToSession: (sessionTitle: string) => void;
}

// Card layout showing:
// - Repository name and path
// - List of worktrees/branches
// - Change counts with colored badges
// - Associated session status
// - Last activity timestamp
// - "Needs attention" warning indicators
```

### Implementation Strategy

#### Phase 0: Distributed Locking Foundation (Story 2b) - PREREQUISITE FOR MULTI-POD

1. Implement `DistributedLock` interface with PostgreSQL/SQLite backends
2. Create `CacheInvalidationNotifier` for cross-pod coordination
3. Add circuit breaker for graceful degradation
4. **Note**: This phase is REQUIRED before deploying to multi-pod environments

#### Phase 1: Core Infrastructure (Story 1-3)

1. Implement `WorkspaceRegistry` for tracking workspaces
2. Create `WorkspaceStatusCache` for efficient status retrieval
3. Integrate distributed locking into cache operations
4. Add workspace discovery from existing sessions

#### Phase 2: Status Collection (Story 4-6)

1. Extend VCSProvider for batch status collection
2. Implement background status refresh worker
3. Add orphaned workspace detection

#### Phase 3: TUI Integration (Story 7-9)

1. Create `WorkspaceStatusOverlay` component
2. Add keyboard navigation and shortcuts
3. Implement drill-down to session detail

#### Phase 4: Web UI Dashboard (Story 10-12)

1. Create workspace dashboard page
2. Implement repository cards and filtering
3. Add real-time status updates via WebSocket

## Stories & Tasks

### Epic: Workspace Status Visualization

#### Story 1: Workspace Registry Core

**Description**: As a developer, I want the system to track all workspaces associated with my sessions so that I can query their status centrally.

**Acceptance Criteria**:
- Given sessions exist, when the registry initializes, then all session workspaces are registered
- Given a new session is created, when it starts, then its workspace is added to the registry
- Given a session is deleted, when cleanup completes, then the workspace is marked as orphaned

**Tasks**:
1. Create `session/workspace/registry.go` with `WorkspaceRegistry` struct
2. Implement `RegisterWorkspace()` and `UnregisterWorkspace()` methods
3. Add workspace grouping by repository root
4. Create tests for registry operations

**Context Boundary**: session/workspace package (new)

#### Story 2: Workspace Status Cache

**Description**: As a developer, I want workspace status to be cached so that status queries are fast and don't block the UI.

**Acceptance Criteria**:
- Given a workspace status request, when cache is fresh (<30s), then cached status is returned
- Given cache is stale, when status is requested, then background refresh is triggered
- Given git command fails, when status is requested, then last known status with error flag is returned

**Tasks**:
1. Create `WorkspaceStatusCache` with TTL-based invalidation
2. Implement async status refresh with worker pool
3. Add error handling and graceful degradation
4. Create tests for cache behavior

**Context Boundary**: session/workspace package

#### Story 2b: Distributed Locking Infrastructure (Multi-Pod Support)

**Description**: As a platform engineer, I want workspace operations to be coordinated across multiple pods so that concurrent status updates don't cause race conditions or data corruption.

**Acceptance Criteria**:
- Given multiple pods running, when status refresh triggers, then only one pod performs the refresh
- Given a lock is held, when another pod requests it, then it waits or fails gracefully with timeout
- Given a pod crashes while holding lock, when timeout expires, then lock is automatically released
- Given cache is updated on one pod, when notification fires, then other pods invalidate their caches
- Given PostgreSQL is configured, when acquiring locks, then advisory locks are used
- Given SQLite is configured, when acquiring locks, then transaction-level locking is used

**Tasks**:
1. Create `session/workspace/lock.go` with `DistributedLock` interface
2. Implement `PostgresAdvisoryLock` using `pg_advisory_lock()`/`pg_try_advisory_lock()`
3. Implement `SQLiteTransactionLock` using `BEGIN IMMEDIATE` transactions
4. Implement `NoOpLock` for testing and single-pod deployments
5. Create `CacheInvalidationNotifier` interface
6. Implement `PostgresListenNotify` using `LISTEN`/`NOTIFY`
7. Implement `PollingNotifier` for SQLite (poll-based invalidation)
8. Add lock key derivation from workspace paths (consistent hashing)
9. Add circuit breaker for lock service failures (graceful degradation)
10. Create integration tests with concurrent access patterns
11. Add `-race` flag to CI for detecting data races

**Context Boundary**: session/workspace package (new files)

**Code Structure**:
```go
// session/workspace/lock.go
type PostgresAdvisoryLock struct {
    db     *sql.DB
    prefix int64  // namespace prefix for lock keys
}

func (l *PostgresAdvisoryLock) Acquire(ctx context.Context, resource string, timeout time.Duration) (LockHandle, error) {
    // Hash resource to int64 key
    key := hashToInt64(l.prefix, resource)

    // Use pg_advisory_lock with timeout context
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    _, err := l.db.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key)
    if err != nil {
        return nil, fmt.Errorf("failed to acquire advisory lock: %w", err)
    }

    return &postgresLockHandle{db: l.db, key: key}, nil
}

// session/workspace/notify.go
type PostgresListenNotify struct {
    pool    *pgxpool.Pool
    channel string
}

func (n *PostgresListenNotify) Subscribe(ctx context.Context, handler func(workspacePath string)) error {
    conn, err := n.pool.Acquire(ctx)
    if err != nil {
        return err
    }

    _, err = conn.Exec(ctx, fmt.Sprintf("LISTEN %s", n.channel))
    if err != nil {
        return err
    }

    go func() {
        for {
            notification, err := conn.Conn().WaitForNotification(ctx)
            if err != nil {
                return // context cancelled or connection lost
            }
            handler(notification.Payload)
        }
    }()

    return nil
}
```

**Database Schema** (PostgreSQL):
```sql
-- Optional: Track active locks for monitoring/debugging
CREATE TABLE IF NOT EXISTS workspace_locks (
    lock_key BIGINT PRIMARY KEY,
    workspace_path TEXT NOT NULL,
    acquired_by TEXT NOT NULL,  -- pod identifier
    acquired_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE
);

-- Index for cleanup queries
CREATE INDEX idx_workspace_locks_expires ON workspace_locks(expires_at);
```

#### Story 3: Session Integration

**Description**: As a developer, I want the workspace registry to be integrated with session lifecycle so that workspaces are tracked automatically.

**Acceptance Criteria**:
- Given session creation, when `Instance.Start()` completes, then workspace is registered
- Given session deletion, when `Instance.Destroy()` completes, then workspace status updates
- Given session pause/resume, when status changes, then workspace tracks session status

**Tasks**:
1. Add `WorkspaceRegistry` dependency to `Storage` and `Server`
2. Hook registry updates into session lifecycle methods
3. Update `FromInstanceData()` to register restored sessions
4. Create integration tests

**Context Boundary**: session package modifications

#### Story 4: VCS Status Batch Collection

**Description**: As a developer, I want to efficiently collect VCS status for multiple workspaces so that the dashboard updates quickly.

**Acceptance Criteria**:
- Given 10 workspaces, when status is refreshed, then all statuses return within 3 seconds
- Given a workspace path, when git status is collected, then all change categories are populated
- Given a non-git directory, when status is requested, then appropriate error is returned

**Tasks**:
1. Add `BatchGetStatus()` method to VCS provider interface
2. Implement parallel status collection with worker pool
3. Add timeout handling for slow repositories
4. Create benchmarks for batch operations

**Context Boundary**: session/vc package modifications

#### Story 5: Orphaned Workspace Detection

**Description**: As a developer, I want to be alerted about orphaned worktrees so that I don't lose uncommitted work.

**Acceptance Criteria**:
- Given a worktree without active session, when scan runs, then it's marked as orphaned
- Given an orphaned worktree with changes, when displayed, then attention indicator appears
- Given worktree cleanup, when session is properly deleted, then worktree is not marked orphaned

**Tasks**:
1. Create `DetectOrphanedWorktrees()` method
2. Scan claude-squad worktree directory for orphans
3. Match worktrees against active sessions
4. Add orphan status to workspace display

**Context Boundary**: session/workspace package

#### Story 6: Background Status Refresh

**Description**: As a developer, I want workspace status to refresh automatically so that I always see current information.

**Acceptance Criteria**:
- Given the dashboard is open, when 30 seconds pass, then all stale statuses refresh
- Given a session with activity, when terminal output changes, then status refreshes
- Given system load is high, when refresh triggers, then rate limiting prevents overload

**Tasks**:
1. Create background refresh worker goroutine
2. Implement adaptive refresh intervals based on activity
3. Add rate limiting and backoff for git operations
4. Create shutdown handling for graceful cleanup

**Context Boundary**: session/workspace package

#### Story 7: TUI Workspace Overlay

**Description**: As a TUI user, I want to press a key to see all workspace statuses so that I can identify pending work at a glance.

**Acceptance Criteria**:
- Given I press `W`, when overlay opens, then I see all workspaces grouped by repository
- Given a workspace has changes, when displayed, then change counts are visible
- Given I press `ESC`, when overlay closes, then I return to previous view

**Tasks**:
1. Create `ui/overlay/workspaceStatusOverlay.go`
2. Implement repository/worktree hierarchical rendering
3. Add change count badges and status indicators
4. Integrate with app state machine

**Context Boundary**: ui/overlay package

#### Story 8: TUI Navigation

**Description**: As a TUI user, I want to navigate the workspace overlay with keyboard so that I can drill down to specific sessions.

**Acceptance Criteria**:
- Given workspace overlay is open, when I press `j/k`, then selection moves up/down
- Given a workspace is selected, when I press `Enter`, then I navigate to its session
- Given collapsed repository, when I press `Enter`, then it expands/collapses

**Tasks**:
1. Add keyboard event handling for navigation
2. Implement expand/collapse for repository groups
3. Add "Go to Session" navigation action
4. Create snapshot tests for overlay states

**Context Boundary**: ui/overlay package

#### Story 9: TUI Status Refresh

**Description**: As a TUI user, I want to refresh workspace status on demand so that I can see the latest changes.

**Acceptance Criteria**:
- Given I press `r`, when refresh starts, then loading indicator appears
- Given refresh completes, when status updates, then view refreshes automatically
- Given refresh fails, when error occurs, then error message is displayed

**Tasks**:
1. Add refresh key binding to overlay
2. Implement loading state with spinner
3. Add error display for failed refreshes
4. Create tests for refresh behavior

**Context Boundary**: ui/overlay package

#### Story 10: Web Dashboard Page

**Description**: As a web UI user, I want a dedicated workspace dashboard page so that I can see all my pending work.

**Acceptance Criteria**:
- Given I navigate to `/workspaces`, when page loads, then workspace cards are displayed
- Given workspaces exist, when page loads, then summary statistics appear
- Given a workspace card, when I click it, then I navigate to the associated session

**Tasks**:
1. Create `web-app/src/app/workspaces/page.tsx`
2. Implement workspace card component
3. Add summary header with statistics
4. Integrate with navigation

**Context Boundary**: web-app package

#### Story 11: Web Dashboard Filtering

**Description**: As a web UI user, I want to filter workspaces by various criteria so that I can focus on specific repositories.

**Acceptance Criteria**:
- Given filter controls, when I select "Has Changes", then only workspaces with changes appear
- Given I type a repository name, when search matches, then results are filtered
- Given filters are applied, when I clear, then all workspaces appear

**Tasks**:
1. Add filter controls to dashboard page
2. Implement repository name search
3. Add status filters (has changes, orphaned, conflicts)
4. Create filter state management

**Context Boundary**: web-app package

#### Story 12: Web Real-time Updates

**Description**: As a web UI user, I want workspace status to update in real-time so that I see changes without manual refresh.

**Acceptance Criteria**:
- Given dashboard is open, when workspace status changes, then card updates automatically
- Given WebSocket disconnects, when reconnected, then full refresh occurs
- Given browser tab is inactive, when tab becomes active, then stale data refreshes

**Tasks**:
1. Add workspace status to WebSocket event types
2. Implement status broadcast on change
3. Add visibility-aware refresh logic
4. Create integration tests for real-time updates

**Context Boundary**: web-app and server packages

## Known Issues & Risk Mitigation

### Potential Bugs Identified During Planning

#### Bug 1: Race Condition in Status Cache [SEVERITY: Medium]

**Description**: Concurrent cache reads and writes during parallel status refresh may cause data corruption or stale reads.

**Mitigation**:
- Use `sync.RWMutex` for cache access
- Implement copy-on-write for cache entries
- Add integration tests with concurrent access patterns

**Files Likely Affected**:
- `session/workspace/cache.go`
- `session/workspace/registry.go`

**Prevention Strategy**:
- Design cache with thread-safety from the start
- Use immutable value objects for status data
- Add `-race` flag to CI test runs

#### Bug 2: Git Operation Timeout [SEVERITY: High]

**Description**: Large repositories with many changes may cause git status commands to timeout, blocking the UI.

**Mitigation**:
- Set explicit timeouts on all git commands (5s default)
- Use git's `--porcelain` format for faster parsing
- Implement incremental status updates
- Add monitoring for slow git operations

**Files Likely Affected**:
- `session/vc/git_provider.go`
- `session/workspace/refresh.go`

**Prevention Strategy**:
- Wrap all git exec calls with context.WithTimeout
- Return partial results with error indicator
- Log slow operations for performance analysis

#### Bug 3: Orphan Detection False Positives [SEVERITY: Medium]

**Description**: Worktrees may be incorrectly marked as orphaned during session restart or pause operations.

**Mitigation**:
- Add grace period before marking worktree as orphaned
- Check session.json before marking orphaned
- Implement "claimed by" field in worktree metadata

**Files Likely Affected**:
- `session/workspace/orphan_detector.go`
- `session/instance.go`

**Prevention Strategy**:
- Create test scenarios for session lifecycle edge cases
- Add explicit worktree ownership tracking
- Implement debounced orphan detection (30s delay)

#### Bug 4: Memory Leak in Status Cache [SEVERITY: Medium]

**Description**: Status cache may grow unbounded if workspaces are removed but cache entries are not cleaned.

**Mitigation**:
- Implement LRU eviction for cache entries
- Add periodic cache cleanup goroutine
- Set maximum cache size limit

**Files Likely Affected**:
- `session/workspace/cache.go`

**Prevention Strategy**:
- Use a bounded cache implementation from the start
- Add metrics for cache size monitoring
- Create benchmarks for memory usage

#### Bug 5: WebSocket Flood During Refresh [SEVERITY: Low]

**Description**: Refreshing many workspaces simultaneously may flood WebSocket connections with updates, causing UI lag.

**Mitigation**:
- Batch status updates into single WebSocket message
- Add client-side debouncing for status updates
- Implement server-side rate limiting

**Files Likely Affected**:
- `server/websocket.go`
- `web-app/src/lib/contexts/WorkspaceContext.tsx`

**Prevention Strategy**:
- Design WebSocket protocol with batching from the start
- Add update coalescing with 100ms window
- Monitor WebSocket message rates in production

#### Bug 6: Distributed Lock Contention [SEVERITY: Medium]

**Description**: In multi-pod deployments, high contention for workspace locks may cause timeouts and degraded performance, particularly during bulk status refreshes or when multiple users modify the same workspace.

**Mitigation**:
- Use fine-grained locks (per-workspace path, not global)
- Implement exponential backoff with jitter for lock retries
- Add lock acquisition metrics for monitoring hotspots
- Use `TryAcquire()` with immediate fallback to cached data
- Consider read-write lock semantics (shared read locks, exclusive write locks)

**Files Likely Affected**:
- `session/workspace/lock.go`
- `session/workspace/registry.go`
- `session/workspace/cache.go`

**Prevention Strategy**:
- Design lock granularity based on workspace path from the start
- Implement circuit breaker to avoid cascade failures
- Add lock wait time metrics to identify bottlenecks early
- Use advisory lock namespacing to avoid conflicts with other applications
- Test with synthetic contention in staging environment

#### Bug 7: PostgreSQL Connection Pool Exhaustion [SEVERITY: High]

**Description**: LISTEN connections are long-lived and may exhaust the connection pool if not managed properly. Each pod needs a dedicated connection for notifications.

**Mitigation**:
- Use separate connection pool for LISTEN connections (min 1, max 2 per pod)
- Implement connection health monitoring with automatic reconnection
- Add connection pool metrics to detect exhaustion
- Use `pgxpool` with connection lifetime management

**Files Likely Affected**:
- `session/workspace/notify.go`
- Database connection configuration

**Prevention Strategy**:
- Configure separate pool for notification listeners from the start
- Set appropriate `max_connections` in PostgreSQL
- Monitor `pg_stat_activity` for connection usage
- Implement graceful shutdown to release connections

### Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Git command performance | Medium | High | Timeouts, caching, incremental updates |
| Large repo support | Medium | Medium | Limit file count, summary only mode |
| Cross-platform git paths | Low | Medium | Use filepath package consistently |
| VCS detection accuracy | Low | Low | Fallback to directory session info |
| WebSocket scalability | Low | Medium | Rate limiting, client batching |
| Distributed lock contention | Medium | Medium | Fine-grained locks, exponential backoff |
| PostgreSQL connection exhaustion | Low | High | Separate pools, monitoring |
| Cache invalidation delays | Medium | Low | Short TTL, aggressive invalidation |

## Testing Strategy

### Unit Tests

- WorkspaceRegistry CRUD operations
- Status cache TTL and eviction
- Orphan detection logic
- VCS status parsing
- Distributed lock interface implementations (mock backends)
- Lock key hashing consistency
- Cache invalidation handler logic

### Integration Tests

- End-to-end workspace discovery
- Session lifecycle integration
- Background refresh worker
- WebSocket status broadcasts
- **Multi-pod coordination tests**:
  - Concurrent lock acquisition (PostgreSQL testcontainer)
  - LISTEN/NOTIFY message delivery
  - Lock timeout and recovery
  - Cache invalidation propagation across connections
  - Connection pool behavior under load

### Concurrency Tests (with `-race` flag)

- Concurrent workspace registration/unregistration
- Parallel status refresh operations
- Cache read/write contention
- Lock acquire/release race conditions

### Performance Benchmarks

- Batch status collection (10, 50, 100 repos)
- Cache hit/miss performance
- Memory usage under load
- WebSocket message throughput

### Snapshot Tests

- TUI overlay rendering
- Various status combinations
- Error state display

## Dependencies

### Internal Dependencies

- `session/vc` package for VCS operations
- `session` package for Instance integration
- `ui/overlay` for TUI components
- `server` package for gRPC endpoints
- `web-app` for dashboard UI

### External Dependencies

- None new for single-pod deployment (uses existing git/jj integration)

### External Dependencies (Multi-Pod Deployment)

- **PostgreSQL 12+** (required for advisory locks and LISTEN/NOTIFY)
- `github.com/jackc/pgx/v5` - PostgreSQL driver with native LISTEN/NOTIFY support
- `github.com/testcontainers/testcontainers-go` - Integration testing with PostgreSQL

### Database Backend Selection

| Deployment Mode | Database | Locking Strategy | Cache Invalidation |
|-----------------|----------|------------------|-------------------|
| Single-pod/Dev  | SQLite   | `BEGIN IMMEDIATE` | Poll-based (5s TTL) |
| Multi-pod/Prod  | PostgreSQL | Advisory locks | LISTEN/NOTIFY |

**Configuration**:
```yaml
# config.yaml
database:
  type: "postgresql"  # or "sqlite"
  connection_string: "postgres://user:pass@host:5432/claudesquad"

  # PostgreSQL-specific
  advisory_lock_namespace: 1000  # Unique namespace for this app
  listen_channel: "workspace_invalidation"

  # Connection pool settings
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: "1h"
  listen_pool_size: 2  # Dedicated pool for LISTEN connections
```

## Rollout Plan

### Phase 0: Distributed Locking Foundation (MULTI-POD PREREQUISITE)

- Implement `DistributedLock` interface
- Create PostgreSQL advisory lock implementation
- Create SQLite transaction lock implementation
- Implement `CacheInvalidationNotifier`
- Add integration tests with testcontainers
- **Gate**: Multi-pod deployment blocked until this phase complete

### Phase 1: Backend Infrastructure

- Implement WorkspaceRegistry and cache
- Integrate distributed locking into registry
- Add session lifecycle integration
- Create gRPC endpoint

### Phase 2: TUI Implementation

- Create workspace overlay
- Add keyboard navigation
- Integrate with app state

### Phase 3: Web Dashboard

- Create dashboard page
- Add filtering and search
- Implement real-time updates

### Phase 4: Polish & Documentation

- Performance optimization
- User documentation
- Error handling improvements
- Operational runbook for multi-pod deployment

## Success Metrics

- Users can view all workspace status with single keystroke
- Status information loads within 2 seconds for typical usage (5-10 sessions)
- Orphaned worktrees are detected within 60 seconds of session deletion
- Web dashboard provides equivalent functionality to TUI overlay
- Zero data loss incidents from untracked uncommitted changes

## References

### Codebase Files

- `/Users/tylerstapler/IdeaProjects/claude-squad/session/instance.go` - Session instance model
- `/Users/tylerstapler/IdeaProjects/claude-squad/session/storage.go` - Session persistence
- `/Users/tylerstapler/IdeaProjects/claude-squad/session/vc/types.go` - VCS type definitions
- `/Users/tylerstapler/IdeaProjects/claude-squad/session/vc/git_provider.go` - Git operations
- `/Users/tylerstapler/IdeaProjects/claude-squad/ui/overlay/gitStatusOverlay.go` - Existing git overlay
- `/Users/tylerstapler/IdeaProjects/claude-squad/web-app/src/components/sessions/VcsPanel.tsx` - Web VCS panel

### ADRs & Documentation

- `docs/adr/004-fugitive-inspired-git-integration.md` - Git integration patterns
- `docs/tasks/session-workspace-management.md` - Related workspace feature plan
