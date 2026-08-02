# Research: Build vs. Buy — backlog-self-resolve

Scope: for each of the three technical questions in `requirements.md`'s "Open
questions for research phase", evaluate whether to build from scratch, extend
existing repo code, or adopt a third-party library.

## 1. GitHub URL parsing/verification for `report_duplicate`

### What exists today

- `session.ParseGitHubURL` (`session/repo_path.go:93`) parses PR/branch/repo
  URLs and shorthand into a `GitHubRef{Owner, Repo, Branch, PRNumber, Type}`.
  `GitHubRefType` only has three values: `GitHubRefTypeRepo`, `GitHubRefTypeBranch`,
  `GitHubRefTypePR` — **no issue or commit URL pattern is recognized today**.
  A `duplicate_ref` like `https://github.com/o/r/issues/42` or a commit SHA
  URL (`.../commit/<sha>`) will fail to parse as-is.
- `VerifyPRMatchesBranch` (`server/mcp/tools_github.go:272`) is narrowly
  scoped to "does this PR number belong to *this session's own branch*" — it
  calls `githubpkg.GetPRForBranch` (GraphQL, by branch name), not "does PR #N
  exist." Wrong shape for `report_duplicate`, which needs simple existence
  verification of an arbitrary PR/issue/commit, not a branch-ownership check.
- The `github/` package (used by `tools_github.go`) is a **hand-rolled GitHub
  client**, not `google/go-github` — confirmed no `go-github` entry in
  `go.mod`. It's internally a mix of two calling conventions:
  - `GetPRInfoCtx` (`github/client.go:247`) shells out to `gh pr view ... --json`
    via `safeexec.CommandContext("gh", ...)` — a subprocess, and on failure
    returns only an unwrapped `fmt.Errorf` (stderr text), no typed
    not-found-vs-transient distinction.
  - `GetIssue` (`github/repos.go:270`) uses native `net/http` against
    `api.github.com` directly (`ghHTTPClient`, `newGHRequest`) and **does**
    switch on HTTP status: 404 → "not found" text, 401/403/429 → distinct
    rate-limit/auth text — much closer to what FR4's two-channel
    (`ErrInvalidArgument` vs `ErrInternalError`) requirement needs, though it
    still returns plain `fmt.Errorf` rather than sentinel/typed errors, so the
    MCP layer would classify by inspecting the returned error text/status
    rather than an `errors.Is` check.
  - There is **no existing "does this commit SHA exist" call** anywhere in
    `github/`.

### Option A — Reuse/extend `github/` package + add `GetCommit`, extend `ParseGitHubURL`

**Pros**
- `github/` already owns GitHub auth (`getGHToken`, keychain, device flow),
  rate-limit-aware status handling (`rate_limit.go`), and etag caching —
  reimplementing that per-tool would duplicate real complexity (auth alone
  spans `client.go`, `keychain.go`, `device_auth.go`, `hosts.go`).
- `GetIssue`'s pattern (native `net/http`, explicit status-code switch) is a
  ready template: a new `GetCommit(ctx, owner, repo, sha) (*CommitResult, error)`
  in `github/repos.go` following the same shape is a small, consistent
  addition — REST endpoint `GET /repos/{owner}/{repo}/commits/{sha}` returns
  404 for a nonexistent commit, exactly analogous to the issue lookup.
  `GetPRInfoCtx` (or a lighter existence-only variant) already covers PR
  existence.
- Consistent with this repo's `prefer-go-git-over-subshells.md` rule in
  spirit: the *newest* pattern in this package (`GetIssue`) already moved off
  the `gh` CLI subshell to native HTTP; extending via that pattern rather
  than the older `GetPRInfoCtx`-style subshell keeps the package converging
  rather than adding a third calling convention.
- `ParseGitHubURL`'s regex-per-`GitHubRefType` structure is easy to extend
  with `GitHubRefTypeIssue` and `GitHubRefTypeCommit` cases (issue: `/issues/(\d+)`;
  commit: `/commit/([0-9a-f]{7,40})`) without touching existing PR/branch/repo
  branches — low regression risk, and `session/url_parser_test.go`-style table
  tests already establish the pattern to extend.

**Cons**
- `GetIssue`/`GetCommit`-style errors are still un-typed (`fmt.Errorf`), so
  `report_duplicate`'s FR4 two-channel classification will need to either (a)
  pattern-match returned error text, which is brittle, or (b) do the
  classification at the point of the HTTP call (i.e. build the verification
  function to return a structured result/error type directly, bypassing
  string-matching entirely). This is a small amount of *new* code regardless
  of which existing helper is reused — not zero-cost, but it's additive, not
  a rewrite.
- Three ref kinds (PR/issue/commit) means three different verification calls
  with three different endpoints — some irreducible per-kind logic no matter
  what's reused.

**Verdict: extend, don't rebuild.** Add `GitHubRefTypeIssue`/`GitHubRefTypeCommit`
to `session.ParseGitHubURL`, add `github.GetCommit` alongside the existing
`GetIssue`, and reuse `GetPRInfoCtx` (or extract a light existence-only path)
for the PR case. Do this at the `net/http`-based-status-check layer (`GetIssue`'s
pattern), not the `gh` CLI subshell layer — new code should return a
result/typed-enough error so `report_duplicate`'s handler can build FR4's
`ErrInvalidArgument` (404 — definitive) vs `ErrInternalError` (401/403/429/
network — transient, "retry" wording) split directly from the HTTP status,
mirroring how `report_pr_created` already splits `verifyPR`'s bool-vs-error
return (`tools_backlog.go` ~L707-715).

### Option B — Adopt `google/go-github`

**Pros**
- Mature, well-typed client with `Issues.Get`, `PullRequests.Get`,
  `Repositories.GetCommit`, and typed `*github.ErrorResponse`/`github.RateLimitError`
  for exactly the not-found-vs-rate-limited distinction FR4 wants "for free."

**Cons**
- Not a current dependency (`go.mod` has no `go-github` entry) — adopting it
  means a new dependency tree, a second auth/token-plumbing path alongside
  the existing hand-rolled `getGHToken`/keychain/device-flow machinery in
  `github/`, and an unclear boundary (does existing `github/` code migrate
  too, or do two GitHub clients coexist?). That migration is out of scope for
  a small, mechanical `report_duplicate` feature and the requirements
  document's non-goals lean toward minimal footprint (no schema changes, no
  new terminal status).
- All of the surrounding wiring this feature needs (auth token resolution,
  rate-limit-aware retry framing, existing PR verification for
  `report_pr_created`) already lives in `github/` — a second client duplicates
  rather than replaces that.

**Verdict: reject for this feature.** Right call for a future "replace the
hand-rolled GitHub client wholesale" project, wrong call for one MCP tool.
Adopting it here would be scope creep against FR8's "no schema changes,
minimal footprint" spirit even though FR8 technically only talks about ent
schema — the same minimalism argument applies to dependencies.

## 2. Existing internal "duplicate"/"supersede" helpers to extend instead of duplicating

Searched the repo for `duplicate|supersede` (case-insensitive, excluding
`_test.go`) across all `.go` files. ~45 hits, but every one is either:
- unrelated uses of the English word "duplicate" (e.g. `dup_fd_unix.go` /
  `dup_fd_windows.go` — file descriptor duplication for pty handling;
  `error_registry.go` — deduplicating registered errors; `search_service.go`,
  `tag_manager.go` — deduplicating slices/results), or
- generic "this replaces an earlier state" language unrelated to backlog
  duplicate-PR detection (e.g. `pipeline_engine.go`, `registry.go` discussing
  workflow/version supersession).

**No existing helper does "mark this backlog item as a duplicate of another
PR/issue" or anything adjacent.** `report_pr_created`'s verification path
(`tools_backlog.go` ~L695-720) is the closest analog in *shape* (two-channel
GitHub verification before mutating item state) but is not itself reusable
for duplicates — it's built around "does PR #N belong to *my own* branch,"
the opposite check from "does PR/issue/commit #N exist, unrelated to my
branch."

**Verdict: build the `report_duplicate` tool as new code**, following
`report_pr_created`'s established two-channel-error *shape* (Option A above)
rather than reusing any duplicate-marking logic, because none exists.

## 3. CAS/optimistic-concurrency pattern for FR1/FR2

- `TransitionBacklogItemStatus` (`server/services/backlog_service_lifecycle.go:486`,
  and the storage-layer entry point called from MCP handlers,
  `h.storage.TransitionBacklogItemStatus(ctx, itemID, targetStatus, precondition, triggeredBy)`)
  already accepts an optional `*session.BacklogItemPrecondition{ExpectedStatus, ExpectedUpdatedAt}`
  and rejects the call if the loaded item's current state doesn't match — a
  standard compare-and-swap over the ent-backed row, already exercised by
  the existing `request_review` code path today (currently hardcoded to
  `ExpectedStatus: string(session.BacklogStatusInProgress)`,
  `tools_backlog.go:414`).
- FR1 only requires computing `ExpectedStatus` from the status actually
  observed on the loaded item (`in_progress` or `pr_pending`) instead of a
  constant — a one-line change to which value gets passed into the existing
  `BacklogItemPrecondition{...}` struct literal, not a change to the
  transition mechanism itself.
- FR2's "refuse if an active review-role session exists" is a pre-check
  against `ListItemSessions(ctx, itemID) ([]ItemSessionSummary, error)`
  (`session/storage.go:1098`, `session/storage_backlog.go:138`), which
  already returns each session's `Role` and `EndedAt` — filtering for
  `Role == session.SessionRoleReview && EndedAt == nil` is a query over data
  the storage layer already exposes, not a new capability.
- `TriggeredByAgent` does not exist yet (only `TriggeredByUser` and
  `TriggeredBySystem` are defined, `session/backlog.go:90-94`) — FR7 requires
  adding it, a one-line const addition next to the existing two.

**Pros of reusing the existing CAS pattern as-is:** it's the repo's proven,
single mechanism for backlog status transitions — every other status change
(including `report_pr_created`'s `SetBacklogItemPRAndTransition`) goes through
the same ent-backed conditional-update path, and this project's ADR-001 (per
FR8) explicitly forbids adding new statuses/schema, which rules out any
parallel state-tracking mechanism (e.g. a separate lock table or version
field) that would only make sense alongside a broader status model.

**Cons:** none identified specific to this feature. The pattern is already
concurrency-safe for the single-writer-wins case this feature needs — no
distributed contention or multi-writer conflict scenario in the requirements
that the existing `ExpectedStatus`/`ExpectedUpdatedAt` precondition couldn't
express.

**Verdict: confirmed — no new concurrency primitive needed.** This is a small,
mechanical generalization of an already-correct pattern: (a) pin
`ExpectedStatus` to the observed status rather than a hardcoded constant, and
(b) add one `ListItemSessions`-backed guard clause before the transition
call. Nothing here justifies a different CAS/locking mechanism.

## Summary

| Area | Verdict |
|---|---|
| GitHub ref parsing (issue/commit) | Extend `session.ParseGitHubURL` — add 2 new `GitHubRefType` cases |
| GitHub existence verification | Extend `github/` package (native `net/http`, `GetIssue`-style) — add `GetCommit`, reuse `GetPRInfoCtx`/`GetIssue`; do **not** adopt `google/go-github` |
| Internal duplicate/supersede helper | None exists — build `report_duplicate` as new code, following `report_pr_created`'s two-channel-error shape |
| CAS precondition mechanism | Reuse `TransitionBacklogItemStatus`'s existing `BacklogItemPrecondition` as-is — no new primitive |
