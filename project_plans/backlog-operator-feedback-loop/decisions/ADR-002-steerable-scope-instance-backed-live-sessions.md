# ADR-002: AC6 Narrowed — Only Instance-Backed, Live Sessions Are Steerable From the Backlog View

**Status**: Accepted
**Date**: 2026-08-12
**Project**: backlog-operator-feedback-loop (Gap 2, AC6)
**Resolves**: requirements.md Open Question 3 ("Does steering a headless, prompt-driven
triage or review session behave the same as steering an interactive session?")

## Context

AC6 as written says: *"A running triage, review, or work session attached to a backlog item
can be steered from the backlog item detail view."* Research established that this is
unsatisfiable as worded for the triage case, and for the headless subset of the review case
— not as a bug, but as a property of how those runs execute.

Evidence, read directly from current `HEAD` (`research/architecture.md` §3.2,
`research/pitfalls.md` Gap 2):

- `session/backlog.go:55-66`'s `IsTmuxBackedSessionRole` doc comment states it outright:
  triage sessions "run as bounded one-shot headless subprocess calls … that exit on their
  own when the call returns, so they were never tracked as a live Instance in the first
  place."
- `TriggerTriage`'s completion path (`server/services/backlog_service_triage.go:~2344`)
  creates only a DB bookkeeping row (`CreateItemSession`) with a synthetic
  `headlessTriageUUIDPrefix + uuid` session ID. The actual LLM work runs through
  `session/headless.Pool.CallBlocking` (`session/headless/caller.go:487-493`) — a bare
  subprocess, no tmux, no PTY, no `session.Instance`.
- **Both** steer paths resolve their target against the live `Instance` registry: the MCP
  tool via `findInstance` (`tools_terminal.go:713-728`), the RPC via
  `reviewQueuePoller.GetInstances()` / `loadInstancesWithWiring()`
  (`session_service.go:1836-1857`). A `headless-*` session ID can never match either,
  because no `Instance` was created.

The frontend already encodes this split, independent of this project:
`classifySessionKind` (`web-app/src/lib/backlog/sessionKind.ts:29-43`) returns
`"headless_diagnostic"` for `role === "triage"` or any `headless-`-prefixed session ID, and
its own doc comment names the distinction — *"'work' and 'review' are Real Sessions (backed
by an actual session.Instance/tmux/PTY); the other three are Synthetic Sessions — DB-only
rows with no backing Instance, no terminal, nothing to attach to."* `SessionsSection.tsx:116`
already branches on exactly this predicate (`isSynthetic`) to decide whether a row renders as
a clickable `<a>` or as a non-interactive diagnostic panel.

## Decision

**Narrow AC6 to: "A running *work or review* session attached to a backlog item — one that
is Instance-backed and has not ended — can be steered from the backlog item detail view."**

The Steer control renders if and only if:

```ts
// web-app/src/lib/backlog/sessionKind.ts (new export)
export function isSteerable(session: Pick<LinkedSession, "role" | "sessionId" | "endedAt">): boolean {
  const kind = classifySessionKind(session);
  return (kind === "work" || kind === "review") && !session.endedAt;
}
```

For a synthetic session kind (`headless_diagnostic`, `blocked_guardrail`,
`manual_review_marker`) the control is **not rendered at all** — those rows already render
as a collapsed `SessionDiagnosticPanel`, and adding a disabled Steer button to a
never-steerable row is noise. For a `work`/`review` row that has **ended**, the control
renders **disabled** with `title="Session has ended — steering is unavailable"`, because
"this used to be steerable" is information the operator needs (an enabled-then-failing
button is the failure mode this decision exists to prevent).

`isSteerable` lives next to `classifySessionKind` in `sessionKind.ts` so both the row's
`isSynthetic` render branch and the Steer gate derive from one classification, per
`research/pitfalls.md` §6's "no canonical derivation function" warning applied to this gap.

### What replaces steering for a triage run

Nothing new is built. The operator's lever for a triage run is Gap 1 of this same project:
answer the clarifying question and re-trigger triage with that answer as feedback. That is
a structurally different capability (supply input to the *next* run) than steering (inject
input into a *running* process), and it is already in scope here — so the narrowing removes
no operator capability that this project delivers, it just stops promising it under the
wrong name.

## Alternatives Considered

1. **Build a side-channel so headless runs can be steered mid-flight** — e.g. a file the
   headless prompt polls, or kill-and-re-invoke the subprocess with amended input.
   Rejected: this is a genuinely new steering mechanism, which AC7 explicitly forbids
   ("no parallel steering implementation"), and it would require the triage prompt itself
   to cooperate — squarely inside requirements.md's out-of-scope line ("Redesigning the
   triage prompt or triage agent logic").

2. **Render an always-disabled Steer button on triage rows with an explanatory tooltip**
   (`research/pitfalls.md` Gap 2 option (b)). Rejected for synthetic rows specifically:
   those rows are already a collapsed diagnostic panel, not an action surface, and a
   permanently-disabled control on a permanently-unsteerable row teaches nothing after the
   first hover. The ended-work-session case is different — there the control is
   *conditionally* unavailable, so the disabled state carries real state information, and
   that case does render disabled-with-reason.

3. **Leave AC6 as written and accept the 404 for triage.** Rejected: `research/ux.md`'s
   JTBD analysis is explicit that a Steer button that silently no-ops damages the
   "I can intervene" job worse than no button, because it teaches the operator the control
   can't be trusted.

## Consequences

- **AC6's wording changes.** requirements.md's AC6 is superseded by the narrowed version
  above; plan.md's Given-When-Then for AC6 and the e2e spec are both written against the
  narrowed form. This is a scope reduction driven by a structural finding, and it is
  recorded here rather than silently absorbed into the plan.
- AC8's e2e test for criterion 6 exercises a **work** session, and additionally asserts the
  control is absent for a `headless-`-prefixed triage row — so the narrowing is itself
  regression-tested, not just documented.
- Requirements.md Open Question 3 is closed: steering a headless triage/review run does not
  behave like steering an interactive session; it cannot happen at all through either
  existing mechanism.
- Follow-up left open (not this project): if headless steering ever becomes a real
  requirement, the honest shape is a first-class "cancel and re-run with amended input"
  operation on the headless pool, not an extension of `steer_session`.
