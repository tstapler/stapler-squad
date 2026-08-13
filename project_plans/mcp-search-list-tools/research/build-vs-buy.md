# Build vs. Buy — MCP search/list tool exposure

Scope: wrap `GetNotificationHistory`, `ListBacklogItems`, `SearchClaudeHistory` (existing
ConnectRPC handlers) as MCP tools, mirroring the existing 55-tool hand-written pattern in
`server/mcp/`.

## 1. Existing OSS library/framework (proto-to-MCP codegen)

**Evidence**: a codegen path does exist —
[`redpanda-data/protoc-gen-go-mcp`](https://github.com/redpanda-data/protoc-gen-go-mcp) is a
protoc/buf plugin that generates `*.pb.mcp.go` files per protobuf service, deriving MCP tool
JSON Schema from the `.proto` method/message descriptors, and is explicitly compatible with
`mark3labs/mcp-go` (this project's library — confirmed in `go.mod:140`,
`github.com/mark3labs/mcp-go v0.48.0`) via a thin adapter. A parallel multi-language option,
[`the-protobuf-project/grpc-mcp-gateway`](https://github.com/the-protobuf-project/grpc-mcp-gateway),
does the same for Go/Python/Rust/C++.

**Pros**
- Removes hand-authored schema/handler boilerplate for future RPC-to-tool wraps.
- Schema derived from `.proto` stays in sync with the message shape automatically (no drift
  between proto field and hand-typed `mcpgo.WithString(...)` args).

**Cons**
- Operates at the *service* level (generates tools for a proto service's RPCs), not a
  hand-picked subset — this project's MCP surface is deliberately curated: 55 tools exist
  today (`grep -c mcpgo.NewTool server/mcp/*.go`), each with custom descriptions tuned for
  LLM consumption (e.g. `list_sessions`'s "Default limit is 10 to avoid filling LLM context"
  hint), feature-flag gating (`featureDisabledResult`), session-UUID injection
  (`WithSessionUUID`), rate limiting (`rate_limiter.go`), and error-shape conventions
  (`errResult(ErrInvalidArgument, ...)`) that a generic codegen tool has no hook for.
- Would introduce a second, structurally different code-generation pipeline
  (`session/ent`'s `--feature sql/upsert` generate step and `make proto-gen` are already the
  two established codegen flows in this repo — see `.claude/rules/ent-schema-generation.md`)
  for a marginal 3-5 tools, alongside 55 hand-written ones that would not be retrofitted.
  A hard cutover (migrate all 55) is out of scope per requirements.md ("Redesigning web UI
  search UX" and broad re-architecture are explicitly out of scope); a partial adoption
  (generated tools for 3, hand-written for 55) is worse — two divergent patterns for the same
  concept in the same package is exactly the kind of unjustified complexity
  `.claude/rules/interface-pollution-checklist.md` flags ("Unjustified generic — used at a
  single call site that a concrete type... would express more clearly," generalized here to
  "unjustified codegen pipeline for a handful of call sites an existing hand-written pattern
  already covers").

**Verdict: do not adopt.** The tool exists and is real, but bringing in a second codegen
dependency to save ~3-5 hand-written tool registrations, when 55 already follow the
hand-written convention and won't be migrated, fails cost/benefit and violates this
project's own interface-pollution discipline. Hand-write these three tools following the
existing pattern.

## 2. SaaS/managed API

Not applicable. `GetNotificationHistory`, `ListBacklogItems`, and `SearchClaudeHistory` are
in-process Go method calls into `*services.NotificationService`, `*services.BacklogService`,
and `*services.SessionService`/`*services.SearchService` respectively (confirmed:
`server/services/notification_service.go:137`, `server/services/backlog_service_query.go:107`,
`server/services/session_service.go:3039` and `:3280`). No external network call or managed
API is involved — this is wiring within the same binary.

## 3. LLM-generated implementation vs. battle-tested library (algorithmic risk)

No new algorithmic surface. All three RPCs are already implemented, tested, and exercised by
the web UI's ConnectRPC handlers:
- `ListBacklogItems` — filtering/sorting logic lives in `BacklogService`, already covered by
  `server/services/backlog_service_test.go`.
- `GetNotificationHistory` — pagination (`limit`/`offset`) and filters
  (`type_filter`/`session_id`/`unread_only`) live in `NotificationService`, covered by
  `server/services/notification_service_extra_test.go`.
- `SearchClaudeHistory` — full-text search runs through the real inverted-index engine in
  `session/search/`, not new code.

The MCP tool layer's own job is a mechanical translation: `mcpgo.CallToolRequest.Arguments`
(untyped `map[string]any`) → typed Connect request struct → `connect.NewRequest(...)` →
existing service method → format `*mcpgo.CallToolResult`. The only genuinely new logic is
argument parsing/validation (type assertions, enum-string-to-proto-enum mapping, cursor
decode/encode for pagination) — and even that isn't new *design*, it's copying the exact
pattern already proven in `list_sessions` (`server/mcp/tools_discovery.go:87-130`): a
`decodeCursor`/`encodeCursor` helper, a `float64`-to-`int` limit coercion with a max clamp,
and `strings.EqualFold` enum matching. No custom data structure or algorithm needs inventing.

**Verdict**: straight mechanical wrap, zero algorithmic risk. LLM-generated implementation
following the in-repo template is appropriate; no need to reach for a battle-tested library.

## 4. Fork or adapt — closest in-repo template

`search_sessions` (`server/mcp/server.go:151-168`) is a valid
reference for the tool-registration shape (`mcpgo.NewTool` + `WithDescription` +
`WithString`/`WithArray`/`WithNumber` + handler function + `d.searchSessions`), but its
argument shape (single free-text query + tag array, no cursor) is simpler than what
`ListBacklogItems`/`GetNotificationHistory` need.

**Better template found**: `list_sessions`, registered immediately above `search_sessions` in
the same file (`server/mcp/server.go:118-137`, handler at
`server/mcp/tools_discovery.go:87-130+`), is a closer match on every axis that matters for
this task:

| Need | `list_sessions` already has it |
|---|---|
| Enum-style string filter | `status_filter` with `mcpgo.Enum(...)` — same shape as `ListBacklogItems`'s `status`/`priority` or `GetNotificationHistory`'s `type_filter` |
| Bounded/defaulted numeric limit | `limit` with `DefaultNumber(10)`, `Min(1)`, `Max(100)` — matches the "default limit 10" convention `requirements.md` (acceptance criterion 3) explicitly asks `SearchClaudeHistory` to follow |
| Pagination | opaque `cursor` string, `decodeCursor`/next-cursor round trip — directly reusable shape for `GetNotificationHistory`'s `limit`/`offset` and any cursoring `SearchClaudeHistory` needs |
| Error convention | `errResult(ErrInternalError, ...)`/`errResult(ErrInvalidArgument, ...)` | 

For the ConnectRPC-call plumbing specifically (calling into a `*services.XxxService` method
via `connect.NewRequest`, not just reading from `InstanceStore` the way `list_sessions`
does), `tools_workflow.go`'s `create_workflow`/`run_workflow` and `tools_backlog.go`'s
existing 22 tools (e.g. `get_backlog_item`, read at `server/mcp/tools_backlog.go:195-213`)
are the closer plumbing template — they already show the `connect.NewRequest(...)` →
`h.svc.Method(ctx, req)` → `errors.Is`/`connect.CodeOf` error-mapping pattern needed for
`ListBacklogItems` (`BacklogService`) and `GetNotificationHistory`/`SearchClaudeHistory`
(`SessionService`/`SearchService`).

**Verdict**: combine two in-repo templates, don't copy `search_sessions` alone —
`list_sessions` for the input-schema shape (enum filter + bounded limit + cursor), and
`tools_backlog.go`'s existing handlers for the Connect-call/error-wrapping plumbing. This is
also the fix for requirements.md's acceptance criterion 4:
`server/mcp/tools_backlog.go:204`'s error-hint text already says
`"from list_backlog_items or get_backlog_item"` — the new tool's registered name should be
literally `list_backlog_items` to make that existing hint correct rather than requiring a
text edit.

## Overall verdict

Build, following the existing in-repo hand-written pattern — specifically
`list_sessions` (schema) + `tools_backlog.go` (Connect-call plumbing) as templates, not
`search_sessions` alone. No SaaS applicable. No algorithmic risk requiring a battle-tested
library. A proto-to-MCP codegen tool (`protoc-gen-go-mcp`) exists and is real, but adopting
it for 3-5 tools while 55 hand-written ones remain would add a second, unjustified codegen
pipeline for marginal benefit — reject per this project's own interface-pollution
discipline.
