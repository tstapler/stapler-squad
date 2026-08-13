# Findings: Pitfalls - PTY/TMux Exit Detection Failure Modes

_Research date: 2026-04-16_
_Author: Research agent (training knowledge + codebase review)_

---

## Summary

Stapler-squad attaches to tmux sessions via a `creack/pty`-based `attach-session` process. Terminal output is streamed by `ResponseStream.streamLoop()`, which reads from a `PTYAccess`-wrapped `*os.File` with a 100 ms deadline. Exit detection is implicit: when the underlying tmux session exits, the PTY eventually returns EOF or an OS-level error. Five distinct failure modes exist in this path. None have a single-point fix; each requires a targeted mitigation strategy.

---

## Options Surveyed

N/A - this is a risk catalogue.

---

## Failure Mode Catalogue

### 1. PTY Close Race Conditions

**Trigger conditions**

Multiple goroutines may touch the same `*os.File` PTY handle concurrently:

- `streamLoop` reads with a 100 ms deadline.
- `PTYAccess.UpdatePTY()` swaps the internal file pointer under a write lock.
- `TmuxSession.Close()` or a recovery path calls `os.File.Close()` on the handle that `streamLoop` is currently reading.

The sequence that produces a "file already closed" panic or spurious error:

1. `streamLoop` acquires the RLock, snapshots `pty := rs.ptyAccess.pty`, and releases it.
2. A concurrent call to `UpdatePTY` or the `Restore` path closes the old fd and assigns a new one.
3. `streamLoop` calls `pty.SetReadDeadline(...)` on the stale, already-closed `*os.File`.

`PTYAccess.Close()` marks `p.closed = true` but does **not** close the underlying `*os.File` (by design, line 108 comment). The actual `os.File.Close()` lives in `TmuxSession.Close()`. This separation is intentional but creates a window: `PTYAccess.closed` is `true`, yet `streamLoop` has already snapshot-ted the non-nil `pty` pointer before the close happened. The stale snapshot escapes the read lock.

**Observed symptoms**

- `Error reading from PTY in response stream for 'X': use of closed network connection` (Linux: "bad file descriptor", macOS: "file already closed").
- These errors are currently caught in the string-matching block at lines 162–170 of `response_stream.go`, which treats them as EOF and exits cleanly. The risk is a panic path that precedes the string match — specifically, `pty.SetReadDeadline()` on a closed fd can panic on some Go runtime versions if the internal poll descriptor has been freed.

**Mitigation strategies**

1. **Snapshot under lock, use deadline before releasing lock.** In `streamLoop`, set `SetReadDeadline` while still holding the RLock on PTYAccess so the fd cannot be freed between the snapshot and the syscall.
2. **Use `sync.Once` to close the underlying fd.** Whoever closes the `*os.File` should use a `sync.Once` guard so double-close is a no-op rather than an error path.
3. **Replace the stale-pointer pattern with a method call.** Instead of `streamLoop` snapshotting `pty := rs.ptyAccess.pty`, expose `PTYAccess.ReadWithDeadline(buf, deadline)` that holds the lock for the entire read operation. The existing `PTYAccess.Read()` already does this but does not set a deadline; extend it with a deadline parameter.

---

### 2. Goroutine Leaks on PTY Read

**Trigger conditions**

`streamLoop` is blocked on `pty.Read(readBuf)` when the PTY fd is closed from another goroutine. Go's `os.File.Read` is implemented using the runtime network poller (Go 1.18+). When the fd is closed, the poller wakes the blocked reader, returning an error — typically `io.ErrClosed` or a wrapping of `EBADF`. This is the _intended_ behaviour, but two edge cases produce leaks:

1. **Deadline not set before Read.** If `SetReadDeadline` fails silently (e.g., because the fd is already being torn down), `Read` may block indefinitely with no deadline to interrupt it. The context cancellation path (`case <-rs.ctx.Done()`) only fires at the top of the `for` loop, not while the goroutine is blocked inside `Read`.

2. **`PTYAccess.closed == true` but goroutine never rechecks.** The `streamLoop` checks `closed` at the top of the loop, then reads. A close that happens while in `Read` is handled by OS wake-up (good), but a close that happens _between_ the closed-check and the `Read` call relies on the OS to wake the goroutine via fd teardown. If the fd is never actually closed (e.g., `PTYAccess.Close()` marks the flag but forgets to call `os.File.Close()` somewhere), the goroutine leaks indefinitely.

3. **`attachCmd` process not killed.** `TmuxSession.Close()` must kill `attachCmd` (the `tmux attach-session` process) as well as close the `ptmx` fd. If `attachCmd` is orphaned, the PTY slave side stays open, and EOF never arrives. `streamLoop` keeps looping on 100 ms timeouts forever. This is noted in a `CRITICAL` comment in `tmux.go` but is easy to regress.

**Observed symptoms**

- In pprof goroutine dumps: `session.(*ResponseStream).streamLoop` parked in `(*os.File).Read`, context long since cancelled.
- Memory: `ResponseStream` not GC'd; all `Subscriber` channels retained in memory.
- Log silence: no "Response stream stopped" message after the expected session exit.

**Mitigation strategies**

1. **Always kill `attachCmd` before closing `ptmx`.** Killing the process causes the kernel to hang up the PTY master, which guarantees EOF to the reader. Add a test that verifies `streamLoop` exits within 500 ms of `Close()`.
2. **Use `context`-aware read via goroutine + `select`.** Wrap the blocking `Read` in a goroutine that writes to a channel; the outer `select` includes `ctx.Done()`. This is the standard pattern for context-cancellable I/O on `*os.File`. Overhead is one goroutine per read, but it eliminates the indefinite-block risk.
3. **Add a `streamLoop` liveness tracker.** Record the last-read timestamp; a watchdog goroutine (already partially implemented as `IdleDetector`) escalates if no read or timeout fires within N×100 ms.

---

### 3. Zombie Detection False Positives

**Trigger conditions**

The health checker (`health.go:checkSingleSession`) calls `instance.TmuxAlive()`, which executes `tmux has-session`. During a tmux server restart, all `has-session` calls return "no server running" for 50–200 ms while the server reinitialises. The health checker sees every session as dead during this window and calls `instance.Start(false)` on each one.

Three overlapping events produce incorrect zombie labelling:

1. **Server restart window.** `has-session` fails with "no server running". The health checker cannot distinguish "server temporarily restarting" from "session permanently dead".
2. **Exists-cache stale read.** `DoesSessionExist()` has a 500 ms TTL cache (`existsCacheTTL`). A cached `false` from before the server restart stays `false` for up to 500 ms after the server is back. This extends the false-positive window.
3. **Keepalive session not yet created.** `CreateKeepaliveSession()` is called during `recoverFromServerFailure()`. Until it exists, a brief "no sessions" state can trip the circuit breaker's list-sessions command class.

**Additional subtlety.** The code at `tmux.go:116–122` uses `recoveryMu + recoveryInFlight` to serialise recovery. However, session-level health checks run independently from the server-level recovery path. A health check goroutine can observe "server not running", call `Start(false)` → `RestoreWithWorkDir` → `DoesSessionExist` → another "no server running", and loop into cascading recreation attempts before the server-level recovery has finished.

**Observed symptoms**

- Log: "Instance marked as started but tmux session doesn't exist" for every session simultaneously.
- Double "Successfully restored PTY connection" log lines for the same session (multiple goroutines both conclude the session is dead and start it).
- Sessions launched with stale program flags or in the wrong working directory (recreated before worktree path is verified).

**Mitigation strategies**

1. **Distinguish server-down from session-dead.** Before declaring a zombie, run a lightweight server-alive check (e.g., `tmux list-sessions -F ''` and check for "no server running"). If the server is down, defer zombie recovery until after server recovery completes.
2. **Add a recovery-in-flight gate at the Instance level.** A per-instance `recoveringMu sync.Mutex` with a `tryLock` (via `TryLock` available since Go 1.18) means only one goroutine proceeds with recovery per session. All others bail early.
3. **Extend the zombie detection debounce.** A session must fail `TmuxAlive()` for at least 2–3 consecutive health check cycles (e.g., 2 seconds) before being treated as a zombie. This absorbs the server restart window.

---

### 4. Exit Code Propagation

**Trigger conditions**

tmux is a terminal multiplexer, not a process supervisor. The child process exit code is consumed by the tmux server and is not forwarded through the PTY. From the perspective of the `attach-session` client (the `ptmx` fd), the session ending looks like: the PTY master receives EOF when the last slave fd closes. There is no out-of-band signal carrying the exit code.

**What tmux does internally.**
tmux stores the exit code of the last foreground process in pane metadata. It is accessible via `tmux display-message -p "#{pane_dead_status}"` or the control mode notification `%pane-died <target> <status>` (tmux ≥ 3.3). However, this data is only available _while the pane still exists_. If the session is configured to close on program exit (`remain-on-exit off`, which is the tmux default), the pane is destroyed before any poller can read the exit code.

**How other tools work around this:**

- **tmux control mode (`-C`):** Subscribe to `%pane-died` notifications. This provides exit status synchronously as part of the tmux protocol stream. The project already has `StartControlMode()` which processes control mode output (`control_mode.go`). Wiring `%pane-died` parsing into the control mode reader is the cleanest path.
- **`remain-on-exit on`:** Configure the pane to stay visible after exit. Then poll `#{pane_dead_status}` before deciding how to handle the session. Cost: pane lingers; must be cleaned up explicitly.
- **Shell wrapper:** Launch the program inside a wrapper shell that echoes the exit code to a file in the worktree before exiting (e.g., `sh -c 'claude; echo $? > .exit_code'`). Fragile but requires no tmux version dependency.
- **Process supervisor integration (systemd/supervisord):** Run the child as a supervised service. Not applicable here since the goal is tmux isolation.

**Current state in stapler-squad.** The project does not capture exit codes. `streamLoop` treats EOF and "file already closed" equivalently — both transition the session to "program exited" without distinguishing exit 0 from exit 1 from a signal kill. This means:
- A claude session that exits cleanly (finished task) looks identical to one that crashed.
- Auto-restart logic in the health checker cannot distinguish "needs restart" (crash) from "should stay stopped" (user quit).

**Mitigation strategies**

1. **Parse `%pane-died` in control mode.** Add a handler in `readControlModeOutput()` for the `%pane-died <target> <status>` notification. Wire the status into the session state machine as a new `ExitCode int` field on `Instance`. Low overhead, uses an existing code path, requires tmux ≥ 3.3.
2. **`remain-on-exit on` + deferred poll.** Set `remain-on-exit on` for all sessions at creation time. After detecting EOF, poll `tmux display-message -p "#{pane_dead_status}"` once, then `tmux kill-pane`. Increases latency between exit and cleanup.
3. **Shell wrapper as fallback.** Emit exit code to `.stapler_squad_exit_code` in the working directory. `streamLoop` can read this file on EOF. Works with any tmux version.

---

### 5. Double-Start Race Condition

**Trigger conditions**

`Instance.start()` (line 780, `instance.go`) does not hold `stateMutex` for the entire duration of the start operation. The critical window:

```
start() called (no lock held)
  → initTmuxSession()              // idempotent
  → setupFirstTimeWorktree()       // may block (git ops)
  → tmuxManager.Start(startPath)   // blocks on tmux new-session + poll
  → stateMutex.Lock()
  → i.started = true               // set after all async work
  → stateMutex.Unlock()
```

If `Start(false)` is called from two goroutines simultaneously (e.g., health checker + server restore path), both goroutines can pass the `if i.started` guard at `instance.go:648` (checked in `Started()`) before either sets `i.started = true`. Both proceed to call `tmuxManager.RestoreWithWorkDir()`, which calls `DoesSessionExist()` → `has-session`. If the tmux session was recently killed, both find it absent and race to recreate it. The second `tmux new-session` call hits the "session already exists" path in `tmux.go:start()` (line 444) and returns nil — but both goroutines then call `RestoreWithWorkDir()` independently, each creating a `ptmx` fd pointing to the same tmux session. This produces two `streamLoop` goroutines reading the same PTY.

**Observed symptoms from logs:**
- Double "Successfully restored PTY connection" for the same session name.
- Two `ResponseStream.streamLoop` goroutines racing to broadcast the same bytes to subscribers.
- Subscriber channels receiving duplicate output chunks.
- When one `streamLoop` calls `closeAllSubscribers()` on EOF, the other finds an empty map and silently exits — masking the event.

**Root cause in code.** `i.started` (line 862) is set _after_ the tmux start and PTY restore complete. There is no "starting in progress" flag or mutex that prevents a second concurrent `Start()` call from entering the same code path. The `stateMutex` protects the `Status` field transition but not the broader start sequence.

**Mitigation strategies**

1. **Add a `startMu sync.Mutex` per instance.** `start()` calls `startMu.Lock()` immediately. This serialises all start calls. Combined with a re-check of `i.started` inside the lock (double-checked locking), concurrent starts become sequential no-ops after the first completes.
2. **Use an atomic `startInProgress` flag.** `sync/atomic` `CompareAndSwap` on a `uint32` before the start path. The second goroutine that loses the CAS returns immediately. This is lighter than a mutex if high-frequency contention is not expected.
3. **Move `i.started = true` before the tmux operations.** Set `started = true` (under `stateMutex`) at the top of the start path, then revert it on error. This is the "optimistic" pattern used in HTTP servers and database pools. Risk: a brief window where `Started()` returns `true` but the session is not yet usable — acceptable if callers handle `TmuxAlive() == false`.

---

### 6. Other Known Pitfalls

#### 6a. streamLoop → session state machine disconnect

**Problem.** When `streamLoop` detects EOF (program exited), it calls `closeAllSubscribers()` and returns. There is no callback or channel that notifies the `Instance` state machine. `Instance.Status` remains `Running` or `Ready`; `i.started` remains `true`. The health checker catches this on the next polling cycle (via `TmuxAlive()`), but the window between EOF and the next health check is several seconds. During this window:
- The web UI shows the session as active.
- Subscribers are closed (no data flowing) but the session object looks healthy.
- New subscribers can be registered to the stopped `ResponseStream` — they receive nothing and are never cleaned up until the next `Stop()` call.

**Mitigation.** Pass an `onEOF func()` callback into `NewResponseStream`. The `start()` path wires this callback to `instance.transitionTo(Stopped)`. This makes the state machine transition synchronous with the PTY event rather than relying on periodic health checking.

#### 6b. `response_stream.go` started flag not reset on Stop

**Problem.** `ResponseStream.Stop()` sets `rs.started = false`. However, `streamLoop` can observe EOF and call `closeAllSubscribers()` without going through `Stop()`. In that code path, `rs.started` remains `true`. A subsequent call to `Start()` returns `fmt.Errorf("response stream already started")`. The only recovery is to create a new `ResponseStream`, which requires recreating `PTYAccess`.

**Mitigation.** `streamLoop` should set `rs.started = false` (under `rs.mu`) before returning, whether it exits via context cancellation or via EOF.

#### 6c. Control mode `controlModeExited` race

**Problem.** In `control_mode.go`, `controlModeExited` is set to `true` inside `readControlModeOutput()` after the goroutine exits. New subscribers that call `SubscribeControlMode()` after exit check this flag and receive a pre-closed channel. However, `StopControlMode()` closes `controlModeDone` and then proceeds to close `controlModeStdin`. If `readControlModeOutput()` is still running when `controlModeDone` is closed, it may attempt to write to already-nil fields. The `controlModeSubMu` protects the subscriber map but not the infrastructure fields (`controlModeCmd`, `controlModeStdout`).

**Mitigation.** Set `controlModeCmd = nil` only after `cmd.Wait()` completes, and guard all field accesses in `StopControlMode` with a lock.

#### 6d. `TmuxSession.start()` success path for existing session

**Problem.** If the tmux session already exists (line 444: `"Session already exists, reusing"`), `start()` returns `nil` _without_ setting up a PTY or monitor. It is the caller's responsibility to call `RestoreWithWorkDir()` separately. If a caller forgets (or assumes `Start()` always leaves the session in a fully usable state), `GetPTY()` returns an error, and `StartController()` silently skips with a log warning. The session appears started but the controller is never registered.

**Mitigation.** Make `start()` self-complete: always call `RestoreWithWorkDir()` before returning when reusing an existing session.

#### 6e. `tmux kill-session` vs. `KillSession` vs. `Destroy`

**Problem.** The codebase has three paths for terminating a session: `tmuxSession.Close()` (kills pane, closes fd), `Instance.KillSession()` (kills the tmux session without cleanup), and `Instance.Destroy()` (full cleanup including worktree). These are not always called in the right order during recovery paths, leading to orphaned `attachCmd` processes, unclosed fds, and worktrees that are neither preserved nor deleted.

**Mitigation.** Centralise teardown in a single `Instance.terminate(mode TerminateMode)` function that enforces the order: cancel context → kill attachCmd → close ptmx → (optionally) cleanup worktree.

---

## Trade-off Matrix

| Mitigation | Complexity | Correctness Gain | Risk |
|---|---|---|---|
| Per-instance `startMu` | Low | High — eliminates double-start | None |
| `ReadWithDeadline` on PTYAccess | Medium | Medium — eliminates stale-ptr race | Requires API change |
| `onEOF` callback to state machine | Low | High — synchronises exit detection | Must be nil-safe |
| `streamLoop` resets `started = false` | Trivial | Medium — fixes restart after EOF | None |
| Parse `%pane-died` in control mode | Medium | High — exit code propagation | tmux ≥ 3.3 required |
| Zombie debounce (2-cycle minimum) | Low | High — eliminates false positives during server restart | Adds 2s detection latency |
| Kill `attachCmd` before `ptmx.Close()` | Trivial | High — guarantees goroutine exit | None |

---

## Risk and Failure Modes

| Failure Mode | Severity | Frequency | Detection | Current Mitigation |
|---|---|---|---|---|
| PTY close race → stale fd syscall | Medium | Rare | Log "bad file descriptor" | String-match error handling in streamLoop |
| Goroutine leak on orphaned attachCmd | High | On every crash | pprof goroutine dump | None (gap) |
| Zombie false positive during server restart | High | Every server restart | Log "Instance marked as started but tmux doesn't exist" | recoveryMu serialises server recovery, but not session health checks |
| No exit code propagation | Medium | Every exit | None | Not implemented |
| Double-start race | High | Under concurrent load | Log "Successfully restored PTY connection" twice | None (gap) |
| streamLoop EOF does not update state machine | High | Every unclean exit | Stale Running status in UI | Health checker catches it within polling interval |
| streamLoop started flag not reset | Low | After EOF-triggered exit | Start() returns error | None (gap) |

---

## Migration and Adoption Cost

All mitigations listed are surgical — they target specific code paths without requiring architectural changes:

- **`startMu` addition:** One new field on `Instance`, two lines in `start()`. No interface changes.
- **`onEOF` callback:** One new parameter to `NewResponseStream`, one call site in `start()`. Fully backward-compatible.
- **`ReadWithDeadline`:** Requires changing `streamLoop` to call a new `PTYAccess` method. The existing `PTYAccess.Read()` method remains for other callers.
- **`%pane-died` parsing:** Additive to `control_mode.go`. No existing handlers change.
- **Zombie debounce:** Requires a per-instance counter in `SessionHealthChecker`. No storage changes needed (counter is in-memory, reset on each health check cycle).

None of these changes require database migrations, proto changes, or UI updates.

---

## Operational Concerns

1. **tmux version dependency.** `%pane-died` requires tmux ≥ 3.3 (released 2022). Most macOS and Linux systems with Homebrew or recent apt have this, but it should be checked at startup with `tmux -V` and the exit code path should gracefully degrade to "unknown" if the version is older.

2. **Health checker interval tuning.** The current `ScheduledHealthCheck` interval is not visible in the code reviewed; it is passed as a parameter. The zombie debounce recommendation (2 consecutive failures) is coupled to whatever interval is used. Document the interval in configuration so operators know the true detection latency.

3. **`pprof` integration.** The existing `--profile` flag enables pprof. Goroutine leak detection (failure mode 2) should be part of a standing operational practice: run `curl /debug/pprof/goroutine` after a stress test or after a tmux server restart to verify no `streamLoop` goroutines outlive their session.

4. **Log volume.** The zombie detection path currently logs at `WarningLog`. During a server restart, this will emit N warnings for N sessions simultaneously. Consider rate-limiting with the existing `log.NewEvery` pattern.

---

## Prior Art and Lessons Learned

**`tmuxinator` / `tmux-resurrect`:** These tools use `tmux list-sessions` + `tmux list-panes` for state persistence. They do not attempt programmatic exit detection — they are user-driven. Not applicable here.

**`libtmux` (Python):** The `tmux wait-for` command (tmux ≥ 2.2) can block until a named event fires, including `%pane-died`. This is the Python ecosystem's standard answer to exit detection. In Go, the control mode approach is more idiomatic.

**`gotmux` / `tmuxgo`:** Thin wrappers around tmux commands. Do not address PTY lifecycle management.

**`kterm` / `ttyd`:** Web-based terminal emulators that pipe PTYs over WebSocket. Both use the "kill the process, then read EOF" pattern — the kill is always the trigger for clean shutdown, never the read error. Lesson: **initiate shutdown from the write side (process kill), not from the read side (EOF detection).** This maps directly to the "kill `attachCmd` before close" mitigation.

**Go standard library `os/exec`:** `cmd.Wait()` blocks until the process exits and its stdout/stderr pipes are closed. The `session/tmux/pty.go` already has a reap goroutine (`go func() { _ = cmd.Wait() }()`) to prevent zombie processes. Extending this to notify the session state machine on exit is the natural next step.

**`creack/pty` package issues:** Known issue #48 (2019): on macOS, closing the master fd does not immediately unblock a goroutine blocked on `Read` of the same fd in some Go versions. The recommended workaround is to kill the child process first, which causes the slave to close, which causes the master read to return EOF. This reinforces the "kill `attachCmd` first" recommendation.

---

## Open Questions

1. **What tmux version is the minimum required by stapler-squad?** The answer determines whether `%pane-died` parsing is viable without a version gate.

2. **Is `ScheduledHealthCheck` always running, or only on demand?** The health check interval determines the true zombie detection latency and affects the debounce recommendation.

3. **Does `STAPLER_SQUAD_USE_CONTROL_MODE=false` mean control mode is optional?** If control mode can be disabled, exit code propagation via `%pane-died` needs a fallback path.

4. **Are there scenarios where `Instance.Start()` is called from multiple goroutines intentionally?** The double-start race might be by design in some recovery codepath. The `startMu` approach would serialize them safely, but understanding the intent affects the choice between mutex and CAS.

5. **What is the expected behaviour when the tmux server restarts?** Should all sessions auto-recover? Should the user be notified? This determines whether the zombie debounce is the right answer or whether a deeper "server restart event" broadcast mechanism is needed.

---

## Recommendation

Top 3 mitigations to implement first, ordered by impact/effort ratio:

**1. Add per-instance `startMu sync.Mutex` to serialize `start()` (double-start race — failure mode 5)**

Impact: eliminates the observed "double PTY connection" log pattern and prevents duplicate `streamLoop` goroutines. Effort: ~10 lines. No interface changes. Can be done independently of any other change.

**2. Wire an `onEOF` callback from `streamLoop` to `instance.transitionTo(Stopped)` (state machine disconnect — failure mode 6a)**

Impact: closes the gap between PTY EOF and session status. The UI immediately reflects the exit; health checker no longer needs to be the primary exit signal path. Effort: ~20 lines across `response_stream.go` and `instance.go`. Makes the system event-driven rather than poll-driven for the common exit path.

**3. Always kill `attachCmd` before closing `ptmx` in all teardown paths (goroutine leak — failure mode 2)**

Impact: guarantees `streamLoop` exits within one read deadline (100 ms) of any teardown call. Eliminates the leak class entirely. Effort: audit 3–4 call sites in `tmux.go` and add a `p.attachCmd.Process.Kill()` before `p.ptmx.Close()`. The `CRITICAL` comment in `tmux.go:54` acknowledges the gap; this closes it.

---

## Pending Web Searches

The following searches could strengthen or update this research. Web search was not available during this session.

1. `creack/pty issue 48 macOS close master unblock read` — verify the macOS-specific fd-close-does-not-unblock-read behaviour and whether it has been fixed in recent pty versions.
2. `tmux control mode %pane-died minimum version` — confirm which tmux release introduced `%pane-died`.
3. `golang os.File close unblocks blocked Read goroutine` — confirm Go runtime behaviour on fd close while another goroutine is blocked in Read (particularly for the non-network-poller codepath on macOS).
4. `tmux wait-for golang exit detection` — survey any Go libraries that have solved the tmux exit detection problem already.
5. `golang sync.Mutex TryLock availability version` — confirm `sync.Mutex.TryLock()` was added in Go 1.18 for the CAS-style per-instance start guard.
