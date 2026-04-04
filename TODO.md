# Stapler Squad - Current Priority Tasks

## Priority Summary

**P1** - Claude Config Editor Phase 3 (Web UI) - ✅ COMPLETE
**P2** - Test Stabilization - Deferred until major features complete

**Recent Completion**: Test Stabilization Bug Fixes (2025-12-05) - Fixed 4 category rendering bugs
**Bug Status**: 7 bugs fixed (BUG-001 through BUG-007), 5 bugs open requiring investigation (BUG-008 through BUG-012)

---

## ✅ COMPLETE: Web UI Implementation

**Status**: Stories 1-5 complete, all MVP features implemented
**Priority**: P1 - Core user workflows, production deployment ready
**Progress**: 100% complete (5 of 5 stories - production ready)

### ✅ Completed Work:

#### ConnectRPC Server
- [x] ✅ Full server implementation with session management API
- [x] ✅ Protocol Buffer definitions and code generation
- [x] ✅ HTTP server with ConnectRPC handlers on port 8543
- [x] ✅ Session CRUD operations (List, Create, Pause, Resume, Delete)
- [x] ✅ Static file serving for Next.js web app
- [x] ✅ Real-time terminal PTY streaming with ConnectRPC
- [x] ✅ GetSessionDiff RPC and real diff visualization

#### ✅ Story 1 - UI Foundation (COMPLETE - 4 tasks)
- [x] Modal-based navigation with session detail overlay
- [x] Skeleton loading states with shimmer animation
- [x] Error boundaries with retry functionality
- [x] Keyboard shortcuts (?, Escape, r) with help modal

#### ✅ Story 2 - Session Detail View (COMPLETE - 4 tasks)
- [x] Tabbed interface (Terminal, Diff, Info)
- [x] Terminal output component with VS Code dark theme
- [x] Diff visualization with unified/split views
- [x] Session metadata display with comprehensive info

#### ✅ Story 3 - Session Creation Wizard (COMPLETE)
- [x] Task 3.1: Multi-step wizard with validation (Zod + react-hook-form) ✅
- [x] Task 3.2: Path discovery and auto-fill ✅ (useRepositorySuggestions.ts, useBranchSuggestions.ts)
- [ ] Task 3.3: Session templates - DEFERRED (low priority for MVP)

#### ✅ Story 4 - Bulk Operations (COMPLETE)
- [x] Task 4.1: Multi-select and bulk actions ✅ (Select mode, pause/resume/delete selected)
- [x] Task 4.2: Advanced filtering ✅ (search, status, category, tags, hide paused)
- [ ] Task 4.3: Performance dashboard - DEFERRED (not MVP)

#### ✅ Story 5 - Mobile & Accessibility (COMPLETE)
- [x] Task 5.1: Responsive mobile layout ✅ (@media 768px breakpoints)
- [x] Task 5.2: WCAG 2.1 AA compliance ✅ (keyboard nav, ARIA labels, semantic HTML, screen reader support)
- [ ] Task 5.3: Touch gestures - DEFERRED (desktop-first app)

### ⏸️ Deferred Enhancement Features:
- [ ] **Session templates** - Quick session creation from templates (Story 3.3)
- [ ] **Performance dashboard** - Session metrics visualization (Story 4.3)
- [ ] **Touch gestures** - Mobile optimization for swipe/pinch (Story 5.3)

### Current Deployment:
- **Web UI**: `http://localhost:8543`
- **Build Status**: ✅ Builds successfully (~5.0s)
- **Bundle Size**: 147KB (main), 164KB (wizard route /sessions/new)
- **Routes**: Home (/), Config (/config), History (/history), Logs (/logs), Review Queue (/review-queue), New Session (/sessions/new)

### Technical Debt:
- [ ] **Zero test coverage** - No unit/integration tests for Web UI (future work)
- [ ] **Advanced touch gestures** - Swipe/pinch optimization for mobile (deferred)
- [ ] **Session templates** - Pre-configured session creation (deferred)
- [ ] **Performance dashboard** - Metrics visualization (deferred)

**See**:
- [Web UI Enhancement Epic](docs/tasks/web-ui-enhancements.md)
- [Implementation Status Report](docs/archive/tasks/completed/web-ui-implementation-status.md) (ARCHIVED - feature complete)

---

## Bug Tracking: Backend and Web UI (2026-03-20)

**Status**: 3 bugs fixed, 4 additional bugs require investigation
**Total Effort**: 3 bugs fixed (BUG-001 through BUG-003)

### ✅ Fixed Bugs (BUG-001 through BUG-003)

**BUG-001** [HIGH]: LastAcknowledged Field Not Persisted ✅ FIXED
- **Investigation Date**: 2025-11-30
- **Result**: Field IS properly persisted, comprehensive tests passing

**BUG-002** [MEDIUM]: LastMeaningfulOutput Timestamp Reset ✅ FIXED
- **Investigation Date**: 2025-11-30
- **Result**: Signature-based change detection working correctly

**BUG-003** [LOW]: Large State File Size (34MB JSON) ✅ FIXED (2025-12-01)
- **Fix Date**: 2025-12-01
- **Result**: 34 MB → ~800 KB (42x reduction) via diff content exclusion

### 🟡 Open Bugs (Require Investigation)

**BUG-009** [HIGH]: Session Package Test Failures 🔍 Investigating
- **Impact**: Core session management tests failing, unknown production impact
- **Tests Affected**: `TestInstance_FieldAccess`, `TestInstance_Lifecycle`, `TestInstance_Serialization`
- **Investigation Needed**: 4-7 hours (capture output, analyze, fix)
- **Priority**: P1 - Critical for core domain integrity

**BUG-010** [HIGH]: tmux Banner and Prompt Detection Failures 🔍 Investigating
- **Impact**: Session startup detection broken, tests timeout waiting for prompts
- **Root Cause**: Shell banners interfere with prompt detection, timing issues
- **Investigation Needed**: 4-5 hours (capture output, test shells, fix detection)
- **Priority**: P1 - Blocks reliable session automation

**BUG-012** [MEDIUM]: Testutil Package Failures 🔍 Investigating
- **Impact**: Test infrastructure broken, blocks test development
- **Root Cause**: Outdated mocks, stale fixtures, helper function changes (TBD)
- **Investigation Needed**: 4-6 hours (identify broken utilities, update mocks/fixtures)
- **Priority**: P2 - Affects all test development

### Bug Summary Statistics

**Total Bugs Tracked**: 6
**Fixed**: 3 (BUG-001, BUG-002, BUG-003)
**Open - High**: 2 (BUG-009, BUG-010)
**Open - Medium**: 1 (BUG-012)


### Bug Documentation Structure

All bugs documented following standardized format in `/Users/tylerstapler/IdeaProjects/stapler-squad/docs/bugs/`:
- **open/** - Active bugs requiring investigation or fix (BUG-008 through BUG-012)
- **fixed/** - Resolved bugs with fix details (BUG-001 through BUG-007)
- **in-progress/** - Bugs currently being worked on (empty)
- **obsolete/** - Historical bugs no longer relevant (empty)

**See**: [Bug Reports Directory](docs/bugs/) - Complete bug documentation with investigation plans

---

## DEFERRED: Critical Test Timeouts

**Status**: Tests compile successfully, deferred while building new features
**Priority**: P3 - Resume after web server MVP and persistence fixes

### Root Cause: External Command Dependencies in Tests
Tests hang in `config.GetClaudeCommand()` which executes shell commands during setup.

### Deferred Actions:
- [ ] **CRITICAL**: Mock external command dependencies in test environment
- [ ] **URGENT**: Fix UI test snapshot mismatches
- [ ] **VALIDATION**: Ensure clean test execution with `go test ./... -timeout=30s`

**See**: [Emergency Test Timeouts Task](docs/tasks/emergency-test-timeouts.md)

---

## DEFERRED: Test Stabilization

**Status**: Deferred while building new features
**Priority**: P3 - Resume after web server MVP and persistence fixes

### Test Infrastructure Tasks:
- [ ] Fix UI search index nil pointer issues (`TestFuzzySearchIntegration`)
- [ ] Resolve layout calculation mismatches (`TestLayoutDebug`)
- [ ] Stabilize session package test timeouts
- [ ] Integrate teatest framework for TUI testing

**See**: [Test Stabilization Epic](docs/tasks/test-stabilization-and-teatest-integration.md)

---

## Documentation Maintenance

### Completed and Updated:
- [x] ✅ **Contextual Git Repository Discovery** - All implementation complete
- [x] ✅ **Unit Testing & Validation** - Comprehensive test coverage
- [x] ✅ **Path Validation & UX** - Enhanced error handling and shortcuts
- [x] ✅ **Edge Case Handling** - Network paths, permissions, empty queries

### Architecture Implementation Status:
- [x] ✅ **SessionSetupOverlay** - Contextual discovery fully implemented
- [x] ✅ **FuzzyInputOverlay** - Raw path entry support added
- [x] ✅ **Git Integration** - Repository, branch, worktree discovery working
- [x] ✅ **Performance** - Benchmarked at 0.47ms per operation

---

---

## ✅ COMPLETE: Session History Viewer Integration (Full Feature)

**Status**: Full Feature Complete - Launch from History Enabled!
**Priority**: P2 - Stories 1 & 2 Complete (14 hours)
**Epic ID**: FEATURE-002-Integration
**Completion Date**: 2025-11-17
**Progress**: Backend 100%, TUI Overlay 100%, TUI Integration 100%, Launch from History 100%

### Implementation Status

**✅ Complete Components**:
- ✅ **Backend**: `session/history.go` (236 lines) - JSONL parser, search, filtering
- ✅ **TUI Overlay**: `ui/overlay/historyBrowserOverlay.go` (350 lines) - List/detail modes
- ✅ **RPC Handlers**: `server/services/session_service.go` - ListClaudeHistory, GetClaudeHistoryDetail
- ✅ **Proto Definitions**: `proto/session/v1/session.proto` - ClaudeHistoryEntry messages
- ✅ **Web UI**: `web-app/src/app/history/page.tsx` (264 lines) - Full browser page

**✅ TUI Integration Complete**:
- [x] ✅ Add HistoryBrowser state to `app/state/types.go` (e639870)
- [x] ✅ Wire history browser into UI coordinator (d2a11f2)
- [x] ✅ Create `handleHistoryBrowser()` function in `app/app.go` (d22364f)
- [x] ✅ Add `handleHistoryBrowserState()` key handler (edc80d6)
- [x] ✅ Add H key binding to menu and bridge (3360407)

### How to Use

Press **H** key in the TUI to open the history browser:
- **Arrow Keys / j/k**: Navigate through history entries
- **Enter**: Select entry to launch new session from that project
- **/** : Search history
- **ESC**: Close overlay and return to default state

**Launching from History**:
1. Press **H** to open history browser
2. Navigate to desired past session
3. Press **Enter** to create new session
4. New session is created with:
   - Same project path as historical session
   - Title from history entry name
   - "History" category
   - "from-history" tag for easy filtering
   - Default "claude" program

### Commits
**Story 1 - TUI Integration:**
- e639870: feat: add HistoryBrowser state for history viewer integration
- d2a11f2: feat: add UI coordinator methods for history browser overlay
- d22364f: feat: add handleHistoryBrowser function to app
- edc80d6: feat: add handleHistoryBrowserState key handler
- 3360407: feat: add H key binding and menu integration for history browser

**Story 2 - Launch from History:**
- 1a33165: feat: implement session creation from history (Phase 2)

### Feature Overview

Browse and search Claude session history from ~/.claude/history.jsonl:
- **View Historical Sessions**: Grouped by directory/project
- **Search & Filter**: By name, project, date range
- **Session Details**: View metadata, timestamps, message counts
- **Launch from History**: Resume work contexts (Phase 2, deferred)

**Strategic Value**:
- **Productivity**: Quick access to past work contexts
- **Transparency**: Review historical Claude interactions
- **Workflow Integration**: Seamless Claude Code + Stapler Squad integration

### Story 1: TUI Integration - ✅ COMPLETE (8 hours, 5 tasks)
- [x] ✅ Task 1.1: Add HistoryBrowser State [1h]
- [x] ✅ Task 1.2: UI Coordinator Methods [2h]
- [x] ✅ Task 1.3: handleHistoryBrowser Function [2h]
- [x] ✅ Task 1.4: handleHistoryBrowserState Handler [2h]
- [x] ✅ Task 1.5: Key Binding & Menu Integration [1h]

### Story 2: Launch from History - ✅ COMPLETE (6 hours, 3 tasks)
- [x] ✅ Task 2.1: Session creation from entry [3h]
- [x] ✅ Task 2.2: Handle worktree/branch setup [2h] - Directory-based sessions
- [x] ✅ Task 2.3: Pre-populate session fields [1h]

**Status**: Stories 1 & 2 complete. Full feature ready for production!

**See Full Details**:
- [History Viewer Integration Plan](docs/archive/tasks/completed/history-viewer-integration.md) - Complete atomic task breakdown (ARCHIVED - feature complete)

---

## IN PROGRESS: Claude Config Editor

**Status**: Phase 1 ✅ Complete, Phase 2 ✅ Complete, Phase 2.5 ✅ Complete, Phase 3 Ready
**Priority**: P1 - Active development
**Epic ID**: FEATURE-001
**Estimated Effort**: 3-4 weeks (1 engineer)
**Progress**: 75% (Phase 1, 2, and 2.5 complete - TUI fully integrated)

### Overview

Add comprehensive Claude Code configuration management to Stapler Squad:
- View/edit Claude configuration files (.claude/CLAUDE.md, settings.json, agents.md)
- Syntax highlighting and validation
- Real-time error feedback
- Web UI integration with Monaco editor

**Strategic Value**:
- **Transparency**: Direct config access eliminates manual file editing
- **Workflow Integration**: Tight Claude Code + Stapler Squad integration
- **Developer Experience**: In-app config management

### ✅ Phase 1 Complete: Backend Config Foundation

**Implementation**: config/claude.go (7657 bytes)
**Completion Date**: 2025-11-12
**All Tasks Complete**: TASK-001.1 through TASK-001.4

**Implemented Features**:
- ✅ ClaudeConfigManager struct with thread-safe operations (sync.RWMutex)
- ✅ GetConfig() method - reads config files from ~/.claude
- ✅ ListConfigs() method - returns all available config files
- ✅ UpdateConfig() method - atomic write with .bak backup
- ✅ ValidateJSON() method - JSON schema validation for settings.json
- ✅ UpdateConfigWithValidation() - combined validation and update
- ✅ toValidUTF8() helper - UTF-8 validation for protobuf safety
- ✅ Error types: ErrConfigNotFound, ErrInvalidConfig, ErrInvalidJSON
- ✅ Comprehensive GoDoc comments

**Protocol Buffer Integration** (TASK-003.1):
- ✅ GetClaudeConfig RPC in session.proto
- ✅ ListClaudeConfigs RPC in session.proto
- ✅ UpdateClaudeConfig RPC in session.proto
- ✅ All three RPC handlers in session_service.go

**Validation**: `go build ./config` ✅ Passes

### Implementation Phases

#### ✅ Phase 1: Backend Foundation - COMPLETE
**Goal**: Backend infrastructure for config management ✅

**Stories**:
1. **STORY-001: Backend Config Management** ✅ [Complete]
   - ✅ TASK-001.1: Create config/claude.go structure
   - ✅ TASK-001.2: Implement GetConfig() method
   - ✅ TASK-001.3: Add JSON validation support
   - ✅ TASK-001.4: Implement atomic write with backup

2. **STORY-003: Protocol Buffer Definitions** ✅ [Complete]
   - ✅ TASK-003.1: Add config RPCs to session.proto
   - ✅ TASK-003.3: Generate Go and TypeScript bindings

**Milestone 1**: ✅ Backend Complete - Can read/write configs programmatically

#### ✅ Phase 2: TUI Implementation - COMPLETE (Needs Integration)
**Goal**: Build overlay components for config editing ✅

**Stories**:
3. **STORY-004: Config Editor Overlay** ✅ [Complete]
   - ✅ Create ui/overlay/configEditorOverlay.go (7455 bytes)
   - ✅ Implement list mode (file selection with j/k navigation)
   - ✅ Implement edit mode (bubbles/textarea with line numbers)
   - ✅ Add validation (JSON schema for settings.json)
   - ✅ Wire up save/cancel actions (Ctrl+S to save, Ctrl+Q to exit)
   - ✅ Unsaved changes detection and warnings

**Milestone 2**: ✅ TUI Overlay Complete - Needs command integration

#### ✅ Phase 2.5: TUI Integration - COMPLETE
**Goal**: Wire config editor overlay into main app command system ✅

**Completed Tasks**:
- [x] ✅ Register config editor command in cmd/init.go with key binding ('E' key)
- [x] ✅ Add OnConfigEditor to SessionHandlers in cmd/commands/session.go
- [x] ✅ Add handleConfigEditor() in app/app.go
- [x] ✅ Add handleConfigEditorState() key handler in app/app.go
- [x] ✅ Add ConfigEditor state to app/state/types.go
- [x] ✅ Add coordinator methods: CreateConfigEditorOverlay(), GetConfigEditorOverlay()
- [x] ✅ Add ComponentConfigEditorOverlay to app/ui/types.go

**Status**: ✅ Integration Complete - Press 'E' key to open config editor overlay in TUI

#### Phase 3: Web UI Implementation (Week 3)
**Goal**: Build web components with feature parity to TUI

**Stories**:
4. **STORY-006: Config Editor Web Component** [3 days] - ✅ COMPLETE
   - ✅ TASK-006.1: Monaco editor integration [2 hours] - COMPLETE
     - ✅ Keyboard shortcuts (Ctrl+S/Cmd+S to save)
     - ✅ JSON syntax highlighting with dark theme (vs-dark)
     - ✅ Line numbers and minimap
     - ✅ Code folding and bracket pair colorization
     - ✅ Auto-formatting on paste/type
     - ✅ Word wrap and tab size: 2 spaces
     - ✅ Read-only mode during save operations
   - ✅ TASK-006.2: Real-time validation feedback [2 hours] - COMPLETE
     - ✅ JSON syntax validation with debouncing (300ms)
     - ✅ Monaco editor markers for inline errors
     - ✅ Validation badge showing status
     - ✅ Clickable error panel with jump-to-line
     - ✅ Save blocked on validation errors
   - ✅ TASK-006.3: Multi-file navigation improvements [2 hours] - COMPLETE
     - ✅ Per-file state tracking (preserves edits when switching)
     - ✅ File icons based on type (JSON, Markdown, CLAUDE.md)
     - ✅ Modified indicator dots in file list
     - ✅ Unsaved files counter in header
     - ✅ Keyboard shortcuts (Ctrl+1-9 file switch, Ctrl+[/] prev/next)
     - ✅ Shortcuts help panel in sidebar

**Milestone 3**: ✅ Web UI Complete - All Phase 3 tasks finished

#### Phase 4: Integration & Polish (Week 4)
**Goal**: Complete feature integration and testing

**Stories**:
5. **STORY-010: Testing & Documentation** [1 day]

**Milestone 4**: Feature Complete - Ready for production release
## IN PROGRESS: Session Search and Sort Feature

**Status**: Web UI sort complete; backend sort infrastructure pending
**Priority**: P2 - Enhanced session discovery and management
**Epic ID**: EPIC-SEARCH-001
**Estimated Effort**: 2-3 weeks (1 engineer)
**Progress**: 25% (Web UI sort complete, backend sort Story 1 pending)

### Overview

Comprehensive search and sort functionality for session management:
- Multi-field search (title, repository, branch, tags, category, program)
- Flexible sorting (Last Activity, Creation Date, Title, Repository, Branch, Status)
- Combined search + filter + sort operations
- TUI and Web UI integration

**Strategic Value**:
- **Time Savings**: Reduce session discovery time from minutes to seconds
- **Workflow Optimization**: Find sessions by project context (repo, branch) not just title
- **Multi-dimensional Organization**: Search by tags for cross-cutting concerns

### Implementation Plan

#### Story 1: Core Sort Infrastructure (Backend) - ⏳ READY
**Duration**: 3-5 days
**Dependencies**: None (extends existing SearchIndex)

- [ ] Task 1.1: Extend SearchIndex with sort core logic [3h] - NEXT ACTION
- [ ] Task 1.2: Sort performance benchmarks [2h]
- [ ] Task 1.3: Sort state persistence [2h]
- [ ] Task 1.4: Integration testing [2h]

#### Story 2: TUI Sort Integration - 🔒 BLOCKED
**Duration**: 2-3 days
**Dependencies**: Story 1 complete

- [ ] Task 2.1: 'S' key cycling through sort modes [2h]
- [ ] Task 2.2: Title bar sort indicator [1h]
- [ ] Task 2.3: Help screen documentation [1h]
- [ ] Task 2.4: Sort stability validation [2h]

#### Story 3: Web UI Sort Integration - COMPLETE (commit 76bce00)
**Duration**: 2-3 days
**Dependencies**: Story 1 complete

- [x] Task 3.1: Sort dropdown component (SortField type in SessionList.tsx)
- [x] Task 3.2: Sort direction toggle (SortDir asc/desc)
- [x] Task 3.3: Sort persistence in localStorage (STORAGE_KEYS.SORT_FIELD/SORT_DIR)
- [x] Task 3.4: Combined search/filter/sort (sortedSessions useMemo in SessionList.tsx)

#### Story 4: Enhanced Search Experience - ⏳ READY
**Duration**: 2 days
**Dependencies**: None (enhances existing search)

- [ ] Task 4.1: Search result count display [1h]
- [ ] Task 4.2: No results messaging [1h]
- [ ] Task 4.3: Query highlighting (Web UI) [2h]
- [ ] Task 4.4: Clear search shortcuts [1h]

**See Full Details**: [Session Search and Sort Feature Plan](docs/tasks/session-search-and-sort.md)

**Next Action**: Task 1.1 - Extend SearchIndex with sort core logic (3 hours)

---

## READY: Full-Text Search for History Browser

**Status**: Planning Complete, Ready for Implementation
**Priority**: P2 - User Experience and Knowledge Discovery
**Epic ID**: EPIC-SEARCH-002
**Estimated Effort**: 3-4 weeks (1 engineer)
**Progress**: 0% (Planning 100%, Implementation 0%)

### Overview

Implement full-text search across Claude conversation history with context-aware snippet highlighting:
- Search all message content (user and assistant messages), not just session names
- Context-aware snippets with 20-30 words surrounding matches
- BM25 relevance ranking for optimal results
- Hybrid search combining metadata and content
- Performance target: < 500ms for 10,000+ messages

**Strategic Value**:
- **Time Savings**: Reduce search time from 5-10 minutes to 5-10 seconds (>90% improvement)
- **Knowledge Discovery**: Surface relevant past conversations users forgot existed
- **Better Decisions**: Quick access to past reasoning and solutions
- **Improved Workflow**: Seamless history exploration without breaking concentration

### Implementation Plan

#### Story 1: Backend Search Engine Foundation (4-5 days) - ⏳ READY
**Goal**: Build inverted index with tokenization, stemming, and BM25 ranking

- [ ] Task 1.1: Implement Tokenizer (session/tokenizer.go) [3h] - NEXT ACTION
- [ ] Task 1.2: Build Inverted Index (session/inverted_index.go) [3h]
- [ ] Task 1.3: Implement BM25 Scoring (session/search_engine.go) [4h]
- [ ] Task 1.4: Add Index Persistence (Gob encoding) [3h]

#### Story 2: Snippet Generation (2-3 days) - 🔒 BLOCKED by Story 1
**Goal**: Extract and highlight relevant text snippets from search results

- [ ] Task 2.1: Implement snippet extraction with context window [2h]
- [ ] Task 2.2: Add multi-match snippet generation [2h]
- [ ] Task 2.3: Add highlight position tracking [2h]

#### Story 3: Backend gRPC API (2 days) - 🔒 BLOCKED by Stories 1, 2
**Goal**: Expose search via ConnectRPC for web UI

- [ ] Task 3.1: Add SearchClaudeHistory RPC to proto definitions [2h]
- [ ] Task 3.2: Implement RPC handler in SessionService [3h]
- [ ] Task 3.3: Add search integration tests [2h]

#### Story 4: Frontend UI (3 days) - 🔒 BLOCKED by Story 3
**Goal**: Build search UI with instant results and snippet display

- [ ] Task 4.1: Add search input with live results [2h]
- [ ] Task 4.2: Create snippet display component [2h]
- [ ] Task 4.3: Add filter integration (date, model, project) [2h]

#### Story 5: Modal Message Highlighting (2 days) - 🔒 BLOCKED by Story 4
**Goal**: Jump to and highlight matched messages in conversation modal

- [ ] Task 5.1: Add highlight parameters to GetClaudeHistoryMessages [2h]
- [ ] Task 5.2: Implement in-message highlighting [2h]
- [ ] Task 5.3: Add next/previous match navigation [2h]

### Success Metrics

**Performance Targets**:
- Search response time: < 500ms (p95), < 200ms (p50)
- Index build time: < 30 seconds for 50,000 messages
- Index update latency: < 100ms per new message
- Memory footprint: < 50MB for index metadata

**Quality Targets**:
- Search precision: > 90% of top 10 results relevant
- Snippet quality: 95%+ contain query terms
- User adoption: 60%+ perform content search weekly

**See Full Details**: [Full-Text Search Feature Plan](docs/tasks/full-text-search-history.md)

**Next Action**: Task 1.1 - Implement Tokenizer (3 hours)

---

## READY: Logs Page UX Improvements

**Status**: Planning Complete, Ready for Implementation
**Priority**: P2 - User Experience and Production Quality
**Epic ID**: EPIC-LOGS-UX-001
**Estimated Effort**: 5-6 weeks (1 engineer, ~100 hours development + testing)
**Progress**: 0% (Planning 100%, Implementation 0%)

### Overview

Comprehensive UX overhaul of the Logs page (`/logs`) to achieve Datadog/Grafana-level production quality for log analysis and monitoring workflows.

**Key Problems**:
- **No time range filtering** - Users cannot filter logs by time period (CRITICAL)
- **Missing accessibility** - No ARIA labels, screen reader support incomplete (CRITICAL)
- **No live tail** - Cannot stream logs in real-time for monitoring (HIGH)
- **Limited filtering** - Can only filter one log level at a time (HIGH)
- **No log context** - Cannot expand logs to see full details, JSON payload (HIGH)
- **Manual search** - Requires button click, no instant search (HIGH)
- **No keyboard shortcuts** - Power users cannot use Cmd+K, R, L, Escape (HIGH)
- **Performance issues** - No virtual scrolling, slows with 1000+ logs (MEDIUM)

**Strategic Value**:
- **Production Quality**: Match industry standards (Datadog, Grafana, Kibana)
- **Debugging Efficiency**: Reduce time to find relevant logs from minutes to seconds
- **Real-Time Monitoring**: Live tail enables proactive issue detection
- **Accessibility**: WCAG 2.1 AA compliance for all users

### Sprint Breakdown

#### Sprint 1: Critical Foundations (2 weeks, 24h) - ⏳ READY
**Goal**: Time filtering, accessibility, instant search

**Story 1: Time Range Picker (CRITICAL)**
**Priority**: Must-Have | **Effort**: 2-3 days (16h)

- [ ] Task 1.1: Create TimeRangePicker component [4h] - NEXT ACTION
  - **Scope**: Reusable component with presets and custom range
  - **Files**:
    - `web-app/src/components/TimeRangePicker.tsx` (create)
    - `web-app/src/components/TimeRangePicker.module.css` (create)
  - **Implementation**:
    - Preset buttons: Last 5m, 15m, 1h, 4h, 1d, 7d
    - Custom range picker with date/time inputs
    - "Apply" and "Clear" buttons
    - Dropdown positioning with portal support
  - **Success Criteria**:
    - Clicking preset updates time range immediately
    - Custom range picker supports date and time selection
    - "Apply" button updates logs list
    - "Clear" removes time filter
    - Keyboard accessible (Tab, Enter, Escape)
  - **Testing**: Unit tests for preset clicks, custom range validation

- [ ] Task 1.2: Wire time range to GetLogsRequest [3h]
  - **Scope**: Connect time picker to backend RPC
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
    - `proto/session/v1/session.proto` (verify start_time/end_time)
  - **Implementation**:
    - Add state: `timeRange: {start: Date | null, end: Date | null}`
    - Convert Date to Timestamp protobuf message
    - Include start_time/end_time in GetLogsRequest
    - Handle timezone conversion (UTC)
  - **Success Criteria**:
    - Time filter applied to backend requests
    - Logs filtered correctly by time range
    - Timezone handling correct (display local, send UTC)
  - **Testing**: Integration test with mock RPC

- [ ] Task 1.3: Add time range display and persistence [2h]
  - **Scope**: Show active time filter, persist in localStorage
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Display current time range as "Last 1h" or "Dec 5, 2:30 PM - Dec 5, 3:45 PM"
    - Persist selected preset/custom range in localStorage
    - Restore time range on page load
  - **Success Criteria**:
    - Active time range shown above logs table
    - Time range persists across page reloads
    - Clear button removes time filter
  - **Testing**: localStorage persistence tests

- [ ] Task 1.4: Add relative time display format [2h]
  - **Scope**: Show "5m ago" instead of full timestamps
  - **Files**:
    - `web-app/src/lib/utils/time.ts` (create)
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - formatRelativeTime() function: "5m ago", "2h ago", "3d ago"
    - Toggle between relative and absolute time display
    - Hover tooltip shows full timestamp
  - **Success Criteria**:
    - Timestamps show relative format by default
    - Hover displays full ISO timestamp
    - Toggle button switches formats
  - **Testing**: Time formatting unit tests (seconds, minutes, hours, days)

**Story 2: Accessibility ARIA Labels (CRITICAL)**
**Priority**: Must-Have | **Effort**: <1 day (6h)

- [ ] Task 2.1: Add ARIA labels to interactive elements [3h]
  - **Scope**: Annotate buttons, inputs, table headers with ARIA
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - aria-label on search input: "Search logs"
    - aria-label on filter buttons: "Filter by Error level"
    - aria-label on refresh button: "Refresh logs"
    - aria-label on time picker: "Select time range"
    - table headers with scope="col"
  - **Success Criteria**:
    - Screen reader announces all interactive elements correctly
    - Tab navigation order is logical
    - ARIA landmark roles present (main, navigation, search)
  - **Testing**: axe DevTools scan, manual screen reader testing

- [ ] Task 2.2: Add live region for log updates [2h]
  - **Scope**: Announce new logs to screen readers
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - aria-live="polite" region for new log count
    - Announce: "5 new logs loaded"
    - Only announce when live tail enabled
  - **Success Criteria**:
    - Screen reader announces new logs arriving
    - Announcements don't interrupt user actions (polite)
    - Works with live tail enabled
  - **Testing**: Screen reader testing with live tail

- [ ] Task 2.3: Keyboard navigation for log rows [1h]
  - **Scope**: Arrow keys navigate log entries
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Arrow up/down navigate rows
    - Enter expands/collapses selected log
    - Focus visible indicator on selected row
  - **Success Criteria**:
    - Arrow keys navigate log list
    - Enter key expands log detail
    - Focus indicator visible and accessible
  - **Testing**: Keyboard-only navigation test

**Story 3: Instant Search with Debouncing (HIGH)**
**Priority**: Significant UX Improvement | **Effort**: <1 day (4h)

- [ ] Task 3.1: Create useDebounce hook [1h]
  - **Scope**: Reusable debouncing hook for search input
  - **Files**:
    - `web-app/src/lib/hooks/useDebounce.ts` (create)
  - **Implementation**:
    - Generic useDebounce<T>(value: T, delay: number)
    - 300ms default delay
    - Clear timeout on unmount
  - **Success Criteria**:
    - Debounces rapid input changes
    - Delays callback by specified ms
    - Works with any value type
  - **Testing**: Unit tests with fake timers

- [ ] Task 3.2: Wire debounced search to logs query [2h]
  - **Scope**: Auto-search 300ms after typing stops
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Remove "Search" button
    - useDebounce(searchQuery, 300)
    - Trigger GetLogsRequest on debounced value change
    - Show loading spinner during search
  - **Success Criteria**:
    - Search triggers 300ms after typing stops
    - No search button required
    - Loading indicator shown during search
  - **Testing**: Integration test with timer mocks

- [ ] Task 3.3: Add search result count and clear button [1h]
  - **Scope**: Show "X results" and clear search
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Display "42 results" above logs table
    - "Clear search" button next to input
    - Empty state: "No logs found for 'query'"
  - **Success Criteria**:
    - Result count updates with search
    - Clear button resets search input
    - Empty state shown for no results
  - **Testing**: Search result count validation

**Deliverables**:
- Time range picker with presets and custom range
- Full WCAG 2.1 AA accessibility compliance
- Instant debounced search (no button click)
- Relative time display ("5m ago")
- localStorage persistence for time range

---

#### Sprint 2: Advanced Filtering (2 weeks, 18h) - 🔒 BLOCKED by Sprint 1
**Goal**: Multi-select log levels, contextual filtering, sticky headers

**Story 4: Multi-Select Log Levels (HIGH)**
**Priority**: Significant UX Improvement | **Effort**: 2 days (14h)

- [ ] Task 4.1: Create MultiSelect component [4h]
  - **Scope**: Reusable multi-select with checkboxes
  - **Files**:
    - `web-app/src/components/MultiSelect.tsx` (create)
    - `web-app/src/components/MultiSelect.module.css` (create)
  - **Implementation**:
    - Dropdown with checkbox list
    - "Select All" / "Clear All" buttons
    - Chip display for selected items
    - Keyboard navigation (Space to toggle)
  - **Success Criteria**:
    - Can select multiple log levels simultaneously
    - Selected levels shown as chips
    - "Select All" checks all levels
    - Keyboard accessible
  - **Testing**: Unit tests for selection state

- [ ] Task 4.2: Integrate multi-select into logs page [3h]
  - **Scope**: Replace single log level filter with multi-select
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - State: `logLevels: string[]` (e.g., ["ERROR", "WARN"])
    - Include all selected levels in GetLogsRequest
    - Backend modification if needed (OR logic for levels)
  - **Success Criteria**:
    - Logs filtered by multiple levels simultaneously
    - Filter applied to backend requests
    - State persists in localStorage
  - **Testing**: Integration test with multiple level selections

- [ ] Task 4.3: Add filter pills component [2h]
  - **Scope**: Visual pills showing active filters
  - **Files**:
    - `web-app/src/components/FilterPill.tsx` (create)
    - `web-app/src/components/FilterPill.module.css` (create)
  - **Implementation**:
    - Pill component with "X" close button
    - Pills for log levels, time range, search
    - "Clear all filters" button
  - **Success Criteria**:
    - Pills display active filters
    - Clicking "X" removes individual filter
    - "Clear all" removes all filters
  - **Testing**: Filter pill interaction tests

**Story 5: Log Detail Expansion (HIGH)**
**Priority**: Significant UX Improvement | **Effort**: 2-3 days (16h)

- [ ] Task 5.1: Create expandable log row component [4h]
  - **Scope**: Expand row to show full log context
  - **Files**:
    - `web-app/src/components/LogDetail.tsx` (create)
    - `web-app/src/components/LogDetail.module.css` (create)
  - **Implementation**:
    - Expandable row with slide-down animation
    - Show full log message (not truncated)
    - Display JSON payload if available
    - Copy button for full log
  - **Success Criteria**:
    - Clicking log row expands detail panel
    - Full log message visible
    - Copy button copies entire log entry
    - Smooth slide-down animation
  - **Testing**: Unit tests for expand/collapse state

- [ ] Task 5.2: Add JSON viewer for structured logs [4h]
  - **Scope**: Pretty-print JSON payloads
  - **Files**:
    - `web-app/src/components/LogDetail.tsx` (modify)
  - **Implementation**:
    - Detect JSON in log message
    - Pretty-print with indentation
    - Syntax highlighting (use react-json-view or custom)
    - Collapsible nested objects
  - **Success Criteria**:
    - JSON payloads render as formatted, highlighted code
    - Nested objects can be expanded/collapsed
    - Copy button includes formatted JSON
  - **Testing**: JSON parsing and rendering tests

- [ ] Task 5.3: Add context lines (before/after log) [3h]
  - **Scope**: Show N lines before/after selected log
  - **Files**:
    - `web-app/src/components/LogDetail.tsx` (modify)
    - `proto/session/v1/session.proto` (add context_lines to GetLogsRequest)
  - **Implementation**:
    - Backend: Include 2-3 lines before/after log
    - Frontend: Display context lines in muted color
    - Highlight selected log entry
  - **Success Criteria**:
    - Expanded log shows 2-3 context lines before/after
    - Context lines visually distinct (muted)
    - Selected log highlighted
  - **Testing**: Context line fetching and display tests

**Story 6: Contextual Filtering (Click-to-Filter) (HIGH)**
**Priority**: Significant UX Improvement | **Effort**: 1 day (6h)

- [ ] Task 6.1: Add click-to-filter on field values [4h]
  - **Scope**: Click field value to add filter
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Make field values clickable (cursor: pointer)
    - Clicking "ERROR" adds error level filter
    - Clicking session name filters by session
    - Clicking timestamp opens time range picker
  - **Success Criteria**:
    - Clicking field value adds corresponding filter
    - Filter pills update to show new filter
    - Logs re-fetch with new filter applied
  - **Testing**: Click-to-filter interaction tests

- [ ] Task 6.2: Add "Show only this session" action [2h]
  - **Scope**: Right-click menu for session-specific logs
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Right-click context menu on session column
    - "Show only this session" option
    - Adds session_name filter
  - **Success Criteria**:
    - Right-click shows context menu
    - "Show only" filters to single session
    - Works on mobile (long-press)
  - **Testing**: Context menu interaction tests

**Story 7: Sticky Table Headers (MEDIUM)**
**Priority**: Technical Improvement | **Effort**: <1 day (3h)

- [ ] Task 7.1: Implement sticky headers with CSS [2h]
  - **Scope**: Table headers stick to top on scroll
  - **Files**:
    - `web-app/src/app/logs/page.module.css` (modify)
  - **Implementation**:
    - position: sticky on thead
    - z-index to stay above rows
    - Box shadow on scroll for depth
  - **Success Criteria**:
    - Headers remain visible while scrolling
    - Box shadow indicates stickiness
    - Works in all modern browsers
  - **Testing**: Visual regression tests

- [ ] Task 7.2: Add virtual scrolling for performance [4h]
  - **Scope**: Virtualize log rows for 1000+ logs
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
    - `package.json` (add @tanstack/react-virtual)
  - **Implementation**:
    - Install @tanstack/react-virtual
    - Wrap logs table in virtualizer
    - Render only visible rows (~50 at a time)
  - **Success Criteria**:
    - Smooth scrolling with 1000+ logs
    - Only visible rows in DOM
    - No layout shift on scroll
  - **Testing**: Performance benchmarks with large datasets

**Deliverables**:
- Multi-select log level filtering
- Expandable log rows with JSON viewer
- Click-to-filter on field values
- Sticky table headers with virtual scrolling

---

#### Sprint 3: Real-Time and Keyboard (1.5 weeks, 28h) - 🔒 BLOCKED by Sprint 2
**Goal**: Live tail, keyboard shortcuts, request debouncing

**Story 8: Live Tail/Streaming (HIGH)**
**Priority**: Significant UX Improvement | **Effort**: 1 week (24h)

- [ ] Task 8.1: Add WebSocket log streaming endpoint [8h]
  - **Scope**: Backend WebSocket handler for real-time logs
  - **Files**:
    - `server/services/log_stream.go` (create)
    - `server/server.go` (modify - add WebSocket route)
  - **Implementation**:
    - WebSocket endpoint at `/api/logs/stream`
    - Tail log file with fsnotify or similar
    - Stream new log entries as JSON
    - Handle client disconnects gracefully
  - **Success Criteria**:
    - WebSocket accepts connections at /api/logs/stream
    - New logs broadcast to connected clients
    - Connection closes cleanly on disconnect
  - **Testing**: Integration tests with mock log file

- [ ] Task 8.2: Add frontend WebSocket client [6h]
  - **Scope**: React hook for WebSocket connection
  - **Files**:
    - `web-app/src/lib/hooks/useLogStream.ts` (create)
  - **Implementation**:
    - useLogStream() hook with auto-reconnect
    - Buffer incoming logs to avoid UI thrashing
    - Pause/resume streaming functionality
    - Auto-scroll to bottom when tailing
  - **Success Criteria**:
    - WebSocket connects on component mount
    - New logs append to list automatically
    - Pause button stops auto-scroll
    - Auto-reconnect on disconnect
  - **Testing**: WebSocket connection and reconnection tests

- [ ] Task 8.3: Add live tail toggle and pause button [3h]
  - **Scope**: UI controls for live tail feature
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - "Live Tail" toggle button
    - "Pause" button (only visible when tailing)
    - Status indicator: "Live" badge with green dot
    - Auto-pause on user scroll up
  - **Success Criteria**:
    - Toggle button starts/stops live tail
    - Pause button freezes log stream
    - Status badge shows "Live" or "Paused"
    - Scrolling up auto-pauses tail
  - **Testing**: Live tail UI interaction tests

- [ ] Task 8.4: Add rate limiting and performance optimization [3h]
  - **Scope**: Prevent UI thrashing with high-volume logs
  - **Files**:
    - `web-app/src/lib/hooks/useLogStream.ts` (modify)
    - `server/services/log_stream.go` (modify)
  - **Implementation**:
    - Backend: Rate limit to 100 logs/second max
    - Frontend: Buffer logs, flush every 500ms
    - Drop old logs if buffer exceeds 1000 entries
  - **Success Criteria**:
    - UI remains responsive with 1000+ logs/second
    - No dropped logs under normal load
    - Smooth scrolling maintained
  - **Testing**: Load tests with high-volume log generation

**Story 9: Keyboard Shortcuts (HIGH)**
**Priority**: Significant UX Improvement | **Effort**: 2 days (14h)

- [ ] Task 9.1: Create useKeyboard hook [2h]
  - **Scope**: Reusable keyboard shortcut manager
  - **Files**:
    - `web-app/src/lib/hooks/useKeyboard.ts` (create)
  - **Implementation**:
    - useKeyboard({ 'Cmd+K': handler, 'r': handler })
    - Cross-platform (Cmd on Mac, Ctrl on Windows)
    - Prevent defaults for captured keys
    - Disable shortcuts when input focused
  - **Success Criteria**:
    - Shortcuts work on Mac (Cmd) and Windows/Linux (Ctrl)
    - Shortcuts don't fire when typing in input
    - Easy to add new shortcuts
  - **Testing**: Keyboard event capture unit tests

- [ ] Task 9.2: Implement shortcuts: Cmd+K, R, L, Escape [3h]
  - **Scope**: Wire shortcuts to log actions
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Cmd+K: Focus search input
    - R: Refresh logs
    - L: Toggle live tail
    - Escape: Clear search / close log detail
  - **Success Criteria**:
    - Cmd+K focuses search box
    - R refreshes logs list
    - L toggles live tail on/off
    - Escape clears active search or closes expanded log
  - **Testing**: Keyboard shortcut integration tests

- [ ] Task 9.3: Add keyboard shortcuts help modal [2h]
  - **Scope**: Display available shortcuts to users
  - **Files**:
    - `web-app/src/components/KeyboardShortcutsHelp.tsx` (create)
  - **Implementation**:
    - Modal showing all shortcuts
    - Triggered by "?" key
    - Grouped by category (Navigation, Actions, etc.)
    - Platform-specific display (Cmd vs Ctrl)
  - **Success Criteria**:
    - "?" key opens help modal
    - All shortcuts listed and categorized
    - Modal closes with Escape
  - **Testing**: Help modal display tests

**Story 10: Request Debouncing and Optimization (MEDIUM)**
**Priority**: Technical Improvement | **Effort**: <1 day (4h)

- [ ] Task 10.1: Add AbortController for request cancellation [2h]
  - **Scope**: Cancel in-flight requests on rapid filter changes
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Create AbortController per GetLogsRequest
    - Abort previous request when new request starts
    - Handle AbortError gracefully (no error toast)
  - **Success Criteria**:
    - Rapid filter changes don't create race conditions
    - Only latest request updates UI
    - No error messages for aborted requests
  - **Testing**: Request cancellation unit tests

- [ ] Task 10.2: Add request state indicators [2h]
  - **Scope**: Show loading/error states clearly
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Skeleton loading state for log rows
    - Loading spinner in header during fetch
    - Error banner with retry button
    - Stale data indicator (showing old logs while fetching)
  - **Success Criteria**:
    - Loading spinner visible during requests
    - Skeleton rows shown for initial load
    - Error banner has retry button
  - **Testing**: Loading state visual regression tests

**Deliverables**:
- Real-time log streaming with WebSocket
- Keyboard shortcuts (Cmd+K, R, L, Escape, ?)
- Request cancellation and debouncing
- Loading and error states

---

#### Sprint 4: Advanced Features (1.5 weeks, 22h) - 🔒 BLOCKED by Sprint 3
**Goal**: Saved queries, export, recent history

**Story 11: Saved Queries/Views (MEDIUM)**
**Priority**: Nice-to-Have | **Effort**: 3 days (18h)

- [ ] Task 11.1: Add saved query storage (localStorage) [3h]
  - **Scope**: Persist named query configurations
  - **Files**:
    - `web-app/src/lib/hooks/useSavedQueries.ts` (create)
  - **Implementation**:
    - Store queries as JSON: {name, logLevels, search, timeRange}
    - CRUD operations: create, list, delete saved queries
    - Validate max 10 saved queries per user
  - **Success Criteria**:
    - Queries saved to localStorage
    - Queries persist across page reloads
    - Max 10 queries enforced
  - **Testing**: localStorage persistence tests

- [ ] Task 11.2: Add saved queries UI (sidebar or dropdown) [4h]
  - **Scope**: UI to manage saved queries
  - **Files**:
    - `web-app/src/components/SavedQueries.tsx` (create)
    - `web-app/src/components/SavedQueries.module.css` (create)
  - **Implementation**:
    - "Save current query" button
    - Dropdown listing saved queries
    - Click query to apply filters
    - Delete button for each saved query
  - **Success Criteria**:
    - "Save" button saves current filter state
    - Dropdown shows saved queries
    - Clicking query applies filters
    - Delete removes query
  - **Testing**: Saved query interaction tests

- [ ] Task 11.3: Add query sharing via URL [3h]
  - **Scope**: Encode query in URL for sharing
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Encode filters in URL query params
    - Decode URL params on page load
    - "Copy link" button for current query
  - **Success Criteria**:
    - URL contains filter state
    - Sharing URL restores exact filters
    - "Copy link" copies shareable URL
  - **Testing**: URL encoding/decoding tests

**Story 12: Export and Download (MEDIUM)**
**Priority**: Nice-to-Have | **Effort**: 1 day (6h)

- [ ] Task 12.1: Add CSV export functionality [3h]
  - **Scope**: Export logs as CSV file
  - **Files**:
    - `web-app/src/lib/utils/export.ts` (create)
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Generate CSV from log entries
    - Include all columns (timestamp, level, session, message)
    - Trigger browser download with correct filename
    - "Export" button in toolbar
  - **Success Criteria**:
    - "Export" button downloads CSV file
    - CSV includes all visible logs
    - Filename includes timestamp: "logs-2025-12-08-14-30.csv"
  - **Testing**: CSV generation unit tests

- [ ] Task 12.2: Add JSON export option [2h]
  - **Scope**: Export logs as JSON array
  - **Files**:
    - `web-app/src/lib/utils/export.ts` (modify)
  - **Implementation**:
    - Generate JSON array from log entries
    - Pretty-print with 2-space indentation
    - Export dropdown: "CSV" or "JSON"
  - **Success Criteria**:
    - Export dropdown shows CSV and JSON options
    - JSON export downloads formatted JSON file
    - Filename: "logs-2025-12-08-14-30.json"
  - **Testing**: JSON export integration tests

**Story 13: Recent Search History (MEDIUM)**
**Priority**: Nice-to-Have | **Effort**: 1 day (6h)

- [ ] Task 13.1: Store recent searches in localStorage [2h]
  - **Scope**: Track last 10 search queries
  - **Files**:
    - `web-app/src/lib/hooks/useSearchHistory.ts` (create)
  - **Implementation**:
    - Store array of recent searches (max 10)
    - Deduplicate searches (don't store duplicates)
    - Order by recency (most recent first)
  - **Success Criteria**:
    - Last 10 searches persisted
    - Duplicates not stored
    - Most recent search at top
  - **Testing**: Search history storage tests

- [ ] Task 13.2: Add search history dropdown [3h]
  - **Scope**: Dropdown showing recent searches
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Dropdown below search input when focused
    - Show last 10 searches
    - Click search to apply
    - "Clear history" button at bottom
  - **Success Criteria**:
    - Dropdown shows on input focus
    - Clicking search applies query
    - "Clear history" empties list
  - **Testing**: Search history interaction tests

**Deliverables**:
- Saved queries with localStorage persistence
- CSV/JSON export functionality
- Recent search history dropdown
- Query sharing via URL

---

#### Sprint 5: Polish and UX (1 week, 14h) - 🔒 BLOCKED by Sprint 4
**Goal**: Column customization, density toggle, query builder

**Story 14: Column Customization (MEDIUM)**
**Priority**: Nice-to-Have | **Effort**: 2 days (12h)

- [ ] Task 14.1: Add column visibility toggle [4h]
  - **Scope**: Show/hide table columns
  - **Files**:
    - `web-app/src/components/ColumnSettings.tsx` (create)
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Settings dropdown with column checkboxes
    - Show/hide columns: Timestamp, Level, Session, Message
    - Persist column visibility in localStorage
  - **Success Criteria**:
    - Settings dropdown lists all columns
    - Toggling checkbox shows/hides column
    - Visibility persists across reloads
  - **Testing**: Column visibility toggle tests

- [ ] Task 14.2: Add column reordering (drag-and-drop) [4h]
  - **Scope**: Reorder columns via drag-and-drop
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
    - `package.json` (add @dnd-kit/core)
  - **Implementation**:
    - Install @dnd-kit/core for drag-and-drop
    - Make table headers draggable
    - Reorder columns on drop
    - Persist order in localStorage
  - **Success Criteria**:
    - Columns can be dragged to reorder
    - New order applied to table
    - Order persists across reloads
  - **Testing**: Drag-and-drop interaction tests

**Story 15: Density Toggle (LOW)**
**Priority**: Polish | **Effort**: 1 day (6h)

- [ ] Task 15.1: Add density toggle (Compact/Default/Comfortable) [3h]
  - **Scope**: Adjust row height and padding
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
    - `web-app/src/app/logs/page.module.css` (modify)
  - **Implementation**:
    - Density dropdown: Compact, Default, Comfortable
    - CSS classes for each density level
    - Persist density in localStorage
  - **Success Criteria**:
    - Compact: 32px row height, minimal padding
    - Default: 48px row height, normal padding
    - Comfortable: 64px row height, generous padding
    - Density persists across reloads
  - **Testing**: Visual regression tests for each density

- [ ] Task 15.2: Add font size adjustment [2h]
  - **Scope**: Increase/decrease font size
  - **Files**:
    - `web-app/src/app/logs/page.tsx` (modify)
  - **Implementation**:
    - Font size slider: 12px - 16px
    - Apply to log message column only
    - Persist in localStorage
  - **Success Criteria**:
    - Slider adjusts font size smoothly
    - Font size persists
    - Applies only to message column
  - **Testing**: Font size adjustment tests

**Story 16: Query Builder UI (HIGH - Deferred)**
**Priority**: Significant UX Improvement (Deferred to Future Sprint) | **Effort**: 1 week (30h)

- [ ] Task 16.1: Design query builder component [4h]
  - **Scope**: Visual query builder for advanced filtering
  - **Files**:
    - `web-app/src/components/QueryBuilder.tsx` (create)
    - `web-app/src/components/QueryBuilder.module.css` (create)
  - **Implementation**:
    - AND/OR logic selector
    - Field selector (Level, Session, Message, Timestamp)
    - Operator selector (equals, contains, greater than, etc.)
    - Value input field
    - Add/remove condition buttons
  - **Success Criteria**:
    - Builder supports multiple conditions
    - AND/OR logic between conditions
    - Operators appropriate for field type
  - **Testing**: Query builder interaction tests

- [ ] Task 16.2: Generate query from builder state [3h]
  - **Scope**: Convert builder state to backend query
  - **Files**:
    - `web-app/src/lib/utils/queryBuilder.ts` (create)
  - **Implementation**:
    - Convert builder state to GetLogsRequest fields
    - Support complex queries (AND/OR combinations)
    - Validate query before sending
  - **Success Criteria**:
    - Builder state correctly maps to RPC request
    - Complex queries supported
    - Invalid queries show error message
  - **Testing**: Query generation unit tests

**Deliverables**:
- Column visibility and reordering
- Density toggle (Compact/Default/Comfortable)
- Font size adjustment
- (Query builder deferred to future sprint)

---

### Success Metrics

**Performance Targets**:
- Time range filtering: < 500ms response time (p95)
- Live tail latency: < 100ms from log write to display
- Virtual scrolling: 60fps with 10,000+ logs
- Search response: < 200ms (p95)

**Accessibility Targets**:
- WCAG 2.1 AA compliance (100% axe DevTools score)
- Keyboard navigation for all actions
- Screen reader support (JAWS, NVDA, VoiceOver)

**Usability Targets**:
- Time to find specific log: < 10 seconds (from 2-5 minutes)
- Keyboard shortcut adoption: 40%+ of users
- Multi-level filter usage: 60%+ of users
- Live tail usage: 30%+ of active monitoring sessions

**Quality Targets**:
- Zero race conditions from rapid filter changes
- No UI freezes with 1000+ logs
- Smooth animations (60fps)

### Files to Create

**New Components**:
- `web-app/src/components/TimeRangePicker.tsx` - Time range selection
- `web-app/src/components/MultiSelect.tsx` - Multi-select filter
- `web-app/src/components/FilterPill.tsx` - Active filter pills
- `web-app/src/components/LogDetail.tsx` - Expandable log row
- `web-app/src/components/SavedQueries.tsx` - Saved query management
- `web-app/src/components/ColumnSettings.tsx` - Column customization
- `web-app/src/components/KeyboardShortcutsHelp.tsx` - Shortcuts help

**New Hooks**:
- `web-app/src/lib/hooks/useDebounce.ts` - Debouncing hook
- `web-app/src/lib/hooks/useKeyboard.ts` - Keyboard shortcuts
- `web-app/src/lib/hooks/useLogStream.ts` - WebSocket log streaming
- `web-app/src/lib/hooks/useSavedQueries.ts` - Saved query management
- `web-app/src/lib/hooks/useSearchHistory.ts` - Search history

**New Utilities**:
- `web-app/src/lib/utils/time.ts` - Time formatting helpers
- `web-app/src/lib/utils/export.ts` - CSV/JSON export
- `web-app/src/lib/utils/queryBuilder.ts` - Query builder logic

**Backend**:
- `server/services/log_stream.go` - WebSocket log streaming

### Files to Modify

- `web-app/src/app/logs/page.tsx` - Main logs page (all features)
- `web-app/src/app/logs/page.module.css` - Styling updates
- `proto/session/v1/session.proto` - Add context_lines, verify time fields
- `server/server.go` - Add WebSocket route

### Technical Debt

- [ ] **No tests** - Zero test coverage for logs page (add during implementation)
- [ ] **Query builder deferred** - Visual query builder (Story 16, 30h)
- [ ] **Backend query optimization** - Index logs for faster time range queries
- [ ] **Log retention policy** - Implement log rotation and archival

**See Full Details**: No separate doc - all details in this TODO section

**Next Action**: Task 1.1 - Create TimeRangePicker component (4 hours)

---

## READY: History Page UX Improvements

**Status**: Planning Complete, Ready for Implementation
**Priority**: P2 - User Experience and Accessibility
**Epic ID**: EPIC-HISTORY-UX-001
**Estimated Effort**: 7 weeks (1 engineer, 46 hours development + testing)
**Progress**: 0% (Planning 100%, Implementation 0%)

### Overview

Comprehensive UX overhaul of the History Browser page (`/history`) to achieve feature parity with Sessions page, address 23 usability issues across Nielsen's 10 Usability Heuristics and WCAG POUR principles (Perceivable, Operable, Understandable, Robust).

**Key Problems**:
- **No keyboard navigation** - Power users cannot use arrow keys, j/k, /, or Escape (CRITICAL)
- **Missing filtering and grouping** - Flat 100-entry list vs Sessions' 8 grouping modes (CRITICAL)
- **Poor error handling** - No retry buttons, unclear empty states (CRITICAL)
- **Accessibility gaps** - Modal lacks focus trap, ARIA attributes (CRITICAL)
- **Information overload** - Cards cram too much text, poor visual hierarchy (HIGH)
- **No bulk actions** - Cannot select multiple entries for batch operations (HIGH)

**Strategic Value**:
- **Consistency**: Unified UX across Sessions and History pages reduces cognitive load
- **Accessibility**: WCAG 2.1 AA compliance enables users with disabilities
- **Efficiency**: Keyboard shortcuts and bulk actions reduce repetitive work
- **Scalability**: Grouping and pagination handle 1000+ history entries

### Sprint Breakdown

#### Sprint 1: Foundation (2 weeks, 20h) - ⏳ READY
**Goal**: Keyboard navigation, error handling, basic filtering

**Story 1: Keyboard Navigation and Accessibility (CRITICAL)**
- [ ] Task 1.1: Add keyboard navigation hook (arrow keys, j/k, /, Escape) [3h] - NEXT ACTION
- [ ] Task 1.2: Implement loading states and feedback [2h]
- [ ] Task 1.3: Fix modal accessibility (focus trap, ARIA, Escape) [3h]
- [ ] Task 1.4: Add keyboard shortcuts help modal [1h]
- [ ] Task 1.5: Mobile responsive breakpoints (375px, 768px, 1024px) [1h]

**Story 2: Error Handling and Empty States (CRITICAL)**
- [ ] Task 2.1: Implement contextual empty states [2h]
- [ ] Task 2.2: Add error retry functionality [2h]

**Story 3: Filtering (Partial)**
- [ ] Task 3.1: Add filter bar component (model, date, status) [3h]
- [ ] Task 3.4: Persist filter state in localStorage [1h]
- [ ] Task 3.5: Add clear all filters button [1h]

**Deliverables**:
- Full keyboard navigation matching Sessions page
- Loading states and error recovery without page refresh
- Basic filtering (model, date range, status)
- Mobile responsive at all breakpoints
- localStorage persistence for filters

---

#### Sprint 2: Organization (2 weeks, 16h) - 🔒 BLOCKED by Sprint 1
**Goal**: Grouping, autocomplete, filter combinations

**Story 3: Filtering and Grouping (HIGH PRIORITY)**
- [ ] Task 3.2: Implement grouping strategies (8 modes: Date, Project, Model, Status, Branch, Tag, Type, None) [4h]
- [ ] Task 3.3: Add autocomplete search with suggestions [2h]
- [ ] Task 3.6: Implement "Group by" dropdown UI [2h]
- [ ] Task 3.7: Add filter combination logic [1h]

**Story 4: Information Architecture (Partial)**
- [ ] Task 4.1: Redesign history card with visual hierarchy (relative time, truncated UUIDs) [3h]

**Story 5: Enhanced Features (Partial)**
- [ ] Task 5.6: Audit and fix dark mode contrast (WCAG AA) [1h]

**Deliverables**:
- 8 grouping strategies (G key to cycle)
- Autocomplete search with project/model/branch suggestions
- Redesigned cards with improved scannability
- WCAG AA compliant dark mode colors
- Combined filter + search + group operations

---

#### Sprint 3: Actions (2 weeks, 14h) - 🔒 BLOCKED by Sprint 2
**Goal**: Bulk actions, pagination, detail panel enhancements

**Story 4: Information Architecture and Bulk Actions (HIGH PRIORITY)**
- [ ] Task 4.2: Implement bulk selection mode (checkboxes, toolbar) [3h]
- [ ] Task 4.3: Add action toolbar to detail panel (Export, Copy ID, Open Folder, Delete) [2h]
- [ ] Task 4.4: Add message search in modal [2h]
- [ ] Task 4.5: Implement pagination controls (Previous/Next, entries-per-page) [2h]

**Story 5: Enhanced Features (Partial)**
- [ ] Task 5.1: Add read/unread indicators [2h]
- [ ] Task 5.4: Truncate UUIDs with copy button [1h]
- [ ] Task 5.7: Preserve scroll position on navigation [1h]

**Deliverables**:
- Bulk selection and delete operations
- Pagination with configurable entries-per-page
- Action toolbar in detail panel
- Message search within conversations
- Read/unread status tracking
- Scroll position preservation

---

#### Sprint 4: Polish (1 week, 6h) - 🔒 BLOCKED by Sprint 3
**Goal**: Export, syntax highlighting, final enhancements

**Story 5: Enhanced Features (MEDIUM/LOW PRIORITY)**
- [ ] Task 5.2: Implement export functionality (JSON/Markdown) [2h]
- [ ] Task 5.3: Add progress count to loading state [1h]
- [ ] Task 5.5: Add syntax highlighting for code messages [2h]

**Testing and Bug Fixes**
- [ ] E2E tests for critical paths [variable]
- [ ] Accessibility audit (Lighthouse, axe-core) [1h]
- [ ] Performance benchmarks (search < 200ms, filter < 100ms) [1h]
- [ ] Bug fixes and polish [variable]

**Deliverables**:
- Export to JSON and Markdown
- Syntax highlighting for code blocks
- Loading progress indicators
- 100% WCAG 2.1 AA compliance
- Performance benchmarks passed
- Feature complete and production-ready

---

### Dependencies and Risks

**External Dependencies**:
1. **Backend API Updates** (Optional but recommended):
   - Pagination support (offset/limit params) - Improves Task 4.5
   - Delete single/bulk entries API - Required for Tasks 4.2, 4.3
   - Export API - Optional, improves Task 5.2 performance

2. **Library Dependencies**:
   - `react-syntax-highlighter` - Required for Task 5.5
   - `focus-trap-react` - Required for Task 1.3 (may already exist)
   - `react-window` - Optional optimization for Task 4.4 (large message lists)

**Known Risks**:
1. **API Pagination Not Implemented**: Task 4.5 blocked until backend adds offset/limit support
   - **Mitigation**: Implement client-side pagination as temporary solution
2. **Large Message Performance**: Rendering 1000+ messages may cause lag
   - **Mitigation**: Virtual scrolling with react-window (adds 3h to Task 4.4)
3. **Export File Size**: Exporting 1000 entries may hit browser memory limits
   - **Mitigation**: Add warning for large exports, implement streaming (adds 2h to Task 5.2)
4. **localStorage Quota**: Storing read status for 10000 entries may exceed 5MB quota
   - **Mitigation**: LRU cache for recent 1000 entries, QuotaExceededError handling (adds 1h to Task 5.1)

**Critical Path**: Sprint 1 → Sprint 2 → Sprint 3 → Sprint 4 (sequential dependencies)

---

### Success Metrics

**Feature Parity**:
- [ ] All keyboard shortcuts from Sessions page work in History page
- [ ] All 8 grouping strategies implemented
- [ ] Filter UI matches Sessions page patterns
- [ ] Bulk operations available

**Accessibility**:
- [ ] WCAG 2.1 AA compliance verified (Lighthouse 100% score)
- [ ] Screen reader test passed (VoiceOver, NVDA)
- [ ] Keyboard-only navigation complete
- [ ] Color contrast audit passed

**Performance**:
- [ ] Search latency < 200ms (1000 entries)
- [ ] Filter/group latency < 100ms (1000 entries)
- [ ] Page load time < 2s (50 entries)
- [ ] Modal open time < 100ms

**User Experience**:
- [ ] Power user can navigate without mouse
- [ ] User can search and open entry in < 5 seconds
- [ ] User can bulk delete 20 entries in < 10 seconds
- [ ] Error recovery works without page refresh

---

### Nielsen Heuristics and WCAG Violations Fixed

**Nielsen's 10 Usability Heuristics**:
- **H1 - Visibility of System Status**: Issues #2, #13 → Tasks 1.2, 4.5, 5.3
- **H3 - User Control and Freedom**: Issues #4, #5 → Tasks 1.3, 2.2
- **H4 - Consistency and Standards**: Issues #6, #7 → Tasks 3.1, 3.2
- **H7 - Flexibility and Efficiency**: Issues #1, #9 → Tasks 1.1, 4.2
- **H8 - Aesthetic and Minimalist Design**: Issue #8 → Task 4.1
- **H9 - Error Recovery**: Issues #3, #4 → Tasks 2.1, 2.2
- **H10 - Help and Documentation**: Issue #19 → Task 1.4

**WCAG POUR Principles**:
- **Perceivable**: Issues #2, #5, #21 → Tasks 1.2, 1.3, 5.6
- **Operable**: Issues #1, #5 → Tasks 1.1, 1.3
- **Understandable**: Issues #3, #4 → Tasks 2.1, 2.2
- **Robust**: Issue #5 → Task 1.3

---

### Files to Modify

**Primary**:
- `web-app/src/app/history/page.tsx` - Main history page component
- `web-app/src/app/history/history.module.css` - Page styles

**New Components**:
- `web-app/src/components/history/HistoryFilters.tsx` - Filter bar
- `web-app/src/components/history/HistoryCard.tsx` - Redesigned card component
- `web-app/src/components/history/MessageSearch.tsx` - Modal message search
- `web-app/src/components/history/GroupedHistoryList.tsx` - Grouping logic
- `web-app/src/components/history/BulkActionToolbar.tsx` - Bulk operations
- `web-app/src/components/history/DetailPanelActions.tsx` - Action toolbar
- `web-app/src/components/history/Pagination.tsx` - Pagination controls
- `web-app/src/components/history/AutocompleteSearch.tsx` - Search with suggestions
- `web-app/src/components/history/KeyboardShortcutsHelp.tsx` - Help modal
- `web-app/src/components/history/EmptyState.tsx` - Contextual empty states

**New Hooks**:
- `web-app/src/lib/hooks/useKeyboard.ts` - Keyboard navigation
- `web-app/src/lib/hooks/useHistoryFilters.ts` - Filter state management
- `web-app/src/lib/hooks/useAutocompleteSuggestions.ts` - Search suggestions
- `web-app/src/lib/hooks/useSelection.ts` - Bulk selection
- `web-app/src/lib/hooks/useReadStatus.ts` - Read/unread tracking
- `web-app/src/lib/hooks/useFocusTrap.ts` - Modal accessibility
- `web-app/src/lib/hooks/useScrollRestoration.ts` - Scroll position

**New Utilities**:
- `web-app/src/lib/grouping/historyGrouping.ts` - Grouping strategies
- `web-app/src/lib/export/historyExporter.ts` - Export to JSON/Markdown

**Reference Patterns From**:
- `web-app/src/components/sessions/SessionList.tsx` - Filtering, keyboard, grouping
- `web-app/src/components/sessions/SessionCard.tsx` - Card design patterns
- `web-app/src/lib/hooks/useRepositorySuggestions.ts` - Autocomplete pattern

**See Full Details**: [History Page UX Improvements Feature Plan](docs/tasks/history-page-ux-improvements.md)

**Next Action**: Task 1.1 - Add keyboard navigation hook (3 hours)

---

## COMPLETE: Debug Snapshot Diagnostic Tool

**Status**: Implemented (commit 72cc63a)
**Priority**: P2 - Developer Experience and Troubleshooting
**Epic ID**: EPIC-DEBUGSNAP-001
**Progress**: 100% (All stories implemented)

### Overview

One-click diagnostic capture from the web UI debug menu. Gathers session metadata, tmux pane content, pending approvals, and recent server logs into a single JSON file at `~/.stapler-squad/logs/debug-snapshot-{timestamp}.json`.

**Key Features**:
- [x] **Story 1**: Backend snapshot collector service (`server/services/debug_snapshot.go`)
- [x] **Story 2**: ConnectRPC endpoint (`CreateDebugSnapshot` in `session_service.go`, proto generated)
- [x] **Story 3**: Web UI integration in existing debug menu (`DebugMenu.tsx` Diagnostics section)

### Technical Approach:
- Single `CreateDebugSnapshot` unary RPC on `SessionService`
- Server-side collector with per-subsystem 5-second timeouts
- Captures: session state, tmux pane output, approval store, recent logs
- JSON file output with version field for future schema evolution
- Partial failure tolerance (errors recorded in snapshot, not fatal)

**See Full Details**: [Debug Snapshot Feature Plan](docs/tasks/debug-snapshot.md)

**Next Action**: Task 1.1 - Create `server/services/debug_snapshot.go` (2 hours)

---

## COMPLETE: Passkey Authentication and Remote Access

**Status**: Implemented (commits 756bd95, 45eda07, af46e5e)
**Priority**: P1 - Enables secure remote access to stapler-squad
**Epic ID**: EPIC-PASSKEY-001
**Progress**: 100% (All stories implemented)

### Overview

Implement WebAuthn/FIDO2 passkey authentication with QR-code-based enrollment to enable secure remote access to the stapler-squad web UI from non-local machines (phone, tablet, laptop).

**Key Problems Solved**:
- **No remote access** - Server binds exclusively to localhost, inaccessible from other devices
- **No authentication** - Localhost-only was the implicit security boundary
- **No enrollment UX** - No way to register devices for access

**Strategic Value**:
- **Remote Monitoring**: Check AI agent sessions from phone while away from desk
- **Approval Workflow**: Respond to permission requests from mobile devices
- **Multi-Device Access**: Use stapler-squad from any device on the network

### Implementation Plan (5 Stories, 16 Tasks)

#### Story 1: Network Exposure and TLS Foundation - COMPLETE
- [x] Task 1.1: Add listen address configuration (config.go, main.go)
- [x] Task 1.2: Auto-generate self-signed TLS certificate (server/tls.go)
- [x] Task 1.3: Enable HTTPS when remote access is active (server/server.go)
- [x] Task 1.4: Update CORS middleware for remote origins (middleware/cors.go)

#### Story 2: WebAuthn Backend - COMPLETE
- [x] Task 2.1: Implement credential store (server/auth/store.go)
- [x] Task 2.2: Implement session token manager (server/auth/session.go)
- [x] Task 2.3: Implement WebAuthn registration endpoint (server/auth/handlers.go)
- [x] Task 2.4: Implement WebAuthn authentication endpoint (server/auth/handlers.go)
- [x] Task 2.5: Implement setup token for first-time registration (server/auth/setup.go)

#### Story 3: QR Code Enrollment Flow - COMPLETE
- [x] Task 3.1: Add QR code generation (server/auth/qrcode.go)
- [x] Task 3.2: Setup page with QR code display (server/auth/handlers.go)

#### Story 4: Frontend Auth UI - COMPLETE
- [x] Task 4.1: Create login page component (web-app/src/app/login/)
- [x] Task 4.2: Create auth context and route guard (web-app/src/lib/contexts/)
- [x] Task 4.3: Passkey management wired into login page

#### Story 5: Auth Middleware - COMPLETE
- [x] Task 5.1: Implement auth middleware (server/middleware/auth.go)
- [x] Task 5.2: Wire auth middleware into server startup (server/server.go)
- [x] Task 5.3: Dual-server mode for local vs remote access (commit af46e5e)

### Key Architecture Decisions
- **WebAuthn/FIDO2** for phishing-resistant authentication (ADR-001)
- **Direct bind with auto-TLS** for network exposure (ADR-002)
- **JSON file storage** for credentials, matching existing patterns (ADR-003)
- **go-webauthn/webauthn** library (de facto Go standard) (ADR-004)
- **QR code encodes registration URL** for device enrollment (ADR-005)

### Known Critical Issues
- WebAuthn rpID must match access hostname (BUG-PASSKEY-001, CRITICAL)
- Self-signed TLS cert trust on phones (BUG-PASSKEY-002, HIGH)
- Bootstrap race condition during first registration (BUG-PASSKEY-003, HIGH)
- CORS with credentials for WebAuthn (BUG-PASSKEY-004, HIGH)

**See Full Details**: [Passkey Authentication Feature Plan](docs/tasks/passkey-authentication.md)

**Next Action**: Task 1.1 - Add listen address configuration (2 hours)

---

## Planned

### Notification De-Duplication and Aggregation

**Status**: COMPLETE (commits b1ec1a3, 8e9d5da)
**Priority**: P1 -- DONE
**Epic ID**: EPIC-NOTIF-DEDUP-001
**Feature Plan**: [docs/tasks/notification-deduplication.md](docs/tasks/notification-deduplication.md)

**Implemented**:
- Server-side dedup in `NotificationHistoryStore.Append()` by `(sessionId, notificationType)` key (commit b1ec1a3)
- Client-side grouping with "xN" count badge in NotificationPanel (commit 8e9d5da)
- Toast suppression for duplicate real-time events (commit 8e9d5da)
- Proto `occurrence_count` and `last_occurred_at` fields added and regenerated

---

## Future Priorities

### New Strategic Plans:
- [ ] **Conductor Feature Parity** - Per-turn checkpoints, run scripts, built-in diff, MCP integration; [docs/tasks/conductor-feature-parity.md](docs/tasks/conductor-feature-parity.md)
- [ ] **Stapler Squad Rebrand** - Hard fork rename from claude-squad to stapler-squad; [docs/tasks/stapler-squad-rebrand.md](docs/tasks/stapler-squad-rebrand.md)

### Medium Term (After Search & Sort):
- [ ] **External Claude PTY Interception** - Discover and monitor external Claude processes
- [ ] **Session Health Check Integration** - Evaluate health check system
- [ ] **Filtering System Enhancement** - Tag vs Category analysis
- [ ] **Help System Consolidation** - Compare current vs unused help generator

---

## PLANNED: External Claude PTY Interception

**Status**: Planned (Feature Plan Complete)
**Priority**: Medium Term - Enhances multi-session workflow
**Estimated Effort**: 4-5 weeks (4 stories, 15 atomic tasks)

### User Value
Enable users to discover, monitor, and interact with external Claude Code processes running on their machine through the stapler-squad web interface, without requiring those sessions to be started by stapler-squad.

### Key Features:
- [ ] **Story 1**: Enhanced external tmux session discovery (1 week)
- [ ] **Story 2**: Real-time terminal streaming for external sessions (1.5 weeks)
- [ ] **Story 3**: Approval detection and notifications for external sessions (1 week)
- [ ] **Story 4**: Session adoption (external to managed) (1 week)

### Technical Approach:
- Hybrid multi-tier discovery using tmux as primary interception layer
- tmux `pipe-pane` for real-time streaming to Unix domain sockets
- Extends existing `PTYDiscovery` infrastructure
- Integrates with existing notification and review queue systems

**See Full Details**: [External Claude PTY Interception Feature Plan](docs/tasks/pty-interception-external-claude.md)

**Next Action**: Task 1.1 - Extend PTYDiscovery for comprehensive tmux scanning (3 hours)

### Long Term (Future Sessions):
- [ ] **Dead Code Removal** - Clean up unused constructors and test mocks
- [ ] **Performance Optimization** - Large directory tree handling improvements
- [ ] **Advanced Features** - Network path support, fuzzy path matching

---

## IN PROGRESS: Claude Code Hook Approval (Remote Approve/Reject)

**Status**: Stories 1-4 substantially complete, Stories 5-6 pending
**Priority**: P1 - Active development (closes the approval response loop)
**Epic ID**: EPIC-APPROVAL-001
**Feature Plan**: [docs/tasks/claude-code-hook-approval.md](docs/tasks/claude-code-hook-approval.md)
**Updated**: 2026-03-13

### What Is Complete

- **Backend Store** (`server/services/approval_store.go`): Thread-safe ApprovalStore with Create, Resolve, CancelSession, CleanupExpired.
- **HTTP Hook Endpoint** (`server/services/approval_handler.go`): POST /api/hooks/permission-request blocks until user decides; timeout at 4 min; wired in server.go.
- **Session Hook Injection**: InjectHookConfig merges .claude/settings.local.json at session creation via session_service.go.
- **ConnectRPC RPCs** (proto + handlers): ResolveApproval and ListPendingApprovals defined and wired.
- **Web UI Components**: ApprovalCard, ApprovalPanel, ApprovalNavBadge, useApprovals hook.
- **Notification Integration**: NotificationToast shows Approve/Deny for approval_needed; useSessionNotifications resolves approval via RPC.
- **Navigation Badge**: ApprovalNavBadge rendered in Header showing live pending count.
- **Go build**: Passes (go build ./... clean as of 2026-03-13).

### What Remains

1. **DONE: Proto regenerated** — `make proto-gen` has been run; PendingApprovalProto, ResolveApproval, and ListPendingApprovals are all in generated Go and TypeScript bindings.
2. **DONE: Hook type migrated** — Hook injection changed from HTTP type to command type using curl (commit a032383). Feature plan ADR still says HTTP but implementation uses command.
3. **End-to-end smoke test** — create a session, trigger a Bash approval in Claude Code, verify the approval appears in the web UI and Approve works.
4. **Story 5: Review Queue Integration** — Approve/Deny buttons on review queue items with pending_approval enrichment (2-3 tasks, ~5h).
5. **Story 6: Mobile UX** — responsive layout, mobile banner (2-3 tasks, ~5h).

### Highest-Priority Next Action

Do an end-to-end smoke test. If the UI renders and approve/deny work, the core feature is shippable. Stories 5-6 are enhancements.

---

## Context Notes

**Last Updated**: 2026-03-20
**Current Phase**: Architecture refactoring wave complete; tmux user options metadata implemented (uncommitted); Approval Stories 5-6 pending
**Next Milestone**: Wire ScanFromUserOptions() into server startup (2h); Approval Story 5 smoke test then Review Queue Integration

**Completed Projects**:
1. **Web UI Implementation** - Complete (all 5 stories)
2. **Session History Viewer Integration** - Complete (TUI + launch from history)
3. **BUG-001 Investigation** - Already fixed in codebase
4. **BUG-002 Investigation** - Already fixed in codebase
5. **BUG-003 Resolution** - FIXED (2025-12-01) - 42x file size reduction
6. **Claude Config Editor Phase 1** - Backend foundation complete
7. **Claude Config Editor Phase 2** - TUI overlay complete
8. **Claude Config Editor Phase 2.5** - TUI integration complete (E key binding)
9. **Claude Config Editor Phase 3** - Web UI complete (Monaco, validation, navigation)
10. **Test Stabilization Bug Fixes** - 4 bugs fixed (BUG-004 through BUG-007) in 90 minutes
11. **Hook Approval Stories 1-4** - Backend, command hook, RPCs, web UI components all implemented; proto regenerated; hook migrated to command type
12. **Passkey Authentication (EPIC-PASSKEY-001)** - Complete (commits 756bd95, 45eda07, af46e5e); server/auth/ with WebAuthn, QR code, dual-server mode
13. **Debug Snapshot (EPIC-DEBUGSNAP-001)** - Complete (commit 72cc63a); DebugMenu.tsx Diagnostics section wired to CreateDebugSnapshot RPC
14. **Notification History Persistence** - Complete (commit 1cc2c6c); server-side storage, GetNotificationHistory RPC, NotificationPanel surfaces history
15. **Review Queue Persistence and Background Monitoring** - Complete (commit 4b55b66); startup scan for pre-existing approval prompts
16. **Session Sort Web UI (EPIC-SEARCH-001 Story 3)** - Complete (commit 76bce00); SortField/SortDir with localStorage persistence in SessionList.tsx
17. **Mobile/Lighthouse Phase 2-4 (partial)** - Complete (commits 6f51ce5, 4a49e82); modal focus trapping, header hidden on login, rem fonts, design tokens, SVG icons, page metadata
18. **History Page Decomposition** - Complete (commit 70700c7); page.tsx decomposed into hooks and components
19. **Terminal Module Decomposition** - Complete (commit 6027479); TerminalOutput and useTerminalStream decomposed
20. **Architecture Refactoring Wave** - Complete (commits 506faf5, 41a0038, 6f03128, 78c6e76, 5b49221); dep hardening, SessionService split, domain invariants, circuit breaker, frontend quick wins
21. **Notification Deduplication (EPIC-NOTIF-DEDUP-001)** - Complete (commits b1ec1a3, 8e9d5da); server-side dedup, client grouping, toast coalescing

**Uncommitted Work (This Session)**:
- `session/mux/tmux_options.go` (new): WriteSessionUserOptions(), ScanByUserOptions() -- stores session metadata as tmux user options
- `session/mux/tmux_options_test.go` (new): 4 tests passing
- `session/mux/multiplexer.go` (modified): calls WriteSessionUserOptions() after session creation
- `session/mux/discovery.go` (modified): adds ScanFromUserOptions() method
- PENDING: Wire ScanFromUserOptions() into server startup (currently implemented but never called) -- 2h

**Current Status**:
- Approval feature: Stories 1-4 + proto regen + hook migration complete; Stories 5-6 pending (smoke test recommended before starting Story 5)
- Mobile/Lighthouse: Task 1.1 (viewport meta tag, SEO +10) still pending; most Phase 2-4 tasks complete
- Search/Sort: Web UI sort complete; backend sort infrastructure (Story 1) still pending
- tmux user options: Implemented but ScanFromUserOptions() not yet wired into server startup
- ExternalTmuxStreamer: Still uses 150ms polling; control mode infrastructure exists but not integrated
- New plans: Conductor Feature Parity (docs/tasks/conductor-feature-parity.md), Stapler Squad Rebrand (docs/tasks/stapler-squad-rebrand.md)
- 7 bugs fixed total (BUG-001 through BUG-007)
- 1 CRITICAL bug blocking test development (BUG-008 - sessions don't render in tests)
- 4 additional bugs require investigation (BUG-009 through BUG-012)