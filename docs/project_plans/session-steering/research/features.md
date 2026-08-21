# Feature Research — Session Steering

## Claude Code JSONL Conversation Log Structure

Claude Code stores two distinct JSONL files:

### 1. `~/.claude/history.jsonl` — Global history index
Each line is one user message, serialized as:
```json
{
  "display": "first ~200 chars of user message",
  "timestamp": 1714500000000,
  "project": "/absolute/path/to/project",
  "sessionId": "<uuid>"
}
```
This is a lightweight index; it does not contain assistant responses.

### 2. `~/.claude/projects/<encoded-path>/<uuid>.jsonl` — Per-conversation messages
Each line is one message turn:
```json
{
  "type": "user" | "assistant",
  "uuid": "<message-uuid>",
  "sessionId": "<conversation-uuid>",
  "timestamp": "2026-01-01T00:00:00Z",
  "cwd": "/project/path",
  "message": {
    "role": "user" | "assistant",
    "model": "claude-sonnet-4-5",
    "content": "string | [{\"type\": \"text\", \"text\": \"...\"}]"
  }
}
```

The `content` field is polymorphic:
- Plain string for simple messages
- Array of `{type: "text", text: "..."}` blocks for rich content
- Other block types (tool_use, tool_result) for tool interactions — these are NOT type "user"/"assistant" entries

**Key insight**: The codebase's `extractMsgContent` function (session/history.go in the agent worktree) demonstrates that entries with `type != "user"` and `type != "assistant"` are skipped. Only user/assistant turns contribute to the conversation transcript.

**Reading implementation** (in `.claude/worktrees/agent-a15233e36c0676aa4/session/history.go`):
- `readAllMessagesFromFile(path)` — bufio.Scanner, 1MB line buffer, skip malformed lines
- `readLastNMessagesFromFile(path, n)` — reads from end in 64KiB chunks for efficiency

---

## Path Algorithm: `~/.claude/projects/<encoded-path>/`

The encoding is implemented in `session/history_detector.go:ClaudeProjectDirName`:

```go
func ClaudeProjectDirName(projectPath string) string {
    result := make([]byte, len(projectPath))
    for i := 0; i < len(projectPath); i++ {
        c := projectPath[i]
        if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
            result[i] = c
        } else {
            result[i] = '-'
        }
    }
    return string(result)
}
```

Replace every non-alphanumeric character with `-`. Examples:
- `/Users/alice/myproject` → `-Users-alice-myproject`
- `/Users/alice/.hidden/my_project` → `-Users-alice--hidden-my-project`

The `HistoryFileDetector.DetectByPath(projectPath)` method uses this to find:
```
~/.claude/projects/<ClaudeProjectDirName(projectPath)>/*.jsonl
```
It picks the most recently modified `.jsonl` file that has a valid UUID filename (excluding `agent-*.jsonl` files).

This path algorithm is the source of truth — do not re-implement it. Always call `ClaudeProjectDirName` or read `inst.HistoryFilePath` which HistoryLinker has already populated.

---

## Good "Continuation Prompt" for Resuming a Half-Done Coding Task

Based on the existing `driverInitialPrompt = "Please proceed with the task described in your instructions."` and the pattern used by `BuildTokenBudgetedPrompt` in backlog:

**Minimal effective prompt** (use when session has been running and we're re-injecting after restart):
```
Your previous session exited unexpectedly. The context above shows your conversation history.
Please continue from where you left off. If you were in the middle of a task, resume it.
Do not re-introduce yourself or repeat completed work.
```

**Richer prompt** (when JSONL is available to summarize last action):
```
Your session restarted after an unexpected exit. Your last message was:
---
<last assistant message, truncated to 500 chars>
---
Please continue from where you left off.
```

**Key considerations**:
1. Claude Code with `--resume <uuid>` will automatically load the conversation history; the continuation prompt should be brief and not duplicate what's already in context
2. If the restart uses `--resume`, Claude already has full context — a one-line "continue" is sufficient
3. If the restart is a fresh session (no UUID), the continuation prompt needs to carry more context from the JSONL

**Recommendation**: The driver should read the last assistant message from `inst.HistoryFilePath` (via `readLastNMessagesFromFile`), extract the last assistant turn, and build a minimal continuation prompt. This is especially useful when `--resume` is not available (UUID was cleared after first failure).

---

## Inactivity / Liveness Detection for Process Supervision in Go

### Pattern Used in This Codebase: `LastMeaningfulOutput` Timestamp

The existing `ReviewState.LastMeaningfulOutput` field (populated by `ReviewQueuePoller` via `UpdateTimestamps`) is the production heartbeat mechanism. It is updated whenever terminal content changes from the last poll.

**For inactivity detection** in the session driver:
```go
last := inst.LastMeaningfulOutputTime()
if !last.IsZero() && time.Since(last) > 10*time.Minute {
    // stuck: no output change for 10 minutes
}
```

`inst.LastMeaningfulOutputTime()` is safe to call from any goroutine — it acquires `stateMutex.RLock()`.

### Alternative: Snapshot-Based Detection (used in ReviewQueuePoller)

`ReviewQueuePoller` stores `LastOutputSignature` (a content hash) and compares it across poll cycles. If content hasn't changed and a threshold has passed, the session is considered stuck. This is heavier but more accurate — it doesn't fire if the session is still generating output even if `LastMeaningfulOutput` hasn't been updated recently.

### Process Heartbeat Pattern (NOT used here)

For reference: typical Go process supervisors use one of:
1. **Timestamp polling** (this codebase): check `time.Since(lastActivity) > threshold` on a ticker
2. **Channel-based**: supervised goroutine sends to a `heartbeat chan struct{}` every N seconds; supervisor resets a timer on receive; timeout = stuck
3. **Context deadline**: set a per-task deadline with `context.WithTimeout`; goroutine checks `ctx.Err()`

For session steering, pattern 1 (timestamp polling on `LastMeaningfulOutput`) is the correct choice since the terminal activity is already tracked.

---

## Existing Session Status Transitions (Running → Stopped, Ready, NeedsApproval)

From `session/instance.go` and `instance_state.go`:

| Event | Trigger | Status After |
|---|---|---|
| tmux control-mode `%exit` | `SetOnExitCallback` in `start()` | `Running`/`Ready` → `Stopped` then `fireLifecycleEvent(EventExited)` |
| PTY EOF | `instance_controller.go:68` | `Running`/`Ready` → fires `EventExited` |
| Terminal scan finds `>` prompt | `ClaudeController` → `MarkReady()` | `Running` → `Ready` |
| Terminal scan finds approval prompt | `ReviewQueuePoller` → `MarkNeedsApproval()` | `Running`/`Ready` → `NeedsApproval` |
| Operator calls `Kill()` | `instance.go:700` | any → `Stopped` (intentional; does NOT fire EventExited) |
| Operator calls `Pause()` | `instance.go:731` | any → `Paused` |

**For the driver**: after `inst.Restart()`, the status transitions back through `Creating` → `Loading` → `Running` → (eventually) `Ready`. The driver's existing startup dialog + ready detection loop handles this correctly. The driver's retry loop should call `inst.Restart(false)` (or `inst.RecoverFromStopped()` + `inst.Start(false)` for Stopped instances) and then continue the main driver loop.

**`NeedsAttention` does not exist as a `Status` value** in the current codebase. It is only a helper method on `InstanceStatusInfo`. The requirements document uses it conceptually — implementation should use `NeedsApproval` or add a new status constant.
