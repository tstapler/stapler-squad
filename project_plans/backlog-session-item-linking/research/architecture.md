# Architecture Research: `backlog-session-item-linking`

Builds directly on `project_plans/backlog-configurable-pipeline/research/architecture.md`
(cited inline as "prior doc"). That project has since shipped: `PipelineEngine` and
`WriteSlashCommands(engine PipelineEngine, item *BacklogItemData, worktreePath string) error`
are real code today (`session/backlog_commands.go:20-31`), not a proposal. This matters
directly for goal (c) below.

## (a) MCP tool reusing `AttachSessionToItem`

### What `AttachSessionToItem` already does (`server/services/backlog_service_sync.go:29-131`)

1. Validates `item_id`/`session_uuid` present (lines 40-46).
2. Loads the item; **requires `item.Status` in `{idea, ready, in_progress}`**, else
   `CodeFailedPrecondition` (lines 55-63). This is the load-bearing guard for goal (a)'s safety —
   see the state-transition analysis below.
3. Snapshots `AcceptanceCriteria` (line 65) and loads prior `ItemSession`s for the item (68-73).
4. Unconditionally **inserts a new `ItemSession` row** (`CreateItemSession`, lines 76-84) with
   `SessionRole: session.SessionRoleWork` hardcoded. There is no dedup/upsert against an existing
   `(session_uuid, item_id)` pair, and no check for whether `session_uuid` already has an
   `ItemSession` pointing at a *different* item — repeated or cross-item calls simply add more
   rows. `ItemSession` has no unique constraint on `(session_uuid, backlog_item)`
   (`session/ent/schema/item_session.go:73-84`, indexes at 86-90 only cover `session_uuid` alone
   and a composite for item+time ordering).
5. If a live `Instance` with matching `UUID` and non-empty `Path` is found in the instance store,
   writes slash commands and the backlog context file into that worktree, under `s.worktreeMu`
   (lines 86-113) — see goal (c) below, this step already exists.
6. **Side effect**: if `session.CanTransitionBacklog(item.Status, BacklogStatusInProgress)` is
   true, transitions the item to `in_progress` (lines 123-130, "step 7" in the source comment —
   there is no separate `transitionItemToInProgress` helper; it's inlined here). On transition
   failure it does not fail the RPC — it logs and calls `s.notifyTransitionFailed` (same pattern
   `SpawnSessionFromItem` uses for the analogous fresh-spawn case).

### Is the auto-transition side effect safe to trigger from an arbitrary relink call?

**Yes, safe as-is, and no new parameter is needed** — the safety comes from the status guard in
step 2, not from the transition logic itself:

- The method already refuses to run at all unless `item.Status ∈ {idea, ready, in_progress}`
  (step 2). An item in `review`, `pr_pending`, `done`, `blocked`, etc. cannot be relinked through
  this path — `CodeFailedPrecondition` fires before any write happens.
- Within the allowed set, `CanTransitionBacklog(idea → in_progress)` and
  `CanTransitionBacklog(ready → in_progress)` are both legitimate forward transitions (this is the
  same transition `SpawnSessionFromItem` triggers on first spawn), and
  `CanTransitionBacklog(in_progress → in_progress)` is a no-op self-transition that
  `CanTransitionBacklog` returns `false` for (repeated relinks of an already-`in_progress` item are
  therefore idempotent no-ops on this step, not repeated transition attempts).
- So "relink" via this method can only ever move an item *forward* into `in_progress`, never
  backward out of `review`/`done`/etc., and never repeatedly. The existing guard is exactly the
  right shape for an MCP-exposed relink tool — do not weaken it, and do not add a bypass flag.

One real design question for the plan phase: `AttachSessionToItem` hardcodes
`SessionRole: session.SessionRoleWork`. If the new MCP tool is meant to be callable from any
session (including a triage or review session self-linking), the tool should probably pass the
caller's own role through, or the plan phase should decide the tool is *work-role-only* by
definition (matching what `AttachSessionToItem` already assumes as its sole existing caller,
`SpawnSessionFromItem`, does). Recommend the latter — scope the new tool to "attach as a work
session," matching the method's only proven semantics today; a distinct triage/review linking
concern is out of scope per the requirements' non-goals ("no generalized multi-agent handoff
protocol").

### Integration point: `backlogHandlers` has no `*BacklogService` reference today

This is the key gap the plan phase must close. `server/mcp/tools_backlog.go:91-110`
(`backlogHandlers` struct) holds `storage *session.Storage`, `store session.InstanceStore`,
`eventBus`, `reviewStopper ReviewCompletionSignaler`, `reviewTrigger ReviewTrigger`,
`enabledCheck`, plus two GitHub-verification func fields — **no `*services.BacklogService`, no
`PipelineEngine`, no `worktreeMu`**. `AttachSessionToItem` needs the latter two internally (for
step 5's `session.WriteSlashCommands(s.pipelineEngine, item, worktreePath)` call and its own mutex
for the concurrent-write race it documents).

`reviewStopper`/`reviewTrigger` are the established pattern for this exact problem —
`server/mcp/server.go:54`:
```go
registerBacklogTools(s, &backlogHandlers{storage: storage, store: store, eventBus: eventBus,
    reviewStopper: svc, reviewTrigger: svc, enabledCheck: backlogEnabled})
```
`svc` here is `*services.SessionService`, passed twice as two different narrow interfaces
(`ReviewCompletionSignaler`, `ReviewTrigger`, defined at `server/mcp/tools_backlog.go:77-87`) that
`*services.SessionService` happens to satisfy. The same pattern should be used for this feature:
define a narrow `SessionAttacher` interface in `tools_backlog.go` —
```go
// SessionAttacher lets the MCP handler link/relink the calling session to a backlog item by
// delegating to BacklogService.AttachSessionToItem, which already owns the ItemSession write,
// slash-command regeneration, and forward-only in_progress transition. Implemented by
// *services.BacklogService.
type SessionAttacher interface {
    AttachSessionToItem(ctx context.Context, req *connect.Request[sessionv1.AttachSessionToItemRequest]) (*connect.Response[sessionv1.AttachSessionToItemResponse], error)
}
```
add `attacher SessionAttacher` to `backlogHandlers`, and wire `attacher: backlogSvc` at the one
call site inside `NewCore` (`server/mcp/server.go:53-56`). The new handler then just builds a
`connect.Request[sessionv1.AttachSessionToItemRequest]{ItemId: itemID, SessionUuid: callerUUID}`
(caller UUID from `callerSessionUUID(ctx)`, same as the 5 existing PERMISSION_DENIED sites) and
calls `h.attacher.AttachSessionToItem(ctx, req)` — zero duplication of steps 2-6 above.

### But `NewCore`/`NewHTTPHandler`/`RunServer` don't currently receive a `*BacklogService` either

Tracing all constructors of the MCP server (`server/mcp/server.go:29` `NewCore`, `:70`
`NewHTTPHandler`, `:85` `RunServer`) — none accept a `*services.BacklogService` parameter today.
Two real call sites, with different reach to a live `BacklogService`:

1. **HTTP path (the common case)** — `server/server.go:502`:
   `servermcp.NewHTTPHandler(deps.Storage, deps.SessionService, deps.ScrollbackManager,
   deps.Storage, deps.EventBus, deps.UserPRCache, deps.BacklogEnabledCheck)`. `deps` here is the
   struct populated at `server/dependencies.go:940`
   (`backlogSvc := services.NewBacklogService(storage, sessionService, cfg, workflowEngine,
   pipelineEngine, pipelineModeRepo)`) and exposed as `deps.BacklogService` (declared at
   `server/dependencies.go:65` and `:398`, assigned at `:1200`). **This path can pass
   `deps.BacklogService` straight through with no new construction** — just thread one more
   parameter down `NewHTTPHandler → NewCore → registerBacklogTools`.

2. **stdio path (`--mcp` flag, `main.go:97`)** — this is the fallback used only "before the daemon
   has started" (comment at `main.go:70-77`); the normal case is a **thin-client proxy**
   (`mcpserver.RunProxyServer`, `main.go:73-82`) that forwards tool calls over HTTP to the already-
   running daemon's `/mcp` endpoint from path 1 above — so the proxy path inherits whatever
   `NewHTTPHandler` is wired with for free, no separate plumbing needed. Only the **local fallback**
   (`buildMCPDeps()`, `main.go:1040-1055`) is missing a `BacklogService`: it calls
   `server.BuildCoreDeps()` and returns only `core.Storage`/`core.SessionService` — no
   `BacklogService`, and `BuildCoreDeps` does not construct `cfg`/`workflowEngine`/`pipelineEngine`/
   `pipelineModeRepo` the way `dependencies.go:940` does (confirmed by the existing comment at
   `main.go:70-72`: "a load-time read of the flag... is sufficient" specifically because
   `BuildCoreDeps` doesn't construct a live `BacklogController`).
   - **Recommendation for the plan phase**: either (a) extend `BuildCoreDeps`/`buildMCPDeps` to
     also construct a `*services.BacklogService` (heavier — pulls in `workflowEngine`,
     `pipelineEngine`, `pipelineModeRepo` construction into the stdio-fallback code path that today
     deliberately avoids it), or (b) accept that the new MCP tool is a no-op/unavailable
     (`CodeUnavailable`, matching the existing `if s.storage == nil` pattern in
     `AttachSessionToItem` itself) when `attacher` is nil, exactly like `reviewStopper`/
     `reviewTrigger` already tolerate nil today. **(b) is the lower-risk choice** — the fallback
     path is explicitly a "daemon not up yet" degraded mode, and relink-while-daemon-is-down is an
     edge case the requirements don't call out. Flag this as a plan-phase decision, not a blocker.

## (b) Making the 5 `PERMISSION_DENIED: this session is not linked...` sites actionable

All 5 sites (`server/mcp/tools_backlog.go:304, 376, 505, 665, 758` — in `reportProgress`,
`requestReview`, and three more MCP handlers) share the identical shape:
```go
_, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
if linkErr != nil {
    if errors.Is(linkErr, session.ErrNotFound) {
        return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", "..."), nil
    }
    ...
}
```
`errResult(code, message, remediation string)` (`server/mcp/tools_discovery.go:73-77`) already
has a `remediation` field designed for exactly this — most of the 5 sites currently pass `""` for
it (only line 304's `reportProgress` populates it, with generic prose, no fix-it tool name).

**The fix is data already on hand, not new plumbing**: `session.ErrNotFound` from
`GetItemSessionBySessionAndItem` only tells you *this* `(session, item)` pair isn't linked — it
doesn't tell the agent what it IS linked to. `session/storage.go:932-938`
(`Storage.GetItemSessionBySessionUUID(ctx, sessionUUID)`, no item filter, already public, already
used by `session/backlog_lifecycle.go:825` etc.) does exactly that reverse lookup — most-recent
`ItemSession` row for a session UUID alone, `ent`-ordered `Desc(created_at)`, with the
`BacklogItem` edge preloaded (`storage_backlog.go:185-198`). A shared helper —
```go
// actionablePermissionDenied builds a PERMISSION_DENIED result that tells the agent what
// item (if any) it IS linked to, and names the fix-it tool.
func (h *backlogHandlers) actionablePermissionDenied(ctx context.Context, callerUUID, wantItemID string) *mcpgo.CallToolResult {
    linked, err := h.storage.GetItemSessionBySessionUUID(ctx, callerUUID)
    if err == nil && linked.BacklogItemID != "" {
        return errResult(ErrPermissionDenied,
            fmt.Sprintf("this session is not linked to item %s — it is currently linked to item %s", wantItemID, linked.BacklogItemID),
            fmt.Sprintf("Call link_session_to_item with item_id=%s to relink, or use item_id=%s if that's what you meant.", wantItemID, linked.BacklogItemID))
    }
    return errResult(ErrPermissionDenied,
        "this session is not linked to any backlog item",
        fmt.Sprintf("Call link_session_to_item with item_id=%s to link this session before retrying.", wantItemID))
}
```
called from all 5 sites in place of the current inline `errResult(...)`, is a small, mechanical,
single-file change. It depends on nothing from goal (a) except the new tool's *name* (pick it in
the plan phase — `link_session_to_item` used above as a placeholder) so the remediation string can
reference it; the lookup itself (`GetItemSessionBySessionUUID`) is already-shipped code.

## (c) Regenerating slash commands after a relink — already implemented for the reuse path

This is the one goal that needs **no new code** if goal (a) is built as "MCP tool thin-wraps
`AttachSessionToItem`" (the recommended approach above): `AttachSessionToItem`'s step 5
(`server/services/backlog_service_sync.go:86-113`) already calls
`session.WriteSlashCommands(s.pipelineEngine, item, worktreePath)` and
`session.WriteBacklogContextFile(item, attachPriorSessions, worktreePath)` for the matching live
`Instance`, under `s.worktreeMu`, every time it's invoked. The requirements' Gap 2 repro ("its
generated commands reference a nonexistent item id") is a symptom of `AttachSessionToItem` never
being callable *by the agent itself* mid-session (only by `SpawnSessionFromItem` at creation
time) — not a symptom of `WriteSlashCommands` lacking a regeneration path. Exposing (a) as an
agent-callable tool **is** the fix for (c); no separate regeneration mechanism needs designing.

One conditional worth flagging for the plan phase: step 5's slash-command rewrite only fires
`if inst.Path != ""` and the instance is found in `LoadInstances()` by UUID match
(`backlog_service_sync.go:87-89`). For the MCP tool's caller — a live session calling its own MCP
tool — this should always resolve (the calling session obviously has a path), but confirm at
implementation time that the `Instance` lookup by `req.Msg.SessionUuid` (== `callerUUID`) reliably
finds the currently-running session and not a stale/duplicate entry.

## (d) Read-only session↔item linkage introspection tool

Same building block as (b): `Storage.GetItemSessionBySessionUUID(ctx, callerUUID)`
(`session/storage.go:932-938`) already returns an `ItemSessionSummary` (`session/repository.go:285-
303`: `BacklogItemID`, `Role`, `PipelineModeSnapshot`, `StartedAt`, `LastCommitSha`, etc.) with the
`BacklogItem` edge preloaded. A new read-only MCP tool (e.g. `get_linked_item` — naming is a
plan-phase decision) is a direct wrap:
```go
func (h *backlogHandlers) getLinkedItem(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    if r := featureDisabledResult(h.enabledCheck); r != nil { return r, nil }
    callerUUID, err := callerSessionUUID(ctx)
    if err != nil { return errResult(ErrPermissionDenied, err.Error(), "..."), nil }
    is, err := h.storage.GetItemSessionBySessionUUID(ctx, callerUUID)
    if err != nil {
        if errors.Is(err, session.ErrNotFound) {
            return okResult(map[string]any{"linked": false}), nil
        }
        return errResult(ErrInternalError, fmt.Sprintf("lookup failed: %v", err), ""), nil
    }
    return okResult(map[string]any{"linked": true, "item_id": is.BacklogItemID, "role": is.Role, ...}), nil
}
```
No storage-layer or service-layer changes required — this is strictly an MCP-layer addition,
following the existing `get_backlog_item` tool's shape (`tools_backlog.go:922-930`) but keyed off
the caller's session identity instead of a supplied `item_id`. This is also the natural
implementation for requirements goal 3 ("resolve which item my session belongs to without
hand-parsing") — no git-branch parsing needed at all; the `item_sessions` table is already the
source of truth and is one already-public storage call away.

## Summary of integration points (for the plan phase task list)

| Concern | File:Line | Change needed |
|---|---|---|
| New MCP tool registration | `server/mcp/tools_backlog.go:920` `registerBacklogTools` | Add `link_session_to_item` (goal a) and `get_linked_item` (goal d) tool defs + handlers |
| `backlogHandlers` needs `BacklogService` access | `server/mcp/tools_backlog.go:91-110` | Add narrow `SessionAttacher` interface + `attacher` field, mirroring `ReviewCompletionSignaler`/`ReviewTrigger` |
| Wiring at HTTP path | `server/mcp/server.go:29` (`NewCore`), `:70` (`NewHTTPHandler`), `server/server.go:502` | Thread `deps.BacklogService` through as a new parameter |
| Wiring at stdio fallback path | `main.go:1040-1055` (`buildMCPDeps`), `main.go:85` (`RunServer`) | Plan-phase decision: extend `BuildCoreDeps` or tolerate nil `attacher` (recommend nil-tolerant) |
| Actionable errors | `server/mcp/tools_backlog.go:304,376,505,665,758` | Replace inline `errResult(...)` with shared `actionablePermissionDenied` helper using `GetItemSessionBySessionUUID` |
| Slash-command regeneration | `server/services/backlog_service_sync.go:86-113` (inside `AttachSessionToItem`) | No change — already fires on every call |
| Item state transition safety | `server/services/backlog_service_sync.go:55-63` (status guard), `:123-130` (transition) | No change — guard already makes the transition forward-only and idempotent |
| `item_sessions` ent edge | `session/ent/schema/item_session.go:73-90` | No schema change; note no unique constraint on `(session_uuid, backlog_item)` — repeated/cross-item attaches accumulate rows rather than upserting, which is what makes `GetItemSessionBySessionUUID`'s "most recent by created_at" semantics load-bearing |
