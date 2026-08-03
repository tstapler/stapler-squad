# Token Cost Tracking Per Session — Requirements

## Source

Backlog item `f5f6d339-c741-4f34-ab39-db60fc619b0b`, migrated from
[TylerStaplerAtFanatics/stapler-squad#181](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/181)
(created 2026-07-10). Skipped the interactive ideation interview — no user present in
this session; requirements below are derived from the item's description plus
pre-implementation codebase triage.

## Problem Statement (from the original item — superseded, kept for provenance only)

> Users running Claude Code or other API-backed agents through stapler-squad have no
> visibility into token consumption or API costs per session. Costs are only
> discoverable after the fact from provider dashboards.

**This framing is stale as of this project's triage** (see below) — the actual current
problem this project solves is narrower: closing 5 specific display/sort/test-coverage
gaps in a token-cost feature that already shipped. Left verbatim above only so the
original item's ask is traceable; do not treat it as the live problem statement.

## IMPORTANT: This feature is already substantially implemented

Pre-implementation triage (2026-08-01) found that the three phases proposed in the
original item — storage, UI display, pricing — already exist in the codebase, built
across a separate, more-recent line of work (`project_plans/token-monitoring/`,
`project_plans/insights-cost-pricing-gaps/`, PRs #104, #98, #280 — most recent
2026-07-27, 5 days before this triage):

| Original ask | Status | Where |
|---|---|---|
| Parse Claude Code JSONL for token usage | **Done** | `session/tokens/parser.go`, `jsonl_types.go` |
| Store cumulative token_usage per session | **Done (different design)** | `session/tokens/store.go` — in-memory `TokenStore` cache keyed off JSONL files + fsnotify, not new ent schema columns. Avoids duplicating data JSONL already holds. |
| Session card token badge | **Component built, NOT wired** *(corrected after research — see below)* | `web-app/src/components/shared/TokenBadge.tsx` exists and is unit-tested but is imported nowhere except its own test file; `SessionList.tsx` has zero token/cost data today |
| Session detail view, per-turn breakdown | **Partial** | `SessionDetailDrawer.tsx` shows session-level rollup (cost, cache hit rate, tool/skill breakdown) but **no per-turn/per-message table** |
| Sessions list sortable by token consumption | **Missing** | `SessionList.tsx` sort dropdown has no tokens/cost option; `insights/SessionsTable.tsx` is hard-sorted by `lastMessageAt`, not user-sortable |
| Pricing table (Anthropic model pricing) | **Done** | `session/tokens/pricing.go` — `DefaultPricingTable()`, override mechanism, "pricing unavailable" flagging (closed by `insights-cost-pricing-gaps`) |
| Estimated USD cost per session | **Done** | `insights_service.go` (`GetInsightsSummary`, `ListSessionTokens`), `ProjectedCostCard.tsx`, `SummaryCards.tsx` |
| Dashboard (list, trends, top-N, model breakdown) | **Done** | `web-app/src/app/insights/*` — full `/insights` route |

Given this, this triage reframes the item from "build token cost tracking" to
**"close the remaining gaps in the already-shipped token cost tracking feature"**,
plus close two test/registry hygiene gaps found in review. Re-implementing the storage
layer, pricing table, or dashboard shell is explicitly out of scope — it exists and
works.

## Correction after Phase 2 research

Research (`research/features.md`, `research/architecture.md`, `research/pitfalls.md`)
found the pre-research triage overstated AC-2's starting point: `TokenBadge.tsx` is
built and tested but **never rendered** in the app (`grep -rn "TokenBadge" web-app/src`
matches only the component and its own test). `SessionList.tsx`'s `Session[]` data has
no cost/token fields at all — `Session` (proto) carries none. AC-2 therefore requires
new data plumbing (fetch `ListSessionTokens`/`GetInsightsSummary`, join by
`session_id`), not just a new sort option on already-displayed data. See
`research/architecture.md` §AC-2/AC-3 and `research/pitfalls.md` §3 for the full
"jumping list" risk this implies. AC-2's wording below is updated accordingly.

## Confirmed Gaps (drive the acceptance criteria)

1. **No per-turn token breakdown in session detail.** `SessionDetailDrawer.tsx` has
   session totals and a tools/skills breakdown but nothing iterates per-message
   (`TurnStats` is already computed by the parser and available on `ParseResult.TurnTimeline`
   — this is a wiring gap, not a data gap).
2. **Session list (both `SessionList.tsx` and `insights/SessionsTable.tsx`) is not
   sortable by token/cost consumption**, despite `SessionsTable.tsx` already rendering
   Input/Output/Cache/Cost columns.
3. **`WatchInsights` RPC has no test coverage** (`docs/registry/features/backend/WatchInsights.json`:
   `tested: false`).
4. **Missing feature-registry entries** for `SessionDetailDrawer`, `ProjectedCostCard`,
   `DailySpendChart`, `ModelOverTimeChart`, `SessionsTable` — they exist and work but
   aren't tracked per this repo's `feature-registry.md` rule, so they don't show up in
   coverage-gap reporting.
5. **Cache hit rate not surfaced above the per-session level** — `ModelBreakdownChart.tsx`
   and `ModelOverTimeChart.tsx` show raw token totals but not cache-creation-vs-read
   split/hit rate, which only appears in `SessionDetailDrawer.tsx` and `SessionsTable.tsx`.

## Non-Goals

- Rebuilding JSONL parsing, the pricing table, or the `/insights` dashboard shell —
  already shipped and tested.
- Adding a new ent schema `token_usage` column set — the JSONL-file-backed `TokenStore`
  design already serves this purpose and duplicating it in ent would be redundant
  storage with a sync-consistency problem.
- Real-time token streaming mid-session (explicit non-goal carried over from
  `token-monitoring/requirements.md`).
- Multi-user/team spend aggregation, live Anthropic billing reconciliation (carried
  over non-goals from `token-monitoring/requirements.md`).

## Acceptance Criteria

| ID | Criterion |
|---|---|
| AC-1 | `SessionDetailDrawer.tsx` (or a sub-panel within it) renders a per-turn/per-message token breakdown table (timestamp, model, input/output/cache tokens) sourced from `ParseResult.TurnTimeline`, for sessions where turn data is available |
| AC-2 | The main `SessionList.tsx` fetches per-session cost data (join by `session_id`) and its sort dropdown gains a "Sort: Cost" option that sorts sessions by `estimatedCostUsd`, with unpriced/not-yet-loaded sessions sorted last (not interspersed at `$0`) |
| AC-3 | `insights/SessionsTable.tsx` supports user-driven sorting by at least one of Input/Output/Cache/Cost columns (click-to-sort or equivalent), not just the hardcoded `lastMessageAt` order |
| AC-4 | `WatchInsights` gains a Go test covering at least one streaming update cycle; `docs/registry/features/backend/WatchInsights.json` updated to `tested: true` with the new testId |
| AC-5 | New per-feature registry files created under `docs/registry/features/frontend/` for `SessionDetailDrawer`, `ProjectedCostCard`, `DailySpendChart`, `ModelOverTimeChart`, and `SessionsTable` per `.claude/rules/feature-registry.md`; `make registry-generate` run afterward with no unexplained growth in `coverage-gaps.json` |
| AC-6 | `ModelBreakdownChart.tsx` or `ModelOverTimeChart.tsx` surfaces a cache hit rate (or cache-creation-vs-read split) at the aggregate/model level, not only per-session |
| AC-7 | `make quick-check` (build + test + lint) passes after all changes |

## Reference

- `project_plans/token-monitoring/requirements.md` — original full-dashboard requirements (mostly shipped)
- `project_plans/insights-cost-pricing-gaps/` — most recent related work (PR #280, 2026-07-27)
- `session/tokens/` — parser, store, pricing, association, skill detection
- `server/services/insights_service.go` — `GetInsightsSummary`, `ListSessionTokens`, `WatchInsights`
- `web-app/src/app/insights/` — dashboard components
- `.claude/rules/feature-registry.md` — registry update requirements
