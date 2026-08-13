# Architecture Research — Session Steering

## Where Background Services Are Registered in `server/dependencies.go`

The background infrastructure is built in three phases in `BuildDependencies()`:
- **Phase 1** (`BuildCoreDeps`): EventBus, ReviewQueue, Storage — pure construction, no goroutines
- **Phase 2** (`BuildServiceDeps`): StatusManager, ReviewQueuePoller, PRStatusPoller — wired but not started
- **Phase 3** (`BuildRuntimeDeps`): instances loaded, goroutines launched

### Goroutines Started in Phase 3 (the background `go func()` at line 409)
1. Instance `Start()` with 200ms stagger
2. `RecoverFromStopped` reconciliation
3. `StartController()` for each running instance
4. `StartupScanner.Scan()` + `syncOrphanedApprovalsToQueue()`

### Long-running tickers after the `go func()`:
- `60s` reconcile ticker for `BacklogLifecycleListener.ReconcileStuck` (lines 625-633)
- `HistoryLinker.Start(ctx)` — called elsewhere (from `server.go`)

### Insertion Point for the Watchdog Coordinator

After `backlogSvc` is created (line 654), before the `RuntimeDeps` struct is returned. Pattern:
```go
// After: backlogSvc := services.NewBacklogService(...)

watchdog := session.NewSessionWatchdog(instances, backlogLifecycleListener)
if cfg.GetFeatureFlag("backlog") {
    watchdog.Start(context.Background())
}
```

The `RuntimeDeps` struct needs a new field:
```go
SessionWatchdog *session.SessionWatchdog
```

And `ServerDependencies` + `ToServerDeps()` need corresponding entries.

---

## Backlog Session Creation: Exact Function Names and Call Sites

### Triage Sessions
**Location**: `server/services/backlog_service.go:TriggerTriage` (line 1022)
**Path**: `BacklogService.TriggerTriage` → `s.sessionCreator.CreateDirectorySession(ctx, "triage:"+slug, item.RepoPath, triagePrompt, []string{"backlog:triage"}, true)`
**Driver wiring**: `sessionCreator` IS `SessionService.CreateDirectorySession` which calls `session.StartSessionDriver` at line 474. **Driver is already wired.**

### Work Sessions
**Location**: `server/services/backlog_service.go:SpawnSessionFromItem` (line 809)
**Path**: `BacklogService.SpawnSessionFromItem` → `s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, prompt, []string{"backlog:work"}, false)`
**Driver wiring**: Same as above — already wired through `CreateDirectorySession`. **Driver is already wired.**

### Review Sessions (auto-spawned by lifecycle listener)
**Location**: `session/backlog_lifecycle.go:spawnReviewGate` (line 156)
**Path**: `l.sessionCreator.SpawnReviewSession(ctx, item, is.ID.String(), prompt)` → `SessionService.SpawnReviewSession` → `CreateDirectorySession(ctx, "review:"+item.ID[:8], ...)`. **Driver is already wired.**

### Review Sessions (manual re-review via RPC)
**Location**: `server/services/backlog_service.go:TriggerReReview` (line 1299)
**Path**: `s.sessionCreator.CreateDirectorySession(ctx, "re-review:"+slug, item.RepoPath, reReviewPrompt, []string{"backlog:review"}, true)`. **Driver is already wired.**

**Summary**: ALL backlog sessions go through `SessionService.CreateDirectorySession` which already calls `session.StartSessionDriver`. The driver IS wired for all backlog session types. No changes needed in `backlog_service.go` for initial driver wiring. The gap is MCP-created sessions.

---

## MCP Session Creation: Where to Add Driver Wiring

**Location**: `server/mcp/tools_lifecycle.go:createSession` (line 92)

Current flow (lines 162-219):
```go
inst, err := session.NewInstance(session.InstanceOptions{...})
inst.Start(true)
// MCP injection
// Hook injection
lh.store.AddInstance(inst)
// returns result — NO driver wired
```

`lh.svc` is `*services.SessionService`. The fix is one line after `inst.Start(true)`:
```go
session.StartSessionDriver(inst, path)
```

This is the only place where an automated session bypasses `CreateDirectorySession`. However, MCP sessions are not necessarily automated — they could be user-created. A flag would allow selective driver wiring, but the requirements say "every automated session gets a driver." MCP sessions tagged `source:mcp` (line 147) are presumed automated.

**Alternative**: Route through `lh.svc.CreateDirectorySession` instead of calling `NewInstance` + `Start` directly. This would automatically inherit all wiring (driver, rate limit callbacks, status change callbacks, lifecycle listener). The tradeoff is losing the `branch`, `session_type` flexibility that `CreateDirectorySession` doesn't currently support.

**Recommendation**: Add `session.StartSessionDriver(inst, path)` directly after `inst.Start(true)` in `createSession` in `tools_lifecycle.go`. This is minimal and matches the exact pattern in `CreateDirectorySession`.

---

## Should the Driver Live in `session/` or `server/`?

**Current state**: `session/session_driver.go` — the driver is already in the `session` package. It takes `*Instance` and `allowedPath string` as parameters, with no import of anything from `server/`.

**Recommendation**: Keep the driver and watchdog coordinator in the `session` package. Rationale:
1. The driver only needs `session.Instance` — no server layer types
2. `backlog_lifecycle.go` (in `session/`) already does complex background work
3. Moving to `server/` would create a circular import risk since `server/` imports `session/`
4. All existing background workers for instance supervision live in `session/`

The watchdog coordinator should be `session/session_watchdog.go` following the naming convention.

---

## Interface the Watchdog Coordinator Needs

The watchdog coordinator needs to:
1. Iterate a list of steered sessions (detect dead/stuck)
2. Trigger restart with continuation prompt
3. Mark a session as `NeedsAttention` (or equivalent) after second failure
4. Report failures to a central log

**Minimum interface for failure reporting**:
```go
// WatchdogFailureReporter allows the watchdog to notify the server layer of failures.
type WatchdogFailureReporter interface {
    OnSessionStuck(inst *Instance, reason string)
    OnSessionNeedsAttention(inst *Instance, reason string)
}
```

In practice, the `ReviewQueue` is the existing mechanism for surfacing sessions that need attention. The watchdog can call `reviewQueue.Add(item)` directly (it already has access to this via `BuildRuntimeDeps`). The `BacklogLifecycleListener` is NOT the right place to report failures — it handles work session exit events, and the watchdog handles a different failure mode (stuck/no-exit).

**Simpler approach**: The watchdog coordinator does not need a separate interface. It holds references to:
- `[]*Instance` (the sessions it monitors)
- `*ReviewQueue` (to add stuck sessions for operator attention)
- `*session.BacklogLifecycleListener` (to check enabled state — skip steered sessions if backlog disabled)

The `ReviewQueue.Add` call with `ReasonSteeredSessionStuck` is the notification mechanism.

---

## Watchdog Coordinator: Proposed Structure

```go
// session/session_watchdog.go
type SessionWatchdog struct {
    mu       deadlock.RWMutex
    sessions []*Instance
    queue    ReviewQueueWriter
    enabled  atomic.Bool
}

func (w *SessionWatchdog) Start(ctx context.Context) {
    go w.run(ctx)
}

func (w *SessionWatchdog) run(ctx context.Context) {
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.checkAll()
        }
    }
}
```

The `checkAll()` method iterates sessions, checks `LastMeaningfulOutputTime`, and adds to `ReviewQueue` if stuck. This follows the `ReconcileStuck` / `ReviewQueuePoller` pattern exactly.
