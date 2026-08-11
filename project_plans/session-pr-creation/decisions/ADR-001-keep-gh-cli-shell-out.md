# ADR-001: Keep shelling out to `gh` CLI for PR creation (no `go-github`)

**Status**: Accepted
**Date**: 2026-08-06
**Project**: session-pr-creation

## Context

The new mechanical `CreatePullRequest` RPC needs to create a GitHub pull
request from a session's worktree. The existing `GitWorktree.CreatePR`
(`session/git/worktree_git.go:329`) already does this today for the backlog
automation path (`pushAndCreatePR`) by shelling out to `gh pr create` via
`safeexec.CommandContext`. `.claude/rules/prefer-go-git-over-subshells.md`
generally prefers native Go integrations over subshells, which raised the
question of whether this feature should instead adopt `google/go-github` (or
a similar typed API client) for the new RPC.

`research/build-vs-buy.md` (Agent 6) already investigated this question in
depth for this project; this ADR formalizes that conclusion as a decision
record per the plan-phase instruction, since it's a "keep the existing
approach" call that's easy for a future contributor to second-guess without
a recorded rationale.

## Decision

Keep shelling out to the `gh` CLI. Do not introduce `google/go-github` (or
any other GitHub API client library) as a new dependency. The new RPC calls
`GitWorktree.CreatePR` directly, exactly as `pushAndCreatePR` does.

## Rationale

1. **The subshell-avoidance rule's own exception applies directly.**
   `.claude/rules/prefer-go-git-over-subshells.md` explicitly carves out "any
   operation needing a credential helper for push/fetch against a real
   remote" as a case where a subshell is still fine. `gh` already owns the
   user's GitHub auth/credential flow end to end (`checkGHCLI()`,
   `session/git/util.go:46`, validates install + `gh auth status`).
   `go-github` takes a bare API token and would push the entire
   token-acquisition/storage/refresh burden onto this codebase — a new
   problem, not a solved one.
2. **The code this RPC calls is already built, tested, and in production use**
   by `pushAndCreatePR`. `CreatePR`/`findExistingPR` already handle PR-number
   parsing (with a documented past-bug fix, `worktree_git.go:365-389`),
   existing-PR reuse, and race retries. Migrating to `go-github` would mean
   rewriting all of this for a feature whose requirements explicitly put
   "any change to the backlog automation path itself" out of scope
   (requirements.md) — the new RPC is required to call `CreatePR` directly
   (AC3), which already forecloses a parallel `go-github`-based
   implementation.
3. **Two competing GitHub auth surfaces would be worse than one.** Backlog
   automation keeps using `gh` regardless of what this feature does (out of
   scope to change) — a partial `go-github` migration would leave the
   codebase authenticating to GitHub two different ways for the same overall
   product, for no functional gain visible in the requirements.
4. **Subprocess overhead is not a real cost here.** `CreatePullRequest` is
   invoked at most once per user click on a modal a human is actively
   waiting on — not a hot path where fork/exec overhead matters.

## Alternatives Considered

- **`google/go-github`**: typed responses, no subprocess, but requires a new
  auth surface and duplicate reimplementation of already-tested logic. Not
  recommended — see rationale above and `research/build-vs-buy.md` §1.
- **A third-party PR-creation SaaS/API**: not applicable — GitHub's own API
  (reached via `gh`) is already the target system there is nothing else to
  evaluate here (`research/build-vs-buy.md` §2).

## Consequences

- The new `CreatePullRequest` RPC inherits `CreatePR`'s existing
  fail-open/race-retry/PR-number-parsing behavior for free, with no new code
  to write or test for the GitHub-interaction layer itself.
- Any future decision to migrate PR creation off `gh` entirely (e.g. for a
  multi-tenant/hosted version of this product where per-request auth tokens
  become necessary) would need to revisit both this RPC and
  `pushAndCreatePR` together, since they'd otherwise diverge in how they
  authenticate to GitHub.
