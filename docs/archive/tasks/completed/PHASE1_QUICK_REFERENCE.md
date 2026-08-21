# Phase 1 Quick Reference Guide

Fast reference for developers working on architecture refactoring.

---

## Task Checklist

### Story 1: Extract Core Services

- [ ] **Task 1.1**: SessionManagementService (4-6h)
  - Files: `app/services/session_management.go`, `app/app.go`, `app/dependencies.go`
  - Test: `go test ./app/services -run TestSessionManagementService -v`
  
- [ ] **Task 1.2**: NavigationService (3-5h)
  - Files: `app/services/navigation.go`, `app/app.go`, `app/dependencies.go`
  - Test: `go test ./app/services -run TestNavigationService -v`
  
- [ ] **Task 1.3**: FilteringService (3-4h)
  - Files: `app/services/filtering.go`, `app/app.go`, `app/dependencies.go`
  - Test: `go test ./app/services -run TestFilteringService -v`
  
- [ ] **Task 1.4**: UICoordinationService (3-5h)
  - Files: `app/services/ui_coordination.go`, `app/app.go`, `app/dependencies.go`
  - Test: `go test ./app/services -run TestUICoordinationService -v`
  
- [ ] **Task 1.5**: Refactor home to Facade (4-8h)
  - Files: `app/app.go` (major refactor), `app/dependencies.go`, `app/test_helpers.go`
  - Test: `go test ./app -v -count=1`

### Story 2: Decouple Framework

- [ ] **Task 2.1**: Define Domain Events (2-3h)
  - Files: `domain/events/session_events.go`, `domain/events/event.go`
  - Test: `go test ./domain/events -v`
  
- [ ] **Task 2.2**: Replace SessionOperation (3-4h)
  - Files: `app/session/controller.go`, `app/session/operations.go`
  - Test: `go test ./app/session -v`
  
- [ ] **Task 2.3**: Create EventAdapter (3-4h)
  - Files: `app/adapters/event_adapter.go`, `app/adapters/event_adapter_test.go`
  - Test: `go test ./app/adapters -v`
  
- [ ] **Task 2.4**: Integrate EventAdapter (2-3h)
  - Files: `app/app.go`, `app/dependencies.go`
  - Test: `go test ./app -v`

### Story 3: Integration & Validation

- [ ] **Task 3.1**: Run Full Test Suite (2-3h)
- [ ] **Task 3.2**: Run Performance Benchmarks (1-2h)
- [ ] **Task 3.3**: Manual Smoke Testing (1-2h)
- [ ] **Task 3.4**: Update Documentation (2-3h)

---

## Quick Commands

### Testing
```bash
# Unit tests (services)
go test ./app/services -v

# Integration tests
go test ./app -v

# Race detection
go test ./... -race -v

# Coverage
go test ./app -cover -coverprofile=coverage.out
go tool cover -html=coverage.out

# Benchmarks (run in background)
go test -bench=. ./app -benchmem &
```

### Quality Checks
```bash
# Complexity
gocyclo -over 10 app/app.go

# Field count
grep -A 50 "^type home struct" app/app.go | wc -l

# Framework imports in domain
cd domain && go list -f '{{.Imports}}' ./... | grep -E "bubbletea"
```

### Validation
```bash
# Manual smoke test
./stapler-squad

# Full system test
make restart-web
```

---

## Checkpoints

### Checkpoint 1: After Task 1.1
```bash
go test ./app/services -run TestSessionManagementService -v
go test ./app -run TestSessionCreation -v
./stapler-squad  # Test: Create and kill session
```

### Checkpoint 3: After Task 1.5
```bash
go test ./app -v -count=1
grep -A 50 "^type home struct" app/app.go | wc -l  # Expect <15
gocyclo -over 10 app/app.go  # Expect complexity <10
```

### Checkpoint 4: After Task 2.2
```bash
cd domain && go list -f '{{.Imports}}' ./... | grep -E "bubbletea"
# Expected: No output
go test ./app/session -v
```

### Checkpoint 6: Final
```bash
go test ./... -v -race -count=3
go test -bench=. ./app -benchmem
./stapler-squad  # Full smoke test
```

---

## Critical Bugs to Prevent

1. **Race Condition** (Task 1.1): Add mutex to SessionManagementService
2. **Index Out of Bounds** (Task 1.2): Validate bounds in NavigationService
3. **Stale Search Index** (Task 1.3): Rebuild index on session changes
4. **Init Deadlock** (Task 1.5): Document dependency order
5. **Event Exhaustion** (Task 2.3): Add default case with logging
6. **Timer Leak** (Task 1.2): Stop timers in cleanup

---

## Git Workflow

```bash
# Create feature branch
git checkout -b feature/architecture-phase1

# Work on task
# ... make changes ...

# Commit with conventional format
git commit -m "refactor(services): add SessionManagementService

- Define interface for session lifecycle
- Extract logic from home.Update()
- Add unit tests with 85% coverage

Related: #architecture-phase1"

# Push to feature branch
git push origin feature/architecture-phase1

# After all validation passes
git checkout main
git merge --squash feature/architecture-phase1
git commit -m "feat(architecture): complete Phase 1 refactoring"
```

---

## Success Criteria

- [ ] home struct has <10 fields
- [ ] home.Update() complexity <10
- [ ] Test coverage >80%
- [ ] Zero framework imports in domain/
- [ ] All 368 files compile
- [ ] Zero test regressions
- [ ] Performance benchmarks stable
- [ ] Manual smoke tests pass

---

## Files Reference

**Main Plan**: [architecture-refactoring-phase1.md](./architecture-refactoring-phase1.md)  
**Summary**: [PHASE1_SUMMARY.md](./PHASE1_SUMMARY.md)  
**This Guide**: [PHASE1_QUICK_REFERENCE.md](./PHASE1_QUICK_REFERENCE.md)

**Architecture Docs**:
- [ARCHITECTURE_REVIEW.md](../ARCHITECTURE_REVIEW.md)
- [ARCHITECTURE_IMPLEMENTATION_PLAN.md](../ARCHITECTURE_IMPLEMENTATION_PLAN.md)

---

## Context Loading (Per Task)

### Task 1.1 Context
```bash
cat app/app.go | grep -A 20 "type home struct"
cat app/app.go | grep -A 50 "case \"n\":"
cat app/session/controller.go | grep -A 30 "type Controller"
```

### Task 1.5 Context
```bash
ls app/services/
grep -A 10 "type.*Service interface" app/services/*.go
cat app/dependencies.go | grep -A 5 "Get.*Service"
```

### Task 2.3 Context
```bash
cat domain/events/session_events.go | grep "type.*struct"
cat app/session/controller.go | grep -A 10 "type SessionResult"
```

---

## Rollback Plans

### Task 1.5 Fails
```bash
git checkout feature/architecture-phase1
git revert HEAD~5..HEAD
git push origin feature/architecture-phase1 --force-with-lease
```

### Story 2 Breaks
```bash
git checkout feature/architecture-phase1
git checkout main -- app/session/controller.go
# Keep SessionResult alongside SessionOperation temporarily
```

---

## Metrics Dashboard

```bash
# Track throughout Phase 1
echo "home fields: $(grep -A 50 '^type home struct' app/app.go | wc -l)"
echo "Update() complexity: $(gocyclo app/app.go | grep 'Update' | awk '{print $1}')"
echo "Test coverage: $(go test ./app -cover | grep coverage | awk '{print $5}')"
echo "Framework imports: $(cd domain && go list -f '{{.Imports}}' ./... | grep -c bubbletea || echo 0)"
```

---

**Last Updated**: 2025-12-05  
**Plan Status**: Not started  
**Estimated Duration**: 2-3 weeks
