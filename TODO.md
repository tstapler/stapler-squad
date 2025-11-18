# Claude Squad - Current Priority Tasks

## Priority Summary

**P1** - Web UI Story 3 (Session Creation) - 4 hours remaining - IN PROGRESS (Story 3.1 complete ✅)
**P2** - Persistence Bug Fixes - ✅ COMPLETE (BUG-001, BUG-002 already fixed and tested)
**P3** - Test Stabilization & BUG-003 Investigation - Deferred

---

## 🚧 IN PROGRESS: Web UI Implementation

**Status**: Stories 1-2 complete, Story 3.1 complete, remaining tasks ready
**Priority**: P1 - Core user workflows, production deployment pending
**Progress**: 53% complete (2.3 of 5 stories - Story 3 is 33% complete)

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

#### ✅ Story 3.1 - Session Creation Wizard (COMPLETE)
- [x] Multi-step wizard with validation (Zod + react-hook-form)
- [x] Step 1: Basic Info (title, category)
- [x] Step 2: Repository (path, workingDir, branch with autocomplete)
- [x] Step 3: Configuration (program, prompt, autoYes)
- [x] Step 4: Review and confirm
- [x] Integration with useSessionService.createSession
- [x] Success/error feedback with navigation to home
- [x] Build verified: 164KB bundle size for /sessions/new route

### 🎯 Next Atomic Task: Story 3.2 - Path Discovery and Auto-fill

**Task**: Implement intelligent repository path discovery and auto-fill
**Estimated Time**: 2 hours
**Priority**: P1 - Enhances session creation UX
**Context Boundary**: 4 files (useRepositorySuggestions.ts, discovery logic, API endpoint, tests) ✅

**Implementation Steps**:
1. Enhance `useRepositorySuggestions` hook with contextual discovery:
   - Scan common development directories (~/Projects, ~/Code, ~/src, ~/dev)
   - Check recently accessed git repositories from state file
   - Detect parent repository if creating from subdirectory
2. Add repository metadata caching for faster suggestions
3. Implement smart branch name suggestions based on repository context
4. Add keyboard shortcuts for quick repository selection (Recent, Favorites)
5. Test autocomplete performance with 50+ repositories

**Context Files to Understand**:
- `web-app/src/lib/hooks/useRepositorySuggestions.ts` - Current suggestion implementation
- `web-app/src/lib/hooks/useBranchSuggestions.ts` - Branch autocomplete logic
- `ui/overlay/sessionSetup.go` - TUI path discovery reference (git integration)
- `web-app/src/components/ui/AutocompleteInput.tsx` - Autocomplete UI component

**Success Criteria**:
- ✅ Common dev directories automatically scanned on first use
- ✅ Recently used repositories appear at top of suggestions
- ✅ Parent repository auto-detected from current directory
- ✅ Branch suggestions contextual to selected repository
- ✅ Autocomplete performs smoothly with 50+ repositories (<100ms)

**Impact**: Dramatically reduces friction in session creation workflow

**See**: [Web UI Enhancement Epic](docs/tasks/web-ui-enhancements.md)

### ⏸️ Pending Stories:

#### Story 3 - Session Creation Wizard (IN PROGRESS - 1 of 3 tasks complete)
- [x] Task 3.1: Multi-step form with validation (3h) ✅ COMPLETE
- [ ] Task 3.2: Path discovery and auto-fill (2h) **← NEXT** (after BUG-001 fix)
- [ ] Task 3.3: Session templates (2h)

#### Story 4 - Bulk Operations (NOT STARTED - 3 tasks, 8h)
- [ ] Task 4.1: Multi-select and bulk actions (3h)
- [ ] Task 4.2: Advanced filtering (2h)
- [ ] Task 4.3: Performance dashboard (3h)

#### Story 5 - Mobile & Accessibility (NOT STARTED - 3 tasks, 8h)
- [ ] Task 5.1: Responsive mobile layout (3h)
- [ ] Task 5.2: WCAG 2.1 AA compliance (3h)
- [ ] Task 5.3: Touch gestures (2h)

### Current Deployment:
- **Web UI**: `http://localhost:8543`
- **Build Status**: ✅ Builds successfully (~5.0s)
- **Bundle Size**: 147KB (main), 164KB (wizard route /sessions/new)
- **Routes**: Home (/), Config (/config), History (/history), Logs (/logs), Review Queue (/review-queue), New Session (/sessions/new)

### Technical Debt:
- [ ] **Zero test coverage** - No unit/integration tests yet
- [ ] **Terminal streaming** - Placeholder implementation only
- [ ] **Diff updates** - Mock data, needs real API integration
- [ ] **Static export limitation** - Can't share direct session detail URLs

**See**:
- [Web UI Enhancement Epic](docs/tasks/web-ui-enhancements.md)
- [Implementation Status Report](docs/tasks/web-ui-implementation-status.md)

---

## ✅ RESOLVED: Persistence Layer Bugs

**Status**: FIXED - All high/medium severity bugs resolved
**Resolution Date**: 2025-01-17
**Total Effort**: ~3 hours (proactive fixes during development)

### Bugs Fixed (2025-01-17)

**BUG-001** [HIGH]: LastAcknowledged Field Not Persisted ✅ FIXED
- **Impact**: Review queue snooze functionality broken across restarts
- **Resolution**: Field added to InstanceData serialization in storage.go:63, instance.go:167, instance.go:259
- **Tests**: Comprehensive test suite in instance_last_acknowledged_test.go (all passing)
- **Status**: ✅ Fixed and verified

**BUG-002** [MEDIUM]: LastMeaningfulOutput Timestamp Reset ✅ FIXED
- **Impact**: Historical activity timestamps incorrect after startup
- **Resolution**: Signature-based change detection in UpdateTerminalTimestamps() (instance.go:1693-1742)
- **Tests**: Test suite in instance_timestamp_signature_test.go (5 tests, all passing)
- **Status**: ✅ Fixed and verified

**BUG-003** [LOW]: Large State File Size (34MB JSON)
- **Impact**: Scalability concern, no immediate functionality impact
- **Root Cause**: Unknown - investigation required (session count vs embedded data)
- **Fix**: Investigation phase (1 hour), then targeted fix (2-4 hours)
- **Status**: Open - investigation deferred until higher priorities complete

### Quick Wins Available

Both BUG-001 and BUG-002 can be fixed with **minimal risk, high impact** changes:
- Total effort: 3 hours (1h + 2h)
- Context boundary: 5 files total, well within AIC limits
- Risk: Very low (backward compatible, well-tested)
- Impact: Restores broken functionality, prevents data loss

**See**:
- [Persistence Quick Wins Task](docs/tasks/persistence-quick-wins.md) - Detailed atomic task breakdown
- [Bug Reports](docs/bugs/) - BUG-001, BUG-002, BUG-003 detailed analysis

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

## PLANNED: Session History Viewer Integration

**Status**: Backend Complete, Integration Pending
**Priority**: P2 - After Web UI Session Creation
**Epic ID**: FEATURE-002-Integration
**Estimated Effort**: 8-10 hours (1-2 days)
**Progress**: Backend 100%, TUI Overlay 100%, Integration 0%

### Implementation Status

**✅ Complete Components**:
- ✅ **Backend**: `session/history.go` (236 lines) - JSONL parser, search, filtering
- ✅ **TUI Overlay**: `ui/overlay/historyBrowserOverlay.go` (350 lines) - List/detail modes
- ✅ **RPC Handlers**: `server/services/session_service.go` - ListClaudeHistory, GetClaudeHistoryDetail
- ✅ **Proto Definitions**: `proto/session/v1/session.proto` - ClaudeHistoryEntry messages
- ✅ **Web UI**: `web-app/src/app/history/page.tsx` (264 lines) - Full browser page

**❌ Missing Integration**:
- [ ] Add HistoryBrowser state to `app/state/types.go`
- [ ] Wire history browser into UI coordinator
- [ ] Create `handleHistoryBrowser()` function in `app/app.go`
- [ ] Add `handleHistoryBrowserState()` key handler
- [ ] Add H key binding to menu and bridge

### 🎯 Recommended Next Task: Task 1.1 - Add HistoryBrowser State

**Task**: Add HistoryBrowser State to Application State Machine
**Estimated Time**: 1 hour
**Priority**: P2 - Foundation for history viewer feature
**Context Boundary**: 1 file (app/state/types.go, 75 lines) ✅

**Why This Task First?**:
1. **Zero Dependencies**: No prerequisites, can start immediately
2. **Minimal Context**: Single file, simple enum addition
3. **Foundation**: Unblocks all 4 subsequent integration tasks
4. **Low Risk**: Trivial change, easy to verify
5. **Quick Win**: Completes in 1 hour

**Implementation Steps**:
1. Add `HistoryBrowser` constant after `TagEditor` in State enum
2. Update `String()` method to return "HistoryBrowser"
3. Update `IsValid()` upper bound to `HistoryBrowser`
4. Add `HistoryBrowser` to `IsOverlayState()` switch case
5. Compile: `go build ./app/state`
6. Test: `go test ./app/state -v`

**Context Files to Understand**:
- `app/state/types.go` (75 lines) - State enum and methods

**Success Criteria**:
- ✅ HistoryBrowser constant added to State enum
- ✅ All state methods updated correctly
- ✅ Code compiles without errors
- ✅ Tests pass

**Next Tasks After This**:
- Task 1.2: Add UI Coordinator History Browser Methods (2h)
- Task 1.3: Add handleHistoryBrowser Function (2h)
- Task 1.4: Add handleHistoryBrowserState Handler (2h)
- Task 1.5: Add Key Binding and Menu Integration (1h)

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

### Remaining Work Breakdown

**Story 1: TUI Integration** (8 hours, 5 tasks):
- [ ] Task 1.1: Add HistoryBrowser State [1h] **← NEXT**
- [ ] Task 1.2: UI Coordinator Methods [2h]
- [ ] Task 1.3: handleHistoryBrowser Function [2h]
- [ ] Task 1.4: handleHistoryBrowserState Handler [2h]
- [ ] Task 1.5: Key Binding & Menu Integration [1h]

**Story 2: Launch from History** (DEFERRED - Phase 2):
- [ ] Task 2.1: Session creation from entry [3h]
- [ ] Task 2.2: Worktree/branch setup [2h]
- [ ] Task 2.3: Pre-populate fields [1h]

**See Full Details**:
- [History Viewer Integration Plan](docs/tasks/history-viewer-integration.md) - Complete atomic task breakdown

---

## PLANNED: Claude Config Editor

**Status**: Planning Complete - Ready for Implementation
**Priority**: P2 - After History Viewer Integration
**Epic ID**: FEATURE-001
**Estimated Effort**: 3-4 weeks (1 engineer)
**Progress**: 0% (Planning phase)

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

### 🎯 Recommended First Task: TASK-001.1 - Backend Config Foundation

**Task**: Create config/claude.go File Structure
**Estimated Time**: 1 hour
**Priority**: P2 - Foundation for all config features
**Context Boundary**: 1 file (config/claude.go) ✅

**Why This Task First?**:
1. **Zero Dependencies**: No prerequisites, can start immediately
2. **Architectural Foundation**: Establishes patterns for entire feature
3. **Minimal Context**: Single file, clear scope, fast validation
4. **Enables Parallel Work**: Unblocks TASK-001.2, 001.3, 001.4
5. **AIC Framework Perfect Fit**: 1h task, 1 file, atomic scope

**Implementation Steps**:
1. Create `config/claude.go` with ClaudeConfigManager struct
2. Add GetClaudeDir() helper method
3. Define ConfigFile struct (Name, Path, Content, ModTime)
4. Add basic error types (ErrConfigNotFound, ErrInvalidConfig)
5. Export manager constructor: NewClaudeConfigManager()
6. Add comprehensive GoDoc comments
7. Compile and validate: `go build ./config`

**Context Files to Understand**:
- `config/config.go` - Existing config patterns and conventions
- `config/state.go` - State management patterns
- `~/.claude/` - Target directory structure (CLAUDE.md, settings.json, agents.md)

**Success Criteria**:
- ✅ File compiles without errors
- ✅ Exports ClaudeConfigManager struct
- ✅ GetClaudeDir() returns ~/.claude path
- ✅ ConfigFile struct properly defined
- ✅ GoDoc comments complete
- ✅ Follows existing config package patterns

**Next Tasks After This**:
- TASK-001.2: Implement GetConfig() method (2h)
- TASK-001.3: Add JSON validation (2h)
- TASK-001.4: Atomic write with backup (3h)

### Implementation Phases

#### Phase 1: Foundation (Week 1) - NEXT
**Goal**: Backend infrastructure for config management

**Stories**:
1. **STORY-001: Backend Config Management** [2 days]
   - TASK-001.1: Create config/claude.go structure [1h] **← RECOMMENDED START**
   - TASK-001.2: Implement GetConfig() method [2h]
   - TASK-001.3: Add JSON validation support [2h]
   - TASK-001.4: Implement atomic write with backup [3h]

2. **STORY-003: Protocol Buffer Definitions** [1 day]
   - TASK-003.1: Add config RPCs to session.proto [2h]
   - TASK-003.3: Generate Go and TypeScript bindings [1h]

**Milestone 1**: Backend Complete - Can read/write configs programmatically

#### Phase 2: TUI Implementation (Week 2)
**Goal**: Build overlay components for config editing

**Stories**:
3. **STORY-004: Config Editor Overlay** [3 days]
   - Create ui/overlay/configEditorOverlay.go
   - Implement syntax highlighting (Markdown/JSON)
   - Add edit mode with validation
   - Wire up save/cancel actions

**Milestone 2**: TUI MVP - Can view/edit configs in TUI

#### Phase 3: Web UI Implementation (Week 3)
**Goal**: Build web components with feature parity to TUI

**Stories**:
4. **STORY-006: Config Editor Web Component** [3 days]
   - Monaco editor integration
   - Syntax highlighting and validation
   - Real-time error feedback

**Milestone 3**: Web UI Complete - Feature parity with TUI

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

**Last Updated**: 2025-11-10
**Current Phase**: Web UI Implementation (40% complete)
**Next Milestone**: Web UI MVP with session creation wizard

**Active Projects**:
1. **Web UI Implementation** (P1 - In Progress)
2. **Claude Config Editor** (P2 - Planned, ready to start)

**Critical Dependencies**:
- Web UI MVP is the current priority (P1)
- Claude Config Editor can start after Web UI Session Creation complete
- Test stabilization work deferred until major features complete