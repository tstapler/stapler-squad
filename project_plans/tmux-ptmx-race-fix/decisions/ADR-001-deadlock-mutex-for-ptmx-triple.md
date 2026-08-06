# ADR-001: `deadlock.Mutex` Over `atomic.Pointer` for the ptmx/attachCmd/attachCmdWaitOnce Triple

**Status**: Accepted
**Date**: 2026-08-06
**Project**: tmux-ptmx-race-fix

## Context

`go test -race` confirmed a real data race on `TmuxSession.ptmx` (`session/tmux/tmux.go:61`): `GetPTY()` (read, called from `SessionService.CreateSession`'s async controller-start goroutine) races `closePTYAndAttachCmd()` (write, `t.ptmx = nil`, called from `SessionService.DeleteSession`'s cleanup goroutine via `Instance.Destroy()`). `TmuxSession.attachCmd` and `TmuxSession.attachCmdWaitOnce` are written at the exact same call sites as `ptmx` (tmux.go:864-866, 1265-1268, 1689-1717) and must change together — they, together with `ptmx`, form one attach "generation," so guarding `ptmx` alone would just relocate the same race one field over.

`session/tmux/tmux.go` already carries 5 existing locks (`detachMutex`, `controlModeSubMu`, `controlModeStartMu`, `cmdSendMu`, `recoveryMu`), all `deadlock.Mutex` (`github.com/linkdata/deadlock`, go.mod:23) — a drop-in `sync.Mutex` wrapper that detects lock-order violations and stuck-lock timeouts at runtime, used 30 times in this file/package vs. 2 plain `sync.Mutex`. This repo also has a standing architectural preference, ADR-011 ("Prefer Lock-Free Concurrency Techniques"), for atomics/lock-free structures over mutexes where they fit.

## Decision

Guard the ptmx/attachCmd/attachCmdWaitOnce triple with one new `deadlock.Mutex` field, `ptmxMu`, accessed exclusively through three unexported helper methods (`lockedPTMX`, `setPTYTriple`, `clearPTYTriple` — see `project_plans/tmux-ptmx-race-fix/implementation/plan.md`'s Domain Glossary and Story 1.1.1). `ptmxMu` is documented as a leaf lock, never held while acquiring any of the file's other 5 locks.

## Alternatives Considered

- **Per-field `atomic.Pointer[os.File]` / `atomic.Pointer[exec.Cmd]` / `atomic.Pointer[sync.Once]`.** This is the option ADR-011 would normally point toward first. Rejected because the three fields are not independent — they describe one attach generation and must be swapped as a unit. Three separate atomics can each be updated correctly in isolation but give no cross-field atomicity: a reader could load `ptmx` from generation N+1 (already reassigned by a concurrent `AttachToExisting`/`Restore`) and `attachCmd` from generation N (not yet reassigned), a "torn triple." ADR-011's own guideline table already carves out this exact case: *"Complex State Transitions → Use `sync.Mutex` or Channels; lock-free logic for complex state machines is high-risk."* This decision is consistent with, not a departure from, ADR-011 — it applies ADR-011's own stated exception rather than contradicting its general preference for lock-free techniques.
- **Reuse `detachMutex`.** Rejected: `DetachSafely()` holds `detachMutex` for its whole body and calls `Restore()` → `RestoreWithWorkDir()`, which is itself one of the three PTY-triple write sites. `deadlock.Mutex` (like `sync.Mutex`) is not reentrant, so reusing `detachMutex` here would self-deadlock the very first time `DetachSafely` triggers a restore.
- **Reuse `controlModeStartMu` (or `controlModeSubMu`/`cmdSendMu`/`recoveryMu`).** Rejected: these guard the control-mode subprocess/pipe subsystem, which never reads or writes `ptmx`/`attachCmd`/`attachCmdWaitOnce`. Reusing an unrelated lock would add contention between two unrelated subsystems and obscure the actual invariant being protected.
- **A wrapping `ptyState{file, cmd, waitOnce}` struct as the fields' storage**, replacing the three separate fields. Not a locking-primitive alternative but a storage-shape alternative considered alongside this decision — rejected for this bugfix's scope; see the Pattern Decisions table in `plan.md` (row: "PTY triple storage shape") for the full reasoning (diff size and loss of per-field doc comments outweigh the marginal benefit, since the new helper methods already enforce "touch all three together" without a rename).

## Consequences

- `session/tmux/tmux.go` gains a 6th lock. Documented as a leaf lock in the field's doc comment (Task 1.1.1a of the implementation plan) so future changes don't introduce a cycle with the existing 5.
- The PTY triple's read/write surface is reduced to exactly 3 helper methods plus `GetPTY()` (built on `lockedPTMX`); no call site is expected to hand-roll `ptmxMu.Lock()`/`Unlock()`.
- `closePTYAndAttachCmd()` now runs its blocking cleanup (`Close()`/`Kill()`/`Wait()`) on locally-captured values outside the lock (snapshot-then-release-then-I/O), so `ptmxMu` is never held across blocking I/O — the hot read path (`GetPTY`, `SendKeys`, resize) is never stalled by a slow teardown.
- Does not change or weaken ADR-011: this is an explicit application of its documented "complex state transition" exception, not a precedent for reaching for mutexes generally in this codebase.
