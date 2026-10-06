# Research: Stack & Dependencies — escape-analytics-global-view

## Pinned versions in use (VERIFIED via go.mod / web-app/package.json)

| Component | Version |
|---|---|
| `entgo.io/ent` | v0.14.5 |
| `connectrpc.com/connect` | v1.19.0 |
| `connectrpc.com/otelconnect` | v0.8.0 |
| `google.golang.org/protobuf` | v1.36.11 |
| `github.com/mattn/go-sqlite3` | driver used by the analytics DB (see below) |
| `@connectrpc/connect` / `@connectrpc/connect-web` (web-app) | ^2.1.1 |
| `react` | ^19.0.0 |
| `@vanilla-extract/css` | ^1.20.1 |
| `typescript` | ^5.9.3 |

**No new dependencies are needed.** Everything required (ent's GroupBy/Aggregate builder, ConnectRPC, vanilla-extract) is already a project dependency at the versions above.

## Analytics DB is a separate SQLite ent client, not the main session DB

`server/analytics/db.go` opens a dedicated SQLite connection (`github.com/mattn/go-sqlite3`, DSN via `sql.Open("sqlite3", dsn)`) and wraps it with `entsql.OpenDB(dialect.SQLite, db)` → `ent.NewClient(ent.Driver(drv))`. This is exposed on `SessionService` as `s.analyticsClient` (used throughout `server/services/analytics_escape_service.go`). The new global endpoint will use this same `s.analyticsClient` — no new DB, no new client.

## ent GroupBy/Aggregate mechanism (resolves the requirements doc's "Rabbit Hole")

ent's code generator (v0.14.5, this repo's pinned version) **does** generate a first-class `GroupBy` + `Aggregate` builder per entity — confirmed present for `EscapeEvent` specifically:

- `session/ent/escapeevent_query.go:273` — `func (_q *EscapeEventQuery) GroupBy(field string, fields ...string) *EscapeEventGroupBy`
- `session/ent/escapeevent_query.go:303` — `func (_q *EscapeEventQuery) Aggregate(fns ...AggregateFunc) *EscapeEventSelect`
- `session/ent/escapeevent_query.go:446` — `func (_g *EscapeEventGroupBy) Aggregate(fns ...AggregateFunc) *EscapeEventGroupBy`
- `session/ent/escapeevent_query.go:452` — `func (_g *EscapeEventGroupBy) Scan(ctx context.Context, v any) error`

Aggregate helper functions live in `session/ent/ent.go:154-217`: `ent.Count()`, `ent.Sum(field)`, `ent.Max(field)`, `ent.Min(field)`, `ent.Mean(field)`, and `ent.As(fn, alias)` for renaming the aggregated column. These compile to real SQL `GROUP BY` + aggregate functions (`sql.Count("*")`, `sql.Sum(...)`, etc.) via ent's underlying `*sql.Selector` — this is a genuine `GROUP BY ... COUNT(...)` at the database layer, not client-side aggregation, satisfying the NFR in `project_plans/escape-analytics-global-view/requirements.md` (lines 26, 40).

**No use of ent's `GroupBy`/`Aggregate` builder exists anywhere else in the codebase yet** (`grep -rn "\.GroupBy(" server/ session/ pkg/` outside `/ent/`-generated files returns nothing) — this feature will be the first real call site. There is precedent for the raw `sql.Selector` escape hatch pattern in general ent usage across the repo, but the generated `GroupBy`/`Aggregate`/`Scan` chain is sufficient here and is the simpler, idiomatic-for-this-ent-version path; falling back to a hand-written `sql.Selector` is not needed.

### Recommended query shape

Two separate `GroupBy` queries (as the requirements doc's rabbit hole anticipates), both against `s.analyticsClient.EscapeEvent.Query()` with the same `WallTimeGTE`/`WallTimeLTE` optional filters used in the existing `GetEscapeAnalyticsSummary` (`server/services/analytics_escape_service.go:139-144`):

```go
// Histogram: GROUP BY sequence_type, mangled -> COUNT(*)
type histoRow struct {
    SequenceType string `json:"sequence_type"`
    Mangled      bool   `json:"mangled"`
    Count        int64  `json:"count"`
}
var rows []histoRow
err := s.analyticsClient.EscapeEvent.Query().
    Where(filters...).
    GroupBy(escapeevent.FieldSequenceType, escapeevent.FieldMangled).
    Aggregate(ent.As(ent.Count(), "count")).
    Scan(ctx, &rows)

// Per-session breakdown: GROUP BY session_id, mangled -> COUNT(*)
type sessionRow struct {
    SessionID string `json:"session_id"`
    Mangled   bool   `json:"mangled"`
    Count     int64  `json:"count"`
}
var sessionRows []sessionRow
err = s.analyticsClient.EscapeEvent.Query().
    Where(filters...).
    GroupBy(escapeevent.FieldSessionID, escapeevent.FieldMangled).
    Aggregate(ent.As(ent.Count(), "count")).
    Scan(ctx, &sessionRows)
```

Both queries fold `mangled` into the `GROUP BY` rather than issuing a second query — a single grouped result set with `mangled bool` as a group key lets Go-side code split each row into "total" vs "mangled" counts per group with one pass, avoiding both an extra round trip and the in-Go-aggregation anti-pattern (only trivial post-aggregation summation over already-grouped rows, not raw-row iteration). Total sequences/total mangled/mangle-rate for the global summary can be derived by summing the histogram query's rows in Go (summing ≤ a few dozen aggregated rows, not raw events) — or with a third simple `Aggregate(ent.Count())`/`Where(escapeevent.Mangled(true))` `Count(ctx)` pair if a cleaner separation is preferred. Confirm the exact split in Phase 3 planning per the requirements doc's rabbit hole note.

## ConnectRPC pattern (unchanged from existing per-session RPCs)

Standard handler signature already used throughout `server/services/analytics_escape_service.go`:
```go
func (s *SessionService) GetEscapeAnalyticsGlobalSummary(
    ctx context.Context,
    req *connect.Request[sessionv1.GetEscapeAnalyticsGlobalSummaryRequest],
) (*connect.Response[sessionv1.GetEscapeAnalyticsGlobalSummaryResponse], error) {
```
No `session_id` guard (global scope), but reuse the `s.analyticsClient == nil → CodeUnavailable` guard and the same `StartTime`/`EndTime` optional-filter pattern. Requires a `// +api: escape:global-summary`-style marker per `.claude/rules/feature-registry.md`, new proto messages in `proto/session/v1/session.proto` + `make proto-gen` (no proto version change — this repo's protobuf toolchain is already pinned at `google.golang.org/protobuf v1.36.11`).

## Frontend: no new libraries

- No `react-query`/`@tanstack/react-query` in the codebase (`@tanstack/react-virtual` is unrelated — used for virtualized lists). Session data-fetching uses hand-rolled hooks (`useEscapeAnalyticsSummary`, `useEscapeEvents` in `web-app/src/lib/hooks/useEscapeAnalytics.ts`) directly over `@connectrpc/connect-web` (^2.1.1) — the new "All Sessions" toggle should add a sibling hook (e.g. `useEscapeAnalyticsGlobalSummary`) following the exact same pattern, not introduce a data-fetching library.
- Styling: vanilla-extract (`^1.20.1`) per `.claude/rules/css-architecture.md` — the new tab/toggle UI goes in a colocated `EscapeAnalyticsPage.css.ts` (or extends the existing one), using `vars.*` tokens, no hardcoded values.
- React version is 19.0.0 — no compatibility concerns for a simple tab/toggle (no need for new Suspense/use() patterns; existing per-session page already establishes the state-management style to mirror).

## Summary of what to build (no new deps)

1. Proto: new `GetEscapeAnalyticsGlobalSummaryRequest`/`Response` messages + RPC in `proto/session/v1/session.proto`, `make proto-gen`.
2. Go handler in `server/services/analytics_escape_service.go` using `EscapeEventQuery.GroupBy(...).Aggregate(ent.As(ent.Count(), "count")).Scan(...)` — real SQL `GROUP BY`, first use of this ent builder in the repo.
3. Frontend hook mirroring `useEscapeAnalyticsSummary`'s existing pattern (no new library), plus a tab/toggle in `EscapeAnalyticsPage.tsx` styled via vanilla-extract.
