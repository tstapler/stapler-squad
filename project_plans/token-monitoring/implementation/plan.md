# Token & Spend Monitoring — Implementation Plan

**Phase:** 3 (Planning)  
**Date:** 2026-05-15  
**Epics:** 3  **Stories:** 18  **Tasks:** 38

---

## 1. Architecture Overview

### Component Diagram

```
~/.claude/projects/<encoded-path>/<uuid>.jsonl
          │
          │  fsnotify (HistoryFileWatcher callback)
          ▼
┌─────────────────────────────────────────────────────────┐
│  session/tokens/  (new package)                         │
│                                                         │
│  Parser         ─── parses JSONL lines               │
│  │  bufio.Scanner (10MB buffer)                         │
│  │  json.Unmarshal per line (errors skipped)            │
│  │  extracts: model, usage fields, tool_use names       │
│  │  detects: /command patterns in user turns            │
│  ▼                                                      │
│  ParseResult    ─── aggregated per-file result          │
│  │  SessionUUID, ProjectPath, Model                     │
│  │  TotalInput, TotalOutput, CacheCreation, CacheRead   │
│  │  TurnTimeline []TurnStats (timestamps + counts)      │
│  │  ToolUsage map[string]ToolTokenStats                 │
│  │  SkillActivations []string                           │
│  │  (NO message content ever stored)                    │
│  ▼                                                      │
│  PricingTable   ─── model → $/MTok lookup              │
│  │  Hardcoded + config-file override                    │
│  │  NormalizeModelFamily() strips date suffixes         │
│  ▼                                                      │
│  TokenStore     ─── in-memory cache                    │
│  │  map[filePath]cachedResult{result, modTime}          │
│  │  sync.RWMutex for concurrent reads                   │
│  │  Background goroutine: walk + pre-parse on startup   │
│  │  fsnotify callback: enqueue changed files            │
│  │  Worker pool (4 goroutines) for parallel parse       │
└─────────────────────────────────────────────────────────┘
          │
          │  ConnectRPC (new InsightsService)
          ▼
┌──────────────────────────────────────┐
│  server/services/insights_service.go │
│                                      │
│  GetInsightsSummary(req) → resp      │
│    reads TokenStore snapshot         │
│    applies time filter               │
│    computes DailyRollup, ModelBreakdown│
│    joins to stapler-squad sessions   │
│                                      │
│  ListSessionTokens(req) → resp       │
│    paginated session-level summaries │
│                                      │
│  WatchInsights(req) → stream         │
│    pushes update on TokenStore change│
└──────────────────────────────────────┘
          │
          │  ConnectRPC over HTTP / WebSocket
          ▼
┌──────────────────────────────────────────────────────────┐
│  web-app/src/app/insights/  (new Next.js route)          │
│                                                          │
│  useInsightsService hook                                 │
│  │  createClient(InsightsService, transport)             │
│  │  GetInsightsSummary called on mount + filter change   │
│                                                          │
│  InsightsDashboard (page.tsx)                            │
│  ├── FilterBar (time range, model, session)              │
│  ├── SummaryCards (total tokens, total cost, cache rate) │
│  ├── SessionsTable (sortable, cost + token columns)      │
│  ├── DailySpendChart (recharts LineChart)                │
│  ├── ModelBreakdownChart (recharts BarChart/PieChart)    │
│  ├── TopSkillsTable                                      │
│  └── ExportButton (CSV download, client-side)            │
│                                                          │
│  TokenBadge component (injected into SessionCard)        │
│  └── shows "42K tokens · $0.03" on session cards        │
└──────────────────────────────────────────────────────────┘
```

### Data Flow

```
JSONL file (disk)
  → fsnotify detects write → enqueue in TokenStore worker pool
  → Parser.ParseFile() → ParseResult{aggregated token counts}
  → TokenStore.cache[filePath] = {result, modTime}
  → InsightsService.GetInsightsSummary() reads all cached results
  → applies time/model filter, builds SessionTokenSummary[]
  → ConnectRPC response → useInsightsService hook → React state
  → InsightsDashboard renders charts + tables
```

### TokenStore in the Existing Architecture

`TokenStore` sits parallel to `AnalyticsStore` (the existing in-memory analytics aggregator in `server/services/analytics_store.go`). Both are dependency-injected into their respective services. `InsightsService` receives a `*TokenStore` reference; `SessionService` is not modified. The `TokenStore` registers its own `HistoryFileWatcher` callback, independent of `HistoryLinker`'s existing callback.

---

## 2. Data Model

### 2.1 Go Structs (`session/tokens/`)

```go
// Package tokens provides JSONL-based token usage parsing and aggregation
// for Claude Code sessions. ParseResult never stores message content.
package tokens

import "time"

// ParseResult holds aggregated token data extracted from one JSONL file.
// Privacy: only tool names, skill names (short strings), and token counts.
// Message content is never stored.
type ParseResult struct {
    SessionUUID      string
    ProjectPath      string            // decoded from project dir name (best-effort)
    PrimaryModel     string            // most-used model in this session
    Models           []string          // all distinct models observed
    TotalInput       int64
    TotalOutput      int64
    CacheCreation    int64
    CacheRead        int64
    MessageCount     int
    TurnTimeline     []TurnStats       // per-assistant-message stats for burn rate chart
    ToolUsage        map[string]ToolTokenStats
    SkillActivations []SkillActivation
    ParsedAt         time.Time
    FileModTime      time.Time         // used for cache invalidation
}

// TurnStats is per-assistant-message token data (for timeline/burn-rate chart).
type TurnStats struct {
    Timestamp    time.Time
    Model        string
    Input        int64
    Output       int64
    CacheCreation int64
    CacheRead    int64
    ToolNames    []string // tool_use block names in this message
}

// ToolTokenStats aggregates attribution for one tool name.
// Token attribution is message-level (not per-tool-call); CallCount is exact.
type ToolTokenStats struct {
    ToolName  string
    CallCount int
    // MCPServer is non-empty when tool follows mcp__<server>__<tool> pattern.
    MCPServer string
}

// SkillActivation records a detected skill or command invocation.
type SkillActivation struct {
    Name      string // e.g. "code-review", "/plan:feature"
    TurnIndex int    // which human turn triggered it
    IsCommand bool   // true for /command, false for skill name
}

// ModelPricing holds per-model token prices in USD per million tokens.
type ModelPricing struct {
    ModelFamily      string  // normalized key, e.g. "claude-sonnet-4"
    InputPricePerMTok   float64 // USD per 1M input tokens
    OutputPricePerMTok  float64 // USD per 1M output tokens
    CacheWritePerMTok   float64 // USD per 1M cache-write tokens
    CacheReadPerMTok    float64 // USD per 1M cache-read tokens
    EffectiveDate    string  // ISO date of last price update
}

// PricingTable maps normalized model family names to pricing.
// Hardcoded defaults; overridable via config JSON.
type PricingTable struct {
    Prices      map[string]ModelPricing
    LoadedAt    time.Time
    ConfigPath  string // empty = hardcoded only
}

// EstimateCost computes USD cost for a ParseResult using the PricingTable.
func (pt *PricingTable) EstimateCost(r *ParseResult) float64

// NormalizeModelFamily strips date/variant suffixes and normalizes to
// a pricing-table key. Examples:
//   "claude-sonnet-4-6"               → "claude-sonnet-4"
//   "claude-sonnet-4-6-20250514"      → "claude-sonnet-4"
//   "claude-opus-4-7"                 → "claude-opus-4"
//   "claude-3-opus-20240229"          → "claude-opus-3"
func NormalizeModelFamily(modelID string) string
```

### 2.2 Proto Messages (`proto/session/v1/insights.proto`)

```protobuf
syntax = "proto3";
package session.v1;

import "google/protobuf/timestamp.proto";

// InsightsService provides token usage analytics derived from JSONL transcripts.
service InsightsService {
  // GetInsightsSummary returns aggregated token and cost data for a time range.
  rpc GetInsightsSummary(GetInsightsSummaryRequest)
      returns (GetInsightsSummaryResponse) {}

  // ListSessionTokens returns per-session token summaries with pagination.
  rpc ListSessionTokens(ListSessionTokensRequest)
      returns (ListSessionTokensResponse) {}

  // WatchInsights streams summary updates when new JSONL data is parsed.
  rpc WatchInsights(WatchInsightsRequest)
      returns (stream InsightsEvent) {}
}

// SessionTokenSummary is the per-session aggregated token record.
message SessionTokenSummary {
  string session_id         = 1;  // stapler-squad session ID (may be empty for orphans)
  string conversation_id    = 2;  // JSONL conversation UUID
  string project_path       = 3;
  string primary_model      = 4;
  int64  total_input_tokens = 5;
  int64  total_output_tokens = 6;
  int64  cache_creation_tokens = 7;
  int64  cache_read_tokens  = 8;
  double estimated_cost_usd = 9;
  double cache_hit_rate     = 10; // cache_read / (input + cache_read)
  int32  message_count      = 11;
  google.protobuf.Timestamp first_message_at = 12;
  google.protobuf.Timestamp last_message_at  = 13;
  bool   is_orphan          = 14; // true = no matching stapler-squad session
  repeated string skill_activations = 15;
  repeated TopToolEntry top_tools   = 16;
}

// TopToolEntry records a tool name and its call count in a session.
message TopToolEntry {
  string tool_name  = 1;
  int32  call_count = 2;
  string mcp_server = 3; // non-empty for mcp__<server>__<tool>
}

// DailyTokenBucket aggregates token usage for one calendar day.
message DailyTokenBucket {
  google.protobuf.Timestamp date           = 1;
  int64  total_input_tokens   = 2;
  int64  total_output_tokens  = 3;
  int64  cache_read_tokens    = 4;
  double estimated_cost_usd   = 5;
  int32  session_count        = 6;
}

// ModelBreakdown aggregates token usage by model family.
message ModelBreakdown {
  string model_family         = 1;  // normalized, e.g. "claude-sonnet-4"
  int64  total_input_tokens   = 2;
  int64  total_output_tokens  = 3;
  int64  cache_read_tokens    = 4;
  double estimated_cost_usd   = 5;
  int32  session_count        = 6;
}

// TopEntry is a generic name/value pair for top-N tables.
message TopEntry {
  string name          = 1;
  int64  token_count   = 2;
  int32  activation_count = 3;
  double cost_usd      = 4;
}

// GetInsightsSummaryRequest filters the summary response.
message GetInsightsSummaryRequest {
  google.protobuf.Timestamp from              = 1;
  google.protobuf.Timestamp to                = 2;
  optional string model_filter                = 3;
  optional string session_id_filter           = 4;
  bool            include_orphans             = 5;
}

// GetInsightsSummaryResponse returns the full dashboard dataset.
message GetInsightsSummaryResponse {
  repeated SessionTokenSummary sessions  = 1;
  double   total_cost_usd                = 2;
  int64    total_input_tokens            = 3;
  int64    total_output_tokens           = 4;
  int64    total_cache_read_tokens       = 5;
  double   overall_cache_hit_rate        = 6;
  repeated DailyTokenBucket daily        = 7;
  repeated ModelBreakdown models         = 8;
  repeated TopEntry top_skills           = 9;
  repeated TopEntry top_tools            = 10;
  bool     is_loading                    = 11; // true = background parse still in progress
  google.protobuf.Timestamp pricing_as_of = 12;
}

// ListSessionTokensRequest supports paginated session listing.
message ListSessionTokensRequest {
  google.protobuf.Timestamp from = 1;
  google.protobuf.Timestamp to   = 2;
  string sort_by                 = 3; // "cost" | "tokens" | "date" (default: "date")
  bool   sort_desc               = 4;
  int32  page_size               = 5;
  string page_token              = 6;
}

// ListSessionTokensResponse returns paginated session summaries.
message ListSessionTokensResponse {
  repeated SessionTokenSummary sessions = 1;
  string next_page_token                = 2;
  int32  total_count                    = 3;
}

// WatchInsightsRequest initiates a streaming subscription.
message WatchInsightsRequest {
  google.protobuf.Timestamp from = 1;
  google.protobuf.Timestamp to   = 2;
}

// InsightsEvent is pushed when TokenStore processes a new or updated JSONL file.
message InsightsEvent {
  string event_type                       = 1; // "update" | "parse_complete"
  optional SessionTokenSummary session    = 2;
  bool   all_parsed                       = 3;
}
```

---

## 3. Epic / Story Breakdown

### Epic 1: Go-Side Token Parser & Store

**E1-S1: JSONL Parser**
- New file: `session/tokens/parser.go`
- `Parser.ParseFile(filePath string) (*ParseResult, error)`
- `bufio.Scanner` with `scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)` (10MB max line)
- Per-line: `json.Unmarshal` into a minimal struct; skip on error
- Extract from `assistant` messages: `message.usage`, `message.model`, `message.content[].type=="tool_use"` name fields
- Extract `timestamp`, `uuid`, `sessionId`, `isSidechain` from outer JSONL object
- Privacy: never retain `.content` values beyond extracting tool names

**E1-S2: Skill/Command Detection**
- New file: `session/tokens/skill_detector.go`
- On `user` message turns: scan `message.content` for `/[a-zA-Z][\w:-]*` patterns → `IsCommand: true`
- Detect skill file reads: scan `tool_result` content blocks for `~/.claude/skills/` path patterns
- Detect `attachment` type messages with `attachment.type == "skill_listing"` for skill name extraction
- Returns `[]SkillActivation` appended to `ParseResult`

**E1-S3: TokenStore with fsnotify-based Cache Invalidation**
- New files: `session/tokens/store.go`, `session/tokens/store_test.go`
- `TokenStore` struct: `sync.RWMutex`, `map[string]cachedEntry{result *ParseResult, modTime time.Time}`
- `NewTokenStore(historyBaseDir string) *TokenStore`
- `Start(ctx context.Context)` — launches background walker + worker pool (4 goroutines)
- `GetAll() []*ParseResult` — snapshot of all cached results under read lock
- `GetByUUID(conversationUUID string) *ParseResult` — O(1) lookup via secondary UUID index
- Background walker: `filepath.WalkDir(historyBaseDir, ...)` → enqueue uncached or stale files
- fsnotify callback: `OnHistoryFileChanged(filePath string)` → enqueue for re-parse
- Cache key: file path; invalidation: stored `modTime != stat.ModTime()`
- Parse deduplication: `sync.Map` of in-flight paths to prevent duplicate concurrent parses

**E1-S4: Pricing Table**
- New file: `session/tokens/pricing.go`
- `DefaultPricingTable() *PricingTable` — hardcoded prices as of 2026-05-15
- Hardcoded entries: claude-opus-4 ($5/$25/$6.25/$0.50), claude-sonnet-4 ($3/$15/$3.75/$0.30), claude-haiku-4 ($1/$5/$1.25/$0.10), claude-opus-3 ($15/$75/$18.75/$1.50), claude-sonnet-3 ($3/$15/$3.75/$0.30)
- `LoadPricingOverride(configPath string) (*PricingTable, error)` — merge from JSON file
- `NormalizeModelFamily(modelID string) string` — prefix-match normalization (see pitfalls.md)
- `EstimateCost(r *ParseResult) float64` — apply per-token rates, return total USD
- `IsStale() bool` — returns true when `EffectiveDate` is > 30 days old

**E1-S5: Session Association**
- New file: `session/tokens/association.go`
- `Associator` struct with access to `session.Storage` (or snapshot of session IDs)
- `Associate(result *ParseResult) (sessionID string, isOrphan bool)`
- Lookup strategy (in order):
  1. Match `result.SessionUUID` to `ClaudeSession.conversation_id` in ent DB
  2. Decode project dir name → path, match sessions with `.path` prefix
  3. Timestamp fallback: JSONL `FileModTime` within ±5 minutes of `Session.created_at`
- Orphan: no match found → `isOrphan = true`, still included in dashboard

---

### Epic 2: ConnectRPC API

**E2-S1: Proto Definition**
- New file: `proto/session/v1/insights.proto`
- Full service and message definitions per §2.2 above
- Run `make generate-proto` → regenerates Go + TypeScript bindings

**E2-S2: InsightsService Implementation**
- New file: `server/services/insights_service.go`
- `InsightsService` struct: holds `*tokens.TokenStore`, `*tokens.PricingTable`, `session.Storage`
- `NewInsightsService(store *tokens.TokenStore, pricing *tokens.PricingTable, storage session.Storage) *InsightsService`
- `GetInsightsSummary`: read TokenStore snapshot, filter by time/model, join to sessions, build `DailyTokenBucket` series by day, compute `ModelBreakdown`, aggregate `TopEntry` for skills and tools, return `is_loading` flag if background parse is still running
- `ListSessionTokens`: paginated slice from sorted snapshot
- `WatchInsights`: goroutine loops, blocks on `TokenStore.Subscribe()` channel, pushes `InsightsEvent` on change; ctx cancel exits

**E2-S3: Service Registration**
- Modify `server/server.go`: wire `InsightsService` into server deps
- Modify `server/deps.go` (or equivalent wiring file): add `InsightsService` field, initialize from `TokenStore`
- Registration pattern (follows `UnfinishedWorkService` pattern in server.go lines 311–315):
  ```go
  insightsPath, insightsHandler := sessionv1connect.NewInsightsServiceHandler(
      deps.InsightsService, ConnectOptions(deps.ErrorRegistry)...)
  srv.RegisterConnectHandler("/api"+insightsPath,
      http.StripPrefix("/api", insightsHandler))
  ```

---

### Epic 3: React Insights Dashboard

**E3-S1: Install recharts**
- `cd web-app && npm install recharts`
- Confirm bundle size impact (recharts ~200KB gzip); add `size-limit` entry if needed
- Verify no CSS-in-JS conflict with vanilla-extract

**E3-S2: useInsightsService Hook**
- New file: `web-app/src/lib/hooks/useInsightsService.ts`
- Pattern: `createClient(InsightsService, transport)` following `useSessionService.ts`
- Import from `@/gen/session/v1/insights_pb` (generated by proto step)
- Exports: `getInsightsSummary(req)`, `listSessionTokens(req)`, `watchInsights(req)`
- Use standard `createConnectTransport` (no streaming needed for summary calls)

**E3-S3: TokenBadge Component**
- New files: `web-app/src/components/insights/TokenBadge.tsx`, `TokenBadge.css.ts`
- Props: `tokens: number, costUsd?: number, model?: string, overBudget?: boolean`
- Renders: `"42K · $0.03"` with model icon; red variant when `overBudget`
- Injected into `SessionCard` component (modify existing file)
- Data source: `InsightsService.GetInsightsSummary` filtered to current session ID, loaded lazily

**E3-S4: /insights Page Scaffold**
- New files: `web-app/src/app/insights/page.tsx`, `web-app/src/app/insights/layout.tsx`, `web-app/src/app/insights/insights.css.ts`
- Feature marker: `// +feature: insights-dashboard` in first 10 lines of `page.tsx`
- URL param state: `?from=<iso>&to=<iso>&model=<family>&session=<id>` via `useSearchParams`
- Nav link: add `/insights` entry to the navigation component (same place `/history` link lives)
- "use client" directive

**E3-S5: Sessions Table**
- New file: `web-app/src/components/insights/SessionsTable.tsx` + `.css.ts`
- Columns: Date, Session Name, Path, Model, Input Tokens, Output Tokens, Cache Rate, Est. Cost
- Sortable by all numeric columns (client-side sort on loaded data)
- Rows link to session detail; orphan rows show "(untracked)" label
- Data from `GetInsightsSummaryResponse.sessions`

**E3-S6: Daily Spend Chart**
- New file: `web-app/src/components/insights/DailySpendChart.tsx` + `.css.ts`
- recharts `ResponsiveContainer` + `LineChart`
- X-axis: date; Y-axis: USD cost
- Tooltip: shows cost + token breakdown per day
- Data from `GetInsightsSummaryResponse.daily`

**E3-S7: Top-N Tables**
- New file: `web-app/src/components/insights/TopNTables.tsx` + `.css.ts`
- Two sections: "Top Skills & Commands" and "Top Tools"
- Each shows rank, name, activation count, estimated token cost
- Data from `GetInsightsSummaryResponse.top_skills` and `top_tools`
- MCP server entries grouped by server prefix under "Top MCP Servers" sub-section

**E3-S8: Model Breakdown Chart**
- New file: `web-app/src/components/insights/ModelBreakdownChart.tsx` + `.css.ts`
- recharts `BarChart` (stacked: input vs output vs cache-read tokens per model family)
- Secondary: `PieChart` showing cost share by model
- Data from `GetInsightsSummaryResponse.models`

**E3-S9: Budget Alert Logic + Red Badge**
- Modify `config/config.go`: add `TokenBudget` struct with `WarnThreshold int64`, `HardStopThreshold int64` per session (optional, per session ID or global default)
- `TokenBadge` checks `tokens >= warnThreshold` → yellow; `>= hardStopThreshold` → red
- Dashboard `SummaryCards` shows "OVER BUDGET" banner if any session exceeds hard threshold
- No actual hard-stop enforcement (that is out of scope); only visual alert

**E3-S10: CSV Export**
- New file: `web-app/src/components/insights/ExportButton.tsx` + `.css.ts`
- Client-side: build CSV string from `GetInsightsSummaryResponse.sessions`
- Columns: date, session_id, conversation_id, path, model, input_tokens, output_tokens, cache_read_tokens, estimated_cost_usd
- `Blob` + `URL.createObjectURL` download pattern (same as history export in `history/page.tsx`)
- Filename: `insights-<from>-<to>.csv`

---

## 4. Task List (Flat, Ordered for Implementation)

Dependencies: tasks within Epic 1 must complete before Epic 2; Epic 2 proto step (T-07) must complete before Epic 3 hook (T-15). All E3 UI tasks depend on T-15.

| # | Task | Files | Complexity | Depends On |
|---|------|-------|------------|------------|
| T-01 | Create `session/tokens/` package skeleton + package doc | `session/tokens/doc.go` | S | — |
| T-02 | Implement JSONL line struct (minimal unmarshal target) | `session/tokens/jsonl_types.go` | S | T-01 |
| T-03 | Implement `Parser.ParseFile()` with 10MB scanner buffer | `session/tokens/parser.go` | M | T-02 |
| T-04 | Unit tests for parser: normal file, partial line, empty file, corrupted last line | `session/tokens/parser_test.go` | M | T-03 |
| T-05 | Implement `SkillDetector` (slash-command + skill file read patterns) | `session/tokens/skill_detector.go` | S | T-02 |
| T-06 | Implement `PricingTable` with hardcoded defaults + `NormalizeModelFamily` | `session/tokens/pricing.go` | S | T-01 |
| T-07 | Unit tests for `NormalizeModelFamily` with date-suffixed and legacy model IDs | `session/tokens/pricing_test.go` | S | T-06 |
| T-08 | Implement `TokenStore`: cache map, RWMutex, worker pool (4 goroutines), `Start/Stop` | `session/tokens/store.go` | L | T-03, T-05 |
| T-09 | Implement `TokenStore.Subscribe()` notification channel for Watch RPC | `session/tokens/store.go` | S | T-08 |
| T-10 | Unit tests for `TokenStore`: cache hit/miss, concurrent parse dedup, stale invalidation | `session/tokens/store_test.go` | M | T-08 |
| T-11 | Implement `Associator`: conversation_id lookup + path decode + timestamp fallback | `session/tokens/association.go` | M | T-01 |
| T-12 | Write `proto/session/v1/insights.proto` (all messages + InsightsService) | `proto/session/v1/insights.proto` | M | T-01 |
| T-13 | Run `make generate-proto`; commit generated Go + TS bindings | `session/gen/`, `web-app/src/gen/` | S | T-12 |
| T-14 | Implement `InsightsService`: `GetInsightsSummary`, `ListSessionTokens`, `WatchInsights` | `server/services/insights_service.go` | L | T-08, T-11, T-13 |
| T-15 | Unit tests for `InsightsService`: filter logic, daily rollup, model breakdown, orphan handling | `server/services/insights_service_test.go` | M | T-14 |
| T-16 | Register `InsightsService` in server deps + `server.go` handler registration | `server/server.go`, `server/deps.go` | S | T-14 |
| T-17 | Wire `TokenStore` startup into `main.go` / server bootstrap | `main.go` or server init file | S | T-08, T-16 |
| T-18 | Manual integration smoke test: start server, check `/insights` RPC returns data | — | S | T-17 |
| T-19 | `npm install recharts` + verify bundle size within size-limit budget | `web-app/package.json`, `web-app/package-lock.json` | S | T-13 |
| T-20 | Create `useInsightsService` hook following `useSessionService` pattern | `web-app/src/lib/hooks/useInsightsService.ts` | M | T-13, T-19 |
| T-21 | Create `/insights` page scaffold with filter bar and URL param state | `web-app/src/app/insights/page.tsx`, `layout.tsx`, `insights.css.ts` | M | T-20 |
| T-22 | Add `/insights` nav link to navigation component | existing nav component file | S | T-21 |
| T-23 | Implement `SummaryCards` component (total cost, tokens, cache rate) | `web-app/src/components/insights/SummaryCards.tsx`, `.css.ts` | S | T-20 |
| T-24 | Implement `SessionsTable` with sortable columns and orphan row label | `web-app/src/components/insights/SessionsTable.tsx`, `.css.ts` | M | T-20 |
| T-25 | Implement `DailySpendChart` (recharts LineChart) | `web-app/src/components/insights/DailySpendChart.tsx`, `.css.ts` | M | T-19, T-20 |
| T-26 | Implement `ModelBreakdownChart` (recharts BarChart + PieChart) | `web-app/src/components/insights/ModelBreakdownChart.tsx`, `.css.ts` | M | T-19, T-20 |
| T-27 | Implement `TopNTables` (skills/commands + tools + MCP grouping) | `web-app/src/components/insights/TopNTables.tsx`, `.css.ts` | M | T-20 |
| T-28 | Assemble components into `InsightsDashboard` page | `web-app/src/app/insights/page.tsx` | M | T-23, T-24, T-25, T-26, T-27 |
| T-29 | Implement `TokenBadge` component (vanilla-extract, red variant) | `web-app/src/components/insights/TokenBadge.tsx`, `.css.ts` | S | T-20 |
| T-30 | Inject `TokenBadge` into existing `SessionCard` component | existing `SessionCard` component file | S | T-29 |
| T-31 | Add budget config to `config.go` + `TokenBadge` alert threshold logic | `config/config.go`, `TokenBadge.tsx` | S | T-29 |
| T-32 | Implement `ExportButton` (CSV download, client-side Blob) | `web-app/src/components/insights/ExportButton.tsx`, `.css.ts` | S | T-20 |
| T-33 | Jest unit tests for `TokenBadge` (normal / warning / over-budget states) | `TokenBadge.test.tsx` | S | T-29 |
| T-34 | Jest unit tests for `useInsightsService` (mock ConnectRPC client) | `useInsightsService.test.ts` | S | T-20 |
| T-35 | Playwright e2e test: `/insights` page loads, sessions table renders, filter changes URL | `tests/e2e/insights-dashboard.spec.ts` | M | T-28 |
| T-36 | Add `insights-dashboard` entry to `docs/registry/frontend-features.json` | `docs/registry/frontend-features.json` | S | T-28 |
| T-37 | Add `InsightsService` RPCs to `docs/registry/backend-features.json` | `docs/registry/backend-features.json` | S | T-16 |
| T-38 | Run `make quick-check` + fix any lint/vet issues; verify `coverage-gaps.json` unchanged | — | S | T-35, T-37 |

**Total: 38 tasks**

---

## 5. Technology Decisions

### Chart Library: recharts

**Decision:** Install `recharts` as a new npm dependency.

**Justification:**
- No charting library currently exists in `web-app/package.json`. This is the first charting use in the project.
- recharts renders SVG via React components — fully compatible with React 19 and Next.js 15 (no class component API issues).
- `ResponsiveContainer` handles the responsive layout requirement automatically.
- vanilla-extract theming can be applied to wrapper `div` elements; chart colors are passed as props (no CSS-in-JS conflict).
- ~200KB gzipped is within the existing "Total JS bundle" size-limit of 5MB (currently well under this cap).
- The required chart types (`LineChart`, `BarChart`, `PieChart`) are all native recharts components.
- Alternative considered: custom SVG charts. Rejected because the `DailySpendChart` requires axis rendering, tooltips, and responsive sizing that would be significant build effort for no meaningful bundle savings.

### No Ent Schema Changes

**Decision:** Token data is not persisted to the ent SQLite database in Phase 1.

**Justification:**
- JSONL is the source of truth. Persisting derived data to SQLite creates a stale-data risk.
- The in-memory `TokenStore` pattern (following `AnalyticsStore` in `analytics_store.go`) satisfies NFR-2 (< 2s for 90 days) via background pre-parsing.
- No schema migration = no downtime risk and no ent generate complexity.
- Optional persistence (e.g., a cached `total_tokens` field on `ClaudeSession`) deferred to Phase 2 if profiling shows the badge-loading path is too slow.

### On-Demand + Background Preload Hybrid

**Decision:** `TokenStore` pre-parses all JSONL files in a background goroutine on server startup; RPC calls return cached results immediately, with `is_loading: true` if background parse is not yet complete.

**Justification:**
- NFR-1: 10k+ line JSONL must not block the UI → background parse moves heavy I/O off the RPC handler path.
- NFR-2: Dashboard loads in < 2s for 90 days → pre-parsing satisfies this because 100 × 1.3MB ÷ 200MB/s ÷ 4 worker goroutines ≈ 163ms total parse time at server startup.
- On-demand fallback: files detected after startup (new sessions) are queued via the `HistoryFileWatcher` callback within seconds of session end.
- The `is_loading` flag lets the UI show a "Still scanning..." notice rather than blank state.

### Privacy Boundary: ParseResult Never Stores Message Content

**Decision:** `ParseResult` and all derived proto messages carry only token counts, tool names (e.g., `"Bash"`, `"mcp__datadog__search_logs"`), skill names (short `/command` strings), and aggregated statistics. The actual text of user prompts, assistant responses, file contents, or command outputs is never placed in `ParseResult`.

**Justification:**
- NFR-3: Token data must not be sent to any external service without explicit user opt-in.
- The existing OpenTelemetry/Datadog integration in stapler-squad captures spans. If message content were in `ParseResult`, it would risk leaking into telemetry. By keeping the privacy boundary at the data model level, this risk is eliminated structurally.
- The pitfalls research confirms: only tool names (short strings) are needed for the features in scope.
- Enforced at code review: `ParseResult` fields are documented as "no message content" in package-level doc comment.

---

## 6. Out of Phase 1 (Deferred)

### Real-Time Token Streaming During Active Session
The requirements mark this as out of scope explicitly. Post-session analysis (parse JSONL after fsnotify fires) is the Phase 1 approach. A future "live mode" would require a tail-follow parser and a websocket push from the running session's parser goroutine. Deferred.

### Subagent Spend Attribution
`isSidechain: true` messages in JSONL mark subagent branches. Subagent tasks may also spawn entirely separate JSONL files in different project directories. Phase 1 counts each JSONL file independently. Linking child sessions to parent sessions via `parentUuid` chain-walking is deferred to Phase 2. This avoids double-counting risk and premature complexity.

### Multi-User Aggregation
Stapler-squad is a single-user local tool. Multi-user team spend aggregation is out of scope.

### Anthropic Usage API Integration
The requirements list this as "live pricing fallback." The hardcoded pricing table with config override satisfies all Phase 1 requirements. Live API integration requires secure key storage and adds an external network dependency; deferred.

### JSON Export
CSV export (T-32) is included. JSON export for programmatic consumption is deferred as P3 per the features research.

### Persistent Token Storage in Ent
Adding `total_tokens` / `estimated_cost_usd` fields to the `ClaudeSession` ent entity is deferred. If the session-card badge loading path proves slow, this would speed up the badge (avoid a full `GetInsightsSummary` call per card). Deferred pending profiling.

---

## 7. Acceptance Criteria (Linked to Requirements)

| Requirement | Task(s) | Acceptance Test |
|------------|---------|----------------|
| **FR-1**: Parse JSONL, extract token fields, handle malformed lines | T-02, T-03, T-04 | `Parser.ParseFile()` on a 10k-line file returns correct totals; corrupted last line does not panic or return error |
| **FR-1**: Extract tool use metadata (tool name, MCP server) | T-02, T-03 | `ParseResult.ToolUsage` contains `Bash`, `Read`, and `mcp__datadog__*` entries from a real session file |
| **FR-1**: Extract `/command` and skill activations | T-05 | `SkillDetector` identifies `/plan:feature`, `/code:review` patterns and SKILL.md read paths |
| **FR-2**: Aggregate tokens at session level | T-03, T-08 | `TokenStore.GetByUUID()` returns correct `TotalInput`, `TotalOutput`, `CacheRead` matching manual sum of JSONL |
| **FR-2**: `TurnTimeline` for burn rate calculation | T-03 | `ParseResult.TurnTimeline` has one entry per assistant message with correct timestamp and token counts |
| **FR-3**: Cost calculation with pricing table | T-06, T-07 | `EstimateCost()` on a sampled `ParseResult` matches manual calculation within 0.01% |
| **FR-3**: Config pricing override | T-06 | `LoadPricingOverride()` with a test JSON file overrides one model's prices correctly |
| **FR-3**: Staleness flag at 30 days | T-06 | `PricingTable.IsStale()` returns `true` when `EffectiveDate` is 31 days ago |
| **FR-4**: Skills & commands detection | T-05 | Activation list populated for sessions known to have skill activations |
| **FR-5**: Session association by conversation_id | T-11 | Sessions with `ClaudeSession.conversation_id` set are matched correctly |
| **FR-5**: Orphan sessions in dashboard | T-14, T-24 | Sessions without stapler-squad records appear in `SessionsTable` with `is_orphan = true` |
| **FR-6**: `/insights` route in React SPA | T-21, T-22 | Navigating to `/insights` loads the dashboard; nav link visible in all pages |
| **FR-6**: Charts render with real data | T-25, T-26 | `DailySpendChart` and `ModelBreakdownChart` render with non-empty data from a real session corpus |
| **FR-6**: Persistent filter state in URL params | T-21 | Changing time range updates URL; reloading page restores same filter |
| **FR-7**: Budget alert badge turns red | T-29, T-31 | `TokenBadge` renders red variant when session tokens exceed configured `HardStopThreshold` |
| **FR-8**: CSV export | T-32 | Export button downloads CSV with correct headers and one row per session |
| **NFR-1**: 10k+ line JSONL does not block UI | T-08, T-14 | `GetInsightsSummary` RPC returns in < 100ms when all files are pre-cached; background parse handles 10k-line file without blocking |
| **NFR-2**: Dashboard loads < 2s for 90 days | T-08, T-35 | Playwright test: `/insights` fully renders within 2000ms with 90-day session corpus (stub or real) |
| **NFR-3**: No token data in external telemetry | T-14, T-38 | Code review confirms `InsightsService` emits no telemetry spans; `ParseResult` fields contain no message content |
| **NFR-4**: Pricing table does not require binary rebuild | T-06 | `LoadPricingOverride()` reads from a config-specified JSON path at runtime; binary unchanged |
| **Success criterion 1**: Dashboard shows total tokens, cost, model, top skills within 5s of session end | T-08, T-09, T-14 | `TokenStore` processes new JSONL file via fsnotify callback; `WatchInsights` push reaches frontend within 5 seconds of file close |
| **Success criterion 2**: 90-day load < 2s | T-08, T-35 | Covered by NFR-2 test above |
| **Success criterion 3**: Token counts match JSONL within 1% | T-03, T-04 | Unit test compares `ParseResult.TotalInput + TotalOutput + CacheRead` sum against manually tallied reference file |
| **Success criterion 4**: Budget alerts fire | T-29, T-31 | `TokenBadge` renders alert state; `SummaryCards` shows banner |

---

## Appendix: File Creation Summary

**New Go files:**
- `session/tokens/doc.go`
- `session/tokens/jsonl_types.go`
- `session/tokens/parser.go`
- `session/tokens/parser_test.go`
- `session/tokens/skill_detector.go`
- `session/tokens/pricing.go`
- `session/tokens/pricing_test.go`
- `session/tokens/store.go`
- `session/tokens/store_test.go`
- `session/tokens/association.go`
- `server/services/insights_service.go`
- `server/services/insights_service_test.go`

**New proto file:**
- `proto/session/v1/insights.proto`

**Generated (do not manually edit):**
- `session/gen/proto/go/session/v1/insights_pb.go` (and connect variant)
- `web-app/src/gen/session/v1/insights_pb.ts`

**New React files:**
- `web-app/src/app/insights/page.tsx`
- `web-app/src/app/insights/layout.tsx`
- `web-app/src/app/insights/insights.css.ts`
- `web-app/src/lib/hooks/useInsightsService.ts`
- `web-app/src/lib/hooks/useInsightsService.test.ts`
- `web-app/src/components/insights/SummaryCards.tsx` + `.css.ts`
- `web-app/src/components/insights/SessionsTable.tsx` + `.css.ts`
- `web-app/src/components/insights/DailySpendChart.tsx` + `.css.ts`
- `web-app/src/components/insights/ModelBreakdownChart.tsx` + `.css.ts`
- `web-app/src/components/insights/TopNTables.tsx` + `.css.ts`
- `web-app/src/components/insights/TokenBadge.tsx` + `.css.ts`
- `web-app/src/components/insights/TokenBadge.test.tsx`
- `web-app/src/components/insights/ExportButton.tsx` + `.css.ts`
- `tests/e2e/insights-dashboard.spec.ts`

**Modified files:**
- `server/server.go` (register InsightsService handler)
- `server/deps.go` or equivalent (add InsightsService + TokenStore to deps struct)
- `main.go` or server init (start TokenStore background goroutine)
- `config/config.go` (add TokenBudget config struct)
- existing `SessionCard` component (inject TokenBadge)
- existing navigation component (add /insights link)
- `docs/registry/backend-features.json`
- `docs/registry/frontend-features.json`
- `web-app/package.json` (add recharts)
