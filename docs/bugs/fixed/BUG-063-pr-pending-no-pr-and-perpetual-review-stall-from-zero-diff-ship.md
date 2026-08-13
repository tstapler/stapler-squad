# BUG-063: Zero-Diff PASS-Verdict Items Land in `pr_pending`/`pr_number=0` or Stall in `review` Forever [SEVERITY: High]

**Status**: ✅ FIXED (2026-08-06)
**Discovered**: 2026-08-06, live incident — backlog item `2668d886-197c-4b26-9c28-ca6731f5070a` in this repo's own running instance. The work session earned a PASS verdict, correctly determined the fix was already shipped by an earlier, unrelated PR (#354), verified every acceptance criterion directly against `main`, and explicitly declined to open a no-op PR ("this branch has no commits and no diff versus main... Opening a no-op PR would just be noise"). The item nonetheless ended up stuck.
**Fixed**: 2026-08-06 — `session/backlog_lifecycle.go`, `session/git/worktree_git.go`

## Live Evidence

`backlog_status_events` for the item:
```
in_progress -> review   (agent, request_review, PASS verdict recorded)
review -> pr_pending    (system, note: "agent-driven ship via one-shot /backlog/ship")
```

`backlog_stuck_states` in the same window:
```
push_failed       first_detected=23:57:13  resolved=00:00:05  context="PR creation failed: gh pr create failed: pull request create failed: GraphQL: No commits between main and backlog/stapler-squad-verify-backlog-emptystate-fix-closed (createPullRequest)\n (exit status 1)"
pr_pending_no_pr  first_detected=00:00:05  resolved=<still open>  context="item is pr_pending but has no PR reference (pr_number=0)"
```

## Root Cause (confirmed by reading the code, not just the DB trace)

The transition note `"agent-driven ship via one-shot /backlog/ship"` is emitted from exactly one call site: `shipViaAgentOrFallback`'s success branch (`session/backlog_lifecycle.go`, previously ~line 3511) — the mechanical `pushAndCreatePR` fallback never produces that note. This pins the actual transition to `pr_pending` on `shipViaAgentOrFallback`, not on `pushAndCreatePR`'s `CreatePR` failure (which only ever leaves an item in `review`, never transitions it — see below).

**Defect #1 (the one that produced this exact incident) — `shipViaAgentOrFallback`'s unconditional transition.** After `RunOneShotForSession` (the agent's own `/backlog/ship` one-shot run) returns a non-empty `prURL`, the old code:

```go
prNumber := 0
if ref, parseErr := ParseGitHubURL(prURL); parseErr == nil {
    prNumber = ref.PRNumber
}
if prNumber > 0 {
    // ...UpdateBacklogItem to persist PrURL/PrNumber...
    // (a persist failure here was only logged, never checked)
}
if transErr := l.resolveToPRPending(...); transErr != nil { ... }  // ALWAYS runs
```

`resolveToPRPending` (the actual status transition) ran **unconditionally**, regardless of whether `prNumber` was ever successfully parsed (`> 0`) or the field write into storage actually succeeded. The most likely trigger here: the agent's final `/backlog/ship` output explained the fix was already shipped by PR #354 — plausibly including a reference to that PR's URL in its last lines — and `extractPRURL`'s heuristic (`server/services/session_service.go`, scans the last 10 non-empty output lines for any `github.com/.../pull/` substring) picked that unrelated mention up as if it were a PR the one-shot had just created. Whether the resulting `prURL` failed to parse, or parsed to a real-but-irrelevant PR number that then failed to persist against this item, either path reaches the same unconditional `resolveToPRPending` call — landing the item in `pr_pending` with `pr_number` left at its prior value (0).

This is the **identical BUG-040 shape** (a persist failure treated as a soft warning instead of a blocking condition, followed by an unconditional transition) that `pushAndCreatePR` was already fixed for in BUG-040 — but that fix was never propagated to this sibling call site. `docs/bugs/open/BUG-058-pushAndCreatePR-non-atomic-pr-field-write-and-transition.md` (filed 2026-08-03) had already flagged these two call sites as sharing a related non-atomic-write shape, but assessed the risk here as low; this incident shows the risk was real.

**Defect #2 (a related but distinct gap) — `pushAndCreatePR`'s `CreatePR` failure has no "genuinely nothing to ship" branch.** When a worktree exists but its branch has zero commits ahead of `main` (the exact scenario in this incident, once the mechanical fallback path is reached), `gh pr create` fails with `"No commits between X and Y"`. The existing code treated this identically to a retryable failure (auth, network, non-fast-forward push): `stayInReviewAndNotify("PR creation failed", prErr)`, which leaves the item in `review` indefinitely with a `push_failed` stuck row that can never resolve — no future retry of an unchanged zero-diff branch will ever make `gh pr create` succeed. A PASS verdict already confirmed the work; the correct terminal state was always `done` (mirroring `pushAndCreatePR`'s own pre-existing `fallbackToDone("no worktree")` precedent for the sibling "definitively nothing to ship" case), not an unresolvable stall.

## Fix Applied

**Defect #1** (`shipViaAgentOrFallback`): the unconditional `resolveToPRPending` call is now gated. If `prNumber <= 0` (unparseable/irrelevant `prURL`), or the `UpdateBacklogItem` persist fails, the item now stays in `review` via a shared `stayInReviewAndNotify` helper (extracted from `pushAndCreatePR`'s previously-private closure) and is notified — never silently transitioned to `pr_pending`. Falling back to `pushAndCreatePR` was deliberately *not* chosen here: we cannot rule out that the agent's one-shot run genuinely created a real PR we simply failed to parse/persist a reference to, so retrying PR creation risks a duplicate. Staying in review (retriable via `TriggerReReview`, same as every other `stayInReviewAndNotify` case) is the safe choice.

**Defect #2** (`pushAndCreatePR`): added a pre-flight check, `GitWorktree.HasCommitsAheadOfMain(mainBranch)` (`session/git/worktree_git.go`, backed by the existing `BranchAheadBehind` go-git helper — no new subshells, per `.claude/rules/prefer-go-git-over-subshells.md`), run immediately before attempting `CreatePR` for a brand-new PR. When the branch exists but has zero commits ahead of `main`, the item now routes through the existing `fallbackToDone` path — the same terminal state reached by the pre-existing "no worktree at all" case — instead of falling through to a doomed `CreatePR` call. A check failure (e.g. the repo can't be opened) is treated as inconclusive and never blocks a real PR attempt.

`HasCommitsAheadOfMain` was added to the `prCreator` interface (consumer-defined, per `.claude/rules/interface-pollution-checklist.md`) so it's testable via the existing `fakePRCreator` double without needing a real on-disk git repo in unit tests.

## Files Affected

- `session/git/worktree_git.go` — new `GitWorktree.HasCommitsAheadOfMain(mainBranch string) (bool, error)`
- `session/backlog_lifecycle.go` — `prCreator` interface gains `HasCommitsAheadOfMain`; `pushAndCreatePR` gets the zero-diff pre-flight check; `shipViaAgentOrFallback`'s PR-number gate; `stayInReviewAndNotify` extracted from a `pushAndCreatePR`-local closure into a shared listener method used by both functions
- `session/backlog_lifecycle_test.go` — `fakePRCreator` gains `HasCommitsAheadOfMain` support; new regression tests (below)

## Verification

- `TestPushAndCreatePR_ZeroDiffBranch_FallsBackToDone` — new BUG-063 regression for defect #2: a branch with zero commits ahead of main falls back to `done`, never attempts `CreatePR`.
- `TestPushAndCreatePR_AheadOfMainCheckErrors_StillAttemptsPRCreation` — new: an inconclusive `HasCommitsAheadOfMain` result never blocks a real PR creation attempt.
- `TestShipViaAgentOrFallback_OneShotReturnsUnparseablePRURL_StaysInReview_DoesNotTransitionToPRPending` — new BUG-063 regression for defect #1, the direct repro of the live incident: an unparseable/irrelevant one-shot `prURL` must leave the item in `review` with `pr_number` still 0, never transition to `pr_pending`, and never call `CreatePR` (no duplicate-PR risk).
- All pre-existing `TestPushAndCreatePR_*` and `TestShipViaAgentOrFallback_*` tests still pass unchanged (the new `fakePRCreator` field defaults to preserving old behavior).
- `go test ./session ./server/services` — full packages, both green (63.6s / 84.2s).
- `go build ./...` and `make build` — clean.
- `make lint` — 0 issues.

## Live Data — Manual Operator Action Required

The specific incident item, `2668d886-197c-4b26-9c28-ca6731f5070a`, is live in this repo's own running instance, permanently wedged in `pr_pending` with `pr_number=0`. This fix was developed and verified in an isolated worktree and has **not** been deployed to the running service, and the live database/API were **not** touched as part of this fix (out of scope — the operator should resolve the one already-affected item manually, e.g. via `UpdateBacklogItem`/`ArchiveBacklogItem` through the running service once this fix is deployed, not via a direct DB edit or from within this worktree).

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Semantic/Intent gap (defect #2 — "PR creation failed" was never split into "genuinely nothing to ship" vs. "retryable failure") paired with an Integration Gap (defect #1 — a field-persist failure treated as a soft warning instead of a blocking precondition before a status transition, at a call site that duplicates another call site's already-fixed logic).

**Earliest achievable enforcement**: For defect #2, the regression test is close to the practical ceiling — the underlying signal (`gh pr create`'s specific stderr) is only observable at the process boundary; a pre-flight go-git check (added here) is the earliest *code-level* enforcement available, but the invariant itself ("no PR-worthy diff exists") is inherently a runtime, not compile-time, fact. For defect #1, the fix *is* the earliest achievable enforcement: gating the transition on the persist's own success is exactly the "prove the precondition before the destructive/visible write" pattern — no type system or lint rule can distinguish a correctly-ordered two-call sequence from an incorrectly-ordered one without encoding this specific invariant.

**Recurring shape confirmed**: this is a second instance of BUG-040's exact shape — "a write into a load-bearing field is best-effort/unchecked, and a status transition proceeds unconditionally regardless of whether it landed" — recurring at a sibling call site (`shipViaAgentOrFallback`) that duplicates `pushAndCreatePR`'s logic but was never given the same fix. `BUG-058` (open, filed 2026-08-03) had already named these two call sites as sharing a related non-atomic-write shape and assessed the specific lost-update risk as low; this incident confirms the *broader* family of risks at these two call sites is not low, even though BUG-058's own narrower concern (concurrent racers) remains correctly out of scope for this fix. Anyone auditing "unconditional transition after a soft-failed write" in this file going forward should check both `pushAndCreatePR` and `shipViaAgentOrFallback` together — the newly-shared `stayInReviewAndNotify` helper is the intended single point of enforcement for both, going forward, rather than a per-call-site convention that can silently diverge again.

## Related

- `docs/bugs/fixed/BUG-040-pr-pending-item-loses-pr-reference-dead-end.md` — the original instance of this exact shape, in `pushAndCreatePR` only.
- `docs/bugs/open/BUG-058-pushAndCreatePR-non-atomic-pr-field-write-and-transition.md` — flagged these two call sites' shared shape three days before this incident; not fixed by this change (its own narrower concurrent-racer concern is out of scope here — see that doc's own "Suggested Fix" section for the separate atomic-primitive follow-up it recommends).
