# Architecture Research: MCP tools for ListBacklogItems, GetNotificationHistory, SearchClaudeHistory

Scope: Agent 3 of the mcp-search-list-tools SDD research phase. No prior architecture doc
covered `server/mcp/`; this is from-scratch research, not a delta on existing notes.

## 1. Wiring pattern: in-process Go calls, never a wire hop

`server/mcp/server.go`'s `NewCore` / `NewHTTPHandler` / `RunServer` all take the *same live
Go objects* the HTTP/ConnectRPC server uses — `session.InstanceStore`, `*services.SessionService`,
`*session.Storage`, `*events.EventBus`, `*services.BacklogService` — as constructor parameters,
and store them on small per-resource handler structs (`discoveryHandlers`, `backlogHandlers`,
`workflowHandlers`, `rulesHandlers`, ...). `server/server.go:564` proves this concretely:

```go
mcpHTTPHandler := servermcp.NewHTTPHandler(deps.Storage, deps.SessionService, deps.ScrollbackManager,
    deps.Storage, deps.EventBus, deps.UserPRCache, deps.BacklogEnabledCheck, autoReopener, deps.BacklogService)
```

`deps.SessionService` is the literal same `*services.SessionService` instance registered as the
ConnectRPC handler elsewhere in `server/server.go`. MCP tool handlers call methods on it directly
(`h.svc.CreateWorkflow(ctx, connect.NewRequest(...))` in `tools_workflow.go:215`) — `connect.NewRequest`
just wraps the proto struct; there is no HTTP round trip, no serialization boundary, no auth
interceptor in between (grepped for an auth/permission Connect interceptor across `server/*.go` —
none exists; this app has no per-request auth layer to bypass in the first place).

**Two coexisting sub-patterns**, both in-process, differing in what layer they call:

| Pattern | Example | What it calls |
|---|---|---|
| **A. Call the ConnectRPC handler method directly** | `workflowHandlers.createWorkflow` → `h.svc.CreateWorkflow(ctx, connect.NewRequest(protoReq))` (`tools_workflow.go:215`); `rulesHandlers.listApprovalRules` → `h.svc.ListApprovalRules(...)` (`tools_rules.go:142`) | The exact same Go method the ConnectRPC transport invokes — full proto request/response types, gets any service-layer logic embedded in that handler for free. |
| **B. Call the storage/session layer directly, bypassing the RPC handler** | `discoveryHandlers.listSessions` → `d.store.LoadInstances()` + hand-rolled status filter/pagination (`tools_discovery.go:97-160`), reimplementing what a `ListSessions` RPC would do rather than calling one; `backlogHandlers.getBacklogItem` → `h.storage.GetBacklogItem(ctx, itemID)` (`tools_backlog.go:200`), not `BacklogService.GetBacklogItem` | The underlying storage/store type, with filtering/formatting logic duplicated in the MCP handler (and typically reshaped into human-readable text, not proto passthrough — see `get_backlog_item`'s markdown-style output). |

Pattern B is used where the MCP tool's output shape diverges heavily from the RPC's proto
response (e.g. `get_backlog_item` builds a markdown-ish summary with acceptance-criteria
checkboxes and the latest review verdict inlined — not a JSON dump of `GetBacklogItemResponse`),
or where the storage call is trivial enough that reimplementing it costs less than plumbing a
service reference through. Pattern A is used where the RPC handler does something the MCP tool
would otherwise have to duplicate (workflow CRUD validation, approval-rule upsert semantics).

**Consequence for this feature**: which pattern to use is not a style choice, it's forced by
how much non-trivial logic sits in the service layer above the storage call — see §2 and §5.

## 2. Data flow per target RPC

### ListBacklogItems
`proto/session/v1/backlog.proto:780` (request at :336) → Go handler
`BacklogService.ListBacklogItems` (`server/services/backlog_service_query.go:107`):

```go
func (s *BacklogService) ListBacklogItems(ctx, req) (*connect.Response[...], error) {
    filter := session.BacklogItemFilter{ SortBy: ..., ExcludeDone: ..., ExcludeArchived: ... }
    // Statuses/Priorities filter overrides ...
    summaries, err := s.storage.ListBacklogItemSummaries(ctx, filter)   // session/storage.go:732
    costFor := s.buildCostLookup()                                      // unexported, backlog_service.go:431
    for i := range summaries {
        protoItems[i] = backlogItemSummaryToProto(&summaries[i], costFor) // unexported, backlog_service.go:590
    }
    return ...
}
```

`buildCostLookup` and `backlogItemSummaryToProto` are **unexported** functions in
`server/services` — an MCP handler in package `mcp` cannot call `session.Storage.ListBacklogItemSummaries`
and reimplement the same cost-annotated proto conversion without either exporting those two
symbols or duplicating their logic. This pushes `list_backlog_items` toward **Pattern A** (call
`BacklogService.ListBacklogItems` directly), not Pattern B.

`backlogHandlers` (`tools_backlog.go:138`) already carries a `backlogSvc *services.BacklogService`
field, wired for `create_backlog_item`/`import_github_issue`'s post-create auto-triage trigger
(`MaybeTriggerTriage`). **That field is optional/nilable** — per `server/mcp/server.go:96-103`,
the stdio fallback path (`buildMCPDeps` in `main.go`, Phase-1-only deps) constructs `NewCore`/`RunServer`
with `backlogSvc: nil`. Existing nil-`backlogSvc` call sites degrade gracefully (skip triage
silently, item still created). `list_backlog_items` has no equivalent degrade-gracefully option —
without `backlogSvc` it cannot get cost-annotated results at all. **Integration point to resolve in
planning**: either (a) require `backlogSvc != nil` and return an internal/unavailable error on the
Phase-1 stdio path (accepting the tool is only usable when the full server wiring is present), or
(b) fall back to `h.storage.ListBacklogItemSummaries` + a locally-reimplemented (nil-cost) proto
conversion when `backlogSvc` is nil. Recommend (a) — it matches the existing `enabledCheck`/
`backlogSvc` optionality doc comments' precedent of "some capability quietly narrows on the Phase-1
path" rather than introducing a second, cost-less code path for a field the rest of the file
already treats as service-owned.

### GetNotificationHistory
`proto/session/v1/session.proto:135` (request at :1367) → `SessionService.GetNotificationHistory`
(`server/services/session_service.go:3280`), a one-line delegate to
`NotificationService.GetNotificationHistory` (`server/services/notification_service.go:137`):

```go
func (ns *NotificationService) GetNotificationHistory(ctx, req) (*connect.Response[...], error) {
    if ns.notificationStore == nil { return empty, nil }
    opts := notifications.ListOptions{ Limit, Offset, TypeFilter, SessionID, UnreadOnly }
    records, totalCount, err := ns.notificationStore.List(opts)
    // convert records -> proto, compute HasMore
}
```

Nothing here is unexported/inaccessible from `mcp` — it's a thin proto→options→store-call→proto
round trip. Both patterns are viable mechanically, but **Pattern A** (`svc.GetNotificationHistory(ctx,
connect.NewRequest(...))`, using the same `svc *services.SessionService` already threaded into
`NewCore`) is simpler: it reuses the request/response proto types verbatim (this data is naturally
tabular/filterable, unlike `get_backlog_item`'s markdown reshaping) and needs zero new plumbing —
`svc` is already a required-ish param on `NewCore` (nilable only in edge-case test paths, unlike
`backlogSvc`).

### SearchClaudeHistory
`proto/session/v1/session.proto:80` (request at :972) → `SessionService.SearchClaudeHistory`
(`server/services/session_service.go:3039`), delegating to `SearchService.SearchClaudeHistory`
(`server/services/search_service.go:459`). This one is **not** a thin wrapper:

```go
func (ss *SearchService) SearchClaudeHistory(ctx, req) (*connect.Response[...], error) {
    hist, err := ss.getOrRefreshHistoryCache(ctx)                 // singleflight-guarded cache
    syncResult, err := ss.searchEngine.IncrementalSync(hist)      // session/search/ inverted index
    // ... telemetry spans, limit/offset clamping (limit default 20, max 100) ...
    // search.SearchOptions{Limit, Offset} -> ss.searchEngine.Search(...)
}
```

It owns an in-memory inverted index (`session/search/`) rebuilt incrementally on each call via
`singleflight` (`golang.org/x/sync/singleflight`) to coalesce concurrent rebuild attempts, plus
OpenTelemetry span instrumentation. Reimplementing this in an MCP handler (Pattern B) would mean
duplicating cache/index lifecycle management — a clear case for **Pattern A**: call
`svc.SearchClaudeHistory(ctx, connect.NewRequest(...))` directly and let the existing service own
the index. This confirms requirements.md's characterization of `SearchClaudeHistory` as "backed by
`session/search/` (inverted index)."

**Summary**: all three target RPCs favor **Pattern A** (call the existing ConnectRPC handler
method in-process) — `list_backlog_items` because the proto-conversion/cost-lookup helpers are
unexported, `get_notification_history` because it's a trivial reuse-as-is win, and
`search_claude_history` because the service owns non-trivial cached/indexed state an MCP handler
has no business reimplementing. This is a partial departure from `get_backlog_item`/`list_sessions`
Pattern-B precedent — justified per-RPC above, not a blanket "always use Pattern A" rule.

## 3. File placement

Existing convention confirmed via `ls server/mcp/`: one `tools_<resource>.go` (+ matching
`tools_<resource>_test.go`) per resource area — `tools_backlog.go`, `tools_discovery.go`,
`tools_github.go`, `tools_goal.go`, `tools_lifecycle.go`, `tools_rules.go`, `tools_terminal.go`,
`tools_vcs.go`, `tools_workflow.go`. Each file defines a `register<Resource>Tools(s, h)` function
called from `NewCore`.

- **`list_backlog_items`** → `server/mcp/tools_backlog.go`, added to `registerBacklogTools`
  (`tools_backlog.go:1716`), reusing the existing `backlogHandlers` struct (already carries
  `storage`, `backlogSvc`, `enabledCheck`). This directly resolves the dangling reference at
  `tools_backlog.go:204` (`get_backlog_item`'s remediation text already says "Provide a valid UUID
  (e.g. from list_backlog_items or get_backlog_item)" for a tool that doesn't exist yet).

- **`get_notification_history`** → new file, `server/mcp/tools_notifications.go` (+
  `tools_notifications_test.go`). No notification-resource file exists today; a new
  `notificationHandlers{svc *services.SessionService}` struct (mirroring `workflowHandlers`'s
  single-field, svc-only shape in `tools_workflow.go`) is the right size — don't fold this into
  `tools_discovery.go` (session-resource-only today) or `tools_backlog.go` (unrelated resource;
  would violate the one-file-per-resource-area convention this repo already enforces).

- **`search_claude_history`** → new file, `server/mcp/tools_history.go` (+ matching test file),
  again with a small `historyHandlers{svc *services.SessionService}` struct. Naming rationale:
  the underlying resource is "Claude conversation history" (`session/search/`, `GetClaudeHistoryMessages`
  lives in the same `SearchService`), distinct from both `tools_discovery.go` (live session
  metadata) and any future `SearchFiles`/`SearchGitHubRepos` tools (explicitly out of scope here,
  but would plausibly land in `tools_history.go` too or their own files later — not this pass's
  call).

  Registration for both new files' `register*Tools` calls belongs inside the existing
  `if svc != nil { ... }` block in `server/mcp/server.go:59-62` (alongside
  `registerWorkflowTools`/`registerRulesTools`), since both new tools require `svc` and have no
  meaningful degraded-but-functional mode without it — matching how workflow/rules tools are
  already gated.

## 4. Resource-scoped tools vs. one unified `search` tool

**Recommendation: resource-scoped**, confirming (not just defaulting to) the requirements.md lean.

Reasoning:

- **Schema clarity per call.** The four candidate "resources" — sessions, backlog items,
  notifications, Claude history — have almost no overlapping filter shape:
  `search_sessions` takes `query` + `tag_filter`; `ListBacklogItems` takes `status`/`priority`
  (repeated enums) + `sort_by` + two include-booleans; `GetNotificationHistory` takes
  `type_filter`/`session_id`/`unread_only` + offset-based pagination; `SearchClaudeHistory` takes
  `project`/`model`/`start_time`/`end_time` + cursor-based pagination. A unified tool with a
  `resource` enum parameter would need a near-union of all these fields, most marked "only valid
  when resource=X" in prose (MCP tool schemas have no conditional-field-visibility primitive) —
  worse for an LLM caller than four tools whose name alone scopes which fields exist.
- **No dispatch logic actually saved.** A unified tool's handler still needs an internal
  switch-on-resource to call the right backend method — the branching moves from
  `NewCore`'s tool registration into one handler function; total code doesn't shrink, and the
  per-resource error/remediation text (`errResult` messages) gets harder to keep resource-specific.
- **Precedent consistency.** All 55+ existing tools in this codebase are resource-scoped
  (`search_sessions`, `get_backlog_item`, `list_sessions`, ...). Introducing one unified `search`
  tool now would be the only multi-resource tool in the file, an inconsistency future maintainers
  would have to explain, not a pattern this pass has any mandate to establish (requirements.md
  explicitly scopes `SearchFiles`/`SearchGitHubRepos` out — a unified tool would need to
  anticipate their eventual inclusion to be worth the schema complexity, which is speculative
  design for RPCs not in scope here).
- **Tool-list discoverability cost is real but small and asymmetric.** Adding 3 more tools grows
  the MCP `tools/list` payload the client sees at session start — a real cost — but it's linear
  and already large (55+ tools). A unified tool doesn't eliminate that cost so much as trade
  "more tool names, simpler each" for "fewer tool names, each with a wider and partially-invalid
  parameter surface" — worse for the model's ability to pick correct arguments on the first try,
  which is the more expensive failure mode (a wrong/malformed call + error-result round trip costs
  more context than one extra line in the tool list).

Net: resource-scoped wins on both schema-correctness and precedent grounds; the "many small tools"
downside is real but smaller than the downside of a multiplexed schema for this specific
low-field-overlap set of resources.

## 5. Consistency/staleness: what does bypassing the RPC layer skip?

- **No auth/permission interceptor exists to skip.** Grepped `server/*.go` and
  `server/services/*.go` for an auth/permission Connect interceptor — none found. This is a
  single-user, localhost-bound app; there is no per-request identity/authorization check at the
  ConnectRPC layer for any of these three RPCs to accidentally bypass by calling the Go method
  in-process instead of over HTTP.
- **The one real gate is the `backlog` feature flag**, and it's opt-in per-resource, not blanket
  middleware: `FeatureFlagService` (`server/services/feature_flag_service.go:50`) registers exactly
  one controllable feature named `"backlog"` — there is no equivalent registered flag for
  notifications or Claude-history search. `backlogHandlers.enabledCheck` /
  `featureDisabledResult(h.enabledCheck)` (`tools_backlog.go:71`) exists specifically because a
  `backlogEnabled func() bool` is threaded through `NewCore`/`server.go` for that one subsystem.
  **Consequence**: `list_backlog_items` must call `featureDisabledResult(h.enabledCheck)` like
  every other tool in `tools_backlog.go` (AC5 applies as written). `get_notification_history` and
  `search_claude_history` have **no corresponding flag to gate on today** — AC5's "feature-flag
  gate (`featureDisabledResult`)" requirement is only satisfiable for these two if the plan phase
  decides to introduce new flags for notifications/search (not currently in scope per
  requirements.md's "no new backend search capability"); otherwise these two tools should register
  unconditionally, same as `search_sessions`/`list_sessions` today (also ungated).
- **Nil-store degrade-gracefully patterns already exist and should be preserved.**
  `NotificationService.GetNotificationHistory` returns an empty response when
  `ns.notificationStore == nil` rather than erroring; `BacklogService.ListBacklogItems` returns an
  empty response when `s.storage == nil`. Calling these methods in-process (Pattern A) means the
  MCP tool inherits these safe defaults for free — another point in favor of Pattern A over
  reimplementing storage calls directly, which would require re-deriving the same nil-guards.
- **Staleness/caching**: `SearchClaudeHistory`'s `IncrementalSync` runs synchronously per-call
  (singleflight-coalesced across concurrent callers), so an MCP tool calling `svc.SearchClaudeHistory`
  gets the same freshness guarantee the web UI gets — no additional staleness risk introduced by
  the MCP path specifically.

## Summary of concrete file/integration targets

| Tool | File | Handler struct | Calls |
|---|---|---|---|
| `list_backlog_items` | `server/mcp/tools_backlog.go` (existing) | `backlogHandlers` (existing) | `h.backlogSvc.ListBacklogItems(ctx, connect.NewRequest(...))` — resolve nil-`backlogSvc` fallback in planning |
| `get_notification_history` | `server/mcp/tools_notifications.go` (new) | `notificationHandlers{svc}` (new) | `h.svc.GetNotificationHistory(ctx, connect.NewRequest(...))` |
| `search_claude_history` | `server/mcp/tools_history.go` (new) | `historyHandlers{svc}` (new) | `h.svc.SearchClaudeHistory(ctx, connect.NewRequest(...))`, default `limit` clamp mirroring `list_sessions`'s LLM-context-safe default |

All three registrations belong inside `server/mcp/server.go`'s existing `if svc != nil { ... }`
block (list_backlog_items also needs the existing `storage != nil && backlogEnabled` gate it's
already under).
