# ADR-001: A bespoke `session/lifecycle.Reason` closed enum, not `qmuntal/stateless` or an FSM library

**Date**: 2026-09-06
**Status**: Accepted
**Project**: `project_plans/session-lifecycle-state-machine/`

## Context

Three production incidents in `session/tymux/stream.go`'s `readAttachLoop` (`477eacd6a`,
2026-08-23; two more this session, 2026-09-06) were each caused by extending the same
multi-clause boolean condition (`s.closing.Load() || exited || ctx.Err() != nil`) by one more
`||` clause, rather than replacing the shape. `requirements.md` set a Large appetite explicitly
to fix the *class*, not patch a 4th time, and named `qmuntal/stateless` directly as a candidate
to evaluate rather than defaulting to "no new dependencies."

`session/tmux/control_mode.go` was independently confirmed (by two research agents reading the
code directly) to carry the same class of smell in a different shape: the same
`!t.intentionalStop.Load()` guard, independently re-checked at 3 call sites
(`control_mode.go:491,656,668`) rather than one multi-clause condition at one site.
`session/external_streamer.go`'s `readLoop` was found to carry a related but distinct smell: a
timeout classifier that falls through to `strings.Contains(err.Error(), "i/o timeout")` when
its typed checks are known-incomplete (per that file's own "fallback for wrapped errors"
comment).

## Decision

Build a small, bespoke `session/lifecycle` package: a closed `type Reason int` (6 named
constants, `iota`-based, zero value `ReasonUnknown` safe-by-default), two exhaustively-switched
predicate methods (`ShouldContinue()`, `ShouldFireExitCallback()`), and supporting helpers
(`StartGeneration`/`EndGeneration`/`RecordEnd` for observability, `AwaitBounded` for the
bounded-wait-then-abandon idiom, `IsBenignTimeout` for the canonical typed timeout classifier).
Do **not** adopt `qmuntal/stateless`, `looplab/fsm`, or generalize
`session/instance_state.go`'s `TransitionDef`/`lookupTransition` table into a shared mechanism.

## Alternatives considered

1. **`qmuntal/stateless`** (evaluated concretely — `research/stack.md`, `research/build-vs-buy.md`,
   `research/pitfalls.md` §4): 1,383 stars, actively maintained (last release 2026-02-10, `v1.8.0`),
   zero transitive dependencies, BSD-2-Clause. Rejected on **fit**, not maturity or license: its
   `Fire(trigger, args...)`/`FireCtx` API models a *caller-triggered, named-event* transition —
   something external tells the machine an event happened, and it validates/executes a legal
   edge. `readAttachLoop`'s bug was never "an illegal transition was attempted" — it was "the
   classification of *why* `Receive()` failed was incomplete." Using `stateless` here would
   require `readAttachLoop` to read `closing`/`exited`/`ctx.Err()` itself (unavoidable — the
   library doesn't observe process state), map that combination to a trigger name, call
   `FireCtx`, then re-read the resulting state to decide what to do — reconstructing the exact
   classification the ad hoc `if` was already doing, just to hand it to the library as a string.
   That adapter is net new code with no corresponding risk reduction: the library never touches
   the part of the system that broke three times. Its own issue history (`#6`, `#61`, `#36`)
   additionally shows its default `FiringQueued` mode has surprising silent-delay semantics for
   an async, self-observed caller (exactly this project's shape), and that its own concurrent
   `Fire`/introspection paths needed 3+ rounds of race fixes (`PR #13`, `#66`, `#68`) — a spike
   would be required regardless of the fit question above, adding cost without addressing it.
2. **`looplab/fsm`**: same caller-triggered `Event(ctx, eventName, args...)` model, if anything
   further from this problem's shape (frequently used for order/ticket-style workflows).
   Rejected for the identical reason as (1); no further libraries were evaluated in depth per
   `requirements.md`'s own "brief comparison only" guidance.
3. **Generalize `session/instance_state.go`'s `TransitionDef`/`lookupTransition`**: a real,
   proven, zero-dependency in-repo pattern — but structurally the same `from→to` graph shape as
   (1)/(2), driven by explicit callers (`Approve()`, `Deny()`, `ForceStatus()`), not a goroutine
   inferring "why" from already-known local signals after a blocking call fails. Generalizing it
   would also risk conflating `session.Status`'s lifecycle with this project's — a distinct
   layer `requirements.md`'s Rabbit Holes section explicitly requires stay separate. Its two
   idioms worth keeping — "an unhandled edge is a typed, visible error, never a silent no-op"
   and the existing `.golangci.yml` `exhaustive` carve-out mechanics — are reused directly by
   this decision (see Consequences), without adopting its transition-graph machinery.

## Consequences

- **New code, no new dependency.** `session/lifecycle` is ~150-250 lines across
  `reason.go`/`observability.go`/`await.go`/`timeout.go` plus tests, owned and auditable by the
  team that owns the three incidents — smaller and more directly targeted than adapting an
  external library's API to a shape it wasn't built for.
- **Exhaustiveness is a repo-idiom, not a compiler guarantee** (Go has no sum types): enforced
  via the same `.golangci.yml` `exhaustive` linter mechanism already proven for
  `session/detection.DetectedStatus`, scoped narrowly to `session/lifecycle` only (see plan.md's
  Pattern Decisions) so unrelated `iota`+`default:` switches elsewhere in `session/tmux`/
  `session/tymux` are not accidentally re-enabled for exhaustiveness checking.
- **Generation-fencing (which incarnation of a goroutine is this) is deliberately left
  out of `Reason`'s scope.** Each subsystem's existing mechanism — tymux's per-generation
  `ctx`/`done` pair, `control_mode.go`'s `doneCh`-identity comparison (`control_mode.go:476-481`,
  already correct prior art) — is kept as-is. `Reason` answers "why did this generation end,"
  not "which generation is this" — folding both into one type was rejected explicitly (plan.md
  Pattern Decisions) as forcing a false uniformity onto two different concerns.
- **`session/actor.go` and `session/tmux/server_registry.go`'s `reconnectLoop` are explicitly
  not migrated.** Confirmed by direct reading (two independent research passes) to not share
  this smell — `runActor`'s real gap is a command-execution-SLA/preemption problem (a different
  mechanism), and `server_registry.go`'s reconnect loop already uses a single unambiguous
  `ctx.Done()` signal with nothing to misclassify. Documented in place (plan.md Phase 5) rather
  than silently skipped, per the Success Metrics requirement.
- **If a future subsystem's needs genuinely outgrow this shape** (multi-state graphs, guarded
  transitions, hierarchical states), that is new information this ADR did not have — revisit the
  library options above at that time rather than retrofitting `Reason` into something it isn't.
- **Follow-on**: two tracked issues from Epic 5.3, filed against `tstapler/stapler-squad`
  (2026-09-06), neither solved by this project:
  - [`tstapler/stapler-squad#715`](https://github.com/tstapler/stapler-squad/issues/715) —
    `session/actor.go`'s command-execution-SLA/preemption gap (the actual mechanism suspected
    behind the actor-wide wedge incidents; Task 5.3.1a).
  - [`tstapler/stapler-squad#716`](https://github.com/tstapler/stapler-squad/issues/716) —
    removing `STAPLER_SQUAD_TMUX_LIFECYCLE_V2` once its dashboarded, staged bake-then-delete
    gate (not a vague "N weeks, no incidents" criterion) is satisfied (Task 5.3.2a).
