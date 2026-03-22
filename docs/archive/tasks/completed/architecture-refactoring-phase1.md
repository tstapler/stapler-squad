# Phase 1 Architecture Refactoring - Feature Plan

**Date**: 2025-12-05  
**Phase**: P0 Critical Issues  
**Timeline**: 2-3 weeks (Sprints 1-2)  
**Based On**: [ARCHITECTURE_REVIEW.md](../ARCHITECTURE_REVIEW.md), [ARCHITECTURE_IMPLEMENTATION_PLAN.md](../ARCHITECTURE_IMPLEMENTATION_PLAN.md)  
**Target Score**: Improve from 7.8/10 to 8.5/10 (Phase 1 only)

---

## Executive Summary

This plan addresses the two **P0 critical architectural issues** identified in the architecture review:

1. **God Object Pattern** - `home` struct with 30+ fields violating Single Responsibility Principle (SRP)
2. **Framework Coupling** - BubbleTea types (`tea.Model`, `tea.Cmd`) leaking into domain layer

**Success Criteria**:
- Reduce `home` struct from 30+ fields to <10 fields
- Extract 4 focused services (SessionManagement, Navigation, Filtering, UICoordination)
- Replace `tea.Model`/`tea.Cmd` in domain with domain events
- Zero test regressions (all 368 Go files compile)
- Maintain navigation responsiveness (no performance degradation)
- 80%+ test coverage for new services

---

## Table of Contents

1. [Epic Breakdown](#epic-breakdown)
2. [Story 1: Extract Core Services](#story-1-extract-core-services)
3. [Story 2: Decouple Framework Dependencies](#story-2-decouple-framework-dependencies)
4. [Story 3: Integration & Validation](#story-3-integration--validation)
5. [Known Issues & Bug Prevention](#known-issues--bug-prevention)
6. [Integration Checkpoints](#integration-checkpoints)
7. [Context Preparation Guide](#context-preparation-guide)
8. [Dependency Visualization](#dependency-visualization)
9. [Git Commit Strategy](#git-commit-strategy)
10. [Risk Mitigation](#risk-mitigation)

---

## Epic Breakdown

### Epic: Phase 1 Architecture Refactoring
**Duration**: 2-3 weeks (10-15 working days)  
**Priority**: P0 (Critical)  
**Business Value**: Improved maintainability, reduced complexity, better testability

**Stories**:
1. Extract Core Services (1 week, 5 days)
2. Decouple Framework Dependencies (1 week, 5 days)
3. Integration & Validation (3-5 days)

**Success Metrics**:
- Cyclomatic complexity of `home.Update()` reduced from 30+ to <10
- God Object score: `home` fields reduced from 30+ to <10
- Test coverage increased to 80%+ for new services
- Zero breaking changes to public APIs
- All 368 Go files compile successfully

---

## Story 1: Extract Core Services

**Story**: As a developer, I want the `home` struct refactored into focused services so that the codebase is more maintainable and testable.

**Priority**: P0  
**Effort**: 5 days (1 week)  
**Dependencies**: None (can start immediately)

### Acceptance Criteria

**AC1**: SessionManagementService handles all session lifecycle operations (create, kill, attach, resume)  
**AC2**: NavigationService handles list navigation with debouncing (150ms delay)  
**AC3**: FilteringService handles search and filter operations  
**AC4**: UICoordinationService manages overlays, errors, status bar  
**AC5**: `home` struct reduced to <10 fields (facade coordinating services)  
**AC6**: All existing tests pass with zero regressions  
**AC7**: New services have 80%+ unit test coverage

---

### Task 1.1: Extract SessionManagementService

**INVEST Validation**:
- **I**ndependent: No dependencies on other tasks
- **N**egotiable: Core logic is clear, implementation details flexible
- **V**aluable: Isolates session lifecycle logic for better testability
- **E**stimable: 4-6 hours
- **S**mall: 3-5 files, single responsibility
- **T**estable: Can verify all session operations work independently

**Effort**: 4-6 hours  
**Files** (3-5):
- **Create**: `app/services/session_management.go` (new service)
- **Create**: `app/services/session_management_test.go` (unit tests)
- **Modify**: `app/app.go` (extract logic from `home`)
- **Modify**: `app/dependencies.go` (add service getter)
- **Modify**: `app/test_helpers.go` (update test setup)

**Context**: This service encapsulates session creation, killing, attachment, and resume operations currently scattered in `home.Update()`.

**Implementation Steps**:

1. **Create Service Interface** (30 min)
```go
// File: app/services/session_management.go
package services

type SessionManagementService interface {
    CreateSession(opts session.InstanceOptions) error
    KillSession(title string) error
    AttachSession(title string) error
    ResumeSession(title string) error
    GetActiveSessionsCount() int
}
```

2. **Implement Service** (2 hours)
```go
type sessionManagementService struct {
    storage           *session.Storage
    sessionController appsession.Controller
    list              *ui.List
    errorHandler      func(error) tea.Cmd
}

func NewSessionManagementService(
    storage *session.Storage,
    controller appsession.Controller,
    list *ui.List,
    errorHandler func(error) tea.Cmd,
) SessionManagementService {
    return &sessionManagementService{
        storage:           storage,
        sessionController: controller,
        list:              list,
        errorHandler:      errorHandler,
    }
}

func (s *sessionManagementService) CreateSession(opts session.InstanceOptions) error {
    // Extract session creation logic from home.Update()
    // Validation logic
    if s.list.NumInstances() >= GlobalInstanceLimit {
        return errors.New("instance limit reached")
    }
    
    // Delegate to session controller
    result := s.sessionController.NewSession()
    if !result.Success {
        return result.Error
    }
    
    return nil
}
```

3. **Add Unit Tests** (1-2 hours)
```go
// File: app/services/session_management_test.go
package services

func TestSessionManagementService_CreateSession(t *testing.T) {
    // Test successful session creation
    // Test instance limit enforcement
    // Test validation errors
}

func TestSessionManagementService_KillSession(t *testing.T) {
    // Test successful session termination
    // Test error handling for non-existent session
}
```

4. **Update Dependencies** (30 min)
```go
// File: app/dependencies.go
type Dependencies interface {
    // ... existing methods
    GetSessionManagementService() services.SessionManagementService
}

func (p *ProductionDependencies) GetSessionManagementService() services.SessionManagementService {
    if p.sessionManagementService == nil {
        p.sessionManagementService = services.NewSessionManagementService(
            p.GetStorage(),
            p.GetSessionController(),
            p.GetList(),
            nil, // Error handler injected later
        )
    }
    return p.sessionManagementService
}
```

5. **Refactor home to Use Service** (1 hour)
```go
// File: app/app.go
type home struct {
    // ... reduced fields
    sessionManagement services.SessionManagementService
}

func (h *home) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "n":
        // Old: Complex inline logic
        // New: Delegate to service
        err := h.sessionManagement.CreateSession(session.InstanceOptions{})
        if err != nil {
            return h, h.handleError(err)
        }
        return h, nil
    }
}
```

**Validation Commands**:
```bash
# Unit tests for service
go test ./app/services -run TestSessionManagementService -v

# Integration tests (ensure no regression)
go test ./app -run TestSessionCreation -v
go test ./app -run TestSessionOperations -v

# Full app test suite
go test ./app -v
```

**Known Issues - Task 1.1**:

#### Bug Risk: Race Condition in Concurrent Session Creation [SEVERITY: High]

**Description**: If multiple goroutines call `CreateSession()` simultaneously, the instance count check may allow exceeding the limit.

**Mitigation**:
- Add mutex protection around instance count check
- Use atomic operations for counter
- Add integration test with concurrent creation attempts

**Prevention**:
```go
type sessionManagementService struct {
    mu sync.Mutex
    // ... other fields
}

func (s *sessionManagementService) CreateSession(opts session.InstanceOptions) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Atomic check and create
    if s.list.NumInstances() >= GlobalInstanceLimit {
        return errors.New("instance limit reached")
    }
    
    result := s.sessionController.NewSession()
    // ...
}
```

**Test Case**:
```go
func TestSessionManagementService_ConcurrentCreation(t *testing.T) {
    // Attempt to create sessions from 10 goroutines
    // Verify only up to limit succeed
}
```

---

### Task 1.2: Extract NavigationService

**INVEST Validation**:
- **I**ndependent: Depends only on Task 1.1 completion
- **N**egotiable: Debounce timing configurable, core logic fixed
- **V**aluable: Isolates navigation logic, improves responsiveness
- **E**stimable: 3-5 hours
- **S**mall: 3-5 files, focused on navigation
- **T**estable: Can verify navigation state changes independently

**Effort**: 3-5 hours  
**Files** (3-5):
- **Create**: `app/services/navigation.go`
- **Create**: `app/services/navigation_test.go`
- **Modify**: `app/app.go` (extract navigation logic)
- **Modify**: `app/dependencies.go` (add service getter)

**Context**: This service handles list navigation with debouncing to prevent expensive operations during fast scrolling.

**Implementation Steps**:

1. **Create Service Interface** (30 min)
```go
// File: app/services/navigation.go
package services

type NavigationService interface {
    NavigateUp() error
    NavigateDown() error
    NavigateToIndex(idx int) error
    NavigatePageUp() error
    NavigatePageDown() error
    NavigateHome() error
    NavigateEnd() error
    GetCurrentIndex() int
    GetVisibleItemsCount() int
}
```

2. **Implement Service with Debouncing** (2 hours)
```go
type navigationService struct {
    list          *ui.List
    responsiveNav *ResponsiveNavigationManager
    mu            sync.Mutex
}

func NewNavigationService(list *ui.List) NavigationService {
    return &navigationService{
        list:          list,
        responsiveNav: NewResponsiveNavigationManager(150 * time.Millisecond),
    }
}

func (n *navigationService) NavigateUp() error {
    n.mu.Lock()
    defer n.mu.Unlock()
    
    // Use debounced navigation
    return n.responsiveNav.ScheduleNavigation(func() error {
        return n.list.PrevInstance()
    })
}

func (n *navigationService) NavigateDown() error {
    n.mu.Lock()
    defer n.mu.Unlock()
    
    return n.responsiveNav.ScheduleNavigation(func() error {
        return n.list.NextInstance()
    })
}
```

3. **Add Debouncing Manager** (1 hour)
```go
// ResponsiveNavigationManager provides debounced navigation updates
type ResponsiveNavigationManager struct {
    debounceDelay time.Duration
    lastNav       time.Time
    mu            sync.Mutex
}

func NewResponsiveNavigationManager(debounceDelay time.Duration) *ResponsiveNavigationManager {
    return &ResponsiveNavigationManager{
        debounceDelay: debounceDelay,
    }
}

func (r *ResponsiveNavigationManager) ScheduleNavigation(navFn func() error) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    now := time.Now()
    if now.Sub(r.lastNav) < r.debounceDelay {
        // Skip expensive operations during rapid navigation
        return nil
    }
    
    r.lastNav = now
    return navFn()
}
```

4. **Add Unit Tests** (1-2 hours)
```go
func TestNavigationService_NavigateUp(t *testing.T) {
    // Test upward navigation
    // Test boundary conditions (at top)
}

func TestNavigationService_Debouncing(t *testing.T) {
    // Test rapid navigation only triggers updates after debounce period
    // Test debounce delay timing
}
```

**Validation Commands**:
```bash
# Unit tests
go test ./app/services -run TestNavigationService -v

# Performance benchmark
go test -bench=BenchmarkNavigationPerformance ./app
```

**Known Issues - Task 1.2**:

#### Bug Risk: Navigation Index Out of Bounds [SEVERITY: Medium]

**Description**: After filtering or deleting sessions, navigation may access invalid indices.

**Mitigation**:
- Always validate index bounds before navigation
- Update index after filter changes
- Add integration tests with dynamic list changes

**Prevention**:
```go
func (n *navigationService) NavigateToIndex(idx int) error {
    n.mu.Lock()
    defer n.mu.Unlock()
    
    // Validate bounds
    if idx < 0 || idx >= n.list.NumInstances() {
        return errors.New("invalid navigation index")
    }
    
    return n.list.SetSelectedInstance(idx)
}
```

---

### Task 1.3: Extract FilteringService

**INVEST Validation**:
- **I**ndependent: Can run in parallel with Task 1.2
- **N**egotiable: Filter logic clear, search algorithm negotiable
- **V**aluable: Isolates filtering/search logic for better maintainability
- **E**stimable: 3-4 hours
- **S**mall: 3-4 files, focused on filtering
- **T**estable: Can verify filter results independently

**Effort**: 3-4 hours  
**Files** (3-4):
- **Create**: `app/services/filtering.go`
- **Create**: `app/services/filtering_test.go`
- **Modify**: `app/app.go` (extract filtering logic)
- **Modify**: `app/dependencies.go` (add service getter)

**Implementation Steps**:

1. **Create Service Interface** (20 min)
```go
// File: app/services/filtering.go
package services

type FilteringService interface {
    TogglePausedFilter() bool
    StartSearch() error
    UpdateSearchQuery(query string) []*session.Instance
    ClearSearch()
    IsSearchActive() bool
    GetSearchQuery() string
    GetFilteredCount() int
}
```

2. **Implement Service** (1-2 hours)
```go
type filteringService struct {
    list         *ui.List
    searchActive bool
    searchQuery  string
    mu           sync.RWMutex
}

func NewFilteringService(list *ui.List) FilteringService {
    return &filteringService{
        list:         list,
        searchActive: false,
        searchQuery:  "",
    }
}

func (f *filteringService) TogglePausedFilter() bool {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    f.list.TogglePausedFilter()
    return f.list.IsPausedFilterActive()
}

func (f *filteringService) UpdateSearchQuery(query string) []*session.Instance {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    f.searchQuery = query
    f.list.FilterBySearch(query)
    return f.list.GetVisibleItems()
}
```

3. **Add Unit Tests** (1 hour)
```go
func TestFilteringService_TogglePausedFilter(t *testing.T) {
    // Test filter toggle
    // Verify filtered results
}

func TestFilteringService_Search(t *testing.T) {
    // Test search query updates
    // Test search result accuracy
}
```

**Validation Commands**:
```bash
go test ./app/services -run TestFilteringService -v
```

**Known Issues - Task 1.3**:

#### Bug Risk: Search Index Stale After Session Changes [SEVERITY: Medium]

**Description**: Search index may not update when sessions are added/removed, leading to stale results.

**Mitigation**:
- Subscribe to session change events
- Rebuild search index on changes
- Add integration test with dynamic session list

**Prevention**:
```go
func (f *filteringService) OnSessionsChanged() {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    // Rebuild search index
    if f.searchActive {
        f.list.FilterBySearch(f.searchQuery)
    }
}
```

---

### Task 1.4: Extract UICoordinationService

**INVEST Validation**:
- **I**ndependent: Can run in parallel with Tasks 1.2 and 1.3
- **N**egotiable: Overlay management pattern negotiable
- **V**aluable: Centralizes UI coordination logic
- **E**stimable: 3-5 hours
- **S**mall: 3-5 files, focused on UI coordination
- **T**estable: Can verify overlay state independently

**Effort**: 3-5 hours  
**Files** (3-5):
- **Create**: `app/services/ui_coordination.go`
- **Create**: `app/services/ui_coordination_test.go`
- **Modify**: `app/app.go` (extract UI coordination logic)
- **Modify**: `app/dependencies.go` (add service getter)

**Implementation Steps**:

1. **Create Service Interface** (30 min)
```go
// File: app/services/ui_coordination.go
package services

type UICoordinationService interface {
    ShowSessionSetupOverlay(overlay *overlay.SessionSetupOverlay)
    HideSessionSetupOverlay()
    ShowError(err error)
    ClearError()
    UpdateStatus(message string)
    IsOverlayActive() bool
}
```

2. **Implement Service** (2 hours)
```go
type uiCoordinationService struct {
    uiCoordinator appui.Coordinator
    menu          *ui.Menu
    statusBar     *ui.StatusBar
    errBox        *ui.ErrBox
    mu            sync.Mutex
}

func NewUICoordinationService(
    coordinator appui.Coordinator,
    menu *ui.Menu,
    statusBar *ui.StatusBar,
    errBox *ui.ErrBox,
) UICoordinationService {
    return &uiCoordinationService{
        uiCoordinator: coordinator,
        menu:          menu,
        statusBar:     statusBar,
        errBox:        errBox,
    }
}

func (u *uiCoordinationService) ShowSessionSetupOverlay(overlay *overlay.SessionSetupOverlay) {
    u.mu.Lock()
    defer u.mu.Unlock()
    
    u.uiCoordinator.SetSessionSetupOverlay(overlay)
}

func (u *uiCoordinationService) ShowError(err error) {
    u.mu.Lock()
    defer u.mu.Unlock()
    
    u.errBox.SetError(err)
    u.statusBar.SetMessage(err.Error())
}
```

3. **Add Unit Tests** (1-2 hours)
```go
func TestUICoordinationService_ShowOverlay(t *testing.T) {
    // Test overlay display
    // Test overlay hiding
}

func TestUICoordinationService_ErrorHandling(t *testing.T) {
    // Test error display
    // Test error clearing
}
```

**Validation Commands**:
```bash
go test ./app/services -run TestUICoordinationService -v
```

---

### Task 1.5: Refactor home to Facade

**INVEST Validation**:
- **I**ndependent: Depends on Tasks 1.1-1.4 completion
- **N**egotiable: Field organization negotiable, delegation pattern fixed
- **V**aluable: Achieves God Object reduction goal
- **E**stimable: 4-8 hours (complex refactoring)
- **S**mall: Primary focus on 2 files (app.go, dependencies.go)
- **T**estable: Can verify all existing tests pass

**Effort**: 4-8 hours  
**Files** (2-5):
- **Major Modify**: `app/app.go` (refactor `home` struct)
- **Modify**: `app/dependencies.go` (update service getters)
- **Update**: All test files in `app/` (verify no regressions)

**Context**: This is the culminating task that ties all services together and reduces `home` to a thin facade.

**Implementation Steps**:

1. **Refactor home Struct** (2-3 hours)
```go
// File: app/app.go (refactored)
package app

type home struct {
    // Context and cancellation
    ctx        context.Context
    cancelFunc context.CancelFunc
    
    // Core services (reduced from 30+ fields to <10)
    sessionManagement services.SessionManagementService
    navigation        services.NavigationService
    filtering         services.FilteringService
    uiCoordination    services.UICoordinationService
    
    // State management
    stateManager state.Manager
    
    // Configuration
    appConfig *config.Config
    appState  config.StateManager
    
    // BubbleTea framework
    bridge *cmd.Bridge
    
    // Terminal size (still needed for view rendering)
    termWidth  int
    termHeight int
}
```

2. **Update Constructor** (1 hour)
```go
func newHomeWithDependencies(deps Dependencies) *home {
    // Create services
    sessionManagement := deps.GetSessionManagementService()
    navigation := deps.GetNavigationService()
    filtering := deps.GetFilteringService()
    uiCoordination := deps.GetUICoordinationService()
    
    return &home{
        ctx:               deps.GetContext(),
        sessionManagement: sessionManagement,
        navigation:        navigation,
        filtering:         filtering,
        uiCoordination:    uiCoordination,
        stateManager:      deps.GetStateManager(),
        appConfig:         deps.GetAppConfig(),
        appState:          deps.GetAppState(),
        bridge:            deps.GetBridge(),
    }
}
```

3. **Refactor Update Method** (2-3 hours)
```go
// Update becomes a thin delegator
func (h *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return h.handleKeyMsg(msg)
    case tea.WindowSizeMsg:
        return h.handleWindowSizeMsg(msg)
    // ... other cases
    }
    return h, nil
}

func (h *home) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    key := msg.String()
    
    switch key {
    case "k", "up":
        // Delegate to navigation service
        if err := h.navigation.NavigateUp(); err != nil {
            h.uiCoordination.ShowError(err)
        }
        return h, nil
        
    case "j", "down":
        // Delegate to navigation service
        if err := h.navigation.NavigateDown(); err != nil {
            h.uiCoordination.ShowError(err)
        }
        return h, nil
        
    case "n":
        // Delegate to session management
        if err := h.sessionManagement.CreateSession(session.InstanceOptions{}); err != nil {
            h.uiCoordination.ShowError(err)
        }
        return h, nil
        
    case "f":
        // Delegate to filtering service
        h.filtering.TogglePausedFilter()
        return h, nil
        
    // ... other keys delegated to appropriate services
    }
}
```

4. **Update Test Helpers** (1-2 hours)
```go
// File: app/test_helpers.go
func NewMockDependenciesWithIsolatedStorage(t *testing.T) *MockDependencies {
    // ... existing setup
    
    // Add service initialization
    mockDeps.sessionManagementService = services.NewSessionManagementService(...)
    mockDeps.navigationService = services.NewNavigationService(...)
    mockDeps.filteringService = services.NewFilteringService(...)
    mockDeps.uiCoordinationService = services.NewUICoordinationService(...)
    
    return mockDeps
}
```

5. **Run Full Test Suite** (validation step)
```bash
# Run all tests
go test ./app -v -count=1

# Run integration tests
go test ./app -run TestRobustConfirmationModalFlow -v
go test ./app -run TestSessionCreationOverlayFix -v

# Verify UI still works (manual)
./stapler-squad
```

**Validation Commands**:
```bash
# Check field count reduction
grep -A 50 "^type home struct" app/app.go | wc -l
# Expected: <15 lines (down from 150+ lines)

# Check cyclomatic complexity
gocyclo -over 10 app/app.go
# Expected: home.Update() complexity <10

# Run full test suite
go test ./... -v

# Check test coverage
go test ./app/services -cover
# Expected: >80% coverage
```

**Known Issues - Task 1.5**:

#### Bug Risk: Service Initialization Order [SEVERITY: High]

**Description**: Services may depend on each other, causing initialization deadlocks or nil pointer panics.

**Mitigation**:
- Define clear initialization order in dependencies.go
- Use lazy initialization for optional dependencies
- Add integration test for full initialization

**Prevention**:
```go
// Dependencies.go - Define clear initialization order
func (p *ProductionDependencies) GetSessionManagementService() services.SessionManagementService {
    if p.sessionManagementService == nil {
        // Initialize dependencies first
        storage := p.GetStorage()        // 1. Storage
        controller := p.GetSessionController()  // 2. Controller
        list := p.GetList()              // 3. List
        
        // Then create service
        p.sessionManagementService = services.NewSessionManagementService(
            storage, controller, list, nil,
        )
    }
    return p.sessionManagementService
}
```

#### Bug Risk: Circular Dependencies Between Services [SEVERITY: High]

**Description**: Services may need to call each other, creating circular dependencies.

**Mitigation**:
- Use dependency injection for service-to-service communication
- Prefer events over direct service calls
- Draw dependency graph before implementation

**Prevention**:
```go
// BAD: Direct circular dependency
type NavigationService struct {
    filtering FilteringService  // Navigation depends on filtering
}
type FilteringService struct {
    navigation NavigationService  // Filtering depends on navigation
}

// GOOD: Use events or interfaces
type NavigationService struct {
    onNavigationComplete func()  // Callback, not direct dependency
}
```

---

## Story 2: Decouple Framework Dependencies

**Story**: As a developer, I want domain logic decoupled from BubbleTea framework so that the codebase is more flexible and testable.

**Priority**: P0  
**Effort**: 5 days (1 week)  
**Dependencies**: Story 1 must be complete

### Acceptance Criteria

**AC1**: Domain events defined (SessionCreated, SessionKilled, SessionStatusChanged)  
**AC2**: `SessionOperation` struct no longer uses `tea.Model` or `tea.Cmd`  
**AC3**: EventAdapter converts domain events to BubbleTea commands  
**AC4**: `home.Update()` uses adapter for event-to-command conversion  
**AC5**: All existing tests pass with zero regressions  
**AC6**: Domain layer has zero framework imports (verified by `go list`)

---

### Task 2.1: Define Domain Events

**INVEST Validation**:
- **I**ndependent: No dependencies (can start after Story 1)
- **N**egotiable: Event fields negotiable, event types fixed
- **V**aluable: Foundation for framework decoupling
- **E**stimable: 2-3 hours
- **S**mall: 2-3 files, focused on event definitions
- **T**estable: Can verify event creation and serialization

**Effort**: 2-3 hours  
**Files** (2-3):
- **Create**: `domain/events/session_events.go`
- **Create**: `domain/events/event.go`
- **Create**: `domain/events/events_test.go`

**Implementation Steps**:

1. **Create Event Interface** (30 min)
```go
// File: domain/events/event.go
package events

import "time"

// DomainEvent is the base interface for all domain events
type DomainEvent interface {
    EventType() string
    Timestamp() time.Time
    AggregateID() string  // For event sourcing
}

// BaseEvent provides common event fields
type BaseEvent struct {
    timestamp   time.Time
    aggregateID string
}

func NewBaseEvent(aggregateID string) BaseEvent {
    return BaseEvent{
        timestamp:   time.Now(),
        aggregateID: aggregateID,
    }
}

func (e BaseEvent) Timestamp() time.Time  { return e.timestamp }
func (e BaseEvent) AggregateID() string   { return e.aggregateID }
```

2. **Create Session Events** (1-2 hours)
```go
// File: domain/events/session_events.go
package events

// SessionCreated is emitted when a new session is created
type SessionCreated struct {
    BaseEvent
    SessionTitle string
    Path         string
    Program      string
    Branch       string
}

func NewSessionCreated(aggregateID, title, path, program, branch string) SessionCreated {
    return SessionCreated{
        BaseEvent:    NewBaseEvent(aggregateID),
        SessionTitle: title,
        Path:         path,
        Program:      program,
        Branch:       branch,
    }
}

func (e SessionCreated) EventType() string { return "session.created" }

// SessionKilled is emitted when a session is terminated
type SessionKilled struct {
    BaseEvent
    SessionTitle string
    Reason       string
}

func NewSessionKilled(aggregateID, title, reason string) SessionKilled {
    return SessionKilled{
        BaseEvent:    NewBaseEvent(aggregateID),
        SessionTitle: title,
        Reason:       reason,
    }
}

func (e SessionKilled) EventType() string { return "session.killed" }

// SessionStatusChanged is emitted when session status changes
type SessionStatusChanged struct {
    BaseEvent
    SessionTitle string
    OldStatus    string
    NewStatus    string
}

func NewSessionStatusChanged(aggregateID, title, oldStatus, newStatus string) SessionStatusChanged {
    return SessionStatusChanged{
        BaseEvent:    NewBaseEvent(aggregateID),
        SessionTitle: title,
        OldStatus:    oldStatus,
        NewStatus:    newStatus,
    }
}

func (e SessionStatusChanged) EventType() string { return "session.status_changed" }

// SessionAttached is emitted when a session is attached to
type SessionAttached struct {
    BaseEvent
    SessionTitle string
}

func NewSessionAttached(aggregateID, title string) SessionAttached {
    return SessionAttached{
        BaseEvent:    NewBaseEvent(aggregateID),
        SessionTitle: title,
    }
}

func (e SessionAttached) EventType() string { return "session.attached" }
```

3. **Add Unit Tests** (30 min)
```go
// File: domain/events/events_test.go
package events

func TestSessionCreated_EventType(t *testing.T) {
    event := NewSessionCreated("id", "test", "/path", "claude", "main")
    assert.Equal(t, "session.created", event.EventType())
}

func TestSessionCreated_Timestamp(t *testing.T) {
    event := NewSessionCreated("id", "test", "/path", "claude", "main")
    assert.WithinDuration(t, time.Now(), event.Timestamp(), time.Second)
}
```

**Validation Commands**:
```bash
# Verify no framework dependencies
cd domain && go list -f '{{.Imports}}' ./... | grep -E "bubbletea|lipgloss|proto"
# Expected: No output

# Run tests
go test ./domain/events -v
```

**Known Issues - Task 2.1**:

#### Bug Risk: Event Timestamp Skew [SEVERITY: Low]

**Description**: Events created in different timezones may have inconsistent timestamps.

**Mitigation**:
- Always use UTC for event timestamps
- Add timezone validation in tests

**Prevention**:
```go
func NewBaseEvent(aggregateID string) BaseEvent {
    return BaseEvent{
        timestamp:   time.Now().UTC(),  // Always UTC
        aggregateID: aggregateID,
    }
}
```

---

### Task 2.2: Replace SessionOperation with SessionResult

**INVEST Validation**:
- **I**ndependent: Depends only on Task 2.1 (domain events)
- **N**egotiable: Result structure negotiable, event usage fixed
- **V**aluable: Removes framework coupling from domain
- **E**stimable: 3-4 hours
- **S**mall: 2-3 files, focused on controller
- **T**estable: Can verify result structure independently

**Effort**: 3-4 hours  
**Files** (2-3):
- **Modify**: `app/session/controller.go` (replace SessionOperation)
- **Modify**: `app/session/operations.go` (update operations)
- **Create**: `app/session/result_test.go` (unit tests)

**Implementation Steps**:

1. **Define SessionResult** (30 min)
```go
// File: app/session/controller.go (refactored)
package session

import (
    "stapler-squad/domain/events"
    "stapler-squad/session"
)

// SessionResult represents the result of a session operation
// Replaces SessionOperation to remove framework coupling
type SessionResult struct {
    Type     SessionOperationType
    Success  bool
    Error    error
    Events   []events.DomainEvent  // Replace tea.Model/tea.Cmd
}

// NewSessionResult creates a successful result with events
func NewSessionResult(opType SessionOperationType, events ...events.DomainEvent) SessionResult {
    return SessionResult{
        Type:    opType,
        Success: true,
        Error:   nil,
        Events:  events,
    }
}

// NewSessionError creates a failed result with error
func NewSessionError(opType SessionOperationType, err error) SessionResult {
    return SessionResult{
        Type:    opType,
        Success: false,
        Error:   err,
        Events:  nil,
    }
}
```

2. **Update Controller Interface** (30 min)
```go
// Controller interface (updated)
type Controller interface {
    NewSession() SessionResult     // Changed return type
    KillSession() SessionResult    // Changed return type
    AttachSession() SessionResult  // Changed return type
    CheckoutSession() SessionResult
    ResumeSession() SessionResult
    // ... other methods
}
```

3. **Update Operations** (1-2 hours)
```go
// Implementation (example)
func (c *controller) NewSession() SessionResult {
    // ... session creation logic ...
    
    if err != nil {
        return NewSessionError(OpNewSession, err)
    }
    
    // Emit domain event instead of tea.Cmd
    event := events.NewSessionCreated(
        instance.Title,
        instance.Title,
        instance.Path,
        instance.Program,
        instance.Branch,
    )
    
    return NewSessionResult(OpNewSession, event)
}

func (c *controller) KillSession() SessionResult {
    selected := c.GetSelectedSession()
    if selected == nil {
        return NewSessionError(OpKillSession, errors.New("no session selected"))
    }
    
    // ... kill logic ...
    
    event := events.NewSessionKilled(
        selected.Title,
        selected.Title,
        "user requested",
    )
    
    return NewSessionResult(OpKillSession, event)
}
```

4. **Add Unit Tests** (1 hour)
```go
// File: app/session/result_test.go
package session

func TestSessionResult_Success(t *testing.T) {
    event := events.NewSessionCreated("id", "test", "/path", "claude", "main")
    result := NewSessionResult(OpNewSession, event)
    
    assert.True(t, result.Success)
    assert.Nil(t, result.Error)
    assert.Len(t, result.Events, 1)
}

func TestSessionResult_Error(t *testing.T) {
    result := NewSessionError(OpNewSession, errors.New("test error"))
    
    assert.False(t, result.Success)
    assert.NotNil(t, result.Error)
    assert.Len(t, result.Events, 0)
}
```

**Validation Commands**:
```bash
# Unit tests
go test ./app/session -run TestSessionResult -v

# Integration tests
go test ./app/session -v
```

---

### Task 2.3: Create EventAdapter

**INVEST Validation**:
- **I**ndependent: Depends on Tasks 2.1 and 2.2
- **N**egotiable: Adapter pattern negotiable, event types fixed
- **V**aluable: Bridges domain and framework layers
- **E**stimable: 3-4 hours
- **S**mall: 2-3 files, focused on adapter
- **T**estable: Can verify event-to-command conversion

**Effort**: 3-4 hours  
**Files** (2-3):
- **Create**: `app/adapters/event_adapter.go`
- **Create**: `app/adapters/event_adapter_test.go`
- **Modify**: `app/app.go` (use adapter in Update method)

**Implementation Steps**:

1. **Create Adapter Interface** (30 min)
```go
// File: app/adapters/event_adapter.go
package adapters

import (
    "stapler-squad/domain/events"
    tea "github.com/charmbracelet/bubbletea"
)

// EventAdapter converts domain events to BubbleTea commands
type EventAdapter interface {
    ConvertToCommand(events []events.DomainEvent) tea.Cmd
    ConvertSingleEvent(event events.DomainEvent) tea.Cmd
}

type eventAdapter struct{}

func NewEventAdapter() EventAdapter {
    return &eventAdapter{}
}
```

2. **Implement Adapter** (2 hours)
```go
// ConvertToCommand converts domain events to tea.Cmd
func (a *eventAdapter) ConvertToCommand(events []events.DomainEvent) tea.Cmd {
    if len(events) == 0 {
        return nil
    }
    
    // Create batch command for multiple events
    cmds := make([]tea.Cmd, 0, len(events))
    
    for _, event := range events {
        cmds = append(cmds, a.ConvertSingleEvent(event))
    }
    
    return tea.Batch(cmds...)
}

func (a *eventAdapter) ConvertSingleEvent(event events.DomainEvent) tea.Cmd {
    return func() tea.Msg {
        // Convert event to BubbleTea message
        switch e := event.(type) {
        case events.SessionCreated:
            return SessionCreatedMsg{
                Title:   e.SessionTitle,
                Path:    e.Path,
                Program: e.Program,
                Branch:  e.Branch,
            }
            
        case events.SessionKilled:
            return SessionKilledMsg{
                Title:  e.SessionTitle,
                Reason: e.Reason,
            }
            
        case events.SessionStatusChanged:
            return SessionStatusChangedMsg{
                Title:     e.SessionTitle,
                OldStatus: e.OldStatus,
                NewStatus: e.NewStatus,
            }
            
        case events.SessionAttached:
            return SessionAttachedMsg{
                Title: e.SessionTitle,
            }
        }
        
        return nil
    }
}

// BubbleTea message types (framework boundary)
type SessionCreatedMsg struct {
    Title   string
    Path    string
    Program string
    Branch  string
}

type SessionKilledMsg struct {
    Title  string
    Reason string
}

type SessionStatusChangedMsg struct {
    Title     string
    OldStatus string
    NewStatus string
}

type SessionAttachedMsg struct {
    Title string
}
```

3. **Add Unit Tests** (1 hour)
```go
// File: app/adapters/event_adapter_test.go
package adapters

func TestEventAdapter_ConvertSessionCreated(t *testing.T) {
    adapter := NewEventAdapter()
    event := events.NewSessionCreated("id", "test", "/path", "claude", "main")
    
    cmd := adapter.ConvertSingleEvent(event)
    require.NotNil(t, cmd)
    
    // Execute command
    msg := cmd()
    require.IsType(t, SessionCreatedMsg{}, msg)
    
    createdMsg := msg.(SessionCreatedMsg)
    assert.Equal(t, "test", createdMsg.Title)
}

func TestEventAdapter_ConvertMultipleEvents(t *testing.T) {
    adapter := NewEventAdapter()
    events := []events.DomainEvent{
        events.NewSessionCreated("id1", "test1", "/path", "claude", "main"),
        events.NewSessionKilled("id2", "test2", "user requested"),
    }
    
    cmd := adapter.ConvertToCommand(events)
    require.NotNil(t, cmd)
}
```

**Validation Commands**:
```bash
# Unit tests
go test ./app/adapters -v

# Integration with controller
go test ./app/session -v
```

**Known Issues - Task 2.3**:

#### Bug Risk: Message Type Exhaustion [SEVERITY: Low]

**Description**: New event types may be added without corresponding message types, causing nil message returns.

**Mitigation**:
- Add default case with panic in development
- Log warning for unknown event types
- Add integration test for all event types

**Prevention**:
```go
func (a *eventAdapter) ConvertSingleEvent(event events.DomainEvent) tea.Cmd {
    return func() tea.Msg {
        switch e := event.(type) {
        case events.SessionCreated:
            return SessionCreatedMsg{...}
        // ... other cases
        default:
            // Log warning in production
            log.Warn("Unknown event type: %s", event.EventType())
            return nil
        }
    }
}
```

---

### Task 2.4: Integrate EventAdapter into home

**INVEST Validation**:
- **I**ndependent: Depends on Task 2.3
- **N**egotiable: Integration points negotiable
- **V**aluable: Completes framework decoupling
- **E**stimable: 2-3 hours
- **S**mall: 2-3 files, focused on integration
- **T**estable: Can verify all operations still work

**Effort**: 2-3 hours  
**Files** (2-3):
- **Modify**: `app/app.go` (use adapter)
- **Modify**: `app/dependencies.go` (add adapter getter)
- **Update**: Test files (verify integration)

**Implementation Steps**:

1. **Add Adapter to home** (30 min)
```go
// File: app/app.go (updated)
package app

type home struct {
    // ... existing fields
    eventAdapter adapters.EventAdapter
}

func newHomeWithDependencies(deps Dependencies) *home {
    return &home{
        // ... existing initialization
        eventAdapter: adapters.NewEventAdapter(),
    }
}
```

2. **Update Key Handlers** (1-2 hours)
```go
func (h *home) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    key := msg.String()
    
    switch key {
    case "n":
        // Call controller (returns domain events)
        result := h.sessionManagement.sessionController.NewSession()
        
        if !result.Success {
            return h, h.handleError(result.Error)
        }
        
        // Convert domain events to BubbleTea commands
        cmd := h.eventAdapter.ConvertToCommand(result.Events)
        
        return h, cmd
        
    case "D":
        // Kill session
        result := h.sessionManagement.sessionController.KillSession()
        
        if !result.Success {
            return h, h.handleError(result.Error)
        }
        
        cmd := h.eventAdapter.ConvertToCommand(result.Events)
        return h, cmd
        
    // ... other keys
    }
}
```

3. **Add Event Message Handlers** (30 min)
```go
// Handle BubbleTea messages from adapter
func (h *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case adapters.SessionCreatedMsg:
        // Update UI for new session
        h.list.RefreshInstances()
        return h, nil
        
    case adapters.SessionKilledMsg:
        // Update UI after session killed
        h.list.RefreshInstances()
        return h, nil
        
    case tea.KeyMsg:
        return h.handleKeyMsg(msg)
        
    // ... other cases
    }
}
```

**Validation Commands**:
```bash
# Full integration test
go test ./app -v

# Verify no framework imports in domain
cd domain && go list -f '{{.Imports}}' ./... | grep -E "bubbletea"
# Expected: No output

# Manual smoke test
./stapler-squad
# Test: Create session, kill session, navigate
```

---

## Story 3: Integration & Validation

**Story**: As a developer, I want comprehensive validation that the refactoring is successful so that I can confidently deploy the changes.

**Priority**: P0  
**Effort**: 3-5 days  
**Dependencies**: Stories 1 and 2 must be complete

### Acceptance Criteria

**AC1**: All 368 Go files compile successfully  
**AC2**: Full test suite passes with zero regressions  
**AC3**: Performance benchmarks show no degradation  
**AC4**: Manual smoke tests pass (create, kill, attach, navigate sessions)  
**AC5**: Code quality metrics meet targets (complexity <10, coverage >80%)  
**AC6**: Documentation updated (ADRs, README)

---

### Task 3.1: Run Full Test Suite

**Effort**: 2-3 hours  
**Files**: All test files (no modifications, validation only)

**Validation Commands**:
```bash
# Full test suite
go test ./... -v -count=1

# With race detection
go test ./... -race -v

# With coverage
go test ./... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Check coverage percentage
go tool cover -func=coverage.out | grep total
# Expected: >80% total coverage
```

---

### Task 3.2: Run Performance Benchmarks

**Effort**: 1-2 hours (benchmarks run in background)  
**Files**: Benchmark files (no modifications)

**Validation Commands**:
```bash
# Navigation performance
go test -bench=BenchmarkNavigationPerformance ./app -benchmem &

# Large session counts
go test -bench=BenchmarkLargeSessionNavigation ./app -benchmem &

# Compare before/after (if baseline exists)
benchstat baseline.txt refactored.txt
```

---

### Task 3.3: Manual Smoke Testing

**Effort**: 1-2 hours  
**Test Scenarios**:

1. **Session Creation**:
   - Create new session with "n" key
   - Verify session appears in list
   - Check session status

2. **Session Navigation**:
   - Navigate up/down with j/k
   - Test rapid navigation (debouncing)
   - Navigate to first/last (g/G)

3. **Session Operations**:
   - Kill session with "D"
   - Attach to session with "a"
   - Resume paused session with "r"

4. **Filtering**:
   - Toggle paused filter with "f"
   - Search sessions with "s"
   - Clear search with ESC

5. **Error Handling**:
   - Attempt to exceed instance limit
   - Kill non-existent session
   - Attach to paused session

---

### Task 3.4: Update Documentation

**Effort**: 2-3 hours  
**Files** (3-5):
- **Create**: `docs/adr/ADR-001-service-extraction.md`
- **Create**: `docs/adr/ADR-002-domain-events.md`
- **Update**: `CLAUDE.md` (architecture section)
- **Update**: `TODO.md` (mark Phase 1 complete)

**ADR Template**:
```markdown
# ADR-001: Service Extraction from God Object

**Date**: 2025-12-05  
**Status**: Accepted  
**Context**: The `home` struct had 30+ fields spanning multiple responsibilities, violating SRP.

**Decision**: Extract 4 focused services (SessionManagement, Navigation, Filtering, UICoordination) and refactor `home` to a thin facade coordinating services.

**Consequences**:
- **Positive**: Improved maintainability, testability, reduced complexity
- **Positive**: Clear separation of concerns
- **Negative**: Additional layer of indirection
- **Negative**: More files to maintain

**Alternatives Considered**:
1. Keep monolithic `home` struct (rejected - too complex)
2. Extract more granular services (rejected - over-engineering)
3. Extract only 2-3 services (rejected - insufficient separation)
```

---

## Known Issues & Bug Prevention

### Critical Issues Identified

#### 1. Race Condition in Concurrent Session Creation [SEVERITY: High]

**Location**: `app/services/session_management.go:CreateSession()`  
**Description**: Multiple goroutines calling `CreateSession()` may exceed instance limit.

**Root Cause**: Non-atomic check-then-create operation:
```go
// BAD: Race condition
if s.list.NumInstances() >= GlobalInstanceLimit {
    return errors.New("instance limit reached")
}
// Another goroutine may create session here
result := s.sessionController.NewSession()
```

**Mitigation**:
```go
// GOOD: Mutex protection
func (s *sessionManagementService) CreateSession(opts session.InstanceOptions) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Atomic check and create
    if s.list.NumInstances() >= GlobalInstanceLimit {
        return errors.New("instance limit reached")
    }
    
    result := s.sessionController.NewSession()
    if !result.Success {
        return result.Error
    }
    
    return nil
}
```

**Test Case**:
```go
func TestSessionManagementService_ConcurrentCreation(t *testing.T) {
    service := setupService(t)
    
    // Fill list to 1 below limit
    for i := 0; i < GlobalInstanceLimit-1; i++ {
        _ = service.CreateSession(session.InstanceOptions{})
    }
    
    // Attempt concurrent creation from 10 goroutines
    var wg sync.WaitGroup
    successes := atomic.Int32{}
    failures := atomic.Int32{}
    
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := service.CreateSession(session.InstanceOptions{})
            if err == nil {
                successes.Add(1)
            } else {
                failures.Add(1)
            }
        }()
    }
    
    wg.Wait()
    
    // Only 1 should succeed, 9 should fail
    assert.Equal(t, int32(1), successes.Load())
    assert.Equal(t, int32(9), failures.Load())
}
```

**Prevention Checklist**:
- [ ] Add mutex to SessionManagementService
- [ ] Write concurrent creation test
- [ ] Add race detector to CI (`go test -race`)
- [ ] Document thread-safety guarantees

---

#### 2. Navigation Index Out of Bounds After Filtering [SEVERITY: Medium]

**Location**: `app/services/navigation.go:NavigateToIndex()`  
**Description**: After filtering or deleting sessions, navigation may access invalid indices.

**Root Cause**: Index not updated after filter changes:
```go
// BAD: No bounds checking
func (n *navigationService) NavigateToIndex(idx int) error {
    return n.list.SetSelectedInstance(idx)  // May panic
}
```

**Mitigation**:
```go
// GOOD: Bounds checking
func (n *navigationService) NavigateToIndex(idx int) error {
    n.mu.Lock()
    defer n.mu.Unlock()
    
    // Validate bounds
    if idx < 0 || idx >= n.list.NumInstances() {
        return fmt.Errorf("invalid navigation index: %d (list size: %d)", idx, n.list.NumInstances())
    }
    
    return n.list.SetSelectedInstance(idx)
}

// Update index after filter changes
func (n *navigationService) OnFilterChanged() {
    n.mu.Lock()
    defer n.mu.Unlock()
    
    // Reset to first visible item
    if n.list.NumInstances() > 0 {
        n.list.SetSelectedInstance(0)
    }
}
```

**Test Case**:
```go
func TestNavigationService_IndexBoundsAfterFilter(t *testing.T) {
    service, list := setupNavigationService(t)
    
    // Add 10 sessions
    for i := 0; i < 10; i++ {
        list.AddInstance(createTestSession(t, fmt.Sprintf("session-%d", i)))
    }
    
    // Navigate to index 5
    err := service.NavigateToIndex(5)
    require.NoError(t, err)
    
    // Apply filter that reduces list to 3 items
    list.FilterBySearch("session-0")
    
    // Attempt to navigate to old index (should fail)
    err = service.NavigateToIndex(5)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid navigation index")
}
```

**Prevention Checklist**:
- [ ] Add bounds validation to all navigation methods
- [ ] Subscribe navigation service to filter change events
- [ ] Write filter boundary tests
- [ ] Add nil checks for empty lists

---

#### 3. Search Index Stale After Session Changes [SEVERITY: Medium]

**Location**: `app/services/filtering.go:UpdateSearchQuery()`  
**Description**: Search index not updated when sessions are added/removed, leading to stale results.

**Root Cause**: Search index built once, not invalidated on changes:
```go
// BAD: Search index never updated
func (f *filteringService) UpdateSearchQuery(query string) []*session.Instance {
    f.searchQuery = query
    f.list.FilterBySearch(query)  // Uses stale index
    return f.list.GetVisibleItems()
}
```

**Mitigation**:
```go
// GOOD: Rebuild index on session changes
type filteringService struct {
    list         *ui.List
    searchActive bool
    searchQuery  string
    searchIndex  map[string][]*session.Instance  // Indexed search
    mu           sync.RWMutex
}

func (f *filteringService) OnSessionsChanged() {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    // Rebuild search index
    f.rebuildSearchIndex()
    
    // Re-apply current search if active
    if f.searchActive {
        f.list.FilterBySearch(f.searchQuery)
    }
}

func (f *filteringService) rebuildSearchIndex() {
    // Rebuild inverted index for fast search
    f.searchIndex = make(map[string][]*session.Instance)
    
    for _, instance := range f.list.GetAllInstances() {
        // Index by title, path, tags
        tokens := extractSearchTokens(instance)
        for _, token := range tokens {
            f.searchIndex[token] = append(f.searchIndex[token], instance)
        }
    }
}
```

**Test Case**:
```go
func TestFilteringService_SearchAfterSessionAdded(t *testing.T) {
    service, list := setupFilteringService(t)
    
    // Add initial session
    list.AddInstance(createTestSession(t, "original"))
    
    // Search for "original"
    results := service.UpdateSearchQuery("original")
    assert.Len(t, results, 1)
    
    // Add new session matching query
    list.AddInstance(createTestSession(t, "original-copy"))
    
    // Trigger index rebuild
    service.OnSessionsChanged()
    
    // Re-search should find both
    results = service.UpdateSearchQuery("original")
    assert.Len(t, results, 2)
}
```

**Prevention Checklist**:
- [ ] Subscribe filtering service to session change events
- [ ] Add index rebuild method
- [ ] Write stale index test
- [ ] Monitor search performance with large session counts

---

#### 4. Service Initialization Order Deadlock [SEVERITY: High]

**Location**: `app/dependencies.go:GetSessionManagementService()`  
**Description**: Services with circular dependencies may cause initialization deadlock.

**Root Cause**: Lazy initialization without dependency ordering:
```go
// BAD: Potential circular dependency
func (p *ProductionDependencies) GetSessionManagementService() services.SessionManagementService {
    if p.sessionManagementService == nil {
        // May call GetNavigationService() which calls GetSessionManagementService()
        p.sessionManagementService = services.NewSessionManagementService(...)
    }
    return p.sessionManagementService
}
```

**Mitigation**:
```go
// GOOD: Define clear initialization order
func (p *ProductionDependencies) GetSessionManagementService() services.SessionManagementService {
    if p.sessionManagementService == nil {
        // 1. Initialize leaf dependencies first
        storage := p.GetStorage()
        controller := p.GetSessionController()
        list := p.GetList()
        
        // 2. Create service with no service dependencies
        p.sessionManagementService = services.NewSessionManagementService(
            storage,
            controller,
            list,
            nil,  // Error handler injected later
        )
    }
    return p.sessionManagementService
}

// Dependency graph (documented):
// Storage <- SessionManagementService
// SessionController <- SessionManagementService
// List <- SessionManagementService
// NavigationService -> List (no circular dependency)
```

**Test Case**:
```go
func TestDependencies_InitializationOrder(t *testing.T) {
    deps := NewProductionDependencies(context.Background(), "claude", false)
    
    // Should be able to initialize all services without deadlock
    done := make(chan bool, 1)
    
    go func() {
        _ = deps.GetSessionManagementService()
        _ = deps.GetNavigationService()
        _ = deps.GetFilteringService()
        _ = deps.GetUICoordinationService()
        done <- true
    }()
    
    select {
    case <-done:
        // Success
    case <-time.After(5 * time.Second):
        t.Fatal("Initialization deadlock detected")
    }
}
```

**Prevention Checklist**:
- [ ] Draw dependency graph before implementation
- [ ] Document initialization order in dependencies.go
- [ ] Add initialization timeout test
- [ ] Use dependency injection (no service-to-service calls in constructors)

---

#### 5. Event Adapter Message Type Exhaustion [SEVERITY: Low]

**Location**: `app/adapters/event_adapter.go:ConvertSingleEvent()`  
**Description**: New domain events may be added without corresponding message types, causing nil returns.

**Root Cause**: Switch statement without default case:
```go
// BAD: Silently returns nil for unknown events
func (a *eventAdapter) ConvertSingleEvent(event events.DomainEvent) tea.Cmd {
    return func() tea.Msg {
        switch e := event.(type) {
        case events.SessionCreated:
            return SessionCreatedMsg{...}
        case events.SessionKilled:
            return SessionKilledMsg{...}
        // Missing default case
        }
        return nil  // Silent failure
    }
}
```

**Mitigation**:
```go
// GOOD: Explicit handling with logging
func (a *eventAdapter) ConvertSingleEvent(event events.DomainEvent) tea.Cmd {
    return func() tea.Msg {
        switch e := event.(type) {
        case events.SessionCreated:
            return SessionCreatedMsg{...}
        case events.SessionKilled:
            return SessionKilledMsg{...}
        case events.SessionStatusChanged:
            return SessionStatusChangedMsg{...}
        case events.SessionAttached:
            return SessionAttachedMsg{...}
        default:
            // Log warning for unknown event types
            log.Warn("Unknown event type: %s (event: %#v)", event.EventType(), event)
            
            // In development, panic to catch missing handlers
            if os.Getenv("GO_ENV") == "development" {
                panic(fmt.Sprintf("Unhandled event type: %s", event.EventType()))
            }
            
            return nil
        }
    }
}
```

**Test Case**:
```go
func TestEventAdapter_AllEventTypesHandled(t *testing.T) {
    adapter := NewEventAdapter()
    
    // List of all domain event types
    events := []events.DomainEvent{
        events.NewSessionCreated("id", "test", "/path", "claude", "main"),
        events.NewSessionKilled("id", "test", "reason"),
        events.NewSessionStatusChanged("id", "test", "running", "paused"),
        events.NewSessionAttached("id", "test"),
    }
    
    for _, event := range events {
        cmd := adapter.ConvertSingleEvent(event)
        require.NotNil(t, cmd, "Event type %s not handled", event.EventType())
        
        msg := cmd()
        require.NotNil(t, msg, "Event type %s returned nil message", event.EventType())
    }
}
```

**Prevention Checklist**:
- [ ] Add default case with logging
- [ ] Write exhaustiveness test for all event types
- [ ] Document message type mapping
- [ ] Add CI check for event-message coverage

---

#### 6. Debouncing Timer Leak [SEVERITY: Medium]

**Location**: `app/services/navigation.go:ResponsiveNavigationManager`  
**Description**: Debounce timers may not be properly cancelled, causing goroutine leaks.

**Root Cause**: Timer not stopped on service shutdown:
```go
// BAD: Timer never cancelled
type ResponsiveNavigationManager struct {
    debounceTimer *time.Timer
}

func (r *ResponsiveNavigationManager) ScheduleNavigation(navFn func() error) error {
    if r.debounceTimer != nil {
        // Old timer not stopped, goroutine leaks
        r.debounceTimer = time.AfterFunc(r.debounceDelay, func() {
            navFn()
        })
    }
}
```

**Mitigation**:
```go
// GOOD: Proper timer cleanup
type ResponsiveNavigationManager struct {
    debounceTimer *time.Timer
    mu            sync.Mutex
}

func (r *ResponsiveNavigationManager) ScheduleNavigation(navFn func() error) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    // Stop existing timer
    if r.debounceTimer != nil {
        r.debounceTimer.Stop()
    }
    
    // Create new timer
    r.debounceTimer = time.AfterFunc(r.debounceDelay, func() {
        navFn()
    })
    
    return nil
}

func (r *ResponsiveNavigationManager) Stop() {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if r.debounceTimer != nil {
        r.debounceTimer.Stop()
        r.debounceTimer = nil
    }
}
```

**Test Case**:
```go
func TestNavigationManager_NoTimerLeak(t *testing.T) {
    manager := NewResponsiveNavigationManager(50 * time.Millisecond)
    
    // Schedule many rapid navigations
    for i := 0; i < 100; i++ {
        manager.ScheduleNavigation(func() error {
            return nil
        })
        time.Sleep(10 * time.Millisecond)
    }
    
    // Stop manager
    manager.Stop()
    
    // Wait for goroutines to finish
    time.Sleep(200 * time.Millisecond)
    
    // Check goroutine count (should not increase)
    initialGoroutines := runtime.NumGoroutine()
    
    // Create new manager and repeat
    manager = NewResponsiveNavigationManager(50 * time.Millisecond)
    for i := 0; i < 100; i++ {
        manager.ScheduleNavigation(func() error { return nil })
        time.Sleep(10 * time.Millisecond)
    }
    manager.Stop()
    time.Sleep(200 * time.Millisecond)
    
    // Goroutine count should be stable
    finalGoroutines := runtime.NumGoroutine()
    assert.InDelta(t, initialGoroutines, finalGoroutines, 5, "Goroutine leak detected")
}
```

**Prevention Checklist**:
- [ ] Add Stop() method to navigation manager
- [ ] Call Stop() in service cleanup
- [ ] Write goroutine leak test
- [ ] Run tests with `-race` flag

---

### Bug Prevention Summary Table

| Bug ID | Severity | Location | Prevention Strategy | Test Coverage |
|--------|----------|----------|---------------------|---------------|
| 1 | High | SessionManagementService | Mutex protection | Concurrent creation test |
| 2 | Medium | NavigationService | Bounds validation | Filter boundary test |
| 3 | Medium | FilteringService | Index rebuild on changes | Stale index test |
| 4 | High | Dependencies | Dependency graph | Initialization timeout test |
| 5 | Low | EventAdapter | Default case logging | Exhaustiveness test |
| 6 | Medium | NavigationManager | Timer cleanup | Goroutine leak test |

---

## Integration Checkpoints

### Checkpoint 1: After Task 1.1 (SessionManagementService)

**Validation**:
```bash
go test ./app/services -run TestSessionManagementService -v
go test ./app -run TestSessionCreation -v
./stapler-squad  # Smoke test: Create and kill session
```

**Success Criteria**:
- [ ] All tests pass
- [ ] Session creation works in TUI
- [ ] Session killing works in TUI
- [ ] No regressions in existing functionality

**Rollback Plan**: If checkpoint fails, revert Task 1.1 commits and investigate.

---

### Checkpoint 2: After Task 1.2-1.3 (Navigation & Filtering)

**Validation**:
```bash
go test ./app/services -v
go test -bench=BenchmarkNavigationPerformance ./app
./stapler-squad  # Smoke test: Navigate, filter, search
```

**Success Criteria**:
- [ ] All service tests pass
- [ ] Navigation responsiveness maintained
- [ ] Filtering works correctly
- [ ] Search results accurate

**Rollback Plan**: If checkpoint fails, revert Tasks 1.2-1.3 and investigate.

---

### Checkpoint 3: After Task 1.5 (Facade Refactoring)

**Validation**:
```bash
go test ./app -v -count=1
go test ./... -v
grep -A 50 "^type home struct" app/app.go | wc -l
# Expected: <15 lines
```

**Success Criteria**:
- [ ] `home` struct has <10 fields
- [ ] All tests pass
- [ ] Full TUI functionality works
- [ ] Cyclomatic complexity <10

**Rollback Plan**: If checkpoint fails, revert Task 1.5 (entire Story 1) and re-plan.

---

### Checkpoint 4: After Task 2.2 (Domain Events Integration)

**Validation**:
```bash
cd domain && go list -f '{{.Imports}}' ./... | grep -E "bubbletea"
# Expected: No output
go test ./domain/events -v
go test ./app/session -v
```

**Success Criteria**:
- [ ] Domain has zero framework imports
- [ ] SessionResult replaces SessionOperation
- [ ] All session operations return events
- [ ] Tests pass

**Rollback Plan**: If checkpoint fails, revert Story 2 and keep SessionOperation pattern.

---

### Checkpoint 5: After Task 2.4 (EventAdapter Integration)

**Validation**:
```bash
go test ./app/adapters -v
go test ./app -v
./stapler-squad  # Full smoke test
```

**Success Criteria**:
- [ ] Event-to-command conversion works
- [ ] All TUI operations functional
- [ ] No performance degradation
- [ ] Framework coupling removed

**Rollback Plan**: If checkpoint fails, revert Story 2 entirely.

---

### Checkpoint 6: Final Validation (Story 3)

**Validation**:
```bash
# Full test suite
go test ./... -v -race -cover

# Performance benchmarks
go test -bench=. ./app -benchmem

# Manual smoke test
./stapler-squad
# Test: Create, kill, attach, navigate, filter, search

# Verify metrics
gocyclo -over 10 app/app.go
go tool cover -func=coverage.out | grep total
```

**Success Criteria**:
- [ ] All 368 files compile
- [ ] Zero test regressions
- [ ] Test coverage >80%
- [ ] Cyclomatic complexity <10
- [ ] Performance benchmarks stable
- [ ] Manual smoke tests pass

**Go/No-Go Decision**: If all criteria met, proceed to merge. Otherwise, investigate failures.

---

## Context Preparation Guide

### Context for Task 1.1 (SessionManagementService)

**Files to Read**:
1. `app/app.go:51-130` - Current `home` struct
2. `app/app.go:handleKeyMsg()` - Session operation handling
3. `app/session/controller.go` - Session controller interface
4. `app/dependencies.go` - Current dependency injection

**Mental Model**:
- `home` currently handles session operations inline
- Extract to service with clear interface
- Service delegates to session controller
- Maintain error handling pattern

**Command to Load Context**:
```bash
# Read key files
cat app/app.go | grep -A 20 "type home struct"
cat app/app.go | grep -A 50 "case \"n\":"
cat app/session/controller.go | grep -A 30 "type Controller"
```

---

### Context for Task 1.5 (Facade Refactoring)

**Files to Read**:
1. All service interfaces (Tasks 1.1-1.4)
2. `app/app.go` - Full file
3. `app/dependencies.go` - All getters
4. `app/test_helpers.go` - Test setup

**Mental Model**:
- Services are independent, home coordinates
- Each key delegates to appropriate service
- Error handling through UI coordination service
- Dependencies injected via interface

**Command to Load Context**:
```bash
# Verify services created
ls app/services/
# Expected: session_management.go, navigation.go, filtering.go, ui_coordination.go

# Read service interfaces
grep -A 10 "type.*Service interface" app/services/*.go
```

---

### Context for Task 2.3 (EventAdapter)

**Files to Read**:
1. `domain/events/session_events.go` - Event types
2. `app/session/controller.go` - SessionResult struct
3. BubbleTea message patterns in existing code

**Mental Model**:
- Domain events are framework-independent
- Adapter converts to BubbleTea messages
- One-to-one mapping of event types to message types
- Adapter lives in app layer (framework boundary)

**Command to Load Context**:
```bash
# Review event types
cat domain/events/session_events.go | grep "type.*struct"

# Review SessionResult
cat app/session/controller.go | grep -A 10 "type SessionResult"
```

---

## Dependency Visualization

### Current Architecture (Before Refactoring)

```
┌──────────────────────────────────────────────┐
│              home (God Object)                │
│  - 30+ fields                                 │
│  - Session management                         │
│  - Navigation                                 │
│  - Filtering                                  │
│  - UI coordination                            │
│  - State management                           │
│  - Error handling                             │
│  - ... (many more responsibilities)           │
└────────────┬─────────────────────────────────┘
             │
             ├───────────► session.Storage
             ├───────────► session.Controller
             ├───────────► ui.List
             ├───────────► ui.Menu
             ├───────────► ui.TabbedWindow
             ├───────────► ui.ErrBox
             ├───────────► state.Manager
             └───────────► ... (many more dependencies)

Issues:
- Single class with 30+ responsibilities
- Difficult to test in isolation
- High cyclomatic complexity (30+)
- Tight coupling to all dependencies
```

---

### Target Architecture (After Refactoring)

```
                ┌──────────────────────────────┐
                │    home (Thin Facade)        │
                │  - <10 fields                 │
                │  - Delegates to services      │
                │  - Coordinates UI             │
                └─────────────┬────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
          ▼                   ▼                   ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│ SessionMgmt     │  │ Navigation      │  │ Filtering       │
│ Service         │  │ Service         │  │ Service         │
│                 │  │                 │  │                 │
│ - CreateSession │  │ - NavigateUp    │  │ - ToggleFilter  │
│ - KillSession   │  │ - NavigateDown  │  │ - Search        │
│ - AttachSession │  │ - Debouncing    │  │ - ClearSearch   │
└────────┬────────┘  └────────┬────────┘  └────────┬────────┘
         │                    │                     │
         ▼                    ▼                     ▼
   ┌─────────────┐      ┌─────────┐         ┌─────────┐
   │ Session     │      │ ui.List │         │ ui.List │
   │ Controller  │      └─────────┘         └─────────┘
   └─────────────┘
         │
         ▼
   ┌─────────────┐
   │ Domain      │      ┌─────────────────┐
   │ Events      │◄─────┤ EventAdapter    │
   └─────────────┘      │ (Framework      │
                        │  Boundary)      │
                        └────────┬────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │ BubbleTea       │
                        │ Commands        │
                        └─────────────────┘

Benefits:
- Clear separation of concerns
- Each service <300 lines
- Easy to test in isolation
- Low cyclomatic complexity (<10)
- Framework-independent domain
```

---

### Service Dependency Graph

```
                    Dependencies
                         │
     ┌───────────────────┼───────────────────┐
     │                   │                   │
     ▼                   ▼                   ▼
SessionMgmt         Navigation          Filtering
     │                   │                   │
     ├──► Storage        └──► List           └──► List
     ├──► Controller
     └──► List

UICoordination
     │
     ├──► UICoordinator
     ├──► Menu
     ├──► StatusBar
     └──► ErrBox

Initialization Order:
1. Storage (leaf dependency)
2. List (depends on Storage)
3. Controller (depends on Storage)
4. Services (depend on List, Storage, Controller)
5. home (depends on Services)

No circular dependencies ✓
```

---

## Git Commit Strategy

### Commit Message Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**: `feat`, `refactor`, `test`, `docs`, `chore`  
**Scopes**: `services`, `domain`, `adapter`, `facade`, `deps`

---

### Task 1.1 Commits

```bash
# Commit 1: Create service interface
git commit -m "refactor(services): add SessionManagementService interface

- Define interface for session lifecycle operations
- Methods: CreateSession, KillSession, AttachSession, ResumeSession
- Part of God Object refactoring (Story 1, Task 1.1)

Related: #architecture-phase1"

# Commit 2: Implement service
git commit -m "refactor(services): implement SessionManagementService

- Extract session creation logic from home.Update()
- Extract session killing logic from home.Update()
- Add error handling and validation
- Part of God Object refactoring (Story 1, Task 1.1)

Related: #architecture-phase1"

# Commit 3: Add tests
git commit -m "test(services): add SessionManagementService tests

- Test successful session creation
- Test instance limit enforcement
- Test error handling
- Test concurrent session creation (race condition prevention)
- Coverage: 85%

Related: #architecture-phase1"

# Commit 4: Update dependencies
git commit -m "refactor(deps): add SessionManagementService to dependencies

- Add GetSessionManagementService() to Dependencies interface
- Implement getter in ProductionDependencies
- Implement getter in MockDependencies
- Part of God Object refactoring (Story 1, Task 1.1)

Related: #architecture-phase1"

# Commit 5: Integrate into home
git commit -m "refactor(facade): use SessionManagementService in home

- Replace inline session logic with service delegation
- Update key handlers to use service
- Maintain error handling pattern
- Part of God Object refactoring (Story 1, Task 1.1)

Related: #architecture-phase1"
```

---

### Task 1.5 Commit (Major Refactoring)

```bash
git commit -m "refactor(facade): reduce home struct to thin facade

BREAKING CHANGE: home struct refactored from 30+ fields to <10

- Extract services: SessionManagement, Navigation, Filtering, UICoordination
- Reduce home to coordinator role
- Update constructor to use service injection
- Refactor Update() method to delegate to services
- Update test helpers for new structure

Metrics:
- home fields: 30+ → 8
- home.Update() complexity: 30+ → 8
- Test coverage: 65% → 82%

Closes: #architecture-phase1-story1
Related: #god-object-refactoring"
```

---

### Task 2.2 Commit (Framework Decoupling)

```bash
git commit -m "refactor(domain): replace SessionOperation with SessionResult

BREAKING CHANGE: SessionOperation no longer uses tea.Model/tea.Cmd

- Replace tea.Model/tea.Cmd with domain events
- Update Controller interface to return SessionResult
- Update all operations to emit domain events
- Zero framework imports in domain layer

Metrics:
- Framework coupling: High → Low
- Domain dependencies: BubbleTea removed ✓

Closes: #architecture-phase1-story2
Related: #framework-decoupling"
```

---

### Final Integration Commit

```bash
git commit -m "feat(architecture): complete Phase 1 refactoring

Complete God Object and Framework Coupling refactoring (P0 issues)

Changes:
1. Extracted 4 focused services from God Object
2. Refactored home to thin facade
3. Replaced framework types with domain events
4. Implemented EventAdapter for framework boundary
5. Updated all tests (zero regressions)

Metrics:
- home fields: 30+ → 8 (73% reduction)
- home.Update() complexity: 30+ → 8 (73% reduction)
- Test coverage: 65% → 82% (+17%)
- Framework coupling: High → Low
- Domain layer: Zero framework imports ✓

Performance:
- Navigation responsiveness: Maintained
- Benchmark results: No degradation
- Test suite time: 28s (stable)

Closes: #architecture-phase1
Related: #architecture-review, #god-object, #framework-coupling"
```

---

### Branch Strategy

```bash
# Feature branch for entire Phase 1
git checkout -b feature/architecture-phase1

# Sub-branches for stories (optional)
git checkout -b feature/architecture-phase1/story1-services
git checkout -b feature/architecture-phase1/story2-events

# Merge strategy
# Story 1 → feature/architecture-phase1
# Story 2 → feature/architecture-phase1
# Story 3 (validation) → feature/architecture-phase1
# feature/architecture-phase1 → main (after final approval)
```

---

## Risk Mitigation

### High-Risk Areas

#### 1. Breaking Changes During Facade Refactoring (Task 1.5)

**Risk Level**: High  
**Impact**: All existing code may break  
**Likelihood**: Medium

**Mitigation Strategy**:
1. **Comprehensive Test Suite**: Run full test suite before starting
2. **Incremental Migration**: Migrate one key handler at a time
3. **Feature Branch**: Keep all changes in feature branch until validated
4. **Rollback Plan**: Keep pre-refactor snapshot for quick rollback

**Rollback Plan**:
```bash
# If Task 1.5 fails validation
git checkout feature/architecture-phase1
git revert HEAD~5..HEAD  # Revert last 5 commits (Task 1.5)
git push origin feature/architecture-phase1 --force-with-lease
```

**Validation Before Merge**:
```bash
# Must pass all checks
go test ./... -v -race -count=3
go test -bench=. ./app -benchmem
./stapler-squad  # Manual smoke test
```

---

#### 2. Framework Coupling Removal (Story 2)

**Risk Level**: Medium-High  
**Impact**: Domain layer may still have hidden framework dependencies  
**Likelihood**: Medium

**Mitigation Strategy**:
1. **Import Verification**: Automated check for framework imports
2. **Adapter Pattern**: Clear framework boundary with adapter
3. **Parallel Implementation**: Keep old code until adapter proven
4. **Integration Tests**: Verify event-to-command conversion works

**Rollback Plan**:
```bash
# If Story 2 breaks functionality
git checkout feature/architecture-phase1
git checkout main -- app/session/controller.go  # Restore old controller
# Keep SessionResult alongside SessionOperation temporarily
```

**Automated Check**:
```bash
# Add to CI pipeline
cd domain && go list -f '{{.Imports}}' ./... | grep -E "bubbletea|lipgloss"
if [ $? -eq 0 ]; then
  echo "ERROR: Framework imports found in domain layer"
  exit 1
fi
```

---

#### 3. Service Initialization Deadlock (Task 1.5)

**Risk Level**: High  
**Impact**: Application may hang on startup  
**Likelihood**: Low

**Mitigation Strategy**:
1. **Dependency Graph**: Draw graph before implementation
2. **Initialization Order**: Document order in dependencies.go
3. **Timeout Test**: Add test that detects initialization hangs
4. **Lazy Initialization**: Use lazy init for optional dependencies

**Detection Test**:
```go
func TestDependencies_NoInitializationDeadlock(t *testing.T) {
    done := make(chan bool, 1)
    
    go func() {
        deps := NewProductionDependencies(context.Background(), "claude", false)
        // Initialize all services
        _ = deps.GetSessionManagementService()
        _ = deps.GetNavigationService()
        _ = deps.GetFilteringService()
        _ = deps.GetUICoordinationService()
        done <- true
    }()
    
    select {
    case <-done:
        // Success
    case <-time.After(5 * time.Second):
        t.Fatal("Initialization deadlock detected")
    }
}
```

---

#### 4. Test Coverage Regression

**Risk Level**: Medium  
**Impact**: Hidden bugs may slip through  
**Likelihood**: Medium

**Mitigation Strategy**:
1. **Coverage Baseline**: Establish baseline before refactoring
2. **Coverage Gates**: Fail CI if coverage drops >5%
3. **Unit Tests First**: Write tests before refactoring
4. **Integration Tests**: Maintain integration test suite

**Coverage Monitoring**:
```bash
# Before refactoring
go test ./app -cover -coverprofile=baseline.out
baseline_coverage=$(go tool cover -func=baseline.out | grep total | awk '{print $3}')
echo "Baseline coverage: $baseline_coverage"

# After refactoring
go test ./app -cover -coverprofile=refactored.out
refactored_coverage=$(go tool cover -func=refactored.out | grep total | awk '{print $3}')
echo "Refactored coverage: $refactored_coverage"

# Compare
# Fail if coverage drops >5%
```

---

#### 5. Performance Degradation

**Risk Level**: Medium  
**Impact**: Navigation may feel sluggish  
**Likelihood**: Low

**Mitigation Strategy**:
1. **Baseline Benchmarks**: Run benchmarks before refactoring
2. **Service Overhead**: Keep services lightweight
3. **Debouncing**: Maintain debouncing for navigation
4. **Profile Analysis**: Profile if performance degrades

**Benchmark Comparison**:
```bash
# Before refactoring
go test -bench=BenchmarkNavigationPerformance ./app -benchmem > baseline_bench.txt

# After refactoring
go test -bench=BenchmarkNavigationPerformance ./app -benchmem > refactored_bench.txt

# Compare
benchstat baseline_bench.txt refactored_bench.txt

# Acceptable: <10% performance degradation
# Fail if: >10% degradation in navigation benchmarks
```

---

### Risk Matrix

| Risk | Severity | Likelihood | Impact | Mitigation | Owner |
|------|----------|------------|--------|------------|-------|
| Breaking changes (Task 1.5) | High | Medium | High | Incremental migration, tests | Developer |
| Framework coupling removal | Medium-High | Medium | Medium | Import checks, adapter | Developer |
| Initialization deadlock | High | Low | High | Timeout test, dep graph | Developer |
| Test coverage regression | Medium | Medium | Medium | Coverage gates | CI |
| Performance degradation | Medium | Low | Medium | Benchmarks | Developer |
| Race conditions | High | Low | High | Race detector, tests | Developer |

---

## Success Metrics Dashboard

Track these metrics throughout Phase 1:

### Code Quality Metrics

| Metric | Baseline | Target | Current | Status |
|--------|----------|--------|---------|--------|
| `home` field count | 30+ | <10 | - | 🔴 Not started |
| `home.Update()` complexity | 30+ | <10 | - | 🔴 Not started |
| Service count | 0 | 4 | - | 🔴 Not started |
| Test coverage (app/) | 65% | 80% | - | 🔴 Not started |
| Framework imports (domain/) | >0 | 0 | - | 🔴 Not started |

---

### Performance Metrics

| Benchmark | Baseline | Target | Current | Status |
|-----------|----------|--------|---------|--------|
| Navigation latency | - | <50ms | - | 🔴 Not measured |
| Session creation time | - | <500ms | - | 🔴 Not measured |
| Test suite time | 28s | <30s | - | 🔴 Not measured |
| Binary size | - | No increase | - | 🔴 Not measured |

---

### Test Metrics

| Test Suite | Count | Pass Rate | Coverage | Status |
|------------|-------|-----------|----------|--------|
| Unit tests (services) | 0 | - | - | 🔴 Not started |
| Integration tests (app) | ~50 | 100% | 65% | 🟢 Baseline |
| Benchmark tests | ~10 | - | - | 🟢 Baseline |

---

## Appendix

### A. Glossary

**God Object**: An object with too many responsibilities, violating Single Responsibility Principle.

**Facade Pattern**: A structural pattern providing a simplified interface to a complex subsystem.

**Domain Event**: An event that represents something significant that happened in the domain.

**Service Layer**: An architectural pattern that defines an application's boundary with a layer of services that establishes a set of available operations.

**Adapter Pattern**: A structural pattern that allows incompatible interfaces to work together.

**Cyclomatic Complexity**: A metric measuring the number of independent paths through a program's source code.

---

### B. References

**Books**:
- "Clean Architecture" (Robert C. Martin)
- "Domain-Driven Design" (Eric Evans)
- "Refactoring: Improving the Design of Existing Code" (Martin Fowler)
- "Code Complete" (Steve McConnell)

**Documents**:
- [Architecture Review](../ARCHITECTURE_REVIEW.md)
- [Architecture Implementation Plan](../ARCHITECTURE_IMPLEMENTATION_PLAN.md)
- [TEATEST_API_REFACTORING.md](../TEATEST_API_REFACTORING.md)

**Patterns**:
- Service Layer Pattern (Fowler)
- Facade Pattern (GoF)
- Adapter Pattern (GoF)
- Domain Events (Evans)

---

### C. Contact & Support

**Architecture Questions**: Reference [ARCHITECTURE_REVIEW.md](../ARCHITECTURE_REVIEW.md)  
**Implementation Help**: Reference [ARCHITECTURE_IMPLEMENTATION_PLAN.md](../ARCHITECTURE_IMPLEMENTATION_PLAN.md)  
**Test Issues**: Reference [TEATEST_API_REFACTORING.md](../TEATEST_API_REFACTORING.md)

---

**Plan Version**: 1.0  
**Last Updated**: 2025-12-05  
**Next Review**: After Story 1 completion  
**Plan Status**: 🔴 Not started

