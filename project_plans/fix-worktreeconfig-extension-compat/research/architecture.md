# Architecture: fix-worktreeconfig-extension-compat

## 0. Prior art and scope

Built on `project_plans/go-git-worktree-and-merge/research/architecture.md` which covers the
overall `session/git/` structure, the `WithRepoWorktreeLock` concurrency model, the
`commandRunner`-dispatch seam idiom, and the `Isolate via seam` disposition for the
worktree/merge reimplementation project. This document focuses narrowly on the two fix layers
named in the requirements: (1) the go-git fork's `repository_extensions.go`, and (2)
`session/git/ops.go`'s `ResolveOriginBranchSHA`/`ResolveLocalBranchSHA`.

## 1. `vetExtension` in `session/git/backend/gogit/layout.go` — exact current behavior

`vetExtension` (`layout.go:371-385`) is called during `view()` construction, the in-process
path's gate before it touches any git objects. Its `worktreeconfig` case (lines 379-382):

```go
case "worktreeconfig":
    if !isTruthy(value) {
        return nil
    }
```

Behavior matrix:

| `extensions.worktreeConfig` value | `isTruthy` result | `vetExtension` returns |
|---|---|---|
| `false`, `no`, `off`, `0`, `""` (absent) | false | `nil` — in-process path allowed |
| `true`, `yes`, `on`, `1`, any other | true | `fallbackEligible(fmt.Errorf("repository extension \"worktreeconfig\" is not supported in-process", ...))` |

`fallbackEligible` wraps the error so the backend router (`session/git/backend/routed.go`) can
detect it as a reason to fall back to the CLI backend rather than surface it as a user-visible
error. The CLI backend (`session/git/backend/cli/`) has no analogous veto for `worktreeconfig`
— it passes every `git` call to the subprocess, which handles `worktreeconfig` natively.

**The backend layer is therefore already correct**: a repo with `extensions.worktreeConfig =
true` is routed to the CLI subprocess for every `ResolveRef`, `Fetch`, and `RefExists` call.
The bug lives entirely in callers that bypass the backend abstraction by calling `OpenRepo`
directly.

## 2. `ResolveOriginBranchSHA` vs `resolveRemoteWorktreeBaseCommit` — approach difference

Both functions resolve the tip SHA of a branch on origin. They differ structurally:

### `ResolveOriginBranchSHA` (`ops.go:83-96`) — bypasses backend

```go
func ResolveOriginBranchSHA(repoPath, mainBranch string) (string, error) {
    if err := FetchBranch(repoPath, mainBranch); err != nil { ... }
    repo, err := OpenRepo(repoPath)  // ← go-git PlainOpenWithOptions direct
    if err != nil { ... }
    ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", mainBranch), true)
    ...
    return ref.Hash().String(), nil
}
```

`FetchBranch` (ops.go:28-45) correctly uses `gitprovider.Backend().Fetch(...)` — it goes
through the backend abstraction and therefore falls back to the CLI on `worktreeconfig=true`
repos. `OpenRepo(repoPath)` on the next line calls `native.OpenRepo` → `git.PlainOpenWithOptions`
(the go-git fork) → `verifyExtensions` → **fails with
`ErrUnsupportedExtensionRepositoryFormatVersion: worktreeconfig`** when
`extensions.worktreeConfig = true`. The ref resolution never happens; the error propagates out
of `ResolveOriginBranchSHA`.

`ResolveLocalBranchSHA` (`ops.go:103-113`) has the identical structure — `OpenRepo` then
`repo.Reference(plumbing.NewBranchReferenceName(...))` — and fails for the same reason.

### `resolveRemoteWorktreeBaseCommit` (`ops.go:194-225`) — backend throughout

```go
func resolveRemoteWorktreeBaseCommit(ctx context.Context, refs, network backend.Backend, ...) (...) {
    loc := remoteLocation(runner, "", repoPath)
    for _, candidate := range CandidateDefaultBranches {
        fetchErr := network.Fetch(ctx, loc, ...)
        sha, revErr := refs.ResolveRef(ctx, loc, backend.RefName("origin/"+candidate))
        ...
    }
    ...
    if _, headErr := refs.ResolveRef(ctx, loc, "HEAD"); errors.Is(headErr, backend.ErrUnborn) { ... }
}
```

Every call goes through `backend.Backend` — the router's `vetExtension` check routes
`worktreeconfig=true` repos to the CLI subprocess. This function is immune to the bug.

**Root cause summary**: `ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` were written
before `resolveRemoteWorktreeBaseCommit` established the backend pattern for ref resolution.
They are the two non-ported survivors of the pre-backend era in `ops.go`. The fix is to port
them to the same `gitprovider.Backend().ResolveRef(ctx, loc, ...)` idiom already proven in the
remote counterpart.

## 3. `gitprovider.Backend()` — definition and return

`gitprovider.Backend()` (`session/gitprovider/gitprovider.go:42-50`) returns the process-wide
`backend.Backend` registered via `gitprovider.Register`. Implementation:

```go
var current atomic.Pointer[provider]  // set once at startup by session/gitwiring

func Backend() backend.Backend {
    if p := current.Load(); p != nil {
        return (*p)()
    }
    // unregistered: returns cli.New(nil) which rejects Local locations
    return unwired
}
```

In production, `session/gitwiring` calls `gitprovider.Register` during `server.BuildDependencies()`,
installing a `*backend.Router` that:
- For `Local` locations: tries the in-process gogit backend first (via `vetExtension`/`view()`),
  falls back to `cli.New(localRunner)` on a `fallbackEligible` error
- For `Remote` locations: uses `cli.New(remoteRunner)`

The returned `backend.Backend` is therefore **the router**, not a bare gogit or CLI backend
directly. `gitprovider.Backend()` is cheap to call per-operation (atomic load + function call);
the backing `*Router` is long-lived and already handles cohort routing, in-process/CLI
selection, and local-runner injection.

## 4. `backend.ResolveRef` return values for `origin/<branch>`

Both backends (`cli/refs.go:94-106` and `gogit/refs.go:79-102`) implement the same contract:

| Condition | Return |
|---|---|
| Branch `origin/<branch>` exists and resolves to a commit | `(CommitSHA("<40-hex>"), nil)` |
| Branch `origin/<branch>` does not exist | `("", backend.ErrObjectNotFound)` (or a wrapping error with `ErrObjectNotFound` as cause) |
| Repo itself is unborn (HEAD resolves but has no commit) | `("", backend.ErrUnborn)` — only for `"HEAD"` ref |
| Any other error (I/O, auth, timeout) | `("", <other error>)` |

CLI backend path: `git rev-parse --verify <ref>^{commit}` exits with code 128 for a missing
ref → `b.git()` maps exit-code-128 to `backend.ErrObjectNotFound`. The empty-string guard
in `resolveRemoteWorktreeBaseCommit` (`if sha != ""`) is belt-and-suspenders only — the
error return is the canonical signal.

The ported `ResolveOriginBranchSHA` should treat `("", err)` with `errors.Is(err,
backend.ErrObjectNotFound)` as "branch does not exist on origin" (a fetch failure, since
FetchBranch succeeded), and surface a descriptive error to the caller consistent with the
current function's existing "failed to resolve origin/%s" message.

## 5. Go-git fork bug in `repository_extensions.go` — exact failure path

The fork (`github.com/tstapler/go-git/v5@v5.19.3-ssq.1`,
`~/.../go-git/repository_extensions.go`) has two related defects:

**Defect A — case mismatch in `extensionsValidForV0`:**
```go
extensionsValidForV0 = map[string]struct{}{
    "noop":            {},
    "partialClone":    {},
    "preciousObjects": {},
    "worktreeConfig":  {},  // BUG: capital C; should be lowercase "worktreeconfig"
}
```

`extensions()` (line 72-74) lowercases every key with `strings.ToLower(opt.Key)`. So
`extensions.worktreeConfig = true` in git config produces the key `"worktreeconfig"`. Lookup
in `extensionsValidForV0` fails (the map contains `"worktreeConfig"`, not `"worktreeconfig"`),
so `"worktreeconfig"` lands in `unsupported` → `verifyExtensions` returns
`ErrUnsupportedExtensionRepositoryFormatVersion: worktreeconfig`. This fires for any repo
with `core.repositoryFormatVersion = 0` (or unset, the default) and `extensions.worktreeConfig
= true`.

**Defect B — `worktreeconfig` absent from `builtinExtensions`:**
```go
builtinExtensions = map[string]struct{}{
    "noop":    {},
    "noop-v1": {},
}
```

For repos with `core.repositoryFormatVersion = 1`, the V0 check is skipped, but the
`builtinExtensions` check still runs. `"worktreeconfig"` is not present → lands in `missing`
→ `ErrUnknownExtension: worktreeconfig`.

**Combined effect**: any repo with `extensions.worktreeConfig = true` causes
`git.PlainOpenWithOptions` (and therefore `OpenRepo`) to return an error, regardless of
`repositoryFormatVersion`. The fix requires both changes:
1. Add `"worktreeconfig": {}` to `extensionsValidForV0` (lowercase, matching the
   lowercased-key processing in `extensions()`)
2. Add `"worktreeconfig": {}` to `builtinExtensions` (so go-git treats the extension as
   understood and harmless — it only affects per-worktree config file reading, which go-git
   doesn't use anyway)

Note: `worktreeconfig = true` tells git to read per-worktree config from
`.git/worktrees/<name>/config.worktree` in addition to the common `.git/config`. go-git v5 has
no concept of linked worktrees at all, so treating `worktreeconfig` as a no-op builtin (like
`noop`) is the correct posture — it neither enables the feature nor errors on it.

## 6. Disposition for `session/git/ops.go` — Extend as-is (targeted port)

**Recommendation: Extend as-is — no seam, no refactor, two targeted function-body
replacements.**

Reasoning:

1. **This is a port, not a capability switch.** The prior-art document's
   "Isolate via seam" recommendation applies to the worktree/merge reimplementation project
   because two independent implementations are gated by a live-settable flag. Here there is
   no flag and no alternative implementation — the backend abstraction is the one and only
   correct path, and the existing `OpenRepo`-using code is simply unported. Introducing a
   seam for a 3-line body replacement is premature abstraction with no benefit.

2. **`ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` are leaf utilities** with no
   subtypes, no callers that need to inject an alternate ref-resolver, and no future-
   planning scenario named in requirements that would add one. Per the
   `interface-pollution-checklist` rule: "one production implementation, no future ones
   named" = internal call, not an interface.

3. **The pattern is already established two functions lower in the same file.**
   `resolveRemoteWorktreeBaseCommit` (ops.go:194-225) demonstrates the exact replacement
   shape: `loc := backend.Local{Root: backend.RepoRoot(repoPath)}` → `gitprovider.Backend().ResolveRef(ctx, loc, refName)`. The ported functions will be structurally identical.

4. **The existing mix of `OpenRepo` and `gitprovider.Backend()` calls in `ops.go`** is
   not a design choice to preserve — it's an artifact of incremental migration. These two
   functions are the remaining unmigrated stragglers. Porting them without adding a seam
   continues the established migration direction, confirmed by the 14 other `OpenRepo` calls
   still in ops.go which are candidates for future porting (not in scope here).

**Concrete change shape for `ResolveOriginBranchSHA`:**
- Keep `FetchBranch(repoPath, mainBranch)` as-is (already uses backend correctly)
- Replace `OpenRepo(repoPath)` + `repo.Reference(...)` with:
  ```go
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  loc := backend.Local{Root: backend.RepoRoot(repoPath)}
  sha, err := gitprovider.Backend().ResolveRef(ctx, loc, backend.RefName("origin/"+mainBranch))
  if err != nil {
      return "", fmt.Errorf("failed to resolve origin/%s: %w", mainBranch, err)
  }
  return string(sha), nil
  ```
  (Timeout value should match `FetchBranch`'s 30s or be tunable; the exact value is a
  planning decision, not an architecture constraint.)

**Concrete change shape for `ResolveLocalBranchSHA`:**
- Replace `OpenRepo(repoPath)` + `repo.Reference(plumbing.NewBranchReferenceName(...))` with:
  ```go
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  loc := backend.Local{Root: backend.RepoRoot(repoPath)}
  sha, err := gitprovider.Backend().ResolveRef(ctx, loc, backend.RefName("refs/heads/"+branchName))
  if err != nil {
      return "", fmt.Errorf("failed to resolve local branch %s: %w", branchName, err)
  }
  return string(sha), nil
  ```

## Summary for planning

Three distinct changes, each independently verifiable:

| Change | File | Root cause addressed |
|---|---|---|
| Add lowercase `"worktreeconfig"` to `extensionsValidForV0` | `go-git/repository_extensions.go` | V0 repos fail `verifyExtensions` due to case mismatch in map key |
| Add lowercase `"worktreeconfig"` to `builtinExtensions` | `go-git/repository_extensions.go` | V1 repos fail `verifyExtensions` as unknown extension |
| Port `ResolveOriginBranchSHA` to `gitprovider.Backend().ResolveRef` | `session/git/ops.go` | `OpenRepo` bypasses backend's `worktreeconfig` fallback gate |
| Port `ResolveLocalBranchSHA` to `gitprovider.Backend().ResolveRef` | `session/git/ops.go` | Same as above |

The go-git fork fix is necessary even after the ops.go port, because other call sites in the
codebase still use `OpenRepo` directly (14 others confirmed in ops.go alone, plus callers in
`session/git/native/`). The two fixes are complementary, not redundant: the fork fix
eliminates the OpenRepo failure at the library layer; the ops.go port eliminates the dependency
on OpenRepo at the two specific call sites named in requirements.
