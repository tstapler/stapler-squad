# Implementation Plan: backlog-workflow-engine

**Feature**: Extensible backlog workflow engine with `refining` state, WorkflowEngine interface, and custom state CRUD
**Date**: 2026-05-19
**Status**: Ready for implementation
**ADRs**: ADR-013 (`docs/adr/013-workflow-engine-replaces-valid-transitions.md`)

---

## Dependency Visualization

```
Phase 0 (Safety Net)
  └─► Phase 1 (refining + WorkflowEngine)
        ├─► Phase 2 (WorkflowConfig persistence + custom state CRUD)
        │     └─► Phase 3 (Gate types + Visual builder)
        └─► (transition history can ship with Phase 1 or Phase 2)

Within Phase 1:
  Epic 1.1 (WorkflowEngine interface)
    └─► Epic 1.2 (refining state + backend)
          └─► Epic 1.3 (triage loop integration)
                └─► Epic 1.4 (frontend + stuck reconciler)

Within Phase 2:
  Epic 2.1 (ent schema: WorkflowConfig)
    └─► Epic 2.2 (RPCs + service layer)
          └─► Epic 2.3 (settings UI)

Phase 3 depends on Phase 2 (gates reference WorkflowConfig transitions).
```

---

## Phase 0: Safety Net — Open the TypeScript Status Union

**Goal**: Audit and open the `BacklogItemStatus` union before any new states land. Prevents TypeScript compile failures and silent runtime breakage when `refining` appears from the server.

### Epic 0.1: Frontend Status Audit

**Goal**: Replace the closed `BacklogItemStatus` union with an open-string pattern so unknown statuses render gracefully instead of breaking.

#### Story 0.1.1: Open the `BacklogItemStatus` type and add fallback rendering
**As a** developer, **I want** the frontend to accept server-returned statuses it doesn't recognize, **so that** deploying a new state never breaks the board UI.
**Acceptance Criteria**:
- `BacklogItemStatus` type in `useBacklogService.ts` accepts `string` for unknown values
- Known-status display paths (labels, CSS classes) have safe fallbacks for unrecognized values
- No TypeScript errors introduced
- All 91 existing status references compile cleanly

**Files**:
- `web-app/src/lib/hooks/useBacklogService.ts`
- `web-app/src/app/backlog/page.tsx`
- `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 0.1.1a: Open the union type (~3 min)
- In `useBacklogService.ts` line 19, change `BacklogItemStatus` from a closed union to:
  ```ts
  export type KnownBacklogStatus = "idea" | "ready" | "refining" | "in_progress" | "review" | "done" | "archived";
  export type BacklogItemStatus = KnownBacklogStatus | (string & {});
  ```
- This preserves autocomplete for known values while accepting any string.
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 0.1.1b: Add fallback to `STATUS_LABELS` and `STATUS_CSS` in `page.tsx` (~3 min)
- Change `STATUS_LABELS: Record<BacklogItemStatus, string>` to `Record<string, string>` with a fallback getter helper:
  ```ts
  const getStatusLabel = (s: string) => STATUS_LABELS[s] ?? s.replace(/_/g, " ");
  const getStatusClass = (s: string) => STATUS_CSS[s] ?? styles.statusDefault;
  ```
- Replace all direct `STATUS_LABELS[item.status]` and `STATUS_CSS[item.status]` calls with the helper.
- Add `"refining"` entry to `ALL_STATUSES`, `STATUS_LABELS`, and `STATUS_CSS` with placeholder styling.
- Files: `web-app/src/app/backlog/page.tsx`

##### Task 0.1.1c: Add fallback to `BacklogItemDetail.tsx` status maps (~3 min)
- Same pattern as 0.1.1b for `STATUS_LABELS` and `STATUS_CLASS` in `BacklogItemDetail.tsx`.
- Add `"refining"` entry with label "Refining" and a neutral CSS class.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 0.1.1d: Verify no TypeScript errors (~2 min)
- Run `cd web-app && npx tsc --noEmit` to confirm zero new errors.
- Run `cd web-app && npx jest --no-coverage` to confirm tests still pass.
- Files: none (verification only)

---

## Phase 1: `refining` State + WorkflowEngine Foundation

**Goal**: Ship the `refining` state end-to-end and introduce the `WorkflowEngine` interface so future state changes are single-file edits.

### Epic 1.1: WorkflowEngine Interface

**Goal**: Extract the transition policy into a seam so `BacklogService` and `BacklogLifecycleListener` are testable and decoupled from the hardcoded map.

#### Story 1.1.1: Define `WorkflowEngine` interface and `DefaultWorkflowEngine`
**As a** developer, **I want** a `WorkflowEngine` interface backed by the existing `validTransitions` logic, **so that** the service layer has a substitutable policy object.
**Acceptance Criteria**:
- `WorkflowEngine` interface defined in `session/workflow_engine.go`
- `DefaultWorkflowEngine` wraps existing `validTransitions` + `TransitionGuard`
- Existing `CanTransitionBacklog` and `TransitionGuard` free functions remain in `backlog.go` (not deleted)
- All existing tests pass with `make test`

**Files**:
- `session/workflow_engine.go` (new file)
- `session/backlog.go`

##### Task 1.1.1a: Create `session/workflow_engine.go` with interface and `DefaultWorkflowEngine` (~5 min)
- Define `WorkflowEngine` interface with three methods: `CanTransition`, `ValidateGates`, `AllowedTransitions` (see ADR-013 for signatures).
- Implement `DefaultWorkflowEngine` struct with a `transitions map[BacklogStatus]map[BacklogStatus]bool` field.
- `CanTransition` delegates to the map. `ValidateGates` delegates to `TransitionGuard`. `AllowedTransitions` iterates the map row.
- Add constructor `NewDefaultWorkflowEngine() *DefaultWorkflowEngine` that copies `validTransitions` into the struct field (no shared mutable state).
- Files: `session/workflow_engine.go`

##### Task 1.1.1b: Wire `WorkflowEngine` into `BacklogService` (~4 min)
- Add `engine WorkflowEngine` field to `BacklogService` struct in `server/services/backlog_service.go`.
- Update `NewBacklogService` signature to accept `engine WorkflowEngine`; store it.
- In `TransitionBacklogItemStatus` (line 576), replace `session.CanTransitionBacklog(from, to)` with `s.engine.CanTransition(from, to)`.
- Replace `session.TransitionGuard(guardInput, to)` (line 599) with `s.engine.ValidateGates(guardInput, to)`.
- Files: `server/services/backlog_service.go`

##### Task 1.1.1c: Wire `WorkflowEngine` into `BacklogLifecycleListener` (~4 min)
- Add `engine WorkflowEngine` field to `BacklogLifecycleListener` in `session/backlog_lifecycle.go`.
- Update `NewBacklogLifecycleListener` and `NewBacklogLifecycleListenerWithSpawner` to accept `engine WorkflowEngine`.
- In `onSessionExited`, replace the hardcoded `BacklogStatusReview` default with `engine.AllowedTransitions(BacklogStatusInProgress)[0]` (first non-archived target, or keep `review` as the default for the triage role check).
- Files: `session/backlog_lifecycle.go`

##### Task 1.1.1d: Update wiring in `server/dependencies.go` (~3 min)
- Construct `engine := session.NewDefaultWorkflowEngine()` near line 380.
- Pass `engine` to `NewBacklogLifecycleListenerWithSpawner` and to `NewBacklogService`.
- Files: `server/dependencies.go`

---

### Epic 1.2: `refining` State — Backend

**Goal**: Add `refining` as a first-class state in the Go layer, with correct transitions and guard rules.

#### Story 1.2.1: Add `refining` to `BacklogStatus` constants and `DefaultWorkflowEngine`
**As a** backend developer, **I want** `refining` to be a valid status with defined transitions, **so that** the triage loop can set it and the board can display it.
**Acceptance Criteria**:
- `BacklogStatusRefining` constant added
- `validTransitions` (and `DefaultWorkflowEngine`) include: `idea→refining`, `refining→ready`, `refining→archived`
- `CanTransitionBacklog` and `engine.CanTransition` both return `true` for those edges
- No existing test broken

**Files**:
- `session/backlog.go`
- `session/workflow_engine.go`

##### Task 1.2.1a: Add `BacklogStatusRefining` constant and update `validTransitions` (~3 min)
- Add `BacklogStatusRefining BacklogStatus = "refining"` to the constants block in `session/backlog.go`.
- Add map entries:
  ```go
  BacklogStatusIdea: { BacklogStatusReady: true, BacklogStatusRefining: true, BacklogStatusArchived: true },
  BacklogStatusRefining: { BacklogStatusReady: true, BacklogStatusArchived: true },
  ```
- Files: `session/backlog.go`

##### Task 1.2.1b: Update `DefaultWorkflowEngine` constructor to include `refining` entries (~2 min)
- `NewDefaultWorkflowEngine` copies from `validTransitions`; this picks up the new entries automatically if task 1.2.1a copies from the map. Confirm constructor still uses the updated map.
- Files: `session/workflow_engine.go`

##### Task 1.2.1c: Add `refining` guard rule to `TransitionGuard` and `DefaultWorkflowEngine.ValidateGates` (~3 min)
- In `TransitionGuard` in `session/backlog.go`, add a case for `idea → refining`: AC is not required (triage will gather it). Return `nil`.
- Add case `refining → ready`: require non-empty `AcCriteriaJSON` (same rule as `idea → ready`).
- Files: `session/backlog.go`

##### Task 1.2.1d: Write unit tests for `refining` transitions (~4 min)
- In `session/backlog_test.go` (or new `session/workflow_engine_test.go`), add table-driven tests:
  - `idea → refining` allowed, no guard error
  - `refining → ready` allowed, fails guard when AC empty, passes when AC non-empty
  - `refining → archived` allowed
  - `idea → ready` (existing) still requires AC
- Files: `session/backlog_test.go` or `session/workflow_engine_test.go`

---

#### Story 1.2.2: Per-item transition history (append-only log)
**As a** user, **I want** to see a history of status transitions for a backlog item, **so that** I can audit the triage loop and understand how the item evolved.
**Acceptance Criteria**:
- `BacklogStatusEvent` ent entity exists with fields: `item_id`, `from_status`, `to_status`, `triggered_by` (string: "user", "triage", "lifecycle"), `created_at`
- `TransitionBacklogItemStatus` in storage appends a `BacklogStatusEvent` row on every successful transition
- `ListBacklogStatusEvents(itemID)` storage method exists
- Proto has `ListBacklogStatusEvents` RPC returning a list of events
- No UI required in v1 (data model only + RPC)

**Files**:
- `session/ent/schema/backlog_status_event.go` (new)
- `session/ent/` (regenerated)
- `session/storage_backlog.go` (or equivalent storage file)
- `proto/session/v1/backlog.proto`
- `server/services/backlog_service.go`

##### Task 1.2.2a: Define `BacklogStatusEvent` ent schema (~3 min)
- Create `session/ent/schema/backlog_status_event.go`:
  ```go
  // Fields: item_id (UUID, FK to BacklogItem), from_status (string), to_status (string),
  //         triggered_by (string, default "user"), created_at (time, immutable)
  // Edges: edge.From("item", BacklogItem.Type).Ref("status_events").Unique()
  // Index: Fields("item_id", "created_at")
  ```
- Add reverse edge `edge.To("status_events", BacklogStatusEvent.Type)` to `BacklogItem` schema.
- Files: `session/ent/schema/backlog_status_event.go`, `session/ent/schema/backlog_item.go`

##### Task 1.2.2b: Regenerate ent schema (~2 min)
- Run: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
- Commit all generated files under `session/ent/`.
- Files: `session/ent/` (all regenerated files)

##### Task 1.2.2c: Add `AppendStatusEvent` to storage and call it from `TransitionBacklogItemStatus` (~4 min)
- In the storage layer (find the file containing `TransitionBacklogItemStatus` DB implementation), after a successful status update, insert a `BacklogStatusEvent` row.
- Add a `triggered_by` parameter to the storage-level `TransitionBacklogItemStatus` (or a separate `AppendStatusEvent` method).
- Propagate `triggered_by` from service callers: `"user"` from RPC, `"lifecycle"` from `BacklogLifecycleListener`, `"triage"` from MCP tool handler.
- Files: storage file containing `TransitionBacklogItemStatus`, `server/services/backlog_service.go`, `session/backlog_lifecycle.go`

##### Task 1.2.2d: Add proto + RPC for `ListBacklogStatusEvents` (~4 min)
- In `proto/session/v1/backlog.proto`, add:
  ```protobuf
  message BacklogStatusEvent { string item_id = 1; string from_status = 2; string to_status = 3; string triggered_by = 4; google.protobuf.Timestamp created_at = 5; }
  message ListBacklogStatusEventsRequest { string item_id = 1; }
  message ListBacklogStatusEventsResponse { repeated BacklogStatusEvent events = 1; }
  rpc ListBacklogStatusEvents(ListBacklogStatusEventsRequest) returns (ListBacklogStatusEventsResponse);
  ```
- Run `make generate-proto`.
- Implement `ListBacklogStatusEvents` handler in `server/services/backlog_service.go`.
- Files: `proto/session/v1/backlog.proto`, `server/services/backlog_service.go`, `server/server.go` (register if needed)

---

### Epic 1.3: Triage Loop Integration

**Goal**: `submit_triage_result` auto-transitions to `refining` when `clarifying_questions` is non-empty, and the `refining` state participates in `BacklogLifecycleListener`.

#### Story 1.3.1: MCP `submit_triage_result` sets `refining` when questions remain
**As an** AI triage agent, **I want** to set an item to `refining` when I have clarifying questions, **so that** the workflow reflects that human input is needed before the item is ready.
**Acceptance Criteria**:
- When `submit_triage_result` is called with non-empty `clarifying_questions`, the item status transitions `idea → refining`
- When `clarifying_questions` is empty (full triage), the existing `idea → ready` logic is unchanged
- `triggered_by = "triage"` is recorded in the status event

**Files**:
- `server/mcp/tools_backlog.go`
- `server/services/backlog_service.go` (if transition logic is in service layer)

##### Task 1.3.1a: Add `refining` branch in `submit_triage_result` handler (~4 min)
- In `server/mcp/tools_backlog.go` around line 478, after saving the triage result, check `len(clarifyingQuestions) > 0`.
- If true: call `s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusRefining, nil)` with `triggered_by = "triage"`.
- If false (full triage): existing logic transitions to `ready` (unchanged, but now uses `triggered_by = "triage"`).
- Log the transition with `[mcp:submit_triage_result] item=%s → refining (clarifying_questions=%d)`.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.3.1b: Write unit test for `refining` branch in triage result handler (~4 min)
- Add test in `server/mcp/tools_backlog_test.go` (or create it):
  - With `clarifying_questions: ["What is the scope?"]` → item transitions to `refining`
  - With empty `clarifying_questions` → item transitions to `ready` (existing behavior)
- Files: `server/mcp/tools_backlog_test.go`

---

#### Story 1.3.2: `BacklogLifecycleListener` handles `refining` triage exit
**As a** backlog system, **I want** triage sessions exiting from `refining` items to advance the item to `ready`, **so that** the AI clarification loop completes properly.
**Acceptance Criteria**:
- When a triage-role session exits and the item is `refining`, the listener transitions it to `ready` (not `review`)
- The existing `in_progress → review/done` logic is unaffected
- `ReconcileStuckItems` is extended to include `refining` items stuck >24h

**Files**:
- `session/backlog_lifecycle.go`
- Storage file containing `ReconcileStuckItems`

##### Task 1.3.2a: Handle triage session exit for `refining` items (~4 min)
- In `BacklogLifecycleListener.onSessionExited`, expand the role check:
  ```go
  switch is.SessionRole {
  case SessionRoleWork:
      // existing in_progress → review/done logic
  case SessionRoleTriage:
      go l.onTriageSessionExited(sessionUUID, is, item)
  default:
      return
  }
  ```
- Implement `onTriageSessionExited`: if `BacklogStatus(item.Status) == BacklogStatusRefining`, transition to `BacklogStatusReady` with `triggered_by = "lifecycle"`.
- Files: `session/backlog_lifecycle.go`

##### Task 1.3.2b: Extend `ReconcileStuckItems` for `refining` timeout (~4 min)
- Find `ReconcileStuckItems` in the storage/ent layer (likely `session/ent/repository.go` or similar).
- Add a query for items with `status = "refining"` AND `updated_at < now - 24h`.
- Transition those items back to `"idea"` (not `"ready"`) so they re-enter the queue, with `triggered_by = "lifecycle"`.
- The 24h default should be a named constant `RefiningStuckThreshold = 24 * time.Hour` in `session/backlog_lifecycle.go`.
- Files: `session/backlog_lifecycle.go`, storage file containing `ReconcileStuckItems`

---

### Epic 1.4: Frontend `refining` Support

**Goal**: The board and detail view show `refining` items correctly and support the `idea → refining → ready` transitions.

#### Story 1.4.1: Board and detail view support `refining`
**As a** user, **I want** to see `refining` items on the board with the correct label and actions, **so that** I can understand what clarification the AI needs.
**Acceptance Criteria**:
- `refining` column appears on board between `idea` and `ready`
- `refining` items show the clarifying questions from the triage result (read-only display)
- "Mark Ready" action available from `refining` detail view (manual override after answering questions)
- Status badge renders with a distinct color (amber/yellow tone)

**Files**:
- `web-app/src/app/backlog/page.tsx`
- `web-app/src/components/backlog/BacklogItemDetail.tsx`
- `web-app/src/app/backlog/board/page.tsx`
- `web-app/src/styles/theme.css.ts` (if `refining` color token needed)

##### Task 1.4.1a: Add `refining` to board column order and status filter chips (~3 min)
- In `page.tsx`, update `ALL_STATUSES` array: insert `"refining"` after `"idea"`.
- Add entry to `STATUS_LABELS`: `refining: "Refining"`.
- Add entry to `STATUS_CSS` (or vanilla-extract style): use `vars.color.statusWarning` or define a new `refining` token in `theme.css.ts`.
- Files: `web-app/src/app/backlog/page.tsx`

##### Task 1.4.1b: Add "Mark Ready" action in `BacklogItemDetail` for `refining` items (~4 min)
- In `BacklogItemDetail.tsx`, in the action button block, add a case for `item.status === "refining"`:
  ```tsx
  {item.status === "refining" && (
    <button onClick={() => transitionStatus(item.id, "ready", "refining")}>
      Mark Ready
    </button>
  )}
  ```
- Display the `clarifyingQuestions` from `item.triageResult` in a read-only list when `item.status === "refining"`.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.4.1c: Update board page to include `refining` column (~3 min)
- In `web-app/src/app/backlog/board/page.tsx`, ensure `refining` is rendered as a Kanban column in the correct position.
- Files: `web-app/src/app/backlog/board/page.tsx`

##### Task 1.4.1d: Add vanilla-extract style token for `refining` status (~3 min)
- In the CSS layer (`.css.ts` files for backlog status badges), add a `refining` variant using `vars.color.statusWarning` or a new amber token.
- No new `.module.css` files; use existing recipe patterns from ADR-009.
- Files: relevant `.css.ts` files (check `web-app/src/components/backlog/` for status badge component)

---

## Phase 2: WorkflowConfig Persistence + Custom State CRUD

**Goal**: Persist workflow configuration to the database and expose CRUD RPCs + a settings UI for managing custom states.

### Epic 2.1: WorkflowConfig ent Schema

**Goal**: Define the DB entities for a persisted workflow graph.

#### Story 2.1.1: `WorkflowConfig`, `WorkflowState`, `WorkflowTransition` ent entities
**As a** developer, **I want** the workflow graph persisted in the DB, **so that** custom states survive restarts.
**Acceptance Criteria**:
- Three new ent schemas: `WorkflowConfig`, `WorkflowState`, `WorkflowTransition`
- `WorkflowConfig` has a singleton constraint (one per instance, enforced by `name = "default"` unique index)
- `WorkflowState` has: `name` (string, unique within config), `label` (string), `terminal` (bool), `is_default` (bool), `sort_order` (int)
- `WorkflowTransition` has: `from_state` (string), `to_state` (string), `gates_json` (string, JSON `[]Gate`)
- FK edges: `WorkflowConfig → WorkflowState` (has many), `WorkflowConfig → WorkflowTransition` (has many)

**Files**:
- `session/ent/schema/workflow_config.go` (new)
- `session/ent/schema/workflow_state.go` (new)
- `session/ent/schema/workflow_transition.go` (new)
- `session/ent/` (regenerated)

##### Task 2.1.1a: Write ent schemas for WorkflowConfig, WorkflowState, WorkflowTransition (~5 min)
- `workflow_config.go`: fields `name` (unique, default "default"), `created_at`, `updated_at`. Edge: `To("states", WorkflowState.Type)`, `To("transitions", WorkflowTransition.Type)`.
- `workflow_state.go`: fields `name`, `label`, `terminal` (bool, default false), `is_default` (bool, default false), `sort_order` (int, default 0). Edge: `From("config", WorkflowConfig.Type).Ref("states").Unique()`.
- `workflow_transition.go`: fields `from_state`, `to_state`, `gates_json` (optional, JSON). Edge: `From("config", WorkflowConfig.Type).Ref("transitions").Unique()`.
- Files: `session/ent/schema/workflow_config.go`, `session/ent/schema/workflow_state.go`, `session/ent/schema/workflow_transition.go`

##### Task 2.1.1b: Regenerate ent schema and verify build (~2 min)
- Run: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
- Run: `go build ./...` to confirm no compilation errors.
- Files: `session/ent/` (all regenerated)

##### Task 2.1.1c: Seed default WorkflowConfig on first startup (~4 min)
- In the server startup path (find `server/dependencies.go` or `session/storage.go` initialization), add a `SeedDefaultWorkflowConfig(ctx)` call.
- The seed inserts the default six states (`idea`, `refining`, `ready`, `in_progress`, `review`, `done`, `archived`) and all transitions from `DefaultWorkflowEngine` only if `WorkflowConfig` count == 0.
- This is a startup-time seed, NOT a migration — idempotent.
- Files: `server/dependencies.go`, `session/storage.go` or new `session/workflow_config_seed.go`

---

#### Story 2.1.2: `ConfiguredWorkflowEngine` backed by DB
**As a** developer, **I want** a `WorkflowEngine` implementation that loads its graph from the DB, **so that** custom states are respected at runtime.
**Acceptance Criteria**:
- `ConfiguredWorkflowEngine` implements `WorkflowEngine`
- It loads `WorkflowConfig` from DB on construction, caches in memory with a 30s TTL
- `Invalidate()` method for forcing cache refresh after writes
- `BacklogService` can be wired with either `Default` or `Configured` engine (controlled by startup flag)
- Gate validation for built-in gates (AC-required, plan-required) preserved in `ConfiguredWorkflowEngine`

**Files**:
- `session/configured_workflow_engine.go` (new)
- `server/dependencies.go`

##### Task 2.1.2a: Implement `ConfiguredWorkflowEngine` with TTL cache (~5 min)
- Struct fields: `storage *Storage`, `mu sync.RWMutex`, `cached *loadedConfig`, `cachedAt time.Time`, `ttl time.Duration` (default 30s).
- `loadedConfig` holds states and transitions loaded from DB.
- `CanTransition`, `AllowedTransitions` read from cached graph.
- `ValidateGates` checks `gates_json` per transition; for built-in gate types ("ac_required", "plan_required"), delegate to existing guard logic.
- `Invalidate()` zeroes `cachedAt` under write lock.
- Files: `session/configured_workflow_engine.go`

##### Task 2.1.2b: Write unit tests for `ConfiguredWorkflowEngine` cache behavior (~4 min)
- Test: cache miss triggers DB load; subsequent calls within TTL skip DB; `Invalidate()` forces reload.
- Use a stub storage with a call counter.
- Files: `session/configured_workflow_engine_test.go`

---

### Epic 2.2: WorkflowConfig RPCs

**Goal**: Expose CRUD operations for states and transitions via ConnectRPC.

#### Story 2.2.1: Proto and service layer for WorkflowConfig CRUD
**As a** user, **I want** API endpoints to list/add/update/delete workflow states, **so that** the settings UI can manage the custom workflow.
**Acceptance Criteria**:
- Proto messages: `WorkflowStateProto`, `WorkflowTransitionProto`, `GetWorkflowConfigRequest/Response`, `UpsertWorkflowStateRequest/Response`, `DeleteWorkflowStateRequest/Response`
- `GetWorkflowConfig` RPC returns current config (states + transitions)
- `UpsertWorkflowState` creates or updates a state by name
- `DeleteWorkflowState` rejects deletion if items exist in that state without `migration_target_status`
- All RPCs invalidate `ConfiguredWorkflowEngine` cache on success

**Files**:
- `proto/session/v1/backlog.proto` (or new `workflow.proto`)
- `server/services/workflow_service.go` (new) or `server/services/backlog_service.go`
- `server/server.go`

##### Task 2.2.1a: Add proto messages and RPCs for WorkflowConfig (~5 min)
- Add to `proto/session/v1/backlog.proto` (or create `proto/session/v1/workflow.proto`):
  ```protobuf
  message WorkflowStateProto { string name = 1; string label = 2; bool terminal = 3; bool is_default = 4; int32 sort_order = 5; }
  message GetWorkflowConfigRequest {}
  message GetWorkflowConfigResponse { repeated WorkflowStateProto states = 1; repeated WorkflowTransitionProto transitions = 2; }
  message UpsertWorkflowStateRequest { WorkflowStateProto state = 1; }
  message UpsertWorkflowStateResponse { WorkflowStateProto state = 1; }
  message DeleteWorkflowStateRequest { string name = 1; string migration_target_status = 2; }
  message DeleteWorkflowStateResponse {}
  ```
- Run `make generate-proto`.
- Files: `proto/session/v1/backlog.proto` or `proto/session/v1/workflow.proto`

##### Task 2.2.1b: Implement `WorkflowService` handlers (~5 min)
- Create `server/services/workflow_service.go` with `WorkflowService` struct holding `storage *session.Storage` and `engine *session.ConfiguredWorkflowEngine`.
- Implement `GetWorkflowConfig`: load from DB, return proto response.
- Implement `UpsertWorkflowState`: upsert in DB, call `engine.Invalidate()`.
- Implement `DeleteWorkflowState`: check for items with `status = req.name`; if found and `migration_target_status` is empty, return `CodeFailedPrecondition`; else bulk-update item statuses then delete state.
- Files: `server/services/workflow_service.go`

##### Task 2.2.1c: Register `WorkflowService` in server and validate deadlock prevention (~4 min)
- In `server/server.go`, register the new service handler.
- In `UpsertWorkflowState` or a shared `ValidateWorkflowGraph` function, reject graphs where any non-terminal state has no outgoing edges leading to a terminal state (basic reachability check).
- Files: `server/server.go`, `server/services/workflow_service.go`

---

### Epic 2.3: Settings UI — Workflow State Editor

**Goal**: A settings page where users can view and manage the workflow state list (no visual graph builder in Phase 2).

#### Story 2.3.1: `/settings/workflow` page with state list editor
**As a** user, **I want** a settings page that lists all workflow states and lets me add, rename, or delete them, **so that** I can customize the backlog workflow without editing code.
**Acceptance Criteria**:
- Route `/settings/workflow` accessible from the settings nav
- Lists all states in `sort_order` order with name, label, terminal badge
- "Add State" form: name (slug), label (display), terminal toggle
- Inline rename: click label to edit, press Enter or blur to save
- Delete button: shows confirmation modal with migration target dropdown when items exist in that state
- No drag-to-reorder in Phase 2 (sort_order editable via explicit up/down arrows)

**Files**:
- `web-app/src/app/settings/workflow/page.tsx` (new)
- `web-app/src/app/settings/workflow/page.css.ts` (new)
- `web-app/src/lib/hooks/useWorkflowService.ts` (new)
- `web-app/src/app/settings/` (settings nav link addition)

##### Task 2.3.1a: Create `useWorkflowService` hook (~4 min)
- Mirror `useBacklogService` pattern: create ConnectRPC client, expose `getWorkflowConfig`, `upsertWorkflowState`, `deleteWorkflowState`.
- Return typed `WorkflowState[]` (mapped from proto).
- Files: `web-app/src/lib/hooks/useWorkflowService.ts`

##### Task 2.3.1b: Create `/settings/workflow/page.tsx` with state list (~5 min)
- Fetch config on mount via `useWorkflowService`.
- Render ordered list of states with name, label, terminal badge.
- "Add State" form at the bottom: controlled inputs for `name` and `label`, terminal checkbox, submit calls `upsertWorkflowState`.
- Inline label edit: replace label `<span>` with `<input>` on click; blur/Enter calls `upsertWorkflowState`.
- Files: `web-app/src/app/settings/workflow/page.tsx`

##### Task 2.3.1c: Delete flow with migration modal (~4 min)
- Delete button opens a `<dialog>` (not a custom modal, to avoid portal issues per CSS ADR).
- If state has items: show a `<select>` with remaining states as migration targets.
- On confirm: call `deleteWorkflowState({ name, migrationTargetStatus })`.
- Files: `web-app/src/app/settings/workflow/page.tsx`, `web-app/src/app/settings/workflow/page.css.ts`

##### Task 2.3.1d: Add "Workflow" link to settings navigation (~2 min)
- Find the settings nav component (likely in `web-app/src/app/settings/` or a layout file).
- Add a `<Link href="/settings/workflow">Workflow</Link>` entry.
- Files: settings nav/layout file

---

## Phase 3: Gate Types + Visual Builder (Deferred)

**Goal**: Implement field, triage, approval, and command gates. Add the React Flow visual graph editor. Deferred to v3.

### Gate Design: Pre-flight Satisfaction State Model

**All gates use a pre-flight model, NOT synchronous evaluation at transition time.**

The key design insight: a gate like `make test` should behave exactly like a GitHub required check — the transition button is **disabled until the gate is green**, but the gate result must be obtained **independently**, before the user attempts the transition.

```
[ Transition: Ready → In Progress ]
  ✓ Acceptance criteria (3 items)     ✅  ← re-evaluates on page load (cheap)
  ✗ make test                          ❌  [Run]  ← user triggers explicitly
  ✗ CI checks (PR #47)                 🔄  [Refresh]

  [Move to In Progress]  ← disabled until all blocking gates are ✅
```

**How it works:**

1. **Gate evaluation records** are stored per-item, per-transition-edge (JSON column on `WorkflowTransition` or a `gate_evaluations` table keyed by `(item_id, from_state, to_state, gate_id)`).

2. **Gate types by evaluation trigger:**
   | Gate type | When evaluated | How triggered |
   |-----------|----------------|---------------|
   | Field gate | Real-time on page load | Automatic (DB check, instant) |
   | Triage gate | Real-time on page load | Automatic (DB check, instant) |
   | Approval gate | Persisted | Human clicks "Approve" button |
   | Command gate | Persisted | Human clicks "Run" button; runs async, updates record |
   | CI gate | Persisted | Human clicks "Refresh"; polls GitHub API, updates record |
   | CEL condition | Real-time on page load | Automatic (expression eval, instant) |

3. **`TransitionBacklogItemStatus`** reads stored gate evaluation records server-side and rejects the transition if any blocking gate is not in `passed` state. It does NOT run gates itself.

4. **`EvaluateGates` RPC** (client UX) returns the current evaluation state for all gates on a given transition — used to populate the UI pre-flight checklist. Does NOT trigger command/CI gate execution.

5. **`RunCommandGate` RPC** — new, triggers async execution of a command gate for a specific item+transition. Returns immediately; result is stored in `gate_evaluations`. The UI polls or subscribes via event bus for completion.

6. **Gate evaluation expiry**: stored evaluations have a `valid_until` timestamp. Field/triage gates are always re-evaluated (no stored state needed). Approval gates never expire. Command/CI gate results expire after a configurable duration (default: 1 hour for CI, 24 hours for command).

**This eliminates the `202 Accepted + gate_run_id` complexity from `TransitionBacklogItemStatus`** — the transition RPC is always synchronous and fast. Long-running work happens in pre-flight, not at transition time.

---

### Epic 3.1: Gate Types (Deferred)

Gate types deferred to Phase 3:
- **Field gate** (`ac_required`, `plan_required`) — partially implemented in `TransitionGuard`; promote to first-class gate in `gates_json`; real-time evaluation (no stored state)
- **Triage gate** — blocks transition until triage session has run and `clarifying_questions` is empty; real-time evaluation
- **Approval gate** — blocks until human clicks "Approve" in a dedicated approval panel; stored as a `gate_evaluations` record with `approved_by` and `approved_at`
- **CEL condition gate** — custom expression evaluated against item fields using `cel-go`; real-time evaluation (compile at config-save time, evaluate on page load)
- **Command gate** — shell command from a registered allowlist; triggered via `RunCommandGate` RPC; stored evaluation result with expiry; uses `Setpgid: true` + pgid kill on timeout
- **CI gate** — polls GitHub PR check runs; triggered via `RefreshCIGate` RPC; stores check run results with expiry

**Required entities for Phase 3:**
- `gate_evaluations` table (or JSON on `WorkflowTransition`): `(item_id, from_state, to_state, gate_id, status: passed|failed|pending, output, evaluated_at, valid_until, evaluated_by)`
- `RunCommandGate` RPC: `{ item_id, from_state, to_state, gate_id }` → triggers async; returns `{ gate_run_id }`
- `RefreshCIGate` RPC: same shape, polls GitHub and stores result

### Epic 3.2: Visual Workflow Builder (Deferred)

- Install `@xyflow/react` (lazy-loaded via `dynamic(..., { ssr: false })`)
- Route: `/settings/workflow/builder`
- Nodes = states, edges = transitions
- Clicking a transition edge opens a side panel for gate configuration showing the pre-flight checklist design
- Gate configuration panel: type selector, params form (field selector for field gates, command selector for command gates, PR field for CI gates), enforcement toggle (blocking/warning), enabled/disabled toggle
- Export/import as JSON

---

## Cross-Cutting: `EventBacklogItemTransitioned` Event Bus Integration

**Goal**: Publish a backlog item transition event to the internal event bus so subscribers (UI streaming, future automation) can react.

#### Story X.1: Add `EventBacklogItemTransitioned` to event bus
**As a** system, **I want** to publish an event when a backlog item transitions status, **so that** UI clients can update in real-time without polling.
**Acceptance Criteria**:
- `EventBacklogItemTransitioned` type defined in `pkg/events/types.go`
- `BacklogService` injects `*events.EventBus` (optional field; nil = no-op)
- `TransitionBacklogItemStatus` publishes the event on success
- `EventBus` injection wired in `server/dependencies.go`

**Files**:
- `pkg/events/types.go`
- `server/services/backlog_service.go`
- `server/dependencies.go`

##### Task X.1.1a: Add `EventBacklogItemTransitioned` event type and constructor (~3 min)
- In `pkg/events/types.go`, add:
  ```go
  EventBacklogItemTransitioned EventType = "backlog.item.transitioned"
  ```
- Add `BacklogItemID`, `BacklogItemTitle`, `FromStatus`, `ToStatus`, `TriggeredBy` fields to `Event` struct.
- Add `NewBacklogItemTransitionedEvent(itemID, itemTitle, from, to, triggeredBy string) *Event` constructor.
- Files: `pkg/events/types.go`

##### Task X.1.1b: Inject `EventBus` into `BacklogService` and publish on transition (~3 min)
- Add `eventBus *events.EventBus` optional field to `BacklogService`.
- Add `func (s *BacklogService) SetEventBus(bus *events.EventBus)` setter (same pattern as `SetBacklogLifecycleListener` in session_service).
- In `TransitionBacklogItemStatus`, after successful storage transition, call `s.eventBus.Publish(...)` if non-nil.
- Wire in `server/dependencies.go`: call `backlogService.SetEventBus(eventBus)`.
- Files: `server/services/backlog_service.go`, `server/dependencies.go`

---

## Implementation Order Summary

```
P0: 0.1.1a → 0.1.1b → 0.1.1c → 0.1.1d (frontend safety net)
P1: 1.1.1a → 1.1.1b → 1.1.1c → 1.1.1d (WorkflowEngine wiring)
    1.2.1a → 1.2.1b → 1.2.1c → 1.2.1d (refining backend)
    1.2.2a → 1.2.2b → 1.2.2c → 1.2.2d (transition history)
    1.3.1a → 1.3.1b (triage submit_triage_result)
    1.3.2a → 1.3.2b (lifecycle listener + reconcile)
    1.4.1a → 1.4.1b → 1.4.1c → 1.4.1d (frontend board/detail)
    X.1.1a → X.1.1b (event bus integration, can run parallel with 1.4)
P2: 2.1.1a → 2.1.1b → 2.1.1c (ent schema + seed)
    2.1.2a → 2.1.2b (ConfiguredWorkflowEngine)
    2.2.1a → 2.2.1b → 2.2.1c (RPCs)
    2.3.1a → 2.3.1b → 2.3.1c → 2.3.1d (settings UI)
P3: (deferred — gate types, visual builder)
```

---

## ADRs

- **ADR-013**: `docs/adr/013-workflow-engine-replaces-valid-transitions.md` — WorkflowEngine interface replaces `validTransitions` map; `DefaultWorkflowEngine` wraps existing logic; `ConfiguredWorkflowEngine` backed by DB for Phase 2.

---

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| TypeScript closed union breaks when `refining` arrives from server | Phase 0 opens the union before any state is added |
| `WorkflowConfig` loaded per-item in `ListBacklogItems` (N+1) | `ConfiguredWorkflowEngine` has a 30s TTL cache; never load per-item |
| State deletion with live items corrupts item status | `DeleteWorkflowState` requires `migration_target_status` when items exist; enforced at RPC level |
| `refining` items never exit (triage session dies without exiting cleanly) | `ReconcileStuckItems` extended with 24h timeout → `idea` regression |
| Deadlock: custom state graph with no terminal path | `ValidateWorkflowGraph` reachability check in `UpsertWorkflowState` |
| Command/CI gate blocks `TransitionBacklogItemStatus` RPC | Pre-flight model: gates are evaluated and stored before the transition is attempted; `TransitionBacklogItemStatus` only reads stored results — always fast |
| Stale gate evaluation allows transition after test starts failing | Gate evaluations have `valid_until` expiry; expired results are treated as `pending` (not `passed`), blocking the transition until re-run |
| User bypasses pre-flight by calling `TransitionBacklogItemStatus` directly | Server reads `gate_evaluations` records authoritatively; a missing or expired evaluation for a blocking gate is a server-side rejection |
