# Stack Research: pr-review-followup

Environment: `gh version 2.86.0 (2026-01-21)`, authenticated as `tstapler` (scopes
`repo`, `workflow`, `read:org`, `admin:public_key`, `gist`).

## 1. `gh` commands needed

### 1a. Fetching review/comment data with IDs + timestamps

`gh pr view <n> --json comments,reviews` is GraphQL-backed (not REST) and **already
returns stable IDs and timestamps** for both fields — confirmed live against PR #300
in this repo:

```json
{
  "id": "IC_kwDORtTofs8AAAABMziiLA",
  "author": {"login": "github-actions"},
  "authorAssociation": "CONTRIBUTOR",
  "body": "...",
  "createdAt": "2026-08-02T01:01:38Z",
  "includesCreatedEdit": false,
  "isMinimized": false,
  "minimizedReason": "",
  "url": "https://github.com/tstapler/stapler-squad/pull/300#issuecomment-5154316844",
  "viewerDidAuthor": false
}
```

Full field list for `gh pr view --json <badfield>` (self-documenting error output,
run 2026-08-02): `additions, assignees, author, autoMergeRequest, baseRefName,
baseRefOid, body, changedFiles, closed, closedAt, closingIssuesReferences,
comments, commits, createdAt, deletions, files, fullDatabaseId, headRefName,
headRefOid, headRepository, headRepositoryOwner, id, isCrossRepository, isDraft,
labels, latestReviews, maintainerCanModify, mergeCommit, mergeStateStatus,
mergeable, mergedAt, mergedBy, milestone, number, potentialMergeCommit,
projectCards, projectItems, reactionGroups, reviewDecision, reviewRequests,
reviews, state, statusCheckRollup, title, updatedAt, url`.

**No version gating found** for any of these fields against 2.86.0 — all were
usable directly. `comments` and `reviews` are the two fields the codebase already
consumes (`session/git/worktree_git.go:536`); both already carry `id`/`createdAt`
in the raw JSON even though the current Go struct (`worktree_git.go:550-578`)
discards them (`Body`/`Author.Login` only). **No struct changes are needed to fetch
IDs — the existing single `gh pr view --json ...` call already returns them; only
the Go unmarshal struct needs new fields** (`ID string`, `CreatedAt time.Time`
per review/comment).

`reviews` (top-level, per-review — not per-comment) came back empty for every PR
sampled in this repo (#295, #289, #280), so no live COMMENTED-state sample was
available locally; structure is documented by GitHub's GraphQL schema
(`PullRequestReview`: `id`, `state`, `body`, `author{login}`, `submittedAt`).

### 1b. Inline / review-thread comments (not just top-level issue comments)

Two more granular sources exist beyond the `comments`/`reviews` fields already in
use, neither of which the codebase currently touches:

- **REST**: `gh api repos/{owner}/{repo}/pulls/{number}/comments` — per-inline-
  comment records with `id`, `created_at`, `path`, `line`, `user.login`,
  `pull_request_review_id`. Tested against PR #300: returned `[]` (no inline
  comments on that PR), confirming the call works but giving no live sample of a
  populated inline thread.
- **GraphQL** (richer — has resolution state): `pullRequest.reviewThreads` via
  `gh api graphql`, tested live:

  ```
  gh api graphql -f query='
  query {
    repository(owner: "tstapler", name: "stapler-squad") {
      pullRequest(number: 300) {
        reviewThreads(first: 5) {
          nodes { id isResolved isOutdated
            comments(first: 3) { nodes { id body author { login } createdAt } } }
        }
      }
    }
  }'
  ```
  returned `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}`
  — call succeeds, this PR just has no threads. `isResolved` on
  `PullRequestReviewThread` is a **native GitHub dedup signal**: a thread can be
  marked resolved independent of the review's `CHANGES_REQUESTED`/`COMMENTED`
  state. This is a materially better dedup primitive than tracking comment IDs by
  hand for *inline* feedback — GitHub itself tracks "addressed" for threads (it
  just doesn't do so for whole `COMMENTED` reviews or top-level issue comments,
  which is the gap the requirements doc correctly identifies).
- `PullRequestReviewComment.isMinimized` (with `minimizedReason` values
  `abuse|off-topic|outdated|resolved|duplicate|spam`) is a second, comment-level
  signal available over the same GraphQL query, per GitHub Community Discussion
  #9175 and #24854 (found via WebSearch, not independently verified live — this
  repo's sampled PRs had no minimized comments to confirm against).

None of `comments`, `reviews`, `reviewThreads`, or `isMinimized` needed elevated
scopes beyond what the current `gh auth status` token already has (`repo`).

### 1c. Requesting a Copilot code review

Confirmed via WebSearch (GitHub Changelog, 2026-03-11,
https://github.blog/changelog/2026-03-11-request-copilot-code-review-from-github-cli/):

```
gh pr edit --add-reviewer @copilot
```

is now first-class syntax (Copilot appears as a selectable reviewer in `gh pr
edit`/`gh pr create`'s interactive and non-interactive reviewer flags). The
underlying GitHub actor login when it actually posts is
`copilot-pull-request-reviewer[bot]` (per WebSearch results on identifying
Copilot's automated review; **not independently reproduced live** — no repo
sampled here had a Copilot review to confirm the exact `author.login` string
against). Match on this login (case-sensitive, `[bot]` suffix literal) to
distinguish Copilot's automated review from a human one in `payload.Reviews[].Author.Login`.

Known limitation (WebSearch, same source): Copilot review requests cannot satisfy
a CODEOWNERS or "required reviewers" branch-protection rule — it comments only,
never approves/requests-changes — and each request consumes a Copilot premium
request quota (plus Actions minutes on private repos, from June 2026 per the
changelog post).

**Gap**: this environment's installed `gh pr edit --help` did not produce real
help output when invoked from this sandboxed shell — see "Environment caveat"
below. The `--add-reviewer @copilot` syntax is therefore VERIFIED against
GitHub's official changelog post but UNVERIFIED as actually working end-to-end
against this specific `gh` 2.86.0 build in this sandbox; confirm with a real dry
run (e.g. against a scratch/throwaway PR) before wiring it into the ship
pipeline.

**Environment caveat (important for implementation)**: in this sandboxed Bash
tool, `gh pr edit --help` (and, by extension, likely any `gh pr edit ...`
invocation) does not print real `gh` help/output — it returned the string
`ok edited` with exit code 0, twice, reproducibly. `gh pr list` immediately after
showed no new `reviewRequests` on any of this user's open PRs (#300, #295, #289,
#286 all still `[]`), so nothing was actually mutated — this looks like a
sandbox-level interception/stub of mutating `gh pr edit` calls rather than a real
execution, consistent with this task's "do not modify source code" scope. Do not
treat `ok edited` as evidence the flag syntax works; it means the call never hit
GitHub's API in this environment. Re-verify against a real `gh` binary outside
the sandbox (or via a throwaway test PR) before shipping.

## 2. Existing `gh` usage in `session/git/worktree_git.go`

All `gh`/`git` calls in this file go through the same two funcnels:
- `safeexec.CommandContext(ctx, "gh"/"git", args...)` — the project's sandboxed
  exec wrapper (`session/git/worktree_git.go:13` imports
  `github.com/tstapler/stapler-squad/executor/safeexec`), always with an explicit
  `context.WithTimeout` (30s for all `gh pr *` calls in this file) and
  `cmd.Dir = g.worktreePath`.
- `g.runExec` / `g.runCombinedOutput` (`worktree_git.go:299-320`) — thin
  indirection through an injectable executor, so calls are unit-testable without
  a real subprocess.

Every existing `gh pr` call site and its JSON usage:

| Line | Call | Purpose |
|---|---|---|
| `worktree_git.go:346` | `gh pr create --title ... --body ... --head ...` | create PR |
| `worktree_git.go:382` | `gh pr view --json number --jq .number --head <branch>` | resolve PR number post-create |
| `worktree_git.go:398` | `gh pr list --head <branch> --json number,url --jq '.[0] \| .number, .url'` | find existing PR for branch |
| `worktree_git.go:535` | `gh pr view <n> --json statusCheckRollup,reviews,comments,mergeable,mergeStateStatus,state,isDraft` | **the call this feature extends** — feeds `parsePRStatusPayload` |
| `worktree_git.go:656` | `gh pr merge <n> --auto --squash` | enable auto-merge |
| `worktree_git.go:675` | `gh pr close <n> --comment <text>` | close as superseded |
| `worktree_git.go:691` | `gh pr view <n> --json state --jq .state` | check merged |

`checkGHCLI()` (referenced at `worktree_git.go:529`, `:660`, `:678`, `:693`) is
called as a guard before every `gh` invocation in this file — any new call this
feature adds should follow the identical guard-then-`safeexec`-then-
`runCombinedOutput` shape used by `GetPRStatus` itself, i.e. **extend the existing
single `gh pr view` call's `--json` field list rather than adding a second `gh`
subprocess**, since `comments`/`reviews` (and their `id`/`createdAt`) are already
in the one call's output — only the Go struct needs new fields. A *second* call is
only required if inline `reviewThreads` (§1b) are added, since that data isn't
reachable via `gh pr view --json`.

`parsePRStatusPayload` (`worktree_git.go:549-643`) is pure/no I/O — deliberately
separated from `GetPRStatus` for direct unit testing (comment at `:546-548`
states this explicitly), and its test file
(`session/git/worktree_git_test.go:142-360+`) is entirely table-driven against
raw JSON literals matching the exact `--json` field shape (`statusCheckRollup`,
`reviews`, `comments`, `mergeable`, `mergeStateStatus`) — any new signal should
follow this same pattern: extend the anonymous unmarshal struct, add fields to
`PRStatus`, and add table-driven tests over literal JSON strings (see
`TestParsePRStatusPayload_HasBlockingReviews` at `:312` for the closest existing
analog — same `reviews[].state` shape a `COMMENTED`-state case would extend).

## 3. Dedup mechanism — Go packages and ent schema fields

**Go stdlib**: `encoding/json` is already imported and used in this exact file
(`worktree_git.go` unmarshals the `gh pr view` payload with it) — no new
dependency needed to serialize a "last-seen IDs" set. A `map[string]struct{}` or
`[]string` marshaled to a `TEXT` column is the natural fit, following the
project's own established pattern (see below).

**go.mod**: no queue/set/dedup-specific package is needed or present beyond
stdlib. `github.com/puzpuzpuz/xsync/v4` (concurrent map) and `golang.org/x/sync`
are already vendored but are for in-memory concurrency, not persistence — not
applicable here since the dedup state must survive restarts (durable, per the
`BacklogStuckState`/`BacklogItem` schema pattern already in use for everything
else in this reconciliation loop).

**Ent schema — no existing field fits; a new one is needed.** Checked both
candidate schemas:

- `session/ent/schema/backlog_item.go` (`BacklogItem`) has `shipped_file_stats`
  (`field.String`, Optional, comment: `"JSON []ShippedFileStat{...} — per-file
  diff stats captured at ship time"`) — this is the **existing, directly
  analogous precedent**: a JSON-encoded string field on `BacklogItem`, documented
  in its own `.Comment()`, populated/parsed by hand (no ent JSON field type is
  used elsewhere in this schema). A new field like
  `last_seen_pr_feedback_ids` (`field.String`, Optional, JSON-encoded
  `[]string` of comment/review-comment/thread IDs already factored into a prior
  fix attempt) follows this exact precedent. `user_modified_fields` is the same
  shape (`"JSON set of field names modified by the user"`) — two prior-art
  examples of "small JSON blob in a `field.String`" already exist in this schema.
- `session/ent/schema/backlog_stuck_state.go` (`BacklogStuckState`) is the other
  candidate — it already durably tracks `remediation_attempts`,
  `next_remediation_at`, and a free-text `context` string keyed by
  `(item_id, reason)`, and is where `RemediationDue`
  (`session/backlog_remediation.go:168`) already gates on
  `StuckReasonPRNeedsFix`. Confirmed by reading `RemediationDue`/
  `evaluateRemediation`: this existing gate is **purely time-based backoff**
  (attempt count + `next_remediation_at`), with no field for tracking which
  specific comment/review IDs were already seen — it answers "is a retry due" but
  not "is this the *same* feedback as last time." The `context` field is
  free-text/human-readable, not a place to safely persist a machine-parsed ID
  list without conflating concerns already documented as that field's purpose.

  **Recommendation for the plan phase**: put the new dedup field on `BacklogItem`
  (item-scoped, not reason-scoped — matches `shipped_file_stats`'s precedent of
  "one snapshot per item") rather than `BacklogStuckState`, since the dedup
  question ("have I already factored this specific comment into a fix?") is
  orthogonal to the stuck-state backoff gate's question ("is a retry due yet");
  the two mechanisms should compose (dedup decides *whether* new-signal exists at
  all; `RemediationDue`'s existing backoff still decides *whether to act on it
  right now*) rather than merge into one field.

Adding this field requires the standard ent workflow already documented in this
repo's own rules
(`.claude/rules/ent-schema-generation.md`): edit
`session/ent/schema/backlog_item.go`, then
`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert
./session/ent/schema` (per `session/ent/generate.go`), then `go build ./...`,
committing all of `session/ent/` together.

## 4. Copilot review request — current (2026) best practice

Per WebSearch (GitHub Changelog 2026-03-11 + community discussion #186152 +
third-party `gh` extensions `k1LoW/gh-copilot-review` and
`ChrisCarini/gh-copilot-review`, both of which predate and were superseded by
native `gh` support):

- **Current mechanism**: `gh pr edit <n> --add-reviewer @copilot` (native, as of
  the gh release the March 2026 changelog post covers). This superseded the need
  for third-party `gh` extensions that existed specifically to work around the
  lack of native CLI support (both linked repos describe themselves as filling
  that former gap, with features like duplicate-prevention and outdated-review
  cleanup gh itself does not (yet) automate).
- Copilot cannot satisfy CODEOWNERS/required-reviewer branch protection (comment-
  only, no approve/request-changes).
- Cost/quota: consumes 1 Copilot premium request per review; from June 1 2026,
  also consumes GitHub Actions minutes on private repos (both from the same
  changelog post).
- No re-request/dedup API surfaced by search results beyond re-running
  `--add-reviewer @copilot` — community discussion #186152 (open as of the
  search) is specifically asking GitHub for a proper re-request API, implying
  none exists yet distinct from adding-as-reviewer again. This matters for the
  "wire a Copilot review into the ship flow" scope item: calling
  `--add-reviewer @copilot` unconditionally on every ship-flow run (rather than
  gating it) risks the same "re-triggers forever" failure mode the requirements
  doc calls out for `COMMENTED` reviews — gate the Copilot review request behind
  a one-time-per-PR flag (e.g. call it once at PR-creation time in the ship
  pipeline, not on every reconcile tick), not inside `ReconcilePRPending`'s
  60-second loop.

## Sources

- [Request Copilot code review from GitHub CLI — GitHub Changelog](https://github.blog/changelog/2026-03-11-request-copilot-code-review-from-github-cli/)
- [gh-copilot-review (k1LoW)](https://github.com/k1LoW/gh-copilot-review)
- [gh-copilot-review (ChrisCarini)](https://github.com/ChrisCarini/gh-copilot-review)
- [Feature Request: API/CLI support for re-requesting PR reviews (including Copilot) — community discussion #186152](https://github.com/orgs/community/discussions/186152)
- [List comments for a pull request review — lack of status — community discussion #9175](https://github.com/orgs/community/discussions/9175)
- [GraphQL resolved conversations — community discussion #24854](https://github.com/orgs/community/discussions/24854)
- [GitHub GraphQL API: Pull requests reference](https://docs.github.com/en/graphql/reference/pulls)
- Local: `gh --version` (2.86.0), `gh auth status`, `gh pr view 300 --json comments,reviews`,
  `gh api repos/tstapler/stapler-squad/pulls/300/comments`,
  `gh api graphql` against `reviewThreads` — all run live in this repo, 2026-08-02.
