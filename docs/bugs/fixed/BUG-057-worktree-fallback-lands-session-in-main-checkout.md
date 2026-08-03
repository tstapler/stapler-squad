# Bug: Failed worktree creation silently spawns a backlog session directly in the main checkout

**Status**: Fixed
**Priority**: High
**Fixed in**: main (2026-08-03)

## Symptoms

A batch of code (a session-resume fix + an SDD planning doc) was found committed
directly against `main` in the primary repo checkout, rather than in an isolated
worktree the way backlog work sessions are supposed to operate. No worktree/branch
existed for that work.

## Root Cause

`resolveSessionPath` (`server/services/backlog_service_triage.go:1220`) is called by
every backlog work-session spawn (`SpawnSessionFromItem`, `backlog_service_triage.go:756`).
It first tries `session.CreateBacklogWorktree(repoPath, slug)`
(`session/instance_worktree.go:103`). If that fails **for any reason** — disk quota,
a detached HEAD, a locked ref, anything past the "is this even a git repo" check —
it logged a warning and fell back to `session.ResolveSessionPath(repoPath)`
(`session/instance.go:549`), which does nothing but tilde-expand and `filepath.Abs()`
the path. It does not scope the result into a subdirectory or append the slug. So on
worktree-creation failure against a git-managed repo, the fallback path was
`repoPath` **verbatim** — i.e. the live main checkout — and `CreateDirectorySession`
spawned the session pointed directly at it.

The comment above the call site named the intended case as "the repo is not
git-managed", but the code never actually distinguished that case from "is
git-managed but worktree creation broke" — both took the identical fallback.

No log evidence ties this specific incident to backlog automation hitting this
exact path (see the investigation in the parent conversation) — the more likely
proximate cause here was a manual/interactive session working directly in the main
checkout. But the fallback bug is real, independent of that, and is exactly the
failure shape that would reproduce the same result the next time a worktree
creation genuinely fails.

## Fix

**`server/services/backlog_service_triage.go`** (`resolveSessionPath`):
- After a worktree-creation failure, resolve the path and check
  `git.IsGitRepo(resolvedRepo)` before deciding how to fall back.
- If the repo **is** git-managed, fail loudly: return a `connect.CodeInternal` error
  instead of falling back to a directory session at `repoPath`.
- Only fall back to a plain directory session (the original behavior) when the repo
  is genuinely not git-managed at all.

## Regression Tests

`server/services/backlog_service_triage_test.go`:
- `TestResolveSessionPath_should_ErrorNotFallBackToRepoPath_When_GitManagedWorktreeCreationFails` —
  forces `CreateBacklogWorktree`'s directory creation to fail (via
  `STAPLER_SQUAD_TEST_DIR` pointed at a location where `worktrees` already exists as
  a regular file, not a directory) against a real git-managed repo, and asserts
  `resolveSessionPath` returns an error rather than a path.
- `TestResolveSessionPath_should_FallBackToDirectory_When_RepoIsNotGitManaged` —
  verifies the legitimate fallback (a plain, never-git-initialized directory) still
  works.

Confirmed the first test fails against the pre-fix code (`An error is expected but
got nil`) and passes after the fix.

## Phase D — Reflect

**Classification**: Framework Pattern Misuse — a fallback/degrade-gracefully branch
that was written broadly ("worktree creation failed, for any reason") when the
actual safe scope was narrow ("worktree creation failed because the repo isn't
git-managed"). The two failure populations were conflated into one catch-all.

**Earliest enforcement point**: A unit test on `resolveSessionPath` is close to the
earliest achievable level here — the failure only manifests as a behavioral
difference in the return value, not a type-level distinction the compiler could
catch. No lint/static-analysis rule would generically flag "this fallback branch
is too broad." The regression test added in Phase B is the right level.

**Recurring shape**: This matches a pattern this codebase has hit before —
`s.tombstoneOrphanWorkSessions` and `AutoReopenForPRFix` comments elsewhere in this
same file reference prior incidents where a broad fallback/retry masked a narrower
failure. Worth naming explicitly for future audits: **"a degrade-gracefully branch
silently covers a failure mode it was never meant to handle."** Any future
`fallback to X if Y fails` pattern in this codebase should ask "does X make sense
for *every* reason Y can fail, or only a subset?" before shipping.
