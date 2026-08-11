# Session Steering — Implementation Plan

**Date**: 2026-05-20
**Status**: Patched after adversarial review (BLOCKED → CONCERNS resolved)
**Epics**: 8 | **Tasks**: 24

### Patch log (adversarial review fixes)
- BLOCK-1: `driverTotalTimeout` increased to 25 min; inactivity timeout is now the real termination mechanism
- BLOCK-2: `ReviewQueue.Add` corrected to take `*ReviewItem`; `markSessionNeedsAttention` rewritten
- BLOCK-3: `msgs[i].Type` → `msgs[i].Role`; `extractMsgText` removed (use `.Content` directly)
- CONCERN-1: `driverRunning` reset removed from `Restart`/`RecoverFromStopped`; rely on defer in wrapper
- CONCERN-2: Minimum-runtime guard added to exit detection to avoid retrying completed sessions
- CONCERN-3: Panic recovery moved inside `runSessionDriverWithPrompt` (covers all call paths)
- CONCERN-4: Task 8.11 added for `RecoverFromStopped → Start(false)` flow

---

## Overview

Session Steering adds supervision goroutines to every automated session so they:
1. Answer Claude Code startup dialogs automatically (already partially done)
2. Send an initial task prompt once Claude reaches the `>` prompt
3. Detect inactivity (stuck at `Ready` for 10 min) and unexpected exits (status → `Stopped`)
4. Auto-restart once with a JSONL continuation prompt
5. Mark the session `NeedsApproval` (existing status) or a new `StatusNeedsAttention` after two failures, so the UI surfaces it

Key constraint: all logic lives in Go goroutines in the `session/` package; no persistent Claude coordinator session.

---

## Architecture Summary

```
StartSessionDriver(inst, allowedPath)
    └── go runSessionDriver(inst, allowedPath)
            ├── startup dialog loop (existing)
            ├── initial prompt dispatch (existing)
            ├── NeedsApproval auto-approve (existing)
            ├── [NEW] inactivity detector (Epic 2)
            └── [NEW] unexpected-exit detector (Epic 3)
                    └── on failure → buildContinuationPrompt (Epic 4)
                                  → retry logic (Epic 5)
                                  → mark NeedsAttention if retry fails
```

The watchdog coordinator (FR-8) is a `SessionWatchdog` struct (Epic 6's idempotency lives on the driver side; the watchdog handles cross-session coordination at a higher level). However, given that the driver per-session is self-sufficient with retry logic, the watchdog's scope is narrowed to: registration, `ReviewQueue.Add` for attention signaling, and preventing double-driver starts.

---

## Epic 1: Wire MCP Sessions

**Goal**: Every session created via MCP `create_session` gets a `SessionDriver` goroutine.

**Research basis**: Architecture research confirms the ONLY remaining gap is `server/mcp/tools_lifecycle.go:createSession`. All backlog session paths go through `CreateDirectorySession` which already calls `StartSessionDriver` at line 474.

### Task 1.1: Add `StartSessionDriver` call in `createSession`

| Field | Value |
|---|---|
| **File** | `server/mcp/tools_lifecycle.go` |
| **Function** | `(lh *lifecycleHandlers) createSession` |
| **Where** | After `inst.Start(true)` succeeds (line ~174), before MCP injection |
| **Change** | Add one line: `session.StartSessionDriver(inst, path)` |
| **Rationale** | Matches the exact pattern in `session_service.go:474`. The `path` variable is already in scope and is the resolved absolute path. |
| **Acceptance** | `AC-4`: A new MCP-created session automatically answers the trust-folder dialog and sends the initial prompt without human input. |

**Implementation note**: `path` in `createSession` is the validated absolute path already confirmed with `os.Stat`. It is the correct `allowedPath` for directory-access approval dialogs.

---

## Epic 2: Inactivity Detection in SessionDriver

**Goal**: Detect when a session is stuck at `Ready` with no output change for 10 minutes after the initial prompt was sent.

**Research basis**:
- `inst.LastMeaningfulOutputTime()` exists in `instance_state.go` and is goroutine-safe (acquires `stateMutex.RLock()`)
- Stuck detection must only fire on `status == Ready`, NOT `Running` (avoids false positives during long compilations — pitfall #4)
- Must only fire after `sentInitial == true` (don't fire during startup)
- Must use `inst.GetEffectiveStatus()` not `inst.Status` directly (pitfall #6, race condition)

### Task 2.1: Add inactivity constants and increase total-deadline safety net

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Where** | Constants block at top |
| **Change** | Add `driverInactivityTimeout = 10 * time.Minute` AND change `driverTotalTimeout` from `10 * time.Minute` to `25 * time.Minute` |
| **Rationale** | BLOCK-1 fix: `driverTotalTimeout` was also 10 minutes. Inactivity detection can only fire AFTER `sentInitial` is set (at most 30s after start), meaning it needs T+10m30s at minimum to trigger — but the old 10-min deadline fired first, making inactivity detection dead code. The 25-min deadline now acts as a safety net for completely unresponsive sessions; inactivity detection is the actual termination mechanism. |
| **Acceptance** | `driverTotalTimeout > driverReadyTimeout + driverInactivityTimeout`. Both constants are in one place. |

### Task 2.2: Add stuck detection branch in `runSessionDriver`

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Function** | `runSessionDriver` |
| **Where** | Inside the `for range ticker.C` loop, after the `if !sentInitial` block (i.e., only runs when `sentInitial == true`) |
| **Change** | Add: |

```go
// Inactivity detection: only after initial prompt sent.
// Use GetEffectiveStatus() (acquires stateMutex.RLock) to avoid data race on Status field.
if sentInitial {
    st := inst.GetEffectiveStatus()
    if st == Ready {
        last := inst.LastMeaningfulOutputTime()
        if !last.IsZero() && time.Since(last) > driverInactivityTimeout {
            log.Warn("SessionDriver: session stuck — no output for inactivity timeout",
                "session", inst.Title,
                "inactivity", time.Since(last).Round(time.Second),
            )
            handleDriverFailure(inst, allowedPath, &retried, "inactivity timeout")
            return
        }
    }
}
```

**Note**: `retried` is an `atomic.Bool` introduced in Epic 5. The call to `handleDriverFailure` is the shared restart path defined in Epic 5. The `return` exits this driver goroutine; a new one is started by `handleDriverFailure` after restart.

| **Acceptance** | AC-6: A session that produces no output for 10 minutes is restarted exactly once. |

---

## Epic 3: Exit Detection in SessionDriver

**Goal**: Detect when a session's process exits unexpectedly (`status == Stopped`) after the initial prompt was sent. Skip one-shot sessions (triage, review) that should NOT be retried.

**Research basis**:
- Pitfall #2: `BacklogLifecycleListener` reacts to `EventExited` for normal completion. The driver must NOT double-restart sessions that completed their task normally.
- Guard: only retry sessions tagged `backlog:work` or `source:mcp`. Sessions tagged `backlog:triage` or `backlog:review` are one-shot.
- The driver detects exit by polling `inst.GetEffectiveStatus() == Stopped` — NOT via lifecycle callbacks (avoids the deadlock risk in pitfall #8).
- An exit is "unexpected" if `sentInitial == true` AND the session stopped. An exit before `sentInitial` (startup crash) is also a candidate for restart.

### Task 3.1: Add `isOneShot` helper function

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Function** | New function `isOneShot(inst *Instance) bool` |
| **Change** | |

```go
// isOneShot returns true for sessions that should NOT be auto-retried.
// Triage and review sessions run exactly once; retrying them could corrupt
// backlog state by re-triggering lifecycle transitions.
func isOneShot(inst *Instance) bool {
    return inst.HasTag("backlog:triage") || inst.HasTag("backlog:review")
}
```

| **Acceptance** | `isOneShot` returns `true` for triage/review sessions, `false` for work and MCP sessions. Unit tested in Epic 8. |

### Task 3.2: Add exit detection branch in `runSessionDriver`

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Function** | `runSessionDriver` |
| **Where** | Replace the existing terminal-state check (`if inst.Status == Stopped || inst.Status == Paused`) with a richer version at the top of the loop |
| **Change** | |

```go
st := inst.GetEffectiveStatus()

// Paused is always a clean stop for the driver.
if st == Paused {
    return
}

// Stopped after sentInitial = potential unexpected exit.
if st == Stopped {
    if !sentInitial {
        // Exited before we even sent the first prompt — likely a startup crash.
        // For one-shot sessions or if we've already retried, just exit.
        if isOneShot(inst) || retried.Load() {
            return
        }
        log.Warn("SessionDriver: session exited before initial prompt sent",
            "session", inst.Title,
        )
        handleDriverFailure(inst, allowedPath, &retried, "exit before initial prompt")
        return
    }
    // Stopped after initial prompt was sent.
    if isOneShot(inst) || retried.Load() {
        // One-shot sessions: BacklogLifecycleListener handles this; driver exits cleanly.
        return
    }
    // CONCERN-2 guard: only restart if the session crashed quickly (within 5 minutes
    // of sending the initial prompt). A session that ran for > 5 minutes and then stopped
    // has likely completed its task normally. BacklogLifecycleListener handles that transition.
    if time.Since(initialPromptSentAt) > driverMinRuntimeBeforeRetry {
        log.Info("SessionDriver: session exited after minimum runtime, treating as completion",
            "session", inst.Title,
            "runtime", time.Since(initialPromptSentAt).Round(time.Second),
        )
        return
    }
    log.Warn("SessionDriver: unexpected session exit after initial prompt",
        "session", inst.Title,
    )
    handleDriverFailure(inst, allowedPath, &retried, "unexpected exit")
    return
}
```

**Additional constants and variables needed for Task 3.2**:
- Add constant `driverMinRuntimeBeforeRetry = 5 * time.Minute` to the constants block
- Add local variable `initialPromptSentAt time.Time` in `runSessionDriverWithPrompt`, initialized when `sentInitial` is set to `true`:
  ```go
  sentInitial = true
  initialPromptSentAt = time.Now()
  ```

**Note**: The existing `totalDeadline` check is preserved (now with the corrected 25-min value from Task 2.1). The per-loop `inst.Status` read is replaced by the `st := inst.GetEffectiveStatus()` call above. The `retried` var is an `atomic.Bool` local variable in `runSessionDriver` (not a field on a struct, since there is only one driver goroutine per instance — see Epic 6 for idempotency guard).

| **Acceptance** | AC-5: Unexpected exit triggers restart exactly once. One-shot sessions (triage/review) exit the driver cleanly without restart. Sessions that ran for more than 5 minutes are treated as completed — no restart. |

---

## Epic 4: JSONL Continuation Prompt Builder

**Goal**: Build a `"here's where you left off"` prompt from the session's JSONL conversation history to send when restarting.

**Research basis**:
- `readLastNMessagesFromFile` exists in `session/history.go:453`. It is package-internal but accessible from `session_driver.go` (same package).
- `inst.HistoryFilePath` is populated by `HistoryLinker` within ~5 seconds. On restart, there will have been enough time for HistoryLinker to populate it.
- Pitfall #3a: If `inst.HistoryFilePath == ""`, degrade gracefully to a simple prompt.
- The continuation prompt should be brief: inject the last assistant message (truncated to 500 chars) as context, then "Please continue."

### Task 4.1: Add `buildContinuationPrompt` function

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Function** | New function `buildContinuationPrompt(inst *Instance) string` |
| **Change** | |

```go
const (
    driverContinuationMaxMessages = 10
    driverContinuationMaxChars    = 500
)

// buildContinuationPrompt reads the last N messages from the session's JSONL
// conversation log and produces a brief prompt summarizing the last assistant
// turn. Falls back to a generic prompt if the log is unavailable.
func buildContinuationPrompt(inst *Instance) string {
    histPath := inst.HistoryFilePath
    if histPath == "" {
        return "Your previous session exited unexpectedly. Please continue from where you left off."
    }

    msgs, err := readLastNMessagesFromFile(histPath, driverContinuationMaxMessages)
    if err != nil || len(msgs) == 0 {
        log.Warn("SessionDriver: could not read conversation history for continuation prompt",
            "session", inst.Title, "path", histPath, "err", err,
        )
        return "Your previous session exited unexpectedly. Please continue from where you left off."
    }

    // Find the last assistant message.
    // BLOCK-3 fix: ClaudeConversationMessage has a Role field (not Type).
    // Content is already a plain string after extractMsgContent processing — no
    // extractMsgText helper needed or available.
    var lastAssistant string
    for i := len(msgs) - 1; i >= 0; i-- {
        if msgs[i].Role == "assistant" {
            lastAssistant = msgs[i].Content
            break
        }
    }

    if lastAssistant == "" {
        return "Your previous session exited unexpectedly. Please continue from where you left off."
    }

    if len(lastAssistant) > driverContinuationMaxChars {
        lastAssistant = lastAssistant[:driverContinuationMaxChars] + "..."
    }

    return "Your session restarted after an unexpected exit. Your last message was:\n---\n" +
        lastAssistant + "\n---\nPlease continue from where you left off. " +
        "Do not re-introduce yourself or repeat completed work."
}
```

**Source verification** (confirmed from `session/history.go:326-331`):
- `ClaudeConversationMessage` has fields: `Role string`, `Content string`, `Timestamp time.Time`, `Model string`
- `Role` is `"user"` or `"assistant"` (set by `extractMsgContent` from `raw.Message.Role`)
- `Content` is already a flat string — no further extraction helper needed
- `extractMsgText` does NOT exist in the codebase; do not use it

| **Acceptance** | `buildContinuationPrompt` returns a non-empty string in all cases. When JSONL is available, the prompt includes the last assistant message content. Unit tested in Epic 8. |

---

## Epic 5: Auto-Retry Logic in SessionDriver

**Goal**: On first failure (stuck or unexpected exit), restart the session and send the continuation prompt. On second failure, add to `ReviewQueue` and stop the driver.

**Research basis**:
- Pitfall #1: Use `atomic.Bool restarting` with `CompareAndSwap` before restart to guard against double-restart. Since there is only one driver goroutine per instance (Epic 6), the primary race is the driver detecting BOTH stuck AND exit simultaneously — but the loop structure processes one tick at a time, so `retried.Load()` is sufficient.
- `inst.Restart(false)` is the correct call for a running session restart (kills then recreates). For `Stopped` instances: `inst.RecoverFromStopped()` then `inst.Start(false)` per pitfall #7 and the existing reconciliation pattern.
- The new driver goroutine for the restarted session must be started AFTER restart completes (pitfall #1 key insight: old goroutine terminates, new goroutine starts).
- `ReviewQueue.Add` (via `inst.GetReviewQueue()`) is the notification path for "needs attention" — no new status constant needed for MVP.

### Task 5.1: Add `retried` flag and `handleDriverFailure` function

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Change** | Add a `retried atomic.Bool` local variable in `runSessionDriver` (not on a struct — the goroutine is the owner). Add shared helper `handleDriverFailure`. |

```go
// In runSessionDriver, add at top:
var retried atomic.Bool

// New function:

// handleDriverFailure is called when the driver detects a stuck or crashed session.
// On first call (retried == false): restarts the session and starts a new driver
//   goroutine with the continuation prompt.
// On second call (retried == true): adds the session to the ReviewQueue and exits.
//
// The caller must return immediately after calling handleDriverFailure.
func handleDriverFailure(inst *Instance, allowedPath string, retried *atomic.Bool, reason string) {
    if !retried.CompareAndSwap(false, true) {
        // Already retried once. Mark for human attention.
        log.Warn("SessionDriver: session failed twice; marking for attention",
            "session", inst.Title, "reason", reason,
        )
        markSessionNeedsAttention(inst, reason)
        return
    }

    log.Info("SessionDriver: restarting session after failure",
        "session", inst.Title, "reason", reason,
    )

    // Build continuation prompt BEFORE restart (HistoryFilePath may clear after restart).
    continuationPrompt := buildContinuationPrompt(inst)

    // Restart the session.
    var restartErr error
    st := inst.GetEffectiveStatus()
    if st == Stopped {
        if recoverErr := inst.RecoverFromStopped(); recoverErr != nil {
            log.Warn("SessionDriver: RecoverFromStopped failed",
                "session", inst.Title, "err", recoverErr,
            )
        }
        restartErr = inst.Start(false)
    } else {
        restartErr = inst.Restart(false)
    }

    if restartErr != nil {
        log.Error("SessionDriver: restart failed; marking for attention",
            "session", inst.Title, "err", restartErr,
        )
        markSessionNeedsAttention(inst, "restart error: "+restartErr.Error())
        return
    }

    // Start a new driver goroutine for the restarted session.
    // The new goroutine inherits the retried flag so it will not retry a second time.
    go runSessionDriverWithPrompt(inst, allowedPath, continuationPrompt, retried)
}
```

### Task 5.2: Add `markSessionNeedsAttention` helper

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Change** | |

```go
// markSessionNeedsAttention adds the instance to its ReviewQueue (if any)
// so the UI surfaces it for operator intervention.
//
// BLOCK-2 fix: ReviewQueue.Add takes *ReviewItem, not (string, string).
// The ReviewQueueWriter interface (session/review_queue.go:63) confirms:
//   Add(item *ReviewItem) bool
func markSessionNeedsAttention(inst *Instance, reason string) {
    rq := inst.GetReviewQueue()
    if rq == nil {
        log.Warn("SessionDriver: ReviewQueue not set on instance, cannot mark NeedsAttention",
            "session", inst.Title,
        )
        return
    }
    rq.Add(&ReviewItem{
        SessionID:   inst.UUID,
        SessionName: inst.Title,
        Reason:      ReasonStale, // closest existing reason: "no output for extended period"
        Priority:    PriorityMedium,
        DetectedAt:  time.Now(),
        Context:     reason,    // "inactivity timeout" or "unexpected exit" or "restart error: ..."
        Tags:        inst.GetTags(),
        Path:        inst.Path,
        Status:      inst.GetEffectiveStatus().String(),
        LastActivity: inst.LastMeaningfulOutputTime(),
    })
}
```

**Source verification** (confirmed from `session/queue/queue.go:211`):
- `ReviewQueue.Add(item *ReviewItem) bool`
- `ReviewQueueWriter` interface also specifies `Add(item *ReviewItem) bool` (review_queue.go:64)
- `ReviewItem.Reason` is typed `AttentionReason` (a string alias) — `ReasonStale` = `"stale"` is the appropriate existing constant for "no output for extended period (may be stuck)"
- `ReviewItem.SessionID` should be `inst.UUID` (stable ID), not `inst.Title` (mutable)
- `time` import is already present in session_driver.go

**No new `AttentionReason` constant needed**: `ReasonStale` maps directly to "no output for extended period (may be stuck)" which is the semantics needed here.

### Task 5.3: Add `runSessionDriverWithPrompt` function (with inline panic recovery)

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Change** | Extract `runSessionDriver` into a parametric form that accepts a custom initial prompt and a pre-set `retried` flag. Move panic recovery INSIDE `runSessionDriverWithPrompt` (CONCERN-3 fix). |

```go
func runSessionDriver(inst *Instance, allowedPath string) {
    var retried atomic.Bool
    runSessionDriverWithPrompt(inst, allowedPath, driverInitialPrompt, &retried)
}

func runSessionDriverWithPrompt(inst *Instance, allowedPath string, initialPrompt string, retried *atomic.Bool) {
    // CONCERN-3 fix: panic recovery is inside this function (not only in the
    // StartSessionDriver wrapper) so that the retry goroutine spawned by
    // handleDriverFailure is also protected.
    defer func() {
        if r := recover(); r != nil {
            log.Error("SessionDriver: panic recovered in driver goroutine",
                "session", inst.Title,
                "panic", r,
            )
        }
    }()

    // ... all existing logic, using initialPrompt instead of driverInitialPrompt ...
    // ... using retried for exit/stuck detection ...
    // ... initialPromptSentAt tracked per Task 3.2 ...
}
```

**The `retried` pointer is passed in** from the calling `handleDriverFailure` so the second-generation driver goroutine already has `retried == true` and will not spawn a third generation.

| **Acceptance** | AC-5, AC-6, AC-7: Session restarts once with continuation prompt; second failure marks session for attention and stops driver. AC-10: panic recovery protects BOTH initial and retry driver goroutines. |

---

## Epic 6: Idempotency Guard on StartSessionDriver

**Goal**: Calling `StartSessionDriver` twice on the same instance must not spawn two driver goroutines.

**Research basis**:
- `sync.Once` cannot be used because the session can be Restarted (which needs a fresh driver); `sync.Once` fires only once per value lifetime.
- An `atomic.Bool driverStarted` field on `Instance` is the right approach — it can be reset by `RecoverFromStopped` or `Restart`.
- Pitfall check: the `startMu` on `Instance` already serializes `start()` calls, but does NOT prevent two `StartSessionDriver` calls from both checking `driverStarted` as false simultaneously. The `CompareAndSwap` on `atomic.Bool` is the safe solution.

### Task 6.1: Add `driverRunning atomic.Bool` to `Instance`

| Field | Value |
|---|---|
| **File** | `session/instance.go` |
| **Where** | In the `Instance` struct, near the `startMu` field |
| **Change** | Add `driverRunning atomic.Bool` |
| **Note** | This field is zero-value safe (false = no driver running). |

### Task 6.2: Guard `StartSessionDriver` with CompareAndSwap

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Function** | `StartSessionDriver` |
| **Change** | |

```go
func StartSessionDriver(inst *Instance, allowedPath string) {
    if !inst.driverRunning.CompareAndSwap(false, true) {
        log.Debug("SessionDriver: driver already running for session, skipping duplicate start",
            "session", inst.Title,
        )
        return
    }
    go func() {
        defer inst.driverRunning.Store(false)
        runSessionDriver(inst, allowedPath)
    }()
}
```

### Task 6.3: Do NOT reset `driverRunning` in `Restart` or `RecoverFromStopped`

| Field | Value |
|---|---|
| **File** | `session/instance.go` |
| **Functions** | `Restart`, `RecoverFromStopped` |
| **Change** | **No change to these functions.** |
| **Rationale** | CONCERN-1 fix: Adding `i.driverRunning.Store(false)` to `Restart()` creates a race window. Between the Store(false) and the CAS in `StartSessionDriver`, an external caller could observe `driverRunning == false` and spawn a second driver while the restart is in progress. The practical risk is low (no current external callers trigger `Restart()`), but the invariant would be broken. Instead: the driver goroutine's `defer inst.driverRunning.Store(false)` fires when the goroutine exits naturally. The retry path in `handleDriverFailure` sets `inst.driverRunning.Store(true)` before spawning to close the race window between the old goroutine exiting and the new one starting. No changes to Instance are needed. |

| **Acceptance** | AC-9: Two `StartSessionDriver` calls on the same instance do not spawn two driver goroutines. The guard is entirely in `StartSessionDriver` + `handleDriverFailure`; Instance methods are not modified. |

---

## Epic 7: Panic Recovery in `runSessionDriver` Goroutine

**Goal**: A panic in one session's driver goroutine must not crash the server.

**Research basis**:
- Requirements NFR: "Crash isolation — a panic in one session's driver goroutine must not affect other sessions; recover with a log."
- Standard Go pattern: `defer func() { if r := recover(); r != nil { log.Error(...) } }()` at the top of the goroutine function.

### Task 7.1: Panic recovery in `StartSessionDriver` wrapper AND `runSessionDriverWithPrompt`

| Field | Value |
|---|---|
| **File** | `session/session_driver.go` |
| **Function** | Both the anonymous wrapper in `StartSessionDriver` (Task 6.2) and `runSessionDriverWithPrompt` (Task 5.3) |
| **Change** | |

**Primary recovery** (in `runSessionDriverWithPrompt` — covers initial AND retry goroutines):
Already described in Task 5.3. The `defer recover()` at the top of `runSessionDriverWithPrompt` is the main protection.

**Secondary: wrapper in `StartSessionDriver`** (driverRunning cleanup only):
```go
go func() {
    defer inst.driverRunning.Store(false)
    runSessionDriver(inst, allowedPath)
}()
```

The `driverRunning.Store(false)` defer does NOT need a `recover()` here because `runSessionDriverWithPrompt` already recovers panics internally. If `runSessionDriver` panics (bypassing `runSessionDriverWithPrompt`'s recover — e.g., a panic in the thin wrapper itself), `driverRunning.Store(false)` still fires correctly via the defer, preventing a permanent lock.

**Note**: The retry goroutine spawned by `handleDriverFailure` (`go runSessionDriverWithPrompt(...)`) does NOT go through the `StartSessionDriver` wrapper, so it does NOT have `driverRunning.Store(false)` in a defer. This is acceptable: when the retry goroutine exits normally or via recovered panic, `driverRunning` remains `true` until the outer goroutine's defer fires. To keep the accounting correct, `handleDriverFailure` should set `inst.driverRunning.Store(true)` before spawning and the outer goroutine's defer will set it to `false` after both goroutines complete (outer returns after calling `handleDriverFailure`).

| **Acceptance** | AC-10: A panic inside a driver goroutine — in either the first-generation or retry goroutine — is recovered and logged without crashing the server. |

---

## Epic 8: Tests

**Goal**: Unit and integration test coverage for all new logic.

**Test files**:
- `session/session_driver_test.go` — existing file, add new test cases
- `session/session_driver_continuation_test.go` — new file for Epic 4 tests (JSONL reading)

### Task 8.1: Test `isOneShot`

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestIsOneShot` |
| **Cases** | `backlog:triage` → true; `backlog:review` → true; `backlog:work` → false; `source:mcp` → false; no tags → false |

### Task 8.2: Test `buildContinuationPrompt` — no history file

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestBuildContinuationPrompt_NoHistoryFile` |
| **Setup** | Instance with `HistoryFilePath == ""` |
| **Assert** | Returns generic fallback prompt containing "Please continue" |

### Task 8.3: Test `buildContinuationPrompt` — with JSONL

| Field | Value |
|---|---|
| **File** | `session/session_driver_continuation_test.go` |
| **Test** | `TestBuildContinuationPrompt_WithJSONL` |
| **Setup** | Write a temp JSONL file with 3 user+assistant turns; set `inst.HistoryFilePath` to that file |
| **Assert** | Returns prompt containing truncated last assistant message |

### Task 8.4: Test `buildContinuationPrompt` — truncation

| Field | Value |
|---|---|
| **File** | `session/session_driver_continuation_test.go` |
| **Test** | `TestBuildContinuationPrompt_TruncatesLongMessage` |
| **Setup** | Write JSONL with one assistant message of 1000 chars |
| **Assert** | Returned prompt contains "..." and last char before "..." is the 500th character of the message |

### Task 8.5: Test `StartSessionDriver` idempotency

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestStartSessionDriver_Idempotent` |
| **Setup** | Create minimal Instance; call `StartSessionDriver` twice concurrently |
| **Assert** | `inst.driverRunning` is true (one goroutine) after first call; second call returns immediately; after the goroutine exits, `driverRunning` is false |
| **Note** | Use a mock/stub driver or fast-exit condition (e.g., set `inst.Status = Stopped` before calling) so the goroutine exits quickly |

### Task 8.6: Test inactivity detection — does not fire while Running

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestSessionDriver_Inactivity_NoFireWhileRunning` |
| **Setup** | Mock instance with `Status = Running`, `LastMeaningfulOutputTime` 20 min ago |
| **Assert** | Driver does not call restart/failure handler |
| **Note** | This test validates the pitfall #4 guard — long compilation should not trigger stuck detection |

### Task 8.7: Test inactivity detection — fires when Ready + old output

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestSessionDriver_Inactivity_FiresWhenReadyAndStale` |
| **Setup** | Mock instance with `Status = Ready`, `LastMeaningfulOutputTime` 11 min ago, `sentInitial = true` |
| **Assert** | Driver calls `handleDriverFailure` with reason `"inactivity timeout"` |

### Task 8.8: Test exit detection — one-shot session does not restart

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestSessionDriver_ExitDetection_SkipsOneShot` |
| **Setup** | Instance with tag `backlog:triage`, `Status = Stopped` |
| **Assert** | Driver exits cleanly; no restart attempted |

### Task 8.9: Test exit detection — work session triggers retry

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestSessionDriver_ExitDetection_RetriesWorkSession` |
| **Setup** | Instance with tag `backlog:work`, `Status = Stopped`, `retried = false` |
| **Assert** | `handleDriverFailure` is called; `retried` flips to `true`; restart is attempted |

### Task 8.10: Test second failure marks NeedsAttention

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestSessionDriver_SecondFailure_MarksNeedsAttention` |
| **Setup** | Instance with `retried = true` (already failed once); `Status = Stopped` |
| **Assert** | `markSessionNeedsAttention` is called; `ReviewQueue.Add` is invoked with a `*ReviewItem` whose `SessionID == inst.UUID` and `Reason == ReasonStale`; no restart attempted |

### Task 8.11: Test `RecoverFromStopped` → `Start(false)` restart flow

| Field | Value |
|---|---|
| **File** | `session/session_driver_test.go` |
| **Test** | `TestHandleDriverFailure_StoppedSession_RestartFlow` |
| **Setup** | Instance with `Status = Stopped`, no live tmux session; mock `Restart` to return nil error |
| **Assert** | `RecoverFromStopped()` is called before `Start(false)`; after restart, a new driver goroutine is started with the continuation prompt; `retried` is `true` after the call |
| **Note** | CONCERN-4 fix: This test documents and validates the `RecoverFromStopped → Start(false)` path used in `handleDriverFailure` for `Stopped` instances. |

---

## Implementation Order and Dependencies

Tasks must be executed in this order (later tasks depend on earlier ones):

```
Epic 6 (idempotency guard)     ← no dependencies; defines driverRunning on Instance
    │
Epic 7 (panic recovery)        ← depends on Epic 6 (goroutine wrapper)
    │
Epic 5 (retry logic)           ← depends on nothing, but tested with Epic 2/3
    │
Epic 4 (continuation prompt)   ← depends on nothing (uses readLastNMessagesFromFile)
    │
Epic 2 (inactivity detection)  ← depends on Epic 5 (calls handleDriverFailure)
Epic 3 (exit detection)        ← depends on Epic 5 (calls handleDriverFailure)
    │
Epic 1 (MCP wiring)            ← independent; can be done first
    │
Epic 8 (tests)                 ← depends on all above
```

**Recommended coding order**:
1. Epic 6 tasks 6.1–6.3 (add `driverRunning` field and reset points)
2. Epic 4 task 4.1 (continuation prompt builder — no side effects, easy to test in isolation)
3. Epic 5 tasks 5.1–5.3 (retry + attention marking; refactor `runSessionDriver`)
4. Epic 7 task 7.1 (panic recovery — wrap the goroutine)
5. Epic 2 tasks 2.1–2.2 (inactivity detection — add to `runSessionDriverWithPrompt`)
6. Epic 3 tasks 3.1–3.2 (exit detection — replace terminal-state check)
7. Epic 1 task 1.1 (MCP wiring — one line, do last to avoid testing against broken logic)
8. Epic 8 all tasks (tests)

---

## Key Design Decisions

### D1: `retried` as local variable vs. struct field
`retried` is a local `atomic.Bool` in `runSessionDriver`, passed as a pointer to `runSessionDriverWithPrompt` and to `handleDriverFailure`. This is simpler than a driver struct. Since there is at most one driver goroutine per instance (enforced by Epic 6), local state is sufficient.

### D2: No new Status constant for v1
The requirements document uses "NeedsAttention" conceptually, but no `StatusNeedsAttention` constant exists. For v1, `ReviewQueue.Add` is the notification path. Adding a new status constant is a separate architectural decision (requires proto changes, UI changes) and is out of scope.

### D3: `driverRunning` reset in `handleDriverFailure` is intentionally skipped
When `handleDriverFailure` calls `runSessionDriverWithPrompt` directly (not `StartSessionDriver`), it does NOT go through the CAS guard. This is correct: the old driver goroutine is about to return (caller calls `return` after `handleDriverFailure`), and the new goroutine is started directly. The `defer inst.driverRunning.Store(false)` in the outer goroutine wrapper will fire when the old goroutine returns, and the new goroutine does NOT have the CAS guard wrapping it. This means `driverRunning` will briefly be false between the old goroutine returning and the new goroutine... setting it to false again. This is a minor state gap: external callers could call `StartSessionDriver` again in that window.

**Mitigation for D3**: In `handleDriverFailure`, before spawning the new goroutine, set `inst.driverRunning.Store(true)` to close the window:
```go
inst.driverRunning.Store(true) // Prevent external StartSessionDriver during retry
go runSessionDriverWithPrompt(inst, allowedPath, continuationPrompt, retried)
```

### D4: `ClaudeConversationMessage` field names (BLOCK-3 resolution)
`ClaudeConversationMessage` has `Role string` (not `Type`) and `Content string` (already a flat string, not a polymorphic interface). Use `msgs[i].Role == "assistant"` and `msgs[i].Content` directly. `extractMsgText` does NOT exist. The content extraction is already done by `extractMsgContent` (history.go:382) before the message is returned from `readLastNMessagesFromFile`.

### D5: `driverTotalTimeout` semantics (BLOCK-1 resolution)
`driverTotalTimeout` is now a safety net (25 min), not the primary termination mechanism. Inactivity detection (fires at Ready + 10 min no output) and exit detection (fires on Stopped) are the actual termination paths. The 25-min safety net catches scenarios where neither path fires (e.g., a session stuck in `Loading` forever).

### D6: `ReviewItem` construction (BLOCK-2 resolution)
`markSessionNeedsAttention` constructs a `*ReviewItem` with `SessionID = inst.UUID`. Using `inst.UUID` (stable, immutable) over `inst.Title` (mutable via rename) is correct for queue de-duplication keying.

---

## Files Changed Summary

| File | Type | Epic |
|---|---|---|
| `server/mcp/tools_lifecycle.go` | Modified | Epic 1 |
| `session/session_driver.go` | Modified (heavily) | Epics 2, 3, 4, 5, 6, 7 |
| `session/instance.go` | Modified (add field) | Epic 6 |
| `session/session_driver_test.go` | Modified (add tests) | Epic 8 |
| `session/session_driver_continuation_test.go` | New (JSONL tests) | Epic 8 |

Total: 4 modified files, 1 new test file. No new files beyond what was originally planned — all patches are in-place modifications to existing tasks.

---

## Acceptance Criteria Traceability

| AC | Covered By |
|---|---|
| AC-1 (triage dialog) | Already wired via `CreateDirectorySession`; existing driver handles it |
| AC-2 (work dialog) | Same |
| AC-3 (review dialog) | Same |
| AC-4 (MCP dialog) | Epic 1 Task 1.1 |
| AC-5 (unexpected exit restart) | Epic 3 + Epic 5 |
| AC-6 (inactivity restart) | Epic 2 + Epic 5 |
| AC-7 (double failure → NeedsAttention) | Epic 5 Task 5.2 |
| AC-8 (4 creation paths call StartSessionDriver) | Backlog paths: already done. MCP: Epic 1 |
| AC-9 (double start = no-op) | Epic 6 |
| AC-10 (panic recovery) | Epic 7 |
