# Implementation Plan: worktree-branch-exists-race

**Feature**: Fix misclassified `repo.Reference()` errors in `GitWorktree.Setup()`/`setupNewWorktree()` — today, any non-nil error from that call (not just `plumbing.ErrReferenceNotFound`) is treated as "branch does not exist" and silently falls through to `git worktree add -b`, which can then fail hard with "branch already exists." This is a **confirmed code defect** (any `repo.Reference()` error is misclassified as absence) and the fix is defensive hardening against ref-read corruption regardless of cause. Whether this specific defect is *the* cause of the originally observed "branch already exists" failure on backlog item `200b6896-3d63-421b-9e96-41cf06d289fa` — versus a TOCTOU race between concurrent/retried `Setup()` calls on the same deterministic branch name (see Unresolved Questions) — is **UNCONFIRMED**: this session has no access to the original failure's log evidence, and go-git's own error-swallowing (transient OS-level read/stat errors on a loose ref file already collapse to `plumbing.ErrReferenceNotFound` before reaching this repo's code — verified via a `chmod 000` repro during adversarial review) makes the "transient ref-read failure gets misclassified" story implausible for the most common transient-I/O case. The fix is kept in scope either way; see Unresolved Questions for the confirmation gap and the TOCTOU follow-up.
**Date**: 2026-08-11
**Status**: Ready for implementation
**ADRs**: None — `errors.Is` vs `==` and extract-vs-inline are both uncontroversial default choices for a 1-file, 2-call-site bug fix; recorded in the Pattern Decisions table below instead.

---

## Domain Glossary
N/A — no new domain concepts introduced. The one new symbol added (`branchRefExists`, a private helper method) is an extraction of existing logic, not a new domain concept.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Ref-not-found check | `errors.Is(err, plumbing.ErrReferenceNotFound)` | research/stack.md #2, `session/unfinished/gogit_vcs_reader.go:751` precedent | Direct `err != plumbing.ErrReferenceNotFound` (`worktree_branch.go:15,24`'s existing style) | `errors.Is` is unwrap-safe if go-git or this repo's own code ever wraps the sentinel later; behaviorally identical today (research/stack.md #1 confirms the sentinel is never wrapped) but costs nothing and matches the repo's own newer precedent over its older one. |
| Classification logic reuse | Extract shared `(g *GitWorktree) branchRefExists(repo *git.Repository, branchRef plumbing.ReferenceName) (bool, error)` helper, called from both `Setup()`'s goroutine and `setupNewWorktree()` | research/pitfalls.md #5 (names this as one of two viable test granularities); interface-pollution-checklist smell #5 (unjustified generic) does not apply — this is a concrete 2-arg method, not a generic | Inline the same 5-line `switch` duplicated at both call sites | Two call sites is the threshold where duplicating identical classification logic (not just similar — literally the same 3-way `switch`) costs more in review/maintenance than one small private method; the two sites still differ in *propagation* (channel send vs. direct return), so only the classification, not the control flow, is shared — avoids over-abstracting the parts that legitimately differ. |
| Non-NotFound error visibility | `log.Error(...)` at the point of detection, in addition to propagating the error via return/errChan | research/pitfalls.md #1 ("log INSIDE goroutine B before sending on errChan... don't rely solely on the return value for visibility") | Rely on the returned/propagated error alone | `Setup()`'s two-goroutine race means if goroutine A (MkdirAll) also errors, only one of the two errors is ever returned (first read off `errChan` wins) — logging at the detection site guarantees the real ref error is visible even when it loses that race. |

---

## Migration Plan
N/A — complexity 2, no schema or data changes.

## Observability Plan
A non-`ErrReferenceNotFound` error from either call site now emits `log.Error("failed to check branch reference", "branch", <name>, "error", <err>)` at detection time (previously: silently swallowed, no log line at all). No new metric, dashboard, or alert is introduced — this is a visibility fix for an existing failure path, not a new observable operation.

## Risk Control
N/A — complexity 2, no rollout risk beyond standard PR revert. The change is a pure error-classification fix behind existing call paths already exercised by every worktree creation; no feature flag or staged rollout is warranted. Standard `git revert` of the merge commit is sufficient if a regression surfaces.

**Severity update (research/features.md §5, added after this plan's initial adversarial review):** Story 1.1.2's fix is not merely a cosmetic error-message improvement — `setupNewWorktree()`'s ref check (line ~235) is the *only* gate before `cleanupExistingBranch()` (`worktree_branch.go:11`) unconditionally calls `repo.Storer.RemoveReference(branchRef)`. Today, a non-`ErrReferenceNotFound` error is misclassified as "branch absent" and falls straight through into that unconditional deletion — i.e. the current bug can silently delete a real branch ref (same destructive-`RemoveReference` class `5087a282e` fixed in `Cleanup()`, but that fix never touched this second call site). Task 1.1.2a's `return err` on a non-nil `branchRefExists` error already prevents this (verified: the early return happens before `cleanupExistingBranch` is reached), so no code change is needed beyond what Story 1.1.2 already specifies — but Task 1.2.1a's regression test should assert the branch ref **still exists** after a forced-error `Setup()`/`setupNewWorktree()` call (mirroring `TestCleanup_PreservesBranchWithCommits`'s pattern), not just that an error is returned, so this data-loss guard is what's actually tested rather than only the error-surfacing behavior.

**Known pre-existing behavior, not introduced or fixed by this plan**: `setupNewWorktree()` already force-removes the target worktree directory (`git worktree remove -f`, `worktree_ops.go:225`) *before* the ref-check this plan fixes (`worktree_ops.go:235`). If `branchRefExists` returns a genuine error and the function bails early, that destructive removal has already run as a side effect regardless of the fix — a ref-check error still lets an unrelated destructive step execute first. Accepted as out of scope; the fix's goal is correct error classification, not reordering the pre-existing cleanup step.

## Known Gaps

- **TOCTOU window not covered by this fix's tests**: no test covers the race between `setupNewWorktree()`'s branch re-check (`worktree_ops.go:235`) and its own `git worktree add -b` call a few lines later (`worktree_ops.go:~262`, after `cleanupExistingBranch` + `rev-parse HEAD`). This is the narrowest and most likely real race window for "branch already exists" given `backlogWorkBranchSlug()`'s deterministic naming (see Unresolved Questions below) — two concurrent or back-to-back-retried `Setup()`/`CreateBacklogWorktree` calls for the same branch name can both correctly see `ErrReferenceNotFound` (no misclassification at all), both proceed toward creation, and whichever's `git worktree add -b` runs second hits git's own real "already exists" error. This plan's classify-and-surface change does nothing for that race, since no erroring `repo.Reference()` call occurs in that scenario. Not covered here per requirements.md's "Out of Scope: Broader retry/backoff strategy redesign for backlog spawn attempts." Recommended follow-up (filed as its own investigation, not silently dropped, per `.claude/rules/fix-flaky-tests-dont-defer.md`'s spirit): a per-branch-name locking/mutex strategy, or a `git worktree add` retry-once-on-already-exists.

## Unresolved Questions

1. **Root cause of the originally observed failure is unconfirmed.** This plan's fix addresses a confirmed code defect (any `repo.Reference()` error — not just `ErrReferenceNotFound` — is misclassified as "branch absent") and is reasonable defensive hardening against ref-read corruption regardless of cause. Whether this defect is *the* mechanism that produced the original "branch already exists" failure on backlog item `200b6896-3d63-421b-9e96-41cf06d289fa` (and the "Not converging 20h" stuck state on `ccbfe7a6-3102-4110-a11f-6e34ece51798`) is **UNCONFIRMED** — this session has no access to the original failure's log evidence (the actual `err.Error()` text from the `CreateBacklogWorktree setup: ...` failure). Before or shortly after this fix ships, pull that log line and confirm it shows a non-`ErrReferenceNotFound` `repo.Reference()` failure, not a `git worktree add -b ... already exists` failure arriving via the TOCTOU race described next. If no such log evidence exists or surfaces, treat this as hardening for a hypothesized-but-unconfirmed cause, not a confirmed fix for the observed symptom.
2. **Named alternate/additional cause, explicitly out of scope here: a TOCTOU race between concurrent/retried `Setup()` calls on the same deterministic branch name.** `session/instance_worktree.go`'s `CreateBacklogWorktree` calls `wt.Setup()` with a branch name derived deterministically from `(repoPath, title)`. Two concurrent or back-to-back-retried calls for the same backlog item can both legitimately see `ErrReferenceNotFound` (a correct classification, not a bug), both proceed toward `setupNewWorktree()`, and whichever's `git worktree add -b` runs second hits git's genuine "already exists" error — see the Known Gaps entry above for the specific window. This plan does not fix that race and does not attempt to (out of scope per requirements.md's retry/backoff exclusion); it should be filed as its own follow-up investigation rather than silently dropped once this fix ships, so the two candidate causes aren't conflated after the fact.

## Dependency Visualization

```
Task 1.1.1a (add branchRefExists helper + "errors" import)
        │
        ▼
Task 1.1.1b (fix Setup() goroutine call site)   Task 1.1.2a (fix setupNewWorktree() call site)
        │                                                 │
        └───────────────────┬─────────────────────────────┘
                             ▼
                Task 1.2.1a (regression test: fabricated bad ref)
                             │
                             ▼
                Task 1.2.1b (regression test: existing pass/fail behavior unchanged)
                             │
                             ▼
                Task 1.2.1c (make quick-check)
```

Task 1.1.1b and Task 1.1.2a both depend only on 1.1.1a and are independent of each other (different functions, same file — sequential edits recommended to avoid diff churn, but no logical ordering constraint between them).

---

## Phase 1: Fix branch-existence ref-error classification

### Epic 1.1: Correct `repo.Reference()` error handling in `session/git/worktree_ops.go`
**Goal**: Both call sites distinguish "branch genuinely absent" (`plumbing.ErrReferenceNotFound`) from any other `repo.Reference()` error, and the latter is surfaced (returned/propagated) instead of silently falling through to branch creation.

#### Story 1.1.1: Extract a shared classification helper and fix `Setup()`'s goroutine
**As a** backlog automation operator, **I want** `Setup()`'s branch-check goroutine to stop treating *any* `repo.Reference()` error as "branch absent", **so that** a genuine ref-read failure (e.g. `packed-refs` corruption) surfaces as a real error instead of silently routing into the branch-creation path. (Whether a "racy read against a concurrent `git worktree add`/`git branch`" is actually the mechanism that produces such an error in practice is unconfirmed — see the Feature description and Unresolved Questions.)

**Acceptance Criteria**:
- `Setup()`'s goroutine calls the new `branchRefExists` helper instead of inlining `if _, err := repo.Reference(branchRef, false); err == nil`.
  - *Given* a repo where `.git/packed-refs` has been overwritten with malformed content (e.g. `"not a valid packed-refs file\nrandom garbage"`) and branch `backlog/fix-typo-abc123` has **no** loose ref file at `.git/refs/heads/backlog/fix-typo-abc123` (the branch has no loose ref at all, so the lookup falls through to the corrupted packed-refs parser — verified against go-git v5.14.0, see fixture note below), *When* `Setup()` runs its goroutine and calls `g.branchRefExists(repo, plumbing.NewBranchReferenceName("backlog/fix-typo-abc123"))`, *Then* the helper returns `(false, err)` with `err` non-nil (observed: `"malformed packed-ref"`) and NOT satisfying `errors.Is(err, plumbing.ErrReferenceNotFound)`.
- A non-`ErrReferenceNotFound` error is logged via `log.Error` and sent on `errChan`, causing `Setup()` to return that error instead of proceeding to `setupNewWorktree()`.
  - *Given* the same packed-refs-corruption fixture, *When* `Setup()` is called, *Then* `Setup()` returns a non-nil error whose message contains `"failed to check branch reference"`, and the process never reaches `git worktree add -b backlog/fix-typo-abc123 ...` (i.e. no `fatal: a branch named ... already exists` is produced, because worktree add is never invoked).
- Existing "branch absent → create" behavior is unchanged.
  - *Given* a fresh repo from `setupTestRepo(t)` where branch `test-new-worktree` has no ref file at all, *When* `Setup()` runs, *Then* `branchRefExists` returns `(false, nil)`, `branchExists` stays `false`, and `Setup()` proceeds to `setupNewWorktree()` and succeeds (matches existing `TestNewWorktreeSetup_SetsBaseCommitSHA` behavior).

**Fixture note (corrected 2026-08-11 after adversarial review)**: an earlier draft of this plan used a malformed *loose* ref file (writing `not-a-valid-sha\n` into `.git/refs/heads/<branch>`) as the "genuine non-NotFound error" fixture. That fixture is wrong: `plumbing.NewHash()` (`plumbing/hash.go`) silently discards hex-decode failures via `hex.DecodeString(s)` and returns a zero `Hash` with a **nil** error, so `repo.Reference()` returns `(zeroHashRef, nil)` — indistinguishable from "branch exists." Verified directly against the pinned `github.com/go-git/go-git/v5@v5.14.0` with a throwaway `go run` program: the loose-ref fixture produced `err: <nil>`, `ref: 0000000000000000000000000000000000000000 refs/heads/backlog/fix-typo-abc123`. The packed-refs-corruption fixture used above was verified in the same run and produced `err: "malformed packed-ref"`, `err == nil: false`, `errors.Is(err, plumbing.ErrReferenceNotFound): false` — a genuine, distinguishable error. All Given/When/Then blocks and the Task 1.2.1a test body in this plan use the verified packed-refs fixture.

**Files**: `session/git/worktree_ops.go`

##### Task 1.1.1a: Add `errors` import and `branchRefExists` helper (~3 min)
- Add `"errors"` to the import block in `session/git/worktree_ops.go` (alongside existing `"context"`, `"fmt"`, etc.).
- Add a new private method directly above `Setup()` (before line 17's doc comment):
  ```go
  // branchRefExists reports whether branchRef exists in repo, distinguishing a genuine
  // "no such branch" (plumbing.ErrReferenceNotFound) from any other ref-read error (I/O,
  // lock contention from a concurrent git worktree add/git branch, etc.) — the latter must
  // never be treated as "branch does not exist".
  func (g *GitWorktree) branchRefExists(repo *git.Repository, branchRef plumbing.ReferenceName) (bool, error) {
      _, err := repo.Reference(branchRef, false)
      switch {
      case err == nil:
          return true, nil
      case errors.Is(err, plumbing.ErrReferenceNotFound):
          return false, nil
      default:
          return false, fmt.Errorf("failed to check branch reference %s: %w", branchRef, err)
      }
  }
  ```
- Files: `session/git/worktree_ops.go`

##### Task 1.1.1b: Rewire `Setup()`'s goroutine to use the helper (~3 min)
- Replace lines 34-47 (the branch-check goroutine) with:
  ```go
  // Goroutine for branch check
  go func() {
      repo, err := git.PlainOpen(g.repoPath)
      if err != nil {
          errChan <- fmt.Errorf("failed to open repository: %w", err)
          return
      }

      branchRef := plumbing.NewBranchReferenceName(g.branchName)
      exists, err := g.branchRefExists(repo, branchRef)
      if err != nil {
          log.Error("failed to check branch reference", "branch", g.branchName, "error", err)
          errChan <- err
          return
      }
      branchExists = exists
      errChan <- nil
  }()
  ```
- Note: `branchExists` is still written exactly once by this goroutine before its single `errChan` send, and read only after the `for i := 0; i < 2; i++` wait loop completes (unchanged happens-before via channel receive — per research/pitfalls.md #1, do not alter this shape).
- Files: `session/git/worktree_ops.go`

#### Story 1.1.2: Fix `setupNewWorktree()`'s pre-creation re-check
**As a** backlog automation operator, **I want** `setupNewWorktree()`'s branch re-check to stop treating *any* `repo.Reference()` error as "branch absent", **so that** it doesn't fall through into `git worktree add -b <branch>` and produce the fatal "branch already exists" error when the branch does in fact exist (or the check itself failed for an unrelated reason, e.g. ref corruption). This closes a confirmed code defect; it does not by itself rule out the TOCTOU race named in Unresolved Questions as an additional or alternate cause of the originally observed failure.

**Acceptance Criteria**:
- `setupNewWorktree()` calls the same `branchRefExists` helper instead of inlining `if _, err := repo.Reference(branchRef, false); err == nil`.
  - *Given* the same packed-refs-corruption fixture (branch `backlog/fix-typo-abc123` has no loose ref; `.git/packed-refs` is overwritten with malformed content), *When* `setupNewWorktree()` is called directly (e.g. via `Setup()` racing `branchExists=false` from a stale read, or by calling it directly in a test), *Then* it returns a non-nil error containing `"failed to check branch reference"` and does not call `g.runGitCommand(g.repoPath, "worktree", "add", "-b", ...)`. Note `setupNewWorktree()`'s `git worktree remove -f` cleanup (line ~225) runs *before* this ref check regardless of outcome — see the Risk Control note below on this pre-existing ordering.
- Existing "branch exists → reuse via `setupFromExistingBranch`" behavior is unchanged.
  - *Given* a repo where branch `existing-branch` already has a valid ref pointing at a real commit, *When* `setupNewWorktree()` is called, *Then* `branchRefExists` returns `(true, nil)`, and `setupNewWorktree()` logs `"branch already exists, using existing branch for worktree"` and returns `g.setupFromExistingBranch()`'s result, same as before.
**Files**: `session/git/worktree_ops.go`

##### Task 1.1.2a: Rewire `setupNewWorktree()`'s branch re-check to use the helper (~3 min)
- Replace lines 233-239 with:
  ```go
  // Check if the branch already exists - if so, use it instead of cleaning up
  branchRef := plumbing.NewBranchReferenceName(g.branchName)
  exists, err := g.branchRefExists(repo, branchRef)
  if err != nil {
      log.Error("failed to check branch reference", "branch", g.branchName, "error", err)
      return err
  }
  if exists {
      // Branch exists - use setupFromExistingBranch instead
      log.Info("branch already exists, using existing branch for worktree", "branch", g.branchName)
      return g.setupFromExistingBranch()
  }
  ```
- Files: `session/git/worktree_ops.go`

---

### Epic 1.2: Regression test coverage
**Goal**: A deterministic (non-timing-based, per `.claude/rules/fix-flaky-tests-dont-defer.md`) unit test proves a non-`ErrReferenceNotFound` ref error is no longer misclassified as "branch does not exist" — i.e. would have caught this bug before the fix.

#### Story 1.2.1: Add fabricated-bad-ref regression test
**As a** maintainer, **I want** a test that fabricates a malformed on-disk ref and asserts `Setup()`/`setupNewWorktree()` surface an error instead of silently proceeding to branch creation, **so that** this misclassification can't regress silently.

**Acceptance Criteria**:
- A new test demonstrates the fix.
  - *Given* `setupTestRepo(t)` creates a repo at `repoDir`, and the test overwrites `filepath.Join(repoDir, ".git", "packed-refs")` with malformed content (`"not a valid packed-refs file\nrandom garbage"`) while leaving branch `backlog/fix-typo-abc123` with no loose ref at all, for a `GitWorktree` constructed via `NewGitWorktreeWithBranch(repoDir, "test-bad-ref", "backlog/fix-typo-abc123")`, *When* `wt.Setup()` is called, *Then* `Setup()` returns a non-nil error whose message contains `"failed to check branch reference"`, and `filepath.Join(repoDir, "worktrees")` does NOT contain a worktree checked out at the branch (i.e. `git worktree add -b` was never reached).
- Existing "exists → reuse" and "absent → create" paths remain covered (already covered by pre-existing tests in `worktree_creation_test.go`; this task only adds the missing non-NotFound case, per requirements criterion 3's "unchanged and covered by tests").
**Files**: `session/git/worktree_ops_test.go` (new file)

##### Task 1.2.1a: Create `worktree_ops_test.go` with the fabricated-bad-ref regression test (~5 min)
- New file `session/git/worktree_ops_test.go`, package `git`, importing `os`, `path/filepath`, `testing`, `github.com/stretchr/testify/assert`, `github.com/stretchr/testify/require`.
- Test function `TestSetup_SurfacesError_When_BranchRefIsMalformed`:
  ```go
  func TestSetup_SurfacesError_When_BranchRefIsMalformed(t *testing.T) {
      repoDir := setupTestRepo(t)

      branchName := "backlog/fix-typo-abc123"
      wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-bad-ref", branchName)
      require.NoError(t, err)

      // Fabricate a malformed packed-refs file (not a missing ref — corrupt data) to force
      // repo.Reference() to return a non-ErrReferenceNotFound error. branchName has no loose
      // ref file, so the lookup falls through to the packed-refs parser and fails there.
      //
      // A malformed *loose* ref file does NOT work for this fixture: go-git's
      // plumbing.NewHash() silently discards hex-decode failures and returns a zero hash
      // with a nil error (verified against go-git v5.14.0 — see plan.md's Story 1.1.1
      // fixture note and research/pitfalls.md).
      packedRefsPath := filepath.Join(repoDir, ".git", "packed-refs")
      require.NoError(t, os.WriteFile(packedRefsPath, []byte("not a valid packed-refs file\nrandom garbage"), 0644))

      err = wt.Setup()
      require.Error(t, err, "Setup() must surface a malformed ref as an error, not silently treat it as branch-absent")
      assert.Contains(t, err.Error(), "failed to check branch reference")

      // Must not have fallen through to worktree creation.
      _, statErr := os.Stat(wt.worktreePath)
      assert.True(t, os.IsNotExist(statErr), "worktree must not have been created after a ref-check error")
  }
  ```
- Note: uses `wt.worktreePath` (unexported field, same package `git` — test file is in package `git`, not `git_test`, matching the rest of `worktree_creation_test.go`).
- Files: `session/git/worktree_ops_test.go`

##### Task 1.2.1b: Verify existing exists/absent tests still pass unchanged (~2 min)
- Run `go test ./session/git/... -run 'TestNewWorktreeSetup_SetsBaseCommitSHA|TestSetup_SurfacesError_When_BranchRefIsMalformed|TestCleanup_PreservesBranchWithCommits'` to confirm both the new regression test and pre-existing exists/absent-path tests pass with the Story 1.1.1/1.1.2 changes applied.
- If any pre-existing test fails, that indicates the fix altered observable behavior beyond error classification — stop and re-diff against Task 1.1.1b/1.1.2a before proceeding.
- Files: none (verification only)

##### Task 1.2.1c: Run `make quick-check` (~5 min)
- Run `make quick-check` (build + full test suite + lint) from the repo root.
- Confirms acceptance criterion 5 from requirements.md: build, all `session/git` tests (including the new regression test), and lint all pass with no new findings.
- If `gofmt`/lint flags the new helper or test file, run `gofmt -w session/git/worktree_ops.go session/git/worktree_ops_test.go` and re-run.
- Files: none (verification only)
