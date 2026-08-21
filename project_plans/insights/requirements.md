# Insights Page Enhancement — Requirements

## Project Overview

Elevate the existing Insights page (Claude session token/cost analytics dashboard) from a functional baseline to a polished, high-performance analytics tool. The page already has: summary cards, daily spend chart, model breakdown chart, stacked area chart, top-N skills/tools tables, and a paginated sessions table with live streaming updates.

**Scope: Full enhancement** — performance, usability, and featureset improvements across frontend and backend.

---

## Stakeholder Context

- **User:** Solo developer / small team tracking Claude API spend across multiple projects
- **Current state:** Page works but feels slow, lacks filtering, and doesn't support drill-down

---

## Pain Points (Confirmed)

1. **Slow to load / laggy** — page waits for all data before rendering anything; charts feel heavy
2. **Hard to find specific sessions** — SessionsTable lacks search, column-level filtering, or multi-sort
3. **Missing time range filter** — no way to scope charts/cards to "last 7 days" or a custom range

---

## Requirements

### R1 — Time Range Filter UI

**Priority: High**

The insights page must provide a time range selector that scopes all data — summary cards, charts, and sessions table — to a user-chosen window.

**Acceptance criteria:**
- Filter bar visible at top of page with preset options: Today, Last 7 days, Last 30 days, Last 90 days, All time (default)
- Custom range: date-picker allowing arbitrary from/to selection
- Changing the filter immediately re-fetches summary data and re-renders all charts and the session table
- Selected range persists through live-update cycles (WatchInsights stream respects current filter)
- Active filter clearly indicated in the UI
- Backend `GetInsightsSummary` already accepts `from`/`to` timestamps — frontend must wire these through

### R2 — Per-Session Detail View

**Priority: High**

Clicking a session row in the SessionsTable opens a detail view showing turn-by-turn token usage, tools called, and skill activations for that session.

**Acceptance criteria:**
- Session row is clickable; opens a slide-over panel or dedicated detail page (routing: `/insights/session/[sessionId]`)
- Detail view shows:
  - Session metadata: ID, model(s), project path, total cost, message count, date range
  - Turn timeline: ordered list of assistant turns with per-turn input/output/cache tokens, model, timestamp, and tool names used
  - Tools breakdown: table of tool name → call count → MCP server
  - Skill activations: list of skills triggered (name, turn index, is_command flag)
- Data comes from the existing `SessionTokenSummary` proto (TurnTimeline, ToolUsage, SkillActivations already in the model)
- Close returns user to the insights page with filters intact
- Loading and empty states handled

### R3 — Cost Projections / Budget Alerts

**Priority: Medium**

Show a projected monthly spend based on recent usage rate, and optionally warn when spend is tracking above a configurable threshold.

**Acceptance criteria:**
- Summary cards section adds a "Projected this month" card
  - Calculated as: (spend in current calendar month so far) / (day of month) × (days in month)
  - Only shown when there is ≥7 days of data in the current month (otherwise too noisy)
- Optional budget threshold: user can set a monthly budget in USD via a settings input (stored in browser localStorage)
- When projected spend exceeds threshold, the projection card changes to warning styling (amber) and a banner appears at top of page
- No backend changes required for projections — computed on frontend from existing daily buckets

### R4 — Performance: Faster Initial Page Load

**Priority: High**

Summary cards must render immediately from cached/stale data or with a skeleton loader, without waiting for full data parse.

**Acceptance criteria:**
- Skeleton loader (card-shaped placeholders) shown immediately on page mount, before first RPC response
- If TokenStore is still loading (`is_loading: true`), cards show skeleton; charts show a loading overlay rather than blocking render
- First meaningful paint (summary cards visible) within 200ms of page navigation
- Charts lazy-load independently — chart placeholders visible immediately, charts hydrate when data arrives

### R5 — Performance: Handle Large Session Counts

**Priority: High**

SessionsTable must not degrade with 500+ sessions.

**Acceptance criteria:**
- SessionsTable uses virtual row rendering (react-window or similar) for lists > 100 rows, OR uses server-side pagination via `ListSessionTokens` RPC (page_size ≤ 50)
- Table renders and scrolls smoothly at 500 sessions on a mid-range laptop
- Sorting and filtering remain responsive (< 100ms to re-render after sort change)

### R6 — Performance: Smooth Live Updates

**Priority: High**

WatchInsights stream updates should not cause full re-renders that disrupt user interaction.

**Acceptance criteria:**
- When a live update arrives, only the changed data regions re-render (charts update smoothly, not flash/remount)
- If user is mid-scroll in SessionsTable, an incoming update does not jump/reset scroll position
- Live update indicator (pulsing dot) remains visible during updates

### R7 — Session Table: Search and Filter

**Priority: High**

SessionsTable must support finding sessions by project path or model.

**Acceptance criteria:**
- Text search input above the table: filters rows by project path (substring match, case-insensitive)
- Model filter dropdown: filter to sessions using a specific model family
- Filters work client-side on the current page's data (no extra RPC needed)
- Filter state preserved when live update arrives
- Clear filter button resets all filters

---

## Out of Scope

- CSV export (deferred — not in top user priorities)
- Multi-user / team cost splitting
- Native mobile view (insights is desktop-only per current nav config)
- Changing the JSONL parser or TokenStore internals

---

## Technical Constraints

- **Stack:** React (Next.js App Router), vanilla-extract CSS, recharts, ConnectRPC/protobuf, Go backend
- **No new proto RPCs required** — all data already exists in `GetInsightsSummary` and `ListSessionTokens`; only frontend wiring and minor backend filter pass-through needed
- **CSS:** Use vanilla-extract `.css.ts` pattern per ADR-009; no inline styles for layout
- **Virtual scrolling:** Prefer `react-window` (already likely in the dep tree) or a lightweight alternative — avoid pulling in large new dependencies
- **Projections:** Client-side only; no budget data stored server-side

---

## Success Metrics

| Metric | Target |
|---|---|
| Time to first summary card visible | < 200ms after navigation |
| SessionsTable with 500 rows — scroll frame rate | 60fps |
| Time range filter — time to re-render all charts after change | < 500ms |
| Per-session detail — time to open slide-over | < 150ms (data already in client) |
