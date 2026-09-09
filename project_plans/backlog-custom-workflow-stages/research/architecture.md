# Architecture Research: `ConfiguredWorkflowEngine` (Custom Stages + Liveness + Gates)

Research Agent 3 (Architecture) — SDD Phase 2, `backlog-custom-workflow-stages`.

## 0. Summary of recommendations

| Open Question | Recommendation |
|---|---|
| Liveness on `WorkflowEngine` or a sibling interface? | **Sibling** — new `LivenessEngine` interface, same separation pattern as `PipelineEngine`/`WorkflowEngine`. |
| Gates on `WorkflowEngine.ValidateGates` or a sibling? | **Extend `WorkflowEngine`**, don't add a sibling — but add one new method (`PendingGates`), not a parallel interface. `ValidateGates` becomes `len(PendingGates(...)) == 0`. |
| Liveness keyed by (stage), (stage×mode), or finer? | **(stage) by default, sparse (stage×mode) override** — a nullable-mode join, not a dense cross-product. Only 1 of ~12 `StuckReason`s needs the mode axis today. |
| One liveness "shape" or several? | **Three distinct shapes exist today**, not one shape with configurable numbers: duration-budget-plus-margin, heartbeat-staleness, and cycle-frequency. The schema must be a tagged union, not a single numeric field. |
| BUG-055's race — two numbers or one derived? | **One stored number (budget) + one margin; threshold = budget + margin is *derived*, never stored independently.** This is enforceable at the schema level, not just a convention. |
| Custom-check gate execution model | **A custom check is itself a "unit of work" resolved through the same `LivenessEngine` duration-budget shape** — narrow invocation contract (named skill/slash-command + `report_progress`-style pass/fail), not arbitrary code execution. |
| Does `review_gate.go`'s PASS/FAIL/UNVERIFIABLE flow generalize mechanically? | **The verdict domain logic (`ReviewOutcome`, `AggregateOutcome`, `CriterionVerdict`) is already status-agnostic and reusable as-is. The orchestration around it (`Run`, `handleReviewSessionExited`) is NOT — it hardcodes `BacklogStatusReview` as both "the stage name" and "the review trigger," and needs real rework to decouple those two meanings for a custom transition.** |
| `BacklogStatus` literal call sites | Surveyed below (§4). The two files requirements.md flagged by name (`review_gate.go`, `autonomous_driver.go`) turn out to have **zero** literal `BacklogStatus` branches — the real anchoring lives in `backlog_lifecycle_review.go`, `autonomous_orchestration_service.go`, and `server/mcp/tools_backlog.go`. |
| Migration safety | `BacklogStuckState` rows (`remediation_attempts`, `next_remediation_at`) are keyed by `(item_id, reason)` with `reason` a **plain unvalidated string** — completely decoupled from the stage/liveness config tables this project adds. No schema migration touches them. The only real risk is a *threshold value* drifting during the hardcoded→configured port, which a characterization-test gate (§6) catches mechanically. |

---

## 1. Liveness shapes: full survey of all ~12 `StuckReason`s

Surveyed `session/domain/backlog.go` (the `StuckReason` enum, [session/domain/backlog.go:36-235](session/domain/backlog.go)), every `reconcile*` function in `session/backlog_lifecycle*.go`, and `session/backlog_remediation.go`. Finding: **there is no single "liveness shape."** Three structurally different detection mechanisms exist, plus a fourth axis (remediation cadence) that is orthogonal to all three and must not be conflated with them.

### Shape A — Duration-budget-plus-margin (headless, no live process to poll)

**Example: `orphaned_triage`.** A headless LLM call has no tmux pane to check — `reconcileOrphanedTriageItems` ([session/backlog_lifecycle_triage.go:169](session/backlog_lifecycle_triage.go)) can only reason about elapsed wall-clock time plus a liveness callback (`TriageRespawner.IsTriageLive`, [session/backlog_lifecycle_triage.go:39](session/backlog_lifecycle_triage.go)). Two numbers are load-bearing and **must stay in a strict inequality with margin**:

- `triageCallBudget` = 30m ([server/services/backlog_service_triage.go:434](server/services/backlog_service_triage.go)) — the call's own timeout.
- `maxHeadlessTriageSessionStaleness` = 35m ([session/backlog_lifecycle_triage.go:64](session/backlog_lifecycle_triage.go)) — the sweep's "this is dead" threshold, with an explicit code comment: "MUST stay strictly greater than `triageCallBudget`... with real margin."

**This is BUG-055 and also the project's own motivating bug**: the "sdd" pipeline mode's triage prompt needs a much larger budget than default, but both numbers are flat, pipeline-mode-blind constants (`docs/tasks/backlog-feature-improvement.md`'s 2026-09-03 update, confirmed live: 12 items parked in `ORPHANED_TRIAGE`, sessions clustering at 30m00s-30m03s with `endReason: timeout`).

### Shape B — Heartbeat staleness (live process, progress timestamp)

**Examples: `stale_work`, `rework_blocked_stale`.** An interactive/autonomous tmux-backed session IS pollable — the question isn't "is this dead" (a liveness callback can answer that directly) but "has it stalled while still alive" (`session/backlog_lifecycle_stale.go:12-33`'s doc comment: "the underlying tmux session and pane process are still alive... the agent inside simply finished its own work and is idle"). Threshold is compared against a **last-progress timestamp**, not call start time:

- `maxWorkSessionStaleness` = 2h ([session/backlog_lifecycle_stale.go:62](session/backlog_lifecycle_stale.go)) for `stale_work` (in_progress).
- `maxReworkBlockStaleness` = 15m (cited in `StuckReasonReworkBlockedStale`'s doc comment, [session/domain/backlog.go:108-122](session/domain/backlog.go)) for the review-status case — same shape, different status, different threshold, deliberately NOT merged with `stale_work` ("different item status, different threshold, different urgency").

There is no "call budget" here at all — the work has no fixed expected duration, only a no-progress ceiling. Trying to force this into Shape A's budget+margin schema would be a category error.

### Shape C — Cycle frequency over a lookback window (not a timeout at all)

**Example: `bouncing`.** `reconcileBouncingItems` ([session/backlog_lifecycle.go:1551](session/backlog_lifecycle.go)) counts `in_progress<->review` transitions within `bounceLookback` and compares against `bounceThreshold` (`isBouncing(count, hasPass)`, [session/backlog_lifecycle.go:1658](session/backlog_lifecycle.go)) — no elapsed-time-since-start or elapsed-time-since-progress check anywhere in this detector. "Liveness" for this reason means "is the process converging," measured by iteration count, not duration.

### Axis D — Remediation cadence (orthogonal, NOT a liveness shape, and already fine as a single global config)

`session/backlog_remediation.go`'s `remediationBackoffSchedule` (30m/2h/8h/24h/72h, [session/backlog_remediation.go:31-37](session/backlog_remediation.go)), `MaxRemediationAttempts` (=5, derived from the schedule length), and `remediationColdRetryInterval` (7 days, BUG-083, [session/backlog_remediation.go:85](session/backlog_remediation.go)) answer a completely different question: **once something IS marked stuck, how often may an automated retry fire?** Every one of the ~12 `StuckReason`s shares the identical global schedule via `RemediationDue` ([session/backlog_remediation.go:265](session/backlog_remediation.go)) — there is no evidence any reason needs a different backoff cadence, and the requirements' bug never asks for one (the bug is entirely "the *initial staleness threshold* used to decide stuck-or-not is wrong for sdd-mode," not "the retry cadence is wrong").

**Recommendation: do not fold Axis D into the new per-stage liveness model.** Requirements.md names `backlog_remediation.go` as a consumer of the liveness model, but the evidence shows only the *staleness-threshold* axis (A/B/C above) needs to move; the backoff-schedule axis can stay a single global constant, unchanged. Treating them as one thing would be exactly the "let it get decided implicitly by whatever's easiest to schema first" mistake the Rabbit Holes section warns against for the (stage) vs (stage×mode) question — here the equivalent mistake is conflating "how long before I call this stuck" with "how often do I retry a thing already known to be stuck." Keeping Axis D untouched also directly de-risks migration (§6): `BacklogStuckState`'s `remediation_attempts`/`next_remediation_at` columns need zero schema or semantic change.

### Consequence for the data model

`LivenessDefinition` must be a **tagged union** (Go: an interface or a struct with a `Kind` discriminator and kind-specific fields, not a flat "one duration field"), with at least these variants:

```go
type LivenessKind string
const (
    LivenessKindDurationBudget LivenessKind = "duration_budget" // Shape A
    LivenessKindHeartbeat      LivenessKind = "heartbeat"        // Shape B
    LivenessKindCycleFrequency LivenessKind = "cycle_frequency"  // Shape C
)

type LivenessDefinition struct {
    Kind LivenessKind
    // Shape A fields:
    ExpectedDuration time.Duration // e.g. triageCallBudget
    StalenessMargin  time.Duration // sweep threshold = ExpectedDuration + StalenessMargin, ALWAYS derived (BUG-055)
    // Shape B fields:
    MaxNoProgressDuration time.Duration // e.g. maxWorkSessionStaleness
    // Shape C fields:
    CycleThreshold int
    CycleLookback  time.Duration
}
```

The derived-threshold requirement for Shape A is the concrete, schema-level answer to the BUG-055 rabbit hole: `StalenessThreshold()` is a **method**, `ExpectedDuration + StalenessMargin`, never a second independently-settable column. A UI that only exposes `ExpectedDuration` and `StalenessMargin` (never a raw "sweep threshold" field) makes the inconsistent-numbers failure mode structurally unreachable, not just discouraged by convention.

---

## 2. Granularity: (stage) alone, (stage × pipeline-mode), or finer

Evidence from §1: **11 of ~12 `StuckReason`s need only (stage) granularity.** Only `orphaned_triage`'s Shape-A budget varies by pipeline mode (`sdd` vs. default) — because `PipelineEngine` (the sibling project) already lets the *content* of triage vary by mode, and content volume drives call duration. No other reason's threshold has any evidence of needing per-mode variation: `bouncing`'s cycle count/lookback, `stale_work`'s 2h heartbeat, and `rework_blocked_stale`'s 15m heartbeat are all mode-independent by their own nature (they measure interactive-session behavior, not a single bounded LLM call whose size PipelineEngine controls).

**Recommendation: a sparse override, not a dense (stage × mode) cross-product table.**

```
stage_liveness_definitions
  id, stage_id (FK), pipeline_mode (nullable string, NULL = "applies to all modes"), kind, ...kind-specific fields...
  UNIQUE(stage_id, pipeline_mode)
```

Resolution: look up `(stage_id, pipeline_mode)` first; fall back to `(stage_id, NULL)` if no mode-specific row exists. This is the same "empty-string mode short-circuits, only a real override touches anything else" convention `PipelineEngine` already established (`PipelineModeDefault = ""`, `research/architecture.md` §1 of the sibling project) — reused here rather than reinvented, per the Constraints section's explicit instruction to follow that project's precedent. It also means **10 of ~11 built-in-stage liveness rows never need a mode column populated at all** — the common case costs nothing extra, and the schema doesn't force an operator to define N mode-rows for every stage just to change one.

This resolves Feasibility Risk's "may need a third axis" concern conservatively: the schema supports arbitrary future (stage, mode) pairs without needing a NEW join table if a third axis is ever discovered, because the tagged-union `Kind` field (§1) is orthogonal to the (stage, mode) key — a future finer-grained axis would be a new nullable column on the same table, not a schema rewrite.

---

## 3. `WorkflowEngine` interface evolution

### 3a. Liveness: sibling interface, not an extension

Mirrors the sibling project's own settled precedent almost exactly. `research/architecture.md` §1 of `backlog-configurable-pipeline` already argued (and shipped) that `PipelineEngine` must NOT be folded into `WorkflowEngine` because their consumer sets are disjoint and their reasons to change are independent. The same argument applies here with equal force:

- **Disjoint consumers**: `WorkflowEngine.CanTransition`/`ValidateGates` are consumed by `TransitionBacklogItemStatus` (the synchronous, transition-time path). A liveness resolver is consumed by the periodic `reconcile*` sweeps (`session/backlog_lifecycle*.go`) and (per Axis D's exclusion above) NOT by `RemediationDue` itself. These are different call sites triggered by different clocks (a status-change request vs. a ~60s periodic tick).
- **Independent evolution**: `ConfiguredWorkflowEngine`'s custom states (this project's other half) does not require knowing anything about liveness to answer "is this edge legal" — exactly as `PipelineEngine` doesn't need `WorkflowEngine` to answer "which prompt do I use."
- **No cross-calls needed**: nothing in `WorkflowEngine.CanTransition` needs a liveness value, and nothing in liveness resolution needs to ask `WorkflowEngine` whether a transition is legal (a `reconcile*` sweep already knows the item's current status directly from the loaded `BacklogItemData`).

```go
// session/liveness_engine.go
type LivenessEngine interface {
    // LivenessFor resolves the liveness definition for a stage, given the
    // item's current pipeline mode (empty string = default/no override).
    // Falls back to the stage's mode-less row, then to a built-in default,
    // per the fail-closed-and-loud contract in Observability Requirements.
    LivenessFor(stage BacklogStatus, mode PipelineMode) (LivenessDefinition, error)
}
```

`BacklogService` and `BacklogLifecycleListener` each gain a `livenessEngine session.LivenessEngine` field, wired at startup exactly like `pipelineEngine` (`server/dependencies.go`'s existing pattern). `DefaultLivenessEngine` reproduces every hardcoded constant surveyed in §1 verbatim (zero-regression requirement).

### 3b. Gates: extend `WorkflowEngine`, don't add a sibling — but add exactly one method

This is the one place the PipelineEngine precedent does NOT transfer cleanly, because gates and transition-legality are the *same question* WorkflowEngine already owns, not a disjoint one. `ValidateGates(item, to) error` already IS "evaluate whatever business rules gate this transition" — `ConfiguredWorkflowEngine`'s job is to make the *rule source* dynamic (a DB-configured gate list instead of `TransitionGuard`'s hardcoded switch, [session/backlog.go:541](session/backlog.go)), not to introduce a new kind of question.

The one real gap: `ValidateGates` returns only `error` — pass/fail — with no structured "which gate(s), who/what can satisfy them" data, which Success Metrics explicitly requires for the item-detail UI ("shows... which gate(s) are blocking it and who/what can satisfy each one"). Recommend adding exactly one new method rather than a parallel interface:

```go
type GateStatus struct {
    GateID      string
    Kind        GateKind // human_approval | automated_review | structural | custom
    Satisfied   bool
    Description string // e.g. "Awaiting PASS verdict from automated review"
    ActionHint  string // e.g. "Use the Approve button" — populated for human_approval only
}

type WorkflowEngine interface {
    CanTransition(from, to BacklogStatus) bool
    // PendingGates returns every gate attached to the from->to transition
    // with its current satisfaction state. ValidateGates becomes a thin
    // wrapper: len(unsatisfied PendingGates) == 0.
    PendingGates(item BacklogItemTransitionInput, to BacklogStatus) ([]GateStatus, error)
    ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error
    AllowedTransitions(from BacklogStatus) []BacklogStatus
}
```

This keeps the interface at 4 methods (not 3+a whole second interface), stays inside the "narrow, consumer-defined" guidance (`.claude/rules/interface-pollution-checklist.md`), and — critically — `ValidateGates`'s existing call sites (`TransitionBacklogItemStatus`, `GuardedTransitionAllowed`, [session/workflow_engine.go:71](session/workflow_engine.go)) need no change at all; only the item-detail UI's new "what's blocking this" panel needs the new method.

### 3c. Gate *initiation* (side effects) is a third, separate concern — deliberately left OUT of both interfaces

`PendingGates`/`ValidateGates` are pure evaluators: given current recorded state (a stored approval record, a stored review verdict, a structural fact about the item), do the gates pass? They must NOT be the thing that spawns a review session or creates a pending-approval UI affordance — that is a side-effecting action, triggered once (when a work session ends and the item is about to attempt a gated transition), not on every `ValidateGates` read. This mirrors the existing split in `review_gate.go`: `Run` (spawn, side-effecting, called once) vs. the eventual `submit_review_verdict`-driven outcome (pure state, read by whatever gate-checks it later). Recommend keeping this initiation logic where equivalent logic already lives today (`ReviewGateRunner`, `server/services/backlog_service_lifecycle.go`'s transition handler) rather than inventing a fourth interface — there is no evidence yet of enough independent initiation-consumers to justify one, and adding a speculative interface here would be exactly the anti-pattern the interface-pollution checklist flags.

---

## 4. `BacklogStatus` literal call-site survey

Requirements.md specifically flagged `session/review_gate.go` and `session/autonomous_driver.go` for this survey (`sg --pattern 'BacklogStatus$STATUS' --lang go` and a full-codebase grep for `BacklogStatus[A-Z]` in non-test `.go` files). Result:

| File | Literal `BacklogStatus` usage? | What it does |
|---|---|---|
| `session/review_gate.go` | **None** (confirmed: zero matches) | `Run` spawns a review session generically; it never branches on status. |
| `session/autonomous_driver.go` | **None** (confirmed: zero matches) | Confirms the sibling project's own finding (`research/architecture.md` §2 of `backlog-configurable-pipeline`): the driver treats `goal` as an opaque string and switches only on `detection.Status*`/orchestrator keywords (`DONE`/`WAIT`), never on `BacklogStatus`. |
| `session/backlog_lifecycle_review.go` | Yes — `BacklogStatusReview` used as a **guard/precondition** in 4 places ([:200](session/backlog_lifecycle_review.go), [:441](session/backlog_lifecycle_review.go), [:545](session/backlog_lifecycle_review.go), [:607](session/backlog_lifecycle_review.go)) | Checks "is this item still in `review`" before acting — anchors `abandoned_review` detection and `handleReviewSessionExited`'s downstream logic to the literal built-in "review" stage. |
| `server/services/autonomous_orchestration_service.go` (`onAutonomousDriverComplete`, complexity 42, the subsystem's highest-complexity function per the prior audit) | Yes — `session.BacklogStatusReady`/`BacklogStatusIdea`/`BacklogStatusInProgress`/`BacklogStatusReview` ([:346-347](server/services/autonomous_orchestration_service.go), [:438](server/services/autonomous_orchestration_service.go), [:463](server/services/autonomous_orchestration_service.go)) | Branches primarily on `is.Role` (triage/work/review — a session-role axis, orthogonal to stage), but the triage-success branch **hardcodes `toStatus = session.BacklogStatusReady`** as the literal successor of a triage-role session — this is the one genuine coupling point: a custom stage that hosts a triage-role session has no way to say what its own successor stage is without going through `WorkflowEngine.AllowedTransitions`/a dedicated "post-work transition" helper (exactly what ADR-013 §"Decision" already anticipated for `BacklogLifecycleListener.onSessionExited`, never implemented). |
| `server/mcp/tools_backlog.go` | Yes — `validBacklogStatuses` ([:147-157](server/mcp/tools_backlog.go)), `allowedSelfResolveSourceStatuses`, `unclaimedDuplicateSourceStatuses`, `reportPRCreatedAllowedSourceStatuses` ([:210-239](server/mcp/tools_backlog.go)) | Hardcoded whitelists of literal statuses gating which MCP tools (`request_review`, `report_duplicate`, `report_pr_created`) apply to which stage. These are the "which stages behave like 'in progress' / 'review' / 'unclaimed'" semantic maps — a custom stage gets NONE of this tool behavior automatically. |
| `server/services/backlog_service_lifecycle.go`, `backlog_service_ship.go`, `deep_link_resolver.go`, `chain_firer.go` | Yes, scattered | Mostly single-status precondition checks (`if to == session.BacklogStatusDone`, [:710](server/services/backlog_service_lifecycle.go)) — narrower, more mechanical instances of the same pattern. |

**Key finding, not previously stated in requirements.md**: the review-gate/stuck-detection/MCP-tool machinery is not anchored to the *transition graph* (which `ConfiguredWorkflowEngine` generalizes) — it's anchored to specific **literal status string constants that also carry implicit semantic roles** ("this string means 'the review stage'," "this string means 'unclaimed'"). Adding a custom stage to the graph does not automatically make it behave like `review` or `in_progress` for review-gate spawning, MCP-tool eligibility, or stuck-detection — those all need to consult the *transition-gate model* (§3b/§5) explicitly, because the built-in detectors will never recognize an arbitrary new stage name. This is a real scope boundary Phase 3 planning must state explicitly: **a custom stage is a full graph citizen for `CanTransition`/liveness, but does NOT inherit `review`/`in_progress`-shaped built-in behaviors unless a gate is attached to give it equivalent behavior.** This is consistent with — and gives a concrete mechanism for — the Scope section's own boundary ("this project decides which stages exist... not... the content of individual stages' work").

### Does `review_gate.go` generalize mechanically?

**Partially.** The pure verdict domain — `domain.ReviewOutcome`, `AggregateOutcome`, `CriterionVerdict` (all in [session/domain/backlog.go:343-413](session/domain/backlog.go)) — is already 100% status-agnostic: it operates on criteria and verdicts, never on `BacklogStatus`. This part is directly reusable by an automated-review-verdict gate on any custom transition with zero changes.

The orchestration is NOT mechanically reusable as-is:
- `ReviewGateRunner.Run` ([session/review_gate.go:106](session/review_gate.go)) is invoked from call sites that already know the item is entering (or already in) the literal `review` status.
- `handleReviewSessionExited` ([session/backlog_lifecycle_review.go:69](session/backlog_lifecycle_review.go)) hardcodes what a PASS verdict does next (`pushAndCreatePR`, i.e., drive toward `pr_pending`) — there is no notion of "which transition is this verdict gating" as a parameter.

For a custom transition's automated-review gate, the same spawn-a-review/record-a-verdict machinery needs a **transition-gate context** threaded through (which gate ID this verdict is for, which transition it should unblock on PASS) instead of assuming "review is the stage, pr_pending is next." This is real, not cosmetic, rework — budget it as such in Phase 3 planning rather than assuming a signature-compatible drop-in.

---

## 5. Custom/pluggable gate check execution — bounding via the liveness primitive

Per the Rabbit Holes section's explicit instruction: a custom check is itself a bounded unit of work, and must reuse Shape A (duration-budget-plus-margin) from §1, not invent a third timeout mechanism. Concretely:

- A custom gate check is defined as: **invoke a named skill/slash-command** (matching the narrowing requirements.md itself suggests: "invoke this named skill/slash-command, treat exit 0 / a specific `report_progress`-style call as pass") with a `LivenessDefinition{Kind: LivenessKindDurationBudget, ExpectedDuration: <configured>, StalenessMargin: <configured>}` resolved exactly like a headless triage call's budget.
- The check runs the same way a headless triage/review call already runs today (`TriggerTriage`'s `context.WithTimeout(s.shutdownCtx, triageCallBudget)` pattern, [server/services/backlog_service_triage.go:2770](server/services/backlog_service_triage.go)) — bounded by `ExpectedDuration`, with the periodic sweep applying `ExpectedDuration + StalenessMargin` as its own "this looks dead" threshold, using the identical `LivenessEngine.LivenessFor` resolution path as every other Shape-A reason. No new timeout primitive, no new sweep, no new StuckReason-shaped detector needs to be invented — a custom gate's stuck row is just another `LivenessEngine` consumer.
- **Execution surface is closed, not open**: "arbitrary code execution" is explicitly rejected. The only thing a custom check's config may name is an existing skill/slash-command identifier already reachable through this codebase's existing skill-invocation surface — it cannot supply a shell command, script path, or URL. This directly satisfies the "single largest scope-blowout risk" warning by making the check content a reference into an already-reviewed, already-sandboxed set of skills, not new arbitrary execution.
- Reported outcome: reuse `domain.ReviewOutcome`'s PASS/FAIL/UNVERIFIABLE shape (§4) for the verdict, so a custom gate's result renders through the exact same UI/verdict-aggregation code as an automated-review gate, rather than a bespoke boolean.

---

## 6. Migration safety

Traced `session/ent/schema/backlog_stuck_state.go:1-60`: `BacklogStuckState.reason` is `field.String("reason")` — a **plain, unvalidated-at-the-DB-layer string**, validated only in Go via `domain.StuckReason.IsValid()` ([session/domain/backlog.go:238](session/domain/backlog.go)). The row is keyed by `(item_id, reason)` via a 2-column unique index, with `remediation_attempts`/`next_remediation_at` as plain columns on that same row (BUG-083's cold-retry heartbeat, [session/backlog_remediation.go](session/backlog_remediation.go)).

**Consequence: this project's new stage/liveness/gate config tables need zero foreign key, zero migration, and zero schema touch on `BacklogStuckState` at all.** The 12 currently-parked `orphaned_triage` rows (and any other live stuck row) are completely decoupled from the stage-config schema change — nothing about adding `stage_liveness_definitions` or a `custom_transitions` table requires touching `backlog_stuck_states`. This is a materially lower migration-risk shape than a naive reading of "migrating StuckReason thresholds onto the new model" might suggest — there is no row-by-row data migration of `BacklogStuckState` at all, only a **code-path migration**: `reconcileOrphanedTriageItems` and friends switch from reading a Go constant to calling `livenessEngine.LivenessFor(stage, mode)`.

The real risk is narrower and mechanical: **does the resolved value match the hardcoded constant, bit-for-bit, for every built-in stage with no configuration set?** Recommended gate (this is the concrete mechanism for the Risk Control section's "bit-for-bit unchanged" requirement, and should be a named Phase 4 task, not left implicit):

1. A characterization test that captures every `reconcile*` sweep's stuck/not-stuck decision for a fixed corpus of item-state fixtures (one per `StuckReason`, covering each of the three shapes in §1) **before** the `DefaultLivenessEngine` swap.
2. Re-run the identical corpus **after** the swap to `DefaultLivenessEngine` and assert byte-identical decisions (same threshold values, same stuck/not-stuck verdicts, same `reasonDetail` strings where they embed the threshold — e.g. `reconcileOrphanedTriageItems`'s `"triage session %s still open after %s"` interpolates the constant directly, [session/backlog_lifecycle_triage.go:232](session/backlog_lifecycle_triage.go), so a wrong migrated value would visibly corrupt operator-facing text too, not just silently change timing).
3. Only after this gate passes does Milestone 1's actual fix (a real, non-default `sdd`-mode override for the `idea` stage) get configured — at which point the 12 parked items recover via BUG-083's existing 7-day cold-retry heartbeat (or an operator-triggered Reset) with **no manual per-row intervention** and no `BacklogStuckState` write of any kind from this migration — exactly as Success Metrics' Milestone 1 describes.

**Fail-closed requirement (Risk Control, Observability Requirements) is the second half of migration safety**: `LivenessEngine.LivenessFor` must fall back to the built-in default (reproducing the pre-migration constant) with a Warn log whenever a stage or liveness row is unresolvable/malformed — this is what prevents a partial or buggy rollout of the config tables from ever producing a zero/infinite threshold on a live item, which would either instantly tombstone everything in that stage or never detect staleness again. Both failure directions are worse than "identical to today," so the fallback target must always be the literal hardcoded value from §1's `DefaultLivenessEngine`, not a schema-level default like `0` or `NULL`.

---

## 7. Event-Command-Policy table (core transition-gate and liveness-detection flows)

Using EventStorming grammar (`Domain Event | Policy trigger | Command | Actor/System`):

| Domain Event | Policy (Whenever…Then) | Command | Actor/System |
|---|---|---|---|
| `WorkSessionEnded{role: triage, outcome: done}` | Whenever a triage-role session completes successfully, then resolve the item's successor stage via `WorkflowEngine.AllowedTransitions`/a dedicated post-work-transition helper instead of the literal `BacklogStatusReady` hardcode (§4) | `TransitionBacklogItemStatus(item, resolvedSuccessor)` | Automated (autonomous driver / `TriggerTriage`'s own completion path) |
| `TransitionAttempted{from, to}` | Whenever a transition is attempted, then evaluate every attached gate via `WorkflowEngine.PendingGates` | `PendingGates(item, to)` | System (`BacklogService.TransitionBacklogItemStatus`) |
| `GateStatus{kind: human_approval, satisfied: false}` returned | Whenever a pending human-approval gate exists and no prior approval record does, then surface a generic approve/reject affordance in the item-detail UI | *(no command yet — UI display only)* | System → Human reviewer |
| Human clicks "Approve" | Whenever a human approves an outstanding gate, then record the approval and re-attempt the transition | `RecordGateApproval(item, gateID)` → `TransitionBacklogItemStatus(item, to)` | Human reviewer |
| `TransitionAttempted{to: <gated by automated-review>}` and no verdict on record | Whenever an automated-review gate has no recorded verdict yet, then initiate a review session (reusing `ReviewGateRunner`'s spawn mechanism, generalized per §4) | `ReviewGateRunner.Run(item, gateContext)` | Automated reviewer (LLM headless review call) |
| `submit_review_verdict` called | Whenever a review session records a verdict, then aggregate criteria via `AggregateOutcome` and store the gate's satisfaction state | `RecordGateVerdict(item, gateID, outcome)` | Automated reviewer → System |
| `GateStatus{kind: structural, satisfied: false}` | Whenever a structural check (AC-complete, PR-green, no open BLOCKERs) fails, then block the transition with a specific, checkable reason (no session spawn) | *(pure evaluation, no command)* | System (structural/mechanical check) |
| `TransitionAttempted{to: <gated by custom check>}` and no verdict on record | Whenever a custom/pluggable gate has no recorded verdict yet, then invoke the named skill/slash-command bounded by `LivenessEngine`'s duration-budget shape (§5) | `InvokeCustomGateCheck(item, gateID, livenessDef)` | Custom/pluggable check (bounded skill invocation) |
| Custom check call exceeds `ExpectedDuration + StalenessMargin` with no liveness signal | Whenever the periodic sweep finds an open custom-gate check past its derived staleness threshold, then mark it stuck exactly like `orphaned_triage` does today | `MarkStuck(item, <new StuckReason or generic gate-timeout reason>, ...)` | Stuck-detection sweep |
| `BacklogStuckState` row marked stuck | Whenever `RemediationDue`'s backoff gate (Axis D, unchanged) says an attempt is due | `RemediationDue(itemID, reason)` → reason-specific respawn action | Remediation backoff gate |
| Item's active work session reports no progress past the resolved `LivenessDefinition`'s heartbeat threshold (Shape B) | Whenever `reconcileStaleWorkSessions`/`reconcileReworkBlockedStaleResolution` ticks and finds staleness, then `MarkStuck` and (for `stale_work` only) dispatch `StaleWorkRemediator` | `MarkStuck(item, stale_work\|rework_blocked_stale, ...)` → `RemediateStaleWorkSession` (stale_work only) | Stuck-detection sweep → automated remediator |
| Item bounces `in_progress<->review` past `CycleThreshold` within `CycleLookback` (Shape C) | Whenever `reconcileBouncingItems` ticks and the PR/commit is NOT already shipped, then `MarkStuck(bouncing)` | `MarkStuck(item, bouncing, ...)` | Stuck-detection sweep |

---

## 8. Constraint compliance check

- **Narrow-interface + deep-copy-on-construct pattern reused**: `LivenessEngine` follows `WorkflowEngine`/`PipelineEngine`'s exact shape (single-purpose interface, `Default*` implementation constructed once, deep-copied config). ✓
- **No duplication of `PipelineEngine`**: `LivenessEngine.LivenessFor` takes a `PipelineMode` as an input parameter (read-only) but never calls into `PipelineEngine` or vice versa — it only needs the mode's *identity* (a string) for its sparse-override lookup key, not any of `PipelineEngine`'s content-resolution behavior. ✓
- **`interface-pollution-checklist.md`**: `WorkflowEngine` grows by exactly one method (`PendingGates`); no new interface is added for gates; `LivenessEngine` is justified by a genuinely disjoint consumer set, matching the sibling project's own justification bar exactly (§3a). ✓
- **`BacklogItemUpdate`/`*T` optional-field pattern**: any new item-level field this project needs (e.g., a recorded gate-approval reference) should follow the existing `*string`/`*bool` pointer convention `session/repository.go` already uses for `SkipReviewGate`/`PipelineMode`, not a bare non-optional type. (No new item-level field is strictly required by the architecture above — gate/approval state can live on new tables keyed by item ID + gate ID — but Phase 3 planning should confirm this before adding one.)
