# Cron Scheduling Research — Quick Workflows

## Summary

Quick Workflows needs a cron-style scheduler to fire session-creation jobs from a background goroutine (e.g. `0 8 * * 1` — every Monday at 08:00). This document covers the five research questions and ends with concrete recommendations.

---

## 1. Existing Scheduler Usage

### Cron libraries

Neither `go.mod` nor `go.sum` contains `robfig/cron`, `gocron`, or `go-co-op`. There is no existing cron dependency — the project will need a new one.

### Goroutine scheduling patterns

The codebase makes heavy use of `time.NewTicker` for periodic work. The canonical pattern is:

```go
func (s *MySweeper) Start(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                s.doWork()
            case <-ctx.Done():
                return
            }
        }
    }()
}
```

Examples in production code:
- `session/hibernation_sweeper.go` — 5-minute sweep cadence
- `server/analytics/retention.go` — 1-hour retention sweep  
- `session/pr_status_poller.go` — configurable poll interval
- `server/auth/session.go` — 10-minute token cleanup

All background goroutines accept a `context.Context` for clean shutdown and follow the `Start(ctx context.Context)` convention. This is the pattern to follow for the scheduler.

---

## 2. Session Service — Can `CreateSession` Be Called From Background Goroutines?

### Function signature

```go
func (s *SessionService) CreateSession(
    ctx context.Context,
    req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error)
```

### Dependencies the function uses

| Dependency | Thread-safe? | Notes |
|---|---|---|
| `s.storage` (`session.InstanceStore`) | Yes | ent-backed SQLite with its own mutex |
| `s.eventBus` | Yes | `sync.RWMutex`-protected pub/sub bus |
| `s.reviewQueuePoller` | Yes | `AddInstance` is internally guarded |
| `s.promptStore` | Yes | uses `sync.Mutex` internally |
| `config.LoadConfig()` | Yes | reads JSON file; acceptable for low-cadence calls |

### Key behaviour

`CreateSession` does all validation synchronously, then spawns a goroutine for the expensive part (`instance.Start()`). The HTTP handler returns in milliseconds after saving the instance and publishing a `SessionCreated` event; the goroutine handles tmux/worktree setup asynchronously.

### Conclusion: yes, it is safe to call from a cron goroutine

`BatchCreateSessions` (lines 2778–2896) already does exactly this — it calls `s.CreateSession(ctx, createReq)` from multiple goroutines concurrently. A cron scheduler can do the same thing:

```go
func (scheduler *WorkflowScheduler) fireJob(ctx context.Context, wf *WorkflowDefinition) {
    req := connect.NewRequest(&sessionv1.CreateSessionRequest{
        Title:        generateTitle(wf),
        Path:         wf.Path,
        Program:      wf.Program,
        InitialPrompt: wf.Prompt,
        // ...
    })
    _, err := scheduler.sessionSvc.CreateSession(ctx, req)
    if err != nil {
        // publish error notification via event bus
    }
}
```

The only thing to be aware of: pass the **scheduler's** context (derived from the server's root context), not a request-scoped context that could be cancelled before `instance.Start()` completes.

---

## 3. Server Startup and Background Service Wiring

### Server struct

`server/server.go` defines a `Server` struct with a `shutdownHooks []func()` slice. Background services are started in `wireDepsIntoServer` using one of two patterns:

**Pattern A — goroutine with context:**
```go
go deps.HistoryLinker.Start(serverCtx)
```

**Pattern B — method that spawns internally:**
```go
deps.PRStatusPoller.Start(serverCtx)
deps.ExternalDiscovery.Start(5 * time.Second)
```

`serverCtx` is a `context.Context` that is cancelled when `Server.Shutdown()` is called (via `connCtxCancel`). All background goroutines respect `<-ctx.Done()` for clean termination.

### Where to add the WorkflowScheduler

The scheduler should be added to `ServerDependencies` in `server/dependencies.go` and started in `wireDepsIntoServer`:

```go
// In ServerDependencies struct
WorkflowScheduler *workflows.Scheduler

// In wireDepsIntoServer (server/server.go)
if deps.WorkflowScheduler != nil {
    deps.WorkflowScheduler.Start(serverCtx)
    log.Info("WorkflowScheduler started")
}
```

The `WorkflowScheduler` is constructed during Phase 1 (`BuildCoreDeps`) because it only needs `SessionService` and `EventBus` — both already available there. No phase-ordering issue.

### Shutdown

The scheduler must stop all cron jobs when `ctx.Done()` fires. `wireDepsIntoServer` also supports explicit shutdown hooks:

```go
srv.shutdownHooks = append(srv.shutdownHooks, func() {
    deps.WorkflowScheduler.Stop()
})
```

---

## 4. Notification Mechanisms

### Backend notification pathway

The project has a fully wired in-app notification system using `EventBus` and `NotificationEvent`. The existing pattern used throughout the backend (e.g. tmux recovery, fork-pressure alert) is:

```go
event := events.NewNotificationEvent(
    sessionID,              // sessionID — can be "workflow-scheduler" for system events
    sessionName,            // display name
    uuid.New().String(),    // unique notification ID
    int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO),
    int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW),
    "Workflow Started",     // title
    "Daily standup workflow launched successfully.", // body
    map[string]string{
        "workflow_id": wf.ID,
        "cron":        wf.CronExpr,
    },
)
deps.EventBus.Publish(event)
```

### Available `NotificationType` values

| Enum | Value | When to use |
|---|---|---|
| `NOTIFICATION_TYPE_INFO` | 10 | Job fired successfully |
| `NOTIFICATION_TYPE_WARNING` | 8 | Job skipped (session limit, etc.) |
| `NOTIFICATION_TYPE_ERROR` | 7 | `CreateSession` returned an error |
| `NOTIFICATION_TYPE_TASK_COMPLETE` | 4 | Appears in history only, no toast |

### Frontend delivery

`EventBus.Publish` → `NotificationHistoryStore` subscriber (coalesces to disk) + `useSessionNotifications` hook (WebSocket/SSE stream to the browser). The toast component (`NotificationToast`) renders in the corner. Types `INFO`, `WARNING`, and `ERROR` produce visible toasts; `TASK_COMPLETE` goes to history-only.

### Push notifications

`server/push/` implements Web Push via `SherClockHolmes/webpush-go`. `push.StartPushSubscriber` listens on `EventBus` and forwards high-priority events to subscribed devices. For workflow start/failure events this is optional but available with no additional plumbing.

---

## 5. Cron Library Options

### Option A: `github.com/robfig/cron/v3`

- Widely used Go cron library; parses 5-field standard cron expressions (`0 8 * * 1`)
- Minimal API surface: `c := cron.New(); c.AddFunc("0 8 * * 1", fn); c.Start(); c.Stop()`
- No external dependencies
- Does NOT natively accept a `context.Context` for individual job cancellation
- Supports `cron.New(cron.WithSeconds())` for 6-field (second-granularity) expressions
- Job IDs are returned as `cron.EntryID` (int); job removal uses `c.Remove(id)`
- Zero external transitive deps beyond stdlib

### Option B: `github.com/go-co-op/gocron/v2`

- Higher-level API with `WithContext`, job chaining, distributed-lock support
- More complex API; better suited when jobs need mutual exclusion across replicas
- Heavier dependency graph

### Recommendation: `github.com/robfig/cron/v3`

For this use case, `robfig/cron/v3` is the better fit:

1. **Project style** — All existing background services use `time.Ticker` in a `select` loop with a `context.Context`. A thin wrapper around `robfig/cron` that cancels/respects context follows the same idiom.
2. **Standard cron expressions** — User-facing workflow schedules will use 5-field cron strings (matching `0 8 * * 1` format from requirements). `robfig/cron/v3` parses these natively.
3. **No lock-manager needed** — Stapler Squad is a single-process application; gocron's distributed features add unnecessary complexity.
4. **Zero transitive deps** — Keeps `go.mod` clean.

Neither library is in `go.sum` yet; `robfig/cron/v3` would be a fresh addition.

---

## Concrete Recommendations

### Which library

Use `github.com/robfig/cron/v3`. Add it with:

```bash
go get github.com/robfig/cron/v3@latest
```

### How to wire the scheduler

Create `server/workflows/scheduler.go`:

```go
package workflows

import (
    "context"

    "github.com/robfig/cron/v3"
    "github.com/tstapler/stapler-squad/server/events"
    "github.com/tstapler/stapler-squad/server/services"
)

type Scheduler struct {
    c          *cron.Cron
    sessionSvc *services.SessionService
    eventBus   *events.EventBus
}

func NewScheduler(svc *services.SessionService, bus *events.EventBus) *Scheduler {
    return &Scheduler{
        c:          cron.New(),  // 5-field standard expressions
        sessionSvc: svc,
        eventBus:   bus,
    }
}

// Start loads workflow definitions from storage, registers each enabled one
// with the cron engine, and begins processing. Stops when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
    s.loadJobs(ctx) // populate s.c with AddFunc calls
    s.c.Start()

    go func() {
        <-ctx.Done()
        s.c.Stop()
    }()
}

// Stop halts the cron engine (called from shutdown hooks).
func (s *Scheduler) Stop() {
    s.c.Stop()
}
```

Register in `server/dependencies.go` `ServerDependencies` struct and wire in `wireDepsIntoServer`:

```go
if deps.WorkflowScheduler != nil {
    deps.WorkflowScheduler.Start(serverCtx)
    log.Info("WorkflowScheduler started")
    srv.shutdownHooks = append(srv.shutdownHooks, deps.WorkflowScheduler.Stop)
}
```

### How to safely call `CreateSession` from a cron job

Use the scheduler's context (derived from `serverCtx`), not a fresh `context.Background()`:

```go
func (s *Scheduler) fireWorkflow(ctx context.Context, wf WorkflowDefinition) {
    title := fmt.Sprintf("%s — %s", wf.Name, time.Now().Format("2006-01-02 15:04"))
    req := connect.NewRequest(&sessionv1.CreateSessionRequest{
        Title:         title,
        Path:          wf.Path,
        Program:       wf.Program,
        InitialPrompt: wf.InitialPrompt,
        Category:      wf.Category,
        SkipDefaults:  false,
    })

    if _, err := s.sessionSvc.CreateSession(ctx, req); err != nil {
        s.publishError(wf, err)
        return
    }
    s.publishInfo(wf, title)
}
```

The `ctx` argument is the scheduler's long-lived context. `CreateSession` itself spawns the async goroutine, so even if the cron goroutine returns quickly, the session startup completes independently.

**Duplicate title guard**: `CreateSession` returns `CodeAlreadyExists` if a session with the same title exists. The caller should include a timestamp in the title (as shown above) or check for and handle this error gracefully — e.g. by publishing a warning notification instead of crashing.

### How to surface notifications to the user

Publish `EventNotification` events via `EventBus` for all scheduler outcomes:

```go
// Job fired successfully → history-only (informational)
events.NewNotificationEvent(
    wf.ID, wf.Name, uuid.New().String(),
    int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO),
    int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW),
    "Workflow Started",
    fmt.Sprintf("'%s' started session '%s'.", wf.Name, title),
    map[string]string{"workflow_id": wf.ID},
)

// Job failed → visible toast (WARNING or ERROR)
events.NewNotificationEvent(
    wf.ID, wf.Name, uuid.New().String(),
    int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
    int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
    "Workflow Failed",
    fmt.Sprintf("'%s' could not start: %s", wf.Name, err),
    nil,
)
```

`NOTIFICATION_TYPE_INFO` is in the frontend's `HISTORY_ONLY_TYPES` set, so successful job launches land silently in the history panel. `NOTIFICATION_TYPE_ERROR` produces a visible, audible toast — appropriate for silent background failures the user must know about.

---

## Open Questions (out of scope for this research)

1. **Persistence of workflow definitions** — The scheduler needs to load workflow configs from somewhere (ent schema table? config JSON? embedded YAML?). This depends on the workflow data model, which is a separate design decision.
2. **Hot-reload on config change** — If workflows can be added/edited without restarting the server, the scheduler needs a `Reload()` method that removes old entries and adds new ones atomically using `cron.Remove(id)` + `cron.AddFunc(...)`.
3. **Missed-fire policy** — `robfig/cron` fires the next scheduled time after startup; it does not back-fill missed fires during downtime. If the app was offline during a scheduled fire, the run is silently skipped. This is acceptable for most workflow types but may need a separate "catch-up" mechanism for critical workflows.
