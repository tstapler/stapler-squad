# Feature Plan: Claude Config Editor & Session History Viewer

**Epic ID**: FEATURE-001  
**Status**: Planning  
**Priority**: High  
**Estimated Effort**: 3-4 weeks (1 engineer)  
**Created**: 2025-11-10  
**Owner**: Engineering Team

---

## Executive Summary

Add comprehensive Claude Code configuration management and session history viewing capabilities to Claude Squad. This feature enables users to:

1. **View and edit Claude configuration files** (.claude/CLAUDE.md, settings.json) within the TUI/Web UI
2. **Browse historical Claude sessions** from the ~/.claude directory
3. **Launch new sessions based on historical sessions** for resuming past work
4. **Review past work** through session history browsing and search

**Strategic Value**:
- **Productivity**: Users can quickly resume interrupted work by launching sessions from history
- **Transparency**: Direct access to Claude configs eliminates manual file editing
- **Workflow Integration**: Tight integration between Claude Code and Claude Squad reduces context switching

**Technical Approach**: Extend existing overlay system with new config editor and history browser components, leverage existing session management infrastructure, and add new protobuf RPCs for config operations.

---

## Table of Contents

1. [Discovery & Requirements](#1-discovery--requirements)
2. [Architecture & Design](#2-architecture--design)
3. [Quality & Testing Strategy](#3-quality--testing-strategy)
4. [Implementation Roadmap](#4-implementation-roadmap)
5. [Documentation & Artifacts](#5-documentation--artifacts)
6. [Known Issues & Mitigation](#6-known-issues--mitigation)
7. [Appendices](#7-appendices)

---

## 1. Discovery & Requirements

### 1.1 Problem Statement

**Current Pain Points**:
- Users must manually edit Claude config files outside the application
- No visibility into historical Claude sessions or conversations
- Cannot easily resume past work contexts
- Difficult to audit or review past Claude interactions
- No structured way to reuse successful session configurations

**User Stories**:

```
US-001: View Claude Configuration
As a Claude Squad user
I want to view my Claude configuration files (CLAUDE.md, settings.json) in the UI
So that I can understand my current Claude setup without leaving the application

Acceptance Criteria:
- GIVEN I press a config key (e.g., 'C')
- WHEN the config viewer opens
- THEN I see syntax-highlighted .claude/CLAUDE.md content
- AND I can navigate between CLAUDE.md, settings.json, and agents.md
- AND file paths are clearly displayed
```

```
US-002: Edit Claude Configuration
As a Claude Squad user
I want to edit my Claude configuration files with validation
So that I can customize Claude behavior without manual file editing

Acceptance Criteria:
- GIVEN the config editor is open
- WHEN I press 'e' to enable edit mode
- THEN I can modify the configuration text
- AND changes are validated before saving (JSON schema for settings.json)
- AND I receive clear error messages for invalid configurations
- AND changes persist to ~/.claude/ files on save
```

```
US-003: Browse Session History
As a Claude Squad user
I want to browse my historical Claude sessions by directory
So that I can review past work and understand previous interactions

Acceptance Criteria:
- GIVEN I press a history key (e.g., 'H')
- WHEN the history browser opens
- THEN I see sessions grouped by directory/project
- AND I can filter by date range (last 7 days, 30 days, all time)
- AND I can search by directory name or session metadata
- AND I see session timestamps and summary information
```

```
US-004: Launch Session from History
As a Claude Squad user
I want to create a new session based on a historical session
So that I can resume work in the same context

Acceptance Criteria:
- GIVEN I have selected a historical session
- WHEN I press Enter or click "Launch"
- THEN a new Claude Squad session is created
- AND the session uses the same directory, program, and category
- AND I can optionally include the previous prompt/context
- AND the new session appears in my active sessions list
```

```
US-005: Review Past Work
As a Claude Squad user
I want to view the history/conversation log for a past session
So that I can understand what was accomplished

Acceptance Criteria:
- GIVEN I have selected a historical session
- WHEN I press 'v' to view details
- THEN I see the session's conversation history (if available)
- AND I see any output logs or terminal history
- AND I can scroll through the history with vim keybindings
- AND I can search within the history content
```

### 1.2 Functional Requirements

#### FR-1: Configuration File Management
- **FR-1.1**: Read and parse ~/.claude/CLAUDE.md, settings.json, and agents.md
- **FR-1.2**: Display configurations with syntax highlighting (Markdown for .md, JSON for .json)
- **FR-1.3**: Support in-place editing with validation
- **FR-1.4**: Validate JSON schema for settings.json before saving
- **FR-1.5**: Provide clear error messages for invalid configurations
- **FR-1.6**: Support file watching for external changes (reload prompt)

#### FR-2: Session History Discovery
- **FR-2.1**: Parse ~/.claude/history.jsonl for session records
- **FR-2.2**: Extract session metadata: project path, timestamp, display type
- **FR-2.3**: Group sessions by directory/project
- **FR-2.4**: Support date range filtering (7d, 30d, 90d, all)
- **FR-2.5**: Implement fuzzy search across session metadata

#### FR-3: Session Launching from History
- **FR-3.1**: Create new session using historical session metadata
- **FR-3.2**: Preserve directory, program, category from historical session
- **FR-3.3**: Optionally reuse previous prompt/context
- **FR-3.4**: Handle missing directories gracefully (validation warning)
- **FR-3.5**: Support duplicate detection (warn if session already active)

#### FR-4: Past Work Review
- **FR-4.1**: Display conversation history if available (__store.db)
- **FR-4.2**: Show terminal output history if captured
- **FR-4.3**: Support scrolling and navigation (vim keybindings)
- **FR-4.4**: Implement search within history content
- **FR-4.5**: Handle missing history gracefully (show metadata only)

### 1.3 Non-Functional Requirements

#### NFR-1: Performance
- **NFR-1.1**: Config file loading < 100ms for files up to 10MB
- **NFR-1.2**: History parsing < 500ms for 10,000 history entries
- **NFR-1.3**: Search results returned < 200ms for typical queries
- **NFR-1.4**: UI remains responsive during file I/O operations (async)

#### NFR-2: Scalability
- **NFR-2.1**: Support history files up to 100MB (pagination)
- **NFR-2.2**: Handle 10,000+ historical sessions efficiently
- **NFR-2.3**: Incremental loading for large history lists (virtual scrolling)

#### NFR-3: Security
- **NFR-3.1**: Validate file paths to prevent directory traversal
- **NFR-3.2**: Sanitize config content before display (XSS prevention)
- **NFR-3.3**: Atomic file writes to prevent corruption
- **NFR-3.4**: Read-only mode for sensitive files (if configured)

#### NFR-4: Reliability
- **NFR-4.1**: Graceful handling of missing or corrupted files
- **NFR-4.2**: Backup creation before config modifications
- **NFR-4.3**: Rollback capability for failed config updates
- **NFR-4.4**: Transaction-safe file operations (temp file + rename)

#### NFR-5: Usability
- **NFR-5.1**: Consistent keybindings with existing TUI patterns
- **NFR-5.2**: Clear visual feedback for unsaved changes
- **NFR-5.3**: Confirmation prompts for destructive actions
- **NFR-5.4**: Contextual help available in all views

#### NFR-6: Maintainability
- **NFR-6.1**: Follow existing overlay pattern architecture
- **NFR-6.2**: Comprehensive unit test coverage (>80%)
- **NFR-6.3**: Integration tests for critical user flows
- **NFR-6.4**: Clear separation of concerns (config I/O, parsing, UI)

---

## 2. Architecture & Design

### 2.1 System Architecture

#### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Claude Squad TUI/Web UI                  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  New Overlays (TUI)                                   │  │
│  │  - ConfigEditorOverlay (CLAUDE.md, settings.json)    │  │
│  │  - SessionHistoryBrowserOverlay (history viewer)     │  │
│  │  - HistoryDetailOverlay (conversation viewer)        │  │
│  └──────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  New React Components (Web UI)                       │  │
│  │  - ConfigEditor (code editor with syntax highlight)  │  │
│  │  - SessionHistoryBrowser (list + filters)           │  │
│  │  - ConversationViewer (history detail view)         │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              ↕
┌─────────────────────────────────────────────────────────────┐
│              gRPC/Connect-RPC API (SessionService)           │
│  New RPCs:                                                   │
│  - GetClaudeConfig(GetClaudeConfigRequest)                  │
│  - UpdateClaudeConfig(UpdateClaudeConfigRequest)            │
│  - ListSessionHistory(ListSessionHistoryRequest)            │
│  - GetSessionHistoryDetail(GetSessionHistoryDetailRequest)  │
└─────────────────────────────────────────────────────────────┘
                              ↕
┌─────────────────────────────────────────────────────────────┐
│                 Go Backend Services (New)                    │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  config/claude.go                                     │  │
│  │  - ClaudeConfigManager (read/write configs)          │  │
│  │  - ConfigParser (parse CLAUDE.md, settings.json)     │  │
│  │  - ConfigValidator (JSON schema validation)          │  │
│  └──────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  session/history.go                                   │  │
│  │  - ClaudeHistoryParser (parse history.jsonl)         │  │
│  │  - SessionHistoryIndex (efficient search/filter)     │  │
│  │  - ConversationStore (access __store.db)             │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              ↕
┌─────────────────────────────────────────────────────────────┐
│                   File System Layer                          │
│  ~/.claude/                                                  │
│  ├── CLAUDE.md (configuration markdown)                     │
│  ├── settings.json (Claude settings)                        │
│  ├── agents.md (agent configurations)                       │
│  ├── history.jsonl (session history log)                    │
│  └── __store.db (SQLite conversation store)                 │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Component Design

#### 2.2.1 Backend Components

**config/claude.go - ClaudeConfigManager**

```go
// ClaudeConfigManager handles reading and writing Claude configuration files
type ClaudeConfigManager struct {
    claudeDir string // ~/.claude
    mu        sync.RWMutex
}

// ConfigFile represents a Claude configuration file
type ConfigFile struct {
    Name     string // "CLAUDE.md", "settings.json", "agents.md"
    Path     string // Full file path
    Content  string // File content
    Type     ConfigFileType // Markdown, JSON
    Modified time.Time
    Size     int64
}

type ConfigFileType int

const (
    ConfigTypeMarkdown ConfigFileType = iota
    ConfigTypeJSON
)

// Methods:
// - GetConfig(filename string) (*ConfigFile, error)
// - UpdateConfig(filename string, content string) error
// - ValidateConfig(filename string, content string) error
// - CreateBackup(filename string) error
// - ListConfigs() ([]*ConfigFile, error)
```

**session/history.go - ClaudeHistoryParser**

```go
// ClaudeHistoryParser parses ~/.claude/history.jsonl
type ClaudeHistoryParser struct {
    historyPath string
    index       *SessionHistoryIndex
    mu          sync.RWMutex
}

// HistoryEntry represents a single entry from history.jsonl
type HistoryEntry struct {
    Display        string    `json:"display"`
    PastedContents map[string]interface{} `json:"pastedContents"`
    Timestamp      int64     `json:"timestamp"`
    Project        string    `json:"project"`
}

// SessionHistorySummary is the aggregated view for UI
type SessionHistorySummary struct {
    ProjectPath      string
    SessionCount     int
    FirstSeen        time.Time
    LastSeen         time.Time
    RecentDisplay    string
    EstimatedContext string // Derived from pastedContents
}

// Methods:
// - Parse() ([]*HistoryEntry, error)
// - GroupByProject() (map[string][]*HistoryEntry, error)
// - FilterByDateRange(start, end time.Time) ([]*HistoryEntry, error)
// - Search(query string) ([]*HistoryEntry, error)
```

**session/conversation.go - ConversationStore**

```go
// ConversationStore accesses ~/.claude/__store.db (SQLite)
type ConversationStore struct {
    dbPath string
    db     *sql.DB
}

// ConversationRecord represents a conversation from __store.db
type ConversationRecord struct {
    ID        string
    ProjectID string
    Timestamp time.Time
    Messages  []Message
}

type Message struct {
    Role      string // "user", "assistant"
    Content   string
    Timestamp time.Time
}

// Methods:
// - GetConversationByProject(projectPath string) ([]*ConversationRecord, error)
// - GetConversationByID(id string) (*ConversationRecord, error)
// - SearchConversations(query string) ([]*ConversationRecord, error)
```

#### 2.2.2 UI Components (TUI)

**ui/overlay/configEditorOverlay.go - ConfigEditorOverlay**

```go
// ConfigEditorOverlay provides in-place editing of Claude configs
type ConfigEditorOverlay struct {
    BaseOverlay
    
    // State
    file         *config.ConfigFile
    originalContent string
    editedContent   string
    editMode        bool
    cursorRow       int
    cursorCol       int
    scrollOffset    int
    hasUnsavedChanges bool
    
    // Validation
    validationErrors []string
    
    // UI components
    textArea     *TextAreaComponent
    statusBar    *StatusBar
    fileSelector *NavigationHandler // Switch between files
    
    // Callbacks
    onSave       func(*config.ConfigFile) error
    onClose      func()
}

// Key bindings:
// - e: Toggle edit mode
// - Esc: Exit edit mode or close overlay
// - Ctrl+S: Save changes
// - Tab: Switch between config files
// - /: Search within config
```

**ui/overlay/sessionHistoryBrowserOverlay.go - SessionHistoryBrowserOverlay**

```go
// SessionHistoryBrowserOverlay displays historical Claude sessions
type SessionHistoryBrowserOverlay struct {
    BaseOverlay
    
    // State
    historySummaries []*session.SessionHistorySummary
    selectedIndex    int
    filterDateRange  DateRange
    searchQuery      string
    
    // UI components
    list            *List
    filterBar       *FilterBar
    previewPane     *PreviewPane
    
    // Callbacks
    onLaunchSession func(*session.SessionHistorySummary)
    onViewDetails   func(*session.SessionHistorySummary)
}

// Key bindings:
// - Enter: Launch new session from selected history
// - v: View conversation details
// - /: Search history
// - f: Filter by date range
// - j/k: Navigate list
```

**ui/overlay/historyDetailOverlay.go - HistoryDetailOverlay**

```go
// HistoryDetailOverlay shows conversation history for a session
type HistoryDetailOverlay struct {
    BaseOverlay
    
    // State
    conversation *session.ConversationRecord
    scrollOffset int
    searchQuery  string
    searchResults []int // Line numbers with matches
    
    // UI components
    conversationView *ScrollableTextView
    searchBar        *SearchBar
}

// Key bindings:
// - /: Search within conversation
// - n/N: Next/previous search result
// - j/k: Scroll
// - Esc: Close overlay
```

#### 2.2.3 UI Components (Web)

**web-app/src/components/config/ConfigEditor.tsx**

```typescript
interface ConfigEditorProps {
  configFile: ConfigFile;
  onSave: (content: string) => Promise<void>;
  onClose: () => void;
  readOnly?: boolean;
}

// Features:
// - Syntax highlighting (Monaco Editor or CodeMirror)
// - Validation on change
// - Diff view (show changes from original)
// - Dark/light mode support
```

**web-app/src/components/sessions/SessionHistoryBrowser.tsx**

```typescript
interface SessionHistoryBrowserProps {
  onLaunchSession: (history: SessionHistorySummary) => void;
  onViewDetails: (history: SessionHistorySummary) => void;
}

// Features:
// - Grouped by project (collapsible)
// - Date range filter (dropdown)
// - Search bar with debouncing
// - Virtual scrolling for large lists
// - Session count badges
```

**web-app/src/components/sessions/ConversationViewer.tsx**

```typescript
interface ConversationViewerProps {
  conversation: ConversationRecord;
  onClose: () => void;
}

// Features:
// - Chat-style message rendering
// - Code block syntax highlighting
// - Search within conversation
// - Export conversation (markdown/JSON)
```

### 2.3 Data Flow Diagrams

#### View Config Flow

```
User presses 'C' (config key)
  ↓
App dispatches ConfigViewRequest
  ↓
UI Coordinator shows ConfigEditorOverlay
  ↓
Overlay calls GetClaudeConfig RPC
  ↓
Backend: ClaudeConfigManager.GetConfig("CLAUDE.md")
  ↓
Read ~/.claude/CLAUDE.md from filesystem
  ↓
Parse file and return ConfigFile proto
  ↓
Overlay renders content with syntax highlighting
  ↓
User navigates (Tab to switch files, / to search)
```

#### Edit Config Flow

```
User presses 'e' (enable edit mode)
  ↓
Overlay enables text editing
  ↓
User modifies content
  ↓
On-change validation (JSON schema for settings.json)
  ↓
User presses Ctrl+S (save)
  ↓
Overlay calls UpdateClaudeConfig RPC
  ↓
Backend: ClaudeConfigManager.ValidateConfig()
  ↓
If valid: CreateBackup() → UpdateConfig()
  ↓
Atomic write (temp file + rename)
  ↓
Return success/error to overlay
  ↓
Overlay shows confirmation or error message
```

#### Browse History Flow

```
User presses 'H' (history key)
  ↓
App dispatches HistoryBrowserRequest
  ↓
UI Coordinator shows SessionHistoryBrowserOverlay
  ↓
Overlay calls ListSessionHistory RPC
  ↓
Backend: ClaudeHistoryParser.Parse()
  ↓
Read ~/.claude/history.jsonl (streaming for large files)
  ↓
GroupByProject() and build SessionHistorySummary[]
  ↓
Return paginated results
  ↓
Overlay renders grouped list with filters
  ↓
User selects entry and presses Enter
  ↓
Trigger onLaunchSession callback
  ↓
Open session creation wizard with pre-filled data
```

#### Launch from History Flow

```
User selects historical session and presses Enter
  ↓
SessionHistoryBrowserOverlay.onLaunchSession()
  ↓
Extract metadata: path, program, category
  ↓
Validate directory exists (warn if missing)
  ↓
Check for duplicate active session (warn if exists)
  ↓
Open SessionSetupOverlay with pre-filled fields
  ↓
User confirms or modifies settings
  ↓
Create new session via CreateSession RPC
  ↓
Session appears in active sessions list
```

### 2.4 Architecture Decision Records (ADRs)

#### ADR-001: Use Overlay Pattern for UI Components

**Status**: Accepted

**Context**: Need modal UI components for config editing and history browsing. Claude Squad uses BubbleTea TUI framework with established overlay pattern.

**Decision**: Extend existing overlay system (ui/overlay/) with new overlays for config editor, history browser, and conversation viewer.

**Consequences**:
- **Positive**: Consistent UX with existing modals, reuse of BaseOverlay infrastructure
- **Positive**: Centralized key management through cmd.Bridge
- **Negative**: TUI-only initially (web UI components separate)
- **Mitigation**: Build web UI components in parallel

**Alternatives Considered**:
- Separate views instead of overlays (rejected: breaks modal UX pattern)
- Single mega-overlay (rejected: violates single responsibility)

#### ADR-002: Parse history.jsonl Instead of SQLite for History

**Status**: Accepted

**Context**: Claude stores session history in history.jsonl (append-only JSONL). Conversation details in __store.db (SQLite).

**Decision**: 
- Parse history.jsonl for session list/metadata
- Use __store.db only for detailed conversation view
- Build in-memory index for fast search/filter

**Consequences**:
- **Positive**: Simpler implementation, no SQLite dependency for basic history
- **Positive**: history.jsonl is append-only, easy to parse incrementally
- **Negative**: Large files (100MB+) require streaming/pagination
- **Mitigation**: Implement incremental parsing with pagination

**Alternatives Considered**:
- SQLite for all history (rejected: adds complexity, requires schema knowledge)
- Cache parsed history (accepted for future optimization)

#### ADR-003: Atomic File Writes for Config Updates

**Status**: Accepted

**Context**: Config file corruption would be catastrophic. Need transaction-safe updates.

**Decision**: Use atomic write pattern:
1. Create backup: config.json → config.json.backup
2. Write to temp: config.json.tmp
3. Atomic rename: config.json.tmp → config.json
4. Delete backup on success

**Consequences**:
- **Positive**: Prevents file corruption
- **Positive**: Enables rollback on failure
- **Negative**: Slight performance overhead (negligible for config files)

**Alternatives Considered**:
- Direct write (rejected: risk of corruption)
- Copy-on-write with versioning (over-engineering for this use case)

#### ADR-004: JSON Schema Validation for settings.json

**Status**: Accepted

**Context**: settings.json has specific structure. Invalid JSON breaks Claude Code.

**Decision**: Implement JSON schema validation before saving settings.json:
- Validate JSON syntax
- Validate against schema (if schema available)
- Provide clear error messages with line numbers

**Consequences**:
- **Positive**: Prevents invalid configurations
- **Positive**: Better UX with clear error messages
- **Negative**: Need to maintain schema or infer from examples
- **Mitigation**: Start with basic JSON validation, add schema later

**Alternatives Considered**:
- No validation (rejected: breaks Claude Code)
- Runtime validation in Claude Code (rejected: not our codebase)

#### ADR-005: Virtual Scrolling for Large History Lists

**Status**: Accepted

**Context**: Users may have 10,000+ historical sessions. Rendering all in UI is slow.

**Decision**: Implement virtual scrolling in history browser:
- Render only visible rows (viewport + buffer)
- Calculate positions dynamically
- Pagination for API calls

**Consequences**:
- **Positive**: Handles large datasets efficiently
- **Positive**: Smooth scrolling performance
- **Negative**: More complex rendering logic
- **Mitigation**: Use proven virtual scrolling library (TUI: custom, Web: react-window)

**Alternatives Considered**:
- Full rendering (rejected: poor performance)
- Pagination only (rejected: breaks smooth scrolling UX)

### 2.5 Integration Points

#### Existing Systems

1. **Session Management** (session/instance.go)
   - Launch session from history reuses NewInstance()
   - Preserve session metadata format
   - Integration: SessionController.CreateSession()

2. **Overlay System** (ui/overlay/)
   - New overlays extend BaseOverlay
   - Register with UICoordinator
   - Follow existing key binding patterns

3. **Configuration System** (config/config.go)
   - Extend Config struct if needed
   - Reuse GetConfigDir() for ~/.claude discovery
   - Integration: config.LoadConfig()

4. **gRPC/Connect-RPC** (proto/session/v1/)
   - Add new RPCs to SessionService
   - Follow existing proto patterns
   - Generate Go and TypeScript bindings

#### External Dependencies

1. **Claude Code** (~/.claude/)
   - Read-only access to history.jsonl and __store.db
   - Write access to CLAUDE.md, settings.json
   - No modifications to Claude Code itself

2. **File System**
   - Atomic file operations (os.Rename)
   - File watching (fsnotify) for external changes
   - Backup creation before modifications

---

## 3. Quality & Testing Strategy

### 3.1 Testing Approach (Test Pyramid)

```
              ╱╲
             ╱  ╲
            ╱ E2E╲         - End-to-end: Critical user journeys (5-10 tests)
           ╱      ╲        - Full TUI/Web UI + Backend integration
          ╱────────╲
         ╱          ╲
        ╱ Integration╲     - Integration: Component interactions (20-30 tests)
       ╱              ╲    - RPC handlers, file I/O, overlay orchestration
      ╱────────────────╲
     ╱                  ╲
    ╱       Unit         ╲  - Unit: Business logic, parsing, validation (100+ tests)
   ╱                      ╲ - 80%+ coverage target
  ╱────────────────────────╲
```

### 3.2 Unit Tests

**Backend Components** (Go)

```go
// config/claude_test.go
func TestClaudeConfigManager_GetConfig(t *testing.T)
func TestClaudeConfigManager_UpdateConfig(t *testing.T)
func TestClaudeConfigManager_ValidateJSON(t *testing.T)
func TestClaudeConfigManager_AtomicWrite(t *testing.T)
func TestClaudeConfigManager_CreateBackup(t *testing.T)
func TestClaudeConfigManager_HandleMissingFile(t *testing.T)
func TestClaudeConfigManager_HandleCorruptedFile(t *testing.T)

// session/history_test.go
func TestClaudeHistoryParser_ParseJSONL(t *testing.T)
func TestClaudeHistoryParser_GroupByProject(t *testing.T)
func TestClaudeHistoryParser_FilterByDateRange(t *testing.T)
func TestClaudeHistoryParser_Search(t *testing.T)
func TestClaudeHistoryParser_HandleLargeFiles(t *testing.T)
func TestClaudeHistoryParser_HandleCorruptedEntries(t *testing.T)

// session/conversation_test.go
func TestConversationStore_GetConversationByProject(t *testing.T)
func TestConversationStore_HandleMissingDB(t *testing.T)
func TestConversationStore_HandleInvalidDB(t *testing.T)
```

**UI Components** (TUI - Go)

```go
// ui/overlay/configEditorOverlay_test.go
func TestConfigEditorOverlay_RenderMarkdown(t *testing.T)
func TestConfigEditorOverlay_RenderJSON(t *testing.T)
func TestConfigEditorOverlay_ToggleEditMode(t *testing.T)
func TestConfigEditorOverlay_DetectUnsavedChanges(t *testing.T)
func TestConfigEditorOverlay_ValidateOnSave(t *testing.T)

// ui/overlay/sessionHistoryBrowserOverlay_test.go
func TestSessionHistoryBrowserOverlay_RenderList(t *testing.T)
func TestSessionHistoryBrowserOverlay_FilterByDate(t *testing.T)
func TestSessionHistoryBrowserOverlay_Search(t *testing.T)
func TestSessionHistoryBrowserOverlay_LaunchSession(t *testing.T)
```

**UI Components** (Web - TypeScript/Jest)

```typescript
// ConfigEditor.test.tsx
describe("ConfigEditor", () => {
  test("renders Markdown with syntax highlighting", () => {});
  test("validates JSON on change", () => {});
  test("shows diff view on edit", () => {});
  test("prevents save with validation errors", () => {});
});

// SessionHistoryBrowser.test.tsx
describe("SessionHistoryBrowser", () => {
  test("groups sessions by project", () => {});
  test("filters by date range", () => {});
  test("searches sessions", () => {});
  test("handles empty history gracefully", () => {});
});
```

### 3.3 Integration Tests

```go
// tests/integration/config_integration_test.go
func TestConfigWorkflow_ViewEditSave(t *testing.T) {
    // Full workflow: Open config → Edit → Validate → Save → Reload
}

func TestConfigWorkflow_ConcurrentEdits(t *testing.T) {
    // Test file locking and conflict detection
}

// tests/integration/history_integration_test.go
func TestHistoryWorkflow_BrowseLaunch(t *testing.T) {
    // Full workflow: Browse history → Select → Launch session → Verify
}

func TestHistoryWorkflow_ViewConversation(t *testing.T) {
    // Full workflow: Browse → View details → Search within conversation
}
```

### 3.4 End-to-End Tests (Playwright)

```typescript
// tests/e2e/configEditor.spec.ts
test("user can view and edit Claude config", async ({ page }) => {
  // Navigate to config editor
  // Edit CLAUDE.md
  // Save changes
  // Verify changes persisted
});

test("user receives error for invalid JSON", async ({ page }) => {
  // Open settings.json
  // Enter invalid JSON
  // Attempt save
  // Verify error message displayed
});

// tests/e2e/sessionHistory.spec.ts
test("user can browse and launch from history", async ({ page }) => {
  // Open history browser
  // Search for project
  // Select session
  // Launch new session
  // Verify session created
});
```

### 3.5 Test Data & Fixtures

**Fixtures**

```
tests/fixtures/claude/
├── CLAUDE.md (sample config)
├── settings.json (sample settings)
├── history.jsonl (sample history with 100 entries)
├── history_large.jsonl (10,000 entries for performance testing)
├── history_corrupted.jsonl (invalid JSON for error handling)
└── __store.db (SQLite with sample conversations)
```

**Test Helpers**

```go
// tests/helpers/claude_fixtures.go
func CreateTestClaudeDir(t *testing.T) string
func CreateTestHistory(t *testing.T, entryCount int) string
func CreateTestConfig(t *testing.T, content string) string
func CleanupTestClaudeDir(t *testing.T, dir string)
```

### 3.6 Observability & Monitoring

**Logging Strategy**

```go
// Add structured logging for all config operations
log.InfoLog.Printf("Reading config file: %s", filename)
log.ErrorLog.Printf("Failed to parse history.jsonl: %v", err)
log.DebugLog.Printf("Parsed %d history entries in %v", count, duration)
```

**Metrics** (Future Enhancement)

```go
// Potential metrics to track
configEditorOpens          // Counter
configEditorSaves          // Counter
configValidationFailures   // Counter
historyBrowserOpens        // Counter
historyParsingDuration     // Histogram
sessionLaunchesFromHistory // Counter
```

**Error Tracking**

```go
// Structured error types for better debugging
type ConfigError struct {
    Op       string // "read", "write", "validate"
    Filename string
    Err      error
}

type HistoryParseError struct {
    Line int
    Err  error
}
```

---

## 4. Implementation Roadmap

### 4.1 Epic Breakdown

**Epic → Features → Stories → Tasks**

```
EPIC-001: Claude Config Editor & Session History Viewer
│
├── FEATURE-001: Configuration File Management
│   ├── STORY-001: View Claude Configuration Files
│   ├── STORY-002: Edit Configuration Files
│   └── STORY-003: Validate and Save Configurations
│
├── FEATURE-002: Session History Browser
│   ├── STORY-004: Parse and Display Session History
│   ├── STORY-005: Filter and Search History
│   └── STORY-006: Launch Session from History
│
└── FEATURE-003: Conversation History Viewer
    ├── STORY-007: Display Conversation Details
    └── STORY-008: Search Within Conversations
```

### 4.2 Implementation Phases

#### Phase 1: Foundation (Week 1)

**Goal**: Establish backend infrastructure for config and history management

**Stories**:

1. **STORY-001: Backend Config Management**
   - Create config/claude.go with ClaudeConfigManager
   - Implement GetConfig() and ListConfigs()
   - Add atomic file write support
   - Add backup/rollback mechanism
   - **Acceptance**: Can read CLAUDE.md and settings.json programmatically
   - **Effort**: 2 days

2. **STORY-002: History Parser Implementation**
   - Create session/history.go with ClaudeHistoryParser
   - Implement JSONL parsing with streaming
   - Build SessionHistoryIndex for search/filter
   - Add grouping by project
   - **Acceptance**: Can parse 10,000 entry history in < 500ms
   - **Effort**: 2 days

3. **STORY-003: Protocol Buffer Definitions**
   - Add new RPCs to proto/session/v1/session.proto
   - Define ConfigFile, SessionHistorySummary messages
   - Generate Go and TypeScript bindings
   - **Acceptance**: Proto definitions compile and generate code
   - **Effort**: 1 day

**Atomic Tasks for STORY-001**:

```
TASK-001.1: Create config/claude.go file structure [1h]
  Files: config/claude.go
  Dependencies: None
  Success: File compiles, exports ClaudeConfigManager struct
  
TASK-001.2: Implement GetConfig() method [2h]
  Files: config/claude.go, config/claude_test.go
  Dependencies: TASK-001.1
  Success: Can read CLAUDE.md, test coverage >80%
  
TASK-001.3: Add JSON validation support [2h]
  Files: config/claude.go, config/claude_test.go
  Dependencies: TASK-001.2
  Success: Validates settings.json, rejects invalid JSON
  
TASK-001.4: Implement atomic write with backup [3h]
  Files: config/claude.go, config/claude_test.go
  Dependencies: TASK-001.3
  Success: Writes safely, creates backups, test coverage >80%
```

**Known Issues Phase 1**:

```
ISSUE-001: Large history.jsonl files (100MB+) may cause memory issues
Severity: Medium
Mitigation: Implement streaming parser with pagination
Prevention: Memory profiling during testing, benchmark with 100MB file

ISSUE-002: Corrupted JSONL entries will break parsing
Severity: Medium
Mitigation: Skip corrupted lines, log errors, continue parsing
Prevention: Add error handling tests with corrupted fixtures

ISSUE-003: Concurrent config edits may cause conflicts
Severity: Low
Mitigation: File locking, conflict detection, user notification
Prevention: Integration test with concurrent edits
```

#### Phase 2: TUI Implementation (Week 2)

**Goal**: Build overlay components for config editing and history browsing

**Stories**:

4. **STORY-004: Config Editor Overlay**
   - Create ui/overlay/configEditorOverlay.go
   - Implement syntax highlighting for Markdown/JSON
   - Add edit mode with validation
   - Wire up save/cancel actions
   - **Acceptance**: Can view and edit CLAUDE.md in TUI
   - **Effort**: 3 days

5. **STORY-005: History Browser Overlay**
   - Create ui/overlay/sessionHistoryBrowserOverlay.go
   - Implement list rendering with grouping
   - Add date range filters
   - Add search functionality
   - **Acceptance**: Can browse and filter session history
   - **Effort**: 2 days

**Atomic Tasks for STORY-004**:

```
TASK-004.1: Create configEditorOverlay.go structure [1h]
  Files: ui/overlay/configEditorOverlay.go
  Dependencies: Phase 1 complete
  Success: Overlay compiles, extends BaseOverlay
  
TASK-004.2: Implement read-only view mode [3h]
  Files: ui/overlay/configEditorOverlay.go
  Dependencies: TASK-004.1
  Success: Displays CLAUDE.md with navigation
  
TASK-004.3: Add syntax highlighting [4h]
  Files: ui/overlay/configEditorOverlay.go, ui/syntax/highlighter.go
  Dependencies: TASK-004.2
  Success: Markdown and JSON syntax highlighted
  
TASK-004.4: Implement edit mode [4h]
  Files: ui/overlay/configEditorOverlay.go
  Dependencies: TASK-004.3
  Success: Can edit text, detect unsaved changes
  
TASK-004.5: Wire up save action with validation [3h]
  Files: ui/overlay/configEditorOverlay.go
  Dependencies: TASK-004.4
  Success: Saves to file, validates JSON, shows errors
  
TASK-004.6: Register overlay with UI coordinator [2h]
  Files: app/app.go, keys/keys.go
  Dependencies: TASK-004.5
  Success: Overlay opens with 'C' key, registered with coordinator
```

**Known Issues Phase 2**:

```
ISSUE-004: Syntax highlighting performance for large files (>10MB)
Severity: Medium
Mitigation: Lazy highlighting (visible lines only), chunking
Prevention: Performance benchmark with 10MB file

ISSUE-005: Terminal resize during editing may corrupt display
Severity: Low
Mitigation: Re-render on resize event, preserve cursor position
Prevention: Manual testing with terminal resize

ISSUE-006: Unsaved changes not visible in overlay title/indicator
Severity: Low
Mitigation: Add asterisk (*) to title when changes detected
Prevention: UX review, ensure clear feedback
```

#### Phase 3: Web UI Implementation (Week 3)

**Goal**: Build React components for web interface

**Stories**:

6. **STORY-006: Config Editor Web Component**
   - Create web-app/src/components/config/ConfigEditor.tsx
   - Integrate Monaco Editor or CodeMirror
   - Add validation UI
   - Wire up gRPC calls
   - **Acceptance**: Can edit configs in web UI
   - **Effort**: 3 days

7. **STORY-007: History Browser Web Component**
   - Create web-app/src/components/sessions/SessionHistoryBrowser.tsx
   - Implement virtual scrolling
   - Add filters and search
   - Wire up gRPC calls
   - **Acceptance**: Can browse history in web UI
   - **Effort**: 2 days

**Atomic Tasks for STORY-006**:

```
TASK-006.1: Create ConfigEditor.tsx structure [1h]
  Files: web-app/src/components/config/ConfigEditor.tsx
  Dependencies: Phase 1 complete
  Success: Component renders, accepts props
  
TASK-006.2: Integrate Monaco Editor [4h]
  Files: web-app/src/components/config/ConfigEditor.tsx
  Dependencies: TASK-006.1, npm install @monaco-editor/react
  Success: Code editor renders with syntax highlighting
  
TASK-006.3: Add validation UI [3h]
  Files: web-app/src/components/config/ConfigEditor.tsx
  Dependencies: TASK-006.2
  Success: Shows validation errors inline
  
TASK-006.4: Wire up GetClaudeConfig RPC [2h]
  Files: web-app/src/components/config/ConfigEditor.tsx
  Dependencies: TASK-006.3
  Success: Loads config from backend
  
TASK-006.5: Wire up UpdateClaudeConfig RPC [2h]
  Files: web-app/src/components/config/ConfigEditor.tsx
  Dependencies: TASK-006.4
  Success: Saves config to backend
  
TASK-006.6: Add diff view [3h]
  Files: web-app/src/components/config/ConfigEditor.tsx
  Dependencies: TASK-006.5
  Success: Shows side-by-side diff of changes
```

**Known Issues Phase 3**:

```
ISSUE-007: Monaco Editor bundle size increases app load time
Severity: Medium
Mitigation: Lazy load Monaco Editor, code splitting
Prevention: Lighthouse performance audit, measure bundle size

ISSUE-008: WebSocket connection dropped during config edit loses changes
Severity: High
Mitigation: Save draft to localStorage, prompt on connection drop
Prevention: Integration test with network interruption

ISSUE-009: Large conversation history causes browser memory issues
Severity: Medium
Mitigation: Virtual scrolling, pagination, lazy loading
Prevention: Test with 1000+ message conversation
```

#### Phase 4: Integration & Polish (Week 4)

**Goal**: Complete integration, testing, and user experience polish

**Stories**:

8. **STORY-008: Launch Session from History**
   - Implement session creation from history metadata
   - Add pre-fill logic to SessionSetupOverlay
   - Add duplicate detection
   - **Acceptance**: Can create session from history
   - **Effort**: 2 days

9. **STORY-009: Conversation Viewer**
   - Create conversation detail overlay (TUI + Web)
   - Implement SQLite query for conversations
   - Add search within conversations
   - **Acceptance**: Can view conversation details
   - **Effort**: 2 days

10. **STORY-010: Integration Testing & Bug Fixes**
    - Write integration tests for all workflows
    - Fix bugs discovered during testing
    - Performance optimization
    - **Acceptance**: All tests passing, no critical bugs
    - **Effort**: 3 days

**Atomic Tasks for STORY-008**:

```
TASK-008.1: Add session template creation from history [3h]
  Files: session/instance.go, session/history.go
  Dependencies: Phase 2 complete
  Success: Can create InstanceOptions from SessionHistorySummary
  
TASK-008.2: Pre-fill SessionSetupOverlay with history data [2h]
  Files: ui/overlay/sessionSetup.go
  Dependencies: TASK-008.1
  Success: Overlay opens with fields pre-filled
  
TASK-008.3: Add duplicate session detection [2h]
  Files: session/storage.go
  Dependencies: TASK-008.2
  Success: Warns if session with same path exists
  
TASK-008.4: Handle missing directories gracefully [2h]
  Files: ui/overlay/sessionSetup.go
  Dependencies: TASK-008.3
  Success: Warns if directory from history doesn't exist
```

**Known Issues Phase 4**:

```
ISSUE-010: Launching from history with missing directory fails silently
Severity: High
Mitigation: Validate directory existence, show clear error/warning
Prevention: Integration test with non-existent directory path

ISSUE-011: SQLite __store.db may have schema changes across Claude versions
Severity: Medium
Mitigation: Schema detection, version compatibility checks
Prevention: Test with multiple Claude Code versions

ISSUE-012: History entries may reference moved/deleted projects
Severity: Low
Mitigation: Show path in warning, allow manual path correction
Prevention: UX design for path resolution flow
```

### 4.3 Dependency Graph

```
Phase 1 (Foundation)
├── STORY-001 (Backend Config)
├── STORY-002 (History Parser)  
└── STORY-003 (Proto Definitions)
     ↓
Phase 2 (TUI)
├── STORY-004 (Config Editor Overlay) [depends: STORY-001, STORY-003]
└── STORY-005 (History Browser Overlay) [depends: STORY-002, STORY-003]
     ↓
Phase 3 (Web UI)
├── STORY-006 (Config Editor Web) [depends: STORY-001, STORY-003]
└── STORY-007 (History Browser Web) [depends: STORY-002, STORY-003]
     ↓
Phase 4 (Integration)
├── STORY-008 (Launch from History) [depends: STORY-005, STORY-007]
├── STORY-009 (Conversation Viewer) [depends: STORY-002]
└── STORY-010 (Testing & Polish) [depends: ALL]
```

### 4.4 Milestones

**Milestone 1: Backend Complete** (End of Week 1)
- Can read/write Claude configs programmatically
- Can parse history.jsonl
- Proto definitions generated
- Unit tests passing (>80% coverage)

**Milestone 2: TUI MVP** (End of Week 2)
- Config editor overlay functional
- History browser overlay functional
- Can view configs in TUI
- Can browse history in TUI

**Milestone 3: Web UI Complete** (End of Week 3)
- Config editor web component functional
- History browser web component functional
- Feature parity with TUI

**Milestone 4: Feature Complete** (End of Week 4)
- All user stories implemented
- Integration tests passing
- Documentation complete
- Ready for release

---

## 5. Documentation & Artifacts

### 5.1 Code Documentation

**GoDoc Comments**

```go
// ClaudeConfigManager handles reading and writing Claude Code configuration files
// located in the ~/.claude directory. It provides thread-safe access to configs
// with atomic write guarantees and automatic backup creation.
//
// Example usage:
//   manager := config.NewClaudeConfigManager()
//   configFile, err := manager.GetConfig("CLAUDE.md")
//   if err != nil {
//       return err
//   }
//   fmt.Println(configFile.Content)
//
// Thread Safety: All methods are safe for concurrent use.
type ClaudeConfigManager struct {
    claudeDir string
    mu        sync.RWMutex
}
```

**API Documentation** (Proto)

```protobuf
// GetClaudeConfig retrieves a Claude configuration file by name.
//
// Supported files:
// - CLAUDE.md: Main configuration markdown file
// - settings.json: Claude Code settings
// - agents.md: Agent configuration markdown
//
// Returns:
// - ConfigFile: The requested configuration file with content and metadata
// - Error: File not found, permission denied, or read error
rpc GetClaudeConfig(GetClaudeConfigRequest) returns (GetClaudeConfigResponse) {}
```

### 5.2 User Documentation

**CLAUDE.md Update**

```markdown
## Configuration Management

Claude Squad provides built-in config management for Claude Code settings.

### Viewing Configuration

Press `C` to open the config viewer. Use `Tab` to switch between:
- CLAUDE.md (instructions)
- settings.json (Claude settings)
- agents.md (agent configurations)

### Editing Configuration

1. Press `C` to open config viewer
2. Press `e` to enable edit mode
3. Make your changes
4. Press `Ctrl+S` to save (validation automatic)
5. Press `Esc` to exit

Changes are validated before saving. Invalid JSON will show error messages.

## Session History

Browse and launch sessions from your Claude Code history.

### Browse History

Press `H` to open the session history browser. Sessions are grouped by project.

**Filters**:
- `f` + `7`: Last 7 days
- `f` + `3`: Last 30 days
- `f` + `9`: Last 90 days
- `f` + `a`: All time

**Search**:
- `/` to search by project path
- Type query and press Enter

### Launch from History

1. Select a historical session (↑/↓ or j/k)
2. Press Enter to launch
3. Confirm or modify settings
4. New session created with same context

### View Conversation

1. Select a historical session
2. Press `v` to view conversation details
3. Use `/` to search within conversation
4. Press `Esc` to close
```

### 5.3 Architecture Diagrams

**Component Diagram** (C4 Model - Container Level)

```
┌─────────────────────────────────────────────────────────────┐
│                   Claude Squad Application                   │
│                                                              │
│  ┌────────────────┐         ┌────────────────┐             │
│  │  TUI Frontend  │         │  Web Frontend  │             │
│  │  (BubbleTea)   │         │   (Next.js)    │             │
│  └────────┬───────┘         └────────┬───────┘             │
│           │                          │                      │
│           └──────────┬───────────────┘                      │
│                      ↓                                      │
│           ┌──────────────────────┐                         │
│           │  gRPC/Connect-RPC    │                         │
│           │   SessionService     │                         │
│           └──────────┬───────────┘                         │
│                      ↓                                      │
│  ┌───────────────────────────────────────────────┐         │
│  │          Backend Services                     │         │
│  │  ┌─────────────────┐  ┌──────────────────┐   │         │
│  │  │ ClaudeConfig    │  │ HistoryParser    │   │         │
│  │  │ Manager         │  │                  │   │         │
│  │  └─────────────────┘  └──────────────────┘   │         │
│  └───────────────────────────────────────────────┘         │
└─────────────────────────────────────────────────────────────┘
                      ↓
┌─────────────────────────────────────────────────────────────┐
│              File System (Claude Config Dir)                 │
│  ~/.claude/                                                  │
│  ├── CLAUDE.md                                              │
│  ├── settings.json                                          │
│  ├── history.jsonl                                          │
│  └── __store.db                                             │
└─────────────────────────────────────────────────────────────┘
```

**Sequence Diagram: Edit Config Flow**

```
User         ConfigEditor     UpdateConfig    ConfigManager    FileSystem
 │               Overlay           RPC                          
 │────press 'C'───→│                                            
 │                 │────GetClaudeConfig────→│                   
 │                 │                         │────read file────→│
 │                 │                         │←───content───────│
 │                 │←─────ConfigFile─────────│                   
 │                 │                                            
 │←────render──────│                                            
 │                                                              
 │──press 'e'─────→│                                            
 │                 │────enable edit mode────→                   
 │                                                              
 │───edit text────→│                                            
 │                 │────validate on change──→                   
 │                                                              
 │──press Ctrl+S──→│                                            
 │                 │────UpdateClaudeConfig──→│                   
 │                 │                         │─validate content─→
 │                 │                         │─create backup───→│
 │                 │                         │←───backup ok─────│
 │                 │                         │─write temp file─→│
 │                 │                         │─atomic rename───→│
 │                 │                         │←───success───────│
 │                 │←───success message──────│                   
 │←─show success───│                                            
```

### 5.4 Testing Documentation

**Test Plan Document** (tests/TEST_PLAN.md)

```markdown
# Claude Config Editor Test Plan

## Scope

This test plan covers the Claude Config Editor & Session History Viewer feature.

## Test Categories

### Unit Tests
- Backend: config/claude_test.go, session/history_test.go
- UI: ui/overlay/configEditorOverlay_test.go
- Coverage Target: >80%

### Integration Tests
- Config workflow: View → Edit → Save → Reload
- History workflow: Browse → Filter → Launch → Verify
- Concurrent edits with conflict detection

### E2E Tests
- Full user journeys in TUI and Web UI
- Cross-platform testing (macOS, Linux)

## Test Data

Fixtures located in tests/fixtures/claude/:
- CLAUDE.md (1KB, 10KB, 1MB variants)
- settings.json (valid, invalid, missing fields)
- history.jsonl (100, 1000, 10000 entries)
- __store.db (sample conversations)

## Test Execution

Run all tests:
```bash
make test
```

Run integration tests only:
```bash
go test ./tests/integration/... -v
```

Run E2E tests:
```bash
cd tests/e2e && npm test
```

## Known Issues

See section 6 for comprehensive known issues list.
```

---

## 6. Known Issues & Mitigation

### 6.1 Proactive Bug Identification

#### Concurrency Issues

**BUG-001: Race Condition in Config File Updates**

**Description**: Concurrent edits to the same config file from multiple instances could result in lost updates or file corruption.

**Severity**: High  
**Likelihood**: Low (single user typically)

**Scenario**:
```
Instance A: Read config at T0 (version 1)
Instance B: Read config at T0 (version 1)
Instance A: Write changes at T1 (version 2)
Instance B: Write changes at T2 (version 2, overwrites A's changes)
→ A's changes lost
```

**Mitigation**:
1. Implement file locking (flock) during write operations
2. Add version tracking (modify timestamp check before save)
3. Detect conflicts and prompt user for resolution
4. Advisory locks on ~/.claude directory

**Prevention Strategy**:
```go
// config/claude.go
func (m *ClaudeConfigManager) UpdateConfig(filename, content string) error {
    lockFile := filepath.Join(m.claudeDir, ".config.lock")
    lock := flock.New(lockFile)
    
    if err := lock.Lock(); err != nil {
        return fmt.Errorf("config locked by another instance: %w", err)
    }
    defer lock.Unlock()
    
    // Check if file was modified since we read it
    currentModTime := getFileModTime(filename)
    if currentModTime.After(m.lastReadTime) {
        return ErrConfigModifiedExternally
    }
    
    // Proceed with atomic write
    return m.atomicWrite(filename, content)
}
```

**Testing**:
```go
func TestConfigManager_ConcurrentEdits(t *testing.T) {
    manager := NewClaudeConfigManager()
    
    // Simulate concurrent edits
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(n int) {
            defer wg.Done()
            content := fmt.Sprintf("edit %d", n)
            err := manager.UpdateConfig("test.md", content)
            // One should succeed, others should get lock error
        }(i)
    }
    wg.Wait()
    
    // Verify file integrity and that no edits were lost
}
```

**BUG-002: History Parser Memory Leak with Large Files**

**Description**: Parsing large history.jsonl files (100MB+) into memory could cause OOM errors, especially when multiple users browse history simultaneously.

**Severity**: Medium  
**Likelihood**: Medium (power users with extensive history)

**Scenario**:
```
User has 100MB history.jsonl (50,000+ entries)
Parser loads entire file into memory
Memory usage spikes to 500MB+ (JSON deserialization overhead)
Server OOM if multiple users browse concurrently
```

**Mitigation**:
1. Implement streaming parser (bufio.Scanner)
2. Use pagination for API responses
3. Implement LRU cache for parsed history
4. Add memory limit checks

**Prevention Strategy**:
```go
// session/history.go
func (p *ClaudeHistoryParser) ParseStreaming(offset, limit int) ([]*HistoryEntry, error) {
    file, err := os.Open(p.historyPath)
    if err != nil {
        return nil, err
    }
    defer file.Close()
    
    scanner := bufio.NewScanner(file)
    entries := make([]*HistoryEntry, 0, limit)
    lineCount := 0
    
    for scanner.Scan() {
        if lineCount < offset {
            lineCount++
            continue
        }
        
        if len(entries) >= limit {
            break
        }
        
        var entry HistoryEntry
        if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
            // Log error but continue parsing
            log.WarningLog.Printf("Skipping corrupted history line %d: %v", lineCount, err)
            continue
        }
        
        entries = append(entries, &entry)
        lineCount++
    }
    
    return entries, scanner.Err()
}
```

**Testing**:
```go
func TestHistoryParser_LargeFile(t *testing.T) {
    // Create 100MB test file
    testFile := createLargeHistoryFile(t, 50000)
    defer os.Remove(testFile)
    
    parser := NewClaudeHistoryParser(testFile)
    
    // Measure memory before
    var memBefore runtime.MemStats
    runtime.ReadMemStats(&memBefore)
    
    // Parse with pagination
    entries, err := parser.ParseStreaming(0, 100)
    require.NoError(t, err)
    require.Len(t, entries, 100)
    
    // Measure memory after
    var memAfter runtime.MemStats
    runtime.ReadMemStats(&memAfter)
    
    // Memory increase should be < 10MB for 100 entries
    memIncrease := memAfter.Alloc - memBefore.Alloc
    assert.Less(t, memIncrease, 10*1024*1024)
}
```

#### Data Integrity Issues

**BUG-003: Partial Config Write on Disk Full**

**Description**: If disk is full during config write, atomic rename may fail, leaving system in inconsistent state with neither old nor new config.

**Severity**: High  
**Likelihood**: Low

**Scenario**:
```
1. Write new config to temp file (succeeds, uses last disk space)
2. Attempt atomic rename (fails, no space for filesystem metadata)
3. Temp file exists, original file exists
4. User sees "save failed" but unclear which version is active
```

**Mitigation**:
1. Check available disk space before write
2. Ensure temp file deletion on failure
3. Preserve backup on failure for manual recovery
4. Clear error message with recovery instructions

**Prevention Strategy**:
```go
func (m *ClaudeConfigManager) UpdateConfig(filename, content string) error {
    // Check available disk space (need at least 2x file size + 10MB buffer)
    requiredSpace := int64(len(content)*2 + 10*1024*1024)
    if available, err := getAvailableDiskSpace(m.claudeDir); err == nil {
        if available < requiredSpace {
            return ErrInsufficientDiskSpace{Available: available, Required: requiredSpace}
        }
    }
    
    // Create backup
    backupPath := filename + ".backup"
    if err := copyFile(filename, backupPath); err != nil {
        return fmt.Errorf("failed to create backup: %w", err)
    }
    
    // Write to temp file
    tempPath := filename + ".tmp"
    if err := os.WriteFile(tempPath, []byte(content), 0644); err != nil {
        return fmt.Errorf("failed to write temp file: %w", err)
    }
    
    // Atomic rename
    if err := os.Rename(tempPath, filename); err != nil {
        // Cleanup temp file
        os.Remove(tempPath)
        // Keep backup for manual recovery
        return fmt.Errorf("failed to save config (backup preserved at %s): %w", backupPath, err)
    }
    
    // Success - remove backup
    os.Remove(backupPath)
    return nil
}
```

**BUG-004: Corrupted JSONL Entries Break History Parsing**

**Description**: Malformed JSON entries in history.jsonl (due to crashes, corruption) cause parser to fail, preventing access to all history.

**Severity**: Medium  
**Likelihood**: Medium

**Scenario**:
```
history.jsonl contains 10,000 entries
Entry 5,000 is corrupted (incomplete JSON)
Parser fails at line 5,000, returns error
User loses access to entries 5,001-10,000
```

**Mitigation**:
1. Skip corrupted lines with warning log
2. Track skipped lines and show summary to user
3. Provide "repair history" tool to clean corrupted entries
4. Continue parsing after errors

**Prevention Strategy**:
```go
func (p *ClaudeHistoryParser) ParseStreaming(offset, limit int) (*HistoryResult, error) {
    result := &HistoryResult{
        Entries: make([]*HistoryEntry, 0, limit),
        Warnings: make([]ParseWarning, 0),
    }
    
    // ... scanner setup ...
    
    for scanner.Scan() {
        var entry HistoryEntry
        if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
            // Don't fail - log warning and continue
            warning := ParseWarning{
                Line:    lineCount,
                Message: fmt.Sprintf("Skipped corrupted entry: %v", err),
            }
            result.Warnings = append(result.Warnings, warning)
            log.WarningLog.Printf("History line %d corrupted: %v", lineCount, err)
            continue
        }
        
        result.Entries = append(result.Entries, &entry)
    }
    
    return result, scanner.Err()
}
```

#### Integration Issues

**BUG-005: Claude Code Version Schema Mismatch**

**Description**: Claude Code may change history.jsonl or __store.db schema in future versions, breaking our parser assumptions.

**Severity**: Medium  
**Likelihood**: Medium (Claude Code updates regularly)

**Scenario**:
```
Claude Code 1.5 changes history.jsonl format
Adds new field: "session_type"
Our parser expects old schema
Parser fails or returns incomplete data
```

**Mitigation**:
1. Implement schema version detection
2. Support multiple schema versions
3. Graceful degradation for unknown fields
4. Add "schema upgrade" logic
5. Test with multiple Claude Code versions

**Prevention Strategy**:
```go
// session/history.go

// HistoryEntry with flexible schema support
type HistoryEntry struct {
    Display        string    `json:"display"`
    PastedContents map[string]interface{} `json:"pastedContents"`
    Timestamp      int64     `json:"timestamp"`
    Project        string    `json:"project"`
    
    // Schema version tracking
    SchemaVersion  int       `json:"schema_version,omitempty"` // New in v2
    
    // Unknown fields stored for future compatibility
    Extra          map[string]interface{} `json:"-"`
}

func (p *ClaudeHistoryParser) ParseEntry(line []byte) (*HistoryEntry, error) {
    var entry HistoryEntry
    var raw map[string]interface{}
    
    // Parse into raw map first
    if err := json.Unmarshal(line, &raw); err != nil {
        return nil, err
    }
    
    // Detect schema version
    schemaVersion := detectSchemaVersion(raw)
    
    // Parse according to schema
    switch schemaVersion {
    case 1:
        return p.parseSchemaV1(raw)
    case 2:
        return p.parseSchemaV2(raw)
    default:
        // Unknown schema - best effort parse
        log.WarningLog.Printf("Unknown history schema version: %d", schemaVersion)
        return p.parseBestEffort(raw)
    }
}
```

**Testing**:
```go
func TestHistoryParser_MultipleSchemaVersions(t *testing.T) {
    tests := []struct {
        name     string
        jsonLine string
        expected *HistoryEntry
    }{
        {
            name: "schema v1",
            jsonLine: `{"display":"continue","timestamp":1234,"project":"/foo"}`,
            expected: &HistoryEntry{Display: "continue", Timestamp: 1234, Project: "/foo"},
        },
        {
            name: "schema v2 with new field",
            jsonLine: `{"display":"continue","timestamp":1234,"project":"/foo","schema_version":2,"session_type":"code"}`,
            expected: &HistoryEntry{Display: "continue", Timestamp: 1234, Project: "/foo", SchemaVersion: 2},
        },
    }
    
    parser := NewClaudeHistoryParser("")
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            entry, err := parser.ParseEntry([]byte(tt.jsonLine))
            require.NoError(t, err)
            assert.Equal(t, tt.expected.Display, entry.Display)
        })
    }
}
```

#### Performance Issues

**BUG-006: Slow Syntax Highlighting for Large Files**

**Description**: Syntax highlighting large config files (1MB+ CLAUDE.md) may cause UI freezes in TUI.

**Severity**: Low  
**Likelihood**: Low

**Scenario**:
```
User opens 1MB CLAUDE.md file
Syntax highlighter processes entire file
TUI freezes for 2-3 seconds during highlighting
User perceives application as unresponsive
```

**Mitigation**:
1. Lazy highlighting (visible lines only)
2. Chunk processing with progress indicator
3. Async highlighting in background goroutine
4. Cache highlighted output

**Prevention Strategy**:
```go
// ui/syntax/highlighter.go

type LazyHighlighter struct {
    content         string
    highlightedChunks map[int]string // chunk index -> highlighted content
    chunkSize       int               // lines per chunk
    mu              sync.RWMutex
}

func (h *LazyHighlighter) GetHighlightedLines(startLine, endLine int) string {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    startChunk := startLine / h.chunkSize
    endChunk := endLine / h.chunkSize
    
    result := strings.Builder{}
    for chunkIdx := startChunk; chunkIdx <= endChunk; chunkIdx++ {
        if highlighted, ok := h.highlightedChunks[chunkIdx]; ok {
            result.WriteString(highlighted)
        } else {
            // Trigger async highlighting for this chunk
            go h.highlightChunk(chunkIdx)
            // Return unhighlighted for now
            result.WriteString(h.getChunkContent(chunkIdx))
        }
    }
    
    return result.String()
}
```

#### Security Issues

**BUG-007: Path Traversal in Config File Access**

**Description**: Malicious or buggy client could request files outside ~/.claude directory.

**Severity**: High  
**Likelihood**: Low

**Scenario**:
```
Client requests GetClaudeConfig with filename: "../../../etc/passwd"
Backend reads /Users/user/.claude/../../../etc/passwd
Sensitive file contents exposed
```

**Mitigation**:
1. Validate filename against whitelist
2. Use filepath.Clean() and check prefix
3. Never allow ".." in paths
4. Restrict to ~/.claude directory only

**Prevention Strategy**:
```go
func (m *ClaudeConfigManager) GetConfig(filename string) (*ConfigFile, error) {
    // Whitelist of allowed files
    allowedFiles := map[string]bool{
        "CLAUDE.md":      true,
        "settings.json":  true,
        "agents.md":      true,
    }
    
    if !allowedFiles[filename] {
        return nil, ErrInvalidFilename{Filename: filename}
    }
    
    // Clean path and check it's within claudeDir
    cleanPath := filepath.Clean(filepath.Join(m.claudeDir, filename))
    if !strings.HasPrefix(cleanPath, m.claudeDir) {
        return nil, ErrPathTraversal{RequestedPath: filename}
    }
    
    // Safe to read now
    return m.readFile(cleanPath)
}
```

**Testing**:
```go
func TestConfigManager_PathTraversal(t *testing.T) {
    manager := NewClaudeConfigManager()
    
    maliciousPaths := []string{
        "../../../etc/passwd",
        "../../.ssh/id_rsa",
        "./../.ssh/config",
        "../../../../sensitive.txt",
    }
    
    for _, path := range maliciousPaths {
        _, err := manager.GetConfig(path)
        assert.ErrorIs(t, err, ErrPathTraversal)
    }
}
```

### 6.2 Known Limitations

**LIMITATION-001: No Real-Time Collaboration**
- Multiple users cannot edit the same config simultaneously
- Last write wins (no operational transformation)
- Future enhancement: CRDT-based collaborative editing

**LIMITATION-002: Limited History Search**
- Search is substring-only (no fuzzy matching initially)
- No full-text search across conversation contents
- Future enhancement: Elasticsearch integration

**LIMITATION-003: SQLite Dependency for Conversations**
- Requires __store.db to exist for conversation viewing
- Schema knowledge may become outdated
- Future enhancement: Abstract conversation store interface

**LIMITATION-004: No Config Version Control**
- Backups are simple copies (no git-like versioning)
- Cannot revert to specific historical versions
- Future enhancement: Git integration for config history

---

## 7. Appendices

### 7.1 Glossary

- **Claude Code**: Anthropic's AI coding assistant CLI tool
- **Claude Squad**: Session management application (this project)
- **CLAUDE.md**: Main configuration file for Claude Code instructions
- **settings.json**: Claude Code settings and permissions
- **history.jsonl**: Append-only log of Claude Code sessions
- **__store.db**: SQLite database storing conversation history
- **Overlay**: Modal UI component in BubbleTea TUI framework
- **Connect-RPC**: gRPC-compatible protocol used for API
- **PTY**: Pseudo-terminal (tmux integration)
- **Atomic Write**: File write pattern preventing corruption

### 7.2 References

**Standards & Frameworks**:
- IEEE 830: Software Requirements Specification
- ISO/IEC 25010: Software Quality Model
- C4 Model: Software architecture diagrams
- INVEST Criteria: User story quality (Independent, Negotiable, Valuable, Estimable, Small, Testable)

**Design Patterns**:
- Overlay Pattern: Modal UI components
- Atomic Write Pattern: Safe file updates
- Streaming Parser: Memory-efficient parsing
- Virtual Scrolling: Performance optimization

**Testing Methodologies**:
- Test Pyramid: Unit → Integration → E2E
- BDD: Given-When-Then acceptance criteria
- TDD: Test-driven development for critical paths

**External Documentation**:
- BubbleTea Framework: https://github.com/charmbracelet/bubbletea
- Connect-RPC: https://connect.build
- Monaco Editor: https://microsoft.github.io/monaco-editor/

### 7.3 Risk Assessment Matrix

| Risk | Likelihood | Impact | Mitigation | Owner |
|------|-----------|--------|------------|-------|
| Claude Code schema change | Medium | Medium | Schema version detection, multi-version support | Backend Lead |
| Large file performance issues | Medium | Low | Streaming, pagination, virtual scrolling | UI Lead |
| File corruption from crashes | Low | High | Atomic writes, backups, transaction safety | Backend Lead |
| Security vulnerability (path traversal) | Low | High | Path validation, whitelist, security review | Security |
| Browser memory issues (web UI) | Medium | Medium | Virtual scrolling, lazy loading, pagination | Frontend Lead |
| Concurrent edit conflicts | Low | Medium | File locking, version checking | Backend Lead |

### 7.4 Success Metrics

**Feature Adoption**:
- 80% of users open config editor within first week
- 50% of users browse session history within first month
- 30% of sessions launched from history (vs manual creation)

**Performance**:
- Config loading < 100ms (p95)
- History search results < 200ms (p95)
- UI responsiveness > 60 FPS during scroll

**Quality**:
- Zero critical bugs in production (first month)
- < 5 support tickets related to config editor
- Test coverage > 80%

**User Satisfaction**:
- Positive feedback on config editing UX
- Reduced context switching (measured via surveys)
- Feature request: "most wanted" in user polls

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-11-10 | Engineering Team | Initial feature plan |

---

**End of Document**
