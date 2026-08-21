# Pitfalls Research — Session Steering

## 1. Race Condition: Two Goroutines Both Detecting Failure and Triggering Retry

**Problem**: The session driver polls `inst.Status` and `inst.LastMeaningfulOutputTime()` on a ticker. If a second driver goroutine is started (e.g., via a restart), both goroutines can simultaneously see the stuck condition and both call `inst.Restart()`.

**What happens**: `inst.Restart()` calls `i.KillSession()` then `i.start()`. The `start()` method acquires `i.startMu`. The second goroutine will block at `startMu.Lock()` until the first restart completes, then also call `start()` — double restart. The second restart is not inherently dangerous (it just wastes a startup cycle) but will reset the retry counter.

**Mitigation**:
1. Ensure only one driver goroutine exists per session. The current `StartSessionDriver` launches a fire-and-forget goroutine. The driver should set an `atomic.Bool` flag on a per-driver struct before calling restart, and skip if already set:
   ```go
   if !d.restarting.CompareAndSwap(false, true) {
       return // another goroutine already handling restart
   }
   defer d.restarting.Store(false)
   ```
2. The `startMu` on `Instance` serializes `start()` calls, so even if two goroutines race to restart, the second will wait and then run a redundant (harmless) start rather than a corrupt state.

**Key insight**: Since `StartSessionDriver` creates a new goroutine and the old one terminates when `inst.Status == Stopped`, restarting the instance transitions it back to `Running` — the old goroutine would have already returned. The new post-restart driver goroutine should be started AFTER the restart, not before, to avoid overlapping drivers.

---

## 2. The "Double Retry" Problem: BacklogLifecycleListener + Driver Both React to Exits

**Problem**: When a work session exits normally (task complete), `BacklogLifecycleListener.onSessionExited` fires and transitions the backlog item to `review` or `done`, then optionally spawns a review session. If the driver ALSO detects the exit (status transitions to `Stopped`), it may attempt a retry restart, which would:
1. Restart a session that correctly exited after completing its work
2. Create a second work session when only a review session should exist
3. Potentially transition the backlog item back to `in_progress` via `EventStarted`

**Mitigation**:
1. The driver's failure detection must check if the exit was "unexpected" vs "expected". A work session that has been running for a while and then exits normally is NOT a failure.
2. The existing `intentionalStop` flag on `TmuxSession` distinguishes operator-initiated stops from crashes — but this is for the control-mode exit callback, not the driver.
3. **Best approach**: The driver should only retry if the session exited BEFORE sending the initial prompt. If the session ran for more than N minutes and then exited, that's a completion, not a failure. Track whether `sentInitial` was true when the exit was detected, AND track the elapsed time since initial prompt was sent.
4. Alternatively: tag sessions by type (triage, work, review) and only apply retry logic to work sessions. Triage and review sessions are one-shot and should NOT be restarted.

**Critical guard**: `BacklogLifecycleListener.onSessionExited` already has a guard: `if is.SessionRole != SessionRoleWork { return }`. The driver must have an analogous guard: don't retry one-shot sessions (triage, review) that are tagged `backlog:triage` or `backlog:review`.

---

## 3. JSONL Path Resolution Failure Modes

### 3a. Session Not Yet Written
Claude Code does not create the JSONL file until the first message exchange is complete. A session that exits before any prompt is sent will have no JSONL file. `inst.HistoryFilePath` will be empty.

**Mitigation**: If `inst.HistoryFilePath == ""` on restart, use the plain continuation prompt without JSONL context. Do NOT fail — silently degrade to the simple prompt.

### 3b. Multiple JSONL Files in the Project Directory
If a project directory has been used for multiple conversations, `DetectByPath` picks the most recently modified `.jsonl` file. This is correct for the current session. No action needed.

However, if the session was restarted and got a new UUID, `DetectByPath` would correctly pick the new file. If `--resume` was used, there's only one active file. The existing `HistoryLinker` handles this correctly via `DetectByPath`.

### 3c. Hash Algorithm Unknown / Wrong
The path encoding `ClaudeProjectDirName` replaces every non-alphanumeric char with `-`. This is already implemented and tested in `session/history_detector.go`. **Do not re-implement this logic.** Always call `ClaudeProjectDirName(effectivePath)` where `effectivePath = inst.GetEffectiveRootDir()`.

Edge case: worktree sessions use `GetEffectiveRootDir()` which returns the worktree path (different from the main repo path). The JSONL is keyed to the worktree path, not the main repo. This is handled correctly by `HistoryLinker.correlateSession` which calls `inst.GetEffectiveRootDir()`.

### 3d. Race Between HistoryLinker and Driver Reading the File
`inst.HistoryFilePath` is set by `HistoryLinker` from a goroutine that runs every 5 seconds. If the driver reads `inst.HistoryFilePath` immediately after a restart, it might be empty until the next HistoryLinker scan cycle.

**Mitigation**: The driver should not read `HistoryFilePath` immediately on restart. Wait until after `sentInitial` would normally be set (the ready/timeout detection path) — by that time, ~30 seconds have elapsed, and HistoryLinker will have had multiple scan cycles.

---

## 4. Inactivity False Positives: Long-Running Computation With No Output

**Problem**: Claude Code may spend 10+ minutes generating code or running tests without producing visible terminal output. For example:
- Compiling a large Go project
- Running a slow test suite
- Generating a large file diff via a tool call

If the driver triggers stuck detection at 10 minutes, it would incorrectly restart a healthy session.

**Mitigation**:
1. Check `inst.Status`: if `inst.Status == Running`, Claude is actively working — do NOT trigger stuck detection. The `Running` status indicates the terminal is changing (Claude is producing output). `LastMeaningfulOutput` may lag if the controller's content sampling hasn't detected the change yet, but `Status == Running` is the more reliable signal.
2. Only trigger stuck detection if `inst.Status == Ready` (waiting at `>` prompt) for more than 10 minutes. Ready means Claude has finished its last response and is awaiting input.
3. Consider the `NeedsApproval` status: a session in `NeedsApproval` is also not stuck — it's waiting for the driver to approve the prompt. The driver already handles this case.

**Safe stuck condition**:
```go
stuck := inst.Status == Ready && time.Since(inst.LastMeaningfulOutputTime()) > 10*time.Minute
```

This fires only when Claude is waiting at the prompt AND hasn't had meaningful output for 10 minutes — a much more precise signal than time-since-exit alone.

---

## 5. Tmux Session Name Conflicts on Restart

**Problem**: `inst.Restart()` kills the tmux session and creates a new one with the same `inst.Title`. If the original tmux session is still in the process of being killed (async cleanup), the new session creation may conflict.

**What actually happens** (from `Restart` implementation, `instance.go:893-1006`):
1. `i.KillSession()` — kills tmux session synchronously
2. Creates a new `TmuxSession` with the same title via `tmux.NewTmuxSessionWithPrefix(i.Title, ...)`
3. Calls `tmuxManager.Start(worktreePath)` — creates the tmux session

The `tmux new-session` command with a duplicate name returns an error ("duplicate session: <name>"). This is caught by `Start()` and returned as an error. The `Restart()` method would return an error, and the driver would need to handle this.

**Mitigation**: 
1. `inst.Restart()` already handles this by killing first (synchronous kill), then recreating. If tmux kill is truly synchronous, there should be no conflict.
2. The existing `TmuxSessionExists()` check in `start()` guards against creating a session that already exists.
3. If a restart races with an ongoing kill, the worst outcome is a `Restart()` error — the driver should log and retry after a brief sleep.

---

## 6. Instance Type Thread-Safety: Is Status Read/Write Safe From Multiple Goroutines?

**Status field access analysis**:

The `Status` field is a plain `int` (type alias `Status int`). In Go, reads/writes of pointer-sized or smaller types are not guaranteed to be atomic without explicit synchronization.

**How it's protected in practice**:
- All Status **mutations** go through `transitionTo()` which requires `stateMutex` held: `instance_state.go:21`
- `MarkNeedsApproval()`, `MarkReady()` etc. all acquire `stateMutex.Lock()` before calling `transitionTo()`
- The control-mode exit callback acquires `stateMutex.Lock()` before checking/mutating status: `instance.go:555`

**How the driver reads Status**:
The current `session_driver.go` reads `inst.Status` WITHOUT a lock (lines 53, 76-77, 102). This is technically a data race. However:
- On modern amd64 hardware, reads of int-sized values are atomic at the hardware level
- The Go race detector would flag this, but it doesn't cause corruption in practice
- The codebase's `GetEffectiveStatus()` method does acquire the lock: `instance_state.go:94`

**Recommendation for new driver code**: Use `inst.GetEffectiveStatus()` (which acquires `stateMutex.RLock()`) instead of reading `inst.Status` directly. This follows the pattern used in `health.go`, `storage.go`, and other safe callers. However, be aware that holding `stateMutex.RLock()` while calling `inst.SendKeys()` would deadlock since `SendKeys` also may need the lock — keep the read and the send as separate operations.

---

## 7. What Happens When `inst.Start()` Is Called on an Already-Started Instance

From `instance.go:start()` (lines 529-534):
```go
i.startMu.Lock()
defer i.startMu.Unlock()
```

`start()` acquires `startMu` but does NOT check if the instance is already running. If called on a running instance:
1. `i.initTmuxSession()` — reinitializes the `TmuxProcessManager` (overwrites existing state)
2. `i.tmuxManager.ResetExitOnce()` — resets the exit callback guard
3. If `!firstTimeSetup && !i.tmuxManager.DoesSessionExist()` — if the session IS alive, it calls `tmuxManager.RestoreWithWorkDir` (hot-attach) rather than `Start`
4. If the session IS alive, it reattaches the PTY and restarts the controller

**Conclusion**: Calling `Start(false)` on a running instance is relatively safe — it hot-attaches to the existing tmux session. Calling `Start(true)` on a running instance would attempt to set up the worktree again, which could fail or conflict.

**For the driver restart path**: Always call `inst.Restart()` (not `inst.Start()`) when the session is in a bad state and needs to be restarted. `Restart()` explicitly kills first. Only use `inst.RecoverFromStopped()` + `inst.Start(false)` for the specific case where `inst.Status == Stopped` but `inst.TmuxSessionExists() == true` (the reconciliation path already handled in `BuildRuntimeDeps`).

---

## 8. `BacklogLifecycleListener` Is Called Synchronously

From `instance_controller.go:122`:
```go
l.OnLifecycleEvent(event, reason) // synchronous call
```

But `instanceBacklogListener.OnLifecycleEvent` immediately dispatches to a goroutine:
```go
case EventExited:
    go il.parent.onSessionExited(il.instanceUUID)
```

So the lifecycle listener itself is safe. However, the driver restart should NOT happen inside an `OnLifecycleEvent` callback for a different reason: the callback fires while `stateMutex` may be held (the exit callback in `instance.go:555` holds `stateMutex.Lock()`). Calling `inst.Restart()` from within the listener would deadlock because `Restart()` calls `inst.StopController()` which acquires `stateMutex`.

**Mitigation**: The driver is a separate polling goroutine and detects exit by observing `inst.Status == Stopped` on its next tick — it does NOT use `RegisterLifecycleListener`. No deadlock risk. The driver's restart is issued from its own goroutine, not from the lifecycle callback.
