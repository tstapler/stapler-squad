# Pitfalls and Risks: `worktreeConfig` Extension Compatibility Fix

## 1. Case-sensitivity bug scope is wider than `worktreeConfig`

**Root cause confirmed.** `extensions()` in `repository_extensions.go` (line 73) lowercases every key with `strings.ToLower(opt.Key)`, so `ext.name` is always lowercase. But `extensionsValidForV0` stores three camelCase keys that never match:

| Map key (camelCase) | Actual lookup key (lowercased) | Match? |
|---|---|---|
| `"partialClone"` | `"partialclone"` | No — silent false negative |
| `"preciousObjects"` | `"preciousobjects"` | No — silent false negative |
| `"worktreeConfig"` | `"worktreeconfig"` | No — the bug being fixed |

**Risk:** Any repo with `extensions.partialClone` or `extensions.preciousObjects` and `repositoryFormatVersion=0` (or unset) currently fails `verifyExtensions` with "unsupported extension for V0" when it should pass. The fix must lowercase all three keys in `extensionsValidForV0` to match what `extensions()` produces — fixing only `worktreeConfig` leaves the other two still broken.

`builtinExtensions` only has `"noop"` and `"noop-v1"`, both already lowercase — that map is structurally consistent; only the missing `"worktreeconfig"` entry needs to be added.

**Test gap in the fork.** `repository_extensions_test.go` has no test case for V0 allowance of a camelCase extension name (e.g., `extensions.worktreeConfig = true` with `repositoryFormatVersion=0`). The passing test for V1 with `"NOOP"` works only because `builtinExtensions` is case-matched after `extensions()` lowercases — it doesn't exercise `extensionsValidForV0`. Add test cases for all three camelCase V0-allowed extensions to avoid a silent regression if the map is edited again.

---

## 2. Adding `worktreeconfig` to `builtinExtensions` is safe — the backend falls back anyway

**Concern:** If go-git's `verifyExtensions` passes for a repo with `extensions.worktreeConfig = true`, go-git opens the repo and reads config from `.git/config` without merging in per-worktree overrides from `.git/worktrees/<name>/config.worktree`. This would silently miss branch-tracking config set per-worktree.

**Why it is safe in practice.** The Router's pre-flight check (`vetExtension` in `gogit/layout.go:371–385`) returns `fallbackEligible(...)` whenever `worktreeConfig` is truthy, forcing every in-process go-git op in such repos to fall back to the CLI backend. The CLI backend (`session/git/native/gitconfig/resolver.go:299–302`) already reads `config.worktree` correctly when `extensions.worktreeConfig = true`.

So:

- `worktreeConfig = false` (or absent): passes both `verifyExtensions` and `vetExtension` → go-git handles it, behavior unchanged (no worktree config to miss).
- `worktreeConfig = true`: passes `verifyExtensions` (after the fix) → go-git opens the repo → `vetExtension` fires → Router falls back to CLI → CLI reads `config.worktree`. No silent data loss.

Existing test evidence: `TestReadEnrichment_RejectsIncludesAndUnsupported` (line 51 in `enrichment_test.go`) explicitly asserts that `ReadEnrichment` returns an error for `[extensions]\n worktreeConfig = true\n`. This remains correct after the fix because `ReadEnrichment` calls `vetExtension`, not `verifyExtensions`. The two functions operate at different layers and are both kept consistent.

---

## 3. Porting `ResolveOriginBranchSHA`/`ResolveLocalBranchSHA` to `backend.ResolveRef`: one naming risk

**Current code path:**

```
FetchBranch(repoPath, mainBranch)       // uses gitprovider.Backend().Fetch (CLI, no go-git open)
OpenRepo(repoPath)                       // triggers verifyExtensions — fails for worktreeConfig
repo.Reference("origin/main", true)     // go-git ref lookup
```

**Proposed replacement:**

```
FetchBranch(repoPath, mainBranch)       // unchanged
backend.Local{Root: backend.RepoRoot(repoPath)}
gitprovider.Backend().ResolveRef(ctx, loc, backend.RefName("origin/"+mainBranch))
```

This matches the pattern already used by `resolveRemoteWorktreeBaseCommit` (ops.go:194–224) and is verified working.

**Risk: `backend.RefName("origin/main")` disambiguation.** The CLI backend passes this string to `git rev-parse --verify origin/main`. Git's disambiguation rules expand `origin/main` to `refs/remotes/origin/main` via the DWIM heuristic. This works after a `FetchBranch` because the ref is written to `refs/remotes/origin/main` on disk. However, if a ref named `origin/main` (without the `refs/remotes/` prefix) exists in `refs/heads/`, git's disambiguation could pick it over the remote-tracking ref. This is an edge case (a local branch literally named `origin/main`) and matches the existing behavior of the remote counterpart. The existing tests (`TestResolveOriginBranchSHA_ReturnsFetchedTip_When_OriginHasAdvanced`, `TestResolveOriginBranchSHA_ReturnsError_When_FetchFails`) remain valid after the port since they clone against a local disk origin, exercising the same CLI path.

**Risk: no new context parameter.** `ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` currently take no `context.Context`. `backend.ResolveRef` requires one. The port must either add `ctx context.Context` as a first parameter (a callers-touching API change) or use `context.Background()` (which skips deadline propagation). Check whether callers already hold a context — `ResolveExplicitBranchSHA` and `ResolveWorktreeBaseCommit` both have callers that pass a context down through `session/instance_worktree.go`; threading it through is cleaner than a background context.

**Fork publishing step.** The replace directive in `go.mod` is version-pinned (`github.com/tstapler/go-git/v5 v5.19.3-ssq.1`), not a local directory. Any change to `repository_extensions.go` in the fork requires: clone `github.com/tstapler/go-git`, push the fix, tag a new pre-release (`v5.19.3-ssq.2`), and update the `replace` line in `go.mod` before `go mod tidy`. There is no local editable checkout of the fork under `~/code/github.com/tstapler/`; the copy at `/home/tstapler/go/pkg/mod/` is the read-only module cache.
