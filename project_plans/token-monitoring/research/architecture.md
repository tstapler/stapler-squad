# Architecture Research — Token Monitoring

## Existing Page Structure

### Session List (`web-app/src/app/page.tsx`)
- Main page at `/` — renders session list + session detail panel
- Uses `useSessionServiceContext()` for data
- Sessions come from `WatchSessions` streaming RPC → Redux store → component
- Session cards rendered in `web-app/src/components/sessions/`

### History Browser (`web-app/src/app/history/page.tsx`)
- Dedicated page at `/history`
- Uses `ClaudeHistoryEntry` and `ClaudeMessage` proto types
- Calls `SearchService.ListClaudeHistory` and `GetClaudeHistoryMessages`
- Has pagination, filtering by model/date, grouping, full-text search
- **This is the closest architectural analog to the `/insights` page**

### Logs Page (`web-app/src/app/logs/page.tsx`)
- Similar structure — standalone page with filter controls

### Routing
- Next.js App Router with file-based routing
- New `/insights` route: create `web-app/src/app/insights/page.tsx` and `web-app/src/app/insights/layout.tsx`
- Route is referenced in navigation (need to add to nav component)

## ConnectRPC Streaming Pattern

**Proto pattern (server streaming):**
```protobuf
rpc WatchSessions(WatchSessionsRequest) returns (stream SessionEvent) {}
```

**Go handler pattern:**
```go
func (s *InsightsService) GetInsightsSummary(
    ctx context.Context,
    req *connect.Request[sessionv1.GetInsightsSummaryRequest],
) (*connect.Response[sessionv1.GetInsightsSummaryResponse], error) {
    // compute and return
}

// For streaming updates:
func (s *InsightsService) WatchInsights(
    ctx context.Context,
    req *connect.Request[sessionv1.WatchInsightsRequest],
    stream *connect.ServerStream[sessionv1.InsightsEvent],
) error {
    // push updates when new JSONL data is parsed
}
```

**Frontend hook pattern:**
- Create `web-app/src/lib/hooks/useInsightsService.ts` following `useSessionService.ts` pattern
- Use `createClient(SessionService, transport)` with the existing transport
- Or extend the existing `SessionService` proto with new RPC methods

**Registration:** In `server/server.go`, add `path, handler := sessionv1connect.NewInsightsServiceHandler(insightsService)` and `mux.Handle(path, handler)`.

## Data Model Design

### Option A: Computed On-Demand (Recommended)
Parse JSONL files when the `/insights` endpoint is called, cache results in memory, invalidate on fsnotify events.

**Pros:** No schema changes to ent, no migration, no storing sensitive data in DB.  
**Cons:** First load may be slow for 90 days of sessions; cache warm-up needed.

**Mitigation:** Background goroutine pre-parses all known JSONL files on startup. New files trigger incremental re-parse via fsnotify. Results stored in a `TokenStore` struct (similar to `AnalyticsStore`).

### Option B: Persist to Ent Schema
Add token fields to `ClaudeSession` ent entity or create new `TokenUsage` entity.

**Pros:** Fast queries, survives restarts.  
**Cons:** Schema migration, storing potentially large data, JSONL is the source of truth so DB could become stale.

### Option C: Hybrid (Future Option)
Compute on demand initially; add optional persistence layer in v2.

**Recommendation: Option A** — matches NFR-2 (< 2s for 90 days) if pre-parsed in background. The `AnalyticsStore` pattern already demonstrates this approach for in-memory aggregation.

## Go-Side Token Aggregation Structure

### New Package: `session/tokens/`

```go
// ParseResult holds token data extracted from one JSONL file.
type ParseResult struct {
    SessionUUID    string
    ProjectPath    string
    Model          string    // most-used model
    TotalInput     int64
    TotalOutput    int64
    CacheCreation  int64
    CacheRead      int64
    MessageCount   int
    TurnTimeline   []TurnStats // for burn rate chart
    ToolUsage      map[string]ToolTokenStats
    SkillActivations []string
    ParsedAt       time.Time
}

// TurnStats is per-assistant-message token data.
type TurnStats struct {
    Timestamp time.Time
    Model     string
    Input     int64
    Output    int64
    CacheCreation int64
    CacheRead int64
    Tools     []string // tool_use block names in this message
}

// ToolTokenStats aggregates token attribution for one tool.
type ToolTokenStats struct {
    ToolName    string
    CallCount   int
    // Note: exact per-tool token attribution requires heuristics —
    // tokens are per-message, not per-tool-call within a message.
}
```

### New Service: `server/services/insights_service.go`

Owns a `TokenStore` (in-memory cache of `ParseResult` per session UUID). Exposes RPC methods. Triggered to re-parse by the `HistoryFileWatcher` callback.

### Proto Changes

New messages in `proto/session/v1/insights.proto` (new file):
```protobuf
message SessionTokenSummary {
  string session_id = 1;      // maps to stapler-squad session
  string conversation_id = 2; // JSONL conversation UUID
  string model = 3;
  int64 total_input_tokens = 4;
  int64 total_output_tokens = 5;
  int64 cache_creation_tokens = 6;
  int64 cache_read_tokens = 7;
  double estimated_cost_usd = 8;
  google.protobuf.Timestamp parsed_at = 9;
  int32 message_count = 10;
  double cache_hit_rate = 11;
}

message GetInsightsSummaryRequest {
  google.protobuf.Timestamp from = 1;
  google.protobuf.Timestamp to = 2;
  optional string model_filter = 3;
  optional string session_id_filter = 4;
}

message GetInsightsSummaryResponse {
  repeated SessionTokenSummary sessions = 1;
  int64 total_cost_microdollars = 2;  // aggregate cost
  repeated DailyTokenBucket daily = 3;
  repeated ModelBreakdown models = 4;
}
```

## Where to Add `/insights` Route

1. **New file:** `web-app/src/app/insights/page.tsx` (and `layout.tsx`)
2. **Navigation:** Add link to existing nav component (likely in `web-app/src/components/` or the main layout)
3. **Feature marker:** Add `// +feature: insights-dashboard` to first 10 lines of `page.tsx`
4. **Registry:** Add entry to `docs/registry/frontend-features.json`

## Background Goroutine vs On-Demand Parsing

**Recommendation: Background goroutine with on-demand fallback**

1. On server start: launch background goroutine that walks `~/.claude/projects/` and parses all JSONL files not yet in cache (using a file-modification-time comparison).
2. `HistoryFileWatcher` callback fires → enqueue file for (re-)parsing.
3. On `/insights` RPC call: return cached results immediately, kick off background parse for any uncached files, return partial results with `loading: true` flag if any files are being parsed.
4. Cache stores `ParseResult` keyed by JSONL file path + modification time (so file changes invalidate cache automatically).

**Why not on-demand only:** NFR-1 says 10k+ line JSONL must not block UI; blocking HTTP handler would violate this. NFR-2 (< 2s for 90 days) requires pre-parsing.

**Why not persistent DB:** Token data is derivable from JSONL at any time. Storing it in SQLite adds migration complexity and stale-data risk without clear benefit.

## Session Association Architecture

**Active sessions:** `Instance.HistoryFilePath` is set by `HistoryLinker` → can read directly.

**Completed/orphaned sessions:** Scan `~/.claude/projects/` for all JSONL files, match to stapler-squad sessions by:
1. `conversation_id` field on `ClaudeSession` ent entity (if set)
2. Path-based matching: decode project dir name back to path → find sessions with matching `path` or `working_dir`
3. Timestamp correlation: JSONL file creation time vs session creation time

**Orphan handling:** JSONL files that don't match any session get shown in an "Other" section in the dashboard.

## Ent Schema Changes Needed

**Minimal approach (Option A — recommended):**  
No new ent schema changes. Token data lives in `TokenStore` (in-memory). `ClaudeSession.conversation_id` already stores the JSONL UUID.

**Optional enrichment fields on `Session` (nice to have):**
```go
field.Int64("total_tokens_cached").Optional()  // populated after session ends
field.Float64("estimated_cost_usd").Optional()  // computed cost
```
These are convenience fields for the session list token badge — avoids full JSONL parse just to show a badge. Add in v2 if performance requires it.

## Navigation and Layout

**Current app structure:**
```
web-app/src/app/
├── page.tsx              // Sessions list (main page)
├── history/              // History browser
├── logs/                 // Log viewer
├── sessions/new/         // Session creation
├── settings/             // Settings
├── review-queue/         // Approval queue
├── notifications/        // Notifications
```

Add `/insights` as a peer to `/history`. Nav link goes in the same navigation component that links to `/history`.
