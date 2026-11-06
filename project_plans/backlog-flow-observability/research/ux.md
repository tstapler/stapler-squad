# UX Research: Backlog Flow View

Scope: system-level "Flow" view for the backlog pipeline (single owner, ~160 items, agent-driven). Inputs: `requirements.md`, `project_plans/backlog-stuck-item-visibility/design/ux.md`, direct inspection of `web-app/src`. Labels: VERIFIED = opened in this repo; PRIOR-ART = general domain knowledge, not re-fetched this session (no web sources cited).

## 1. Existing UI inventory (VERIFIED)

- `/backlog` (list, `web-app/src/app/backlog/page.tsx:649-665`) and `/backlog/board` (`board/page.tsx`) are two routes sharing a `<nav aria-label="Backlog views">` tab bar: List is a `button aria-current="page"`, Board an `AppLink`. Styles `tabBar`/`tab`/`tabActive` in `backlog.css.ts:75-99`. Tabs are routes, not ARIA tablists. `backlog/layout.tsx` gates both on the `backlog` feature flag, so a third route `/backlog/flow` inherits the gate for free.
- Stuck visibility is a section on `/unfinished` plus a nav badge (per the stuck-item design doc), and `useStuckBacklogItems` is already consumed by the board. Its conventions to reuse: `role="group"` + `aria-pressed` filter chips, flex-wrap chip rows, status chip = color + text, distinct filter-aware empty copy, explicit degraded banner instead of silent empty, last-known count on RPC failure, never show "0" before first fetch resolves.
- Charts: `recharts ^3.8.1`, used only in `app/insights/*` (`DailySpendChart.tsx`: `ResponsiveContainer` + `LineChart`, hardcoded `rgba(128,128,128,0.15)` grid, `var(--primary)` stroke, plain "No data" empty box, no `aria-*` on the chart). No Sankey/d3/visx dependency. `/analytics/escape` is a single-purpose terminal-escape page, not a general analytics home, so `/analytics` owns no slot (answers Open Question 4 in part).
- Shared UI primitives under `web-app/src/components/ui/` worth reusing: `EstimatedValue` (shared marker for modeled/heuristic values, with sr-only text), `LiveRegion`, `ErrorState`, `Skeleton`, `Collapsible`, `Badge`, `NavBadge`, `Tooltip`, `SystemBanner`.
- CSS: vanilla-extract `.css.ts` colocated, tokens from `styles/theme.css` (`vars.color.*`, `vars.space[n]`), per `docs/reference/css-architecture.md`. Existing `.module.css` only for legacy.
- The `dataviz` skill is listed as available but its files are not on disk (`~/.claude/skills/dataviz` absent), so its guidance was not read; recommendations below follow standard accessible-dataviz practice instead. GAP: re-check against that skill if it becomes available.

## 2. Comparable patterns and fit

| Pattern | Answers | Fit for 160 items / one owner | Verdict |
|---|---|---|---|
| Cumulative flow diagram (CFD) | WIP growth, bottlenecks (band widening) | Archived items dominate (149 of ~160) so bands are just a growing "archived" slab; thrash events would distort it if not collapsed | Optional later; use only on collapsed status, excluding archived/done bands |
| Funnel / stage conversion bars (with rework back-edges) | "Where does it leak?" | Directly matches "leaking factory" model; small N is fine as counts, not percentages | **Primary** |
| Sankey / transition flow | Which state goes where, loops | PRIOR-ART: Sankey handles loops badly (review->in_progress is a cycle), needs a new dependency, poor keyboard/SR story | Reject as primary. Substitute a **transition matrix table** (from-state rows, to-state columns, counts) which is accessible and shows loops |
| Cycle/dwell-time scatter or box plot | Outliers, tail risk | At ~160 items one dot per item is legible and each dot is an item (drill-down is natural); median alone hides the long tail | **Secondary**: per-stage dwell median + p90 as a table with a dot-strip; scatter optional |
| DORA/value-stream (lead time, deploy frequency, CFR, MTTR) | Throughput and quality trend | PRIOR-ART: map to throughput/week, lead time (idea->done), first-pass yield (analogue of change failure rate), time-in-stuck (analogue of MTTR). Skip the DORA branding | **Borrow the four KPI tiles** |
| CI flaky/retry dashboards (e.g. top-N flaky tests, retry counts, rerun-pass rate) | Which thing is noisy and how often | Direct analogue for thrash episodes, likely_flaky stuck reason, PR-fix reopen loops: ranked "top offenders" list with count and sparkline, each row links to the thing | **Primary pattern for rework/thrash/deadlock sections** |
| Before/after comparison (CI "compare to previous period") | Did a tuning change help? | Required by success metrics | Delta column on every KPI + two-window selector |

Design principle: with ~160 items and a single reader, prefer **ranked lists and tables with a small inline mark** over rich charts. Every number is a link to the items behind it. Charts are used only for (a) trend over time and (b) the funnel.

## 3. Mental model and JTBD

Job statements:
1. When the factory feels slow, I want to see which stage items dwell in and which loop sends them backward, so I can pick one thing to tune.
2. When I changed a threshold or prompt, I want a same-length before/after comparison, so I know whether it worked without SQL.
3. When a number looks bad, I want to click it and see the exact items, so I can judge whether it is a real leak or one pathological item.
4. When two items generate 175k events, I want that surfaced as an anomaly and excluded from averages, so totals aren't drowned.
5. When I'm unsure the data is complete, I want to see freshness and which metrics are estimated, so I know how much to trust it.
6. (Claude/MCP consumer) Same aggregates as JSON with the same as-of and estimated flags; the UI and MCP must share one backend response shape so numbers cannot diverge.

Reading order = the question order: **Is it healthy? -> Where does it leak? -> Why (cause)? -> Which items?** Leak-first, not metric-first. Triage, not investigation (same stance as the stuck-items design).

Drill-down contract: every count renders as a button/link opening a **side drawer item list** (reusing the stuck-section card row: title, status chip, reason, duration) with the filter echoed as a heading ("Review-FAIL rework, last 30d, 12 items"). Drawer row links to the existing `?item=<id>` detail panel. URL carries `window`, `compare`, and `drill` params so views are linkable and back-button safe.

## 4. Recommended placement

**Own route `/backlog/flow`, added as a third tab ("Flow") in the existing `Backlog views` nav**, after Board. Reasons (VERIFIED against code above): tab bar and feature-flag layout already exist; routes already model List/Board; `/analytics` has no general slot; `/unfinished` is a per-item triage queue and would mix scales. Do not put Flow on `/insights` (that is session/spend analytics). Fold the shared tab bar into a small `BacklogViewTabs` component rather than copying the nav a third time (the nav is currently inline in `page.tsx`; board page needs the same).

Add a one-line link from the stuck section ("See stuck reasons over time") to `/backlog/flow#stuck`, and nothing in the primary nav (no new badge; the stuck badge already covers urgency).

## 5. Section layout (top to bottom)

```
Flow                          Window [7d 30d 90d]  Compare [none | previous period]
As of 14:02:11 (12s ago) [Refresh]   ● 2 metrics estimated   ● 1 anomaly
-------------------------------------------------------------------------
A. Health tiles (role=group, 5 tiles, each with delta vs compare window)
   Throughput (done/wk) | Median lead time | First-pass yield | Open stuck | Review FAIL rate
B. Where does it leak?  Funnel: idea > ready > in_progress > review > pr_pending > done
   each stage bar: items entered, % advanced, median/p90 dwell; rework back-edges
   listed under bar: "review -> in_progress 425 (rework)", "pr_pending -> in_progress 4,437 -> collapsed 61 episodes"
C. Why: rework causes (ranked list): review FAIL, PR CI failure, merge conflict, bounce, other
   each row: count, items affected, sparkline per week, [View items]
D. Review gate: verdict mix over time (stacked bars with pattern+label), UNVERIFIABLE rate,
   override count, Deadlock candidates table (item, verdict sequence P/F/F/F, cap hit?, agent claimed done?)
E. PR health: reopen-for-PR-fix, nudge->resolution outcomes, CI vs conflict dwell
F. Stuck reasons: current vs historical table with time-in-stuck (median/p90), current count linked to /unfinished
G. Anomalies & thrash: ranked episodes (item, from<->to, count, duration, last seen), "excluded from aggregates" note
H. Data notes (Collapsible, closed by default): collapse rules, sources, estimated metrics list, row counts, query time
```

Notes:
- A is a single scannable row; only A and the as-of header are above the fold on 1080p, B follows. Sections get `<h2>` headings and a skip/jump list at top (anchors `#leaks #causes #review #pr #stuck #anomalies`).
- Anomalies (G) also appear as a banner under the header when any exist ("2 items thrashing; excluded from averages [View]") because the thrash dominates raw data and must not be missed by someone who never scrolls.
- Funnel (B) rendered as horizontal bars in plain HTML/CSS (vanilla-extract) with a visually hidden data table, not recharts: simpler, fully accessible, no new dependency. Trend sparklines/stacked bars (C, D) use existing recharts via a shared wrapper.
- Avoid percentages with small denominators without the count: show "6/9 (67%)". At N~160 and 5 overrides, show the count first.
- Archived is excluded from all funnel stages by default but counted in throughput; state this in H.

## 6. Accessibility (WCAG AA; Axe blocks CI on AA violations per CLAUDE.md)

- **Not color-only**: every chart series/status gets a text label and a second channel (pattern fill or marker shape, or position). Verdict mix uses the verdict word inside/next to the segment; legend items are text. Status chips reuse `BacklogItemBadge` (color + label).
- **Contrast**: use theme tokens only; text 4.5:1, chart marks and UI boundaries 3:1 in both light and dark. Do not copy the hardcoded `rgba(128,128,128,0.15)` grid for anything carrying meaning. Add a unit/axe check in both themes.
- **Charts are not focusable SVG soup**: each chart is a `figure` with `figcaption`, `role="img"` plus `aria-label` summarizing the takeaway ("Review FAIL share rose from 28% to 35%"), and an adjacent visually-hidden or toggleable `<table>` of the same data. Tooltips-on-hover are never the only way to get a value; values are printed in the row/table.
- **Keyboard**: tiles, rows and "View items" are real `button`/`a`; window and compare selectors are `RadioGroup` (exists in `ui/`) or `role="group"` with `aria-pressed`; drawer is a `Modal`-style dialog with focus trap, Escape closes, focus returns to the invoking control (precedent: `BacklogItemDetail.focusReturn.test.tsx`). Sortable tables follow the list view's header-button pattern (`handleHeaderKeyDown`).
- **Live updates**: refresh results announced politely via `LiveRegion` only on user-initiated refresh or window change ("Flow updated, as of 14:02"), not on every background poll, to avoid chatter. Data tables have `<caption>` and `scope="col"/"row"`.
- **Motion/touch**: no animation required; honor `prefers-reduced-motion` on any bar transitions. Targets >= 24x24 CSS px (WCAG 2.2 AA 2.5.8).
- **Responsive**: tiles and chip rows wrap (flex-wrap, as in the stuck design); tables scroll within a labelled region (`tabindex=0`, `aria-label`) on narrow viewports.
- E2E conventions: `// @feature backlog:flow`, `data-testid`/ARIA locators only, no `waitForTimeout`.

## 7. State matrix (error, empty, partial, freshness)

| State | Behavior |
|---|---|
| First load | Skeleton per section (`Skeleton`); tiles show no number, never "0" (same rule as stuck badge). Page remains navigable. |
| Success | As-of timestamp from the server response (not client clock) with relative age; tooltip shows absolute time. |
| Stale (age > 2x poll interval or fetch failed after success) | Keep last data, dim not hide, `SystemBanner`-style notice "Showing data from 14:02 - refresh failed [Retry]"; never silently empty. |
| Hard error, no prior data | `ErrorState` with retry and the error text; sections not rendered with fake zeros. |
| Partial (one section's query failed or timed out) | Per-section error block with retry; the rest renders. Response must carry per-section status so UI and MCP can express it. |
| Estimated metrics | `EstimatedValue` marker on any modeled metric (e.g. deadlock candidates, cause attributed from free-form note text) plus an entry in H explaining the method. |
| Empty window (no items entered/finished) | "No items finished in the last 7 days" with a one-click widen to 30d; distinct from zero-failure ("0 review FAILs - 14 reviews") which shows the denominator. |
| Small sample (denominator < ~5) | Show counts, grey the percentage, label "low sample". Avoids over-reading 1 of 2. |
| Compare window empty | Delta shows "n/a (no data in previous period)", not +inf%. |
| Thrash present | Banner + section G; aggregates computed on collapsed episodes with a visible "N raw events collapsed into M episodes" note in H. |
| Recently shipped tuning change | Optional annotation markers on trend charts (needs a source of change dates; flagged as a plan open item, not assumed to exist). |

Freshness: the backend should return `asOf`, `windowStart/End`, `rowCounts`, `queryMs`, and `estimated: string[]`; the same fields appear in MCP responses (requirements: "surface data freshness in both UI and MCP").

## 8. Recommendations for the plan

1. Route `/backlog/flow` + `BacklogViewTabs` extraction; register feature `backlog:flow` and RPC(s) in `docs/registry/features/` with `tested:true` + testIds (CI gate).
2. One RPC returning sectioned response (`sections[].status`, shared with MCP) plus one drill-down RPC (`metric` key + window -> item ids with row fields), so every displayed number is backed by a drill key. Design the drill key set alongside the metrics, not afterward.
3. Funnel and tiles in HTML/CSS; trends in recharts through one `FlowChart` wrapper that enforces `figure`/`aria-label`/hidden-table/pattern rules, so individual charts cannot skip accessibility. No Sankey dependency.
4. Axe test in both themes on populated, empty, error, partial states.
5. Defer CFD and change-date annotations; both are additive.

## Gaps / unverified
- Pattern comparisons (CFD, Sankey, DORA, CI flaky dashboards) are domain knowledge, not sourced from fetched references.
- `dataviz` skill content not read (missing on disk).
- Did not inspect `BacklogItemBadge`, `EstimatedValue`, `ErrorState` APIs beyond file names/headers; confirm props before wiring.
- Whether `board/page.tsx` imports the tab bar styles from `backlog.css.ts` or duplicates them was not checked.
