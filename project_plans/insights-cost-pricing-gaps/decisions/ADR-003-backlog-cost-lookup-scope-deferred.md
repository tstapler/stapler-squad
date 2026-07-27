# ADR-003: Backlog's Cost-Lookup Path Keeps the Silent-$0 Bug — Fast-Follow Required

**Date**: 2026-07-27 (added during plan repair)
**Status**: Accepted (scope deferred, not forgotten)

## Context

`architecture-review.md`'s Concerns section (and independently, Task 1.3.1g's
compile fix) surfaced that `EstimateCost` has a second real, shipped consumer
outside `session/tokens`/`insights_service.go`: Backlog's per-item cost
display.

```go
// server/services/backlog_service.go:399-429 (buildCostLookup)
// ... builds a func(tmuxUUID string) float64 closure that calls
// return pt.EstimateCost(r)  — discards any pricing-completeness signal

// server/services/backlog_service_query.go:455-477
// cost := s.pricing.EstimateCost(result)  — feeds SessionCostEntry / resp.TotalCostUsd
```

This is tracked in the repo's own registry as
`docs/registry/features/backlog/get-item-cost.json` (per `requirements.md`'s
Context table). Once Task 1.3.1g's minimal compile fix lands (`cost, _ :=
pt.EstimateCost(r)` at both sites — required regardless, or the build breaks),
Backlog's per-item cost display will keep silently rendering `$0.00` for a
Sonnet-5-only (or any future unpriced-family-only) session **forever**, even
after this project ships — a second live instance of the exact bug this
project exists to fix ("Sonnet 5 costs blank"), surviving under a different
feature surface.

## Options considered

1. **Fold in now** — thread `unpriced`/`UnpricedModels` through
   `buildCostLookup`'s return value and `SessionCostEntry`, plus a Backlog UI
   badge, mirroring Epic 1.3/Phase 2's treatment for Insights.
   - *Strength*: closes the second instance of the bug in the same PR, while
     the exact call sites are already open for Task 1.3.1g's compile fix —
     the marginal diff for the *backend* half looks small.
   - *Weakness*: `buildCostLookup` returns a bare `func(tmuxUUID string)
     float64` consumed by an unknown number of downstream call sites within
     `backlog_service.go` that were never audited as part of this project's
     research phase (`requirements.md`/`research/*.md` scope entirely
     excludes `backlog_service.go`) — changing its signature to
     `func(tmuxUUID string) (float64, []string)` ripples into every caller,
     none of which this plan's research, ADRs, or reviews have examined for
     correctness risk. `SessionCostEntry`'s proto shape (if it's proto-backed)
     would also need a schema change of its own outside `insights.proto`,
     requiring its own `make proto-gen` + call-site + frontend badge work —
     effectively a second, unscoped copy of Epic 1.3 + a slice of Phase 2,
     bolted onto a PR whose own reviews (`architecture-review.md`,
     `adversarial-review.md`) were run against a fixed, already-large scope.
     This is the same shape of risk ADR-002 already identified and rejected
     for `capacity_monitor.go`: expanding scope mid-flight into a
     never-researched consumer, under review-cycle time pressure, is how
     half-measures and un-reviewed regressions get introduced.

2. **Descope as an explicit fast-follow**, filed the same way as ADR-002's
   `capacity_monitor.go` deferral — Task 1.3.1g does the minimal compile fix
   only (`cost, _ := ...`), and a backlog item is filed to bring
   `buildCostLookup`/`SessionCostEntry` up to the same unpriced-signal
   standard as Insights.
   - *Strength*: keeps this PR's diff matched to what `architecture-review.md`
     and `adversarial-review.md` actually reviewed — no unreviewed schema/UI
     work added at the last mile. Mirrors ADR-002's own reasoning exactly:
     "a half-measure... adds scope/review surface to a PR that's meant to be
     a targeted pricing-visibility fix."
   - *Weakness*: the Backlog UI keeps showing `$0.00` for unpriced-only
     sessions for one more release cycle. Lower severity than
     `capacity_monitor.go` (ADR-002): this is a read-only display bug, not a
     live automated action (no pause/stop trigger depends on Backlog's cost
     figure), so the deferral window carries less risk than ADR-002's.

## Decision

**Option 2 — descope, file as an explicit fast-follow.** Task 1.3.1g
performs only the minimal compile fix (`cost, _ := pt.EstimateCost(r)` /
`cost, _ := s.pricing.EstimateCost(result)`) at both call sites, discarding
the unpriced signal. A fast-follow backlog item must be filed covering:
threading `unpriced`/`UnpricedModels` through `buildCostLookup`'s return
value and `SessionCostEntry`, plus a Backlog UI indicator mirroring Epic
2.3's `SessionsTable.tsx` badge treatment — scoped and reviewed on its own,
the same way this project scoped and reviewed the Insights half.

Rationale mirrors ADR-002's directly: `requirements.md`'s AC-1 through AC-7
only name `session/tokens/pricing.go` and the Insights dashboard's read path;
`backlog_service.go`/`backlog_service_query.go` are not named anywhere in
`requirements.md`, and no research document in this project audited
`buildCostLookup`'s callers or `SessionCostEntry`'s full shape. Folding the
fix in now would mean doing that audit and review work under this PR's time
pressure rather than as its own properly-scoped change — precisely the
half-measure risk ADR-002 already rejected for a structurally similar case.

## Consequences

- No behavior change to `BacklogService.buildCostLookup`'s return shape or
  `SessionCostEntry`'s fields as part of this project — only the minimal
  `cost, _ := ...` compile fix (Task 1.3.1g) lands.
- Backlog's per-item cost display continues to render `$0.00` (indistinguishable
  from genuinely-zero cost) for sessions whose only usage is an unpriced
  model family, until the fast-follow ships. Lower severity than
  `capacity_monitor.go`'s deferred gap (ADR-002): this is read-only display,
  not a live automated trigger.
- A fast-follow item must be filed (via `pm:log-bug` or a new backlog item)
  covering: (1) threading the unpriced signal through `buildCostLookup` /
  `SessionCostEntry`, (2) a Backlog UI badge/indicator mirroring
  `SessionsTable.tsx`'s treatment (Epic 2.3), and (3) auditing
  `buildCostLookup`'s other callers within `backlog_service.go` that this
  project's research never scoped.
- `plan.md`'s Unresolved Questions section records this decision as resolved
  (not an open question blocking any story in this plan) and points to this
  ADR, mirroring how ADR-002 is referenced there.
