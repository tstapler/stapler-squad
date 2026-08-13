# BUG-049: Backlog per-item cost display silently renders $0.00 for unpriced model families [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-07-27
**Impact**: Any Backlog item whose session usage is entirely on an unpriced model family (e.g. a Claude model missing from `DefaultPricingTable()`) shows a per-item cost of `$0.00` in the Backlog UI, indistinguishable from a genuinely free/zero-cost session. Affects anyone reading Backlog cost figures to gauge spend — the number silently understates cost rather than signaling "unknown."

## Problem Description

`BacklogService.buildCostLookup` (`server/services/backlog_service.go:399-429`) builds a `func(tmuxUUID string) float64` closure that calls `pt.EstimateCost(r)` and discards any pricing-completeness signal. Its cost query counterpart, `server/services/backlog_service_query.go:455-477`, does the same — `cost := s.pricing.EstimateCost(result)` feeds `SessionCostEntry` / `resp.TotalCostUsd` with no unpriced-model indicator.

This is the identical bug class the `insights-cost-pricing-gaps` project fixed for the Insights dashboard (the original "Sonnet 5 costs blank" issue): when `EstimateCost` encounters a model not present in `DefaultPricingTable()`, it silently contributes `$0` to the total instead of surfacing that the figure is incomplete. That project's fix threaded an `unpriced`/`UnpricedModels` signal through the Insights read path and added a UI badge (`SessionsTable.tsx`, Epic 2.3). The Backlog UI's per-item cost display was explicitly left out of that project's scope — per `EstimateCost`'s signature change, Backlog's two call sites only received the minimal compile fix (`cost, _ := pt.EstimateCost(r)` / `cost, _ := s.pricing.EstimateCost(result)`), discarding the new unpriced signal rather than surfacing it.

This bug is a deliberate, documented scope deferral, not an oversight: see `project_plans/insights-cost-pricing-gaps/decisions/ADR-003-backlog-cost-lookup-scope-deferred.md` for the full rationale (Option 2 — descope as an explicit fast-follow, mirroring ADR-002's treatment of `capacity_monitor.go`). This bug is the filed fast-follow that ADR-003 requires.

## Reproduction Steps

1. Create/observe a Backlog item whose session token usage is entirely on a model family absent from `session/tokens`'s `DefaultPricingTable()` (e.g. a newly released Claude model not yet added to the pricing table).
2. View that item's cost figure in the Backlog UI (per-item cost display) or via the cost query response (`SessionCostEntry` / `TotalCostUsd`).
3. Expected: the UI indicates the cost figure is incomplete/unknown for that item (unpriced-model signal surfaced, mirroring Insights' `SessionsTable.tsx` badge treatment).
4. Actual: the item silently shows `$0.00`, identical in appearance to a session that genuinely cost nothing.

## Root Cause

`EstimateCost`'s two-value return (`(cost float64, unpriced []string)` or equivalent completeness signal, introduced by the `insights-cost-pricing-gaps` project) is called at both Backlog sites but the second return value is discarded (`cost, _ := ...`). Neither `buildCostLookup`'s closure signature (`func(tmuxUUID string) float64`) nor `SessionCostEntry`'s shape carries an unpriced-model indicator, so there is no path for the signal to reach the Backlog UI even if the call sites were fixed to retain it.

## Files Likely Affected

- `server/services/backlog_service.go:399-429` (`buildCostLookup`) — closure discards the unpriced signal from `EstimateCost`; return shape would need to become `func(tmuxUUID string) (float64, []string)` (or similar) to carry it.
- `server/services/backlog_service_query.go:455-477` — `s.pricing.EstimateCost(result)` call site feeding `SessionCostEntry` / `resp.TotalCostUsd`; needs the same treatment.
- `SessionCostEntry` (proto-backed, exact location TBD — needs its own `make proto-gen` if a new field is added) — would need an unpriced-model field to carry the signal to the frontend.
- Backlog UI per-item cost display component (frontend, exact component TBD — not yet audited) — would need a badge/indicator mirroring `SessionsTable.tsx`'s treatment (Epic 2.3 of `insights-cost-pricing-gaps`).
- `docs/registry/features/backlog/get-item-cost.json` — registry entry tracking this feature (per `insights-cost-pricing-gaps`'s `requirements.md` Context table).

## Fix Approach

Mirror the `insights-cost-pricing-gaps` project's Epic 1.3/Phase 2 treatment for Insights, scoped independently for Backlog:

1. Thread the unpriced-model signal through `buildCostLookup`'s return value and through `SessionCostEntry` (proto schema change → `make proto-gen`).
2. Add a Backlog UI indicator/badge for unpriced-cost items, mirroring `SessionsTable.tsx`'s badge treatment.
3. Audit `buildCostLookup`'s other callers within `backlog_service.go` — `insights-cost-pricing-gaps`'s research phase never scoped or examined these callers, so a correctness pass is needed before changing the closure's signature.

This should go through its own SDD pass (research/plan/validate) rather than a quick patch, given the proto/schema surface and unaudited caller set — see ADR-003 Option 1's rejected "fold in now" analysis for why this was deferred rather than done inline.

## Verification

- A session whose entire usage is on an unpriced model family shows an explicit "cost unknown" / unpriced indicator in the Backlog UI, not a bare `$0.00`.
- `SessionCostEntry` (or equivalent) carries the unpriced-model signal end to end from `EstimateCost` through the RPC response.
- Existing Backlog cost display tests continue to pass for genuinely-zero-cost and fully-priced sessions (no regression to the normal-cost display path).

## Related Tasks

- ADR: `project_plans/insights-cost-pricing-gaps/decisions/ADR-003-backlog-cost-lookup-scope-deferred.md` — full context and rationale for this deferral.
- Sibling deferral: ADR-002 (`capacity_monitor.go`), same project — analogous fast-follow pattern for a different consumer of `EstimateCost`.
- Filed as part of `insights-cost-pricing-gaps` Epic 3.2 / Story 3.2.1 / Task 3.2.1a (`project_plans/insights-cost-pricing-gaps/implementation/plan.md:716-731`).
