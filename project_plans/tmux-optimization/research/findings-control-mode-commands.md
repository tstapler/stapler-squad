# Findings: tmux Control Mode Command Protocol

## Summary

tmux control mode (`-C`) is a bidirectional text protocol. Commands are sent over stdin as plain
text (same syntax as the tmux command-line) terminated with newlines. Responses are framed by
`%begin`/`%end` (success) or `%begin`/`%error` (failure) pairs that carry a monotonically
increasing integer MSGID matching the order of commands. The protocol allows full pipelining:
multiple commands can be in flight simultaneously; each response block is tagged with the MSGID of
the command that triggered it, so concurrent goroutines can demultiplex responses without
serialization. This is sufficient to replace **all nine subprocess call sites** listed in the
research brief.

The codebase already has `controlModeStdin io.WriteCloser` open and a
`processControlModeLine()` / `%begin` / `%end` handler that currently returns early. The migration
path is to wire a request/response multiplexer into that existing infrastructure.

---

## Options Surveyed

### Option A — Serialize over existing per-session control mode stdin

The `TmuxSession` already runs `tmux -C attach-session -t SESSION`. Commands sent on its stdin
target that specific session directly. This is the lowest-friction path: one struct holds the
pending-request map, all commands are funneled through it.

**Upside**: zero new processes; reuses code already written.
**Downside**: the control mode process is per-session (for output streaming), so it exists only
while a session is active. For commands like `SetWindowSize` / `RefreshClient` that may be called
before streaming starts, we must ensure the process is running. Control mode started for output
streaming will deliver `%begin`/`%end` responses interleaved with `%output` lines — the reader
goroutine must handle both.

### Option B — Dedicated command-only control mode connection per session

Start a second `tmux -C attach-session -t SESSION` purely for command dispatch. No `%output`
events to handle; cleaner separation of concerns.

**Upside**: simpler reader state machine (no `%output` mixed in).
**Downside**: doubles the number of persistent control-mode processes (one per session for output
streaming + one for commands). At ~138 subprocesses/s currently, the existing streaming connection
already accounts for the bulk of load; adding more persistent processes is counterproductive.

### Option C — Route commands through the server-level `TmuxServerRegistry` connection

The registry runs `tmux -C attach-session -t keepalive` (a server-wide connection, not
session-scoped). Commands addressed to a specific session (`-t SESSION`) sent over this connection
should be forwarded by the tmux server correctly. [TRAINING_ONLY — verify that `-t` flag in a
command sent over a non-target session's control-mode connection is honored]

**Upside**: one connection shared across all sessions.
**Downside**: the registry's reader (`readLines`) must be extended to handle `%begin`/`%end`
framing; concurrent response correlation becomes global state. Adds complexity to code that
currently only handles lifecycle events.

### Option D — Use tmux `source-file -` or `load-buffer`

Not applicable; these are for configuration injection, not query operations.

### Chosen Direction

**Option A** is recommended as the primary path for session-scoped operations (`capture-pane`,
`display-message`). The reader goroutine in `readControlModeOutput()` is already scanning all
lines; extending it to dispatch `%begin`/`%end`/`%error` blocks to a pending-request map is
minimal code. **Option C** is worth a follow-up investigation for `RefreshClient` and
`SetWindowSize` (which are fire-and-forget and have lower correctness requirements) because those
could be routed through the already-running registry connection without adding new processes.

---

## Trade-off Matrix

| Dimension | Current (subprocess) | Option A (per-session stdin) | Option B (2nd CM conn) | Option C (registry conn) |
|---|---|---|---|---|
| Subprocess forks/s | ~138 | 0 (for covered cmds) | 0 | 0 |
| New persistent processes | 0 | 0 (reuses existing) | +1 per session | 0 |
| Code complexity | Low (exec.Command) | Medium (request map + reader extension) | Medium | High (global demux) |
| Correctness risk | Low | Low | Low | Medium |
| Latency | ~1–5ms/fork | <0.1ms (pipe write) | <0.1ms | <0.1ms |
| Works when CM is not running | Yes | No (must start CM first) | No | No |
| Handles `capture-pane -S -E` | Yes | Yes [TRAINING_ONLY — verify] | Yes | Yes |
| Timeout handling | OS-level (context) | Must implement explicit timeout | Must implement | Must implement |

---

## Risk and Failure Modes

### 1. MSGID counter and response ordering

tmux control mode assigns MSGIDs sequentially starting at 1 per connection lifetime.
[TRAINING_ONLY — verify exact numbering]. Each `%begin TIME MSGID FLAGS` header identifies which
command the following output belongs to. Matching by MSGID is safe for concurrent requests, but
**only if** we maintain an atomic counter on the sender side and store `chan result` in a
`sync.Map` keyed by MSGID.

**Failure mode**: if MSGID is parsed as a string that doesn't match the stored key (e.g., due to
a base-10 vs base-8 formatting discrepancy), responses will be orphaned and callers will hang until
a timeout fires.

### 2. Mixed `%output` and `%begin`/`%end` on the same stream

The current `readControlModeOutput()` receives both `%output` lines (streaming) and `%begin`/`%end`
frames (command responses) on the same stdout. The switch statement today returns early on
`%begin`/`%end`. After the change, that case must dispatch to the pending-request map. Since
`bufio.Scanner` is single-reader, no locking is needed on the scan loop itself; locking is only
needed on the pending-map writes/reads.

### 3. Multi-line response bodies

`capture-pane -p` output spans many lines between `%begin` and `%end`. The reader must buffer all
lines between those markers and deliver the complete body as one response. The current scanner loop
processes one line at a time, so state machine logic is needed:

```
state: idle       → on %begin:     state = collecting(MSGID), reset body buffer
state: collecting → on body line:  append to buffer
state: collecting → on %end:       dispatch body to pending chan, state = idle
state: collecting → on %error:     dispatch error to pending chan, state = idle
```

This state machine must handle `%output` lines arriving during collection (they belong to the
output stream, not to the command response; broadcast them normally).

### 4. Control mode process not running at call time

`capture-pane` may be called before `StartControlMode()` (e.g., `HasUpdated()` is called on a
session that hasn't started streaming yet). Options:
- Lazy-start: if `controlModeCmd == nil`, start it; add a short startup wait.
- Fallback: if CM not running, fall back to subprocess. This maintains the current behavior for the
  non-streaming path.

The fallback is the safer migration: new path active only when `controlModeCmd != nil`.

### 5. Timeout / hung commands

If tmux does not respond with `%begin`/`%end` (e.g., malformed command, tmux bug), the caller
goroutine will block. Each pending request must have a `context.Context`-based timeout (recommend
defaulting to 3–5s, matching `sessionExistsTimeout`). On timeout, remove the entry from the
pending map and return an error. If the response arrives late (after timeout), the goroutine that
does the dispatch will find no waiting channel and should silently discard.

### 6. `%error` frame format

When a command fails, tmux emits [TRAINING_ONLY — verify exact format]:
```
%begin TIME MSGID FLAGS
%error TIME MSGID FLAGS
```
Some sources suggest `%error` replaces `%end`, others suggest it is emitted instead of output lines
before `%end`. The error description may appear on lines between `%begin` and `%error`, or the
`%error` line itself may carry a message. Defensive parsing: treat any content between `%begin` and
`%error` as the error message body.

### 7. `resize-window` and `refresh-client` over control mode

These are fire-and-forget in the current implementation (errors are logged but not fatal). They can
be sent over control mode stdin without waiting for `%begin`/`%end` — just write and move on. If
confirmation is wanted, await the response. For the initial migration, fire-and-forget is acceptable
and avoids adding latency to resize hot paths.

### 8. `display-message` format string handling

The format string (`#{pane_width} #{pane_height}`) must be passed as a single argument with proper
quoting. Over stdin, the tmux command parser treats the entire line as the command, splitting on
whitespace. Format strings containing spaces must be quoted. [TRAINING_ONLY — verify quoting rules
for control mode stdin; some docs suggest single-quotes work, others suggest the parser is simpler
than the shell parser and quoting is unnecessary].

---

## Migration and Adoption Cost

### Estimated implementation scope

1. **`TmuxSession` struct** — add `pendingCmds sync.Map` (keyed by MSGID `uint64`) and
   `cmdCounter atomic.Uint64`.

2. **`processControlModeLine()`** — extend the `%begin`/`%end`/`%error` cases with state machine
   logic. Approximately 50–80 lines.

3. **`sendControlModeCommand(ctx, args...)`** — new method that:
   - Atomically increments `cmdCounter`
   - Writes `fmt.Sprintf("%s\n", strings.Join(args, " "))` to `controlModeStdin`
   - Registers a `chan result` in `pendingCmds`
   - Selects on `<-resultCh` and `<-ctx.Done()`
   - Returns `([]byte, error)`
   Approximately 40 lines.

4. **Refactor each call site** (9 functions) to call `sendControlModeCommand` when CM is running,
   falling back to `cmdExec.Output()` otherwise. Each refactor is 5–15 lines.

5. **Tests** — unit tests for the state machine transitions (mock stdin/stdout); integration test
   firing multiple concurrent commands and verifying responses are correctly demultiplexed.

**Total estimated LOC**: 200–350 lines new/modified, excluding tests.

### Rollout strategy

1. Implement `sendControlModeCommand` behind a feature flag (`STAPLER_SQUAD_CM_COMMANDS=true`).
2. Migrate one low-risk function first (`GetPaneDimensions` — small output, easy to verify).
3. Run both paths in parallel temporarily; log discrepancies.
4. Once stable, migrate remaining 8 functions and remove the flag.

---

## Operational Concerns

### Backpressure

The current `controlModeStdin` writer is unbuffered (`io.WriteCloser` from `cmd.StdinPipe()`). At
138 commands/s, the OS pipe buffer (64KB on Linux, 65536 bytes on macOS) should absorb bursts
without blocking the writer. Each command is ~50 bytes; the buffer can hold ~1300 commands at once.
No explicit rate limiting is needed initially.

### Session-scoped connection lifecycle

`StartControlMode()` must be called before any `sendControlModeCommand()` call. The existing code
starts it from `session/instance.go` on session creation. The `StopControlMode()` path must drain
`pendingCmds` and return errors to all waiting callers before cleaning up stdin/stdout.

### Goroutine leak prevention

When `StopControlMode()` closes `controlModeStdin`, tmux will flush any in-flight responses and
then send `%exit`. The reader goroutine will then hit EOF, close subscriber channels (existing
code), and return. Any pending commands that haven't received responses must have their channels
closed with an error at that point. The existing `controlModeSubMu` locking pattern can be extended
to cover `pendingCmds` cleanup.

### Observability

Add a metric counter or debug log line per control-mode command dispatched vs. subprocess fallback.
This makes it easy to verify the fork rate has dropped after rollout.

---

## Prior Art and Lessons Learned

### Common Go tmux control mode client pattern

Several Go projects implement tmux control mode clients. The common pattern:
- `bufio.Scanner` on stdout
- Monotonic integer MSGID counter
- `map[int]chan<- Response` for pending requests
- State machine with `inResponse bool` + body accumulation
[TRAINING_ONLY — verify exact library names and API surface for any that could be vendored]

### Lessons from wezterm / zellij control mode clients

Rust terminal emulator projects have implemented similar request/response multiplexers over tmux
control mode sockets. The key lesson from those implementations: **always handle `%output` events
during command response collection**; failing to do so causes `%output` events to be mistaken for
command output and corrupts the response body.

### The existing `TmuxServerRegistry.readLines()` pattern

The registry already has a production-quality event-reading loop with reconnect logic. It does NOT
do request/response (only one-way events), but its structure is a useful reference for the reader
goroutine extension.

### The `%begin`/`%end` framing (known format from tmux source)

From the tmux source (`server-client.c`, `cmd-queue.c`) [TRAINING_ONLY]:
```
%begin <time_secs> <msgid> <flags>
... output lines (zero or more) ...
%end <time_secs> <msgid> <flags>
```
On error:
```
%begin <time_secs> <msgid> <flags>
... optional error description lines ...
%error <time_secs> <msgid> <flags>
```
`<time_secs>` is Unix time as a decimal integer. `<msgid>` is a decimal integer matching the
command sequence number. `<flags>` is always `0` in modern tmux (reserved for future use).

Command format sent over stdin: one command per line, same syntax as the tmux command prompt.
Example:
```
capture-pane -p -e -t staplersquad_mysession
display-message -p -t staplersquad_mysession "#{pane_width} #{pane_height}"
resize-window -t staplersquad_mysession -x 220 -y 50
```

---

## Open Questions

1. **Does `-S` / `-E` (line range) work with `capture-pane` over control mode stdin?** The
   `CapturePaneContentWithOptions` call passes `-S start -E end`. These flags are documented for
   the `capture-pane` command and should work regardless of invocation method, but this needs
   verification against a running tmux 3.2+ instance.

2. **Quoting rules for format strings**: Does `display-message -p -t TARGET "#{pane_width}
   #{pane_height}"` parse correctly over control mode stdin? The tmux control mode parser may
   handle quoting differently from the shell. Need to test with and without quotes.

3. **MSGID numbering**: Is the MSGID in `%begin`/`%end` the sequence number of the command (1,
   2, 3...) or some other identifier? Can we safely use the send-side counter as the lookup key?
   [TRAINING_ONLY — verify against tmux source]

4. **`%error` exact format**: Does the error text appear as lines between `%begin` and `%error`,
   or is it appended to the `%error` line itself? Need to test with an intentionally malformed
   command (e.g., `capture-pane -t nonexistent-session`).

5. **Behavior when `attach-session` target is the same session as the one being queried**: The
   current `StartControlMode` does `attach-session -t SESSION`. When we send `capture-pane -t
   SESSION` on that same connection, does tmux treat the target correctly or does it produce
   unexpected behavior due to the recursive attach?

6. **`refresh-client` over control mode**: This command targets a client, not a session. In
   control mode, the "client" is the control-mode process itself. Behavior of
   `refresh-client -t SESSION` may differ from subprocess invocation. Needs testing.

7. **Option C viability**: Can the `TmuxServerRegistry` connection (attached to `keepalive`) be
   used to send commands targeting other sessions (e.g., `capture-pane -t SESSION`)? If yes, a
   single server-level connection could replace all per-session command dispatching.

---

## Recommendation

**Implement Option A** (extend per-session control mode stdin) with the fallback to subprocess when
control mode is not running. The 9 call sites reduce to zero subprocess forks when control mode is
active (the common case during active session monitoring). The fallback preserves correctness for
edge cases (session starting up, control mode restarting after a tmux crash).

The key implementation steps in priority order:
1. Add the state machine to `processControlModeLine()` for `%begin`/`%end` body accumulation.
2. Implement `sendControlModeCommand(ctx, args...)` with MSGID demultiplexing.
3. Migrate `GetPaneDimensions` and `GetCursorPosition` first (small output, easy to verify).
4. Migrate `capture-pane` variants once the state machine is verified under load.
5. Migrate `RefreshClient` and `SetWindowSize` last (fire-and-forget semantics allow looser
   correctness guarantees during migration).

Do **not** attempt to route through the `TmuxServerRegistry` connection until the per-session path
is proven — it adds global state complexity with unclear benefit.

---

## Web Search Results

### Protocol ordering (confirmed)
From [tmux Control Mode Wiki](https://github.com/tmux/tmux/wiki/Control-Mode):
> "The time and command number for %begin will always match the corresponding %end or %error, although **tmux will never mix output for different commands** so there is no requirement to use these."

**Implication**: A simple FIFO channel queue is sufficient — no MSGID map needed. Responses arrive in the same order commands were sent.

### %begin/%end format (confirmed from wiki)
```
%begin <unix_time_secs> <command_number> <flags>
... output lines ...
%end <unix_time_secs> <command_number> <flags>
```
On error: `%error` replaces `%end`. Flags are always `0` in current tmux.

### capture-pane -S/-E (confirmed)
`capture-pane -S -50 -E -10 -p` works in tmux command syntax. Since control mode stdin accepts the same syntax as the tmux command prompt, `-S`/`-E` flags work unchanged over control mode stdin.

### Go libraries (none implement control mode command dispatch)
- [gotmux](https://github.com/GianlucaP106/gotmux) — subprocess-based, no CM command dispatch
- [go-tmux](https://github.com/jubnzv/go-tmux) — subprocess-based
- [gomux](https://github.com/wricardo/gomux) — subprocess-based

No existing Go library implements the request/response multiplexer over control mode stdin. Must implement custom.

### Sources
- [Control Mode · tmux/tmux Wiki](https://github.com/tmux/tmux/wiki/Control-Mode)
- [tmux(1) man page](https://man7.org/linux/man-pages/man1/tmux.1.html)
- [ghostty discussion #2839](https://github.com/ghostty-org/ghostty/discussions/2839)
- [gotmux](https://github.com/GianlucaP106/gotmux)

---

## Pending Web Searches

Run these searches to validate training-only claims before implementation:

1. `tmux control mode %begin %end MSGID format "site:github.com/tmux/tmux"`
   — Verify exact MSGID numbering and `%error` format from tmux source (`server-client.c`).

2. `tmux control mode stdin command format quoting "display-message" format string`
   — Verify whether quotes are needed around format strings like `#{pane_width} #{pane_height}`.

3. `tmux "capture-pane" "-S" "-E" control mode stdin send`
   — Confirm `-S`/`-E` line range flags work when sent over control mode stdin.

4. `go tmux control mode client library "msgid" OR "msg_id" site:github.com`
   — Find existing Go implementations that could be referenced or vendored.

5. `tmux control mode "%error" format "error message" lines before after`
   — Determine whether error text appears before or on the `%error` line.

6. `tmux control mode "refresh-client" attach session same session target`
   — Verify `refresh-client -t SESSION` behavior when sent from the control mode connection that is
   itself attached to that session.

7. `tmux control mode stdin "attach-session" same session "capture-pane" recursive`
   — Confirm there is no issue querying the same session that the control mode is attached to.

8. `site:github.com "tmux" "-C" "attach-session" "capture-pane" stdin "pending"`
   — Find real-world Go/Rust/Python implementations that multiplex commands over control mode.
