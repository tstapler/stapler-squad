# ADR-002: `capacity_monitor.go`'s Independent Pricing Table Is Out of Scope — Fast-Follow Required

**Date**: 2026-07-27
**Status**: Accepted (scope deferred, not forgotten)

## Context

While researching `insights-cost-pricing-gaps` (AC-1..AC-7, scoped to
`session/tokens/pricing.go` and the Insights dashboard), `research/pitfalls.md`
§6 and `research/architecture.md` surfaced a second, independent,
**hardcoded, substring-matched** pricing table:

```go
// server/services/capacity_monitor.go:351-371
func (m *CapacityMonitor) estimateCost(model string, input, output int64) float64 {
    inputPrice := 3.0   // sonnet default
    outputPrice := 15.0 // sonnet default
    model = strings.ToLower(model)
    if strings.Contains(model, "opus") { inputPrice = 15.0; outputPrice = 75.0 }
    else if strings.Contains(model, "haiku") { inputPrice = 0.25; outputPrice = 1.25 }
    else if strings.Contains(model, "flash") { inputPrice = 0.075; outputPrice = 0.3 }
    else if strings.Contains(model, "pro") { inputPrice = 1.25; outputPrice = 5.0 }
    return (float64(input)*inputPrice + float64(output)*outputPrice) / 1_000_000.0
}
```

This feeds `EstimatedCostUSD`, compared against `config.CostBudgetUSD`
(`config/types.go:257`) in `checkThresholds` (`capacity_monitor.go:243`):
`if m.config.CostBudgetUSD > 0 && limits.EstimatedCostUSD >=
m.config.CostBudgetUSD { return "cost_budget_exceeded" }`. That trigger flows
into `handleTransitionTrigger`, which can pause/stop or auto-switch a live
autonomous session (`capacity_monitor.go:305-311`).

Two properties make this **worse** than the bug this project fixes, not
milder:
1. It does not fail visibly to `$0` — an unrecognized model (e.g.
   `claude-sonnet-5`, since the substring checks only match
   `opus`/`haiku`/`flash`/`pro`) falls through to the `"sonnet default"`
   `$3.00`/`$15.00` rate, which may be **wrong**, not merely missing, and
   there is no signal anywhere that this happened.
2. It drives a real, live automated action today (pausing/switching
   autonomous sessions on budget), unlike the Insights dashboard which is
   read-only reporting.
3. It also prices **non-Claude models** (`flash`, `pro` — Gemini families),
   which `session/tokens/pricing.go`'s `PricingTable` does not and, per this
   project's Non-Goals ("Pricing for non-Claude models/providers, unless the
   codebase already tracks them in the same table"), is explicitly not meant
   to start doing as part of this change.

## Decision

**Descope from `insights-cost-pricing-gaps`. File as an explicit fast-follow
bug**, not silently leave unaddressed.

Rationale:
- AC-1 through AC-7 as written only reference `session/tokens/pricing.go` and
  the Insights dashboard's read path — `capacity_monitor.go` is not named
  anywhere in `requirements.md`.
- Folding a fix in now would mean either (a) extending
  `tokens.PricingTable` to cover non-Claude providers (`flash`, `pro`),
  directly conflicting with this project's own Non-Goal, or (b) building a
  Claude-only partial fix for `CapacityMonitor.estimateCost` that still
  leaves Gemini models on the old substring-matched table — a half-measure
  that doesn't close the real risk and adds scope/review surface to a PR
  that's meant to be a targeted pricing-visibility fix.
- Silently doing nothing and saying nothing would risk the bug being mistaken
  as "already covered" once G1-G4 ship, since both tables live under the
  informal umbrella of "session/model cost estimation" — this ADR exists
  specifically to prevent that outcome.

## Consequences

- No code in `server/services/capacity_monitor.go` changes as part of this
  project.
- A fast-follow item must be filed (via `pm:log-bug` or as a new backlog
  item) covering: (1) `CapacityMonitor.estimateCost`'s substring-match
  fallback silently mis-prices any Claude model not matching
  `opus`/`haiku`/`flash`/`pro` at the generic "sonnet default" rate rather
  than flagging it as unknown, and (2) whether/how to unify it with
  `tokens.PricingTable` for Claude models while keeping non-Claude provider
  pricing (Gemini `flash`/`pro`) on its own path, given `tokens.PricingTable`
  is Claude-scoped by this project's Non-Goals.
- `project_plans/insights-cost-pricing-gaps/implementation/plan.md`'s
  "Unresolved Questions" section records this decision as resolved (not an
  open question blocking any story in this plan) and points to this ADR.
