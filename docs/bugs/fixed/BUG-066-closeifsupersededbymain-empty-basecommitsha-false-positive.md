# BUG-066: `closeIfSupersededByMain` Closes Real, Unmerged PRs When `BaseCommitSha` Was Never Recorded [SEVERITY: High]

**Status**: ✅ Fixed
**Discovered**: 2026-08-06, live in this repo's own deployed instance — backlog item `a8a2505e` ("feat: user-extensible agent detection plugins") was marked `done` with PR #307 closed as "superseded," but a background audit confirmed `session/detection/plugins.go` does not exist anywhere in the codebase — the described feature was never actually merged.

## Live Evidence

`backlog_status_events` for the item:
```
review -> pr_pending  (system, note: "...All 8 ACs independently reviewed and PASSed. Draft PR opened for CI + human merge.")
pr_pending -> done    (system, note: "self-heal: PR #307 closed as superseded — commit 32f504c803addd4306acb34913e0e3de1f3ae2c4 already on main")
```

Independently verified: `git merge-base --is-ancestor 32f504c803addd4306acb34913e0e3de1f3ae2c4 origin/main` returns false — that commit is **not** an ancestor of `origin/main`, despite `closeIfSupersededByMain`'s own log line claiming exactly that.

## Root Cause

`closeIfSupersededByMain` (`session/backlog_lifecycle.go`) already has a guard against its own prior incident (BUG-047): before checking `git.IsCommitOnMain` on a session's `LastCommitSha`, it refuses to act if `lastCommitSha == lastWork.BaseCommitSha` — because a session's own pre-work base commit is, by construction, always already on main, and treating it as "the session's real work" would false-positive on every session that hasn't committed anything yet.

That guard only fires when `BaseCommitSha` is a real, non-empty value. `GetBaseCommitSHAsForSessions` (`session/storage_backlog.go`) already documents that `BaseCommitSha` reads `""` both for legacy `ItemSession` rows written before `BaseCommitSha`/`LastCommitSha` were split into separate fields, and for any session that spawned and was retried/abandoned before its base commit was ever seeded. `closeIfSupersededByMain`'s guard was never given the same fallback: an empty `BaseCommitSha` can never equal a real SHA, so the equality check silently no-ops, and the session's own untouched spawn-time base slides straight through `IsCommitOnMain` — which correctly reports "yes, on main" (a base commit always is) — as if it were the session's own later, real work.

This is the **third** distinct gap in this function's "is this actually the session's own authored work" trust boundary — after BUG-032 (original motivation) and BUG-047 (the `BaseCommitSha` equality guard itself, for the common case where the field is populated).

## Fix

Added a fallback check immediately after the existing `BaseCommitSha` equality guard: when `BaseCommitSha` is empty, fall back to `CommitCountSinceSpawn == 0`. That field is written alongside `LastCommitSha` on every reconciliation tick (`refreshWorkSessionGitActivity`) and is `0` for any session that has made no commits since spawn, regardless of what `lastCommitSha` resolves to — a resolved commit from a session with zero commits since spawn is, by construction, that session's own pre-work snapshot, the same conclusion the `BaseCommitSha` check reaches when it has the data to reach it at all.

```go
if lastWork.BaseCommitSha == "" && lastWork.CommitCountSinceSpawn == 0 {
    return false
}
```

## Regression Tests

`session/backlog_lifecycle_superseded_test.go` (pre-existing file for BUG-047's regression coverage, extended with one new test):

- `TestReconcilePRPending_should_NotCloseUnmergedPRAsSuperseded_When_BaseCommitShaWasNeverRecorded` — reproduces the exact live incident shape: a work session whose worktree never advanced past its own base commit, `BaseCommitSha` never recorded (empty), `LastCommitSha` seeded only with the base value. Asserts the PR is NOT closed and the item stays `pr_pending`.

Existing tests in the same file continue to pass, including the positive control (`TestReconcilePRPending_should_StillCloseSupersededPR_When_SessionsRealTipIsOnMain`) confirming the fix doesn't disable genuinely-superseded detection, and the sibling BUG-047 regression tests for the already-populated-`BaseCommitSha` case.

`go test ./session/...` (all green), `go build ./session/... ./server/...` (clean), `go vet ./session/...` (clean), `golangci-lint run ./session/...` (0 issues).

## Phase D — Classification

**Classification**: Type Safety Gap / incomplete precedent. `BaseCommitSha` is a plain string with no invariant enforcing "populated whenever a comparison against it matters" — the BUG-047 fix correctly closed the gap for the common case (field populated) but didn't account for the field's own documented empty-state (already known and worked around elsewhere in `GetBaseCommitSHAsForSessions`) reaching this specific call site.

**Earliest enforcement point**: The regression test is the earliest achievable level — this is a data-shape assumption (a string field that's sometimes legitimately empty) that no type system change can make illegal without a larger refactor (e.g. an `Option<CommitSHA>` wrapper thread through every consumer), which would be disproportionate for a single call site.

**Recurring shape**: Third instance of "is this genuinely the session's own authored work, or its own untouched starting point" in the same function (`closeIfSupersededByMain`). Confirms the reflect-and-fix framing from this function's own doc comment history — any future change to how base/tip commits are tracked should re-check this specific function's assumptions, since it has now needed three independent hardenings against the same underlying question.

## Related

- `docs/bugs/fixed/BUG-047-...md` (if present) — the original `BaseCommitSha` equality guard this extends.
- `session/storage_backlog.go`'s `GetBaseCommitSHAsForSessions` — already documents and works around the same empty-`BaseCommitSha` legacy-row shape this fix accounts for at a second call site.
- Backlog item `a8a2505e` was requeued (`done` → `idea`, fresh triage triggered) as part of the same session that found this bug — not fixed by this PR, tracked separately.
