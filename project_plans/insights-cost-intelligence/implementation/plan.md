# Implementation Plan: insights-cost-intelligence

**Feature**: Verdict-bearing waste findings, per-tool/activity cost, richer sort/search, a bookmarkable session drill-down route, and a fix for `WatchInsights`'s dead per-session live-update path, all layered onto the existing `/insights` dashboard.
**Date**: 2026-09-03
**Status**: Ready for implementation
**ADRs**: [ADR-001-per-tool-cost-attribution](../decisions/ADR-001-per-tool-cost-attribution.md), [ADR-002-findings-non-summable-dollar-impact](../decisions/ADR-002-findings-non-summable-dollar-impact.md), [ADR-003-sessions-table-sort-stays-client-side](../decisions/ADR-003-sessions-table-sort-stays-client-side.md)

---

## Step 0.5 — Creative pass (alternatives explored before committing)

Three high-level shapes for "where findings/cost live and how the frontend gets them":

1. **Everything rides `GetInsightsSummary`'s existing per-session loop** (chosen). *Strength*: findings/cost/activity are derived from data that loop already assembles once per request — no second scan of `TokenStore.GetAll()`, satisfying the NFR's "no full re-scan per request." *Weakness*: `GetInsightsSummaryResponse` keeps growing into a kitchen-sink message that pays the cost of computing findings/waste-score even for a page load that never opens the findings panel.
2. **A separate `GetWasteFindings`/`GetToolCostBreakdown` RPC, lazy-fetched by the frontend.** *Strength*: decouples the (currently cheap, but growing) findings computation from the hot summary path, and isolates its failure blast radius from the rest of the dashboard. *Weakness*: forces either a second full `s.store.GetAll()` scan or re-plumbing the same per-session intermediates (cost, cache-hit rate, tool usage) out of `GetInsightsSummary` into a second handler — duplicated work the existing loop already does, which `research/architecture.md` §1 already rejected for exactly this reason.
3. **Compute findings/cost entirely client-side** over data already fetched. *Strength*: zero backend/proto changes, fastest to ship. *Weakness*: `requirements.md`'s own Alternatives Considered section rejects this — it duplicates backend pricing/aggregation logic and risks drifting from the authoritative source.

**Chosen: Option 1**, per `research/architecture.md` §1 (cited throughout this plan) — this plan does not re-litigate that choice, only extends it consistently across all five workstreams. Options 2 and 3 are recorded in the Pattern Decisions table below as the rejected alternatives for the components they'd have affected.

A second, narrower creative pass for the route (workstream 4): (a) compose two already-existing RPCs behind a new Next.js dynamic route (chosen, per `research/architecture.md` §4 — zero new RPCs); (b) a single new consolidated `GetSessionDetail` RPC bundling both — rejected, adds proto/backend surface for no benefit over composing two calls the drawer already makes; (c) keep it a query-param-driven modal only, no path segment — rejected outright, since the requirement is explicitly a **path**-based, bookmarkable route (`requirements.md` Scope item 4), which a query param on the dashboard route does not satisfy.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `WasteFinding` | New proto message: one detected waste condition for one session — `finding_type`, `severity`, `dollar_impact_usd`, `session_id`, `conversation_id`, `message`. | `proto/session/v1/insights.proto`. Serializes `Finding`/`DollarImpact` (below) at the RPC boundary. |
| `Finding` | Go struct (`session/tokens/findings.go`) mirroring `WasteFinding` before proto marshaling: `{Type FindingType; Severity Severity; DollarImpact DollarImpact; SessionUUID string; Message string}`. | Detector functions return `[]Finding`. |
| `FindingType` | Proto `enum FindingType` (`proto/session/v1/insights.proto`): `FINDING_TYPE_UNSPECIFIED = 0` \| `FINDING_TYPE_CACHE_HIT_FLOOR_BREACH = 1` \| `FINDING_TYPE_SESSION_TOKEN_CEILING = 2` \| `FINDING_TYPE_MODEL_SWITCH_CACHE_BUST = 3` \| `FINDING_TYPE_OVERSIZED_START_CONTEXT = 4`. Go domain type (`session/tokens/finding_types.go`) is a type alias of the generated `sessionv1.FindingType`, per the precedent at `proto/session/v1/types.proto:423` ("Maps to session.Status enum in Go"). | Exactly 4 non-zero values shipped in v1 — see "Detector scope cut" note below Epic 1.1. |
| `Severity` | Proto `enum Severity`: `SEVERITY_UNSPECIFIED = 0` \| `SEVERITY_INFO = 1` \| `SEVERITY_WARN = 2` \| `SEVERITY_CRITICAL = 3`. Go domain type aliases the generated proto enum, same pattern as `FindingType`. | Maps onto the existing 4-tier `theme-contract.css.ts` palette (`success`/`warning`/`error`/`critical`) — findings never use `success`/`SEVERITY_INFO`. |
| `DollarImpact` | Go newtype `float64` wrapping one finding's modeled dollar estimate. **No exported `Sum`/`Add` helper anywhere in the codebase.** | See ADR-002. Go cannot forbid `+` on a numeric newtype — this is a review-convention guardrail, not a compiler guarantee, and the plan says so explicitly rather than overclaiming. |
| `WasteScore` | Go newtype `float64`, 0–100 clamped, one documented formula per session. `nil` means "not evaluated" (too few turns), distinct from a computed `0`. | See ADR-002; `optional double waste_score` on `SessionTokenSummary` so proto "unset" carries the same nil-vs-zero distinction. |
| `CacheROI` | Signed $ saved by cache reads vs. an all-fresh-input counterfactual (`cacheeconomics`'s "saving_vs_uncached" idea, ported as a formula per `research/build-vs-buy.md` §4). Negative = caching cost more than it saved. | `double cache_roi_usd` on `SessionTokenSummary`; `ok bool` return from `ComputeCacheROI` signals "undefined for this session" (unpriced model), not 0. |
| `ActivityType` | Proto `enum ActivityType`: `ACTIVITY_TYPE_UNSPECIFIED = 0` \| `ACTIVITY_TYPE_DEBUGGING = 1` \| `ACTIVITY_TYPE_REFACTORING = 2` \| `ACTIVITY_TYPE_FEATURE_DEV = 3` \| `ACTIVITY_TYPE_EXPLORATORY = 4` \| `ACTIVITY_TYPE_OTHER = 5`. Go domain type aliases the generated proto enum, same pattern as `FindingType`/`Severity`. | Computed by `ClassifyActivity`; see "Classification scope cut" note below Epic 1.2. |
| `ToolCostAttribution` (concept, not a type) | The tool-type-level session-sum method for attributing turn cost to tools — see ADR-001. Implemented as `AttributeToolCosts`. | Deliberately double-counts within a multi-tool turn, never across turns. |
| `CostMayDoubleCount` | `bool` field on `TopToolEntry` — true iff that tool ever co-occurred with another tool in the same turn anywhere in the session. | Drives the frontend's inline caveat marker; only rendered when true. |
| `CostUnpriced` | `bool` field on `TopToolEntry` — true iff none of that tool's turns had priced-model coverage. | Drives the frontend rendering `—` instead of `$0.00` for that tool's cost cell — same "abstain rather than guess" discipline as `ComputeCacheROI`'s `ok bool`. |
| `ParseResult` | Existing Go struct (`session/tokens/types.go`) — one JSONL file's aggregated token/tool/skill data. | Unchanged shape; findings/cost/activity are all pure functions *over* it, per `research/architecture.md` §1. |
| `TurnStats` | Existing per-assistant-turn struct — `Input`/`Output`/`CacheCreation`/`CacheRead`/`Model`/`ToolNames`. | The only granularity turn-level cost math can key off; no per-tool-call split exists in the source data. |
| `ToolTokenStats` | Existing per-tool-name aggregate (`ToolName`, `CallCount`, `MCPServer`). Gains a new `CostUsd float64` field this project. | `session/tokens/types.go`. |
| `PricingTable` | Existing per-model-family pricing lookup. Gains `EstimateTurnCost` (new, turn-granularity) and is the input to `AttributeToolCosts`/`ComputeCacheROI`/detector dollar-impact math. | `session/tokens/pricing.go`. |
| `EstimateTurnCost` | New method on `PricingTable`: USD cost for one turn's token counts under one model, mirroring `EstimateCost`'s arithmetic at turn granularity. | No existing per-turn cost function today — confirmed by reading `pricing.go` in full. |
| `AttributeToolCosts` | New function: `map[toolName]cost`, tool-type-level session sum (ADR-001). | `session/tokens/pricing.go`. |
| `ComputeCacheROI` | New function computing the signed cache-ROI counterfactual for one session. | `session/tokens/pricing.go`. |
| `ComputeFindings` | New function: runs all in-scope detectors over one `ParseResult`, isolates a panicking/erroring detector so it can't fail the whole session's findings (or the whole RPC). | `session/tokens/findings.go`. |
| `ComputeWasteScore` | New function: the single documented waste-score formula. Returns `nil` below the minimum-turn guard. | `session/tokens/findings.go`. |
| `ClassifyActivity` | New function: skill-name-substring signal, falling back to a tool-call-ratio heuristic. | `session/tokens/activity.go`. |
| `ComputeCacheHitRate` | Existing cache-hit-rate arithmetic (`cache_read / (input + cache_read)`), **exported** from `session/tokens` this project (moved out of `insights_service.go`'s private helper) so detectors can reuse it without duplicating the formula. | `session/tokens/pricing.go`. |
| `buildSessionSummary` | New consolidation function: the one place that builds a `SessionTokenSummary` proto message from a `ParseResult` — replaces the near-duplicate inline blocks in `GetInsightsSummary` and `ListSessionTokens`, and is reused by the `WatchInsights` fix (Epic 5). | `server/services/insights_service.go`; identified as a pre-existing duplication by `research/architecture.md` §5. |
| `TokenStore.Subscribe` | Existing method, changing return type from `<-chan struct{}` to `<-chan *tokens.ParseResult` this project — the changed file's result (or `nil` for "full walk complete, recompute everything"). | `session/tokens/store.go`. Root cause of the `WatchInsights` bug: the channel carries no payload today. |
| `InsightsEvent.Session` | Existing proto field, **never populated** by the server today (confirmed: zero `.Session =` assignments touching `InsightsEvent` in `insights_service.go`). This project populates it for real per-session live updates. | `proto/session/v1/insights.proto`. |
| `SessionDetailContent` | New React component: the pure content of a session's detail view (metadata, per-turn table, tools table, skill activations), with no dialog/route chrome — extracted from `SessionDetailDrawer.tsx` so the modal and the new route render identical markup from one source. | `web-app/src/app/insights/SessionDetailContent.tsx`. |
| `SessionDetailPageClient` | New client component: the `/insights/session/[sessionId]` route's data-loading wrapper around `SessionDetailContent`. | `web-app/src/app/insights/session/[sessionId]/SessionDetailPageClient.tsx`. |
| `useSessionDetail` | New hook: fetches one session's `SessionTokenSummary` via `GetInsightsSummary` with both `session_id_filter` and `conversation_id_filter` set (Epic 4's orphan-lookup fix), independent of the dashboard's global date-range filter. | `web-app/src/lib/hooks/useInsightsService.ts`. |
| `conversation_id_filter` | New optional proto field on `GetInsightsSummaryRequest`, OR'd with the existing `session_id_filter` — closes the gap where an orphan session (empty `session_id`) could never be selected by `session_id_filter` alone. | See Epic 4, Story 1.4.1. |
| `FindingsPanel` | New React component: the ranked, severity-badged list of findings, with four distinguishable states (loading / computed-empty / unpriced / error) — "unpriced" (couldn't be evaluated) is distinct from "computed-empty" (genuinely clean) per pre-mortem Failure #1. | `web-app/src/app/insights/FindingsPanel.tsx`. |
| `ActivityBreakdownTable` | New React component: dashboard-level cost-by-activity-type table, same pattern as `TopNTables.tsx`. | `web-app/src/app/insights/ActivityBreakdownTable.tsx`. |
| `EstimatedValue` | New shared component: the one visual treatment ("~" prefix + `aria-describedby` tooltip) for any modeled/heuristic number (per-tool cost, activity cost, cache ROI, waste score) as opposed to a directly-measured one. | `web-app/src/components/ui/EstimatedValue.tsx`. Reused, not forked, across all three new surfaces per `research/pitfalls.md` §2. |
| `SortColumn` | Existing frontend union type (`"input" \| "output" \| "cache" \| "cost"`) in `SessionsTable.tsx`, extended this project with `"duration" \| "costPerMessage" \| "cacheRoi" \| "wasteScore"`. | Stays 100% client-side per ADR-003. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Findings/cost/activity exposure | Extend existing `GetInsightsSummary` response (Transaction-Script-style aggregation loop, PoEAA) | Fowler / `research/architecture.md` §1 | A dedicated `GetWasteFindings` RPC (Service Layer split) | Forces a second full scan or re-plumbed intermediates — duplicates work the existing loop already does; violates the NFR's "no full re-scan per request" |
| 6 candidate waste detectors | Pipeline of independent `func(*ParseResult, *PricingTable) *Finding` strategies (Strategy, GoF), run by `ComputeFindings` | GoF | A rules-engine library (Grule/RuleGo/ZEN) | `research/build-vs-buy.md` §1: buys externalized/user-authored rule capability this project explicitly excludes (no rules-config UI); adds a DSL, hot-reload machinery, and (for ZEN) a Rust FFI dependency for zero benefit here |
| Detector scope for v1 | Ship only the 4 of 6 heuristics computable from existing `ParseResult` fields (cache-hit floor, session-token ceiling, model-switch cache-bust, oversized-start-context); defer redundant-file-reads and tool-failure-rate | `research/pitfalls.md` §1 ("fewer, higher-precision detectors") + direct grep of `parser.go` | Shipping all 6 as originally listed in `requirements.md` | Verified by grep: `parser.go` never parses `tool_result`/`is_error` blocks and never captures per-call file-path arguments — both deferred detectors need new parser capture surface, which is new scope beyond "pure functions over already-parsed `ParseResult`" (`research/architecture.md` §1's stated design). Flagged as explicit follow-up, not silently dropped — see Unresolved Questions. |
| `WasteFinding.DollarImpact` / `WasteScore` | Named non-summable newtype (`type-driven-design`) | type-driven-design skill / ADR-002 | Plain `float64` + a UI disclaimer only | A disclaimer doesn't stop a future `Σ finding.DollarImpact` one-liner; a named type at least makes the moment visible in a diff, even though Go can't compiler-enforce non-summability |
| Per-tool cost attribution | Tool-type-level session sum (ADR-001) | `research/architecture.md` §2 / ADR-001 | (a) full-turn attribution to every tool, (b) even split per turn | (a) provably inflates the summed tool-cost total past the session's real cost on multi-tool turns; (b) produces a precise-looking number with no operational meaning, at the same implementation cost as (c) |
| Activity classification | Two-tier rule: skill-name-substring match, then tool-call-ratio fallback (Strategy-like rule list) | GoF / `research/features.md` §"unstated needs" | LLM-at-runtime classification | `requirements.md`'s explicit non-goal (cost/latency/non-determinism on a cost dashboard) |
| Activity taxonomy granularity | 5-value `ActivityType` (debugging/refactoring/feature_dev/exploratory/other), skill-name signal takes priority over tool-ratio | Direct grep of `parser.go`/`types.go` | The reference tools' finer taxonomy driven by tool-call args (e.g. distinguishing test-run `Bash` calls) | Tool-call arguments aren't captured by the parser (`ToolNames []string` has no args) — a finer taxonomy would need new parsing surface, same reasoning as the detector scope cut above |
| Sessions-table sort/search | Extend the existing client-side `useMemo` comparator (ADR-003) | `research/pitfalls.md` §3 / ADR-003 | Wire `ListSessionTokens`'s server-side `sort_by`/pagination | Breaks Fuse.js's full-set search unless search also moves server-side (new scope); offset pagination + a live-mutating sort key is the documented "page drift" bug class |
| Route ↔ modal content sharing | Extract Shared Component (`SessionDetailContent`) — a Fowler-style "Extract Function/Component" refactor | Fowler (Refactoring) | Duplicate JSX in both the drawer and the route | Two hand-maintained copies of the same rendering drift out of sync (`research/pitfalls.md` §4) |
| Route data loading | Compose two existing RPCs (`GetInsightsSummary` + `GetSessionTurnTimeline`) from a thin Next.js page (Service Layer composition) | `research/architecture.md` §4 | A new consolidated `GetSessionDetail` RPC | Zero new backend/proto surface beats a new RPC that bundles what two existing calls already do |
| `WatchInsights` payload | Observer pattern (GoF) — unchanged shape, richer payload (`*ParseResult` instead of `struct{}`) | GoF / `research/architecture.md` §5 | A broader rewrite of daily/model aggregate recompute-on-update | Explicit guardrail in `requirements.md`'s Rabbit Holes — keep the fix narrowly scoped to the per-session payload |
| Per-session summary construction | Extract Function (`buildSessionSummary`) reused by 3 call sites | Fowler (Refactoring) | Leave the 2 (soon 3) near-duplicate inline blocks as-is | `research/architecture.md` §5 flags this as a pre-existing duplication that the `WatchInsights` fix would otherwise triple, not just double |
| Estimated/modeled value display | One shared `EstimatedValue` component/style token reused across 3 new surfaces | `research/pitfalls.md` §2 / `research/ux.md` §4 | Per-component ad hoc "~" styling | Prevents visual drift between per-tool cost, activity cost, and waste score/cache ROI treatments |

---

## Migration Plan

Omit — no database schema or data migration is involved. All changes are additive `proto/session/v1/insights.proto` fields/messages (regenerated via `make proto-gen`, generated Go/TS never committed per this repo's `.gitignore` policy) plus new pure Go functions and new/extended React components. Each task below that changes the proto explicitly calls out the `make proto-gen` step in place of a migration file.

## Observability Plan

- **Logs**: `ComputeFindings`'s per-detector `recover()` (Story 1.1.3c) logs a `slog.Warn` with the session's conversation ID and the recovered panic value when a detector fails, so a computation error is visible in `logs/staplersquad.log` without paging anyone — matches `requirements.md`'s Observability Requirements verbatim. No new metric/alert is added anywhere in this plan (none requested, none warranted for a single-operator local tool).
- **Metrics**: none new. Per `research/pitfalls.md` §5, findings/cost computation over ~600 sessions is microseconds of CPU — not worth instrumenting a latency histogram for.
- **Alerts**: no new alerts required (per `requirements.md`'s Observability Requirements section — this is a dashboard a human reads, not a paging system).

## Risk Control

- **Feature flag**: not gated — additive UI/RPC-field changes behind the existing `/insights` route, no destructive change to existing behavior (per `requirements.md`'s Risk Control section).
- **Rollback procedure**: standard `git revert` of the offending PR; no data migration to unwind.
- **Staged rollout**: full rollout on merge — single-operator tool, no staged-rollout mechanism exists or is warranted.

## Unresolved Questions

None blocking — every open question `requirements.md` deferred to Phase 2/3 was resolved during this research/planning pass. Recorded here for traceability rather than left implicit:

- [x] Exact detector set for v1 → **resolved**: 4 of the 6 candidate heuristics (cache-hit floor, session-token ceiling, model-switch cache-bust, oversized-start-context). Redundant-file-reads and tool-failure-rate are **deferred**, not silently dropped — both need new parser capture surface (`tool_result`/`is_error` blocks; per-call file-path arguments) that today's `parser.go` doesn't record. **Follow-up owner**: whoever picks up a future "extend the JSONL parser to capture tool-call arguments/results" project — file as a backlog item at Epic 1.1's completion, not blocking this pass.
- [x] Per-tool cost attribution formula → **resolved**: tool-type-level session sum (ADR-001).
- [x] `ListSessionTokens` vs. client-side sort → **resolved**: stays client-side (ADR-003); `ListSessionTokens` remains implemented/tested but unused by the table.
- [x] Route vs. `WatchInsights` fix coupling → **resolved**: fully decoupled, separate epics (1.4 and 1.5), per `requirements.md`'s explicit instruction.
- [x] Orphan-session deep-linking gap (found during this planning pass, not in prior research: `session_id_filter` alone can never select an orphan since its `sessionID` is always `""` and the filter check itself is gated by `!= ""`) → **resolved**: new `conversation_id_filter`, OR'd with `session_id_filter` (Story 1.4.1).

## Dependency Visualization

```
Epic 1.5 (WatchInsights fix)
  Story 1.5.1 (channel payload) ───────────────────────────────────► parallel-safe,
                                                                       ship anytime
    └─► Story 1.5.2 (buildSessionSummary) ◄── GATED on Epic 1.1 AND Epic 1.2
          │                                    merging first (see note below) —
          │                                    NOT parallel-safe like 1.5.1
          └─► Story 1.5.3 (populate evt.Session)
                └─► Story 1.5.4 (frontend verification, no code change)

Epic 1.1 (Findings)                     Epic 1.2 (Per-tool/activity cost)
  1.1.1 proto + domain types               1.2.1 EstimateTurnCost + AttributeToolCosts
    └─► 1.1.2 cache-floor + ceiling           └─► 1.2.2 wire TopToolEntry.cost_usd
          └─► 1.1.3 model-switch + ctx              1.2.3 ClassifyActivity (independent)
                └─► 1.1.4 WasteScore                  └─► 1.2.4 activity_breakdown aggregate
                      └─► 1.1.5 wire into GIS                └─► 1.2.5 frontend display
                            └─► 1.1.6 FindingsPanel UI              └─► 1.2.6 registry
                                  └─► 1.1.7 registry
                                        │
                                        ▼
Epic 1.3 (Sort/search) ── needs SessionTokenSummary.cache_roi_usd (1.3.1, independent
  1.3.1 CacheROI + proto field              of 1.1/1.2 but shares the proto file — batch
    └─► 1.3.2 duration/cost-per-msg sort     proto-gen runs where convenient) and
          └─► 1.3.3 cacheRoi/wasteScore sort  .waste_score (from 1.1.4/1.1.5)
                └─► 1.3.4 search+sort coexistence check

Epic 1.4 (Drill-down route) ── mostly independent; FindingsPanel's single-session
  1.4.1 conversation_id_filter fix           click-through (1.1.6b) links here once
    └─► 1.4.2 extract SessionDetailContent   both epics have shipped — sequence 1.1
          └─► 1.4.3 the route itself         before 1.4, or ship 1.1.6b pointing at the
                └─► 1.4.4 wire navigation    modal first and swap the link target after
                      └─► 1.4.5 registry     1.4 lands (noted inline in Story 1.1.6b).

Suggested implementation order: Story 1.5.1 is parallel-safe and can ship anytime,
alongside any other epic — it only changes the channel's payload type. The rest of
Epic 1.5 is NOT parallel with 1.1/1.2: Story 1.5.2 onward (buildSessionSummary
extraction, 1.5.3, 1.5.4) is gated on Epic 1.1 AND Epic 1.2 merging first, not run
concurrently with them, because 1.5.2 extracts the per-session build logic
verbatim — landing it before 1.1/1.2's fields (findings, waste score, tool cost,
activity type) exist means redoing the extraction once they land, which is the
exact duplication/drift the story exists to prevent. For everything else: 1.1 → 1.2
→ 1.3 → 1.4, in that order. 1.1 and 1.2 both touch insights.proto and
insights_service.go's per-session loop — sequencing 1.1 fully before starting 1.2
avoids a merge-conflict-prone proto/service file being edited by two stories at
once.
```

---

## Phase 1: insights-cost-intelligence v1

### Epic 1.1: Waste-pattern findings panel

**Goal**: Surface severity- and dollar-impact-tagged verdicts (not raw metrics) for specific waste conditions, ranked, on the dashboard — the core "find the driver fast" success metric from `requirements.md`.

**Detector scope note**: v1 ships 4 of the 6 heuristics named in `requirements.md` Scope item 1. Redundant/large-file-read and tool-failure-rate detectors are deferred — see Pattern Decisions and Unresolved Questions above for why and how that's tracked, not silently dropped.

#### Story 1.1.1: Domain types & proto surface
**As a** dashboard operator, **I want** a `WasteFinding` message and matching Go domain types, **so that** later stories have a stable shape to compute into and marshal.
**Acceptance Criteria**:
- `WasteFinding` proto message and a `findings` field on `GetInsightsSummaryResponse` compile and round-trip correctly.
  - *Given* a `GetInsightsSummaryResponse` built in a Go test with one `WasteFinding{FindingType: sessionv1.FindingType_FINDING_TYPE_CACHE_HIT_FLOOR_BREACH, Severity: sessionv1.Severity_SEVERITY_CRITICAL, DollarImpactUsd:4.20}` appended to `Findings`, *When* it is marshaled to bytes and unmarshaled back, *Then* `resp.Findings[0].DollarImpactUsd == 4.20` and `resp.Findings[0].FindingType == sessionv1.FindingType_FINDING_TYPE_CACHE_HIT_FLOOR_BREACH`.
**Files**: `proto/session/v1/insights.proto`, `session/tokens/finding_types.go`

##### Task 1.1.1a: Add `Severity`/`FindingType`/`ActivityType` enums + `WasteFinding` message + `findings` field (~6 min)
- Add `enum Severity { SEVERITY_UNSPECIFIED = 0; SEVERITY_INFO = 1; SEVERITY_WARN = 2; SEVERITY_CRITICAL = 3; }`, `enum FindingType { FINDING_TYPE_UNSPECIFIED = 0; FINDING_TYPE_CACHE_HIT_FLOOR_BREACH = 1; FINDING_TYPE_SESSION_TOKEN_CEILING = 2; FINDING_TYPE_MODEL_SWITCH_CACHE_BUST = 3; FINDING_TYPE_OVERSIZED_START_CONTEXT = 4; }`, and `enum ActivityType { ACTIVITY_TYPE_UNSPECIFIED = 0; ACTIVITY_TYPE_DEBUGGING = 1; ACTIVITY_TYPE_REFACTORING = 2; ACTIVITY_TYPE_FEATURE_DEV = 3; ACTIVITY_TYPE_EXPLORATORY = 4; ACTIVITY_TYPE_OTHER = 5; }` to `proto/session/v1/insights.proto` (all three defined together now — per architecture-review.md's Blocker remediation — even though `ActivityType` isn't consumed until Epic 1.2, matching this repo's 20+ existing `enum`-for-closed-value-set convention in `proto/session/v1/types.proto` rather than a plain `string`). `ActivityType` is used later by Task 1.1.5a's `SessionTokenSummary.activity_type` field reservation and Epic 1.2's `ActivityCostBreakdown`.
- Add `message WasteFinding { FindingType finding_type = 1; Severity severity = 2; double dollar_impact_usd = 3; string session_id = 4; string conversation_id = 5; string message = 6; }` to `proto/session/v1/insights.proto`.
- Add `repeated WasteFinding findings = 14;` to `GetInsightsSummaryResponse` (next free field number after `unpriced_models = 13`).
- Run `make proto-gen`, then `go build ./...` to confirm generated code compiles (do not commit generated output, per this repo's `.gitignore`).
- Files: `proto/session/v1/insights.proto`

##### Task 1.1.1b: Domain types for findings (~4 min)
- Create `session/tokens/finding_types.go` with `FindingType` and `Severity` as Go type aliases of the generated proto enums (`type FindingType = sessionv1.FindingType`, `type Severity = sessionv1.Severity`), plus package-level constant aliases for each non-zero value (e.g. `FindingCacheHitFloorBreach = sessionv1.FindingType_FINDING_TYPE_CACHE_HIT_FLOOR_BREACH`, `SeverityCritical = sessionv1.Severity_SEVERITY_CRITICAL` — 4 `FindingType` values now, 2 more reserved as comments for the deferred detectors) so detector code in `findings.go` never has to spell out the `sessionv1.` prefix, mirroring the precedent at `proto/session/v1/types.proto:423` ("Maps to session.Status enum in Go"). `DollarImpact float64` / `WasteScore float64` remain plain newtypes (unaffected by this — they're not closed value sets), each with a doc comment stating the ADR-002 non-summability convention.
- Files: `session/tokens/finding_types.go`

#### Story 1.1.2: Cache-hit-floor-breach & session-token-ceiling detectors
**As a** dashboard operator, **I want** the two simplest, highest-precision waste detectors implemented first, **so that** the panel has real content before the more complex detectors land.
**Acceptance Criteria**:
- The cache-hit-floor detector is guarded against cold-start noise on short sessions.
  - *Given* a synthetic `ParseResult` with 6 turns, `TotalInput=100_000`, `CacheRead=10_000` (a 9% hit rate) on a priced model family, *When* `tokens.ComputeFindings(r, pt)` runs, *Then* it returns a `Finding{Type: FindingCacheHitFloorBreach, Severity: SeverityCritical}` with `DollarImpact > 0`; *Given* the same session truncated to its first 3 turns, *When* re-run, *Then* no cache-hit-floor finding is returned.
- Both detectors abstain rather than fire a misleading `$0.00` finding when pricing is unavailable.
  - *Given* the same 6-turn, 9%-hit-rate `ParseResult` from above but on a model with no entry in `PricingTable` (unpriced), *When* `tokens.ComputeFindings(r, pt)` runs, *Then* no `FindingCacheHitFloorBreach` is returned at all (not one with `DollarImpact == 0`); *Given* a `ParseResult` whose total tokens exceed `sessionTokenCeiling` on an unpriced model, *When* re-run, *Then* no `FindingSessionTokenCeiling` is returned either — matching the "abstain rather than guess" discipline `ComputeCacheROI` already uses (`requirements.md`'s Rabbit Holes).
- Each finding's message names a concrete, session-specific detail, not a category label.
  - *Given* the 6-turn, 9%-hit-rate, priced-model `ParseResult` above, *When* the cache-hit-floor detector fires, *Then* `finding.Message` reads like `"Cache hit rate 9% is below the 40% floor over 6 turns — an estimated $4.20 in avoidable input-token cost."` (states the actual rate, the floor, the turn count, and the dollar estimate — not just "cache hit rate too low"); *Given* a session with 3,000,000 total tokens on a priced model, *When* the session-token-ceiling detector fires, *Then* `finding.Message` reads like `"Session used 3,000,000 tokens, over the 2,000,000 ceiling — estimated cost $24.00."`.
**Files**: `session/tokens/pricing.go`, `session/tokens/findings.go`, `session/tokens/findings_test.go`, `server/services/insights_service.go`

##### Task 1.1.2a: Export `ComputeCacheHitRate` (~3 min)
- Move the cache-hit-rate formula out of `insights_service.go`'s private `computeCacheHitRate` into an exported `tokens.ComputeCacheHitRate(input, cacheRead int64) float64` in `session/tokens/pricing.go`; update `insights_service.go`'s 2 call sites (`GetInsightsSummary`, `ListSessionTokens`) to call the exported version, and delete the now-unused private copy.
- Files: `session/tokens/pricing.go`, `server/services/insights_service.go`

##### Task 1.1.2b: `detectCacheHitFloorBreach` (~6 min)
- Add to `session/tokens/findings.go`: consts `minTurnsForCacheFloor = 5` and `cacheHitFloor = 0.40`, each with a doc comment stating the provenance/pricing-snapshot-dependency per `research/pitfalls.md` §1. Implement `detectCacheHitFloorBreach(r *ParseResult, pt *PricingTable) *Finding`: skip (return `nil`) if `len(r.TurnTimeline) < minTurnsForCacheFloor`; look up `pricing, priced := pt.LookupByModel(r.PrimaryModel)` — skip (return `nil`, not a zero-impact finding) if `!priced`, per the "abstain rather than guess" rule (same discipline as `ComputeCacheROI`'s `ok bool` return); else compute `hitRate := ComputeCacheHitRate(r.TotalInput, r.CacheRead)`; if `hitRate >= cacheHitFloor`, return `nil`; else return a `Finding` with `DollarImpact = (cacheHitFloor-hitRate) * float64(r.TotalInput+r.CacheRead) * (pricing.InputPricePerMTok-pricing.CacheReadPerMTok) / 1e6` (clamped ≥ 0), `Severity = SeverityCritical if hitRate < 0.15 else SeverityWarn`, and `Message` stating the actual hit rate (as a whole-number percent), the 40% floor, the turn count, and the dollar estimate.
- Files: `session/tokens/findings.go`

##### Task 1.1.2c: `detectSessionTokenCeiling` (~4 min)
- Add const `sessionTokenCeiling = 2_000_000` (documented as "an unusually large single session at this project's current ~600-session scale"). Implement `detectSessionTokenCeiling(r *ParseResult, pt *PricingTable) *Finding`: skip (return `nil`) if `r.TotalInput+r.TotalOutput+r.CacheCreation+r.CacheRead <= sessionTokenCeiling`; else `costUSD, unpricedModels := pt.EstimateCost(r)`; skip (return `nil`, not a zero-impact finding) if `len(unpricedModels) > 0`; else fire (`Severity = SeverityWarn`, or `SeverityCritical` if `costUSD > 20`) with `DollarImpact = DollarImpact(costUSD)` and `Message` stating the actual total token count, the ceiling, and the cost. No turn-count guard needed (can't fire on a short session by construction). (This also makes the detector's signature `func(*ParseResult, *PricingTable) *Finding`, uniform with the other 3 detectors and the Pattern Decisions table's stated Strategy shape — it previously took a pre-computed `costUSD float64` instead of `pt`.)
- Files: `session/tokens/findings.go`

##### Task 1.1.2d: Fixture tests (~7 min)
- `session/tokens/findings_test.go`, table-driven, `Test<Type>_When<Condition>_Expect<Outcome>` naming: just-under/at/over threshold for both detectors, the <5-turn cache-floor guard case, an unpriced-model case for each detector (model absent from `PricingTable` — asserts `nil` returned, not a `Finding` with `DollarImpact == 0`), and a message-content assertion for each detector's firing case (`strings.Contains` on the rate/count/threshold/dollar figures named in the Story's AC).
- Files: `session/tokens/findings_test.go`

#### Story 1.1.3: Model-switch-cache-bust & oversized-start-context detectors, plus the isolating aggregator
**As a** dashboard operator, **I want** the remaining two v1 detectors plus a `ComputeFindings` entry point that can't let one bad session break the whole request, **so that** the findings computation matches the Observability Requirement ("a computation error should show up as an empty/error state, not page anyone").
**Acceptance Criteria**:
- A session with only 1 distinct model is skipped by the model-switch detector, not scored as "no waste."
  - *Given* a synthetic `ParseResult` with `Models=["claude-sonnet-4"]` (1 distinct model), *When* `tokens.ComputeFindings(r, pt)` runs, *Then* the returned slice contains no `FindingModelSwitchCacheBust` entry at all (not a zero-impact one).
- A panicking detector doesn't take down the batch. (None of the 4 shipped detectors panics on any constructible `ParseResult` — each guards its indexing/division with an early `nil` return, per Tasks 1.1.2b/1.1.2c/1.1.3a/1.1.3b — so this property is proved against the recover-wrapping mechanism itself, not a real detector.)
  - *Given* the same `defer func() { recover() ... }()` closure Task 1.1.3c wraps around every detector call, wrapped instead around a test-local detector double that deliberately panics, *When* the wrapped call executes, *Then* the panic does not propagate past the closure and a `slog.Warn` naming the detector is logged — demonstrating that if a real detector ever panicked on a malformed session, `ComputeFindings`'s other detector calls (and the caller's other sessions) would be unaffected.
- The model-switch and oversized-context detectors abstain rather than fire on an unpriced model.
  - *Given* a `ParseResult` with a detected model switch where the post-switch model has no `PricingTable` entry, *When* `tokens.ComputeFindings(r, pt)` runs, *Then* no `FindingModelSwitchCacheBust` is returned for that switch (not one with `DollarImpact == 0`); *Given* a `ParseResult` whose first turn's model is unpriced and whose first-turn context exceeds `oversizedContextFloor`, *When* re-run, *Then* no `FindingOversizedStartContext` is returned either.
- Each finding's message names the specific turn/model/token detail that triggered it.
  - *Given* a model switch detected at turn 7 (`claude-sonnet-4` → `claude-opus-5`) that busts the cache, *When* the model-switch detector fires, *Then* `finding.Message` reads like `"Turn 7 switched model from claude-sonnet-4 to claude-opus-5, busting the cache — estimated $1.35 in avoidable cache-write cost."`; *Given* a first-turn context of 45,000 tokens (threshold 30,000), *When* the oversized-start-context detector fires, *Then* `finding.Message` reads like `"Session started with 45,000 tokens of context (threshold: 30,000) — consider trimming CLAUDE.md or start-of-session file reads."`.
**Files**: `session/tokens/findings.go`, `session/tokens/findings_test.go`

##### Task 1.1.3a: `detectModelSwitchCacheBust` (~6 min)
- Precondition: skip (return `nil`, not a zero-impact finding) unless `len(r.Models) >= 2 && len(r.TurnTimeline) >= 2`. Scan adjacent turns in `r.TurnTimeline` for a model change immediately followed by a turn with `CacheRead == 0 && CacheCreation > 0` (the cache-bust signature); for each such switch, look up `pricing, priced := pt.LookupByModel(<post-switch turn's Model>)` and skip that switch (contribute nothing) if `!priced`. If no priced switch is found among however many were detected, return `nil` (not a zero-impact finding). Otherwise sum `CacheCreation * pricing.CacheWritePerMTok/1e6` across all detected, priced busts for `DollarImpact`; `Severity = SeverityWarn`; `Message` names the triggering turn index and both model names (pre-switch → post-switch) plus the dollar estimate.
- Files: `session/tokens/findings.go`

##### Task 1.1.3b: `detectOversizedStartContext` (~5 min)
- Const `oversizedContextFloor = 30_000` (documented: "a typical Claude Code system prompt + modest `CLAUDE.md` is a few thousand tokens"). Precondition: skip if `len(r.TurnTimeline) == 0`. Compute `contextTokens := r.TurnTimeline[0].Input + r.TurnTimeline[0].CacheRead`; skip (return `nil`) if `contextTokens <= oversizedContextFloor`; look up `pricing, priced := pt.LookupByModel(r.TurnTimeline[0].Model)` — skip (return `nil`, not a zero-impact finding) if `!priced`; else fire (`Severity = SeverityWarn`, or `SeverityCritical` if `> 80_000`) with `DollarImpact = float64(contextTokens)/1e6*pricing.InputPricePerMTok` (the first turn structurally has no cache read yet, so this is exactly that turn's input cost) and `Message` stating the actual token count and the threshold.
- Files: `session/tokens/findings.go`

##### Task 1.1.3c: `ComputeFindings` aggregator with panic isolation (~5 min)
- `func ComputeFindings(r *ParseResult, pt *PricingTable) []Finding`: pre-size `make([]Finding, 0, 4)`; call each of the 4 detectors (now all sharing the uniform `func(*ParseResult, *PricingTable) *Finding` signature, per Task 1.1.2c's fix); wrap each call in a `func() { defer func() { if rec := recover(); rec != nil { log.Warn("finding detector panicked", "detector", name, "session", r.SessionUUID, "recover", rec) } }(); ... }()` closure so one detector's panic doesn't propagate to the caller or to sibling detectors.
- Files: `session/tokens/findings.go`

##### Task 1.1.3d: Fixture tests (~7 min)
- Cover both new detectors' boundary conditions, an unpriced-model case for each (skip, not zero-impact), and a message-content assertion for each detector's firing case.
- Panic-isolation test: `detectOversizedStartContext`'s `len(r.TurnTimeline) == 0` guard (Task 1.1.3b) means no constructible `ParseResult` reaches a panicking index/divide in any of the 4 shipped detectors (verified against 1.1.2b/1.1.2c/1.1.3a/1.1.3b's guards too) — a 0-turn fixture fed through `ComputeFindings` would pass without ever exercising `recover()`. Instead, in `findings_test.go` declare an unexported test-local function variable matching the detector signature (`func(*ParseResult, *PricingTable) *Finding`) whose body does `panic("...")`, wrap a call to it in the identical recover-wrapping closure Task 1.1.3c specifies (copied inline in the test, not imported from `findings.go`), and assert (a) the panic does not escape the wrapping call and (b) `slog.Warn` is emitted naming the detector. This test double lives only in `findings_test.go` — no mutable package-level seam is added to `findings.go` — consistent with `architecture-review.md`'s rejection of an exported/mutable package-level test hook.
- Files: `session/tokens/findings_test.go`

#### Story 1.1.4: WasteScore formula
**As a** dashboard operator, **I want** one documented waste-score formula, **so that** the sessions table (Epic 1.3) has a single sortable badness number that is never confused with a dollar sum.
**Acceptance Criteria**:
- A session too sparse to evaluate returns `nil`, not `0`.
  - *Given* a synthetic `ParseResult` with 2 turns, *When* `tokens.ComputeWasteScore(r, pt)` runs, *Then* it returns `nil`; *Given* a 10-turn session with a 10% cache-hit rate and a 3,000,000-token ceiling breach, *When* re-run, *Then* it returns a non-nil `*WasteScore` between 70 and 100.
**Files**: `session/tokens/findings.go`, `session/tokens/findings_test.go`

##### Task 1.1.4a: `ComputeWasteScore` (~5 min)
- `func ComputeWasteScore(r *ParseResult, pt *PricingTable) *WasteScore`: return `nil` if `len(r.TurnTimeline) < minTurnsForCacheFloor`. Else compute a documented weighted blend — e.g. `score := clamp(0, 100, (1-hitRate)*40 + ceilingPenalty*30 + contextPenalty*30)` where `ceilingPenalty`/`contextPenalty` are 0–1 ratios of the session's totals against the ceiling/floor consts from Story 1.1.2/1.1.3. Comment explicitly: "not a sum of finding dollar impacts — see ADR-002."
- Files: `session/tokens/findings.go`

##### Task 1.1.4b: Fixture tests (~4 min)
- Cases: nil-for-sparse, low score (clean session), high score (multiple breaches). Assert the returned pointer's dereferenced value, and assert `nil` is `nil` (not a pointer to `0`).
- Files: `session/tokens/findings_test.go`

#### Story 1.1.5: Wire findings + waste score into `GetInsightsSummary`
**As a** dashboard operator, **I want** the response-level findings list sorted, capped, and populated from the existing per-session loop, **so that** the frontend has something to render without a second backend pass.
**Acceptance Criteria**:
- Findings are capped at 20 and sorted by dollar impact descending.
  - *Given* 25 sessions each producing exactly one finding with a strictly increasing `DollarImpactUsd` (1.0, 2.0, ..., 25.0), *When* `GetInsightsSummary` is called, *Then* `resp.Findings` has exactly 20 entries and `resp.Findings[0].DollarImpactUsd == 25.0` (the 20 largest, in descending order).
**Files**: `proto/session/v1/insights.proto`, `server/services/insights_service.go`, `server/services/insights_service_test.go`

##### Task 1.1.5a: Reserve `SessionTokenSummary` fields for this epic and Epics 1.2–1.3 (~3 min)
- Add to `SessionTokenSummary` in `insights.proto`: `double cache_roi_usd = 18;`, `optional double waste_score = 19;`, `ActivityType activity_type = 20;` (the `ActivityType` enum from Task 1.1.1a, not a plain `string`) — declared together now (fields 18–20 all free after `unpriced_models=17`) to avoid a second `make proto-gen` pass across epics; `cache_roi_usd`/`activity_type` are populated by Epics 1.2/1.3, `waste_score` by this story. Run `make proto-gen`, `go build ./...`.
- Files: `proto/session/v1/insights.proto`

##### Task 1.1.5b: Populate findings + waste score in the loop (~5 min)
- In `GetInsightsSummary`'s per-session loop (`insights_service.go`, after `costUSD, unpriced := s.pricing.EstimateCost(r)`), call `tokens.ComputeFindings(r, s.pricing)`, append each to a session-scoped `[]*sessionv1.WasteFinding` built by translating `tokens.Finding` → `sessionv1.WasteFinding` (setting `SessionId`/`ConversationId` from the loop's already-computed `sessionID`/`r.SessionUUID`; `FindingType`/`Severity` copy straight across with no string conversion, since `tokens.FindingType`/`tokens.Severity` are Go aliases of the same generated `sessionv1` enum types); accumulate into a request-scoped `allFindings []*sessionv1.WasteFinding`. Call `tokens.ComputeWasteScore(r, s.pricing)` and set `summary.WasteScore` via `proto.Float64(*score)` when non-nil (leave unset otherwise). After the loop, `sort.Slice(allFindings, ...)` by `DollarImpactUsd` desc, cap at 20, set `resp.Findings`.
- Files: `server/services/insights_service.go`

##### Task 1.1.5c: Tests (~5 min)
- Assert the cap-at-20 + descending-sort-order behavior; assert a fake `ParseResult` engineered to panic one detector doesn't fail the whole `GetInsightsSummary` call (leans on Story 1.1.3c's isolation).
- Files: `server/services/insights_service_test.go`

#### Story 1.1.6: Findings panel UI
**As a** dashboard operator, **I want** a ranked findings panel with four distinguishable states, **so that** an empty panel never gets misread as "nothing wrong" when it might mean "computation failed" or "couldn't be evaluated" (pre-mortem Failure #1: an all-unpriced-model dashboard must not render the same "clean" text as a genuinely clean one).
**Acceptance Criteria**:
- Computed-empty and error states are visually and textually distinct.
  - *Given* `summary.findings = []` and `summary.sessions.length > 0` (genuinely clean), *When* `FindingsPanel` renders, *Then* it shows "No waste patterns detected" text (not a bare empty `<div>`); *Given* the parent's `error` state is set instead, *When* `FindingsPanel` renders, *Then* it shows an `errorBox`-styled message with different text, in the same panel location.
- The "unpriced" state and the "computed-empty" (genuinely clean) state are visually and textually distinct — an all-unpriced-model dashboard must never render the "No waste patterns detected" message.
  - *Given* `summary.findings = []` and every session in `summary.sessions` has `unpricedModels.length > 0` (no session was evaluable), *When* `FindingsPanel` renders, *Then* it shows "N sessions could not be evaluated (unpriced model)" text naming the actual count N (`summary.sessions.length`), not "No waste patterns detected"; *Given* `summary.findings = []` and at least one session has `unpricedModels.length === 0` (evaluable and clean), *When* `FindingsPanel` renders, *Then* it shows "No waste patterns detected" as today. A fixture test in `FindingsPanel.test.tsx` asserts these two empty-looking states render different text strings.
- Each finding card's action is keyboard-operable.
  - *Given* a finding card rendered with a "View session" action, *When* a keyboard user tabs to it and presses Enter, *Then* the same navigation/callback fires as a mouse click — because the action is a real `<button>`/`<a>`, not a `<div onClick>`.
**Files**: `web-app/src/components/ui/Badge.css.ts`, `web-app/src/app/insights/FindingsPanel.tsx`, `web-app/src/app/insights/FindingsPanel.css.ts`, `web-app/src/app/insights/InsightsDashboard.tsx`, `web-app/src/app/insights/FindingsPanel.test.tsx`

##### Task 1.1.6a: Add a `critical` intent to the shared `Badge` component (~3 min)
- Add a `critical` variant to `web-app/src/components/ui/Badge.css.ts`'s `intent` map using `vars.color.critical`/`criticalBg`/`criticalText` (already defined in `theme-contract.css.ts` — reuse, don't fork a new severity palette).
- Files: `web-app/src/components/ui/Badge.css.ts`

##### Task 1.1.6b: `FindingsPanel` component (~5 min)
- Create `FindingsPanel.tsx` + `FindingsPanel.css.ts`: `role="list"` container; one card per finding (`role="listitem"`) with a `Badge` for severity — mapping the generated `Severity` enum value (e.g. `Severity.CRITICAL` in the TS client generated from Task 1.1.1a's proto enum) to the Badge's `intent`/label via a small lookup table, not a raw string compare — (text label always rendered, never color-only), the finding's one-line `message`, a `~$`-prefixed dollar-impact (via Epic 1.2's shared `EstimatedValue` once that lands — until then, a local `~$` span is fine, swap it in during Story 1.2.5), and a focusable action element navigating to `/insights/session/[sessionId]` — since Epic 1.4's route may not exist yet, wire this to the existing `onSessionClick` prop (opens the modal) for now, and note in a code comment that the target should become a `<Link>` to the route once Epic 1.4 ships (Story 1.4.4 supersedes this). Four states: loading (skeleton, matches `InsightsDashboardSkeleton.tsx`'s pattern), computed-empty (genuinely clean), unpriced (couldn't be evaluated), error. Compute `unevaluableCount := summary.sessions.filter(s => s.unpricedModels.length > 0).length` when `findings.length === 0`: if `unevaluableCount > 0`, render "N sessions could not be evaluated (unpriced model)" (N = `unevaluableCount`) instead of the clean message — this must be checked before falling back to the clean-state text, since a dashboard where every session is unpriced would otherwise satisfy `findings.length === 0` and render the misleading "No waste patterns detected" (pre-mortem Failure #1); only when `unevaluableCount === 0` does `findings.length === 0` render "No waste patterns detected."
- Files: `web-app/src/app/insights/FindingsPanel.tsx`, `web-app/src/app/insights/FindingsPanel.css.ts`

##### Task 1.1.6c: Mount in `InsightsDashboard.tsx` (~3 min)
- Add a new `section` containing `FindingsPanel` above the existing `grid2` charts row (per `research/ux.md` §1 — charts stay verdict-free; findings own the verdict). Pass `summary?.findings`, `loading`, `error`, and `onSessionClick={(s) => setSelectedSession(s)}`.
- Files: `web-app/src/app/insights/InsightsDashboard.tsx`

##### Task 1.1.6d: Tests (~5 min)
- Assert all four states (loading, computed-empty, unpriced, error) render distinct, correct text; specifically assert the computed-empty and unpriced states render *different* strings given otherwise-identical `findings = []` input (the case pre-mortem Failure #1 flags — an all-unpriced fixture must not render "No waste patterns detected"); assert a finding card's action element is a real `<button>`/`<a>` and fires on Enter/Space.
- Files: `web-app/src/app/insights/FindingsPanel.test.tsx`

#### Story 1.1.7: Feature registry
**As a** maintainer, **I want** the registry to reflect the new component, **so that** `make registry-diff` stays clean.
**Acceptance Criteria**:
- The registry generator picks up the new marker with no manual edits needed.
  - *Given* `FindingsPanel.tsx` has `// +feature: insights-findings-panel` in its first 10 lines, *When* `make registry-generate` runs, *Then* `docs/registry/features/frontend/insights-findings-panel.json` is created and `git status` shows exactly that one new file for this story (plus any files already staged by prior stories in this epic).
**Files**: `docs/registry/features/frontend/insights-findings-panel.json`

##### Task 1.1.7a: Add marker and regenerate (~2 min)
- Confirm `FindingsPanel.tsx` (from 1.1.6b) carries the `// +feature: insights-findings-panel` marker; run `make registry-generate`; commit the generated/changed registry file(s).
- Files: `docs/registry/features/frontend/insights-findings-panel.json`

---

### Epic 1.2: Per-tool/activity cost breakdown

**Goal**: Extend `TopToolEntry` with a defensible cost figure (ADR-001) and add an activity-type classification so spend can be sliced by kind of work, not just model/project.

**Classification scope note**: v1 ships a 5-value `ActivityType` driven by skill-activation names first, tool-call ratios second — see Pattern Decisions above for why a finer, tool-argument-driven taxonomy is out of scope for this pass.

#### Story 1.2.1: Per-turn cost helper & tool-cost attribution
**As a** dashboard operator, **I want** turn-granularity cost math and the tool-type-level attribution from ADR-001, **so that** per-tool cost never inflates a session's summed tool-cost past its real total across a single tool type's own calls (ADR-001's caveat is about cross-tool-type double-counting, which is by design and surfaced via `cost_may_double_count`).
**Acceptance Criteria**:
- A multi-tool turn's cost is added once per distinct tool, not once per call.
  - *Given* a `ParseResult` with turn 1 (`ToolNames=["Read"]`, turn cost $1.00) and turn 2 (`ToolNames=["Read","Read","Grep"]`, turn cost $2.00 — 2 `Read` calls, 1 `Grep` call), *When* `tokens.AttributeToolCosts(r, pt)` runs, *Then* `costs["Read"] == 3.00` (not $4.00 — turn 2's cost is added to `Read` once, not per call) and `costs["Grep"] == 2.00`, and `doubleCounted["Read"] == true`, `doubleCounted["Grep"] == true`.
**Files**: `session/tokens/pricing.go`, `session/tokens/types.go`, `session/tokens/pricing_test.go`

##### Task 1.2.1a: `EstimateTurnCost` (~4 min)
- Add `func (pt *PricingTable) EstimateTurnCost(turn TurnStats) (cost float64, priced bool)` to `pricing.go`, mirroring `EstimateCost`'s per-family arithmetic at turn granularity (single model per turn, so no per-family map needed — a direct `pt.Prices[NormalizeModelFamily(turn.Model)]` lookup).
- Files: `session/tokens/pricing.go`

##### Task 1.2.1b: `AttributeToolCosts` (~5 min)
- Add `func AttributeToolCosts(r *ParseResult, pt *PricingTable) (costs map[string]float64, doubleCounted map[string]bool, unpriced map[string]bool)`: for each turn, compute `cost, priced := pt.EstimateTurnCost(turn)`; if `!priced`, mark every distinct name in `turn.ToolNames` `unpriced[name] = true` and skip (contribute nothing to `costs`) — this is what lets a tool that never had a priced turn stay distinguishable from a genuinely free one, per `requirements.md`'s "abstain rather than guess" rule; else for each **distinct** name in `turn.ToolNames` (dedupe via a per-turn `map[string]bool`), add `cost` to `costs[name]` once; if `len(distinct names in this turn) > 1`, mark all of them `doubleCounted[name] = true`. A tool name that appears in both `costs` and `unpriced` (some turns priced, some not) is not "unpriced" overall — `unpriced[name]` is only consulted by callers for names absent from `costs`.
- Files: `session/tokens/pricing.go`

##### Task 1.2.1c: `ToolTokenStats.CostUsd` (~2 min)
- Add `CostUsd float64` field to `ToolTokenStats` in `types.go` (doc comment: populated by `AttributeToolCosts`, not by the parser).
- Files: `session/tokens/types.go`

##### Task 1.2.1d: Fixture tests (~5 min)
- Cases: single-tool turn (no double-count), multi-tool turn (double-count flag set for all its tools), mixed session (some tools double-counted, some not), unpriced-model turn (skipped from `costs`, name present in `unpriced` instead of contributing 0).
- Files: `session/tokens/pricing_test.go`

#### Story 1.2.2: Wire tool cost into `TopToolEntry`
**As a** dashboard operator, **I want** `sessionTopTools` to populate the new cost fields without disturbing its existing ordering contract, **so that** the "top tools" list keeps meaning "most-called," with cost as an additional, not replacing, signal.
**Acceptance Criteria**:
- Cost population doesn't change the call-count-desc sort order.
  - *Given* a session where the most-called tool (`Read`, 50 calls) is on an unpriced model (no priced turns) and a less-called tool (`Bash`, 5 calls) has $5.00 attributed, *When* `sessionTopTools(r, pt)` runs, *Then* `Read` still sorts first (unchanged call-count-desc contract), `Bash`'s entry has `CostUsd == 5.00`, and `Read`'s entry has `CostUnpriced == true` (not `CostUsd == 0`).
**Files**: `proto/session/v1/insights.proto`, `server/services/insights_service.go`, `server/services/insights_service_test.go`

##### Task 1.2.2a: Proto fields (~2 min)
- Add `double cost_usd = 4;`, `bool cost_may_double_count = 5;`, and `bool cost_unpriced = 6;` (next free field number after `cost_may_double_count = 5`) to `TopToolEntry` in `insights.proto` — `cost_unpriced` is set when none of this tool's turns had priced-model coverage, so the frontend can render `—` instead of a misleading `$0.00`, mirroring the existing `unpricedModels`/unpriced-cost pattern already used elsewhere in this plan (e.g. Cache ROI, session-token-ceiling detector). Run `make proto-gen`.
- Files: `proto/session/v1/insights.proto`

##### Task 1.2.2b: Wire into `sessionTopTools` (~5 min)
- Change `sessionTopTools(r *tokens.ParseResult)` to `sessionTopTools(r *tokens.ParseResult, pt *tokens.PricingTable)`, calling `tokens.AttributeToolCosts(r, pt)` once at the top and populating `CostUsd`/`CostMayDoubleCount`/`CostUnpriced` per entry from its result (`CostUnpriced = unpriced[name] && !hasEntryIn(costs, name)`, i.e. true only when the tool never once got a priced turn). Update both call sites in `insights_service.go` (`GetInsightsSummary`, `ListSessionTokens`) to pass `s.pricing`.
- Files: `server/services/insights_service.go`

##### Task 1.2.2c: Tests (~4 min)
- Assert `cost_usd`/`cost_may_double_count`/`cost_unpriced` on a multi-tool-turn fixture; assert a tool with zero priced turns gets `cost_unpriced == true` and `cost_usd == 0` (not indistinguishable from a genuinely free tool); assert the call-count-desc ordering is unchanged.
- Files: `server/services/insights_service_test.go`

#### Story 1.2.3: Activity-type classification
**As a** dashboard operator, **I want** a documented, testable classification rule, **so that** a session's activity label is defensible from data already available, not a vague guess.
**Acceptance Criteria**:
- A skill-name signal outranks the tool-ratio fallback.
  - *Given* a `ParseResult` with `SkillActivations=[{Name:"code-debugging"}]` and a 90% `Read`-call ratio (which alone would suggest exploratory), *When* `tokens.ClassifyActivity(r)` runs, *Then* it returns `ActivityDebugging`.
**Files**: `session/tokens/activity.go`, `session/tokens/activity_test.go`, `server/services/insights_service.go`

##### Task 1.2.3a: `ActivityType` + `ClassifyActivity` (~5 min)
- Create `session/tokens/activity.go`: `ActivityType` as a Go type alias of the generated `sessionv1.ActivityType` proto enum (defined in Task 1.1.1a), plus constant aliases for each non-zero value (`ActivityDebugging = sessionv1.ActivityType_ACTIVITY_TYPE_DEBUGGING`, `ActivityRefactoring`, `ActivityFeatureDev`, `ActivityExploratory`, `ActivityOther`) — same alias pattern as `FindingType`/`Severity` (Task 1.1.1b). `ClassifyActivity(r *ParseResult) ActivityType`: priority 1 — case-insensitive substring match on each `r.SkillActivations[i].Name` against `"debug"`→`ActivityDebugging`, `"refactor"`→`ActivityRefactoring` (first match wins, checked in that order); priority 2 (no skill match) — compute `writeRatio`/`readRatio` from `r.ToolUsage` call counts (`Edit`+`Write`+`NotebookEdit` vs. `Read`+`Grep`+`Glob`, both over total calls); `writeRatio >= 0.3` → `ActivityFeatureDev`; else `readRatio >= 0.6` → `ActivityExploratory`; else `ActivityOther` (also `ActivityOther` when `len(r.ToolUsage) == 0`).
- Files: `session/tokens/activity.go`

##### Task 1.2.3b: Fixture tests (~4 min)
- Cover both signal tiers, the skill-match-wins-over-ratio case, and the zero-tool-call edge case.
- Files: `session/tokens/activity_test.go`

##### Task 1.2.3c: Set `SessionTokenSummary.ActivityType` (~3 min)
- In `GetInsightsSummary` and `ListSessionTokens`'s per-session build, set `summary.ActivityType = tokens.ClassifyActivity(r)` — a direct enum assignment, no string conversion, since `tokens.ActivityType` is a Go alias of the same generated `sessionv1.ActivityType` enum. (If Epic 1.5's `buildSessionSummary` extraction has already landed by the time this task runs, add it there instead of duplicating into 2 call sites — check `insights_service.go` for the helper's existence first.)
- Files: `server/services/insights_service.go`

#### Story 1.2.4: Dashboard-level activity-cost breakdown
**As a** dashboard operator, **I want** total cost sliced by activity type across all in-range sessions, **so that** I can see where spend concentrates by kind of work, not just by model.
**Acceptance Criteria**:
- Costs and session counts aggregate correctly per activity type.
  - *Given* 2 sessions classified `feature_dev` (costs $3.00, $4.00) and 1 session classified `debugging` (cost $2.00), *When* `GetInsightsSummary` runs, *Then* `resp.ActivityBreakdown` contains `{ActivityType: sessionv1.ActivityType_ACTIVITY_TYPE_FEATURE_DEV, EstimatedCostUsd:7.00, SessionCount:2}` and `{ActivityType: sessionv1.ActivityType_ACTIVITY_TYPE_DEBUGGING, EstimatedCostUsd:2.00, SessionCount:1}`.
**Files**: `proto/session/v1/insights.proto`, `server/services/insights_service.go`, `server/services/insights_service_test.go`

##### Task 1.2.4a: `ActivityCostBreakdown` message + field (~3 min)
- Add `message ActivityCostBreakdown { ActivityType activity_type = 1; double estimated_cost_usd = 2; int32 session_count = 3; }` (reusing the `ActivityType` enum defined in Task 1.1.1a, not a plain `string`) and `repeated ActivityCostBreakdown activity_breakdown = 15;` on `GetInsightsSummaryResponse`. Run `make proto-gen`.
- Files: `proto/session/v1/insights.proto`

##### Task 1.2.4b: Accumulate in the loop (~4 min)
- Add `activityMap := make(map[sessionv1.ActivityType]*sessionv1.ActivityCostBreakdown)` alongside `modelMap`/`toolMap` in `GetInsightsSummary`; in the per-session loop, increment the matching entry's `EstimatedCostUsd`/`SessionCount` using the just-computed `activityType`/`costUSD`. After the loop, build a cost-desc-sorted slice into `resp.ActivityBreakdown`, same pattern as `models`.
- Files: `server/services/insights_service.go`

##### Task 1.2.4c: Tests (~3 min)
- Files: `server/services/insights_service_test.go`

#### Story 1.2.5: Frontend — per-tool cost + activity breakdown display
**As a** dashboard operator, **I want** modeled numbers visually distinct from measured ones, **so that** I never mistake a heuristic figure for a metered one.
**Acceptance Criteria**:
- A double-counting-eligible tool cost renders with the estimated marker; a non-eligible one doesn't.
  - *Given* `session.topTools[0].costMayDoubleCount === true` and `session.topTools[1].costMayDoubleCount === false`, *When* `SessionDetailDrawer`'s Tools Breakdown table renders both rows, *Then* row 0's cost cell shows a `~$` prefix with an `aria-describedby` tooltip explaining the attribution method, and row 1's cost cell shows a plain `$` figure with no marker.
- A tool with no priced turns renders `—`, never `$0.00`.
  - *Given* `session.topTools[2].costUnpriced === true`, *When* the Tools Breakdown table renders that row, *Then* the cost cell shows `—` (matching the existing unpriced-cost badge convention elsewhere on the dashboard), not `$0.00` and not the `~$` estimated-value marker — per `requirements.md`'s "abstain rather than guess" rule.
**Files**: `web-app/src/components/ui/EstimatedValue.tsx`, `web-app/src/components/ui/EstimatedValue.css.ts`, `web-app/src/app/insights/SessionDetailDrawer.tsx`, `web-app/src/app/insights/ActivityBreakdownTable.tsx`, `web-app/src/app/insights/ActivityBreakdownTable.css.ts`, `web-app/src/app/insights/InsightsDashboard.tsx`

##### Task 1.2.5a: Shared `EstimatedValue` component (~5 min)
- Create `web-app/src/components/ui/EstimatedValue.tsx` + `.css.ts`: renders `~{children}` with `aria-describedby` pointing at a `title`/tooltip prop explaining the estimation method, using a single shared muted-weight style token (`estimatedValueMarker`) per `research/pitfalls.md` §2 and `research/ux.md` §4. Reused (not forked) by this story's tool-cost column, Story 1.2.5c's activity table, and Epic 1.1/1.3's waste-score/cache-ROI cells.
- Files: `web-app/src/components/ui/EstimatedValue.tsx`, `web-app/src/components/ui/EstimatedValue.css.ts`

##### Task 1.2.5b: Cost column in `SessionDetailDrawer`'s Tools Breakdown table (~3 min)
- Add a "Cost" `<th>`/`<td>` column: render `—` (the existing unpriced-cost badge treatment) when `t.costUnpriced`; else render via `EstimatedValue` when `t.costMayDoubleCount`; else plain `fmtCost` text. `costUnpriced` is checked first — a tool can't simultaneously be "no priced turns" and "possibly double-counted," but ordering the check this way keeps the precedence explicit for future fields.
- Files: `web-app/src/app/insights/SessionDetailDrawer.tsx`

##### Task 1.2.5c: `ActivityBreakdownTable` (~5 min)
- New component, same shape as `TopNTables.tsx`'s `TopNTable`, rendered from `summary.activityBreakdown`; map each row's generated `ActivityType` enum value to a human label via a small `activityTypeLabels` lookup (e.g. `ACTIVITY_TYPE_FEATURE_DEV` → "Feature Dev") rather than rendering the raw enum name; mount it in `InsightsDashboard.tsx`'s existing "Top Usage" `grid2` section alongside `TopNTable` for skills/tools.
- Files: `web-app/src/app/insights/ActivityBreakdownTable.tsx`, `web-app/src/app/insights/ActivityBreakdownTable.css.ts`, `web-app/src/app/insights/InsightsDashboard.tsx`

##### Task 1.2.5d: Component tests (~4 min)
- `EstimatedValue.test.tsx`: marker + tooltip render correctly. `ActivityBreakdownTable.test.tsx`: rows render sorted by cost desc.
- Files: `web-app/src/components/ui/EstimatedValue.test.tsx`, `web-app/src/app/insights/ActivityBreakdownTable.test.tsx`

#### Story 1.2.6: Feature registry
**Acceptance Criteria**:
- *Given* the new `ActivityBreakdownTable.tsx`/`EstimatedValue.tsx` components carry `// +feature:` markers, *When* `make registry-generate` runs, *Then* their registry entries appear with no manual edits.
**Files**: `docs/registry/features/frontend/*.json`

##### Task 1.2.6a: Regenerate and commit (~2 min)

---

### Epic 1.3: Richer sort/search on the sessions table

**Goal**: Add duration, cost-per-message, cache-ROI, and waste-score as sortable columns, entirely client-side (ADR-003), each with its own documented missing-value handling.

#### Story 1.3.1: Cache ROI computation
**As a** dashboard operator, **I want** cache ROI computed and abstained on cleanly for unpriced sessions, **so that** the new column never shows a misleading `$0.00` or `NaN`.
**Acceptance Criteria**:
- An unpriced-model session's cache ROI is undefined, not zero.
  - *Given* a `SessionTokenSummary` with `unpricedModels=["claude-opus-5"]`, *When* the sessions table renders that row's Cache ROI cell, *Then* it shows "—" (matching the existing `unpricedBadge` treatment on the Cost column), never `$0.00`.
**Files**: `session/tokens/pricing.go`, `session/tokens/pricing_test.go`, `server/services/insights_service.go`

##### Task 1.3.1a: `ComputeCacheROI` (~4 min)
- `func ComputeCacheROI(r *ParseResult, pt *PricingTable) (roi float64, ok bool)`: look up the primary model family's pricing; if not found, return `(0, false)`; else `roi = float64(r.CacheRead)*(pricing.InputPricePerMTok-pricing.CacheReadPerMTok)/1e6 - float64(r.CacheCreation)*pricing.CacheWritePerMTok/1e6`, return `(roi, true)`.
- Files: `session/tokens/pricing.go`

##### Task 1.3.1b: Fixture tests (~4 min)
- Positive ROI, negative ROI (cache write never read back — e.g. `CacheCreation` high, `CacheRead` 0), unpriced-model `ok=false`.
- Files: `session/tokens/pricing_test.go`

##### Task 1.3.1c: Set `SessionTokenSummary.CacheRoiUsd` (~3 min)
- In `GetInsightsSummary`/`ListSessionTokens`'s per-session build, call `ComputeCacheROI`; set `summary.CacheRoiUsd = roi` when `ok` (leave as `0` with `UnpricedModels` already non-empty signaling "undefined" when `!ok` — no new proto flag needed, the frontend keys off `unpricedModels.length > 0` exactly as the existing Cost column already does).
- Files: `server/services/insights_service.go`

#### Story 1.3.2: Client-side sort — duration & cost-per-message
**As a** dashboard operator, **I want** to sort sessions by how long they ran and their cost efficiency per message, **so that** I can find outliers the existing 4 columns don't surface.
**Acceptance Criteria**:
- Cost-per-message never produces `NaN` in the comparator.
  - *Given* two sessions, A (`messageCount=0`, `estimatedCostUsd=5`) and B (`messageCount=10`, `estimatedCostUsd=5`), *When* sorted by "cost per message" ascending, *Then* A sorts to the end regardless of direction (guarded before the ascending/descending flip, mirroring the existing unpriced-cost pattern at `SessionsTable.tsx:113-120`) — never compared via a raw `5/0` division.
**Files**: `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/SessionsTable.test.tsx`

##### Task 1.3.2a: Extend `SortColumn` + comparators (~5 min)
- Add `"duration" | "costPerMessage"` to the `SortColumn` union. Duration comparator: `(lastMessageAt - firstMessageAt)` in seconds, missing timestamp → treated as `0` (documented decision: unlike cost, a missing duration isn't "bad," so it sorts at its natural numeric position, not pushed to either end). Cost-per-message comparator: guard `messageCount === 0` as a sort-last bucket (same early-return pattern as the existing `"cost"` case), else `estimatedCostUsd / messageCount`.
- Files: `web-app/src/app/insights/SessionsTable.tsx`

##### Task 1.3.2b: Sortable header cells (~3 min)
- Add "Duration" and "Cost/Msg" header cells via the existing `sortableHeaderCell` helper — no new markup pattern.
- Files: `web-app/src/app/insights/SessionsTable.tsx`

##### Task 1.3.2c: Tests (~4 min)
- `aria-sort` assertions for both new columns (mirrors the existing pattern at `SessionsTable.test.tsx:113,129`); the 0-message guard case.
- Files: `web-app/src/app/insights/SessionsTable.test.tsx`

#### Story 1.3.3: Client-side sort — cache ROI & waste score
**As a** dashboard operator, **I want** to sort by cache ROI and waste score, **so that** the richer heuristics from Epics 1.1/1.3 are actually usable for finding the worst sessions.
**Acceptance Criteria**:
- Negative ROI and "no ROI" are visually distinct, not the same badge.
  - *Given* session A with `cacheRoiUsd = -0.42` and session B with `unpricedModels.length > 0`, *When* both render in the Cache ROI column, *Then* A's cell shows `-$0.42` as plain signed text (no badge) and B's cell shows the same `unpricedBadge` the Cost column uses — two visually distinct states.
- Waste score distinguishes "not evaluated" from "evaluated, unpriced" from a real value.
  - *Given* session A (waste score unset — too few turns), session B (`unpricedModels.length > 0`), and session C (`wasteScore = 62`), *When* the Waste Score column renders all three, *Then* A shows "Not evaluated", B shows "—" (unpriced), and C shows "62", each textually distinct — not all three rendering as the same blank/dash.
**Files**: `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/SessionsTable.css.ts`, `web-app/src/app/insights/SessionsTable.test.tsx`

##### Task 1.3.3a: Extend `SortColumn` + comparators (~5 min)
- Add `"cacheRoi" | "wasteScore"`. Cache-ROI comparator: sort-last bucket = `unpricedModels.length > 0` (same guard as `"cost"`), else compare `cacheRoiUsd` directly (negative values sort normally, not treated as missing). Waste-score comparator: sort-last bucket = `unpricedModels.length > 0` OR `wasteScore === undefined` (both go to the end; which of the two doesn't matter for sort order, only for cell text — see task b), else compare `wasteScore`.
- Files: `web-app/src/app/insights/SessionsTable.tsx`

##### Task 1.3.3b: Header cells + three-state cell rendering (~5 min)
- Add "Cache ROI" and "Waste Score" header cells. Cache-ROI cell: signed `$` text (`+$X.XX`/`-$X.XX`), no color-only cue. Waste-score cell: `"Not evaluated"` when `wasteScore === undefined && unpricedModels.length === 0`, `"—"` when `unpricedModels.length > 0`, else the numeric score.
- Files: `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/SessionsTable.css.ts`

##### Task 1.3.3c: Tests (~5 min)
- `aria-sort` for both columns; the sign-aware ROI text assertion; the three-bucket waste-score text assertion.
- Files: `web-app/src/app/insights/SessionsTable.test.tsx`

#### Story 1.3.4: Search/sort coexistence verification
**As a** dashboard operator, **I want** confirmation that text search still scans the full session set after adding sortable columns, **so that** ADR-003's whole rationale (no page-drift risk) is actually verified, not just argued.
**Acceptance Criteria**:
- Search results are unaffected by which column is sorted.
  - *Given* 600 in-memory sessions and an active search term matching 3 of them, *When* the user then clicks the "Waste Score" column header, *Then* the displayed set stays those same 3 matching sessions, now ordered by waste score.
**Files**: `web-app/src/app/insights/SessionsTable.test.tsx`

##### Task 1.3.4a: Add the coexistence test (~3 min)
- Files: `web-app/src/app/insights/SessionsTable.test.tsx`

---

### Epic 1.4: Deep-linkable session drill-down route

**Goal**: A real `/insights/session/[sessionId]` route, bookmarkable/shareable, reusing existing RPCs (`research/architecture.md` §4), keeping the modal as a quick-peek that shares one rendering implementation with the route.

#### Story 1.4.1: Backend filter gap fix — support orphan sessions
**As a** dashboard operator, **I want** to deep-link an orphan session (one with no matching stapler-squad session), **so that** the route works for every row the table can show, not just associated sessions.
**Acceptance Criteria**:
- An orphan session, unselectable via `session_id_filter` alone today, becomes selectable via the new filter.
  - *Given* an orphan `ParseResult` (`SessionUUID="conv-123"`, `AssociateWithSnapshot` returns `sessionID=""` because no stapler-squad session matches), *When* `GetInsightsSummary` is called with `conversation_id_filter="conv-123"`, *Then* the response contains exactly that one session — confirmed as a real gap (not a hypothetical) by reading `insights_service.go:144-149`: `session_id_filter`'s check is itself gated by `*msg.SessionIdFilter != ""`, so no value of that filter can ever match an orphan's empty `sessionID`.
**Files**: `proto/session/v1/insights.proto`, `server/services/insights_service.go`, `server/services/insights_service_test.go`

##### Task 1.4.1a: `conversation_id_filter` proto field (~2 min)
- Add `optional string conversation_id_filter = 6;` to `GetInsightsSummaryRequest`. Run `make proto-gen`.
- Files: `proto/session/v1/insights.proto`

##### Task 1.4.1b: OR the two filters (~3 min)
- In `GetInsightsSummary`'s filter block, change the session-ID filter check to: skip the session unless (`session_id_filter` unset or matches `sessionID`) **or** (`conversation_id_filter` unset or matches `r.SessionUUID`) — i.e. if either filter is set and matches, keep the session; if both are set and neither matches, skip it. (Equivalent framing: apply each filter only when non-empty, keep the session if it passes at least one of the filters that was actually set; if neither filter is set, keep everything as today.)
- Files: `server/services/insights_service.go`

##### Task 1.4.1c: Tests (~4 min)
- Orphan-lookup-by-conversation-id-filter case; regression case confirming `session_id_filter` alone still works exactly as before for non-orphan sessions.
- Files: `server/services/insights_service_test.go`

#### Story 1.4.2: Extract shared session-detail content component
**As a** developer, **I want** the modal and the route to render from one component, **so that** they can't drift apart the way `research/pitfalls.md` §4 warns a naive migration would let them.
**Acceptance Criteria**:
- Modal and route render identical output from the same inputs.
  - *Given* the same `SessionTokenSummary`, `backlogEntry`, and `turns` props, *When* rendered via `SessionDetailDrawer` (modal path) and via the new route's client component (Story 1.4.3), *Then* both produce the same `SessionDetailContent` output — verified by both `SessionDetailDrawer.test.tsx` and the new route's test asserting against the same rendering assertions extracted into `SessionDetailContent.test.tsx`.
- The modal's pre-existing missing-focus-management gap is fixed in this same pass, not left inconsistent with the new route's higher a11y bar.
  - *Given* the modal opens, *When* it mounts, *Then* focus moves to its close button (`useRef` + `.focus()`), and *When* it closes, *Then* focus restores to the element that triggered it — neither of which happens today (confirmed: no focus-trap/initial-focus code exists in current `SessionDetailDrawer.tsx`).
**Files**: `web-app/src/app/insights/SessionDetailContent.tsx`, `web-app/src/app/insights/SessionDetailDrawer.tsx`, `web-app/src/app/insights/SessionDetailContent.test.tsx`, `web-app/src/app/insights/SessionDetailDrawer.test.tsx`

##### Task 1.4.2a: Extract `SessionDetailContent` (~5 min)
- Move everything inside `SessionDetailDrawer.tsx`'s `drawer` div (Metadata section through Skill Activations section) into a new `SessionDetailContent.tsx` taking `{session, backlogEntry, turns}` as props — no dialog/overlay chrome, no `role="dialog"`/Escape handling (those stay in the drawer).
- Files: `web-app/src/app/insights/SessionDetailContent.tsx`, `web-app/src/app/insights/SessionDetailDrawer.tsx`

##### Task 1.4.2b: Fix modal focus management in the same pass (~4 min)
- In `SessionDetailDrawer.tsx`: add a `closeButtonRef`, focus it on mount (`useEffect` when `session` becomes non-null), and restore focus to `document.activeElement` captured before opening, on close.
- Files: `web-app/src/app/insights/SessionDetailDrawer.tsx`

##### Task 1.4.2c: Split tests (~4 min)
- Move content-rendering assertions to `SessionDetailContent.test.tsx`; keep dialog-chrome/focus-management assertions in `SessionDetailDrawer.test.tsx`.
- Files: `web-app/src/app/insights/SessionDetailContent.test.tsx`, `web-app/src/app/insights/SessionDetailDrawer.test.tsx`

#### Story 1.4.3: The route itself
**As a** dashboard operator, **I want** a direct navigation (bookmark, refresh, shared link) to `/insights/session/[sessionId]` to render correctly with no dependency on the dashboard's in-memory state, **so that** the route is genuinely deep-linkable, not just reachable by clicking through the table.
**Acceptance Criteria**:
- A cold direct navigation works with no parent state.
  - *Given* a fresh browser tab navigated directly to `/insights/session/abc123` (no prior client-side navigation from `InsightsDashboard`), *When* the page mounts, *Then* it fetches `GetInsightsSummary({sessionIdFilter:"abc123", conversationIdFilter:"abc123", includeOrphans:true})` (ignoring any dashboard date-range filter, per `research/features.md` §5's bookmarkability recommendation) and `GetSessionTurnTimeline({conversationId: <resolved conversationId>})`, then renders `SessionDetailContent` with the result.
- A session that no longer exists degrades gracefully.
  - *Given* a `sessionId` that matches no session in the fetch response, *When* the page finishes loading, *Then* it shows an explicit "Session not found" message, not a crash or a blank page.
**Files**: `web-app/src/app/insights/session/[sessionId]/page.tsx`, `web-app/src/app/insights/session/[sessionId]/SessionDetailPageClient.tsx`, `web-app/src/lib/hooks/useInsightsService.ts`, `web-app/src/app/insights/session/[sessionId]/SessionDetailPageClient.test.tsx`

##### Task 1.4.3a: Route entry (~3 min)
- Create `page.tsx`: `export default async function SessionDetailPage({ params }: { params: Promise<{ sessionId: string }> }) { const { sessionId } = await params; return <SessionDetailPageClient sessionId={sessionId} />; }` (Next 15's `params` is a Promise — the one version gotcha per `research/stack.md`).
- Files: `web-app/src/app/insights/session/[sessionId]/page.tsx`

##### Task 1.4.3b: `useSessionDetail` hook (~4 min)
- Add to `useInsightsService.ts`: `useSessionDetail(sessionId: string)` — calls `client.getInsightsSummary` with `sessionIdFilter: sessionId, conversationIdFilter: sessionId, includeOrphans: true` and no `from`/`to` (filter-independent of the dashboard's global range). Returns `{summary, loading, error}` where `summary` is `res.sessions[0] ?? null`.
- Files: `web-app/src/lib/hooks/useInsightsService.ts`

##### Task 1.4.3c: `SessionDetailPageClient` (~5 min)
- Fetch via `useSessionDetail(sessionId)` + `useSessionTurnTimeline(summary?.conversationId)`; render `SessionDetailContent`; explicit "Session not found" state when `!loading && !error && !summary`.
- Files: `web-app/src/app/insights/session/[sessionId]/SessionDetailPageClient.tsx`

##### Task 1.4.3d: Focus management on route mount (~3 min)
- Move focus to the page's heading on mount (`ref` + `tabIndex={-1}` + `.focus()` in a `useEffect`) — per `research/ux.md` §3, a route gets no free `role="dialog"` AT signal, so this is the only cue a screen reader gets that content changed.
- Files: `web-app/src/app/insights/session/[sessionId]/SessionDetailPageClient.tsx`

##### Task 1.4.3e: Tests (~5 min)
- Found / not-found / direct-navigation-without-parent-state cases.
- Files: `web-app/src/app/insights/session/[sessionId]/SessionDetailPageClient.test.tsx`

#### Story 1.4.4: Wire navigation from the table; keep the modal as quick-peek
**As a** dashboard operator, **I want** the table's row click to keep opening the quick-peek modal, with a separate explicit link to the full bookmarkable page, **so that** I don't lose my place in the table for a quick glance, per `requirements.md`'s explicit allowance.
**Acceptance Criteria**:
- An orphan session's "open full page" link resolves to a non-empty path segment.
  - *Given* the table is showing an orphan session (`isOrphan=true`, empty `sessionId`, `conversationId="conv-999"`), *When* the user activates its "Open full page" link inside the (now-open) modal, *Then* the browser navigates to `/insights/session/conv-999` — built from `session.sessionId || session.conversationId`, never a bare `/insights/session/`.
- Row click behavior is unchanged.
  - *Given* the sessions table, *When* a user clicks anywhere on a session row, *Then* the existing quick-peek modal opens (unchanged `onSessionClick` behavior) — no navigation occurs.
**Files**: `web-app/src/app/insights/SessionDetailDrawer.tsx`, `web-app/src/app/insights/SessionsTable.test.tsx`, `web-app/src/app/insights/FindingsPanel.tsx`

##### Task 1.4.4a: "Open full page" link in the modal (~3 min)
- Add a `next/link` `<Link href={`/insights/session/${session.sessionId || session.conversationId}`}>Open full page ↗</Link>` in `SessionDetailDrawer.tsx`'s header area (rendered alongside `SessionDetailContent`, not inside it — the drawer decides its own chrome).
- Files: `web-app/src/app/insights/SessionDetailDrawer.tsx`

##### Task 1.4.4b: Regression test for row click (~3 min)
- Assert clicking a row still calls `onSessionClick` (opens the modal), not a route push.
- Files: `web-app/src/app/insights/SessionsTable.test.tsx`

##### Task 1.4.4c: Point `FindingsPanel`'s single-session action at the route (~3 min)
- Now that the route exists, change `FindingsPanel.tsx` (Story 1.1.6b) to render its per-finding action as a `<Link href={`/insights/session/${finding.sessionId || finding.conversationId}`}>` instead of the interim `onSessionClick` call — satisfies `research/ux.md` §2's "single hop, not filter-then-scan" recommendation for single-session findings.
- Files: `web-app/src/app/insights/FindingsPanel.tsx`

#### Story 1.4.5: Feature registry
**Acceptance Criteria**:
- *Given* the new route's page component carries a `// +feature:` marker and `GetInsightsSummaryRequest`'s new field is used, *When* `make registry-generate` runs, *Then* the relevant frontend/backend registry files update with no manual edits.
**Files**: `docs/registry/features/frontend/*.json`, `docs/registry/features/backend/GetInsightsSummary.json`

##### Task 1.4.5a: Regenerate and commit (~2 min)

---

### Epic 1.5: Fix `WatchInsights`'s dead per-session live-update path

**Goal**: Thread the changed `*tokens.ParseResult` through `TokenStore`'s subscribe channel so `InsightsEvent.Session` is actually populated, making the frontend's already-written per-session live-patch branch fire for real. Fully decoupled from Epic 1.4 per `requirements.md`'s explicit instruction — no dependency either direction.

#### Story 1.5.1: Thread the changed `ParseResult` through `TokenStore`'s subscribe channel
**As a** dashboard operator, **I want** `TokenStore.Subscribe()` to hand subscribers the file that actually changed, **so that** `watchInsights` has something to build a per-session event from.
**Acceptance Criteria**:
- A single-file reparse notification carries that file's result; a full-walk-complete notification carries `nil`.
  - *Given* a subscriber channel from `ts.Subscribe()`, *When* `parseAndCache` finishes reparsing one file, *Then* the subscriber receives a non-nil `*tokens.ParseResult` equal to that file's freshly parsed result; *Given* the initial directory walk completes, *When* its deferred notify fires, *Then* the subscriber receives `nil` on the same channel.
**Files**: `session/tokens/store.go`, `session/tokens/types.go`, `session/tokens/store_test.go`

##### Task 1.5.1a: Change the channel/field types (~3 min)
- `TokenStore.subs []chan struct{}` → `[]chan *ParseResult`; `Subscribe() <-chan struct{}` → `Subscribe() <-chan *ParseResult`; `notify()` → `notify(result *ParseResult)` (sends `result` non-blockingly to each subscriber, same `select`/`default` pattern as today).
- Files: `session/tokens/store.go`

##### Task 1.5.1b: Update the two call sites (~3 min)
- `parseAndCache` (currently `ts.notify()` at line 226) → `ts.notify(result)` (the just-parsed result is already in scope there). `walkAndEnqueue`'s deferred call (currently `ts.notify()` at line 239) → `ts.notify(nil)`.
- Files: `session/tokens/store.go`

##### Task 1.5.1c: Update `TokenStoreReader` interface (~2 min)
- `Subscribe() <-chan struct{}` → `Subscribe() <-chan *ParseResult`; `Unsubscribe(ch <-chan struct{})` → `Unsubscribe(ch <-chan *ParseResult)`.
- Files: `session/tokens/types.go`

##### Task 1.5.1d: Update tests + the `fakeTokenStore` test double (~5 min)
- Update `store_test.go`'s existing Subscribe-based assertions for the new channel type; add a test asserting the single-file-reparse path delivers the correct `*ParseResult`. Update `server/services/insights_service_test.go`'s `fakeTokenStore.Subscribe()`/`Unsubscribe()` signatures to match.
- Files: `session/tokens/store_test.go`, `server/services/insights_service_test.go`

#### Story 1.5.2: Consolidate the per-session summary builder
**As a** developer, **I want** one `buildSessionSummary` function instead of near-duplicate inline blocks, **so that** Story 1.5.3's fix builds its per-session `WasteFinding`-aware, cost-aware summary from the same logic `GetInsightsSummary`/`ListSessionTokens` already use, rather than adding a third copy.
**Acceptance Criteria**:
- The extracted function's output matches what the inline blocks produced.
  - *Given* the same `ParseResult`, `PricingTable`, and associator snapshot, *When* `buildSessionSummary` is called directly, *Then* its output is byte-identical (via `proto.Equal`) to a hand-built expected `SessionTokenSummary` covering every field the old inline blocks set (including this project's `WasteScore`/`CacheRoiUsd`/`ActivityType`/tool-cost fields from Epics 1.1–1.3, whichever have landed by the time this task runs).
**Files**: `server/services/insights_service.go`, `server/services/insights_service_test.go`

##### Task 1.5.2a: Extract `buildSessionSummary` (~5 min)
- `func buildSessionSummary(r *tokens.ParseResult, pt *tokens.PricingTable, associator *tokens.Associator, snapshot []tokens.SessionRecord) *sessionv1.SessionTokenSummary`, folding in the full per-session build logic (sessionID/isOrphan resolution, cost, cache-hit-rate, top tools, skill names, timestamps, and — per whichever of Epics 1.1–1.3 have landed — waste score, cache ROI, activity type) currently duplicated across `GetInsightsSummary` (`:169-192`) and `ListSessionTokens` (`:404-427`).
- Files: `server/services/insights_service.go`

##### Task 1.5.2b: Update both existing call sites (~3 min)
- `GetInsightsSummary` and `ListSessionTokens` call `buildSessionSummary` instead of their inline blocks.
- Files: `server/services/insights_service.go`

##### Task 1.5.2c: Verify existing tests still pass unmodified (~2 min)
- Pure refactor — `make test` on `server/services` should pass with no test-file edits. If any test asserted on internal ordering/structure that changed incidentally, fix the test, not the refactor.
- Files: `server/services/insights_service_test.go` (verification-only, edit only if a test genuinely breaks)

#### Story 1.5.3: Populate `InsightsEvent.Session` in `watchInsights`
**As a** dashboard operator, **I want** a real per-session payload on live "update" events, and a real "parse_complete" event when the initial walk finishes, **so that** the frontend's existing (currently dead) per-session patch branch fires, and the frontend's full-refetch branch fires when it's supposed to.
**Acceptance Criteria**:
- A non-nil channel receive produces a populated `update` event.
  - *Given* `watchInsights`'s subscribed channel receives a non-nil `*tokens.ParseResult` for conversation "abc", *When* it builds and sends the event, *Then* `evt.EventType == "update"` and `evt.Session.ConversationId == "abc"` — populated, not the current always-nil status quo.
- A nil channel receive (walk complete) produces a real `parse_complete`, not another bare `update`.
  - *Given* the channel instead receives `nil`, *When* it builds and sends the event, *Then* `evt.EventType == "parse_complete"` and `evt.AllParsed == true` — matching the shape of the initial pre-subscribe event, correcting a second gap found during this planning pass: today, the walk-complete `notify()` call produces an `"update"` event indistinguishable from any other, so the frontend's `fetchSummary()`-triggering branch (which only listens for `"parse_complete"`) never re-fires after the very first stream message.
**Files**: `server/services/insights_service.go`, `server/services/insights_service_test.go`

##### Task 1.5.3a: Update the receive loop (~5 min)
- In `watchInsights`'s `select`/`case result, ok := <-ch:` branch: if `!ok`, return as today; else if `result != nil`, call `associator.Snapshot()` + `buildSessionSummary(result, s.pricing, s.associator, snapshot)`, set `evt.EventType = "update"`, `evt.Session = summary`, `evt.AllParsed = !s.store.IsLoading()`; else (`result == nil`), set `evt.EventType = "parse_complete"`, `evt.AllParsed = true`, leave `evt.Session` unset.
- Files: `server/services/insights_service.go`

##### Task 1.5.3b: Tests (~5 min)
- Using the existing `insightsEventSender` fake: assert both event shapes (populated `update`, real `parse_complete` on `nil`) are sent correctly for their respective channel inputs.
- Files: `server/services/insights_service_test.go`

#### Story 1.5.4: Frontend verification (no code change expected)
**As a** developer, **I want** confirmation that `useInsightsService.ts`'s existing per-session patch branch now actually executes, **so that** this fix is verified end-to-end, not just on the backend.
**Acceptance Criteria**:
- The previously-unreachable branch now fires and patches in place.
  - *Given* a mocked `WatchInsights` stream emitting `{eventType:"update", session:{conversationId:"abc", estimatedCostUsd:9.99, ...}}`, *When* `useInsightsSummary`'s stream handler processes it, *Then* `summary.sessions` is updated in place for `conversationId==="abc"` with no `fetchSummary()` refetch call — exercising `useInsightsService.ts:109-133`'s existing logic for the first time in a test.
**Files**: `web-app/src/lib/hooks/useInsightsService.test.ts`

##### Task 1.5.4a: Add the test (~4 min)
- If no test file exists yet for this hook, create one; mock the ConnectRPC stream to emit the event above and assert the resulting state.
- Files: `web-app/src/lib/hooks/useInsightsService.test.ts`

#### Story 1.5.5: Feature registry
**Acceptance Criteria**:
- *Given* `WatchInsights`'s backend behavior changed but its RPC signature/fields did not, *When* `make registry-generate` runs, *Then* it reports no diff for `docs/registry/features/backend/WatchInsights.json` (or a trivial one) — confirming this fix needed no registry update, which is itself worth verifying rather than assuming.
**Files**: `docs/registry/features/backend/WatchInsights.json`

##### Task 1.5.5a: Run and verify (~2 min)
- Run `make registry-generate`; if a diff appears, commit it; if not, note "no registry change needed" in the PR description.
