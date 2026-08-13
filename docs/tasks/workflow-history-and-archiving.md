# Workflow History & Session Archiving

## Epic Overview

**Problem**: Users have no way to manually trigger a workflow from the UI, no visibility into past executions, and completed workflow sessions accumulate indefinitely — polluting the session list and burdening features that iterate over all sessions (review queue, analytics, indexing).

**User value**: Trigger workflows with one click, see recent runs at a glance, and keep the session list clean without losing history.

**Success metrics**:
- Run button triggers workflow and navigates to session within 2s
- Recent runs per workflow visible without leaving the Workflows page
- Session list excludes archived sessions by default; filter available to view them
- Workflow sessions auto-archive on completion (opt-in default)

**Scope — In:**
- `workflow_id` FK on Session to link sessions back to the workflow that spawned them
- `archived_at` timestamp on Session for soft-archiving
- Run ▶ button in WorkflowsPanel → navigates to created session
- Recent Runs accordion per workflow row showing last 5 sessions
- ArchiveSession / UnarchiveSession RPCs
- Auto-archive workflow sessions on Stopped status
- "Show archived" filter in session list

**Scope — Out:**
- Workflow run notifications / webhooks
- Bulk archive operations
- Archived session TTL / hard-delete
- Workflow run metrics / analytics

---

## Architecture Decisions

| ADR | Decision |
|-----|----------|
| [ADR-001](../project_plans/workflow-history-and-archiving/decisions/ADR-001-workflow-id-field.md) | Store `workflow_id` as plain optional UUID string on Session (not ent edge) — orphan-safe when workflow is deleted |
| [ADR-002](../project_plans/workflow-history-and-archiving/decisions/ADR-002-archived-at-field.md) | Use `archived_at` nillable timestamp instead of boolean — enables "archived before X" queries and audit trail |
| [ADR-003](../project_plans/workflow-history-and-archiving/decisions/ADR-003-auto-archive-trigger.md) | Auto-archive via lifecycle hook in SessionService.updateSessionStatus — single choke-point for all status transitions |

---

## Story Breakdown

### Story 1: Session↔Workflow linkage [2–3 days]

Enables sessions to know which workflow created them. Foundation for run history.

**Acceptance criteria:**
- Sessions created by `FireNow` have `workflow_id` set
- `ListSessions` accepts `workflow_id` filter
- Existing sessions unaffected (field is optional/nullable)

#### Task 1.1 — Session ent schema: add `workflow_id` + `archived_at` [1h]

**Files:** `session/ent/schema/session.go`

Add two optional fields:
```go
field.String("workflow_id").Optional().Comment("UUID of the Workflow that spawned this session, if any."),
field.Time("archived_at").Optional().Nillable().Comment("Set when the session is archived; nil = not archived."),
```
Add indexes:
```go
index.Fields("workflow_id"),
index.Fields("archived_at"),
```
Then run: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`

**Done when:** `go build ./...` passes; `session/ent/session.go` has `WorkflowID` and `ArchivedAt` fields.

---

#### Task 1.2 — Proto: `workflow_id` in CreateSessionRequest + Session; Archive RPCs [1h]

**Files:** `proto/session/v1/session.proto`

Changes:
1. Add `string workflow_id = 23;` to `CreateSessionRequest`
2. Add `string workflow_id = <next>;` to `Session` message
3. Add `google.protobuf.Timestamp archived_at = <next>;` to `Session` message
4. Add `string workflow_id = <next>;` filter to `ListSessionsRequest`
5. Add `bool include_archived = <next>;` to `ListSessionsRequest`
6. Add RPCs:
```protobuf
rpc ArchiveSession(ArchiveSessionRequest) returns (ArchiveSessionResponse) {}
rpc UnarchiveSession(UnarchiveSessionRequest) returns (UnarchiveSessionResponse) {}
```
7. Add message definitions for those RPCs.

Run `make proto-gen`.

**Done when:** TypeScript and Go bindings regenerated with no errors.

---

#### Task 1.3 — Backend: wire `workflow_id` through CreateSession + FireNow [2h]

**Files:** `server/services/session_service.go`, `server/workflows/scheduler.go`

In `session_service.go` CreateSession handler (~line 925):
- Read `req.Msg.WorkflowId` and store in `InstanceOptions` or directly set on the ent session after creation via `s.storage.UpdateSession(ctx, sess.ID, func(u *ent.SessionUpdateOne) { u.SetWorkflowID(wfID) })`

In `scheduler.go` FireNow:
- Pass `WorkflowId: wf.ID.String()` in `CreateSessionRequest`

In `session_service.go` ListSessions handler:
- If `req.Msg.WorkflowId != ""` → add `.Where(session.WorkflowIDEQ(req.Msg.WorkflowId))` predicate
- If `!req.Msg.IncludeArchived` → add `.Where(session.ArchivedAtIsNil())` predicate (default excludes archived)

**Done when:** `go test ./server/...` passes; manual test: run a workflow, query sessions by workflow_id.

---

### Story 2: Archive RPCs + auto-archive [1–2 days]

**Acceptance criteria:**
- `ArchiveSession` sets `archived_at`; `UnarchiveSession` clears it
- Archived sessions excluded from default `ListSessions`
- Workflow sessions auto-archived when they reach Stopped status

#### Task 2.1 — ArchiveSession / UnarchiveSession handlers [1h]

**Files:** `server/services/session_service.go`

```go
func (s *SessionService) ArchiveSession(ctx context.Context, req *connect.Request[sessionv1.ArchiveSessionRequest]) (*connect.Response[sessionv1.ArchiveSessionResponse], error) {
    now := time.Now()
    err := s.storage.UpdateSession(ctx, req.Msg.SessionId, func(u *ent.SessionUpdateOne) {
        u.SetArchivedAt(now)
    })
    return connect.NewResponse(&sessionv1.ArchiveSessionResponse{}), err
}

func (s *SessionService) UnarchiveSession(ctx context.Context, req *connect.Request[sessionv1.UnarchiveSessionRequest]) (*connect.Response[sessionv1.UnarchiveSessionResponse], error) {
    err := s.storage.UpdateSession(ctx, req.Msg.SessionId, func(u *ent.SessionUpdateOne) {
        u.ClearArchivedAt()
    })
    return connect.NewResponse(&sessionv1.UnarchiveSessionResponse{}), err
}
```

**Done when:** RPC callable; `go build ./...` passes.

---

#### Task 2.2 — Auto-archive workflow sessions on Stopped [1.5h]

**Files:** `server/services/session_service.go` (status update path)

Find where sessions transition to `Stopped` status (likely `updateSessionStatus` or the WatchSessions event handler). After confirming status = Stopped, if `sess.WorkflowID != ""`, call archive logic automatically.

Consider making this configurable (default: on for workflow sessions, off for regular sessions).

**Done when:** Running a workflow creates a session; when that session stops, it disappears from the default session list and appears under "Show archived."

---

### Story 3: WorkflowsPanel — Run button + Recent Runs [1–2 days]

**Acceptance criteria:**
- Each workflow row has a "Run ▶" button
- Clicking it calls `RunWorkflow` RPC and navigates to the session
- Expanding a row shows last 5 runs with timestamp, status badge, and session link

#### Task 3.1 — `useWorkflowRuns` hook [1h]

**File:** `web-app/src/lib/hooks/useWorkflowRuns.ts`

```typescript
export function useWorkflowRuns(workflowId: string, limit = 5) {
  // calls ListSessions with workflow_id filter
  // returns { sessions, isLoading }
}
```

---

#### Task 3.2 — WorkflowsPanel: Run button + Recent Runs accordion [2h]

**Files:** `web-app/src/components/workflows/WorkflowsPanel.tsx`, `WorkflowsPanel.css.ts`

- Add "Run ▶" button to Actions column
- `onClick` → calls `runWorkflow({ id })` from `useSessionService`, then `router.push(?session=...)`
- Add expandable "Recent Runs" row below each workflow (toggle with ▸/▾)
- Each run row: `[timestamp] [status badge] [→ go to session]`

---

### Story 4: Session list — archive button + filter [1 day]

**Acceptance criteria:**
- Session items have an "Archive" action (⊗ or archive icon)
- Archived sessions hidden from default list
- "Show archived" toggle reveals them with visual distinction

#### Task 4.1 — Archive/Unarchive in session list UI [2h]

**Files:** Session list item component (wherever delete/pause actions live), `useSessionService.ts`

Add `archiveSession(sessionId)` and `unarchiveSession(sessionId)` to the service hook and wire to a button on the session list item.

---

#### Task 4.2 — "Show archived" filter [1h]

**Files:** Session list filter component

Add `include_archived` toggle. When on, pass `includeArchived: true` to ListSessions. Show archived sessions with muted styling and a "🗄 archived" badge.

---

## Dependency Graph

```
Task 1.1 (schema) ──► Task 1.2 (proto) ──► Task 1.3 (backend wire)
                                                │
                                    ┌───────────┼───────────┐
                                    ▼           ▼           ▼
                              Task 2.1      Task 3.1    Task 4.1
                              (archive      (runs hook)  (archive
                               RPCs)            │         button)
                                 │          Task 3.2        │
                             Task 2.2       (panel UI)  Task 4.2
                             (auto-arch)                (filter)
```

Tasks 2.1, 3.1, 4.1 can run in parallel once Task 1.3 is done.

---

## Known Issues

**🐛 Race: session created but workflow_id not yet set when list refreshes**
- Risk: Frontend requests ListSessions immediately after RunWorkflow; session might appear without workflow_id
- Mitigation: Set workflow_id during CreateSession (field 23 in request), not as a separate update

**🐛 Cascade delete: archived sessions referencing deleted workflow**
- Risk: Deleting a workflow while archived sessions still reference it
- Mitigation: `workflow_id` stored as plain string, not an ent FK edge — orphan-safe

**🐛 ListSessions default excludes archived: existing callers get fewer results**
- Risk: Review queue, analytics, backlog sync might silently skip archived sessions
- Mitigation: Pass `include_archived: true` from all internal callers that need full session sets; audit call sites

**🐛 Auto-archive fires on externally-stopped sessions**
- Risk: If a non-workflow session stops and somehow has workflow_id set (edge case), it gets auto-archived
- Mitigation: Auto-archive only when `workflow_id != ""` AND the workflow still exists (check before archiving)

---

## Integration Checkpoints

**After Story 1:** `go test ./...` passes; can manually set workflow_id via proto; ListSessions workflow_id filter works.

**After Story 2:** Running a workflow → session created → session stops → session disappears from list (auto-archived). Manual archive/unarchive via RPC.

**After Story 3:** WorkflowsPanel has functional Run button and shows recent runs accordion.

**After Story 4:** Session list has archive button; "Show archived" toggle works; full round-trip verified.
