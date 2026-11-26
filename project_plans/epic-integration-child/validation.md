# Validation: epic-integration-child

**Date**: 2026-11-24

## Adversarial Review

### Challenge 1: Hard Dependency on Unshipped Prerequisite

**Risk**: The plan calls for `OnEpicPlanningComplete` to enumerate sibling children via the Epic items feature's parent-child edge. That feature is not shipped. Without it, there is no way to know which items are "siblings" of an epic.

**Verdict**: REAL. The plan must explicitly gate Phase 2's trigger on the prerequisite feature landing. Until then, Phases 1 and 3–5 can proceed independently (schema, attribution logic, UI, tests), but Phase 2's hook cannot fire.

**Mitigation in plan**: Phase 2 task 2.2 step 1 notes the dependency. Add a feature flag `epic_integration_child_enabled` gating the `OnEpicPlanningComplete` hook; it ships off, and the Epic items team flips it on when their parent-child edge is live.

---

### Challenge 2: Attribution Heuristic Is Probabilistic

**Risk**: A 60% file-overlap threshold for attribution is arbitrary. With a small codebase or broad cross-cutting changes, the wrong sibling gets blamed and an incorrect fix child is created.

**Verdict**: REAL. The mitigation (escalate below threshold) is correct, but the threshold itself needs empirical validation. The plan should require the threshold to be configurable (a `GlobalSettings` or `PipelineMode` field, not a hardcoded constant).

**Mitigation in plan**: Store threshold in `GlobalSettings.IntegrationAttributionThreshold` with a default of 0.6, and add a note to Phase 3 task 3.1 to read from config rather than a constant.

---

### Challenge 3: Integration Child Can Block Epic Forever

**Risk**: If the integration e2e check is flaky or permanently broken, the epic can never transition to `done`, blocking the entire roadmap.

**Verdict**: REAL. Two escape hatches exist: (a) operator archives the integration child (counts as resolved in dependency gate), (b) operator waives the relevant AC on the integration child. Both require deliberate operator action — good, not a bug. Document both escape hatches in the feature's help text (AC-6).

---

### Challenge 4: Fix-Child Escalation Chain

**Risk**: Each integration failure creates a fix child. If that fix child itself fails, another fix child is created, and so on — an unbounded rework escalation chain.

**Verdict**: REAL. Plan task 3.2 should cap fix-child creation at `rework_cap_override` (the existing per-item rework cap). After cap reached, always escalate via `GuidanceRequest` regardless of attribution confidence.

---

### Challenge 5: UI Scope

**Risk**: The plan adds UI changes but doesn't specify the route-meta row (required by `.claude/rules/nav-facade.md` for new pages) or feature-flag guard.

**Verdict**: MINOR. The UI changes are to existing components (no new page), so no route-meta row is needed. The feature flag gate on the epic detail section should use `<FeatureGuard flag="epic_integration_child">` once the feature is flagged.

---

## Verdict

Plan is sound with four targeted amendments:
1. Add `epic_integration_child_enabled` feature flag gating Phase 2's hook.
2. Store attribution threshold in `GlobalSettings`, not a hardcoded constant.
3. Cap fix-child creation at `rework_cap_override`; always escalate after cap.
4. Document operator escape hatches (archive or waive) in the feature's help text.

None of these invalidate the approach; all are additive constraints. Plan proceeds to implementation.
