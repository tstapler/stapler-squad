# ADR-001: Replace stderr-string-matching with ground-truth re-query in the worktree self-heal fallback

**Status**: Accepted
**Date**: 2026-08-22

## Context

`session/git/worktree_ops.go`'s self-heal fallback (added in `492b0d6df`) lets a caller
that loses a `git worktree add` race reuse the winner's result instead of hard-failing. It
decides "did I lose a legitimate race" by `strings.Contains`-matching the failing
subprocess's `err.Error()` against literal English strings: `"already exists"`
(`setupNewWorktree`, worktree_ops.go:336) and `"already checked out"` /
`"already used by worktree"` (`setupFromExistingBranch`, worktree_ops.go:136).

Two independent research passes (`research/pitfalls.md`, `research/features.md`) confirmed
this control mechanism has an open-ended failure class, not just the one flake observed on
PR #583:

1. A `runGitCommand` subprocess killed by its fixed 30s timeout under CI load returns
   `"signal: killed"` (a `*exec.ExitError`) — reproduced directly, not inferred — which
   matches neither check and falls through to a hard error. This is the most likely trigger
   for the observed flake.
2. Nothing in the call chain (`runGitCommand` → `LocalRunner.Run` → `safeexec.CommandContext`)
   sets `LC_ALL=C`/`LANG=C`, so a non-English git locale would produce translated fatal text
   that also matches neither check — a second, load-independent gap.
3. The in-repo doc comment at worktree_ops.go:133-135 already shows this pattern needed a
   second literal string once before (git 2.50.1's rewording) — direct evidence the approach
   is inherently version-fragile, not just theoretically so.

Per `.claude/rules/fix-flaky-tests-dont-defer.md`, a flake must be root-caused and fixed for
the *class*, not just the one observed string. Adding a third literal (`"signal: killed"`)
or widening `runGitCommand`'s timeout only narrows the odds of hitting an unrecognized
string again; neither closes the class, and AC-4 explicitly disallows both as the fix.

## Decision

Replace both `strings.Contains` gates with **ground-truth re-query**: on *any* failure of
the relevant `worktree add` call, re-verify the actual git state instead of inferring the
outcome from error text —

- `setupNewWorktree`: re-check `branchRefExists(repo, branchRef)` (go-git, in-process ref
  read, no subprocess) in a small bounded retry loop.
- `setupFromExistingBranch`: re-run the existing `worktree list --porcelain` +
  `findWorktreeForBranch` lookup unconditionally on any add failure, not just on a matched
  string.

If the re-query confirms the expected winner state, self-heal exactly as today. If it
doesn't (e.g. disk full, a genuine unrelated failure), return the original hard error — this
is *more* correct than the string match for that case too, which today accidentally
self-heals on some false-positive substrings (see `research/features.md` §2's
`.lock`-contention near-miss) and never verifies state at all.

This mirrors an idiom already used elsewhere in the same file/package
(`worktreeAlreadyRegisteredForBranch`, `findWorktreeForBranch`, and `util.go`'s
`getHeadCommitSHA` retry-with-backoff) — no new abstraction, just applying the same
state-verification pattern to this decision point.

## Consequences

- The self-heal decision no longer depends on git's error-message wording at all, closing
  the timeout, locale, and future-git-version gaps in one change.
- `LC_ALL=C`/`LANG=C` hardening (pitfalls.md's independently-found gap) becomes
  non-load-bearing for this fix, since no remaining logic in this bug's scope inspects error
  text. It is real, and tracked as a follow-up (see plan.md's Unresolved Questions), but
  implementing it requires touching `LocalRunner.Run` (session/tmux/command_runner.go),
  which is shared by every tmux and `gh` CLI call site — broader blast radius than this
  bug's scope, so it is deliberately not bundled into this fix.
- CI-scoped timeout widening (the `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS` precedent) is
  evaluated and rejected as unnecessary: ground-truth re-query is correct regardless of
  whether the underlying add call timed out, so widening the budget would only be a
  probability tweak with no remaining correctness gap to close.
