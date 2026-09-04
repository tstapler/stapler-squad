# Research: pitfalls retrofitting a mutex onto `TmuxSession.ptmx`/`attachCmd`/`attachCmdWaitOnce`

Scope: `session/tmux/tmux.go`. Grounds the design phase in what actually goes wrong when a
previously-unsynchronized `*os.File` field gets a mutex bolted on after the fact.

## 1. Don't hold the new lock across blocking I/O

Call sites that must NOT be inside the critical section for their `Write`/`Close`/`Copy` call:

- `Attach()`'s first goroutine: `io.Copy(os.Stdout, t.ptmx)` (tmux.go:1484) — unbounded blocking
  read loop, lives for the whole attach session.
- `Attach()`'s second goroutine: `t.ptmx.Write(buf[:nr])` inside a `for` loop reading stdin
  (tmux.go:1544) — re-evaluates `t.ptmx` **every iteration** today (it's a fresh field read each
  time through the loop, not hoisted), so if `Restore()` swaps in a new PTY mid-loop this
  goroutine transparently starts writing to the new one. That's existing behavior and is a
  non-goal to change (see requirements.md "No behavior change to PTY lifecycle semantics") — the
  fix must preserve "read the current pointer on every iteration," it just can't hold a lock
  across the `Write` itself.
- `closePTYAndAttachCmd()` — `t.ptmx.Close()` (tmux.go:1690) is fast/non-blocking in practice, but
  `t.attachCmd.Process.Kill()` + `t.attachCmdWaitOnce.Do(func() { t.attachCmd.Wait() })`
  (tmux.go:1703-1714) can block on process reaping. If `attachCmd`/`attachCmdWaitOnce` move under
  the same lock as `ptmx` (required per requirements.md's explicit scope), the `Wait()` call must
  happen with the lock released, or every reader of `ptmx` blocks for the duration of process
  reaping on every session teardown.

**Standard pattern**: snapshot the pointer(s) under the lock, release, then do I/O on the local
copy.

```go
t.ptmxMu.Lock()
f := t.ptmx
t.ptmxMu.Unlock()
if f == nil {
    return nil, fmt.Errorf("PTY not initialized")
}
return f, nil // or f.Write(...), io.Copy(dst, f), etc. — outside the lock
```

For `closePTYAndAttachCmd()`, the same idea applies to the teardown fields: swap `t.ptmx`,
`t.attachCmd`, `t.attachCmdWaitOnce` to `nil` under the lock while capturing the old values in
locals, release the lock, then run `Close()`/`Kill()`/`Wait()` against the locals. This also
naturally prevents two concurrent callers of `closePTYAndAttachCmd()` from double-closing/double-
killing the same fd/process — only whichever goroutine wins the lock gets a non-nil local to act
on; the loser sees nils it swapped in are already nil and skips the block. That's a strictly
stronger guarantee than what exists today (today's "file already closed" string-matching in
`Detach()`/`DetachSafely()` at tmux.go:1626-1643 exists precisely because concurrent double-close
is currently possible).

## 2. Is `Write`/`Close` racing on the same `*os.File` value memory-unsafe?

No — this is a Go stdlib guarantee independent of any mutex we add, and it's the reason the
existing code's "file already closed" string-matching (tmux.go:1692, 1705, 1629, 1641) works at
all today. `os.File` (and `net.Conn` implementations that wrap a file descriptor) are built on
`internal/poll.FD`, which holds its own internal per-fd `fdMutex` with reference counting
specifically so that a concurrent `Close()` racing a `Read`/`Write` on the *same* `*os.File`
value is safe: the in-flight `Read`/`Write` either completes normally, or `Close()` waits for
outstanding I/O to unblock (on POSIX, by interrupting it) and the losing operation returns an
`fs.ErrClosed`-wrapped error ("file already closed" / "use of closed file"). It does not panic,
corrupt memory, or invoke UB. This is why `-race` is not actually flagging "is it safe to call
`.Write()` on a `*os.File` that another goroutine might `Close()`" — it's flagging the plain,
unsynchronized read/write of the **Go pointer variable** `t.ptmx` on the struct (a field access
with no happens-before edge), which is a genuine data race under the Go memory model even though
in practice a pointer-sized store/load rarely tears on common architectures.

Practical implication for the design: once a goroutine has captured a valid `*os.File` into a
local variable (under the lock, per §1), it is safe to call `Write`/`Read` on that local copy
even if another goroutine concurrently calls `Close()` on the "same" file via `t.ptmx` (which by
then may already be a different pointer, or nil, in the struct). The local copy's `Write` will
either succeed or return a `closed file` error — never crash. So the lock only needs to protect
the **field access**, not serialize field access against in-flight I/O on already-captured
handles.

## 3. Deadlock / reentrancy check against existing locks

Grepped every `Lock()/RLock()` in `session/tmux/*.go` (`detachMutex`, `controlModeSubMu`,
`controlModeStartMu`, `cmdSendMu`, `recoveryMu`) and traced call sites of
`closePTYAndAttachCmd()` and `GetPTY()`:

- **`detachMutex`**: `DetachSafely()` (tmux.go:1555) and `Detach()` (tmux.go:1609) both acquire
  `detachMutex` and then call `closePTYAndAttachCmd()` directly (tmux.go:1572, 1624) *and*
  indirectly via `t.Restore()` → `RestoreWithWorkDir()` → `closePTYAndAttachCmd()`
  (tmux.go:1241, reached from `Detach()`'s call to `Restore()` at tmux.go:1648). This means
  `closePTYAndAttachCmd()` is called **twice sequentially** within one `detachMutex` critical
  section on the `Detach()` path. That's fine for a plain (non-reentrant) new lock as long as
  each call is its own acquire/release — never held open across both calls — since they're
  sequential, not nested, on the same goroutine. Lock order is `detachMutex` → new lock, never
  the reverse, and the new lock is always released before `detachMutex` is released.
- **`controlModeStartMu` / `controlModeSubMu` / `cmdSendMu`**: confirmed via `control_mode.go` —
  control mode uses a completely separate process (`t.controlModeCmd`) with its own
  `controlModeStdin`/`controlModeStdout` pipes, never touches `t.ptmx`. These locks are orthogonal
  to the ptmx lock; no ordering constraint needed between them.
- **`recoveryMu`**: package-level (not per-`TmuxSession`) lock guarding session recovery
  (tmux.go:1858-1869), also doesn't touch `t.ptmx` directly in the surrounding code. Orthogonal.
- **`GetPTY()` callers** (`session/tmux_process_manager.go:243`, `session/tmux_backend.go:53`,
  `session/instance.go:907,977,1109,1203`, `session/instance_shells.go:195,337`,
  `session/instance_tmux.go:443`): none of these call sites hold any of `TmuxSession`'s internal
  mutexes — they're all one layer up in `session/` package code operating through the
  `TmuxProcessManager`/`TmuxBackend`/`Instance` abstractions. No reentrancy risk found.

Net: a single new leaf lock (never held while calling out to another lock in this file) covering
just `ptmx`/`attachCmd`/`attachCmdWaitOnce` field access does not create a new cycle with any
existing lock. Document this explicitly in the implementation (comment + maybe a short "lock
order" note near the field declarations) since `detachMutex` → new-lock is now a real (if benign)
nesting relationship that didn't exist before.

## 4. Making the regression test actually catch the race (not just "look right")

The acceptance criteria's `-count=10` / `-count=20` repetition is a probabilistic net, not a
guarantee — it only catches the race if the specific goroutine interleaving (GetPTY reading
`ptmx` concurrently with `Close()`'s goroutine nil-ing it) actually gets scheduled within those
runs. Go's race detector only flags races it *observes*; a fix that "looks right" but leaves a
narrower window (e.g., a lock that's dropped one line too early) can pass `-count=20` today and
regress silently months later when scheduling happens to change (GOMAXPROCS, added logging,
different hardware).

Two complementary things to design for, not just repetition count:

- **Deterministic reproduction test**, not just a stress test: add a unit test at the
  `TmuxSession` level that forces the exact interleave from the bug report — goroutine A calls
  `GetPTY()`, goroutine B concurrently calls `closePTYAndAttachCmd()` (or `Close()`) — using an
  explicit synchronization point (e.g., a test-only channel/hook fired right before the field
  read in `GetPTY()`) so the test *always* exercises the interleave instead of hoping the
  scheduler does. This repo is on Go 1.26 (`go.mod`), so `testing/synctest` (stable since Go 1.24)
  is available and worth evaluating in the design phase: it lets a test control goroutine
  scheduling deterministically inside a "bubble" (via `synctest.Wait()` to park until all
  goroutines in the bubble are blocked) while `-race` still observes genuine concurrent execution
  within it — a stronger guarantee than `-count=N` alone. Flag this as a design-phase choice, not
  a foregone conclusion — first confirm the test can be written without a synctest-only clock
  dependency, since this code path doesn't inherently need virtual time.
- **Broader stress test** matching the acceptance criteria's intent (concurrent
  `CreateSession`/`DeleteSession`, tmux.go's actual repro path through
  `SessionService.CreateSession`'s async controller-start goroutine vs `DeleteSession`'s cleanup
  goroutine) still has value as an integration-level regression net beyond the narrow unit test,
  and is what's already prescribed in requirements.md criteria 2-3. Keep both — the deterministic
  unit test guards the exact mechanism, the stress test guards the exact reported scenario.

## 5. Known Go idiom for "readers use a resource, one writer can close+swap it"

This is a well-established pattern in the Go ecosystem, not something to invent from scratch:

- **`os/exec.Cmd`** already uses exactly this shape in this same file: `attachCmdWaitOnce
  *sync.Once` (tmux.go:65-68) guards `attachCmd.Wait()` so it's called exactly once regardless of
  which goroutine gets there first (`closePTYAndAttachCmd()` vs the diagnostic goroutine spawned
  in `RestoreWithWorkDir()` at tmux.go:1272-1276). The design should keep this `sync.Once`-per-
  generation pattern rather than replacing it — the new mutex protects the **field**, the
  `sync.Once` still protects the **Wait() call** itself from being invoked twice for the same
  `attachCmd` generation.
- **General idiom** (seen throughout gRPC-go's transport layer, and in `net`-adjacent code that
  wraps a swappable connection): guard the field with a mutex scoped only to
  read/write/nil-check of the pointer; never call blocking methods on the pointed-to value while
  holding that mutex. Optionally add a small helper (e.g. `t.getPTYLocked() (*os.File, *exec.Cmd,
  *sync.Once)` returning all three fields as a single locked snapshot) so every one of the 10+
  call sites goes through one function instead of hand-rolling lock/unlock at each site — reduces
  the chance a future call site is added without the lock.
- Nothing repo-external needed beyond general knowledge here (no new dependency) — this is
  stdlib-shaped, consistent with `.claude/rules/interface-pollution-checklist.md`'s preference for
  concrete types over premature abstraction: a plain `sync.Mutex` + a couple of small accessor
  methods, not an interface.
