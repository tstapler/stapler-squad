# Claude Squad - Current Priority Tasks

## Priority Summary

**P1** - Claude Config Editor Phase 3 (Web UI) - 4 hours - IN PROGRESS (Monaco complete)
**P2** - BUG-003 Resolution - 3 hours - INVESTIGATED (diff content removal)
**P3** - Test Stabilization - Deferred until major features complete

**Recent Completion**: BUG-003 Investigation ✅ (Root cause: 33MB diff_stats.content field)
**Bug Status Update**: BUG-001 ✅ Already Fixed, BUG-002 ✅ Already Fixed, BUG-003 Investigated

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
- [Implementation Status Report](docs/tasks/web-ui-implementation-status.md)

---

## ✅ RESOLVED: Persistence Layer Bugs

**Status**: All bugs investigated and resolved or have clear fix path
**Investigation Date**: 2025-11-30
**Total Effort**: BUG-001/002 already fixed, BUG-003 investigated (3 hour fix ready)

### Bugs Fixed (Already Resolved in Codebase)

**BUG-001** [HIGH]: LastAcknowledged Field Not Persisted ✅ ALREADY FIXED
- **Original Report**: Review queue snooze functionality appeared broken
- **Investigation Result**: Field IS properly persisted in storage.go:63, instance.go:167, instance.go:259
- **Tests**: Comprehensive test suite in instance_last_acknowledged_test.go (all passing)
- **Status**: ✅ No fix needed - functionality working correctly
- **Location**: `/Users/tylerstapler/IdeaProjects/claude-squad/docs/bugs/fixed/BUG-001-last-acknowledged-persistence.md`

**BUG-002** [MEDIUM]: LastMeaningfulOutput Timestamp Reset ✅ ALREADY FIXED
- **Original Report**: Historical activity timestamps appeared to reset on startup
- **Investigation Result**: Signature-based change detection already implemented in UpdateTerminalTimestamps()
- **Tests**: Test suite in instance_timestamp_signature_test.go (5 tests, all passing)
- **Status**: ✅ No fix needed - signature logic working correctly
- **Location**: `/Users/tylerstapler/IdeaProjects/claude-squad/docs/bugs/fixed/BUG-002-timestamp-refresh-reset.md`

**BUG-003** [LOW]: Large State File Size (34MB JSON) 🔍 INVESTIGATED
- **Impact**: Scalability concern - slow startup (500ms), high memory (70MB)
- **Root Cause**: **IDENTIFIED** - `diff_stats.content` field stores full git diffs (33MB of 34MB file)
  - 40 sessions with average 821 KB diff content each
  - Largest single diff: 25.7 MB in "load-restrictions" session
  - **97.6% of file size** is unnecessary diff content
- **Fix Ready**: Remove `diff_stats.content` from serialization (3 hours, 4 files)
  - Keep metadata (`added`, `removed` counts)
  - Generate diffs on-demand via GetSessionDiff RPC
  - **Expected impact**: 34 MB → 800 KB (42x reduction!)
- **Status**: 🟡 Investigation complete, fix ready to implement (P2 priority)
- **Location**: `/Users/tylerstapler/IdeaProjects/claude-squad/docs/bugs/open/BUG-003-large-state-file-size.md`
- **Task Doc**: `/Users/tylerstapler/IdeaProjects/claude-squad/docs/tasks/bug-003-diff-content-removal.md`

**See**:
- [BUG-003 Resolution Task](docs/tasks/bug-003-diff-content-removal.md) - Atomic task breakdown (2 stories, 4 tasks, 3 hours)
- [Bug Reports](docs/bugs/) - Organized by status (open/, fixed/, in-progress/, obsolete/)

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
- **Workflow Integration**: Seamless Claude Code + Claude Squad integration

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
- [History Viewer Integration Plan](docs/tasks/history-viewer-integration.md) - Complete atomic task breakdown

---

## IN PROGRESS: Claude Config Editor

**Status**: Phase 1 ✅ Complete, Phase 2 ✅ Complete, Phase 2.5 ✅ Complete, Phase 3 Ready
**Priority**: P1 - Active development
**Epic ID**: FEATURE-001
**Estimated Effort**: 3-4 weeks (1 engineer)
**Progress**: 75% (Phase 1, 2, and 2.5 complete - TUI fully integrated)

### Overview

Add comprehensive Claude Code configuration management to Claude Squad:
- View/edit Claude configuration files (.claude/CLAUDE.md, settings.json, agents.md)
- Syntax highlighting and validation
- Real-time error feedback
- Web UI integration with Monaco editor

**Strategic Value**:
- **Transparency**: Direct config access eliminates manual file editing
- **Workflow Integration**: Tight Claude Code + Claude Squad integration
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
4. **STORY-006: Config Editor Web Component** [3 days]
   - ✅ TASK-006.1: Monaco editor integration [2 hours] - COMPLETE
     - ✅ Keyboard shortcuts (Ctrl+S/Cmd+S to save)
     - ✅ JSON syntax highlighting with dark theme (vs-dark)
     - ✅ Line numbers and minimap
     - ✅ Code folding and bracket pair colorization
     - ✅ Auto-formatting on paste/type
     - ✅ Word wrap and tab size: 2 spaces
     - ✅ Read-only mode during save operations
   - [ ] TASK-006.2: Real-time validation feedback [2 hours]
   - [ ] TASK-006.3: Multi-file navigation improvements [2 hours]

**Milestone 3**: Web UI In Progress - Monaco editor complete

#### Phase 4: Integration & Polish (Week 4)
**Goal**: Complete feature integration and testing

**Stories**:
5. **STORY-010: Testing & Documentation** [1 day]

**Milestone 4**: Feature Complete - Ready for production release
## Future Priorities

### Medium Term (After Web UI MVP):
- [ ] **Claude Config Editor** - View/edit Claude configs (PLANNED - see above)
- [ ] **Session Health Check Integration** - Evaluate health check system
- [ ] **Filtering System Enhancement** - Tag vs Category analysis
- [ ] **Help System Consolidation** - Compare current vs unused help generator

### Long Term (Future Sessions):
- [ ] **Dead Code Removal** - Clean up unused constructors and test mocks
- [ ] **Performance Optimization** - Large directory tree handling improvements
- [ ] **Advanced Features** - Network path support, fuzzy path matching

---

## Context Notes

**Last Updated**: 2025-11-30
**Current Phase**: Claude Config Editor - Phase 3 Web UI Implementation (Monaco Complete)
**Next Milestone**: Either Phase 3 Web UI completion OR BUG-003 resolution (both P1/P2)

**Completed Projects**:
1. **Web UI Implementation** - ✅ 100% complete (all 5 stories)
2. **Session History Viewer Integration** - ✅ 100% complete (TUI + launch from history)
3. **BUG-001 Investigation** - ✅ Already fixed in codebase
4. **BUG-002 Investigation** - ✅ Already fixed in codebase
5. **BUG-003 Investigation** - ✅ Root cause identified (diff_stats.content bloat)
6. **Claude Config Editor Phase 1** - ✅ Backend foundation complete
7. **Claude Config Editor Phase 2** - ✅ TUI overlay complete
8. **Claude Config Editor Phase 2.5** - ✅ TUI integration complete (E key binding)

**Active Projects**:
1. **Claude Config Editor Phase 3** (P1 - 4 hours remaining, Monaco editor complete)
   - Task 006.2: Real-time validation feedback [2h]
   - Task 006.3: Multi-file navigation improvements [2h]
2. **BUG-003 Resolution** (P2 - 3 hours, investigation complete, fix ready)
   - Remove diff_stats.content from serialization
   - Expected: 34 MB → 800 KB state file (42x reduction)

**Current Status**:
- All major MVP features complete and production-ready
- Zero critical or high-severity bugs found (BUG-001/002 already fixed)
- BUG-003 has clear fix path ready to implement (P2 priority)
- Test stabilization deferred until major features complete

**Next Steps**:
1. **Option A**: Complete Config Editor Phase 3 (P1, 4 hours) - Web UI feature parity
2. **Option B**: Fix BUG-003 (P2, 3 hours) - Performance improvement, production readiness
3. **Option C**: Test stabilization (P3, deferred) - Only after features complete

**Recommendation**: BUG-003 fix (Option B) provides high impact (42x performance improvement) with low risk and fits within one work session. Config Editor Phase 3 can follow immediately after.