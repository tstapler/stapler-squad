# Pitfalls Research: Custom Shells

## PTY Lifecycle and Zombie Process Risks

**The primary risk**: shell processes spawned inside tmux windows can exit and leave zombie entries if `Wait()` is never called. The existing `StartZombieReaper` (`session/tmux/zombie_reaper.go`) runs `syscall.Wait4(-1, WNOHANG)` periodically and will catch these, but it only fires at its configured interval (typically 60s). There will be a window of zombie existence.

**More specific risk**: when `tmux kill-window` is called to stop a shell, tmux itself reaps the child process. However the `TmuxSession.attachCmd` (the `tmux attach-session` process) is a separate child. If the attach command is killed before it exits cleanly, *its* child references may linger. The pattern in `TmuxSession.Close()` (kill-session + abort controller + timeout) must be replicated for shell windows.

**Mitigation**:
- Always call `attachCmd.Wait()` (or the PTY's equivalent) after closing a shell's PTY.
- On `StopShell`, close the PTY file first, then send `tmux kill-window`, then await the attach command's exit.
- The existing `StartZombieWatcher` will alert if any accumulate.

**Exit code capture**: when the shell process exits, `tmux` detects it and the window closes automatically (unless `remain-on-exit` is set). The attach PTY will get an EOF / read error. The server needs to handle this EOF as a clean "shell exited" event, capture the exit code via `tmux display-message -t {session}:{window} "#{pane_dead_status}"`, and push a `ShellStatusUpdate` to the client.

## tmux Window Naming and Index Collisions

tmux window indices are not stable: `new-window` assigns the next available index, but if windows are deleted and recreated, indices can be reused. Relying on the integer index as a persistent shell identifier is unsafe across restart.

**Mitigations**:
- Store the **tmux window name** (not index) as the durable identifier. Use `new-window -n {shellId}` where `shellId` is the ent Shell UUID.
- Use `tmux attach-session -t {session}:{windowName}` (name-based targeting) instead of index-based.
- On server restart, run `tmux list-windows -t {session} -F "#{window_name} #{window_index}"` to rediscover window→index mapping.
- The sanitization rule already replaces colons/periods. Shell IDs (UUIDs) are safe to use as window names directly.

**Risk**: if a session is paused (tmux session persists) and the server restarts, the shell window indices may have shifted (unlikely for tmux, but possible if tmux server was also restarted). Name-based targeting avoids this.

## xterm.js Memory Pressure with Many Shell Tabs

The terminal pool in `SessionDetailView` is capped at **8 instances** (`MAX_POOL_SIZE = 8` inferred from the pooledSessionIds state). Each xterm instance holds a scrollback buffer (5000 lines default in `TerminalOutput`). At 5000 lines × ~100 bytes avg = ~500KB per instance. At 8 instances = ~4MB. Adding shells increases pool pressure.

**Risks**:
- If a user opens 6 shell tabs plus the main terminal tab, pool eviction kicks in. The eviction strategy (LRU) will unmount the xterm for the evicted tab. When re-selected, it reconnects and replays scrollback — a visible delay.
- Each shell tab that is active (not display:none) has an active WebSocket stream. Many concurrent streams are fine for the server, but each stream has a goroutine pair (read + write) and a flow control channel. At 10 shells this is ~20 goroutines, well within Go's capabilities, but worth noting.

**Mitigations**:
- Shell tabs share the existing terminal pool. No separate pool needed.
- Cap shells per session at 8 in V1 (matching pool size) to prevent forced eviction of the main terminal.
- Consider reducing shell scrollback buffer to 1000 lines (vs. 5000 for main terminal) since shells are typically shorter-lived.
- Document the 8-tab practical limit in the UI (dim the "Add Shell" button when at limit).

## Scrollback Limits

The scrollback recording (`session/scrollback/`) stores output for replay on reconnect. Shell scrollback is unbounded in theory — a shell running `find /` produces megabytes of output. If shell scrollback is stored in the same store as session scrollback, a verbose shell could crowd out the session scrollback under any size limits.

**Mitigations**:
- Key shell scrollback as `{sessionId}/{shellId}` — already isolated per shell.
- Apply a smaller per-shell scrollback cap (e.g., 500 lines vs. the session default of 1000).
- If scrollback storage uses SQLite (via ent), large scrollback blobs in BLOB columns cause WAL file growth. Consider capping blob size in the scrollback writer.

## Security: Arbitrary Command Execution

`StartShell` allows the caller to specify an arbitrary `command` string. In the current trust model (single-user local server), this is acceptable. However:

- **Path injection**: if `command` is passed directly to `tmux new-window`, a malicious command string with semicolons or tmux-special characters could break out of the intended command. Use `--` and proper argument quoting:
  ```bash
  tmux new-window -t {session} -n {shellId} -c {workDir} -- /bin/sh -c {command}
  ```
  The `exec.Command` approach (already used elsewhere in `TmuxSession`) is safe: pass command and args as separate slice elements, never shell-interpolated.

- **Working directory traversal**: `working_dir` should be validated against the session's root path if sandboxing is desired. For V1, accept any valid absolute path.

- **Multi-user future**: if stapler-squad ever gains multi-user auth, `StartShell` must verify the requesting user owns `session_id`. The current single-user model is fine; add a TODO comment now.

## Migration Path and Backwards Compatibility

**ent auto-migrate**: `client.Schema.Create(ctx)` runs on startup. Adding the `Shell` entity creates the `shells` table automatically. No manual migration script needed. Existing installs that upgrade will get the table created on first boot.

**Proto field `shell_id = 17`**: field 17 is new; old clients that send `TerminalData` without it will have `shell_id = ""` which the server treats as the main terminal (backwards compatible). Old servers receiving a `shell_id` from a new client will ignore the unknown field (proto3 unknown field forwarding). **No breaking change.**

**`SessionDetailTab` union type**: adding dynamic shell tabs alongside the static tab union requires care. The TypeScript union `"terminal" | "diff" | "vcs" | "logs" | "info" | "files"` should **not** be extended with shell IDs — that would require compile-time knowledge of runtime data. Instead, the tab routing logic should branch: if `activeTab` starts with `"shell:"`, render the shell terminal; otherwise use the existing switch. The `onTabChange` prop callers need to handle the new string format gracefully.

**Session registry touchpoints**: per `.claude/rules/session-creation-registry.md`, shells are NOT a new session creation mode — they're a sub-resource of an existing session. The 7-touchpoint registry does not apply. However, the feature registry (`docs/registry/`) will need new entries for the `StartShell`/`StopShell`/etc. RPCs once implemented.

## Race Conditions

1. **Spawn race**: `SpawnShell` must be serialized — two concurrent calls could both pick the same next tmux window index. Use a mutex on the shell registry in `Instance` before calling `new-window`.

2. **Stop race**: if `StopShell` is called while `StreamTerminal` is actively reading from the shell's PTY, the PTY close will cause a read error. The stream handler must treat read errors from shell PTYs as normal EOF events (not errors), check if the shell is intentionally stopped, and emit `ShellStatusUpdate{status: STOPPED}` rather than `TerminalError`.

3. **Server restart with live shells**: if the server restarts while shells are running in tmux windows, the in-memory `shells` map is gone. On `ListShells`, the server should reconcile ent-stored shells against `tmux list-windows` output to rebuild the in-memory state. Shells that have no corresponding tmux window are marked `STOPPED`.
