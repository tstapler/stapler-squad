# Architecture Implementation Plan - Claude Squad

**Plan Date**: 2025-12-05
**Based On**: [ARCHITECTURE_REVIEW.md](./ARCHITECTURE_REVIEW.md)
**Overall Goal**: Improve architecture score from 7.8/10 to 9.0/10
**Timeline**: 3 months (Sprint-based approach)

---

## Phase 1: Critical Refactorings (Sprint 1-2) - P0 Priority

### 1.1 Refactor God Object (`home` struct)

**Current Problem**: The `home` struct in app/app.go has 30+ fields spanning multiple responsibilities, violating Single Responsibility Principle.

**Impact**: High - This is the most critical architectural issue
**Effort**: Medium-High (2 weeks)
**Risk**: Medium - Requires careful refactoring with comprehensive tests

#### Step-by-Step Implementation

**Step 1: Extract SessionManagementService (Week 1, Days 1-2)**

```go
// File: app/services/session_management.go
package services

type SessionManagementService struct {
    storage           *session.Storage
    sessionController appsession.Controller
    list              *ui.List
}

func NewSessionManagementService(
    storage *session.Storage,
    controller appsession.Controller,
    list *ui.List,
) *SessionManagementService {
    return &SessionManagementService{
        storage:           storage,
        sessionController: controller,
        list:              list,
    }
}

func (s *SessionManagementService) CreateSession(opts session.InstanceOptions) error {
    // Extract session creation logic from home.Update()
}

func (s *SessionManagementService) KillSession(title string) error {
    // Extract session killing logic from home.Update()
}

func (s *SessionManagementService) AttachSession(title string) error {
    // Extract session attachment logic from home.Update()
}
```

**Files to Modify**:
- Create: `app/services/session_management.go`
- Modify: `app/app.go` (extract methods to service)
- Tests: `app/services/session_management_test.go` (new unit tests)

**Validation**:
```bash
# Run existing tests to ensure no regression
go test ./app -v
go test ./app/services -v
```

---

**Step 2: Extract NavigationService (Week 1, Days 3-4)**

```go
// File: app/services/navigation.go
package services

type NavigationService struct {
    list           *ui.List
    responsiveNav  *ResponsiveNavigationManager
}

func NewNavigationService(list *ui.List) *NavigationService {
    return &NavigationService{
        list:          list,
        responsiveNav: NewResponsiveNavigationManager(150 * time.Millisecond),
    }
}

func (n *NavigationService) NavigateUp() error {
    // Extract navigation logic from home.handleKeyMsg()
    return n.list.PrevInstance()
}

func (n *NavigationService) NavigateDown() error {
    // Extract navigation logic from home.handleKeyMsg()
    return n.list.NextInstance()
}

func (n *NavigationService) NavigateToIndex(idx int) error {
    // Direct navigation to specific index
    return n.list.SetSelectedInstance(idx)
}
```

**Files to Modify**:
- Create: `app/services/navigation.go`
- Modify: `app/app.go` (delegate navigation to service)
- Tests: `app/services/navigation_test.go`

---

**Step 3: Extract FilteringService (Week 1, Days 5-7)**

```go
// File: app/services/filtering.go
package services

type FilteringService struct {
    list         *ui.List
    searchActive bool
    searchQuery  string
}

func NewFilteringService(list *ui.List) *FilteringService {
    return &FilteringService{
        list: list,
    }
}

func (f *FilteringService) TogglePausedFilter() {
    // Extract filter logic from home.Update()
    f.list.TogglePausedFilter()
}

func (f *FilteringService) StartSearch() error {
    // Extract search activation logic
    f.searchActive = true
    return nil
}

func (f *FilteringService) UpdateSearchQuery(query string) {
    // Extract search query update logic
    f.searchQuery = query
    f.list.FilterBySearch(query)
}

func (f *FilteringService) ClearSearch() {
    // Extract search clear logic
    f.searchActive = false
    f.searchQuery = ""
    f.list.ClearSearch()
}
```

**Files to Modify**:
- Create: `app/services/filtering.go`
- Modify: `app/app.go` (delegate filtering to service)
- Tests: `app/services/filtering_test.go`

---

**Step 4: Extract UICoordinationService (Week 2, Days 1-3)**

```go
// File: app/services/ui_coordination.go
package services

type UICoordinationService struct {
    uiCoordinator  appui.Coordinator
    menu           *ui.Menu
    statusBar      *ui.StatusBar
    errBox         *ui.ErrBox
}

func NewUICoordinationService(
    coordinator appui.Coordinator,
    menu *ui.Menu,
    statusBar *ui.StatusBar,
    errBox *ui.ErrBox,
) *UICoordinationService {
    return &UICoordinationService{
        uiCoordinator: coordinator,
        menu:          menu,
        statusBar:     statusBar,
        errBox:        errBox,
    }
}

func (u *UICoordinationService) ShowOverlay(overlay *overlay.SessionSetupOverlay) {
    // Extract overlay management from home
    u.uiCoordinator.SetSessionSetupOverlay(overlay)
}

func (u *UICoordinationService) HideOverlay() {
    // Extract overlay hiding logic
    u.uiCoordinator.SetSessionSetupOverlay(nil)
}

func (u *UICoordinationService) ShowError(err error) {
    // Extract error display logic
    u.errBox.SetError(err)
}
```

**Files to Modify**:
- Create: `app/services/ui_coordination.go`
- Modify: `app/app.go` (delegate UI coordination to service)
- Tests: `app/services/ui_coordination_test.go`

---

**Step 5: Refactor `home` to Facade (Week 2, Days 4-7)**

```go
// File: app/app.go (refactored)
package app

type home struct {
    // Context and cancellation
    ctx        context.Context
    cancelFunc context.CancelFunc

    // Core services (reduced from 30+ fields)
    sessionManagement *services.SessionManagementService
    navigation        *services.NavigationService
    filtering         *services.FilteringService
    uiCoordination    *services.UICoordinationService

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

func newHomeWithDependencies(deps Dependencies) *home {
    // Create services
    sessionManagement := services.NewSessionManagementService(
        deps.GetStorage(),
        // ... other dependencies
    )

    navigation := services.NewNavigationService(deps.GetList())
    filtering := services.NewFilteringService(deps.GetList())
    uiCoordination := services.NewUICoordinationService(
        deps.GetUICoordinator(),
        deps.GetMenu(),
        deps.GetStatusBar(),
        deps.GetErrBox(),
    )

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

// Update becomes a thin delegator
func (h *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return h.handleKeyMsg(msg)
    // ... other cases
    }
}

func (h *home) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    key := msg.String()

    switch key {
    case "k", "up":
        // Delegate to navigation service
        err := h.navigation.NavigateUp()
        if err != nil {
            return h, h.uiCoordination.ShowError(err)
        }
        return h, nil

    case "j", "down":
        // Delegate to navigation service
        err := h.navigation.NavigateDown()
        if err != nil {
            return h, h.uiCoordination.ShowError(err)
        }
        return h, nil

    case "n":
        // Delegate to session management
        err := h.sessionManagement.CreateSession(session.InstanceOptions{})
        if err != nil {
            return h, h.uiCoordination.ShowError(err)
        }
        return h, nil

    // ... other keys delegated to appropriate services
    }
}
```

**Files to Modify**:
- Major refactor: `app/app.go`
- Update: `app/dependencies.go` (add service getters)
- Update all tests: `app/*_test.go`

**Validation**:
```bash
# Run full test suite
go test ./app -v -count=1

# Run integration tests
go test ./app -run TestRobustConfirmationModalFlow -v
go test ./app -run TestSessionCreationOverlayFix -v

# Verify UI still works
./claude-squad
```

---

### 1.2 Decouple Framework Dependencies

**Current Problem**: BubbleTea types (`tea.Model`, `tea.Cmd`) leak into domain layer (app/session/controller.go:23-25).

**Impact**: High - Reduces framework coupling, improves testability
**Effort**: Medium (1 week)
**Risk**: Low - Can be done incrementally

#### Step-by-Step Implementation

**Step 1: Define Domain Events (Days 1-2)**

```go
// File: domain/events/session_events.go
package events

import "time"

// DomainEvent is the base interface for all domain events
type DomainEvent interface {
    EventType() string
    Timestamp() time.Time
}

// SessionCreated is emitted when a new session is created
type SessionCreated struct {
    SessionTitle string
    Path         string
    Program      string
    timestamp    time.Time
}

func (e SessionCreated) EventType() string { return "session.created" }
func (e SessionCreated) Timestamp() time.Time { return e.timestamp }

// SessionKilled is emitted when a session is terminated
type SessionKilled struct {
    SessionTitle string
    timestamp    time.Time
}

func (e SessionKilled) EventType() string { return "session.killed" }
func (e SessionKilled) Timestamp() time.Time { return e.timestamp }

// SessionStatusChanged is emitted when session status changes
type SessionStatusChanged struct {
    SessionTitle string
    OldStatus    string
    NewStatus    string
    timestamp    time.Time
}

func (e SessionStatusChanged) EventType() string { return "session.status_changed" }
func (e SessionStatusChanged) Timestamp() time.Time { return e.timestamp }
```

**Files to Create**:
- `domain/events/session_events.go`
- `domain/events/ui_events.go` (for UI-related events)

---

**Step 2: Replace SessionOperation with Domain Result (Days 3-4)**

```go
// File: app/session/controller.go (refactored)
package session

import (
    "claude-squad/domain/events"
    "claude-squad/session"
)

// SessionResult represents the result of a session operation
type SessionResult struct {
    Type     SessionOperationType
    Success  bool
    Error    error
    Events   []events.DomainEvent  // Replace tea.Model/tea.Cmd
}

// Controller interface (updated)
type Controller interface {
    NewSession() SessionResult     // Changed return type
    KillSession() SessionResult    // Changed return type
    AttachSession() SessionResult  // Changed return type
    // ...
}

// Implementation (example)
func (c *controller) NewSession() SessionResult {
    // ... session creation logic ...

    if err != nil {
        return SessionResult{
            Type:    OpNewSession,
            Success: false,
            Error:   err,
            Events:  nil,
        }
    }

    return SessionResult{
        Type:    OpNewSession,
        Success: true,
        Error:   nil,
        Events: []events.DomainEvent{
            events.SessionCreated{
                SessionTitle: instance.Title,
                Path:         instance.Path,
                Program:      instance.Program,
                timestamp:    time.Now(),
            },
        },
    }
}
```

**Files to Modify**:
- `app/session/controller.go` (replace SessionOperation)
- `app/session/operations.go` (update all operations)

---

**Step 3: Create Event-to-Command Adapter (Days 5-7)**

```go
// File: app/adapters/event_adapter.go
package adapters

import (
    "claude-squad/domain/events"
    tea "github.com/charmbracelet/bubbletea"
)

// EventAdapter converts domain events to BubbleTea commands
type EventAdapter struct{}

func NewEventAdapter() *EventAdapter {
    return &EventAdapter{}
}

// ConvertToCommand converts domain events to tea.Cmd
func (a *EventAdapter) ConvertToCommand(events []events.DomainEvent) tea.Cmd {
    if len(events) == 0 {
        return nil
    }

    // Create batch command for multiple events
    cmds := make([]tea.Cmd, 0, len(events))

    for _, event := range events {
        cmds = append(cmds, a.eventToCommand(event))
    }

    return tea.Batch(cmds...)
}

func (a *EventAdapter) eventToCommand(event events.DomainEvent) tea.Cmd {
    return func() tea.Msg {
        // Convert event to BubbleTea message
        switch e := event.(type) {
        case events.SessionCreated:
            return SessionCreatedMsg{
                Title:   e.SessionTitle,
                Path:    e.Path,
                Program: e.Program,
            }

        case events.SessionKilled:
            return SessionKilledMsg{
                Title: e.SessionTitle,
            }

        // ... other event conversions
        }

        return nil
    }
}

// BubbleTea message types (framework boundary)
type SessionCreatedMsg struct {
    Title   string
    Path    string
    Program string
}

type SessionKilledMsg struct {
    Title string
}
```

**Files to Create**:
- `app/adapters/event_adapter.go`
- `app/adapters/event_adapter_test.go`

---

**Step 4: Update home to Use Adapter (Day 7)**

```go
// File: app/app.go (updated)
package app

type home struct {
    // ... existing fields ...
    eventAdapter *adapters.EventAdapter
}

func newHomeWithDependencies(deps Dependencies) *home {
    return &home{
        // ... existing initialization ...
        eventAdapter: adapters.NewEventAdapter(),
    }
}

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

    // ... other keys
    }
}
```

**Validation**:
```bash
# Run tests
go test ./app/session -v
go test ./app/adapters -v
go test ./domain/events -v

# Integration test
go test ./app -run TestSessionOperations -v
```

---

## Phase 2: Domain Model Strengthening (Sprint 3-4) - P1 Priority

### 2.1 Introduce Value Objects

**Impact**: High - Strengthens domain model with validation and immutability
**Effort**: Medium (1.5 weeks)
**Risk**: Low - Incremental changes

#### Step-by-Step Implementation

**Step 1: Create SessionTitle Value Object (Days 1-2)**

```go
// File: domain/session/title.go
package session

import (
    "errors"
    "strings"
)

// SessionTitle is a value object representing a validated session title
type SessionTitle struct {
    value string
}

// NewSessionTitle creates a new session title with validation
func NewSessionTitle(title string) (SessionTitle, error) {
    // Validation rules
    title = strings.TrimSpace(title)

    if title == "" {
        return SessionTitle{}, errors.New("session title cannot be empty")
    }

    if len(title) > 100 {
        return SessionTitle{}, errors.New("session title cannot exceed 100 characters")
    }

    // Check for invalid characters (e.g., path separators)
    if strings.ContainsAny(title, "/\\:*?\"<>|") {
        return SessionTitle{}, errors.New("session title contains invalid characters")
    }

    return SessionTitle{value: title}, nil
}

// Value returns the underlying string value
func (t SessionTitle) Value() string {
    return t.value
}

// String implements fmt.Stringer
func (t SessionTitle) String() string {
    return t.value
}

// Equals checks equality with another SessionTitle
func (t SessionTitle) Equals(other SessionTitle) bool {
    return t.value == other.value
}
```

**Files to Create**:
- `domain/session/title.go`
- `domain/session/title_test.go` (comprehensive validation tests)

**Files to Modify**:
- `session/instance.go` (use SessionTitle instead of string)

**Migration Strategy**:
```go
// Gradual migration - keep backward compatibility initially
type Instance struct {
    Title     string       // Deprecated: use TitleVO
    TitleVO   SessionTitle // New: value object
    // ...
}

// Constructor enforces value object usage
func NewInstance(opts InstanceOptions) (*Instance, error) {
    titleVO, err := NewSessionTitle(opts.Title)
    if err != nil {
        return nil, err
    }

    return &Instance{
        Title:   opts.Title,    // Keep for backward compat
        TitleVO: titleVO,       // New field
        // ...
    }, nil
}
```

---

**Step 2: Create SessionPath Value Object (Days 3-4)**

```go
// File: domain/session/path.go
package session

import (
    "errors"
    "os"
    "path/filepath"
)

// SessionPath is a value object representing a validated file system path
type SessionPath struct {
    value string
}

// NewSessionPath creates a validated session path
func NewSessionPath(path string) (SessionPath, error) {
    if path == "" {
        return SessionPath{}, errors.New("session path cannot be empty")
    }

    // Convert to absolute path
    absPath, err := filepath.Abs(path)
    if err != nil {
        return SessionPath{}, errors.New("invalid path: " + err.Error())
    }

    // Check if path is accessible (not necessarily exists yet)
    parentDir := filepath.Dir(absPath)
    if _, err := os.Stat(parentDir); os.IsNotExist(err) {
        return SessionPath{}, errors.New("parent directory does not exist")
    }

    return SessionPath{value: absPath}, nil
}

// Value returns the absolute path
func (p SessionPath) Value() string {
    return p.value
}

// Exists checks if the path exists on the file system
func (p SessionPath) Exists() bool {
    _, err := os.Stat(p.value)
    return !os.IsNotExist(err)
}

// IsDirectory checks if the path is a directory
func (p SessionPath) IsDirectory() bool {
    info, err := os.Stat(p.value)
    if err != nil {
        return false
    }
    return info.IsDir()
}
```

---

**Step 3: Create SessionStatus Value Object (Days 5-6)**

```go
// File: domain/session/status.go
package session

import "errors"

// SessionStatus is a value object representing session lifecycle status
type SessionStatus struct {
    value Status
}

// Status enum type (keep existing for backward compat)
type Status int

const (
    StatusUnknown Status = iota
    StatusRunning
    StatusPaused
    StatusStopped
    StatusReady
)

// NewSessionStatus creates a validated session status
func NewSessionStatus(status Status) (SessionStatus, error) {
    if status < StatusUnknown || status > StatusReady {
        return SessionStatus{}, errors.New("invalid session status")
    }

    return SessionStatus{value: status}, nil
}

// Value returns the underlying status
func (s SessionStatus) Value() Status {
    return s.value
}

// IsActive checks if session is in an active state
func (s SessionStatus) IsActive() bool {
    return s.value == StatusRunning
}

// IsPaused checks if session is paused
func (s SessionStatus) IsPaused() bool {
    return s.value == StatusPaused
}

// CanTransitionTo validates state transitions
func (s SessionStatus) CanTransitionTo(target SessionStatus) error {
    // Define valid state transitions
    validTransitions := map[Status][]Status{
        StatusReady:   {StatusRunning},
        StatusRunning: {StatusPaused, StatusStopped},
        StatusPaused:  {StatusRunning, StatusStopped},
        StatusStopped: {StatusReady},
    }

    allowed, ok := validTransitions[s.value]
    if !ok {
        return errors.New("no valid transitions from current state")
    }

    for _, allowedStatus := range allowed {
        if allowedStatus == target.value {
            return nil
        }
    }

    return errors.New("invalid state transition")
}
```

---

**Step 4: Create Tag Value Object (Day 7)**

```go
// File: domain/session/tag.go
package session

import (
    "errors"
    "strings"
)

// Tag is a value object representing a normalized session tag
type Tag struct {
    value string
}

// NewTag creates a validated and normalized tag
func NewTag(tag string) (Tag, error) {
    // Normalize: trim, lowercase
    normalized := strings.TrimSpace(strings.ToLower(tag))

    if normalized == "" {
        return Tag{}, errors.New("tag cannot be empty")
    }

    if len(normalized) > 50 {
        return Tag{}, errors.New("tag cannot exceed 50 characters")
    }

    // Validate characters (alphanumeric + dash/underscore)
    if !isValidTagFormat(normalized) {
        return Tag{}, errors.New("tag must contain only alphanumeric, dash, or underscore")
    }

    return Tag{value: normalized}, nil
}

func isValidTagFormat(s string) bool {
    for _, r := range s {
        if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
            return false
        }
    }
    return true
}

// Value returns the normalized tag value
func (t Tag) Value() string {
    return t.value
}

// Equals checks equality with another tag
func (t Tag) Equals(other Tag) bool {
    return t.value == other.value
}
```

**Validation**:
```bash
# Run value object tests
go test ./domain/session -v

# Integration tests
go test ./session -v
```

---

### 2.2 Define Aggregate Boundaries

**Impact**: High - Enforces invariants and prevents data corruption
**Effort**: Medium (1 week)
**Risk**: Medium - Requires careful refactoring

#### Step-by-Step Implementation

**Step 1: Identify Aggregate Root (Days 1-2)**

`session.Instance` is the aggregate root. All operations must go through `Instance` methods.

**Current Problem**:
- Direct database access to `WorktreeInfo`, `ClaudeSession`, `Tag` entities
- No enforcement of aggregate invariants

**Solution**: Encapsulate child entities

```go
// File: session/instance.go (refactored)
package session

type Instance struct {
    // Identity (using value objects from Phase 2.1)
    titleVO  SessionTitle
    statusVO SessionStatus
    pathVO   SessionPath

    // Child entities (private - enforce encapsulation)
    worktree      *WorktreeInfo
    claudeSession *ClaudeSession
    tags          []Tag  // Value objects

    // Metadata
    createdAt time.Time
    updatedAt time.Time

    // ... other fields
}

// Public getters (read-only access)
func (i *Instance) Title() SessionTitle {
    return i.titleVO
}

func (i *Instance) Status() SessionStatus {
    return i.statusVO
}

func (i *Instance) Path() SessionPath {
    return i.pathVO
}

// Aggregate methods (enforce invariants)
func (i *Instance) AddTag(tag Tag) error {
    // Check for duplicates (aggregate invariant)
    for _, existing := range i.tags {
        if existing.Equals(tag) {
            return errors.New("tag already exists")
        }
    }

    // Enforce maximum tags (business rule)
    if len(i.tags) >= 10 {
        return errors.New("maximum 10 tags per session")
    }

    i.tags = append(i.tags, tag)
    i.updatedAt = time.Now()
    return nil
}

func (i *Instance) RemoveTag(tag Tag) error {
    // Find and remove tag
    for idx, existing := range i.tags {
        if existing.Equals(tag) {
            i.tags = append(i.tags[:idx], i.tags[idx+1:]...)
            i.updatedAt = time.Now()
            return nil
        }
    }
    return errors.New("tag not found")
}

// State transition (enforce rules via value object)
func (i *Instance) Pause() error {
    newStatus, err := NewSessionStatus(StatusPaused)
    if err != nil {
        return err
    }

    // Check if transition is valid
    if err := i.statusVO.CanTransitionTo(newStatus); err != nil {
        return err
    }

    i.statusVO = newStatus
    i.updatedAt = time.Now()
    return nil
}
```

**Files to Modify**:
- `session/instance.go` (encapsulate fields, add aggregate methods)
- `session/repository.go` (update to work with aggregate boundaries)

---

**Step 2: Update Repository to Respect Aggregates (Days 3-5)**

```go
// File: session/repository.go (refactored)
package session

// Repository interface updated
type Repository interface {
    // Aggregate-level operations (no direct child entity access)
    Create(ctx context.Context, instance *Instance) error
    Update(ctx context.Context, instance *Instance) error
    Delete(ctx context.Context, title SessionTitle) error
    Get(ctx context.Context, title SessionTitle) (*Instance, error)

    // Query methods (return full aggregates)
    ListByStatus(ctx context.Context, status SessionStatus) ([]*Instance, error)
    ListByTag(ctx context.Context, tag Tag) ([]*Instance, error)

    // Removed: Direct child entity access
    // ❌ UpdateWorktree(ctx, title, worktree)
    // ❌ AddTag(ctx, title, tag)
}
```

**Implementation Strategy**:
- Repository loads entire aggregate from database
- Changes are made via aggregate methods
- Repository persists entire aggregate (or uses unit of work)

---

**Step 3: Implement Unit of Work Pattern (Days 6-7)**

```go
// File: session/unit_of_work.go
package session

type UnitOfWork interface {
    // Register aggregate for tracking
    RegisterNew(instance *Instance)
    RegisterDirty(instance *Instance)
    RegisterDeleted(instance *Instance)

    // Commit all changes in transaction
    Commit(ctx context.Context) error

    // Rollback changes
    Rollback() error
}

type unitOfWork struct {
    repository Repository
    newItems   []*Instance
    dirtyItems []*Instance
    deleted    []SessionTitle
}

func NewUnitOfWork(repo Repository) UnitOfWork {
    return &unitOfWork{
        repository: repo,
        newItems:   make([]*Instance, 0),
        dirtyItems: make([]*Instance, 0),
        deleted:    make([]SessionTitle, 0),
    }
}

func (u *unitOfWork) Commit(ctx context.Context) error {
    // Begin transaction (database-specific)
    // ...

    // Insert new instances
    for _, instance := range u.newItems {
        if err := u.repository.Create(ctx, instance); err != nil {
            return err
        }
    }

    // Update modified instances
    for _, instance := range u.dirtyItems {
        if err := u.repository.Update(ctx, instance); err != nil {
            return err
        }
    }

    // Delete instances
    for _, title := range u.deleted {
        if err := u.repository.Delete(ctx, title); err != nil {
            return err
        }
    }

    // Commit transaction
    // ...

    return nil
}
```

**Validation**:
```bash
# Test aggregate invariants
go test ./session -run TestAggregateInvariants -v

# Test repository with aggregates
go test ./session -run TestRepositoryAggregates -v
```

---

## Phase 3: Extract Pure Domain Layer (Sprint 5-6) - P1 Priority

### 3.1 Create Domain Package

**Impact**: High - Eliminates framework coupling in domain
**Effort**: High (2 weeks)
**Risk**: Medium - Large refactoring

#### Step-by-Step Implementation

**Step 1: Create Domain Structure (Days 1-2)**

```
domain/
├── session/
│   ├── aggregate.go       # Instance aggregate root
│   ├── title.go          # SessionTitle value object
│   ├── path.go           # SessionPath value object
│   ├── status.go         # SessionStatus value object
│   ├── tag.go            # Tag value object
│   ├── repository.go     # Repository interface
│   └── errors.go         # Domain errors
├── events/
│   ├── session_events.go # Session domain events
│   └── event.go          # Event interface
└── services/
    ├── session_service.go # Domain services
    └── validation.go      # Validation rules
```

**Create files with NO external dependencies** (pure Go only):
- No BubbleTea imports
- No UI imports
- No infrastructure imports

---

**Step 2: Move Value Objects to Domain (Days 3-4)**

Move all value objects created in Phase 2.1 to `domain/session/`:
- `domain/session/title.go`
- `domain/session/path.go`
- `domain/session/status.go`
- `domain/session/tag.go`

**Validation**: Ensure no framework dependencies
```bash
# Check imports - should only see "errors", "time", "strings", etc.
grep -r "import" domain/ | grep -v "errors\|time\|strings\|fmt\|context"
```

---

**Step 3: Move Repository Interface to Domain (Days 5-6)**

```go
// File: domain/session/repository.go
package session

import "context"

// Repository defines pure domain persistence interface
// NO framework dependencies (no tea.Model, no protobuf, etc.)
type Repository interface {
    Create(ctx context.Context, instance *Instance) error
    Update(ctx context.Context, instance *Instance) error
    Delete(ctx context.Context, title Title) error
    Get(ctx context.Context, title Title) (*Instance, error)
    List(ctx context.Context) ([]*Instance, error)
}
```

**Move**: `session/repository.go` → `domain/session/repository.go`

---

**Step 4: Create Infrastructure Adapters (Days 7-10)**

```go
// File: infrastructure/persistence/sqlite_repository.go
package persistence

import (
    "claude-squad/domain/session"
    // OK to import infrastructure dependencies here
)

// SQLiteRepository implements domain.session.Repository
type SQLiteRepository struct {
    db *sql.DB
}

func (r *SQLiteRepository) Create(ctx context.Context, instance *session.Instance) error {
    // Adapt domain.Instance to database schema
    // Convert value objects to primitive types for DB
    title := instance.Title().Value()
    path := instance.Path().Value()
    status := instance.Status().Value()

    // Execute SQL
    _, err := r.db.ExecContext(ctx, "INSERT INTO sessions ...")
    return err
}
```

**Directory Structure**:
```
infrastructure/
├── persistence/
│   ├── sqlite_repository.go      # SQLite implementation
│   ├── ent_repository.go         # Ent ORM implementation
│   └── repository_adapter.go     # Adapter pattern
└── messaging/
    └── event_bus.go               # Event bus implementation
```

---

**Step 5: Update Imports Throughout Codebase (Days 11-14)**

**Before**:
```go
import "claude-squad/session"
```

**After**:
```go
import (
    "claude-squad/domain/session"
    "claude-squad/infrastructure/persistence"
)
```

**Files to Update** (estimate 50+ files):
- All `app/*.go` files
- All `server/*.go` files
- All test files

**Incremental Strategy**:
1. Use `go mod` replace to create alias during migration
2. Update one package at a time
3. Run tests after each package migration
4. Remove alias when complete

**Validation**:
```bash
# Verify domain has no framework dependencies
cd domain && go list -f '{{.Imports}}' ./... | grep -E "bubbletea|lipgloss|proto"
# Should return nothing

# Run full test suite
go test ./...
```

---

## Phase 4: Introduce Domain Events (Sprint 7) - P2 Priority

### 4.1 Implement Event Bus

**Impact**: Medium - Enables decoupled communication
**Effort**: Medium (1 week)
**Risk**: Low

#### Step-by-Step Implementation

**Step 1: Create Event Bus Interface (Days 1-2)**

```go
// File: domain/events/event_bus.go
package events

import "context"

// EventBus defines the interface for publishing and subscribing to domain events
type EventBus interface {
    Publish(ctx context.Context, event DomainEvent) error
    Subscribe(eventType string, handler EventHandler) error
    Unsubscribe(eventType string, handler EventHandler) error
}

// EventHandler processes domain events
type EventHandler interface {
    Handle(ctx context.Context, event DomainEvent) error
    CanHandle(eventType string) bool
}

// DomainEvent interface (already defined in Phase 1.2)
type DomainEvent interface {
    EventType() string
    Timestamp() time.Time
    AggregateID() string  // Add for event sourcing
}
```

---

**Step 2: Implement In-Memory Event Bus (Days 3-4)**

```go
// File: infrastructure/messaging/memory_event_bus.go
package messaging

import (
    "claude-squad/domain/events"
    "context"
    "sync"
)

type memoryEventBus struct {
    handlers map[string][]events.EventHandler
    mu       sync.RWMutex
}

func NewMemoryEventBus() events.EventBus {
    return &memoryEventBus{
        handlers: make(map[string][]events.EventHandler),
    }
}

func (b *memoryEventBus) Publish(ctx context.Context, event events.DomainEvent) error {
    b.mu.RLock()
    defer b.mu.RUnlock()

    handlers, ok := b.handlers[event.EventType()]
    if !ok {
        return nil // No handlers, not an error
    }

    // Execute handlers concurrently
    var wg sync.WaitGroup
    errChan := make(chan error, len(handlers))

    for _, handler := range handlers {
        wg.Add(1)
        go func(h events.EventHandler) {
            defer wg.Done()
            if err := h.Handle(ctx, event); err != nil {
                errChan <- err
            }
        }(handler)
    }

    wg.Wait()
    close(errChan)

    // Collect errors
    for err := range errChan {
        if err != nil {
            return err // Return first error
        }
    }

    return nil
}

func (b *memoryEventBus) Subscribe(eventType string, handler events.EventHandler) error {
    b.mu.Lock()
    defer b.mu.Unlock()

    b.handlers[eventType] = append(b.handlers[eventType], handler)
    return nil
}
```

---

**Step 3: Create Event Handlers (Days 5-6)**

```go
// File: app/handlers/session_event_handler.go
package handlers

import (
    "claude-squad/domain/events"
    "context"
)

// SessionEventHandler handles session-related events
type SessionEventHandler struct {
    uiUpdater UIUpdater  // Interface for updating UI
}

func NewSessionEventHandler(updater UIUpdater) *SessionEventHandler {
    return &SessionEventHandler{
        uiUpdater: updater,
    }
}

func (h *SessionEventHandler) Handle(ctx context.Context, event events.DomainEvent) error {
    switch e := event.(type) {
    case events.SessionCreated:
        return h.handleSessionCreated(e)

    case events.SessionKilled:
        return h.handleSessionKilled(e)

    case events.SessionStatusChanged:
        return h.handleStatusChanged(e)
    }

    return nil
}

func (h *SessionEventHandler) CanHandle(eventType string) bool {
    return eventType == "session.created" ||
           eventType == "session.killed" ||
           eventType == "session.status_changed"
}

func (h *SessionEventHandler) handleSessionCreated(e events.SessionCreated) error {
    // Update UI to show new session
    return h.uiUpdater.RefreshSessionList()
}
```

---

**Step 4: Integrate Event Bus into Application (Day 7)**

```go
// File: app/app.go (updated)
type home struct {
    // ... existing fields ...
    eventBus events.EventBus
}

func newHomeWithDependencies(deps Dependencies) *home {
    // Create event bus
    eventBus := messaging.NewMemoryEventBus()

    // Register event handlers
    sessionHandler := handlers.NewSessionEventHandler(/* UI updater */)
    eventBus.Subscribe("session.created", sessionHandler)
    eventBus.Subscribe("session.killed", sessionHandler)
    eventBus.Subscribe("session.status_changed", sessionHandler)

    return &home{
        // ... existing initialization ...
        eventBus: eventBus,
    }
}

// In Update method
func (h *home) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "n":
        // Create session
        result := h.sessionManagement.CreateSession(opts)

        // Publish domain events
        for _, event := range result.Events {
            h.eventBus.Publish(h.ctx, event)
        }

        return h, nil
    }
}
```

**Validation**:
```bash
# Test event bus
go test ./infrastructure/messaging -v

# Test event handlers
go test ./app/handlers -v

# Integration test
go test ./app -run TestEventDrivenFlow -v
```

---

## Success Metrics

Track these metrics throughout implementation:

### Code Quality Metrics
- **Cyclomatic Complexity**: Reduce `home.Update()` from 30+ to <10
- **Lines per Method**: Reduce average from 100+ to <50
- **Test Coverage**: Increase from 60% to 80%+
- **God Object Score**: Reduce `home` struct from 30+ fields to <10

### Architecture Metrics
- **Dependency Graph**: Measure coupling with `go mod graph`
- **Interface Adherence**: 100% of public APIs use interfaces
- **Framework Coupling**: Zero framework imports in `domain/` package
- **Event-Driven**: 90%+ of state changes emit domain events

### Developer Experience Metrics
- **Build Time**: Should remain <10s
- **Test Suite Time**: Should remain <30s
- **Refactoring Safety**: Zero regressions during refactoring
- **Onboarding Time**: Reduce from 2 days to 1 day (better architecture)

---

## Risk Mitigation

### High-Risk Areas

**1. Breaking Changes During God Object Refactoring**
- **Mitigation**: Comprehensive test suite before refactoring
- **Rollback Plan**: Feature branch with pre-refactor snapshot
- **Validation**: Run full test suite after each service extraction

**2. Framework Coupling Removal**
- **Mitigation**: Incremental adapter pattern introduction
- **Rollback Plan**: Keep old code paths until adapters proven
- **Validation**: Parallel running of old and new paths

**3. Repository Aggregate Boundary Changes**
- **Mitigation**: Database migration scripts with rollback
- **Rollback Plan**: Dual-write to old and new schemas
- **Validation**: Data integrity tests before cutover

### Communication Strategy

**Weekly Architecture Reviews**: Share progress with team
**Pair Programming**: Complex refactorings done in pairs
**Documentation**: Update architecture docs incrementally
**Stakeholder Updates**: Biweekly demos of improvements

---

## Conclusion

This implementation plan provides a **systematic, incremental approach** to improving the architecture from 7.8/10 to 9.0/10 over 7 sprints (3 months).

**Key Success Factors**:
1. ✅ Incremental refactoring with continuous validation
2. ✅ Comprehensive test coverage before major changes
3. ✅ Clear rollback plans for high-risk refactorings
4. ✅ Parallel implementation during transition periods
5. ✅ Team collaboration and code review

**Expected Outcomes**:
- 🎯 Reduced technical debt
- 🎯 Improved maintainability and testability
- 🎯 Clearer separation of concerns
- 🎯 Stronger domain model with enforced invariants
- 🎯 Reduced framework coupling for long-term flexibility

**Next Steps**:
1. Review this plan with the team
2. Prioritize P0 tasks for Sprint 1
3. Set up tracking metrics (Jira/Linear)
4. Begin Phase 1.1 implementation

---

**Plan Maintained By**: Architecture Team
**Last Updated**: 2025-12-05
**Next Review**: After Sprint 2 (Week 4)
