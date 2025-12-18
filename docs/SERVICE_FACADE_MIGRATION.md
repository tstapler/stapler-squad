# Service Facade Migration Guide

## Overview

Phase 1 of the architecture refactoring has successfully extracted core services from the God Object `home` struct and introduced a Facade pattern for cleaner access. This document guides the gradual migration from direct field access to facade-based access.

## Completed Work (Phase 1)

### Services Extracted

1. **SessionManagementService** (`app/services/session_management.go`)
   - Session lifecycle operations (create, kill, attach, resume, checkout)
   - Validation and queries
   - Thread-safe with mutex protection

2. **NavigationService** (`app/services/navigation.go`)
   - Navigation operations (up, down, page up/down, home, end)
   - Debounced updates (150ms default) for performance
   - Thread-safe navigation state

3. **FilteringService** (`app/services/filtering.go`)
   - Search and filter operations
   - Paused filter toggle
   - Thread-safe filter state management

4. **UICoordinationService** (`app/services/ui_coordination.go`)
   - Overlay management (session setup, etc.)
   - Error handling
   - Menu and status bar updates
   - Thread-safe UI state

5. **Facade** (`app/services/facade.go`)
   - Unified interface to all services
   - Convenience delegation methods
   - Thread-safe service access

### Integration

The facade has been integrated into the `home` struct at `app/app.go:66`:

```go
// services provides unified access to all application services
// This facade reduces coupling and simplifies testing
// TODO: Gradually migrate direct field access to use this facade
services services.Facade
```

Initialized at `app/app.go:278-290`.

## Migration Strategy (Strangler Fig Pattern)

The facade exists alongside the old direct field access, allowing for gradual migration without breaking existing functionality.

### Current State

```go
// Old way (still works)
m.list.SearchByTitle(query)
m.list.TogglePausedFilter()

// New way (via facade)
m.services.Filtering().UpdateSearchQuery(query)
m.services.Filtering().TogglePausedFilter()
```

### Migration Steps

#### Step 1: Identify High-Value Refactoring Targets

Focus on areas with:
- High complexity (many conditional branches)
- Frequent changes
- Testing difficulties
- Performance issues

Priority areas:
1. Key event handlers in `app.go` (handleKey*, handleNavigation*)
2. Session creation and lifecycle methods
3. Filter and search functionality

#### Step 2: Migrate Method by Method

For each method to migrate:

1. **Update the implementation** to use facade:
   ```go
   // Before
   func (m *home) handleFilterPaused() (tea.Model, tea.Cmd) {
       m.list.TogglePausedFilter()
       return m, nil
   }

   // After
   func (m *home) handleFilterPaused() (tea.Model, tea.Cmd) {
       m.services.Filtering().TogglePausedFilter()
       return m, nil
   }
   ```

2. **Run tests** to ensure no regressions:
   ```bash
   go test ./app -run TestHandleFilterPaused -v
   ```

3. **Update related tests** to use facade if beneficial:
   ```go
   // Tests can now inject mock services via facade
   mockFacade := &MockFacade{
       filtering: NewMockFilteringService(),
   }
   home := &home{services: mockFacade}
   ```

#### Step 3: Remove Direct Field Access (Future)

Once all usages are migrated, remove the redundant fields:

```go
// Before (app.go:117-119)
list *ui.List
menu *ui.Menu
statusBar *ui.StatusBar

// After - These can be removed once fully migrated
// Access via: m.services.UICoordination().GetMenu(), etc.
```

### Example Migration: Navigation Handlers

**Target:** `handleNavigationUp()` and `handleNavigationDown()` in `app/app.go:923-956`

**Current Implementation:**
```go
func (m *home) handleNavigationUp() (tea.Model, tea.Cmd) {
    if m.viewMode == ViewModePTYs {
        // ... PTY logic
    }
    if m.isAtNavigationStart() {
        return m, nil
    }
    m.navigateUp()
    return m, m.instanceChanged()
}
```

**After Migration:**
```go
func (m *home) handleNavigationUp() (tea.Model, tea.Cmd) {
    if m.viewMode == ViewModePTYs {
        // ... PTY logic (unchanged)
    }

    // Use facade for navigation
    if m.services.Navigation().GetCurrentIndex() == 0 {
        return m, nil
    }

    if err := m.services.Navigation().NavigateUp(); err != nil {
        return m, m.services.ShowError(err)
    }

    return m, m.instanceChanged()
}
```

**Benefits:**
- Clearer abstraction boundary
- Easier to test navigation in isolation
- Debouncing logic encapsulated in service
- Thread-safe by default

### Example Migration: Filter Handlers

**Target:** `handleFilterPaused()` in `app/app.go:1107-1108`

**Current Implementation:**
```go
func (m *home) handleFilterPaused() (tea.Model, tea.Cmd) {
    m.list.TogglePausedFilter()
    return m, nil
}
```

**After Migration:**
```go
func (m *home) handleFilterPaused() (tea.Model, tea.Cmd) {
    if err := m.services.Filtering().TogglePausedFilter(); err != nil {
        return m, m.services.ShowError(err)
    }
    return m, nil
}
```

**Benefits:**
- Consistent error handling
- Thread-safe filter operations
- Service can track filter state independently

## Testing Strategy

### Unit Testing Services

Each service has comprehensive unit tests:

```bash
# Run all service tests
go test ./app/services/... -v

# Run specific service tests
go test ./app/services -run TestFilteringService -v
go test ./app/services -run TestNavigationService -v
```

### Integration Testing

Test facade integration in `app` package:

```bash
# Run app integration tests
go test ./app -v

# Focus on specific integration points
go test ./app -run TestFacadeIntegration -v
```

### Regression Testing

After each migration:

1. Run full test suite:
   ```bash
   go test ./... -timeout=30s
   ```

2. Perform manual smoke tests:
   ```bash
   go build . && ./claude-squad
   # Test: navigation (j/k), filtering (f), search (s)
   ```

3. Run benchmarks to check performance impact:
   ```bash
   go test -bench=. -benchmem ./app -timeout=10m &
   ```

## Benefits of Facade Pattern

### 1. Reduced Coupling
- `home` struct reduced from 40+ fields to core dependencies + facade
- Services don't need to know about each other
- Changes to service implementations don't affect `home` struct

### 2. Improved Testability
- Services can be tested in complete isolation
- Mock facades can be injected for unit tests
- Integration tests focus on facade coordination

### 3. Better Thread Safety
- Each service manages its own mutex
- No risk of deadlocks from cross-service calls
- Clear ownership of state

### 4. Clearer Responsibilities
- Each service has a single, well-defined responsibility
- Service interfaces document available operations
- Facade provides unified access without coupling

## Next Steps (Phase 2 & 3)

### Phase 2: Decouple Framework Dependencies
- Extract BubbleTea-specific logic into adapters
- Create framework-agnostic service interfaces
- Make services testable without BubbleTea runtime

### Phase 3: Integration & Validation
- Comprehensive integration tests
- Performance benchmarking
- Documentation updates
- Gradual rollout to production

## Performance Considerations

### Debouncing in NavigationService

The NavigationService uses a 150ms debounce delay to prevent expensive operations during rapid key presses:

```go
// Configurable debounce delay
m.services.Navigation().SetDebounceDelay(200 * time.Millisecond)
```

### Lock Contention

Services use read-write locks (`sync.RWMutex`) where appropriate:
- Read operations use `RLock()` for concurrent access
- Write operations use `Lock()` for exclusive access

Monitor lock contention with profiling:
```bash
./claude-squad --profile
curl http://localhost:6060/debug/pprof/mutex?debug=1
```

## Troubleshooting

### Issue: Compilation errors after migration

**Symptom:** `undefined: m.list` or similar errors

**Solution:** Check if you're using direct field access instead of facade. Update to:
```go
// Before: m.list.Something()
// After: m.services.Navigation().Something() or other appropriate service
```

### Issue: Tests fail after migration

**Symptom:** Tests that worked before now fail

**Solution:**
1. Check if test setup initializes the facade
2. Verify mock services implement the correct interfaces
3. Ensure service initialization order is correct

### Issue: Performance regression

**Symptom:** Navigation feels slower after migration

**Solution:**
1. Check debounce delay: `m.services.Navigation().GetDebounceDelay()`
2. Profile with: `go test -bench=BenchmarkNavigation -benchmem ./app`
3. Adjust debounce or disable for specific operations

## References

- Architecture Review: `ARCHITECTURE_REVIEW.md`
- Implementation Plan: `ARCHITECTURE_IMPLEMENTATION_PLAN.md`
- Service Tests: `app/services/*_test.go`
- Main Application: `app/app.go`

## Questions or Issues?

If you encounter issues during migration:
1. Check this guide for similar scenarios
2. Review service interfaces in `app/services/*.go`
3. Look at test examples in `app/services/*_test.go`
4. Create an issue with details if needed
