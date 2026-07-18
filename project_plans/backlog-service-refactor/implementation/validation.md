# Validation Plan: backlog-service-refactor
**Date**: 2026-07-09

## Happy Path Scenario

Given a backlog item with 2 acceptance criteria (one pending, one in-progress) and one active
triage `ItemSession` tracked in the database, when `ListBacklogItemSummaries` is called to
populate the board view, then the response contains a `BacklogItemSummary` with all scalar
fields populated and `ItemSessions` carrying `Role="triage"`, `Status="running"`, and the
correct `TriageResultSummary` for that item — without over-hydrating the full `BacklogItemData`.

---

## Requirement → Test Mapping

| Req | Test File | Test Name | Type | Scenario |
|-----|-----------|-----------|------|----------|
| **P1** Split `backlog_service.go` | `server/services/backlog_service_test.go` | `TestGetBacklogItem_should_returnItemData_When_itemExistsAfterFileSplit` | Unit | `GetBacklogItem` is callable from `backlog_service_query.go` (moved method); returns populated data |
| **P1** Split `backlog_service.go` | `server/services/backlog_service_test.go` | `TestGetBacklogItem_should_returnNotFound_When_itemMissingAfterFileSplit` | Unit | `GetBacklogItem` returns `connect.CodeNotFound` when storage returns `session.ErrNotFound` |
| **P1** Split `backlog_service.go` | `server/services/backlog_service_test.go` | `TestBacklogService_should_routeAllRPCs_When_splitAcrossMultipleFiles` | Integration | `go build ./server/services/...` green; ConnectRPC handler registration accepts all methods across 4 files |
| **P2** Extract `MergeAcCriteria` | `session/backlog_test.go` | `TestMergeAcCriteria_should_updateExistingAndAppendNew_When_validIncoming` | Unit | 1 existing (idx 0 pending) + incoming (idx 0 done, idx 1 pending) → idx 0 updated, idx 1 appended; parses cleanly via `ParseAcCriteria` |
| **P2** Extract `MergeAcCriteria` | `session/backlog_test.go` | `TestMergeAcCriteria_should_returnError_When_incomingContainsDuplicateIndex` | Unit | Incoming has two `{Index:0, …}` entries → error returned; result is zero-value `AcCriteriaJSON` |
| **P2** Extract `MergeAcCriteria` | `session/backlog_test.go` | `TestMergeAcCriteria_should_preserveUnmentionedCriteria_When_incomingIsPartialUpdate` | Unit | 3 existing (idx 0,1,2); incoming only updates idx 1 → idx 0 and idx 2 survive unchanged |
| **P2** Extract `MergeAcCriteria` | `session/backlog_test.go` | `TestMergeAcCriteria_should_appendAtNonContiguousIndex_When_gapInIncoming` | Unit | Existing has idx 0; incoming adds idx 5 → result has idx 0 and idx 5, no renumbering |
| **P2** Extract `MergeAcCriteria` | `session/backlog_test.go` | `TestMergeAcCriteria_should_returnEmptyList_When_bothExistingAndIncomingEmpty` | Unit | `existing=nil`, `incoming=[]` → result parses to empty `[]AcCriterion` with no error |
| **P2** Extract `MergeAcCriteria` | `server/mcp/tools_backlog_test.go` | `TestSubmitTriageResult_should_mergeCriteriaAndPersist_When_partialAcUpdate` | Integration | MCP handler calls `session.MergeAcCriteria`; merged `AcCriteriaJSON` is written back to storage; round-trip verified via `GetBacklogItem` |
| **P3** Extract `ReviewGateRunner` | `session/review_gate_test.go` | `TestReviewGateRunner_Run_should_invokeOnPass_When_headlessReturnsPass` | Unit | Fake pool returns PASS for all criteria; `onPass` called exactly once; `autoReopener` NOT called; no error |
| **P3** Extract `ReviewGateRunner` | `session/review_gate_test.go` | `TestReviewGateRunner_Run_should_invokeAutoReopener_When_headlessReturnsFail` | Unit | Fake pool returns FAIL; `autoReopener` called exactly once with correct `itemID`; `onPass` NOT called |
| **P3** Extract `ReviewGateRunner` | `session/review_gate_test.go` | `TestReviewGateRunner_Run_should_skipHeadlessPool_When_skipReviewGateTrue` | Unit | Item has `SkipReviewGate: true`; pool `CallBlockingWithOptions` never invoked; session ended without verdict; no error |
| **P3** Extract `ReviewGateRunner` | `session/backlog_integration_test.go` | `TestReviewGateRunner_Run_should_persistItemSessionVerdict_When_reviewCompletes` | Integration | Real SQLite (ent in-memory via `createTestStorage`); after `Run` with PASS fake pool, fetched `ItemSessionSummary.VerdictAt` is non-nil; `OverallOutcome` equals `ReviewOutcomePASS` |
| **P4** `BacklogItemSummary` | `session/backlog_test.go` | `TestBacklogItemSummary_should_exposeAllScalarFields_When_constructed` | Unit | Construct `BacklogItemSummary` with known field values; assert all 14 scalar fields and `ItemSessions` slice are accessible; no `*ent.BacklogItem` type in signature |
| **P4** `BacklogItemSummary` | `session/backlog_integration_test.go` | `TestListBacklogItemSummaries_should_returnEmptyItemSessions_When_itemHasNoSessions` | Integration | Item with no `ItemSession` rows in DB → `BacklogItemSummary.ItemSessions` is non-nil empty slice (not nil) |
| **P4** `BacklogItemSummary` | `session/backlog_integration_test.go` | `TestListBacklogItemSummaries_should_includeCorrectSessionData_When_itemHasActiveTriage` | Integration | Item A: 1 triage session (role=triage, status=running, triage_result_summary="All clear"); item B: no sessions; assert A.`ItemSessions[0].Role=="triage"`, A.`ItemSessions[0].Status=="running"`, A.`ItemSessions[0].TriageResultSummary=="All clear"`; B.`ItemSessions` is empty |
| **P4** `BacklogItemSummary` | `session/backlog_integration_test.go` | `TestListBacklogItemSummaries_should_notCrossPageBoundaries_When_pageScopeApplied` | Integration | 50 items in DB; items 1–10 each have 1 session; items 11–50 have none; first page (10 items); second IN-list query uses ≤ 10 IDs; sessions for items 11–50 do NOT appear in first-page results |
| **P5** `session/domain` sub-package | `session/domain/backlog_test.go` | `TestParseAcCriteria_should_returnCriteriaSlice_When_validJSON` | Unit | `domain.ParseAcCriteria` on a valid JSON string returns correct `[]AcCriterion` with all fields populated |
| **P5** `session/domain` sub-package | `session/domain/backlog_test.go` | `TestParseAcCriteria_should_returnError_When_invalidJSON` | Unit | Malformed JSON → non-nil error; returned slice is nil |
| **P5** `session/domain` sub-package | `session/backlog_test.go` | `TestDomainTypeAlias_should_compileTransparently_When_callerUsesSessionPackage` | Unit | `session.AcCriterion{}` assigned to `domain.AcCriterion` variable without type conversion; `session.BacklogStatus` constant compared to `domain.BacklogStatus` constant without cast; `go build ./session/...` green |
| **P6** Storage interface / ent cleanup | `session/ent_repository_backlog_test.go` | `TestGetItemSessionBySessionAndItem_should_returnSummaryDTO_When_recordExists` | Unit | `GetItemSessionBySessionAndItem` return type is `ItemSessionSummary` (not `*ent.ItemSession`); all mapped fields (`ID`, `SessionUUID`, `Role`, `Status`, `TriageResultSummary`) match DB row |
| **P6** Storage interface / ent cleanup | `session/ent_repository_backlog_test.go` | `TestGetItemSessionBySessionAndItem_should_returnErrNotFound_When_recordMissing` | Unit | Non-existent session UUID → `errors.Is(err, session.ErrNotFound)` is true; `ent.IsNotFound` is NOT called by the caller (call site never sees raw ent errors) |
| **P6** Storage interface / ent cleanup | `session/backlog_integration_test.go` | `TestEntRepository_should_wrapEntErrors_When_multipleGetMethodsReturnNotFound` | Integration | `GetItemSession`, `GetItemSessionBySessionUUID`, `GetItemSessionBySessionAndItem` each called with non-existent IDs on real SQLite DB; all return `session.ErrNotFound` via `errors.Is`; none surface `ent.IsNotFound` |

---

## Table-Driven Sub-Cases for `TestMergeAcCriteria`

The five unit tests above map to a single `TestMergeAcCriteria` table-driven test with named
sub-cases, matching the plan's Story 2.1.2 requirement of ≥5 cases:

```go
// session/backlog_test.go — package session

func TestMergeAcCriteria(t *testing.T) {
    cases := []struct {
        name     string
        existing []AcCriterion
        incoming []AcCriterion
        wantLen  int
        wantErr  bool
        wantIdx0 AcStatus // status of criterion at index 0 in result
    }{
        {
            name:     "empty_existing_non_empty_incoming",
            existing: nil,
            incoming: []AcCriterion{{Index: 3, Text: "AC A", Status: AcStatusPending}, {Index: 7, Text: "AC B", Status: AcStatusPending}},
            wantLen:  2,
        },
        {
            name:     "update_existing_criterion",
            existing: []AcCriterion{{Index: 0, Text: "Write unit tests", Status: AcStatusPending}},
            incoming: []AcCriterion{{Index: 0, Text: "Write unit tests", Status: AcStatusDone}},
            wantLen:  1,
            wantIdx0: AcStatusDone,
        },
        {
            name:     "preserve_unmentioned_criteria",
            existing: []AcCriterion{{Index: 0, Text: "A", Status: AcStatusPending}, {Index: 1, Text: "B", Status: AcStatusPending}, {Index: 2, Text: "C", Status: AcStatusPending}},
            incoming: []AcCriterion{{Index: 1, Text: "B", Status: AcStatusDone}},
            wantLen:  3,
        },
        {
            name:     "append_new_criterion_with_gap_in_indices",
            existing: []AcCriterion{{Index: 0, Text: "A", Status: AcStatusPending}},
            incoming: []AcCriterion{{Index: 5, Text: "B", Status: AcStatusPending}},
            wantLen:  2,
        },
        {
            name:    "duplicate_index_in_incoming_returns_error",
            existing: []AcCriterion{{Index: 0, Text: "A", Status: AcStatusPending}},
            incoming: []AcCriterion{{Index: 0, Text: "X"}, {Index: 0, Text: "Y"}},
            wantErr: true,
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            result, err := MergeAcCriteria(tc.existing, tc.incoming)
            if tc.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            parsed, err := ParseAcCriteria(string(result))
            require.NoError(t, err)
            require.Len(t, parsed, tc.wantLen)
        })
    }
}
```

---

## UX Acceptance Tests

N/A — no user-facing surface. This is a pure infrastructure refactoring.

The one UI-adjacent risk (board view triage spinner and "View Session" button silently broken
by P4) is covered by the integration tests `TestListBacklogItemSummaries_should_includeCorrectSessionData_When_itemHasActiveTriage`
and the manual smoke test step described in plan.md Story 5.1.3:

> Manual smoke: start a triage session for a backlog item; load board view; confirm spinner.

This is a post-implementation verification step, not an automated test. Document it as a
required step in each P4 PR checklist.

---

## Test Stack

- **Unit**: Go stdlib `testing` + `testify/require`, table-driven tests; no real DB or network
- **Integration**: Go stdlib `testing` with ent SQLite in-memory via `createTestStorage(t)`
  (existing pattern in `session/backlog_integration_test.go`); database is seeded per-test and
  cleaned up via `defer cleanup()`
- **E2E / UX**: N/A (no user-facing surface changed)
- **Fuzz** (optional, Story 2.1.2): `FuzzMergeAcCriteria` in `session/backlog_test.go` with
  1 seed corpus entry; invoked via `go test ./session/... -run=^$ -fuzz=FuzzMergeAcCriteria -fuzztime=5s`

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go (all packages) | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line coverage |
| `session/` package focus | `go test ./session/... -coverprofile=session.out && go tool cover -func=session.out \| grep -E "MergeAcCriteria\|ReviewGateRunner\|ListBacklogItemSummaries"` | 100% line on all three new functions |
| `server/services/` regression guard | `go test ./server/services/... -count=1` | All existing tests pass; no new failures |

Run `make quick-check` after each phase PR to confirm build + test + lint all green before
opening the PR.

---

## Test ID Cross-Reference

Test IDs used here correspond to the plan's `IT-NNN` / `UT-NNN` notation for traceability:

| Plan Story | Test ID | Test Name |
|---|---|---|
| Story 2.1.2 | UT-MRG-001 | `TestMergeAcCriteria/empty_existing_non_empty_incoming` |
| Story 2.1.2 | UT-MRG-002 | `TestMergeAcCriteria/update_existing_criterion` |
| Story 2.1.2 | UT-MRG-003 | `TestMergeAcCriteria/preserve_unmentioned_criteria` |
| Story 2.1.2 | UT-MRG-004 | `TestMergeAcCriteria/append_new_criterion_with_gap_in_indices` |
| Story 2.1.2 | UT-MRG-005 | `TestMergeAcCriteria/duplicate_index_in_incoming_returns_error` |
| Story 3.2.2 | UT-RGR-001 | `TestReviewGateRunner_Run_should_invokeOnPass_When_headlessReturnsPass` |
| Story 3.2.2 | UT-RGR-002 | `TestReviewGateRunner_Run_should_invokeAutoReopener_When_headlessReturnsFail` |
| Story 3.2.2 | UT-RGR-003 | `TestReviewGateRunner_Run_should_skipHeadlessPool_When_skipReviewGateTrue` |
| Story 3.2.2 | IT-RGR-004 | `TestReviewGateRunner_Run_should_persistItemSessionVerdict_When_reviewCompletes` |
| Story 5.1.2 | IT-SUM-001 | `TestListBacklogItemSummaries_should_returnEmptyItemSessions_When_itemHasNoSessions` |
| Story 5.1.2 | IT-SUM-002 | `TestListBacklogItemSummaries_should_includeCorrectSessionData_When_itemHasActiveTriage` |
| Story 5.1.2 | IT-SUM-003 | `TestListBacklogItemSummaries_should_notCrossPageBoundaries_When_pageScopeApplied` |
| Story 4.1.2 | UT-DTO-001 | `TestGetItemSessionBySessionAndItem_should_returnSummaryDTO_When_recordExists` |
| Story 4.1.2 | UT-DTO-002 | `TestGetItemSessionBySessionAndItem_should_returnErrNotFound_When_recordMissing` |
| Story 4.1.4 | IT-DTO-003 | `TestEntRepository_should_wrapEntErrors_When_multipleGetMethodsReturnNotFound` |
| Story 6.1.1 | UT-DOM-001 | `TestParseAcCriteria_should_returnCriteriaSlice_When_validJSON` |
| Story 6.1.1 | UT-DOM-002 | `TestParseAcCriteria_should_returnError_When_invalidJSON` |
| Story 6.1.2 | UT-DOM-003 | `TestDomainTypeAlias_should_compileTransparently_When_callerUsesSessionPackage` |
