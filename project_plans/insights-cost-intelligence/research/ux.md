# Research: UX — insights-cost-intelligence

Scope per requirements.md: (1) waste-pattern findings panel, (2) per-tool/activity cost
breakdown, (3) richer sort/search on the sessions table, (4) deep-linkable session
drill-down route.

## 1. Comparable UX patterns (Datadog/APM, cost tools)

Observability/cost tools that combine "what happened" (charts) with "what's wrong"
(verdicts) converge on the same layout, not a single all-purpose widget:

- **Severity-ranked card/list panel, separate from the chart area** — Datadog APM's
  "Watchdog Insights" and cost tools like the reference `cacheeconomics`/`TokenUsage`
  render findings as a **ranked list of cards**, not a banner and not inline table-row
  highlighting alone. Each card: severity chip, one-line verdict, $ impact, and a
  single primary action (usually "view detail"/"jump to X"). This works because a
  banner can only hold one message at a time (findings here are multi-item: 6 heuristic
  types × N sessions can all fire at once) and inline-row-only highlighting has no room
  for the *why* — a colored row tells you *something* is wrong with that row but not
  what, forcing a second lookup anyway.
- **Charts stay verdict-free; findings own the verdict.** Datadog's own dashboards keep
  raw metric tiles/graphs purely descriptive (no color judgment baked into a line
  chart) and put anomaly/verdict framing in a distinct "Insights"/"Recommendations"
  rail. Mixing the two (e.g., coloring a chart line red because a threshold breached)
  tends to produce false urgency on tiles that are just naturally variable (token
  count swings run-to-run) — the reference tools' complaint that "a 99.8% cache-hit
  tile looks great even when the cache write is wasted" is exactly this problem:  a
  bare metric can't self-flag, so it needs an adjacent verdict, not a color rule baked
  into the metric tile itself.
  Recommendation for this feature: **do not recolor `SummaryCards`/chart tiles based on
  findings.** Keep the new findings panel as the single place verdicts render.
- **Row-level tie-back, not just a list.** Best-in-class implementations (Datadog,
  most cost tools) still connect a finding to its source row: clicking a finding item
  scrolls/highlights the responsible table row (or navigates to its detail), and,
  separately, hovering/selecting a table row that a finding names shows a small badge
  ("2 findings") inline. Two mechanisms, not one — a card list for scanning severity at
  a glance, and inline badges for "is *this specific* row implicated."

**Applied recommendation:** place the findings panel as its own `section` above (or
beside, in the `grid2` two-column layout already used for `DailySpendChart`/
`ModelBreakdownChart`) the sessions table, using the existing card visual language
(`vars.color.errorBg`/`warningBg`/`criticalBg` borders — see §3) — not a full-width
banner (too disruptive for something that can have 5+ simultaneous findings) and not
color-only row shading in `SessionsTable` (insufficient bandwidth for the verdict text
and fails the color-only a11y rule anyway, see §3).

## 2. User mental models: "finding" vs. "metric"

- **A metric answers "how much."** A finding answers **"is this normal, and if not,
  what do I do about it."** Users expect a metric tile (`SummaryCards`,
  `ProjectedCostCard`) to be a number with maybe a trend arrow; they expect a finding
  to look like an actionable sentence: *severity + specific subject (a session ID, a
  project path, a model) + the number that makes it bad + a suggested fix.* The
  existing reference tools' phrasing pattern ("cache-hit-rate floor breach: session
  `abc123` at 12% vs 80% baseline, ~$4.20 wasted — consider batching reads") is the
  right shape; a finding that only says "cache hit rate low" without naming a specific
  session/session-set fails the mental model and just becomes a second metric tile in
  disguise.
- **Clicking a finding: users expect BOTH jump-to-session AND filter-the-table**,
  scoped by cardinality:
  - **Single-session finding** (e.g., "kitchen-sink session token ceiling breach in
    session X") → clicking should **navigate directly to that session's drill-down**
    (the new `/insights/session/[sessionId]` route from workstream 4). This is the
    "find the driver fast" success metric from requirements.md — a single hop, not a
    filter-then-scan.
  - **Pattern-level finding spanning multiple sessions** (e.g., "cache-bust from
    mid-session model switches: 6 sessions matched") → clicking should **filter/scroll
    the sessions table** to just those sessions (reusing the search/sort work in
    workstream 3), because there's no single "the" session to jump to.
  - Do not force one mechanism for both cases — a finding card should carry its own
    affordance (a session-id chip that link-navigates, vs. a "View N sessions" button
    that filters) rather than making every finding non-committally do "sort of both."
- **Users don't expect findings to be exhaustive or auto-refreshing in real time** —
  unlike the live `WatchInsights` metric tiles, a findings list recomputing on every
  streamed token update would be visually noisy (severity chips reordering while
  someone reads them). Compute findings on page load / explicit refresh, not on every
  `WatchInsights` patch tick. (This also sidesteps the `WatchInsights` aggregate-lag
  bug flagged in Rabbit Holes — findings should recompute against a stable, refreshed
  snapshot, not against a partially-patched live aggregate.)

## 3. Accessibility

Current app already has real a11y infrastructure worth building on, not from scratch —
Axe Core gates PRs touching `web-app/src/` per CLAUDE.md.

**Findings panel — color must not be the only signal.**
- `theme-contract.css.ts` already defines a **4-tier status system**:
  `success`/`warning`/`error` plus a distinct **`critical`** tier
  (`critical`/`criticalBg`/`criticalText`, added specifically so "a Critical-risk badge
  doesn't share a hue with a High-risk badge" — see the comment at
  `web-app/src/styles/theme-contract.css.ts:52-57`). Waste findings should map onto
  this existing 4-tier vocabulary (e.g. critical/high/medium/low) rather than inventing
  a parallel severity palette — reuse, don't fork.
- Each finding card must pair color with a **text label** ("Critical", "Warning") and
  ideally an icon/glyph, not rely on badge background hue alone — same rule the repo
  already applies elsewhere (e.g. `liveIndicator` in `InsightsDashboard.css.ts` pairs
  color with a pulsing dot *and* the word "Live" via `aria-label`, not color alone).
  Concretely: `<span className={severityBadge} data-severity="critical">Critical</span>`
  with the word rendered, never a bare colored dot.
- Findings panel container: use `role="list"`/`role="listitem"` (or a native `<ul>`)
  so screen readers announce count ("5 items"), and give each finding card a
  `aria-label` summarizing severity + subject + $ impact in one string (mirroring how
  `ProjectedCostCard.tsx:55` and `TimeRangeFilter.tsx:77` already build one-line
  `aria-label`s for compound controls).

**Sortable table headers — already correct, extend the same pattern.**
- `SessionsTable.tsx:175` already sets `aria-sort` correctly
  (`ascending`/`descending`/`none`) and is covered by an existing test
  (`SessionsTable.test.tsx:113,129` assert the attribute changes on click). New sort
  columns (duration, cost-per-message, cache ROI, waste score) must follow the exact
  same header component/pattern — no new one-off header markup — and get the same
  aria-sort assertion added to `SessionsTable.test.tsx` for each new sortable column.
- Cache ROI's signed value (can be negative) needs a non-color cue too if it's
  presented with a color (e.g. red for negative $ saved) — add a `+`/`-` sign or
  "saved"/"cost" word in the cell text, not color-only, consistent with the findings
  panel rule above.

**Route-based drill-down — focus management is a real gap to close, not inherited for
free from the modal.**
- The current `SessionDetailDrawer.tsx` sets `role="dialog"`, `aria-modal="true"`,
  `aria-label`, and handles Escape-to-close (`SessionDetailDrawer.tsx:47-54`), but on
  inspection it has **no focus-trap and no initial-focus management** — no `useRef`/
  autofocus moving focus into the dialog (e.g. onto the close button) when it opens,
  and no focus restoration to the triggering element on close. This is a pre-existing
  gap in the modal, not something the route migration can copy forward and call done.
- For the new route (`/insights/session/[sessionId]`), Next.js App Router does a full
  client-side navigation with no native "focus the new view" behavior — so the route
  page must **explicitly move focus to its `<h1>`/heading-equivalent on mount**
  (`useEffect` + `ref.current?.focus()` with `tabIndex={-1}` on a heading), the
  standard SPA-route a11y fix for "screen reader still announces the old page." This
  is *more* important for a route than a modal, since there's no native modal
  semantics (`role="dialog"`) to lean on — a route is just a new page from an AT
  perspective, so its heading-focus is the only signal an AT gets that content changed.
- If the modal is kept as an "optional quick-peek" per requirements.md, fix its
  missing initial-focus/focus-trap in the same pass rather than letting it linger as a
  known-but-unfixed a11y gap now that the route version is held to a higher bar right
  next to it — an inconsistent a11y bar between two views of the same content (modal
  vs. route) is more confusing than either alone.
- Keyboard nav for the findings panel: each finding card's primary action (jump/filter)
  must be a real focusable `<button>`/`<a>`, not a `<div onClick>` — check this
  explicitly since Axe Core in CI catches missing-role but not always missing
  keyboard-operability on custom-styled clickable divs.

## 4. Error/edge-case UX: computation failures and "estimated" data

Ties into the `cacheeconomics`-inspired evidence-class distinction requirements.md
flags (measured vs. modeled/estimated numbers) and the per-tool cost attribution
caveat in Rabbit Holes (turn-level cost can't be cleanly split across multiple tool
calls in a turn).

- **Findings-computation failure → an explicit empty/error state inside the panel,
  never a silently empty list.** Per Observability Requirements
  ("a computation error should show up as an empty/error state in the findings panel,
  not page anyone"), the panel needs three distinguishable states, not two:
  1. *Loading* (skeleton, consistent with `InsightsDashboardSkeleton.tsx`'s existing
     pattern),
  2. *Computed, zero findings* ("No waste patterns detected" — a genuinely good-news
     empty state, styled like `emptyState` in `InsightsDashboard.css.ts`), and
  3. *Computation failed* (styled like `errorBox` — `vars.color.errorBg`/`error`/
     `errorText`, already used for the dashboard's top-level error state) with the
     actual error surfaced, not swallowed into an empty list. States 2 and 3 look
     identical to a user unless the empty state explicitly says "no findings" and the
     error state explicitly says "couldn't compute findings" — conflating them (e.g.
     both rendering as just an empty panel) directly defeats the "find the driver
     fast" success metric, since a user seeing nothing can't tell if spend is fine or
     the panel is broken.
- **Estimated/modeled numbers need a visual "not measured" marker, inline, not just a
  tooltip.** The per-tool cost attribution formula (Rabbit Holes: even-split vs.
  full-turn attribution vs. tool-type-level session sum) and activity-type
  classification are both heuristics, not ground truth measured from the JSONL. The
  `cacheeconomics` precedent requirements.md cites is "abstain rather than guess" —
  applied to UI, that means:
  - A distinct, reusable **"estimated" badge/marker** (e.g. a small `~` prefix on the
    number plus a `title`/`aria-describedby` tooltip explaining the attribution
    method, following the same `aria-describedby` pattern already used in
    `SessionDetailDrawer.tsx:72`) on every per-tool cost figure and activity-type
    label — not buried in a footnote or a separate "methodology" page nobody opens.
  - This marker should be a single shared component/style token (e.g.
    `estimatedValueMarker` in a shared `.css.ts`) used identically in the per-tool
    breakdown, the activity-type view, and the waste-score column — one visual
    language for "modeled" across all three new surfaces, not three different
    treatments.
  - Per Rabbit Holes, if a tool-cost number can't be attributed with the chosen
    formula for some reason (e.g. no turn data), **abstain and show "—" with an
    explanatory tooltip**, rather than a $0.00 that reads as "this tool is free."

## 5. Jobs-to-be-done

- **Functional — "find the cost driver fast."** Directly the requirements.md success
  metric #1: given a spend spike, identify the responsible session/pattern in under a
  minute without manually opening sessions one by one. The findings panel + direct
  session-jump (§2) is the mechanism; the richer sort (cost-per-message, waste score)
  is the fallback path when no automated finding caught it.
- **Emotional — confidence, not surprise-anxiety.** The problem statement's framing
  ("a 99.8% cache-hit-rate tile looks great even if... most of that cache write is
  never read back") is explicitly about *false confidence* being worse than no
  confidence — the emotional job is closing the gap between "the dashboard looks fine"
  and "spend is actually fine." A findings panel that stays empty when things are
  genuinely fine (§4's good-news empty state) is doing this job correctly; one that's
  always empty because it silently failed is doing the opposite of the job. The
  estimated-value markers (§4) also serve this job indirectly — knowing which numbers
  are solid vs. modeled prevents a false sense of precision that could later be
  undermined ("wait, that $40 'tool cost' was made up?"), which is worse for
  confidence than an honest "~$40, estimated."
- **Social — none.** Requirements.md explicitly scopes this as a single-operator tool
  (Tyler, or another solo operator of their own instance) with no external/third-party
  consumers — no team dashboard, no sharing, no "look how efficient I am" framing to
  design for. The deep-linkable route (workstream 4) is bookmarkable for the
  operator's own workflow (e.g. pinning a specific expensive session to revisit), not
  for sharing with others — no share-card/preview-metadata work is implied by
  "deep-linkable" here.

## Sources checked in this repo

- `web-app/src/app/insights/SessionsTable.tsx` (existing `aria-sort` at line 175,
  search/model-filter `aria-label`s at 295/301)
- `web-app/src/app/insights/SessionsTable.test.tsx` (existing `aria-sort` assertions,
  lines 113/129 — pattern to replicate for new sort columns)
- `web-app/src/app/insights/SessionDetailDrawer.tsx` (dialog role/aria-modal/
  aria-describedby at 69-72, Escape handling at 47-54, no focus-trap/initial-focus
  found)
- `web-app/src/app/insights/InsightsDashboard.css.ts` (existing `errorBox`,
  `emptyState`, `loadingBanner`, `liveIndicator` visual tokens — reuse targets)
- `web-app/src/styles/theme-contract.css.ts` (4-tier severity system:
  success/warning/error/**critical**, lines ~40-57 — reuse for finding severity, don't
  fork a new palette)
- `project_plans/insights-cost-intelligence/requirements.md` (scope, rabbit holes,
  evidence-class/estimate framing, success metrics)
