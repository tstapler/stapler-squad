# Feature Research: GitHub Work Continuity — Comparable Tools

Research date: 2026-06-24
Task: Understand how existing tools surface GitHub PR data and "unfinished work" for developers.

---

## 1. GitHub CLI (`gh`) — `gh status` and `gh pr status`

### `gh status` (cross-repo dashboard)

`gh status` is the closest thing in the CLI ecosystem to a global developer dashboard. It prints information about your work on GitHub across **all repositories you're subscribed to**, including:

- Assigned Issues
- Assigned Pull Requests
- Review Requests
- Mentions
- Repository Activity (new issues/pull requests, comments)

Supports `--exclude` to drop repos and `--org` to scope to one org.

**Key limitation:** It does not surface CI status, review state, or staleness. It is an inbox-style list, not a prioritized attention queue.

Source: https://cli.github.com/manual/gh_status

### `gh pr status` (per-repo PR status)

`gh pr status` is scoped to the current repo (or a specific `--repo`). It outputs three sections:

- **Current branch** — the PR for the branch you're on, with CI and review state
- **Created by you** — your open PRs
- **Requesting a code review from you** — PRs waiting for your review

Example output:
```
Current branch
  #12 Remove the test feature [user:patch-2]
   - All checks failing - Review required

Created by you
  You have no open pull requests

Requesting a code review from you
  #13 Fix tests [branch] - 3/4 checks failing - Review required
  #15 New feature [branch] - Checks passing - Approved
```

**Key limitation:** Only one repo at a time, not cross-org. The `--repo` flag allows targeting a different repo but still one at a time.

Supports `--json` output with rich fields: `reviewDecision`, `statusCheckRollup`, `mergeable`, `isDraft`, `latestReviews`, `mergeStateStatus`, etc.

Source: https://cli.github.com/manual/gh_pr_status

### `gh-dash` — Third-party TUI extension

`gh-dash` (https://github.com/dlvhdr/gh-dash) is the most capable CLI-based PR dashboard. It is YAML-configured with named sections, each backed by a GitHub search filter query. Key design decisions relevant to Stapler Squad:

- **Named sections with GitHub search query filters** — users define exactly what they want to see (e.g., `is:open review-requested:@me`, `is:open author:@me`, `is:open -author:@me repo:myorg/myrepo`).
- **Smart repo filtering** — if launched from inside a git repo, it automatically adds `repo:owner/name` to sections that lack an explicit repo filter.
- **Multiple dashboards** — run with `--config <path>` to switch between work/personal dashboards.
- **Per-row status columns** — `updatedAt`, `author`, `reviewStatus`, `ci`, `lines`, `assignees`, `base`.
- **Template functions for relative dates** — `nowModify` calculates relative dates in ISO-8601 for queries like "updated in last 7 days."

Source: https://www.gh-dash.dev/configuration/pr-section/

---

## 2. Linear and Jira — Issue Trackers Linking Branches/PRs

### Linear: GitHub PR + Branch Integration

Linear's GitHub integration is bidirectional: branch naming drives issue linking, and PR events drive issue status.

**Branch → Issue linking:** Creating a branch from Linear provides a branch name like `eng-75-fix-link-bug`. When a developer uses this convention, the branch is automatically linked to the issue. Creating the branch can also auto-assign the issue and move it to a "Started" status.

**PR event → Issue status automation:**
| GitHub Event | Linear Issue Status |
|---|---|
| Branch created | Started + Auto-assigned |
| PR opened | In Progress (or custom, e.g., "In Review") |
| All PRs merged | Completed (or custom) |

Teams can customize which workflow status maps to PR open and PR merge, enabling distinct "In Code Review" states separate from "In Progress."

**"My Issues" view:** Assigned issues appear in a default "My Issues" view that updates automatically based on assignment. A developer can see all their work — including delegated items — in one place, with status directly reflecting PR state.

**Key insight for Stapler Squad:** Linear uses **branch naming conventions as the primary link between local work and issue tracking**. No plugin or IDE integration needed — the branch name itself is the integration point. Stapler Squad could use the same inference: detect branch name patterns to correlate sessions to issues without requiring explicit linking.

Sources: https://linear.app/changelog/2019-07-11-github-workflow-configuration, https://matthaliski.com/blog/ci-with-linear-github-and-xcode-cloud

### Jira: GitHub Development Panel Integration

Jira surfaces GitHub data via a development panel on each work item card. Linking is triggered by including the Jira issue key (e.g., `JRA-123`) in branch names, commit messages, or PR titles.

**What appears in Jira when linked:**
- Linked branches, pull requests, commits, builds, and deployments (in the development panel)
- PR status icons on board cards
- A "Development" page showing PRs from the last 30 days
- GitHub workflow runs displayed as "builds"

**Automated workflow transitions (built-in automation library):**
| Event | Jira Status |
|---|---|
| Branch created | In Progress |
| Commit made | In Progress |
| PR merged | Done |

**JQL for finding issues with open PRs:** You can query for issues that are "Done" but still have open PRs — a useful "work not fully finished" pattern.

**Key limitation:** The native integration is read-heavy (status display), not action-focused. It doesn't help a developer answer "what should I work on next" — it's a manager/team view, not a developer-facing continuity tool.

Sources: https://support.atlassian.com/jira-cloud-administration/docs/use-the-github-for-jira-app/, https://support.atlassian.com/jira-cloud-administration/docs/integrate-with-github/

---

## 3. VS Code GitHub Pull Requests Extension

### Sidebar Structure and UX Pattern

The extension adds an Activity Bar icon that opens a "Pull Requests and Issues" sidebar. The sidebar is organized into **named query groups**, each backed by a GitHub search query. Default groups:

| Sidebar Section | Default Query |
|---|---|
| Waiting For My Review | `is:open review-requested:${user}` |
| Assigned To Me | `is:open assignee:${user}` |
| Created By Me | `is:open author:${user}` |

Users can hover over any section header and click a pencil icon to edit the query, or add entirely new sections. This is the same query-per-section pattern as gh-dash.

**"Needs Attention" UX implementation:** There is no single "Needs Attention" panel. Instead, the default query groups implicitly define attention states through their names and ordering. "Waiting For My Review" is listed first — a strong signal that review obligation is considered the highest-priority attention state.

**2024 improvements:**
- PRs in sidebar views now show **status icons** (open, closed, merged, draft) configurable via `"githubPullRequests.pullRequestAvatarDisplay"`.
- The sidebar description panel collapses into a compact readonly view that can be expanded.
- All review action buttons appear in the Active Pull Request sidebar when there's enough space.

**Local work linking:** The extension detects the current branch and shows which PR (if any) corresponds to it — a "Current Branch" view equivalent. Clicking a file from a PR opens a diff editor against the base branch. This is the primary "local work ↔ PR" link in the VS Code UX.

Sources: https://marketplace.visualstudio.com/items?itemName=GitHub.vscode-pull-request-github, https://smashdev.com/2023/12/GitHub-Pull-Requests-and-Issues-VS-Code-Extension

---

## 4. Raycast GitHub Extension — "My PRs" View

### Official Extension: PR Sections

The official Raycast GitHub extension includes a **"My Pull Requests"** command with the following sections:

| Section | Description |
|---|---|
| Created (Open) | PRs you authored that are still open |
| Assigned (Open) | PRs assigned to you |
| Mentioned | PRs where you're mentioned |
| Review Requested | PRs where your review is requested |
| Reviewed By | PRs you have already reviewed |
| Recently Closed | Your recently closed/merged PRs |

Action Panel (`Cmd+K`) on any PR offers: merge, copy number, open in browser.

### "My Pull Requests Menu Bar" (Advanced / Inbox-style)

This is a menu bar app variant with a more sophisticated triage design:

- Configurable sections: **assigned / mentioned / reviewed / review-requested / drafts**
- Repository filter (restrict to specific repos)
- CI status icons per PR: draft, merge-queue, success, failure, pending
- Sortable list

**Key insight:** Raycast distinguishes between "Created" (your active authored PRs) and "Reviewed By" (PRs you've already reviewed — your obligation is done). This is a clean separation between "in-flight authored work" and "review obligation completed."

### Third-Party: GitHub Review Requests Extension

A community extension (https://www.raycast.com/resessh/github-review-requests) with a sharper focus on review triage:

- **"Wait For Merge"** section — your authored PRs that have been approved and are waiting for you to merge. This is a unique "unblocked but unfinished" state that most tools miss.
- **"New Request Review"** section — others' PRs that you haven't reviewed yet.
- Org/owner filter for focusing on workspace PRs.

**Key insight for Stapler Squad:** The "Wait For Merge" section explicitly models the state "approved but not yet merged" — a PR that is done from a review standpoint but unfinished from a shipping standpoint. This is a distinct attention state worth modeling.

Sources: https://www.raycast.com/raycast/github, https://www.raycast.com/resessh/github-review-requests

---

## 5. GitHub Native Notification Inbox

### Attention States in the Inbox

GitHub's notification inbox assigns each notification exactly one "reason" label representing why you received it. Possible reasons:

| Reason | Meaning |
|---|---|
| `review-requested` | You or a team you're on was requested to review |
| `assigned` | You were assigned to the PR/issue |
| `author` | You opened the PR/issue |
| `mention` | You were @mentioned |
| `comment` | You commented on the thread |
| `subscribed` | You manually subscribed |
| `participating` | Catch-all for indirect participation |

When you match multiple roles, GitHub picks one as the primary reason. The priority ordering is not publicly documented but empirically tends to favor the most action-requiring role (review-requested > assigned > author > mention).

**Filtering:** The inbox supports `reason:` query filters (e.g., `reason:review-requested`). These can be applied via the UI filter bar or by constructing direct URLs like `https://github.com/notifications?query=reason:assign`.

**Known pain points:**
- Cannot distinguish between "my review requested" and "team I'm on review requested."
- Merged/closed PRs remain in the inbox until manually cleared — no auto-archival.
- No "staleness" signal (e.g., PR opened 3 weeks ago, no activity).
- The inbox is notification-centric (events), not state-centric (current PR status). A PR approved after you last checked will still show a stale notification.

Sources: https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications, https://docs.github.com/en/subscriptions-and-notifications/reference/inbox-filters

---

## 6. PR Work History: "Recently Merged" and Why It Matters

### Why Work History Is Valuable

Several tools surface recently merged/closed PRs as a first-class feature:

- **Raycast GitHub extension** includes a "Recently Closed" section in "My Pull Requests" — described as "a perfect source of truth for your daily update."
- **Graphite** auto-creates an "Merging and recently merged" section in its PR inbox.
- **GitHub's own notification inbox** surfaces closed PR notifications if you were subscribed, but they don't auto-expire.

### What "Recently Merged" Enables

1. **Daily standup / async update material:** A developer can answer "what did I ship yesterday?" from the recently merged list without manually tracking work.
2. **Context recovery after breaks:** After a vacation or context switch, recent merges show what was last completed, helping reorient to what's still open.
3. **Branch cleanup cue:** Recently merged PRs indicate branches that can be deleted locally. Some tools (e.g., VS Code extension's "Merged branches" detection) use merge history to prompt cleanup.
4. **Team-level "what shipped" digest:** For leads reviewing team output, merged PRs in a time window provide a factual list of completed work.

### Graphite's PR Inbox Sections (Most Comprehensive Model Found)

Graphite (https://graphite.dev) implements the most complete PR attention taxonomy found in research:

| Section | Meaning |
|---|---|
| Needs your review | Review requested, not yet acted on |
| Approved | You have approved this PR |
| Merging and recently merged | In merge queue or recently merged |
| Waiting for CI | Blocked on CI |
| Changes requested | You requested changes, author has not responded |
| Drafts | Your draft PRs |

This taxonomy is notable because it models **both sides of the review loop** — not just "needs review" but "you approved, now waiting on author" and "you requested changes, waiting on author."

Source: https://graphite.com/guides/viewing-github-pull-request-history

---

## Cross-Tool UX Pattern Summary

### Pattern 1: Role-based section taxonomy (universal)

Every tool researched uses the same foundational taxonomy of 4–7 named sections based on your relationship to the PR:

- **Authored / Created** — your open PRs
- **Review Requested** — your review is pending
- **Assigned** — assigned to you
- **Mentioned** — you are referenced
- **Reviewed By / Already Reviewed** — your review obligation is complete
- **Recently Closed / Merged** — historical record

The key design question is ordering (which section is "most urgent") and what CI/review state is shown inline. gh-dash and VS Code let users reorder; Raycast and GitHub's inbox have fixed orderings.

### Pattern 2: Branch-as-local-work-link (used by Linear, VS Code, gh pr status)

The branch you currently have checked out is the bridge between "what I am doing locally" and "what is happening on GitHub." Every tool that surfaces "current work" (vs. "all work") uses the current branch as its anchor:

- `gh pr status` shows the current branch PR at the top.
- VS Code PR extension shows which PR corresponds to the current branch.
- Linear detects branch name patterns to link to issues.

This is the low-cost, no-configuration approach to local-work linking.

### Pattern 3: Action-state beyond "open/closed" (best practice, partially implemented)

The most sophisticated tools (Graphite, the Raycast Review Requests extension) model intermediate action states that are not captured by GitHub's native open/closed PR state:

| State | Meaning | Tool that models it |
|---|---|---|
| Approved but not merged | Review done, author needs to act | Raycast "Wait For Merge," Graphite "Approved" |
| Changes requested, author silent | Reviewer is blocked on author | Graphite "Changes Requested" |
| CI blocked | Waiting on external signal | Graphite "Waiting for CI," Raycast menu bar CI icons |
| Draft | Author not ready for review | Raycast, gh-dash |

Most tools (gh, VS Code, GitHub inbox) do not expose these intermediate states at the section level — they show review state as a text label on individual PRs but do not group by it.

---

## Gaps Identified Across All Tools

1. **No tool cross-links local worktree/session state to PR status.** All tools use the current branch as the "local work" signal, but none know about uncommitted work, session history, or how long a branch has been idle.
2. **"Unfinished" is not modeled as a first-class concept.** Tools show open PRs and review state, but no tool answers "which of my PRs have been sitting idle for N days?" as a surfaced section (gh-dash can approximate it with date filters, but it requires user configuration).
3. **Work history is shallow.** "Recently merged" is shown but typically limited to the last 5–10 items and not linked back to what was worked on locally. No tool surfaces "you merged this PR from this branch on this machine last week" as context for current sessions.
4. **PR → issue linkage is project-manager-facing.** Linear and Jira link PRs to issues, but the "My Work" views in those tools are issue-centric, not PR-centric. A developer thinking in terms of branches and PRs has to mentally translate to issues to use these views.
5. **Review obligation loop is incomplete.** Most tools show "review requested" but not "I reviewed and requested changes — has the author responded yet?" This is a common source of forgotten work.
