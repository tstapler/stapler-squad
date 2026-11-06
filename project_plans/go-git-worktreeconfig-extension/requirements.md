# Requirements: go-git rejects `extensions.worktreeConfig`

Backlog item: 9c33e913-0aa0-4eeb-b6a0-ee33f15ad8a8

## Problem

Session creation fails with `failed to create git worktree: resolve default branch ... failed to open git repo at
<repo>: core.repositoryformatversion does not support extension: worktreeconfig`. The checkout has
`core.repositoryformatversion=0` and `extensions.worktreeconfig=true` (set by `git worktree` / sparse-checkout tooling),
which stock git accepts.

## Root cause (verified)

`repository_extensions.go` in `tstapler/go-git` `v5.19.3-ssq.1`:

- `extensions()` lowercases every key (`strings.ToLower(opt.Key)`), but `extensionsValidForV0` is keyed
  `partialClone` / `preciousObjects` / `worktreeConfig` (camelCase). The V0 lookup therefore never matches, so
  `worktreeconfig` (and also `partialclone`, `preciousobjects`) is reported as
  `ErrUnsupportedExtensionRepositoryFormatVersion`.
- `worktreeconfig` is absent from `builtinExtensions`, so even once the V0 gate passes (and for V1 repos) it ends in
  `ErrUnknownExtension`.

`ResolveOriginBranchSHA` / `ResolveLocalBranchSHA` (`session/git`) open the repo via go-git and so inherit the failure.

## Scope

1. Fix `verifyExtensions` in the go-git fork (lowercase V0 keys; accept `worktreeconfig` as supported), add fork tests,
   tag a new fork release, bump the `replace` in stapler-squad's `go.mod`.
2. Add stapler-squad integration tests for both resolvers with `worktreeConfig = true`.
3. Keep the in-process gogit backend's V0 guard (`enrichment_test.go`) behaving as before.

## Acceptance criteria

See the backlog item (11 criteria): OpenRepo succeeds for V0+V1 with worktreeConfig; `partialclone`/`preciousobjects`
still accepted for V0; both resolvers return a 40-hex SHA; fork tests; `ops_test.go` integration tests using `file://`
remotes; `TestReadEnrichment_RejectsIncludesAndUnsupported` unchanged; `make test`, `make lint` green; no
`worktreeconfig` errors in the log after deploy.
