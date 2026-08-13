# Adversarial Review — Session Steering Implementation Plan

**Reviewer**: Adversarial subagent  
**Date**: 2026-05-20  
**Plan reviewed**: `docs/project_plans/session-steering/implementation/plan.md`

---

## VERDICT: BLOCKED

Three blocking issues prevent safe implementation without plan corrections. Additionally, four concerns require resolution before coding begins.

---

## BLOCKING Issues

### BLOCK-1: `driverTotalTimeout` kills the goroutine before inactivity detection can ever fire

**File**: `session/session_driver.go`, lines 24 and 48  
**Evidence**:
```go
driverTotalTimeout  = 10 * time.Minute   // line 24
// ...
if time.Now().After(totalDeadline) {     // line 48 — fires at T+10m
    return
}
```

The plan introduces `driverInactivityTimeout = 10 * time.Minute` (Epic 2, Task 2.1). Inactivity detection can only fire AFTER `sentInitial == true`, which requires at minimum the 2-second poll interval plus the time to reach Ready/timeout. In the worst case, `sentInitial` is set at T+30s (when `readyDeadline` expires). That means inactivity detection would need T+30s + 10min = T+10m30s to fire. But `totalDeadline` fires at T+10m and returns unconditionally.

**Result**: Inactivity detection is dead code as written. The `totalDeadline` exits the driver before any inactivity check can ever trigger.

**Fix**: `driverTotalTimeout` must be increased to at least `driverReadyTimeout + driverInactivityTimeout + margin` — e.g., `25 * time.Minute`. The inactivity timeout should be the main termination mechanism for stuck sessions; the total deadline is a safety net for completely broken sessions, not the first thing to fire. Alternatively, remove `totalDeadline` entirely since inactivity + exit detection now handle termination.

---

### BLOCK-2: `ReviewQueue.Add` signature mismatch — plan assumes `(title, reason string)` but actual signature is `(item *ReviewItem) bool`

**File**: `session/review_queue.go`, lines 60-65  
**Evidence**:
```go
// ReviewQueueWriter is the write-side interface for the review queue.
type ReviewQueueWriter interface {
    Add(item *ReviewItem) bool   // line 64 — takes *ReviewItem, not (string, string)
}
```

The plan's Task 5.2 writes:
```go
rq.Add(inst.Title, reason)   // WRONG — does not match signature
```

`ReviewItem` is a rich struct (`session/queue/queue.go` lines 152–185) requiring fields: `SessionID`, `SessionName`, `Reason` (typed `AttentionReason`, not `string`), `Priority`, `DetectedAt`, etc. The plan provides no guidance on constructing this struct.

**Fix**: Task 5.2 must be rewritten to construct a `*ReviewItem` with appropriate fields:
```go
rq.Add(&ReviewItem{
    SessionID:   inst.UUID,
    SessionName: inst.Title,
    Reason:      ReasonStale,        // or a new reason constant
    Priority:    PriorityMedium,
    DetectedAt:  time.Now(),
    Context:     reason,
    Tags:        inst.GetTags(),
    Path:        inst.Path,
    Status:      inst.GetEffectiveStatus().String(),
})
```

The plan must also add `ReasonSteeredSessionStuck` (or justify reusing `ReasonStale`) as an `AttentionReason` constant in `review_queue.go` — or explicitly state that `ReasonStale` is the correct reuse. This affects the acceptance criterion for AC-7.

---

### BLOCK-3: `msgs[i].Type` field does not exist on `ClaudeConversationMessage` — the field is `Role`

**File**: `session/history.go`, lines 326-331  
**Evidence**:
```go
type ClaudeConversationMessage struct {
    Role      string     // line 327 — "user" or "assistant"
    Content   string
    Timestamp time.Time
    Model     string
}
```

The plan's Task 4.1 code writes:
```go
if msgs[i].Type == "assistant" {   // WRONG — no Type field; correct field is Role
```

This will fail to compile. The correct code is:
```go
if msgs[i].Role == "assistant" {
```

Note: The plan does mention "verify `extractMsgText`" but does not flag this specific field-name error, even though the struct definition is clear. The `Type` field exists on `conversationMessage` (the raw parse struct), not on the exported `ClaudeConversationMessage`. The implementer will hit a compile error.

**Fix**: Replace `msgs[i].Type` with `msgs[i].Role` throughout `buildContinuationPrompt`. Also: `extractMsgText` does not exist anywhere in the codebase. The correct approach is to access `msgs[i].Content` directly (it's already a `string` after `extractMsgContent` processes it).

---

## CONCERNS

### CONCERN-1: `driverRunning` reset in `Restart` and `RecoverFromStopped` creates a window where external callers can race to start a second driver

**Epic**: 6, Task 6.3  
The plan proposes calling `i.driverRunning.Store(false)` at the start of `Restart()` and `RecoverFromStopped()`. However, `Restart()` takes `startMu` (`instance.go:533`), and the period between `Store(false)` and the new goroutine starting (which calls `Store(true)` only if it wins the CAS) is a data race window: another caller can observe `driverRunning == false` and call `StartSessionDriver`, spawning a driver goroutine that runs in parallel with the restart in progress.

The plan's D3 mitigation (set `driverRunning.Store(true)` before spawning in `handleDriverFailure`) only covers the retry path, not external callers racing with `Restart()`.

**Risk level**: Medium. The practical risk is low since no current caller except the driver itself triggers restarts. But the plan's stated invariant (one driver goroutine per instance) is not guaranteed.

**Suggested fix**: Do not reset `driverRunning` inside `Restart()`. Instead, only reset it inside the defer in the `StartSessionDriver` goroutine wrapper. For the retry path, keep the `inst.driverRunning.Store(true)` mitigation in `handleDriverFailure` before spawning the new goroutine.

---

### CONCERN-2: Exit detection retries work sessions that complete normally — `sentInitial == true` is not sufficient to distinguish task completion

**Epic**: 3, Task 3.2  
The plan guards work-session exit retry with `sentInitial == true && isOneShot == false`. But a `backlog:work` session that runs for 45 minutes and completes its task also has `sentInitial == true` when it exits — the driver cannot distinguish "healthy completion" from "crash after work."

The pitfall research (#2) identifies this issue and suggests checking elapsed time since `sentInitial` was set, but the plan's code (Task 3.2) does not implement this check. The only guard against double-processing is `BacklogLifecycleListener.onSessionExited`, which transitions the item to review. If the driver also restarts the session, a second work session starts against the same item, potentially creating two review sessions.

**Suggested mitigation** (not currently in plan): Add a minimum-runtime guard: only retry if `sentInitial` was set less than N minutes ago (e.g., 5 minutes), indicating the session crashed quickly after starting rather than completing a long task. Alternatively, track a `taskCompleted bool` by listening for Claude's standard completion signals in the output.

---

### CONCERN-3: `handleDriverFailure` spawns a goroutine without the panic recovery wrapper

**Epic**: 5/7 interaction  
The goroutine spawned by `handleDriverFailure` (`go runSessionDriverWithPrompt(...)`) does NOT go through the wrapper in `StartSessionDriver` that contains the panic recovery defer (Task 7.1). If the retry driver goroutine panics, it crashes the process.

The plan does not address this interaction. The panic recovery needs to be either:
1. Moved inside `runSessionDriverWithPrompt` (applies to all invocations), or
2. Replicated in `handleDriverFailure`'s goroutine spawn.

---

### CONCERN-4: `RecoverFromStopped` has no-op semantics when called on non-Stopped instance, but `handleDriverFailure` calls it unconditionally before `Start(false)`

**File**: `session/instance_state.go`, lines 144-154  
```go
func (i *Instance) RecoverFromStopped() {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    if i.Status == Stopped {   // no-op if not Stopped
        i.setStatus(Ready)
        i.started = false
    }
}
```

In `handleDriverFailure`, the plan checks `st == Stopped` before calling `RecoverFromStopped()` — that's correct. However, `RecoverFromStopped` sets `started = false`. If `Start(false)` is then called with `firstTimeSetup=false`, it takes the hot-attach path. But a truly stopped session (tmux process killed) has no tmux session to attach to. `DoesSessionExist()` returns false, triggering the cold-start path. This should work but the interaction between `RecoverFromStopped` (which sets `started=false`) and `Start(false)` should be explicitly tested. The existing reconciliation tests cover this path, but the plan's test suite (Epic 8) does not include a test for this specific flow.

**Suggested fix**: Add Task 8.11 — `TestHandleDriverFailure_StoppedSession_RestartFlow` to verify the `RecoverFromStopped → Start(false)` path works on a truly stopped (no tmux) session.

---

## APPROVED Items

The following are correct and well-reasoned:

1. **Epic 1 (MCP wiring)**: One-line fix, correct location, `path` variable is in scope. Matches existing `CreateDirectorySession` pattern exactly.

2. **`isOneShot` design (Epic 3, Task 3.1)**: Tag-based one-shot detection is the right approach. `inst.HasTag()` acquires `stateMutex.RLock()` (instance_tags.go:34) — goroutine-safe. The function correctly excludes triage and review sessions from retry.

3. **`GetEffectiveStatus()` usage**: Correctly identified (pitfall #6) as the safe alternative to reading `inst.Status` directly. All new code uses this method.

4. **`readLastNMessagesFromFile` reuse**: Confirmed to exist at `session/history.go:453`. Package-internal but same package as `session_driver.go`. No re-implementation needed.

5. **`driverRunning atomic.Bool` idempotency guard**: `CompareAndSwap` is the correct primitive for this pattern. Zero-value safe. The guard addresses AC-9.

6. **Panic recovery placement**: Correctly identifies that `recover()` must be inside the goroutine, not the caller.

7. **`retried` as pointer shared between driver generations**: Correct. The first-generation driver creates a local `atomic.Bool`, passes `&retried` to `handleDriverFailure`, which passes it to the second-generation goroutine. Second generation starts with `retried == true` and will not spawn a third.

8. **`ReviewQueueWriter` interface**: Using the interface (not the concrete type) for `markSessionNeedsAttention` is the right abstraction — allows test doubles.

9. **Removing `totalDeadline` removal from the refactored `runSessionDriverWithPrompt`**: The plan implies the driver no longer has a fixed total deadline once inactivity/exit detection is in place. This is correct in principle (the deadline is now the inactivity timeout), though the implementation of removing it is not explicit in the plan.

10. **`buildContinuationPrompt` before restart**: Correctly reads `HistoryFilePath` before calling `Restart()` since `inst.HistoryFilePath` may be cleared or changed after restart.

---

## Summary of Required Plan Patches

| Issue | Type | Action |
|---|---|---|
| BLOCK-1: `driverTotalTimeout` conflict | BLOCKING | Increase `driverTotalTimeout` to 25+ minutes or remove it |
| BLOCK-2: `ReviewQueue.Add` signature | BLOCKING | Rewrite Task 5.2 to construct `*ReviewItem` |
| BLOCK-3: `msgs[i].Type` → `msgs[i].Role` | BLOCKING | Fix field name; remove `extractMsgText` reference |
| CONCERN-1: `driverRunning` reset race | CONCERN | Remove reset from `Restart`/`RecoverFromStopped`; rely on defer |
| CONCERN-2: Normal completion vs crash | CONCERN | Add minimum-runtime guard in exit detection |
| CONCERN-3: Retry goroutine missing panic recovery | CONCERN | Move `recover()` into `runSessionDriverWithPrompt` |
| CONCERN-4: Missing test for `RecoverFromStopped → Start(false)` | CONCERN | Add Task 8.11 |
