# BUG-036: `ReconcilePRPending`'s "PR Closed Without Merging" Branch Doesn't Check for Already-Shipped Work Before Reopening, Unlike Its CI-Failing Sibling Branch [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22 — live incident, backlog item `9264efe7-b4c2-455a-9e2a-ab0196a63ecd` ("Backlog History feature Broken"), while manually recovering session `stapler-squad-fix-backlog-status-audit-trail-r16` from a BUG-031 paste/submit stall (see that bug's doc). Same item BUG-032 and BUG-029 were fixed against earlier the same day.
**Fixed**: 2026-07-22 — `session/backlog_lifecycle.go`
**Impact**: `ReconcilePRPending` has two branches that both ultimately decide whether to spawn another rework cycle for a `pr_pending` item: one for "PR is still open but CI-failing/blocked/conflicting," and one for "PR was closed without merging." BUG-032 added a `closeIfSupersededByMain` check to the first branch (skip the rework spawn if the item's code already landed on `main` through another path). That check was never added to the second, structurally identical branch — so an item whose PR gets closed (by a human, or by an autonomous session itself running `gh pr close` directly, bypassing this reconciler) because the work already shipped elsewhere still triggers a full wasted rework+review cycle, the exact waste BUG-032 was meant to eliminate.

## Live Incident

While unsticking session `stapler-squad-fix-backlog-status-audit-trail-r16` from a stuck paste (BUG-031), the resumed session independently re-verified BUG-032's original finding — `gh api compare` showed 0 files different from `main` (the item's fix had shipped via PR #198) — and closed the item's own stale PR #173 directly via `gh pr close`, with an explanatory comment. This is a *good* outcome (see BUG-031/user feedback on documenting AI decisions), but it happened entirely outside the app's own `closeIfSupersededByMain` mechanism.

Watching the item afterward: 60-second reconcile ticks kept running (`[BacklogLifecycle] stuck sweep tick: ... panicked=[] openRows=1`), but the item's status stayed at `pr_pending` for 7+ minutes with no forward progress. Tracing `ReconcilePRPending`'s logic against the item's state (PR closed, not merged) showed the item would hit the `prStatus.IsClosed` branch — which, unlike the CI-failing/blocked/conflicting branch below it, unconditionally clears the item's `PrNumber`/`PrURL` and calls `fixSpawner.AutoReopenForPRFix(...)`, restarting the item into `in_progress` for a rework cycle that isn't needed, since the code is already on `main`.

## Root Cause

`ReconcilePRPending` (`session/backlog_lifecycle.go`) has two sibling code paths reachable from the same `pr_pending` polling loop, both of which can end in `fixSpawner.AutoReopenForPRFix(...)`:

1. **"PR still open, unhealthy" branch** (CI-failing / has blocking reviews / has conflicts) — BUG-032 added a `closeIfSupersededByMain` check here, immediately before the `AutoReopenForPRFix` call, specifically to catch "this PR looks broken but the real reason is it's stale behind an already-shipped fix."
2. **"PR closed without merging" branch** (`prStatus.IsClosed`) — added earlier, specifically to stop a closed PR from being polled forever as if it were a healthy open one. It unconditionally clears `PrNumber`/`PrURL` and calls `AutoReopenForPRFix`, with **no** equivalent supersession check.

Both branches represent the identical underlying question — "does this PR's unhealthy/closed state mean the item's own code is actually broken, or has the real fix already shipped through some other path?" — but BUG-032's fix was only wired into branch 1. Branch 2 was not audited against the same failure shape at the time, even though it is reachable for exactly the same root cause (and, per this incident, is *more* likely to be hit once an autonomous session starts closing its own superseded PRs directly, since a manually `gh pr close`d PR always lands in this branch, never branch 1).

## Fix Applied

`session/backlog_lifecycle.go`, `ReconcilePRPending`: the `prStatus.IsClosed` branch now calls `l.closeIfSupersededByMain(ctx, g, &supersededItemData)` first — the same call, same helper, same trust boundary (`git.IsCommitOnMain` against the item's last work session's `LastCommitSha`) already used in branch 1 — before falling through to the existing clear-fields-and-reopen behavior. If the item's work is confirmed already on `main`, the helper closes the PR with an explanatory comment (a no-op/idempotent close-comment call if the PR was already closed manually, as in this incident), clears the PR fields, transitions the item straight to `done`, and posts a notification — exactly matching branch 1's existing behavior. If the check can't determine supersession (no work session, no commit SHA, `IsCommitOnMain` error, or the commit genuinely isn't on `main`), it no-ops and the existing "clear fields, reopen for fix" logic runs unchanged.

## Files Affected

- `session/backlog_lifecycle.go` — `ReconcilePRPending`'s `prStatus.IsClosed` branch, new `closeIfSupersededByMain` call site
- `session/backlog_lifecycle_test.go` — new `TestReconcilePRPending_ClosedPR_ClosesAsSupersededInsteadOfReopening_When_LastCommitAlreadyOnMain`

## Verification

- `TestReconcilePRPending_ClosedPR_ClosesAsSupersededInsteadOfReopening_When_LastCommitAlreadyOnMain` — mirrors BUG-032's own `TestReconcilePRPending_ClosesSupersededPR_When_LastCommitAlreadyOnMain` fixture (a real git repo whose `main` already contains the item's "last commit") but with `prStatus.IsClosed=true` instead of `CIFailing=true`. Asserts no fix-session spawn, the PR gets an explanatory close comment, and the item transitions straight to `done` with PR fields cleared.
- **Verified to fail against pre-fix code**: `git stash push -- session/backlog_lifecycle.go` then re-running the new test shows the pre-fix failure directly — `fakeSpawner.spawnCalled=true` (wasted rework triggered), `checker.closeCalled=false`, item status remained `pr_pending` instead of transitioning to `done`.
- `TestReconcilePRPending_ClosedWithoutMerge_ClearsPRFieldsAndReopens` (the pre-existing test for a genuinely-broken closed PR, using `newPRPendingTestItem` which creates no work session) — still passes unmodified: with no work session, `closeIfSupersededByMain` no-ops and the original reopen behavior is preserved, confirming the fix doesn't over-trigger.
- All 14 `TestReconcilePRPending_*` tests pass together.
- Full `go test ./session/... ./server/mcp/... ./server/services/...`, `go build ./...`, `golangci-lint run ./session/...` — clean (see commit for final confirmation).

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap — identical to BUG-032's own classification, and a direct instance of that bug's Related-section warning: "If a *different* item ever reaches review with a genuinely empty diff for some other reason, factors 2/3 remain live risks." This is that risk materializing at a sibling code path within days, not a different item — same fix, wrong/incomplete placement.

**Earliest achievable enforcement**: A test asserting the invariant directly — "every branch of `ReconcilePRPending` that can call `AutoReopenForPRFix` must first check `closeIfSupersededByMain`" — is hard to express generically (it's a call-graph shape, not a type constraint), but is now covered concretely for both branches via the two branch-specific tests. A plausible stronger enforcement worth flagging for a future refactor: since the check is now needed at *every* `AutoReopenForPRFix` call site within this function, extracting a single `reopenForFixOrCloseIfSuperseded(ctx, g, item, fixCtx)` wrapper that always runs the check internally (rather than requiring each call site to remember to call it first) would make a third future branch structurally unable to skip it — not implemented here since only two call sites exist today and a three-line duplicated check was the smaller, more obviously-correct diff; worth revisiting if a third branch is ever added.

**Recurring shape**: The same "add path wired, sibling/parallel path forgotten" shape as BUG-034 (watch-list `AddRepo`/`RemoveRepo` symmetry) and BUG-027 (one teardown path wired, siblings forgotten) — but this time within a *single function*, between two branches of the same conditional that clearly should share an invariant. Reinforces the pattern already flagged after BUG-034/035/027 this session: when a fix closes a gap at one call site, actively check for structurally parallel call sites in the same function/file before considering the fix complete, not just the one site the live incident happened to surface.

## Related

- BUG-032 (`docs/bugs/fixed/BUG-032-review-cycles-against-already-merged-empty-diff.md`) — the original fix this bug completes; same item, same underlying `closeIfSupersededByMain` mechanism, missing sibling call site.
- BUG-031 (`docs/bugs/fixed/BUG-031-autonomous-driver-large-prompt-paste-not-submitted.md`) — the stall this bug's discovery investigation started from, on the same session.
- BUG-029 (`docs/bugs/fixed/BUG-029-unprocessed-review-verdict-sweep-picks-wrong-session.md`) — earlier fix against the same backlog item, same day.
