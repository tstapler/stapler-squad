# ADR-027: GitHub API Usage Events Reuse the Generic `AnalyticsEvent` Table in `analytics.db`

**Status**: Accepted
**Date**: 2026-08-10
**Project**: github-api-usage-tracking

## Context

`requirements.md`'s Open Questions leaves the persisted-storage mechanism explicitly for Phase 3 to
decide, and the Phase 2 research agents converged on two *different* reuse targets:

- `research/architecture.md` §4 and `research/features.md` §1 recommend forking
  `server/services/analytics_store.go`'s shape into a new `APIUsageStore` backed by a **new ent
  schema table** written through `session.Storage` into the **main session DB**
  (`~/.stapler-squad/sessions.db`).
- `research/build-vs-buy.md` §3/§4 recommends the **existing generic `AnalyticsEvent` schema** in
  the **dedicated `analytics.db`**, written through `server/analytics`'s already-built
  `SQLiteAnalyticsProvider`, with `server/analytics/retention.go`'s existing eviction reused.

Both are real, working, in-repo precedents. The relevant verified facts:

- `session/ent/schema/analytics_event.go` is domain-neutral: `event_name`, `event_category`,
  `session_id` (optional), `duration_ms` (optional), `page`, `component`,
  `labels map[string]string` (JSON), `created_at`, with indexes on `event_name`,
  `event_category`, `session_id`, `created_at`, and a composite `(event_name, created_at)`.
  Nothing about it is UI- or approval-specific.
- `server/analytics/db.go:23` (`OpenAnalyticsDB`) already opens a **separate** `analytics.db`
  (`db.go:28`) in WAL mode with `SetMaxOpenConns(1)` (`db.go:38`) to serialise writers, and runs
  `client.Schema.Create(ctx)` (`db.go:46`).
- `server/analytics/retention.go:24` (`StartRetentionEnforcer`) already implements hourly
  age-based **and** row-count-based eviction over `AnalyticsEvent`, wired at
  `server/server.go:635` from `cfg.AnalyticsMaxRowsOrDefault()` (100 000 —
  `config/config.go:584`) and `cfg.AnalyticsMaxAgeDaysOrDefault()` (90 days —
  `config/config.go:641`).
- `server/services/analytics_escape_service.go:116` (`GetEscapeAnalyticsSummary`) is a working
  precedent for a ConnectRPC handler that queries `analytics.db` directly via the ent client
  injected by `SessionService.SetAnalyticsClient` (`server/services/session_service.go:723`,
  wired at `server/server.go:656`).
- By contrast, `analytics_store.go`'s row shape (`Decision`, `RiskLevel`, `RuleID`,
  `CommandProgram`/`CommandCategory`/`CommandSubcommand`, `PythonImports`) is specific to
  approval classification and maps onto "a GitHub API request" only by inventing parallel fields.

## Decision

**Persist GitHub API usage events as `AnalyticsEvent` rows in the existing `analytics.db`, with
`event_category = "github_api"`.** No new ent schema entity is added and no ent codegen run is
required by this feature.

Row mapping:

| `AnalyticsEvent` field | GitHub usage meaning |
|---|---|
| `event_category` | constant `"github_api"` |
| `event_name` | the `CallSite` (e.g. `pr_status_poller`, `gh_cli.merge_pr`, `backlog_issue_import`) |
| `session_id` | originating session ID when one exists; empty for timer-driven pollers |
| `duration_ms` | wall time of the round-trip (native) or of the `gh` subprocess (CLI) |
| `labels` | `resource`, `status`, `method`, `fidelity`, `quota_charged`, `remaining`, `limit`, `reset_at`, `host` |
| `created_at` | request completion time (ent default) |

Two adjustments are adopted alongside the reuse:

1. **Writes are buffered, not synchronous.** `SQLiteAnalyticsProvider.Record`
   (`server/analytics/sqlite_provider.go:27`) is a synchronous ent insert on a
   `MaxOpenConns(1)` connection shared with the escape-event batch writer
   (`server/server.go:643`). Calling it inline from `RoundTrip` would put a contended disk write
   in the critical path of every GitHub call — the exact regression `research/pitfalls.md` §3c
   names. The **async-buffer pattern** from `analytics_store.go` (bounded channel, non-blocking
   `Record`, drop-with-counter on overflow, background flush, drain-on-`Stop`) is therefore
   adopted as the *pattern*, feeding `SQLiteAnalyticsProvider` as the *storage*. This is the
   synthesis of the two research recommendations, not a choice between them.
2. **A category-scoped retention phase is added.** The existing count-based eviction deletes the
   globally-oldest `AnalyticsEvent` rows regardless of category, so unbounded `github_api` growth
   would compete with UI telemetry for the same 100 000-row budget. A `github_api`-scoped
   age prune (`GitHubUsageRetentionDays`, default 30) is added to `runRetention`
   (`server/analytics/retention.go:49`) so GitHub rows self-limit before they can crowd out the
   shared cap. This directly closes `research/pitfalls.md` §3a's "don't inherit no-policy by
   copying the pattern uncritically."

## Consequences

- **Zero schema churn.** No `session/ent/schema` change means no
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` run,
  no regenerated `session/ent/**` diff to review, and no risk of the
  `.claude/rules/ent-schema-generation.md` footgun. The `Migration Plan` section of `plan.md` is
  correspondingly "no migration."
- **No blast radius on the main session DB.** Writes land in `analytics.db`, which no session
  lifecycle code reads. A corrupted or full analytics DB cannot wedge session state — consistent
  with `research/features.md` §5's "treat usage data as best-effort telemetry, not source of
  truth."
- **Multi-process safety comes for free.** `analytics.db` is already opened per config dir with a
  single serialised writer connection, and `STAPLER_SQUAD_INSTANCE` already scopes the config dir
  — so the orphaned-process multi-writer hazard in
  `.claude/rules/service-restart-orphan-process.md` is handled by the same mechanism that already
  protects escape analytics, with no new file or lock introduced.
- **Attribution is stringly-typed at the storage boundary.** `event_name` and `labels` are
  `string`/`map[string]string`. This is mitigated in application code by the `CallSite` and
  `GitHubResource` newtypes (see `plan.md`'s Pattern Decisions), which are the only things
  permitted to produce those strings — the DB is a serialisation format, not the type boundary.
- **Query ergonomics are slightly worse than a purpose-built table.** Filtering by resource means
  a JSON `labels` comparison rather than an indexed column. Accepted because the composite
  `(event_name, created_at)` index already covers the dominant query (window + call-site
  breakdown), and the volume ceiling is "hundreds to low thousands of requests/day"
  (`requirements.md` Non-functional Requirements) — a full-window scan of ~30 days is tens of
  thousands of rows, not millions. If this ever becomes a real cost, promoting `resource` to a
  first-class column is a strictly additive follow-up.
- **The public POST analytics endpoint stays closed to this category.**
  `server/handlers/analytics_handler.go`'s `validCategories` gate is deliberately *not* extended
  with `"github_api"` — these events originate server-side and go straight through the provider,
  so no browser can forge them.

## Alternatives Considered

- **Fork `server/services/analytics_store.go` into a new `APIUsageStore` + new ent schema table in
  the main session DB** (`research/architecture.md` §4, `research/features.md` §1): rejected as
  the *storage* target. It requires a new ent entity, a regenerated `session/ent/**`, new
  `session.Storage`/`session.Repository` interface methods, and schema churn on the database that
  holds live session state — all to obtain a table whose useful columns are exactly the ones
  `AnalyticsEvent` already has. Its async-write *pattern* is adopted (see Decision point 1); its
  schema and target DB are not.
- **A flat JSON/JSONL file under `~/.stapler-squad/`**: rejected — no time-window query, no
  aggregation, no retention, and `requirements.md` explicitly asks for windowed
  volume-over-time and per-source breakdown, which would mean hand-writing a query engine.
- **In-memory ring buffer only**: rejected as disqualifying, not merely weaker —
  `requirements.md` Scope requires "persisted (survives restart)", and per
  `.claude/rules/tmux-keep-server-on-restart.md` and
  `.claude/rules/service-restart-orphan-process.md` restarts in this app are both routine and
  sometimes involuntary.
- **Emit to OTel/Datadog instead of storing locally**: rejected by `requirements.md`'s Constraints
  — OTel is opt-in (`OTEL_ENABLED=true`) and must not become a hard requirement. Remains a
  strictly additive future companion on the same write path.
