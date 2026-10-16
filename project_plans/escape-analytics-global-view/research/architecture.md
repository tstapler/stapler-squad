# Architecture Research: escape-analytics-global-view

## 1. ent's real GROUP BY / COUNT mechanism — RESOLVED

**ent already generates a first-class `GroupBy(...).Aggregate(...)` builder for every entity, including `EscapeEvent`.** This is not something that needs to be hand-rolled via `sql.Selector`/`Modify` — it's already present in the generated code this repo has checked in.

Verified in `session/ent/escapeevent_query.go:259-305`:

```go
// GroupBy is used to group vertices by one or more fields/columns.
// It is often used with aggregate functions, like: count, max, mean, min, sum.
//
//	client.EscapeEvent.Query().
//		GroupBy(escapeevent.FieldSessionID).
//		Aggregate(ent.Count()).
//		Scan(ctx, &v)
func (_q *EscapeEventQuery) GroupBy(field string, fields ...string) *EscapeEventGroupBy
```

`sqlScan` (`session/ent/escapeevent_query.go:460-485`) shows exactly what SQL this produces: it builds a `sql.Selector`, appends the aggregation expressions (`fn(selector)` for each `AggregateFunc`) to the SELECT list, and calls `selector.GroupBy(selector.Columns(*_g.flds...)...)` — a real `SELECT col1, col2, COUNT(*) ... GROUP BY col1, col2` sent to the driver via `_g.build.driver.Query(ctx, query, args, rows)`. No rows are pulled into Go and aggregated in a loop; the database does the grouping.

The aggregation primitives (`session/ent/ent.go:154-217`) are:

```go
type AggregateFunc func(*sql.Selector) string

func As(fn AggregateFunc, end string) AggregateFunc   // rename via SQL AS
func Count() AggregateFunc                             // COUNT(*)
func Sum(field string) AggregateFunc                   // SUM(col)
func Mean(field string) AggregateFunc                   // AVG(col)
func Max(field string) AggregateFunc / Min(field string) AggregateFunc
```

**Important repo-specific fact confirmed**: this project uses SQLite (`mattn/go-sqlite3`, confirmed via `server/services/database_service.go`, `session/agy_adapter.go`), and ent's `field.Bool("mangled")` is stored as an INTEGER 0/1 column in SQLite. That means `ent.Sum(escapeevent.FieldMangled)` computes exactly "count of mangled=true rows" in the same query as the `Count()` — no second boolean-specific aggregate function is needed, no `CASE WHEN` required.

Grep results: `GroupBy`/`Aggregate` builders exist for *every* generated entity (`grep -rl GroupBy session/ent/*.go` → 26 files) but **no current call site in this repo actually uses them yet** (`grep -rn "\.GroupBy(\|\.Aggregate(" server/services/*.go session/*.go` → zero matches). This feature would be the first real usage of ent's aggregation builder in the codebase — there's no existing in-repo pattern to copy, but the generated API itself is fully functional and well-documented in its own doc comments.

### Escape hatch (not needed here, but confirmed to exist)

If a query shape ever needed something `GroupBy`/`Aggregate` can't express (e.g. a `HAVING` clause, a window function, or a join the fluent builder doesn't support), the drop-down is `query.Modify(func(s *sql.Selector) {...})` or building a raw `sql.Dialect(...).Select(...)` against `client.EscapeEvent.Query()`'s underlying `_q.driver` — the same `dialect/sql` package `GroupBy` itself uses internally (`sql.Selector`, `sql.Count`, `sql.Sum`, etc., all in `entgo.io/ent/dialect/sql`). This repo's `analyticsClient *ent.Client` (`server/services/session_service.go:181`) exposes `.Driver()` for that purpose if ever required. **Not needed for this feature** — `GroupBy`+`Aggregate` covers both required queries.

## 2. Recommended approach — two queries, not one

The requirements ask for two independent aggregate shapes:
- A sequence-type histogram: `GROUP BY sequence_type` (with COUNT and mangled-COUNT)
- A per-session breakdown: `GROUP BY session_id` (with COUNT and mangled-COUNT)

These are two different `GROUP BY` keys over the same base table/filter (optional time range). A single SQL query cannot produce both grain levels at once without pulling `session_id × sequence_type` cross-product rows and re-rolling one dimension in Go — which just reintroduces the anti-pattern this feature is explicitly avoiding. **Two separate `GroupBy(...).Aggregate(...).Scan(...)` calls, both against `s.analyticsClient.EscapeEvent.Query()` with the same time-range predicate applied to each**, is the correct and simplest plan. Both queries are still real SQL aggregates — no `.All(ctx)` row pull, and the total row count returned to Go is `O(#distinct sequence types) + O(#sessions)`, not `O(#events)`.

A third tiny scalar query (or a `Count()`-only aggregate with no `GroupBy`) gets the overall `total_sequences`/`total_mangled` — or, more efficiently, derive those by summing the per-sequence-type histogram in Go (cheap: at most a few dozen rows, not raw events), avoiding a third round-trip. Recommend the latter to keep it at exactly 2 SQL queries.

### Code sketch

```go
// GetEscapeAnalyticsGlobalSummary returns aggregate escape sequence statistics across all sessions.
// +api: escape:global-summary
func (s *SessionService) GetEscapeAnalyticsGlobalSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetEscapeAnalyticsGlobalSummaryRequest],
) (*connect.Response[sessionv1.GetEscapeAnalyticsGlobalSummaryResponse], error) {
	if s.analyticsClient == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("escape analytics not available"))
	}

	var timeFilters []predicate.EscapeEvent
	if req.Msg.StartTime != nil {
		timeFilters = append(timeFilters, escapeevent.WallTimeGTE(req.Msg.StartTime.AsTime()))
	}
	if req.Msg.EndTime != nil {
		timeFilters = append(timeFilters, escapeevent.WallTimeLTE(req.Msg.EndTime.AsTime()))
	}

	// Query 1: sequence-type histogram, GROUP BY sequence_type — real SQL aggregate.
	var histRows []struct {
		SequenceType string `json:"sequence_type"`
		Count        int64  `json:"count"`
		MangledCount int64  `json:"mangled_count"`
	}
	err := s.analyticsClient.EscapeEvent.Query().
		Where(timeFilters...).
		GroupBy(escapeevent.FieldSequenceType).
		Aggregate(
			ent.As(ent.Count(), "count"),
			ent.As(ent.Sum(escapeevent.FieldMangled), "mangled_count"),
		).
		Scan(ctx, &histRows)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Query 2: per-session breakdown, GROUP BY session_id — real SQL aggregate.
	var sessionRows []struct {
		SessionID    string `json:"session_id"`
		Count        int64  `json:"count"`
		MangledCount int64  `json:"mangled_count"`
	}
	err = s.analyticsClient.EscapeEvent.Query().
		Where(timeFilters...).
		GroupBy(escapeevent.FieldSessionID).
		Aggregate(
			ent.As(ent.Count(), "count"),
			ent.As(ent.Sum(escapeevent.FieldMangled), "mangled_count"),
		).
		Scan(ctx, &sessionRows)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Derive grand totals in Go from the (small, O(#sequence types)) histogram rows —
	// avoids a third round-trip. NOT the anti-pattern: histRows is bounded by distinct
	// sequence_type values, not by event count.
	var totalSeq, totalMangled int64
	histogram := make([]*sessionv1.EscapeSequenceCount, 0, len(histRows))
	for _, r := range histRows {
		histogram = append(histogram, &sessionv1.EscapeSequenceCount{
			SequenceType: r.SequenceType,
			Count:        r.Count,
			MangledCount: r.MangledCount,
		})
		totalSeq += r.Count
		totalMangled += r.MangledCount
	}

	perSession := make([]*sessionv1.SessionEscapeSummary, 0, len(sessionRows))
	for _, r := range sessionRows {
		var rate float64
		if r.Count > 0 {
			rate = float64(r.MangledCount) / float64(r.Count)
		}
		perSession = append(perSession, &sessionv1.SessionEscapeSummary{
			SessionId:      r.SessionID,
			TotalSequences: r.Count,
			TotalMangled:   r.MangledCount,
			MangleRate:     rate,
		})
	}

	var mangleRate float64
	if totalSeq > 0 {
		mangleRate = float64(totalMangled) / float64(totalSeq)
	}

	return connect.NewResponse(&sessionv1.GetEscapeAnalyticsGlobalSummaryResponse{
		Histogram:      histogram,
		TotalSequences: totalSeq,
		TotalMangled:   totalMangled,
		MangleRate:     mangleRate,
		PerSession:     perSession,
	}), nil
}
```

Notes on the sketch:
- `ent.As(ent.Sum(escapeevent.FieldMangled), "mangled_count")` relies on the SQLite bool-as-0/1 storage confirmed above. If this codebase ever migrated off SQLite to a dialect where `bool` isn't 0/1-castable in `SUM`, this specific aggregate would need revisiting — call this out as a footnote in the plan doc, not a blocker now.
- The anonymous scan-target struct pattern (`[]struct{...}` with `json` tags matching column aliases) is exactly what ent's own doc comment for `GroupBy` shows (`session/ent/escapeevent_query.go:264-267`) — `sql.ScanSlice` uses the struct tags to map result columns.
- Both queries share the identical `timeFilters` slice — matches the existing `GetEscapeAnalyticsSummary`'s pattern of applying the same `StartTime`/`EndTime` predicates to `query.Where(...)`.

## 3. Architectural pattern: read-only aggregate query

This is a straightforward **read-only aggregate query** pattern, not a new architectural concept for the repo:
- No new persistence, no schema change — `escape_event` schema (`session/ent/schema/escape_event.go`) already has `session_id`, `sequence_type`, `mangled` as indexed fields (see `Indexes()`: `index.Fields("session_id")`, `index.Fields("sequence_type")`, `index.Fields("mangled")`, `index.Fields("wall_time")`). The two `GROUP BY` queries above can use these existing indexes — no new index needed for the histogram (`sequence_type`) or per-session (`session_id`) grouping keys, and the optional time-range predicate hits the `wall_time` index.
- One caveat: there's no composite index covering `(sequence_type, mangled)` or `(session_id, mangled)` together, but with SQLite and this data volume (dev-tool scale, not production OLAP) a full-table scan filtered by `wall_time` range and grouped is expected to be fast; this is explicitly out of scope per the requirements' Non-functional section ("no specific target given").
- Follows the exact same shape as the sibling read-only aggregate `GetEscapeAnalyticsSummary` (`server/services/analytics_escape_service.go:124-187`) it's extending — same file, same `SessionService` receiver, same `s.analyticsClient` field, same error-handling conventions (`connect.CodeUnavailable` if `analyticsClient == nil`, `connect.CodeInternal` wrapping ent errors).

## 4. Integration points

- **Handler location**: `server/services/analytics_escape_service.go` — add `GetEscapeAnalyticsGlobalSummary` as a third method alongside `QueryEscapeAnalytics` and `GetEscapeAnalyticsSummary`, using the shared `// +api: escape:global-summary` marker convention (see `.claude/rules/feature-registry.md`).
- **Client wiring**: no change needed to `SetAnalyticsClient` (`server/services/session_service.go:721-725`) — the same `*ent.Client` on `SessionService.analyticsClient` serves this new method; escape events are already a single cross-session table (`session_id` is a plain field, not a per-client/per-DB boundary), so "global" here literally means "no `Where(escapeevent.SessionID(...))` predicate" rather than any new data-access wiring.
- **Proto**: extend `proto/session/v1/session.proto` — new RPC `GetEscapeAnalyticsGlobalSummary(GetEscapeAnalyticsGlobalSummaryRequest) returns (GetEscapeAnalyticsGlobalSummaryResponse)` near the existing escape analytics RPCs (`session.proto:330-334`). New request message mirrors `GetEscapeAnalyticsSummaryRequest` minus `session_id` (just `start_time`/`end_time`). New response message mirrors `GetEscapeAnalyticsSummaryResponse` (`histogram`, `total_sequences`, `total_mangled`, `mangle_rate`) plus a new `repeated SessionEscapeSummary per_session` field; new `SessionEscapeSummary` message shape: `{string session_id; int64 total_sequences; int64 total_mangled; double mangle_rate;}` — same shape as the existing `GetEscapeAnalyticsSummaryResponse`'s top-level fields, just keyed per session. Run `make proto-gen` after editing.
- **Frontend**: `web-app/src/components/analytics/EscapeAnalyticsPage.tsx` gets the "Per-Session | All Sessions" toggle per the requirements; a new hook (e.g. `useEscapeAnalyticsGlobalSummary`) parallel to the existing `useEscapeAnalyticsSummary`/`useEscapeEvents` hooks, gated so switching to "All Sessions" suspends the per-session hooks' requests (per the requirements' explicitly named Rabbit Hole) rather than just hiding their rendered output.

## 5. Data flow / consistency requirements

- **Eventual consistency is acceptable.** This is a local dev-tool read view over an already-async-written analytics table — `AnalyticsStore` (referenced via `s.GetAnalyticsStore()` in `session_service.go:697-704`) has its own flush loop, meaning escape events are already written to the `EscapeEvent` table on some delay from the moment they're observed in a session's PTY stream, not synchronously. The existing per-session `GetEscapeAnalyticsSummary` already tolerates this same lag; the global endpoint inherits the identical consistency characteristics — no new consistency requirement is introduced by aggregating across sessions instead of within one.
- **No cross-request consistency needed either**: the two `GROUP BY` queries (histogram, per-session) run as two separate round-trips, not inside a transaction. A dev-tool aggregate snapshot view has no correctness requirement that both queries observe the exact same instant of table state — a few events landing between the two queries (worst case, on an actively-running session mid-observation) would only produce a totals mismatch on the order of single-digit events, invisible at dev-tool usage scale and self-correcting on next refresh. Do not add transactional wrapping (e.g. `ent.Tx`) for this — it would add complexity with no requirement driving it.

## Summary of open questions from requirements.md — now resolved

- **"Exact ent mechanism for GROUP BY"** (Open Questions #1): resolved above — it's ent's own generated `GroupBy(...).Aggregate(...).Scan(...)` builder, not a raw-SQL escape hatch. No `sql.Selector`/`Modify` needed for this feature.
- **"One query or two for histogram vs per-session breakdown"**: two queries, same predicate, different `GroupBy` key — see §2.
- **"Per-session breakdown pagination"**: out of scope for this research doc (per requirements, deferred to Phase 3 planning), but worth flagging: the per-session `GROUP BY session_id` query already only returns one row per session *with events* (not all sessions), bounding the row count naturally without needing a `LIMIT`/pagination for realistic dev-tool session counts.
