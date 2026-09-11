# Requirements: insights-cost-intelligence

**Date**: 2026-09-03
**Type**: feature addition
**Complexity**: 3 — system design (multiple workstreams: proto/backend aggregation, a new heuristics engine, and a new frontend route, Large appetite)

## Problem Statement

The `/insights` dashboard (shipped via `project_plans/insights/`, `insights-cost-pricing-gaps/`, `token-cost-tracking/` — PRs #98, #104, #280, #304) already surfaces totals, trends, model breakdowns, and a session drill-down modal, but it only shows *what happened*, never *what to do about it*. Charts are all description, no verdict: a 99.8% cache-hit-rate tile looks great even if most of that cache write is never read back; a $131 session shows up as a row, not a flagged anomaly; per-tool usage is call counts with no cost attached; and finding a specific session requires scanning path/model text search rather than sorting by the metric that actually matters (duration, cost-per-message, cache ROI, a waste score). External references reviewed for this project — Netflix's internal `claude-code-cost-optimizer` skill, `Tanisha-Katara/cacheeconomics`, `happy-token/TokenUsage`, and `lucemia/claude-session-analyzer` — all converge on the same idea stapler-squad is missing: severity-ranked, dollar-impact-tagged findings that tell the user which lever to pull first, not just a bigger pile of charts.

## Baseline

Today a user who wants to know "why did spend spike" or "which session/pattern is wasting money" has to manually eyeball the Daily Spend chart for a spike, open sessions one at a time via the drill-down modal, and mentally estimate whether a given cache-hit rate or tool-call count is actually bad — there is no computed verdict, no per-tool cost, and no way to sort/search sessions by anything beyond path substring match, model, and the four raw token/cost columns already rendered.

## Users / Consumers

Solo developer (Tyler) and any other operator of a stapler-squad instance, reviewing their own Claude Code spend/usage inside the existing `/insights` route (React SPA + Go/ConnectRPC backend). No external or third-party consumers.

## Success Metrics

1. **Find the driver fast**: given an elevated-spend period, the findings panel surfaces the specific responsible session or pattern directly (not buried in a chart) — target: identifiable in under a minute without manually opening sessions one-by-one.
2. **Findings get acted on and spend drops**: at least one waste finding per active week gets surfaced with enough specificity (which session/config, what to change) that acting on it is a specific action, not a vague number; measurable as a downward step in relevant per-model or per-project spend after the fix lands.

Both are qualitative-with-a-number targets (no existing telemetry pipeline to compute these automatically) — validated by Tyler's own usage of the dashboard post-ship, not an automated dashboard-of-the-dashboard.

## Appetite

Large (3–6 weeks). Five workstreams are in scope for this pass (see below) — if research/planning finds the full set doesn't fit, cut a workstream entirely rather than shipping all five half-finished.

## Constraints

- No deadline pressure — personal/internal tool, no external commitment.
- Must build on the existing `SessionTokenSummary`/`ListSessionTokens`/`GetInsightsSummary`/`WatchInsights` proto and `session/tokens` package rather than replacing them (per prior projects' non-goals: don't re-litigate JSONL parsing, the pricing table, or the dashboard shell — all shipped and working).
- Any new proto RPC/field requires `make proto-gen` and updating `docs/registry/features/` per this repo's feature-registry rule (already a recurring gap flagged in `token-cost-tracking/requirements.md`).
- CSS via vanilla-extract `.css.ts` per ADR-009; no inline layout styles.

## Non-functional Requirements

- **Performance SLO**: findings computation and per-tool cost aggregation must not visibly regress current `GetInsightsSummary`/`ListSessionTokens` response times (baseline: dashboard usable with 605 sessions today per current screenshot) — target no more than low-hundreds-of-ms added latency; compute incrementally/cached where the existing `TokenStore` design already allows it, not a full re-scan per request.
- **Scalability**: current volume is ~600 sessions / tens of millions of tokens for a single-operator instance; no need to design for multi-tenant or order-of-magnitude-larger volume.
- **Security classification**: internal/local-only tool; no new external data egress. Waste-finding text must not leak prompt/tool-result content beyond what already appears in the existing session detail view.
- **Data residency**: not applicable — all data is local JSONL already on the operator's machine.

## Scope

### In Scope

1. **Waste-pattern findings panel** — a new backend-computed set of findings (adapted from the Netflix skill's and TokenUsage's heuristics: cache-hit-rate floor breach, oversized/inferable-content CLAUDE.md (oversized start context), kitchen-sink session token ceiling, mid-session model-switch cache-bust), each severity- and dollar-impact-tagged, rendered as a ranked panel on the dashboard — verdicts, not just raw numbers. Follow-up (deferred, not in this pass): redundant/large-file-read and tool-failure-rate detectors need parser capture surface (`tool_result`/`is_error` blocks, per-call file-path arguments) that today's JSONL parser doesn't record — see `implementation/plan.md`'s Pattern Decisions table and Unresolved Questions.
2. **Per-tool/activity cost breakdown** — extend `TopToolEntry` (or an equivalent new aggregate) to carry cost, not just call count; add activity-type classification (Feature Dev / Debugging / Refactoring / etc., à la TokenUsage) so spend can be sliced by kind of work, not just model/project.
3. **Richer sort/search on the sessions table** — extend `SessionsTable` sort/search beyond path+model to duration, cost-per-message, cache ROI (signed $ saved vs. an all-fresh-input counterfactual — the `cacheeconomics` "saving_vs_uncached" idea, which can go negative), and a computed waste score.
4. **Deep-linkable session drill-down route** — promote session detail from the existing modal to a real `/insights/session/[sessionId]` route (bookmarkable/shareable), keeping the modal as an optional quick-peek if that's cheap, or replacing it outright if not.
5. **Fix `WatchInsights` dead per-session live-update path** — Phase 2 architecture research (`research/architecture.md`) confirmed this is worse than the original code survey's "aggregate lag" framing: `InsightsEvent.Session` is never populated server-side (`session/tokens/store.go:226,239`'s `Subscribe()` channel carries no payload), so the frontend's per-session live-patch branch (`useInsightsService.ts:109`) is dead code today — live updates do nothing until the next full `parse_complete`. Thread the changed `*ParseResult` through the subscribe channel so per-session updates actually patch the UI live. Pulled into scope per explicit user direction (2026-09-03) rather than deferred — this also removes the risk of the new findings panel/route shipping on top of a live-update path that silently does nothing.

### Out of Scope

- Rebuilding JSONL parsing, the pricing table, or the dashboard shell (carried over from `token-cost-tracking/requirements.md` — already shipped).
- CSV/JSON export of dashboard data (deferred in `insights/requirements.md`; still deferred here).
- Multi-user/team cost splitting or spend aggregation across operators.
- Live Anthropic billing reconciliation against JSONL-derived costs.
- Automated alerting/oncall integration for findings — this is a dashboard a human reads, not a paging system.
- Budget threshold alerts beyond what's already shipped (`ProjectedCostCard`) — no new budget-limit feature in this pass.
- A general-purpose rules/config UI for tuning waste-detector thresholds — thresholds are hardcoded constants for this pass (à la the reference tools), not user-configurable.

## Rabbit Holes

- **Per-tool cost attribution is not free**: `TopToolEntry` currently has no cost field because token usage in the JSONL is per-turn, not per-tool-call within a turn — a turn with 3 tool calls doesn't cleanly apportion its output tokens across them. Research must resolve whether to (a) attribute a turn's full cost to its tool set (double-counts on multi-tool turns), (b) split evenly, or (c) only attribute cost at the tool-*type* level across a session (sum of turn-costs where that tool appears), not per-call. Pick one, document the caveat in the UI (à la cacheeconomics' "abstain rather than guess" precedent) rather than silently producing a misleading number.
- **Activity-type classification is a heuristic, not ground truth** — misclassifying a session as "Debugging" when it was really exploratory research could mislead the cost-by-activity view. Needs a documented, testable classification rule (e.g. tool-call pattern signatures), not a vague LLM-guess-at-runtime approach (no LLM calls from the Go backend for this).
- **Waste-score formula composability** — the reference tools warn findings/scores are often non-additive (overlapping byte surfaces, overlapping fixes). If a single "waste score" column is added to the sessions table, it must be clearly one heuristic number with a defined formula, not an implied sum of all findings' dollar impacts.
- **Route vs. modal migration** — moving drill-down to a real route touches existing `SessionDetailDrawer` state management (`InsightsDashboard.tsx`'s `selectedSession` state, escape-key handling, WatchInsights live-patch behavior). The `WatchInsights` live-update fix is now explicitly in scope (see Scope item 5) — keep the two changes decoupled in planning (separate stories/tasks) so a regression in one doesn't block the other from shipping.
- **`ListSessionTokens` is already implemented server-side (sort+pagination) but unused by the frontend** — richer sort/search should probably finally wire this up rather than extending the client-side full-scan sort in `SessionsTable.tsx`, but switching to server-side pagination changes virtualization/search UX (Fuse.js text search currently runs over the full in-memory set) — resolve in planning, don't discover it mid-implementation.

## Alternatives Considered

- **Client-side-only findings computation** (no backend changes): rejected — findings need cost/token data already aggregated server-side (`session/tokens`), and computing them per-page-load client-side would duplicate backend logic and risk drifting from the authoritative pricing/token source.
- **LLM-generated findings/summaries**: rejected for this pass — every reference tool (including the Netflix skill) uses hardcoded heuristics/thresholds, not LLM judgment, for cost findings; adding an LLM call here would add cost, latency, and non-determinism to a cost dashboard, which is a bad look.
- **Configurable/user-tunable thresholds UI**: considered, deferred — adds real scope (a settings surface, persistence) for a single-operator tool where hardcoded constants (as all four reference tools use) are sufficient for v1.

## Feasibility Risks

- Per-tool cost attribution formula (see Rabbit Holes) could turn into a design debate that stalls implementation if not resolved crisply in planning.
- `WatchInsights`'s dead per-session live-update path (now in scope, item 5) plus its known aggregate-lag behavior (live updates don't recompute daily/model aggregates — that recompute is NOT in scope, only making the per-session payload actually flow) — keep the fix narrowly scoped to "populate and consume `InsightsEvent.Session`," not a broader rewrite of aggregate recomputation.
- No existing test coverage pattern for "heuristic produces the right verdict" — will need a fixture-based test approach (synthetic `SessionTokenSummary` data with known expected findings), which doesn't exist yet in this codebase and will need to be established.

## Observability Requirements

Standard structured logging (existing `slog` JSON-lines pattern) for the new findings-computation and per-tool-cost RPCs is sufficient — no new metrics/alerting needed. This is a single-operator local tool with no oncall; a computation error should show up as an empty/error state in the findings panel, not page anyone.

## Risk Control

Not needed as a feature flag — this is additive UI (new panel, new route, new table columns) behind the existing `/insights` route with no destructive change to existing behavior. Standard PR review + `make quick-check`/`make ci` gate before merge; rollback is a normal `git revert` if something regresses. No staged rollout mechanism exists or is warranted for a single-operator tool.

## Open Questions

- Exact set and thresholds for the waste-pattern detectors in scope for v1 (all six-ish heuristics researched, or a smaller starter set?) — resolve in Phase 2 research/Phase 3 planning.
- Per-tool cost attribution formula choice (even split vs. full-turn attribution vs. tool-type-level session sum) — resolve in Phase 2 research.
- ~~Whether the session drill-down route migration also fixes the `WatchInsights` aggregate-lag bug, or explicitly defers it~~ — **resolved 2026-09-03**: fixing the dead per-session live-update path is in scope (item 5), decoupled from the route migration as a separate story; aggregate recompute-on-update is explicitly NOT included.
- Whether `ListSessionTokens` server-side sort/pagination replaces the current client-side Fuse.js search entirely, or the two need to coexist (server-side sort + client-side text search on the current page) — resolve in Phase 3 planning.
