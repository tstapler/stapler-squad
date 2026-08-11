# BUG-039: A Freshly-Spawned Work Session's Own Base Commit Is Trivially "On Main," So `reconcileBouncingItems` Auto-Marks Brand-New Items Done Within ~1 Minute of Spawn [SEVERITY: Critical]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22 — live incident, ~1 minute after unblocking two queued items (BUG-038's fix). Both `e1fb6825-39b2-4f06-9bf8-c9d1678a6824` and `12981e9d-0ad5-4a79-af99-a2be35b22856` were auto-transitioned to `done` within the first reconcile tick after their work sessions spawned — one of them (`e1fb6825`) had a live session actively producing real work at the exact moment it happened; the other (`12981e9d`) went on to complete all 6 real acceptance criteria in a worktree that could never ship, since its autonomous driver's own `in_progress→review` transition then failed with `"concurrent modification detected: expected status \"in_progress\", got \"done\""`.
**Fixed**: 2026-07-22 — `session/backlog_lifecycle.go`
**Impact**: `reconcileBouncingItems`' no-PR fallback (`mostRecentWorkCommitShippedToMain`) checks whether a work session's current HEAD commit is an ancestor of `main` to decide "did this item's code ship outside the normal PR flow." A freshly created worktree's HEAD is, by construction, identical to the commit it branched from — and a branch's own base commit is *always* an ancestor of `main` (that is literally where it came from). The check therefore returns "shipped" for **any** item in its first moments after spawn, before the agent has made a single commit, on every 60-second reconcile tick. This is not an edge case — it fires deterministically for every newly dequeued item, and is a critical-severity bug because it silently destroys real, in-progress (or about-to-be-genuinely-completed) work by marking the item `done` with no PR ever opened.

## Live Incident

```
[SpawnSessionFromItem] starting autonomous driver item=e1fb6825... session=f48da8a2...  (20:25:02)
[DequeueNextQueuedItems] dequeued and spawned item=e1fb6825... session=f48da8a2...        (20:25:02)
[BacklogLifecycle] reconcileBouncingItems item=e1fb6825... → done (commit 5d77b70b... shipped to main without a PR)  (20:25:57)
```

`5d77b70b...` was `main`'s tip at the moment the worktree was created (this session's own earlier BUG-037 commit) — not any code the new work session had produced. 55 seconds after spawn, before any real work happened, the item was marked `done`.

The same tick hit `12981e9d` identically:
```
[DequeueNextQueuedItems] dequeued and spawned item=12981e9d... session=33ccf320...  (20:25:10)
[BacklogLifecycle] reconcileBouncingItems item=12981e9d... → done (commit 5d77b70b... shipped to main without a PR)  (20:25:57)
...
[AutonomousDriver] failed to transition backlog item item=12981e9d to=review err="concurrent modification detected: expected status \"in_progress\", got \"done\""  (20:28:13)
```
The work session went on to complete all 6 real acceptance criteria (confirmed via `report_progress`, which doesn't itself check item status), but by the time it tried to transition to `review` for a real human/reviewer look, the item was already `done` — its own driver's protocol had no way to recover, since nothing tells an `AutonomousDriver` "actually, you're not done, resume."

## Root Cause

`mostRecentWorkCommitShippedToMain` (`session/backlog_lifecycle.go`) resolves the work session's *current, live* HEAD via `resolveLatestWorkCommit` — this is correct and was specifically hardened (2026-07-21, per that function's own doc comment) to never trust the stale `ItemSessionSummary.LastCommitSha` field, which is only ever seeded once at spawn with the base SHA. But that 2026-07-21 fix addressed the *stale-field* instance of this problem, not the deeper structural one: even a **live, correctly-resolved, genuinely-current** HEAD SHA is indistinguishable from "shipped" when the branch simply hasn't diverged from its base yet — which is the state of every branch for the first several minutes (or longer, for a slow-starting agent) after creation. `git.IsCommitOnMain` has no way to know "this SHA is on main because it *is* main's own tip, unchanged" versus "this SHA is on main because real work landed there."

## Fix Applied

`mostRecentWorkCommitShippedToMain`: before calling `IsCommitOnMain`, compare the resolved `sha` against the work session's own recorded `BaseCommitSHA` (`GitWorktreeData`, already tracked for exactly this "where did this branch diverge from main" purpose — used elsewhere for diff computation). If they're equal **and** the work session's branch is not `bounceMainBranch` itself, there are no new commits yet — return `(sha, false)` (not shipped) instead of falling through to the trivially-true `IsCommitOnMain` check. The branch-name exclusion preserves the legitimate sibling case (`TestReconcileBouncingItems_should_transitionToDone_When_ShippedWithoutPR`): work committed directly to `main` with no separate feature branch ever used has `sha == base` by construction too, but that genuinely *is* shipped — there's no "hasn't diverged yet" state possible when the branch already is main.

## Files Affected

- `session/backlog_lifecycle.go` — `mostRecentWorkCommitShippedToMain`
- `session/backlog_lifecycle_stuck_test.go` — two new regression tests

## Verification

- `TestReconcileBouncingItems_should_notTreatFreshBranchBaseAsShipped_When_ZeroCommitsYet` — a feature branch created from main's tip with zero commits (the exact live-incident shape) must not be auto-marked done; must still go through normal bouncing detection.
- `TestReconcileBouncingItems_should_stillTransitionToDone_When_WorkCommittedDirectlyToMainBranch` — confirms the fix doesn't regress the legitimate "worked directly on main, no feature branch" case the existing `ShippedWithoutPR` test covers.
- **Verified to fail against pre-fix code**: `git stash` on `backlog_lifecycle.go` alone reproduces the exact live failure — the new test's item transitions to `done` instead of staying `in_progress`, with the stuck-bouncing row never created.
- All 7 pre-existing `TestReconcileBouncingItems_*` tests (including the 2026-07-21 stale-field regression test) pass unmodified — 9/9 total in the suite.
- `go build ./...`, `golangci-lint run ./session/...` — clean.

## Live Data Recovery

Both corrupted items were manually restored after this fix was verified (not before — restoring first would have let the same bug immediately re-corrupt them on the next tick):
- `12981e9d` (Unfinished page CSS work) — genuinely completed all 6 acceptance criteria in its worktree; restored to a state where it can actually ship the real, finished work rather than sitting falsely `done` with no PR.
- `e1fb6825` (workspace peer awareness) — restored to let its live, actively-working revision session continue and reach a real review/ship cycle.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Semantic/Intent gap — `IsCommitOnMain(sha)` answers "is sha reachable from main," which is a different question from the one `mostRecentWorkCommitShippedToMain` actually needs answered: "did this item's *own new work* land on main." Those two questions coincide for any branch that has diverged, and silently diverge (pun intended) for any branch that hasn't yet — a distinction the code never encoded.

**Earliest achievable enforcement**: The regression test is the practical level. A stronger structural fix worth flagging: `mostRecentWorkCommitShippedToMain`'s true precondition is "the work session has produced at least one commit of its own" — expressing that as an explicit, named check (e.g. a `hasNewCommits(sha, baseSHA, branch)` helper reused everywhere this "did real work happen" question is asked) rather than inline base-comparison logic would make the invariant harder to accidentally omit at a future call site.

**Recurring shape**: This is the *second* base-commit/main-ancestry false-positive found in this exact function within 24 hours (2026-07-21's stale-field bug, this one 2026-07-22) — both are variations of "a branch's base commit is trivially an ancestor of main, and something conflated that with 'shipped.'" Strongly suggests the underlying operation (git.IsCommitOnMain, and by extension the entire `closeIfSupersededByMain`/`mostRecentWorkCommitShippedToMain` family this session's BUG-036 also touched) deserves a single, well-tested, base-aware helper rather than each call site reasoning about base commits independently — flagged for the architecture-review pass already recommended repeatedly today.

## Related

- The 2026-07-21 stale-field bug this fix's sibling test (`TestReconcileBouncingItems_should_stillFlag_When_LastCommitShaIsStaleBaseSeed`) already covers — same function, adjacent but distinct root cause.
- BUG-036 (`docs/bugs/fixed/BUG-036-reconcile-pr-closed-branch-missing-superseded-check.md`) — touches the sibling `closeIfSupersededByMain`, which uses the same `IsCommitOnMain` primitive against a *different* commit source (a work session's `LastWorkCommitSha` via `ItemSessionSummary`, not a live-resolved worktree HEAD) — worth checking whether that call site has the same base-commit exposure if it's ever reached for a freshly-spawned rework session with zero new commits.
- BUG-038 (`docs/bugs/fixed/BUG-038-queued-items-silently-blocked-by-unapproved-plan.md`) — the fix that, by unblocking two long-stuck queued items, caused fresh worktree spawns and surfaced this bug within a minute.
