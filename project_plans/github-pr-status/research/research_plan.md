# Research Plan: GitHub PR Status Integration

Date: 2026-04-09
Input: project_plans/github-pr-status/requirements.md

## Scope

3 parallel research dimensions. Stack is already resolved (gh CLI, no new deps).

---

## Dimension 1: Features

**Question:** How do comparable tools surface PR status? Which status states matter most to developers managing concurrent work?

**Search strategy:**
1. "GitHub PR status display developer tools" — what statuses are most actionable
2. "Linear GitHub PR sync status display" — how Linear surfaces PR info in issues
3. "Raycast GitHub extension PR status" — compact status display patterns
4. "GitHub pull request review decision API states" — canonical status vocabulary

**Search cap:** 4 searches
**Output:** `research/features.md`

---

## Dimension 2: Architecture

**Question:** Where should the PR poller live in a Go codebase with ConnectRPC streaming? Session-level goroutine vs. workspace-level ticker? Cache invalidation strategy?

**Search strategy:**
1. Explore existing polling patterns in the codebase (review_queue_poller.go, history_watcher.go)
2. "Go background polling goroutine per resource vs shared ticker pattern"
3. "ConnectRPC streaming server push state updates Go"

**Search cap:** 3 searches (1 codebase, 2 web)
**Output:** `research/architecture.md`

---

## Dimension 3: Pitfalls

**Question:** What goes wrong when polling GitHub CLI per-session? Auth failures, rate limits, sessions without remotes, fork PRs, stale data.

**Search strategy:**
1. "gh cli rate limiting github api" — how gh handles rate limits
2. "github pull request head branch fork cross-repo PR detection"
3. Codebase: how does the existing CheckGHAuth handle failures?

**Search cap:** 3 searches
**Output:** `research/pitfalls.md`
