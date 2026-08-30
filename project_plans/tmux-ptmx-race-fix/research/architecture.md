# Research: architecture / call chains for the ptmx race

## 1. CreateSession → GetPTY call chain

`server/services/session_service.go:1574` spawns an unguarded `go func() { ... }()` inside
`CreateSession` (function starts at `session_service.go:1251`) immediately after the RPC has
already saved the instance to storage and published `SessionCreated`. The goroutine is fully
detached — nothing tracks its lifetime (no `WaitGroup`, no context passed in, no cancellation
channel), and `CreateSession` returns its HTTP response to the caller before the goroutine even
starts running.

Inside that goroutine, the path to `GetPTY` is:

```
CreateSession (RPC handler, returns immediately)
  └─ go func() { ... }                                   session_service.go:1574
       ├─ instance.Start(true)                            session_service.go:1582
       ├─ instance.SetStatusManager(...)                  session_service.go:1613
       └─ instance.StartController()                      session_service.go:1614
            └─ Instance.StartController()                 instance_controller.go:19
                 ├─ i.mu.Lock() / preconditions / i.mu.Unlock()   (released BEFORE controller creation,
                 │                                                explicitly to avoid deadlocking with
                 │                                                Start()'s own GetPTYReader() call —
                 │                                                see comment at instance_controller.go:45)
                 └─ NewClaudeController(i)                 instance_controller.go:49
                      └─ (controller construction reads the PTY via i.GetPTYReader(), typically
                          to wire up the response-stream reader)
                           └─ Instance.GetPTYReader()       instance_tmux.go:439
                                └─ i.pm().GetPTY()
                                     └─ TmuxBackend.GetPTY()        tmux_backend.go:53
                                          └─ TmuxProcessManager.GetPTY()  tmux_process_manager.go:238
                                               └─ TmuxSession.GetPTY()    tmux.go:1332
                                                    └─ reads t.ptmx (tmux.go:1333-1336) — UNGUARDED
```

**No synchronization exists between this goroutine and `DeleteSession`'s cleanup goroutine.**
Specifically:
- `Instance.mu` (the RWMutex guarding `controllerManager`/status fields) is explicitly released
  before `NewClaudeController` runs (`instance_controller.go:44-46`, comment: "Release lock
  before creating/starting controller ... prevents deadlock when Start() calls GetPTYReader()
  which acquires read lock"). So by the time `GetPTYReader()` is called, no Instance-level lock
  is held that could block a concurrent `Destroy()`.
- There is no `WaitGroup`/context tracking of the `CreateSession` async goroutine anywhere in
  `SessionService` — `DeleteSession` has no way to know "wait, CreateSession's setup goroutine
  for this instance is still running" before it starts tearing the instance down.
- `Instance.started` (an `atomic.Bool`, set inside `Start()`) is the only cross-goroutine signal,
  and it only gates whether `StartController`/`GetPTYReader` *attempt* the call at all — it does
  not serialize against a concurrent `Destroy()` that runs after `started` flips true.

This confirms the requirements doc's framing: the two goroutines are a genuine, structural gap,
not a hypothetical race — `CreateSession` truly returns before its async setup completes, and
`DeleteSession` has no way to wait for or cancel that setup before tearing the instance down.

## 2. DeleteSession → closePTYAndAttachCmd call chain

`server/services/session_service.go:2008` (`DeleteSession`). After removing the session from
storage/pollers, it spawns its own unguarded goroutine (`session_service.go:2062`) calling
`liveInst.Destroy()`:

```
DeleteSession (RPC handler)
  └─ go func() { liveInst.Destroy() }()                   session_service.go:2062
       └─ Instance.Destroy()                               instance.go:1257
            ├─ i.StopController()                          instance.go:1266 → instance_controller.go:148
            │    (i.mu.Lock/Unlock — only unregisters the controller reference from
            │     controllerManager; does NOT wait for any in-flight goroutine that is mid-call
            │     inside StartController/NewClaudeController/GetPTYReader to finish)
            ├─ i.stopVNC() / i.stopCDP()
            └─ i.KillSession()                              instance.go:1277 → instance_tmux.go:282
                 └─ i.pm().Close()  (if pm().HasSession())
                      └─ TmuxBackend.Close()                 tmux_backend.go:47
                           └─ TmuxProcessManager.Close()      tmux_process_manager.go:102
                                └─ s := tm.session.Load(); s.Close()
                                     └─ TmuxSession.Close()   tmux.go:1724
                                          └─ t.closePTYAndAttachCmd()  tmux.go:1685
                                               ├─ t.ptmx.Close(); t.ptmx = nil       tmux.go:1689-1696 — UNGUARDED WRITE
                                               └─ t.attachCmd.Process.Kill(); t.attachCmd.Wait(); t.attachCmd = nil;
                                                  t.attachCmdWaitOnce = nil          tmux.go:1701-1717 — UNGUARDED WRITE
```

`StopController()` (`instance_controller.go:148`) is the only thing `Destroy()` does that even
brushes against controller/PTY wiring, and it is a pure "remove the reference" operation under
`i.mu` — it has no knowledge of, and does not block on, a concurrent `CreateSession` goroutine
that is currently *inside* `NewClaudeController`/`GetPTYReader`. So `Destroy()` can proceed
straight through to `closePTYAndAttachCmd()`'s unguarded `t.ptmx = nil` while the create-side
goroutine is concurrently inside `GetPTY()`'s unguarded `return t.ptmx, nil` — exactly the
confirmed race.

## 3. Do TmuxProcessManager / TmuxBackend / Instance already have locks a fix should compose with?

- **`TmuxBackend`** (`session/tmux_backend.go`): pure forwarding shim, zero fields besides `mgr
  ProcessManager`, zero locks. Every method is a one-line delegate (`GetPTY()`, `Close()`, etc.).
  Nothing to compose with here.
- **`TmuxProcessManager`** (`session/tmux_process_manager.go`): holds `session
  atomic.Pointer[tmux.TmuxSession]` (confirmed via `tm.session.Load()` at `Close()`
  line 103 and `GetPTY()` line ~239). This atomic pointer protects *reassignment of the
  `*TmuxSession` pointer itself* (e.g. when a new session object replaces an old one across
  Start/Restore) — it says nothing about the mutable fields (`ptmx`, `attachCmd`,
  `attachCmdWaitOnce`) living *inside* the `*TmuxSession` object once loaded. A fix scoped
  inside `TmuxSession` is orthogonal to this atomic pointer and does not need to interact with
  it — `tm.session.Load()` returns a stable `*TmuxSession`, and both `GetPTY()` and `Close()`
  immediately call straight through to methods on that same instance.
- **`Instance`** (`session/instance.go`, `instance_controller.go`, `instance_tmux.go`): has its
  own `i.mu` (RWMutex, guards `controllerManager`, `Status`, and other Instance-level fields) and
  an actor-style command queue (`sendSyncErr`/`instanceState`, seen in `pauseLocked`). Crucially,
  `StartController()` *deliberately drops* `i.mu` before touching the PTY
  (`instance_controller.go:44-46`), specifically to avoid a self-deadlock with `Start()`'s own
  `GetPTYReader()` call. That means `i.mu` cannot be reused/extended to guard `t.ptmx` — doing so
  would either reintroduce that exact deadlock or require restructuring lock-drop points that are
  explicitly a non-goal (requirements.md's "no change to ... locks already in the file").

**Conclusion: none of these outer layers provide (or can safely be extended to provide) the
synchronization this bug needs.** The fix belongs entirely inside `TmuxSession` — a new
`TmuxSession`-local mutex (matching the existing `deadlock.Mutex` style already used for
`detachMutex`, `cmdSendMu`, and the package-level `recoveryMu` in `tmux.go`) guarding exactly
`t.ptmx` / `t.attachCmd` / `t.attachCmdWaitOnce`, composes cleanly underneath all three outer
layers without requiring any changes to them.

## 4. Other lifecycle methods that could plausibly race the same way

Per requirements.md's scope (ptmx + attachCmd + attachCmdWaitOnce only, all reassigned in
lockstep), every method that reads OR writes any of these three fields needs the same guard.
Confirmed call sites in `session/tmux/tmux.go`:

| Method | Line(s) | Access |
|---|---|---|
| `AttachToExisting` | 859, 864-866 | read (`t.ptmx == nil` check) + write (all 3 fields) |
| `RestoreWithWorkDir` retry loop | 1240-1268 | read + write (all 3 fields; also calls `closePTYAndAttachCmd()` internally at 1241) |
| `closePTYAndAttachCmd` | 1689-1717 | write (all 3 fields, nils them) — called from `Close()` and `RestoreWithWorkDir` |
| `GetPTY` | 1332-1336 | read (`t.ptmx`) |
| `TapEnter` | 1309 | read (`t.ptmx.Write`) |
| `TapDAndEnter` | 1318 | read (`t.ptmx.Write`) |
| `SendKeys` | 1326 | read (`t.ptmx.Write`) |
| `Attach` (goroutine 1) | 1484 | read (`io.Copy(os.Stdout, t.ptmx)` — captured once, used for the goroutine's lifetime) |
| `Attach` (goroutine 2 / stdin forwarder) | 1544 | read (`t.ptmx.Write`) |
| `updateWindowSize` (used by `SetDetachedSize` and the resize path) | 1781, 1786, 1800 | read (`t.ptmx == nil` check, `t.ptmx.Fd()`, passed to `pty.Setsize`) |

All 10 sites named in requirements.md are accounted for; no additional unguarded site for these
three fields was found elsewhere in `tmux.go`. `Attach`'s two goroutines are the trickiest: they
capture `t.ptmx` implicitly via closure over `t` and read it repeatedly across the goroutine's
lifetime (not just once at spawn), so the guard needs to protect each individual read
(`io.Copy`/`Write` call), not just a single snapshot at goroutine-start — a snapshot pattern
would still race if `closePTYAndAttachCmd` reassigns `t.ptmx` mid-`io.Copy`. This is a
correctness note for the *how* of the fix (an implementation detail for phase 3/plan), not a new
scope item — `Attach` is already the mechanism this requirement's non-goal ("no behavior change
to PTY lifecycle semantics") needs preserved as-is.

## 5. Is an internal-only `TmuxSession` lock sufficient, or does any external caller need atomic read-then-use?

Confirmed: **all external access to `ptmx` is funneled through `TmuxSession` methods.** No caller
outside `tmux.go` ever reads `t.ptmx` directly — `Instance.GetPTYReader()` (`instance_tmux.go:439`)
only calls `i.pm().GetPTY()`, which chains through `TmuxBackend.GetPTY()` →
`TmuxProcessManager.GetPTY()` → `TmuxSession.GetPTY()`, and the only thing that comes back to the
caller is the returned `*os.File` value (a snapshot pointer), never a reference to the struct
field itself. Same for `SendKeys`/`TapEnter`/`TapDAndEnter` — callers invoke the method and never
touch `t.ptmx` directly.

**Caveat (out of scope, but worth naming for the plan phase):** once `GetPTY()` returns a
`*os.File` snapshot to a caller (e.g. `Instance.GetPTYReader()` → whatever wires up the response
stream reader in `NewClaudeController`), the caller then uses that returned file handle
independently, outside any `TmuxSession` lock, for potentially the life of the response stream.
If `closePTYAndAttachCmd()` closes and nils `t.ptmx` concurrently, the returned `*os.File` is
still a valid Go value (the field write doesn't affect the caller's already-copied pointer), but
calling `.Read()`/`.Write()` on it will return an OS-level "file already closed" error — this is
an existing, already-handled condition (see `closePTYAndAttachCmd`'s explicit "file already
closed" string check at tmux.go:1692) and is explicitly the PTY-EIO/lifecycle behavior
requirements.md's non-goals section says NOT to change. So: an internal-only mutex on
`TmuxSession` (guarding the field read/write, i.e. eliminating the `-race`-flagged unsynchronized
memory access) is sufficient to satisfy this bug's scope. It does not, and per non-goals should
not, attempt to serialize a caller's post-`GetPTY()` usage of the returned file handle against a
concurrent close — that TOCTOU-after-return is a pre-existing, already-tolerated behavior, not
part of this fix.

## Related-but-different: `ci-hookurl-race-flake` context

Read `project_plans/ci-hookurl-race-flake/research/architecture.md` for the prior investigation.
That work diagnosed a CI-timeout/contention issue in the async hook-injection pipeline
(`InjectHookConfig`) and tmux 3.4 version pinning — a **different** race from this one. The one
genuinely reusable fact from it: it independently corroborates that `CreateSession` really does
kick off multiple pieces of async controller/hook wiring in detached goroutines that return
before completing, and that this async wiring is known to interleave with unrelated concurrent
operations in flaky ways. It does not touch `ptmx`/`attachCmd`/`closePTYAndAttachCmd` at all and
should not be conflated with this bug's root cause or fix.

## Summary

- No existing synchronization (no context cancellation, no `WaitGroup`, no channel) connects
  `CreateSession`'s async controller-start goroutine to `DeleteSession`'s cleanup goroutine —
  `Instance.mu` is explicitly dropped before the PTY is touched in `StartController`, and
  `StopController()` only unregisters a reference without waiting for in-flight callers.
- None of the outer layers (`TmuxBackend` — zero locks, pure forwarding; `TmuxProcessManager` —
  `atomic.Pointer[TmuxSession]` protects pointer *reassignment*, not the target's internal
  fields; `Instance.mu` — deliberately released before PTY access to avoid a known deadlock) can
  or should be extended to guard `ptmx`; the fix must live entirely inside `TmuxSession`, ideally
  as a `deadlock.Mutex` matching the existing `detachMutex`/`cmdSendMu` style.
- All 10 read/write sites for `ptmx`/`attachCmd`/`attachCmdWaitOnce` named in requirements.md
  were confirmed and no extra site was found; access is fully internal to `TmuxSession` (no
  external caller holds a reference to the field itself), so an internal-only lock is sufficient
  — `Attach()`'s two long-lived goroutines are the one implementation wrinkle worth flagging for
  the plan phase since they read `t.ptmx` repeatedly across their lifetime, not just once.
