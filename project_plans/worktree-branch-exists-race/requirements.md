# Requirements: worktree-branch-exists-race

## Source

Backlog item `200b6896-3d63-421b-9e96-41cf06d289fa`: "CreateBacklogWorktree fails with
'branch already exists' — swallowed ref-lookup errors in GitWorktree.Setup". Entry point
`sdd:fix-bug` (localized root cause, no architectural change). Non-interactive triage —
this doc is derived directly from the item's description, not an interview.

## Problem Statement

`GitWorktree.Setup()` and `GitWorktree.setupNewWorktree()` in
[`session/git/worktree_ops.go`](https://github.com/tstapler/stapler-squad/blob/04d485a4401539430f54ebc4d762808279266c51/session/git/worktree_ops.go)
each call `repo.Reference(branchRef, false)` and treat **any** non-nil error as "branch does
not exist" (`branchExists` only set `true` on `err == nil`; verified by reading both call
sites, lines ~40-44 and ~234-239 in the current worktree). This is a confirmed code defect:
any such error — not just `plumbing.ErrReferenceNotFound` — is misclassified as "no such
branch," routing into `setupNewWorktree`'s branch-creating path (`git worktree add -b
<branch> ...`) even when the branch already exists on disk. Git then fails hard with `fatal:
a branch named '...' already exists`, which `resolveSessionPath` (per the documented
`BUG-057` decision at `server/services/backlog_service_triage.go:1426`) surfaces as a hard,
non-recoverable spawn error rather than degrading gracefully.

**Caveat (added post-adversarial-review, 2026-08-11): the "transient ref-read failure"
mechanism named below (packed-refs lock contention, I/O hiccup, racy read against a
concurrent `git worktree add`/`git branch`) as the trigger for the misclassification is
UNCONFIRMED, not established fact.** Adversarial review of the implementation plan found
that go-git itself already converts common transient OS-level read/stat errors on a loose
ref file into `plumbing.ErrReferenceNotFound` before they ever reach this repo's code
(verified via a `chmod 000` repro) — so that specific transient-I/O story does not actually
reach the misclassified-error code path. The only repro found that produces a genuine
non-`ErrReferenceNotFound` error is persistent `packed-refs` corruption, not a transient
blip. A more plausible alternate/additional cause is a TOCTOU race between
concurrent/back-to-back-retried `Setup()`/`CreateBacklogWorktree` calls for the same
deterministic branch name, where no misclassification occurs at all — see
`implementation/plan.md`'s Unresolved Questions for the full analysis. This session has no
access to the original failure's log evidence, so neither mechanism is confirmed against
the actual incident.

Because `backlogWorkBranchSlug()` derives a **deterministic** branch name from
`(repoPath, title)`, every automatic respawn for the same backlog item recomputes the
identical branch name. If any earlier attempt got far enough to create the branch — and
`GitWorktree.Cleanup()` deliberately never deletes the branch (to avoid losing unpushed
commits, see the doc comment at `worktree_ops.go:269-276`) — every subsequent retry races
the same swallowed-error path against a branch that now genuinely exists. This produces a
permanently stuck item (observed as `Not converging 20h` on item
`ccbfe7a6-3102-4110-a11f-6e34ece51798`), since nothing about the failure mode changes
between retries.

## Root Cause (confirmed by reading the code)

Two call sites in `session/git/worktree_ops.go` share the same bug — collapsing
`err != nil` from `repo.Reference()` to "branch does not exist" instead of checking
specifically for `plumbing.ErrReferenceNotFound`:

1. `Setup()`, the parallel branch-check goroutine (~line 40):
   ```go
   if _, err := repo.Reference(branchRef, false); err == nil {
       branchExists = true
   }
   errChan <- nil   // any non-nil, non-NotFound error is silently discarded here too
   ```
2. `setupNewWorktree()`, the pre-creation re-check (~line 235):
   ```go
   if _, err := repo.Reference(branchRef, false); err == nil {
       log.Info(...)
       return g.setupFromExistingBranch()
   }
   ```

Both need to distinguish `plumbing.ErrReferenceNotFound` (the only case that legitimately
means "does not exist") from any other error (which should propagate/log loudly rather than
be folded into "does not exist").

## In Scope

- Fix both `repo.Reference()` call sites in `session/git/worktree_ops.go` to distinguish
  `plumbing.ErrReferenceNotFound` from other errors, per the fix sketch in the backlog item.
- Ensure a genuine non-NotFound error surfaces (propagates or is logged and retried) instead
  of silently degrading to "branch does not exist."
- Add/adjust a Go test that exercises the distinction (e.g. a fake/wrapped reference error
  that isn't `ErrReferenceNotFound` should not be treated as "branch missing").
- Preserve existing behavior for the two legitimate cases: branch truly absent (create it)
  and branch truly present (reuse via `setupFromExistingBranch`).

## Out of Scope

- Any change to `resolveSessionPath`'s BUG-057 hard-fail-on-git-worktree-error policy —
  that policy is intentional per its own doc comment and not part of this bug.
- Any change to `GitWorktree.Cleanup()`'s branch-retention behavior — also intentional
  (avoids losing unpushed commits).
- Recovery/self-heal logic for backlog items already stuck in this state before the fix
  ships (e.g. a one-off admin action to unstick `ccbfe7a6-...`) — not requested by the item.
- Broader retry/backoff strategy redesign for backlog spawn attempts.

## Acceptance Criteria (initial)

1. `repo.Reference()` errors in both `Setup()` and `setupNewWorktree()` are checked against
   `plumbing.ErrReferenceNotFound` specifically, not treated as equivalent to "no such
   branch" for any other error type.
2. A non-`ErrReferenceNotFound` error from either call site is surfaced (returned as an
   error and/or logged at a visible level) rather than silently causing a fall-through to
   branch creation.
3. Existing "branch already exists → reuse" and "branch absent → create" behaviors are
   unchanged and covered by tests.
4. A new or updated unit test in `session/git/` demonstrates that a non-NotFound reference
   error is no longer misclassified as "branch does not exist" (i.e. would have caught this
   bug).
5. `make quick-check` (build + test + lint) passes.
