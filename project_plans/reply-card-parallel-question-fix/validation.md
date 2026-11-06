# Validation: Reply Card Parallel Question & Cold-Load Disabled

## Plan completeness check

| Requirement | Covered by task | Notes |
|---|---|---|
| REQ-Q1: parallel questions → "Answer in the terminal" | Task 1, Task 2 | occurrenceCount>1 gates the card |
| REQ-Q2: single-question happy path unchanged | Task 3 (test confirms) | undefined/1 → replyable as before |
| REQ-Q3: no new round-trip per render | Task 1 (pure function change) | occurrenceCount is already in the notification record |
| REQ-Q4: future multi-id deep fix | Non-scope (plan.md notes) | tracked separately |
| REQ-Q5: OccurrenceCount display unchanged | Task 1 only touches replyAvailability | mergeOccurrence/grouping logic untouched |
| REQ-C1: cold load never shows "Offline" | Task 4 | hasLoadedOnce gates timer start |
| REQ-C2: post-connect drop still triggers offline | Task 4 + Task 5 (test confirms) | hasLoadedOnce=true after first connect |
| REQ-C3: fix is in the hook, not the card | Task 4 | useNotificationConnectivity change |
| REQ-C4: STREAM_DOWN_GRACE_MS unchanged | Task 4 | timer still 10 s, just not started before first connect |

All 9 requirements are covered. Non-scope items are explicitly named.

---

## Acceptance criteria traceability

| AC | Task(s) | Test |
|---|---|---|
| AC-1: parallel → unavailable card | T1, T2 | T3 unit test |
| AC-2: single → option buttons | T2 | T3 (occurrenceCount=1 case) + existing ReplyCard tests |
| AC-3: cold load → never "Offline" | T4 | T5 (hasLoadedOnce=false, 11s) |
| AC-4: post-connect drop → offline | T4 | T5 (hasLoadedOnce=true, 11s, historyError) |
| AC-5: unit tests a+b | T3, T5 | T3 and T5 |
| AC-6: no regression to existing tests | T6 | T6 review + `make test` |

---

## Edge cases and mitigations

### E1: occurrenceCount is 0 (edge case from pre-dedup data)
`(occurrenceCount ?? 1) > 1` evaluates to `false` for 0, so the card shows buttons. Correct:
a count of 0 means the field was not populated (old record), not that there were zero events.

### E2: user taps immediately on cold load before stream connects
Buttons not disabled (`isOffline=false`). RPC fires. If server is reachable, it succeeds.
If server is down, RPC fails → `retryable` phase → Retry shown. Fail-closed.

### E3: occurrenceCount is a lifetime total (CORRECTED after review)
The first draft claimed an answered-then-read question restarts the count at 1. It does not:
`server/notifications/store.go` `findDuplicate`/`mergeOccurrence` keep one record per
(session, type), increment `OccurrenceCount` forever and only flip `IsRead` back to false. So
`occurrenceCount > 1` alone would disable Reply for every question after the first in a session.
The signal is now `question_pending_count`, stamped by `registerQuestion` from the live
`PendingQuestionStore` (registered, unexpired, unclaimed entries of the session). The
`occurrenceCount > 1` rule remains only as a fallback when that key is absent. Residual:
a question answered in the terminal stays registered until its 30 min TTL, so a later question
in that window reads as parallel (fail-closed).

### E4: WatchSessions stream never connects (server dead at startup)
`hasLoadedOnce` stays `false`. `isOffline` stays `false`. User tap → RPC error → Retry shown.
This is a degraded-but-safe experience; "Connecting..." copy could be added in a follow-up.

---

## What is explicitly NOT tested

- Remote-caller refusal (non-loopback)
- Race with a permission dialog
- Free-text digit rejection
- Real phone soft keyboard
- The deep parallel fix (REQ-Q4 / future RPC)

---

## Test execution plan

```bash
# Run all unit tests for the affected files
cd web-app && npx jest --testPathPatterns="replyQuestion|ReplyCard|useNotificationConnectivity" --no-coverage

# Full unit suite (no coverage run needed for a fix this size)
cd web-app && npx jest --no-coverage

# Build check
make build

# Full CI gate
make ci
```

All tests must be green before the PR is opened.

---

## Open questions

1. **Should "Connecting…" be shown explicitly in the card during cold load?** The plan silently
   keeps buttons enabled (not disabled). An explicit "Connecting…" status message could help
   users who load the deep link on a slow connection. Deferred to a follow-up unless the owner
   asks for it.

2. **Is `occurrenceCount > 1` the right long-term signal for parallel questions?** It could
   also be triggered by a question that fired, was answered elsewhere, and then fired again —
   though that would be a new notification record (not a dedup merge). Current dedup semantics
   mean any merge increments the count, so this is the safest proxy for "more than one question
   arrived for this session while it was unread."
