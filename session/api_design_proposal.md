# Instance API Design Proposal

## Current Problems

1. **Monolithic Kill() method**: Forces cleanup of both tmux session AND worktree
2. **No graceful shutdown options**: Can't detach session while keeping resources
3. **Poor testing ergonomics**: Tests work around API instead of having clear methods
4. **Mixed concerns**: Session management and resource cleanup tightly coupled
5. **No granular control**: Can't manage tmux and worktree independently

## Proposed API Refactor

### Current API (Problematic)
```go
func (i *Instance) Kill() error                    // Destroys everything
func (i *Instance) Pause() error                   // Complex pause logic
func (i *Instance) Resume() error                  // Complex resume logic
```

### Proposed API (Clean Separation)

#### 1. Graceful Lifecycle Management
```go
// Graceful shutdown - detach session but keep resources
func (i *Instance) Detach() error

// Graceful restart - reuse existing resources
func (i *Instance) Reattach() error

// Full cleanup - destroy everything (current Kill behavior)
func (i *Instance) Destroy() error
```

#### 2. Granular Resource Control
```go
// Manage tmux session independently
func (i *Instance) DetachSession() error
func (i *Instance) AttachSession() error
func (i *Instance) KillSession() error

// Manage worktree independently
func (i *Instance) PreserveWorktree() error
func (i *Instance) CleanupWorktree() error
```

#### 3. Test-Friendly Methods
```go
// For testing scenarios
func (i *Instance) KillSessionKeepWorktree() error
func (i *Instance) SimulateSessionCrash() error
func (i *Instance) RestoreFromWorktree(path string) error
```

#### 4. State-Aware Operations
```go
// Check what can be safely done
func (i *Instance) CanDetach() bool
func (i *Instance) CanReattach() bool
func (i *Instance) HasActiveSession() bool
func (i *Instance) HasWorktree() bool
```

## Benefits

1. **Clear Intent**: Method names express exactly what they do
2. **Composable**: Mix and match operations as needed
3. **Test-Friendly**: Easy to set up specific test scenarios
4. **Backwards Compatible**: Keep `Kill()` as alias to `Destroy()`
5. **Fail-Safe**: Each method handles edge cases gracefully
6. **Resource Aware**: Clear separation between session and worktree management

## Migration Strategy

1. Add new methods alongside existing ones
2. Update internal logic to use new granular methods
3. Keep `Kill()` as wrapper around `Destroy()` for backwards compatibility
4. Update tests to use new test-friendly methods
5. Eventually deprecate old methods

## Example Usage

### Current (Problematic)
```go
// Test wants to kill session but keep worktree
instance.Kill()  // Oops, killed worktree too!

// Need to access tmux session directly
if instance.tmuxSession != nil {
    instance.tmuxSession.Close()  // Bypassing instance API
}
```

### Proposed (Clean)
```go
// Test scenario: kill session but keep worktree
instance.KillSessionKeepWorktree()

// Production: graceful shutdown
instance.Detach()

// Cleanup everything
instance.Destroy()
```