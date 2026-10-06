# Stack Research: regression test for durable ship-snapshot rendering

## 1. Component-test stack and existing pattern

`web-app/package.json`: Jest 30.2, `@testing-library/react` 16.3, `@testing-library/user-event`
14.5, `ts-jest` 29.4, `jest-environment-jsdom` 30.2. Run via `npx jest --no-coverage`.

`web-app/src/components/backlog/detail/VersionControlSection.test.tsx` already exists and is the
pattern to match: RTL `render`/`screen`, local `makeGithub()`/`makeWidgetData()`/`makeItem()`
builder functions returning typed fixtures (`GithubSummary`, `VcsWidgetData`, `BacklogItem`), no
hook/RPC mocking needed because `VersionControlSection` takes `widgetData`/`item` as props (no
internal data fetching). Current fixtures only cover `kind: "live"` with a green PR — there is
**no `kind: "historical"` fixture and no assertion on CI conclusion, review counts, or
capture-failure copy** in this file today.

## 2. Existing e2e coverage

`tests/e2e/vcs-widget.spec.ts` (`@feature vcs-widget, backlog:get-item-ship-status`) already
covers the durable-snapshot path end-to-end via `page.route()` interception of
`GetBacklogItemShipStatus` (documented rationale: snapshot fields are response-only server
state with no RPC to set them directly, so this is the same precedent as
`backlog-pipeline-mode.spec.ts`'s `injectFakeSessionWithPipelineSnapshot`). It uses
`tests/e2e/pages/VcsWidgetPage.ts` (data-testid locators only, per
`.claude/rules/e2e-test-conventions.md`) and asserts:
- `VcsWidget_should_RenderPillFileListCommitListAndAsOfTimestamp_When_DurableSnapshotPresent`:
  mergeability pill text `/Shipped/`, file rows, commit rows, "As of" timestamp — with
  `shippedCheckConclusion: "success"`, `shippedApprovedCount: 2`, `shippedChangesReqCount: 0`,
  `snapshotCaptureFailed: false` in the mocked payload, but **no assertion reads the CI badge
  text or the approved/changes-requested counts specifically** (`VcsWidgetGithubRow.tsx`
  renders `CI: {checkConclusion}`, an `aria-label="N approved"` span, and an
  `aria-label="N changes requested"` span — none are asserted).
- `VcsWidget_should_RenderNoHistoryCapturedCopy_When_ShippedSnapshotAtUnset`: neutral copy when
  no snapshot exists.
- **No test sets `snapshotCaptureFailed: true`** — that branch (`VcsWidgetGithubRow.tsx:85-87`,
  `"Couldn't fully capture PR status at ship time"`) only has unit coverage today, in
  `web-app/src/lib/vcs/adapters.test.ts:166-180`
  (`fromShipStatus_should_PreservePartiallySuccessfulGithubGroup_When_SnapshotCaptureFailedTrueAndFileStatsEmpty`),
  which checks the adapter's output object, not rendered DOM.

## 3. Recommendation

**Extend the existing e2e spec (`tests/e2e/vcs-widget.spec.ts`), not a new Playwright file or a
new Jest component test.** Concretely:

- Add assertions to the existing `RenderPillFileListCommitListAndAsOfTimestamp` test (or a new
  sibling `test()` in the same `describe`) for the CI conclusion badge and review-count spans,
  and add one new `test()` that mocks `snapshotCaptureFailed: true` and asserts the
  capture-failure copy renders. This needs 1-2 new `data-testid` locators added to
  `VcsWidgetPage.ts` (e.g. `vcs-widget-ci-conclusion`, `vcs-widget-capture-failed`) since the
  page-helper convention forbids CSS-class locators and `VcsWidgetGithubRow.tsx` currently
  exposes these via `aria-label`/plain `<span>` only — an ARIA-role locator
  (`getByLabelText(/approved/)`) can be used instead of a new testid where the `aria-label`
  already exists, avoiding a source change for the approved/changes-requested counts; the CI
  badge and capture-failure copy have no `aria-label`/role today and would need either a
  `data-testid` added to the component or a text-content locator (`getByText(/^CI: /)`) — text
  locator is lower-friction and still spec-compliant (data-testid/ARIA rule targets structural
  selection, not literal rendered strings already used elsewhere in this spec, e.g.
  `/Shipped/`, `/As of/`).
- **Why e2e over a new Jest component test**: the CI conclusion / review counts / capture-failure
  UI all live inside `VcsWidgetGithubRow`, which is already exercised end-to-end (real backlog
  item, real page navigation, real `VcsWidget` composition) by `vcs-widget.spec.ts` via the same
  `mockShipStatus()` interception this task would reuse — adding assertions there is a ~10-line
  diff with zero new fixtures/mocking infra. A new `VersionControlSection.test.tsx` case would
  need to duplicate a `historical`-kind `VcsWidgetData` builder (not in `VcsWidgetData`'s
  `GithubSummary` fixture there — that file only builds `kind: "live"` shapes) and would
  test one layer removed from what actually ships (`VersionControlSection` → `VcsWidget` →
  `VcsWidgetGithubRow`), duplicating coverage the e2e spec already has scaffolding for.
- If a fast unit-level DOM check is still wanted in addition (cheaper to run in isolation than
  full Playwright), the second-choice location is `VersionControlSection.test.tsx`, matching its
  existing `makeWidgetData()` builder pattern but adding a `kind: "historical"` variant with
  `shippedCheckConclusion`/`shippedApprovedCount`/`shippedChangesReqCount`/`snapshotCaptureFailed`
  fields (see `VcsWidgetData`'s historical variant in `web-app/src/lib/vcs/types.ts` and
  `fromShipStatus` in `web-app/src/lib/vcs/adapters.ts` for the exact shape). This is additive,
  not a replacement for the e2e extension above.

Run to verify: `cd web-app && npx jest --no-coverage --testPathPatterns="VersionControlSection.test"`
(if extended) and `cd tests/e2e && npx playwright test vcs-widget.spec.ts` (primary
recommendation).
