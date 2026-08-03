# Feature Landscape Research: pr-review-followup

## 1. Existing "detect X → auto-fix Y, capped, surfaced on exhaustion" conventions

The codebase already has a **shared, generic remediation framework** — this feature
should be a new detector plugged into it, not a parallel mechanism. Confirmed
architecture (`session/backlog_lifecycle.go`, `session/backlog_remediation.go`,
`session/domain/backlog.go`):

- **`domain.StuckReason`** (`session/domain/backlog.go:31-158`) is a closed,
  validated string enum. 13 current values (`AllStuckReasons`), each with an
  `IsValid()` case and a doc comment describing the exact condition. A new
  "unaddressed PR feedback" trigger, if it needs to be visually/notification-distinct
  from `pr_needs_fix`, would be a 14th value here — but see §4, reuse is likely
  sufficient per requirements.md's explicit scope.
- **`Storage.RemediationDue` / `RemediationBlocked`** (`session/backlog_remediation.go:168,213`)
  is the *single* shared exponential-backoff gate every automated remediation call
  site goes through: `remediationBackoffSchedule = [30m, 2h, 8h, 24h, 72h]`,
  `MaxRemediationAttempts = len(schedule) = 5`. Attempt 1 fires immediately (nil
  `next_remediation_at`); each subsequent attempt waits the schedule's next gap.
  After exhausting all 5, the row "parks" and needs a human `Reset`.
- **`*WithBackoffGate`-family helpers** (`autoReopenWithBackoffGate`,
  `retryPushFailedWithBackoffGate`, `remediateStaleWorkWithBackoffGate`,
  `retryOrphanedTriageWithBackoffGate`, and the one most relevant here,
  **`remediatePRFixWithBackoffGate`**, `backlog_lifecycle.go:3804-3845`) share one
  shape: `er.MarkStuck` (idempotent open-or-refresh of a durable
  `BacklogStuckState` row) → notify-once via `MarkStuckNotified` (checked via
  `row.NotifiedAt == nil`) → `RemediationDue` gate → only if due, actually dispatch
  the fix action. All of it is **best-effort and fails open**: `MarkStuck`/
  `FindOpenStuckStates`/`RemediationDue` errors are logged, never returned, and a
  `RemediationDue` error defaults `due = true` rather than silently stranding the
  item (see the comment at `backlog_lifecycle.go:3826-3830`).
- **Dedup granularity is per `(itemID, StuckReason)`**, not finer. `MarkStuck`/
  `MarkStuckNotified` dedup on that pair (`backlog_lifecycle.go:1230,1896,2043,3207,4204`
  all reference this "notify-once dedup" pattern). **There is currently no
  comment-ID- or review-ID-level dedup key anywhere in the codebase** — confirmed
  by grepping for `CommentID`/`commentID`/`seenComment` (zero hits). This is the
  one piece this feature must add net-new: the existing framework dedups "have we
  already opened a stuck-row for this reason on this item," not "have we already
  reacted to this specific piece of feedback." A comment/review ID (or content
  hash) watermark — most likely a new column on `BacklogStuckState` or a small
  sibling table — is required; nothing analogous exists to copy verbatim.
- `remediatePRFixWithBackoffGate`'s own doc comment (`backlog_lifecycle.go:3777-3803`)
  is worth reading directly — it documents *why* the backoff gate was retrofitted
  onto `ReconcilePRPending`'s fix-spawn path (a MAJOR bug: unbounded fix-session
  respawn every ~60s tick with no backoff before this fix), which is precisely the
  failure mode requirements.md's "dedup/staleness" scope item is trying to
  pre-empt for comment/review feedback.

**Convention to follow**: extend `remediatePRFixWithBackoffGate`'s caller
(`ReconcilePRPending`, `backlog_lifecycle.go:3850`) to also evaluate the new
comment/review trigger, feed it through the *same* `MarkStuck(..., StuckReasonPRNeedsFix, ...)`
→ `RemediationDue` → `AutoReopenForPRFix` pipeline, and add the new watermark
check as a pre-filter before `MarkStuck` is even called (so an already-addressed
comment never opens/refreshes the stuck row at all, rather than opening it and
then being gated by backoff — those are different failure semantics: backoff
gates "would fix again but too soon," the watermark should gate "nothing new to
fix").

## 2. Edge cases beyond requirements.md's explicit list

**What `gh pr view --json` actually exposes** (verified via `gh pr view --help`'s
JSON fields list, and `session/git/worktree_git.go:535-536`'s exact field set:
`statusCheckRollup,reviews,comments,mergeable,mergeStateStatus,state,isDraft`):

- `reviews` returns **every** review object, not deduped per-reviewer — so a
  reviewer (human or bot) who submits multiple reviews over time (e.g. Copilot
  posting a new COMMENTED review after each push) shows up as N separate review
  entries. `gh pr view`'s available JSON fields also include `latestReviews`
  (visible in the full field list from `gh pr view --help`), which — unlike
  `reviews` — returns only the most recent review per reviewer. The current code
  uses `reviews`, not `latestReviews`. **This directly answers the "same review
  updated or new review each time" question**: GitHub creates a **new** review
  object per Copilot re-review (not an update to the prior one), so a naive
  "any COMMENTED review present" check will refire on every re-review unless the
  design either switches to `latestReviews` or fingerprints on review ID.
- **Thread-resolution state (`isResolved`) is NOT available at all via
  `gh pr view --json`** — it is not in the JSON fields list (`additions,
  assignees, author, ... reviews, statusCheckRollup, ...`; no `reviewThreads`).
  GitHub only exposes `isResolved` on `reviewThreads` via the **GraphQL API**
  (`PullRequestReviewThread.isResolved`), reachable through `gh api graphql` (not
  `gh pr view`), confirmed by community GitHub docs/discussions (see Sources
  below). **This means**: today's `parsePRStatusPayload`/`GetPRStatus` cannot see
  "marked resolved via the Resolve Conversation button" at all — a new GraphQL
  call (separate from the existing `gh pr view` REST-flavored call) is required
  if the design wants to honor manual thread-resolution as a suppression signal.
  This is a bigger implementation lift than it looks from the requirements doc's
  framing as a single bullet.
- `comments` (top-level PR/issue comments) and inline review-thread comments are
  **different GitHub objects** with different resolution semantics: top-level
  `comments` (what `payload.Comments` in `parsePRStatusPayload` currently
  consumes, `worktree_git.go:568-573,638-641`) have no "resolved" concept at all —
  only inline review-thread comments do. A "PR author replies to a comment
  thread" scenario only maps to GitHub's resolution model for inline threads, not
  top-level comments; a reply to a top-level comment is just another top-level
  comment with no linkage back to "the thing it's replying to" in the JSON gh
  exposes today (no parent/in-reply-to field in the current payload struct).
- `HasBlockingReviews` today only flips on `CHANGES_REQUESTED`
  (`worktree_git.go:628-632`); `COMMENTED` reviews are **silently dropped** —
  neither added to `blockingReviews` nor even to `generalComments`. A `COMMENTED`
  review's `Body` (the review-level summary, as opposed to its inline comments)
  is currently invisible to the fix agent entirely, confirming requirements.md's
  premise.
- No existing "substantive" text filter anywhere in this file for comments/review
  bodies — `parsePRStatusPayload` includes every `payload.Comments` entry into
  `generalComments` unconditionally, with no length/keyword check. A new
  substantive-content filter (bare "LGTM", empty body) is net-new logic, not an
  extension of something existing.
- Existing tests to model new ones on: `worktree_git_test.go`'s
  `TestParsePRStatusPayload_HasBlockingReviews` (line ~312) and
  `TestParsePRStatusPayload_CIFailing` (line ~271) are the direct precedent for
  how a new `TestParsePRStatusPayload_CommentedReviewTrigger`-shaped test should
  be structured — table-driven against a raw JSON payload, asserting on the
  parsed `PRStatus` struct fields.

## 3. Industry precedent

- **CodeRabbit's Autofix** (`coderabbitai/skills` on GitHub, CodeRabbit docs)
  scans for review threads *it itself created* that remain unresolved, and treats
  manually resolving a thread (GitHub's native "Resolve conversation" button) as
  the suppression signal — the automation checks `isResolved` before acting, and
  a tool built on this pattern uses GraphQL's `resolveReviewThread` mutation
  (documented idempotent — safe to call on an already-resolved thread) to close
  threads it has itself addressed. This directly validates requirements.md's
  instinct to use thread-resolution as the dedup signal, and confirms GraphQL
  (not REST) is the necessary API surface.
- **GitHub's native Copilot code review** integrates with this same
  resolved/unresolved model — Copilot's own re-review behavior (per GitHub's
  docs) is oriented around "outdated" comments getting superseded by new reviews
  rather than persisting a single mutable review, consistent with the
  `reviews`-not-deduped finding in §2.
- No close analog was found for Dependabot/Renovate specifically, since those
  operate on a different trigger (new dependency version available, not human/bot
  review feedback) — their "don't re-open a PR for something already resolved"
  logic is branch-name/commit-based, not comment-ID-based, and doesn't transfer
  directly to this problem.

## 4. Unstated needs (solo-developer angle)

requirements.md's "Users/Consumers" is a solo developer running this
autonomously. Beyond the explicit ACs:

- **A manual override / suppression escape hatch.** If Copilot posts a
  wrong/noisy COMMENTED review, the *only* current way to stop it re-triggering
  fix sessions is either (a) the dedup mechanism naturally not re-firing because
  nothing changed, or (b) disabling `ReconcilePRPending`'s whole fix-trigger path
  for that item. Neither is a clean "ignore this specific piece of feedback"
  control. Given CodeRabbit's precedent (manually resolving a thread = suppress),
  the natural reuse here is: **resolving the GitHub thread yourself is already
  the override** — no new stapler-squad-side UI/flag needed, *if* the design
  treats `isResolved` as authoritative suppression. This should be made explicit
  in the plan phase rather than left implicit, since it's the only override path
  and isn't called out as an AC.
- **Idempotent/duplicate Copilot-review-request avoidance.** requirements.md
  scopes "wire a Copilot review request into the ship flow" but doesn't say what
  happens if `ReconcilePRPending` or a rework cycle runs again after a Copilot
  review was already requested (re-requesting on every fix-session cycle would be
  noisy/wasteful). The `pushAndCreatePR` call site (`backlog_lifecycle.go:3158`,
  which calls `GitWorktree.CreatePR` at `session/git/worktree_git.go:328`) is a
  one-shot "PR just created" hook — requesting Copilot review there once is
  natural and won't re-fire on later reconciliation ticks, since `pushAndCreatePR`
  only runs at PR-creation time, not on every poll.
- **Mechanism for requesting the review**: `gh pr edit --add-reviewer @copilot`
  (native `gh` CLI support, landed March 2026 per GitHub's changelog) or the
  equivalent REST call requesting `copilot-pull-request-reviewer[bot]` as
  reviewer — both fit the existing `safeexec.CommandContext("gh", ...)` pattern
  already used throughout `worktree_git.go` (e.g. `CreatePR` at line 328,
  `EnablePRAutoMerge` at line 650) for a `RequestCopilotReview`-shaped new method
  alongside them.
- **Rate/cost awareness**: since this system already runs a 60s reconciliation
  tick indefinitely (`ReconcileStuck`, `server/dependencies.go:918`) and every
  `gh pr view` call is a live GitHub API hit, adding thread-resolution checking
  via a *second* GraphQL call per tick per `pr_pending` item roughly doubles
  GitHub API call volume for this feature specifically — worth flagging in the
  plan phase's cost/complexity tradeoff, not just a correctness question.

## Sources

- [Request Copilot code review from GitHub CLI — GitHub Changelog](https://github.blog/changelog/2026-03-11-request-copilot-code-review-from-github-cli/)
- [gh-copilot-review (k1LoW) — gh extension, duplicate prevention / outdated review cleanup](https://github.com/k1LoW/gh-copilot-review)
- [coderabbitai/skills — autofix/github.md](https://github.com/coderabbitai/skills/blob/main/skills/autofix/github.md)
- [CodeRabbit Autofix blog post](https://www.coderabbit.ai/blog/you-don-t-need-to-implement-that-autofix-will)
- [GitHub community discussion — GraphQL resolved conversations / reviewThreads.isResolved](https://github.com/orgs/community/discussions/24854)
- [github/github-mcp-server issue #1768 — resolving/unresolving PR review threads needs GraphQL](https://github.com/github/github-mcp-server/issues/1768)

## Key file/line references (this repo)

- `session/domain/backlog.go:31-158` — `StuckReason` enum + `IsValid()`
- `session/backlog_remediation.go:23-58` — backoff schedule, `MaxRemediationAttempts`
- `session/backlog_lifecycle.go:3777-3845` — `remediatePRFixWithBackoffGate` (doc comment + impl)
- `session/backlog_lifecycle.go:3850` — `ReconcilePRPending` (registered in `server/dependencies.go:918`)
- `session/backlog_lifecycle.go:3158` — `pushAndCreatePR` (natural one-shot hook for Copilot review request)
- `session/git/worktree_git.go:328` — `CreatePR`
- `session/git/worktree_git.go:526-645` — `GetPRStatus` / `parsePRStatusPayload`
- `session/git/worktree_git.go:421-461` — `PRStatus` struct
- `session/git/worktree_git_test.go:146,229,271,312,353` — existing `parsePRStatusPayload` test precedents
