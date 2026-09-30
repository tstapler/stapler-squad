# Research: Pitfalls — backlog-custom-workflow-stages

Grounded first in this exact codebase's failure history (BUG-055, BUG-083, the 2026-09-03
sdd-triage-timeout incident, and the recurring "silently" shapes tracked in
`docs/tasks/backlog-feature-improvement.md`), then in the sibling `backlog-configurable-pipeline`
project's own pitfalls research, then in general workflow-engine/liveness/gate pitfalls scoped to
what this specific design must guard against.

---

## 1. BUG-055 — the sweep-vs-budget race this project's liveness model must structurally prevent

**What happened** (`docs/bugs/fixed/BUG-055-staleness-sweep-tombstones-live-headless-triage-past-30min.md`):
`maxHeadlessTriageSessionStaleness` (the sweep's staleness threshold) and `triageCallBudget` (the
real call's own timeout) were two independently-declared constants in two different packages,
originally both `30 * time.Minute` with a code comment claiming the sweep constant "MUST stay
strictly greater than" the budget — enforced by nothing but the comment. A real call running
27–31 minutes collided with the sweep on every attempt. The fix had two parts: (1) a structural
liveness signal (`IsTriageLive`, backed by an in-process `triageInFlight` registry) so the sweep
checks "is this actually still running" instead of guessing from elapsed time alone, and (2) a
regression test, `TestMaxHeadlessTriageSessionStaleness_should_ExceedRealTriageCallBudgetWithMargin`,
that pins the *invariant* (sweep threshold > budget, with real margin) so a future edit can't
silently reintroduce a zero-margin race even if the liveness check is later bypassed.

**Confirms requirements.md's Constraints section is aimed correctly**, but note precisely *why*
both halves of the BUG-055 fix matter, because a naive "make the model derive one number" design
only reproduces half of it:

- **Deriving the sweep threshold from the budget (`sweep = budget + margin`) closes the
  configuration-drift half** — a UI that lets a user set a stage's `expectedDuration`/`budget`
  once and computes `stalenessThreshold` from it (rather than storing both as independently
  editable fields) makes the invariant impossible to violate by construction, which is stronger
  than a regression test alone. This is the right target for the new liveness model's schema:
  the DB should very likely persist *one* duration (the budget) plus a *margin policy* (a fixed
  or percentage-based buffer), not two independently-settable absolute thresholds — otherwise a
  custom-stage-defining UI reintroduces exactly the "two numbers a human keeps in sync" design the
  Rabbit Holes section already flags as the failure mode to avoid.
- **The derived-threshold structure alone does not eliminate the need for an actual liveness
  signal.** BUG-055's real fix was `IsTriageLive` querying an in-process registry, not the
  constant bump — the margin bump was explicitly called out in the bug doc as "defense in depth,"
  not the structural fix. A stage's liveness definition in the new model must therefore carry (or
  be paired with) *how to ask whether the work is still actually running* — for a headless call,
  an in-process in-flight registry check; for a tmux-backed work session, `hasActiveWorkSession`;
  these are **different liveness shapes for different kinds of work**, not one universal signal
  (see §4 below, and Feasibility Risks' own note that `stale_work` and `orphaned_triage` are
  shaped differently). If the new model reduces "liveness" to only a duration/threshold pair with
  no query-time "is it actually alive" hook, it regresses BUG-055's real fix back to
  duration-based guessing, even with a perfectly-derived margin.
- **Verification for Phase 3/5**: whatever schema/interface change implements this, re-run (or
  port) `TestMaxHeadlessTriageSessionStaleness_should_ExceedRealTriageCallBudgetWithMargin`'s
  *intent* against the new model — assert the invariant holds for every migrated `StuckReason`
  threshold pair, not just re-verify the old constants still satisfy it once and then stop
  checking.

---

## 2. BUG-083 — would migrating stuck-detection onto the new model become a 7th "write side fixed, recovery side missing" instance?

**What happened** (`docs/bugs/fixed/BUG-083-parked-remediation-rows-never-automatically-retry.md`):
`evaluateRemediation`'s parked branch (`remediation_attempts >= MaxRemediationAttempts`) was a
dead end with no automatic path out — a code fix that resolved the underlying failure (PR #535)
left 20 already-parked items stranded for up to 2 weeks because parking and "can never self-heal"
were conflated. The doc explicitly names this the **sixth** recorded instance of "a fix closes the
write side of a gap but not the recovery side" and the fix was scoped to the *shared gate*
(`RemediationDue`), not the one call site the live incident happened to hit — closing it for every
current and future `StuckReason` at once.

**Where this project's design risks becoming a 7th instance**: the requirements' own Milestone 1
scope (migrating `maxHeadlessTriageSessionStaleness`/`triageCallBudget`/siblings onto the new
per-stage liveness model) is, structurally, exactly the kind of change class BUG-083 warns about —
it changes *how a threshold is computed/looked up* for the write side (the sweep, the call
budget), but says nothing on its face about the recovery side (`RemediationDue`/
`evaluateRemediation`'s cold-retry heartbeat). Concrete risk: if a stage/liveness identifier fails
to resolve for an *already-parked* row (e.g. its `pipelineModeSnapshot` refers to a stage
config that was since edited/deleted — see §5), and the fallback path is "leave it parked, log a
Warn," that row silently never gets BUG-083's cold-retry heartbeat re-evaluated against a
corrected config the way the 2026-09-03 incident's Root Cause 1 fix is explicitly expected to let
the 12 parked items "recover on their next remediation attempt... with no manual per-item
intervention" (Success Metrics, Milestone 1). If the new model's config-resolution failure path
and `RemediationDue`'s cold-retry path are not both exercised together for a parked-row scenario,
this ships the write-side fix (Milestone 1) without confirming the recovery-side path (BUG-083's
mechanism) actually still fires for it.

**How to structurally avoid it**: treat "a parked row whose `StuckReason` now resolves against a
migrated/updated liveness config" as an explicit test scenario in Milestone 1's regression suite —
not just "the sdd/triage pair gets the right numbers now," but "a row parked under the *old*
hardcoded thresholds becomes due again via `RemediationDue`'s existing cold-retry heartbeat once
Milestone 1 ships, with no code change to `RemediationDue`/`evaluateRemediation` required." This is
exactly the scenario Risk Control's "12 parked items... recovering on their next remediation
attempt" success metric already implies, but it must be an explicit test, not an assumption — the
6-instances-and-counting history says assumptions here are exactly where this shape keeps
resurfacing.

---

## 3. Sibling project's pitfalls: what transfers from `backlog-configurable-pipeline`, what doesn't

Read in full (`project_plans/backlog-configurable-pipeline/research/pitfalls.md`). Same threat
model (single-operator, DB-mutable config, structural-integrity-not-access-control) — most of its
§1 "reconciliation-bug classes" transfer directly since this project touches the same files
(`backlog_service_triage.go`, `backlog_lifecycle*.go`, `TransitionBacklogItemStatus` call sites):

**Transfers directly:**
- **§1a (silent error-swallowing) / §1d (unrecognized-value no-op)** — directly on point for
  this project's config-resolution path: resolving a stage/liveness/gate identifier that doesn't
  exist (deleted, typo'd, malformed) is a `switch`/map-lookup exactly like `PipelineEngine.Resolve`
  on an unrecognized mode string, and needs the identical fail-closed-and-loud treatment
  requirements.md's Observability section already specifies. This is the single most directly
  reusable lesson — apply it to `ConfiguredWorkflowEngine`'s stage/transition/gate lookups the
  same way `PipelineEngine.Resolve` was required to.
- **§1f (missing audit trail on direct-ent status mutation)** and the newer finding from
  `docs/tasks/backlog-feature-improvement.md` (line ~1413-1418): **`session.CanTransitionBacklog`
  is already called directly, bypassing `s.engine`, at two confirmed sites** —
  `backlog_service_lifecycle.go:801` (`OverrideVerdict`) and `backlog_service_sync.go:121`
  (`AttachSessionToItem`). The doc's own words: "harmless today (the default engine *is* the
  static map it bypasses to), but both sites would silently ignore a per-item
  `ConfiguredWorkflowEngine` gate the day ADR-013 ships." **This project is that day.** These two
  call sites must be found and re-routed through the injected `WorkflowEngine` (not the static
  `domain.CanTransitionBacklog`) before `ConfiguredWorkflowEngine` ships, or a custom transition's
  gates are silently bypassable through these two paths on day one. Treat this as a Milestone
  2 blocking checklist item, not a follow-up — the gap is already documented and dated, it just
  hasn't bitten anyone yet because the default engine happens to match the bypassed function.
- **§4 (read-once-at-goroutine-start / multi-call-site consistency)** — this project's liveness
  resolution has the identical shape risk: `WriteSlashCommands`, the triage prompt builder, the
  stuck-detection sweep, and the remediation gate are *four* different call sites (not the
  sibling's three) that may each need to resolve "what's this item's current stage/liveness
  config" independently, at different points in the item's lifecycle. The same recommendation
  applies: either snapshot the resolved liveness/gate config onto the `ItemSession` at the point
  work starts (extending the existing `AcSnapshot` precedent), or explicitly scope
  config-changes-mid-flight as out of scope for now (matching the sibling's own scoping choice for
  pipeline mode) — but state which, explicitly, in Phase 3 planning, since silently reading fresh
  config at every call site while another call site cached a snapshot is exactly the drift this
  section warns about.
- **§5 (rollout/blast-radius: what makes "Default" silently diverge)** — same audit-by-call-site
  discipline needed here: every place `CanTransition`/`ValidateGates`/`AllowedTransitions` (or
  their new liveness/gate-carrying siblings) are consulted must be enumerated and confirmed to go
  through the same `ConfiguredWorkflowEngine` instance, the same way the sibling project required
  confirming both `BacklogService` and `BacklogLifecycleListener` share one `PipelineEngine`
  instance. Given the workflow-engine-bypass finding above already proves at least two call sites
  don't currently go through the engine at all, this audit is not hypothetical — it will find
  real work.

**Specific to `PipelineEngine`, does not transfer as-is:**
- §2 (proto3-bool-clobbering, `optional string pipeline_mode`) — the *lesson* ("optional wire
  types for partial-update semantics, presence not truthiness") transfers as general guidance
  (and requirements.md's Constraints section already cites it), but the specific field doesn't
  exist here; this project's new fields (stage ID, transition ID, gate config) are new schema, not
  a retrofit of an existing plain-bool/string field, so there's no existing clobbering bug to
  avoid repeating — just the general principle to apply to any new optional per-item override
  field this project adds (e.g. a per-item liveness override, if planning adds one).
- §3 (prompt-injection / `SanitizeDiff`) — not directly relevant unless the "custom/pluggable
  check" gate type (Rabbit Holes) ever interpolates a custom check's *name* or *config* into an
  LLM prompt (e.g. an automated-review-verdict gate on a custom transition needs to tell the
  reviewer prompt "you are gating transition X→Y for reason Z"). If any custom/user-authored
  string (a stage name, a gate description) is ever spliced into a prompt, the sibling's
  allow-list-not-escaping recommendation applies (validate against a registry of known stage/gate
  IDs before use; never interpolate a raw free-text field). Flag for Phase 3 planning if the
  custom-stage UI allows free-text stage/transition names that later reach a prompt.

---

## 4. The "silently" pattern across this subsystem, applied to a configurable gate/liveness model

`docs/tasks/backlog-feature-improvement.md` names this shape repeatedly (BUG-030, BUG-040,
BUG-041, BUG-046, BUG-048, the 2026-07-27 batch of four, BUG-083 as a sixth instance of a related
but distinct sub-shape) and explicitly calls it out as recurring faster than individual fixes can
close it, recommending "the earliest enforceable rung" over another one-off patch. Two concrete
ways a configurable gate/liveness model reintroduces this exact shape, both **already flagged as
live risks by requirements.md's own Observability/Risk Control sections** — this section is
mainly a "yes, the mitigation you already planned is the right one, and here's why it isn't
automatically sufficient":

1. **A malformed/deleted custom gate or stage definition silently never blocks** — e.g. a
   transition's gate list references a gate-type identifier that was valid when the transition was
   defined but the gate-type was since removed, or a custom/pluggable check's target skill was
   deleted. If `ValidateGates`-equivalent resolution treats "gate definition not found" as
   "no gate to check" (an empty/no-op result) rather than an explicit error, this is
   §1d's unrecognized-value-no-op shape reborn one layer up — a transition that *should* be
   blocked pending human approval silently completes instead. Requirements.md's Observability
   section already specifies "fail closed and loud... falling back to the default/built-in
   behavior rather than crashing or silently using zero/infinite thresholds" — the wording needs
   one addition for the gate case specifically: **for a *transition gate* (as opposed to a
   liveness threshold), "fail closed" must mean *block the transition*, not fall back to a
   default that might be "no gate."** A liveness threshold has a safe default (the built-in
   stage's threshold); a gate does not have an equally safe universal default — "fall back to
   built-in behavior" for a transition that has no built-in equivalent (a wholly custom
   transition) has no default to fall back *to*. Phase 3 planning should make this an explicit
   rule: an unresolvable gate on a transition is a hard block (transition refused, loud Warn,
   visible in the item detail UI's "what's blocking this" surface already required by Success
   Metrics), never an implicit pass.
2. **A malformed liveness definition silently blocks forever (the inverse failure)** — e.g. a
   stage's `expectedDuration` is missing/zero and the derived staleness threshold (per §1 above)
   computes to zero or a negative margin, or a stage-×-pipeline-mode join resolves to no row and
   the code doesn't know whether "no row" means "use default" or "no threshold configured, never
   flag." This is the *other* direction of the same "unrecognized value, ambiguous default" shape
   — instead of a check that should block silently passing, a check that should eventually fire
   never resolves and never fires, so the item sits forever with no operator signal (mirroring the
   many "item sits in `review`/`idea` forever with zero signal" findings already logged in that
   doc, e.g. the 2026-07-30/2026-08-03 entries about `PLAN_NOT_APPROVED` dead ends). Requirements'
   Observability section's Warn-log-on-fallback requirement covers the *logging* half; it does not
   by itself guarantee the *fallback value* is safe in both directions. Phase 3 planning must
   define, per config type (liveness threshold vs. gate requirement), which direction "fail safe"
   points — they are not the same direction, and a single generic "fall back to default" rule
   glossed over both cases would get one of them wrong.

**Is requirements.md's current Observability/Risk Control wording sufficient?** Directionally
yes — "fail closed and loud, never silent no-op or crash" is exactly right and already matches
this subsystem's established lesson (see `PipelineEngine`'s own `1d` guard, cited above, which
shipped this same fix already for mode strings). It is **not yet sufficient as written** because
it doesnn't yet distinguish "closed" for a liveness threshold (falls back to a safe *default
duration*) from "closed" for a gate requirement (falls back to *blocking*, since there is no safe
default gate state for an arbitrary custom transition) — Phase 3 planning should make this
distinction explicit rather than let "fail closed" get implemented as one generic code path that
happens to pick the wrong direction for one of the two config types.

---

## 5. Workflow-engine version drift (in-flight item references an edited/deleted stage or gate)

Not yet named anywhere in this codebase's bug history because no custom/editable state machine
has existed before this project — a genuinely new risk class here, not a recurrence.

**The concrete failure shape**: an item currently sitting in a custom stage, or with a pending
transition gated by a gate definition, has that stage/transition/gate definition edited or deleted
out from under it by the single operator (via the new management UI) while the item is in flight.
Three sub-cases, each needing an explicit answer in Phase 3 planning (this list is meant to sharpen,
not replace, the Open Questions section's existing "how does a stage vary by pipeline mode"
question, which is a related but narrower version of the same underlying "what does a config
identifier resolve to over time" question):

- **An item's current status references a deleted stage.** `AllowedTransitions(from)` and
  `CanTransition(from, to)` need a defined answer for `from` not existing in the current config at
  all — not just "unrecognized `to`." Given this project's own Risk Control requirement ("fail
  closed to default built-in behavior with a loud Warn"), the natural answer is: a deleted stage's
  in-flight items keep functioning under a *frozen snapshot* of that stage's definition (liveness
  + allowed transitions) captured at the time they entered it, rather than either erroring or
  silently reinterpreting `from` against nothing. This again points at the `AcSnapshot`
  precedent (§3's §4 transfer above) — the same "snapshot at entry" pattern likely needs to cover
  the *stage/transition/gate config itself*, not just AC and pipeline mode.
- **A transition's gate list changes between "gate evaluation started" and "gate evaluation
  resolved."** E.g. a human-approval gate is pending, the operator edits the transition to add a
  second gate (a structural check) after the human already approved. Does the already-recorded
  approval still count, or does the whole gate set need re-satisfaction? This is a variant of
  gate-evaluation idempotency (§6 below) but specifically about the *definition* changing mid-flight
  rather than the *item state* changing mid-flight — Phase 3 planning should pick one explicit rule
  (most conservative and easiest to reason about: any edit to a transition's gate list after gate
  evaluation has started for a given item resets that item's gate-satisfaction state for that
  transition) and document it, rather than leaving it to fall out of implementation-detail choices
  no one deliberately made.
- **The audit trail (`BacklogStatusEvent`) references a stage/transition ID that's since been
  deleted.** History/UI code that renders "item transitioned idea → custom-stage-X" must not
  break or blank out when custom-stage-X's definition later goes away — store enough of a
  human-readable snapshot (name, at minimum) on the event row itself rather than only a foreign
  key to a config table row that might not exist by the time anyone views the history. This is the
  same category of lesson as `ItemSessionData.AcSnapshot`: history must be immune to later
  config edits, not just later item-field edits.

---

## 6. Circular/unreachable transition graphs a UI-defined custom workflow could create

Nothing in the current fixed 9-state machine validates graph shape today because the graph is
hardcoded and was hand-verified once; a UI that lets an operator define arbitrary transitions
needs this validated at write time, not discovered live:

- **Unreachable stages** — a stage with no incoming transition from anything reachable from the
  existing start state(s) is a dead end nothing can ever enter; a stage with no outgoing
  transition (other than an intentional terminal state like `done`/`archived`) traps every item
  that reaches it. The management UI (or its backing RPC) should validate on save: every non-
  terminal stage has at least one outgoing transition, and every stage is reachable from at least
  one designated entry stage — reject the save (or warn loudly) rather than silently persisting
  a graph with a trap, mirroring this project's own "fail closed and loud" posture applied to
  *configuration validation* specifically, not just runtime resolution.
- **Cycles are not inherently wrong** (the existing graph almost certainly already has legitimate
  cycles — e.g. `review` → back to `in_progress` on FAIL) — the risk is not "a cycle exists" but
  "a cycle with no gate anywhere in it that can ever be satisfied," which produces an item that
  can loop forever consuming remediation/retry budget without ever making terminal progress. This
  is a live, named shape already: the 2026-09-03 update's "no escalation/different-approach retry
  once non-converging" finding for `orphaned_triage` (retrying an identical failing approach 5
  times) and the 2026-07-17 "bouncing" entry are both instances of *retry loops*, not transition-
  graph loops, but the design principle is the same — a custom transition graph should be
  checkable for "can this set of transitions + gates ever terminate for a well-behaved item," at
  least as a lint-style warning at definition time, since a live incident already proves this
  codebase's operators can and do create configurations that loop without escaping.
- **Practical validation scope for Milestone 2**: full graph-theoretic cycle/reachability analysis
  is a reasonable, bounded check to add to the stage/transition CRUD RPC (it's a small graph —
  dozens of nodes at most, not thousands) — this is squarely in scope of "structural integrity
  against self-inflicted misconfiguration," the security classification requirements.md already
  states for this whole project, not a new NFR.

---

## 7. Gate-evaluation ordering/idempotency

`session/review_gate.go`'s `ReviewGateRunner.Run` is the existing precedent to generalize (per
Rabbit Holes' explicit instruction to extend it, not build a parallel mechanism) — it already
handles several partial-state hazards for the one gate type it implements today: a worktree-
identity mismatch, branch drift, an empty diff, and a diff-computation failure are all checked
*before* invoking the LLM review call, each short-circuiting to a recorded terminal FAIL verdict
rather than letting a stale/wrong diff reach the reviewer. This is exactly the discipline a
generalized multi-gate model needs to preserve:

- **Re-evaluation after partial state changes.** If a transition requires multiple gates (e.g.
  "structural check AND human approval") and the structural check passes, then before the human
  approves, an item's underlying data changes such that the structural check would now fail (e.g.
  an AC gets unchecked, or a PR's CI goes red) — does the transition re-validate the structural
  check at the moment of the human's approval, or does a stale "structural check passed" result
  stand from when it was first evaluated? The safe default here mirrors `ReviewGateRunner`'s own
  pattern (recompute the diff/identity checks fresh, immediately before use, never trust a cached
  "checked earlier" result) — **all gates for a transition should be re-evaluated atomically at
  the moment the transition is actually attempted, not evaluated once and cached as "satisfied"
  indefinitely**, unless a gate is explicitly modeled as a one-time event (e.g. "a human clicked
  Approve" is inherently a point-in-time fact, not a re-checkable predicate, and should stay
  recorded rather than re-asked). This means the gate model needs to distinguish **stateless/
  re-checkable gates** (structural checks, automated review re-run) from **stateful/one-shot
  gates** (a recorded human approval) as a first-class distinction, not treat all four "Actors for
  Transition Gates" as the same shape.
- **Idempotency of a re-evaluated automated-review-verdict gate.** `session/review_gate.go`
  already has to guard against re-triggering a review on an item whose most recent verdict is
  already terminal (`recordTerminalReviewVerdict`, the duplicate-ref parsing at line 69 explicitly
  built to detect and route around a false-PASS regression). A generalized gate model reusing this
  mechanism for arbitrary custom transitions must preserve the same "don't re-run a review call
  that already produced a terminal verdict for the current diff" check — otherwise every poll/sweep
  that re-checks "is this transition's gate satisfied yet" risks re-triggering an expensive LLM
  call each tick instead of reading a cached terminal result, which is both a cost problem and a
  correctness problem if the recomputed verdict can legitimately differ run to run (LLM
  non-determinism) for what should be a one-time gate decision.
- **Partial multi-gate satisfaction and ordering.** If a transition has gates [A, B, C] and B fails
  after A already passed, does a retry re-check A too, or resume from B? Given the codebase's own
  established "don't trust a stale intermediate result, recompute what's cheap to recompute, cache
  what's expensive and terminal" split (visible in `ReviewGateRunner`'s structure — cheap
  mechanical checks recomputed every call, the expensive LLM verdict itself persisted and reused),
  the same split is the template: cheap/structural gates (an AC-done check, a CI-green check)
  should always re-evaluate fresh; expensive/stateful gates (a review call, a recorded human
  approval) should persist their terminal result and not redo the work. Phase 3 planning should
  make this split an explicit property of the gate-type taxonomy (Actors for Transition Gates),
  not an emergent accident of which gate types happen to be cheap or expensive to implement.

---

## 8. Clock/timezone pitfalls in staleness-threshold comparisons

**Existing code is already timezone-safe in the way that matters most**: every staleness/liveness
comparison surveyed (`backlog_lifecycle_triage.go:208`, `backlog_remediation.go`, multiple sites in
`backlog_service_triage.go`) uses `time.Since(x)`/`time.Now()` — Go's `time.Since` uses the
monotonic clock reading embedded in a `time.Time` when both operands carry one, so these
comparisons are immune to wall-clock adjustments (NTP step corrections, DST transitions) as long
as both timestamps originate from `time.Now()` on the same process. This is a genuine positive
finding, not a gap — the new liveness model does **not** need to invent new timezone-safety
machinery for pure elapsed-time comparisons within one process's lifetime, and should keep using
`time.Since`/`time.Now()` rather than introducing manual `time.Time` subtraction (`t2.Sub(t1)` on
two independently-sourced timestamps loses the monotonic reading if either has been serialized
through a DB round-trip — see below).

Where a real pitfall does exist for this project specifically:

- **DB round-trips strip the monotonic reading.** `CreatedAt`/`next_remediation_at`/etc. are
  persisted via ent and read back later (potentially by a different process, after a restart, or
  simply after enough time that the in-memory `time.Time` was never the same value) — a
  `time.Time` read back from the DB has no monotonic component, so any comparison against it uses
  wall-clock time only. This is already true today and already handled correctly (the sweep
  logic's `time.Since(latestTriage.CreatedAt)` pattern is comparing a DB-sourced timestamp against
  wall-clock `time.Now()`, which is the correct/only option once a timestamp round-trips through
  storage) — flagging it here only because the new liveness model, if it introduces a *duration*
  that's computed once and cached (e.g. a resolved "stage config as of item creation" snapshot)
  and later compared against a *fresh* wall-clock read, must not silently mix a monotonic-derived
  duration with a wall-clock timestamp is a bug; keep the existing pattern (always compare wall
  clock to wall clock via `time.Since`/`time.Sub` on DB-sourced times, never assume a monotonic
  reading survives).
- **Whichever timezone ent stores timestamps in must stay consistent app-wide.** This isn't a bug
  found in this codebase, but a check worth doing explicitly in Phase 3 rather than assuming: if
  the new stage/liveness config tables introduce a *scheduled* time field (unlikely given
  everything surveyed here is duration-based, not wall-clock-based, but worth ruling out
  explicitly since "expected duration" could tempt someone toward a wall-clock deadline field
  instead of a pure duration) — durations (`time.Duration`, stored as e.g. seconds/nanoseconds)
  sidestep timezone entirely and are the correct representation; a wall-clock "this must complete
  by HH:MM" field would not be, and nothing in the requirements suggests one is needed. Recommend
  Phase 3 planning explicitly constrain all new liveness fields to `time.Duration`-shaped values
  (expected duration, staleness threshold, margin), never wall-clock timestamps, to keep this
  entire class of pitfall structurally inapplicable.
- **DST/leap-second edge cases at long durations (7-day cold-retry interval, `remediationColdRetryInterval`
  from BUG-083) are already a non-issue for the same monotonic-vs-wall-clock reason** — a 7-day
  `next_remediation_at` deadline compared via wall-clock `time.Now()` after a DST transition is off
  by at most the DST shift (1 hour) relative to "wall clock days," which is immaterial at
  week-long granularity and already accepted behavior; no new handling needed for this project's
  comparable-or-shorter thresholds.

---

## Summary of recommendations for Phase 3 planning

1. Persist liveness as **one budget duration + a margin policy**, deriving the staleness threshold
   — never two independently-editable absolute thresholds (§1).
2. The liveness model must expose a **live query hook** ("is this actually still running"), not
   just a duration comparison — different work shapes (headless call vs. tmux session) need
   different liveness-check implementations, mirroring `IsTriageLive` vs. `hasActiveWorkSession`
   (§1, §4).
3. Explicitly test that a row **parked under old hardcoded thresholds** becomes due again via the
   existing `RemediationDue` cold-retry heartbeat once Milestone 1 ships, with no code change to
   the remediation gate itself required (§2) — the concrete regression check that would catch a
   7th "write-side-only" fix.
4. Audit and close the two confirmed `session.CanTransitionBacklog`-bypasses
   (`backlog_service_lifecycle.go:801`, `backlog_service_sync.go:121`) before
   `ConfiguredWorkflowEngine` ships — already documented as a live, dated gap, not a hypothetical
   (§3).
5. Split "fail closed" into two directions explicitly: a liveness threshold falls back to a safe
   *default duration*; a gate requirement that can't resolve falls back to *blocking the
   transition* — never the same generic fallback for both (§4).
6. Treat stage/transition/gate config the same way `AcSnapshot` treats acceptance criteria: capture
   a snapshot at the point an item enters a stage or a gate evaluation begins, and make historical
   audit rows immune to later config edits/deletions (§5).
7. Validate the transition graph at definition time (reachability, no orphaned terminal-less
   stages) as part of the management UI's save path, not left to be discovered by a stuck item in
   production (§6).
8. Split gate types into **stateless/re-checkable** (always recompute fresh at transition time) vs.
   **stateful/one-shot** (persist and never silently re-ask) as a first-class property of the gate
   taxonomy, following `ReviewGateRunner`'s existing precedent (§7).
9. Keep all new liveness fields `time.Duration`-shaped (never wall-clock deadlines), and keep using
   `time.Since`/`time.Now()` for in-process comparisons — the codebase's existing pattern is
   already timezone-safe; the only discipline needed is not breaking it (§8).
