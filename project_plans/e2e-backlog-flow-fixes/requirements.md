# Requirements: e2e-backlog-flow-fixes

## Context

Triage item: "e2e: 40 baseline-fail rows in backlog flows (state-dependent or selector drift)"
Issue: https://github.com/tstapler/stapler-squad/issues/1183
Baseline: `tests/e2e/baseline/next-e2e-baseline.json` (commit 9611070, captured 2026-10-29)

After PR #1166, the Next baseline recorded 529 specs of class `fail`. Of those, 40 rows fall in
11 backlog-* spec files (`baseline-fail-triage.json` category `backlog-flows`). Two rows
(`backlog-copy-link.spec.ts` × 2 and `backlog-detail-live-updates.spec.ts` × 2) are covered by
PR #1208; the remaining ≈36–38 rows remain open. This project fixes them.

## Failing Spec Files and Row Counts

| Spec file | Baseline fail rows |
|---|---|
| `backlog-connection-indicator.spec.ts` | 2 |
| `backlog-copy-link.spec.ts` | 2 (remaining; 2 covered by #1208) |
| `backlog-deep-link-errors.spec.ts` | 4 |
| `backlog-detail-live-updates.spec.ts` | 4 (2 may be covered by #1208) |
| `backlog-edit-buffering.spec.ts` | 4 (all pass in current baseline — already fixed) |
| `backlog-github-sync-settings.spec.ts` | 4 |
| `backlog-manual-override.spec.ts` | 2 |
| `backlog-pipeline-mode.spec.ts` | 2 |
| `backlog-rich-text.spec.ts` | 2 |
| `backlog-sources-settings.spec.ts` | 8 |
| `backlog.spec.ts` | 6 |

## Root Causes (four categories)

### Category A — shared-server state pollution
Tests create backlog items or sources in `beforeEach`/within the test body but have no
`afterEach` cleanup. Items accumulate across test runs (or between retry attempts). Later
tests in the same run, or the next CI run against a warmed shared server, see unexpected
state.

Affected: `backlog-sources-settings`, `backlog-github-sync-settings`,
`backlog-detail-live-updates`, `backlog-manual-override`, `backlog-pipeline-mode`,
`backlog-rich-text`, `backlog-connection-indicator` (guard item leaks).

### Category B — `test.fail(true)` + early-return anti-pattern
`backlog.spec.ts` empty-state tests do:
```ts
if (tableRows > 0) {
  test.fail(true, 'Empty-state test requires a clean backlog, ...');
  return; // <-- early return; no assertion actually throws
}
```
Playwright marks the test as "expected to fail." The early `return` means no assertion fires,
so the test actually *passes*. `test.fail(true)` + implicit pass = "Expected to fail, but
passed" → Playwright reports a failure (`class: fail`).

Similarly, `backlog-copy-link.spec.ts`'s `copyLink_should_ShowCopyFailedState` has `test.fail(true)`
at the top; if the "Copy failed" state was added to the UI, all assertions pass → unexpected pass.

Affected: `backlog.spec.ts` (empty-state tests), `backlog-copy-link.spec.ts` (ShowCopyFailed).

### Category C — stale string / selector drift
Expected text or selectors no longer match current component output.

- `backlog-copy-link.spec.ts` `ProduceWellFormedURLWithNoEncodingArtifacts`: calls
  `new URL(clipboardText)` where `clipboardText` starts with `ssq://`; the browser's URL API
  rejects custom schemes → `not.toThrow()` assertion fails.
- `backlog-deep-link-errors.spec.ts` `unreachableHostLink` / `unregisteredHostLink`: use
  `page.getByRole('status')` to find the banner, then assert it contains `OTHER_HOST = 'otherhost'`.
  If the component renders the hostname differently or the element's role changed, the assertion fails.
- Other spec files: stale `data-testid` or text expectations that no longer match current
  component output.

Affected: `backlog-copy-link`, `backlog-deep-link-errors`, possibly others.

### Category D — environment-dependent / best-effort assertions
`backlog-connection-indicator.spec.ts`'s `setOffline` test acknowledges in comments that
`context.setOffline(true)` does not reliably interrupt an already-open streaming fetch in
headless Chromium — the indicator may stay "Live" through the offline window, so the
"Reconnecting" assertion fails.

Affected: `backlog-connection-indicator.spec.ts` (1 test, 2 rows).

## Acceptance Criteria

AC-01: All 11 backlog spec files have zero rows of class `fail` in the Next baseline after
       re-recording. (Skipped / fixme rows are acceptable.)

AC-02: Empty-state tests in `backlog.spec.ts` do not use `test.fail(true)` as a guard.
       Instead they either (a) assert and throw when the precondition is unmet, or (b) each
       spec creates and cleans up its own items, or (c) are consolidated into a spec that
       uses an isolated server state.

AC-03: `backlog-sources-settings.spec.ts` and `backlog-github-sync-settings.spec.ts` delete
       sources they created in `afterEach` or `afterAll`, so each test run starts with no
       stale sources.

AC-04: `backlog-copy-link.spec.ts`'s `ProduceWellFormedURLWithNoEncodingArtifacts` test
       validates ssq:// link structure without calling `new URL()` (which rejects custom
       schemes). The test must pass on a fresh run against a clean server.

AC-05: `backlog-copy-link.spec.ts`'s `ShowCopyFailedState` test is converted to
       `test.skip(true, reason)` if the "Copy failed" UI state remains unimplemented, or the
       `test.fail(true)` annotation is removed if the feature has since been implemented.

AC-06: `backlog-deep-link-errors.spec.ts` locates the error banner correctly and the hostname
       assertion matches the current component output for `unreachableHostLink` and
       `unregisteredHostLink` scenarios.

AC-07: `backlog-connection-indicator.spec.ts`'s `setOffline` test is either (a) made
       deterministic by finding a reliable interrupt path, or (b) demoted to `test.fixme`
       with a documented rationale pointing to the existing comment in the file.

AC-08: Items created by tests that lacked cleanup (`backlog-detail-live-updates`,
       `backlog-manual-override`, `backlog-pipeline-mode`, `backlog-rich-text`) are archived
       or deleted in `afterEach` using `archiveBacklogItemDirect` / `deleteBacklogItemDirect`.

AC-09: No test introduces `waitForTimeout`; all waits use `expect(locator).toHaveXxx` or
       polling assertions per the `e2e-test-conventions` skill.

AC-10: `tests/e2e/baseline/next-e2e-baseline.json` is updated (or a new baseline is recorded)
       so none of the fixed tests appear in the `fail` class list.

## Out of Scope

- Fixing baseline-fail rows in non-backlog spec files (other categories in
  `baseline-fail-triage.json`).
- Implementing the missing "Copy failed" UI state in `BacklogItemDetail.tsx` (that is a
  separate product feature; this project only adjusts the test annotation).
- Fixing the `backlog-edit-buffering.spec.ts` rows (already passing in the current baseline).
