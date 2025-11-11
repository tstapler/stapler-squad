# Feature Implementation Plan: History Viewer Integration

**Epic ID**: FEATURE-002-Integration
**Status**: Ready for Implementation
**Priority**: P2 - After Web UI Session Creation
**Estimated Effort**: 8-10 hours (1-2 days)
**Progress**: Backend 100%, TUI 100%, Integration 0%

---

## Executive Summary

The Claude Session History Viewer backend and TUI overlay are **100% complete** but not integrated into the main application. This plan breaks down the remaining integration work into atomic, context-bounded tasks following the AIC framework.

**Current State**:
- ✅ **Backend Complete**: `session/history.go` (236 lines) - Fully implemented with JSONL parsing, search, filtering
- ✅ **TUI Overlay Complete**: `ui/overlay/historyBrowserOverlay.go` (350 lines) - Full overlay with list/detail modes
- ✅ **RPC Handlers Complete**: `server/services/session_service.go` - ListClaudeHistory, GetClaudeHistoryDetail
- ✅ **Proto Definitions Complete**: `proto/session/v1/session.proto` - ClaudeHistoryEntry messages
- ✅ **Web UI Complete**: `web-app/src/app/history/page.tsx` (264 lines) - Full history browser page
- ❌ **TUI Integration Missing**: Not wired into app.go state machine or menu system

**What's Missing**:
1. Add `HistoryBrowser` state to `app/state/types.go`
2. Create `handleHistoryBrowser()` function in `app/app.go`
3. Add `handleHistoryBrowserState()` for key handling
4. Wire up history browser to UI coordinator
5. Add key binding (H key) to menu and bridge

---

## Story 1: TUI History Browser Integration

**Goal**: Wire history browser overlay into main app state machine and menu system

**Estimated Effort**: 8 hours
**Context Boundary**: 5 files, ~600 lines total context ✅
**Dependencies**: None (all backend work complete)

### Task 1.1: Add HistoryBrowser State [1 hour]

**Scope**: Add HistoryBrowser to application state enum

**Files** (1 file, 75 lines):
- `app/state/types.go` (modify) - Add HistoryBrowser state constant

**Context**:
- Understand existing state patterns (ClaudeSettings, TagEditor)
- Follow IsOverlayState() pattern for modal overlays
- Update String() method for human-readable name

**Implementation**:
```go
// In app/state/types.go

const (
    // ... existing states ...
    TagEditor
    HistoryBrowser  // NEW: Add after TagEditor
)

func (s State) String() string {
    switch s {
    // ... existing cases ...
    case TagEditor:
        return "TagEditor"
    case HistoryBrowser:  // NEW
        return "HistoryBrowser"
    default:
        return "Unknown"
    }
}

func (s State) IsValid() bool {
    return s >= Default && s <= HistoryBrowser  // Update upper bound
}

func (s State) IsOverlayState() bool {
    switch s {
    case New, Prompt, Help, Confirm, AdvancedNew, Git, ClaudeSettings, ZFSearch, TagEditor, HistoryBrowser:  // Add HistoryBrowser
        return true
    default:
        return false
    }
}
```

**Success Criteria**:
- ✅ HistoryBrowser constant added to State enum
- ✅ String() method returns "HistoryBrowser"
- ✅ IsValid() returns true for HistoryBrowser
- ✅ IsOverlayState() returns true for HistoryBrowser
- ✅ Code compiles: `go build ./app/state`

**Testing**: Run `go test ./app/state -v` to verify state changes compile

**Dependencies**: None
**Status**: ⏳ Pending

---

### Task 1.2: Add UI Coordinator History Browser Methods [2 hours]

**Scope**: Add history browser management to UI coordinator

**Files** (2 files, ~200 lines):
- `app/ui/coordinator.go` (modify) - Add historyBrowserOverlay field and methods
- `app/ui/components.go` (modify) - Add ComponentHistoryBrowserOverlay constant

**Context**:
- Follow existing overlay patterns (ClaudeSettings, TagEditor)
- Use overlay.NewHistoryBrowserOverlay() constructor
- Implement Create/Get/Hide methods consistently

**Implementation**:
```go
// In app/ui/components.go
const (
    // ... existing components ...
    ComponentTagEditorOverlay
    ComponentHistoryBrowserOverlay  // NEW
)

// In app/ui/coordinator.go
type UICoordinator struct {
    // ... existing fields ...
    historyBrowserOverlay *overlay.HistoryBrowserOverlay  // NEW
}

func (c *UICoordinator) CreateHistoryBrowserOverlay() error {
    if c.historyBrowserOverlay != nil {
        return fmt.Errorf("history browser overlay already exists")
    }

    hbOverlay, err := overlay.NewHistoryBrowserOverlay()
    if err != nil {
        return fmt.Errorf("failed to create history browser overlay: %w", err)
    }

    c.historyBrowserOverlay = hbOverlay

    // Set callbacks
    hbOverlay.OnSelectEntry = func(entry session.ClaudeHistoryEntry) {
        // TODO: Launch session based on history entry
        // This will be implemented in Phase 2 (Story 2)
    }

    hbOverlay.OnCancel = func() {
        c.HideOverlay(ComponentHistoryBrowserOverlay)
    }

    c.activeOverlay = ComponentHistoryBrowserOverlay
    return nil
}

func (c *UICoordinator) GetHistoryBrowserOverlay() *overlay.HistoryBrowserOverlay {
    return c.historyBrowserOverlay
}

func (c *UICoordinator) HideHistoryBrowserOverlay() {
    c.historyBrowserOverlay = nil
    if c.activeOverlay == ComponentHistoryBrowserOverlay {
        c.activeOverlay = ComponentNone
    }
}
```

**Success Criteria**:
- ✅ ComponentHistoryBrowserOverlay constant added
- ✅ historyBrowserOverlay field added to UICoordinator
- ✅ CreateHistoryBrowserOverlay() implemented with error handling
- ✅ GetHistoryBrowserOverlay() returns overlay instance
- ✅ HideHistoryBrowserOverlay() cleans up properly
- ✅ Callbacks wired up (OnSelectEntry, OnCancel)
- ✅ Code compiles: `go build ./app/ui`

**Testing**: Unit test for overlay creation/destruction lifecycle

**Dependencies**: Task 1.1 (HistoryBrowser state defined)
**Status**: ⏳ Pending

---

### Task 1.3: Add handleHistoryBrowser Function [2 hours]

**Scope**: Create main handler function to open history browser overlay

**Files** (1 file, ~2800 lines total):
- `app/app.go` (modify) - Add handleHistoryBrowser() following TagEditor pattern

**Context**:
- Line 766: `handleTagEditor()` is the reference pattern
- Line 685: `handleClaudeSettings()` shows full example
- Follow state transition pattern: create overlay → transition to state

**Implementation**:
```go
// Add after handleTagEditor() around line 810 in app/app.go

func (m *home) handleHistoryBrowser() (tea.Model, tea.Cmd) {
    // Create history browser overlay
    if err := m.uiCoordinator.CreateHistoryBrowserOverlay(); err != nil {
        return m, m.handleError(fmt.Errorf("failed to open history browser: %w", err))
    }

    historyOverlay := m.uiCoordinator.GetHistoryBrowserOverlay()
    if historyOverlay == nil {
        return m, m.handleError(fmt.Errorf("history browser overlay is nil"))
    }

    // Set up callbacks
    historyOverlay.OnSelectEntry = func(entry session.ClaudeHistoryEntry) {
        // TODO: Launch session from history entry
        // For now, just close the overlay
        m.uiCoordinator.HideOverlay(appui.ComponentHistoryBrowserOverlay)
        m.transitionToDefault()

        // Future: Create new session based on entry.Project path
        // This will be implemented in Story 2
    }

    historyOverlay.OnCancel = func() {
        m.uiCoordinator.HideOverlay(appui.ComponentHistoryBrowserOverlay)
        m.transitionToDefault()
    }

    // Set menu state
    if m.menu != nil {
        if currentItem := m.list.CurrentItem(); currentItem != nil {
            m.menu.SetState(ui.StateSessionSelected)
        } else {
            m.menu.SetState(ui.StateDefault)
        }
    }

    // Transition to history browser state
    m.transitionToState(state.HistoryBrowser)

    return m, nil
}
```

**Success Criteria**:
- ✅ handleHistoryBrowser() function implemented
- ✅ Error handling for overlay creation
- ✅ OnSelectEntry callback wired up (placeholder implementation)
- ✅ OnCancel callback transitions back to default
- ✅ Menu state properly set
- ✅ State transition to HistoryBrowser
- ✅ Code compiles: `go build ./app`

**Testing**: Manual - open TUI and verify history overlay opens

**Dependencies**: Task 1.2 (UI coordinator methods)
**Status**: ⏳ Pending

---

### Task 1.4: Add handleHistoryBrowserState Key Handler [2 hours]

**Scope**: Handle key presses when in HistoryBrowser state

**Files** (1 file):
- `app/app.go` (modify) - Add handleHistoryBrowserState() function

**Context**:
- Line 1706: `handleClaudeSettingsState()` is reference pattern
- Line ~1714: `handleTagEditorState()` shows simpler overlay pattern
- Delegate key handling to overlay's HandleKeyPress() method

**Implementation**:
```go
// Add after handleTagEditorState() around line 1750 in app/app.go

func (m *home) handleHistoryBrowserState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    historyOverlay := m.uiCoordinator.GetHistoryBrowserOverlay()
    if historyOverlay == nil {
        // Overlay disappeared, return to default
        m.transitionToDefault()
        return m, nil
    }

    // Let overlay handle the key press
    shouldClose := historyOverlay.HandleKeyPress(msg)
    if shouldClose {
        // Overlay requested close
        m.uiCoordinator.HideOverlay(appui.ComponentHistoryBrowserOverlay)
        m.transitionToDefault()
    }

    return m, nil
}
```

**Wire into handleKeyPress()**:
```go
// Add to handleKeyPress() around line 1715 in app/app.go

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, teaCmd tea.Cmd) {
    // ... existing state handlers ...
    
    if m.isInState(state.TagEditor) {
        return m.handleTagEditorState(msg)
    }

    if m.isInState(state.HistoryBrowser) {  // NEW
        return m.handleHistoryBrowserState(msg)
    }

    // ... rest of function ...
}
```

**Success Criteria**:
- ✅ handleHistoryBrowserState() function implemented
- ✅ Null check for overlay existence
- ✅ Key delegation to overlay.HandleKeyPress()
- ✅ Proper cleanup and state transition on close
- ✅ Wired into handleKeyPress() switch statement
- ✅ Code compiles: `go build ./app`

**Testing**: Manual - navigate history overlay with arrow keys, ESC to close

**Dependencies**: Task 1.3 (handleHistoryBrowser function)
**Status**: ⏳ Pending

---

### Task 1.5: Add Key Binding and Menu Integration [1 hour]

**Scope**: Add H key binding to open history browser

**Files** (2 files):
- `app/app.go` (modify) - Add key handler for "H" key
- `app/bridge.go` (modify) - Add to bridge available commands

**Context**:
- Line 480: OnClaudeSettings in bridge shows pattern
- Need to add "H" key handler in default state key switch
- Add to menu system for discoverability

**Implementation**:
```go
// In app/app.go, add to bridge initialization around line 480

OnClaudeSettings: func() (tea.Model, tea.Cmd) {
    return m.handleClaudeSettings()
},
OnTagEditor: func() (tea.Model, tea.Cmd) {
    return m.handleTagEditor()
},
OnHistoryBrowser: func() (tea.Model, tea.Cmd) {  // NEW
    return m.handleHistoryBrowser()
},
```

**Add to key handling in default state**:
```go
// Find the switch statement that handles keys in default state
// Add "H" or "shift+h" case:

case "H", "shift+h":
    return m.handleHistoryBrowser()
```

**Add to bridge.go available commands**:
```go
// In app/bridge.go GetAvailableKeys()
// Add to returned commands:
{
    Key:         "H",
    Description: "History",
    Action:      OnHistoryBrowser,
}
```

**Success Criteria**:
- ✅ "H" key opens history browser overlay
- ✅ Menu displays "H: History" when available
- ✅ Bridge exposes OnHistoryBrowser callback
- ✅ Key works in default state
- ✅ Manual test: Press H to open history browser

**Testing**: Manual - press H key in TUI, verify overlay opens

**Dependencies**: Task 1.4 (state handler)
**Status**: ⏳ Pending

---

## Story 2: Launch Session from History [FUTURE]

**Status**: Deferred (Phase 2)
**Estimated Effort**: 6 hours

This story will enable launching new sessions based on historical session data. Currently blocked as "nice-to-have" but not required for basic history viewing.

**Tasks**:
- Task 2.1: Implement OnSelectEntry session creation (3h)
- Task 2.2: Handle worktree/branch setup from history (2h)
- Task 2.3: Pre-populate session fields from history entry (1h)

---

## Testing Strategy

### Unit Tests
- `app/state/types_test.go` - Test HistoryBrowser state validity
- `app/ui/coordinator_test.go` - Test overlay lifecycle

### Integration Tests
- Manual TUI testing checklist:
  1. Press H to open history browser
  2. Navigate with arrow keys (j/k)
  3. Search with '/' key
  4. View entry details with Enter
  5. Close with ESC
  6. Verify no memory leaks after open/close cycles

### Performance
- History loading should be < 100ms for 1000 entries (already achieved)
- Overlay rendering should be instant (no observable lag)

---

## Dependencies Already Satisfied

- ✅ session.ClaudeSessionHistory fully implemented
- ✅ overlay.HistoryBrowserOverlay fully implemented
- ✅ RPC handlers complete (ListClaudeHistory, GetClaudeHistoryDetail)
- ✅ Proto definitions complete
- ✅ Web UI implementation complete
- ✅ No external package dependencies needed

---

## Risk Mitigation

**Risk**: ~/.claude/history.jsonl doesn't exist
**Mitigation**: NewClaudeSessionHistoryFromClaudeDir() handles missing file gracefully (returns empty history)

**Risk**: Large history files (>10MB) slow down startup
**Mitigation**: Already implemented streaming parser with 1MB line buffer

**Risk**: Concurrent access to history.jsonl
**Mitigation**: Uses sync.RWMutex for thread-safe access

---

## Progress Tracking

**Overall Progress**: 0% (Integration not started)

### Story 1: TUI Integration (0/5 tasks)
- [ ] Task 1.1: Add HistoryBrowser State [1h] - ⏳ **NEXT**
- [ ] Task 1.2: UI Coordinator Methods [2h]
- [ ] Task 1.3: handleHistoryBrowser Function [2h]
- [ ] Task 1.4: handleHistoryBrowserState Handler [2h]
- [ ] Task 1.5: Key Binding & Menu [1h]

### Story 2: Launch from History (DEFERRED)
- [ ] Task 2.1: Session creation from entry [3h]
- [ ] Task 2.2: Worktree/branch setup [2h]
- [ ] Task 2.3: Pre-populate fields [1h]

---

## Recommended Next Action

**Task 1.1: Add HistoryBrowser State** [1 hour]

**Why This Task?**:
1. **Zero Dependencies**: No prerequisites, can start immediately
2. **Minimal Context**: Single file (75 lines), trivial changes
3. **Foundation**: Unblocks all subsequent integration tasks
4. **Low Risk**: Simple enum addition, easy to verify
5. **Quick Win**: Completes in 1 hour, provides immediate progress

**Files to Understand**:
- `app/state/types.go` (75 lines) - State enum definitions

**Next After This**: Task 1.2 (UI Coordinator methods)

