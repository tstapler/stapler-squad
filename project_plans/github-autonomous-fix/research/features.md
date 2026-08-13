# Features Research: AutonomousDriver Goroutine

## 1. What needs to be built: AutonomousDriver concept

`AutonomousMode bool` exists on `Instance` (line 146, `instance.go`) but is entirely unused — no goroutine reads it, no behavior changes on it. The "github-autonomous-fix" feature needs to build the **AutonomousDriver** that activates when `AutonomousMode == true`.

---

## 2. Idle Detection API (session/detection/idle.go + claude_controller.go)

The idle state is already fully observable at the `ClaudeController` level:

```go
// Poll approach (any goroutine can call these):
state, lastActivity := cc.GetIdleState()   // returns (IdleState, time.Time)
info := cc.GetIdleStateInfo()              // full struct: state, duration, lastChange

// Event-driven approach (preferred):
cc.SetStatusChangeListener(func(status detection.DetectedStatus, sessionName string) {
    // Called on every terminal status transition
    // StatusIdle / StatusReady → Claude finished, waiting for next prompt
    // StatusActive → Claude is working
    // StatusSuccess → turn complete (cost summary line visible)
})
```

The `StatusChangeListener` fires from `runStatusChangeLoop`, which is driven by `statusCheckCh` — a capacity-1 channel signaled on every PTY write. This is **event-driven with no polling overhead** when Claude transitions to idle.

Key: `detection.StatusSuccess` fires when the terminal shows the cost-summary line (`$X.XX •`) or `✻ Verb for Ns`. This is the most reliable "task complete" signal.

`detection.IdleStateWaiting` and `detection.IdleStateTimeout` are the "ready for next command" states.

---

## 3. WriteToPTY / SendCommand API

The AutonomousDriver can inject prompts via:

```go
// Via ClaudeController (queued, with history tracking):
cmdID, err := cc.SendCommand("Fix the failing tests.\r", priorityNormal)

// Via ClaudeController (immediate, bypasses queue):
result, err := cc.SendCommandImmediate("Fix the failing tests.\r")

// Directly via Instance (raw PTY, like session_driver.go does):
err := inst.SendKeys("Fix the failing tests.\r")
```

The existing `session_driver.go` uses `inst.SendKeys()` directly. For an AutonomousDriver, `cc.SendCommandImmediate()` is better because it:
1. Waits for the current session to be at a prompt before sending
2. Adds the command to history (auditable)
3. Can have a timeout

---

## 4. session_driver.go Patterns (existing driver to mirror)

`StartSessionDriver(inst *Instance, allowedPath string)` is the canonical reference:
- Uses `inst.driverRunning.CompareAndSwap(false, true)` for idempotency
- Polls at `driverPollInterval = 2s`
- Reads `inst.GetEffectiveStatus()` to get current lifecycle status (`Active`, `Stopped`, `Paused`, etc.)
- Reads `inst.GetStatusManager().GetStatus(inst)` for Claude-level status (`StatusNeedsApproval`, etc.)
- Sends `inst.SendKeys(prompt + "\r")` to inject prompts
- Handles retries via `handleDriverFailure`

An **AutonomousDriver** would follow the same `CompareAndSwap` idempotency pattern, but instead of a static initial prompt, it would be driven by external signals (GitHub CI status, PR review feedback, etc.).

---

## 5. BacklogLifecycleListener (`session/backlog_lifecycle.go` + session_service.go)

`BacklogLifecycleListener` is wired to every session created via `CreateDirectorySession` (session_service.go line 577):
```go
if s.backlogLifecycleListener != nil {
    s.backlogLifecycleListener.WireToInstance(instance)
}
```

It listens to `LifecycleEvent` (EventStarted, EventExited) via `RegisterLifecycleListener`. The AutonomousDriver should plug into this same mechanism — the `OnLifecycleEvent` callback is the natural place to start/stop the driver goroutine.

---

## 6. SpawnSessionFromItem wiring pattern (`server/services/backlog_service.go` lines 843-961)

`SpawnSessionFromItem` creates a work session with:
```go
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, prompt,
    []string{"backlog:work"}, false, false)
```

The `sessionCreator` interface is `CreateDirectorySession(ctx, title, path, prompt, tags, oneShot, hidden)`. An AutonomousDriver for GitHub fix sessions would use the same interface, adding `PermissionMode: "bypassPermissions"` or `AllowedTools` to allow automated operation.

`CreateDirectorySession` in `session_service.go` (lines 542-580) already wires:
- `session.StartSessionDriver(instance, path)` — existing driver goroutine
- `s.wireRateLimitCallbacks(instance)`
- `s.wireStatusChangeCallback(instance)`
- `s.wireClaudeSessionIDCallback(instance)`
- `s.backlogLifecycleListener.WireToInstance(instance)` — if set

The AutonomousDriver would need to be wired here too, or alternatively activated by the `StatusChangeListener` that's already wired.

---

## 7. BacklogPlugin Interface (`session/backlog_plugin.go`)

The plugin system for external item sources:
```go
type ItemSourcePlugin interface {
    PluginID() string
    Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error)
    MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData
}
```

`ExternalItem` has: `ExternalID`, `Title`, `Description`, `Labels`, `Priority`, `URL`.

The `GitHubIssuesPlugin` (`backlog_plugin_github.go`) fetches via `GET /repos/{owner}/{repo}/issues`. This is issues, not PRs — for the "github-autonomous-fix" feature (fixing failing CI on PRs), we need a **PR-focused plugin** or repurpose the existing PR status poller.

---

## 8. What AutonomousDriver needs to DO (inferred from feature name)

The "github-autonomous-fix" feature appears to mean: when a PR has failing CI checks, autonomously spawn a Claude session to fix the failures.

**Minimum viable loop:**
1. PR status poller detects `GitHubCheckConclusion == "failure"` on a session's PR
2. AutonomousDriver activates (if `AutonomousMode == true`)
3. Waits for `IdleStateWaiting` via `StatusChangeListener`
4. Injects a prompt: "CI is failing. Check the CI logs and fix the failures."
5. Waits for `StatusSuccess` (turn complete)
6. Checks CI status again → repeat if still failing
7. Stops when CI passes or after N attempts

The `SetStatusChangeCallback` on `Instance` is already wired to a server-layer callback — the AutonomousDriver needs to either register a second listener or co-opt this one.

**Note**: Multiple listeners aren't directly supported on `Instance` — only one `onStatusChange func` is stored. The AutonomousDriver would need to either:
- Chain callbacks (store old fn, call both), or
- Read state via polling `cc.GetIdleState()`, or  
- Add a multi-listener mechanism to the controller

---

## 9. Session Creation via CreateDirectorySession for Autonomous Mode

```go
// In CreateDirectorySession (session_service.go line 545):
opts := session.InstanceOptions{
    Title:           title,
    Path:            path,
    Program:         resolved.Program,
    AutoYes:         true,   // automated sessions must not block
    SessionType:     session.SessionTypeDirectory,
    Prompt:          prompt,
    Tags:            tags,
    OneShot:         oneShot,
    Hidden:          hidden,
    MCPServerURL:    s.mcpServerURL,
    CreateIfMissing: true,
    // For autonomous mode, also set:
    // AllowedTools:   "Bash,Edit,Read,Write",
    // PermissionMode: "bypassPermissions",
    // AutonomousMode: true,
}
```

`InstanceOptions` fields `AllowedTools` and `PermissionMode` are already present and wired through to CLI flags in `instance_tmux.go`.
