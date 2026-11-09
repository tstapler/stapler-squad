# Research: e2e-backlog-flow-fixes

## Codebase Evidence

### Baseline JSON structure
`tests/e2e/baseline/next-e2e-baseline.json` stores specs as `{spec, project, title, class, runs[]}`.
`class` is one of `pass | fail | flaky | skipped`. The 40 rows in category `backlog-flows`
(`tests/e2e/baseline/baseline-fail-triage.json`) are those with `class: fail`.

### Category A — State Pollution: What exists for cleanup

`tests/e2e/pages/BacklogMutations.ts` exports:
- `archiveBacklogItemDirect(request, itemId)` — `POST /api/debug/backlog/mutate-archive`
- `deleteBacklogItemDirect(request, itemId)` — `POST /api/debug/backlog/mutate-delete`
- `createBacklogItemDirect(request, opts)` → returns `itemId` (string)

For sources, the `DeleteItemSource` RPC exists in the backend
(`server/services/backlog_service_lifecycle.go`) and is reachable at
`POST /api/session.v1.BacklogService/DeleteItemSource`. The `BacklogSourcesSettingsPage.ts`
helper exposes a `row(displayName)` locator but no `deleteSource()` method — one must be added,
or callers can call the RPC directly.

The `Triage` describe block in `backlog.spec.ts` already shows the correct cleanup pattern:
```ts
test.afterEach(async ({ request }) => {
  if (!createdItemId) return;
  await request.post(`${BASE_URL}/api/session.v1.BacklogService/ArchiveBacklogItem`, ...);
  createdItemId = undefined;
});
```
All other describe blocks that create items lack this.

For `PipelineModes`, `DeletePipelineMode` RPC exists
(`server/services/backlog_service_pipeline_mode.go`).

### Category B — `test.fail(true)` anti-pattern

Playwright's `test.fail(condition, message)` with `condition = true`:
- Marks the test as "expected to fail".
- If the test body **does** throw (assertion failure), Playwright records it as "passed"
  (expected failure occurred as predicted).
- If the test body **does not** throw, Playwright records "Expected to fail, but passed" →
  `class: fail`.

Pattern in `backlog.spec.ts`:
```ts
if (tableRows > 0) {
  test.fail(true, 'Requires clean backlog, but found N item(s).');
  return; // <-- no assertion fires; test body exits cleanly
}
```
When `tableRows > 0` (shared server has items), `test.fail(true)` is set, `return` runs, no
assertion throws → Playwright sees "expected failure, but test passed" → `class: fail`.

Affected tests (by title):
- `Backlog > Backlog Tour > e2e:backlog-tour-first-visit` — tour fails to appear because
  empty-state detection is broken AND because the `beforeEach` pre-seeds `backlog-onboarded=true`,
  so clearing it in the tour test must still work after earlier tests have run and left items.
- `Backlog > Status Transitions > e2e:backlog-item-appears-in-list-after-creation`
- `Backlog > Status Transitions > e2e:backlog-transition-idea-to-ready`
These may be failing because an earlier test in the same spec run (e.g., `Triage` describe block's
`afterEach` only cleans `createdItemId`, not items created in `Status Transitions`).

Correct fix for the empty-state guard: replace with `expect(tableRows, msg).toBe(0)` so the
test *actually* fails when the precondition is not met, with a clear message. This turns "Expected
to fail but passed" into a genuine assertion failure that surfaces the environment problem clearly.

For `ShowCopyFailed` in `backlog-copy-link.spec.ts`: the comment at lines 73–82 confirms the
"Copy failed" state does not exist in `BacklogItemDetail.tsx:161-171`. If this is still true,
the correct annotation is `test.skip(true, reason)` not `test.fail(true)`. If the UI state was
added, the annotation should be removed entirely.

### Category C — Selector / String Drift

**`backlog-copy-link.spec.ts` `ProduceWellFormedURLWithNoEncodingArtifacts`:**
```ts
expect(() => new URL(clipboardText)).not.toThrow();
```
`new URL("ssq://localhost:8544/backlog/v1/bl_01...")` throws `TypeError: Invalid URL` in both
Node.js and the browser (Chromium's `URL` constructor only accepts registered or parseable
schemes). The test was written assuming an `http://` link, but the current app produces
`ssq://`. Fix: replace with structural parsing:
```ts
expect(clipboardText).toMatch(/^ssq:\/\/[^/]+\/backlog\/v1\/bl_[A-Za-z0-9]+$/);
```
Or parse the ssq URL manually: `new URL(clipboardText.replace(/^ssq:/, 'http:'))`.

**`backlog-deep-link-errors.spec.ts` unreachable/unregistered tests:**
```ts
const banner = page.getByRole('status');
await expect(banner).toContainText(OTHER_HOST); // 'otherhost'
```
`DeepLinkErrorBanner.tsx`'s `copyFor()` switch renders different content for `unreachable` vs
`not-registered` reasons. The `role="status"` element should contain the hostname extracted
from the `ssq://` URL (`otherhost`). If the component now wraps the hostname in a `<code>` or
`<strong>` element that changes accessible text computation, `toContainText` may still work
(it ignores child element boundaries for text matching). But if the component was refactored to
render the hostname differently or removed the `role="status"` attribute, the locator breaks.
The fix is to run the test locally against the actual component and confirm the banner text and
role, then update the locator.

**`backlog-sources-settings.spec.ts` placeholder drift:**
`BacklogSourcesSettingsPage.ts` line 18 uses `getByPlaceholder("GitHub personal access token")`.
Verified against `web-app/src/components/settings/backlogSourceSchemas.ts` line 39:
`tokenLabel: "GitHub personal access token"` — the placeholder is still correct. The failures
are due to state pollution (accumulating sources from prior runs), not selector drift.

### Category D — Environment-Dependent Test

`backlog-connection-indicator.spec.ts` test `indicator flips to Reconnecting`:
The test comment (lines 82–91) explicitly documents that `context.setOffline(true)` does not
reliably interrupt an already-open streaming fetch in headless Chromium, and no other test in
the suite exercises a forced-disconnect scenario. This is inherently flaky.

Recommended action: demote to `test.fixme(true, existing-comment-text)` and reference
GitHub issue tracking the reliable-interrupt problem (or cross-reference the existing comment).

## Infrastructure and Helper Availability

- `deleteBacklogItemDirect` exists in `BacklogMutations.ts` and uses the debug mutate-delete endpoint.
- `archiveBacklogItemDirect` is the lighter alternative (soft-delete, same effect for test isolation).
- `BacklogSourcesSettingsPage.ts` needs a `deleteSource(displayName)` helper that calls
  `DeleteItemSource` RPC. The RPC payload is `{ id: string }` where `id` is the internal UUID
  returned by `ListItemSources`. Alternatively, the page helper can click the "Remove" button in
  the row (which is already exercised by AC-04).
- `PipelineModesSettingsPage` (used in `backlog-pipeline-mode.spec.ts`) needs similar cleanup.

## Related Existing Patterns

The `Triage` describe block in `backlog.spec.ts` demonstrates the correct item-cleanup pattern
with `afterEach` + `ArchiveBacklogItem`. The `backlog-connection-indicator.spec.ts` demonstrates
creating a "guard item" in `beforeEach` without cleanup — this item leaks into the shared server.

## Existing Utilities

- `tests/e2e/pages/BacklogMutations.ts` — mutation helpers (create, update, transition, archive, delete)
- `tests/e2e/pages/BacklogSourcesSettingsPage.ts` — sources settings page object
- `tests/e2e/pages/BacklogPage.ts` — main backlog page object
- `tests/e2e/pages/PipelineModesSettingsPage.ts` — pipeline modes page object (assumed, used in backlog-pipeline-mode.spec.ts)
- `tests/e2e/helpers/base-url.ts` — `baseUrl()`, `slotTmp()`
