# Pitfalls Research: Risks and Edge Cases

## 1. Rate Limit Handling in instance_controller.go

Rate limit detection is already built and wired. The `ratelimit.PTYConsumer` in `ClaudeController` detects Claude's rate-limit output and calls:
```go
mgr.SetDetectionCallback(func(det ratelimit.Detection) {
    onDetected(sessionID, det.ResetTime)
})
mgr.SetRecoveryCallback(func(success bool, _ ratelimit.Detection) { ... })
```

**Risk for AutonomousDriver**: If the driver sends a command while Claude is rate-limited, the command will be buffered in the PTY but Claude will not respond until the rate limit clears. The driver loop must check `cc.GetRateLimitState()` before sending commands:

```go
if cc.GetRateLimitState() != ratelimit.StateNone {
    resetTime := cc.GetRateLimitResetTime()
    // Wait or abort — do NOT send another command
}
```

`ratelimit.RateLimitState` values (from package): `StateNone`, `StateDetected`, `StateWaiting`, `StateRecovering`.

---

## 2. CommandQueue / CommandExecutor Race (`session/claude_controller.go`)

`ClaudeController` has both a `queue` (async) and an `executor.ExecuteImmediate` (sync) path. Key hazard:

- `SendCommand` enqueues; the executor dequeues and sends at the next idle window
- `SendCommandImmediate` bypasses the queue and sends directly
- If both are used concurrently, the PTY receives interleaved bytes

**Design Decision**: The AutonomousDriver should use **only** `SendCommandImmediate` with a mutex/semaphore guarding driver sends, OR use `SendCommand` and wait for the specific command ID to complete. Mixing both is a data race on the PTY.

The `CommandExecutor` has `GetCurrentCommand()` — check this before calling `SendCommandImmediate` to avoid sending during an in-progress command.

---

## 3. Approval Race Conditions (`server/services/approval_handler.go` lines 260-330)

The approval handler blocks the HTTP connection for up to 4 minutes. Three concurrent hazards:

**Hazard A: Context cancellation on server restart**
If the server restarts while an approval is pending, the HTTP connection closes, hitting the `case <-r.Context().Done()` branch which **silently allows** the tool call:
```go
case <-r.Context().Done():
    h.store.Remove(approvalID)
    decision = ApprovalDecision{Behavior: "allow", Message: ""}
    return  // Don't write to disconnected client
```
An AutonomousDriver with `bypassPermissions` avoids this, but if relying on auto-approval rules instead, server restarts could silently approve previously-denied operations.

**Hazard B: Approval timeout → native dialog fallback**
On 4-minute timeout, the handler returns an empty HTTP 200 (no `hookSpecificOutput`). Claude Code falls back to its native terminal dialog. If the AutonomousDriver is not watching for `StatusNeedsApproval`, the session will block indefinitely at the terminal prompt.

**Hazard C: Session ID resolution**
The handler resolves session ID from `X-CS-Session-ID` header first, then falls back to cwd prefix matching. If the AutonomousDriver's session is not yet in storage (race between session creation and first tool call), `sessionID` becomes `"unknown"` — the notification won't reach the UI, and the 4-minute timeout will fire.

---

## 4. GitHub API Rate Limiting (`session/backlog_plugin_github.go`)

The existing `GitHubIssuesPlugin.Fetch` handles rate limits:
```go
if resp.StatusCode == http.StatusTooManyRequests ||
    (resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
    return nil, cursor, fmt.Errorf("github_issues: rate limited (status %d)", resp.StatusCode)
}
```

**Pattern to follow**: Always check `X-RateLimit-Remaining` from GitHub API responses. The PR status poller and any CI-fetching code in the AutonomousDriver must respect GitHub's 5000 req/hr authenticated limit. Use `X-RateLimit-Reset` to back off until the reset time.

**Token management**: Config is per-plugin (`githubPluginConfig.Token`). An empty token causes `Fetch` to return `nil, cursor, nil` silently. The AutonomousDriver needs to handle missing tokens gracefully (log a warning, mark the session as needing manual attention, exit the driver).

---

## 5. session_driver.go: Retry / Inactivity Risks

The existing driver has important guards that the AutonomousDriver must replicate:

**Guard 1: `driverRunning.CompareAndSwap(false, true)` idempotency**
```go
func StartSessionDriver(inst *Instance, allowedPath string) {
    if !inst.driverRunning.CompareAndSwap(false, true) { return }
    // ...
}
```
The AutonomousDriver needs its own `atomic.Bool` guard — calling it twice would spawn two goroutines that both write to the PTY.

**Guard 2: `driverMinRuntimeBeforeRetry = 5min`**
Sessions that ran for >5 minutes and exited are treated as completions, not crashes. An AutonomousDriver must distinguish "session completed its fix attempt" from "session crashed".

**Guard 3: `retried` flag prevents infinite retry loops**
Only one restart is attempted. After the second failure, `markSessionNeedsAttention` adds to the ReviewQueue. The AutonomousDriver should have a similar max-attempts mechanism (e.g., `maxFixAttempts = 3`).

**Guard 4: Panic recovery**
```go
defer func() {
    if r := recover(); r != nil { log.Error(...) }
}()
```
All driver goroutines must have this recovery wrapper — a panic in a goroutine without recovery kills the entire server.

---

## 6. StatusNeedsApproval Blocking the Loop

`session_driver.go` checks for `StatusNeedsApproval` and auto-approves directory-access prompts:
```go
if si.ClaudeStatus == detection.StatusNeedsApproval {
    if shouldApprovePrompt(output, allowedPath) {
        inst.SendKeys("1\n")
    }
}
```

**Risk for AutonomousDriver**: If `PermissionMode != "bypassPermissions"`, the session could get stuck at a permission dialog the AutonomousDriver doesn't know how to handle. The existing session_driver.go only handles the trust-folder dialog (numbered menu "1\n"). Other tool approval dialogs need the HTTP hook path.

For autonomous fix sessions, the recommended config is:
```go
AllowedTools:   "Bash,Read,Edit,Write,Glob,Grep",
PermissionMode: "bypassPermissions",
AutoYes:        true,
```

All three in combination ensure no interactive prompt can block the driver.

---

## 7. Test Coverage for Controller Behavior

From `session/claude_controller_test.go`, the existing tests only cover:
- `NewClaudeController(nil)` → error
- `NewClaudeController("")` (empty title) → error
- `IsStarted()` before Start → false
- `GetSessionName()` → correct
- `Start()` requires PTY (skipped in unit tests)

**Gap**: No tests for `StatusChangeListener` invocation, `GetIdleState()` returning correct values after status changes, or `SendCommandImmediate` behavior under concurrent calls. The AutonomousDriver will need integration tests using a fake/mock PTY (similar to the pattern in `command_executor_test.go`).

Test files that show patterns to follow:
- `session/approval_automation_test.go` — approval flow mocking
- `session/backlog_lifecycle_test.go` — lifecycle event wiring
- `session/session_driver_test.go` — driver goroutine patterns (idempotency, failure handling)

---

## 8. StatusChangeListener Single-Listener Constraint

**Critical architectural risk**: `Instance.SetStatusChangeCallback` stores a single `fn`. The server layer (`wireStatusChangeCallback` in `session_service.go`) already uses this for metrics/events. When AutonomousDriver tries to set its own listener, it **overwrites** the server's listener.

Current wiring path:
```
session_service.go:wireStatusChangeCallback
    → instance.SetStatusChangeCallback(fn)
    → instance.onStatusChange = fn
    → controller.SetStatusChangeListener(fn)
```

Options in order of safety:
1. **Add `RegisterStatusChangeListener(fn)`** to `ClaudeController` that appends to a `[]StatusChangeListener` — breaks no existing code, cleanest fix
2. **Compose in `wireStatusChangeCallback`** — check if `onStatusChange` is already set, chain if so
3. **Poll `cc.GetIdleState()` in AutonomousDriver** — no listener needed, 2s polling lag acceptable for CI-driven use case

---

## 9. GitHub PR Number / Session Correlation

Sessions with GitHub PR context already have these fields:
```go
GitHubPRNumber   int    // set from PR URL detection on creation
GitHubPRState    string // set by PRStatusPoller
GitHubCheckConclusion string // set by PRStatusPoller ("success"/"failure"/...)
```

The `PRStatusPoller` already polls GitHub and updates these fields. The AutonomousDriver can **react to these field changes** via a storage watch or by polling `inst.GitHubCheckConclusion`. No new GitHub API calls are needed if the poller is already running.

**Risk**: `GitHubCheckConclusion` is polled on an interval (likely 30-120s based on similar pollers). There's a race where the AutonomousDriver acts on stale CI status. Always re-fetch the live PR status from GitHub before deciding to trigger a fix attempt.

---

## 10. `AutonomousMode` Field Is Orphaned

`AutonomousMode bool` is:
- Defined on `Instance` (line 146)
- Serialized to JSON (`json:"autonomous_mode,omitempty"`)
- Stored in `SessionData` struct (storage.go line 65)
- **Never read anywhere** in non-test code

There is no `AutonomousMode` field in `InstanceOptions` or `CreateSessionRequest` proto. Before the AutonomousDriver can use it:
1. Add `AutonomousMode bool` to `InstanceOptions`
2. Pass it through `NewInstance`
3. Add `bool autonomous_mode = NNN` to `CreateSessionRequest` proto
4. Thread it through `CreateSession` RPC handler

This is a clean 4-touchpoint addition, not a breaking change.
