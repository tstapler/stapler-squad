# Requirements: GitHub PR Status Integration

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-09

## Problem Statement

Stapler Squad users manage multiple AI agent sessions concurrently, each working on a different
git branch. When sessions complete work and push code, PRs open on GitHub - but users have no
visibility into PR status from within Stapler Squad. This forces context switching to GitHub to
answer "which sessions are actually done?" and "which PRs need my attention?"

The secondary problem is that AI agents (Claude) don't know when their PR has been merged or
closed, so they may continue running unnecessarily or ask users to take actions that are already
complete.

**Primary users:** Developers running 3–10+ concurrent AI sessions across branches.

## Success Criteria

**Phase 1 (initial - this project):**
- Every session with a branch that has an open PR shows PR status in the web UI
- Status is visible on session cards without clicking into the session
- Status includes: open / draft / needs-review / approved / changes-requested / merged / closed / CI-failing
- Status refreshes automatically (background polling) — not stale after page load
- Branch-based sessions (not just PR-URL-created sessions) get PR status auto-detected

**Phase 2 (future, out of scope here):**
- MCP server endpoint exposes PR status so AI agents can query their own PR state
- Claude can be told "your PR is merged, you're done" via MCP

## Scope

### Must Have (MoSCoW)
- Auto-detect PR for any session by matching its git branch against open PRs (`gh pr list --head <branch>`)
- Derive GitHub owner/repo from git remote URL for sessions that weren't created from a PR URL
- Persist PR number and URL on Instance after auto-detection (so it survives restarts)
- Background polling: refresh PR status every N minutes (configurable, default ~5 min)
- Enriched PR status beyond what's currently stored: CI/checks status, review decision (approved/changes-requested), review count
- Update GitHubBadge component to show status with color/icon (e.g., green=merged, yellow=needs review, red=changes requested / CI failing)
- Status updates propagate to web UI via existing ConnectRPC streaming (no new transport needed)

### Out of Scope
- Creating, merging, or commenting on PRs from the web UI
- Multi-repo sessions (single git remote per session is assumed)
- Automatically stopping/killing AI sessions when PR merges (Phase 2)
- Showing full PR review comments inline
- Webhook-based push updates (polling is sufficient for Phase 1)
- GitHub Enterprise or non-GitHub VCS (GitLab, Bitbucket)
- MCP server endpoint (Phase 2)

## Constraints

**Tech stack:**
- Go backend (all polling/fetch logic must be in Go)
- `gh` CLI as auth method (already in use for PR operations, zero-config for existing users)
- React/TypeScript frontend with vanilla-extract CSS (per ADR 009)
- No new Go module dependencies for the PR polling backend
- Session proto fields already defined in `proto/session/v1/types.proto` (extend, don't replace)

**Existing work on this branch (`claude-squad-pr-integration`):**
- `github/client.go`: `GetPRInfo()`, `CheckGHAuth()` via `gh` CLI — reuse as-is
- `session/instance.go`: `GitHubOwner`, `GitHubRepo`, `GitHubPRNumber` fields already present
- `session/pr_tracking.go`: `RefreshPRInfo()` already exists — reuse in poller
- `session/github_metadata.go`: `GitHubMetadataView` value object
- `web-app/src/components/sessions/GitHubBadge.tsx`: badge shows PR # + link — extend for status
- Proto `types.proto`: `PRInfo` message + `github_*` fields on `Session` — add status fields

**Gap (what doesn't exist yet):**
- Branch-to-PR auto-discovery (`gh pr list --head <branch>` lookup)
- Background polling goroutine with per-session refresh scheduling
- CI/checks status fetching (`gh pr checks <pr>`)
- Review decision status (approved/changes-requested)
- Status fields in proto and on Instance
- GitHubBadge color/status rendering

**Dependencies:**
- `gh` CLI installed and authenticated (`gh auth status` — already checked by `CheckGHAuth()`)
- Git remote URL parseable to owner/repo (already done in `github/url_parser.go`)

## Context

### Existing Work
The `claude-squad-pr-integration` branch has significant groundwork:
- GitHub API client using `gh` CLI (not direct API calls)
- PR-URL-based session creation flow (create session from a PR URL, auto-clone, set branch)
- Static PR fields on Instance (stored at creation, not refreshed)
- `GitHubBadge` component in web UI showing PR # as a clickable link

What's missing is the **active tracking** side: discover PRs for branch-based sessions,
and keep status fresh via polling.

### Stakeholders
- Tyler (solo developer / primary user)
- Future: any Stapler Squad user managing concurrent AI sessions

## Research Dimensions Needed

- [x] Stack — `gh` CLI already chosen; no new dependencies needed; `gh pr list --json` for discovery
- [ ] Features — survey how comparable tools (Linear, GitHub Desktop, Raycast) show PR status
- [ ] Architecture — where does the poller live? session-level vs. workspace-level polling; cache strategy
- [ ] Pitfalls — `gh` auth failures gracefully; rate limiting (gh CLI handles this); stale/closed PRs
