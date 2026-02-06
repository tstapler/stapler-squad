# Epic: App Architecture Refactoring

## Epic Overview

### Goal
Decompose the monolithic `app/app.go` god object (1,938 lines, 50 functions) into focused, maintainable components following SOLID principles and BubbleTea architecture patterns.

### Value Proposition
- **Maintainability**: Reduce change risk in most frequently modified file (11 commits/90 days)
- **Testability**: Enable isolated unit testing of application components
- **Developer Experience**: Improve code navigation and reduce cognitive load
- **Architecture Quality**: Establish patterns for future feature development

### Success Metrics
- `app/app.go` reduced to <500 lines focusing only on BubbleTea orchestration
- 4+ focused components with single responsibilities
- Test coverage increase from current baseline to 80%+
- No regression in existing functionality or performance benchmarks

### Business Impact
- Faster feature development velocity
- Reduced bug introduction rate
- Improved developer onboarding experience
- Better code review efficiency

---

## Story Breakdown

### Story 1: State Management Extraction (1-2 weeks)
**Value**: Centralized, testable state management with clear transitions
**Dependencies**: None (foundation story)
**Integration**: New state manager integrates with existing BubbleTea model

### Story 2: Session Controller Extraction (1-2 weeks)
**Value**: Isolated session operations with proper error handling
**Dependencies**: Story 1 (requires state management)
**Integration**: Session operations routed through dedicated controller

### Story 3: UI Coordination Layer (1-2 weeks)
**Value**: Clean separation of UI orchestration from business logic
**Dependencies**: Stories 1, 2 (requires state and session management)
**Integration**: UI coordinator manages component communication

### Story 4: Terminal and System Integration (1 week)
**Value**: Isolated system concerns with proper abstractions
**Dependencies**: Stories 1-3 (requires clean interfaces)
**Integration**: System services injected into main application

---

## Atomic Task Decomposition

### Story 1: State Management Extraction

#### Task 1.1: Create State Manager Foundation (3h)
**Scope**: Extract state definitions and create state manager interface
**Files**:
- `app/state/manager.go` (new)
- `app/state/types.go` (new)
- `app/app.go` (modify imports/types)
**Context**: Current state constants and transitions in app.go:36-58
**Success Criteria**:
- State manager interface defined with all transitions
- State constants moved to dedicated types file
- Zero compilation errors
**Testing**: Unit tests for state transition validation
**Dependencies**: None

#### Task 1.2: Implement Core State Transitions (4h)
**Scope**: Move state transition logic to state manager
**Files**:
- `app/state/manager.go` (implement)
- `app/state/manager_test.go` (new)
- `app/app.go` (refactor Update method)
**Context**: State transition logic in app.go Update method (lines ~800-1200)
**Success Criteria**:
- All state transitions handled by state manager
- State validation and error handling preserved
- BubbleTea integration maintained
**Testing**: State transition matrix tests, invalid state handling
**Dependencies**: Task 1.1

#### Task 1.3: Extract State-Dependent UI Logic (3h)
**Scope**: Move state-specific view logic to state manager
**Files**:
- `app/state/manager.go` (extend)
- `app/app.go` (simplify View method)
- `app/state/manager_test.go` (extend)
**Context**: State-dependent view rendering in app.go View method
**Success Criteria**:
- State manager provides view directives
- View method simplified to orchestration only
- All existing UI states preserved
**Testing**: View state integration tests
**Dependencies**: Task 1.2

### Story 2: Session Controller Extraction

#### Task 2.1: Define Session Controller Interface (2h)
**Scope**: Create session operations interface and basic structure
**Files**:
- `app/session/controller.go` (new)
- `app/session/operations.go` (new)
- `app/app.go` (modify struct)
**Context**: Session operation methods in app.go (lines ~1300-1700)
**Success Criteria**:
- Session controller interface defined
- Dependency injection structure prepared
- Method signatures preserve existing behavior
**Testing**: Interface compliance tests
**Dependencies**: Task 1.2 (requires state management)

#### Task 2.2: Extract Session Creation Logic (4h)
**Scope**: Move session creation handlers to controller
**Files**:
- `app/session/controller.go` (implement creation)
- `app/session/controller_test.go` (new)
- `app/app.go` (delegate to controller)
**Context**: Session creation logic in handleAdvancedSessionSetup.go and app.go
**Success Criteria**:
- Session creation fully delegated to controller
- Error handling and validation preserved
- Async session creation maintained
**Testing**: Session creation scenarios, error conditions
**Dependencies**: Task 2.1

#### Task 2.3: Extract Session Lifecycle Management (3h)
**Scope**: Move start/stop/delete operations to controller
**Files**:
- `app/session/controller.go` (extend lifecycle)
- `app/session/controller_test.go` (extend)
- `app/app.go` (remove lifecycle methods)
**Context**: Session lifecycle methods in app.go
**Success Criteria**:
- All session operations through controller
- State transitions properly coordinated
- Cleanup and error handling preserved
**Testing**: Lifecycle operation tests, state coordination
**Dependencies**: Task 2.2

### Story 3: UI Coordination Layer

#### Task 3.1: Create UI Coordinator Structure (2h)
**Scope**: Define UI coordinator for component management
**Files**:
- `app/ui/coordinator.go` (new)
- `app/ui/types.go` (new)
- `app/app.go` (modify struct)
**Context**: UI component management in app.go home struct (lines ~108-140)
**Success Criteria**:
- UI coordinator interface defined
- Component lifecycle management planned
- BubbleTea message routing preserved
**Testing**: Coordinator initialization tests
**Dependencies**: Tasks 1.3, 2.1 (requires state and session interfaces)

#### Task 3.2: Extract Component Orchestration (4h)
**Scope**: Move UI component management to coordinator
**Files**:
- `app/ui/coordinator.go` (implement)
- `app/ui/coordinator_test.go` (new)
- `app/app.go` (delegate UI operations)
**Context**: Component initialization and management in newHome function
**Success Criteria**:
- UI components managed by coordinator
- Message routing maintained
- Component lifecycle preserved
**Testing**: Component orchestration tests, message routing
**Dependencies**: Task 3.1

#### Task 3.3: Extract Overlay Management (3h)
**Scope**: Move overlay state and management to coordinator
**Files**:
- `app/ui/coordinator.go` (extend overlays)
- `app/ui/coordinator_test.go` (extend)
- `app/app.go` (remove overlay logic)
**Context**: Overlay state management in app.go (stateNew, statePrompt, etc.)
**Success Criteria**:
- Overlay lifecycle through coordinator
- Modal state management centralized
- Existing overlay behavior preserved
**Testing**: Overlay state tests, modal interactions
**Dependencies**: Task 3.2

### Story 4: Terminal and System Integration

#### Task 4.1: Extract Terminal Management (2h)
**Scope**: Create terminal service for size and capability management
**Files**:
- `app/system/terminal.go` (new)
- `app/system/terminal_test.go` (new)
- `app/app.go` (use terminal service)
**Context**: Terminal size handling in app.go (lines ~103-106, terminal_size.go)
**Success Criteria**:
- Terminal concerns in dedicated service
- Size detection and handling isolated
- IntelliJ compatibility preserved
**Testing**: Terminal size detection tests
**Dependencies**: Task 3.2 (requires UI coordinator)

#### Task 4.2: Extract System Integration (3h)
**Scope**: Create system service for external integrations
**Files**:
- `app/system/service.go` (new)
- `app/system/service_test.go` (new)
- `app/app.go` (use system service)
**Context**: System integration logic scattered in app.go
**Success Criteria**:
- System operations isolated
- Service interfaces well-defined
- Error handling preserved
**Testing**: System integration tests, error scenarios
**Dependencies**: Task 4.1

#### Task 4.3: Final Integration and Cleanup (2h)
**Scope**: Complete integration and remove obsolete code
**Files**:
- `app/app.go` (final cleanup)
- `app/integration_test.go` (new)
- All component files (final validation)
**Context**: Remaining orchestration logic in app.go
**Success Criteria**:
- app.go <500 lines, orchestration only
- All tests passing
- Performance benchmarks maintained
**Testing**: Full integration test suite
**Dependencies**: All previous tasks

---

## Dependency Visualization

```
Story 1 (State Management)
├─ Task 1.1: State Foundation → Task 1.2: Transitions → Task 1.3: UI Logic

Story 2 (Session Controller)
├─ Task 2.1: Interface (depends on 1.2)
├─ Task 2.2: Creation (depends on 2.1)
└─ Task 2.3: Lifecycle (depends on 2.2)

Story 3 (UI Coordination)
├─ Task 3.1: Coordinator (depends on 1.3, 2.1)
├─ Task 3.2: Orchestration (depends on 3.1)
└─ Task 3.3: Overlays (depends on 3.2)

Story 4 (System Integration)
├─ Task 4.1: Terminal (depends on 3.2)
├─ Task 4.2: System Service (depends on 4.1)
└─ Task 4.3: Final Integration (depends on all)
```

### Parallel Execution Opportunities
- **Phase 1**: Task 1.1 (standalone)
- **Phase 2**: Tasks 1.2, 1.3 (sequential within story)
- **Phase 3**: Tasks 2.1, 2.2, 2.3 (sequential, parallel to phase 2 completion)
- **Phase 4**: Tasks 3.1, 3.2, 3.3 (sequential)
- **Phase 5**: Tasks 4.1, 4.2, 4.3 (sequential)

---

## Context Preparation Guide

### For State Management Tasks (Story 1)
**Required Understanding**:
- BubbleTea model/update pattern
- Current state constants and transitions
- State-dependent UI rendering logic
- Error state handling patterns

**Key Files to Study**:
- `app/app.go` lines 36-58 (state definitions)
- `app/app.go` Update method (state transitions)
- `app/app.go` View method (state-dependent rendering)

### For Session Controller Tasks (Story 2)
**Required Understanding**:
- Session lifecycle and operations
- Error handling patterns
- Async operation management
- Storage integration patterns

**Key Files to Study**:
- `app/app.go` session-related methods
- `app/handleAdvancedSessionSetup.go`
- `session/instance.go` (domain model)
- `session/storage.go` (persistence)

### For UI Coordination Tasks (Story 3)
**Required Understanding**:
- BubbleTea component patterns
- Message routing and handling
- Overlay state management
- Component lifecycle

**Key Files to Study**:
- `app/app.go` newHome function
- `ui/overlay/` directory structure
- BubbleTea component patterns in existing UI

### For System Integration Tasks (Story 4)
**Required Understanding**:
- Terminal size detection
- System service patterns
- Error handling for system operations
- IntelliJ compatibility requirements

**Key Files to Study**:
- `app/terminal_size.go`
- System integration points in `app/app.go`
- Error handling patterns

---

## INVEST Validation Matrix

| Task | Independent | Negotiable | Valuable | Estimable | Small | Testable |
|------|------------|-----------|----------|-----------|-------|----------|
| 1.1  | ✅ No deps | ✅ Interface design | ✅ Foundation | ✅ 3h confident | ✅ Single focus | ✅ Unit testable |
| 1.2  | ✅ Clean interface | ✅ Implementation approach | ✅ Core functionality | ✅ 4h bounded | ✅ State transitions | ✅ Behavior tests |
| 1.3  | ✅ Uses stable API | ✅ UI approach flexible | ✅ Simplifies main | ✅ 3h UI focused | ✅ View logic only | ✅ Integration tests |
| 2.1  | ✅ Interface only | ✅ Design negotiable | ✅ Clean architecture | ✅ 2h interface | ✅ Just interface | ✅ Contract tests |
| 2.2  | ✅ Stable interfaces | ✅ Implementation style | ✅ Core session ops | ✅ 4h complex | ✅ Creation only | ✅ Creation scenarios |
| 2.3  | ✅ Creation complete | ✅ Lifecycle approach | ✅ Complete delegation | ✅ 3h lifecycle | ✅ Lifecycle only | ✅ Lifecycle tests |
| 3.1  | ✅ Clean dependencies | ✅ Coordinator design | ✅ UI organization | ✅ 2h structure | ✅ Structure only | ✅ Setup tests |
| 3.2  | ✅ Coordinator ready | ✅ Orchestration style | ✅ Component management | ✅ 4h orchestration | ✅ Management focus | ✅ Message routing |
| 3.3  | ✅ Orchestration stable | ✅ Overlay approach | ✅ Modal management | ✅ 3h overlay focus | ✅ Overlays only | ✅ Modal state tests |
| 4.1  | ✅ UI coordinator ready | ✅ Terminal handling | ✅ System abstraction | ✅ 2h terminal | ✅ Terminal only | ✅ Size detection |
| 4.2  | ✅ Terminal service ready | ✅ Service design | ✅ System isolation | ✅ 3h services | ✅ System focus | ✅ Integration tests |
| 4.3  | ✅ All components ready | ✅ Integration approach | ✅ Complete refactor | ✅ 2h cleanup | ✅ Integration only | ✅ Full test suite |

---

## Integration Checkpoints

### Checkpoint 1: State Management Foundation (After Task 1.3)
**Validation**:
- State transitions work correctly
- UI rendering preserved
- Performance maintained
- All existing tests pass

### Checkpoint 2: Session Operations Complete (After Task 2.3)
**Validation**:
- Session creation/management works
- Error handling preserved
- Async operations maintained
- Integration with state management

### Checkpoint 3: UI Coordination Active (After Task 3.3)
**Validation**:
- Component management functional
- Overlay system working
- Message routing preserved
- UI responsiveness maintained

### Checkpoint 4: Complete Integration (After Task 4.3)
**Validation**:
- app.go under 500 lines
- All functionality preserved
- Performance benchmarks pass
- Full test suite passing
- Ready for production deployment

---

## Success Criteria Summary

### Technical Criteria
- ✅ app.go reduced from 1,938 to <500 lines
- ✅ 4 focused components with single responsibilities
- ✅ All existing functionality preserved
- ✅ Performance benchmarks maintained
- ✅ Test coverage increased to 80%+

### Architectural Criteria
- ✅ SOLID principles applied
- ✅ BubbleTea patterns preserved
- ✅ Clean dependency injection
- ✅ Testable component interfaces
- ✅ Clear separation of concerns

### Quality Criteria
- ✅ Zero regression in functionality
- ✅ Improved code maintainability
- ✅ Enhanced developer experience
- ✅ Reduced cognitive complexity
- ✅ Foundation for future features