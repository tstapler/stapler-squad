# Validation Plan: insights-cost-intelligence

**Date**: 2026-09-03

## Happy Path Scenario

Given the Baseline in `requirements.md` — a dashboard operator with ~600 sessions in the `TokenStore` and no computed verdicts, only raw charts — when the operator loads `/insights`, then the `FindingsPanel` surfaces a ranked, dollar-impact-tagged `WasteFinding` (e.g. a cache-hit-floor breach) at the top of the page, and clicking its action navigates in one hop to that session's detail (via the modal today, via `/insights/session/[sessionId]` once Epic 1.4 lands), satisfying the "find the driver fast" success metric without the operator manually opening sessions one-by-one.

## Threshold Calibration (blocking — run before/during Epic 1.1 implementation)

Pre-mortem Failure #2: `plan.md`'s four detector thresholds are fiat numbers, never checked
against the operator's real ~600-session local JSONL corpus (per `requirements.md`'s
Baseline). They either fire on nearly every session (alert fatigue, panel ignored) or
almost never (feature perceived as broken) if left untuned. This step must run — and its
findings applied to `session/tokens/findings.go` — before Story 1.1.6's `FindingsPanel` UI
ships, not treated as a "tune later" footnote.

**Exact hardcoded values to calibrate** (as currently written in `plan.md`):
- `cacheHitFloor = 0.40` — Task 1.1.2b, `detectCacheHitFloorBreach`.
- `sessionTokenCeiling = 2_000_000` — Task 1.1.2c, `detectSessionTokenCeiling`.
- `oversizedContextFloor = 30_000` — Task 1.1.3b, `detectOversizedStartContext`.
- Task 1.1.3a's `detectModelSwitchCacheBust` has no numeric constant to tune — it fires
  structurally on any priced model-switch immediately followed by a cache-bust turn. It's
  still included in the calibration run below to record its firing rate: a detector that
  fires on nearly every session is exactly as much a "feature perceived as noisy/broken"
  risk as a mistuned numeric threshold, per Failure #2.

**Calibration procedure:**
1. After Tasks 1.1.2b/1.1.2c/1.1.3a/1.1.3b land (detectors implemented and unit-tested) but
   before Story 1.1.6 (FindingsPanel UI) starts, write a throwaway harness — a `go run` one-off
   under `session/tokens/`, or a local `_test.go` with a `t.Skip()` removed — that loads the
   operator's real local JSONL corpus (`~/.stapler-squad/`, ~600 sessions per
   `requirements.md`'s Baseline) and runs `tokens.ComputeFindings(r, pt)` over every session's
   `ParseResult`.
2. Record, per detector: how many of the ~600 sessions fire, and the distribution of
   `DollarImpact` values among the sessions that do.
3. Flag any detector firing on more than ~50% of sessions (alert fatigue — the panel becomes
   noise) or fewer than ~2% (feature reads as broken/inert) as needing its constant adjusted.
4. Adjust `cacheHitFloor` / `sessionTokenCeiling` / `oversizedContextFloor` in
   `session/tokens/findings.go` based on the recorded distribution; record the final chosen
   values and the corpus-based rationale in the doc comment above each constant (the
   provenance comment Tasks 1.1.2b/1.1.2c/1.1.3b already require).
5. Re-run `findings_test.go`'s boundary-condition fixtures (just-under/at/over threshold)
   after any constant change to confirm they still target the new values.
6. Delete or gitignore the one-off calibration harness itself — it reads a local,
   machine-specific corpus path and is not meant to run in CI.

This step gates Story 1.1.6: do not ship the findings panel against uncalibrated,
guessed thresholds.

## Requirement -> Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Epic 1.1 (Story 1.1.2): cache-hit-floor-breach detector | `session/tokens/findings_test.go` | `TestDetectCacheHitFloorBreach_When9PercentHitRateOver6Turns_ExpectCriticalFindingWithDollarImpact` | Unit | Happy path |
| Epic 1.1 (Story 1.1.2): cache-hit-floor-breach detector, unpriced model | `session/tokens/findings_test.go` | `TestDetectCacheHitFloorBreach_WhenModelUnpriced_ExpectNilNotZeroImpactFinding` | Unit | Error/abstain path |
| Epic 1.1 (Story 1.1.5): findings wired into `GetInsightsSummary`, capped+sorted | `server/services/insights_service_test.go` | `TestGetInsightsSummary_When25SessionsEachProduceOneFinding_ExpectTop20SortedByDollarImpactDesc` | Integration | Happy path (exercises real `TokenStore`-backed service) |
| Epic 1.1 (Story 1.1.3): `ComputeFindings` panic isolation | `session/tokens/findings_test.go` | `TestComputeFindings_WhenOneOfFiveSessionsDetectorPanics_ExpectOtherFourFindingsStillReturned` | Unit | Error path |
| Epic 1.1 (Story 1.1.4): `ComputeWasteScore` nil-vs-zero | `session/tokens/findings_test.go` | `TestComputeWasteScore_WhenSessionHasFewerThan5Turns_ExpectNilNotZero` | Unit | Error/edge path |
| Epic 1.1 (Story 1.1.6): `FindingsPanel` three states | `web-app/src/app/insights/FindingsPanel.test.tsx` | `FindingsPanel_should_showComputedEmptyText_when_findingsArrayIsEmpty` | Unit (Jest/RTL) | Happy path |
| Epic 1.1 (Story 1.1.6): `FindingsPanel` error state | `web-app/src/app/insights/FindingsPanel.test.tsx` | `FindingsPanel_should_showErrorBoxWithRetry_when_parentErrorStateIsSet` | Unit (Jest/RTL) | Error path |
| Epic 1.2 (Story 1.2.1): `AttributeToolCosts` tool-type-level sum (ADR-001) | `session/tokens/pricing_test.go` | `TestAttributeToolCosts_WhenMultiToolTurn_ExpectCostAddedOncePerDistinctToolAndDoubleCountedFlagSet` | Unit | Happy path |
| Epic 1.2 (Story 1.2.1): `AttributeToolCosts` unpriced turn | `session/tokens/pricing_test.go` | `TestAttributeToolCosts_WhenTurnModelUnpriced_ExpectTurnSkippedContributesZero` | Unit | Error/abstain path |
| Epic 1.2 (Story 1.2.4): `ActivityCostBreakdown` aggregation | `server/services/insights_service_test.go` | `TestGetInsightsSummary_When3SessionsAcross2ActivityTypes_ExpectActivityBreakdownAggregatedCorrectly` | Integration | Happy path |
| Epic 1.2 (Story 1.2.3): `ClassifyActivity` skill-signal priority | `session/tokens/activity_test.go` | `TestClassifyActivity_WhenSkillNameSignalPresent_ExpectSkillSignalOutranksToolRatio` | Unit | Happy path |
| Epic 1.2 (Story 1.2.3): `ClassifyActivity` zero-tool-call edge case | `session/tokens/activity_test.go` | `TestClassifyActivity_WhenNoToolUsageRecorded_ExpectActivityOther` | Unit | Error/edge path |
| Epic 1.3 (Story 1.3.1): `ComputeCacheROI` abstain on unpriced | `session/tokens/pricing_test.go` | `TestComputeCacheROI_WhenModelUnpriced_ExpectOkFalseNotZero` | Unit | Error/abstain path |
| Epic 1.3 (Story 1.3.1): `ComputeCacheROI` negative ROI | `session/tokens/pricing_test.go` | `TestComputeCacheROI_WhenCacheWriteNeverReadBack_ExpectNegativeROI` | Unit | Happy path (negative is a valid, expected outcome) |
| Epic 1.3 (Story 1.3.2/1.3.3): sessions-table comparators, guarded division | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_sortZeroMessageSessionLast_when_sortedByCostPerMessageEitherDirection` | Unit (Jest/RTL) | Error/edge path |
| Epic 1.3 (Story 1.3.4): search/sort coexistence | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_preserve3SearchMatchedRows_when_wasteScoreHeaderClickedAfterSearch` | Integration (component-level, exercises the real `useMemo` filter+sort pipeline over a 600-row fixture) | Happy path |
| Epic 1.4 (Story 1.4.1): `conversation_id_filter` orphan lookup | `server/services/insights_service_test.go` | `TestGetInsightsSummary_WhenConversationIdFilterMatchesOrphanSession_ExpectExactlyThatSessionReturned` | Integration | Happy path |
| Epic 1.4 (Story 1.4.1): regression — `session_id_filter` alone still works | `server/services/insights_service_test.go` | `TestGetInsightsSummary_WhenSessionIdFilterSetAndNoConversationIdFilter_ExpectOnlyMatchingNonOrphanSessionReturned` | Unit | Error path (regression guard) |
| Epic 1.4 (Story 1.4.3): route cold-navigation fetch | `web-app/src/app/insights/session/[sessionId]/SessionDetailPageClient.test.tsx` | `SessionDetailPageClient_should_fetchByBothFiltersAndRenderContent_when_mountedWithNoParentState` | Unit (Jest/RTL) | Happy path |
| Epic 1.4 (Story 1.4.3): route not-found state | `web-app/src/app/insights/session/[sessionId]/SessionDetailPageClient.test.tsx` | `SessionDetailPageClient_should_showSessionNotFoundMessage_when_sessionIdMatchesNoSession` | Unit (Jest/RTL) | Error path |
| Epic 1.5 (Story 1.5.1): `TokenStore.Subscribe` channel payload | `session/tokens/store_test.go` | `TestTokenStoreSubscribe_WhenSingleFileReparsed_ExpectSubscriberReceivesThatFilesParseResult` | Unit | Happy path |
| Epic 1.5 (Story 1.5.1): full-walk-complete sends nil | `session/tokens/store_test.go` | `TestTokenStoreSubscribe_WhenInitialWalkCompletes_ExpectSubscriberReceivesNil` | Unit | Edge path (distinct from error, but the "no payload" branch) |
| Epic 1.5 (Story 1.5.3): `watchInsights` populates `InsightsEvent.Session` | `server/services/insights_service_test.go` | `TestWatchInsights_WhenChannelReceivesNonNilParseResult_ExpectUpdateEventWithPopulatedSession` | Integration | Happy path |
| Epic 1.5 (Story 1.5.3): `watchInsights` nil receive sends real `parse_complete` | `server/services/insights_service_test.go` | `TestWatchInsights_WhenChannelReceivesNil_ExpectParseCompleteEventNotBareUpdate` | Integration | Error/regression path (today's bug: indistinguishable `"update"`) |
| Epic 1.5 (Story 1.5.2): `buildSessionSummary` extraction parity | `server/services/insights_service_test.go` | `TestBuildSessionSummary_WhenCalledDirectly_ExpectProtoEqualToHandBuiltExpectedSummary` | Unit | Happy path (refactor-safety net) |
| Epic 1.5 (Story 1.5.4): frontend per-session live-patch fires | `web-app/src/lib/hooks/useInsightsService.test.ts` | `useInsightsSummary_should_patchSessionInPlaceWithNoRefetch_when_updateEventWithPopulatedSessionReceived` | Unit (Jest/RTL) | Happy path (was previously unreachable/dead code) |

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| User identifies the single most expensive finding and acts on it in 2 clicks (ux.md B1) | `tests/e2e/insights-findings-panel.spec.ts` | `insights-findings-panel_should_navigateToSession_when_viewSessionActionActivatedAfterPageLoad` | Playwright | Load `/insights` → assert top finding card visible with no additional click → click its "View session" action → assert session detail (modal or route) opens for that `sessionId` |
| Loading / computed-empty / error states are visually and textually distinct (ux.md B1) | `tests/e2e/insights-findings-panel.spec.ts` | `insights-findings-panel_should_renderDistinctText_when_loadingVsEmptyVsErrorStates` | Playwright | Mock `GetInsightsSummary` to return (a) a pending promise, (b) `findings: []`, (c) an error; assert three distinct rendered strings via `getByTestId('findings-panel')` |
| Every finding card conveys severity via text, not color alone (ux.md B1) | `tests/e2e/insights-findings-panel.spec.ts` | `insights-findings-panel_should_renderSeverityAsText_when_findingCardDisplayed` | Playwright | Seed a `CRITICAL` finding → assert `getByText('Critical')` visible on the card, independent of computed CSS |
| Finding card's primary action is keyboard-operable (ux.md B1) | `tests/e2e/insights-findings-panel.spec.ts` | `insights-findings-panel_should_triggerNavigation_when_actionFocusedAndEnterPressed` | Playwright | Tab to the finding card's action element → press `Enter` → assert same navigation as a click (no `waitForTimeout`; wait on the resulting URL/dialog state) |
| Error state offers a `[Retry]` action, never a dead end (ux.md B1) | `tests/e2e/insights-findings-panel.spec.ts` | `insights-findings-panel_should_refetchFindings_when_retryButtonClickedAfterError` | Playwright | Mock first `GetInsightsSummary` call to fail, second to succeed → click `getByRole('button', { name: 'Retry' })` → assert findings render |
| Every new sortable column exposes correct `aria-sort` on click (ux.md B2) | `tests/e2e/insights-sessions-table-sort.spec.ts` | `insights-sessions-table-sort_should_setAriaSortDescending_when_wasteScoreHeaderClickedFirstTime` | Playwright | Load `/insights` with seeded sessions → click `getByRole('columnheader', { name: /Waste Score/i })` → assert `aria-sort="descending"` |
| Negative Cache ROI is visually distinguishable from "no ROI" via text, not color (ux.md B2) | `tests/e2e/insights-sessions-table-sort.spec.ts` | `insights-sessions-table-sort_should_renderSignedTextVsUnpricedBadge_when_negativeRoiAndUnpricedRowsBothVisible` | Playwright | Seed one session with negative `cacheRoiUsd` and one unpriced → assert `getByText('-$0.42')` and the unpriced badge both render, with distinct text |
| Sort and search compose without resetting each other (ux.md B2) | `tests/e2e/insights-sessions-table-sort.spec.ts` | `insights-sessions-table-sort_should_keepSameSearchMatchedRows_when_columnHeaderClickedAfterTyping` | Playwright | Type a search term matching a known subset → click "Waste Score" header → assert the same row set stays visible, now reordered |
| User locates the single worst-waste-score session in 2 interactions (ux.md B2) | `tests/e2e/insights-sessions-table-sort.spec.ts` | `insights-sessions-table-sort_should_surfaceWorstSessionFirst_when_wasteScoreHeaderClickedOnce` | Playwright | Click "Waste Score" header once → assert the first data row matches the seeded highest-waste-score session |
| No dead ends: every route state has a reachable "Back to dashboard" (ux.md B3) | `tests/e2e/insights-session-route.spec.ts` | `insights-session-route_should_showBackToDashboardLink_when_sessionNotFoundOrFetchErrors` | Playwright | Navigate directly to `/insights/session/does-not-exist` → assert `getByRole('link', { name: /Back to dashboard/i })` visible; repeat with a mocked fetch error |
| Cold direct navigation renders correctly with no parent dashboard state (ux.md B3) | `tests/e2e/insights-session-route.spec.ts` | `insights-session-route_should_renderSessionDetailContent_when_navigatedDirectlyWithNoPriorClientNavigation` | Playwright | Open a fresh Playwright `page.goto()` directly to `/insights/session/{seededId}` (no click-through) → assert `SessionDetailContent`'s metadata section renders |
| Modal focus management: open moves focus to close button, close restores it (ux.md B3) | `tests/e2e/insights-session-route.spec.ts` | `insights-session-route_should_moveFocusToCloseButtonThenRestoreToTriggerRow_when_modalOpenedAndClosed` | Playwright | Click a session row → assert `document.activeElement` (via `page.evaluate`) is the close button → press `Escape` → assert focus returns to the row element |
| Route mount moves focus to the heading (ux.md B3) | `tests/e2e/insights-session-route.spec.ts` | `insights-session-route_should_moveFocusToHeading_when_routeMounts` | Playwright | Navigate to the route → assert the heading element is `document.activeElement` |
| "Open full page" link never resolves to an empty path segment, including for orphans (ux.md B3) | `tests/e2e/insights-session-route.spec.ts` | `insights-session-route_should_resolveHrefUsingConversationId_when_orphanSessionOpenFullPageClicked` | Playwright | Seed an orphan session (`sessionId=""`, `conversationId="conv-999"`) → open its modal → assert the "Open full page" link's `href` is `/insights/session/conv-999` |
| Focus-trap parity: Tab from the modal's last focusable element cycles back to the first (ux.md B3) | `tests/e2e/insights-session-route.spec.ts` | `insights-session-route_should_cycleFocusBackToFirstElement_when_tabbedFromLastFocusableInModal` | Playwright | Open modal → Tab through all focusable elements → assert the next Tab returns focus to the first one, not page content behind it |
| Per-tool cost estimated-value marker shown only when `costMayDoubleCount` (ux.md A1) | `tests/e2e/insights-session-route.spec.ts` | `insights-session-route_should_showEstimatedMarkerOnlyOnDoubleCountedTool_when_toolsBreakdownTableRenders` | Playwright | Open a session detail view with one double-counted and one non-double-counted tool row → assert `~$` marker present on one, absent on the other |
| Activity breakdown table: every row carries the estimated marker (ux.md A2) | `tests/e2e/insights.spec.ts` | `insights-dashboard_should_showEstimatedMarkerOnEveryRow_when_activityBreakdownTableRenders` | Playwright | Load `/insights` with seeded activity data → assert every row in `getByTestId('activity-breakdown-table')` shows the `~` marker |
| Findings panel does not recompute/re-render on every live-patch tick (ux.md A4) | `tests/e2e/insights.spec.ts` | `insights-dashboard_should_notRerenderFindingsPanel_when_watchInsightsUpdateEventArrives` | Playwright | Load dashboard → capture findings panel's rendered snapshot → trigger a mocked `WatchInsights` `update` event → assert findings panel markup is unchanged while `SessionsTable`/`SummaryCards` patch |

Page helpers: extend `tests/e2e/pages/SessionDetailPage.ts` with methods for the new "Open full page" link and focus assertions (reuse the existing session-detail page object rather than forking one); add a new `tests/e2e/pages/InsightsPage.ts` for findings-panel and sessions-table-sort interactions used across `insights-findings-panel.spec.ts`, `insights-sessions-table-sort.spec.ts`, and `insights.spec.ts`, per `e2e-test-conventions`'s "new page helpers go in `tests/e2e/pages/`" rule — do not inline repeated navigation/query logic across the three new spec files.

## Test Stack

- **Unit (Go)**: standard `testing` package, table-driven fixtures over synthetic `tokens.ParseResult`/`tokens.PricingTable` values — the fixture-based approach `requirements.md`'s Feasibility Risks flagged as not yet established in this codebase, now set by `findings_test.go`/`activity_test.go`. No new assertion library; matches existing `pricing_test.go` style (plain `if got != want { t.Errorf(...) }` / `reflect.DeepEqual` where needed).
- **Unit (TypeScript)**: Jest + React Testing Library (`@testing-library/react`), matching existing `SessionsTable.test.tsx`/`SessionDetailDrawer.test.tsx` conventions — `render`, `screen.getByRole`/`getByTestId`, `fireEvent`/`userEvent` for keyboard interaction assertions.
- **Integration (Go)**: `server/services/insights_service_test.go`'s existing pattern — a real `insightsService` wired to a fake/in-memory `TokenStoreReader` (`fakeTokenStore`) and `PricingTable`, exercising the actual ConnectRPC handler logic (`GetInsightsSummary`, `watchInsights`) end-to-end rather than mocking the service itself. This is the project's substitute for a DB integration test — there is no schema/DB in scope (Step 5).
- **E2E / UX**: Playwright, per `tests/e2e/`'s existing conventions (`e2e-test-conventions` skill) — `// @feature` header, no `waitForTimeout`, `data-testid`/ARIA-role locators only, new page helpers under `tests/e2e/pages/`.

## Test Stack Additions Needed

- `session/tokens/findings_test.go`, `session/tokens/activity_test.go` are new files (no prior fixture-based "heuristic produces the right verdict" pattern existed — `requirements.md`'s Feasibility Risks called this out explicitly; this plan establishes it).
- `web-app/src/lib/hooks/useInsightsService.test.ts` is a new file (Story 1.5.4 notes no test file exists yet for this hook).
- Three new Playwright spec files: `tests/e2e/insights-findings-panel.spec.ts`, `tests/e2e/insights-sessions-table-sort.spec.ts`, `tests/e2e/insights-session-route.spec.ts` — kept separate from the existing `tests/e2e/insights.spec.ts` (dashboard-shell smoke tests) rather than growing one file past readability, mirroring this repo's existing one-spec-per-surface convention (e.g. `backlog-board-live-updates.spec.ts` vs. `backlog.spec.ts`).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | >=80% line, with `session/tokens/findings.go`, `session/tokens/activity.go`, and the new `pricing.go` functions (`EstimateTurnCost`, `AttributeToolCosts`, `ComputeCacheROI`) at 100% branch coverage on their abstain-vs-fire guard conditions specifically (per the Rabbit Holes "abstain rather than guess" discipline — a missed guard branch is exactly how a `$0.00` misleading finding would slip through) |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | >=80% line |

- All public service methods (`ComputeFindings`, `ComputeWasteScore`, `AttributeToolCosts`, `ComputeCacheROI`, `ClassifyActivity`, `buildSessionSummary`): happy path + error/abstain paths covered.
- The one external-call-shaped boundary in scope, `TokenStore.Subscribe`'s channel contract (Epic 1.5), is covered by both a unit test (`store_test.go`) and an integration test (`insights_service_test.go`'s `watchInsights` tests) — the closest analog to "external integration" this plan has, since there is no real external service call (per Constraints: all data is local JSONL).
- UX acceptance criteria: every criterion listed in `design/ux.md`'s "UX acceptance criteria" subsections (B1, B2, B3) and the Part A condensed-surface acceptance-criteria bullets (A1, A2, A4) has a corresponding row in the UX Acceptance Tests table above.

## Migration Test

N/A — no schema/DB migration exists in this plan (proto field additions only, regenerated via `make proto-gen`, generated code not committed per this repo's `.gitignore` policy). Skipped per Step 5.
