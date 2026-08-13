# Stack Research: Triage Pipeline Technology

## Overview

The triage pipeline is a multi-layer flow from a ConnectRPC call through session spawning, Claude invocation (with MCP tool access), and result persistence via ent ORM into SQLite.

---

## 1. Go Packages and Files Involved (End-to-End)

### Entry Point — ConnectRPC Handler
**`server/services/backlog_service.go`**
- `BacklogService.TriggerTriage` (line 1065) is the primary entry point
- Also called automatically from `CreateBacklogItem` (line 402) when `skip_triage=false` and `repo_path` is set

Key sub-functions:
- `buildTriagePrompt` (line 1213) — constructs the full text prompt injected into Claude
- `slugify` (line 159) — generates the artifact dir slug from the item title

### Session Spawning — SessionCreator Interface
**`server/services/session_service.go`**
- `SessionService.CreateDirectorySession` (line 588) satisfies the `SessionCreator` interface
- Wires `MCPServerURL`, sets `AutoYes: true`, `OneShot: !useAutonomous`, `Hidden: true` for triage sessions
- Calls `session.NewInstance(opts)`, then `instance.Start(true)`
- Registers the instance with the storage, event bus, lifecycle listeners, and review queue poller

### Session Layer — Instance and Launch Command
**`session/instance_tmux.go`**
- `buildLaunchCommand` (line 74) dispatches to `buildClaudeCommand`
- `buildClaudeCommand` (line 88) assembles the final `claude` CLI invocation with all flags
- `claudeMCPConfigFlag` (line 118) generates `--mcp-config` JSON, injecting the session UUID as an HTTP header
- `initTmuxSession` (line 126) creates the `tmux.TmuxSession` object; sets `STAPLER_SESSION_UUID` env var on the tmux session (line 152)

### MCP Tool Handlers — Result Ingestion
**`server/mcp/tools_backlog.go`**
- `backlogHandlers.submitTriageResult` (line 423) is the handler for the `submit_triage_result` MCP tool
- `registerBacklogTools` (line 560) mounts all 5 backlog tools on the MCP server
- Session UUID is extracted from context by `callerSessionUUID` -> `sessionUUIDFromContext` (lines 30-42)

**`server/mcp/server.go`**
- `NewCore` (line 22) assembles the MCP server with all tool registrations
- `NewHTTPHandler` (line 49) wraps in `StreamableHTTPServer` for HTTP transport
- HTTP middleware in `server/server.go` (lines 439-445) extracts `X-Stapler-Session-UUID` from the request header and injects it into `context` via `servermcp.WithSessionUUID`

### Storage Layer
**`session/storage.go`**
- `Storage.UpdateItemSessionTriageResult` (line 651) — thin delegation wrapper to `EntRepository`
- `Storage.UpdateBacklogItem` — used to persist `plan_artifacts_path`

**`session/storage_backlog.go`**
- `EntRepository.UpdateItemSessionTriageResult` (line 227) — actual ent ORM write

### Domain Types
**`session/backlog.go`**
- `TriageResultPayload` (line 178) — canonical JSON envelope; used by both write path (`tools_backlog.go`) and read path (`backlog_service.go`)
- `TriageSuggestion`, `TriageTask` — component types
- `BacklogStatus` constants and state machine (`validTransitions` map, line 121)
- `CanTransitionBacklog` (line 154)

---

## 2. Proto RPCs and Message Types

**File: `proto/session/v1/backlog.proto`**

### Key RPCs (service `BacklogService`)

| RPC | Request | Response | Purpose |
|-----|---------|----------|---------|
| `TriggerTriage` | `TriggerTriageRequest` | `TriggerTriageResponse` | Kick off a triage session for an item |
| `CreateBacklogItem` | `CreateBacklogItemRequest` | `CreateBacklogItemResponse` | Create item, optionally auto-triggering triage |
| `ApprovePlan` | `ApprovePlanRequest` | `ApprovePlanResponse` | Approve triage plan artifacts |
| `GetBacklogItem` | `GetBacklogItemRequest` | `GetBacklogItemResponse` | Fetch item with triage result populated |

### Key Messages

| Message | Fields of Note |
|---------|---------------|
| `TriggerTriageRequest` | `item_id` (string) |
| `TriggerTriageResponse` | `item_session` (ItemSession) |
| `ItemSession` | `session_uuid`, `session_role`, `triage_result` (TriageResult) |
| `TriageResult` | `summary`, `suggestions[]`, `clarifying_questions[]`, `tasks[]` |
| `TriageSuggestion` | `text`, `rationale` ("question" marks clarifying questions) |
| `TriageTask` | `text`, `estimate`, `category` |
| `BacklogItem` | `plan_artifacts_path`, `plan_approved`, `item_sessions[]` |

Generated Go bindings: `gen/proto/go/session/v1/`
Generated TypeScript bindings: `web-app/src/gen/session/v1/`

---

## 3. How Claude Is Launched

### Prompt Injection

`buildTriagePrompt` (`server/services/backlog_service.go:1213`) constructs the prompt passed as `Instance.Prompt`. Key contents:
1. Role preamble: "You are a senior software architect performing pre-implementation triage."
2. Item title, `item_id` (the UUID, explicitly labeled "pass this as item_id to submit_triage_result")
3. Description and acceptance criteria (parsed from JSON)
4. Five-step task instructions:
   - Step 1: Spawn 4 parallel subagents writing `research/{stack,features,architecture,pitfalls}.md`
   - Step 2: Write `plan.md`
   - Step 3: Write `validation.md`
   - Step 4: Call `submit_triage_result` MCP tool
   - Step 5: Optionally include clarifying questions in suggestions

Artifact directory path: `{item.RepoPath}/docs/tasks/{slug}/` — created by `os.MkdirAll` before spawning (line 1166).

### Claude CLI Flags (`session/instance_tmux.go:88`)

For a triage session (using `AutonomousDriver` when available, `OneShot` as fallback):

```
claude \
  --mcp-config '{"mcpServers":{"stapler-squad":{"type":"http","url":"http://localhost:8543/mcp","headers":{"X-Stapler-Session-UUID":"<uuid>"}}}}' \
  --dangerously-skip-permissions \
  [-p --output-format json] \
  "<prompt text>"
```

When `AutonomousDriver` is available:
- `oneShot=false` — the `-p` flag is NOT added; Claude runs interactively with the AutonomousDriver managing I/O
- `startAutonomousDriverWithTimeout(inst, 5*time.Minute)` is called (`backlog_service.go:1190`)

When `AutonomousDriver` is unavailable:
- `oneShot=true` — `-p --output-format json` flags are added; Claude runs as one-shot

### Session Identity Flow

1. `Instance.UUID` is generated at `session.NewInstance` time
2. Passed to `tmux.TmuxSession.SetExtraEnv(["STAPLER_SESSION_UUID=<uuid>"])` (`instance_tmux.go:152`)
3. Also embedded in `--mcp-config` header: `"X-Stapler-Session-UUID": "<uuid>"`
4. HTTP middleware at `server/server.go:441` extracts this header and injects into Go context
5. MCP handler reads it via `sessionUUIDFromContext` to authenticate tool calls

---

## 4. How the Result Comes Back

### MCP Tool: `submit_triage_result`

**Registration:** `tools_backlog.go:640`
**Handler:** `backlogHandlers.submitTriageResult` (`tools_backlog.go:423`)

**Parameters accepted:**
- `item_id` (required): UUID of the backlog item
- `summary` (required): 2-3 sentence executive summary
- `suggestions` (optional): `[]{ text, rationale }` — AC gap suggestions or clarifying questions
- `tasks` (optional, max 12): `[]{ text, estimate, category }` — implementation checklist
- `plan_artifact_path` (optional): absolute path to `docs/tasks/{slug}/` directory

**Handler flow:**
1. Extracts caller UUID from context; returns `PERMISSION_DENIED` if absent
2. Validates `item_id` UUID format
3. Looks up `ItemSession` by `(callerUUID, itemID)` via `GetItemSessionBySessionAndItem`
4. Asserts `session_role == "triage"`; returns `PERMISSION_DENIED` otherwise
5. Parses `suggestions[]` into `[]session.TriageSuggestion`
6. Parses `tasks[]` into `[]session.TriageTask`, capped at 12
7. Marshals `session.TriageResultPayload{Summary, Suggestions, Tasks}` to JSON
8. If `plan_artifact_path` non-empty: calls `storage.UpdateBacklogItem` to set `plan_artifacts_path` on `BacklogItem`
9. Calls `storage.UpdateItemSessionTriageResult(itemSession.ID, payloadJSON)` to persist on `ItemSession`
10. If `eventBus` wired: publishes a `NOTIFICATION_TYPE_INPUT_REQUIRED` event to notify the operator

### Persistence Chain (`submit_triage_result` -> SQLite)

```
tools_backlog.go:submitTriageResult
  +-- storage.UpdateItemSessionTriageResult(ctx, itemSession.ID, jsonString)
      [session/storage.go:651]
      +-- EntRepository.UpdateItemSessionTriageResult(ctx, id, triageResult)
          [session/storage_backlog.go:227]
          +-- r.client.ItemSession.UpdateOneID(parsedID).
                SetTriageResult(triageResult).
                Save(ctx)
              writes to SQLite `item_sessions.triage_result` column
```

### Read Path (triage result back to proto)

`backlogItemToProto` in `backlog_service.go` calls `itemSessionToProto` (line 173). For each `ItemSession`, if `is.TriageResult != ""`, it unmarshals JSON into `session.TriageResultPayload` and converts to `*sessionv1.TriageResult` (lines 225-246).

---

## 5. Key Dependencies

### `sync.Map` Usage in `BacklogService`

`server/services/backlog_service.go`:
- `triggerInProgress sync.Map` (field, line 78): per-item mutex preventing concurrent `TriggerTriage` TOCTOU races. Uses `LoadOrStore` (line 1097) and `defer Delete` (line 1101).
- `worktreeMu sync.Mutex` (field, line 74): serializes `WriteSlashCommands` + `WriteBacklogContextFile` calls within `SpawnSessionFromItem` and `AttachSessionToItem` to prevent interleaved writes.

### ent ORM Schema — Backlog-Related Tables

**`BacklogItem`** (`session/ent/schema/backlog_item.go`):
- Key fields: `id` (UUID PK), `title`, `description`, `acceptance_criteria` (JSON `[]AcCriterion`), `status`, `repo_path`, `plan_approved`, `plan_artifacts_path`
- Edges: `item_sessions` (->ItemSession), `status_events` (->BacklogStatusEvent), `source` (<-ItemSource)
- Indexes: `(status, priority)`, `(status, updated_at)`, `external_id`, `status`

**`ItemSession`** (`session/ent/schema/item_session.go`):
- Key fields: `id` (UUID PK), `session_uuid` (loose FK to Session — not an ent edge), `session_role` ("work"/"triage"/"review"), `triage_result` (JSON `TriageResultPayload`), `ac_snapshot` (JSON `[]AcCriterion`)
- Edges: `backlog_item` (<-BacklogItem, required), `review_verdict` (->ReviewVerdict, unique)
- Indexes: `session_uuid` (critical — used on every EventExited hook); `(created_at, backlog_item)`

**`ReviewVerdict`** (`session/ent/schema/review_verdict.go`):
- Key fields: `overall_outcome`, `per_criterion` (JSON `[]CriterionVerdict`), `summary`, `override_by/reason/at`
- Edge: `item_session` (<-ItemSession, required, unique)
- Index: `item_session` (unique — one verdict per ItemSession)

### External Libraries

| Library | Role |
|---------|------|
| `github.com/mark3labs/mcp-go` | MCP server framework (`mcpgo`, `mcpserver` packages) |
| `connectrpc.com/connect` | ConnectRPC framework for BacklogService RPC handlers |
| `entgo.io/ent` | ORM for SQLite; generates type-safe query builders |
| `github.com/google/uuid` | UUID generation and parsing |

### MCP Transport

The triage session connects to the MCP server over **HTTP Streamable transport** (MCP 2025-03-26 spec), not stdio. The endpoint is `http://localhost:8543/mcp`. Claude's `--mcp-config` flag configures this. The server identifies the calling Claude instance by the `X-Stapler-Session-UUID` HTTP header (`server/server.go:441`).

---

## Critical Invariants for Validation

1. **Role enforcement**: `submit_triage_result` returns `PERMISSION_DENIED` if `session_role != "triage"`. The `ItemSession` must exist and be linked to both the caller UUID and the item.
2. **Concurrent protection**: `triggerInProgress sync.Map` prevents two concurrent `TriggerTriage` calls from racing through orphan checks on the same item.
3. **Orphan detection**: An open triage `ItemSession` is tombstoned (EndedAt set, tmux session killed) if: `started_at == nil`, session is not live in memory, or session has run > 2 hours (`maxTriageDuration`).
4. **Stale tmux cleanup**: Before spawning, `KillTmuxSessionByTitle("triage:<slug>")` is called to prevent reattachment to a stale tmux session that would skip prompt injection.
5. **Artifact path persistence**: `plan_artifacts_path` is written to `BacklogItem` (not `ItemSession`) via `UpdateBacklogItem` — required before `ApprovePlan` can succeed.
