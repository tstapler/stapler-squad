# Validation Plan: worktree-branch-exists-race

**Date**: 2026-08-11

## Happy Path Scenario

Given a fresh repo from `setupTestRepo(t)` where branch `test-new-worktree` has no ref
file at all (neither loose nor packed), when `wt.Setup()` is called, then
`branchRefExists` correctly classifies the branch as genuinely absent
(`errors.Is(err, plumbing.ErrReferenceNotFound)` internally, surfaced as `(false, nil)`),
`Setup()` proceeds to `setupNewWorktree()`, and the worktree is created successfully with
no false "branch already exists" failure.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: distinguish `ErrReferenceNotFound` from other errors at both call sites | `session/git/worktree_ops_test.go` | `TestBranchRefExists_ReturnsFalseNil_When_BranchAbsent` | Unit | Happy path — branch genuinely absent (no loose or packed ref) → `branchRefExists` returns `(false, nil)` |
| REQ-1: distinguish `ErrReferenceNotFound` from other errors at both call sites | `session/git/worktree_ops_test.go` | `TestBranchRefExists_ReturnsTrueNil_When_BranchExists` | Unit | Happy path — branch genuinely present → `branchRefExists` returns `(true, nil)` |
| REQ-1: distinguish `ErrReferenceNotFound` from other errors at both call sites | `session/git/worktree_ops_test.go` | `TestBranchRefExists_ReturnsError_When_PackedRefsCorrupted` | Unit | Error path — `.git/packed-refs` overwritten with malformed content, branch has no loose ref → `branchRefExists` returns `(false, err)` with `err` non-nil and NOT satisfying `errors.Is(err, plumbing.ErrReferenceNotFound)` |
| REQ-1: distinguish `ErrReferenceNotFound` from other errors at both call sites | `session/git/worktree_ops_test.go` | `TestSetup_SurfacesError_When_BranchRefIsMalformed` | Integration | End-to-end confirmation that `Setup()`'s goroutine call site uses the corrected classification (real temp-dir repo, real `git.PlainOpen`, both goroutines racing) |
| REQ-2: non-NotFound error surfaced, not silently treated as "branch absent" | `session/git/worktree_ops_test.go` | `TestSetup_SurfacesError_When_BranchRefIsMalformed` | Integration | Error path — packed-refs-corruption fixture; `Setup()` returns a non-nil error containing `"failed to check branch reference"` and `setupNewWorktree()`/`git worktree add -b` is never reached |
| REQ-2: non-NotFound error surfaced, not silently treated as "branch absent" | `session/git/worktree_ops_test.go` | `TestSetupNewWorktree_SurfacesError_When_BranchRefIsMalformed` | Integration | Error path — same fixture, but calls `setupNewWorktree()` directly (unexported, same package) to cover its own re-check call site independently of `Setup()`'s goroutine; asserts the error propagates and `git worktree add -b` is never invoked |
| REQ-2: non-NotFound error surfaced, not silently treated as "branch absent" | `session/git/worktree_creation_test.go` | `TestNewWorktreeSetup_SetsBaseCommitSHA` (existing, cited) | Integration | Happy path baseline — confirms the non-error path is unaffected by the fix (no spurious error surfaced when the ref genuinely doesn't exist) |
| REQ-3: existing "exists → reuse" / "absent → create" behaviors unchanged, covered by tests | `session/git/worktree_creation_test.go` | `TestNewWorktreeSetup_SetsBaseCommitSHA` (existing, cited — no new test needed) | Integration | branch-absent → create succeeds, exercised end-to-end via `Setup()` → `setupNewWorktree()` |
| REQ-3: existing "exists → reuse" / "absent → create" behaviors unchanged, covered by tests | `session/git/worktree_creation_test.go` | `TestExistingBranchWorktree_SetsBaseCommitSHA` (existing, cited — no new test needed) | Integration | branch-exists → reuse succeeds, exercised via `Setup()`'s own upfront goroutine detection (`branchExists` short-circuits straight to `setupFromExistingBranch`) |
| REQ-3: existing "exists → reuse" / "absent → create" behaviors unchanged, covered by tests | `session/git/worktree_ops_test.go` | `TestSetupNewWorktree_UsesExistingBranch_When_BranchRefExists` | Integration | branch-exists → reuse succeeds via `setupNewWorktree()`'s *own* re-check call site specifically — not exercised by the two existing tests above, which only reach the reuse path through `Setup()`'s upfront goroutine, never through `setupNewWorktree()`'s internal re-check block (`worktree_ops.go:233-239`) |
| REQ-4: new/updated test demonstrates the misclassification is fixed | `session/git/worktree_ops_test.go` | `TestSetup_SurfacesError_When_BranchRefIsMalformed` (named in plan.md Task 1.2.1a — reused, not duplicated) | Integration | Would have failed against the pre-fix code (old code silently proceeds to `setupNewWorktree()` on any `repo.Reference()` error); passes against the fix |
| REQ-5: `make quick-check` passes | N/A (Makefile target, not a Go test) | `make quick-check` (build + test + lint) | Build/Lint/Test gate | Task 1.2.1c — run from repo root after all above tests are added; confirms build, full `session/git` suite (including new tests), and lint are all green |

## UX Acceptance Tests
N/A — pure infrastructure, no user-facing surface.

## Addendum (research/features.md §5, post-initial-review)
`setupNewWorktree()`'s ref check is the only gate before `cleanupExistingBranch()`
unconditionally calls `repo.Storer.RemoveReference(branchRef)` — so a misclassified error
here can silently delete a real branch ref, not just produce a confusing worktree-add
failure. `TestSetupNewWorktree_SurfacesError_When_BranchRefIsMalformed` (REQ-2, above)
should assert the branch ref **still exists** after the forced-error call (query
`repo.Reference(branchRef, false)` post-`Setup()` and expect success), in addition to
asserting the error and the absence of a created worktree — this is what proves the
data-loss guard, not just the error-surfacing behavior.

## Test Stack
- **Unit**: Go stdlib `testing` + `testify` (`assert`/`require`) — matches existing `session/git` test files. `TestBranchRefExists_*` tests call the private `branchRefExists` helper directly (test file is package `git`, same as `worktree_creation_test.go`) against a real temp-dir repo opened via `git.PlainOpen`, with no mocking of go-git — isolates the classification logic from the two calling code paths.
- **Integration**: real temp-dir git repos via `setupTestRepo(t)` (no mocking of go-git or the `git` CLI), exercising `Setup()` and `setupNewWorktree()` as a whole — including the goroutine race in `Setup()` and the actual `runGitCommand` shellouts (`worktree add`, `worktree remove`). This is the "integration" tier for this package per the existing convention in `worktree_creation_test.go`.
- **E2E / UX**: N/A

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/git/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% of the new `branchRefExists` helper's three branches (`err == nil`, `errors.Is(...ErrReferenceNotFound)`, `default`) covered |

- Both call sites (`Setup()`'s goroutine, `setupNewWorktree()`'s re-check) have happy-path and error-path coverage.
- All three `branchRefExists` branches are covered directly (unit) and indirectly through both call sites (integration).
- No mocking of go-git — all fixtures use real `t.TempDir()` repos, consistent with the rest of `session/git`'s test suite and the plan's verified fixture note (malformed *loose* refs don't work; packed-refs corruption does, per the `go-git/v5@v5.14.0` repro in plan.md).
