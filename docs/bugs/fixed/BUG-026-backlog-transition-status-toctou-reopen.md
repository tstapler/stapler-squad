# BUG-026: Stale Reconciliation Writes Could Reopen an Already-Shipped Backlog Item [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-20)
**Discovered**: 2026-07-20 — live incident on backlog item `0fd4a940-b7c9-4270-9708-307c08821d44` ("Backlog Rich text")
**Fixed**: 2026-07-20 — `session/ent_repository_backlog.go`, `server/services/backlog_service_triage.go`, `session/backlog_lifecycle.go`
**Impact**: A backlog item that had already legitimately shipped (PR #176 merged, item at `done`) was silently reopened and left permanently stuck in `review`, hitting both `STUCK_REASON_ABANDONED_REVIEW` and `STUCK_REASON_REWORK_CAP`. Three independent, compounding defects combined to produce this: a non-atomic CAS primitive, an unconditional rollback write, and a reconciliation sweep with no notion of "this verdict was already handled."

## Live Incident Reproduction (2026-07-20)

Backlog item `0fd4a940-b7c9-4270-9708-307c08821d44` had a real, merged PR (#176, `db11cf56`) — merged well before any of the timestamps below. Its `backlog_status_events` table (queried directly against the live workspace DB, not just the 4-event summary that first surfaced this) shows:

```
pr_pending  -> done         2026-07-20 00:22:34.712  (legitimate: PR #176 merges)
done        -> review       2026-07-20 07:47:03.772  (BUG #1 — no audit note)
review      -> pr_pending   2026-07-20 07:47:38.842  (BUG #2 — stale verdict reprocessed)
pr_pending  -> done         2026-07-20 07:47:41.126  (duplicate reship, lands back on done)
done        -> in_progress  2026-07-20 07:47:42.101  (a second, concurrent stale trigger)
in_progress -> review       2026-07-20 07:47:42.190  (spawns a redundant 2nd review session)
```
*(local PDT timestamps from the DB; UTC = +7h, matching the originally reported `14:47:38–42Z` window)*

Cross-referencing `item_sessions` / `review_verdicts` for this item: its only two review-role sessions are one from **2026-07-19 11:26–11:28 PDT** (verdict: PASS — the one that legitimately drove the `pr_pending -> done` at `00:22:34`) and one from **2026-07-20 07:47:42–08:13:08 PDT** (created *by* this incident, ended without ever submitting a verdict — the source of the still-open `STUCK_REASON_ABANDONED_REVIEW`). No review session exists to explain the `07:47:03` or `07:47:38` transitions — both are stale reconciliation writes acting on data left over from the *first, already-concluded* review cycle.

## Root Causes (three, compounding)

### 1. `TransitionBacklogItemStatus`'s precondition was a read-then-write race (TOCTOU)

The precondition (`ExpectedStatus`/`ExpectedUpdatedAt`) was checked against a row fetched by a separate `Get()` call, then the actual write went through an unrelated, unconditional `UpdateOneID(...).Save(ctx)`. Nothing tied the two together — a concurrent writer landing in the gap between the `Get()` and the `Save()` was invisible to the check, and the stale write would still succeed. This is the same class of bug `.claude/rules/go-double-checked-locking.md` warns about for in-memory state, here applied to a database-backed "CAS" that every caller (`AutoReopenAfterFailedReview` et al.) relied on for correctness.

### 2. `AutoReopenAfterFailedReview`'s rollback used `precondition: nil` (unconditional overwrite)

When `AutoReopenAfterFailedReview` (`server/services/backlog_service_triage.go`) successfully transitions `review -> in_progress` but its follow-up `SpawnSessionFromItem` call then fails, it rolls the item back to `review` — but the rollback passed `precondition: nil`, an *unconditional* write with no audit note. If, by the time the rollback runs, some other legitimate process has already moved the item on (e.g. shipped it to `done`), the rollback silently clobbers that state with zero record of why — this is exactly the `done -> review` transition at `07:47:03.772` with no note in the audit log.

### 3. `reconcileUnprocessedReviewVerdicts` had no notion of "this verdict was already consumed"

`FindReviewItemsWithUnprocessedVerdict` matches any item in `review` status whose *most recent* review-role session is dead and has a verdict — full stop. Nothing marks a verdict as "already applied." Once bug #2 forced the item back into `review` (with no *new* review-role session created), this sweep found the **prior, already-shipped** review session's PASS verdict still sitting there as "most recent," treated it as fresh, and reprocessed it with `forcePush=true` — reshipping the item (`review -> pr_pending -> done`, `07:47:38–41`) as a side effect of a bug, not a real review cycle.

The final `done -> in_progress -> review` hop (`07:47:42`) is a second, concurrent stale trigger racing the same window — closed by fix #1, since any such call's `TransitionBacklogItemStatus` precondition can no longer succeed once the row has moved past what the call captured.

## Fixes Applied

**#1 — `session/ent_repository_backlog.go`, `TransitionBacklogItemStatus`**: the precondition now lives inside the same SQL statement as the write, via a conditional `BacklogItem.Update().Where(id, status = ExpectedStatus, updated_at = ExpectedUpdatedAt)` instead of a separate `Get()` read followed by an unconditional `UpdateOneID().Save()`. `affected == 0` is reported as `ErrPreconditionFailed`, not silently applied. Mirrors the atomic conditional-update pattern already used elsewhere in the same file (`ResolveStuck`, `MarkStuckNotified`).

**#2 — `server/services/backlog_service_triage.go`, `AutoReopenAfterFailedReview`**: the rollback-to-`review` call now passes a precondition scoped to the `in_progress` row this same function call just wrote (`ExpectedStatus: in_progress`, `ExpectedUpdatedAt` from that write's own result), instead of `nil`. The rollback only fires if nothing else has touched the item since.

**#3 — `session/backlog_lifecycle.go`, `reconcileUnprocessedReviewVerdicts`**: before acting on a review session's verdict, the sweep now checks the review session's `CreatedAt` against the item's most recent transition *into* `review` (`GetMostRecentStatusEventAt`). A session created before the item's current review-entry cannot be the outcome that stay is meant to be judged on — it belongs to a prior, already-concluded cycle, and is skipped.

## Files Affected

- `session/ent_repository_backlog.go` — `TransitionBacklogItemStatus` (fix #1)
- `server/services/backlog_service_triage.go` — `AutoReopenAfterFailedReview` (fix #2)
- `session/backlog_lifecycle.go` — `reconcileUnprocessedReviewVerdicts` (fix #3)

## Live Item Remediation

Item `0fd4a940-b7c9-4270-9708-307c08821d44` self-corrected during this investigation (its `abandoned_review` and `rework_cap` stuck rows show `resolved_at` timestamps from the existing (unpatched) live server's own remediation machinery, and `GetBacklogItem` now reports `status: done` with PR #176 still linked). No manual reconciliation was needed by the time this PR was opened — confirmed live via `mcp__stapler-squad__get_backlog_item` and a direct query against `~/.stapler-squad/workspaces/*/sessions.db`.

## Verification

- `TestTransitionBacklogItemStatus_should_rejectStaleReopen_When_ItemAlreadyShippedSinceReview` (`session/ent_repository_backlog_transition_test.go`) — reproduces the precondition race directly: captured while `review`, a legitimate concurrent ship all the way to `done`, then the stale transition attempt. Asserts `ErrPreconditionFailed` and that `done` survives untouched. Confirmed to pass against old code too in isolation (the internal `Get()` inside the old implementation was still fresh per-call) — see the concurrent test below for the version that actually distinguishes old vs. new.
- `TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently` (same file) — two goroutines race a real concurrent CAS; asserts exactly one winner, exactly one `ErrPreconditionFailed`, and a single consistent final status.
- `TestReconcileUnprocessedReviewVerdicts_should_skipStaleVerdict_When_ItemReenteredReviewAfterAlreadyShipping` (`session/backlog_lifecycle_stuck_test.go`) — the direct regression test for fix #3: ships an item once via the real crash-recovery path, forces it back into `review` with no new review session, and asserts the sweep no longer reprocesses the stale verdict. **Verified to fail against the pre-fix code** (item incorrectly re-shipped to `done` a second time) and pass with the fix.
- `go test ./session/... ./server/services/...` — full existing suite green.
- `make build && make test && make lint` — see PR for results (one pre-existing, unrelated `session/tmux` sandbox flake noted on the full `make test` run — passes in isolation; no new failures).

## Related

- `session/ent_repository_backlog.go`'s `ResolveStuck` / `MarkStuckNotified` already used the correct atomic `Update().Where(...)` pattern fix #1 brings `TransitionBacklogItemStatus` in line with.
- `UpdateBacklogItem` (same file) has an identical non-atomic precondition shape to the one fixed in #1, but no live caller currently passes a non-nil precondition to it. Tracked as a follow-up, not fixed here, to keep this change scoped to the primitives actually implicated in the incident.
