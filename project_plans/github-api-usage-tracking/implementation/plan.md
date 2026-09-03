# Implementation Plan: github-api-usage-tracking

**Feature**: Build the never-implemented GitHub API instrumentation seam, persist per-call-site/per-resource request history to the existing `analytics.db`, and surface live quota + burn-rate + attribution in a web panel so rate-limit exhaustion becomes diagnosable instead of mysterious.
**Date**: 2026-08-10
**Status**: Ready for implementation
**ADRs**:
- `../decisions/ADR-027-github-usage-events-reuse-analytics-event-table.md` — storage target
- `../decisions/ADR-028-wrap-gh-cli-invocations-do-not-migrate-to-native-http.md` — migrate-vs-wrap

---

## Approaches Considered (Step 0.5 — creative pass)

Three distinct architectures were sketched before committing. All three assume the
**verified** baseline from `../research/architecture.md` §0 and `../research/pitfalls.md` §0:
`github/rate_limit.go:49`'s `Update()` has **zero callers**, the `rateLimitTransport` its doc
comments at `:23`/`:47` describe **does not exist**, and `github/http_client.go:17`'s
`ghHTTPClient` sets no `Transport` — so `IsLimited()` (checked at
`session/pr_status_poller.go:197` and `session/worktree_pr_poller.go:187`) always returns
`false` in production today.

**A — Single transport choke point + `gh` invocation wrapper, into the existing analytics DB.**
Build `usageTransport` (an `http.RoundTripper` decorator) on `ghHTTPClient`, wrap all 7 real `gh`
subprocess sites behind one `runGHCommand` helper, feed both into one buffered recorder writing
`AnalyticsEvent` rows with `event_category="github_api"`.
*Strength*: one counting point per physical request, covers all 14 existing `ghHTTPClient.Do`
sites plus every future one, and revives the dead `Update()` wiring in the same change.
*Weakness*: two fidelity tiers coexist in one dataset (`gh` rows are invocation-count
approximations with no per-request headers) and must be labelled everywhere they are summed.

**B — Migrate every `gh` call site to the native client, then instrument only the transport.**
*Strength*: one uniform, header-exact data path; `DefaultRateLimiter` sees 100% of consumption
with no approximation tier at all.
*Weakness*: `github/client.go:265`'s `gh pr view --json …reviews,reviewDecision,statusCheckRollup`
is one GraphQL call that REST needs ≥3 calls to reproduce, so the migration would *raise* quota
consumption on the hottest poller path — and `CloneRepository` (`client.go:709`) has no HTTP
equivalent at all.

**C — Poll GitHub's own `GET /rate_limit` on a timer and store the deltas.**
*Strength*: authoritative token-global truth that automatically captures `gh` CLI usage, a second
`STAPLER_SQUAD_INSTANCE`, and any other tool sharing the token — for almost no instrumentation
code, and the endpoint does not itself consume core quota.
*Weakness*: gives **zero attribution** — a delta says "quota dropped by 40," never which poller
spent it, which is exactly the question `requirements.md`'s Success Metric requires answering.

**Chosen: A, with C folded in as a reconciliation probe rather than discarded.** A supplies
attribution; C supplies a ground-truth denominator to measure A's own completeness against. The
combination is what makes the Success Metric ("verified using the new tracking data itself…
trustworthy enough to attribute cause") actually satisfiable: the panel reports not just "who
used what" but "N requests this window were unaccounted for," turning the `gh`-wrapper
approximation and the multi-instance blind spot (`../research/features.md` §5) into a *measured
error bar* instead of an unstated assumption. B is rejected in ADR-028.

---

## Domain Glossary

*(Ubiquitous language. These exact names must be used in code, tests, comments, and proto fields.
Where a Go name and a proto/TS name differ, both are given.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `GitHubResource` | The GitHub rate-limit bucket a request draws from: `core`, `search`, `graphql`, `integration_manifest`, or `unknown`. | Newtype over `string` in `github/usage_event.go`; constants `ResourceCore`, `ResourceSearch`, `ResourceGraphQL`, `ResourceUnknown`. Buckets are independent server-side and must never be blended into one number. |
| `CallSite` | The named origin of a GitHub API request — the thing a user would throttle. E.g. `pr_status_poller`, `worktree_pr_poller`, `user_pr_cache`, `auth_check`, `backlog_issue_import`, `mcp_list_github_prs`, `gh_cli.get_pr_info`, `rate_limit_probe`. | Newtype over `string`; the full closed set lives in `AllCallSites` so the UI can render zero-count rows for sources that *could* contribute but haven't. |
| `Fidelity` | How exactly an event maps to physical HTTP requests: `header_exact` (native round-trip, headers observed) or `invocation_approx` (one `gh` subprocess, may be >1 request). | Sum type over `string`; drives the "≈" marker in the UI. |
| `APIUsageEvent` | One recorded unit of GitHub API consumption: `CallSite`, `GitHubResource`, `Fidelity`, `SessionID`, `Method`, `StatusCode`, `Duration`, `QuotaCharged`, plus observed `Remaining`/`Limit`/`ResetAt` when known. | Value struct in `github/usage_event.go`. Serialised to an `AnalyticsEvent` row per ADR-027. |
| `QuotaCharged` | Whether this request actually consumed quota. `false` for `304 Not Modified` conditional responses, for `403`/`429` responses GitHub rejected outright because the token was already over-limit (`X-RateLimit-Remaining: 0`), and for the `/rate_limit` probe itself. | Boolean on `APIUsageEvent`. `github/etag_cache.go:74` makes 304s common; counting either 304s or already-rejected requests would inflate every number and corrupt `UsageReconciliation`'s residual (adversarial-review.md Blocker 1). |
| `RateLimitRejected` | Whether this event's `QuotaCharged: false` was specifically because the response was an already-over-limit rejection, as opposed to a `304`/probe. | Boolean on `APIUsageEvent`. Lets a consumer distinguish "conditional hit" from "rate-limit bounce" without re-deriving it from `StatusCode`. |
| `UsageRecorder` | The sink interface the `github` package writes events to. One method: `RecordAPIUsage(APIUsageEvent)`. | Declared in `github` (the **consumer**) per `.claude/rules/interface-pollution-checklist.md`, so `github` never imports `server/*`. |
| `noopUsageRecorder` | Null-object implementation installed by default, so instrumentation is safe before wiring completes and in every test that doesn't opt in. | GoF Null Object. |
| `usageTransport` | The `http.RoundTripper` decorator installed on `ghHTTPClient.Transport`. Calls the next transport, then `DefaultRateLimiter.Update(resp)`, then `recordUsage(...)`. | The single counting point for the native path. This is the type `rate_limit.go`'s existing comments call `rateLimitTransport`. |
| `runGHCommand` | The single wrapper through which every `gh` subprocess in `github/client.go` is invoked. Records exactly one `invocation_approx` event. | The only place in `client.go` allowed to build a `safeexec.CommandContext(ctx, "gh", …)`. |
| `AttributionContext` | The `context.Context` carrier for `CallSite` and originating session ID. `WithCallSite(ctx, cs)` / `CallSiteFromContext(ctx)`, `WithSessionAttribution(ctx, id)` / `SessionAttributionFromContext(ctx)`. | Lets a `RoundTripper` know its caller without changing 14 call-site signatures. |
| `ResourceQuota` | Last-observed quota state for one `GitHubResource`: `Remaining`, `Limit`, `ResetAt`, `ObservedAt`. | Value struct. `ObservedAt` is what makes the UI's "stale" state possible. |
| `QuotaSnapshot` | An immutable `map[GitHubResource]ResourceQuota` published copy-on-write via `atomic.Pointer` on `RateLimiter`. | Never mutated in place; replaced wholesale. Read by the RPC handler and the probe. |
| `WarnThresholdPercent` | The percent-of-limit below which a low-quota WARN fires. Replaces the hardcoded `rateLimitWarnPercent = 10` const at `github/rate_limit.go:20`. | Config-driven, clamped to `[1, 90]`, read once at startup. |
| `BurnRate` | Quota-charged requests per hour for a resource over the current reset window. | Derived, not stored. |
| `TimeToExhaustion` | `Remaining / BurnRate`, expressed as a duration; `nil` when `BurnRate == 0`. | The metric that answers "do I need to act now," which a static percentage cannot (`../research/features.md` §6). |
| `UsageWindow` | The query time window in days. Clamped `[1, 90]`, default 7 — identical semantics to `GetApprovalAnalyticsRequest.window_days`. | Proto field `window_days`. |
| `SourceStat` | One row of the attribution breakdown: `CallSite`, `Resource`, `Count`, `ApproxCount`, `SharePercent`. | Proto `GitHubSourceStatProto`. |
| `VolumeBucket` | One time bucket of request volume: `Date`/`HourStart`, `Total`, and per-resource counts. | Proto `GitHubVolumeBucketProto`. Daily for windows > 2 days, hourly otherwise. |
| `GitHubUsageRecorder` | The buffered async writer: bounded channel in, background flush out to `SQLiteAnalyticsProvider`. `Record` never blocks; overflow drops and counts. | `server/analytics/github_usage.go`. Pattern lifted from `server/services/analytics_store.go`; storage is `analytics.db` per ADR-027. |
| `GitHubUsageQuery` | The read model — pure aggregation over `AnalyticsEvent` rows where `event_category = "github_api"`. | `server/analytics/github_usage_query.go`. No entities, no mutation. |
| `EventCategoryGitHubAPI` | The constant `"github_api"` written to `AnalyticsEvent.event_category`. | Single source of truth for the category string, exported from `server/analytics`. |
| `RateLimitProbe` | The periodic `GET /rate_limit` call that refreshes the whole `QuotaSnapshot` from GitHub's authoritative response body. | `github/rate_limit_probe.go`. Recorded with `QuotaCharged = false`. |
| `UsageReconciliation` | The comparison of `limit - remaining` (probe truth) against the sum of tracked quota-charged events in the same reset window, yielding `UnaccountedRequests`. | The Success Metric's trust mechanism. Surfaced in the panel, not just logged. |
| `ExhaustionEvent` | A persisted record that `"github API: primary rate limit exhausted"` fired — `remaining == 0` with a known `ResetAt`. | Proto field `exhaustion_events`. This is the quantity `requirements.md`'s Success Metric requires to read 0 over 14 days, counted from stored data rather than by grepping logs. |
| `PauseStats` | Live count and cumulative duration of the pause-on-limit behaviour (`PauseCount`, `TotalPausedDuration`, `CurrentlyPaused`, `PausedUntil`). | `github/rate_limit.go`. Proto fields `pause_events`/`total_paused_seconds`. Exists so a "0 exhaustions" reading achieved by the pause escape hatch silently starving pollers is distinguishable from a genuine fix (pre-mortem.md P1 #3) — the same trust-signal role `UsageReconciliation` plays for tracking completeness, applied to suppression instead of undercounting. |

**26 glossary terms.**

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `usageTransport` | Decorator on `http.RoundTripper` | GoF | Instrumenting each of the 14 `ghHTTPClient.Do` call sites | One choke point covers every present and future native call; per-site counters duplicate `Update()`'s header parsing and invite the double-count in `../research/pitfalls.md` §1b. |
| `UsageRecorder` sink | Consumer-defined interface + Null Object | GoF / `.claude/rules/interface-pollution-checklist.md` | `github` importing `server/analytics` directly | Keeps `github` a leaf package with no `server/*` dependency, and makes instrumentation a safe no-op before `BuildAllDeps` finishes wiring (`server/dependencies.go:960`). |
| `GitHubUsageRecorder` | Queue-Based Load Levelling (bounded channel + background flush + drain-on-Stop) | `server/services/analytics_store.go` precedent | Synchronous `SQLiteAnalyticsProvider.Record` inside `RoundTrip` | `analytics.db` runs `SetMaxOpenConns(1)` (`server/analytics/db.go:38`) shared with the escape batch writer; an inline write puts contended disk I/O in every GitHub call's critical path (`../research/pitfalls.md` §3c). |
| Event storage | Reuse generic `AnalyticsEvent` table via ent Data Mapper | PoEAA / ADR-027 | New `GitHubAPIEvent` ent schema in the main session DB (forking `analytics_store.go`) | No ent codegen, no `session/ent/**` diff, no schema churn on the DB holding live session state. See ADR-027. |
| `GitHubUsageQuery` | Transaction Script over a windowed query + pure aggregation functions | PoEAA | Domain Model / Repository with `GitHubUsageEvent` entities | Read-only telemetry aggregation with no invariants, no lifecycle, no cross-aggregate rules — a Domain Model here would be ceremony over a `GROUP BY`. Mirrors `ComputeSummary`/`ComputeDailyBuckets` in `analytics_store.go`. |
| `GitHubResource` | Newtype over `string` with a closed constant set | type-driven-design / `.claude/rules/primitive-obsession-checklist.md` | Raw `string` label | `resource` and `callSite` are both strings sitting next to each other in every event constructor — exactly the swappable-same-typed-parameter shape the repo's own checklist exists to catch. |
| `CallSite` | Newtype over `string` + exported `AllCallSites` slice | type-driven-design | Raw `string` label | A typo'd attribution key silently creates a phantom source that looks like a real one; and `AllCallSites` is what lets the UI render zero-count rows for quiet-vs-broken diagnosis (`../research/ux.md` §6). |
| `Fidelity` | Sum type (closed `string` set, exhaustively switched) | type-driven-design | A `bool isApproximate` | A boolean cannot grow a third tier (e.g. a future migrated-native site) without every call site relearning what `false` now means; and the exhaustive switch is the compile-time guard against forgetting the "≈" marker. |
| `QuotaSnapshot` | Copy-on-write publication via `atomic.Pointer` | `docs/adr/011-prefer-lock-free-concurrency.md` | Extending `RateLimiter`'s existing `sync.RWMutex` | Keeps the per-response quota write off the `IsLimited()` read path that both pollers hit on every tick (`../research/pitfalls.md` §2b); and the whole map is swapped atomically so `Remaining`/`Limit`/`ResetAt` can never be read torn apart from each other. |
| `AttributionContext` | Ambient context value (`context.WithValue`) | GoF-adjacent / stdlib idiom | Threading a `callSite` parameter through the 14 native call sites | Every native site already builds its request from a context-carrying constructor (`newGHRequestForHostWithToken`, `github/http_client.go:85`), so only the *originating* poller/handler needs one line — no signature churn. |
| `runGHCommand` | Thin wrapper / single-seam Facade over `safeexec` | GoF (Facade) / ADR-028 | Migrating all 7 `gh` sites to `ghHTTPClient` | Migration would triple `GetPRInfoCtx`'s request count (1 GraphQL → 3 REST) and is undefined for `CloneRepository`. See ADR-028. |
| `RateLimitProbe` + `UsageReconciliation` | Ground-truth Reconciliation (measure the residual rather than assume completeness) | — | Asserting the tracker is complete and moving on | `gh`-wrapper approximation, multi-instance `STAPLER_SQUAD_INSTANCE` split (`../research/features.md` §5), and possible `gh`-vs-native token divergence are all real gaps; measuring the residual is what makes the Success Metric verifiable rather than aspirational. |
| Poll-interval config | Load-once configuration, restart to apply | Existing `DaemonPollInterval` precedent (`config/config.go:247`) | fsnotify watcher + `ticker.Reset` on both pollers | No hot-reload exists anywhere in `config/` (verified by all three research agents); two independent tickers plus `time.Ticker.Reset`'s same-goroutine constraint make a partial implementation *worse* than none (`../research/pitfalls.md` §4a). Out of appetite; UI copy says "restart required." |
| Usage panel charts | Hand-rolled `Bar` + `<table>` rows, vanilla-extract | `ApprovalAnalyticsPanel.tsx` house style | `recharts` (already a dependency, used in `web-app/src/app/insights/DailySpendChart.tsx`) | The table-row form puts every value in accessible text for free; a bare SVG chart needs a separate text alternative (`../research/ux.md` §5). recharts stays available if a later reviewer demands a true line chart, but it is not needed to answer any of the three JTBD questions. |
| Quota gauge | Existing `barTrack`/`barFill` primitives + 3-tier token swap | `ApprovalAnalyticsPanel.css.ts` + `.claude/rules/css-architecture.md` | New CSS custom properties / hardcoded hex | `vars.color.success|warning|error|critical` and their `*Bg`/`*Text` pairs are already WCAG-AA-checked in `theme.css.ts`; a new pairing would re-enter the contrast trap those comments document. |
| Warn-threshold write path | Bespoke `UpdateGitHubUsageConfig` RPC, `config.LoadConfig()`/mutate field/`config.SaveConfig()` | `DefaultsService.UpdateGlobalDefaults` (`server/services/defaults_service.go:118-162`) and `UnfinishedWorkService.GetUnfinishedWorkConfig`/`UpdateUnfinishedWorkConfig` (`proto/session/v1/unfinished.proto:40-44`) precedents | Assuming a generic config-update RPC already exists and routing the threshold save through it | Grepping `proto/session/v1/session.proto` for a generic config-write RPC (`rpc Update...Config`) finds none — every settings surface in this codebase (global defaults, feature flags, unfinished-work sources) has its own bespoke RPC + handler + `config.Config` field. Story 3.2.3 follows that same shape rather than inventing a shared one, which is out of this feature's appetite. |

---

## Migration Plan

**No database schema migration is required.** Per ADR-027, GitHub usage events are written as
`AnalyticsEvent` rows (`session/ent/schema/analytics_event.go`) with a new *value* in the existing
`event_category` column — not a new entity, column, or index. Therefore:

- **Migration file**: none. No `session/ent/schema/*.go` file is added or edited, so
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  is **not** run and no `session/ent/**` generated diff appears in this PR. If a future reviewer
  believes a purpose-built table is warranted, that is a separate, additive change.
- **Reversibility**: fully reversible. Reverting the PR stops the writes; existing
  `event_category="github_api"` rows become inert and age out through the normal retention path.
  No other subsystem reads them.
- **Config schema change (additive, backward compatible)**: five new optional JSON fields on
  `config.Config` — `github_usage_retention_days`, `github_rate_limit_warn_percent`,
  `pr_status_poll_interval_seconds`, `worktree_pr_poll_interval_seconds`,
  `github_rate_limit_probe_interval_seconds`. All are `omitempty` with `…OrDefault()` accessors
  returning today's hardcoded values (30 days, 10%, 60s, 60s, 300s), so an untouched
  `~/.stapler-squad/config.json` produces byte-identical behaviour to today.
- **Retention interaction (must not be skipped)**:
  `server/analytics/retention.go:49`'s count-based phase deletes the globally-oldest
  `AnalyticsEvent` rows across **all** categories against
  `cfg.AnalyticsMaxRowsOrDefault()` = 100 000 (`config/config.go:584`). At the requirements'
  own volume estimate (hundreds–low thousands/day), 90 days of `github_api` rows would approach or
  exceed that budget on its own and crowd out UI telemetry. Story 2.2.1 therefore adds a
  `github_api`-scoped age prune at `GitHubUsageRetentionDaysOrDefault()` (30) that runs *before*
  the global count phase. The 14-day Success-Metric window is unaffected either way — eviction is
  oldest-first — but the interaction is documented rather than discovered later.
- **Rollback procedure**: `gh pr revert` / revert commit. Optionally purge rows with
  `DELETE FROM analytics_events WHERE event_category='github_api';` against
  `~/.stapler-squad/analytics.db` — never required, since retention reclaims them.

## Observability Plan

Tied directly to `requirements.md`'s Success Metric: *zero occurrences of
`"github API: primary rate limit exhausted"` over a 14-day window, verified using the new tracking
data itself.*

- **Logs** (structured, via `github.com/tstapler/stapler-squad/log`):
  - `github/rate_limit.go` keeps its existing three WARN lines verbatim
    (`"github API: rate limit running low"`, `"github API: secondary rate limit hit"`,
    `"github API: primary rate limit exhausted"`) — extended, not duplicated, per
    `requirements.md`'s Observability Requirements. Each gains a `call_site` field read from the
    request context so a log line alone names the culprit.
  - `INFO "github usage recorder started"` / `"stopped"` with `buffer_size`, `flushed`, `dropped`.
  - `WARN "github usage recorder buffer full — dropping event"` with a cumulative `dropped`
    counter (mirrors `AnalyticsStore.Record`'s drop-with-count convention).
  - `WARN "github usage: gh CLI token identity differs from native client"` (Story 5.1.2).
  - `WARN "github usage: reconciliation residual exceeds tolerance"` with `probe_used`,
    `tracked`, `unaccounted`, `resource` (Story 5.1.1).
- **Metrics** (all derived from stored rows — no new metrics backend; OTel export stays out of
  scope):
  - `github_api_requests_total{call_site,resource,fidelity,status_class}` — every quota-charged
    event.
  - `github_api_quota_remaining{resource}` and `github_api_quota_limit{resource}` — from
    `QuotaSnapshot`, exposed through `GetGitHubAPIUsage`, with `observed_at` so staleness is
    visible.
  - `github_api_burn_rate_per_hour{resource}` and `github_api_time_to_exhaustion_seconds{resource}`
    — the metrics that actually drive a decision.
  - `github_api_exhaustion_events_total` — count of persisted `primary rate limit exhausted`
    events. **This is the Success Metric's counter**: it must read 0 over the 14-day trial, and it
    is queryable from the panel rather than by grepping logs.
  - `github_api_unaccounted_requests{resource}` — the reconciliation residual. A Success-Metric
    reading of 0 exhaustions is only trustworthy while this stays small; the panel shows both
    together for exactly that reason. Expected to be routinely nonzero under normal multi-instance
    dev workflow (pre-mortem.md P1 #2) — see Story 5.2.3's doc for the "not itself a trust signal"
    caveat.
  - Recorder health: `github_usage_events_dropped_total` (>0 means the dataset has holes and the
    Success Metric cannot be claimed).
  - `github_api_pause_events_total` and `github_api_paused_seconds_total` (Task 1.2.4c,
    pre-mortem.md P1 #3) — a Success-Metric reading of 0 exhaustions achieved via a high pause
    total means the pause escape hatch masked the problem rather than solved it; the panel and
    Story 5.2.3's verification procedure both check this alongside the exhaustion count.
- **Alerts**: no paging (single-user local service). The panel's critical tier plus the existing
  WARN log lines are the alert surface. `requirements.md` places Slack/email/push explicitly out
  of scope.

## Risk Control

- **Feature flag**: **not gated.** Backend tracking is additive and defaults reproduce today's
  behaviour exactly; poll intervals and warn threshold default to today's hardcoded constants.
  The one genuine behaviour change — `DefaultRateLimiter.Update()` finally firing, so
  `IsLimited()` can return `true` and pollers can actually skip a tick — is the *documented
  intent* of code that already exists, not a new policy. It is nonetheless called out in the PR
  body as the single non-additive consequence, and Story 1.2.4 adds the escape hatch
  `STAPLER_SQUAD_GITHUB_PAUSE_ON_LIMIT=false` for the case where reviving the pause turns out to
  stall a poller unexpectedly.
- **Rollback procedure**: standard revert via PR close + revert commit. No data migration to undo
  (see Migration Plan). If only the pause behaviour misbehaves, set
  `STAPLER_SQUAD_GITHUB_PAUSE_ON_LIMIT=false` and restart — no code change.
- **Staged rollout**: full rollout on merge (single local instance).
- **Manual verification constraint (hard rule)**: never run `make install-service` to try this
  out — it restarts the live `:8543` service and kills every live tmux session
  (`.claude/rules/tmux-keep-server-on-restart.md`). Use the second-instance pattern from
  `CLAUDE.md`: `go build -o /tmp/ssq-manual-test . && PORT=8999
  STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`.

## Unresolved Questions

- [ ] **Does `gh` CLI resolve the same GitHub token as the native client on this machine?**
  `gh` reads `gh auth status`/`~/.config/gh/hosts.yml`; the native path uses
  `GITHUB_TOKEN` → `GH_TOKEN` → keychain (`github/http_client.go:36`). If they differ, the `gh`
  wrapper's events describe a *different* quota than the gauge shows. — blocks the fidelity claim
  in Story 1.3.2 / ADR-028; resolved by Story 5.1.2's startup identity check, which must run
  before the 14-day trial begins. Owner: implementation subagent for Story 5.1.2.
- [ ] **Is `GET /rate_limit` genuinely free of core-quota cost?** GitHub documents it as not
  counting against the primary REST rate limit, but this is **UNVERIFIED against the live API**
  in this repo. If it does consume quota, the probe interval default (300s = 288 calls/day out of
  5000) must be raised. — blocks Story 5.1.1's `QuotaCharged=false` labelling; resolved by a
  one-off empirical check (call `/rate_limit` twice, confirm `core.remaining` is unchanged).
  Owner: implementation subagent for Story 5.1.1, Task 5.1.1a.
- [ ] **Should usage be tracked per-instance or per-token?** `STAPLER_SQUAD_INSTANCE` gives each
  instance its own `analytics.db`, so two concurrent instances sharing one token each see a
  partial view while the real quota is jointly consumed (`../research/features.md` §5). This plan
  chooses **per-instance storage with a measured residual** (the reconciliation probe makes the
  other instance's consumption visible as `unaccounted`, rather than invisible) — a shared
  cross-instance DB is deliberately not built. — no story is blocked; recorded so a reviewer can
  challenge the choice rather than discover it. Owner: reviewer at Phase 4 validation.
- [ ] **Does `X-RateLimit-Resource` appear on GraphQL responses from
  `github/user_pr_cache.go:608`?** If GitHub omits it there, GraphQL requests land in
  `ResourceUnknown` and the panel's graphql tile stays empty even though the probe's body reports
  a graphql bucket. — blocks the completeness of Story 1.2.2's resource attribution; resolved by
  inspecting one live response header set. Owner: implementation subagent for Story 1.2.2.
- [ ] **Dynamic detection of *which*/*how many* other `STAPLER_SQUAD_INSTANCE`s are running,
  deferred as future work (pre-mortem.md P1 #2).** This plan's reconciliation residual (Story
  5.1.1) names "another instance" as a possible cause in static copy only — no code counts or
  tags concurrent-instance activity, because no cross-instance registry exists in this codebase
  today (confirmed: `grep -rln STAPLER_SQUAD_INSTANCE --include="*.go"` finds only
  single-instance consumers reading their own name, not a shared list of siblings). A future
  enhancement could have each instance write a lightweight heartbeat file under a shared
  `~/.stapler-squad/instances/` directory (that directory already exists per
  `.claude/docs/state-isolation.md`) so the panel could report "N other instances were active
  during this window" instead of only the static clause — not built here; scoped out to keep this
  story's appetite tight. Owner: a future proposal, not this plan.

## Dependency Visualization

```
Phase 1 — Instrumentation Foundation (github pkg; hard prerequisite for everything)
  1.1 domain types + attribution ctx ──┐
                                       ├──> 1.2 usageTransport + QuotaSnapshot + threshold
                                       └──> 1.3 gh wrapper + IsForkRepo deletion
                                                   │
Phase 2 — Persistence                              │
  2.1 GitHubUsageRecorder  <─────────────(needs 1.1's UsageRecorder iface)
  2.2 retention + startup wiring  <──────(needs 2.1, and 1.2/1.3 to have something to record)
                                                   │
Phase 3 — Read model, RPC, config                  │
  3.1 GitHubUsageQuery  <────────────────(needs 2.1's row shape)
  3.2 proto + GetGitHubAPIUsage handler  <(needs 3.1 + 1.2's QuotaSnapshot)
    └─ 3.2.3 UpdateGitHubUsageConfig write RPC <(needs 3.3.1a's config field — see below)
  3.3 config-driven intervals + threshold <(needs 1.2's SetWarnThresholdPercent; independent of 3.1/3.2)
    └─ 3.3.1a's `GitHubRateLimitWarnPercent` field is itself a prerequisite for 3.2.3, so 3.3.1a
       must land before 3.2.3 even though 3.3 is numbered after 3.2 — a numbering/dependency
       inversion, called out explicitly rather than silently mis-ordered.
                                                   │
Phase 4 — Web UI                                   │
  4.1 hook + route + nav  <──────────────(needs 3.2's generated client)
  4.2 panel sections  <──────────────────(needs 4.1)
  4.3 states, a11y, threshold editor  <──(needs 4.2; threshold editor also needs 3.2.3)
                                                   │
Phase 5 — Trust + ship gate                        │
  5.1 probe + reconciliation + token check <(needs 1.2, 2.1, 3.2)
  5.2 e2e, registry, docs, verify  <───────(needs 4.3 + 5.1)

Parallelisable once Phase 1 lands: {2.1}, {3.3}, and the CSS/skeleton half of {4.2}.
Serial chokepoints: 1.1 → everything; 3.3.1a → 3.2.3 → 4.3.2; 3.2 → 4.x; 5.1 → the Success-Metric
claim.
```

---

## Phase 1: Instrumentation Foundation

*This phase builds the seam that `github/rate_limit.go`'s doc comments have described since it was
written but that has never existed. Nothing else in the feature can be built or tested until it
does.*

### Epic 1.1: Usage event domain types and caller attribution

**Goal**: Give the `github` package a typed vocabulary for "a unit of GitHub API consumption" and
a way to know which of its callers triggered a request — without importing anything from
`server/*` and without changing any existing function signature.

#### Story 1.1.1: Typed usage-event vocabulary
**As a** developer reading GitHub usage data, **I want** resources, call sites and fidelity to be
distinct types with closed value sets, **so that** a typo or a swapped argument cannot silently
invent a phantom source that looks real in the dashboard.
**Acceptance Criteria**:
- `GitHubResource`, `CallSite`, and `Fidelity` are defined types over `string`, not raw `string`,
  and every legal value exists as an exported constant.
  - *Given* a developer writing `APIUsageEvent{Resource: "pr_status_poller", CallSite: "core"}`
    with the arguments transposed, *When* they run `go build ./github/`, *Then* compilation fails
    with a type mismatch on both fields, rather than producing an event that the panel would
    render as a resource named `pr_status_poller`.
- `AllCallSites` enumerates every known `CallSite` so consumers can render sources with zero
  observed requests.
  - *Given* `AllCallSites` contains `CallSiteWorktreePRPoller` and the store holds no events for
    it in the last 7 days, *When* `GitHubUsageQuery.SourceBreakdown(7)` is called, *Then* the
    result includes a `SourceStat{CallSite: "worktree_pr_poller", Count: 0}` row so a silently
    dead poller is distinguishable from a correctly quiet one.
- `Fidelity` is exhaustively switchable: `FidelityHeaderExact` and `FidelityInvocationApprox` are
  the only members, and `String()` panics-free-defaults on an unknown value.
  - *Given* `Fidelity("bogus")`, *When* `ev.Fidelity.IsApproximate()` is called, *Then* it returns
    `true` (fail safe toward "mark it as an estimate") rather than `false`.
**Files**: `github/usage_event.go`, `github/usage_event_test.go`

##### Task 1.1.1a: Define the three newtypes and their constant sets (~4 min)
- Create `github/usage_event.go` with `package github`.
- Declare `type GitHubResource string` with `ResourceCore = "core"`, `ResourceSearch = "search"`,
  `ResourceGraphQL = "graphql"`, `ResourceIntegrationManifest = "integration_manifest"`,
  `ResourceUnknown = "unknown"`; add `ParseGitHubResource(header string) GitHubResource` mapping
  an empty/unrecognised `X-RateLimit-Resource` to `ResourceUnknown`.
- Declare `type CallSite string` with constants for every in-scope origin:
  `CallSitePRStatusPoller`, `CallSiteWorktreePRPoller`, `CallSiteUserPRCache`, `CallSiteAuthCheck`,
  `CallSiteETagConditional`, `CallSiteBacklogIssueImport`, `CallSiteMCPBacklogTools`,
  `CallSiteBacklogForwardSync`, `CallSiteBacklogSearchRepos`, `CallSiteBacklogListIssues`,
  `CallSiteRateLimitProbe`, `CallSiteUnknown`, plus the seven
  `gh_cli.*` sites (`CallSiteGHGetPRInfo`, `CallSiteGHGetPRComments`, `CallSiteGHGetPRDiff`,
  `CallSiteGHPostPRComment`, `CallSiteGHMergePR`, `CallSiteGHClosePR`, `CallSiteGHCloneRepository`),
  plus `CallSiteGHTokenIdentity` for the Story 5.1.2 parity probe (a `gh api user` call that is
  diagnostic, not one of the 7 product call sites — it must be enumerable so its own quota cost
  shows up in the breakdown rather than hiding inside `unknown`).
  `CallSiteBacklogSearchRepos`/`CallSiteBacklogListIssues` attribute
  `server/services/backlog_service_query.go`'s `SearchGitHubRepos` (→ `gh.SearchUserRepos`) and
  `ListGitHubIssues` (→ `gh.ListRepoIssues`) — named in `requirements.md`'s Scope but originally
  missing from this enumeration, which would have left both RPCs' requests attributed to
  `CallSiteUnknown` in the panel despite Story 1.3.3 existing specifically to prevent that.
- Declare `var AllCallSites = []CallSite{…}` containing every constant above.
- Files: `github/usage_event.go`

##### Task 1.1.1b: Define `Fidelity` and `APIUsageEvent` (~3 min)
- Add `type Fidelity string` with `FidelityHeaderExact`/`FidelityInvocationApprox` and
  `func (f Fidelity) IsApproximate() bool { return f != FidelityHeaderExact }`.
- Add `type APIUsageEvent struct` with fields `CallSite`, `Resource`, `Fidelity`, `SessionID`,
  `Host`, `Method`, `StatusCode`, `Duration time.Duration`, `QuotaCharged bool`,
  `RateLimitRejected bool`, `Remaining int`, `Limit int`, `ResetAt time.Time`,
  `ObservedAt time.Time`. Document that `Remaining`/`Limit` are `-1`/`0` when unobserved (matching
  `rate_limit.go:53,59`'s existing sentinels), and that `RateLimitRejected` is `true` exactly when
  `QuotaCharged` was set `false` *because* the response was an already-over-limit rejection (as
  opposed to the `304 Not Modified` case, which is also `QuotaCharged: false` but
  `RateLimitRejected: false`) — so a consumer can distinguish "conditional hit" from "rate-limit
  bounce" without re-deriving it from `StatusCode` (adversarial-review.md Blocker 1).
- Files: `github/usage_event.go`

##### Task 1.1.1c: Table-driven tests for the newtypes (~4 min)
- Add `TestParseGitHubResource_should_MapToUnknown_When_HeaderAbsentOrUnrecognised`,
  `TestFidelity_should_ReportApproximate_When_ValueIsNotHeaderExact`, and
  `TestAllCallSites_should_ContainEveryDeclaredCallSiteConstant` (uses a literal list so adding a
  constant without registering it fails the test).
- Files: `github/usage_event_test.go`

---

#### Story 1.1.2: Context-carried caller attribution
**As a** poller or RPC handler, **I want** to stamp my identity onto the context I already pass
down, **so that** a transport-level recorder can attribute a request to me without any of the 14
native call sites changing their signatures.
**Acceptance Criteria**:
- `WithCallSite(ctx, cs)` / `CallSiteFromContext(ctx)` round-trip a `CallSite`, and an unstamped
  context yields `CallSiteUnknown` rather than an empty string.
  - *Given* `ctx := context.Background()` with no stamp, *When* `CallSiteFromContext(ctx)` is
    called, *Then* it returns `CallSiteUnknown`, so unattributed traffic appears as an explicit
    "unknown" row in the panel rather than as a blank label.
- `WithSessionAttribution(ctx, sessionID)` / `SessionAttributionFromContext(ctx)` round-trip an
  originating session ID, and timer-driven pollers leave it empty.
  - *Given* the MCP handler stamps `WithSessionAttribution(ctx, "sess-7f3a")` before calling
    `github.GetPRComments`, *When* the resulting event is recorded, *Then*
    `APIUsageEvent.SessionID == "sess-7f3a"`; *and Given* `PRStatusPoller.checkAllSessions` stamps
    only `WithCallSite`, *Then* `SessionID == ""` and the row's `session_id` column is left NULL.
- Context keys are unexported struct types, not strings.
  - *Given* an unrelated package that does `context.WithValue(ctx, "callSite", "spoof")`,
    *When* `CallSiteFromContext(ctx)` runs, *Then* it returns `CallSiteUnknown` — the collision is
    impossible by construction.
**Files**: `github/usage_context.go`, `github/usage_context_test.go`

##### Task 1.1.2a: Implement the context carriers (~3 min)
- Create `github/usage_context.go` with unexported `type callSiteKey struct{}` and
  `type sessionAttrKey struct{}`, plus the four exported functions above.
- Files: `github/usage_context.go`

##### Task 1.1.2b: Tests for round-trip, default, and key isolation (~3 min)
- Add `TestCallSiteFromContext_should_ReturnUnknown_When_ContextUnstamped`,
  `TestWithCallSite_should_RoundTripValue_When_Stamped`, and
  `TestCallSiteFromContext_should_IgnoreStringKeyedValue_When_UnrelatedPackageSpoofsIt`.
- Files: `github/usage_context_test.go`

---

#### Story 1.1.3: The `UsageRecorder` sink and its null object
**As the** `github` package, **I want** to emit usage events through a narrow interface I declare
myself, **so that** I stay a leaf package with no `server/*` dependency and instrumentation is a
harmless no-op until the server wires a real sink.
**Acceptance Criteria**:
- `UsageRecorder` is a one-method interface declared in `github`, and the package-level default is
  a null object — never `nil`.
  - *Given* a unit test that calls `github.GetPRDiff(...)` without ever calling
    `github.SetUsageRecorder`, *When* the call completes, *Then* no panic occurs and no event is
    stored, because `noopUsageRecorder` is installed at package init.
- `SetUsageRecorder` is safe to call concurrently with in-flight requests.
  - *Given* `server/dependencies.go` calls `github.SetUsageRecorder(rec)` while the auth-check
    singleflight is mid-request, *When* the race detector runs
    `go test -race ./github/ -run TestSetUsageRecorder`, *Then* no data race is reported — the
    recorder is published through an `atomic.Pointer`, not a plain field.
- `github` imports nothing from `server/`.
  - *Given* the finished implementation, *When* `go list -deps ./github/ | grep stapler-squad/server`
    is run, *Then* it produces no output.
**Files**: `github/usage_event.go`, `github/usage_event_test.go`

##### Task 1.1.3a: Declare the interface, null object, and atomic setter (~4 min)
- In `github/usage_event.go`: `type UsageRecorder interface { RecordAPIUsage(ev APIUsageEvent) }`,
  `type noopUsageRecorder struct{}` with an empty method, and
  `var usageRecorder atomic.Pointer[UsageRecorder]` seeded in `init()` with the noop.
- Add `func SetUsageRecorder(r UsageRecorder)` (nil argument reinstalls the noop) and unexported
  `func recordUsage(ev APIUsageEvent)` that loads and calls.
- Files: `github/usage_event.go`

##### Task 1.1.3b: Recorder-swap and leaf-package tests (~4 min)
- Add `TestSetUsageRecorder_should_NotRace_When_SwappedDuringConcurrentRecords` (goroutine
  hammering `recordUsage` while another swaps recorders; run under `-race`).
- Add `TestRecordUsage_should_NoOp_When_NoRecorderInstalled`.
- Add `TestGithubPackage_should_NotDependOnServerPackages` executing
  `go list -deps ./...`-equivalent via `packages.Load`, or a simpler `go:generate`-free assertion
  scanning `github/*.go` imports for the `server/` prefix.
- Files: `github/usage_event_test.go`

---

### Epic 1.2: The transport hook and the live quota snapshot

**Goal**: Build the `RoundTripper` that `rate_limit.go` has always claimed exists, make
`DefaultRateLimiter.Update()` fire for the first time, and publish a per-resource quota snapshot
the UI can read.

#### Story 1.2.1: `usageTransport` installed on the shared client
**As a** developer, **I want** every native GitHub request to pass through one instrumented
transport, **so that** all 14 existing `ghHTTPClient.Do` call sites and every future one are
counted exactly once with no per-site code.
**Acceptance Criteria**:
- `ghHTTPClient.Transport` is a `*usageTransport` wrapping `http.DefaultTransport`, and every
  response passes through `DefaultRateLimiter.Update` before being returned to the caller.
  - *Given* an `httptest.Server` set as `GhBaseURL` returning `200` with
    `X-RateLimit-Resource: core`, `X-RateLimit-Remaining: 4999`, `X-RateLimit-Limit: 5000`,
    *When* `github.GetCurrentUserLogin(ctx)` is called, *Then* exactly one `APIUsageEvent` is
    recorded with `CallSite: CallSiteAuthCheck`, `Resource: ResourceCore`,
    `Fidelity: FidelityHeaderExact`, `Remaining: 4999`, `QuotaCharged: true`.
- Conditional `304 Not Modified` responses are recorded but not charged.
  - *Given* the same test server returning `304` for a request carrying `If-None-Match`
    (the path `github/etag_cache.go:65` takes), *When* the round-trip completes, *Then* the
    recorded event has `StatusCode: 304` and `QuotaCharged: false`, so the panel's "requests that
    cost quota" total excludes it.
- A request rejected because the token is already rate-limited is not counted as quota-charged.
  - *Given* an `httptest.Server` returning `403` with `X-RateLimit-Remaining: 0`,
    `X-RateLimit-Limit: 5000` — the header shape GitHub sends for a request made while the token
    is already over its primary rate limit — *When* the round-trip completes, *Then* the recorded
    event has `StatusCode: 403`, `QuotaCharged: false`, and `RateLimitRejected: true`. GitHub does
    not decrement further quota for a request it rejects outright, so counting it as charged would
    inflate `total_requests`/`ComputeBurnRate` and corrupt `UsageReconciliation`'s residual during
    the exact exhaustion windows the feature exists to diagnose (adversarial-review.md Blocker 1).
    The same rule applies to `429` responses carrying `X-RateLimit-Remaining: 0`.
  - *Given* a `403` response **without** a zero `X-RateLimit-Remaining` (e.g. a permissions-denied
    `403` unrelated to rate limiting — GitHub still processes and bills these), *When* the
    round-trip completes, *Then* `QuotaCharged: true` and `RateLimitRejected: false` — the fix must
    not blanket-exclude every `403`, only the ones GitHub's own headers identify as a rate-limit
    bounce.
- Transport errors do not record a fabricated event and do not swallow the error.
  - *Given* a server that closes the connection without responding, *When* `ghHTTPClient.Do` is
    called, *Then* the caller receives the original transport error unchanged and **no**
    `APIUsageEvent` is recorded (there is no response to attribute a resource or status to).
- The hook performs no blocking I/O.
  - *Given* a `UsageRecorder` whose `RecordAPIUsage` is a channel send on a full buffer,
    *When* 50 concurrent round-trips complete, *Then* none of them blocks longer than the
    underlying HTTP call — verified by asserting the recorder's drop counter increments rather
    than the test timing out.
- A panic anywhere in the instrumentation code never fails the underlying GitHub request or
  crashes the process.
  - *Given* a `UsageRecorder` test double whose `RecordAPIUsage` panics on every call (simulating a
    bug in header parsing, `ParseGitHubResource`, or event construction — the single choke point
    every native GitHub HTTP call passes through), *When* `github.GetCurrentUserLogin(ctx)`
    round-trips against a `200` response, *Then* the caller receives the response normally
    (`err == nil`, `resp.StatusCode == 200`, body intact) and one
    `WARN "github usage: instrumentation panic recovered"` is logged with the recovered value —
    the underlying API call's success is never affected by a bug in tracking code
    (adversarial-review.md Blocker 2). This directly backs the Risk Control section's "purely
    additive" claim, which otherwise has no test covering it.
**Files**: `github/usage_transport.go`, `github/http_client.go`, `github/usage_transport_test.go`

##### Task 1.2.1a: Implement `usageTransport.RoundTrip` (~5 min)
- Create `github/usage_transport.go`: `type usageTransport struct { next http.RoundTripper }`,
  with `RoundTrip` that records `start := time.Now()`, delegates to `t.next` (defaulting to
  `http.DefaultTransport`) **outside** any `recover()` scope, and returns `(nil, err)` untouched on
  error with no event recorded.
- On a successful round-trip, wrap only the instrumentation portion — `DefaultRateLimiter.Update`,
  header parsing, `ParseGitHubResource`, `APIUsageEvent` construction, and `recordUsage` — in
  `defer func() { if r := recover(); r != nil { log.Warn("github usage: instrumentation panic
  recovered", "panic", r) } }()`. The already-obtained `(resp, err)` from `t.next.RoundTrip` is
  returned to the caller regardless of whether this deferred block panics — never gate the return
  of `resp`/`err` behind the instrumentation succeeding (adversarial-review.md Blocker 2).
- Derive `Resource` from `ParseGitHubResource(resp.Header.Get("X-RateLimit-Resource"))`,
  `CallSite`/`SessionID` from `req.Context()`, `Host` from `req.URL.Host`, and `QuotaCharged` from
  a helper `isQuotaCharged(resp) bool`: `false` for `304 Not Modified` (unchanged), **and** `false`
  — with `RateLimitRejected: true` — for a `403`/`429` response whose `X-RateLimit-Remaining`
  header parses to `0` (GitHub does not decrement quota for a request it rejects for being already
  over-limit; adversarial-review.md Blocker 1); `true` otherwise, including other 403s.
- Files: `github/usage_transport.go`

##### Task 1.2.1b: Install it on `ghHTTPClient` (~2 min)
- Edit `github/http_client.go:17` so the shared client is
  `&http.Client{Timeout: 30 * time.Second, Transport: &usageTransport{}}`, with a comment
  replacing the stale `rateLimitTransport` naming in `rate_limit.go:23,47` (update those two doc
  comments to name `usageTransport` so the code and its documentation finally agree).
- Files: `github/http_client.go`, `github/rate_limit.go`

##### Task 1.2.1c: Transport tests against `httptest.Server` (~5 min)
- Add `TestUsageTransport_should_RecordHeaderExactEvent_When_ResponseCarriesRateLimitHeaders`,
  `TestUsageTransport_should_MarkQuotaNotCharged_When_ResponseIs304`,
  `TestUsageTransport_should_RecordNothingAndPropagateError_When_TransportFails`,
  `TestUsageTransport_should_NotBlockCaller_When_RecorderBufferIsFull`,
  `TestUsageTransport_should_MarkQuotaNotChargedAndRejected_When_403WithZeroRemaining` (and the
  `429` equivalent), `TestUsageTransport_should_MarkQuotaCharged_When_403IsNotARateLimitRejection`
  (adversarial-review.md Blocker 1), and
  `TestUsageTransport_should_ReturnResponseNormally_When_InstrumentationPanics` (a
  `recordingUsageRecorder` whose `RecordAPIUsage` panics; asserts the caller still gets the real
  response and no process crash — adversarial-review.md Blocker 2).
- Use a `recordingUsageRecorder` test double installed via `SetUsageRecorder` and restored with
  `t.Cleanup`; override `GhBaseURL` per the existing pattern documented at
  `github/http_client.go:19-21`.
- Files: `github/usage_transport_test.go`

---

#### Story 1.2.2: Per-resource `QuotaSnapshot` published copy-on-write
**As the** usage panel, **I want** the last-observed remaining/limit/reset for each GitHub
resource plus when it was observed, **so that** I can render an honest gauge and distinguish
"never observed" from "0% remaining" and from "this reading is 40 minutes old."
**Acceptance Criteria**:
- `RateLimiter.Snapshot()` returns a per-resource map, and each entry records `ObservedAt`.
  - *Given* a response with `X-RateLimit-Resource: search`, `X-RateLimit-Remaining: 3`,
    `X-RateLimit-Limit: 30`, `X-RateLimit-Reset: <now+900s>`, *When* `Update(resp)` runs,
    *Then* `Snapshot()[ResourceSearch]` equals
    `ResourceQuota{Remaining: 3, Limit: 30, ResetAt: <now+900s>, ObservedAt: <now>}` and
    `Snapshot()[ResourceCore]` is absent — resources never blend.
- Resources are never blended, and an unobserved resource is absent rather than zero-valued.
  - *Given* the process has only ever made core requests, *When* the panel reads the snapshot,
    *Then* the search entry is missing (rendered as "—", not as "0 / 0" or "100% remaining").
- Snapshot publication is copy-on-write and does not touch the `IsLimited()` mutex.
  - *Given* `go test -race ./github/ -run TestQuotaSnapshot`, with one goroutine calling
    `Update()` in a loop and another calling `IsLimited()` and `Snapshot()` in a loop,
    *When* the test completes, *Then* no data race is reported and the existing
    `rateLimitedUntil` `sync.RWMutex` is untouched by the snapshot path.
- Existing WARN/pause behaviour is byte-for-byte preserved.
  - *Given* a `429` response with `Retry-After: 30`, *When* `Update(resp)` runs, *Then*
    `IsLimited()` returns `true` with a resume time ~30s out, and the log line
    `"github API: secondary rate limit hit"` is emitted with the same fields as before this change
    plus a new `call_site` field.
**Files**: `github/rate_limit.go`, `github/rate_limit_test.go`

##### Task 1.2.2a: Add `ResourceQuota` / `QuotaSnapshot` and the atomic pointer (~4 min)
- In `github/rate_limit.go`, add `type ResourceQuota struct { Remaining, Limit int; ResetAt,
  ObservedAt time.Time }` and `type QuotaSnapshot map[GitHubResource]ResourceQuota`.
- Add `quota atomic.Pointer[QuotaSnapshot]` to `RateLimiter` and
  `func (r *RateLimiter) Snapshot() QuotaSnapshot` returning an empty map when unset.
- Files: `github/rate_limit.go`

##### Task 1.2.2b: Publish the snapshot from `Update` (~4 min)
- Inside `Update`, after the existing header parsing (`rate_limit.go:50-70`), build a new map by
  copying the current snapshot and replacing the entry for this resource, then `quota.Store`.
  Per `.claude/rules/go-double-checked-locking.md`, any value returned to the caller must be the
  locally-built map, never a re-read of the slot.
- Skip publication entirely when `remaining < 0` (headers absent) so a header-less response cannot
  erase a good reading.
- Add `call_site` (from a new `Update(resp)` context read — take it from `resp.Request.Context()`)
  to all three existing WARN log lines without altering their messages or existing fields.
- Files: `github/rate_limit.go`

##### Task 1.2.2c: Snapshot tests including the race and preservation cases (~5 min)
- Create `github/rate_limit_test.go` (none exists today) with
  `TestUpdate_should_PublishPerResourceQuota_When_HeadersPresent`,
  `TestSnapshot_should_OmitResource_When_NeverObserved`,
  `TestUpdate_should_NotEraseSnapshot_When_HeadersAbsent`,
  `TestQuotaSnapshot_should_NotRace_When_UpdatedConcurrentlyWithIsLimited` (`-race`), and
  `TestUpdate_should_PreserveSecondaryRateLimitPause_When_429WithRetryAfter`.
- Files: `github/rate_limit_test.go`

##### Task 1.2.2d: Resolve the GraphQL resource-header question (~3 min)
- Add `TestUpdate_should_AttributeGraphQLResource_When_ResponseDeclaresIt` and, in the same task,
  record the empirical finding for Unresolved Question #4 as a code comment on
  `ParseGitHubResource`: whether real GraphQL responses from
  `github/user_pr_cache.go:608` carry `X-RateLimit-Resource: graphql`. If they do not, map the
  GraphQL request path explicitly by stamping `ResourceGraphQL` from the call site instead of the
  header.
- Files: `github/rate_limit_test.go`, `github/usage_event.go`

---

#### Story 1.2.3: User-configurable warn threshold
**As** Tyler, **I want** to change the "quota running low" threshold without a rebuild, **so that**
I can make the warning fire earlier on the search resource where 10% is only 3 requests.
**Acceptance Criteria**:
- `rateLimitWarnPercent` becomes a settable, clamped field rather than a `const`.
  - *Given* `SetWarnThresholdPercent(25)` at startup and a core response with
    `X-RateLimit-Remaining: 1200`, `X-RateLimit-Limit: 5000`, *When* `Update(resp)` runs,
    *Then* the `"github API: rate limit running low"` WARN fires (1200 < 1250), whereas at the
    old hardcoded 10% (threshold 500) it would not have.
- Out-of-range values are clamped, not rejected, and clamping is logged once.
  - *Given* `SetWarnThresholdPercent(500)`, *When* the next `Update` runs, *Then* the effective
    threshold is 90% and a single `WARN "github rate-limit warn threshold clamped"` was emitted at
    set time.
- The existing floor is preserved.
  - *Given* threshold 10% and a search response with `Limit: 30` (so `30*10/100 = 3`),
    *When* `Update` runs with `Remaining: 4`, *Then* the WARN fires, because the existing
    `if threshold < 5 { threshold = 5 }` floor at `rate_limit.go:78-80` is unchanged.
**Files**: `github/rate_limit.go`, `github/rate_limit_test.go`

##### Task 1.2.3a: Replace the const with a clamped atomic (~3 min)
- Change `rateLimitWarnPercent` (`rate_limit.go:20`) from a `const` to
  `var rateLimitWarnPercent atomic.Int64` seeded to 10, and add
  `func SetWarnThresholdPercent(pct int)` clamping to `[1, 90]` with a one-time WARN on clamp.
- Update the threshold computation at `rate_limit.go:77` to read the atomic.
- Files: `github/rate_limit.go`

##### Task 1.2.3b: Threshold tests (~3 min)
- Add `TestSetWarnThresholdPercent_should_ClampToRange_When_OutOfBounds`,
  `TestUpdate_should_WarnEarlier_When_ThresholdRaised`, and
  `TestUpdate_should_KeepMinimumThresholdOfFive_When_LimitIsSmall`.
- Files: `github/rate_limit_test.go`

---

#### Story 1.2.4: Escape hatch for the newly-live pause behaviour
**As** Tyler, **I want** to disable the now-functioning rate-limit pause without reverting the
feature, **so that** reviving four-year-dormant code cannot wedge a poller with no way out short
of a rollback.
**Acceptance Criteria**:
- `STAPLER_SQUAD_GITHUB_PAUSE_ON_LIMIT=false` makes `IsLimited()` always report unlimited while
  still recording and logging everything.
  - *Given* the env var set to `false` and a `429` with `Retry-After: 60`, *When*
    `PRStatusPoller.checkAllSessions` runs its guard at `session/pr_status_poller.go:197`,
    *Then* the tick proceeds as it does today, *and* the `"github API: secondary rate limit hit"`
    WARN plus the `APIUsageEvent` are still recorded — visibility is never traded away for the
    escape hatch.
- The default (unset) enables the pause.
  - *Given* the env var unset, *When* the same `429` arrives, *Then* `IsLimited()` returns `true`
    and the poller logs `"PR status poller: rate limited, skipping tick"`.
- Pausing is tracked as its own visible signal, distinct from `exhaustion_events`, so a clean
  exhaustion count achieved by silent suppression cannot be mistaken for a genuine fix
  (pre-mortem.md P1 #3). Reviving four-year-dormant pause behaviour with no way to see *how much*
  it fired would let "0 exhaustions in 14 days" mean either "no problem" or "pollers went quiet
  the whole time and nobody noticed" — indistinguishable without this.
  - *Given* three separate pause windows during a trial — 90s, 4m, and 12m — *When* the panel's
    window covers all three, *Then* `PauseStats().PauseCount == 3` and
    `PauseStats().TotalPausedDuration ≈ 16m30s`, computed from real pause start/end timestamps,
    not estimated from tick-skip counts.
  - *Given* no pause has occurred in the window, *When* `PauseStats()` is read, *Then*
    `PauseCount == 0` and `TotalPausedDuration == 0` — a real computed zero, mirroring
    `ComputeExhaustionEvents`'s own "must be a real zero, not an unwired default" requirement
    (Task 3.1.1d).
- The poller's own skip log is visible at WARN, not buried at INFO, on the first skip of a given
  pause window (adversarial-review.md's carried-forward recommendation, elevated by
  pre-mortem.md P1 #3).
  - *Given* `session/pr_status_poller.go`'s tick-skip guard (`:198`) fires for the first time
    since `IsLimited()` last returned `false`, *When* the tick is skipped, *Then* the log line is
    emitted at `WARN`, not `INFO`; *and Given* the *next* tick during the same still-active pause
    window, *When* it is also skipped, *Then* no additional WARN is logged (only the transition
    into a pause is loud — a WARN every tick for a multi-minute pause would be its own noise
    problem).
**Files**: `github/rate_limit.go`, `github/rate_limit_test.go`, `session/pr_status_poller.go`,
`session/worktree_pr_poller.go`

##### Task 1.2.4a: Gate `IsLimited`'s return, not `setLimitedUntil` (~3 min)
- Read the env var once into a package `bool` at init; in `IsLimited`, return
  `false, time.Time{}` when disabled. Leave `setLimitedUntil`, the logs, and the snapshot
  untouched so the data stays complete.
- Files: `github/rate_limit.go`

##### Task 1.2.4b: Escape-hatch tests (~2 min)
- Add `TestIsLimited_should_ReportUnlimited_When_PauseDisabledByEnv` and
  `TestIsLimited_should_StillRecordAndLog_When_PauseDisabled`.
- Files: `github/rate_limit_test.go`

##### Task 1.2.4c: Track pause count and cumulative paused duration (~4 min)
- In `github/rate_limit.go`, add `type PauseStats struct { PauseCount int64;
  TotalPausedDuration time.Duration; CurrentlyPaused bool; PausedUntil time.Time }` and a
  `pauseStats atomic.Pointer[PauseStats]` field on `RateLimiter` (copy-on-write, same pattern as
  `QuotaSnapshot` per Task 1.2.2a — never touches the existing `rateLimitedUntil`
  `sync.RWMutex`).
- In `setLimitedUntil` (where `rateLimitedUntil` is set), detect a **new** pause (the previous
  `rateLimitedUntil` was zero or already in the past) vs. an *extension* of an already-active
  pause (e.g. a second `429` arriving before the first pause window ends): only a new pause
  increments `PauseCount`; both cases update `PausedUntil`. Add
  `func (r *RateLimiter) PauseStats() PauseStats` returning the current snapshot, and accumulate
  `TotalPausedDuration` by adding the elapsed wall-clock time each time `IsLimited()` observes
  the pause has ended (transition from `CurrentlyPaused: true` to `false`) — not by summing
  configured pause durations, so an early-cleared pause (e.g. a fresh `200` response arriving
  before `rateLimitedUntil`) is measured accurately rather than over-counted.
- Files: `github/rate_limit.go`

##### Task 1.2.4d: Pause-stats tests (~4 min)
- Add `TestPauseStats_should_CountOnePause_When_MultipleTicksSkipDuringSameWindow`,
  `…_should_CountTwoPauses_When_ASecondPauseStartsAfterTheFirstEnds`,
  `…_should_AccumulateActualElapsedDuration_When_PauseEndsEarly`, and
  `…_should_ReportZero_When_NoPauseHasOccurred`.
- Files: `github/rate_limit_test.go`

##### Task 1.2.4e: Bump the pollers' skip log to WARN on first skip of a pause window (~3 min)
- In `session/pr_status_poller.go` (`:198`) and `session/worktree_pr_poller.go`'s equivalent
  guard, track a per-poller `wasLimited bool` (or read `PauseStats().CurrentlyPaused`'s edge)
  and log `WARN "PR status poller: rate limited, skipping tick"` /
  `WARN "worktree PR poller: rate limited, skipping tick"` only on the transition into a pause;
  subsequent skips within the same still-active window stay silent (or drop to a lower-frequency
  DEBUG) so a multi-minute pause doesn't spam WARN once per tick.
- Files: `session/pr_status_poller.go`, `session/worktree_pr_poller.go`

---

### Epic 1.3: `gh`-CLI instrumentation and dead-code removal

**Goal**: Make the subprocess path visible with exactly one counting point per invocation, and
delete the one function that is scoped for removal rather than instrumentation.

#### Story 1.3.1: Delete `IsForkRepo`
**As a** maintainer, **I want** the confirmed-dead `gh api` shell-out removed, **so that** the
call-site inventory this feature is built around contains only live code.
**Acceptance Criteria**:
- `github/client.go`'s `IsForkRepo` and its only `safeexec` invocation are gone, and the build is
  clean.
  - *Given* the current tree where `grep -rn "IsForkRepo" --include="*.go" .` returns exactly two
    lines (the declaration at `github/client.go:535` and its doc comment at `:534`, with **no**
    callers in production or test), *When* the function is deleted, *Then* `make build` and
    `make lint` both pass with no unused-import or unused-symbol error.
- No behaviour anywhere changes.
  - *Given* the full Go test suite before the deletion, *When* it is re-run after,
    *Then* the same set of tests passes — the deletion touches no call graph.
**Files**: `github/client.go`

##### Task 1.3.1a: Remove the function and re-verify no callers (~2 min)
- Re-run `grep -rn "IsForkRepo" --include="*.go" .` immediately before deleting to confirm the
  zero-caller finding still holds (a caller could have landed since the audit); delete
  `github/client.go:534-553`; run `go build ./...`.
- Files: `github/client.go`

---

#### Story 1.3.2: One wrapper for all 7 `gh` invocations
**As a** developer diagnosing a quota burst, **I want** every `gh` subprocess to produce exactly
one attributed usage event, **so that** a burst of backlog auto-merge PR comments and merges shows
up in the breakdown instead of being invisible.
**Acceptance Criteria**:
- `runGHCommand` is the only place in `github/client.go` that constructs a `gh` subprocess, and
  each of the 7 live sites passes its own `CallSite`.
  - *Given* the finished file, *When*
    `grep -c 'safeexec.CommandContext(ctx, "gh"' github/client.go` and the equivalent for the
    other context variables (`commentsCtx`, `diffCtx`, `commentCtx`, `mergeCtx`, `closeCtx`,
    `cloneCtx`) are run, *Then* the total is exactly 1 (inside `runGHCommand`), and the combined
    number of `runGHCommand(` plus `runGHCommandNoOutput(` **call sites** is exactly 7 — 3 of the
    former (`GetPRInfoCtx`, `GetPRComments`, `GetPRDiff`) and 4 of the latter (`PostPRComment`,
    `MergePR`, `ClosePR`, `CloneRepository`).
- Each invocation records one `invocation_approx` event, regardless of exit status.
  - *Given* `MergePR("tstapler", "stapler-squad", 391, "squash")` where `gh` exits non-zero,
    *When* the call returns its error, *Then* one `APIUsageEvent{CallSite: "gh_cli.merge_pr",
    Fidelity: FidelityInvocationApprox, QuotaCharged: true, StatusCode: 0}` was still recorded —
    a failed merge consumed quota too.
- The wrapper changes no observable behaviour of any of the 7 functions.
  - *Given* the existing error text `"failed to get PR info: <stderr>"` produced at
    `github/client.go:270`, *When* `GetPRInfoCtx` fails after the refactor, *Then* the returned
    error string is character-identical, because `runGHCommand` returns `(stdout []byte, err
    error)` and each caller keeps its own `*exec.ExitError` formatting.
- `gh` rows are explicitly marked as approximations wherever they are aggregated.
  - *Given* a window containing 12 `header_exact` and 5 `invocation_approx` events,
    *When* `GetGitHubAPIUsage` responds, *Then* the source breakdown reports
    `count: 12, approx_count: 5` as separate fields — never a single unqualified `17`.
- A `gh` invocation that fails specifically because of a GitHub rate limit is detected and feeds
  the same exhaustion-detection pipeline `ComputeExhaustionEvents` (Task 3.1.1d) reads — not just
  a raw invocation count. Without this, `runGHCommand`'s events always carry `StatusCode: 0` (per
  the worked example above), which `ComputeExhaustionEvents`'s `StatusCode ∈ {403,429}` predicate
  can never match — so the panel could read "0 exhaustions" for the full 14-day trial while the 7
  in-scope `gh` call sites are actively being rate-limited (pre-mortem.md P1 #1).
  - *Given* `gh pr merge 391 --squash` exits non-zero with stderr containing GitHub's own
    primary-rate-limit phrase (`"API rate limit exceeded"`), *When* `runGHCommand` records the
    event, *Then* it sets `StatusCode: 403`, `Remaining: 0`, `RateLimitRejected: true`,
    `QuotaCharged: false`, and a synthetic `ResetAt` (current time truncated to the hour, plus one
    hour — `gh` does not surface an exact reset timestamp, so this is an approximation, not the
    header-exact value native calls get), and an `APIUsageEvent` recording
    `RateLimitRejected: true` is recorded that counts toward `ComputeExhaustionEvents`'s result.
  - *Given* `gh` stderr containing GitHub's secondary-rate-limit phrase
    (`"secondary rate limit"`), *When* `runGHCommand` records the event, *Then* it sets
    `StatusCode: 429` (with the same `Remaining`/`RateLimitRejected`/`ResetAt` treatment as the
    primary case), so secondary-limit `gh` bounces are distinguishable from primary ones the same
    way `github/rate_limit.go`'s existing WARN lines already distinguish them.
  - *Given* `gh` fails for any other reason (network error, invalid args, a genuine merge
    conflict), *When* `runGHCommand` records the event, *Then* `StatusCode` stays `0` and
    `RateLimitRejected` stays `false` — the detection is a narrow stderr-phrase match, not a
    blanket "any failure is a rate limit" assumption.
  - *Given* this detection lands, *When* Story 5.2.3's reference doc is written, *Then* it states
    explicitly that `gh`-CLI exhaustion detection is stderr-pattern-based (approximate, no
    `X-RateLimit-*` headers available) rather than header-exact, so a reader does not read
    "0 gh-CLI exhaustions" with the same confidence as a native-transport "0."
**Files**: `github/client.go`, `github/client_gh_wrapper_test.go`

##### Task 1.3.2a: Implement `runGHCommand` (~4 min)
- Add to `github/client.go`:
  `func runGHCommand(ctx context.Context, cs CallSite, args ...string) ([]byte, error)` which
  times the call, invokes `safeexec.CommandContext(ctx, "gh", args...).Output()`, and emits one
  `APIUsageEvent{CallSite: cs, Resource: ResourceUnknown, Fidelity: FidelityInvocationApprox,
  QuotaCharged: true, Duration: elapsed, SessionID: SessionAttributionFromContext(ctx)}` in a
  `defer`, then returns the raw output and error untouched.
- Add a sibling `runGHCommandNoOutput` for `PostPRComment`/`MergePR`/`ClosePR`/`CloneRepository`
  which use `cmd.Run()`, so their existing semantics are preserved.
- Document the one-invocation-≈-one-request approximation in the function doc comment, citing
  `../research/pitfalls.md` §1c.
- Files: `github/client.go`

##### Task 1.3.2b: Route all 7 call sites through it (~5 min)
- Replace the `safeexec` construction at `client.go:266` (`GetPRInfoCtx`), `:567`
  (`GetPRComments`), `:611` (`GetPRDiff`), `:634` (`PostPRComment`), `:667` (`MergePR`), `:689`
  (`ClosePR`), `:709` (`CloneRepository`) with the corresponding `runGHCommand*` call and its
  `CallSite` constant. Leave the four `git` invocations at `:725`, `:740`, `:762` alone — they are
  not GitHub API calls.
- Files: `github/client.go`

##### Task 1.3.2c: Wrapper tests + the single-seam guard (~5 min)
- Add `TestRunGHCommand_should_RecordOneApproxEvent_When_CommandSucceeds`,
  `…_When_CommandFails`, and
  `TestClientGo_should_ConstructGHSubprocessInExactlyOnePlace` — a source-scanning test over
  `github/client.go` asserting exactly one `safeexec.CommandContext(<ctx>, "gh"` occurrence, so a
  future call site added outside the wrapper fails CI rather than silently going uncounted
  (`../research/pitfalls.md` §2c).
- Stub `gh` via a `t.TempDir()` PATH entry containing a shell script, per the existing test idiom
  in this package.
- Files: `github/client_gh_wrapper_test.go`

##### Task 1.3.2d: Detect `gh`-CLI rate-limit exhaustion from stderr (~5 min)
- Add `func classifyGHRateLimitFailure(exitErr *exec.ExitError) (statusCode int, rejected bool)`
  to `github/client.go`: on a non-zero exit, inspect the captured stderr (already available from
  `safeexec.CommandContext(...).Output()`'s `*exec.ExitError.Stderr` for `runGHCommand`, and from
  the buffered stderr `runGHCommandNoOutput` already captures for its existing error-formatting)
  for a case-insensitive match on `"api rate limit exceeded"` → `(403, true)`, or
  `"secondary rate limit"` / `"you have exceeded a secondary rate limit"` → `(429, true)`;
  anything else → `(0, false)`.
- In both `runGHCommand` and `runGHCommandNoOutput`'s deferred event-recording block, when
  `classifyGHRateLimitFailure` reports `rejected == true`, override the emitted
  `APIUsageEvent`'s `StatusCode`, set `RateLimitRejected: true`, `QuotaCharged: false`, and
  `ResetAt: time.Now().Truncate(time.Hour).Add(time.Hour)` (an hour-bucketed approximation, since
  `gh` surfaces no exact reset timestamp — documented inline as a known imprecision: two genuinely
  separate `gh`-CLI exhaustions within the same clock hour will dedupe into one
  `ComputeExhaustionEvents` count per Task 3.1.1d's `(Resource, ResetAt)` key, which undercounts
  in that narrow case but never overcounts, and is still strictly better than the current "always
  0" blind spot). Emit `WARN "github usage: gh CLI rate-limit signal detected"` with `call_site`
  and the classified `status_code`, mirroring `rate_limit.go`'s existing WARN convention.
- Do not change the function's returned `error` — the caller-visible error text stays
  character-identical (Story 1.3.2's existing "no observable behaviour change" acceptance
  criterion still applies); only the recorded `APIUsageEvent` changes.
- Files: `github/client.go`

##### Task 1.3.2e: gh-CLI rate-limit detection tests (~4 min)
- Add `TestClassifyGHRateLimitFailure_should_DetectPrimaryLimitPhrase_When_StderrMatches`,
  `…_should_DetectSecondaryLimitPhrase_When_StderrMatches`,
  `…_should_ReportNotRejected_When_StderrIsUnrelatedFailure`,
  `TestRunGHCommand_should_RecordSyntheticExhaustionSignal_When_GhExitsWithRateLimitStderr`
  (stub `gh` to exit 1 with the rate-limit stderr text; assert the recorded event's `StatusCode`/
  `RateLimitRejected`/`QuotaCharged`), and
  `TestRunGHCommandNoOutput_should_RecordSyntheticExhaustionSignal_When_GhExitsWithRateLimitStderr`
  (same, for the `MergePR`/`ClosePR`/`PostPRComment`/`CloneRepository` path).
- Files: `github/client_gh_wrapper_test.go`

---

#### Story 1.3.3: Stamp attribution at the originating call sites
**As** the usage panel, **I want** each background poller and on-demand handler to identify
itself, **so that** "which poller do I turn down" has an answer rather than a wall of `unknown`.
**Acceptance Criteria**:
- Both timer-driven pollers stamp a `CallSite` and no session ID.
  - *Given* `PRStatusPoller.checkAllSessions` runs a tick that fetches 3 PRs,
    *When* the events land, *Then* all 3 carry `CallSite: "pr_status_poller"` and `SessionID: ""`,
    because a global singleton timer has no session to attribute to
    (`../research/features.md` §6).
- On-demand handlers stamp both a `CallSite` and the originating session where one exists.
  - *Given* the MCP tool `list_github_prs` invoked from session `sess-7f3a`,
    *When* it calls into `github`, *Then* the recorded event carries
    `CallSite: "mcp_backlog_tools"` and `SessionID: "sess-7f3a"`.
- Unstamped traffic is visible, not hidden.
  - *Given* a native call from a path nobody remembered to stamp,
    *When* the panel renders, *Then* an `unknown` row appears with its count, so the gap is
    obvious rather than absorbed into another source.
- The two on-demand backlog GitHub-lookup RPCs stamp their own `CallSite`, not just the
  issue-import path.
  - *Given* `BacklogService.SearchGitHubRepos` (→ `gh.SearchUserRepos`) is invoked from the
    omnibar's repo-search box, *When* the resulting event is recorded, *Then* it carries
    `CallSite: "backlog_search_repos"` rather than `unknown`; *and Given*
    `BacklogService.ListGitHubIssues` (→ `gh.ListRepoIssues`) is invoked from the issue picker,
    *Then* its event carries `CallSite: "backlog_list_issues"` — both requests are named in
    `requirements.md`'s Scope and are counted by `usageTransport` regardless, so leaving them
    unstamped would silently undercut this story's own purpose for exactly two of the RPCs it
    exists to cover.
**Files**: `session/pr_status_poller.go`, `session/worktree_pr_poller.go`,
`session/backlog_plugin_github.go`, `server/mcp/tools_github.go`,
`server/services/backlog_service_sync.go`, `server/services/backlog_service_query.go`

##### Task 1.3.3a: Stamp the two pollers (~3 min)
- In `session/pr_status_poller.go`'s `checkAllSessions` (from `:196`) and
  `session/worktree_pr_poller.go`'s equivalent (from `:186`), derive the per-tick context as
  `ctx := github.WithCallSite(p.ctx, github.CallSitePRStatusPoller)` (respectively
  `CallSiteWorktreePRPoller`) and pass it into the fetch calls.
- Files: `session/pr_status_poller.go`, `session/worktree_pr_poller.go`

##### Task 1.3.3b: Stamp the on-demand handlers (~4 min)
- Stamp `CallSiteBacklogIssueImport` in `session/backlog_plugin_github.go`'s import path,
  `CallSiteMCPBacklogTools` in `server/mcp/tools_github.go` and `server/mcp/tools_backlog.go`
  (adding `WithSessionAttribution` where a session ID is in scope), and
  `CallSiteBacklogForwardSync` in `server/services/backlog_service_sync.go`.
- Files: `session/backlog_plugin_github.go`, `server/mcp/tools_github.go`,
  `server/mcp/tools_backlog.go`, `server/services/backlog_service_sync.go`

##### Task 1.3.3c: Stamp the in-package native callers (~3 min)
- Inside `github`, stamp `CallSiteAuthCheck` in `CheckGHAuth` (`github/client.go:158`),
  `CallSiteUserPRCache` in `github/user_pr_cache.go`'s fetch (`:590-608`), and
  `CallSiteETagConditional` in `github/etag_cache.go`'s conditional request path (`:65`).
- Files: `github/client.go`, `github/user_pr_cache.go`, `github/etag_cache.go`

##### Task 1.3.3d: Stamp the backlog GitHub search/list RPC handlers (~3 min)
- In `server/services/backlog_service_query.go`, wrap the context passed to `gh.SearchUserRepos`
  in `SearchGitHubRepos` with `github.WithCallSite(ctx, github.CallSiteBacklogSearchRepos)`, and
  the context passed to `gh.ListRepoIssues` in `ListGitHubIssues` with
  `github.WithCallSite(ctx, github.CallSiteBacklogListIssues)` — mirroring how Task 1.3.3b stamps
  `ImportGitHubIssue` in the sibling file `server/services/backlog_service_sync.go`. Neither
  handler currently carries a session ID in its request message, so no
  `WithSessionAttribution` call is added here (consistent with the pollers' call-site-only
  stamping in Task 1.3.3a).
- Files: `server/services/backlog_service_query.go`

---

## Phase 2: Persistence

### Epic 2.1: The buffered usage recorder

**Goal**: Get events off the request path and into `analytics.db` without ever blocking a GitHub
call, per ADR-027.

#### Story 2.1.1: `GitHubUsageRecorder` with a bounded buffer and drain-on-stop
**As a** GitHub request, **I want** to hand off my usage event in nanoseconds, **so that** a
contended SQLite write never adds latency to a poller's time-boxed call.
**Acceptance Criteria**:
- `Record` is non-blocking and drops with a counter when the buffer is full.
  - *Given* a recorder constructed with `bufferSize = 4` and a deliberately stalled flush,
    *When* 10 events are recorded, *Then* all 10 calls return without blocking, `Dropped()`
    returns 6, and one `WARN "github usage recorder buffer full — dropping event"` was logged.
- `Stop()` drains what is buffered before returning.
  - *Given* 3 buffered events and a working provider, *When* `Stop()` is called,
    *Then* it returns only after all 3 rows are visible via
    `client.AnalyticsEvent.Query().Where(analyticsevent.EventCategoryEQ("github_api")).Count(ctx)`
    == 3 — synchronised on the recorder's own done channel, **not** a `time.Sleep`
    (`.claude/rules/fix-flaky-tests-dont-defer.md`).
- `Stop()` never races a concurrent `RecordAPIUsage` into a "send on closed channel" panic.
  - *Given* one goroutine calling `RecordAPIUsage` in a tight loop and another calling `Stop()`
    concurrently, *When* `go test -race ./server/analytics/ -run
    TestGitHubUsageRecorder_should_NotPanic_When_RecordAPIUsageRacesStop` runs for 200+
    iterations, *Then* no panic and no data race occur, because `Stop()` sets an internal
    `closing atomic.Bool` and waits on an internal `sync.WaitGroup` of in-flight
    `RecordAPIUsage` calls *before* closing `ch` — never closing `ch` while a send to it could
    still be in flight (architecture-review.md Blocker 1).
- Events map onto `AnalyticsEvent` exactly as ADR-027 specifies.
  - *Given* `APIUsageEvent{CallSite: "gh_cli.merge_pr", Resource: ResourceUnknown,
    Fidelity: FidelityInvocationApprox, SessionID: "sess-7f3a", Duration: 812ms,
    QuotaCharged: true, StatusCode: 200, Remaining: 4999, Limit: 5000}`, *When* it is flushed,
    *Then* the persisted row has `event_category="github_api"`, `event_name="gh_cli.merge_pr"`,
    `session_id="sess-7f3a"`, `duration_ms=812`, and `labels` containing
    `{"resource":"unknown","fidelity":"invocation_approx","quota_charged":"true",
    "rate_limit_rejected":"false","status_code":"200","remaining":"4999","limit":"5000"}`.
    `status_code`/`remaining`/`limit`/`quota_charged`/`rate_limit_rejected` are not decorative —
    `GitHubUsageQuery.ComputeExhaustionEvents` (Story 3.1.1) and `ComputeReconciliation` (Story
    5.1.1) both read them back out of `labels` via `parseUsageRow`, so omitting any of them here
    would silently make those aggregations impossible to compute from stored data.
- A provider write failure degrades to a log, never a panic or a stall.
  - *Given* a provider returning an error for every `Record`, *When* 5 events flush,
    *Then* the recorder logs a WARN with a failure count and keeps accepting new events —
    matching the best-effort telemetry posture in `../research/features.md` §5.
**Files**: `server/analytics/github_usage.go`, `server/analytics/github_usage_test.go`

##### Task 2.1.1a: Implement the recorder skeleton (~5 min)
- Create `server/analytics/github_usage.go` with `EventCategoryGitHubAPI = "github_api"`,
  `type GitHubUsageRecorder struct { ch chan github.APIUsageEvent; provider AnalyticsProvider;
  dropped, failed atomic.Int64; done chan struct{}; closing atomic.Bool; inFlight sync.WaitGroup }`,
  `NewGitHubUsageRecorder(provider, size)`.
- Implement `RecordAPIUsage(ev)` to be safe against a concurrent `Stop()`: return immediately
  (dropped, uncounted) if `closing.Load()` is already true; otherwise `inFlight.Add(1)`, `defer
  inFlight.Done()`, re-check `closing.Load()` once more (in case `Stop()` started between the
  first check and `Add`) and return without sending if so, then do the non-blocking
  `select`/`default` send with drop+count.
- Implement `Stop()` as: `closing.Store(true)` (so no new send can start), `inFlight.Wait()` (so
  every send that *did* start is guaranteed finished), **then** `close(ch)`, wait on `done`, and
  return after the final drain. This ordering — stop new sends, wait for in-flight sends, only
  then close — is what makes closing `ch` safe (architecture-review.md Blocker 1); closing before
  the `Wait()` would still race an in-flight send.
- Files: `server/analytics/github_usage.go`

##### Task 2.1.1b: Implement the flush loop and the row mapping (~5 min)
- Add `flush(ctx)` draining the channel on a 5s ticker (mirroring `analytics_store.go`'s cadence)
  and a pure `func toAnalyticsEvent(ev github.APIUsageEvent) Event` performing the ADR-027 mapping,
  including omitting `session_id`/`duration_ms` when zero so the columns stay NULL (matching
  `SQLiteAnalyticsProvider.Record`'s existing convention at `sqlite_provider.go:38-50`).
- The `labels` map must include `resource`, `fidelity`, `quota_charged`, `rate_limit_rejected`,
  `status_code`, `remaining`, and `limit` for every event (stringified) — these are the fields
  `GitHubUsageQuery`'s `ComputeExhaustionEvents` (Story 3.1.1) and `ComputeReconciliation` (Story
  5.1.1) read back out; a partial label set would leave those aggregations with no data to compute
  from despite the storage layer otherwise looking complete.
- Files: `server/analytics/github_usage.go`

##### Task 2.1.1c: Recorder tests, synchronised not slept (~5 min)
- Add `TestGitHubUsageRecorder_should_NotBlock_When_BufferFull`,
  `TestGitHubUsageRecorder_should_DrainBufferedEvents_When_Stopped`,
  `TestToAnalyticsEvent_should_MapAllLabels_When_EventFullyPopulated`,
  `TestGitHubUsageRecorder_should_KeepAcceptingEvents_When_ProviderWriteFails`, and
  `TestGitHubUsageRecorder_should_NotPanic_When_RecordAPIUsageRacesStop` (run under `-race`,
  hammering `RecordAPIUsage` from N goroutines while `Stop()` is called concurrently —
  architecture-review.md Blocker 1).
- Back the tests with a real in-memory ent client (`OpenAnalyticsDB` against `t.TempDir()`), so the
  mapping is asserted against the actual schema rather than a mock.
- Files: `server/analytics/github_usage_test.go`

---

#### Story 2.1.2: Wire the recorder into server startup
**As the** running service, **I want** the recorder installed as soon as the analytics DB is open
and shut down cleanly, **so that** events are captured for the whole process lifetime and none are
lost at exit.
**Acceptance Criteria**:
- The recorder is installed via `github.SetUsageRecorder` right after the analytics client opens,
  and falls back to the null object when it doesn't.
  - *Given* `analytics.OpenAnalyticsDB` fails at `server/dependencies.go:960` (disk full),
    *When* the server finishes starting, *Then* `deps.AnalyticsEntClient` is nil, no recorder is
    installed, GitHub calls proceed normally, and one
    `WARN "github usage tracking disabled (no analytics DB)"` is logged — tracking degrades, the
    app does not.
- Startup order guarantees no event is emitted into a live-but-unstarted recorder.
  - *Given* the wiring, *When* the first poller tick fires, *Then* `Start(serverCtx)` has already
    been called, verified by asserting `Started()` is true before `PRStatusPoller.Start` is
    invoked in a wiring test.
- Shutdown drains.
  - *Given* `serverCtx` cancellation during shutdown, *When* the server exits, *Then*
    `GitHubUsageRecorder.Stop()` ran and the drop counter is logged at INFO with the final totals.
- Shutdown swaps the package-level recorder to the noop *before* stopping it, so an in-flight
  request racing shutdown never panics.
  - *Given* a goroutine mid-`RoundTrip` (its `recordUsage` call in flight) when shutdown begins,
    *When* `server.go`'s shutdown path runs, *Then* it calls `github.SetUsageRecorder(nil)` (which
    installs `noopUsageRecorder`, per Story 1.1.3) **before** calling `rec.Stop()` — not after —
    so any request that hasn't yet loaded the recorder pointer observes the noop, and the
    `GitHubUsageRecorder`'s own `closing`/`inFlight` guard (Task 2.1.1a) covers the residual case
    of a request that loaded the real recorder microseconds earlier. The combination is what makes
    "in-flight request during shutdown does not panic" true under `-race`, not either guard alone.
**Files**: `server/server.go`, `server/dependencies.go`

##### Task 2.1.2a: Construct and install the recorder (~4 min)
- In `server/server.go`, immediately after the existing provider selection block
  (`server.go:622-630`), construct `analytics.NewGitHubUsageRecorder(analyticsProvider, 2048)`,
  call `go rec.Start(serverCtx)`, then `githubpkg.SetUsageRecorder(rec)`, logging
  `"GitHub usage recorder started"` — or the disabled-path WARN when
  `deps.AnalyticsEntClient == nil`.
- Files: `server/server.go`

##### Task 2.1.2b: Register the shutdown drain, recorder swap before close (~3 min)
- In the shutdown path, immediately before calling `rec.Stop()`, call
  `githubpkg.SetUsageRecorder(nil)` so every new call to `recordUsage()` from this point on
  resolves to `noopUsageRecorder` instead of the recorder that's about to have its channel closed
  (architecture-review.md Blocker 1). Only then call `rec.Stop()`, hooked into the same shutdown
  path that closes `deps.AnalyticsEntClient`, so the drain happens before the DB handle closes.
- Order in code: `githubpkg.SetUsageRecorder(nil)` → `rec.Stop()` → close `deps.AnalyticsEntClient`.
  Getting this backwards (closing the DB or calling `Stop()` first) reopens the race.
- Files: `server/server.go`, `server/dependencies.go`

---

### Epic 2.2: Retention

#### Story 2.2.1: Category-scoped retention for `github_api` rows
**As** Tyler, **I want** GitHub usage rows to self-limit, **so that** months of accumulation
neither grow unbounded nor evict my UI telemetry through the shared 100 000-row cap.
**Acceptance Criteria**:
- A `github_api`-scoped age prune runs before the existing global count phase.
  - *Given* `GitHubUsageRetentionDaysOrDefault() == 30`, 200 `github_api` rows aged 45 days and
    50 aged 5 days, *When* `runRetention` executes one cycle, *Then* the 200 old rows are deleted,
    the 50 recent ones remain, and rows in other categories are untouched.
- Disabling is explicit.
  - *Given* `github_usage_retention_days` set to `0` in config, *When* retention runs, *Then* no
    `github_api`-scoped deletion occurs and only the existing global age/count phases apply.
- The 14-day Success-Metric window is never at risk.
  - *Given* 30-day retention and the global 90-day/100 000-row caps,
    *When* any eviction runs, *Then* every `github_api` row newer than 14 days survives, because
    both phases delete oldest-first and 14 < 30.
**Files**: `server/analytics/retention.go`, `server/analytics/retention_test.go`,
`server/server.go`

##### Task 2.2.1a: Add the scoped prune phase (~4 min)
- In `server/analytics/retention.go`, add
  `func runGitHubUsageRetention(ctx, client, retentionDays int)` deleting
  `analyticsevent.EventCategoryEQ(EventCategoryGitHubAPI)` AND `CreatedAtLT(cutoff)`, and call it
  from `runRetention` (`retention.go:49`) as a new Phase 0.5, before the existing age phase.
- Extend `StartRetentionEnforcer`'s signature with `githubUsageRetentionDays int` and update its
  sole caller at `server/server.go:635`.
- Files: `server/analytics/retention.go`, `server/server.go`

##### Task 2.2.1b: Retention tests (~4 min)
- Add `TestRunGitHubUsageRetention_should_DeleteOnlyGitHubRowsOlderThanCutoff`,
  `…_should_NoOp_When_RetentionDaysIsZero`, and
  `…_should_PreserveOtherCategories_When_Pruning`, seeding rows through the real ent client.
- Files: `server/analytics/retention_test.go`

---

## Phase 3: Read Model, RPC, and Configuration

### Epic 3.1: The aggregation read model

#### Story 3.1.1: Windowed usage query and pure aggregation
**As the** RPC handler, **I want** windowed, DB-filtered reads with pure in-memory aggregation,
**so that** the panel is fast and the aggregation logic is unit-testable without a database.
**Acceptance Criteria**:
- Reads are time-bounded at the database, not in Go.
  - *Given* 30 days of rows and a request for `windowDays = 7`, *When* `LoadWindow(ctx, since)`
    runs, *Then* the ent query carries `analyticsevent.CreatedAtGTE(since)` and
    `EventCategoryEQ("github_api")` — never a full-table fetch followed by a Go-side filter
    (`../research/pitfalls.md` §3b).
- `ComputeSourceBreakdown` separates exact and approximate counts and includes zero rows.
  - *Given* rows `{pr_status_poller × 40 exact, gh_cli.merge_pr × 3 approx}` and
    `AllCallSites` containing `worktree_pr_poller`, *When* `ComputeSourceBreakdown(rows)` runs,
    *Then* it returns `[{pr_status_poller, count:40, approxCount:0, share:93.0},
    {gh_cli.merge_pr, count:0, approxCount:3, share:7.0},
    {worktree_pr_poller, count:0, approxCount:0, share:0}]`.
- `ComputeVolumeBuckets` buckets daily for windows > 2 days and hourly otherwise, in local time.
  - *Given* `windowDays = 1` and 26 events spread across 3 clock hours, *When*
    `ComputeVolumeBuckets(rows, 1)` runs, *Then* it returns 24 hourly buckets covering the window
    with the 3 populated, so a burst is visible rather than flattened into one daily bar.
- `ComputeBurnRate` and `ComputeTimeToExhaustion` are pure and nil-safe.
  - *Given* `ResourceQuota{Remaining: 900, Limit: 5000, ResetAt: now+50m}` and 340
    quota-charged core events in the last hour, *When* `ComputeTimeToExhaustion` runs,
    *Then* it returns ~2h39m — i.e. **later than the reset**, which the panel must render as
    "resets first, no action needed", distinguishing it from the 10-minutes-to-zero case a static
    "18% remaining" gauge cannot.
  - *Given* zero events in the window, *When* `ComputeTimeToExhaustion` runs, *Then* it returns
    `nil` and the panel shows "—", not `+Inf`.
- `ComputeExhaustionEvents` counts primary-rate-limit-exhaustion incidents from stored rows —
  this is the Success Metric's own counter, and it must actually be computed, not left at its
  zero-value default (architecture-review.md Blocker 2, adversarial-review.md Blocker 3).
  - *Given* rows for the `core` resource where 4 consecutive requests each carry
    `StatusCode ∈ {403,429}` and `Remaining == 0` with the *same* `ResetAt` (a poller retrying
    every ~60s against the `setLimitedUntil` clamp described in
    `../research/pitfalls.md`/adversarial-review.md while still exhausted), *When*
    `ComputeExhaustionEvents(rows)` runs, *Then* it returns `1` — deduped by
    `(Resource, ResetAt)` so one exhaustion incident isn't inflated into 4 just because the poller
    retried during it. This mirrors `rate_limit.go`'s own detection condition for the
    `"github API: primary rate limit exhausted"` WARN (`remaining == 0` on a non-304 response),
    computed from the same `APIUsageEvent.StatusCode`/`Remaining` fields Task 1.2.1a already
    records — no new detection logic is needed at the transport layer, only the aggregation.
  - *Given* a second `core` exhaustion later in the window with a *different* `ResetAt`,
    *When* `ComputeExhaustionEvents(rows)` runs, *Then* it returns `2` — a genuinely new
    exhaustion window is not folded into the first.
  - *Given* zero rows matching the predicate, *When* `ComputeExhaustionEvents(rows)` runs,
    *Then* it returns `0` — a real, computed zero, which is what makes a "0 exhaustions in 14
    days" reading in the panel a verified claim rather than an unwired default.
**Files**: `server/analytics/github_usage_query.go`,
`server/analytics/github_usage_query_test.go`

##### Task 3.1.1a: Implement the windowed loader (~4 min)
- Create `server/analytics/github_usage_query.go` with
  `type GitHubUsageQuery struct { client *ent.Client }`, `NewGitHubUsageQuery(client)`,
  `LoadWindow(ctx, since time.Time) ([]UsageRow, error)` selecting only
  `event_name`, `session_id`, `duration_ms`, `labels`, `created_at`, and a
  `func parseUsageRow(*ent.AnalyticsEvent) UsageRow` decoding the labels map back into typed
  fields.
- `UsageRow` must include `StatusCode int`, `Remaining int`, `Limit int`, `QuotaCharged bool`,
  `RateLimitRejected bool`, `Resource github.GitHubResource`, and `Fidelity github.Fidelity`
  decoded from `labels`, plus `CallSite` from `event_name` and `CreatedAt` — `StatusCode`/
  `Remaining` are what `ComputeExhaustionEvents` (Task 3.1.1d) and `ComputeReconciliation`
  (Story 5.1.1) key off of; a `UsageRow` missing them would make both un-implementable.
- Files: `server/analytics/github_usage_query.go`

##### Task 3.1.1b: Implement the pure aggregators (~5 min)
- Add `ComputeSourceBreakdown([]UsageRow) []SourceStat` (seeded from `github.AllCallSites` so
  zero rows appear), `ComputeVolumeBuckets([]UsageRow, windowDays int) []VolumeBucket`,
  `ComputeBurnRate([]UsageRow, github.GitHubResource, window time.Duration) float64`, and
  `ComputeTimeToExhaustion(github.ResourceQuota, burnRate float64) *time.Duration`.
- Files: `server/analytics/github_usage_query.go`

##### Task 3.1.1c: Aggregation tests (~5 min)
- Add `TestComputeSourceBreakdown_should_SeparateExactAndApproxCounts`,
  `…_should_IncludeZeroRowsForKnownCallSites`,
  `TestComputeVolumeBuckets_should_UseHourlyBuckets_When_WindowIsOneDay`,
  `TestComputeTimeToExhaustion_should_ReturnNil_When_BurnRateIsZero`,
  `TestComputeTimeToExhaustion_should_ExceedResetWindow_When_BurnRateIsLow`, and
  `TestLoadWindow_should_FilterAtDatabase_When_WindowRequested` (asserts row count from a seeded
  DB containing out-of-window rows).
- Files: `server/analytics/github_usage_query_test.go`

##### Task 3.1.1d: Implement `ComputeExhaustionEvents` (~4 min)
- Add `func ComputeExhaustionEvents(rows []UsageRow) int` to `github_usage_query.go`: filter rows
  where `StatusCode == 403 || StatusCode == 429` and `Remaining == 0` (the same predicate
  `rate_limit.go`'s existing "primary rate limit exhausted" WARN fires on), then count distinct
  `(Resource, ResetAt)` pairs among the matches — a `map[struct{ Resource
  github.GitHubResource; ResetAt time.Time }]struct{}` is sufficient, no new type needed. This is
  the Success Metric's counter (architecture-review.md Blocker 2, adversarial-review.md Blocker
  3): it must exist as a real computation over stored rows, not a field left at its zero-value
  default.
- Add `TestComputeExhaustionEvents_should_CountRowsWithZeroRemainingAndLimitStatus`,
  `…_should_DedupeRepeatedRetriesWithinSameResetWindow_When_PollerRetriesDuringExhaustion`,
  `…_should_CountSeparately_When_ResetAtDiffersAcrossExhaustions`, and
  `…_should_ReturnZero_When_NoRowsMatch`.
- Files: `server/analytics/github_usage_query.go`, `server/analytics/github_usage_query_test.go`

---

### Epic 3.2: The ConnectRPC surface

#### Story 3.2.1: `GetGitHubAPIUsage` proto contract
**As the** web UI, **I want** one typed RPC returning current quota, volume over time, source
breakdown, and trust signals, **so that** the panel needs a single round-trip to answer all three
JTBD questions.
**Acceptance Criteria**:
- The request mirrors the existing analytics window convention exactly.
  - *Given* `GetGitHubAPIUsageRequest{window_days: 200}`, *When* the handler runs,
    *Then* the window is clamped to 90 — identical semantics to
    `GetApprovalAnalyticsRequest.window_days` (`proto/session/v1/session.proto:1438-1441`),
    so the frontend selector code can be lifted from `ApprovalAnalyticsPanel` unchanged.
- The response never blends resources.
  - *Given* observed quota for `core` and `search`, *When* the response is built,
    *Then* `quotas` is a repeated `GitHubQuotaProto` with one entry per resource — there is no
    scalar "overall quota" field anywhere in the message, making the blended-number mistake
    (`../research/ux.md` §4) unrepresentable.
- Trust signals are first-class response fields, not derived client-side.
  - *Given* a window in which 6 events were dropped by buffer overflow and 40 requests were
    unaccounted by reconciliation, *When* the response is built, *Then* it carries
    `dropped_events: 6` and `unaccounted_requests: 40`, so the panel can qualify its own numbers.
- Pause visibility is a first-class response field too, not just a log line
  (pre-mortem.md P1 #3).
  - *Given* `github.DefaultRateLimiter.PauseStats()` (Task 1.2.4c) reporting
    `{PauseCount: 3, TotalPausedDuration: 16m30s, CurrentlyPaused: false}`, *When* the response is
    built, *Then* it carries `pause_events: 3` and `total_paused_seconds: 990` — so a "0
    exhaustions" reading can be cross-checked against "how much did the pause escape hatch
    actually fire" in the same round-trip, without a second request.
**Files**: `proto/session/v1/session.proto`, `proto/session/v1/types.proto`

##### Task 3.2.1a: Add the shared message types (~4 min)
- In `proto/session/v1/types.proto`, add `GitHubQuotaProto` (`resource`, `remaining`, `limit`,
  `reset_at`, `observed_at`, `burn_rate_per_hour`, `seconds_to_exhaustion`,
  `warn_threshold_percent`), `GitHubSourceStatProto` (`call_site`, `count`, `approx_count`,
  `share_percent`), and `GitHubVolumeBucketProto` (`bucket_start`, `total`, `per_resource` map).
- Files: `proto/session/v1/types.proto`

##### Task 3.2.1b: Add the RPC and its request/response (~3 min)
- In `proto/session/v1/session.proto`, add
  `rpc GetGitHubAPIUsage(GetGitHubAPIUsageRequest) returns (GetGitHubAPIUsageResponse) {}`
  adjacent to `GetApprovalAnalytics`/`GetProgramAnalytics` (`:154-159`), plus the two messages
  with `optional int32 window_days = 1;` in, and `quotas`, `volume_buckets`, `sources`,
  `total_requests`, `total_approx_requests`, `exhaustion_events`, `dropped_events`,
  `unaccounted_requests`, `tracking_available`, `pause_events` (int32),
  `total_paused_seconds` (int64) out. `pause_events`/`total_paused_seconds` are the pause-visibility
  fields from Task 1.2.4c (pre-mortem.md P1 #3).
- Files: `proto/session/v1/session.proto`

##### Task 3.2.1c: Regenerate bindings (~2 min)
- Run `make proto-gen`; confirm `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/*_pb.ts` are updated and committed together (the generated TS is
  tracked despite `.gitignore`).
- Files: `session/gen/session/v1/**`, `web-app/src/gen/session/v1/**`

---

#### Story 3.2.2: `GetGitHubAPIUsage` handler
**As the** panel, **I want** the handler to degrade to an empty-but-labelled response rather than
erroring, **so that** a missing analytics DB shows "tracking unavailable" instead of a red error
banner that looks like a bug.
**Acceptance Criteria**:
- Missing analytics client returns a successful response with `tracking_available = false`.
  - *Given* `SessionService.analyticsClient == nil`, *When* `GetGitHubAPIUsage` is called,
    *Then* it returns a `200` response with `tracking_available: false` and empty collections —
    **not** `CodeUnavailable` — so the UI can distinguish "tracking unavailable" from
    "0 requests observed", the conflation warned about in `../research/features.md` §5.
- A query error degrades to an empty summary plus a WARN, matching the house convention.
  - *Given* `LoadWindow` returning an error, *When* the handler runs, *Then* it logs a WARN and
    returns empty aggregates with `tracking_available: false`, mirroring
    `rules_service.go`'s `GetApprovalAnalytics` behaviour.
- Live quota comes from the snapshot, not the DB.
  - *Given* `github.DefaultRateLimiter.Snapshot()` holding
    `{core: {Remaining: 4200, Limit: 5000, ObservedAt: now-14m}}`, *When* the handler runs,
    *Then* the response's core `GitHubQuotaProto` has `remaining: 4200` and
    `observed_at: now-14m`, letting the panel render "as of 14m ago".
- `exhaustion_events` is populated from `ComputeExhaustionEvents`, never left at its zero-value
  default (architecture-review.md Blocker 2, adversarial-review.md Blocker 3).
  - *Given* `LoadWindow` returning rows containing 2 distinct primary-rate-limit-exhaustion
    incidents (per Task 3.1.1d's dedupe rule), *When* the handler runs, *Then*
    `GetGitHubAPIUsageResponse.exhaustion_events == 2` — a value actually derived from stored
    data, not a struct field nobody assigned.
- The handler carries the API marker so the feature registry picks it up.
  - *Given* `make registry-generate`, *When* it scans `server/services/`, *Then* the
    `// +api: github:usage` marker on the handler yields a backend feature entry.
- `pause_events`/`total_paused_seconds` are populated from `PauseStats()`, never left at zero-value
  (pre-mortem.md P1 #3, same discipline as `exhaustion_events`).
  - *Given* `github.DefaultRateLimiter.PauseStats()` reporting `{PauseCount: 3,
    TotalPausedDuration: 16m30s}`, *When* the handler runs, *Then*
    `GetGitHubAPIUsageResponse.pause_events == 3` and `total_paused_seconds == 990`.
**Files**: `server/services/github_usage_service.go`,
`server/services/github_usage_service_test.go`, `server/features/analytics.go`

##### Task 3.2.2a: Implement the handler (~5 min)
- Create `server/services/github_usage_service.go` with
  `func (s *SessionService) GetGitHubAPIUsage(ctx, req) (*connect.Response[...], error)` carrying
  `// +api: github:usage`, clamping `window_days` to `[1, 90]` (default 7), reading
  `github.DefaultRateLimiter.Snapshot()` and `github.DefaultRateLimiter.PauseStats()` (Task
  1.2.4c), and running the `GitHubUsageQuery` aggregations. The handler must explicitly populate
  every proto field it owns from a named aggregator — in particular `exhaustion_events` from
  `ComputeExhaustionEvents(rows)` (Task 3.1.1d), `dropped_events` from the recorder's drop
  counter, and `pause_events`/`total_paused_seconds` from `PauseStats()` — rather than leaving
  any of them at Go's zero-value default, which would be indistinguishable from a real "0" in the
  response.
- Files: `server/services/github_usage_service.go`

##### Task 3.2.2b: Register the feature and handler tests (~4 min)
- Add `AnalyticsGetGitHubAPIUsage` to `server/features/analytics.go` and its `init()`.
- Add `TestGetGitHubAPIUsage_should_ReturnTrackingUnavailable_When_AnalyticsClientNil`,
  `…_should_ClampWindow_When_WindowExceedsNinety`,
  `…_should_ReturnPerResourceQuotas_When_SnapshotPopulated`,
  `…_should_ReturnEmptySummaryNotError_When_QueryFails`.
- Files: `server/features/analytics.go`, `server/services/github_usage_service_test.go`

---

#### Story 3.2.3: `UpdateGitHubUsageConfig` write RPC for the warn threshold
**As** Tyler, **I want** a real persistence path behind the panel's warn-threshold editor,
**so that** Surface B (`../design/ux.md` §3) has somewhere to actually save to — `GetGitHubAPIUsage`
(Story 3.2.1) is read-only, and this codebase has no generic config-update RPC for a UI form to
fall back on. **Depends on**: Story 3.3.1's `config.Config.GitHubRateLimitWarnPercent` field
(Task 3.3.1a) must land first — this handler mutates that field.

**Why this story exists**: `../design/ux.md` §3.2 step 4 and this plan's own prior draft of Story
4.3.2 both asserted the threshold editor "persists through the existing config-update RPC path
(the same one other settings surfaces use)." That path does not exist: `grep -n "^service \|rpc
Update" proto/session/v1/session.proto` lists `UpdateSession`, `UpdateClaudeConfig`,
`UpdateGlobalDefaults`, `UpdateProject`, `UpdateFeatureFlag`, `UpdateWorkflow` — one bespoke RPC
per settings surface, none generic — and `UnfinishedWorkService.UpdateUnfinishedWorkConfig`
(`server/services/unfinished_work_service.go:529-558`) is the same shape again. Every settings
surface in this codebase builds its own RPC; this story does the same for the warn threshold
instead of silently having no write path at all.

**Acceptance Criteria**:
- A dedicated RPC exists next to the read RPC and takes exactly the one field the UI edits.
  - *Given* `proto/session/v1/session.proto`, *When* it is inspected, *Then*
    `rpc UpdateGitHubUsageConfig(UpdateGitHubUsageConfigRequest) returns
    (UpdateGitHubUsageConfigResponse) {}` sits adjacent to `GetGitHubAPIUsage`, and
    `UpdateGitHubUsageConfigRequest` carries only `int32 warn_threshold_percent = 1;` — poll
    intervals, retention, and the probe interval stay config-file-only per `../design/ux.md` §4.1,
    which this RPC does not change.
- The handler clamps, persists via the existing `config.LoadConfig`/`config.SaveConfig` pattern,
  and returns the value actually saved.
  - *Given* a request with `warn_threshold_percent: 25`, *When* `UpdateGitHubUsageConfig` runs,
    *Then* it calls `cfg := config.LoadConfig()`, sets `cfg.GitHubRateLimitWarnPercent = 25`,
    calls `config.SaveConfig(cfg)`, and returns
    `UpdateGitHubUsageConfigResponse{WarnThresholdPercent: 25}` — the same load/mutate/save shape
    as `DefaultsService.UpdateGlobalDefaults` (`defaults_service.go:122-140`).
  - *Given* a request with `warn_threshold_percent: 150`, *When* the handler runs, *Then* the
    persisted and returned value is clamped to 90 (matching `SetWarnThresholdPercent`'s existing
    `[1, 90]` clamp from Story 1.2.3), and a `CodeInvalidArgument` is **not** returned — clamping,
    not rejection, matches this feature's house convention (`../design/ux.md` §4.1).
- The write does **not** call `github.SetWarnThresholdPercent` in-process.
  - *Given* a successful save, *When* the handler returns, *Then* the running process's live
    threshold (read by `github/rate_limit.go`'s `Update`) is unchanged until the next restart —
    matching Story 4.3.2's confirmation copy "Saved. Restart the service to apply." Applying the
    value live here would make that copy false and risk Tyler believing he throttled a warning
    he has not yet throttled (the exact failure `../design/ux.md` §3.2 step 5 calls out).
- A save failure surfaces as an error, not a false success.
  - *Given* `config.SaveConfig` returning an error (e.g. disk full), *When* the handler runs,
    *Then* it returns `connect.CodeInternal` and the in-memory config is not left partially
    mutated from the caller's perspective — mirroring `UpdateGlobalDefaults`'s own error path.
**Files**: `proto/session/v1/session.proto`, `server/services/github_usage_service.go`,
`server/services/github_usage_service_test.go`

##### Task 3.2.3a: Add the RPC and request/response messages (~3 min)
- In `proto/session/v1/session.proto`, add
  `rpc UpdateGitHubUsageConfig(UpdateGitHubUsageConfigRequest) returns
  (UpdateGitHubUsageConfigResponse) {}` immediately after `GetGitHubAPIUsage` (Task 3.2.1b), and
  `message UpdateGitHubUsageConfigRequest { int32 warn_threshold_percent = 1; }` /
  `message UpdateGitHubUsageConfigResponse { int32 warn_threshold_percent = 1; }`.
- Files: `proto/session/v1/session.proto`

##### Task 3.2.3b: Regenerate bindings (~2 min)
- Run `make proto-gen`; confirm `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/*_pb.ts` pick up the new RPC and are committed together (same
  convention as Task 3.2.1c).
- Files: `session/gen/session/v1/**`, `web-app/src/gen/session/v1/**`

##### Task 3.2.3c: Implement the handler (~4 min)
- Add `func (s *SessionService) UpdateGitHubUsageConfig(ctx, req) (*connect.Response[...],
  error)` to `server/services/github_usage_service.go`: `cfg := config.LoadConfig()`, clamp
  `req.Msg.WarnThresholdPercent` to `[1, 90]` (reuse the clamp helper from Task 1.2.3a if
  exported, otherwise a local equivalent), set `cfg.GitHubRateLimitWarnPercent = clamped`, call
  `config.SaveConfig(cfg)` and return `connect.CodeInternal` on failure, else return the clamped
  value. Do not call `github.SetWarnThresholdPercent` here (see acceptance criteria).
- Files: `server/services/github_usage_service.go`

##### Task 3.2.3d: Handler tests (~4 min)
- Add `TestUpdateGitHubUsageConfig_should_PersistClampedValue_When_ThresholdInRange`,
  `…_should_ClampNotReject_When_ThresholdOutOfRange`,
  `…_should_ReturnInternal_When_SaveConfigFails`, and
  `…_should_NotApplyLiveThreshold_When_Saved` (asserts `github.DefaultRateLimiter`'s in-process
  threshold is unchanged after the call).
- Files: `server/services/github_usage_service_test.go`

---

### Epic 3.3: Configuration surface

#### Story 3.3.1: Config-driven poll intervals and thresholds
**As** Tyler, **I want** to change poll intervals, the warn threshold, retention, and the probe
interval by editing `config.json`, **so that** I can throttle a noisy poller without rebuilding —
accepting that a service restart applies the change.
**Acceptance Criteria**:
- Defaults reproduce today's behaviour byte-for-byte.
  - *Given* a `config.json` with none of the five new keys, *When* the service starts,
    *Then* `PRStatusPollerConfig.PollInterval` and `WorktreePRPollerConfig.PollInterval` are both
    `60 * time.Second` (matching `session/pr_status_poller.go:41` and
    `session/worktree_pr_poller.go:49` today), the warn threshold is 10%, retention is 30 days,
    and the probe interval is 300s.
- A configured interval is honoured at construction.
  - *Given* `{"pr_status_poll_interval_seconds": 300}`, *When* the service restarts,
    *Then* the startup log line `"PR status poller started"`
    (`session/pr_status_poller.go:161`) reports `interval=5m0s`.
- Invalid values are clamped with a WARN, never applied raw.
  - *Given* `{"pr_status_poll_interval_seconds": 1}`, *When* the service starts, *Then* the
    interval is clamped to the 10s floor and one WARN names the clamp — a 1-second poll would
    itself cause the exhaustion this feature exists to prevent.
- The restart requirement is stated where the user sets the value.
  - *Given* the panel's warn-threshold editor, *When* the user saves a new value, *Then* the
    confirmation text reads "Saved — restart the service to apply", not "Applied", because no
    hot-reload exists in `config/` (verified by all three research agents).
**Files**: `config/config.go`, `config/config_test.go`, `server/dependencies.go`

##### Task 3.3.1a: Add the five config fields and accessors (~5 min)
- Add to `config.Config`: `GitHubUsageRetentionDays`, `GitHubRateLimitWarnPercent`,
  `PRStatusPollIntervalSeconds`, `WorktreePRPollIntervalSeconds`,
  `GitHubRateLimitProbeIntervalSeconds` (all `int`, all `omitempty`), each with an
  `…OrDefault()` accessor that is nil-receiver-safe and clamps
  (intervals `[10s, 1h]`, threshold `[1, 90]`, retention `[0, 365]`, probe `[60s, 6h]`)
  following the `MaxConcurrentBacklogWorkItemsOrDefault` precedent at `config/config.go:618`.
- Files: `config/config.go`

##### Task 3.3.1b: Wire the accessors into construction (~4 min)
- At `server/dependencies.go:357`, replace `session.NewPRStatusPoller(core.Storage)` with
  `session.NewPRStatusPollerWithConfig(core.Storage, cfg)` built from
  `PRStatusPollIntervalOrDefault()`; do the same for the `NewWorktreePRPoller` call at `:935`.
- Call `github.SetWarnThresholdPercent(cfg.GitHubRateLimitWarnPercentOrDefault())` during startup
  in `server/server.go`, before the first request can fire.
- Files: `server/dependencies.go`, `server/server.go`

##### Task 3.3.1c: Config default/clamp tests (~4 min)
- Add `TestPRStatusPollIntervalOrDefault_should_Return60s_When_Unset`,
  `…_should_ClampToFloor_When_BelowTenSeconds`,
  `TestGitHubRateLimitWarnPercentOrDefault_should_ClampToNinety_When_Excessive`,
  `TestGitHubUsageRetentionDaysOrDefault_should_ReturnZero_When_ExplicitlyDisabled`.
- Files: `config/config_test.go`

---

## Phase 4: Web UI

### Epic 4.1: Data hook, route, and navigation

#### Story 4.1.1: `useGitHubApiUsage` hook
**As a** panel component, **I want** a hook mirroring `useApprovalAnalytics`, **so that** the
fetch/loading/error/refresh contract is identical to the panel this one is modelled on.
**Acceptance Criteria**:
- The hook's return shape and refetch-on-window-change behaviour match the existing precedent.
  - *Given* the panel renders with `windowDays = 7` and the user clicks the `30` button,
    *When* `windowDays` changes, *Then* exactly one new `getGitHubAPIUsage` call is issued with
    `windowDays: 30` — the same `useCallback`-keyed effect pattern as
    `web-app/src/lib/hooks/useApprovalAnalytics.ts`.
- Errors are surfaced without discarding the last good data.
  - *Given* a successful load followed by a failing refresh, *When* the error arrives,
    *Then* `error` is set **and** the previously-loaded `data` is still returned, so the panel can
    render the stale-content-plus-error-banner pattern the house style uses.
- No client-side polling is introduced.
  - *Given* the panel mounted for 10 minutes with no interaction, *When* network activity is
    inspected, *Then* exactly one request was made — refresh is manual or selector-triggered,
    matching the established convention.
**Files**: `web-app/src/lib/hooks/useGitHubApiUsage.ts`,
`web-app/src/lib/hooks/__tests__/useGitHubApiUsage.test.ts`

##### Task 4.1.1a: Implement the hook (~4 min)
- Copy the structure of `useApprovalAnalytics.ts` verbatim (client ref in an effect,
  `getConnectTransport()`, `create(...RequestSchema, { windowDays })`), returning
  `{ data, loading, error, refresh }` and preserving previous data on error.
- Files: `web-app/src/lib/hooks/useGitHubApiUsage.ts`

##### Task 4.1.1b: Hook tests (~4 min)
- Add `useGitHubApiUsage_should_RefetchOnce_When_WindowDaysChanges`,
  `…_should_PreservePreviousData_When_RefreshFails`,
  `…_should_NotPoll_When_Idle` (fake timers, assert single call).
- Files: `web-app/src/lib/hooks/__tests__/useGitHubApiUsage.test.ts`

---

#### Story 4.1.2: Route and navigation entry
**As** Tyler, **I want** the panel at a stable URL reachable from the nav, **so that** I can jump
straight to it the moment I see a rate-limit WARN.
**Acceptance Criteria**:
- The route is registered centrally and grouped with the other diagnostics.
  - *Given* `routes.githubApiUsage = "/analytics/github-api"` added to
    `web-app/src/lib/routes.ts` and a `NAV_PAGES` entry with `group: "insights"`,
    `headerNav: false`, *When* the drawer/More sheet renders, *Then* "GitHub API Usage" appears
    under Insights alongside "Escape Analytics" (`web-app/src/lib/nav-pages.ts:67`), not in the
    always-visible header row.
- The page is a thin shell.
  - *Given* `web-app/src/app/analytics/github-api/page.tsx`, *When* it renders,
    *Then* it mounts `<GitHubApiUsagePanel />` and nothing else, matching how
    `web-app/src/app/rules/page.tsx` mounts `<ApprovalAnalyticsPanel />`.
**Files**: `web-app/src/lib/routes.ts`, `web-app/src/lib/nav-pages.ts`,
`web-app/src/app/analytics/github-api/page.tsx`

##### Task 4.1.2a: Add route + nav entry (~3 min)
- Add `githubApiUsage: "/analytics/github-api"` to `routes.ts` and the `NAV_PAGES` row with the
  lucide `Gauge` icon (import it alongside the existing icons).
- Files: `web-app/src/lib/routes.ts`, `web-app/src/lib/nav-pages.ts`

##### Task 4.1.2b: Add the page shell (~2 min)
- Create `web-app/src/app/analytics/github-api/page.tsx` with `"use client"` and a
  `// +feature: github-api-usage` marker in the first 10 lines.
- Files: `web-app/src/app/analytics/github-api/page.tsx`

---

### Epic 4.2: The usage panel

#### Story 4.2.1: Panel shell, per-resource quota tiles, and gauges
**As** Tyler opening the panel after a WARN, **I want** the current per-resource quota visible
with zero clicks, **so that** "am I about to hit a wall" is answered before I scroll or select
anything.
**Acceptance Criteria**:
- One independently-scaled tile+gauge per observed resource; never a blended number.
  - *Given* a response with `core {remaining: 4200, limit: 5000}` and
    `search {remaining: 3, limit: 30}`, *When* the panel renders, *Then* two separate tiles show
    "4,200 / 5,000" and "3 / 30" with independent gauge fills — and no element anywhere displays a
    combined percentage across resources.
- Colour is never the only signal.
  - *Given* the search tile in its critical tier, *When* it renders, *Then* the visible text reads
    "3 / 30 remaining (10.0%) · resets in 12m" alongside the `vars.color.error` fill, and the
    `barFill` div itself carries `aria-hidden="true"` — the exact `Bar` convention in
    `ApprovalAnalyticsPanel.tsx`.
- Low-limit resources show absolute counts, not only percentages.
  - *Given* the search resource at the 10% threshold, *When* the tile renders, *Then* the
    threshold annotation reads "warn below 3 of 30", because a percentage alone understates how
    few requests a 30/hr bucket has left (`../research/ux.md` §6).
- Burn-rate context accompanies the gauge.
  - *Given* `core` at 4,200 remaining with a 340/hr burn rate and a reset 50 minutes out,
    *When* the tile renders, *Then* the sub-line reads "≈12h to exhaustion · resets in 50m",
    making it obvious that the reset arrives first.
**Files**: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`,
`web-app/src/components/sessions/GitHubApiUsagePanel.css.ts`

##### Task 4.2.1a: Panel shell and CSS module (~5 min)
- Create `GitHubApiUsagePanel.css.ts` reusing the structural primitives from
  `ApprovalAnalyticsPanel.css.ts` (`panel`, `titleRow`, `cards`, `card`, `cardValue`,
  `cardLabel`, `cardSub`, `tableSection`, `sectionTitle`, `tableWrapper`, `barTrack`, `barFill`,
  `windowSelector`, `windowBtn`, `windowBtnActive`, `empty`, `emptyHint`) with three tier modifier
  classes bound to `vars.color.success|warning|error` + their `*Bg`/`*Text` pairs. No hardcoded
  hex, no `zIndex` (`.claude/rules/css-architecture.md`).
- Files: `web-app/src/components/sessions/GitHubApiUsagePanel.css.ts`

##### Task 4.2.1b: Quota tiles + gauge sub-component (~5 min)
- Create `GitHubApiUsagePanel.tsx` with a `QuotaTile` sub-component rendering one
  `GitHubQuotaProto` — value, label, gauge (`aria-hidden` fill), threshold annotation, burn-rate
  and reset sub-line, and a `data-testid={`quota-tile-${resource}`}` (the one place `../research/ux.md`
  §1 recommends a testid, since resource labels repeat in a grid).
- Files: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`

##### Task 4.2.1c: Tile rendering tests (~4 min)
- Add `GitHubApiUsagePanel_should_RenderOneTilePerResource_When_MultipleResourcesObserved`,
  `…_should_ShowAbsoluteCountsAlongsidePercent_When_ResourceLimitIsSmall`,
  `…_should_NeverRenderBlendedQuota_When_MultipleResourcesPresent` (asserts no element matches a
  combined total).
- Files: `web-app/src/components/sessions/__tests__/GitHubApiUsagePanel.test.tsx`

---

#### Story 4.2.2: Volume-over-time and source-breakdown sections
**As** Tyler diagnosing a burst, **I want** volume over time and a ranked source table, **so that**
I can tell whether today is abnormal and which poller to turn down.
**Acceptance Criteria**:
- Volume is a bar-per-bucket table row, with the numeric value in a cell.
  - *Given* 7 daily buckets with totals `[120, 96, 340, 88, 91, 87, 40]`, *When* the section
    renders, *Then* there are 7 `<tr>` rows each containing the date, the numeric total, and an
    `aria-hidden` bar scaled to the section-local max (340) — never a bare SVG requiring a
    separate text alternative.
- The source table separates exact from approximate counts, with the explanation visible
  without hovering.
  - *Given* `pr_status_poller` with 40 exact and `gh_cli.merge_pr` with 3 approximate,
    *When* the table renders, *Then* the `gh_cli.merge_pr` count cell reads "≈3", a `title`
    attribute supplies the same explanation for mouse users, **and** a persistent, visible
    footnote below the table (rendered whenever ≥1 row uses "≈") reads "≈ = approximate (gh CLI
    invocation count — one invocation may issue more than one API request)" — the `title`
    attribute alone is not sufficient, since it isn't reliably exposed to screen readers or
    touch input (`../design/ux.md` §2.1/§2.2, mirroring this doc's own rejection of
    hover-/tooltip-only affordances for the threshold editor's validation error in Story 4.3.2).
    The two figures are never summed into an unqualified total.
- Sources that could contribute but haven't appear as zero rows.
  - *Given* `worktree_pr_poller` with no events in the window, *When* the table renders,
    *Then* a `worktree_pr_poller — 0` row is present, so a silently-broken poller is
    distinguishable from a correctly-quiet one (the `UnifiedActivityTable` precedent).
- Per-resource scaling is local to each section.
  - *Given* core (thousands) and search (tens) in the same window, *When* the volume section
    renders per resource, *Then* each resource's bars scale to its own max, so search's trend is
    not flattened to a zero line next to core's.
**Files**: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`,
`web-app/src/components/sessions/GitHubApiUsagePanel.css.ts`

##### Task 4.2.2a: Volume-over-time section (~5 min)
- Add a `tableSection` rendering `volumeBuckets` as rows with a section-local `max` computed in
  the component (the `maxDayTotal` convention), plus a per-resource toggle in the section header
  reusing the `windowSelector` button-group classes.
- Files: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`

##### Task 4.2.2b: Source-breakdown section (~5 min)
- Add a `tableSection` rendering `sources` sorted by `count + approxCount` descending, with
  columns: source, count (with "≈" prefix when `approxCount > 0`), share %, and an `aria-hidden`
  bar; render zero rows for sources absent from the response. Add a persistent, visible footnote
  below the table (not tooltip-only — see Story 4.2.2's acceptance criteria) reading "≈ =
  approximate (gh CLI invocation count — one invocation may issue more than one API request)",
  rendered whenever at least one row has `approxCount > 0`.
- Files: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`

##### Task 4.2.2c: Section tests (~4 min)
- Add `GitHubApiUsagePanel_should_ScaleBarsPerSectionLocalMax_When_ResourcesDifferInMagnitude`,
  `…_should_MarkApproximateCounts_When_SourceIsGhCli`,
  `…_should_RenderZeroRow_When_KnownSourceHasNoEvents`,
  `…_should_ShowVisibleApproxFootnote_When_ApproximateCountsPresent` (asserts the footnote text
  is present in the rendered DOM, not only in a `title` attribute).
- Files: `web-app/src/components/sessions/__tests__/GitHubApiUsagePanel.test.tsx`

---

#### Story 4.2.3: Time-window selector and refresh control
**As** Tyler, **I want** the same window selector and refresh affordance the other analytics panel
has, **so that** the interaction is muscle memory and keyboard-accessible without new widget code.
**Acceptance Criteria**:
- The selector is a `role="group"` of native buttons with `aria-pressed`.
  - *Given* the panel with `windowDays = 7`, *When* a screen reader focuses the "30" button,
    *Then* it announces an unpressed button inside a group labelled "Time window"; after
    activation with Enter, `aria-pressed="true"` moves to it — no custom listbox, no arrow-key
    handling to hand-roll.
- Options match the existing panel.
  - *Given* the rendered selector, *When* its buttons are enumerated, *Then* they are
    `[1, 7, 14, 30, 90]` days — the `ApprovalAnalyticsPanel` set plus `1` for the hourly-bucket
    burst view.
- Refresh is an icon button with an accessible name and a disabled loading state.
  - *Given* a refresh in flight, *When* the button is inspected, *Then* it is `disabled` and its
    `aria-label` is "Refresh GitHub API usage".
**Files**: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`

##### Task 4.2.3a: Selector + refresh header row (~4 min)
- Add the `titleRow` with title, `windowSelector` group, and refresh button, wired to the hook's
  `windowDays` state and `refresh()`.
- Files: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`

##### Task 4.2.3b: Selector tests (~3 min)
- Add `GitHubApiUsagePanel_should_ToggleAriaPressed_When_WindowChanged` and
  `…_should_DisableRefresh_When_Loading`.
- Files: `web-app/src/components/sessions/__tests__/GitHubApiUsagePanel.test.tsx`

---

### Epic 4.3: Honest states and the threshold editor

#### Story 4.3.1: Five distinct empty/stale/unavailable/failed states
**As** Tyler, **I want** "never observed", "no data in window", "stale reading", "tracking
unavailable", and "fetch failed" to look different, **so that** I never mistake a broken tracker
for a quiet quota, and never mistake a failed request for a successful one reporting bad news.
**Acceptance Criteria**:
- Never-observed quota renders "—", not a fabricated 100%.
  - *Given* a fresh install where no GitHub request has been made, *When* the panel renders,
    *Then* each quota tile shows "—" with the sub-line "not yet observed" — never
    "5,000 / 5,000 (100%)".
- Empty history gets the two-line hint treatment.
  - *Given* `volumeBuckets` empty for a 7-day window, *When* the section renders, *Then* it shows
    "No GitHub API activity recorded in the last 7 days." plus the hint "Recorded automatically as
    pollers and RPCs make GitHub API calls — check back after your first poll cycle."
- Stale readings are annotated, not silently presented as current.
  - *Given* `observed_at` 47 minutes ago, *When* the tile renders, *Then* it appends "as of 47m
    ago" and drops to a muted treatment, so a stale "4,800 / 5,000" cannot be trusted as live.
- Tracking-unavailable is visually distinct from zero activity.
  - *Given* `tracking_available: false`, *When* the panel renders, *Then* a `role="alert"` banner
    reads "Usage tracking is unavailable — the analytics database could not be opened. Counts
    below are not reliable." and the numeric sections render disabled, **not** as zeros.
- A failed fetch is visually distinct from a successful response reporting `tracking_available:
  false` — one means "the request itself failed," the other means "the request succeeded and is
  reporting a known backend limitation."
  - *Given* `useGitHubApiUsage`'s `error` field is set (Story 4.1.1 — timeout, 5xx, or malformed
    payload), *When* the panel renders, *Then* a `role="alert"` banner reads "Failed to load
    GitHub API usage: {error message}." with an inline Retry button that calls the hook's
    `refresh()`; *and Given* prior data exists from an earlier successful load, *Then* that data
    remains visible beneath the banner rather than being cleared (`../design/ux.md` §2.3).
- Dropped events qualify the totals.
  - *Given* `dropped_events: 6`, *When* the panel renders, *Then* a visible note reads
    "6 events were dropped (buffer full) — totals below are a lower bound."
- Exhaustion events are a headline trust signal, not buried in a table — this is the number the
  Success Metric is checked against, so the panel must make it checkable at a glance
  (adversarial-review.md Blocker 3).
  - *Given* `exhaustion_events: 0` for the selected window, *When* the panel renders, *Then* a
    dedicated stat reads "0 rate-limit exhaustions in the last N days" in a neutral/success
    treatment, placed near the quota tiles rather than requiring the user to open the source
    table.
  - *Given* `exhaustion_events: 2`, *When* the panel renders, *Then* the same stat reads
    "2 rate-limit exhaustions in the last N days" in the critical tier, so a nonzero reading is
    unmissable rather than only discoverable via the raw JSON response.
- A "polling paused" stat sits next to the exhaustion-events stat, always visible, so a clean
  exhaustion count achieved by silent pausing is never mistaken for "problem solved"
  (pre-mortem.md P1 #3) — the two failure modes described in Story 1.2.4's rationale ("no
  problem" vs. "pollers went quiet") must be visually distinguishable at a glance, not only
  discoverable by cross-referencing the raw response or `journalctl`.
  - *Given* `pause_events: 0`, `total_paused_seconds: 0`, *When* the panel renders, *Then* a
    `data-testid="polling-paused-stat"` element reads "0 polling pauses in the last N days" in a
    neutral/success tier, next to the exhaustion stat.
  - *Given* `pause_events: 3`, `total_paused_seconds: 990`, *When* the panel renders, *Then* the
    same stat reads "3 polling pauses (≈16m total) in the last N days" in the `warning` tier
    (pausing is expected occasional behaviour, not itself critical the way an exhaustion is —
    hence `warning`, not `error`, distinguishing it from the exhaustion stat's critical tier at a
    nonzero reading).
**Files**: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`,
`web-app/src/components/sessions/GitHubApiUsagePanel.css.ts`

##### Task 4.3.1a: Implement the five states (~5 min)
- Add explicit branches for `neverObserved`, `emptyWindow`, `stale` (observed_at older than
  3× the probe interval), `!tracking_available`, and `fetchFailed` (the hook's `error` field,
  Story 4.1.1), using the `empty`/`emptyHint` and `role="alert"`+Retry-button conventions already
  present in `ApprovalAnalyticsPanel` (its existing error banner is the precedent for
  `fetchFailed` — no new pattern).
- Add the exhaustion-events stat described in Story 4.3.1's acceptance criteria: a
  `data-testid="exhaustion-events-stat"` element near the quota tiles reading "{n} rate-limit
  exhaustions in the last {windowDays} days", tier-coloured (`vars.color.success` at 0,
  `vars.color.error`/critical otherwise) via the same 3-tier token set used elsewhere in this
  panel (`.claude/rules/css-architecture.md`) — not a new ad hoc color.
- Files: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`,
  `web-app/src/components/sessions/GitHubApiUsagePanel.css.ts`

##### Task 4.3.1b: Add the polling-paused stat (~4 min)
- Next to the `exhaustion-events-stat` element, add a `data-testid="polling-paused-stat"` element
  reading "{pauseEvents} polling pauses in the last {windowDays} days" when `pauseEvents === 0`
  (`vars.color.success` tier), or "{pauseEvents} polling pauses (≈{humanized
  totalPausedSeconds} total) in the last {windowDays} days" in `vars.color.warning` tier
  otherwise — reusing the existing 3-tier token set, no new color (`.claude/rules/
  css-architecture.md`). Humanize `total_paused_seconds` with a minute-granularity formatter
  consistent with the existing "≈12h to exhaustion" burn-rate sub-line style (Story 4.2.1).
- Files: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`,
  `web-app/src/components/sessions/GitHubApiUsagePanel.css.ts`

##### Task 4.3.1c: State tests (~5 min)
- Add `GitHubApiUsagePanel_should_ShowDashNotHundredPercent_When_QuotaNeverObserved`,
  `…_should_ShowDistinctBanner_When_TrackingUnavailable`,
  `…_should_AnnotateStaleness_When_ObservedAtIsOld`,
  `…_should_QualifyTotals_When_EventsWereDropped`,
  `…_should_ShowEmptyHint_When_WindowHasNoData`,
  `…_should_ShowExhaustionEventCount_When_WindowHasExhaustions`,
  `…_should_ShowZeroExhaustionsInSuccessTier_When_NoneOccurred`,
  `…_should_ShowZeroPausesInSuccessTier_When_NoPausesOccurred`,
  `…_should_ShowPauseCountInWarningTier_When_PausesOccurred`, and
  `…_should_ShowRetryableErrorBanner_When_FetchFails` (asserts the `role="alert"` banner, Retry
  button, and — when prior data exists — that it remains visible beneath the banner).
- Files: `web-app/src/components/sessions/__tests__/GitHubApiUsagePanel.test.tsx`

---

#### Story 4.3.2: Warn-threshold editor with honest apply semantics
**As** Tyler, **I want** to edit the warn threshold in the panel and be told plainly that it
applies on restart, **so that** I never believe I've throttled something I haven't.
**Depends on**: Story 3.2.3's `UpdateGitHubUsageConfig` RPC. `../design/ux.md` §3.2 step 4
currently reads "persists through the existing config-update RPC path (the same one other
settings surfaces use — no new persistence mechanism for this one field)" — that RPC does not
exist (see Story 3.2.3's rationale); this plan.md has been corrected accordingly, but
`../design/ux.md` itself still has the stale wording and needs its own correction pass by a
human/future session, since this plan-repair pass is scoped to plan.md and pre-mortem.md only.
**Acceptance Criteria**:
- A labelled, validated numeric input — not placeholder-only.
  - *Given* the editor, *When* inspected, *Then* it has a visible `<label>` reading "Warn when
    remaining drops below (% of limit)" associated by `htmlFor`/`id`.
- Out-of-range input surfaces a text error near the field.
  - *Given* the user types `150` and blurs, *When* validation runs, *Then* a `role="alert"` text
    node reads "Enter a value between 1 and 90" and the save button is disabled — not a
    browser-native validation bubble.
- The confirmation states the restart requirement explicitly.
  - *Given* a successful save of `25`, *When* the confirmation renders, *Then* it reads
    "Saved. Restart the service to apply." — never "Applied" or "Updated", because
    `config/` has no hot-reload.
- The value persists through the new `UpdateGitHubUsageConfig` RPC (Story 3.2.3).
  - *Given* a save of `25`, *When* the RPC call resolves, *Then* it was
    `updateGitHubUsageConfig({ warnThresholdPercent: 25 })` against the generated client (no
    generic config-update RPC exists to reuse — see Story 3.2.3); *and Given* the service is then
    restarted, *When* it starts, *Then* `github.SetWarnThresholdPercent(25)` runs at startup
    (Task 3.3.1b) and the low-quota WARN fires at 25%.
**Files**: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`,
`web-app/src/components/sessions/GitHubApiUsagePanel.css.ts`

##### Task 4.3.2a: Threshold editor UI + validation (~5 min)
- Add the labelled input, clamp/validate on change, disable save while invalid, and render the
  restart-required confirmation copy. Persist through the new `UpdateGitHubUsageConfig` RPC
  (Story 3.2.3, Task 3.2.3a) — this is the feature's own dedicated write path, not a shared one.
- Files: `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`

##### Task 4.3.2b: Editor tests (~4 min)
- Add `GitHubApiUsagePanel_should_ShowRoleAlertError_When_ThresholdOutOfRange`,
  `…_should_SayRestartRequired_When_ThresholdSaved`,
  `…_should_HaveVisibleLabel_When_ThresholdEditorRendered`.
- Files: `web-app/src/components/sessions/__tests__/GitHubApiUsagePanel.test.tsx`

---

## Phase 5: Trustworthiness and Ship Gate

### Epic 5.1: Reconciliation against ground truth

#### Story 5.1.1: `/rate_limit` probe and the unaccounted-requests residual
**As** Tyler, **I want** the tracker to tell me how much consumption it *couldn't* attribute,
**so that** "zero exhaustions in 14 days" is a claim I can trust rather than one that depends on
the tracker being silently complete.
**Acceptance Criteria**:
- A periodic probe refreshes the whole snapshot from GitHub's authoritative response body.
  - *Given* a probe interval of 300s and a `/rate_limit` response body listing `core`, `search`,
    and `graphql`, *When* the probe runs, *Then* `Snapshot()` contains all three entries with
    fresh `ObservedAt`, including resources this process has never requested — which is what makes
    the search tile show real numbers before the first search call.
- The probe is recorded but marked as not charging quota.
  - *Given* one probe execution, *When* the event is stored, *Then* it has
    `CallSite: "rate_limit_probe"` and `QuotaCharged: false`, so probes never inflate the
    "requests that cost quota" figure.
- The residual is computed per reset window and surfaced, always naming multi-instance usage as a
  possible cause in the copy itself (pre-mortem.md P1 #2) — no dynamic detection of *how many*
  other instances are running is built (no cross-instance registry exists in this codebase today;
  confirmed by `grep -rln STAPLER_SQUAD_INSTANCE --include="*.go"` returning only single-instance
  consumers, no shared instance list), so the static wording is the only signal, by design.
  - *Given* a core probe reporting `limit: 5000, remaining: 4600` (400 consumed this window) while
    the store holds 360 quota-charged core events since the window start, *When* reconciliation
    runs, *Then* `unaccounted_requests[core] == 40` and it is rendered in the panel as
    "40 requests this window were not attributed (gh CLI, another instance, or another tool)" —
    the "another instance" clause is load-bearing, not incidental, given Tyler's documented
    multi-instance manual-testing workflow (`CLAUDE.md`'s `STAPLER_SQUAD_INSTANCE` pattern) makes
    a nonzero residual routine rather than exceptional.
- A large residual escalates to a WARN.
  - *Given* a residual exceeding 20% of consumed quota, *When* reconciliation runs, *Then*
    `WARN "github usage: reconciliation residual exceeds tolerance"` is logged with
    `probe_used`, `tracked`, `unaccounted`, `resource`.
**Files**: `github/rate_limit_probe.go`, `github/rate_limit_probe_test.go`,
`server/analytics/github_usage_query.go`, `server/server.go`

##### Task 5.1.1a: Verify the probe's own cost, then implement it (~5 min)
- **First** resolve Unresolved Question #2 empirically: call `GET /rate_limit` twice against the
  live API and confirm `core.remaining` is unchanged; record the result as a doc comment. If it
  *does* consume quota, raise the default probe interval and set `QuotaCharged: true`.
- Create `github/rate_limit_probe.go` with
  `func FetchRateLimitSnapshot(ctx context.Context) (QuotaSnapshot, error)` issuing
  `newGHRequest(ctx, "rate_limit")` (so it flows through `usageTransport`), decoding the
  `resources` object, and publishing the full snapshot via a new
  `DefaultRateLimiter.PublishSnapshot(QuotaSnapshot)`.
- Files: `github/rate_limit_probe.go`, `github/rate_limit.go`

##### Task 5.1.1b: Start the probe loop (~3 min)
- In `server/server.go`, start a goroutine ticking at
  `cfg.GitHubRateLimitProbeIntervalOrDefault()` calling `FetchRateLimitSnapshot` with
  `WithCallSite(ctx, CallSiteRateLimitProbe)`, exiting on `serverCtx`; run once immediately at
  startup so the gauge is populated before the first user visit.
- Files: `server/server.go`

##### Task 5.1.1c: Reconciliation computation + tests (~5 min)
- Add `ComputeReconciliation(snapshot github.QuotaSnapshot, rows []UsageRow) map[github.GitHubResource]int`
  to `github_usage_query.go`, wire it into the handler's `unaccounted_requests` field, and add
  `TestFetchRateLimitSnapshot_should_PopulateAllResources_When_BodyListsThem`,
  `TestFetchRateLimitSnapshot_should_MarkQuotaNotCharged`,
  `TestComputeReconciliation_should_ReportResidual_When_TrackedIsLessThanConsumed`,
  `…_should_ReportZero_When_TrackedMatchesConsumed`.
- Files: `server/analytics/github_usage_query.go`,
  `server/analytics/github_usage_query_test.go`, `github/rate_limit_probe_test.go`

---

#### Story 5.1.2: `gh`-vs-native token identity check
**As** Tyler, **I want** to be told if `gh` and the native client are using different tokens,
**so that** I don't read a healthy gauge while `gh` quietly exhausts a different quota.
**Acceptance Criteria**:
- A startup check compares the two identities and logs a WARN on mismatch.
  - *Given* `gh api user --jq .login` returning `tstapler` and
    `github.GetCurrentUserLogin(ctx)` returning `TylerStaplerAtFanatics`, *When* the check runs at
    startup, *Then* `WARN "github usage: gh CLI token identity differs from native client"` is
    logged with both logins.
- The mismatch is visible in the panel, not just the log.
  - *Given* a detected mismatch, *When* the panel renders, *Then* a banner reads "`gh` CLI is
    authenticated as a different user — quota figures below describe the native client's token
    only."
- The check is cheap and non-fatal.
  - *Given* `gh` is not installed at all, *When* the check runs, *Then* it logs INFO and returns
    without error — the app never fails to start over a diagnostic.
- The check runs once, not per request.
  - *Given* a 30-minute uptime, *When* the identity check is counted, *Then* it ran exactly once,
    and its native half was attributed to `CallSiteAuthCheck`.
**Files**: `github/token_identity.go`, `github/token_identity_test.go`, `server/server.go`,
`web-app/src/components/sessions/GitHubApiUsagePanel.tsx`

##### Task 5.1.2a: Implement the identity check (~4 min)
- Create `github/token_identity.go` with
  `func CheckTokenIdentityParity(ctx context.Context) (ghLogin, nativeLogin string, matched bool)`
  using `runGHCommand(ctx, CallSiteGHTokenIdentity, "api", "user", "--jq", ".login")` and
  `GetCurrentUserLogin(ctx)`; return gracefully when `gh` is absent.
- Files: `github/token_identity.go`

##### Task 5.1.2b: Wire it at startup and expose it in the response (~3 min)
- Call it once from `server/server.go` startup, store the result, and add a
  `token_identity_mismatch` bool + both logins to `GetGitHubAPIUsageResponse`; render the banner.
- Files: `server/server.go`, `proto/session/v1/session.proto`,
  `server/services/github_usage_service.go`,
  `web-app/src/components/sessions/GitHubApiUsagePanel.tsx`

##### Task 5.1.2c: Identity tests (~3 min)
- Add `TestCheckTokenIdentityParity_should_ReportMismatch_When_LoginsDiffer`,
  `…_should_DegradeGracefully_When_GhBinaryMissing`.
- Files: `github/token_identity_test.go`

---

### Epic 5.2: Registry, e2e, docs, and the ship gate

#### Story 5.2.1: Feature registry entries
**As a** maintainer, **I want** the new RPC and UI feature registered, **so that** the coverage
report reflects reality and CI's registry check passes.
**Acceptance Criteria**:
- The backend and frontend entries exist and regeneration is idempotent.
  - *Given* the `// +api: github:usage` marker on the handler and the
    `// +feature: github-api-usage` marker in the page's first 10 lines, *When*
    `make registry-generate` runs twice, *Then* the second run produces no diff.
- Coverage gaps do not grow.
  - *Given* `docs/registry/coverage-gaps.json` before the change, *When* it is regenerated after
    the e2e test lands, *Then* the untested-feature count has not increased.
**Files**: `docs/registry/features/backend/analytics/…json`,
`docs/registry/features/frontend/…json`

##### Task 5.2.1a: Add per-feature registry files and regenerate (~3 min)
- Create the backend entry (`id: "github:usage"`, `markerFound: true`) and the frontend entry
  (`id: "github-api-usage"`, `filePath: "web-app/src/components/sessions/GitHubApiUsagePanel.tsx"`),
  then run `make registry-generate` and commit the changed files.
- Files: `docs/registry/features/backend/analytics/`, `docs/registry/features/frontend/`

---

#### Story 5.2.2: Playwright e2e coverage
**As** CI, **I want** an e2e spec exercising the panel's real states, **so that** a regression in
the empty/unavailable distinction is caught before it misleads a real diagnosis.
**Acceptance Criteria**:
- The spec follows all four enforced conventions.
  - *Given* `tests/e2e/github-api-usage.spec.ts`, *When* the convention linter runs, *Then* it
    passes: a `// @feature github:usage` header on line 1, zero `waitForTimeout` calls, only
    `getByRole`/`getByTestId` locators, and any reusable navigation extracted into
    `tests/e2e/pages/`.
- Empty state is asserted on the isolated test server.
  - *Given* the auto-managed isolated instance (which has made no GitHub calls), *When* the spec
    navigates to `/analytics/github-api`, *Then* the "No GitHub API activity recorded" empty text
    and the "—" quota placeholders are both visible, and no "100%" appears anywhere.
- The window selector is asserted through ARIA, not CSS.
  - *Given* the selector, *When* the spec clicks the "30" button by role and name,
    *Then* `aria-pressed="true"` moves to it and the section heading updates to the 30-day window.
**Files**: `tests/e2e/github-api-usage.spec.ts`, `tests/e2e/pages/GitHubApiUsagePage.ts`

##### Task 5.2.2a: Page helper + spec (~5 min)
- Add the page helper with `goto()`, `quotaTile(resource)`, `windowButton(days)`, and
  `emptyState()`; write the spec with the three assertions above.
- Files: `tests/e2e/github-api-usage.spec.ts`, `tests/e2e/pages/GitHubApiUsagePage.ts`

---

#### Story 5.2.3: Documentation and the Success-Metric verification procedure
**As** future-Tyler starting the 14-day trial, **I want** a written procedure for verifying the
Success Metric from the tracking data, **so that** the metric is checkable rather than
aspirational.
**Acceptance Criteria**:
- A reference doc exists and is indexed.
  - *Given* `.claude/docs/github-api-usage-tracking.md`, *When* `CLAUDE.md`'s Reference Documents
    Index is read, *Then* it contains a row pointing at that file.
- The doc states the verification procedure concretely.
  - *Given* the doc, *When* the "Verifying the Success Metric" section is read, *Then* it
    specifies: open `/analytics/github-api` with the 14-day window; the metric passes only if
    `exhaustion_events == 0` **and** `dropped_events == 0` **and** the reconciliation residual
    stayed under 20% for the whole window **and** `total_paused_duration` (Story 1.2.4's pause
    stats, surfaced per Story 3.2.2/4.3.1) is not concerningly high — because a zero-exhaustion
    reading from an incomplete dataset, or one achieved by the pause escape hatch silently
    starving pollers instead of a genuine fix, both prove nothing (pre-mortem.md P1 #3).
- The restart-required and approximation caveats are documented where a reader will hit them.
  - *Given* the doc, *When* the "Caveats" section is read, *Then* it states that poll-interval and
    threshold changes require a service restart, that `gh`-CLI rows are one-invocation
    approximations, that `gh`-CLI rate-limit exhaustion detection (Task 1.3.2d) is a stderr-phrase
    match with hour-bucketed `ResetAt` rather than header-exact, and that a second
    `STAPLER_SQUAD_INSTANCE` has its own separate database.
  - *Given* the doc, *When* the "Reconciliation" caveat is read, *Then* it states explicitly that
    the `unaccounted_requests` residual is **expected to be routinely nonzero** under Tyler's
    normal multi-instance dev workflow (a second manual-test instance, the isolated e2e test
    server, a separate mobile-app instance all sharing one token per
    `docs/registry/`/`CLAUDE.md`'s documented patterns) and is **not itself evidence of a tracking
    gap** unless it correlates with a window where *no* other known instance was running
    (pre-mortem.md P1 #2) — a reviewer reading a nonzero residual in isolation should not treat it
    as a red flag.
  - *Given* the doc, *When* the "Verifying the Success Metric" section is read, *Then* it also
    requires checking the pause-visibility signal from Story 1.2.4/4.3.1 (see the updated
    criterion below) alongside `exhaustion_events`/`dropped_events`/residual, so a "0 exhaustions"
    reading achieved by silent pausing is not misread as "problem solved" (pre-mortem.md P1 #3).
**Files**: `.claude/docs/github-api-usage-tracking.md`, `CLAUDE.md`

##### Task 5.2.3a: Write the reference doc (~5 min)
- Cover: what is instrumented (native transport vs `gh` wrapper, including the stderr-based
  `gh`-CLI exhaustion detection and its hour-bucketing imprecision), the storage location
  (`~/.stapler-squad/analytics.db`, `event_category="github_api"`), the five config keys and their
  restart semantics, the four panel states, the pause-visibility stat (Story 4.3.1), the
  verification procedure (updated per Story 5.2.3's acceptance criteria to check pause duration
  alongside exhaustions/drops/residual), and the caveats — including that a nonzero reconciliation
  residual is routine under normal multi-instance dev workflow, not itself a trust signal.
- Files: `.claude/docs/github-api-usage-tracking.md`

##### Task 5.2.3b: Index it in CLAUDE.md (~2 min)
- Add a row to the Reference Documents Index table.
- Files: `CLAUDE.md`

---

#### Story 5.2.4: Ship gate
**As** the merging developer, **I want** the definitive checks green before the PR is opened,
**so that** no completion is claimed without proof.
**Acceptance Criteria**:
- The full pipeline passes and its output is shown, not summarised.
  - *Given* the finished branch, *When* `make ci` runs, *Then* it exits 0, and the transcript of
    that run is what justifies the "done" claim — per this repo's no-completion-claim-without-proof
    rule.
- Frontend tests pass separately (they are not part of `make ci`).
  - *Given* the branch, *When* `cd web-app && pnpm exec jest --no-coverage
    --testPathPatterns="GitHubApiUsage|useGitHubApiUsage"` runs, *Then* every new test passes.
    Use `pnpm`, never `npm` (`.claude/rules/package-manager.md`).
- Any flake surfaced during this work is fixed or filed, never re-excused.
  - *Given* an intermittent failure in a new ticker/goroutine test, *When* it appears,
    *Then* it is root-caused and fixed in this session, or filed as its own bug immediately
    (`.claude/rules/fix-flaky-tests-dont-defer.md`) — "known pre-existing flake" is not an
    acceptable disposition for a test this feature introduced.
- Planning artifacts are committed.
  - *Given* this plan and both ADRs, *When* the session ends, *Then*
    `project_plans/github-api-usage-tracking/` is committed
    (`.claude/rules/sdd-planning-artifacts-commit.md`).
**Files**: — (verification only)

##### Task 5.2.4a: Run the gate (~5 min)
- Run `make build && make ci`; then the web-app jest run; then
  `cd tests/e2e && npx playwright test github-api-usage.spec.ts`. Capture output.
- Files: —

##### Task 5.2.4b: Manual smoke on an isolated instance (~4 min)
- `go build -o /tmp/ssq-manual-test .` then
  `PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`;
  open `http://localhost:8999/analytics/github-api`; confirm the never-observed state, then confirm
  a populated tile after the probe's first run. **Never** `make install-service`.
- Files: —

---

## Plan Summary

| Level | Count |
|---|---|
| Phases | 5 |
| Epics | 13 |
| Stories | 31 |
| Tasks | 83 |
| Glossary terms | 26 |
| ADRs | 2 |

*(Counts updated during Phase 4 plan-repair: +1 story (3.2.3, the missing `UpdateGitHubUsageConfig`
write RPC), +11 tasks (1.3.3d; 3.2.3a–d; 1.3.2d–e; 1.2.4c–e; 4.3.1c), +1 glossary term
(`PauseStats`) — resolving cross-artifact consistency BLOCKERs 1–2 and pre-mortem P1 #1–#3.)*
