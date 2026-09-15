# Build vs. Buy: plan-feedback

## Scope of the question

The feature is a textarea + submit button in `BacklogItemDetail.tsx`, wired to
two RPCs the app already calls elsewhere: `transitionStatus` and
`triggerTriage(item.id, feedback)` (see `handleRejectPlan`/
`handleRegeneratePlanWithFeedback`, `web-app/src/components/backlog/BacklogItemDetail.tsx:990-1013`).
No new algorithm, protocol, or data structure is introduced. The four build-vs-buy
axes below are evaluated against that narrow scope, not against "backlog UI" in general.

## 1. Existing OSS library / framework — Not recommended (for this form)

Checked `web-app/package.json`'s `dependencies`: no form library is installed
(no `react-hook-form`, `formik`, `zod` + resolver, etc.). What *is* installed
and relevant:

- `@radix-ui/react-dialog` — a modal-primitive library, already a dependency.
  Not needed here: the feedback box is an inline expand/collapse section
  (see `PlanVerdictBox.tsx`'s `showReject` toggle pattern), not a dialog.
- No form-state library — and none is warranted. The form has exactly one
  field (a `<textarea>`), one derived value (`canSubmit = text.trim().length > 0`),
  and one submit handler. `react-hook-form` exists to manage validation,
  dirty-state, and re-render scoping across *many* fields; adopting it for a
  single controlled `useState<string>` would add a dependency, a learning
  surface, and bundle weight to replace four lines of code the codebase
  already has a working pattern for (`PlanVerdictBox.tsx:80-127`).
- A "feedback widget" SaaS-style library (e.g. a canned NPS/feedback-capture
  component) doesn't apply — this isn't end-user product feedback, it's an
  internal operator note threaded into an existing backend RPC parameter.

**Verdict: Not recommended.** Use a plain controlled `<textarea>` +
`useState`, matching `PlanVerdictBox.tsx`'s existing reject-reason form. No
new dependency justified.

## 2. SaaS / managed API — Not applicable

There is no third-party concern here (auth, payments, email, search) that a
hosted API would plausibly replace. The "service" being called is this
project's own `TransitionBacklogItemStatus` and `TriggerTriage` ConnectRPC
endpoints — internal workflow state on a single-operator, localhost-bound
instance (per `requirements.md`'s Non-functional Requirements: "internal,
single-operator tool"). No managed API applies.

**Verdict: Not applicable.**

## 3. LLM-generated implementation vs. battle-tested library — Mostly not applicable

The requirements name real backend precondition/guard logic
(`TriggerTriage`'s in-flight/orphan-session/concurrency-semaphore guards,
`requirements.md`'s Rabbit Holes) — but that logic **already exists and is
already tested** in `backlog_service_trigger_triage.go`; this feature calls
into it, it doesn't reimplement it. The only genuinely new logic this project
adds is:

- Which status guard/target (`ready` vs `refining`) the "send back" transition
  uses — a Phase 3 design decision, not an algorithm.
- Sequencing two RPC calls from one UI submit action (`transitionStatus` then
  `triggerTriage`, matching `handleRegeneratePlanWithFeedback`'s existing
  two-call pattern at `BacklogItemDetail.tsx:965`) and surfacing failure of
  either call.

Neither is a concurrency primitive, CAS loop, or data structure with
correctness risk that would favor a "battle-tested library" over hand-written
code — it's an ordered pair of `await` calls with a try/catch, the same shape
already proven out by `handleRejectPlan`/`handleRegeneratePlanWithFeedback`.

**Verdict: Mostly not applicable.** No algorithm/data-structure axis to
compare; write it directly, following the existing sequencing pattern.

## 4. Fork or adapt existing component — Recommended

`web-app/src/components/backlog/PlanVerdictBox.tsx` is a near-identical
precedent already in this codebase:

- Same interaction shape: toggle button → inline `<textarea>` form → Cancel /
  Submit, with `aria-expanded`, `aria-busy`, Escape-to-cancel, and
  focus-on-open (`PlanVerdictBox.tsx:80-127`, `193-243`).
- Same backend call shape: reject action persists a reason
  (`onReject`), and a second action re-triggers triage with that reason
  (`onRegenerateWithFeedback`) — both ultimately calling
  `transitionStatus`/`triggerTriage(item.id, feedback)` from
  `BacklogItemDetail.tsx:990-1013`, the exact two RPCs this feature reuses.
- Same length-cap and error-handling conventions (`InlineError` component,
  `actionError`/`actionErrorHeadline` state, dismiss-only error since there's
  no wireable retry distinct from re-opening the form).
- Same test precedent to model new tests on: `PlanVerdictBox.test.tsx`.

The one difference `requirements.md` calls out is that this project wants a
**single combined action** (type feedback, submit, done) rather than
`PlanVerdictBox`'s deliberate two-click reject-then-regenerate split (ADR-002).
That's a one-click UI wrapping the same two-RPC sequence underneath — not a
reason to avoid the fork; it changes the button count, not the underlying
call/error/state pattern worth reusing.

**Verdict: Recommended.** Fork/adapt `PlanVerdictBox.tsx`'s toggle-form
pattern (structure, ARIA attributes, focus handling, error display, test
scaffolding) as the template for the new feedback box, collapsing its two
buttons into one combined submit action per the requirements' explicit
one-click direction.

## Summary

| Axis | Verdict |
|---|---|
| 1. OSS form/library | Not recommended — no form library installed or warranted for one textarea |
| 2. SaaS/managed API | Not applicable — internal workflow state, no third-party concern |
| 3. LLM-gen vs. battle-tested lib | Mostly not applicable — no new algorithm; reuses already-tested backend guards |
| 4. Fork/adapt existing component | **Recommended** — adapt `PlanVerdictBox.tsx`'s toggle/form/submit pattern |

This is bespoke internal product glue. The only real "buy" decision is
reusing what the codebase already built for the structurally identical
`PlanVerdictBox` reject-plan flow, not adopting any external package or service.
