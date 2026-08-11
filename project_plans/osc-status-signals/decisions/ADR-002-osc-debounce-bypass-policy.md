# ADR-002: OSC Transitions Get a Dedicated, Shorter Debounce Window — Not Literal Zero-Debounce

**Status**: Accepted
**Date**: 2026-08-06
**Project**: osc-status-signals

## Context

Acceptance criterion 6 asks that "OSC-derived status transitions are not subject to the same
debounce/stabilization delay that applies to text-based transitions ... or an explicit, documented
reason is given if full debounce-bypass is infeasible."

`session/detection/idle.go`'s `IdleDetector` gates every state transition on a single mechanism:

```go
newState := id.mapStatusToIdleState(status)
if newState != id.currentState {
    if id.currentState == IdleStateUnknown || id.timeNow().Sub(id.lastStateChange) >= id.config.DebounceDelay {
        id.currentState = newState
        id.lastStateChange = id.timeNow()
    }
}
```

`DebounceDelay` defaults to 500ms and was itself already tuned once (reduced from 2s, per its own
comment — "for faster response"), i.e. this is a knob this codebase has adjusted before and treats
as sensitive.

`research/pitfalls.md` §5 documents that this codebase has a **repeated, named incident history**
of exactly one failure shape: a second, uncoordinated transition path writing to the same shared
state as an existing debounced path (`project_plans/backlog-session-thrashing`, `BUG-043`,
`BUG-048`). Applied here: literally skipping `DebounceDelay` for every OSC-derived transition would
let a genuinely rapid spinner↔✳ toggle — plausible between two back-to-back tool calls in the same
turn, per `research/ux.md`'s note that the spinner glyph itself redraws every ~80-100ms — flap
`IdleDetector.currentState` at a rate the 500ms window exists specifically to prevent, reproducing
the flapping class rather than avoiding it.

## Decision

Add a second, independent debounce window, `IdleDetectorConfig.OSCDebounceDelay` (default 150ms —
roughly the same order of magnitude as a spinner redraw interval, so a single stray redraw cannot
both flip the OSC classification *and* survive the window, while still committing an OSC-confirmed
transition well over 3x faster than the 500ms text-pattern path). A new method,
`IdleDetector.ApplyOSCStatus(osc dtypes.OSCStatus) IdleState`, applies this window using the exact
same gate shape as the existing one, but **reuses the same `id.lastStateChange` timestamp field**
rather than introducing a second, separately-tracked timestamp:

```go
if newState != id.currentState {
    if id.currentState == IdleStateUnknown || id.timeNow().Sub(id.lastStateChange) >= id.config.OSCDebounceDelay {
        id.currentState = newState
        id.lastStateChange = id.timeNow()
    }
}
```

Both the text-pattern path (`DetectStateFromContent`) and the OSC path (`ApplyOSCStatus`) read and
write the *same* `id.currentState`/`id.lastStateChange` fields under the *same* `id.mu` lock — the
only thing that differs between them is which config duration is compared against the elapsed time.

## Consequences

- **Directly answers pitfalls.md's core warning by construction, not by convention.** There is only
  ever one "last state change" clock; a reviewer auditing this code cannot introduce a second,
  disagreeing clock by accident the way two genuinely independent timestamp fields could drift.
- **Not literal zero-debounce.** This is the documented reason AC6 explicitly allows for: full bypass
  was evaluated and rejected as unsafe given this codebase's specific incident history: a coalescing
  window is kept, just a much shorter, independently-tuned one.
- Because `OSCStatus` only has two non-`None` values (`Executing`, `Idle`), repeated observations of
  the *same* class within the window never re-trigger the outer `newState != id.currentState` check
  at all — the 150ms window only ever gates genuine class-to-class toggles, not per-frame spinner
  animation noise. This means the practical cost of the window is close to zero for the common case
  (a sustained spinner, or a sustained ✳) and only applies smoothing to the actual noisy case
  (rapid toggling).
- `OSCDebounceDelay`, like `DebounceDelay` before it, is a tunable default subject to revision if
  real-world false-flap reports surface — not treated as load-bearing precision.
