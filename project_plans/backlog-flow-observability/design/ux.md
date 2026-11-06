# UX Design: Backlog Flow View (`/backlog/flow`)

Inputs: `requirements.md`, `research/ux.md`, `implementation/plan.md` Phase 4 (Stories 4.1.1 to 4.3.3). Reading order is the question order: Is it healthy? Where does it leak? Why? Which items? Triage, not investigation.

## 1. Surface inventory

| # | Surface | Type | Plan story |
|---|---|---|---|
| S1 | Backlog view tabs (List / Board / Flow) | interactive | 4.1.1 (error boundary link: 4.1.2) |
| S2 | Page header: window, compare, refresh, freshness, jump list | interactive | 4.1.2 (ACs for selectors, refresh, live region, jump list, widen, unsupported window) |
| S3 | Page-level states (loading, stale, hard error, partial, empty window) | interactive | 4.1.2 |
| S4 | Thrash banner | interactive | 4.2.3 |
| S5 | Health tiles | interactive | 4.2.1, 4.2.5 |
| S6 | Funnel with rework back-edges | interactive | 4.2.1 |
| S7 | Rework causes (ranked list with weekly sparklines) | interactive | 4.2.2, 4.2.5 |
| S8 | Review gate (verdict mix over time + deadlock table) | interactive | 4.2.2, 4.2.5 |
| S9 | PR health | interactive | 4.2.3, 5.2.2, 5.2.3 (combined dwell: 1.2.3) |
| S10 | Stuck reasons table | interactive | 4.2.3 |
| S11 | Anomalies and thrash (one row per item) | interactive | 4.2.3, 4.2.6 ("View events") |
| S12 | Data notes (Collapsible) | interactive | 4.2.3 |
| S13 | Drill-down drawer | interactive | 4.2.4 |
| S14 | `FlowChart` wrapper (component contract) | interactive | 4.1.3 |
| S15 | MCP tool JSON output (freshness/estimated/partial fields) | non-interactive | Phases 1-3 |
| S16 | Debug log line for aggregation | non-interactive | requirements; plan Observability Plan |

Interactive: 14. Non-interactive: 2 (condensed in section 5).

## 2. Page layout and global conventions

```
Backlog views:  [List] [Board] [Flow*]                         (nav aria-label="Backlog views")
-----------------------------------------------------------------------------------------
Flow
Window (o 7d  o 30d  o 90d  o All time)   Compare (o none  o previous period)
As of 14:02:11 (12s ago)  [Refresh]     (i) 2 metrics estimated
Jump to: Leaks | Causes | Review | PR | Stuck | Anomalies
[!] 2 items thrashing; excluded from averages  [View]          <- S4, only when present (archived items counted, see S4)
-----------------------------------------------------------------------------------------
A  Health tiles (role=group aria-label="Health summary")
   [Throughput 9/wk (+2)] [Lead time 3.1d (-0.4d)]   (lead time: plan Story 1.2.2, `tiles.lead_time`) [FPY 32/105 (30.5%)] [Open stuck 8] [Review FAIL 88/355 judgment, raw 186/528]
B  Where does it leak?  (h2 id=leaks)
C  Why: rework causes   (h2 id=causes)
D  Review gate          (h2 id=review)
E  PR health            (h2 id=pr)
F  Stuck reasons        (h2 id=stuck)       collapsed by default (criterion 43)
G  Anomalies & thrash   (h2 id=anomalies)   collapsed by default; the S4 banner is its summary
H  > Data notes (closed)
```

Conventions applied to every section:
- Every number is a button opening the drawer (S13). No orphan numbers (enforced by test).
- Ratios always print the count first: `32/105 (30.5%)`. Denominator < 5 greys the percent and adds "low sample".
- Modeled values carry `EstimatedValue` (marker plus sr-only "estimated") and an entry in Data notes.
- Section state is independent: each of B to G renders its own skeleton, error block or content from its per-section status.
- Window, compare and drill live in the URL (`?window=30&compare=prev&drill=rework.review_fail`).
- Density rule: sections A to E are expanded; F and G are `Collapsible` with a one-line summary, expand on jump-list or banner activation (focus moves to the `h2`), and render expanded when their status is failed or partial (criterion 43).

## 3. Interactive surfaces

### S1. Backlog view tabs

```
[List] [Board] [Flow]      active tab: aria-current="page", visible underline + bold (not color only)
```
Flow: user clicks Flow, route changes to `/backlog/flow`, tab marked current. Keyboard: Tab to each, Enter activates. Flag off: layout gate applies exactly as for Board (feature absent, tab hidden, direct URL shows the gate's existing message). Error: route fails to load, Next error boundary shows existing app error page with a "Back to Backlog" link. Exit: any other tab or browser back.

### S2. Page header controls

Flow:
1. User picks window (7/30/90/All time) or compare (none / previous period). "All time" sets `?window=all`, is sent as `all_time=true`, and shows a one-line "slow path" note when the response has `slow_path=true`. Both are `RadioGroup`s (arrow keys move, Tab leaves).
2. System updates URL (`history.pushState`, back restores prior state), shows skeletons in sections, fetches, announces via `LiveRegion` "Flow updated, as of 14:02" on completion.
3. Refresh button re-fetches without changing selection; same announcement. Background polls update silently (no announcement).
4. Jump list links scroll to section headings and move focus to the heading (`tabindex=-1`).

Compare and All time: "previous period" is undefined for All time, so with All time selected the compare option is `aria-disabled` with the hint "Compare needs a 7, 30 or 90 day window", the value resets to `none`, and a URL carrying `window=all&compare=prev` loads with compare `none` and the parameter removed.

Header facts: as-of comes from the server `as_of`, never the client clock; tooltip shows absolute time. "N metrics estimated" opens Data notes and scrolls to it. Unknown `window` value in URL falls back to 30 with an inline note "Unsupported window, showing 30d". During refresh the Refresh button shows "Refreshing..." and is `aria-disabled` (not removed from tab order). Exit: selections are always reversible; nothing modal.

### S3. Page-level states

| State | What the user sees | Exit / recovery |
|---|---|---|
| First load | Skeleton per section; tiles show skeleton, never "0"; header shows "Loading..." in place of as-of | Resolves automatically; page stays navigable (tabs, jump list work) |
| Success | Content plus as-of with relative age | Normal use |
| Stale (more than 2x the poll interval, 120s at the 60s `FLOW_POLL_INTERVAL_MS`, since the last successful client fetch, or refresh failed after success) | Last data stays, dimmed (opacity plus "Stale" text label), banner `Showing data from 14:02 - refresh failed [Retry]` with role=status | Retry button; data auto-recovers on next successful poll and banner clears |
| Hard error, no prior data | `ErrorState`: "Could not load flow data" plus server error text, `[Retry]`, link "Back to Board" | Retry, or Board tab |
| Partial (a section status `failed`; a timeout is `failed` with the error text) | That section only: error block "Review section failed to load: <reason> [Reload]"; other sections render. Header shows "1 section unavailable" | Reload re-issues the whole read RPC (the label says reload, not per-section retry) and re-renders only failed sections |
| Empty window | Section copy "No items finished in the last 7 days" with `[Widen to 30d]` button. Distinct from zero-failure, which reads "0 review FAILs of 14 reviews" | Widen button, or change window |
| Compare window empty | Delta cell "n/a (no data in previous period)" | Informational; change compare to none |
| All time over the row ceiling (`ResourceExhausted`) | Not the generic error: "All time has too many events to load (N rows, limit M). Pick a shorter window." with `[7d] [30d] [90d]` buttons; no Retry (it cannot succeed); header stays usable | Pick a shorter window |
| Server returns `unavailable` (feature/DB not ready) | `ErrorState` with message from server and Retry | Retry, Board tab |
| Low sample | Count shown, percent greyed, label "low sample" | Informational |
| Thrash present | S4 banner plus section G; Data notes shows "N raw events collapsed into M episodes" | View button |

### S4. Thrash banner

```
[!] 2 items thrashing; excluded from averages   [View]
```
The count is items, not episodes (a thrashing item fragments into several episodes; live data: 35 items all-time, 103 episodes at the 15-minute gap and 61 after the 1h merge, all causes together). Archived items count (their events are excluded from aggregates like live ones) and are labeled: `35 items thrashing (2 archived); excluded from averages`, and an "archived" text chip on their Anomalies rows. Singular copy for one item: `1 item thrashing; excluded from averages`. `role="status"` region under the header, icon plus text (not color only). View scrolls to `#anomalies` and focuses its heading. Not dismissible (the condition is the data); disappears when no episodes in window. Banner failure: if anomaly data failed (partial), banner is replaced by "Anomaly data unavailable [Reload]" (plan Story 4.2.3 AC) so absence of a banner is never misread as "no thrash". Exit: View, or ignore and scroll.

### S5. Health tiles

```
+-----------------+ +-----------------+ +--------------------+ +-------------+ +-------------------+
| Throughput      | | Median lead time| | First-pass yield   | | Open stuck  | | Review FAIL (judg.)|
| 9 done/wk       | | 3.1 d           | | 32/105 (30.5%)     | | 8           | | 88/355 (24.8%)    |
| vs prev: +2 up  | | vs prev: -0.4d  | | vs prev: n/a ...   | | -> /unfinished| | raw 186/528 (35.2%)|
+-----------------+ +-----------------+ +--------------------+ +-------------+ +-------------------+
```
Flow: click a tile's number opens the drawer with the items behind it (`data-testid="flow-tile-<name>"`). The Throughput tile embeds a weekly `FlowChart` sparkline (from `trends.throughput`) with first/last weekly values printed as text. Delta shows direction as an arrow glyph plus signed text ("up +2"), and a text word "better"/"worse" where direction is meaningful, so meaning is not color. Open stuck links to `/unfinished` (an `a`, not a button, and labelled "opens Unfinished page").
The Review FAIL tile leads with the adjusted rate (judgment verdicts only) and shows the raw rate as the secondary line, so the headline is not the number the feature itself calls misleading. Compare delta for a metric below the minimum sample reads "n/a (low sample: 8 vs 26 items, need 20)" with raw values visible and no better/worse word.
States: loading skeleton; section error block with Reload (tiles are one section); zero denominators render "no data" not "0%"; compare empty gives "n/a (no data in previous period)"; low sample greys percent. Exit: drawer Escape; Open stuck navigates away with browser back returning to the same URL state.

### S6. Funnel with back-edges

```
idea       ######################  entered 158  advanced 150  median 2h  p90 1d
refining   ####################    ...                         (only when in the pipeline's stage order)
ready      ###################     entered 150  advanced 141  median 5h  p90 2d
in_progress ################       entered 141  advanced 105  median 1.1d p90 4d
   back-edge: review -> in_progress 426 (rework)    pr_pending -> in_progress 96 real reopens (4,341 raw events in 7 thrash episodes excluded)
review     ##########              ...
pr_pending ########                ...
done       ######                  ...
Other stages: 3 items              (statuses outside the pipeline's stage order, e.g. a custom stage)
```
Stages come from the response's `stage_order` (the pipeline definition), not a constant: a custom stage appears in order, `refining` appears when configured, and anything unknown lands in the "Other stages" row with its count.
HTML/CSS bars with the count printed in the row (width is a redundant channel). Each stage row is a `li` in an ordered list with text "Stage 3 of N: in_progress" (N from `stage_order`). Back-edges are buttons opening the drawer. Visually hidden `<table>` mirrors the data (`data-testid="flow-funnel-stage-<name>"`). Archived excluded (stated in the row footnote and Data notes). Real counts are primary; the excluded raw events appear only inside the label ("4,341 raw events in 7 thrash episodes excluded"), so the funnel and the S4 banner say the same thing.
States: empty stage "no items entered" (not a zero-width bar with no text); loading skeleton; section error with retry; dwell missing for a stage shows "n/a" with tooltip reason. Exit: drawer Escape.

### S7. Rework causes

```
Cause                     Count   Items  Trend (weekly)      
1 Review FAIL rework      398     93     ▂▃▅▆▃  [View items]
2 PR fix (CI/review/conflict not separable until trigger capture)  96 real + 7 thrash episodes  43  ▁▂▂▃  [View items]
3 Bounce                  28 ..
(all-time mock; 7 thrash episodes are on 3 items; 61 is the all-cause thrash episode count, not the PR-fix count)
4 Other                   ..
```
Ranked table with `<caption>`, `scope` attributes. The count cell is the button (`drill=rework.review_fail`). Sparklines are `FlowChart`s (consumers of Story 4.2.5, fed by `trends.rework_by_cause`) with a text takeaway label and a hidden table; the row also prints first/last weekly values as text. Sorting: default by count descending; header buttons toggle (Enter/Space), `aria-sort` set on the active `th` (criterion 41). States: no rework "No rework in this window" with the item count denominator; section error with retry. Exit: drawer Escape.

### S8. Review gate and deadlock candidates

```
Verdict mix per week (stacked bars from trends.verdict_mix; segment label inside bar or in table: PASS / FAIL / PARTIAL / UNVERIFIABLE; empty week = labelled zero)
  Adjusted FAIL 88/355 (24.8%)      Raw 186/528 (35.2%)  (i) 98 "no committed changes" and 34 self-check failures excluded; 18 reviewer-degraded and 23 auto-verified also excluded
  UNVERIFIABLE 110/528 (20.8%)   Human overrides 5
Review-fail streaks (heuristic)  (i) estimated, top 10 of N (ranked by run length; weak filter: not a verdict on the reviewer; see Data notes)
  Renamed "Deadlock candidates" only if the Story 6.1.1 hand-labeled hit rate is at least 50%.
 Item      Sequence        Resolution     Run  Same criterion  Diff unchanged  [+ Agent claimed done: hidden by default, toggle "Show agent-claimed-done (info only)"]
 0791f5b4  F R U U U R P   auto verified  3    yes (c3)        no              [Open item]
```
Verdict segments differ by pattern fill plus text label (the chart is a `FlowChart` consumer, Story 4.2.5; takeaway label example: "FAIL share fell from 41% to 28% over 5 weeks"); legend is text with the pattern swatch. Sequence letters are spelled out for screen readers via `aria-label` ("fail, rework, unverifiable..."). The streaks table (the "Deadlock" table in plan Story 4.2.2) is wrapped in `EstimatedValue`, row action "Open item" links to `?item=<id>`. Table scrolls in a labelled region (`tabindex=0`).
States: no verdicts "No review verdicts in this window"; no candidates "No review-fail streaks in this window (checked 12 items)" so empty is distinguishable from not-run; verdict query failed gives section error with retry while the deadlock table can still show if its own sub-status is OK (each shows its own error block); low sample on adjusted rate. Exit: drawer Escape; Open item leaves to the detail panel, back returns.

### S9. PR health

Rows: reopen-for-PR-fix count (episodes, raw reopens in parens), combined PR-fix dwell (median, p90, N; `pr_fix_dwell:combined`), nudge outcomes (steered / respawned / skipped_active, resolved within 24h), and the PR-fix trigger split (CI / conflict / mixed / review / feedback) for both counts and dwell. Until trigger capture exists (Story 5.2.3) the split is one bucket with the visible note "CI vs conflict split not recorded before <capture date>" and `pr_fix_cause:unavailable` in Data notes; after capture it shows the split for events since the capture date, flagged `pr_fix_cause:since_capture` (dwell: `pr_fix_dwell_split:since_capture`). Counts are drill buttons. Outcome counts carry `EstimatedValue` flagged `pr_fix_outcome:since_capture` with text "counts since <capture date>". Before Phase 5 capture exists, the outcomes block shows "Outcome tracking not yet recording" with no numbers (not zeros). Empty: "No PR fixes in this window". Error: section error with retry. Exit: drawer Escape.

### S10. Stuck reasons

Table: reason, current count (links to `/unfinished`, labelled), historical episodes, median/p90 time-in-stuck. When `stuck_history:lower_bound` is in `estimated[]`: `EstimatedValue` marker and text "history before <capture_since> is a lower bound". Reasons use the existing chip (text plus color). Empty: "Nothing stuck now; N episodes historically". Error: section error with retry. Exit: drawer Escape; `/unfinished` via browser back.

### S11. Anomalies and thrash

```
Item      Flip pair                   Episodes  Total events  Duration  Last seen    
d91477d5  ready <-> in_progress       8         ~87,400       48 d      (all-time view)   [Open item] [View events]
a90b99cb  pr_pending <-> in_progress  5         ~8,700        28 d      1 h ago
Excluded from aggregates: raw dwell and rework totals use collapsed episodes.
```
One row per item (episodes merge across gaps under 1h; a thrashing item still has several). Episodes and Total events are drill buttons (episode detail). "View events" opens the item timeline dialog (plan Story 4.2.6, `GetItemFlowTimeline`, collapsed spans only, at most 50). "Still active" is text-labelled when last-seen is within the poll horizon. Archived items stay in the table with an "archived" text chip (they are what the all-time banner count includes). Empty: "No thrash episodes in this window". Error: banner (S4) and this section both show error with retry. Exit: drawer Escape.

### S12. Data notes

`Collapsible`, closed by default, trigger is a button with `aria-expanded`. Contents: collapse rules, sources, estimated-metric list with explanation per entry, row counts, query ms, "Archived excluded from stages, included in throughput". The "N metrics estimated" header link opens it and moves focus to its heading. Error: if meta missing, shows "Methodology details unavailable" (never blank; plan Story 4.2.3 AC, criterion 40). Exit: toggle closed.

### S13. Drill-down drawer

```
+----------------------------------------------+
| Review-FAIL rework, last 30d, 12 items   [x] |
| [filter echo: window 30d, compare none]      |
| Title            Status      Reason   Duration|
| Fix login test   [review]    FAIL x3  2.1d  > |
| ...                                           |
| [Close]                                       |
+----------------------------------------------+
```
Flow: click a number, dialog opens (`role="dialog"` `aria-modal`, labelled by heading), focus moves to heading, trapped. Row links to `?item=<id>` (detail panel opens over the flow page, back returns to the drawer via `drill` URL param). Escape, X or Close dismisses, focus returns to the invoking button, `drill` param removed.
States: loading skeleton rows plus `aria-busy`; empty "No items match (the count may have changed since as-of 14:02) [Refresh]"; error "Could not load items [Retry] [Close]"; truncated list "Showing 100 of 240 items" with a link "Open full list in List view" when a filter mapping exists, otherwise a note; stale count mismatch (header count differs from rows) shows "Counts changed since 14:02; refresh". Invalid `drill` key in URL: drawer does not open, inline notice "Unknown drill target ignored", param removed. Exit: always present (Escape, X, Close, browser back). Plan Story 4.2.4 has an AC for every state above (criterion 38).

### S14. FlowChart wrapper contract

`figure` plus `figcaption`; inner `role="img"` with takeaway `aria-label`; adjacent hidden `<table>` with identical values and a "Show data table" toggle button for sighted users; series differ by pattern/marker plus direct text labels; theme tokens only; `prefers-reduced-motion` disables transitions. Empty data renders the figure with "No data in this window" text (not an empty box). Missing token or render exception: error boundary renders the data table only with note "Chart unavailable, showing data" so values remain reachable. Plan Story 4.1.3 has an AC for each state (criterion 39).

## 4. Flow exit-path and state audit

| Flow | Exit path | Error | Empty | Partial | Stale |
|---|---|---|---|---|---|
| S1 tabs | other tab / back | yes | n/a | n/a | n/a |
| S2 header | reversible controls | yes (fallback + banner) | n/a | yes | yes |
| S3 page states | retry / tabs | yes | yes | yes | yes |
| S4 banner | View / ignore | yes (unavailable variant) | hidden | yes | inherits dim |
| S5 tiles | drawer Esc / link | yes | yes | yes | yes |
| S6 funnel | drawer Esc | yes | yes | yes | yes |
| S7 causes | drawer Esc | yes | yes | yes | yes |
| S8 review | drawer Esc / open item | yes | yes | yes (sub-status) | yes |
| S9 PR | drawer Esc | yes | yes | yes | yes |
| S10 stuck | drawer Esc / link | yes | yes | yes | yes |
| S11 anomalies | drawer Esc | yes | yes | yes | yes |
| S12 notes | toggle | yes | n/a | yes | n/a |
| S13 drawer | Esc / X / Close | yes | yes | truncated | count-mismatch |
| S14 chart | n/a (embedded) | yes (fallback) | yes | n/a | inherits dim |

Gaps found: none in this table. Two items rely on assumptions flagged in section 7.

## 5. Non-interactive surfaces (condensed)

### S15. MCP JSON output (shared shape with the UI)

```json
{"as_of":"2026-11-05T14:02:11Z","window":{"start":"...","end":"..."},
 "estimated":["deadlock_candidates:heuristic","stuck_history:lower_bound"],
 "sections":{"review":{"status":"ok"},"pr":{"status":"failed","error":"query timeout"}},
 "row_counts":{"status_events":257988},"query_ms":412,
 "anomalies":{"total_count":35,"returned_count":10,"has_more":true,"truncated":false}}
```
Snake_case because the MCP adapter marshals the RPC response with `protojson` `UseProtoNames: true` (`FlowJSONOptions()`, plan Story 3.1.1). Tool inputs: `window_days` (1..365) or the boolean `all_time` (never a bare 0), `sections[]` and `compact` on the summary; `compare_flow_windows` takes day offsets or absolute `a_start/a_end/b_start/b_end` dates; `list_flow_drilldown.key` is a schema enum of the valid drill keys and a bad key returns the valid list; every summary carries `estimated_legend`. Default and `compact` response sizes are bounded by a budget set from the measured fixture payload times 1.25 (plan Story 3.2.1), not a guessed figure. Every number in the JSON has a `drill_key` beside it, and date bounds are half-open `[start, end)` (the tool description says "end is exclusive"). Tools: `get_flow_summary`, `compare_flow_windows`, `list_flow_drilldown`, `list_flow_deadlock_candidates`, `list_flow_thrash_episodes`, `get_item_flow_timeline` (one per RPC).
- Always include `as_of`, window, `estimated[]`, per-section `status` (the one enum `ok|partial|failed|skipped|empty`, see plan `SectionStatus`) so a failed section is never returned as zeros.
- Empty sections return `status:"empty"` with denominators, not omitted keys.
- Field names match the UI's hidden tables and drill keys so UI and MCP numbers cannot diverge.
- Errors use the existing MCP error shape with an actionable message and no stack traces.
- Collapsed thrash is flagged by an `anomalies` array, never silently merged into totals.
- Every top-N list carries `total_count`, `returned_count`, `has_more`, `truncated` so a clamped list (default 10, max 50) is never mistaken for the whole set; the deadlock list adds a `weak_filter` note.

### S16. Debug log line

`flow aggregate window=30d section=review rows=528 query_ms=41 cache=miss` at debug level (the plan's Observability Plan format), one line per section; failures at warn with the error and `status=failed`. Criteria: duration and row count present; no per-row logging; no item titles in logs.

## 6. UX acceptance criteria

Task completion
1. User answers "where do items spend time per stage" from the funnel with no scrolling beyond B on a 1080p viewport after the tiles (1 view, 0 clicks).
2. User reaches the items behind any displayed number in 1 click, and the item detail panel in 2 clicks.
3. User switches to a before/after comparison in 1 click (Compare radio) and reads a delta on each tile.
4. User identifies thrash items from the banner in 1 click (View) without scrolling manually.
5. User reaches Flow from the Backlog in 1 click on the tab.

State and error handling
6. First load shows skeletons; no tile, count or percentage displays "0" before the first response resolves.
7. A failed refresh after success keeps last data dimmed with the text `Showing data from <time> - refresh failed` and a Retry button.
8. A hard error with no data shows the server error text, a Retry button, and a link to Board.
9. A failed section shows an inline error with its own Reload button and every other section still renders.
10. An empty window shows "No items finished in the last N days" with a Widen button, and zero-failure cases display the denominator.
11. A denominator under 5 greys the percentage and shows "low sample".
12. An empty compare window shows "n/a (no data in previous period)", never infinity or NaN.
13. Estimated values show a visible marker and sr-only "estimated", and each is explained in Data notes.
14. Stuck history shows "history before <capture_since> is a lower bound" when applicable.
15. As-of time shows the server timestamp and relative age.
16. Outcome tracking not yet recorded renders "not yet recording", not zeros.
17. No dead ends: every error, empty, stale, and partial state offers at least one action (Retry, Widen, Back to Board, or navigate away).
18. Unknown URL `window` or `drill` values are ignored with a visible note and do not crash the page.

URL and navigation
19. Reloading a URL with `window`, `compare`, `drill` restores selectors and drawer; browser back restores the previous state.
20. Closing the drawer removes `drill` from the URL.

Accessibility (WCAG 2.2 AA)
21. Entire page is operable by keyboard alone; tab order follows reading order; no keyboard traps outside the drawer.
22. Drawer traps focus, closes on Escape, and returns focus to the invoking control (test mirrors `BacklogItemDetail.focusReturn.test.tsx`).
23. Window and compare selectors are RadioGroups with arrow-key navigation and a group label.
24. Every chart is a `figure` with `figcaption`, `role="img"` with a takeaway `aria-label`, and a same-data table reachable by screen reader.
25. No information is conveyed by color alone: statuses use text, verdicts use pattern plus label, deltas use arrow plus signed text, active tab uses underline.
26. Text contrast is at least 4.5:1 and chart marks and control boundaries at least 3:1 in light and dark themes (Axe plus manual check).
27. Interactive targets are at least 24x24 CSS px.
28. Tables have `<caption>`, `scope="col"/"row"`, and scrollable regions have `tabindex=0` and `aria-label`.
29. Refresh and window changes announce via `LiveRegion` once; background polls do not announce.
30. `prefers-reduced-motion` disables bar and chart transitions.
31. Headings are a single h1 with h2 per section; the jump list links move focus to the target heading.
32. Axe reports zero WCAG AA violations for populated, empty, error and partial states in both themes.
33. Layout reflows to 320 CSS px wide without horizontal page scroll (tables scroll inside labelled regions).

Content and quality
34. Percentages always accompany the raw count (`32/105 (30.5%)`).
35. Raw and adjusted review FAIL rates both display, adjusted as the primary value and raw as secondary, with the excluded categories named.
36. Every number rendered has an associated drill button (coverage test), except explicitly labelled non-drillable values (as-of, query ms).
37. E2E conventions satisfied: `// @feature backlog:flow` header, no `waitForTimeout`, `data-testid`/ARIA locators only.

Triad repair 1 additions
38. Drill-down drawer states each offer an action: loading, error with Retry, empty with count mismatch and Refresh, "Counts changed" mismatch, truncated "Showing 100 of N items", and an invalid `drill` key ignored with a note.
39. `FlowChart` renders "No data in this window" when empty, falls back to the data table with a note on error, and has a "Show data table" toggle.
40. The "Anomaly data unavailable [Reload]" banner variant and the "Methodology details unavailable" Data notes variant exist, so absence is never misread as "nothing found".
41. Sortable table headers (Causes) set `aria-sort` on the active column and toggle with Enter/Space.
42. The page is stale when more than 2x `FLOW_POLL_INTERVAL_MS` (60s, so 120s) has passed since the last successful fetch; it then dims, says "Stale" and shows the Retry banner; polling pauses while the tab is hidden.
43. Sections F (Stuck reasons) and G (Anomalies) are collapsed by default and expand on jump-list or banner activation, or when failed or partial.
44. On the first live render the owner answers the four key questions (healthy? where does it leak? why? which items?) without SQL or docs in under 5 minutes (manual, plan Story 6.1.1).

Final triad patch additions
45. All time over the row ceiling shows the "pick a shorter window" state with 7d/30d/90d buttons and no Retry, not the generic error (plan Story 4.1.2).
46. Compare is disabled (`aria-disabled`, hint "Compare needs a 7, 30 or 90 day window") while All time is selected, and a URL combining `window=all` with `compare=prev` loads with compare `none` (plan Story 4.1.2).

Total: 46 criteria (37 original, 7 added in Triad repair 1, 2 added in the Final triad patch).

## 7. Assumptions and open items
- Numbers quoted in mockups are live-data illustrations from the 2026-11-05 snapshot (verdict totals 528 = FAIL 186 + PARTIAL 82 + PASS 150 + UNVERIFIABLE 110; adjusted FAIL 88/355; 426 `review>in_progress`; thrash 35 items, 103 episodes, 61 after merge, all causes), not fixed expectations; throughput, lead time, FPY, open-stuck and compare values are placeholders. The debug-log example `rows=528` for the review section equals the verdict total.
- Per-section retry re-issues the single read RPC (plan has one sectioned response); a section-scoped RPC is not assumed. If the backend adds one, retry becomes cheaper but UX is unchanged.
- Drawer truncation cap (100 rows) and the "Open full list in List view" link depend on whether a List filter exists for each drill key; otherwise only the truncation note is shown.
- "Still active" thrash label depends on `last_seen` being returned (in plan Story 4.2.3 columns).
- The Trends charts (verdict mix, sparklines) depend on `trends` in the sectioned response (plan Stories 1.4.2, 3.1.1, 4.2.5); if absent the sparkline cell shows "No trend data" and the verdict-mix figure shows its table only.
- Release-annotation markers on trends are deferred (needs a change-date source).
- `dataviz` skill files were absent when research ran; chart rules here follow standard accessible-dataviz practice.
