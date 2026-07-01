# Pitfalls and Failure Modes: GitHub PR Data Integration

## 1. Context-Free `gh` CLI Calls (Potential Hangs)

Every `gh` and `git` invocation in `github/client.go` uses `safeexec.CommandContext` with either a caller-supplied context or a fresh `context.WithTimeout(context.Background(), ...)`. There are **no raw `exec.Command` calls without a timeout**. However, a class of functions creates its own background context rather than accepting one from the caller, meaning **caller cancellation cannot propagate to the subprocess**:

| Function | Line | Context source | Timeout |
|---|---|---|---|
| `GetPRInfo` | 179 | `context.Background()` (wraps to `GetPRInfoCtx`) | none (inherits caller ctx) |
| `GetPRComments` | 399 | `context.WithTimeout(context.Background(), 30s)` | 30s hard |
| `GetPRDiff` | 443 | `context.WithTimeout(context.Background(), 30s)` | 30s hard |
| `PostPRComment` | 466 | `context.WithTimeout(context.Background(), 30s)` | 30s hard |
| `MergePR` | 499 | `context.WithTimeout(context.Background(), 60s)` | 60s hard |
| `ClosePR` | 521 | `context.WithTimeout(context.Background(), 30s)` | 30s hard |
| `CloneRepository` | 541 | `context.WithTimeout(context.Background(), 120s)` | 120s hard |
| `FetchBranch` | 557 | `context.WithTimeout(context.Background(), 30s)` | 30s hard |
| `CheckoutBranch` | 572 | `context.WithTimeout(context.Background(), 30s)` | 30s hard |
| `GetRemoteURL` | 587 | `context.WithTimeout(context.Background(), 10s)` | 10s hard |

**Risk when adding PR work-continuity polling**: New polling callers will pass a session context. If they call `GetPRComments`, `GetPRDiff`, etc. on session teardown, those functions will NOT cancel even after the session context is cancelled — they will run to completion (up to 30s) after the caller has given up. Under high session churn (many sessions pausing/resuming), this can leave a backlog of orphaned subprocesses.

**Recommendation**: Add `Ctx` variants for `GetPRComments`, `GetPRDiff`, and any new PR-fetch functions, accepting a caller context and skipping the `context.Background()` wrapper.

---

## 2. BUG-020 and BUG-021: Mutex Contention That Gets Worse With More gh Calls

### BUG-020: VCS Mutex Contention (`github/client.go`, `server/services/`)

Mutex profiling (2026-04-24) shows `GetVCSStatus` and `GetSessionDiff` RPC handlers execute git subprocesses while holding a session or server mutex. This accounts for ~24% of cumulative mutex delay (9.6s total). The problem is that the mutex scope is too wide — it wraps subprocess calls that can each take hundreds of milliseconds.

**Impact on PR integration**: If `GetPRForBranch` or `GetPRInfoCtx` is called inside the same lock scope (e.g. during session status polling), it adds another subprocess to the critical section. Even a 200ms `gh pr view` call, when serialized under one mutex across N concurrent sessions, produces N × 200ms of total delay. With 10 active sessions, a single polling tick can lock the server for 2 seconds.

**Recommendation**: Read session state (worktree path, branch, repo slug) under the lock, then release the lock before any `gh` or `git` subprocess. See the fix approach in `docs/bugs/open/BUG-020-vcs-status-diff-mutex-contention.md`.

### BUG-021: `CheckGHAuth` Mutex Contention (`github/client.go`)

`CheckGHAuth` accounts for ~21% of mutex delay (2.02s cumulative). The root cause is not yet confirmed but is likely a broad caller-side lock that is held during the auth check, or an internal cache mutex held during the subprocess call.

Current implementation (lines 136–176 of `github/client.go`): the function uses `singleflight` and `atomic.Value` for the cache, which is correct and lock-free. The problem is almost certainly at the **call site** — `GetPRComments`, `GetPRDiff`, `PostPRComment`, `MergePR`, and `ClosePR` all call `CheckGHAuth()` as their first step, and if those functions are called while a session mutex is held, the singleflight group blocks the entire critical section for up to 10s (the `gh auth status` timeout).

**Impact on PR integration**: Every new PR-fetching function that calls `CheckGHAuth()` at the top, if called inside an existing lock scope, will contribute further to BUG-021. This is a compounding problem: each new feature adds another entry point that multiplies mutex contention.

**Recommendation**: Do not call `CheckGHAuth()` inside any lock scope. Validate auth once at session start or at polling tick initialization, not inside per-PR or per-worktree fetch calls.

---

## 3. Auth TTL and Expiry Mid-Session

### `CheckGHAuth` TTL Analysis

`CheckGHAuth` (lines 136–176) caches auth results for `ghAuthTTL = 5 minutes` (line 36). This means:
- A valid auth result is cached for 5 minutes from the last successful check.
- A failed auth result is also cached for 5 minutes (the `authResult` struct caches both success and error states with the same TTL).

**Edge case — token expiry mid-session**: If the user's `gh` token expires between two polling cycles (e.g. during a long work session), the cached auth result (`err: nil`) stays valid for up to 5 minutes. During that window, every PR fetch will proceed with an expired token, fail at the GitHub API level with a 401, and return an error to the caller. The `CheckGHAuth` fast-path will not detect the expiry until the cache expires.

The 5-minute TTL is short enough to detect a revoked token within one polling cycle (if polling is 30s), but the failure at the API level will produce confusing errors (401 on `gh pr view`) rather than a clear "auth expired" message.

**Token source precedence** (`http_client.go` lines 35–63): `GITHUB_TOKEN` env > `GH_TOKEN` env > `gh auth token` (cached 1 hour). If `GITHUB_TOKEN` is set but expired, the hourly cache in `getGHToken` will continue serving the expired token for up to an hour before re-running `gh auth token`. This is a separate, longer-TTL cache that can cause extended auth failures.

**Recommendation**: When a 401 is received from the GitHub REST API, immediately invalidate both the `ghAuthState` and `ghTokenState` caches, forcing a fresh check on the next call.

---

## 4. Edge Cases for PR Data Integration

### 4.1 Repos the User No Longer Has Access To (404 on PR Fetch)

`GetPRForBranch` (lines 316–366) calls the GitHub REST API. The response handling covers 401 and 403 explicitly, but **HTTP 404 is handled by the generic fallthrough** (line 338: `"GitHub API returned status %d for PR list"`). A 404 can mean either "repo does not exist" or "repo exists but you do not have access" — GitHub returns 404 (not 403) for private repos the user lost access to, to avoid leaking repo existence.

**Impact**: The scanner will log an error and return no PR info, but since `GetPRForBranch` is a one-shot lookup (not retried), the repo will continue to be scanned by `scanner.go` at every 30s tick. Each tick will trigger a 404, consuming rate-limit quota and producing log noise.

**Recommendation**: Track per-repo "access denied" state in a short-TTL cache (e.g. 10 minutes). If a repo returns 404 on PR lookup, skip its PR fetch for the backoff window. The existing `circuitBreaker` pattern in `scanner.go` (line 151: `breakerStore sync.Map`) could be extended.

### 4.2 Branch Name Collision (Same Branch Name in Multiple Repos)

The `GetPRForBranch` API path (line 317) uses the `owner` parameter as the head qualifier: `head=%s:%s` (owner:branch). This correctly scopes the lookup to a single (owner, repo, branch) triple — **no collision risk across repos**.

However, the `ScanResult` key in `scanner.go` (line 445) is `repoPath + "|" + branch`. If the same local branch name appears in two repos that map to different GitHub upstreams, the `resultStore` keys are different (because `repoPath` differs). No collision.

**Residual risk**: Shared worktrees where two `ScanResult` entries reference the same physical path but different branch names (e.g., detached HEAD reattached to a different branch between scan cycles). The cache key changes, so the old entry is orphaned in `resultStore` until the next full scan purges it.

### 4.3 User Pushes During Polling Window (PR Number Changes)

`GetPRForBranch` queries `state=all&per_page=10` (line 317) and picks the most recently updated PR. If the user force-pushes and GitHub closes the old PR and opens a new one during a polling window:

1. Old PR (e.g. #42) is closed.
2. New PR (e.g. #43) is opened on the same branch.
3. Next poll: `GetPRForBranch` returns PR #43 (most recently updated).
4. If `GetPRInfoConditional` is being used with an ETag cache, the cache key is `owner/repo/42` — PR #43 will not be in the cache and will be fetched fresh. No stale data.

**Risk**: The window between closing #42 and opening #43 can be non-zero. During that window, `GetPRForBranch` returns `ErrNoPR`, and the PR badge in the UI disappears momentarily. This is cosmetically jarring but not a correctness bug.

**Recommendation**: Tolerate `ErrNoPR` silently at the UI layer and display the last-known PR info for one polling cycle before clearing it.

### 4.4 GitHub API Secondary Rate Limits (Concurrent Requests)

The `ghHTTPClient` (http_client.go line 17) is a single `*http.Client` with a 30s timeout but **no rate-limit awareness**. GitHub's primary rate limit is 5000 requests/hour for authenticated users; the secondary rate limit (undocumented, enforced via 403 with `Retry-After`) applies to:
- More than 100 concurrent requests
- More than 900 points per minute (for GraphQL, but REST has similar limits)
- Rapid creation of content (comments, etc.)

**Current state**: `etag_cache.go` uses conditional GET requests (ETag / If-None-Match) to consume zero rate-limit quota on cache hits, which is good. However, there is **no retry logic on 403 with `Retry-After`**, no exponential backoff, and no global request rate limiter.

**Impact**: With polling across many repos (e.g. 20 sessions × 30s tick = ~40 requests/minute baseline), secondary rate limits are unlikely but not impossible, especially if PR comments or diffs are also fetched each cycle.

**Recommendation**: Inspect `resp.Header.Get("Retry-After")` and `resp.Header.Get("X-RateLimit-Remaining")` after every REST call. If `X-RateLimit-Remaining` is low or a 429/403 with `Retry-After` is received, back off globally using a token bucket or `time.Sleep(retryAfter)`.

### 4.5 gh CLI Not Installed

`CheckGHAuth` (lines 148–152) uses `exec.LookPath("gh")` to detect a missing binary and returns a clear error: `"GitHub CLI (gh) is not installed. Please install it: https://cli.github.com/"`. This error is cached for `ghAuthTTL` (5 minutes).

**Issue**: Every function that calls `CheckGHAuth()` will return this error for 5 minutes before re-checking. This is intentional to avoid repeated LookPath calls, but if the user installs `gh` mid-session, they must wait up to 5 minutes for the cache to expire before work-continuity features activate.

**Recommendation**: Expose a "retry now" mechanism (e.g. a button in the UI that invalidates `ghAuthState`) to allow immediate re-detection.

### 4.6 Multiple GitHub Accounts (SSO Orgs vs. Personal)

`getGHToken` (http_client.go lines 35–63) has a single token slot. The precedence is `GITHUB_TOKEN` > `GH_TOKEN` > `gh auth token`. The `gh auth token` command returns the active account's token.

**Problem**: A developer who has both a personal account (for open source) and an enterprise account (for work, behind SAML SSO) will have `gh` authenticated to one account at a time. The `gh auth switch` command exists, but `gh auth token` returns whichever account is currently active. If the active account lacks access to the work repos (or vice versa), all API calls will fail with 404 or 403.

`gh` version 2.x supports multiple active tokens via `gh auth login --hostname <enterprise-host>`. If the work repos are on `github.com` with SSO, the user must `gh auth refresh --scopes read:org` to include the SSO-authorized organization scope.

**Impact**: No token rotation or multi-account support exists in the codebase. A user with two accounts will need to ensure their active `gh` account has the correct SSO authorization for each repo being polled.

**Recommendation**: When a 403 with an SSO-enforcement body is returned from the GitHub API, surface a specific error message: `"This repository requires SAML SSO authorization. Run 'gh auth refresh' for the [org] organization."` GitHub returns `X-Github-Sso` headers in this case that identify the required SSO org.

---

## 5. Scanner Context Propagation: Does It Block?

### Scanner Loop Analysis (lines 285–299, 301–411)

The `worker` goroutine correctly exits on `ctx.Done()` (line 289). However, **context is NOT passed into `scanRepo` or `scanWorktree`**:

```go
// worker goroutine (line 286)
func (s *Scanner) worker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case task, ok := <-s.scanQueue:
            // ...
            results := s.scanRepo(task.repoPath)  // <-- ctx not passed
            s.publishResults(results)
        }
    }
}
```

`scanRepo` calls `s.reader.ListWorktrees`, `s.reader.HasUncommitted`, `s.reader.AheadBehind`, `s.reader.CommitMessages`, and `s.reader.DiffShortstat` — none of which accept a context. The `GoGitVCSReader` implementation runs entirely in-process (no subprocesses) with its own per-repo mutex.

**Can a slow VCS call block the entire scan?** Yes and no:
- The 4 worker goroutines run independently, so a slow scan of one repo blocks only 1 of the 4 workers.
- If all 4 workers are simultaneously stuck on slow VCS operations (e.g. 4 repos with large histories), the `scanQueue` backs up. New tasks are dropped with a warning (line 281: `"scan queue full, dropping repo"`).
- On `ctx.Done()`, the coordinator and event subscriber goroutines exit immediately, but the 4 worker goroutines are blocked inside `scanRepo`. They will not exit until the current VCS operation completes.

**Worst case**: On application shutdown with 4 workers blocked on large-repo VCS operations, shutdown is delayed by the duration of the longest in-flight scan. The `GoGitVCSReader` BFS for `AheadBehind` is bounded by `mergeBaseBFSLimit = 2000` commits, and `reachableSet` in `CommitMessages` is **unbounded** — it walks every reachable commit from a given start hash. A repo with 100K commits could block a worker for seconds.

**If PR fetches are added to the scan loop**: If `GetPRForBranch` is called from `scanWorktree`, the same context-propagation gap applies. The HTTP call has a 30s timeout from the `ghHTTPClient`, but it ignores the worker's context. On shutdown, all 4 workers could simultaneously block on 30s HTTP calls.

**Recommendation**: Pass `ctx` into `scanRepo` and `scanWorktree`. For `GoGitVCSReader`, add context-aware cancellation to `reachableSet` (the unbounded commit walk) and to any new PR-fetch calls.

---

## Summary of Highest-Severity Pitfalls

**Pitfall 1 — BUG-020/BUG-021 compounds with every new gh call**: Both open bugs describe mutex contention caused by running subprocess/network calls inside a lock scope. Every new `GetPR*` call added to the PR polling path will make these bugs measurably worse if called inside any existing lock. The fix (read state under lock, release lock, then call gh) must be applied before adding more gh CLI callers.

**Pitfall 2 — Scanner workers cannot be cancelled mid-scan**: The `scanRepo`/`scanWorktree` path does not propagate the parent context. If PR fetches are added to the scan loop, a 30s HTTP timeout per PR × 4 concurrent workers means application shutdown can block for up to 30 seconds after ctx is cancelled. The `reachableSet` commit walk is also unbounded and provides no cancellation point.

**Pitfall 3 — No rate-limit awareness or retry for secondary rate limits**: `ghHTTPClient` has no backoff, no `Retry-After` handling, and no `X-RateLimit-Remaining` inspection. Polling PRs across many repos concurrently — especially if each poll also fetches comments and diffs — risks hitting GitHub's secondary rate limits (undocumented, enforced via 403), which currently produce an opaque error with no automatic recovery.
