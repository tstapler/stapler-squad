# Validation Plan: insights-session-visibility

**Date**: 2026-09-12

## Naming conventions used

- **Go**: `insights_service_test.go` mixes two styles historically, but every
  test added for `buildSessionSummary`/`GetInsightsSummary`/`ListSessionTokens`
  in the immediately-preceding Epics 1.1-1.3 (the code this project extends —
  e.g. `TestGetInsightsSummary_WhenUnpricedModelFamily_ExpectPricingUnavailableFlagged`,
  `TestGetInsightsSummary_WhenSessionClassified_ExpectActivityTypeSetOnSummary`)
  uses `TestFunctionName_WhenCondition_ExpectBehavior`, which is also the exact
  style plan.md's own Tasks 1.2.3b/1.3.2d/1.3.3f already prescribe for the new
  tests. This plan follows that convention for consistency with the code it
  sits next to, rather than the older `should_..._When_...` style used
  elsewhere in the same file for unrelated (streaming/timeline) tests.
- **TS/jest**: matches `SessionsTable.test.tsx` exactly — outer
  `describe("ComponentName_should_behavior_when_condition", ...)`, inner
  `it("plain English sentence", ...)`.

## Requirement -> Test Mapping

### R1 — Token-type visual breakdown (Success Metric 1 / Scope §1)

| PR | Requirement | Test File | Test Name | Type | Scenario |
|----|-------------|-----------|-----------|------|----------|
| PR2 | R1 table | `web-app/src/app/insights/TokenBreakdownBar.test.tsx` | `TokenBreakdownBar_should_renderProportionalSegments_when_mixedTokenCounts` | Unit | Happy path: 4 nonzero token counts render 4 segments sized to their share of the total |
| PR2 | R1 table | `web-app/src/app/insights/TokenBreakdownBar.test.tsx` | `TokenBreakdownBar_should_renderNeutralBarWithoutNaN_when_totalIsZero` | Unit | Edge: all 4 counts are 0 — must render a single neutral segment, never `NaN%` widths |
| PR2 | R1 table | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_renderCompositionBarPerRow_when_sessionsHaveTokenCounts` | Integration | `TokenBreakdownBar` wired into a real table row; asserts the "Composition" column renders and its `title` attribute carries all 4 raw formatted counts |
| PR2 | R1 detail | `web-app/src/app/insights/TokenBreakdownChart.test.tsx` | `TokenBreakdownChart_should_renderFourSeriesWithCorrectValues_when_sessionHasTokenBreakdown` | Unit | Happy path: 4 Recharts series render with values matching the session's token fields |
| PR2 | R1 detail | `web-app/src/app/insights/TokenBreakdownChart.test.tsx` | `TokenBreakdownChart_should_renderEmptyStateWithoutThrowing_when_totalIsZero` | Unit | Edge: zero-total session renders a placeholder, not an empty chart / Recharts "no data" warning |
| PR2 | R1 detail | `web-app/src/app/insights/SessionDetailContent.test.tsx` | `SessionDetailContent_should_renderTokenCompositionSection_when_sessionProvided` | Integration | `TokenBreakdownChart` wired into the detail view; asserts the new "Token Composition" section renders with the chart inside |

### R2 — Tag data + filter (Success Metric 2 / Scope §2)

| PR | Requirement | Test File | Test Name | Type | Scenario |
|----|-------------|-----------|-----------|------|----------|
| PR1 | `tags` on `SessionTokenSummary` | `server/services/insights_service_test.go` | `TestGetInsightsSummary_WhenSessionHasTags_ExpectTagsPopulated` | Unit | Happy path: session record carries tags, response's `Tags` field matches |
| PR1 | `tags` on `SessionTokenSummary` | `server/services/insights_service_test.go` | `TestGetInsightsSummary_WhenSessionOrphaned_ExpectTagsEmpty` | Unit | Edge: no matching `SessionRecord` (orphan) — `Tags` is empty, not an error |
| PR1 | `Tags` threading | `session/tokens/association_test.go` | `TestAssociateRecordWithSnapshot_MatchReturnsFullRecord` | Unit | Happy path: matched `SessionRecord` (including `.Tags`) is returned, not just its ID |
| PR1 | `Tags` threading | `session/tokens/association_test.go` | `TestAssociateRecordWithSnapshot_NoMatchReturnsZeroValueAndOrphanTrue` | Unit | Edge: no match returns a zero-value record and `isOrphan=true`, no panic |
| PR1 | `Tags` sourcing | `session/storage_test.go` | `TestListSessionRecords_WhenInstanceHasTags_ExpectSessionRecordTagsPopulatedFromRealStorage` | Integration | Real `Storage.ListSessionRecords()` against a persisted `InstanceData.Tags` — proves the wire-up from the actual store, not just the fixture struct literal |
| PR2 | Tag filter UI | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_filterToSessionsWithSelectedTag_when_oneTagChipSelected` | Unit | Happy path: selecting one chip narrows to sessions containing that tag |
| PR2 | Tag filter UI | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_showAllSessions_when_noTagChipsSelected` | Unit | Edge: zero chips selected — filter is a no-op, all sessions shown |
| PR2 | Tag filter UI (OR semantics) | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_matchSessionsWithEitherSelectedTag_when_multipleTagChipsSelected` | Unit | Edge: 2 chips selected — sessions matching *either* tag are included (OR, per plan Task 2.2.2b) |
| PR2 | Tag filter UI | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_excludeUntaggedSession_when_tagFilterActive` | Unit | Edge: a session with `tags: []` is correctly excluded while a filter is active |
| PR2 | Fuse.js tag search | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_surfaceTagMatch_when_searchTextMatchesTagName` | Integration | Typing a tag name into the existing search box surfaces sessions by tag, through the real Fuse index build |

### R3 — Session role as a first-class, batched field (Success Metric 3 / Scope §3)

| PR | Requirement | Test File | Test Name | Type | Scenario |
|----|-------------|-----------|-----------|------|----------|
| PR1 | `session_role` populated | `server/services/insights_service_test.go` | `TestGetInsightsSummary_WhenSessionHasItemSessionRole_ExpectSessionRolePopulated` | Unit | Happy path: a matching `ItemSession` row's role reaches the response |
| PR1 | `session_role` populated | `server/services/insights_service_test.go` | `TestGetInsightsSummary_WhenNoItemSessionRow_ExpectSessionRoleEmpty` | Unit | Edge: no `ItemSession` row for the session UUID — role is `""`, not an error |
| PR1 | Nil-safety | `server/services/insights_service_test.go` | `TestGetInsightsSummary_WhenBacklogReaderNil_ExpectNoPanicAndEmptyRole` | Unit | Edge: `backlogReader == nil` (e.g. a test/older wiring) — no panic, role is `""` |
| PR1 | Most-recent-wins ordering | `session/ent_repository_backlog_test.go` | `TestGetAllItemSessionsWithBacklogInfo_MultipleSessionsPerUUID_OrdersNewestFirst` | Integration | Real ent-backed DB: two `ItemSession` rows for the same `session_uuid` with different roles/`created_at` — asserts `.Order(ent.Desc(...))` puts the newer row first |
| PR1 | Batched lookup — no N+1 | `server/services/insights_service_test.go` | `TestGetInsightsSummary_WhenMultipleSessionsRequested_ExpectBacklogReaderCalledExactlyOnce` | Unit | Regression guard: a fake `insightsBacklogReader` with a call counter; 3+ sessions in the request must still trigger exactly 1 `GetAllItemSessionsWithBacklogInfo` call |
| PR1 | Batched lookup — no N+1 | `server/services/insights_service_test.go` | `TestListSessionTokens_WhenMultipleSessionsRequested_ExpectBacklogReaderCalledExactlyOnce` | Unit | Same regression guard for the `ListSessionTokens` call site |
| PR1 | Batched lookup — per-event, not per-session | `server/services/insights_service_test.go` | `TestWatchInsights_WhenEventReceived_ExpectBacklogReaderCalledExactlyOncePerEvent` | Integration | Real goroutine-driven `watchInsights` loop (mirrors existing `TestWatchInsights_*` tests' channel-driven style) with a counting fake reader — one streamed event triggers exactly one extra call, not zero and not one-per-session |
| PR1 | "First entry seen wins" map semantics | `server/services/insights_service_test.go` | `TestSessionRolesForSessions_WhenSessionUUIDHasMultipleItemSessionEntries_ExpectFirstEntrySeenWins` | Unit | Fake reader returns two entries for the same UUID in DESC-by-created_at order (as the real query now guarantees) — asserts the map keeps the *first* (i.e. newest) role, complementing the ent-layer ordering integration test above |
| PR1 | `ItemSession.session_uuid` vs `ParseResult.SessionUUID` keying gotcha | `server/services/insights_service_test.go` | `TestBuildSessionSummary_WhenRoleMapKeyedBySessionIDNotConversationUUID_ExpectCorrectRoleAttributed` | Unit | Fixture where `ParseResult.SessionUUID` (conversation ID) and the associated `SessionRecord.SessionID` (the real stapler-squad session/instance UUID) differ; role map is keyed by the *SessionRecord*'s ID. Fails if `buildSessionSummary` were wired to look up `roleMap[r.SessionUUID]` instead of `roleMap[sessionID]` |
| PR1 | `buildSessionSummary` tags+role happy path | `server/services/insights_service_test.go` | `TestBuildSessionSummary_WhenSessionHasTagsAndRole_ExpectBothPopulated` | Unit | Direct call: session has both tags and a role map entry — both fields populated together |
| PR1 | `buildSessionSummary` no tags | `server/services/insights_service_test.go` | `TestBuildSessionSummary_WhenSessionHasNoTags_ExpectTagsFieldEmptySlice` | Unit | Edge: matched record has `Tags: nil`/`[]` — `Tags` on the summary is empty, not a proto nil/`[]` ambiguity |
| PR1 | `buildSessionSummary` no backlog linkage | `server/services/insights_service_test.go` | `TestBuildSessionSummary_WhenNoBacklogLinkage_ExpectSessionRoleEmptyStringNotError` | Unit | Edge: `roleMap` has no entry for this session's ID — `SessionRole` is `""`, function does not error/panic (Go's nil-map-read-returns-zero-value semantics) |
| PR2 | Role badge sourced from field | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_showRoleFromSessionField_when_backlogEntryAlsoPresent` | Unit | Happy path: `s.sessionRole` drives the badge text/title; `backlogEntry`'s item title/link still used for the link portion |
| PR2 | Role badge survives deleted item | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_showRoleOnlyBadgeWithNoLink_when_sessionRoleSetButBacklogEntryMissing` | Unit | Edge: `s.sessionRole` set, no matching `backlogIndex` entry (hard-deleted item) — plain `<span>` badge renders, no `<a href>` to a dangling item |
| PR2 | Role row always visible | `web-app/src/app/insights/SessionDetailContent.test.tsx` | `SessionDetailContent_should_alwaysRenderRoleRow_when_sessionRoleSetRegardlessOfBacklogEntry` | Integration | Detail view's "Role" `<dt>/<dd>` renders from `session.sessionRole` whether or not `backlogEntry` is passed at all |

### R4 — Missing metric tooltips (Success Metric 4 / Scope §4)

| PR | Requirement | Test File | Test Name | Type | Scenario |
|----|-------------|-----------|-----------|------|----------|
| PR2 | Waste Score tooltip | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_showWasteScoreTooltipExplanation_when_headerFocused` | Unit | Happy path: hovering/focusing the header reveals the weighted-blend/higher-is-worse/"Not evaluated"/"—" explanation text |
| PR2 | Cache ROI tooltip | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_showCacheRoiTooltipExplanation_when_headerFocused` | Unit | Happy path: hovering/focusing the header reveals the dollar-savings/sign-convention explanation text |
| PR2 | No a11y regression from wrapping | `web-app/src/app/insights/SessionsTable.test.tsx` | `SessionsTable_should_notAddNestedInteractiveElement_when_tooltipWrapsSortableHeader` | Unit | Edge: after wrapping in `Tooltip`, each sortable header still exposes exactly one `role="button"` element (no nested interactive element regression per pitfalls.md §4) |
| PR2 | e2e locator regression (Task 2.4.1b) | `tests/e2e/insights-sessions-table-sort.spec.ts` | *(existing spec, re-run — no new test added)* | E2E | Re-run against the wrapped header: confirms `/Waste Score/i`-based locators (lines 23/25/91/117 per adversarial-review.md) still resolve after the `Tooltip` wrap. Flagged as a **verification task**, not a new spec, since `Tooltip`'s `asChild` prop introduces no extra DOM node |

### Process requirement (Success Metric 5 — not a new test row)

`make ci`/`make ready` passing and all pre-existing Insights tests
(`insights_service_test.go`, `SessionsTable.test.tsx`, `SessionDetailContent.test.tsx`,
`insights-sessions-table-sort.spec.ts`) continuing to pass is a CI gate, not an
individual test case — tracked in Epic 1.4/Epic 2.5's validation tasks in
plan.md (`gofmt -w .`, `go build ./...`, `go test ./server/services/... ./session/...`,
`make lint`, `make ci`/`make ready`, `pnpm run lint:duplicates`,
`make registry-generate`).

## Test Stack

- **Unit (Go)**: `go test ./server/services/... ./session/... -timeout=20m`
  (via `gotestsum` per repo convention: `make test`). New/changed files:
  `server/services/insights_service_test.go`, `session/tokens/association_test.go`.
  Fakes follow this file's existing patterns: `fakeTokenStore` (already
  present), a new `fakeInsightsBacklogReader` (mirrors `fakeSessionStorage`'s
  shape — a struct wrapping a slice of `session.ItemSessionBacklogEntry` plus
  a `callCount int` field incremented in `GetAllItemSessionsWithBacklogInfo`,
  read back by the N+1-regression tests) satisfying the new
  `insightsBacklogReader` interface.
- **Unit (TS)**: `cd web-app && npx jest --no-coverage --testPathPatterns="insights"`.
  React Testing Library + `@testing-library/user-event`, matching
  `SessionsTable.test.tsx`'s/`SessionDetailContent.test.tsx`'s existing
  `makeSession()` fixture-builder pattern (extended with `tags: string[]` and
  `sessionRole: string` fields/overrides).
- **Integration**:
  - Go: `session/storage_test.go`'s existing real-storage fixtures (not the
    `fakeSessionStorage` used elsewhere in this file) for
    `TestListSessionRecords_WhenInstanceHasTags_...`; `session/ent_repository_backlog_test.go`'s
    existing real ent/sqlite test-DB setup for the ordering test.
  - TS: component-composition tests already listed above (`SessionsTable`
    rendering `TokenBreakdownBar`/tag chips/role badge together;
    `SessionDetailContent` rendering `TokenBreakdownChart`/role row) — these
    exercise real component wiring, not isolated unit fixtures, so they're
    classified Integration even though they run under jest.
- **E2E**: no new Playwright spec required by this project's scope. One
  existing spec, `tests/e2e/insights-sessions-table-sort.spec.ts`, must be
  re-run (Task 2.4.1b) to confirm its `/Waste Score/i` locators still resolve
  after the header gets wrapped in `Tooltip`. `tests/e2e/pages/InsightsPage.ts`
  needs no new page-object methods (no new page navigation/interaction
  surface beyond what jest already covers at the component level) — tag
  chips and the composition bar are exercised in jest, not e2e, since they
  are Insights-page-internal filtering behavior with no distinct route.

## Coverage Targets

- Unit test coverage: >=80% (line) for `server/services/insights_service.go`'s
  new code paths (`sessionRolesForSessions`, the `Tags`/`SessionRole` branches
  in `buildSessionSummary`) and for the new `TokenBreakdownBar.tsx`/`TokenBreakdownChart.tsx`
  components.
- All public service methods touched by this project (`GetInsightsSummary`,
  `ListSessionTokens`, `watchInsights`, `AssociateRecordWithSnapshot`): happy
  path + error/edge path covered above.
- All external integrations (`Storage.GetAllItemSessionsWithBacklogInfo` via
  `insightsBacklogReader`, `Storage.ListSessionRecords()`): unit-mocked (fake
  reader/storage) **and** at least one integration test against the real
  ent-backed storage — both present above (`TestGetAllItemSessionsWithBacklogInfo_MultipleSessionsPerUUID_OrdersNewestFirst`,
  `TestListSessionRecords_WhenInstanceHasTags_ExpectSessionRecordTagsPopulatedFromRealStorage`).
