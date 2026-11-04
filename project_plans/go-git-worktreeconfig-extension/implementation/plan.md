# Plan: accept extensions.worktreeConfig in the go-git fork

Complexity 1. Domain glossary, migration, observability, risk-control: N/A.

## Story 1 — Fork fix (repo tstapler/go-git, branch ssq/v5)
- T1.1 `repository_extensions.go`: lowercase `extensionsValidForV0` keys (partialclone, preciousobjects, worktreeconfig); add `"worktreeconfig": {}` to `builtinExtensions`. (AC1-3)
- T1.2 Tests: `TestVerifyExtensions` table (V0/V1 x worktreeconfig, partialclone, preciousobjects, unknown ext still rejected, V0 + unknown -> ErrUnsupportedExtensionRepositoryFormatVersion) and `TestOpenRepo_should_Succeed_When_V0RepoHasWorktreeConfigExtension`. (AC6)
- T1.3 Run `go test .` in the fork; push to `ssq/v5`, tag `v5.19.3-ssq.2`, push tag.

## Story 2 — stapler-squad consumption
- T2.1 Bump `replace` in go.mod to `v5.19.3-ssq.2`; `go mod tidy`; `go mod verify`.
- T2.2 `session/git/ops_test.go`: integration tests for `ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` on a repo with `core.repositoryformatversion=0` + `extensions.worktreeConfig=true`, local `file://` remote, assert 40-hex SHA. Verify they FAIL on ssq.1 (red) then pass on ssq.2. (AC4,5,7)
- T2.3 Confirm `TestReadEnrichment_RejectsIncludesAndUnsupported` unchanged and green (AC8).
- T2.4 `make test`, `make lint` (AC9,10).

## Story 3 — Deploy check
- T3.1 After merge+deploy, grep `~/.stapler-squad/logs/staplersquad.log` for "does not support extension: worktreeconfig" and confirm no lines newer than the restart (AC11; post-deploy, owner/ship phase).

## Risks
- go-git ignores `config.worktree` (per-worktree overrides); acceptable for ref/remote resolution.
- Bump go.mod only after tag is fetchable.

## Adversarial-review resolution (adversarial-review.md, verdict was BLOCKED)
- B1 (AC3): resolved by option (a): `partialclone` and `preciousobjects` are added to `builtinExtensions` with a caveat
  comment (promisor objects cannot be lazy-fetched; go-git never gc's). T1.1/T1.2 amended; fork tests expect success for
  V0 worktreeConfig, V1 worktreeConfig, V0 partialClone+preciousObjects; V0 + unsupported extension still rejected.
- C1: `project_plans/fix-worktreeconfig-extension-compat/` is a pre-existing untracked parallel plan (its "Bug 3" would
  route resolvers through the backend). Not adopted: the fork fix alone makes `OpenRepo` succeed (verified by red/green
  tests); left untouched for the owner to reconcile.
- M5: fork open test uses an on-disk repo and `PlainOpenWithOptions{DetectDotGit, EnableDotGitCommonDir}`.
- C4: `docs/how-to/roll-back-go-git-fork.md` warns that rollback reintroduces the bug.
- C3 (latent): go-git V1 config writes emit an empty `objectformat =`; production config writes go through the CLI. Not
  addressed here.
