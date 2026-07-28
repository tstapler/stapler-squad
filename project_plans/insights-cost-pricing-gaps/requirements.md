# Insights Cost Pricing Gaps: Requirements

## Project
`insights-cost-pricing-gaps` — Fix blank/unaccounted-for costs on the Insights page (e.g. "Sonnet 5") by keeping the pricing lookup table current and giving unpriced-model usage a visible, non-silent representation instead of rendering as `$0.00`.

## Source Item
- item_id: `bcb20604-2660-4210-86dd-529f44c70966`
- Title: "Sonnet 5 costs blank"
- Description: "We need to actually improve the insights page so that we fix all the blank unaccounted for costs and make sure we can lookup the prices using our tokens" (with a screenshot showing blank/zero cost entries on the Insights page).

## Context & Current State (confirmed via code research)

| Layer | Current state | Problem |
|---|---|---|
| Pricing table | `session/tokens/pricing.go` `DefaultPricingTable()` (lines 23-77) — hardcoded map of model-family → $/MTok, dated "as of 2026-05-15" | Only covers `claude-{opus,sonnet,haiku}-{3,4}`. No entry for `claude-sonnet-5` / `claude-opus-5` / newer families. |
| Model normalization | `NormalizeModelFamily()` (lines 115-136) strips date/variant suffixes, e.g. `claude-sonnet-4-6-20250514` → `claude-sonnet-4` | Correctly normalizes `claude-sonnet-5-...` → `claude-sonnet-5`, but that key simply isn't in the table — normalization isn't the bug, missing table entries are. |
| Cost estimation | `EstimateCost()` (lines 140-181) and `ModelFamilyCost()` (lines 203-237) | Both do `pricing, ok := pt.Prices[family]; if !ok { continue }` — silently skip unknown families and contribute **$0.00** with no warning/error/indicator anywhere. Confirmed as an intentional, tested fallback (`pricing_test.go:51-63`, `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero`). |
| Override mechanism | `LoadPricingOverride()` (lines 79-102) merges a JSON override file on top of defaults | Exists but is **not wired up** — `server/dependencies.go:1070-1076` only calls `tokens.DefaultPricingTable()`. No live config path to patch in new models without a code change/redeploy. |
| Backend call sites | `server/services/insights_service.go:115, 179, 192, 314` | Per-session cost, daily cost, and model-breakdown cost all flow through the same silently-zeroing pricing lookup. |
| Frontend | `web-app/src/app/insights/` — `InsightsDashboard.tsx`, `SummaryCards.tsx`, `DailySpendChart.tsx`, `ModelBreakdownChart.tsx`, `SessionsTable.tsx`/`TopNTables.tsx`, `ModelOverTimeChart.tsx`; formatting via `insightsFormatters.ts:5` (`fmtCost`) | No "pricing unavailable" state exists. An unpriced model renders as `$0.000`, which reads as free/blank and is invisible as a bar in `ModelBreakdownChart.tsx`. |
| Registry | No `docs/registry/features/**` entry for cost/pricing exists (only `GetInsightsSummary` RPC + `insights-dashboard` UI shell, and an unrelated `backlog/get-item-cost.json`) | Pricing-table maintenance isn't tracked as its own feature; nothing prompts anyone to update it when Anthropic ships a new model family. |
| Docs | None in `docs/` or `.claude/docs/` describe the cost-calculation approach or the need to keep the pricing table updated | No documented process for adding new model pricing. |

## Goals

### G1 — Close the immediate pricing gap
Add current pricing entries for missing/newer model families (at minimum Sonnet 5, and any other Claude model families in active use that are missing) to `DefaultPricingTable()`.

### G2 — Make unknown-model cost visibly distinct from zero-cost
Stop conflating "$0 because pricing is missing" with "$0 because usage was actually free/zero." Surface unpriced usage distinctly end-to-end: backend response includes an "unpriced"/"unaccounted" signal (e.g. per-model or aggregate flag or a token-count-only fallback figure), and the Insights UI visibly indicates "pricing unavailable for `<model>`" instead of silently rendering `$0.00` or an invisible bar.

### G3 — Wire up (or replace) the pricing override mechanism
Make it possible to patch in pricing for a new model without a full code change + redeploy — either by actually wiring `LoadPricingOverride()` into `server/dependencies.go`, or by an equivalent low-friction mechanism (config file, env var, etc.) if investigation in the research phase finds a better-fitting approach already used elsewhere in the codebase.

### G4 — Prevent regression when new model families ship
Add a lightweight guardrail (test, lint check, or doc'd process) so that the next new Claude model family produces a visible signal rather than another silent blank-cost gap.

## Non-Goals
- Building a live/dynamic pricing-API integration (e.g. fetching current prices from Anthropic at runtime) — out of scope unless research finds this is trivially cheap; static table + override is sufficient.
- Redesigning the Insights page's charts/layout beyond what's needed to surface the "pricing unavailable" state.
- Historical cost re-computation/backfill for past sessions that were already recorded with $0 due to this gap (may be called out as a follow-up, not required here).
- Pricing for non-Claude models/providers, unless the codebase already tracks them in the same table.

## Acceptance Criteria

| ID | Criterion |
|---|---|
| AC-1 | `DefaultPricingTable()` includes pricing entries for `claude-sonnet-5` (and other currently-missing active model families identified during research) |
| AC-2 | `EstimateCost()` / `ModelFamilyCost()` (or their callers) distinguish "no usage" from "usage present but unpriced" — the unpriced case is detectable by the caller, not silently folded into `0.0` |
| AC-3 | Insights page visibly indicates when a model's cost is "unavailable"/"unaccounted" rather than rendering as a blank/zero-cost row or an invisible chart bar |
| AC-4 | A pricing-override or equivalent mechanism is wired into the running server so new model pricing can be added without a full code deploy, OR a documented decision explains why this is deferred |
| AC-5 | A test exists asserting that an unknown/unpriced model family is flagged as such (not just silently returning `0.0`) — extending or replacing `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero` as needed |
| AC-6 | `docs/registry/features/` gains or updates an entry reflecting the pricing/cost-lookup feature per this repo's feature-registry rule |
| AC-7 | `make quick-check` (build + test + lint) passes after the change |
