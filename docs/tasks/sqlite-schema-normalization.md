# SQLite Schema Normalization with Domain-Driven Design

## Executive Summary

This document presents a revised domain-driven approach to normalizing the SQLite schema for claude-squad. The design separates the core Session entity (containing only universally required fields) from optional contexts that are attached based on deployment scenarios. This enables support for diverse environments including local git-based development, ephemeral containers, cloud instances, and headless API-driven sessions.

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
