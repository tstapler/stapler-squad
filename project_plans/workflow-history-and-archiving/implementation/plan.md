# Post-Implementation Plan: Workflow History, Archiving & Pause Memory Optimization

## Epic

Adds workflow run history and session archiving to Stapler Squad. Sessions created by the scheduler now carry a `workflow_id` string that links them back to their originating workflow. A new `archived_at` nillable timestamp field enables soft-archiving: `ArchiveSession` / `UnarchiveSession` RPCs set and clear it, `ListSessions` excludes archived sessions by default, and workflow-spawned sessions are automatically archived via a `LifecycleListener` hook when they exit. The WorkflowsPanel gains a Run button (calls `RunWorkflow` RPC) and a per-workflow Recent Runs accordion (queries `listSessionsByWorkflow`). The Pause path is rewritten to kill the tmux session rather than detach it, freeing memory on resource-constrained hosts; Resume reconstructs the `TmuxSession` object from scratch using the latest stored Claude conversation UUID so `--resume <uuid>` is always correct.

---

## Stories and Tasks (as implemented)

### Story 1: Session<->Workflow Linkage — DONE

Foundation: allows sessions to know which workflow created them and enables run-history queries.

#### Task 1.1 — Ent schema: `workflow_id` + `archived_at` fields — DONE

Added to `session/ent/schema/session.go`:
- `field.String("workflow_id").Optional()` with `.Comment()`
- `field.Time("archived_at").Optional().Nillable()` with `.Comment()`
- Two new indexes: `index.Fields("workflow_id")`, `index.Fields("archived_at")`

Ent codegen run with the required `--feature sql/upsert` flag (per CLAUDE.md constraint).

#### Task 1.2 — Proto: new fields on Session + CreateSessionRequest + ListSessionsRequest + Archive RPCs — DONE

Changes across two proto files:
- `proto/session/v1/types.proto`: `string workflow_id = 60` and `google.protobuf.Timestamp archived_at = 61` added to the `Session` message.
- `proto/session/v1/session.proto`:
  - `string workflow_id = 23` added to `CreateSessionRequest`
  - `optional string workflow_id = 7` added to `ListSessionsRequest`
  - `bool include_archived = 8` added to `ListSessionsRequest`
  - `rpc ArchiveSession(ArchiveSessionRequest) returns (ArchiveSessionResponse) {}`
  - `rpc UnarchiveSession(UnarchiveSessionRequest) returns (UnarchiveSessionResponse) {}`
  - `ArchiveSessionRequest` / `ArchiveSessionResponse` / `UnarchiveSessionRequest` / `UnarchiveSessionResponse` message definitions added

`make generate-proto` run; Go and TypeScript bindings regenerated.

#### Task 1.3 — Backend: `WorkflowID` threaded through Instance, serialization, repo, adapter — DONE

- `session/instance.go`: `WorkflowID string` field on `Instance` struct (line ~262) and `WorkflowID string` in `InstanceOptions` (line ~473).
- `session/instance_serialization.go`: `WorkflowID` and `ArchivedAt` serialized in `ToInstanceData()` (lines 93–94) and deserialized in `FromInstanceData()` (lines 243–244).
- `session/ent_repository.go`: `SetWorkflowID` / `ClearWorkflowID` and `SetArchivedAt` / `ClearArchivedAt` in the upsert path (lines ~412–421); `data.WorkflowID` and `data.ArchivedAt` populated on read (lines 940–941).
- `server/adapters/instance_adapter.go`: `protoSession.WorkflowId = inst.WorkflowID` and `timestamppb.New(*inst.ArchivedAt)` → `protoSession.ArchivedAt` (lines 140–142).
- `server/services/session_service.go` `CreateSession`: `WorkflowID: req.Msg.WorkflowId` passed to `InstanceOptions` (line ~957).
- `server/services/session_service.go` `ListSessions`: in-memory filter skips archived sessions when `!req.Msg.IncludeArchived` (line ~701); skips sessions where `WorkflowID != *req.Msg.WorkflowId` when filter is set (line ~706).
- `server/workflows/scheduler.go` `FireNow`: `WorkflowId: wf.ID.String()` in the `CreateSessionRequest` (line 173).

---

### Story 2: Archive RPCs + Auto-Archive — DONE

#### Task 2.1 — `ArchiveSession` / `UnarchiveSession` handlers — DONE

Both implemented in `server/services/session_service.go` (lines ~3571–3611):
- Both require `session_id`, call `FindLiveInstance`, mutate `inst.ArchivedAt` directly on the in-memory instance, then persist via `s.storage.SaveInstances`.
- `ArchiveSession` sets `inst.ArchivedAt = &now`; `UnarchiveSession` clears it to `nil`.
- Return `CodeNotFound` if session is not in memory; `CodeInternal` on save failure.

Note: handlers operate on in-memory `FindLiveInstance` only — they do NOT load sessions from DB if not already in memory.

#### Task 2.2 — `autoArchiveListener` + `wireAutoArchiveCallback` + `maybeAutoArchive` — DONE

- `autoArchiveListener` struct (lines ~3240–3250): implements `LifecycleListener`. Fires `go maybeAutoArchive(inst)` on `EventExited`.
- `wireAutoArchiveCallback` (lines ~3231–3238): registers the listener on the instance; no-ops if `inst.WorkflowID == ""`.
- `maybeAutoArchive` (lines ~3613–3626): guards on `inst == nil || inst.WorkflowID == "" || inst.ArchivedAt != nil`, then sets `ArchivedAt` and calls `SaveInstances`.
- `wireAutoArchiveCallback` called at three callsites: `loadInstancesWithWiring` (line ~289), `CreateDirectorySession` (line ~578), and the async `CreateSession` path (line ~1012).

---

### Story 3: WorkflowsPanel UI — DONE

#### Task 3.1 — `archiveSession`, `unarchiveSession`, `listSessionsByWorkflow`, `runWorkflow` in `useSessionService` — DONE

All four methods added to `web-app/src/lib/hooks/useSessionService.ts` (lines ~554–616 and ~955–958):
- `archiveSession(id)` → calls `client.archiveSession(ArchiveSessionRequest)`
- `unarchiveSession(id)` → calls `client.unarchiveSession(UnarchiveSessionRequest)`
- `listSessionsByWorkflow(workflowId, includeArchived = true)` → calls `client.listSessions` with `{ workflowId, includeArchived }`; returns up to all matching sessions (slicing to 5 done in the UI layer)
- `runWorkflow({ id, arg? })` → calls `client.runWorkflow(RunWorkflowRequest)`; returns `sessionId` string or `null`

#### Task 3.2 — WorkflowsPanel: Run button + RecentRuns accordion — DONE

`web-app/src/components/workflows/WorkflowsPanel.tsx`:
- `RecentRuns` component (lines ~33–87): collects sessions lazily on first expand via `listSessionsByWorkflow(workflowId, true)`, slices to last 5 (`.slice(-5).reverse()`), renders status badge + session link + formatted timestamp.
- Run button in Actions column (lines ~234–239): `▶ Run` text; disabled and shows `…` while `runningId === wf.id`; calls `handleRun(wf)` → `runWorkflow({ id: wf.id })`.
- Form modal uses `createPortal(..., document.body)` (per CSS architecture rule).
- `WorkflowsPanel.css.ts` updated with `runsAccordion`, `runsToggle`, `runsList`, `runRow`, `statusBadge`, `runLink`, `runButton` styles.

---

### Story 4: Pause Memory Optimization — DONE

#### Task 4.1 — Pause kills tmux instead of detach — DONE

`session/instance.go` Pause path (lines ~970–980):
- Calls `i.KillSession()` after the controller is stopped and the Claude session UUID is confirmed saved.
- Falls back to `i.pm().DetachSafely()` on kill failure (non-fatal, logged at Warn level).
- Fallback-to-detach on `DetachSafely` failure appends to `errs`.

#### Task 4.2 — Resume reinitializes TmuxSession with latest UUID — DONE

`session/instance.go` Resume dead-tmux path (lines ~1074–1108):
- Reads `i.claudeSession.ConversationUUID` as `claudeSessionID`.
- Calls `i.buildLaunchCommand(claudeSessionID)` to construct a command string with `--resume <uuid>`.
- Type-asserts `i.processManager.(*TmuxBackend)` and calls `tb.TmuxManager().SetSession(newSession)` — either `NewTmuxSessionWithServerSocket` or `NewTmuxSessionWithPrefix` depending on `TmuxServerSocket`.
- Sets `STAPLER_SESSION_UUID` env var on the new session object.
- Calls `i.pm().Start(worktreePath)` to launch the fresh tmux session.

---

## Key Files Changed

| File | Change |
|---|---|
| `session/ent/schema/session.go` | Added `workflow_id` optional string field, `archived_at` nillable time field, and two new indexes |
| `proto/session/v1/types.proto` | Added `workflow_id = 60` and `archived_at = 61` to `Session` message |
| `proto/session/v1/session.proto` | Added `workflow_id = 23` to `CreateSessionRequest`; `workflow_id = 7` and `include_archived = 8` to `ListSessionsRequest`; `ArchiveSession` / `UnarchiveSession` RPCs and their message types |
| `gen/proto/go/session/v1/session.pb.go` | Regenerated (includes new fields and Archive RPCs) |
| `gen/proto/go/session/v1/sessionv1connect/session.connect.go` | Regenerated (includes `ArchiveSession` / `UnarchiveSession` client/server stubs) |
| `session/instance.go` | Added `WorkflowID string` to `Instance` struct and `InstanceOptions`; added `ArchivedAt *time.Time` to `Instance`; rewrote Pause to call `KillSession` with detach fallback; added dead-tmux Resume reinit path |
| `session/instance_serialization.go` | Serializes/deserializes `WorkflowID` and `ArchivedAt` in `ToInstanceData` / `FromInstanceData` |
| `session/ent_repository.go` | Upsert path: `SetWorkflowID` / `ClearWorkflowID`, `SetArchivedAt` / `ClearArchivedAt`; read path: populate `WorkflowID` and `ArchivedAt` from ent row |
| `server/adapters/instance_adapter.go` | `InstanceToProto`: maps `WorkflowID` → `proto.WorkflowId` and `ArchivedAt` → `proto.ArchivedAt` |
| `server/services/session_service.go` | `ListSessions` in-memory filter (archived exclusion + workflow_id filter); `ArchiveSession` / `UnarchiveSession` handlers; `autoArchiveListener` struct; `wireAutoArchiveCallback`; `maybeAutoArchive`; `wireAutoArchiveCallback` called at 3 callsites; `CreateSession` passes `WorkflowID` through `InstanceOptions` |
| `server/workflows/scheduler.go` | `FireNow`: passes `WorkflowId: wf.ID.String()` in `CreateSessionRequest` |
| `web-app/src/lib/hooks/useSessionService.ts` | Added `archiveSession`, `unarchiveSession`, `listSessionsByWorkflow`, `runWorkflow` methods |
| `web-app/src/components/workflows/WorkflowsPanel.tsx` | Added `RecentRuns` component and Run button to workflow table rows |
| `web-app/src/components/workflows/WorkflowsPanel.css.ts` | Added styles for `runsAccordion`, `runsToggle`, `runsList`, `runRow`, `statusBadge`, `runLink`, `runButton` |

---

## Architecture Decisions

Three decisions were documented in `research/architecture.md` and implemented as described:

**Plain string `workflow_id` (not FK edge):** `workflow_id` is stored as `field.String(...).Optional()` with no ent edge. Orphan-safe: deleting a Workflow leaves session history intact. Avoids join overhead in the hot `ListSessions` path — filter is a simple string equality check on an indexed field. The `ListSessions` filter is applied in-memory against the loaded instance slice, not as a DB predicate.

**`archived_at` nillable timestamp (not bool):** Provides nil-vs-non-nil semantics for "never archived" vs "currently archived", an implicit audit timestamp, and the `ClearArchivedAt` / `SetArchivedAt` ent API that maps cleanly to `*time.Time`. The index on `archived_at` supports future range queries.

**`autoArchiveListener` as LifecycleListener (not polling):** Matches the existing `BacklogLifecycleListener` pattern. Fires exactly once per session exit. `maybeAutoArchive` is dispatched in a goroutine from `OnLifecycleEvent` so it does not block the exit event path.

**Pause kills tmux (not detach):** Primary memory optimization for US-6. The kill-then-reinit contract requires `wireClaudeSessionIDSavedCallback` to persist the UUID before the kill, and `Resume()` to call `SetSession` on the `TmuxBackend` with a fresh `TmuxSession` built from `buildLaunchCommand(claudeSessionID)`.

---

## Deferred

The following items were explicitly deferred during requirements scoping:

- **"Show archived" toggle in main session list** — `archiveSession` / `unarchiveSession` hooks exist in `useSessionService` but there is no archive button or filter toggle in the session list UI. This was noted as out-of-scope in `requirements.md` but was listed as in-scope in the task document (`docs/tasks/workflow-history-and-archiving.md` Story 4). It was not implemented.
- **Bulk archive operations**
- **Archived session TTL / hard-delete**
- **Workflow run notifications / webhooks**
- **Analytics / metrics on workflow runs**
- **`runWorkflow` navigation to created session** — the `runWorkflow` hook returns `sessionId` but `WorkflowsPanel` does not navigate to the new session after clicking Run; it only shows a loading spinner and returns to idle state.
