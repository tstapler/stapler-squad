# Architecture Analysis: Triage Pipeline

## Overview

The triage pipeline takes a backlog item from "idea" status, spawns a hidden Claude agent
session to perform pre-implementation analysis, receives results via MCP tool callback,
and surfaces them in the UI as a TriageReviewPanel. This document traces every hop with
exact file:line references for use in test design.

---

## 1. Full Data Flow (End-to-End)

```
UI (TriggerTriage button / auto on CreateBacklogItem)
  -> BacklogService.TriggerTriage RPC  [server/services/backlog_service.go:1065]
    -> buildTriagePrompt(item, artifactAbsPath, slug)  [backlog_service.go:1213]
         embeds item_id as plain text in prompt: "item_id (pass this...): <uuid>"
    -> sessionCreator.CreateDirectorySession(
           title="triage:<slug>", path=item.RepoPath, prompt=triagePrompt,
           tags=["backlog:triage"], oneShot=!useAutonomous, hidden=true)
         [backlog_service.go:1184-1185]
      -> session.NewInstance(InstanceOptions{MCPServerURL: s.mcpServerURL, ...})
         [session_service.go:601]
      -> instance.buildClaudeCommand()  [session/instance_tmux.go:88]
           adds: --mcp-config '{"mcpServers":{"stapler-squad":{"type":"http",
                   "url":"<MCPServerURL>","headers":{"X-Stapler-Session-UUID":"<inst.UUID>"}}}}'
                 [session/instance_tmux.go:120]
      -> tmux SetExtraEnv(["STAPLER_SESSION_UUID=<inst.UUID>"])
         [session/instance_tmux.go:151]
    -> storage.CreateItemSession(role="triage", sessionUUID=inst.UUID, itemID=item.ID)
         [backlog_service.go:1194]
    -> (if AutonomousDriverStarter wired):
         StartAutonomousDriverWithTimeout(inst, 5*time.Minute)  [backlog_service.go:1191]

Claude agent runs triage prompt inside tmux:
  -> researches codebase, writes research/{stack,features,architecture,pitfalls}.md
  -> writes plan.md, validation.md
  -> calls MCP tool: submit_triage_result(item_id, summary, suggestions, tasks, plan_artifact_path)
      <- HTTP POST /mcp  header: X-Stapler-Session-UUID: <inst.UUID>
      <- server/server.go middleware:
           r = r.WithContext(servermcp.WithSessionUUID(r.Context(), uuid))  [server.go:441-442]
      <- mcp.submitTriageResult(ctx, req)  [server/mcp/tools_backlog.go:423]
           callerSessionUUID(ctx) -> inst.UUID    [tools_backlog.go:36]
           GetItemSessionBySessionAndItem(ctx, inst.UUID, itemID) -> itemSession
             [storage_backlog.go:119]
           role check: itemSession.SessionRole == "triage"  [tools_backlog.go:452]
           marshal TriageResultPayload -> JSON  [tools_backlog.go:498]
           UpdateItemSessionTriageResult(ctx, itemSession.ID, payloadJSON)
             [storage_backlog.go:227]
           UpdateBacklogItem plan_artifacts_path (if provided)  [tools_backlog.go:514]
           EventBus.Publish(NotificationEvent)  [tools_backlog.go:539]

Session exits (oneShot) or AutonomousDriver fires DONE:
  -> BacklogLifecycleListener.onSessionExited(sessionUUID)
  -> UpdateItemSessionEnded(ctx, itemSession.ID, now)
  -> role="triage" -> no item status transitions (lifecycle listener skips)

UI polls / receives push notification:
  -> GetBacklogItem returns item with item_sessions[].triage_result populated
  -> itemSessionToProto() deserializes triage_result JSON -> proto TriageResult
     [backlog_service.go:225]
  -> BacklogItemDetail renders TriageReviewPanel when triageResult is non-empty
```

---

## 2. How item_id Is Threaded from TriggerTriage → Session → MCP Callback

This is the central authorization chain. There are two independent threads:

### Thread A: item_id embedded in the Claude prompt (not used for auth)

`buildTriagePrompt()` at `backlog_service.go:1218` writes:
```
item_id (pass this as item_id to submit_triage_result): <item.ID>
```
Claude reads this from the prompt and includes it as the `item_id` argument when calling
`submit_triage_result`. This is a **data input** — it tells Claude which item to submit for.

### Thread B: session_uuid links session → item (used for authorization)

1. `TriggerTriage` spawns a session via `CreateDirectorySession` → receives `inst` with a
   freshly-assigned `inst.UUID`.  [backlog_service.go:1184]
2. `CreateItemSession` writes a row to `item_sessions` with:
   - `session_uuid = inst.UUID`
   - `backlog_item edge → item.ID`
   - `session_role = "triage"`
   [backlog_service.go:1194]
3. At launch, the tmux session sets `STAPLER_SESSION_UUID=inst.UUID` in env.
4. The `--mcp-config` flag embeds `X-Stapler-Session-UUID: inst.UUID` in the HTTP header.
5. When Claude calls `submit_triage_result`, the HTTP header arrives at `/mcp`.
6. The server middleware extracts the header and calls `WithSessionUUID(ctx, uuid)`.
   [server/server.go:441]
7. `callerSessionUUID(ctx)` returns `inst.UUID`.  [tools_backlog.go:36]
8. `GetItemSessionBySessionAndItem(ctx, inst.UUID, itemID)` queries:
   `WHERE session_uuid = inst.UUID AND backlog_item.ID = itemID`
   [storage_backlog.go:125-131]
9. If found AND `session_role == "triage"`, the call is authorized.

**Key invariant**: `inst.UUID` must appear in BOTH `item_sessions.session_uuid` (written in
step 2) and the `X-Stapler-Session-UUID` MCP header (set in step 4). If either is wrong,
`GetItemSessionBySessionAndItem` returns `ErrNotFound` → `ErrPermissionDenied`.

---

## 3. MCPServerURL and Header Injection

### Where MCPServerURL comes from

`SessionService.mcpServerURL` is set by `SetMCPServerURL(url string)` at server startup
after the listen address is known. [server/services/session_service.go:567]

It propagates to every session created via `CreateDirectorySession`:
```go
opts := InstanceOptions{
    MCPServerURL: s.mcpServerURL,  // session_service.go:601
}
```

### How the header is encoded in the launch command

`instance_tmux.go:118-123` (`claudeMCPConfigFlag`):
```go
func (i *Instance) claudeMCPConfigFlag() string {
    if i.UUID != "" {
        return fmt.Sprintf(`--mcp-config '{"mcpServers":{"stapler-squad":{"type":"http",`+
            `"url":%q,"headers":{"X-Stapler-Session-UUID":%q}}}}'`, i.MCPServerURL, i.UUID)
    }
    return fmt.Sprintf(`--mcp-config '{"mcpServers":{"stapler-squad":{"type":"http","url":%q}}}'`,
        i.MCPServerURL)
}
```

The UUID is embedded in the header value at launch time. It does not change after session
creation. Claude's MCP client sends this header on every HTTP tool call.

### Server-side extraction

`server/server.go:440-445`:
```go
mcpWithUUID := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if uuid := r.Header.Get("X-Stapler-Session-UUID"); uuid != "" {
        r = r.WithContext(servermcp.WithSessionUUID(r.Context(), uuid))
    }
    mcpHTTPHandler.ServeHTTP(w, r)
})
srv.mux.Handle("/mcp", mcpWithUUID)
```

### Stdio fallback (legacy)

For stdio MCP mode, `instance_tmux.go:151` sets `STAPLER_SESSION_UUID` in the tmux env.
`server/mcp/server.go:64` reads `os.Getenv("STAPLER_SESSION_UUID")` and calls
`WithSessionUUID(ctx, uuid)`. The rest of the authorization chain is identical.

---

## 4. The MCP Callback: submit_triage_result

File: `server/mcp/tools_backlog.go:423`

Full authorization and persistence flow:
1. `callerSessionUUID(ctx)` — fails with `ErrPermissionDenied` if UUID absent in context
2. Validate `item_id` UUID format with `uuidRe`  [tools_backlog.go:45]
3. `GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)` — `ErrPermissionDenied` if
   no matching row (session not linked to this item)
4. Check `itemSession.SessionRole == "triage"` — `ErrPermissionDenied` for other roles
5. Parse `suggestions` array: each `{text, rationale}`  → `[]session.TriageSuggestion`
6. Parse `tasks` array: each `{text, estimate, category}` → `[]session.TriageTask`, capped at 12
7. Build `session.TriageResultPayload{Summary, Suggestions, Tasks}` and marshal to JSON
   — uses canonical types from `session/backlog.go:162-183`
8. If `plan_artifact_path` provided: `storage.UpdateBacklogItem(ctx, itemID, {PlanArtifactsPath})`
9. `storage.UpdateItemSessionTriageResult(ctx, itemSession.ID.String(), payloadJSON)`
   — ent update: `SetTriageResult(triageResult).Save(ctx)`  [storage_backlog.go:233]
10. If `eventBus != nil`: publish `NotificationEvent` with `item_id` in metadata

---

## 5. Interface Between Session Layer and Service Layer

### BacklogService's storage access

`BacklogService` holds a `*session.Storage` directly (not an interface):
```go
type BacklogService struct {
    storage       *session.Storage
    sourceBackend itemSourceBackend   // narrow interface, also *session.Storage
    sessionCreator SessionCreator     // narrow interface → *SessionService
    sessionStopper SessionStopper     // narrow interface → *SessionService
    ...
}
```
`session.Storage` wraps an `EntRepository` and delegates to it for all ent operations.
`GetItemSessionBySessionAndItem` is on `*session.Storage` (delegates to `*EntRepository`).
`UpdateItemSessionTriageResult` is on `*EntRepository` directly (called from MCP layer).

### backlogHandlers' storage access

```go
type backlogHandlers struct {
    storage       *session.Storage
    store         session.InstanceStore  // for in-memory session lookup
    eventBus      *events.EventBus
    reviewStopper ReviewCompletionSignaler
}
```
Both `BacklogService` and `backlogHandlers` hold a `*session.Storage`. They are wired
from the same instance in `server/server.go:438`.

### Key storage methods used by the triage pipeline

| Method | File | Purpose |
|---|---|---|
| `CreateItemSession` | storage.go | Creates triage session record |
| `ListItemSessions` | storage.go | Orphan-guard check in TriggerTriage |
| `UpdateItemSessionEnded` | storage.go | Tombstone orphaned sessions |
| `UpdateItemSessionStarted` | storage.go | Mark session as running |
| `GetItemSessionBySessionAndItem` | storage_backlog.go:119 | Auth check in MCP handler |
| `UpdateItemSessionTriageResult` | storage_backlog.go:227 | Persist triage result JSON |
| `UpdateBacklogItem` | storage.go | Write plan_artifacts_path |

---

## 6. Test Types and Mock Points

### What kind of test validates the triage pipeline?

The pipeline has three distinct test boundaries:

**A. Unit tests (existing, fast)** — `server/services/backlog_service_test.go`
- Test `TriggerTriage` business logic: status guards, orphan detection, concurrent-call
  protection, prompt content
- Use real SQLite via `createTestStorage(t)` (temp db per test via `t.TempDir()`)
- Mock the session layer via `mockSessionCreator` (records calls, returns fake `*session.Instance`)
- Mock the session stopper via `mockSessionStopper` (controls `IsSessionLive` return)
- `mockSessionCreator.CreateDirectorySession` returns `&session.Instance{Title: title}` — no UUID
  unless set explicitly; tests that inspect UUID must set it

**B. Unit tests (existing, fast)** — `server/mcp/tools_backlog_test.go`
- Test `submitTriageResult` MCP handler in isolation
- Use real SQLite via `newTestBacklogStorage(t)` (identical pattern to services tests)
- Inject session UUID via `WithSessionUUID(context.Background(), sessUUID)`
- Set up item + ItemSession manually before calling handler
- `setupTriageSession(t, storage)` helper: creates item + triage ItemSession, returns IDs

**C. Integration test (build tag `integration`)** — `session/mcp_integration_test.go`
- Requires `tmux`, `claude`, `git` in PATH; skips otherwise
- `TestSessionStartInWorktreeWithMCP`: verifies MCPServerURL → `--mcp-config` → `mcpServers`
  wrapper in launch command; checks session starts in worktree dir

### Mock points for new tests

| Seam | How to mock | What it isolates |
|---|---|---|
| `SessionCreator` | `mockSessionCreator` | Session spawning (no tmux needed) |
| `SessionStopper` | `mockSessionStopper` | Live-session check (IsSessionLive) |
| `AutonomousDriverStarter` | nil / stub | Driver wiring (oneShot vs autonomous) |
| SQLite storage | `createTestStorage(t)` or `newTestBacklogStorage(t)` | Real DB, test-isolated |
| HTTP middleware | `WithSessionUUID(ctx, uuid)` | Bypasses HTTP layer entirely |
| EventBus | `events.NewEventBus(N)` or nil | Notification publishing |
| MCP handler directly | `handler.submitTriageResult(ctx, req)` | Full handler logic without HTTP |

### Testing the full prompt injection path

The triage prompt is a string built by `buildTriagePrompt()` and passed to
`CreateDirectorySession`. To test it:
- Call `TriggerTriage` with a `mockSessionCreator`
- Inspect `creator.calls[0].prompt` for required substrings:
  - `item_id (pass this as item_id...): <uuid>`
  - `submit_triage_result`
  - `plan_artifact_path`
  - research file paths
- Inspect `creator.calls[0].title` == `"triage:<slug>"`
- Inspect `creator.calls[0].tags` contains `"backlog:triage"`
- Inspect `creator.calls[0].oneShot` == true (when no AutonomousDriver wired)

### Testing the MCP callback (submit_triage_result)

The handler is fully testable without HTTP. Pattern from existing tests:
```go
storage := newTestBacklogStorage(t)
itemID, sessUUID := setupTriageSession(t, storage) // creates item + triage ItemSession
handler := &backlogHandlers{storage: storage}
ctx := WithSessionUUID(context.Background(), sessUUID)
req := makeToolReq(map[string]interface{}{
    "item_id": itemID,
    "summary": "...",
    "tasks":   []interface{}{...},
    "plan_artifact_path": "/some/path",
})
result, err := handler.submitTriageResult(ctx, req)
```
`makeToolReq` and `parseResult` are defined in `server/mcp/testhelpers_test.go`.

For success cases, `result.Content[0].(mcpgo.TextContent).Text` contains the plain-text
response. For error cases, `parseResult(t, result)` returns `map[string]interface{}` with
`"success": false` and `"error": {"code": "..."}`.

---

## 7. Data Structures

### session.TriageResultPayload (canonical JSON type)

File: `session/backlog.go:177`
```go
type TriageResultPayload struct {
    Summary             string            `json:"summary"`
    Suggestions         []TriageSuggestion `json:"suggestions"`
    ClarifyingQuestions []string          `json:"clarifying_questions,omitempty"`
    Tasks               []TriageTask      `json:"tasks,omitempty"`
}
type TriageSuggestion struct { Text, Rationale string }
type TriageTask struct { Text, Estimate, Category string }
```
Both `tools_backlog.go:submitTriageResult` (write path) and `backlog_service.go:itemSessionToProto`
(read path) use these canonical types. The comment at `backlog_service.go:249` explicitly
warns against creating separate inline structs.

### ItemSession DB columns used by triage

```
session_uuid    string   -- links to Instance.UUID; auth key
session_role    string   -- must be "triage" for submit_triage_result to succeed
started_at      *time.Time  -- NULL = orphaned (never confirmed started)
ended_at        *time.Time  -- NULL = still open
triage_result   string   -- JSON-serialized TriageResultPayload
ac_snapshot     string   -- AC JSON at session spawn time
```
Schema: `session/ent/schema/` (generated); mutations via ent builder in `storage_backlog.go`.

---

## 8. Critical Invariants for Test Design

1. **UUID linkage**: `inst.UUID` must be in both `item_sessions.session_uuid` (set by
   `CreateItemSession`) AND the `X-Stapler-Session-UUID` MCP header (encoded in
   `claudeMCPConfigFlag`). Tests bypassing HTTP inject UUID via `WithSessionUUID(ctx, uuid)`.

2. **Role enforcement**: `GetItemSessionBySessionAndItem` must find a row with
   `session_role="triage"`. A "work" or "review" session calling `submit_triage_result`
   returns `ErrPermissionDenied`.

3. **JSON schema stability**: `TriageResultPayload` in `session/backlog.go` is the single
   source of truth. Do not create parallel inline structs — field name drift causes silent
   data loss in `itemSessionToProto`'s `json.Unmarshal`.

4. **started_at NULL = orphan**: A triage session with `started_at=NULL` is tombstoned
   on re-trigger (treated as never-started orphan). Tests that need to block re-trigger
   must call `storage.UpdateItemSessionStarted` before the second `TriggerTriage` call.
   [backlog_service_test.go:392]

5. **oneShot vs autonomous**: `TriggerTriage` sets `oneShot = (s.autonomousStarter == nil)`.
   When no `AutonomousDriverStarter` is wired (most unit tests), `oneShot=true` and the
   prompt is passed as a CLI positional argument. When wired, `oneShot=false` and the prompt
   is injected by `SessionDriver` after Claude reaches idle.

6. **Artifact dir must exist**: `TriggerTriage` calls `os.MkdirAll(artifactAbsPath, 0755)`
   before spawning. Tests that pass a non-temp `RepoPath` may fail here; use `t.TempDir()`
   as the repo path.

7. **`mockSessionCreator` returns no UUID**: The mock returns `&session.Instance{Title: title}`
   with an empty UUID. If a test needs to verify the ItemSession UUID linkage, set UUID on
   the returned instance or use a custom creator.

---

## 9. Existing Test Coverage Gaps Relevant to Validation

Tests that exist:
- `TriggerTriage` status guards, orphan detection, concurrent-call lock, hung-session timeout
  → `backlog_service_test.go:368-469`
- `submitTriageResult` permission checks, notification publishing
  → `tools_backlog_test.go:374-441`
- `GetItemSessionBySessionAndItem` link verification → `backlog_integration_test.go:413`

Gaps (no current tests):
- End-to-end: TriggerTriage → CreateItemSession → MCP callback persists result and the
  result is retrievable via GetBacklogItem (cross-layer integration)
- Prompt content: that `buildTriagePrompt` embeds `item_id` correctly
- Plan artifact path propagation: that `submit_triage_result` writes `plan_artifacts_path`
  to the BacklogItem and it survives a `GetBacklogItem` roundtrip
- Tasks cap enforcement: that > 12 tasks are truncated to 12
- MCPServerURL propagation to InstanceOptions: that `CreateDirectorySession` receives the URL
  (unit-testable via a custom `SessionCreator` mock that captures `InstanceOptions`)
