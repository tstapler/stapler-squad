# Analytics Drill-Down — Implementation Plan

**Project**: `analytics-drill-down`
**Date**: 2026-05-19
**Covers**: AC-1 through AC-15 from requirements.md

---

## Summary

4 epics, 14 stories, 36 tasks. All changes are additive (no breaking schema migrations,
no existing proto field renumbering, no removal of existing repo methods).

---

## Epic 1 — DB Schema + Indexes

**Goal**: AC-2. Add the compound index `(command_program, created_at)` so program-scoped
time-windowed queries hit the covering index instead of the existing `idx_created_at`
single-field index. The single-field `created_at` index already satisfies the bare
`WHERE created_at >= ?` case; the compound index is needed for
`WHERE command_program = ? AND created_at >= ?`.

---

### Story 1.1 — Add compound index to ent schema

**Files to modify**:
- `session/ent/schema/classificationanalytics.go`

**Task 1.1.1 — Append compound index**

In the `Indexes()` method, append after the existing five single-field indexes:

```go
index.Fields("command_program", "created_at"),
```

The full `Indexes()` return slice becomes:

```go
return []ent.Index{
    index.Fields("session_id"),
    index.Fields("decision"),
    index.Fields("risk_level"),
    index.Fields("rule_id"),
    index.Fields("created_at"),
    index.Fields("command_program", "created_at"), // NEW: AC-2
}
```

No other fields change in this file.

**Test approach**: After `make build`, start the server once against the dev database and
confirm `sqlite3 ~/.stapler-squad/*/db.sqlite ".indexes classification_analytics"` shows
`classification_analytics_command_program_created_at`.

---

### Story 1.2 — Regenerate ent client

**Task 1.2.1 — Run ent generate**

```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

The `--feature sql/upsert` flag is mandatory (documented in CLAUDE.md and
`session/ent/generate.go`). Omitting it silently removes `OnConflict*` methods,
breaking `RecordAnalytics`.

**Task 1.2.2 — Verify build**

```bash
go build ./...
```

**Task 1.2.3 — Commit all session/ent/ changes together**

All files under `session/ent/` must be committed in one commit to keep the generated
client in sync with the schema. Partial commits leave the migration and client out of sync.

**Test approach**: `make build && make test` — existing tests that call `RecordAnalytics`
(which uses `OnConflictColumns`) must pass, proving `sql/upsert` feature was preserved.

---

## Epic 2 — Repository Layer

**Goal**: AC-1, AC-3 through AC-6. Add four new repository methods, delegate them through
`session.Storage`, and fix `AnalyticsStore.LoadWindow` to use the time-windowed query.

---

### Story 2.1 — New domain type

**Files to modify**:
- `session/repository.go`

**Task 2.1.1 — Add SubcommandDecisionCount type**

After the existing `AnalyticsData` struct definition, add:

```go
// SubcommandDecisionCount holds a (subcommand, decision) aggregate count.
// Returned by GetSubcommandBreakdown.
type SubcommandDecisionCount struct {
    Subcommand string
    Decision   string
    Count      int
}
```

The empty-string subcommand (`""`) is valid — it represents commands with no
subcommand token. The service layer filters or labels it as "(none)".

---

### Story 2.2 — Repository interface additions

**Files to modify**:
- `session/repository.go`

**Task 2.2.1 — Add five methods to the Repository interface**

In the `Repository` interface, after the existing `ListAnalytics` declaration:

```go
// ListAnalyticsSince retrieves analytics entries with created_at >= since.
// Replaces the in-Go date filter in LoadWindow. Implements AC-1.
// Pass limit=0 for no limit.
ListAnalyticsSince(ctx context.Context, since time.Time, limit int) ([]AnalyticsData, error)

// ListAnalyticsByProgramSince retrieves entries for a specific program since a time.
// Uses the compound index (command_program, created_at). Implements AC-3.
// Pass limit=0 for no limit.
ListAnalyticsByProgramSince(ctx context.Context, program string, since time.Time, limit int) ([]AnalyticsData, error)

// GetSubcommandBreakdown returns per-(subcommand, decision) counts for a program
// in the given time window. Uses SQL GROUP BY via ent Aggregate. Implements AC-4.
GetSubcommandBreakdown(ctx context.Context, program string, since time.Time) ([]SubcommandDecisionCount, error)

// ListRecentCommandsByProgram returns the most recent n command_preview strings
// for (program, subcommand). Pass subcommand="" to match all subcommands.
// Implements AC-5.
ListRecentCommandsByProgram(ctx context.Context, program, subcommand string, since time.Time, n int) ([]string, error)

// GetSubcommandTrend returns raw analytics rows for (program, subcommand) since
// a given time. The caller buckets these using ComputeDailyBuckets. Implements AC-6.
// Pass subcommand="" to match all subcommands for the program.
GetSubcommandTrend(ctx context.Context, program, subcommand string, since time.Time) ([]AnalyticsData, error)
```

**Test approach**: The interface change forces a compile-time failure if any existing
`Repository` implementation (mock or real) doesn't implement the new methods. Running
`go build ./...` after adding the interface stubs (but before adding implementations)
will show which types need updating — use this list to ensure no mock is missed.

---

### Story 2.3 — EntRepository implementations

**Files to modify**:
- `session/ent_repository.go`

All five new methods go in this file, grouped with the existing `RecordAnalytics` and
`ListAnalytics` implementations around line 1117.

**Pre-task — Extract convertAnalyticsEntries helper**

The existing `ListAnalytics` function contains an inline for-loop that maps
`*ent.ClassificationAnalytics` to `AnalyticsData`. Before adding new methods, extract
this mapping into shared private helpers:

```go
// convertAnalyticsEntry maps a single ent row to the domain model.
func convertAnalyticsEntry(e *ent.ClassificationAnalytics) AnalyticsData {
    return AnalyticsData{
        ID:                 e.AnalyticsID,
        SessionID:          e.SessionID,
        ToolName:           e.ToolName,
        CommandPreview:     e.CommandPreview,
        Cwd:                e.Cwd,
        Decision:           e.Decision,
        RiskLevel:          e.RiskLevel,
        RuleID:             e.RuleID,
        RuleName:           e.RuleName,
        Reason:             e.Reason,
        Alternative:        e.Alternative,
        DurationMs:         e.DurationMs,
        ApprovalID:         e.ApprovalID,
        CommandProgram:     e.CommandProgram,
        CommandCategory:    e.CommandCategory,
        CommandSubcategory: e.CommandSubcategory,
        PythonImports:      e.PythonImports,
        CreatedAt:          e.CreatedAt,
    }
}

// convertAnalyticsEntries maps a slice of ent rows to the domain model.
func convertAnalyticsEntries(es []*ent.ClassificationAnalytics) []AnalyticsData {
    out := make([]AnalyticsData, len(es))
    for i, e := range es {
        out[i] = convertAnalyticsEntry(e)
    }
    return out
}
```

Then update the existing `ListAnalytics` to call `convertAnalyticsEntries(entries)`
instead of the inline loop. All five new methods then call this helper.

**Note**: Verify the exact field names on `*ent.ClassificationAnalytics` by reading the
generated `session/ent/classificationanalytics.go` — field accessors are capitalized
camel-case of the schema field names (e.g., `e.AnalyticsID` for `analytics_id`,
`e.CommandSubcategory` for `command_subcategory`).

**Task 2.3.1 — ListAnalyticsSince**

```go
func (r *EntRepository) ListAnalyticsSince(ctx context.Context, since time.Time, limit int) ([]AnalyticsData, error) {
    query := r.client.ClassificationAnalytics.Query().
        Where(classificationanalytics.CreatedAtGTE(since)).
        Order(ent.Desc(classificationanalytics.FieldCreatedAt))
    if limit > 0 {
        query = query.Limit(limit)
    }
    entries, err := query.All(ctx)
    if err != nil {
        return nil, fmt.Errorf("list analytics since %s: %w", since.Format(time.RFC3339), err)
    }
    return convertAnalyticsEntries(entries), nil
}
```

Uses `classificationanalytics.CreatedAtGTE` predicate (generated by ent, available after
schema regeneration). The existing `idx_created_at` single-field index covers this query.

**Task 2.3.2 — ListAnalyticsByProgramSince**

```go
func (r *EntRepository) ListAnalyticsByProgramSince(ctx context.Context, program string, since time.Time, limit int) ([]AnalyticsData, error) {
    query := r.client.ClassificationAnalytics.Query().
        Where(
            classificationanalytics.CommandProgramEQ(program),
            classificationanalytics.CreatedAtGTE(since),
        ).
        Order(ent.Desc(classificationanalytics.FieldCreatedAt))
    if limit > 0 {
        query = query.Limit(limit)
    }
    entries, err := query.All(ctx)
    if err != nil {
        return nil, fmt.Errorf("list analytics by program %q since %s: %w", program, since.Format(time.RFC3339), err)
    }
    return convertAnalyticsEntries(entries), nil
}
```

The compound index `(command_program, created_at)` added in Epic 1 makes this efficient.
`CommandProgramEQ` is the exact-match predicate generated by ent. Note: if `program` is
empty string, this returns rows with `command_program = ""` — the caller should validate
input. The service layer guards against empty program.

**Task 2.3.3 — GetSubcommandBreakdown (SQL GROUP BY via ent)**

```go
func (r *EntRepository) GetSubcommandBreakdown(ctx context.Context, program string, since time.Time) ([]SubcommandDecisionCount, error) {
    type breakdownRow struct {
        CommandSubcategory string `json:"command_subcategory"`
        Decision           string `json:"decision"`
        Count              int    `json:"count"`
    }
    var rows []breakdownRow
    err := r.client.ClassificationAnalytics.Query().
        Where(
            classificationanalytics.CommandProgramEQ(program),
            classificationanalytics.CreatedAtGTE(since),
        ).
        GroupBy(
            classificationanalytics.FieldCommandSubcategory,
            classificationanalytics.FieldDecision,
        ).
        Aggregate(ent.Count()).
        Scan(ctx, &rows)
    if err != nil {
        return nil, fmt.Errorf("subcommand breakdown for %q: %w", program, err)
    }
    result := make([]SubcommandDecisionCount, 0, len(rows))
    for _, row := range rows {
        result = append(result, SubcommandDecisionCount{
            Subcommand: row.CommandSubcategory,
            Decision:   row.Decision,
            Count:      row.Count,
        })
    }
    return result, nil
}
```

**Critical ent pitfall**: The `Scan` destination struct fields must use `json` tags matching
the SQL column names exactly (`command_subcategory`, `decision`). Ent's `GroupBy().Scan()`
populates the struct via SQL column aliases derived from the field constant strings. The
`Count` field gets the value from `ent.Count()` — its json tag must be `"count"`.

NULL values in `command_subcategory` will scan as `""`. This is handled in the service
layer (Story 3.2, Task 3.2.1).

**Task 2.3.4 — ListRecentCommandsByProgram**

```go
func (r *EntRepository) ListRecentCommandsByProgram(ctx context.Context, program, subcommand string, since time.Time, n int) ([]string, error) {
    predicates := []predicate.ClassificationAnalytics{
        classificationanalytics.CommandProgramEQ(program),
        classificationanalytics.CreatedAtGTE(since),
        classificationanalytics.CommandPreviewNotNil(),
    }
    if subcommand != "" {
        predicates = append(predicates, classificationanalytics.CommandSubcategoryEQ(subcommand))
    }
    entries, err := r.client.ClassificationAnalytics.Query().
        Where(predicates...).
        Order(ent.Desc(classificationanalytics.FieldCreatedAt)).
        Limit(n).
        Select(classificationanalytics.FieldCommandPreview).
        All(ctx)
    if err != nil {
        return nil, fmt.Errorf("recent commands for %q/%q: %w", program, subcommand, err)
    }
    previews := make([]string, 0, len(entries))
    for _, e := range entries {
        if e.CommandPreview != "" {
            previews = append(previews, e.CommandPreview)
        }
    }
    return previews, nil
}
```

When `subcommand == ""`, no subcommand predicate is added — all subcommands for the
program are included. `CommandPreviewNotNil()` ensures we only fetch rows with a non-NULL
preview. The `Select` call reduces data transfer (only fetches `command_preview` column).

**Task 2.3.5 — GetSubcommandTrend**

```go
func (r *EntRepository) GetSubcommandTrend(ctx context.Context, program, subcommand string, since time.Time) ([]AnalyticsData, error) {
    predicates := []predicate.ClassificationAnalytics{
        classificationanalytics.CommandProgramEQ(program),
        classificationanalytics.CreatedAtGTE(since),
    }
    if subcommand != "" {
        predicates = append(predicates, classificationanalytics.CommandSubcategoryEQ(subcommand))
    }
    entries, err := r.client.ClassificationAnalytics.Query().
        Where(predicates...).
        Order(ent.Asc(classificationanalytics.FieldCreatedAt)).
        All(ctx)
    if err != nil {
        return nil, fmt.Errorf("subcommand trend for %q/%q: %w", program, subcommand, err)
    }
    return convertAnalyticsEntries(entries), nil
}
```

Returns raw rows; `ComputeDailyBuckets` in `analytics_store.go` does the bucketing in Go.
Ordered ascending so buckets are already chronological. The compound index covers this
query efficiently (even the subcommand-filtered case narrows further in Go after the
program+date index scan).

**Note on `convertAnalyticsEntries`**: This is the existing private helper in
`ent_repository.go` that maps `[]*ent.ClassificationAnalytics` to `[]AnalyticsData`.
Reuse it directly — do not duplicate the field-mapping logic.

**Test approach**:
- Unit tests in `session/ent_repository_test.go` (or a new `ent_repository_analytics_test.go`)
- Use the real SQLite in-memory ent test client (same pattern as existing ent tests)
- Seed 10–20 rows with varied programs, subcommands, decisions, and timestamps
- Assert `GetSubcommandBreakdown` returns correct counts per (subcommand, decision)
- Assert `ListAnalyticsSince` excludes rows before `since`
- Assert `ListRecentCommandsByProgram` with `subcommand=""` returns all subcommands
- Assert `ListRecentCommandsByProgram` with a specific subcommand is filtered
- Assert `GetSubcommandTrend` returns rows in ascending created_at order
- Edge case: empty `command_preview` (NULL) rows are excluded from `ListRecentCommandsByProgram`
- Edge case: program with no rows returns empty slice, nil error from all methods

---

### Story 2.4 — session.Storage delegation

**Files to modify**:
- `session/storage.go`

**Task 2.4.1 — Add five delegation methods to Storage**

After the existing `ListAnalytics` delegation method (around line 399):

```go
// ListAnalyticsSince retrieves analytics entries with created_at >= since.
func (s *Storage) ListAnalyticsSince(ctx context.Context, since time.Time, limit int) ([]AnalyticsData, error) {
    return s.repo.ListAnalyticsSince(ctx, since, limit)
}

// ListAnalyticsByProgramSince retrieves entries for a specific program since a time.
func (s *Storage) ListAnalyticsByProgramSince(ctx context.Context, program string, since time.Time, limit int) ([]AnalyticsData, error) {
    return s.repo.ListAnalyticsByProgramSince(ctx, program, since, limit)
}

// GetSubcommandBreakdown returns per-(subcommand, decision) counts for a program.
func (s *Storage) GetSubcommandBreakdown(ctx context.Context, program string, since time.Time) ([]SubcommandDecisionCount, error) {
    return s.repo.GetSubcommandBreakdown(ctx, program, since)
}

// ListRecentCommandsByProgram returns the most recent n command_preview strings.
func (s *Storage) ListRecentCommandsByProgram(ctx context.Context, program, subcommand string, since time.Time, n int) ([]string, error) {
    return s.repo.ListRecentCommandsByProgram(ctx, program, subcommand, since, n)
}

// GetSubcommandTrend returns raw analytics rows for (program, subcommand) since a time.
func (s *Storage) GetSubcommandTrend(ctx context.Context, program, subcommand string, since time.Time) ([]AnalyticsData, error) {
    return s.repo.GetSubcommandTrend(ctx, program, subcommand, since)
}
```

**Test approach**: These are one-liner delegations; they are implicitly tested by the
service-layer integration tests (Story 3.2).

---

### Story 2.5 — Fix AnalyticsStore.LoadWindow + new store wrappers

**Files to modify**:
- `server/services/analytics_store.go`

**Task 2.5.1 — Fix LoadWindow to call ListAnalyticsSince**

Replace:

```go
data, err := s.storage.ListAnalytics(context.Background(), 0)
if err != nil {
    return nil, fmt.Errorf("list analytics from DB: %w", err)
}

var entries []AnalyticsEntry
for _, d := range data {
    if !d.CreatedAt.Before(since) {
        entries = append(entries, AnalyticsEntry{ ... })
    }
}
return entries, nil
```

With:

```go
data, err := s.storage.ListAnalyticsSince(context.Background(), since, 0)
if err != nil {
    return nil, fmt.Errorf("list analytics since %s from DB: %w", since.Format(time.RFC3339), err)
}

entries := make([]AnalyticsEntry, 0, len(data))
for _, d := range data {
    entries = append(entries, analyticsDataToEntry(d))
}
return entries, nil
```

Remove the `if !d.CreatedAt.Before(since)` in-Go filter — the DB now filters. The
`analyticsDataToEntry` helper is the extracted field-mapping that was previously inline
in the for-loop (extract it to avoid repeating the 15-field mapping in every new method).

**Task 2.5.2 — Add LoadProgramWindow wrapper on AnalyticsStore**

```go
// LoadProgramWindow loads analytics entries for a specific program in the given window.
// Used by GetProgramAnalytics RPC (Epic 3) for trend computation.
func (s *AnalyticsStore) LoadProgramWindow(program string, since time.Time) ([]AnalyticsEntry, error) {
    data, err := s.storage.ListAnalyticsByProgramSince(context.Background(), program, since, 0)
    if err != nil {
        return nil, fmt.Errorf("list analytics by program %q since %s: %w", program, since.Format(time.RFC3339), err)
    }
    entries := make([]AnalyticsEntry, 0, len(data))
    for _, d := range data {
        entries = append(entries, analyticsDataToEntry(d))
    }
    return entries, nil
}
```

**Task 2.5.3 — Add GetSubcommandBreakdown wrapper on AnalyticsStore**

```go
// GetSubcommandBreakdown returns per-(subcommand, decision) aggregate counts.
func (s *AnalyticsStore) GetSubcommandBreakdown(program string, since time.Time) ([]session.SubcommandDecisionCount, error) {
    return s.storage.GetSubcommandBreakdown(context.Background(), program, since)
}
```

**Task 2.5.4 — Add ListRecentCommands wrapper on AnalyticsStore**

```go
// ListRecentCommands returns up to n command_preview strings for (program, subcommand).
// Pass subcommand="" to match all subcommands.
func (s *AnalyticsStore) ListRecentCommands(program, subcommand string, since time.Time, n int) ([]string, error) {
    return s.storage.ListRecentCommandsByProgram(context.Background(), program, subcommand, since, n)
}
```

**Test approach**:
- `TestAnalyticsStore_LoadWindow_UsesDBFilter` — seed rows at t-2h and t-48h, call
  `LoadWindow(t-24h)`, assert only t-2h row is returned. Confirms the in-Go filter is gone
  and DB-level WHERE is working.
- Existing tests in `approval_handler_secret_test.go` and `rules_service_test.go` must
  continue to pass without modification (they call `LoadWindow` with a real SQLite backend).

---

## Epic 3 — Proto + RPC

**Goal**: AC-7. Add `GetProgramAnalytics` RPC, implement it in `rules_service.go`, and
delegate from `session_service.go`.

---

### Story 3.1 — Proto changes

**Files to modify**:
- `proto/session/v1/types.proto`
- `proto/session/v1/session.proto`

**Task 3.1.1 — Add SubcommandBreakdownProto to types.proto**

After the `SubcommandStatProto` message (currently ends at line 1026), add:

```protobuf
// SubcommandBreakdownProto is a per-subcommand decision breakdown for the drill-down panel.
message SubcommandBreakdownProto {
  // subcommand is the first positional argument (e.g., "commit", "push").
  // Empty string means no subcommand was detected.
  string subcommand = 1;
  // total is the total call count for this subcommand in the window.
  int32 total = 2;
  // Per-decision counts.
  int32 auto_allow = 3;
  int32 auto_deny = 4;
  int32 escalate = 5;
  int32 manual_allow = 6;
  int32 manual_deny = 7;
  // has_rule_coverage is true if any existing rule covers this (program, subcommand) pair.
  bool has_rule_coverage = 8;
  // suggested_rule_hint is an optional pre-fill pattern hint for the rule form.
  // Format: the subcommand string itself (e.g., "push") — the UI appends context.
  string suggested_rule_hint = 9;
}
```

Field numbers start at 1 — this is a new message, no collision risk. All 9 fields are
defined; `bool has_rule_coverage` defaults to `false` in proto3. The service layer MUST
explicitly set this to `true` when coverage exists (do not rely on zero value).

**Task 3.1.2 — Add GetProgramAnalyticsRequest and GetProgramAnalyticsResponse to session.proto**

After the `GetApprovalAnalyticsResponse` message (currently ends at line 1226), add:

```protobuf
// ============================================================================
// Program Analytics Drill-Down Messages
// ============================================================================

message GetProgramAnalyticsRequest {
  // program is the executable name (e.g., "git", "gh", "npm").
  string program = 1;
  // window_days controls the time window (default 7, max 90).
  optional int32 window_days = 2;
}

message GetProgramAnalyticsResponse {
  // program echoed from the request.
  string program = 1;
  // category is the program's category (e.g., "vcs", "node").
  string category = 2;
  // subcommands contains per-subcommand decision breakdown, sorted by total descending.
  repeated SubcommandBreakdownProto subcommands = 3;
  // recent_examples contains the last 20 raw command_preview strings across all subcommands.
  repeated string recent_examples = 4;
  // trend contains per-day counts for the whole program in the window.
  // Reuses DailyBucketProto (already defined in types.proto).
  repeated DailyBucketProto trend = 5;
}
```

Field numbers start at 1 — new messages, no collision risk. `optional int32 window_days`
matches the existing proto3 optional pattern used by `GetApprovalAnalyticsRequest`.

**Note on RuleCoverageRows**: The requirements mention `RuleCoverageRows` in the response.
This is served by the `has_rule_coverage` bool on each `SubcommandBreakdownProto` row
(which rule covers it is implicit — the "Add rule →" link points to the rules page).
A separate `RuleCoverageRowProto` message type is NOT needed; adding it would duplicate
information already inferable from `subcommands`. If operators need to see the specific
rule names, that is a future enhancement (out of scope per requirements non-goals).

**Task 3.1.3 — Add GetProgramAnalytics RPC to session.proto service block**

After the `GetApprovalAnalytics` RPC (line 155), add:

```protobuf
  // GetProgramAnalytics returns drill-down analytics for a single command program.
  // Shows subcommand breakdown, recent examples, and daily trend for the time window.
  rpc GetProgramAnalytics(GetProgramAnalyticsRequest) returns (GetProgramAnalyticsResponse) {}
```

**Task 3.1.4 — Run make generate-proto**

```bash
make generate-proto
```

Regenerates:
- `session/gen/session/v1/*.go` (Go bindings)
- `web-app/src/gen/session/v1/*_pb.ts` (TypeScript bindings)

Verify no proto field numbering errors in the output. Run `go build ./...` after.

---

### Story 3.2 — Implement GetProgramAnalytics handler

**Files to modify**:
- `server/services/rules_service.go`
- `server/services/session_service.go`

**Task 3.2.1 — Implement GetProgramAnalytics in rules_service.go**

Add after `GetApprovalAnalytics` (around line 183):

```go
// GetProgramAnalytics returns drill-down analytics for a single program.
// Implements AC-7 (subcommand breakdown, examples, trend).
func (rs *RulesService) GetProgramAnalytics(
    ctx context.Context,
    req *connect.Request[sessionv1.GetProgramAnalyticsRequest],
) (*connect.Response[sessionv1.GetProgramAnalyticsResponse], error) {
    program := strings.TrimSpace(req.Msg.Program)
    if program == "" {
        return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("program is required"))
    }

    days := 7
    if req.Msg.WindowDays != nil {
        days = int(*req.Msg.WindowDays)
        if days <= 0 || days > 90 {
            return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("window_days must be 1–90"))
        }
    }
    since := time.Now().AddDate(0, 0, -days)

    // AC-4: subcommand breakdown (SQL GROUP BY)
    breakdownRows, err := rs.analyticsStore.GetSubcommandBreakdown(program, since)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("subcommand breakdown: %w", err))
    }

    // AC-5: recent examples (up to 20, all subcommands)
    examples, err := rs.analyticsStore.ListRecentCommands(program, "", since, 20)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("recent examples: %w", err))
    }

    // AC-6: trend (all rows for program in window → Go-level daily bucketing)
    entries, err := rs.analyticsStore.LoadProgramWindow(program, since)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("program window: %w", err))
    }
    dailyBuckets := ComputeDailyBuckets(entries)

    // Derive category from entries (use first non-empty value)
    category := ""
    for _, e := range entries {
        if e.CommandCategory != "" {
            category = e.CommandCategory
            break
        }
    }

    // Rule coverage check (AC-10 / has_rule_coverage)
    // Build a set of (program+subcommand) pairs covered by existing rules.
    // Uses in-memory rule specs — no additional DB query needed.
    coveredSubcmds := rs.coveredSubcommands(program)

    // Aggregate per-subcommand breakdown into proto messages
    type subKey struct{ sub, dec string }
    aggr := make(map[string]map[string]int32) // subcommand → decision → count
    for _, row := range breakdownRows {
        sub := row.Subcommand // may be ""
        if aggr[sub] == nil {
            aggr[sub] = make(map[string]int32)
        }
        aggr[sub][row.Decision] += int32(row.Count) // #nosec G115 — count fits int32
    }

    subProtos := make([]*sessionv1.SubcommandBreakdownProto, 0, len(aggr))
    for sub, decMap := range aggr {
        p := &sessionv1.SubcommandBreakdownProto{
            Subcommand:          sub,
            AutoAllow:           decMap["auto_allow"],
            AutoDeny:            decMap["auto_deny"],
            Escalate:            decMap["escalate"],
            ManualAllow:         decMap["manual_allow"],
            ManualDeny:          decMap["manual_deny"],
            HasRuleCoverage:     coveredSubcmds[sub],
            SuggestedRuleHint:   sub,
        }
        p.Total = p.AutoAllow + p.AutoDeny + p.Escalate + p.ManualAllow + p.ManualDeny
        subProtos = append(subProtos, p)
    }
    // Sort by total descending
    sort.Slice(subProtos, func(i, j int) bool {
        return subProtos[i].Total > subProtos[j].Total
    })

    // Build daily trend proto (reuse existing dailyBucketToProto helper)
    trendProtos := make([]*sessionv1.DailyBucketProto, 0, len(dailyBuckets))
    for _, b := range dailyBuckets {
        trendProtos = append(trendProtos, dailyBucketToProto(b))
    }

    return connect.NewResponse(&sessionv1.GetProgramAnalyticsResponse{
        Program:        program,
        Category:       category,
        Subcommands:    subProtos,
        RecentExamples: examples,
        Trend:          trendProtos,
    }), nil
}

// coveredSubcommands returns a map of subcommand → true for all subcommands
// of the given program that are covered by at least one existing rule.
//
// Rule coverage heuristic: a rule covers (program, subcommand) if it is enabled,
// applies to Bash (ToolName == "Bash" or ToolCategory == "bash"), and its
// CommandPattern string, when interpreted as a prefix regex, matches the string
// "<program> <subcommand>". We use a simple prefix-match heuristic rather than
// compiling the full regex, because: (a) RuleSpec only stores the raw CommandPattern
// string, not the compiled *regexp.Regexp; (b) full regex compilation requires
// the classifier package; (c) we only need a fast approximation for the UI badge.
//
// The approximation: if strings.Contains(spec.CommandPattern, program+" "+subcommand)
// or the pattern starts with program+" "+subcommand, we consider it covered.
// False positives are acceptable; false negatives lead to spurious "✗ gap" badges,
// which operators can dismiss. True gaps (escalated with no rule) are still detected
// via the existing ReclassifyGaps path used by GetApprovalAnalytics.
//
// NOTE: RuleSpec has no CommandProgram field — coverage is based on CommandPattern text.
func (rs *RulesService) coveredSubcommands(program string) map[string]bool {
    specs := rs.allRuleSpecs()
    covered := make(map[string]bool)
    for _, spec := range specs {
        if !spec.Enabled {
            continue
        }
        // Check for Bash tool match (exact or category)
        isBashTool := strings.EqualFold(spec.ToolName, "Bash")
        isBashCat := strings.EqualFold(spec.ToolCategory, "bash")
        if !isBashTool && !isBashCat && spec.ToolName != "" {
            continue
        }
        if spec.CommandPattern == "" {
            // A rule with no CommandPattern matches all commands — every subcommand covered.
            covered[""] = true // marks the catch-all; individual subcommands not enumerated here
            continue
        }
        // Heuristic: extract the first two tokens from the pattern as "program subcommand".
        // This works for patterns like "git push", "git push.*", "^git commit".
        pat := strings.TrimLeft(spec.CommandPattern, "^")
        tokens := strings.Fields(pat)
        if len(tokens) >= 2 && strings.EqualFold(tokens[0], program) {
            // second token is the subcommand (may have regex chars — strip them)
            sub := strings.TrimRight(tokens[1], ".*+?$")
            covered[sub] = true
        } else if len(tokens) == 1 && strings.EqualFold(tokens[0], program) {
            // Pattern matches just the program name — covers all subcommands (mark "")
            covered[""] = true
        }
    }
    return covered
}
```

**Note on `dailyBucketToProto`**: Check whether this helper already exists in
`rules_service.go` or `analytics_store.go` as a private function (it likely does, given
`GetApprovalAnalytics` already returns `DailyBucketProto` items). If not, add it:

```go
func dailyBucketToProto(b DailyBucket) *sessionv1.DailyBucketProto {
    return &sessionv1.DailyBucketProto{
        Date:        b.Date,
        AutoAllow:   int32(b.AutoAllow),
        AutoDeny:    int32(b.AutoDeny),
        Escalate:    int32(b.Escalate),
        ManualAllow: int32(b.ManualAllow),
        ManualDeny:  int32(b.ManualDeny),
        Total:       int32(b.Total),
    }
}
```

**Task 3.2.2 — Delegate from session_service.go**

In `server/services/session_service.go`, after the existing `GetApprovalAnalytics`
delegation (around line 2075), add:

```go
func (s *SessionService) GetProgramAnalytics(
    ctx context.Context,
    req *connect.Request[sessionv1.GetProgramAnalyticsRequest],
) (*connect.Response[sessionv1.GetProgramAnalyticsResponse], error) {
    return s.rulesSvc.GetProgramAnalytics(ctx, req)
}
```

No changes to `server/server.go` are needed — `SessionService` is already registered
via `sessionv1connect.NewSessionServiceHandler`. The new RPC is picked up automatically
because `SessionService` now satisfies the full generated interface.

**Test approach**:
- `TestRulesService_GetProgramAnalytics_ReturnsBreakdown` — seed known rows in SQLite,
  call RPC, assert subcommand totals match, assert `has_rule_coverage` is correct for
  a rule that covers the program.
- `TestRulesService_GetProgramAnalytics_EmptyProgram` — assert `CodeInvalidArgument`.
- `TestRulesService_GetProgramAnalytics_InvalidWindowDays` — assert `CodeInvalidArgument`
  for `window_days = 0` and `window_days = 91`.
- `TestRulesService_GetProgramAnalytics_NoProgramRows` — assert empty subcommands and
  trend slices, not nil, with no error.
- `TestRulesService_GetProgramAnalytics_EmptySubcommandKey` — seed rows with NULL
  `command_subcategory`, assert they appear as `subcommand=""` in the response (not
  panicking or being silently dropped).

---

## Epic 4 — Frontend

**Goal**: AC-8 through AC-13. Add `useProgramAnalytics` hook, `ProgramDetailPanel`
component, and wire click-to-drill-down from the "Uncovered Bash Programs" table.

---

### Story 4.1 — useProgramAnalytics hook

**Files to create**:
- `web-app/src/lib/hooks/useProgramAnalytics.ts`

**Task 4.1.1 — Implement the hook**

```typescript
import { useCallback, useEffect, useState } from "react";
import { useSessionServiceClient } from "./useSessionServiceClient";
import type { GetProgramAnalyticsResponse } from "../../gen/session/v1/session_pb";

interface UseProgramAnalyticsResult {
  data: GetProgramAnalyticsResponse | null;
  isLoading: boolean;
  error: Error | null;
  refresh: () => void;
}

export function useProgramAnalytics(
  program: string | null,
  windowDays: number
): UseProgramAnalyticsResult {
  const client = useSessionServiceClient();
  const [data, setData] = useState<GetProgramAnalyticsResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const fetch = useCallback(() => {
    if (!program) {
      setData(null);
      setIsLoading(false);
      return () => {};  // always return a cleanup; prevents AbortController leak on null→value transition
    }
    const controller = new AbortController();
    setIsLoading(true);
    setError(null);
    client
      .getProgramAnalytics(
        { program, windowDays },
        { signal: controller.signal }
      )
      .then((resp) => {
        setData(resp);
        setIsLoading(false);
      })
      .catch((err: unknown) => {
        if (err instanceof Error && err.name === "AbortError") return;
        setError(err instanceof Error ? err : new Error(String(err)));
        setIsLoading(false);
      });
    return () => controller.abort();
  }, [client, program, windowDays]);

  useEffect(() => {
    const cleanup = fetch();
    return cleanup;
  }, [fetch]);

  return { data, isLoading, error, refresh: fetch };
}
```

**Hook dependency notes**:
- `program` is `string | null` — when `null` (no selection), no fetch is issued and
  data is cleared. This prevents stale data when the panel is closed.
- `windowDays` is a dependency; changing the window re-fetches.
- `AbortController` cancels in-flight requests when `program` or `windowDays` change
  before the previous request completes.
- `client` from `useSessionServiceClient()` is a stable singleton — it does not change
  between renders and will not cause extra re-fetches.

**Test approach**:
- Jest + React Testing Library + msw (or mock the client)
- `it should fetch data when program is set`
- `it should clear data when program becomes null`
- `it should refetch when windowDays changes`
- `it should set error state on network failure`

---

### Story 4.2 — ProgramDetailPanel component

**Files to create**:
- `web-app/src/components/sessions/ProgramDetailPanel.tsx`
- `web-app/src/components/sessions/ProgramDetailPanel.css.ts`

**Task 4.2.1 — CSS (vanilla-extract)**

`ProgramDetailPanel.css.ts` — all tokens from `vars` (theme contract), no hardcoded values:

```typescript
import { style } from "@vanilla-extract/css";  // keyframes not used in this file
import { vars } from "../../styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.border}`,
  borderRadius: vars.radii.lg,
  padding: vars.space[4],
  marginTop: vars.space[3],
});

export const panelHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  marginBottom: vars.space[3],
  borderBottom: `1px solid ${vars.color.border}`,
  paddingBottom: vars.space[2],
});

export const panelTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const closeButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: vars.space[1],
  borderRadius: vars.radii.sm,
  selectors: {
    "&:hover": { color: vars.color.textPrimary, background: vars.color.hoverBackground },
  },
});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: vars.space[2],
  marginTop: vars.space[4],
});

export const breakdownTable = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
});

export const th = style({
  textAlign: "left",
  padding: `${vars.space[1]} ${vars.space[2]}`,
  color: vars.color.textMuted,
  borderBottom: `1px solid ${vars.color.border}`,
  fontWeight: 500,
});

export const td = style({
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderBottom: `1px solid ${vars.color.border}`,
  color: vars.color.textPrimary,
});

export const examplesList = style({
  listStyle: "none",
  padding: 0,
  margin: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
});

export const exampleItem = style({
  fontFamily: "monospace",
  fontSize: vars.fontSize.xs,
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderRadius: vars.radii.sm,
  overflowX: "auto",
  whiteSpace: "nowrap",
});

export const coverageBadge = style({
  display: "inline-flex",
  alignItems: "center",
  fontSize: vars.fontSize.xs,
  padding: `1px ${vars.space[1]}`,
  borderRadius: vars.radii.sm,
});

export const coverageYes = style([coverageBadge, {
  background: vars.color.successBackground,
  color: vars.color.success,
}]);

export const coverageNo = style([coverageBadge, {
  background: vars.color.errorBackground,
  color: vars.color.error,
}]);

export const addRuleLink = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space[1],
  fontSize: vars.fontSize.xs,
  color: vars.color.actionPrimary,
  textDecoration: "none",
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

// Sparkline bar within the table cell
export const sparklineBar = style({
  display: "inline-block",
  height: "12px",
  minWidth: "2px",
  background: vars.color.actionPrimary,
  borderRadius: "1px",
  opacity: 0.7,
});

export const loadingState = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: vars.space[4],
  textAlign: "center",
});

export const errorState = style({
  color: vars.color.error,
  fontSize: vars.fontSize.sm,
  padding: vars.space[3],
  background: vars.color.errorBackground,
  borderRadius: vars.radii.sm,
});
```

**Task 4.2.2 — React component**

```typescript
// +feature: analytics-drill-down
import React, { useMemo } from "react";
import { useProgramAnalytics } from "../../lib/hooks/useProgramAnalytics";
import type { SubcommandBreakdownProto } from "../../gen/session/v1/types_pb";
import * as styles from "./ProgramDetailPanel.css";

interface ProgramDetailPanelProps {
  program: string;
  windowDays: number;
  onClose: () => void;
}

export function ProgramDetailPanel({
  program,
  windowDays,
  onClose,
}: ProgramDetailPanelProps) {
  const { data, isLoading, error } = useProgramAnalytics(program, windowDays);

  // Compute max total for sparkline scaling
  const maxTotal = useMemo(() => {
    if (!data?.subcommands?.length) return 1;
    return Math.max(...data.subcommands.map((s) => s.total), 1);
  }, [data?.subcommands]);

  const totalForProgram = useMemo(() => {
    if (!data?.subcommands?.length) return 0;
    return data.subcommands.reduce((sum, s) => sum + s.total, 0);
  }, [data?.subcommands]);

  return (
    <div className={styles.panel} data-testid="program-detail-panel">
      <div className={styles.panelHeader}>
        <span className={styles.panelTitle}>
          {program}
          {data?.category ? ` · ${data.category}` : ""}
        </span>
        <button
          className={styles.closeButton}
          onClick={onClose}
          aria-label="Close program detail panel"
        >
          ✕
        </button>
      </div>

      {isLoading && (
        <div className={styles.loadingState}>Loading…</div>
      )}

      {error && (
        <div className={styles.errorState} role="alert">
          Failed to load analytics: {error.message}
        </div>
      )}

      {data && !isLoading && (
        <>
          {/* Subcommand frequency table — AC-9 */}
          <div className={styles.sectionTitle}>Subcommand Breakdown</div>
          <table className={styles.breakdownTable}>
            <thead>
              <tr>
                <th className={styles.th}>Subcommand</th>
                <th className={styles.th}>Count</th>
                <th className={styles.th}>%</th>
                <th className={styles.th}>Allow</th>
                <th className={styles.th}>Deny</th>
                <th className={styles.th}>Manual</th>
                <th className={styles.th}>Coverage</th>
                <th className={styles.th}></th>
              </tr>
            </thead>
            <tbody>
              {data.subcommands.map((row) => (
                <SubcommandRow
                  key={row.subcommand || "(none)"}
                  row={row}
                  program={program}
                  totalForProgram={totalForProgram}
                  maxTotal={maxTotal}
                />
              ))}
              {data.subcommands.length === 0 && (
                <tr>
                  <td className={styles.td} colSpan={8}>
                    No subcommand data for this program in the selected window.
                  </td>
                </tr>
              )}
            </tbody>
          </table>

          {/* Example commands — AC-10 */}
          {data.recentExamples.length > 0 && (
            <>
              <div className={styles.sectionTitle}>Recent Examples</div>
              <ul className={styles.examplesList}>
                {data.recentExamples.map((cmd, i) => (
                  <li key={i} className={styles.exampleItem}>
                    {cmd}
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      )}
    </div>
  );
}

interface SubcommandRowProps {
  row: SubcommandBreakdownProto;
  program: string;
  totalForProgram: number;
  maxTotal: number;
}

function SubcommandRow({ row, program, totalForProgram, maxTotal }: SubcommandRowProps) {
  const label = row.subcommand || "(none)";
  const pct = totalForProgram > 0 ? ((row.total / totalForProgram) * 100).toFixed(1) : "0.0";
  const barWidth = maxTotal > 0 ? Math.round((row.total / maxTotal) * 80) : 0;
  const manual = row.escalate + row.manualAllow + row.manualDeny;

  // AC-13: "Add rule →" link pre-populates the rule form
  const addRuleHref = `/rules?program=${encodeURIComponent(program)}&subcommand=${encodeURIComponent(row.subcommand)}`;

  return (
    <tr>
      <td className={styles.td}>{label}</td>
      <td className={styles.td}>
        <span
          className={styles.sparklineBar}
          style={{ width: `${barWidth}px` }}
          aria-hidden="true"
        />
        {" "}
        {row.total}
      </td>
      <td className={styles.td}>{pct}%</td>
      <td className={styles.td}>{row.autoAllow}</td>
      <td className={styles.td}>{row.autoDeny}</td>
      <td className={styles.td}>{manual}</td>
      <td className={styles.td}>
        {row.hasRuleCoverage ? (
          <span className={styles.coverageYes}>✓ covered</span>
        ) : (
          <span className={styles.coverageNo}>✗ gap</span>
        )}
      </td>
      <td className={styles.td}>
        {!row.hasRuleCoverage && (
          <a href={addRuleHref} className={styles.addRuleLink}>
            Add rule →
          </a>
        )}
      </td>
    </tr>
  );
}
```

**Implementation notes**:
- The sparkline uses a CSS bar per subcommand row (scaled by `maxTotal`). Per-subcommand
  day-by-day sparklines require per-subcommand trend data; the current API returns whole-
  program trend. For per-subcommand day-by-day sparklines, a future enhancement would add
  `repeated SubcommandTrendProto per_subcommand_trends = 6` to the response. The current
  plan satisfies AC-12 with bar-width-as-proportion (simpler, no extra RPC needed).
- `style={{ width: ... }}` on `sparklineBar` is a dynamic inline style for bar width.
  This is a pure numeric pixel value, not a theme override, so it does not violate the
  CSS architecture rule (which prohibits inline styles for *layout* properties that fight
  the cascade). An alternative is `data-width={barWidth}` + CSS `attr()`, but browser
  support for `attr()` in non-content properties is not universal. The inline `width` is
  acceptable here.
- `recentExamples` index as `key` is acceptable since examples are read-only and never
  reordered.

**Test approach**:
- `ProgramDetailPanel.test.tsx` — render with mock `useProgramAnalytics` data:
  - Assert subcommand table rows render with correct counts
  - Assert `(none)` label for empty subcommand
  - Assert coverage badge shows ✓/✗ correctly
  - Assert "Add rule →" link has correct href (`program=X&subcommand=Y`)
  - Assert panel closes when close button is clicked (onClose called)
  - Assert loading state renders when `isLoading = true`
  - Assert error state renders on error
  - Assert example list renders up to 20 items

---

### Story 4.3 — Wire ProgramDetailPanel into ApprovalAnalyticsPanel

**Files to modify**:
- `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

**Task 4.3.1 — Add selectedProgram state and panel toggle**

In `ApprovalAnalyticsPanel.tsx`:

1. Add state:
   ```typescript
   const [selectedProgram, setSelectedProgram] = useState<string | null>(null);
   ```

2. Import `ProgramDetailPanel`:
   ```typescript
   import { ProgramDetailPanel } from "./ProgramDetailPanel";
   ```

3. In the "Rule Coverage Gaps" section (the `top_uncovered_programs` table), make each
   program row clickable:
   ```tsx
   <tr
     key={prog.programName}
     style={{ cursor: "pointer" }}
     onClick={() => setSelectedProgram(
       selectedProgram === prog.programName ? null : prog.programName
     )}
     aria-expanded={selectedProgram === prog.programName}
   >
     ...existing cells...
   </tr>
   {selectedProgram === prog.programName && (
     <tr>
       <td colSpan={/* number of columns */}>
         <ProgramDetailPanel
           program={prog.programName}
           windowDays={windowDays}
           onClose={() => setSelectedProgram(null)}
         />
       </td>
     </tr>
   )}
   ```

4. Optionally, also wire the click in the "Command Distribution" table
   (`CommandDistributionTable` inline component, lines 533–599 of the current file).
   Clicking a program row in that table should also open the detail panel for that program.
   This satisfies AC-8 more broadly.

**Implementation notes**:
- Only one program panel is open at a time (replace, not stack). Clicking the currently
  selected program toggles it off. Clicking a different program opens it for that program.
- The `onClick` toggle on `tr` must not conflict with any existing click handlers in the
  program rows. Check the existing `top_uncovered_programs` rendering section for
  conflicting `onClick` handlers before adding.
- `cursor: "pointer"` on `tr` — use a CSS class from the existing module or a new
  vanilla-extract class in `ApprovalAnalyticsPanel.css.ts` (if it already exists) or add
  to a shared styles file. Do NOT add a new `.module.css` file (see css-architecture.md).

**Test approach**:
- Integration test in `ApprovalAnalyticsPanel.test.tsx`:
  - Click a program row → assert `data-testid="program-detail-panel"` appears
  - Click the same row again → assert panel disappears
  - Click a different program row → assert panel switches to the new program
  - Click close button → assert panel disappears

---

## Cross-Cutting Concerns

### Nil safety

All new Go code follows nil-safety conventions:
- `GetSubcommandBreakdown` returns `[]SubcommandDecisionCount{}` (not nil) on no-data,
  nil error. Both `entRepository` impls should ensure empty slice on no rows.
- `LoadProgramWindow`, `ListRecentCommands` return empty slices, not nil, on no data.
- The handler guards `program == ""` at the top before any data access.
- `coveredSubcmds[sub]` on a nil map panics — the map is always initialized via `make`.

### Empty subcommand handling

`command_subcategory` is optional in the schema (`.Optional()` in ent). NULL values:
- Scan as `""` in ent's `GroupBy().Scan()` result struct.
- Appear as `subcommand: ""` in `SubcommandDecisionCount`.
- Are represented as `subcommand: ""` in proto (proto3 string default is `""`).
- Are displayed as `(none)` in the UI.

Do NOT filter out empty-string subcommands at the repository layer — they are real data
(commands like `ls`, `pwd`, `date` have no subcommand).

### Large dataset protection

The compound index limits `GetSubcommandBreakdown` and `ListAnalyticsByProgramSince` to
scanning only the program's rows. For programs with very high volume (e.g., `git` called
10,000 times), `LoadProgramWindow` fetches all rows for that program. The `ComputeDailyBuckets`
call after is O(n). At SQLite scale (typical dev use: <100k total rows, <10k per program),
this is acceptable. If a program exceeds 50k rows, add a hard limit of `n=50000` to
`LoadProgramWindow` to prevent OOM. Add a TODO comment in `analytics_store.go`.

### Feature registry

Per `.claude/rules/feature-registry.md`:
- Add entry to `docs/registry/frontend-features.json` for `analytics-drill-down`:
  ```json
  {
    "id": "analytics-drill-down",
    "type": "frontend",
    "component": "ProgramDetailPanel",
    "file": "web-app/src/components/sessions/ProgramDetailPanel.tsx",
    "tested": false,
    "testIds": []
  }
  ```
- Add entry to `docs/registry/backend-features.json` for `program:analytics`:
  ```json
  {
    "id": "program:analytics",
    "type": "backend",
    "rpc": "GetProgramAnalytics",
    "markerFound": false,
    "tested": false,
    "testIds": [],
    "lastModified": "2026-05-19T00:00:00Z"
  }
  ```
- Update both to `tested: true` with test IDs once tests are written.

### make quick-check (AC-15)

Run before pushing:
```bash
gofmt -w .
make quick-check   # build + test + lint
```

Lint failures will surface unused imports (especially after ent regeneration adds new
predicate functions that may shadow imports).

---

## Implementation Sequence

Execute epics in order (each epic's output is required input for the next):

1. **Epic 1** (schema + ent generate) → enables `CreatedAtGTE`, `CommandProgramEQ`,
   `CommandSubcategoryEQ`, `CommandPreviewNotNil` predicates in generated code
2. **Epic 2** (repo + store) → `go build ./...` must pass; run existing tests
3. **Epic 3** (proto + RPC) → `make generate-proto`; run `make build && make test`
4. **Epic 4** (frontend) → `cd web-app && npx jest --no-coverage` after each component

Fresh session recommended before Epic 4 (frontend context is cleanest with backend done).

---

## Acceptance Criteria Traceability

| AC | Epic/Story/Task |
|----|-----------------|
| AC-1 | Epic 2, Story 2.5, Task 2.5.1 |
| AC-2 | Epic 1, Story 1.1, Task 1.1.1 |
| AC-3 | Epic 2, Story 2.2, Task 2.2.1 + Story 2.3, Task 2.3.2 |
| AC-4 | Epic 2, Story 2.2, Task 2.2.1 + Story 2.3, Task 2.3.3 |
| AC-5 | Epic 2, Story 2.2, Task 2.2.1 + Story 2.3, Task 2.3.4 |
| AC-6 | Epic 2, Story 2.2, Task 2.2.1 + Story 2.3, Task 2.3.5 |
| AC-7 | Epic 3 (all stories) |
| AC-8 | Epic 4, Story 4.3, Task 4.3.1 |
| AC-9 | Epic 4, Story 4.2, Task 4.2.2 (subcommand table) |
| AC-10 | Epic 4, Story 4.2, Task 4.2.2 (example list section) |
| AC-11 | Epic 4, Story 4.2, Task 4.2.2 (coverage badge column) |
| AC-12 | Epic 4, Story 4.2, Task 4.2.2 (sparkline bar) |
| AC-13 | Epic 4, Story 4.2, Task 4.2.2 (addRuleHref) |
| AC-14 | All epics (no removal of existing methods/tests) |
| AC-15 | Cross-cutting (gofmt + make quick-check) |
