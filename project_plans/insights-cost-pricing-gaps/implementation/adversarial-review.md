# Adversarial Review: insights-cost-pricing-gaps
**Date**: 2026-07-27
**Verdict**: CLEAN

## Blockers

(none)

The prior BLOCKER — 3 of 6 cost-rendering frontend surfaces silently excluding unpriced usage, with `DailyTokenBucket.UnpricedModels` a dead field on arrival — is resolved. The updated plan adds **Epic 2.4: Remaining cost-rendering surfaces** (plan.md:550-663) with three new stories, each carrying a Given-When-Then AC and concrete tasks:

- **Story 2.4.1** (`DailySpendChart.tsx`): threads `hasUnpriced` through `DataPoint`/`toDataPoints`, renders a pluralized footnote ("N days include unpriced model usage"). Verified against actual source (`web-app/src/app/insights/DailySpendChart.tsx`) — the `DataPoint` interface and `toDataPoints` function are exactly where the plan says (lines ~21-34), and the proposed `(b.unpricedModels?.length ?? 0) > 0` mirrors the existing `estimatedCostUsd` field access pattern in the same function. Sound.
- **Story 2.4.2** (`ModelOverTimeChart.tsx`): adds a `collectUnpricedModels` helper mirroring the existing `collectModels` helper, and appends a `"(pricing unavailable)"` suffix to matching legend entries. Verified against source — `collectModels` (lines 52-59), the component signature (line 87), and the legend `.map()` block (lines 153-163) all match the plan's cited line numbers closely enough to trust the diffs described. The approach correctly reuses the family-keyed `costByModel`/`tokensByModel` shape already iterated by `collectModels`.
- **Story 2.4.3** (`useProjectedCost.ts`/`ProjectedCostCard.tsx`) — the one with real decision consequences, correctly flagged as such in the plan's rationale. Verified against source: `ProjectedCostResult` interface is exactly lines 4-8 as cited; the `currentMonthBuckets` filter/reduce logic the plan hooks `hasUnpricedUsage` onto (`currentMonthBuckets.some(...)`) is a straightforward analog of the existing `totalCost` reduction one line below it; and `ProjectedCostCard.tsx:40` is, verified exactly, the `"Based on {daysData} of {daysInMonth} days"` sub-line the plan says to render the caveat directly after. This is technically sound and directly closes the "unpriced $0 looks cheapest, not unknown" risk `research/pitfalls.md` §7 warned about for this specific consumer.

All three stories consume `DailyTokenBucket.UnpricedModels` (added to the proto by Task 1.3.2c, populated backend-side by 1.3.3b/c) — it is no longer a dead field. The `warningText` token the new styles reference (`vars.color.warningText`) already exists in `web-app/src/styles/theme.css.ts` and is already used by `ProjectedCostCard.css.ts`, so the CSS approach is consistent with `css-architecture.md`'s token-only rule and won't need a new token added.

The plan is explicit that Epic 2.4 was added during "plan repair" specifically to close this BLOCKER (plan.md:551), and states its only dependency is the same Story 1.3.2 generated types Epics 2.1-2.3 depend on — a reasonable, low-risk ordering claim.

## Concerns

All 5 prior CONCERNS are resolved:

- **(a) WebFetch failure mode** — Task 1.1.1a (plan.md:149) now specifies: retry once, then `WebSearch` for an official pricing announcement, then escalate to the user rather than falling back to memorized figures or stalling. Resolved.
- **(b) ADR-002 fast-follow bug visibility** — Story 3.1.1 (plan.md:677) now requires the filed bug's link/ID to appear in this project's own PR description under an explicit "Known related issue (not fixed here)" heading, explicitly citing this review's prior concern. Resolved.
- **(c) Regression-gate checkpoint after Epic 1.3** — new "Epic 1.3 Checkpoint: Regression gate before Epic 1.4" (plan.md:334-339) runs `go test ./session/tokens/... -v` and is explicitly gated as "must be green before Epic 1.4 begins," positioned immediately after Story 1.3.3 and before Epic 1.4 — exactly the placement recommended. Resolved.
- **(d) Mixed known/unknown-family test** — Task 1.3.1f (plan.md:235-238), inside Story 1.3.1, adds `TestModelFamilyCost_WhenMixedKnownAndUnknownFamilies_ExpectKnownPricedAndUnknownFlagged` with a `ParseResult` containing both a priced and unpriced family. Resolved.
- **(e) `TopNTables.tsx` scope confirmation** — an explicit "`TopNTables.tsx` scope note" (plan.md:523) records that it was checked and confirmed out of scope (no cost column), not silently dropped. Resolved.

No new concerns surfaced during this verification pass.

## Minors

- Carried forward, still true and still low-stakes (not re-verified this pass, no indication they changed): Task 1.4.1b's two-possible-test-location hedge, and `InsightsService.loggedUnpricedFamilies`'s unbounded-but-safe growth lacking an intentionality comment.
- New minor: Epic 2.4's three new Jest/RTL test files (`DailySpendChart.test.tsx`, `ModelOverTimeChart.test.tsx`, `ProjectedCostCard.test.tsx`) all reference `DailyTokenBucket`-shaped or `ProjectedCostResult`-shaped props with an `unpricedModels`/`hasUnpricedUsage` field that doesn't exist in the currently-generated `insights_pb.ts` yet (confirmed: `unpricedModels` is absent from the current generated file). This is correct and expected — Story 1.3.2's proto regeneration must land first — but the plan doesn't explicitly restate the Epic 1.3 → Epic 2.4 ordering dependency inside Epic 2.4's own task list (it's stated once at the epic-goal level, plan.md:128). Low-stakes since the dependency is stated elsewhere and is obvious from the field usage itself.
