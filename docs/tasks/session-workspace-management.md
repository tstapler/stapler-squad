# Session Workspace Management Feature Plan

## Executive Summary

This feature plan outlines the implementation of comprehensive workspace management capabilities for claude-squad sessions, enabling users to modify working directories, switch branches, and manage worktrees after session creation. Currently, these properties are immutable once a session is created, limiting flexibility and forcing users to create new sessions for different branches or directories.

## Problem Statement

### Current Limitations
1. **Immutable Working Directory**: Once a session is created with a specific `WorkingDir`, it cannot be changed
2. **Fixed Branch Assignment**: Sessions are locked to their initial branch, preventing branch switching
3. **Static Worktree Binding**: No ability to move sessions between worktrees or create new worktrees
4. **No Migration Path**: Users must destroy and recreate sessions to change these properties

### User Impact
- Developers lose context and history when switching branches
- Multiple sessions needed for the same project across different branches
- Inefficient resource usage with duplicate sessions
- Poor user experience for exploratory development workflows

## Requirements Analysis (ATOMIC-INVEST-CONTEXT)

### Functional Requirements (IEEE 830 Format)

#### FR-1: Working Directory Management
- **FR-1.1**: System SHALL allow users to change the working directory of an existing session
- **FR-1.2**: System SHALL validate new directory paths exist and are accessible
- **FR-1.3**: System SHALL preserve terminal history when changing directories
- **FR-1.4**: System SHALL update tmux pane working directory via `send-keys "cd"`
- **FR-1.5**: System SHALL persist working directory changes to storage

#### FR-2: Branch Switching
- **FR-2.1**: System SHALL enable branch switching within existing sessions
- **FR-2.2**: System SHALL check for uncommitted changes before switching
- **FR-2.3**: System SHALL offer to stash/commit changes if workspace is dirty
- **FR-2.4**: System SHALL update worktree to new branch using git commands
- **FR-2.5**: System SHALL handle branch creation if target doesn't exist

#### FR-3: Worktree Management
- **FR-3.1**: System SHALL support creating new worktrees for sessions
- **FR-3.2**: System SHALL allow moving sessions to different existing worktrees
- **FR-3.3**: System SHALL cleanup orphaned worktrees when switching
- **FR-3.4**: System SHALL validate worktree compatibility with repository
- **FR-3.5**: System SHALL maintain branch-worktree associations

#### FR-4: User Interface Integration
- **FR-4.1**: TUI SHALL provide keyboard shortcuts for workspace operations
- **FR-4.2**: Web UI SHALL expose workspace controls in session cards
- **FR-4.3**: System SHALL display current workspace state prominently
- **FR-4.4**: System SHALL provide confirmation dialogs for destructive operations
- **FR-4.5**: System SHALL show progress indicators for long operations

### Non-Functional Requirements (ISO/IEC 25010)

#### Performance
- **NFR-P1**: Directory changes SHALL complete within 100ms
- **NFR-P2**: Branch switches SHALL complete within 5 seconds for typical repos
- **NFR-P3**: Worktree creation SHALL not block UI responsiveness
- **NFR-P4**: Status updates SHALL propagate within 500ms

#### Reliability
- **NFR-R1**: System SHALL maintain data consistency during concurrent operations
- **NFR-R2**: System SHALL rollback failed operations to previous state
- **NFR-R3**: System SHALL handle git operation failures gracefully
- **NFR-R4**: System SHALL preserve session state across crashes

#### Security
- **NFR-S1**: System SHALL validate all paths against directory traversal attacks
- **NFR-S2**: System SHALL respect file system permissions
- **NFR-S3**: System SHALL sanitize branch names for shell execution
- **NFR-S4**: System SHALL prevent execution of arbitrary commands

#### Usability
- **NFR-U1**: Operations SHALL require maximum 3 user interactions
- **NFR-U2**: System SHALL provide clear error messages with recovery steps
- **NFR-U3**: System SHALL maintain keyboard navigation consistency
- **NFR-U4**: System SHALL support undo for recent operations

## Architecture & Design (Domain-Driven Design)

### Bounded Contexts

#### Session Management Context
- **Aggregate Root**: `Instance` (session/instance.go)
- **Entities**: `GitWorktree`, `TmuxSession`
- **Value Objects**: `WorkspaceConfig`, `BranchState`
- **Domain Events**: `WorkspaceChanged`, `BranchSwitched`, `WorktreeCreated`

#### Version Control Context
- **Aggregate Root**: `GitWorktree` (session/git/worktree.go)
- **Entities**: `Branch`, `Worktree`
- **Value Objects**: `CommitSHA`, `BranchName`, `WorktreePath`
- **Domain Services**: `WorktreeManager`, `BranchSwitcher`

### Architectural Patterns

#### Command Pattern for Workspace Operations
```go
type WorkspaceCommand interface {
    Execute() error
    Undo() error
    Validate() error
}

type ChangeDirectoryCommand struct {
    instance *Instance
    oldDir   string
    newDir   string
}

type SwitchBranchCommand struct {
    instance  *Instance
    oldBranch string
    newBranch string
    stashed   bool
}
```

#### Strategy Pattern for Git Operations
```go
type GitStrategy interface {
    SwitchBranch(from, to string) error
    CreateWorktree(branch string) (string, error)
    ValidateWorkspace() error
}

type StandardGitStrategy struct{}
type WorktreeGitStrategy struct{}
```

### Component Design

#### 1. Workspace Manager Component
**Location**: `session/workspace/manager.go`

**Responsibilities**:
- Orchestrate workspace operations
- Validate operation preconditions
- Coordinate between git and tmux
- Emit domain events
- Handle rollback on failure

**Key Methods**:
```go
func (m *WorkspaceManager) ChangeDirectory(instance *Instance, newDir string) error
func (m *WorkspaceManager) SwitchBranch(instance *Instance, branch string, opts BranchSwitchOptions) error
func (m *WorkspaceManager) CreateWorktree(instance *Instance, branch string) (*GitWorktree, error)
func (m *WorkspaceManager) MoveToWorktree(instance *Instance, worktreePath string) error
```

#### 2. Enhanced Instance Methods
**Location**: `session/instance.go`

**New Methods**:
```go
func (i *Instance) ChangeWorkingDirectory(path string) error
func (i *Instance) SwitchBranch(branch string, opts BranchSwitchOptions) error
func (i *Instance) CreateNewWorktree(branch string) error
func (i *Instance) MoveToWorktree(path string) error
func (i *Instance) GetWorkspaceState() WorkspaceState
```

#### 3. UI Overlays
**Location**: `ui/overlay/workspaceEditor.go`

**Components**:
- `WorkspaceEditorOverlay` - Main editor interface
- `DirectoryChanger` - Directory selection widget
- `BranchSwitcher` - Branch selection/creation
- `WorktreeManager` - Worktree operations UI

### API Design

#### gRPC Service Extensions
```protobuf
// Add to session.proto
message ChangeWorkingDirectoryRequest {
  string session_id = 1;
  string new_directory = 2;
}

message SwitchBranchRequest {
  string session_id = 1;
  string branch_name = 2;
  bool auto_stash = 3;
  bool create_if_missing = 4;
}

message CreateWorktreeRequest {
  string session_id = 1;
  string branch_name = 2;
  string base_commit = 3;
}

service SessionService {
  // ... existing methods ...
  rpc ChangeWorkingDirectory(ChangeWorkingDirectoryRequest) returns (Session);
  rpc SwitchBranch(SwitchBranchRequest) returns (Session);
  rpc CreateWorktree(CreateWorktreeRequest) returns (Session);
}
```

## Known Issues & Bug Prevention

### 🐛 Concurrency Risk: Race Condition in Workspace Changes [SEVERITY: High]

**Description**: Concurrent workspace modifications may corrupt session state when multiple operations execute simultaneously.

**Mitigation**:
- Implement session-level mutex for workspace operations
- Use atomic operations for state updates
- Add operation queue with serial execution
- Test with concurrent operation stress tests

**Prevention Strategy**:
```go
type Instance struct {
    // ... existing fields ...
    workspaceMutex sync.Mutex
    operationQueue chan WorkspaceCommand
}
```

### 🐛 Data Integrity Risk: Orphaned Worktrees [SEVERITY: Medium]

**Description**: Worktree switching may leave orphaned git worktrees consuming disk space.

**Mitigation**:
- Track worktree lifecycle in database
- Implement garbage collection for unused worktrees
- Add cleanup on session destruction
- Periodic background cleanup task

**Files Affected**:
- session/git/worktree_ops.go
- session/instance.go
- server/background_tasks.go

### 🐛 Integration Risk: Tmux State Desynchronization [SEVERITY: High]

**Description**: Directory changes via git may not reflect in tmux pane, causing confusion.

**Mitigation**:
- Always use tmux send-keys for directory changes
- Verify directory after change with pwd
- Add retry logic for tmux commands
- Monitor tmux pane working directory

**Prevention Code**:
```go
func (i *Instance) syncTmuxDirectory(path string) error {
    // Send cd command
    if _, err := i.tmuxSession.SendKeys(fmt.Sprintf("cd %q\n", path)); err != nil {
        return err
    }
    
    // Verify change
    time.Sleep(100 * time.Millisecond)
    actualDir := i.tmuxSession.GetPaneWorkingDirectory()
    if actualDir != path {
        return fmt.Errorf("directory sync failed: expected %s, got %s", path, actualDir)
    }
    return nil
}
```

### 🐛 Security Risk: Path Traversal Vulnerability [SEVERITY: Critical]

**Description**: User-supplied paths could escape repository boundaries.

**Mitigation**:
- Canonicalize all paths before use
- Validate paths are within repository
- Reject paths with ".." components
- Use filepath.Clean and filepath.Abs

**Validation Function**:
```go
func validatePathSecurity(repoRoot, targetPath string) error {
    cleanPath := filepath.Clean(targetPath)
    absPath, err := filepath.Abs(cleanPath)
    if err != nil {
        return err
    }
    
    absRepo, _ := filepath.Abs(repoRoot)
    if !strings.HasPrefix(absPath, absRepo) {
        return fmt.Errorf("path outside repository: %s", targetPath)
    }
    
    return nil
}
```

### 🐛 Performance Risk: Large Repository Operations [SEVERITY: Medium]

**Description**: Branch switching in large repos may cause UI freezes.

**Mitigation**:
- Execute git operations in background goroutines
- Show progress indicators during operations
- Implement operation cancellation
- Cache branch list for quick access

### 🐛 Edge Case: Uncommitted Changes Handling [SEVERITY: High]

**Description**: Branch switching with uncommitted changes may lose work.

**Mitigation**:
- Always check git status before switching
- Offer stash/commit/abort options
- Create automatic backup commits
- Warn about potential data loss

## Implementation Roadmap

### Phase 1: Core Infrastructure (Week 1-2)

**Deliverables**:
- [ ] WorkspaceManager component implementation
- [ ] Instance method extensions for workspace operations
- [ ] Mutex-based concurrency control
- [ ] Basic validation and error handling

**Key Files**:
- session/workspace/manager.go (new)
- session/workspace/commands.go (new)
- session/instance.go (modify)
- session/git/worktree_ops.go (extend)

### Phase 2: Directory Management (Week 2-3)

**Deliverables**:
- [ ] ChangeWorkingDirectory implementation
- [ ] Tmux pane synchronization
- [ ] Path validation and security
- [ ] TUI keyboard shortcut (Ctrl+D)

**Key Files**:
- session/instance_directory.go (new)
- app/app.go (add handler)
- keys/keys.go (add KeyChangeDirectory)

### Phase 3: Branch Switching (Week 3-4)

**Deliverables**:
- [ ] SwitchBranch with stash handling
- [ ] Branch creation flow
- [ ] Dirty workspace detection
- [ ] UI overlay for branch selection

**Key Files**:
- session/instance_branch.go (new)
- ui/overlay/branchSwitcher.go (new)
- session/git/worktree_branch.go (extend)

### Phase 4: Worktree Management (Week 4-5)

**Deliverables**:
- [ ] CreateWorktree functionality
- [ ] MoveToWorktree implementation
- [ ] Orphaned worktree cleanup
- [ ] Worktree browser UI

**Key Files**:
- session/instance_worktree.go (new)
- ui/overlay/worktreeManager.go (new)
- session/git/worktree_lifecycle.go (new)

### Phase 5: Web UI Integration (Week 5-6)

**Deliverables**:
- [ ] gRPC service methods
- [ ] Web UI workspace controls
- [ ] Real-time status updates
- [ ] Progress indicators

**Key Files**:
- proto/session/v1/workspace.proto (new)
- server/services/workspace_service.go (new)
- web-app/src/components/WorkspaceControls.tsx (new)

### Phase 6: Testing & Refinement (Week 6-7)

**Deliverables**:
- [ ] Unit tests (80% coverage)
- [ ] Integration tests for git operations
- [ ] Concurrency stress tests
- [ ] Performance benchmarks
- [ ] User documentation

**Test Files**:
- session/workspace/manager_test.go
- session/instance_workspace_test.go
- app/workspace_integration_test.go

## Testing Strategy

### Unit Tests
```go
func TestChangeDirectory_ValidPath(t *testing.T)
func TestChangeDirectory_InvalidPath(t *testing.T)
func TestSwitchBranch_CleanWorkspace(t *testing.T)
func TestSwitchBranch_DirtyWorkspace(t *testing.T)
func TestCreateWorktree_NewBranch(t *testing.T)
func TestWorkspaceMutex_ConcurrentOperations(t *testing.T)
```

### Integration Tests
```go
func TestWorkspaceFlow_Complete(t *testing.T)
func TestGitWorktreeIntegration(t *testing.T)
func TestTmuxSynchronization(t *testing.T)
```

### Performance Benchmarks
```go
func BenchmarkDirectoryChange(b *testing.B)
func BenchmarkBranchSwitch_SmallRepo(b *testing.B)
func BenchmarkBranchSwitch_LargeRepo(b *testing.B)
func BenchmarkWorktreeCreation(b *testing.B)
```

## Success Metrics

### Quantitative Metrics
- Directory change latency < 100ms (p99)
- Branch switch success rate > 95%
- Zero data loss incidents
- Worktree cleanup efficiency > 90%

### Qualitative Metrics
- User satisfaction score > 4.0/5.0
- Reduced session recreation by 60%
- Improved developer workflow efficiency
- Positive user feedback on flexibility

## Risk Assessment

### Technical Risks
1. **Git Operation Failures** (High Impact, Medium Probability)
   - Mitigation: Comprehensive error handling, rollback mechanisms
   
2. **Performance Degradation** (Medium Impact, Low Probability)
   - Mitigation: Background operations, progress indicators

3. **Data Loss** (Critical Impact, Low Probability)
   - Mitigation: Auto-stash, backup commits, confirmation dialogs

### Operational Risks
1. **User Confusion** (Medium Impact, Medium Probability)
   - Mitigation: Clear UI, comprehensive documentation, tooltips

2. **Backward Compatibility** (Low Impact, Low Probability)
   - Mitigation: Feature flags, gradual rollout

## Documentation Requirements

### User Documentation
- Feature overview and benefits
- Step-by-step usage guides
- Keyboard shortcuts reference
- Troubleshooting guide
- Video tutorials

### Developer Documentation
- Architecture decision records (ADRs)
- API documentation
- Code examples
- Testing guidelines
- Contribution guide

## Alternative Approaches Considered

### Alternative 1: New Session with State Transfer
- Create new session and transfer state
- Rejected: Complex state management, poor UX

### Alternative 2: Git Worktree per Branch
- Automatic worktree creation for each branch
- Rejected: Disk space concerns, management overhead

### Alternative 3: Detach/Reattach Pattern
- Detach from tmux, modify, reattach
- Rejected: Risk of session loss, complex recovery

## Dependencies

### External Dependencies
- git >= 2.20 (worktree support)
- tmux >= 3.0 (pane working directory)
- go-git/v5 library
- BubbleTea TUI framework

### Internal Dependencies
- Session storage layer
- UI coordinator system
- gRPC service infrastructure
- Configuration management

## Appendix A: User Stories

```gherkin
Feature: Change Working Directory
  As a developer
  I want to change my session's working directory
  So that I can navigate my project without creating new sessions

  Scenario: Change to subdirectory
    Given I have an active session in "/project"
    When I press Ctrl+D and select "/project/src"
    Then the session working directory should be "/project/src"
    And the tmux pane should show the new directory

Feature: Switch Git Branch
  As a developer
  I want to switch branches in my current session
  So that I can work on different features without losing context

  Scenario: Switch with clean workspace
    Given I have a session on branch "main" with no changes
    When I press Ctrl+B and select "feature-branch"
    Then the session should switch to "feature-branch"
    And the worktree should reflect the new branch

  Scenario: Switch with uncommitted changes
    Given I have uncommitted changes on "main"
    When I try to switch to "feature-branch"
    Then I should see options to stash, commit, or abort
    And my changes should be preserved based on my choice
```

## Appendix B: Configuration Schema

```json
{
  "workspace_management": {
    "enabled": true,
    "auto_stash": false,
    "cleanup_orphaned_worktrees": true,
    "worktree_gc_interval": "24h",
    "max_worktrees_per_repo": 10,
    "confirm_destructive_operations": true,
    "default_branch_prefix": "claudesquad/",
    "preserve_terminal_history": true
  }
}
```

## Appendix C: Monitoring & Observability

### Key Metrics to Track
- workspace_operation_duration_seconds{operation, status}
- workspace_operation_total{operation, status}
- worktree_count{repository}
- orphaned_worktree_cleanup_total
- git_operation_errors_total{operation, error_type}

### OpenTelemetry Spans
- `workspace.change_directory`
- `workspace.switch_branch`
- `workspace.create_worktree`
- `git.stash_changes`
- `tmux.sync_directory`

### Logging Strategy
```go
log.InfoLog.Printf("[Workspace] Changing directory for session %s: %s -> %s", 
    instance.Title, instance.WorkingDir, newDir)
log.ErrorLog.Printf("[Workspace] Failed to switch branch for session %s: %v",
    instance.Title, err)
```

---

*Document Version: 1.0.0*  
*Author: Claude Squad Architecture Team*  
*Date: December 2024*  
*Status: DRAFT - Ready for Review*
