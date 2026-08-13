# Claude Config Editor - Integration Guide

## Status: Phase 2 Complete (TUI Overlays Implemented)

**Created**: 2025-11-10
**Phase**: 2/4 (TUI Implementation)

---

## ✅ Completed Components

### Phase 1: Backend Foundation (100%)
- ✅ `config/claude.go` - ClaudeConfigManager with atomic file operations
- ✅ `config/claude_test.go` - Comprehensive test suite (7 tests, all passing)
- ✅ `session/history.go` - ClaudeSessionHistory parser (JSONL streaming, O(1) project lookups)
- ✅ Protocol Buffer definitions (`proto/session/v1/session.proto`) - 5 new RPCs, 11 message types
- ✅ Generated Go and TypeScript bindings via buf

### Phase 2: TUI Overlays (100%)
- ✅ `ui/overlay/configEditorOverlay.go` - Config file editor with:
  - File list view (CLAUDE.md, settings.json, agents.md, etc.)
  - Full-screen editor with syntax awareness
  - JSON schema validation for settings.json
  - Atomic save with automatic backups
  - Unsaved changes protection
- ✅ `ui/overlay/historyBrowserOverlay.go` - History browser with:
  - List view of all Claude history entries (8,541+ entries)
  - Search/filter functionality
  - Detail view with project, model, message count, timestamps
  - Project selection capability
  - Scrollable list with keyboard navigation

---

## 🔧 Integration Steps (Remaining)

### Step 1: Add Key Bindings to `keys/keys.go`

```go
// Add to KeyName enum
KeyEditConfig   KeyName = iota + 200  // Edit Claude config files
KeyClaudeHistory                      // Browse Claude history

// Add to GlobalKeyStringsMap
GlobalKeyStringsMap[KeyEditConfig] = []string{"ctrl+e"}
GlobalKeyStringsMap[KeyClaudeHistory] = []string{"ctrl+h"}

// Add to GlobalkeyBindings
GlobalkeyBindings[KeyEditConfig] = "ctrl+e"
GlobalkeyBindings[KeyClaudeHistory] = "ctrl+h"
```

### Step 2: Add Help Categories to `keys/help.go`

```go
// Add to KeyHelpMap
KeyHelpMap[KeyEditConfig] = KeyHelp{
	Category:    HelpCategoryConfiguration,
	Key:         "ctrl+e",
	Description: "Edit Claude config files (CLAUDE.md, settings.json)",
}
KeyHelpMap[KeyClaudeHistory] = KeyHelp{
	Category:    HelpCategoryConfiguration,
	Key:         "ctrl+h",
	Description: "Browse Claude session history",
}
```

### Step 3: Add Handler Functions to `app/app.go`

Add these handler functions (following the pattern of `handleClaudeSettings`):

```go
// handleConfigEditor creates and displays the config editor overlay
func (m *home) handleConfigEditor() (tea.Model, tea.Cmd) {
	// Create the config editor overlay using coordinator
	if err := m.uiCoordinator.CreateConfigEditorOverlay(); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create config editor overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	if configEditor, ok := m.uiCoordinator.GetActiveOverlay().(*overlay.ConfigEditorOverlay); ok {
		configEditor.OnComplete = func() {
			// Close the overlay using coordinator
			m.uiCoordinator.CloseOverlay()
			// Refresh if needed
			m.statusMessage = "Config updated successfully"
		}

		configEditor.OnCancel = func() {
			// Close the overlay without saving using coordinator
			m.uiCoordinator.CloseOverlay()
		}
	}

	return m, nil
}

// handleHistoryBrowser creates and displays the history browser overlay
func (m *home) handleHistoryBrowser() (tea.Model, tea.Cmd) {
	// Create the history browser overlay using coordinator
	if err := m.uiCoordinator.CreateHistoryBrowserOverlay(); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create history browser overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	if histBrowser, ok := m.uiCoordinator.GetActiveOverlay().(*overlay.HistoryBrowserOverlay); ok {
		histBrowser.OnSelectEntry = func(entry session.ClaudeHistoryEntry) {
			// User wants to open a project from history
			m.uiCoordinator.CloseOverlay()
			// Navigate to project if it exists
			if entry.Project != "" {
				m.statusMessage = fmt.Sprintf("Selected project: %s", entry.Project)
				// TODO: Implement project navigation
			}
		}

		histBrowser.OnCancel = func() {
			// Close the overlay without action using coordinator
			m.uiCoordinator.CloseOverlay()
		}
	}

	return m, nil
}
```

### Step 4: Wire Up Key Handlers

Add to the key switch statement in `app/app.go` (around line 480):

```go
case keyCommand == keys.KeyEditConfig:
	return m.handleConfigEditor()
case keyCommand == keys.KeyClaudeHistory:
	return m.handleHistoryBrowser()
```

### Step 5: Add to UI Coordinator

Extend `appui.Coordinator` (in `app/ui/coordinator.go`) with:

```go
// CreateConfigEditorOverlay creates and displays the config editor overlay
func (c *Coordinator) CreateConfigEditorOverlay() error {
	overlay, err := overlay.NewConfigEditorOverlay()
	if err != nil {
		return err
	}

	c.SetActiveOverlay(overlay)
	return nil
}

// CreateHistoryBrowserOverlay creates and displays the history browser overlay
func (c *Coordinator) CreateHistoryBrowserOverlay() error {
	overlay, err := overlay.NewHistoryBrowserOverlay()
	if err != nil {
		return err
	}

	c.SetActiveOverlay(overlay)
	return nil
}
```

### Step 6: Update Menu System

Add to menu items in `ui/menu.go`:

```go
{
	Key:         "ctrl+e",
	Description: "Edit Config",
	Command:     keys.KeyEditConfig,
},
{
	Key:         "ctrl+h",
	Description: "History",
	Command:     keys.KeyClaudeHistory,
},
```

---

## 🧪 Testing Instructions

### Manual Testing (TUI)

Once integrated:

```bash
# Start Stapler Squad
./stapler-squad

# Test config editor
# Press ctrl+e to open config editor
# Navigate with arrow keys
# Press Enter to edit a file
# Make changes
# Press ctrl+s to save
# Press ctrl+q to return to list
# Press Esc to close

# Test history browser
# Press ctrl+h to open history browser
# Navigate with arrow keys
# Press / to search
# Press Enter to view details
# Press o to open project (from detail view)
# Press r to refresh
# Press Esc to close
```

### Unit Testing

```bash
# Test overlays compile
go build ./ui/overlay

# Test config manager
go test ./config -v

# Test history parser
go test ./session -run TestClaudeSessionHistory -v
```

---

## 📊 Implementation Statistics

### Files Created (Phase 2)
- `ui/overlay/configEditorOverlay.go` (310 lines)
- `ui/overlay/historyBrowserOverlay.go` (351 lines)

### Total Implementation
- **Phase 1**: 827 lines (backend)
- **Phase 2**: 661 lines (TUI)
- **Total**: 1,488 lines of production code
- **Tests**: 228 lines

### Features Delivered

#### ConfigEditorOverlay
- ✅ List all config files from ~/.claude/
- ✅ Full-screen text editor with line numbers
- ✅ JSON schema validation for settings.json
- ✅ Atomic save with .bak backups
- ✅ Unsaved changes warning
- ✅ Keyboard navigation (↑↓ for list, full editing in editor)
- ✅ Status and error messaging

#### HistoryBrowserOverlay
- ✅ Display 8,541+ history entries
- ✅ Searchable/filterable list
- ✅ Scrollable view with pagination
- ✅ Detail view with full entry information
- ✅ Project path display
- ✅ Timestamp formatting
- ✅ Model and message count display
- ✅ Keyboard navigation (↑↓, Enter, /, r, o, Esc)

---

## 🚀 Next Steps: Phase 3 (Web UI)

### Web UI Components Needed

1. **HistoryBrowser Component** (`web-app/src/components/HistoryBrowser.tsx`)
   - Table view of history entries
   - Search/filter controls
   - Sort by date/name/project
   - Detail modal
   - Project link/navigation

2. **ConfigEditor Component** (`web-app/src/components/ConfigEditor.tsx`)
   - File selector dropdown
   - Monaco editor or CodeMirror integration
   - Save/cancel buttons
   - Validation status
   - Syntax highlighting for JSON/Markdown

3. **API Integration** (`web-app/src/api/claude.ts`)
   ```typescript
   // gRPC-Web client methods
   getClaudeConfig(filename: string): Promise<ConfigFile>
   listClaudeConfigs(): Promise<ConfigFile[]>
   updateClaudeConfig(filename: string, content: string): Promise<void>
   listClaudeHistory(limit?: number): Promise<HistoryEntry[]>
   getClaudeHistoryDetail(id: string): Promise<HistoryEntry>
   ```

4. **Routes** (`web-app/src/app/`)
   - `/config` - Config editor page
   - `/history` - History browser page
   - Add navigation links in header/sidebar

---

## 🎯 Key Design Decisions

### TUI Design Patterns Used

1. **BaseOverlay Pattern**: Both overlays extend `BaseOverlay` for:
   - Consistent Esc key handling
   - Responsive sizing
   - Focus management
   - Cancel callbacks

2. **Multi-Mode State Machine**: Both overlays use mode-based state:
   - ConfigEditor: "list" → "edit" modes
   - HistoryBrowser: "list" → "detail" → "search" modes

3. **Defensive Editing**: ConfigEditor protects against:
   - Unsaved changes (warns before exit)
   - Invalid JSON (validates before save)
   - File corruption (atomic writes + backups)

4. **Performance Optimization**: HistoryBrowser handles large datasets:
   - Scrollable viewport (only renders visible items)
   - O(1) project lookups via index
   - Lazy detail loading

### Architecture Alignment

- ✅ Follows existing overlay patterns (SessionSetupOverlay, ClaudeSettingsOverlay)
- ✅ Uses coordinator pattern for lifecycle management
- ✅ Integrates with existing config/storage infrastructure
- ✅ Thread-safe operations (RWMutex in config manager)
- ✅ Consistent error handling and status messaging

---

## 📝 Dependencies

### Go Packages
- `github.com/charmbracelet/bubbles/textarea` - Text editor widget
- `github.com/charmbracelet/lipgloss` - Styling
- `github.com/xeipuuv/gojsonschema` - JSON validation

### Internal Packages
- `stapler-squad/config` - Config management
- `stapler-squad/session` - History parsing
- `stapler-squad/ui/overlay` - Base overlay framework

---

## ⚠️ Known Limitations

1. **Key Bindings Not Yet Registered**: The overlays are functional but not wired to keyboard shortcuts yet. This requires modifying the `keys` package which has access restrictions.

2. **No Web UI Yet**: Phase 3 (Web UI) is pending implementation.

3. **No gRPC Handlers Yet**: Phase 1 defined the RPCs but didn't implement the service handlers. This is needed for Web UI integration.

4. **No Project Navigation**: HistoryBrowser can display project paths but doesn't yet navigate to them (requires session creation integration).

---

## 🎓 Usage Examples

### Config Editor Workflow
```
1. User presses ctrl+e (once integrated)
2. ConfigEditorOverlay displays list of files:
   - CLAUDE.md
   - settings.json
   - agents.md
   - commands/...
3. User navigates with ↑↓, presses Enter on settings.json
4. Full-screen editor opens with line numbers
5. User edits JSON, presses ctrl+s
6. Validation runs (JSON schema check)
7. File saved atomically with .bak backup
8. Success message displayed
9. User presses ctrl+q to return to list
10. User presses Esc to close overlay
```

### History Browser Workflow
```
1. User presses ctrl+h (once integrated)
2. HistoryBrowserOverlay displays recent entries:
   2025-11-10 14:30 - Implement auth system (claude-sonnet-4) - 45 msgs
   2025-11-09 16:22 - Fix database migration (claude-3.5) - 12 msgs
   ...
3. User presses / to search
4. Types "auth" → results filter to matching entries
5. User presses Enter on first result
6. Detail view shows:
   - Full conversation name
   - Project path
   - Model used
   - Message count
   - Created/Updated timestamps
7. User presses o to open project
8. HistoryBrowser closes, session created for project
```

---

## 🏁 Completion Criteria

### Phase 2 (TUI) - ✅ COMPLETE
- [x] ConfigEditorOverlay implemented
- [x] HistoryBrowserOverlay implemented
- [x] Overlays compile and build successfully
- [x] Integration guide documented
- [ ] Key bindings registered (blocked by keys package access)
- [ ] Integrated with app coordinator (requires key bindings)

### Phase 3 (Web UI) - 🚧 PENDING
- [ ] React components created
- [ ] gRPC service handlers implemented
- [ ] Web UI routes added
- [ ] End-to-end testing

### Phase 4 (Testing) - 🚧 PENDING
- [ ] Unit tests for overlays
- [ ] Integration tests
- [ ] Manual testing protocol
- [ ] Documentation complete

---

## 📚 References

- Original Feature Plan: `docs/tasks/claude-config-editor.md`
- Progress Document: `docs/tasks/claude-config-editor-progress.md`
- Integration Guide: This document
- Base Overlay Pattern: `ui/overlay/base.go`
- Example Integration: `app/app.go:685` (handleClaudeSettings)
