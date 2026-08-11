# BUG-048: `autonomous_stuck` Rows Opened by a Stuck Review-Role Session Have No Remediation Path — `next_remediation_at` Drifts Forever Without Ever Firing [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-07-24)
**Discovered**: 2026-07-24, same investigation as BUG-047 — backlog item `12981e9d` ("Unfinished page needs CSS work for sizing", PR #210) had an `autonomous_stuck` stuck-state row whose `next_remediation_at` was over 2 hours in the past (`2026-07-24 12:09:32`, checked against a current time of `14:09`+) with `resolved_at` still `NULL`, and no code path was ever going to act on it.
**Fixed**: 2026-07-24 — `server/services/autonomous_orchestration_service.go`
**Impact**: For items that run through the autonomous pipeline (`queued_autonomous=1`), a **review-role** autonomous driver session that exits without producing a verdict opens a durable `autonomous_stuck` stuck-state row (via the shared `MarkStuck` call that fires for *any* role's non-`Done` exit) — but the remediation/respawn logic that's supposed to act once `next_remediation_at` is due only exists for the **work**-role case. A review-role stuck row has no analogous respawn call, so once opened, it sits forever: `next_remediation_at` becomes arbitrarily overdue and nothing ever revisits it, clears it, or surfaces it beyond the one-time notification fired when it was created. In this investigation this was a secondary, latent finding — not the primary explanation for item `12981e9d`'s specific 2.5-hour stall (that was BUG-047, and the item's live work session was still genuinely active/idle-waiting, not blocked by this gap) — but it is a real gap that will bite the next item whose review-role autonomous session gets stuck with no live work session left to eventually resolve it via the unconditional `resolveAutonomousStuck` call on a later `Done` transition.

## Live Evidence

```
$ sqlite3 sessions.db "SELECT reason, first_detected_at, remediation_attempts, next_remediation_at, resolved_at
                        FROM backlog_stuck_states WHERE item_id='12981e9d-...' ORDER BY first_detected_at;"

bouncing          2026-07-22 22:16:01  attempts=2  next_remediation_at=2026-07-24 14:12:32  resolved_at=NULL   (this gate IS ticking — confirmed live, see below)
autonomous_stuck  2026-07-24 11:39:32  attempts=1  next_remediation_at=2026-07-24 12:09:32  resolved_at=NULL   (2+ hours overdue, never revisited)
```

Log grep for `autonomous_stuck` across the current log window shows every occurrence originates from `onAutonomousDriverComplete`'s shared `MarkStuck`/`MarkStuckNotified` calls; there is no corresponding `RemediationDue(autonomous_stuck)`/respawn log line anywhere for a review-role session — only the work-role branch ever calls `RemediationDue`.

By contrast, the sibling `bouncing` gate for the same item **was** confirmed actively ticking: its `next_remediation_at` moved from `14:12:32` to `22:13:05` between two checks minutes apart during this investigation, proving the periodic reconcile sweep is alive and does act once a gate's deadline passes — `autonomous_stuck` (for a review-triggered occurrence) simply has no equivalent action to take.

## Root Cause (confirmed by code read)

`server/services/autonomous_orchestration_service.go`, `onAutonomousDriverComplete`:

```go
// Applies to ANY role — Triage, Work, or Review — whenever the driver exits
// without a DONE signal:
if !outcome.Done {
    if _, markErr := concreteStorage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck, ...); markErr != nil {
        ...
    } else if _, notifyErr := concreteStorage.MarkStuckNotified(ctx, item.ID, domain.StuckReasonAutonomousStuck); notifyErr != nil {
        ...
    }
}

switch is.Role {
case session.SessionRoleTriage:
    // stuck triage: notify only, item stays at 'idea'
case session.SessionRoleWork:
    if !outcome.Done {
        // *** the only place RemediationDue(autonomous_stuck) + AutoRespawnAutonomousWork
        //     are ever called ***
        due, justParked, gateErr := concreteStorage.RemediationDue(ctx, itemID, domain.StuckReasonAutonomousStuck)
        ...
        if due {
            go func() { respawner.AutoRespawnAutonomousWork(...) }()
        }
    }
case session.SessionRoleReview:
    // Only resolves the row on outcome.Done == true. A stuck (non-Done) review
    // exit falls through to "log and return" with NO remediation attempt at all.
    if outcome.Done {
        a.resolveAutonomousStuck(ctx, concreteStorage, item.ID)
    }
    log.Info("[AutonomousDriver] skipping status transition for role", "role", is.Role, "item", item.ID)
    return
}
```

So: a review-role session that exits without a verdict correctly opens/refreshes the `autonomous_stuck` row (same as work-role would), but the `case session.SessionRoleReview` branch has no analog to work-role's `RemediationDue` + `respawner.AutoRespawnAutonomousWork` block — it just logs and returns. The row's `next_remediation_at` is set once at creation and never checked again by anything, because nothing ever calls `RemediationDue(autonomous_stuck)` for a review-originated occurrence.

The row *can* still be cleared eventually — but only as a side effect of something else entirely succeeding later: `resolveAutonomousStuck` is called unconditionally whenever *any* subsequent `onAutonomousDriverComplete` call has `outcome.Done == true` and successfully transitions the item (line ~426-428), or when a later review-role run itself completes with `outcome.Done == true` (line ~397-399). In other words, the row silently rides along, doing nothing, until something unrelated eventually resolves it as a bystander — not because anything actively remediated the review-role stuck condition itself.

## Suggested Fix

Add a review-role analog to the work-role respawn block: when `SessionRoleReview` exits with `!outcome.Done`, check `RemediationDue(ctx, itemID, domain.StuckReasonAutonomousStuck)` the same way work-role does, and if due, respawn a fresh headless review session for the item (likely via the same `reviewGateTrigger.TriggerReviewForSession` mechanism used after a work session completes, applied to the item's current review-role session context) instead of leaving the item to rely on the separate `bouncing`/`autoReopenWithBackoffGate` path in `session/backlog_lifecycle.go` to eventually reopen it back to `in_progress`. Needs design attention on:
- Whether respawning a review directly is preferable to just deferring to the existing `bouncing` backoff gate (which *does* independently work for this same "review exited without a verdict" condition, per `session/backlog_lifecycle.go`'s `handleReviewSessionExited`/`autoReopenWithBackoffGate`) — there may be two parallel systems both nominally responsible for the same recovery here (`session/backlog_lifecycle.go`'s bouncing-gate reopen vs. `autonomous_orchestration_service.go`'s autonomous_stuck respawn), and it's not obvious from the code alone whether `autonomous_stuck`'s review-role gap is a real second responder that's missing, or whether `bouncing`'s existing handling already fully covers this case and `autonomous_stuck` for review is effectively vestigial/redundant and should instead just be resolved (not respawned) once `bouncing` has already reopened the item.
- If the two systems are meant to be independent responders (defense in depth), the review-role branch should at minimum resolve its own `autonomous_stuck` row once `bouncing`'s reopen has already handled the same underlying condition, so it doesn't sit falsely "still open" indefinitely even when the item has, in fact, recovered via the other path.

## Recommended Routing

`sdd:fix-bug` or a short `plan:fix-bug` pass — the fix itself (adding a respawn/resolve branch) is small, but understanding how it should interact with the pre-existing `bouncing` gate in `session/backlog_lifecycle.go` (same condition, different subsystem) needs a deliberate design decision, not a mechanical port of the work-role branch. Add a regression test mirroring `TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue`'s shape but for a review-role `onAutonomousDriverComplete` exit, asserting that once `RemediationDue(autonomous_stuck)` reports true, *something* actually happens (respawn or explicit hand-off/resolve) — not just a log line.

## Related

- Discovered during the same investigation as BUG-047 (`docs/bugs/fixed/BUG-047-write-to-session-uses-newline-instead-of-carriage-return.md`), which was the confirmed primary cause of item `12981e9d`'s specific stall.
- Same general "gate exists, nothing ever checks it for this branch" family as BUG-043 (`docs/bugs/fixed/BUG-043-chronic-abandoned-review-respawn-failures.md`) and BUG-046 — this codebase has now surfaced this shape multiple times across different subsystems (`session/backlog_lifecycle.go`'s bouncing gate, `server/services/autonomous_orchestration_service.go`'s autonomous_stuck gate) independently.

## Fix Applied — Deviation from the Brief's Default Recommendation (Justified)

The filing brief's default recommendation was: "defer to `bouncing`, don't build a second responder — on `!outcome.Done`, resolve the row if the item already moved on, otherwise leave the row open as an honest signal and do not respawn." That structure (resolve-if-already-handled, otherwise no competing respawn) is exactly what was implemented — **but a live code read turned up solid evidence that `bouncing`'s existing machinery does not, by itself, ever revisit the "otherwise" branch**, which the brief explicitly invited deviating on if found. Both halves are covered below.

### Confirmed root cause (code read, `server/services/autonomous_orchestration_service.go`, `session/autonomous_driver.go`, `session/storage_backlog.go`, `session/backlog_lifecycle.go`)

The originally-filed root cause was correct as far as it went (`case session.SessionRoleReview` had no analog to work-role's `RemediationDue`/respawn block). Tracing where a review-role `autonomous_stuck` row *could* still get resolved turned up a second, more specific gap the brief asked to be checked for explicitly:

1. **`AutonomousDriver.run` never kills the underlying session on a stuck (turn-cap) exit** (`session/autonomous_driver.go:270-278`). It simply stops injecting turns and returns; the tmux/CLI process backing the review `Instance` stays alive and idle, and the review `ItemSession` row's `EndedAt` stays `nil` ("active") indefinitely — nothing else in the driver or its completion callback ever ends it.
2. **`bouncing`'s only entry point for "review exited without a verdict" is `handleReviewSessionExited`** (`session/backlog_lifecycle.go:816`), itself only invoked from `onSessionExited`, itself only invoked on a genuine `Instance` `EventExited`/`EventStopped` lifecycle event. A driver that merely stops driving a still-alive session never produces that event — `bouncing`'s `autoReopenWithBackoffGate` (the code path the brief's live evidence saw actively ticking on item `12981e9d`) is consequently never reached for *this* condition. The "bouncing IS ticking" evidence cited in the original filing was real, but for that item's `bouncing` row was independently opened and clocked by an earlier, unrelated bounce cycle — not by this review-role stuck occurrence, which is exactly why the filing correctly flagged this as a "secondary, latent finding" for `12981e9d` rather than its primary stall cause.
3. **The periodic `abandoned_review` detector also cannot see this item.** `FindStuckReviewItems` (`session/storage_backlog.go:700`) explicitly excludes any item with an `EndedAt`-nil review or work `ItemSession` — a deliberate "nothing in flight yet" guard, documented in its own comment. `FindZombieReviewItems`'s liveness-checker path also does not apply: the session is not *confirmed dead* (the tmux process is often still genuinely running, just no longer being driven), it is *abandoned by its driver*, a condition neither existing detector's query models.
4. **`reconcileStaleWorkSessions`, the one staleness sweep that does run periodically regardless of session exit, is scoped to `SessionRoleWork` and status `in_progress` only** (`session/backlog_lifecycle.go:1917-1943`) — it has no review-role counterpart.

Net effect: for a review-role autonomous driver that gives up after its turn cap with the underlying session still nominally alive, **no existing subsystem — not `bouncing`, not `abandoned_review`, not the stale-work sweep — ever gets a signal that would let it act.** The row genuinely has no path to remediation, confirming the bug's core complaint, and confirming (with live code evidence, per the brief's own invitation to check) that "just defer to `bouncing`" alone would not have closed the gap.

### Fix implemented

`onAutonomousDriverComplete`'s `case session.SessionRoleReview` (`server/services/autonomous_orchestration_service.go`) now branches three ways on a review-role exit, still with **zero new respawn/spawn logic**:

1. `outcome.Done` → unchanged: resolve any open `autonomous_stuck` row (this behavior predates this fix).
2. `!outcome.Done` **and** the item's status has already moved off `review` → resolve the `autonomous_stuck` row immediately. This covers the brief's "bouncing already handled it via a different path" case: whatever moved the item off `review` already addressed the underlying condition, so the row must not sit "falsely still open."
3. `!outcome.Done` **and** the item is still genuinely in `review` → **do not spawn a competing review session.** Instead, call `Storage.UpdateItemSessionEnded` on the review `ItemSession` row. This is pure bookkeeping — no new actor, no new session — but it directly closes gap #1/#3 above: once the row shows `EndedAt` set, the *existing*, already-`bouncing`-aware `abandoned_review` detector (`reconcileStuckReviewItems` → `markAbandonedReview`, itself hardened against racing `bouncing` by BUG-043's `RemediationBlocked` check) sees the item as "nothing in flight" on its very next `ReconcileStuck` tick (~60s) and takes over exactly as it already does for every other "review abandoned" scenario. `autonomous_stuck`'s own row is left open and honestly overdue only until that next tick resolves the underlying condition through the one subsystem that already owns it — not forever, and not via a second responder.

This keeps to the brief's "don't build a second responder" instruction to the letter (no `RemediationDue`/respawn call was added to the review branch at all) while fixing the specific, verified reason the brief's plain "defer" framing wouldn't have worked here: `bouncing` and `abandoned_review` both structurally require the session to look inactive before they can act, and nothing was ever making it look inactive.

## Verification

- `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ReviewStuck_ResolvesRow_When_ItemAlreadyMovedOn` — seeds an item already back at `in_progress` with an open `autonomous_stuck` row, fires a stuck (`outcome.Done=false`) review-role completion, asserts the row is resolved and the `ItemSession` is left untouched.
- `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ReviewStuck_EndsSession_NoCompetingRespawn` — seeds an item still in `review` with an active review `ItemSession`, fires a stuck review-role completion, asserts: the `autonomous_stuck` row survives (still an honest signal), the review `ItemSession`'s `EndedAt` is now set, and exactly one `ItemSession` still exists for the item (no competing session was spawned).
- **Verified to fail against pre-fix code**: stashed `server/services/autonomous_orchestration_service.go` only (kept the new tests), reran both new tests — both failed (`Should be empty, but was [...]` and `Expected value not to be nil`), matching pre-fix behavior exactly. Restored the fix; both pass.
- `go test ./server/services/... ./session/...` — full suite green (including `session/unfinished/gogitstore`, the slowest package at ~61s).
- `golangci-lint run ./server/services/...` — 0 issues.
- `go build ./server/services/...` and `go vet ./server/services/... ./session/...` — clean. (Root `go build ./...` fails on a pre-existing, unrelated `server/web/embed.go:8` `pattern all:dist: no matching files found` — the web UI dist bundle is not built in this environment; not caused by or related to this change.)

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Integration Gap — same family as BUG-043. Two (here, effectively three: `bouncing`, `abandoned_review`, and the never-added review-role `autonomous_stuck` remediation) independently-owned subsystems each assume "the session will eventually look inactive" is someone else's job to produce, and none of them produced it for the specific "driver gave up, session still alive" case.

**Earliest achievable enforcement**: The regression tests are close to the earliest practical level — this is cross-subsystem runtime state coordination (three independent DB-row/session-liveness signals), not something a type system or lint rule can express formally. No systemic enforcement beyond the two regression tests is being added: the fix itself *is* the systemic improvement (routing through the one existing, already-hardened `abandoned_review`/`bouncing` pipeline instead of adding a fourth independent responder), which is the point.

**Recurring shape**: This is at least the fourth instance of the "gate exists, nothing ever revisits it" / "a real subsystem exists but nothing ever hands this specific case to it" family this codebase has surfaced in the same investigation window (BUG-040, BUG-041, BUG-043, BUG-046). The specific sub-shape here is new and worth naming for future audits alongside BUG-043's "gate chaining without gate awareness": **"liveness-gated detector, no path to inactivity"** — any detector/gate that only acts once a session/resource "looks done" (an `EndedAt`, a `resolved_at`, a liveness check) is silently unreachable for any code path that abandons that resource without ever formally ending it. When adding a new "driver gives up" exit path anywhere in this codebase, check whether every downstream detector it might need to hand off to requires that kind of inactivity signal — and if so, whether anything actually produces it.
