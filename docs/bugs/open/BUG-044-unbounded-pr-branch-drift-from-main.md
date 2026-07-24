# BUG-044: PR Branches Drift Unboundedly From Main, Eventually Producing an Unmergeable, Unreviewable Diff That Reads as "No Related Work" [SEVERITY: High]

**Status**: 🔴 Open
**Discovered**: 2026-07-23/24, investigating why item `693c2700`'s review kept failing with "the diff contains no code related to the backlog item ID feature at all"
**Impact**: A backlog item's PR branch can silently accumulate unbounded drift from `main` across a multi-day item lifecycle, with no circuit breaker. Eventually the branch becomes unmergeable (GitHub reports `CONFLICTING`) and its diff is dominated by thousands of lines of unrelated upstream drift rather than the item's actual work — which then fails review as "unrelated," triggers the `bouncing` auto-respawn loop (burning remediation attempts, see BUG-043), and ultimately parks the item needing human intervention, even though the real feature work was done correctly.

## Live Evidence

Item `693c2700-d6b8-4d98-aaa4-c0e5eb2d42c5` ("Expose ID functionality in Backlog"), branch `backlog/stapler-squad-expose-backlog-item-id`, PR #213:

- Branch diverged from `main` at commit `3c563eb6` (**2026-07-18 20:09**, "Merge origin/main (benchmark baseline auto-updates)"). Item was created 2026-07-13; by review time (2026-07-23) `main` had advanced **289 commits** past that merge-base.
- `gh pr view 213 --json mergeable` → `"CONFLICTING"`. `gh pr view 213 --json additions,deletions,changedFiles` → **405 files, 78,210 insertions, 3,067 deletions** — nearly all of it upstream drift (the entire backlog item-detail redesign, event-driven-updates work, etc. that landed on `main` after this branch's divergence point), not this item's own change.
- The branch's own commit log (first-parent from the merge-base) shows the real work is genuinely present:
  ```
  4c81cf3a feat(backlog): expose item ID with copy/deep-link and fix board restore
  99d6942e chore(backlog): commit pending catch-up-with-main work from prior sessions
  950c10a9 feat(backlog): expose item ID with copy/deep-link and fix board restore  (earlier attempt)
  ```
  One attempted catch-up commit (`99d6942e`) exists but clearly didn't close the gap — the branch is still 289 commits behind days later.
- The `abandoned_review`/`bouncing` circuit breaker (see BUG-043, fixed 2026-07-23) correctly stopped auto-respawning after two identical review failures, parking the item with context: *"the diff contains no code related to the backlog item ID feature at all... entirely unrelated infrastructure changes."* The reviewer wasn't wrong about what it saw — the diff genuinely was unreadable — it just misattributed the cause to the work itself rather than to branch staleness.

## Root Cause

`AutoReopenForPRFix`'s `syncPRBranchWithMain` (`server/services/backlog_service_triage.go`, called from the PR-fix reopen path) is the only mechanism that resyncs a stuck PR branch with `main`, and it is explicitly **best-effort**:

> "This is preventive rather than reactive... Never blocks the spawn — any failure here is logged and swallowed."

On a real conflict, `git.MergeMainIntoWorktree` aborts the merge and returns a `Conflicted` result; `syncPRBranchWithMain` turns that into a text note appended to the fix session's context ("resolving these conflicts... is part of this fix") but does **not** itself land any resync — resolution is left entirely to whatever LLM session picks up the fix context next. If that session doesn't fully resolve and push the merge (or the item cycles through review again before a human/agent gets to it), the branch is exactly as far behind on the next cycle, plus whatever landed on `main` in the meantime. There's no cap on how far this can drift and no separate detection for "this branch is now too far behind main to be reviewable," so the drift compounds silently across every work/review cycle until the diff itself becomes the review failure.

## Suggested Fix Direction

1. **Make the main-sync a precondition of review, not a best-effort side effect of the fix-retry path.** Before a review session is spawned (`TriggerReReview`/`AutoRespawnReview`, or the normal work→review transition), check how far the branch is behind `main` (`git rev-list --count HEAD..main` equivalent) and require a successful sync (or an explicit, surfaced conflict-resolution step) before proceeding to review — rather than only attempting a sync reactively after a PR-fix failure.
2. **Add a drift threshold as its own detectable stuck condition** — e.g. a new `StuckReason` (or a check folded into the existing `bouncing` detector) for "branch is N commits/days behind main and hasn't synced," surfaced on `/unfinished` with a clear, actionable message ("this branch needs a manual rebase/merge with main") rather than only discovering the problem after review has already failed and misdiagnosed it as bad work.
3. Consider whether `effectiveReworkCap`/the rework-session cap should account for elapsed wall-clock time since branch creation, not just session count — an item that's been open 5+ days with many work/review cycles but zero successful main-syncs is a different failure mode than one that's genuinely bouncing on review feedback, and conflating them (as `abandoned_review`/`bouncing` currently do) makes the parked message misleading, as it was here.

## Recommended Routing

`sdd:fix-bug`, though this is more architecturally involved than the other backlog fixes from this pass (BUG-040/041/043) since it likely touches the review-gate entry conditions, not just a single reconciler function — may warrant `sdd:full` or at least a `sdd:quick` planning pass rather than a pure mechanical fix. Concrete repro: item `693c2700` (still parked, needs a human "Reopen for Revision" per BUG-043's finding — that action alone won't fix this, since the underlying branch will still be catastrophically behind `main`). A live, low-risk first step for whoever picks this up: manually sync `693c2700`'s branch with current `main` and confirm the *next* review pass actually finds the real feature work, to further validate this root cause before generalizing the fix.
