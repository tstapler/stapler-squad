# Implementation Plan: mcp-search-list-tools

**Feature**: Wrap three existing search/list/filter RPCs (`ListBacklogItems`, `GetNotificationHistory`, `SearchClaudeHistory`) as MCP tools so an LLM client can list/filter backlog items, query notification history, and full-text-search Claude/session history without a raw RPC/curl call.
**Date**: 2026-08-12
**Status**: Ready for implementation
**ADRs**: None — no non-standard technology choice is introduced (see Pattern Decisions; `mark3labs/mcp-go v0.48.0` is already the pinned, in-use MCP SDK).

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `backlogHandlers` | Existing struct (`server/mcp/tools_backlog.go:118`) holding the dependencies (`storage`, `backlogSvc`, `enabledCheck`, etc.) every backlog MCP tool handler needs. | Existing — `listBacklogItems` becomes a new method on it. |
| `listBacklogItems` | New handler method `func (h *backlogHandlers) listBacklogItems(ctx, req) (*mcpgo.CallToolResult, error)` backing the `list_backlog_items` MCP tool. | New. |
| `BacklogItemBrief` | New MCP-wire-only struct: a slimmed, JSON-serializable view of one `sessionv1.BacklogItem` (id, title, priority, status, repo_path, pr_url, pr_number, category, created_at, updated_at) — no description/AC/notes, matching `get_backlog_item`'s "richer detail lives behind a second call" convention. | New. Deliberately *not* named `BacklogItemSummary` — that name is already taken by `session.BacklogItemSummary` (`session/repository.go:516`); reusing it here would be confusing across packages. |
| `ListBacklogItemsResult` | New MCP wrapper struct: `{MCPResult, Items []BacklogItemBrief, TotalCount int, HasMore bool}`, returned by `okResult(...)`. | New. Field names (`total_count`, `has_more`) chosen to match the cross-tool naming convention in features.md §4. |
| `validBacklogStatuses` | New package-level `map[string]bool` (or `[]string`) enumerating the 9 valid `session.BacklogStatus*` string values, used both to build the `mcpgo.Items(...)` enum in the tool schema and to reject an unrecognized `status` value in the handler before it reaches the RPC (which would otherwise silently match zero rows — pitfalls.md §4). | New. |
| `notificationHandlers` | New struct `{svc *services.SessionService}` in new file `server/mcp/tools_notifications.go`, holding the dependency `getNotificationHistory` needs. | New — mirrors `workflowHandlers`/`rulesHandlers`' existing one-field shape. |
| `registerNotificationTools` | New func `func registerNotificationTools(s *mcpserver.MCPServer, h *notificationHandlers)`, called from `NewCore`'s existing `if svc != nil { ... }` block. | New. |
| `getNotificationHistory` | New handler method backing the `get_notification_history` MCP tool. | New. |
| `notificationTypeByName` | New package-level `map[string]sessionv1.NotificationType` mapping the 14 lowercase snake_case names (`"approval_needed"`, `"error"`, ... `"custom"`) used in the MCP schema's `type_filter` enum to their proto enum values — the explicit string→enum mapping pitfalls.md §4 requires (an unvalidated string silently resolves to `NOTIFICATION_TYPE_UNSPECIFIED`, which is wrong). | New. |
| `NotificationRecordBrief` | New MCP-wire struct: a slimmed view of one `sessionv1.NotificationHistoryRecord` (id, session_id, session_name, type, priority, title, message [sanitized], is_read, created_at, occurrence_count). | New. Not named `NotificationHistoryRecord` — that name is the proto message; using a distinct Go name avoids import-aliasing confusion in the handler. |
| `GetNotificationHistoryResult` | New MCP wrapper struct: `{MCPResult, Notifications []NotificationRecordBrief, TotalCount int, UnreadCount int, HasMore bool, Offset int}`. | New. |
| `searchHandlers` | New struct `{svc *services.SessionService}` in new file `server/mcp/tools_search.go`. | New — same one-field shape as `notificationHandlers`. |
| `registerSearchTools` | New func `func registerSearchTools(s *mcpserver.MCPServer, h *searchHandlers)`, called from `NewCore`'s `if svc != nil { ... }` block. | New. |
| `searchClaudeHistory` | New handler method backing the `search_claude_history` MCP tool. | New. Distinct from the existing RPC method `(*services.SearchService).SearchClaudeHistory` / `(*services.SessionService).SearchClaudeHistory` it wraps — same name is fine since it lives on a different receiver type (`*searchHandlers`) in a different package. |
| `SearchResultBrief` | New MCP-wire struct: a slimmed view of one `sessionv1.SearchResult` (session_id, session_name, project, message_index, score, snippets []`SearchSnippetBrief`, metadata). | New. |
| `SearchSnippetBrief` | New MCP-wire struct: `{Text string [sanitized via SanitizeForAgentContext], MessageRole string, MessageTime time.Time}` — the one field (`Text`) pitfalls.md/features.md flag as unbounded and requiring truncation. | New. |
| `SearchClaudeHistoryResult` | New MCP wrapper struct: `{MCPResult, Results []SearchResultBrief, TotalMatches int, QueryTimeMs int64, HasMore bool, Offset int}`. | New. |
| `workflowServiceErrResult` | Existing func (`server/mcp/tools_workflow.go:375`) mapping a `*connect.Error` to an MCP error result (NotFound/InvalidArgument/Unavailable → their MCP equivalents, else Internal). | Existing — already reused by `tools_rules.go` across a different domain (workflow ⇄ approval rules); this plan reuses it a third time from `tools_notifications.go`/`tools_search.go` rather than writing near-duplicate mapping funcs. |
| `int32PtrArg`, `stringPtrArg`, `boolPtrArg`, `stringArg`, `boolArg` | Existing arg-coercion helpers (`server/mcp/tools_workflow.go:338-371`) that read a typed value out of the untyped `map[string]any` MCP arguments, returning a pointer/nil for "not supplied". | Existing — reused as-is for optional proto fields (`Limit *int32`, `Offset *int32`, `SessionId *string`, `UnreadOnly *bool`, `Project *string`, `Model *string`). |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall tool-surface shape | Resource-scoped tools, one per RPC (`list_backlog_items`, `get_notification_history`, `search_claude_history`) | architecture.md §5, features.md — repo's own 55-tool convention | Single unified cross-resource `search` tool with a discriminated-union input schema | The three RPCs' request shapes share almost no fields (`query` only on history search; `status`/`priority` only on backlog; `type_filter`/`unread_only` only on notifications) — a discriminated-union schema would worsen LLM tool-selection accuracy and break the repo's uniform verb_noun single-purpose-tool convention (every one of the 55 existing tools is resource-scoped). |
| Tool-generation approach | Hand-write following the existing in-repo pattern | build-vs-buy.md §1/§3 | Proto-to-MCP codegen (`protoc-gen-go-mcp`, `grpc-mcp-gateway`) | Both operate at whole-service granularity and have no hook for this repo's per-tool custom descriptions, feature-flag gating, session-UUID injection, or error-shape conventions. Introducing a second codegen pipeline for 3 tools while 55 stay hand-written would itself be an unjustified-complexity violation of `.claude/rules/interface-pollution-checklist.md`. |
| MCP handler → backend call path | Pattern B: call the existing `*services.BacklogService`/`*services.SessionService` method in-process via `connect.NewRequest(...)` (PoEAA: reuse the existing **Service Layer**, treat the MCP handler as a thin presentation adapter — Transaction-Script-shaped, matching every other MCP handler in this file) | architecture.md §1, §5 | Pattern A: bypass ConnectRPC and call `session.Storage`/domain layer directly (as `search_sessions`/`get_backlog_item` do) | `ListBacklogItems`'s RPC does non-trivial proto-conversion work (`backlogItemSummaryToProto` + the **unexported** `BacklogService.buildCostLookup()`) beyond a raw storage call — Pattern A would require either exporting `buildCostLookup()` (new surface area for one caller, a speculative-export smell) or duplicating cost computation. `GetNotificationHistory`/`SearchClaudeHistory` have no storage-layer shortcut at all (they read from `*notifications.NotificationHistoryStore` / the FTS `SearchEngine`, not `session.Storage`). |
| String→enum validation (`status`, `type_filter`) | A package-level `map[string]T` lookup table (`validBacklogStatuses`, `notificationTypeByName`), checked in the handler *and* mirrored into the tool schema via `mcpgo.Items(map[string]any{"enum": [...]})`/`mcpgo.Enum(...)` | pitfalls.md §4, requirements.md "Enum discoverability" | A GoF Strategy/Factory abstraction for "resolve a filter value" | The problem doesn't recur (each has one concrete instance — a Go map is data, not a design pattern) and there is exactly one caller of each map. Per `.claude/rules/interface-pollution-checklist.md` smell #5 (unjustified generic/abstraction), a plain map lookup is the correct-weight solution. |
| New Go types for `limit`/`offset` pairs | None — reuse `int32PtrArg`/plain `int32` per the proto field's own optionality, looked up from the MCP args map by string key (`args["limit"]`, `args["offset"]`) | `.claude/rules/primitive-obsession-checklist.md` | A `Pagination{Limit, Offset int32}` newtype | The primitive-obsession checklist's specific harm is a **positional** same-typed-parameter swap at a Go call site (`f(a, b int)` → `f(b, a)` compiles silently wrong). MCP arguments are looked up by string key from an untyped map, not passed positionally — the swap hazard the checklist exists to prevent does not apply here. Introducing a newtype for two fields used once each, in two different handlers, with no shared behavior, would itself be an unjustified type per the same checklist's spirit. |
| Result envelope shape | Structured JSON via existing `okResult(v any)`/`errResult(code, message, remediation)` helpers (`tools_discovery.go:73-85`), typed `...Result` struct embedding `MCPResult` | features.md §1 "Recommendation" | Human-readable Markdown (the `get_backlog_item` pattern) | `get_backlog_item` is Markdown because it returns one rich item with role-aware workflow guidance meant to be read directly by an LLM as instructions. These three tools return **lists/pages of results** for programmatic filtering (matching `list_sessions`/`search_sessions`'s existing JSON-envelope precedent) — Markdown would make `total_count`/`has_more`/pagination fields awkward to consume reliably. |

---

## Migration Plan

Omitted — no schema or data changes. All three backing RPCs already exist and are already exercised by the web UI; this plan adds only MCP tool registrations (Go code) with no proto changes, no `make proto-gen`, and no `docs/registry/` entries (registry tracks RPCs/components, not MCP tool wrappers — confirmed pitfalls.md §3).

## Observability Plan
- **Logs**: No new logging. `list_sessions`/`search_sessions`/`list_workflows` (the closest precedent read-only tools) log nothing per-call; the three new tools follow the same precedent. Errors already surface through the returned `MCPResult.Error` envelope, which is sufficient for a stateless read tool.
- **Metrics**: None new. No existing MCP tool in this package emits metrics; adding instrumentation for exactly these three would be an inconsistent one-off.
- **Alerts**: None new.

## Risk Control
- **Feature flag**: `list_backlog_items` inherits the existing `backlogEnabled` flag automatically — it is registered inside `registerBacklogTools`, which `NewCore` only calls when `storage != nil && (backlogEnabled == nil || backlogEnabled())` (`server/mcp/server.go:63-66`), and its handler starts with the same `featureDisabledResult(h.enabledCheck)` guard every other backlog tool uses. `get_notification_history`/`search_claude_history` have **no** feature flag — they register whenever `svc != nil`, identical to `create_workflow`/`list_approval_rules` today. This is a known, accepted asymmetry (pitfalls.md §5 names it explicitly as "worth flagging in review," not a defect to fix in this pass) — no new flag is introduced for them, matching precedent rather than inventing a bespoke one-off gate.
- **Rollback procedure**: Purely additive — no existing tool's registration, schema, or handler is modified (only `registerBacklogTools` gains one more `s.AddTool(...)` call; two new files add their own `register*Tools` funcs). Rollback is reverting the commit / redeploying the previous binary; no data cleanup is needed since all three tools are read-only and write nothing to disk.
- **Staged rollout**: None applicable — this codebase has no per-tool gradual-rollout mechanism for read-only MCP tools (the only precedent, `backlogEnabled`, is an all-or-nothing settings toggle, not a rollout percentage). The new tools take effect on the next server restart, same as any other MCP tool addition.

## Unresolved Questions

None. All open questions from requirements.md were resolved by research (see requirements.md's "Open questions carried into research/planning — ALL RESOLVED BY RESEARCH" section).

## Dependency Visualization

```
Epic 1.1 (list_backlog_items)                Epic 1.2 (get_notification_history)         Epic 1.3 (search_claude_history)
  1.1.1a types+validation                      1.2.1a new file + struct skeleton           1.3.1a new file + struct skeleton
      |                                              |                                            |
  1.1.1b handler                                1.2.1b enum map + result types              1.3.1b result types
      |                                              |                                            |
  1.1.1c schema registration                    1.2.1c handler                              1.3.1c handler
      |                                              |                                            |
  1.1.1d tests                                  1.2.1d schema registration                  1.3.1d schema registration
      |                                              |                                            |
  1.1.2a verify dangling-ref string (AC4)       1.2.1e wire into NewCore (server.go)         1.3.1e wire into NewCore (server.go)
      |                                              |                                            |
      |                                         1.2.1f tests                                 1.3.1f tests
      |                                              |                                            |
      +----------------------------------------------+--------------------------------------------+
                                                       |
                                          1.4.1a full-suite verification (depends on all above)
```

Epics 1.1, 1.2, and 1.3 have no cross-dependencies and can be implemented in any order (or in parallel by different subagents) — they touch disjoint files except for the single shared edit to `server/mcp/server.go` (Tasks 1.2.1e and 1.3.1e both add one line inside the same `if svc != nil { }` block; sequence those two tasks, not the whole epics).

---

## Phase 1: MCP Search/List Tool Wiring

### Epic 1.1: List Backlog Items Tool
**Goal**: An LLM client can list/filter backlog items via MCP by status, priority, sort order, and terminal/archived visibility — closing AC1 and resolving the AC4 dangling reference.

#### Story 1.1.1: `list_backlog_items` MCP tool
**As an** LLM client operating stapler-squad via MCP, **I want** to list and filter backlog items by status/priority without a raw RPC call, **so that** I can triage or pick up work the way a human does in the web UI.

**Acceptance Criteria**:
- An LLM client can call `list_backlog_items` with a `status` filter and get back only items in that status.
  - *Given* three `session.BacklogItemData` records exist with IDs `bi-001`/`bi-002`/`bi-003` and statuses `"idea"`/`"in_progress"`/`"done"` respectively, *When* `list_backlog_items` is called with `status: ["in_progress"]`, *Then* the returned `ListBacklogItemsResult.Items` contains exactly one `BacklogItemBrief` with `ID == "bi-002"`, `TotalCount == 1`, `HasMore == false`.
- An unrecognized `status` value is rejected with a remediation hint rather than silently matching zero rows.
  - *Given* no special setup, *When* `list_backlog_items` is called with `status: ["not_a_real_status"]`, *Then* the tool returns `MCPResult.Success == false` with `Error.Code == ErrInvalidArgument` and a message listing the 9 valid status values — the underlying RPC is never called.
- The tool applies its own MCP-layer `limit` (the RPC has none) and respects the repo's "default 10" convention.
  - *Given* 25 backlog items exist in status `"ready"`, *When* `list_backlog_items` is called with `status: ["ready"]` and no `limit`, *Then* `ListBacklogItemsResult.Items` has length 10, `TotalCount == 25`, `HasMore == true`.
- The feature flag gate applies exactly like every other backlog tool.
  - *Given* `backlogHandlers.enabledCheck` returns `false`, *When* `list_backlog_items` is called, *Then* the result is `Error.Code == ErrFeatureDisabled`, matching the existing `TestBacklogHandlers_FeatureDisabled` table's shape for `get_backlog_item`.

**Files**: `server/mcp/tools_backlog.go`, `server/mcp/tools_backlog_test.go`, `server/mcp/feature_flag_test.go`

##### Task 1.1.1a: Add validation table + MCP-wire result types (~4 min)
- In `server/mcp/tools_backlog.go`, near the top (after the existing `allowedSelfResolveSourceStatuses` var, ~line 84), add:
  - `var validBacklogStatuses = []string{"idea", "refining", "ready", "queued", "in_progress", "review", "pr_pending", "done", "archived"}` (matches `session.BacklogStatus*` constants, `session/domain/backlog.go:16-24`).
  - A small helper `func isValidBacklogStatus(s string) bool` (linear scan over `validBacklogStatuses` — 9 entries, no map needed).
- Add `type BacklogItemBrief struct { ID, Title, Status, RepoPath, PrURL, Category string; Priority, PrNumber int; CreatedAt, UpdatedAt time.Time }` with JSON tags (`id`, `title`, `status`, `repo_path`, `pr_url`, `category,omitempty`, `priority`, `pr_number`, `created_at`, `updated_at`) and `type ListBacklogItemsResult struct { MCPResult; Items []BacklogItemBrief; TotalCount int; HasMore bool }` (JSON tags `items`, `total_count`, `has_more`) near the top of the file, alongside the other type declarations.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.1b: Implement `listBacklogItems` handler (~5 min)
- Add `func (h *backlogHandlers) listBacklogItems(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`, placed after `getBacklogItem` (~line 318).
- Body: `featureDisabledResult` guard first; if `h.backlogSvc == nil`, return `errResult(ErrInternalError, "backlog service not available for list_backlog_items", "")`. Parse `args["status"]`/`args["priority"]` (both `[]interface{}` → `[]string`/`[]int32`), validating each status via `isValidBacklogStatus` and returning `errResult(ErrInvalidArgument, ...)` listing `validBacklogStatuses` on the first bad value; parse `sort_by` (`stringArg`), `include_terminal`/`include_archived` (`boolArg`); parse `limit` (`args["limit"].(float64)`, default 10, clamp to max 50, mirroring `listSessions`'s `limitF`/clamp shape at `tools_discovery.go:93-99`).
- Call `resp, err := h.backlogSvc.ListBacklogItems(ctx, connect.NewRequest(&sessionv1.ListBacklogItemsRequest{Status: statuses, Priority: priorities, SortBy: sortBy, IncludeTerminal: includeTerminal, IncludeArchived: includeArchived}))`; on error, reuse `workflowServiceErrResult(err)`.
- Truncate `resp.Msg.Items` to `limit`, computing `totalCount := len(resp.Msg.Items)` *before* truncation and `hasMore := totalCount > limit`.
- Map each `*sessionv1.BacklogItem` to a `BacklogItemBrief`, sanitizing `Title` via `session.SanitizeForAgentContext(item.Title, 200)` (matches `get_backlog_item`'s own title budget, `tools_backlog.go:217`).
- Return `okResult(ListBacklogItemsResult{MCPResult: MCPResult{Success: true}, Items: briefs, TotalCount: totalCount, HasMore: hasMore})`.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.1c: Register `list_backlog_items` tool schema (~3 min)
- In `registerBacklogTools` (`server/mcp/tools_backlog.go:1716`), add one `s.AddTool(...)` call (10th in this file) before or after `get_backlog_item`'s registration:
  ```go
  s.AddTool(
      mcpgo.NewTool("list_backlog_items",
          mcpgo.WithDescription("List/filter backlog items by status, priority, sort order, and terminal/archived visibility. Default limit is 10 to avoid filling LLM context — use get_backlog_item for full detail on one item."),
          mcpgo.WithArray("status",
              mcpgo.Description("Filter to items with any of these statuses. Overrides include_terminal/include_archived when set."),
              mcpgo.Items(map[string]any{"type": "string", "enum": validBacklogStatuses}),
          ),
          mcpgo.WithArray("priority",
              mcpgo.Description("Filter to items with any of these priorities (1=highest, 5=lowest)"),
              mcpgo.Items(map[string]any{"type": "integer", "minimum": 1, "maximum": 5}),
          ),
          mcpgo.WithString("sort_by", mcpgo.Description("Sort order (default: priority)"), mcpgo.Enum("priority", "updated_at")),
          mcpgo.WithBoolean("include_terminal", mcpgo.Description("Include done items in the default result set (ignored if status is set)")),
          mcpgo.WithBoolean("include_archived", mcpgo.Description("Include archived items in the default result set (ignored if status is set)")),
          mcpgo.WithNumber("limit", mcpgo.Description("Max items to return (default 10, max 50) — applied client-side; the underlying RPC has no native limit"), mcpgo.DefaultNumber(10), mcpgo.Min(1), mcpgo.Max(50)),
      ),
      h.listBacklogItems,
  )
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.1d: Unit tests (~5 min)
- In `server/mcp/tools_backlog_test.go`, add `TestListBacklogItems_ReturnsOnlyMatchingStatus_When_StatusFilterSet`, `TestListBacklogItems_RejectsInvalidStatus_When_UnknownValueGiven`, `TestListBacklogItems_ClampsToDefaultLimit_When_MoreThanTenItemsMatch` (create 25 items via `newTestBacklogStorage`, assert `len(Items) == 10`, `HasMore == true`), each constructing `&backlogHandlers{storage: storage, backlogSvc: services.NewBacklogService(storage, nil, nil, nil, nil, nil)}` — this exact `NewBacklogService(storage, nil, nil, nil, nil, nil)` call (nil `SessionCreator`/`cfg`/`engine`/`pipelineEngine`/`pipelineModeRepo`) is the existing test-construction pattern already used 3x in this file (e.g. `tools_backlog_test.go:754`) for tools that only need read/list behavior, not session spawning.
- In `server/mcp/feature_flag_test.go`, add a `t.Run("list_backlog_items", ...)` case to `TestBacklogHandlers_FeatureDisabled` (`feature_flag_test.go:19`), following the exact shape of the existing `get_backlog_item` subtest.
- Files: `server/mcp/tools_backlog_test.go`, `server/mcp/feature_flag_test.go`

#### Story 1.1.2: Confirm the `tools_backlog.go:204` dangling reference is resolved (AC4)
**As a** maintainer, **I want** the `get_backlog_item` remediation hint that already names `list_backlog_items` to be accurate, **so that** an LLM following that hint finds a real tool.
**Acceptance Criteria**:
- The remediation string requires no code change once `list_backlog_items` exists under that exact name.
  - *Given* Story 1.1.1 has landed and `list_backlog_items` is registered in `registerBacklogTools`, *When* `grep -n "list_backlog_items" server/mcp/tools_backlog.go` is run, *Then* line 204's string `"Provide a valid UUID (e.g. from list_backlog_items or get_backlog_item)."` is unchanged and now references a real tool.
**Files**: `server/mcp/tools_backlog.go` (verification only — no edit expected)

##### Task 1.1.2a: Verify the dangling reference is resolved (~2 min)
- After Task 1.1.1c lands, run `grep -n "list_backlog_items" server/mcp/tools_backlog.go` and confirm line 204 still reads exactly `return errResult(ErrInvalidArgument, err.Error(), "Provide a valid UUID (e.g. from list_backlog_items or get_backlog_item)."), nil`. No edit needed — this task is a verification checkpoint, not a code change.
- Files: `server/mcp/tools_backlog.go`

---

### Epic 1.2: Notification History Tool
**Goal**: An LLM client can query notification history via MCP with the same filters `GetNotificationHistory` already supports (AC2).

#### Story 1.2.1: `get_notification_history` MCP tool
**As an** LLM client, **I want** to query notification history filtered by type/session/read-state, **so that** I can see what's happened across sessions without polling the web UI.

**Acceptance Criteria**:
- An LLM client can filter by `type_filter` and `unread_only` together.
  - *Given* the `NotificationHistoryStore` holds 2 unread `NOTIFICATION_TYPE_ERROR` records and 1 read `NOTIFICATION_TYPE_INFO` record for `session_id = "sess-42"`, *When* `get_notification_history` is called with `session_id: "sess-42"`, `unread_only: true`, `type_filter: "error"`, *Then* `GetNotificationHistoryResult.Notifications` has length 1 and its single `NotificationRecordBrief.Type == "error"`; `UnreadCount` reflects the store's own unread count (2), not the filtered count.
- An invalid `type_filter` string is rejected rather than silently resolving to `NOTIFICATION_TYPE_UNSPECIFIED`.
  - *Given* no special setup, *When* `get_notification_history` is called with `type_filter: "not_a_real_type"`, *Then* the tool returns `Error.Code == ErrInvalidArgument` and the RPC is never called.
- `limit` defaults to 10 (not the backend's native 50) and is clamped to 50 (not the backend's native 500 cap).
  - *Given* the store holds 60 matching notifications, *When* `get_notification_history` is called with no `limit`, *Then* `GetNotificationHistoryResult.Notifications` has length 10.
- Empty result sets return success, not an error.
  - *Given* no notifications exist for `session_id = "sess-does-not-exist"`, *When* `get_notification_history` is called with that `session_id`, *Then* `MCPResult.Success == true`, `Notifications` is an empty (non-nil) slice, `TotalCount == 0`.

**Files**: `server/mcp/tools_notifications.go` (new), `server/mcp/tools_notifications_test.go` (new), `server/mcp/server.go`

##### Task 1.2.1a: Create `tools_notifications.go` with struct + registration skeleton (~3 min)
- Create `server/mcp/tools_notifications.go`:
  ```go
  package mcp

  import (
      "context"
      "fmt"
      "strings"
      "time"

      "connectrpc.com/connect"
      mcpgo "github.com/mark3labs/mcp-go/mcp"
      mcpserver "github.com/mark3labs/mcp-go/server"
      sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
      "github.com/tstapler/stapler-squad/server/services"
      "github.com/tstapler/stapler-squad/session"
  )

  // notificationHandlers implements notification-history MCP tools.
  type notificationHandlers struct {
      svc *services.SessionService
  }

  // registerNotificationTools registers the notification-related MCP tools.
  func registerNotificationTools(s *mcpserver.MCPServer, h *notificationHandlers) {
  }
  ```
- Files: `server/mcp/tools_notifications.go`

##### Task 1.2.1b: Add enum map + MCP-wire result types (~4 min)
- In `tools_notifications.go`, add `var notificationTypeByName = map[string]sessionv1.NotificationType{"approval_needed": sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED, "input_required": ..., "confirmation_needed": ..., "task_complete": ..., "process_started": ..., "process_finished": ..., "error": ..., "warning": ..., "failure": ..., "info": ..., "debug": ..., "status_change": ..., "auto_approved": ..., "custom": sessionv1.NotificationType_NOTIFICATION_TYPE_CUSTOM}` (all 14 non-`UNSPECIFIED` values from `proto/session/v1/types.proto:780-805`).
- Add `type NotificationRecordBrief struct { ID, SessionID, SessionName, Type, Priority, Title, Message string; IsRead bool; CreatedAt time.Time; OccurrenceCount int32 }` (JSON tags snake_case) and `type GetNotificationHistoryResult struct { MCPResult; Notifications []NotificationRecordBrief; TotalCount, UnreadCount int; HasMore bool; Offset int }`.
- Files: `server/mcp/tools_notifications.go`

##### Task 1.2.1c: Implement `getNotificationHistory` handler (~5 min)
- Add `func (h *notificationHandlers) getNotificationHistory(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`.
- Parse `limit` (default 10, clamp max 50 — see pitfalls.md §1's recommendation to clamp below the backend's native 500 cap), `offset` (default 0), `type_filter` (string; if non-empty, look up in `notificationTypeByName`, return `ErrInvalidArgument` on miss listing valid names), `session_id` (`stringPtrArg`), `unread_only` (`boolPtrArg`).
- Build `sessionv1.GetNotificationHistoryRequest{Limit: &limit32, Offset: &offset32, TypeFilter: typeFilterPtr, SessionId: sessionIDPtr, UnreadOnly: unreadOnlyPtr}` (only set `Limit`/`Offset` pointers — required fields for this tool's own default/clamp behavior to take effect, unlike the other three optional fields which stay nil when unset).
- Call `resp, err := h.svc.GetNotificationHistory(ctx, connect.NewRequest(&req))`; on error, `workflowServiceErrResult(err)`.
- Map each `*sessionv1.NotificationHistoryRecord` to `NotificationRecordBrief`, sanitizing `Message` via `session.SanitizeForAgentContext(rec.Message, 500)`, and rendering `Type`/`Priority` via `strings.ToLower(strings.TrimPrefix(rec.NotificationType.String(), "NOTIFICATION_TYPE_"))` (and the priority equivalent) so the wire format matches the lowercase names the schema accepts.
- Return `okResult(GetNotificationHistoryResult{MCPResult: MCPResult{Success: true}, Notifications: briefs, TotalCount: int(resp.Msg.TotalCount), UnreadCount: int(resp.Msg.UnreadCount), HasMore: resp.Msg.HasMore, Offset: offset})`.
- Files: `server/mcp/tools_notifications.go`

##### Task 1.2.1d: Register `get_notification_history` tool schema (~3 min)
- In `registerNotificationTools`, add:
  ```go
  s.AddTool(
      mcpgo.NewTool("get_notification_history",
          mcpgo.WithDescription("Query notification history across sessions, filtered by type/session/read-state. Default limit is 10 to avoid filling LLM context."),
          mcpgo.WithNumber("limit", mcpgo.Description("Max notifications to return (default 10, max 50)"), mcpgo.DefaultNumber(10), mcpgo.Min(1), mcpgo.Max(50)),
          mcpgo.WithNumber("offset", mcpgo.Description("Number of notifications to skip for pagination (default 0)"), mcpgo.DefaultNumber(0), mcpgo.Min(0)),
          mcpgo.WithString("type_filter", mcpgo.Description("Filter to one notification type"), mcpgo.Enum("approval_needed", "input_required", "confirmation_needed", "task_complete", "process_started", "process_finished", "error", "warning", "failure", "info", "debug", "status_change", "auto_approved", "custom")),
          mcpgo.WithString("session_id", mcpgo.Description("Filter to notifications for one session")),
          mcpgo.WithBoolean("unread_only", mcpgo.Description("Only return unread notifications")),
      ),
      h.getNotificationHistory,
  )
  ```
- Files: `server/mcp/tools_notifications.go`

##### Task 1.2.1e: Wire `registerNotificationTools` into `NewCore` (~2 min)
- In `server/mcp/server.go`, inside the existing `if svc != nil { registerWorkflowTools(...); registerRulesTools(...) }` block (`server.go:59-62`), add `registerNotificationTools(s, &notificationHandlers{svc: svc})`.
- Files: `server/mcp/server.go`

##### Task 1.2.1f: Unit tests (~6 min)
- Create `server/mcp/tools_notifications_test.go` with `TestGetNotificationHistory_FiltersByTypeAndUnreadOnly_When_BothSet`, `TestGetNotificationHistory_RejectsInvalidTypeFilter_When_UnknownValueGiven`, `TestGetNotificationHistory_DefaultsLimitToTen_When_MoreMatch`, `TestGetNotificationHistory_ReturnsEmptySuccess_When_NoNotificationsMatch`.
- Construction recipe (verified against `server/notifications/store.go` and `server/services/session_service.go:1123-1125`): `store, _ := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))`, seed records directly via `store.Append(&notifications.NotificationRecord{ID: "n1", SessionID: "sess-42", NotificationType: int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), IsRead: false, CreatedAt: time.Now(), ...})` (no RPC round-trip needed to seed — `Append` is exported); then `storage := newTestBacklogStorage(t)` (the in-package `mcp` test helper already used by `tools_backlog_test.go` — `server/services`' own `createTestStorage(t)` is unexported to that other package and not reachable from `tools_notifications_test.go`), `bus := events.NewEventBus(32)`, `svc := services.NewSessionService(storage, bus)`, `svc.SetNotificationStore(store)`, `t.Cleanup(func() { svc.Shutdown() })`, and `h := &notificationHandlers{svc: svc}`.
- Files: `server/mcp/tools_notifications_test.go`

---

### Epic 1.3: Claude History Search Tool
**Goal**: An LLM client can run a full-text Claude/session-history search via MCP, wrapping `SearchClaudeHistory`, with results respecting an LLM-context-safe default limit (AC3).

#### Story 1.3.1: `search_claude_history` MCP tool
**As an** LLM client, **I want** to full-text search past Claude session history, **so that** I can find prior context (e.g. "when did I last debug this flaky test?") without manually paging through sessions.

**Acceptance Criteria**:
- A query returns matching sessions with snippets, defaulting to 10 results.
  - *Given* the FTS index (via `SearchEngine.IncrementalSync`) contains a session titled `"debug-flaky-test"` with a message containing the phrase `"goroutine leak"`, *When* `search_claude_history` is called with `query: "goroutine leak"` and no `limit`, *Then* `SearchClaudeHistoryResult.Results` has length ≤ 10, contains an entry with `SessionName == "debug-flaky-test"`, and that entry's `Snippets[0].Text` contains the substring `"goroutine leak"`.
- An empty query is rejected before any search work happens.
  - *Given* no special setup, *When* `search_claude_history` is called with `query: ""`, *Then* the tool returns `Error.Code == ErrInvalidArgument` without calling the underlying RPC.
- An unparseable `start_time`/`end_time` is rejected with a clear message.
  - *Given* no special setup, *When* `search_claude_history` is called with `start_time: "not-a-date"`, *Then* the tool returns `Error.Code == ErrInvalidArgument` naming the expected RFC3339 format.
- `limit` defaults to 10 (the repo convention), not the RPC's native default of 20, and is clamped to the RPC's own max of 100.
  - *Given* 150 messages match the query, *When* `search_claude_history` is called with no `limit`, *Then* `SearchClaudeHistoryResult.Results` has length 10; *When* called with `limit: 500`, *Then* the RPC receives `Limit: 100` (the proto-documented max), not 500.

**Files**: `server/mcp/tools_search.go` (new), `server/mcp/tools_search_test.go` (new), `server/mcp/server.go`

##### Task 1.3.1a: Create `tools_search.go` with struct + registration skeleton (~3 min)
- Create `server/mcp/tools_search.go`:
  ```go
  package mcp

  import (
      "context"
      "fmt"
      "time"

      "connectrpc.com/connect"
      mcpgo "github.com/mark3labs/mcp-go/mcp"
      mcpserver "github.com/mark3labs/mcp-go/server"
      sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
      "github.com/tstapler/stapler-squad/server/services"
      "github.com/tstapler/stapler-squad/session"
      "google.golang.org/protobuf/types/known/timestamppb"
  )

  // searchHandlers implements Claude/session-history full-text search MCP tools.
  type searchHandlers struct {
      svc *services.SessionService
  }

  // registerSearchTools registers the history-search MCP tools.
  func registerSearchTools(s *mcpserver.MCPServer, h *searchHandlers) {
  }
  ```
- Files: `server/mcp/tools_search.go`

##### Task 1.3.1b: Add MCP-wire result types (~3 min)
- In `tools_search.go`, add `type SearchSnippetBrief struct { Text, MessageRole string; MessageTime time.Time }`, `type SearchResultBrief struct { SessionID, SessionName, Project string; MessageIndex int32; Score float32; Snippets []SearchSnippetBrief }`, `type SearchClaudeHistoryResult struct { MCPResult; Results []SearchResultBrief; TotalMatches int; QueryTimeMs int64; HasMore bool; Offset int }`.
- Files: `server/mcp/tools_search.go`

##### Task 1.3.1c: Implement `searchClaudeHistory` handler (~5 min)
- Add `func (h *searchHandlers) searchClaudeHistory(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`.
- Parse `query` (`stringArg`; required — `ErrInvalidArgument` on empty), `project`/`model` (`stringPtrArg`), `limit` (default 10, clamp 1-100 per the proto doc's own max), `offset` (default 0). Parse `start_time`/`end_time` if present via `time.Parse(time.RFC3339, s)`; on error return `errResult(ErrInvalidArgument, fmt.Sprintf("start_time must be RFC3339 (e.g. 2026-08-01T00:00:00Z): %v", err), "")`; convert to `*timestamppb.Timestamp` via `timestamppb.New(t)` when present.
- Build `sessionv1.SearchClaudeHistoryRequest{Query: query, Project: project, Model: model, StartTime: startTS, EndTime: endTS, Limit: int32(limit), Offset: int32(offset)}`.
- Call `resp, err := h.svc.SearchClaudeHistory(ctx, connect.NewRequest(&req))`; on error, `workflowServiceErrResult(err)`.
- Map each `*sessionv1.SearchResult` to `SearchResultBrief`, sanitizing each snippet's `Text` via `session.SanitizeForAgentContext(snippet.Text, 500)` (the one unbounded field, per pitfalls.md §1/features.md §1).
- Return `okResult(SearchClaudeHistoryResult{MCPResult: MCPResult{Success: true}, Results: briefs, TotalMatches: int(resp.Msg.TotalMatches), QueryTimeMs: resp.Msg.QueryTimeMs, HasMore: resp.Msg.HasMore, Offset: offset})`.
- Files: `server/mcp/tools_search.go`

##### Task 1.3.1d: Register `search_claude_history` tool schema (~3 min)
- In `registerSearchTools`, add:
  ```go
  s.AddTool(
      mcpgo.NewTool("search_claude_history",
          mcpgo.WithDescription("Full-text search past Claude/session conversation history. Default limit is 10 to avoid filling LLM context (RPC max is 100)."),
          mcpgo.WithString("query", mcpgo.Description("Search query — supports natural language"), mcpgo.Required()),
          mcpgo.WithString("project", mcpgo.Description("Optional project path filter")),
          mcpgo.WithString("model", mcpgo.Description("Optional model filter (e.g. claude-sonnet-4)")),
          mcpgo.WithString("start_time", mcpgo.Description("Optional RFC3339 start of date range (e.g. 2026-08-01T00:00:00Z)")),
          mcpgo.WithString("end_time", mcpgo.Description("Optional RFC3339 end of date range")),
          mcpgo.WithNumber("limit", mcpgo.Description("Max results (default 10, max 100)"), mcpgo.DefaultNumber(10), mcpgo.Min(1), mcpgo.Max(100)),
          mcpgo.WithNumber("offset", mcpgo.Description("Number of results to skip for pagination (default 0)"), mcpgo.DefaultNumber(0), mcpgo.Min(0)),
      ),
      h.searchClaudeHistory,
  )
  ```
- Files: `server/mcp/tools_search.go`

##### Task 1.3.1e: Wire `registerSearchTools` into `NewCore` (~2 min)
- In `server/mcp/server.go`, in the same `if svc != nil { ... }` block as Task 1.2.1e, add `registerSearchTools(s, &searchHandlers{svc: svc})`.
- Files: `server/mcp/server.go`

##### Task 1.3.1f: Unit tests (~15 min — larger than most tasks here; see fixture note below)
- Create `server/mcp/tools_search_test.go` with `TestSearchClaudeHistory_RejectsEmptyQuery_When_QueryOmitted` and `TestSearchClaudeHistory_RejectsUnparseableStartTime_When_NotRFC3339` first — these need no FTS fixture (`h := &searchHandlers{svc: nil}` never reaches the RPC call for the first; a `nil` `svc` is fine since the empty-query/bad-timestamp checks happen before `h.svc.SearchClaudeHistory` is called).
- For `TestSearchClaudeHistory_ReturnsMatchingSnippet_When_QueryMatches` and `TestSearchClaudeHistory_DefaultsLimitToTen_When_LimitOmitted`, **no existing test file seeds real FTS content through the service layer** — confirmed by inspecting `server/services/search_pagination_test.go` (cursor-encoding unit tests only, no content fixture) and `session/search/engine_test.go` (seeds via the package-private `engine.IndexMessage(...)` helper, which is not usable here: `SearchService.SearchClaudeHistory` unconditionally calls `ss.searchEngine.IncrementalSync(hist)` on every call, and on a fresh engine `IncrementalSync` triggers `buildIndexLocked(history)` — a full rebuild from `hist` that would wipe any content seeded directly via `IndexMessage`, per `session/search/engine.go:399-416`). The real path requires an on-disk fixture: `indexSessionLocked` (`session/search/engine.go:537-541`) calls `history.GetMessagesFromConversationFile(entry.ID, 0)` — read that method (`session/history.go`) to confirm the exact expected file layout, write a temp `history.jsonl` (one `ClaudeHistoryEntry`-shaped line, `session/history.go:18-33`) plus the matching per-session conversation file it points to, use `t.Setenv("HOME", tempDir)` (mirrors `TestNewSessionService_TestMode_NeverTouchesRealSearchIndex`'s existing pattern, `server/services/session_service_search_index_test.go:34-36`) so `session.NewClaudeSessionHistoryFromClaudeDir()` resolves into the fixture, then wire `svc := services.NewSessionServiceWithSearchEngine(storage, bus, search.NewSearchEngine())` (`server/services/session_service_search_index_test.go:69`) and construct `&searchHandlers{svc: svc}`.
- If the fixture proves too heavy for this task's scope, an acceptable fallback (flag explicitly in the PR, don't silently skip) is testing `searchClaudeHistory`'s argument-parsing/mapping logic only (query/project/model/time/limit/offset → correct `sessionv1.SearchClaudeHistoryRequest` fields) against a `*services.SessionService` built the same way but with zero fixture content, asserting an empty-but-successful result — and covering the actual content-matching path at the `session/search` package level instead (already has engine-level coverage) rather than duplicating it end-to-end through MCP.
- Files: `server/mcp/tools_search_test.go`

---

### Epic 1.4: Registration-Count Safety Check
**Goal**: Confirm the additions in Epics 1.1–1.3 don't silently break `TestToolRegistrationCount` or leave `server_integration_test.go` out of sync.

#### Story 1.4.1: Full-suite verification
**As a** maintainer, **I want** confirmation that adding 3 tools across `tools_backlog.go` (existing file) and two new files doesn't require a manual count bump, **so that** CI doesn't fail on a stale hardcoded count.
**Acceptance Criteria**:
- `TestToolRegistrationCount` still passes unmodified.
  - *Given* `list_backlog_items` was added to `tools_backlog.go` (not one of the 5 files `TestToolRegistrationCount` scans: `server.go`, `tools_discovery.go`, `tools_lifecycle.go`, `tools_terminal.go`, `tools_vcs.go` — pitfalls.md §2) and `get_notification_history`/`search_claude_history` were added to two brand-new files also outside that list, *When* `go test ./server/mcp/... -run TestToolRegistrationCount` is run, *Then* it passes with the hardcoded count still `16` — no edit to `server_test.go` needed.
- The full `server/mcp` suite (including the new tests from 1.1.1d/1.2.1f/1.3.1f) passes.
  - *Given* all prior tasks are complete, *When* `make build && go test ./server/mcp/...` is run, *Then* all tests pass, including the pre-existing `TestBacklogHandlers_FeatureDisabled` table (now with its new `list_backlog_items` subtest).

**Files**: none (verification-only epic)

##### Task 1.4.1a: Run full verification (~3 min)
- Run `make build && go test ./server/mcp/... ./server/services/...` and `make lint`. Confirm `TestToolRegistrationCount` passes without modification (per the file-list rationale above) and no new lint findings in the two new files.
- Files: none (command execution only)
