# BUG-026: TransitionBacklogItemStatus's Precondition Is a Read-Then-Write Race, Letting a Stale Reopen Clobber an Already-Shipped Item [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-20)
**Discovered**: 2026-07-20 — live incident on backlog item `0fd4a940-b7c9-4270-9708-307c08821d44` ("Backlog Rich text")
**Fixed**: 2026-07-20 — `session/ent_repository_backlog.go`, `TransitionBacklogItemStatus`
**Impact**: A backlog item that had already legitimately shipped (PR merged, item at `done`) was silently reopened and left permanently stuck in `review`, hitting both `STUCK_REASON_ABANDONED_REVIEW` and `STUCK_REASON_REWORK_CAP`. Any status transition guarded by `BacklogItemPrecondition` was vulnerable to the same class of race — the CAS guard callers relied on for correctness was not actually atomic.

## Resolution Summary

**Fix Applied**: `TransitionBacklogItemStatus`'s precondition check now happens inside the same SQL statement as the write, via a conditional `BacklogItem.Update().Where(id, status = ExpectedStatus, updated_at = ExpectedUpdatedAt)`, instead of a separate `Get()` read followed by an unconditional `UpdateOneID().Save()`. This mirrors the atomic conditional-update pattern already used elsewhere in the same file (`ResolveStuck`, `MarkStuckNotified`).

**Before**:
```go
current, err := r.client.BacklogItem.Get(ctx, parsedID)
// ...
if precondition != nil {
    if precondition.ExpectedStatus != "" && current.Status != precondition.ExpectedStatus {
        return nil, fmt.Errorf("%w: ...", ErrPreconditionFailed)
    }
    // ...
}
// Gap here: any other writer can land a transition before this next line runs.
item, err := r.client.BacklogItem.UpdateOneID(parsedID).
    SetStatus(string(toStatus)).
    SetUserModifiedStatusAt(now).
    Save(ctx)
```

**After**:
```go
update := r.client.BacklogItem.Update().Where(backlogitem.ID(parsedID))
if precondition != nil {
    if precondition.ExpectedStatus != "" {
        update = update.Where(backlogitem.StatusEQ(precondition.ExpectedStatus))
    }
    if precondition.ExpectedUpdatedAt != nil {
        update = update.Where(backlogitem.UpdatedAtEQ(*precondition.ExpectedUpdatedAt))
    }
}
affected, err := update.
    SetStatus(string(toStatus)).
    SetUserModifiedStatusAt(now).
    Save(ctx)
// affected == 0 means the precondition no longer held at write time — reported
// as ErrPreconditionFailed, not silently applied.
```

## Problem Description

`TransitionBacklogItemStatus` is the CAS-guarded status-transition primitive the whole backlog lifecycle state machine is built on. Every caller that passes a non-nil `BacklogItemPrecondition{ExpectedStatus, ExpectedUpdatedAt}` believes it is getting an atomic compare-and-swap — see `AutoReopenAfterFailedReview`'s own comment: *"Transition review → in_progress with a precondition to guard against races (e.g. concurrent manual reopen firing at the same time)."*

In reality the old implementation checked the precondition against a row fetched by a separate `Get()` call, then performed the actual write via an unrelated, unconditional `UpdateOneID(...).Save(ctx)`. Nothing tied the two together — any other goroutine's transition landing in the gap between the `Get()` and the `Save()` was invisible to the check, and the stale write would still succeed.

## Live Incident Reproduction (2026-07-20)

Backlog item `0fd4a940-b7c9-4270-9708-307c08821d44` had a real, merged PR (#176, `db11cf56`). Its `statusEvents` show, all within ~2 seconds:

```
review -> pr_pending   2026-07-20T14:47:38.842504579Z
pr_pending -> done      2026-07-20T14:47:41.126919526Z
done -> in_progress     2026-07-20T14:47:42.101464427Z
in_progress -> review   2026-07-20T14:47:42.190223776Z
```

The first two transitions are the item legitimately shipping (PASS verdict → PR → merge). The final two are the bug: `AutoReopenAfterFailedReview` (`server/services/backlog_service_triage.go:664`) had, at some earlier point, read the item while it was still `review` and captured `updatedAt` for its precondition. By the time its (queued/async — the actual reopen call is dispatched via `go func()` from `autoReopenWithBackoffGate`, and is itself gated by an exponential remediation backoff that can defer it for a long time) `TransitionBacklogItemStatus(itemID, in_progress, precondition{ExpectedStatus: review, ExpectedUpdatedAt: T0})` call actually executed, the item had already raced past it to `done` via the ship path above. The old non-atomic precondition check read `current.Status == "review"` from a `Get()` that happened *before* the ship landed, so the check "passed" — and the subsequent unconditional write clobbered `done` back to `in_progress`. `AutoReopenAfterFailedReview`'s own follow-up `SpawnSessionFromItem` call then failed (the item's real state was inconsistent with what the reopen path expected), triggering its unconditional (no-precondition) rollback write straight to `review` — the final observed transition, landing the item exactly where it was found: stuck in `review`, `STUCK_REASON_ABANDONED_REVIEW` and (briefly) `STUCK_REASON_REWORK_CAP` both open, despite already having a merged PR.

## Root Cause

Classic TOCTOU (time-of-check-to-time-of-use): the precondition check and the write it was meant to guard were two separate, non-transactional database round trips, so a concurrent writer could invalidate the check's premise in the gap between them without the write ever noticing. This is exactly the failure mode `.claude/rules/go-double-checked-locking.md` warns about for in-memory state, applied here to a database-backed "CAS."

## Files Affected

- `session/ent_repository_backlog.go` — `TransitionBacklogItemStatus` (the fix)
- `server/services/backlog_service_triage.go` — `AutoReopenAfterFailedReview` (the caller whose precondition this race defeated; unchanged — its precondition usage was already correct, the primitive underneath it was not)

## Live Item Remediation

Item `0fd4a940-b7c9-4270-9708-307c08821d44` is still incorrectly sitting in `review` as of this fix (its merged PR #176 predates the fix, so it was never re-processed). This PR does not include an automated one-time reconciliation for it — the fix closes the race going forward, but retroactively repairing already-corrupted rows needs an operator decision (the `abandoned_review`/`rework_cap` stuck rows are also still open and carry their own context worth preserving or clearing deliberately). **Operator action needed**: use the Backlog UI's manual status controls (or `TransitionBacklogItemStatus` directly) to move item `0fd4a940-b7c9-4270-9708-307c08821d44` to `done`, since PR #176 is already merged.

## Verification

- `TestTransitionBacklogItemStatus_should_rejectStaleReopen_When_ItemAlreadyShippedSinceReview` (`session/ent_repository_backlog_transition_test.go`) — reproduces the exact incident shape: a precondition captured while `review`, a legitimate concurrent ship all the way to `done`, then the stale transition attempt. Asserts `ErrPreconditionFailed` and that `done` survives untouched.
- `TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently` (same file) — two goroutines race a real concurrent CAS with a start barrier; asserts exactly one winner, exactly one `ErrPreconditionFailed`, and a single consistent final status.
- `go test ./session -run TestBacklogIntegration` — full existing backlog integration suite still green.
- `make build && make test && make lint` — see PR for results (one pre-existing, unrelated `session/tmux` sandbox flake noted; no new failures).

## Related

- `session/ent_repository_backlog.go`'s `ResolveStuck` / `MarkStuckNotified` already used the correct atomic `Update().Where(...)` pattern this fix brings `TransitionBacklogItemStatus` in line with.
- `UpdateBacklogItem` (same file) has an identical non-atomic precondition shape, but as of this fix has no live caller passing a non-nil precondition — tracked as a follow-up, not fixed here to keep this change scoped to the primitive actually implicated in the incident.
