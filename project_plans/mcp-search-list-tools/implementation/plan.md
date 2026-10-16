# Implementation Plan: mcp-search-list-tools

**Feature**: Expose `ListBacklogItems`, `GetNotificationHistory`, and `SearchClaudeHistory` as new MCP tools (`list_backlog_items`, `get_notification_history`, `search_claude_history`), following the existing `server/mcp/*.go` tool conventions with no new backend capability.
**Date**: 2026-08-13
**Status**: Ready for implementation
**ADRs**: ADR-001-search-claude-history-omits-unimplemented-filters, ADR-002-list-backlog-items-requires-backlog-service

---

## Step 0.5 — Creative Pass (Alternatives Considered)

1. **Resource-scoped tools** (one `mcpgo.NewTool` per RPC: `list_backlog_items`, `get_notification_history`, `search_claude_history`). *Strength*: each tool's schema only contains fields valid for that resource, matching all 55+ existing tools and letting an LLM caller pick correct arguments on the first try. *Weakness*: grows the `tools/list` payload by 3 entries in an already-large (55+) tool surface.
2. **One unified `search` tool with a `resource` enum dispatch parameter.** *Strength*: fewer tool names for the client to enumerate. *Weakness*: the four candidate resources (sessions, backlog, notifications, Claude history) have almost no overlapping filter shape, so the unified schema would need a near-union of fields marked "only valid when resource=X" in prose — MCP tool schemas have no conditional-field-visibility primitive, and the internal dispatch switch doesn't save any code, it just relocates it.
3. **Generic reflection/codegen bridge from ConnectRPC service methods to MCP tools.** *Strength*: would auto-generate tool wrappers for all matching RPCs, staying in sync as schemas evolve. *Weakness*: no such library exists in this module's dependency graph (`go.mod` has exactly one MCP dependency, `mark3labs/mcp-go` — the low-level SDK, not a generator), and it would bypass the repo's deliberately hand-tuned MCP UX (context-safe default limits, feature-flag gating, `SanitizeForAgentContext` truncation) that a reflection-based bridge has no way to infer.

**Chosen: Option 1 (resource-scoped tools).** Rejected alternatives are recorded in the Pattern Decisions table below.

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `list_backlog_items` | New MCP tool name; lists/filters backlog items by status, priority, sort order, with MCP-layer limit/offset since the backing RPC has no native pagination. | Resolves the dangling reference at `server/mcp/tools_backlog.go:204`. |
| `get_notification_history` | New MCP tool name; queries notification history with `type_filter`, `session_id`, `unread_only`, and pagination. | Wraps `GetNotificationHistory`. |
| `search_claude_history` | New MCP tool name; full-text searches Claude conversation history via the existing BM25 inverted index, with an LLM-context-safe default result limit and per-result snippet truncation. | Wraps `SearchClaudeHistory`. |
| `backlogHandlers` | Existing struct in `server/mcp/tools_backlog.go` holding `storage`, `backlogSvc *services.BacklogService`, `enabledCheck`; gains one new method for `list_backlog_items`. | No new struct needed — reuse. |
| `notificationHandlers` | New struct in `server/mcp/tools_notifications.go`, single field `svc *services.SessionService`. | Mirrors `workflowHandlers`'s shape. |
| `historyHandlers` | New struct in `server/mcp/tools_history.go`, single field `svc *services.SessionService`. | Mirrors `workflowHandlers`'s shape. |
| `listBacklogItems` | Handler method on `backlogHandlers` implementing the `list_backlog_items` tool. | Calls `h.backlogSvc.ListBacklogItems`. |
| `getNotificationHistory` | Handler method on `notificationHandlers` implementing `get_notification_history`. | Calls `h.svc.GetNotificationHistory`. |
| `searchClaudeHistory` | Handler method on `historyHandlers` implementing `search_claude_history`. | Calls `h.svc.SearchClaudeHistory`. |
| `registerBacklogTools` | Existing registration function in `tools_backlog.go`; gains one more `s.AddTool(...)` call for `list_backlog_items`. | Already gated by `storage != nil && (backlogEnabled == nil \|\| backlogEnabled())` in `server.go:63`. |
| `registerNotificationTools` | New registration function in `tools_notifications.go`, called from `NewCore`'s existing `if svc != nil { ... }` block. | Mirrors `registerWorkflowTools`. |
| `registerHistoryTools` | New registration function in `tools_history.go`, called from the same `if svc != nil { ... }` block. | Mirrors `registerWorkflowTools`. |
| `featureDisabledResult` | Existing helper (`tools_backlog.go`) checked first in every `backlogHandlers` method; returns an `ErrFeatureDisabled` result if the backlog flag is off. | Only `listBacklogItems` calls this — `get_notification_history`/`search_claude_history` have no corresponding flag. |
| `errResult` / `okResult` | Existing result constructors (`server/mcp/tools_backlog.go` top of file) — JSON-marshal into `mcpgo.NewToolResultText`. Universal across all 3 new tools. | Do not reinvent; reuse verbatim. |
| `ErrInvalidArgument` / `ErrInternalError` / `ErrItemNotFound` / `ErrFeatureDisabled` | Existing error code constants (`server/mcp/types.go:61-72`, `tools_backlog.go:60-64`) reused by the new tools. | No new error codes needed. |
| `ListBacklogItemsResult` | New MCP result struct (`server/mcp/types.go`): `MCPResult`, `Items []BacklogItemSummaryResult`, `TotalCount int`, `HasMore bool`. | JSON envelope, matches `ListSessionsResult` shape. |
| `GetNotificationHistoryResult` | New MCP result struct: `MCPResult`, `Notifications []NotificationRecordResult`, `TotalCount int`, `UnreadCount int`, `HasMore bool`. | Mirrors `GetNotificationHistoryResponse` field-for-field. |
| `SearchClaudeHistoryResult` | New MCP result struct: `MCPResult`, `Results []SearchResultSummary`, `TotalCount int`, `HasMore bool`, `QueryTimeMs int64`. | `SearchResultSummary.Snippets` is truncated per `truncateSearchSnippets`. Field renamed from `TotalMatches` to `TotalCount` (architecture-review concern) to match `ListBacklogItemsResult.TotalCount`/`GetNotificationHistoryResult.TotalCount` — same concept, one name across all 3 sibling tools. |
| `validateBacklogStatus` | New helper in `tools_backlog.go` validating each requested status string against `session.BacklogStatus*` constants before calling the RPC; returns `ErrInvalidArgument` on an unknown value. | Closes the "typo silently returns empty" trap (pitfalls.md §4). |
| `validateBacklogPriority` | New helper validating each requested priority int is in `[1,5]`. | Same trap class, lower severity (no string typo risk). |
| `parseNotificationTypeFilter` | New helper in `tools_notifications.go` mapping a friendly type-filter string (e.g. `"TASK_COMPLETE"`, with or without the `NOTIFICATION_TYPE_` prefix) to `sessionv1.NotificationType` via the generated `sessionv1.NotificationType_value` map; returns `ErrInvalidArgument` on an unrecognized string. | A proto-enum-backed sibling of `validateBacklogStatus` — same trap, different mechanism since this one *is* a real proto enum. |
| `truncateSearchSnippets` | New helper in `tools_history.go` capping snippets-per-result (3) and per-snippet text length via `session.SanitizeForAgentContext`. | Addresses features.md's "result shape is heavy" finding — caps tokens-per-result, not just result count. |
| `backlogSvc` | Existing `*services.BacklogService` field on `backlogHandlers`; `listBacklogItems` requires it non-nil (returns `ErrInternalError` with an "unavailable on this server configuration" message if nil — no cost-less degraded fallback). | See ADR-002. |
| `MCPResult` | Existing base struct (`Success bool`, `Error *MCPError`) embedded in every `*Result` type. | Unchanged; new result types embed it. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall handler structure (all 3 tools) | Transaction Script (PoEAA) — each handler is a linear parse-args → validate → call-service → map-response function | architecture.md §1, build-vs-buy.md §4 (`listWorkflows` template) | Domain Model | No business logic to encapsulate in the MCP layer — filtering/ranking/pagination logic already lives in `BacklogService`/`NotificationService`/`SearchService`; a Domain Model here would just duplicate existing service-layer state. |
| Data access, all 3 tools | Pattern A — call the existing ConnectRPC handler method in-process (`h.svc.Method(ctx, connect.NewRequest(...))`) | architecture.md §1-§2 (per-RPC reasoning) | Pattern B — call storage/session layer directly, reimplementing filter/format logic (the `get_backlog_item`/`list_sessions` precedent) | `ListBacklogItems`'s cost-lookup + proto conversion helpers are unexported in `server/services`; `SearchClaudeHistory` owns a singleflight-guarded cache + inverted-index sync an MCP handler has no business reimplementing; `GetNotificationHistory`'s nil-store degrade-gracefully guard is inherited for free via Pattern A. |
| Tool granularity | Resource-scoped tools (3 separate tools) | requirements.md, architecture.md §4 | Unified `search` tool with a `resource` dispatch enum | Near-zero filter-field overlap across the 3 target resources; a unified schema needs prose-only "only valid when resource=X" caveats MCP schemas can't enforce structurally; no dispatch logic is actually saved, just relocated. |
| Response shape, all 3 tools | JSON `okResult(...)` envelope (matches `list_sessions`/`search_sessions`) | features.md §1 ("fork in the road" flag) | Markdown-rendered text (`get_backlog_item`'s style, with `SanitizeForAgentContext`-truncated prose) | All 3 new tools are list/search tools returning tabular/structured data (arrays of items/records/results), not a single-item narrative detail view — markdown checklists don't fit a result set. |
| `list_backlog_items` pagination | MCP-layer `limit` (default 10, max 50) + `offset` int, computed by post-fetch slicing since the RPC has no native pagination field | pitfalls.md §1, features.md §2 | Opaque base64 cursor (the `list_sessions` `paginationCursor{LastTitle, CreatedAt}` pattern) | `ListBacklogItemsRequest` has no `offset` semantics to match at the wire level (unlike the other two RPCs), and backlog result sets are DB-capped at 1000 (`ent_repository_backlog.go`'s `defaultSafetyLimit`) — a cursor's ordering-stability benefit isn't worth its extra code for a Complexity-2 feature; a bare offset is adequate and matches the other two tools' shape more closely. |
| `get_notification_history` / `search_claude_history` pagination | Pass through the RPC's own native `offset` int argument | pitfalls.md §1, stack.md | Opaque cursor | Both RPCs already expose `offset` on the wire and the store/search-engine already degrade gracefully for an out-of-range offset — inventing a cursor here would duplicate semantics that already exist. |
| Filter-value validation (`status`, `priority`, `type_filter`) | Schema-level `mcpgo.Enum(...)`/`mcpgo.Min`/`mcpgo.Max` where the mcp-go v0.48.0 API supports it on array elements, with a handler-level fallback check regardless | pitfalls.md §4, features.md §1 (`list_sessions.status_filter` precedent) | Passthrough with no validation (RPC's own silent-empty-result behavior) | A typo'd status/type string today silently returns an empty result with no error signal — closing this is explicitly recommended by pitfalls.md and matches the existing `list_sessions.status_filter` precedent of making bad input unrepresentable/fail-fast. |
| `NotificationType` string→enum mapping | Smart-constructor helper `parseNotificationTypeFilter` built on the generated `sessionv1.NotificationType_value map[string]int32` | features.md §2, type-driven-design skill | Hand-rolled `switch` statement enumerating each `NotificationType` value | Avoids duplicating the 11+ enum values the proto compiler already generated; a lookup-table helper is the smallest correct implementation and self-updates if the proto enum grows. |
| `search_claude_history` filter surface | Omit `project`/`model`/`start_time`/`end_time` from the MCP tool schema entirely | features.md §2 ("critical gap" finding), ADR-001 | Expose all 4 fields as passthrough tool arguments (as requirements.md's "Open questions" section implied) | Confirmed by reading `SearchClaudeHistory`'s full handler body: these 4 fields are declared on the wire but never read — exposing them would create a tool that looks filtered but silently isn't, worse than not offering the filter at all. |
| MCP tool as protocol adapter | Implicit Adapter (GoF) — each handler function *is* the adapter between the MCP wire format and the ConnectRPC proto types; no explicit `Adapter` interface/type introduced | GoF, interface-pollution-checklist | A named `Adapter` interface with per-resource implementations | Exactly one implementation per resource exists and none is planned — an interface here would be speculative per `interface-pollution-checklist.md`'s smell #1. |
| File placement, `get_notification_history`/`search_claude_history` | One new file per resource: `tools_notifications.go`, `tools_history.go` | architecture.md §3 | Fold both into `tools_discovery.go` (session-resource file) | Violates the repo's established one-`tools_<resource>.go`-per-resource-area convention; `tools_discovery.go` is session-metadata-scoped today. |
| File placement, `list_backlog_items` | Reuse existing `tools_backlog.go` / `backlogHandlers` | architecture.md §3, build-vs-buy.md §4 | New `tools_backlog_list.go` file | Same resource area as `get_backlog_item` et al. — no fragmentation benefit, and it's the file whose dangling remediation string this tool resolves. |

---

## Migration Plan

N/A — no schema, proto, or data-model changes. All three request/response proto messages already exist (`proto/session/v1/backlog.proto:336-364`, `proto/session/v1/session.proto:972-987,1367-1381`) and are already used by the web UI; no `make proto-gen` step is part of this feature.

## Observability Plan
- **Logs**: each new handler logs at the error path only (validation failure, RPC error), using the existing `server/mcp` logging conventions (structured, includes tool name + failing argument) — matching how existing handlers in `tools_backlog.go`/`tools_workflow.go` log today. No new entry/exit logging is added beyond what the existing pattern already provides, since these are low-risk read-only wrappers.
- **Metrics**: no new metrics. These are thin, in-process, read-only wrappers around already-instrumented service methods (`SearchClaudeHistory` already carries its own OpenTelemetry spans per architecture.md §2) — a new MCP-layer metric would duplicate that instrumentation for no added signal.
- **Alerts**: no new alerts required.

## Risk Control
- **Feature flag**: `list_backlog_items` is gated by the existing `backlog` feature flag (`FeatureFlagService`, `server/services/feature_flag_service.go:50`) via the existing `featureDisabledResult(h.enabledCheck)` gate — default is whatever the `backlog` flag is already set to. `get_notification_history`/`search_claude_history` are **not gated** — no corresponding flag exists today, matching `search_sessions`/`list_sessions`'s ungated precedent (see Pattern Decisions).
- **`search_claude_history`'s per-call `IncrementalSync` cost (pre-mortem P1, accepted with mitigation)**: `SearchClaudeHistory` calls `ss.searchEngine.IncrementalSync(hist)` unconditionally on **every** invocation (`search_service.go:481`) — this is *not* gated by the history-cache TTL/singleflight guard (that guard only covers the disk scan that builds `hist`, not the sync itself, per adversarial-review.md's verification). `IncrementalSync` (`engine.go:399-496`) takes a full write lock for the duration of an O(session-count) diff walk on every call, even when nothing changed, contending with `Search()`'s read lock. This is pre-existing RPC behavior, not introduced by this plan, and likely cheap in absolute terms for a single-operator instance — but MCP exposes it to a caller pattern (autonomous agent retry loops) the RPC wasn't originally sized for. Accepted for this pass rather than adding new rate-limiting infrastructure, with two concrete mitigations: (1) Task 1.3.1c logs the call's duration so a cost spike is visible if this bound proves wrong in practice, and (2) if a future profiling pass shows this tool is measurably hot, wire the existing `writeLim`/`tokenBucket` pattern already in `server/mcp/rate_limiter.go` — no new limiter primitive needs inventing, only a call site.
- **`list_backlog_items` inherits the backend's fully unbounded fetch (accepted)**: `ListBacklogItemsResponse` has no server-side pagination — the RPC returns every matching row and the MCP handler paginates client-side via `limit`/`offset` after the fact. Pre-existing RPC behavior; accepted at current backlog table sizes.
- **Content-sensitivity exposure (accepted, deliberate)**: `search_claude_history`/`get_notification_history` are the first MCP tools to expose real historical content (Claude conversation snippets, notification message bodies) rather than just session metadata, and both register unconditionally with no access-control layer beyond "the service is running" — same posture as existing low-sensitivity tools like `list_workflows`. This is a deliberate, accepted decision for this personal-use tool, not an oversight; revisit if `--remote-access` mode's threat model changes.
- **Rollback procedure**: standard revert via PR close + revert commit. No data migration, no state to unwind — the tools are pure query wrappers.
- **Staged rollout**: full rollout on merge. New tools only become reachable when an MCP client calls `tools/list` and then invokes them by name; there is no existing caller to break.

## Unresolved Questions
- [x] Priority order among the three tools — **resolved by default**: no cost/complexity asymmetry was found (build-vs-buy.md, architecture.md) that forces an ordering, so this plan sequences all three within one implementation pass, with `list_backlog_items` (Epic 1.1) first since it resolves acceptance criterion 4's dangling reference at `server/mcp/tools_backlog.go:204`.
- [x] Does `mark3labs/mcp-go` v0.48.0's `mcpgo.WithArray(...)` support per-element `Enum()`/`Min()`/`Max()` constraints? — **resolved by adversarial-review.md**: verified directly from `mcp-go@v0.48.0/mcp/tools.go` (lines 1023-1081, 1290-1308) that `Enum`/`Min`/`Max` passed straight to `WithArray` would constrain the array *value* as a whole, not each element — per-element constraints require nesting inside `mcpgo.Items(map[string]any{...})` instead. Task 1.1.1d's schema already routes through `Items(...)` for `status`/`priority`, so no plan change is needed — this confirms the existing approach is correct.
- [x] Does `session/search/engine.go`'s `Search()` return an empty result (vs. panic/error) for an offset beyond the total match count? — **resolved by adversarial-review.md and pre-mortem.md**, independently confirmed twice: `Search()` (`engine.go:224-229`) explicitly handles `opts.Offset >= len(scoredDocs)` by setting `scoredDocs = nil` — empty result, no panic/error. Task 1.3.1c needs no defensive offset-clamping beyond what the engine already does.

---

## Dependency Visualization

```
Epic 1.1 (list_backlog_items)      Epic 1.2 (get_notification_history)   Epic 1.3 (search_claude_history)
  1.1.1a (validators)                1.2.1a (new file + struct)            1.3.1a (new file + struct)
    v                                   v                                     v
  1.1.1b (result type)                1.2.1b (result type)                 1.3.1b (result type + snippet helper)
    v                                   v                                     v
  1.1.1c (handler method)             1.2.1c (handler method)              1.3.1c (handler method)
    v                                   v                                     v
  1.1.1d (schema + AddTool)           1.2.1d (schema + register func)      1.3.1d (schema + register func)
    v                                   v                                     v
        \____________________ (independent — no cross-epic deps) ___________/
                                        v
                         1.2.1e / 1.3.1e (wire into server.go's `if svc != nil` block — same file, sequential)
    v                                   v                                     v
  1.1.2 (tests)                       1.2.2 (tests)                        1.3.2 (tests)
    \___________________________________|_____________________________________/
                                        v
                        Epic 1.4 (registration-count check, build, lint, registry no-op confirmation)
```

Epics 1.1–1.3 have no dependency on each other and can be implemented in any order or in parallel; 1.2.1e and 1.3.1e both touch `server/mcp/server.go` and must be applied sequentially (not concurrently) to avoid an edit conflict on the same `if svc != nil { ... }` block. Epic 1.4 depends on all three epics' implementation + test tasks being complete.

---

## Phase 1: MCP Tool Wrappers for Search/List/Filter RPCs

### Epic 1.1: `list_backlog_items` tool
**Goal**: Give MCP clients a way to list/filter backlog items by status and priority without a raw RPC call, resolving the dangling `list_backlog_items` reference in `get_backlog_item`'s remediation text.

#### Story 1.1.1: Implement and register `list_backlog_items`
**As an** LLM client driving stapler-squad via MCP, **I want** to list and filter backlog items by status/priority, **so that** I can find work items without a raw RPC/curl call.

**Acceptance Criteria**:
- An LLM client can filter backlog items by `status` and `priority` and receive only matching items, capped by a context-safe default limit.
  - *Given* a backlog with items in statuses `ready`, `in_progress`, and `done`, *When* `list_backlog_items` is called with `status: ["ready", "in_progress"]`, `limit: 10`, *Then* the response's `Items` contains only the `ready`/`in_progress` items, `done` items are excluded, and `Success: true`.
- An invalid `status` value fails fast instead of silently returning an empty result.
  - *Given* the known `BacklogStatus` values `idea, refining, ready, queued, in_progress, review, pr_pending, done, archived`, *When* `list_backlog_items` is called with `status: ["readdy"]` (typo), *Then* the tool returns `Success: false` with `Error.Code == ErrInvalidArgument` and a message listing the valid status set — not an empty `Items` array.
- The tool is gated by the same `backlog` feature flag as every other tool in `tools_backlog.go`.
  - *Given* `backlogEnabled()` returns `false`, *When* `list_backlog_items` is called, *Then* the response is `featureDisabledResult(h.enabledCheck)`'s output (`Error.Code == ErrFeatureDisabled`), and no call reaches `h.backlogSvc`.
- Calling the tool resolves acceptance criterion 4's dangling reference.
  - *Given* `server/mcp/tools_backlog.go:204`'s existing remediation string `"Provide a valid UUID (e.g. from list_backlog_items or get_backlog_item)."`, *When* an MCP client sends `tools/list`, *Then* `list_backlog_items` appears in the returned tool set.

**Files**: `server/mcp/tools_backlog.go`, `server/mcp/types.go`

##### Task 1.1.1a: Add `validateBacklogStatus`/`validateBacklogPriority` helpers (~4 min)
- In `server/mcp/tools_backlog.go`, add `validateBacklogStatus(statuses []string) error` checking each value against `session.BacklogStatus{Idea,Refining,Ready,Queued,InProgress,Review,PrPending,Done,Archived}` string constants (`session/domain/backlog.go:16-24`); return an error listing the valid set on the first bad value.
- Add `validateBacklogPriority(priorities []int) error` checking each value is in `[1,5]`.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.1b: Add `ListBacklogItemsResult` type (~2 min)
- In `server/mcp/types.go`, add `ListBacklogItemsResult struct { MCPResult; Items []BacklogItemSummaryResult; TotalCount int; HasMore bool }` and a small `BacklogItemSummaryResult` (ID, Title, Status, Priority, CreatedAt — trimmed fields, not the full proto).
- Files: `server/mcp/types.go`

##### Task 1.1.1c: Implement `listBacklogItems` handler method (~5 min)
- In `server/mcp/tools_backlog.go`, add `func (h *backlogHandlers) listBacklogItems(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`: call `featureDisabledResult(h.enabledCheck)` first; parse `status`/`priority`/`sort_by`/`include_terminal`/`include_archived`/`limit`/`offset` from `req.GetArguments()`; validate via the Task 1.1.1a helpers; if `h.backlogSvc == nil`, return `errResult(ErrInternalError, "backlog service unavailable on this server configuration", "")` per ADR-002; else call `h.backlogSvc.ListBacklogItems(ctx, connect.NewRequest(&sessionv1.ListBacklogItemsRequest{...}))`; **on error, return `errResult(ErrInternalError, fmt.Sprintf("failed to list backlog items: %v", err), "")`** (matches every existing handler's RPC-error convention in this file — don't skip this branch); on success, slice the response by `limit`/`offset`, compute `TotalCount`/`HasMore`, return `okResult(ListBacklogItemsResult{...})`.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.1d: Register `list_backlog_items` tool schema (~4 min)
- In `registerBacklogTools` (`tools_backlog.go:1716`), add `s.AddTool(mcpgo.NewTool("list_backlog_items", mcpgo.WithDescription(...), mcpgo.WithArray("status", mcpgo.Items(...)), mcpgo.WithArray("priority", ...), mcpgo.WithString("sort_by", ...), mcpgo.WithBoolean("include_terminal", ...), mcpgo.WithBoolean("include_archived", ...), mcpgo.WithNumber("limit", mcpgo.DefaultNumber(10), mcpgo.Min(1), mcpgo.Max(50)), mcpgo.WithNumber("offset", mcpgo.DefaultNumber(0), mcpgo.Min(0))), h.listBacklogItems)`. Resolve the Unresolved Question on per-element array constraints before finalizing the array schema; fall back to description-only + handler validation if unsupported.
- Files: `server/mcp/tools_backlog.go`

#### Story 1.1.2: Tests for `list_backlog_items`
**As a** maintainer, **I want** test coverage matching the existing `tools_backlog_test.go` conventions, **so that** regressions in filtering/validation/gating are caught in CI.

**Acceptance Criteria**:
- Every new behavior (filter, validation, feature-flag gate, nil-`backlogSvc` fallback, default limit) has a passing test.
  - *Given* `go test ./server/mcp/...` run after these tasks, *When* the suite executes, *Then* all new `TestListBacklogItems_*` tests pass and no existing test regresses.

**Files**: `server/mcp/tools_backlog_test.go`, `server/mcp/feature_flag_test.go`

##### Task 1.1.2a: `TestListBacklogItems_ReturnsFilteredItems` (~4 min)
- Happy-path test: seed a fake/mock `backlogSvc` (or in-memory storage per existing test helpers in `tools_backlog_test.go`) with items across 3+ statuses, call `listBacklogItems` with a `status` filter, assert only matching items returned.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.1.2b: `TestListBacklogItems_ValidatesStatusValues` (~3 min)
- Call with an invalid status string, assert `Success: false`, `Error.Code == ErrInvalidArgument`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.1.2c: `TestListBacklogItems_ValidatesPriorityRange` (~3 min)
- Call with `priority: [0]` and `priority: [99]`, assert `ErrInvalidArgument` for both.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.1.2d: `TestListBacklogItems_AppliesDefaultLimit` (~3 min)
- Seed more than 10 matching items, call with no `limit` arg, assert exactly 10 returned and `HasMore: true`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.1.2e: Add `list_backlog_items` to `TestBacklogHandlers_FeatureDisabled` table + `TestListBacklogItems_ReturnsUnavailable_When_BacklogSvcNil` (~4 min)
- Add a row to the existing feature-flag table test; add a separate test constructing `backlogHandlers{backlogSvc: nil}` and asserting `ErrInternalError`.
- Files: `server/mcp/feature_flag_test.go`, `server/mcp/tools_backlog_test.go`

##### Task 1.1.2f: `TestListBacklogItems_OffsetPagesPastFirstLimit` (~4 min)
- Adversarial-review gap fill: `list_backlog_items`'s offset/limit slicing is new code with no wire-level RPC test protecting it (unlike the other two tools, which pass `offset` straight through to already-tested RPCs). Seed 15+ items in one status, call with `limit: 10, offset: 0` then `limit: 10, offset: 10`; assert the second call's `Items` is the non-overlapping remainder (not a duplicate/empty page) and `HasMore: false` on the second call once all items are exhausted.
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 1.2: `get_notification_history` tool
**Goal**: Give MCP clients the same notification-history filtering the web UI already has (`type_filter`, `session_id`, `unread_only`, pagination).

#### Story 1.2.1: Implement and register `get_notification_history`
**As an** LLM client, **I want** to query notification history with type/session/read-status filters, **so that** I can check on approval requests, errors, or task completions without a raw RPC call.

**Acceptance Criteria**:
- An LLM client can combine `type_filter` and `unread_only` and get correctly AND'd results.
  - *Given* the notification store holds 5 unread `NOTIFICATION_TYPE_TASK_COMPLETE` records and 3 read `NOTIFICATION_TYPE_ERROR` records, *When* `get_notification_history` is called with `type_filter: "TASK_COMPLETE"`, `unread_only: true`, *Then* the response's `Notifications` contains exactly the 5 unread `TASK_COMPLETE` records, `UnreadCount: 5`.
- An invalid `type_filter` string fails fast rather than silently resolving to `NOTIFICATION_TYPE_UNSPECIFIED`.
  - *Given* the tool schema, *When* `get_notification_history` is called with `type_filter: "NOT_A_REAL_TYPE"`, *Then* the response is `Success: false`, `Error.Code == ErrInvalidArgument`.
- Results default to an LLM-context-safe limit lower than the RPC's own 500-record cap.
  - *Given* 100 matching notification records exist, *When* `get_notification_history` is called with no `limit` argument, *Then* at most 10 records are returned (the MCP tool's own default, distinct from the backend's default of 50) with `HasMore: true`.

**Files**: `server/mcp/tools_notifications.go` (new), `server/mcp/types.go`, `server/mcp/server.go`

##### Task 1.2.1a: Create `tools_notifications.go` with `notificationHandlers` struct (~3 min)
- New file `server/mcp/tools_notifications.go`: package `mcp`, imports, `type notificationHandlers struct { svc *services.SessionService }`.
- Files: `server/mcp/tools_notifications.go`

##### Task 1.2.1b: Add `parseNotificationTypeFilter` helper + `GetNotificationHistoryResult` type (~4 min)
- In `tools_notifications.go`: `func parseNotificationTypeFilter(s string) (sessionv1.NotificationType, error)` using `sessionv1.NotificationType_value`, accepting either the full `NOTIFICATION_TYPE_X` name or the bare `X` suffix.
- In `server/mcp/types.go`: `GetNotificationHistoryResult struct { MCPResult; Notifications []NotificationRecordResult; TotalCount int; UnreadCount int; HasMore bool }`.
- Files: `server/mcp/tools_notifications.go`, `server/mcp/types.go`

##### Task 1.2.1c: Implement `getNotificationHistory` handler method (~5 min)
- Parse `limit` (default 10, max 50 — MCP-layer cap below backend's 500), `offset`, `type_filter` (via Task 1.2.1b helper), `session_id`, `unread_only`; call `h.svc.GetNotificationHistory(ctx, connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{...}))`; **on error, return `errResult(ErrInternalError, fmt.Sprintf("failed to get notification history: %v", err), "")`**; on success, map response to `GetNotificationHistoryResult`; return `okResult(...)`.
- Files: `server/mcp/tools_notifications.go`

##### Task 1.2.1d: Add `registerNotificationTools` + tool schema (~4 min)
- `func registerNotificationTools(s *mcpserver.MCPServer, h *notificationHandlers)`: one `s.AddTool(mcpgo.NewTool("get_notification_history", ...), h.getNotificationHistory)` with `limit`/`offset`/`type_filter` (enum of the 11+ `NotificationType` names)/`session_id`/`unread_only` params.
- Files: `server/mcp/tools_notifications.go`

##### Task 1.2.1e: Wire `registerNotificationTools` into `server.go` (~3 min)
- In `NewCore` (`server/mcp/server.go`), inside the existing `if svc != nil { ... }` block (alongside `registerWorkflowTools`), add `registerNotificationTools(s, &notificationHandlers{svc: svc})`.
- Files: `server/mcp/server.go`

#### Story 1.2.2: Tests for `get_notification_history`
**As a** maintainer, **I want** test coverage for the new tool's filter combinations and validation, **so that** the enum-mapping and default-limit logic don't regress silently.

**Acceptance Criteria**:
- Filter combination, invalid-enum, and default-limit behaviors are all covered.
  - *Given* `go test ./server/mcp/...`, *When* run after these tasks, *Then* all `TestGetNotificationHistory_*` tests pass.

**Files**: `server/mcp/tools_notifications_test.go` (new)

##### Task 1.2.2a: `TestGetNotificationHistory_ReturnsFilteredRecords` (~4 min)
- Files: `server/mcp/tools_notifications_test.go`

##### Task 1.2.2b: `TestGetNotificationHistory_ValidatesTypeFilter` (~3 min)
- Files: `server/mcp/tools_notifications_test.go`

##### Task 1.2.2c: `TestGetNotificationHistory_CombinesUnreadOnlyAndTypeFilter` (~4 min)
- Files: `server/mcp/tools_notifications_test.go`

##### Task 1.2.2d: `TestGetNotificationHistory_AppliesContextSafeDefaultLimit` (~3 min)
- Files: `server/mcp/tools_notifications_test.go`

##### Task 1.2.2e: `TestGetNotificationHistory_ReturnsEmptySuccess_When_NotificationStoreNil` (~3 min)
- Adversarial-review gap fill: Pattern Decisions claims `GetNotificationHistory`'s nil-store degrade path (`notification_service.go:141-145`, returns an empty non-error response) is "inherited for free via Pattern A," but no task exercised it through the new MCP wrapper. Construct `svc` with no notification store set; call `get_notification_history`; assert `Success: true` (not `ErrInternalError`), `Notifications` empty.
- Files: `server/mcp/tools_notifications_test.go`

---

### Epic 1.3: `search_claude_history` tool
**Goal**: Give MCP clients full-text search over Claude conversation history, with an LLM-context-safe default limit and per-result snippet truncation.

#### Story 1.3.1: Implement and register `search_claude_history`
**As an** LLM client, **I want** to full-text search Claude conversation history, **so that** I can find past work without a raw RPC call, without the results overflowing my context.

**Acceptance Criteria**:
- Results default to a lower limit than the RPC's native default, matching the `list_sessions` convention.
  - *Given* Claude history contains 50 sessions mentioning "database migration", *When* `search_claude_history` is called with `query: "database migration"` and no `limit` argument, *Then* at most 10 results are returned (not the RPC's native default of 20), `TotalCount: 50`, `HasMore: true`.
- `query` is required.
  - *Given* no `query` argument, *When* `search_claude_history` is called, *Then* the response is `Success: false`, `Error.Code == ErrInvalidArgument`, message `"query is required"`.
- Per-result snippets are truncated so one heavy result can't dominate context.
  - *Given* a matching session with 8 snippet hits, *When* `search_claude_history` returns that result, *Then* at most 3 snippets are included for that result, each truncated via `session.SanitizeForAgentContext`.
- `project`/`model`/`start_time`/`end_time` are not exposed as tool arguments (per ADR-001).
  - *Given* the tool's `mcpgo.NewTool` schema, *When* inspected, *Then* it defines only `query`, `limit`, `offset` — no `project`/`model`/`start_time`/`end_time` parameters.

**Files**: `server/mcp/tools_history.go` (new), `server/mcp/types.go`, `server/mcp/server.go`

##### Task 1.3.1a: Create `tools_history.go` with `historyHandlers` struct (~3 min)
- New file `server/mcp/tools_history.go`: `type historyHandlers struct { svc *services.SessionService }`.
- Files: `server/mcp/tools_history.go`

##### Task 1.3.1b: Add `truncateSearchSnippets` helper + `SearchClaudeHistoryResult` type (~4 min)
- In `tools_history.go`: `func truncateSearchSnippets(snippets []*sessionv1.SearchSnippet, maxCount int, maxLen int) []SnippetResult` — caps count to 3, truncates each `Text` via `session.SanitizeForAgentContext`.
- In `server/mcp/types.go`: `SearchClaudeHistoryResult struct { MCPResult; Results []SearchResultSummary; TotalCount int; HasMore bool; QueryTimeMs int64 }`.
- Files: `server/mcp/tools_history.go`, `server/mcp/types.go`

##### Task 1.3.1c: Implement `searchClaudeHistory` handler method (~5 min)
- Parse `query` (required — `errResult(ErrInvalidArgument, "query is required", "")` if empty), `limit` (default 10, max 100), `offset`; resolve the offset-out-of-range Unresolved Question before finalizing offset handling; log the call's start time for an `IncrementalSync`-duration debug log line (Risk Control mitigation); call `h.svc.SearchClaudeHistory(ctx, connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{Query: ..., Limit: ..., Offset: ...}))`; **on error, return `errResult(ErrInternalError, fmt.Sprintf("failed to search claude history: %v", err), "")`**; on success, map each `SearchResult` through Task 1.3.1b's truncation helper; return `okResult(SearchClaudeHistoryResult{...})`.
- Files: `server/mcp/tools_history.go`

##### Task 1.3.1d: Add `registerHistoryTools` + tool schema (~4 min)
- `func registerHistoryTools(s *mcpserver.MCPServer, h *historyHandlers)`: one `s.AddTool(mcpgo.NewTool("search_claude_history", mcpgo.WithDescription("... Default limit is 10 to avoid filling LLM context ..."), mcpgo.WithString("query", mcpgo.Required()), mcpgo.WithNumber("limit", mcpgo.DefaultNumber(10), mcpgo.Min(1), mcpgo.Max(100)), mcpgo.WithNumber("offset", mcpgo.DefaultNumber(0), mcpgo.Min(0))), h.searchClaudeHistory)` — no `project`/`model`/`start_time`/`end_time` params per ADR-001.
- Files: `server/mcp/tools_history.go`

##### Task 1.3.1e: Wire `registerHistoryTools` into `server.go` (~3 min)
- In `NewCore`'s `if svc != nil { ... }` block, add `registerHistoryTools(s, &historyHandlers{svc: svc})` (sequenced after Task 1.2.1e's edit to the same block, not concurrent).
- Files: `server/mcp/server.go`

#### Story 1.3.2: Tests for `search_claude_history`
**As a** maintainer, **I want** test coverage for the required-query check, default limit, and snippet truncation, **so that** the context-safety guarantees don't silently regress.

**Acceptance Criteria**:
- Required-query, default-limit, and snippet-truncation behaviors are all covered.
  - *Given* `go test ./server/mcp/...`, *When* run after these tasks, *Then* all `TestSearchClaudeHistory_*` tests pass.

**Files**: `server/mcp/tools_history_test.go` (new)

##### Task 1.3.2a: `TestSearchClaudeHistory_ReturnsResults` (~4 min)
- Files: `server/mcp/tools_history_test.go`

##### Task 1.3.2b: `TestSearchClaudeHistory_RequiresQuery` (~3 min)
- Files: `server/mcp/tools_history_test.go`

##### Task 1.3.2c: `TestSearchClaudeHistory_AppliesDefaultLimitOf10` (~3 min)
- Files: `server/mcp/tools_history_test.go`

##### Task 1.3.2d: `TestSearchClaudeHistory_TruncatesSnippets` (~4 min)
- Files: `server/mcp/tools_history_test.go`

---

### Epic 1.4: Cross-cutting verification
**Goal**: Confirm the new tools don't silently break the hardcoded tool-count test, and that the build/lint/registry surface is clean.

#### Story 1.4.1: Registration-count, build, lint, and registry verification
**As a** maintainer, **I want** confirmation that adding 3 tools didn't silently break `TestToolRegistrationCount` or introduce a required-but-missing registry entry, **so that** CI stays green and the registry stays accurate.

**Acceptance Criteria**:
- `TestToolRegistrationCount` either still passes unmodified or is bumped with an explicit reason.
  - *Given* `server/mcp/server_test.go`'s `TestToolRegistrationCount` scans exactly `server.go`, `tools_discovery.go`, `tools_lifecycle.go`, `tools_terminal.go`, `tools_vcs.go` for tool registrations, *When* the new tools are registered in `tools_backlog.go`, `tools_notifications.go`, and `tools_history.go` (none of the 5 scanned files), *Then* `go test ./server/mcp/... -run TestToolRegistrationCount` passes with no code change to the hardcoded count.
- Full build/test/lint is green.
  - *Given* the 3 new tools and their tests, *When* `make build && make test && make lint` is run, *Then* all three succeed with no new failures.
- No new `docs/registry/features/*.json` entry is required.
  - *Given* `.claude/rules/feature-registry.md`'s scope (ConnectRPC handlers and React components, not MCP tool wrappers) and pitfalls.md §3's confirmation that zero `// +api:`/`// +feature:` markers exist in `server/mcp/*.go` today, *When* the 3 new tools are added, *Then* no new registry file is created for them (the underlying RPCs' existing markers, where present, are untouched).

**Files**: none changed by this story (verification only) — touches `server/mcp/server_test.go` only if the count assertion is contradicted by the above.

##### Task 1.4.1a: Run and confirm `TestToolRegistrationCount` (~2 min)
- `go test ./server/mcp/... -run TestToolRegistrationCount -v`; if it fails because a new tool landed in one of the 5 scanned files (shouldn't happen per this plan's file placement), bump the hardcoded count with a one-line comment citing the new tool names.
- Files: `server/mcp/server_test.go` (only if bump is needed)

##### Task 1.4.1b: `make build && make test` (~3 min)
- Confirms proto/build generation (no-op here, no proto changes) and full test suite, including the new `_test.go` files.
- Files: none

##### Task 1.4.1c: `make lint` (~2 min)
- Confirms no lint regressions in the 5 new/modified Go files.
- Files: none

##### Task 1.4.1d: Confirm no `docs/registry/` update needed (~2 min)
- Grep `server/mcp/tools_backlog.go`, `tools_notifications.go`, `tools_history.go` for `// +api:`/`// +feature:` markers (expect none, per pitfalls.md §3) and note this in the PR description rather than running `make registry-generate` with no expected diff.
- Files: none
