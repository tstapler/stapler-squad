# Pitfalls Research: PR Status Polling

Date: 2026-04-09

## gh CLI Auth & Availability

**Current implementation (`github/client.go:79-93`):**
- `CheckGHAuth()` checks gh binary via `exec.LookPath("gh")` then `gh auth status`
- Returns descriptive errors pointing to https://cli.github.com/

**Gaps:**

1. **Silent stale auth** — tokens can expire between `CheckGHAuth()` calls. Need to detect `HTTP 401`/`Unauthorized` in stderr of actual API calls, not just at auth-check time.
2. **No graceful degradation** — if gh is missing/unauthenticated, poller errors out. Need a "PR status unavailable" non-fatal state.
3. **No retry with backoff** — auth failures are immediate. Cache "auth failed" state to avoid repeated `gh auth status` calls every poll cycle.
4. **gh multi-account** — poller uses default gh config. Edge case if user has multiple accounts; not blocking for Phase 1.

---

## Rate Limiting

**GitHub API limits:**
- Primary: 5000 req/hr (authenticated)
- Secondary: 100 concurrent requests; 15s reset if exceeded
- gh CLI does NOT have built-in backoff — fails immediately on rate limit

**Failure pattern in stderr:**
```
error: API rate limit exceeded. Please wait a few minutes before trying again.
```

**Critical risk:** If polling runs per-session every 2s (like ReviewQueuePoller), 100 sessions = 100 calls in 2s = secondary rate limit hit immediately.

### ETag Conditional Requests (key optimization)

GitHub REST API supports `ETag` / `If-None-Match` conditional requests:
- Every `GET /repos/:owner/:repo/pulls/:pr_number` response includes an `ETag` header
- On subsequent calls, pass `If-None-Match: <cached-etag>`
- If PR is unchanged: GitHub returns `304 Not Modified` — **costs zero rate limit quota**
- If PR changed: GitHub returns `200` with new data and a new ETag

This means polling every 60s (instead of 5min) is safe in practice — inactive PRs return 304s and don't consume quota. Only changed PRs count against the 5000/hr limit.

`gh api` supports this via `--header "If-None-Match: <etag>"`. Store ETag per `(owner, repo, prNumber)` in the poller.

**Mitigations:**
- ETag caching per `(owner, repo, prNumber)` — enables safe aggressive polling (60s)
- Single workspace-level ticker iterating all sessions (not per-session goroutine)
- Semaphore: max 5 concurrent `gh` calls at any time
- Jitter: stagger checks across poll interval
- On rate limit error: pause all polling 60s, track `rateLimitedUntil` timestamp
- Parse stderr to discriminate `rate limit` vs `HTTP 401` vs `HTTP 404`

**No push alternative exists:** GitHub has no GraphQL subscriptions, no `gh webhook forward` for local tools, no SSE endpoints. `gh run watch` itself uses polling. Conditional polling is the best available approach.

---

## Fork/Cross-Repo PRs

**The problem:** `gh pr list --head <branch> --repo <owner>/<repo>` only finds PRs opened *against* that repo. If a user is working in a fork (`myuser/project`), their PR is likely opened against the upstream (`upstream-org/project`). The `--head` lookup returns empty.

**Detection approach:**
1. Call `gh api repos/:owner/:repo --jq '.isFork'` to detect forks
2. If fork, get upstream: `gh api repos/:owner/:repo --jq '.parent.full_name'`
3. Search for PR on upstream: `gh pr list --head <fork-owner>:<branch> --repo <upstream_owner>/<upstream_repo>`

Note: for fork PRs, the `--head` filter uses `owner:branch` syntax (e.g., `myuser:feature-x`).

**Phase 1 decision:** Implement basic fork detection (detect + skip with a "fork - upstream PR lookup not yet supported" message) rather than full dual-repo search. Reduces complexity; can extend in Phase 2.

---

## Sessions Without Remotes

**Scenarios:**
- Session created in a local directory with no git repo
- Subdirectory of a monorepo (git top-level is elsewhere)
- Detached HEAD state (no branch name)
- Bare repo clone

**Detection sequence before any PR lookup:**
```
1. git rev-parse --is-inside-work-tree   → if fails: skip, no git repo
2. git rev-parse --show-toplevel         → find actual repo root
3. git remote get-url origin             → if fails: skip, no remote
4. git symbolic-ref -q HEAD              → if fails: skip, detached HEAD
5. Parse branch name from symbolic-ref output
```

**Non-fatal handling:** Set `PRStatusState = "no_remote"` (or `"no_branch"`). Don't error, don't retry. Show neutral badge in UI.

---

## Terminal PR States (merged/closed)

**GitHub PR states:** `OPEN`, `MERGED`, `CLOSED`

**Problem:** Once a PR is merged/closed, there's no reason to keep polling it at the normal 5-minute interval. But current design has no stop-polling logic.

**Recommended tiered polling:**
- `OPEN` → poll every 5 minutes (configurable default)
- `MERGED` / `CLOSED` → mark as terminal; switch to 1x/hour refresh (in case of edge cases) or stop entirely
- `NOTFOUND` (no PR on branch) → poll every 15 minutes (low priority, branch may not have a PR yet)

**Implementation:** Store `PRStatusTerminal bool` on Instance; poller skips terminal sessions except for a low-frequency background sweep.

---

## Concurrency Hazards

**Current model:** `ReviewQueuePoller` loops over all sessions every 2s. `Instance.stateMutex` exists for protecting state fields.

**Hazards:**

1. **Session deleted during fetch** — `RefreshPRInfo()` takes ~500ms; session may be deleted mid-call.
   - Mitigation: Check session still in storage *after* fetch before storing result. Use context cancellation.

2. **Stale Instance pointers** — Poller holds pointers; if Instance is reloaded from storage, old pointer is orphaned.
   - Mitigation: Use session ID (Title string) as key; re-fetch from storage before writing.

3. **No timeout on gh calls** — if `gh` CLI hangs (network issue), entire poller blocks.
   - Mitigation: `exec.CommandContext(ctx, "gh", ...)` with 8-10s timeout per call.

4. **Race on GitHubPRNumber write** — PR number discovered by poller; needs `stateMutex` lock when writing back.
   - Mitigation: Wrap all GitHub field reads/writes in `stateMutex`.

5. **Thundering herd on startup** — all sessions try to fetch PR info simultaneously on app start.
   - Mitigation: Stagger initial fetches with jitter; use semaphore (max 5 concurrent).

---

## gh pr list --json Fields Available

**Currently used:** `number,title,body,headRefName,baseRefName,state,url,createdAt,updatedAt,isDraft,mergeable,additions,deletions,changedFiles,author,labels`

**Additional fields needed for Phase 1 status:**
```
reviews              → review decisions (APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED)
requestedReviewers   → who hasn't reviewed yet
statusCheckRollup    → CI/CD status (SUCCESS, FAILURE, PENDING, ERROR, NEUTRAL)
```

**Complete enriched field list:**
```
number,title,headRefName,baseRefName,state,url,updatedAt,isDraft,
reviews,requestedReviewers,statusCheckRollup
```
(Drop body, additions, deletions, changedFiles from the status-poll call — save those for the detail view)

---

## Recommended Error Handling Strategy

```
Per-Session PR Fetch:
  1. Pre-flight checks (no gh calls):
     - Is session still in storage? (re-fetch by ID)
     - Does session have a branch? (not detached HEAD)
     - Does session have a git remote? (git remote get-url origin)
     → If any fail: set PRStatus{State: "unavailable", Reason: <why>}, return

  2. Auth check (cached, not every call):
     - CheckGHAuth() result cached for 5 minutes
     → If fails: set PRStatus{State: "auth_error"}, pause ALL polling, surface error in UI

  3. Fetch with timeout:
     - exec.CommandContext with 8s timeout
     - Parse stderr for error classification
     → rate limit:  PRStatus{State: "rate_limited"}, pause all polling 60s
     → HTTP 401:    PRStatus{State: "auth_error"}, pause all polling
     → HTTP 404:    PRStatus{State: "not_found"}
     → timeout:     PRStatus{State: "error", Msg: "timeout"}, use stale data
     → no results:  PRStatus{State: "no_pr"}

  4. Parse and cache:
     - Extract number, state, review decision, CI status
     - Store on Instance with LastRefreshed timestamp
     - If MERGED/CLOSED: set Terminal=true
```

**Error-to-UI mapping:**

| Error | User-visible state |
|---|---|
| gh not installed | "GitHub CLI not installed" (one-time banner) |
| HTTP 401 | "Auth error – run `gh auth login`" |
| HTTP 403 rate limit | "Rate limited, retrying in Ns" |
| HTTP 404 | "Repo not found" |
| Network timeout | "Network error (using cached data from Xm ago)" |
| No .git / no remote | Badge hidden (not an error) |
| Detached HEAD | Badge hidden |
| No PR on branch | No badge shown |

---

## Sources

- Direct codebase analysis: `github/client.go`, `session/pr_tracking.go`, `session/review_queue_poller.go`
- GitHub API documentation (rate limits, fork PR detection)
- GitHub CLI source behavior (error message patterns)
- Go `exec.CommandContext` timeout patterns
