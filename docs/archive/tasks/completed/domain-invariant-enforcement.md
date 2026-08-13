# Domain Invariant Enforcement: Fix Anemic Domain Model

## Epic Overview

**User Value**: Session state transitions (Running→Paused, NeedsApproval→Running on approval, etc.) are currently unguarded — service layer code sets `instance.Status` directly without validating whether the transition is legal. This makes it possible for a bug to put a session into an impossible state (e.g., Paused→NeedsApproval with no prior approval request). Enforcing transitions in the domain layer prevents invalid states, makes state machine logic auditable in one place, and enables meaningful error messages when something tries an illegal transition.

**Success Metrics**:
- All `Status` mutations go through `Instance.TransitionTo()` or named domain methods
- Invalid transitions return typed errors that callers handle explicitly
- `SetStatus()` is package-private (unexported) or removed
- Tag invariants (no duplicates, max 50 chars) enforced at the domain boundary

**Scope**:
- Introduce `TransitionTo(Status) error` on `Instance`
- Add `Approve() error` and `Deny() error` domain methods
- Enforce tag invariants in `AddTag`/`SetTags`
- Update all 8 status mutation callsites to use domain methods
- **Excluded**: Status mutations from the detection/poller subsystem (these set `NeedsApproval` based on output patterns — handled in a follow-up)

**Constraints**:
- Public API of `Instance` must remain stable (proto adapter and service layer call patterns preserved)
- `FromInstanceData()` constructor bypass is acceptable (loading persisted state)
- Must not break existing Go tests in `testutil/`

---

## Caller Audit (All Status Mutation Sites)

| Location | Line | Code | Action |
|----------|------|------|--------|
| `session/instance.go` | 573 | `SetStatus(status)` | Keep as internal, make unexported |
| `session/instance.go` | 675 | `i.SetStatus(Running)` | Replace with `i.transitionTo(Running)` |
| `session/instance.go` | 1044 | `i.SetStatus(Paused)` | Replace with `i.transitionTo(Paused)` |
| `session/instance.go` | 1120 | `i.SetStatus(Running)` | Replace with `i.transitionTo(Running)` |
| `session/instance.go` | 1268 | `i.SetStatus(Running)` | Replace with `i.transitionTo(Running)` |
| `session/instance.go` | 349 | `instance.Status = Paused` | Bypass allowed (constructor from persisted data) |
| `session/instance.go` | 1430 | `i.Status = Paused` | Replace with `i.transitionTo(Paused)` |
| `server/review_queue_manager.go` | 216 | `inst.SetStatus(session.Running)` | Replace with `inst.Approve()` |

---

## Architecture Decision Records

### ADR-P2-3-01: Error Type Design for Invalid Transitions

**Context**: When `TransitionTo(newStatus)` is called with an invalid from→to combination, it must return an error. Callers need to distinguish "invalid transition" from "internal error".

**Decision**: Use a typed sentinel error with `from` and `to` fields:
```go
type ErrInvalidTransition struct {
    From Status
    To   Status
}
func (e ErrInvalidTransition) Error() string {
    return fmt.Sprintf("invalid transition: %s → %s", e.From, e.To)
}
```

**Rationale**: Typed errors allow callers to use `errors.As()` and provide specific error messages. Existing `ErrInvalidTitleLength` and `ErrCannotRestart` in `session/types.go` establish this pattern.

**Consequences**: Callers that receive `ErrInvalidTransition` should log and return `connect.NewError(connect.CodeFailedPrecondition, err)` to the frontend.

---

### ADR-P2-3-02: Internal vs Exported SetStatus

**Context**: `SetStatus` is currently exported, allowing any package to bypass state machine logic.

**Decision**: Rename to unexported `setStatus` (or `transitionTo`). Retain `SetStatus` as a thin exported wrapper only for the detection/poller subsystem during the transition period, with a deprecation comment.

**Rationale**: The detection subsystem sets `NeedsApproval` based on terminal output patterns — these don't fit the standard state machine and are addressed in a follow-up. Keeping `SetStatus` exported temporarily avoids breaking poller code in the same PR.

**Consequences**: Detection/poller callers are not blocked. Reviewed in follow-up as part of Instance decomposition epic.

---

### ADR-P2-3-03: Tag Invariant Enforcement Location

**Decision**: Enforce in `AddTag(tag string) error` and `SetTags(tags []string) error` on `Instance`. Return `ErrDuplicateTag` and `ErrTagTooLong` sentinel errors.

**Rationale**: Tags are a domain concept. The frontend already deduplicates in the UI, but the backend has no guard. Service layer code that calls `instance.AddTag()` will automatically inherit the validation.

---

## Story Breakdown

### Story 1: Define State Machine and TransitionTo [1 week]

**User Value**: The valid state machine is documented and enforced in code. Any future developer adding a new transition must explicitly update the allowed-transitions map — it cannot happen silently.

**Acceptance Criteria**:
- `session/state_machine.go` (new file) defines the full transition table
- `Instance.TransitionTo(Status) error` validates against the table
- `ErrInvalidTransition` type defined in `session/types.go`
- All internal `SetStatus` calls within `instance.go` replaced with `transitionTo`
- `SetStatus` exported signature retained but marked deprecated

#### Task 1.1: Define transition table and ErrInvalidTransition [2h]

**Objective**: Create `session/state_machine.go` with the complete allowed-transitions map and typed error.

**Context Boundary**:
- Primary: `session/state_machine.go` (new, ~60 lines)
- Supporting: `session/types.go` (add ErrInvalidTransition, ~20 lines), `session/instance.go` (lines 570-600 for Status type reference)
- ~80 lines total

**State Machine**:
```go
var allowedTransitions = map[Status][]Status{
    Creating:       {Running, Stopped},
    Running:        {Paused, NeedsApproval, Stopped},
    Paused:         {Running, Stopped},
    NeedsApproval:  {Running, Paused, Stopped},
    Loading:        {Running, Stopped},
    Stopped:        {},
}
```

**Implementation**:
1. Create `session/state_machine.go` with `allowedTransitions` map and `CanTransition(from, to Status) bool`
2. Add `ErrInvalidTransition{From, To Status}` to `session/types.go`
3. Write table-driven unit test in `session/state_machine_test.go` covering all valid + 5 invalid transitions

**Validation**:
- `go test ./session/ -run TestStateMachine` passes
- All valid transitions return `true`, invalid ones return `false`

---

#### Task 1.2: Add TransitionTo method to Instance [2h]

**Objective**: Wire `state_machine.go` into `Instance` by adding an unexported `transitionTo` method and replacing all internal `SetStatus` calls.

**Context Boundary**:
- Primary: `session/instance.go` (lines 573, 675, 987-1050, 1120, 1268, 1430)
- Supporting: `session/state_machine.go` (from Task 1.1)
- ~200 lines total

**Implementation**:
1. Add `func (i *Instance) transitionTo(s Status) error` that calls `CanTransition`, logs rejected transitions, returns `ErrInvalidTransition` if invalid, calls `setStatus` if valid
2. Replace 6 internal `i.SetStatus(...)` calls with `i.transitionTo(...)`
3. Handle errors: lifecycle methods (`Pause`, `Resume`, `Start`) propagate the error to their callers
4. `SetStatus` exported method retained, body calls `transitionTo`, annotated with deprecation comment

**Validation**:
- `go build ./...` passes
- `go test ./session/` passes
- Manual test: attempting to resume a Running session returns an error

---

### Story 2: Approval Domain Methods [1 week]

**User Value**: The approval flow (`Approve`/`Deny`) is expressed as domain concepts rather than scattered service-layer status mutations.

**Acceptance Criteria**:
- `Instance.Approve() error` transitions NeedsApproval → Running
- `Instance.Deny() error` transitions NeedsApproval → Paused
- `review_queue_manager.go` uses `inst.Approve()` instead of `inst.SetStatus(session.Running)`

#### Task 2.1: Add Approve and Deny domain methods [2h]

**Context Boundary**:
- Primary: `session/instance.go` (add ~30 lines)
- Supporting: `server/review_queue_manager.go` (line 216, update caller), `server/services/approval_handler.go` (approval flow)
- ~100 lines total

**Implementation**:
```go
func (i *Instance) Approve() error {
    if err := i.transitionTo(Running); err != nil {
        return fmt.Errorf("approve: %w", err)
    }
    // Clear review state timestamps
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    i.ReviewState.PendingApprovalSince = time.Time{}
    return nil
}

func (i *Instance) Deny() error {
    if err := i.transitionTo(Paused); err != nil {
        return fmt.Errorf("deny: %w", err)
    }
    return nil
}
```

**Validation**:
- `go test ./session/ -run TestApprove` passes (test in `session/instance_test.go`)
- `server/review_queue_manager.go` uses `inst.Approve()` — no direct `SetStatus` calls remain in review queue code

---

### Story 3: Tag Invariant Enforcement [1 week]

**User Value**: The backend rejects duplicate tags and overlong tag names with a clear error, matching the UI validation that currently only exists client-side.

**Acceptance Criteria**:
- `AddTag` returns `ErrDuplicateTag` if tag already present
- `AddTag` returns `ErrTagTooLong` if tag > 50 chars
- `SetTags` deduplicates and validates each tag
- Service layer propagates errors to frontend as `CodeInvalidArgument`

#### Task 3.1: Add tag sentinel errors and enforce in AddTag/SetTags [2h]

**Context Boundary**:
- Primary: `session/instance.go` (tag methods at lines ~1932-1993)
- Supporting: `session/types.go` (add ErrDuplicateTag, ErrTagTooLong)
- ~80 lines total

**Implementation**:
1. Add `ErrDuplicateTag` and `ErrTagTooLong` to `session/types.go`
2. Update `AddTag(tag string)` signature to `AddTag(tag string) error`; add duplicate check and length guard
3. Update `SetTags(tags []string)` to deduplicate and validate; return first validation error
4. Update callers of `AddTag` in service layer to handle the error

**Validation**:
- `go test ./session/ -run TestTagInvariants` passes
- Adding a duplicate tag returns `ErrDuplicateTag`
- Adding a 51-char tag returns `ErrTagTooLong`

---

### Story 4: Audit and Remove Remaining SetStatus Bypasses [1 week]

**User Value**: No code outside of `instance.go` can bypass the state machine. The domain boundary is complete.

**Acceptance Criteria**:
- `grep -r "SetStatus\|\.Status =" session/ server/` finds only constructor and deprecated wrapper
- All callers use domain methods
- `SetStatus` exported wrapper removed or marked for removal in next release

#### Task 4.1: Final audit and cleanup [2h]

**Context Boundary**:
- Primary: Full grep across `session/` and `server/` (~20 callsites to audit)
- Supporting: `session/instance.go`, `server/review_queue_manager.go`

**Implementation**:
1. Run `grep -rn "\.SetStatus\|\.Status = " session/ server/` to find remaining callsites
2. For each: replace with appropriate domain method or document why bypass is required
3. Remove `SetStatus` exported wrapper if no external callers remain
4. Final `go build ./... && go test ./...`

**Validation**:
- Zero external `SetStatus` calls remain outside `instance.go`
- All tests pass

---

## Known Issues

### 🐛 Detection/Poller Sets NeedsApproval Directly [SEVERITY: Medium]

**Description**: The detection subsystem (`session/detection/`) and review queue poller set `NeedsApproval` status based on terminal output patterns. These are legitimate status transitions but don't fit the lifecycle state machine (they originate from external observations, not API calls).

**Mitigation**: Retain `SetStatus` as exported during this epic. Address in the Instance decomposition epic by extracting a `DetectionInterface` that owns the `NeedsApproval` transition path.

**Files affected**: `session/detection/*.go`, `session/review_queue_poller.go`

---

### 🐛 Concurrent TransitionTo Calls [SEVERITY: Low]

**Description**: If two goroutines call `transitionTo` simultaneously, the `CanTransition` check and `setStatus` write are not atomic. The existing `stateMutex` must be held across both operations.

**Mitigation**: Acquire `stateMutex.Lock()` at the start of `transitionTo` before calling `CanTransition`, release after `setStatus`. This is the same pattern used by `Pause()` and `Resume()` today.

---

## Dependency Visualization

```
Task 1.1: State machine + ErrInvalidTransition
    ↓
Task 1.2: transitionTo on Instance (replaces internal SetStatus)
    ↓
    ├── Task 2.1: Approve/Deny domain methods  ─┐
    └── Task 3.1: Tag invariant enforcement    ─┤──► Task 4.1: Final audit
```

Stories 2 and 3 can proceed in parallel after Story 1 completes.

---

## Integration Checkpoints

**After Story 1**: `transitionTo` in place, all internal `SetStatus` calls migrated. `go test ./session/` passes. State machine is documented and tested.

**After Story 2**: Approval flow uses domain methods. `review_queue_manager.go` has no direct `SetStatus` calls.

**After Story 3**: Tag validation enforced backend. Frontend tag editor gets server-side validation backing.

**Final**: Zero external `SetStatus` calls. Domain invariants enforced at all entry points.

---

## Context Preparation Guide

**Task 1.1**: Read `session/types.go` (existing error patterns), `session/instance.go` lines 560-590 (Status type and SetStatus)

**Task 1.2**: Read `session/instance.go` lines 570-580 (setStatus), 660-690 (start), 980-1055 (Pause/Resume), 1260-1280 (Restart), 1425-1435 (UpdateDiffStats recovery)

**Task 2.1**: Read `server/review_queue_manager.go` lines 205-225, `server/services/approval_handler.go`, `session/instance.go` ReviewState fields

**Task 3.1**: Read `session/instance.go` lines 1930-1995 (AddTag, RemoveTag, HasTag, SetTags)

---

## Success Criteria

- [ ] `ErrInvalidTransition` type defined and used
- [ ] All internal status mutations go through `transitionTo`
- [ ] `Approve()` and `Deny()` domain methods on Instance
- [ ] Tag uniqueness and length enforced at domain layer
- [ ] Zero external `SetStatus` callers outside instance.go
- [ ] All existing Go tests pass
- [ ] New unit tests for state machine transitions
