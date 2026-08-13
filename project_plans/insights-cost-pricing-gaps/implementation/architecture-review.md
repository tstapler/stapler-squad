# Architecture Review: insights-cost-pricing-gaps
**Date**: 2026-07-27
**Verdict**: CONCERNS

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository. No constitution-derived hard constraints apply — skipping this section.

## Blockers

*(none — the prior BLOCKER is resolved, verified below)*

**Prior BLOCKER — RESOLVED.** The prior finding was that Task 1.3.1a's `EstimateCost` signature change (`float64` → `(float64, []string)`) would break compilation at `server/services/backlog_service.go:429` and `server/services/backlog_service_query.go:471`, which the plan didn't cover, and that ADR-001's Consequences section incorrectly claimed Go lets you ignore an added return value at an existing call site.

Verified directly against the live files (not just the plan's claims):
- `server/services/backlog_service.go:429` currently reads `return pt.EstimateCost(r)` inside `buildCostLookup`'s `func(tmuxUUID string) float64` closure — confirmed via `grep -n`, exact line match.
- `server/services/backlog_service_query.go:471` currently reads `cost := s.pricing.EstimateCost(result)` — confirmed via `grep -n`, exact line match.
- Both are still single-value call sites today, so the original hypothesis is still accurate: Task 1.3.1a's signature change would break both without intervention.

Plan repair correctly addresses this:
- **`plan.md` Task 1.3.1g** ("Fix `EstimateCost` call sites outside `session/tokens`") adds the exact fix at both files, matching the real line numbers and real surrounding code: `backlog_service.go:429` → `cost, _ := pt.EstimateCost(r); return cost`, and `backlog_service_query.go:471` → `cost, _ := s.pricing.EstimateCost(result)`. This is not superficial — it correctly discards the new `unpriced` return value with `_` (neither call site needs it) rather than plumbing it through, which matches ADR-003's scope decision (see below).
- **`plan.md` Task 1.3.1h** adds an explicit `go build ./...` checkpoint immediately after Tasks 1.3.1a/1.3.1c/1.3.1g land, before Story 1.3.2 begins — catching any other stray call site this plan might have missed, not just at the final Epic 4.2 gate.
- **ADR-001's Consequences section** now has an explicit correction paragraph (dated 2026-07-27, "recorded during plan repair") stating plainly that the original claim was incorrect, naming both call sites and line numbers, and pointing to Task 1.3.1g/1.3.1h as the fix — the ADR record itself is now accurate, not just the plan.

This is a genuine fix, not a paper-over: the task text matches the actual code, the line numbers are correct, and the `cost, _ :=` idiom is the right minimal fix given both call sites are out of scope for surfacing the unpriced signal (per ADR-003).

## Concerns

- [ ] **ADR-003 exists and documents the Backlog cost-lookup scope decision, but — unlike ADR-002's `capacity_monitor.go` deferral — no task in Phase 3 operationalizes filing its fast-follow.** `ADR-003-backlog-cost-lookup-scope-deferred.md` is a well-reasoned, ADR-002-mirroring document: it correctly rejects folding the fix in now (unaudited `buildCostLookup` callers, a second unscoped proto/UI change) and requires a fast-follow backlog item covering (1) threading `unpriced` through `buildCostLookup`/`SessionCostEntry`, (2) a Backlog UI badge, (3) auditing `buildCostLookup`'s other callers. `plan.md`'s Unresolved Questions section also states "a fast-follow bug must be filed." However, Phase 3 (`## Phase 3: Scope Decision Documentation`) contains only **Epic 3.1: File the `capacity_monitor.go` fast-follow** (Story 3.1.1, Tasks 3.1.1a/3.1.1b) — there is no equivalent Epic/Story for ADR-003's fast-follow filing, and no reference to ADR-003 anywhere in Phase 3 or Phase 4's PR-description requirement (Task 3.1.1b only names ADR-002's bug). The same "silently left unaddressed" failure mode ADR-002/ADR-003 both warn against for their respective deferred items now applies one level up: a documented decision that says "must be filed" with no task in the plan that actually files it, and no PR-description requirement surfacing it to a reviewer.
  - **Recommendation**: Add a Story 3.1.2 (or extend 3.1.1) mirroring Task 3.1.1a/3.1.1b for ADR-003 — file the Backlog fast-follow bug referencing `server/services/backlog_service.go:399-429`, `server/services/backlog_service_query.go:455-477`, and ADR-003, and add it to the same "Known related issue (not fixed here)" PR-description heading Task 3.1.1b already establishes for ADR-002. Low severity (this is process/documentation, not code-breaking), but cheap to close before shipping.

- [x] **RESOLVED — `ModelBreakdown.SessionCount` not incremented for unpriced families.** `plan.md` Task 1.3.3d now explicitly includes `mb.SessionCount++` inside the new `for family := range unpricedFamilies` loop, with the task text calling out the reason directly: *"increments `SessionCount` the same way the priced-family loop above it does, so an unpriced-but-heavily-used family's row doesn't show real nonzero token counts alongside a contradictory `SessionCount: 0`."* This is the exact fix the prior review recommended, stated explicitly in the plan (not left implicit or superficial).

- [ ] **Unchanged, still accepted as non-blocking — `ModelBreakdown.PricingUnavailable bool` + `EstimatedCostUsd double` as independent flat fields permits a structurally-representable illegal state.** No change in this repair pass (confirmed: no mention of `oneof`, "illegal state", or this tradeoff anywhere else in the updated `plan.md`). This is fine — the prior review already classified this as low-severity and explicitly "Not blocking," reasoning that a `oneof` sum type is real but disproportionate complexity here (mirroring ADR-001's already-considered and correctly-rejected wrapper-type option), and the invariant is genuinely guaranteed by the single-lookup-per-family design rather than by convention alone. No new information changes that assessment.

## Nitpicks

- `ModelBreakdownChart`'s fix (Task 2.1.1b) still only annotates the legend text; the bar mark for an unpriced-but-used model remains zero-height. Unchanged from the prior review — still a defensible, documented "informational, not urgent" call per `research/ux.md` §4, not a required fix.
- ADR-002's deferral of `capacity_monitor.go` remains architecturally sound as scoped; the Strategy-pattern `CostEstimator` note from the prior review still applies to that eventual fast-follow's own design phase, not actionable now.
- `Pattern Decisions`' rejection of a `ModelFamily` newtype and of hot-reload machinery remain well-reasoned; no changes recommended.
- ADR-003 itself is a genuinely good piece of documentation — it explicitly mirrors ADR-002's reasoning rather than hand-waving a new justification, and correctly identifies that folding the Backlog fix in now would require auditing `buildCostLookup`'s other callers that this project's research never scoped. The gap is purely that the plan doesn't yet *act* on ADR-003's own "must be filed" requirement (see Concerns above).
