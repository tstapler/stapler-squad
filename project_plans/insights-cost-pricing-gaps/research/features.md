# Insights Cost Pricing Gaps — Feature Landscape Research (Agent 2)

## 1. Existing "we have data but can't fully interpret it" UX patterns in this codebase

Three reusable precedents exist already — none of them is a generic "unknown/N/A"
component, so the new pricing-gap UI will be a new but idiomatically-consistent piece:

| Pattern | Location | Shape | Reusable for pricing gap? |
|---|---|---|---|
| **Inline badge on a table cell** | `web-app/src/app/insights/SessionsTable.tsx:134-141` + `SessionsTable.css.ts:87` (`orphanBadge`) and `:171` (`backlogBadge`) | Small pill/badge rendered next to the primary cell value, e.g. `{shortId(...)}<span className={orphanBadge}>orphan</span>` | **Yes** — direct precedent for an `unpriced` badge next to a cost cell or model-family row. |
| **Dismissible alert callout with `role="alert"` / `aria-live="polite"`** | `web-app/src/components/sessions/MemoryPressureCallout.tsx` | Banner with icon+title, list of affected items, bulk action button, per-item dismiss (persisted to `sessionStorage`) | Good precedent for a page-level "N model(s) have no pricing data" callout above the charts, though the AC only requires visibility, not dismissal. |
| **Fallback placeholder string for missing scalar data** | `insightsFormatters.ts:25-32` (`fmtDate` returns `"—"` when timestamp is undefined) and `ModelBreakdownChart.tsx:52` (`m.modelFamily || "unknown"`) | Simple em-dash / "unknown" fallback text | Shows the codebase convention is em-dash (`—`) for "no value", not `$0.00` — reinforces that rendering unpriced cost as `$0.000` is the actual bug: the convention for "we don't know" already exists elsewhere and just wasn't applied to cost. |
| **Toggle to reveal/hide flagged rows** | `SessionsTable.tsx:45,225-232` (`showOrphans` state + "Show/Hide orphans (N)" button) | Same UI shape could gate an "unpriced" toggle/filter if the list of affected sessions/models is long | Optional nice-to-have, not required by AC but consistent extension point. |

There is **no existing generic `<Unavailable>`/`<Unknown>` component** — each screen invents
its own local convention. The new pricing-unavailable indicator should follow the badge +
optional callout pattern above rather than introduce a fourth pattern.

`ConnectionIndicator.tsx` (`web-app/src/components/layout/ConnectionIndicator.tsx`) also has
a "stale" state precedent (`ConnectionIndicator_should_renderDistinctStaleLabel_When_connectionStateIsStale`)
worth glancing at for wording/tone if a "pricing may be stale" signal is added (see §4 —
`PricingTable.IsStale()` already exists in Go but is completely unused).

## 2. "Orphaned sessions" — distinct root cause, NOT the same as unpriced-model cost

Confirmed: **two unrelated concepts**, both currently reported under the umbrella of
"unaccounted for" in the item description, but with independent code paths and independent
root causes.

- **Orphan** = a token-usage record (`ParseResult`, parsed from a Claude Code JSONL transcript
  under `~/.claude/projects/`) that could not be matched to any stapler-squad `Session` record.
  Computed by `session.tokens.Associator.Associate()` — `session/tokens/association.go:42-82`.
  Three-tier matching strategy: (1) exact conversation UUID match, (2) project-path prefix
  match, (3) file-mtime-vs-session-CreatedAt proximity within ±5 minutes. If none match,
  `isOrphan = true`. This has **nothing to do with pricing** — an orphaned session's cost is
  computed by the exact same `EstimateCost()`/`ModelFamilyCost()` pricing path as a linked one;
  orphan-ness only affects whether it's attributable to a stapler-squad session/backlog item in
  the UI, and whether it's filtered by default (`IncludeOrphans` flag, `SessionsTable.tsx:45`
  `showOrphans` defaults `true` but `InsightsDashboard.tsx:87` requests `includeOrphans: true`
  from the backend always — filtering happens client-side).
- **Unpriced model** = a `ParseResult`/turn whose `NormalizeModelFamily(turn.Model)` key is
  absent from `PricingTable.Prices`, so `EstimateCost()`/`ModelFamilyCost()` silently `continue`
  on that turn (`session/tokens/pricing.go:170-173`, `:212-215`), contributing literal `0.0` to
  cost — a data/config gap, not a linkage gap.

Both are labeled "unaccounted for" in the *user's* mental model (the screenshot shows both an
"ORPHANED: 24" tile and blank/zero cost rows), but they are orthogonal bugs requiring
independent fixes:
- Orphaned-session fix space: association/matching heuristics (out of scope per this project's
  Non-Goals — no scope creep into `Associator` logic implied by AC-1..AC-7).
- Unpriced-model fix space: `DefaultPricingTable()` coverage + visible-vs-silent unknown
  handling (the actual subject of this project).

**Scoping implication for the plan phase:** AC-1 through AC-7 only address the pricing gap.
Nothing in requirements.md asks to fix orphan association, and this research confirms that's
correct — they're separate bugs with separate code owners (`Associator` vs `PricingTable`).
If Tyler wants the orphan count addressed too, it should be a **separate backlog item**, not
folded into this one, since the fix technique (matching heuristics: UUID/path/time-window
tuning) is unrelated to pricing-table maintenance.

## 3. Edge cases the design must handle

1. **Multiple unpriced models in one breakdown.** `ModelFamilyCost()` returns a
   `map[string]float64` per family already (`pricing.go:203-237`) — the loop in
   `insights_service.go:192-216` builds one `ModelBreakdown` per family regardless of pricing
   status. The gap is that unpriced families **never enter `modelFamilyCosts`** at all (the
   `continue` on missing price happens before the map is populated) but families with actual
   token usage **do** enter `modelMap` via the turn-timeline loop (`:193-206`, which runs
   unconditionally, building `TotalInputTokens`/`TotalOutputTokens` regardless of pricing). So
   today an unpriced model already produces a `ModelBreakdown` row with real token counts and
   `EstimatedCostUsd == 0` — the row exists, it just looks identical to a genuinely free/no-cost
   row. AC-2's "no usage" vs "usage present but unpriced" distinction maps directly onto: does
   this family have `TotalInputTokens/TotalOutputTokens > 0` but no corresponding entry
   contributed to `modelFamilyCosts`? A boolean/flag per `ModelBreakdown` (e.g. `pricing_unavailable
   bool` on the proto) is the natural fix point, computed in the same loop, not a client-side
   heuristic.
2. **Model unpriced for only part of a date range** (pricing added mid-period, e.g. Sonnet 5
   pricing added 2026-07-27 but usage exists from 2026-07-01 onward). `DailyTokenBucket` per-day
   `CostByModel` map (`insights_service.go:179-182`) is keyed by day and built from
   `ModelFamilyCost()` per-session — once the table gains a `claude-sonnet-5` entry, **all**
   historical daily buckets will suddenly show nonzero cost for days that previously showed
   $0.00, because `EstimateCost`/`ModelFamilyCost` re-derive cost live from raw tokens on every
   request rather than persisting a cost-at-time-of-use. This is good (no backfill needed per
   Non-Goals) but means: there's no notion of "priced only after date X" — pricing table changes
   retroactively re-price the *entire* history the moment a new entry is added. Nothing to build
   here, just confirm the design doesn't need a temporal pricing-version mechanism (Non-Goals
   already excludes "historical cost re-computation/backfill", but note it's re-computed live by
   construction, not backfilled — worth stating explicitly in the plan so nobody builds a
   backfill job that isn't needed).
3. **Cache-token pricing tiers.** Already modeled — `ModelPricing` has 4 rates:
   `InputPricePerMTok`, `OutputPricePerMTok`, `CacheWritePerMTok` (`CacheCreation`),
   `CacheReadPerMTok`. All 4 are used together per family in both `EstimateCost` and
   `ModelFamilyCost`. Adding a new model family (Sonnet 5, etc.) just needs all 4 rates filled
   in — no structural change needed. Risk: if Anthropic pricing docs list only input/output (no
   distinct cache tiers) for some model, `CacheWritePerMTok`/`CacheReadPerMTok` would need `0.0`
   or a derived value; currently no code path treats a `0.0` rate as "missing" (a legitimately
   free cache tier and an unset field are indistinguishable) — worth flagging if any new model
   genuinely has zero cache-write cost, since AC-2's flag must key off of "missing from
   `pt.Prices` map" (the `ok` bool), never off any rate being `0.0`.
4. **`<synthetic>` model family — confirmed NOT expected to have real pricing, but currently
   silently swallowed exactly like a real unpriced model, which is a correctness gap.** Traced
   through `session/tokens/parser.go`: `processAssistantEntry` (`:118-184`) sets
   `turn.Model = msg.Model` verbatim from the JSONL `message.model` field with **no filtering**
   for the literal string `"<synthetic>"` (a value Claude Code's own transcript writer emits for
   internal/non-billed assistant turns, e.g. injected tool-result-only messages). If such a turn
   carries a non-nil `msg.Usage` block (`:146-156`), its input/output/cache tokens are added to
   `result.TotalInput`/`TotalOutput`/etc. and to `TurnTimeline` under family `"<synthetic>"`
   (since `NormalizeModelFamily("<synthetic>")` doesn't match any of the three regexes and
   returns it unchanged). Downstream this behaves identically to an unpriced real model: `pt.Prices["<synthetic>"]`
   misses, cost is silently `0.0`. **This means the planned AC-2/AC-3 "pricing unavailable"
   badge would incorrectly fire for `<synthetic>` turns**, which is misleading (there is no
   Anthropic price list entry for `<synthetic>` and there never will be — it's not a real model,
   it's an internal marker). The plan phase needs an explicit decision: either (a) filter
   `<synthetic>` (and any other non-billable sentinel model strings, if more exist) out of the
   turn/model aggregation entirely in `parser.go` before it ever reaches pricing, so it never
   contributes to `TurnTimeline`/`modelCounts`, or (b) special-case it in the pricing lookup as
   "known to be free" (add an explicit `"<synthetic>"` entry to `DefaultPricingTable()` with all
   rates `0.0`, distinct from "missing"). Option (a) is cleaner — it's a parser-level filtering
   concern, not a pricing concern — but needs verification of how much token volume `<synthetic>`
   turns actually carry in practice (recommend Agent checking real transcript samples in
   `~/.claude/projects/*/*.jsonl` for `"model":"<synthetic>"` lines with non-zero usage before
   deciding, since this file is off-repo and wasn't sampled in this research pass).

## 4. Unstated needs beyond the literal AC list

- **`PricingTable.IsStale()` already exists (`pricing.go:185-200`) but is completely dead code**
  — no caller anywhere in `server/` or `web-app/` (confirmed via repo-wide grep). It checks
  whether any entry's `EffectiveDate` is >30 days old. Tyler pays real API costs and the
  underlying bug here (Sonnet 5 shipped, table wasn't updated) is exactly the scenario
  `IsStale()` was built to detect — it's just never wired to anything. G4 ("prevent regression
  when new model families ship") should surface `IsStale()` in the UI (e.g. next to
  `PricingAsOf` which the backend already returns, `insights_service.go:273`, but the frontend
  doesn't appear to render `pricingAsOf` anywhere in the components read so far — worth
  double-checking in the plan phase) and/or wire it into `make lint`/a CI check per AC-4/AC-6's
  "guardrail" language, so a future new-model-family gap is caught before Tyler notices a blank
  number on the dashboard again.
- **A rough dollar-magnitude estimate for "how much am I NOT seeing," even without exact
  pricing.** The literal AC only asks for a "pricing unavailable" *indicator*, not a number. But
  Tyler's actual pain (per the item description: "we need to actually improve the insights page
  ... fix all the blank unaccounted for costs") is almost certainly "how much am I actually
  spending, roughly, right now" — a pure "unavailable" badge with no estimate leaves the
  `TOTAL COST` tile still silently under-reporting by the unpriced amount, which is the original
  complaint, just now correctly labeled instead of fixed. Worth proposing (not mandating) in the
  plan: a **secondary "estimated additional (~$X using nearest-known-family rate)" figure**
  computed by falling back to the most-recently-added family's rate (e.g. reuse
  `claude-sonnet-4`'s rates for an unpriced `claude-sonnet-5` as a placeholder) purely for the
  purpose of a *labeled estimate*, never silently substituted into the authoritative total. This
  is an enhancement beyond the literal AC-2/AC-3 wording, not a requirement — flag for Tyler's
  explicit decision at the plan/pre-mortem stage rather than building it speculatively, since
  Non-Goals excludes "live/dynamic pricing-API integration" and a rough-estimate feature edges
  close to that line without crossing it (it's still a static, locally-computed fallback).
- **Registry hygiene reminder (repo-specific, not app-specific):** per this repo's own
  `.claude/rules/feature-registry.md`, AC-6 isn't just "add a JSON file" — it also implies a new
  `// +feature:` or `// +api:` marker in the touched source and an e2e test per
  `docs/registry/README.md`'s conventions, then `make registry-generate` to confirm
  `coverage-gaps.json` doesn't grow. No existing per-feature file mentions pricing/cost lookup
  as its own feature (only `GetInsightsSummary.json`, `ListSessionTokens.json`,
  `WatchInsights.json`, and an unrelated `backlog/get-item-cost.json` exist under
  `docs/registry/features/`) — this is a genuine registry gap independent of the AC list, and
  the plan phase should decide whether pricing-table maintenance becomes its own
  `docs/registry/features/backend/pricing-lookup.json` entry (recommended, since it's a distinct
  maintenance surface from the RPC itself) or stays folded into `GetInsightsSummary`.

## Key files for the plan phase

- `session/tokens/pricing.go` — `DefaultPricingTable()`, `EstimateCost()`, `ModelFamilyCost()`, `IsStale()` (dead), `LoadPricingOverride()` (unwired)
- `session/tokens/parser.go` — `processAssistantEntry()` (`<synthetic>` model flows through unfiltered, `:118-184`)
- `session/tokens/association.go` — `Associator.Associate()` (orphan logic, confirmed unrelated to pricing)
- `server/services/insights_service.go` — `:97-227` (GetInsightsSummary aggregation, both orphan and pricing paths side by side), `:273` (`PricingAsOf` already returned to frontend)
- `server/dependencies.go:1070-1076` — where `DefaultPricingTable()` is wired, `LoadPricingOverride()` is not called anywhere
- `web-app/src/app/insights/insightsFormatters.ts` — `fmtCost()` (no unpriced-awareness), `fmtDate()` (existing "—" fallback precedent)
- `web-app/src/app/insights/ModelBreakdownChart.tsx` — `:52` (`|| "unknown"` fallback precedent), zero-cost bars render invisibly
- `web-app/src/app/insights/SessionsTable.tsx` + `SessionsTable.css.ts` — `orphanBadge`/`backlogBadge` pattern to mirror for an `unpricedBadge`
- `web-app/src/components/sessions/MemoryPressureCallout.tsx` — dismissible alert-callout pattern precedent
- `session/tokens/pricing_test.go:51-63` — `TestEstimateCost_WhenUnknownModel_ExpectFallbackToZero` (the test that will need to change/gain a sibling per AC-5)
- `docs/registry/features/backend/GetInsightsSummary.json`, `ListSessionTokens.json`, `WatchInsights.json`, `docs/registry/features/frontend/ui/insights-dashboard.json` — existing registry entries; no pricing-specific entry exists yet (AC-6 gap)
