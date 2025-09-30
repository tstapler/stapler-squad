# List Component Refactor: Elm Architecture Design

## Current Issues

1. **Mixed Concerns**: Single struct handles data, UI state, filtering, scrolling, caching, and persistence
2. **Direct Mutation**: State changes happen directly without centralized update flow
3. **Complex State**: Multiple maps and caches that can become inconsistent
4. **No Message Pattern**: Doesn't follow BubbleTea's message-based updates
5. **Tight Coupling**: Renderer, persistence, and logic are intertwined

## Proposed Architecture

### 1. Model Separation

```go
// Core data model - immutable business logic
type SessionListModel struct {
    sessions []session.Instance
    repos    map[string][]session.Instance // Group by repository
}

// UI state model - all view-specific state
type ListViewState struct {
    selectedIndex    int
    scrollOffset     int
    searchQuery      string
    searchActive     bool
    hidePaused       bool
    expandedGroups   map[string]bool
    organizationMode OrganizationMode // Category, Repository, Flat
}

// Display model - computed view data
type ListDisplayModel struct {
    visibleItems     []DisplayItem
    totalItems       int
    scrollIndicators ScrollInfo
    groupHeaders     []GroupHeader
}

// Main component following BubbleTea pattern
type ListComponent struct {
    model       SessionListModel
    viewState   ListViewState
    displayData ListDisplayModel
    renderer    *Renderer
}
```

### 2. Message-Based Updates

```go
type ListMessage interface {
    IsListMessage()
}

// Navigation messages
type SelectNextMsg struct{}
type SelectPrevMsg struct{}
type ScrollToIndexMsg struct{ Index int }

// Data messages
type AddSessionMsg struct{ Session *session.Instance }
type RemoveSessionMsg struct{ Index int }
type UpdateSessionMsg struct{ Index int, Session *session.Instance }

// UI state messages
type ToggleSearchMsg struct{}
type SetSearchQueryMsg struct{ Query string }
type TogglePausedFilterMsg struct{}
type ToggleGroupMsg struct{ GroupID string }
type ChangeOrganizationMsg struct{ Mode OrganizationMode }

// Update method following Elm pattern
func (lc *ListComponent) Update(msg tea.Msg) (*ListComponent, tea.Cmd) {
    switch msg := msg.(type) {
    case SelectNextMsg:
        return lc.handleNavigation(1), nil
    case AddSessionMsg:
        return lc.handleAddSession(msg.Session), nil
    case ToggleSearchMsg:
        return lc.handleToggleSearch(), nil
    // ... handle all message types
    }
    return lc, nil
}
```

### 3. Pure Functions for State Updates

```go
// Pure functions that return new state instead of mutating
func selectNext(model SessionListModel, viewState ListViewState) ListViewState {
    visibleItems := getVisibleItems(model, viewState)
    if len(visibleItems) == 0 {
        return viewState
    }

    newIndex := (viewState.selectedIndex + 1) % len(visibleItems)
    return ListViewState{
        ...viewState,
        selectedIndex: newIndex,
    }
}

func toggleSearch(viewState ListViewState) ListViewState {
    return ListViewState{
        ...viewState,
        searchActive: !viewState.searchActive,
        searchQuery: "", // Reset query when toggling
    }
}
```

### 4. Hierarchical Organization

```go
type OrganizationMode int

const (
    OrganizeByCategory OrganizationMode = iota
    OrganizeByRepository
    OrganizeFlat
)

type GroupHeader struct {
    ID          string
    Title       string
    Count       int
    Expanded    bool
    Level       int // For nested hierarchies
    Type        GroupType // Category, Repository, etc.
}

// Repository-based organization
func organizeByRepository(sessions []session.Instance) map[string][]session.Instance {
    repos := make(map[string][]session.Instance)

    for _, session := range sessions {
        repoName := extractRepositoryName(session)
        repos[repoName] = append(repos[repoName], session)
    }

    return repos
}

// Nested hierarchy: Repository > Category > Sessions
func organizeHierarchical(sessions []session.Instance) HierarchicalView {
    // Group by repo first, then by category within each repo
    return buildHierarchy(sessions)
}
```

### 5. Renderer Separation

```go
// Pure rendering logic separated from state management
type Renderer struct {
    width     int
    height    int
    styles    *StyleConfig
}

func (r *Renderer) RenderList(display ListDisplayModel, viewState ListViewState) string {
    // Pure function that takes display data and renders it
    // No access to mutable state
}

func (r *Renderer) RenderSession(session session.Instance, index int, selected bool) string {
    // Pure session rendering
}
```

## Migration Plan

### Phase 1: Extract Message Types
- Define all message types for list operations
- Create Update method framework
- Keep existing functionality working

### Phase 2: Separate Models
- Extract SessionListModel from current List struct
- Extract ListViewState
- Create pure update functions

### Phase 3: Repository Hierarchy
- Implement repository-based grouping
- Add hierarchical organization options
- Update UI to show repo groups

### Phase 4: Renderer Cleanup
- Make renderer pure (no state access)
- Separate display logic from business logic
- Clean up caching complexity

### Phase 5: Testing & Polish
- Update all tests for new architecture
- Performance testing
- Documentation

## Benefits

1. **Predictable State**: All state changes go through Update method
2. **Testable**: Pure functions are easy to test
3. **Maintainable**: Clear separation of concerns
4. **Extensible**: Easy to add new organization modes
5. **Performance**: Better caching through immutable data
6. **BubbleTea Compliant**: Follows framework patterns properly

## Repository Hierarchy Design

```
📁 claude-squad-repo
├── 🏷️  Work (category)
│   ├── feat/auth-system
│   └── fix/login-bug
└── 🏷️  Personal (category)
    └── docs/readme-update

📁 my-project-repo
├── 🏷️  Development
│   ├── main
│   └── feature/new-ui
└── 🏷️  Uncategorized
    └── hotfix/critical-patch
```

This provides much better organization for users with multiple projects.