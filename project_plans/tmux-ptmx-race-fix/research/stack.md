# Research: concurrency primitive for `ptmx`/`attachCmd`/`attachCmdWaitOnce`

## Recommendation

**Option (a): a dedicated `deadlock.Mutex` (named e.g. `ptyMu`) guarding exactly these three
fields, accessed only through getter/setter helper methods.** Reject (b) (per-field atomics)
on correctness grounds and (c) (reuse an existing lock) on both domain-fit and deadlock
grounds. Details below.

## 1. Existing lock usage in `session/tmux/tmux.go`

| Field | Type | Scope | Guards |
|---|---|---|---|
| `detachMutex` (tmux.go:88) | `deadlock.Mutex` | per-instance | `detaching` flag + serializes `DetachSafely` |
| `controlModeSubMu` (tmux.go:136) | `sync.RWMutex` | per-instance | `controlModeSubscribers`, `controlModeExited`, `pendingCmds`, `controlModeRefCount` |
| `controlModeStartMu` (tmux.go:138) | `sync.Mutex` | per-instance | serializes control-mode Start/Stop |
| `cmdSendMu` (tmux.go:149) | `deadlock.Mutex` | per-instance | stdin-close vs. sender-goroutine writes in control mode |
| `recoveryMu` (tmux.go:198) | `deadlock.Mutex` | **package-level `var`, not per-instance** | `recoveryInFlight` — confirmed intentional: comment above it ("When the server dies all sessions detect the failure simultaneously; only one should run EnsureServerRunning...") explains it's deliberately global so *all* `TmuxSession` instances serialize on a single shared recovery attempt, not a per-instance bug. |

`deadlock.Mutex`/`deadlock.RWMutex` (`github.com/linkdata/deadlock v0.5.5`, `go.mod:23`) are
confirmed drop-in wrappers — `go doc github.com/linkdata/deadlock Mutex` shows:
```
type Mutex struct{ sync.Mutex }
    Mutex is sync.Mutex wrapper
type RWMutex struct{ sync.RWMutex }
    RWMutex is sync.RWMutex wrapper
```
They embed the real `sync` type and add opt-in deadlock detection (`deadlock.Enabled`/`deadlock.Debug`,
both `false` by default per `go doc`), so `Lock()`/`Unlock()`/`RLock()`/`RUnlock()` behave
identically to stdlib in production. No semantic difference from `sync.Mutex` other than the
detector.

**Convention check**: `deadlock.Mutex`/`RWMutex` is the dominant pattern package-wide — 30
occurrences across `session/` (health.go, pty_discovery.go, external_tmux_streamer.go,
pr_status_poller.go, prompts/store.go, approval_policy.go, git_worktree_manager.go,
history_linker.go, review_queue_poller.go, unfinished/*.go, instance.go, instance_shells.go,
tmux/fork_metrics.go, tmux/server_registry_test.go), vs. only 2 uses of plain `sync.Mutex`/
`sync.RWMutex` for per-instance locks in the entire package, both of which are in this same
file (`controlModeSubMu`, `controlModeStartMu` — older code, predates the deadlock-detection
convention). **Any new mutex in this codebase should be `deadlock.Mutex`/`deadlock.RWMutex`,
not plain `sync.Mutex`.**

## 2. Evaluation of the three options

### (a) Dedicated mutex guarding just the 3 fields — RECOMMENDED

Add a `ptyMu deadlock.Mutex` field (name to be finalized in the plan phase) next to `ptmx`/
`attachCmd`/`attachCmdWaitOnce` in the struct, and route every read/write through helper
methods, e.g.:
```go
func (t *TmuxSession) getPTY() *os.File           // read-lock, return t.ptmx
func (t *TmuxSession) setPTYAndAttachCmd(f *os.File, cmd *exec.Cmd, once *sync.Once)  // write-lock, set all 3
func (t *TmuxSession) clearPTYAndAttachCmd() (*os.File, *exec.Cmd, *sync.Once)        // write-lock, nil all 3, return old values for the caller to Close()/Kill()/Wait() outside the lock
```
This is the only option that gives atomicity across all three fields as a unit, matching the
non-goal in requirements.md ("`t.attachCmd`/`t.attachCmdWaitOnce` are written under the exact
same critical sections as `t.ptmx` at every call site"). It's a direct, minimal-surface-area
extension of the existing pattern already used for `detachMutex`/`cmdSendMu` in the same file
— a small `deadlock.Mutex` dedicated to one specific piece of state, not a broad instance-wide
lock.

Blocking-goroutine caveat that must carry into the plan: `Attach()`'s two goroutines
(tmux.go:1484 `io.Copy(os.Stdout, t.ptmx)`, tmux.go:1544 `t.ptmx.Write(buf[:nr])`) currently
close over `t.ptmx` implicitly via the receiver `t`. The fix must read `t.ptmx` under the lock
**once**, capture it into a local `*os.File`, and have the goroutine body use only that local
copy — the goroutine must not re-enter the lock on every loop iteration (that would be both a
behavior-preserving mismatch and needless contention on a hot path). This preserves the
existing lifecycle semantics (goroutine dies when the captured `*os.File` returns EOF/error
after `closePTYAndAttachCmd` closes it) while making the *initial* field access race-free.

### (b) `atomic.Pointer[os.File]` for `ptmx` alone — REJECTED

`lastKnownCols`/`lastKnownRows` (tmux.go:111-112) and `existsCache` (tmux.go:117) already show
the codebase is comfortable with `atomic.*` for genuinely independent scalar/snapshot state.
But `attachCmd` (`*exec.Cmd`) and `attachCmdWaitOnce` (`*sync.Once`) are reassigned in lockstep
with `ptmx` at every single write site (tmux.go:864-866, 1265-1268, 1689-1696+1716-1717) — not
independent values. Using `atomic.Pointer[os.File]` for `ptmx` would require *also* wrapping
`attachCmd` in `atomic.Pointer[exec.Cmd]` and `attachCmdWaitOnce` in `atomic.Pointer[sync.Once]`
to be race-free per-field, but three independent atomics give **no cross-field atomicity**: a
concurrent reader could observe the new `ptmx` already stored via its atomic while `attachCmd`
still holds the *old* (possibly already-killed) command, or observe `ptmx` cleared to nil while
`attachCmd` is still the live process, depending on store order — exactly the "relocate the
same class of race one field over" failure mode the requirements doc explicitly warns against
(requirements.md:47-52, "Non-goals"). Making the triple atomic *as a unit* is precisely what a
mutex gives you and three independent atomics cannot, short of also introducing a fourth
synchronization mechanism (e.g. a version counter/CAS loop) to stitch the three atomics back
into one atomic transaction — at which point you've reinvented a mutex with extra steps.
**Disqualified on correctness, not style.**

### (c) Reuse `controlModeStartMu` or another existing lock — REJECTED

- `controlModeStartMu`/`controlModeSubMu` guard an unrelated subsystem (control-mode process
  start/stop, `-C` attach stdin/stdout piping) that has no current interaction with
  `ptmx`/`attachCmd`/`attachCmdWaitOnce` (those are the *interactive* `attach-session` PTY, a
  separate `exec.Cmd` from `controlModeCmd`). Coupling two unrelated pieces of state under one
  lock only to save a field declaration adds an artificial dependency: any future change to
  control-mode locking could now unintentionally block PTY access and vice versa.
- `detachMutex` is the closest domain match (also PTY/attach lifecycle) but is disqualified for
  a concrete deadlock reason: `DetachSafely()` (tmux.go:1553-1556) takes `detachMutex.Lock()`
  and, while still holding it, calls `t.Restore()` (tmux.go:1648) → `RestoreWithWorkDir`, which
  is one of the three sites that writes `t.ptmx`/`t.attachCmd`/`t.attachCmdWaitOnce`
  (tmux.go:1265-1268). `deadlock.Mutex` embeds `sync.Mutex`, which is **not reentrant** — a
  second `Lock()` call by the same goroutine while already holding it blocks forever (and
  `deadlock.Mutex`'s own detector would flag this as a same-goroutine self-deadlock if enabled).
  Reusing `detachMutex` to also guard the PTY triple would require either (i) making
  `RestoreWithWorkDir`'s PTY-guarding calls lock-free when called from inside `DetachSafely`
  (fragile, easy to regress) or (ii) switching to a reentrant-lock pattern the rest of the file
  doesn't use anywhere. Both add more risk than a new 3-line mutex declaration.
- `cmdSendMu`/`recoveryMu` are further from the domain (control-mode stdin, package-level
  server-recovery) with no shared call sites at all.

**Disqualified**: domain mismatch for `controlModeStartMu`, concrete self-deadlock risk for
`detachMutex`.

## 3. `deadlock` package version and adoption

- `go.mod:23`: `github.com/linkdata/deadlock v0.5.5`.
- Confirmed project-wide convention (not just this file) — see the 30-occurrence grep above.
  Every *new* per-instance lock added to `session/` in recent history uses `deadlock.Mutex`/
  `deadlock.RWMutex`; plain `sync.Mutex`/`sync.RWMutex` only remains on two pre-existing fields
  in this exact file. A new lock for this fix should follow the dominant convention:
  `deadlock.Mutex`.

## 4. Exhaustive call-site list: `t.ptmx`, `t.attachCmd`, `t.attachCmdWaitOnce`

All line numbers from `session/tmux/tmux.go` (2499 lines total), current HEAD.

### `t.ptmx`

| Line | Function | Access | Note |
|---|---|---|---|
| 859 | `AttachToExisting` | read (nil check) | guards whether to attach |
| 864 | `AttachToExisting` | **write** | sets new PTY |
| 1240 | `RestoreWithWorkDir` | read (nil check) | decides whether to close existing PTY first |
| 1241 | `RestoreWithWorkDir` | call `t.closePTYAndAttachCmd()` | indirect write via helper |
| 1243 | `RestoreWithWorkDir` | read (nil check) | guards retry loop entry |
| 1265 | `RestoreWithWorkDir` | **write** | sets new PTY (retry loop) |
| 1309 | `TapEnter` | read | `t.ptmx.Write(...)` |
| 1318 | `TapDAndEnter` | read | `t.ptmx.Write(...)` |
| 1326 | `SendKeys` | read | `t.ptmx.Write(...)` |
| 1333 | `GetPTY` | read (nil check) | the confirmed race's reader (per requirements.md) |
| 1336 | `GetPTY` | read | returns `t.ptmx` |
| 1484 | `Attach` (goroutine 1) | read | `io.Copy(os.Stdout, t.ptmx)` — captured once at goroutine start, see §2(a) caveat |
| 1544 | `Attach` (goroutine 2) | read | `t.ptmx.Write(buf[:nr])` — same captured-once caveat |
| 1689 | `closePTYAndAttachCmd` | read (nil check) | the confirmed race's writer-path entry (per requirements.md) |
| 1690 | `closePTYAndAttachCmd` | read | `t.ptmx.Close()` |
| 1696 | `closePTYAndAttachCmd` | **write** | `t.ptmx = nil` — the exact line named in the confirmed `-race` report |
| 1781 | `updateWindowSize` | read (nil check) | |
| 1786 | `updateWindowSize` | read | `t.ptmx.Fd()` |
| 1800 | `updateWindowSize` | read | `pty.Setsize(t.ptmx, ...)` |

(Line 1647 is a comment only, not a code access — excluded.)

### `t.attachCmd`

| Line | Function | Access | Note |
|---|---|---|---|
| 865 | `AttachToExisting` | **write** | `t.attachCmd = cmd` |
| 1266 | `RestoreWithWorkDir` | **write** | `t.attachCmd = attachCmd` (retry loop) |
| 1701 | `closePTYAndAttachCmd` | read (nil check, compound: `t.attachCmd != nil && t.attachCmd.Process != nil`) | |
| 1702 | `closePTYAndAttachCmd` | read | logs `t.attachCmd.Process.Pid` |
| 1703 | `closePTYAndAttachCmd` | read | `t.attachCmd.Process.Kill()` |
| 1712 | `closePTYAndAttachCmd` | read | `t.attachCmd.Wait()` inside `once.Do` |
| 1714 | `closePTYAndAttachCmd` | read | `t.attachCmd.Wait()` (else branch, no `once`) |
| 1716 | `closePTYAndAttachCmd` | **write** | `t.attachCmd = nil` |

### `t.attachCmdWaitOnce`

| Line | Function | Access | Note |
|---|---|---|---|
| 866 | `AttachToExisting` | **write** | `t.attachCmdWaitOnce = new(sync.Once)` |
| 1268 | `RestoreWithWorkDir` | **write** | `t.attachCmdWaitOnce = waitOnce` (retry loop); the same `waitOnce` local is also handed to the diagnostic goroutine at line 1272-1276 (`go func(..., once *sync.Once) { once.Do(func() { err = cmd.Wait() }) }`) — that goroutine closes over the **local** `waitOnce` variable, not `t.attachCmdWaitOnce`, so it needs no lock of its own; only the assignment to the struct field at line 1268 is a race. |
| 1711 | `closePTYAndAttachCmd` | read (nil check) | |
| 1712 | `closePTYAndAttachCmd` | read | `t.attachCmdWaitOnce.Do(...)` |
| 1717 | `closePTYAndAttachCmd` | **write** | `t.attachCmdWaitOnce = nil` |

### Summary of write sites (must all move into the new critical section as atomic triples)

1. `AttachToExisting`, tmux.go:864-866 — sets all 3 fields together.
2. `RestoreWithWorkDir` (retry loop), tmux.go:1265-1268 — sets all 3 fields together.
3. `closePTYAndAttachCmd`, tmux.go:1689-1696 (ptmx close+nil) and tmux.go:1701-1717
   (attachCmd/attachCmdWaitOnce kill+wait+nil) — these two blocks are currently sequential
   within one function but operate on the shared triple; both must be covered by the new lock
   (they can be one critical section or two, but must not let another goroutine observe a
   half-cleared triple in between — recommend one critical section spanning both, or capture
   old values under lock and do the actual `Close()`/`Kill()`/`Wait()` I/O outside the lock to
   avoid holding it during blocking syscalls — this tradeoff is for the plan phase to decide).

### Reader sites (must acquire read access before dereferencing)

`GetPTY` (1332-1336), `TapEnter` (1309), `TapDAndEnter` (1318), `SendKeys` (1326),
`updateWindowSize` (1781-1800), `Attach`'s two goroutines (1484, 1544, one-time capture),
`AttachToExisting`'s and `RestoreWithWorkDir`'s own nil-checks (859, 1240, 1243) before they
write.
