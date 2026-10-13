# Research: Reply Card Parallel Question & Cold-Load Disabled

## Codebase findings

### Bug 1: Parallel questions

#### Notification deduplication path
`NotificationHistoryStore.Append` (`server/notifications/store.go:198`) deduplicates records
by `(sessionId, notificationType)` via `findDuplicate`. When a second question arrives for the
same session, `mergeOccurrence` (line 255) increments `OccurrenceCount` and replaces the
record's `Metadata` with the new question's `question_id`, `question_options` and
`question_shape`. The old `question_id` is lost.

#### Question registration
`registerQuestion` (`server/services/approval_question.go:99`) assigns a unique `question_id`
to every single-select question and registers it in `PendingQuestionStore`. Both questions A
and B remain in `PendingQuestionStore` (keyed by their own question_id), but the notification
record only carries B's `question_id` after the second question arrives.

#### Card metadata parsing
`replyAvailability` (`web-app/src/lib/reply/replyQuestion.ts:39`) reads `question_id` from the
notification's metadata. With only B's id available, the card always attempts to answer B,
which `DialogMatch` rejects (B is not on screen) → `STALE_PROMPT`.

#### occurrenceCount is already available
`NotificationData.occurrenceCount?: number` (`web-app/src/lib/types/notification.ts:53`) is
already passed through `notificationHistoryMerge.ts`. `occurrenceCount >= 2` is a reliable
signal that a second question has overwritten the first.

#### Fix surface area
- `replyAvailability` in `web-app/src/lib/reply/replyQuestion.ts` — add `occurrenceCount`
  parameter; return `{ kind: "unavailable", cause: "parallel_question" }` when count > 1.
- `ReplyCard.tsx` and callers that pass `notification` to `replyAvailability` — pass
  `occurrenceCount` through.
- `unavailableCopy` — add human-readable copy for the new cause.
- `ReplyUnavailableCause` type — add `"parallel_question"` variant.
- Unit test in `replyQuestion.test.ts`.

No backend change needed for this minimal fix.

#### Future deep fix (REQ-Q4)
Exposing all pending question IDs from `PendingQuestionStore.bySession` via a new
`ListPendingQuestions` RPC would let the card pick the ID matching the live dialog. This
requires a proto change and a new handler. Not in scope for this fix.

---

### Bug 2: Cold-load card disabled for ~20 s

#### Offline detection chain
`useNotificationConnectivity` (`web-app/src/lib/hooks/useNotificationConnectivity.ts`):
```
isOffline = !browserOnline || (streamDownLong && historyError !== null)
```
- `streamDownLong` becomes `true` after `STREAM_DOWN_GRACE_MS = 10_000` ms of `state !== "connected"`.
- Initial Redux `connectionState = "disconnected"` (`sessionsSlice.ts:51`).
- So if the history fetch is also slow/erroring, `isOffline` can become `true` after 10 s even
  before the stream has ever been live.

#### hasLoadedOnce is the right sentinel
`selectSessionsHasLoadedOnce` (`sessionsSlice.ts:178`) becomes `true` when `setSessions` fires
(first WatchSessions snapshot). It tracks whether the stream has ever connected, which is
exactly the "never been connected" guard needed.

#### Precedent in this codebase
`TerminalOutput.tsx` already uses a `hasEverConnectedRef` pattern (line 173) to suppress the
reconnect banner until a connection has been established at least once. The same principle
applies here.

#### Fix surface area
- `useNotificationConnectivity.ts` — consume `selectSessionsHasLoadedOnce` from Redux. Gate
  `streamDownLong` on `hasLoadedOnce`: only start the 10 s timer once the stream has been live
  at least once. Before `hasLoadedOnce`, `isOffline` stays `false` even if `historyError` is set.
- Alternatively, expose a `hasNeverConnected` boolean in `NotificationConnectivity` and let
  callers display "Connecting..." instead of "Offline". The hook-level fix is preferred (REQ-C3).
- Test: render the `ReplyCard` / call the hook with `connectionState="disconnected"`,
  `hasLoadedOnce=false`, `historyError` set — assert `isOffline` is `false`.

---

## Risk assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `occurrenceCount` is undefined on old records (pre-dedup data) | Low | Low | treat `undefined` or `0` as 1 — single question path unchanged |
| `hasLoadedOnce` is not reset on reconnect (correct by design) | N/A | N/A | the field is "set once, never cleared" by design |
| Suppressing offline on cold load masks genuine offline (e.g. server down from the start) | Low | Medium | history fetch error alone without `hasLoadedOnce` is not enough; server down = history fetch fails before 10s → `historyError` set but `isOffline` stays false until `hasLoadedOnce`. User taps → RPC fails → `retryable` phase → Retry button. Fail-closed is preserved. |
| Parallel question fix hides a usable card when only ONE question is live | None | High | `occurrenceCount === 1` (or undefined) shows the card normally |

---

## Files touched

| File | Change |
|---|---|
| `web-app/src/lib/reply/replyQuestion.ts` | Add `parallel_question` cause; accept `occurrenceCount` in `replyAvailability` |
| `web-app/src/lib/hooks/useNotificationConnectivity.ts` | Gate `streamDownLong` on `hasLoadedOnce` |
| `web-app/src/components/sessions/ReplyCard.tsx` | Pass `occurrenceCount` through; update copy |
| `web-app/src/lib/__tests__/replyQuestion.test.ts` (or sibling) | Unit tests for new cause |
| `web-app/src/lib/__tests__/useNotificationConnectivity.test.ts` (new) | Cold-load isOffline=false test |
| Callers of `replyAvailability` that pass `notification` | Pass `occurrenceCount` |

Backend files: none for minimal fix.
