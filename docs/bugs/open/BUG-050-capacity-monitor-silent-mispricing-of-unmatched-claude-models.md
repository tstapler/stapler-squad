# BUG-050: `CapacityMonitor.estimateCost` Silently Mis-Prices Any Claude Model Not Matching `opus`/`haiku`/`flash`/`pro` at a Wrong Nonzero "Sonnet Default" Rate — Feeds a Live Autonomous-Session-Pause Trigger [SEVERITY: High]

**Status**: 🐛 Open
**Discovered**: 2026-07-27, during `insights-cost-pricing-gaps` (AC-1..AC-7) research/planning — `research/pitfalls.md` §6 and `research/architecture.md` surfaced this as a second, independent pricing table out of scope for that project. Deferred and filed per `project_plans/insights-cost-pricing-gaps/decisions/ADR-002-capacity-monitor-pricing-scope-deferred.md` (see that ADR for full context and the two remediation options considered).
**Impact**: `CapacityMonitor.checkThresholds` (`server/services/capacity_monitor.go:243`) compares `EstimatedCostUSD` against `config.CostBudgetUSD` (`config/types.go:257`) and returns the `cost_budget_exceeded` trigger once the budget is met/exceeded. That trigger flows into `handleTransitionTrigger` (`capacity_monitor.go:305-311`), which can **pause or stop a live autonomous session**, or auto-switch it, based on this estimate. Any Claude model whose name doesn't substring-match `opus`/`haiku`/`flash`/`pro` — e.g. `claude-sonnet-5`, or any newly-released Claude model — silently falls through to a "sonnet default" rate ($3.00/$15.00 per MTok) with **no signal anywhere that the fallback fired**. This is worse than a missing-price bug: the estimate is a plausible, nonzero cost figure, so it looks legitimate while potentially being wrong by a large factor, driving real automated pause/stop decisions on bad data.

## Problem Description

`server/services/capacity_monitor.go:351-371`:

```go
func (m *CapacityMonitor) estimateCost(model string, input, output int64) float64 {
    inputPrice := 3.0   // sonnet default
    outputPrice := 15.0 // sonnet default

    model = strings.ToLower(model)
    if strings.Contains(model, "opus") {
        inputPrice = 15.0
        outputPrice = 75.0
    } else if strings.Contains(model, "haiku") {
        inputPrice = 0.25
        outputPrice = 1.25
    } else if strings.Contains(model, "flash") {
        inputPrice = 0.075
        outputPrice = 0.3
    } else if strings.Contains(model, "pro") {
        inputPrice = 1.25
        outputPrice = 5.0
    }

    return (float64(input)*inputPrice + float64(output)*outputPrice) / 1_000_000.0
}
```

This is a second, independent, hardcoded, substring-matched pricing table — separate from `session/tokens/pricing.go`'s `PricingTable`, which is the canonical/maintained pricing source for the Insights dashboard (`insights-cost-pricing-gaps`, AC-1..AC-7). `estimateCost` is not kept in sync with it.

Two properties make the fallback dangerous rather than merely incomplete:
1. It does not fail visibly to `$0` or an error — an unrecognized model name falls through to the "sonnet default" `$3.00`/`$15.00` per-MTok rate, which may be **wrong**, not merely missing, and nothing logs or flags that the fallback path was taken.
2. It feeds a real, live automated action today: `checkThresholds`'s `cost_budget_exceeded` trigger can pause/stop or auto-switch an in-progress autonomous session. This is not read-only reporting (unlike the Insights dashboard) — a wrong estimate can incorrectly halt a session that's actually under budget, or (in the opposite direction, if the true price is higher than the fallback) let a session run well past its intended budget before the trigger fires.

It also prices **non-Claude models** (`flash`, `pro` — Gemini families), which `session/tokens/pricing.go`'s `PricingTable` intentionally does not cover, per `insights-cost-pricing-gaps`'s Non-Goals ("Pricing for non-Claude models/providers, unless the codebase already tracks them in the same table"). Any fix needs to either unify the Claude-model portion with `tokens.PricingTable` while leaving non-Claude pricing on its own explicit path, or otherwise resolve the duplication deliberately — see ADR-002's "Consequences" section for the two options considered and why neither was folded into `insights-cost-pricing-gaps`.

## Reproduction Steps

1. Run an autonomous session against a Claude model whose name does not contain `opus`, `haiku`, `flash`, or `pro` as a substring (e.g. `claude-sonnet-5`, or any future model name that breaks the substring assumption).
2. Let the session accumulate token usage; `CapacityMonitor` periodically calls `estimateCost(model, input, output)` to compute `EstimatedCostUSD`.
3. Expected: cost estimate uses the correct per-model rate, or the estimator visibly signals "unknown model, rate not verified" so a wrong budget-trigger decision isn't silently made on bad data.
4. Actual: the estimator silently applies the "sonnet default" rate ($3.00 input / $15.00 output per MTok) regardless of whether that rate matches the actual model — with zero logging, metric, or other signal that the fallback path was used — and the resulting (possibly wrong) `EstimatedCostUSD` feeds directly into the `cost_budget_exceeded` autonomous-session-pause/stop trigger.

## Root Cause

Two independent, hand-maintained pricing tables exist in the codebase for cost estimation:
- `session/tokens/pricing.go`'s `PricingTable` (maintained/updated as part of `insights-cost-pricing-gaps`, Claude-scoped)
- `server/services/capacity_monitor.go`'s `estimateCost` (out of scope for that project, substring-matched, silent-fallback, and also covers non-Claude Gemini models)

`estimateCost` was never updated to route through `tokens.PricingTable`, and its substring-match approach has no "unknown model" branch — every unmatched name falls through to a default rate meant for Sonnet specifically, with no differentiation between "this is actually Sonnet" and "this is some other Claude model we don't recognize."

## Files Likely Affected

- `server/services/capacity_monitor.go:351-371` — the `estimateCost` function itself
- `server/services/capacity_monitor.go:243` — `checkThresholds`, where `EstimatedCostUSD >= config.CostBudgetUSD` produces the `cost_budget_exceeded` trigger
- `server/services/capacity_monitor.go:305-311` — `handleTransitionTrigger`, where the trigger can pause/stop/auto-switch a live autonomous session
- `config/types.go:257` — `CostBudgetUSD` config field being compared against
- `session/tokens/pricing.go` — the canonical, maintained `PricingTable` this should likely be unified with for Claude models

## Fix Approach

Not implemented here (out of scope for `insights-cost-pricing-gaps` per ADR-002). Two options were considered and deferred:
1. Extend `tokens.PricingTable` to also cover non-Claude providers (Gemini `flash`/`pro`), then have `estimateCost` call into it for all models — rejected for this project because it conflicts with `insights-cost-pricing-gaps`'s explicit Non-Goal of adding non-Claude pricing.
2. Build a Claude-only partial fix for `estimateCost` (route Claude models through `tokens.PricingTable`, leave Gemini `flash`/`pro` on the existing substring table) plus an explicit "unknown model" signal (log line, metric, or error) instead of a silent default-rate fallback — this closes the "wrong nonzero rate with no signal" risk without touching non-Claude pricing, but was still judged out of scope/review-surface for the targeted `insights-cost-pricing-gaps` PR.

Whichever approach is chosen, the fix must specifically address: (a) no more silent fallback to a rate that may not match the actual model, and (b) some visible signal (log/metric/error) when a model isn't recognized, so a future maintainer can tell the difference between "this genuinely priced as Sonnet" and "we didn't know what this was."

## Verification

- Unit test: call `estimateCost` with a model name that doesn't substring-match any of `opus`/`haiku`/`flash`/`pro` (e.g. `"claude-sonnet-5"`) and assert either (a) the correct Sonnet rate is used only when the model is confirmed to actually be Sonnet, or (b) an unknown-model signal is emitted/returned rather than a silent default.
- Confirm `checkThresholds`'s `cost_budget_exceeded` trigger only fires on a verified cost estimate, or that an unknown-model estimate is flagged distinctly from a verified one in whatever surfaces the trigger (logs, session state, notification).

## Related Tasks

- ADR: `project_plans/insights-cost-pricing-gaps/decisions/ADR-002-capacity-monitor-pricing-scope-deferred.md` — full context, scope decision, and rationale for deferring this out of `insights-cost-pricing-gaps`
- `project_plans/insights-cost-pricing-gaps/implementation/plan.md` Epic 3.1 (Story 3.1.1) — the fast-follow-filing task that produced this bug doc
- `project_plans/insights-cost-pricing-gaps/research/pitfalls.md` §6 — original surfacing of this risk
