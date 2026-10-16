# Requirements: Reply Card Parallel Question & Cold-Load Disabled

**Item ID:** 59f488b2-50fd-4da4-a167-c03fe7e0035e  
**Title:** fix(web): Reply card shows the wrong question for parallel AskUserQuestion and is disabled on cold load  
**Discovered in:** Live Reply check of PR #974  
**Category:** bugfix — two UX defects, both fail-closed (no bad data written)

---

## Background

The Reply card (ADR-010, `ReplyCard.tsx`) enables one-tap answers to pending `AskUserQuestion`
dialogs in hidden sessions. Notifications deduplicate by `(sessionId, notificationType)` in
the `NotificationHistoryStore`, so only one notification record exists per session even when
two questions are raised in the same turn. The card reads the notification metadata
(`question_id`, `question_options`, `question_shape`) to decide which buttons to render.

The card's offline guard (`useNotificationConnectivity`) reports `isOffline` when the browser
is offline OR when the `WatchSessions` stream has been non-live for ≥10 s **AND** the history
fetch is also failing. The initial Redux `connectionState` is `"disconnected"`.

---

## Bug 1: Parallel questions — card shows wrong question's options

### Symptom
When two `AskUserQuestion` calls fire in the same turn (two separate dialogs, both registered
with distinct `question_id`s in `PendingQuestionStore`), only one notification record exists
for the session. On the second arrival `mergeOccurrence` replaces the metadata with the **newest**
question's `question_id` and `question_options`.

While question A is on screen (the first question, still visible in the terminal), the card
renders B's options and sends B's `question_id` on tap. The `beforeFirstByteChecks` finds B's
dialog not on screen and returns `STALE_PROMPT`. A's `question_id` was never on the card.
Question A can only be answered in the terminal.

### Root cause
The notification deduplication key is `(sessionId, notificationType)`. Questions are a single
`notificationType` ("question"), so the second question collapses into the first notification
record, overwriting `question_id` and `question_options` even though both questions are
simultaneously live in `PendingQuestionStore`.

### Requirements
- **REQ-Q1:** When a question notification arrives for a session that already has an unread
  question notification, the card must not offer buttons it knows cannot work. Specifically:
  if the newest registered `question_id` in the notification metadata does not match what is
  live on screen (which the client cannot verify), the card must surface an "Answer in the
  terminal" message rather than actionable option buttons.
- **REQ-Q2:** The fix must not regress the single-question happy path: one unread question with
  a matching `question_id` in metadata must render option buttons normally.
- **REQ-Q3:** The fix must not require a new network round-trip on every render.
- **REQ-Q4 (preferred):** If a future server-side change exposes all pending question IDs per
  session, the card should be able to select the one whose `question_id` matches what is on
  screen. This is optional scope; the immediate fix is the fallback described in REQ-Q1.
- **REQ-Q5:** `OccurrenceCount` display and notification-history behaviour must be unchanged.

### Accepted approach (from item description)
Show "Answer in the terminal" when the notification's `occurrence_count > 1` (i.e., a second
question merged into the record). This is the minimal, immediately-safe fix. A deeper fix
would expose all pending `question_id`s from the server so the card can match the live question.

---

## Bug 2: Cold-load card is disabled for ~20 s

### Symptom
On a fresh deep link the card renders "Offline - reconnect to reply" with buttons disabled
until the `WatchSessions` stream first connects (state becomes `"connected"`). On a slow
connection this can take ~20 s.

### Root cause
The initial Redux `connectionState` is `"disconnected"` (see `sessionsSlice.ts` line 51).
`useStreamConnectionState()` returns `"disconnected"` before the stream establishes.
`STREAM_DOWN_GRACE_MS = 10_000` means that after 10 s without a live stream, `streamDownLong`
becomes `true`. If `historyError` is also non-null at that moment (history fetch slow or
failing), `isOffline` becomes `true` and the card shows the offline message and disables
buttons.

The 10 s grace was designed for reconnect cycles, not the initial cold load. On cold load there
was never a prior connection, so "Offline" is misleading — "Connecting..." would be accurate.

### Requirements
- **REQ-C1:** On cold load (before `WatchSessions` has ever connected), the card must never
  show "Offline - reconnect to reply" regardless of whether the history fetch is slow. It
  should either show "Connecting…" or simply not disable the buttons until the stream has been
  live at least once.
- **REQ-C2:** The existing post-connect offline detection must be preserved: once the stream
  has been live and then drops for ≥10 s with a history error, `isOffline` should still
  trigger.
- **REQ-C3:** The fix must apply to the `useNotificationConnectivity` hook (or a thin shim
  above it) rather than patching the card directly, so all consumers of the hook benefit.
- **REQ-C4:** `STREAM_DOWN_GRACE_MS` and `historyError` conditions must remain unchanged for
  the reconnect path.

---

## Non-requirements (explicitly out of scope)

- Remote-caller refusal (non-loopback/proxied)
- A race with a permission dialog
- Free-text / "Type something." digit rejection
- Real phone soft keyboard

---

## Acceptance Criteria

| # | Criterion | Status |
|---|-----------|--------|
| AC-1 | When two questions for the same session are in flight (occurrence_count ≥ 2 on the notification record), the card renders "Answer in the terminal" instead of option buttons. | pending |
| AC-2 | Single-question happy path: one unread question with matching metadata renders option buttons and successfully replies. | pending |
| AC-3 | On fresh page load (stream has never connected), the card does not show "Offline - reconnect to reply" and does not disable buttons until the stream has been live at least once and subsequently drops. | pending |
| AC-4 | Once the stream has been live and drops for >10 s with a history fetch error, `isOffline` still triggers as before (reconnect offline path unchanged). | pending |
| AC-5 | Unit tests cover: (a) parallel-question card renders "Answer in terminal" when occurrence_count > 1; (b) cold-load card with stream "disconnected" but `hasConnectedOnce=false` does not report isOffline. | pending |
| AC-6 | No regression to `ReplyCard` tests (`ReplyCard.test.tsx` and `useReplyToQuestion.test.ts`). | pending |
