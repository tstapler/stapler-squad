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

**Known limitation, deliberately not fixed here (pre-mortem finding #1)**: guarding each individual read/write of the PTY triple makes every *single* field access memory-safe, but `AttachToExisting` (Task 1.1.2a) and `RestoreWithWorkDir` (Task 1.1.2b) each still do an unguarded check-then-act (`if lockedPTMX() == nil { ...install... }`) across two independent lock/unlock cycles — a TOCTOU, not a data race, so `go test -race` cannot catch it. Confirmed real via `session/instance_tmux.go:87-94` (the `pmMu` there only guards lazy-init of the process-manager reference, not calls through it) and `TmuxSession.Close()` never taking `detachMutex` — so a concurrent `AttachToExisting()`/`RestoreWithWorkDir()` and `Close()` on the same session can genuinely interleave. Fixing it correctly needs a generation-counter compare-and-swap (release the lock around the blocking `ptyFactory.Start()` call, then only install if no concurrent winner appeared, discarding the loser's PTY/process) — a real behavioral change, which conflicts with this item's own non-goal of "no behavior change to PTY lifecycle semantics" and would expand the diff well beyond a pure concurrency-safety fix. Filed as backlog item `0f4b1300-d667-437b-b51f-89d81a668693` instead of leaving it implicit; Tasks 1.1.2a/1.1.2b add a short inline comment pointing at it.

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
| PTY triple synchronization primitive | `deadlock.Mutex`, new field, not reused | research/pitfalls.md | Reuse `detachMutex` or `controlModeStartMu` | `detachMutex`: `DetachSafely`/`Detach` hold it across calls to `closePTYAndAttachCmd()` and (in `Detach`) `Restore()` → `RestoreWithWorkDir()` — three of the PTY-triple write sites — so reusing it as the triple's own lock would self-deadlock the first time those call `ptmxMu.Lock()` reentrantly (non-reentrant lock). Keeping `ptmxMu` separate makes this a safe, consistently-ordered nesting (`detachMutex` outer → `ptmxMu` inner) instead. `controlModeStartMu`: orthogonal subsystem, never touches `t.ptmx` — reuse would add false contention and an unjustified coupling. |
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
  // Leaf lock: never acquire ptmxMu while already holding controlModeSubMu,
  // controlModeStartMu, cmdSendMu, or recoveryMu. detachMutex IS held across ptmxMu
  // critical sections in both Detach() and DetachSafely() (both call
  // closePTYAndAttachCmd(), and Detach() also calls Restore() → RestoreWithWorkDir(),
  // while still holding detachMutex) — that nesting is fine because the order is
  // always detachMutex (outer) → ptmxMu (inner/leaf) and never reversed; ptmxMu's own
  // critical sections never call back into detachMutex.
  ptmxMu deadlock.Mutex
  ```
- Cross-package callers (pre-mortem finding #4): `GetPTY()` is also reached externally via
  `TmuxProcessManager.GetPTY()` → `TmuxBackend.GetPTY()` → `Instance.GetPTYReader()`
  (`session/instance_tmux.go:438-443`). Verified `Instance.Destroy()` (which reaches
  `TmuxSession.Close()`) and `Instance.Start()`'s async flow (which reaches
  `RestoreWithWorkDir()`) do not hold `Instance.mu` while making those calls — `Instance.pmMu`
  (`session/instance_tmux.go:87-94`) only guards lazy-initialization of the process-manager
  reference itself, not calls made through it. Since `ptmxMu`'s own critical sections never
  call back into any `Instance`-level lock (`session/tmux` has no reference to `session`'s
  `Instance` type), no reversed-order deadlock is structurally possible regardless of what
  `Instance`/backend-layer locking does around its calls into these methods — the analysis in
  this doc comment only needs to cover locks declared inside `tmux.go` because a two-lock cycle
  requires both locks to appear in both call orders, and `ptmxMu` never appears in the
  `Instance`-holds-lock-then-calls-in direction from the other side.
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
  // Check-then-act: this is not atomic with respect to a concurrent AttachToExisting()
  // or RestoreWithWorkDir() call on the same session — each individual field access is
  // now memory-safe, but two concurrent callers can still both observe nil and both
  // install a PTY (the second setPTYTriple silently wins). Known limitation, not a
  // data race (go test -race cannot catch it); tracked separately as backlog item
  // 0f4b1300-d667-437b-b51f-89d81a668693.
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
  with (same known check-then-act limitation as Task 1.1.2a — tracked as backlog item
  `0f4b1300-d667-437b-b51f-89d81a668693`, not fixed here):
  ```go
  // Check-then-act: see AttachToExisting's identical comment above (backlog item
  // 0f4b1300-d667-437b-b51f-89d81a668693) — not atomic against a concurrent
  // AttachToExisting()/RestoreWithWorkDir()/Close() call on the same session.
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
- **Pre-existing limitation, not newly introduced (pre-mortem finding #5)**: this goroutine
  pins to whichever PTY generation was live at `Attach()`-call time for its whole (blocking)
  lifetime — unchanged from the pre-fix code, where the argument to `io.Copy` was already a
  one-time read. If a `Restore()`/second `AttachToExisting()` installs a new generation while
  this goroutine is still blocked reading the old one, the old fd is never closed by anything
  and the goroutine leaks. Out of scope here (same non-goal as pre-mortem finding #1: no PTY
  lifecycle behavior change) — noted explicitly rather than left implicit so it isn't mistaken
  for something this fix was supposed to close.
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
- AC6 (requirements.md #6, test-coverage clause): the accepted serialization side effect is covered by a dedicated test.
  - *Given* `TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized` (Task 1.2.2a), *When* it runs, *Then* it exercises exactly the accepted side effect (concurrent `closePTYAndAttachCmd` callers serializing on `ptmxMu`) and asserts the documented-intentional outcome, not a regression.
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

### Epic 1.3: CI enforcement and the direct repro test

**Goal**: Added after the backlog item's acceptance criteria were refined post-plan — this
epic closes the two gaps between the original plan (Epics 1.1/1.2, which satisfy AC2, AC4,
AC5, AC6, and AC1's *mechanism*) and the current AC text: AC1's requirement for CI
*enforcement* (not just correct code), and AC3's requirement for a second, purpose-built
regression test that races `CreateSession`/`DeleteSession` directly rather than relying on
`TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`'s incidental exposure of
the same interleave.

#### Story 1.3.1: `make ptmx-field-guard` CI target
**As a** maintainer, **I want** a CI target that fails the build if a future change reads or
writes `t.ptmx`/`t.attachCmd`/`t.attachCmdWaitOnce` outside the three helper methods, **so
that** the invariant Epic 1.1/1.2 establish can't silently erode later.
**Acceptance Criteria**:
- AC1 (backlog item #1): *Given* the guard target, *When* run against the post-fix
  `session/tmux/tmux.go` (only `lockedPTMX`/`setPTYTriple`/`clearPTYTriple` touch the fields
  directly), *Then* it exits 0. *Given* a hypothetical future diff that adds a fourth direct
  `t.ptmx = ...` call site outside those three methods, *When* the guard runs, *Then* it exits
  1 with a message naming the violation.
**Files**: `Makefile`, `session/tmux/tmux.go`

##### Task 1.3.1a: Mark the 3 helper methods' direct field-access lines (~2 min)
- Precedent: `actor-field-guard` (Makefile:758) is a pure grep over caller files — it works
  there because the guarded field writes are supposed to live in *different* files than the
  scan targets. Here the 3 permitted call sites live in the *same* file being scanned, so file
  exclusion doesn't discriminate; instead, mark each direct-access line inside `lockedPTMX`,
  `setPTYTriple`, and `clearPTYTriple` (added in Task 1.1.1b) with a trailing
  `// allow-direct-ptmx-access` comment, e.g.:
  ```go
  func (t *TmuxSession) lockedPTMX() *os.File {
      t.ptmxMu.Lock()
      defer t.ptmxMu.Unlock()
      return t.ptmx // allow-direct-ptmx-access
  }
  ```
  Apply the same trailing marker to every `t.ptmx`/`t.attachCmd`/`t.attachCmdWaitOnce`
  reference inside all three helpers' bodies (both reads and writes).
- Files: `session/tmux/tmux.go`

##### Task 1.3.1b: Add the `ptmx-field-guard` Makefile target (~3 min)
- Add to `Makefile`, following the `actor-field-guard` target's shape (grep-based, comment
  lines excluded, marker-annotated lines excluded):
  ```makefile
  ptmx-field-guard: ## tmux-ptmx-race-fix guard: fail if ptmx/attachCmd/attachCmdWaitOnce are touched outside the ptmxMu helpers
  	@echo "ptmx-field-guard: scanning session/tmux/*.go for direct PTY-triple field access..."
  	@if grep -nE '\bt\.(ptmx|attachCmd|attachCmdWaitOnce)\b' session/tmux/*.go \
  	    | grep -v '^session/tmux/shell_handle.go:' \
  	    | grep -vE ':[0-9]+:[[:space:]]*//' \
  	    | grep -v 'allow-direct-ptmx-access' ; then \
  	    echo "❌ ptmx-field-guard: direct PTY-triple field access found outside lockedPTMX/setPTYTriple/clearPTYTriple — route through the ptmxMu helpers (session/tmux/tmux.go)"; \
  	    exit 1; \
  	fi
  	@echo "✅ ptmx-field-guard: no direct PTY-triple field access outside the guarded helpers"
  ```
  Kept as a `session/tmux/*.go` package-wide scan per requirements.md AC1's literal wording, but
  with `shell_handle.go` explicitly excluded by path (pre-mortem finding #2): that file declares
  an unrelated `ShellTmuxHandle` struct with its own, differently-guarded `ptmx`/`attachCmd`
  fields under a receiver named `h`, guarded by its own `spawnMu`, not `ptmxMu` — without the
  exclusion, a future unrelated edit to that file (e.g. a receiver rename to `t` for style
  consistency) would fail this guard for a mutex it doesn't even use. Any *other* file added to
  the package later is still scanned. The residual gap this doesn't close — a real
  `TmuxSession` violation written with a receiver name other than `t` (e.g. copy-pasted test
  setup like Task 1.2.2a's own `session.ptmx = r` line, which uses a `session` receiver and is
  a legitimate pre-test direct assignment, not a violation, but illustrates the pattern the
  guard can't see) — is accepted for this bugfix's scope, matching `actor-field-guard`'s
  equally receiver-name-coupled precedent; worth revisiting with an `ast-grep`/gritql rule
  scoped to methods on `*TmuxSession` if this class of guard becomes more common in the repo.
- Add `ptmx-field-guard` to the `.PHONY` line and to the `ci:` target's prerequisite list
  (alongside `actor-field-guard`, the direct precedent for a field-discipline guard gating CI).
  Not added to `quick-check` — matches `actor-field-guard`'s placement (CI-only, not part of
  the fast local loop), and AC5 only requires `quick-check` to keep passing, not to grow a new
  prerequisite.
- Verify: `make ptmx-field-guard` exits 0 on the fixed tree; temporarily add a throwaway
  `t.ptmx = nil` outside the helpers and confirm the target exits 1, then revert the throwaway
  line.
- Files: `Makefile`

#### Story 1.3.2: Direct `CreateSession`/`DeleteSession` race repro test
**As a** maintainer, **I want** a test that races `CreateSession` and `DeleteSession` without
waiting for the instance to go live, **so that** the exact interleave from the original
`-race` report (async controller-start goroutine's `GetPTY()` vs. the delete cleanup
goroutine's `closePTYAndAttachCmd()`) is exercised directly and deterministically by CI, not
just incidentally by a hook-URL test that happens to share the same timing window.
**Acceptance Criteria**:
- AC3 (backlog item #3): *Given* `TestSessionService_CreateThenImmediateDelete_NoDataRace`,
  *When* run under `go test -race ./server/...`, *Then* it passes with no data race reported,
  and it does not call `waitForLiveInstance` before issuing `DeleteSession` (that wait is
  exactly what makes the existing hook-URL test's exposure of this race incidental/rare rather
  than reliable).
**Files**: `server/server_integration_test.go`

##### Task 1.3.2a: Add `TestSessionService_CreateThenImmediateDelete_NoDataRace` (~8 min)
- **Best-effort, not deterministic (pre-mortem finding #3)**: unlike Task 1.2.2a's unit test
  (which forces the exact interleave by holding `ptmxMu` before spawning either goroutine),
  this test relies on real scheduler timing to land `DeleteSession`'s cleanup goroutine inside
  `CreateSession`'s async controller-start window. A production test-hook that blocks the real
  controller-start goroutine mid-`GetPTY()` would make this deterministic too, but was rejected
  as disproportionate blast-radius for this PR (it would add a test-only synchronization seam
  to `server/services/session_service.go`'s async goroutine, outside `session/tmux`).
- **Course-correction found during implementation**: the first attempt mitigated the
  non-determinism by racing 8 concurrent create/delete pairs within one test invocation.
  `go test -race -count=20` showed this introduces its own, unrelated flake — "database is
  locked" from the session store's SQLite connection, whose 5s busy_timeout
  (`session/ent_repository.go`'s `_timeout=5000`) 8-way concurrent writes can exceed once
  `-race`'s slowdown is factored in. Reverted to a single sequential create/delete pair per
  invocation; probabilistic coverage comes from `-count=20`'s outer repetition (Task 1.3.2b),
  not in-test concurrency — verified clean across 20 consecutive `-race` runs with zero data
  races and zero DB-lock failures. This test's role is a realistic, close-to-production repro;
  the deterministic correctness proof is Task 1.2.2a.
- In `server/server_integration_test.go`, alongside
  `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`, add:
  ```go
  // TestSessionService_CreateThenImmediateDelete_NoDataRace deliberately does NOT call
  // waitForLiveInstance before deleting: CreateSession's async controller-start goroutine
  // (which calls GetPTY()) is very likely still in flight when DeleteSession's cleanup
  // goroutine runs closePTYAndAttachCmd(). This is the exact interleave from the original
  // -race report. This test is a realistic, best-effort repro under real scheduler timing
  // (relying on -count=N outer repetition, not in-test concurrency, to raise the odds of
  // landing in the window across runs -- an earlier version raced several concurrent
  // create/delete pairs within one run, but that tripped the session store's SQLite
  // busy_timeout under -race's slowdown, an unrelated flake this test must not introduce).
  // The deterministic proof that the fix is correct is
  // TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized in session/tmux/tmux_test.go,
  // which forces the interleave directly instead of relying on timing.
  func TestSessionService_CreateThenImmediateDelete_NoDataRace(t *testing.T) {
      installFakeClaudeBinary(t)
      deps, err := BuildDependencies()
      if err != nil {
          t.Fatalf("BuildDependencies: %v", err)
      }

      title := fmt.Sprintf("ptmx-race-repro-%d", time.Now().UnixNano())
      resp, err := deps.SessionService.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
          Title:   title,
          Path:    t.TempDir(),
          Program: "claude",
      }))
      if err != nil {
          t.Fatalf("CreateSession: %v", err)
      }

      if _, err := deps.SessionService.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: resp.Msg.Session.Id})); err != nil {
          t.Fatalf("DeleteSession: %v", err)
      }
  }
  ```
- No new imports needed — `fmt`, `context`, `time`, `connect`, `sessionv1` are already
  imported by this file for the neighboring test.
- Files: `server/server_integration_test.go`

##### Task 1.3.2b: Re-run the full verification suite including the new pieces (~5 min, no new code)
- Re-run Task 1.2.2b's four commands, plus:
  5. `make ptmx-field-guard`
  6. `go test -race ./server/... -run TestSessionService_CreateThenImmediateDelete_NoDataRace -count=20`
- All six must be green before requesting review.
- Files: none (verification only).

---
