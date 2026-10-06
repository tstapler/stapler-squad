# Features Research: github-call-optimization

Agent 2 (Features), Phase 2 research.

## 1. Comparable implementations already in stapler-squad

### `server/services/quota_gate.go` — `QuotaGate` (strongest comparable, model this on it)

This is the closest existing analogue to "background-origin callers back off before
interactive callers start failing" (scope item 2), just applied to Claude-session
token quota instead of GitHub API quota:

- **Foreground-vs-background distinction already exists as a first-class concept.**
  `foregroundSessionActive()` (`quota_gate.go:181`) classifies instances via
  `snap.Category != session.CategoryBacklog && snap.Status == session.Active` —
  i.e. it already has a two-way origin split (interactive/foreground vs.
  backlog/background) baked into `session.Category`. A GitHub-call "origin" tag
  should reuse/extend this category system rather than invent a parallel enum —
  `interactive`/`poller`/`backlog_sync`/`webhook_reconcile` from the requirements
  don't map 1:1 onto `session.Category`, but the *foreground throttles, background
  never blocks foreground* relationship is exactly `ShouldThrottleForeground()`'s
  job (`quota_gate.go:144`), just for a different resource.
- **Soft signal + hard signal, evaluated independently, hard always wins.**
  `Reconcile()` (`quota_gate.go:200`) checks a reactive "hard override" (a rate-limit
  event fired in the last N minutes — `RateLimitAggregate`) before ever consulting
  the proactive "soft" heuristic (`computeHeadroom`, a %-of-budget estimate). This
  maps directly onto GitHub's real primary/secondary rate-limit split
  (`github/rate_limit.go`'s own doc comment already distinguishes them) — the new
  admission control should treat "GitHub told us we're limited" (hard) and "we're
  approaching self-imposed background budget" (soft, if one is added) the same way:
  hard short-circuits soft, and resets soft's hysteresis counters when it fires
  (`quota_gate.go:255-256`, called out explicitly as a bug they already had to fix).
- **Hysteresis via consecutive-tick counters, not a single sample.** `consecutiveBelow`/
  `consecutiveAbove` (`quota_gate.go:66`) require `ConsecutiveTicksToPause`/
  `ConsecutiveTicksToResume` consistent ticks before flipping state — avoids
  flapping on a single noisy sample. A GitHub background-caller pause/resume
  decision should use the same pattern rather than reacting to one rate-limit
  header.
- **Provenance tracking for manual overrides.** `state.lastSetEnabled`/
  `manualOverrideAt` (`quota_gate.go:64-65`, `232-241`) detect when a human
  toggled the gated feature externally and grant a grace window
  (`ManualOverrideGraceMinutes`) before re-asserting automated control. If the new
  design ever lets a user manually force background callers to resume despite a
  pause, steal this pattern instead of a bare boolean flag.
- **Notification discipline**: cooldown-gated (`quotaGateNotifyCooldown`, 5 min),
  bypassed only within the manual-override grace window, coalesced onto a stable
  synthetic key (`quotaGateNotifierKey`) so repeated pause/resume events collapse
  in notification history instead of spamming. Directly reusable for "GitHub
  background sync paused due to rate limit" notifications (see §3, unstated needs).
- **Not a queue — a gate.** QuotaGate doesn't queue and dispatch background work
  itself; it flips `FeatureController.Enable()/Disable()` on `BacklogController`
  and lets that stop dispatching new work. The GitHub equivalent (scope item 2)
  is closer to this than to a request-queue: something that tells
  `PRStatusPoller`/`WorktreePRPoller`/`SyncLoop` "don't start your next tick,"
  not something that queues in-flight HTTP calls with different weights.

### `github/priority.go` — naming collision to avoid, not a comparable

`PRPriority` (`blocking`/`ready`/`pending`/`draft`/...) is a **derived UI-display
priority** for a single PR's review state — completely orthogonal to GitHub-call
admission priority. Do **not** call the new origin/priority concept `Priority`
without a distinguishing prefix (e.g. `CallOrigin`, as the requirements already
lean toward) — this file already owns that bare name in the same package
(`package github`), and a second unrelated `Priority` type in the same package
would be actively confusing.

### `session/command_queue.go` — a genuine priority queue, different problem shape

`CommandQueue` (`command_queue.go:87`) is a real container/heap `priorityQueue`
(int `Priority` field, higher-first, FIFO tiebreak on timestamp) for ordering
tmux command execution within one session. This is the right pattern *if* the
design ever needs to order multiple pending GitHub calls against each other
(e.g. "drain this session's queued GitHub actions in priority order"), but it's
solving a different problem than admission control against an external quota —
it doesn't back off, throttle, or fail-fast; it just orders what's already been
accepted. Scope item 2 needs a gate/breaker (QuotaGate-shaped), not a bigger
heap.

### Poller/consumer landscape — a third+fourth category beyond the two named in scope

Scope item 2 lists background-origin callers as "the two PR-status pollers,
backlog issue sync." Grepping confirms **three** ticker-driven consumers, not
two, plus the webhook path as a fourth non-ticker origin:

1. `session.PRStatusPoller` (`session/pr_status_poller.go:52`) — session-backed,
   one ticker over `SetInstances()`-registered instances, ETag-cached.
2. `session.WorktreePRPoller` (`session/worktree_pr_poller.go:78`) — worktree-backed
   (sessions *and* orphan worktrees with no live session), reacts to a
   `WorktreeSource` scan completion **or** a fallback ticker
   (`pollLoop`, lines 152-185) — already has the "event-driven with ticker
   fallback" shape scope item 4 wants to add to the PR-status pollers.
3. `session.SyncLoop` (`session/backlog_sync.go:34`) — the actual "backlog issue
   sync" ticker (`Start`/`runAllSources`, `backlog_sync.go:73,101`), separate
   Go type from both PR pollers, driven by `BacklogService.TriggerSync` /
   `SyncLoopForForwardSync()` (`server/services/backlog_service_sync.go:171,226`).
   This is scope item 2's "backlog_sync" origin tag — confirmed as its own
   ticker/consumer, not a subset of the PR pollers.
4. `server/services/github_webhook_handler.go` + `github_webhook_pr_fix.go` —
   not ticker-driven at all; inbound push-based. Currently wired only to the
   autonomous PR-fix loop (per requirements), not to invalidate the three
   pollers above (that's scope item 4's job). Its own outbound calls (signature
   verification is local; but reconcile actions it triggers make outbound GitHub
   calls) need the `webhook_reconcile` origin tag distinct from `webhook` itself
   as an inbound-delivery source — worth confirming in planning whether
   "webhook_reconcile" tags the *outbound* calls a webhook triggers or the
   webhook receipt itself.

**Confirming the origin-threading rabbit hole with a concrete call chain.**
`session/pr_tracking.go`'s `Instance.RefreshPRInfo()` (line 14, wrapping
`GetPRInfoCtx`) has both an interactive caller
(`server/services/github_service.go:62`, the RPC handler behind the UI's
manual-refresh action) and internal/background callers (`Instance.SetCommitStatus`,
`pr_tracking.go:100`, and another call at `pr_tracking.go:184`) — same method,
same call depth, genuinely different origin depending on who invoked it.
`RefreshPRInfo()`'s signature today has no room for an origin parameter, so
whichever mechanism the plan phase picks (context.Context value vs. explicit
parameter) must retrofit through this exact function without breaking its three
existing call sites — this is live evidence for the rabbit hole, not a
hypothetical.

So the origin taxonomy needs (at minimum) 5 tag values in practice:
`interactive`, `pr_status_poller`, `worktree_pr_poller`, `backlog_sync`,
`webhook_reconcile` — collapsing the two PR pollers into one `poller` tag (as
requirements.md's list implies) loses the session-backed/worktree-backed
distinction that's already load-bearing elsewhere in the code (different
structs, different config, different backoff state — `noPRPollAfter` in
`pr_status_poller.go` vs `listCacheEntry`/`setNoPRBackoff` in
`worktree_pr_poller.go`). Recommend keeping them distinguishable in the
observability tags (span attribute can carry both a coarse `origin=poller` and
a finer `poller_kind=pr_status|worktree_pr` without forcing the rate-limiter
itself to treat them differently).

## 2. Edge cases the design must handle

- **Webhook arrives for a PR/branch the poller doesn't know about (no ETag
  entry).** `ETagCache` (`github/etag_cache.go:17`) is a `sync.Map` keyed by
  `owner/repo/prNumber` (`cacheKey`, line 30), populated lazily on first poll.
  A webhook-driven invalidation (scope item 4) has nothing to invalidate in this
  case — the correct behavior is "trigger an immediate poll/fetch for that
  session if one exists" rather than "invalidate a cache entry," since
  invalidation of a nonexistent entry is a no-op by construction (`sync.Map`
  won't error on a missing key). The design must route webhook deliveries by
  `(owner, repo, branch)` → session lookup first, and only fall back to
  cache-key invalidation when a session/worktree is already tracking that PR.
  If no session matches at all (e.g. PR closed before a worktree was ever
  created), the delivery should be a documented no-op, not an error.
- **Feature flag flipped off mid-poll-tick while a GraphQL request is
  in-flight.** `ETagCache`'s `sync.Map` has no versioning/generation field
  (`etagEntry` is just `{etag, prInfo}`, `etag_cache.go:20`) — a REST-fetched
  entry and a GraphQL-fetched entry for the same key are structurally
  interchangeable today (both produce a `*PRInfo`). The risk isn't corruption of
  the cache *entry* itself (last-write-wins on a `sync.Map.Store` is safe), it's
  a **stale ETag being replayed against the wrong endpoint** if a REST
  conditional request is issued using an ETag that was actually returned by (or
  intended for) a GraphQL call, since GraphQL's `POST /graphql` doesn't carry
  `ETag` semantics at all (see the conditional-requests tension below). Design
  should treat the in-flight request as owning its own result — let it complete
  and land normally; simply route the *next* tick through whichever path the
  now-current flag value selects. No cancellation is needed as long as the
  cache write path never assumes the ETag it's about to overwrite came from the
  same protocol.
- **GraphQL batch partially fails (one alias errors, others succeed).**
  GitHub's GraphQL API returns partial data + a top-level `errors` array for
  this case (not a single hard failure) — the existing `userPRGraphQLQuery`
  pattern (`github/user_pr_cache.go:516`) doesn't yet handle partial-alias
  failure since it queries a single `viewer.pullRequests` connection, not N
  aliased single-PR lookups. The new batched `GetPRInfoCtx`-equivalent must
  parse per-alias results independently and apply per-PR fallback (skip that
  PR's session for this tick, log/metric it, retry next tick) rather than
  discarding the whole batch — otherwise one deleted/inaccessible PR poisons
  every other tracked PR's freshness every tick.
- **User explicitly clicks refresh while `DefaultRateLimiter.IsLimited()` is
  true.** Current code has exactly one behavior: `WaitIfLimited` blocks until
  clear or `ctx` cancellation (`github/rate_limit.go:153`) — there's no
  fail-fast path today at all, interactive or otherwise. Given this is a
  personal single-user tool (see §3), the right default for an *interactive*
  action during a **primary** rate limit (reset could be up to an hour away,
  `rate_limit.go:128-138`) is almost certainly fail-fast with a clear "rate
  limited until HH:MM, background sync paused" message — waiting an hour on a
  manual merge click is not acceptable UX. For a **secondary** rate limit
  (capped at `maxRetryAfterSleep` = 60s, line 15), a short bounded wait with a
  visible "retrying in Ns" indicator is plausibly fine and matches what
  `octokit/plugin-throttling`'s `onSecondaryRateLimit` retry-with-backoff does
  (see §4). This should probably be threshold-based on the wait duration, not a
  blanket rule — needs a planning-phase decision, not an engineering default.
- **PR spans a fork.** `pr_status_poller.go:242` already special-cases
  `instSnap.GitHub.GitHubIsFork` by skipping upstream lookup entirely ("Phase 2"
  TODO in the log line itself — this is explicitly unfinished, not a deliberate
  permanent skip). A GraphQL batch query aliases PRs by `owner/repo/number` in a
  single request; a fork PR's number is on the **fork's own repo**, not the
  upstream repo the session is nominally tracking, so it cannot share a batch
  keyed by the upstream repo without querying a second repo in the same
  request (GraphQL supports multiple root-level aliased fields across
  different repos in one query, so this is technically possible) — but since
  fork lookups are already excluded pre-batching today, the pragmatic scope-4
  answer is: continue excluding fork sessions from the batched query for now
  (consistent with existing behavior) and file the "Phase 2" fork-PR lookup as
  explicitly out of scope for this project too, rather than silently
  inheriting an already-incomplete feature into the new batching path.

## 3. Unstated needs (personal-tool usability expectations)

- **Tyler will want to see, not just benefit from, deprioritization.** The
  existing `QuotaGate.StatusDetail()` pattern (`quota_gate.go:150`) sets a
  precedent: when an automated system silently changes behavior for resource
  reasons, this codebase already surfaces a human-readable reason string to a
  Settings/Feature-Flags UI row, not just a log line. The GitHub-call admission
  control should have an equivalent — e.g. "Backlog GitHub sync paused: PR
  status poller backing off (rate limit resets in 12m)" — surfaced somewhere a
  user actually looks (dashboard header, notification), following the same
  `feedback_document_ai_decisions_in_edge_cases` principle already in the
  user's memory: self-heal/backoff actions should post a visible signal, not
  act silently.
- **A CLI/log query for "what's my current GitHub call rate right now" is
  implied, and there's currently nothing.** `DefaultRateLimiter` only ever logs
  a `Warn` when a threshold is crossed (`rate_limit.go:83-88`) or when a limit
  actually hits (`109`, `134`) — there is no introspection point today (grep
  confirms `IsLimited`/`rateLimitedUntil` are read only by `rate_limit.go`
  itself, `github/testing.go`, and `github/http_client.go`; nothing exposes it
  externally). `docs/how-to/debug-with-logs.md`'s own guidance — prefer metrics
  over per-event log lines for anything frequent (its final "Metrics as an
  alternative to log volume" section) — argues the right answer here is an
  OTel metric (`github.rate_limit.remaining`, `github.rate_limit.reset_at` as
  gauges, tagged by resource) rather than a bespoke CLI subcommand, consistent
  with scope item 1's OTel requirement and avoiding scope item's explicit
  exclusion of a new analytics-DB/UI panel (ADR-027 is out of scope, but a
  Prometheus/OTel gauge queryable via existing `promql-cli`-style tooling is
  not the same thing as that panel).
- **Debug-log conventions already warn against per-tick log spam** — the same
  doc's "Reducing log volume" section (case #1, `session/health.go`'s
  ~15s-ticker reload) is directly analogous to a naive per-tick "still rate
  limited" log in a poller; any new admission-control logging must dedupe on
  state *transition* (limited → not limited), matching the existing
  `RateLimiter.Update`'s current one-shot-per-transition behavior (it already
  does this correctly — `setLimitedUntil` only extends the window, doesn't
  re-log every check) — new code should follow that precedent, not regress it.
- **Single-user, single-token, multi-machine — no auth/tenancy layer needed,
  but budget IS still contended.** Because scope explicitly excludes
  cross-machine coordination (deferred; `session/host_identity.go` gossip layer
  exists but isn't reused here), the design should assume the local admission
  control can only see its own process's call volume, not the true global
  quota consumption across Tyler's other machines. This means the soft/local
  budget signal is inherently an underestimate of real exhaustion risk — worth
  flagging as a known limitation in the plan doc rather than a bug to fix here.

## 4. Industry comparables (verified via web search)

- **Octokit's `@octokit/plugin-throttling`** (used by Probot apps) — a
  request-queuing layer wrapping every outbound call; on a secondary rate limit
  it invokes a required `onSecondaryRateLimit(retryAfter, options, octokit)`
  callback, and returning `true` auto-retries after `retryAfter` seconds
  (defaulting to 60s absent a `Retry-After` header) while `false` throws
  instead — and it proactively self-throttles concurrency to avoid tripping
  GitHub's recommended limits in the first place, rather than reacting only
  after a 403/429. [octokit/plugin-throttling.js README](https://github.com/octokit/plugin-throttling.js/blob/main/README.md)
- **`gh` CLI is purely reactive, and still lacks proactive throttling** — it
  does not pre-check `x-ratelimit-remaining` before dispatching ([cli/cli
  discussion #7754](https://github.com/cli/cli/discussions/7754), [issue
  #3292](https://github.com/cli/cli/issues/3292), both still open), but it does
  expose current quota for manual introspection via `gh api rate_limit`
  (`GET /rate_limit`) — the direct precedent for the "query current rate right
  now" affordance in §3. [GitHub REST rate-limit docs](https://docs.github.com/en/rest/rate-limit/rate-limit)
- **Renovate's `prHourlyLimit`/`prConcurrentLimit`** cap PR/branch-creation
  volume per repo per rolling hour as a simple ceiling, deliberately distinct
  from GitHub's own rate limit — but it is **per-repository only**; a
  long-requested `prHourlyLimitGlobal` for one token serving many repos is
  still an open, unresolved issue. Even a mature bot with a large install base
  hasn't cracked cross-consumer shared-token budgeting — relevant precedent for
  why this project's own cross-machine case is correctly deferred rather than
  attempted here. [renovatebot/renovate discussion #35047](https://github.com/renovatebot/renovate/discussions/35047), [issue #4279](https://github.com/renovatebot/renovate/issues/4279)

## Additional finding: conditional-requests-by-default (scope item 5) is in
tension with GraphQL migration (scope item 3)

`ETagCache` (`github/etag_cache.go`) works by conditional-GET semantics
(`If-None-Match` / `ETag`, `etag_cache.go:12-13`) that are REST-only — GitHub's
GraphQL endpoint is a single `POST /graphql` with no per-query `ETag`/304
support. `GetPRInfoCtx` today is a `gh pr view` CLI shell-out with **no**
conditional-request semantics at all — `ETagCache.fetchWithETag`
(`etag_cache.go:90-124`) does a native HTTP conditional GET first, and only
calls `GetPRInfoCtx` as the "PR changed, now fetch full detail" fallback when
the conditional check returns a 200 (not 304). Migrating `GetPRInfoCtx` to
native GraphQL (scope item 3) does not, by itself, give it or its callers any
conditional-request behavior — the existing REST ETag pre-check remains the
only thing that avoids full fetches, and it wraps `GetPRInfoCtx` rather than
being inside it. Scope item 5 ("make skipping conditional-request semantics
structurally hard for future call sites") therefore can't rely on GraphQL
having ETag support the way REST does; the design needs its own
GraphQL-side freshness signal (e.g. compare `updatedAt` across ticks
server-side, since GitHub GraphQL does expose `updatedAt` fields as seen in
`userPRGraphQLQuery`, `github/user_pr_cache.go`) rather than assuming the
GraphQL migration inherits the REST ETag mechanism for free.
