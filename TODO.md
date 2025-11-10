# Claude Squad - Current Priority Tasks

## 🚧 IN PROGRESS: Web UI Implementation

**Status**: Foundation complete (Stories 1-2), Session Creation pending
**Priority**: P1 - Core user workflows, production deployment pending
**Progress**: 40% complete (2 of 5 stories)

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

### 🎯 Next Atomic Task: Story 3.1 - Session Creation Wizard

**Task**: Create Multi-Step Session Creation Form
**Estimated Time**: 3 hours
**Priority**: P1 - Unblocks core user workflow
**Context Boundary**: 3 files (page.tsx, SessionWizard.tsx, sessionSchema.ts) ✅

**Implementation Steps**:
1. Install dependencies: `zod`, `react-hook-form`, `@hookform/resolvers`
2. Create `web-app/src/app/sessions/new/page.tsx` - Creation page route
3. Create `web-app/src/components/sessions/SessionWizard.tsx` - Multi-step wizard component
4. Create `web-app/src/lib/validation/sessionSchema.ts` - Zod validation schema
5. Implement three wizard steps:
   - Step 1: Basic Info (title, category)
   - Step 2: Repository (path, workingDir, branch)
   - Step 3: Configuration (program, prompt, autoYes)
6. Integrate with `useSessionService.createSession` hook
7. Add success/error feedback with navigation
8. Test complete session creation flow

**Context Files to Understand**:
- `web-app/src/gen/session/v1/types_pb.ts` - CreateSessionRequest structure
- `ui/overlay/sessionSetup.go` - TUI session creation reference
- `web-app/src/lib/hooks/useSessionService.ts` - createSession implementation

**Success Criteria**:
- ✅ Three-step wizard with progress indicator
- ✅ Field validation prevents invalid data
- ✅ Form state persists across steps
- ✅ Success navigates to session list
- ✅ Error handling with retry

**Dependencies**: Story 1 complete ✅

### ⏸️ Pending Stories:

#### Story 3 - Session Creation Wizard (NOT STARTED - 3 tasks, 7h)
- [ ] Task 3.1: Multi-step form with validation (3h) **← NEXT**
- [ ] Task 3.2: Path discovery and auto-fill (2h)
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
- **Build Status**: ✅ Builds successfully (~1.5s)
- **Bundle Size**: 138KB (main), 153KB (wizard route)
- **Routes**: Home (/), Review Queue (/review-queue), New Session (/sessions/new)

### Technical Debt:
- [ ] **Zero test coverage** - No unit/integration tests yet
- [ ] **Terminal streaming** - Placeholder implementation only
- [ ] **Diff updates** - Mock data, needs real API integration
- [ ] **Static export limitation** - Can't share direct session detail URLs

**See**:
- [Web UI Enhancement Epic](docs/tasks/web-ui-enhancements.md)
- [Implementation Status Report](docs/tasks/web-ui-implementation-status.md)

---

## DEFERRED: Critical Test Timeouts

**Status**: Tests compile successfully, deferred while building new features
**Priority**: P2 - Resume after web server MVP

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
**Priority**: P2 - Resume after web server MVP

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

## PLANNED: Claude Config Editor & Session History Viewer

**Status**: Planning Complete - Ready for Implementation
**Priority**: P2 - After Web UI MVP complete
**Epic ID**: FEATURE-001
**Estimated Effort**: 3-4 weeks (1 engineer)
**Progress**: 0% (Planning phase)

### Overview

Add comprehensive Claude Code configuration management and session history viewing to Claude Squad:
- View/edit Claude configuration files (.claude/CLAUDE.md, settings.json, agents.md)
- Browse historical Claude sessions from ~/.claude/history.jsonl
- Launch new sessions based on historical sessions
- Review past work through conversation history

**Strategic Value**:
- **Productivity**: Resume interrupted work by launching from history
- **Transparency**: Direct config access eliminates manual file editing
- **Workflow Integration**: Tight Claude Code + Claude Squad integration

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
**Goal**: Backend infrastructure for config and history management

**Stories**:
1. **STORY-001: Backend Config Management** [2 days]
   - TASK-001.1: Create config/claude.go structure [1h] **← RECOMMENDED START**
   - TASK-001.2: Implement GetConfig() method [2h]
   - TASK-001.3: Add JSON validation support [2h]
   - TASK-001.4: Implement atomic write with backup [3h]

2. **STORY-002: History Parser Implementation** [2 days]
   - TASK-002.1: Create session/history.go structure [1h]
   - TASK-002.2: Implement JSONL streaming parser [3h]
   - TASK-002.3: Build SessionHistoryIndex [2h]
   - TASK-002.4: Add project grouping logic [2h]

3. **STORY-003: Protocol Buffer Definitions** [1 day]
   - TASK-003.1: Add config RPCs to session.proto [2h]
   - TASK-003.2: Add history RPCs to session.proto [2h]
   - TASK-003.3: Generate Go and TypeScript bindings [1h]

**Milestone 1**: Backend Complete - Can read/write configs and parse history programmatically

#### Phase 2: TUI Implementation (Week 2)
**Goal**: Build overlay components for config editing and history browsing

**Stories**:
4. **STORY-004: Config Editor Overlay** [3 days]
   - Create ui/overlay/configEditorOverlay.go
   - Implement syntax highlighting (Markdown/JSON)
   - Add edit mode with validation
   - Wire up save/cancel actions

5. **STORY-005: History Browser Overlay** [2 days]
   - Create ui/overlay/sessionHistoryBrowserOverlay.go
   - Implement list rendering with grouping
   - Add date range filters and search
   - Enable session launch from history

**Milestone 2**: TUI MVP - Can view/edit configs and browse history in TUI

#### Phase 3: Web UI Implementation (Week 3)
**Goal**: Build web components with feature parity to TUI

**Stories**:
6. **STORY-006: Config Editor Web Component** [3 days]
   - Monaco editor integration
   - Syntax highlighting and validation
   - Real-time error feedback

7. **STORY-007: History Browser Web Component** [2 days]
   - Virtual scrolling for performance
   - Filtering and search UI
   - Project-based grouping

**Milestone 3**: Web UI Complete - Feature parity with TUI

#### Phase 4: Integration & Polish (Week 4)
**Goal**: Complete feature integration and testing

**Stories**:
8. **STORY-008: Launch Session from History** [2 days]
9. **STORY-009: Conversation Viewer** [2 days]
10. **STORY-010: Testing & Documentation** [1 day]

**Milestone 4**: Feature Complete - Ready for production release

### Technical Notes

**Dependencies Already Available**:
- ✅ `github.com/gofrs/flock` - File locking (already in go.mod)
- ✅ `github.com/mattn/go-sqlite3` - SQLite driver (already in go.mod)
- Need to add: `github.com/xeipuuv/gojsonschema` - JSON validation

**Integration Points**:
- Reuse existing config directory resolution (config/config.go)
- Follow overlay pattern from ui/overlay/sessionSetup.go
- Extend proto/session/v1/session.proto with new RPCs
- Add key bindings in keys/keys.go (C for config, H for history)

**Context Boundaries**:
- Phase 1 tasks: 1-3 files each, 1-3 hours
- All tasks respect AIC framework (3-5 files max, single responsibility)
- No task requires > 800 lines of total context

**Testing Strategy**:
- Unit tests for all backend services (>80% coverage)
- Integration tests for file operations
- TUI overlay tests with teatest framework
- E2E tests with Playwright for web UI

**See Full Details**:
- [Complete Feature Plan](docs/tasks/claude-config-editor.md) - 70KB, 2,145 lines
- [Implementation Summary](docs/tasks/claude-config-editor-summary.md) - 12KB, 348 lines

---

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