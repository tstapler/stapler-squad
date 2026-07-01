# Requirements: GitHub Work Continuity

**Status**: Draft | Phase 1 — Ideation complete
**Created**: 2026-06-24
**Supersedes**: `docs/tasks/github-pr-status.md` (planning complete, merged into this unified plan)
**Builds on**: `project_plans/find-unfinished-work/requirements.md`, `project_plans/branch-resume-unfinished-tab/requirements.md`

---

## Problem Statement

Stapler Squad already surfaces local unfinished work: uncommitted changes, branches ahead of
main, dormant worktrees. But the developer's actual work-in-progress state lives in two places —
local git and GitHub — and the two are completely disconnected in the current UI.

The result:
- A session card shows a branch name, but gives no signal about whether its PR needs attention
  (changes requested, CI failing, ready to merge).
- The Unfinished tab shows local changes but has no visibility into open PRs that are waiting
  for follow-up.
- Recent merged PRs are invisible — there's no "what did I finish recently" view to use as
  context when picking up a related task.
- The user must open GitHub in a browser, cross-reference branch names manually, and hold the
  full PR↔session↔worktree mapping in their head.

**Primary user**: Tyler — solo developer, 3–15+ concurrent AI-assisted sessions across multiple
repos and branches.

---

## Triad Review: Existing Unfinished Work Specs

This section is the product/tech/UX triad review of the two existing requirements documents.

### find-unfinished-work requirements review

**Strengths:**
- Excellent scope definition: watch dirs + auto-spider + pinned repos covers real discovery needs.
- Dismiss/snooze lifecycle is well-specified.
- AI summary on demand is a meaningful differentiation over raw git output.
- Background scan + fsnotify refresh cadence is pragmatic.

**Gaps / concerns:**

| # | Area | Gap |
|---|------|-----|
| G1 | GitHub | No mention of GitHub PR status. An item "ahead of main" with an open PR waiting on CI failure is *higher priority* than one with no PR — but the current spec treats them identically. |
| G2 | GitHub | No view of PRs that exist but have no corresponding local worktree (e.g. PR opened from another machine, or a PR the user created weeks ago). These are invisible in the current design. |
| G3 | GitHub | "Could Have: GitHub PR status shown on items that have an associated open PR" — this was deprioritized, but it's actually load-bearing for the feature's primary goal (prioritizing what to work on next). |
| G4 | Sessions | `ScanResult.SessionIDs []string` is defined in the scanner, but it's unclear whether the UI actually surfaces this linkage (which sessions are for this worktree). |
| G5 | UX | The "Commit & push shortcut" is a high-risk one-shot operation (`git add -A && git commit && git push`) with no preview of what's being committed. Needs a diff review step before execution. |
| G6 | UX | No entry point from a session card to its Unfinished item — navigation is one-way (Unfinished → open session) with no reverse link. |
| G7 | Tech | No spec for how `SessionIDs` is populated at scan time — the scanner runs in isolation, it doesn't have access to the session store's in-memory `Path` field. Needs explicit wiring spec. |
| G8 | Tech | `git rev-list main..HEAD` hardcodes "main" as default branch — repos using "master" or "develop" as default will show all branches as "ahead". `ResolveDefaultBranch()` exists but isn't mentioned. |

### branch-resume-unfinished-tab requirements review

**Strengths:**
- Correctly identifies the gap: dormant local branches (ahead, no session) are invisible.
- Resume button with omnibar pre-fill is the right UX — doesn't auto-create, gives user control.
- Open Questions section is honest about the unknowns (scanner extension vs. new enumerator).

**Gaps / concerns:**

| # | Area | Gap |
|---|------|-----|
| B1 | GitHub | Dormant branches that have open PRs are the highest-value items in this list — yet PR status is "out of scope". This inverts the priority ordering. |
| B2 | GitHub | A dormant branch with a merged PR is *finished work* — it should not appear in the Unfinished tab. The spec has no mechanism to filter these out. |
| B3 | Scope | "Remote-only branches" are out of scope — but branches I created from another machine that I want to continue locally are exactly the kind of unfinished work this feature should surface. This deserves a "Could Have". |
| B4 | UX | The visual design question (interleaved vs separate section) is unresolved. A "Dormant Branches" section heading is needed to distinguish from worktree-level items. |
| B5 | Tech | Open Question on `ResolveDefaultBranch()` is answered in the code: `git_vcs_reader.go` uses it. The spec should be updated to reference it. |

---

## Success Criteria

1. **Session cards surface PR status** — every session whose branch has an open GitHub PR shows
   a color-coded status badge (blocking / ready / pending / draft / complete) within 60 seconds
   of state change. Sessions without a PR show a repo/branch badge unchanged.

2. **Unfinished tab shows GitHub PRs** — a "GitHub PRs" section in the Unfinished tab lists:
   - All open PRs authored by the user, with PR priority badge (blocking / ready / pending)
   - The 5 most recently closed/merged PRs authored by the user (work history context)
   - Each item links to a session if one exists for that branch; shows "Start session" if not

3. **PR enriches unfinished worktree items** — when a worktree item in the Unfinished tab has
   an associated GitHub PR, the PR priority badge is shown on that item's card. A merged/closed
   PR causes the worktree item to be deprioritized (shown at bottom, grayed out).

4. **GitHub account is auto-detected** — no manual configuration required. The tool derives the
   GitHub user via `gh api user --jq .login`. Multiple accounts / orgs are not required.

5. **Zero new browser tabs needed** — the user can determine which PR needs attention, see
   review status and CI results, and navigate to the right session entirely within Stapler Squad.

---

## Scope

### In Scope (Must Have)

**GitHub PR status on session cards** (replacing `docs/tasks/github-pr-status.md`)
- Background `PRStatusPoller` discovers PR for each session's branch, polls every 60 seconds
- ETag-conditional polling to minimize GitHub API rate limit consumption
- Priority derivation: blocking (changes requested / CI failure), ready (approved + CI pass),
  pending (awaiting review / CI running), draft, complete (merged/closed), no_pr
- Color-coded `GitHubBadge` component on session cards with accessible tooltip
- Auto-discovery of PR from branch name via `GetPRForBranch()`

**GitHub PRs section in Unfinished tab**
- New collapsible section "GitHub PRs" at the top of the Unfinished Work tab
- Lists open PRs authored by the authenticated `gh` user, fetched on page load + 5-minute refresh
- Each PR card shows: PR number, title, repo, priority badge, review counts, CI status, last updated
- Clicking a PR card expands to show: PR description excerpt, review status detail, CI check list,
  associated session (if branch is open in a session) or "Start session" button
- "Recent" subsection: 5 most recently merged/closed PRs as compact history rows (no expansion)

**Worktree item enrichment**
- When the scanner's `ScanResult` for a worktree matches a known PR (by branch name), enrich
  the Unfinished item card with the PR priority badge
- Worktrees with merged/closed PRs are moved to a "Completed / Low priority" section at the
  bottom of the tab and shown grayed out
- Fix G7: explicit wiring between `PRStatusPoller` discovered PR data and the scanner results

**GitHub account detection**
- On server start, call `gh api user` to determine the authenticated user's login
- Store in server config / session context for use by poller and PR list fetcher
- Surface auth state: if `gh` is not authenticated, show a dismissible banner in the Unfinished
  tab and disable the GitHub section (do not crash or show empty state without explanation)

**Triad review gap fixes**
- Fix G5: "Commit & push" shortcut shows a file diff preview modal before executing
- Fix G8: All "ahead of main" checks use `ResolveDefaultBranch()` instead of hardcoded "main"
- Fix B2: Dormant branch cards disappear when their PR is merged/closed (poller triggers refresh)

### Should Have

- Fix G6: Session card has a "View in Unfinished" link that deep-links to that worktree's item
- Fix B4: Dormant branches render in a distinct "Dormant Branches" section (not interleaved)
- Fix B3 (Could Have → Should Have): "Remote-only branches" with open PRs visible in GitHub PRs section

### Could Have

- PR review requests: PRs where the user is a requested reviewer (separate sub-section)
- Staleness aging: items not touched in 30+ days are visually de-emphasized
- Sort within GitHub PRs section: by priority (blocking first), then by last updated

### Out of Scope

- Multiple GitHub accounts / organizations
- GitHub Issues (only PRs)
- Cross-device sync or remote state
- PR review submission (read-only view only)

---

## Constraints

- Go backend + React frontend + ConnectRPC — no new frameworks
- GitHub data sourced exclusively via `gh` CLI (already installed for auth) and GitHub REST API
  via the existing HTTP client in `github/http_client.go`
- Rate limit budget: combined polling for N sessions at 60s interval must stay under 60
  requests/hour per authenticated user (handled by ETag conditional requests)
- The `session/unfinished/` package scanner runs independently of the `github/` package —
  enrichment must happen at the API/proto layer, not inside the scanner
- The existing `docs/tasks/github-pr-status.md` plan is superseded; any partially-implemented
  work from branch `claude-squad-pr-integration` should be evaluated and either merged or
  intentionally dropped
- Incremental delivery — each story must be independently releasable without breaking existing
  session card behavior

---

## Open Questions

1. The `docs/tasks/github-pr-status.md` branch `claude-squad-pr-integration` — how much of
   Stories 1–2 (backend) is already merged to main? Need a `git log` audit before planning.

2. For the "GitHub PRs" section, should open PRs from all repos the user has ever touched be
   listed, or only repos that have a tracked session/worktree in Stapler Squad?
   → Default assumption: **all repos** (use `gh pr list --author @me --state open`), with a
   future filter-by-repo option.

3. The `ScanResult.SessionIDs` field exists — is it currently populated via the scanner, or is
   it a stub? Need to verify in `scanner.go` before planning the enrichment approach.
