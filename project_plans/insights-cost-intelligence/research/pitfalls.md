# Research: Pitfalls — Cost/Waste Analytics + Dashboard Sort/Search

Grounding: `project_plans/insights-cost-intelligence/requirements.md`; codebase check of
`server/services/insights_service.go` (`ListSessionTokens` already implements server-side
`SortBy`/pagination, unused by frontend), `web-app/src/app/insights/SessionsTable.tsx`
(client-side `Fuse.js` full-scan search + `react-virtuoso` `TableVirtuoso`),
`web-app/src/app/insights/InsightsDashboard.tsx` (`WatchInsights` live-patch, modal
`selectedSession` state).

## 1. Waste-detector / anomaly-heuristic engines

**False-positive fatigue.** The single most-cited failure mode across cost-anomaly and
lint-style heuristic engines (SonarQube, Dependabot alerts, AWS Trusted Advisor, and the
`cacheeconomics`/`TokenUsage` reference tools named in requirements.md): if thresholds are
tuned for sensitivity, the panel fills with findings the user has already triaged and
dismissed mentally, and they stop reading it — the exact "findings panel becomes wallpaper"
failure. Standard mitigations, all applicable here given the "hardcoded thresholds, no
config UI" non-goal already set:
- **Fewer, higher-precision detectors over more, sensitive ones.** Requirements.md already
  lists ~6 heuristics as candidates; resist the urge to ship all of them just because
  they're documented in the reference tools — each one is a permanent line of "why is this
  flagged" support burden with no user-facing toggle to quiet it.
- **Severity floors, not just severity labels.** A finding below a minimum dollar-impact
  floor (e.g. cache-hit-rate breach that costs $0.03) should not render at all — Trusted
  Advisor and Dependabot both suppress below a materiality floor rather than rendering
  "Low" severity noise.
- **Stable identity per finding** so the same underlying condition doesn't re-surface as a
  "new" finding after a page refresh/live update — ties directly into the `WatchInsights`
  aggregate-lag rabbit hole: if findings are recomputed per snapshot without a stable key,
  a flapping value near a threshold will visibly appear/disappear on every live update,
  which reads as noisier than it is.

**Threshold staleness.** Hardcoded constants (already decided as v1's approach) reflect the
pricing/model landscape and the operator's own usage patterns at the moment they were
chosen. As Anthropic's own pricing changes (already happened multiple times per the prior
`token-cost-tracking` project's pricing-table work) or Tyler's usage habits shift (e.g.
moving from Sonnet to Haiku by default), a cache-hit-rate floor tuned for one regime silently
becomes miscalibrated for the next — either firing constantly (fatigue) or never firing
(false confidence). Mitigation used by the reference tools and generally: **document the
threshold's provenance and the pricing snapshot it assumes in a code comment next to the
constant**, so a future pricing-table change is force-multiplied into "go re-check these
thresholds too" rather than silently drifting. Given the explicit non-goal of a tunable
thresholds UI, this comment-level traceability is the only guard available for v1 — call it
out in the plan so it isn't dropped as "just a comment."

**Non-additive severity/dollar scores (the flagged rabbit hole — going deeper).**
`cacheeconomics` flags this same issue; the standard mitigation pattern across cost-anomaly
tools (AWS Cost Anomaly Detection, Datadog Cost Management, and `cacheeconomics` itself) is
**"abstain rather than guess" plus explicit non-summability in the data model, not just the
UI copy**:
1. Each finding's dollar-impact is a *counterfactual estimate against one specific
   baseline* (e.g. "if this session's cache-hit rate matched the fleet median, it would
   have cost $X less") — not a measured loss. Different findings on the same session use
   different baselines and often double-count the same root cause (e.g. a kitchen-sink
   session token-ceiling finding and a cache-hit-rate-floor finding on the *same* session
   are both downstream of the same oversized CLAUDE.md).
2. **Do not expose a "total potential savings" by summing finding dollar-impacts** — this is
   the single most common mistake reference tools warn against, because a reader will do
   the sum themselves the instant it's visually implied (e.g. by rendering all findings'
   `$` amounts in the same column with a visible total row). If an aggregate number is
   wanted, it must be computed independently as "session cost minus a single alternative
   scenario," never as `Σ finding.dollarImpact`.
3. Represent this in the type system, not just documentation: e.g. make `Finding.DollarImpact`
   a value type that is not `Add`-able across findings (no `Sum(findings []Finding) float64`
   helper exported at all — force any aggregation through a named, single-purpose function
   that computes it from raw data, so there's no accidental one-liner `sum += f.DollarImpact`
   in a future PR). This is exactly the `type-driven-design` skill's "illegal states
   unrepresentable" principle applied to a UX/analytics correctness problem, not just a
   type-safety one.
4. **Waste score is one heuristic number with a defined, documented formula** (already
   stated in requirements.md) — same treatment: give it its own named type/field distinct
   from "dollar impact," and never render it in a `$` context or let it participate in a sum
   with actual currency values.

## 2. Heuristic cost-attribution formulas vs. visual weight of measured vs. modeled numbers

**Standard failure mode**: a heuristic-derived number rendered with the same font size,
color, and column position as a directly-measured number gets read with equal trust — users
anchor on precision-looking numbers (e.g. "$4.37") as if they were metered, when they are
actually a modeling choice (e.g. even-split-per-tool-call attribution, explicitly flagged in
requirements.md as double-counting on multi-tool turns). This is the generalized version of
"significant digits imply precision" — showing `$4.37` instead of `~$4` or `$4.37±$1.20`
signals a confidence level the number doesn't have.

**How well-designed dashboards distinguish measured from modeled**, beyond
`cacheeconomics`'s evidence-class idea already noted:
- **AWS Cost Explorer** marks "Unblended" vs "Amortized" vs "Net Amortized" cost as
  explicitly different named columns/toggles rather than one column — the user picks which
  cost *model* they're viewing, and the UI never merges them into a single number.
- **Datadog Cost Management** and **CloudHealth** use a distinct visual treatment (lighter
  weight, an info-icon with a tooltip explaining the estimation method, often an "est."
  prefix or tilde) for any inferred/allocated cost versus a billed line item.
- **Grafana / Prometheus-based dashboards** commonly use a dashed line style or reduced
  opacity for "predicted"/"forecasted" series versus solid lines for "observed" — the same
  measured-vs-modeled distinction applied to time series rather than tables.
- Concretely for this project: per-tool cost (heuristic-attributed) should visually differ
  from total session cost (directly measured, sum of turn costs from the JSONL) — e.g. a
  `~` prefix, a distinct muted color token, and a tooltip stating the attribution method
  chosen (even-split / full-turn / tool-type-level, whichever Phase 2's attribution-formula
  research picks). The waste score and finding dollar-impacts get the same treatment. This
  is a `vanilla-extract` token-level decision (per ADR-009) — define a shared "estimated"
  visual style once (e.g. `styles.estimatedValue`) rather than ad hoc per-component styling,
  so it can't drift component-to-component.
- Do **not** rely on a legend or a one-time onboarding tooltip alone — evidence-class
  distinction needs to be visible at the point of reading the number (inline glyph/color),
  because users skim dashboards without re-reading legends each time.

## 3. Server-side sort/pagination vs. derived/computed columns (cache ROI, waste score)

Confirmed in code: `ListSessionTokens` (`server/services/insights_service.go:353`) already
takes a `SortBy` (line ~437) and paginates server-side; `SessionsTable.tsx` currently does a
full client-side scan with `Fuse.js` (line 71) and does not call `ListSessionTokens` at all —
this is exactly the rabbit hole requirements.md flags as unresolved.

**Standard failure mode when adding a derived sort key to an already-paginated endpoint**:
sorting by a column that isn't a stored/indexed field forces one of:
1. **Score the entire dataset before returning page N** — this defeats pagination's
   purpose (you must compute waste-score/cache-ROI for all ~600 sessions to know which ones
   sort into position 21–40), but at ~600 sessions and "tens of millions of tokens" (per
   requirements.md's stated current scale) this full-scan-then-slice is likely *fine*
   performance-wise — the risk is treating it as "true" pagination when it's actually
   "compute everything, paginate the response," which matters for the NFR's "no full
   re-scan per request" language. If `TokenStore` already holds per-session aggregates in
   memory (worth confirming in Phase 3 planning), this reduces to a cheap in-memory sort, not
   a re-parse of JSONL — the NFR is about not re-scanning raw JSONL per request, not about
   avoiding an in-memory sort over already-aggregated structs.
2. **Inconsistent results across page fetches** ("page drift"): if new sessions/data arrive
   between fetching page 1 and page 2 (live sessions still writing JSONL, `WatchInsights`
   pushing updates), a sort key that changes between requests can cause a session to appear
   on two pages or be skipped entirely — classic offset-pagination-under-mutation bug, well
   documented for any "sort by computed/frequently-changing column + offset pagination"
   combination (e.g. Stripe's and GitHub's API pagination docs both call this out
   explicitly for time-ordered-but-mutable collections). Standard mitigations: cursor-based
   pagination keyed on a stable tiebreaker (session ID) in addition to the sort column, or
   accept a documented "snapshot as of request time" semantics and don't promise
   real-time-consistent paging.
3. **Client-side text search (Fuse.js) and server-side sort/pagination don't naturally
   compose** — Fuse.js needs the full in-memory candidate set to fuzzy-rank; if
   `ListSessionTokens` returns only one page, Fuse.js can only search within that page,
   which is a silent regression from today's full-dataset search. The two realistic
   resolutions (both already named as an open question in requirements.md, this confirms
   the pitfall is real, not hypothetical):
   - Fetch all sessions for search (client-side Fuse.js as today) but use server-side
     `SortBy` only when no search text is active — i.e., search and sort-by-computed-column
     are mutually exclusive UX modes, not simultaneous.
   - Or move text search server-side too (a `Filter`/`Query` param on `ListSessionTokens`),
     which is more consistent but is new proto/backend surface not currently scoped.
   Whichever is picked, do not let the two mechanisms silently coexist with different data
   scopes (page vs. full-set) without an explicit UX decision — that's the "inconsistent
   results" failure mode again, just from a UX angle instead of a data-consistency one.

## 4. Modal → route drill-down migration

Confirmed in code: `InsightsDashboard.tsx` holds `selectedSession` state driving the
existing drawer/modal, plus `WatchInsights` live-patch logic requirements.md already flags
as buggy at the aggregate level.

Common, well-documented failure modes for this exact migration pattern (modal → deep-linked
route), seen repeatedly in Next.js App Router migrations specifically:
- **State loss on direct navigation**: a modal typically inherits list-page state (which
  session was hovered, scroll position, applied filters) for free because it's the same
  React tree. A route (`/insights/session/[sessionId]`) loaded directly (bookmark, shared
  link, browser refresh) has none of that — it must independently fetch everything the
  modal used to get "for free" from parent state. If the new route quietly assumes it's
  always reached via client-side navigation from the table (and never hardens the
  direct-load path), it will crash or render blank on a bookmarked/shared link — precisely
  the deep-linkability this workstream is meant to deliver.
- **Double-fetching**: a naive migration fetches session-list data (for the table) and then
  separately fetches full session detail again when the route mounts, even when navigating
  from the table where much of that data is already in memory — wasteful and a source of
  visible flicker/loading-state churn if not memoized/shared (e.g. via a shared ConnectRPC
  query cache or Next.js route prefetching).
- **Back-button/history stack surprises**: with a modal, back-button behavior is whatever
  the browser does for the underlying page (usually nothing, since URL didn't change). With
  a route, every navigation into a session detail pushes a history entry — if the app
  additionally pushes an entry for filter/sort-state changes on the list page (common when
  those are stored in the URL, which server-side sort/search work in this same project might
  introduce), the back button can require multiple presses to get back to the *previous*
  list view a user expected, or land on a stale filter state. Standard mitigation: use
  `router.replace` (not `push`) for filter/sort state changes so only genuine navigations
  (opening a session) create history entries, and test the exact back-button path in Phase 6
  verification, not just forward navigation.
- **Losing the "quick peek" affordance**: requirements.md already flags this — the modal's
  value is *not losing your place* in the table (scroll position, current sort/filter) while
  glancing at one session's detail. A route-only migration is a strict UX regression for that
  use case unless the modal is explicitly kept as a "peek" (e.g. `Shift+Click` or a small
  inline expand) with the route reserved for "I want to bookmark/share/deep-link this."
  Requirements.md's Rabbit Holes section already allows for "keep the modal for quick-peek if
  cheap" — treat "cheap" narrowly: it means the modal becomes a thin wrapper that pushes the
  URL (so both affordances share one data-fetching/rendering implementation, not two parallel
  ones that can drift out of sync), not a second hand-maintained code path.
- **WatchInsights lag surfacing more visibly**: once a route can be the *first* thing loaded
  (deep link), a stale/lagging aggregate becomes a first-impression bug rather than something
  masked by already having fresh data in the table beforehand. This raises the odds that the
  known aggregate-lag bug becomes user-visible during this project even if the fix is
  explicitly out of scope — worth a call-out in Phase 3 planning's risk section rather than
  discovering it in Phase 6 verification.

## 5. Go-specific pitfalls for a new per-request heuristics-computation path

Checked against `/go-development` and `/go-concurrency` skill guidance. Key applicable
points for a function that scores potentially ~600 `SessionTokenSummary` structs per
request:

- **Allocation patterns**: computing N findings × M sessions naively with `append` inside
  nested loops without pre-sizing is the classic Go performance footgun for exactly this
  shape of workload (bulk transform over a slice) — pre-allocate `make([]Finding, 0,
  expectedCount)` where a reasonable upper bound is knowable (e.g. `len(sessions) *
  numDetectors` as a ceiling before filtering), per `golang-development`/`golang-safety`
  guidance on avoiding repeated slice-growth reallocation in hot paths. At ~600 sessions
  this is not going to show up as a measurable regression on its own, but it's a cheap
  correctness-adjacent habit to establish now since this code will likely grow more
  detectors over time (per requirements.md's framing of six-ish heuristics as a *starter*
  set).
- **Concurrency**: given the NFR's explicit "low-hundreds-of-ms added latency" budget and
  "compute incrementally/cached, not full re-scan per request," the real lever is **caching
  the computed findings alongside the existing `TokenStore` aggregate**, invalidated the
  same way existing aggregates are (on new JSONL data / `WatchInsights` events) — not
  parallelizing the per-session detector loop with goroutines. At 600 sessions, six
  detectors, each detector doing simple arithmetic over already-in-memory struct fields,
  this is microseconds of CPU work total; introducing goroutines/channels here would add
  synchronization complexity (exactly the "don't reach for a concurrency primitive you don't
  need" caution in `golang-concurrency`) for no measurable win, and risks new race surfaces
  in a codebase that already has one documented actor-based lock-free-read discipline
  (`.claude/rules/instance-lock-free-reads.md`) to be consistent with. If per-request compute
  ever does become a bottleneck (e.g. after detector count grows well past six, or session
  count grows well past 600), reach for memoization/caching before reaching for concurrency.
- **Memoization consistency with existing lock-free-read pattern**: if the findings cache is
  hung off `TokenStore` (or a comparable long-lived struct) rather than recomputed from
  scratch, follow the same snapshot/publish pattern already established for `*Instance` in
  `.claude/rules/instance-lock-free-reads.md` — i.e. don't add an ad hoc `sync.RWMutex` around
  a new findings cache when the codebase already has a working `atomic.Pointer[T]`
  publish-on-mutation convention for exactly this "background recompute, many concurrent
  readers" shape. Confirm in Phase 3 planning whether `TokenStore` already follows that
  pattern or a different one before choosing the findings-cache's own concurrency approach,
  so the new code is consistent with whichever convention the existing package actually uses.
- **Fixture-based testing for heuristics** (already flagged in requirements.md's Feasibility
  Risks as a net-new pattern for this codebase): standard practice from the reference tools
  and general heuristic-engine testing is table-driven tests keyed on synthetic
  `SessionTokenSummary` fixtures with one fixture per detector's boundary condition
  (just-under-threshold → no finding, just-over-threshold → finding fires, exactly-at-
  threshold → pick and document one side deliberately). Go's table-driven test idiom
  (`golang-testing` skill) is a direct fit — don't invent a bespoke fixture DSL; a slice of
  `{name string, summary SessionTokenSummary, wantFindings []FindingType}` structs is
  sufficient and keeps the "which threshold produced which finding" mapping explicit and
  reviewable, which doubles as the threshold-staleness documentation from section 1.

## Sources

- `project_plans/insights-cost-intelligence/requirements.md` (Rabbit Holes, Feasibility
  Risks, Open Questions sections) — primary source for scope and already-identified risks.
- Codebase: `server/services/insights_service.go:353` (`ListSessionTokens`), `:437`
  (`SortBy`); `web-app/src/app/insights/SessionsTable.tsx:5` (`react-virtuoso`
  `TableVirtuoso`), `:71` (`new Fuse(...)`); `web-app/src/app/insights/InsightsDashboard.tsx`
  (`selectedSession` modal state, `WatchInsights` usage).
- `.claude/rules/instance-lock-free-reads.md` — this repo's existing concurrency convention
  for actor-mutated, concurrently-read struct fields, relevant if a findings cache is added.
- General industry practice (not project-specific, drawn from well-documented patterns in
  AWS Cost Explorer/Cost Anomaly Detection, Datadog Cost Management, Stripe/GitHub API
  pagination docs, and standard Next.js App Router modal-to-route migration guidance) —
  applied here as domain knowledge, not verified against live external sources in this
  session (no WebFetch/WebSearch used); flag as INFERRED where a claim rests on this rather
  than an opened primary source.
