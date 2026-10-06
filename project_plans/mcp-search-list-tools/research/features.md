# Research: Features & Edge Cases — MCP search/list tools

Agent 2 (Features) research for the `mcp-search-list-tools` SDD cycle. Grounded in code read
directly (paths/lines below), not the MCP ecosystem in the abstract — this repo already has a
strong internal convention to imitate, so external MCP examples are secondary.

## 1. Existing precedent in this codebase

### `search_sessions` / `list_sessions` / `get_session` — `server/mcp/tools_discovery.go`

These three are the direct template named in the requirements doc. Registration:
`server/mcp/server.go:118-169` (`registerDiscoveryTools`).

- **`list_sessions`** (`server.go:120`, handler `tools_discovery.go:87`): default `limit=10`,
  clamps to `max=100`, opaque base64 JSON pagination cursor (`encodeCursor`/`decodeCursor`,
  `tools_discovery.go:19-41`) built from `{last_title, created_at}` of the last item in the page.
  `status_filter` is a single string constrained via `mcpgo.Enum(...)` at the tool-schema level
  (`server.go:124`) — invalid values are rejected by the MCP framework before the handler runs,
  not hand-validated in Go.
- **`search_sessions`** (`server.go:151`, handler `tools_discovery.go:183`): default `limit=10`,
  clamps to `max=50` (lower cap than list — search results are considered "found what you want,"
  list is "browsing"). No cursor — just `TotalCount` in the response so the caller knows if it was
  truncated. Query is required (`errResult(ErrInvalidArgument, "query is required", "")` if
  missing/empty) — this is the standard "required string arg" shape reused across the codebase.
  `tag_filter` is parsed defensively from `interface{}` → `[]interface{}` → `[]string`, skipping
  any non-string elements rather than erroring (`tools_discovery.go:199-208`).
- **`get_session`** (`server.go:140`, handler `tools_discovery.go:157`): not-found path returns a
  distinct error code (`ErrSessionNotFound`) with a **remediation hint** pointing back at the list/
  search tools (`"Use list_sessions or search_sessions to find valid session IDs."`,
  `tools_discovery.go:179-180`) — this "point the LLM at the right tool" remediation string is a
  recurring pattern worth replicating for all three new tools.

### `get_backlog_item` — `server/mcp/tools_backlog.go:204` (handler starts ~193)

The UUID-taking single-item lookup pattern, useful for `list_backlog_items`'s per-item ID shape
and for validating IDs that flow into other backlog tools:

- `validateUUID` (`tools_backlog.go:52-57`, a permissive regex `^[0-9a-f-]{36}$` — format check
  only, not RFC4122 strict) runs *before* the storage call, giving `ErrInvalidArgument` with a
  remediation string that references the not-yet-existing `list_backlog_items` tool
  (`tools_backlog.go:204`: `"Provide a valid UUID (e.g. from list_backlog_items or
  get_backlog_item)."`) — **this is the dangling reference acceptance criterion #4 in
  requirements.md is about**; confirmed still dangling as of this read.
- `featureDisabledResult(h.enabledCheck)` (`tools_backlog.go:73-79` in server.go's neighbor file,
  defined once, called first line of every backlog handler) is the universal feature-flag gate —
  every new backlog-adjacent tool must call this first, matching acceptance criterion #5.
- Not-found uses `errors.Is(err, session.ErrNotFound)` → `ErrItemNotFound`, distinct from a
  generic `ErrInternalError` fallback for anything else (`tools_backlog.go:212-217`).
- Response bodies are **rendered as human-readable markdown text**, not raw JSON dumps
  (`# {title}`, `## Acceptance Criteria` checklist with `[✓]/[✗]/[ ]` markers, `## Description`,
  `## Latest Review Verdict` — `tools_backlog.go:220-260+`), and every free-text field is passed
  through `session.SanitizeForAgentContext(text, maxLen)` with an explicit per-field cap (200 for
  title, 500 for criteria, 2000 for description, 500 for reviewer summary). **This is a second,
  competing response-shape convention** to `okResult(...)` + a typed JSON struct (used by
  `list_sessions`/`search_sessions`/`get_session`) — the plan phase needs to pick one per new tool
  rather than assume JSON-only. Given `search_claude_history` results are inherently long-form
  (snippets, scores), and `list_backlog_items`/`get_notification_history` are structured/tabular,
  a JSON `okResult` shape (matching the session discovery tools, since these are also list/search
  tools) is the closer precedent — but flag this as a real fork in the road for `/sdd:3-plan`.

### Cross-cutting helpers (`server/mcp/tools_backlog.go` top of file + `tools_discovery.go:73-85`)

- `errResult(code, message, remediation string)` / `okResult(v any)` — universal result
  constructors, both JSON-marshal into `mcpgo.NewToolResultText`. All 3 new tools should use these
  verbatim.
- Error code constants live in `server/mcp/types.go:61-72` (`ErrSessionNotFound`,
  `ErrInvalidArgument`, `ErrInternalError`, `ErrRateLimitExceeded`, etc.) plus
  `ErrItemNotFound`/`ErrFeatureDisabled`/`ErrPermissionDenied` in `tools_backlog.go:60-64`. No
  existing code for "notification not found" or "search index unavailable" — new codes will be
  needed (e.g. reuse `ErrInternalError` for index-sync failures, since there's no per-resource
  "index broken" precedent to imitate).

## 2. Per-RPC edge cases (grounded in the actual request/response/service code)

### `ListBacklogItems` — `proto/session/v1/backlog.proto:336-364`, service
`server/services/backlog_service_query.go:107-142`

```protobuf
message ListBacklogItemsRequest {
  repeated string status = 1;
  repeated int32 priority = 2;
  string sort_by = 3;
  bool include_terminal = 4;
  bool include_archived = 5;
}
message ListBacklogItemsResponse {
  repeated BacklogItem items = 1;
}
```

- **No pagination at all** — no `limit`/`offset` field on request or response, unlike the other
  two RPCs. The service (`backlog_service_query.go:107-142`) fetches and returns **every matching
  item**, unbounded. This is the single biggest edge case for the new tool: without a
  client-side (MCP-layer) `limit`, a broad/no-filter call could return the entire backlog into the
  LLM's context — the "LLM-context-safe default limit" requirement (acceptance criterion #3) is
  written against `search_claude_history` specifically, but `list_backlog_items` needs the same
  treatment even though it's not called out — **the MCP handler must add its own limit/truncation
  the RPC itself doesn't provide**, following `list_sessions`'s post-fetch slicing pattern
  (`tools_discovery.go:135-142`) rather than assuming the RPC does it.
- `status` values are raw strings with **no server-side validation against the
  `BacklogStatus` enum** (`session/domain/backlog.go:16-24`: `idea`, `refining`, `ready`, `queued`,
  `in_progress`, `review`, `pr_pending`, `done`, `archived`) — an invalid status string (typo,
  wrong case) silently filters to an empty/wrong result set rather than erroring, since the filter
  is a plain string match downstream. The MCP tool should either (a) validate against this enum
  itself before calling (mirroring `list_sessions`'s `mcpgo.Enum(...)` tool-schema-level
  constraint at `server.go:124`, which rejects bad values before the handler runs), or (b)
  document the silent-empty-result behavior — (a) is strongly preferred since it fails fast and is
  free (the enum is small and stable).
- `priority` is `repeated int32` with **no declared bounds in the proto**, but the domain range is
  1–5 (`DefaultBacklogPriority = 3`, `session/domain/backlog.go:29`; `result.Priority >= 1 &&
  result.Priority <= 5` validation elsewhere at `backlog_service_triage.go:3199`). Passing 0 or 99
  is not rejected by `ListBacklogItems` itself — it will just match nothing. Same
  validate-fast-at-the-tool-schema argument applies (`mcpgo.Min(1)`/`mcpgo.Max(5)` on the number
  array elements, if the mcp-go library's `WithArray` supports per-element bounds — verify during
  planning; if not, validate in the handler before calling the RPC).
- `include_terminal`/`include_archived` interact subtly with explicit `status`: per the proto
  comment (`backlog.proto:355-361`) and the service code (`backlog_service_query.go:118-124`), an
  **explicit `status` filter always overrides both flags** — passing `status: ["done"]` shows done
  items regardless of `include_terminal`. Worth a one-line note in the tool description so an LLM
  operator doesn't think it needs to set both.
- **Empty result set**: not an error path anywhere in the chain — `ListBacklogItemSummaries`
  returns an empty slice, proto response has `items: []`. The MCP tool's `okResult` should return
  `Success: true` with an empty array, not an error — consistent with how `search_sessions` treats
  zero matches (no special-casing in `tools_discovery.go:233-237`).
- `sort_by` is an unvalidated free string threaded straight to `ListBacklogItemSummaries` — worth
  checking during planning what invalid values do downstream (likely: no-op/default sort) so the
  tool description can enumerate the actually-supported values rather than leaving it a blind
  passthrough.

### `GetNotificationHistory` — `proto/session/v1/session.proto:1367-1381`, service
`server/services/notification_service.go:137-191`, store `server/notifications/store.go:68-74,236-277`

```protobuf
message GetNotificationHistoryRequest {
  optional int32 limit = 1;
  optional int32 offset = 2;
  optional NotificationType type_filter = 3;
  optional string session_id = 4;
  optional bool unread_only = 5;
}
message GetNotificationHistoryResponse {
  repeated NotificationHistoryRecord notifications = 1;
  int32 total_count = 2;
  int32 unread_count = 3;
  bool has_more = 4;
}
```

- **Best-behaved of the three RPCs for edge cases** — the store's `List()`
  (`server/notifications/store.go:236-277`) already handles everything defensively:
  `limit<=0` → defaults to 50, `limit > MaxNotifications(500)` → clamps to 500,
  `offset<0` → clamps to 0, **`offset > len(filtered)` → returns `[]` (not an error)**, and
  `has_more` is computed correctly server-side. The MCP tool mostly needs to pass through and set
  its own (likely lower, e.g. 10-20) LLM-context-safe default, since 50 raw notification
  records — each with `title`+`message`+`metadata` map — is plausibly too much for a default call.
- `type_filter` is an `optional NotificationType` enum (`proto/session/v1/types.proto:778-800+`:
  `NOTIFICATION_TYPE_APPROVAL_NEEDED`, `..._TASK_COMPLETE`, `..._ERROR`, `..._INFO`, etc. — 11+
  values plus `UNSPECIFIED=0`). No existing MCP tool converts an enum by name today (only one
  int32 literal use exists, `tools_backlog.go:1698`). The generated Go package exposes
  `sessionv1.NotificationType_value map[string]int32`
  (`gen/proto/go/session/v1/types.pb.go:848`) — the MCP handler should accept the enum's string
  name (e.g. `"NOTIFICATION_TYPE_TASK_COMPLETE"` or a shortened `"TASK_COMPLETE"` with a
  `"NOTIFICATION_TYPE_"` prefix added server-side, mirroring `status_filter`'s friendlier string
  style) and look it up via that generated map rather than hand-rolling a switch. An unrecognized
  string should be `ErrInvalidArgument`, not silently ignored (unlike backlog's `status`, which
  has no such validation today — this is a case where the new tool can do better than its proto
  sibling).
- **`unread_only` combined with `type_filter`**: the store applies both as independent AND'd
  filters (`store.go:243-250`) — no special-case bug found, safe to combine. Worth stating
  explicitly in the tool description since acceptance criterion #2 calls this combination out.
- `session_id` filters to one session but is **not validated** to be a real/live session id — an
  unknown session_id just yields zero matches (`store.go:245-246`, plain string equality). No
  error path exists to surface "that session doesn't exist" — same "document the silent-empty"
  choice as `ListBacklogItems`'s status filter, or the MCP tool could optionally cross-check
  against `store.LoadInstances()` the way `get_session` does, at the cost of an extra call.
- **No date-range filter exists on this RPC at all** (unlike `SearchClaudeHistory`) — don't invent
  one in the tool schema; only `type_filter`/`session_id`/`unread_only`/`limit`/`offset` are real.

### `SearchClaudeHistory` — `proto/session/v1/session.proto:972-987`, service
`server/services/search_service.go:459-591`, index `session/search/engine.go`

```protobuf
message SearchClaudeHistoryRequest {
  string query = 1;
  optional string project = 2;
  optional string model = 3;
  optional google.protobuf.Timestamp start_time = 4;
  optional google.protobuf.Timestamp end_time = 5;
  int32 limit = 6;
  int32 offset = 7;
}
message SearchClaudeHistoryResponse {
  repeated SearchResult results = 1;
  int32 total_matches = 2;
  int64 query_time_ms = 3;
  bool has_more = 4;
}
```

- **Critical gap, confirmed by reading the full handler body
  (`server/services/search_service.go:459-591`): `project`, `model`, `start_time`, and `end_time`
  are declared on the wire but never read anywhere in `SearchClaudeHistory`.** Only `Query`,
  `Limit`, and `Offset` are used. `session/search/engine.go`'s `SearchOptions` struct
  (lines 22-29) only has `Limit`/`Offset`/`SessionID` fields — no project/model/date filtering
  exists in the search engine at all. (A **different** RPC, `GetClaudeHistoryMessages`, does use
  `req.Msg.Project` via `hist.GetByProject` at `search_service.go:256-257` — that's a distinct
  code path, not `SearchClaudeHistory`.) **This means if the new MCP tool schema exposes
  `project`/`model`/`start_time`/`end_time` as filters (as the requirements doc's "Open questions"
  section implies it should, since it lists them as existing filters), they will silently do
  nothing** — a filtered-looking call actually returns unfiltered results. Two honest options for
  planning: (1) omit those fields from the MCP tool schema until the backend supports them
  (safest, avoids a misleading tool surface — closest to "wrap existing RPC" scope), or (2)
  implement client-side post-filtering in the MCP handler using data already available per-result
  (`entry.Model`/`entry.Project`/`entry.CreatedAt` are already fetched via `hist.GetByID` at
  `search_service.go:566-570` for the response's `Metadata`/`Project` fields — filtering on them
  post-search is mechanical, though it means over-fetching from the engine and trimming, which
  changes `total_matches`/`has_more` semantics subtly). This is arguably outside "no new backend
  capability" if done in the RPC itself, but doing it in the *MCP handler* (post-processing an
  already-returned page) stays within scope. **Flag explicitly for `/sdd:3-plan` — don't let this
  surface silently as a "works in the demo, filters do nothing in practice" bug.**
- `limit`/`offset` defaulting mirrors notifications: `limit<=0` → 20, `limit>100` → clamp to 100,
  `offset<0` → clamp to 0 (`search_service.go:497-506`) — no explicit "offset beyond count" guard
  visible in this function, but `searchEngine.Search` presumably returns an empty slice for an
  out-of-range offset the same way pagination normally degrades (not independently confirmed by
  reading the engine's `Search()` body — worth a quick check during implementation, not just
  planning, since a wrong assumption here would surface as an off-by-one/panic rather than a
  silent empty page).
- **Every call triggers `IncrementalSync` before searching**
  (`search_service.go:472-490`, `getOrRefreshHistoryCache` + `searchEngine.IncrementalSync(hist)`)
  — i.e., the "stale/deleted session results" concern in the research question is actively
  mitigated by design: each search re-syncs the index against the current on-disk Claude history
  before querying, and a full rebuild happens automatically when needed
  (`syncResult.WasFullRebuild`). The **failure mode to worry about instead is latency**, not
  staleness — a full rebuild on a cold cache could make the first `search_claude_history` call
  from a fresh MCP server process noticeably slower than subsequent ones. Worth surfacing in the
  tool description or as a known limitation, not silently eating a timeout.
- `query` is the one truly required field — empty string is already rejected server-side
  (`connect.CodeInvalidArgument`, `search_service.go:469-471`), so the MCP tool's own
  `errResult(ErrInvalidArgument, "query is required", "")` pre-check (matching
  `search_sessions`'s pattern) is a convenience/fast-fail duplicate of a check that already exists
  server-side, not a strictly new requirement — fine to keep for consistency with the other tools
  and to avoid a round-trip.
- **Result shape is heavy relative to the other two RPCs**: each `SearchResult` carries
  `session_id`, `session_name`, `project`, `message_index`, `score`, a `repeated SearchSnippet`
  (each with `text`, `highlight_ranges`, `message_role`, `message_time`), and a
  `SearchResultMetadata`. This is the RPC acceptance criterion #3 explicitly calls out for
  "LLM-context-safe default limit" — the *default* limit matters less here than **snippet
  truncation/count-per-result**, since one result with many long snippets can dominate context
  more than several results with one snippet each. The proto doesn't cap snippet count or length
  itself (that's generated by `ss.snippetGenerator.GenerateFromSearchResult`, not inspected in
  this pass) — worth a targeted look during planning at whether the snippet generator already
  self-limits, or whether the MCP handler needs to (e.g. cap snippets-per-result to 2-3, truncate
  snippet text length) on top of the RPC's own `limit` (which only caps *result count*, not
  *tokens per result*).

## 3. LLM-operator unstated needs (beyond the explicit requirements)

- **Consistent error/result shape across all three new tools, and with the existing 55 tools.**
  The requirements doc names `errResult`/`featureDisabledResult`/argument validation explicitly
  (acceptance criterion #5), but the deeper unstated need is that an LLM client calling 55+ tools
  benefits from *not* having to special-case parsing per tool — every tool already returns
  `MCPResult{Success, Error{Code, Message, Remediation}}` via `okResult`/`errResult`
  (`tools_discovery.go:73-85`). The temptation with `get_backlog_item`'s markdown-text convention
  (section 1 above) is real prior art but breaks this consistency for a *list* tool specifically —
  worth explicitly deciding "list/search tools return JSON `okResult`, single-item detail tools
  may render markdown" as a rule during planning, not leaving each new tool's author to reinvent
  it.
- **Discoverability of valid filter values without a failed call first.** `list_sessions`'s
  `status_filter` solves this today via `mcpgo.Enum("running", "paused", ...)`
  (`server.go:122-125`) — the tool schema itself documents the valid set, and an MCP client
  (including an LLM) can introspect it before calling. The same technique should be applied to
  `list_backlog_items`'s `status`/`priority` and `get_notification_history`'s `type_filter` rather
  than relying on free-text description strings alone — this directly closes the "invalid
  status/priority/type value" edge case class from the research question by making bad input
  unrepresentable at the schema level, not just handled gracefully at runtime.
- **Result summarization / total-vs-returned clarity.** Every one of the three RPCs already
  returns enough to build this (`ListBacklogItemsResponse` has no total distinct from
  `len(items)` since there's no pagination; `GetNotificationHistoryResponse` has
  `total_count`/`unread_count`/`has_more`; `SearchClaudeHistoryResponse` has
  `total_matches`/`has_more`). The tool response should surface "returned N of M results, more
  available" explicitly in a `total_count`/`has_more`-shaped field (matching
  `ListSessionsResult.TotalCount` / `SearchSessionsResult.TotalCount` in `types.go`) so an LLM
  operator can decide whether to page further, rather than silently truncating with no signal —
  this matters most for `list_backlog_items` since that RPC has no native pagination to report
  `has_more` from; the MCP layer will need to compute and report it itself once it adds
  client-side limiting (see section 2 above).
- **Pagination continuation shape should match `list_sessions`'s cursor, not raw offset, where
  practical** — `list_sessions` deliberately hides `offset` behind an opaque cursor
  (`tools_discovery.go:19-41`) so an LLM can't construct an invalid offset by hand; for
  `get_notification_history`/`search_claude_history`, which already have `offset` on the wire
  (and the store/engine already handle out-of-range offsets gracefully per section 2), a raw
  `offset` int argument is defensible and lower-effort than inventing a cursor for RPCs that don't
  need one — but `list_backlog_items`, which needs the MCP layer to invent pagination from
  scratch, is the one place a cursor (vs. a bare `limit`) would be worth the extra code, since
  there's no existing `offset` semantics to match.
- **A single point of truth for "is the backlog feature enabled" gating extends naturally to
  `list_backlog_items`** (it's a backlog-adjacent tool) but **not** to
  `get_notification_history`/`search_claude_history`, which have no existing feature flag in the
  code read here — don't reflexively wrap all three in `featureDisabledResult`; only
  `list_backlog_items` has a real gate to call (`registerBacklogTools` is conditional on
  `storage != nil && (backlogEnabled == nil || backlogEnabled())`, `server.go:63-66`), matching
  where it would be registered.

## Sources read (all in this repo, this pass)

- `project_plans/mcp-search-list-tools/requirements.md`
- `server/mcp/server.go:1-169` (registration, `search_sessions`/`list_sessions`/`get_session` tool
  schemas)
- `server/mcp/tools_discovery.go` (full file — handler implementations + `errResult`/`okResult`)
- `server/mcp/tools_backlog.go:1-260` (handler struct, `validateUUID`, `featureDisabledResult`,
  `get_backlog_item` handler and its markdown-rendering response shape)
- `server/mcp/types.go:55-72` (`ListSessionsResult`, error code constants)
- `proto/session/v1/backlog.proto:300-420` (`ListBacklogItemsRequest`/`Response` and neighboring
  messages)
- `proto/session/v1/session.proto:972-1013` (`SearchClaudeHistoryRequest`/`Response`/`SearchResult`)
- `proto/session/v1/session.proto:1344-1397` (`NotificationHistoryRecord`,
  `GetNotificationHistoryRequest`/`Response`)
- `proto/session/v1/types.proto:778-800` (`NotificationType` enum)
- `server/services/backlog_service_query.go:90-166` (`ListBacklogItems` handler)
- `server/services/notification_service.go:120-200` (`GetNotificationHistory` handler)
- `server/services/search_service.go:440-591` (`SearchClaudeHistory` handler, full body)
- `server/notifications/store.go:60-90,236-277` (`ListOptions`, `List()` pagination/filter logic,
  `MaxNotifications`)
- `session/search/engine.go:1-40` (`SearchOptions`/`SearchResults` struct shapes)
- `session/domain/backlog.go:13-29` (`BacklogStatus` enum values, `DefaultBacklogPriority`)
- `gen/proto/go/session/v1/types.pb.go:831-848` (`NotificationType_name`/`_value` generated maps)
