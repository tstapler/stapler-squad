# Stack Research: worktreeConfig Extension Compat Bugfix

## 1. Fork Location

**`go.mod`** (line 232):
```
replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.1
```

**Source on disk**:
`/home/tstapler/go/pkg/mod/github.com/tstapler/go-git/v5@v5.19.3-ssq.1/repository_extensions.go`

The `replace` directive swaps the canonical `go-git/go-git/v5 v5.19.3` import for the fork at every import site in the module.

---

## 2. `extensionsValidForV0` and `verifyExtensions`

**File**: `repository_extensions.go` in the fork (same path as above).

```go
extensionsValidForV0 = map[string]struct{}{
    "noop":            {},
    "partialClone":    {},
    "preciousObjects": {},
    "worktreeConfig":  {},   // camelCase key
}
```

**Bug — `extensions()` lowercases first (line 73)**:
```go
out = append(out, extension{name: strings.ToLower(opt.Key), value: strings.ToLower(opt.Value)})
```

So `extensions.worktreeConfig` from `.git/config` becomes `ext.name = "worktreeconfig"`.

**`verifyExtensions()` then looks it up (line 95)**:
```go
if _, ok := extensionsValidForV0[ext.name]; !ok {
    unsupported = append(unsupported, ext.name)
}
```

`extensionsValidForV0["worktreeconfig"]` → not found (key is `"worktreeConfig"`).
Result: `ErrUnsupportedExtensionRepositoryFormatVersion: worktreeconfig`.

This fires for any repo where git set `extensions.worktreeConfig = true` automatically — i.e., any repo that has ever had a linked worktree, which includes most active stapler-squad sessions.

The check is reached via the `repositoryFormatVersion == ""` or `Version_0` branch (lines 91-103). Git sets `core.repositoryformatversion = 0` by default; `extensions.worktreeConfig` is explicitly allowed at V0 by the git spec, but the lookup fails due to the case mismatch.

---

## 3. `builtinExtensions`

```go
builtinExtensions = map[string]struct{}{
    "noop":    {},
    "noop-v1": {},
}
```

Only two entries — neither `"worktreeconfig"` nor `"worktreeConfig"`. This matters for the `repositoryFormatVersion = 1` (Version_1) path: after the V0 guard passes, the second gate (lines 107-116) checks every extension against `builtinExtensions`, and `"worktreeconfig"` would fail there too with `ErrUnknownExtension`. So the fix needs to touch both maps (or the normalization step).

---

## 4. `ResolveOriginBranchSHA` vs `resolveRemoteWorktreeBaseCommit`

### Broken path — `ResolveOriginBranchSHA` (`session/git/ops.go:83`)

```go
func ResolveOriginBranchSHA(repoPath, mainBranch string) (string, error) {
    if err := FetchBranch(repoPath, mainBranch); err != nil { ... }
    repo, err := OpenRepo(repoPath)   // ← triggers go-git Open() → verifyExtensions()
    ...
}
```

`OpenRepo` calls go-git's `Open(storage, worktree)`, which calls `verifyExtensions` on the repo's config — and fails for repos with `extensions.worktreeConfig`.

`ResolveLocalBranchSHA` (`ops.go:103`) has the same problem — it also calls `OpenRepo`.

Both are called by: `ResolveExplicitBranchSHA`, `CreateBacklogWorktree`, and other callers that need a SHA before creating a worktree.

### Working path — `resolveRemoteWorktreeBaseCommit` (`session/git/ops.go:194`)

```go
func resolveRemoteWorktreeBaseCommit(...) (defaultBranch, baseSHA string, err error) {
    loc := remoteLocation(runner, "", repoPath)
    for _, candidate := range CandidateDefaultBranches {
        fetchErr := network.Fetch(ctx, loc, backend.FetchRequest{...})
        sha, revErr := refs.ResolveRef(ctx, loc, backend.RefName("origin/"+candidate))
        ...
    }
}
```

This uses `gitprovider.Backend()` — the native git CLI backend (`session/git/backend/`) that shells out to `git fetch` / `git rev-parse`. It never calls go-git's `Open()`, so `verifyExtensions` is never invoked. The backend pattern is immune to this bug by construction.

---

## 5. Tests for `verifyExtensions`

**File**: `repository_extensions_test.go` in the fork (same dir as source).

Five test cases exist in `TestVerifyExtensions`:

| Test name | Covers |
|---|---|
| `repositoryformatversion=0: invalid extension` | unknown ext rejected |
| `repositoryformatversion=0: allows supported noop` | `"noop"` passes V0 |
| `repositoryformatversion='': allows supported noop` | empty version + noop |
| `repositoryformatversion=1: rejects unknown extensions` | unknown ext rejected on V1 |
| `repositoryformatversion=1: allows known extension` | `"NOOP"` and `"noop-v1"` pass V1 |

**Gap**: No test for `worktreeConfig` (or any other camelCase key) on V0/empty — the exact case that is broken. The `"NOOP"` test on V1 inadvertently tests that uppercase survives the `strings.ToLower` round-trip (since `builtinExtensions` has `"noop"` as lowercase), but no camelCase V0 key is exercised.

---

## Fix surface

Three options (in order of scope):

1. **Normalize `extensionsValidForV0` keys to lowercase** — change all keys to `"worktreeconfig"`, `"partialclone"`, `"preciousObjects"` → `"preciousobjects"`. Matching the lowercase that `extensions()` already produces. Minimal; also need to add `"worktreeconfig"` to `builtinExtensions` for V1 path.

2. **Normalize in `verifyExtensions` at lookup time** — keep the maps as-is, do `strings.ToLower(key)` before map lookup. Same effect, more defensive.

3. **Port `ResolveOriginBranchSHA` / `ResolveLocalBranchSHA` to the backend abstraction** — bypass go-git `Open()` entirely, like `resolveRemoteWorktreeBaseCommit` does. Addresses root cause 3 from the requirements but leaves the fork bug unfixed for other callers.

Option 1 is the smallest fork diff and is the fix the requirements describe.
