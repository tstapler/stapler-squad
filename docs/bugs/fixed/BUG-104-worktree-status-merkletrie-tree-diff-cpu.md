# BUG-104: session/git Dirty-Check Uses go-git's Full Merkletrie Tree-Diff for a Boolean Question [SEVERITY: Medium]

**Status**: Fixed
**Discovered**: 2026-09-09 (live Pyroscope CPU profiling, 30-minute production window)
**Fixed in**: `stapler-squad-perf` branch

## Problem Description

Live production CPU profiling (Pyroscope, after the `gitignoreFSCache` fix in
`d2255922e`/`a48d54cd6` had already landed) showed `merkletrie.DiffTree`/`diffNodes`
at 16.58% cumulative CPU inside `worktreeIsDirtyWithFS` — the single largest remaining
CPU cost in `session/git`, larger than the gitignore-pattern cost already fixed.

## Root Cause

`worktreeIsDirtyWithFS` (`session/git/worktree_git.go`) and `HasStagedChanges`
answered a boolean question ("did anything change?") by calling go-git's
`Worktree.Status()`, which always computes a *full* tree diff: it builds
noder-wrapped trees for HEAD, the index, and the worktree, then recursively compares
every node via `package merkletrie`. That's the right tool for producing a full
status listing, but it's the wrong algorithmic complexity for a boolean check that can
short-circuit on the first difference found.

`session/unfinished/gogit_vcs_reader.go`'s `GoGitVCSReader.HasUncommitted` already
proved the cheaper approach in production: compare the index against HEAD by hash
(no tree-diff), then compare tracked files' on-disk mtime/size against the index
(no hashing), short-circuiting on the first mismatch.

Classification (per `quality:reflect-and-fix`): **Semantic/Intent** — `Status()` is
syntactically correct and produces the right answer, but its generality (full diff)
is the wrong tool for a call site that only needs a boolean.

## Fix

Added `session/git/worktree_dirty_fast.go`, a self-contained reimplementation of the
same proven algorithm (`session/git` cannot import `session/unfinished` directly — its
test files already import `session/git`, so that direction would be an import cycle):

- `headTreeHashes` — walks HEAD's tree via `object.NewTreeWalker` (visits tree objects
  only, never loads blob content) into a `path -> hash` map.
- `worktreeStagedDirty` — compares the index against `headTreeHashes` by hash;
  detects new/modified/deleted staged entries and unresolved merge-conflict stages
  (`index.Entry.Stage != 0`).
- `worktreeUnstagedDirty` — stats each tracked file and compares size/mtime against
  its index record; no file reads or hashing.
- `worktreeHasUntrackedFiles` — recursive directory walk, gitignore-filtered, for the
  untracked-file case go-git's `Status()` also has to check.
- `worktreeIsDirtyFast` composes the three checks, short-circuiting on the first
  `true`.

`session/git/worktree.go`'s `dirtyCheckerFunc()` now defaults to `worktreeIsDirtyFast`
instead of `worktreeIsDirtyWithFS`. `HasStagedChanges` (`worktree_git.go`) now calls
`worktreeStagedDirty`/`headTreeHashes` directly instead of `Worktree.Status()`.
`worktreeIsDirtyWithFS`/`worktreeIsDirty` are kept (used by existing direct test
callers and as the benchmark comparison baseline).

## Regression Tests

`session/git/worktree_dirty_fast_test.go`:
- `TestWorktreeIsDirtyFast_MatchesWorktreeIsDirty_OnCleanUntrackedAndModified` — clean
  repo, untracked file, modified tracked file.
- `TestWorktreeIsDirtyFast_DetectsStagedAddition` — new file staged via `wt.Add`.
- `TestWorktreeIsDirtyFast_DetectsStagedDeletion` — `git rm --cached`, worktree copy
  untouched.
- `TestWorktreeIsDirtyFast_DetectsMergeConflictStage` — real unresolved merge conflict.
- `TestWorktreeIsDirtyFast_UnbornHEAD_ReportsCleanRegardlessOfIndex` — no commits yet,
  matches `hasUncommittedGoGitPhase`'s existing unborn-HEAD rule.
- `BenchmarkWorktreeIsDirty_FastVsGoGit` — Fast vs. go-git `Status()` on a repo with a
  wide tracked tree (40 dirs × 20 files). Allocations drop by more than half (~27.4K
  vs. ~69.7K allocs/op on a clean repo); wall-clock is a wash in this synthetic
  fixture because both paths are dominated by filesystem syscalls
  (`syscall.rawsyscalln`, ~70-77% of CPU samples in `-cpuprofile`) rather than by
  `merkletrie` itself at this tree size — `merkletrie.(*Iter).current` was <1% of
  samples in either subtest's profile. The 16.58% cum-CPU production finding is
  the number this fix is actually validated against; confirm via a fresh Pyroscope
  profile after deploy, not the synthetic benchmark's wall-clock.
- Existing `TestHasStagedChanges_ExcludesUntrackedButIncludesStaged`,
  `TestIsDirtyWithHint_DetectsRealWorktreeChanges`,
  `TestWorktreeIsDirty_DetectsUntrackedAndModifiedFiles` continue to pass unchanged
  (the first two now exercise the new code path through `dirtyCheckerFunc()`/
  `HasStagedChanges()`).

Full `go test ./session/git/... -race` run: only two pre-existing, unrelated
failures (`findRealGitBinary: no real (non-script) git binary found on PATH`, and
`no git author identity configured`) — both environment issues predating this change,
not caused by it. No data races.

## Systemic Fix / Recurring Shape

This is the second instance of the same shape as the `gitignoreFSCache` fix
(`d2255922e`): **go-git's generic, full-fidelity APIs (`Worktree.Status()`,
unconditional `gitignore.ReadPatterns`) are more expensive than this codebase's
actual call-site needs, which are usually boolean or narrowly-scoped.** No new
lint rule was added — the pattern is repo-specific enough (which go-git call is
"too expensive for its call site") that a general rule would false-positive on
legitimate full-status use elsewhere (e.g. session diff/status RPCs that need
the full listing). The regression tests plus the two fixes' doc comments are the
enforcement: any future dirty-check call site can point at
`worktreeIsDirtyFast`'s doc comment for the rationale instead of reintroducing
`Status()`.

## Related

- BUG (gitignoreFSCache): `d2255922e`, `a48d54cd6` — same recurring shape, gitignore-pattern cost instead of tree-diff cost.
- `session/unfinished/gogit_vcs_reader.go`'s `HasUncommitted` — the proven, already-in-production algorithm this fix mirrors.
