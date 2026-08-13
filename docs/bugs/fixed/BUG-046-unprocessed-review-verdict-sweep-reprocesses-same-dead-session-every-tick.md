# BUG-046: `reconcileUnprocessedReviewVerdicts` Reprocesses the Same Dead Review Session on Every Sweep Tick, Spamming Notifications While the Backoff Gate Correctly Blocks Any Real Action [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-07-24)
**Discovered**: 2026-07-24, live on the mobile Alerts page — a single "Review session ended without a verdict" notification on item `12981e9d` reached `occurrence_count: 95` over ~94 minutes (one per ~60s tick), still climbing.
**Fixed**: 2026-07-24 — `session/backlog_lifecycle.go`
**Impact**: Once a review session exits without calling `submit_review_verdict` while its item's `bouncing` backoff gate isn't due yet, the periodic reconcile sweep re-detects and re-"handles" the exact same dead session on every tick — re-logging a WARNING, re-firing an ERROR notification (deduped into one record, but its `occurrence_count`/`last_occurred_at` keep incrementing forever), and re-invoking the gated reopen path, which correctly no-ops but is invoked regardless. No real state ever changed and the backoff gate itself was working correctly — the bug was purely that the sweep had no way to distinguish "already handled, just gated" from "never handled," so it treated the same condition as fresh on every pass. Purely a notification/log-spam and wasted-work bug — not a stuck-item or data-loss bug (the underlying item, `12981e9d`, continued progressing normally via a separate live work session throughout).

## Live Evidence

Notification record (`~/.stapler-squad/workspaces/d685c4b1a423cca3/notifications.json`):
```json
{
  "session_id": "12981e9d-0ad5-4a79-af99-a2be35b22856",
  "title": "Review session ended without a verdict",
  "message": "Unfinished page needs CSS work for sizing — the review session exited without calling submit_review_verdict. Treating as a failed review.",
  "created_at": "2026-07-24T11:40:32-07:00",
  "occurrence_count": 95,
  "last_occurred_at": "2026-07-24T13:14:55-07:00"
}
```
95 occurrences over 94 minutes ≈ one every 59 seconds — matching this sweep's poll cadence, not a legitimate one-review-attempt-per-occurrence cadence (the `bouncing` backoff schedule's tiers are 30min/2h/8h/24h/72h; nothing in this system legitimately retries a full review cycle every 60 seconds). Correlated log pattern (`~/.stapler-squad/logs/staplersquad.log`), also recurring every ~60s for the same item ID:
```
DEBUG ReactiveQueueManager user interaction 12981e9d-0ad5-4a79-af99-a2be35b22856
DEBUG ReactiveQueueManager instance not found 12981e9d-0ad5-4a79-af99-a2be35b22856
```

## Root Cause (confirmed)

`reconcileUnprocessedReviewVerdicts` (`session/backlog_lifecycle.go:1609`) is a periodic sweep that finds items whose most recent review-role session is dead with no verdict, and calls `handleReviewSessionExited(ctx, ..., forcePush=true)` (line 1672) to process it. Its own doc comment already identified the general shape of this risk: *"FindReviewItemsWithUnprocessedVerdict has no notion of 'already consumed'... if the item later re-enters 'review' for any other reason... this sweep would treat that stale, already-shipped verdict as fresh and reprocess it."* The existing guard against that (comparing the dead session's `CreatedAt` against the item's most recent transition-into-review timestamp, `GetMostRecentStatusEventAt`) only catches the "item left review and came back" case — it did **not** catch the case live here: the item **never leaves `review` at all**, because `handleReviewSessionExited`'s no-verdict branch (`session/backlog_lifecycle.go:870-883`, pre-fix) called `l.notify(...)` **unconditionally**, then called `autoReopenWithBackoffGate` (line 943), which checks `RemediationDue(ctx, itemID, domain.StuckReasonBouncing)` and — correctly, when the backoff gate isn't due yet — just logs and returns without transitioning the item anywhere. Since the item's review-entry timestamp never advances (nothing transitioned it), the sweep's existing guard didn't skip it on the next tick either, and the identical dead session got "handled" again: same WARNING log, same `notify()` call (incrementing the deduped notification's `occurrence_count`), same gated no-op reopen attempt. Repeated every sweep tick until the backoff gate finally opened (or the item's state changed for some unrelated reason).

## Fix Applied

Mirrors the idempotency pattern BUG-043 established (`session/backlog_remediation.go`'s `Storage.RemediationBlocked` — a read-only peek at whether a `StuckReason`'s remediation gate is currently closed, added by that fix and left unchanged here). In `handleReviewSessionExited`'s no-verdict branch, check `RemediationBlocked(ctx, item.ID, domain.StuckReasonBouncing)` **before** deciding whether to notify/log:

```go
// session/backlog_lifecycle.go, handleReviewSessionExited's no-verdict branch
if reviewEntry == nil || reviewEntry.ReviewVerdict == nil {
    blocked, blockedErr := l.storage.RemediationBlocked(ctx, item.ID, domain.StuckReasonBouncing)
    if blockedErr != nil {
        log.WarningLog.Printf("[BacklogLifecycle] handleReviewSessionExited RemediationBlocked(bouncing) item=%s: %v", item.ID, blockedErr)
    }
    if !blocked {
        log.WarningLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s review session %s exited without a verdict", item.ID, reviewIS.SessionUUID)
        l.notify(item.ID,
            "Review session ended without a verdict",
            fmt.Sprintf("%s — the review session exited without calling submit_review_verdict. Treating as a failed review.", item.Title),
            7, 3,
        )
    }
    l.autoReopenWithBackoffGate(ctx, item.ID, item.Title)
    return
}
```

`autoReopenWithBackoffGate`'s own gating logic (`RemediationDue`) is unchanged and still correctly no-ops until the gate opens — this fix only changes whether the redundant notify+WARNING-log fires on top of that no-op. When no `bouncing` row exists yet (the very first detection of this failure shape for an item) or the row's backoff has genuinely elapsed, `RemediationBlocked` reports `false` and behavior is identical to before this fix — a fresh detection still notifies and still attempts the gated reopen. It only suppresses the notify+log when the `bouncing` gate is already known-blocked (mid-backoff or parked), which is exactly the condition under which reprocessing the same dead session can produce no new information. Query errors fail open (proceed with the notify) rather than silently going quiet, per this codebase's established fail-open convention for gate-check errors (see `autoReopenWithBackoffGate`'s identical `RemediationDue` error handling directly below it).

Called at two sites (`onSessionExited`'s real-time review-role case, `forcePush=false`, and `reconcileUnprocessedReviewVerdicts`'s crash-recovery sweep, `forcePush=true`) — the fix applies uniformly to both, since the underlying signal (whether the `bouncing` gate is already known-blocked for this item) is equally meaningful regardless of which caller detected the no-verdict exit; a genuinely new no-verdict exit arriving while the gate is still cooling down from an earlier one is exactly as redundant to notify about as the sweep reprocessing the identical `SessionUUID`.

## Correlated `ReactiveQueueManager ... instance not found` Log Pattern — Confirmed Separate, Minor, Cosmetic Issue

Traced this during the investigation. `l.notify(item.ID, ...)` (this call site and effectively all backlog-lifecycle `notify()` calls) uses the **backlog item ID**, not a real tmux session UUID, as the notification's `session_id` field — that's why the live notification JSON's `session_id` (`12981e9d-0ad5-4a79-af99-a2be35b22856`) is actually the item ID. The frontend's `useAuditLog` hook (`web-app/src/lib/hooks/useAuditLog.ts`) calls the `LogUserInteraction` RPC with that same field verbatim whenever a user (or the mobile app) views/interacts with the notification (`server/services/review_queue_service.go:247-289` → publishes `events.NewUserInteractionEvent(sessionID, ...)`). `ReactiveQueueManager.handleUserInteraction` (`server/review_queue_manager.go:252-273`) then tries to resolve that value via `FindInstance(sessionID)`, which always fails since it's an item ID, not a session UUID — producing the DEBUG `instance not found` log, an early-return no-op with no functional impact.

This mismatch is **general** — it applies to essentially every backlog-item notification whose `notify()` call passes `item.ID`, not something specific to this bug's no-verdict path. Its ~60s recurrence in the live evidence is far more likely explained by the mobile Alerts page's own refresh/view cadence (re-rendering the same still-unread, top-of-list notification and re-firing a "view" interaction log each time) than by any shared root cause with BUG-046's sweep-reprocessing defect — the two just happened to correlate because the same still-open notification was both being re-notified (BUG-046) and being re-viewed (this separate issue) on similar cadences. **Not fixed here** — cosmetic, DEBUG-level only, and out of this bug's scope; worth its own filing if the log noise becomes a signal-to-noise problem (e.g. giving `Notification` an explicit "this session_id is actually an item ID" flag, or having the frontend only call `LogUserInteraction` for notifications it knows are session-shaped).

## Files Affected

- `session/backlog_lifecycle.go` — `handleReviewSessionExited`'s no-verdict branch checks `RemediationBlocked(bouncing)` before notifying/logging
- `session/backlog_lifecycle_test.go` — new regression test

## Verification

- `TestHandleReviewSessionExited_NoVerdict_NotifiesOnlyOnce_AcrossRepeatedSweepTicks` — reproduces the realistic timeline: tick 1 fires before any `bouncing` row exists (a genuinely fresh detection — must notify, and does), a `bouncing` row then opens mid-backoff between ticks (mirroring `reconcileBouncingItems` tripping independently, the exact live DB state BUG-043 found on this same item), then tick 2 reprocesses the identical dead `SessionUUID` with the gate now blocked and asserts the notifier's title list is unchanged (still length 1) — i.e. no second notification.
- **Verified to fail against pre-fix code**: stashed the fix (`session/backlog_lifecycle.go` only, keeping the new test), reran the regression test — failed with `Not equal: expected: []string{"Review session ended without a verdict"} / actual: []string{"Review session ended without a verdict", "Review session ended without a verdict"}`, exactly the pre-fix double-notify behavior. Restored the fix; test passes.
- Existing coverage unaffected: `TestHandleReviewSessionExited_NoVerdict_NotifiesAndInvokesAutoReopener`, `TestHandleReviewSessionExited_Fail_InvokesAutoReopener`, `TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue`, and the rest of the `handleReviewSessionExited`/`autoReopenWithBackoffGate` suite all still pass — the fix only changes behavior when the `bouncing` gate is genuinely already blocked.
- `make build` — full proto/ent/web-UI/Go build, clean.
- `go test ./session/...` — full package suite green.
- `golangci-lint run ./session/...` — 0 issues.

## Reflection

**Classification**: Same general family as BUG-043 — a correctly-gated downstream action (`autoReopenWithBackoffGate`'s reopen) paired with an *ungated* side effect (`notify()`+log) that assumed "this branch ran" implies "this is new information," when in fact the branch can legitimately run repeatedly for the identical underlying condition once nothing transitions the item out of the state that keeps re-triggering it.

**Earliest achievable enforcement**: The regression test is close to the earliest practical level — this is inherently about repeated-invocation idempotency against a stateful gate (a DB row's backoff schedule), not something a type system or lint rule can express. `RemediationBlocked` (added by BUG-043) is now a shared, documented primitive doing double duty for exactly this shape — any future remediation-adjacent side effect that only wants to fire "once per blocked cycle" rather than "once per invocation" can reuse it directly.

**Recurring shape**: The specific sub-shape here — "an always-executed side effect co-located with a correctly-gated action, where the side effect has no gate of its own" — is a sibling of, but distinct from, BUG-043's "two independently-clocked gates governing sequential steps of one recovery flow." Both are instances of the same broader family this codebase keeps re-discovering: a real action succeeds/no-ops correctly in isolation, but the surrounding bookkeeping (notify, log, attempt-budget) doesn't know enough about that gate's state to avoid producing misleading repetition on top of it.

## Related

- Builds directly on BUG-043's `Storage.RemediationBlocked` primitive (`docs/bugs/fixed/BUG-043-chronic-abandoned-review-respawn-failures.md`) rather than introducing a new one.
- Same general "silent dead end" / gate-awareness family as BUG-040 (`docs/bugs/fixed/BUG-040-pr-pending-item-loses-pr-reference-dead-end.md`) and `StuckReasonSpawnFailed` (`session/domain/backlog.go`).
