# Detection Architecture: Call-Site & Construction Audit

Generated from source reading of the five target files.

---

## 1. *StatusDetector methods called from outside the `detection` package

All callers are in the `session` package.

| Method | Signature | Call site(s) |
|---|---|---|
| `NewStatusDetector` | `func NewStatusDetector() *StatusDetector` | `review_queue_poller.go:146` (inside `NewReviewQueuePollerWithConfig`); `claude_controller.go:207` (inside `ClaudeController.Start`) |
| `SetSessionID` | `func (sd *StatusDetector) SetSessionID(id string)` | `claude_controller.go:209` (inside `ClaudeController.Start`, immediately after construction) |
| `DetectWithContextFromLines` | `func (sd *StatusDetector) DetectWithContextFromLines(lines []string) (DetectedStatus, string)` | `claude_controller.go:654` (inside `ClaudeController.GetCurrentStatus`); `review_queue_determiner.go:195` (inside `DefaultStatusDeterminer.Determine`, no-controller branch) |
| `GetStatusDetector` (on `ClaudeController`) | returns `*detection.StatusDetector` | `claude_controller.go:971` — used by the `GetDetectionEvents` RPC to expose the detector to the server layer |

> Note: `GetStatusDetector()` is a `ClaudeController` method, not a `StatusDetector` method, but it hands out the raw `*StatusDetector` pointer to callers outside the package.

### Indirect usages (via `StatusDeterminer.Determine` signature)

`review_queue_poller.go:615`:
```go
result := rqp.statusDeterminer.Determine(inst, content, statusInfo, rqp.statusDetector)
```
The `rqp.statusDetector` field (`*detection.StatusDetector`) is passed as the fourth argument to `Determine`, which then calls `detector.DetectWithContextFromLines` at `review_queue_determiner.go:195`.

---

## 2. *IdleDetector methods called from outside the `detection` package

All callers are in the `session` package (`claude_controller.go`).

| Method | Signature | Call site |
|---|---|---|
| `NewIdleDetector` | `func NewIdleDetector(sessionName string, ptyAccess PTYReader) *IdleDetector` | `claude_controller.go:212` (inside `ClaudeController.Start`) |
| `InitializeFromTimestamp` | `func (id *IdleDetector) InitializeFromTimestamp(timestamp time.Time)` | `claude_controller.go:226` (post-LastMeaningfulOutput restore path) and `claude_controller.go:256` (migration path for old sessions) |
| `RecordActivity` | `func (id *IdleDetector) RecordActivity()` | `claude_controller.go:296` (inside the `rs.SetOnOutput` closure — called on every PTY write event) |
| `DetectStateFromContent` | `func (id *IdleDetector) DetectStateFromContent(content string) IdleState` | `claude_controller.go:855` (inside `ClaudeController.GetIdleState`, cache-miss branch) |
| `GetState` | `func (id *IdleDetector) GetState() IdleState` | `claude_controller.go:862` (buffer-empty branch) and `claude_controller.go:865` (no-PTY-access branch) inside `ClaudeController.GetIdleState` |
| `GetLastActivity` | `func (id *IdleDetector) GetLastActivity() time.Time` | `claude_controller.go:868` (inside `ClaudeController.GetIdleState`, after state resolution) |
| `GetIdleDuration` | `func (id *IdleDetector) GetIdleDuration() time.Duration` | `claude_controller.go:897` (inside `ClaudeController.GetIdleDuration`) |
| `GetStateInfo` | `func (id *IdleDetector) GetStateInfo() IdleStateInfo` | `claude_controller.go:889` (inside `ClaudeController.GetIdleStateInfo`, to retrieve `LastStateChange`) |

---

## 3. IdleDetector's internal StatusDetector construction (idle.go lines ~70-100)

`NewIdleDetectorWithConfig` (line 76) creates the `StatusDetector` itself via the package-private constructor:

```go
// idle.go:76-87
func NewIdleDetectorWithConfig(sessionName string, ptyAccess PTYReader, config IdleDetectorConfig) *IdleDetector {
    now := time.Now()
    return &IdleDetector{
        sessionName:     sessionName,
        statusDetector:  NewStatusDetector(),   // ← owns its own StatusDetector
        ptyAccess:       ptyAccess,
        config:          config,
        currentState:    IdleStateUnknown,
        lastStateChange: now,
        lastActivity:    now,
    }
}
```

Key observations:
- `NewStatusDetector()` is called **without** `SetSessionID` — the detector embedded inside `IdleDetector` never has a session ID set on it, so its ring-buffer events carry an empty `SessionID`.
- The embedded `statusDetector` is private (`statusDetector *StatusDetector`); no accessor is exposed. The only external observable is the `IdleState` returned by `DetectStateFromContent` / `DetectState`.
- This is a **second, independent `StatusDetector` instance** per session that uses `IdleDetector` — separate from the one stored in `ClaudeController.statusDetector`.

---

## 4. How ClaudeController creates its StatusDetector

Inside `ClaudeController.Start` (`claude_controller.go:207-209`):

```go
// Create status detector and tag it with the session name for detection event attribution.
sd := detection.NewStatusDetector()
sd.SetSessionID(cc.sessionName)
```

Then immediately after, the `IdleDetector` is created separately (line 212):

```go
id := detection.NewIdleDetector(cc.sessionName, pa)
```

Both are stored as `atomic.Pointer[T]` fields on the controller and published together at lines 326-327:

```go
cc.statusDetector.Store(sd)
cc.idleDetector.Store(id)
```

`ClaudeController.GetCurrentStatus` loads `cc.statusDetector` and calls `sd.DetectWithContextFromLines(lines)` (line 654). `ClaudeController.GetIdleState` loads `cc.idleDetector` and calls `id.DetectStateFromContent(filtered)` (line 855). The two detectors never share data.

---

## Summary of key structural facts

1. **Two separate `StatusDetector` instances per session with a `ClaudeController`**: one in `ClaudeController.statusDetector` (has `SetSessionID` called on it, used for status/ring-buffer events) and one embedded inside the `IdleDetector` created at the same time (no session ID, used only for `IdleState` mapping via `mapStatusToIdleState`).

2. **`ReviewQueuePoller` owns a third `StatusDetector`** at `rqp.statusDetector` (`review_queue_poller.go:146`, `detection.NewStatusDetector()`) used as a fallback detector for sessions without an active `ClaudeController` — it is never tagged with a session ID and is shared across all sessions polled by that poller instance.

3. **`IdleDetector`'s internal `StatusDetector` is invisible from outside the package**: no getter is exposed, `SetSessionID` is never called on it, and its detection ring buffer events are effectively orphaned (empty `SessionID`). The only public surface of `IdleDetector` is its `IdleState` output, not the raw `DetectedStatus` that the internal detector produces.
