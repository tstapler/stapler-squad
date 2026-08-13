# Adversarial Review: worktree-branch-exists-race

**Date**: 2026-08-11
**Verdict**: CLEAN

## Blockers

None open.

- [x] **RESOLVED — fixture (was Blocker 1).** The malformed-loose-ref fixture (`not-a-valid-sha\n` in a loose ref file) has been replaced everywhere with the packed-refs-corruption fixture (`.git/packed-refs` overwritten with `"not a valid packed-refs file\nrandom garbage"` while the target branch has no loose ref): Story 1.1.1's Given/When/Then (plan.md:78,80), Story 1.1.2's Given/When/Then (plan.md:140), and Task 1.2.1a's test body (plan.md:196-197). A "Fixture note (corrected 2026-08-11...)" (plan.md:84) explicitly documents the old fixture's failure mode and cites the corrected one. `research/pitfalls.md:206-225` carries a matching inline "CORRECTION" retraction of its own earlier (false) claim, so the two docs are consistent, not contradictory.

  **Independently re-verified** (not just trusted the plan's claim) by writing a throwaway Go program (`/tmp/verify-blocker1/main.go`) against a `go.mod` pinned to `github.com/go-git/go-git/v5 v5.14.0` — the exact version in this repo's `go.mod` (`grep go-git go.mod` confirms `v5.14.0`). The program: `git.PlainInit`'d a fresh temp repo, overwrote `.git/packed-refs` with the plan's exact malformed content, left `backlog/fix-typo-abc123` with no loose ref, then called `repo.Reference(branchRef, false)`. Observed output:

  ```
  packed-refs-corruption fixture:
    ref: <nil>
    err: malformed packed-ref
    err == nil: false
    errors.Is(err, plumbing.ErrReferenceNotFound): false

  loose-ref malformed fixture (for comparison):
    ref: 0000000000000000000000000000000000000000 refs/heads/backlog/fix-typo-abc123
    err: <nil>
  ```

  This matches the plan's claims exactly on both counts: the packed-refs fixture produces a genuine, distinguishable non-`ErrReferenceNotFound` error (`"malformed packed-ref"`), and the old loose-ref fixture produces `err: <nil>` with a zero-hash ref (confirming it would have been indistinguishable from "branch exists," i.e. confirming the original Blocker 1 finding was itself correct and the fix for it is now verified correct).

- [x] **RESOLVED — root-cause overclaim (was Blocker 2).** The plan now explicitly labels the root-cause mechanism as unconfirmed rather than asserting it as fact, and names the TOCTOU race as an alternate/additional cause with a recommended follow-up:
  - Feature description (plan.md:3): states the fix addresses "a confirmed code defect" but that whether this defect is *the* cause of the original failure "is **UNCONFIRMED**," cites the lack of access to original log evidence, and notes go-git's own error-swallowing makes the "transient ref-read failure" story implausible for the most common case.
  - Story 1.1.1's "so that" clause (plan.md:74): explicitly flags "Whether... is actually the mechanism that produces such an error in practice is unconfirmed."
  - Story 1.1.2's "so that" clause (plan.md:136): "This closes a confirmed code defect; it does not by itself rule out the TOCTOU race... as an additional or alternate cause."
  - Unresolved Questions (plan.md:40-43): item 1 states root cause is unconfirmed and recommends pulling the original log line before/after shipping; item 2 names the TOCTOU race explicitly and recommends it be "filed as its own follow-up investigation rather than silently dropped."

## Concerns

None open.

- [x] **RESOLVED — TOCTOU test gap (was Concern 1).** `Known Gaps` (plan.md:36-38) explicitly acknowledges the race window between `setupNewWorktree()`'s re-check (`worktree_ops.go:~232`) and its own `git worktree add -b` call, states it is not covered by this fix's tests, cites the requirements.md out-of-scope carve-out, and recommends a specific follow-up (per-branch-name locking or retry-once-on-already-exists), consistent with `.claude/rules/fix-flaky-tests-dont-defer.md`'s spirit of not silently re-excusing a known gap.
- [x] **RESOLVED — destructive removal ordering (was Concern 2).** Risk Control (plan.md:33-34) adds an explicit "Known pre-existing behavior, not introduced or fixed by this plan" note: `setupNewWorktree()`'s `git worktree remove -f` runs before the ref-check regardless of outcome, and this is accepted as out of scope. Story 1.1.2's acceptance criteria (plan.md:140) cross-references the same note.

## Minors

- (Carried over, unchanged, non-blocking) Pattern Decisions table still cites `session/unfinished/gogit_vcs_reader.go:751` (confirmed to exist, at `resolveHeadTreeHashes`, lines ~751) as an `errors.Is` precedent; the containing package is named `unfinished`, so it may be dead/unwired code rather than a live convention. `worktree_branch.go`'s own `!=` comparison is already cited alongside it as the primary precedent, so this doesn't affect the plan's soundness.
- (Carried over, unchanged, non-blocking) `setupTestRepo`'s doc comment (`worktree_creation_test.go:15-16`) still claims it returns "the repo directory path and a cleanup function," but the function only returns a string — pre-existing doc/code mismatch, unrelated to this plan.
- No new issue was introduced by the edits: line-number references in Task 1.1.1a/1.1.1b/1.1.2a (e.g. "lines 34-47", "lines 233-239") are off by ~1 line against the current `session/git/worktree_ops.go` (verified: branch-check goroutine is actually lines 34-46; the `setupNewWorktree()` ref-check block is actually lines ~232-238), but this drift predates the fixture-correction edits and doesn't point at the wrong code — not worth blocking on.
