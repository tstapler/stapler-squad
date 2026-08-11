# Findings: ReviewQueuePoller Optimization

## Summary

The profiler attribution of 57% CPU / 5.3s syscall time to `session.(*ReviewQueuePoller).pollLoop`
is primarily from **two subprocess types per poll tick**, not from GitHub (`gh`) calls.
`gh` CLI calls happen in the separate `PRStatusPoller` at a 60-second interval, which is already
well-engineered. The hot path in `ReviewQueuePoller` is:

1. **`tmux capture-pane`** — one `exec.Command` per session per 2-second tick via `inst.Preview()`
   (for sessions without an active `ClaudeController`, where the activity-cache miss path is hit)
2. **`git status --porcelain`** — one `exec.Command` per session *with a git worktree* per 2-second tick
   via `worktree.IsDirty()`

The mutex profile showing `github.CheckGHAuth` at 2.02s cumulative delay is a **separate problem**:
`CheckGHAuth` runs `gh auth status` (a subprocess) and every function in `github/client.go` calls it
redundantly before each operation. The `PRStatusPoller` already has a TTL auth cache (`isAuthOK()`);
the plain `client.go` helpers do not, and callers that hold locks while invoking them create contention.

**Bottom line**: To eliminate the profiler hot spot, the highest-leverage interventions are:

- Stop calling `git status --porcelain` on every 2-second tick (raise interval or cache result)
- Skip `IsDirty()` when the session is actively processing (waste if Claude is about to generate output)
- Ensure the activity cache in `getContent()` reliably skips `tmux capture-pane` for active-controller
  sessions (this already works; remaining subprocess calls are from no-controller sessions)
- Fix `CheckGHAuth` with a package-level TTL cache so it is not a repeated subprocess per `gh` call

The question of replacing `gh` CLI with a native Go GitHub client is valid but solves a different
problem than what is currently burning CPU.

---

## Options Surveyed

### 1. Raise the `IsDirty()` poll interval / move to a separate lower-frequency goroutine

**Current behavior**: `worktree.IsDirty()` runs every 2 seconds for every session that has a git
worktree and is not already flagged with a higher-priority reason. On a machine with N managed
sessions (each with a worktree), this is N `git status --porcelain` subprocesses per tick.
The check runs even when `shouldAdd=false` and falls into the low-priority guard (lines 652–670
and 752–773 in `session/review_queue_poller.go`).

**Fix**: Track `lastDirtyCheckAt map[string]time.Time` on `ReviewQueuePoller`. Gate `IsDirty()`
behind a minimum elapsed time (e.g., 15–30 seconds). Also skip entirely when
`statusInfo.ClaudeStatus == detection.StatusActive || StatusProcessing` — there is no point
checking for uncommitted changes while Claude is actively executing.

**Expected savings**: Reduces `git status` subprocess count by ~7–15x per session per minute.

### 2. Package-level TTL cache for `CheckGHAuth`

**Current behavior**: Every function in `github/client.go` calls `CheckGHAuth()` which runs
`exec.Command("gh", "auth", "status")` unconditionally. The `PRStatusPoller` already has its own
`isAuthOK()` TTL cache, but that cache is private to the struct. Any other caller (e.g., direct
`GetPRInfoCtx` or `GetPRComments` from the server layer) re-runs `gh auth status` on every
invocation. Under concurrent calls, multiple goroutines block on the same subprocess, which shows
up as mutex contention when a lock wraps the call site.

**Fix**: Extract the TTL cache logic from `PRStatusPoller.isAuthOK()` into a package-level
`sync/atomic` + TTL variable in `github/client.go`. Use `golang.org/x/sync/singleflight` to
coalesce concurrent calls so only one `gh auth status` subprocess runs at a time. Cache a
successful result for 5 minutes, a failure for 30 seconds.

**Expected savings**: Eliminates duplicate subprocess calls. Eliminates mutex contention at call
sites that hold a lock while waiting for auth.

### 3. `singleflight` to coalesce concurrent `tmux capture-pane` calls

**Current behavior**: If multiple goroutines ask for the same session's content simultaneously
(e.g., the poller tick coinciding with a reactive trigger from `ReactiveQueueManager.handleUserInteraction`),
two `tmux capture-pane` subprocesses may run for the same session concurrently.

**Fix**: Wrap `inst.Preview()` (or `tmuxManager.CapturePaneContent()`) in a per-session
`singleflight.Group`. Any concurrent caller waiting for the same session's content shares the result.

**Expected savings**: Modest — relevant mainly under high-reactivity workloads. Lower priority
than options 1 and 2.

### 4. Replace `gh` CLI subprocesses with a native Go GitHub REST/GraphQL client

**Current behavior**: All GitHub operations (PR info, auth check, PR list, diff, comments, merge)
are done by `exec.Command("gh", ...)`, forking a subprocess for each request.

**Options** [TRAINING_ONLY — verify current status of each library]:
- `google/go-github` — REST client, mature, well-maintained, handles pagination
- `shurcooL/githubv4` — GraphQL client, allows precise field selection, reduces over-fetching
- `cli/go-gh` — official GitHub CLI Go library, provides `gh api` equivalent but in-process
- Raw `net/http` + `golang.org/x/oauth2` — maximum control, most implementation work

**Key benefit of GraphQL**: A single `search` query can find all open PRs awaiting review from a
specific user across all repos, eliminating the per-branch `gh pr list` + `gh pr view` two-step
that the current code does for each session.

**Key drawback**: `gh` CLI handles GitHub Enterprise, SSO re-auth, and keychain integration
automatically. A raw Go client requires replicating all of that. The simplest bridge is calling
`gh auth token` once at startup to read the token — but that introduces one subprocess at startup.
Reading `~/.config/gh/hosts.yml` directly avoids the subprocess but couples to `gh`'s file format
[TRAINING_ONLY — verify current gh config format and path].

**ETag observation**: `github/etag_cache.go` already implements conditional `gh api --include`
requests with `If-None-Match`. Switching to `google/go-github` or a raw HTTP client preserves
this pattern natively through standard HTTP `If-None-Match` headers.

### 5. Adaptive polling interval for `ReviewQueuePoller`

**Current behavior**: Fixed 2-second `PollInterval` regardless of session count or activity level.
The existing `consecutiveErrors` backoff handles error conditions but not quiet periods.

**Fix**: When all monitored sessions are in `IdleStateActive` or `IdleStateWaiting` (not awaiting
approval/input/error), step the tick interval to 8–10 seconds. When any session transitions to a
high-priority state, snap back to 2 seconds. Wire into `EventBus` via a channel so the poller
receives `EventUserInteraction` and `EventApprovalResponse` signals without polling.

**Safety**: `ReactiveQueueManager` already handles immediate re-evaluation on those events.
The periodic poll is a safety net; lengthening it during quiet periods is safe.

**Expected savings**: Linear reduction in all subprocess calls proportional to the interval increase.
At 10 sessions (5 with worktrees), 2s interval: 10 `tmux capture-pane` + 5 `git status` = 900
subprocess/minute. At 8s interval during quiet periods: ~225/minute.

### 6. GitHub Webhooks instead of polling (for `PRStatusPoller`)

**Current behavior**: `PRStatusPoller` polls at 60-second intervals with ETag caching. This is
already efficient for a personal tool.

**Assessment**: Overkill for a local developer tool. Requires a public HTTPS endpoint (ngrok,
Cloudflare Tunnel, or hosted server), webhook secret validation, and per-repo configuration.
The 60-second interval with ETag caching means most requests return 304 and cost zero rate-limit
quota. Webhook complexity adds operational burden with minimal benefit.

---

## Trade-off Matrix

| Option | Subprocess overhead | Freshness | Implementation effort | Operational complexity | Notes |
|--------|--------------------|-----------|-----------------------|----------------------|-------|
| 1. Raise `IsDirty()` interval (15–30s) | High reduction (~7–15x fewer `git status`) | Minimal impact — dirty state changes slowly | Low (~1 hour, add timestamp gate) | None | Best immediate bang-for-buck |
| 2. Package-level `CheckGHAuth` TTL + singleflight | Eliminates redundant `gh auth status` calls | None — auth state changes rarely | Low (~2 hours, extract from `PRStatusPoller`) | None | Fixes mutex contention too |
| 3. `singleflight` for `Preview()` | Modest reduction under burst scenarios | None | Low (~1 hour) | None | Defensive; less urgent |
| 4. Replace `gh` with native Go client | Eliminates all fork+exec for GitHub; enables connection pooling | Marginally better (no CLI startup ~20–50ms) [TRAINING_ONLY] | High (2–5 days) | Moderate — must retest all GitHub paths | Correct long-term direction; not urgent for current hot spot |
| 5. Adaptive poll interval | Linear reduction in all subprocesses | Max ~10s added delay vs 2s during quiet periods | Medium (~3 hours, EventBus wiring) | None | High leverage; `ReactiveQueueManager` already handles urgency |
| 6. GitHub webhooks | N/A for review queue poller | Real-time | High | High — public endpoint, secret rotation | Overkill for local tool |

---

## Risk and Failure Modes

**Option 1 (raise `IsDirty()` interval)**
- Risk: Dirty-change notification is delayed. If a session completes work and leaves uncommitted
  changes, the user will not see it in the review queue for up to 30 seconds instead of 2 seconds.
- Mitigation: `ReactiveQueueManager` handles high-priority events (approval, error) immediately.
  `ReasonUncommittedChanges` is low-priority (`PriorityLow`), so a 30-second delay is acceptable.
- Risk: The `lastDirtyCheckAt` map grows proportionally to session count; negligible memory cost.

**Option 2 (package-level auth cache)**
- Risk: Auth state is stale if the user runs `gh auth logout` between cache refreshes (up to 5
  minutes). This is intentional — 5 minutes is an acceptable stale window.
- Risk: `singleflight` means if the first auth check fails, all concurrent callers fail together.
  This is correct behavior (fail fast).
- Risk: Package-level state is harder to test in isolation. Provide a `resetAuthCacheForTest()`
  function guarded by a build tag, or inject the cache as a dependency.

**Option 4 (native Go client)**
- Risk: Token acquisition is platform-specific. `gh auth token` subprocess is simplest; reading
  `~/.config/gh/hosts.yml` is faster but brittle to format changes.
- Risk: Enterprise GitHub or SSO token refresh is complex. `gh` CLI handles this transparently.
- Risk: Subtle behavioral differences from existing `gh` flag combinations.

**Option 5 (adaptive interval)**
- Risk: Reactive triggers via `EventBus` must reliably cover all high-urgency transitions. If an
  event is dropped or the event bus is slow, detection delay increases beyond the intended 2s.
- Mitigation: Keep the slow-interval cap at 8–10s (not 30s+), so worst-case detection is still
  acceptable. The existing `consecutiveErrors` pattern proves the approach is sound.

---

## Migration and Adoption Cost

**Option 1**: ~1 hour. Add `lastDirtyCheckAt map[string]time.Time` and `dirtyCheckMu sync.Mutex`
to `ReviewQueuePoller`. Gate `worktree.IsDirty()` in `checkSession` behind an elapsed-time check.
No interface or API changes. Add one unit test.

**Option 2**: ~2 hours. Move the TTL+singleflight pattern from `PRStatusPoller.isAuthOK()` into
`github/client.go` as a package-level struct. Update `CheckGHAuth()` to delegate to it. Write
one test verifying that concurrent calls result in a single subprocess execution.

**Option 3**: ~1 hour. Add a `sync.Map` of `singleflight.Group` (keyed by session title) to
`ReviewQueuePoller`. Wrap the `inst.Preview()` call inside `getContent()`.

**Option 4**: 2–5 days. New `github/api_client.go` implementing the same `GetPRInfoCtx`,
`GetPRForBranch`, etc. interfaces behind an interface type, using `google/go-github` or raw HTTP.
Requires token acquisition strategy, rate-limit handler, and full re-test of PR flow. Can be
done incrementally — replace one function at a time behind an interface without changing callers.

**Option 5**: ~3 hours. Add an `activityCh <-chan struct{}` field to `ReviewQueuePoller`, wired
from `EventBus` in `Start()`. In `pollLoop`, use `select` with a timer that resets on activity
events. Use two timer values: `fastInterval` (2s) and `slowInterval` (8s).

---

## Operational Concerns

**`go test` isolation**: The `PRStatusPoller.isAuthOK()` TTL cache is struct-scoped, so tests
already get a clean instance. A package-level cache (Option 2) needs a reset mechanism for tests —
use a package-level `var authCache = newGHAuthCache()` with an exported `ResetGHAuthCacheForTest()`
function that creates a fresh instance, callable only from test code.

**Rate limits**: The GitHub REST API rate limit is 5,000 requests/hour for authenticated users,
with secondary rate limits on concurrent requests [TRAINING_ONLY — verify current limits].
The ETag cache already makes conditional request cost zero quota on 304 responses. Switching to a
native client should preserve ETag behavior; `google/go-github` supports conditional request
headers natively via `github.ListOptions`.

**macOS subprocess cost**: On macOS, `exec.Command` forks a new process. Typical overhead is
5–15ms per subprocess for fork+exec, plus the command's own startup time. For `gh` (a Go binary),
startup involves Go runtime initialization, typically 20–50ms [TRAINING_ONLY — measure for local
`gh` version]. For `git status`, startup is ~5–10ms on a warm filesystem cache. These numbers
explain why N sessions × 2s interval becomes syscall-dominant in traces.

**`tmux capture-pane` vs PTY read**: Sessions with an active `ClaudeController` already avoid
`tmux capture-pane` via the `getContent()` activity cache. The remaining subprocess calls are
from no-controller sessions (external tmux sessions, paused controller, etc.). These are
unavoidable without a different monitoring mechanism for those session types.

---

## Prior Art and Lessons Learned

**`PRStatusPoller` design (already in this codebase)**:
- Single shared ticker (not per-session goroutines) — correct pattern.
- ETag conditional requests — eliminates rate-limit cost on unchanged PRs.
- Semaphore (`chan struct{}`) for concurrency control — prevents thundering-herd.
- TTL auth cache — eliminates repeated `gh auth status` subprocess.
- Rate-limit backoff — respects GitHub's secondary rate limits.
This is the target design for any future GitHub polling work.

**`getContent()` activity cache (already in this codebase)**:
- Correctly avoids `tmux capture-pane` when `lastActivity` has not changed.
- The cache hit log line at line 419 makes it observable.
- The `IsDirty()` call is NOT covered by this cache — it runs unconditionally when
  `inst.HasGitWorktree()` returns true and the priority is low. This is the primary remaining
  subprocess hotspot.

**Other tools' polling intervals** [TRAINING_ONLY]:
- GitHub's own status-check polling tools typically use 30–60 second intervals for background data.
- Developer tools with approval-style workflows (Slack approval bots, CI notification tools)
  commonly use 10–30 second intervals for "needs attention" signals and event-driven paths for
  real-time triggers.
- The 2-second interval is justified for approval detection (Claude blocked waiting for user input).
  It is not justified for `ReasonUncommittedChanges`, which is low-priority and slowly-changing.

---

## Open Questions

1. **What fraction of polled sessions have an active `ClaudeController`?** If most sessions are
   controller-active, the content cache hit rate is high and `tmux capture-pane` cost is already
   low. In that case `IsDirty()` is the dominant subprocess source.

2. **How many sessions are typically monitored simultaneously?** The profiler time scales linearly
   with session count. At 3 sessions the numbers are modest; at 20 sessions the subprocess rate
   becomes significant.

3. **Is the mutex profile `CheckGHAuth` contention from `PRStatusPoller` or from direct
   `client.go` callers?** The `PRStatusPoller` already caches auth. If the 2.02s mutex delay is
   from concurrent `GetPRInfoCtx` calls in the server layer (e.g., multiple sessions triggering
   PR fetches simultaneously), Option 2 (package-level cache) is the fix. If it is from
   `PRStatusPoller` running `isAuthOK()` check while another goroutine holds `p.mu.Lock()`, the
   fix is finer-grained locking around the write-back only.

4. **Does `worktree.IsDirty()` run on sessions in `IdleStateActive`?** Looking at the code,
   the dirty check runs when `!shouldAdd || priority == PriorityLow` — even when the controller
   shows `IdleStateActive`. The check should be skipped when `idleState == IdleStateActive`.

5. **Can dirty state be derived from in-memory signals?** Git worktree objects could maintain a
   `dirtyUnknown bool` flag that gets set whenever `LastMeaningfulOutput` changes (indicating the
   session did something). This would make the dirty check demand-driven rather than time-driven,
   eliminating the need for an interval-based TTL.

---

## Recommendation

**Immediate (highest ROI, implement first)**:

1. **Option 2 — Package-level `CheckGHAuth` TTL cache + singleflight** (~2 hours).
   Fixes the mutex contention reported in the profiler. Extracts the already-proven pattern
   from `PRStatusPoller.isAuthOK()`. No behavioral change visible to callers.

2. **Option 1 — Gate `IsDirty()` behind a 15-second TTL per session** (~1 hour).
   Add `lastDirtyCheckAt map[string]time.Time` to `ReviewQueuePoller`. Skip `IsDirty()` when fewer
   than 15 seconds have elapsed since the last check for that session. Also skip entirely when
   `statusInfo.ClaudeStatus == detection.StatusActive || detection.StatusProcessing` — there is no
   point checking for dirty worktrees while Claude is actively running.

**Short-term (after measuring impact of above)**:

3. **Option 5 — Adaptive poll interval** (~3 hours).
   When all monitored sessions are quiet (no pending approvals, no active processing), step the
   tick interval to 8 seconds. Wire into `EventBus` to snap back to 2 seconds on
   `EventUserInteraction` or `EventApprovalResponse`. This is safe because `ReactiveQueueManager`
   already provides immediate re-evaluation on those events.

**Long-term (architectural, not urgent)**:

4. **Option 4 — Replace `gh` CLI with `google/go-github`** for `PRStatusPoller` operations.
   Primary benefit: eliminates fork+exec overhead and enables persistent HTTP connection pooling.
   Secondary benefit: enables GraphQL `search` query for cross-repo PR discovery. Scope to
   `PRStatusPoller` only initially; leave merge/comment/auth operations on `gh` CLI until the
   full replacement is validated.

Do **not** implement GitHub webhooks (Option 6) — the operational complexity is not justified
for a local developer tool where polling latency is acceptable.

---

## Web Search Results

### singleflight + TTL pattern (confirmed)
Standard Go pattern: check atomic TTL timestamp → if expired, call `singleflight.Group.Do()` → store result in atomic value with new expiry. This eliminates both concurrent duplicate calls AND stale polling. Libraries: `golang.org/x/sync/singleflight` (stdlib-adjacent, zero deps). Sources: [singleflight overview](https://victoriametrics.com/blog/go-singleflight/), [ttlcache](https://pkg.go.dev/github.com/jellydator/ttlcache).

### git status performance (key finding)
- `libgit2` / `go-git` are **2.5–6x SLOWER** than `git status` subprocess for status operations — do NOT replace subprocess with go-git for dirty check.
- **gitstatusd** (used by powerlevel10k) is 46x faster by using cached index and all CPU cores. But adding a daemon dependency is heavyweight for this use case.
- **Best option for IsDirty()**: TTL cache (15s) + skip when Claude is actively processing. No library change needed. Sources: [libgit2 slower issue](https://github.com/libgit2/libgit2/issues/4230), [gitstatusd 10x post](https://lobste.rs/s/nyuvvt/10x_faster_implementation_git_status).

---

## Pending Web Searches

1. `google/go-github vs cli/go-gh golang github api client 2025` — confirm which library is
   idiomatic for Go programs embedding GitHub API access today
2. `golang.org/x/sync singleflight TTL cache pattern example` — verify the standard pattern for
   combining TTL expiry with singleflight deduplication
3. `github rest api rate limit 2025 authenticated user secondary` — verify current primary and
   secondary rate limit values and reset windows
4. `gh cli token storage location golang read without subprocess` — determine how to read gh's
   stored OAuth token in-process for Option 4
5. `git status porcelain subprocess latency macos exec.Command benchmark` — get measured overhead
   numbers to confirm the subprocess cost assumption
6. `github graphql search pull requests review required golang query` — verify the GraphQL query
   structure for finding PRs awaiting review across multiple repos
