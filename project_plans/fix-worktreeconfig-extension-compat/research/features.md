# Feature Landscape: worktreeConfig Extension Compat Bugfix

## 1. Extension handling in `vetExtension` / `vetRepoConfig`

File: `session/git/backend/gogit/layout.go`

### Two-bug structure

**Bug A — version gate (line 336–337 of `vetRepoConfig`):**
```go
case "extensions":
    if version != "1" {
        return fallbackEligible(fmt.Errorf("extensions.%s in a repositoryformatversion %s repository: git rejects it", ...))
    }
```
Any extension entry in a `repositoryformatversion = 0` repo causes an immediate fallback error, before `vetExtension` is even reached. Git 2.37+ relaxed this: `extensions.worktreeConfig = true` is explicitly permitted with v0 (it predates the v1 format, and git writes it that way by default). The fix must carve out `worktreeConfig` (and only `worktreeConfig`) as a v0-allowed extension before calling `vetExtension`.

**Bug B — `vetExtension` for `worktreeConfig` when truthy (line 379–382):**
```go
case "worktreeconfig":
    if !isTruthy(value) {
        return nil
    }
```
Only the falsy case (`worktreeConfig = false`) is allowed; the truthy case (`worktreeConfig = true`) falls through to the error. The intent is the reverse: `true` is the meaningful, supported value; `false` is a no-op. Fix: `return nil` unconditionally for `worktreeconfig` (both values are harmless — go-git doesn't split config by worktree at all, so the flag is irrelevant to in-process operation).

### Other extensions handled by `vetExtension`

| Extension | Current handling |
|---|---|
| `noop` | Always allowed |
| `objectformat` | Allowed only when value is `sha1`; anything else (e.g. `sha256`) triggers fallback |
| `worktreeconfig` | Allowed only when value is falsy — Bug B above |
| `partialClone` | Not in the switch; falls through to error unconditionally |
| `preciousObjects` | Not in the switch; falls through to error unconditionally |

`partialClone` and `preciousObjects` both need a case entry if repos with partial clones are expected. `partialClone = <remote>` means pack-object references may be missing locally — go-git operations on those objects would fail anyway, so triggering fallback is correct behavior. `preciousObjects = true` means no object deletion, which is irrelevant to read operations; it could be allowed. For this fix's scope, only `worktreeconfig` is in scope, but the case-sensitivity fix (lowercase name matching) will implicitly help `noop` and `objectformat` as well since the switch uses the already-lowercased `name`.

## 2. OpenRepo callers in `session/git/*.go` that bypass CLI fallback

`OpenRepo` calls go-git directly with no fallback mechanism. All of the following are exposed to the extension failure:

| File | Function | Line(s) |
|---|---|---|
| `ops.go` | `ResolveOriginBranchSHA` | 87 |
| `ops.go` | `ResolveLocalBranchSHA` | 104 |
| `ops.go` | Various SHA/commit resolution helpers | 275, 296, 352, 392, 434, 550, 742, 830, 948, 968 |
| `diff.go` | `Diff` | 109 |
| `branch_merge.go` | `MergeBranch` | 36 |
| `scaffolding.go` | worktree scaffolding | 53 |
| `worktree_git.go` | Multiple worktree ops | 259, 326, 362, 483 |
| `worktree_ops.go` | `CheckoutBranch`, related | 87, 151 |

The secondary-bug scope (requirements item 2) specifically names `ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` (ops.go:87 and :104). Both call `OpenRepo` and then call `.Reference(...)` — the same go-git method that `resolveRemoteWorktreeBaseCommit` reaches via `refs.ResolveRef(ctx, loc, ...)`. The backend path gets CLI fallback; the direct `OpenRepo` path does not.

The diff and merge callers (diff.go:109, branch_merge.go:36) are also at risk but are separate from SHA resolution.

## 3. `ResolveRef` API signature and use in `resolveRemoteWorktreeBaseCommit`

From `session/git/backend/backend.go`:
```go
ResolveRef(ctx context.Context, loc RepoLocation, ref RefName) (CommitSHA, error)
```

`resolveRemoteWorktreeBaseCommit` (`ops.go:194`) uses it exclusively through the `refs backend.Backend` parameter — never `OpenRepo` directly. It calls:
- `refs.ResolveRef(ctx, loc, backend.RefName("origin/"+candidate))` — remote branch tip
- `refs.ResolveRef(ctx, loc, backend.RefName("refs/heads/"+candidate))` — local branch fallback
- `refs.ResolveRef(ctx, loc, "HEAD")` — unborn-repo detection

This function is correctly on the abstraction layer and already has CLI fallback via `gitprovider.Backend()`. No fix is needed here for the secondary bug if `ResolveOriginBranchSHA`/`ResolveLocalBranchSHA` are ported to use `backend.Backend.ResolveRef` instead of `OpenRepo`.

## 4. `enrichment_test.go` coverage for `worktreeConfig = true`

File: `session/git/backend/gogit/enrichment_test.go`, line 51

`TestReadEnrichment_RejectsIncludesAndUnsupported` includes `"[extensions]\n worktreeConfig = true\n"` in its "must reject" table. This test asserts that `worktreeConfig = true` causes an error — it documents the **current buggy behavior**, not the desired behavior. After the fix, this test case must be:
- Removed from the "rejects" table, and
- A new passing case added to verify that `[extensions]\n worktreeConfig = true\n` is accepted (or at least does not block in-process operation).

## 5. Existing integration tests with linked worktrees

Several oracle test files create repos with real linked worktrees:

| File | Description |
|---|---|
| `symlink_oracle_test.go:42` | "seed makes a repository with two commits, a tag, a second branch and a linked worktree" — the core oracle fixture used by most linked-worktree tests |
| `fixtures_oracle_test.go:312` | `fixtureHeadNames` creates one linked worktree per branch-ambiguity case |
| `fixtures_oracle_test.go:501` | Tests that read delta-compressed objects from both main checkout and a linked worktree |
| `mutation_oracle_test.go:111` | "CurrentBranch ignores the linked worktree (main's HEAD)" |
| `mutation_oracle_test.go:143` | "GitDir returns the common dir in a linked worktree" |

None of these fixtures write `[extensions]\n worktreeConfig = true\n` to the base repo's config. Git only writes that extension when the user explicitly runs `git config extensions.worktreeConfig true` or uses a tool that does so (e.g., per-worktree `.git/config` split). The failing production case is a user-managed repo that has this set, not a stapler-created worktree.

A regression test for the fix should:
1. Use `symlink_oracle_test.go`'s `seed` helper (or a copy) to create a linked worktree.
2. Append `[extensions]\nworktreeConfig = true\n` to the base repo's `.git/config`.
3. Assert that `OpenRepo` (via `vetRepoConfig`) does NOT return an error for this config.
