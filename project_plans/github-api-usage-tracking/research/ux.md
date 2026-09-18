# UX Research: GitHub API Usage Tracking Panel

Research for the web UI panel added by `github-api-usage-tracking` (see
`../requirements.md`). Scope: quota remaining/limit per resource, request
volume over time with a time-range selector, breakdown by source/poller, and
a user-configurable warn threshold.

## 1. House style to follow: `ApprovalAnalyticsPanel.tsx`

Requirements.md explicitly names this component as the pattern to follow. Read in full
(`web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`, 931 lines) plus its
stylesheet (`ApprovalAnalyticsPanel.css.ts`, 697 lines). Conventions the new panel must match:

- **No charting library.** All "charts" are hand-rolled `<div>` bars driven by
  vanilla-extract classes — a `Bar` component (single-series, width = `value/max*100%`)
  and a `StackedBar` component (multi-segment composition, e.g. allow/deny/manual).
  Both are marked `aria-hidden="true"` on the bar `<div>` itself — the accessible
  content lives in adjacent `<td>` text (the raw number + a `pctLabel` percentage
  span), not in the bar. **This means there is no chart library dependency to add for
  the new panel** — a time-series "chart" should follow the same pattern: an inline
  bar-per-bucket row, or a simple `<svg>` sparkline if a true line/area shape is
  wanted, not a Recharts/D3/Chart.js import.
- **Layout shell**: outer `panel` div (card background, border, 12px radius, 20px
  padding, `flex-direction: column`, `gap: 20`). Inside: `titleRow` (title + controls,
  `justify-content: space-between`, wraps), then a `cards` grid of stat tiles
  (`repeat(auto-fill, minmax(130px, 1fr))`), then one or more `tableSection` blocks
  each with a `sectionTitle` (`<h3>`) and a `tableWrapper` (bordered, `overflow-x: auto`)
  containing a plain `<table>`.
- **Stat tiles** (`card`/`cardValue`/`cardLabel`/`cardSub`): big number (28px bold),
  label underneath (12px), optional sub-line (11px, 70% opacity) for supporting context
  like "1234/day avg". Semantic color variants exist via modifier classes
  (`cardAllow`/`cardDeny`/`cardManual` — success/error/warning border+bg+value color) —
  the quota panel should reuse this variant pattern for "OK / near threshold / exhausted"
  tiles rather than inventing new classes.
- **Time-range / window selector**: a `role="group" aria-label="Time window"` wrapper
  (`windowSelector`) containing plain `<button>` elements per option, each with
  `aria-pressed={selected}` and an active-state class swap (`windowBtnActive`) rather
  than a native `<select>`. `ApprovalAnalyticsPanel` hardcodes `[7, 14, 30, 90]` days.
  The `UnifiedActivityTable`'s filter chips reuse the exact same `windowSelector`/`windowBtn`
  classes for a *different* kind of toggle group (categorical filter, not time range) —
  confirms this is the house "segmented control" pattern generally, not window-specific.
- **Loading / empty / error states**: `error` is a `role="alert"` banner above the
  content with an inline "Retry" button — rendered *in addition to* stale content, not
  instead of it (renders alongside `cards`/tables using last-known `summary`/`dailyBuckets`
  if present). Loading is only its own full-replacement state when `loading && data.length
  === 0` (skeleton-free — just a text string "Loading analytics…"). Empty state
  (`empty`/`emptyHint`) is two-line: a bold statement ("No data for the last N days.")
  plus a smaller hint line explaining *why* there's no data and what would populate it
  ("Analytics are recorded when Claude Code sends hook requests."). **The new panel should
  write an equivalent hint** (e.g. "Recorded automatically as GitHub API requests are made")
  for its fresh-install empty state.
- **Refresh control**: a small icon-only button (⟳ spinning glyph while loading, ↻
  otherwise) with `aria-label="Refresh analytics"`, disabled during load — not a full
  page-reload pattern.
- **data-testid usage is sparse and targeted** — only present on elements that need
  disambiguation beyond ARIA role/name for e2e locators, e.g.
  `data-testid={`suggest-rule-tool-${r.name}`}` / `suggest-rule-program-${r.name}`
  (dynamic per-row action buttons where many identical-looking buttons exist in one
  table). Static, singular controls (refresh button, window buttons, retry button) rely on
  `aria-label`/`aria-pressed`/visible text and role instead of `data-testid` — per
  `.claude/rules/e2e-test-conventions.md` ("data-testid or ARIA roles only"), that's
  consistent: testid is the fallback for cases ARIA/role/text can't uniquely identify,
  not the default. **Recommendation for the new panel**: give the resource stat tiles
  (core/search/graphql) `data-testid="quota-tile-{resource}"` since they're visually
  distinguished only by label text repeated in a grid (an ARIA role/name locator would
  work too — `getByRole` scoped by tile label is arguably sufficient and preferred per the
  house pattern above; use `data-testid` only if a resource name collides with something
  else on the page). The time-range selector and warn-threshold input should follow
  `ApprovalAnalyticsPanel`'s existing pattern exactly: `role="group" aria-label="..."`
  + `aria-pressed` per button, no `data-testid` needed.
- **Color-only encoding is avoided textually but not visually**: allow/deny/manual bars
  and stat tiles use color (success/error/warning) *plus* always-present text labels
  and percentages alongside — never color as the sole signal. The quota gauge must do
  the same (see §3 accessibility below): pair the color band with a text readout
  ("342 / 5000 remaining (6.8%)"), not a bare colored bar.

## 2. Theme tokens available for a quota gauge/progress indicator

`web-app/src/styles/theme.css.ts` (re-exports `vars`/`breakpoints`/`zIndex` from
`theme-contract.css.ts`) defines per-theme tokens across 6 themes (light, dark, matrix,
cyberpunk77, wh40k, clean). Relevant tokens for a 3-state (OK/warning/critical) quota
gauge, all with WCAG-contrast-checked pairs already documented in code comments:

| State | Fill color | Background | Text |
|---|---|---|---|
| OK / plenty remaining | `vars.color.success` | `vars.color.successBg` | `vars.color.successText` |
| Near warn threshold | `vars.color.warning` | `vars.color.warningBg` | `vars.color.warningText` |
| Exhausted / critical | `vars.color.error` (or `vars.color.critical` for a more severe tier) | `vars.color.errorBg` (`vars.color.criticalBg`) | `vars.color.errorText` (`vars.color.criticalText`) |

`ApprovalAnalyticsPanel.css.ts` already has the exact structural primitive needed —
`barTrack` (8px height, `panelBgSecondary` background, 4px radius, `overflow: hidden`)
+ `barFill` (`height: 100%`, `border-radius: 4px`, `transition: width 0.3s ease`,
`minWidth: 2px`) — a percent-width gauge is just `barFill` with a state-conditional
background color (`vars.color.success`/`warning`/`error`) swapped in via a modifier
class, exactly like `cardAllow`/`cardDeny`/`cardManual` swap `card`'s border/bg/value
color. **No new token is needed** for a basic 3-state gauge; reuse `success`/`warning`/
`error` + their `*Bg`/`*Text` pairs. If a 4th "critical" tier is wanted (e.g.
distinguishing "below warn threshold" from "actually exhausted, 0 remaining"),
`vars.color.critical`/`criticalBg`/`criticalText` already exists and is unused
elsewhere in this panel family — free to adopt without adding new hex values, honoring
`.claude/rules/css-architecture.md`'s no-hardcoded-hex rule.

No existing `zIndex` slot is needed for this panel (it's inline content, not an
overlay/modal) — skip `zIndex.*` entirely rather than inventing a magic number.

## 3. Comparable UX patterns — API quota/rate-limit dashboards

General industry pattern (WebSearch, Aug 2026 — Axway/Speakeasy/Gravitee/getknit.dev
API-rate-limiting guides; no single canonical example dominates because most API
providers' own dashboards, e.g. GitHub's own account settings, Stripe's dashboard, and
observability tools like Grafana/Datadog quota panels, converge on the same shape):

- **Real-time remaining/limit as the primary number**, framed as "used of total" or
  "X remaining" rather than raw percentage alone — percentage is a secondary
  annotation, not the headline (matches `ApprovalAnalyticsPanel`'s stat-tile pattern:
  big number + small label + smaller sub-line).
- **A visible reset/refill time** is standard alongside remaining/limit (GitHub's own
  `X-RateLimit-Reset` header is a Unix timestamp for exactly this) — the panel should
  show "resets in Xm" per resource, not just a static remaining count, since a
  developer's mental model of "when can I try again" depends on it.
- **Burn-down / trend over time** is typically a simple time-series (line or bar per
  interval) of request *volume*, separate from the "remaining quota" gauge — they
  answer different questions ("how much am I using" vs "how much is left right now").
  This maps directly to the requirement's two distinct pieces: (a) quota
  remaining/limit per resource [current-state gauge] and (b) request volume over time
  [trend chart] — they should be visually and structurally separate sections, not
  merged into one widget, matching how `ApprovalAnalyticsPanel` separates its summary
  `cards` row from its `Daily Breakdown` table below.
- **Threshold-based alerting** (industry-standard: warn at 10-20% remaining) is
  exactly what `github/rate_limit.go`'s `rateLimitWarnPercent = 10` already does
  server-side; the UI's job is to expose that threshold as a user-editable setting and
  visually flag when current-state crosses it (e.g. gauge fill color flips from
  success→warning at the threshold, matching §2's 3-state color mapping).
- **Per-source/per-consumer breakdown** (who/what is consuming quota) is the
  "attribution" ask in requirements.md — commonly rendered as a ranked table (source →
  request count → share of total), i.e. exactly `ApprovalAnalyticsPanel`'s existing
  "Top Tools" table pattern (name, count, inline `Bar`), reusable near-verbatim for
  "poller/source → request count".

## 4. User mental models and expectations

For this feature's actual user (Tyler, solo developer, local systemd service — see
Job-to-be-Done in §6), the mental model is closer to **"a fuel gauge for a shared
resource"** than a general analytics dashboard:

- Primary question on opening the panel: *"Am I about to hit a wall?"* — answered by
  the gauge/remaining-vs-limit view, must be the first thing visible (no scrolling,
  no time-range selection required to see current state) — matches
  `ApprovalAnalyticsPanel`'s pattern of showing summary `cards` before any table/chart
  requiring interaction.
- Secondary question: *"What used it up?"* — answered by the source/poller breakdown
  table — this is a diagnostic, not a monitoring, need: it's consulted **after** a
  near-miss or WARN log line, not proactively on a daily cadence. This favors a table
  sortable/scannable by count (as `ApprovalAnalyticsPanel`'s tables already are) over a
  dense multi-series chart.
- Tertiary question: *"Is this normal, or is something new eating my quota?"* —
  answered by the time-range volume chart — this is the one place a genuine
  time-series view earns its place, since "normal" requires comparing today against a
  recent baseline.
- Users of quota dashboards (general pattern, not stapler-squad-specific) expect the
  **resource split to be explicit and separately gauged** — GitHub's own API splits
  core (5000/hr) and search (30/hr) into entirely separate buckets with no shared
  pool, and conflating them (e.g. a single blended percentage) actively misleads,
  since search's 30/hr can be silently exhausted while core sits at 95% remaining.
  The panel must never show one combined "quota" number.

## 5. Accessibility requirements (WCAG / ARIA / keyboard)

Confirmed via WebSearch (Aug 2026 — AEL Data, 216digital, A11Y Collective, DubBot chart-a11y
guides) and cross-checked against `ApprovalAnalyticsPanel`'s existing (compliant)
implementation:

- **Never encode state by color alone.** Every gauge/bar must pair its color with
  visible text (e.g. "342 / 5000 (6.8% remaining)") — `ApprovalAnalyticsPanel` already
  does this everywhere (`allowCount`/`denyCount`/`manualCount` spans always render
  alongside their colored `Bar`). ~1 in 12 men have color vision deficiency; a
  success/warning/error-only gauge with no numeric readout fails this.
- **`aria-hidden="true"` on purely decorative bar `<div>`s**, with the real
  accessible value living in sibling text — exactly the existing `Bar`/`StackedBar`
  pattern. Do not add `role="img"` + `aria-label` to the bar itself when adjacent text
  already conveys the same value (redundant, and `ApprovalAnalyticsPanel` doesn't do
  this either) — but if a genuinely bare visual (e.g. an SVG sparkline with no
  adjacent per-point text) is used for the time-series volume chart, that SVG **must**
  get a text alternative — either `aria-label` summarizing the trend ("Request volume
  over last 7 days, peak 340 on Tuesday") or `aria-describedby` pointing at a visually
  hidden data table, since per-point values aren't otherwise exposed anywhere else on
  the page. Simplest compliant approach: keep the same bar-per-bucket table row
  pattern the rest of the panel uses (numbers in cells, decorative bar in an adjacent
  cell) instead of a bare SVG line, which sidesteps needing a separate text
  alternative entirely.
- **Time-range selector keyboard/ARIA**: continue the `role="group" aria-label="..."`
  + `aria-pressed` button-group pattern already used for the window selector and
  filter chips — this is already keyboard-navigable (native `<button>` elements, Tab
  order, Enter/Space activate) and announces state changes via `aria-pressed` without
  extra work. Do not introduce a custom `<select>`-replacement widget requiring
  hand-rolled arrow-key handling — the existing convention avoids the exact keyboard
  trap that a custom dropdown or slider would otherwise require WAI-ARIA APG's
  `listbox`/`slider` patterns to avoid.
- **Announce state changes.** When the time-range selector changes and the volume
  chart/table re-renders, a screen-reader user needs to know the view updated. The
  existing panel doesn't add an explicit live region for this (the window-selector
  buttons rely on `aria-pressed` state change, which most screen readers do announce
  on the focused control) — this is very likely acceptable to carry forward unchanged
  given it's the established pattern here, but if the new panel is reviewed for a11y
  regressions, consider whether a `aria-live="polite"` region on the results
  summary (e.g. "Showing 7 days, 1,204 requests") is warranted; note as an open item
  rather than over-engineering ahead of an actual finding.
- **Contrast**: reuse `vars.color.success/warning/error` + their `*Text` variants only
  — every one of those pairs has a WCAG-AA-contrast comment already verified in
  `theme.css.ts` (e.g. `successText: "#065f46" /* success on successBg = 3.83:1, fails
  WCAG AA; #065f46 = 6.78:1 */`). Do not introduce a new color pairing without
  running the same contrast check — several existing comments in that file document a
  *previous* AA failure that was fixed, which is exactly the trap a new ad-hoc color
  choice would fall back into.
- **Warn-threshold input**: a numeric input (percent) needs a visible `<label>` (not
  placeholder-only) and should validate/clamp to a sane range (e.g. 1-90%) with the
  error surfaced as text near the field, following the `error`/`role="alert"` pattern
  already in the panel rather than a browser-native-only validation message (which
  screen readers may not reliably announce depending on browser).

## 6. Error states and edge cases

Cases the UI must handle gracefully, informed by the requirements' explicit call-outs
(fresh install, resource with very low limit) plus general dashboard failure modes:

- **Fresh install / no data yet**: zero historical rows. Must not render an empty
  chart or a table with a header row and nothing else — follow the `empty`/`emptyHint`
  two-line pattern: primary line ("No GitHub API activity recorded yet.") + hint line
  explaining what triggers data ("Recorded automatically as pollers and RPCs make
  GitHub API calls — check back after your first poll cycle."). The *current-quota*
  gauge is a separate concern from historical data — if the app has never made a
  GitHub API call, there may be no `X-RateLimit-*` headers observed yet either, so the
  gauge itself needs its own "not yet observed" state (e.g. "—" instead of a
  percentage, not a misleading "100% remaining" default), distinct from the
  history-empty state.
- **Stale data**: if the last observed rate-limit headers are old (e.g. app was
  restarted and hasn't made a request in this resource recently, or GitHub API is
  unreachable), the gauge should indicate staleness rather than silently showing a
  last-known value as if it were current — e.g. a relative "as of 14m ago" annotation
  next to the gauge value, reusing the "resets in Xm" style noted in §3. This avoids
  the failure mode of a developer trusting a stale "4800/5000 remaining" reading that's
  actually hours old right before hitting a wall.
- **Resource with an unusually low limit (GitHub search API: 30/hr vs core: 5000/hr)**:
  per §4, resources must never be blended into one number. Additionally, the *scale*
  of the volume-over-time chart must be per-resource (search's 30/hr chart needs
  different y-axis granularity than core's 5000/hr) — a shared linear scale across
  resources would make search's trend line look like a flat zero next to core's.
  `ApprovalAnalyticsPanel`'s `Bar`/`StackedBar` already scale `max` per-table (`maxDayTotal`,
  `topTools[0].count`, etc., computed locally in each section) rather than globally —
  the same per-resource-local-max convention should apply here, computed independently
  per resource card/section.
  For a `30`-limit resource, prefer showing the search API's warn threshold in **absolute
  request counts**, not just percent (e.g. "3 of 30 remaining" — where 10% is only 3
  requests, a single burst can jump multiple percentage points, so percent-only framing
  understates how fast the search resource can be exhausted).
- **Rate limit actually exhausted (0 remaining)**: this is the state the whole feature
  exists to prevent reaching invisibly (per requirements.md's Success Metrics). The
  gauge's critical/error tier (§2) should be visually unmistakable — not just the
  "warning" tier scaled up, but the distinct `critical`/`error` token pairing — and the
  reset-countdown becomes the single most important piece of information on the panel
  in that state (when can I resume).
- **A poller/source with zero attributed requests in the selected window** (e.g. a
  disabled poller, or a new call site added but not yet exercised): should appear in
  the breakdown as a zero row or be cleanly omitted — follow the existing precedent in
  `UnifiedActivityTable`, which explicitly includes zero-count "uncovered" rows
  alongside populated ones rather than hiding them, since visibility into *what could
  contribute but hasn't* is itself useful diagnostic information here (e.g. confirming
  a poller is correctly quiet vs. silently broken and not polling at all).
- **Config-driven poll interval changes** (in scope per requirements.md) — if changing
  a poll interval requires a restart (per the "Open Questions" hot-reload uncertainty),
  the settings UI for that must say so explicitly rather than implying the change is
  live, to avoid a developer believing they've throttled a poller when they haven't
  yet (until restart). This is a plan-phase decision (hot-reload vs restart-required),
  but the UX consequence — the setting's UI copy must match whichever is chosen — is
  worth flagging now so Phase 3 planning doesn't leave it as a silent gap.

## 7. Jobs-to-be-Done (solo developer, local debugging)

Applying the JTBD lens to Tyler as the sole user of this panel, debugging his own
local rate-limit issue:

- **Functional job**: "Tell me, right now, whether I'm close to being blocked, and if
  I've already been blocked, tell me who did it and when I get quota back." This is a
  diagnostic tool reached for reactively (after seeing a WARN log line, a stalled
  poller, or a failed on-demand RPC), not a dashboard checked on a routine cadence —
  the design should optimize for *fast time-to-answer from a cold open* (current state
  visible with zero clicks) over information density or historical depth.
- **Emotional job**: reduce the "is something silently broken" anxiety that comes from
  the *current* baseline behavior — a WARN buried in a log file that's easy to miss,
  with no way to confirm after the fact what actually happened. The panel should leave
  the developer feeling like they have **ground truth**, not just another log stream —
  which is why the requirement's persistence-across-restart and per-source attribution
  matter emotionally as much as functionally: "I can now prove which poller did it"
  is a materially different feeling than "I have a rough guess."
- **Social job**: minimal in this solo-local-app context — there's no team or
  stakeholder consuming this data. The nearest analogue to a "social" job is
  Tyler-as-past-self vs Tyler-as-future-self: the historical record exists so that a
  future debugging session doesn't have to re-derive what happened from scratch,
  which is really a durability/trust job more than a social one. This reinforces that
  the persisted-history requirement (surviving restart) is not a nice-to-have but core
  to the job — an in-memory-only view would satisfy the functional job in the moment
  but fail the "prove it after the fact" emotional/trust job entirely.

## Summary of concrete UI recommendations

1. **No new charting library** — reuse `ApprovalAnalyticsPanel`'s hand-rolled
   `Bar`/`StackedBar` + table-row pattern for both the volume-over-time view and the
   source/poller breakdown.
2. **Structure**: `panel` → `titleRow` (title + time-range `windowSelector` +
   refresh button) → `cards` grid (one stat tile per GitHub resource: core, search,
   graphql, etc. — each independently gauged, never blended) → `tableSection` for
   request-volume-over-time (bar-per-bucket table, same shape as "Daily Breakdown") →
   `tableSection` for source/poller breakdown (same shape as "Top Tools").
3. **Gauge = `barTrack`/`barFill` with a 3-tier color swap** (`success` → `warning` at
   the user's configured threshold → `error`/`critical` at 0 remaining), always paired
   with numeric text ("X / Y remaining, resets in Zm"). No new color tokens required.
4. **Warn threshold**: a labeled numeric input (validated/clamped range), following
   the existing `error`/`role="alert"` validation-message convention — not
   placeholder-only, not browser-native-only validation.
5. **Empty/stale states get distinct copy**: "never observed" (no rate-limit headers
   seen yet) vs "no historical data in this window" vs "data is stale" are three
   different states and must not collapse into one generic empty message.
6. **data-testid**: only on elements ARIA role/label can't uniquely target (e.g. a
   per-resource stat tile if resource names could collide with other page text) —
   default to `getByRole`/`aria-label` locators per
   `.claude/rules/e2e-test-conventions.md`, matching the sparse existing usage in
   `ApprovalAnalyticsPanel.tsx`.

Sources:
- [API Rate Limiting Best Practices (2026): Implementation Guide for Developers](https://www.getknit.dev/blog/10-best-practices-for-api-rate-limiting-and-throttling)
- [Rate Limiting Best Practices in REST API Design | Speakeasy](https://www.speakeasy.com/api-design/rate-limiting/)
- [API Rate Limiting at Scale: Patterns, Failures, and Control Strategies](https://www.gravitee.io/blog/rate-limiting-apis-scale-patterns-strategies)
- [The Ultimate Guide to Making Charts, Graphs, and Data Accessible - AEL Data](https://aeldata.com/guide-to-making-charts-graphs-and-data-accessible/)
- [Creating Accessible Data for Charts and Graphs - 216digital](https://216digital.com/creating-accessible-data-for-charts-and-graphs/)
- [The Ultimate Checklist for Accessible Data Visualisations - The A11Y Collective](https://www.a11y-collective.com/blog/accessible-charts/)
- [Beyond the Chart: A Guide to Accessible Data Visualization | DubBot](https://dubbot.com/dubblog/2024/charts-graphs.html)
