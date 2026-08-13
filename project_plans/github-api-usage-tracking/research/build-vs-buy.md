# Research: Build vs. Buy — GitHub API Usage Tracking

**Agent**: 6 (Build vs. Buy)
**Date**: 2026-08-10

## Summary Recommendation

**Recommended**: Adapt the existing `server/analytics` (ent + dedicated SQLite `analytics.db`) infrastructure — write GitHub API request events through a new `EventCategory` ("github_api") into the *existing* `AnalyticsEvent` table via a new backend-only writer, reusing `SQLiteAnalyticsProvider`, the retention enforcer, and `recharts` (already a dependency, already used for time-series charts) for the UI. Do **not** adopt `go-github`, do **not** reach for a SaaS/OTel dependency, and treat `server/services/analytics_store.go` (the approval-analytics store) as a *pattern reference* rather than a fork target — the closer, more directly reusable precedent is `server/analytics/db.go` + `AnalyticsEvent`, not `analytics_store.go`.

---

## 1. Existing OSS Library or Framework

**Finding**: This repo does **not** depend on `github.com/google/go-github` or any GitHub SDK. `go.mod` (verified: `grep -n github go.mod`, no `google/go-github` line) shows only a hand-rolled client. `github/http_client.go` (148 lines) builds raw `*http.Request`s with `http.NewRequestWithContext`, manual `Authorization: Bearer` headers, and manual `X-GitHub-Api-Version` headers. `github/rate_limit.go`'s `DefaultRateLimiter.Update(resp *http.Response)` already hand-parses `X-RateLimit-Remaining`/`X-RateLimit-Limit`/`X-RateLimit-Reset`/`Retry-After` headers directly off the raw response — this is a deliberate choice already made, not an oversight.

- **Pros of adopting go-github now**: would bring a typed `Rate`/`RateLimits` struct and a `RateLimits(ctx)` call, saving some header-parsing boilerplate.
- **Cons**: go-github does not provide *history* or *tracking* — it only exposes the same current-snapshot headers this repo already parses by hand. Adopting it would mean rewriting `github/client.go` (896 lines) and `github/http_client.go` around a new client shape, for zero net gain toward this feature's actual goal (persisted history, attribution, UI). It also does nothing for the `gh` CLI shell-out call sites (`GetPRInfoCtx`), which are the harder problem (see Rabbit Holes in requirements.md).
- **Verdict**: **Not recommended.** No existing Go library solves "persisted rate-limit history + call-site attribution" — that's inherently bespoke to this app's data model. The hand-rolled client is an intentional, working precedent; migrating clients is an unrelated, much larger, unjustified-by-this-feature refactor.

## 2. SaaS / Managed API

**Finding**: GitHub's REST API only exposes the current-snapshot `/rate_limit` endpoint and per-response headers — there is no GitHub-hosted historical rate-limit API to query after the fact (well-known GitHub API limitation, not something this repo could work around). Separately, this app already has an optional Datadog/OTel export path (`.claude/docs/opentelemetry.md`), which is a real observability SaaS integration already wired in.

- **Pros of leaning on OTel/Datadog instead of building a store**: no new storage code, dashboards "for free" if already using Datadog.
- **Cons**: `requirements.md`'s Constraints section is explicit — OTel is opt-in (`OTEL_ENABLED=true`), off by default locally, and **must not become a hard requirement** for this feature to be useful standalone. Success Metrics also requires the tracking data to be queryable to verify the "zero occurrences of rate-limit-exhausted" goal over a 14-day window — that only works if the data exists unconditionally, not gated behind an opt-in flag Tyler may not have enabled. Out of Scope explicitly excludes "OTel/Datadog metric export for this data" as a *future* addition, not this iteration's job.
- **Verdict**: **Not recommended** as the primary mechanism (violates the standalone-usefulness constraint); **viable as a future companion** — nothing in a local SQLite-backed design blocks *also* emitting OTel metrics later, since that's additive instrumentation on the same write path.

## 3. LLM-Generated Implementation vs. Battle-Tested Library (storage + charting)

**Storage**: three real options exist in-repo already:
1. Hand-rolled JSON file (like `session/storage.go`'s older patterns) — rejected: no query capability, awkward for time-range filtering/aggregation the UI needs (time-range selector, per-resource breakdown).
2. Embedded SQLite/BoltDB from scratch — redundant: `github.com/mattn/go-sqlite3` is **already a project dependency**, already wired through `entgo.io/ent`, with a working dedicated-DB pattern in `server/analytics/db.go` (`OpenAnalyticsDB`, single-writer `MaxOpenConns(1)` WAL-mode SQLite, auto-migration via `client.Schema.Create(ctx)`). Building a second, parallel SQLite-via-database/sql path from scratch would duplicate this with no benefit.
3. Reusing `ent` via the existing `analytics.db` and `AnalyticsEvent` schema (`session/ent/schema/analytics_event.go`) — **already generic**: `event_name`, `event_category`, `session_id` (optional), `duration_ms` (optional), `page`/`component` (optional, frontend-oriented but not required), `labels map[string]string` (JSON), `created_at`, with indexes on `event_name`, `event_category`, `session_id`, `created_at`, and a composite `(event_name, created_at)` index — exactly the shape needed for `(call_site, timestamp, resource, status)` query patterns. `server/analytics/retention.go`'s `StartRetentionEnforcer` already implements age- and row-count-based eviction, which this feature needs anyway (low volume, hundreds–thousands of requests/day, but unbounded over time without retention).

- **Verdict**: **Recommended** — write new events into the existing `AnalyticsEvent` table (new `event_category = "github_api"`, `event_name` = call site or endpoint, `labels` = `{resource, status, poller}` etc.) via `server/analytics.SQLiteAnalyticsProvider.Record`. This reuses tested schema-migration, retention, and connection-serialization code with zero new storage abstraction. Two adjustments needed: (a) `server/handlers/analytics_handler.go`'s `validCategories` map currently rejects anything outside `{"user_action","performance","navigation","rpc"}`, but that only gates the public POST endpoint used by the *frontend* — server-side writers (rate-limit transport, pollers) call `SQLiteAnalyticsProvider.Record` directly and are not subject to that validation, so either add `"github_api"` to the allowed set (if any future frontend-originated write is wanted) or bypass the HTTP path entirely (likely correct, since these events originate server-side). (b) confirm current `analytics.db` retention defaults are appropriate for this volume or add a GitHub-specific retention call.

**Charting**: `recharts` (`"recharts": "^3.8.1"`) is already a `web-app/package.json` dependency and is already in active use for time-series + breakdown charts at `web-app/src/app/insights/DailySpendChart.tsx`, `ModelBreakdownChart.tsx`, and `ModelOverTimeChart.tsx` — a directly analogous "requests over time + breakdown by category" visualization already exists as a pattern to copy.

- **Verdict**: **Recommended** — build the new usage-history panel's charts with `recharts`, following the `DailySpendChart`/`ModelBreakdownChart` pattern, not hand-rolled SVG. A hand-rolled SVG chart would be reinventing what's already proven and would introduce untested rendering/accessibility edge cases (axis scaling, tooltips, responsive resize) recharts already handles.

## 4. Fork or Adapt — `server/services/analytics_store.go`

**Finding**: `analytics_store.go` (682 lines) is the **approval-analytics** store — it's a `sync`-guarded in-memory channel + 5-second-interval flush (`AnalyticsStore.flush`) that writes `AnalyticsEntry` values through `session.Storage.RecordAnalytics` → `session.EntRepository.RecordAnalytics` (`session/ent_repository.go:1341`) — a *different* ent-backed table living in the **main session database**, not the dedicated `analytics.db`. Its schema (`AnalyticsEntry`) is tightly shaped around approval-classifier concepts: `Decision`, `RiskLevel`, `RuleID`/`RuleName`, `CommandProgram`/`CommandCategory`/`CommandSubcommand`, `PythonImports` — none of which map cleanly onto "GitHub API request, resource, remaining/limit, call site."

Repurposing it directly would require: a parallel `GitHubRequestEntry` struct, a new `RecordFromGitHubRequest`-style constructor, new fields on `session.AnalyticsData` (or a new `AnalyticsData`-shaped struct) threaded through `session/repository.go`'s interface and `session/ent_repository.go`'s implementation, plus new `session/ent/schema` fields/migration on the *main* session DB — schema churn on a database not otherwise touched by this feature, and conceptually a mismatch (rate-limit/request events are not "classification decisions").

By contrast, `server/analytics`'s `AnalyticsEvent` (see §3) is already domain-neutral (`event_name`/`event_category`/`labels`) and lives in its own dedicated, already-retention-managed database — a strictly better fit with less schema risk.

- **Pros of forking `analytics_store.go`**: its async-channel + periodic-flush pattern is a reasonable model for non-blocking writes off the hot request path, if write-path latency ever becomes a concern (it currently isn't, per requirements.md's Non-functional Requirements — negligible overhead expected, hundreds–low-thousands of requests/day).
- **Cons**: schema shape is approval-specific and not reusable without new fields; writes go to the main session DB, an unrelated blast radius; would require its own new UI data-hook (`useApprovalAnalytics`-equivalent) built from scratch anyway since the RPC/response shape is bespoke to approvals.
- **Verdict**: **Viable but not recommended** as the storage target. **Recommended** only for its async-write *pattern* (if buffering is later found necessary) — but the schema and target database should be `server/analytics`'s `AnalyticsEvent`/`analytics.db`, not a fork of `analytics_store.go` itself.

---

## Consolidated Verdicts

| Option | Verdict |
|---|---|
| 1. Adopt `go-github` or similar OSS lib | Not recommended |
| 2. SaaS / OTel-Datadog as primary mechanism | Not recommended (violates opt-in constraint); viable as future companion |
| 3a. Hand-rolled JSON file storage | Not recommended |
| 3b. New embedded SQLite/BoltDB from scratch | Not recommended (duplicates existing `analytics.db` pattern) |
| 3c. Reuse `ent` + existing `AnalyticsEvent`/`analytics.db` | **Recommended** |
| 3d. Hand-rolled SVG charting | Not recommended |
| 3e. `recharts` (already a dependency, already used for time-series) | **Recommended** |
| 4. Fork `analytics_store.go` (approval-analytics store) directly | Not recommended as storage target; viable only for its async-write pattern |
| 4b. Adapt `server/analytics` (`AnalyticsEvent` + `SQLiteAnalyticsProvider` + `retention.go`) | **Recommended** |

## Open Question Resolved

Per requirements.md's Open Questions: "What's the right persisted-storage mechanism — reuse `analytics_store.go`'s pattern, a flat JSON file, or something else?" — **something else**: reuse `server/analytics`'s `AnalyticsEvent`/`SQLiteAnalyticsProvider`/`retention.go`, which is a closer structural match than `analytics_store.go` and requires no new database, migration framework, or charting library.

## Sources (files opened, this repo, verified 2026-08-10)

- `go.mod` — confirmed no `google/go-github` dependency; confirmed `github.com/mattn/go-sqlite3` and `github.com/fsnotify/fsnotify` present
- `github/http_client.go`, `github/rate_limit.go`, `github/client.go` — hand-rolled GitHub HTTP client, current-snapshot-only rate limit tracking
- `server/services/analytics_store.go` — approval-analytics store (async channel, 5s flush, writes via `session.Storage.RecordAnalytics`)
- `session/ent_repository.go:1341`, `session/repository.go:115-116`, `session/storage.go:641-643` — approval-analytics write path (main session DB)
- `server/analytics/db.go` — `OpenAnalyticsDB`, dedicated `analytics.db`, WAL mode, single-writer, ent auto-migration
- `server/analytics/sqlite_provider.go` — `SQLiteAnalyticsProvider.Record`, generic `Event` → `AnalyticsEvent` write
- `server/analytics/retention.go` — `StartRetentionEnforcer`, age- and row-count-based eviction, already generic
- `session/ent/schema/analytics_event.go` — `AnalyticsEvent` schema: `event_name`, `event_category`, `session_id`, `duration_ms`, `page`, `component`, `labels map[string]string`, `created_at`, indexed on name/category/session/created_at
- `server/handlers/analytics_handler.go` — `validCategories` gate on the public POST endpoint (`user_action`/`performance`/`navigation`/`rpc`) — only applies to frontend-originated writes
- `web-app/package.json:91` — `"recharts": "^3.8.1"`
- `web-app/src/app/insights/DailySpendChart.tsx`, `ModelBreakdownChart.tsx`, `ModelOverTimeChart.tsx` — existing recharts time-series/breakdown chart precedents
- `grep -rn fsnotify` across non-test `.go` files — no config hot-reload usage found (fsnotify used for tmux/history/plugin watching, not `config/` reload), confirming requirements.md's Open Question that config hot-reload does not currently exist
