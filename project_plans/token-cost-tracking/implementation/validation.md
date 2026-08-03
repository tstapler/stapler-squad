# Validation Plan: token-cost-tracking

**Date**: 2026-08-02

## Happy Path Scenario
Given a stapler-squad user with an active session whose Claude Code JSONL transcript has
already been parsed into the in-memory `TokenStore`, when they open that session's detail
drawer and separately sort the main session list and the `/insights` sessions table by cost,
then they see a per-turn token breakdown table (AC-1), a cost-ordered session list with
unpriced/not-yet-loaded sessions always trailing (AC-2), a click-to-sort `SessionsTable`
(AC-3), and an aggregate cache-hit-rate label on the model breakdown chart (AC-6) — all
derived from data the parser/pricing table already produce, with no new storage writes.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC-1 | `server/services/insights_service_test.go` | `TestGetSessionTurnTimeline_should_returnTurns_When_ConversationIdMatches` | Unit | Happy path — known `conversation_id` returns populated `TurnTokenStat` list |
| AC-1 | `server/services/insights_service_test.go` | `TestGetSessionTurnTimeline_should_returnEmptyTurns_When_ConversationIdUnknown` | Unit (error/edge) | Unmatched UUID returns empty `Turns`, not an error |
| AC-1 | `server/services/insights_service_test.go` | `TestGetSessionTurnTimeline_should_returnTurns_When_backedByRealTokenStore` | Integration | RPC handler backed by a real `tokens.TokenStore` loaded from `session/tokens/testdata/valid_session.jsonl` (not a fake), confirming end-to-end wiring from parser → `TurnTimeline` → wire message |
| AC-1 | `web-app/src/app/insights/turnTimelineUtils.test.ts` | `sortTurnsByTokensDesc_should_orderTurnsByTotalTokensDescending_When_turnsVarySize` | Unit | Mixed-size turn list sorts by `input+output` descending |
| AC-1 | `web-app/src/app/insights/turnTimelineUtils.test.ts` | `isOutlierTurn_should_returnFalseAndNotThrow_When_turnsArrayEmpty` | Unit (error/edge) | Empty-array edge case for both `computeOutlierThreshold` and `isOutlierTurn` |
| AC-1 | `web-app/src/app/insights/turnTimelineUtils.test.ts` | `isOutlierTurn_should_returnFalse_When_totalEqualsTwiceMeanThresholdExactly` | Unit (error/edge) | Boundary case: total tokens exactly at (not above) 2× mean must not flag |
| AC-1 | `web-app/src/app/insights/SessionDetailDrawer.test.tsx` | `SessionDetailDrawer_should_fetchTurnTimelineOnce_When_drawerOpensForSession` | Integration | RTL render with a stubbed ConnectRPC transport for `GetSessionTurnTimeline`, asserting the lazy on-open fetch fires exactly once and renders the table |
| AC-2 | `web-app/src/components/sessions/__tests__/sessionCostSort.test.ts` | `compareSessionsByCost_should_sortDescendingByCost_When_sortDirDesc` | Unit | Happy path — higher-cost session sorts first on `desc` |
| AC-2 | `web-app/src/components/sessions/__tests__/sessionCostSort.test.ts` | `compareSessionsByCost_should_sortMissingCostLast_When_sortDirAscOrDesc` | Unit (error/edge) | The specific bug class Pattern Decisions rules out: a session missing from `costById` sorts last in **both** `asc` and `desc` (early-return-before-flip); two missing-cost sessions compare equal |
| AC-2 | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `SessionList_should_populateCostByIdAndSortByCost_When_insightsSummaryResolves` | Integration | RTL render with a mocked `useInsightsSummary`/ConnectRPC transport verifying the `session_id`-keyed join populates `costById` and `"Sort: Cost"` reorders the rendered list |
| AC-3 | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_supportClickToSort_When_headerClicked > sorts by cost descending on first click` | Unit | Happy path — first click on "Cost" header sorts descending |
| AC-3 | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_supportClickToSort_When_headerClicked > toggles to ascending on second click of the same header` | Unit | State-machine transition — second click flips direction |
| AC-3 | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_supportClickToSort_When_headerClicked > sorts unpriced sessions last for cost regardless of direction` | Unit (error/edge) | Unpriced session stays last in both sort directions (mirrors AC-2's comparator bug guard) |
| AC-3 | N/A | N/A | Integration | Not applicable — `SessionsTable` sorts already-fetched `SessionTokenSummary[]` props client-side; no new RPC/store call is introduced by this AC (confirmed in plan's Pattern Decisions, AC-3 row) |
| AC-4 | `server/services/insights_service_test.go` | `TestWatchInsights_should_forwardUpdateEvent_When_TokenStoreNotifies` | Integration | Happy path — real `tokens.TokenStore` (not `fakeTokenStore`, whose `Subscribe()` is a dead end) drives one full initial-event + update-event streaming cycle through the extracted `insightsEventSender` seam |
| AC-4 | `server/services/insights_service_test.go` | `TestWatchInsights_should_unsubscribeAndReturn_When_ContextIsCanceled` | Integration (error/edge) | Context cancellation causes `watchInsights` to unsubscribe and return `nil` within a bounded wait, without a real network round-trip |
| AC-4 | `server/services/insights_service_test.go` | `insightsEventSender_should_beSatisfiedByFakeSender_When_compiled` | Unit | Compile-time assertion `var _ insightsEventSender = (*fakeInsightsEventSender)(nil)` — the narrow interface seam itself is satisfied by the test double, independent of any store/notify behavior |
| AC-5 | `docs/registry/frontend-features.json` (script-driven, no new test file) | `registryAggregate_should_includeAllFiveNewFrontendIds_When_registryGenerateRuns` | Verification | Run `make registry-generate` once; grep `docs/registry/frontend-features.json` for all 5 new `insights-*` ids (proves `registry-aggregate` actually folded the per-feature JSON files in — corrected during `/sdd:4-validate`'s pre-mortem pass, since `coverage-gaps.json` never reads that directory and its `unmatchedFrontend` count is provably unchanged regardless of these files' content; see plan.md's corrected Epic 1.2/5.1 and pre-mortem.md finding #3) |
| AC-5 | `tools/scanner/backend/cmd/main.go` (existing scanner, verified by reading source — no new test authored) | `registry-generate_should_preserveHandEditedTestIds_When_rerunWithNonEmptyTestIds` | Integration | Confirms the `len(existingIDs) > 0` guard means Task 1.1.3a's hand-edited `WatchInsights.json` (`tested: true`, non-empty `testIds`) survives a subsequent `registry-generate-backend` run unmodified |
| AC-6 | `web-app/src/app/insights/ModelBreakdownChart.test.tsx` | `ModelBreakdownChart_should_showCacheHitRate_When_modelHasCacheReads` | Unit | Happy path — nonzero `cacheReadTokens`/`totalInputTokens` fixture renders the computed percentage in the legend |
| AC-6 | `web-app/src/app/insights/ModelBreakdownChart.test.tsx` | `ModelBreakdownChart_should_showZeroPercentCacheHit_When_noCacheEligibleTokens` | Unit (error/edge) | Divide-by-zero guard renders `"0.0% cache hit"`, never `NaN%`/`Infinity%`/blank |
| AC-6 | N/A | N/A | Integration | Not applicable — cache hit rate is derived client-side from already-fetched `ModelBreakdown` props; no new RPC/store call (Pattern Decisions, AC-6 row) |
| AC-7 | `Makefile` target (no new test file) | `make_quickCheck_should_passWithZeroFailures_When_runAfterAllChanges` | Verification | Happy path gate — `make quick-check` (build + test + lint) passes after every other AC's changes land |
| AC-7 | `web-app` (jest, no new test file) | `jest_should_passAllNewFrontendTests_When_runInIsolation` | Verification (error/edge) | `cd web-app && npx jest --no-coverage` confirms every new frontend test (`turnTimelineUtils.test.ts`, `sessionCostSort.test.ts`, `SessionsTable.test.tsx`/`ModelBreakdownChart.test.tsx` additions) passes standalone, not only inside `quick-check`'s aggregate run |
| AC-7 | `Makefile` target (no new test file) | `make_ci_should_passAsDefinitivePrePushGate_When_runBeforeShip` | Integration | Full `make ci` pipeline — the final, definitive pre-push check — passes before the project is considered ready to ship |

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UX-AC-a1 | `tests/e2e/sessions-table-sort.spec.ts` | `SessionsTable_should_sortByCostInOneClickAndToggleInOne_When_costHeaderClicked` | Playwright | Navigate to `/insights`; click `getByRole('columnheader', {name: 'Cost'})`; assert row order is descending by cost; click again; assert ascending |
| UX-AC-a2 | `tests/e2e/sessions-table-sort.spec.ts` | `SessionsTable_should_beKeyboardOperable_When_headerFocusedAndActivated` | Playwright | Tab through the table until each of Input/Output/Cache/Cost headers receives focus; press Enter, then Space on a separate run; assert sort applies each time |
| UX-AC-a3 | `tests/e2e/sessions-table-sort.spec.ts` | `SessionsTable_should_exposeCorrectAriaSort_When_headerStateChanges` | Playwright | For each of the 4 sortable `<th>`, read `aria-sort` at unsorted/`asc`/`desc` states and assert `"none"`/`"ascending"`/`"descending"` respectively |
| UX-AC-a4 | `tests/e2e/sessions-table-sort.spec.ts` | `SessionsTable_should_excludeNonSortableHeadersFromTabOrder_When_tabbingThroughHeaders` | Playwright | Assert Session/Model/Path `<th>` have no `aria-sort`/`tabIndex`/click handler and are skipped in sequential Tab traversal |
| UX-AC-a5 | `tests/e2e/sessions-table-sort.spec.ts` | `SessionsTable_should_pinUnpricedSessionLast_When_sortedByCostInBothDirections` | Playwright | Seed fixture with 1 unpriced + 2 priced sessions; sort Cost desc then asc; assert unpriced row is last in both renders |
| UX-AC-a6 | `tests/e2e/sessions-table-sort.spec.ts` | Manual: "Unsorted table order matches pre-change baseline" | Manual checklist | Before/after screenshot comparison of an unmodified fixture's row order with no header clicked — no existing visual-regression harness to automate this against |
| UX-AC-a7 | Manual checklist | "Sort indicator glyph is legible and unclipped at default font size" | Manual checklist | Visually inspect `↕`/`↑`/`↓` glyphs in each of the 4 sortable headers at the shipped font size; confirm no clipping/overlap with header text |
| UX-AC-b1 | `tests/e2e/insights-model-breakdown.spec.ts` | `ModelBreakdownChart_should_showCacheHitRatePerLegendEntry_When_pageLoads` | Playwright | Navigate to `/insights`; assert every model-family legend entry's text includes a `"% cache hit"` string with no click/hover required |
| UX-AC-b2 | `tests/e2e/insights-model-breakdown.spec.ts` | `ModelBreakdownChart_should_showZeroPercent_When_familyHasNoCacheEligibleTokens` | Playwright | Seed a model family fixture with zero input+cache-read tokens; assert legend text is `"0.0% cache hit"`, never `NaN%`/`Infinity%`/blank |
| UX-AC-b3 | Manual checklist | "Pricing-unavailable and cache-hit-rate labels coexist without collision" | Manual checklist | Resize viewport to desktop and to the narrowest supported width; visually confirm both labels render on one legible line with no overlap (CI's Axe/Lighthouse gate on `web-app/src/` covers WCAG separately) |
| UX-AC-c1 | Manual checklist | "Drawer shows Per-Turn Breakdown without visible loading flicker" | Manual checklist | Open the drawer for a representative session with turn data; time subjectively; confirm no skeleton flash given the in-memory `GetByUUID` lookup's expected sub-100ms latency |
| UX-AC-c2 | `tests/e2e/session-detail-drawer.spec.ts` | `SessionDetailDrawer_should_showExactEmptyStateCopy_When_sessionHasNoTurnData` | Playwright | Open drawer for a session/orphan with no turn data; assert exact text `"No per-turn data available for this session."` and that it shares the `emptyState` class with the existing "No tools recorded..." message |
| UX-AC-c3 | `tests/e2e/session-detail-drawer.spec.ts` | `SessionDetailDrawer_should_renderHighestTokenTurnFirst_When_turnsVarySize` | Playwright | Seed fixture with turns of varying size; assert the row with the largest `input+output` total renders first |
| UX-AC-c4 | `tests/e2e/session-detail-drawer.spec.ts` | `SessionDetailDrawer_should_visuallyDistinguishOutlierCell_When_turnExceedsTwiceMean` | Playwright | Seed a turn exceeding 2× mean; use `page.evaluate`/`getComputedStyle` on the flagged cell to assert a visibly different `background-color` from a non-outlier cell (not color-only) |
| UX-AC-c5 | Manual checklist (Axe DevTools / CI Axe gate) | "Outlier highlight meets WCAG AA contrast as rendered" | Manual checklist | Run an automated contrast checker against the live DOM cell (not `TokenBadge.tsx` in isolation); confirm the cell has a visible background fill, not just colored text on the table's ambient row background |
| UX-AC-c6 | `tests/e2e/session-detail-drawer.spec.ts` | `SessionDetailDrawer_should_closeViaAnyExitMethod_When_turnTableLoadedOrNot` | Playwright | Close the drawer via ×, Escape, and overlay-click, both before and after the per-turn table has resolved; assert drawer unmounts identically each time |
| UX-AC-d1 | `tests/e2e/session-list-sort-cost.spec.ts` | `SessionList_should_sortByCostInOneActionAndToggleInOne_When_costOptionSelected` | Playwright | Select `"Sort: Cost"` from the existing dropdown; assert descending-by-default order; click the existing ↑/↓ button; assert ascending |
| UX-AC-d2 | `tests/e2e/session-list-sort-cost.spec.ts` | `SessionList_should_notReorderMoreThanOnce_When_costDataArrivesAfterSortSelected` | Playwright | Select `"Sort: Cost"` before mocking a delayed `GetInsightsSummary` response; observe render order snapshots before/after resolution; assert no more than one reorder event and unloaded rows trail throughout |
| UX-AC-d3 | `tests/e2e/session-list-sort-cost.spec.ts` | `SessionList_should_composeWithExistingFilters_When_sortedByCost` | Playwright | Apply a status/category/tag filter, then select `"Sort: Cost"`; assert the filtered subset is retained and correctly ordered, with no thrown error or blank list |
| UX-AC-d4 | `tests/e2e/session-list-sort-cost.spec.ts` | `SessionList_should_persistTokenCostSortAcrossReload_When_pageReloaded` | Playwright | Select `"Sort: Cost"`; reload the page; assert `STORAGE_KEYS.SORT_FIELD` round-trips `'tokenCost'` and the dropdown still shows `"Sort: Cost"` |

## Test Stack
- **Unit**: Go stdlib testing + testify (assert/require); Jest + React Testing Library for TS
- **Integration**: real in-memory `tokens.TokenStore` (not mocks) per this repo's existing convention
- **E2E / UX**: Playwright (`tests/e2e/`) for anything with an existing e2e harness; manual checklist for the rest (this repo's e2e conventions require data-testid/ARIA locators, no `waitForTimeout`, `@feature` annotation header — see `.claude/rules/e2e-test-conventions.md`)

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods: happy path + error paths covered
- All external integrations: unit mocked + at least one integration test
- UX acceptance criteria: each criterion in design/ux.md has a corresponding test or manual step

## Migration Plan
N/A — confirmed in `implementation/plan.md`: "No Migration Plan section — nothing here touches storage." No `migration_should_be_reversible` test is applicable; this gap-closure project adds a new RPC (`GetSessionTurnTimeline`) and frontend sort/derivation logic only, with no ent schema or storage-format changes.
