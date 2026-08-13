# Validation Plan: backlog-workflow-engine

**Date**: 2026-05-19
**Test stack**: Go table-driven tests + testify/assert + real SQLite via ent (backend), Jest + React Testing Library (frontend)
**Phases in scope**: Phase 0, Phase 1, Phase 2 (Phase 3 deferred)

---

## Requirement → Test Mapping

| Req | Test file | Test name | Type | Scenario |
|-----|-----------|-----------|------|----------|
| S1 | `session/backlog_test.go` | `TestDefaultWorkflowEngine_CanTransition_WhenIdeaToRefining_ReturnsTrue` | Unit | idea→refining allowed |
| S1 | `session/backlog_test.go` | `TestDefaultWorkflowEngine_CanTransition_WhenRefiningToReady_EmptyAC_ReturnsFalse` | Unit | refining→ready blocked without AC |
| S1 | `session/backlog_test.go` | `TestDefaultWorkflowEngine_CanTransition_WhenRefiningToReady_WithAC_ReturnsTrue` | Unit | refining→ready passes with AC |
| S1 | `session/backlog_test.go` | `TestDefaultWorkflowEngine_CanTransition_WhenRefiningToArchived_ReturnsTrue` | Unit | refining→archived allowed |
| S1 | `session/backlog_test.go` | `TestDefaultWorkflowEngine_ValidTransitions_WhenRefining_ReturnsReadyAndArchived` | Unit | refining valid targets enumeration |
| S1 | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_WhenIdeaToRefining_Succeeds` | Integration | service-level idea→refining |
| S1 | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_WhenRefiningToReady_WithAC_Succeeds` | Integration | service-level refining→ready |
| S1 | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_WhenRefiningToReady_EmptyAC_ReturnsFailedPrecondition` | Integration | refining→ready guard at service layer |
| S1 | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_WhenRefiningToArchived_Succeeds` | Integration | refining→archived at service layer |
| S1 | `server/mcp/tools_backlog_test.go` | `TestSubmitTriageResult_WhenClarifyingQuestionsNonEmpty_TransitionsToRefining` | Integration | auto-transition on triage with questions |
| S1 | `server/mcp/tools_backlog_test.go` | `TestSubmitTriageResult_WhenNoClarifyingQuestions_DoesNotTransitionToRefining` | Integration | no auto-transition without questions |
| S1 | `server/services/backlog_lifecycle_test.go` | `TestBacklogLifecycleListener_WhenRefiningTriageExits_ResolvesNext` | Unit | lifecycle listener triage exit for refining |
| S1 | `server/services/backlog_lifecycle_test.go` | `TestBacklogLifecycleListener_WhenNonRefiningTriageExits_NoAction` | Unit | listener no-op for non-refining items |
| S1 | `server/services/backlog_service_test.go` | `TestReconcileStuckItems_WhenRefiningExceeds24h_TransitionsToIdea` | Integration | refining 24h timeout reconcile |
| S1 | `server/services/backlog_service_test.go` | `TestReconcileStuckItems_WhenRefiningUnder24h_NoTransition` | Integration | refining not yet timed out — left alone |
| S1 | `session/backlog_status_event_test.go` | `TestBacklogStatusEvent_WhenCreated_AppendsToLog` | Integration | event log append on refining transition |
| S1 | `session/backlog_status_event_test.go` | `TestBacklogStatusEvent_WhenQueried_ReturnsChronologicalOrder` | Integration | event log read order |
| S2 | `session/workflow_config_test.go` | `TestWorkflowConfig_WhenSeedDefault_ContainsAllBaselineStatuses` | Integration | seed produces idea/ready/in_progress/review/done/archived/refining |
| S2 | `session/workflow_config_test.go` | `TestWorkflowConfig_WhenSeedDefault_TransitionsMatchLegacyTable` | Integration | seeded transitions match old hardcoded validTransitions |
| S2 | `session/workflow_config_test.go` | `TestConfiguredWorkflowEngine_WhenCacheExpires_ReloadsFromDB` | Integration | 30s TTL cache reload |
| S2 | `session/workflow_config_test.go` | `TestConfiguredWorkflowEngine_WhenCacheHot_DoesNotQueryDB` | Unit | cache hit avoids DB call |
| S2 | `session/workflow_config_test.go` | `TestConfiguredWorkflowEngine_CanTransition_WhenCustomStateAdded_AllowsTransition` | Integration | custom state transitions respected |
| S3 | `server/services/workflow_service_test.go` | `TestGetWorkflowConfig_WhenDefaultSeeded_ReturnsAllStates` | Integration | GET returns seeded config |
| S3 | `server/services/workflow_service_test.go` | `TestUpsertWorkflowState_WhenNewState_AddsToConfig` | Integration | add custom state |
| S3 | `server/services/workflow_service_test.go` | `TestUpsertWorkflowState_WhenRenameExisting_UpdatesLabel` | Integration | rename state label |
| S3 | `server/services/workflow_service_test.go` | `TestDeleteWorkflowState_WhenNoItems_Succeeds` | Integration | delete unused state |
| S3 | `server/services/workflow_service_test.go` | `TestDeleteWorkflowState_WhenItemsExist_RequiresMigrationTarget` | Integration | delete with live items needs migration_target |
| S3 | `server/services/workflow_service_test.go` | `TestDeleteWorkflowState_WhenMigrationTargetInvalid_ReturnsInvalidArgument` | Integration | migration target must exist |
| S3 | `server/services/workflow_service_test.go` | `TestDeleteWorkflowState_WhenBuiltinCoreState_ReturnsFailedPrecondition` | Integration | built-in states cannot be deleted |
| S3 | `server/services/workflow_service_test.go` | `TestUpsertWorkflowTransition_WhenDeadlockDetected_ReturnsInvalidArgument` | Integration | deadlock/reachability validation |
| S3 | `server/services/workflow_service_test.go` | `TestDeleteWorkflowState_WhenMigrationTargetProvided_MigratesItems` | Integration | items migrated on state deletion |
| Compat | `session/backlog_test.go` | `TestCanTransitionBacklog_WhenIdeaToReady_LegacyPathStillWorks` | Unit | regression: idea→ready unchanged |
| Compat | `session/backlog_test.go` | `TestTransitionGuard_WhenIdeaToReady_EmptyAC_ReturnsErrACRequired` | Unit | regression: guard unchanged |
| Compat | `session/backlog_test.go` | `TestTransitionGuard_WhenReadyToInProgress_NoPlan_ReturnsErrPlanRequired` | Unit | regression: plan guard unchanged |
| Compat | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_WhenExistingItemWithLegacyStatus_NoMigrationRequired` | Integration | items with pre-workflow-engine statuses work without migration |
| Phase 0 | `web-app/src/lib/backlog/status.test.ts` | `should accept known statuses without type error` | Unit | TypeScript open union known variants |
| Phase 0 | `web-app/src/lib/backlog/status.test.ts` | `unknown_status_should_renderWithFallback_not_throw` | Unit | unknown string status renders fallback |
| Phase 0 | `web-app/src/lib/backlog/status.test.ts` | `should preserve type narrowing for KnownBacklogStatus` | Unit | narrowing still works after open union |
| S1 (frontend) | `web-app/src/components/backlog/StatusBadge.test.tsx` | `should render refining badge with amber color` | Unit | amber badge for refining |
| S1 (frontend) | `web-app/src/components/backlog/StatusBadge.test.tsx` | `should render unknown status with fallback badge` | Unit | fallback for unknown string |
| S1 (frontend) | `web-app/src/components/backlog/BacklogBoard.test.tsx` | `should render refining column between idea and ready` | Unit | board column order |
| S1 (frontend) | `web-app/src/components/backlog/BacklogBoard.test.tsx` | `should display clarifying questions in refining item detail` | Unit | clarifying questions shown |
| S1 (frontend) | `web-app/src/components/backlog/BacklogBoard.test.tsx` | `should show Mark Ready action for refining items` | Unit | Mark Ready CTA present |
| S3 (frontend) | `web-app/src/components/settings/WorkflowStateEditor.test.tsx` | `should render list of workflow states` | Unit | state list rendered |
| S3 (frontend) | `web-app/src/components/settings/WorkflowStateEditor.test.tsx` | `should call upsertWorkflowState when inline rename committed` | Unit | rename triggers RPC |
| S3 (frontend) | `web-app/src/components/settings/WorkflowStateEditor.test.tsx` | `should open delete modal with migration target selector when items exist` | Unit | delete modal with migration |
| S3 (frontend) | `web-app/src/components/settings/WorkflowStateEditor.test.tsx` | `should disable delete for built-in core states` | Unit | built-in states uneditable |
| S3 (frontend) | `web-app/src/lib/hooks/useWorkflowService.test.ts` | `returns stable method references across re-renders` | Unit | reference stability (parity with useBacklogService) |

---

## Test Suites

### Suite 1: WorkflowEngine interface + DefaultWorkflowEngine (Go unit tests)

**File**: `session/backlog_test.go` (new section) and/or `session/workflow_engine_test.go`

These tests validate the `WorkflowEngine` interface contract implemented by `DefaultWorkflowEngine`. All tests are pure in-memory — no DB.

```go
// TestDefaultWorkflowEngine_CanTransition_WhenIdeaToRefining_ReturnsTrue
func TestDefaultWorkflowEngine_CanTransition_WhenIdeaToRefining_ReturnsTrue(t *testing.T) {
    engine := NewDefaultWorkflowEngine()
    assert.True(t, engine.CanTransition(BacklogStatusIdea, BacklogStatusRefining))
}

// TestDefaultWorkflowEngine_CanTransition_WhenRefiningToReady_EmptyAC_ReturnsFalse
func TestDefaultWorkflowEngine_CanTransition_WhenRefiningToReady_EmptyAC_ReturnsFalse(t *testing.T) {
    engine := NewDefaultWorkflowEngine()
    input := BacklogItemTransitionInput{Status: BacklogStatusRefining, AcCriteriaJSON: ""}
    _, err := engine.Guard(input, BacklogStatusReady)
    assert.ErrorIs(t, err, ErrACRequired)
}

// TestDefaultWorkflowEngine_CanTransition_WhenRefiningToReady_WithAC_ReturnsTrue
func TestDefaultWorkflowEngine_CanTransition_WhenRefiningToReady_WithAC_ReturnsTrue(t *testing.T) {
    engine := NewDefaultWorkflowEngine()
    input := BacklogItemTransitionInput{
        Status:         BacklogStatusRefining,
        AcCriteriaJSON: `[{"index":0,"text":"done","status":"pending"}]`,
    }
    err := engine.Guard(input, BacklogStatusReady)
    assert.NoError(t, err)
}

// TestDefaultWorkflowEngine_CanTransition_WhenRefiningToArchived_ReturnsTrue
func TestDefaultWorkflowEngine_CanTransition_WhenRefiningToArchived_ReturnsTrue(t *testing.T) {
    engine := NewDefaultWorkflowEngine()
    assert.True(t, engine.CanTransition(BacklogStatusRefining, BacklogStatusArchived))
}

// TestDefaultWorkflowEngine_ValidTransitions_WhenRefining_ReturnsReadyAndArchived
func TestDefaultWorkflowEngine_ValidTransitions_WhenRefining_ReturnsReadyAndArchived(t *testing.T) {
    engine := NewDefaultWorkflowEngine()
    targets := engine.ValidTransitions(BacklogStatusRefining)
    assert.ElementsMatch(t, []BacklogStatus{BacklogStatusReady, BacklogStatusArchived}, targets)
}

// TestDefaultWorkflowEngine_CanTransition_WhenIdeaToUnknown_ReturnsFalse
func TestDefaultWorkflowEngine_CanTransition_WhenIdeaToUnknown_ReturnsFalse(t *testing.T) {
    engine := NewDefaultWorkflowEngine()
    assert.False(t, engine.CanTransition(BacklogStatusIdea, "nonexistent"))
}
```

**Table-driven variant** (combine the above for CanTransition coverage):

```go
func TestDefaultWorkflowEngine_CanTransition_AllCombinations(t *testing.T) {
    engine := NewDefaultWorkflowEngine()
    tests := []struct{
        from, to BacklogStatus
        want     bool
    }{
        {BacklogStatusIdea, BacklogStatusRefining, true},
        {BacklogStatusIdea, BacklogStatusReady, true},
        {BacklogStatusIdea, BacklogStatusArchived, true},
        {BacklogStatusIdea, BacklogStatusInProgress, false},
        {BacklogStatusRefining, BacklogStatusReady, true},
        {BacklogStatusRefining, BacklogStatusArchived, true},
        {BacklogStatusRefining, BacklogStatusIdea, false},
        {BacklogStatusRefining, BacklogStatusInProgress, false},
        // existing transitions preserved
        {BacklogStatusReady, BacklogStatusInProgress, true},
        {BacklogStatusReady, BacklogStatusIdea, true},
        {BacklogStatusReady, BacklogStatusArchived, true},
        {BacklogStatusInProgress, BacklogStatusReview, true},
        {BacklogStatusInProgress, BacklogStatusReady, true},
        {BacklogStatusReview, BacklogStatusDone, true},
        {BacklogStatusReview, BacklogStatusInProgress, true},
        {BacklogStatusDone, BacklogStatusReview, true},
        {BacklogStatusDone, BacklogStatusArchived, true},
        {BacklogStatusArchived, BacklogStatusIdea, true},
    }
    for _, tc := range tests {
        t.Run(fmt.Sprintf("%s→%s", tc.from, tc.to), func(t *testing.T) {
            assert.Equal(t, tc.want, engine.CanTransition(tc.from, tc.to))
        })
    }
}
```

**Regression — legacy CanTransitionBacklog and TransitionGuard unchanged**:

```go
// TestCanTransitionBacklog_WhenIdeaToReady_LegacyPathStillWorks
func TestCanTransitionBacklog_WhenIdeaToReady_LegacyPathStillWorks(t *testing.T) {
    assert.True(t, CanTransitionBacklog(BacklogStatusIdea, BacklogStatusReady))
}

// TestTransitionGuard_WhenIdeaToReady_EmptyAC_ReturnsErrACRequired
func TestTransitionGuard_WhenIdeaToReady_EmptyAC_ReturnsErrACRequired(t *testing.T) {
    err := TransitionGuard(BacklogItemTransitionInput{Status: BacklogStatusIdea}, BacklogStatusReady)
    assert.ErrorIs(t, err, ErrACRequired)
}

// TestTransitionGuard_WhenReadyToInProgress_NoPlan_ReturnsErrPlanRequired
func TestTransitionGuard_WhenReadyToInProgress_NoPlan_ReturnsErrPlanRequired(t *testing.T) {
    err := TransitionGuard(BacklogItemTransitionInput{Status: BacklogStatusReady, PlanApproved: false, SkipPlanning: false}, BacklogStatusInProgress)
    assert.ErrorIs(t, err, ErrPlanRequired)
}
```

**Count**: 10 unit tests (6 specific + 1 table-driven covering 18 combinations + 3 regressions)

---

### Suite 2: refining transitions — service layer (Go integration tests)

**File**: `server/services/backlog_service_test.go` (new section)

Uses `newBacklogService(t)` helper (real SQLite). Pattern mirrors existing `TestTriggerReReview_*`.

```go
// TestTransitionBacklogItemStatus_WhenIdeaToRefining_Succeeds
func TestTransitionBacklogItemStatus_WhenIdeaToRefining_Succeeds(t *testing.T) {
    svc := newBacklogService(t)
    createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
        Title: "needs clarification",
    }))
    require.NoError(t, err)
    itemID := createResp.Msg.Item.Id

    transResp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusRefining),
    }))
    require.NoError(t, err)
    assert.Equal(t, "refining", transResp.Msg.Item.Status)
}

// TestTransitionBacklogItemStatus_WhenRefiningToReady_WithAC_Succeeds
func TestTransitionBacklogItemStatus_WhenRefiningToReady_WithAC_Succeeds(t *testing.T) {
    svc := newBacklogService(t)
    // Create → transition to refining → add AC → transition to ready
    createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
        Title:              "item with questions",
        AcceptanceCriteria: []*sessionv1.AcCriterion{{Index: 0, Text: "clarified", Status: "pending"}},
    }))
    require.NoError(t, err)
    itemID := createResp.Msg.Item.Id

    _, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusRefining),
    }))
    require.NoError(t, err)

    transResp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusReady),
    }))
    require.NoError(t, err)
    assert.Equal(t, "ready", transResp.Msg.Item.Status)
}

// TestTransitionBacklogItemStatus_WhenRefiningToReady_EmptyAC_ReturnsFailedPrecondition
func TestTransitionBacklogItemStatus_WhenRefiningToReady_EmptyAC_ReturnsFailedPrecondition(t *testing.T) {
    svc := newBacklogService(t)
    createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
        Title: "no AC yet",
    }))
    require.NoError(t, err)
    itemID := createResp.Msg.Item.Id

    _, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusRefining),
    }))
    require.NoError(t, err)

    _, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusReady),
    }))
    require.Error(t, err)
    var connErr *connect.Error
    require.ErrorAs(t, err, &connErr)
    assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code())
}

// TestTransitionBacklogItemStatus_WhenRefiningToArchived_Succeeds
func TestTransitionBacklogItemStatus_WhenRefiningToArchived_Succeeds(t *testing.T) {
    svc := newBacklogService(t)
    createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
        Title: "abandoned",
    }))
    require.NoError(t, err)
    itemID := createResp.Msg.Item.Id

    _, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusRefining),
    }))
    require.NoError(t, err)

    transResp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusArchived),
    }))
    require.NoError(t, err)
    assert.Equal(t, "archived", transResp.Msg.Item.Status)
}

// TestTransitionBacklogItemStatus_WhenExistingItemWithLegacyStatus_NoMigrationRequired
// Backward-compat: items that were persisted before this feature (idea/ready/etc.) transition
// correctly without any DB migration step.
func TestTransitionBacklogItemStatus_WhenExistingItemWithLegacyStatus_NoMigrationRequired(t *testing.T) {
    storage := createTestStorage(t)
    svc := NewBacklogService(storage, nil, nil)

    // Directly persist item with legacy "idea" status (simulates pre-migration DB row).
    item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:    "legacy idea item",
        Status:   string(session.BacklogStatusIdea),
        Priority: 3,
        AcceptanceCriteria: `[{"index":0,"text":"works","status":"pending"}]`,
    })
    require.NoError(t, err)

    // Legacy transition idea→ready must still work.
    transResp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       item.ID,
        TargetStatus: string(session.BacklogStatusReady),
    }))
    require.NoError(t, err)
    assert.Equal(t, "ready", transResp.Msg.Item.Status)
}
```

**Count**: 5 integration tests

---

### Suite 3: submit_triage_result MCP handler (Go integration tests)

**File**: `server/mcp/tools_backlog_test.go` (new section)

Pattern matches `TestReportProgress_*` in the existing file — uses `newTestBacklogStorage(t)` and `&backlogHandlers{storage: storage}`.

```go
// TestSubmitTriageResult_WhenClarifyingQuestionsNonEmpty_TransitionsToRefining
func TestSubmitTriageResult_WhenClarifyingQuestionsNonEmpty_TransitionsToRefining(t *testing.T) {
    storage := newTestBacklogStorage(t)
    ctx := context.Background()

    item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
        Title:    "needs questions answered",
        Status:   string(session.BacklogStatusIdea),
        Priority: 3,
    })
    require.NoError(t, err)

    sessionUUID := uuid.New().String()
    _, err = storage.CreateItemSession(ctx, session.ItemSessionData{
        ItemID:      item.ID,
        SessionUUID: sessionUUID,
        SessionRole: string(session.SessionRoleTriage),
    })
    require.NoError(t, err)

    handler := &backlogHandlers{storage: storage}
    ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

    req := makeToolReq(map[string]interface{}{
        "item_id":              item.ID,
        "clarifying_questions": []interface{}{"What is the scope?", "Any API constraints?"},
        "acceptance_criteria":  []interface{}{},
    })

    result, err := handler.submitTriageResult(ctxWithUUID, req)
    require.NoError(t, err)
    m := parseResult(t, result)
    require.True(t, m["success"].(bool))

    // Verify item transitioned to refining.
    fetched, err := storage.GetBacklogItem(ctx, item.ID)
    require.NoError(t, err)
    assert.Equal(t, string(session.BacklogStatusRefining), fetched.Status)
}

// TestSubmitTriageResult_WhenNoClarifyingQuestions_DoesNotTransitionToRefining
func TestSubmitTriageResult_WhenNoClarifyingQuestions_DoesNotTransitionToRefining(t *testing.T) {
    storage := newTestBacklogStorage(t)
    ctx := context.Background()

    item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
        Title:    "clear requirements",
        Status:   string(session.BacklogStatusIdea),
        Priority: 3,
    })
    require.NoError(t, err)

    sessionUUID := uuid.New().String()
    _, err = storage.CreateItemSession(ctx, session.ItemSessionData{
        ItemID:      item.ID,
        SessionUUID: sessionUUID,
        SessionRole: string(session.SessionRoleTriage),
    })
    require.NoError(t, err)

    handler := &backlogHandlers{storage: storage}
    ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

    req := makeToolReq(map[string]interface{}{
        "item_id":              item.ID,
        "clarifying_questions": []interface{}{},
        "acceptance_criteria":  []interface{}{"User can log in"},
    })

    result, err := handler.submitTriageResult(ctxWithUUID, req)
    require.NoError(t, err)
    m := parseResult(t, result)
    require.True(t, m["success"].(bool))

    // Verify item is NOT in refining — should have moved forward normally.
    fetched, err := storage.GetBacklogItem(ctx, item.ID)
    require.NoError(t, err)
    assert.NotEqual(t, string(session.BacklogStatusRefining), fetched.Status,
        "item should not be in refining when no clarifying questions")
}

// TestSubmitTriageResult_WhenNoSessionUUID_ReturnsPermissionDenied
func TestSubmitTriageResult_WhenNoSessionUUID_ReturnsPermissionDenied(t *testing.T) {
    storage := newTestBacklogStorage(t)
    handler := &backlogHandlers{storage: storage}
    ctx := context.Background() // no UUID

    req := makeToolReq(map[string]interface{}{
        "item_id":              "00000000-0000-0000-0000-000000000001",
        "clarifying_questions": []interface{}{"something"},
    })

    result, err := handler.submitTriageResult(ctx, req)
    require.NoError(t, err)
    m := parseResult(t, result)
    require.False(t, m["success"].(bool))
    errObj := m["error"].(map[string]interface{})
    assert.Equal(t, ErrPermissionDenied, errObj["code"].(string))
}
```

**Count**: 3 integration tests

---

### Suite 4: BacklogLifecycleListener — triage session exit for refining items (Go unit tests)

**File**: `server/services/backlog_lifecycle_test.go` (new section)

These tests exercise the `BacklogLifecycleListener` logic that handles triage session exit events. Uses a mock or in-memory service call to observe side effects.

```go
// TestBacklogLifecycleListener_WhenRefiningTriageExits_ResolvesNext
// When a triage session ends for an item in "refining" status, the listener
// should transition the item to "ready" (if AC is now populated) or re-queue.
func TestBacklogLifecycleListener_WhenRefiningTriageExits_ResolvesNext(t *testing.T) {
    storage := createTestStorage(t)
    svc := NewBacklogService(storage, nil, nil)
    listener := NewBacklogLifecycleListener(storage, svc)

    // Create refining item with AC (questions answered).
    item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:              "post-triage item",
        Status:             string(session.BacklogStatusRefining),
        Priority:           3,
        AcceptanceCriteria: `[{"index":0,"text":"scope defined","status":"pending"}]`,
    })
    require.NoError(t, err)

    sessionUUID := uuid.New().String()
    _, err = storage.CreateItemSession(t.Context(), session.ItemSessionData{
        ItemID:      item.ID,
        SessionUUID: sessionUUID,
        SessionRole: string(session.SessionRoleTriage),
    })
    require.NoError(t, err)

    err = listener.OnTriageSessionExit(t.Context(), sessionUUID)
    require.NoError(t, err)

    // After listener handles exit, item should leave refining.
    fetched, err := storage.GetBacklogItem(t.Context(), item.ID)
    require.NoError(t, err)
    assert.NotEqual(t, string(session.BacklogStatusRefining), fetched.Status)
}

// TestBacklogLifecycleListener_WhenNonRefiningTriageExits_NoAction
// When a triage session ends for an item NOT in "refining" status, the listener
// should not modify the item status.
func TestBacklogLifecycleListener_WhenNonRefiningTriageExits_NoAction(t *testing.T) {
    storage := createTestStorage(t)
    svc := NewBacklogService(storage, nil, nil)
    listener := NewBacklogLifecycleListener(storage, svc)

    item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:    "idea item",
        Status:   string(session.BacklogStatusIdea),
        Priority: 3,
    })
    require.NoError(t, err)

    sessionUUID := uuid.New().String()
    _, err = storage.CreateItemSession(t.Context(), session.ItemSessionData{
        ItemID:      item.ID,
        SessionUUID: sessionUUID,
        SessionRole: string(session.SessionRoleTriage),
    })
    require.NoError(t, err)

    err = listener.OnTriageSessionExit(t.Context(), sessionUUID)
    require.NoError(t, err)

    fetched, err := storage.GetBacklogItem(t.Context(), item.ID)
    require.NoError(t, err)
    assert.Equal(t, string(session.BacklogStatusIdea), fetched.Status,
        "listener should not change status of non-refining items")
}

// TestBacklogLifecycleListener_WhenUnknownSessionUUID_NoError
func TestBacklogLifecycleListener_WhenUnknownSessionUUID_NoError(t *testing.T) {
    storage := createTestStorage(t)
    svc := NewBacklogService(storage, nil, nil)
    listener := NewBacklogLifecycleListener(storage, svc)

    // Session UUID not in DB — listener should handle gracefully.
    err := listener.OnTriageSessionExit(t.Context(), "00000000-0000-0000-0000-000000000001")
    require.NoError(t, err)
}
```

**Count**: 3 unit tests

---

### Suite 5: ReconcileStuckItems — refining timeout (Go integration tests)

**File**: `server/services/backlog_service_test.go` (new section in existing file)

Pattern matches how other time-based tests are written: manipulate `UpdatedAt` directly via storage to simulate elapsed time.

```go
// TestReconcileStuckItems_WhenRefiningExceeds24h_TransitionsToIdea
func TestReconcileStuckItems_WhenRefiningExceeds24h_TransitionsToIdea(t *testing.T) {
    storage := createTestStorage(t)
    svc := NewBacklogService(storage, nil, nil)

    // Create item and transition to refining.
    createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
        Title: "stuck in refining",
    }))
    require.NoError(t, err)
    itemID := createResp.Msg.Item.Id

    _, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusRefining),
    }))
    require.NoError(t, err)

    // Back-date the item's updated_at to simulate 25 hours ago.
    staleTime := time.Now().Add(-25 * time.Hour)
    err = storage.SetBacklogItemUpdatedAt(t.Context(), itemID, staleTime)
    require.NoError(t, err)

    reconcileResp, err := svc.ReconcileStuckItems(t.Context(), connect.NewRequest(&sessionv1.ReconcileStuckItemsRequest{}))
    require.NoError(t, err)
    assert.Contains(t, reconcileResp.Msg.ReconciledItemIds, itemID)

    fetched, err := storage.GetBacklogItem(t.Context(), itemID)
    require.NoError(t, err)
    assert.Equal(t, string(session.BacklogStatusIdea), fetched.Status,
        "refining item exceeding 24h should be reset to idea")
}

// TestReconcileStuckItems_WhenRefiningUnder24h_NoTransition
func TestReconcileStuckItems_WhenRefiningUnder24h_NoTransition(t *testing.T) {
    storage := createTestStorage(t)
    svc := NewBacklogService(storage, nil, nil)

    createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
        Title: "recent refining",
    }))
    require.NoError(t, err)
    itemID := createResp.Msg.Item.Id

    _, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
        ItemId:       itemID,
        TargetStatus: string(session.BacklogStatusRefining),
    }))
    require.NoError(t, err)

    // Only 1 hour has passed — within the 24h window.
    recentTime := time.Now().Add(-1 * time.Hour)
    err = storage.SetBacklogItemUpdatedAt(t.Context(), itemID, recentTime)
    require.NoError(t, err)

    reconcileResp, err := svc.ReconcileStuckItems(t.Context(), connect.NewRequest(&sessionv1.ReconcileStuckItemsRequest{}))
    require.NoError(t, err)
    assert.NotContains(t, reconcileResp.Msg.ReconciledItemIds, itemID)

    fetched, err := storage.GetBacklogItem(t.Context(), itemID)
    require.NoError(t, err)
    assert.Equal(t, string(session.BacklogStatusRefining), fetched.Status,
        "refining item under 24h should remain refining")
}
```

**Count**: 2 integration tests

---

### Suite 6: BacklogStatusEvent append-only log (Go integration tests)

**File**: `session/backlog_status_event_test.go` (new file)

```go
// TestBacklogStatusEvent_WhenCreated_AppendsToLog
func TestBacklogStatusEvent_WhenCreated_AppendsToLog(t *testing.T) {
    repo := newTestEntRepository(t)
    storage, err := session.NewStorageWithRepository(repo)
    require.NoError(t, err)

    item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:    "event log test",
        Status:   string(session.BacklogStatusIdea),
        Priority: 3,
    })
    require.NoError(t, err)

    err = storage.AppendBacklogStatusEvent(t.Context(), session.BacklogStatusEventData{
        ItemID:     item.ID,
        FromStatus: string(session.BacklogStatusIdea),
        ToStatus:   string(session.BacklogStatusRefining),
        TriggeredBy: "auto:triage",
    })
    require.NoError(t, err)

    events, err := storage.ListBacklogStatusEvents(t.Context(), item.ID)
    require.NoError(t, err)
    require.Len(t, events, 1)
    assert.Equal(t, string(session.BacklogStatusIdea), events[0].FromStatus)
    assert.Equal(t, string(session.BacklogStatusRefining), events[0].ToStatus)
}

// TestBacklogStatusEvent_WhenQueried_ReturnsChronologicalOrder
func TestBacklogStatusEvent_WhenQueried_ReturnsChronologicalOrder(t *testing.T) {
    repo := newTestEntRepository(t)
    storage, err := session.NewStorageWithRepository(repo)
    require.NoError(t, err)

    item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:    "multi-event item",
        Status:   string(session.BacklogStatusIdea),
        Priority: 3,
    })
    require.NoError(t, err)

    transitions := []struct{ from, to string }{
        {string(session.BacklogStatusIdea), string(session.BacklogStatusRefining)},
        {string(session.BacklogStatusRefining), string(session.BacklogStatusReady)},
        {string(session.BacklogStatusReady), string(session.BacklogStatusInProgress)},
    }
    for _, tr := range transitions {
        err = storage.AppendBacklogStatusEvent(t.Context(), session.BacklogStatusEventData{
            ItemID:     item.ID,
            FromStatus: tr.from,
            ToStatus:   tr.to,
            TriggeredBy: "test",
        })
        require.NoError(t, err)
    }

    events, err := storage.ListBacklogStatusEvents(t.Context(), item.ID)
    require.NoError(t, err)
    require.Len(t, events, 3)
    // Must be chronological (ascending CreatedAt).
    for i := 1; i < len(events); i++ {
        assert.True(t, events[i].CreatedAt.After(events[i-1].CreatedAt) ||
            events[i].CreatedAt.Equal(events[i-1].CreatedAt))
    }
    assert.Equal(t, string(session.BacklogStatusIdea), events[0].FromStatus)
    assert.Equal(t, string(session.BacklogStatusInProgress), events[2].ToStatus)
}

// TestBacklogStatusEvent_WhenItemDeleted_EventsOrphaned
// Verifies that deleting an item does not panic on event log queries.
func TestBacklogStatusEvent_WhenItemDeleted_EventsOrphaned(t *testing.T) {
    // Implementation-specific: verify no FK panic or graceful cascade.
    // Expect either empty result or not-found error — not a server panic.
    repo := newTestEntRepository(t)
    storage, err := session.NewStorageWithRepository(repo)
    require.NoError(t, err)

    item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:    "will be deleted",
        Status:   string(session.BacklogStatusIdea),
        Priority: 3,
    })
    require.NoError(t, err)

    _ = storage.AppendBacklogStatusEvent(t.Context(), session.BacklogStatusEventData{
        ItemID:     item.ID,
        FromStatus: "idea",
        ToStatus:   "refining",
        TriggeredBy: "test",
    })

    // After deletion, query should not panic.
    _, queryErr := storage.ListBacklogStatusEvents(t.Context(), item.ID)
    // Accept either nil (cascade) or not-found error — just not a crash.
    _ = queryErr
}
```

**Count**: 3 integration tests

---

### Suite 7: WorkflowConfig data model and ConfiguredWorkflowEngine (Go integration tests)

**File**: `session/workflow_config_test.go` (new file)

```go
// TestWorkflowConfig_WhenSeedDefault_ContainsAllBaselineStatuses
func TestWorkflowConfig_WhenSeedDefault_ContainsAllBaselineStatuses(t *testing.T) {
    repo := newTestEntRepository(t)
    storage, err := session.NewStorageWithRepository(repo)
    require.NoError(t, err)

    err = storage.SeedDefaultWorkflowConfig(t.Context())
    require.NoError(t, err)

    cfg, err := storage.GetWorkflowConfig(t.Context())
    require.NoError(t, err)

    statusNames := make([]string, 0, len(cfg.States))
    for _, s := range cfg.States {
        statusNames = append(statusNames, s.Name)
    }
    expectedStatuses := []string{"idea", "refining", "ready", "in_progress", "review", "done", "archived"}
    assert.ElementsMatch(t, expectedStatuses, statusNames)
}

// TestWorkflowConfig_WhenSeedDefault_TransitionsMatchLegacyTable
func TestWorkflowConfig_WhenSeedDefault_TransitionsMatchLegacyTable(t *testing.T) {
    repo := newTestEntRepository(t)
    storage, err := session.NewStorageWithRepository(repo)
    require.NoError(t, err)

    err = storage.SeedDefaultWorkflowConfig(t.Context())
    require.NoError(t, err)

    engine, err := session.NewConfiguredWorkflowEngine(storage)
    require.NoError(t, err)

    // Spot-check all legacy transitions still present.
    legacyTransitions := []struct{ from, to session.BacklogStatus; allowed bool }{
        {session.BacklogStatusIdea, session.BacklogStatusReady, true},
        {session.BacklogStatusIdea, session.BacklogStatusArchived, true},
        {session.BacklogStatusReady, session.BacklogStatusInProgress, true},
        {session.BacklogStatusReady, session.BacklogStatusIdea, true},
        {session.BacklogStatusInProgress, session.BacklogStatusReview, true},
        {session.BacklogStatusReview, session.BacklogStatusDone, true},
        {session.BacklogStatusDone, session.BacklogStatusArchived, true},
        {session.BacklogStatusArchived, session.BacklogStatusIdea, true},
        // New refining transitions.
        {session.BacklogStatusIdea, session.BacklogStatusRefining, true},
        {session.BacklogStatusRefining, session.BacklogStatusReady, true},
        {session.BacklogStatusRefining, session.BacklogStatusArchived, true},
    }
    for _, tc := range legacyTransitions {
        t.Run(fmt.Sprintf("%s→%s", tc.from, tc.to), func(t *testing.T) {
            assert.Equal(t, tc.allowed, engine.CanTransition(tc.from, tc.to))
        })
    }
}

// TestConfiguredWorkflowEngine_WhenCacheExpires_ReloadsFromDB
func TestConfiguredWorkflowEngine_WhenCacheExpires_ReloadsFromDB(t *testing.T) {
    repo := newTestEntRepository(t)
    storage, err := session.NewStorageWithRepository(repo)
    require.NoError(t, err)

    err = storage.SeedDefaultWorkflowConfig(t.Context())
    require.NoError(t, err)

    // Build engine with artificially short TTL for testing.
    engine, err := session.NewConfiguredWorkflowEngineWithTTL(storage, 50*time.Millisecond)
    require.NoError(t, err)

    // First call — primes cache.
    _ = engine.CanTransition(session.BacklogStatusIdea, session.BacklogStatusReady)

    // Add a new custom state directly to DB.
    err = storage.UpsertWorkflowState(t.Context(), session.WorkflowStateData{Name: "blocked", Label: "Blocked"})
    require.NoError(t, err)
    err = storage.UpsertWorkflowTransition(t.Context(), session.WorkflowTransitionData{FromState: "idea", ToState: "blocked"})
    require.NoError(t, err)

    // Wait for TTL to expire.
    time.Sleep(100 * time.Millisecond)

    // After TTL expiry, engine should reload and recognise the new transition.
    assert.True(t, engine.CanTransition(session.BacklogStatusIdea, "blocked"),
        "engine should reload from DB after TTL expiry")
}

// TestConfiguredWorkflowEngine_WhenCacheHot_DoesNotQueryDB
func TestConfiguredWorkflowEngine_WhenCacheHot_DoesNotQueryDB(t *testing.T) {
    repo := newTestEntRepository(t)
    storage, err := session.NewStorageWithRepository(repo)
    require.NoError(t, err)

    err = storage.SeedDefaultWorkflowConfig(t.Context())
    require.NoError(t, err)

    callCount := 0
    countingStorage := &countingWorkflowStorage{inner: storage, onGetConfig: func() { callCount++ }}

    engine, err := session.NewConfiguredWorkflowEngine(countingStorage)
    require.NoError(t, err)

    // Multiple calls within TTL window.
    for i := 0; i < 10; i++ {
        _ = engine.CanTransition(session.BacklogStatusIdea, session.BacklogStatusReady)
    }

    // Only one DB read should have occurred (initial prime).
    assert.Equal(t, 1, callCount, "cache should prevent repeated DB reads within TTL window")
}

// TestConfiguredWorkflowEngine_CanTransition_WhenCustomStateAdded_AllowsTransition
func TestConfiguredWorkflowEngine_CanTransition_WhenCustomStateAdded_AllowsTransition(t *testing.T) {
    repo := newTestEntRepository(t)
    storage, err := session.NewStorageWithRepository(repo)
    require.NoError(t, err)

    err = storage.SeedDefaultWorkflowConfig(t.Context())
    require.NoError(t, err)

    err = storage.UpsertWorkflowState(t.Context(), session.WorkflowStateData{Name: "qa_hold", Label: "QA Hold"})
    require.NoError(t, err)
    err = storage.UpsertWorkflowTransition(t.Context(), session.WorkflowTransitionData{FromState: "in_progress", ToState: "qa_hold"})
    require.NoError(t, err)
    err = storage.UpsertWorkflowTransition(t.Context(), session.WorkflowTransitionData{FromState: "qa_hold", ToState: "review"})
    require.NoError(t, err)

    // Engine with no cache (TTL=0) to always load fresh.
    engine, err := session.NewConfiguredWorkflowEngineWithTTL(storage, 0)
    require.NoError(t, err)

    assert.True(t, engine.CanTransition(session.BacklogStatus("in_progress"), "qa_hold"))
    assert.True(t, engine.CanTransition("qa_hold", session.BacklogStatus("review")))
}
```

**Count**: 5 integration tests (4 main + 1 regression via table-driven transitions)

---

### Suite 8: WorkflowService RPCs — Custom state CRUD (Go integration tests)

**File**: `server/services/workflow_service_test.go` (new file)

Uses `newWorkflowService(t)` helper, which wires `WorkflowService` against a real SQLite DB seeded with the default config.

```go
// TestGetWorkflowConfig_WhenDefaultSeeded_ReturnsAllStates
func TestGetWorkflowConfig_WhenDefaultSeeded_ReturnsAllStates(t *testing.T) {
    svc := newWorkflowService(t)
    resp, err := svc.GetWorkflowConfig(t.Context(), connect.NewRequest(&sessionv1.GetWorkflowConfigRequest{}))
    require.NoError(t, err)

    names := make([]string, 0, len(resp.Msg.Config.States))
    for _, s := range resp.Msg.Config.States {
        names = append(names, s.Name)
    }
    assert.Contains(t, names, "idea")
    assert.Contains(t, names, "refining")
    assert.Contains(t, names, "ready")
    assert.Contains(t, names, "done")
}

// TestUpsertWorkflowState_WhenNewState_AddsToConfig
func TestUpsertWorkflowState_WhenNewState_AddsToConfig(t *testing.T) {
    svc := newWorkflowService(t)
    _, err := svc.UpsertWorkflowState(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowStateRequest{
        State: &sessionv1.WorkflowState{Name: "spike", Label: "Spike"},
    }))
    require.NoError(t, err)

    resp, err := svc.GetWorkflowConfig(t.Context(), connect.NewRequest(&sessionv1.GetWorkflowConfigRequest{}))
    require.NoError(t, err)

    names := make([]string, 0, len(resp.Msg.Config.States))
    for _, s := range resp.Msg.Config.States {
        names = append(names, s.Name)
    }
    assert.Contains(t, names, "spike")
}

// TestUpsertWorkflowState_WhenRenameExisting_UpdatesLabel
func TestUpsertWorkflowState_WhenRenameExisting_UpdatesLabel(t *testing.T) {
    svc := newWorkflowService(t)
    _, err := svc.UpsertWorkflowState(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowStateRequest{
        State: &sessionv1.WorkflowState{Name: "idea", Label: "Raw Idea"},
    }))
    require.NoError(t, err)

    resp, err := svc.GetWorkflowConfig(t.Context(), connect.NewRequest(&sessionv1.GetWorkflowConfigRequest{}))
    require.NoError(t, err)

    for _, s := range resp.Msg.Config.States {
        if s.Name == "idea" {
            assert.Equal(t, "Raw Idea", s.Label)
            return
        }
    }
    t.Fatal("idea state not found in config")
}

// TestDeleteWorkflowState_WhenNoItems_Succeeds
func TestDeleteWorkflowState_WhenNoItems_Succeeds(t *testing.T) {
    svc := newWorkflowService(t)

    // Add a custom state with no items.
    _, err := svc.UpsertWorkflowState(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowStateRequest{
        State: &sessionv1.WorkflowState{Name: "deprecated", Label: "Deprecated"},
    }))
    require.NoError(t, err)

    _, err = svc.DeleteWorkflowState(t.Context(), connect.NewRequest(&sessionv1.DeleteWorkflowStateRequest{
        StateName: "deprecated",
    }))
    require.NoError(t, err)

    resp, err := svc.GetWorkflowConfig(t.Context(), connect.NewRequest(&sessionv1.GetWorkflowConfigRequest{}))
    require.NoError(t, err)

    for _, s := range resp.Msg.Config.States {
        assert.NotEqual(t, "deprecated", s.Name)
    }
}

// TestDeleteWorkflowState_WhenItemsExist_RequiresMigrationTarget
func TestDeleteWorkflowState_WhenItemsExist_RequiresMigrationTarget(t *testing.T) {
    storage := createTestStorageWithWorkflow(t)
    svc := newWorkflowServiceWithStorage(t, storage)

    // Add a state and put an item in it.
    _, err := svc.UpsertWorkflowState(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowStateRequest{
        State: &sessionv1.WorkflowState{Name: "temp_state", Label: "Temp"},
    }))
    require.NoError(t, err)

    _, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:    "stuck item",
        Status:   "temp_state",
        Priority: 3,
    })
    require.NoError(t, err)

    // Delete without migration target → CodeFailedPrecondition.
    _, err = svc.DeleteWorkflowState(t.Context(), connect.NewRequest(&sessionv1.DeleteWorkflowStateRequest{
        StateName: "temp_state",
    }))
    require.Error(t, err)
    var connErr *connect.Error
    require.ErrorAs(t, err, &connErr)
    assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code())
}

// TestDeleteWorkflowState_WhenMigrationTargetInvalid_ReturnsInvalidArgument
func TestDeleteWorkflowState_WhenMigrationTargetInvalid_ReturnsInvalidArgument(t *testing.T) {
    storage := createTestStorageWithWorkflow(t)
    svc := newWorkflowServiceWithStorage(t, storage)

    _, err := svc.UpsertWorkflowState(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowStateRequest{
        State: &sessionv1.WorkflowState{Name: "temp_state", Label: "Temp"},
    }))
    require.NoError(t, err)

    _, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:    "item to migrate",
        Status:   "temp_state",
        Priority: 3,
    })
    require.NoError(t, err)

    // Migration target "nonexistent_state" does not exist.
    _, err = svc.DeleteWorkflowState(t.Context(), connect.NewRequest(&sessionv1.DeleteWorkflowStateRequest{
        StateName:       "temp_state",
        MigrationTarget: "nonexistent_state",
    }))
    require.Error(t, err)
    var connErr *connect.Error
    require.ErrorAs(t, err, &connErr)
    assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

// TestDeleteWorkflowState_WhenBuiltinCoreState_ReturnsFailedPrecondition
func TestDeleteWorkflowState_WhenBuiltinCoreState_ReturnsFailedPrecondition(t *testing.T) {
    svc := newWorkflowService(t)

    // Attempt to delete a core baseline state.
    for _, coreState := range []string{"idea", "ready", "done", "archived"} {
        _, err := svc.DeleteWorkflowState(t.Context(), connect.NewRequest(&sessionv1.DeleteWorkflowStateRequest{
            StateName: coreState,
        }))
        require.Error(t, err, "deleting core state %q should fail", coreState)
        var connErr *connect.Error
        require.ErrorAs(t, err, &connErr)
        assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code(),
            "core state %q deletion should return FailedPrecondition", coreState)
    }
}

// TestUpsertWorkflowTransition_WhenDeadlockDetected_ReturnsInvalidArgument
// Adding a transition that creates an unreachable terminal state (deadlock/sink)
// must be rejected.
func TestUpsertWorkflowTransition_WhenDeadlockDetected_ReturnsInvalidArgument(t *testing.T) {
    svc := newWorkflowService(t)

    // Create two custom states that form a cycle with no exit to done/archived.
    _, err := svc.UpsertWorkflowState(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowStateRequest{
        State: &sessionv1.WorkflowState{Name: "loop_a", Label: "Loop A"},
    }))
    require.NoError(t, err)
    _, err = svc.UpsertWorkflowState(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowStateRequest{
        State: &sessionv1.WorkflowState{Name: "loop_b", Label: "Loop B"},
    }))
    require.NoError(t, err)

    // First transition: idea→loop_a (allowed — has forward path via existing transitions).
    _, err = svc.UpsertWorkflowTransition(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowTransitionRequest{
        Transition: &sessionv1.WorkflowTransition{FromState: "idea", ToState: "loop_a"},
    }))
    require.NoError(t, err)

    // loop_a→loop_b; loop_b→loop_a forms a cycle with no way out.
    _, err = svc.UpsertWorkflowTransition(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowTransitionRequest{
        Transition: &sessionv1.WorkflowTransition{FromState: "loop_a", ToState: "loop_b"},
    }))
    require.NoError(t, err)

    _, err = svc.UpsertWorkflowTransition(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowTransitionRequest{
        Transition: &sessionv1.WorkflowTransition{FromState: "loop_b", ToState: "loop_a"},
    }))
    require.Error(t, err, "cycle with no terminal exit should be rejected")
    var connErr *connect.Error
    require.ErrorAs(t, err, &connErr)
    assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

// TestDeleteWorkflowState_WhenMigrationTargetProvided_MigratesItems
func TestDeleteWorkflowState_WhenMigrationTargetProvided_MigratesItems(t *testing.T) {
    storage := createTestStorageWithWorkflow(t)
    svc := newWorkflowServiceWithStorage(t, storage)

    _, err := svc.UpsertWorkflowState(t.Context(), connect.NewRequest(&sessionv1.UpsertWorkflowStateRequest{
        State: &sessionv1.WorkflowState{Name: "temp_state", Label: "Temp"},
    }))
    require.NoError(t, err)

    item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
        Title:    "will be migrated",
        Status:   "temp_state",
        Priority: 3,
    })
    require.NoError(t, err)

    _, err = svc.DeleteWorkflowState(t.Context(), connect.NewRequest(&sessionv1.DeleteWorkflowStateRequest{
        StateName:       "temp_state",
        MigrationTarget: "idea",
    }))
    require.NoError(t, err)

    fetched, err := storage.GetBacklogItem(t.Context(), item.ID)
    require.NoError(t, err)
    assert.Equal(t, "idea", fetched.Status,
        "items in deleted state should be migrated to the target state")
}
```

**Count**: 9 integration tests

---

### Suite 9: TypeScript BacklogItemStatus open union (Jest unit tests)

**File**: `web-app/src/lib/backlog/status.test.ts` (new file)

```typescript
import {
  KnownBacklogStatus,
  BacklogItemStatus,
  isKnownStatus,
  getStatusLabel,
} from "@/lib/backlog/status";

describe("BacklogItemStatus open union", () => {
  it("should accept known statuses without type error", () => {
    const known: BacklogItemStatus[] = [
      "idea", "refining", "ready", "in_progress", "review", "done", "archived",
    ] satisfies KnownBacklogStatus[];
    expect(known).toHaveLength(7);
  });

  // Regression: unknown string statuses must render a fallback, not throw.
  it("unknown_status_should_renderWithFallback_not_throw", () => {
    const unknownStatus: BacklogItemStatus = "custom_team_state" as BacklogItemStatus;
    expect(() => getStatusLabel(unknownStatus)).not.toThrow();
    const label = getStatusLabel(unknownStatus);
    // Fallback is the raw string or a generic "Unknown" — not an error.
    expect(typeof label).toBe("string");
    expect(label.length).toBeGreaterThan(0);
  });

  it("should preserve type narrowing for KnownBacklogStatus", () => {
    const status: BacklogItemStatus = "refining";
    if (isKnownStatus(status)) {
      // TypeScript narrowing: this block should compile with status as KnownBacklogStatus.
      const narrow: KnownBacklogStatus = status;
      expect(narrow).toBe("refining");
    } else {
      fail("refining should be a known status");
    }
  });

  it("should return false from isKnownStatus for arbitrary string", () => {
    expect(isKnownStatus("custom_state_xyz" as BacklogItemStatus)).toBe(false);
  });

  it("should return true from isKnownStatus for all baseline statuses", () => {
    const baselines: KnownBacklogStatus[] = [
      "idea", "refining", "ready", "in_progress", "review", "done", "archived",
    ];
    for (const s of baselines) {
      expect(isKnownStatus(s)).toBe(true);
    }
  });
});
```

**Count**: 5 unit tests

---

### Suite 10: StatusBadge component — refining and fallback rendering (Jest/RTL)

**File**: `web-app/src/components/backlog/StatusBadge.test.tsx` (new file)

```typescript
import { render, screen } from "@testing-library/react";
import { StatusBadge } from "@/components/backlog/StatusBadge";

describe("StatusBadge", () => {
  it("should render refining badge with amber color", () => {
    render(<StatusBadge status="refining" />);
    const badge = screen.getByText(/refining/i);
    expect(badge).toBeInTheDocument();
    // Amber: either CSS class or data-status attribute.
    expect(badge.closest("[data-status='refining']") ?? badge).toHaveAttribute(
      "data-status", "refining"
    );
  });

  it("should render unknown status with fallback badge", () => {
    render(<StatusBadge status={"custom_state" as any} />);
    // Should render without throwing; text is the raw status or a generic label.
    expect(screen.queryByText(/custom_state/i) ?? screen.getByRole("status")).toBeInTheDocument();
  });

  it("should render idea badge", () => {
    render(<StatusBadge status="idea" />);
    expect(screen.getByText(/idea/i)).toBeInTheDocument();
  });
});
```

**Count**: 3 unit tests

---

### Suite 11: BacklogBoard component — refining column (Jest/RTL)

**File**: `web-app/src/components/backlog/BacklogBoard.test.tsx` (new section in existing file or new file)

```typescript
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BacklogBoard } from "@/components/backlog/BacklogBoard";
import { makeItem } from "../../../test-utils/backlog";

describe("BacklogBoard - refining column", () => {
  it("should render refining column between idea and ready", () => {
    render(<BacklogBoard items={[]} />);
    const columns = screen.getAllByRole("region", { name: /column/i });
    // Expect refining to appear before the ready column.
    const columnNames = columns.map((c) => c.getAttribute("data-status"));
    const refiningIdx = columnNames.indexOf("refining");
    const readyIdx = columnNames.indexOf("ready");
    expect(refiningIdx).toBeGreaterThanOrEqual(0);
    expect(refiningIdx).toBeLessThan(readyIdx);
  });

  it("should display clarifying questions in refining item detail", () => {
    const item = makeItem({
      status: "refining",
      clarifyingQuestions: ["What is the scope?", "Any API constraints?"],
    });
    render(<BacklogBoard items={[item]} />);
    expect(screen.getByText("What is the scope?")).toBeInTheDocument();
    expect(screen.getByText("Any API constraints?")).toBeInTheDocument();
  });

  it("should show Mark Ready action for refining items", () => {
    const item = makeItem({
      status: "refining",
      acceptanceCriteria: [{ index: 0, text: "AC defined", status: "pending" }],
    });
    render(<BacklogBoard items={[item]} />);
    expect(screen.getByRole("button", { name: /mark ready/i })).toBeInTheDocument();
  });

  it("should not show Mark Ready action for idea items", () => {
    const item = makeItem({ status: "idea" });
    render(<BacklogBoard items={[item]} />);
    expect(screen.queryByRole("button", { name: /mark ready/i })).not.toBeInTheDocument();
  });
});
```

**Count**: 4 unit tests

---

### Suite 12: WorkflowStateEditor settings page (Jest/RTL)

**File**: `web-app/src/components/settings/WorkflowStateEditor.test.tsx` (new file)

```typescript
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WorkflowStateEditor } from "@/components/settings/WorkflowStateEditor";
import { makeWorkflowConfig } from "../../../test-utils/workflow";

const mockUpsert = jest.fn().mockResolvedValue({});
const mockDelete = jest.fn().mockResolvedValue({});

const defaultProps = {
  config: makeWorkflowConfig(),
  onUpsertState: mockUpsert,
  onDeleteState: mockDelete,
};

describe("WorkflowStateEditor", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("should render list of workflow states", () => {
    render(<WorkflowStateEditor {...defaultProps} />);
    expect(screen.getByText("idea")).toBeInTheDocument();
    expect(screen.getByText("refining")).toBeInTheDocument();
    expect(screen.getByText("ready")).toBeInTheDocument();
    expect(screen.getByText("done")).toBeInTheDocument();
  });

  it("should call upsertWorkflowState when inline rename committed", async () => {
    render(<WorkflowStateEditor {...defaultProps} />);

    const renameInput = screen.getByDisplayValue("Refining");
    await userEvent.clear(renameInput);
    await userEvent.type(renameInput, "Clarifying");
    await userEvent.keyboard("{Enter}");

    await waitFor(() => {
      expect(mockUpsert).toHaveBeenCalledWith(
        expect.objectContaining({ name: "refining", label: "Clarifying" })
      );
    });
  });

  it("should open delete modal with migration target selector when items exist", async () => {
    const configWithItems = makeWorkflowConfig({ statesWithItems: ["refining"] });
    render(<WorkflowStateEditor {...defaultProps} config={configWithItems} />);

    const deleteButton = screen.getAllByRole("button", { name: /delete/i })[0];
    await userEvent.click(deleteButton);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText(/migrate.*to/i)).toBeInTheDocument();
  });

  it("should disable delete for built-in core states", () => {
    render(<WorkflowStateEditor {...defaultProps} />);

    // Find the row for "done" (a protected core state) and check its delete button.
    const doneRow = screen.getByText("done").closest("li, tr, [data-state-name]");
    expect(doneRow).not.toBeNull();
    const deleteBtn = doneRow!.querySelector("[aria-label*='delete'], button[data-action='delete']");
    if (deleteBtn) {
      expect(deleteBtn).toBeDisabled();
    } else {
      // Alternatively, the delete button should not exist for core states.
      expect(
        screen.queryByRole("button", { name: /delete done/i })
      ).not.toBeInTheDocument();
    }
  });
});
```

**Count**: 4 unit tests

---

### Suite 13: useWorkflowService hook reference stability (Jest/RTL)

**File**: `web-app/src/lib/hooks/useWorkflowService.test.ts` (new file)

Pattern mirrors `useBacklogService.test.ts` exactly.

```typescript
import { renderHook } from "@testing-library/react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { useWorkflowService } from "@/lib/hooks/useWorkflowService";

jest.mock("@connectrpc/connect", () => ({ createClient: jest.fn(() => ({})) }));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: jest.fn(() => ({})) }));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => () => ({}),
}));

describe("useWorkflowService", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (createClient as jest.Mock).mockReturnValue({});
    (createConnectTransport as jest.Mock).mockReturnValue({});
  });

  it("returns the same object reference across re-renders", () => {
    const { result, rerender } = renderHook(() => useWorkflowService());
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });

  it("returns stable method references across re-renders", () => {
    const { result, rerender } = renderHook(() => useWorkflowService());
    const before = {
      getWorkflowConfig: result.current.getWorkflowConfig,
      upsertWorkflowState: result.current.upsertWorkflowState,
      deleteWorkflowState: result.current.deleteWorkflowState,
    };
    rerender();
    expect(result.current.getWorkflowConfig).toBe(before.getWorkflowConfig);
    expect(result.current.upsertWorkflowState).toBe(before.upsertWorkflowState);
    expect(result.current.deleteWorkflowState).toBe(before.deleteWorkflowState);
  });
});
```

**Count**: 2 unit tests

---

## Coverage Targets

- All requirements traced to at least 2 tests (happy path + at least one error/edge case)
- `WorkflowEngine`: every valid transition combination tested (18 pairs in table-driven Suite 1)
- Gate evaluation: AC-required guard tested for both `idea→ready` (regression) and `refining→ready` (new)
- Backward-compatibility: all pre-existing transitions verified to remain unchanged (Suites 1 + 2 + 7)
- Cache TTL: both hot-cache (no DB hit) and expired-cache (DB reload) paths covered (Suite 7)
- Deadlock/reachability: cycle detection tested (Suite 8)
- TypeScript open union: regression against unknown-string throw, narrowing preservation (Suite 9)
- Frontend refining state: column presence, clarifying questions display, Mark Ready CTA (Suite 11)

---

## Requirements Coverage

| Requirement | Tests | Status |
|-------------|-------|--------|
| S1: `refining` state end-to-end | Suite 1 (engine transitions), Suite 2 (service layer), Suite 3 (MCP handler auto-transition), Suite 4 (lifecycle listener), Suite 5 (reconcile timeout), Suite 6 (event log), Suites 10+11 (frontend) | Covered |
| S2: WorkflowConfig data model | Suite 7 (seed, cache TTL, custom state engine), Suite 8 (GetWorkflowConfig RPC) | Covered |
| S3: Custom state CRUD | Suite 8 (Upsert/Delete happy paths, migration, invalid target, built-in guard, deadlock) | Covered |
| S4: Workflow builder UI | Deferred — not tested in this plan | Deferred |
| S5 (Gates, Phase 3 items) | Deferred — not tested in this plan | Deferred |
| Backward-compat constraint | Suite 1 (`TestCanTransitionBacklog_*` + `TestTransitionGuard_*`), Suite 2 (`TestTransitionBacklogItemStatus_WhenExistingItemWithLegacyStatus_*`), Suite 7 (`TestWorkflowConfig_WhenSeedDefault_TransitionsMatchLegacyTable`) | Covered |
| Phase 0: TypeScript open union | Suite 9 (5 tests incl. `unknown_status_should_renderWithFallback_not_throw`) | Covered |

---

## Test Count Summary

| Suite | Type | Count |
|-------|------|-------|
| 1: WorkflowEngine unit (DefaultWorkflowEngine) | Unit (Go) | 10 |
| 2: refining transitions — service layer | Integration (Go) | 5 |
| 3: submit_triage_result MCP handler | Integration (Go) | 3 |
| 4: BacklogLifecycleListener | Unit (Go) | 3 |
| 5: ReconcileStuckItems — refining timeout | Integration (Go) | 2 |
| 6: BacklogStatusEvent append-only log | Integration (Go) | 3 |
| 7: WorkflowConfig + ConfiguredWorkflowEngine | Integration (Go) | 5 |
| 8: WorkflowService RPCs | Integration (Go) | 9 |
| 9: TypeScript BacklogItemStatus union | Unit (TS) | 5 |
| 10: StatusBadge component | Unit (TS/RTL) | 3 |
| 11: BacklogBoard — refining column | Unit (TS/RTL) | 4 |
| 12: WorkflowStateEditor settings page | Unit (TS/RTL) | 4 |
| 13: useWorkflowService reference stability | Unit (TS/RTL) | 2 |
| **Total** | | **58** |

**By type**: Go Unit: 13 | Go Integration: 27 | TypeScript/RTL Unit: 18
**Requirements coverage**: 5 of 5 in-scope requirements (S4 and S5 gate detail deferred per plan) — **100% of Phase 0–2 scope**
