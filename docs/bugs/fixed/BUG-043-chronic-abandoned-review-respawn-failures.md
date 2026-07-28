# BUG-043: `abandoned_review` Remediation Burns Its Entire Attempt Budget on Foregone-Conclusion Respawns When a Separate `bouncing` Backoff Gate Silently Blocks the Reopen [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-23)
**Discovered**: 2026-07-23, live audit of items stuck in `review` for many hours
**Fixed**: 2026-07-23 — `session/backlog_lifecycle.go`, `session/backlog_remediation.go`
**Impact**: Three real backlog items (`e1fb6825`, `693c2700`, `12981e9d`) sat in `review` status for 8–20+ hours with no active session and no PR, each having burned 4 of 5 automated `abandoned_review` remediation attempts without resolving. **The originally-filed hypothesis for this bug (a `SendKeys`/`SessionDriver` initial-prompt injection failure on freshly spawned review sessions, mirroring BUG-041's dead-pane shape) was confirmed FALSE by a live trace.** The actual mechanism is unrelated to tmux/`SessionDriver` entirely: these review respawns go through `TriggerReReview`'s **headless** path (a synchronous LLM call, no tmux session, no `SendKeys` anywhere in the flow), which correctly produces a real, valid review verdict (FAIL) on every single attempt. The verdict is simply never *acted on* — the reopen-for-rework step that a FAIL verdict should trigger is gated by a **separate, independently-clocked** `bouncing` remediation backoff that was already deep in its own cooldown, silently discarding every fresh verdict `abandoned_review` produced. `abandoned_review`'s remediation kept "succeeding" (real review, real verdict, on-topic) while making zero forward progress, until it too exhausted its 5-attempt budget and parked — with a generic "use Reset to retry" notification that never mentioned `bouncing` was the actual blocker.

## Live Evidence

**Pre-investigation DB state** (`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db`, `backlog_stuck_states`): all three items at `abandoned_review` 4/5 attempts, `status=review`, `pr_number=0`, no active session — matching the originally-filed report.

**Live trace** (forced the next attempt via `TriggerRemediationNow` on item `12981e9d` rather than waiting ~10h for the scheduled 08:17 attempt, then tailed `~/.stapler-squad/logs/staplersquad.log`):

```
[BacklogLifecycle] item 12981e9d...: review session headless-re-review-e62a8678... has an unprocessed FAIL verdict — applying it now
[BacklogLifecycle] handleReviewSessionExited item=12981e9d... outcome=FAIL (review session headless-re-review-e62a8678...)
[BacklogLifecycle] autoReopenWithBackoffGate item=12981e9d...: bouncing remediation backoff not yet due, skipping auto-reopen
[TriggerReReview] headless re-review complete for item 12981e9d... (outcome FAIL, path=diff, duration_ms=15809)
[AutoRespawnReview] item 12981e9d... re-review triggered
[BacklogService] TriggerRemediationNow item=12981e9d... reason=abandoned_review: this was the final attempt before parking
```

Critically, **no tmux session was ever created** for this respawn (`tmux list-sessions | grep review` showed nothing new), and **no `SendKeys` call appears anywhere in this flow** — `TriggerReReview` took the `s.headlessPool != nil` branch (`server/services/backlog_service_triage.go:1894`), which runs the review as a direct, blocking LLM call and writes the verdict straight to the DB. The BUG-041-style hypothesis (dead pane, `SendKeys` failing 3x, `sentInitial` set anyway) simply does not apply to this code path.

Watching subsequent 60-second sweep ticks confirmed the deadlock is **structural, not transient** — the identical block repeated on every tick with the exact same (by-then-stale) session:

```
22:29:57  handleReviewSessionExited ... outcome=FAIL (headless-re-review-6ecec378...)
22:29:57  autoReopenWithBackoffGate ...: bouncing remediation backoff not yet due, skipping auto-reopen
22:30:57  handleReviewSessionExited ... outcome=FAIL (headless-re-review-6ecec378...)   ← same session, reprocessed
22:30:57  autoReopenWithBackoffGate ...: bouncing remediation backoff not yet due, skipping auto-reopen
22:31:57  handleReviewSessionExited ... outcome=FAIL (headless-re-review-6ecec378...)   ← same session, reprocessed again
22:31:57  autoReopenWithBackoffGate ...: bouncing remediation backoff not yet due, skipping auto-reopen
```

DB state after the forced attempt confirmed `abandoned_review` reached 5/5 (parked, `next_remediation_at` pushed to 2026-07-26) while `bouncing` sat independently at 4/5 with `next_remediation_at` ≈ 2026-07-24 08:49 — a fully separate clock that had been ticking since an earlier, unrelated bounce cycle.

## Root Cause (confirmed, supersedes the filed hypothesis)

Two remediation reasons on the *same* item govern two *sequential* halves of one recovery flow, but are gated by two **independently-scheduled** backoff clocks:

1. **`abandoned_review`** (`markAbandonedReview` → `AutoRespawnReview` → `TriggerReReview`): produces a fresh review verdict. This step was working correctly the entire time.
2. **`bouncing`** (`handleReviewSessionExited` → `autoReopenWithBackoffGate` → `AutoReopenAfterFailedReview`): the *only* thing that can turn a FAIL/PARTIAL/UNVERIFIABLE verdict into forward progress (reopening the item to `in_progress` for rework). This is gated on `StuckReasonBouncing`'s own `RemediationDue` check (`session/backlog_lifecycle.go:906`) — a completely separate `BacklogStuckState` row with its own attempt counter and its own exponential backoff schedule (30m/2h/8h/24h/72h), typically started earlier by an *earlier* bounce cycle unrelated to the current `abandoned_review` occurrence.

Because nothing links these two clocks, `markAbandonedReview` kept spending its own limited attempt budget respawning a review whose diff could not possibly have changed (the item was never reopened, so no rework happened) — producing the *identical* FAIL verdict on every attempt, which then hit the *same* still-not-due `bouncing` gate every time. Each of these attempts was a **foregone conclusion**: real work was done (a genuine ~15s headless LLM review call), a correct result was produced, and none of it could ever help, because the step that mattered was gated shut by a clock `abandoned_review` had no visibility into. `abandoned_review` eventually exhausted its own 5-attempt cap this way and parked, at which point its only remaining automated action (`markAbandonedReview`) also stops firing — permanently deadlocking the item, since the sole caller of `autoReopenWithBackoffGate` is a fresh verdict, and nothing produces a fresh verdict anymore.

This is the same *general* "silent dead end, nothing detects it" shape as `StuckReasonSpawnFailed` (`session/domain/backlog.go`) and BUG-040 — a real failure gets converted into "looks fine, will retry later" with no distinguishable signal — but the specific mechanism (two independently-clocked backoff gates governing sequential steps of one compound recovery, with no cross-linkage) is new to this codebase's bug history.

## Fix Applied

`markAbandonedReview` now checks, **before** calling `RemediationDue(abandoned_review)` (i.e. before spending an attempt), whether the item's `bouncing` gate is currently closed — via a new read-only peek, `Storage.RemediationBlocked` (`session/backlog_remediation.go`), which reuses the existing pure `evaluateRemediation` decision function without mutating any row or consuming any attempt:

```go
// session/backlog_remediation.go
func (s *Storage) RemediationBlocked(ctx context.Context, itemID string, reason domain.StuckReason) (blocked bool, err error) {
    row, ok, err := s.findOpenStuckStateForReason(ctx, itemID, reason)
    if err != nil {
        return false, fmt.Errorf("remediation blocked %s/%s: %w", itemID, reason, err)
    }
    if !ok {
        return false, nil
    }
    switch evaluateRemediation(row, time.Now(), serverStartTime) {
    case remediationSkippedParked, remediationSkippedNotDue:
        return true, nil
    default:
        return false, nil
    }
}
```

```go
// session/backlog_lifecycle.go, inside markAbandonedReview, before RemediationDue(abandoned_review)
if blocked, blockedErr := l.storage.RemediationBlocked(ctx, itemID, domain.StuckReasonBouncing); blockedErr != nil {
    log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview RemediationBlocked(bouncing) item=%s: %v", itemID, blockedErr)
} else if blocked {
    log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview item=%s: skipping respawn — a fresh verdict would be discarded by the bouncing reopen gate, which is not due yet; not spending an abandoned_review attempt on a foregone conclusion", itemID)
    return
}
```

When `bouncing` has no open row yet (first-time flow) or is currently due, `RemediationBlocked` reports `false` and the respawn proceeds exactly as before — this only changes behavior in the specific deadlocked case. Query errors fail open (proceed with the respawn) rather than silently stalling an item that might otherwise be fine.

This directly stops `abandoned_review` from ever parking as a side effect of a downstream gate it can't control: instead of spending all 5 attempts on identical, discarded verdicts, `markAbandonedReview` now waits, keeping its own attempt budget intact, until `bouncing`'s clock actually opens — at which point the very next scheduled respawn's verdict has a real chance to reach `AutoReopenAfterFailedReview`.

## Files Affected

- `session/backlog_remediation.go` — new `Storage.RemediationBlocked` read-only gate peek
- `session/backlog_lifecycle.go` — `markAbandonedReview` checks `RemediationBlocked(bouncing)` before spending an `abandoned_review` attempt
- `session/backlog_lifecycle_test.go` — new regression test

## Verification

- `TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue` — seeds a `bouncing` row with one consumed attempt and a 2h-future `next_remediation_at` (mirroring the live DB state on all three affected items), then asserts `reconcileStuckReviewItems` (which calls `markAbandonedReview`) does **not** dispatch to the fake `ReviewRespawner` and does **not** increment the `abandoned_review` row's `remediation_attempts`.
- **Verified to fail against pre-fix code**: stashed the fix (`session/backlog_lifecycle.go`, `session/backlog_remediation.go`), reran the new test — failed with `must not respawn while the bouncing reopen gate is not due ... got call for item=<uuid>`, exactly the pre-fix behavior. Restored the fix; test passes.
- Existing coverage unaffected: `TestMarkAbandonedReview_AutoRespawnsReview_OncePastGrace` and `TestMarkAbandonedReview_NoRespawn_WhenNoReviewRespawnerConfigured` both still pass — the fix only changes behavior when `bouncing` is genuinely blocking.
- `make build` — full proto/ent/web-UI/Go build, clean.
- `go test ./session/... ./server/services/...` — full suite green.
- `golangci-lint run ./session/...` — 0 issues.
- `gofmt -l` on all changed files — no output (clean).

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Integration Gap — two independently-owned subsystems (`abandoned_review`'s respawn-a-verdict action and `bouncing`'s act-on-a-verdict action) share a single compound recovery flow but were never designed with awareness of each other's backoff state. Each one's local behavior (respect my own backoff schedule, spend my own attempt budget) is individually correct; the *composition* is what silently fails.

**Earliest achievable enforcement**: The regression test is close to the earliest practical level — this is inherently cross-subsystem runtime coordination (two DB rows' independent schedules), not something a type system or lint rule can express. The one systemic improvement beyond the regression test: `RemediationBlocked` is now a small, reusable, well-documented primitive in `backlog_remediation.go` alongside `RemediationDue` — any *future* remediation action that depends on a downstream gate can reuse it directly rather than re-discovering this failure mode from scratch. No other reason pairs in the current codebase have this same sequential-dependency shape (checked: `stale_work`, `autonomous_stuck`, `push_failed`, `pr_pending_no_pr` each drive their own independent, self-contained action), so a lint rule enforcing "every remediation action must declare its downstream dependencies" would be speculative machinery for a class with exactly one known member today — flagged as a watch item rather than built now.

**Recurring shape**: This is the third instance in this codebase's bug history of the general "a real action succeeds but its result silently cannot be acted upon, and nothing tells the operator why" family (alongside `StuckReasonSpawnFailed` and BUG-040's PR-reference dead end) — but the *specific* sub-shape ("two independently-clocked backoff gates governing sequential steps of one compound recovery flow, with no cross-linkage") is new. Worth naming for future audits: **"gate chaining without gate awareness"** — any time remediation action B's value depends on remediation action A's downstream gate being open, and A and B are scheduled independently, expect the same silent-deadlock pattern until A also exhausts its own budget.

## Related

- Supersedes the originally-filed hypothesis in this same document, which attributed the failure to `session/session_driver.go:307-341`'s `SendKeys`/`sentInitial` give-up path (the BUG-041 dead-pane shape). Live trace confirmed that code path is never reached by any of the three affected items — `TriggerReReview`'s headless branch has no tmux/`SessionDriver` involvement at all. `session/session_driver.go:335-341`'s `sentInitial = true`-after-giveup behavior remains as originally written; it may still be worth hardening independently, but it is not implicated in this bug.
- Distinct from BUG-041 (`docs/bugs/fixed/BUG-041-backlog-nudge-retry-never-backs-off.md`) — that bug was a genuine dead-pane `SendKeys` retry-forever loop on an already-running work session's nudge; this bug has no dead pane and no `SendKeys` involved anywhere.
- Same general "silent dead end" family as BUG-040 (`docs/bugs/fixed/BUG-040-pr-pending-item-loses-pr-reference-dead-end.md`) and `StuckReasonSpawnFailed` (`session/domain/backlog.go`).
