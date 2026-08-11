# Session Rename and Restart Feature Plan

## Epic Overview

### User Value Statement
Enable users to rename existing sessions for better organization and restart sessions to recover from crashes or apply configuration changes, improving session management flexibility and system reliability.

### Success Metrics
- **Adoption Rate**: 60% of users rename at least one session within first week
- **Restart Success Rate**: >95% of restart operations complete without data loss
- **User Satisfaction**: Reduction in session management related support tickets by 40%
- **Performance**: Rename operation completes in <100ms, restart in <3s

### Scope
- **In Scope**: Session renaming, session restart, UI integration (TUI/Web), state preservation
- **Out of Scope**: Bulk operations, automatic restart policies, session templates

## Architecture Decisions

### ADR-001: Session Rename Implementation Strategy

**Status**: Proposed

**Context**: Sessions use their title as the primary identifier throughout the system (tmux sessions, git worktrees, storage keys). Renaming requires careful coordination.

**Decision**: Implement rename as a metadata-only operation, keeping tmux session names and worktree paths unchanged.

**Consequences**:
- ✅ Simple implementation, no resource migration needed
- ✅ Fast operation (<100ms)
- ✅ No risk of data loss
- ❌ Mismatch between displayed name and internal identifiers
- ❌ Potential confusion when debugging

**Alternatives Considered**:
1. **Full Migration**: Rename tmux session and move worktree
   - ❌ Complex, risky, slow
   - ❌ Potential for data loss
2. **Lazy Migration**: Update on next restart
   - ❌ Inconsistent state
   - ❌ Complex state management

### ADR-002: Session Restart Architecture

**Status**: Proposed

**Context**: Sessions may need restart due to crashes, configuration changes, or user request. Must preserve work and context.

**Decision**: Implement restart as kill-and-recreate with state preservation, reusing existing worktree.

**Consequences**:
- ✅ Clean slate for tmux session
- ✅ Preserves git work (worktree intact)
- ✅ Applies configuration changes
- ❌ Brief downtime (1-3 seconds)
- ❌ Loss of terminal scrollback

**Implementation Details**:
- Preserve: worktree, branch, uncommitted changes, Claude session ID
- Reset: tmux session, terminal state, process tree
- Optional: preserve terminal output to file before restart

### ADR-003: Concurrency and State Management

**Status**: Proposed

**Context**: Rename and restart operations affect shared state and must handle concurrent access safely.

**Decision**: Use optimistic locking with version fields and atomic operations.

**Consequences**:
- ✅ No blocking operations
- ✅ Safe concurrent access
- ✅ Consistent state
- ❌ Potential for conflict errors (retry needed)

## User Stories

### Story 1: Rename Session from TUI
**As a** developer using the TUI  
**I want to** rename a session by pressing 'r'  
**So that** I can better organize my work

**Acceptance Criteria**:
- [ ] 'r' key opens rename overlay with current name pre-filled
- [ ] Validates new name (no duplicates, valid characters)
- [ ] Updates immediately upon confirmation
- [ ] Shows success/error message
- [ ] Preserves selection after rename
- [ ] Works for both active and paused sessions

### Story 2: Rename Session from Web UI
**As a** developer using the web interface  
**I want to** rename sessions through the UI  
**So that** I can organize without switching to terminal

**Acceptance Criteria**:
- [ ] Edit button/icon on session card
- [ ] Inline editing with validation
- [ ] Real-time duplicate checking
- [ ] Optimistic UI updates
- [ ] Error recovery on failure

### Story 3: Restart Active Session
**As a** developer with a frozen session  
**I want to** restart the session  
**So that** I can continue working without losing progress

**Acceptance Criteria**:
- [ ] Shift+R restarts selected session
- [ ] Confirmation dialog for active sessions
- [ ] Preserves git worktree and changes
- [ ] Restores to same directory
- [ ] Maintains Claude conversation (--resume flag)
- [ ] Shows progress indicator during restart

### Story 4: Batch Restart After Crash
**As a** developer after system crash  
**I want to** restart all previously running sessions  
**So that** I can quickly resume work

**Acceptance Criteria**:
- [ ] Detect orphaned sessions on startup
- [ ] Option to restart all or select specific sessions
- [ ] Parallel restart for performance
- [ ] Progress indicator for batch operations
- [ ] Error handling for partial failures

## Implementation Tasks

### Task Group 1: Core Rename Functionality

#### Task 1.1: Add Rename Method to Instance [ATOMIC]
**Files**: 
- `/session/instance.go` - Add Rename() method
- `/session/types.go` - Add validation constants
- `/session/instance_test.go` - Unit tests

**Implementation**:
```go
// instance.go
func (i *Instance) Rename(newTitle string) error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    
    // Validation
    if newTitle == "" || len(newTitle) > 32 {
        return fmt.Errorf("invalid title length")
    }
    if !isValidTitle(newTitle) {
        return fmt.Errorf("invalid characters in title")
    }
    
    oldTitle := i.Title
    i.Title = newTitle
    i.UpdatedAt = time.Now()
    
    // Emit event for UI updates
    if i.eventBus != nil {
        i.eventBus.Publish(SessionRenamedEvent{
            OldTitle: oldTitle,
            NewTitle: newTitle,
        })
    }
    
    return nil
}
```

**Known Issues**:
- 🐛 **Race Condition**: Concurrent renames might override each other
  - **Mitigation**: Use version field for optimistic locking
- 🐛 **Tmux Mismatch**: Display name differs from tmux session name
  - **Mitigation**: Add helper to get tmux name vs display name

#### Task 1.2: Update Storage Layer [ATOMIC]
**Files**:
- `/session/storage.go` - Handle rename in save/load
- `/server/services/session_service.go` - RPC endpoint updates
- `/proto/session/v1/session.proto` - Ensure title field is mutable

**Implementation**:
```go
// storage.go - Add duplicate checking
func (s *Storage) RenameInstance(oldTitle, newTitle string) error {
    instances, err := s.LoadInstances()
    if err != nil {
        return err
    }
    
    // Check for duplicates
    for _, inst := range instances {
        if inst.Title == newTitle {
            return ErrDuplicateTitle
        }
    }
    
    // Find and rename
    for _, inst := range instances {
        if inst.Title == oldTitle {
            inst.Rename(newTitle)
            break
        }
    }
    
    return s.SaveInstances(instances)
}
```

### Task Group 2: Core Restart Functionality

#### Task 2.1: Implement Restart Method [ATOMIC]
**Files**:
- `/session/instance.go` - Add Restart() method
- `/session/tmux/tmux.go` - Add recreation logic
- `/session/instance_test.go` - Tests

**Implementation**:
```go
// instance.go
func (i *Instance) Restart() error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    
    if !i.started {
        return fmt.Errorf("cannot restart unstarted session")
    }
    
    // Save state for restoration
    state := i.captureRestartState()
    
    // Kill tmux session only (preserve worktree)
    if err := i.KillSession(); err != nil {
        log.Warn("Failed to kill session: %v", err)
    }
    
    // Recreate tmux session
    i.tmuxSession = tmux.NewTmuxSession(i.Title, i.Program)
    
    // Restore with saved state
    return i.restoreFromState(state)
}

type RestartState struct {
    WorkingDir string
    ClaudeSession *ClaudeSessionData
    Environment map[string]string
}
```

**Known Issues**:
- 🐛 **Terminal State Loss**: Scrollback history lost on restart
  - **Mitigation**: Option to save terminal output before restart
- 🐛 **Process Cleanup**: Child processes might survive restart
  - **Mitigation**: Use process groups for clean termination

#### Task 2.2: Add Claude Session Continuity [ATOMIC]
**Files**:
- `/session/instance.go` - Preserve Claude session ID
- `/session/claude_controller.go` - Handle resume flag
- `/session/session_restart_test.go` - Integration tests

**Implementation**:
```go
// instance.go - Modify Start() to handle restart with --resume
func (i *Instance) buildStartCommand() string {
    cmd := i.Program
    
    // Add --resume flag for Claude sessions on restart
    if i.claudeSession != nil && 
       strings.Contains(i.Program, "claude") &&
       i.claudeSession.SessionID != "" {
        cmd += fmt.Sprintf(" --resume %s", i.claudeSession.SessionID)
    }
    
    return cmd
}
```

### Task Group 3: TUI Integration

#### Task 3.1: Add Rename Key Binding [ATOMIC]
**Files**:
- `/cmd/commands/session.go` - Add rename handler
- `/cmd/registry.go` - Register 'r' key
- `/app/app.go` - Wire up handler

**Implementation**:
```go
// session.go
type SessionHandlers struct {
    // ... existing handlers
    OnRenameSession  func() (tea.Model, tea.Cmd)
    OnRestartSession func() (tea.Model, tea.Cmd)
}

func RenameSessionCommand(ctx *interfaces.CommandContext) error {
    if sessionHandlers.OnRenameSession != nil {
        model, cmd := sessionHandlers.OnRenameSession()
        ctx.Args["model"] = model
        ctx.Args["cmd"] = cmd
    }
    return nil
}
```

#### Task 3.2: Create Rename Overlay [ATOMIC]
**Files**:
- `/ui/overlay/rename_input.go` - New overlay component
- `/app/ui/coordinator.go` - Add to coordinator
- `/ui/overlay/rename_input_test.go` - Tests

**Implementation**:
```go
// rename_input.go
type RenameInputOverlay struct {
    textInput textinput.Model
    oldTitle  string
    validator func(string) error
}

func (r *RenameInputOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
    switch msg.Type {
    case tea.KeyEnter:
        if err := r.validator(r.textInput.Value()); err != nil {
            r.showError(err)
            return false
        }
        r.submitted = true
        return true // close overlay
    case tea.KeyEsc:
        return true // cancel
    }
    r.textInput, _ = r.textInput.Update(msg)
    return false
}
```

**Known Issues**:
- 🐛 **Focus Management**: Overlay might not capture focus properly
  - **Mitigation**: Ensure state transition disables list navigation
- 🐛 **Validation Lag**: Real-time duplicate checking might be slow
  - **Mitigation**: Debounce validation, show loading state

### Task Group 4: Web UI Integration

#### Task 4.1: Add Rename UI to SessionCard [ATOMIC]
**Files**:
- `/web-app/src/components/sessions/SessionCard.tsx` - Add rename button
- `/web-app/src/components/sessions/RenameDialog.tsx` - New component
- `/web-app/src/lib/hooks/useSessionMutation.ts` - Add rename mutation

**Implementation**:
```typescript
// SessionCard.tsx
const [showRename, setShowRename] = useState(false);

const handleRename = async (newTitle: string) => {
    try {
        await updateSession({
            id: session.id,
            title: newTitle
        });
        setShowRename(false);
    } catch (error) {
        showError(error.message);
    }
};

// Add button to card actions
<button onClick={() => setShowRename(true)}>
    <EditIcon /> Rename
</button>
```

#### Task 4.2: Add Restart Action [ATOMIC]
**Files**:
- `/proto/session/v1/session.proto` - Add RestartSession RPC
- `/server/services/session_service.go` - Implement RPC
- `/web-app/src/components/sessions/SessionCard.tsx` - Add restart button

**Implementation**:
```protobuf
// session.proto
rpc RestartSession(RestartSessionRequest) returns (RestartSessionResponse) {}

message RestartSessionRequest {
    string id = 1;
    bool preserve_output = 2;  // Save terminal output
}
```

### Task Group 5: Error Handling and Recovery

#### Task 5.1: Add Robust Error Recovery [ATOMIC]
**Files**:
- `/session/instance.go` - Add recovery mechanisms
- `/session/recovery.go` - New recovery utilities
- `/session/recovery_test.go` - Tests

**Implementation**:
```go
// recovery.go
type RecoveryManager struct {
    maxRetries int
    backoff    time.Duration
}

func (r *RecoveryManager) RestartWithRecovery(instance *Instance) error {
    for attempt := 0; attempt < r.maxRetries; attempt++ {
        err := instance.Restart()
        if err == nil {
            return nil
        }
        
        if !isRecoverable(err) {
            return err
        }
        
        time.Sleep(r.backoff * time.Duration(attempt+1))
    }
    return ErrMaxRetriesExceeded
}
```

## Known Issues and Mitigation

### Critical Issues

#### 🐛 Concurrent Modification [SEVERITY: High]
**Description**: Multiple users/windows modifying same session simultaneously.

**Mitigation**:
- Implement optimistic locking with version fields
- Add conflict resolution UI
- Use atomic operations in storage layer

**Prevention**:
```go
type Instance struct {
    Version int64 // Increment on every modification
    // ...
}

func (i *Instance) CheckAndUpdate(expectedVersion int64, update func()) error {
    if i.Version != expectedVersion {
        return ErrConcurrentModification
    }
    update()
    i.Version++
    return nil
}
```

#### 🐛 Orphaned Resources [SEVERITY: High]
**Description**: Restart failure leaving tmux sessions or worktrees orphaned.

**Mitigation**:
- Implement cleanup goroutine for orphan detection
- Add manual cleanup command
- Use finalizers for guaranteed cleanup

#### 🐛 State Corruption [SEVERITY: High]
**Description**: Partial rename/restart leaving inconsistent state.

**Mitigation**:
- Use database transactions or equivalent
- Implement rollback mechanism
- Add state validation on load

### Performance Issues

#### 🐛 Rename Broadcast Storm [SEVERITY: Medium]
**Description**: Rename events causing excessive UI updates in multi-window setup.

**Mitigation**:
- Debounce event publishing
- Use event aggregation
- Optimize UI update batching

## Testing Strategy

### Unit Tests
- `TestInstanceRename` - Basic rename functionality
- `TestRenameDuplicateValidation` - Duplicate detection
- `TestRestartStatePreservation` - State maintained across restart
- `TestConcurrentRename` - Race condition handling

### Integration Tests
- `TestRenameWithStorage` - Persistence across restart
- `TestRestartWithClaudeSession` - --resume flag integration
- `TestBatchRestart` - Multiple session restart
- `TestRenameRPCEndpoint` - Web API functionality

### E2E Tests
- TUI rename flow with validation
- Web UI inline editing
- Restart with active git changes
- Recovery from failed restart

## Rollout Plan

### Phase 1: Core Implementation (Week 1)
- Task Groups 1-2 (Core rename/restart)
- Basic testing
- Feature flag: `ENABLE_SESSION_RENAME_RESTART=false`

### Phase 2: TUI Integration (Week 2)
- Task Group 3
- Power user testing
- Documentation

### Phase 3: Web UI & Polish (Week 3)
- Task Groups 4-5
- Error handling improvements
- Performance optimization

### Phase 4: GA Release (Week 4)
- Remove feature flag
- Monitor metrics
- Gather feedback

## Success Validation

### Functional Validation
- [ ] Rename updates all UI components correctly
- [ ] Restart preserves all expected state
- [ ] No data loss during operations
- [ ] Concurrent operations handled safely

### Performance Validation
- [ ] Rename completes in <100ms (95th percentile)
- [ ] Restart completes in <3s (95th percentile)
- [ ] No UI freezing during operations
- [ ] Memory usage stable after multiple operations

### User Experience Validation
- [ ] Intuitive key bindings ('r' for rename, 'Shift+R' for restart)
- [ ] Clear error messages
- [ ] Smooth animations/transitions
- [ ] Consistent behavior across TUI/Web

## Dependencies

### External Dependencies
- BubbleTea v0.24+ (for overlay improvements)
- tmux 3.0+ (for session management)
- Go 1.21+ (for improved error handling)

### Internal Dependencies
- Command registry system (existing)
- Event bus for state propagation
- Storage layer with atomic operations
- RPC framework for web integration

## Open Questions

1. **Q**: Should rename affect tmux session name?
   - **A**: No, keep as display-only change for simplicity

2. **Q**: Should restart be available for paused sessions?
   - **A**: Yes, but behavior differs (just resume, no kill needed)

3. **Q**: How to handle rename during active editing?
   - **A**: Allow it - name is UI-only, doesn't affect running session

4. **Q**: Should we preserve terminal scrollback on restart?
   - **A**: Optional - add flag to save to file if requested

## Appendix

### A. Title Validation Rules
- Length: 1-32 characters
- Characters: Alphanumeric, spaces, dashes, underscores
- No leading/trailing whitespace
- Case-sensitive uniqueness check

### B. State Preservation Matrix

| Component | Rename | Restart | Notes |
|-----------|--------|---------|-------|
| Title | ✅ Changed | ✅ Preserved | Display name only |
| Tmux Session | ✅ Unchanged | ❌ Recreated | Name remains original |
| Git Worktree | ✅ Unchanged | ✅ Preserved | Path remains same |
| Branch | ✅ Preserved | ✅ Preserved | No modification |
| Uncommitted | ✅ Preserved | ✅ Preserved | In worktree |
| Terminal Output | ✅ Preserved | ❌ Lost | Unless saved to file |
| Claude Session | ✅ Preserved | ✅ Preserved | Used for --resume |
| Environment | ✅ Preserved | ✅ Restored | From saved state |

### C. Error Code Reference

| Code | Description | User Message | Recovery |
|------|-------------|--------------|----------|
| ERR_DUPLICATE_TITLE | Title already exists | "A session with this name already exists" | Choose different name |
| ERR_INVALID_TITLE | Invalid characters/length | "Title must be 1-32 characters, alphanumeric" | Fix input |
| ERR_RESTART_FAILED | Restart operation failed | "Failed to restart session" | Manual cleanup, retry |
| ERR_CONCURRENT_MOD | Version mismatch | "Session was modified elsewhere" | Reload and retry |
| ERR_SESSION_NOT_FOUND | Session doesn't exist | "Session no longer exists" | Refresh list |
