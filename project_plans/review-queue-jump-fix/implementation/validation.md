# Validation Plan: Review Queue Auto-Advance Bug Fix

## Problem Statement

The "deleted externally" auto-advance `useEffect` in `ReviewQueueContent` (`web-app/src/app/review-queue/page.tsx`, lines 236–243) checks `reviewQueueItems` — a derived list built from `queueItems` × `sessions` — to determine whether the selected session is still in the queue. When a session transitions to `ACTIVE` or `PROCESSING`, the live session data (`sessions`) changes, causing `reviewQueueItems` to be recomputed. If the live-status overlay causes the selected session to fall out of the filtered view (e.g., because `selectReviewQueueItemsWithLiveStatus` filters on status), the effect fires auto-advance even though the session is still legitimately in the review queue.

**Fix**: Change the existence check to use `allQueueItems` sourced from `useReviewQueueContext().items` (the Redux store's `liveItems`), which only changes when actual `addItem` / `removeItem` events fire — not when session status transitions occur.

## Requirements Coverage

| ID | Requirement | Test Count | Test Names |
|----|-------------|------------|------------|
| REQ-1 | Status transition (ACTIVE / PROCESSING) does NOT trigger auto-advance | 3 | T-AA-001, T-AA-002, T-AA-003 |
| REQ-2 | Genuine removal from review queue DOES trigger auto-advance | 3 | T-AA-004, T-AA-005, T-AA-006 |
| REQ-3 | Regression: behavior stable across all non-removal working states | 1 | T-AA-007 (parameterized, 4 cases) |

Total: **7 test cases** (10 assertions counting parameterized sub-cases)

---

## Test File

`web-app/src/app/review-queue/__tests__/ReviewQueueContent.auto-advance.test.tsx`

### Test Stack

- **Framework**: Jest + React Testing Library (RTL)
- **Environment**: jsdom (per `jest.config.js`)
- **Mocked modules**: `next/navigation`, `@/lib/contexts/ReviewQueueContext`, `@/lib/contexts/SessionServiceContext`, `@/components/sessions/ReviewQueuePanel`, `@/components/sessions/SessionDetail`, `@/lib/hooks/useFocusTrap`, `@/lib/hooks/useKeyboard`, `@/lib/analytics/usePageView`

### Why unit tests, not e2e

The timing-sensitive `setTimeout(300ms)` in `handleAutoAdvance` can be controlled with `jest.useFakeTimers()` in unit tests. Playwright e2e tests cannot reliably distinguish "navigation fired" from "navigation never fired" within a 300 ms window without flake.

---

## Test Specifications

### T-AA-001 — Status transition to ACTIVE does not auto-advance (REQ-1 happy path)

**Setup**:
1. `allQueueItems` (from `useReviewQueueContext().items`) contains session `"s1"` throughout.
2. `reviewQueueItems` initially contains `"s1"`.
3. `selectedSession` = session `"s1"`.
4. Render `ReviewQueueContent`.
5. Simulate `reviewQueueItems` update that removes `"s1"` (e.g., session `"s1"` transitions to `SESSION_STATUS_ACTIVE` causing live-status filter to drop it from the derived list).
6. `allQueueItems` still contains `"s1"` (queue item not actually removed).

**Assert**: `router.push` is NOT called with any `?session=` navigation URL after advancing timers.

**Test ID**: `T-AA-001`
**Name**: `should_not_autoAdvance_when_selectedSession_transitionsToActive`

---

### T-AA-002 — Status transition to PROCESSING does not auto-advance (REQ-1 edge path)

**Setup**: Same as T-AA-001 but the live-status change that causes `reviewQueueItems` recomputation is due to `SESSION_STATUS_PROCESSING` (or a processing sub-status).

**Assert**: `router.push` is NOT called after advancing timers by 400 ms.

**Test ID**: `T-AA-002`
**Name**: `should_not_autoAdvance_when_selectedSession_transitionsToProcessing`

---

### T-AA-003 — Auto-advance disabled preference does not advance (REQ-1 guard)

**Setup**:
1. `localStorage.setItem("review-queue-auto-advance", "false")` before render.
2. `allQueueItems` still contains `"s1"` (queue item present).
3. `reviewQueueItems` drops `"s1"` (status change).

**Assert**: `router.push` is NOT called — confirms the `autoAdvanceRef` guard and the `allQueueItems` check both protect against false firing.

**Test ID**: `T-AA-003`
**Name**: `should_not_autoAdvance_when_autoAdvancePreferenceIsDisabled`

---

### T-AA-004 — Genuine removal fires auto-advance to next item (REQ-2 happy path)

**Setup**:
1. `allQueueItems` = `["s1", "s2"]`; `reviewQueueItems` = sessions for `["s1", "s2"]`.
2. `selectedSession` = `"s1"`.
3. Render.
4. Simulate removal: update `allQueueItems` to `["s2"]` and `reviewQueueItems` to `["s2"]`.
5. Advance timers by 400 ms.

**Assert**: `router.push` is called with `/review-queue?session=s2`.

**Test ID**: `T-AA-004`
**Name**: `should_autoAdvance_to_nextItem_when_selectedSession_genuinelyRemoved`

---

### T-AA-005 — Queue empties after removal, modal closes (REQ-2 edge path)

**Setup**:
1. `allQueueItems` = `["s1"]`; `reviewQueueItems` = `["s1"]`.
2. `selectedSession` = `"s1"`.
3. Simulate removal: `allQueueItems` = `[]`, `reviewQueueItems` = `[]`.
4. Advance timers.

**Assert**: `router.push` is called with `/review-queue` (no `?session=`).

**Test ID**: `T-AA-005`
**Name**: `should_closeModal_when_queueEmptiesAfterRemoval`

---

### T-AA-006 — Acknowledge via panel triggers auto-advance (REQ-2 integration path)

**Setup**:
1. Two queue items `["s1", "s2"]`.
2. `selectedSession` = `"s1"`.
3. Call `handleAcknowledged("s1")` directly (via the `onAcknowledged` prop captured from `ReviewQueuePanel` mock).
4. Advance timers.

**Assert**: `router.push` is called with `/review-queue?session=s2`.

**Test ID**: `T-AA-006`
**Name**: `should_autoAdvance_when_acknowledgedFromPanel`

---

### T-AA-007 — Parameterized regression: status transitions do not auto-advance (REQ-3)

**Test cases** (one `it.each` row per case):

| Case | Status simulated | `allQueueItems` contains selected? | Expected router call |
|------|-----------------|-------------------------------------|----------------------|
| A | ACTIVE (live update) | yes | none |
| B | PROCESSING sub-status | yes | none |
| C | session re-appears in `reviewQueueItems` after re-filter | yes | none |
| D | multiple status flips (ACTIVE → WAITING → ACTIVE) | yes | none |

**Assert per case**: `router.push` is NOT called after each status flip.

**Test ID**: `T-AA-007`
**Name**: `should_not_autoAdvance_for_statusTransition_<case>`

---

## Mock Strategy

### `next/navigation`

```ts
const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
  useRouter: () => ({ push: mockPush }),
}));
```

### `useReviewQueueContext`

Mocked with `jest.fn()` so individual tests can call `mockUseReviewQueueContext.mockReturnValue(...)` to control `items` and `acknowledgeSession` independently.

```ts
jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: jest.fn(),
}));
```

### `useSessionServiceContext`

Returns stable `sessions` array and a no-op `runOneShot`. Tests that need to trigger session-status changes rerender with a new `sessions` value.

```ts
jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: () => ({
    sessions: [],
    runOneShot: jest.fn().mockResolvedValue(null),
  }),
}));
```

### `ReviewQueuePanel`

Replaced with a minimal stub that captures `onItemsChange` and `onAcknowledged` props so tests can call them directly to simulate queue events.

```ts
let capturedOnItemsChange: ((items: ReviewItem[]) => void) | undefined;
let capturedOnAcknowledged: ((id: string) => void) | undefined;

jest.mock("@/components/sessions/ReviewQueuePanel", () => ({
  ReviewQueuePanel: (props: {
    onItemsChange?: (items: ReviewItem[]) => void;
    onAcknowledged?: (id: string) => void;
  }) => {
    capturedOnItemsChange = props.onItemsChange;
    capturedOnAcknowledged = props.onAcknowledged;
    return <div data-testid="review-queue-panel" />;
  },
}));
```

### `SessionDetail`

Replaced with minimal stub — the modal render is irrelevant to auto-advance behavior.

```ts
jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: () => <div data-testid="session-detail" />,
}));
```

### Timer control

```ts
beforeEach(() => { jest.useFakeTimers(); });
afterEach(() => { jest.useRealTimers(); });
```

Advance timers: `act(() => { jest.advanceTimersByTime(400); });`

---

## Render Helper

```ts
function makeSession(id: string, status = SessionStatus.UNSPECIFIED): Session { ... }
function makeReviewItem(sessionId: string): ReviewItem { ... }

function renderContent({
  allQueueItems = [] as ReviewItem[],
  sessions = [] as Session[],
  selectedSessionId = null as string | null,
} = {}) {
  mockUseReviewQueueContext.mockReturnValue({
    items: allQueueItems,
    acknowledgeSession: jest.fn().mockResolvedValue(undefined),
    ...
  });
  mockUseSessionServiceContext.mockReturnValue({ sessions, runOneShot: jest.fn() });
  return render(<ReviewQueueContent />);
}
```

Rerender to simulate state changes:

```ts
const { rerender } = renderContent({ allQueueItems: [item1, item2] });
// ... interact ...
rerender(/* updated context values via mockReturnValue */);
```

---

## Acceptance Criteria

All 7 test cases must pass before the PR is merged. Zero `test.skip` or `xit` stubs are permitted.

CI command: `cd web-app && npx jest --no-coverage --testPathPatterns="ReviewQueueContent.auto-advance"`
