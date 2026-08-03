# Build vs. Buy: pr-review-followup

**Date**: 2026-08-02

## Current implementation (baseline for comparison)

`GetPRStatus`/`parsePRStatusPayload` (`session/git/worktree_git.go:528,549`) already
does a single `gh pr view --json statusCheckRollup,reviews,comments,mergeable,
mergeStateStatus,state,isDraft` call per poll and evaluates every signal from one
JSON payload:

- CI failures → `PRStatus.CIFailing`
- `CHANGES_REQUESTED` reviews → `PRStatus.HasBlockingReviews` (worktree_git.go:629)
- `mergeStateStatus`/`mergeable` → `PRStatus.HasConflicts`
- Plain PR comments are already parsed into `status.generalComments`
  (worktree_git.go:638-641) and folded into `FeedbackText`, but **only as context
  text** — they don't set any bool, so `ReconcilePRPending`
  (`session/backlog_lifecycle.go:4048`) never treats them as a trigger.
- `COMMENTED`-state reviews (which is what Copilot's automated review posts as) are
  parsed into the `Reviews` loop (worktree_git.go:627-636) but fall through the
  `switch` untouched — only `CHANGES_REQUESTED` and `APPROVED` are handled.

`ReconcilePRPending` triggers `AutoReopenForPRFix` only on
`CIFailing || HasBlockingReviews || HasConflicts` (backlog_lifecycle.go:4048,4101).
Dedup/backoff for that trigger already exists and is durable: `MarkStuck`/
`MarkStuckNotified` write a `BacklogStuckState` row keyed by
`domain.StuckReasonPRNeedsFix` with a `NotifiedAt` timestamp, gated by
`remediatePRFixWithBackoffGate` (backlog_lifecycle.go:3777-3844), which calls a
`RemediationDue` backoff check before spawning another `AutoReopenForPRFix`. This is
the same notify-once-dedup pattern used by every other stuck-item detector in the
file (`abandoned_review`, `pr_ready_unmerged`, etc. — see `grep -n dedup
session/backlog_lifecycle.go`). No new field exists yet, however, for "have I
already reacted to *this specific* COMMENTED review / this specific comment ID" —
that's the actual gap, not the backoff mechanism itself.

## 1. Existing OSS library vs. continuing to shell out to `gh` CLI

`go.mod` was checked directly — **neither `google/go-github` nor
`shurcooL/githubv4` (nor any other GitHub API client) is a current dependency.**
The repo's only GitHub touchpoint anywhere is the `gh` CLI, invoked via
`safeexec.CommandContext` throughout `session/git/worktree_git.go` (pr view, pr
create, pr merge, pr close, etc.) — the same pattern
`.claude/rules/prefer-go-git-over-subshells.md` describes as acceptable "when the
native option can't do the job," except here it's not that go-git can't do the
job — `gh` CLI is simply the project's one and only established GitHub data
source.

**Pros of adding `go-github`/`githubv4` for just this signal:**
- Typed responses, no JSON payload hand-parsing.
- Native pagination handling for repos with very high review/comment volume.

**Cons:**
- Fragments PR-status fetching into two different data sources (`gh` CLI for
  CI/reviews/conflicts, an API client for comments/COMMENTED-reviews) for what is
  logically one `PRStatus` struct populated from what is already one `gh pr view`
  call. The new signal (COMMENTED reviews, plain comments) is **already present in
  the same JSON payload** `GetPRStatus` fetches today — `payload.Reviews[].State ==
  "COMMENTED"` and `payload.Comments` are already unmarshaled at
  worktree_git.go:561-573. Nothing needs a second API round-trip, let alone a
  second client library.
- New dependency + auth plumbing (a GitHub App token or PAT, separate from `gh`'s
  own OAuth device-flow auth already relied on via `checkGHCLI` at
  `session/git/util.go:46`) for zero net new data.
- This repo's PR volume is single-repo, single-maintainer scale — pagination
  edge cases from a hypothetical 1000+-comment PR are not a realistic constraint
  here.

**Verdict: Not recommended.** Stay on `gh` CLI — the new signal requires zero new
API calls, only new logic in the `switch` at worktree_git.go:627-636 (add a
`case "COMMENTED":` branch) and the comments loop at worktree_git.go:638-641 (set a
bool instead of / in addition to appending to `generalComments`). Adding a second
client for this would be paying an integration cost for a signal already sitting
unused in a payload the code already fetches.

## 2. SaaS / managed "PR review bot" replacement

Evaluated categories: hosted AI PR reviewers (Qodo/PR-Agent-as-a-service, CodeRabbit,
Greptile) and generic "auto-respond to review comments" bots (Pullfrog).

**Pros:**
- Zero maintenance of polling/dedup logic; managed vendor handles GitHub App
  webhooks, review dismissal semantics, etc.
- Some (e.g. Pullfrog, see search below) explicitly market "auto-address new human
  reviews" as a feature.

**Cons — architectural incompatibility, not just cost:**
- Every one of these tools' "fix" action is to open its own PR/commit via a
  GitHub App identity. This project's entire point is the opposite: fixes must be
  spawned as `AutoReopenForPRFix` sessions running in *this app's own* tmux +
  git-worktree infrastructure (`session/backlog_lifecycle.go:49-53`), so they
  inherit the backlog item's context, acceptance criteria, prior session history,
  and reporting back into `BacklogStuckState`/toast notifications. A SaaS bot has
  no hook into any of that — it would at best be a second, uncoordinated actor
  pushing commits to the same branch a stapler-squad session might be mid-rework
  on, which is a correctness hazard (two independent commit-and-force-push actors),
  not a convenience.
- Requirements explicitly scope this to "no new GitHub App/webhook integration...
  poll-based, matching the rest of the pipeline" — a SaaS replacement is a webhook
  integration by construction, directly contradicting the stated constraint.
- Would still need the exact same reconciliation logic (map "external tool acted on
  this PR" back to backlog item state) to close the loop, so it doesn't remove the
  custom code, it just adds a second system in front of it.

**Verdict: Not recommended.** Incompatible with the existing
`AutoReopenForPRFix`/worktree-session architecture and with the stated no-webhook
constraint — this isn't a cost/fit tradeoff, it's a structural mismatch.

## 3. LLM-generated dedup/staleness logic vs. a library

The dedup problem here is narrower than generic "GitHub API pagination": it's "has
backlog item X already reacted to review/comment ID Y." No pagination is in play —
`gh pr view --json reviews,comments` returns the full list in one call (no cursor),
and this repo's usage is single-repo/single-actor scale.

More importantly, **this repo already has a battle-tested, in-house pattern for
exactly this class of problem** — the `BacklogStuckState`
notify-once-dedup-with-backoff mechanism (`domain.StuckReason` +
`NotifiedAt`/`RemediationDue`, `session/ent_repository_backlog.go:1027-1250`) is
used by ~6 other detectors in `backlog_lifecycle.go` and has dedicated regression
tests (`session/backlog_lifecycle_stuck_test.go`, e.g.
`TestPushAndCreatePR_RepeatedPushFailure_DedupsToast`). The correct move for the
new signal isn't "write a new bespoke algorithm," it's "add one more field
(e.g. a comparable marker: highest-seen comment/review `id`, or a
content hash of the concatenated COMMENTED-review bodies) to `backlog_item`'s ent
schema and reuse the existing `MarkStuck`/`RemediationDue` gate," which is squarely
in the "no meaningful correctness risk" category — it's the same
compare-a-persisted-marker-against-a-freshly-fetched-value shape already proven out
for `pr_needs_fix`/`pr_ready_unmerged`/`abandoned_review`.

Two real edge cases worth naming (not blockers, just test-worthy):
- **GitHub review/comment IDs, not timestamps**, should be the dedup key — clock
  skew between the GitHub API server and this app's poll loop is a real but
  avoidable risk if a `createdAt` timestamp comparison were used instead. `gh pr
  view --json reviews` includes stable review IDs; comments likewise. Prefer
  "highest ID seen" or a set of seen IDs over a timestamp comparison.
- **A human amending/re-requesting an already-actioned COMMENTED review** doesn't
  produce a new review ID on GitHub (there's no "dismiss" for COMMENTED, as the
  requirements note) — so the dedup key must be per-review-id, not "any COMMENTED
  review exists," or a fixed COMMENTED review would trigger a fix loop exactly
  once and then silently stop mattering even if the PR changes substantially later.
  Using the review `id` (not just its existence) as the marker avoids this, since a
  genuinely new round of Copilot feedback after new commits still gets a new
  review ID.

**Verdict: Recommended** to write this in-house, reusing the existing
`BacklogStuckState` dedup convention rather than reaching for a library — there
isn't a Go library that specifically does "dedup GitHub review/comment IDs against
a persisted marker" (it's a 5-line set/comparison, not a domain with known-hard
edge cases like OAuth token refresh or rate-limit backoff), and importing one would
just be an abstraction wrapping a comparison this codebase already has a proven
pattern for.

## 4. Fork/adapt prior art for the dedup mechanism itself

Searched for GitHub's own Copilot review tooling and open-source "auto-address
PR comments" bots specifically for dedup approaches worth studying:

- **[`k1LoW/gh-copilot-review`](https://github.com/k1LoW/gh-copilot-review)** — a
  `gh` CLI extension that requests a Copilot code review with explicit "duplicate
  prevention, outdated review cleanup, and wait-for-completion support." This is
  the closest prior art to requirement scope item 3 ("wiring an auto-request for a
  Copilot review into the PR-creation flow") — worth reading its source for how it
  avoids requesting a second Copilot review when one is already pending/fresh,
  since that's the same "don't re-trigger on unchanged state" shape as the fix-loop
  dedup. Not worth depending on as a binary (it's a CLI wrapper, and this project
  needs the logic embedded in `pushAndCreatePR`, not shelled out to yet another
  external tool), but its README/source is a useful reference for the actual
  request-Copilot-review implementation task.
- **GitHub CLI native support**: as of `gh` 2.88.0 (2026-03-11, [GitHub Changelog
  post](https://github.blog/changelog/2026-03-11-request-copilot-code-review-from-github-cli/)),
  `gh pr edit --add-reviewer @copilot` and `gh pr create --reviewer @copilot` work
  directly — no extension needed. **Caveat confirmed against this machine**: `gh
  --version` here reports `2.86.0`, which predates that feature. The
  Copilot-review-request implementation task should verify/require `gh` ≥ 2.88 (or
  fall back to `gh api repos/:owner/:repo/pulls/:number/requested_reviewers -f
  'reviewers[]=copilot-pull-request-reviewer[bot]'`, the raw REST endpoint the CLI
  feature wraps, which works on older CLI versions since it's just a generic `gh
  api` call, not dependent on the 2.88 parsing/UX layer).
- **[`pbakaus/agent-reviews`](https://github.com/pbakaus/agent-reviews)** — polls
  the GitHub API for new PR review comments, filters for "unresolved" /"unanswered
  bot comments," and has a watch mode with an inactivity timeout. Conceptually the
  closest analog to the polling+dedup half of this feature (comment-level, not
  review-level), though it's a terminal tool for feeding an AI agent interactively,
  not a backend automation loop — worth skimming for its "what counts as already
  answered" heuristic, but not adoptable as a dependency (no Go client, different
  execution model).
- **[`tag1consulting/ai-pr-review`](https://github.com/tag1consulting/ai-pr-review)** —
  a GitHub Action that "auto-resolves stale bot threads and dismisses superseded
  reviews." Confirms dismissal-by-supersession (new review invalidates the old
  one) is a known pattern elsewhere, which validates the id-based (not
  existence-based) dedup approach recommended in §3 above.

**Verdict: Viable, but as reading material, not a dependency.** None of these are
worth forking or depending on directly (wrong language ecosystem for
`agent-reviews`/`ai-pr-review`, and `gh-copilot-review`'s functionality is now
native to `gh` CLI ≥ 2.88 anyway) — but `gh-copilot-review`'s duplicate-prevention
logic and `ai-pr-review`'s dismiss-on-supersede pattern are useful references for
the implementation task, and confirm the id-based dedup approach in §3 is the
right shape rather than a novel design.

## Summary Table

| Option | Verdict |
|---|---|
| Add `go-github`/`githubv4` for the new signal | Not recommended — signal already in the existing `gh pr view` payload; would fragment the data source |
| SaaS PR-review-bot replacement | Not recommended — incompatible with `AutoReopenForPRFix`'s worktree/tmux-session architecture and the explicit no-webhook constraint |
| Bespoke dedup/staleness logic | Recommended — reuse the existing `BacklogStuckState` notify-once-dedup pattern; use review/comment IDs, not timestamps, as the marker |
| Study `gh-copilot-review` / `ai-pr-review` dedup approach | Viable as reference material; `gh` CLI ≥ 2.88 now natively supports `--add-reviewer @copilot` (local `gh` here is 2.86 — verify/upgrade or use the raw `gh api` reviewer-request endpoint as a fallback) |
