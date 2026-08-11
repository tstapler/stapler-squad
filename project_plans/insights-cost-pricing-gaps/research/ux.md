# UX Research: Insights Pricing Gaps (Agent 5)

## 1. Comparable UX patterns — "data present but can't be fully computed"

No single canonical pattern dominates; the ecosystem converges on **layering
multiple weak signals rather than one strong one**, because a single loud
banner is too disruptive for a partial/per-row gap. Recurring treatments,
roughly in order of how surgical they are:

1. **Inline badge/pill next to the value** — "Estimated", "N/A", "Pending",
   or an outlined `?`/`!` icon sitting directly on the stat tile or table
   cell, usually with a tooltip on hover/focus giving the reason. This is
   the dominant pattern for **per-row or per-series** gaps (AWS Cost
   Explorer's "Unblended cost" footnotes, GitHub's Billing & usage page
   marking metered items "estimate — final invoice may differ", Stripe
   Billing's `~` prefix + tooltip on prorated/estimated line items).
2. **Distinct visual encoding on the chart mark itself** — a hatched/striped
   fill, a dashed outline, or a muted/desaturated version of the series
   color instead of the solid fill used for "real" data. This is the
   standard fix for the *specific* failure mode here (a bar that's
   literally invisible because its value is 0) — Plotly, Highcharts, and
   Datadog's own metric-graph guidance ("Graphing anti-patterns") call out
   using pattern fills or a "no data" placeholder mark instead of letting a
   0/null value collapse to nothing. A double-encoded signal (color *and*
   pattern/text) is also the standard accessibility fix, see §4.
3. **A footnote / caption under the chart or table**, e.g. "* cost estimate
   unavailable for 1 model" — cheap, low-intrusion, doesn't require
   touching the chart rendering, but is easy to miss and doesn't scale past
   ~1-2 affected series.
4. **A page-level banner** (Datadog's "this dashboard uses data from a
   monitor that no longer exists", GitHub Actions' "Some jobs failed to
   report status") — reserved for **systemic** problems (a whole widget or
   the whole page is degraded), not for "one model out of five lacks
   pricing." A banner here would be over-scoped for what's fundamentally a
   per-model annotation.

**Takeaway for this feature**: this repo's existing `isOverBudget` banner
(`InsightsDashboard.tsx:142-146`, `errorBox` + `role="alert"`) is the right
*pattern class* for page-level problems but the wrong *scope* for pricing
gaps — pricing-unavailable is inherently per-model, so it should use pattern
2 (chart) + pattern 1 (badge/legend) at minimum, with pattern 3 (aggregate
footnote) as the cheap catch-all for anywhere a per-model badge isn't
practical (e.g. `SummaryCards`' single `Total Cost` tile, which aggregates
across all models).

## 2. Current component structure — where an indicator slots in

Read: `InsightsDashboard.tsx`, `SummaryCards.tsx`, `ModelBreakdownChart.tsx`,
`insightsFormatters.ts`, `ModelBreakdownChart.css.ts`, `InsightsDashboard.css.ts`,
and `proto/session/v1/insights.proto` (`SessionTokenSummary`, `DailyTokenBucket`,
`ModelBreakdown`, `GetInsightsSummaryResponse`).

- **No pricing-availability signal exists anywhere in the proto today.**
  `ModelBreakdown` (insights.proto:64-71) has `model_family`,
  `total_input_tokens`, `total_output_tokens`, `cache_read_tokens`,
  `estimated_cost_usd`, `session_count` — no bool/flag. `SessionTokenSummary`
  is the same shape for per-session cost. `GetInsightsSummaryResponse`
  already carries a `pricing_as_of` timestamp (insights.proto:103) that is
  **defined but never read by the frontend** (`grep` for `pricingAsOf`
  across `web-app/src/app/insights/` and `insights_service.go` returns
  nothing) — a preexisting dead field, worth noting for G3/G4 but not
  something to build the new indicator on since it answers "when was
  pricing last updated," not "which models are unpriced."
- Per the repo's `interface-pollution-checklist.md`, the minimal-abstraction
  move is: add **one new field** to `ModelBreakdown` (e.g.
  `bool pricing_unavailable = 7;`) and mirror it on `SessionTokenSummary`
  and the `cost_by_model` map on `DailyTokenBucket` (that one needs a
  sibling `repeated string unpriced_models = 9;` since it's a map, not a
  list of structs). Do **not** invent a `PricingStatus` enum or a wrapper
  type for what's fundamentally a boolean per row — that would be exactly
  the "unjustified generic" smell the checklist calls out, given there are
  currently only two states (priced / unpriced), not an open-ended set.
- **`ModelBreakdownChart.tsx` (lines 42-56, 98-102)** is the direct fix
  target for the screenshot bug: `toDataPoints()` maps `ModelBreakdown[]` to
  `{family, cost, color}`; the `<Bar>` renders `Cell`s colored from a fixed
  `PALETTE`. A model with `estimated_cost_usd = 0` (unpriced) is currently
  indistinguishable from one with a real `$0` (e.g. free preview access) —
  both render a zero-height bar with no color/pattern difference from a
  hypothetical real one. The fix slots into three existing seams with no
  new component needed:
  - `toDataPoints()` — pass through the new `pricingUnavailable` flag onto
    `DataPoint`.
  - the `<Cell>` render (line 99-101) — for unpriced entries, force a
    minimum visible bar height (a UX nicety, not required) or leave 0-height
    but render the family's legend entry and X-axis label distinctly (see
    below) so the *absence* itself reads as "flagged," not "missing."
  - the `legendRow`/`legendItem` block (lines 106-113,
    `ModelBreakdownChart.css.ts:40-53`) — already iterates `data` per
    family with a colored dot + label; add a small inline badge/icon next
    to the family name when `pricingUnavailable` is true (e.g. a `⚠` glyph
    or a text suffix "(pricing unavailable)"), reusing the existing
    `legendItem`/`legendDot` styles rather than a new component.
- **`SummaryCards.tsx`** aggregates to one `Total Cost` tile
  (`summary.totalCostUsd`, line 20). This is the right place for pattern 3
  (footnote), not pattern 1 (per-item badge) — there's no per-model
  granularity at this level. Minimal change: if the response carries any
  unpriced models (a new top-level `repeated string unpriced_models` on
  `GetInsightsSummaryResponse`, or simply `models.some(m =>
  m.pricingUnavailable)` computed client-side from the `ModelBreakdown[]`
  already in `summary.models`), render a small `cardSub`-styled caption
  under `Total Cost` — e.g. "excludes N unpriced model(s)" — using the
  existing `cardSub` className, no new CSS needed.
- **`insightsFormatters.ts`** — `fmtCost()` (lines 5-9) is the single
  formatting chokepoint for every dollar figure across the dashboard
  (`SummaryCards`, `ModelBreakdownChart`'s `fmtDollar` is actually a
  separate local copy at `ModelBreakdownChart.tsx:58-60`, not reusing
  `fmtCost` — a minor pre-existing duplication, not in scope to fix here
  unless trivial). Do **not** overload `fmtCost` to return a sentinel
  string like `"N/A"` for unpriced values — that conflates formatting with
  business logic and would require every call site to string-match the
  output. Keep the boolean flag as a sibling field the caller branches on
  before calling `fmtCost`, e.g.:
  ```tsx
  {m.pricingUnavailable ? (
    <span className={unpriceBadge}>pricing unavailable</span>
  ) : (
    fmtCost(m.estimatedCostUsd)
  )}
  ```
- **`SessionsTable.tsx` / `TopNTables.tsx`** (not fully read, but referenced
  in requirements as also flowing through the same per-session cost field)
  should get the same per-row treatment if they render a cost column —
  confirm during planning whether they already import `fmtCost` directly
  (likely yes, given it's exported from the shared formatters file) since
  that's the natural place to wrap with the same conditional.

## 3. User mental model — what does Tyler actually need to *do*?

Tyler is the sole user and the actual bill-payer, which simplifies this
considerably relative to a multi-tenant SaaS:

- He does **not** need a CTA/deep-link to "add pricing override" baked into
  the Insights UI itself. G3's override mechanism is an operator/config-file
  concern (wiring `LoadPricingOverride()` into `server/dependencies.go`, or
  similar) that he'll exercise via the codebase/config, not via a UI click
  path — this is a single-operator internal tool, not a product with
  self-serve admins. Building an in-app "configure pricing" flow would be
  scope creep relative to G2/G3 as written, and risks the "speculative
  interface" smell (a UI for a hypothetical future multi-user override flow
  that doesn't exist yet).
- What he *does* need from the indicator: **enough information to know
  it's a known, bounded, already-being-tracked gap** rather than a bug he
  needs to go investigate. Concretely: the model family name (already
  shown in the legend/table), and ideally the word "pricing" somewhere in
  the label so it reads as "we don't have a price for this model" rather
  than "something crashed." A tooltip is a reasonable enhancement (e.g.
  "Pricing for claude-sonnet-5 was added to the lookup table on
  2026-07-27") but is not required for MVP — a static label is sufficient
  since AC-1 closes the specific Sonnet-5 gap in the same change, so this
  indicator's job going forward is mostly a **regression signal for the
  next new model family** (G4), not a recurring workflow he'll interact
  with often.
- No dismiss/acknowledge/snooze affordance is needed — this isn't an
  alert queue, it's a factual annotation that should simply disappear on
  its own once the pricing table is updated (which is exactly what
  happens for free, since the flag is computed server-side from whether
  the family exists in `pt.Prices`).

## 4. Accessibility

This is a single-user internal tool (Tyler is colorblind-status unknown but
not a stated requirement), so **full WCAG AA rigor is not warranted** — no
screen-reader-only user, no external audience, no legal/compliance driver.
That said, the specific bug here (a bar that's invisible because it's
*zero-height*, not just wrong-colored) is a **general usability defect
independent of accessibility** — a sighted, non-colorblind user also can't
see a 0px-tall bar. So the fix is warranted on ordinary usability grounds,
and the "don't rely on color alone" principle is worth applying cheaply
since it's nearly free here:

- **Primary fix**: don't rely on the bar's presence/absence at all — the
  legend entry (which always renders regardless of bar height, per
  `ModelBreakdownChart.tsx:106-113`) is the reliable place to put a
  text/icon indicator, since it's never subject to a zero-height problem.
  This solves both the sighted-zero-height-bar issue and the
  color-blind-user issue in one move, without needing SVG hatching/pattern
  fills in the bar itself (which recharts supports via `<pattern>` defs in
  an SVG `<defs>` block, but is a heavier lift for marginal benefit given
  the legend-based fix already covers it).
  - Recommendation: **text label, not color/icon alone** — e.g. a "⚠"
    prefix *and* the word "(unpriced)" appended to the family name in the
    legend, since an icon alone still fails a screen-reader/color-blind
    user if unlabeled, and this app has no existing icon-font/aria-label
    convention to lean on cheaply (confirm during planning: check whether
    an icon library is already a dependency before introducing one just
    for this).
- **ARIA**: if a badge element is added, no `role="alert"` — that's for the
  page-level banner precedent (`isOverBudget`, `InsightsDashboard.tsx:143`)
  signaling something actionable/urgent; a per-row "pricing unavailable"
  annotation is informational, not urgent, so a plain `<span>` with visible
  text is sufficient (the text itself is what a screen reader announces —
  no extra `aria-label` needed beyond making sure the badge text isn't
  purely symbolic, e.g. don't ship a bare "⚠" with no accompanying text).
- Skip: full axe-core/Lighthouse contrast audit of a new badge color — this
  repo's e2e CI already runs Axe/Lighthouse on PRs touching `web-app/src/`
  (per this repo's own `CLAUDE.md`), so it'll be caught automatically if it
  regresses; no need for manual WCAG contrast-ratio verification during
  planning as a separate task.

## 5. Partial-period pricing gaps

Scenario: pricing was missing for `claude-sonnet-5` from (say) 2026-07-01
through today (2026-07-27, whenever AC-1 ships), then the table is updated
— should the UI distinguish "this $0 in the June 1-27 window is due to a gap
that's now fixed" from "this $0 today is still a live gap"?

**Recommendation: blanket "was unpriced, now recomputed going forward" is
sufficient — do not build per-date-range provenance tracking.** Reasoning:

- The **Non-Goals** section of requirements.md explicitly excludes
  "historical cost re-computation/backfill for past sessions" — so those
  old $0 entries stay $0 in the DB regardless of what the UI shows. A
  partial-period indicator would be UI polish describing data that the
  backend has explicitly decided not to fix, which is an odd asymmetry:
  don't build a UI to explain a gap the project deliberately isn't
  closing.
  - **However**, note this creates one *slightly* NEW rough edge worth
    flagging to planning: once `claude-sonnet-5` pricing lands in
    `DefaultPricingTable()` (AC-1), the `pricingUnavailable` flag this
    feature adds will read `false` for all sonnet-5 usage — including the
    already-recorded-as-$0 historical rows the Non-Goals section says stay
    $0. Those old rows would then show as **plainly-priced $0**, i.e.
    exactly the original "invisible/free-looking" bug, just for
    already-elapsed dates instead of live ones. This isn't in scope to fix
    (backfill is a non-goal) but is worth one sentence in the plan/PR
    description so it's a documented, accepted gap rather than a surprise
    later — e.g. "historical pre-fix sonnet-5 entries will still read as
    $0 with no unpriced flag, since the flag is computed live against
    current `DefaultPricingTable()` contents, not against what was priced
    at recording time."
- If Tyler later wants "as-of-pricing-date" provenance, that's naturally
  the `pricing_as_of` field already sitting unused in the proto
  (`GetInsightsSummaryResponse.pricing_as_of`, insights.proto:103) — a
  single global "table was last updated on X" timestamp is the
  low-effort, already-half-built version of this, and is enough to answer
  "was this number computed with current pricing" without per-session
  provenance. Surfacing that existing-but-dead field (e.g. a small caption
  under the dashboard header, "Pricing as of Jul 27, 2026") is a
  reasonable low-cost addition to bundle with G2/G4 if planning wants a
  cheap win, but is not required by any AC and shouldn't block this
  feature if it adds complexity.

## Summary recommendation for planning (Phase 3)

1. Add a single `bool pricing_unavailable` (or equivalently-scoped) field
   per model-family/session record in the proto — no new message type, no
   enum.
2. Fix `ModelBreakdownChart.tsx`'s legend (always renders, unlike the bar)
   to carry a plain-text "(unpriced)" suffix — this is the primary,
   cheapest, most robust fix for the screenshot bug and also satisfies the
   accessibility concern without new dependencies.
3. Add a `cardSub`-styled one-line footnote on `SummaryCards`' Total Cost
   tile when any model in the response is unpriced, using existing CSS
   classes.
4. Branch at each `fmtCost()` call site on the new flag rather than
   changing `fmtCost` itself — keep formatting and "is this even a valid
   number" as separate concerns.
5. Skip: CTA/link to override config (not this tool's UI model), per-date
   provenance UI, hatched SVG chart fills, and a page-level banner — all
   scope creep relative to G2/G3 as written and/or already covered by the
   legend-text fix.
