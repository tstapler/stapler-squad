# Architecture Research: github-api-usage-tracking

## 0. Critical correction to the Baseline: rate-limit tracking is currently dead-wired

The requirements' Baseline section states `DefaultRateLimiter` "logs a WARN... and pauses
dispatch after a hard 403/429" as if this is active today. Verified by grep across the whole
repo: it is not.

- `github/rate_limit.go:23,47` reference a type called `rateLimitTransport` in doc comments
  ("Called automatically by rateLimitTransport on every response") — **this type does not
  exist anywhere in the codebase.** `grep -rn "rateLimitTransport" --include="*.go"` matches
  only those two comment lines.
- `RateLimiter.Update(resp *http.Response)` (`github/rate_limit.go:49`) — the method that
  parses headers, logs the WARN, and calls `setLimitedUntil` — **has zero callers** anywhere
  in the repo (`grep -rn "DefaultRateLimiter.Update\|\.Update(resp"` returns nothing but the
  declaration itself).
- `github/http_client.go:17` — `ghHTTPClient = &http.Client{Timeout: 30 * time.Second}` has
  no custom `Transport` set anywhere (`grep -rn "\.Transport ="` in `github/*.go` is empty).
  It uses `http.DefaultTransport` implicitly.
- The only things that *do* consume `DefaultRateLimiter` are `session/pr_status_poller.go:197`
  and `session/worktree_pr_poller.go:187`, which call `IsLimited()` before dispatching a poll
  batch. Since `Update()` is never called, `rateLimitedUntil` is always the zero value —
  `IsLimited()` always returns `false`. **The existing pause-after-403 behavior claimed in the
  Baseline does not currently execute; it is aspirational/orphaned code.**

Implication for this feature: "instrumenting `github/rate_limit.go`" is not a matter of adding
a request counter next to already-working header parsing — the header-parsing wiring itself
has to be connected for the first time. This raises the floor of the work but also means there
is no existing behavior to preserve/not-regress on this specific path (no test currently
exercises `Update()` being invoked from a live request, since nothing invokes it).

## 1. Instrumentation choke point: wrap the transport, not the call sites

`ghHTTPClient.Do(req)` is called directly from 14 distinct call sites across 7 files, with no
shared wrapper function:

```
github/etag_cache.go:68    github/repos.go:134,206,276,337
github/device_auth.go:79,137   github/commits.go:51
github/client.go:168,226,398,460,833   github/user_pr_cache.go:606
```
(`grep -n "ghHTTPClient.Do" github/*.go`)

Two options:

**A. Instrument each of the 14 call sites individually.** Rejected — this is the "more
invasive" option the requirements flag, and it doesn't scale: every future native call site
(and there is no code-review gate preventing a new one from being added ad hoc) would need to
remember to instrument itself. It also duplicates the exact header-parsing logic
`RateLimiter.Update` already has, just to also increment a counter.

**B. Wrap `ghHTTPClient.Transport` with a single `http.RoundTripper`.** This is the correct
architectural choice and is also the *only* way to make the currently-dead `Update()` call
actually fire — since nothing currently sets `ghHTTPClient.Transport`, adding one
`instrumentedTransport` type that (1) calls the real `http.DefaultTransport.RoundTrip`, (2)
calls `DefaultRateLimiter.Update(resp)` (finally wiring the existing warn/pause logic), and
(3) records a request-tracking event, fixes the orphaned-code issue and the new tracking
requirement in the same change, at a single choke point. This is presumably what the
`rateLimitTransport` doc comment describes as already existing — implementing it for real
should reuse that name (or `instrumentedTransport`) to match the pre-existing comments in
`rate_limit.go`.

```go
// github/http_client.go — sketch, not final
type instrumentedTransport struct {
    next http.RoundTripper // http.DefaultTransport when nil
}

func (t *instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    next := t.next
    if next == nil {
        next = http.DefaultTransport
    }
    start := time.Now()
    resp, err := next.RoundTrip(req)
    if err == nil {
        DefaultRateLimiter.Update(resp)          // finally wired
        recordRequestEvent(req, resp, time.Since(start)) // new tracking hook
    }
    return resp, err
}
```
This covers every present and future native call in `github/*.go` with one line
(`ghHTTPClient.Transport = &instrumentedTransport{}`) — no per-call-site changes needed for
requests that already go through `ghHTTPClient`.

**Attribution caveat**: a `RoundTripper` only sees the `*http.Request`, not caller context
(session ID, poller name). Per-source/per-worktree-session attribution (in scope, see §5) needs
the caller identity threaded through `context.Context` and read back out inside `RoundTrip`
via `req.Context()` — every native call site already takes a `ctx` (confirmed:
`newGHRequestForHostWithToken(ctx, ...)` at `github/http_client.go:85` and all 14
`ghHTTPClient.Do(req)` sites build `req` from a context-carrying constructor), so this is a
context-value addition, not a signature change to the 14 call sites themselves — only the
*originating* call site (poller/RPC handler) needs one line to stamp the context.

## 2. `gh` CLI shell-outs: a second, structurally different instrumentation path

`github/client.go` has at least 9 `safeexec.CommandContext(ctx, "gh", ...)` call sites, not
just the two the requirements name explicitly:

```
client.go:266  GetPRInfoCtx        → gh pr view
client.go:540  IsForkRepo (dead)   → gh api  (delete per requirements, don't migrate)
client.go:567  → gh pr view --json comments
client.go:611  → gh pr diff
client.go:634  → gh pr comment
client.go:667  → gh pr merge
client.go:689  → gh pr close
client.go:709  → gh repo clone
client.go:725/740/762 → git fetch/checkout/remote (not gh, no rate-limit relevance)
```

These bypass `ghHTTPClient` (and therefore the new `instrumentedTransport`) entirely — `gh`
CLI manages its own HTTP client and its own token resolution, invisible to this repo's Go
process. The two sub-options from the requirements' Rabbit Holes:

- **Wrap the CLI invocation**: parse `gh`'s own stderr/exit-code rate-limit signals (it does
  emit rate-limit info on 403) or just count "1 request" per invocation without header-level
  remaining/limit visibility. Cheaper, but produces lower-fidelity data than the native path
  (no remaining/limit/reset numbers, only a raw count) and leaves `DefaultRateLimiter`
  unaware of `gh`-CLI-driven quota consumption — the two counters (native transport vs. gh-CLI)
  would never reconcile against GitHub's single actual quota.
- **Migrate to native `http.Client`** (`github/http_client.go`'s pattern, e.g.
  `newGHRequestForHostWithToken` + `ghHTTPClient.Do`): every migrated call becomes visible to
  `instrumentedTransport` for free, and shares one accounting story with the rest of the
  package. This is "arguably more correct" per the requirements, and is the only path that
  keeps `DefaultRateLimiter`'s remaining/limit view accurate for 403/429 pause behavior — a
  gh-CLI-driven exhaustion would otherwise go undetected by the (now-finally-working) pause
  logic in §0/§1, defeating half the point of fixing it.

**Recommendation**: migrate `GetPRInfoCtx` only (the one confirmed live caller, see §3) in this
feature's scope, per the requirements' explicit in-scope list; leave the other 6 gh-CLI call
sites (`comments`, `diff`, `comment`, `merge`, `close`, `clone`) un-migrated but flag them in
the plan as follow-up debt with the same rationale — mixing "migrate everything" into a
Complexity-4 feature already touching many other call sites risks exactly the rabbit-hole the
requirements warn about. `IsForkRepo` (`client.go:540`) is separately marked for deletion, not
migration — requirements confirm it has no real callers outside tests.

## 3. Migration risk enumeration: `GetPRInfoCtx` → native

Confirmed sole caller: `session/pr_tracking.go:21`'s `Instance.RefreshPRInfo()` calls
`github.GetPRInfo(owner, repo, prNumber)` (`github/client.go:251-253`), which is a thin
`context.Background()` wrapper around `GetPRInfoCtx` (`github/client.go:257`). What could break
if `GetPRInfoCtx` moves from a `gh pr view --json <fields>` shell-out to a native
`GET /repos/{owner}/{repo}/pulls/{number}` call:

- **Field/shape parity**: `gh pr view --json` returns GitHub's *decorated* PR view (includes
  `reviewDecision`, `statusCheckRollup` computed fields that `gh` itself derives from multiple
  REST/GraphQL calls under the hood). The plain REST `pulls/{number}` endpoint does **not**
  include review-decision or check-rollup in one response — those require separate
  `/reviews` and `/commits/{sha}/check-runs` (or the GraphQL API) calls. A naive migration
  that just swaps the transport but keeps parsing the same JSON shape will silently drop
  fields (`PRInfo` struct fields fed from those, if any — needs a field-by-field diff against
  `PRInfo`'s definition before migrating, not assumed compatible).
- **Auth/host resolution divergence**: `gh` CLI resolves its token/host from `gh auth status`
  and `~/.config/gh/hosts.yml`, independent of this repo's `github/keychain.go` /
  `getGHTokenForAccount`. A user authenticated to `gh` CLI but not through stapler-squad's own
  keychain flow (or vice versa) would see different auth outcomes pre- vs. post-migration —
  worth an explicit compatibility check, not just "assume same token."
- **Error surface change**: `gh pr view`'s exit codes/stderr text differ entirely from HTTP
  status codes. `RefreshPRInfo`'s error wrapping (`session/pr_tracking.go:22-24`) and any
  caller-side error-string matching (grep for callers matching on `RefreshPRInfo`'s error text)
  need re-verification against the new error path — `classifyGHResponse`
  (`github/http_client.go:112-148`) already exists and is the right error classifier to reuse,
  but it changes the concrete error values callers see (e.g. `ErrGitHubAccessDenied` sentinel
  vs. a raw `exec.ExitError`).
- **Timeout behavior**: `gh` shell-outs run under `safeexec.CommandContext(ctx, ...)` with
  whatever timeout the caller's `ctx` carries; the native path uses `ghHTTPClient`'s hardcoded
  30s `Timeout` field (`github/http_client.go:17`) *in addition to* `ctx`. A caller passing a
  longer-lived context could see a new, tighter effective timeout post-migration.
- **Test coverage**: any existing test that stubs/mocks the `gh` binary (via a fake `PATH` entry
  or `safeexec` test seam) for `GetPRInfoCtx` needs an equivalent HTTP-level stub
  (`httptest.Server`, matching the existing `GhBaseURL` override pattern noted in
  `github/http_client.go:19-21`) — this is a test-infrastructure swap, not just a prod-code
  change. Confirm via `grep -rn "GetPRInfoCtx\|RefreshPRInfo" **/*_test.go` before starting the
  migration task in planning.

## 4. Data flow: request event → persisted store → RPC → web UI

### Correction to prior research framing (Rabbit Holes / token-monitoring/stack.md)

`server/services/analytics_store.go` is **not** in-memory-only — this corrects the "unconfirmed,
in-memory suspected" framing carried over from this round's brief. Verified:

- `AnalyticsStore` (`analytics_store.go:128-147`) holds a buffered channel
  (`ch chan AnalyticsEntry`, capacity 1000, `analyticsBufferSize` at line 138) and a
  `*session.Storage` reference — not a raw in-memory slice/map.
- `Record()` (line 175) is non-blocking: enqueues to the channel, drops + counts on overflow.
- A background goroutine started by `Start(ctx)` (line 151) runs `flush(ctx)` (line 551), which
  batches entries off the channel and calls `s.storage.RecordAnalytics(context.Background(), data)`
  (line 576) — i.e. every entry is written through `session.Storage` to durable storage (ent/SQL,
  per this repo's existing storage layer), not held only in process memory.
- Read paths (`LoadWindow`, `LoadProgramWindow` at line 290, `GetSubcommandBreakdown` at line
  303, `ListRecentCommands` at line 309) all go through `s.storage.List...`/`Get...` methods —
  reads also come from durable storage, so data really does survive a restart.
- `approval_handler_test.go:435` even asserts on this: `"analytics entry must persist within 2s"`
  polling `LoadWindow` after `Record()`, confirming durability is a tested property of this
  pattern, not incidental.

**This is good news for the Rabbit Hole "Historical storage choice"**: `analytics_store.go`'s
async-channel-buffer + background-flush-to-`session.Storage` pattern already satisfies the
requirement's persistence need (survives restart) at low risk, without introducing a new
storage mechanism. The architecture for this feature should reuse this exact shape — an
`APIUsageStore` (or similar) with the same `Record()`/`Start()`/`Stop()`/buffered-channel
skeleton, backed by a new ent schema table (or extending `session.Storage`'s existing
analytics table if the row shape is compatible) rather than inventing flat-JSON-file
persistence. This directly resolves one of the two Open Questions in requirements.md.

### Full chain (mirroring the `ApprovalAnalyticsPanel.tsx` precedent)

```
github/http_client.go:instrumentedTransport.RoundTrip()
        │  (per response: resource, remaining, limit, resetAt, status, caller-ctx source)
        ▼
new APIUsageStore.Record(event)      — server/services/, buffered chan, mirrors AnalyticsStore
        │  (async, non-blocking; background goroutine)
        ▼
APIUsageStore.flush() → storage.RecordAPIUsage(ctx, event)   — new session.Storage method,
        │                                                       new ent schema table
        ▼
Persisted store (ent/SQL) — survives restart
        │
        ▼
New ConnectRPC service method, e.g. rpc GetAPIUsageAnalytics(...)
        in proto/session/v1/session.proto, next to the existing
        rpc GetApprovalAnalytics (session.proto:155) /
        rpc GetProgramAnalytics (session.proto:159) — same message-shape convention
        (windowed query params in, aggregated + time-bucketed response out)
        │
        ▼  make proto-gen
web-app/src/gen/session/v1/*_pb.ts (generated client stubs)
        │
        ▼
New hook web-app/src/lib/hooks/useApiUsageAnalytics.ts
        — mirrors useApprovalAnalytics.ts's ConnectRPC query-hook shape
        │
        ▼
New web-app/src/components/sessions/ApiUsagePanel.tsx
        — mirrors ApprovalAnalyticsPanel.tsx's structure/CSS (.css.ts, vanilla-extract per
          .claude/rules/css-architecture.md) for drill-down + time-range selector UI
```

This is a mechanical extension of an existing, working, tested pattern — the main new work is
the event schema (resource, remaining, limit, source/caller attribution, timestamp) and the ent
migration, not new plumbing patterns.

## 5. Consistency: does poll-interval config need to propagate live?

**Confirmed: no hot-reload mechanism exists in `config/`.** `config/config.go:782`
(`LoadConfig()`) and `:847` (`LoadConfigFromPath()`) both parse JSON once, synchronously, at
call time — `grep -n "fsnotify\|Watch\|Viper\|watcher" config/*.go` returns zero matches. This
answers the first Open Question in requirements.md directly: **config is load-once; nothing in
this codebase currently re-reads it after startup.**

There is a directly relevant precedent already in `Config`: `DaemonPollInterval` (`config.go:247-248`,
default set at `config.go:430`) is exactly this shape — a poll interval sourced from config —
and it is likewise read once at daemon-start, not hot-reloaded. This feature's new
`PRStatusPollerPollInterval`/`WorktreePRPollerPollInterval`-equivalent config fields should
follow the same precedent for consistency, not invent a new hot-reload mechanism just for this
feature.

Separately, at the poller level: neither `PRStatusPoller` nor `WorktreePRPoller` exposes a
`SetPollInterval`/`Reconfigure` method — `grep -n "func (p \*PRStatusPoller)\|func (p
\*WorktreePRPoller)"` across both files shows only `Start`/`Stop`/`SetSource`/`SetOnUpdated`
lifecycle methods. The ticker is created once inside `pollLoop()` from `p.config.PollInterval`
(`pr_status_poller.go:178`, `worktree_pr_poller.go:155`) and never touched again.

**Recommendation**: treat "restart required to change poll interval" as the accepted answer —
consistent with `DaemonPollInterval`'s existing precedent, zero new mechanism needed, and
`make install-service` (the documented way to apply config changes) already restarts the
service. Building live propagation (e.g., swapping `time.NewTicker` for a `time.Timer` +
reset-on-config-change channel) is a real option but is net-new machinery this codebase has
never needed for any other poller — it should only be taken if Phase 3 planning judges the
restart-required UX as unacceptable, and if so, scope it as an explicit, separately-reviewable
addition rather than folding it silently into "config-driven intervals."

The same restart-vs-hot-reload question applies to the **configurable warn threshold**
(`rateLimitWarnPercent`, currently a `const` at `rate_limit.go:20`) — it needs to become a
`Config` field read by `RateLimiter` at construction, with the same load-once semantics as the
poll intervals, for consistency (and to avoid a second, different consistency model in the same
feature).

## 6. Event-Command-Policy table

This domain has enough distinct actors (two timer pollers, N on-demand RPC/MCP callers, gh-CLI
shell-outs, the rate limiter's own pause policy, and a human operator reading the UI) and
cross-cutting policy (warn-at-threshold, pause-on-exhaustion) to warrant an EventStorming-style
table rather than being simple CRUD:

| Domain Event | Policy trigger | Command | Actor/System |
|---|---|---|---|
| `GitHubRequestDispatched` | (none — direct action) | `RoundTrip(req)` | `PRStatusPoller`, `WorktreePRPoller`, on-demand RPC/MCP handlers, `gh`-CLI-migrated call sites |
| `GitHubResponseReceived` | Always, after every native/migrated call | `instrumentedTransport.RoundTrip` records headers | `instrumentedTransport` (new) |
| `RateLimitHeadersParsed` | On every `GitHubResponseReceived` | `RateLimiter.Update(resp)` | `DefaultRateLimiter` (now actually wired, see §0) |
| `APIUsageEventRecorded` | On every `GitHubResponseReceived` | `APIUsageStore.Record(event)` | `APIUsageStore` (new) |
| `RateLimitLow` (< configurable threshold %) | `RateLimitHeadersParsed` where `remaining < threshold` | Log WARN + persist warn event | `DefaultRateLimiter` |
| `SecondaryRateLimitHit` (429 / 403+Retry-After) | `RateLimitHeadersParsed` matches secondary-limit shape | `setLimitedUntil(until)` | `DefaultRateLimiter` |
| `PrimaryRateLimitExhausted` (remaining==0) | `RateLimitHeadersParsed` matches primary-exhaustion shape | `setLimitedUntil(until)` | `DefaultRateLimiter` |
| `PollDispatchSuppressed` | `PrimaryRateLimitExhausted`/`SecondaryRateLimitHit` still active at next tick | `IsLimited()` check short-circuits `checkAllSessions`/`pollWorktrees` | `PRStatusPoller`, `WorktreePRPoller` |
| `APIUsageEventFlushed` | Buffered channel drained (background goroutine) | `storage.RecordAPIUsage(ctx, event)` | `APIUsageStore.flush` (new) |
| `APIUsageQueried` | Operator opens the new UI panel / selects a time range | `GetAPIUsageAnalytics(req)` RPC | Tyler (via web UI), `ApiUsagePanel.tsx` |
| `PollIntervalConfigChanged` | Operator edits config + restarts service (no hot-reload, §5) | `LoadConfig()` at next process start | Tyler, `config.LoadConfig` |
| `WarnThresholdConfigChanged` | Same as above | `LoadConfig()` at next process start → `RateLimiter` construction | Tyler, `config.LoadConfig` |

## Summary

- The biggest architectural surprise is that `github/rate_limit.go`'s header-parsing/pause logic
  is currently **orphaned code with zero callers** — `rateLimitTransport` referenced in comments
  doesn't exist, and `ghHTTPClient` has no custom `Transport`. This feature's instrumentation
  work is really "build `instrumentedTransport` for the first time," not "extend an existing
  wrapper." Wrapping `ghHTTPClient.Transport` with one `http.RoundTripper` is the correct choke
  point — it covers all 14 existing `.Do()` call sites plus every future one, and fixes the dead
  wiring and the new tracking requirement in the same change.
- `server/services/analytics_store.go` **is disk-persisted** (buffered channel → background
  flush → `session.Storage`, confirmed by `RecordAnalytics`/`LoadWindow` and a test asserting
  "must persist within 2s") — this corrects the "in-memory suspected" framing carried into this
  round, and means the Rabbit Hole "Historical storage choice" has a low-risk, precedented
  answer: reuse this exact shape for a new `APIUsageStore`, backed by a new ent table.
  Full RPC→UI chain to follow: `GetApprovalAnalytics`/`GetProgramAnalytics` RPCs
  (`proto/session/v1/session.proto:155,159`) → `useApprovalAnalytics.ts` → `ApprovalAnalyticsPanel.tsx`.
- No hot-reload exists in `config/` (confirmed by grep — `LoadConfig`/`LoadConfigFromPath` are
  load-once), and neither poller exposes a reconfigure method — the existing `DaemonPollInterval`
  precedent (`config.go:247`) is also restart-only, so restart-required is the consistent,
  lowest-risk answer for both the poll-interval and warn-threshold config, resolving two of
  requirements.md's Open Questions. `gh`-CLI migration should be scoped to `GetPRInfoCtx` only
  (its sole caller is `session/pr_tracking.go:21`'s `RefreshPRInfo`) — six other `gh`-CLI call
  sites in `github/client.go` are real but explicitly out of this feature's migration scope to
  avoid the rabbit hole the requirements already flagged.
