# ADR-001: Unpriced-Model Signal as an Additive Second Return Value

**Date**: 2026-07-27
**Status**: Accepted

## Context

`session/tokens/pricing.go`'s `EstimateCost(r *ParseResult) float64` and
`ModelFamilyCost(r *ParseResult) map[string]float64` silently skip any model
family absent from `PricingTable.Prices` (`pricing, ok := pt.Prices[family];
if !ok { continue }` at `pricing.go:170-173` and `:212-215`/`:226-227`),
contributing `0.0` with no signal distinguishing "genuinely free/zero usage"
from "usage present but unpriced." This is the root cause of the reported bug
("Sonnet 5 costs blank") and is confirmed as an intentional, tested fallback
(`pricing_test.go:51-63`).

Fixing this (G2 in requirements.md) requires a way for callers
(`server/services/insights_service.go` lines 115, 179, 192, 314) to detect
which families were skipped, then propagate that fact through
`proto/session/v1/insights.proto` to the frontend.

## Options considered

1. **Additive second return value** — `EstimateCost(r) (cost float64,
   unpriced []string)`, `ModelFamilyCost(r) (costs map[string]float64,
   unpriced map[string]bool)`.
   - *Strength*: idiomatic Go (mirrors `LookupByModel(modelID string)
     (ModelPricing, bool)` one function below, `pricing.go:241-245`); every
     call site consumes both values immediately, so there is no need to keep
     a bundled value+status pair alive across a boundary; a pure signature
     change, no new type.
   - *Weakness*: touches every existing call site (3 in `insights_service.go`
     plus 3 in `pricing_test.go`) — a mechanical but real diff footprint.

2. **Sentinel value** — e.g. `EstimatedCostUsd == -1` means "unpriced," or a
   magic string.
   - *Strength*: zero signature changes, no new fields anywhere.
   - *Weakness*: a `-1.0` (or any sentinel) in a `double` field is silently
     summed by any `total += cost` that doesn't special-case it — reintroducing
     the exact class of silent-corruption bug this project exists to fix, and
     violates this repo's general preference for explicit signals over magic
     numbers (parallel: the CSS rule against hardcoded `zIndex` magic numbers).

3. **Full wrapper type** — `type CostEstimate struct { USD float64; Complete
   bool }` threaded through `EstimateCost` → `insights_service.go` → proto →
   frontend.
   - *Strength*: bundles value and provenance in one place, can't be
     accidentally separated in Go code.
   - *Weakness*: a proto message can't embed a Go struct, so the wrapper is
     destructured back into two flat fields at the one real serialization
     boundary anyway — ceremony without payoff, and a violation of
     `.claude/rules/interface-pollution-checklist.md` smell #6
     ("struct-wraps-struct-wraps-struct... each layer re-exposes the layer
     below without adding new exported behavior").

## Decision

**Option 1 — additive second return value.** `EstimateCost` returns
`(cost float64, unpriced []string)`; `ModelFamilyCost` returns
`(costs map[string]float64, unpriced map[string]bool)` (a map because the
caller needs O(1) per-family membership checks when building
`ModelBreakdown`/`DailyTokenBucket` rows, not an ordered list).

Both methods keep their existing per-family loop structure intact
(`session/tokens/pricing.go:168-178` and `:210-234`) and additively collect
the skipped-family set alongside the existing accumulation — per
`research/pitfalls.md` §3, restructuring the loop bodies while retrofitting
this signal risks corrupting the already-correct priced-model totals
(`TestEstimateCost_WhenKnownModel_ExpectExactPrice`,
`TestEstimateCost_WhenCacheReadTokens_ExpectCacheRateIncluded`), which would
be a strictly worse regression than the bug being fixed.

`ModelFamilyCost` has **two independent** `pt.Prices[family]` lookup sites —
a per-turn loop (`pricing.go:210-221`) and a separate `PrimaryModel`-fallback
branch used when `TurnTimeline` is empty (`pricing.go:224-234`) — each doing
its own `if !ok { ... }` skip. Both must gain the unpriced-collection logic,
or the fallback path will silently regress back to old behavior while the
primary path is fixed.

`EstimateCost`, by contrast, has only **one** lookup site: its
`PrimaryModel`-fallback branch (`pricing.go:160-166`) populates the same
`modelInputs`/`modelOutputs`/etc. maps that its single pricing loop
(`pricing.go:168-178`) consumes, rather than doing its own separate lookup —
confirmed by direct trace, and a correction to `research/pitfalls.md` §3's
framing, which describes the duplication as applying symmetrically to both
functions. Only `ModelFamilyCost` needs the two-spot fix; `EstimateCost`
needs the fix in exactly one place.

## Consequences

- `proto/session/v1/insights.proto` gains a `bool pricing_unavailable` field
  on `ModelBreakdown` and `repeated string unpriced_models` fields on
  `SessionTokenSummary`, `DailyTokenBucket`, and `GetInsightsSummaryResponse`
  — flat fields, no new message type.
- All 4 backend call sites in `server/services/insights_service.go` (lines
  115, 179, 192, 314) and all 3 existing tests in
  `session/tokens/pricing_test.go` that call `EstimateCost` must be updated
  for the new signature in the same change (compile-breaking otherwise).
- **Correction (recorded during plan repair, 2026-07-27, per `architecture-review.md`'s BLOCKER finding)**: the paragraph originally here claimed no other consumer of `PricingTable` was "forced to adopt" the new return value because "Go allows ignoring a second return value entirely." **That is incorrect as a claim about existing call sites.** Go allows a *newly written* call site to discard a second return value (`cost, _ := ...`), but it does **not** allow an *existing* single-value-assignment call site to keep compiling unmodified after the function it calls gains a return value — the call site itself must be edited. There are two such existing call sites outside `insights_service.go`/`pricing_test.go`, confirmed by grep: `server/services/backlog_service.go:429` (`return pt.EstimateCost(r)` inside `BacklogService.buildCostLookup`'s closure, which returns plain `float64` — this becomes a "too many return values" compile error) and `server/services/backlog_service_query.go:471` (`cost := s.pricing.EstimateCost(result)` — this becomes an "assignment mismatch" compile error). Both require a trivial one-line edit (`cost, _ := ...`) in the **same commit** as Task 1.3.1a, not a separate follow-up — `plan.md`'s Task 1.3.1g is the task that makes this edit, and Task 1.3.1h adds a `go build ./...` checkpoint immediately after to catch any other stray call site. Neither of these two call sites needs the unpriced signal itself (they discard it via `_`); whether Backlog's cost display should *surface* unpriced usage is a separate scope decision — see `ADR-003-backlog-cost-lookup-scope-deferred.md`.
