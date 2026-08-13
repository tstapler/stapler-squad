# Findings: tmux Unix Socket IPC from Go

## Summary

The tmux client speaks to its server over a Unix domain socket using a **private, undocumented
binary protocol** built on OpenBSD's `imsg` framing library. The protocol is not versioned with
any stable API contract; it is changed without notice between tmux releases (the tmux project
explicitly states the socket protocol is internal). No production-quality Go library implements
this protocol natively. The implementation cost from scratch is high (2–4 weeks of careful
reverse-engineering plus ongoing maintenance), the maintenance burden is severe, and the payoff
relative to the **control mode stdin pipe approach** is minimal to negative.

The control mode approach (findings-control-mode-commands.md) already achieves zero subprocess
overhead for all query operations while using an officially documented, text-based, stable
protocol. The Unix socket IPC path adds complexity with no measurable performance advantage for
this workload.

**Recommendation: Do not implement raw socket IPC. The control mode stdin approach is the
correct path. This document is included for completeness and to close the question definitively.**

---

## Options Surveyed

### Option 1 — Raw Unix socket with custom imsg-based protocol implementation

Connect to `/tmp/tmux-UID/default` (or the `-L`/`-S` override path), speak the binary framing
protocol directly without spawning any process.

**Protocol overview** (from reverse-engineering tmux source; [TRAINING_ONLY — verify against
tmux 3.x source]):

- The transport is a Unix domain socket (SOCK_STREAM, not SOCK_DGRAM).
- Message framing uses `imsg` from OpenBSD's `libutil` / `imsg.c`. Each message is a fixed-size
  header (`struct imsg_hdr`: `uint32_t type`, `uint16_t len`, `uint16_t flags`, `uint32_t peerid`,
  `uint32_t pid`) followed by `len - IMSG_HEADER_SIZE` bytes of payload.
- The tmux client binary (`tmux_client.c`) opens the socket, sends `MSG_VERSION` with the
  protocol version integer, waits for `MSG_READY` from the server, then sends the command as a
  null-terminated string in a `MSG_COMMAND` / `MSG_EXEC` message.
- Command output flows back as `MSG_PRINT` / `MSG_OUTPUT` messages; exit code via `MSG_EXIT`.
- For control mode (`-C`), the client sends `MSG_FLAGS` with terminal feature flags and
  `MSG_ATTACH` or `MSG_EXEC`, then the server begins emitting binary-framed control mode text
  lines wrapped inside MSG_OUTPUT messages.
- Message types visible in the installed binary (from `strings /opt/homebrew/bin/tmux`):
  MSG_FLAGS, MSG_DETACH, MSG_EXEC, MSG_EXIT, MSG_EXITED, MSG_READY, MSG_SHELL,
  MSG_SHUTDOWN, MSG_SUSPEND, MSG_LOCK, MSG_VERSION, MSG_WRITE_OPEN, MSG_WRITE,
  MSG_WRITE_CLOSE, MSG_WRITE_READY, MSG_READ_OPEN, MSG_READ_CANCEL, MSG_READ_DATA,
  MSG_READ_DONE.
- The protocol version integer is embedded in the binary and changes between major tmux releases.
  [TRAINING_ONLY — confirm version numbering convention]

**Effort to implement in Go**:
1. Read and understand ~500 lines of `tmux_client.c` + `proc.c` + `imsg.c` from the tmux source.
2. Implement the `imsg` framing in Go (read/write with length prefix, 4-byte alignment, ancillary
   fd passing for `MSG_WRITE_OPEN` / `MSG_READ_OPEN`).
3. Negotiate the version handshake correctly.
4. Implement at minimum: MSG_VERSION, MSG_FLAGS, MSG_EXEC or MSG_COMMAND, MSG_OUTPUT,
   MSG_EXIT, MSG_EXITED.
5. Test against tmux 3.3, 3.4, 3.5, 3.6 to verify protocol compatibility.

**Estimated effort**: 2–4 weeks for a robust, tested implementation. High ongoing maintenance.

### Option 2 — CGo bindings to tmux source compiled as a library

Compile tmux source as a `.a` archive and call its internal functions via CGo.

**Practical blockers**:
- tmux is not architected as a library. Its source is a server binary that forks and exec's;
  there is no `libtmux.h` or stable API surface.
- The tmux project has explicitly rejected the library approach upstream.
- CGo disables cross-compilation, adds OS/arch-specific build constraints, and slows build times.
- Maintenance nightmare: every tmux version update requires recompilation and API compatibility
  verification.

**Verdict**: Not viable for production use.

### Option 3 — Python `libtmux` as an IPC sidecar

The Python library `libtmux` (github.com/tmux-python/libtmux) is not a socket client — it wraps
the `tmux` CLI via subprocess. This option does not eliminate subprocess spawning; it merely moves
it into a Python process. Not relevant.

### Option 4 — Control mode stdin pipe (the actually correct IPC path)

The **officially documented** bidirectional IPC mechanism is control mode (`tmux -C`). The tmux
project documents this on the tmux wiki and in the man page. Commands sent over stdin get
`%begin`/`%end` framed responses. This is what iTerm2, Kitty, and other serious tmux-aware
terminals use for programmatic control.

**Key distinction from raw socket IPC**: control mode speaks text (same syntax as the tmux
command-line) over a persistent subprocess's stdin/stdout pipes. This subprocess is already
running in stapler-squad (`controlModeCmd`, `controlModeStdin`, `controlModeStdout`). The
cost of the persistent subprocess is incurred once per session — but zero additional forks are
needed for any query operation once commands are piped through stdin.

See `findings-control-mode-commands.md` for the full protocol analysis and implementation plan.

---

## Trade-off Matrix

| Option | Subprocess overhead | Implementation effort | Protocol stability | Go library support | Notes |
|--------|--------------------|-----------------------|-------------------|-------------------|-------|
| Current (fork per call) | ~3–8 ms per call; 138/s aggregate | None (already done) | Stable (just shell) | Native `exec.Command` | Baseline; the problem to solve |
| Control mode commands (stdin pipe) | Zero forks for queries; one persistent process per session | Low (1–3 days; infrastructure already exists) | Stable; documented; used by iTerm2/Kitty | None needed — plain text protocol over pipes | **Recommended path** |
| Raw Unix socket IPC | Zero forks; no persistent process needed | Very high (2–4 weeks + ongoing maintenance) | Unstable; changes between tmux versions without notice | None exist; must write from scratch | Over-engineered; negative ROI |
| CGo bindings to tmux source | Zero forks | Extremely high; not viable | None (tmux is not a library) | None | Not viable |

---

## Risk and Failure Modes

### Raw Socket IPC Risks

**Protocol versioning breakage**: The `imsg` protocol version integer is encoded in both the
client and server binaries and must match. tmux increments this version on incompatible changes.
If a user upgrades tmux and stapler-squad has not caught up, connections will be rejected with
`MSG_EXITED` after a failed version handshake. This is a hard failure mode with no graceful
degradation possible unless a subprocess fallback is retained — negating the complexity savings.

**Platform divergence**: The socket path, socket permissions, and ancillary fd behavior differ
between Linux and macOS. On macOS, `SO_PEERCRED` is not available; socket fd passing uses
`SCM_RIGHTS`. The imsg implementation in tmux's bundled copy may diverge from the system
`libutil` version on some Linux distros. [TRAINING_ONLY — verify macOS imsg bundling in tmux]

**Authentication / permissions**: tmux sockets are `0600` (owner-only). A Go program running as
the same user can connect, but if the app ever runs as a different user or in a container context,
socket access fails. The CLI subprocess approach is unaffected because tmux itself handles auth.

**Server state corruption risk**: Sending malformed `imsg` messages to the server can crash the
server process, killing all sessions. With the CLI approach, a malformed command produces an error
exit code and no server-side damage.

### Control Mode Risks (for comparison)

**Process lifecycle coupling**: If the control mode subprocess crashes, the IPC channel is lost
until restarted. The existing code already handles `%exit` and restart logic.

**Backpressure on stdin**: Writing too many commands without reading responses can deadlock if
the stdin buffer fills. Needs async write with timeout or a command rate limit — addressed in
findings-control-mode-commands.md.

---

## Migration and Adoption Cost

### Raw Socket IPC (not recommended)

- Implement `imsg` framing in Go: ~500 lines of carefully tested code.
- Implement message type constants, handshake, command dispatch, response demux: ~1000 more lines.
- Write compatibility tests for tmux 3.3–3.6: ~2 weeks.
- Ongoing cost: test against every new tmux release. Likely requires a version negotiation layer
  or explicit version pinning in the application's config.

**Total**: 2–4 engineer-weeks initial + ~1 day per tmux release for compatibility verification.

### Control Mode Commands (recommended)

- Wire a request/response multiplexer into the existing `controlModeStdin` /
  `processControlModeLine()` path.
- Replace 9 subprocess call sites with `SendControlModeCommand()` calls.
- **Total**: 1–3 engineer-days. No new dependencies. No ongoing versioning cost.

---

## Operational Concerns

**Observability**: The raw socket protocol is binary and opaque. Debugging a misbehaving client
requires running `tcpdump`/`socat` on the Unix socket and decoding `imsg` frames manually.
Control mode is plaintext and trivially observable with `tee` on stdin/stdout or by enabling
debug logging on the scanner goroutine.

**Testing**: Raw socket IPC requires a live tmux server for every test. Control mode also
requires a live server for integration tests, but the text protocol is trivially mockable with a
`bufio.Scanner` on a `bytes.Buffer` or `net.Pipe`.

**Graceful degradation**: With raw socket IPC, if the protocol breaks on a tmux upgrade, the
only fallback is subprocess spawning — requiring a parallel code path to be maintained forever.
With control mode, if the stdin pipe gets wedged, the existing circuit breaker executor
(`executor/`) already provides subprocess fallback with no extra code.

**Concurrency**: The raw socket `imsg` protocol does not provide out-of-order response delivery;
the client must serialize requests or implement its own request-id multiplexing. Control mode
provides explicit `%begin TIME MSGID`/`%end TIME MSGID` framing that enables safe concurrent
pipelining with a map of pending requests keyed on MSGID.

---

## Prior Art and Lessons Learned

**iTerm2**: Uses control mode exclusively for all programmatic tmux interaction. Does not speak
the raw socket protocol. The iTerm2 source (Objective-C) uses `tmux -CC` (double `-C` for
unescaped output) and pipes commands over stdin. This is the authoritative existence proof that
control mode is sufficient for complete programmatic tmux control.
[TRAINING_ONLY — confirm iTerm2 uses -CC not raw socket]

**Kitty terminal**: Similarly uses control mode for tmux integration; no raw socket protocol.

**tmuxp / libtmux (Python)**: Both wrap the `tmux` CLI via subprocess. Neither attempts raw
socket IPC. The libtmux author has been asked about native socket communication and declined
on grounds of maintenance burden. [TRAINING_ONLY — verify libtmux maintainer position]

**tmux-go (github.com/derekmarcotte/tmux-go)**: A Go library that wraps the tmux CLI via
subprocess. Does not implement the socket protocol. Shows the Go community has found subprocess
wrapping sufficient. [TRAINING_ONLY — verify this library exists and its approach]

**No known production Go implementation of the raw tmux socket protocol exists** as of the
knowledge cutoff date (August 2025). The absence of such a library after 15+ years of tmux
existence is itself strong evidence that the cost/benefit ratio does not justify it.

**OpenBSD imsg**: The `imsg` framing library is well-documented in OpenBSD man pages and its
source is public. A pure-Go reimplementation of the framing layer would be ~200 lines and is
technically straightforward. The obstacle is not `imsg` itself but the undocumented,
version-specific message type integer assignments and payload layouts inside tmux.

---

## Open Questions

1. **Does tmux's protocol version integer change between minor versions or only major versions?**
   This determines how often a raw socket client would break in practice.
   [Pending web search: `tmux PROTOCOL_VERSION history git log site:github.com/tmux/tmux`]

2. **Is there a `protocol.h` or equivalent in the tmux source that could be machine-parsed to
   generate Go constants automatically, reducing ongoing maintenance cost?**
   [Pending web search: `tmux source protocol.h MSG_ constants site:github.com/tmux/tmux`]

3. **Does the tmux project have any stated intention to stabilize or document the socket
   protocol?** (Almost certainly not, but worth confirming.)
   [Pending web search: `tmux stable IPC API socket protocol stabilization feature request`]

4. **Is there a Rust or C project that has successfully implemented raw tmux socket IPC that
   could be referenced for message layout details?**
   [Pending web search: `tmux raw socket protocol implementation reverse engineer imsg`]

5. **Does the existing server-level `TmuxServerRegistry` control mode connection (keepalive
   session) accept commands for arbitrary sessions via `-t SESSION` targeting?**
   This is the key open question for whether a single control mode connection can replace all
   per-session subprocess calls — answered in findings-control-mode-commands.md Option C.

---

## Recommendation

**Do not implement raw Unix socket IPC for tmux.**

The cost-benefit is decisively negative compared to control mode:

- The raw socket protocol is undocumented, unstable across versions, and binary — hard to debug,
  test, and maintain.
- No Go library implements it; building from scratch takes 2–4 weeks versus 1–3 days for the
  control mode commands approach.
- The performance gain over control mode is near-zero for this workload: both eliminate per-query
  subprocess forks. The only remaining overhead in control mode is the one-time startup of the
  persistent subprocess per session — which is already paid for output streaming and cannot be
  eliminated without raw socket IPC anyway.
- iTerm2, Kitty, and every serious tmux-aware application uses control mode, not raw sockets.
  This is the ecosystem's consensus after 15 years.

**The correct path is Option A from findings-control-mode-commands.md**: send commands over the
existing `controlModeStdin` pipe and demultiplex `%begin`/`%end` responses in
`processControlModeLine()`. This eliminates subprocess overhead with 1–3 days of work, zero new
dependencies, and no ongoing versioning risk.

---

## Pending Web Searches

1. `tmux PROTOCOL_VERSION history git log site:github.com/tmux/tmux`
   — Verify how often the internal protocol version integer changes.

2. `tmux MSG_COMMAND MSG_EXEC imsg protocol reverse engineer site:github.com`
   — Find any existing partial implementations or protocol documentation by third parties.

3. `tmux stable socket API IPC stabilization feature request`
   — Confirm there is no upstream effort to stabilize the protocol.

4. `iterm2 tmux integration control mode source site:github.com/gnachman/iTerm2`
   — Confirm iTerm2 uses control mode (not raw socket) as the canonical reference implementation.

5. `libtmux raw socket protocol python issue site:github.com/tmux-python/libtmux`
   — Confirm the libtmux maintainer's stated position on socket IPC.

6. `go tmux library socket protocol site:pkg.go.dev OR site:github.com`
   — Confirm the absence of any production Go raw-socket-protocol tmux library.
