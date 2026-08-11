# BUG-044: PR Branches Drift Unboundedly From Main, Eventually Producing an Unmergeable, Unreviewable Diff That Reads as "No Related Work" [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-23)
**Discovered**: 2026-07-23/24, investigating why item `693c2700`'s review kept failing with "the diff contains no code related to the backlog item ID feature at all"
**Fixed**: 2026-07-23 — `session/git/drift.go`, `session/git/ops.go`, `session/review_gate.go`, `server/services/backlog_service_triage.go`
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

## Root Cause (confirmed)

`AutoReopenForPRFix`'s `syncPRBranchWithMain` (`server/services/backlog_service_triage.go`, called from the PR-fix reopen path) was the *only* mechanism that resynced a stuck PR branch with `main`, and it was explicitly **best-effort**:

> "This is preventive rather than reactive... Never blocks the spawn — any failure here is logged and swallowed."

On a real conflict, `git.MergeMainIntoWorktree` aborts the merge and returns a `Conflicted` result; `syncPRBranchWithMain` turned that into a text note appended to the fix session's context ("resolving these conflicts... is part of this fix") but did **not** itself land any resync — resolution was left entirely to whatever LLM session picked up the fix context next. If that session didn't fully resolve and push the merge (or the item cycled through review again before a human/agent got to it), the branch was exactly as far behind on the next cycle, plus whatever landed on `main` in the meantime. There was no cap on how far this could drift and no separate detection for "this branch is now too far behind main to be reviewable," so the drift compounded silently across every work/review cycle until the diff itself became the review failure. Confirmed unchanged as of this fix's start: `gh pr view 213` still reported `"mergeable":"CONFLICTING"`, 405 files / 78,210 insertions / 3,067 deletions — item `693c2700` remains parked exactly as originally filed (its worktree has since been reaped by the normal park/cleanup path, so no new drift accumulated in the interim).

## Live Repro Validation

Per the bug doc's own suggested first step, I attempted to manually sync `693c2700`'s real branch against current `main` and re-run review as end-to-end confirmation before generalizing the fix. This was not completed against the live item: the item has been parked (per BUG-043) since filing, and its git worktree — looked up via the workspace DB's `worktrees` table (`worktree_path` for `backlog/stapler-squad-expose-backlog-item-id`) — no longer exists on disk; parked items have their worktrees reaped by the normal cleanup path. Recreating one would require driving the live, actively-serving production service's own spawn machinery (`SpawnSessionFromItem` / "Reopen for Revision") from outside this session's sandboxed worktree — assessed as unsafe scope creep for this fix (the sandbox itself refuses cross-worktree git operations for the same reason). In place of a live manual re-sync, the regression tests below reproduce the *exact* failure shape end-to-end against real git repos (a real conflicting merge, the real `EnsureBranchSyncedWithMain`/`TriggerReReview` code paths, a fake headless pool primed with 693c2700's actual misleading verdict text) — equally strong evidence that the root-cause understanding is correct and that the fix addresses it, without touching the live shared system.

## Fix Applied

Implemented as an `sdd:fix-bug`-shaped change — the root cause was narrow enough (one missing precondition, reusable in two call sites) not to require scaling up to `sdd:quick`/`sdd:full`; see **Scope: Implemented vs. Deferred** below for what was deliberately left for a follow-up.

**1. New git-layer drift primitives (`session/git/ops.go`, `session/git/drift.go`):**

- `BehindOriginMain(worktreePath, mainBranch string) (int, error)` — always fetches `origin/mainBranch` first, then counts commits behind it via the existing `countCommitsNotAncestorOf` walker. Deliberately distinct from the pre-existing `BranchAheadBehind` (which reads a repo's *local* `mainBranch` ref and does no fetch of its own — fine for a UI badge computed against a repo some other process keeps fresh, but would have been silently stale here) — this always self-fetches, so the drift count can never be stale.
- `EnsureBranchSyncedWithMain(worktreePath, branchName, mainBranch string, driftThreshold int) (ok bool, blockedSummary string)` — the actual precondition. Checks `BehindOriginMain`; under `driftThreshold` (default 50 commits — comfortably past normal multi-day activity, well short of the 289-commit scale that broke `693c2700`) it's a no-op. Past threshold, it reuses the same `git.MergeMainIntoWorktree` sync `syncPRBranchWithMain` already used reactively, but runs it *proactively*: a clean merge is synced and pushed transparently (`ok=true`, review proceeds on the now-current branch); a real conflict blocks (`ok=false`) with an explicit message naming the conflicted files and the branch, `git`-command-ready for a human, and phrased for a follow-up fix session's prompt context. Fails open (`ok=true`) on any detection/fetch error, matching every other best-effort git check in this codebase.

**2. Wired as a precondition of review at both entry points into the review gate:**

- `session/review_gate.go` (`ReviewGateRunner.Run`) — the normal work-session-exit→review transition. Checked immediately after resolving the session's worktree, *before* any diff is computed, so a clean sync never even reaches the reviewer and a conflict blocks before a misleading diff can be built.
- `server/services/backlog_service_triage.go` (`TriggerReReview`) — the abandoned-review re-spawn path (`AutoRespawnReview` → `TriggerReReview`), which is `693c2700`'s *actual* live entry point (its reviews came from repeated `abandoned_review` respawns, not a fresh work-session exit — see BUG-043). Checked before `getWorkSessionDiff` is called, for the same reason.

Both block paths reuse the existing terminal-verdict/notify/auto-reopen machinery already established for the diff-computation-failure and security-check blocks in this codebase (`recordTerminalReviewVerdict` / `RecordDegradedReviewVerdict`, an operator notification, and — in `ReviewGateRunner.Run` — feeding `AutoReopenAfterFailedReview` so a fix session with the conflict details in context gets spawned automatically) rather than inventing new machinery. Critically, the *recorded verdict text* now explicitly names the real cause ("this branch is N commits behind main and merging the latest main in produced conflicts in: ...") instead of the misleading, content-focused "no code related to the feature" text `693c2700` actually received — so even without a dedicated `StuckReason` (see Deferred below), the parked message a human reads is now accurate.

## Scope: Implemented vs. Deferred

The bug doc's suggested fix direction had three parts; here's what shipped now vs. what's follow-up work, and why:

| # | Suggestion | Status | Why |
|---|---|---|---|
| 1 | Make main-sync a precondition of review | ✅ Implemented | Core fix — see above. Covers both review entry points (`ReviewGateRunner.Run` and `TriggerReReview`). |
| 2 | Dedicated `StuckReason`/detectable drift condition, surfaced on `/unfinished` | ⏸️ Deferred | A new `StuckReason` here means a new proto enum value + `make proto-gen` + a `toProtoStuckReason`/`fromProtoStuckReason` case (`server/services/backlog_service_stuck.go`) + a new entry in **every** `Record<StuckReason, T>` map in `web-app/src/components/backlog-stuck/stuckReason.ts` (label, icon, CSS class) — a real, TypeScript-compiler-enforced surface, but a materially larger blast radius than this fix-bug pass, and not required to fix the misdiagnosis itself: the blocked verdict's summary text (see above) already carries the accurate, branch-drift-specific explanation into the exact place `693c2700`'s misleading text was surfaced (the parked notification / review history). Reusing `StuckReasonBouncing` for the parked state (as `notifyRepeatedFailure` already does for a different variant of "non-converging cycle") is consistent with existing convention. A dedicated reason remains valuable for `/unfinished` filtering/analytics specifically, but is a UI/observability enhancement on top of an already-fixed root cause, not a blocking part of the fix. |
| 3 | Wall-clock-aware rework cap, distinguishing content-bouncing from never-synced | ⏸️ Deferred | With #1 fixed, a branch can no longer drift far enough for this distinction to matter in practice — the sync now happens *before* every review, not after repeated failures, so the two failure modes this suggestion was trying to separate (genuine content bouncing vs. accumulating drift) can no longer be conflated the way they were in `693c2700`, because drift never has the chance to accumulate across cycles anymore. Revisit only if a future audit finds items still accumulating multi-day drift despite #1 (e.g. a `driftThreshold`-adjacent edge case) — no evidence of that today. |

## Files Affected

- `session/git/ops.go` — new `BehindOriginMain`
- `session/git/drift.go` (new) — `EnsureBranchSyncedWithMain`, `DefaultBranchDriftThreshold`
- `session/git/drift_test.go` (new) — unit tests for both, real-repo-backed (clone + origin remote, matching this package's existing test conventions)
- `session/review_gate.go` — `ReviewGateRunner.Run` calls `EnsureBranchSyncedWithMain` before diff computation; blocks with a FAIL verdict + notification + `AutoReopenAfterFailedReview` on conflict
- `session/review_gate_test.go` — two new regression tests (conflict-blocks, clean-sync-proceeds)
- `server/services/backlog_service_triage.go` — `TriggerReReview` calls the same precondition before `getWorkSessionDiff`; blocks with a degraded (`UNVERIFIABLE`) verdict + notification on conflict
- `server/services/backlog_service_triage_test.go` — new regression test reproducing `693c2700`'s actual live entry point (`AutoRespawnReview` → `TriggerReReview`)

## Verification

- **`TestReviewGateRunner_BranchDrift_BlocksReviewWithConflictDetails_When_AutoSyncConflicts`** (`session/review_gate_test.go`) — real git repos (origin + clone-with-remote), a branch drifted 55+ commits behind main with a genuine conflicting edit on both sides. Asserts: the session creator (reviewer) is never consulted, a FAIL verdict naming the conflicted file and "BUG-044" is recorded, the operator notification fires, `AutoReopenAfterFailedReview` is invoked, and the worktree is left clean (no half-merged state).
- **`TestReviewGateRunner_BranchDrift_SyncsAutomaticallyAndProceeds_When_NoConflict`** (`session/review_gate_test.go`) — same drift depth, no conflict. Asserts the review proceeds normally (session creator called once) and the sync's merge commit is actually pushed to origin, not left local-only.
- **`TestTriggerReReview_should_BlockOnBranchDriftInsteadOfMisleadingFailVerdict`** (`server/services/backlog_service_triage_test.go`) — reproduces `693c2700`'s actual live shape end-to-end: item in `review` status, branch 55+ commits behind main with a real conflict, a fake headless pool primed with the *exact* misleading verdict text `693c2700` received ("the diff contains no code related to the feature at all"). Asserts the pool is never called (proving the block happens before a drift-inflated diff could ever reach a reviewer) and an explicit `UNVERIFIABLE` blocked verdict is recorded instead.
- **All three verified to fail against pre-fix code**: stashed each production file in turn (`session/review_gate.go`; `server/services/backlog_service_triage.go` with `session/git/drift.go` temporarily removed for the `session/git`-only revert), reran the corresponding new tests — all failed exactly as expected (review proceeded / headless pool was called with the misleading response / no sync landed on origin). Restored the fixes; all pass again.
- Existing coverage unaffected: full `TestReviewGateRunner_*` suite (`session/review_gate_test.go`) and full `TestTriggerReReview_*` suite (`server/services/backlog_service_triage_test.go` + `backlog_service_test.go`) still pass — the new precondition only changes behavior when a worktree exists and drift is genuinely past threshold, and none of the pre-existing fixtures in either file register an `origin` remote (they use standalone repos), so `BehindOriginMain`'s fetch fails open and the drift check is a silent no-op for all of them.
- `session/git` package unit tests (`session/git/drift_test.go`) — 8 new tests covering `BehindOriginMain` (zero drift, freshly-pushed drift with no prior fetch, unreachable-origin error) and `EnsureBranchSyncedWithMain` (under-threshold no-op, clean sync + push, conflict block, empty-worktree-path fail-open, broken-detector fail-open).
- `make build` — full proto/ent/web-UI/Go build, clean.
- `make test` (`go test -short ./...`) — clean except two pre-existing, environment-dependent flaky tests unrelated to this change, both confirmed to pass in isolation on rerun: `TestStreamTerminal_SendsRawOutput` (`server/services`, real tmux-backed PTY streaming) and `TestEnsureServerRunning_NoOp` (`session/tmux`, real tmux server lifecycle) — neither touches backlog, review, or git-drift code.
- `golangci-lint run ./session/git/... ./session/... ./server/services/...` — 0 issues.
- `gofmt -l` on all changed/new files — no output (clean).

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Missing Precondition — a resync mechanism existed (`syncPRBranchWithMain`) but was wired only as a best-effort side effect of one specific reactive path (PR-fix retry), not as a gate on the thing that actually needed protecting (review). The same shape as BUG-043's "gate chaining without gate awareness," but here the gap was an *absent* gate rather than two gates with mismatched clocks.

**Earliest achievable enforcement**: The regression tests are close to the practical ceiling here — branch drift is fundamentally a runtime git-state condition (how far two refs have diverged), not something a type system or lint rule can express statically. The one systemic improvement beyond the regression tests: `EnsureBranchSyncedWithMain` is now a small, reusable, well-documented primitive in `session/git` (mirroring `RemediationBlocked`'s role from BUG-043) that any *future* review-adjacent entry point can call rather than re-discovering this failure mode from scratch — the two current call sites (`ReviewGateRunner.Run`, `TriggerReReview`) are exactly the two places today's codebase spawns a review, so this is now a closed set, not an open-ended "remember to add this everywhere" convention.

**Recurring shape**: Fourth instance in this codebase's bug history of "a real mechanism exists and works correctly, but is scoped/triggered too narrowly to prevent the actual failure" (alongside BUG-040's PR-reference dead end, BUG-041's nudge-retry-never-backs-off, and BUG-043's gate-chaining) — worth naming: **"reactive machinery mistaken for a guarantee."** `syncPRBranchWithMain`'s own doc comment already said "preventive rather than reactive," but its only call site was itself reactive (after a PR-fix failure) — the comment described an aspiration the wiring didn't deliver. When reviewing new best-effort/self-healing mechanisms, check not just "does this work when it runs" but "does anything actually guarantee it runs early enough to prevent the failure it's meant to prevent."

## Related

- Complementary to BUG-043 (`docs/bugs/fixed/BUG-043-chronic-abandoned-review-respawn-failures.md`): BUG-043's `bouncing` gate fix is what let `693c2700` park cleanly instead of looping forever burning remediation attempts; this fix addresses the upstream cause of *why* its reviews kept failing in the first place. Both fixes are needed together — BUG-043 without this fix means clean, prompt parking on a misdiagnosed problem; this fix without BUG-043 means the drift-blocked verdict still needs a working circuit breaker to avoid looping.
- `693c2700` itself remains parked and was not automatically un-parked by this fix (its worktree is gone; per BUG-043's own finding, it still needs a human "Reopen for Revision" to respawn a fresh work session) — but the *next* time its branch (or any item's branch) drifts past 50 commits behind main, the review gate will sync it transparently before ever reaching a reviewer, or block with an accurate, actionable message if the sync itself conflicts.
