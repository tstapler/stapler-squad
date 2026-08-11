# Feature Plan: Review Queue Auto-Advance and Queue Visibility

**Date**: 2026-03-13
**Status**: Draft
**Scope**: Improve review queue UX with auto-advance after actions, queue position indicator, and completion state

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [Research Findings: Current Implementation](#research-findings-current-implementation)
- [ADR-013: Auto-Advance Strategy After Approval Resolution](#adr-013-auto-advance-strategy-after-approval-resolution)
- [ADR-014: Queue Position Tracking Approach](#adr-014-queue-position-tracking-approach)
- [Architecture Overview](#architecture-overview)
- [Story 1: Queue Position Indicator and Completion State](#story-1-queue-position-indicator-and-completion-state)
- [Story 2: Auto-Advance After Approval and Acknowledge Actions](#story-2-auto-advance-after-approval-and-acknowledge-actions)
- [Story 3: Keyboard Navigation and Accessibility](#story-3-keyboard-navigation-and-accessibility)
- [Known Issues and Bug Risks](#known-issues-and-bug-risks)
- [Testing Strategy](#testing-strategy)
- [Dependency Graph](#dependency-graph)

---

## Problem Statement

When a user works through the review queue at `/review-queue/`, they currently must
manually click the next item after resolving an approval or acknowledging a session.
There is no indication of how many items remain, no automatic progression to the next
pending item, and no clear "done" state when all items have been processed. This creates
friction in a workflow where the user is triaging multiple items in rapid succession.

**User goals:**
1. After approving/denying a tool-use request or acknowledging a queue item, automatically
   advance to the next pending item without additional clicks.
2. See a clear progress indicator showing their position in the queue (e.g., "2 of 5 remaining").
3. Know definitively when the queue is empty and all items have been handled.

---

## Research Findings: Current Implementation

### Review Queue Page (`/review-queue/`)

**File**: `web-app/src/app/review-queue/page.tsx`

The page manages `selectedSession` state and a `reviewQueueItems` array. It passes
`handleNextSession` and `handlePreviousSession` to `SessionDetail`, which uses
Shift+ArrowRight/Left keyboard shortcuts. Navigation is circular (wraps at ends).
The page also tracks `queueItems` (full `ReviewItem[]`) for constructing minimal Session
objects before full session data loads.

### ReviewQueuePanel Component

**File**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

Displays the queue list with priority/reason filters, statistics (average age, oldest age),
and item cards. Uses three hooks:
- `useReviewQueue` for data fetching (WebSocket push + 30s fallback polling)
- `useReviewQueueNavigation` for keyboard navigation (`[` / `]` shortcuts)
- `useReviewQueueNotifications` for sound/toast when new items arrive

The panel already has an empty state ("No sessions need attention!") but it is
only shown when `items.length === 0` with no transition or celebration state.

### useReviewQueueNavigation Hook

**File**: `web-app/src/lib/hooks/useReviewQueueNavigation.ts`

Provides `currentIndex`, `goToNext`, `goToPrevious`, `goToIndex`, `hasNext`, `hasPrevious`.
Navigation is circular. Resets index when items change and current index goes out of bounds.
Keyboard shortcuts are `]` (next) and `[` (previous). Does NOT have auto-advance logic.

### SessionDetail with ApprovalPanel

**File**: `web-app/src/components/sessions/SessionDetail.tsx`

The modal shows an `ApprovalPanel` at the top of the terminal tab. Navigation buttons
(left/right arrows) appear when `showNavigation` is true. Keyboard shortcuts are
Shift+ArrowRight/Left. There is NO callback from ApprovalPanel when an approval is
resolved, so the parent cannot react to it.

### ApprovalPanel and ApprovalCard

**Files**:
- `web-app/src/components/sessions/ApprovalPanel.tsx`
- `web-app/src/components/sessions/ApprovalCard.tsx`

ApprovalPanel uses `useApprovals` hook (5s polling) and renders `ApprovalCard` components.
ApprovalCard has Approve/Deny buttons with optimistic removal. Neither component exposes
a callback when an action completes. The `useApprovals` hook calls `resolveApproval` RPC
and optimistically removes the item from local state.

### useApprovals Hook

**File**: `web-app/src/lib/hooks/useApprovals.ts`

Polls `ListPendingApprovals` every 5s. Exposes `approve(id)` and `deny(id, message?)`.
Both perform optimistic removal then RPC call. No event/callback system for external
listeners to know when an action was taken.

### Backend: ApprovalStore

**File**: `server/services/approval_store.go`

Thread-safe in-memory store with `pending` map and `bySession` index. `Resolve()` removes
from store and sends decision on buffered channel. `ListAll()` returns all pending.
No changes needed on the backend -- the existing `ListPendingApprovals` and
`ResolveApproval` RPCs are sufficient.

### Data Model Assessment

No proto or backend changes are needed. The existing `ReviewQueue.total_items`,
`ReviewItem` list, and `PendingApprovalProto` types provide all the data required.
The queue position can be computed client-side from the items array index. The
"remaining" count is simply `items.length` from the `useReviewQueue` hook.

---

## ADR-013: Auto-Advance Strategy After Approval Resolution

### Context

When a user resolves an approval (approve/deny) from within the SessionDetail modal,
or acknowledges a review queue item, we need to decide how to advance to the next item.

### Options Considered

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A. Callback from ApprovalPanel | Add `onResolved` callback prop to ApprovalPanel/ApprovalCard | Simple, follows React patterns, component-level | Only covers approval actions, not acknowledge |
| B. Watch items array for changes | Detect when `reviewQueueItems` shrinks and advance | Covers all removal reasons | Race conditions with optimistic updates; might advance on unrelated removals |
| C. Unified action handler in page | All actions (approve, deny, acknowledge) go through page-level handler that chains advance | Single control point, deterministic | More wiring, but cleanest |

### Decision

**Option C: Unified action handler in page with Option A as the mechanism.**

The review queue page (`page.tsx`) will pass an `onApprovalResolved` callback through
SessionDetail to ApprovalPanel. ApprovalPanel will invoke this callback after a
successful approve/deny. Similarly, the acknowledge button in ReviewQueuePanel will
invoke a callback that triggers advance. The page orchestrates the advance logic
centrally.

The auto-advance will:
1. Wait a brief delay (300ms) so the user sees the action result.
2. Check if there are remaining items in the queue.
3. If yes: navigate to the next item. If the resolved item was the last one, wrap to the first.
4. If no: close the modal and show the completion state.

### Rationale

- Deterministic: advance only happens in response to explicit user actions
- No race conditions from watching array length changes
- Single orchestration point in the page component
- Easy to add a "disable auto-advance" toggle later if needed

---

## ADR-014: Queue Position Tracking Approach

### Context

The user needs to see their position in the queue (e.g., "2 of 5 remaining") while
reviewing items in the SessionDetail modal.

### Options Considered

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A. Derive from reviewQueueItems index | Use `indexOf(selectedSession)` in the items array | Zero state to manage, always consistent | Items may reorder mid-review due to WebSocket updates |
| B. Maintain separate counter state | Track `currentPosition` and `totalAtStart` in page state | Stable during review session | Can drift from reality if queue changes |
| C. Hybrid: derive index, snapshot total | Derive current index from live array, but also show live total | Accurate position and total | Slight complexity |

### Decision

**Option C: Hybrid approach.**

- `currentIndex`: Derived from `reviewQueueItems.findIndex(s => s.id === selectedSession.id)`
- `totalItems`: Live from `reviewQueueItems.length` (reflects real-time queue changes)
- Display format: `"Reviewing 2 of 5"` where 2 is 1-indexed position, 5 is live total

When a new item enters the queue during review, the total updates immediately.
When the current item is resolved and removed, the auto-advance logic moves to the
next item and the indicator updates accordingly.

### Rationale

- Users see accurate, real-time information
- No stale counters that lie about queue state
- Consistent with the existing WebSocket push update model

---

## Architecture Overview

```
ReviewQueuePage (page.tsx)
  |
  |-- [State] selectedSession, reviewQueueItems, queueItems
  |-- [New State] autoAdvanceEnabled (default: true)
  |
  |-- ReviewQueuePanel
  |     |-- useReviewQueue (WebSocket push + polling)
  |     |-- useReviewQueueNavigation (keyboard nav)
  |     |-- [Modified] onAcknowledge callback -> triggers auto-advance
  |     |-- [New] Queue progress bar + empty completion state
  |
  |-- SessionDetail modal (when selectedSession != null)
  |     |-- [New] Queue position indicator: "Reviewing 2 of 5"
  |     |-- [New] onApprovalResolved callback prop
  |     |-- ApprovalPanel
  |     |     |-- useApprovals
  |     |     |-- [Modified] onResolved callback -> bubbles up
  |     |     |-- ApprovalCard (Approve/Deny buttons)
  |     |
  |     |-- TerminalOutput
  |
  |-- [New] CompletionOverlay (shown when queue empties during review)
```

**Data flow for auto-advance:**

1. User clicks "Approve" in ApprovalCard
2. ApprovalCard calls `onApprove()` (existing)
3. `useApprovals.approve()` optimistically removes item, calls RPC
4. ApprovalPanel detects all approvals for this session are resolved
5. ApprovalPanel calls `onResolved()` (new callback)
6. SessionDetail receives it and calls `onApprovalResolved()` (new prop)
7. ReviewQueuePage receives it, waits 300ms, then calls `handleNextSession()`
   or closes the modal if queue is empty

**Data flow for acknowledge auto-advance:**

1. User clicks dismiss button in ReviewQueuePanel
2. `acknowledgeSession()` optimistically removes item
3. ReviewQueuePanel calls `onAcknowledged(sessionId)` (new callback)
4. ReviewQueuePage receives it, waits 300ms, advances to next item

---

## Story 1: Queue Position Indicator and Completion State

**Value**: Users can see how many items remain and know when they are done.

### Task 1.1: Add queue position indicator to SessionDetail header

**Files**:
- `web-app/src/components/sessions/SessionDetail.tsx`
- `web-app/src/components/sessions/SessionDetail.module.css`

**Changes**:
- Add `queuePosition` and `queueTotal` props to `SessionDetailProps`
- Render a position badge between the title and navigation arrows: `"2 of 5"`
- Style the badge with muted color, smaller font, vertically centered with nav buttons
- Only show when `showNavigation` is true and `queueTotal > 0`

**Acceptance Criteria**:
- [ ] Position badge shows "1 of N" format when SessionDetail is opened from review queue
- [ ] Badge updates in real-time when queue items are added or removed
- [ ] Badge is not shown when SessionDetail is opened from other contexts (e.g., main page)
- [ ] Badge is visually consistent with existing header styling

### Task 1.2: Compute and pass queue position from page

**Files**:
- `web-app/src/app/review-queue/page.tsx`

**Changes**:
- Compute `queuePosition` as `reviewQueueItems.findIndex(s => s.id === selectedSession.id) + 1`
- Compute `queueTotal` as `reviewQueueItems.length`
- Pass both to `SessionDetail` as new props
- Handle edge case where `selectedSession` is no longer in `reviewQueueItems` (returns 0)

**Acceptance Criteria**:
- [ ] Position is 1-indexed (human-friendly)
- [ ] Total reflects live queue count
- [ ] When the current session is removed from queue (acknowledged/resolved), position recomputes

### Task 1.3: Enhanced empty state and completion overlay

**Files**:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `web-app/src/components/sessions/ReviewQueuePanel.module.css`
- `web-app/src/app/review-queue/page.tsx`
- `web-app/src/app/review-queue/page.module.css`

**Changes**:
- Track whether the queue was previously non-empty (`hadItems` state in ReviewQueuePage)
- When queue transitions from non-empty to empty, show a "completion" state instead of
  the generic empty state. The completion state includes:
  - A checkmark icon (text-based, not emoji)
  - "All done! 0 items remaining" message
  - Subtle animation (CSS fade-in)
- The generic empty state ("No sessions need attention") remains for cold loads
  where the queue was always empty
- If the SessionDetail modal is open when queue empties, close it after a brief delay (500ms)
  and show the completion state in the panel

**Acceptance Criteria**:
- [ ] Cold load with empty queue shows "No sessions need attention" (existing behavior preserved)
- [ ] Queue draining to empty during active review shows "All done!" completion state
- [ ] Completion state has a visual distinction from the generic empty state (animation, icon)
- [ ] If modal is open when last item is processed, modal auto-closes and completion shows

### Task 1.4: Queue progress summary in panel header

**Files**:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `web-app/src/components/sessions/ReviewQueuePanel.module.css`

**Changes**:
- Replace or augment the existing `Total: {totalItems}` stat with a more actionable
  format: `"5 items need attention"` (or `"1 item needs attention"` for singular)
- When items are being processed (queue was larger and is shrinking), show contextual
  text: `"3 remaining"` rather than just the total

**Acceptance Criteria**:
- [ ] Header shows count in human-readable format
- [ ] Singular/plural grammar is correct
- [ ] Count updates in real-time via WebSocket push

---

## Story 2: Auto-Advance After Approval and Acknowledge Actions

**Value**: Users can process the entire queue without manual navigation between items.

### Task 2.1: Add onResolved callback to ApprovalPanel

**Files**:
- `web-app/src/components/sessions/ApprovalPanel.tsx`
- `web-app/src/components/sessions/ApprovalCard.tsx`

**Changes**:
- Add `onResolved?: (approvalId: string, decision: "allow" | "deny") => void` prop to
  `ApprovalPanel`
- After a successful `approve()` or `deny()` call in ApprovalPanel, invoke `onResolved`
- The callback fires after the optimistic removal succeeds (not on rollback)
- ApprovalCard does not need changes -- it already calls `onApprove`/`onDeny` which
  the panel handles

**Acceptance Criteria**:
- [ ] `onResolved` fires after successful approve action
- [ ] `onResolved` fires after successful deny action
- [ ] `onResolved` does NOT fire if the RPC fails and state is rolled back
- [ ] Existing behavior is preserved when `onResolved` is not provided

### Task 2.2: Thread onApprovalResolved through SessionDetail

**Files**:
- `web-app/src/components/sessions/SessionDetail.tsx`

**Changes**:
- Add `onApprovalResolved?: () => void` prop to `SessionDetailProps`
- Pass it to `ApprovalPanel` as `onResolved`
- When ApprovalPanel reports resolution AND there are no more pending approvals for this
  session, call `onApprovalResolved`

**Acceptance Criteria**:
- [ ] SessionDetail passes callback to ApprovalPanel
- [ ] Callback fires when all approvals for the current session are resolved
- [ ] If session has multiple pending approvals, auto-advance waits until all are resolved
- [ ] If session has no approvals (queue item for other reasons), this path is not triggered

### Task 2.3: Implement auto-advance logic in ReviewQueuePage

**Files**:
- `web-app/src/app/review-queue/page.tsx`

**Changes**:
- Add `handleAutoAdvance()` function:
  ```
  1. Wait 300ms (let user see the result)
  2. If reviewQueueItems.length > 0:
     a. Find next item after the current one
     b. If current item is gone from list, take the item at the same index (or last item)
     c. Navigate to the next item (setSelectedSession, router.push)
  3. If reviewQueueItems.length === 0:
     a. Close the modal (handleCloseSessionDetail)
  ```
- Wire `handleAutoAdvance` to:
  - `onApprovalResolved` prop on SessionDetail
  - A new `onAcknowledged` callback from ReviewQueuePanel (for dismiss actions while modal is open)
- Handle the edge case where the user resolves an approval but the review queue item
  persists (e.g., session has multiple attention reasons). In this case, stay on the
  same session rather than advancing.
- Use `useRef` to hold `reviewQueueItems` so the timeout callback reads current state

**Acceptance Criteria**:
- [ ] After approving/denying the last approval for a session, auto-advances to next queue item
- [ ] After acknowledging an item, auto-advances to next queue item
- [ ] 300ms delay is perceptible but not sluggish
- [ ] When queue empties, modal closes and completion state shows
- [ ] If the resolved session still has other pending reasons, stays on same session
- [ ] Auto-advance respects circular navigation (last item wraps to first)

### Task 2.4: Add auto-advance toggle (optional UX safeguard)

**Files**:
- `web-app/src/app/review-queue/page.tsx`
- `web-app/src/app/review-queue/page.module.css`

**Changes**:
- Add a small toggle/checkbox in the review queue header: "Auto-advance" (default: on)
- Store preference in `localStorage` key `review-queue-auto-advance`
- When disabled, resolving an action stays on the current session (existing behavior)
- Visual indicator: subtle toggle that does not distract from primary workflow

**Acceptance Criteria**:
- [ ] Toggle defaults to enabled
- [ ] Preference persists across page reloads via localStorage
- [ ] When disabled, approve/deny/acknowledge do not trigger auto-advance
- [ ] Toggle is discoverable but not visually dominant

---

## Story 3: Keyboard Navigation and Accessibility

**Value**: Power users can process the entire queue without touching the mouse.

### Task 3.1: Enter key triggers approve for focused approval

**Files**:
- `web-app/src/components/sessions/ApprovalCard.tsx`
- `web-app/src/components/sessions/ApprovalCard.module.css`

**Changes**:
- When an ApprovalCard is the only pending approval for the current session AND the
  SessionDetail modal is focused, pressing Enter triggers "Approve"
- Add a subtle visual indicator (pulsing border or highlight) to show which card will
  receive the Enter keypress
- Guard against Enter triggering approve when the user is typing in an input/textarea
  or when the terminal has focus
- Add Shift+Enter as "Deny" shortcut
- Only active when the SessionDetail modal is open (not in the queue list)

**Acceptance Criteria**:
- [ ] Enter approves the currently focused approval card
- [ ] Shift+Enter denies
- [ ] Shortcuts are inactive when typing in input fields
- [ ] Shortcuts are inactive when terminal has focus
- [ ] Shortcuts are inactive when no approvals are pending
- [ ] Visual indicator shows which card will receive the action
- [ ] ARIA attributes properly describe keyboard behavior

### Task 3.2: Keyboard shortcut help overlay

**Files**:
- `web-app/src/app/review-queue/page.tsx`
- `web-app/src/app/review-queue/page.module.css`

**Changes**:
- Add a `?` keyboard shortcut that shows a help overlay listing all shortcuts:
  - `Enter` - Approve pending request
  - `Shift+Enter` - Deny pending request
  - `Shift+Right` - Next session
  - `Shift+Left` - Previous session
  - `]` - Next queue item
  - `[` - Previous queue item
  - `Esc` - Close modal / Exit fullscreen
  - `?` - Show/hide this help
- Overlay is a simple floating card, dismissible by pressing `?` again or `Esc`

**Acceptance Criteria**:
- [ ] Pressing `?` toggles the help overlay
- [ ] Overlay lists all available shortcuts with descriptions
- [ ] Overlay does not interfere with terminal input
- [ ] Overlay is styled consistently with the rest of the UI

### Task 3.3: ARIA live region for queue updates

**Files**:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `web-app/src/components/sessions/SessionDetail.tsx`

**Changes**:
- Add `aria-live="polite"` region in ReviewQueuePanel that announces queue count changes
  (e.g., "3 items remaining" -> "2 items remaining" -> "All items reviewed")
- Add `aria-live="assertive"` on the SessionDetail position indicator when auto-advance
  moves to a new session (announces "Reviewing session: {name}, 3 of 4")
- Ensure the completion state is announced to screen readers

**Acceptance Criteria**:
- [ ] Screen readers announce queue count decrements
- [ ] Screen readers announce auto-advance navigation
- [ ] Screen readers announce completion state
- [ ] Live regions do not fire on initial page load (only on changes)

---

## Known Issues and Bug Risks

### Bug Risk: Race Condition Between Optimistic Removal and Auto-Advance [SEVERITY: High]

**Description**: The `useApprovals` hook optimistically removes the approval from local
state, but `useReviewQueue` receives the queue update asynchronously via WebSocket. The
auto-advance logic fires after `onResolved`, but `reviewQueueItems` may still contain
the just-resolved session because the WebSocket event has not arrived yet.

**Mitigation**:
- Use the 300ms delay to allow WebSocket events to propagate
- When computing the "next" item for auto-advance, explicitly exclude the just-resolved
  session ID from the candidates (pass `excludeSessionId` to the advance function)
- If after 300ms the item is still in the list, advance anyway (treat it as stale)

**Files Likely Affected**:
- `web-app/src/app/review-queue/page.tsx`

**Prevention Strategy**:
- `handleAutoAdvance(resolvedSessionId: string)` receives the ID of the session that was
  just resolved, and filters it out when finding the next item

### Bug Risk: Modal Closes Prematurely When New Item Arrives [SEVERITY: Medium]

**Description**: If the user resolves the last item in the queue (triggering modal close),
but a new item arrives via WebSocket in the same event loop tick, the modal might close
and immediately reopen, causing a visual flash.

**Mitigation**:
- The 300ms delay provides a natural debounce window
- After the delay, re-check `reviewQueueItems.length` before deciding to close
- If new items arrived during the delay, navigate to them instead of closing

**Files Likely Affected**:
- `web-app/src/app/review-queue/page.tsx`

**Prevention Strategy**:
- Use `useRef` to hold the latest `reviewQueueItems` so the timeout callback reads
  current state, not stale closure state

### Bug Risk: Keyboard Shortcut Conflict with Terminal Input [SEVERITY: High]

**Description**: The Enter key shortcut for approving requests will conflict with terminal
input when the TerminalOutput component has focus. Users typing in the terminal could
accidentally approve a pending request.

**Mitigation**:
- Check `document.activeElement` before processing keyboard shortcuts
- The TerminalOutput component uses xterm.js which captures keyboard events in its own
  container. Check if the active element is within the terminal container.
- Approval keyboard shortcuts should only activate when a non-terminal element is focused
  or when the approval card itself has focus

**Files Likely Affected**:
- `web-app/src/components/sessions/ApprovalCard.tsx`
- `web-app/src/components/sessions/SessionDetail.tsx`

**Prevention Strategy**:
- Use a ref on the terminal container and check `terminalRef.current.contains(document.activeElement)`
- Only bind Enter shortcut when approval panel has an actionable item AND terminal is not focused
- Add integration test that simulates terminal focus + Enter and verifies no approval is triggered

### Bug Risk: Stale Closure in setTimeout Auto-Advance [SEVERITY: Medium]

**Description**: The 300ms `setTimeout` in `handleAutoAdvance` captures `reviewQueueItems`
from the closure at the time it was created. By the time it fires, the items array may
have changed (new items added/removed via WebSocket).

**Mitigation**:
- Store `reviewQueueItems` in a `useRef` that is always up-to-date
- Read from the ref inside the timeout callback instead of the closure

**Files Likely Affected**:
- `web-app/src/app/review-queue/page.tsx`

**Prevention Strategy**:
```tsx
const reviewQueueItemsRef = useRef(reviewQueueItems);
useEffect(() => { reviewQueueItemsRef.current = reviewQueueItems; }, [reviewQueueItems]);

const handleAutoAdvance = (resolvedSessionId: string) => {
  setTimeout(() => {
    const currentItems = reviewQueueItemsRef.current;
    // ... use currentItems instead of reviewQueueItems
  }, 300);
};
```

### Bug Risk: Multiple Auto-Advances from Multiple Approvals [SEVERITY: Medium]

**Description**: If a session has 3 pending approvals and the user approves all 3 in quick
succession, each approval resolution might trigger `onResolved`, resulting in 3 auto-advance
calls that race with each other.

**Mitigation**:
- ApprovalPanel should only call `onResolved` when ALL pending approvals for the session
  are resolved (not on each individual resolution)
- Track `approvals.length` in ApprovalPanel and fire `onResolved` when it transitions
  from >0 to 0

**Files Likely Affected**:
- `web-app/src/components/sessions/ApprovalPanel.tsx`

**Prevention Strategy**:
- Use `useEffect` watching `approvals.length === 0` with a guard for the previous length
  being > 0. This fires exactly once when the last approval is resolved.

### Bug Risk: Auto-Advance After Deny with Message Shows Stale UI [SEVERITY: Low]

**Description**: When the user denies an approval with a custom message, the deny modal
or input field may still be visible when auto-advance fires, creating a visual glitch.

**Mitigation**:
- The current ApprovalCard does not have a deny message input (it calls `deny(id)` without
  a message parameter). If a deny message feature is added later, ensure the message
  input is dismissed before auto-advance triggers.

**Files Likely Affected**:
- `web-app/src/components/sessions/ApprovalCard.tsx`

**Prevention Strategy**:
- Keep the 300ms delay, which gives time for any transient UI to settle

---

## Testing Strategy

### Unit Tests

| Test Case | Component | Description |
|-----------|-----------|-------------|
| Position indicator renders correctly | SessionDetail | Verify "2 of 5" badge renders with correct props |
| Position indicator hidden when not in queue | SessionDetail | Verify badge hidden when `queuePosition=0` |
| Auto-advance computes next item | page.tsx | Verify next item selection after removal |
| Auto-advance wraps around | page.tsx | Verify circular navigation at end of list |
| Auto-advance closes modal on empty | page.tsx | Verify modal closes when last item resolved |
| Auto-advance excludes resolved session | page.tsx | Verify resolved session ID is skipped |
| Completion state shows after drain | ReviewQueuePanel | Verify "All done" state after queue empties during review |
| Generic empty state on cold load | ReviewQueuePanel | Verify standard empty state when queue was never populated |
| Singular/plural text | ReviewQueuePanel | Verify "1 item" vs "2 items" grammar |
| onResolved fires once for multiple approvals | ApprovalPanel | Verify callback fires once when all approvals cleared |
| Keyboard Enter triggers approve | ApprovalCard | Verify Enter keydown triggers approve |
| Keyboard ignored in terminal | ApprovalCard | Verify Enter in terminal does not trigger approve |

### Integration Tests

| Test Case | Description |
|-----------|-------------|
| Full approve-and-advance flow | Open item, approve, verify auto-advance to next item |
| Full deny-and-advance flow | Open item, deny, verify auto-advance to next item |
| Acknowledge-and-advance flow | Dismiss item from list, verify selection moves to next |
| Queue drain to completion | Process all items, verify completion state appears |
| WebSocket update during review | New item arrives while reviewing, verify counter updates |
| Auto-advance toggle off | Disable toggle, approve, verify no auto-advance |

### Edge Case Tests

| Test Case | Description |
|-----------|-------------|
| Single item queue | Approve only item, verify modal closes and completion shows |
| Rapid successive approvals | Approve 3 items in <500ms, verify no double-advance |
| Network failure during approve | RPC fails, verify rollback and no advance |
| Session detail opened from non-queue context | Verify no position indicator, no auto-advance |

---

## Dependency Graph

```
Story 1 (Position Indicator + Completion State)
  |
  |-- Task 1.1: Position indicator in SessionDetail
  |-- Task 1.2: Compute position in page.tsx (depends on 1.1)
  |-- Task 1.3: Completion state (independent)
  |-- Task 1.4: Queue progress summary (independent)

Story 2 (Auto-Advance) -- depends on Story 1.2 for position awareness
  |
  |-- Task 2.1: onResolved callback in ApprovalPanel
  |-- Task 2.2: Thread callback through SessionDetail (depends on 2.1)
  |-- Task 2.3: Auto-advance logic in page.tsx (depends on 2.2, 1.2)
  |-- Task 2.4: Auto-advance toggle (depends on 2.3)

Story 3 (Keyboard + Accessibility) -- depends on Story 2.1 for approval callbacks
  |
  |-- Task 3.1: Enter key shortcut for approve (depends on 2.1)
  |-- Task 3.2: Keyboard shortcut help overlay (independent)
  |-- Task 3.3: ARIA live regions (depends on 1.3, 1.4)
```

**Recommended implementation order:**
1. Task 1.1 + 1.3 + 1.4 (parallel, no dependencies)
2. Task 1.2 (needs 1.1)
3. Task 2.1 (needs understanding of 1.x but no code dependency)
4. Task 2.2 (needs 2.1)
5. Task 2.3 (needs 2.2 + 1.2, this is the core feature)
6. Task 2.4 + 3.1 + 3.2 + 3.3 (parallel polish tasks)

---

## Integration Checkpoints

### Checkpoint 1: After Story 1

- [ ] Open review queue page with items, verify position shows in header
- [ ] Process all items manually, verify completion state
- [ ] Refresh page with empty queue, verify generic empty state (not completion)
- [ ] Verify no backend changes were needed

### Checkpoint 2: After Story 2

- [ ] Approve an item and verify auto-advance to next
- [ ] Deny an item and verify auto-advance to next
- [ ] Acknowledge from list and verify auto-advance
- [ ] Process last item and verify modal closes + completion shows
- [ ] Turn off auto-advance toggle and verify manual navigation still works
- [ ] Verify no regressions in approval flow (RPC still works correctly)

### Checkpoint 3: After Story 3

- [ ] Use only keyboard to process entire queue (Enter to approve, Shift+Right to advance)
- [ ] Verify `?` help overlay shows and dismisses
- [ ] Test with screen reader (VoiceOver on macOS)
- [ ] Verify no keyboard conflicts with terminal input
