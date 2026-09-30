# Requirements: session-lifecycle-state-machine

**Date**: 2026-09-06
**Type**: cross-cutting architecture refactor
**Complexity**: 4 — cross-cutting change with Large appetite

## Problem Statement

Goroutine-lifecycle decisions in the `session` package — "is this reader goroutine done,
or should it keep going/reconnect?" — are repeatedly implemented as ad hoc, independently
evolving boolean flags checked at one decision point, rather than as an explicit, typed
state machine. This has now produced **three separate production incidents in a single
file** (`session/tymux/stream.go`'s `readAttachLoop`), each adding one more `||` clause to
the same condition, confirmed via git archaeology:

1. `477eacd6a` (2026-08-23) added `|| exited` — a clean pane exit's next `Receive()` was
   misclassified as a transport drop, triggering a needless `ReviveSession` call and a
   false `BackendRestarted()` report.
2. This session's fix #1 added `|| ctx.Err() != nil` — `teardownStandingStream`'s internal
   "tear down the old stream before opening a new one" call (from `RestoreWithWorkDir` /
   `cacheFromSession`) wasn't recognized as deliberate, so the reader reconnected instead
   of exiting, and `teardownStandingStream`'s `<-done` wait blocked forever. Because this
   whole call chain runs inside the owning `Instance`'s serialized actor
   (`session/actor.go`), one wedged goroutine wedged *every* future operation on that
   session, including its own destruction. Reproduced live (an 8-minute-stuck goroutine),
   root-caused, fixed, and regression-tested.
3. This session's fix #2, found while verifying #1 live: a reader's `Receive()` stayed
   blocked for minutes after cancellation even with fix #1 applied, against an otherwise
   idle, responsive `tymuxd` (suspected — not confirmed — missing HTTP/2
   `ReadIdleTimeout`/`PingTimeout` on the h2c client in `transport.go`'s `newH2CClient`).
   Mitigated with a bounded 5s timeout on the teardown wait (abandons the wedged goroutine
   with a logged warning rather than hanging forever) — a reliability patch, not a
   structural fix.

**The user's explicit assessment, which sets this project's scope: this pattern "litters
the session package more broadly" and is not isolated to `session/tymux`.** The classic
`session/tmux` backend has its own analogous reader-goroutine/reconnect/lock machinery
(`control_mode.go`'s command-response goroutine, its own `%exit`/reconnect handling,
`instance_actor_setters.go`'s actor-write races the `instance-lock-free-reads.md` rule
already documents a *related* but distinct class of), and `session/actor.go` implements
the generic single-goroutine "actor" pattern every backend's lifecycle commands are
serialized through. This project's job is to find every place this smell recurs across
the `session` package, and close the *class*, not just the one file it was first proven in
three times.

## Baseline

Today: goroutine-lifecycle "should I stop or continue" decisions are inferred after the
fact from independently-set, loosely-related fields (booleans, atomics, ad hoc `ctx.Err()`
checks) that nothing forces a new call site — or a new goroutine elsewhere in the package —
to consult correctly. There is no structural (compile-time or exhaustiveness) guarantee
that every legitimate "this generation/actor/session is ending" cause is classified
correctly anywhere in `session/`, and no shared building block for representing this shape
of state machine — every subsystem that needs one (so far: `session/tymux`'s stream reader,
and by the user's assessment likely others) invents its own ad hoc flags from scratch.
Diagnosing incident #2 above required a live `pprof` goroutine dump and manual stack-trace
reading rather than a trace/metric — there is also no standard observability pattern for
"a goroutine lifecycle just made a stop-vs-continue decision, here's why."

## Users / Consumers

- `session.TymuxBackend` / `tymuxGRPCSession` (`session/tymux/`) — where the pattern was
  first proven, 3 times.
- `session.TmuxBackend` / `TmuxProcessManager` (`session/tmux/`) — the classic backend;
  audit required to confirm/deny the same smell (see Scope).
- `session.Instance`'s actor (`session/actor.go`) — the generic serialization mechanism
  every backend's lifecycle commands run through; the wedging *mechanism* in incident #2
  (one stuck goroutine blocks the whole actor) is a property of this shared machinery, not
  `session/tymux`-specific, so its own state/contract is in scope for audit.
- Whichever shared building block this project produces becomes a dependency for future
  `session` package subsystems — this is now explicitly infrastructure work, not a one-off
  fix (per the user's Large appetite and "done right" framing).
- Broader direction: the user's stated goal is making the tymux backend viable as the
  long-term *default* session backend. This project removes a class of hang bug (not just
  one instance of it) that currently blocks that goal.

## Success Metrics

- The recurring "ad hoc boolean-OR classification of a goroutine-lifecycle decision" smell
  is closed as a **class**, not patched a 4th time: a shared, typed building block exists
  for "does this goroutine stop or continue, and why," and every subsystem in `session/`
  identified during the Phase 2 audit as having this smell is migrated to it (or has an
  explicit, reviewed reason it's out of scope — not silently skipped).
- Adding a future legitimate "this is ending deliberately" cause anywhere the building
  block is used must be a change that is either impossible to omit from the decision point
  or fails safe (defaults to "stop," never "loop/reconnect forever") if the author forgets
  to handle it — verified by a test that adds a new, unhandled cause and asserts safe
  failure, not just the 3 known incidents.
- Every migrated subsystem's goroutine-lifecycle transitions are observable (spans/metrics)
  via the pattern already established this session (`exec_gate_observability.go`,
  `control_mode_observability.go`) — a future incident anywhere the building block is used
  should be diagnosable from a trace, not require a live `pprof` dump.
- All 3 documented `session/tymux` incidents have a regression test that fails against the
  pre-fix shape of the code (2 already exist from this session's work — port/re-verify
  against the refactored mechanism). Any additional subsystem migrated in Phase 2's audit
  gets an equivalent regression test for whatever concrete bug or near-miss justified its
  inclusion.
- `session/tymux`'s existing 46-test suite and any `session/tmux` tests touched both stay
  green, including `-race`.

## Appetite

**Large (3–6 weeks)** — explicitly set by the user ("I have a large appetite for making
this work get done and done right"), superseding this document's own earlier inference of
Small. This licenses: a real build-vs-buy evaluation and likely adoption of a shared
mechanism (library or bespoke), a full-package audit rather than a single-file fix, and
migrating every subsystem the audit finds — not just documenting them as follow-up work.

## Constraints

- Go (no native sum/union types) — "type-level" enforcement here means idiomatic Go
  patterns (an unexported-method-gated closed type, a required constructor parameter,
  exhaustive `switch` with a compile-time-checked default, or a well-chosen library's own
  idiom), not literal sealed classes.
- Must not change externally-observed behavior for any currently-passing scenario in either
  backend: genuine transient transport/process drops must still reconnect transparently
  where that's the existing contract; genuine daemon/process restarts must still be
  detected and recovered the way they are today.
- Must not regress either backend's existing test suites (`session/tmux`, `session/tymux`),
  including `-race`.
- A new external dependency is explicitly acceptable if justified — the user named
  [`qmuntal/stateless`](https://github.com/qmuntal/stateless) directly; evaluate it
  concretely (see Alternatives) rather than defaulting to "no new dependencies."

## Non-functional Requirements

- **Performance SLO**: not specified — these are correctness/observability paths (a
  goroutine's error/lifecycle branch), not per-message hot paths. Any shared abstraction
  must not introduce per-message overhead on `session/tmux`'s or `session/tymux`'s actual
  I/O paths (capture-pane, send-keys, output streaming) — confine it to lifecycle/control
  decisions.
- **Scalability**: not applicable.
- **Security classification**: internal.
- **Data residency**: not applicable.

## Scope

### In Scope

1. **Audit** (Phase 2 research): find every place in `session/` where a goroutine's
   continue-vs-stop (or more generally, "which of N mutually exclusive lifecycle states am
   I in") decision is represented as scattered ad hoc flags rather than an explicit,
   exhaustively-handled type. Confirmed starting candidates to check concretely, not just
   assume:
   - `session/tymux/stream.go` — the proven, 3-times-patched case.
   - `session/tmux/control_mode.go` — the classic backend's own command-response reader
     goroutine and its `%exit`/reconnect handling; does it have an analogous ad hoc
     condition?
   - `session/actor.go` — the generic actor's own run/shutdown state; does *it* have a
     similar gap, independent of which backend is using it?
   - Any other goroutine in `session/` whose exit condition is a multi-clause boolean the
     audit turns up.
   Produce a concrete list (per-file, per-decision-point) — do not stop at "probably more
   exist somewhere."
2. **Build-vs-buy decision** for a shared building block (Phase 2/3): concretely evaluate
   `qmuntal/stateless` against this problem shape (see Alternatives' evaluation criteria)
   and against a bespoke internal generic helper. Land on one and justify it in
   `plan.md`/an ADR — this is exactly the kind of decision `docs/adr/` and `/sdd:adr` exist
   for.
3. **Migrate every subsystem the audit confirms has the smell** to the chosen mechanism —
   not just `session/tymux`. Each migration:
   - Replaces its ad hoc flags with the shared mechanism.
   - Gets observability (span/metric) via the established pattern.
   - Gets a regression test for its motivating incident (real, from this session's history,
     or a concretely demonstrated near-miss from the audit — not a hypothetical).
4. Fix the 3 documented `session/tymux` incidents through this new mechanism specifically
   (superseding, not duplicating, this session's inline `ctx.Err()`/bounded-timeout patches
   — fold them into the new design rather than leaving both old and new mechanisms
   present).

### Out of Scope

- Root-causing incident #2's underlying transport stall (missing HTTP/2 keepalive is a
  hypothesis, not confirmed) — the bounded-timeout mitigation already shipped this session
  stands as the safety net regardless of which lifecycle mechanism wraps it; deeper
  transport-layer investigation is a separate effort.
- Actually promoting `BackendTymux` to the default backend (a rollout/config decision, not
  an engineering readiness task) — this project removes a blocker for that decision, it
  doesn't make the decision.
- Rewriting `ReconnectLoop`'s backoff/retry *algorithm*, `ClientFanout`, or any
  wire-protocol concern in either backend — only how lifecycle/stop-vs-continue decisions
  are *represented and classified* is in scope, not the retry policy itself.
- Wiring/fixing the frontend browser-OTel pipeline (`NEXT_PUBLIC_OTEL_ENABLED`) — unrelated
  browser/UX observability concern; already handled separately this session (Makefile
  fix applied).
- Migrating `session.Status`'s own state machine (`session/instance_state.go`'s
  `TransitionDef`/`lookupTransition`) to whatever new mechanism this project produces —
  that is a *different* lifecycle (session-level: Creating/Active/Paused/...) that already
  has its own working, table-driven transition-validation design. It's valuable **prior
  art** to study (see Alternatives), not a migration target — don't destabilize a system
  that isn't broken.

## Rabbit Holes

- **The audit finding the smell is nearly everywhere and "migrate everything" balloons
  past even a Large appetite.** If so, the plan phase must explicitly prioritize (e.g. by
  incident history / blast radius) and propose a phased rollout — say so plainly rather
  than silently cutting the audit short or silently declaring victory after one file.
- **Building a fully general internal FSM framework when the actual need is narrower.**
  Even with a Large appetite, "does this goroutine stop or continue" is a small, specific
  shape of state machine (bounded set of terminal/non-terminal reasons); resist generalizing
  toward supporting arbitrary transition graphs, guards, and hierarchical states unless the
  audit's concrete findings actually need that generality.
- **Conflating this project's state machine with `session.Status`'s.** They are different
  lifecycles at different layers (goroutine-internal vs. session-level, observable
  externally via the API). Keep them architecturally separate even if both end up expressed
  with similar idioms.
- **Adopting `qmuntal/stateless` (or any library) without validating it actually fits an
  async, event-driven, per-goroutine decision** rather than the synchronous
  explicitly-triggered model such libraries are often designed around — validate with a
  small spike against the *real* `readAttachLoop` shape before committing the whole
  migration to it.

## Alternatives Considered

- **Do nothing / patch a 4th time when it recurs** — rejected; this is the status quo that
  already produced 3 incidents in one file, and the user has explicitly asked for
  systemic, not incident-by-incident, enforcement.
- **Adopt [`qmuntal/stateless`](https://github.com/qmuntal/stateless)** (user-proposed; a
  Go port of the .NET `stateless` library — explicit states/triggers, guarded transitions,
  entry/exit actions, DOT/mermaid diagram export). Concrete evaluation criteria for
  research/a spike:
  - Does it fit an *async, event-driven* transition (a background reader goroutine reacting
    to a `Receive()` error), or is its model built around synchronous, explicitly-triggered
    calls (`sm.Fire(trigger)`) that would need an adapter layer?
  - Does its state/trigger vocabulary map cleanly onto "why did this generation end"
    (transport drop → reconnect trigger; deliberate close/supersede/clean-exit → stop
    triggers), or would forcing that mapping be more awkward than a bespoke type?
  - Maturity/maintenance signal (releases, open issues, adoption elsewhere) and license
    compatibility.
  - Given the Large appetite and multi-subsystem migration in scope, is its cost
    proportionate as a *shared* dependency used by several `session/` subsystems (much more
    favorable than as a one-off single-file dependency)?
  If it fits, plan its adoption directly — don't default to hand-rolling an equivalent just
  because that was the first instinct.
- **Build a small internal, strongly-typed generic building block** (the user's own
  suggestion, e.g. a `typedfsm.Machine[State, Event]`-shaped helper) if `qmuntal/stateless`
  doesn't fit the async/event-driven shape well. Research must first check **existing
  precedent already in this exact repo**: `session/instance_state.go`'s
  `TransitionDef`/`lookupTransition` table (`session.Status`'s own transition-validation
  design) is a real, working table-driven pattern to learn from — even though its lifecycle
  is out of scope to migrate, its *design* may be the right shape to generalize from.
- **Lint rule (Level 2) flagging bare multi-clause boolean conditions in goroutine error
  branches** — a plausible *complement* (catches a human reintroducing the ad hoc pattern
  after migration) but not a replacement for the type-level fix, per the reflect-and-fix
  ladder's Level 1b gate. Evaluate in Phase 3 alongside the `exhaustive` linter carve-out
  (see Feasibility Risks) once the chosen type exists to check exhaustiveness against.
- **Minimal in-file fix only** (a single closed enum in `session/tymux/stream.go` alone,
  no cross-package building block) — this was the pre-scope-expansion floor; explicitly
  superseded by the user's Large appetite and cross-cutting framing. Kept here only as the
  cost/complexity floor the chosen approach should clearly justify exceeding.

## Feasibility Risks

- Go's lack of sealed/union types means any "exhaustiveness" guarantee is an idiom (a
  closed type + exhaustive `switch`, ideally lint-checked), not a compiler-enforced sum
  type — the plan must be explicit about which idiom it uses and why it's enough.
  `exhaustive` is **already a globally-enabled linter in this repo** — currently carved
  *out* (excluded) for every package except `session/detection/`, via a
  `linters.exclusions.rules` regex chain in `.golangci.yml` (see the comment there:
  "exhaustive enforces DetectedStatus switch coverage in session/detection/ only. All other
  packages use iota types with intentional default: clauses."). Enabling it for whatever
  new type(s) this project introduces means adding carve-outs to that existing chain
  (mirroring `session/detection`'s), not adding a new linter dependency.
- The audit may find that different subsystems' "ad hoc flags" aren't actually the same
  shape of problem (e.g. `session/actor.go`'s concern might be about actor shutdown
  ordering, not stop-vs-reconnect classification) — a single mechanism must not be forced
  onto a genuinely different problem shape just for uniformity. Confirm each candidate
  concretely before assuming it's the same class as the `session/tymux` incidents.
- `session/tymux`'s existing three signals (`closing`, `exited`, `ctx.Err()`) are not fully
  redundant today: `exited` fires on a natural clean-exit before any `cancel()` call, while
  `ctx.Err()` only covers deliberately-cancelled generations — whatever replaces them must
  still capture both origins correctly, not accidentally collapse a real distinction.
- Risk of touching hot, already-fragile concurrency paths in **two** backends without
  adequate coverage of the *new* mechanism itself, not just the known incidents — Phase 4
  validation must pre-mortem "what new way could a generation/goroutine fail to report its
  state correctly under the new design" for each migrated subsystem, not only the original
  `session/tymux` case.
- Large appetite + cross-cutting migration raises real regression risk in `session/tmux`,
  which is the currently-*default*, most-used backend — any change there needs proportionally
  more scrutiny (broader test coverage, possibly a staged/behind-a-flag rollout for that
  specific migration) than the already-opt-in `session/tymux`.

## Observability Requirements

- A span (or span attribute) per goroutine-lifecycle transition in every migrated
  subsystem (opened/started, reconnect attempt/success/exhaustion where applicable, ended +
  explicit typed reason), following the `telemetry.StartSpan` + registered-OTel-meter
  pattern already established this session (`exec_gate_observability.go`,
  `control_mode_observability.go`, `session/streamhub/observability.go`).
- A metric (counter) of lifecycle-end events labeled by subsystem and reason, so "how often
  does each stop-reason fire, in which subsystem" is a queryable question across the whole
  migrated surface, not per-file log archaeology.
- No new alerting/oncall requirement for the `session/tymux` side (still opt-in, internal).
  If the `session/tmux` migration (the default backend) is included, evaluate whether its
  metrics warrant a dashboard panel given it's the higher-traffic path — decide in Phase 3,
  not assumed here.

## Risk Control

- `session/tymux` is already gated behind `config.TymuxSessionOverrides` (opt-in per
  session) — low blast radius there regardless of appetite.
- `session/tmux` is the **default** backend — if the audit confirms it needs migration,
  Phase 3 must define a specific risk-control plan for that piece (e.g. a build tag,
  gradual rollout, or extra bake time in review) distinct from the tymux side's inherent
  opt-in safety net. Do not treat both backends as equally low-risk just because tymux is.
- Rollback procedure: standard `git revert` per migrated subsystem (structure the
  implementation as separable commits/PRs per subsystem specifically so a bad migration in
  one backend doesn't force reverting the other) — no data migration, no persisted-state
  format change, no wire-protocol change anticipated.
- Staged rollout: verify each migrated subsystem against its own full existing test suite
  (+ `-race`) plus new regression/pre-mortem-derived tests before merging that subsystem's
  migration — land per-subsystem, don't batch the whole cross-cutting change into one
  unreviewable commit.

## Open Questions

- Exact list of subsystems needing migration — resolved by Phase 2's audit, not guessed
  here. `session/tmux/control_mode.go` and `session/actor.go` are named starting
  candidates, not a confirmed final list.
- `qmuntal/stateless` vs. a bespoke generic helper vs. (if the audit finds the smell is
  narrower than feared) staying with per-file closed enums — resolved by Phase 2/3's
  concrete evaluation, recorded as an ADR.
- Whether the `session/tmux` (default-backend) migration ships in the same pass as
  `session/tymux`'s, or is sequenced as a separate, more cautiously-reviewed follow-on
  given its default-backend risk profile (see Risk Control) — a Phase 3 planning decision,
  not predetermined here despite the Large appetite covering both in scope.
- Whether to add `exhaustive` linter carve-outs for the new type(s) now, given it's already
  globally configured and just excluded per-package (see Feasibility Risks) — likely yes,
  low-cost, but Phase 3 makes the final call per subsystem.
