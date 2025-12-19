# Session Workspace Management Feature Plan

## Executive Summary

This feature enables users to modify working directories, switch branches/revisions, and manage worktrees for existing sessions. The implementation uses **Jujutsu (JJ) as the primary VCS** with automatic fallback to Git, preserving Claude conversation history across workspace changes.

## Problem Statement

### Current Limitations
1. **Immutable Working Directory**: Once a session is created, `WorkingDir` cannot be changed
2. **Fixed Branch Assignment**: Sessions are locked to their initial branch
3. **Static Worktree Binding**: No ability to move sessions between worktrees
4. **No Migration Path**: Users must destroy and recreate sessions to change these properties

### User Impact
- Developers lose Claude conversation context when switching branches
- Multiple sessions needed for the same project across different branches
- Poor user experience for exploratory development workflows

## Key Design Decisions

### Decision 1: JJ-First with Git Fallback
- **Primary**: Use Jujutsu (JJ) for all VCS operations
- **Fallback**: Automatic detection with Git fallback if JJ unavailable
- **Preference**: User-configurable VCS preference in config

### Decision 2: Working Copy Changes Handling
When switching revisions with uncommitted changes:
- **Default**: Keep changes as a separate WIP revision (parent of new location)
- **Option**: Bring changes along (kept as parent, user can squash later)
- **Option**: Abandon changes

### Decision 3: Worktree Creation
- Use `jj git clone --colocate` style setup for new worktrees
- JJ colocated repos work seamlessly with existing Git infrastructure

### Decision 4: Directory Changes
- Simple `cd` operation via tmux send-keys
- No VCS involvement for pure directory navigation

### Decision 5: Claude Session Continuity
- Preserve `claudeSession.SessionID` across workspace switches
- Restart Claude with `--resume <id>` after switch completes

## Architecture

### VCS Abstraction Layer

```go
// session/vcs/vcs.go

// VCSType represents the version control system in use
type VCSType string

const (
    VCSTypeJJ  VCSType = "jj"
    VCSTypeGit VCSType = "git"
)

// VCS is the abstraction over JJ and Git operations
type VCS interface {
    // Detection
    Type() VCSType

    // Status
    HasUncommittedChanges() (bool, error)
    GetCurrentRevision() (string, error)
    GetCurrentBookmark() (string, error) // JJ bookmark or Git branch

    // Navigation
    SwitchTo(target string, opts SwitchOptions) error
    CreateBookmark(name string, base string) error

    // Working copy management
    DescribeWIP(message string) error  // JJ: jj describe, Git: git stash
    AbandonChanges() error              // JJ: jj abandon, Git: git checkout .

    // Worktree management
    CreateWorktree(path string, bookmark string) error
    ListWorktrees() ([]Worktree, error)
}

// SwitchOptions configures how to handle the switch
type SwitchOptions struct {
    // How to handle uncommitted changes
    ChangeStrategy ChangeStrategy
    // Create bookmark/branch if target doesn't exist
    CreateIfMissing bool
    // Base revision for new bookmark (empty = current)
    BaseRevision string
}

type ChangeStrategy int
const (
    // KeepAsWIP keeps changes as a separate WIP revision
    KeepAsWIP ChangeStrategy = iota
    // BringAlong keeps changes as parent of new location
    BringAlong
    // Abandon discards uncommitted changes
    Abandon
)
```

### JJ Implementation

```go
// session/vcs/jj.go

type JJClient struct {
    repoPath string
    executor executor.Executor
}

func (j *JJClient) SwitchTo(target string, opts SwitchOptions) error {
    // 1. Handle uncommitted changes
    hasChanges, _ := j.HasUncommittedChanges()
    if hasChanges {
        switch opts.ChangeStrategy {
        case KeepAsWIP:
            // Describe current change as WIP
            j.run("jj", "describe", "-m", "WIP: uncommitted changes")
        case BringAlong:
            // Will squash after switch - just describe for now
            j.run("jj", "describe", "-m", "WIP: changes to bring along")
        case Abandon:
            j.run("jj", "abandon", "@")
        }
    }

    // 2. Switch to target
    if opts.CreateIfMissing {
        // Check if target exists
        if !j.bookmarkExists(target) {
            base := opts.BaseRevision
            if base == "" {
                base = "@"
            }
            // Create new change from base, then create bookmark
            j.run("jj", "new", base)
            j.run("jj", "bookmark", "create", target)
        } else {
            j.run("jj", "new", target)
        }
    } else {
        j.run("jj", "new", target)
    }

    // 3. Handle "bring along" by keeping WIP as parent
    // (user can jj squash later if desired)

    return nil
}

func (j *JJClient) CreateWorktree(path string, bookmark string) error {
    // Use jj workspace add for colocated setup
    return j.run("jj", "workspace", "add", "--name", bookmark, path)
}
```

### Git Fallback Implementation

```go
// session/vcs/git.go

type GitClient struct {
    repoPath string
    executor executor.Executor
}

func (g *GitClient) SwitchTo(target string, opts SwitchOptions) error {
    hasChanges, _ := g.HasUncommittedChanges()
    if hasChanges {
        switch opts.ChangeStrategy {
        case KeepAsWIP:
            g.run("git", "stash", "push", "-m", "claude-squad: WIP before switch")
        case BringAlong:
            g.run("git", "stash", "push", "-m", "claude-squad: bringing changes")
        case Abandon:
            g.run("git", "checkout", ".")
            g.run("git", "clean", "-fd")
        }
    }

    if opts.CreateIfMissing {
        g.run("git", "checkout", "-b", target)
    } else {
        g.run("git", "checkout", target)
    }

    // Restore stashed changes if bringing along
    if hasChanges && opts.ChangeStrategy == BringAlong {
        g.run("git", "stash", "pop")
    }

    return nil
}
```

### VCS Detection

```go
// session/vcs/detect.go

func Detect(repoPath string) (VCS, error) {
    // Check user preference first
    pref := config.GetVCSPreference()

    switch pref {
    case "jj":
        if jjAvailable(repoPath) {
            return NewJJClient(repoPath), nil
        }
        return nil, fmt.Errorf("JJ preferred but not available")
    case "git":
        return NewGitClient(repoPath), nil
    default:
        // Auto-detect: prefer JJ if available
        if jjAvailable(repoPath) {
            return NewJJClient(repoPath), nil
        }
        if gitAvailable(repoPath) {
            return NewGitClient(repoPath), nil
        }
        return nil, fmt.Errorf("no VCS detected in %s", repoPath)
    }
}

func jjAvailable(repoPath string) bool {
    // Check if jj command exists
    if _, err := exec.LookPath("jj"); err != nil {
        return false
    }
    // Check if repo is jj-managed (has .jj directory)
    jjDir := filepath.Join(repoPath, ".jj")
    if _, err := os.Stat(jjDir); err == nil {
        return true
    }
    // Check if colocated (has both .jj and .git)
    return false
}
```

### Instance Workspace Switch Method

```go
// session/instance_workspace.go

type WorkspaceSwitchType int
const (
    SwitchTypeDirectory WorkspaceSwitchType = iota  // Simple cd
    SwitchTypeRevision                               // JJ revision / Git branch
    SwitchTypeWorktree                               // Different worktree
)

type WorkspaceSwitchRequest struct {
    Type            WorkspaceSwitchType
    Target          string              // Directory path, revision/branch, or worktree path
    ChangeStrategy  vcs.ChangeStrategy  // How to handle uncommitted changes
    CreateIfMissing bool                // Create new bookmark/branch if doesn't exist
}

func (i *Instance) SwitchWorkspace(req WorkspaceSwitchRequest) error {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()

    // Validate session state
    if !i.started || i.Status == Paused {
        return fmt.Errorf("cannot switch workspace for stopped/paused session")
    }

    // Handle simple directory change separately (no VCS, no restart)
    if req.Type == SwitchTypeDirectory {
        return i.changeDirectory(req.Target)
    }

    // For revision/worktree switches, we need to restart Claude

    // 1. Get VCS client
    vcsClient, err := vcs.Detect(i.Path)
    if err != nil {
        return fmt.Errorf("failed to detect VCS: %w", err)
    }

    // 2. Preserve Claude session ID (already stored in i.claudeSession)
    // This will be used by ClaudeCommandBuilder on restart

    // 3. Kill tmux session (but keep claudeSession data)
    if err := i.KillSession(); err != nil {
        return fmt.Errorf("failed to stop session: %w", err)
    }
    i.started = false

    // 4. Perform VCS operation
    switch req.Type {
    case SwitchTypeRevision:
        err = vcsClient.SwitchTo(req.Target, vcs.SwitchOptions{
            ChangeStrategy:  req.ChangeStrategy,
            CreateIfMissing: req.CreateIfMissing,
        })
        if err != nil {
            // Try to recover by restarting at original location
            i.Start(false, nil)
            return fmt.Errorf("failed to switch revision: %w", err)
        }
        i.Branch = req.Target

    case SwitchTypeWorktree:
        // Either switch to existing worktree or create new one
        if req.CreateIfMissing {
            err = vcsClient.CreateWorktree(req.Target, i.Branch)
            if err != nil {
                i.Start(false, nil)
                return fmt.Errorf("failed to create worktree: %w", err)
            }
        }
        // Update instance to point to new worktree
        i.Path = req.Target
        i.gitWorktree, _ = git.NewGitWorktreeFromExisting(req.Target, i.Title)
    }

    // 5. Restart Claude (ClaudeCommandBuilder adds --resume automatically)
    if err := i.Start(false, nil); err != nil {
        return fmt.Errorf("failed to restart session: %w", err)
    }

    return nil
}

// changeDirectory handles simple directory navigation without VCS
func (i *Instance) changeDirectory(newDir string) error {
    // Validate path
    absPath, err := filepath.Abs(newDir)
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }

    // Security: ensure path is within allowed boundaries
    if err := validatePathSecurity(i.Path, absPath); err != nil {
        return err
    }

    // Send cd command to tmux
    cdCmd := fmt.Sprintf("cd %q\n", absPath)
    if _, err := i.tmuxSession.SendKeys(cdCmd); err != nil {
        return fmt.Errorf("failed to change directory: %w", err)
    }

    // Update instance state
    i.WorkingDir = absPath

    return nil
}
```

## UI Design

### TUI: Workspace Switch Overlay

```
┌─────────────────────────────────────────────────────────────────┐
│  Switch Workspace                                          [Esc]│
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Session: my-feature-session                                    │
│  Current: main @ abc123 (2 files modified)                      │
│  VCS: jj (colocated)                                           │
│                                                                 │
│  ┌─ Destination ───────────────────────────────────────────┐   │
│  │ > feature-auth         bookmark  (2 days ago)           │   │
│  │   feature-payments     bookmark  (1 week ago)           │   │
│  │   xyz789abc            change    "Fix login bug"        │   │
│  │   main                 bookmark  (up to date)           │   │
│  │   ─────────────────────────────────────────────────     │   │
│  │   [+] Create new bookmark...                            │   │
│  │   [w] Create new worktree...                            │   │
│  │   [d] Change directory only...                          │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  Uncommitted changes: (2 files)                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ (•) Keep as WIP revision                                │   │
│  │ ( ) Bring changes to destination                        │   │
│  │ ( ) Abandon changes                                     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ⚠ Claude will restart with conversation preserved              │
│                                                                 │
│  [Enter] Switch   [/] Filter   [n] New   [?] Help              │
└─────────────────────────────────────────────────────────────────┘
```

### Key Bindings

| Key | Action |
|-----|--------|
| `W` | Open workspace switch overlay (from main list) |
| `Enter` | Confirm switch to selected destination |
| `n` | Create new bookmark/branch at current location |
| `w` | Create new worktree |
| `d` | Change directory only (no VCS) |
| `/` | Filter/search destinations |
| `Tab` | Cycle through change handling options |
| `Esc` | Cancel |

### Web UI

Add workspace controls to session detail panel:
- Dropdown for revision/bookmark selection
- "Switch" button with confirmation modal
- Radio buttons for change handling strategy
- "New Bookmark" and "New Worktree" buttons

## Implementation Phases

### Phase 1: VCS Abstraction Layer (3-4 hours)
**Files:**
- `session/vcs/vcs.go` - Interface definitions
- `session/vcs/detect.go` - VCS detection logic
- `session/vcs/jj.go` - JJ implementation
- `session/vcs/git.go` - Git fallback implementation

**Tests:**
- VCS detection with JJ repo
- VCS detection with Git-only repo
- VCS detection with colocated repo
- Preference override behavior

### Phase 2: Instance Workspace Methods (3-4 hours)
**Files:**
- `session/instance_workspace.go` - New file with SwitchWorkspace method
- `session/instance.go` - Minor modifications for restart support

**Tests:**
- Directory change (simple cd)
- Revision switch with clean workspace
- Revision switch with uncommitted changes (all strategies)
- Worktree switch
- New bookmark creation
- Error recovery (restart at original location)

### Phase 3: TUI Overlay (4-5 hours)
**Files:**
- `ui/overlay/workspaceSwitch.go` - New overlay component
- `keys/keys.go` - Add workspace switch key binding
- `app/app.go` - Wire up key handler and overlay

**Tests:**
- Overlay rendering with bookmarks/branches
- Navigation and selection
- Change strategy toggling
- Filter/search functionality

### Phase 4: Web UI Integration (3-4 hours)
**Files:**
- `proto/session/v1/workspace.proto` - New protobuf definitions
- `server/services/workspace_service.go` - gRPC handlers
- `web-app/src/components/WorkspaceSwitch.tsx` - React component

### Phase 5: Configuration & Polish (2-3 hours)
**Files:**
- `config/config.go` - Add VCS preference setting
- Documentation updates
- Error message refinement

## Known Issues & Mitigations

### Issue 1: Claude Context Staleness
**Risk**: After switching revisions, Claude's understanding of the codebase may be stale.
**Mitigation**:
- Display warning in UI: "Claude will restart. File contents may have changed."
- Consider adding optional "context refresh" prompt to Claude after switch.

### Issue 2: JJ Operation Failures
**Risk**: JJ operations may fail (conflicts, missing revisions, etc.)
**Mitigation**:
- Always capture Claude session ID before any operation
- Implement rollback: restart session at original location on failure
- Parse JJ error messages for user-friendly display

### Issue 3: Tmux Session State
**Risk**: Killing tmux session loses terminal history.
**Mitigation**:
- This is acceptable - Claude conversation history is preserved via --resume
- Terminal history is less important than conversation continuity

### Issue 4: Concurrent Operations
**Risk**: Multiple workspace switch requests could race.
**Mitigation**:
- Use `stateMutex` in Instance (already exists)
- Disable UI controls during switch operation
- Show progress indicator

## Success Criteria

1. **Functional**: User can switch revisions while preserving Claude conversation
2. **JJ-First**: JJ operations work correctly when available
3. **Git Fallback**: Git operations work when JJ unavailable
4. **UX**: Switch operation completes in < 5 seconds for typical repos
5. **Safety**: Uncommitted changes are never lost without explicit user consent
6. **Recovery**: Failed operations leave session in usable state

## Configuration Schema

```json
{
  "workspace_management": {
    "vcs_preference": "auto",  // "auto" | "jj" | "git"
    "default_change_strategy": "keep_as_wip",  // "keep_as_wip" | "bring_along" | "abandon"
    "confirm_abandon": true,
    "show_recent_revisions": 10
  }
}
```

## Testing Strategy

### Unit Tests
- VCS detection logic
- JJ command building
- Git command building
- Path validation/security

### Integration Tests
- Full switch flow with mock VCS
- Error recovery scenarios
- Concurrent operation handling

### Manual Testing Checklist
- [ ] Switch revision with clean workspace (JJ)
- [ ] Switch revision with changes, keep as WIP (JJ)
- [ ] Switch revision with changes, bring along (JJ)
- [ ] Switch revision with changes, abandon (JJ)
- [ ] Create new bookmark and switch (JJ)
- [ ] Same tests with Git fallback
- [ ] Directory change only
- [ ] Worktree creation and switch
- [ ] Error recovery after failed switch
- [ ] Claude conversation continuity verified

---

*Document Version: 2.0.0*
*Updated: December 2024*
*Status: REFINED - Ready for Implementation*
