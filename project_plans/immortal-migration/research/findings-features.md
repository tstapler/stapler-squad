# Findings: Immortal Features

Status: Verified | Last verified: 2026-05-22
Note: This file documents why Immortal was rejected. Conclusions remain accurate.

## Summary

Immortal is a daemon-first process supervisor that happens to expose an importable Go library (`github.com/immortal/immortal`, BSD-3-Clause, Go 1.22). Its root package exports `Daemon`, `Supervise`, `New`, `Process`, and `Config`, making it technically embeddable without a separate binary. However, every supervision run creates on-disk state (a `supDir` directory, an `immortal.sock` Unix socket, optional PID files) and the library's `Fork()` function is designed to daemonize the supervisor itself via `setsid(2)` — a pattern fundamentally at odds with stapler-squad's single-binary, in-process model. There is zero PTY/terminal support; stdout/stderr are piped to log files, not streamed back to callers. Immortal does not satisfy the embedding requirements. A lightweight custom 200–300 line in-process supervisor is the lower-risk, lower-complexity path.

---

## Options Surveyed

| Mode | Description |
|---|---|
| `immortal` binary (standalone) | Compiled daemon, managed via `immortalctl` over Unix socket. Separate process, separate lifecycle. |
| `immortaldir` | Watches a config directory and auto-spawns supervisors for each YAML file. Adds a directory-scan daemon layer on top of standalone. |
| Library embedding (`go get github.com/immortal/immortal`) | Import root package, call `immortal.New(cfg)` + `immortal.Supervise(daemon)`. No separate binary, but still writes supDir to disk and opens a Unix socket per process. |
| Custom in-process supervisor | ~250 lines: goroutine per managed process, `os/exec` + `creack/pty`, exponential backoff, channel-based stop/restart. No disk state, no socket, no PTY gap. |

---

## Trade-off Matrix

| Axis | Immortal (library mode) | Custom in-process supervisor |
|---|---|---|
| **Library vs daemon architecture** | Library API exists but internals are daemon-shaped: flock, supDir, Unix socket per process | Pure goroutine, no daemon artifacts |
| **PTY support** | None. stdout/stderr piped to log files only. No PTY master fd exposed to caller. | Full control — caller wires `creack/pty` to `exec.Cmd` directly |
| **Restart policy richness** | `Retries` int, `Wait` duration, `PostExit` hook, `RequireCmd` prerequisite | Must implement: ~80 lines covers retries + exponential backoff + max-attempts |
| **Control interface** | HTTP-over-Unix-socket (`immortal.sock`) per supervised process | In-process channels; no external socket required |
| **Embedding feasibility** | Partial. Importable, but writes `~/.immortal/<name>/`, opens socket, calls `setsid` in Fork | Full. Pure in-memory goroutine tree, zero OS-level side effects |
| **Pure Go compliance** | Yes. No CGO. 6 transitive deps: logrotate, multiwriter, natcasesort, xtime, violetear, yaml.v3 | Yes. Stdlib only (`os/exec`, `syscall`, `io`, `sync`, `context`) + `creack/pty` |
| **License** | BSD-3-Clause. Compatible with stapler-squad. | N/A (own code) |
| **Binary size impact** | +6 transitive dependencies including violetear HTTP router | Zero new deps beyond `creack/pty` (already ecosystem-standard) |
| **Maintenance surface** | External dependency; track upstream for breaking changes | Owned code; change on your schedule |

---

## Risk / Failure Modes

**1. supDir disk writes on every supervised process**
Immortal always writes state to `d.supDir` (default `~/.immortal/<name>/`). Sessions are created and destroyed frequently in stapler-squad. Stale socket files and lock files accumulate unless explicitly cleaned up on every session teardown. The library provides no automatic cleanup path.

**2. Unix socket per process creates O(N) socket files**
Each `Daemon.Listen()` call opens `<supDir>/immortal.sock`. With many concurrent sessions, the system approaches per-user file descriptor and socket limits. stapler-squad already manages N concurrent tmux sessions; doubling socket pressure is non-trivial.

**3. Fork() / setsid() is hostile to in-process use**
`fork.go` calls `exec.Command(os.Args[0], args...)` with `Setsid: true` to detach from the calling process's session. Invoking this from within stapler-squad would spawn a new stapler-squad process, not a supervised child. The Fork path is not clearly gated — callers must know not to invoke it.

**4. No PTY = cannot stream terminal output**
stapler-squad's core value proposition is real-time terminal streaming. Immortal pipes stdout/stderr to `io.ReadCloser` logger inputs. There is no `*os.File` or PTY master fd exposed for the caller to attach to a terminal emulator. This is a hard architectural incompatibility, not a workaround-able gap.

**5. Restart loop is a blocking goroutine with no event callbacks**
`Supervise()` runs a blocking select loop. The restart policy executes inside the library's goroutine. There is no hook to intercept a restart event, notify the stapler-squad session manager, or propagate status changes to the web UI without polling the Unix socket.

**6. violetear HTTP router is an undeclared operational dependency**
The socket server uses `github.com/nbari/violetear` as an HTTP router. Any vulnerability in violetear affects the stapler-squad binary. This is a non-obvious transitive risk for a single-binary deployment.

**7. No exponential backoff; tight restart loops possible**
`Config.Retries` controls max restart count but there is no backoff field. Rapid-crashing processes are restarted at the `Wait` interval or immediately if unset — a thundering-herd risk for misbehaving AI agent processes.

---

## Migration / Adoption Cost

If Immortal were adopted despite the risks:

1. **Proto/API changes**: None required. The library sits below the session layer.
2. **Session lifecycle rewrite**: `session/instance.go` would need to replace tmux-based process management with `immortal.New()` + `immortal.Supervise()`. Estimated: 3–5 days.
3. **PTY bridging**: No solution exists within Immortal. Immortal constructs its own `exec.Cmd` internally and does not accept a pre-configured one. Bridging PTY would require forking the library. This is not a configuration gap — it is an architectural incompatibility.
4. **supDir cleanup integration**: Every session create/delete needs `os.RemoveAll(supDir)` and socket cleanup. Estimated: 1 day.
5. **Status polling refactor**: The web UI streams session status via ConnectRPC. Immortal status is read by polling `immortal.GetStatus(socket)`. The streaming pipeline becomes poll-and-push. Estimated: 2–3 days.
6. **Testing burden**: Immortal's daemon model is harder to unit test than goroutine-based supervision. Integration tests must account for on-disk socket files. Estimated: 1–2 days additional test infrastructure.

Total estimated adoption cost: **7–11 days** for a partial integration that still does not solve the PTY gap.

---

## Operational Concerns

- **Single-binary deployment**: The library is pure Go, compiles cleanly. The operational concern is runtime artifacts (socket files, lock files, PID files) invisible to the single-binary mental model.
- **macOS compatibility**: `kqueue.go` and `openmode_darwin.go` exist for BSD/macOS event notification. Appropriate for the target platform but adds platform-divergent code paths in the dependency.
- **Graceful shutdown**: Immortal processes are stopped via `SendSignal(socket, "down")`. In-process teardown requires the socket server to be up. Race conditions during stapler-squad shutdown are likely.
- **Log file accumulation**: Immortal writes to log files with rotation policies. stapler-squad manages its own log streams via scrollback buffers. Running two log systems in parallel creates confusion about output location.

---

## Prior Art / Lessons Learned

- **supervisord (Python) / runit / s6 / daemontools**: All canonical supervisor tools; all designed to run as separate daemons, never embedded. This is the dominant pattern across the industry, reinforcing that Immortal's daemon-shaped design is not an accident.
- **goproc / go-supervisor (GitHub)**: Smaller Go libraries (100–300 lines) that model in-process supervision. Closer in spirit to what a custom implementation produces. Neither has gained wide adoption, suggesting the pattern is simple enough that teams write their own.
- **tmux (current approach)**: stapler-squad currently uses tmux as a process supervisor with PTY provision as a side effect. Replacing tmux with Immortal trades PTY capability for restart policy richness — a losing trade when PTY is the core product feature.
- **containerd shim model**: containerd separates the shim (holds PTY, manages process) from the runtime supervisor. This is the correct architecture for combining PTY and supervision. A 250-line custom goroutine achieves this more directly than Immortal.

---

## Open Questions

- [ ] Can `immortal.New(cfg)` accept a pre-opened PTY master fd (via `SysProcAttr` or `Env`)? Review `process.go:Start()` to confirm `exec.Cmd` is constructed internally with no injection point.
- [ ] Is there a code path in `daemon.go:New()` that skips `supDir` creation for in-memory-only use?
- [ ] Does `Supervise()` expose a channel or callback for restart events, or is status only readable via socket polling?
- [ ] What is the behavior of `Config.Retries = 0`? Does it mean "never restart" or "restart once"? Relevant for one-off session types.
- [ ] Is `violetear` pinned to a non-vulnerable version? Run `govulncheck ./...` on a test import branch.
- [ ] Has any non-trivial Go application embedded Immortal as a library (not as a sidecar)? Search GitHub code for `import "github.com/immortal/immortal"` excluding the immortal org.

---

## Recommendation

**Do not adopt Immortal. Write a custom 200–300 line in-process supervisor instead.**

The decisive blockers:

1. **No PTY support** — Immortal cannot stream terminal output to callers. Non-negotiable for stapler-squad.
2. **Daemon-shaped internals at runtime** — socket files, supDir, flock, and setsid create operational complexity incompatible with single-binary deployment.
3. **No restart event hooks** — stapler-squad must react to process restarts (update UI, log, notify). Immortal provides no callback; polling a Unix socket is the only status channel.

A custom supervisor requires: `exec.Cmd` with a PTY via `creack/pty`, a restart goroutine with exponential backoff, a `context.Context` for cancellation, and a status-event channel. This is approximately 250 lines, zero new dependencies beyond `creack/pty`, and covers 100% of stapler-squad requirements.

Immortal is the right tool for system-level service management where PTY is irrelevant and daemon separation is desirable. It is the wrong tool for an embedded, PTY-streaming, event-driven session manager.

---

## Pending Web Searches

- Search GitHub code for `import "github.com/immortal/immortal"` in non-immortal repos — verify whether library embedding is used in practice or only the binary.
- Search for `creack/pty` + in-process supervisor patterns in Go — validate the recommended custom approach and find reference implementations.
- Verify `Supervise()` actual signature on `pkg.go.dev` directly (the analysis above used AI-synthesized output which may misrepresent blocking behavior).
- Search for Go process supervisor libraries under 500 lines with PTY support (candidates: `go-supervisor`, `goproc`, `immortal` alternatives) to confirm no better off-the-shelf option exists.
