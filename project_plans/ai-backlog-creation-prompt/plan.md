# Implementation Plan: AI Backlog Creation Prompt

## Overview

Restore the AI-powered review step for items created via the "Create backlog item" compose
widget. Single 3-line change in `Omnibar.tsx` + one test update. No backend changes required.

## Tasks

### Task 1 — Fix the onSubmit handler in Omnibar.tsx (30 min)

**File:** `web-app/src/components/sessions/Omnibar.tsx`, lines 1762–1767

Replace:
```tsx
onSubmit={(text) => {
  setBacklogComposeText(null);
  onClose();
  void createBacklogInBackground(text);
}}
```

With:
```tsx
onSubmit={(text) => {
  setBacklogComposeText(null);
  setBacklogReviewText(text);
}}
```

**Why no `onClose()`:** The `backlogReviewText !== null` render branch (lines 1723–1746)
already renders `BacklogItemIntentReview`. Its `onDone` callback calls `onClose()` after
the item is created. Calling `onClose()` before `setBacklogReviewText` would close the
omnibar before the review UI can render.

**Why remove `createBacklogInBackground`:** Routing through `setBacklogReviewText` hands
control to `BacklogItemIntentReview`, which calls `parseBacklogItemIntent` (LLM) and then
`createBacklogItem`. `createBacklogInBackground` is no longer needed in this branch.

### Task 2 — Update the affected compose test (30 min)

**File:** `web-app/src/components/sessions/__tests__/Omnibar.backlogCompose.test.tsx`,
lines 224–253

The test `"closes immediately on Create, then notifies with a deep link when creation finishes"`
validates the old fire-and-forget behavior. After the fix, submitting in the compose widget
opens `BacklogItemIntentReview` instead.

Replace this test with one that asserts:
1. Typing text in `BacklogComposeWidget` and pressing Cmd+Enter (or clicking Create) renders
   `BacklogItemIntentReview` (data-testid: `"backlog-intent-review"`)
2. `BacklogComposeWidget` is no longer visible
3. The omnibar is still open
4. `createBacklogItemFromChat` is NOT called

Keep all other tests in this file unchanged — they cover cancel/Escape, draft preservation,
`backlog:` prefix regression guard, and image attachment, none of which are affected.

### Task 3 — Verify regression guards pass (15 min)

Run the two existing test suites that cover both paths:

```bash
cd web-app && npx jest --testPathPatterns="Omnibar.backlogCompose|Omnibar.backlogIntentReview|BacklogItemIntentReview" --no-coverage
```

Expected: all tests green. Specifically verify:
- `Omnibar.backlogIntentReview.test.tsx`: `"opens the intent review UI instead of creating immediately"` still passes (no regression to the `backlog:` prefix path)
- `BacklogItemIntentReview.test.tsx`: all existing tests pass (component unchanged)

## Implementation Sequence

1. Fix `Omnibar.tsx` (Task 1)
2. Update `Omnibar.backlogCompose.test.tsx` (Task 2)
3. Run tests to verify (Task 3)
4. Manual smoke test: start the app, click "Create backlog item," type a description, submit,
   confirm the parsing spinner appears then the review form pre-fills

## Out of Scope

- The `useBacklogBackgroundCreate` hook and its notification logic — the hook is still used
  by other paths (not specifically the compose widget flow after this fix), so no cleanup needed
  unless a follow-up decides to remove the compose-path-specific code
- Backend changes — `ParseBacklogItemIntent` and `CreateBacklogItem` RPCs are already correct
- The triage gap for compose-created items is resolved as a side effect (items created via
  `createBacklogItem` include the AI-suggested `repoPath` and thus trigger triage)

## Adversarial Review

**Challenge:** What if `setBacklogReviewText` is called but the LLM call takes a long time?
**Answer:** `BacklogItemIntentReview` already handles this with a `ParsingIndicator` spinner
(`phase === "parsing"` state in `useParseBacklogItemIntent`). No new loading state needed.

**Challenge:** What if the user cancels out of the review step? Is the compose text lost?
**Answer:** `BacklogItemIntentReview.onCancel` calls `setBacklogReviewText(null)` which
re-renders the plain omnibar input. The text is gone — but this matches the existing behavior
for the `backlog: <msg>` prefix path. If preserving draft text is desired, that's a separate
UX concern outside this fix's scope.

**Challenge:** Could removing `onClose()` leave the omnibar stuck open if `setBacklogReviewText`
is set but `BacklogItemIntentReview.onDone` never fires (e.g. network error)?
**Answer:** `BacklogItemIntentReview.onCancel` always allows the user to exit. No stuck state.

**Challenge:** Does `createBacklogInBackground` import become unused and cause a build error?
**Answer:** Check if `createBacklogInBackground` is used elsewhere in `Omnibar.tsx` before
removing the import. If the compose path is its only use, remove the import too.

### Adversarial Review Result

**Verdict: PASS (not BLOCKED).** All four challenges above have answers grounded in existing
behavior; none requires a plan change. Two additions made during implementation:
`useBacklogBackgroundCreate` had no other caller (the import-usage challenge), so the hook was
deleted rather than left orphaned, and `docs/reference/feature-testing-registry.md` was updated
to describe the new compose flow.
