# Architecture Research: github-call-optimization (Agent 3)

## 0. Correcting/updating the prior `github-api-usage-tracking` research

`project_plans/github-api-usage-tracking/research/architecture.md` (2026-08-10) is now
**partially stale** — read and cite, not re-derived, per its own valid parts:

- **Outdated**: its central finding ("`rateLimitTransport` doesn't exist, `Update()` has zero
  callers, `ghHTTPClient` has no custom Transport") is no longer true. `github/http_client.go:22-25`
  now sets `Transport: &rateLimitTransport{next: http.DefaultTransport}` on `ghHTTPClient`, and
  `rateLimitTransport.RoundTrip` (`http_client.go:40-56`) both fails fast on `IsLimited()` and
  calls `DefaultRateLimiter.Update(resp)` on every response. This exact wrapper was built,
  exactly where that doc recommended (§1, "wrap the transport, not the call sites"). Its sketch
  code and this project's `rateLimitTransport` are structurally the same shape.
- **Still valid and directly reusable**:
  - §2's `gh`-CLI-shell-out enumeration and the recommendation to migrate only the one live
    `GetPRInfoCtx` caller — this project's scope item 3 does exactly that.
  - §3's migration-risk list for `GetPRInfoCtx` (field/shape parity between `gh pr view --json`
    and REST, auth/host resolution divergence, error-surface change via `classifyGHResponse`,
    timeout-stacking with `ghHTTPClient`'s 30s `Timeout`, test-seam swap) — all five risks are
    unaddressed by anything shipped since; carry them into this project's plan verbatim.
  - §6's Event-Command-Policy table's `RateLimitHeadersParsed`/`PrimaryRateLimitExhausted`/
    `SecondaryRateLimitHit` rows are now *live* (they weren't in that doc's timeframe) — this
    project's own table (§7 below) builds on top of them rather than re-deriving rate-limit
    detection semantics.
- **Superseded by this project's own scope, not by code that shipped**: that doc's §4/§5
  (API-usage-analytics persistence layer, poll-interval hot-reload) belong to a different,
  separate feature (`github-api-usage-tracking`) and are out of scope here — this project's
  Scope item 1 (OTel spans/metrics via `telemetry.GetTracer()`/`GetMeter()`) explicitly rejects
  that doc's bespoke `APIUsageStore`/ent-table/RPC/UI-panel chain in favor of riding the existing
  telemetry pipe with no new storage, per the requirements' own text ("no new storage/UI").

## 1. Architectural patterns

### (a) Priority/admission control over the shared rate-limited resource

**Recommendation: an origin-tagged token-bucket-with-reserved-headroom gate layered in front of
the existing fail-fast check, not a priority queue or per-origin circuit breaker.**

Rationale, tied to what's already in this codebase:

- `DefaultRateLimiter` (`github/rate_limit.go:41-44`) already tracks one thing precisely: "are
  we currently past a known-bad point" (`rateLimitedUntil time.Time`, `sync.RWMutex`-guarded,
  read via lock-free-ish `IsLimited()`/write via `setLimitedUntil`). It does **not** track
  "how much *remaining* quota is left right now" — `remaining`/`limit` are parsed in `Update()`
  (`rate_limit.go:53-64`) only to decide the warn-threshold log and the reset-time math; they are
  discarded, not stored on the struct. A priority queue or circuit breaker needs richer state
  (queue depth, breaker half-open timers) that doesn't fit this codebase's existing
  "cheap atomic/RWMutex snapshot, no goroutine-driven state machine" style (same style as
  `PRStatusPoller.authState atomic.Value`, BUG-023's fix). A **reserved-headroom gate** only
  needs one more number the transport already sees for free: store `remaining`/`limit` on
  `RateLimiter` (two more `int` fields under the same `mu`, or promote to `atomic.Int64` pairs)
  and add `AdmitOrigin(origin string) bool`: interactive origins are admitted whenever
  `!IsLimited()`; background origins (`poller`, `backlog_sync`, `webhook_reconcile`) are
  additionally rejected once `remaining < headroomFor(origin)` (a configurable percentage of
  `limit`, mirroring `rateLimitWarnPercent`'s existing percentage-of-limit pattern at
  `rate_limit.go:17-20` — reuse that exact math, don't invent a second formula).
- This is strictly additive to `rateLimitTransport.RoundTrip` (`http_client.go:40-56`): the
  existing `if limited, until := DefaultRateLimiter.IsLimited(); limited { ... }` fail-fast stays
  as the hard floor for *everyone*; the new headroom check becomes a second, origin-aware
  short-circuit checked immediately after, using the origin recovered from `req.Context()` (see
  §3 below for how it gets there).
- A priority queue (goroutines block/reorder waiting for a token) is the wrong shape here because
  nothing in this codebase currently blocks callers on GitHub rate limits — both pollers already
  choose to **skip the tick entirely** on `IsLimited()` (`pr_status_poller.go:205-208`,
  `worktree_pr_poller.go:187-190`) rather than queue and retry. A priority-admission-control layer
  should preserve that same "skip, don't queue" semantics for background origins — it's a second
  independent trigger for the same skip-the-tick behavior the pollers already implement, not a new
  blocking primitive. Interactive callers (RPC handlers) have no such skip-and-retry-later option
  today (a user who clicked "refresh" needs *some* answer), so for them the gate should degrade to
  today's behavior (fail fast with the existing error) rather than silently succeeding past
  reserved headroom.
- A circuit-breaker-per-origin (open/half-open/closed with timers) is overkill: it exists to
  protect against a flaky *downstream* whose health is independently uncertain per-caller-class;
  here every origin shares one upstream (GitHub) and one already-observed piece of state
  (remaining quota) — a shared gauge with per-origin thresholds captures the same protection with
  far less state-machine surface, and is a natural extension of `RateLimiter`'s existing shape
  rather than a new component type.

### (b) Fan-in batching for poller-tick GraphQL coalescing

**Recommendation: a per-tick, explicitly-built aliased GraphQL query (the pattern
`docs/adr/020-graphql-for-user-pr-list.md` already established), not `singleflight`.**

- `singleflight.Group` (already imported and used in `github/http_client.go:96-136` for
  token-fetch coalescing, and referenced generically in `github/client.go:172`) coalesces
  **duplicate concurrent calls with the same key** into one in-flight call. That's the wrong
  shape for this problem: a poller tick's N tracked PRs are N *different* keys (different
  owner/repo/number tuples) that need to become **one** request, not N duplicate requests
  collapsing into one. `singleflight` has no batching/aggregation primitive — it dedups
  identical work, it doesn't combine distinct work items into a single call.
  See the `golang-concurrency` skill's coverage of `singleflight`'s scope for the same
  distinction if unsure.
  See also `github/user_pr_cache.go:516-549`'s query (already read for ADR-020) — it fetches an
  unbounded *set* (viewer's own PRs) in one call by construction, not by aggregating from
  per-item requests; it never had a fan-in problem to solve. This project's new query is
  different in kind: it must aggregate a caller-supplied *list* of already-known
  (owner, repo, number) tuples, one that changes every tick as sessions/worktrees come and go.
- The correct pattern is a **query builder that runs once per poll tick, inline in
  `checkAllSessions`/`pollWorktrees`**, not a background aggregation loop with its own
  scheduling: since both pollers already gate every tick's dispatch behind one ticker
  (`pollLoop`, `pr_status_poller.go:184-201` / `worktree_pr_poller.go:153-182` — the "single
  shared ticker per poller" constraint), the natural place to build the batch is exactly where
  `checkAllSessions`/`pollWorktrees` already assembles the instance/worktree list
  (`pr_status_poller.go:210-213`, `worktree_pr_poller.go:209-231`) — group the tracked
  `(owner, repo, prNumber)` tuples by `owner/repo`, alias each into one GraphQL document:

  ```graphql
  query BatchedPRStatus {
    repo0: repository(owner: "acme", name: "widgets") {
      pr0: pullRequest(number: 42) { ...prFields }
      pr1: pullRequest(number: 57) { ...prFields }
    }
    repo1: repository(owner: "acme", name: "other") {
      pr0: pullRequest(number: 9) { ...prFields }
    }
  }
  ```

  and issue exactly **one** `ghHTTPClient.Do` (or its migrated-to-native equivalent) per tick,
  replacing the current per-instance `sem := make(chan struct{}, ConcurrentFetches)` fan-out
  (`pr_status_poller.go:219-259`, `worktree_pr_poller.go:206-230`) entirely — no goroutines, no
  semaphore, needed for the fetch step itself (result-application/callback dispatch can stay
  per-item, sequential over the single response).
- **Conditional-request semantics don't carry over 1:1.** GraphQL has no per-field ETag/304
  concept the way the REST `pulls/{number}` endpoint does (`github/etag_cache.go:62-83`) — a
  batched GraphQL call always returns 200 with fresh data for every aliased field, at whatever
  point cost GitHub's GraphQL rate limiter assigns the query (see ADR-020's Negative section:
  "complexity budget per query"). This is the real trade-off scope item 3 is making: fewer HTTP
  round-trips (1/tick instead of up to `ConcurrentFetches` individual ones) in exchange for
  losing the "free" 304-costs-nothing win `ETagCache`/`GetPRInfoConditional` currently give
  *unchanged* PRs. The plan phase needs to decide whether `ETagCache` still gates *whether* a PR
  is included in the batch at all (e.g. skip a PR from the query if its cached entry's
  `updatedAt` is fresher than some staleness bound) — that's the only way to keep any
  cache-hit-avoidance benefit once ETags themselves stop applying to the batched path.

### (c) Webhook-driven cache invalidation into an existing poller

**Recommendation: the webhook handler calls a new exported `ETagCache.Invalidate(owner, repo,
prNumber)` (currently no such method exists — `get`/`set` are unexported,
`github/etag_cache.go:35-45`) directly, synchronously, then triggers the poller's existing
single-item reconciliation path — not a pub/sub or event-bus layer.**

- The `sync.Map`-backed `ETagCache` (BUG-022's fix) is lock-free for reads; adding
  `Invalidate(owner, repo string, prNumber int) { c.store.Delete(c.cacheKey(owner, repo,
  prNumber)) }` is a pure addition with zero risk to the existing read/write pattern —
  `sync.Map.Delete` is safe to call concurrently with `Load`/`Store` by design, so this needs no
  new locking at all.
- **This project already has a working template for "webhook delivery → targeted single-item
  reconciliation" — it's just aimed at a different consumer today.** `session/backlog_lifecycle_pr.go:1646-1660`'s
  `TriggerPRFixForEvent` (built by the already-shipped `pr-event-webhooks` project — see §2 below)
  does *exactly* this shape for `BacklogItem`/`pr_pending` rows: look up the tracked item for
  `(repoFullName, prNumber)`, then dispatch one targeted reconciliation instead of waiting for the
  next tick. Scope item 4 needs the **same shape for a second, structurally different tracked-item
  type** — `*Instance` (tracked by `PRStatusPoller.instances`, `pr_status_poller.go:53`) and
  `WorktreeScanItem` (tracked transiently per-tick by `WorktreePRPoller.pollWorktrees`,
  `worktree_pr_poller.go:186-232`) — which have no `TriggerPRFixForEvent`-equivalent lookup path
  today (`PRStatusPoller` has no `(owner, repo, prNumber) → *Instance` index; `GetInstances()`
  returns the full list, `pr_status_poller.go:118-126`, requiring a linear scan comparable to
  `findPRPendingItemForEvent`'s existing pattern).
- Recommended shape: a new method on each poller, e.g. `PRStatusPoller.InvalidateAndRefresh(ctx,
  owner, repo string, prNumber int) (matched bool)` that (1) calls `p.etagCache.Invalidate(...)`
  (2) linear-scans `p.GetInstances()` for a matching `(GitHubOwner, GitHubRepo, GitHubPRNumber)`
  triple, (3) if found, calls the existing per-instance `fetchAndUpdatePRStatus(inst)`
  (`pr_status_poller.go:285-363`) directly in a goroutine — reusing that function verbatim rather
  than duplicating its fetch/apply logic, exactly the "reuse the existing per-item body" principle
  `pr-event-webhooks`'s research already established for the backlog-item case. `WorktreePRPoller`
  needs the equivalent using its own `fetchAndStore` (`worktree_pr_poller.go:235-...`). No new
  pub/sub/event-bus component is needed — this is a direct method call from the webhook handler
  into two already-constructed, already-injected poller instances (both are long-lived singletons
  constructed once in `server/dependencies.go`, same wiring shape `ServerDependencies` already
  uses for `BacklogLifecycleListener`).
- **Does not threaten the "single shared ticker per poller" constraint.** The ticker
  (`pollLoop`'s `time.NewTicker`) stays the only thing that triggers a *bulk* tick; webhook
  invalidation triggers a narrowly-scoped, single-item, out-of-band refresh that runs
  concurrently with (not instead of) the ticker — the same relationship `WorktreePRPoller.pollLoop`
  already has between its ticker branch and its `ScanDone()`-triggered branch
  (`worktree_pr_poller.go:163-182`): two independent event sources feeding the same processing
  function, no new synchronization primitive required beyond what already coordinates those two.

## 2. Integration points (file:line map)

| File | Line(s) | What plugs in |
|---|---|---|
| `github/http_client.go` | `22-25` (`ghHTTPClient` var decl) | No change to the client itself; `rateLimitTransport` (below) is the extension point |
| `github/http_client.go` | `36-56` (`rateLimitTransport.RoundTrip`) | New origin-aware admission check, inserted after the existing `IsLimited()` fail-fast (line 48-50) and before dispatch (line 51); origin read from `req.Context()` (needs a context key added here, see §3) |
| `github/http_client.go` | `96-136` (`ghTokenSF singleflight.Group`, `getGHToken`) | Unchanged — cited only as the existing singleflight precedent to preserve, per constraints |
| `github/rate_limit.go` | `25` (`var DefaultRateLimiter`) | Needs new fields (`remaining`, `limit` ints, or an `atomic` pair) alongside `rateLimitedUntil`, populated in `Update()` |
| `github/rate_limit.go` | `49-140` (`Update`) | Store `remaining`/`limit` (currently parsed at lines 53-64 and discarded) onto the struct instead of only using them locally |
| `github/rate_limit.go` | new method | `AdmitOrigin(origin string) bool` — the priority-admission-control entry point (§1a) |
| `github/etag_cache.go` | `17-45` (`ETagCache`, `get`/`set`) | New exported `Invalidate(owner, repo string, prNumber int)` method, `sync.Map.Delete` on the same `cacheKey` |
| `github/etag_cache.go` | `53-130` (`GetPRInfoConditional`) | Its internal calls to `GetPRInfoCtx` (lines 109, 122) are exactly the calls scope item 3 needs migrated to native GraphQL/REST — once migrated, `ETagCache` gains native-path rate-limit/cache visibility for the first time, per requirements' explicit callout |
| `github/client.go` | `284-359` (`GetPRInfoCtx`) | Migration target — `gh pr view --json ...` shell-out replaced with GraphQL (single-PR query, reusing the aliasing shape from §1b for the batched-poller case, or a plain single-`pullRequest(number:)` query for the interactive-RPC case) |
| `github/client.go` | `299` | The `safeexec.CommandContext` call site being replaced |
| `github/client.go` | 8 other `safeexec.CommandContext(ctx, "gh", ...)` sites (`573, 600, 644, 667/700, 722, 742`) | Out of scope for migration per this project's own scope (only `GetPRInfoCtx`); still need the "one instrumented wrapper" from scope item 1 — see §4's disposition note on the two-executor-shape problem |
| `github/commit_status.go` | `90` (`safeexec.CommandContext(ctx, "gh", args...)`) | 9th `github/*.go` gh-CLI site; in the instrumentation wrapper's scope (item 1) even though not in the GraphQL-migration scope (item 3) |
| `github/cli_import.go` | `80` | 10th site, same disposition as above |
| `session/git/worktree_git.go` | `41` (`g.commandRunner().Run(ctx, path, "git", args...)`) | Shows the seam: `tmux.CommandRunner` (defined via `g.commandRunner()`, `session/git/worktree.go:366-369`) is generic (git *and* gh), not gh-specific — the instrumentation wrapper for this file's 11 gh-CLI sites cannot simply wrap `CommandRunner` wholesale without also instrumenting every git call |
| `session/git/worktree_git.go` | `71, 84, 301, 356, 390, 404, 674, 834, 854, 871, 885` | The 11 `gh`-prefixed `g.commandRunner().Run(...)` call sites — candidates for a new `runGHCommand(ctx, g, args...)` helper that wraps `commandRunner().Run` with the same telemetry/origin logic as `github/*.go`'s wrapper, filtered on the command name rather than the executor type |
| `session/git/util.go` | `194` (`gh auth status`) | 12th `session/git` site — same disposition |
| `session/pr_status_poller.go` | `184-201` (`pollLoop`), `204-262` (`checkAllSessions`) | Batching integration point (§1b) — replace the `sem`/`wg` fan-out (`219-259`) with one batched GraphQL call; new `InvalidateAndRefresh` method (§1c) for webhook wiring |
| `session/pr_status_poller.go` | `285-363` (`fetchAndUpdatePRStatus`) | Reused verbatim by the new webhook-triggered single-item refresh path (§1c) |
| `session/pr_status_poller.go` | `107-109` (`ETagCache()` getter) | Already exposes the shared cache for `WorktreePRPoller`; the new `Invalidate` needs no new plumbing to reach both pollers since they already share one `*github.ETagCache` instance |
| `session/worktree_pr_poller.go` | `152-182` (`pollLoop`), `186-232` (`pollWorktrees`) | Same batching integration shape as `pr_status_poller.go` |
| `session/worktree_pr_poller.go` | `235-` (`fetchAndStore`) | Reused verbatim by the webhook-triggered single-item refresh path, mirroring `fetchAndUpdatePRStatus` |
| `server/services/github_webhook_handler.go` | `90-99` (event-type switch in `Handle`) | Already routes `check_run`/`workflow_run`/`pull_request_review`/`issue_comment` to `handlePRFixEvent` (shipped by `pr-event-webhooks`) — scope item 4's new invalidation call is a **second** consumer added inside (or alongside) that existing branch, not a new route or new signature-verification path |
| `server/services/github_webhook_pr_fix.go` | `27` (`prFixEventTypes`), `68-97` (`extractPRFixEvent` + per-type extractors) | Already extracts `(repoFullName, prNumbers)` for exactly the 4 needed event types — scope item 4 reuses this extraction verbatim; no new parsing needed |
| `session/backlog_lifecycle_pr.go` | `1646-1660` (`TriggerPRFixForEvent`) | Existing template for "webhook → targeted single-item action," reused as the *design pattern* (not the code) for the new poller-invalidation consumer |
| `server/features/flags.go` | `1-28` | This file only registers RPC-level feature *descriptions* (`FlagsGet`/`FlagsUpdate` for the `GetFeatureFlags`/`UpdateFeatureFlag` RPCs) — it is **not** where flag values live or are checked; see `config/config.go:1486` (`GetFeatureFlag`) / `:1503` (`GetFeatureFlagWithDefault`) and `config/config.go:330` (`FeatureFlags map[string]bool`) for the actual live-settable mechanism scope items 2/3 must gate behind, following `webhook_triggers`'s existing pattern (`config/config.go:1479-1486`, checked at `github_webhook_handler.go:71` and again inside `handlePRFixEvent`'s own flag, per that file's "defense in depth, handler re-checks its own flag" comment) |
| `telemetry/telemetry.go` | `80-90` (`Provider`, `globalProvider`) | `GetTracer()`/`GetMeter()` (referenced by name in requirements; not yet grepped in this pass but implied by `Provider.tracer`/`Provider.meter` fields at lines 83/85) are the ride-along points for scope item 1's spans/metrics — no new telemetry plumbing needed, just call sites in `rateLimitTransport.RoundTrip` and the new gh-CLI wrapper(s) |

## 3. Data flow: origin tag and webhook invalidation

### Origin tag: RPC/poller → RoundTrip, and the precedent that resolves half the "rabbit hole"

The requirements flag "threading an origin tag through call chains several layers deep... `context.Context`
value vs. explicit parameter, undecided" as a Phase 3 planning decision. **This codebase already
answers the general shape of that question once, for a structurally identical problem**:
`session/backlog_lifecycle_pr.go:1662-1685` (`prFixTriggerSourceKey`, `withPRFixTriggerSource`,
`prFixTriggerSourceFrom`) tags a `context.Context` with `"webhook"` vs. `"poller"` so a single
shared function (`reconcilePRPendingItem`) can distinguish its caller for a metric, using an
unexported struct-typed context key (avoids collision, the idiomatic Go pattern) plus a
`With`/`From` accessor pair and a documented default (`"poller"` when untagged). This is the exact
shape scope item 1's origin tag needs:

```go
type githubCallOriginKey struct{}

const (
    OriginInteractive     = "interactive"
    OriginPoller          = "poller"
    OriginBacklogSync     = "backlog_sync"
    OriginWebhookReconcile = "webhook_reconcile"
)

func WithGitHubCallOrigin(ctx context.Context, origin string) context.Context {
    return context.WithValue(ctx, githubCallOriginKey{}, origin)
}

func githubCallOriginFrom(ctx context.Context) string {
    if v, ok := ctx.Value(githubCallOriginKey{}).(string); ok && v != "" {
        return v
    }
    return OriginInteractive // matches the "fail closed toward the stricter gate" default
}
```

placed in `github` (not `session`, since `rateLimitTransport.RoundTrip` — the reader — lives
there) with `session`/`server/services` callers wrapping their `ctx` at the outermost point that
knows its own identity: `server/services/github_service.go`'s RPC handlers wrap with
`OriginInteractive` (or leave untagged, since that's the default); `PRStatusPoller`/
`WorktreePRPoller`'s tick loops wrap with `OriginPoller`; `session/backlog_plugin_github.go`'s
issue sync wraps with `OriginBacklogSync`; the new webhook-triggered `InvalidateAndRefresh` (§1c)
wraps with `OriginWebhookReconcile`. **This resolves the "value vs. explicit parameter" question
as "value," matching the one precedent that already exists** — an explicit parameter would mean
threading a new argument through `SetCommitStatus` → `RefreshPRInfo` → `GetPRInfoCtx`
(`session/pr_tracking.go`, named directly in the requirements) and every other multi-hop chain,
which is exactly the kind of signature-pollution `prFixTriggerSourceKey` was already built to
avoid one call chain over.

**Caveat carried from the existing precedent's own scope**: `prFixTriggerSourceFrom` is read at
exactly one call site today (a log line). The new `githubCallOriginFrom` is read inside
`rateLimitTransport.RoundTrip` to make an actual admission-control *decision* — a materially
higher-stakes read than a log tag. This means a caller that forgets to wrap its `ctx` doesn't just
mislabel a log line, it silently gets `OriginInteractive`'s (no-extra-restriction) treatment even
if it's actually background work — a background caller that never wraps its context invisibly
escapes admission control entirely rather than failing loudly. Flag this as a real correctness
risk for planning: consider whether an untagged/default origin should default to the *most*
restrictive treatment (background-shaped) instead of the least, or whether a lint/`go vet`-style
check should verify every poller/backlog-sync call site wraps its context — this is exactly the
kind of gap `quality:reflect-and-fix`'s enforcement-ladder thinking is for once a first bug surfaces.

### Webhook-delivered invalidation flow

```
GitHub → POST /webhooks/github  (X-GitHub-Event: check_run|workflow_run|pull_request_review|issue_comment)
    │
    ▼
GitHubWebhookHandler.Handle (github_webhook_handler.go:90-99)
    │  — same HMAC-verify-per-candidate loop as today (unchanged)
    ▼
handlePRFixEvent (github_webhook_pr_fix.go) → extractPRFixEvent → (repoFullName, prNumbers, actionable)
    │
    ├──────────────────────────────────────────────┐
    ▼                                               ▼
PRFixEventRouter.TriggerPRFixForEvent          NEW: PollerInvalidationRouter (name TBD in Phase 3)
(existing, unchanged — BacklogItem/pr_pending)  .InvalidateAndRefresh(ctx, owner, repo, prNumber)
    │                                               │
    ▼                                               ├─→ PRStatusPoller.InvalidateAndRefresh (§1c)
session.BacklogLifecycleListener                    └─→ WorktreePRPoller.InvalidateAndRefresh (§1c)
(session package)                                        (session package)
```

**No new tight coupling between `server/services` and `session` beyond what already exists**: the
existing `PRFixEventRouter` interface (`server/services/github_webhook_pr_fix.go:21-23`) is
already defined at the consumer (`server/services`) per the interface-pollution-checklist, and
`GitHubWebhookHandler` already holds one `session`-satisfied interface field
(`prFixRouter PRFixEventRouter`). The new poller-invalidation consumer should follow the **same**
pattern — a second narrow interface defined in `server/services` (e.g. `PRPollerInvalidator`
with one method), satisfied by a small adapter type in `session` (or by `PRStatusPoller`/
`WorktreePRPoller` directly, if both expose the same method name/signature and a two-field struct
in `server/services` holds both). This is additive to `GitHubWebhookHandler`'s existing
constructor-injection wiring (`NewGitHubWebhookHandler`, `github_webhook_handler.go:48-58`) — one
more field, one more constructor parameter, wired in `server/dependencies.go` next to wherever
`PRStatusPoller`/`WorktreePRPoller` are already constructed as long-lived singletons (need to
confirm their exact construction site in Phase 3 planning; not grepped in this pass since it's a
mechanical wiring detail, not an architectural one).

**Important scoping distinction the requirements' own wording papers over**: scope item 4 says
"invalidate the affected **session's** ETag entry" — but the webhook payload only carries
`(repoFullName, prNumber)`, and `PRStatusPoller` tracks `*Instance` values, while
`WorktreePRPoller` tracks ad hoc `WorktreeScanItem`s with no persistent identity between ticks.
"The affected session" is really "whichever `*Instance`(s) currently have `GitHubOwner/GitHubRepo/
GitHubPRNumber` matching the event" — plural in principle (nothing prevents two `*Instance`s
tracking the same PR, e.g. a duplicate session), and `WorktreePRPoller`'s worktrees are looked up
fresh every tick from `p.source.GetWorktrees()` (`worktree_pr_poller.go:159-179`) rather than held
in a persistent list, so its half of the invalidation is really just "clear the shared
`ETagCache` entry and let the next natural trigger (ticker or `ScanDone()`) pick it up fresh" —
it has no per-item "refresh this one now" hook to call into the way `PRStatusPoller` does, unless
one is added. Flag this as a concrete Phase 3 design decision: does `WorktreePRPoller` get a new
`RefreshOne(ctx, repoPath, branch)` entry point, or does invalidating the shared `ETagCache` alone
suffice (accepting up to one full `PollInterval`'s staleness for the worktree-only, non-session
case, which is arguably fine since worktree PRs have no interactive session watching them anyway)?

## 4. Tech debt disposition

**Extend as-is** for `github/rate_limit.go`, `github/http_client.go`, and `github/etag_cache.go` —
these three files already show the exact concurrency idioms this project should keep using
(`atomic`/`sync.Map`/`sync.RWMutex` chosen per BUG-022/023's now-fixed guidance, singleflight for
coalescing) and none of BUG-022/023/080/081 point at a structural problem in *these* files that
would block adding an admission-control gate or an `Invalidate` method — they're small,
single-responsibility files that took a fix once and stayed fixed (BUG-022/023 status:
✅ RESOLVED, re-verified 2026-07-22, no regression found in this pass).

**Isolate via seam** for the "22 gh CLI subprocess sites" as a *whole*, not extend-as-is — because
they are not architecturally uniform. §2's map shows two structurally different execution
primitives: `github/*.go`'s 10 sites call `safeexec.CommandContext` directly (no injected seam,
no test double beyond faking `PATH`), while `session/git/*.go`'s 12 sites call
`g.commandRunner().Run(ctx, path, "gh", args...)` through the already-existing, already-injectable
`tmux.CommandRunner` interface (`session/git/worktree.go:366-369`) — the same interface git calls
also use. "One instrumented wrapper collapsing 22 sites" (as the requirements phrase it) cannot
literally be one function; it needs to be **two thin adapters converging on one shared
instrumentation core** (record span/metric/origin/call-site, then delegate) — one wrapping
`safeexec.CommandContext` for `github/*.go`, one wrapping (or filtering by command name inside)
`tmux.CommandRunner.Run` for `session/git/*.go`. This is a seam-isolation job, not a rewrite: no
call site's actual `gh` invocation/argument list needs to change, only how each site invokes it.

**Refactor-first is not warranted** for either poller file — `PRStatusPoller`/`WorktreePRPoller`
already went through BUG-023's atomic/lock-free conversion and read as clean, single-purpose
files with a consistent internal shape (ticker loop → snapshot instance list → bounded-concurrency
fan-out → per-item fetch/apply). The batching change (§1b) *replaces* the fan-out block
(`pr_status_poller.go:219-259`, `worktree_pr_poller.go:206-230`) but doesn't require restructuring
anything around it — the ticker, snapshot, and per-item apply logic all stay as-is.

## 5. Failure modes specific to migration/rollout

1. **Flag-flip mid-rollout leaves `ETagCache` in a mixed key state.** If the GraphQL-batching flag
   (scope item 3) is flipped on, then off, then on again while sessions are actively polling, some
   ticks populate `ETagCache` entries via the old per-PR REST conditional path
   (`GetPRInfoConditional`, keyed by `owner/repo/prNumber` string, `etag_cache.go:31-33`) and
   others never touch `ETagCache` at all (a batched GraphQL response has no per-PR ETag to store).
   A PR whose entry was last written by the REST path, then the flag flips to GraphQL-only for a
   few ticks, then flips back — its stale REST-cached ETag is still sitting in the map and will be
   sent as `If-None-Match` on the next REST-path tick, which is *harmless* (worst case: an
   unnecessary 304 vs. a slightly-stale cache), **but** if the plan's design has GraphQL responses
   ever *write into* the same `ETagCache` (e.g. to keep `GetPRInfoConditional`'s fallback path
   warm), a half-populated cache (some entries have real ETags, some have GraphQL-sourced
   zero-value ETags) needs an explicit invariant: an entry with `etag == ""` must always be
   treated as "no conditional request possible," never as "value known to be empty," or a future
   `GetPRInfoConditional` call could send `If-None-Match: ""` and get undefined behavior from
   GitHub. Verify this explicitly in the plan/implementation phase — `etagEntry.etag`'s zero value
   already means "no ETag yet" per current code (`GetPRInfoConditional`'s `hasCached && entry.etag
   != ""` check, `etag_cache.go:68`), so this is likely already safe, but the GraphQL-write path
   must not introduce a code path that writes a *real but stale* ETag string next to
   GraphQL-sourced data, which would look valid but not correspond to the data actually cached.

2. **Partial GraphQL rollout: some sessions on gh-CLI `GetPRInfoCtx`, some on native GraphQL,
   simultaneously.** Since the flag gates behavior per-process (not per-session,
   per `server/features/flags.go`'s live-settable-but-global model, confirmed via
   `config.Config.GetFeatureFlag` being a single process-wide config read, no per-session
   override mechanism found), a flip mid-flight affects *every* in-flight poll tick and RPC call
   from that point forward, not a gradual per-session cutover — so "partial rollout" in this
   codebase's actual flag mechanism means "partial across time" (before-flip calls used gh-CLI,
   after-flip calls use GraphQL), not "partial across sessions" at any given instant. The risk is
   narrower than a canary-style gradual rollout would imply: at most one in-flight request per
   call site straddles the flip (started under the old path, the flag read happens once at call
   entry per the existing `webhook_triggers`-style "handler re-checks its own flag" pattern). The
   real risk is **field/shape drift being silently absorbed differently by callers who assume one
   shape**: `session/pr_tracking.go`'s `RefreshPRInfo` (the requirements' named example) consumes
   `PRInfo` fields regardless of which path populated them — if the GraphQL migration's field
   mapping has any gap (per the inherited §3 migration-risk list from `github-api-usage-tracking`'s
   research, §0 above), the bug only manifests intermittently, exactly when a flag flip happens to
   land between two polls of the same PR, making it look like a flaky/transient bug rather than a
   deterministic migration gap. Mitigation: a field-by-field parity test asserting
   `GetPRInfoCtx`'s gh-CLI output and its GraphQL replacement produce byte-identical `PRInfo`
   structs for the same live PR, run in CI before the flag can default to on.

3. **Priority-admission-control's blast radius precedes its own flag check.** The requirements
   name this exact risk directly: "a priority-admission-control bug shipped behind a flag but the
   flag's blast radius still hits shared state before the flag check happens." Concretely: if
   `AdmitOrigin`'s implementation (§1a) is added *inside* `RateLimiter.Update()` itself (e.g.
   storing `remaining`/`limit` unconditionally, which this design already recommends doing
   unconditionally since it's harmless bookkeeping) versus *only* the admission-decision branch in
   `rateLimitTransport.RoundTrip` being flag-gated — the storing-side must ship unconditionally
   (cheap, side-effect-free) while only the **decision** side (rejecting a request because of
   headroom) must check the flag. Getting this split wrong — e.g. gating the whole `Update()` call
   behind the flag instead of just the new rejection branch — would silently stop updating
   `remaining`/`limit` (and therefore the *existing*, already-shipped warn-threshold log at
   `rate_limit.go:82-89`) whenever the new feature is flagged off, an unrelated regression to
   already-working behavior. The plan must specify precisely which lines are inside the flag check
   and which aren't, at the statement level, not just "gate the feature."

4. **`gh` CLI's own retry/backoff fighting `DefaultRateLimiter`'s fail-fast (requirements' own
   flagged rabbit hole).** This applies specifically to the *unmigrated* 21 remaining gh-CLI sites
   (everything except the now-migrated `GetPRInfoCtx`) — `gh` the binary has its own internal
   retry-on-5xx/secondary-rate-limit behavior invisible to `DefaultRateLimiter`. Once scope item 1's
   instrumentation wrapper (§4's "isolate via seam") is in place around those sites, a rollout
   sequencing risk emerges: if the wrapper is implemented to *also* call
   `DefaultRateLimiter.setLimitedUntil` based on gh's exit code/stderr (tempting, since it would
   give gh-CLI-driven exhaustion the same visibility native calls get, resolving the prior research's
   §2 "the two counters would never reconcile" gap) — a `gh` invocation that internally retried and
   *succeeded* after its own backoff would look, from the wrapper's outside view, like a single
   slow call with no error, while a `gh` invocation that retried and *still* failed would report a
   single terminal failure with no indication of how many attempts (and how much of the shared
   quota) it silently burned along the way. This makes gh-CLI-attributed rate-limit accounting
   inherently lossy compared to the native path's exact-header visibility — worth stating in the
   plan as an accepted limitation (call-count metrics for gh-CLI sites are "at least 1 attempt,
   possibly more" rather than exact) rather than something the wrapper can fully solve.

## 6. Event-Command-Policy table

| Domain Event | Policy trigger | Command | Actor/System |
|---|---|---|---|
| `GitHubRequestDispatched` (native HTTP) | always | `rateLimitTransport.RoundTrip(req)` | Any caller via `ghHTTPClient` |
| `GitHubCLIInvoked` (gh subprocess) | always | new instrumented wrapper(s) around `safeexec.CommandContext` / `commandRunner().Run` (§4) | `github/*.go` (10 sites) / `session/git/*.go` (12 sites) |
| `GitHubResponseReceived` | after every native RoundTrip | `DefaultRateLimiter.Update(resp)` (existing, unchanged) + new OTel span/metric record (scope item 1) | `rateLimitTransport` |
| `RateLimitHeadersParsed` | on every `GitHubResponseReceived` | store `remaining`/`limit` on `RateLimiter` (new — currently discarded) | `RateLimiter.Update` |
| `BackgroundOriginNearHeadroom` (`remaining` below background-reserved threshold) | on `RateLimitHeadersParsed`, when caller origin ∈ {poller, backlog_sync, webhook_reconcile} | `AdmitOrigin` rejects; poller's tick short-circuits exactly like an `IsLimited()` hit today | `RateLimiter.AdmitOrigin` (new) → `PRStatusPoller`/`WorktreePRPoller`/backlog sync |
| `InteractiveCallAdmitted` | always, for `OriginInteractive` | request proceeds even if a background-origin call would have been rejected at the same headroom level | `rateLimitTransport` |
| `PollTickDue` | ticker fires (unchanged, single shared ticker per poller) | build batched GraphQL query over all tracked `(owner, repo, prNumber)` tuples (§1b), issue one call | `PRStatusPoller.pollLoop` / `WorktreePRPoller.pollLoop` |
| `GitHubWebhookPRSignalReceived` (`check_run`/`workflow_run`/`pull_request_review`/`issue_comment`, already shipped) | signature verified, flag on, PR number extracted (all existing, `pr-event-webhooks`) | `TriggerPRFixForEvent` (existing, unchanged) **and, new,** `ETagCache.Invalidate` + poller `InvalidateAndRefresh` (§1c) | `GitHubWebhookHandler.handlePRFixEvent` → both `BacklogLifecycleListener` (existing) and the new poller-invalidation consumer |
| `ETagCacheInvalidated` | on `GitHubWebhookPRSignalReceived`'s new consumer | targeted `fetchAndUpdatePRStatus`/`fetchAndStore` re-run for the matching instance/worktree, out of band from the ticker | `PRStatusPoller`/`WorktreePRPoller` (new methods) |
| `GetPRInfoCtxMigrationFlagOn` | operator flips the rollout flag | `GetPRInfoCtx` internally dispatches to GraphQL instead of `gh pr view`; `ETagCache`/`DefaultRateLimiter` see it for the first time | `config.Config.GetFeatureFlag` → `github.GetPRInfoCtx` |

## Summary

- The prior `github-api-usage-tracking` research's central premise (rate limiting not wired up)
  is now **stale and fixed** — `rateLimitTransport` exists and is wired
  (`github/http_client.go:22-25,36-56`). Its migration-risk enumeration for `GetPRInfoCtx`
  (§3 of that doc) remains fully valid and unaddressed; carry it forward rather than re-deriving.
- The two hardest new-code decisions are (1) admission control needs `RateLimiter` to start
  *storing* `remaining`/`limit` (currently parsed and discarded) rather than being a wholly new
  component, and (2) the "22 gh-CLI sites, one wrapper" framing in the requirements actually
  requires **two** adapters (`safeexec.CommandContext` in `github/*.go` vs. the already-injectable
  `tmux.CommandRunner` seam in `session/git/*.go`) converging on one shared instrumentation core —
  not literally one function.
- Webhook-driven poller invalidation (scope item 4) has a **direct, already-shipped precedent to
  copy the shape of**: `session/backlog_lifecycle_pr.go:1646-1660`'s `TriggerPRFixForEvent`,
  built by the already-completed `pr-event-webhooks` project, already does "webhook event →
  targeted single-item reconciliation" for `BacklogItem`s. This project needs the same shape for
  `PRStatusPoller`/`WorktreePRPoller`'s tracked instances/worktrees, which have no equivalent
  lookup-by-`(owner,repo,prNumber)` index today — that's genuinely new, small code, not a
  redesign. The origin-tag rabbit hole also already has a working precedent in the same file
  (`prFixTriggerSourceKey`/`withPRFixTriggerSource`/`prFixTriggerSourceFrom`,
  lines 1662-1685) — reuse that exact `context.Context`-value shape, with the caveat that this
  project's read site is a rate-limit *decision*, not just a log tag, raising the stakes of an
  untagged/default-origin caller silently escaping admission control.
