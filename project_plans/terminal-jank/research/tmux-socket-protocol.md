# Research: tmux Direct Socket Communication

Status: Complete | Phase: 2 - Research
Created: 2026-04-09

## Summary

tmux uses a custom binary imsg protocol over a Unix domain socket that requires a complex handshake sequence involving TTY file descriptors, environment variables, and binary-serialized MSG_IDENTIFY messages. No Go library provides direct socket communication with tmux - every existing library uses subprocess exec. The correct approach for Stapler Squad is to use tmux control mode (`tmux -C`) which is the only officially supported API for programmatic tmux interaction.

---

## tmux Socket Protocol

### Connection Handshake

The tmux client-server connection requires:

1. Connect to Unix domain socket at `$TMUX_TMPDIR/tmux-<UID>/<socket-name>` or `/tmp/tmux-<UID>/<socket-name>`
2. Send `MSG_IDENTIFY_*` messages in a specific sequence:
   - `MSG_IDENTIFY_LONGFLAGS` - extended client flags
   - `MSG_IDENTIFY_FEATURES` - terminal feature flags
   - `MSG_IDENTIFY_TERM` - terminal name (e.g., "xterm-256color")
   - `MSG_IDENTIFY_TTYNAME` - TTY device name (e.g., "/dev/ttys001")
   - `MSG_IDENTIFY_CWD` - current working directory
   - `MSG_IDENTIFY_STDIN` / `MSG_IDENTIFY_STDOUT` - **file descriptors passed via SCM_RIGHTS ancillary data** (not plain data)
   - `MSG_IDENTIFY_ENVIRON` - all environment variables (one message per variable)
   - `MSG_IDENTIFY_TERMINFO` - terminal capabilities (ncaps messages)
   - `MSG_IDENTIFY_CLIENTPID` - client process ID
   - `MSG_IDENTIFY_DONE` - handshake complete

3. Send `MSG_COMMAND` or `MSG_SHELL` to execute a command

### Why This Is Not Viable for Go

The fundamental blocker is **file descriptor passing via SCM_RIGHTS**: `MSG_IDENTIFY_STDIN` and `MSG_IDENTIFY_STDOUT` require Unix ancillary data (`sendmsg`/`recvmsg` with `SCM_RIGHTS`). This is OS-specific socket functionality that requires a real TTY to be created and passed. Implementing this from scratch in Go would require:

1. Creating a PTY pair (`openpty`)
2. Building MSG structures matching tmux's internal `imsg` format (not publicly documented)
3. Passing file descriptors via `sendmsg` with `SCM_RIGHTS` ancillary data
4. Implementing the full state machine for the client-server message loop

The imsg wire format is not documented. The protocol version is embedded in the binary and has changed across tmux versions (tmux 3.x vs 2.x have different protocol versions). Implementing against an undocumented, version-sensitive binary protocol is a significant engineering risk.

### Go Libraries Surveyed

All existing Go tmux libraries use subprocess exec:

- **github.com/adrg/strutil/xterm** - not tmux-specific
- **github.com/disneystreaming/go-tmux** - subprocess exec
- **github.com/nicholasgasior/gsfmt** - subprocess exec
- **github.com/zulkarneev/gotmux** - subprocess exec
- Any "gotmux" or "go-tmux" library found - subprocess exec pattern only

None implement direct socket communication. This is the universal consensus in the Go ecosystem.

---

## Correct Approach: tmux Control Mode

tmux provides an official programmatic API: **control mode** (`tmux -C`). Stapler Squad already uses this correctly.

Control mode characteristics:
- Launched as a subprocess: `tmux -C new-session` or `tmux -C attach-session`
- Communication via stdin/stdout (text protocol, not binary)
- Server sends `%output`, `%session-changed`, `%window-pane-changed` and other events
- Client sends tmux commands as text, gets `%begin`/`%end`/`%error` response blocks
- Fully documented in `tmux(1)` man page under "CONTROL MODE"
- Version-stable - the text protocol hasn't changed in tmux 2.x/3.x

The existing `session/tmux/` package in Stapler Squad uses control mode correctly and is the right foundation. The `StartControlMode()`, `SubscribeToControlModeUpdates()`, and `%output` event handling being added for the quiescence detector is the canonical use of the tmux API.

---

## Conclusion

**Direct socket communication is not viable** for any reasonable implementation effort. The binary imsg protocol is undocumented, version-sensitive, requires PTY creation and file descriptor passing, and no Go library exists for it.

**Control mode (`tmux -C`) is the correct and only supported API** for programmatic tmux interaction. The existing Stapler Squad tmux package already uses this correctly. No changes to the communication mechanism are needed.

ADR decision: Not creating a separate ADR for this - the decision is to keep using control mode as-is.

---

## References

- tmux source: `client.c` - `client_send_identify()` handshake sequence
- tmux source: `server-client.c` - server-side message type handling
- tmux(1) man page: "CONTROL MODE" section
- Existing codebase: `session/tmux/tmux.go` - `StartControlMode()`, `SubscribeToControlModeUpdates()`
