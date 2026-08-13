# Implementation Plan: Workflow Session Affordances

**Date:** 2026-06-13
**Requirements:** `project_plans/workflow-session-affordances/requirements.md`
**Research:** `project_plans/workflow-session-affordances/research/`

---

## Architecture Decisions

### ADR-1: Per-workflow retention fields use `optional int32` in proto3

**Decision:** Add `keep_sessions` and `archive_after_hours` as `optional int32` to `WorkflowProto`, `CreateWorkflowRequest`, and `UpdateWorkflowRequest`. In ent schema, add as `Optional()` with `Default(0)`. Zero means "disabled" (not "archive everything").

**Rationale:** Proto3 zero-values are indistinguishable from "not set" for scalar types. Using `optional` generates `*int32` in Go, allowing nil-check to detect absence. Existing workflows will get nil (not zero) until they are explicitly set, preventing accidental bulk archive on upgrade.

### ADR-2: Workflow name denormalized on Session proto via `workflow_name` field

**Decision:** Add `string workflow_name = 64` to the `Session` proto (types.proto). Populated in `InstanceToProto` by looking up the workflow name from a `WorkflowRepository` reference held by the adapter, or passed as a pre-built `map[string]string` at query time.

**Rationale:** The alternative (client-side map from `ListWorkflows`) requires a second RPC on every page load and fails if workflows are deleted. Denormalization at write-time (storing workflow name on session creation) is simpler but creates stale data if the workflow is renamed. The pragmatic approach: populate `workflow_name` at read-time in `InstanceToProto` using a cached workflow map maintained by `SessionService`. This avoids per-session DB queries.

**Implementation:** `SessionService` keeps a `workflowNameCache map[string]string` populated by `ListWorkflows` on startup and refreshed whenever a workflow is created/updated/deleted. `InstanceToProto` takes this map and fills `WorkflowName` from it.

### ADR-3: "Suppress workflow sessions by default" is client-side filter

**Decision:** Add `showWorkflowSessions` boolean to `SessionList` state with localStorage key `stapler-squad-show-workflow-sessions` defaulting to `false`. Filter is applied client-side after the session list is received (same as `hidePaused`). **Do not** add server-side filter to `ListSessionsRequest` in v1.

**Rationale:** The server-side `ListSessionsRequest` approach is clean but breaks any current consumer that expects all non-archived sessions. A client-side filter is backward-compatible and follows the existing pattern for `hidePaused`. When grouped by Workflow, the `showWorkflowSessions` filter is bypassed so workflow sessions always appear in their groups.

### ADR-4: Retention sweep is a background goroutine in `server/workflows/retention.go`

**Decision:** New file `server/workflows/retention.go` following the `server/analytics/retention.go` goroutine structure (not parameters, which differ). Registered in `server/server.go` after `workflowScheduler` starts. Sweeps once per hour. The sweep operates directly on the ent client, not through the session poller.

**Critical interaction with `maybeAutoArchive`:** The existing `maybeAutoArchive` fires on `EventExited` and immediately sets `archived_at = now()`. If this runs, the retention enforcer's `AND archived_at IS NULL` predicate will never match. The two features must be mutually exclusive:
- If a workflow has `archive_after_hours > 0`, the `wireAutoArchiveCallback` must **not** register the immediate archive listener for that workflow (or `maybeAutoArchive` must check `archive_after_hours == 0` before archiving).
- If `archive_after_hours == 0`, `maybeAutoArchive` fires immediately as today (backward-compatible).

**Implementation:** `maybeAutoArchive` gains a new guard:
```go
func (s *SessionService) maybeAutoArchive(inst *session.Instance) {
    if inst == nil || inst.WorkflowID == "" {
        return
    }
    // If the workflow has a configured delay, let the retention enforcer handle it.
    if wf, ok := s.workflowMetaCache[inst.WorkflowID]; ok && wf.archiveAfterHours > 0 {
        return
    }
    // ... existing immediate archive logic ...
}
```
This requires `workflowNameCache` to also cache `archiveAfterHours` — use a `workflowMeta` struct instead of `map[string]string`.

**Rationale:** The retention enforcer adds time-delayed archival (archive N hours after completion) and keep-N enforcement. It requires direct DB access because the in-memory poller only holds live instances.

### ADR-5: Bulk archive RPCs added to SessionService (not WorkflowService)

**Decision:** Add two new RPCs to `SessionService`: `ArchiveWorkflowSessions` and `DeleteWorkflowFailedSessions`. Both take `workflow_id` only (no `force_active` flag — active sessions are always skipped).

**Rationale:** Keeps session lifecycle operations in `SessionService`. `WorkflowService` does not own session state. The bulk archive queries the ent DB directly (not the in-memory poller) so it catches all sessions including stopped ones that may not be loaded in memory. Active/Creating/Paused sessions are silently skipped; the RPC returns the count of successfully archived sessions.

### ADR-6: Workflow name cache uses `workflowMeta` struct (not plain string map)

**Decision:** Instead of `workflowNameCache map[string]string`, `SessionService` caches `workflowMetaCache map[string]workflowMeta` where `workflowMeta` holds `{name string; archiveAfterHours int}`. This supports both the `workflow_name` field in `InstanceToProto` and the `maybeAutoArchive` guard. Since `SessionService` does not currently hold a `WorkflowRepository`, the meta cache is populated by calling `workflowSvc.ListWorkflows()` (or adding a `workflowRepo session.WorkflowRepository` dependency to `SessionService`). The latter is simpler; add `workflowRepo` as a new constructor parameter and populate the cache on `Start()`.

### ADR-7: `groupSessions()` receives a `workflowIdToName` map parameter

**Decision:** Add an optional `workflowIdToName?: Map<string, string>` parameter to `groupSessions()`. Used only when `strategy === GroupingStrategy.Workflow`. Falls back to UUID display if map is missing or key is absent.

**Rationale:** The grouping function is pure and needs context it doesn't currently have. Adding an optional parameter maintains backward compatibility (all existing callers pass `undefined` or omit it) and avoids global state. The `SessionList` component passes this map (populated from `useWorkflows()`).

---

## Epic / Story / Task Breakdown

---

## Epic 1: Session List — Visual Identity (FR-1, FR-2)

### Story 1.1: Proto — Add `workflow_name` to Session and retention fields to Workflow

**Tasks:**

#### Task 1.1.1 — Add `workflow_name` to Session proto
- **File:** `proto/session/v1/types.proto`
- **Change:** Add `string workflow_name = 64;` to the `Session` message after `archived_at = 63`
- **Acceptance:** `make generate-proto` succeeds; `Session` generated type has `workflowName: string`

#### Task 1.1.2 — Add `keep_sessions` and `archive_after_hours` to WorkflowProto
- **File:** `proto/session/v1/session.proto`
- **Change:**
  - Add to `WorkflowProto`: `optional int32 keep_sessions = 15;` and `optional int32 archive_after_hours = 16;`
  - Add to `CreateWorkflowRequest`: `optional int32 keep_sessions = 12;` and `optional int32 archive_after_hours = 13;`
  - Add to `UpdateWorkflowRequest`: `optional int32 keep_sessions = 12;` and `optional int32 archive_after_hours = 13;`
- **Acceptance:** `make generate-proto` succeeds; `WorkflowProto` generated type has `keepSessions?: number` and `archiveAfterHours?: number`

#### Task 1.1.3 — Add bulk archive RPCs to SessionService proto
- **File:** `proto/session/v1/session.proto`
- **Change:**
  - Add RPCs (after `ArchiveSession`):
    ```protobuf
    rpc ArchiveWorkflowSessions(ArchiveWorkflowSessionsRequest) returns (ArchiveWorkflowSessionsResponse) {}
    rpc DeleteWorkflowFailedSessions(DeleteWorkflowFailedSessionsRequest) returns (DeleteWorkflowFailedSessionsResponse) {}
    ```
  - Add messages:
    ```protobuf
    message ArchiveWorkflowSessionsRequest {
      string workflow_id = 1;
    }
    message ArchiveWorkflowSessionsResponse {
      int32 archived_count = 1;
    }
    message DeleteWorkflowFailedSessionsRequest {
      string workflow_id = 1;
    }
    message DeleteWorkflowFailedSessionsResponse {
      int32 deleted_count = 1;
    }
    ```
- **Note:** Run `make generate-proto` after all proto tasks (1.1.1–1.1.3) are complete.
- **Acceptance:** Generated Go and TypeScript bindings include the new RPCs and messages.

---

### Story 1.2: Backend — workflow schema + retention fields

#### Task 1.2.1 — Add `keep_sessions` and `archive_after_hours` to ent Workflow schema
- **File:** `session/ent/schema/workflow.go`
- **Change:** Add two new optional integer fields with defaults matching FR-4a and FR-5a/5b:
  ```go
  field.Int("keep_sessions").Optional().Default(1).
      Comment("Keep only the N most recent sessions per workflow (0 = keep all)."),
  field.Int("archive_after_hours").Optional().Default(24).
      Comment("Auto-archive completed sessions after this many hours (0 = disabled)."),
  ```
  **Default is 1 (not 0) for `keep_sessions` and 24 (not 0) for `archive_after_hours`** per FR-4a and FR-5a/5b. Existing workflows in the DB that have NULL for these columns will get the ent Default values (1 and 24) when read back — meaning auto-archive and keep-1 will activate for ALL existing workflows on upgrade. Document this in release notes and in the WorkflowForm field hints.
  
  **Migration consideration:** If activating retention on all existing workflows on upgrade is too aggressive, use `Optional().Default(0)` (disabled for existing workflows) and change the WorkflowForm to show 1/24 as placeholder hints but leave fields empty by default. Decide before implementation and update this task.
- **After change:** Run the correct ent generate command:
  ```bash
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
  ```
- **Acceptance:** `go build ./...` passes; SQLite migration adds the two nullable columns without errors

#### Task 1.2.2 — Add `KeepSessions` and `ArchiveAfterHours` to `WorkflowRepository` and `EntWorkflowRepository`
- **Files:** `session/workflow_repository.go`, `session/ent_workflow_repository.go`
- **Change in `workflow_repository.go`:** Update `WorkflowCreateInput` and `WorkflowUpdateInput` structs (the actual types — **not** a phantom `WorkflowData` struct) to add `KeepSessions *int` and `ArchiveAfterHours *int` as optional pointer fields, following the existing pointer-field pattern in `WorkflowUpdateInput`
- **Change in `ent_workflow_repository.go`:** Update all Create/Update/Get/List operations to read and write the new fields from/to ent
- **Acceptance:** `go test ./session/...` passes; new fields round-trip through the repository

#### Task 1.2.3 — Populate `workflow_name` in `InstanceToProto` (adapter)
- **File:** `server/adapters/instance_adapter.go`
- **Change:** 
  - Add `workflowNames map[string]string` parameter to `InstanceToProto`
  - Set `WorkflowName: workflowNames[inst.WorkflowID]` (empty string if not found — fine for non-workflow sessions)
- **Critical: there are 13 call sites in `session_service.go`** (lines approximately 748, 774, 804, 834, 841, 858, 1069, 1339, 1395, 1440, 2195, 2252, 2557). Every single caller must be updated — use grep to find all: `grep -n "InstanceToProto" server/services/session_service.go`. Also update `instance_adapter_test.go`.
- **Supporting change in `session_service.go`:** 
  - Add `workflowRepo session.WorkflowRepository` to `SessionService` via the **deferred-setter pattern** (following the existing `SetWorkflowService` pattern): add `func (s *SessionService) SetWorkflowRepository(repo session.WorkflowRepository)`. Call this setter in `server/dependencies.go` after constructing both `SessionService` and `WorkflowRepository`. This avoids changing the `NewSessionService` constructor signature (which has many test call sites).
  - Add `workflowMetaCache map[string]workflowMeta` + `workflowMetaMu sync.RWMutex` where `workflowMeta = struct{ name string; archiveAfterHours int }`
  - Populate cache on startup (after `workflowRepo` is available) and refresh after any workflow mutation (requires `WorkflowService` to call back into `SessionService`, or more simply, refresh the cache lazily on each `InstanceToProto` call with a TTL)
  - Simplest approach: refresh cache once per minute via a goroutine started in `SessionService.Start()`
  - Pass `s.workflowNames()` (a method that returns `map[string]string` from the cache) to all `InstanceToProto` calls
- **Guard in `maybeAutoArchive`:** Use `workflowMetaCache` to check `archiveAfterHours > 0` before immediately archiving (see ADR-4)
- **Acceptance:** `ListSessions` response includes `workflowName` for workflow sessions; `go test ./server/...` passes

#### Task 1.2.4 — Update `entWorkflowToProto` to include retention fields
- **File:** `server/services/workflow_service.go`
- **Change:** Update `entWorkflowToProto()` to set `KeepSessions` and `ArchiveAfterHours` from ent fields (using pointer arithmetic for optional values)
- **Change:** Update `CreateWorkflow` and `UpdateWorkflow` handlers to read and persist the new fields
- **Acceptance:** Create/update/list workflow RPCs include retention fields; `go test ./server/services/...` passes

---

### Story 1.3: Backend — retention sweep goroutine

#### Task 1.3.1 — Create `server/workflows/retention.go`
- **File:** `server/workflows/retention.go` (new file)
- **Pattern:** Mirror `server/analytics/retention.go`
- **Content:**
  ```go
  package workflows

  // StartRetentionEnforcer starts a background goroutine that periodically:
  // 1. Archives completed workflow sessions older than archive_after_hours
  // 2. Archives excess sessions beyond keep_sessions count (oldest first)
  //
  // Guards:
  // - Never archives sessions with status Active, Creating, or Paused
  // - archive_after_hours == 0 means disabled (skip that workflow)
  // - keep_sessions == 0 means disabled (keep all)
  func StartRetentionEnforcer(
      ctx context.Context,
      entClient *ent.Client,
      workflowRepo session.WorkflowRepository,
      interval time.Duration,
  )
  ```
- **`runRetention(ctx, entClient, workflowRepo)` logic:**
  1. Load all workflows from `workflowRepo.ListAll(ctx)`
  2. For each workflow with `ArchiveAfterHours > 0`:
     - Because `maybeAutoArchive` is suppressed for these workflows (see ADR-4), sessions will have `archived_at IS NULL` after they stop.
     - Query `session WHERE workflow_id = wf.ID AND archived_at IS NULL AND status NOT IN (Active, Creating, Paused) AND updated_at < now() - archive_after_hours hours`
     - Set `archived_at = now()` on matching sessions via ent bulk update
  3. For each workflow with `KeepSessions > 0`:
     - Count non-archived non-live sessions WHERE `workflow_id = wf.ID AND archived_at IS NULL AND status NOT IN (Active, Creating, Paused)` ORDER BY `created_at DESC`
     - If count > KeepSessions: archive the oldest `count - KeepSessions` sessions
- **Status guard — CRITICAL:** The ent `session.Status` field stores Go-layer integer values, **not** proto wire values. The correct mapping is: `Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4`. The proto wire values (`SESSION_STATUS_ACTIVE=1, SESSION_STATUS_CREATING=6, SESSION_STATUS_PAUSED=4`) are **different** and must NOT be used here. Use the Go ent constants: `session.StatusIn(session.Active, session.Creating, session.Paused)` where these are the typed status constants from the generated ent code, or use the integer literals 0, 1, 2. Failure to use the correct constants would cause the predicate to exclude `Hibernated=4` (mistaken as PAUSED) while failing to exclude `Paused=2` — which would archive sessions with live git worktrees.
- **Acceptance:** Unit test in `server/workflows/retention_test.go` verifies:
  - Active sessions are not archived
  - Sessions older than archive_after_hours are archived
  - Only the N newest are kept when keep_sessions is set
  - Zero values skip the workflow

#### Task 1.3.2 — Register retention enforcer in `server/server.go`
- **File:** `server/server.go`
- **Change:** After `workflowScheduler.Start(ctx)`, add:
  ```go
  entClient := deps.Storage.GetEntClient() // same pattern used elsewhere
  if entClient != nil {
      workflows.StartRetentionEnforcer(serverCtx, entClient, deps.WorkflowRepository, time.Hour)
  }
  ```
  - `server.go` does not currently import the `workflows` package. Add: `"github.com/tstapler/stapler-squad/server/workflows"` to the import block.
  - Use `deps.Storage.GetEntClient()` — check the exact method name by looking at how the analytics enforcer accesses the ent client in `server.go`; it uses `deps.AnalyticsEntClient` (a separate field). For the session ent client, look for `GetEntClient()` on the storage interface or use a similar dedicated field. **Before implementing, verify the correct accessor by checking `server/dependencies.go` and `RuntimeDeps` struct.**
  - The nil guard prevents panics when running without a persistent ent backend.
- **Acceptance:** App starts without error; log line emitted on startup cycle

---

### Story 1.4: Backend — bulk archive RPCs

#### Task 1.4.1 — Implement `ArchiveWorkflowSessions` RPC
- **File:** `server/services/session_service.go`
- **Change:** Implement `ArchiveWorkflowSessions`:
  1. Validate `workflow_id` non-empty
  2. **Query the ent DB via a new repository method** (not the in-memory poller — the poller misses stopped sessions that may have been removed from memory; and `SessionService` has no `entClient` field directly):
     - Add `ListByWorkflowIDNonArchived(ctx context.Context, workflowID string) ([]InstanceData, error)` to `EntRepository` in `session/ent_repository.go`
     - The implementation queries:
       ```go
       client.Session.Query().
           Where(
               session.WorkflowID(workflowID),
               session.ArchivedAtIsNil(),
               session.StatusNotIn(session.Active, session.Creating, session.Paused),
           ).
           All(ctx)
       ```
       where `session.Active`, `session.Creating`, `session.Paused` are the **Go-layer ent constants** (values 1, 0, 2 respectively) — not proto wire values
     - `SessionService` calls this method via `s.storage.ListByWorkflowIDNonArchived(...)` (where `s.storage` is the `SessionRepository` that has ent access)
  3. Set `archived_at = now()` on each row; also update in-memory instance if it exists in the poller
  4. Return count of archived sessions
- **Register in:** `server/server.go` where service handler is registered (same pattern as other RPCs)
- **Acceptance:** RPC archives all stopped/hibernated sessions for a workflow (including sessions not in memory); active sessions are skipped; returns accurate count

#### Task 1.4.2 — Implement `DeleteWorkflowFailedSessions` RPC
- **File:** `server/services/session_service.go`
- **Change:** Implement `DeleteWorkflowFailedSessions`:
  - "Failed" sessions definition (chosen for reliability): sessions with `workflow_id = X AND status = Stopped AND archived_at IS NULL AND last_meaningful_output IS NULL`
  - Rationale: `initial_prompt` is set by the session driver at Ready state — sessions that never reached Ready will have `initial_prompt = ""`, making `initial_prompt != ""` exclude the most common failure mode. `last_meaningful_output IS NULL` is a better heuristic for sessions that exited without doing useful work.
  - This archives (soft-deletes via `archived_at = now()`), **not** hard-deletes, despite the RPC name. Update the proto response message comment to say "archived" not "deleted" to avoid ambiguity.
  - Query: `session WHERE workflow_id = X AND status = Stopped AND archived_at IS NULL AND last_meaningful_output IS NULL`
- **Acceptance:** RPC archives only sessions with no meaningful output; active/paused sessions skipped; count returned accurately

---

## Epic 2: Session List Frontend (FR-1, FR-2)

### Story 2.1: Session Card — workflow badge

#### Task 2.1.1 — Add workflow badge to `SessionCard`
- **File:** `web-app/src/components/sessions/SessionCard.tsx`
- **Change:**
  - After the `autonomousBadge` span (last in badge row), add:
    ```tsx
    {session.workflowId && (
      <span className={workflowBadge} title={session.workflowName || session.workflowId}>
        ⚙ {session.workflowName || "Workflow"}
      </span>
    )}
    ```
  - Or use a Lucide `Workflow` or `Repeat` icon instead of the ⚙ character
- **File:** `web-app/src/components/sessions/SessionCard.css.ts`
- **Change:** Add `workflowBadge` style using `vars.color.*` tokens:
  ```ts
  export const workflowBadge = style({
    fontSize: vars.fontSize.xs,
    padding: `2px ${vars.space[2]}`,
    borderRadius: vars.radii.sm,
    background: vars.color.accentBg,
    color: vars.color.textSecondary,
    border: `1px solid ${vars.color.borderColor}`,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    maxWidth: '120px',
  });
  ```
- **Acceptance:** Session card for a workflow session shows the workflow name badge; manual sessions show nothing extra

---

### Story 2.1b: Session List — Filter by workflow (FR-1d)

#### Task 2.1b.1 — Add workflow filter dropdown to `SessionList`
- **File:** `web-app/src/components/sessions/SessionList.tsx`
- **Change:**
  - Add `useWorkflows()` call (or reuse the call added in Task 2.2.3) to get the `workflows` list
  - Add state: `const [selectedWorkflow, setSelectedWorkflow] = useState<string | "all">(() => loadFromStorage(STORAGE_KEYS.SELECTED_WORKFLOW, "all"))`
  - Add to `BASE_STORAGE_KEYS`: `SELECTED_WORKFLOW: 'stapler-squad-selected-workflow'`
  - Add to filter pipeline: if `selectedWorkflow !== "all"`, filter to sessions where `session.workflowId === selectedWorkflow`
  - Add `<select>` dropdown in the filter bar (similar to `selectedCategory` dropdown):
    ```tsx
    <select value={selectedWorkflow} onChange={(e) => setSelectedWorkflow(e.target.value)}>
      <option value="all">All workflows</option>
      {workflows.map(wf => (
        <option key={wf.id} value={wf.id}>{wf.name}</option>
      ))}
    </select>
    ```
  - Note: the server-side `workflow_id` filter in `ListSessionsRequest` (field 7) already exists — use it as an alternative if a server-side filter is preferred; for v1, client-side filter is simpler and consistent with the existing pattern for `selectedCategory`
- **Acceptance:** Session list shows a "Workflow" filter dropdown; selecting a workflow shows only sessions from that workflow (FR-1d)

---

### Story 2.2: Session List — `showWorkflowSessions` filter and Workflow grouping

#### Task 2.2.1 — Add `showWorkflowSessions` filter to `SessionList`
- **File:** `web-app/src/components/sessions/SessionList.tsx`
- **Change:**
  - Add to `BASE_STORAGE_KEYS`: `SHOW_WORKFLOW_SESSIONS: 'stapler-squad-show-workflow-sessions'`
  - Add state: `const [showWorkflowSessions, setShowWorkflowSessions] = useState(() => loadFromStorage(STORAGE_KEYS.SHOW_WORKFLOW_SESSIONS, false))`
  - Add to filter pipeline (after `hidePaused` check):
    ```ts
    if (!showWorkflowSessions && session.workflowId && groupingStrategy !== GroupingStrategy.Workflow) {
      return false;
    }
    ```
    — bypassed when grouped by Workflow
  - Add toggle UI in the filter bar (similar to `hidePaused` checkbox):
    ```tsx
    <label>
      <input type="checkbox" checked={showWorkflowSessions} onChange={(e) => setShowWorkflowSessions(e.target.checked)} />
      Show workflow sessions
    </label>
    ```
- **Acceptance:** Workflow sessions hidden by default; toggle makes them visible; grouping by Workflow overrides the filter

#### Task 2.2.2 — Add `Workflow` grouping strategy
- **File:** `web-app/src/lib/grouping/strategies.ts`
- **Change:**
  - Add to `GroupingStrategy` enum: `Workflow = "workflow"`
  - Add to `GroupingStrategyLabels`: `[GroupingStrategy.Workflow]: "Workflow"`
  - Update `groupSessions()` signature: add `options?: { workflowIdToName?: Map<string, string> }` parameter
  - Add case in the switch:
    ```ts
    case GroupingStrategy.Workflow:
      groupKey = session.workflowId
        ? (options?.workflowIdToName?.get(session.workflowId) ?? session.workflowId)
        : "Manual Sessions";
      groupKeys = [groupKey];
      break;
    ```
  - Add `"Manual Sessions"` to the `specialGroups` set so it sorts last
- **File:** `web-app/src/lib/grouping/strategies.test.ts`
- **Change:** Add test cases for `GroupingStrategy.Workflow`:
  - Sessions with `workflowId` group into named workflow groups
  - Sessions without `workflowId` go into "Manual Sessions" group
  - "Manual Sessions" sorts last
- **Acceptance:** `npx jest --testPathPatterns="strategies.test"` passes with new tests

#### Task 2.2.3 — Pass `workflowIdToName` map from `SessionList` to `groupSessions()`
- **File:** `web-app/src/components/sessions/SessionList.tsx`
- **Change:**
  - Import and call `useWorkflows()` at top of `SessionList` component to get `workflows`
  - Build `workflowIdToName: Map<string, string>` via `useMemo`: `new Map(workflows.map(w => [w.id, w.name]))`
  - Pass to `groupSessions(sortedSessions, groupingStrategy, { workflowIdToName })` call
- **Note:** `useWorkflows()` inside `SessionList` creates a separate `ListWorkflows` poller per `SessionList` instance. In split-pane mode, two `SessionList` instances = two pollers. To avoid this, wrap the workflow list in a `WorkflowsContext` (similar to `ReviewQueueContext`) provided at the page level, and use `useWorkflowsContext()` inside `SessionList` instead of `useWorkflows()` directly. This is the recommended implementation. The context would call `ListWorkflows` once and distribute to all consumers.
- **Acceptance:** Grouping by Workflow displays workflow names (not UUIDs) as group headers

---

## Epic 3: Session Detail Panel (FR-3)

### Story 3.1: Show workflow metadata in `SessionDetailView`

#### Task 3.1.1 — Add workflow info section to `SessionDetailView`
- **File:** `web-app/src/components/sessions/SessionDetailView.tsx`
- **Change:** In the Info tab (or at the top of the detail panel where `initialPrompt` is already shown), add a "Workflow" section when `session.workflowId` is set:
  ```tsx
  {session.workflowId && (
    <section className={styles.workflowSection}>
      <h4>Workflow</h4>
      <p><strong>Name:</strong> {session.workflowName || session.workflowId}</p>
      {workflowDetails?.description && (
        <p><strong>Description:</strong> {workflowDetails.description}</p>
      )}
      {workflowDetails?.cronExpression && (
        <p><strong>Schedule:</strong> {workflowDetails.cronExpression}</p>
      )}
      <p><strong>Fired at:</strong> {formatTimestamp(session.createdAt)}</p>
      <Link href={`/workflows?id=${session.workflowId}`}>Open workflow configuration</Link>
    </section>
  )}
  ```
- **Fetching workflow details:** Use a small `useWorkflowById(id)` hook (or reuse `useWorkflows()` and filter) to get the full `WorkflowProto` for the workflow that created this session. Cache via `useMemo`. If `session.workflowName` alone is sufficient for initial display, lazy-load the full details.
- **File:** Add CSS to `SessionDetail.css.ts` (**not** `SessionDetailView.css.ts` — `SessionDetailView.tsx` imports from `"./SessionDetail.css"` which resolves to `SessionDetail.css.ts`):
  - Add `workflowSection` style using `vars` tokens
- **Acceptance:** Opening a workflow session detail shows workflow name, description, schedule, and a link to the workflow page; non-workflow sessions show nothing extra (FR-3a, FR-3c, FR-3d)

#### Task 3.1.2 — Verify `initial_prompt` display (FR-3b)
- **Investigation:** Confirm `initial_prompt` is already shown in `SessionDetailView` at the known location (line ~1150 per research)
- **File:** `web-app/src/components/sessions/SessionDetailView.tsx`
- **Change (if needed):** Ensure the "Terminal Prompt" / `initialPrompt` display is visible and co-located with the new workflow section for workflow sessions
- **Acceptance:** Workflow session detail shows the injected command/prompt alongside workflow name

---

## Epic 4: WorkflowForm — Retention Settings (FR-5)

### Story 4.1: Add retention fields to WorkflowForm

#### Task 4.1.1 — Update `WorkflowFormData` to include retention fields
- **File:** `web-app/src/lib/hooks/useWorkflows.ts`
- **Change:**
  ```ts
  export interface WorkflowFormData {
    // ... existing fields ...
    keepSessions?: number;      // 0 or undefined = disabled
    archiveAfterHours?: number; // 0 or undefined = disabled
  }
  ```
- **Update `createWorkflow` and `updateWorkflow`** to pass new fields to `CreateWorkflowRequest` and `UpdateWorkflowRequest`

#### Task 4.1.2 — Render retention fields in `WorkflowForm`
- **File:** `web-app/src/components/workflows/WorkflowForm.tsx`
- **Change:** Add two new optional number inputs after the cron fields:
  ```tsx
  <label>
    Keep sessions (0 = keep all)
    <input type="number" min={0} value={formData.keepSessions ?? 0}
      onChange={(e) => setFormData({ ...formData, keepSessions: Number(e.target.value) })} />
  </label>
  <label>
    Archive after (hours, 0 = disabled)
    <input type="number" min={0} value={formData.archiveAfterHours ?? 0}
      onChange={(e) => setFormData({ ...formData, archiveAfterHours: Number(e.target.value) })} />
  </label>
  ```
- **Acceptance:** Workflow form shows retention fields; saving persists them; `go test ./server/services/...` passes; Jest tests pass

---

## Epic 5: Workflow Page — Bulk Actions (FR-6)

### Story 5.1: Add bulk archive actions to `WorkflowsPanel`

#### Task 5.1.1 — Add `ArchiveWorkflowSessions` and `DeleteWorkflowFailedSessions` calls to `useWorkflows`
- **File:** `web-app/src/lib/hooks/useWorkflows.ts`
- **Change:** Add two new functions to `UseWorkflowsReturn`:
  ```ts
  archiveWorkflowSessions: (workflowId: string) => Promise<{ archivedCount: number }>;
  deleteWorkflowFailedSessions: (workflowId: string) => Promise<{ deletedCount: number }>;
  ```
- Both call the corresponding ConnectRPC methods via the session service client

#### Task 5.1.2 — Add bulk action buttons to `WorkflowsPanel`
- **File:** `web-app/src/components/workflows/WorkflowsPanel.tsx`
- **Change:** In the `RecentRuns` section (or below it), add two action buttons per workflow:
  - "Archive all sessions" button:
    - On click: show confirmation with session count (fetch count from `listSessionsByWorkflow`)
    - On confirm: call `archiveWorkflowSessions(wf.id)`, show success toast with count
  - "Delete failed runs" button:
    - On click: show confirmation
    - On confirm: call `deleteWorkflowFailedSessions(wf.id)`, show success toast with count
- **Session count for confirmation:** Reuse `listSessionsByWorkflow` (already exists in `useSessionService.ts`) to get and display `"Archive N sessions?"` before the user confirms
- **Acceptance:** Buttons appear on workflow panel; clicking archive shows count in confirmation; archived sessions disappear from session list (FR-6a, FR-6b, FR-6c)

---

## Migration / Rollout Notes

### Schema migration (additive, safe)
- Ent SQLite `ALTER TABLE ADD COLUMN` for `keep_sessions` and `archive_after_hours` on the `workflows` table — fully backward compatible; existing rows get NULL which maps to Go `nil`, not zero
- No sessions table migration needed (all required columns exist)

### Proto field numbers
- `Session.workflow_name = 64` — new field, backward compatible (zero-value is empty string for non-workflow sessions)
- `WorkflowProto.keep_sessions = 15`, `archive_after_hours = 16` — new optional fields, zero-value compatible
- `CreateWorkflowRequest` / `UpdateWorkflowRequest` additions at fields 12 and 13 — new optional, backward compatible

### Frontend storage migration
- New localStorage key `stapler-squad-show-workflow-sessions` defaults to `false` — existing users with no key in storage will correctly get the "hide workflow sessions" behavior on upgrade (intended)

### Workflow name cache
- The `workflowNameCache` in `SessionService` is populated on startup via `workflowRepo.ListAll()`. If a workflow is renamed, the cache must be invalidated. Simplest approach: invalidate and re-populate on every `CreateWorkflow`, `UpdateWorkflow`, and `DeleteWorkflow` RPC. Since these are low-frequency operations, this is acceptable.

### Default retention values
- `keep_sessions = 0` (nil in DB) → enforcer skips the workflow (safe for existing workflows)
- `archive_after_hours = 0` (nil in DB) → enforcer skips time-based archival (safe for existing workflows)
- **Breaking change risk:** When users set `keep_sessions = 1`, the retention enforcer will archive all but the newest session on first sweep. This is expected behavior but should be documented in the WorkflowForm with a clear "0 = unlimited" hint.

### Retention enforcer status guard
- The enforcer uses ent predicates to exclude sessions by **DB/Go-layer status values** (NOT proto wire values): `Active=1, Creating=0, Paused=2`. Proto wire values (`SESSION_STATUS_ACTIVE=1, SESSION_STATUS_CREATING=6, SESSION_STATUS_PAUSED=4`) are DIFFERENT and must not be used in ent queries.
- Hibernated sessions (DB value=4) are safe to archive (no live process, worktree cleanup TBD — follow existing behavior)
- The enforcer reads status from the DB (not the in-memory poller) — so it reflects persisted state after session exit

---

## Test Plan

### Go tests
| File | Test | Covers |
|---|---|---|
| `server/workflows/retention_test.go` | `TestRetentionEnforcer_SkipsActiveSessions` | ADR-4 safety guard |
| `server/workflows/retention_test.go` | `TestRetentionEnforcer_ArchivesExpiredSessions` | `archive_after_hours` logic |
| `server/workflows/retention_test.go` | `TestRetentionEnforcer_KeepsNMostRecent` | `keep_sessions` logic |
| `server/workflows/retention_test.go` | `TestRetentionEnforcer_DisabledWhenZero` | zero = disabled |
| `server/services/session_service_test.go` | `TestArchiveWorkflowSessions_SkipsActive` | Bulk archive safety |
| `server/services/workflow_service_test.go` | `TestCreateWorkflow_WithRetentionFields` | Retention fields round-trip |

### Frontend tests (Jest)
| File | Test | Covers |
|---|---|---|
| `web-app/src/lib/grouping/strategies.test.ts` | `Workflow_strategy_groups_by_workflow_name` | FR-1c |
| `web-app/src/lib/grouping/strategies.test.ts` | `Workflow_strategy_puts_manual_sessions_last` | FR-1c fallback |
| `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `hides_workflow_sessions_by_default` | FR-2a |
| `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `shows_workflow_sessions_when_toggle_on` | FR-2b |
| `web-app/src/components/sessions/__tests__/SessionCard.test.tsx` | `shows_workflow_badge_when_workflowId_set` | FR-1a |

### Manual acceptance test
1. Create a workflow with `keep_sessions=1`, `archive_after_hours=1`, and cron enabled
2. Fire the workflow twice manually
3. Verify second session appears with workflow badge in session list
4. Verify session list hides workflow sessions by default (toggle to reveal)
5. Verify session detail shows workflow name, schedule, and link to workflow page
6. Use "Archive all sessions" on workflow page — verify confirmation count and archival
7. Set system clock +2h; verify retention sweeper archives the older session (or trigger manually via test)
8. Verify zero regressions: existing non-workflow sessions unaffected

---

## File Change Summary

| File | Change Type |
|---|---|
| `proto/session/v1/types.proto` | Add `workflow_name = 64` to Session |
| `proto/session/v1/session.proto` | Add retention fields to WorkflowProto/Request; add bulk archive RPCs |
| `session/ent/schema/workflow.go` | Add `keep_sessions`, `archive_after_hours` fields |
| `session/ent/` (generated) | Run ent codegen |
| `gen/proto/go/session/v1/` (generated) | Run `make generate-proto` |
| `web-app/src/gen/` (generated) | Run `make generate-proto` |
| `session/workflow_repository.go` | Add retention fields to data structs |
| `session/ent_workflow_repository.go` | Read/write new retention fields |
| `session/ent_repository.go` | Add `ListByWorkflowIDNonArchived` method |
| `server/adapters/instance_adapter.go` | Add `workflowNames map[string]string` param to `InstanceToProto` |
| `server/services/session_service.go` | Add `workflowMetaCache`; `SetWorkflowRepository` setter; bulk archive RPCs; update all 13 `InstanceToProto` call sites |
| `server/services/workflow_service.go` | Update `entWorkflowToProto`; handle retention fields in create/update; invalidate name cache |
| `server/workflows/retention.go` | New file — retention enforcer |
| `server/workflows/retention_test.go` | New file — retention tests |
| `server/server.go` | Register retention enforcer |
| `web-app/src/lib/grouping/strategies.ts` | Add `Workflow` strategy; update `groupSessions()` signature |
| `web-app/src/lib/grouping/strategies.test.ts` | Add Workflow strategy tests |
| `web-app/src/components/sessions/SessionCard.tsx` | Add workflow badge |
| `web-app/src/components/sessions/SessionCard.css.ts` | Add `workflowBadge` style |
| `web-app/src/components/sessions/SessionList.tsx` | Add `showWorkflowSessions` filter; workflow filter dropdown (FR-1d); pass `workflowIdToName`; use `WorkflowsContext` |
| `web-app/src/lib/contexts/WorkflowsContext.tsx` | New file — context providing workflow list to avoid per-instance polling |
| `web-app/src/components/sessions/SessionDetailView.tsx` | Add workflow section |
| `web-app/src/styles/SessionDetail.css.ts` | Add `workflowSection` style (note: SessionDetailView imports from `SessionDetail.css.ts`) |
| `web-app/src/components/workflows/WorkflowForm.tsx` | Add retention input fields |
| `web-app/src/components/workflows/WorkflowsPanel.tsx` | Add bulk archive buttons and confirmation |
| `web-app/src/lib/hooks/useWorkflows.ts` | Add retention fields; add bulk archive functions |

---

## Risk Register

| Risk | Mitigation |
|---|---|
| `maybeAutoArchive` defeats `archive_after_hours` delay | `maybeAutoArchive` checks `workflowMetaCache[WorkflowID].archiveAfterHours > 0`; if so, skips immediate archive (ADR-4) |
| Status constant mismatch (proto vs ent DB values) | Retention enforcer uses Go-layer ent constants (Active=1, Creating=0, Paused=2), not proto wire values; explicitly documented in Task 1.3.1 |
| `WorkflowData` phantom struct | Tasks use actual types `WorkflowCreateInput` / `WorkflowUpdateInput` (corrected) |
| FR-1d (workflow filter) missing | Added as Story 2.1b with Task 2.1b.1 |
| Retention sweep archives active session (race) | Status guard in ent query uses Go-layer ent status constants |
| `keep_sessions=0` treated as "archive all" | 0 means "keep all" (disabled); default is 1 per FR-5a |
| Workflow meta cache stale after rename | Cache refreshed by background goroutine in `SessionService` every minute |
| Bulk archive loop races with new cron fire | Acceptable; archive only sessions existing at time of click |
| `groupSessions()` signature change breaks tests | Optional parameter; all existing callers unaffected (TypeScript optional) |
| Duplicate `ListWorkflows` polling in split-pane | Mitigated by `WorkflowsContext` (see Task 2.2.3) |
| `InstanceToProto` signature change (13 call sites) | All 13 callers in `session_service.go` must be updated; use grep to find all; included in Task 1.2.3 scope |
| `strategies.test.ts` misses new strategy | Explicit test task in Story 2.2 |
| `instance_adapter_test.go` breaks on new param | Update fixture in same PR as adapter change |
| Retention enforcer DB writes bypass in-memory state | Direct DB writes do not fire `WatchSessions` events; sessions archived by the enforcer disappear from UI only on next poll/reconnect (acceptable UX gap for a background sweeper) |
