# Research Findings: Pitfalls — Embedding a Process Supervisor in a Go Application

Status: Complete | Last verified: 2026-05-22
Date: 2026-04-29
Input: requirements.md, codebase audit (session/, session/tmux/, daemon/, session/mux/)

## 2026-05-22 Update: Executor Framework Changes F1 Risk Profile

The `executor/` package (merged from main 2026-05-14) provides infrastructure that reduces F1/F2 risks for NativeProcessManager:

- `executor.ManagedProcess` — handles `cmd.Wait()` in its internal reaper goroutine; if used for non-PTY subprocesses, eliminates zombie risk for those calls. **Not usable for PTY launch** (pipe-based only).
- `executor.ShortLivedCmd` — one-shot subprocess wrapper; safe for metadata queries (GetPanePID equivalent, CWD checks).
- `norawexec` lint rule (`tools/lint/norawexec/`) — custom go/analysis pass that enforces `safeexec.CommandContext()` usage; bare `exec.Command`/`exec.CommandContext` outside the `executor` and `executor/safeexec` packages fails the build. PTY launch (`pty.Start(cmd)`) must carry `//nolint:norawexec long-running PTY process; WaitDelay not applicable` to pass the linter.

**Revised F1 guidance for NativeProcessManager:** Use `executor.ShortLivedCmd` for all short-lived subprocess calls. For the PTY process itself, the `//nolint:norawexec` exemption applies and the Wait() goroutine discipline from NM-1 is still required.

---

## 1. Summary

Embedding a process supervisor inside a long-running Go application introduces five distinct failure
mode categories. The stapler-squad codebase has already encountered and fixed the two most dangerous
ones (zombie accumulation — see docs/zombie-process-fix.md; double-start race — see
ADR-002-zombie-reconciliation.md). The remaining three are structurally new risks that arise
specifically from replacing Tmux with an embedded supervisor: PTY ownership ambiguity, signal
propagation across the supervisor boundary, and test isolation breakdown from shared global signal
handlers and goroutine leaks.

The central tension is that a process supervisor naturally wants to own the entire process lifecycle
(spawn, monitor, restart, kill), but stapler-squad also wants to own it for state management
purposes. Reconciling those two ownership claims without creating split-brain is the core
architectural challenge.

Key verdict: the pitfalls are solvable, but several require non-negotiable design commitments before
the first line of implementation code is written. Those commitments are called out explicitly in
Section 9.

---

## 2. Options Surveyed — Failure Mode Categories

Five failure categories are examined, ranked roughly by historical frequency in this codebase and
by impact severity:

| # | Category | Source |
|---|---|---|
| F1 | Zombie process accumulation | cmd.Start() without Wait()/Release() |
| F2 | Double-start race / false zombie detection | Concurrent goroutines calling start() |
| F3 | PTY ownership ambiguity | Supervisor restart replaces process; PTY fd points at dead master |
| F4 | Signal propagation failure | SIGTERM not forwarded to child; supervisor absorbs or drops signals |
| F5 | Test isolation breakdown | Global signal.Notify channels, goroutine leaks across test cases |

Categories F1 and F2 are already fixed in the codebase. This document focuses on F3–F5 as the
net-new risks of the immortal/embedded-supervisor migration, and revisits F1–F2 in the context of
a new supervisor implementation which would re-introduce the same risks if the patterns are not
followed.

---

## 3. Trade-off Matrix

Axes:
- Severity: how bad when it hits in production (High/Med/Low)
- Detection ease: how quickly it shows up (Fast = test or immediate; Slow = hours/days of runtime)
- Mitigation availability: does a clear fix exist (Good/Partial/Poor)
- Single-binary goal impact: does the failure mode push toward or against keeping a single binary

| Failure Mode | Severity | Detection | Mitigation | Binary Goal Impact |
|---|---|---|---|---|
| F1: Zombie accumulation (new impl) | High | Slow (gradual) | Good — Release() or Wait() goroutine | Neutral |
| F2: Double-start race (new impl) | High | Fast (test) | Good — startMu pattern already proven | Neutral |
| F3: PTY ownership on restart | High | Fast (broken terminal) | Partial — fd handoff or lifecycle event | Negative — complexity grows |
| F4: Signal propagation failure | High | Slow (silent hang) | Partial — explicit forward or process group | Neutral |
| F5: Test isolation breakdown | Med | Fast (flaky tests) | Good — no globals; per-test socket/instance | Neutral |

---

## 4. Risk Failure Modes — Detailed

### F1: Zombie Process Accumulation (Re-introduction Risk)

**What happened in stapler-squad**: LaunchDaemon() called cmd.Start() and then returned without
calling cmd.Wait() or cmd.Process.Release(). When the daemon exited, its process table entry
persisted as a zombie. With frequent daemon launches, the system hit the process limit (10,662 of
10,666 slots used). Fixed by adding cmd.Process.Release() immediately after Start() for detached
processes. See: docs/zombie-process-fix.md.

**Why an embedded supervisor re-introduces this risk**: An embedded supervisor is itself a goroutine
or set of goroutines that call cmd.Start() to launch managed processes. If the supervisor's restart
loop calls Start() on a new instance before calling Wait() on the old one, the old process becomes
a zombie. This is exactly the bug that was fixed, but it will reappear in the new supervisor code
if the maintainer writes start() without the paired Wait() goroutine.

**The correct pattern** for any supervised process:
```go
// For processes you wait on (supervised):
cmd.Start()
go func() { cmd.Wait() }() // always in a goroutine, never block

// For truly detached (fire-and-forget) processes:
cmd.Start()
cmd.Process.Release()
```

**Specific risk from immortal's architecture** [TRAINING_ONLY - verify]: The `immortal` project
runs as a standalone OS daemon, not an importable Go library. If stapler-squad embeds the immortal
binary (via the same embed_tmux build-tag pattern used for tmux), the embedded binary runs its own
wait loop and stapler-squad is not responsible for Wait(). If immortal is used as a Go library
import, the internal wait goroutine must be explicitly started. If neither applies and the
supervisor logic is written directly in Go, the Wait() goroutine discipline falls to this codebase.

**Detection**: `ps aux | awk '$8=="Z"'` count growing over time. go.uber.org/goleak in TestMain
catches leaked goroutines from Wait() failures in tests.

---

### F2: Double-Start Race (Re-introduction Risk)

**What happened in stapler-squad**: The health checker and the server restore path both call
Start(false) on the same Instance simultaneously. Both read i.started == false before either sets
it to true. Both attach a ptmx file descriptor and start a streamLoop goroutine. Fixed by adding
startMu sync.Mutex with double-checked locking. See: ADR-002-zombie-reconciliation.md.

**Why an embedded supervisor re-introduces this risk**: The supervisor has its own restart trigger
(process exit event). If the supervisor fires a restart callback at the same time the health
checker or an operator calls Start(), the same double-start race reappears with a third concurrent
caller. The startMu pattern must be applied to ProcessManager.Start() wherever it can be called
concurrently.

**The correct pattern**:
```go
func (s *SupervisedSession) Start() error {
    s.startMu.Lock()
    defer s.startMu.Unlock()
    if s.started { return nil } // double-check inside lock
    s.started = true
    // ... proceed
}
```

**Specific new risk**: The supervisor's internal restart goroutine is a net-new concurrent caller
that did not exist with the Tmux backend. The startMu guard must explicitly cover restart paths,
not just the initial Start().

---

### F3: PTY Ownership Ambiguity on Supervised Restart

This is the most structurally novel pitfall and the one with no existing fix in the codebase.

**The problem**: When a supervised process is restarted by the supervisor, a new OS process is
created with a new PID. The old PTY master file descriptor (ptmx *os.File) held by the session
layer now refers to the slave side of a PTY whose master process is gone. Reads on this fd either
block indefinitely or return EIO.

The web UI is streaming from this fd via the streamLoop goroutine. When the supervised process dies
and is restarted:
- The old ptmx is now orphaned. Its slave side (the process's stdin/stdout) is gone.
- If the supervisor creates a new PTY for the new process, the new ptmx fd is held only by the
  supervisor. The streamLoop is still reading the old fd.
- Result: the web UI freezes, shows a stale terminal, or the streamLoop goroutine crashes with EIO
  without notifying subscribers.

**Three possible mitigations**:

Option A — PTY fd handoff via atomic pointer swap
The supervisor hands off the new ptmx fd to the session layer on restart via an atomic pointer
swap. The streamLoop detects the swap (sentinel value or channel notification) and reopens from the
new fd.
Complexity: high. fd passing across goroutine boundaries is unsafe without careful synchronization.
The streamLoop must be restarted, not just redirected.

Option B — Named pipe / Unix socket indirection (no raw PTY fd sharing)
The session layer does not hold a PTY fd directly. Instead it reads from a Unix socket or named
pipe that the supervisor maintains. When the supervised process restarts, the supervisor re-pipes
the new PTY output into the same socket. From the session layer's perspective, the read stream
briefly pauses and resumes — no fd replacement needed.
This is how tmux works: stapler-squad attaches to the tmux session socket, not the process PTY
directly. The tmux server owns the PTY; the client reads from a separate protocol socket.
Complexity: medium. Requires the supervisor to act as a PTY proxy, not just a process launcher.

Option C — Restart fires a lifecycle event; session layer creates a new stream (recommended)
When the supervised process dies, fire EventExited (the existing LifecycleListener interface). The
auto-restart logic creates a new session (new ptmx, new streamLoop, new subscriber channels). The
web UI receives the lifecycle event and reconnects its terminal stream.
Complexity: low. This is functionally equivalent to what happens today when a tmux session dies
and the health checker detects it and calls Start(). The difference is latency: event-driven
(milliseconds) vs. polling-based (up to health-check interval).

Option C is the recommended approach for a first implementation. Option B is the right long-term
shape if "seamless restart with preserved scrollback" becomes a hard requirement.

**Codebase evidence**: The existing streamLoop in control_mode.go reads from a PTY entirely owned
by tmux, not by the supervised child process. When the tmux session is killed and restarted, the
attach-session command is re-run, creating a new ptmx. The control mode goroutine
(readControlModeOutput) exits when controlModeCmd exits; a restart requires StopControlMode +
StartControlMode in sequence. This is the pattern to follow in the new backend.

---

### F4: Signal Propagation Failure

**The problem**: Go's os/signal package registers signal handlers at the process level. When a
supervisor goroutine is embedded in the same process as the server, signals sent to the
stapler-squad process (SIGTERM, SIGINT) are delivered to the Go runtime, which dispatches them to
registered signal.Notify channels. Unless explicitly forwarded, they are not automatically
delivered to the supervised child processes.

This creates two distinct failure modes:

**F4a — SIGTERM sent to stapler-squad does not reach the supervised child**
The operator or OS sends SIGTERM to the stapler-squad process (e.g., `kill <pid>` or
`systemctl stop`). The server shuts down, but the supervised AI agent process keeps running in the
background, consuming resources and potentially holding file locks on git worktrees.

Current state: the daemon/daemon.go shutdown path handles SIGTERM for its own process (main.go
registers signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)). The tmux path avoids this
problem because tmux's kill-session propagates to all panes. An embedded supervisor does not have
this propagation automatically.

Required pattern in the supervisor's Shutdown():
```go
func (s *Supervisor) Shutdown() {
    for _, proc := range s.allManagedProcesses() {
        _ = proc.Signal(syscall.SIGTERM)
    }
    // wait for drain, then SIGKILL after timeout
    time.AfterFunc(5*time.Second, func() {
        for _, proc := range s.allManagedProcesses() {
            _ = proc.Kill()
        }
    })
}
```

**F4b — Process group leakage: supervised children inherit stapler-squad's process group**
If the supervised child is started without Setpgid: true in SysProcAttr, it joins the same process
group as stapler-squad. A SIGTERM sent to the process group (e.g., from a shell with Ctrl-C) kills
both the supervisor and the children simultaneously. The supervisor cannot perform orderly shutdown
or persist state before exiting.

Current state: daemon/daemon_unix.go correctly sets Setsid: true for the daemon process. Supervised
AI agent sessions in the new backend must similarly set Setpgid: true to decouple their process
group from stapler-squad's.

**SIGWINCH special case**: signal.Notify is additive — multiple registrations on the same signal
from different goroutines do not conflict. The existing SIGWINCH handlers in tmux_unix.go and
signals_unix.go both use it. However, if the channel buffer is 0 (unbuffered), a blocked receiver
causes the runtime to drop the signal silently. All signal.Notify channels must use buffer size
>= 1.

**SIGCHLD and Go's runtime** [TRAINING_ONLY - verify]: Go's runtime does not expose SIGCHLD to
user code via signal.Notify on most platforms — it is used internally for its own goroutine
scheduling. An embedded Go supervisor cannot rely on SIGCHLD to detect child process exit. It must
use a Wait() goroutine (blocking cmd.Wait() in a dedicated goroutine) for each supervised process.

---

### F5: Test Isolation Breakdown

**The problem**: An embedded supervisor that registers global signal handlers, starts background
goroutines, or holds shared state (e.g., a package-level map of supervised processes) breaks Go's
test isolation model. `go test ./...` runs packages in parallel. Two test packages that both
exercise the supervisor may share global state, produce goroutine leaks that fail subsequent tests,
or cause signal handler conflicts.

**Evidence that this codebase has solved the equivalent problem for tmux**: The tmux backend uses
the serverSocket field. Each test creates a unique socket name:
```go
socketName := fmt.Sprintf("test_ensure_noop_%d", rand.Int63())
t.Cleanup(func() { /* kill isolated tmux server */ })
```
This creates a fully isolated tmux server per test. Cross-test session name collisions are
impossible because each test server is on a separate socket path.

**Net-new risks from an embedded supervisor**:

Risk 5a — Global signal.Notify registration at package init time
If the supervisor calls signal.Notify at package init or in a constructor that is called
package-wide, every test that imports the package registers a new handler. In practice this is
harmless but it creates the impression that signal delivery is exclusive, and a poorly-written
supervisor that assumes exclusive SIGTERM ownership will behave incorrectly in tests where the test
harness also handles SIGTERM.

Risk 5b — Goroutine leak from a non-stopped restart loop
The supervisor's internal restart goroutine (the loop calling cmd.Wait() and deciding to restart)
must stop cleanly when the test session is torn down. If Shutdown() or Close() does not cancel the
goroutine's context, go test -v reports leaked goroutines; go.uber.org/goleak (if added to
TestMain) will fail the test suite.

Risk 5c — Shared filesystem state written to fixed paths
If the supervisor writes PID files, lock files, or socket files to fixed paths that are not
parameterized, parallel tests collide. The existing STAPLER_SQUAD_INSTANCE isolation (state under
~/.stapler-squad/instances/{INSTANCE_ID}/) helps for session-level state but not for
supervisor-internal state unless the supervisor explicitly uses the same base-dir pattern.

**Required mitigations for test isolation**:
- The ProcessManager constructor must accept a base directory for all on-disk state. Tests inject
  t.TempDir(). Production uses the workspace-specific state dir.
- The supervisor's background goroutines must all terminate on context cancellation. Use
  context.WithCancel passed at construction time.
- No package-level signal.Notify registrations. Signal setup lives in Supervisor.Start(); teardown
  lives in Supervisor.Shutdown() (signal.Stop(ch)).

---

## 5. Migration Adoption Cost

What additional work is required to handle all pitfalls correctly, beyond the core supervisor
implementation?

| Pitfall | Required Work | Rough Estimate |
|---|---|---|
| F1 re-introduction | Add Wait() goroutine to every cmd.Start() in supervisor | 1–2 hours |
| F2 re-introduction | Apply startMu + double-check to ProcessManager.Start() | 2–4 hours |
| F3 PTY ownership (Option C) | Wire lifecycle event on crash; reconnect stream on restart | 1–2 days |
| F4a SIGTERM forward | Shutdown() iterates and signals all supervised processes | 2–4 hours |
| F4b process group | Setpgid: true in all child SysProcAttr | 1 hour |
| F4 signal buffer audit | Verify all signal.Notify channel buffers >= 1 | 1 hour |
| F5a no global handlers | Enforce via code review; no package-level signal.Notify | 1 hour |
| F5b goroutine leak | Context propagation through all supervisor goroutines | 4–8 hours |
| F5c filesystem state | BaseDir injection into ProcessManager constructor | 2–4 hours |

Total estimate for pitfall mitigations alone: approximately 3–5 days of focused work, separate from
the core supervisor implementation. F3 (PTY ownership on restart) carries the most uncertainty
because it requires redesigning how the streamLoop interacts with process restart, even under the
recommended Option C (lifecycle event) approach. The remaining items are each well-bounded.

---

## 6. Operational Concerns

How to detect these problems in production before users report them:

**Zombie accumulation (F1)**:
- Metric: child process count for the stapler-squad process group growing monotonically
- Alert: `ps --ppid <pid> -o state= | grep -c Z` > 0
- Existing runbook in docs/zombie-process-fix.md applies

**Signal delivery failures (F4a)**:
- Symptom: orphaned AI agent process with working directory on a closed worktree; CPU/RAM usage
  after stapler-squad shuts down
- Detection: PID-file check on startup — read last-known PID file, verify process is not still
  running under the previous stapler-squad parent
- Log signal: supervisor logs "sent SIGTERM to pid X" at shutdown; absence of this log for an
  expected session is a detection signal

**PTY stream freeze (F3)**:
- Symptom: web UI terminal output stops updating; session status stays Running
- Detection: output timestamp tracking — alert if a Running session produces no terminal bytes for
  > 30s
- The existing status monitor already polls for session liveness; extend it to track last-output
  timestamp and emit EventExited if stale

**Goroutine leak (F5b)**:
- Metric: runtime.NumGoroutine() sampled every 60s; alert if growing unbounded without
  corresponding new sessions
- Tool: pprof goroutine endpoint already exposed by the --profile flag
  (`curl http://localhost:6060/debug/pprof/goroutine?debug=2`)
- In tests: add go.uber.org/goleak to TestMain in the supervisor package

---

## 7. Prior Art and Lessons Learned

**Docker and the zombie reaping problem**: Docker's early daemon ran as PID 1 inside containers,
which is responsible for reaping all orphaned processes. This led to the tini project
(https://github.com/krallin/tini) and Docker's --init flag, which embeds a minimal init process.
The lesson: any Go process that starts children and does not call Wait() accumulates zombies. In
containers, the implicit parent is PID 1; in stapler-squad, the implicit parent is the supervisor
goroutine. The discipline is identical. [TRAINING_ONLY - verify tini adoption timeline]

**Kubernetes SIGTERM propagation**: Kubernetes sends SIGTERM to the main container process and
expects it to propagate to children. Container workloads using shell scripts as entrypoints
commonly fail because `sh -c "exec myapp"` in some configurations does not exec (it forks), so the
actual app process is not PID 1 and does not receive the signal. The embedded supervisor equivalent:
if the supervisor catches SIGTERM on its signal.Notify channel but does not call
child.Signal(SIGTERM), the children exceed their shutdown window and are SIGKILL'd without
flushing state. [TRAINING_ONLY - verify k8s SIGTERM propagation specifics]

**Traditional init supervisors (runit, s6, supervisor.d)**: These explicitly separate supervisor
ownership (the supervisor calls waitpid) from application ownership (the application manages its
own resources). They are designed as PID 1 replacements, not embedded libraries. The design
principle is: simple supervisors are separate processes, not goroutines. This is relevant to the
question of whether to use immortal-as-embedded-library vs. immortal-as-subprocess vs. custom
in-process supervisor.

**immortal (github.com/immortal/immortal)** [TRAINING_ONLY - verify entire paragraph]: Immortal
is written in Go and runs as a standalone daemon process. It communicates with supervised processes
via Unix sockets. It tracks PIDs and calls waitpid in a loop. It forwards SIGTERM before exiting.
It does not handle PTYs — it redirects stdout/stderr to log files. The practical implication: to
use immortal, stapler-squad would launch it as a subprocess (similar to how it launches tmux),
not import it as a library. This contradicts the single-binary deployment goal unless the immortal
binary is embedded via the same embed_tmux build-tag technique. An alternative is to implement
supervisor semantics directly in Go (approximately 200–300 lines using cmd.Start() + Wait() +
restart loop), which avoids the daemon-vs-library problem entirely.

**thejerf/suture**: This library supervises goroutines, not OS processes. It handles goroutine
panics and restarts, not process lifecycle, PTYs, or signal propagation. It is not applicable
here unless the abstraction level is goroutine-level rather than process-level. [TRAINING_ONLY -
verify suture scope]

---

## 8. Open Questions

1. **Is immortal importable as a Go library, or only launchable as a subprocess daemon?**
   This is the highest-priority open question. If immortal is library-importable, it can satisfy
   the single-binary goal without embedding a binary blob. If it is daemon-only, the choice is
   between embedding the binary (like tmux) or writing a custom in-process supervisor.
   Verification: code-archaeology on https://github.com/immortal/immortal.

2. **Does the ProcessManager interface need to expose PTY handoff semantics, or is lifecycle event
   notification sufficient?**
   Option C (lifecycle event on restart) is recommended above. If "seamless terminal restart with
   preserved scrollback" is added to requirements, Option B becomes necessary and the interface
   must expose a stream abstraction, not a raw fd.

3. **What is the correct behavior when a supervised process exceeds its restart budget?**
   requirements.md says "detect crash and restart automatically" but does not specify a restart
   budget (max N restarts, exponential backoff, give-up threshold, mark Stopped permanently). This
   must be answered before writing the restart loop.

4. **How does PTY resize work after a supervised restart?**
   The current resize path (monitorWindowSize → updateWindowSize → tmux resize-window) is
   tmux-specific. For a direct PTY, resize is via pty.Setsize(ptmx, &pty.Winsize{}). After restart
   with a new ptmx, the stored lastKnownCols/lastKnownRows must be replayed to the new PTY fd.
   This is a small but non-obvious detail.

5. **Does Go's runtime intercept SIGCHLD, and at which versions?**
   Marked TRAINING_ONLY above. Must verify before relying on the absence of SIGCHLD in user code
   for supervisor design.

---

## 9. Recommendation — Non-Negotiable Mitigations Before Shipping

The following are pre-conditions that must be implemented before the supervisor backend is
considered shippable. They are non-negotiable because the failure modes they prevent are either
already proven to have occurred in this codebase (F1, F2) or are certain to occur under normal
use (F3, F4).

**NM-1: Every cmd.Start() in the supervisor implementation must be paired with a Wait() goroutine
or Process.Release().**
This is the zombie prevention contract proven necessary by the 2025-10 incident. Each new process
launch site in the supervisor must be reviewed against this rule before merge.

**NM-2: ProcessManager.Start() must use startMu with double-checked locking.**
The double-start race is proven to occur in this codebase. The supervisor's restart callback is a
third concurrent caller that makes this race more likely, not less.

**NM-3: ProcessManager.Shutdown() must iterate all supervised processes and send SIGTERM before
context cancellation completes.**
Without this, supervised AI agent processes survive stapler-squad shutdown, leaving orphaned
processes holding worktree locks.

**NM-4: All supervised child processes must be launched with Setpgid: true in SysProcAttr.**
This decouples the child's process group from stapler-squad's, preventing Ctrl-C from killing
children before the supervisor can perform orderly shutdown.

**NM-5: The supervisor must accept a context.Context at construction. All background goroutines
must exit when the context is cancelled.**
Required for both graceful shutdown (NM-3) and test isolation (F5b). No goroutine in the
supervisor may block indefinitely without a context cancellation path.

**NM-6: The ProcessManager constructor must accept a base directory for all on-disk state.**
Tests inject t.TempDir(). This prevents cross-test filesystem collisions and aligns with the
existing STAPLER_SQUAD_INSTANCE workspace isolation pattern.

**NM-7: PTY ownership transition on restart must be handled as a full session lifecycle event
(Option C), not a silent fd swap.**
The web UI must receive EventExited notification that the session has restarted and must reconnect
its terminal stream. Silent fd swapping is complex and error-prone; lifecycle events use existing
infrastructure (LifecycleListener, EventExited, EventStarted).

---

## 10. Pending Web Searches

The following queries were identified during research but could not be executed (web search
unavailable in this session). Claims marked [TRAINING_ONLY - verify] should be validated against
these searches before the planning phase commits to an implementation approach.

1. `"golang SIGCHLD os.Wait embedded supervisor"` — verify whether Go 1.22+ exposes SIGCHLD to
   user code and what the runtime uses it for internally
2. `"go PTY ownership transfer process replace supervisor"` — find prior art for PTY fd handoff
   patterns in Go supervisors
3. `"golang process supervisor signal handling SIGTERM propagation children"` — find production
   examples of explicit child SIGTERM forwarding in Go
4. `"immortal process manager known issues bugs"` — check GitHub issues for immortal to identify
   operational problems users have encountered in production
5. `"go embed process manager test isolation goroutine leak"` — find community patterns for testing
   embedded supervisors without goroutine leaks
