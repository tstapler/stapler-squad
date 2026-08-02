# ADR-005: GitHub Ref Verification — Single Dispatcher, HTTP-Only Auth, Typed Status Sentinels

## Status
Proposed — **CONFLICTS with a concurrently-written `ADR-002-gh-cli-pr-existence-classification.md`
in this same directory.** A parallel planning session (this item is self-referential — see
features.md §9.3 — and appears to have a second concurrent session also planning it) reached
the opposite conclusion for the PR-verification sub-decision: that ADR keeps the existing
`gh`-CLI-subshell `GetPRInfoCtx` + stderr-substring classification for the PR case, rejecting
a new HTTP-based `GetPR` as unrequested scope growth. This ADR instead adds `GetPR` (HTTP-based)
specifically to keep all three ref types on one consistent auth mechanism. **Both cannot be
implemented as written — this must be reconciled by whoever picks up Phase 5 implementation,
before Epic 1.2/3.2 of `implementation/plan.md` starts.** See the "Conflicting Decision" note
at the bottom of this ADR.

## Context

`report_duplicate` must verify a `duplicate_ref` (PR, issue, or commit URL) exists on
GitHub before mutating any backlog state (FR3), and must distinguish a definitive
"doesn't exist" (`ErrInvalidArgument`, no retry) from a transient failure (`ErrInternalError`,
retry) per FR4.

Three separate problems surfaced during research, all resolved by this one ADR because they
share the same code surface:

1. **Dispatch shape.** Three ref kinds (PR/issue/commit) need three different GitHub API
   calls. What's the seam?
2. **Auth mechanism inconsistency.** The existing `github/` package has two calling
   conventions: `GetPRInfoCtx` shells out to `gh pr view` (auth via `gh auth login` /
   `CheckGHAuth()`), while `GetIssue` uses native `net/http` (auth via `GITHUB_TOKEN`/
   `GH_TOKEN` env or keychain, `getGHToken`). These are two independent auth-resolution
   paths. A host with one configured but not the other would pass verification for some
   ref kinds and fail for others with an unrelated-looking error — pitfalls.md §4 flags
   this as the single most likely inconsistency bug if `report_duplicate` copy-pastes
   whichever existing helper looks closest per ref type.
3. **Error classification.** `GetIssue` already discriminates 401/404/403(rate-limit)/429
   in its status-code branches but returns plain `fmt.Errorf` text, not `errors.Is`-checkable
   sentinels. No commit-existence or (HTTP-based) PR-existence call exists at all.

## Decision

**1. Dispatcher shape — Option A: single function, internal switch.**

```go
func (h *backlogHandlers) verifyGitHubRefExists(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
    switch ref.Type {
    case githubpkg.RefTypePR:
        _, err := githubpkg.GetPR(ctx, ref.Owner, ref.Repo, ref.PRNumber)
        return err
    case githubpkg.RefTypeIssue:
        _, err := githubpkg.GetIssue(ctx, ref.Owner, ref.Repo, ref.IssueNumber)
        return err
    case githubpkg.RefTypeCommit:
        _, err := githubpkg.GetCommit(ctx, ref.Owner, ref.Repo, ref.CommitSHA)
        return err
    default:
        return fmt.Errorf("unsupported ref type %s", ref.Type)
    }
}
```

Rejected: a `GitHubRefVerifier` interface, dependency-injected the way `h.verifyPRMatchesBranch`
is. Three closed, fixed ref kinds with a compiler-checked switch don't need runtime
polymorphism — the interface would have exactly one implementation with no second one
imminent (interface-pollution-checklist.md smell #1). Also rejected: three separate calls
inlined directly in the `reportDuplicate` handler body — this works but couples arg-parsing/
refusal-check code with verification-dispatch code in one large function, and can't be
called/tested in isolation from the full handler prologue.

**2. Auth mechanism — HTTP-only for all three ref types.**

Add a new `github.GetPR(ctx, owner, repo, number) (*PRResult, error)` using the same
`newGHRequest`/`ghHTTPClient`/`getGHToken` machinery as `GetIssue`, and do **not** call
`GetPRInfoCtx` from `report_duplicate`. This makes all three ref-type checks share one auth
resolution path — a `GITHUB_TOKEN` set (or not) has the same effect across PR/issue/commit
verification, no silent per-kind divergence.

`GetPRInfoCtx` remains unchanged and untouched — it's still used by `report_pr_created`
(a different tool, out of scope here) and by anything else that needs the richer `gh pr view`
JSON fields (`Mergeable`, `Additions`/`Deletions`, etc.) that a lean existence check doesn't need.

**3. Error classification — shared typed sentinels.**

```go
var ErrGitHubRefNotFound  = errors.New("github: reference not found")
var ErrGitHubAccessDenied = errors.New("github: access denied")
```

`GetIssue` (retrofit), `GetPR` (new), `GetCommit` (new) all wrap these on 404 and on
401/403-without-rate-limit-signal respectively, via `fmt.Errorf("%w: ...", ErrGitHubRefNotFound)`.
429 and 403-with-`Retry-After` remain plain, non-sentinel transient errors (retryable).
`verifyGitHubRefExists`'s callers classify via `errors.Is`, not string/status matching.

**403 with no rate-limit signal — resolved as non-retryable (`ErrGitHubAccessDenied` →
`ErrInvalidArgument`), not `ErrInternalError`.** Retrying an auth/permission failure with
the same configured credentials will produce the identical 403 every time — "retry" framing
would be actively misleading. The message text is deliberately distinct from the 404 case
("GitHub denied access... credentials may lack access... retrying will not help unless
credentials change") so the agent doesn't conflate "this ref doesn't exist" with "this ref
might exist but I can't see it."

**Private/inaccessible repos returning 404** (GitHub's own behavior — it disguises "exists,
no access" as "not found" for security) fall into the `ErrGitHubRefNotFound`/`ErrInvalidArgument`
branch, not `ErrGitHubAccessDenied`, because GitHub itself gives no signal to distinguish the
two cases from a 404 alone. The tool description and error message both mention this ambiguity
explicitly so an agent that's sure the ref exists knows to check its own token's repo access
rather than assume a typo.

## Consequences

### Positive
- One code path for all three ref types' auth resolution — no host-dependent divergence.
- `errors.Is`-based classification is robust to future message-text edits.
- `GetIssue`'s retrofit is additive (existing callers, if any appear later, see the same
  error text plus a wrapped sentinel — no behavior change for anything checking `err != nil`).

### Negative
- `GetPR` duplicates most of `GetPRInfoCtx`'s shape but hits a different (leaner) REST
  endpoint and returns a smaller struct — two "get a PR" functions now exist in the package
  for different purposes. Acceptable: they serve different callers with different needs
  (existence-only vs. full mergeability/diff-stat detail), and consolidating them is out of
  scope for this feature (would require auditing/migrating `report_pr_created` too).

### Neutral
- If a future feature needs the same existence-check pattern for a 4th ref kind (e.g.
  releases/tags), the switch in `verifyGitHubRefExists` gets a new case — no structural
  change needed, confirming the switch-over-interface choice scales fine for the actually-
  closed set of ref kinds GitHub URLs can express.

## Conflicting Decision (found post-hoc, unresolved)

`ADR-002-gh-cli-pr-existence-classification.md` (same directory, written by a concurrent
planning pass on this same item) makes the opposite call for the PR sub-case: keep
`GetPRInfoCtx` (the `gh`-CLI-subshell path) and classify "not found" via a stderr-substring
match, explicitly rejecting a new HTTP `GetPR` function as scope growth beyond what any
acceptance criterion requests. That ADR's own text flags its approach as "UNVERIFIED... as
of this planning phase" and a known-fragile heuristic.

This ADR's position (add `GetPR`, HTTP-only) is preferred here because: (a) it closes the
auth-mechanism-inconsistency risk pitfalls.md §4 calls the single most likely copy-paste
mistake, which the other ADR's approach leaves open (PR case still uses `gh auth`, issue/commit
still use `GITHUB_TOKEN`/keychain); (b) stderr-substring matching against a third-party CLI's
output is a fragile classification signal with no compiler/type backing, whereas an HTTP 404
is a stable, versioned API contract. But the other ADR's argument — that `GetCommit` was the
only new `github/`-package function any research pass unanimously called for, and a second
new function is unrequested scope — is not unreasonable either.

**This is a genuine, unresolved design disagreement between two independently-produced plans
for the same item, not a case where one side is simply wrong. Whoever begins implementation
(Phase 5) must pick one before Epic 1.2/3.2 (this plan) or the equivalent story in the other
plan can start, and should delete or clearly supersede whichever ADR is not chosen.**
