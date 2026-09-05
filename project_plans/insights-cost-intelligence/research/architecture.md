# Architecture Research — insights-cost-intelligence

Builds on `project_plans/insights/research/architecture.md` (frontend routing/filter/virtualization
decisions — R1-R7), `project_plans/insights-cost-pricing-gaps/research/architecture.md` (pricing
unpriced-family signal, now shipped — `ModelBreakdown.pricing_unavailable`,
`SessionTokenSummary.unpriced_models`, `GetInsightsSummaryResponse.unpriced_models` all exist),
and `project_plans/token-cost-tracking/research/architecture.md` (per-turn timeline RPC, now
shipped as `GetSessionTurnTimeline`). This doc only covers the four gaps requirements.md's Phase 2
scoped to; it does not re-derive anything those three already settled.

**Confirmed already shipped since those docs were written** (re-verified against current source,
not re-trusted from the docs): `GetSessionTurnTimeline` RPC (`insights.proto:21-24`, `TurnTokenStat`
message `:151-160`), `ModelBreakdown.pricing_unavailable` (`:82`), `SessionTokenSummary.unpriced_models`
(`:46`), `GetInsightsSummaryResponse.unpriced_models` (`:118`). `ListSessionTokens` sort (`cost`,
`tokens`, `date`) is implemented (`insights_service.go:437-464`) but still has zero frontend callers
(`grep -rn ListSessionTokens web-app/src` → only a feature-registry rpcId string,
`web-app/src/lib/features/features/misc-session.ts:28`).

## 1. Where waste-pattern findings computation should live, and how exposed

**New package: `session/tokens/findings.go`** (or a `session/tokens/findings/` sub-package if the
detector set grows past ~6 functions), not inside `insights_service.go`. Rationale:

- `insights_service.go` (732 lines) is already an RPC-handler file whose functions
  (`GetInsightsSummary`, `ListSessionTokens`, `WatchInsights`, `GetSessionTurnTimeline`) each do
  aggregation-then-serialize. Waste-finding detectors are pure functions over `*tokens.ParseResult`
  (cache-hit floor, CLAUDE.md size, redundant-file-read, session token ceiling, model-switch
  cache-bust, tool failure-rate) with no proto/connect dependency — they belong next to
  `ParseResult`/`ToolTokenStats`/`PricingTable` in `session/tokens`, the same placement precedent
  `pricing.go` already sets (pricing logic lives in `tokens`, not in the service layer).
- Keeps `session/tokens` as the single place that knows how to interpret a `ParseResult`; the
  service layer stays a thin proto-marshaling shim, consistent with how `EstimateCost`/
  `ModelFamilyCost` already work (computed in `tokens`, called from `insights_service.go`).
- Testability: a fixture-based test approach (flagged in requirements.md's Feasibility Risks as
  not yet established) is far easier against synthetic `*tokens.ParseResult` values in
  `session/tokens/findings_test.go` (same pattern as `pricing_test.go`) than against a full
  `InsightsService` + connect harness.

**Proto exposure: new fields on `GetInsightsSummaryResponse` and `SessionTokenSummary`, not a new
RPC.** Findings are a derived view of data `GetInsightsSummary` already assembles per-request (it
already loops every `ParseResult` once via `s.store.GetAll()`, `insights_service.go:69-`); a
separate RPC would force either a second full scan of `s.store.GetAll()` or plumbing the same
per-session intermediate values (cache hit rate, cost, tool-failure counts) out of `GetInsightsSummary`
into a second handler, duplicating work the aggregation loop already does in one pass. Concretely:

- `message WasteFinding { string finding_type = 1; string severity = 2; /* "info"|"warn"|"critical" */
  double dollar_impact_usd = 3; string session_id = 4; string conversation_id = 5; string message = 6; }`
  — new message, next free top-level slot in `insights.proto`.
- `repeated WasteFinding findings = 14;` on `GetInsightsSummaryResponse` (next free field number
  after `unpriced_models = 13`) for the dashboard-level ranked panel.
- Optionally `repeated WasteFinding findings = 18;` on `SessionTokenSummary` (next free after
  `unpriced_models = 17`) only if a per-session findings sub-list is needed in the drill-down view;
  the dashboard-level ranked panel (Scope item 1) only needs the response-level list scoped to
  each session via `session_id`/`conversation_id`, so start without the per-session field and add
  it only if planning finds a concrete UI need — avoids a field that's populated but unconsumed.
- Compute inside the existing `GetInsightsSummary` loop (`insights_service.go:~184-293`, same loop
  that already builds `modelMap`/`dailyMap`/`toolMap` per `r := range results`): call
  `tokens.ComputeFindings(r, costUSD, cacheHitRate, ...)` (or similar, taking already-computed
  per-session values as params rather than recomputing cost/cache-hit-rate a second time) once per
  session, append to a `[]*sessionv1.WasteFinding` slice, sort by `dollar_impact_usd` desc at the
  end, cap at a fixed N (mirrors `buildTopEntries`'s `limit` pattern, e.g. top 20) before returning.
  This keeps the NFR's "no full re-scan per request" constraint satisfied — findings ride the
  existing per-request scan, no new one.
- `ListSessionTokens` does **not** need findings — it is the raw sortable table, not the ranked
  panel; keep findings scoped to `GetInsightsSummary`.

This satisfies the NFR ("compute incrementally/cached where the TokenStore design already allows
it, not a full re-scan per request") since `TokenStore.GetAll()` is already a cheap in-memory read
(no I/O — `store.go`'s cache is filled by the fsnotify-driven `walkAndEnqueue`/single-file parse
path, `store.go:200-225`) and the findings loop rides the summary's existing single pass.

## 2. Per-tool cost attribution given turn-level granularity

Confirmed by direct read (not just requirements.md's claim): `session/tokens/types.go:28-37`'s
`TurnStats` carries `Input`/`Output`/`CacheCreation`/`CacheRead` at the **turn** level plus
`ToolNames []string` (which tools fired in that turn, no per-tool split), and `:39-46`'s
`ToolTokenStats` is purely a `{ToolName, CallCount, MCPServer}` counter with **no token/cost
field at all** — `insights_service.go:290-292`'s tool-aggregation loop (`for toolName, stat :=
range r.ToolUsage { toolMap[toolName] += stat.CallCount }`) only ever sums call counts, and
`sessionTopTools()` (`insights_service.go:~636-666`) builds `TopToolEntry{ToolName, CallCount,
McpServer}` with the same limitation. So a turn with 3 tool calls has one shared
input/output/cache figure with no way to know which of the 3 tools "caused" how much of it —
requirements.md's Rabbit Hole is accurate, not overstated.

**Recommendation: option (c) — tool-*type*-level session sum, not (a) full-turn-attribution or
(b) even-split.**

- **(a) full attribution to every tool in the turn** double-counts turn cost N times for an
  N-tool turn — the sum of `TopToolEntry.CostUsd` across tools in a session would exceed the
  session's real `EstimatedCostUsd`, which is actively misleading for a *cost* dashboard (the
  exact failure mode requirements.md wants avoided).
- **(b) even split across a turn's N tools** produces a number with no operational meaning (a
  turn with a cheap `Read` and an expensive multi-file `Grep` gets both attributed the same
  per-tool share) — precise-looking but not more true than (c), just costlier to compute and
  harder to caveat honestly in the UI.
- **(c) tool-type sum of parent-turn cost**: for each turn, compute `turnCost =
  EstimateTurnCost(turn.Input, turn.Output, turn.CacheCreation, turn.CacheRead, turn.Model)`
  (a new small helper mirroring `EstimateCost`'s per-turn arithmetic, since `EstimateCost` today
  only operates on session totals, not `TurnStats` — confirmed no existing per-turn cost function
  in `pricing.go`), then for every distinct tool name in `turn.ToolNames`, add `turnCost` once to
  that tool's running total **for that turn only** (i.e. a turn with 2 different tools adds
  `turnCost` to each of the 2 tool totals — deliberately double-counted, same principle as (a) but
  scoped to distinct-tool-per-turn rather than every call) — no, on reflection this is still (a)'s
  problem at the type level. The requirements.md framing of (c) is specifically "sum of turn-costs
  where that tool appears" — i.e. exactly this: a session-level `map[toolName]float64` where each
  turn's cost is added once per distinct tool name that appeared in that turn. This **does**
  double-count across tools within a multi-tool turn, but not across turns, and — critically — the
  UI caveat is simple and honest to state once: *"cost is attributed at turn granularity; a turn
  with multiple tools counts its full cost toward each tool it used, so per-tool costs across a
  session can sum to more than the session total."* This is the same "abstain rather than guess"
  precedent requirements.md cites from `cacheeconomics` — state the caveat, don't hide it, and it
  is architecturally the cleanest of the three because it requires only one new small per-turn cost
  helper plus a `map[string]float64` accumulator next to the existing `ToolUsage` counter loop —
  no schema change to `ParseResult`/`TurnStats`, no new per-tool-call data source that doesn't
  exist in the JSONL to begin with (Claude Code transcripts don't record output tokens per
  `tool_use` block, only per assistant message/turn — confirmed by `TurnStats`'s shape, this is a
  transcript-format ceiling, not a stapler-squad parsing gap).

**Concretely**: add `CostUsd float64` to `ToolTokenStats` (`session/tokens/types.go:41-46`),
populate it in the parser or in a new `tokens.AttributeToolCosts(r *ParseResult, pt *PricingTable)
map[string]float64` helper called from `insights_service.go` alongside the existing
`s.pricing.EstimateCost(r)` call; add `double cost_usd = 4;` to `TopToolEntry`
(`insights.proto:50-54`, next free field number) and a `bool cost_may_double_count = 5;` sibling
field (always `true` when the tool ever co-occurred with another tool in the same turn, `false`
otherwise) so the frontend can render an inline caveat marker only where it actually applies rather
than a blanket disclaimer on every row.

## 3. `ListSessionTokens` new sort keys — cheap extension vs. precompute/cache

**Duration and cost-per-message: cheap, extend in place.** Both are one-line derivations from
fields already on each `SessionTokenSummary` being sorted (`LastMessageAt - FirstMessageAt` for
duration; `EstimatedCostUsd / MessageCount` for cost-per-message) — no new storage, just two more
`case` branches in the existing `switch sortBy` at `insights_service.go:444-459`, computed inline
in the `sort.Slice` comparator exactly like `"tokens"` already is (`ti := ... + ...`,
`insights_service.go:447-450`). No caching needed: `ListSessionTokens` already recomputes
`EstimateCost`/`computeCacheHitRate` per session per request (`:392,396`) with no memoization, and
duration/cost-per-message are strictly cheaper (no pricing-table lookup) than what's already
running unconditionally every call.

**Cache ROI and waste score: also cheap to add as sort keys without precomputation, but only if
computed from data already resident in the per-session loop — not from anything requiring a
second pass over `ParseResult` or cross-session context.** Cache ROI (`cacheeconomics`'s
"saving_vs_uncached" idea — $ saved by cache reads vs. an all-fresh-input counterfactual) is
`(cache_read_tokens * (pricing.InputPricePerMTok - pricing.CacheReadPerMTok) / 1e6) -
(cache_creation_tokens * pricing.CacheWritePerMTok / 1e6)` — every input is already available at
the point `costUSD, unpriced := s.pricing.EstimateCost(r)` is called (`:392`); it's a second
small arithmetic expression over the same `r.CacheRead`/`r.CacheCreation`/pricing lookup, not a
new data source. Waste score, per requirements.md's own constraint ("one heuristic number with a
defined formula, not an implied sum"), should be defined as a single documented formula (e.g. a
weighted combination of low-cache-hit-rate + high-tool-failure-rate + oversized-session, clamped
0-100) computed by the same `tokens.ComputeFindings`-adjacent helper from §1 — reuse, don't
duplicate the findings math in a second "waste score" function.

**The one thing that *would* force precompute/caching**: if `ListSessionTokens` needs to *sort
by* cache ROI or waste score across the **full unfiltered session set** while also paginating,
the current architecture already handles this correctly since sorting happens in Go before
pagination (`:436-464` sorts `summaries` in full, `:474-488` slices after) — this is unaffected
by adding two more `case` branches. No precompute/cache layer is needed for v1's ~600-session
scale (NFR explicitly scopes to this volume); revisit only if session count grows an order of
magnitude and per-request O(n log n) sort-with-inline-arithmetic becomes measurably slow (not
indicated by anything in the NFRs).

**Conclusion: extend the existing `switch sortBy` with 4 more cases (`"duration"`,
`"cost_per_message"`, `"cache_roi"`, `"waste_score"`), no precompute/cache layer, no proto
change beyond whatever new fields the panel itself needs for display** (cache ROI and waste score
should probably also be *displayed*, not just sortable — see §1's `WasteFinding.dollar_impact_usd`
for waste, and a new `double cache_roi_usd = 19;` on `SessionTokenSummary` for cache ROI, next free
field number after the `findings` field from §1 if added, else after `unpriced_models = 17`).

## 4. `/insights/session/[sessionId]` route integration point

**Reuses `GetSessionTurnTimeline` — no new RPC needed.** The prior insights architecture doc (R2,
`project_plans/insights/research/architecture.md:32-49`) chose a slide-over drawer over a route
specifically because "no additional RPC is needed — `SessionTokenSummary` has all detail-view
data" and cited the existing in-memory `summary.sessions` array. That premise still holds for a
route: `SessionDetailDrawer.tsx` (current implementation, confirmed via
`web-app/src/lib/hooks/useInsightsService.ts:184-229`'s `useSessionTurnTimeline` hook) already
does exactly what a route's data-loading would need — takes a `conversationId`, calls
`GetSessionTurnTimeline` for the per-turn table, and otherwise reads the already-fetched
`SessionTokenSummary` from the parent's `summary.sessions` array (passed as a prop, not
re-fetched). A route at `/insights/session/[sessionId]/page.tsx` has the same two data needs:

1. **The session's `SessionTokenSummary`** — on a hard navigation/deep-link (no parent
   `InsightsDashboard` state to read from), the route needs its own fetch. Reuse
   `GetInsightsSummaryRequest.session_id_filter` (`insights.proto:99`, already exists,
   confirmed unused by any current frontend caller — `grep -rn sessionIdFilter web-app/src`
   → only the hook's pass-through in `useInsightsService.ts:66`, no caller ever sets it) —
   call `GetInsightsSummary({ sessionIdFilter: sessionId, includeOrphans: true })` from the new
   route to get a 1-session-scoped response instead of adding a new single-session RPC.
2. **The per-turn timeline** — call `GetSessionTurnTimeline({conversationId})` exactly as the
   drawer does today, via the same `useSessionTurnTimeline` hook (no change needed to the hook
   itself — it already takes a bare `conversationId` string, not drawer-specific props).

So the route needs **zero new backend/proto work**: it's a new Next.js page that composes two
already-existing RPCs (`GetInsightsSummary` with `session_id_filter` set, and
`GetSessionTurnTimeline`), reusing `SessionDetailDrawer`'s internal rendering (either by
extracting its content into a shared component the route also renders, or by keeping the drawer
as a thin wrapper that a route-based page also mounts) rather than duplicating the drawer's JSX.
This matches requirements.md's own framing in scope item 4 ("keeping the modal as an optional
quick-peek if that's cheap") — cheap here means: drawer keeps using the in-memory
`summary.sessions` lookup (fast path, already fetched), route uses the `session_id_filter` fetch
(slow path, only hit on direct navigation/refresh/deep-link) — both end up rendering the same
inner component.

## 5. `WatchInsights` aggregate-lag bug — does it block or complicate findings freshness

**It's worse than requirements.md's Feasibility Risk describes, and yes, it complicates findings
freshness — findings need this fixed (or worked around) to be trustworthy on live updates.**
Direct read of `session/tokens/store.go` and `server/services/insights_service.go` shows:

- `TokenStore.Subscribe()` returns `<-chan struct{}` (`store.go:49,131-137`) — a bare signal
  channel with **no payload**. `notify()` (`store.go:152-160`) sends `struct{}{}` to every
  subscriber; it has no way to say *which* file/session changed.
- `watchInsights` (`insights_service.go:527-563`) only ever sends two event shapes: an initial
  `"parse_complete"`/`"loading"` event with no session, and on every subsequent `<-ch` receive,
  `&sessionv1.InsightsEvent{EventType: "update", AllParsed: ...}` — **`Session` is never set**
  (confirmed by grepping the whole file for `.Session =` — zero matches touching `InsightsEvent`).
- The frontend (`useInsightsService.ts:107-138`) has a branch for `event.eventType === "update"
  && event.session` that does the surgical per-session patch the prior insights architecture doc
  (R6) designed — but since the server never populates `event.session`, **that branch is dead
  code in production today**: every real `"update"` event fails the `&& event.session` guard and
  falls through with no effect at all (not even a full refetch — only `"parse_complete"` triggers
  `fetchSummary()`). So the actual current behavior on a new/changed JSONL file mid-session is:
  *no UI update whatsoever* until the next `"parse_complete"` event (sent once at watch-subscribe
  time and, per `walkAndEnqueue`'s defer at `store.go:236-239`, once at the end of a directory
  walk) — worse than "aggregates lag," it's "nothing refreshes between walks."

**This blocks findings freshness directly**: a waste finding computed once at page load (or at
the last `parse_complete`) will silently go stale as new turns/sessions arrive, with no live
signal to recompute it — the dashboard would show a findings panel that looks live (it's on the
same page as the live-updating cost totals) but is not, which is a worse UX gap than a plainly
static page would be.

**Recommended fix, scoped minimally** (do not rebuild `WatchInsights` merge logic wholesale, per
requirements.md's explicit guardrail): change `TokenStore.Subscribe()`'s channel type from `<-chan
struct{}` to `<-chan *tokens.ParseResult` (the just-updated result), update `notify()`'s two call
sites (`store.go:226` after a single-file parse, `:239` after a full walk) to pass the relevant
`*ParseResult` (the single-file path already has `result` in scope at `:226`; the walk-complete
path at `:239` has no single result — send `nil` there to mean "recompute everything," preserving
today's `"parse_complete"`-triggers-full-refetch behavior for that case only). Then in
`watchInsights` (`insights_service.go:546-562`), on receiving a non-nil result, build the same
`SessionTokenSummary` the existing `ListSessionTokens`/`GetInsightsSummary` loops build for one
session (extract a shared `buildSessionSummary(r, pricing, associator) *sessionv1.SessionTokenSummary`
helper reused by all three RPCs — currently `GetInsightsSummary`'s per-session block and
`ListSessionTokens`'s per-session block, `insights_service.go:~184-230` and `:392-428`, are two
independent near-duplicates of the same construction, itself worth consolidating regardless of this
fix) and set it on `evt.Session`, finally making the frontend's already-written surgical-patch
branch actually fire. This closes the per-session gap but **does not** fix daily/model-aggregate
recompute on every update (`daily[]`/`models[]` still only refresh on `parse_complete`) — that
half of the bug is explicitly out of scope per requirements.md's Rabbit Holes guardrail unless
Phase 3 planning pulls it in; findings that depend only on *per-session* fields (cache-hit floor,
tool failure-rate, oversized-session — all computable from one `SessionTokenSummary`) become live
once `evt.Session` is populated, while any *aggregate-level* finding (e.g. "average cache hit rate
across all sessions this week dropped") would still lag until the next `parse_complete` — worth
flagging to Phase 3 as a reason to keep v1's waste-detector set (Scope item 1) scoped to per-session
heuristics only, deferring cross-session/aggregate findings to a later pass.

## Summary of architectural decisions for Phase 3 (plan) to consume

| Question | Decision |
|---|---|
| 1. Findings computation location | New `session/tokens/findings.go` (pure functions over `*ParseResult`); new `WasteFinding` proto message; `repeated WasteFinding findings` field on `GetInsightsSummaryResponse` (not a new RPC), computed inside `GetInsightsSummary`'s existing per-session loop |
| 2. Per-tool cost attribution | Option (c): session-level `map[toolName]cost` where each turn's cost is added once per distinct tool name in that turn (double-counts within multi-tool turns, not across turns); new `TopToolEntry.cost_usd` + `cost_may_double_count` proto fields; new small per-turn cost helper in `pricing.go`; `ToolTokenStats.CostUsd` field |
| 3. New sort keys | Cheap in-place extension of `ListSessionTokens`'s existing `switch sortBy` — 4 more cases, no precompute/cache layer needed at current (~600-session) scale |
| 4. Route integration | Zero new RPCs — compose existing `GetInsightsSummary{session_id_filter}` (already exists, currently unused) + existing `GetSessionTurnTimeline`; share rendering with `SessionDetailDrawer` rather than duplicating |
| 5. WatchInsights lag vs. findings freshness | Confirmed worse than documented: `InsightsEvent.Session` is never populated today, so the frontend's per-session live-patch code is dead and nothing refreshes between `parse_complete` events. Minimal fix: thread the changed `*ParseResult` through `TokenStore`'s subscribe channel and populate `evt.Session`. This unblocks *per-session* finding freshness; cross-session aggregate findings still lag and should be scoped out of v1's detector set for that reason |
