# Implementation Plan: tmux-ptmx-race-fix

**Feature**: Eliminate the confirmed `-race` data race on `TmuxSession.ptmx`/`attachCmd`/`attachCmdWaitOnce` by guarding all reads/writes in `session/tmux/tmux.go` with a dedicated mutex accessed through three small helper methods.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001: deadlock.Mutex over atomic.Pointer for the PTY triple](../decisions/ADR-001-deadlock-mutex-for-ptmx-triple.md)

---

## Step 0.5 — Creative Pass (Alternatives Considered)

1. **Dedicated `deadlock.Mutex` (`ptmxMu`) behind helper methods.** Strength: correct for a 3-field lockstep invariant with mixed reads/writes at low contention (this is a lifecycle field touched on attach/detach/restore/delete, not a hot loop); matches the file's dominant idiom (30 existing `deadlock.Mutex` uses vs 2 plain `sync.Mutex`). Weakness: adds one more lock to reason about in a file that already has 5 (`detachMutex`, `controlModeSubMu`, `controlModeStartMu`, `cmdSendMu`, `recoveryMu`) — mitigated by documenting it as a leaf lock.
2. **Per-field `atomic.Pointer[os.File]` / `atomic.Pointer[exec.Cmd]` / `atomic.Pointer[sync.Once]`.** Strength: lock-free, zero blocking on the hot read path (`GetPTY`, `SendKeys`). Weakness: three independent atomics cannot be updated as one unit — a reader can observe `ptmx` from generation N+1 and `attachCmd` from generation N (a "torn triple"), which is exactly the invariant requirements.md calls out as in-scope. Rejected.
3. **Reuse an existing lock (`detachMutex` or `controlModeStartMu`).** Strength: zero new fields. Weakness: `DetachSafely`/`Detach` hold `detachMutex` and call `Restore()` → `RestoreWithWorkDir()`, one of the three PTY-triple write sites — reusing `detachMutex` for the triple would self-deadlock on a non-reentrant lock. `controlModeStartMu` guards an unrelated subsystem (control-mode process/pipes) that never touches `t.ptmx`; reusing it adds false contention and a confusing coupling. Rejected.

**Chosen**: Option 1 (dedicated `deadlock.Mutex` + helper methods). Recorded in the Pattern Decisions table below, alongside two finer-grained implementation-shape decisions (helper-method count, and struct-vs-separate-fields for the triple's storage).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| PTY triple | The three `TmuxSession` fields that must be read/written together for correctness: `ptmx *os.File`, `attachCmd *exec.Cmd`, `attachCmdWaitOnce *sync.Once`. | Named in requirements.md's non-goals section. Storage stays as 3 separate fields (see Pattern Decisions row 4) — "triple" describes the invariant, not a new struct type. |
| `ptmxMu` | New `deadlock.Mutex` field on `TmuxSession` guarding exactly the PTY triple. Declared adjacent to the existing `ptmx`/`attachCmd`/`attachCmdWaitOnce` fields. | Leaf lock: never held while acquiring `detachMutex`, `controlModeSubMu`, `controlModeStartMu`, `cmdSendMu`, or `recoveryMu`. |
| `lockedPTMX()` | Unexported helper: `t.ptmxMu.Lock()` → read `t.ptmx` → `Unlock()` → return the `*os.File` (possibly nil). No error handling — callers decide what nil means. | Used internally by `GetPTY`, `updateWindowSize`, and both `Attach()` goroutines. |
| `setPTYTriple(file, cmd, waitOnce)` | Unexported helper: locks `ptmxMu`, assigns all three fields, unlocks. | Used at all 3 write sites that install a *new* PTY: `AttachToExisting`, `RestoreWithWorkDir`'s retry loop, and (indirectly, via `clearPTYTriple`) as the inverse operation in `closePTYAndAttachCmd`. |
| `clearPTYTriple()` | Unexported helper: locks `ptmxMu`, captures the current triple into local variables, sets all three fields to nil, unlocks, returns the captured locals. | Used once, at the top of `closePTYAndAttachCmd`. Implements the snapshot-then-release-then-I/O pattern (see Pattern Decisions). |
| Generation | Informal term (already used in existing code comments, e.g. `attachCmdWaitOnce`'s doc comment) for one attach-cycle's worth of `(ptmx, attachCmd, attachCmdWaitOnce)` values, replaced as a unit on each `AttachToExisting`/`Restore`. | Not a new field — clarifies why the triple must move together: mixing fields across generations is the bug being fixed. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| PTY triple synchronization primitive | Dedicated `deadlock.Mutex` (`ptmxMu`) guarding the lockstep triple | research/stack.md, research/build-vs-buy.md; ADR-011's own guideline table ("Complex State Transitions → sync.Mutex or Channels") | Per-field `atomic.Pointer[T]` (3 independent atomics) | Three independent atomics give no cross-field atomicity — a reader could observe `ptmx` from generation N+1 paired with `attachCmd` from generation N (a "torn triple"). ADR-011 itself classifies this as a "Complex State Transition," for which it recommends `sync.Mutex`/channels, not lock-free CAS. |
| PTY triple synchronization primitive | `deadlock.Mutex`, new field, not reused | research/pitfalls.md | Reuse `detachMutex` or `controlModeStartMu` | `detachMutex`: `DetachSafely` holds it, then calls `Restore()` → `RestoreWithWorkDir()`, one of the 3 triple-write sites — reusing it would self-deadlock (non-reentrant lock). `controlModeStartMu`: orthogonal subsystem, never touches `t.ptmx` — reuse would add false contention and an unjustified coupling. |
| `closePTYAndAttachCmd` cleanup ordering | Snapshot-then-release-then-I/O: capture the triple into locals under the lock, release, then `Close()`/`Kill()`/`Wait()` against the locals | research/pitfalls.md #1, #2 | Hold `ptmxMu` across `Close()`/`Kill()`/`Wait()` | Those calls can block on process teardown/OS scheduling; holding the lock across them would stall every concurrent `GetPTY`/`TapEnter`/`SendKeys`/resize call for the duration. `os.File`'s own `internal/poll` fdMutex already makes concurrent `Write`/`Close` on a captured local `*os.File` memory-safe, so releasing early costs nothing in safety — it also incidentally serializes concurrent `closePTYAndAttachCmd` callers against each other, since only one caller can ever receive a non-nil snapshot. |
| PTY triple storage shape | Keep the 3 existing named fields (`ptmx`, `attachCmd`, `attachCmdWaitOnce`) unchanged; add `setPTYTriple`/`clearPTYTriple` helpers that always touch all 3 together | This plan's Step 3 evaluation, informed by pitfalls.md #5 | Unify into one unexported `ptyState{file, cmd, waitOnce}` struct field | A struct would turn the "clear all 3 atomically" case into one assignment, but it renames the field at 10+ call sites for the rename's own sake, discards each field's individually-written doc comment, and inflates the diff for a bugfix whose own requirements flag blast radius as the reason it wasn't fixed inline. The two helper methods give the same "can't forget a field" guarantee (both fields are always read/written together inside one function body) without the rename tax. Revisit only if a second PTY-triple-shaped field group appears elsewhere in the file. |
| Read-site access | Locked accessor helper (`lockedPTMX`, and `GetPTY` built on top of it) as a thin Facade over `ptmxMu` | research/pitfalls.md #5; GoF Facade (encapsulating a subsystem — here, "lock + field read" — behind one call) | Hand-rolled `t.ptmxMu.Lock()`/`Unlock()` at each of the 6+ read call sites | Hand-rolling at every site is exactly the "a future call site forgets the lock" risk pitfalls.md warns about. A single accessor is the smallest surface that can be wrong, and every read site already funnels through `GetPTY`, `lockedPTMX`, or (for the resize path) `updateWindowSize`'s own single snapshot. |

---

## Observability Plan

No new metrics or logs are needed. This is an internal concurrency-correctness fix — the existing `log.Info`/`log.Error` calls in `AttachToExisting`, `RestoreWithWorkDir`, and `closePTYAndAttachCmd` are preserved unchanged (they now log using the locally-captured values instead of the struct fields directly, but the log lines and their conditions are identical).

## Risk Control
- **Feature flag**: not gated — internal correctness fix, no user-observable behavior change.
- **Rollback procedure**: standard revert via PR close + revert commit. No data/schema involved.
- **Staged rollout**: full rollout on merge.

## Unresolved Questions

None. (Acceptance criterion 4's "document lock order if a new mutex could nest" is satisfied by the doc comment added in Task 1.1.1a; it is not left open.)

## Dependency Visualization

```
Epic 1.1: Locking infrastructure + write sites
  Task 1.1.1a (add ptmxMu field + lock-order comment)
        |
        v
  Task 1.1.1b (add lockedPTMX/setPTYTriple/clearPTYTriple helpers)
        |
        +----------------+----------------+
        v                v                v
  Task 1.1.2a       Task 1.1.2b       Task 1.1.2c
  (AttachToExisting) (RestoreWithWorkDir) (closePTYAndAttachCmd)
        |                |                |
        +----------------+----------------+
                         v
Epic 1.2: Read sites + tests
        +----------------+----------------+----------------+----------------+
        v                v                v                v                v
  Task 1.2.1a      Task 1.2.1b      Task 1.2.1c      Task 1.2.1d      Task 1.2.1e
  (GetPTY)         (TapEnter/       (updateWindowSize) (Attach         (Attach
                    TapDAndEnter/                       goroutine 1)    goroutine 2)
                    SendKeys)
        +----------------+----------------+----------------+----------------+
                         v
                  Task 1.2.2a (deterministic regression test)
                         |
                         v
                  Task 1.2.2b (final verification: run all AC commands)
```

---

## Phase 1: PTY Triple Synchronization

### Epic 1.1: Locking infrastructure and write-site conversion
**Goal**: Introduce `ptmxMu` and its three helper methods, then convert every write site (`AttachToExisting`, `RestoreWithWorkDir`'s retry loop, `closePTYAndAttachCmd`) so the PTY triple is never partially written outside a critical section.

#### Story 1.1.1: Add `ptmxMu` and the PTY-triple helper methods
**As a** maintainer of `session/tmux/tmux.go`, **I want** one mutex and three small helper methods guarding the PTY triple, **so that** every call site (existing and future) has one obvious, hard-to-skip way to touch `ptmx`/`attachCmd`/`attachCmdWaitOnce`.
**Acceptance Criteria**:
- AC1 (requirements.md #1): `t.ptmx`, `t.attachCmd`, and `t.attachCmdWaitOnce` are never read or written outside a consistent synchronization mechanism.
  - *Given* the modified `session/tmux/tmux.go`, *When* grepping for `t.ptmx`, `t.attachCmd`, or `t.attachCmdWaitOnce` outside the bodies of `lockedPTMX`, `setPTYTriple`, and `clearPTYTriple`, *Then* zero matches remain (all other call sites go through `GetPTY()`, `lockedPTMX()`, `setPTYTriple()`, or the locals returned by `clearPTYTriple()`).
- AC4 (requirements.md #4, partial — lock order documentation): no new deadlock ordering is introduced.
  - *Given* the new `ptmxMu` field declaration, *When* a reviewer reads its doc comment, *Then* it states `ptmxMu` is a leaf lock — never acquired while holding `detachMutex`, `controlModeSubMu`, `controlModeStartMu`, `cmdSendMu`, or `recoveryMu` — matching pitfalls.md's finding that the only existing nesting (`DetachSafely` → `Restore()` → `RestoreWithWorkDir()`) is sequential, not nested.
**Files**: `session/tmux/tmux.go`

##### Task 1.1.1a: Add `ptmxMu deadlock.Mutex` field (~3 min)
- In the `TmuxSession` struct, immediately after the `attachCmdWaitOnce *sync.Once` field (tmux.go:68), add:
  ```go
  // ptmxMu guards the PTY triple: ptmx, attachCmd, and attachCmdWaitOnce, which must
  // always be read/written together (all three describe one attach "generation").
  // Leaf lock: never acquire ptmxMu while already holding detachMutex, controlModeSubMu,
  // controlModeStartMu, cmdSendMu, or recoveryMu. detachMutex → ptmxMu is fine (used
  // sequentially by DetachSafely → Restore → RestoreWithWorkDir, never nested); the
  // reverse order must never occur.
  ptmxMu deadlock.Mutex
  ```
- Files: `session/tmux/tmux.go`

##### Task 1.1.1b: Add `lockedPTMX`, `setPTYTriple`, `clearPTYTriple` helper methods (~5 min)
- Add the three unexported helpers near `closePTYAndAttachCmd` (tmux.go, just above its current definition at line ~1682):
  ```go
  // lockedPTMX returns the current ptmx pointer (possibly nil) under ptmxMu.
  func (t *TmuxSession) lockedPTMX() *os.File {
      t.ptmxMu.Lock()
      defer t.ptmxMu.Unlock()
      return t.ptmx
  }

  // setPTYTriple atomically installs a new PTY triple (one attach generation).
  func (t *TmuxSession) setPTYTriple(file *os.File, cmd *exec.Cmd, waitOnce *sync.Once) {
      t.ptmxMu.Lock()
      defer t.ptmxMu.Unlock()
      t.ptmx = file
      t.attachCmd = cmd
      t.attachCmdWaitOnce = waitOnce
  }

  // clearPTYTriple atomically captures and clears the PTY triple, returning the
  // captured values so the caller can run blocking cleanup (Close/Kill/Wait) outside
  // the lock. Safe to call even if the triple is already nil (returns nils).
  func (t *TmuxSession) clearPTYTriple() (file *os.File, cmd *exec.Cmd, waitOnce *sync.Once) {
      t.ptmxMu.Lock()
      defer t.ptmxMu.Unlock()
      file, cmd, waitOnce = t.ptmx, t.attachCmd, t.attachCmdWaitOnce
      t.ptmx, t.attachCmd, t.attachCmdWaitOnce = nil, nil, nil
      return file, cmd, waitOnce
  }
  ```
- Do not wire these into call sites yet — that's Story 1.1.2/Epic 1.2. This task only adds the (temporarily unused) helpers so `go vet`/lint may flag them as unused until the next tasks land; that's expected within this story's scope and resolved by Task 1.1.2a-c in the same PR.
- Files: `session/tmux/tmux.go`

#### Story 1.1.2: Convert all 3 write sites to use the helpers
**As a** maintainer, **I want** every place that installs or tears down a PTY generation to go through `setPTYTriple`/`clearPTYTriple`, **so that** no goroutine ever observes a half-written triple.
**Acceptance Criteria**:
- AC1 (requirements.md #1): as above, applied specifically to the 3 write sites.
  - *Given* `AttachToExisting()` is called concurrently with `closePTYAndAttachCmd()` on the same `TmuxSession`, *When* both run under `go test -race`, *Then* no race is reported on `ptmx`/`attachCmd`/`attachCmdWaitOnce` because both paths acquire `ptmxMu`.
- AC6 (requirements.md #6): no behavior change to PTY lifecycle semantics.
  - *Given* `closePTYAndAttachCmd`'s existing retry/EIO/orphan-kill logic (the `"file already closed"` and `"process already finished"`/`"no such process"` string-match suppressions), *When* the function is converted to operate on locals returned by `clearPTYTriple()`, *Then* the same suppressions apply to the same error strings and the same `errs` slice is returned — only the field access is guarded, not the logic.
**Files**: `session/tmux/tmux.go`

##### Task 1.1.2a: Convert `AttachToExisting` (~3 min)
- In `AttachToExisting()` (tmux.go:852-874), replace the nil-check-then-triple-write block:
  ```go
  if t.ptmx == nil {
      ptmx, cmd, err := t.ptyFactory.Start(t.buildAttachCommand())
      if err != nil {
          return fmt.Errorf("failed to attach PTY to session '%s': %w", t.sanitizedName, err)
      }
      t.ptmx = ptmx
      t.attachCmd = cmd
      t.attachCmdWaitOnce = new(sync.Once)
      log.Info(...)
  }
  ```
  with:
  ```go
  if t.lockedPTMX() == nil {
      ptmx, cmd, err := t.ptyFactory.Start(t.buildAttachCommand())
      if err != nil {
          return fmt.Errorf("failed to attach PTY to session '%s': %w", t.sanitizedName, err)
      }
      t.setPTYTriple(ptmx, cmd, new(sync.Once))
      log.Info("successfully attached PTY to existing tmux session", "session", t.sanitizedName, "pid", cmd.Process.Pid)
  }
  ```
- Files: `session/tmux/tmux.go`

##### Task 1.1.2b: Convert `RestoreWithWorkDir`'s pre-check and retry loop (~4 min)
- At tmux.go:1240-1243, replace the two direct `t.ptmx` nil checks:
  ```go
  if t.ptmx != nil {
      _ = t.closePTYAndAttachCmd()
  }
  if t.ptmx == nil {
  ```
  with:
  ```go
  if t.lockedPTMX() != nil {
      _ = t.closePTYAndAttachCmd()
  }
  if t.lockedPTMX() == nil {
  ```
- At tmux.go:1265-1268, inside the retry loop, replace:
  ```go
  t.ptmx = ptmx
  t.attachCmd = attachCmd
  waitOnce := new(sync.Once)
  t.attachCmdWaitOnce = waitOnce
  ```
  with:
  ```go
  waitOnce := new(sync.Once)
  t.setPTYTriple(ptmx, attachCmd, waitOnce)
  ```
  (the local `waitOnce` variable is still needed unchanged for the diagnostic goroutine closure at tmux.go:1272-1276, which closes over the local, not the field — no lock needed there per architecture.md's finding).
- Files: `session/tmux/tmux.go`

##### Task 1.1.2c: Convert `closePTYAndAttachCmd` to snapshot-then-release-then-I/O (~5 min)
- Replace the body of `closePTYAndAttachCmd()` (tmux.go:1685-1721):
  ```go
  func (t *TmuxSession) closePTYAndAttachCmd() []error {
      var errs []error
      file, cmd, waitOnce := t.clearPTYTriple()

      if file != nil {
          if err := file.Close(); err != nil {
              if !strings.Contains(err.Error(), "file already closed") {
                  errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
              }
          }
      }

      if cmd != nil && cmd.Process != nil {
          log.Info("killing orphaned tmux attach process", "session", t.sanitizedName, "pid", cmd.Process.Pid)
          if err := cmd.Process.Kill(); err != nil {
              if !strings.Contains(err.Error(), "process already finished") && !strings.Contains(err.Error(), "no such process") {
                  errs = append(errs, fmt.Errorf("error killing attach process: %w", err))
              }
          }
          if waitOnce != nil {
              waitOnce.Do(func() { _ = cmd.Wait() })
          } else {
              _ = cmd.Wait()
          }
      }

      return errs
  }
  ```
- This is a pure refactor of the existing logic onto locals — same suppressed-error strings, same `errs` accumulation, same kill/wait ordering. The only behavioral difference (an intended improvement, not a lifecycle change) is that concurrent callers of `closePTYAndAttachCmd` now naturally serialize: only the first caller to acquire `ptmxMu` inside `clearPTYTriple()` gets non-nil locals, so a second concurrent caller is a fast no-op instead of racing on `.Close()`/`.Kill()` — this directly narrows the existing `"file already closed"` string-match workaround's necessity without removing it (removal is out of scope; the workaround stays as defense-in-depth per non-goals).
- Files: `session/tmux/tmux.go`

---

### Epic 1.2: Read-site conversion and regression coverage
**Goal**: Convert every read site to the locked accessors, then add a deterministic test that forces the exact `GetPTY()`-vs-`closePTYAndAttachCmd()` interleave from the original `-race` report, plus run the full acceptance-criteria verification suite.

#### Story 1.2.1: Convert all read sites to locked accessors
**As a** maintainer, **I want** every reader of the PTY triple to go through `lockedPTMX()`/`GetPTY()`, **so that** readers never observe a torn or half-cleared triple.
**Acceptance Criteria**:
- AC1 (requirements.md #1), applied to reads: *Given* `GetPTY()` is called concurrently with `closePTYAndAttachCmd()` on the same `TmuxSession` (the exact interleave from the original `-race` report: `SessionService.CreateSession`'s async controller-start goroutine vs `SessionService.DeleteSession`'s cleanup goroutine), *When* both run under `go test -race`, *Then* no race is reported and `GetPTY()` returns either the valid `*os.File` from the still-live generation or the `"PTY not initialized - session may not be started"` error — never a torn read.
- AC6 (requirements.md #6): the `Attach()` stdin-forward goroutine's existing "re-reads `t.ptmx` every loop iteration, picks up a mid-session `Restore()` swap" behavior is preserved exactly.
  - *Given* an attached session where `Restore()` installs a new PTY mid-loop (a new generation via `setPTYTriple`), *When* the stdin-forward goroutine's next loop iteration executes `t.lockedPTMX()`, *Then* it observes the new generation's `*os.File` and writes to it — not a stale pointer captured at goroutine start.
**Files**: `session/tmux/tmux.go`

##### Task 1.2.1a: Convert `GetPTY` (~2 min)
- Replace tmux.go:1332-1337:
  ```go
  func (t *TmuxSession) GetPTY() (*os.File, error) {
      file := t.lockedPTMX()
      if file == nil {
          return nil, fmt.Errorf("PTY not initialized - session may not be started")
      }
      return file, nil
  }
  ```
- Files: `session/tmux/tmux.go`

##### Task 1.2.1b: Convert `TapEnter`, `TapDAndEnter`, `SendKeys` (~4 min)
- tmux.go:1308-1327, replace each method's direct `t.ptmx.Write(...)` with a `GetPTY()` call first:
  ```go
  func (t *TmuxSession) TapEnter() error {
      file, err := t.GetPTY()
      if err != nil {
          return fmt.Errorf("error sending enter keystroke to PTY: %w", err)
      }
      _, err = file.Write([]byte{0x0D})
      if err != nil {
          return fmt.Errorf("error sending enter keystroke to PTY: %w", err)
      }
      return nil
  }
  ```
  Mirror the same pattern for `TapDAndEnter` (bytes `0x44, 0x0D`) and `SendKeys` (returns `(int, error)`, so thread the byte count through: `return file.Write([]byte(keys))` after the `GetPTY()` nil check, returning `(0, err)` on the not-initialized path).
- Note: this makes the previously-implicit "os.File nil-receiver methods return `ErrInvalid` safely" behavior into an explicit, more informative error (`"PTY not initialized..."`) — memory-safety was never at risk (pitfalls.md #2), this only improves the error message. Not a lifecycle behavior change.
- Files: `session/tmux/tmux.go`

##### Task 1.2.1c: Convert `updateWindowSize` (~3 min)
- tmux.go:1779-1806: replace the initial `if t.ptmx == nil` check and all 3 subsequent uses of `t.ptmx` (`.Fd()`, the `/dev/fd/%d` stat, and `pty.Setsize(t.ptmx, ...)`) with a single snapshot at the top:
  ```go
  func (t *TmuxSession) updateWindowSize(cols, rows int) error {
      file := t.lockedPTMX()
      if file == nil {
          return fmt.Errorf("PTY is not initialized")
      }
      fd := int(file.Fd())
      if fd < 0 {
          return fmt.Errorf("PTY file descriptor is invalid")
      }
      if _, err := os.Stat(fmt.Sprintf("/dev/fd/%d", fd)); err != nil {
          return fmt.Errorf("PTY file descriptor is closed or invalid: %v", err)
      }
      return pty.Setsize(file, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols), X: 0, Y: 0})
  }
  ```
- This snapshots once instead of re-reading the racy field 3 times across the function body — strictly safer than before, same observable return values/errors.
- Files: `session/tmux/tmux.go`

##### Task 1.2.1d: Convert `Attach()` goroutine 1 (`io.Copy`) — snapshot once (~2 min)
- tmux.go:1482-1484, before the goroutine's `io.Copy` call, snapshot the file once and use the local for the goroutine's entire (blocking) lifetime — matching existing behavior where `io.Copy` takes one `io.Reader` value up front:
  ```go
  ptmxForCopy := t.lockedPTMX()
  go func() {
      defer t.wg.Done()
      _, _ = io.Copy(os.Stdout, ptmxForCopy)
      ...
  }()
  ```
- Do NOT re-fetch inside the goroutine or loop — `io.Copy` blocks on one reader for its whole call, exactly like the pre-fix code implicitly did via the closure.
- Files: `session/tmux/tmux.go`

##### Task 1.2.1e: Convert `Attach()` goroutine 2 (stdin-forward loop) — re-snapshot every iteration (~3 min)
- tmux.go:1503-1546: inside the `for` loop, replace the direct write at line 1544:
  ```go
  _, _ = t.ptmx.Write(buf[:nr])
  ```
  with:
  ```go
  if file := t.lockedPTMX(); file != nil {
      _, _ = file.Write(buf[:nr])
  }
  ```
- **This must re-acquire the lock and re-snapshot on every loop iteration, not hoist the snapshot outside the loop.** This is the critical nuance from build-vs-buy.md/pitfalls.md: the pre-fix code already re-read `t.ptmx` fresh on every iteration (not hoisted), so a mid-session `Restore()` swap transparently redirects this goroutine to the new PTY. Hoisting the snapshot outside the loop (as in Task 1.2.1d) would silently change this behavior and violate AC6/non-goals. Added nil-check (`file != nil`) is defensive only — pre-fix code had no nil check either (relied on `os.File` nil-receiver safety), so this is at most a no-op-instead-of-relying-on-stdlib-nil-handling, not an observable change.
- Files: `session/tmux/tmux.go`

#### Story 1.2.2: Deterministic regression test and full verification pass
**As a** maintainer, **I want** a test that deterministically forces the original racing interleave (not just a probabilistic `-count=N` stress run), plus an explicit final run of every acceptance-criteria command, **so that** the fix's correctness is proven rather than merely likely.
**Acceptance Criteria**:
- AC2 (requirements.md #2): `go test -race ./session/... ./server/... -count=10` passes.
  - *Given* the fully converted `session/tmux/tmux.go`, *When* running `go test -race ./session/... ./server/... -count=10` from the repo root, *Then* the command exits 0 and no `"DATA RACE"` string appears in its output.
- AC3 (requirements.md #3): the originally-flaky test passes cleanly under repetition.
  - *Given* the fix applied, *When* running `go test -race ./server/... -run TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort -count=20`, *Then* all 20 runs pass with no race reported.
- AC4 (requirements.md #4): no new deadlocks; existing `session/tmux` suite passes.
  - *Given* the fix applied, *When* running `go test -race ./session/tmux/...`, *Then* it passes with no `deadlock.Mutex` timeout/`"POTENTIAL DEADLOCK"` output and no test failures.
- AC5 (requirements.md #5): `make quick-check` passes.
  - *Given* the fix applied, *When* running `make quick-check`, *Then* build, test, and lint all succeed with no regressions attributable to this change.
- AC2/AC3 intent (deterministic proof, not just probabilistic): the concrete scenario from requirements.md's own framing.
  - *Given* a `TmuxSession` constructed with a real `*os.File` pair from `os.Pipe()` assigned directly to `t.ptmx` (same-package test, no exported seam needed) and `t.attachCmd` left nil, *When* the test goroutine holds `t.ptmxMu.Lock()`, spawns one goroutine calling `t.GetPTY()` and another calling `t.closePTYAndAttachCmd()`, then unlocks, *Then* both goroutines complete (via a `select`+`time.After` deadlock guard matching the existing `TestDoesSessionExist_LockReleasedBeforeRecovery`/`TestRecoverFromServerFailure_ConcurrentGuard` idiom in this file) and `GetPTY()`'s result is exactly one of: the valid `*os.File`, or the `"PTY not initialized..."` error — asserted via `require.True(t, err == nil || strings.Contains(err.Error(), "not initialized"))` — never a panic, a corrupted pointer, or a hang.
**Files**: `session/tmux/tmux_test.go`

##### Task 1.2.2a: Add `TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized` (~5 min)
- In `session/tmux/tmux_test.go` (package `tmux`, so unexported fields/helpers are directly accessible — no production test-only seam needed), add:
  ```go
  func TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized(t *testing.T) {
      ptyFactory := NewMockPtyFactory(t)
      cmdExec := MockCmdExec{
          RunFunc:            func(cmd *exec.Cmd) error { return nil },
          OutputFunc:         func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
          CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
      }
      session := newTmuxSession("ptmx-race-test", "echo", ptyFactory, cmdExec, TmuxPrefix)

      r, w, err := os.Pipe()
      require.NoError(t, err)
      t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
      session.ptmx = r // direct field assignment: pre-test setup, not concurrent with anything yet

      // Force both goroutines to contend on ptmxMu at (as close as the scheduler allows to)
      // the same instant: hold the lock before spawning either, release only after both are
      // launched, then use a deadlock-guard select (matches TestDoesSessionExist_LockReleasedBeforeRecovery
      // and TestRecoverFromServerFailure_ConcurrentGuard's existing idiom in this file).
      session.ptmxMu.Lock()
      var wg sync.WaitGroup
      wg.Add(2)
      var getErr error
      go func() {
          defer wg.Done()
          _, getErr = session.GetPTY()
      }()
      go func() {
          defer wg.Done()
          session.closePTYAndAttachCmd()
      }()
      session.ptmxMu.Unlock()

      completed := make(chan struct{})
      go func() {
          wg.Wait()
          close(completed)
      }()
      select {
      case <-completed:
      case <-time.After(2 * time.Second):
          t.Fatal("GetPTY/closePTYAndAttachCmd deadlocked under concurrent access — ptmxMu not released correctly")
      }

      if getErr != nil {
          require.Contains(t, getErr.Error(), "not initialized",
              "GetPTY's only valid error outcome when racing closePTYAndAttachCmd is the not-initialized error")
      }
  }
  ```
- Run with `go test -race -run TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized -count=50 ./session/tmux/` to confirm both possible interleavings (GetPTY-wins vs closePTYAndAttachCmd-wins) are exercised across repeated runs and neither reports a race.
- Justification for not using `testing/synctest` (available and stable in this repo's Go 1.26.3): `synctest` exists to give deterministic control over *virtual time* for goroutines that sleep/wait on timers — this test has no such goroutines to control; the barrier here is a real `sync.Mutex` held by the test itself, which is already a fully deterministic rendezvous point (both spawned goroutines are guaranteed blocked on `ptmxMu.Lock()` before the test's `Unlock()` runs, since neither can pass that line without the lock). Pulling in `synctest`'s bubble/goroutine-tracking machinery for a single-mutex rendezvous would add ceremony with no additional determinism gained — the simpler channel/`select`-with-timeout idiom already used throughout this file (`TestDoesSessionExist_LockReleasedBeforeRecovery`, `TestRecoverFromServerFailure_ConcurrentGuard`) is preferred for consistency.
- Files: `session/tmux/tmux_test.go`

##### Task 1.2.2b: Run full acceptance-criteria verification suite (~5 min, no new code)
- Run each of the following from the repo root and capture pass/fail output — this task is the explicit "actually run it" step; nothing about the fix's correctness is assumed done until all four commands are shown green:
  1. `go test -race ./session/... ./server/... -count=10`
  2. `go test -race ./server/... -run TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort -count=20`
  3. `go test -race ./session/tmux/...`
  4. `make quick-check`
- If any command fails, do not mark this story or the PR complete — return to the relevant Epic 1.1/1.2 task, fix, and re-run all four commands (not just the one that failed, since a fix to one site can affect lock ordering elsewhere).
- Files: none (verification only — no source changes in this task).

---
