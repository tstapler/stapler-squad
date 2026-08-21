# Findings: Features - Comparable Tools & Patterns

Status: Draft | Phase: 2 - Research
Date: 2026-04-16
Input: requirements.md + codebase review (session/tmux/, session/controller_manager.go)

---

## Summary

Surveyed seven tools across two categories: terminal multiplexers/session managers (Zellij,
GNU Screen, tmuxinator, Wezterm, Overmind) and Go-compatible process supervisors (supervisord,
s6, goreman). The core finding is that **every mature tool separates the persistence layer from
the observation layer**. The session process runs independently of any manager; the manager
re-attaches and re-observes state on restart rather than assuming it owns the process lifecycle.

For stapler-squad's specific problem — detecting that the program inside a tmux pane has exited
while `IsStarted=true` remains cached — the directly applicable pattern is the **reconciliation
loop**: a periodic background goroutine that compares desired state (IsStarted) against observed
state (tmux session exists + pane process alive) and emits lifecycle events on divergence. This
pattern is used by all tools surveyed in some form, and it integrates cleanly with the existing
`DoesSessionExist()` infrastructure already in the codebase.

---

## Options Surveyed

| Tool | Category | Language | Key pattern |
|------|----------|----------|-------------|
| Zellij | Terminal multiplexer | Rust | Daemon model; IPC message bus; explicit pane-exit event |
| GNU Screen | Terminal multiplexer | C | Poll-based; `logfile` hook; `escape` on exit |
| tmuxinator | tmux session launcher | Ruby | No runtime monitoring; relies entirely on tmux for session survival |
| Wezterm | Terminal emulator + mux | Rust | Per-pane process watcher; OS-level wait() on child PID |
| Overmind | Procfile runner + tmux | Go | `SIGTERM` broadcast; re-reads process list on start; no reconnect |
| supervisord | UNIX process supervisor | Python | `waitpid()` exit detection; event listener protocol; statefile |
| s6 / s6-rc | Service supervision suite | C | `s6-supervise` per-service; fifo-triggered events; state in filesystem |
| goreman | Procfile runner | Go | `os.Process.Wait()` in goroutine per process; no session survival |

---

## Trade-off Matrix

### Dimension 1: Exit event delivery model

**Poll-based (GNU Screen, tmuxinator)**
- Check session existence every N seconds via external command
- Latency: up to N seconds to detect exit
- No implementation complexity; works across manager restarts
- Matches current `DoesSessionExist()` approach in stapler-squad
- Weakness: high latency; misses rapid start-exit-restart cycles

**PTY/pipe EOF (Overmind, goreman, Wezterm in-process)**
- Manager reads from the PTY or pipe; EOF signals process exit
- Latency: milliseconds — detected in the read loop
- Requires the manager process to be alive during the exit event
- Weakness: events are lost if the manager restarts while the process is still running

**OS-level wait() per child (supervisord, s6, goreman)**
- `waitpid()` or `os.Process.Wait()` in a goroutine per tracked process
- Accurate exit code capture; immediate notification
- Requires the manager to be the direct parent of the process, OR use a re-parenting trick
- Weakness: does not work when a third party (tmux) is the actual parent

**IPC message bus (Zellij)**
- Daemon process owns all panes; emits typed events to subscribers
- Latency: essentially zero; events carry pane ID + exit code
- Requires the daemon to survive all client disconnects
- Weakness: requires building or depending on a protocol daemon

**Control mode protocol (tmux -C attach)**
- stapler-squad already implements this via `StartControlMode()` / `processControlModeLine()`
- `%exit` notification is already received and logged
- The gap: `%exit` is received but **not propagated to session lifecycle** (not wired to state machine)
- This is the lowest-cost fix: the infrastructure exists; only the wiring is missing

### Dimension 2: Reconnect / survive manager restart

**tmux as persistence layer (tmuxinator, Overmind)**
- Sessions survive in tmux; manager reconnects to named sessions by name on startup
- Widely battle-tested; naming convention is the only coordination needed
- stapler-squad already uses this: `RestoreWithWorkDir()` reconnects by `sanitizedName`
- Gap: after reconnect, does the manager know the session's current health state?
  Current answer: no — `IsStarted=true` is set but pane liveness is not re-verified

**Statefile (supervisord)**
- On startup, supervisord reads a `.pid` file to decide whether to adopt or re-launch processes
- Works across restarts; exit codes and timestamps are persisted
- Adoption pattern: if PID is still in `/proc`, the process is adopted; otherwise it is restarted
- Applicable to stapler-squad: on startup, verify `DoesSessionExist()` + pane PID liveness
  before setting `IsStarted=true`

**Filesystem-based service state (s6-rc)**
- Service state is a directory tree, not in-process memory
- Supervisor restart = scan directory, re-derive state, wire supervise loops
- Very robust; overly complex for stapler-squad's needs

**No restart recovery (goreman)**
- goreman owns the process directly; if goreman exits, so do all managed processes
- Not applicable to stapler-squad (managed sessions must survive)

### Dimension 3: API surface cleanliness

**supervisord**: XML-RPC API with explicit `start`, `stop`, `restart`, `status`, `tail` methods.
Events delivered via a separate event listener protocol (stdin/stdout of a registered listener
process). Clean separation between control plane and event plane.

**s6**: No API — state is filesystem. Tools (`s6-svc`) write to FIFOs. Events are files.
Extremely robust but alien to Go idioms.

**Zellij**: Plugin API + `zellij action` CLI. Events are typed protobuf messages. Close to what
stapler-squad's `SessionController` interface aims to be.

**Overmind**: Minimal. Start/stop/restart via `overmind {start,stop,restart,connect}` commands.
No event subscription; exit triggers `SIGTERM` to all and exit.

---

## Risk and Failure Modes

### Risk 1: Control mode `%exit` not reaching lifecycle (current gap)
`processControlModeLine()` handles `%exit` with a log statement and comment "let the caller
handle cleanup." No caller does. This is a dead event. The fix is to fire an `onExit` callback
from within `processControlModeLine` when `%exit` is received.
Severity: HIGH — this is the primary driver of zombie sessions.

### Risk 2: Control mode process dying independently of the managed session
`tmux -C attach` (the control mode process) can die without the underlying tmux session dying.
This produces a control mode EOF without any `%exit` notification. The `readControlModeOutput`
goroutine already sets `controlModeExited=true` and closes all subscriber channels. But there is
no signal to the session lifecycle layer that the control mode stream ended unexpectedly.
If control mode dies but the tmux session is still alive, the session is no longer observed.
Severity: MEDIUM — session works but exits will be undetected until the next reconciliation.

### Risk 3: Reconciliation gap between manager restart and first poll cycle
On startup, `RestoreWithWorkDir()` sets `IsStarted=true` if the session is found. But if the
session's program exited between the time stapler-squad stopped and restarted, the session exists
in tmux (the shell prompt is back) but the program is not running. The reconciliation loop would
detect this on the next poll, but there is a window (up to one poll interval) where state is wrong.
Supervisord avoids this with explicit PID adoption at startup.
Severity: LOW — brief incorrect state, corrects itself.

### Risk 4: Polling interval vs. event latency trade-off
Poll-based reconciliation (the zombie detection approach) has inherent latency equal to the poll
interval. Under heavy load (many sessions), polling tmux repeatedly creates subprocess load.
A 10-second interval is reasonable for the review queue use case.
Severity: LOW — acceptable for current scale.

### Risk 5: Double-transition race
If both `%exit` (control mode event) and the reconciliation loop fire nearly simultaneously, both
could call `onExit` for the same session. Without idempotency guards on the state machine, the
second call could corrupt state. Supervisord solves this with a `STOPPED` → `STOPPING` guard state
that ignores duplicate stop events.
Severity: MEDIUM — needs careful implementation.

---

## Migration and Adoption Cost

| Approach | Effort | Risk | Disrupts existing tests? |
|----------|--------|------|--------------------------|
| Wire `%exit` → `onExit` callback | 1-2 hours | Low | No |
| Add reconciliation loop in review queue | 2-4 hours | Low | Unlikely |
| Add startup liveness re-verification | 1-2 hours | Low | No |
| Adopt supervisord-style statefile | High | Medium | Yes — state format changes |
| Replace tmux with Zellij daemon | Very High | Very High | Yes — full rewrite |
| Implement s6-style filesystem state | High | High | Yes |

The three lightweight options (wire `%exit`, add reconciliation loop, startup liveness check)
can all ship independently and combine to cover all three success criteria from requirements.md.

---

## Operational Concerns

**Concern: tmux keepalive session**
stapler-squad already creates a `staplersquad_keepalive` session to prevent the tmux server
from exiting when all user sessions are closed. This is the same pattern used by tmuxinator
(`tmux new-session -d -s main`) and Overmind. It must be preserved in any refactor.

**Concern: Circuit breaker interaction**
`DoesSessionExist()` uses a circuit breaker executor that can refuse to run tmux commands during
a server outage. A reconciliation loop calling `DoesSessionExist()` in a tight loop during
server recovery could stress the circuit breaker. The existing `recoverFromServerFailure()`
guard (single in-flight recovery) handles this, but the reconciliation loop must respect the
breaker's open state rather than hammering it.

**Concern: Session name → state mapping**
All of the tools surveyed treat the session name as the durable identifier. stapler-squad already
does this via `sanitizedName`. The reconciliation loop should also use the name, not an in-memory
pointer, when checking liveness. This ensures correctness after manager restarts.

---

## Prior Art and Lessons Learned

**Lesson from supervisord**: The most reliable exit detection approach is a tight coupling between
the state machine and the IO loop that reads from the process. supervisord's `ProcessGroup`
monitors the `waitpid()` result in a tight goroutine, transitions state to `EXITED`, and fires
event listeners. stapler-squad's `readControlModeOutput()` goroutine is the direct analog —
it reads from the control mode process and already detects stream end. The lesson: close the loop
from that goroutine into the state machine.

**Lesson from Zellij**: Zellij's architecture (daemon owns all sessions, clients connect/disconnect)
means any client can restart without session disruption, and the daemon fires typed events to
all connected clients. stapler-squad is not a daemon architecture, but the control mode process
(`tmux -C attach`) plays the role of a daemon connection. When it exits, the session lifecycle
should be notified just as Zellij notifies clients of pane exits.

**Lesson from tmuxinator**: tmuxinator does zero runtime monitoring. It creates sessions and walks
away. This works only because the operator is watching the terminal directly. stapler-squad is a
background manager; it cannot rely on operator observation. This confirms: passive monitoring
(control mode events + reconciliation) is necessary, not optional.

**Lesson from Overmind**: Overmind uses tmux as the persistence layer and `SIGTERM` as the exit
signal. It does not try to observe individual pane exit codes; it treats all exits as "done".
stapler-squad needs more fidelity (why did it exit? was it normal or abnormal?). The control mode
`%exit` event + exit code from `capture-pane` output analysis is the right direction.

**Lesson from s6**: Service state stored as filesystem structure (not in-process memory) means
the supervisor can crash and restart without losing service state. stapler-squad's JSON state
file plays this role. The lesson: on startup, state must be re-derived from both the JSON file
(desired state) and tmux reality (actual state) — not blindly trusted from the JSON file.

**Lesson from goreman**: goreman owns its processes directly; `os.Process.Wait()` gives exact
exit codes. This is simpler but requires being the parent process. Since tmux is the parent of
`claude`, goreman-style direct wait is not available. The workaround is reading exit codes from
the shell's `$?` via `capture-pane` or from the tmux `pane-exited` hook.

---

## Open Questions

1. **tmux `set-hook pane-exited`**: tmux supports server-side hooks (`set-hook -g pane-exited
   'run-shell "..."'`). This could write a file or send a signal to stapler-squad when a pane
   exits. This is a push model that does not require polling or control mode. Cost: requires
   stapler-squad to configure the hook on startup and handle the notification channel.
   Is this more reliable than control mode `%exit`? Research needed.

2. **Exit code retrieval**: `%exit` in control mode does not include the exit code of the pane's
   foreground process. tmux 3.2+ supports `#{pane_dead_status}` for capturing the exit code via
   `display-message`. Is this reliable across tmux versions in production use?

3. **Multiple panes in one session**: requirements.md implies one pane per session. If that ever
   changes, `%exit` would need to include the pane ID and the reconciliation loop would need
   per-pane tracking. Confirm scope with tyler before implementing.

4. **`set-hook` vs. control mode `%exit`**: web searches blocked; need to verify which approach
   other tools that wrap tmux actually use in production (e.g., tmux-based CI runners).

---

## Recommendation

**Adopt the three-layer detection model:**

**Layer 1 — Reactive (immediate): Wire `%exit` into `onExit` callback**
In `processControlModeLine()`, when the `%exit` case fires, call the lifecycle callback
registered by the session/Instance layer. This gives near-zero-latency exit detection when
the control mode stream is live. This is the primary detection path.

**Layer 2 — Reactive (stream-end): Wire control mode EOF into reconciliation trigger**
When `readControlModeOutput()` reaches EOF without a preceding `%exit`, this means the control
mode process died unexpectedly. Treat this as a trigger to run an immediate liveness check on
the session, and fire `onExit` if the tmux session no longer exists or the pane program is not
running.

**Layer 3 — Defensive (periodic): Reconciliation loop in the review queue poller**
A background goroutine (or added logic in `ReviewQueuePoller.getContent()`) that, for every
session where `IsStarted=true`, calls `DoesSessionExist()` and checks pane liveness. On
discrepancy, transitions state and fires `onExit`. This catches any case that Layers 1 and 2
miss (e.g., control mode never started, session died while control mode was starting).
Poll interval: 10 seconds (matches the existing review queue cadence).

**Startup guard**: In `RestoreWithWorkDir()`, after confirming `DoesSessionExist()` is true,
add a pane liveness check (e.g., `capture-pane` succeeds + pane is not dead). Only set
`IsStarted=true` if the pane is actively running the expected program. If the pane shows a shell
prompt instead of the managed program, transition to `Stopped` and log the discrepancy.

This model mirrors supervisord (reactive waitpid + periodic state reconciliation), avoids any
new external dependencies, and fits within the existing codebase without restructuring the
tmux transport layer.

---

## Pending Web Searches

Web search was unavailable during this research session. The following searches would add
confidence to specific claims:

1. `tmux set-hook pane-exited "run-shell" site:github.com` — find production examples of
   using server-side hooks for exit notification instead of control mode
2. `tmux control mode "%exit" pane dead site:man.openbsd.org OR site:github.com/tmux/tmux` —
   verify whether `%exit` carries the pane's exit code in recent tmux versions
3. `zellij pane exit event plugin architecture site:github.com/zellij-org/zellij` — confirm
   Zellij's event model for pane-exit
4. `supervisord process state machine EXITED STOPPED goroutine` — verify the double-transition
   guard idiom and whether it applies cleanly to Go channel-based state machines
