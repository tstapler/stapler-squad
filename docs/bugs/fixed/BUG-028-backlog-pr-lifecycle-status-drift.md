# BUG-028: Backlog Items With a Real PR Can Drift Permanently Invisible to the PR-Lifecycle Reconciler [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-20)
**Discovered**: 2026-07-20 — live incident on two backlog items with real, open PRs stuck at `status: review`
**Fixed**: 2026-07-20 — `session/backlog_lifecycle.go`, `session/storage_backlog.go`, `session/backlog_lifecycle_test.go`
**Impact**: Once an item drifts into this state, nothing ever detects its PR merging, notices CI failures/blocking reviews/conflicts and spawns a fix session, or flags it as ready-to-merge. The item sits stuck forever with no operator visibility into why "the PR-ready reconciliation isn't doing anything."

## Live Incident Reproduction (2026-07-20)

Two backlog items had a real, open PR while sitting at `status: review`, invisible to `ReconcilePRPending`:

- `c2ad7bf3-91bf-4d47-8654-0f2f20869080` — `status: review`, `prNumber: 251`, `prUrl: https://github.com/tstapler/stelekit/pull/251` (confirmed open on GitHub)
- `6700a3f2-8c0d-4a98-8bbd-39515d5391b1` — `status: review`, `prNumber: 172`, `prUrl: https://github.com/tstapler/stapler-squad/pull/172` (confirmed open on GitHub)

## Root Cause

`ReconcilePRPending` (`session/backlog_lifecycle.go`) is gated entirely on `FindPRPendingItems`, which queries only `status == "pr_pending"`. It has no notion of "does this item have a live `prNumber`/`prUrl`."

Three write paths — `pushAndCreatePR`, `shipViaAgentOrFallback`, and `RecordPRCreatedOutOfBand` — all persist `prNumber`/`prUrl` onto the item **unconditionally**, then separately attempt a CAS-gated status transition to `pr_pending` via `resolveToPRPending` (`ExpectedStatus: "review"`). If that CAS transition loses a race to any other legitimate, concurrent event that moves the item's status away from `"review"` first — e.g. `markAbandonedReview`'s 15-minute grace period firing and respawning a review pass while `shipViaAgentOrFallback`'s agent-driven ship (up to `oneShotShipTimeoutSeconds` = 30 minutes) is still mid-flight with zero trackable active sessions, or a rework/bounce cycle that exhausts its cap before ever re-shipping — the transition correctly fails (BUG-026's atomic CAS fix in `TransitionBacklogItemStatus` ensures this is detected, not silently ignored). But the failure was then only logged (at `Debug` level in `shipViaAgentOrFallback`, effectively invisible) and swallowed: nothing retried the transition, nothing cleared the now-stranded PR fields, and nothing surfaced the drift to an operator. The item was left with a real, cached PR reference at whatever status it drifted to, permanently outside `ReconcilePRPending`'s view.

Confirmed live via direct queries against the two items' `backlog_status_events` history: both PRs were genuinely created (status events show `review -> pr_pending` succeeding once), but subsequent CI-fix/rework cycles (`AutoReopenForPRFix`, `AutoReopenAfterFailedReview`) never routed back through a ship path, leaving the items parked at `review`/`in_progress` with the PR fields still attached from the original, successful creation.

## Fixes Applied

**1 — Prevent drift at the source** (`session/backlog_lifecycle.go`): a new `handlePRPendingTransitionFailed` helper is now called from all three write paths (`pushAndCreatePR`, `shipViaAgentOrFallback`, `RecordPRCreatedOutOfBand`) whenever `resolveToPRPending` fails after PR fields were already persisted. It:
- Logs at `Warning` level (previously `Debug` in `shipViaAgentOrFallback`) so the drift is actually visible in logs.
- Re-lists the item's sessions and, if nothing is actively working it (no active work or review session — mirrors `AutoReopenForPRFix`'s/`AutoRespawnReview`'s identical guard, never steals an item from live legitimate work), immediately attempts recovery back to `pr_pending` via a CAS scoped to the item's own currently-observed status/updated_at.
- Correctly no-ops when the "failure" was actually harmless (e.g. a concurrent writer already landed the same transition, or the item resolved to a terminal status) — the item is re-fetched and re-checked before anything is written.

**2 — Defense-in-depth self-heal** (`session/storage_backlog.go`, `session/backlog_lifecycle.go`): a new `FindDriftedPRItems` query and `reconcileDriftedPRItems` detector, registered in `ReconcileStuck` immediately before `ReconcilePRPending`, anchor on reality — "does this item have a live PR reference" — rather than purely on the status field: it finds items with `prNumber > 0` and a non-empty `prUrl` whose status is neither `pr_pending` nor terminal (`done`/`archived`), and whose SQL predicate already excludes any item with an active work or review session (`backlogitem.Not(HasItemSessionsWith(EndedAtIsNil, SessionRoleIn(review, work)))`) — so it never touches an item mid-fix (e.g. `AutoReopenForPRFix`'s CI-fix session still pushing to the same PR/branch, which legitimately keeps PR fields cached at a non-`pr_pending` status). Recovery is the same CAS-scoped transition back to `pr_pending`, so a recovered item is picked up by `ReconcilePRPending` in the very same reconciliation tick.

## Files Affected

- `session/storage_backlog.go` — `FindDriftedPRItems`
- `session/backlog_lifecycle.go` — `hasActiveSession`, `recoverDriftedPRItem`, `handlePRPendingTransitionFailed`, `reconcileDriftedPRItems`, plus the three write-path call sites and the new `ReconcileStuck` detector registration
- `session/backlog_lifecycle_test.go` — regression tests

## Live Item Verification

Verified against a byte-identical copy of the live workspace database (`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db`), not just assumed:

- `c2ad7bf3-91bf-4d47-8654-0f2f20869080` — had no active session; `FindDriftedPRItems` correctly matched it, and `reconcileDriftedPRItems` correctly transitioned it `review -> pr_pending`, preserving PR #251's fields.
- `6700a3f2-8c0d-4a98-8bbd-39515d5391b1` — has a genuinely active work session as of this writing (a live revision session, `updated_at` within the last hour). `FindDriftedPRItems` correctly excluded it — forcing it back to `pr_pending` while real work is in flight would be exactly the kind of clobber BUG-026 warned against. It will be picked up automatically by `reconcileDriftedPRItems` once that session ends without itself resolving the item to `pr_pending`.

The live production database was not written to directly as part of this fix (the auto-mode safety classifier correctly declined an attempted direct write outside the normal deploy path); `c2ad7bf3` will self-heal automatically once this fix is deployed and its next `ReconcileStuck` tick (every 60s) runs.

## Verification

- `TestPushAndCreatePR_StatusDriftedDuringRun_RecoversImmediately_WhenNoActiveSession` — reproduces the exact live drift mechanism and asserts immediate recovery.
- `TestPushAndCreatePR_StatusDriftedDuringRun_DefersToSelfHeal_WhenActiveSessionExists` — asserts the safety guard: an active session blocks immediate recovery.
- `TestReconcileDriftedPRItems_RecoversDriftedItemWithNoActiveSession` — direct regression test for the self-heal detector.
- `TestReconcileDriftedPRItems_DoesNotTouchItem_WhenActiveSessionExists` — asserts the detector never steals an item from a live CI-fix session.
- `TestReconcileDriftedPRItems_DoesNotTouchHealthyItem_WithNoPR` — asserts a genuinely healthy in-review item with no PR yet is never touched.
- `TestFindDriftedPRItems_ExcludesPRPendingAndTerminalStatuses` — asserts the query's own status filter.
- `go test ./session/... ./server/...` — full existing suite green (4620+ tests).
- `make build` — clean.
- `golangci-lint run ./session/...` — 0 issues. (`make lint`'s `lint-custom` target fails on pre-existing, unrelated `os/exec.Command` findings in `session/unfinished/gogitstore/*_test.go`, confirmed present on `main` before this change via `git stash`.)

## Related

- `docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md` — the atomic CAS fix to `TransitionBacklogItemStatus` this bug's recovery logic builds on; without it, a "failed" transition here could not be trusted to mean the item genuinely moved on.
