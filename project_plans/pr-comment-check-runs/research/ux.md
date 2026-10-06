# UX Research: Reduce PR/Issue Comment Noise, Prefer Check Runs

Agent 5 (UX Research), SDD research phase for `project_plans/pr-comment-check-runs/`.

## 1. Comparable UX patterns (industry)

Direct search results for exact bot behavior (Renovate/Dependabot/CodeRabbit) were thin on
specifics — most public docs describe *what* the bots report, not the check-run-vs-comment
split mechanics. What is VERIFIED from the GitHub Checks API docs and general practice:

- **Checks API `output` object** (VERIFIED — [GitHub Docs: Building CI checks with a GitHub
  App](https://docs.github.com/en/apps/creating-github-apps/writing-code-for-a-github-app/building-ci-checks-with-a-github-app)):
  `title` and `summary` are required; both `summary` and `text` support Markdown up to 64K
  characters. `text` is for expandable detail (rendered behind a "Details" disclosure on the
  Checks tab), `summary` is the always-visible blurb. `annotations` (up to 50 per update,
  appended not replaced) attach inline notices/warnings/failures to specific file/line
  ranges — this is how a check run can point at a specific line without a review comment.
  `conclusion` is one of `success | failure | neutral | cancelled | timed_out | action_required
  | skipped` (plus `stale` as a historical value) — `neutral` is the state a bot without a
  clear pass/fail should use rather than forcing a false success/failure.
- **General industry pattern** (INFERRED from the surveyed docs/blog posts, not a single
  authoritative source): status-only signals (CI ran, lint passed, dependency compatible)
  go to a check run so they show up as a pass/fail badge next to the merge button and don't
  clutter the timeline; anything requiring human judgment or a decision (a flagged
  vulnerability needing triage, a suggested code change, "this PR conflicts with X") goes to
  a comment or inline review comment because check-run output isn't independently
  notifiable/actionable the way a comment (with @mention, reaction, reply-thread) is.
- Renovate's **Dependency Dashboard** ([docs.renovatebot.com](https://docs.renovatebot.com/key-concepts/dashboard/))
  is itself a variant of this principle at the repo level: instead of posting a comment
  per pending/blocked update, it maintains a single persistent issue with a checklist, and
  Renovate's own "pending status checks" state is explicitly a status-check gate, not a
  comment ([renovatebot/renovate discussion #21720](https://github.com/renovatebot/renovate/discussions/21720)).
  This is the same shape as this project's requirement: collapse many potential
  notifications into one durable, glanceable surface.
- CodeRabbit's pattern (INFERRED, [coderabbit.ai docs](https://docs.coderabbit.ai/faq) and
  [product blog](https://www.coderabbit.ai/blog/how-coderabbit-delivers-accurate-ai-code-reviews-on-massive-codebases))
  is the inverse split: it edits the **PR description** for the persistent summary/walkthrough
  (not a new comment each time — avoids re-notifying on every push) and posts **new comments
  only for actionable, line-specific findings**. That "edit-in-place for status, new comment
  only for actionable item" pattern maps directly onto this project's Acceptance Criterion 2
  ("remaining comments require attention").

**Gap named explicitly**: I could not verify the exact `conclusion`/`output` payloads
Renovate or Dependabot emit today (their bots are closed implementation details, not
documented at that level of granularity) — the above is grounded in the public Checks API
contract and observable bot behavior patterns, not decompiled bot source.

## 2. User mental model / scan path

For a single developer (Tyler) triaging many concurrent AI-agent PRs, the fastest scan path
on GitHub's native PR list/PR page is (INFERRED from GitHub's own UI layout, which puts the
check-status icon to the left of the PR title in list view and the Checks tab + merge-box
summary at the top of a PR page, ahead of the comment thread):

1. **Check badge** (green check / red X / yellow dot) — binary "does this need CI attention"
   signal, visible without opening the PR (PR list view shows it inline).
2. **Comment count** — visible in the same list row; today, if every automation run posts a
   comment, this number is inflated and uninformative (a PR with 14 comments could be "14
   routine progress updates" or "1 real blocker + 13 noise" — indistinguishable at a glance).
3. **Diff** — only opened once 1 and 2 signal something's actionable.

Collapsing "N automation comments" to "1 check run + 0-1 comments" directly shortens step 2:
a nonzero comment count becomes a reliable predicate for "open this PR," rather than
requiring the user to open every PR with >0 comments to find out whether any of them matter.
This is exactly the AC4 requirement ("glance at checks + comment count and know if PR needs
anything"). The value is proportional to PR volume — for a single PR this is a minor
convenience; for the many-PRs-concurrently model this repo's own backlog automation implies
(`docs/registry/`, `session/pr_status_poller.go` polling many sessions' PR state), it's the
difference between a scannable queue and one that requires opening every item.

## 3. Does stapler-squad's web UI already surface check-run status or comment counts?

**Check-run/CI status: yes, already surfaced.** VERIFIED via grep and read:

- `web-app/src/lib/vcs/types.ts` — `GithubSummary.checkConclusion: CheckConclusion` where
  `CheckConclusion = "success" | "failure" | "pending" | ""` — already a first-class field on
  the VCS widget data model.
- `web-app/src/lib/vcs/mergeability.ts` (`deriveMergeabilityState`) — already branches PR
  state into `ci_failing` / `ci_pending` / `ready_to_merge` etc. using `checkConclusion`,
  independent of comments.
- `web-app/src/components/sessions/CIStatusBadge.tsx` — a dedicated, already-shipped badge
  component (`// +feature: session:ci-status-badge`) rendering Passing/Failing/Pending/"No
  checks" with icon + color variant, linking to `${prUrl}/checks`. This is driven by
  `checkConclusion` sourced from `github/client.go`'s `getCheckConclusion(resp.StatusCheckRollup)`
  (the aggregate `gh pr view --json statusCheckRollup` rollup, i.e. all check runs + legacy
  statuses combined into one conclusion) — see
  [`github/client.go:289`](github/client.go#L289) and
  [`session/pr_status_poller.go:392`](session/pr_status_poller.go#L392) for the poll→propagate
  path into `Instance.GitHubCheckConclusion`.
- `session/backlog_plugin_github_prs.go`'s `fetchCILabel` separately calls the check-runs API
  directly (`/repos/{owner}/{repo}/commits/{sha}/check-runs`) to derive a `pr:ci-failing`
  label for backlog items — a second, independent path that already consumes check-run data
  form the same underlying source.

**Comment count: no, this is a gap.** Grepped `web-app/src` for `commentCount`,
`comment_count`, `numComments`, `reviewCommentCount` — zero matches outside test files, and
zero matches for "comment" in `GitHubPRsSection.tsx` or `VcsWidgetGithubRow.tsx`, the two
components most likely to show it. **Nothing in the current web UI displays PR/issue comment
count or distinguishes "N routine" from "N actionable" comments.**

**Implication for this feature**: the check-run half of this requirement gets a UI payoff for
free — once check-run-based status conclusions exist for whatever automation currently posts
status comments, `CIStatusBadge`/`deriveMergeabilityState` can very likely surface them
without new UI work, provided the new check run's `conclusion` maps cleanly onto the existing
`CheckConclusion` enum (it already aggregates via `statusCheckRollup`, so a well-named new
check run should just show up). The comment-reduction half has **no existing UI affordance**
to show "comment count" as a triage signal today — if AC4 ("glance at checks + comment count")
is meant to apply to stapler-squad's own web UI (not just github.com), a new UI element would
be net-new scope, not a reuse of existing plumbing. Flagging this as an open question for
planning: is AC4's "glance" happening on github.com (already works, zero UI work) or in
stapler-squad's own UI (requires new work)? The requirements doc doesn't specify surface, and
existing session/PR data models (`GithubSummary`) have no comment-count field to reuse.

## 4. Error states — check-run creation fails silently

Framed via jobs-to-be-done for a solo user:

- **Functional job**: "let me triage N parallel PRs fast without reading every comment."
- **Emotional job**: "trust that automation would speak up (via a comment) if something
  needed me, and that a clean check state means I can safely ignore this PR for now."
  This is the load-bearing job for the error-state question — the whole value proposition
  of "no comment = nothing to see" collapses if a failed check-run write can look identical
  to "all good."
- **Social job**: N/A — solo user, no reviewer-facing audience to manage.

**Recommendation (INFERRED, not yet validated against a decision from the user)**: silent
degradation to no-signal-at-all is **not** acceptable for this feature's own emotional job.
A `checks:write` scope failure (or any check-run API error) that is swallowed produces the
worst-case outcome this feature exists to prevent: a PR that looks clean (no comment, no
check badge, or a stale/missing check) but actually has unreported status, and the user has
no way to distinguish "genuinely nothing to report" from "reporting broke." Given the design
already imposes "comment = something needs you," the natural, consistent fallback is: **on a
confirmed check-run write failure, fall back to posting a single comment** (e.g. "Unable to
post check status — see logs" with the actual status/error inline) rather than dropping the
signal. This keeps the comment channel's meaning intact (comment = you need to look at this)
and reuses the exact mechanism this feature is designed around, rather than inventing a third
notification channel. This should be a stated behavior in whatever "written convention"
Acceptance Criterion 1 asks for, not left as an implicit assumption — flag for the planning
phase.

**Open question for planning phase**: should the fallback comment be deduplicated/updated in
place (edit existing "check status unavailable" comment) the same way `forwardSyncCloseComment`
posts a single one-shot comment (`server/services/backlog_github_forward_sync.go:32`,
`server/services/backlog_github_forward_sync.go:152` — `PostIssueComment`), to avoid the
fallback path itself becoming a new source of comment noise on a persistently-broken token?
Not resolved here; AC5 only guarantees `forwardSyncCloseComment` itself is unaffected by this
feature, it doesn't specify the new fallback comment's dedup behavior.

## Sources

- [GitHub Docs: Building CI checks with a GitHub App](https://docs.github.com/en/apps/creating-github-apps/writing-code-for-a-github-app/building-ci-checks-with-a-github-app)
- [Creating GitHub Checks (and Understanding the Checks API) — Ken Muse](https://www.kenmuse.com/blog/creating-github-checks/)
- [Renovate Docs: Dependency Dashboard](https://docs.renovatebot.com/key-concepts/dashboard/)
- [renovatebot/renovate discussion #21720 — pending status checks](https://github.com/renovatebot/renovate/discussions/21720)
- [CodeRabbit Docs FAQ](https://docs.coderabbit.ai/faq)
- [How CodeRabbit delivers accurate AI code reviews on massive codebases](https://www.coderabbit.ai/blog/how-coderabbit-delivers-accurate-ai-code-reviews-on-massive-codebases)
