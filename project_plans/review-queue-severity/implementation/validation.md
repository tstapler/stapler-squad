# Validation Plan: review-queue-severity

**Date**: 2026-08-06

## Happy Path Scenario

Given a session escalates `git push --force origin main`, which the classifier
(`pkg/classifier.RiskLevel`) scores `RiskCritical` via its seed rule table, when the
resulting `PendingApproval` is created and a user opens `ReviewQueuePanel` with no prior
sort/filter preference, then the item renders with a red "Critical" `SeverityBadge` and sorts
to the top of the queue by default — ahead of every Medium/Low/unrecorded item — so the
single most dangerous pending request is visible in one glance, zero clicks.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1 (AC1): `createApproval` sets `RiskLevel` from the classifier at creation time | `server/services/approval_handler_test.go` | `TestCreateApproval_should_SetRiskLevelFromClassifier_When_EscalationMatchesRiskCriticalRule` | Unit | Happy path — `git push --force` escalation → `PendingApproval.RiskLevel == "critical"` |
| REQ-1 (AC1) | `server/services/approval_handler_test.go` | `TestCreateApproval_should_SetEmptyRiskLevel_When_ClassifierIsNilAtCreation` | Unit | Error path — `h.classifier == nil` guard yields `RiskLevel == ""`, not `"low"` |
| REQ-1 (AC1) | `server/server_test.go` | `TestServerWiring_should_CallSetClassifierBeforeServingTraffic_When_ServerStarts` | Unit | Regression guard closing the only known zero-value-`escalation` path (Task 1.1.3) |
| REQ-1 (AC1, extends to 1.3) | `server/services/approval_store_test.go` | `TestApprovalStore_should_ReturnRiskLevelViaGetApprovalMetadataBySession_When_ApprovalCreatedWithClassifiedRisk` | Integration | Store round-trip: created approval → `ApprovalMetadata.RiskLevel` (foundation for Path B) |
| REQ-2 (AC2): `ListPendingApprovals` RPC includes severity | `server/services/approval_service_test.go` | `TestListPendingApprovals_should_IncludeRiskLevelInProto_When_ApprovalHasClassifiedRisk` | Unit | Happy path — `PendingApprovalProto.risk_level == "critical"` |
| REQ-2 (AC2) | `server/services/approval_service_test.go` | `TestListPendingApprovals_should_ReturnEmptyRiskLevel_When_ApprovalPredatesFeature` | Unit | Error/edge — legacy approval with no recorded `RiskLevel` serializes as `""`, not `"low"` |
| REQ-2 (AC2) | `server/services/approval_service_test.go` | `TestListPendingApprovals_should_RoundTripRiskLevelThroughStoreAndProto_When_MultipleApprovalsPending` | Integration | Handler → store → service → proto, multiple approvals at different risk levels |
| REQ-3 (AC3): `SeverityBadge` renders correctly | `web-app/src/components/sessions/__tests__/SeverityBadge.test.tsx` | `SeverityBadge_should_RenderCriticalBadgeWithIconAndLabel_When_RiskLevelIsCritical` | Unit | Happy path — icon + "Critical" text + `role="status"` + `aria-label="Critical risk"` |
| REQ-3 (AC3) | `web-app/src/components/sessions/__tests__/SeverityBadge.test.tsx` | `SeverityBadge_should_RenderNotRecordedState_When_RiskLevelIsEmptyString` | Unit | Error/edge — `""`/`undefined` never falls back to "Low" |
| REQ-3 (AC3, Path B enrichment) | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_should_SetRiskLevelMetadataKey_When_ApprovalMetadataHasClassifiedRisk` | Integration | Poller reads `ApprovalStore.GetApprovalMetadataBySession` → `ReviewItem.Metadata["risk_level"]` set |
| REQ-3 (AC3, Path B enrichment) | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_should_OmitRiskLevelMetadataKey_When_ApprovalMetadataRiskLevelEmpty` | Integration | Absent-key (not empty-string) signal preserved when `RiskLevel == ""` |
| REQ-3 (AC3, default sort) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `ReviewQueuePanel_should_SortItemsBySeverityDescending_When_DefaultSortFieldIsSeverity` | Unit | Happy path — Critical→High→Medium→Low render order, `created_at` tiebreaker |
| REQ-3 (AC3, sort stability) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `ReviewQueuePanel_should_FreezeSortOrder_When_NewItemArrivesDuringSnapshot` | Unit | Edge case — sort frozen at `reviewingIdsSnapshot` capture, not recomputed on every poll |
| REQ-3 (AC3, Path A badge) | `web-app/src/components/sessions/__tests__/ApprovalCard.test.tsx` | `ApprovalCard_should_RenderSeverityBadgeInHeader_When_ApprovalHasRiskLevel` | Unit | Happy path — badge appears next to tool name/countdown |
| REQ-3 (AC3, Path A sort) | `web-app/src/components/sessions/__tests__/ApprovalDrawer.test.tsx` | `ApprovalDrawer_should_SortBySeverityThenExpiry_When_MultipleApprovalsPending` | Unit | Happy path — B(Critical)→C(Medium)→A(Low), `secondsRemaining` tiebreaker |
| REQ-3 (AC3, Path A fail-safe) | `web-app/src/components/sessions/__tests__/ApprovalDrawer.test.tsx` | `ApprovalDrawer_should_RankUnrecordedSeverityAsHigh_When_RiskLevelIsEmpty` | Unit | Edge case — unclassified item sorts near top, never buried last |
| REQ-4 (AC4): filter by severity level | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `ReviewQueuePanel_should_ShowOnlyCriticalItems_When_CriticalSeverityChipClicked` | Unit | Happy path — single-chip filter narrows list, `aria-pressed="true"` |
| REQ-4 (AC4) | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `ReviewQueuePanel_should_ShowSharedEmptyState_When_SeverityFilterMatchesZeroItems` | Unit | Error/edge — reuses existing filter-miss empty state copy/component |
| REQ-4 (AC4) | N/A | — | Integration | Client-side predicate over already-fetched data; no store/RPC call to test in isolation — covered end-to-end by `tests/e2e/review-queue-severity.spec.ts` (see UX Acceptance Tests) |
| REQ-5 (AC5): analytics breakdown by severity | `server/services/analytics_store_test.go` | `TestComputeSummary_should_PopulateRiskLevelCounts_When_EntriesSpanAllFourRiskLevels` | Unit | Happy path — 40 decisions → exact `{"critical":5,"high":10,"medium":15,"low":10}` |
| REQ-5 (AC5) | `server/services/analytics_store_test.go` | `TestComputeSummary_should_ReturnEmptyRiskLevelCounts_When_NoEntriesInWindow` | Unit | Error/edge — zero-escalation window yields empty map, not nil-panic |
| REQ-5 (AC5) | `server/services/rules_service_test.go` | `TestGetApprovalAnalytics_should_ReturnRiskLevelCountsOnWire_When_ClassificationAnalyticsRecordedMultipleRiskLevels` | Integration | `ComputeSummary` → `summaryToProto` → `AnalyticsSummaryProto.risk_level_counts` over ent-backed SQLite data |
| REQ-5 (AC5, frontend) | `web-app/src/components/sessions/__tests__/ApprovalAnalyticsPanel.test.tsx` | `ApprovalAnalyticsPanel_should_RenderRiskLevelBreakdownTable_When_SummaryHasRiskLevelCounts` | Unit | Happy path — 4 rows (Critical/High/Medium/Low) with counts + scaled bar |
| REQ-5 (AC5, frontend) | `web-app/src/components/sessions/__tests__/ApprovalAnalyticsPanel.test.tsx` | `ApprovalAnalyticsPanel_should_RenderSharedEmptyState_When_AllRiskLevelCountsAreZero` | Unit | Error/edge — reuses "No escalations in this window." copy |
| REQ-6 (AC6): severity survives a server restart | `server/services/approval_store_test.go` | `TestApprovalStore_should_PreserveRiskLevelAcrossPersistAndReload_When_ApprovalIsOrphaned` | Unit | Happy path — persist→reload round-trip preserves `"high"` |
| REQ-6 (AC6) | `server/services/approval_store_test.go` | `TestApprovalStore_should_LoadEmptyRiskLevel_When_LegacyJSONHasNoRiskLevelKey` | Unit | Error/edge — pre-feature JSON with no `risk_level` key deserializes to `""`, not `"low"` |
| REQ-6 (AC6) | `server/services/approval_store_test.go` | `TestApprovalStore_should_ReturnPersistedRiskLevelFromListPendingApprovals_When_ServerRestartsWithOrphanedApprovals` | Integration | Disk load → store → service layer wire-out, full restart simulation |
| REQ-7 (AC7): existing approval flow unaffected (regression) | `server/services/approval_handler_test.go` | `TestApprovalHandler_should_AutoDenySecretScanRequest_When_PlaintextAWSKeyDetected` | Unit | Happy path — pre-existing secret-scan auto-deny test passes unmodified (early-return path never constructs `PendingApproval`) |
| REQ-7 (AC7) | `server/services/approval_handler_test.go` | `TestApprovalHandler_should_AutoAllowRequest_When_RuleMatchesAllowDecisionUnaffectedByRiskLevelField` | Unit | Error/edge — auto-allow path behavior unchanged by the new field |
| REQ-7 (AC7) | `server/services/approval_handler_test.go` | `TestApprovalHandler_should_ProduceUnchangedDecisionResponse_When_ApprovalFlowRunsEndToEndWithRiskLevelThreading` | Integration | Full approve/deny/expiry request path, same response bodies as pre-feature |
| REQ-8 (Epic 7 gap closure): `ApprovalRulesPanel` renders already-wired `riskLevel` | `web-app/src/components/sessions/__tests__/ApprovalRulesPanel.test.tsx` | `ApprovalRulesPanel_should_RenderRiskColumnWithSeverityBadge_When_RuleHasRiskLevel` | Unit | Happy path — new "Risk" column shows compact `SeverityBadge` |
| REQ-8 | `web-app/src/components/sessions/__tests__/ApprovalRulesPanel.test.tsx` | `ApprovalRulesPanel_should_RenderNotRecordedBadge_When_RuleRiskLevelIsEmpty` | Unit | Error/edge — rule predating the field renders "N/A", not blank/Low |
| REQ-8 | N/A | — | Integration | `riskLevel` already threaded through `upsertRule`/`ApprovalRuleProto` (pre-existing, unchanged); this story is display-only, no new store/RPC call |

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Triage speed — highest severity item in 1 glance, 0 clicks | `tests/e2e/review-queue-severity.spec.ts` | `should identify highest-severity item as the first row with no sort interaction` | Playwright | Seed queue via test API with items at all 4 severities + one unrecorded; load `ReviewQueuePanel` fresh; assert the first `data-testid="review-item"` row matches the Critical item without clicking Sort |
| 2. Filter to Critical in ≤2 clicks | `tests/e2e/review-queue-severity.spec.ts` | `should narrow to Critical items within two clicks` | Playwright | Click "Expand filters" (1), click the Critical severity chip (2); assert only Critical items render and `aria-pressed="true"` on the chip |
| 3. Clear a filter in 1 click | `tests/e2e/review-queue-severity.spec.ts` | `should restore the unfiltered queue with a single Clear click` | Playwright | Apply severity + another filter dimension; click "✕ Clear" once; assert full unfiltered item count returns |
| 4. See a rule's risk level in 0 clicks | `web-app/src/components/sessions/__tests__/ApprovalRulesPanel.test.tsx` | `ApprovalRulesPanel_should_ExposeRiskColumnWithoutAnyInteraction_When_TableRenders` | Jest + RTL | Render panel with rule fixtures; assert the Risk column badge is present in the initial render output, no expand/edit action triggered |
| 5. See severity trend in ≤1 click | `tests/e2e/review-queue-severity.spec.ts` | `should render risk breakdown after a single window selection` | Playwright | Open Analytics panel; select a 7-day window (1 click); assert "Risk Level Breakdown" table with counts renders with no further drill-down click |
| 6. "Not recorded" never confused with "Low" | `web-app/src/components/sessions/__tests__/SeverityBadge.test.tsx` | `SeverityBadge_should_RenderVisuallyDistinctFromLow_When_RiskLevelIsEmptyVsLow` | Jest + RTL | Render `riskLevel="low"` and `riskLevel=""` side by side; assert distinct icon glyph, distinct `aria-label` text, and distinct CSS variant class between the two |
| 7. Severity filter zero-match state | `tests/e2e/review-queue-severity.spec.ts` | `should show the shared zero-match empty state for a severity filter combo` | Playwright | Select a severity + reason filter combo yielding 0 items; assert exact copy "No items match the current filter." + "N items in queue" + a working "Clear filter" button |
| 8. Unclassified items surface above Medium (fail-safe sort) | `tests/e2e/review-queue-severity.spec.ts` | `should render unrecorded-severity item between Critical and Medium in default sort` | Playwright | Seed one Critical, one unrecorded (no `risk_level` key), one Medium item; load panel with default sort; assert row order via `data-testid` is Critical → unrecorded → Medium |
| 9. No dead ends | `tests/e2e/review-queue-severity.spec.ts` | `should always expose a single-click path back to the default view` | Playwright | From each of: severity-filtered view, combined-filter zero-match view, analytics empty-window view — assert one click (Clear filter / change window) returns to a non-empty default view |
| 10. Keyboard navigation | `tests/e2e/review-queue-severity.spec.ts` | `should operate every new severity control via Tab + Enter/Space alone` | Playwright | Tab to each severity chip, the Sort dropdown, and Clear/✕ Clear; activate via Enter/Space only (no mouse); assert focus outline visible and action fires |
| 11. Screen-reader labels | `tests/e2e/review-queue-severity.spec.ts` | `should expose role=status and full-word aria-label on every SeverityBadge variant` | Playwright | Query every rendered `SeverityBadge` (full + compact) for `role="status"` and a full-word `aria-label`; assert the compact abbreviation/icon nodes carry `aria-hidden="true"` |
| 12. Colour contrast ≥4.5:1 in all 6 themes | CI gate (no new spec file) | Existing Axe Core WCAG AA gate on PRs touching `web-app/src/` | Axe Core CI | Run the existing CI job against pages rendering `SeverityBadge` (queue, drawer, rules panel, analytics panel) in each of the 6 theme blocks; build fails on any AA violation. Cross-check against the inline contrast-ratio comments added to `theme.css.ts` (Task 4.2.2) |
| 13. Severity is never colour-only (WCAG 1.4.1) | `web-app/src/components/sessions/__tests__/SeverityBadge.test.tsx` | `SeverityBadge_should_PairIconAndTextWithColour_When_AnyOfFiveStatesRenders` | Jest + RTL | For each of the 5 states, assert both a shape-distinct icon and a text/abbreviation node are present alongside the colour class (not colour alone). Manual follow-up: view the panel with a greyscale filter and confirm all 5 states remain distinguishable |
| 14. No new colour tier collides with an existing one | Manual checklist (no automated spec) | "Critical vs. High token distinctness" review | Manual visual review + `theme.css.ts` diff | Screenshot `SeverityBadge` Critical and High variants side by side in each of the 6 themes; confirm `criticalBg`/`criticalText` are distinct hex values from `errorBg`/`errorText` by diffing the token definitions in `theme.css.ts` |

## Test Stack

- **Unit (Go)**: standard library `testing` + `testify` (assert/require), matching existing conventions in `server/services/*_test.go` and `session/*_test.go`.
- **Unit (TypeScript)**: Jest + React Testing Library, matching existing conventions in `web-app/src/components/sessions/__tests__/*.test.tsx`.
- **Integration (Go)**: same `testing`/`testify` stack, exercised against the real `ApprovalStore` (in-memory + temp-dir disk persistence) and the real ent-backed SQLite `ClassificationAnalytics` store — no mocks for these two data stores, per existing test conventions in this codebase.
- **E2E / UX**: Playwright, per `.claude/rules/e2e-test-conventions.md` — `data-testid`/ARIA locators only, no `waitForTimeout`, `// @feature review-queue-severity-sort-filter, approval-analytics-risk-breakdown` header annotation on `tests/e2e/review-queue-severity.spec.ts`.
- **Accessibility**: existing Axe Core CI gate (blocks on WCAG AA violations) for automated contrast checks; manual checklist for grayscale/colour-collision review, which Axe does not cover.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods touched by this plan (`ApprovalHandler.createApproval`, `ApprovalStore.GetApprovalMetadataBySession`/`persistToDiskLocked`/`loadFromDisk`, `ApprovalService.ListPendingApprovals`, `AnalyticsStore.ComputeSummary`, `ReviewQueuePoller` enrichment): happy path + error/edge path covered per the table above.
- Both data-store-backed surfaces (`PersistedApproval` disk JSON, `ClassificationAnalytics` SQLite via ent) have at least one integration test reading/writing the real store, not a mock.
- All 7 numbered Acceptance Criteria in `requirements.md` plus the Epic 7 rules-panel gap (REQ-8) have at least one Unit test; REQ-1/2/5/6/7 additionally have an Integration test as required by their data-store/RPC involvement.
- All 14 UX acceptance criteria in `design/ux.md` have a corresponding automated test (Playwright or Jest/RTL) or an explicit manual step (contrast/colour-collision review), per the UX Acceptance Tests table above.
- Regression safety (AC7): the existing secret-scan/auto-allow/auto-deny test cases in `approval_handler_test.go` must continue to pass **unmodified** — no assertion in those tests should need to change, since none of them construct or inspect a `PendingApproval`.

## Migration Test

**N/A** — per `implementation/plan.md`'s Migration Plan section, this feature has no schema/DB migration. All three persistence-adjacent surfaces touched (`PersistedApproval` disk JSON, `ClassificationAnalytics` SQLite, proto wire format) are additive and backward-compatible by construction; the "legacy data has no `risk_level` key" behavior is covered directly by `TestApprovalStore_should_LoadEmptyRiskLevel_When_LegacyJSONHasNoRiskLevelKey` and `TestListPendingApprovals_should_ReturnEmptyRiskLevel_When_ApprovalPredatesFeature` above, in lieu of a dedicated migration test.
