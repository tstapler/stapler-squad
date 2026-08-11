# Findings: Pitfalls — MCP Server Failure Modes and Risks

## Summary

The Stapler Squad MCP server bridges Claude (an LLM) with session lifecycle and terminal I/O control via tmux and git worktrees. This creates a high-risk surface: LLMs can inadvertently create orphaned sessions, trigger cascading command failures, exhaust buffers with malformed output, or misinterpret tool results due to encoding issues. Terminal I/O is fraught with ANSI escapes, control characters, encoding edge cases, and non-deterministic timing. The primary risks are **unrecoverable process state leaks**, **output formatting mismatches between LLM expectations and reality**, **deadlock from synchronous blocking I/O**, and **LLM-driven runaway loops** (e.g., repeated session creation, infinite command retries).

This analysis covers protocol-level MCP pitfalls, terminal I/O challenges specific to tmux/PTY, lifecycle management risks, and debugging/observability gaps.

---

## Risk Categories Examined

1. **Protocol and Tool Result Formatting** — MCP JSON serialization, tool result size limits, chunking, encoding mismatches
2. **Terminal I/O Edge Cases** — ANSI escapes, non-printable characters, binary output, large scrollback, race conditions
3. **Process Lifecycle Management** — orphaned sessions, zombie processes, cleanup ordering, cascading failures
4. **LLM-Driven Automation Pitfalls** — hallucinated tool parameters, runaway loops, state divergence, stuck agents
5. **Concurrency and Blocking** — PTY deadlock, unbounded buffering, channel saturation, goroutine leaks
6. **Security (local-only context)** — unchecked command injection (stdin to pane), file descriptor leaks, traversal
7. **Debuggability Gaps** — error context loss, silent failures, trace reconstruction

---

## Trade-off Matrix: Risk Category Assessment

| Risk Category | Likelihood | Severity | Easy Mitigation | Operational Burden |
|---|---|---|---|---|
| ANSI/encoding corruption in tool results | **HIGH** | **HIGH** | Partial (normalize output) | Medium |
| Orphaned tmux/worktree on LLM failure | **MEDIUM** | **HIGH** | Moderate (cleanup guards) | High |
| PTY buffer overflow → deadlock | **MEDIUM** | **CRITICAL** | Difficult (async I/O redesign) | High |
| LLM runaway session creation loop | **MEDIUM** | **MEDIUM** | Easy (rate limit tool) | Low |
| Tool result JSON size exceeds MCP limit | **MEDIUM** | **MEDIUM** | Easy (streaming/truncation) | Low |
| Race: session killed while LLM reads output | **LOW-MEDIUM** | **HIGH** | Difficult (snapshot isolation) | High |
| Control mode EOF not propagated to LLM | **LOW** | **MEDIUM** | Easy (explicit status field) | Low |
| Octal decode misses non-ASCII sequences | **LOW** | **MEDIUM** | Easy (test coverage) | Low |

---

## Risk and Failure Modes

### 1. Terminal I/O and ANSI/Encoding Issues

#### 1.1 ANSI Escape Sequences Corrupt Tool Results
**Failure Mode:** Tmux control mode decodes octal-escaped output via `decodeControlModeOutput()` (control_mode.go:282–307). However:
- Escape sequences like `\x1b[31m` (red text) are valid in terminal output but may confuse LLMs or JSON parsers if included raw in tool results.
- The code uses octal encoding (`\012` for newline) but this assumes all non-printable chars are control codes; actual ANSI escape sequences (ESC followed by `[` and parameters) may not be fully escaped by tmux.
- **Example:** A pane prints colored text `ESC[31mERRORESC[0m`. Octal decode produces byte-for-byte match but includes ANSI codes in the JSON string. LLM receives `"...ERROR..."` with embedded escape codes, potentially confusing the parser or causing the LLM to hallucinate about the content.

**Evidence from codebase:**
```go
// control_mode.go:282–307 decodes octal but doesn't strip ANSI
data := t.decodeControlModeOutput(encodedData)
t.broadcastControlModeUpdate(data)
// data is sent raw to MCP tool result
```

**Mitigation:** Normalize or strip ANSI codes before returning in MCP tool results, OR document that LLMs must ignore them. [TRAINING_ONLY — verify tmux control mode's exact handling of escape sequences in real deployments].

---

#### 1.2 Binary or Non-UTF8 Output Crashes Tool Results
**Failure Mode:**
- Tmux can output raw binary data (e.g., from `cat /bin/ls`).
- `decodeControlModeOutput()` naively treats every byte as valid UTF-8. When the octal-decoded bytes don't form valid UTF-8, JSON marshaling in the MCP tool result fails or produces mojibake.
- The circular buffer (circular_buffer.go) stores raw bytes, so binary data persists.
- When the LLM requests session output via an MCP tool, the tool result JSON serializer crashes or silently mangles binary.

**Evidence:**
```go
// No UTF-8 validation in decodeControlModeOutput
func (t *TmuxSession) decodeControlModeOutput(encoded string) []byte {
    // Processes raw bytes without checking if result is valid UTF-8
    return result.Bytes()
}
// Broadcast sends raw bytes to subscribers; no encoding check
t.broadcastControlModeUpdate(data)
```

**Mitigation:**
- Validate UTF-8 on output (log and skip invalid sequences).
- Use `strings.ToValidUTF8()` or `utf8.Valid()` before returning in tool results.
- Escape binary as hex (`\xHH`) or base64 in tool results.

---

#### 1.3 Large Scrollback Output Exhausts MCP Protocol Limits
**Failure Mode:**
- The circular buffer defaults to 10MB (circular_buffer.go:26).
- When an LLM requests `get_session_output` or similar, returning the full 10MB as a single tool result exceeds typical MCP message size limits (often 100KB–1MB depending on the transport layer).
- MCP spec does not define a standard chunking mechanism for large results; servers typically must truncate or error.
- If the LLM gets an error, it may retry the same request, causing a retry loop.

**Evidence from codebase:**
```go
// PreviewFullHistory captures full scrollback with no size limit
content, err := i.tmuxManager.CapturePaneContentWithOptions("-", "-")
// Returns potentially 10MB+ as a single string
```

**Mitigation:**
- Introduce a `max_output_size` parameter (e.g., last 100KB).
- Stream output via pagination (cursor-based).
- Use a separate HTTP endpoint for large blobs if MCP is only the control channel.

---

#### 1.4 Control Mode EOF / Detach Not Signaled Cleanly to LLM
**Failure Mode:**
- When `StopControlMode()` is called (control_mode.go:82), it closes the stdout pipe and marks `controlModeExited = true`.
- However, the LLM doesn't receive an explicit "session detached" signal; it relies on the channel being closed or a status field.
- If the LLM is streaming output and the session is killed, the channel closes, but the LLM may not interpret this as a terminal condition; it may retry or hang waiting for more data.

**Mitigation:**
- Always include an explicit `status` field in tool results (e.g., `"status": "active" | "detached" | "error"`).
- Document that channel closure means end-of-stream.

---

### 2. Process Lifecycle and Orphaned Sessions

#### 2.1 Session Creation Fails Partway; Cleanup is Incomplete
**Failure Mode:**
- `start()` in tmux.go creates a detached tmux session, then polls for existence.
- If the program (claude, aider) crashes before the session is fully ready, `DoesSessionExist()` returns true, but the session is in a broken state.
- The LLM then attempts to use the session; commands fail, but the session remains in the database and disk (worktree, git refs).
- Later, when the user stops the agent, cleanup may fail if the worktree has been modified in an unexpected state.

**Evidence:**
```go
// start() in tmux.go:442
// Session exists, but program may have crashed during initialization
if !t.DoesSessionExist() {
    // Poll and wait
}
// No explicit health check after session creation
log.InfoLog.Printf("Tmux session '%s' created successfully, program '%s' starting", ...)
```

**Mitigation:**
- Perform a "smoke test" after session creation (e.g., send a marker command and verify the prompt appears within a timeout).
- Track session "initialized" vs. "created" state separately.
- If the smoke test fails, force-kill the session and retry.

---

#### 2.2 LLM Repeatedly Creates Sessions; Orphaned Tmux Sessions Accumulate
**Failure Mode:**
- If the `create_session` MCP tool lacks rate limiting or deduplication, a looping LLM can call it 100+ times before the user notices.
- Each failed or duplicate session creation leaves a tmux process running and a worktree directory on disk.
- The next invocation of the MCP server sees stale sessions and may attempt to "reuse" them (as per start() logic), causing confusion.
- Memory/disk eventually exhausted; system becomes unstable.

**Evidence:**
```go
// start() in tmux.go:444–456 reuses existing sessions without validation
if t.DoesSessionExist() {
    log.InfoLog.Printf("Tmux session '%s' already exists, reusing existing session", t.sanitizedName)
    return nil
}
// LLM can't distinguish between a reused session and a new one; may have stale history
```

**Mitigation:**
- Add a tool-level check: if the session already exists and was created less than N seconds ago, return an error (not a success).
- Implement rate limiting in the MCP server (e.g., max 1 new session per 10 seconds per agent type).
- Return a creation timestamp and session ID in the response; let the LLM check before retrying.

---

#### 2.3 CleanupWorktree Fails; Git Worktree Left Behind
**Failure Mode:**
- `CleanupWorktree()` in instance.go calls `gitManager.Cleanup()`.
- If the git worktree has lock files (`.git/index.lock`), or the directory is in use, the cleanup fails with a "directory not empty" error.
- The error is propagated but the MCP server continues; the session is marked as destroyed in the database, but the directory remains on disk.
- Over time, orphaned worktrees accumulate and consume disk space.
- If the LLM later tries to create a new session with the same name, it may collide with the orphaned worktree.

**Evidence:**
```go
// instance.go:1024
func (i *Instance) CleanupWorktree() error {
    if i.gitManager.HasWorktree() {
        if err := i.gitManager.Cleanup(); err != nil {
            return fmt.Errorf("failed to cleanup git worktree: %w", err)
        }
    }
    return nil
}
// If Cleanup() returns an error, the worktree is NOT removed; error propagates to MCP client
```

**Mitigation:**
- Implement an async cleanup task that retries failed cleanups after 5–10 seconds.
- Use `sync.Mutex` on worktree cleanup to serialize concurrent attempts.
- Log a "cleanup failure" event and alert the user that manual intervention is needed.
- Consider moving to a quarantine directory instead of immediate deletion, allowing for recovery.

---

#### 2.4 Control Mode Process Hangs; Subscribers Block Forever
**Failure Mode:**
- `readControlModeOutput()` (control_mode.go:142) reads from tmux control mode stdout in a goroutine.
- If tmux hangs or the pipe deadlocks, this goroutine blocks forever.
- Subscribers waiting on the channel in `broadcastControlModeUpdate()` (line 323–338) never receive a close signal if the process hangs.
- The LLM gets stuck in a long-poll waiting for output.

**Evidence:**
```go
// control_mode.go:142–154
// No timeout; blocks indefinitely if tmux hangs
for scanner.Scan() {
    select {
    case <-doneCh:
        return
    default:
        line := scanner.Text()
        t.processControlModeLine(line)
    }
}
```

**Mitigation:**
- Add a `readTimeout` to the scanner (e.g., 30 seconds of no data = force reconnect).
- Use `context.WithTimeout()` to bound the goroutine lifetime.
- Implement a watchdog timer that signals recovery if no data is seen for N seconds.

---

### 3. LLM-Driven Runaway Loops and State Divergence

#### 3.1 Runaway Command Execution Loop
**Failure Mode:**
- The LLM calls `send_input` repeatedly without waiting for command completion.
- If the tool result doesn't include a clear "command finished" signal, the LLM may assume the command is still running and send more input.
- This causes command buffering in the tmux pane, potentially overflowing the PTY input buffer (usually 4KB).
- Once the buffer overflows, subsequent input is lost, and the LLM's model of session state becomes incorrect.
- The LLM may then attempt recovery commands, which fail unexpectedly.

**Evidence:**
```go
// tmux.go:682
func (t *TmuxSession) SendKeys(keys string) (int, error) {
    return t.ptmx.Write([]byte(keys))
}
// No buffering or flow control; LLM can overflow the PTY input queue
// No echo back or acknowledgment that input was received
```

**Mitigation:**
- Implement input acknowledgment: after sending input, wait for a marker (e.g., `echo <UUID>`) to appear in the output, confirming delivery.
- Bound the MCP tool to send a max of 100 bytes per call, or a max of 5 consecutive calls without an intervening `get_output` call.
- Add a tool-level timeout: if no output changes within 5 seconds of input, fail the tool with "command appears stuck."

---

#### 3.2 LLM Misinterprets "Status" Field; Creates False Success Assumptions
**Failure Mode:**
- When a command fails in the tmux pane, the tool result may return `"status": "ok"` based on the HTTP 200, not the actual command exit code.
- The LLM assumes the command succeeded and continues with the next step.
- 20 commands later, something critical fails because an earlier setup step actually failed.
- By then, the worktree is in an inconsistent state; cleanup is difficult.

**Evidence:**
```go
// instance.go:1468–1518 (ExecutionResult)
// Captures output and status, but doesn't clearly expose command exit code to MCP tool result
type ExecutionResult struct {
    Command       *Command
    Success       bool
    Output        string
    Error         error
    FinalStatus   detection.DetectedStatus
    StatusChanges []StatusChange
}
// If Error is nil, the MCP tool result might report "success" even if the command failed
```

**Mitigation:**
- Always include the command exit code in the tool result JSON (e.g., `"exit_code": 0`).
- Use explicit terminal statuses: `"status": "success" | "error" | "timeout" | "not_found"`.
- Document that `Error` field in the result means the MCP tool itself failed, not the command.

---

#### 3.3 LLM Gets Cached Output; Doesn't Detect State Changes
**Failure Mode:**
- The circular buffer stores the last 10MB of output.
- If the LLM calls `get_session_output` twice without changing the session, it gets the same cached output.
- It assumes the session is idle, but in reality, the session crashed or the prompt changed, and the LLM doesn't know.
- It then sends a command that fails or hangs.

**Evidence:**
```go
// circular_buffer.go caches output; Preview() and PreviewFullHistory() don't check if session is still alive
func (i *Instance) PreviewFullHistory() (string, error) {
    if !i.started || i.Status == Paused {
        return "", nil
    }
    // No check for "did the session change since last call?"
    content, err := i.tmuxManager.CapturePaneContentWithOptions("-", "-")
    return content, err
}
```

**Mitigation:**
- Include a "last modified timestamp" in the tool result so the LLM can detect stale output.
- Use a checksum/hash of the output and return it; the LLM can detect when it changes.
- Implement a `get_session_status` tool that returns `"alive" | "dead" | "paused"` explicitly.

---

### 4. Concurrency, Buffering, and Deadlock

#### 4.1 PTY Write Blocks While Read is Blocked; Deadlock
**Failure Mode:**
- The PTY (created by `creack/pty`) is a bidirectional pipe. If:
  1. The tmux pane's stdout buffer is full (writes blocked in `readControlModeOutput`).
  2. The LLM calls `send_input()`, which calls `ptmx.Write()`.
  3. The pane is awaiting input to proceed, but Write blocks because the stdout buffer is full.
- This is a **classic PTY deadlock**: the process waits for input to free stdout, but input can't be sent until stdout is drained.

**Evidence:**
```go
// tmux.go:682 — Write with no timeout or non-blocking option
func (t *TmuxSession) SendKeys(keys string) (int, error) {
    return t.ptmx.Write([]byte(keys))
}
// control_mode.go:142–154 — Read from ptmx in a goroutine
// If the read buffer is full, Write blocks until data is drained
```

**Mitigation:**
- Use non-blocking I/O or select/timeout on writes: `ptmx.SetWriteDeadline()`.
- Ensure the read goroutine is always draining the PTY (it should be, but verify).
- If a Write blocks for >1 second, fail the MCP tool with a "PTY unresponsive" error.

---

#### 4.2 Control Mode Subscriber Channel Overflows; Dropped Updates
**Failure Mode:**
- `broadcastControlModeUpdate()` (control_mode.go:323) sends to subscriber channels with a buffer of 100 (line 348).
- If a subscriber is slow (e.g., slow network to the LLM), the channel fills up after 100 updates.
- The broadcast code then drops the update and logs a warning (line 334–335).
- The LLM never sees that output; it gets incomplete session history.

**Evidence:**
```go
// control_mode.go:348 — Buffered channel with size 100
ch := make(chan []byte, 100)

// control_mode.go:328–336 — Dropped if channel is full
select {
case ch <- data:
    // Successfully sent
default:
    // Channel full - subscriber can't keep up
    log.WarningLog.Printf("Control mode subscriber %s channel full..., dropping update",
        subscriberID, t.sanitizedName)
}
```

**Mitigation:**
- Increase buffer size (but this trades memory for latency).
- Implement backpressure: if a subscriber can't keep up, disconnect it and log an error to the LLM.
- Use a time-based ringbuffer per subscriber so new updates drop old ones (not the other way around).

---

#### 4.3 Goroutine Leak in Command Execution
**Failure Mode:**
- `command_executor.go` spawns goroutines to execute commands and monitor status.
- If the context is cancelled before the goroutine exits (e.g., LLM timeout), the goroutine may not clean up properly.
- Over time, goroutines accumulate and consume memory.

**Evidence:**
```go
// command_executor.go:57–70 — Goroutines managed by context
type CommandExecutor struct {
    ctx    context.Context
    cancel context.CancelFunc
    wg     *sync.WaitGroup
}
// If execution is abandoned mid-stream, goroutines may not respect the context cancellation
```

**Mitigation:**
- Verify that all goroutines use the context for early exit.
- Add a goroutine leak detector in tests.
- Implement a maximum lifetime for command execution (hard timeout).

---

### 5. Security in Local-Only Context

#### 5.1 Unchecked Input to PTY; Command Injection Risk
**Failure Mode:**
- The LLM can call `send_input("cat /etc/passwd && rm -rf /home")`.
- There's no validation; the input is written directly to the tmux pane.
- The risk is lower because this is local-only and single-user, but:
  - The LLM is untrusted (it's a remote model, called over the network).
  - If Claude's API is compromised or the prompt is jailbroken, an attacker can run arbitrary commands.
  - Even a buggy LLM can accidentally delete important files or cause resource exhaustion.

**Evidence:**
```go
// tmux.go:682 — No sanitization
func (t *TmuxSession) SendKeys(keys string) (int, error) {
    return t.ptmx.Write([]byte(keys))
}
```

**Mitigation:**
- Implement command allowlist: only permit a whitelist of safe commands (e.g., `npm install`, `git checkout`).
- Alternatively, run the tmux session in a sandboxed environment (Docker, chroot, etc.).
- Log all commands sent by the LLM for audit trails.

---

#### 5.2 Worktree Path Traversal
**Failure Mode:**
- If the LLM can control the worktree name or path, it may create `../../../etc/passwd` as a directory name.
- The code should validate names, but if there's a bug, files outside the intended sandbox could be affected.

**Mitigation:**
- Enforce a naming scheme for worktrees (alphanumeric + underscore only).
- Always resolve paths to absolute form and check they're within the expected directory.

---

### 6. Debugging and Observability Gaps

#### 6.1 Silent Failures in Control Mode Processing
**Failure Mode:**
- `processControlModeLine()` (control_mode.go:206) handles various control mode notifications.
- If an unknown or malformed notification arrives, it logs at DebugLog level (line 273), which may not be enabled in production.
- The LLM doesn't receive an error; it just gets incomplete output.
- Debugging becomes very difficult because the issue is silent.

**Evidence:**
```go
// control_mode.go:273 — Only logged at Debug level
default:
    if log.DebugLog != nil {
        log.DebugLog.Printf("Unknown control mode notification for session '%s': %s", t.sanitizedName, line)
    }
```

**Mitigation:**
- Log unknown notifications at `WarningLog` or `ErrorLog` level, not Debug.
- Return an error to the LLM if a critical notification is malformed.

---

#### 6.2 Octal Decode Errors Silently Ignored
**Failure Mode:**
- `decodeControlModeOutput()` (control_mode.go:282) tries to decode octal but silently falls back to literal characters if parsing fails.
- If the encoding is corrupted, the LLM gets garbled output, but no error is reported.

**Evidence:**
```go
// control_mode.go:289–296
if isOctalDigits(octal) {
    value, err := strconv.ParseUint(octal, 8, 8)
    if err == nil {
        result.WriteByte(byte(value))
        i += 4
        continue
    }
}
// If parse fails, fall through to write the literal backslash + octal chars
result.WriteByte(encoded[i])
```

**Mitigation:**
- Log decode failures with the problematic sequence.
- Return a checksum or hash of the decoded output so the LLM can detect corruption.

---

#### 6.3 Limited Tool Result Context
**Failure Mode:**
- When an MCP tool fails, the error message is often generic (e.g., "failed to send input").
- The LLM can't determine root cause (PTY buffer full? Session dead? Permission denied?).
- It retries with the same parameters, getting the same error in an infinite loop.

**Mitigation:**
- Use error codes / error types in tool results (e.g., `"error_code": "SESSION_NOT_FOUND" | "PTY_BUFFER_FULL" | "TIMEOUT"`).
- Include a `remediation` field in error results (e.g., "retry in 5 seconds" or "session may be dead, check status").

---

### 7. MCP Protocol-Level Pitfalls

#### 7.1 Tool Result Size Exceeds MCP Limits
**Failure Mode:**
- MCP implementations often have a max message size (e.g., 4MB in some transports).
- If a tool returns 10MB of scrollback as a single result, the message is dropped or the connection closes.
- The LLM doesn't receive a clear error; it just loses the response.

**Evidence:**
- No explicit message size limits in the session codebase; relies on MCP server implementation.

**Mitigation:**
- Implement pagination in the MCP tool: `get_session_output(session_id, offset, limit)`.
- Return metadata: `{ "total_size": 10000000, "returned": 100000, "offset": 0, "next_cursor": "..." }`.

---

#### 7.2 Tool Parameter Validation Weak
**Failure Mode:**
- The MCP tool definition might not validate session IDs, input length, or command types.
- An LLM can pass a 10MB string to `send_input`, which crashes the PTY or causes OOM.

**Mitigation:**
- Strict validation at the MCP server boundary (before calling the underlying Go code).
- Return `"error": "input too long (max 4KB)"` for invalid parameters.

---

#### 7.3 Concurrent Tool Invocations on Same Session
**Failure Mode:**
- Multiple concurrent LLMs or threads calling tools on the same session simultaneously.
- If `send_input` and `get_output` happen at the same time, race conditions can occur in the circular buffer or control mode reader.

**Evidence:**
```go
// circular_buffer.go has a mutex, but control mode read/write may not be fully synchronized
type CircularBuffer struct {
    mu sync.RWMutex
    // ...
}
// Control mode broadcast is RLock only; doesn't prevent concurrent Subscribe/Unsubscribe
```

**Mitigation:**
- Document that tools are NOT concurrent-safe; MCP server must serialize invocations per session.
- Add explicit locking at the instance level.

---

## Migration and Adoption Cost

**Development Cost:**
- Implementing robust error handling, validation, and observability will add 20–30% to development time.
- Each failure mode above requires a fix + test, totaling ~15–20 additional test cases.

**Operational Cost:**
- Monitoring and alerting for orphaned sessions, control mode hangs, and buffer overflows.
- Runbooks for common failure scenarios (e.g., "session stuck, how to recover").
- Log aggregation and analysis to detect goroutine leaks, dropped updates, etc.

**User Cost:**
- Users need to understand the limitations: single LLM per session, no concurrent access.
- Clear error messages and recovery instructions are essential.

---

## Operational Concerns

1. **Session Health Monitoring:** Add a background task that detects orphaned or stuck sessions and alerts the user.
2. **Cleanup Automation:** Implement a scheduled cleanup of:
   - Sessions with no activity for >24 hours (mark for deletion, require confirmation).
   - Orphaned worktrees (no corresponding session in the database).
   - Stale control mode processes.
3. **Resource Limits:** Cap the number of concurrent sessions, subscribers, and goroutines. Return an error if limits are exceeded.
4. **Logging and Tracing:** Structured logging (JSON) for all MCP tool invocations, with request/response IDs for tracing.
5. **Metrics:** Track:
   - Tool invocation count and latency.
   - Control mode update frequency and dropped updates.
   - Goroutine count and memory usage per session.
   - Cleanup failure rate.

---

## Prior Art and Lessons Learned

### MCP Pitfalls from Training Knowledge [TRAINING_ONLY — verify]:
1. **Result Formatting:** Many MCP server implementations naively JSON-encode binary or large data; best practice is to base64-encode binary and use pagination for large results.
2. **Timeouts:** Tools that interact with external processes must have strict timeouts; MCP servers that block indefinitely cause LLM timeouts.
3. **Error Handling:** Generic errors confuse LLMs; specific error codes and remediation advice are more useful.
4. **Concurrency:** MCP servers are often single-threaded or use goroutine pools; concurrent access to shared state requires explicit synchronization.

### Terminal/PTY Lessons:
1. **PTY Deadlock:** Classic issue with bidirectional pipes; always ensure reads and writes use non-blocking I/O or are on separate threads.
2. **ANSI Codes:** Terminal emulators emit ANSI escape sequences; stripping or escaping them before returning results is essential.
3. **Binary Data:** Raw binary in PTY output is common; UTF-8 validation and hex/base64 encoding are standard practices.
4. **Control Mode:** Tmux control mode is more reliable than pipe-pane + FIFO, but still requires careful handling of notifications and encoding.

---

## Open Questions

1. **Scope of "session state":** Does the LLM need to be aware of the exact cursor position, terminal dimensions, and scroll position? Or just the visible output?
   - **Impact:** If full state is needed, snapshots may be more suitable than streaming.

2. **Rate limiting policy:** How many sessions can an LLM create per hour? Per day? Should there be per-workspace limits?
   - **Impact:** Prevents runaway loop but requires user configuration.

3. **Async cleanup vs. synchronous:**  Should `kill_session` block until cleanup completes, or return immediately and clean up in the background?
   - **Impact:** Sync = slower MCP response; async = risk of double-delete or state confusion.

4. **Handling session crashes:** If the Claude program crashes unexpectedly, should the session be marked as dead or auto-recovered?
   - **Impact:** Affects LLM's expectation of reusability.

5. **Output normalization:** Should ANSI codes be stripped, preserved, or converted to a semantic representation?
   - **Impact:** Affects LLM's understanding of terminal state (colors, formatting, etc.).

---

## Recommendation

### Immediate Design Decisions:

1. **Implement Explicit Status Fields:**
   - Every tool result must include `"status": "success" | "error" | "timeout" | "session_not_found"`.
   - Include command exit code, if applicable.

2. **Add Output Normalization:**
   - Strip or escape ANSI codes in all tool results.
   - Validate UTF-8; replace invalid sequences with replacement character or hex notation.
   - Implement output truncation with a `max_output_size` parameter (default 100KB, configurable).

3. **Rate Limiting at Tool Level:**
   - Implement per-session request rate limiting in the MCP server (e.g., max 100 tools/second).
   - Track consecutive calls to the same tool without intervening output; error if >5 in a row without status change.

4. **Session Health Checks:**
   - Add a smoke test after session creation: send a marker and wait for it in the output (timeout 10s).
   - Implement a `get_session_status` MCP tool that returns `{ "status": "alive" | "dead", "last_activity": timestamp }`.

5. **Error Context in Tool Results:**
   - Use error codes: `SESSION_NOT_FOUND`, `PTY_BUFFER_FULL`, `TIMEOUT`, `ENCODING_ERROR`, etc.
   - Include a `remediation` field with actionable advice.

6. **Async Cleanup with Retry:**
   - Implement a cleanup task that retries failed cleanups every 5 seconds for up to 5 attempts.
   - Log cleanup failures and alert the user.
   - Consider quarantine directory instead of immediate deletion for safety.

7. **Observability:**
   - Log all MCP tool invocations at INFO level with: session ID, tool name, parameters (redacted), result status, latency.
   - Add metrics: active sessions, orphaned sessions, control mode hangs, dropped updates.
   - Implement structured logging (JSON) for automated alerting.

8. **Concurrency Safety:**
   - Document that tools are NOT concurrent-safe; MCP server must serialize invocations per session.
   - Add instance-level mutex to prevent concurrent access to the same session's PTY/control mode.

9. **PTY Deadlock Prevention:**
   - Add write timeout: `ptmx.SetWriteDeadline(time.Now().Add(5 * time.Second))`.
   - Monitor control mode reader for hangs; if no data in 30s, reconnect.

10. **Test Coverage:**
    - Add tests for all failure modes above (ANSI codes, binary data, large output, orphaned sessions, runaway loops, etc.).
    - Implement goroutine leak detector.
    - Stress test with concurrent tool invocations.

---

## Pending Web Searches

- [ ] "MCP server message size limits by transport" — clarify protocol constraints.
- [ ] "tmux control mode encoding edge cases" — verify octal decode behavior with special characters.
- [ ] "golang creack/pty deadlock prevention" — best practices for PTY I/O safety.
- [ ] "ANSI escape code stripping libraries Go" — recommended packages.
- [ ] "golang goroutine leak detection testing" — tools and patterns.
- [ ] "MCP tool result pagination examples" — reference implementations.
- [ ] "LLM runaway loop detection and prevention" — techniques used in production systems.


## Web Search Results

### Query: "MCP server security risks local process injection prompt injection 2025"
**Key findings** (HIGH SEVERITY — verified from real incidents):
- **Tool poisoning**: Malicious tool descriptions can manipulate LLM into unsafe actions — even locally, this matters if session input is user-controlled
- **Command injection**: Confirmed in `aws-mcp-server` and `markdownify-mcp` — any tool that passes input to shell/PTY without sanitization is vulnerable
- **CVE-2025-6514**: Shell command injection via OAuth metadata passed to system shell — 437k developer environments compromised
- **Supabase incident**: LLM processed attacker-supplied input as SQL commands via MCP — privilege escalation
- For Stapler Squad: `write_to_session` tool passes input directly to tmux PTY. This IS a command injection surface if the LLM is fed attacker-controlled content
- Sources: https://www.practical-devsecops.com/mcp-security-vulnerabilities/, https://simonwillison.net/2025/Apr/9/mcp-prompt-injection/

### Query: "MCP tool result size limits streaming output truncation"
**Key findings**:
- Claude Code silently truncates tool output at 256 lines / 10KB — terminal scrollback that exceeds this disappears silently
- No error is returned to the LLM when truncation occurs — LLM sees partial output and may make wrong decisions
- Mitigation: always apply explicit line_limit parameter, return metadata showing truncation occurred
- Source: https://github.com/anthropics/claude-code/issues/2638

**Severity upgrades based on search:**
- Command injection via PTY write upgraded to CRITICAL (real CVEs exist)
- Silent truncation of terminal output upgraded to HIGH (confirmed Claude Code behavior)
