# Findings: Stack - TMux Session Management Approach

Status: Complete | Phase: 2 - Research
Created: 2026-04-16

---

## Summary

Three approaches are evaluated for session lifecycle management in stapler-squad: the current
`tmux attach-session -CC` via PTY, direct tmux unix socket protocol, and replacing tmux with an
alternative process supervisor. The key requirement is that the child process (claude) must survive
a stapler-squad Go process restart; stapler-squad must be able to reconnect and receive reliable
exit notifications.

**The correct approach is Option A: improve the existing control mode path.** The current code
already uses `tmux -C attach-session` (stdout pipe, not PTY), already parses `%exit`, and already
has subscriber channels. The exit notification problem is a propagation gap, not a transport gap.
The fix is surgical: propagate `%exit` from `processControlModeLine` up to session lifecycle state.

Direct socket protocol (Option B) is not viable — prior research (`terminal-jank/research/
tmux-socket-protocol.md`) confirmed the imsg wire format requires SCM_RIGHTS file-descriptor
passing, is undocumented, and version-sensitive. No Go library implements it.

Replacing tmux (Option C) fails the core survival-across-restart requirement for reasons detailed
below; adopting a new supervisor daemon adds more failure modes than it removes.

---

## Options Surveyed

### Option A: Current approach — `tmux -C attach-session` via subprocess pipe

**What the code does today:**

`TmuxSession.StartControlMode()` in `session/tmux/control_mode.go` runs:

```
tmux -C attach-session -t <session-name>
```

...as a Go `exec.Cmd` with `StdoutPipe()` / `StdinPipe()` (not a PTY). The `readControlModeOutput`
goroutine scans lines and calls `processControlModeLine`, which handles `%output`, `%exit`,
`%session-changed`, `%begin`, `%end`, `%error`.

Separately, `session/response_stream.go` `streamLoop()` reads from a PTY (`creack/pty`) opened by
`session/tmux/pty.go` for the older `tmux attach-session -CC` path. This PTY path handles EOF
detection and calls `log.ForSession()`.

**The gap:** `processControlModeLine` logs `%exit` but does not propagate it to session lifecycle
state (`IsStarted`, status, zombie detection). The two paths (PTY stream + control mode) are
partially redundant and not fully reconciled.

**What `tmux -C` (control mode) provides:**

Per `tmux(1)` man page, control mode sends these lifecycle notifications:
- `%exit [reason]` — the tmux server is exiting (all sessions gone)
- `%session-closed <session-id>` — a specific session was closed
- `%window-close <session-id> <window-id>` — a window was closed
- `%pane-exited <pane-id> <exit-code>` — a pane's process exited with exit code [TRAINING_ONLY - verify exact syntax across tmux versions]
- `%client-detached <client-name>` — a client disconnected

The `%exit` notification arrives when the tmux server is shutting down. `%session-closed` and
`%pane-exited` are the more targeted events for individual program exit. The current
`processControlModeLine` does not handle `%session-closed` or `%pane-exited`.

**Survival across restart:** Tmux sessions survive stapler-squad restarts by design. The tmux
server is a separate process; `StartControlMode()` simply reattaches. `CreateKeepaliveSession()`
in `tmux.go` keeps the server alive across session deletions.

**Exit notification reliability:** After `%pane-exited` or `%session-closed`, no more `%output`
lines arrive. The `readControlModeOutput` scanner loop exits when the pipe closes, setting
`controlModeExited = true` and closing all subscriber channels. This is detectable. The timing
between program exit and `%exit`/pipe close varies by tmux version and whether other clients are
attached. [TRAINING_ONLY - verify: does `tmux -C attach` exit immediately when the attached session
closes, or only when the server exits?]

**Improvement needed:** Handle `%session-closed` and `%pane-exited` in `processControlModeLine`.
Add a callback field (`onExit func(reason string)`) to `TmuxSession` that `processControlModeLine`
invokes. Wire this callback into session state in the `Instance` layer.

---

### Option B: Direct tmux unix socket protocol

**Protocol summary:**

The tmux server listens on a Unix domain socket at:
```
$TMUX_TMPDIR/tmux-<UID>/default   (or named socket with -L)
```

The client-server protocol is `imsg` (a BSD IPC message passing library). The connection
handshake requires:
1. `MSG_IDENTIFY_*` messages in sequence
2. **File descriptors passed via `SCM_RIGHTS` ancillary data** (not inline bytes) for
   `MSG_IDENTIFY_STDIN` and `MSG_IDENTIFY_STDOUT`
3. A real TTY name in `MSG_IDENTIFY_TTYNAME`
4. Binary-packed structs — the format is defined in tmux's `tmux.h` as internal C structs,
   not a public API

**Why this is not viable (confirmed by prior research):**

`terminal-jank/research/tmux-socket-protocol.md` (2026-04-09) documents the same conclusion
based on tmux source code (`client.c`, `server-client.c`):

- No Go library implements direct socket communication. Every surveyed library uses
  subprocess exec: `go-tmux`, `gotmux`, `disneystreaming/go-tmux`.
- `SCM_RIGHTS` requires `syscall.Sendmsg`/`Recvmsg` with control message encoding — doable
  in Go but requires creating a PTY pair first, defeating the purpose.
- The `imsg` binary format is undocumented and has changed across tmux major versions.
  Protocol version is embedded in the binary; tmux 2.x and 3.x have different layouts.
- Control mode text protocol (`%output`, `%session-closed`, etc.) is already available via
  `tmux -C` without implementing the binary layer.

**What direct socket would theoretically gain:**

If the protocol were implemented, the client would receive the same control mode notifications
without spawning a subprocess. This removes one process from the chain. The gain is marginal:
the subprocess approach works, adds <1ms latency, and uses <1MB RSS.

**Verdict:** Not viable. Prohibitive implementation cost (~4–8 weeks), fragile against tmux version
changes, zero community support. The control mode text protocol over subprocess is the supported
API for exactly this use case.

---

### Option C: Replace tmux — alternative process persistence layers

**The core survival requirement:**

Sessions must survive a stapler-squad Go process restart. When stapler-squad restarts, claude
(or any child program) must still be running, and stapler-squad must reconnect to it.

**How tmux satisfies this:**

Tmux is a separate persistent daemon (`tmux server`). The server holds sessions and panes; client
processes attach and detach. When the stapler-squad Go process exits, its `tmux -C attach` child
exits, but the tmux server and the claude process inside it continue running. On restart,
stapler-squad calls `StartControlMode()` and reattaches.

This is the fundamental architectural contract. Any replacement must provide the same guarantee.

**Alternatives evaluated:**

**supervisord / s6 / runit:**

Process supervisors manage long-running daemons. They provide:
- Automatic restart on crash
- Structured log files per service
- Start/stop/restart/status commands
- Exit code capture

They do NOT provide:
- Interactive PTY / terminal multiplexing
- Ability to attach a human (or program) to a running process's stdin/stdout after the fact
- "Reconnect to a running process" semantics

Supervisord manages a process from birth; it cannot adopt an existing process or let a new
observer read its live stdout stream. This fails the reconnect requirement.

The exit notification story IS better: supervisord captures exit codes and can invoke a
`eventlistener` process or fire a NOTIFY mechanism. But gaining exit codes from supervisord
requires restructuring how sessions are started (supervisord must own the lifecycle).

**Migration cost:** Very high. Every session creation, the directory isolation (git worktrees),
the PTY attach/detach flow, the scrollback buffer, `capture-pane` integration — all built on
tmux semantics. Replacing the persistence layer would require rewriting the majority of
`session/tmux/`, `session/external_streamer.go`, and `server/services/connectrpc_websocket.go`.

**Named pipe / FIFO approach:**

A Go daemon could wrap each child process and relay stdin/stdout over named pipes or Unix sockets.
The daemon survives the parent restart; the parent reconnects to the socket.

Prior code in this codebase used `tmux pipe-pane` + FIFO before the control mode migration. The
FIFO approach was replaced specifically because of reliability problems: EOF on FIFO when no
reader is attached, race conditions on writer-before-reader ordering, no backpressure.

A custom daemon (similar to `claude-mux` in `session/mux/`) could be purpose-built. `claude-mux`
already does exactly this for external sessions: it wraps a PTY and exposes it via a Unix domain
socket at `/tmp/claude-mux-<PID>.sock`. The `mux.Discovery` and `ExternalSessionDiscovery` in
`session/external_discovery.go` discover and connect to these sockets.

The `claude-mux` approach is already implemented for the "external session" use case. Extending it
to all sessions is conceptually straightforward, but it would mean abandoning the tmux server as
the persistence mechanism, losing `capture-pane`, `send-keys`, and all tmux introspection commands.

**Go-native PTY supervisor (new daemon):**

A purpose-built Go daemon per session: runs the child process under a PTY, exposes a Unix socket
for readers, captures all output to a ring buffer, and sends exit events when the process exits.

This is essentially reimplementing a subset of tmux. The exit notification story is excellent
(the daemon owns the process, so exit is synchronous and reliable). Survival across parent restart
is guaranteed (daemon is a separate process). But:

- No `tmux capture-pane` for visible screen snapshots
- No `tmux send-keys` for sending input without attaching
- No tmux session naming, windows, or pane management
- Significant new code to maintain
- Interoperability with external sessions (`claude-mux`) becomes two different code paths

**Verdict:** Replacement is not warranted. The problems being solved (exit notification, zombie
detection, clean API) are fixable within the existing tmux wrapper. The survival-across-restart
requirement is already satisfied by tmux. Replacing tmux trades known, well-tested infrastructure
for unknown failure modes in new code.

---

## Trade-off Matrix

| Criterion | A: Improve control mode | B: Direct socket | C: Replace tmux |
|---|---|---|---|
| Exit notification reliability | Medium (needs propagation fix) | High (theoretical) | High (if custom daemon) |
| Session survival across restart | High (tmux handles this today) | High (theoretical) | Medium (new daemon risk) |
| Exit code capture | Low (tmux 3.3+ only, `%pane-exited`) | High (theoretical) | High (supervisor pattern) |
| Implementation cost | Low (surgical fix, 1–2 days) | Very High (4–8 weeks) | Very High (months) |
| Maintenance burden | Low (existing code) | Very High (undocumented protocol) | High (new daemon) |
| tmux version sensitivity | Low (text protocol is stable) | Very High (binary protocol changes) | None |
| capture-pane / send-keys preserved | Yes | Yes | No |
| Reconnect to running session | Yes (tmux attach) | Yes (theoretical) | Requires new protocol |
| Risk of regression | Low | Very High | Very High |
| No new Go dependencies | Yes | Yes (syscall heavy) | Depends |

---

## Risk and Failure Modes

### Option A risks

**Race: %pane-exited vs. pipe close**

If stapler-squad processes `%pane-exited` and transitions the session to Stopped, then later the
control mode pipe closes and `readControlModeOutput` tries to invoke the same callback, double-
transition could occur. The `onExit` callback must be idempotent (once-only semantics, use
`sync.Once` or a state check).

**Race: StopControlMode vs. readControlModeOutput**

`StopControlMode` closes `controlModeDone` and the stdin pipe. `readControlModeOutput` may
concurrently receive a `%exit` line and call the `onExit` callback. The shutdown path must not
invoke `onExit` for intentional (operator-initiated) stops. Solution: distinguish shutdown source
(operator vs. program exit) via a flag set before `StopControlMode` is called.

**tmux version: %pane-exited availability**

`%pane-exited` was added in tmux 3.3 (2022). [TRAINING_ONLY - verify exact version] Systems
running tmux 2.x or 3.0–3.2 will not receive it. The fallback is detecting pipe close (scanner
EOF) as the exit signal, which works but loses exit code and exact reason.

**Timing of %session-closed**

`%session-closed` fires when the last client detaches and the session is destroyed. If the session
was destroyed by the program exiting (last pane closed, tmux option `remain-on-exit off`), this
arrives quickly. If `remain-on-exit on` is set, the pane persists after the program exits and
`%session-closed` does NOT fire until the pane is manually killed. Default is `remain-on-exit off`,
so this is correct behavior in the default case. [TRAINING_ONLY - verify: does stapler-squad set
remain-on-exit anywhere?]

**EOF timing on pipe vs. PTY**

The existing `response_stream.go` PTY path and `control_mode.go` pipe path both detect exits.
Having two detection paths means the session must handle receiving the same exit signal twice.
Consolidation is desirable: the control mode pipe should be the authoritative exit signal; the
PTY EOF should be logged but not independently transition state.

### Option B risks

All risks are fundamental (protocol incompatibility, version fragility, SCM_RIGHTS complexity).
Not enumerated further since Option B is not recommended.

### Option C risks

**Orphaned daemon processes**

A Go daemon per session that outlives the parent means OS-level process management becomes
critical. If the daemon crashes, the session is lost. If the daemon leaks (parent restarts leave
old daemons), resource exhaustion follows. tmux handles this by being a single well-tested server.

**Lost tmux integrations**

`capture-pane` is used in multiple places (`CapturePaneContent`, `CapturePaneContentRaw`, the
visible-screen handshake). `send-keys` is used for program interaction. `tmux kill-session` is
used for cleanup. All require tmux. Removing tmux removes these capabilities.

---

## Migration and Adoption Cost

### Option A (recommended path)

**What changes:**

1. Add `onPaneExited func(exitCode int, reason string)` and `onSessionClosed func()` fields to
   `TmuxSession`
2. In `processControlModeLine`, invoke callbacks for `%pane-exited` and `%session-closed`
   in addition to existing `%exit` handling
3. Ensure the scanner-EOF path in `readControlModeOutput` also fires the callback (for tmux
   versions without `%pane-exited`)
4. Wire callbacks from `Instance` layer into `TmuxSession` during session start
5. Implement `onExit` using `sync.Once` to prevent double-fire
6. Add zombie reconciliation in the review queue or a dedicated background ticker

**Estimated effort:** 1–3 days for a careful implementation with tests.

**No new dependencies.** The change is purely within the existing `session/tmux/` and `session/`
packages. The constraint "no new packages beyond what already exists" (requirements.md) is met.

### Option C (not recommended)

**Would require:**

- New Go daemon binary or in-process supervisor with separate lifecycle
- New Unix socket protocol for reconnection
- Migration of all PTY session creation, worktree management, scrollback buffering
- Migration or abandonment of `capture-pane` integrations
- Parallel support for tmux external sessions (`claude-mux`) and new supervisor sessions

**Estimated effort:** 3–6 months. Not justified by the problem being solved.

---

## Operational Concerns

### tmux version compatibility

Stapler-squad targets macOS (`tmux_unix.go`, `term.GetSize`) and Linux. On macOS, Homebrew tmux
is typically 3.3+. On Linux (Ubuntu 22.04 LTS), the apt package is tmux 3.2a (pre `%pane-exited`).
Ubuntu 24.04 LTS ships tmux 3.3. [TRAINING_ONLY - verify exact versions]

The implementation must degrade gracefully on tmux 3.2: fall back to scanner-EOF detection when
`%pane-exited` is not received. This means scanning for `%pane-exited` opportunistically but
treating it as a bonus, not a requirement.

### keepalive session and exit-empty

`tmux.go` creates a `staplersquad_keepalive` session and sets `exit-empty off` to keep the server
alive. This means the server never exits due to empty sessions — `%exit` from the control mode
only arrives if the server is explicitly killed. This is correct behavior: `%exit` should not be
the primary exit signal; `%session-closed` or `%pane-exited` are session-scoped signals.

The `%exit` handler in `processControlModeLine` currently just logs. This is acceptable: a server
exit is handled at a higher level (server recovery logic in `recoverFromServerFailure`).

### Multiple control mode clients

Each call to `StartControlMode` spawns a new `tmux -C attach-session` process. Multiple such
processes can attach to the same session simultaneously. Each receives the same stream of
notifications independently. This means if multiple components call `StartControlMode` for the
same session, each gets its own pipe and its own copy of `%pane-exited`. The subscriber map
inside `TmuxSession` deduplicates at the Go level, but the OS-level tmux processes are not
deduplicated. [TRAINING_ONLY - verify: does tmux limit the number of `-C` attach processes
per session?]

The current code is a singleton per `TmuxSession` (`controlModeCmd` is checked for nil). This is
correct — only one control mode process should run per TmuxSession instance.

### Control mode process as a goroutine lifetime anchor

`readControlModeOutput` is the goroutine that owns the subscriber channels. If `StopControlMode`
is called while a subscriber is waiting on its channel, the channel close unblocks the waiter.
This is the current design and it is correct. The `controlModeExited` flag handles the
subscribe-after-exit race via the pre-closed channel pattern.

---

## Prior Art and Lessons Learned

### FIFO approach (prior to current code)

The codebase previously used `tmux pipe-pane` + named FIFO for output streaming. This was replaced
because:
- FIFO blocks on open until both reader and writer are present (race on startup)
- FIFO EOF when the writer (tmux) has no data flushes through to reader unexpectedly
- No structured event protocol — raw bytes only, no lifecycle signals

The control mode migration solved these problems. The lesson: raw pipe approaches fail on the
semantics that matter (lifecycle events, backpressure, reconnect).

### claude-mux (existing in session/mux/)

`claude-mux` is a PTY wrapper daemon already deployed for external sessions. It wraps a process
in a PTY, exposes output via a Unix domain socket, and is discovered via filesystem watching. The
`ExternalSessionDiscovery` in `session/external_discovery.go` uses `mux.Discovery` and
`mux.DiscoveredSession` to find and stream from these sockets.

This demonstrates the project can build and operate a custom PTY daemon. However, for tmux-managed
sessions, the value of switching from tmux's control mode to a custom daemon is negative: tmux
provides capture-pane, session persistence, send-keys, and a well-tested PTY environment.
`claude-mux` fills a gap (external sessions that stapler-squad didn't start); it is not a model
for replacing tmux-started sessions.

### Wezterm, Zellij, and other terminal multiplexers

Wezterm uses a multiplexer domain protocol (custom binary over Unix/TCP socket). Zellij uses a
plugin WASM API. Neither provides a Go-native library for direct socket communication. Both are
designed for human terminal use, not programmatic supervision. Not applicable here.

### screen (GNU screen)

Screen has no programmatic API comparable to tmux control mode. Screen sessions survive process
restarts, but the only way to read output is via screen's log file feature or by attaching a pty.
No structured exit notification. tmux is strictly superior for this use case.

### process supervision in Go (hashicorp/go-plugin, grpc subprocess pattern)

hashicorp/go-plugin uses gRPC over stdio to communicate with child processes. The parent detects
child exit via the gRPC connection closing. This is a viable pattern for structured command
invocation but requires the child process to speak the protocol — not applicable to claude, aider,
or other programs that are black boxes from stapler-squad's perspective.

---

## Open Questions

1. **Does `tmux -C attach-session` exit when the attached session closes?**
   The current code treats the scanner EOF as the exit signal. If tmux -C stays connected after
   the session closes (e.g., because the server is still running with other sessions), the pipe
   would stay open and the EOF signal would not arrive. Needs verification against tmux source
   or empirical test.
   [TRAINING_ONLY — verify with: `tmux new-session -d -s test; tmux -C attach-session -t test;
   tmux kill-session -t test` and observe whether the attach process exits]

2. **Exact tmux version that added `%pane-exited`?**
   The implementation plan needs a version check to decide whether to wait for `%pane-exited`
   or fall through to scanner-EOF detection immediately.
   [TRAINING_ONLY — verify in tmux changelog / CHANGES file]

3. **Does `remain-on-exit` affect `%session-closed` delivery?**
   If stapler-squad or the user has set `remain-on-exit on` (e.g., for debugging), `%session-
   closed` will not fire when the program exits — only when the pane is manually killed.
   [TRAINING_ONLY — verify in tmux(1) man page]

4. **Is `controlModeCmd` truly a singleton constraint, or can multiple callers share one?**
   The current code guards with `if t.controlModeCmd != nil { return nil }`. This is a best-effort
   guard for calling code but is not protected by a mutex. A concurrent second call to
   `StartControlMode` could race past the nil check. Minor issue but worth noting for the clean
   API design.

5. **Does the PTY path (response_stream.go) and the control mode path (control_mode.go) need to
   coexist, or should one replace the other?**
   The requirements mention both PTY EOF and `%exit` as exit detection paths. Clarifying which
   is authoritative (and which is the fallback) determines whether to keep both or consolidate.

---

## Recommendation

**Adopt Option A: targeted improvements to the existing control mode path.**

The implementation is surgical:

1. Handle `%pane-exited` and `%session-closed` in `processControlModeLine` (add to the switch
   statement that already handles `%exit`, `%output`, `%session-changed`, `%begin`, `%end`,
   `%error`)
2. Add an `onPaneExited func(paneID string, exitCode int)` and `onSessionClosed func(sessionID
   string)` callback pair to `TmuxSession`, set from the `Instance` layer
3. Protect the callback with `sync.Once` to prevent double-fire from the scanner-EOF fallback
   arriving after a `%pane-exited` event
4. Deprecate the PTY path in `response_stream.go` as the exit authority — it remains for data
   streaming but should not independently transition session state
5. Add zombie reconciliation as a separate, low-frequency background check (every 10s, poll
   `tmux has-session`) as a defense-in-depth layer against missed notifications

This approach requires no new dependencies, touches only `session/tmux/` and the `Instance` layer,
and is directly scoped to the requirements.md success criteria: exit logging, zombie detection in
≤10s, clean API, lifecycle hooks.

---

## Pending Web Searches

The following claims are marked `[TRAINING_ONLY]` and should be verified before implementation:

1. **tmux `%pane-exited` availability by version**
   Query: `tmux changelog %pane-exited added version site:github.com/tmux/tmux`
   What to verify: Which tmux version introduced `%pane-exited`; whether it fires on process exit
   within a pane vs. only on pane destruction.

2. **tmux -C attach-session exit behavior**
   Query: `tmux control mode attach-session exit when session closed`
   What to verify: Does `tmux -C attach-session -t <name>` exit immediately when the named session
   is destroyed, or does it stay open until `detach-client` is issued?

3. **tmux `%session-closed` vs `remain-on-exit`**
   Query: `tmux remain-on-exit %session-closed control mode notification`
   What to verify: Does `%session-closed` fire when last pane's process exits (not when the pane
   is destroyed), or only when the session is destroyed?

4. **Ubuntu/Debian tmux package version by distro release**
   Query: `ubuntu 22.04 tmux package version apt`
   Query: `ubuntu 24.04 tmux package version apt`
   What to verify: Which tmux versions are in the apt repos for major LTS releases, to bound the
   version compatibility matrix.

5. **creack/pty EOF semantics on Linux vs macOS**
   Query: `creack/pty EOF EAGAIN EIO linux macos PTY child exit`
   What to verify: On Linux, PTY master read returns `EIO` (not `EOF`) when the slave side is
   closed after the child exits. The current `response_stream.go` checks for "input/output error"
   which catches `EIO` — confirm this is correct and complete for Linux.

6. **tmux control mode -CC vs -C**
   Query: `tmux control mode -CC vs -C difference`
   What to verify: The requirements mention `tmux attach-session -CC` (PTY) vs. the current code
   which uses `tmux -C attach-session` (pipe). Clarify that `-CC` means "control mode without the
   exit-when-session-closes default" vs. `-C` (single C). This distinction affects whether the
   attach process exits on session close.
