# Research: Build vs. Buy — escape-analytics-global-view

**Agent**: Research Agent 6 (Build vs. Buy)
**Date**: 2026-08-11

## 1. Existing OSS library/framework for the GROUP BY aggregation problem

The requirements doc's "Rabbit Hole" section assumes ent's fluent builder has no
first-class `GROUP BY` with count aggregation and that a `sql.Selector`/raw-SQL
escape hatch would be needed. **This assumption is wrong** — verified against the
generated code in this repo.

`session/ent/escapeevent_query.go` already has a fully generated `GroupBy`/`Aggregate`
builder for `EscapeEventQuery`, identical in shape to every other entity in the schema
(confirmed the same pattern exists on `AnalyticsEventQuery`,
`session/ent/analyticsevent_query.go:259-274`, and is standard ent codegen output
present across all 78 `*_query.go` files in `session/ent/`). `session/ent/ent.go:160-209`
defines the aggregate function helpers: `ent.Count()`, `ent.As(fn, alias)`, `ent.Sum(field)`,
`ent.Mean(field)`.

The doc-comment on `EscapeEventQuery.GroupBy` (`session/ent/escapeevent_query.go:259-271`)
spells out exactly the query this feature needs:

```go
client.EscapeEvent.Query().
    GroupBy(escapeevent.FieldSequenceType).
    Aggregate(ent.Count()).
    Scan(ctx, &v)
```

For the mangled-count-per-type histogram, group by two columns
(`GroupBy(escapeevent.FieldSequenceType, escapeevent.FieldMangled)`) with
`Aggregate(ent.Count())`, then fold the `mangled=true`/`mangled=false` rows per
sequence type in Go (cheap — one row per (type, mangled) pair, not per event). The
per-session breakdown is a second, structurally identical query grouped by
`escapeevent.FieldSessionID` instead.

This is ent's own idiomatic aggregation API, generated from the schema, not a
third-party library — so there is no external OSS package to evaluate as an
alternative; the "alternative" is genuinely already inside the dependency that's
already in use, one level down. It requires zero new dependencies and zero raw
`*sql.DB`/`sql.Selector` code.

- **Pros**: Zero new dependency; type-safe field references (`escapeevent.FieldSequenceType`)
  catch renames at compile time; identical code shape to `GetEscapeAnalyticsSummary`'s
  existing filter-building (`query.Where(...)` chains), so reviewers already know the
  idiom; SQL `GROUP BY` executes in the database, satisfying the NFR that no per-event
  row set gets pulled into Go memory; maturity is exactly ent's own maturity — this is
  the framework's flagship aggregation feature, not an edge case.
- **Cons**: `Scan(ctx, &v)` requires a matching destination struct/slice shape
  (field-tag-to-column mapping) — a mismatch is a runtime error, not a compile error, so
  the destination struct needs a unit test. Two-column `GroupBy` for the mangled-count
  histogram returns one row per (sequence_type, mangled) pair rather than one row per
  sequence_type with two counts — requires a small in-Go fold step after the query
  (bounded by distinct-type-count, not event-count, so this does not reintroduce the
  in-Go-aggregation-at-event-scale problem the NFR is guarding against).
- **Verdict: Recommended.** Use ent's generated `GroupBy`/`Aggregate` builder. The
  `sql.Selector` raw-SQL escape hatch flagged in the requirements doc's Rabbit Holes and
  Open Questions is unnecessary — this should be flagged to Phase 3 planning as a
  resolved assumption, not an open question.

## 2. SaaS / managed API

Not applicable, confirmed for completeness. This aggregates proprietary,
repo-specific `EscapeEvent` rows (terminal escape-sequence parsing telemetry) stored
in this project's own ent-ORM SQLite schema (`session/ent/schema/`). There is no
external vendor whose product ingests or aggregates this schema — it exists nowhere
outside this codebase. No SaaS/managed-API angle exists for this feature; the
build-vs-buy question here is purely "which in-repo/in-framework mechanism," addressed
in section 1.

## 3. LLM-generated implementation vs. battle-tested library for the aggregation query

With ent's generated `GroupBy`/`Aggregate` builder (section 1) as the mechanism, the
"hand-written" surface area shrinks to: the `GroupBy` field list, the `Aggregate(ent.Count())`
call, the destination struct for `Scan`, and the Go-side fold/rate-calculation logic —
not a hand-assembled raw SQL string.

- **SQL injection**: no risk. All identifiers are typed ent field constants
  (`escapeevent.FieldSequenceType`) resolved by the generated code to known column
  names; no user input is ever concatenated into a query string. This eliminates the
  injection concern the task description raises as something to check for — verified
  by reading the generated `sqlScan` implementation (`session/ent/escapeevent_query.go:460+`),
  which builds the query via the `sql.Selector` builder API (parameterized), not string
  formatting.
- **Off-by-one / correctness risk in the rate calculation**: real, but small and
  identical in shape to code already shipped and presumably reviewed in
  `GetEscapeAnalyticsSummary` (`server/services/analytics_escape_service.go:176-179`):
  `mangleRate = totalMangled / totalSequences` guarded by `if totalSeq > 0`. The new
  global endpoint reuses this exact guarded-division pattern per the existing
  precedent (see section 4) rather than reinventing it — so the correctness risk is
  bounded to "did the fold step count correctly," which a unit test with a small
  multi-session fixture (2-3 sessions, 2 sequence types, some mangled) fully covers.
  This is a straightforward read-only COUNT/GROUP BY with no writes, no migrations, and
  no external side effects, so the blast radius of a bug is a wrong number on a debug
  dashboard, not data corruption.
- **Pros of hand-writing (LLM-authored) here**: The query is simple enough (two
  `GroupBy` calls + one fold + one division) that introducing a query-building
  abstraction layer or third-party aggregation helper would be over-engineering for a
  1-2 day, complexity-2 feature — directly counter to
  `.claude/rules/interface-pollution-checklist.md`'s "unjustified generic" and
  "speculative interface" smells if wrapped in unnecessary abstraction.
- **Cons**: none rising to "meaningful risk" — the main mitigant (test coverage for the
  fold/rate logic) is already required by this project's own conventions
  (`.claude/rules/feature-registry.md`, `.claude/rules/e2e-test-conventions.md`) and by
  the requirements doc's own Feasibility Risks section, which already calls out needing
  new multi-session test fixtures.
- **Verdict: Recommended (acceptable risk).** Hand-writing the aggregation call against
  ent's generated builder is appropriate; ensure a unit test covers the multi-row fold
  and the zero-events divide-by-zero guard specifically, since those are the only two
  places a subtle bug could hide.

## 4. Fork or adapt — is `GetEscapeAnalyticsSummary` reusable as a helper?

Read in full (`server/services/analytics_escape_service.go:122-187`). Its *shape* is
directly reusable as a template, but not as a callable helper, because its data-access
strategy is exactly what the new endpoint's NFR forbids repeating:

- It does `query.Select(FieldSequenceType, FieldMangled).All(ctx)` — pulls every
  matching row into a Go slice — then aggregates with an in-Go `map[string]*EscapeSequenceCount`
  and a manual loop (lines 147-169). The requirements doc explicitly flags this pattern
  as out of scope to fix for the per-session endpoint, but says the *new* global
  endpoint "should not repeat that pattern at a much larger — all-sessions — scale"
  (Non-functional Requirements). Calling this function, or extracting its loop body as
  a shared helper, would inline the exact anti-pattern the NFR rules out.
- What **is** reusable: the request-validation shape (`analyticsClient == nil` guard,
  `StartTime`/`EndTime` optional-filter `Where` chain, lines 128-144), the response
  shape (`Histogram`/`TotalSequences`/`TotalMangled`/`MangleRate` — matches almost
  exactly what the new proto response needs per the requirements doc's Scope section),
  and the guarded-division mangle-rate calculation (lines 176-179, see section 3).
- Adaptation path: build the global summary handler as a structural sibling of
  `GetEscapeAnalyticsSummary` — same validation/filter/response shape — but swap the
  `Select(...).All(ctx)` + Go loop for the `GroupBy`/`Aggregate` query from section 1.
  If a shared piece is worth factoring out at all, it's the guarded mangle-rate
  division (`if totalSeq > 0 { rate = mangled/total }`), which is a one-line pure
  function — not the row-fetching/aggregation logic, which necessarily differs between
  the two endpoints (in-Go loop vs. SQL `GROUP BY`).
- **Pros of adapting the pattern (not the function)**: consistent code shape for
  reviewers already familiar with the per-session handler; reuses proven
  request-validation and response-shape decisions; avoids the interface-pollution
  smell of wrapping two structurally different data-access strategies behind one
  shared abstraction (`.claude/rules/interface-pollution-checklist.md`, smell #4
  "forwarding-only wrapper" / #6 "struct-wraps-struct").
- **Cons**: none significant — the two-query approach (histogram `GroupBy` +
  per-session `GroupBy`) mentioned as a Rabbit Hole in the requirements doc is simply
  two independent, structurally similar calls, not a shared code-path problem.
- **Verdict: Viable (adapt the pattern, do not fork/call the function).** Write the new
  handler as a sibling following the same request/response shape, but implement its
  data access with ent's `GroupBy`/`Aggregate` builder from section 1 rather than
  reusing `GetEscapeAnalyticsSummary`'s in-Go aggregation loop.

## Summary Table

| Option | Verdict |
|---|---|
| ent's generated `GroupBy`/`Aggregate` builder (vs. raw SQL `sql.Selector`) | **Recommended** |
| SaaS/managed API | Not applicable (confirmed, internal proprietary schema) |
| LLM-authored hand-written aggregation query vs. battle-tested library | **Recommended** (acceptable risk; no injection surface, bounded correctness risk covered by unit tests) |
| Fork/reuse `GetEscapeAnalyticsSummary` as a helper | **Viable** — adapt its request/response shape as a sibling handler; do not reuse its in-Go aggregation loop, which is the anti-pattern the NFR rules out |
