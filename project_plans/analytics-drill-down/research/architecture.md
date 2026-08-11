# Agent 3 — Architecture Research

## Ent Schema Pattern

Directory: `session/ent/schema/` (17 schema files)

Each schema file follows the same pattern:
```go
type ClassificationAnalytics struct { ent.Schema }

func (ClassificationAnalytics) Fields() []ent.Field { ... }
func (ClassificationAnalytics) Edges() []ent.Edge { ... }
func (ClassificationAnalytics) Indexes() []ent.Index { ... }
```

Adding a compound index is purely additive — append to the `Indexes()` slice, run
the generate command, and ent handles the migration via `AutoMigrate`.

### Index addition for AC-2

In `session/ent/schema/classificationanalytics.go`, add to `Indexes()`:

```go
index.Fields("command_program", "created_at"),
```

This enables efficient `WHERE command_program = ? AND created_at >= ?` queries.
The existing `created_at` single-field index covers AC-2 (partial) and the `LoadWindow`
WHERE clause. Both indexes are needed for different query shapes.

## Ent Query Patterns (from ent_repository.go)

The existing `ListAnalytics` at line 1117 uses:
```go
r.client.ClassificationAnalytics.Query().
    Order(ent.Desc(classificationanalytics.FieldCreatedAt)).
    Limit(limit).
    All(ctx)
```

Pattern for time-windowed query (AC-1 / AC-3):
```go
r.client.ClassificationAnalytics.Query().
    Where(classificationanalytics.CreatedAtGTE(since)).
    Order(ent.Desc(classificationanalytics.FieldCreatedAt)).
    All(ctx)
```

Pattern for program+time filter (AC-3):
```go
r.client.ClassificationAnalytics.Query().
    Where(
        classificationanalytics.CommandProgram(program),
        classificationanalytics.CreatedAtGTE(since),
    ).
    Order(ent.Desc(classificationanalytics.FieldCreatedAt)).
    Limit(limit).
    All(ctx)
```

### Note on ent predicates

The generated predicate functions live in `session/ent/classificationanalytics/where.go`.
After schema generation, `CreatedAtGTE`, `CommandProgramEQ`, etc. will be available.
Use `classificationanalytics.CreatedAtGTE(since)` — not raw SQL.

### Aggregation queries (AC-4, AC-6)

Ent supports `GroupBy().Aggregate()` for SQL-level aggregations. The pattern for
subcommand breakdown (AC-4):

```go
var breakdown []struct {
    Subcommand string
    Decision   string
    Count      int
}
r.client.ClassificationAnalytics.Query().
    Where(
        classificationanalytics.CommandProgram(program),
        classificationanalytics.CreatedAtGTE(since),
    ).
    GroupBy(
        classificationanalytics.FieldCommandSubcategory,
        classificationanalytics.FieldDecision,
    ).
    Aggregate(ent.Count()).
    Scan(ctx, &breakdown)
```

For trend (AC-6), SQL-level grouping by date is harder with ent's ORM layer. Two options:
1. Use ent's raw SQL via `r.client.ClassificationAnalytics.Query().sqlQuery(ctx)` — more complex
2. Fetch filtered rows (program + subcommand + since) then run Go-level date bucketing
   using existing `ComputeDailyBuckets` logic

Option 2 is simpler and consistent with existing `ComputeDailyBuckets`. Performance is
acceptable because the compound index limits the scan to one program's rows in the window.

## Repository Interface Extensions

New methods to add to `session/repository.go` (Repository interface):

```go
// ListAnalyticsSince retrieves analytics entries created at or after since.
// Implements AC-1 and AC-2 (time-windowed query with DB-level WHERE).
ListAnalyticsSince(ctx context.Context, since time.Time, limit int) ([]AnalyticsData, error)

// ListAnalyticsByProgramSince retrieves entries for a specific program since a time.
// Implements AC-3.
ListAnalyticsByProgramSince(ctx context.Context, program string, since time.Time, limit int) ([]AnalyticsData, error)

// GetSubcommandBreakdown returns per-(subcommand, decision) counts for a program.
// Implements AC-4.
GetSubcommandBreakdown(ctx context.Context, program string, since time.Time) ([]SubcommandDecisionCount, error)

// ListRecentCommandsByProgram returns the most recent command_preview strings.
// Implements AC-5.
ListRecentCommandsByProgram(ctx context.Context, program, subcommand string, since time.Time, n int) ([]string, error)

// GetSubcommandTrend returns per-day counts for a (program, subcommand) pair.
// Implements AC-6.
GetSubcommandTrend(ctx context.Context, program, subcommand string, since time.Time) ([]AnalyticsData, error)
```

New domain types (add to `repository.go`):
```go
// SubcommandDecisionCount is a (subcommand, decision) pair with count.
type SubcommandDecisionCount struct {
    Subcommand string
    Decision   string
    Count      int
}
```

## AnalyticsStore Extensions

`LoadWindow` in `analytics_store.go` should be changed to call `ListAnalyticsSince`
instead of `ListAnalytics(ctx, 0)`:

```go
func (s *AnalyticsStore) LoadWindow(since time.Time) ([]AnalyticsEntry, error) {
    data, err := s.storage.ListAnalyticsSince(context.Background(), since, 0)
    ...
    // Remove the in-Go date filter — the DB now handles it.
    return convertAll(data), nil
}
```

New methods on `AnalyticsStore` (thin wrappers over the new repo methods):

```go
func (s *AnalyticsStore) LoadProgramWindow(program string, since time.Time) ([]AnalyticsEntry, error)
func (s *AnalyticsStore) GetSubcommandBreakdown(program string, since time.Time) ([]SubcommandDecisionCount, error)
func (s *AnalyticsStore) ListRecentCommands(program, subcommand string, since time.Time, n int) ([]string, error)
```

## Service Layer (RulesService)

New handler in `rules_service.go`:

```go
func (rs *RulesService) GetProgramAnalytics(
    ctx context.Context,
    req *connect.Request[sessionv1.GetProgramAnalyticsRequest],
) (*connect.Response[sessionv1.GetProgramAnalyticsResponse], error) {
    program := req.Msg.Program
    days := 7
    if req.Msg.WindowDays != nil { days = int(*req.Msg.WindowDays) }
    since := time.Now().AddDate(0, 0, -days)

    // AC-4: subcommand breakdown
    breakdown, _ := rs.analyticsStore.GetSubcommandBreakdown(program, since)

    // AC-5: recent examples (up to 20)
    examples, _ := rs.analyticsStore.storage.ListRecentCommandsByProgram(ctx, program, "", since, 20)

    // AC-6: trend (all rows for program in window, compute daily buckets in Go)
    entries, _ := rs.analyticsStore.LoadProgramWindow(program, since)
    buckets := ComputeDailyBuckets(entries)

    // Build response...
}
```

The rule coverage check for AC-10 (`has_rule_coverage`) can be computed in the service
layer by calling `rs.rulesStore.All()` and checking whether any rule's
`CommandProgram` + `CommandPattern` would match. This avoids a new DB query.

## Proto Changes

In `proto/session/v1/session.proto`:
1. Add `rpc GetProgramAnalytics(GetProgramAnalyticsRequest) returns (GetProgramAnalyticsResponse);`
   to the `SessionService` block

In `proto/session/v1/types.proto` (or session.proto message section):
2. Add `GetProgramAnalyticsRequest` message
3. Add `GetProgramAnalyticsResponse` message with `SubcommandBreakdownProto`
4. Add `SubcommandBreakdownProto` message

## Server Wiring

`SessionService` already delegates to `rulesSvc`. Add to `session_service.go`:

```go
func (s *SessionService) GetProgramAnalytics(
    ctx context.Context,
    req *connect.Request[sessionv1.GetProgramAnalyticsRequest],
) (*connect.Response[sessionv1.GetProgramAnalyticsResponse], error) {
    return s.rulesSvc.GetProgramAnalytics(ctx, req)
}
```

No changes to `server.go` needed — `SessionService` is already registered via
`sessionv1connect.NewSessionServiceHandler`.

## Implementation sequence

1. Add compound index to `classificationanalytics.go` schema
2. Run ent generate: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
3. Add new repo methods to `Repository` interface + `EntRepository`
4. Update `LoadWindow` to use `ListAnalyticsSince`
5. Add new `AnalyticsStore` wrapper methods
6. Add proto messages + RPC
7. Run `make generate-proto`
8. Implement `RulesService.GetProgramAnalytics`
9. Delegate from `SessionService`
10. Add frontend hook `useProgramAnalytics`
11. Add `ProgramDetailPanel` component
