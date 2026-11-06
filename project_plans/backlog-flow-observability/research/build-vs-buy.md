# Build vs. Adopt: Flow Analytics + MCP Exposure

**Date**: 2026-11-05. Evidence labels: VERIFIED (opened/grepped in this worktree) vs INFERRED.

## Decisive constraint

Claude must query the data through this app's own MCP server (`server/mcp`). Any external system (Grafana, Datadog, Tempo) is invisible to that server unless the app proxies it, so external options add a second query path rather than replacing the first. The data is already in the app's SQLite, and the app already owns the MCP surface.

## Repo facts (VERIFIED)

- Charts: `recharts ^3.8.1` is installed (`web-app/package.json:109`) and used in `web-app/src/app/insights/{DailySpendChart,ModelOverTimeChart,ModelBreakdownChart}.tsx`. No d3, visx, or sankey package is in package.json (grep for `d3|visx|sankey` is empty).
- OTel: `go.opentelemetry.io/otel` v1.44, `otlpmetricgrpc`, `sdk/metric` are direct deps (`go.mod:51-57`). `telemetry/telemetry.go:236` exposes `Meter()`. Existing instruments: HTTP, connect RPC, cache, search, `safeexec.sigkill_escalations`, cgroup gauges, connection gauges (`docs/how-to/enable-opentelemetry.md`). Disabled by default (`OTEL_ENABLED`). No backlog/flow instruments exist.
- Datadog is only an OTLP target described in `docs/how-to/enable-opentelemetry.md`; there is no Datadog-specific code path beyond OTLP.
- Existing SQL-native aggregation precedent: `server/services/analytics_escape_service.go:229-243` runs `GroupBy(...).Aggregate(Count, Sum)` in SQL via ent, not in Go. `analytics.db` is a separate SQLite file (ADR-013, `project_plans/analytics-system/decisions/ADR-013-separate-analytics-sqlite-file.md`) holding `AnalyticsEvent`/`EscapeEvent`, not backlog data.
- **Correction to requirements.md Open Question 1 / Rabbit Holes**: ent `StageTransition` (`session/ent/schema/stage_transition.go`) is a *workflow-graph edge* (from_stage, to_stage, enabled, gates). It is not a log of item transitions and cannot supply dwell time. `backlog_status_events` (`session/ent/schema/backlog_status_event.go`: item_id, from_status, to_status, triggered_by, note) is the only transition log; `backlog_stuck_states` is resolve-in-place with episode history intentionally not retained (comment in `session/ent/schema/backlog_stuck_state.go`, ADR-001). So stuck *history* (requirements (c) "historical") is not recoverable from that table and is a candidate capture gap.
- `get_backlog_stuck_report`: callable as an MCP tool in this session, but `grep -rn "get_backlog_stuck_report|backlog_stuck_report"` over `*.go`/`*.md` in this worktree finds no definition. UNVERIFIED where it is registered; the only `StuckReport` hit is `server/server.go` (a file match, not inspected further). Treat as per-item/current-state oriented (per requirements.md:18) and confirm before reuse.
- SQLite percentile support: no percentile/NTILE code exists in non-ent Go (grep empty). SQLite has window functions but no built-in `percentile_cont`; modernc.org/sqlite v1.56 and mattn/go-sqlite3 v1.40 are both in go.mod (`go.mod:37,66`). No gonum/tdigest dependency.

## 1. OSS libraries and external observability stack

### 1a. Process-mining / flow-metrics libraries
- Candidates: pm4py (Python; concepts only), Go: no mature process-mining library found; flow metrics (cycle time, CFD, throughput) are small enough to be plain SQL.
- Pros: pm4py's vocabulary (variants, rework loops, self-loops/"thrash", directly-follows graph) is a useful design reference for the episode model.
- Cons: Python runtime dependency in a single-binary Go app; designed for event logs of a few thousand cases with case_id/activity/timestamp, requiring export, with no MCP path. Our needs (dwell per stage, rework counts, FPY) are group-bys over a collapsed episode table.
- **Verdict: Not recommended as a dependency. Adopt the concepts only** (directly-follows counts, rework-loop = revisit of a stage).

### 1b. Chart libraries (React)
- recharts (installed): line/area/bar/stacked-area cover stage-dwell bars, CFD (stacked area), verdict mix over time, funnel (bar). Zero new dependencies, follows existing `insights/` patterns. Pros: no bundle growth, jscpd/pnpm constraints unaffected. Cons: no Sankey beyond a limited `Sankey` component (recharts ships `Sankey`; INFERRED adequate for ~9 stages, not verified in the repo).
- d3-sankey / visx: Pros: richer flow diagram. Cons: new deps, new lint/duplication surface (`web-app/.jscpd.json` threshold 0.1%), more code for a 9-node graph.
- **Verdict: recharts Recommended; d3-sankey/visx Viable later only if a funnel bar chart proves insufficient; defer.**

### 1c. Grafana / VictoriaMetrics / Tempo
- Pros: repo has skills `observability-grafana-dashboards` and `observability-victoriametrics-tempo`; app already exports OTLP metrics and traces, so gauges like `backlog_items_by_stage` are cheap to add; good for ops-style alerts and long-term time series.
- Cons: (i) Fails the key constraint: Claude cannot query it through this app's MCP without building a proxy. (ii) OTel metrics are pre-aggregated; "which items thrash" and drill-down to items need per-item data, which is high-cardinality (item_id labels) and a poor fit for Prometheus-style stores. (iii) Historical backfill of 258k existing events into metrics is not natural; counters start at zero when instrumentation lands. (iv) Extra infra for a single-user local app; `OTEL_ENABLED` is off by default.
- **Verdict: Not recommended as the primary path. Viable as an additive layer**: emit a few low-cardinality OTel gauges/counters (items per stage, thrash episodes opened, stuck reasons by count) from the same aggregator, for a Grafana user who wants them. Out of this pass's critical path.

### 1d. OTel metrics from existing code (in-app emission only)
- Pros: `Meter()` already wired; `session_creation_metrics.go` and `watch_stream_metrics.go` are patterns to copy.
- Cons: same cardinality and backfill limits as 1c; no MCP query.
- **Verdict: Viable as a complement, not Recommended as the source of truth.**

## 2. SaaS (Datadog)

- Pros: OTLP ingestion already documented; dashboards and anomaly monitors out of the box.
- Cons: recurring cost and lock-in for a single-user local tool (no pricing verified; INFERRED non-trivial for custom metrics with item-level tags); violates "Data residency: local only" (requirements.md:45); would export internal item titles/notes if used for drill-down; still not queryable via this app's MCP without wrapping Datadog's API (new credentials, auth surface).
- **Verdict: Not recommended** (conflicts with stated residency requirement).

## 3. LLM-written (bespoke) vs. SQL-native/library aggregation: where bespoke is reckless

Guideline: push set logic into SQL (precedent: `analytics_escape_service.go:229`), keep Go for the state-machine bits, and never hand-roll the following without property tests against a reference.

| Piece | Bespoke OK? | Why / recommendation |
|---|---|---|
| Counts, sums, group-bys (funnel, reasons, verdict mix, FPY) | Yes, in SQL | Declarative, matches existing ent `GroupBy().Aggregate()` usage. |
| Window-bounded queries on 260k-row table | Yes, with an index on (item_id, created_at) and (created_at) verified on live-size data | Requirement: <1s for 30 days. |
| Thrash/episode collapse (run-length grouping of ready<->in_progress flips by note) | Bespoke is unavoidable; keep it small and deterministic | No library does domain-specific noise collapse. Implement as one pure function over ordered events (gaps-and-islands via `LAG`/running sum, or a Go pass) with table-driven tests including the two real thrash items. Risk: off-by-one at episode boundaries and open-ended episodes (last event, no close); define dwell for open episodes against an explicit as-of time. |
| Percentiles (p50/p90/p95 dwell) | **Reckless to hand-roll an approximation; exact is fine at this scale** | SQLite lacks `percentile_cont`. With tens of items and thousands of collapsed episodes, compute exact nearest-rank percentiles in Go from a sorted slice (≈10 lines, one stated definition), or use `NTILE`/`ROW_NUMBER` in SQL. Do not add t-digest/streaming sketches (unneeded; error bars hidden). State the percentile method in the tool response. Median/p90 on n<20 is misleading: return `n` and flag low-sample. |
| Dwell time across state changes | Careful | Must use collapsed episodes (requirements rabbit hole); clamp negative/zero gaps from clock skew; attribute overlapping stuck states once. |
| Time-window before/after comparison | Careful | Compare same metric definitions on two windows; include `n`, as-of, and a partial/estimated flag instead of computing significance. A hand-rolled p-value is reckless; return raw counts and ratios, let the reader judge. |
| Reviewer-vs-agent "deadlock candidate" | Heuristic, label as heuristic | Verdicts link via `item_sessions`, not item_id; join errors silently drop rows. Add a reconciliation test (row counts match 526 verdicts). |
| Cause attribution from free-form `note` | Reckless if regex-guessed silently | Maintain an explicit closed set of recognized note patterns plus an `unclassified` bucket whose count is always reported. |

- **Verdict: SQL-native aggregation with one small, heavily tested Go episode-collapse function: Recommended. Library for percentiles/episodes: Not recommended (none fit). LLM-authored logic is acceptable only for the above bespoke pieces if every definition has a golden test built from the live DB numbers in requirements.md (e.g., 633 in_progress>review, 425 review>in_progress, 526 verdicts).**

## 4. Fork/adapt in-repo pieces

- **`project_plans/analytics-system` (AnalyticsEvent / `analytics.db`)**: Designed for user-behavior and web-vitals events (FR-2), with 90-day/100k retention (`requirements.md` FR-2) and a separate file (ADR-013). Pros: ent plumbing, retention goroutine (`server/analytics/retention.go`), summary-endpoint pattern. Cons: wrong subject (UI telemetry), a 100k-event retention cap is smaller than the 258k-row status table, and putting backlog flow into a different DB from `backlog_status_events` forces cross-file joins. **Verdict: Not recommended as a store; reuse only the retention-goroutine and summary-RPC patterns. Do not create a parallel store** (requirements.md:68).
- **Escape analytics (`server/services/analytics_escape_service.go`, `QueryEscapeAnalytics`/`GetEscapeAnalyticsSummary`/`GlobalSummary`)**: Pros: closest structural template (time-filter -> SQL `GroupBy` -> proto response; registry/feature entries and tests exist). Cons: different domain tables. **Verdict: Recommended as the pattern to copy (service shape, SQL-side aggregation, test layout), not as code to extend.**
- **`backlog_stuck_states` and the stuck report MCP tool**: Pros: ready-made current-state stuck data with reasons. Cons: no episode history (resolve-in-place; reopening resets `first_detected_at`), so "historical top reasons" and "time-in-stuck" are undercounted. **Verdict: Viable for the *current* stuck view; new capture (append-only episode log, or derive episodes from `backlog_item_activity_log` if reasons are noted there; INFERRED, not verified) is needed for historical.**
- **`backlog_status_events` as the core source**: Pros: only real transition log, already has item_id/from/to/note/created_at. Cons: 92% noise (238,552 of 257,973 per requirements.md:11). **Verdict: Recommended as the single source for dwell and rework, behind the episode-collapse layer.**
- **MCP surface**: add tools beside `server/mcp/tools_backlog.go` (per-item today) with tests alongside `tools_backlog_test.go` (requirements.md:37). Recommended; reuses auth, registration, and test harness.

## Recommendation summary

| Option | Verdict |
|---|---|
| Build in-app: SQL aggregation over `backlog_status_events` + `review_verdicts` + stuck states, thin Go episode-collapse, ConnectRPC + MCP tools, recharts Flow view | **Recommended** |
| Copy escape-analytics service pattern | **Recommended** |
| recharts for charts | **Recommended** |
| Add a few OTel gauges/counters from the same aggregator | Viable (later) |
| Grafana/VictoriaMetrics/Tempo as primary | Not recommended (no MCP path, cardinality, no backfill) |
| d3-sankey / visx | Viable (defer) |
| Reuse `analytics.db` / AnalyticsEvent as store | Not recommended |
| Datadog / SaaS | Not recommended (residency, cost, no MCP) |
| pm4py / process-mining libs | Not recommended (concepts only) |
| Streaming percentile sketches | Not recommended (exact is cheap at this n) |

## Gaps to carry into planning
1. Stuck-episode history is not retained; decide between a new append-only episode table and deriving from the activity log.
2. Confirm where `get_backlog_stuck_report` is registered; it is not present in `*.go` under this worktree.
3. Agent "believes done" signal: not evaluated here (see sibling research on capture).
4. Index coverage on `backlog_status_events` for window queries must be measured on the live DB, not assumed.
5. Open Question 1 answer: use `backlog_status_events`; `stage_transition` (ent) is the workflow graph, not a log.
