# ADR-003: Soft Budget Warning Evaluates Inline at Cost-Recording Time, as a New, Separate Comparison — Not the ADR-029 Batched Snapshot, Not a `CapacityMonitor` Extension

## Status
Accepted

## Context
Two existing mechanisms in this codebase look superficially reusable for a
"per-item/per-stage soft budget warning," and pitfalls research (§3b, §3d)
flagged both as the wrong fit if reused wholesale:

1. **ADR-029's batched-snapshot pattern**
   (`project_plans/insights-session-visibility/decisions/ADR-029-session-role-batched-snapshot-not-live-join.md`)
   — `session_role` is fetched via one unfiltered scan per Insights request,
   accepted as "eventually consistent" because `session_role` is
   write-once/immutable. A budget *warning*'s entire value is in firing
   promptly; sourcing it from the same request-scoped, only-computed-when-a-
   dashboard-loads snapshot would silently under-warn for however long a
   user leaves the Insights tab closed.
2. **`CapacityMonitor.checkThresholds`** (`server/services/capacity_monitor.go:229`)
   — already does inline threshold comparison against `CostBudgetUSD`, but
   its consequence is **enforcement** (pausing/stopping a live autonomous
   session, per BUG-050's incident history) via its own, already-diverged
   pricing table. Folding this project's **advisory-only** warning into that
   same code path would conflate two different responsibilities (soft
   visibility vs. hard-ish enforcement) and risks re-introducing BUG-050's
   exact failure class (an independent pricing/threshold surface drifting
   from the canonical one) one layer up, in the threshold-comparison logic
   instead of the per-token pricing.

## Decision
Build a new, narrow, **advisory-only** threshold check —
`session.EvaluateBudgetThreshold(itemID, stage string, thresholdUSD *float64,
cumulativeSpentUSD float64) (warn bool)` — invoked **inline, synchronously, at
the point a headless call's cost is recorded** (the existing `CostSink`
callback already threaded through `CallBlocking`,
`session/headless/caller.go:727`) and, for the work stage, at the point its
live cost is next recomputed and read. This codebase has no periodic ticker
that persists a work-stage session's cost — confirmed by direct read:
`ItemSession.estimated_cost_usd` is populated for headless sessions only
(`session/ent/schema/item_session.go:80-83`'s own schema comment). A
work-stage session's cost is instead recomputed fresh from its transcript on
every read via `server/services/backlog_service.go`'s `buildCostLookup()` +
`itemSessionToProto`, consumed by `GetBacklogItem` and `WatchBacklogItems`'s
snapshot/live-event paths — that recompute-on-read cadence, not a dedicated
ticker, is what "periodically re-read" means here (plan.md Story 4.2.3). It:
- Reads from `session/tokens/pricing.go`'s canonical pricing table — never a
  second table.
- Compares against `BacklogItemData.CostBudgetThresholdUsd` (new, per-item,
  optional — see plan.md Story 4.2.1), not a global config value.
- Never pauses, blocks, or retries anything — it only logs a Warn (mirroring
  `CachingPipelineEngine`'s existing `[PipelineEngine]`-prefixed Warn-log
  convention) and surfaces via the existing Insights read path on next load,
  satisfying "visibly, not silently" without pretending to be real-time push
  notification infrastructure.

## Rationale
- **Different consistency requirement, so a different mechanism — not a
  fork of either existing one.** ADR-029's staleness tolerance is fine for a
  read-only display field; it is not fine for something whose entire job is
  "notice promptly." `CapacityMonitor`'s enforcement semantics are a
  different responsibility than an advisory warning; conflating them would
  make a future change to auto-pause behavior also silently change the
  soft-warning's behavior, and vice versa.
- **One pricing source, not three.** `insights-cost-pricing-gaps` already
  found and only partially reconciled two independently-drifting pricing
  surfaces (`session/tokens/pricing.go` and `capacity_monitor.go`'s own
  substring-matching table, BUG-050). This project must not add a third.
  Building the new threshold check directly against `session/tokens/pricing.go`
  keeps it at two, not three, and does not require fixing BUG-050 as a
  prerequisite (that remains ADR-002-unpriced-signal-return-shape's/
  `insights-cost-pricing-gaps`' own deferred scope).

## Consequences
- `session.EvaluateBudgetThreshold` is a small, pure function — easy to unit
  test in isolation with fixed spend/threshold values, no dependency on
  `CapacityMonitor` or the Insights batched-snapshot machinery.
- A future project that wants to unify soft-warning and hard-enforcement
  threshold comparison can do so deliberately, with both call sites already
  isolated behind named functions (`EvaluateBudgetThreshold` vs.
  `CapacityMonitor.checkThresholds`) rather than tangled together from the
  start.
- The soft warning's log line and Insights-surfaced state can lag a live
  session's true current spend by however long it takes the next headless
  call (or work-stage cost refresh tick) to fire — acceptable per the
  Out-of-Scope "no hard enforcement" framing and pitfalls research's edge
  case 4 (a threshold can only ever be crossed by a call that has already
  completed and been paid for; there is no pre-flight cost estimate
  anywhere in this codebase's headless path today).
