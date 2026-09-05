# ADR-002: Finding Dollar-Impact and Waste Score Are Non-Summable by Convention

**Date**: 2026-09-03
**Status**: Accepted

## Context

Each waste finding's dollar impact is a counterfactual estimate against one
specific baseline (e.g. "if this session's cache-hit rate matched the 40% floor,
it would have cost $X less"). Different findings on the same session use
different baselines and routinely double-count the same root cause — a session
with a bloated `CLAUDE.md` will likely also breach the cache-hit floor and the
session-token ceiling, all three findings pointing at the same underlying
problem. `requirements.md`'s Rabbit Holes section and `research/pitfalls.md` §1
both flag the standard failure mode this produces: a reader sums the `$` column
themselves the instant a UI implies it's summable (e.g. a visible total row),
producing a "potential savings" number with no basis. The waste score has the
same problem one level up — it must be "one heuristic number with a defined
formula, not an implied sum" (`requirements.md`:67).

## Options considered

1. **Do nothing beyond a UI note** — render a disclaimer near the findings panel,
   keep `DollarImpact`/`WasteScore` as plain `float64`.
   - *Strength*: zero extra code.
   - *Weakness*: a disclaimer a screen away from the numbers is exactly the
     failure mode `research/pitfalls.md` §1 describes — nothing stops a future
     PR from adding `Σ finding.DollarImpact` as a one-liner once someone wants a
     "total potential savings" card.

2. **Distinct named Go types (`DollarImpact`, `WasteScore`) with no exported
   `Sum`/`Add` helper, enforced by review convention.**
   - *Strength*: matches the `type-driven-design` skill's "illegal states
     unrepresentable" principle as far as Go's type system allows; a
     `DollarImpact` value can't be silently assigned into a plain `float64`
     total or a currency field without an explicit conversion, which is a
     visible, greppable moment in a diff.
   - *Weakness (must be stated honestly)*: Go's arithmetic operators work on any
     type whose underlying kind is numeric — a `DollarImpact` newtype **can**
     still be added with `+` to another `DollarImpact`. The type system cannot
     forbid that the way a sealed sum type can forbid an illegal state. The real
     enforcement is: never export a `Sum`/`Total` helper over `[]Finding`, and
     any aggregate dollar figure must be computed independently from raw session
     data (e.g. "session cost minus one specific alternative scenario"), never
     by iterating findings. This is a procedural guardrail backed by a type
     rename, not a compile-time guarantee — call it what it is in code review,
     not a silver bullet.

3. **A full wrapper struct with an explicit "not summable" marker field** (e.g.
   `{USD float64; Baseline string}`).
   - *Strength*: self-documents which counterfactual baseline produced the
     number.
   - *Weakness*: ceremony without a corresponding payoff here — the baseline is
     already implied by `FindingType`, and this repo's
     `.claude/rules/interface-pollution-checklist.md`-style guidance (see
     `ADR-001-unpriced-signal-return-shape.md` in `insights-cost-pricing-gaps`
     for the precedent) warns against a wrapper whose only job is re-exposing a
     float with an extra label.

## Decision

**Option 2.** `session/tokens/findings.go` defines:

```go
type DollarImpact float64
type WasteScore float64
```

Neither type gets an exported `Sum`, `Add`, or `Total` function anywhere in the
codebase. The only aggregate dollar figures the findings panel or dashboard may
show are computed by dedicated, independently-named functions operating on raw
`ParseResult`/pricing data (e.g. `ComputeCacheROI`), never by folding over
`[]Finding`. This mirrors ADR-001's tool-cost caveat in spirit: the fix is an
honest, visible convention plus a naming signal, not a claim that Go's compiler
enforces non-summability — it does not, and this ADR says so explicitly rather
than overstating the guarantee.

## Consequences

- Code review must treat `for _, f := range findings { total += float64(f.DollarImpact) }`
  (or equivalent) as a rejection-worthy pattern, not a style nit — it reintroduces
  the exact number the reference tools (`cacheeconomics`, `TokenUsage`) warn is
  misleading.
- The findings panel UI must not render a "total potential savings" row derived
  from the visible findings list. If a dashboard-level "estimated total waste"
  figure is ever wanted, it needs its own Phase-2-style research pass on what
  baseline it should use — it is explicitly not in this project's scope.
- `WasteScore` must never appear in a `$`-labeled UI element or participate in
  arithmetic with `DollarImpact`/`EstimatedCostUsd` — enforced the same way, by
  convention plus the type name signaling intent to reviewers.
