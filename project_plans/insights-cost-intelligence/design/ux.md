# UX Design: insights-cost-intelligence

Source docs: `../requirements.md`, `../research/ux.md`, `../implementation/plan.md`.
This artifact designs the user-facing surfaces for the five in-scope workstreams
(Epics 1.1–1.5). It does not re-derive decisions already settled by research/ux.md
(severity palette reuse, "findings own the verdict" placement, estimated-value
marker pattern) — it applies them concretely to each surface's layout and flow.

---

## Surface inventory

| # | Surface | Interactive? | Epic |
|---|---|---|---|
| 1 | Findings panel (list of waste-finding cards) | Yes — clickable actions, keyboard nav | 1.1 |
| 2 | Per-tool cost column (Tools Breakdown table, inside session detail) | No — read-only data display | 1.2 |
| 3 | Activity cost breakdown table (dashboard-level) | No — read-only data display | 1.2 |
| 4 | Sessions table — extended sort (Duration, Cost/Msg, Cache ROI, Waste Score) | Yes — sortable headers, existing search | 1.3 |
| 5 | Session drill-down: `/insights/session/[sessionId]` route | Yes — full page, focus management, not-found state | 1.4 |
| 6 | Session detail modal (`SessionDetailDrawer`) — quick-peek, now with focus trap fix | Yes — dialog semantics, keyboard trap | 1.4 |
| 7 | `WatchInsights` live-patch (no new visible UI — existing "Live" indicator now reflects real per-session updates) | No — background behavior | 1.5 |

Surfaces 4, 5, and 6 all involve the same underlying content (session detail) with two
different presentation shells (route vs. modal) sharing `SessionDetailContent` — designed
together in one section below since their interaction model is the point.

---

## Part A — Condensed surfaces (non-interactive)

### A1. Per-tool cost column (Tools Breakdown table)

Representative output (inside the existing `SessionDetailDrawer` / `SessionDetailContent` Tools Breakdown table):

```
Tool     Calls   Cost
Read     50      $0.02
Bash     5       ~$5.00 ⓘ
Grep     5       ~$5.00 ⓘ
```
`~$5.00 ⓘ` renders via the shared `EstimatedValue` component: muted-weight `~` prefix,
`aria-describedby` pointing at hidden text — "Attributed at tool-type level: this turn's
full cost is counted once per distinct tool used in it, so totals across tools may exceed
the session's real cost when tools co-occur in the same turn." `Read`'s cost has no marker
because it never co-occurred with another tool in the same turn (`costMayDoubleCount === false`).

**Acceptance criteria:**
- A tool row with `costMayDoubleCount === true` always shows the `~` marker and tooltip text; one with `false` never shows it.
- The marker's tooltip text is reachable via `aria-describedby`, not only a mouse-hover `title` (screen-reader users get the same explanation).
- Cost column never renders `$0.00` for a tool that had no priced turns — renders `—` instead (abstain, per requirements.md's "abstain rather than guess").
- Column order/sort of the existing "most-called" table is unchanged by adding the cost column (call-count-desc stays the primary order).

### A2. Activity cost breakdown table (dashboard level)

Representative output (`ActivityBreakdownTable`, same visual pattern as the existing `TopNTable`):

```
Activity      Est. Cost   Sessions
Feature Dev   ~$41.20 ⓘ   12
Debugging     ~$18.75 ⓘ   6
Refactoring   ~$9.40 ⓘ    3
Exploratory   ~$3.10 ⓘ    2
Other         ~$1.05 ⓘ    1
```
Every row carries the `EstimatedValue` marker — activity classification is a heuristic
(skill-name match, then tool-ratio fallback) for every row, never just some.

**Acceptance criteria:**
- Rows sort cost-descending, matching the existing `TopNTable` convention.
- Every cost cell in this table shows the `~` marker (unlike the per-tool table, there is no "not double-counted" case here — activity classification is always heuristic).
- Table mounts in the existing "Top Usage" `grid2` section, not as a new full-width block — keeps the "findings own the verdict, charts/tables stay descriptive" separation from research/ux.md §1.
- Empty state ("no sessions with activity data in range") reuses the existing `emptyState` token rather than an empty table shell.

### A3. Loading skeleton (all new panel/table content)

Representative output: reuse `InsightsDashboardSkeleton.tsx`'s existing shimmer-row pattern for the Findings panel and Activity breakdown table while `GetInsightsSummary` is in flight — no new skeleton component invented.

**Acceptance criteria:**
- Skeleton row count for the findings panel is a fixed small number (e.g. 3), not zero-height, so the panel doesn't visually collapse/expand between loading and loaded.
- Skeleton disappears the moment `loading` flips false, whether the result is zero findings, N findings, or an error — never lingers alongside real content or an error box.

### A4. `WatchInsights` live-patch (background, no new UI)

No new visible surface — the existing pulsing "Live" indicator (`liveIndicator`, `InsightsDashboard.css.ts`) already pairs color with the word "Live" via `aria-label` (per research/ux.md §3). This epic makes the indicator's implied promise ("data updates live") true for per-session rows for the first time.

**Acceptance criteria:**
- A session row's cost/tokens update in place within one `WatchInsights` event of a JSONL file changing, with no visible page flash/full-table re-render.
- The Findings panel does **not** recompute/re-render on every live-patch tick (research/ux.md §2 — findings compute on load/explicit refresh only); only `SessionsTable` row data and `SummaryCards` patch live.

---

## Part B — Interactive surfaces (full treatment)

## B1. Findings panel

### Wireframe

```
┌─ Insights ──────────────────────────────────────────────────────────────┐
│  [Time Range Filter]                                    ● Live          │
│                                                                          │
│  ┌─ Waste Findings ───────────────────────────────────── role="list" ─┐ │
│  │ ┌────────────────────────────────────────────────────────────────┐ │ │
│  │ │ [Critical] Cache-hit floor breach                               │ │ │
│  │ │ Session a1b2c3d4 · 9% hit rate vs. 40% floor · ~$4.20 wasted    │ │ │
│  │ │                                            [View session →]    │ │ │
│  │ └────────────────────────────────────────────────────────────────┘ │ │
│  │ ┌────────────────────────────────────────────────────────────────┐ │ │
│  │ │ [Warning] Model-switch cache-bust                               │ │ │
│  │ │ Session e5f6a7b8 · turn 7 switched sonnet→opus · ~$1.35 wasted  │ │ │
│  │ │                                            [View session →]    │ │ │
│  │ └────────────────────────────────────────────────────────────────┘ │ │
│  │ ...                                                                  │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  [SummaryCards row — verdict-free, unchanged]                          │
│  [DailySpendChart]  [ModelBreakdownChart]   ← charts stay verdict-free  │
│  [SessionsTable]                                                        │
└──────────────────────────────────────────────────────────────────────────┘
```

Placed as its own `section`, above the existing `grid2` charts row (per research/ux.md §1:
findings own the verdict, charts stay descriptive; a full-width banner is too disruptive
for 5+ simultaneous findings, and row-only shading has no room for the "why").

### Four states

```
LOADING            COMPUTED — EMPTY          COMPUTED — UNPRICED         COMPUTED — ERROR
┌───────────┐      ┌───────────────────┐    ┌───────────────────────┐  ┌───────────────────────────┐
│ ▓▓▓▓▓▓▓▓  │      │ ✓ No waste patterns│    │ ⓘ 42 sessions could    │  │  ⚠ Couldn't compute        │
│ ▓▓▓▓▓▓    │      │   detected         │    │   not be evaluated     │  │    findings                │
│ ▓▓▓▓▓▓▓▓▓ │      │ (genuinely clean — │    │   (unpriced model)     │  │  <error detail text>       │
│ (skeleton)│      │  not "we don't     │    │ (not "genuinely clean" │  │  [Retry]                   │
│           │      │  know")            │    │  — we couldn't check)  │  │                            │
└───────────┘      └───────────────────┘    └───────────────────────┘  └───────────────────────────┘
   gray/shimmer      success-tinted, static    info-tinted, static        errorBox tokens (red)
```

The **unpriced** state (pre-mortem Failure #1) exists because `findings.length === 0` is
ambiguous on its own — it's true both when every session was checked and came up clean,
and when every detector abstained because none of the operator's models are in
`PricingTable` (Story 1.1.6's abstain-rather-than-guess discipline). Rendering the same
"No waste patterns detected" text for both would silently hide a dashboard-wide pricing
gap behind good news. The frontend distinguishes them by checking whether any in-range
session has `unpricedModels.length > 0`: if so, and `findings.length === 0`, it shows the
unpriced count instead of the clean message — checked *before* falling back to "genuinely
clean."

### Interaction flow

1. Dashboard loads → `GetInsightsSummary` in flight → panel shows loading skeleton (3 placeholder rows).
2. Response returns:
   - `findings.length === 0`, no request error, and at least one in-range session has `unpricedModels.length > 0` → "N sessions could not be evaluated (unpriced model)" (info-tinted, distinct from both the clean and error states) — checked *before* the clean-state branch below, so an all-unpriced dashboard is never mistaken for a clean one.
   - `findings.length === 0`, no request error, and no in-range session is unpriced → "No waste patterns detected" (styled with the existing success-adjacent `emptyState` token, explicitly worded as good news, not blank).
   - Request itself errored → panel shows `errorBox`-styled message with the actual error text and a `[Retry]` button that re-triggers the fetch — never a silently empty list standing in for a failure.
   - `findings.length > 0` → ranked list, one card per finding, sorted by dollar impact desc (backend already sorts/caps at 20).
3. User reads a card's severity badge (text label always rendered — "Critical"/"Warning" — never a bare colored dot) + one-line message + `~$` dollar impact.
4. User activates the card's action (click or Enter/Space on the focused `<button>`/`<a>`) — every finding is a single-session finding (one `WasteFinding` per session, per `implementation/plan.md`'s `WasteFinding` proto), so the action always navigates directly to `/insights/session/[sessionId]` (one hop, per research/ux.md §2's "find the driver fast" requirement). Interim state before Epic 1.4 ships: opens the existing quick-peek modal instead (plan Story 1.1.6b), swapped to the route link once 1.4.4c lands.
5. Findings do **not** recompute on every `WatchInsights` live-patch tick — only on page load or an explicit user-triggered refresh (avoids severity chips visibly reordering while someone is mid-read).

### Error / edge cases

| Condition | What the user sees | Exit path |
|---|---|---|
| `GetInsightsSummary` fails outright | `errorBox`-styled panel: "Couldn't compute findings" + underlying error text + `[Retry]` button | Retry re-fetches; dashboard's own top-level error state (unchanged) covers total failure |
| Zero findings, request succeeded, at least one session unpriced (pre-mortem Failure #1) | "N sessions could not be evaluated (unpriced model)" (info-styled, distinct from both the clean and error states) | Operator adds a `PricingTable` entry for the model, or accepts the gap — no dead end, just an honest "we couldn't check" |
| Zero findings, request succeeded, no session unpriced | "No waste patterns detected" (success-styled, not gray/blank) | None needed — this is a resting state, not an error |
| A single detector panics server-side on one session | That session silently contributes zero findings (server-side isolation, Story 1.1.3c); other sessions' findings still render normally — no visible difference to the user beyond one fewer expected finding | N/A — invisible by design; server logs the panic (`slog.Warn`) for the operator to find later, not surfaced as a UI error since it's a partial, not total, failure |
| A finding names a session that's since been deleted/rotated out of the JSONL window | Clicking "View session" leads to the route's own "Session not found" state (see B3) | The route's not-found state offers "Back to dashboard" |

### UX acceptance criteria

- User can identify the single most expensive waste finding and act on it (jump to that session) in **2 clicks**: (1) load dashboard — findings visible with no click, (2) click "View session."
- Loading, computed-empty, unpriced, and error states are visually **and textually** distinct (no two of the four ever render identical markup) — testable by asserting four different rendered strings in `FindingsPanel.test.tsx`. In particular, an all-unpriced-model dashboard (`findings.length === 0`, every session's `unpricedModels.length > 0`) must render "N sessions could not be evaluated (unpriced model)," never the "No waste patterns detected" clean-state text (pre-mortem Failure #1).
- Every finding card's severity is conveyed by a rendered text label, not color alone — testable via DOM text assertion, no reliance on computed CSS color.
- Every finding card's primary action is a real `<button>`/`<a>` operable via Tab + Enter/Space — no `<div onClick>`.
- Error state's message is specific ("Couldn't compute findings: <error>"), not a generic "Something went wrong," and offers a `[Retry]` action — no dead end.
- The panel container uses `role="list"`/`role="listitem"` (or native `<ul>`/`<li>`) so a screen reader announces item count.
- Each finding card has an `aria-label` (or accessible name from its rendered text) summarizing severity + subject + dollar impact in one string, mirroring `ProjectedCostCard.tsx`'s pattern.
- Color contrast of severity badge text against its background is ≥ 4.5:1 for all four tiers (success/warning/error/critical) — verified against `theme-contract.css.ts`'s existing token values (already-shipped tokens are the source of truth, not new ad hoc colors).

---

## B2. Sessions table — extended sort/search

### Wireframe (header row, new columns appended)

```
┌────────┬────────┬───────┬────────┬──────┬──────────┬───────────┬───────────┬────────────┐
│ Path ▲ │ Model  │ Input │ Output │ Cost │ Duration │ Cost/Msg  │ Cache ROI │ Waste Score│
├────────┼────────┼───────┼────────┼──────┼──────────┼───────────┼───────────┼────────────┤
│ ~/repo │ sonnet │ 1.2M  │ 340K   │ $4.10│ 12m 04s  │ $0.34     │ +$0.82    │ 22         │
│ ~/svc  │ opus   │ 3.9M  │ 900K   │ ~$12 │ 41m 55s  │ $0.60     │  -$0.42   │ 71         │
│ ~/x    │ ???    │ 500K  │ 90K    │  —   │  8m 10s  │ Not eval. │  —        │ Not eval.  │
└────────┴────────┴───────┴────────┴──────┴──────────┴───────────┴───────────┴────────────┘
        [search: "svc"                                              ] → filters to row 2 only,
                                                                        sort still applies within
                                                                        the filtered set
```

Each header cell follows the existing `sortableHeaderCell` pattern (`aria-sort`
ascending/descending/none, click to cycle) — no new header markup invented for the
four new columns.

### Interaction flow

1. User clicks "Waste Score" header → `aria-sort="descending"` (first click; matches existing convention) → table re-sorts client-side over the currently-filtered (search-matched) set only, not the full 600-session set — search and sort compose, they don't reset each other (Story 1.3.4's explicit coexistence guarantee).
2. User types in the existing search box → Fuse.js filters the in-memory set as today; the active sort column/direction is preserved and reapplied to the new filtered result.
3. User clicks a header a second time → direction flips (`ascending`); a third click (if the existing 3-state cycle includes "none") returns to unsorted/default order — unchanged from current behavior, just extended to the 4 new columns.

### Missing/undefined-value handling (three-state distinction, per column)

| Column | "Sorts last" bucket | Cell text when in that bucket |
|---|---|---|
| Duration | none — missing timestamp sorts at its natural `0` position (not treated as "bad") | `—` (timestamp genuinely absent) |
| Cost/Msg | `messageCount === 0` | `Not evaluated` (division would be undefined, not zero) |
| Cache ROI | `unpricedModels.length > 0` | `—` (matches the existing unpriced-cost badge convention) — negative values (e.g. `-$0.42`) sort and render normally, signed, never treated as missing |
| Waste Score | `unpricedModels.length > 0` **or** `wasteScore === undefined` (two distinct causes, same sort bucket) | `Not evaluated` (too few turns to score) vs. `—` (unpriced) — two different strings for two different reasons, even though they sort identically |

### Error / edge cases

| Condition | What the user sees | Exit path |
|---|---|---|
| Cost/Msg on a 0-message session | "Not evaluated" text, sorts to the end regardless of direction | N/A — not an error, a defined non-value |
| Cache ROI on an unpriced model | `—` badge (same visual treatment as the existing unpriced Cost column) | N/A |
| Waste Score never evaluated (too few turns) vs. unpriced | Two distinct strings ("Not evaluated" vs. "—") so a user scanning the column doesn't conflate "we didn't look" with "we can't price it" | N/A |
| Search yields 0 rows after typing | Existing "no sessions match" empty state (unchanged) | Clear-search affordance (existing) |

### UX acceptance criteria

- Every new sortable column exposes correct `aria-sort` state changes on click — testable exactly like the existing `SessionsTable.test.tsx:113,129` assertions, replicated per new column.
- No comparator ever divides by a value that can be `0` without a guard producing a defined sort-last bucket instead of `NaN`/`Infinity` — testable via the 0-message and unpriced-model fixture cases already specified in the plan.
- Negative Cache ROI values are visually distinguishable from "no ROI" (signed `-$0.42` text vs. a `—` badge) without relying on color alone — testable by asserting rendered text content, not computed style.
- Sort and search compose: changing sort column never changes which rows are shown, and searching never changes the current sort order — testable with the plan's 600-row / 3-match fixture (Story 1.3.4).
- A user can locate the single worst-waste-score session in **2 interactions**: click "Waste Score" header once (descending is first click, since higher waste score is the "interesting" direction) — no need to also reverse text search.

---

## B3. Session drill-down — route vs. modal

The modal (`SessionDetailDrawer`, existing) and the new route
(`/insights/session/[sessionId]`) render the **same** `SessionDetailContent` — this
section designs their relationship, not two independent UIs.

### Flow diagram

```
Sessions table row click
        │
        ▼
┌───────────────────────────────┐        click "Open full page ↗"        ┌────────────────────────────────┐
│   Modal (quick-peek)          │ ───────────────────────────────────►   │  Route: /insights/session/{id} │
│   role="dialog", aria-modal   │                                        │  full page, own URL, bookmarkable│
│   Esc to close                │ ◄─────────────────────────────────── │  Back / browser-back returns    │
│   FOCUS: on open → close btn  │        (no equivalent "collapse to    │  to wherever the user came from │
│   on close → restores trigger │         modal" action — one-way is    │  FOCUS: heading gets initial     │
│   (NEW this project — was     │         fine, per plan Story 1.4.4)   │  focus + tabIndex=-1 on mount    │
│   entirely missing before)    │                                        │  (NEW — no native dialog role     │
└───────────────────────────────┘                                        │   to lean on for a route)        │
                                                                          └────────────────────────────────┘
        ▲
        │ direct navigation / bookmark / shared link / browser refresh
        │ (bypasses the modal entirely — the whole point of "deep-linkable")
        │
   URL bar / bookmark
```

### Interaction flow — modal (quick-peek)

1. User clicks anywhere on a sessions-table row → `onSessionClick` fires (unchanged) → modal opens.
2. **New this project:** on mount, focus moves programmatically to the modal's close button (`closeButtonRef.current.focus()`); the element that had focus before opening is captured and restored to it when the modal closes. This closes the pre-existing gap research/ux.md flagged: today's `SessionDetailDrawer` has `role="dialog"`/`aria-modal="true"`/Escape-to-close but zero initial-focus or focus-restoration code.
3. User reads `SessionDetailContent` (metadata, turn table, tools table, skill activations — unchanged rendering, now shared with the route).
4. User either presses Escape (closes, focus restores to the row/trigger) or clicks "Open full page ↗" (navigates to the route) or clicks the close button.

### Interaction flow — route (full page)

1. Arrival via any of: clicking "Open full page ↗" in the modal, clicking a single-session finding card, a bookmark, a shared link, or a browser refresh on that URL.
2. Route's `page.tsx` awaits the `sessionId` param, renders `SessionDetailPageClient`, which calls `GetInsightsSummary({sessionIdFilter, conversationIdFilter, includeOrphans: true})` — filter-independent of the dashboard's global date range (a bookmarked link must resolve regardless of what date range was active when it was created).
3. **New this project:** on mount, focus moves to the page's `<h1>`-equivalent heading (`ref` + `tabIndex={-1}` + `.focus()`) — the only signal a screen reader gets that new content loaded, since a route has no native `role="dialog"` semantics to lean on.
4. Three outcomes:
   - Session found → renders `SessionDetailContent` with the fetched summary + turn timeline.
   - Session not found (deleted, rotated out, or a stale/mistyped link) → explicit "Session not found" message, not a blank page or crash.
   - Fetch error → error state with retry, consistent with the findings panel's error-state language (same visual token, `errorBox`).

### Wireframe — route page

```
┌─ /insights/session/a1b2c3d4 ─────────────────────────────────────────┐
│  ← Back to dashboard                                                  │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ # Session a1b2c3d4                    ← receives focus on mount │  │
│  │   (tabIndex=-1, programmatically focused)                       │  │
│  └────────────────────────────────────────────────────────────────┘  │
│  [SessionDetailContent — same component the modal renders]           │
│    Metadata │ Per-turn table │ Tools Breakdown │ Skill Activations    │
└────────────────────────────────────────────────────────────────────────┘

NOT FOUND STATE:
┌─ /insights/session/deleted-xyz ───────────────────────────────────────┐
│  ← Back to dashboard                                                  │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │  Session not found                                              │  │
│  │  This session may have been deleted or rotated out of the       │  │
│  │  tracked window.                                                │  │
│  │  [← Back to dashboard]                                          │  │
│  └────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────┘
```

### Error / edge cases

| Condition | What the user sees | Exit path |
|---|---|---|
| Cold direct navigation, no parent dashboard state | Page fetches independently (no reliance on any dashboard-provided context/state) and renders normally | N/A — this is the whole point of "deep-linkable" |
| `sessionId` matches nothing in the response | Explicit "Session not found" text + reason hint (deleted/rotated) | "← Back to dashboard" link |
| Fetch itself errors (network/backend) | `errorBox`-styled error message with retry, same visual language as the findings panel's error state | Retry button; "← Back to dashboard" link always present regardless |
| Orphan session (`sessionId=""`, only a `conversationId`) | Route resolves via `conversationId` fallback (`session.sessionId || session.conversationId`) so "Open full page" never produces a link to a bare `/insights/session/` | N/A — a real link is always constructible for any row the table can show |
| Modal opened, then user hits Escape mid-focus-trap | Modal closes, focus restores to the triggering row element (not lost to `<body>`) | N/A |

### UX acceptance criteria

- **No dead ends**: every state of the route (found, not-found, error) has a visible, reachable "Back to dashboard" exit — testable by asserting the link/button exists in all three states' rendered output.
- Cold direct navigation to the route (no client-side navigation history) renders correctly — testable by mounting `SessionDetailPageClient` standalone with no parent context, per plan Story 1.4.3's acceptance criteria.
- Modal open → close button receives focus within the same render pass (no visible flash before focus lands); modal close → focus returns to the exact element that had focus before opening — testable via `document.activeElement` assertions before/after.
- Route mount moves focus to the heading (`tabIndex={-1}` + `.focus()`) — testable via `document.activeElement === headingRef.current` after mount.
- The "Open full page ↗" link's `href` never resolves to `/insights/session/` with an empty segment, for any session including orphans — testable with the plan's orphan fixture (`sessionId="" `, `conversationId="conv-999"`) asserting the resolved `href`.
- A user can go from a sessions-table row to the full bookmarkable page in **2 clicks**: (1) click row (opens modal), (2) click "Open full page ↗." (Directly bookmarking/sharing a URL, once known, is 0 clicks — the whole point of the route.)
- The route and the modal are visually/structurally consistent for identical data (verified by the same `SessionDetailContent` component rendering in both — no separate "route looks different from modal" drift is acceptable per research/pitfalls.md §4).
- Focus-trap parity: since the modal is now held to the same a11y bar the route was designed for, Tab from the last focusable element inside the modal cycles back to the first one (a true focus trap while it's open) rather than escaping to page content behind it.

---

## Cross-cutting UX acceptance criteria (apply to all surfaces above)

1. **No dead ends** — every error state (findings panel computation failure, route fetch error, route not-found, sessions-table empty search) renders a visible next action (retry, back-to-dashboard, or clear-search); none of the four error/edge states above render as a bare blank area.
2. **Estimated vs. measured is always marked** — every per-tool cost, activity cost, cache-ROI, and waste-score value that is a heuristic/model output (not a direct JSONL-derived count) carries the shared `EstimatedValue` `~` marker with an `aria-describedby` explanation, using one shared component across all three new surfaces (per-tool table, activity table, sessions-table cells) — never three different ad hoc treatments.
3. **Color is never the only signal** — severity badges (findings panel), signed Cache ROI, and the estimated-value marker all pair color with rendered text; verified by asserting text content independent of computed CSS.
4. **Keyboard operability** — every clickable affordance introduced by this feature (finding card action, sortable column header, "Open full page" link, "Back to dashboard" link, retry button) is a real focusable element reachable by Tab and operable by Enter/Space — no custom-styled `<div onClick>`.
5. **Color contrast** — all new text/background pairings (severity badges, estimated-value marker, error/empty states) reuse existing `theme-contract.css.ts` tokens rather than introducing new colors, so contrast stays at the ≥4.5:1 ratio those tokens are already validated at; no new ad hoc hex value is introduced by this feature.
6. **Multi-state distinction wherever "empty" is ambiguous** — the findings panel (loading / computed-empty / unpriced / error — 4 states, per pre-mortem Failure #1: "couldn't be evaluated" must never render as "genuinely clean") and the waste-score table cell (not-evaluated / unpriced / real value — 3 states) each render textually distinct outcomes, never collapsing two different "nothing to show" reasons into the same blank appearance.
