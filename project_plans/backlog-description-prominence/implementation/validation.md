# Validation Plan: backlog-description-prominence

**Date**: 2026-08-01

## Happy Path Scenario

Given a backlog item with a non-empty description and no stored
`backlog-detail-section-<itemId>-description` localStorage entry (per the
Baseline in requirements.md), when the user opens that item's detail view,
then the Description section is rendered already expanded — `aria-expanded="true"`
on `collapsible-header-description` and `backlog-description-rendered`
present in the DOM — with zero clicks required.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 1.1.1 — new/never-toggled items show Description expanded on first view (driving-default flip, `BacklogItemDetail.tsx:323`) | `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_ExpandDescriptionByDefault_When_NoStoredPreferenceExists` (Task 1.4.1a, currently named `"expands Description by default when no stored preference exists for the item"`) | Component/"integration" (React Testing Library, full `BacklogItemDetail` tree) | Happy path — no localStorage key for `item-1`'s description → `aria-expanded="true"`, content visible, 0 clicks |
| Story 1.2.1 — `DescriptionSection` accepts `defaultExpanded` prop like every sibling section | `web-app/src/components/backlog/detail/DescriptionSection.test.tsx` | `DescriptionSection_should_DefaultExpanded_When_AnyItemRenders` (Task 1.3.1a rename of `DescriptionSection_should_DefaultCollapsed_When_AnyItemRenders`) | Unit (standalone component, no `CollapsibleGroup` ancestor) | Happy path — `<DescriptionSection item={makeItem()} defaultExpanded={true} />` → `aria-expanded="true"`, `backlog-description-rendered` present, no click |
| Story 1.2.1 — edge/error path: prop still honored when caller passes `false` | `web-app/src/components/backlog/detail/DescriptionSection.test.tsx` | **Gap** — no test currently asserts `defaultExpanded={false}` standalone (see Coverage Gaps below) | Unit | N/A today — not in plan.md's task list; optional addition |
| Story 1.3.1 — markdown reveal test still passes under new default | `web-app/src/components/backlog/detail/DescriptionSection.test.tsx` | `DescriptionSection_should_RenderMarkdownContent_When_DefaultExpandedTrue` (Task 1.3.1a rename of `"reveals the markdown description once expanded"`, drops `fireEvent.click`) | Unit | Happy path — `<DescriptionSection item={makeItem({description:"**bold**"})} defaultExpanded={true} />` → content visible without clicking |
| Story 1.3.1 — empty-state message still shown under new default | `web-app/src/components/backlog/detail/DescriptionSection.test.tsx` | `DescriptionSection_should_ShowEmptyStateMessage_When_NoDescriptionAndDefaultExpandedTrue` (Task 1.3.1a rename of `"shows an empty-state message when there is no description"`, drops `fireEvent.click`) | Unit — edge path (empty description content) | Edge — `description: ""`, `defaultExpanded={true}` → `"No description."` visible with no click |
| Story 1.3.2 — `expandDescription()` helper stays idempotent under the new default | `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx` | No new test — `expandDescription()` guard (Task 1.3.2a) is exercised indirectly by the 3 existing markdown tests: `"renders bold text and links instead of literal markdown syntax"`, `"renders an embedded image"`, `"never executes an injected <script> tag in the description"` | Component/"integration" | Regression guard — helper must not toggle an already-open section closed; covered by the 3 pre-existing tests continuing to pass after the guard is added |
| Story 1.4.1 — fresh item (no stored preference) renders expanded | `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_ExpandDescriptionByDefault_When_NoStoredPreferenceExists` (same as Story 1.1.1 row — one test satisfies both) | Component/"integration" | Happy path — see above |
| Story 1.4.2 — item with stored `"false"` preference stays collapsed (non-regression) | `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_KeepDescriptionCollapsed_When_StoredPreferenceIsFalse` (Task 1.4.2a, currently named `"keeps Description collapsed when the item has a stored false preference"`) | Component/"integration" — edge/regression path | Edge — `localStorage.setItem("backlog-detail-section-item-1-description", "false")` before render → `aria-expanded="false"`, `backlog-description-rendered` absent |
| Story 1.4.3 — Acceptance Criteria's always-visible rendering unaffected (Success Metric 3, added post-consistency-review) | `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx` | Extends `BacklogItemDetail_should_ExpandDescriptionByDefault_When_NoStoredPreferenceExists` (Task 1.4.3a) with an added AC-visibility assertion | Component/"integration" | Regression — same render as Story 1.1.1's test, plus assert Acceptance Criteria's existing selector is present, confirming no collapse affordance was introduced |
| Story 1.5.1 — e2e spec updated for new default (added post-pre-mortem, P1 finding #1) | `tests/e2e/backlog-item-detail-redesign.spec.ts` | Rewritten test (Task 1.5.1a), replacing `"expands a top-level section from its own default-collapsed state"` | E2E (Playwright) | Happy path + toggle — fresh item shows `aria-expanded="true"` with 0 clicks, then collapse→re-expand still works via a single click each direction |

## UX Acceptance Tests

| UX Criterion (design/ux.md §5, item #) | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Fresh item shows Description expanded, 0 clicks, non-empty case | `BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_ExpandDescriptionByDefault_When_NoStoredPreferenceExists` | Jest + RTL | Render with no stored key, resolve `getBacklogItem` with non-empty description, assert `aria-expanded="true"` + content present |
| 1. Fresh item shows Description expanded, 0 clicks, empty case | `DescriptionSection.test.tsx` | `DescriptionSection_should_ShowEmptyStateMessage_When_NoDescriptionAndDefaultExpandedTrue` | Jest + RTL | Render standalone with `description: ""`, `defaultExpanded={true}`, assert `"No description."` visible with no click. *(Covers the empty-description case at the unit level; no `BacklogItemDetail`-level empty-description test exists — acceptable, since the unit test already proves the same `defaultExpanded` prop path the integration test exercises for the non-empty case.)* |
| 2. Never-toggled user sees Description open on every fresh visit | `BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_ExpandDescriptionByDefault_When_NoStoredPreferenceExists` | Jest + RTL | Same test as #1 — "every fresh visit" is provable per-render since `useSectionExpandState` reads localStorage synchronously each mount; one representative render is sufficient coverage |
| 3. User who previously collapsed stays collapsed on revisit | `BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_KeepDescriptionCollapsed_When_StoredPreferenceIsFalse` | Jest + RTL | Seed `localStorage` with `"false"` before render, assert collapsed |
| 4. Collapsing remains a single click from either state | *(no new test — pre-existing `Collapsible`/`CollapsibleSection` toggle behavior, unmodified by this plan)* | — | — | Out of scope: this plan changes only the initial default value, not the toggle mechanism; existing `Collapsible` component tests (outside this plan) already cover click-to-toggle |
| 5. Re-expanding remains a single click; choice persists across reload | *(no new test — `useSectionExpandState` persistence is untouched)* | — | — | Out of scope: persistence write/read path (`useSectionExpandState.ts`) has no behavior change in this plan |
| 6. Acceptance Criteria pixel-identical before/after | `BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_ExpandDescriptionByDefault_When_NoStoredPreferenceExists` (extended by Task 1.4.3a) | Jest + RTL | Now directly asserted: the fresh-item test also confirms Acceptance Criteria's existing selector is present/unaffected, in addition to the file-list diff-review argument (plan.md never touches `AcCriteriaList.tsx`) |
| 7. No layout shift/flash — state resolved before first paint | `BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_ExpandDescriptionByDefault_When_NoStoredPreferenceExists` / `BacklogItemDetail_should_KeepDescriptionCollapsed_When_StoredPreferenceIsFalse` | Jest + RTL | Indirect: both tests assert `aria-expanded` immediately after `render()` settles (via `screen.findByTestId`) with no intermediate "collapsed-then-expanded" assertion step — RTL/jsdom doesn't model paint timing, so this AC is only partially provable by unit test. Not a gap in practice: `useSectionExpandState` reads `localStorage` synchronously before first paint (plan.md Verification section), so there is no async gap that could produce a flash. |
| 8. Keyboard accessibility — Tab/Enter/Space unchanged | *(no new test — Radix's built-in keyboard handling, unmodified by this plan)* | — | — | Out of scope: no markup/ARIA authored by hand in this plan (ux.md line 121: "no new ARIA attributes are hand-authored") |
| 9. Screen reader — `aria-expanded="true"` on first render for no-stored-preference item | `BacklogItemDetail.markdown.test.tsx` | `BacklogItemDetail_should_ExpandDescriptionByDefault_When_NoStoredPreferenceExists` | Jest + RTL | Direct assertion: `expect(header).toHaveAttribute("aria-expanded", "true")` |
| 10. No new visual elements/tokens | *(no test — no CSS/markup change in this plan)* | — | — | Confirmed by Constraints in requirements.md ("no CSS changes anticipated") and plan.md's file list containing no `.css.ts` files |

## Test Stack

- **Unit**: Jest + React Testing Library, standalone component render (`DescriptionSection.test.tsx`) — no `CollapsibleGroup` ancestor, exercises the `defaultExpanded` prop directly.
- **Integration** (closest analog in this stack — no data store/external call to integrate against): Jest + RTL full-tree render of `BacklogItemDetail` (`BacklogItemDetail.markdown.test.tsx`) with `useBacklogService`/`useWatchBacklogItems`/etc. mocked, exercising the real `CollapsibleGroup` + `useSectionExpandState` + localStorage interaction end to end within jsdom.
- **E2E / UX**: One existing Playwright spec required an update — `tests/e2e/backlog-item-detail-redesign.spec.ts` hardcoded the *old* collapsed-by-default precondition for Description (found during the pre-mortem pass, since it lives outside `web-app/src` and was missed by the original research grep). Task 1.5.1a rewrites it to assert the new default and exercise the collapse/re-expand toggle. No other new Playwright spec is needed — everything else is fully covered by the two Jest suites above.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest (targeted) | `cd web-app && npx jest --no-coverage --testPathPatterns="DescriptionSection|BacklogItemDetail.markdown|BacklogItemDetail.test"` | All 3 files pass — this is plan.md's own Verification gate |
| TypeScript/Jest (full gate) | `make quick-check` (build + test + lint) | Green — repo-wide pre-ship gate per plan.md's Risk Control |

- All public component props: `DescriptionSection`'s new `defaultExpanded` prop has happy-path (`true`) coverage; `false` standalone coverage is a noted gap (see below), but the `false` path is fully covered end-to-end through `BacklogItemDetail`'s stored-preference test (Story 1.4.2), which is the only real-world path that produces `defaultExpanded={false}` today.
- No external integrations (pure client-side default UI state, per requirements.md's Non-functional Requirements) — no mocking-vs-integration split needed beyond the existing `useBacklogService` mock already in `BacklogItemDetail.markdown.test.tsx`.
- UX acceptance criteria: 6 of 10 criteria (#1, #2, #3, #7 partial, #9) have direct or indirect test coverage; 4 (#4, #5, #6, #8, #10 — 5 actually) are explicitly out of scope for new tests because this plan changes no code path they depend on (toggle mechanism, persistence read/write, `AcCriteriaList`, keyboard handling, visual tokens) — verified by cross-checking plan.md's file list.

## Coverage Gaps (noted, not blocking)

1. **`DescriptionSection` standalone with `defaultExpanded={false}`** has no direct unit test after the Task 1.3.1a rewrite (all 3 rewritten tests pass `defaultExpanded={true}`). The `false` path is exercised only through the full `BacklogItemDetail` integration test (Story 1.4.2), which passes `false` indirectly via the `CollapsibleGroup`'s controlled `value` (per Pattern Decisions in plan.md, the child's own `defaultExpanded` is a no-op in grouped mode anyway — Collapsible.tsx:139-157). Given the prop is genuinely inert in the only place it's used in production (inside the group), this is a low-value gap; add a 4th standalone test only if `DescriptionSection` is ever rendered outside a `CollapsibleGroup` in production code.
2. **UX AC #7 (no layout shift/flash before first paint)** cannot be fully proven by jsdom-based Jest/RTL tests, which don't model browser paint timing — the existing `aria-expanded` assertions after `render()` settles are the closest available proxy. If stronger evidence is wanted, a manual visual check (load the item detail page, confirm no visible flash) during PR review is the appropriate supplement — not a new automated test, per requirements.md's Appetite (XS, no new tooling).

## Migration

N/A — no schema or persisted-data change in this plan (per requirements.md's Constraints, existing localStorage entries are read as-is; no migration of previously-collapsed state is performed or tested).

## Summary

- **Unit tests**: 3 in `DescriptionSection.test.tsx` (1 happy-path default, 1 happy-path markdown reveal, 1 edge-path empty-state) — all pre-specified by plan.md Task 1.3.1a.
- **Component/"integration" tests**: 2 new in `BacklogItemDetail.markdown.test.tsx` (1 happy-path fresh-item extended with the AC-visibility assertion from Task 1.4.3a, 1 edge-path stored-false-preference) — pre-specified by Tasks 1.4.1a/1.4.2a/1.4.3a — plus 3 pre-existing markdown tests whose continued passing validates the `expandDescription()` helper guard (Task 1.3.2a).
- **E2E**: 1 rewritten in `tests/e2e/backlog-item-detail-redesign.spec.ts` (Task 1.5.1a) — replaces the stale collapsed-by-default assertion found during pre-mortem.
- **Requirements coverage**: 8/8 plan.md Stories (1.1.1, 1.2.1, 1.3.1, 1.3.2, 1.4.1, 1.4.2, 1.4.3, 1.5.1) mapped to at least one test = 100%.
- **UX acceptance tests**: 6 of 10 UX criteria now have direct/indirect automated coverage (#1, #2, #3, #6, #7-partial, #9); 4 are correctly out of scope (unchanged code paths: #4, #5, #8, #10) — 10/10 criteria accounted for in the traceability matrix, 0 unexplained gaps.
- **Migration test**: N/A.
