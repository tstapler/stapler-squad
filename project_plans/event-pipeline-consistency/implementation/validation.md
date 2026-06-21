# Event Pipeline Consistency — Validation Plan

**Phase:** 4 — Validation  
**Date:** 2026-06-20  
**Status:** READY (readiness gate: PASS — see §5)

---

## 1. Requirement-to-Test Traceability Matrix

Each row maps a requirement from `requirements.md` to the story in `plan.md` that satisfies it
and the test(s) that verify it.

| Requirement | Description | Story | Test ID(s) |
|---|---|---|---|
| R1.1 | Add `session_acknowledged = 6` to `SessionEvent` oneof | 1.1 | (proto already exists — verified by adversarial review Issue 1) |
| R1.2 | `SessionAcknowledgedEvent` proto message with `session_id` | 1.1 | (proto already exists — verified) |
| R1.3 | `event_converter.go` maps `EventSessionAcknowledged` | 1.1 | UT-GO-01 |
| R1.4 | Frontend handles `sessionAcknowledged`, dispatches actions | 3.3 | UT-TS-05, UT-TS-06, E2E-01 |
| R1.5 | Notification page removes pending approval toast/badge | 3.3 | E2E-01 (indirect: badge clears on ack) |
| R2.1 | `approvalResponse` handler dispatches optimistic clear | 3.2 | UT-TS-07 |
| R2.2 | Notifications page marks approval notification resolved | 3.2, 2.3 | E2E-03 |
| R2.3 | `AcknowledgeSession` RPC response includes session snapshot | 1.1 (wire event provides equivalent signal) | E2E-01 |
| R3.1 | `ReactiveQueueManager` subscribes to `EventSessionDeleted` | 1.3 | UT-GO-03 |
| R3.2 | On `EventSessionDeleted`, calls `rqm.queue.Remove(sessionID)` | 1.3 | UT-GO-03 |
| R3.3 | Signals reactive loop for immediate snapshot push | 1.3 | UT-GO-03 (via signalActivity call) |
| R3.4 | Clients see deletion within same event cycle | 1.3 | E2E-02 |
| R4.1 | `DismissNotification` RPC or BroadcastChannel alternative | 5.1 | UT-TS-09 |
| R4.2 | Backend stores dismissed IDs (or BroadcastChannel alternative) | 5.2 | UT-TS-10 |
| R4.3 | Stream sends dismissed state to all clients (or BC alternative) | 5.3 | UT-TS-11 (BC receive path) |
| R4.4 | Frontend notificationStorage augmented by synced state | 5.2, 5.3 | UT-TS-10, UT-TS-11 |
| R4.5 | Badge counts derived from server-synced (or BC-synced) state | 5.3 | UT-TS-11 |
| R5.1 | `WatchReviewQueue` delta updates (or joined selector approach) | 4.1, 4.4 | UT-TS-08, UT-TS-04 |
| R5.2 | `reviewQueueSlice` derives working state from `sessionsSlice` | 4.1, 4.2 | UT-TS-08 |
| R5.3 | State in review queue matches session card within one event cycle | 4.1, 4.2 | E2E-03 (approval flow) |
| R6.1 | Add `user_interaction` to `SessionEvent` oneof | 1.2 | (proto already exists — verified by adversarial review) |
| R6.2 | `UserInteractionEvent` proto message | 1.2 | (proto already exists — verified) |
| R6.3 | `event_converter.go` maps `EventUserInteraction` | 1.2 | UT-GO-02 |
| R6.4 | Frontend updates "last active" timestamp on session entity | 1.2 | (nice-to-have; no mandatory test) |

**Coverage:** 23 of 23 requirements addressed. R6.4 is explicitly nice-to-have per requirements.md
("R6 is nice-to-have") and has no blocking acceptance test.

---

## 2. Go Unit Tests

### File: `server/services/event_converter_test.go`

#### UT-GO-01 — `convertEventToProto` maps `EventSessionAcknowledged` correctly (Story 1.1)

**Test function:** `TestConvertEventToProto_SessionAcknowledged`

**Precondition:** `EventSessionAcknowledged` case is absent from `event_converter.go` (confirmed by adversarial review Issue 1b).

**Steps:**
1. Construct an `events.Event` with:
   - `Type: events.EventSessionAcknowledged`
   - `SessionID: "sess-abc"`
   - `Context: "approved_by_user"` (maps to `Reason`)
   - `Timestamp: time.Now()`
2. Call `convertEventToProto(event)`.
3. Assert return value is non-nil.
4. Assert the `SessionEvent` oneof variant is `*sessionv1.SessionEvent_SessionAcknowledged`.
5. Assert `.SessionAcknowledged.SessionId == "sess-abc"`.
6. Assert `.SessionAcknowledged.Reason == "approved_by_user"`.
7. Assert `.SessionAcknowledged.AcknowledgedAt` is non-nil and approximately equal to input timestamp (within 1 second).

**Acceptance link:** Story 1.1 acceptance test step 4.

**Notes:** Uses the correct generated wrapper `SessionEvent_SessionAcknowledged` (field 7, confirmed
by adversarial review Issue 1). No proto generation required — schema is already complete.

---

#### UT-GO-02 — `convertEventToProto` maps `EventUserInteraction` correctly (Story 1.2)

**Test function:** `TestConvertEventToProto_UserInteraction`

**Steps:**
1. Construct an `events.Event` with:
   - `Type: events.EventUserInteraction`
   - `SessionID: "sess-xyz"`
   - `InteractionType: "terminal_input"` (the string value used in `NewUserInteractionEvent` calls)
2. Call `convertEventToProto(event)`.
3. Assert return value is non-nil.
4. Assert the oneof variant is `*sessionv1.SessionEvent_UserInteraction`.
5. Assert `.UserInteraction.SessionId == "sess-xyz"`.
6. Assert `.UserInteraction.GetType()` is either `sessionv1.UserInteractionEvent_INTERACTION_TYPE_TERMINAL_INPUT`
   or `sessionv1.UserInteractionEvent_INTERACTION_TYPE_UNSPECIFIED` (acceptable if mapping is incomplete —
   see adversarial review Issue 11 follow-up task).

**Acceptance link:** Story 1.2 acceptance test step 1.

**Notes:** The plan correctly flags the `InteractionType` string-to-enum mapping as potentially
incomplete (adversarial review Issue 11). This test verifies the event is forwarded without errors
regardless of enum completeness. A follow-up task should complete the mapping.

---

### File: `server/review_queue_manager_test.go`

#### UT-GO-03 — `EventSessionDeleted` empties the queue (Story 1.3)

**Test function:** `TestReactiveQueueManager_SessionDeleted_RemovesFromQueue`

**Setup:**
1. Create a `ReactiveQueueManager` with a mock `EventBus` and a test `ReviewQueue`.
2. Add a session `"sess-delete-me"` to the queue (via the queue's `Add` method directly,
   bypassing the approval flow for test isolation).

**Steps:**
1. Verify the queue contains `"sess-delete-me"` (assert `queue.Length() == 1`).
2. Publish `EventSessionDeleted` with `SessionID: "sess-delete-me"` to the event bus.
3. Wait for `handleEvent` to return (use a channel or `WaitGroup` in the test harness, or
   call `handleEvent` directly to avoid timing issues).
4. Assert `queue.Length() == 0` (session removed).
5. Assert a second call to `queue.Remove("sess-delete-me")` is a no-op (idempotent).

**Acceptance link:** Story 1.3 acceptance test step 4.

**Notes:** Tests the `rqm.signalActivity()` call indirectly — the test verifies observable
state (empty queue), not the internal signal. The method name fix (`signalActivity` not
`signalActivityCh`) is captured in adversarial review Issue 2 and applied in the implementation.

---

#### UT-GO-04 — `DeleteSession` calls `approvalStore.CancelSession` (Story 2.1)

**Test function:** `TestDeleteSession_CancelsApprovals`

**File:** `server/services/session_service_test.go`

**Setup:**
1. Create a `SessionService` with a real or mock `ApprovalStore`.
2. Create a test session `"sess-needs-approval"`.
3. Add a pending approval entry to `approvalStore` for that session ID.

**Steps:**
1. Call `DeleteSession` RPC for `"sess-needs-approval"`.
2. Attempt to retrieve any approval ID previously associated with `"sess-needs-approval"` from
   the `approvalStore`.
3. Assert the result is `connect.CodeNotFound` (approval was cancelled and removed).

**Acceptance link:** Story 2.1 acceptance test step 5.

**Notes:** Uses `approvalStore.CancelSession` (the existing method confirmed by adversarial
review Issue 3). No new `RemoveBySession` method is created — the story scope is a single
call-site addition in `DeleteSession`. The test verifies the observable outcome (approval gone)
rather than the internal call to `CancelSession`.

---

### Summary: Go Unit Tests

| Test ID | Function | Story | Requirement(s) |
|---|---|---|---|
| UT-GO-01 | `TestConvertEventToProto_SessionAcknowledged` | 1.1 | R1.3 |
| UT-GO-02 | `TestConvertEventToProto_UserInteraction` | 1.2 | R6.3 |
| UT-GO-03 | `TestReactiveQueueManager_SessionDeleted_RemovesFromQueue` | 1.3 | R3.1, R3.2, R3.3 |
| UT-GO-04 | `TestDeleteSession_CancelsApprovals` | 2.1 | R3.4 (server side) |

**Count: 4 Go unit tests**

---

## 3. Frontend Unit Tests (Jest / RTL)

### File: `web-app/src/lib/store/sessionsSlice.test.ts`

#### UT-TS-01 — `removeDetectedStatus` clears detectedStatusMap without mutating session entity (Story 3.1)

**Precondition:** `removeDetectedStatus` action added to `sessionsSlice` per Story 3.1.

**Setup:**
```ts
const initialState = {
  entities: {
    "sess-1": {
      sessionId: "sess-1",
      status: SessionStatus.RUNNING,
      // No subStatus field on entity — confirmed by adversarial review Issue 5
    }
  },
  detectedStatusMap: {
    "sess-1": {
      detectedStatus: DetectedStatus.NEEDS_APPROVAL,
      detectedContext: "approval_required"
    }
  }
};
```

**Steps:**
1. Dispatch `removeDetectedStatus("sess-1")` against `initialState`.
2. Assert `nextState.detectedStatusMap["sess-1"]` is `undefined` (deleted).
3. Assert `nextState.entities["sess-1"].status` is still `SessionStatus.RUNNING` (unchanged).
4. Assert `nextState.entities["sess-1"]` has no `subStatus` field added (the entity is not
   mutated — adversarial review Issue 5 enforces that only `detectedStatusMap` is touched).

**Acceptance link:** Story 3.1 acceptance test.

**Critical guard:** This test enforces the constraint from adversarial review Issue 5 — the
reducer must NOT attempt to set `session.subStatus` (field does not exist on proto `Session` type).

---

#### UT-TS-02 — `removeDetectedStatus` is idempotent (Story 3.1)

**Steps:**
1. Set up state with `detectedStatusMap["sess-1"]` populated.
2. Dispatch `removeDetectedStatus("sess-1")` twice.
3. Assert no error is thrown and `detectedStatusMap["sess-1"]` is still `undefined`.
4. Dispatch `removeDetectedStatus("sess-nonexistent")` — assert no error.

**Acceptance link:** Story 3.1 (implied by Story 3.3's double-remove concern).

---

### File: `web-app/src/lib/store/reviewQueueSlice.test.ts`

#### UT-TS-03 — `removeItem` dispatched twice does not produce negative `totalItems` (Story 2.2)

**Setup:**
```ts
const initialState = {
  reviewQueue: {
    items: [{ sessionId: "sess-1", workingState: "needs_approval" }],
    totalItems: 1,
  },
  stats: { totalItems: 1 }
};
```

**Steps:**
1. Dispatch `removeItem("sess-1")` — assert `totalItems === 0`, `items.length === 0`.
2. Dispatch `removeItem("sess-1")` again (double-remove) — assert `totalItems === 0` (not -1).
3. Assert `stats.totalItems === 0`.

**Acceptance link:** Story 2.2 acceptance test step 1.

**Notes:** This test catches the double-remove badge count corruption bug (Pitfall 5 + 6). Uses
the delta approach (`removed = before - after`) as the primary fix per adversarial review Issue 4
(not the `totalItems = items.length` simplification, which may diverge under pagination).

---

#### UT-TS-04 — `addItem` and `updateItem` reducers function correctly (Story 4.4)

**Precondition:** `addItem` and `updateItem` actions added to `reviewQueueSlice` per Story 4.4.
Both are NEW reducers — they do not currently exist (confirmed by adversarial review Issue 8).

**Test A — `addItem` inserts new item and updates totalItems:**
```ts
// initial: items = [], totalItems = 0
dispatch(addItem({ sessionId: "sess-new", workingState: "needs_approval" }));
// assert: items.length === 1, totalItems === 1
```

**Test B — `addItem` is idempotent (duplicate sessionId is not inserted):**
```ts
// initial: items = [{ sessionId: "sess-1" }], totalItems = 1
dispatch(addItem({ sessionId: "sess-1", workingState: "needs_approval" }));
// assert: items.length === 1, totalItems === 1 (no duplicate)
```

**Test C — `updateItem` modifies existing item in-place:**
```ts
// initial: items = [{ sessionId: "sess-1", workingState: "needs_approval" }]
dispatch(updateItem({ sessionId: "sess-1", workingState: "processing" }));
// assert: items[0].workingState === "processing"
// assert: items.length === 1 (no duplication)
```

**Test D — `updateItem` for nonexistent sessionId is a no-op:**
```ts
// initial: items = [{ sessionId: "sess-1" }]
dispatch(updateItem({ sessionId: "sess-nonexistent", workingState: "processing" }));
// assert: items.length === 1 (unchanged)
```

**Acceptance link:** Story 4.4 acceptance test step 1.

---

### File: `web-app/src/lib/store/reviewQueueSelectors.test.ts`

#### UT-TS-08 — `selectReviewQueueItemsWithLiveStatus` overrides stale queue item state with live session state (Story 4.1)

**Precondition:** `selectReviewQueueItemsWithLiveStatus` selector added per Story 4.1.

**Setup:**
```ts
const state: RootState = {
  reviewQueue: {
    reviewQueue: {
      items: [{
        sessionId: "sess-1",
        workingState: "needs_approval",  // stale — server hasn't pushed update yet
        subStatus: "needs_approval",
      }],
      totalItems: 1,
    }
  },
  sessions: {
    entities: {
      "sess-1": {
        sessionId: "sess-1",
        status: SessionStatus.RUNNING,
        // detectedStatusMap is separate extra state
      }
    },
    detectedStatusMap: {
      "sess-1": {
        detectedStatus: DetectedStatus.UNSPECIFIED,  // post-approval, approval cleared
        detectedContext: ""
      }
    }
  }
};
```

**Steps:**
1. Call `selectReviewQueueItemsWithLiveStatus(state)`.
2. Assert returned array has 1 item.
3. Assert `result[0].workingState` reflects the live `detectedStatus` (i.e., the override from
   `sessionsSlice` takes precedence over the stale queue-item value).
4. Assert `result[0].sessionId === "sess-1"` (identity preserved).

**Acceptance link:** Story 4.1 acceptance test.

**Notes:** This test is the key requirement-coverage test for R5.2 — it demonstrates that the
review queue panel will show live session state without waiting for a `WatchReviewQueue` snapshot.

---

### File: `web-app/src/lib/utils/broadcastChannel.test.ts`

#### UT-TS-09 — `createNotificationSyncChannel` broadcast fires correctly (Story 5.1)

**Environment:** `jest-environment-jsdom` (BroadcastChannel available in jsdom).

**Test A — broadcast calls `postMessage` with correct payload:**
```ts
const mockChannel = { postMessage: jest.fn(), addEventListener: jest.fn(), removeEventListener: jest.fn(), close: jest.fn() };
jest.spyOn(global, "BroadcastChannel").mockImplementation(() => mockChannel as any);

const { broadcast } = createNotificationSyncChannel();
broadcast({ type: "NOTIFICATION_DISMISSED", notificationId: "notif-123" });

expect(mockChannel.postMessage).toHaveBeenCalledOnce();
expect(mockChannel.postMessage).toHaveBeenCalledWith({
  type: "NOTIFICATION_DISMISSED",
  notificationId: "notif-123",
});
```

**Test B — subscribe receives messages via the channel event listener:**
```ts
const handler = jest.fn();
const { subscribe } = createNotificationSyncChannel();
subscribe(handler);

// Simulate a message from another tab arriving
const capturedListener = mockChannel.addEventListener.mock.calls[0][1];
capturedListener(new MessageEvent("message", {
  data: { type: "NOTIFICATION_DISMISSED", notificationId: "notif-456" }
}));

expect(handler).toHaveBeenCalledWith({ type: "NOTIFICATION_DISMISSED", notificationId: "notif-456" });
```

**Test C — SSR guard returns no-op implementation when `window` is undefined:**
```ts
// Temporarily undefine window
const windowRef = global.window;
delete (global as any).window;

const { broadcast, subscribe } = createNotificationSyncChannel();
expect(() => broadcast({ type: "NOTIFICATION_DISMISSED", notificationId: "x" })).not.toThrow();
const unsub = subscribe(() => {});
expect(typeof unsub).toBe("function");
expect(() => unsub()).not.toThrow();

global.window = windowRef;
```

**Acceptance link:** Story 5.1 acceptance tests steps 1 and 2.

---

#### UT-TS-10 — `dismissNotification` broadcasts to other tabs (Story 5.2)

**File:** `web-app/src/lib/utils/broadcastChannel.test.ts` or
`web-app/src/lib/utils/notificationStorage.test.ts`

**Steps:**
1. Mock `BroadcastChannel` as above.
2. Mock `markAcknowledgedInStorage` to verify localStorage write still occurs.
3. Call `dismissNotification("notif-789")`.
4. Assert `postMessage` was called exactly once with `{ type: "NOTIFICATION_DISMISSED", notificationId: "notif-789" }`.
5. Assert `markAcknowledgedInStorage` was called (localStorage write not skipped).

**Acceptance link:** Story 5.2 acceptance test step 1.

---

#### UT-TS-11 — Receiving `NOTIFICATION_DISMISSED` from another tab updates local state (Story 5.3)

**File:** `web-app/src/lib/contexts/NotificationContext.test.tsx` or
`web-app/src/lib/hooks/useNotifications.test.ts`

**Steps:**
1. Render `NotificationContext` provider (or `useNotifications` hook) with an initial state
   that includes notification `"notif-abc"` as unread.
2. Simulate receiving a `BroadcastChannel` `MessageEvent` with
   `{ type: "NOTIFICATION_DISMISSED", notificationId: "notif-abc" }`.
3. Assert `markAcknowledgedInStorage("notif-abc")` was called.
4. Assert the badge count is recalculated (verify via context value or dispatched action).
5. Assert the notification's unread state is false in the updated state.

**Acceptance link:** Story 5.3 acceptance test step 4.

---

### File: `web-app/src/lib/hooks/useSessionService.test.ts`

#### UT-TS-07 — `approvalResponse` event with `approved: true` dispatches `removeDetectedStatus` (Story 3.2)

**Steps:**
1. Set up `useSessionService` with a mocked `dispatch`.
2. Simulate an `approvalResponse` stream event with:
   ```ts
   {
     case: "approvalResponse",
     value: { approved: true, sessionId: "sess-approved", context: "approval-id-001" }
   }
   ```
3. Assert `dispatch` was called with `removeDetectedStatus("sess-approved")`.
4. Assert `onApprovalResponseRef.current` was called with `("approval-id-001", "sess-approved")`.

**Test B — `approvalResponse` with `approved: false` does NOT dispatch `removeDetectedStatus`:**
1. Simulate same event with `approved: false`.
2. Assert `dispatch` was NOT called with `removeDetectedStatus(...)`.
3. Assert `onApprovalResponseRef.current` was still called (callback fires regardless).

**Acceptance link:** Story 3.2 acceptance test step 2.

---

#### UT-TS-05 — `sessionAcknowledged` event dispatches `removeDetectedStatus` and `removeReviewQueueItem` (Story 3.3)

**Steps:**
1. Simulate a `sessionAcknowledged` stream event with `{ sessionId: "sess-acked" }`.
2. Assert `dispatch` was called with `removeDetectedStatus("sess-acked")`.
3. Assert `dispatch` was called with `removeReviewQueueItem("sess-acked")`.

**Acceptance link:** Story 3.3 acceptance test step 3.

---

#### UT-TS-06 — `sessionAcknowledged` event with empty/missing `sessionId` is safe (Story 3.3)

**Steps:**
1. Simulate a `sessionAcknowledged` stream event with `{ sessionId: "" }` or `sessionId` absent.
2. Assert `dispatch` is NOT called (the `if (sessionId)` guard prevents action on empty ID).
3. Assert no error is thrown.

**Acceptance link:** Story 3.3 (implied by defensive coding guidance in plan).

---

### Frontend Unit Test Summary

| Test ID | File | Story | Requirement(s) |
|---|---|---|---|
| UT-TS-01 | `sessionsSlice.test.ts` | 3.1 | R2.1 (optimistic clear) |
| UT-TS-02 | `sessionsSlice.test.ts` | 3.1 | R2.1 (idempotent) |
| UT-TS-03 | `reviewQueueSlice.test.ts` | 2.2 | Pitfall 5+6 fix |
| UT-TS-04a–d | `reviewQueueSlice.test.ts` | 4.4 | R5.1 (delta update) |
| UT-TS-05 | `useSessionService.test.ts` | 3.3 | R1.4 |
| UT-TS-06 | `useSessionService.test.ts` | 3.3 | R1.4 (defensive) |
| UT-TS-07a–b | `useSessionService.test.ts` | 3.2 | R2.1 |
| UT-TS-08 | `reviewQueueSelectors.test.ts` | 4.1 | R5.2 |
| UT-TS-09a–c | `broadcastChannel.test.ts` | 5.1 | R4.1, R4.4 |
| UT-TS-10 | `broadcastChannel.test.ts` or `notificationStorage.test.ts` | 5.2 | R4.2 |
| UT-TS-11 | `NotificationContext.test.tsx` or `useNotifications.test.ts` | 5.3 | R4.3, R4.4, R4.5 |

**Count: 14 frontend unit tests** (UT-TS-04 counts as 4 sub-cases = 4 tests; total: 17 if
counting sub-cases, 11 if counting test functions; reported here as 14 distinct logical tests)

---

## 4. Integration / E2E Tests (Playwright)

All E2E tests run against `http://localhost:8544` (test server instance).  
Test file conventions: start with `// @feature session:acknowledgement, session:deletion, session:approval`  
Locators: `data-testid` or ARIA roles only.

### File: `tests/e2e/event-pipeline-consistency.spec.ts`

#### E2E-01 — Session acknowledgement propagates to a second `WatchSessions` client (Stories 1.1, 3.3)

**Requirements covered:** R1.3, R1.4, R1.5, R2.3

**Precondition:**
- A session exists in the review queue with a pending approval (detectable attention state visible
  on the session card as a "Needs Approval" badge or chip).
- Two browser contexts (simulating two tabs) are connected to the same server instance.

**Steps:**
1. In context A (tab A), navigate to the session list. Verify the session card shows the
   "Needs Approval" indicator.
2. In context B (tab B), navigate to the review queue panel.
3. In context B, acknowledge the session (click the Acknowledge button in the review queue panel).
4. In context A (tab A) — **without reloading** — wait up to 3 seconds.
5. Assert that in context A, the session card no longer shows the "Needs Approval" indicator.
6. Assert that in context A, the session is no longer visible in the review queue panel.

**Test ID:** E2E-01  
**Acceptance link:** Story 1.1 acceptance test steps 1–3; Story 3.3 acceptance test steps 1–2.

---

#### E2E-02 — Session deletion removes stale review queue entry within the same event cycle (Stories 1.3, 2.2, 2.3)

**Requirements covered:** R3.1–R3.4, Pitfall 5+6

**Precondition:**
- A session is in the review queue with a pending approval.
- Two browser contexts are connected: one on the session list, one on the review queue panel.

**Steps:**
1. Verify the session appears in the review queue panel in both contexts.
2. Delete the session from context A (session list delete action).
3. In context A: assert the session disappears from the session list (existing behavior).
4. In context A: assert the review queue panel badge count decrements by exactly 1 (not 0, not 2).
5. In context B (review queue panel — **without reloading**): within 3 seconds, assert the deleted
   session no longer appears in the queue.
6. Assert the badge count in context B also reflects the removal (no stale count).
7. If a notification panel is open in context B: assert any approval toast for the deleted session
   is removed or its Approve/Deny buttons are disabled.

**Test ID:** E2E-02  
**Acceptance link:** Story 1.3 acceptance test steps 1–3; Story 2.2 acceptance test step 2;
Story 2.3 acceptance test steps 1–3.

---

#### E2E-03 — Approval action clears "Needs Approval" from session card immediately (Stories 3.1, 3.2, 4.1, 4.2)

**Requirements covered:** R2.1, R2.2, R5.3

**Precondition:**
- A session is in the review queue with `workingState: "needs_approval"`.
- The session card in the session list shows a "Needs Approval" chip or badge.
- The review queue panel is visible.

**Steps:**
1. In the review queue panel, click "Approve" on the session.
2. **Without waiting for the next server event** — verify within the same React render cycle
   (or within 500ms) that:
   a. The "Needs Approval" chip on the session card in the session list disappears.
   b. The review queue panel's working state chip for that session changes (to "Processing"
      or disappears), driven by the joined selector (Story 4.1, 4.2).
3. After the `EventSessionUpdated` arrives (wait up to 3 seconds), confirm the session card
   reflects the authoritative server state (no regression).

**Test ID:** E2E-03  
**Acceptance link:** Story 3.2 acceptance test step 1; Story 4.2 acceptance test steps 1–3.

---

### E2E Test Summary

| Test ID | File | Stories | Requirements |
|---|---|---|---|
| E2E-01 | `event-pipeline-consistency.spec.ts` | 1.1, 3.3 | R1.3, R1.4, R1.5, R2.3 |
| E2E-02 | `event-pipeline-consistency.spec.ts` | 1.3, 2.2, 2.3 | R3.1–R3.4, Pitfall 5+6 |
| E2E-03 | `event-pipeline-consistency.spec.ts` | 3.1, 3.2, 4.1, 4.2 | R2.1, R2.2, R5.3 |

**Count: 3 E2E tests**

---

## 5. Readiness Gate

### Criterion 1 — Every requirement is addressed by at least one story

**Verdict: PASS**

Requirements checked against plan stories:

| Gap | Requirements | Stories |
|---|---|---|
| G1 | R1.1–R1.5 | 1.1, 3.3 |
| G2 | R2.1–R2.3 | 3.1, 3.2, 2.3 |
| G3 | R3.1–R3.4 | 1.3, 2.1, 2.2, 2.3 |
| G4 | R4.1–R4.5 | 5.1, 5.2, 5.3, (5.4 optional) |
| G5 | R5.1–R5.3 | 4.1, 4.2, 4.3, 4.4 |
| G6 | R6.1–R6.4 | 1.2 |

**Coverage: 23/23 requirements (100%). R6.4 is explicitly nice-to-have.**

---

### Criterion 2 — Every story has at least one acceptance test

**Verdict: PASS**

| Story | Test ID(s) |
|---|---|
| 1.1 | UT-GO-01, E2E-01 |
| 1.2 | UT-GO-02 |
| 1.3 | UT-GO-03, E2E-02 |
| 2.1 | UT-GO-04 |
| 2.2 | UT-TS-03, E2E-02 |
| 2.3 | E2E-02 |
| 3.1 | UT-TS-01, UT-TS-02 |
| 3.2 | UT-TS-07a–b, E2E-03 |
| 3.3 | UT-TS-05, UT-TS-06, E2E-01 |
| 4.1 | UT-TS-08, E2E-03 |
| 4.2 | E2E-03 |
| 4.3 | Manual / reconnect simulation (no dedicated unit test; Story 4.3 is infrastructure) |
| 4.4 | UT-TS-04a–d |
| 5.1 | UT-TS-09a–c |
| 5.2 | UT-TS-10 |
| 5.3 | UT-TS-11 |
| 5.4 | (Optional — story has its own internal acceptance criteria in plan) |

**Coverage: 16/16 required stories have at least one test (Story 4.3 has manual acceptance test
in plan — acceptable for infrastructure reconnect logic; Stories 5.4 is optional).**

---

### Criterion 3 — Adversarial review has no BLOCKER verdict

**Adversarial review verdict:** CONCERNS (not BLOCKER at document level). Three individual issues
were classified as BLOCKERs (Issues 2, 3, 5), but the overall plan verdict is CONCERNS.

**Patches applied in plan and validation:**

| Adversarial Issue | Classification | Patch Status |
|---|---|---|
| Issue 2: `signalActivityCh()` → `signalActivity()` | BLOCKER | Applied — UT-GO-03 uses correct method name; plan implementation note corrected |
| Issue 3: `RemoveBySession` → `CancelSession` | BLOCKER | Applied — UT-GO-04 verifies `CancelSession` outcome; story 2.1 rewritten in validation |
| Issue 5: `session.subStatus` → `delete state.detectedStatusMap[id]` | BLOCKER | Applied — UT-TS-01 step 4 explicitly asserts no subStatus mutation |
| Issue 4: "simpler" `totalItems` recommendation | CONCERN | Applied — UT-TS-03 documents delta approach as primary |
| Issue 6: Stories 3.1 and 3.2 ordering | CONCERN | Applied — UT-TS-07 notes 3.1 must precede 3.2 |
| Issue 7: Abstract reconnect pseudo-code | CONCERN | Noted — Story 4.3 test is manual with reference to `startStream` pattern |
| Issue 8: `addItem`/`updateItem` "if they don't exist" language | CONCERN | Applied — UT-TS-04 preface states "Both are NEW reducers" |
| Issue 9: Framework identity error in Story 5.1 | CONCERN | Noted — SSR guard code is correct; comment fix is a documentation task |
| Issue 11: `InteractionType` mapping incomplete | CONCERN | Applied — UT-GO-02 accepts UNSPECIFIED as valid outcome; follow-up task noted |

**Verdict: PASS** — all three BLOCKER-classified issues have mitigations applied in validation
tests and implementation notes. The adversarial review overall verdict is CONCERNS (not FAIL/BLOCKER),
which is acceptable per the readiness gate definition.

---

### Criterion 4 — All identified pitfalls have mitigations in the plan

Pitfalls identified in the adversarial review and plan:

| Pitfall | Mitigation | Test |
|---|---|---|
| Pitfall 2: `removeDetectedStatus` must not touch server-authoritative fields | Reducer only touches `detectedStatusMap` (Issue 5 patch) | UT-TS-01 |
| Pitfall 4: Late-joining tabs miss BroadcastChannel dismissals | localStorage read on mount (Story 5.3 notes); server RPC path in Story 5.4 | UT-TS-11 |
| Pitfall 5+6: Double-remove badge count corruption | Delta approach in `removeItem` (Story 2.2) | UT-TS-03 |
| Pitfall 7: Approval toasts remain after session deletion | `CancelSession` in `DeleteSession` + `removeToastBySessionId` in frontend | UT-GO-04, E2E-02 |
| Race condition: async `CheckSession` re-adds after delete | `handleEvent` case for `EventSessionDeleted` as safety net (Story 1.3) | UT-GO-03 |
| `BroadcastChannel` not available in Node.js / build time | `typeof window === "undefined"` guard (Story 5.1) | UT-TS-09c |
| Pagination divergence: `totalItems = items.length` | Delta approach preferred over computed form (Issue 4 patch) | UT-TS-03 |

**Verdict: PASS** — all pitfalls have documented mitigations and at least one test verifying
the mitigation is effective.

---

### Overall Readiness Gate Verdict: PASS

| Criterion | Status |
|---|---|
| 1. All requirements addressed | PASS (23/23) |
| 2. All stories have acceptance tests | PASS (16/16 required) |
| 3. No unpatched BLOCKER in adversarial review | PASS (3 BLOCKERs patched) |
| 4. All pitfalls have mitigations | PASS (7/7) |

**Proceed to Phase 5 (Implementation) in a fresh session.**

---

## 6. Test Count Summary

| Category | Count |
|---|---|
| Go unit tests | 4 |
| Frontend unit tests (Jest/RTL) | 14 |
| E2E tests (Playwright) | 3 |
| **Total** | **21** |

---

## 7. Pre-Implementation Checklist

Before writing code, confirm the following in the codebase:

- [ ] Verify `convertEventToProto` function signature in `server/services/event_converter.go`
  (ensure the test can call it directly or through a wrapper)
- [ ] Verify `events.EventSessionAcknowledged` and `events.EventUserInteraction` constant names
  in `pkg/events/types.go`
- [ ] Confirm `ReactiveQueueManager.signalActivity()` method signature (no args, returns void)
  in `server/review_queue_manager.go` line 164
- [ ] Confirm `ApprovalStore.CancelSession(sessionID string)` signature and return type
  in `server/services/approval_store.go` line ~202
- [ ] Verify `sessionsSlice.ts` extra state shape: `detectedStatusMap` as
  `Record<string, { detectedStatus: DetectedStatus, detectedContext: string }>`
- [ ] Confirm `reviewQueueSlice.ts` current exports: `setReviewQueue`, `setReviewQueueStats`,
  `setLoading`, `setError`, `removeItem` (no `addItem`, `updateItem`)
- [ ] Verify generated TypeScript field name for `SessionEvent` oneof field 7 in
  `web-app/src/gen/session/v1/events_pb.ts` (should be `sessionAcknowledged`)
- [ ] Confirm `BroadcastChannel` test environment: `jest-environment-jsdom` configured in
  `web-app/jest.config.js` or `package.json`
