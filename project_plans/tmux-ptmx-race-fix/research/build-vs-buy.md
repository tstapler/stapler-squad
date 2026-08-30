# Build vs. Buy: synchronizing `TmuxSession.ptmx` / `attachCmd` / `attachCmdWaitOnce`

## Framing

There is no real "buy" option here — this is three private fields on an internal struct
(`session/tmux/tmux.go:58-68`), not a subsystem with an external service boundary. Per the
build-vs-buy ladder (stdlib first → native language feature → already-installed dependency →
minimum custom code), the actual question is **which already-available synchronization
primitive to reach for**, evaluated against this repo's own `go-concurrency` skill
(`~/dotfiles/.claude/skills/go-concurrency/SKILL.md`, referenced from this repo's
`.claude/CLAUDE.md` as `/go-concurrency`) and this file's own existing conventions.

## The three fields, and why they must move together

```go
// session/tmux/tmux.go:58-68
ptmx *os.File
attachCmd *exec.Cmd
attachCmdWaitOnce *sync.Once
```

All three are written together at every write site (`AttachToExisting` tmux.go:864-866,
`RestoreWithWorkDir` retry loop tmux.go:1265-1268, `closePTYAndAttachCmd` tmux.go:1689-1717)
and several read sites use `ptmx` for blocking I/O — critically, `Attach()`'s first goroutine
does `io.Copy(os.Stdout, t.ptmx)` (tmux.go:1484), which blocks for the entire lifetime of the
attach session, and the second goroutine calls `t.ptmx.Write(buf[:nr])` (tmux.go:1544) in a
`for` loop reading stdin. **A lock held across either of those violates this repo's own
`go-concurrency` skill's Core Principle** ("Never hold a mutex across I/O... Any lock held
during a network call, git subprocess, database query, or file read serializes all concurrent
callers for the full duration of that I/O"). This is the deciding constraint for every option
below: whatever primitive is chosen, the design must snapshot the `*os.File` under the lock
and then do I/O against the local copy — never hold the lock during `io.Copy`/`Write`.

## Option 1: `sync.Mutex` / `github.com/linkdata/deadlock.Mutex`

This file already uses `deadlock.Mutex` for exactly this kind of small-field guard:
`detachMutex` (tmux.go:88, guards `detaching`/`attachCh`), `cmdSendMu` (tmux.go:149, "guards
stdin-close in StopControlMode vs sender goroutine writes"), `recoveryMu` (tmux.go:198). It
also uses plain `sync.RWMutex`/`sync.Mutex` for `controlModeSubMu` and `controlModeStartMu`
(tmux.go:136,138). `github.com/linkdata/deadlock v0.5.5` is already a direct dependency
(`go.mod:23`) — it's a drop-in `sync.Mutex`/`sync.RWMutex` replacement that adds deadlock
detection (lock-order-cycle and stuck-lock diagnostics) with no API difference, so switching
between it and stdlib `sync.Mutex` later is free.

**Pros**
- Directly satisfies acceptance criterion 1: one consistent mechanism guards all three fields
  together, since they're always mutated as a unit.
- Matches this file's own established idiom (4 existing mutex-guarded field groups) — no new
  pattern for future maintainers to learn.
- Composable by construction: a single `Lock()`/`Unlock()` naturally covers a
  read-then-decide-then-write sequence across multiple fields (e.g. "if `t.ptmx != nil`, close
  it and nil out all three fields" in `closePTYAndAttachCmd`), which is exactly what
  compound-field atomics cannot do (see Option 2).
- Critical sections here are trivial — assign a pointer, check nil, maybe call
  `.Do()`/`.Kill()` — no I/O inside the lock as long as the actual `io.Copy`/`Write`/`Close`
  calls snapshot the file pointer first and release the lock before blocking on it.

**Cons**
- Requires touching all 10+ call sites to add lock/unlock (the acceptance criteria already
  scope this as expected work, not a downside unique to this option).
- `Close()` itself is a syscall, not pure memory — if naively done as
  `t.mu.Lock(); t.ptmx.Close(); t.ptmx = nil; t.mu.Unlock()`, `Close()` runs under the lock.
  This is a brief, non-blocking-in-practice syscall (not a network call or subprocess), so it
  is defensible, but the design should still snapshot-then-unlock-then-close where the extra
  discipline is cheap, per the skill's "never hold a mutex across I/O" principle taken
  strictly. Worth stating explicitly in the plan rather than leaving it implicit.

**Verdict: adopt.** This is rung 4 of the skill's ladder ("small critical section with mixed
reads/writes at moderate concurrency... correct for the vast majority of Go code — don't
preemptively avoid it") and matches 4 existing precedents in this exact file. Use
`deadlock.Mutex` specifically (not bare `sync.Mutex`) for consistency with `detachMutex`,
`cmdSendMu`, and `recoveryMu`, and to get free deadlock-cycle detection given a new lock is
being introduced alongside `detachMutex`/`controlModeSubMu`/`controlModeStartMu`/`cmdSendMu`/
`recoveryMu` (acceptance criterion 4 explicitly requires documenting lock-order safety here).

## Option 2: `atomic.Pointer[os.File]` (and equivalents for the other two fields)

`sync/atomic` is already imported (tmux.go:18) and already used for exactly this style of
lock-free field guard elsewhere in the same struct: `lastKnownCols`/`lastKnownRows`
(`atomic.Int32`, tmux.go:111-112), `existsCache` (`atomic.Value` snapshot, tmux.go:117,
documented as "lock-free via atomic.Value snapshot"), and `intentionalStop` (`atomic.Bool`,
tmux.go:161). So this is a real, precedented option in this file, not a hypothetical.

**Pros**
- Zero lock contention on the hot read paths (`GetPTY`, `TapEnter`, resize) — matches rung 2/3
  of the skill's ladder for "read constantly, written rarely" fields.
- `atomic.Pointer[os.File]` is generic and type-safe (Go 1.19+), replacing the file pointer
  cleanly; `attachCmd` (`*exec.Cmd`) could similarly become `atomic.Pointer[exec.Cmd]`.

**Cons — this is the deciding problem**
- **Composability**: `atomic.Pointer[T]` (or three separate atomics, one per field) gives no
  way to update or read `ptmx`, `attachCmd`, and `attachCmdWaitOnce` as one consistent unit.
  Every write site in this codebase sets all three together
  (`t.ptmx = ptmx; t.attachCmd = cmd; t.attachCmdWaitOnce = new(sync.Once)` at tmux.go:864-866,
  and the mirror at 1265-1268 and the mirror nil-out at 1689-1717). Three independent
  `.Store()` calls are not atomic *as a group* — a reader could observe a torn state (e.g. new
  `ptmx` with old/nil `attachCmd`) between the three stores, which is precisely the class of
  bug the skill's decision tree calls out: "Multiple fields that must be read consistently...
  → `atomic.Value` storing an IMMUTABLE snapshot struct." That would mean wrapping all three
  pointers in one `attachState struct { ptmx *os.File; cmd *exec.Cmd; waitOnce *sync.Once }`
  behind a single `atomic.Value`/`atomic.Pointer[attachState]` — functionally reinventing a
  mutex-guarded struct, but with worse ergonomics for the multi-step
  check-then-kill-then-nil-out sequence in `closePTYAndAttachCmd` (tmux.go:1689-1717), which
  needs to *read* the current state, conditionally call `.Close()`/`.Kill()`/`.Wait()`, and
  only then swap in a nil'd state — a compare-and-swap loop, not a plain store.
- `sync.Once` (`attachCmdWaitOnce`) has no atomic-swap equivalent that preserves its "exactly
  once" semantics across concurrent producers of a *new* `*sync.Once`; wrapping it in the same
  snapshot struct is required either way.

**Verdict: reject as the sole mechanism.** Correct for a single independent field (this file's
own `lastKnownCols`/`existsCache`/`intentionalStop` precedents are all single-field or
single-snapshot), but the three fields here are a compound unit with multi-step
read-modify-write logic at the write sites, which is exactly the case the skill's ladder
routes to rung 4 (mutex), not rung 2/3 (atomics) — see the ladder's explicit fallback: "Truly
complex multi-field update where atomics don't compose → `sync.Mutex` (last resort)."

## Option 3: golang.org/x/sync toolkit (already imported: `singleflight`)

This file already imports `golang.org/x/sync/singleflight` (tmux.go:27) for
`existsSF`/`noCacheSF`, coalescing concurrent `listSessionsRaw` subprocess calls. None of
`errgroup`, `semaphore`, or `singleflight` fit this problem:

- **`singleflight`** coalesces N *identical concurrent calls* into one execution — there's no
  "expensive duplicate call" being made here; `ptmx` is a stored field being read/written by
  different unrelated operations (create vs. delete), not repeated calls to the same function.
- **`errgroup`** is for parallel work with first-error cancellation — not applicable to a field
  guard.
- **`semaphore`** bounds concurrency N-at-a-time — not applicable.

**Verdict: not applicable.** Noted for completeness per the requirement to evaluate
third-party sync libraries already present in the file, but there is no fit; using any of
these here would be a category error, not overkill-but-defensible.

## Option 4: LLM-generated custom/lock-free structure

Is there any justification for something more exotic than a mutex (custom lock-free struct,
hazard pointers, RCU-by-hand, etc.)? **No**, and it's worth stating explicitly so the
implementation plan doesn't second-guess this later:

- Contention is not the problem being solved here — there is no profiling evidence (and none
  was requested by the acceptance criteria) showing `t.ptmx` access is a throughput
  bottleneck. The bug is a **correctness** race (two goroutines touching an unsynchronized
  pointer, one nil-ing it while another dereferences it), not a **performance** problem. The
  skill's diagnosis workflow is explicit that lock-free structures are reached for only "after
  profiling shows a genuine MPMC queue is the bottleneck" (rung 7) — this is nowhere near that
  rung.
  Access patterns here are bounded by the lifecycle of a single `TmuxSession` (attach, resize,
  detach, close): a handful of calls per session lifetime, not a hot loop.
- The skill's own words: "Never start at rung 7. Lock-free data structures fix a narrow
  problem and are easy to misapply," and "A lock-free queue... is not a substitute for a mutex
  guarding mutable struct fields." This is precisely "a mutex guarding mutable struct fields."
- A custom lock-free structure here would also fail acceptance criterion 4 (document lock
  ordering against existing mutexes) in spirit — a bespoke primitive has no established
  ordering discipline to reason about, unlike a plain mutex added to a file that already has
  five.

**Verdict: reject, unambiguously.** A rarely-contended lifecycle field guard is textbook
mutex territory per this repo's own skill.

## Final verdict

**Use `deadlock.Mutex`** (this file's existing convention — see `detachMutex`, `cmdSendMu`,
`recoveryMu`, tmux.go:88,149,198) as a single new lock — e.g. `ptmxMu deadlock.Mutex` — guarding
`ptmx`, `attachCmd`, and `attachCmdWaitOnce` together at every read and write site. This is
rung 4 of the repo's own `go-concurrency` skill ladder ("small critical section with mixed
reads/writes... correct for the vast majority of Go code"), matches 4 existing precedents in
this exact file rather than introducing a new pattern, and is the only option among those
evaluated that correctly handles the compound three-field update — `atomic.Pointer[T]` fails
on composability (Option 2), `singleflight`/`errgroup`/`semaphore` don't fit the problem shape
(Option 3), and a custom lock-free structure has no justification for a rarely-contended
correctness fix (Option 4).

Design constraint to carry into the implementation plan: per the skill's Core Principle
("never hold a mutex across I/O"), the two `Attach()` goroutines that block on `t.ptmx` for
extended I/O (`io.Copy(os.Stdout, t.ptmx)` at tmux.go:1484, and the stdin-forwarding
`t.ptmx.Write(buf[:nr])` loop at tmux.go:1544) must snapshot `t.ptmx` under the new mutex once
before entering their blocking loop/call, not hold the lock for the duration of the I/O. Lock
ordering: document that `ptmxMu` is a leaf lock (never calls into code that acquires
`detachMutex`, `controlModeSubMu`, `controlModeStartMu`, `cmdSendMu`, or `recoveryMu` while
held), consistent with how the other four single-purpose mutexes in this file are used.
