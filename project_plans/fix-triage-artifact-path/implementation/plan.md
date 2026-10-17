# Plan: fix-triage-artifact-path

## Summary

Two orthogonal fixes plus a test:

1. **Primary fix** — Guard `PlanArtifactsPath` persistence in `TriggerTriage` on `os.Stat` success.  
2. **Defense in depth** — Add a disk check inside `TransitionGuard`'s `Queued/Ready → InProgress` branch.  
3. **Improved error message** — Make `ApprovePlan`'s error distinguish "never written" from "deleted."  
4. **Tests** — Cover the newly-guarded code paths.

---

## Task Breakdown

### Task 1 — Guard PlanArtifactsPath persistence in TriggerTriage (backend, 1h)

**File:** `server/services/backlog_service_trigger_triage.go`

Change the update struct construction (lines ~659–666) so `PlanArtifactsPath` is only included when `os.Stat(pap)` succeeds:

```go
// Before (always writes pap):
update := session.BacklogItemUpdate{
    PlanArtifactsPath:   &pap,
    PlanApproved:        &approvalReset,
    ...
}
if item.AutoApprovePlan {
    if _, statErr := os.Stat(pap); statErr == nil { ... }
    else { log.Warn(...) }
}

// After:
update := session.BacklogItemUpdate{
    PlanApproved:        &approvalReset,
    PlanRejectionReason: &clearedReason,
    ClearPlanRejectedAt: true,
}
if _, statErr := os.Stat(pap); statErr == nil {
    update.PlanArtifactsPath = &pap
    if item.AutoApprovePlan {
        approved := true
        approvedAt := time.Now()
        update.PlanApproved = &approved
        update.PlanApprovedAt = &approvedAt
    }
} else {
    log.Warn("[TriggerTriage] plan_artifacts_path directory not created by triage", "path", pap, "item", itemID, "error", statErr)
    if item.AutoApprovePlan {
        log.Warn("[TriggerTriage] auto_approve_plan set but plan artifacts path missing", "path", pap, "item", itemID, "error", statErr)
    }
}
```

The single `os.Stat` replaces two. `PlanArtifactsPath` is `*string` — nil means "don't update" to the repo layer.

---

### Task 2 — Add disk existence check to TransitionGuard (backend, 1h)

**File:** `session/domain/backlog.go`

In the `Queued/Ready → InProgress` case, after the existing `PlanArtifactsPath == ""` check, add:

```go
if item.PlanApproved && !item.SkipPlanning && item.PlanArtifactsPath != "" {
    if _, statErr := os.Stat(item.PlanArtifactsPath); statErr != nil {
        return ErrPlanArtifactsMissing  // new sentinel
    }
}
```

Add a new sentinel error in `session/domain/backlog.go`:
```go
ErrPlanArtifactsMissing = errors.New("plan artifacts path recorded but directory does not exist on disk — re-run triage")
```

Wire it into `TransitionBacklogItemStatus`'s error-type switch (same file: `server/services/backlog_service_lifecycle.go` ~line 910) as `CodeFailedPrecondition`.

**Note:** `os.Stat` in `TransitionGuard` is a hot path side effect. The function is called frequently (dequeue sweep, GetPendingGates). Gate this with the non-empty-path condition so it's only reached for the rare stale-path case.

---

### Task 3 — Improve ApprovePlan error message (backend, 30m)

**File:** `server/services/backlog_service_lifecycle.go`

Replace the current error at line 1028:
```go
// Current:
fmt.Errorf("plan artifacts path %q does not exist on disk — re-run TriggerTriage", item.PlanArtifactsPath)

// Better (split into two checks):
if _, statErr := os.Stat(item.PlanArtifactsPath); statErr != nil {
    if os.IsNotExist(statErr) {
        return nil, connect.NewError(connect.CodeFailedPrecondition,
            fmt.Errorf("plan artifacts directory %q does not exist — triage ran but produced no plan output; re-run TriggerTriage", item.PlanArtifactsPath))
    }
    return nil, connect.NewError(connect.CodeFailedPrecondition,
        fmt.Errorf("cannot verify plan artifacts path %q: %w — re-run TriggerTriage", item.PlanArtifactsPath, statErr))
}
```

This is a minor text change that AC-7 requires.

---

### Task 4 — Test: TriggerTriage SDD mode with no implementation/ dir (test, 2h)

**File:** `server/services/backlog_service_triage_test.go`

Add test `TestTriggerTriage_should_NotPersistPlanArtifactsPath_When_SDDImplementationDirMissing`:
- Set up a fake SDD-mode item with a real git worktree
- Make the headless call succeed but NOT create the `implementation/` subdirectory
- Assert `item.PlanArtifactsPath == ""` after triage
- Assert `item.PlanApproved == false`

Add companion test `TestTriggerTriage_should_PersistPlanArtifactsPath_When_SDDImplementationDirExists`:
- Same setup but the fake triage output DOES create `implementation/plan.md`
- Assert `PlanArtifactsPath != ""`

---

### Task 5 — Test: TransitionGuard rejects stale PlanArtifactsPath (test, 1h)

**File:** `session/domain/backlog_test.go` or `session/gate_structural_test.go`

Add test `TestTransitionGuard_should_RejectQueuedToInProgress_When_PlanArtifactsPathIsStale`:
- Build `BacklogItemTransitionInput` with `PlanApproved=true`, `PlanArtifactsPath="/nonexistent/path"`
- Call `TransitionGuard(item, BacklogStatusInProgress)`
- Assert `errors.Is(err, ErrPlanArtifactsMissing)`

---

### Task 6 — Test: ApprovePlan returns informative error (test, 30m)

**File:** `server/services/backlog_service_lifecycle_test.go` (or similar)

Verify the updated error message text in the existing `TestApprovePlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition` test, or add a new focused assertion.

---

## Risk Assessment

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `os.Stat` in `TransitionGuard` impacts dequeue-sweep performance | Low (only if `PlanArtifactsPath != ""`) | Already gated; only reached for items in `Queued/Ready → InProgress` path with a plan |
| Nil `PlanArtifactsPath` in update clears a prior good value on re-triage failure | Medium | Explicitly: `nil` means "don't update" — the existing value is preserved. Only a successful `os.Stat` overwrites it. |
| Existing items in DB with stale paths still stuck | Medium | Task 2's `TransitionGuard` check surfaces them at dequeue time; operator can re-run triage |

## Out of Scope

- DB migration to clean existing stale `plan_artifacts_path` rows (useful but separate task)
- Changing where SDD triage writes its output (would break the design rationale in `pipeline_mode_seed.go`)
- Changing how the triage worktree is cleaned up at terminal transitions
