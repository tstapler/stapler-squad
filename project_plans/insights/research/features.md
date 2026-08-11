# Features Research — Insights Page Enhancement

## useInsightsSummary Hook (`lib/hooks/useInsightsService.ts`)

### InsightsFilters Interface

```typescript
export interface InsightsFilters {
  from?: Date;
  to?: Date;
  modelFilter?: string;
  sessionIdFilter?: string;
  includeOrphans?: boolean;
}
```

The `from` and `to` fields are already defined in `InsightsFilters` but **not currently passed** to `GetInsightsSummaryRequest`. The `fetchSummary` callback uses:

```typescript
const req = create(GetInsightsSummaryRequestSchema, {
  includeOrphans: filters.includeOrphans ?? true,
  ...(filters.modelFilter && { modelFilter: filters.modelFilter }),
  ...(filters.sessionIdFilter && { sessionIdFilter: filters.sessionIdFilter }),
});
```

**Gap**: `filters.from` and `filters.to` are in the type but are not spread into the request object. They need to be added:

```typescript
...(filters.from && { from: Timestamp.fromDate(filters.from) }),
...(filters.to && { to: Timestamp.fromDate(filters.to) }),
```

The `useCallback` dep array also needs `filters.from` and `filters.to` added.

### WatchInsights Stream Filters Gap

`startWatch` creates `WatchInsightsRequestSchema` with empty fields — it does not pass the time range:

```typescript
const req = create(WatchInsightsRequestSchema, {});
```

This needs to mirror the summary filter to avoid receiving events outside the time window. The `startWatch` callback should accept `from`/`to` from the filters.

## GetInsightsSummaryRequest Proto Fields

From `proto/session/v1/insights.proto`:

```protobuf
message GetInsightsSummaryRequest {
  google.protobuf.Timestamp from   = 1;
  google.protobuf.Timestamp to     = 2;
  optional string model_filter     = 3;
  optional string session_id_filter = 4;
  bool            include_orphans  = 5;
}
```

All time-range fields are `google.protobuf.Timestamp`. Use `@bufbuild/protobuf` `Timestamp` helpers to convert from JS `Date`.

## SessionTokenSummary Proto Fields

Full field list from proto:

| Field | Type | Notes |
|---|---|---|
| `session_id` | `string` | May be empty for orphans |
| `conversation_id` | `string` | JSONL UUID |
| `project_path` | `string` | Full path |
| `primary_model` | `string` | e.g. "claude-sonnet-4" |
| `total_input_tokens` | `int64` (bigint in TS) | |
| `total_output_tokens` | `int64` | |
| `cache_creation_tokens` | `int64` | |
| `cache_read_tokens` | `int64` | |
| `estimated_cost_usd` | `double` | |
| `cache_hit_rate` | `double` | cache_read / (input + cache_read) |
| `message_count` | `int32` | Turn count = proxy for TurnTimeline |
| `first_message_at` | `Timestamp` | Session start |
| `last_message_at` | `Timestamp` | Session end |
| `is_orphan` | `bool` | No matching stapler-squad session |
| `skill_activations` | `repeated string` | Skill names used |
| `top_tools` | `repeated TopToolEntry` | Tool name + call_count + mcp_server |

**No TurnTimeline field exists** — the per-session detail view will use `message_count` as a proxy and `skill_activations` / `top_tools` for breakdown. There is no separate per-turn data in the current proto; the "turn timeline" in requirements will need to be simulated from what's available.

## ListSessionTokens RPC

```protobuf
message ListSessionTokensRequest {
  google.protobuf.Timestamp from = 1;
  google.protobuf.Timestamp to   = 2;
  string sort_by                 = 3; // "cost" | "tokens" | "date"
  bool   sort_desc               = 4;
  int32  page_size               = 5;
  string page_token              = 6;
}

message ListSessionTokensResponse {
  repeated SessionTokenSummary sessions = 1;
  string next_page_token                = 2;
  int32  total_count                    = 3;
}
```

This RPC supports cursor-based pagination with `page_token` and `next_page_token`. It is NOT currently used by the frontend — the dashboard loads all sessions via `GetInsightsSummary`. For R5 (large session counts), this RPC provides server-side pagination as an alternative to client-side virtual scroll.

**Note**: `ListSessionTokens` does NOT have a `model_filter` or `session_id_filter` parameter, unlike `GetInsightsSummaryRequest`. Filtering by model or search would need to be done client-side if using this RPC, or a new proto field would be needed (out of scope per requirements).

## DailyTokenBucket Fields

```protobuf
message DailyTokenBucket {
  Timestamp date         = 1;
  int64  total_input_tokens  = 2;
  int64  total_output_tokens = 3;
  int64  cache_read_tokens   = 4;
  double estimated_cost_usd  = 5;
  int32  session_count       = 6;
  map<string, double> cost_by_model   = 7;
  map<string, double> tokens_by_model = 8; // Note: proto says int64, TS gen uses bigint
}
```

For **cost projections (R3)**:
- Daily buckets provide up to 90 days of data
- Projection formula: `projectedMonthly = (totalCostInRange / daysInRange) * 30`
- The `GetInsightsSummaryResponse` has `total_cost_usd` + `daily[]` array — both needed for projection
- No server-side projection endpoint — must be computed client-side from the daily array

## GetInsightsSummaryResponse Fields

```
sessions[]        — all session summaries (used in SessionsTable)
total_cost_usd    — for SummaryCards
total_input_tokens, total_output_tokens, total_cache_read_tokens — SummaryCards
overall_cache_hit_rate — SummaryCards
daily[]           — DailyTokenBucket array for charts + projections
models[]          — ModelBreakdown for ModelBreakdownChart
top_skills[]      — TopEntry for TopNTables
top_tools[]       — TopEntry for TopNTables
is_loading        — background parse in progress flag
pricing_as_of     — Timestamp; unused in current UI
```

## Summary

- **from/to filter fields exist in both InsightsFilters and proto** but are not yet wired through the `fetchSummary` call — this is the primary gap to close for R1
- **SessionTokenSummary has no TurnTimeline** — per-session detail will use `message_count`, `skill_activations`, `top_tools`, `first/last_message_at` as a proxy; full turn-level data would require a new RPC (not in scope)
- **ListSessionTokens provides server-side pagination** with cursor tokens, sort_by, and time range — ready to use for R5 without new proto changes
