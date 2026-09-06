# ADR-003: Custom/Pluggable Gate Checks Are Bounded to Named Skill Invocation, Never Arbitrary Code

**Status**: Accepted
**Date**: 2026-09-03
**Deciders**: Tyler Stapler (via SDD Phase 3 planning, `backlog-custom-workflow-stages`)
**Related**: `ADR-001-liveness-engine-sibling-interface.md` (the timeout primitive this ADR reuses)

---

## Context

requirements.md's Scope explicitly includes a fourth gate type — "custom/pluggable check" — as "an
extensibility point for a check type not enumerated above... open-ended by the user's explicit choice
to include this, not a closed enum," but its own Rabbit Holes section immediately flags this as "the
single largest scope-blowout risk in the whole project if left unbounded," and suggests constraining
it to "invoke this named skill/slash-command, treat exit 0 / a specific `report_progress`-style call
as pass" rather than arbitrary code execution.

`research/build-vs-buy.md` and `research/architecture.md` §5 both independently converge on the same
narrowing: a custom check is itself a bounded unit of work with the exact same liveness/timeout shape
(`LivenessKindDurationBudget`, ADR-001) as a headless triage call — it should reuse that primitive,
not invent a third timeout mechanism, and its execution surface should be closed (a reference into an
already-reviewed set of skills), not open (a shell command, script path, or URL).

## Decision

A `GateKindCustom` gate's configuration may name **only** an identifier from a pre-registered
allowlist of existing skill/slash-command identifiers already reachable through this codebase's
existing skill-invocation surface. It may **not** supply:

- A shell command or script path.
- A URL or network callback.
- Any form of inline code (a script body, an expression to `eval`).

`InvokeCustomGateCheck` (`session/gate_custom_check.go`) spawns the named skill/slash-command bounded
by a `LivenessDefinition{Kind: LivenessKindDurationBudget, ExpectedDuration, StalenessMargin}`
resolved through the same `LivenessEngine.LivenessFor` path every other Shape-A liveness consumer
uses (headless triage, per ADR-001). An overdue custom-check invocation is picked up by the same
periodic `reconcile*` sweep infrastructure already built for `orphaned_triage` — no new detector, no
new timeout primitive. The check's outcome is reported through the existing
`domain.ReviewOutcome` PASS/FAIL/UNVERIFIABLE vocabulary, rendering through the same UI/verdict-
aggregation code as an automated-review gate (Epic 2.10's `GateChecklist.tsx`).

## Rationale

- **Closed execution surface removes the scope-blowout risk at its root**: naming only pre-reviewed
  skill/slash-command identifiers means a custom check's content has already passed whatever review
  this codebase's own skills go through — there is no new "run whatever the operator typed" surface
  to secure.
- **Reusing `LivenessEngine` avoids a third timeout mechanism**: `research/architecture.md` §5's
  explicit finding is that a custom check "is itself a piece of work that can hang, crash, or run
  long" — exactly the shape `LivenessKindDurationBudget` already models. Building a separate timeout
  concept for this one gate type would duplicate ADR-001's design for no reason.
- **Consistent with this project's single-operator, structural-integrity threat model**
  (requirements.md's Non-functional Requirements): the goal is protecting the operator from
  self-inflicted misconfiguration, not defending against an untrusted third party — a closed
  allowlist achieves that without needing sandboxing infrastructure this tool has no other reason to
  build.

## Alternatives Considered

- **Arbitrary shell command or script path** — rejected: requirements.md names this explicitly as
  the scope-blowout risk to avoid; no sandboxing story exists or is proposed elsewhere in this
  project, and building one would dwarf this project's own Large (3-6 week) appetite for the entire
  feature.
- **Outbound webhook callback to an operator-hosted service** — rejected: reintroduces a network
  dependency, and its own auth/retry/timeout semantics from scratch, that doesn't reuse
  `LivenessEngine` — a new integration surface this local, single-operator tool has no other need
  for.
- **A fully open string enum for "gate type," with custom types defined by arbitrary future code** —
  rejected in favor of the closed `GateKind` sum type (ADR-002's Pattern Decisions): compiler-enforced
  exhaustive handling is more valuable here than open extensibility, given the stated goal is a
  *bounded* extensibility point, not an unbounded one.

## Consequences

**Positive**: no new sandboxing, auth, or network-security work is required by this project; a
custom check's stuck-detection, remediation-backoff, and UI rendering are all free reuses of
machinery Milestone 1 and the rest of Milestone 2 already build.

**Negative**: an operator cannot define a custom check that isn't already expressible as an existing
skill/slash-command — genuinely less flexible than arbitrary code execution would be. This is an
intentional trade accepted by requirements.md's own Rabbit Holes framing, not an oversight.

**Neutral**: the pre-registered allowlist mechanism (Epic 2.4, Task 2.4.4a) needs its own small design
choice (a static Go list vs. a DB-configurable one) — left to Epic 2.4's implementation, since either
choice satisfies this ADR's closed-surface requirement equally.
