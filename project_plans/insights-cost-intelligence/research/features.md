# Research: FEATURES — insights-cost-intelligence

Scope: prior art for the four in-scope workstreams, plus edge cases/failure
modes and unstated user needs. Grounded in the existing code (proto, hooks,
store types) rather than speculation — file:line citations throughout.

## 1. Prior art (already named in requirements.md, verified relevant)

- **Netflix internal `claude-code-cost-optimizer` skill**, **`Tanisha-Katara/cacheeconomics`**,
  **`happy-token/TokenUsage`**, **`lucemia/claude-session-analyzer`** — all converge on
  severity-ranked, dollar-tagged findings over raw charts (requirements.md:9). Two
  specific mechanics worth carrying into design, already flagged there:
  - `cacheeconomics`'s "saving_vs_uncached" signed counterfactual for cache ROI
    (requirements.md:50) — this repo has the raw ingredients (`cache_read_tokens`,
    `cache_creation_tokens`, per-model pricing in `session/tokens/pricing.go`) but no
    existing counterfactual computation; it's new work, not a rewire.
  - The "abstain rather than guess" precedent for ambiguous attribution
    (requirements.md:65) — directly applicable to per-tool cost (see §3 below): when
    a number would be misleading, omit or caveat it in the UI rather than render a
    plausible-looking wrong number.
- **In-repo precedent for a "not enough data yet" abstention pattern** already
  exists and should be reused verbatim for findings, not reinvented: `useProjectedCost`
  returns `null` when `daysData < 7`
  (`web-app/src/lib/hooks/useProjectedCost.ts:31`), and `InsightsDashboard.tsx`
  renders an explicit fallback message instead of a card when that happens
  (`web-app/src/app/insights/InsightsDashboard.tsx:183-187`: "Projected monthly cost
  needs at least 7 days of usage data..."). This is the template to copy for the
  findings panel's sparse-data case — same `null`-return + explicit-message
  contract, not a spinner or an empty-looking panel.
- **Sort-with-missing-values precedent**: `SessionsTable`'s existing cost sort
  already handles "column value effectively missing" by defining unpriced as a
  distinct always-last bucket, checked before the ascending/descending flip
  (`web-app/src/app/insights/SessionsTable.tsx:113-120`). This is the pattern new
  sort columns (duration, cost-per-message, cache ROI, waste score) must extend,
  not a novel design decision.

## 2. Waste-pattern findings panel — sparse-data edge cases

**Does the "not enough data" pattern need to extend to findings, not just cost projection?**
Yes, and for a different reason than the projection card: the projection card's `<7 days`
gate exists because a monthly *extrapolation* is statistically meaningless from a short
window. Findings have a **per-heuristic** version of this problem, not one global gate:

- **Cache-hit-rate floor breach**: `cache_hit_rate` is `cache_read / (input + cache_read)`
  (`proto/session/v1/insights.proto:41`). A session with 1-2 turns has this ratio computed
  over almost no denominator — the first turn of *any* session structurally has zero cache
  reads (nothing written yet to read back), so a <5-turn session's cache-hit-rate is
  dominated by cold-start noise, not a real signal of waste. A hardcoded floor (e.g. "flag
  if <40%") will false-positive on every short session. Needs a minimum-turn-count guard
  per finding, most acutely on this one.
- **Kitchen-sink session token ceiling**: inherently a large-N-turn detector; a <5-turn
  session cannot trigger this one by construction, no guard needed, but worth stating
  explicitly in the design so it's not "silently never fires" mistaken for a bug.
- **Mid-session model-switch cache-bust**: needs ≥2 turns with different models to even be
  evaluable — a 1-turn session can't be classified either way; must skip (not "score 0"),
  since 0 waste and "not applicable" are different verdicts a user would read differently.
- **Tool failure-rate**: needs at least one tool call to have a denominator; a session with
  zero tool calls must be excluded from this finding's population, not scored as 0%
  failure (0% failure across 0 calls reads as "healthy," which overstates confidence).
- **Brand-new project (<7 days history)**: this is a *project-level* time-window sparsity,
  distinct from per-session turn-count sparsity above. Findings that aggregate across a
  project's session history (vs. a single-session finding) inherit the same reasoning as
  `useProjectedCost`'s 7-day gate — e.g. any finding that's actually a trend ("spend is
  climbing week over week") needs the same explicit `null`/abstain state, while a
  single-session finding (this one specific session breached the cache-hit floor) doesn't
  care about project age at all. The design should split findings into
  **session-scoped** (evaluate independently of history length) vs. **history-scoped**
  (needs the 7-day-style guard) rather than applying one blanket data-sufficiency gate to
  all six heuristics.

**Failure mode to design for explicitly**: a findings computation error for one heuristic
must not blank the whole panel. Per requirements.md:85 ("a computation error should show
up as an empty/error state in the findings panel, not page anyone"), the natural
implementation is a `[]Finding` result where the backend catches/skips a panicking or
erroring detector per-session rather than failing the whole `GetInsightsSummary` call —
worth stating as an explicit isolation requirement in the plan doc, since Go's absence of
built-in per-detector isolation (no try/catch) makes it easy to accidentally let one bad
heuristic (e.g. divide-by-zero on a 0-turn orphan) 500 the entire endpoint.

## 3. Per-tool cost breakdown — turn/tool cardinality edge cases

Verified structurally in `session/tokens/types.go:29-46`: token counts (`Input`,
`Output`, `CacheCreation`, `CacheRead`) are **per-turn** (`TurnStats`), while
`ToolNames []string` is a **list on that same turn** — i.e. exactly the ambiguity
requirements.md:65 names. `ToolTokenStats` (types.go:41-46) already documents this:
"Token attribution is message-level (not per-tool-call); CallCount is exact." Confirmed
in the parser: `turn.ToolNames = append(turn.ToolNames, c.Name)` (`session/tokens/parser.go:175`)
— every tool_use block in an assistant message appends to the same turn's `ToolNames`,
with no per-call token split anywhere in the pipeline.

Concrete cardinality cases from this structure:
- **Zero tools in a turn**: turn's tokens are pure conversation (thinking/response text),
  correctly excluded from every tool's attribution — no ambiguity here.
- **Exactly one tool**: turn's full cost unambiguously belongs to that one tool — the
  only case where "attribute full turn cost" and "split evenly" produce the same number.
  This is also the common case in practice (most turns call 0 or 1 tools), which matters
  for how misleading the wrong choice would look in aggregate.
- **Many tools in one turn** (parallel tool calls, which Claude Code does routinely for
  independent reads): this is where option (a) "attribute full turn cost to every tool
  in its set" silently multiplies double-counted cost across the session's `top_tools`
  total — a session with $10 of cost and three tools called together every turn would
  show ~$30 of "cost" summed across `TopToolEntry` rows, which is worse than just wrong,
  it's wrong in the direction users most need to trust (dollar figures). Given the
  Rabbit Hole's own framing (requirements.md:65-66), option (c) — tool-*type*-level
  attribution, summing whole-turn cost for every turn where that tool type appears,
  documented as "this session's turns involving X cost $Y total" rather than "$Y is X's
  share" — is the only one of the three that can't produce a number that's provably too
  high when summed. It still double-counts across *different* tool types in the same
  turn (by design, since the whole point is "no clean split exists"), so the UI caveat
  from cacheeconomics' abstain precedent is required regardless of which option is
  picked: label it "turn-cost when this tool was used," not "this tool's cost."
- **Unstated need**: users will intuitively try to sum the per-tool cost column and
  compare it to the session total — any attribution formula that lets that sum exceed
  the session total needs a visible caveat near the column header, not just a doc
  comment, or the dashboard's own numbers will look self-contradictory (which undermines
  the "verdicts I can trust" goal from requirements.md's Problem Statement).

## 4. Sort/search on sessions — missing-value edge cases

`SessionsTable.tsx`'s current cost sort (`:113-120`) is the model to extend:
```
const aUnpriced = a.unpricedModels.length > 0;
const bUnpriced = b.unpricedModels.length > 0;
if (aUnpriced !== bUnpriced) return aUnpriced ? 1 : -1;
```
This unconditionally pushes unpriced-model sessions to the *end* regardless of sort
direction (checked before the `sortAsc` flip). New columns need the equivalent guard,
but the "missing" condition differs per column and needs its own definition:

- **Duration**: computed from `firstMessageAt`/`lastMessageAt`
  (`SessionDetailDrawer.tsx:114` reads `firstMessageAt`; proto fields at
  `insights.proto:44-45`). Either timestamp can be zero/unset for a malformed or
  single-message session — "missing" here means duration is 0 or the timestamp is
  absent, and it's genuinely ambiguous whether that should sort as "worst" (unknown,
  push to end like unpriced) or "0" (legitimately smallest, sort first ascending) —
  unlike cost, a missing duration isn't obviously "bad," so this needs an explicit
  design decision, not a copy-paste of the unpriced-cost guard.
- **Cost-per-message**: divide-by-zero when `message_count` is 0 (orphaned/malformed
  session with token usage but no parsed messages) — must guard the division itself,
  not just apply the sort-last convention after computing `Infinity`/`NaN`, since `NaN`
  comparisons in a sort comparator silently produce non-deterministic ordering in JS.
- **Cache ROI** (signed $ saved vs. all-fresh-input counterfactual): undefined/NaN for
  any unpriced-model session (no price to compute the counterfactual against) — same
  "push to end" treatment as cost, but also needs a sign-aware default: unlike cost
  (where "missing" and "very high" are visually distinct), a *negative* ROI (cache made
  things worse) is a valid, important, non-missing value that must not be confused with
  the missing/unpriced bucket in the UI (e.g. don't reuse the same "unpriced" badge
  styling for "negative ROI").
- **Waste score**: a synthesized single number (requirements.md:67 warns explicitly it's
  "one heuristic number... not an implied sum") — its own missing/not-applicable case
  compounds every upstream heuristic's missing case (§2 above): a session too sparse for
  every waste heuristic to fire has no waste score at all, distinct from a session that
  was evaluated and scored 0. Sorting by waste score therefore needs a third bucket
  beyond "has value" / "missing due to no pricing" — "not evaluated due to insufficient
  turns" — or these two distinct kinds of absence will conflate in the UI (e.g. a
  brand-new 2-turn session sorting identically to an unpriced 200-turn session at
  "bottom of the waste-sorted list" reads as "nothing to see here" for very different,
  and differently actionable, reasons).
- **Server-side vs. client-side sort tension**: requirements.md:69 already flags that
  `ListSessionTokens` supports server-side `sort_by: "cost"|"tokens"|"date"`
  (`proto/session/v1/insights.proto:110-121`) but the frontend never calls it — all
  sorting today is the client-side `useMemo` in `SessionsTable.tsx`. Any new sort column
  (duration, cost-per-message, cache ROI, waste score) that isn't a raw stored proto
  field requires either (a) computing it server-side and adding it as a real
  `sort_by` value, or (b) keeping it client-side-only and never wiring `ListSessionTokens`
  for it — a column-by-column decision, not an all-or-nothing switch, since e.g. "cost"
  already has server support today and duration doesn't.

## 5. Route migration — WatchInsights merge-logic edge cases

Read `useInsightsService.ts` in full (`web-app/src/lib/hooks/useInsightsService.ts:107-152`).
Confirmed facts, directly answering the research question:

- **`WatchInsightsRequest` has no session-ID filter** — only `from`/`to`
  (`proto/session/v1/insights.proto:105-108`). There is no server-side concept of "watch
  just this one session"; a route-scoped subscription for `/insights/session/[sessionId]`
  would either (a) need a new proto field/RPC, or (b) reuse the existing full-list
  `WatchInsights` stream and filter client-side to the one session of interest — option
  (b) requires no backend change and matches the Rabbit Hole's instruction
  (requirements.md:68) to scope the route migration to navigation/URL state only.
- **The merge logic only knows how to patch the sessions list, confirming the flagged bug**:
  on `event_type === "update"`, it does a keyed upsert into `prev.sessions` by
  `conversationId || sessionId` (`useInsightsService.ts:122-131`) — it never touches
  `prev.daily`, `prev.models`, `prev.totalCostUsd`, etc. Those aggregate fields only get
  refreshed on `event_type === "parse_complete"`, which triggers a full `fetchSummary()`
  refetch (`useInsightsService.ts:134-138`). This is exactly the lag requirements.md:80
  describes: "live updates patch individual sessions but don't recompute daily/model
  aggregates." A single-session drill-down route reading from this same shared summary
  object would see its one session update live and correctly (the upsert works fine at
  session granularity) — the bug only bites aggregate-level UI (daily chart, model
  breakdown cards), which a session detail route mostly doesn't render. So: **the route
  migration itself does not require fixing the aggregate-lag bug** to work correctly for
  its own content, confirming requirements.md's scoping instinct — but if the new
  waste-score/findings panel is rendered *inside* the session detail route (plausible,
  since a per-session finding like "this session breached the cache floor" belongs on
  its own drill-down page), and that finding depends on any aggregate rather than
  this-session-only data, it inherits the lag. Whether that's in scope depends on
  whether any planned finding is aggregate-dependent — worth flagging explicitly in
  planning rather than discovering mid-implementation.
- **Time-filter interaction**: the merge callback also silently drops an incoming update
  whose `lastMessageAt` falls outside an active `from`/`to` filter
  (`useInsightsService.ts:113-121`, `return prev` unchanged). A deep-linked
  `/insights/session/[sessionId]` route reached directly (not via the table, e.g. a
  bookmark or shared link) may not carry the same `from`/`to` filter state the table had
  when the user clicked through — if the route re-derives its filters from URL params
  that don't include the original date range, a session whose `lastMessageAt` is outside
  today's *default* range could load once via the initial `fetchSummary()` (unfiltered
  request keyed on the route's own params) but then never receive live updates if the
  route's filter state doesn't match. This is a real edge case for "bookmarkable" per
  requirements.md:51 — the route needs to decide whether session-detail fetches are
  filter-independent (probably correct: a specific session, once linked, should ignore
  the dashboard's global date filter entirely) rather than inheriting `useInsightsSummary`'s
  filter-aware fetch as-is.
- **Escape-key/modal-to-route interaction**: `InsightsDashboard.tsx` currently manages
  `selectedSession` as local component state (`InsightsDashboard.tsx:109`) set via
  `onSessionClick` (`:247`) and rendered into `SessionDetailDrawer`
  (`:254-257`, keyed by `backlogIndex.get(selectedSession.sessionId)`). Promoting this to
  a route means either the route becomes the sole source of truth (URL param drives
  `selectedSession`, no local state) or a synced dual-state (state mirrors URL) — the
  existing drawer's escape-key handling (mentioned in requirements.md:68 as touched by
  this migration but not read in detail here) will need re-wiring to `router.back()` /
  URL clearing instead of a local `setSelectedSession(null)`, since closing a route-driven
  drawer must also update the URL or a back-button press and an escape-key press will
  leave the URL and the UI state out of sync.

## Unstated user needs beyond the explicit requirements

1. **Trust-preserving abstention, not just correctness.** The recurring theme across
   §2–§4 (sparse findings, tool-cost attribution, waste-score "not evaluated" state) is
   that Tyler's actual need is a dashboard whose numbers never look self-contradictory —
   he'll notice if per-tool costs summed exceed the session total, or if a 2-turn
   session's cache-hit-rate looks alarmingly bad for structural reasons unrelated to
   waste. The explicit requirements ask for verdicts; the unstated need is that a
   *wrong-looking* verdict (even if technically defensible) costs more trust than an
   admitted "not enough data" — matching the reference tools' own abstain precedent
   already cited in the doc.
2. **Distinguishing "not evaluated" from "evaluated and clean."** For findings and
   waste-score alike, a session with zero findings and a session too sparse to evaluate
   look identical today (both render nothing). Tyler will read "no findings" as "this
   session is fine," which is false for a 2-turn session that was never checked. The UI
   needs a visible distinct state for "insufficient data to evaluate," not just an
   absence of findings — otherwise the findings panel's core value prop (surfacing the
   driver fast, per requirements.md's Success Metric 1) actively misleads on exactly the
   sessions where evaluation was skipped.
3. **Bookmark stability across live data.** Since a core motivation for the route
   migration is "bookmarkable/shareable" (requirements.md:51), Tyler's real want is
   probably to paste a session link into a note/PR/Slack message and have it still
   resolve correctly weeks later — meaning the route should be keyed by the stable
   `conversation_id` (persists across re-parses) rather than anything that could change,
   and should degrade gracefully (a clear "session not found" state, not a crash) if the
   underlying JSONL is ever pruned/rotated.
