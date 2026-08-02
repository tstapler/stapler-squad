# ADR-002: Accept `gh` CLI subshell for PR-ref verification; classify "not found" via stderr text match

**Status**: Accepted
**Date**: 2026-08-02

## Context

`report_duplicate` (FR3/FR4) must verify a `duplicate_ref` against GitHub
*before* mutating any state, and must split verification failures into two
channels: definitively-nonexistent (`ErrInvalidArgument`, no retry implied)
vs. transient (`ErrInternalError`, "retry" wording) — see FR4.

For issue and commit refs, `github.GetIssue` (`github/repos.go:270-346`) and
the new `github.GetCommit` (this project) are native HTTP calls against the
GitHub REST API and return a distinct, structured signal for "not found": an
HTTP 404 (`GetIssue` already returns the literal string `"GitHub API: issue
not found (404)"` — `github/repos.go:291-292`). That message is
`errors.New`/`fmt.Errorf`-wrapped, not a sentinel error, but the literal
`"(404)"` substring is a reliable, already-established classification signal
in this codebase (no existing caller does anything more sophisticated).

For PR refs, no native-HTTP "get PR by number" function exists in `github/`.
The two closest existing functions are unsuitable:
- `GetPRForBranch` (`github/client.go:378`) looks up a PR *by branch name*,
  not by number — wrong shape for verifying an arbitrary `duplicate_ref` PR
  number.
- `GetPRInfoCtx` (`github/client.go:247`) is what `report_pr_created`'s own
  verification path already uses (via `verifyPR` →
  `VerifyPRMatchesBranch`/`GetPRForBranch`, `server/mcp/tools_backlog.go:603-609`
  — note `report_pr_created` actually verifies via `GetPRForBranch`+`ErrNoPR`,
  not `GetPRInfoCtx` directly, but `GetPRInfoCtx` is the only function in the
  package that fetches a PR *by number*, which is what a `duplicate_ref` PR
  URL gives us). It shells out to `gh pr view <number> --repo <owner>/<repo>
  --json <fields>` and on failure returns
  `fmt.Errorf("failed to get PR info: %s", exitErr.Stderr)` — a raw stderr
  blob from the `gh` binary, not a typed/coded error.

Building a new HTTP-based "get PR by number" function (mirroring `GetIssue`)
was considered explicitly (see `research/build-vs-buy.md`) and would give a
clean 404-based split identical to the issue/commit case. It was *not* built
here, per `stack.md`'s unanimous cross-research-agent finding that
`github.GetCommit` is the only genuinely new GitHub-package code this feature
needs — adding a second new function purely to get a cleaner error shape for
one of three ref types is scope growth not requested by any acceptance
criterion.

## Decision

Use `GetPRInfoCtx` as-is for the PR case, and classify its error via a
stderr substring match against known "PR does not exist" phrasings from
`gh`'s GraphQL-backed `pr view` command (e.g. `"Could not resolve to a
PullRequest"`, `"no pull requests found"`). Everything else (network
failure, `gh auth` failure, rate limiting, an unrecognized stderr shape) is
treated as the transient/`ErrInternalError` channel — i.e., the classifier
fails *safe* toward "retry", never toward silently treating an ambiguous
error as "definitively invalid."

**This substring list is UNVERIFIED against a real `gh pr view <nonexistent>`
invocation as of this planning phase** (Task 1.3.2a in `implementation/plan.md`
requires the implementer to actually run `gh pr view 999999999 --repo
<any-real-repo>` against a real nonexistent PR number, capture the exact
stderr text, and adjust the match list to what `gh` actually emits — matching
this repo's own evidence discipline: reading `GetPRInfoCtx`'s code is a
hypothesis about `gh`'s error text, not a verified fact).

## Consequences

- **Risk accepted**: this is a fragile, string-matching heuristic over a
  third-party CLI's stderr output, which can change across `gh` versions
  without a compiler or type error to catch it. If `gh` changes its wording,
  the classifier silently degrades toward "transient" (fails safe — a
  genuinely-nonexistent PR would incorrectly get `ErrInternalError` with
  "retry" wording instead of `ErrInvalidArgument`, which is a confusing but
  non-destructive failure mode: the caller retries, the transient-looking
  error persists, and a human eventually notices via the stuck-item detector,
  not silent data corruption).
- If this pattern needs to be reused a third time (e.g., a future
  GitHub-verification tool), promote it to a real HTTP-based "get PR by
  number" function at that point — two ad-hoc uses of a CLI-stderr heuristic
  is tolerable technical debt; three is a smell worth fixing.
- `report_pr_created`'s existing verification path is unaffected — it
  continues using `GetPRForBranch`/`ErrNoPR`, a typed sentinel, for its own
  (branch-based, not number-based) check.

## Alternatives considered

- **Build `github.GetPR(ctx, owner, repo, number)` as a new HTTP function**:
  rejected as unrequested scope growth beyond `GetCommit` (see above); may be
  revisited if a third caller needing PR-by-number verification appears.
- **Treat all `GetPRInfoCtx` errors as `ErrInternalError`** (no attempt at a
  two-channel split for the PR case): rejected — FR4 requires the split for
  `report_duplicate` broadly, and a PR-shaped `duplicate_ref` is very likely
  the *most common* case given the origin incident (superseded by an already
  merged PR #272); silently downgrading it to "always retry" would mean a
  genuinely-wrong PR number never gets a clear "this doesn't exist" signal.
