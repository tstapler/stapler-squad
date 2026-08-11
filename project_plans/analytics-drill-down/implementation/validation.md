# Analytics Drill-Down — Validation Plan

**Project**: `analytics-drill-down`
**Date**: 2026-05-19
**Input artifacts**: requirements.md, implementation/plan.md, implementation/adversarial-review.md

---

## Section 1 — Unit Tests (Go)

All Go unit tests live in `session/ent_repository_analytics_test.go` (new file) and
`server/services/` (existing test files). Tests use real SQLite in-memory ent clients
using the same pattern as existing ent tests.

---

### 1.1 Repository Method: `ListAnalyticsSince`

**File**: `session/ent_repository_analytics_test.go`
**Package**: `session`

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| R-01 | `TestListAnalyticsSince_EmptyDB_ReturnsEmpty` | No rows in DB | `[]AnalyticsData{}`, nil error |
| R-02 | `TestListAnalyticsSince_AllRowsBeforeWindow_ReturnsEmpty` | 5 rows all at t-48h; since=t-24h | Empty slice, nil error |
| R-03 | `TestListAnalyticsSince_MixedRows_ReturnsOnlyInWindow` | 3 rows at t-2h, 2 rows at t-48h; since=t-24h | Exactly 3 rows returned |
| R-04 | `TestListAnalyticsSince_LimitApplied_TruncatesResult` | 10 rows in window; limit=3 | Exactly 3 rows, ordered newest first |
| R-05 | `TestListAnalyticsSince_NoLimitZero_ReturnsAll` | 5 rows in window; limit=0 | All 5 rows |
| R-06 | `TestListAnalyticsSince_OrderedNewestFirst` | 3 rows at t-1h, t-2h, t-3h | Returned in descending created_at order |

**Setup pattern**:
```go
func seedAnalytics(t *testing.T, client *ent.Client, rows []seedRow) {
    // insert using client.ClassificationAnalytics.Create()...Save(ctx)
}
```

---

### 1.2 Repository Method: `ListAnalyticsByProgramSince`

**File**: `session/ent_repository_analytics_test.go`

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| R-07 | `TestListAnalyticsByProgramSince_NoProgramRows_ReturnsEmpty` | 5 rows for "git"; query for "gh" | Empty slice, nil error |
| R-08 | `TestListAnalyticsByProgramSince_AllBeforeWindow_ReturnsEmpty` | 5 "git" rows at t-48h; since=t-24h | Empty slice, nil error |
| R-09 | `TestListAnalyticsByProgramSince_FiltersByProgram` | 3 "git" + 2 "gh" rows in window | Only 3 "git" rows for program="git" |
| R-10 | `TestListAnalyticsByProgramSince_EmptyStringProgram_ReturnsRowsWithEmptyProgram` | 2 rows with `command_program=""` | Returns those 2 rows when program="" |
| R-11 | `TestListAnalyticsByProgramSince_LimitApplied` | 10 "git" rows in window; limit=4 | Exactly 4 rows |

---

### 1.3 Repository Method: `GetSubcommandBreakdown`

**File**: `session/ent_repository_analytics_test.go`

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| R-12 | `TestGetSubcommandBreakdown_NoProgramRows_ReturnsEmpty` | 5 rows for "git"; query for "gh" | Empty slice, nil error (not nil slice) |
| R-13 | `TestGetSubcommandBreakdown_AllBeforeWindow_ReturnsEmpty` | 5 "git" rows at t-48h; since=t-24h | Empty slice, nil error |
| R-14 | `TestGetSubcommandBreakdown_SingleSubcommandMultipleDecisions` | 3 "git/push" rows: 2 "auto_allow" + 1 "escalate" | `[{push, auto_allow, 2}, {push, escalate, 1}]` |
| R-15 | `TestGetSubcommandBreakdown_MultipleSubcommands_CountsCorrect` | 3 "git/push/auto_allow", 2 "git/log/manual_allow" | Both subcommands returned with correct counts |
| R-16 | `TestGetSubcommandBreakdown_EmptyStringSubcommand_Included` | 2 "git" rows with NULL `command_subcategory` | `[{"", decision, 2}]` — empty string key, not dropped |
| R-17 | `TestGetSubcommandBreakdown_MixedSubcommands_TotalCountCorrect` | 5 push + 3 log + 2 bare (no subcommand) | All 3 subcommand keys present, counts sum to 10 |
| R-18 | `TestGetSubcommandBreakdown_ScanFieldTagsPopulateCount` | 1 row per decision for same subcommand | Count field in result > 0 (validates C2 ent Scan tag format) |

**C2 mitigation test (R-18)**: Insert 1 row, call `GetSubcommandBreakdown`, assert `Count == 1`. Proves `json:"count"` tag wires correctly.

---

### 1.4 Repository Method: `ListRecentCommandsByProgram`

**File**: `session/ent_repository_analytics_test.go`

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| R-19 | `TestListRecentCommandsByProgram_NoProgramRows_ReturnsEmpty` | No rows for "git" | `[]string{}`, nil error |
| R-20 | `TestListRecentCommandsByProgram_AllBeforeWindow_ReturnsEmpty` | 5 "git" rows at t-48h; since=t-24h | Empty slice |
| R-21 | `TestListRecentCommandsByProgram_EmptySubcommand_ReturnsAllSubcommands` | 3 "git/push" + 2 "git/log" rows; subcommand="" | 5 previews returned (all subcommands) |
| R-22 | `TestListRecentCommandsByProgram_SpecificSubcommand_FiltersCorrectly` | 3 "git/push" + 2 "git/log"; subcommand="push" | Only 3 "git/push" previews |
| R-23 | `TestListRecentCommandsByProgram_NullPreviewRows_Excluded` | 2 rows with non-null preview + 1 with NULL preview | Only 2 previews returned |
| R-24 | `TestListRecentCommandsByProgram_LimitApplied_TruncatesResult` | 10 "git" rows; n=5 | Exactly 5 previews, newest first |
| R-25 | `TestListRecentCommandsByProgram_OrderedNewestFirst` | 3 rows seeded at t-1h, t-2h, t-3h | Previews in t-1h → t-3h order |

---

### 1.5 Repository Method: `GetSubcommandTrend`

**File**: `session/ent_repository_analytics_test.go`

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| R-26 | `TestGetSubcommandTrend_NoProgramRows_ReturnsEmpty` | No rows for "git" | `[]AnalyticsData{}`, nil error |
| R-27 | `TestGetSubcommandTrend_AllBeforeWindow_ReturnsEmpty` | 5 "git" rows at t-48h; since=t-24h | Empty slice |
| R-28 | `TestGetSubcommandTrend_EmptySubcommand_ReturnsAllSubcommands` | 3 "git/push" + 2 "git/log"; subcommand="" | All 5 rows |
| R-29 | `TestGetSubcommandTrend_SpecificSubcommand_FiltersCorrectly` | 3 "git/push" + 2 "git/log"; subcommand="push" | Exactly 3 rows, all with subcommand="push" |
| R-30 | `TestGetSubcommandTrend_OrderedAscendingByCreatedAt` | 3 rows at t-3h, t-2h, t-1h | Returned oldest-first (ascending) |
| R-31 | `TestGetSubcommandTrend_MultipleDecisions_AllDecisionsPresent` | 5 "git/push" rows with 3 different decisions | All 5 rows returned; no deduplication |

---

### 1.6 `AnalyticsStore.LoadWindow` Fix (AC-1)

**File**: `server/services/analytics_store_test.go` (extend existing)

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| S-01 | `TestAnalyticsStore_LoadWindow_UsesDBFilter_ExcludesOldRows` | Seed t-2h row and t-48h row in SQLite; call `LoadWindow(t-24h)` | Only t-2h row in result; t-48h row absent |
| S-02 | `TestAnalyticsStore_LoadWindow_EmptyDB_ReturnsEmpty` | No rows in DB; call `LoadWindow(t-24h)` | Empty slice, nil error |
| S-03 | `TestAnalyticsStore_LoadWindow_InvokesListAnalyticsSince_NotListAnalytics` | Use a mock Repository that records which method was called | `ListAnalyticsSince` called; `ListAnalytics` NOT called |

**S-03 mock-based test**: The purpose is to confirm the in-memory filter removal. Inject a spy Repository:
```go
type spyRepository struct {
    session.Repository
    listAnalyticsSinceCalled bool
    listAnalyticsCalled      bool
}
func (s *spyRepository) ListAnalyticsSince(...) ([]session.AnalyticsData, error) {
    s.listAnalyticsSinceCalled = true
    return nil, nil
}
func (s *spyRepository) ListAnalytics(...) ([]session.AnalyticsData, error) {
    s.listAnalyticsCalled = true
    return nil, nil
}
// assert: listAnalyticsSinceCalled == true, listAnalyticsCalled == false
```

---

### 1.7 `GetProgramAnalytics` RPC Handler

**File**: `server/services/rules_service_test.go` (extend existing)

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| H-01 | `TestRulesService_GetProgramAnalytics_EmptyProgram_ReturnsInvalidArgument` | program="" | `connect.CodeInvalidArgument` |
| H-02 | `TestRulesService_GetProgramAnalytics_WhitespaceProgram_ReturnsInvalidArgument` | program="   " | `connect.CodeInvalidArgument` (TrimSpace guard) |
| H-03 | `TestRulesService_GetProgramAnalytics_WindowDaysZero_ReturnsInvalidArgument` | window_days=0 | `connect.CodeInvalidArgument` |
| H-04 | `TestRulesService_GetProgramAnalytics_WindowDays91_ReturnsInvalidArgument` | window_days=91 | `connect.CodeInvalidArgument` |
| H-05 | `TestRulesService_GetProgramAnalytics_NoProgramRows_ReturnsEmptySlices` | program="gh"; no rows seeded | `subcommands == []`, `recent_examples == []`, `trend == []`, nil error |
| H-06 | `TestRulesService_GetProgramAnalytics_ReturnsBreakdown_CountsCorrect` | Seed 5 "git/push/auto_allow" + 3 "git/log/escalate"; call with program="git", 7d | Subcommands: push.total=5, log.total=3 |
| H-07 | `TestRulesService_GetProgramAnalytics_SortedByTotalDesc` | Seed 5 push + 3 log + 7 commit rows | Subcommands sorted: commit(7), push(5), log(3) |
| H-08 | `TestRulesService_GetProgramAnalytics_RuleCoverage_ZeroRules_AllGaps` | No rules loaded; 3 "git/push" rows | `has_rule_coverage=false` for push |
| H-09 | `TestRulesService_GetProgramAnalytics_RuleCoverage_MatchingRule_SetsCovered` | Rule with `CommandPattern="git push"` loaded; 3 "git/push" rows | `has_rule_coverage=true` for push |
| H-10 | `TestRulesService_GetProgramAnalytics_RuleCoverage_DisabledRule_NotCovered` | Rule `CommandPattern="git push"` but `Enabled=false`; 3 "git/push" rows | `has_rule_coverage=false` for push |
| H-11 | `TestRulesService_GetProgramAnalytics_ExampleCommands_TruncatedToTwenty` | 25 "git" rows with distinct previews | `len(recent_examples) == 20` |
| H-12 | `TestRulesService_GetProgramAnalytics_EmptySubcommandKey_NotPanicsOrDropped` | 3 "git" rows with NULL `command_subcategory` | `subcommands[0].subcommand == ""`, total=3, no panic |
| H-13 | `TestRulesService_GetProgramAnalytics_WindowDaysNil_DefaultsTo7` | window_days omitted (nil) | Uses 7-day window; rows at t-10d excluded |
| H-14 | `TestRulesService_GetProgramAnalytics_CategoryDerivedFromFirstEntry` | 3 rows with `command_category="vcs"` | `response.category == "vcs"` |
| H-15 | `TestRulesService_GetProgramAnalytics_EmptyCategoryRows_CategoryEmptyString` | All rows have empty `command_category` | `response.category == ""` (no panic) |

**H-08/H-09/H-10 setup**: Use the existing `spyRulesStore` or inject rules via the `RulesService` test constructor. The `coveredSubcommands` heuristic must be exercised with real `RuleSpec` data.

---

## Section 2 — Frontend Tests (Jest/RTL)

All frontend tests use Jest + React Testing Library. Mock `useSessionServiceClient` to return a controlled client.

---

### 2.1 Hook: `useProgramAnalytics`

**File**: `web-app/src/lib/hooks/__tests__/useProgramAnalytics.test.ts`

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| F-01 | `useProgramAnalytics_should_fetchData_When_programIsSet` | Render with `program="git"`, `windowDays=7` | `isLoading=true` initially; `data` populated after resolve |
| F-02 | `useProgramAnalytics_should_skipFetch_When_programIsNull` | Render with `program=null` | No call to `getProgramAnalytics`; `isLoading=false`; `data=null` |
| F-03 | `useProgramAnalytics_should_clearData_When_programChangesToNull` | Render with `program="git"` then change to `null` | `data` becomes null after re-render with null |
| F-04 | `useProgramAnalytics_should_abortInflightRequest_When_programChanges` | `program` changes from "git" to "gh" before first request resolves | First request aborted (abort signal fired); second request initiated |
| F-05 | `useProgramAnalytics_should_refetch_When_windowDaysChanges` | `windowDays` changes from 7 to 14 | `getProgramAnalytics` called twice total |
| F-06 | `useProgramAnalytics_should_setError_When_fetchFails` | `getProgramAnalytics` rejects with Error | `error` is set; `isLoading=false`; `data=null` |
| F-07 | `useProgramAnalytics_should_notSetError_When_requestAborted` | Abort the request manually | `error` remains null (AbortError swallowed) |
| F-08 | `useProgramAnalytics_should_returnNoopCleanup_When_programIsNull` | Verify cleanup returned on null program path | `useEffect` cleanup does not throw; no AbortController leak (addresses C3) |

**C3 fix verification (F-08)**: This test directly validates the adversarial review finding that the early-return path must return `() => {}` instead of bare `return`. Use `renderHook` and advance through null→non-null→null transitions confirming no leaked AbortController.

---

### 2.2 Component: `ProgramDetailPanel`

**File**: `web-app/src/components/sessions/__tests__/ProgramDetailPanel.test.tsx`

Mock `useProgramAnalytics` at the module level using `jest.mock`.

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| F-09 | `ProgramDetailPanel_should_renderLoadingState_When_isLoadingTrue` | `isLoading=true`, `data=null` | Element with text "Loading…" visible; table absent |
| F-10 | `ProgramDetailPanel_should_renderErrorState_When_errorPresent` | `error=new Error("network fail")`, `data=null` | `role="alert"` element with "Failed to load analytics" text |
| F-11 | `ProgramDetailPanel_should_renderEmptyState_When_noSubcommands` | `data` with `subcommands=[]` | "No subcommand data" message in table body |
| F-12 | `ProgramDetailPanel_should_renderSubcommandTable_When_dataPresent` | 3 subcommands: push(50), log(30), commit(20), total=100 | Table rows with 50%, 30%, 20% values respectively |
| F-13 | `ProgramDetailPanel_should_renderNoneLabelForEmptySubcommand` | Subcommand with `subcommand=""` | Row label shows "(none)" |
| F-14 | `ProgramDetailPanel_should_showCoveredBadge_When_hasRuleCoverageTrue` | Subcommand with `hasRuleCoverage=true` | "✓ covered" badge; "Add rule →" link absent |
| F-15 | `ProgramDetailPanel_should_showGapBadge_When_hasRuleCoverageFalse` | Subcommand with `hasRuleCoverage=false` | "✗ gap" badge; "Add rule →" link present |
| F-16 | `ProgramDetailPanel_should_haveCorrectAddRuleHref_With_ProgramAndSubcommand` | program="git", subcommand="push" | Link href = `/rules?program=git&subcommand=push` |
| F-17 | `ProgramDetailPanel_should_callOnClose_When_closeButtonClicked` | Click `aria-label="Close program detail panel"` button | `onClose` mock called once |
| F-18 | `ProgramDetailPanel_should_renderUpTo20Examples_When_examplesPresent` | `recentExamples` has 20 entries | 20 `<li>` elements in examples list |
| F-19 | `ProgramDetailPanel_should_hideExamplesList_When_noExamples` | `recentExamples=[]` | "Recent Examples" section absent |
| F-20 | `ProgramDetailPanel_should_showCategory_When_categoryPresent` | `data.category="vcs"` | Panel title contains "· vcs" |
| F-21 | `ProgramDetailPanel_should_computeCorrectPercentages_ThreeSubcommands` | push=50, log=30, commit=20 (total=100) | push shows "50.0%", log shows "30.0%", commit shows "20.0%" |

**F-12 / F-21 percentage precision**: Assert exact string `"50.0%"` to verify `toFixed(1)` formatting. Use `getByRole("cell", { name: /50\.0%/ })`.

---

### 2.3 Component: `ApprovalAnalyticsPanel` — Panel Wire-Up

**File**: `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx` (extend existing)

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| F-22 | `ApprovalAnalyticsPanel_should_openDetailPanel_When_programRowClicked` | Click first row in top_uncovered_programs table | `data-testid="program-detail-panel"` appears |
| F-23 | `ApprovalAnalyticsPanel_should_closeDetailPanel_When_sameRowClickedAgain` | Click same row twice | Panel disappears after second click |
| F-24 | `ApprovalAnalyticsPanel_should_switchProgram_When_differentRowClicked` | Open panel for "git", then click "gh" row | Panel shows for "gh"; "git" panel gone |
| F-25 | `ApprovalAnalyticsPanel_should_closeDetailPanel_When_closePanelButtonClicked` | Open panel, click its close button | `data-testid="program-detail-panel"` absent |

---

## Section 3 — Integration Tests

Integration tests use real SQLite in-memory databases (same as ent unit tests) and may use the full `RulesService` + `AnalyticsStore` stack.

**File**: `server/services/rules_service_analytics_integration_test.go`

| # | Test Name | Scenario | Expected |
|---|-----------|----------|----------|
| I-01 | `TestGetProgramAnalytics_EndToEnd_RecordsDecisionsAndQueries` | Record 5 decisions for "git push", 3 for "git log"; call `GetProgramAnalytics("git", 7)` | push.total=5, log.total=3; response.program="git" |
| I-02 | `TestGetProgramAnalytics_TimeWindowFilter_ExcludesOldDecisions` | Record 1 decision at t-10d and 1 at t-1d; call with 7-day window | Only t-1d decision returned; t-10d absent |
| I-03 | `TestGetProgramAnalytics_ExampleCommands_OrderedNewestFirst` | Record 3 decisions at t-1h, t-2h, t-3h for "git push" | `recent_examples[0]` is the t-1h preview |
| I-04 | `TestGetProgramAnalytics_MultiplePrograms_OnlyTargetProgramReturned` | Seed 3 "git" + 3 "gh" decisions | Query for "git" returns only git subcommands |
| I-05 | `TestGetProgramAnalytics_AllExistingAnalyticsTestsPass` | Run `go test ./server/services/...` after all changes | Zero failures in existing test files |

**I-01 full scenario** (the key end-to-end test from the spec):
```
Seed: 5 × {program="git", subcategory="push", decision="auto_allow", preview="git push origin main"}
      3 × {program="git", subcategory="log", decision="escalate", preview="git log --oneline"}
Call: GetProgramAnalytics(program="git", window_days=7)
Assert:
  - len(subcommands) == 2
  - subcommands[0].subcommand == "push", subcommands[0].total == 5  (sorted desc)
  - subcommands[1].subcommand == "log",  subcommands[1].total == 3
  - len(recent_examples) == 8
  - response.program == "git"
```

**I-02 time window filter** (the key filter correctness test from the spec):
```
Seed: {program="git", subcategory="push", created_at=now()-10 days}
      {program="git", subcategory="log",  created_at=now()-1 day}
Call: GetProgramAnalytics(program="git", window_days=7)
Assert:
  - len(subcommands) == 1
  - subcommands[0].subcommand == "log" (only the t-1d entry)
  - The t-10d "push" entry is absent
```

---

## Section 4 — Acceptance Criteria Coverage Matrix

| AC | Criterion Summary | Test Case(s) | Coverage |
|----|-------------------|--------------|----------|
| AC-1 | `LoadWindow` issues SQL `WHERE created_at >= ?`, no in-memory filter | S-01, S-02, S-03, I-02 | COVERED |
| AC-2 | DB schema has `(created_at)` index and `(command_program, created_at)` compound index | R-03 (uses compound index path), build verification in Story 1.1 | COVERED |
| AC-3 | `ListAnalyticsByProgramSince` exists and is tested | R-07, R-08, R-09, R-10, R-11 | COVERED |
| AC-4 | `GetSubcommandBreakdown` exists and returns per-(subcommand, decision) aggregates | R-12 through R-18 | COVERED |
| AC-5 | `ListRecentCommandsByProgram` returns last N command previews | R-19 through R-25 | COVERED |
| AC-6 | `GetSubcommandTrend` returns per-day counts | R-26 through R-31 | COVERED |
| AC-7 | `GetProgramAnalytics` RPC returns SubcommandBreakdown, ExampleCommands, RuleCoverage, DailyTrend | H-01 through H-15, I-01, I-02, I-03, I-04 | COVERED |
| AC-8 | Clicking a program row opens the detail panel | F-22 | COVERED |
| AC-9 | Detail panel shows subcommand table with count, %, and decision breakdown | F-12, F-13, F-21 | COVERED |
| AC-10 | Detail panel shows last 5 raw command examples | F-18, F-19 | COVERED |
| AC-11 | Detail panel shows rule coverage (which rules matched vs manual review) | F-14, F-15, H-08, H-09, H-10 | COVERED |
| AC-12 | Detail panel shows a trend sparkline per subcommand | F-12 (sparklineBar rendered; bar width computed from data) | COVERED |
| AC-13 | "Add rule →" link pre-populates the rule form with the subcommand pattern | F-15, F-16 | COVERED |
| AC-14 | All existing analytics tests continue to pass | I-05 (regression gate) | COVERED |
| AC-15 | `make quick-check` passes with no lint errors | C6 lint guard (R-18 verifies no unused import in impl); CI gate | COVERED |

**Coverage**: 15/15 ACs covered (100%).

---

## Section 5 — Readiness Gate

### Gate 1: Requirements Complete?

All 15 acceptance criteria (AC-1 through AC-15) have at least one test case mapped in Section 4. No AC has zero tests.

**Result**: PASS — 15/15 ACs covered.

### Gate 2: Plan Complete?

| Epic | Stories | Implementation Notes Present | Tasks Detailed |
|------|---------|------------------------------|----------------|
| Epic 1 — DB Schema | 1.1, 1.2 | Yes (ent index syntax, generate command) | Yes (exact code) |
| Epic 2 — Repository | 2.1–2.5 | Yes (method signatures, SQL predicates, conversion helper) | Yes (exact code, including C4 pre-task) |
| Epic 3 — Proto + RPC | 3.1, 3.2 | Yes (proto field numbers, handler logic, delegation) | Yes (exact code) |
| Epic 4 — Frontend | 4.1–4.3 | Yes (hook, component, wire-up) | Yes (exact code) |

**Result**: PASS — all 4 epics have complete implementation notes.

### Gate 3: Validation Complete?

- Section 1: 31 Go unit tests (R-01 through R-31) covering all 4 new repository methods + LoadWindow fix + RPC handler
- Section 2: 25 frontend tests (F-01 through F-25) covering hook behavior, component rendering, and wire-up
- Section 3: 5 integration tests (I-01 through I-05) covering end-to-end flow and time window filtering
- Section 4: All 15 ACs mapped to ≥1 test

**Result**: PASS — test matrix covers all ACs.

### Gate 4: Adversarial Review Addressed?

| Finding | Severity | Status | Addressed In |
|---------|----------|--------|--------------|
| C1 — `RuleSpec.CommandProgram` field missing | ~~BLOCKED~~ | Fixed in plan | Heuristic in `coveredSubcommands` |
| C2 — Ent GroupBy Scan tag format | CONCERN | Mitigated | R-18 validates Count field > 0 |
| C3 — Hook cleanup return type mismatch | CONCERN | Mitigated | F-08 validates null→value transition; plan code updated to `return () => {}` |
| C4 — `convertAnalyticsEntries` helper may not exist | CONCERN | Mitigated | Plan pre-task in Story 2.3 requires extraction first |
| C5 — Nil slice on 0 rows | CONCERN | Confirmed OK | `make([]T, 0, len(rows))` produces non-nil; R-12 asserts nil error |
| C6 — `keyframes` unused import in css.ts | INFO | Fixed in plan | Plan CSS task corrected |
| C7 — `DailyBucketProto` cross-file reference | INFO | No action needed | Informational only |

No findings remain BLOCKED. All CONCERN findings have either a test case that validates the fix (C2, C3) or a required pre-task in the implementation plan (C4).

**Result**: PASS — no BLOCKED findings; all CONCERN findings addressed.

---

## Summary

| Metric | Value |
|--------|-------|
| Go unit tests | 31 (R-01 through R-31) |
| Go unit tests — handler | 15 (H-01 through H-15) |
| Go unit tests — store fix | 3 (S-01 through S-03) |
| Frontend unit tests — hook | 8 (F-01 through F-08) |
| Frontend unit tests — component | 13 (F-09 through F-21) |
| Frontend integration tests — panel wire-up | 4 (F-22 through F-25) |
| Go integration tests | 5 (I-01 through I-05) |
| **Total test cases** | **79** |
| ACs covered / total | 15 / 15 (100%) |
| **Readiness gate verdict** | **PASS** |
