# Research: Feature Landscape for Custom Workflow Stages, Per-Stage Liveness, and Transition Gates

Builds directly on `project_plans/backlog-configurable-pipeline/research/features.md` ("the sibling
doc") — not re-derived. Where a finding there already answers something here, it's cited, not
repeated in full.

## 1. `WorkflowEngine` today: the seam to extend, and its actual blast radius

`session/workflow_engine.go` (76 lines) is a 3-method interface —
`CanTransition(from,to)`, `ValidateGates(item,to)`, `AllowedTransitions(from)` — implemented once
today by `DefaultWorkflowEngine`, which deep-copies `validTransitions` (a
`map[BacklogStatus]map[BacklogStatus]bool`, `session/backlog.go:210`, sourced from
`domain.ValidTransitions()`) at construction and delegates gate checks to the free function
`domain.TransitionGuard` (`session/domain/backlog.go:541-614`). This is exactly the sibling doc's
§1d "closest prior art for a pluggable policy engine," and it succeeded once already
(ADR-013 Phase 1) using the wrap-don't-replace strategy this project is instructed to reuse.

**What `TransitionGuard` actually encodes** (the thing "transition gates" must generalize): five
hardcoded `switch` arms, each checking 1-3 boolean/string conditions on `BacklogItemTransitionInput`
(a flat struct: `Status`, `AcCriteria`, `PlanApproved`, `SkipPlanning`, `PlanArtifactsPath`,
`OverrideReason`, `OverallOutcome`, `HasUnshippedCode`, `HasUnresolvedBlockers`) — e.g.
idea→ready requires non-empty `AcCriteria`; ready/queued→in_progress requires
`PlanApproved || SkipPlanning` plus no unresolved blockers; →done requires
`OverallOutcome == ReviewOutcomePass` (or a manual `OverrideReason`) plus no unshipped code. Every
gate here is what requirements.md calls a **structural/mechanical check** — a synchronous predicate
over already-persisted item state, never a live-in-progress verdict. This is a key asymmetry vs.
the **automated-review-verdict** gate type (§3 below): today's structural gates block a
transition attempt inline (return an error, no state change), but the review-verdict "gate" is
resolved asynchronously by a separate live session calling `submit_review_verdict` — a materially
different shape a generalized gate model must support (see Rabbit Holes discussion, §5).

**Literal `BacklogStatus` branch sites outside `backlog.go`/`workflow_engine.go` itself** (answers
requirements.md's Open Question "does adding custom stages require changes to `ReviewGateRunner`
and `AutonomousDriver`, which today assume the fixed 9-state set"): a grep for `case BacklogStatus`
/ `== BacklogStatus` across `session/*.go` and `server/services/*.go` found 7 sites beyond the
transition table itself:
- `session/backlog_sync.go:535` — `case BacklogStatusIdea, ...Refining, ...Ready, ...Queued:` (a
  GitHub-sync status-bucketing switch)
- `session/backlog_lifecycle.go:798` — `toStatus == BacklogStatusReview && !item.SkipReviewGate` (the
  review-gate spawn condition — this is the literal call site that decides whether an automated
  review-verdict gate fires today, and it is hardcoded to fire only on the built-in `review` status)
- `session/backlog_lifecycle.go:1781` — terminal-status check (`Done || Archived`)
- `session/backlog_lifecycle_pr.go:1479` — `== BacklogStatusPRPending` (drives `ReconcilePRPending`'s
  polling scope)
- `session/ent_repository_backlog.go:1408,1468` — `toStatus == BacklogStatusDone` (chain-workflow
  triggering and a terminal-status side effect)

None of these are in `review_gate.go` or `autonomous_driver.go` proper — `ReviewGateRunner.Run` and
`AutonomousDriver`'s orchestration loop are status-agnostic once *invoked*; it's the **caller**
(`backlog_lifecycle.go:798`) that hardcodes "review-gate fires only when `toStatus ==
BacklogStatusReview`." This means a custom transition with an automated-review-verdict gate needs a
new, generalized version of that one call site (turn it into "does the *target status/transition*
have a review-verdict gate attached" rather than a literal status equality), not changes scattered
through `review_gate.go` itself — a narrower blast radius than the Feasibility Risk implied, but the
generalization is still real work since today's single call site assumes exactly one review-gated
transition exists in the whole state machine.

## 2. `review_gate.go`: the existing automated-review-verdict gate, and what's reusable vs. not

`ReviewGateRunner` (`session/review_gate.go`) is already `PipelineEngine`-aware — its
`reviewPromptFor` method (line 90) calls `pipelineEngine.InteractiveReviewPromptFor(...)` when a
`PipelineEngine` is wired, falling back to `BuildReviewPrompt` otherwise. This is directly relevant
to requirements.md's last Open Question ("which `PipelineEngine`-selected review prompt runs for a
custom transition's review gate") — the mechanism to select a mode-specific review prompt **already
exists and is already wired into the review-gate runner**; a custom-transition review gate reusing
it is a matter of calling the same method with a different target status context, not inventing new
plumbing.

**Reusable as-is**: the PASS/FAIL/PARTIAL/UNVERIFIABLE verdict vocabulary
(`domain.ReviewOutcome*`), the `submit_review_verdict` MCP tool → `handleReviewSessionExited`
async-completion pattern, and `AggregateOutcome` (multi-criterion verdict rollup).

**Not mechanically reusable, needs generalization**: `Run`'s hardcoded assumptions that the target
status is always `review`→`pr_pending`-shaped — e.g. it calls `pushAndCreatePR` semantics implicitly
via `onPass`'s original callers, and several inline guards (worktree-identity mismatch, branch
drift, empty-diff, security-block — lines 106-450) record a terminal FAIL verdict framed around "this
is a code-review of a diff about to become a PR." A review-verdict gate on an arbitrary custom
transition (e.g. "idea→ready requires an automated feasibility-review PASS," which has no diff, no
PR, no worktree) would need these diff/PR-specific pre-checks made conditional or factored out,
not just parameterized by prompt. Recommend Phase 3 planning scope the generalization to "reuse the
verdict vocabulary, the async session+MCP-tool completion pattern, and prompt selection; treat the
diff/PR/worktree pre-checks as `review`→`pr_pending`-specific enrichment layered on top of a leaner
core" rather than trying to make one `Run` function branch correctly for every possible
transition.

## 3. Liveness/staleness shapes actually present across the ~12 `StuckReason`s

Feasibility Risks explicitly flags this as unresolved — surveyed all `~19` current `StuckReason`
constants (`session/domain/backlog.go:38-211`; the doc's "~12" underestimates the current count,
several were added since). They cluster into **at least four distinct liveness shapes**, not one
shape with configurable numbers — confirming the risk and giving Phase 3 concrete cases to design
against:

1. **Polled active-process staleness** (`StuckReasonStaleWork`, `session/backlog_lifecycle_stale.go`):
   a live tmux `Instance` exists; "stuck" = no progress reported for
   `maxWorkSessionStaleness` (2h, a package `const`). Liveness signal comes from the Instance's own
   activity (output/progress), polled periodically. Remediation exists
   (`StaleWorkRemediator.RemediateStaleWorkSession` — kill pane, respawn with fresh turn budget).

2. **Bounded headless-call staleness with an optional liveness backstop**
   (`StuckReasonOrphanedTriage`, `session/backlog_lifecycle_triage.go:52-230`): no live `Instance` to
   poll (a headless subprocess call, not a tmux session) — "stuck" is inferred from wall-clock time
   since the session row's `CreatedAt` exceeding `maxHeadlessTriageSessionStaleness` (35m, **required
   by comment to stay strictly greater than** `server/services.triageCallBudget`, 30m, with "real
   margin" — this exact relationship is BUG-055's race and the motivating bug for this whole
   project). A secondary `IsTriageLive(itemID)` check (line 39) is consulted before tombstoning, as
   a structural fix for the same race the staleness margin only defends in depth. This shape has
   **three sub-cases** sharing one `StuckReason` (still-open-and-stale; already-ended-and-never-
   transitioned; already-ended-but-advanced-with-no-usable-result) — see the exhaustive doc comment
   at `backlog_lifecycle_triage.go:128-162`. This is the shape the motivating sdd-triage bug lives in.

3. **External-resource polling against a third-party system's state**
   (`StuckReasonPRReadyUnmerged`, `StuckReasonPRNeedsFix`, `StuckReasonPRPendingNoPR`,
   `session/backlog_lifecycle_pr.go`): liveness isn't about *our* process at all — it's GitHub's PR
   state (mergeable, CI status, merged-at), polled via `ReconcilePRPending`, gated by a threshold
   (`prReadyThreshold`, line 1926) measuring how long a PR has sat green-and-unmerged. No "still
   running" signal exists here in the same sense as shapes 1-2 — a PR can be indefinitely "ready" with
   no process alive anywhere; the timeout measures human/external inaction, not process health.

4. **By-design indefinite gate states with no timeout at all**
   (`StuckReasonPlanNotApproved`, `StuckReasonBlockedByDependency`): these are *not* staleness/timeout
   detectors — they fire immediately and stay open indefinitely by design (their own doc comments say
   so explicitly) until a human acts (approve plan) or a dependency resolves. No duration threshold
   applies; the "liveness model" here is purely "is the blocking condition still true," polled but
   never timed out.

There are also **meta/aggregate reasons with no liveness of their own**
(`StuckReasonMultipleReasons`, `StuckReasonBounceCapExhausted`) that are derived from *other* open
reasons crossing a count/attempt threshold — a fifth "shape" that isn't really a liveness definition
at all but a composition rule over other stages' liveness outcomes.

**Consequence for Phase 3 schema design**: a single `{expected_duration, staleness_threshold}` pair
per stage cannot represent shape 3 (no process to be "expected" to run — the threshold is about
external inaction) or shape 4 (no threshold applies at all) using the same fields shape 1/2 need. The
liveness model needs at minimum a **shape/kind discriminator** (e.g. `LivenessKindProcessPoll` /
`LivenessKindHeadlessBudget` / `LivenessKindExternalPoll` / `LivenessKindIndefinite`), not one
universal numeric schema — closing this project's Feasibility Risk with a concrete answer rather
than leaving it open.

## 4. The shared remediation backoff gate — and its BUG-055 relationship

`session/backlog_remediation.go` is the single shared exponential-backoff gate
(`remediationBackoffSchedule` = 30m/2h/8h/24h/72h, then a cold-retry heartbeat per BUG-083) that
every automated remediation action across all shapes above must pass through
(`Storage.RemediationDue`). This is a **second, independent** timing axis from the liveness
threshold itself: liveness answers "is this stage/item stuck," remediation backoff answers "how
often may we *retry* fixing a stuck item." BUG-055 is specifically the *liveness* threshold (a
sweep interval) racing a *work budget* (a call timeout) — a different pair than the remediation
backoff schedule, though both are "two independently-editable durations that must stay ordered." The
new liveness model's enforced-invariant requirement (Rabbit Holes: "derive the sweep threshold as
budget-plus-margin rather than storing both independently") should be scoped to the
liveness-threshold/work-budget pair specifically; the remediation backoff schedule is a separate,
already-working mechanism this project should not need to touch, only consult (a stage's liveness
definition determines *when* an item becomes eligible for the existing `RemediationDue` gate, not
replace that gate).

**Existing "manual retry" affordance, not a "snooze"**: `Storage.ResetStuckRemediation` /
`BulkResetStuckRemediation` are exposed via RPC (`server/services/backlog_service_stuck.go`) and
surfaced in the UI (`web-app/src/components/backlog-stuck/StuckItemsSection.tsx`,
`useStuckBacklogItems.ts`) as a "Retry Now" action — the single-operator can already force
remediation to fire immediately, bypassing the backoff wait. This is the closest existing precedent
to an "operator override" affordance, but it resets the **retry backoff counter**, not the
**staleness/liveness threshold itself** — it doesn't let an operator say "this specific item's
triage call is legitimately going to take 45 minutes this once, don't flag it stuck at 35." See §6
for why that's a distinct, currently-unmet need.

## 5. Industry conceptual models: state / transition / gate / SLA separation

Surveyed for how mature workflow engines separate these four concerns — not to recommend adopting
any of them (requirements.md scopes this as in-repo Go/ent, no new dependency), but to check this
project's proposed separation against established prior art:

- **GitHub branch protection** (required reviewers + required status checks) cleanly separates
  "state" (open/merged PR — not really configurable) from "gate": a merge is blocked until N
  required-reviewer approvals AND all required status checks report success. Each required check is
  registered independently and has its **own timeout semantics owned by the check itself** (a GitHub
  Action job has its own timeout-minutes), not a property GitHub's merge gate imposes uniformly. This
  maps directly onto this project's "gate" concept — a gate is a pass/fail predicate with its own
  liveness envelope, not something the transition-level model times out on behalf of the gate — which
  supports the Rabbit Holes instruction that a custom/pluggable check gate should be "expressible
  using the same liveness primitives" the stage model already has, rather than the transition model
  inventing a second timeout mechanism around whatever the gate does.
- **Jira workflow schemes**: states + transitions, and each transition can carry **conditions**
  (who/what may even attempt it — visibility/permission), **validators** (must pass or the transition
  is rejected, analogous to this project's structural checks), and **post-functions** (side effects
  after a successful transition). Notably, Jira's model has *no native per-state SLA/timeout concept*
  at all — that's bolted on via separate "SLA" add-ons (e.g. Jira Service Management's calendars/goals
  keyed to a state) layered on top of, not inside, the workflow scheme. This is a useful signal: even
  a mature, widely-deployed configurable-workflow product treats "state machine shape" and
  "time-in-state SLA" as **separate, independently-evolving concerns with their own schema**, which
  supports requirements.md's own Open Question leaning toward a sibling interface (mirroring
  `PipelineEngine`/`WorkflowEngine` staying separate) rather than cramming liveness into
  `WorkflowEngine` itself.
- **Temporal / Camunda (BPMN) workflow engines**: both model a "gate" primitive with an inherent
  timeout as a first-class citizen of the flow definition itself — BPMN's boundary timer events and
  Temporal's activity/workflow timeouts are declared *on the step*, and a triggered timeout is itself
  a first-class transition (to an error/escalation state), not a side-channel sweep that polls state
  from outside. This is the strongest conceptual argument for this project's core design instinct
  (liveness lives with the stage, and a timeout is itself a legitimate way to leave the stage) over
  the current codebase's actual implementation (an external periodic `reconcile*` sweep polling
  state from outside the state machine) — worth naming explicitly in Phase 3 planning as "the
  aspirational model these engines use vs. the polling-sweep model this codebase already has and
  isn't being asked to replace" so planning doesn't accidentally scope-creep into rebuilding the
  scheduler.

**Where this project's scope correctly stays narrower than any of the above**: none of GitHub,
Jira, Temporal, or Camunda are single-operator tools with a "structural integrity, not access
control" threat model — their condition/permission layers (Jira's "conditions," GitHub's "who can
approve") are solving a multi-tenant authorization problem this project explicitly doesn't have
(Out of Scope: "multi-tenant/multi-user permissions... remains a single-operator tool"). The
generalization this project needs is the **shape** (state/transition/gate/liveness as four separable
concerns), not the permission machinery layered on top of that shape in every one of these products.

## 6. Unstated single-operator needs beyond the explicit requirements

- **Per-item timeout override ("snooze this one"), distinct from redefining the stage config**:
  confirmed as a real gap, not speculative — §4 shows the *only* existing operator override
  (`ResetStuckRemediation`) resets the retry-backoff clock, not the liveness/staleness threshold. If
  an operator knows a specific item's sdd-mode triage is going to legitimately run 50 minutes this
  one time (a large research doc, a slow model day), today's only lever is either wait for it to
  falsely flag stuck-then-auto-recover, or edit the stage's liveness config for *every future item*
  of that mode — there's no "extend this item's deadline by N minutes" affordance. Given this
  project's explicit success metric of fixing the sdd-mode timeout as a config change rather than a
  code change, the natural next operator want is the same lever at per-item grain. Recommend Phase 3
  at least scope-flag this (even if deferred past Milestone 1/2) — it's the kind of "small UI
  affordance, disproportionate day-to-day value" gap the sibling `PipelineEngine` project's own
  research doc flagged for "preview/dry-run" (§4 there) and turned out to be worth a line item.
- **"What's blocking this transition right now" visibility is already a named success metric**
  (requirements.md's own scope item) — but note it has a **partial existing precedent** worth reusing
  visually: `BlockerChip` (referenced in `StuckReasonBlockedByDependency`'s doc comment,
  `session/domain/backlog.go:169`) already renders "why is this item not progressing" for the
  dependency-gate case on the item detail page. A generalized gate-status UI should extend that
  existing chip pattern to every gate type (human-approval pending, review-verdict pending,
  structural-check unmet, custom-check running/failed) rather than inventing a new visual language —
  cheap consistency win worth naming to the UX/Phase-3 design pass.
- **Fail-closed-and-loud is already this subsystem's house style, not a new ask**: requirements.md's
  Risk Control section ("malformed custom stage/liveness definition fails closed to default built-in
  behavior with a loud Warn") matches the existing pattern at, e.g.,
  `triageEndReasonOrUnknown`(`backlog_lifecycle_triage.go:94`, falls back to `"unknown"` rather than
  rendering a blank) and the sibling `PipelineEngine` project's own established fallback-to-default
  convention (per that project's requirements). No new design principle needed here — just apply the
  existing one consistently to the new config surface, which Phase 3 should call out explicitly
  rather than re-justify from scratch.
- **A stage/liveness "what will this do" preview** — the sibling doc's §4 flagged this gap for
  `PipelineEngine` modes ("what will run" preview, no dry-run execution). The identical need applies
  here for a *stage's* liveness definition: an operator defining a custom stage's expected-duration/
  staleness values benefits from seeing, statically, "this stage will be flagged stuck after Xh of no
  progress, escalated per the standard backoff schedule (30m/2h/8h/24h/72h), then cold-retried every
  7 days" rendered from the config rather than having to mentally simulate `evaluateRemediation`.
  Same low-incremental-cost recommendation as the sibling doc: render existing static metadata, not a
  new execution-preview concept.

## Key files referenced

- `session/workflow_engine.go` (`WorkflowEngine`/`DefaultWorkflowEngine`, the seam to extend)
- `session/backlog.go:205-228` (type aliases into `domain`, `validTransitions` snapshot)
- `session/domain/backlog.go:38-249` (`StuckReason` enum, all ~19 current constants surveyed for §3)
- `session/domain/backlog.go:541-614` (`TransitionGuard` — every existing structural gate)
- `session/review_gate.go:21-95` (`ReviewGateRunner`, existing `PipelineEngine`-aware prompt
  selection — reusable seam for a generalized review-verdict gate)
- `session/backlog_lifecycle.go:798` (the one call site hardcoding "review gate fires only on
  `toStatus == BacklogStatusReview`" — the actual generalization point, not `review_gate.go` itself)
- `session/backlog_lifecycle_stale.go:59-62` (`maxWorkSessionStaleness`, liveness shape 1)
- `session/backlog_lifecycle_triage.go:42-230` (`maxHeadlessTriageSessionStaleness`,
  `IsTriageLive`, BUG-055 margin comment, liveness shape 2 — the motivating bug's home)
- `session/backlog_lifecycle_pr.go:1479,1507-1951` (`prReadyThreshold`, liveness shape 3)
- `session/backlog_remediation.go` (`remediationBackoffSchedule`, `RemediationDue`,
  `ResetStuckRemediation` — the separate, already-working retry-backoff axis)
- `server/services/backlog_service_stuck.go`,
  `web-app/src/components/backlog-stuck/StuckItemsSection.tsx` (existing "Retry Now" manual
  override — precedent for, but distinct from, a needed per-item timeout-snooze affordance)
- `project_plans/backlog-configurable-pipeline/research/features.md` (sibling doc — `PipelineEngine`
  precedent, naming-collision risk, caching-layer risk, "what will run" preview gap)
