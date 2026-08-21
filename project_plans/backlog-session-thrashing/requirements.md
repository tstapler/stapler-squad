# Requirements: backlog-session-thrashing

**Date**: 2026-07-25
**Type**: bug fix / reliability (existing project — backlog automation subsystem)

**Note on process**: This requirements document was derived directly from a live
operational problem report (Tyler, 2026-07-25) rather than through the interactive
Phase 1 interview. No user was available to answer the interview questions
synchronously — this follows the same convention already used by this repo's own
backlog-item prompts (see `.claude/rules/sdd-planning-artifacts-commit.md` and the
`backlog:*` skill family), where an operator-flagged problem statement stands in for
the interview transcript. If the Phase 2/3 research surfaces a materially different
root cause than hypothesized below, this document should be revised before Phase 3
plan is finalized.

## Problem Statement

Backlog work sessions — autonomous Claude Code sessions spawned by
stapler-squad's backlog automation to work an item end-to-end — are thrashing in
two related but distinct ways that are currently live and observed in production:

1. **Concurrency/duplication**: too many concurrent work sessions are spawned for
   the same backlog item, instead of at most one live worker per item.
2. **Turn-budget exhaustion**: work sessions get stuck or are terminated/restarted
   because a default ~20-turn budget is insufficient for realistic tasks to reach
   a natural completion point (request_review or a genuine done state), and the
   system's response to hitting that cap (retry silently, restart from scratch,
   spawn a duplicate, or park as "stuck" with no operator visibility) is not
   well understood or well designed.

This directly affects Tyler as the operator of the backlog automation system: he
is spending time noticing and manually intervening on thrashing/stuck items
instead of the system self-managing its own work-session lifecycle. It is an
every-day, ongoing problem (not a one-time incident) — every backlog item that
requires non-trivial work is at risk of tripping one or both failure modes.

Related, already-partially-addressed context (do not re-litigate, but check for
entanglement):
- PR #222 (commit d2b57fc9) fixed `onAutonomousDriverComplete` so the
  orchestrator's `Done=true` signal (from `AutonomousDriver.run()` in
  `session/autonomous_driver.go`) no longer forces a `review` transition when
  `request_review` was never actually called. That fix addressed *premature
  review transitions*, not turn-cap thrashing or session duplication — the two
  problems are likely separate, and this project's research phase must confirm
  whether they share a root cause or are independent.
- `docs/bugs/open/BUG-042-...md` (commit a3dda249) and
  `.claude/rules/tmux-keep-server-on-restart.md` describe adjacent
  session-lifecycle fragility (orphaned tmux control-mode clients, session
  destruction on service restart). Research must determine whether these
  mechanisms could be misfiring in a way that manifests as "session appears
  stuck" or "duplicate session spawned" even though the root cause is
  tmux/process lifecycle rather than the orchestrator's turn/done logic.

## Users / Consumers

- **Primary**: Tyler, as the sole operator of the backlog automation system —
  he observes thrashing/stuck items in the backlog UI and via logs, and is the
  one who currently has to intervene manually.
- **Secondary/systemic**: the backlog automation pipeline itself (orchestrator
  LLM, `AutonomousDriver`, `autonomous_orchestration_service.go`) is a
  "consumer" in the sense that its own scheduling decisions are the actors
  causing the thrashing — this is a self-inflicted systemic issue, not an
  external-input problem.
- No end users outside the operator are exposed to this; it is purely an
  internal reliability/efficiency issue in an automation system.

## Success Metrics

- **Primary**: at most one live (non-terminal) work session exists per backlog
  item at any time — no duplicate concurrent workers for the same item. This
  should be verifiable both by code inspection (an enforced invariant) and by
  observation (no recurrence of the duplicate-session pattern Tyler has seen).
- **Primary**: a work session that is making genuine forward progress is not
  killed or forced to restart purely because it crossed a fixed turn count —
  the turn budget (or whatever mechanism replaces/augments it) must
  distinguish "still productively working" from "stuck/looping" rather than
  applying one hard cutoff to both.
- **Secondary**: when a session *does* need to stop (budget genuinely
  exhausted, no progress happening), the outcome is a well-defined, visible
  state — e.g., a stuck/needs-attention marker the operator can see and act
  on — rather than a silent retry loop, a silent duplicate spawn, or a
  vanished/forgotten item.
- **Secondary**: regression tests exist that would fail on the current
  (pre-fix) behavior for both failure modes, so this class of bug cannot
  silently reappear (per `.claude/rules` convention on structural
  enforcement — see `quality:reflect-and-fix` skill).

## Constraints

- No fixed deadline; this is operator-flagged live pain, so priority is high,
  but there is no external SLA driving a specific date.
- No specified numeric performance/SLA target for "how many turns is enough"
  — the correct number (or the correct *mechanism* to replace a fixed number)
  is itself an open question this project's research/plan phases must answer,
  not an assumed constraint.
- Must not regress the fix already merged in PR #222 (commit d2b57fc9) —
  any redesign of the driver/orchestration completion logic must preserve
  "don't force review transition without an explicit request_review call."
  the `.claude/rules/tmux-keep-server-on-restart.md` and
  `docs/bugs/open/BUG-042-...` operational hazards must be considered but not
  necessarily fixed by this project unless research shows they are the actual
  root cause of the thrashing (in which case scope should be revisited, not
  silently expanded — see Out of Scope below).
- Solution must work within the existing architecture: Go backend
  (`session/`, `server/services/`), tmux-based session execution, orchestrator
  LLM polling pattern in `AutonomousDriver.run()`. This is a fix/hardening
  project, not a rewrite.
- Per repo convention (`.claude/rules/feature-testing-registry.md`-style
  discipline generalized to backend logic), any new/changed backend behavior
  needs test coverage and, where applicable, a way for the registry/state
  machine to make the "one live session per item" invariant a structural
  guarantee rather than a convention enforced by scattered call sites.

## Scope

### In Scope

- Auditing and, if needed, redesigning the mechanism(s) responsible for
  preventing duplicate/concurrent work sessions per backlog item — including
  verifying whether an existing WIP-limit/dedup mechanism (e.g.
  `countLiveBacklogWorkSessions`, `notifyIfActiveWorkSessionStale`,
  `StuckReasonReworkBlockedStale`, or equivalents currently in the codebase)
  actually covers all the spawn paths that can create a new work session for
  an item, and closing any gap found.
- Auditing and redesigning the turn-budget enforcement for work sessions:
  where "20 turns" (or whatever the actual current default is) is configured,
  how/where it's enforced, and what happens on cap — with the goal of making
  the cap-exceeded behavior explicit, visible, and non-duplicating, and of
  making the budget itself either configurable, adaptive to observed
  progress, or otherwise no longer the single point of failure for realistic
  tasks.
  - Case in point: `AutonomousDriver.run()` polls an orchestrator LLM off a
    raw terminal-tail snapshot with no visibility into acceptance criteria or
    diff state — the research phase must determine whether this
    signal-quality gap is itself contributing to false "stuck"/"done"
    determinations that interact badly with the turn cap.
- Defining what "stuck" means going forward (a single, well-defined state) and
  ensuring the system transitions a genuinely-stuck item into that state
  exactly once, with operator-visible signal, instead of looping.
- Test coverage (unit + integration where applicable) for the corrected
  dedup and turn-budget behavior.

### Out of Scope

- Fixing the general tmux control-mode/session-lifecycle fragility described
  in `docs/bugs/open/BUG-042-...md` and
  `.claude/rules/tmux-keep-server-on-restart.md`, *unless* Phase 2 research
  demonstrates it is a direct contributing cause of the thrashing reported
  here — if so, flag it explicitly in the plan as a dependency or a
  call-out, but do not silently fold an unrelated bug's full fix into this
  project's task list.
  found duplicate.
- Any change to the orchestrator LLM's underlying model, prompt strategy
  beyond what's needed to fix signal quality feeding into
  done/stuck detection (e.g., swapping models, wholesale prompt rewrites for
  unrelated quality improvements) — narrowly scoped to what's needed for
  accurate done/stuck signal.
- Rearchitecting `AutonomousDriver`/`autonomous_orchestration_service.go`
  beyond what's needed for this fix (no speculative refactors bundled in).
- UI/UX redesign of how stuck/thrashing items are displayed — a visible
  signal is in scope, but polished UI treatment is not (that can be a
  follow-up, e.g. via `backlog-stuck-item-visibility` if that overlaps).
- Implementation itself — this SDD run stops after Phase 4 (validate); no
  code is written in this run.

## Open Questions

1. What is the actual current default turn budget, and where exactly is it
   configured/enforced today (config default, hardcoded constant, per-item
   override)? (Research: Phase 2 stack/architecture agents)
2. When a work session hits the turn cap today, what concretely happens —
   does the driver retry, restart the session fresh, spawn a new session
   (duplicate), or mark the item stuck? Is this behavior even deterministic,
   or does it vary by code path? (Research: Phase 2 architecture/pitfalls
   agents)
3. Does an existing WIP-limit/dedup mechanism already claim to prevent
   concurrent sessions per item (e.g. `countLiveBacklogWorkSessions`), and if
   so, through what spawn paths could it still be bypassed (e.g., a stale
   session not counted as "live", a race between two triage/orchestration
   passes, a stuck-then-retried item not correctly excluded)? (Research:
   Phase 2 architecture agent; confirm in Phase 3 plan)
4. Is turn count the right unit for the budget at all, or should the
   trigger for "check if this session should still be running" be
   progress-based (e.g., diff churn, time since last meaningful tool call,
   distance from acceptance criteria) rather than a fixed turn count?
   (Research: Phase 2 features/pitfalls agents; decision recorded in Phase 3
   plan, ADR if the answer changes the architecture materially)
5. Are the two failure modes (duplication and turn-cap thrashing) actually
   coupled — e.g., does hitting the turn cap trigger a code path that
   (re)spawns a session without properly retiring the old one, meaning a
   single root-cause fix addresses both? (Research: Phase 2 architecture
   agent; this determines whether Phase 3's plan is one fix or two
   coordinated fixes)
6. How does the tmux-server-restart / orphaned-control-mode-client fragility
   (BUG-042, `.claude/rules/tmux-keep-server-on-restart.md`) interact with
   session liveness checks used by the dedup mechanism — could a session
   that's actually dead (due to tmux fragility) still be counted as "live",
   preventing legitimate re-spawn, or could a session that's actually alive
   be miscounted as dead, causing a duplicate? (Research: Phase 2 pitfalls
   agent)
