# Requirements: fix-worktreeconfig-extension-compat

**Date**: 2026-11-03
**Type**: bugfix
**Complexity**: 2 — focused/contained cross-module fix

## Problem Statement

Session creation fails with the following error chain when the target repo has
`extensions.worktreeConfig = true` in its `.git/config`:

```
[session pipeline] async start failed err=failed to create git worktree: resolve default branch:
resolve default branch (origin fetch failed: no candidate default branch ([main master develop trunk])
exists on origin: failed to open git repo at /home/tstapler/Programming/stapler-squad:
core.repositoryformatversion does not support extension: worktreeconfig
...
```

The main stapler-squad repo itself (`/home/tstapler/Programming/stapler-squad`) has:
- `core.repositoryformatversion = 0`
- `extensions.worktreeConfig = true`

because git automatically sets `worktreeConfig = true` when linked worktrees are created
(which stapler-squad does constantly for sessions). This means **every session using this
repo as its base will fail to start** once the repo has accumulated any linked worktrees.

## Root Cause (confirmed)

**Bug 1 — go-git fork case sensitivity** (`repository_extensions.go` in
`github.com/tstapler/go-git/v5@v5.19.3-ssq.1`):

```go
extensionsValidForV0 = map[string]struct{}{
    "worktreeConfig":  {},   // camelCase key
    "partialClone":    {},
    "preciousObjects": {},
    ...
}
```

`verifyExtensions` lowercases extension names via `strings.ToLower(opt.Key)` before
the map lookup. `"worktreeConfig"` → `"worktreeconfig"`. The map has `"worktreeConfig"`,
not `"worktreeconfig"`. Go map lookups are case-sensitive. So the lookup returns `ok=false`,
the extension is classified as "unsupported for v0", and go-git returns
`ErrUnsupportedExtensionRepositoryFormatVersion: "core.repositoryformatversion does not
support extension: worktreeconfig"`.

**Bug 2 — `builtinExtensions` missing `worktreeconfig`** (same file):

Even after fixing Bug 1, `verifyExtensions` has a second gate: every extension must be
in `builtinExtensions` (currently only `"noop"` and `"noop-v1"`). `"worktreeconfig"`
is not there, so it would fail with `ErrUnknownExtension`. Adding it tells go-git
"I know about this extension; I won't honor per-worktree config splitting, but I won't
crash either."

**Bug 3 — `ResolveOriginBranchSHA` / `ResolveLocalBranchSHA` bypass the backend**
(`session/git/ops.go`):

Both functions call `OpenRepo(repoPath)` (direct go-git call) after `FetchBranch`
(which correctly routes through the backend abstraction with CLI fallback). The backend's
`vetExtension` case in `session/git/backend/gogit/layout.go` already triggers
`fallbackEligible` for `worktreeconfig = true`, but that fallback is never reached
because `OpenRepo` calls go-git directly. The correct pattern — already in use by
`resolveRemoteWorktreeBaseCommit` — is to use `refs.ResolveRef(ctx, loc, ...)` from the
backend rather than `OpenRepo`.

## Users / Consumers

- Every stapler-squad session whose base repo has `extensions.worktreeConfig = true`
  (which is **all sessions** once the first linked worktree is created on the base repo).
- Backlog automation (`ResolveDefaultBranchSHA` is called from worktree creation on
  every backlog work session).
- Manual sessions started from the UI.

## Success Metrics

- Repos with `repositoryformatversion = 0` + `extensions.worktreeConfig = true` can be
  opened by go-git without error (Bug 1 + 2 fixed).
- `ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` succeed for repos that have
  `extensions.worktreeConfig = true` (Bug 3 fixed).
- `make test` passes for `session/git/` package with a new regression test using a
  repo with `worktreeConfig = true`.
- No new session creation failures in logs after deploy.

## Appetite

Small (1–2 days): two targeted code changes — one in the go-git fork, one in ops.go — plus tests and go.mod bump.

## Constraints

- The go-git fork fix must keep `worktreeConfig` in `extensionsValidForV0` (for semantic correctness — git docs say it's valid at v0); it just needs to use lowercase keys.
- go-git won't honor per-worktree config splitting after the fix (sections in
  `.git/worktrees/<name>/config` won't be read). This is acceptable: the failing operations
  (resolve default branch, fetch, ref resolution) don't depend on per-worktree config.
- The stapler-squad `vetExtension` in `session/git/backend/gogit/layout.go` correctly
  returns `fallbackEligible` for `worktreeconfig = true`. **Do not change it**: the in-process
  go-git backend deliberately falls back to the CLI for repos with this extension, because
  go-git can't correctly handle per-worktree config. The go-git fork fix is a different layer
  — it fixes the crash in the direct-go-git code paths that `OpenRepo` exposes.
- No new cgo dependencies.

## Non-Goals

- Full go-git support for per-worktree config (`.git/worktrees/<name>/config` reading).
  That would require significant go-git changes beyond the scope of this fix.
- Changing the backend's `vetExtension` behavior (it correctly falls back to CLI for
  the in-process backend).

## Root cause (confirmed 2026-11-04, backlog item 9c33e913)

- `.git/config:1653-1654` sets `[extensions] worktreeConfig = true` with `core.repositoryformatversion = 0`.
- Stock `github.com/go-git/go-git/v5 v5.19.3` `PlainOpenWithOptions` rejects that: `core.repositoryformatversion does not support extension: worktreeconfig`.
- Repro: scratch module opening this repo — stock v5.19.3 returns the error; the `tstapler/go-git` fork `v5.19.3-ssq.3` (current `replace` in go.mod) returns nil.
- Remaining open items: AC 11 (no such log lines after deploy) and the last review's FAIL (ops.go `ResolveOriginBranchSHA`/`ResolveLocalBranchSHA` still call `OpenRepo`; works only via the fork fix).
