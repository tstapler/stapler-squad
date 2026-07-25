# Implementation Plan: review-queue-jump-fix

**Feature**: Fix review queue auto-advance on session status transition
**Date**: 2026-06-23
**Status**: Ready for implementation
**ADRs**: None

---

## Problem Statement

The "deleted externally" auto-advance effect (lines 236–243 of `page.tsx`) uses `reviewQueueItems` — the _filtered visible queue_ — as its existence oracle. When a session transitions to ACTIVE or PROCESSING it is filtered out of the visible queue but **stays in the Redux store** (`allItems`). This makes the effect incorrectly fire auto-advance, jumping the user away from the session they are currently reviewing.

The fix is a two-line change: use `allItems` from `useReviewQueueContext().items` as the existence oracle, while keeping `reviewQueueItems` as the effect dependency.

---

## Dependency Visualization

```
useReviewQueueContext()
  └── items: ReviewItem[]          ← allQueueItems (ground-truth Redux state)
        ↕ item.sessionId === selectedSession.id
  └── acknowledgeSession()         ← already destructured (no change)

reviewQueueItems: Session[]        ← filtered visible queue
  ↕ stays as [reviewQueueItems] dep (effect fires when visible queue changes)
```

```
Effect trigger:  reviewQueueItems changes (dep)
Effect guard:    allQueueItems.some(i => i.sessionId === selected.id)
                 ↑ TRUE  → session still exists somewhere (filtered or active) → do nothing
                 ↑ FALSE → session genuinely removed from queue → auto-advance
```

---

## Phase 1: Fix and Test

### Epic 1.1: Core fix in page.tsx

**Story 1.1.1** — Extend the `useReviewQueueContext()` destructure

- **File**: `web-app/src/app/review-queue/page.tsx` line 67
- **Task**: Add `items: allQueueItems` to the existing destructure:
  ```ts
  // Before
  const { acknowledgeSession } = useReviewQueueContext();

  // After
  const { acknowledgeSession, items: allQueueItems } = useReviewQueueContext();
  ```
- No other imports needed. `ReviewItem` is already imported from `@/gen/session/v1/types_pb` (line 7).

**Story 1.1.2** — Change the existence oracle in the "deleted externally" effect

- **File**: `web-app/src/app/review-queue/page.tsx` lines 236–243
- **Task**: Replace `reviewQueueItems.some(s => s.id === selectedSession.id)` with `allQueueItems.some(item => item.sessionId === selectedSession.id)`:
  ```ts
  // Before
  useEffect(() => {
    if (!selectedSession) return;
    const stillInQueue = reviewQueueItems.some((s) => s.id === selectedSession.id);
    if (!stillInQueue) {
      handleAutoAdvance(selectedSession.id, true);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reviewQueueItems]);

  // After
  useEffect(() => {
    if (!selectedSession) return;
    const stillInQueue = allQueueItems.some((item) => item.sessionId === selectedSession.id);
    if (!stillInQueue) {
      handleAutoAdvance(selectedSession.id, true);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reviewQueueItems]);
  ```
- The `[reviewQueueItems]` dependency array is intentionally preserved — the effect only needs to re-run when the visible queue changes, not on every WatchSessions heartbeat.

### Epic 1.2: Test coverage for auto-advance behavior

**Story 1.2.1** — Create test file for `ReviewQueueContent` auto-advance

- **File**: `web-app/src/app/review-queue/__tests__/ReviewQueueContent.autoadvance.test.tsx` (new file)
- **Task**: Scaffold the test file with required mocks for:
  - `useReviewQueueContext` — mock `acknowledgeSession` and `items`
  - `useSessionServiceContext` — mock `sessions` and `runOneShot`
  - `useRouter` / `useSearchParams` — mock Next.js navigation
  - `localStorage` — `getItem('review-queue-auto-advance')` returns `'true'`

**Story 1.2.2** — Test: status transition to ACTIVE does NOT trigger auto-advance

- **Test name**: `ReviewQueueContent_should_NOT_autoAdvance_when_sessionTransitionsToActive`
- **Setup**:
  - `allQueueItems` contains the selected session (`sessionId === 'sess-1'`)
  - `reviewQueueItems` does NOT contain the session (simulating status filter removed it)
  - `selectedSession` is `sess-1`
- **Assertion**: `router.push` is NOT called; `selectedSession` is NOT changed.
- **Why this matters**: This is the bug scenario. The visible queue loses the session but it still exists in `allQueueItems`, so no advance should happen.

**Story 1.2.3** — Test: genuine `removeItem` dispatch DOES trigger auto-advance

- **Test name**: `ReviewQueueContent_should_autoAdvance_when_sessionGenuinelyRemovedFromQueue`
- **Setup**:
  - `allQueueItems` does NOT contain `sess-1` (it was removed via `removeItem` dispatch)
  - `reviewQueueItems` does NOT contain `sess-1` (consistent — also gone from visible queue)
  - `reviewQueueItems` has one other session `sess-2`
  - `selectedSession` is `sess-1`
- **Assertion**: `router.push` is called with `/review-queue?session=sess-2` within the 300ms `setTimeout` window (use `jest.useFakeTimers()` + `jest.runAllTimers()`).

**Story 1.2.4** — Test: modal closed when queue is genuinely empty after removal

- **Test name**: `ReviewQueueContent_should_closeModal_when_queueEmptyAfterGenuineRemoval`
- **Setup**:
  - `allQueueItems` is empty
  - `reviewQueueItems` is empty
  - `selectedSession` is `sess-1`
- **Assertion**: `router.push('/review-queue')` called; `selectedSession` cleared (modal closed).

---

## Key Implementation Notes

1. **Field name mismatch**: `ReviewItem.sessionId` vs `Session.id` — both hold the same session ID string but with different field names. The existing code on lines 113–114 already navigates this correctly. The fix must use `item.sessionId` when accessing `ReviewItem` objects.

2. **Effect dependency is intentional**: The `// eslint-disable-next-line react-hooks/exhaustive-deps` comment on line 242 is intentional — `allQueueItems` is deliberately excluded from the dep array because we only want to fire when the visible queue changes, not on every Redux update. Do not remove the comment or add `allQueueItems` to the dep array.

3. **Dismiss flow is safe**: `acknowledgeSession` dispatches `removeItem` _before_ returning, so by the time the `[reviewQueueItems]` effect fires after a dismiss, `allQueueItems` will already lack the dismissed session. No double-advance risk.

4. **Test timer strategy**: `handleAutoAdvance` wraps its logic in `setTimeout(..., 300)`. Tests must use `jest.useFakeTimers()` and `act(() => jest.runAllTimers())` to observe the auto-advance effects synchronously.

5. **Scope**: No proto changes, no Redux changes, no other component changes. This is a two-line change to `page.tsx` plus a new test file.

---

## Files Modified

| File | Change |
|---|---|
| `web-app/src/app/review-queue/page.tsx` | +1 field in destructure, +1 changed guard expression |
| `web-app/src/app/review-queue/__tests__/ReviewQueueContent.autoadvance.test.tsx` | New test file (4 tests) |

---

## Validation Checklist

- [ ] `make build` passes (TypeScript must compile cleanly)
- [ ] `cd web-app && npx jest --no-coverage --testPathPatterns="ReviewQueueContent.autoadvance"` — all 4 tests green
- [ ] `make quick-check` passes
- [ ] Manual smoke: open a session in the review queue modal, approve it in another way that transitions it to ACTIVE — confirm modal stays on the session (does not jump)
- [ ] Manual smoke: delete a session externally — confirm modal advances to next session
