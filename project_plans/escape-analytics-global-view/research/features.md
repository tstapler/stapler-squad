# Research: Similar Aggregate/Summary Features

## 1. Closest precedent: Insights Dashboard (`server/services/insights_service.go`, `web-app/src/app/insights/`)

`InsightsService.GetInsightsSummary` (`server/services/insights_service.go:68-345`) is the
existing fleet-wide aggregate pattern this feature should mirror structurally:

- Same shape the requirements ask for: fleet-wide totals (`totalCostUSD`, token counts) **plus**
  a per-session breakdown list (`[]*sessionv1.SessionTokenSummary`, built at
  `insights_service.go:103,164`) so a user can spot an outlier session — exactly the ask in
  requirements.md's "per-session breakdown ... to spot the outlier session."
- Time-range filtering via `msg.From`/`msg.To` (`insights_service.go:78-83`, applied per-session
  at lines 114-119) — same optional `start_time`/`end_time` pattern requirements.md specifies.
- Frontend: `InsightsDashboard.tsx` composes `SummaryCards` (aggregate totals) +
  `SessionsTable` (per-session breakdown) + `TimeRangeFilter`, all driven by one
  `useInsightsSummary` hook. This is a good structural template for the new "All Sessions" tab
  on `EscapeAnalyticsPage.tsx`: one summary/aggregate section + one per-session table, gated by
  a single hook.
- **Important divergence, not to copy**: `GetInsightsSummary` does its aggregation by pulling
  `s.store.GetAll()` (all sessions' full parsed results) into Go memory and looping — this is
  the exact anti-pattern requirements.md's NFR explicitly says the new global endpoint must
  *not* repeat ("must not pull all matching rows into Go memory... use a real SQL aggregate").
  Insights can get away with it because its source (`s.store`) is an in-memory parsed-log store,
  not a SQL-backed ent table like `EscapeEvent`. Do not use `GetInsightsSummary` as a
  performance template — only as a UI/shape template.

## 2. ent GroupBy/Aggregate capability — resolves the Rabbit Hole / Open Question

The requirements doc flags as unresolved whether ent's fluent builder can do
`GROUP BY sequence_type, mangled` with counts, or whether a raw-SQL `sql.Selector` escape hatch
is needed.

**Finding: ent's generated code already has first-class GroupBy + Aggregate support.**
`session/ent/escapeevent_query.go`:
- `func (_q *EscapeEventQuery) GroupBy(field string, fields ...string) *EscapeEventGroupBy` (line 273)
- `func (_q *EscapeEventQuery) Aggregate(fns ...AggregateFunc) *EscapeEventSelect` (line 303)
- `EscapeEventGroupBy.Aggregate(fns ...AggregateFunc)` (line 446) and `.Scan(ctx, v)` (line 452)
- The doc comment example at line 271 shows the exact pattern needed:
  `GroupBy(field).Aggregate(ent.Count())...`

This is the standard ent-generated `GroupBy`/`Select`/`Aggregate` API present on every ent query
builder in this repo (also confirmed on `AnalyticsEventQuery` in
`session/ent/analyticsevent_query.go:259-273`), not something special-cased for one entity — it
runs a real `GROUP BY ... COUNT(...)` SQL query, satisfying the NFR without needing
`sql.Selector`/raw SQL. Two aggregate queries will likely be needed (one `GroupBy(sequence_type,
mangled)` for the histogram, one `GroupBy(session_id, mangled)` for the per-session breakdown) —
consistent with the Rabbit Hole's note that this may require two separate queries rather than one
combined query.

## 3. Existing per-session summary to extend, not replace

`server/services/analytics_escape_service.go:122-187` (`GetEscapeAnalyticsSummary`) is the
existing per-session summary the new global endpoint mirrors in *response shape* (histogram +
total_sequences + total_mangled + mangle_rate). Note it currently does in-Go aggregation via
`query.Select(...).All(ctx)` + a Go loop (lines 147-169) — the exact anti-pattern the NFR calls
out as "out of scope to fix here" but explicitly says not to repeat at global scale. The new
endpoint should follow this response shape/proto naming convention but implement aggregation via
`GroupBy`/`Aggregate` (see #2) instead of copying this method's `.All(ctx)` + loop technique.

## 4. Other candidate "all sessions" summaries checked (weaker matches)

- `server/services/session_summary_service.go`, `unfinished_work_service.go`,
  `backlog_service_query.go` — these list/summarize sessions but are status/lifecycle summaries
  (not numeric aggregates with histograms), less directly relevant as a template.
- `session/ent_repository.go` has `List`, `ListByStatus`, `ListByTag`, `GetAllSessionArtifacts`
  — simple "fetch all" list methods, not aggregate/GROUP BY patterns. Not a precedent for the
  aggregation mechanism itself, though `GetAllSessionArtifacts` (returns `map[string]...` keyed
  by session) is a reasonable precedent for the per-session breakdown's map-then-slice shape.

## 5. Edge Cases and Failure Modes to Handle

- **Zero sessions / zero events overall**: `GetEscapeAnalyticsSummary`'s existing code guards
  division by zero for mangle rate (`analytics_escape_service.go:176-179`,
  `if totalSeq > 0`) — the global endpoint must apply the same guard for the aggregate mangle
  rate AND independently for each per-session breakdown entry's mangle rate (a session with 0
  events must not divide by zero even if the global total is nonzero).
- **Time-range filter excludes everything**: with `start_time`/`end_time` set such that no
  events match, response should return zero totals and an empty histogram/breakdown list, not
  an error — mirror how `GetEscapeAnalyticsSummary` already handles this today (empty
  `events` slice, all downstream code degrades gracefully) rather than treating empty results as
  a failure.
- **`analyticsClient == nil`**: `GetEscapeAnalyticsSummary` returns
  `connect.CodeUnavailable` when `s.analyticsClient == nil` (line 132-134) — the new global
  handler needs the identical guard since it's the same client dependency.
- **Huge session counts / huge event counts**: this is exactly what the NFR's "real SQL
  aggregate" requirement exists for — with GroupBy/Aggregate (see #2), the per-session breakdown
  query returns one row per `session_id`, so its result size scales with *session count*, not
  *event count*, which should stay reasonable even at scale. Confirm in planning whether an
  unbounded number of distinct sessions (e.g. thousands of one-off/ephemeral sessions each with
  a handful of events) makes the breakdown table's *row count* itself a problem worth paginating
  — requirements.md's Open Questions already flags this as unresolved and defers it to Phase 3.
- **Session no longer exists / was deleted but has orphaned EscapeEvent rows**: the per-session
  breakdown is keyed by raw `session_id` from `EscapeEvent`, not joined against the live
  `Session` table — decide in planning whether to display raw session IDs as-is (simplest, most
  consistent with existing per-session dropdown behavior which already tolerates arbitrary
  session IDs) or filter out/label orphaned IDs.
- **Time-range filter with only one bound set** (`start_time` but no `end_time`, or vice versa):
  `GetEscapeAnalyticsSummary` already handles this correctly with independent `if` checks
  (lines 139-144) — same pattern to reuse, applied to whatever WHERE-clause-equivalent the
  GroupBy query construction uses.
- **Frontend stale-request race on toggle**: requirements.md's own Rabbit Holes section already
  flags that switching to "All Sessions" must suspend `useEscapeAnalyticsSummary`/
  `useEscapeEvents` cleanly rather than leaving them firing for a stale `selectedSessionId` —
  worth checking during implementation whether React Query's (or whatever data-fetching lib is
  in use here) `enabled: false` gate is already the established idiom elsewhere in this codebase
  for suspending a hook on tab switch (check `useInsightsSummary`/other session-scoped hooks for
  a precedent conditional-fetch pattern before inventing a new one).

## 6. Unstated User Needs Beyond Explicit Requirements

- **Sortable/highlighted outlier in the per-session breakdown table**: requirements.md's stated
  goal is explicitly "spot the outlier session," but the in-scope bullet only specifies the data
  shape (`{session_id, total_sequences, total_mangled, mangle_rate}`), not sorting/highlighting
  behavior. The Insights dashboard precedent (`SessionList.tokenCostSort.test.tsx`) shows this
  codebase already has a convention for sortable-by-metric session tables (a "Sort sessions by"
  dropdown with a `tokenCost` option) — the per-session breakdown table should very likely
  default-sort by `mangle_rate` descending (or expose a sort control) so the outlier is visually
  first, rather than sorting by `session_id` or insertion order, which would bury the very thing
  users need this feature for. Worth raising explicitly in Phase 3 planning since it's implied
  by the Problem Statement but not written into the Scope bullet.
- **Visual flagging of an anomalous mangle rate**, not just sortability: `MangleRateIndicator.tsx`
  already exists as a component for the per-session view (`web-app/src/components/analytics/MangleRateIndicator.tsx`)
  — reusing it per-row in the breakdown table (rather than a bare percentage number) would let a
  user visually scan for the outlier without needing to sort at all, consistent with existing
  visual language on this same page.
- **A quick way to jump from an outlier row to that session's own per-session detail view**: since
  the whole point of the aggregate view is diagnostic triage before drilling in, a per-session
  breakdown row that's clickable (switching the tab back to "Per-Session" pre-selected to that
  session) would close the loop the Problem Statement describes ("user has to click through
  sessions one by one") — worth flagging as a nice-to-have UX affordance even though
  requirements.md's Scope doesn't call it out, since without it the user still ends up manually
  re-selecting the session in the dropdown after finding it in the aggregate table.
- **Total/distinct session count in the aggregate summary** (e.g. "N sessions with events" as
  a stat alongside total_sequences/total_mangled): useful context for interpreting the mangle
  rate and breakdown table size, cheap to derive as a side effect of the `GroupBy(session_id)`
  query (row count), and not explicitly requested but a natural companion stat to the existing
  totals.
