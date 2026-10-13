# Plan: Reply Card Parallel Question & Cold-Load Disabled

## Summary

Two frontend-only fixes. No proto or backend changes required for the minimal scope.

---

## Task 1: Add `parallel_question` unavailability cause (Bug 1, minimal fix)

**File:** `web-app/src/lib/reply/replyQuestion.ts`

1. Add `"parallel_question"` to `ReplyUnavailableCause`:
   ```ts
   export type ReplyUnavailableCause = "no_proof" | "bad_proof" | "shape" | "path_only" | "parallel_question";
   ```

2. Update `replyAvailability` signature to accept `occurrenceCount`:
   ```ts
   export function replyAvailability(
     n: Pick<NotificationData, "notificationType" | "metadata" | "message">,
     occurrenceCount?: number,
   ): ReplyAvailability | null
   ```

3. Before returning `replyable`, check `occurrenceCount`:
   ```ts
   if ((occurrenceCount ?? 1) > 1) {
     return { kind: "unavailable", cause: "parallel_question" };
   }
   ```

4. Update `unavailableCopy` to handle the new cause:
   ```ts
   case "parallel_question":
     return "Multiple questions in this turn — answer in the terminal";
   ```

**Estimate:** 1 h

---

## Task 2: Pass `occurrenceCount` from `ReplyCard` callers

**File:** `web-app/src/components/sessions/ReplyCard.tsx`

The `notification` prop already carries `occurrenceCount` through `NotificationData`. Pass it
to `replyAvailability`:

```ts
const availability = replyAvailability(notification, notification.occurrenceCount);
```

If `ReplyCardProps` uses a `Pick<NotificationData, ...>` type, add `occurrenceCount` to the
pick list.

**Estimate:** 30 min

---

## Task 3: Unit tests for `replyAvailability` with parallel questions

**File:** `web-app/src/lib/reply/replyQuestion.test.ts` (or wherever existing tests live — check `web-app/src/lib/__tests__/`)

Add test cases:
- `occurrenceCount = 2` with otherwise-valid metadata → `{ kind: "unavailable", cause: "parallel_question" }`
- `occurrenceCount = 1` with valid metadata → `{ kind: "replyable", ... }`
- `occurrenceCount = undefined` with valid metadata → `{ kind: "replyable", ... }` (backwards compat)

**Estimate:** 1 h

---

## Task 4: Gate cold-load offline in `useNotificationConnectivity`

**File:** `web-app/src/lib/hooks/useNotificationConnectivity.ts`

1. Import `selectSessionsHasLoadedOnce` from the sessions slice and read it via Redux:
   ```ts
   const hasLoadedOnce = useSelector(selectSessionsHasLoadedOnce);
   ```
   (The hook already has a Redux dependency via `useStreamConnectionState`.)

2. Gate the 10-s timer start on `hasLoadedOnce`:
   ```ts
   useEffect(() => {
     if (!hasLoadedOnce) {
       setStreamDownLong(false);
       return; // don't start the timer before the first connect
     }
     if (streamLive) {
       setStreamDownLong(false);
       return;
     }
     const timer = setTimeout(() => setStreamDownLong(true), STREAM_DOWN_GRACE_MS);
     return () => clearTimeout(timer);
   }, [streamLive, hasLoadedOnce]);
   ```

3. Verify: `isOffline = !browserOnline || (streamDownLong && historyError !== null)` is
   unchanged — the guard lives in the timer effect, not the isOffline expression.

**Estimate:** 1 h

---

## Task 5: Unit tests for cold-load connectivity

**File:** `web-app/src/lib/hooks/__tests__/useNotificationConnectivity.test.ts` (create if absent)

Test scenarios:
- `connectionState = "disconnected"`, `hasLoadedOnce = false`, `historyError = new Error("x")`,
  after 11 s → `isOffline` is `false` (never-connected case)
- `connectionState = "connected"` → `isOffline` is `false`
- `connectionState = "disconnected"`, `hasLoadedOnce = true`, `historyError = new Error("x")`,
  after 11 s → `isOffline` is `true` (post-connect drop case, existing behavior preserved)
- Browser offline → `isOffline` is `true` regardless of stream state

Use `jest.useFakeTimers()` for the 10 s grace.

**Estimate:** 2 h

---

## Task 6: Update `ReplyCard.test.tsx` if any assertions reference `replyAvailability` shape

Check `web-app/src/components/sessions/__tests__/ReplyCard.test.tsx` for any test that
currently passes a notification with valid metadata and no `occurrenceCount` — these should
still pass (undefined = single question). Add one test for the parallel-question "Answer in
the terminal" render path.

**Estimate:** 1 h

---

## Adversarial review

### Challenge 1: `occurrenceCount` is undefined on pre-dedup records — does this hide the bug?
**Resolution:** `(occurrenceCount ?? 1) > 1` → `undefined` maps to 1 → single-question path.
Pre-dedup records where two questions exist as two separate records would show the most-recent
record's question_id anyway (the dedup collapses on next server restart), so the parallel-
question scenario is only observable post-dedup. Safe.

### Challenge 2: What if the second question arrives and the first is already answered?
In that case `OccurrenceCount` goes to 2 but B is now the live question. The card shows
"Answer in the terminal" even though B is replyable. This is acceptable: the parallel case is
rare, and "Answer in the terminal" is never wrong. A deeper fix (REQ-Q4) can handle this later.

### Challenge 3: Does `hasLoadedOnce` reset on page navigation?
No — it is a Redux in-memory value set once and never cleared. On tab refocus (stream
reconnects), `hasLoadedOnce` is `true` so the existing grace-period logic still applies. Safe.

### Challenge 4: Does the cold-load fix mask a genuine server-down scenario?
On a genuinely down server: `hasLoadedOnce` is `false` forever (stream never connects), so
`isOffline` stays `false`. User taps a reply button → `ReplyToPendingQuestion` RPC fails →
`retryable` phase → Retry shown. This is a worse UX message than "Offline" but still
fail-closed: nothing wrong is sent. Post-connect: if server comes up and stream connects,
`hasLoadedOnce` becomes `true`; subsequent drops will correctly trigger the offline path.

### Challenge 5: `useNotificationConnectivity` has no existing test file — how to test the hook?
Use `renderHook` from `@testing-library/react` with a mock Redux store, as done in
`ConnectionIndicator.test.tsx` (which already wraps with a test-store provider). The pattern
is established.

---

## Correction after review

Tasks 1-3 as written used `occurrenceCount > 1` as the parallel signal; that count is a lifetime
total (see validation.md E3). Implemented instead: `PendingQuestionStore.PendingCount` stamped as
`question_pending_count` notification metadata; `replyAvailability` prefers it and falls back to
`occurrenceCount` only when absent.

## Non-scope: deeper parallel-question fix (REQ-Q4)

A future task could expose a `ListPendingQuestions(sessionId)` RPC that returns all live
question IDs. The card would then call `PendingQuestionStore.Peek` for each and pick the one
whose `Dialog` matches the live screen capture. This would require:
- New proto RPC
- New handler in `hidden_question_reply.go`
- New `useListPendingQuestions` hook
- `pendingQuestionFor` in `replyQuestion.ts` updated to prefer a matching id

Track separately.
