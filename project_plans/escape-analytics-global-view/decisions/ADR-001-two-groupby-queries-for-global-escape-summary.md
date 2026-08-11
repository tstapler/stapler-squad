# ADR-001: Two independent ent `GroupBy`/`Aggregate` queries for the global escape summary

## Context

`GetEscapeAnalyticsGlobalSummary` needs two differently-shaped aggregate results from the same `EscapeEvent` table, filtered by the same optional time range: a histogram grouped by `sequence_type`, and a per-session breakdown grouped by `session_id`. The requirements doc flagged the query shape as a Rabbit Hole/Open Question rather than resolving it. Three approaches were considered (Step 0.5 of `implementation/plan.md`):

1. Two independent `GroupBy`/`Aggregate` queries (one per grouping key), sharing one time-filter predicate slice.
2. One combined `GroupBy(session_id, sequence_type, mangled)` query producing a three-key cross-product, folded into both shapes in Go.
3. Extend the existing per-session `GetEscapeAnalyticsSummary` handler with an "all sessions" flag, reusing its `Select(...).All(ctx)` + in-Go map/loop aggregation.

## Decision

Use two independent ent `GroupBy`/`Aggregate` queries (Approach 1). Both apply the same `[]predicate.EscapeEvent` time-range filter. Grand totals are folded in Go from the small histogram result set (bounded by distinct sequence-type count).

## Consequences

- Two round trips to the analytics SQLite DB per RPC call — acceptable for a read-only, low-frequency, dev-tool endpoint.
- Each query's row count is bounded by its own grouping key's cardinality (distinct sequence types; distinct sessions with ≥1 matching event), not by total event count — satisfies the NFR against pulling event-scale data into Go memory.
- Two independent queries are simpler to unit test in isolation (one test fixture, two separate assertions) than one three-key cross-product needing two different fold passes.
- Sessions with zero matching events are naturally excluded from the per-session breakdown (GROUP BY inherently behaves like an inner join) — this is the desired "spot sessions with activity" behavior, not a bug, and is documented explicitly so it isn't mistaken for one later.

## Alternatives Considered

- **Single combined `GroupBy(session_id, sequence_type, mangled)` query**: rejected. Row count scales with `sessions × sequence_types × 2`, which is larger than either of the two targeted queries in the common case, for no round-trip savings that matters at this endpoint's traffic scale. Also requires two separate Go-side fold passes over one three-key row shape instead of one pass each over a query already shaped for its purpose.
- **Extend `GetEscapeAnalyticsSummary` with an "all sessions" flag**: rejected. That handler's data-access strategy is `Select(...).All(ctx)` followed by in-Go map/loop aggregation (`server/services/analytics_escape_service.go:147-169`) — acceptable at single-session scale (bounded by one session's event count) but exactly the anti-pattern the new endpoint's NFR forbids once the `session_id` bound is removed. It also has no concept of a per-session grouping key at all, since `session_id` is a fixed input rather than a group-by dimension, so it can't produce a per-session breakdown without a structural rewrite anyway.
