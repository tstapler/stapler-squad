# Validation Plan: fix-worktreeconfig-extension-compat

**Date**: 2026-11-04

## Happy Path Scenario

Given a repo with `core.repositoryformatversion = 0` and `extensions.worktreeConfig = true` in its `.git/config` (the state stapler-squad accumulates automatically once any linked worktree is created), when `ResolveOriginBranchSHA` is called during session creation, then it returns a non-empty 40-hex SHA string with nil error and no `"core.repositoryformatversion does not support extension: worktreeconfig"` log line.

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: `extensionsValidForV0` lowercase keys (Bug 1) | `repository_extensions_test.go` (fork) | `TestVerifyExtensions/repositoryformatversion=0:_allows_worktreeConfig=true` | Unit | Happy — V0 repo + `worktreeConfig=true` → no error from `verifyExtensions` |
| REQ-1: `extensionsValidForV0` lowercase keys (Bug 1) | `repository_extensions_test.go` (fork) | `TestVerifyExtensions/repositoryformatversion=0:_rejects_unknown_extension` | Unit | Error — V0 repo + unknown extension name → `ErrUnsupportedExtensionRepositoryFormatVersion` |
| REQ-1: `extensionsValidForV0` lowercase keys (Bug 1) | `repository_extensions_test.go` (fork) | `TestVerifyExtensions/repositoryformatversion=0:_allows_partialClone` | Unit | Happy — implicit regression guard: `partialClone` (also formerly camelCase) accepted at V0 |
| REQ-1: `extensionsValidForV0` lowercase keys (Bug 1) | `repository_extensions_test.go` (fork) | `TestVerifyExtensions/repositoryformatversion=0:_allows_preciousObjects` | Unit | Happy — implicit regression guard: `preciousObjects` (also formerly camelCase) accepted at V0 |
| REQ-2: `builtinExtensions` includes `"worktreeconfig"` (Bug 2) | `repository_extensions_test.go` (fork) | `TestVerifyExtensions/repositoryformatversion=1:_allows_worktreeConfig=true` | Unit | Happy — V1 repo + `worktreeConfig=true` → no `ErrUnknownExtension` |
| REQ-2: `builtinExtensions` includes `"worktreeconfig"` (Bug 2) | `repository_extensions_test.go` (fork) | `TestVerifyExtensions/repositoryformatversion=1:_rejects_unrecognized_builtin` | Unit | Error — V1 repo + extension name not in `builtinExtensions` → `ErrUnknownExtension` |
| REQ-1 + REQ-2: `OpenRepo` succeeds end-to-end | `repository_extensions_test.go` (fork) | `TestOpenRepo_should_Succeed_When_V0RepoHasWorktreeConfigExtension` | Integration | V0 on-disk repo with `extensions.worktreeConfig = true` → `git.PlainOpenWithOptions` returns non-nil `*Repository`, nil error |
| REQ-3: `ResolveOriginBranchSHA` uses `backend.ResolveRef` (Bug 3) | `session/git/ops_test.go` | `TestResolveOriginBranchSHA_should_ReturnSHA_When_WorktreeConfigExtensionPresent` | Unit | Happy — repo has `extensions.worktreeConfig=true`, origin/main exists → non-empty SHA, nil error |
| REQ-3: `ResolveOriginBranchSHA` uses `backend.ResolveRef` (Bug 3) | `session/git/ops_test.go` | `TestResolveOriginBranchSHA_should_ReturnErrRefNotFound_When_BranchMissingOnOrigin` | Unit | Error — setup per adversarial-review-phase2.md: create branch on origin after cloning, `git config --unset-all remote.origin.fetch` on the clone so fetch succeeds but no `refs/remotes/origin/<b>` exists → error wrapping `backend.ErrRefNotFound` (a plain missing branch fails at fetch, not ResolveRef) |
| REQ-3: `ResolveOriginBranchSHA` uses `backend.ResolveRef` (Bug 3) | `session/git/ops_test.go` | `TestResolveOriginBranchSHA_WorktreeConfigExtension` | Integration | Local `file://` bare remote; clone has `[extensions]\nworktreeConfig = true`; call returns fetched tip SHA, no network |
| REQ-4: `ResolveLocalBranchSHA` uses `backend.ResolveRef` (Bug 3) | `session/git/ops_test.go` | `TestResolveLocalBranchSHA_should_ReturnSHA_When_WorktreeConfigExtensionPresent` | Unit | Happy — repo has `extensions.worktreeConfig=true`, refs/heads/main exists → non-empty SHA, nil error |
| REQ-4: `ResolveLocalBranchSHA` uses `backend.ResolveRef` (Bug 3) | `session/git/ops_test.go` | `TestResolveLocalBranchSHA_should_ReturnError_When_BranchDoesNotExist` | Unit | Error — `refs/heads/nonexistent` absent → non-nil error containing `"nonexistent"` and `errors.Is(err, backend.ErrRefNotFound)` |
| REQ-4: `ResolveLocalBranchSHA` uses `backend.ResolveRef` (Bug 3) | `session/git/ops_test.go` | `TestResolveLocalBranchSHA_WorktreeConfigExtension` | Integration | On-disk repo, `[extensions]\nworktreeConfig = true` appended to `.git/config`, local branch present; call returns SHA |
| REQ-5: Existing in-process backend behavior preserved | `session/git/backend/gogit/enrichment_test.go` | `TestReadEnrichment_RejectsIncludesAndUnsupported` (existing — verify only, no change) | Unit | Confirm: after go-git fork bump, `ReadEnrichment` for V0 + `worktreeConfig=true` still returns non-nil error (V0 guard in `vetRepoConfig` fires before `vetExtension`; in-process backend does not reach CLI path) |

---

## UX Acceptance Tests

N/A — pure infrastructure fix

---

## Test Stack

- **Unit**: Go standard `testing` package; run via `gotestsum` (`make test`) for readable output
- **Integration**: Same `testing` package; test helpers use `os/exec` to `git init --bare` / `git clone` / `git push` local disk repos via `file://` remotes — no network required, deterministic
- **E2E / UX**: N/A

---

## Coverage Targets

| Package | New Tests | Test Types | Requirement Coverage |
|---|---|---|---|
| `github.com/tstapler/go-git` (`repository_extensions_test.go`) | 6 new cases added to `TestVerifyExtensions` + 1 new `TestOpenRepo_*` | 6 unit + 1 integration | REQ-1 (4 cases), REQ-2 (2 cases + integration) |
| `session/git` (`ops_test.go`) | 4 new test functions | 4 unit + 2 integration | REQ-3 (3 tests), REQ-4 (3 tests) |
| `session/git/backend/gogit` (`enrichment_test.go`) | 0 (existing test, verify-only) | 1 unit (existing) | REQ-5 |

**Total new test cases**: 13 (6 unit in fork + 1 integration in fork + 4 unit in stapler-squad + 2 integration in stapler-squad)

**Requirements coverage**: 5 / 5 (100%)
- REQ-1 (`extensionsValidForV0` lowercase): 4 unit cases
- REQ-2 (`builtinExtensions` add `"worktreeconfig"`): 2 unit + 1 integration
- REQ-3 (`ResolveOriginBranchSHA` backend port): 2 unit + 1 integration
- REQ-4 (`ResolveLocalBranchSHA` backend port): 2 unit + 1 integration
- REQ-5 (enrichment test preserved): 1 existing test verified unchanged

---

## Key Test Design Notes

**`enrichment_test.go:51` is a verify-only gate, not a new test.** `ReadEnrichment` for a V0 repo with `worktreeConfig=true` still returns an error after the fork fix because `vetRepoConfig`'s V0 guard (line 336 of `layout.go`) fires *before* `vetExtension` — any extension in a V0 repo causes an in-process backend error. The go-git fork fix operates at the `git.PlainOpen` layer; the in-process backend intentionally routes such repos to CLI fallback. Run `go test ./session/git/backend/gogit/ -run TestReadEnrichment_RejectsIncludesAndUnsupported -v` and confirm PASS after the go.mod bump (Story 1.1.5).

**Integration tests use `file://` remotes.** The two ops.go integration tests (`TestResolveOriginBranchSHA_WorktreeConfigExtension`, `TestResolveLocalBranchSHA_WorktreeConfigExtension`) must not make real network calls. Each test: (1) `git init --bare` a temp dir as the origin; (2) clone it; (3) add a commit and push; (4) manually append `[extensions]\nworktreeConfig = true\n` to the clone's `.git/config`; (5) call the target function. This mirrors the production state exactly and runs hermetically in CI.

**`backend.ErrRefNotFound`, not `ErrObjectNotFound`.** The error path test for `ResolveOriginBranchSHA` must assert the error wraps `backend.ErrRefNotFound`. `ErrObjectNotFound` is for a missing ODB object (blob/tree/commit), not a missing ref name. Using the wrong sentinel would make the error test pass for the wrong reason.

## Phase 2 addendum (2026-11-04)

- Pre-mortem P1 #1: add one pipeline-level test — `CreateBacklogWorktree` (or the nearest hermetic entry point) on a `file://` repo with `extensions.worktreeConfig=true` returns a worktree without any `worktreeconfig` error. Pre-fix evidence: with the `replace` removed, the existing worktreeConfig tests at `ops_test.go:366`/`:377` fail on pre-port code (adversarial-review-phase2.md).
- Gate for committing the `go.mod`/`go.sum` ssq.3 bump: `make test` and `make lint` exit 0 against it.
- AC 11 (no `worktreeconfig` log lines after deploy) is verified post-deploy via `grep -c 'does not support extension: worktreeconfig' ~/.stapler-squad/logs/staplersquad.log` on lines newer than the deploy time.
