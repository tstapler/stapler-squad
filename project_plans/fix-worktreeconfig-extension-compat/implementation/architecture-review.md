# Architecture Review: fix-worktreeconfig-extension-compat

**Reviewer**: architecture-review subagent
**Date**: 2026-11-04
**Verdict**: APPROVED — 0 blockers, 2 concerns

No `docs/adr/ADR-000-architecture-constitution.md` found; constitution check is N/A.

---

## Lens 1 — Structural integrity (SOLID, Clean Architecture, DDD)

**SOLID**: No violations. Phase 2 replaces two function bodies with a narrower, single-responsibility call; it does not introduce new responsibilities or new dependencies beyond those already in `ops.go`.

**Layer coupling**: Clean. `ops.go` already imports `backend`, `gitprovider`, and the `gitprovider.Backend()` accessor. The proposed change moves two callers from `OpenRepo` (a go-git direct dependency leaking into `ops.go`) to `gitprovider.Backend().ResolveRef` (the established abstraction seam). This is a tightening of layer compliance, not a loosening.

**DDD aggregate boundaries**: Not applicable — this is a git-plumbing utility package, not a domain aggregate. No aggregate boundary issues.

**Testability**: Both ported functions will be testable against a file-system-local git repo (the regression test plan in Story 3.1.1 sets one up with `git init --bare`). No new global state introduced. The `context.Background()` + timeout approach matches `FetchBranch`; callers that need timeout control can still wrap the call.

---

## Lens 2 — Type-level design

**Primitive obsession**: None introduced. `backend.RefName`, `backend.Local`, `backend.RepoRoot`, and `backend.CommitSHA` are the correct typed wrappers and are used correctly in the proposed replacement code.

**Illegal states**: The proposed code converts `CommitSHA` → `string` with `string(sha)` — correct, since `CommitSHA` is a string type alias.

**Parse-at-boundary**: Correct. The only user-supplied string entering `ResolveRef` is `mainBranch` / `branchName`, which is already validated by `ValidateBranchName` upstream (called before `ResolveExplicitBranchSHA`, which routes to both functions). `FetchBranch` already guards against empty branch names.

**Concern 1 — Wrong error sentinel in Story 2.1.1 AC** (concern, not blocker):

Story 2.1.1's third AC reads:
> `("", backend.ErrObjectNotFound)` from `ResolveRef` is wrapped as an error.

This is the wrong sentinel. `backend.ResolveRef` returns `backend.ErrRefNotFound` when a revision does not exist — documented in `session/git/backend/backend.go:29` ("ErrRefNotFound when the revision does not exist"), confirmed in the CLI implementation (`cli/cli.go:259-262` maps "unknown revision" → `ErrRefNotFound`) and the gogit implementation (`gogit/refs.go:101` calls `notFound(...)` which wraps `backend.ErrRefNotFound`).

`backend.ErrObjectNotFound` means a well-formed SHA is missing from the object database — a different failure class (`errors.go:18-19`).

The fix code itself (`return "", fmt.Errorf("failed to resolve origin/%s: %w", mainBranch, err)`) is correct — it wraps whatever `ResolveRef` returns. The concern is that a test written against the AC asserting `errors.Is(err, backend.ErrObjectNotFound)` will silently test the wrong sentinel and either fail or give false confidence about the right error path.

**Correction**: The AC for Story 2.1.1 Criterion 3 should read `backend.ErrRefNotFound`, not `backend.ErrObjectNotFound`.

---

## Lens 3 — Pattern selection (GoF + PoEAA)

**PoEAA pattern fit**: The backend Router (`session/git/backend`) is a Gateway (PoEAA). Using it here is correct — `ops.go` has always been a Service Layer client of the Gateway. Removing the direct go-git call restores this alignment.

**GoF pattern appropriateness**: No new patterns introduced. The fix removes a Strategy leak (direct call to a concrete implementation instead of going through the interface).

**API contract design**: Signatures of `ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` are unchanged. No callers need updating.

**Concern 2 — Short-form ref for origin resolution** (concern, mitigated):

The plan uses `backend.RefName("origin/"+mainBranch)` for `ResolveOriginBranchSHA`. This is a DWIM (Do What I Mean) short form that `git rev-parse` expands to `refs/remotes/origin/<mainBranch>`. The plan acknowledges this in the Risk Control section. The prior art in `resolveRemoteWorktreeBaseCommit` (`ops.go:203`) uses the identical form. The unambiguous alternative `refs/remotes/origin/`+mainBranch` would eliminate any DWIM ambiguity at no cost.

However, since: (a) the prior art already uses `"origin/"` short form without issue, (b) the risk is documented, and (c) the only ambiguous case (a local branch literally named `origin/main`) is acknowledged, this is acceptable as-is. Using the full `refs/remotes/` form is a refinement the implementor can apply if preferred; it does not need to block the fix.

**Consistency with build-vs-buy decision**: The go-git fork fix is the right scope. Modifying `verifyExtensions` behavior (the actual bug) in the fork is the minimal correct fix. The plan correctly avoids broader changes to the fork.

---

## Lens 4 — Tech debt trajectory

**Debt disposition present and honest**: Yes. The plan explicitly names 14+ remaining direct `OpenRepo` callers in `ops.go` and other files, declares them pre-existing debt not regressed by this fix, and states the fork fix mitigates their risk. This is accurate and proportionate.

**Disposition sized correctly**: The plan avoids both under-scoping (not fixing the two functions in the session-creation path) and over-scoping (not porting all 14+ `OpenRepo` callers at once). The two-function scope is exactly the right aperture for a targeted bugfix.

**Enrichment test assertion**: The plan correctly identifies that `enrichment_test.go:51` remains valid after the fork fix because `vetRepoConfig`'s V0 guard (line 336 of `layout.go`) fires before `vetExtension` for V0 repos. The in-process backend path still returns an error; the test does not need to change. This reasoning is verified against the source: `layout.go:336` returns `fallbackEligible` for any extension in a non-V1 repo without calling `vetExtension` at all.

---

## Summary

| Finding | Category | Severity |
|---|---|---|
| Story 2.1.1 AC Criterion 3 names `backend.ErrObjectNotFound`; correct sentinel is `backend.ErrRefNotFound` | Type-level / AC correctness | Concern |
| `"origin/"+mainBranch` DWIM short ref vs `"refs/remotes/origin/"+mainBranch` | Pattern / precision | Concern (mitigated by prior art + risk doc) |

No architectural blockers. The plan is well-scoped, respects the existing seam, and has correct prior art for both changes.
