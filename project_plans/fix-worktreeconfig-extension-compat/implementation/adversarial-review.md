# Adversarial Review: fix-worktreeconfig-extension-compat

**Reviewer**: Adversarial architecture review agent
**Date**: 2026-11-04
**Plan reviewed**: `implementation/plan.md`
**Verdict**: PROCEED — 0 blockers, 2 concerns, 3 minors

---

## Summary verdict

The plan is architecturally sound. The two root causes are real, the fixes are correct, and the prior-art pattern (`resolveRemoteWorktreeBaseCommit`) validates the Phase 2 approach. No issue found that must block implementation.

---

## CONCERN 1 — Wrong error sentinel in Story 2.1.1 acceptance criterion

**Classified**: CONCERN

**Location**: `plan.md` Story 2.1.1 AC, third bullet:

> `("", backend.ErrObjectNotFound)` from `ResolveRef` is wrapped as an error …
> *Then* it returns a non-nil error wrapping `backend.ErrObjectNotFound`.

**Problem**: `backend/backend.go:28` documents `ResolveRef`'s behavior as: "`ErrRefNotFound` when the revision does not exist." `ErrObjectNotFound` is for a different failure class — "a SHA is well-formed but its object is missing from the object database" (`backend/errors.go:19`). A missing `origin/feature-x` ref returns `ErrRefNotFound`, not `ErrObjectNotFound`.

A test written to verify `errors.Is(err, backend.ErrObjectNotFound)` would compile and appear to pass (any non-nil error satisfies `require.Error`) but would not prove the correct sentinel is propagated. If the backend ever changes to return a richer error wrapping `ErrRefNotFound`, the test would fail to catch a regression — or fail to pass in the first place if it uses `errors.Is`.

**Resolution**: Change the AC and any test body to assert `backend.ErrRefNotFound`. The fix code itself (`fmt.Errorf("... %w", err)`) is agnostic to which sentinel is wrapped, so no code change is needed — only the AC and test assertion.

---

## CONCERN 2 — No regression test for `ResolveLocalBranchSHA` with `worktreeConfig = true`

**Classified**: CONCERN

**Location**: Story 3.1.1 covers only `ResolveOriginBranchSHA`. Story 2.1.2 ports `ResolveLocalBranchSHA` but has no corresponding targeted regression test.

**Problem**: `ResolveLocalBranchSHA` is ported for the same root cause (`OpenRepo` failure). Without a test, a future revert or accidental re-introduction of a direct `OpenRepo` call in that function would be caught only if the caller path happens to be exercised by `make test`. The existing test coverage for `ResolveDefaultLocalBranchSHA` (which calls `ResolveLocalBranchSHA`) does not set `worktreeConfig = true` in the test repo config.

**Resolution**: Add a companion test `TestResolveLocalBranchSHA_WorktreeConfigExtension` alongside the existing Story 3.1.1 test, using the same `setupTestRepo` / `runGit` helpers that the other `ops_test.go` tests use. Append `[extensions]\nworktreeConfig = true\n` to the clone's `.git/config` and assert no error + non-empty SHA.

---

## MINOR 1 — Timeout asymmetry and non-propagating context in Phase 2

**Classified**: MINOR

**Location**: Task 2.1.1a and 2.1.2a — both use `context.WithTimeout(context.Background(), 10*time.Second)`.

`ResolveOriginBranchSHA` runs `FetchBranch` (30s timeout, `context.Background()`) and then immediately calls `gitprovider.Backend().ResolveRef` (10s timeout, also `context.Background()`). The asymmetry between a 30s network operation and a 10s local-read that follows it is harmless in practice (the local read is fast), but:

1. Neither context propagates caller cancellation — if a caller's HTTP request or parent context is cancelled, the sub-operations run to their own timeout regardless.
2. A future maintainer who threads a real context from callers must revisit both functions; the asymmetry may confuse them.

This is acknowledged tech debt in the plan. No action needed before implementation — note it as a follow-up for the context-propagation cleanup the plan already names.

---

## MINOR 2 — Fork tag commit lineage not specified

**Classified**: MINOR

**Location**: Task 1.1.4a — "Commit, tag, and push to fork."

The plan doesn't say whether the commit lands on a named branch or as a continuation of the commit `v5.19.3-ssq.1` was tagged from. If `v5.19.3-ssq.1` was tagged off `main` (or a maintenance branch) of the fork, the natural approach is to commit to the same branch and tag it. If the tag was on a detached commit, a future `v5.19.3-ssq.3` will need to cherry-pick rather than branch forward.

No action needed before implementation — just ensure the commit lands on a real branch in the fork so the tag's lineage is navigable.

---

## MINOR 3 — Phase 1 and Phase 2 PR sequencing not stated

**Classified**: MINOR

**Location**: Plan overall structure.

Phase 2 alone fixes the specific broken session-creation path (`ResolveOriginBranchSHA` / `ResolveLocalBranchSHA`). Phase 1 is defense-in-depth for the 14+ remaining `OpenRepo` callers. The plan doesn't state whether both phases ship in one PR or two.

If they ship in separate PRs (which the plan's risk table anticipates — "Fork tag publishing fails … ops.go port can be developed and merged independently"), there is a window between Phase 2 merging and Phase 1 merging during which the other `OpenRepo` callers remain broken for `worktreeConfig = true` repos. For the stated bug (session creation failing), that window is fine — the named failing functions are fixed by Phase 2. But it is worth documenting the sequencing intent so reviewers don't question why the fork bump is a separate follow-up.

---

## Checks not triggered

**Missing failure modes**: The plan correctly notes the fetch timeout, the CLI fallback through the Router, and the `enrichment_test.go:51` behavior. No missing failure modes found.

**Architecture risks**: Both changes are narrowly scoped (map keys + two function bodies). No new abstractions are introduced. The backend pattern is prior art in the same file. No risk.

**Scope drift**: Phase 2 explicitly excludes the 14+ other `OpenRepo` callers with a written disposition. Phase 1 is a minimal two-line change to the fork. No scope drift.

**Technology bets**: The go-git fork is pre-existing. The fix reduces the fork's divergence from upstream. The plan correctly commits to filing an upstream PR (good citizenship). No new technology bets.

**Missing coverage**: Concern 2 above (no test for `ResolveLocalBranchSHA`). Otherwise the stated behaviors map to stories.
