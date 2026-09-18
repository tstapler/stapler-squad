# Stack Research: backlog-session-item-linking

## Summary

This is a zero-new-dependency change. Every primitive the feature needs already
exists in the codebase — the work is wiring, not new tech adoption. No `go.mod`
or `package.json` changes are anticipated.

## Existing stack (verified from `go.mod` / code)

| Component | Value | Source |
|---|---|---|
| Go MCP tool library | `github.com/mark3labs/mcp-go v0.48.0` | `go.mod:132` |
| Go version | `1.26.3` | `go.mod:3` |
| RPC framework | ConnectRPC (`connectrpc.com/connect`) | `server/services/backlog_service_sync.go:10` |
| Proto codegen | `protoc`/`buf` → `server/gen/proto/go/session/v1`, `web-app/src/gen` via `make proto-gen` | CLAUDE.md |
| Session DB | `github.com/mattn/go-sqlite3 v1.14.40`, ent ORM (`session/ent/`) | `go.mod:24` |
| Session identity | `github.com/google/uuid` | `server/mcp/tools_backlog.go:11` |

## mcp-go: current pin vs. community-recommended version

Current pin is `v0.48.0`. `go list -m -versions github.com/mark3labs/mcp-go` (proxy query, run
2026-08-16) shows the module has moved well past that:

```
... v0.52.0 v0.53.0 v0.54.0 v0.54.1 v0.55.0 v0.56.0 v0.57.0 v0.58.0 v1.0.0-beta.1
```

Latest stable is `v0.58.0`; a `v1.0.0-beta.1` pre-release also exists. **Recommendation: do not
bump mcp-go as part of this feature.** The tool-registration API this feature needs
(`mcpgo.NewTool`, `mcpgo.WithString/WithNumber/WithArray/Required/Enum/Min/Description`,
`s.AddTool`, `mcpgo.NewToolResultText`) is stable across the v0.4x→v0.5x line and is already
exercised by five existing tools in this file — no API surface this feature touches has changed
in a way that would require the bump. Bumping is a separate, unrelated upgrade with its own
regression risk; flag it as a follow-up item, not part of this plan.

## Patterns already established in `server/mcp/tools_backlog.go` (follow these exactly)

1. **Tool registration** (`registerBacklogTools`, `server/mcp/tools_backlog.go:920`): each tool
   is a `s.AddTool(mcpgo.NewTool(name, mcpgo.WithDescription(...), mcpgo.With<Type>(argname,
   mcpgo.Description(...), mcpgo.Required(), ...)), h.<handlerMethod>)` triplet, appended inside
   `registerBacklogTools`. A new `link_session_to_item` (or similarly named) tool follows the
   identical shape — see the `report_progress` block (`tools_backlog.go:932-953`) as the closest
   template (single `item_id` UUID arg + one enum/string arg).

2. **Error envelope**: `errResult(code, message, remediation string) *mcpgo.CallToolResult`
   (`server/mcp/tools_discovery.go:73`) with sentinel codes `ErrPermissionDenied`,
   `ErrItemNotFound`, `ErrFeatureDisabled` (`tools_backlog.go:58-62`) — add any new code
   (e.g. `ErrAlreadyLinked`, `ErrInvalidArgument` if not already present — `ErrInvalidArgument`
   and `ErrInternalError` are already used at `tools_backlog.go:121-132` though not in the
   `const` block shown, confirm before adding a duplicate) to this same block. Goal 2 in
   requirements ("actionable PERMISSION_DENIED errors — name cause + fix-it tool") means the
   existing bare `errResult(ErrPermissionDenied, "this session is not linked to the specified
   backlog item", "")` calls (5 call sites: `tools_backlog.go:304,376,505,665,758`) should have
   their third arg (remediation) filled in to name the new tool, e.g. `"Call
   link_session_to_item with this item_id to attach this session before retrying."`.

3. **Session UUID from context**: `callerSessionUUID(ctx)` (`tools_backlog.go:35`) pulls the
   calling session's UUID out of context (injected via `WithSessionUUID`) — this is how the new
   tool determines *which* session to attach, it should not take a session UUID as a tool
   argument (agents can't discover their own UUID reliably; the harness already injects it).

4. **UUID validation**: `validateUUID(id string) error` (`tools_backlog.go:47`) via
   `uuidRe = regexp.MustCompile(`^[0-9a-f-]{36}$`)` — reuse directly for `item_id`.

5. **Handler struct + interface-scoping**: `backlogHandlers` (`tools_backlog.go:91`) holds
   narrow consumer-defined interfaces, not the concrete `*BacklogService` — e.g.
   `ReviewCompletionSignaler` (`tools_backlog.go:77`, one method:
   `StopDriverForSession(sessionTitle string)`) and `ReviewTrigger` (`tools_backlog.go:85`, one
   method: `TriggerReviewForSession(sessionUUID string)`), both satisfied by `*BacklogService`
   (via `svc`) at the call site `server/mcp/server.go:54`. **This is the pattern to follow per
   `.claude/rules/interface-pollution-checklist.md`**: define a new narrow interface in
   `server/mcp` (the consumer package), e.g.
   ```go
   // SessionAttacher allows the MCP handler to (re)link a session to a backlog item.
   type SessionAttacher interface {
       AttachSessionToItem(ctx context.Context, req *connect.Request[sessionv1.AttachSessionToItemRequest]) (*connect.Response[sessionv1.AttachSessionToItemResponse], error)
   }
   ```
   add a `sessionAttacher SessionAttacher` field to `backlogHandlers`, and pass `svc` for it at
   `server/mcp/server.go:54` alongside `reviewStopper: svc, reviewTrigger: svc`. Do **not** add a
   getter/setter or a forwarding wrapper type — `svc` (`*BacklogService`) already satisfies the
   interface structurally.

## The backend primitive already exists and is already wired end-to-end except MCP exposure

- **Proto**: `AttachSessionToItemRequest`/`Response` messages already defined
  (`proto/session/v1/backlog.proto:421,426`) and the RPC is already declared on the service
  (`proto/session/v1/backlog.proto:752`: `rpc AttachSessionToItem(AttachSessionToItemRequest)
  returns (AttachSessionToItemResponse) {}`). **No proto changes needed** — this eliminates the
  `make proto-gen` step entirely for Gap 1, unlike a typical new-session-creation-mode change
  (contrast with `.claude/rules/session-creation-registry.md`'s 7-touchpoint proto-first flow,
  which does not apply here since this isn't a new session-creation mode).
- **Service method**: `BacklogService.AttachSessionToItem` (`server/services/backlog_service_sync.go:29`)
  is implemented, unit-tested (`backlog_service_test.go:1709`, `backlog_service_triage_test.go:2022`),
  and **already calls `session.WriteSlashCommands(s.pipelineEngine, item, worktreePath)` at
  `backlog_service_sync.go:94`** — meaning Gap 2 (stale slash-command `item_id`) is *already*
  fixed by routing relink through this exact method, with zero additional code. The plan phase
  should confirm this call path is unconditional (not behind a flag) and covers the case where
  `worktreePath` differs between the session doing the relinking and the item's canonical
  worktree.
- **Only missing piece**: no MCP tool exposes `AttachSessionToItem` to an agent session. It is
  only reachable today via `SpawnSessionFromItem` (triage-time session creation) — never
  callable by an already-running session that discovers mid-run it needs relinking, and never
  reachable except via direct SQLite inspection (the workaround the requirements doc describes).

## Read-only introspection (Goal 3 + Goal 5)

Requirements ask for (a) "which item does my branch belong to" resolution and (b) read-only
session↔item linkage introspection. Check before designing a new RPC:
- `get_backlog_item` (`tools_backlog.go:922`) already takes `item_id` and returns role-specific
  guidance keyed on whether the *calling* session is linked — it implicitly proves linkage
  status for a known item_id, but doesn't support "reverse lookup by branch" or "list items
  linked to my session" without already knowing the item_id.
- No existing tool or RPC does branch → item_id resolution. This is new surface (a new query
  method or an addition to an existing one), not a wrapper around an existing primitive — worth
  flagging in the plan phase for scope/complexity, since `item_sessions` join logic and worktree
  branch resolution (likely `session.GetWorktreeDataBySessionUUID`, referenced as
  `resolveSessionBranch`'s default at `tools_backlog.go:106`) both already exist as building
  blocks but aren't currently composed for this query.

## Testing stack (no change)

Standard Go `testing` + table-driven tests, matching existing coverage patterns in
`server/mcp/tools_backlog_test.go`, `server/services/backlog_service_test.go`, and
`server/services/backlog_service_triage_test.go`. No new test framework/dependency needed.
