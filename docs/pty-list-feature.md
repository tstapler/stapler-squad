# PTY List View - Feature Documentation

## Overview

The PTY List View provides a way to discover, monitor, and interact with running PTY (pseudo-terminal) sessions. This feature allows users to view all available Claude instances, orphaned processes, and other terminal tools, then connect to them directly.

## User Experience

### Accessing PTY List

Press `t` from the main session list to toggle into PTY list view.

```
┌─────────────────────────────────────────────────────────────────┐
│ Available PTYs (5)              │ PTY Preview: /dev/pts/12      │
├─────────────────────────────────┼───────────────────────────────┤
│ ▼ Squad Sessions (3)            │ claude-code v1.2.3            │
│   ● /12  feature-auth           │ Working directory: ~/project  │
│   ◐ /15  bug-fix-123            │                                │
│   ● /18  test-suite             │ > git status                  │
│                                 │ On branch main                │
│ ▼ Orphaned (1)                  │ Your branch is up to date.    │
│   ◯ /22  (claude)               │                                │
│                                 │ Changes not staged:           │
│ ▼ Other (1)                     │   modified: src/app.go        │
│   ◐ /25  (aider)                │                                │
│                                 │ > _                           │
├─────────────────────────────────────────────────────────────────┤
│ [t] Sessions │ [↑↓] Navigate │ [enter] Attach │ [i] Send Cmd │
│ [d] Disconnect │ [r] Refresh │ [?] Help │ [q] Quit            │
└─────────────────────────────────────────────────────────────────┘
```

### Key Bindings

**In PTY List View:**
- `t` - Toggle back to session list
- `↑/↓` - Navigate PTYs
- `enter` - Attach to selected PTY
- `i` - Send command to selected PTY
- `d` - Disconnect from current PTY
- `r` - Refresh PTY list
- `space` - Expand/collapse category
- `esc` - Return to session list

### Status Indicators

- `●` Green - Ready (waiting for input)
- `◐` Yellow - Busy (executing command)
- `◯` Gray - Idle (no recent activity)
- `✗` Red - Error (connection failed)

### Categories

1. **Squad Sessions** - PTYs associated with managed claude-squad sessions
2. **Orphaned** - Unmanaged Claude instances running independently
3. **Other** - Other tools (Aider, etc.)

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      Application Layer                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   app.go     │  │   commands   │  │   registry   │      │
│  │  (view mode) │←→│   (handlers) │←→│  (bindings)  │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│                        UI Layer                              │
│  ┌──────────────┐  ┌──────────────┐                         │
│  │  PTYList     │  │  PTYPreview  │                         │
│  │  (display)   │  │  (output)    │                         │
│  └──────────────┘  └──────────────┘                         │
└─────────────────────────────────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────────┐
│                      Service Layer                           │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              PTYDiscovery Service                     │   │
│  │  - Discovers PTYs from squad sessions               │   │
│  │  - Finds orphaned Claude processes                  │   │
│  │  - Status detection (Ready/Busy/Idle/Error)        │   │
│  │  - Background monitoring with auto-refresh          │   │
│  │  - Category organization                            │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Core Components

#### 1. PTYDiscovery Service (`session/pty_discovery.go`)

**Purpose:** Discovers and monitors PTY connections

**Key Methods:**
```go
func NewPTYDiscovery() *PTYDiscovery
func (pd *PTYDiscovery) Start()
func (pd *PTYDiscovery) Stop()
func (pd *PTYDiscovery) SetSessions(sessions []*Instance)
func (pd *PTYDiscovery) Refresh() error
func (pd *PTYDiscovery) GetConnections() []*PTYConnection
func (pd *PTYDiscovery) GetConnectionsByCategory() map[PTYCategory][]*PTYConnection
```

**Discovery Process:**
1. Scans squad-managed sessions for PTYs
2. Finds orphaned Claude processes via `pgrep`
3. Detects PTY status from process state
4. Categorizes connections
5. Refreshes automatically every 5 seconds

#### 2. PTYList Component (`ui/pty_list.go`)

**Purpose:** Displays list of available PTYs

**Key Methods:**
```go
func NewPTYList() *PTYList
func (pl *PTYList) SetConnections(connections []*PTYConnection)
func (pl *PTYList) MoveUp()
func (pl *PTYList) MoveDown()
func (pl *PTYList) ToggleCategory(category PTYCategory)
func (pl *PTYList) GetSelected() *PTYConnection
func (pl *PTYList) Render() string
```

**Features:**
- Category organization with expand/collapse
- Visual status indicators
- Selection tracking
- Filtered navigation

#### 3. PTYPreview Component (`ui/pty_preview.go`)

**Purpose:** Shows live output from selected PTY

**Key Methods:**
```go
func NewPTYPreview() *PTYPreview
func (pp *PTYPreview) SetConnection(conn *PTYConnection)
func (pp *PTYPreview) ScrollUp()
func (pp *PTYPreview) ScrollDown()
func (pp *PTYPreview) RefreshOutput()
func (pp *PTYPreview) Render() string
```

**Features:**
- Live output streaming
- Scroll support
- Metadata display
- Status information

## Integration Guide

### Adding PTY View to Application

**1. Add State to Application Model:**

```go
// In app/app.go
type home struct {
    // Existing fields...

    // PTY management
    viewMode      ViewMode              // Session list or PTY list
    ptyDiscovery  *session.PTYDiscovery // PTY discovery service
    ptyList       *ui.PTYList           // PTY list component
    ptyPreview    *ui.PTYPreview        // PTY preview pane
    activePTY     *session.PTYConnection // Currently connected PTY
}

type ViewMode int
const (
    ViewModeSessions ViewMode = iota
    ViewModePTYs
)
```

**2. Initialize PTY Components:**

```go
// In app initialization
func initialModel() home {
    h := home{
        // Existing initialization...

        viewMode:     ViewModeSessions,
        ptyDiscovery: session.NewPTYDiscovery(),
        ptyList:      ui.NewPTYList(),
        ptyPreview:   ui.NewPTYPreview(),
    }

    // Start PTY discovery
    h.ptyDiscovery.Start()

    return h
}
```

**3. Register Command Handlers:**

```go
// In app/app.go
func (h *home) registerPTYHandlers() {
    commands.SetPTYHandlers(&commands.PTYHandlers{
        OnTogglePTYView: h.handleTogglePTYView,
        OnAttachPTY:     h.handleAttachPTY,
        OnSendCommand:   h.handleSendCommandToPTY,
        OnDisconnectPTY: h.handleDisconnectPTY,
        OnRefreshPTYs:   h.handleRefreshPTYs,
    })
}

func (h *home) handleTogglePTYView() (tea.Model, tea.Cmd) {
    if h.viewMode == ViewModeSessions {
        h.viewMode = ViewModePTYs
        h.updatePTYList()
    } else {
        h.viewMode = ViewModeSessions
    }
    return h, nil
}

func (h *home) updatePTYList() {
    // Update PTY discovery with current sessions
    h.ptyDiscovery.SetSessions(h.sessionService.GetAllInstances())
    h.ptyDiscovery.Refresh()

    // Update PTY list component
    connections := h.ptyDiscovery.GetConnections()
    h.ptyList.SetConnections(connections)

    // Update preview with selected PTY
    if selected := h.ptyList.GetSelected(); selected != nil {
        h.ptyPreview.SetConnection(selected)
    }
}
```

**4. Update Render Logic:**

```go
func (h home) View() string {
    if h.viewMode == ViewModePTYs {
        return h.renderPTYView()
    }
    return h.renderSessionView()
}

func (h home) renderPTYView() string {
    leftPane := h.ptyList.Render()
    rightPane := h.ptyPreview.Render()

    // Split view 50/50
    return lipgloss.JoinHorizontal(
        lipgloss.Top,
        lipgloss.NewStyle().Width(h.width/2).Render(leftPane),
        lipgloss.NewStyle().Width(h.width/2).Render(rightPane),
    )
}
```

**5. Handle Navigation in PTY View:**

```go
func (h home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if h.viewMode == ViewModePTYs {
            return h.handlePTYViewKeys(msg)
        }
        return h.handleSessionViewKeys(msg)
    }
    return h, nil
}

func (h *home) handlePTYViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "up", "k":
        h.ptyList.MoveUp()
        h.updatePTYPreview()
    case "down", "j":
        h.ptyList.MoveDown()
        h.updatePTYPreview()
    case "space":
        if h.ptyList.IsOnCategoryHeader() {
            category := h.ptyList.GetSelectedCategory()
            h.ptyList.ToggleCategory(category)
        }
    }
    return h, nil
}
```

## Performance Considerations

### PTY Discovery
- **Refresh Rate:** 5 seconds (configurable)
- **Discovery Methods:**
  - Squad sessions: O(n) where n = number of sessions
  - Orphaned processes: System `pgrep` call
- **Memory:** Minimal - stores only metadata, not output buffers

### Output Buffering
- **PTYPreview Buffer:** 1000 lines maximum
- **ClaudeController Buffer:** 4KB circular buffer
- **Refresh Strategy:** On-demand when PTY selected

### Optimization Tips
1. Disable auto-refresh if not in PTY view
2. Limit preview buffer size for long-running sessions
3. Use lazy loading for orphaned PTY discovery
4. Cache PTY status between refreshes

## Testing

### Unit Tests

**PTYDiscovery Tests:**
```bash
go test ./session -run TestPTYDiscovery -v
```

**PTYList Tests:**
```bash
go test ./ui -run TestPTYList -v
```

**Integration Tests:**
```bash
go test ./app -run TestPTYViewIntegration -v
```

### Manual Testing Checklist

- [ ] Press `t` to toggle to PTY view
- [ ] Verify squad sessions appear in "Squad Sessions" category
- [ ] Navigate with arrow keys
- [ ] Expand/collapse categories with space
- [ ] Select PTY and verify preview updates
- [ ] Press `t` again to return to session list
- [ ] Verify status indicators update (run long command to see busy state)
- [ ] Test with no PTYs (empty state)
- [ ] Test with orphaned Claude process

## Troubleshooting

### PTY Not Discovered

**Issue:** Squad session not appearing in PTY list

**Solutions:**
1. Check session status (must be Running or Ready)
2. Verify tmux session is active: `tmux ls`
3. Check PTY discovery logs in `~/.claude-squad/logs/`
4. Manually refresh with `r` key

### Preview Shows No Output

**Issue:** PTY preview pane is empty

**Solutions:**
1. Verify ClaudeController is initialized for session
2. Check PTY file descriptor permissions
3. Ensure GetPTYReader() is working: check logs
4. Try selecting different PTY

### Status Always Shows Idle

**Issue:** PTY status doesn't update

**Solutions:**
1. Check `/proc/{pid}/stat` access
2. Verify PTY discovery refresh is running
3. Check process state detection logic
4. Increase refresh rate for testing

## Future Enhancements

### Planned Features
1. **Multi-PTY Broadcast** - Send command to multiple PTYs simultaneously
2. **PTY Recording** - Record and playback PTY sessions
3. **Output Filtering** - Search and filter PTY output
4. **Custom Categories** - User-defined PTY grouping
5. **PTY Metrics** - CPU/memory usage per PTY
6. **Connection Sharing** - Multiple clients to same PTY

### API Extensions
```go
// Future PTYDiscovery methods
func (pd *PTYDiscovery) RecordPTY(path string, outputFile string) error
func (pd *PTYDiscovery) GetMetrics(path string) (*PTYMetrics, error)
func (pd *PTYDiscovery) BroadcastCommand(paths []string, command string) error
```

## References

- PTY Discovery Implementation: `session/pty_discovery.go`
- UI Components: `ui/pty_list.go`, `ui/pty_preview.go`
- Command Handlers: `cmd/commands/pty.go`
- Command Registry: `cmd/init.go` (PTY commands section)
- Test Suite: `session/pty_discovery_test.go`

## Contributing

When extending PTY list functionality:

1. **Add Tests** - Unit tests for new discovery methods
2. **Update Docs** - Document new key bindings and features
3. **Performance** - Profile discovery methods for large PTY counts
4. **Error Handling** - Graceful degradation when PTYs unavailable
5. **Accessibility** - Maintain keyboard-only navigation

## Version History

- **v1.0.0** (2025-01-06) - Initial implementation
  - PTY discovery service
  - Squad session integration
  - Orphaned process detection
  - Split-pane UI with preview
  - Category organization
  - Status indicators
  - Command handlers and key bindings
