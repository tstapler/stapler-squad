# ADR-002: Per-Instance Creation Epoch as a Fencing Token for Terminal Status Writes

**Status**: Accepted
**Date**: 2026-08-26

## Context

Once creation resolution is asynchronous, up to four independent actors can
attempt a terminal status write on the same `*session.Instance`: (1) the
background resolution goroutine (success or failure), (2) a user-triggered
cancel, (3) a user-triggered retry (which restarts resolution with a new
goroutine), and (4) the stale-creation sweeper. `session/actor.go`'s mailbox
(`sendCtx`/`sendSyncErr`) already serializes concurrent *writes* to a given
instance — two calls never corrupt shared state or data-race — but it does
not know that a write is *stale*. A cancel enqueued at T0 followed by a
resolution goroutine's stale success write enqueued at T0+ε both execute,
**in enqueue order**, and the late write silently wins (e.g. reviving a
cancelled session, or overwriting a fresh retry's `Creating` state with the
original attempt's late `Failed`).

`ForceStatus` (`session/instance_state.go:367`) is a bare, unguarded setter
by design (it's documented as an escape hatch for error-recovery paths where
the normal `transitionTo` validation would itself fail) — it has no concept
of "this caller's view of the world is stale."

## Decision

Add a `creationEpoch uint64` field to `session.Instance`, incremented exactly
twice: once when a cancel is issued, once when a retry starts. The field is
actor-owned (read/written only inside the mailbox, like every other
`instanceState` field) and exposed via a new read helper,
`Instance.CreationEpoch() uint64`.

Every actor that intends to make a **terminal** write (Active on success,
Failed on any resolution/startup error, Failed on stale-timeout) must go
through one new, atomic, actor-routed primitive instead of composing a
separate epoch-read followed by a separate `ForceStatus` call (composing two
mailbox round-trips reopens the exact race this ADR closes):

```go
// TryForceStatusIfEpoch performs a compound check-and-set entirely inside
// one enqueued actor command: if capturedEpoch no longer matches the
// instance's current creationEpoch, it is a no-op (the caller has been
// superseded by a cancel or retry) and returns false. Otherwise it applies
// the status/progress/failure-reason write and returns true.
func (i *Instance) TryForceStatusIfEpoch(capturedEpoch uint64, s Status, failureReason string) bool
```

The background resolution goroutine captures its epoch once, at spawn time,
before doing any work, and passes it to `TryForceStatusIfEpoch` at its one
terminal call site (success or failure). The stale-creation sweeper does the
same at its terminal call site. Cancel and retry are the only two writers of
`creationEpoch` itself — both go through the same actor mailbox, so
incrementing it and observing/using it for gating are both linearized by the
same infrastructure already in place, no new lock required.

`FailureReason` is terminal-write metadata (meaningful only when
`Status == Failed`), not progress text — it is therefore written *only*
inside `TryForceStatusIfEpoch`'s own command closure, via an unexported
`setFailureReasonLocked` helper with no other caller. There is no
independent, public `SetFailureReason` entry point: one would let a
superseded or future caller set it outside the epoch gate, producing the
illegal state `Status == Active, FailureReason == "GitHubResolutionError"`
that this ADR's fencing guarantee exists to prevent.

**Retry must also be fenced against itself, not just against stale
terminal writes.** The epoch alone detects a *stale* epoch (mismatch); it
does not, by itself, stop two concurrent `RetrySessionCreation` calls from
each observing `Status == Failed`, each bumping the epoch (two separate
mailbox round-trips), and each spawning a live pipeline goroutine that
captures the same final epoch value once both bumps have landed — two live
writers sharing one *current* epoch, which `TryForceStatusIfEpoch` cannot
distinguish. To close this, "validate `Failed` → bump epoch → mark retry
started" is itself one atomic, actor-routed compound operation,
`TryStartRetry`, mirroring `TryForceStatusIfEpoch`'s own shape:

```go
// TryStartRetry performs a compound check-and-set entirely inside one
// enqueued actor command: if the instance is not currently Failed, it is a
// no-op and returns (0, false). Otherwise it bumps creationEpoch, resets
// status to Creating with fresh creation_progress, and returns the new
// epoch and true. Exactly one of two concurrent callers observes true.
func (i *Instance) TryStartRetry() (newEpoch uint64, started bool)
```

`RetrySessionCreation`'s handler calls `TryStartRetry` as its first
state-mutating step, before running `cleanupPartialCreation` or spawning
the new pipeline goroutine. A second, concurrent retry call on the same
instance therefore deterministically observes `started == false` (the
instance is no longer `Failed` by the time its own command executes in the
mailbox) and returns `FailedPrecondition` without ever reaching cleanup or
spawning a goroutine — never two live pipelines for one instance. The same
"bump-then-check" shape is used by `CancelSessionCreation` (Task 3.2.1c),
so both RPCs share one race-resolution idiom.

## Consequences

- Exactly one terminal write per creation attempt is guaranteed: only the
  writer whose captured epoch matches current can succeed, and only one
  epoch value is ever "current" at a time.
- Non-terminal writes (`SetCreationProgress` for in-flight phase text) are
  **not** epoch-gated — a stale goroutine's progress text updates are
  harmless UI noise at worst (overwritten by the next legitimate update) and
  gating every single call would add mailbox round-trips for no correctness
  benefit; only the terminal transition needs the guarantee.
- New required unit tests (called out directly in `research/architecture.md`
  §7): epoch-matches path writes through; epoch-mismatch path correctly
  no-ops; back-to-back cancel+retry doesn't leave the epoch permanently
  "stuck" (i.e., a legitimate subsequent write with the *new* current epoch
  still succeeds).
- `context.CancelFunc` for the background goroutine's own context is a
  **separate** mechanism (stored per-instance, see plan.md Story 2.2) — the
  epoch stops a stale *write*, the CancelFunc stops the stale *work*
  (subprocess/clone) from continuing to run at all. Both are needed; neither
  substitutes for the other (killing the goroutine's context doesn't
  retroactively invalidate a write it already enqueued moments earlier).
  Unlike `creationEpoch`, `CancelFunc` is **process-local and never
  persisted** — it cannot appear in the Migration Plan's ent-schema fields,
  because a `context.CancelFunc` cannot survive a process restart by
  definition. A `Creating` instance reloaded from storage in a new process
  therefore has a `nil` `CancelFunc`, which is the expected, common
  post-restart state, not an error: `CancelSessionCreation` (Task 3.2.1b)
  must nil-check it and, when `nil`, skip straight to
  `cleanupPartialCreation` + removal — there is no live goroutine in this
  process to interrupt, so only the state-machine transition is needed, not
  goroutine interruption.

## Alternatives Rejected

- **Rely on the actor mailbox's ordering alone**: rejected — ordering
  guarantees "no corruption," not "no stale write wins," which is the
  actual bug class here (see Context).
- **Instance-scoped `sync.Mutex` outside the actor**: rejected — the actor
  mailbox already *is* the instance's serialization point; adding a second,
  independent lock outside it would be two competing serialization
  mechanisms for the same data, and (per `.claude/rules/interface-pollution-checklist.md`'s
  sibling concern for over-engineering) unjustified when the actor can host
  the same guarantee as one more field plus one more command type.
- **Await the goroutine's `WaitGroup` entry before allowing cancel/retry to
  proceed** (a "wait, don't fence" design): rejected as the sole mechanism —
  a wedged clone subprocess can block for the goroutine's entire timeout
  window, so cancel/retry would hang waiting for it; the epoch makes
  cancel/retry return immediately while the stale goroutine's eventual
  wake-up becomes a guaranteed no-op instead of something callers must wait
  out.
