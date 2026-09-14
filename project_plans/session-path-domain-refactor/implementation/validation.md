# Validation Plan: session-path-domain-refactor

**Date**: 2026-09-13
**Scope**: Phase A (backend vocabulary, Session wire fields, corrected docs)

The refactor is additive and must not change any existing value. So the test strategy
is characterization-first: pin today's behavior for every accessor *before* touching
them, then assert the new fields against the same fixtures.

## Requirement → Test mapping

| Requirement | Test file | Test name | Type | Scenario |
|---|---|---|---|---|
| R1 `ActiveDir` is disk-agnostic | `session/instance_worktree_test.go` | `TestWorkspace_ActiveDir_KeepsWorktreeDir_WhenMissingFromDisk` | Unit | worktree declared, directory deleted → `ActiveDir` is still the worktree path |
| R1 | `session/instance_worktree_test.go` | `TestWorkspace_ActiveDir_IsRepoRoot_WhenNoWorktree` | Unit | no worktree → `ActiveDir == RepoRoot` |
| R2 `ExistingDir` falls back | `session/instance_worktree_test.go` | `TestWorkspace_ExistingDir_FallsBackToRepoRoot_WhenWorktreeMissingFromDisk` | Unit | worktree gone → `ExistingDir == RepoRoot` while `ActiveDir` does not change |
| R2 | `session/instance_worktree_test.go` | `TestWorkspace_ExistingDir_IsWorktreeDir_WhenPresentOnDisk` | Unit | worktree exists → no fallback (guards against an unconditional preference for RepoRoot) |
| R3 `ActiveDir` and `ExistingDir` diverge — the bug's mechanism | `session/instance_worktree_test.go` | `TestWorkspace_ActiveDirAndExistingDir_Diverge_WhenWorktreeMissing` | Unit | **the regression test for the `WorkspacePeersPanel` false collision**: two instances, different deleted worktrees, same repo → equal `ExistingDir`, distinct `ActiveDir` |
| R4 `WorktreeDir` is empty without a worktree | `session/instance_worktree_test.go` | `TestWorkspace_WorktreeDir_IsEmpty_WhenNoWorktree` | Unit | directory session → `WorktreeDir == ""` |
| R5 No behavior change for existing callers | `session/instance_worktree_test.go` | existing `TestGetEffectiveRootDir_ReturnsWorktreePath_EvenWhenMissingFromDisk` (:183), `TestWorkspace_FallsBackToRepoRoot_WhenWorktreePathMissingFromDisk` (:202), `TestWorkspace_UsesWorktreePath_WhenPresentOnDisk` (:218) | Unit | **must pass unmodified** — the contract that this refactor renames rather than changes |
| R5 | `session/instance_worktree_test.go` | `TestWorkspace_ExistingDir_MatchesFormerEffectivePath` | Unit | `ExistingDir` reproduces the removed `EffectivePath` exactly, for each of the three migrated readers' cases |
| R6 The one intended behavior change | `session/instance_worktree_test.go` | `TestGetWorkingDirectory_ReturnsRepoRoot_WhenWorktreePathEmpty` | Unit | `HasWorktree()` true but `GetWorktreePath()` `""` — today `GetWorkingDirectory` returns `""` and `GetEffectiveRootDir` returns the repo root. Pins the convergence on the non-empty answer. |
| R7 Race safety preserved | `session/instance_worktree_test.go` | existing `TestGetEffectiveRootDir_ConcurrentWithSetGitHubResolution_NoRace` (:260) + a `Workspace()` equivalent | Unit (`-race`) | concurrent `setGitHubResolutionLocked` write vs. `Workspace()` read |
| R8 Proto fields carry the right values | `server/adapters/instance_adapter_test.go` | `TestInstanceToProto_PopulatesPathVocabulary` | Unit | all four new fields match `inst.Workspace()` |
| R8 | `server/adapters/instance_adapter_test.go` | `TestInstanceToProto_ActiveDirAndExistingDir_Diverge_WhenWorktreeMissing` | Unit | the divergence survives the adapter boundary — this file currently has **zero** assertions on any path field |
| R9 Legacy fields unchanged | `server/adapters/instance_adapter_test.go` | `TestInstanceToProto_LegacyPathFields_Unchanged` | Unit | `path` still equals `Workspace().ExistingDir` (the former `EffectivePath`), `working_dir` still equals `GetWorkingDirectory()` — the additive guarantee |

Not covered here, because deferred out of Phase A with reasoning in `plan.md`: the MCP
surface, the `ReviewItem` message, the frontend, and the `norawinstancepath` analyzer.

## Test stack

- **Unit (Go)**: stdlib `testing` + `testify/require`, matching the existing suite.
- **Fixtures**: `t.TempDir()` with `newTestGitWorktree(repoPath, worktreePath)`, the helper
  the existing worktree tests already use. Note `git.CanonicalizeWorktreePath` runs when
  the directory exists on disk, so on-disk expectations must be canonicalized —
  `instance_worktree_test.go:218` documents the macOS `/var` → `/private/var` flake (#558)
  this guards against.


- **No new frontend tests in Phase A** — no frontend file changes in Phase A.

## Coverage targets

- Every new `Workspace` field: at least one worktree-present and one worktree-absent case.
- Every new proto field: one adapter assertion.
- The `ActiveDir` / `ExistingDir` divergence: asserted at both the domain layer and the
  adapter layer, because that divergence is the entire reason the vocabulary exists and a
  single-layer test would not catch the adapter wiring the wrong one.

## Gate

`make build`, `make test` (affected packages), `make lint`, `make registry-diff`.
Registry baseline recorded before any change: **267 committed / 266 generated, 0.37%
divergence, validation passed** — pre-existing, so `registry-diff` must still report
0.37% afterwards, not 0.00%.
jscpd baseline: **0.1% against a 0.12% threshold** — 0.02% headroom, so Phase B must
extract shared comparison logic rather than copy it.
