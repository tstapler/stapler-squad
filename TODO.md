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

## Future Priorities (After Emergency Resolution)

### Medium Term (Next 3-5 Sessions):
- [ ] **Session Health Check Integration** - Evaluate health check system
- [ ] **Filtering System Enhancement** - Tag vs Category analysis
- [ ] **Help System Consolidation** - Compare current vs unused help generator

### Long Term (Future Sessions):
- [ ] **Dead Code Removal** - Clean up unused constructors and test mocks
- [ ] **Performance Optimization** - Large directory tree handling improvements
- [ ] **Advanced Features** - Network path support, fuzzy path matching

---

## Context Notes

**Last Updated**: 2025-01-17
**Current Phase**: Emergency Build Stabilization
**Next Milestone**: Restore compilation and test execution capability

**Critical Dependencies**:
- Build failures must be resolved before any other development work
- Test stabilization required for production deployment confidence
- All major feature work is complete and functional (when builds work)