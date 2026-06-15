# Architecture Research: Workflow Session Affordances

## 1. Scheduler Design

### Package and Location
`server/workflows/scheduler.go` — package `workflows`.

### Struct
```go
type Scheduler struct {
    c        *cron.Cron           // robfig/cron v3 engine
    repo     session.WorkflowRepository
    sessionSvc SessionServiceInterface
    eventBus *events.EventBus
    mu       sync.Mutex
    entryMap map[string]cron.EntryID  // workflowID (UUID string) → cron entry
}
```

### Goroutine Model
The scheduler does **not** spawn its own goroutine. `robfig/cron` manages an internal goroutine
per `Cron` instance. Each registered cron function runs in a goroutine spawned by the cron engine
on schedule. `Start()` calls `s.c.Start()` after loading all enabled workflows from the DB.

`Stop()` (called as a server shutdown hook) drains in-flight jobs with an 8-second timeout using
the `cron.Cron.Stop()` blocking channel.

### Timing / Cron Expression Format
5-field cron: `Minute | Hour | Dom | Month | Dow` (no seconds field). Validated in
`ValidateCronExpression()` using the same parser, exported so `workflow_service.go` can validate
before persisting.

### Workflow Registration
- `Start(ctx)` — loads all `cron_enabled=true` workflows via `repo.ListEnabled()`, registers each.
- `Reload(ctx, wf)` — called after create/update: removes old entry (if any) then adds new one (if `cron_enabled=true`).
- `Remove(workflowID)` — called after delete.

All map mutations are guarded by `s.mu`.

### Session Creation on Fire
`FireNow(ctx, wf, arg)` is the single entry point for firing a workflow (both cron and manual):
1. Builds a prompt from `wf.Command` + `wf.InputTemplate` + `arg` (with `{{input}}` interpolation).
2. Constructs title: `"<workflow.Name> — YYYY-MM-DD HH:MM"`.
3. Appends `--model <model>` to the program string when `wf.Model` is set and program is `claude`.
4. Calls `sessionSvc.CreateSession` via the `SessionServiceInterface` with `WorkflowId` set to `wf.ID.String()`.
5. Returns the created session ID.

The `SessionServiceInterface` is a minimal interface (only `CreateSession`) defined in the
`workflows` package to avoid a circular import with `server/services`.

### Initialization in `server/dependencies.go`
```
WorkflowRepository  → EntWorkflowRepository (ent-backed, using storage's ent client)
WorkflowScheduler   → workflows.NewScheduler(workflowRepo, sessionService, eventBus)
WorkflowService     → services.NewWorkflowService(workflowRepo, workflowScheduler)
```
`workflowScheduler.Start(ctx)` is called in `server.go` / `wireDepsIntoServer`.

---

## 2. Existing Archive Mechanism

### Archive is a Soft-Delete on the Session
`sessions.archived_at` (`*time.Time`, nullable) — set when archived, nil when not archived.

Index: `index.Fields("archived_at")` exists in the ent schema.

### Manual Archive RPCs
- `ArchiveSession` (`// +api: session:archive`) — sets `inst.ArchivedAt = &now`, saves.
- `UnarchiveSession` (`// +api: session:unarchive`) — clears `inst.ArchivedAt`, saves.

### Auto-Archive on Workflow Session Exit (the key mechanism)
**`wireAutoArchiveCallback(inst)`** — called in three places:
1. `loadInstancesWithWiring()` — on server startup, for all persisted instances.
2. `CreateSession` (main creation path).
3. `CreateDirectorySession` (automated-session path used by backlog).

If `inst.WorkflowID == ""`, the function is a no-op.

Otherwise it registers an `autoArchiveListener` that implements `session.LifecycleListener`:
```go
func (l *autoArchiveListener) OnLifecycleEvent(event session.LifecycleEvent, _ string) {
    if event == session.EventExited {
        go l.svc.maybeAutoArchive(l.inst)
    }
}
```

**`maybeAutoArchive(inst)`**:
- Guards on `inst.WorkflowID == ""`.
- CAS via `inst.SetArchivedAtIfNil(now)` — uses `stateMutex`; returns false if already set
  (prevents double-archive from concurrent `EventExited` fires).
- Saves the instance via `s.storage.SaveInstances`.

**`EventExited`** fires from `instance.go` when the underlying program exits (`fireLifecycleEvent`).

### ListSessions Filter
```go
if inst.ArchivedAt != nil && !req.Msg.IncludeArchived {
    continue
}
```
Archived sessions are excluded from the default list unless `include_archived` is requested.
There is also a `workflow_id` filter in the same loop:
```go
if req.Msg.WorkflowId != nil && *req.Msg.WorkflowId != "" && inst.WorkflowID != *req.Msg.WorkflowId {
    continue
}
```

### No Background Cleanup Goroutine for Sessions
There is **no** periodic job that bulk-deletes or bulk-archives sessions. Archive is event-driven
(on exit) or user-triggered. The only background cleanup goroutines are:
- 60s reconcile ticker (`backlogLifecycleListener.ReconcileStuck`).
- 30-minute reaper (`ReapPausedTmuxSessions`).
- Approval expiry cleanup (every 30s).
- Analytics retention enforcer (every 1h, in `server/analytics/retention.go`).

---

## 3. Workflow Session Creation Flow (End-to-End)

```
Cron fires (robfig/cron goroutine)
  └─ Scheduler.addCronEntry closure
       └─ Scheduler.FireNow(ctx, wf, "")
            └─ sessionSvc.CreateSession(ctx, req) with WorkflowId=wf.ID.String()
                 └─ SessionService.CreateSession (server/services/session_service.go)
                      ├─ Resolves path, session type, etc.
                      ├─ session.NewInstance(opts) — sets inst.WorkflowID = req.WorkflowId
                      ├─ inst.Start(true) — starts tmux session
                      ├─ session.StartSessionDriver(inst, ...) — background driver goroutine
                      ├─ wireAutoArchiveCallback(inst) — registers lifecycle listener
                      ├─ storage.AddInstance(inst) — persists to ent DB (writes workflow_id)
                      ├─ reviewQueuePoller.AddInstance(inst)
                      └─ eventBus.Publish(SessionCreatedEvent)
```

**Key fields written to DB:**
- `sessions.workflow_id` = UUID string of the Workflow that fired.
- `sessions.title` = `"<Workflow.Name> — YYYY-MM-DD HH:MM"`.
- `sessions.one_off` = true if `wf.SessionType == session.SessionTypeOneOff`.
- `sessions.archived_at` = nil at creation; set by `maybeAutoArchive` on exit.

**`session.WorkflowID`** lives on the `Instance` struct (in-memory) and is serialized/deserialized
via `instance_serialization.go` ↔ `InstanceData.WorkflowID` ↔ ent session row `workflow_id`.

---

## 4. DB Relations: Workflow → Sessions

### Schema

**`workflow` table** (`session/ent/schema/workflow.go`):
| Field | Type | Notes |
|---|---|---|
| `id` | UUID | PK |
| `slug` | string unique | human-readable key |
| `name` | string | display name |
| `command` | string | base instruction |
| `target_directory` | string optional | working directory |
| `input_template` | string optional | `{{input}}` interpolation |
| `session_type` | string optional | default `"directory"` |
| `model` | string optional | appended as `--model` flag |
| `agent_type` | string optional | program override |
| `cron_expression` | string optional | 5-field cron |
| `cron_enabled` | bool | default false |
| `created_at` | time | immutable |
| `updated_at` | time | auto-updated |

Indexes: `slug`, `cron_enabled`, `created_at`. **No edges** — the workflow table has no foreign key edges to sessions.

**`sessions` table** — carries a bare string column `workflow_id` (optional, indexed):
```go
field.String("workflow_id").Optional().Comment("UUID of the Workflow that spawned this session, if any.")
index.Fields("workflow_id")
```

This is a **denormalized string FK**, not an ent edge. The workflow table has `Edges() []ent.Edge { return nil }`, so there is no back-reference edge from workflow to sessions.

### Querying Sessions by Workflow ID
The ent-generated `session/where.go` provides:
```go
session.WorkflowID(v string)        // EQ predicate
session.WorkflowIDIn(vs ...string)  // IN predicate
session.WorkflowIDNEQ(v string)
// ...and GT/GTE/LT/LTE/Contains/HasPrefix/etc.
```

A query for all sessions belonging to workflow `X` would be:
```go
client.Session.Query().
    Where(session.WorkflowID(workflowIDString)).
    Order(ent.Desc(session.FieldCreatedAt)).
    All(ctx)
```

**No such method exists today** in `EntRepository` or `WorkflowRepository`. The `ListSessions`
RPC performs this filter in-memory (iterating the live poller's instances and skipping
non-matching `WorkflowID`). For retention/cleanup purposes a direct DB query will be needed.

---

## 5. Proposed Architecture for Retention

### Options Considered

**Option A: Inside the Scheduler**
The scheduler already has a `*cron.Cron` engine and fires per-workflow. A retention sweep could
be added as an additional cron entry (e.g., hourly). However the scheduler's interface is minimal
(`SessionServiceInterface` = only `CreateSession`), and it does not own a session query interface.
Adding retention there would require expanding the interface or adding a second repository
dependency (session store), which widens the scheduler's scope.

**Option B: Dedicated retention goroutine in `server/dependencies.go` or `server/server.go`**
The existing pattern for periodic jobs is a `go func()` with a `time.NewTicker` spawned in
`BuildRuntimeDeps` or `wireDepsIntoServer`. Examples:
- 60s reconcile ticker (in `dependencies.go`).
- 30m reaper (in `dependencies.go`).
- Hourly analytics retention enforcer (in `server.go` via `analytics.StartRetentionEnforcer`).

This is the strongly preferred pattern. A `StartWorkflowSessionRetentionEnforcer` function in a
new `server/workflows/retention.go` file mirrors the `server/analytics/retention.go` model.

**Option C: Extend `WorkflowService`**
Could add a `SweepOldSessions(ctx, workflowID, maxCount, maxAgeDays)` method. Problem: `WorkflowService`
does not currently have access to the session store (it only holds `WorkflowRepository` and the
scheduler). Would need to add a new dependency.

### Recommended: Option B — Separate Retention Goroutine

```go
// server/workflows/retention.go

// StartRetentionEnforcer starts a background goroutine that periodically archives
// or deletes old workflow sessions according to per-workflow or global retention policy.
//
// - interval: how often to sweep (e.g. time.Hour)
// - The goroutine exits when ctx is cancelled.
func StartRetentionEnforcer(
    ctx context.Context,
    entClient *ent.Client,
    workflowRepo session.WorkflowRepository,
    interval time.Duration,
) {
    if entClient == nil {
        return
    }
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        runRetention(ctx, entClient, workflowRepo)     // run once on startup
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                runRetention(ctx, entClient, workflowRepo)
            }
        }
    }()
}
```

**Registered in** `server/server.go` after `workflowScheduler` is available (same location as
the analytics retention call), receiving `serverCtx` so it exits cleanly on shutdown.

### Retention Policy Fields (proposed additions to Workflow schema)
To support per-workflow retention config, add fields to the `workflow` ent schema:
```go
field.Int("max_sessions").Optional().Comment("Maximum number of sessions to retain per workflow; oldest archived first. 0 = unlimited.")
field.Int("session_retention_days").Optional().Comment("Sessions archived more than this many days ago are deleted. 0 = unlimited.")
```

For a simpler v1, global defaults from `config.json` (similar to `AnalyticsMaxRowsOrDefault()`)
are sufficient without schema changes.

### Retention Logic Per Workflow
```
For each workflow:
  1. Query sessions WHERE workflow_id = wf.ID AND archived_at IS NOT NULL
     ORDER BY archived_at ASC (oldest first)
  2. If count > max_sessions: delete the excess oldest rows
  3. If session_retention_days > 0: delete rows WHERE archived_at < cutoff
```

Note: only archived sessions are candidates for deletion. Live (unarchived) sessions are never
purged automatically — `maybeAutoArchive` already handles the archive-on-exit path.

---

## 6. Query Patterns Available for "Sessions by Workflow"

### In-Memory (Current)
`ListSessions` RPC filters in-memory across all live instances held by `reviewQueuePoller`:
```go
if req.Msg.WorkflowId != nil && *req.Msg.WorkflowId != "" && inst.WorkflowID != *req.Msg.WorkflowId {
    continue
}
```
This works for live sessions but misses fully-stopped sessions that have been evicted from memory
(if any such eviction path exists) and does not give access to archived sessions without the
`include_archived` flag.

### Direct DB Query (Needed for Retention)
Using the ent client directly:
```go
// All sessions for a workflow (including archived)
client.Session.Query().
    Where(session.WorkflowID(wfID)).
    Order(ent.Desc(session.FieldCreatedAt)).
    All(ctx)

// Only archived sessions for a workflow, oldest first
client.Session.Query().
    Where(
        session.WorkflowID(wfID),
        session.ArchivedAtNotNil(),
    ).
    Order(ent.Asc(session.FieldArchivedAt)).
    All(ctx)

// Count of archived sessions per workflow
client.Session.Query().
    Where(
        session.WorkflowID(wfID),
        session.ArchivedAtNotNil(),
    ).
    Count(ctx)
```

The `Where` predicates are already generated in `session/ent/session/where.go`:
- `session.WorkflowID(v)` / `session.WorkflowIDIn(vs...)` — exact match
- `session.ArchivedAtNotNil()` — sessions that have been archived
- `session.ArchivedAtLT(t)` — archived before a cutoff time

All queries benefit from the existing `index.Fields("workflow_id")` and `index.Fields("archived_at")`
indexes on the sessions table.

### Adding `ListByWorkflowID` to EntRepository (Optional)
For clean separation, a `ListByWorkflowID(ctx, workflowID string) ([]InstanceData, error)` method
could be added to `EntRepository` following the same pattern as `ListByTag` and `ListByStatus`.
This would be the appropriate place to add it if the RPC layer needs to expose this to the frontend
(e.g., a "workflow history" panel).

---

## Summary of Key Files

| Purpose | File |
|---|---|
| Scheduler (cron engine, `FireNow`) | `server/workflows/scheduler.go` |
| Workflow CRUD + RunWorkflow RPC | `server/services/workflow_service.go` |
| Session creation + auto-archive wiring | `server/services/session_service.go` |
| Auto-archive listener and `maybeAutoArchive` | `server/services/session_service.go:3388–3787` |
| Lifecycle event types (`EventExited`) | `session/instance.go:63–80` |
| `SetArchivedAtIfNil` (CAS archive) | `session/instance_state.go:224` |
| WorkflowRepository interface | `session/workflow_repository.go` |
| EntWorkflowRepository implementation | `session/ent_workflow_repository.go` |
| EntRepository (session queries) | `session/ent_repository.go` |
| Workflow ent schema | `session/ent/schema/workflow.go` |
| Session ent schema (workflow_id, archived_at) | `session/ent/schema/session.go` |
| Session ent predicates (WorkflowID, ArchivedAt) | `session/ent/session/where.go` |
| Analytics retention (goroutine pattern reference) | `server/analytics/retention.go` |
| Background goroutine startup location | `server/dependencies.go:713–808` |
| Scheduler startup + retention registration location | `server/server.go:443–476` |
