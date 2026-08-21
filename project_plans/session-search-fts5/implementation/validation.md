# Validation Plan: session-search-fts5

**Date**: 2026-08-02

## Happy Path Scenario

Given a triage operator has opened a backlog item's triage panel (Baseline: today they'd have to leave the panel and hand-craft a search on the history page), when the panel mounts and `TriageRelatedWorkSection` auto-fires `RelatedWorkQuery` (item title as query, `groupBySession=true`, `includeContext=true`, `excludeAutomationSessions=true`, `project=repoPath`, `limit=5`), then the operator sees up to 5 session-deduped `SessionHitCard`s — each representing one prior session (not one row per matching message), scoped to the current repo, with automation/background sessions filtered out — without leaving the triage panel or typing a query.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-0: Proto fields additive, wire-compat with existing callers (Story 1.1.1) | `server/services/search_service_test.go` | `TestSearchClaudeHistory_ResponseUnchanged_When_NewFlagsUnset` | Integration | Happy path — request with no new fields set produces byte-identical response shape to pre-change behavior |
| REQ-1: Session-level dedup (Story 1.2.1) | `server/services/search_related_work_test.go` | `TestGroupResultsBySession_KeepsHighestScoredHitPerSession` | Unit | Happy path — 3-message session collapses to 1 row, `more_matches_in_session_count=2` |
| REQ-1: Session-level dedup | `server/services/search_related_work_test.go` | `TestGroupResultsBySession_LeavesSingleHitSessionsUntouched` | Unit | Edge — single-hit session keeps `more_matches_in_session_count=0` |
| REQ-1: Session-level dedup | `server/services/search_related_work_test.go` | `TestGroupResultsBySession_PreservesInputOrderWhenNoGrouping` | Unit | Error/default path — `group_by_session=false`; behavior byte-identical to today (one row per message) |
| REQ-1: Session-level dedup | `server/services/search_related_work_test.go` | `TestGroupResultsBySession_ReturnsEmptySlice_When_InputEmpty` | Unit | Error path (missing from plan.md, added here) — empty input does not panic, returns empty slice |
| REQ-1: Session-level dedup + oversampling | `server/services/search_service_test.go` | `TestSearchClaudeHistory_DedupOversamplesBeforeTruncatingToRequestedLimit` | Integration | Happy path — busy-session-vs-5-distinct-sessions scenario; engine queried with `Limit=25`, final response has 5 session rows not 1 |
| REQ-2: ±5 context window + bookends (Story 1.3.1) | `server/services/search_related_work_test.go` | `TestContextWindowAndBookends_ClampsAtSessionBoundaries` | Unit | Happy path — 20-message session, hit at index 10 → window `[5:16]`, bookends `[0:3]`/`[17:20]` |
| REQ-2: ±5 context window + bookends | `server/services/search_related_work_test.go` | `TestContextWindowAndBookends_SuppressesBookendsWhenWindowCoversFullSession` | Unit | Edge — 8-message session, window already spans transcript, bookends suppressed |
| REQ-2: ±5 context window + bookends | `server/services/search_related_work_test.go` | `TestContextWindowAndBookends_EmptySessionReturnsNil` | Unit | Error path — zero-message session returns `nil, nil, nil` without panicking |
| REQ-2: ±5 context window + bookends | `server/services/search_service_test.go` | `TestSearchClaudeHistory_ContextSourcedFromRawConversationFile_NotDocumentStore` | Integration | Happy path — tokenizer-skipped message ("ok", zero tokens) still appears in `context_window` because it's read via `GetMessagesFromConversationFile`, not `DocumentStore` |
| REQ-3: Scroll mode anchor paging (Story 1.4.1) | `server/services/search_related_work_test.go` | `TestGetClaudeHistoryMessages_AnchorIndexCentersWindow` | Integration | Happy path — 40-message session, `AnchorIndex=20, Limit=10` → returns `messages[15:25]` |
| REQ-3: Scroll mode anchor paging | `server/services/search_related_work_test.go` | `TestGetClaudeHistoryMessages_OffsetLimitUnchanged_When_AnchorIndexUnset` | Integration | Error/default path — `AnchorIndex` unset, `Offset:10,Limit:5` → returns `messages[10:15]`, matching current behavior exactly |
| REQ-4: Automation-session exclusion (Story 1.5.1) | `server/services/search_related_work_test.go` | `TestFilterAutomationSessions_ExcludesSessionsWithHiddenTrue` | Unit | Happy path — session with live `Instance.Hidden=true` excluded |
| REQ-4: Automation-session exclusion | `server/services/search_related_work_test.go` | `TestFilterAutomationSessions_KeepsSessionsWithNoLiveInstanceMatch` | Unit | Error/edge path — no live `Instance` match; best-effort filter keeps the session rather than assuming "human" |
| REQ-4: Automation-session exclusion | `server/services/search_related_work_test.go` | `TestFilterAutomationSessions_KeepsSessionsWithHiddenFalse` | Unit | Happy path (negative case) — `Hidden=false` session kept |
| REQ-4: Automation-session exclusion | `server/services/search_related_work_test.go` | `TestFilterAutomationSessions_KeepsAutonomousModeSessionsThatAreNotHidden` | Unit | Regression guard — `AutonomousMode=true, Hidden=false` kept, confirming the field-selection fix (not `AutonomousMode`) |
| REQ-4: Automation-session exclusion | `server/services/search_service_test.go` | `TestSearchClaudeHistory_LogsExcludedCountOnlyWhenSessionsActuallyExcluded` | Integration | Happy/edge path — conditional log line fires only when `excluded>0`, matching `syncResult.HasChanges()` convention |
| REQ-5: Triage "Find related past work" search box (Story 2.2.1/2.2.2) | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `pre-populates query with backlog item title on mount` | Unit | Happy path — `itemTitle` set → auto-search fires with `RelatedWorkQuery` bundle |
| REQ-5: Triage "Find related past work" search box | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `shows inline alert with retry on search failure` | Unit | Error path — `search()` rejects → `role="alert"` + `Retry` button rendered, `TriageErrorBanner` actions absent |
| REQ-5: Triage "Find related past work" search box | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `does not auto-search when itemTitle is empty` | Unit | Edge path — blank/whitespace title guard, no RPC call |
| REQ-5: Triage "Find related past work" search box | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `shows reassuring copy when zero matches found` | Unit | Edge path — `results:[], totalMatches:0` shows reassuring copy, not generic "no results" |
| REQ-5: Triage "Find related past work" search box | `tests/e2e/triage-related-work.spec.ts` | `Find related past work surfaces prior sessions` | Integration (E2E) | Happy path — seeded triage item, input auto-populates with title, results region renders without a fixed timeout |
| REQ-6: Project scoping filter (Story 1.6.1) | `server/services/search_related_work_test.go` | `TestFilterByProject_KeepsOnlyMatchingProject` | Unit | Happy path — only same-`Project` sessions kept |
| REQ-6: Project scoping filter | `server/services/search_related_work_test.go` | `TestFilterByProject_NoOpWhenProjectEmpty` | Unit | Error/default path — `project=""` is a no-op, matching current (unfiltered) behavior |
| REQ-6: Project scoping filter | `server/services/search_service_test.go` | `TestSearchClaudeHistory_ProjectFilterAppliedBeforeAutomationFilterAndDedup` | Integration | Happy path — verifies filter ordering (project → automation → dedup) so an out-of-project/excluded hit never inflates `more_matches_in_session_count` on a kept session |
| REQ-7: `useHistoryFullTextSearch` carries new fields (Story 2.1.1) | `web-app/src/lib/hooks/useHistoryFullTextSearch.test.ts` | `useHistoryFullTextSearch_should_IncludeNewFlagsInRequest_When_OptionsSet` | Unit | Happy path — `search({groupBySession, includeContext, excludeAutomationSessions})` produces a request payload with those three fields `true` |
| REQ-7: `useHistoryFullTextSearch` carries new fields | `web-app/src/lib/hooks/useHistoryFullTextSearch.test.ts` | `useHistoryFullTextSearch_should_OmitNewFieldsFromResult_When_OptionsNotSet` | Unit | Error/default path — existing callers (`HistorySearchResults.tsx`'s usage) get default `false`/`0`/`[]`, unaffected by the new fields |

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. 0 extra clicks — related work visible on panel load | `tests/e2e/triage-related-work.spec.ts` | `Related work results render automatically on triage panel mount` | Playwright | Navigate to a triage-completed item's detail page → assert `triage-related-work-results` becomes visible without any user interaction, within the 300ms debounce + RPC round trip (no `waitForTimeout`; wait on `toBeVisible()`) |
| 2. Revise search in 1 step (edit the pre-filled box) | `tests/e2e/triage-related-work.spec.ts` | `Editing the pre-filled query re-fires a debounced search` | Playwright | Clear+type new text into `triage-related-work-input` → assert a new request fires and `triage-related-work-results` updates to reflect the new query, via `toHaveValue`/network assertion, not a timeout |
| 3. Retry a failed search in 1 click | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `clicking Retry re-invokes search with the same query and options` | Jest + RTL | Mock `search()` to reject once; render component; click `Retry` inside `triage-related-work-error`; assert `search` called again with identical args |
| 4. Card activation opens a new tab, panel state preserved | `tests/e2e/triage-related-work.spec.ts` | `Activating a session hit card opens the session in a new tab without disturbing the triage panel` | Playwright | Click `triage-related-work-hit-{sessionId}` → assert a new page/tab opens to the session detail route (anchored on `messageIndex` if supported) → assert original tab's triage panel state (query text, results) is unchanged |
| 5. Zero-match copy is the exact reassuring text | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `shows reassuring copy when zero matches found` | Jest + RTL | Resolve `search()` with `results:[]` → assert `triage-related-work-empty` text is exactly "No related past sessions found — this looks like new territory." |
| 6. Search-failure copy + role=alert, no TriageErrorBanner actions | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `shows inline alert with retry on search failure` | Jest + RTL | Reject `search()` → assert `triage-related-work-error` has `role="alert"`, text "Search failed —", a `Retry` button, and absence of "Reload item"/"Skip without applying" text |
| 7. Blank/whitespace title → no RPC, unfocused input | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `does not auto-search when itemTitle is empty` | Jest + RTL | Render with `itemTitle=""` → assert `search` never called, `triage-related-work-input` renders empty and does not have focus |
| 8. No dead ends — every state leaves input editable, Apply/Skip reachable | `tests/e2e/triage-related-work.spec.ts` | `All four related-work states leave the input editable and panel actions reachable` | Playwright | For each of loading/error/empty/populated (force via mocked responses or timing): assert `triage-related-work-input` remains enabled and focusable, and Apply/Skip buttons in the parent panel remain in the tab order |
| 9. Keyboard: Tab + Enter/Space activate cards | `tests/e2e/triage-related-work.spec.ts` | `Result cards are keyboard-reachable and activatable via Enter and Space` | Playwright | `page.keyboard.press("Tab")` to reach a `triage-related-work-hit-*` button, press `Enter` → assert new-tab navigation; repeat with `Space` |
| 10. Screen reader semantics — aria-label, ul/li, single aria-live | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `exposes correct aria-label and list semantics for assistive tech` | Jest + RTL | Assert input `aria-label` equals `"Search past sessions for {itemTitle}"` (or `"this item"` fallback); assert results container is a `<ul>` with `<li>` children; assert no nested `aria-live` attribute is present on any element within the component |
| 11. Color contrast ≥4.5:1 in light and dark theme | `tests/e2e/triage-related-work.spec.ts` (axe integration, per repo's UX-analysis CI) | `Axe Core reports no contrast violations on TriageRelatedWorkSection in light and dark theme` | Playwright + Axe Core | Render the section in both themes → run Axe Core scan scoped to the component → assert zero `color-contrast` violations |
| 12. Focus never programmatically moved on mount/auto-search | `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` | `does not move focus into the input or results on mount or after auto-search resolves` | Jest + RTL | Render component, wait for auto-search to resolve → assert `document.activeElement` is unchanged from pre-mount (e.g. `document.body`), not the input or a result card |
| 13. Read-only mode — section fully absent from DOM | `web-app/src/components/backlog/TriageReviewPanel.test.tsx` | `omits TriageRelatedWorkSection entirely when readOnly is true` | Jest + RTL | Render `TriageReviewPanel` with `readOnly=true` → assert `queryByTestId("triage-related-work-input")` is `null` (absence, not `disabled`/hidden) |

## Test Stack

- **Unit**: Go stdlib testing (`server/services`), Jest + React Testing Library (`web-app`)
- **Integration**: Go stdlib testing with in-memory fakes (existing `search_service_test.go` conventions — fake `hist`/`ss.getInstances`, temp conversation files for `GetMessagesFromConversationFile`)
- **E2E / UX**: Playwright (`tests/e2e/`), per `.claude/rules/e2e-test-conventions.md` — feature annotation header, no `waitForTimeout`, `data-testid`/ARIA locators only, page helpers in `tests/e2e/pages/`

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods (`groupResultsBySession`, `contextWindowAndBookends`, `filterAutomationSessions`, `filterByProject`, `SearchClaudeHistory`, `GetClaudeHistoryMessages`): happy path + error/edge paths covered.
- All external integrations (`hist.GetMessagesFromConversationFile`, `ss.getInstances()`, ConnectRPC `searchClaudeHistory` call): unit mocked + at least one integration test.
- UX acceptance criteria: all 13 criteria in `design/ux.md` have a corresponding automated test above; none require a manual-only step.
- Migration: N/A — no schema or persisted-data changes in this plan (confirmed in `plan.md`'s Migration Plan section); no migration test needed.

## Requirements Coverage Summary

- 7 requirement groups (REQ-0 through REQ-7, covering all 6 requirements.md In-Scope items plus the Story 1.6.1 project-scoping bug fix and Story 1.1.1 wire-compat guarantee that Epic 2.2 depends on) — 7/7 mapped to at least one unit and one integration/error test.
- 13/13 UX acceptance criteria from `design/ux.md` mapped to an automated test.
