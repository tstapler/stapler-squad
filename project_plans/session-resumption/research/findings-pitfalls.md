# Pitfalls Research: Session Resumption

_Researched: 2026-04-03_
_Method: Stack research cross-reference + targeted verification_

---

## macOS TTY Re-parenting Limitations

### Hard Constraint: No reptyr on macOS

The canonical Linux tool for re-parenting a running process to a new terminal is **reptyr**. It works by:
1. Attaching via `ptrace(PTRACE_ATTACH, pid)`
2. Injecting `dup2()` syscalls via `PTRACE_POKEDATA` / `PTRACE_GETREGS` / `PTRACE_SETREGS`
3. Setting a new controlling terminal via injected `ioctl(TIOCSCTTY)`

**None of this works on macOS.** The macOS `ptrace(2)` implementation is a stub compared to Linux:
- `PT_ATTACH` requires: (a) the attaching process is the parent, OR (b) the `com.apple.security.cs.debugger` entitlement — which Apple does not grant to non-Apple software
- `PTRACE_GETREGS`, `PTRACE_SETREGS`, `PTRACE_POKEDATA`, `PTRACE_SYSCALL` — **do not exist** on macOS
- Even with the debugger entitlement, SIP blocks attachment to system processes and processes with hardened runtime enabled (which Claude Code likely has as a signed binary)

**Source**: https://github.com/nelhage/reptyr (explicitly states macOS is not supported)

### SIP (System Integrity Protection) Impact

SIP (`csrutil`) blocks:
- Attaching debuggers to processes owned by root or Apple-signed binaries
- Using DTrace to inspect process internals
- Modifying process memory via task ports for hardened binaries

For processes started by the user (like `claude`) without hardened runtime: `task_for_pid` is technically accessible for same-user processes, but using it to inject code still requires the debugger entitlement to be meaningful.

**Practical conclusion**: You cannot perform TTY re-parenting on macOS from outside the process. The only viable adoption path is the cooperative wrapper pattern (`claude-mux`), which wraps the process at start time.

### What CAN Be Done on macOS (Without Privileges)

- **Read process metadata**: `proc_pidinfo` (libproc) gives cwd, terminal device, open files for same-user processes — no entitlements needed
- **Watch process lifecycle**: `kqueue EVFILT_PROC NOTE_EXIT | NOTE_FORK | NOTE_EXEC` — notified when the process exits/forks
- **Read terminal output** (if claude-mux socket exists): connect as another socket client
- **Identify which TTY a process is on**: `gopsutil Process.Terminal()` returns `/dev/ttys004` etc.

---

## File Descriptor / History File Tracking Gotchas

### lsof Reliability Issues

`lsof` on macOS is reliable for listing open files but has known issues in automated/polling use:

1. **Slow**: Each `lsof -p PID` call takes ~50-100ms due to kernel calls. At 3-second polling intervals, this is acceptable. At 100ms polling, it would use ~10% of a CPU core just for lsof.

2. **Transient failures**: `lsof` can fail with `permission denied` or return empty results during process startup (race between process launch and the `lsof` call). Always handle empty/error results gracefully — don't conclude "no open files" from a single empty result.

3. **Symlinks vs. real paths**: `lsof` may return the symlink path rather than the canonical path. `filepath.EvalSymlinks()` is needed when comparing paths.

4. **Race with file close**: Between a file being identified as open and the session record being written, the process could close the file. Use the presence of the file path (not whether it's currently open) as the persisted session reference.

### FSEvents / fsnotify Gotchas

`fsnotify` (backed by FSEvents on macOS) has several issues with append-only JSONL files:

1. **Coalescing**: FSEvents coalesces multiple rapid changes into a single event. If Claude writes 100 lines in 1 second, you may get 1 or 2 events, not 100. **Do not try to detect individual messages via FSEvents.** Use it only for "this file has changed" notifications, then read the tail.

2. **Latency**: FSEvents has configurable latency (default ~1 second). The first event for a newly created file may arrive 1-3 seconds after the file is created.

3. **Move/rename detection**: When `claude` creates a `.jsonl` file, `fsnotify` fires `CREATE`. But the file name is the session UUID — you need to correlate this to a running process via `lsof`. The correlation is best done by watching the file creation event and then doing a `gopsutil` scan for open files matching that path.

4. **Recursive watch limitation**: `fsnotify.Add()` on macOS with FSEvents will NOT automatically watch new subdirectories created after the watch starts, unless you add them explicitly. The `~/.claude/projects/` directory structure is flat (project-dir → JSONL files), but project dirs are created dynamically. Solution: watch `~/.claude/projects/` for new directory creation events, then add each new directory to the watcher.

   A more robust approach: watch `~/.claude/projects/` with the **recursive flag** via FSEvents directly (not fsnotify). The Go `fsnotify` library does support recursive watches on macOS since v1.6.0 via `watcher.Add("path/", fsnotify.WithRecursive)`.

5. **Rename vs. CREATE**: Some editors and tools create files via write-to-temp + rename. FSEvents would fire `RENAME` not `CREATE`. Ensure both `CREATE` and `RENAME` events are handled.

**Recommended approach**: Use `fsnotify` with recursive flag on `~/.claude/projects/` to detect new JSONL files, then correlate with `gopsutil OpenFiles()` to find the owning PID. This combines fast detection (push-based) with reliable PID resolution (pull-based).

---

## History File Locking & Concurrent Access

### Claude CLI's Write Behavior

Claude CLI writes to `~/.claude/projects/<hash>/<session-id>` (a UUID-named file) with **append-only** writes. Each message adds one or more JSONL lines atomically via `O_APPEND`.

**Key properties of `O_APPEND` on macOS/Linux**:
- Writes of less than `PIPE_BUF` (512 bytes on macOS, 4096 on Linux) are atomic at the OS level
- For writes larger than `PIPE_BUF`, atomicity is not guaranteed — partial lines can appear
- Claude's JSONL entries can be large (tool results, file contents), exceeding `PIPE_BUF`

**Pitfall**: If you `io.ReadAll()` a JSONL file that Claude is actively writing, you may read a partial final line (JSON without closing `}`). The last line will be incomplete if read mid-write.

**Mitigation**: When reading an active JSONL file for scrollback or history correlation:
1. Use `bufio.Scanner` which reads line by line
2. Skip the last line if it cannot be JSON-parsed (it may be mid-write)
3. Track the last successfully parsed line's byte offset, not line count (byte offsets survive truncation, line counts don't)

### Concurrent Access (stapler-squad reading while Claude writes)

Claude does NOT use `flock()` or advisory locks on its JSONL files. Reading from another process is safe as long as you handle partial lines.

**No exclusive lock needed**: `O_APPEND` + atomic line reads (skipping unparseable lines) is sufficient.

**Watch out for file rotation**: Claude does NOT rotate its history files — it always appends to the same UUID-named file for the session's lifetime. However, if the user manually deletes the file, the `open` file handle Claude has will continue writing to the deleted file (the inode is kept alive by the open handle). Your file watcher will fire a `REMOVE` event — handle this gracefully by detaching the watcher for that path.

### Aider's `.aider.chat.history.md` Specifics

Aider appends Markdown to `.aider.chat.history.md` in the working directory. Reading this file concurrently is safe — it's append-only Markdown text, not binary. Reading partial content just gives you an incomplete conversation, not corrupted data.

---

## PID Stability & Session Identity

### PID Reuse Risk

macOS recycles PIDs aggressively (default max PIDs: 99,998). A PID used by `claude` session A may be reused by an unrelated process within minutes of session A exiting.

**Failure scenario**: stapler-squad stores `{pid: 12345, sessionId: "abc"}`. Session A exits. New process B (unrelated) gets PID 12345. stapler-squad polls process 12345 and finds it alive — it thinks session A is still running.

**Mitigation 1**: Never use PID alone as the session identity. The stable session identity is the **claude-mux socket path** (`/tmp/claude-mux-<PID>.sock`) combined with the socket's creation time. If the socket doesn't exist, the session is dead regardless of what `gopsutil` reports.

**Mitigation 2**: For PID-based liveness checks, always verify the process name matches expected value:
```go
p, err := process.NewProcess(int32(pid))
name, _ := p.Name()
if name != "claude" && !strings.Contains(name, "node") {
    // PID reuse — this is not our session
}
```

**Mitigation 3**: Use process start time as a disambiguator. `gopsutil Process.CreateTime()` returns epoch milliseconds. Store `{pid, startTimeMs}` as the process fingerprint. A reused PID will have a different `CreateTime`.

**Mitigation 4**: The most reliable session identity anchor is the **JSONL conversation file path** (`~/.claude/projects/<hash>/<uuid>`). The UUID in the filename is stable and does not change across PID reuse. Always correlate sessions by conversation UUID, not PID.

### Session Identity Strategy

The recommended stable session ID is a composite:
```go
type SessionIdentity struct {
    ConversationUUID string    // from ~/.claude/projects/hash/UUID filename
    ClaudeMuxSocket  string    // /tmp/claude-mux-<PID>.sock path, if available
    PID              int32     // current PID
    PIDStartTime     int64     // epoch ms — disambiguates PID reuse
    CWD              string    // working directory (additional anchor)
}
```

On restart, session resolution order:
1. Try to match by `ConversationUUID` (most stable)
2. Verify `ClaudeMuxSocket` exists (confirms session is live)
3. Verify `PID + PIDStartTime` match (confirms PID not reused)

---

## PTY Adoption Race Conditions

### Buffer Loss Window

When stapler-squad connects to a `claude-mux` Unix socket for the first time, there is a race: Claude may have been running for minutes before the connection. All output written before the connection is **not** in the socket buffer.

**How claude-mux handles this (current behavior)**: The `Multiplexer` in `session/mux/multiplexer.go` likely buffers recent output in a ring buffer. When a new client connects, it should receive the buffered history.

**Risk**: If the ring buffer is too small or not implemented, new connections get no historical output. A newly adopted session in the web UI would appear empty.

**Mitigation**: On adoption, before relying on the socket stream, read the process's existing JSONL conversation file to reconstruct conversation history independently of what's in the PTY stream. The scrollback (PTY output) can start from "now" — the conversation context is recovered from the JSONL.

### Socket Creation Timing

When the user runs `claude` with the `claude-mux` wrapper, there is a small window between:
1. The user launches the command
2. `claude-mux` creates the socket
3. `fsnotify` detects the socket
4. stapler-squad connects

If stapler-squad tries to connect during this window, the connection fails. The socket may not exist yet.

**Mitigation**: Retry with exponential backoff (3 attempts at 100ms, 300ms, 1s). After 1 second, if socket still not reachable, fall back to read-only adoption.

### Multiple Simultaneous Connections

If stapler-squad restarts while a `claude-mux` session is running, it will reconnect to the same socket. The `claude-mux` server must handle multiple concurrent client connections without broadcasting input from one client to all clients (input echo to wrong terminal).

**Pitfall**: If the socket server has a bug where it broadcasts input messages to all clients (instead of routing input from a specific client to the PTY), then the web UI reconnecting would cause its queued keystrokes to appear in the original terminal.

**Mitigation**: Verify the `claude-mux` protocol: `Input` messages should only be forwarded from the designated active client (the one that is "attached"), or from a separate "controller" connection that the web UI uses. Read-only observers should only receive `Output` messages.

### Socket Cleanup on Process Exit

When the `claude` process exits, `claude-mux` should close the socket and clean up `/tmp/claude-mux-<PID>.sock`. If `claude-mux` crashes (SIGKILL), the socket file may be left behind as a stale socket.

**Pitfall**: stapler-squad sees the socket file, tries to connect, gets `ECONNREFUSED`. If not handled gracefully, this causes a reconnect loop.

**Mitigation**:
1. On `ECONNREFUSED`, mark the session as dead and remove the socket from the active adoption list
2. `claude-mux` should use a lockfile or pid file alongside the socket to indicate the server is alive
3. Alternatively, check `lsof -U /tmp/claude-mux-<PID>.sock` to verify the socket has a listening process before connecting

### Scrollback Write Contention

When stapler-squad is simultaneously writing PTY output to `FileScrollbackStorage` AND the forking code is reading from it to create a checkpoint clone, there is potential for write contention.

**Mitigation**: The existing `FileScrollbackStorage` already uses per-session mutex locks (`getFileLock(sessionID)`). A fork reads up to sequence N (which was the snapshot count at checkpoint time), so even if new lines are appended during the fork, the fork only reads lines [0..N] — the read is bounded and deterministic.

---

## Recommendations

### 1. Never Use PID as Session Identity

Always store `{conversationUUID, pidStartTime, claudeMuxSocketPath}` as the session fingerprint. Use conversationUUID as the primary stable anchor, PID+startTime for liveness disambiguation.

### 2. Handle Partial JSONL Lines in Append-Only Files

Skip unparseable final lines when reading active JSONL files. Use `bufio.Scanner` (line-by-line), not `io.ReadAll`. Track byte offsets, not line counts, for resumable reads.

### 3. Do Not Attempt TTY Re-parenting on macOS

Never try to re-parent an already-running process's controlling terminal on macOS. The cooperative `claude-mux` wrapper is the only viable adoption path. For processes started without the wrapper, offer read-only metadata display only.

### 4. Always Retry Socket Connections with Backoff

New socket connections to `claude-mux` can fail during the startup window. Use 3 retries with 100ms/300ms/1s delays before giving up and marking the session as non-adoptable.

### 5. Verify Socket Liveness Before Connection

Before connecting to `/tmp/claude-mux-<PID>.sock`, verify the listening process is alive (check `/tmp/claude-mux-<PID>.sock` inode with `lsof -U` or a non-blocking `connect()` probe). This prevents the stale socket reconnect loop.

### 6. Use Recursive fsnotify for `~/.claude/projects/`

Always use `fsnotify.WithRecursive` (or equivalent) when watching `~/.claude/projects/` so that new project directories created after the watch starts are automatically watched.

### 7. Separate "last seen alive" timestamp from "session identity"

Polling-based liveness checks (gopsutil) are eventually consistent. Maintain a `last_seen_at` timestamp in session state. If `last_seen_at` is > 30 seconds old AND the JSONL file hasn't changed, treat the session as potentially dead and attempt reconnection before declaring it dead.

---

## Sources

| Source | Topic |
|--------|-------|
| https://github.com/nelhage/reptyr | reptyr macOS non-support; ptrace limitations |
| https://github.com/fsnotify/fsnotify | fsnotify behavior on macOS (FSEvents backend) |
| https://github.com/fsnotify/fsnotify/issues/166 | fsnotify recursive watch discussion |
| https://github.com/fsnotify/fsnotify/releases/tag/v1.6.0 | Recursive watch support added |
| https://pkg.go.dev/github.com/shirou/gopsutil/v3/process | gopsutil Process.CreateTime() |
| https://stackoverflow.com/questions/53055100/macos-ptrace-pt-attach-fails-with-permission-denied | macOS PT_ATTACH restrictions |
| https://developer.apple.com/documentation/security/hardened_runtime | macOS hardened runtime + SIP |
| https://github.com/fsnotify/fsnotify/wiki/Porting | FSEvents coalescing behavior |
| https://pubs.opengroup.org/onlinepubs/9699919799/functions/write.html | POSIX O_APPEND atomicity guarantees |
| https://man7.org/linux/man-pages/man7/pipe.7.html | PIPE_BUF atomic write size |
| Codebase: `session/mux/multiplexer.go` | claude-mux socket multiplexer implementation |
| Codebase: `session/scrollback/storage.go` | FileScrollbackStorage mutex design |
| Stack research findings (cross-reference) | gopsutil, kqueue, lsof on macOS |
