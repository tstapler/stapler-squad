# Features Research: MCP search/list/filter tool exposure

## 1. Existing tool conventions (55 `mcpgo.NewTool` call sites across `server/mcp/*.go`)

**Naming**: verb_noun, snake_case — `list_sessions`, `get_session`, `search_sessions`,
`create_workflow`, `list_workflows`, `get_backlog_item`, `list_approval_rules`,
`create_backlog_item`. No noun_verb examples found. New tools should follow this:
`list_backlog_items`, `search_backlog_items` (or similar), `get_notification_history`,
`search_claude_history`.

**Registration structure**: one `register<Domain>Tools(s *mcpserver.MCPServer, h
*<domain>Handlers)` function per file (`registerDiscoveryTools`, `registerBacklogTools`,
`registerWorkflowTools`, etc.), called conditionally from `NewCore`
(`server/mcp/server.go:34-70`) based on which dependencies are non-nil — e.g. backlog tools
only register `if storage != nil && (backlogEnabled == nil || backlogEnabled())`. The new
tools' registration function should gate the same way: notification/backlog tools need
`svc`/`backlogSvc` non-nil (already true for existing backlog/workflow tools registered
under the same `if svc != nil` / `if storage != nil` blocks).

**Pagination/limit convention** — `list_sessions` doc comment
(`server/mcp/server.go:121`): *"Default limit is 10 to avoid filling LLM context."* Pattern,
consistently applied:
- `mcpgo.WithNumber("limit", mcpgo.Description(...), mcpgo.DefaultNumber(10), mcpgo.Min(1),
  mcpgo.Max(N))` — `list_sessions` caps at 100, `search_sessions` caps at 50.
- Handler-side: `limitF, _ := args["limit"].(float64); limit := 10; if limitF > 0 { limit =
  int(limitF) }; if limit > N { limit = N }` — defends against a client omitting the field
  (Go zero-value `false`-assertion silently defaults) AND against a client passing an
  out-of-range value (server-side clamp, not just declarative `Max()` in the schema — the
  schema's Max/Min are documentation/UI hints for the client, not enforced server-side by
  mcp-go).
- `list_sessions` supports opaque base64-JSON cursor pagination (`encodeCursor`/
  `decodeCursor`, `server/mcp/tools_discovery.go:19-41`); `search_sessions` does not
  paginate — it just truncates to `limit` and returns `TotalCount` so the caller knows more
  exist. `SearchClaudeHistory`'s own proto (`limit`+`offset`, `has_more` bool) more closely
  matches `GetNotificationHistoryRequest`'s shape (`limit`, `offset`) than `list_sessions`'
  cursor scheme — offset-based pass-through is the natural mapping for both new search/list
  wraps, not cursor reinvention.

**Result formatting for LLM consumption**: two competing patterns found:
1. **Structured JSON envelope** (`list_sessions`, `search_sessions`, `get_session`) — a
   typed `...Result` struct in `server/mcp/types.go` embedding `MCPResult{Success bool,
   Error *MCPError}`, marshaled via `okResult(v any)` / `errResult(code, message,
   remediation string)` helpers (`server/mcp/tools_discovery.go:73-85`). Machine-readable,
   good for filter/list tools where the caller wants to iterate structured fields.
2. **Human-readable Markdown text** (`get_backlog_item`,
   `server/mcp/tools_backlog.go:216-244`) — builds a `strings.Builder` with `# Title`,
   `## Acceptance Criteria` checklist (`[✓]`/`[✗]`/`[ ]`), `## Description`, using
   `session.SanitizeForAgentContext(field, maxLen)` per field to strip HTML and truncate
   with a `" [truncated]"` suffix (`session/backlog_context.go:16-27`) — field-specific
   budgets, e.g. title capped at 200 chars, description at 2000, individual AC criteria at
   500. This is the LLM-context-budget-aware truncation convention closest to what
   `search_claude_history` needs (result snippets can be arbitrarily long free text).

For the new tools: `list_backlog_items` and `get_notification_history` should follow
pattern 1 (structured list of items, JSON), matching `list_sessions`/`search_sessions`.
`search_claude_history` should apply `SanitizeForAgentContext`-style per-field truncation to
each result's snippet text (pattern 2's budgeting idea) even inside a JSON envelope (pattern
1's shape) — snippets are the one field here with unbounded length.

## 2. Proto field enumeration

**`GetNotificationHistoryRequest`** (`proto/session/v1/session.proto:1367-1373`):
- `optional int32 limit = 1`
- `optional int32 offset = 2`
- `optional NotificationType type_filter = 3` — enum, 14 values incl.
  `NOTIFICATION_TYPE_UNSPECIFIED=0` through `NOTIFICATION_TYPE_CUSTOM=100`
  (`proto/session/v1/types.proto:780-805`): `APPROVAL_NEEDED`, `INPUT_REQUIRED`,
  `CONFIRMATION_NEEDED`, `TASK_COMPLETE`, `PROCESS_STARTED`, `PROCESS_FINISHED`, `ERROR`,
  `WARNING`, `FAILURE`, `INFO`, `DEBUG`, `STATUS_CHANGE`, `AUTO_APPROVED`, `CUSTOM`.
- `optional string session_id = 4`
- `optional bool unread_only = 5`

`GetNotificationHistoryResponse`: `notifications []NotificationHistoryRecord`,
`total_count int32`, `unread_count int32`, `has_more bool`.

Handler already exists and is directly callable: `SessionService.GetNotificationHistory`
(`server/services/session_service.go:3280`) — thin passthrough to
`s.notificationSvc.GetNotificationHistory`. `svc *services.SessionService` is already
threaded into `NewCore`/other MCP handlers (e.g. `workflowHandlers{svc: svc}`,
`server/mcp/tools_workflow.go:20`), so the new tool can call
`svc.GetNotificationHistory(ctx, connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{...}))`
directly — no new service wiring needed.

**`ListBacklogItemsRequest`** (`proto/session/v1/backlog.proto:336-353`):
- `repeated string status = 1` — 9 valid values per `session/domain/backlog.go:16-24`:
  `idea`, `refining`, `ready`, `queued`, `in_progress`, `review`, `pr_pending`, `done`,
  `archived`.
- `repeated int32 priority = 2` — no explicit min/max constant found in this pass; treat as
  an open int (existing UI likely constrains via a fixed set, but nothing in `session/`
  enforces bounds at this layer — worth confirming in planning phase whether an invalid
  priority silently returns zero results or errors).
- `string sort_by = 3`
- `bool include_terminal = 4` — includes `done` items in default (no explicit `status`)
  result set; ignored once `status` is set explicitly (explicit filter always wins — see
  doc comment).
- `bool include_archived = 5` — same "ignored once `status` set explicitly" rule, split
  from `include_terminal` so a client can default-show `done` but hide `archived`.

`ListBacklogItemsResponse`: `repeated BacklogItem items = 1` — no total_count/pagination at
all; this RPC returns everything matching the filter in one shot (unlike the other two,
which paginate). A `list_backlog_items` MCP tool therefore needs to invent its own
limit/truncation at the MCP layer (the RPC gives no cursor to defer to) — apply the
`list_sessions` "default limit 10" convention as an MCP-side post-filter truncation, not a
pass-through param, since the proto has no `limit` field to forward.

Handler: `BacklogService.ListBacklogItems` (`server/services/backlog_service_query.go:107`,
marked `// +api: backlog:list-items`). `backlogSvc *services.BacklogService` is already
threaded into `NewCore` for `create_backlog_item`/`import_github_issue`
(`server/mcp/server.go:64-65`, field `backlogSvc` on `backlogHandlers`), so the new tool can
reuse that same handler struct or add a sibling call.

**`SearchClaudeHistoryRequest`** (`proto/session/v1/session.proto:972-987`):
- `string query = 1` — required, natural language.
- `optional string project = 2`
- `optional string model = 3` (e.g. `claude-sonnet-4`)
- `optional google.protobuf.Timestamp start_time = 4`
- `optional google.protobuf.Timestamp end_time = 5`
- `int32 limit = 6` — proto doc comment: *"default: 20, max: 100"* (not enforced at the
  proto layer — that's a comment, not a validation rule; the MCP wrapper must clamp exactly
  like `list_sessions` does, and per the requirements doc should default to 10, not the
  RPC's native 20, to match the repo's stated LLM-context-safe convention).
- `int32 offset = 7`

`SearchClaudeHistoryResponse`: `results []SearchResult` (each with `session_id`,
`session_name`, `project`, `message_index`, `score float`, `snippets
[]SearchSnippet`, `metadata SearchResultMetadata`), `total_matches int32`, `query_time_ms
int64`, `has_more bool`. `SearchSnippet.text` is free-form — the one field that needs
explicit truncation (see §1).

Handler: `SessionService.SearchClaudeHistory` (`server/services/session_service.go:3039`,
`// +api: history:search`), thin passthrough to `s.searchSvc.SearchClaudeHistory` — backed
by the real inverted-index FTS engine in `session/search/` per the requirements doc. Same
`svc` dependency already available.

## 3. Edge cases the new MCP tools should handle

- **Empty result sets**: all three RPCs return a valid empty response (zero-length slice),
  not an error — the MCP wrapper should return `Success: true` with an empty array and
  `total_count: 0`/`total_matches: 0`, not synthesize a "not found" error. `get_backlog_item`
  is the only existing tool with a true not-found case (single-item lookup); list/search
  tools returning zero rows is a normal, successful outcome and must be distinguished from
  the not-found path (see `ErrItemNotFound` vs an empty `items: []`).
- **Invalid filter enum values from an LLM**: `type_filter` (NotificationType) and `status`
  (BacklogStatus, string-typed on the wire) are both susceptible to an LLM guessing a
  plausible-but-wrong string (e.g. `"unread"` instead of `unread_only: true`, or `"open"`
  instead of `"in_progress"`). `list_sessions`' `status_filter` param already demonstrates
  the mitigation: `mcpgo.Enum("running", "paused", ...)` declares the closed set in the tool
  schema so an MCP-aware client can validate/autocomplete before calling. The new
  `list_backlog_items` tool's `status` param and `get_notification_history`'s `type_filter`
  param should both declare `mcpgo.Enum(...)` with the full valid value lists found above.
  Whether an out-of-enum string sent anyway should hard-error
  (`ErrInvalidArgument`) or silently pass through to the RPC (which may just return zero
  matches, masking the mistake as "no results") is a planning-phase decision — precedent
  (`get_backlog_item`'s `validateUUID` at `tools_backlog.go:203-205`) favors explicit
  pre-validation with a remediation hint over silent pass-through.
- **Huge result sets needing truncation**: `SearchClaudeHistory` results carry
  free-text `snippets[].text` with no length cap in the proto — a query matching many
  messages could return large payloads even at `limit=10`. Apply
  `session.SanitizeForAgentContext`-style per-snippet truncation (see §1) independent of the
  row-count limit.
- **Pagination continuation across multiple tool calls**: `GetNotificationHistory` and
  `SearchClaudeHistory` both use `offset`+`has_more` (stateless, re-computable each call —
  simpler to wrap than `list_sessions`' opaque cursor, and should be exposed as a literal
  `offset` MCP parameter rather than inventing a cursor for these two). `ListBacklogItems`
  has no server-side pagination primitive at all (see §2) — if the item count can be large,
  the MCP tool needs to either accept that (return everything, capped by an MCP-only
  post-filter limit with no continuation) or add its own offset slicing purely at the MCP
  layer, documented as an MCP-only construct not backed by the RPC.
- **`session_id` filter referencing a nonexistent session**:
  `GetNotificationHistoryRequest.session_id` takes a raw string with no visible validation
  in the request message; unclear from the proto alone whether the backend errors or
  silently returns zero rows for an unknown session_id — worth a quick check of
  `NotificationService.GetNotificationHistory`'s implementation during planning, but the
  MCP-layer behavior should treat "zero rows" as success either way (not synthesize a
  session-not-found error the RPC itself doesn't raise), consistent with the empty-result
  guidance above.
- **`unread_only` combined with `type_filter`**: both are independent optional filters on
  the same request message (fields 3 and 5) — nothing in the proto suggests they're
  mutually exclusive, so the MCP tool should just pass both through when both are supplied
  and let the backend AND them together (standard multi-filter intersection), no special
  MCP-side handling needed.
- **`ListBacklogItems`'s `status` vs `include_terminal`/`include_archived` interaction**:
  the proto doc comments (`backlog.proto:340-352`) are explicit that an explicit `status`
  filter overrides both `include_terminal` and `include_archived` — the MCP tool schema
  should surface this in its param descriptions (e.g. "ignored if `status` is set") so an
  LLM caller doesn't set both expecting them to combine.

## 4. Unstated needs beyond the literal ACs

- **Enum discoverability**: an LLM operator has no way to learn the valid `NotificationType`
  or `BacklogStatus` values except by trial/error or reading source, unless the tool schema
  itself lists them via `mcpgo.Enum(...)` (as `list_sessions` already does for
  `status_filter`). This repo's own convention
  (`.claude/rules/interface-pollution-checklist.md`,
  `.claude/rules/primitive-obsession-checklist.md`) favors compiler/schema-level guardrails
  over runtime guessing — the `mcpgo.Enum()` declarative list *is* that guardrail for MCP
  tool params (it's inspectable by the calling LLM before invocation, unlike a Go-side error
  message which only surfaces after a failed call). A dedicated "list valid filter values"
  discovery tool is not needed — `mcpgo.Enum()` on each param, populated from the proto enum
  / domain constants enumerated above, is sufficient and matches precedent. Error-message-
  driven discovery (per `get_backlog_item`'s `validateUUID` remediation-hint pattern) is the
  right fallback for cases with no closed enum (e.g. free-text `project`/`model` filters on
  `SearchClaudeHistory` — no fixed set to declare).
- **Consistent result-shape convention across the 3 new tools**: `list_sessions` /
  `search_sessions` return `total_count`; `SearchClaudeHistory`'s proto uses `total_matches`
  — the MCP JSON envelope should probably normalize the field name across all list/search
  tools (e.g. always `total_count` in the MCP-facing JSON regardless of the underlying
  proto's field name) so an LLM caller doesn't have to remember two different key names for
  the same concept across tools it uses interchangeably. Similarly `has_more` (proto,
  bool) vs `next_cursor` (`list_sessions`' MCP shape, opaque string-or-nil) are two
  different "is there more" signals already coexisting in this codebase — picking one
  MCP-facing convention (recommend `has_more: bool` + `offset`, since that's what 2 of the 3
  new tools' underlying protos already natively expose, and matches §3's pagination
  guidance) avoids adding a third pattern.

## 5. Dangling reference at `server/mcp/tools_backlog.go:204`

Exact code (`get_backlog_item` handler, `server/mcp/tools_backlog.go:203-205`):
```go
if err := validateUUID(itemID); err != nil {
    return errResult(ErrInvalidArgument, err.Error(), "Provide a valid UUID (e.g. from list_backlog_items or get_backlog_item)."), nil
}
```
This is the `remediation` string on an `INVALID_ARGUMENT` error result, returned when a
caller passes a non-UUID `item_id` to `get_backlog_item`. It tells the LLM caller to go run
`list_backlog_items` to find a valid ID — but that tool doesn't exist yet anywhere in the 55
registered tools (confirmed via the full `mcpgo.NewTool` grep in §1: no `list_backlog_items`
entry). Once this backlog item's `list_backlog_items` tool is added (per AC1), this
remediation string becomes accurate and needs no further change — it was written
prospectively, naming the tool this very backlog item is meant to create. If the planning
phase decides on a different tool name (e.g. `search_backlog_items` instead), this string
must be updated to match at implementation time; otherwise no action needed beyond adding
the tool.
