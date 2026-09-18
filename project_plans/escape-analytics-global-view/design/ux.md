# UX Design: escape-analytics-global-view

**Phase**: Design (post-plan) · **Source inputs**: `requirements.md`, `research/ux.md`, `implementation/plan.md`, `web-app/src/components/sessions/SessionDetailView.tsx:571-612` (tab precedent), `web-app/src/components/analytics/EscapeAnalyticsPage.tsx` (current page).

This document specifies every user-facing surface introduced by this feature: wireframes,
interaction flow, and error/edge-case handling, followed by testable UX acceptance criteria.
It implements the recommendations in `research/ux.md` and the frontend decisions in
`implementation/plan.md` Step 3c / Story 2.4 — it does not re-derive them.

Note on scope: `plan.md` Story 3.1 leaves the time-range filter's UI control status open
("if a UI control exists — else RPC-level only"). This doc designs the control as an MVP
surface (§4) since the RPC supports it and leaving it RPC-only would silently orphan the
"filtered-empty" state research called for — but flags it as the one surface that may be
cut to RPC-only without invalidating the rest of this design.

---

## Surface inventory

1. View-mode tab toggle ("Per-Session" / "All Sessions")
2. Aggregate summary card (mangle rate + histogram + dominant-contributor annotation)
3. Per-session breakdown table
4. Time-range filter control
5. Empty/loading/error states (cross-cutting, enumerated per surface below)

---

## 1. View-mode tab toggle

### Wireframe

```
┌─────────────────────────────────────────────────────────────────┐
│ Escape Analytics                                                 │
│ Inspect terminal escape sequence statistics and mangle events…   │
├─────────────────────────────────────────────────────────────────┤
│  ┌───────────────┐┌───────────────┐                              │
│  │  Per-Session  ││ All Sessions  │   ← role="tablist"           │
│  └───────────────┘└───────────────┘                              │
│  ▲ active (aria-selected=true, underline/bg per vars.color.*)    │
└─────────────────────────────────────────────────────────────────┘
```

Positioned exactly where `sessionSelectorRow` sits today (`EscapeAnalyticsPage.tsx:51`) —
the tablist replaces that row's vertical slot; the session `<select>` moves *below* it,
inside the "Per-Session" tabpanel only.

### Interaction flow

| Actor | Action | System response |
|---|---|---|
| User | Loads `/analytics` (or equivalent route) | "Per-Session" tab active by default (`viewMode: "per_session"`); behavior identical to today. |
| User | Clicks "All Sessions" tab, or presses `→`/`←` to move focus onto it (auto-activation model, per `research/ux.md` §3) | `viewMode` flips to `"all_sessions"`; per-session tabpanel unmounts (hooks `enabled: false`); all-sessions tabpanel mounts (`useEscapeAnalyticsGlobalSummary(enabled: true)` fires); focus **stays on the tab button** (does not jump into the panel — per ARIA Tabs pattern, confirmed in `research/ux.md` §3 "Focus management"). |
| User | Clicks "Per-Session" tab again | Reverses the above; `selectedSessionId` and filters from before the switch are preserved (component state, not cleared) so returning to "Per-Session" restores the prior view exactly. |

### Error/edge cases

- Neither tab can be individually disabled in this feature (both always available) — no
  disabled-tab state to design, per `research/ux.md` §3's note that `aria-disabled` support
  exists in the reused pattern but isn't needed here.
- Rapid tab-switching before a fetch resolves: the `cancelled`-flag guard (Story 2.1/2.2)
  ensures a stale response from the tab just left never calls `setState` for the tab now
  active — user-visible effect is simply "no flash of wrong data," not a distinct state to
  render.

### ARIA wiring (closes the gap `plan.md` Step 3c names in the reused pattern)

```
<div role="tablist">
  <button id="tab-per_session" role="tab" aria-selected="true"
          aria-controls="tabpanel-per_session">Per-Session</button>
  <button id="tab-all_sessions" role="tab" aria-selected="false"
          aria-controls="tabpanel-all_sessions">All Sessions</button>
</div>
<div id="tabpanel-per_session" role="tabpanel" aria-labelledby="tab-per_session"> … </div>
<div id="tabpanel-all_sessions" role="tabpanel" aria-labelledby="tab-all_sessions" hidden> … </div>
```

---

## 2. Aggregate summary card

### Wireframe

```
┌─────────────────── All Sessions tabpanel ────────────────────────┐
│ ┌─ Mangle Rate ──────────────┐  ┌─ Sequence Histogram ──────────┐│
│ │  3.2%  (640 / 20,000)      │  │  CSI  ████████████  12,400    ││
│ │  ⚠ fleet-wide              │  │  OSC  ██████        4,900     ││
│ │                            │  │  DCS  ██            2,700     ││
│ └────────────────────────────┘  └────────────────────────────────┘│
│                                                                     │
│ ℹ Top contributor: session "worktree-fix-tls" — 71% of mangled     │
│    events (455 of 640). Fleet-wide rate may be driven by this      │
│    one session rather than a systemic issue.                      │
└─────────────────────────────────────────────────────────────────┘
```

Reuses `MangleRateIndicator` and `SequenceHistogram` components unchanged (per
`plan.md` Story 2.4 Task 2.4.3) — same visual language as the per-session tab, just fed
`useEscapeAnalyticsGlobalSummary`'s aggregate numbers instead of a single session's.

The dominant-contributor line is new: computed client-side from `perSession` (the row with
max `totalMangled`, shown only if that row's share of `totalMangled` exceeds a fixed
threshold, e.g. >50%) — informational tone (`role` omitted, not `alert`), not an error.

### Interaction flow

| Actor | Action | System response |
|---|---|---|
| User | Switches to "All Sessions" | Card shows `loadingText` ("Loading…") in place of both sub-cards while `useEscapeAnalyticsGlobalSummary`'s `loading` is true; on resolve, renders mangle rate + histogram + (conditionally) the dominant-contributor line. |
| User | Applies/changes a time-range filter (§4) | Card re-fetches and re-renders with the new aggregate numbers; loading state shown during re-fetch, old numbers do not linger stale (guarded by the `cancelled` pattern). |

### Error/edge cases

| Case | Condition | Treatment |
|---|---|---|
| RPC failure | `useEscapeAnalyticsGlobalSummary`'s `error` set | `role="alert"` banner, `styles.errorBanner`, text: `Failed to load global summary: {error.message}` — placed where `summaryError`'s banner sits today, above the card grid. Card grid is not rendered underneath (no stale/mixed state). |
| Global empty | No sessions have any escape events at all (`totalSequences === 0` and `perSession.length === 0`, no active time filter) | Replace the card grid with a neutral message: `"No escape sequence events recorded across any session yet."` — not an error banner, no `role="alert"`. |
| Filtered empty | Same zero condition, but a time-range filter is active | Different copy, referencing the filter: `"No escape events in the selected time range."` Include a visible "Clear filter" action (see §4) so the user isn't left wondering whether the feature is broken. |
| Dominant contributor absent | Largest contributor's share ≤ threshold (no single session dominates) | Omit the "Top contributor" line entirely — do not render an empty/placeholder line. |

---

## 3. Per-session breakdown table

### Wireframe

```
┌─ Per-Session Breakdown ───────────────────────────────────────────┐
│ Session          ▲▼ │ Total Sequences ▲▼ │ Total Mangled ▲▼ │ Mangle Rate ▼      │
├──────────────────────┼─────────────────────┼──────────────────┼────────────────────┤
│ worktree-fix-tls    │        5,200        │       455        │ ⚠ 8.8% (above 2×) │  ← tinted row
│ backlog-ui-polish   │        6,100        │       120        │    2.0%            │
│ session-abc123      │        8,700        │        65        │    0.7%            │
├──────────────────────┴─────────────────────┴──────────────────┴────────────────────┤
│ (no pagination — all rows rendered; sorted by Mangle Rate desc by default)          │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

- Native `<table>`/`<thead>`/`<tbody>`, `<th scope="col">` per column, matching
  `EscapeEventTable`'s existing markup convention (per `research/ux.md` §3).
- Each sortable header is a `<button>` inside its `<th>`, with `aria-sort` on the `<th>`:
  `"descending" | "ascending" | "none"`.
- Outlier row (mangle rate > 2× the fleet-wide rate, or a flat 5% floor if fleet-wide rate
  is ~0) gets a `vars.color.errorBg`/`warningBg` tint **plus** a `⚠` glyph and a visually
  hidden `" (above threshold)"` span in the Mangle Rate cell — non-color-only per WCAG 1.4.1.

### Interaction flow

| Actor | Action | System response |
|---|---|---|
| User | Switches to "All Sessions" | Table renders `perSession`, default-sorted by Mangle Rate descending — worst offender is row 1 with zero clicks (per `research/ux.md` §2). |
| User | Clicks "Total Sequences" header button | Re-sorts ascending; `aria-sort="ascending"` on that `<th>`, `"none"` on the others; click again toggles to descending. |
| User | Tabs to a header button via keyboard, presses Enter/Space | Same sort behavior as click — native `<button>` semantics, no custom key handling needed (per `research/ux.md` §3). |
| User | Scans rows for the tinted/⚠ row | Identifies the outlier without needing to sort — the highlight is independent of current sort order. |

### Error/edge cases

| Case | Condition | Treatment |
|---|---|---|
| Fetch error | Shares the aggregate card's `error` state (one hook, one error) | Table area not rendered; covered by the same `role="alert"` banner as §2 — one error surface for the whole tabpanel, not a duplicate. |
| Global empty / filtered empty | Same conditions as §2 | Table area not rendered; same neutral message as §2 covers both card and table (single combined empty state for the tabpanel, not two separate empty messages). |
| Exactly one session has events | `perSession.length === 1` | Table renders normally with one row — no special-casing; the aggregate card's numbers will equal that one row's numbers, which is itself informative (visibly "this is a single-session issue"). |
| Many sessions (>200) | Deferred per `plan.md` Step 3.5 — no pagination/virtualization in this design; documented follow-up trigger only. | Not designed here; out of scope for MVP per plan. |

---

## 4. Time-range filter control

### Wireframe

```
┌─ Filter ───────────────────────────────────────────────────────────┐
│ From: [2026-08-01T00:00]   To: [2026-08-11T23:59]   [Clear]        │
└──────────────────────────────────────────────────────────────────────┘
```

Two `datetime-local` inputs + a "Clear" button, placed above the aggregate summary card
inside the "All Sessions" tabpanel — mirrors the existing per-session `filterRow` pattern
(`EscapeAnalyticsPage.tsx:112-138`) visually, but with date/time inputs instead of text
filters since the RPC field is `start_time`/`end_time`, not a string match.

### Interaction flow

| Actor | Action | System response |
|---|---|---|
| User | Sets "From" and/or "To" | On each change, `useEscapeAnalyticsGlobalSummary`'s params update; hook re-fetches (debounced or on blur — implementation detail, not a UX requirement); summary card and table both reflect the filtered result. |
| User | Clicks "Clear" | Both inputs reset to empty; hook re-fetches with no time bounds — returns to the fleet's all-time view. |
| User | Leaves "To" before "From" (invalid range) | Inline validation message under the inputs (`"End time must be after start time"`); no RPC call is made with an invalid range — this is a client-side guard, not a server error surface. |

### Error/edge cases

- Filtered-empty result → handled by §2/§3's "filtered empty" state, which must include the
  "Clear filter" action described there — this control **is** that action's target.
- If plan.md's build ships this as RPC-level-only (no UI control) for MVP, the filtered-empty
  copy and Clear-filter action in §2/§3 become dead code paths until the control ships —
  flag this explicitly in the PR description rather than silently shipping unreachable UI text.

---

## 5. Cross-cutting states summary

| State | Trigger | Surfaces affected | Visual treatment |
|---|---|---|---|
| Loading (first fetch) | Tab switch to "All Sessions", or filter change while hook fetches | Summary card, table | `loadingText` ("Loading…") in place of content — matches existing per-session convention |
| RPC error | `GetEscapeAnalyticsGlobalSummary` fails | Summary card + table (shared) | `role="alert"`, `styles.errorBanner`, `"Failed to load global summary: {error.message}"` |
| Global empty | Zero events fleet-wide, no filter | Summary card + table (shared) | Neutral text, no `role="alert"`: `"No escape sequence events recorded across any session yet."` |
| Filtered empty | Zero events for active time range | Summary card + table (shared) | Neutral text referencing the filter + visible Clear action: `"No escape events in the selected time range."` |
| Dominant contributor | One session > threshold share of mangled events | Summary card only | Informational `ℹ` line, not an alert |
| Outlier row | Per-row mangle rate > 2× fleet rate (or 5% floor) | Table row | Background tint + `⚠` glyph + visually-hidden text — never color alone |

All four "big" states (loading/error/global-empty/filtered-empty) are mutually exclusive and
rendered in place of the summary-card-and-table content, not stacked — a user never sees a
loading spinner overlapping an error banner or an empty message.

---

## UX Acceptance Criteria

### Tab toggle
1. User can switch from "Per-Session" to "All Sessions" in exactly 1 click (or 1 arrow-key move with auto-activation).
2. `role="tablist"` container has exactly 2 `role="tab"` children; the active tab has `aria-selected="true"`, the inactive tab `aria-selected="false"`.
3. Each tab button has `aria-controls` pointing to an existing element ID; each tabpanel has `role="tabpanel"` + `aria-labelledby` pointing back to its tab's ID — verifiable via DOM query, not just visual inspection.
4. `ArrowLeft`/`ArrowRight` move focus and activate the adjacent tab; focus is never lost (always lands on a `role="tab"` button) after a switch.
5. Switching tabs never triggers a network request from the now-inactive tab's hooks — verifiable by asserting the per-session fetch mock is not called while `viewMode === "all_sessions"` (per Story 2.6's Jest test).
6. Switching away and back to "Per-Session" preserves `selectedSessionId` and per-session filter state exactly as it was before the switch.

### Aggregate summary card
7. On successful load, mangle rate and histogram are visible without any additional user action beyond switching tabs.
8. When one session's mangled-event share exceeds the dominant-contributor threshold, the "Top contributor" line is visible and names the session and its percentage share; when no session exceeds the threshold, the line is absent (not rendered as empty/blank).
9. RPC failure renders a `role="alert"` banner with the exact prefix `"Failed to load global summary:"` and never renders stale/prior data underneath it.
10. Global-empty and filtered-empty states use distinct copy from each other and neither uses `role="alert"`.

### Per-session breakdown table
11. Table is default-sorted by Mangle Rate descending on first render — the highest-rate session is row 1 with zero interaction.
12. Every sortable column header is a real `<button>` element inside a `<th>` with `aria-sort` reflecting current state (`"ascending"`, `"descending"`, or `"none"` on non-active columns).
13. Clicking a header re-sorts by that column; a second click on the same header reverses direction.
14. All sort interactions are reachable via keyboard alone (Tab to header button, Enter/Space to activate) with no custom key handler required beyond native button semantics.
15. A row whose mangle rate exceeds the outlier threshold is visually distinguishable **and** carries a non-color cue (icon glyph or visually-hidden text) — verify by disabling CSS color and confirming the outlier is still identifiable in the accessibility tree / plain-text rendering (WCAG 1.4.1).
16. Table renders unpaginated for any `perSession.length` observed in this feature's own test fixtures (≤ ~10 sessions) — no "load more"/pagination control present in MVP.
17. Table is not rendered (and no stray header row appears) during the global-empty or filtered-empty state — confirmed absent, not merely empty-bodied.

### Time-range filter
18. Setting a valid "From"/"To" pair narrows both the summary card and table to matching results — verifiable by comparing counts before/after against a fixture with events both inside and outside the range.
19. "To" earlier than "From" shows an inline validation message and does not trigger an RPC call with the invalid range.
20. "Clear" resets both inputs and restores the unfiltered (all-time) aggregate view in one click.
21. The filtered-empty state's message text differs from the global-empty state's message text (assert via distinct string, not just presence of *some* empty message).

### Accessibility (cross-cutting)
22. Every text/background color pairing used in the new surfaces (tab active/inactive, error banner, outlier tint, dominant-contributor line) meets ≥4.5:1 contrast ratio using the existing `vars.color.*` token set — verify against tokens actually used, not assumed.
23. No information on any of these surfaces is conveyed by color alone: outlier rows (icon + hidden text, criterion 15), error vs. informational states (distinct `role`/text, not just color), dominant-contributor annotation (text-based, not a colored dot).
24. Tab switching does not forcibly move focus into panel content — focus remains on the activated tab button (assert via `document.activeElement` in a component test).
25. All new interactive elements (tabs, sort-header buttons, filter inputs, Clear button) are reachable via Tab key in a logical order and have an accessible name (via visible label, `aria-label`, or associated `<label>`).

---

## Summary

**5 surfaces designed** (view-mode tab toggle, aggregate summary card, per-session breakdown
table, time-range filter, cross-cutting empty/loading/error states) · **25 UX acceptance
criteria** written, covering interaction (1 click/1 arrow-key switch, default sort, 1-click
clear), the four empty/error states per surface, and accessibility (roving tabindex,
`aria-selected`/`aria-controls`/`aria-labelledby`, ≥4.5:1 contrast, non-color outlier cue
per WCAG 1.4.1).
