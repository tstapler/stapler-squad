# SQLite Schema Normalization with Domain-Driven Design

## Executive Summary

This document presents a revised domain-driven approach to normalizing the SQLite schema for stapler-squad. The design separates the core Session entity (containing only universally required fields) from optional contexts that are attached based on deployment scenarios. This enables support for diverse environments including local git-based development, ephemeral containers, cloud instances, and headless API-driven sessions.

## Vision & Principles

### Deployment Scenarios to Support

1. **Local Git Development** (Current primary use case)
   - Full filesystem access with git worktrees
   - TTY-based terminal sessions via tmux
   - Rich UI with categorization and tags

2. **Ephemeral Containers**
   - No persistent filesystem
   - Session state stored centrally
   - Minimal core requirements

3. **Cloud/Web Instances** (claude-web style)
   - Browser-based interaction
   - No local terminal or filesystem
   - API-driven communication

4. **Non-Git Projects**
   - Simple directory-based sessions
   - No version control integration
   - Basic file editing capabilities

5. **Headless/API Sessions**
   - Programmatic interaction only
   - No UI state or preferences
   - Minimal metadata requirements

### Design Principles

1. **Core Minimalism**: The core session table contains only fields that EVERY session type requires
2. **Context Separation**: Optional functionality lives in context tables with 1:1 relationships
3. **Lazy Context Creation**: Context records are created only when needed for that session type
4. **Query Optimization**: Use LEFT JOINs to load only relevant contexts
5. **Extensibility**: New contexts can be added without modifying existing schema
6. **Zero Null Fields**: Core table has no nullable columns; contexts handle optionality

## Domain Model

### Core Session Entity

The `sessions` table contains only universally required fields:

```sql
CREATE TABLE sessions (
    -- Identity
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT UNIQUE NOT NULL,          -- Human-readable unique identifier
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    
    -- Process
    status INTEGER NOT NULL,              -- Running/Paused/Stopped enum
    program TEXT NOT NULL,                -- claude/aider/etc
    
    -- Configuration
    auto_yes INTEGER NOT NULL DEFAULT 0,  -- Auto-confirm prompts
    prompt TEXT NOT NULL DEFAULT ''       -- Initial prompt (empty string if none)
);
```

**Key Decisions:**
- `title` serves as the primary human identifier (unique constraint)
- `status` uses integer enum for efficiency
- `prompt` defaults to empty string to avoid nulls
- No path, branch, or terminal fields - these are context-specific

### Optional Context Tables

#### 1. GitContext - Git Integration

For sessions with version control:

```sql
CREATE TABLE git_context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    branch TEXT NOT NULL,
    base_commit_sha TEXT NOT NULL DEFAULT '',
    
    -- Worktree information (if applicable)
    worktree_id INTEGER,  -- FK to worktrees table
    
    -- GitHub PR integration (if applicable)
    pr_number INTEGER,
    pr_url TEXT,
    owner TEXT,
    repo TEXT,
    source_ref TEXT,
    
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (worktree_id) REFERENCES worktrees(id) ON DELETE SET NULL
);
```

#### 2. FilesystemContext - Local File Access

For sessions with filesystem access:

```sql
CREATE TABLE filesystem_context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    
    -- Paths
    project_path TEXT NOT NULL,        -- Root project/repo directory
    working_dir TEXT NOT NULL,         -- Current working directory
    
    -- Worktree detection
    is_worktree INTEGER NOT NULL DEFAULT 0,
    main_repo_path TEXT,               -- Parent repo if this is a worktree
    
    -- Cloned repos (for external PRs)
    cloned_repo_path TEXT,
    
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

#### 3. TerminalContext - TTY/Terminal State

For sessions with terminal interfaces:

```sql
CREATE TABLE terminal_context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    
    -- Dimensions
    height INTEGER NOT NULL DEFAULT 24,
    width INTEGER NOT NULL DEFAULT 80,
    
    -- Tmux integration
    tmux_session_name TEXT,
    tmux_prefix TEXT,
    tmux_socket TEXT,  -- For isolated tmux servers
    
    -- Terminal type
    terminal_type TEXT NOT NULL DEFAULT 'tmux',  -- tmux/mux/pty/web
    
    -- Scrollback reference (actual content in separate table)
    last_scrollback_id INTEGER,
    
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

#### 4. UIPreferences - Client-Side Display

For UI presentation (could eventually move client-side):

```sql
CREATE TABLE ui_preferences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    
    -- Organization
    category TEXT,
    is_expanded INTEGER NOT NULL DEFAULT 1,
    
    -- Display preferences
    grouping_strategy TEXT,
    sort_order TEXT,
    
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

#### 5. ActivityTracking - Notification/Queue Features

For review queue and activity monitoring:

```sql
CREATE TABLE activity_tracking (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    
    -- Terminal activity
    last_terminal_update DATETIME,
    last_meaningful_output DATETIME,
    last_output_signature TEXT,        -- Hash to detect actual changes
    
    -- Review queue
    last_added_to_queue DATETIME,
    last_viewed DATETIME,
    last_acknowledged DATETIME,
    
    -- Metrics
    total_output_lines INTEGER DEFAULT 0,
    session_duration_seconds INTEGER DEFAULT 0,
    
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

#### 6. CloudContext - Cloud/API Sessions

For cloud-hosted or API-driven sessions:

```sql
CREATE TABLE cloud_context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    
    -- Cloud provider info
    provider TEXT NOT NULL,            -- aws/gcp/azure/custom
    region TEXT,
    instance_id TEXT,
    
    -- API access
    api_endpoint TEXT,
    api_key_ref TEXT,                  -- Reference to secure key storage
    
    -- Session persistence
    cloud_session_id TEXT,
    conversation_id TEXT,
    
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

### Existing Tables (Retained)

These tables already follow good normalization practices:

- **worktrees** - Git worktree management (unchanged)
- **tags** - Tag definitions (unchanged)
- **session_tags** - Many-to-many tag relationships (unchanged)
- **diff_stats** - Git diff statistics (unchanged)
- **claude_sessions** - Claude-specific session data (unchanged)
- **claude_metadata** - Key-value metadata (unchanged)

## Query Patterns

### Loading Sessions by Deployment Type

#### Local Git Development (Full Context)
```sql
SELECT 
    s.*,
    g.branch, g.pr_number,
    f.project_path, f.working_dir,
    t.height, t.width, t.tmux_session_name,
    u.category, u.is_expanded,
    a.last_meaningful_output
FROM sessions s
LEFT JOIN git_context g ON s.id = g.session_id
LEFT JOIN filesystem_context f ON s.id = f.session_id
LEFT JOIN terminal_context t ON s.id = t.session_id
LEFT JOIN ui_preferences u ON s.id = u.session_id
LEFT JOIN activity_tracking a ON s.id = a.session_id
WHERE s.title = ?
```

#### Cloud/API Session (Minimal Context)
```sql
SELECT 
    s.*,
    c.provider, c.api_endpoint, c.cloud_session_id
FROM sessions s
LEFT JOIN cloud_context c ON s.id = c.session_id
WHERE s.title = ?
```

#### Ephemeral Container (Core Only)
```sql
SELECT * FROM sessions WHERE title = ?
```

### Efficient List Queries

#### List with Minimal Joins (UI Display)
```sql
SELECT s.*, u.category
FROM sessions s
LEFT JOIN ui_preferences u ON s.id = u.session_id
WHERE s.status IN (?, ?)
ORDER BY s.updated_at DESC
LIMIT ?
```

#### Activity-Based Queries (Review Queue)
```sql
SELECT s.*, a.last_meaningful_output
FROM sessions s
INNER JOIN activity_tracking a ON s.id = a.session_id
WHERE a.last_meaningful_output > a.last_acknowledged
  AND a.last_meaningful_output > datetime('now', '-1 hour')
ORDER BY a.last_meaningful_output DESC
```

## Migration Strategy

### Phase 1: Non-Breaking Addition (Week 1)

1. **Create new context tables** alongside existing schema
2. **Add migration infrastructure** with version tracking
3. **Implement dual-write mode** in repository layer
4. **Deploy with feature flag** for gradual rollout

### Phase 2: Data Migration (Week 2)

1. **Batch migration script** with progress tracking
2. **Verification checksums** for data integrity
3. **Parallel migration** of different contexts
4. **Rollback savepoints** at each stage

### Phase 3: Code Updates (Week 3)

1. **Repository refactoring** to use context pattern
2. **Query optimization** with selective JOINs
3. **LoadOptions pattern** for context selection
4. **Performance testing** with production data

### Phase 4: Cleanup (Week 4)

1. **Remove deprecated columns** from sessions table
2. **Drop COALESCE workarounds** in queries
3. **Remove dual-write code**
4. **Update documentation**

## Implementation Tasks

### Core Infrastructure
- [ ] Create context table schemas
- [ ] Design ContextLoader interface
- [ ] Implement LoadOptions for selective loading
- [ ] Add connection pooling configuration
- [ ] Create migration framework with rollback

### Context Implementations
- [ ] GitContext with worktree integration
- [ ] FilesystemContext with path validation
- [ ] TerminalContext with tmux/mux support
- [ ] UIPreferences with client-side sync
- [ ] ActivityTracking with metrics
- [ ] CloudContext for API sessions

### Repository Updates
- [ ] Refactor Create() to populate contexts
- [ ] Update Get() with LoadOptions
- [ ] Optimize List() with minimal joins
- [ ] Add context-specific update methods
- [ ] Implement batch operations

### Migration Execution
- [ ] Write migration scripts for each context
- [ ] Create verification queries
- [ ] Implement progress logging
- [ ] Add dry-run mode
- [ ] Create rollback procedures

## Architecture Decision Records

### ADR-001: Separate Core from Contexts

**Status**: Accepted

**Context**: The current schema mixes required and optional fields, causing null handling issues and limiting deployment flexibility.

**Decision**: Separate universally required fields into a minimal core table, with optional functionality in context tables.

**Consequences**:
- ✅ Supports diverse deployment scenarios
- ✅ Eliminates null handling in core fields
- ✅ Enables lazy context creation
- ⚠️ Requires JOIN operations for full data
- ⚠️ More complex repository implementation

### ADR-002: One-to-One Context Relationships

**Status**: Accepted

**Context**: Need to decide between embedding optional data vs normalizing into separate tables.

**Decision**: Use 1:1 relationships with UNIQUE constraints on session_id in context tables.

**Consequences**:
- ✅ Clear separation of concerns
- ✅ Easy to add new contexts
- ✅ Efficient storage (no empty columns)
- ⚠️ Multiple JOINs for full session load
- ⚠️ More tables to maintain

### ADR-003: LoadOptions Pattern

**Status**: Accepted

**Context**: Loading all contexts for every query would be inefficient.

**Decision**: Implement LoadOptions to specify which contexts to fetch.

**Consequences**:
- ✅ Optimized queries for specific use cases
- ✅ Reduced memory footprint
- ✅ Better performance for list operations
- ⚠️ More complex API
- ⚠️ Risk of N+1 queries if misused

### ADR-004: Keep Existing Specialized Tables

**Status**: Accepted

**Context**: Tables like worktrees, tags, and diff_stats are already well-normalized.

**Decision**: Retain existing specialized tables, only refactor the main sessions table.

**Consequences**:
- ✅ Minimal migration effort for specialized data
- ✅ Preserves existing relationships
- ✅ Backward compatibility maintained
- ⚠️ Mixed patterns (some data in contexts, some in specialized tables)

## Performance Considerations

### Query Optimization

1. **Selective JOINs**: Only join tables needed for the specific operation
2. **Index Strategy**: Create indexes on foreign keys and commonly queried fields
3. **Connection Pooling**: Configure appropriate pool sizes for concurrent access
4. **Query Caching**: Cache frequently accessed context combinations

### Benchmarks

**Target Performance** (for 10,000 sessions):
- Core-only query: < 10ms
- Single context join: < 20ms
- Full context load: < 50ms
- List with UI context: < 100ms
- Migration completion: < 30 seconds

## Risk Mitigation

### Data Migration Risks

**Risk**: Data corruption during migration
**Mitigation**:
- Transactional migration with savepoints
- Pre-migration backup
- Checksums at each stage
- Dry-run mode for testing

### Performance Risks

**Risk**: Query performance degradation
**Mitigation**:
- Comprehensive indexing strategy
- Query plan analysis
- Performance benchmarks before/after
- Caching layer for hot paths

### Compatibility Risks

**Risk**: Breaking existing functionality
**Mitigation**:
- Dual-write mode during transition
- Feature flags for gradual rollout
- Comprehensive test coverage
- Rollback procedures

## Success Metrics

1. **Zero Data Loss**: All sessions migrated successfully
2. **Performance**: No more than 10% latency increase
3. **Flexibility**: Support for 3+ new deployment scenarios
4. **Code Quality**: Elimination of all NULL-related workarounds
5. **Maintainability**: 50% reduction in schema modification complexity

## Next Steps

1. **Review & Approval**: Architecture team review of domain model
2. **Prototype**: Build proof-of-concept with core + 2 contexts
3. **Performance Testing**: Benchmark with production-like data
4. **Migration Plan**: Detailed step-by-step migration runbook
5. **Implementation**: Phased rollout with monitoring

## Comprehensive Architecture Impact Analysis

### System-Wide Impact Overview

This schema normalization represents a fundamental architectural shift affecting 200+ files across all layers of the stapler-squad system. The refactoring touches approximately 15,000 lines of production code with cascading effects through the domain model, repository, service, API, and UI layers.

### Layer-by-Layer Impact Analysis

#### 1. Domain/Model Layer (Critical Impact)
**Affected Files:**
- `session/instance.go` (2,209 lines) - Core session entity requiring complete restructuring
- `session/storage.go` (800+ lines) - Storage interface needs context-aware methods
- `session/types.go` (500+ lines) - Type definitions must be split into contexts

**Changes Required:**
- Split monolithic `Instance` struct into `Session` + 6 context structs
- Implement context interfaces for each optional component
- Add factory methods for context creation
- Implement lazy loading for context data

#### 2. Repository Layer (High Impact)
**Affected Files:**
- `session/sqlite_repository.go` (1,500+ lines) - Complete query rewrite
- `session/sqlite_schema.go` (400+ lines) - New schema v3 definition
- `session/index_store.go` (300+ lines) - Index structure changes

**Changes Required:**
- Implement dual-read/write for migration period
- Create context-aware query builders
- Add LoadOptions pattern throughout
- Optimize JOINs for performance

#### 3. Service Layer (Extensive Impact)
**Affected Methods:** 80+ methods in `instance.go` need context validation

**Critical Service Methods Requiring Updates:**
```go
// Before: Direct field access
func (i *Instance) GetPath() string { return i.Path }

// After: Context-aware access
func (s *Session) GetPath() (string, error) {
    if s.Filesystem == nil {
        return "", ErrNoFilesystemContext
    }
    return s.Filesystem.ProjectPath, nil
}
```

#### 4. Application Layer (Moderate Impact)
**Affected Files:**
- `app/app.go` (3,000+ lines) - State management updates
- `app/services/*.go` (8 files) - Service facade updates
- `app/handleAdvancedSessionSetup.go` - Session creation flow

**Changes Required:**
- Update all session field accesses to use context checks
- Modify session creation to populate appropriate contexts
- Update navigation and filtering logic

#### 5. API/Protocol Layer (Breaking Changes)
**Affected Files:**
- `proto/session/v1/*.proto` (3 files) - New v2 protocol needed
- `server/services/session_service.go` - RPC handler updates
- `server/adapters/instance_adapter.go` - Adapter pattern changes

**New Proto Messages Required:**
```protobuf
message SessionV2 {
    CoreSession core = 1;
    GitContext git = 2;
    FilesystemContext filesystem = 3;
    TerminalContext terminal = 4;
    UIPreferences ui = 5;
    ActivityTracking activity = 6;
    CloudContext cloud = 7;
}
```

#### 6. UI Layers (Significant Updates)

**TUI Layer** (`ui/` directory):
- `ui/list.go` - List rendering with optional contexts
- `ui/session_card.go` - Card display logic
- `ui/overlay/*.go` - Session creation overlays

**Web UI Layer** (`web-app/src/`):
- `lib/hooks/useSessionService.ts` - API client updates
- `components/sessions/*.tsx` - Component updates for new structure
- `lib/types/*.ts` - TypeScript type definitions

### New Go Type Definitions

```go
// Core session with optional contexts
type Session struct {
    // Core fields (always present)
    ID        int64
    Title     string
    Status    Status
    Program   string
    AutoYes   bool
    Prompt    string
    CreatedAt time.Time
    UpdatedAt time.Time
    
    // Optional contexts (nil when not applicable)
    Git        *GitContext
    Filesystem *FilesystemContext
    Terminal   *TerminalContext
    UI         *UIPreferences
    Activity   *ActivityTracking
    Cloud      *CloudContext
}

// Context interfaces for clean separation
type Context interface {
    SessionID() int64
    Validate() error
    IsEmpty() bool
}

type GitContext struct {
    ID            int64
    SessionID     int64
    Branch        string
    BaseCommitSHA string
    WorktreeID    *int64
    PRNumber      *int
    PRUrl         string
    Owner         string
    Repo          string
    SourceRef     string
}

type FilesystemContext struct {
    ID             int64
    SessionID      int64
    ProjectPath    string
    WorkingDir     string
    IsWorktree     bool
    MainRepoPath   string
    ClonedRepoPath string
}
```

### Implementation Phases (13 Weeks Total)

#### Phase 1: Foundation (Weeks 1-2)
**Objective:** Establish new types alongside existing ones

**Tasks:**
- Create new Go type definitions
- Implement context interfaces
- Add feature flags for gradual rollout
- Set up dual-write infrastructure
- Create comprehensive test suite

**Deliverables:**
- New type system in place
- Tests passing with old and new types
- Feature flag system operational

#### Phase 2: Repository Layer (Weeks 3-4)
**Objective:** Implement new schema with backward compatibility

**Tasks:**
- Create schema v3 migration
- Implement dual-read repository pattern
- Add LoadOptions throughout repository
- Create context-aware query builders
- Performance benchmark new queries

**Deliverables:**
- Schema v3 deployed (not active)
- Dual-read/write operational
- Performance benchmarks documented

#### Phase 3: API Evolution (Weeks 5-6)
**Objective:** Create v2 API with context support

**Tasks:**
- Define Proto v2 with context messages
- Implement parallel v1/v2 endpoints
- Create adapter layer for compatibility
- Update gRPC service handlers
- Add context-specific RPCs

**Deliverables:**
- Proto v2 fully defined
- Both API versions operational
- Client SDKs generated

#### Phase 4: Service Refactoring (Weeks 7-8)
**Objective:** Make services context-aware

**Tasks:**
- Refactor 80+ methods in instance.go
- Add context validation throughout
- Implement lazy loading patterns
- Update business logic for contexts
- Create service-level tests

**Deliverables:**
- All services context-aware
- Comprehensive test coverage
- No regressions in functionality

#### Phase 5: TUI Migration (Weeks 9-10)
**Objective:** Update terminal UI for new architecture

**Tasks:**
- Update app.go state management
- Refactor session display logic
- Modify filtering/navigation
- Update overlay components
- Test all user workflows

**Deliverables:**
- TUI fully migrated
- All shortcuts working
- Performance maintained

#### Phase 6: Web UI Migration (Weeks 11-12)
**Objective:** Update web interface for new architecture

**Tasks:**
- Update TypeScript types
- Refactor React components
- Modify API client hooks
- Update state management
- Test all UI interactions

**Deliverables:**
- Web UI fully migrated
- TypeScript types aligned
- All features functional

#### Phase 7: Cutover & Cleanup (Week 13)
**Objective:** Complete migration and remove legacy code

**Tasks:**
- Switch to v2 API as primary
- Remove dual-write code
- Drop deprecated columns
- Update all documentation
- Final performance validation

**Deliverables:**
- Legacy code removed
- Documentation updated
- System fully migrated

### File Impact Classification

#### Critical Files (Breaking Changes Required)
| File | Lines | Impact | Priority |
|------|-------|--------|----------|
| `session/instance.go` | 2,209 | Complete restructure | P0 |
| `session/sqlite_repository.go` | 1,500 | Query rewrite | P0 |
| `app/app.go` | 3,000 | State management | P0 |
| `proto/session/v1/*.proto` | 500 | New v2 protocol | P0 |

#### High Impact Files (Significant Refactoring)
| File | Lines | Impact | Priority |
|------|-------|--------|----------|
| `session/storage.go` | 800 | Interface changes | P1 |
| `server/services/session_service.go` | 600 | RPC updates | P1 |
| `ui/list.go` | 1,200 | Display logic | P1 |
| `web-app/src/lib/hooks/*.ts` | 400 | API client | P1 |

#### Medium Impact Files (Moderate Changes)
| File | Lines | Impact | Priority |
|------|-------|--------|----------|
| `app/services/*.go` | 2,000 | Facade updates | P2 |
| `ui/overlay/*.go` | 1,500 | Creation flows | P2 |
| `web-app/src/components/sessions/*.tsx` | 800 | UI components | P2 |

### Risk Analysis & Mitigation

#### Technical Risks

**Risk 1: Data Migration Failure**
- **Probability:** Medium
- **Impact:** Critical
- **Mitigation:**
  - Implement transactional migrations with rollback points
  - Create comprehensive backup before migration
  - Test with production data copy
  - Implement checksums for verification

**Risk 2: Performance Degradation**
- **Probability:** High
- **Impact:** High
- **Mitigation:**
  - Benchmark all queries before/after
  - Implement query result caching
  - Use selective JOINs with LoadOptions
  - Add database indexes strategically

**Risk 3: API Compatibility Break**
- **Probability:** Low
- **Impact:** Critical
- **Mitigation:**
  - Maintain v1 API for 6 months
  - Implement adapter pattern
  - Version all proto messages
  - Gradual client migration

#### Operational Risks

**Risk 4: Feature Regression**
- **Probability:** Medium
- **Impact:** Medium
- **Mitigation:**
  - Comprehensive test coverage (80%+)
  - Feature flags for gradual rollout
  - Parallel run of old/new code
  - Automated regression testing

**Risk 5: Rollback Complexity**
- **Probability:** Low
- **Impact:** High
- **Mitigation:**
  - Dual-write maintains compatibility
  - Schema v2 remains readable
  - Feature flags allow instant revert
  - Keep migration code for 3 months

### Architectural Benefits

#### Memory Optimization
- **Current:** Full Instance struct loaded for all operations (2KB per session)
- **New:** Load only required contexts (200B - 2KB based on need)
- **Benefit:** 50-90% memory reduction for list operations

#### Testability Improvements
- **Current:** Monolithic Instance difficult to mock
- **New:** Individual contexts can be mocked/stubbed
- **Benefit:** Faster, more focused unit tests

#### Extensibility Gains
- **Current:** Adding fields requires schema changes to core table
- **New:** New contexts added without touching existing code
- **Benefit:** Support for new deployment scenarios without disruption

#### Query Performance
- **Current:** All fields loaded even when unused
- **New:** Selective loading with LoadOptions
- **Benefit:** 3-5x faster list operations

### Implementation Stories (AIC Framework)

#### Story 1: Foundation Types
**Story:** As a developer, I need new type definitions so that I can start migrating code to the context pattern.

**Atomic Tasks:**
1. Define Session struct with contexts (3 files)
2. Create context interfaces (6 files)
3. Implement validation methods (6 files)
4. Add factory functions (3 files)
5. Create comprehensive tests (10 files)

**INVEST Validation:**
- Independent: Can be done without affecting existing code
- Negotiable: Context structure can be refined
- Valuable: Enables all subsequent work
- Estimable: 3-5 days
- Small: Limited to type definitions
- Testable: Unit tests for all methods

#### Story 2: Repository Dual-Mode
**Story:** As a system, I need dual-read/write capability so that migration can happen safely.

**Atomic Tasks:**
1. Create v3 schema migration (2 files)
2. Implement dual-write logic (3 files)
3. Add LoadOptions pattern (5 files)
4. Create query builders (4 files)
5. Benchmark performance (2 files)

**Success Criteria:**
- Both schemas remain in sync
- No performance degradation
- All tests pass with both modes

#### Story 3: Context-Aware Services
**Story:** As a service layer, I need context-aware methods so that I can handle optional data properly.

**Atomic Tasks:**
1. Refactor getter methods (20 methods)
2. Add context validation (20 methods)
3. Implement lazy loading (10 methods)
4. Update business logic (30 methods)
5. Create service tests (15 files)

**Success Criteria:**
- All methods handle nil contexts gracefully
- No panics from missing contexts
- Performance maintained or improved

### Validation Strategy

#### Unit Testing (80% Coverage Target)
- Test each context independently
- Validate nil handling throughout
- Ensure backward compatibility
- Test migration logic thoroughly

#### Integration Testing
- End-to-end workflows with new types
- API v1/v2 compatibility tests
- Database migration scenarios
- Performance benchmarks

#### User Acceptance Testing
- Create test environment with migrated data
- Run through all user workflows
- Validate UI functionality
- Test rollback procedures

### Post-Migration Optimization

Once migration is complete, additional optimizations become possible:

1. **Materialized Views** for common query patterns
2. **Read Replicas** for list operations
3. **Context Caching** in application layer
4. **Lazy Loading** for expensive contexts
5. **GraphQL API** for flexible data fetching

### Conclusion

This architectural refactoring represents a significant investment (13 weeks) but delivers substantial long-term benefits:
- **50% memory reduction** for list operations
- **3-5x query performance** improvement
- **Unlimited extensibility** for new contexts
- **Improved testability** and maintainability
- **Support for diverse deployment scenarios**

The phased approach with dual-mode operation ensures zero downtime and safe rollback capability throughout the migration process.

## Appendix A: Context Loading Examples

### LoadOptions Usage

```go
// Load core only (ephemeral container)
session, err := repo.Get(ctx, "my-session", LoadOptions{
    LoadCore: true,
})

// Load for terminal attachment (local development)
session, err := repo.Get(ctx, "my-session", LoadOptions{
    LoadCore:       true,
    LoadGit:        true,
    LoadFilesystem: true,
    LoadTerminal:   true,
})

// Load for web UI display
sessions, err := repo.List(ctx, LoadOptions{
    LoadCore:         true,
    LoadUIPreferences: true,
    LoadActivity:     true,
})

// Load for API access (cloud session)
session, err := repo.Get(ctx, "api-session", LoadOptions{
    LoadCore:  true,
    LoadCloud: true,
})
```

### Repository Interface

```go
type LoadOptions struct {
    LoadCore         bool  // Always true (for clarity)
    LoadGit          bool
    LoadFilesystem   bool
    LoadTerminal     bool
    LoadUIPreferences bool
    LoadActivity     bool
    LoadCloud        bool
    LoadTags         bool  // From session_tags join
    LoadWorktree     bool  // From worktrees join
}

type Repository interface {
    Create(ctx context.Context, session *Session, contexts ...Context) error
    Get(ctx context.Context, title string, opts LoadOptions) (*Session, error)
    List(ctx context.Context, filter Filter, opts LoadOptions) ([]*Session, error)
    UpdateCore(ctx context.Context, title string, updates CoreUpdates) error
    UpdateContext(ctx context.Context, title string, context Context) error
    Delete(ctx context.Context, title string) error
}
```

## Appendix B: Migration SQL

### Create Context Tables

```sql
-- Phase 1: Create new context tables
BEGIN TRANSACTION;

-- Git context for version control
CREATE TABLE IF NOT EXISTS git_context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    branch TEXT NOT NULL,
    base_commit_sha TEXT NOT NULL DEFAULT '',
    worktree_id INTEGER,
    pr_number INTEGER,
    pr_url TEXT,
    owner TEXT,
    repo TEXT,
    source_ref TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (worktree_id) REFERENCES worktrees(id) ON DELETE SET NULL
);

-- Filesystem context for local file access
CREATE TABLE IF NOT EXISTS filesystem_context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    project_path TEXT NOT NULL,
    working_dir TEXT NOT NULL,
    is_worktree INTEGER NOT NULL DEFAULT 0,
    main_repo_path TEXT,
    cloned_repo_path TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Terminal context for TTY sessions
CREATE TABLE IF NOT EXISTS terminal_context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    height INTEGER NOT NULL DEFAULT 24,
    width INTEGER NOT NULL DEFAULT 80,
    tmux_session_name TEXT,
    tmux_prefix TEXT,
    tmux_socket TEXT,
    terminal_type TEXT NOT NULL DEFAULT 'tmux',
    last_scrollback_id INTEGER,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- UI preferences for client display
CREATE TABLE IF NOT EXISTS ui_preferences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    category TEXT,
    is_expanded INTEGER NOT NULL DEFAULT 1,
    grouping_strategy TEXT,
    sort_order TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Activity tracking for notifications
CREATE TABLE IF NOT EXISTS activity_tracking (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    last_terminal_update DATETIME,
    last_meaningful_output DATETIME,
    last_output_signature TEXT,
    last_added_to_queue DATETIME,
    last_viewed DATETIME,
    last_acknowledged DATETIME,
    total_output_lines INTEGER DEFAULT 0,
    session_duration_seconds INTEGER DEFAULT 0,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Cloud context for API sessions
CREATE TABLE IF NOT EXISTS cloud_context (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL UNIQUE,
    provider TEXT NOT NULL,
    region TEXT,
    instance_id TEXT,
    api_endpoint TEXT,
    api_key_ref TEXT,
    cloud_session_id TEXT,
    conversation_id TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Create indexes for foreign keys and common queries
CREATE INDEX IF NOT EXISTS idx_git_context_session_id ON git_context(session_id);
CREATE INDEX IF NOT EXISTS idx_filesystem_context_session_id ON filesystem_context(session_id);
CREATE INDEX IF NOT EXISTS idx_terminal_context_session_id ON terminal_context(session_id);
CREATE INDEX IF NOT EXISTS idx_ui_preferences_session_id ON ui_preferences(session_id);
CREATE INDEX IF NOT EXISTS idx_ui_preferences_category ON ui_preferences(category);
CREATE INDEX IF NOT EXISTS idx_activity_tracking_session_id ON activity_tracking(session_id);
CREATE INDEX IF NOT EXISTS idx_activity_tracking_meaningful ON activity_tracking(last_meaningful_output);
CREATE INDEX IF NOT EXISTS idx_cloud_context_session_id ON cloud_context(session_id);

COMMIT;
```

### Migrate Existing Data

```sql
-- Phase 2: Migrate data to context tables
BEGIN TRANSACTION;

-- Migrate git context
INSERT INTO git_context (session_id, branch, pr_number, pr_url, owner, repo, source_ref)
SELECT 
    id,
    COALESCE(branch, ''),
    github_pr_number,
    github_pr_url,
    COALESCE(github_owner, ''),
    COALESCE(github_repo, ''),
    github_source_ref
FROM sessions
WHERE branch IS NOT NULL 
   OR github_pr_number IS NOT NULL 
   OR github_owner IS NOT NULL;

-- Migrate filesystem context
INSERT INTO filesystem_context (session_id, project_path, working_dir, is_worktree, main_repo_path, cloned_repo_path)
SELECT 
    id,
    path,
    COALESCE(working_dir, path),  -- Use path as working_dir if not specified
    COALESCE(is_worktree, 0),
    main_repo_path,
    cloned_repo_path
FROM sessions;

-- Migrate terminal context (for tmux sessions)
INSERT INTO terminal_context (session_id, height, width, tmux_prefix, tmux_session_name)
SELECT 
    id,
    COALESCE(height, 24),
    COALESCE(width, 80),
    tmux_prefix,
    'claudesquad_' || title  -- Reconstruct tmux session name
FROM sessions;

-- Migrate UI preferences
INSERT INTO ui_preferences (session_id, category, is_expanded)
SELECT 
    id,
    category,
    COALESCE(is_expanded, 1)
FROM sessions
WHERE category IS NOT NULL;

-- Migrate activity tracking
INSERT INTO activity_tracking (
    session_id,
    last_terminal_update,
    last_meaningful_output,
    last_output_signature,
    last_added_to_queue,
    last_viewed,
    last_acknowledged
)
SELECT 
    id,
    last_terminal_update,
    last_meaningful_output,
    last_output_signature,
    last_added_to_queue,
    last_viewed,
    last_acknowledged
FROM sessions
WHERE last_terminal_update IS NOT NULL
   OR last_meaningful_output IS NOT NULL
   OR last_viewed IS NOT NULL;

COMMIT;
```

## Appendix C: Implementation Timeline

### Week 1: Foundation
- Day 1-2: Create context table schemas and migrations
- Day 3-4: Implement LoadOptions and ContextLoader
- Day 5: Set up dual-write mode and feature flags

### Week 2: Migration
- Day 1-2: Write and test migration scripts
- Day 3: Implement verification and rollback
- Day 4-5: Run migration on test data

### Week 3: Integration
- Day 1-2: Refactor repository with context pattern
- Day 3-4: Update queries with LoadOptions
- Day 5: Performance testing and optimization

### Week 4: Deployment
- Day 1: Deploy to staging environment
- Day 2-3: Monitor and validate
- Day 4: Production deployment
- Day 5: Remove deprecated code

This domain-driven approach provides the flexibility to support diverse deployment scenarios while maintaining a clean, normalized schema that eliminates null handling issues and improves maintainability.
