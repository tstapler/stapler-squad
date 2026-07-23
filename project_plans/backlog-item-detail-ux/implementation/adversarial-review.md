# Adversarial Review: backlog-item-detail-ux

**Date**: 2026-07-21
**Verdict**: CONCERNS

## Blockers

(none — all 3 prior blockers verified resolved on re-review)

## Concerns
- [ ] `useStuckBacklogItems()`'s failure path is silently dropped — `LifecycleSummary` never reads `error`/`isLoading`, so a fetch failure means the Blocker Chip silently never renders with no "data may be stale" signal. **Recommendation**: add an acceptance criterion for surfacing the error state.
- [ ] `classifySessionKind()`'s prefix strings are hardcoded independently in both the classifier and its test fixtures, with no shared constant/generated source — a future backend rename would silently regress classification to `"work"` with no test catching it. **Recommendation**: add a comment/TODO pointing at the backend constants.
- [ ] `SessionDiagnosticPanel`'s `headless_diagnostic` dispatch has no fallback when neither `triageResult` nor `reviewVerdict` is populated (crash-before-persist / async-write race). No acceptance criterion or test covers this.
- [ ] `StageTracker.deriveStageDisplay()` has no specified fallback for a `status` value outside the known 8 — unspecified behavior if the backend adds a status later.
- [ ] Task 3.1.4g modifies `LifecycleSummary.tsx` (an Epic 2 artifact) from within Epic 3, but the Dependency Visualization diagram doesn't show this cross-epic coupling — complicates the "each epic is independently revertable" claim.
- [ ] D6 (pipeline badge promotion, Task 3.1.4g) is framed as "duplication resolved" but `research/features.md`'s own D6 entry calls it speculative ("worth flagging as a candidate for promotion"), not a duplication finding — this is scope creep from a speculative research aside, not something requirements.md's success metrics asked for.
- [ ] Task 3.1.3c (suspend polling while `showManualReview` is open) fixes a real pre-existing bug that `pitfalls.md` explicitly flagged as optional/not required — the plan bakes it into binding acceptance criteria without calling out that it's a deliberate extra bug fix riding along with the refactor.
- [ ] Story 3.1.5 restricts auto-expand to first-mount-only, stricter than `ux.md`'s recommendation (allow one auto-expand on any genuine state transition, not just at mount) — probably the safer simplification, but the plan doesn't say this is a deliberate narrowing.
- [ ] New (introduced by the fix): Task 3.1.4i adds a `CollapsibleGroup`/`CollapsibleSection` split — one shared `Accordion.Root` wrapping 8+ sibling sections — that didn't exist in the prior plan revision. This is real added structural complexity (a wrapper component that must be correctly positioned as an ancestor for the keyboard-nav benefit ADR-027 cites to actually apply, plus a "standalone `CollapsibleSection` without a `CollapsibleGroup` ancestor" fallback mode needing its own test coverage) introduced specifically to make ADR-027's Accordion-over-`-collapsible` justification true in practice rather than aspirational. It is justified and covered by tests (Tasks 1.1.1e, 3.1.4i, 3.1.4j), but is a second new abstraction layered on the original single-component `Collapsible.tsx` ask and worth an explicit reviewer sign-off rather than being nodded through as a docs-only ADR fix.

## Minors
- The Dependency Visualization diagram references "SessionsSection from Epic 3 Story 3.4," but the actual story is numbered 3.1.4 — a naming mismatch.
- Story 4.1.3's acceptance criteria assume Story 4.1.1's security-review "render verbatim" outcome before that gate has run — low risk given the stated redaction fallback, but worth flagging as a mid-implementation replan risk if the review comes back negative.
- No task in Epic 6 adds a regression test confirming section expand/collapse state survives a poll-triggered `GetBacklogItem` error — behavior is inherited safely from existing code, but nothing locks it in with a test.
