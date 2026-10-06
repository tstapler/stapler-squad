# ADR-006: `RecordGateApproval` Gains Reject Semantics; Config-Error Detection Runs Before Satisfaction Lookup

**Status**: Accepted
**Date**: 2026-09-11
**Review**: Adversarial review (2026-09-11) found a real TOCTOU race in Part A's original Update path
and two smaller gaps — all three folded into the Decision below; see "Revisions from review" at the
end.
**Deciders**: Tyler Stapler (via SDD Phase 3 follow-on, `backlog-custom-workflow-stages`)
**Related**: `ADR-002-configured-workflow-engine-and-gates.md` (defines `GateStatus`/`PendingGates` this
extends), `ADR-004-stage-config-snapshot-persistence.md` (the most recent fail-closed-gate precedent
this ADR follows), `project_plans/backlog-custom-workflow-stages/design/ux.md` Surface 4 (the mockup
both gaps close)

---

## Context

Implementing ADR-005's item-detail gate checklist surfaced two things `GateChecklist.tsx` was already
built to display, that nothing on the backend produces:

1. **Reject has no RPC.** `RecordGateApprovalRequest` has no `approved`/`decision` field —
   `session.RecordGateApproval` unconditionally writes `Satisfied: true`. ux.md's own interaction flow
   says both the Approve and Reject buttons "call `RecordGateApproval(itemId, gateId, approved)`" —
   the design already assumed a boolean parameter that was never added to the proto.
2. **No evaluator ever populates a "configuration error" state.** `GateChecklist.tsx` already renders
   a `configError` field per-row (the "⚠ Configuration error... [Fix in Stages settings →]" row from
   ux.md's mockup), but `GateStatus` (Go and proto) has no field to carry it, and no evaluator checks
   whether an `automated_review` gate's referenced pipeline mode or a `custom` gate's referenced skill
   still exists at evaluation time — exactly the case ux.md's error table calls out: "Malformed/
   unresolvable gate config discovered only at evaluation time... surfaced instead on the item-detail
   gate checklist... per the 'fail closed and loud' mandate."

## Decision

### Part A: Reject is a reversible decision; Approve stays permanently one-shot

`RecordGateApprovalRequest` gains `bool approved = 4`. `session.RecordGateApproval` gains an
`approved bool` parameter and changes its write strategy from unconditional `Create` to:

1. Look up the existing record via `GetByItemAndGate`.
2. **No existing record** → `Create` with `Satisfied: approved` (an initial Approve or an initial
   Reject both start here).
3. **Existing record, `Satisfied: true`** (already approved) → always `ErrConflict`, regardless of
   `approved`'s value. Approval is final — matches ux.md's explicit "one-shot — does not re-ask" and
   today's exact behavior; there is no un-approve.
4. **Existing record, `Satisfied: false`** (previously rejected — the only way a `human_approval`
   gate's row can have `Satisfied: false`, since no other code path writes to a human-approval gate's
   row) → `Update` to the new `approved` value. A reject can be reversed into an approval (locking it
   per case 3 from then on), or re-rejected (idempotent, refreshes `SatisfiedBy`).

`SatisfiedAt` is set only when `approved` is true, matching the field's literal name — a reject has no
"satisfied at" moment.

**Concurrency**: step 4's read-then-write (`GetByItemAndGate` then `Update`) is a TOCTOU race between
two concurrent `RecordGateApproval` calls for the same (item, gate) — e.g. a reject and an approve
racing, where an unconditional `Update` (today's `EntGateSatisfactionRepository.Update`, a plain
Query-then-`UpdateOneID`) would let the loser silently clobber the winner's already-committed decision
with no error to either caller. `GateSatisfactionUpdateInput` gains one new field,
`ExpectedSatisfied *bool`: when set, the repository performs the write as a single conditional bulk
update (`ent`'s `Update().Where(item_id, gate_id, satisfied == *ExpectedSatisfied)`, checking the
affected-row count) instead of a separate select-then-`UpdateOneID`. Zero rows affected means either
the row doesn't exist (a real `ErrNotFound`, disambiguated with one follow-up lookup only in that
error path) or another writer already changed `Satisfied` out from under this call (`ErrConflict`,
never applied). `RecordGateApproval` always passes `ExpectedSatisfied: &existing.Satisfied` (the value
it just read in step 1) when taking the step-4 `Update` path, so a losing racer gets `ErrConflict` —
the same error code case 3 already returns for "can't re-decide an approval" — rather than silently
succeeding. `ExpectedSatisfied` is optional on the interface: every existing unconditional `Update`
caller (`InvokeCustomGateCheck`'s in-flight→terminal transition, `ReviewGateRunner`'s satisfaction
recording) passes nothing and keeps today's exact unconditional-update behavior — this is additive,
not a behavior change for anything already calling `Update`.

### Part B: Config-error detection runs before the satisfaction lookup, for `automated_review` and `custom` gates only

`GateStatus` (Go struct, `session/gate_status.go`; proto message) gains one additive field:
`ConfigError string` (Go) / `config_error` (proto, field 6) — empty means no config error. When
non-empty, the frontend renders the "⚠ Configuration error" row (already built, per ADR-005) instead
of the normal satisfied/unsatisfied row, using `ConfigError`'s text as the reason.

`evaluateGate`'s `GateKindAutomatedReview`/`GateKindCustom` branches (the only two kinds with an
external, mutable reference that can go stale — `human_approval` has no config, `structural` checks a
closed enum validated at save time) run a resolution check **before** consulting
`GateSatisfactionRepository`, not after:

- **`custom`**: parse `g.Config` into `CustomCheckConfig`; if `cfg.SkillID` is absent from
  `registeredCustomCheckSkills` (already a package-level var in `session`, no new dependency needed —
  `session/gate_config.go`), return `Satisfied: false, ConfigError: "the custom check skill %q is no
  longer registered"`.
- **`automated_review`**: parse into `AutomatedReviewConfig`; if `cfg.PipelineMode != ""`, resolve it
  via a newly-threaded, nil-guarded `PipelineModeRepository.GetBySlug` (mirrors `gateSatisfactionRepo`'s
  existing nil-guarded-optional-dependency pattern in this same struct). Not found (or repository
  unwired) → same `ConfigError`-populated blocking status, worded for the pipeline-mode case.
- The check running *before* the satisfaction lookup means a gate whose config went stale **after** it
  was already recorded satisfied still surfaces as a config error on the next `PendingGates` call for
  that pending transition — this only matters while the transition is still pending (an item that
  already transitioned past this gate is unaffected, since `PendingGates` is only ever evaluated for
  the *current* stage's outgoing edges).

`ConfiguredWorkflowEngine`'s constructor gains one new nilable parameter, `pipelineModeRepo
PipelineModeRepository`, wired from `server/dependencies.go` — `pipelineModeRepo` is already
constructed there, inside the same `entClient != nil` branch, strictly before
`NewConfiguredWorkflowEngine` is called, so this is additive wiring, not new construction-ordering
risk. Note: `PipelineModeRepository.GetBySlug` returns `*ent.PipelineMode`, not an ent-free DTO like
`GateSatisfactionRepository`'s methods — `session/configured_workflow_engine.go` has imported no
`session/ent` type until now, so this is that file's first ent import. Not blocked by any lint rule
(`no_ent_in_services` scopes only `server/services/**`, and several other `session/` files already
import `session/ent` directly), but worth naming as a small style cost: this one file's previously
ent-free surface ends here, for a lookup that only ever needs the existence check (`err == nil`), not
any field on the returned struct.

## Rationale

- **Reject-reversible / approve-final matches how a human actually uses this**: reject-then-fix-
  then-approve is a normal operator workflow; un-approving after the fact is not (per ux.md's own
  "one-shot" framing for the satisfied case specifically, never generalized to "no state ever
  changes"). Modeling this as Create/Update on the schema's existing `Satisfied` bool, rather than
  a new column or a new table, reuses the exact same one-shot enforcement (`Create`'s
  `UNIQUE(item_id, gate_id)` constraint) that already protects the approve path.
- **Config-error runs before the satisfaction lookup, not after**, so a previously-satisfied gate
  whose backing config later disappears doesn't silently keep reporting `Satisfied: true` off a stale
  record — it re-surfaces as a loud config error, per the exact "fail closed and loud" mandate ux.md
  cites for this case, and consistent with ADR-004's precedent of never letting a resolution failure
  silently degrade to "pass."
- **`registeredCustomCheckSkills` needs no new wiring** (already visible in-package); `PipelineModeRepository`
  needs exactly one new constructor parameter, using the identical nil-guarded-optional-dependency shape
  every other cross-cutting lookup in `ConfiguredWorkflowEngine` already uses (`gateSatisfactionRepo`).
  No new pattern introduced.

## Alternatives Considered

**Alt A (Part A): A separate `RejectGateApproval` RPC instead of extending `RecordGateApproval`.**
Rejected — ux.md's own design already names one call, `RecordGateApproval(itemId, gateId, approved)`,
for both actions; splitting into two RPCs is scope not asked for by the design this ADR is closing the
gap against, and would leave the proto inconsistent with the doc it's implementing.

**Alt A (Part A): Reject also permanently one-shot (no reversal, symmetric with approve).**
Rejected — an operator who rejects prematurely (e.g. before the item's plan was actually ready) would
have no path forward except re-creating the whole item/gate row out-of-band. Reject-then-approve
is the realistic recovery path and costs nothing extra in the schema.

**Alt B (Part B): A boolean `has_config_error` flag instead of a `ConfigError` string.** Rejected —
the frontend row needs the actual reason text (`GateChecklist.tsx`'s existing "Configuration error —
this gate can't be evaluated (<reason>)" wording, per ux.md), and a separate boolean-plus-reused-
`Description` pairing would just move the same information into two fields with an implicit coupling
between them instead of one self-contained field.

**Alt C (Part B): Wire `pipelineModeRepo` via a post-construction `SetPipelineModeRepository` setter,
matching `livenessRepo`/`gateSatisfactionRepo`'s own late-bound-setter precedent used elsewhere in
`dependencies.go`, instead of a 3rd constructor parameter.** Considered, kept as the constructor
parameter instead — `NewConfiguredWorkflowEngine` already takes `gateSatisfactionRepo` as a
constructor argument (not a setter), so a 3rd constructor parameter is consistent with this specific
type's own established pattern, whereas the setter precedent cited belongs to a different struct
(`BacklogLifecycleListener`/`BacklogService`, which take many more post-construction dependencies for
test-call-site reasons that don't apply to `ConfiguredWorkflowEngine`'s much smaller, 2-call-site
constructor surface). The mechanical cost either way is one line at each of the same two call sites
(`server/dependencies.go` and one test-double constructor); a setter buys nothing here.

**Alt B (Part B): Detect config-error at gate-save time only (already partially true — `ParseGateConfig`
validates the skill allowlist and config shape at save), never re-check at evaluation time.** Rejected
outright by ux.md's own error table, which explicitly distinguishes "invalid at save time" (caught by
`ParseGateConfig`, unrelated to this ADR) from "valid at save time, invalid later" (a pipeline mode or
skill removed *after* the gate was configured) — the second case is this ADR's actual subject and has
no save-time hook to catch it by construction.

## Consequences

**Positive**: closes both remaining gaps `GateChecklist.tsx` was already built to display; reuses the
schema's existing one-shot enforcement mechanism rather than adding new state; zero new tables.

**Negative**: `ConfiguredWorkflowEngine`'s constructor gains a 3rd parameter — every existing call site
and test double constructing it directly needs a one-line update (mechanical, not a design cost).

**Neutral**: `RecordGateApprovalRequest.approved`'s proto3 zero-value is `false` — every caller (there
is exactly one, the frontend's `useGateApproval()` wrapper, itself part of ADR-005's unshipped work)
must be updated to pass it explicitly; there is no external client relying on the old implicit-always-
true behavior to preserve compatibility for.

---

## Revisions from review

Adversarial review (2026-09-11) confirmed every factual claim in this ADR against the actual code and
found one real correctness bug in the original draft: the Part A `Update` path (existing
`Satisfied: false` → overwrite) was a plain read-then-write with no concurrency guard, so two
concurrent `RecordGateApproval` calls for the same (item, gate) — e.g. a reject racing an approve —
could let the loser silently clobber the winner's decision with no error surfaced to either caller,
directly undermining Part A's own "approve is permanently final" guarantee. The Decision above now
specifies a compare-and-swap (`GateSatisfactionUpdateInput.ExpectedSatisfied`, an ent conditional bulk
update checking affected-row count) as part of the accepted design, not a follow-up. The review's two
smaller findings — the `session/ent` import this introduces into `configured_workflow_engine.go`, and
the constructor-parameter-vs-setter choice for `pipelineModeRepo` — are addressed inline above and in
Alt C respectively.
