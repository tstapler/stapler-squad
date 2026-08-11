# BUG-032: Reconciliation Keeps Re-Requesting Review for an Item Whose Fix Already Landed on `main`, Producing Unreliable Verdicts Against an Empty Diff [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22 — live incident, backlog item `9264efe7-b4c2-455a-9e2a-ab0196a63ecd` ("Backlog History feature Broken"), session `stapler-squad-fix-backlog-status-audit-trail-r16`
**Fixed**: 2026-07-22 — `session/backlog_lifecycle.go`, `session/git/worktree_git.go`
**Impact**: An item whose real fix already merged into `main` (via an earlier round's PR) can still get bounced back into `in_progress`/`review` rework cycles by the reconciliation loop. Each new review session then diffs the (already-merged, effectively empty) branch against `main` and finds nothing relevant — producing a run of confident-looking but useless FAIL verdicts ("no occurrence of X in the diff") for reasons that have nothing to do with code quality, until eventually a review PASSes based only on unverifiable self-reported text (since the diff itself provides no real signal either way). Confirmed **not** the same mechanism as BUG-029 (each review session here is genuinely fresh, with its own new verdict row — not a stale/wrong-session-selection bug).

## Live Incident Reproduction (2026-07-22)

This is the same backlog item BUG-029 was fixed against earlier today (`9264efe7`, PR #173) — evidently still cycling afterward. Pulled via `BacklogService.GetBacklogItem`:

- `16:49:47Z` — review session `headless-re-review-601776ac`, verdict **FAIL**. All 9 criteria FAIL/UNVERIFIABLE; evidence: *"No occurrence of ReconcileStuckItems, recordStatusEvent, or storage_backlog.go in the diff; diff only modifies config/config.go, Makefile, CHANGELOG.md, etc."*
- `16:50:05Z` (18s later) — review session `headless-re-review-a9f81e5f`, verdict **FAIL**, near-identical evidence text to the previous one.
- `16:54:30Z` — status event: `review → pr_pending`, note *"self-heal (reconcileDriftedPRItems): recovered from drift — item has PR #173 cached but status was 'review', not pr_pending"*.
- `16:54:39Z` — review session `headless-re-review-73707f01`, verdict **FAIL**, again near-identical evidence text.
- `16:55:30Z` — status event: `pr_pending → in_progress`, note *"auto-reopened for PR fix (CI/review)"*.
- `16:55:35Z` — new work session created on the same branch (`backlog/stapler-squad-fix-backlog-status-audit-trail`).
- `17:12:14Z` — review session `75ff3a42`, verdict **PASS**. Its own summary is explicit about why: *"The visible diff contains none of the actual backlog-status-audit-trail code... it only shows LFS benchmark baseline bumps, registry JSON entries for an unrelated WatchBacklogItems streaming feature, and generated proto code for that same unrelated feature. Per the work session's own merge-mechanics note, the branch was merged with main and ended up 'tree identical to origin/main HEAD,' and the diff was truncated... All 9 criteria are therefore judged solely on the self-reported Verification Evidence... Recommend the human/CI double-check make build && make test given the diff itself provides no independent confirmation."*

The live work session (r16, per the tmux pane) independently confirmed the same finding via `gh api compare`: **"6 commits ahead but 0 files changed — nothing to merge."** It then closed PR #173 itself ("pointed at an unrelated, conflicting branch from an earlier failed attempt... superseded by #198, no new PR needed") and concluded "The backlog item's fix is live on main; no further action required." Confirmed via `gh pr view 173`: `state: CLOSED`, `closedAt: 2026-07-22T17:29:31Z`.

**Resulting inconsistency at discovery time**: the backlog item's own record still showed `status: "review"` and `prNumber: 173` — pointing at a PR the session itself had just closed as superseded, with no replacement PR recorded.

## Root Cause

Three compounding factors were identified, none of which is BUG-029's wrong-session-selection bug (every review session above is a distinct, freshly-run session with its own freshly-written verdict):

1. **No detection of "this branch is already merged / nothing to review."** The item's fix had already landed on `main` via an earlier round (referenced repeatedly as "PR #198" by the work session's own narration). Nothing in the reopen/review-request path checked "does this branch actually differ from `main`" before spawning another full rework+review cycle — `reconcileDriftedPRItems`'s self-heal (`16:54:30`) detected *a* drift (status said `review`, PR was cached) but its remediation (transition to `pr_pending`) didn't hold — the very next tick (`16:55:30`) auto-reopened it again "for PR fix (CI/review)," restarting the same cycle. **This is the factor this fix addresses** — see below.
2. Reviews against a near-empty diff produce confident-sounding but meaningless FAIL verdicts — not fixed here (a distinct, larger change to the review pipeline's outcome vocabulary; see Not Fixed below).
3. A PASS can be reached with zero actual diff-based verification — not fixed here, same reason.

## Fix Applied

`session/backlog_lifecycle.go`, `ReconcilePRPending`: before dispatching `AutoReopenForPRFix` for a CI-failing/conflicting/blocked-review PR, a new `closeIfSupersededByMain` check runs first. It looks up the item's most recent work-role session's `LastCommitSha` and checks whether that commit is already an ancestor of `main` via `git.IsCommitOnMain` — the exact same trust boundary `GetBacklogItemShipStatus` already relies on elsewhere in this codebase for "did this item's code actually ship," not a new, less-verified standard. If the commit is already on `main`, the PR is treated as superseded rather than broken: it's closed via a new `GitWorktree.ClosePR` method (with an explanatory comment), the item's cached `PrNumber`/`PrURL` are cleared, and the item transitions straight to `done` — instead of spawning yet another wasted rework+review cycle against a diff that will show nothing relevant.

If there's no work session, no recorded commit SHA, an `IsCommitOnMain` error, or the commit genuinely isn't on `main` yet, the check no-ops and `ReconcilePRPending` proceeds exactly as before (spawns the normal CI-fix session) — this only changes behavior for the specific "already shipped elsewhere" shape.

## Files Affected

- `session/backlog_lifecycle.go` — `ReconcilePRPending` (new call site), new `closeIfSupersededByMain` helper, `prPendingChecker` interface (added `ClosePR`)
- `session/git/worktree_git.go` — new `GitWorktree.ClosePR` method

## Verification

- `TestReconcilePRPending_ClosesSupersededPR_When_LastCommitAlreadyOnMain` — reproduces the live shape (a real git repo fixture whose `main` branch already contains the item's "last commit"), asserts the PR is closed, fields cleared, and the item transitions to `done` with no fix-session spawn. **Verified to fail against the pre-fix code** (`git stash` isolation test: without the fix, `spawnCalled=false→ still asserts wrong`, actually observed `closeCalled=false`, `status=pr_pending`, `PrNumber` uncleared) and pass with the fix restored.
- `TestReconcilePRPending_SpawnsFixSession_When_LastCommitNotOnMain` — the negative case: a real repo fixture with genuinely unmerged feature work confirms the new check doesn't over-trigger — the normal fix-session spawn path still runs.
- All pre-existing `TestReconcilePRPending_*` tests pass unchanged (none of them create a work session with a `LastCommitSha`, so the new check correctly no-ops for all of them).
- `go test ./session/...` — full suite green (confirmed twice: once with a `TMPDIR` override needed to work around unrelated `/tmp` disk pressure at the time, which produced one unrelated false-positive VCS-detection failure in `session/vc` traced to that override polluting Jujutsu ancestor detection — not a regression; re-confirmed clean with the default `TMPDIR` once `/tmp` had room again).
- `go build ./...`, `golangci-lint run ./session/...` — clean.

## Not Fixed (scoped out, per explicit instruction to prioritize the highest-leverage single fix)

Factors 2 and 3 above (no distinct "diff is empty/irrelevant" review outcome; a PASS reachable on self-report alone with an empty diff) were not addressed — they're a larger change to the review pipeline's outcome vocabulary, not a single well-scoped fix. The fix applied here removes the *cause* that produced an empty/irrelevant diff for review in the first place (an already-shipped, now-superseded PR), which is higher leverage: it prevents the wasteful cycle at its source rather than making the review pipeline better at coping with it. If a *different* item ever reaches review with a genuinely empty diff for some other reason, factors 2/3 remain live risks — worth their own follow-up bug doc if that's observed.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap — `ReconcilePRPending`'s CI-fix dispatch implicitly assumed "PR is CI-failing/conflicting" always means "this PR's own code needs fixing." Nothing enforced or even checked the precondition that the PR's underlying work was still relevant/unshipped before treating a bad CI signal as "spawn a fix."

**Earliest achievable enforcement**: A unit test against a real git fixture (as written) is the practical achievable level — "is this branch's last commit already on main" is an inherently runtime, repository-state question; no type or lint rule can express it. The two tests added (positive and negative case) are the correct level.

**Recurring shape**: The third bug found in this session in the "a safety/self-heal mechanism fires but its result doesn't hold, or a downstream sweep undoes it" family (alongside BUG-026's precedent and BUG-030's swallowed-rollback-failure) — `reconcileDriftedPRItems` correctly detected the drift and self-healed, but the very next `ReconcilePRPending` tick, acting on stale assumptions about what a CI-failing PR means, undid the benefit. Flagged, same as BUG-029/030/031, for the future `quality:architecture-review` pass already recommended against this general reopen/reconciliation pathway as a whole.

## Related

- BUG-029 (`docs/bugs/fixed/BUG-029-unprocessed-review-verdict-sweep-picks-wrong-session.md`) — same backlog item, different mechanism; ruled out as the cause here (every review session in this incident is fresh, not a stale/wrong-session pick).
- BUG-030 (`docs/bugs/fixed/BUG-030-autoreopen-spawn-silent-stall.md`) — same general reopen pathway (`AutoReopenAfterFailedReview`/`AutoReopenForPRFix`), different specific gap.
- BUG-033 (`docs/bugs/fixed/BUG-033-worktree-escaped-workingdir-captured-unvalidated.md`) — found investigating the shared-checkout collision this same live item's r16 session's investigation surfaced; unrelated mechanism.
