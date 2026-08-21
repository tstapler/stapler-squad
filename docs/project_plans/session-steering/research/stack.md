# Stack Research — Session Steering

## Goroutine Lifecycle Patterns in This Codebase

### Pattern: Simple `for range ticker.C` with deadline check
The existing `session_driver.go` is the canonical model for session-steering goroutines. It uses `time.NewTicker` + `defer ticker.Stop()` and checks a `totalDeadline` on each tick. There is **no context cancellation** — it relies on `inst.Status == Stopped || inst.Status == Paused` as a terminal guard. Follow this pattern for the new dead/stuck detection loop.

### Pattern: `context.WithCancel` for background services
`HistoryLinker.Start(ctx)` uses `ctx.Done()` in a `select` to terminate its poll loop. This is the preferred pattern for longer-lived services that need clean cancellation (e.g., the watchdog coordinator).

### Pattern: `atomic.Bool` for enable/disable toggle
`BacklogLifecycleListener.enabled` is an `atomic.Bool`. The toggle is set by `SetEnabled(v bool)`. All event handler entry points check `l.enabled.Load()` as a guard. Copy this pattern for a watchdog coordinator's enabled state.

### Pattern: `sync/atomic` for counters
`ReviewQueuePoller.tickCount` is an `atomic.Int64`. For tracking retry counts in a driver, prefer `atomic.Int64` over a mutex-protected int — simpler and avoids lock contention with the main driver loop.

### Pattern: `deadlock.Mutex` / `deadlock.RWMutex`
The project uses `github.com/linkdata/deadlock` as a drop-in replacement for `sync.Mutex`. All new mutexes must use this package for deadlock detection in dev. Never use raw `sync.Mutex` or `sync.RWMutex`.

### No WaitGroups at Session Level
No `sync.WaitGroup` is used for session goroutines — they are fire-and-forget. Shutdown is handled by the deadline check or `ctx.Done()`. The watchdog coordinator should follow the same approach.

---

## `sync.Once` Usage
`tmuxManager.ResetExitOnce()` / `tmuxManager.SetOnExitCallback()` in `instance.go:551` shows the pattern for one-shot exit callbacks. The `sync.Once` is wrapped in a resettable helper because `Start()` can be called multiple times (restart). If the driver stores any `sync.Once` state, it must be reset on each retry cycle.

---

## `Instance` Type: Key Methods for the Driver

| Method | File | Notes |
|---|---|---|
| `inst.Status` | `instance.go` | Read under `stateMutex.RLock()` for safety; the driver currently reads it without a lock — acceptable since `Status` is an int (word-sized atomic on most platforms) but the codebase uses `stateMutex` for all mutations |
| `inst.SendKeys(keys string) error` | `instance_tmux.go:376` | Wraps `tmuxManager.SendKeys`. Returns error if session not started. Does NOT acquire `stateMutex` |
| `inst.Preview() (string, error)` | `instance_terminal.go:105` | Returns visible pane content. Used by the existing driver for startup dialog detection |
| `inst.GetEffectiveRootDir() string` | `instance_worktree.go:112` | Returns worktree path (or `inst.Path` for directory sessions) — used by `HistoryLinker.DetectByPath` |
| `inst.TmuxSessionExists() bool` | `instance_tmux.go:198` | Delegates to `tmuxManager.DoesSessionExist()`. Safe to call from a goroutine |
| `inst.Restart(preserveOutput bool) error` | `instance.go:893` | The correct way to restart; calls `start()` which re-arms the exit callback and controller |
| `inst.RecoverFromStopped()` | `instance_state.go:144` | Resets a stale `Stopped` status to `Ready` before calling `Start()`. Required before restart from Stopped |
| `inst.HistoryFilePath` | `instance.go:193` | Set by `HistoryLinker`. May be empty if the session has not yet been correlated |
| `inst.LastMeaningfulOutput` | `review_state.go:48` | Timestamp of last meaningful terminal output. Protected by `stateMutex`. Read via `inst.LastMeaningfulOutputTime()` |
| `inst.LastMeaningfulOutputTime()` | `instance_state.go:76` | Safe accessor — acquires `stateMutex.RLock()` |

---

## Backlog Session Creation: How Sessions Are Spawned

All three automated session types go through `SessionService.CreateDirectorySession` (server/services/session_service.go:452):

1. **Triage sessions**: `BacklogService.TriggerTriage` → `s.sessionCreator.CreateDirectorySession(ctx, "triage:"+slug, item.RepoPath, triagePrompt, []string{"backlog:triage"}, true)`
2. **Work sessions**: `BacklogService.SpawnSessionFromItem` → `s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, prompt, []string{"backlog:work"}, false)`
3. **Review sessions**: `SessionService.SpawnReviewSession` → `CreateDirectorySession(ctx, "review:"+item.ID[:8], item.RepoPath, prompt, []string{"backlog:review"}, true)`

`CreateDirectorySession` already calls `session.StartSessionDriver(instance, path)` at line 474. The driver is wired here. The issue is that `BacklogService.TriggerTriage` and `SpawnSessionFromItem` go through `s.sessionCreator` which IS `SessionService.CreateDirectorySession`, so the driver is already wired for all three types when created through the normal path. The gap is the `MCP create_session` tool, which does NOT call `CreateDirectorySession`.

---

## BacklogLifecycleListener: Events It Fires

`BacklogLifecycleListener` implements `LifecycleListener` via the per-instance `instanceBacklogListener` shim:
- `EventStarted` → `go l.parent.onSessionStarted(instanceUUID)` (records start time on ItemSession)
- `EventExited` → `go l.parent.onSessionExited(instanceUUID)` (transitions item: in_progress→review or done, spawns review gate)

Both handlers are dispatched to goroutines, making the `OnLifecycleEvent` call non-blocking. The session driver's failure detection must NOT call `fireLifecycleEvent(EventExited)` directly — that would conflict with the control-mode exit callback. Instead, the driver should transition the instance to `NeedsAttention` (new status) or call a dedicated watchdog callback.

---

## JSONL Conversation Logs: Path Algorithm

Claude Code stores conversation history at:
```
~/.claude/projects/<encoded-path>/<uuid>.jsonl
```

The path encoding is implemented in `session/history_detector.go:ClaudeProjectDirName`:
- Every non-alphanumeric character is replaced with `-`
- Example: `/Users/alice/myproject` → `-Users-alice-myproject`

The `HistoryLinker` populates `inst.HistoryFilePath` when it successfully correlates a session. For JSONL continuation, read `inst.HistoryFilePath` — it will be the absolute path to the active conversation file.

---

## Current Session Polling Patterns (server/dependencies.go)

The background pollers are initialized in `BuildRuntimeDeps` and started in a background goroutine (the `go func()` at line 409):
- `ReviewQueuePoller` (per-instance terminal content polling)
- `PRStatusPoller` (GitHub PR state polling)
- `BacklogLifecycleListener` with a `60s` reconcile ticker goroutine (line 625)
- `HistoryLinker` (5s polling + fsnotify)

**Insertion point for the watchdog coordinator**: The `BacklogController` and `BacklogService` are initialized at the end of `BuildRuntimeDeps` (lines 635-654). The watchdog coordinator should be constructed immediately after `BacklogService` is created, wired with the instance list, and stored in `RuntimeDeps`. It can reuse the same `60s` reconcile ticker pattern or use its own interval.
