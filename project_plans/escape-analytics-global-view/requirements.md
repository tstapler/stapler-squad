# Requirements: escape-analytics-global-view

**Date**: 2026-08-11
**Type**: feature addition
**Complexity**: 2 — focused feature

## Problem Statement
The Escape Analytics page (`web-app/src/components/analytics/EscapeAnalyticsPage.tsx`) only shows escape-sequence statistics (histogram, mangle rate) for one session at a time, selected from a dropdown. There is no way to see whether a mangle-rate spike or unusual sequence-type distribution is isolated to one session or systemic across the fleet — a user has to click through sessions one by one and mentally compare.

## Baseline
Today, to spot a systemic escape-parsing issue, a user must select each session individually in the dropdown, note its histogram/mangle rate, and repeat across all sessions — no cross-session comparison exists in the UI or API.

## Users / Consumers
Same audience as the existing per-session page: developers/operators of stapler-squad debugging terminal rendering/mangle issues. Consumed via the existing `web-app` React SPA over ConnectRPC; no external/third-party consumers.

## Success Metrics
A user can view aggregate escape-sequence totals, histogram, and mangle rate across all sessions (optionally within a time range) without selecting a session first, and can see each session's own totals side-by-side to spot outliers — replacing the current one-by-one dropdown click-through.

## Appetite
Small (1–2 days). Scope is mirroring an existing, already-implemented per-session summary pattern (`GetEscapeAnalyticsSummary`) at the aggregate level, plus a UI toggle — not new architecture.

## Constraints
None specified. Follow existing repo conventions (proto/ent/ConnectRPC patterns, vanilla-extract CSS, feature registry, e2e test conventions).

## Non-functional Requirements
- **Performance SLO**: not specified; aggregate query must not pull all matching rows into Go memory for large event volumes — use a real SQL aggregate (`GROUP BY`), matching the concern already raised for the existing per-session summary endpoint (`server/services/analytics_escape_service.go`'s `GetEscapeAnalyticsSummary` currently does pull rows client-side and is out of scope to fix here, but the new global endpoint should not repeat that pattern at a much larger — all-sessions — scale).
- **Scalability**: not applicable / no specific target given.
- **Security classification**: internal (local dev tool, no auth boundary beyond existing session auth).
- **Data residency**: not applicable.

## Scope
### In Scope
- New ConnectRPC endpoint `GetEscapeAnalyticsGlobalSummary` returning:
  - Sequence-type histogram (counts + mangled counts) aggregated across all sessions
  - Total sequences, total mangled, mangle rate — same shape as today's per-session summary
  - Optional time-range filter (`start_time`/`end_time`), mirroring the existing per-session summary's filter
  - Per-session breakdown: list of `{session_id, total_sequences, total_mangled, mangle_rate}` so a user can spot the outlier session
- Frontend: a "Per-Session | All Sessions" tab/toggle on the existing `EscapeAnalyticsPage`. Selecting "All Sessions" hides the session dropdown and event table, and renders the aggregate histogram/mangle-rate summary plus the per-session breakdown table.
- Proto changes (`make proto-gen`), `// +api:` marker on the new handler, feature registry entries (backend + frontend), e2e test coverage per `.claude/rules/e2e-test-conventions.md`.
- ent aggregate query implemented via `GROUP BY` (raw SQL via ent's `sql.Selector`/`Modify`, or ent's `GroupBy` aggregation builder) rather than `Select(...).All(ctx)` + in-Go aggregation.

### Out of Scope
- Historical trending / time-series charts (e.g. mangle rate over time) — this feature is a snapshot aggregate, not a dashboard with graphs.
- Fixing the existing per-session summary's in-Go aggregation pattern (noted above as a pre-existing concern, not this feature's job).
- Cross-session event-level browsing (the paginated `QueryEscapeAnalytics` event table) in the aggregate view — only summary-level aggregation is in scope; per-event browsing stays session-scoped.
- Alerting/notification on aggregate mangle-rate thresholds.

## Rabbit Holes
- **ent aggregate query shape**: ent's fluent query builder doesn't have a first-class `GROUP BY sequence_type, mangled` with count aggregation out of the box for arbitrary group keys — this may require ent's `sql.Selector` escape hatch or raw SQL via the underlying `*sql.DB`. Needs explicit resolution in Phase 3 planning (which mechanism, and how it interacts with ent's driver abstraction) rather than assumed to be a one-line change.
- **Per-session breakdown at scale**: if there are many sessions with events, a `GROUP BY session_id` alongside the `GROUP BY sequence_type` histogram means either two separate aggregate queries or a more complex single query — resolve the exact query plan in planning, not implementation.
- **UI toggle vs. existing session-selector state**: switching to "All Sessions" must cleanly suspend/hide the per-session hooks (`useEscapeAnalyticsSummary`, `useEscapeEvents`) without them still firing requests for a stale `selectedSessionId` — needs explicit handling in the component, not just conditional rendering of the output.

## Alternatives Considered
- Standalone new page/route for the global view — rejected per user's explicit preference (tab/toggle on the existing page keeps analytics in one place).
- Client-side aggregation (fetch all sessions' summaries and sum in the browser) — rejected: doesn't scale, N+1 RPC calls, and duplicates aggregation logic that belongs server-side next to the existing per-session summary logic.

## Feasibility Risks
- ent's aggregation API limitations (see Rabbit Holes) could push the "real SQL aggregate" NFR toward a raw-SQL escape hatch — acceptable, but the research/planning phases must confirm ent's actual capability here rather than assuming.
- No existing test fixtures/seed data for multi-session escape event scenarios — Phase 5 implementation will need to construct these for both unit and e2e tests.

## Observability Requirements
*(complexity 2 — not required, but noted)* Standard request logging via existing ConnectRPC middleware is sufficient; no new metrics/alerts needed for a read-only summary endpoint.

## Risk Control
*(complexity 2 — not required)* Not needed — read-only, additive endpoint and UI toggle; no rollout risk beyond normal review/testing. No feature flag needed.

## Open Questions
- Exact ent mechanism for the `GROUP BY` aggregate (raw SQL via `sql.Selector` vs. ent's aggregation helpers) — resolve in Phase 2 research.
- Whether the per-session breakdown table needs its own pagination if the number of sessions with events grows large — resolve in Phase 3 planning (likely: no pagination needed for an MVP dev tool, but confirm against realistic session counts in this repo's own data).
