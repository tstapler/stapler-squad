# Agent 1 — Stack/Infrastructure Research

## DB Schema: ClassificationAnalytics

File: `session/ent/schema/classificationanalytics.go`

Fields:
- `analytics_id` (string, unique, PK)
- `session_id` (string, optional)
- `tool_name` (string)
- `command_preview` (string, optional, truncated to 200 chars)
- `cwd` (string, optional)
- `decision` (string: "auto_allow", "auto_deny", "escalate", "manual_allow", "manual_deny")
- `risk_level` (string: "low", "medium", "high", "critical")
- `rule_id` (string, optional)
- `rule_name` (string, optional)
- `reason` (string, optional)
- `alternative` (string, optional)
- `duration_ms` (int64, default 0)
- `approval_id` (string, optional)
- `command_program` (string, optional) — primary executable (e.g. "git")
- `command_category` (string, optional) — e.g. "vcs", "node"
- `command_subcategory` (string, optional) — first positional subcommand (e.g. "commit")
- `python_imports` ([]string, optional)
- `created_at` (time.Time, immutable, default: time.Now)

### Existing indexes

| Index | Fields |
|---|---|
| idx_session_id | session_id |
| idx_decision | decision |
| idx_risk_level | risk_level |
| idx_rule_id | rule_id |
| idx_created_at | created_at |

**Missing for drill-down**: compound index on `(command_program, created_at)`. The
`created_at` single-field index exists (AC-2 partial) but the compound is not present.

## Repository Interface

File: `session/repository.go`

Analytics-related methods today:
```go
RecordAnalytics(ctx context.Context, data AnalyticsData) error
ListAnalytics(ctx context.Context, limit int) ([]AnalyticsData, error)
```

No time-windowed, program-filtered, subcommand-breakdown, or trend query methods exist.
All four new repo methods required by AC-3 through AC-6 are missing.

## ListAnalytics Implementation

File: `session/ent_repository.go` lines 1117–1154

```go
func (r *EntRepository) ListAnalytics(ctx context.Context, limit int) ([]AnalyticsData, error) {
    query := r.client.ClassificationAnalytics.Query().
        Order(ent.Desc(classificationanalytics.FieldCreatedAt))
    if limit > 0 {
        query = query.Limit(limit)
    }
    entries, err := query.All(ctx)
    ...
}
```

No `WHERE created_at >= ?` clause. Full table scan always. This is the root cause of AC-1
failing and the O(n) load confirmed in the requirements.

## AnalyticsStore

File: `server/services/analytics_store.go` (602 lines)

Key types:
- `AnalyticsEntry` — service-layer struct mirroring DB fields
- `AnalyticsSummary` — aggregated stats (total, decision counts, top tools, top rules,
  top programs, top python imports, coverage gaps, full subcommand distribution)
- `SubcommandStat` / `ProgramStat` — in-memory aggregation types
- `DailyBucket` — per-day aggregation
- `AnalyticsStore` — writes via async channel (buffer 1000), flush every 5s

`LoadWindow(since time.Time)`:
```go
func (s *AnalyticsStore) LoadWindow(since time.Time) ([]AnalyticsEntry, error) {
    data, err := s.storage.ListAnalytics(context.Background(), 0)  // loads ALL rows
    ...
    for _, d := range data {
        if !d.CreatedAt.Before(since) {
            entries = append(entries, ...)
        }
    }
    return entries, nil
}
```
TODO comment in source acknowledges this is a full-load anti-pattern.

`ComputeSummary(entries)` and `ComputeDailyBuckets(entries)` are pure Go functions —
they will remain valid after the query layer is fixed.

`ReclassifyGaps(entries, classifier)` re-applies current rules to previously-escalated
entries. This must still be called after any new query method.

## Existing RPCs

From `proto/session/v1/session.proto`:
```
rpc GetApprovalAnalytics(GetApprovalAnalyticsRequest) returns (GetApprovalAnalyticsResponse)
```
Request: `optional int32 window_days = 1`
Response: `AnalyticsSummaryProto summary + repeated DailyBucketProto daily_buckets`

No `GetProgramAnalytics` RPC exists yet. It needs to be added (AC-7).

## Key gap summary

| Gap | Location |
|---|---|
| No WHERE created_at >= ? | `ListAnalytics` in `ent_repository.go` |
| No compound index (command_program, created_at) | `classificationanalytics.go` schema |
| No ListAnalyticsByProgramSince | `Repository` interface + `EntRepository` |
| No GetSubcommandBreakdown | `Repository` interface + `EntRepository` |
| No ListRecentCommandsByProgram | `Repository` interface + `EntRepository` |
| No GetSubcommandTrend | `Repository` interface + `EntRepository` |
| No GetProgramAnalytics RPC | `session.proto` + `session_service.go` |
