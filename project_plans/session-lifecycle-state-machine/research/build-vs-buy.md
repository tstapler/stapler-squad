# Build vs. Buy: shared goroutine-lifecycle-decision mechanism

Agent 6, SDD Phase 2. Scope per `project_plans/session-lifecycle-state-machine/requirements.md`:
evaluate `qmuntal/stateless`, other Go FSM libraries, a bespoke internal helper, and
generalizing `session/instance_state.go`'s existing `TransitionDef`/`lookupTransition` table.

## The concrete shape being decided for

`session/tymux/stream.go`'s `readAttachLoop` (lines 191-232), the proven 3-times-patched
case:

```go
func (s *tymuxGRPCSession) readAttachLoop(ctx context.Context, paneID string, stream attachStream, done chan struct{}) {
	for {
		event, err := stream.Receive()
		if err != nil {
			s.mu.RLock()
			exited := s.exited
			s.mu.RUnlock()
			if s.closing.Load() || exited || ctx.Err() != nil {
				close(done)
				return
			}
			newStream, first, ok := s.ReconnectLoop(paneID, "error")
			if !ok {
				close(done)
				return
			}
			stream = newStream
			continue
		}
		s.handleAttachEvent(event)
	}
}
```

This is one decision point, inside one goroutine's `for` loop, triggered by a single
error return from a blocking call. There is no external caller "firing a trigger" —
the goroutine itself observes its own environment (three independently-set signals:
`closing`, `exited`, `ctx.Err()`) and must classify "why did `Receive()` fail" into
either "stop" (return one of finitely many terminal reasons) or "continue" (reconnect).
Every option below is evaluated against reshaping exactly this call site (plus its
`session/tmux/control_mode.go` and `session/actor.go` analogues named in Scope).

## Option 1: `qmuntal/stateless`

**Verified repo data** (`gh api repos/qmuntal/stateless`, checked 2026-09-06):
- 1,383 stars, 23 open issues, not archived.
- Latest release `v1.8.0`, published 2026-02-10; latest commit same date (merged PR #102,
  "Add StateMachine.StateWithArgs") — actively maintained, not dormant.
- License: BSD-2-Clause (`gh api repos/qmuntal/stateless --jq '.license.spdx_id'`).
- `go.mod` requires `go 1.24`; this repo is on `go 1.26.6` (`go.mod:3`) — no version
  conflict.

**License compatibility**: this repo has no `LICENSE` file at the root (`ls LICENSE` →
not found) and `go.mod` carries no license-policy annotations — there's no existing
convention to check compatibility against beyond "is it a permissive OSS license,"
which BSD-2-Clause is. No blocker.

**API fit — the core question**. Read from the library's own README
(`gh api repos/qmuntal/stateless/contents/README.md`): the model is `sm.Fire(trigger,
args...)` / `sm.FireCtx(ctx, trigger, args...)` — a **caller-triggered, named-event**
transition. It advertises "Thread-safe" and a `FiringQueued` mode (queues a `Fire` call
if one is already in progress on another goroutine), which answers the concurrency
question but not the shape question:

- `stateless` is built around *something else telling the machine an event happened*
  (a phone rings, a call is dialed) and the machine deciding the resulting state via a
  transition table plus `Permit`/guard callbacks. That fits systems where discrete
  named events arrive over time and the state machine's job is answering "is this
  transition legal from here."
- `readAttachLoop`'s decision is the opposite shape: nothing "fires a trigger" into it.
  A single blocking call returns an error, and the goroutine must inspect **its own
  already-existing state** (three booleans/ctx) to classify *why*, then act (stop vs.
  reconnect) in the same statement. There's no sequence of events to a `sm.Fire()`
  call site — there's one synchronous classification of already-known facts.
- To use `stateless` here, `readAttachLoop` would need to: read `closing`/`exited`/
  `ctx.Err()` itself (unavoidable — `stateless` doesn't observe process state), map
  that ad hoc combination to a trigger name (e.g. `TriggerCleanExit`,
  `TriggerDeliberateCancel`, `TriggerTransportDrop`), call `sm.FireCtx(ctx, trigger)`,
  then re-read `sm.MustState()` to decide what to actually do (`close(done)` and
  `return`, or call `ReconnectLoop`). That adapter — reconstructing the classification
  that the ad hoc `if` was already doing, just to hand it to the library as a trigger
  name — is exactly the "adapter layer that partially defeats the point of adopting a
  library" the requirements doc warned about (Rabbit Holes). The library would own the
  *legality* of transitions between named states, which was never the bug: incident
  #1/#2/#3 were never "an illegal transition was attempted," they were "the classification
  of *why Receive() failed* was incomplete." `stateless` has no facility for that
  classification step at all — the caller still has to write it, by hand, before ever
  touching the library.
- What `stateless` *would* buy: a validated transition graph (`Permit`, `PermitReentry`,
  guards), entry/exit hooks, and DOT/Mermaid export for documentation. None of that
  addresses the actual defect class (an incomplete boolean-OR at one decision point) —
  it would formalize the states *after* classification, not the classification itself.

**What still needs custom wrapping regardless of adopting `stateless`**: the
observability requirement (a span/metric per lifecycle transition, following
`exec_gate_observability.go`'s pattern) is not something `stateless` — or any generic
FSM library — provides. It would need a `stateless.StateMachine.OnTransitioned` callback
wired to `telemetry.StartSpan` and a registered OTel counter, written and tested by this
project either way. Adopting the library saves nothing on this requirement.

**Verdict: Not recommended.** Real, mature, well-licensed library — but it solves a
different problem shape (caller-triggered named-event legality) than the one in scope
(async self-classification of an error's cause inside a single goroutine's loop). Forcing
it on would require writing the actual fix (the classification) by hand anyway, then
wrapping it in an adapter whose only job is translating that classification into a
trigger name the library can consume. That adapter is net new code with no corresponding
risk reduction — the library never touches the part of the system that broke three times.

## Option 2: Other Go state-machine libraries

Checked one clear alternative, `looplab/fsm`, since it's the other library commonly
recommended for Go FSMs (`gh api repos/looplab/fsm`, checked 2026-09-06): 3,412 stars,
29 open issues, not archived, last push 2026-08-27, Apache-2.0. More popular and equally
current than `stateless`, but the same `Event(ctx, eventName, args...)` caller-triggered
model applies — it is, if anything, a closer fit to "workflow with named states and
transitions" (it's frequently used for order/ticket workflows) and further from "a
goroutine classifying why its own blocking call just failed." No further libraries were
deep-dived per the requirements doc's own guidance ("brief comparison only... unless one
looks clearly better suited") — none did.

**Verdict: Not recommended**, same reasoning as Option 1, and it doesn't even improve on
license/maturity (both are fine on both axes) enough to change the API-fit conclusion.

## Option 3: Bespoke internal generic helper

The problem's actual shape, stated precisely: at one decision point, classify a bounded,
closed set of "why is this generation ending" reasons (deliberate close, deliberate
supersede/reopen, clean process exit, transport drop) and act on exactly two buckets
(terminal vs. retry), *safely* (a reason nobody classified must fall into "stop," never
"loop forever" — the Success Metrics' explicit invariant). That is not a general
transition-graph problem — there's no need for guarded multi-state graphs, reentrant
states, or hierarchical states (the Rabbit Holes section explicitly warns against
generalizing toward that). It's an enum plus an exhaustive switch, wrapped in a name that
makes "did you forget a case" a code-review-visible, ideally lint-checked, gap:

```go
type LifecycleReason int

const (
	ReasonUnknown LifecycleReason = iota // zero value — safe-by-default: unclassified => stop
	ReasonDeliberateClose
	ReasonSupersededForReopen
	ReasonCleanExit
	ReasonTransportDrop
)

func (r LifecycleReason) ShouldContinue() bool {
	switch r {
	case ReasonTransportDrop:
		return true
	case ReasonDeliberateClose, ReasonSupersededForReopen, ReasonCleanExit, ReasonUnknown:
		return false
	default:
		return false // fail safe: an unhandled future value still stops, never loops
	}
}
```

This is roughly 50-100 lines including the classification function that replaces the
`if s.closing.Load() || exited || ctx.Err() != nil` line, a constructor that forces
callers to pick a reason from the closed set rather than constructing a raw int, and the
`exhaustive` linter carve-out (mechanical, already precedented — see Option 4). The
correctness property this project actually needs — exhaustiveness, safe-by-default zero
value — is a Go-idiom-discipline problem, not a transition-graph problem a library's
guard/entry-hook machinery would help with. A hand-rolled closed enum with an exhaustive
switch is *lower* surface area than adapting an external library's API to a shape it
wasn't built for, and every line is auditable by the team that owns the three incidents,
rather than trusting a general-purpose library's behavior under a usage pattern (async,
self-observed, no external trigger) its own docs and examples don't demonstrate.

**Risk framing**: the Large appetite justifies paying for a library if it fits — but
"proportionate cost as a shared dependency" (per the Alternatives section) only holds if
the dependency does the *specific* job. Here it wouldn't: the classification logic has to
be hand-written regardless (Option 1's analysis), so adopting `stateless` is strictly
more code (adapter + library) than the bespoke helper alone, for equal or worse fit.

**Verdict: Recommended**, pending Option 4's comparison — a bespoke helper is justified
on the merits, but the next section asks whether the repo already has a working version
of exactly this pattern in a different lifecycle layer.

## Option 4: Generalize `session/instance_state.go`'s `TransitionDef`/`lookupTransition`

Read in full (`session/state_machine.go`, `session/instance_state.go`,
`session/actor.go`). `session.Status`'s state machine is a **from→to transition-graph**
design: `TransitionDef{From, To, Guard, After}` entries in a `[]TransitionDef` slice,
indexed at `init()` time into a `map[transitionKey]TransitionDef` for O(1)
`lookupTransition(from, to)`, with `CanTransition`/`canTransitionLocked` built on top and
`Guard`/`After` hooks for pre-condition checks and post-transition side effects
(`state_machine.go:35-70`). This is a real, working, proven design — but it is the *same
shape* as `qmuntal/stateless` and `looplab/fsm` (a validated graph between a bounded set
of named states, walked one edge at a time), just implemented in-repo instead of
imported. It shares Option 1/2's mismatch with `readAttachLoop`'s actual problem:
`session.Status` transitions are driven by explicit callers (`Approve()`, `Deny()`,
`ForceStatus()`, `RecoverFromStopped()`) invoking a transition by name, not a goroutine
inferring "why" from three independently-set signals after a blocking call fails.
Generalizing `TransitionDef` to `TransitionDef[S, E]` would still leave `readAttachLoop`
needing to map its three raw signals to a "trigger" before it could call into the
generalized table — the identical adapter problem as Option 1, just against in-house
code instead of an import.

What *is* directly transferable from this file, without generalizing its from→to graph
machinery at all:
- **The safe-default idiom.** `ErrInvalidTransition` (referenced at
  `instance_state.go:45`) makes an unhandled edge an explicit, typed error rather than a
  silent no-op — the same "fail safe on omission" property Success Metrics demands from
  the new mechanism, already proven in this codebase under real production load.
- **The exhaustive-switch-plus-lint precedent.** `.golangci.yml`'s existing carve-out
  structure for `exhaustive` (currently scoped to `session/detection/`'s `DetectedStatus`
  per the comment at `.golangci.yml:181`, `linters.exclusions.rules` entries at lines
  184-212) is the exact mechanical template Option 3's `LifecycleReason` type should
  reuse: add one more carve-out entry for the new type, not a new linter or new config
  shape.
- **`session/actor.go`'s serialization discipline** (`instanceState` capability token,
  `sendSyncErr`/`send`/`sendCtx`) is a different, complementary pattern this project
  should study for *how a decision gets applied without racing the rest of the actor* —
  not something to fold into the lifecycle-reason type itself. Per the Alternatives/Scope
  sections, `session/actor.go`'s shutdown-ordering concern is confirmed (by this file's
  own reading) to be a genuinely different problem (single-goroutine mailbox draining,
  not multi-signal error classification) — evidence supporting the Feasibility Risks
  warning against forcing one mechanism onto both.

**Verdict: Viable, but only for its idioms, not its transition-graph machinery.**
Generalizing `TransitionDef` itself into a shared `session`-wide FSM would repeat Option
1/2's mismatch (a graph-shaped tool applied to a classification-shaped problem) while
adding the risk of coupling two lifecycles the requirements doc explicitly says must stay
architecturally separate (Rabbit Holes: "Conflating this project's state machine with
`session.Status`'s"). The right reuse is narrower and lower-risk: borrow the
safe-default-error idiom and the `exhaustive` carve-out mechanics for Option 3's bespoke
type, and leave `TransitionDef`/`lookupTransition` untouched and unmigrated, exactly as
Out of Scope specifies.

## Summary table

| Option | Maturity/License | API fit for async self-classification | Extra work needed regardless | Verdict |
|---|---|---|---|---|
| `qmuntal/stateless` | 1,383★, active (Feb 2026), BSD-2-Clause — no concerns | Poor: caller-triggered named-event model; needs an adapter that duplicates the classification logic | Observability wiring (`OnTransitioned` → span/metric) either way | Not recommended |
| `looplab/fsm` | 3,412★, active (Aug 2026), Apache-2.0 — no concerns | Same mismatch as above, if anything more workflow/event-oriented | Same | Not recommended |
| Bespoke internal helper (closed enum + exhaustive switch) | N/A (no new dependency) | Direct fit: this *is* the classification, expressed as a Go idiom | Still needs observability wiring, but no adapter layer | **Recommended** |
| Generalize `instance_state.go`'s `TransitionDef` | N/A, proven in-repo, zero new dependency | Same from→to graph mismatch as libraries; also risks conflating two lifecycles the requirements explicitly keep separate | Would still need the classification step in front of it | Viable for idioms only (safe-default error pattern, `exhaustive` carve-out mechanics) — not as the transition machinery itself |

## Recommendation

**Build, not buy — a small internal generic helper**, in the shape sketched in Option 3
(a closed `LifecycleReason`-style type per subsystem or a shared generic wrapper,
constructor-gated, exhaustive-switch-checked, safe-by-default on the zero value), while
explicitly reusing two pieces of `session/instance_state.go`'s proven design (Option 4):
the "unhandled case is a typed, visible failure" idiom, and the existing `exhaustive`
linter carve-out mechanism, extended with one more entry rather than new tooling.

This matches the project's own stated risk framing: the Large appetite makes an external
dependency's *cost* easy to justify, but cost was never the blocker here — **fit** was.
`qmuntal/stateless` and `looplab/fsm` are both mature, well-licensed, and would be
reasonable choices for a genuinely different problem (a caller-driven workflow with named
states and legal-edge validation, which is what `session.Status` already has and is
correctly staying on). The problem this project is scoped to — a bounded, closed set of
"why did this goroutine stop" reasons, checked exhaustively at one decision point per
subsystem — is narrower and better served by Go idiom discipline (closed types,
exhaustive switches, a fail-safe default) than by a transition-graph library's guard/
entry-hook machinery, none of which addresses the actual defect class. Adopting a library
here would add a dependency and an adapter layer without removing any of the
classification logic that produced all three incidents — the adapter would still have to
get that classification right by hand, just before handing the result to the library
instead of acting on it directly.

Plan/ADR consequence: `plan.md` should specify the bespoke helper's exact shape (generic
`Machine[Reason]`-style wrapper vs. one closed type per subsystem — a Phase 3 design
question, not resolved here) plus the `exhaustive` carve-out addition, and record this
build-vs-buy verdict as the ADR's decision with this document as its evidence trail.
