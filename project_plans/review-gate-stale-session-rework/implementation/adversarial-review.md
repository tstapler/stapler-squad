# Adversarial Review: review-gate-stale-session-rework

**Date**: 2026-07-24
**Verdict**: CONCERNS — 0 blockers, 2 concerns, 3 minors

**Process note**: No subagent-dispatch tool was available in this environment for this planning run — this adversarial pass was performed by the same planner adopting a deliberately skeptical stance, not by an independently-instantiated reviewer agent. Flagged for transparency about review independence.

## 1. Missing failure modes

- **CONCERN — error handling on `MarkStuck`/`ResolveStuck` calls isn't fully specified.** `MarkStuck` returns `(applied bool, err error)`; the plan's Task 2.1.1a discusses the `applied == false` (precondition mismatch) branch explicitly, but doesn't say what happens on a genuine `err != nil` (e.g., a transient DB error). Given this function's established "best-effort, fail open, never block the existing notification" philosophy (documented in its own doc comment and cited repeatedly in this plan), the correct behavior is almost certainly "log a warning and still publish the existing notification" — but the plan should say so explicitly rather than leave it to implementation-time judgment, since silently swallowing a DB error with no log would be a real regression in observability. Same applies to `ResolveStuck` in Task 2.1.2b. **Recommendation**: add one sentence to each affected task specifying "on error, log warning and continue — do not let a storage failure block the existing notification path or crash the reconcile tick," matching `reconcileStaleWorkSessions`' own established error-handling style verbatim.
- **Checked, not a finding**: what happens if `sessionStopper`/`eventBus` are nil (already-existing early-return branches in `notifyIfActiveWorkSessionStale`)? The plan's own pitfalls.md (#6) already flags this and requirements.md's research explicitly calls out that this needs an explicit decision at implementation time rather than inheriting the existing silent-skip behavior by default — acceptable to leave as an implementation-time call given it's flagged, not silently missed.
- **Checked, not a finding**: does changing the shared `DefaultReviewQueuePollerConfig().StalenessThreshold` default (Story 1.1.2) affect any consumer other than the two already identified? Verified via `grep -rn "StalenessThreshold"` across the repo — only `review_queue_determiner.go:262` and the soon-to-be-decoupled `backlog_service_triage.go:952` reference it in production code. After this plan lands, exactly one production consumer remains, matching the plan's intent. No hidden blast radius.

## 2. Architecture risks

- **MINOR — the new `ReworkBlockStaleResolver` interface's single implementer (`BacklogService`) means it's currently unit-testable only via the concrete type or a hand-rolled fake, not via a pre-existing test double.** This mirrors `StaleWorkRemediator`'s exact situation today, so it's consistent with the codebase's status quo rather than a new problem — noted as a minor, not a concern, precisely because it isn't a regression.
- No other architecture risk beyond what's already surfaced and resolved in architecture-review.md (the layering correction made during planning). The plan's total new surface area (one interface, one enum value, one small orchestration function, a handful of map entries) is small enough that "hard to change, scale, or test in isolation" doesn't meaningfully apply here.

## 3. Scope drift

- No scope drift found. Cross-checked every Phase/Epic/Story against requirements.md's explicit In Scope list (items A, B, D + registry entries) — nothing in plan.md builds toward requirements.md's explicit Out of Scope items (C: auto-escalation; changing the force-kill policy; the other 4 `backlog-stuck-item-visibility` reasons; general working/idle/stuck heuristics). Story 2.1.1's explicit "no automated remediation action" note is a good defensive design choice specifically preventing scope creep into item C.
- **MINOR** — Task 1.3.1b ("Regenerate bindings... verify only the expected new-value diff appears") is a reasonable guard against `make proto-gen` drift, but the plan doesn't say what to do if unrelated regeneration drift *does* appear (a known risk class in this repo — CLAUDE.md's proto-gen notes hint at this being a recurring nuisance). Not a scope-drift risk to the feature itself, just a minor process gap: if it happens, the correct move is to isolate the feature's own diff into its own commit and treat any unrelated regenerated-file drift as a separate, pre-existing issue not introduced by this change — worth a one-line note during implementation, not a plan blocker.

## 4. Technology bets

- None. No new dependencies, libraries, or non-standard technology choices — confirmed by build-vs-buy.md's own analysis and unchanged by planning.

## 5. Missing coverage

- **CONCERN — requirements.md's Success Metrics include a live-instance re-verification step** ("re-checking the live Review Queue after the fix ships against the same class of items that triggered the original report"), but plan.md's task list is entirely unit/component-test-level — there is no task representing this live check. This is correctly deferred to validation.md (Phase 4's job, per the SDD workflow's own phase boundaries) rather than plan.md, but flagging here to ensure Phase 4 doesn't drop it: the live re-verification is a *requirement*, not a nice-to-have, given the bug was originally discovered via live observation, not a failing test.
- **Checked, not a finding**: item D (task-protocol cadence) is covered by Epic 3.1 with an explicit acceptance criterion and task. Item A (threshold decoupling) is covered by Epic 1.1 with both sub-parts (rework gate + general badge). Item B (durable state) is covered by Epics 1.2/1.3/2.1/2.2 comprehensively, including the resolve-path gap that a shallower plan could easily have missed (confirmed present: Story 2.1.2). No requirements.md In Scope item lacks a corresponding story.

## Minors

- The Domain Glossary's `ReworkBlockStaleResolver` entry and Pattern Decisions table both explain the layering rationale in similar but not identical words — mildly redundant, harmless.
- Task 2.2.1a's icon suggestion ("🟥 or ⏸️") is explicitly non-final — already flagged as a CONCERN in architecture-review.md; not re-flagging here, just cross-referencing.
- ADR-001's "Alternatives Considered" section doesn't explicitly discuss the option of leaving the general Review Queue badge threshold at 2 minutes and only fixing the rework-gate — it's implicitly covered by the "leave the badge unchanged" bullet, but a reader skimming just the Decision table might miss that this was a deliberate rejection rather than an oversight. Cosmetic; the content is there, just not maximally scannable.
