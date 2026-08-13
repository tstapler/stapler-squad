# Agent 4 — Pitfalls Research

## Ent Schema Generation — Critical Rule

Source: `CLAUDE.md` and `session/ent/generate.go` line 3

The generate command MUST include `--feature sql/upsert`:

```bash
# CORRECT
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema

# WRONG — breaks UpsertRule and RecordAnalytics (OnConflictColumns)
go run entgo.io/ent/cmd/ent generate ./session/ent/schema
```

`RecordAnalytics` uses `OnConflictColumns(classificationanalytics.FieldAnalyticsID)` via the
upsert path. Omitting `--feature sql/upsert` silently removes the `OnConflict*` methods
from the generated code, breaking existing writes.

After running generate: commit ALL changed files under `session/ent/` together. Partial
commits leave the client and schema out of sync.

## Schema Migration Safety

Ent uses `AutoMigrate` at startup. Adding indexes is purely additive and safe — no data
migration needed, no downtime, no schema version bump.

**Risk**: If the compound index `(command_program, created_at)` is added to the schema
but the generate step is not re-run (or run without `--feature sql/upsert`), the index
won't appear in the migration and no error is raised at compile time. Always confirm with
`go build ./...` after generate.

**SQLite index build time**: On a large table (100k+ rows), adding a new index triggers
a table scan during `AutoMigrate`. For SQLite this is done under a write lock. If the
stapler-squad service restarts with a large analytics table, startup may be momentarily
slow (~1-3s for typical dev use). Not a production concern but worth noting.

## Proto Generation

`make generate-proto` regenerates both Go and TypeScript bindings:
- `session/gen/session/v1/*.go`
- `web-app/src/gen/session/v1/*_pb.ts`

**Pitfall**: Field numbers in proto messages must never be reused or renumbered. The
`AnalyticsSummaryProto` already uses fields 1–16. New fields for `GetProgramAnalyticsResponse`
start at 1 (it's a new message — no collision risk). For adding to existing messages,
check the highest field number and use next available.

**Pitfall**: `optional int32 window_days` uses proto3 optional syntax. The generated Go
uses `*int32`. Existing code uses `if req.Msg.WindowDays != nil` guard. Match this pattern
in `GetProgramAnalyticsRequest`.

## LoadWindow Refactor Risk

`LoadWindow` is called in two places:
1. `rules_service.go` line 151 — `GetApprovalAnalytics` handler
2. `rules_service.go` line 413 — `buildPromptContext` (for AI rule generation)

Both call `rs.analyticsStore.LoadWindow(since)`. Changing `LoadWindow` to use
`ListAnalyticsSince` (the new time-windowed repo method) is safe and fixes both call
sites for free. The Go-level date filter removal is correct because the DB now filters.

**Risk**: Tests that mock `ListAnalytics` on the repository will need to be updated to
mock `ListAnalyticsSince` instead. Check `server/services/approval_service_test.go` —
it uses `newRulesService(t)` which wires a real SQLite test storage, so no mock changes
needed there.

## In-Go Aggregation vs SQL Aggregation for Subcommand Breakdown

`GetSubcommandBreakdown` (AC-4) could be implemented two ways:

### Option A: SQL GROUP BY via ent
```go
r.client.ClassificationAnalytics.Query().
    Where(...).
    GroupBy(FieldCommandSubcategory, FieldDecision).
    Aggregate(ent.Count()).
    Scan(ctx, &result)
```

**Pitfall**: Ent's `GroupBy().Scan()` requires a custom struct with exported fields
matching the SQL column names. The generated predicate field constants use different
capitalization than the SQL column names. Use `classificationanalytics.FieldCommandSubcategory`
(= `"command_subcategory"`) for the GroupBy arg and ensure the scan struct tags match.

**Pitfall**: Empty `command_subcategory` values (NULL or `""`) will appear as a
`""` key in the results. Filter them out in the service layer.

### Option B: Fetch all rows for program+window, aggregate in Go
Same approach as existing `ComputeSummary`. Simpler, no ent GroupBy pitfalls, but
loads more data than necessary for large programs.

**Recommendation**: Use Option A for `GetSubcommandBreakdown` (it's a hot path that
needs to be fast), Option B for `GetSubcommandTrend` (fewer rows per subcommand,
reuses `ComputeDailyBuckets`).

## AnalyticsStore.storage field is *session.Storage not Repository

`AnalyticsStore.storage` is typed as `*session.Storage` (a concrete type), not
`session.Repository`. `session.Storage` is a thin wrapper around `Repository` that
provides the `ListAnalytics`, `RecordAnalytics` methods. New repo methods added to the
`Repository` interface must also be exposed on `session.Storage` for `AnalyticsStore`
to call them.

Check `session/storage.go` — it likely delegates to `r.repo.ListAnalytics(...)`.
New methods follow the same delegation pattern.

## ComputeSummary and ReclassifyGaps are Pure Functions

`ComputeSummary` and `ReclassifyGaps` are pure — no I/O, no side effects. They work
on `[]AnalyticsEntry` regardless of how those entries were loaded. Fixing `LoadWindow`
does not require changing these functions.

`ComputeDailyBuckets` is also pure and can be reused for `GetSubcommandTrend` (filter
`[]AnalyticsEntry` to a single program+subcommand before calling).

## Frontend: No New Build Tooling Needed

The `SubcommandBreakdownProto` type will be auto-generated by `make generate-proto` into
`web-app/src/gen/session/v1/types_pb.ts`. The frontend can import it directly.

**Pitfall**: Proto3 `bool has_rule_coverage = 6` will default to `false` in the
TypeScript generated type if not set by the server. Ensure the service layer explicitly
sets this field; don't rely on absence of the field to mean "not covered".

## AnalyticsStore `storage` field access for new repo methods

`AnalyticsStore.storage` is a `*session.Storage` (not exported). New methods on
`AnalyticsStore` that call the new repo methods must either:
1. Access `s.storage.ListAnalyticsByProgramSince(...)` — requires adding the method to
   `session.Storage`
2. Or expose the repo directly via a `s.storage.Repo()` accessor

Follow existing pattern: add the delegation method to `session.Storage` alongside
the existing `ListAnalytics`, `RecordAnalytics` delegations.

## Make Targets

```bash
make build && make test    # After ent generate: build first (generates protos too)
make generate-proto        # After proto changes
make lint                  # Required before commit — build fails if lint fails
gofmt -w .                 # Required formatting
```

Full CI: `make ci` — run before pushing.
