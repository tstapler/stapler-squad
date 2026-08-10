# UX Design: Review Queue Severity Levels

Source: `requirements.md`, `research/ux.md`, `implementation/plan.md`. This document is the
UX design artifact for implementation — wireframes, interaction flows, edge cases, and
testable acceptance criteria for the four surfaces the plan touches. It does not
re-litigate decisions already made in `research/ux.md` (vocabulary = plain `RiskLevel`
words, not P0/P1/P2; badge = icon + text + colour; severity-primary default sort frozen at
snapshot time; "not recorded" fail-safe treatment) — it operationalizes them into concrete
layouts and pass/fail criteria.

Vocabulary used throughout: **Critical (🔴/⛔) / High (🟠) / Medium (🟡) / Low (⚪) /
Severity not recorded (⬜, neutral grey)** — 5 visual states for a 4-level enum plus the
"unknown" sentinel, per ADR-001.

---

## Surface 1 — `SeverityBadge.tsx` (shared component, referenced by every surface below)

This isn't a standalone screen but is designed first because Surfaces 2-5 all embed it.

### Wireframe — full variant

```
┌─────────────────────────────┐
│ 🔴 Critical                  │   role="status" aria-label="Critical risk"
└─────────────────────────────┘

┌─────────────────────────────┐
│ ⬜ Severity not recorded      │   role="status" aria-label="Severity not recorded"
└─────────────────────────────┘
```

### Wireframe — compact variant (icon + abbreviation)

```
[🔴 CRIT]   [🟠 HIGH]   [🟡 MED]   [⚪ LOW]   [⬜ N/A]
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Component receives `riskLevel` prop (`"critical"`\|`"high"`\|`"medium"`\|`"low"`\|`""`\|`undefined`) | Renders icon + colour + text label per `getRiskLevelInfo()` lookup; no user interaction — this is a read-only indicator, not a control |
| 2 | User hovers (desktop) | Native `title` tooltip repeats the full label (matches `ReviewQueueBadge`'s `title={...}` precedent) |
| 3 | Screen reader focuses/announces the badge's container | `role="status"` + `aria-label` announces the full word ("Critical risk" / "Severity not recorded") — never just the icon or abbreviation |

### Edge cases

- `riskLevel === ""` or `undefined` → renders the **"Severity not recorded"** state (icon ⬜, neutral grey, distinct from Low's ⚪/blue) — never silently defaults to Low, never omits the badge. This is the single most important edge case in this whole feature: an omitted or falsely-Low badge on a genuinely dangerous unclassified item defeats the feature's purpose (see requirements.md's "Problem" statement).
- Unrecognized string value (defensive — should not happen if backend only emits the 4 known values, but the TS type is a hand-maintained mirror per plan.md, so drift is possible) → falls into the same "not recorded" branch rather than throwing or rendering blank.

---

## Surface 2 — `ApprovalCard.tsx` / `ApprovalDrawer.tsx` (Path A)

### Wireframe — `ApprovalCard` header (badge added next to tool name/countdown)

```
┌──────────────────────────────────────────────────────────────┐
│ 🖥  Bash                              🔴 Critical      0:47   │  ← header row
│                                                                 │
│ Command                                                         │
│ rm -rf /tmp/build/**                                            │
│                                                                 │
│  [Show details ▾]                                               │
│                                                                 │
│   [ Approve ]              [ Deny ]              [ Dismiss ]    │
└──────────────────────────────────────────────────────────────┘
```

### Wireframe — `ApprovalDrawer` list (severity-sorted, tiebreak by expiry)

```
┌─ Pending Approvals (3) ─────────────────────────────────────────┐
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ 🔴 Critical   Bash: git push --force origin main    3:20   │ │  ← B, was last by expiry-only sort
│ └────────────────────────────────────────────────────────────┘ │
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ 🟡 Medium     Bash: npm install left-pad             0:10  │ │  ← C
│ └────────────────────────────────────────────────────────────┘ │
│ ┌────────────────────────────────────────────────────────────┐ │
│ │ ⚪ Low        Bash: git commit -am "wip"              0:30  │ │  ← A, was first by expiry-only sort
│ └────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Drawer opens / new approval arrives | List re-sorts: severity descending (Critical→High→Medium→Low, unrecorded ranked as High per fail-safe) primary key, `secondsRemaining` ascending as tiebreaker |
| 2 | User reads top card first | Sees the highest-risk item regardless of which arrived first or expires soonest — directly serves the "find the one dangerous request" job from `research/ux.md` §5 |
| 3 | User clicks Approve/Deny on any card | Existing approve/deny flow unchanged (AC7) — card leaves the list, remaining cards re-sort by the same rule |
| 4 | Countdown reaches 0 on a card | Existing expiry behavior unchanged; severity badge remains visible until the card is removed |

### Edge cases

- Two items with identical `riskLevel` → ordered by `secondsRemaining` ascending (existing behavior preserved as tiebreaker), so nothing about today's "most urgent expiry first within a tier" experience is lost.
- Item with `riskLevel === ""` → sorts as if High (between Critical and Medium) — visible near the top, not buried at the bottom where an unclassified `rm -rf` could hide.

---

## Surface 3 — `ReviewQueuePanel.tsx` (Path B) — badge, default sort, filter

This is the panel most users triage from day-to-day (per requirements.md #6), so it gets
the most detailed design.

### Wireframe — queue item row (badge added next to escalation reason)

```
┌────────────────────────────────────────────────────────────────────┐
│ ● session-web-app-fix-auth          🔴 CRIT   [Approval Pending]    │
│   ⚠ git push --force origin main                                    │
│   branch: fix-auth · 3 files changed · 2m ago                       │
└────────────────────────────────────────────────────────────────────┘
┌────────────────────────────────────────────────────────────────────┐
│ ● session-docs-typo-fix              ⬜ N/A    [Approval Pending]    │
│   Severity not recorded — this request predates severity tracking.  │
│   branch: docs-fix · 1 file changed · 15m ago                       │
└────────────────────────────────────────────────────────────────────┘
```

### Wireframe — filter panel (Severity row added, mirrors existing Priority/Reason rows)

```
┌─ Filters ────────────────────────────────────────────────────────┐
│ Search: [________________________]                                │
│                                                                     │
│ Priority (any):  [Urgent (2)] [High (5)] [Medium (3)] [Low (1)]   │
│ Reason (any):    [Approval Pending (4)] [Input Required (1)] ...   │
│ Severity (any):  [🔴 Critical (1)] [🟠 High (2)] [🟡 Medium (3)]   │
│                  [⚪ Low (0)] [⬜ Not recorded (1)]                 │
│ Program (any):   ...                                               │
│ Sort by:  [Severity ▾]     ← now the default selection, was "Queue order"
│ Group by: [None ▾]                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Wireframe — empty state when severity filter matches zero items

```
┌────────────────────────────────────────────────────────────────┐
│                                                                    │
│              No items match the current filter.                   │
│                    12 items in queue                               │
│                                                                    │
│                    [ Clear filter ]                                 │
│                                                                    │
└────────────────────────────────────────────────────────────────┘
```
(Identical copy/component to the existing filter-miss empty state at
`ReviewQueuePanel.tsx:1222-1235` — the severity filter is just another predicate feeding
the same `hasActiveFilter` branch, per plan.md Story 6.3. No new empty-state UI.)

### Interaction flow — default severity sort

| Step | User action | System response |
|---|---|---|
| 1 | User opens the panel (no prior sort preference) | `sortField` initializes to `"severity"` (not `"default"`/queue-order); items render Critical→High→Medium→Low→Not-recorded-as-High, ties broken by existing `created_at`/age order |
| 2 | Panel is polling and a new item arrives while the user is mid-review | "N new items added — click to refresh" banner appears (existing behavior, `ReviewQueuePanel.tsx:993-1002`); **the already-rendered rows do not silently reorder** — sort is frozen at `reviewingIdsSnapshot` capture time |
| 3 | User clicks the banner | Snapshot refreshes, sort recomputes once against the new full set, new order renders |
| 4 | User manually changes Sort by → "Last activity" | Explicit override; severity badge still renders on every row regardless of sort field — sort field only controls order, not badge visibility |

### Interaction flow — severity filter

| Step | User action | System response |
|---|---|---|
| 1 | User expands the filter panel | Sees "Severity (any):" row with 5 chips (4 levels + "Not recorded"), each showing a live count in parens, mirroring the existing Priority/Reason chip pattern exactly |
| 2 | User clicks "Critical (1)" | Chip becomes active (`aria-pressed="true"`, active style); list narrows to only Critical items; `hasActiveFilter` becomes true; "✕ Clear" button appears in the filter toggle row |
| 3 | User clicks a second chip, e.g. "High (2)" | Multi-select OR semantics (matches existing `priorityFilter`/`reasonFilter` `Set`-based toggle) — list shows Critical + High items |
| 4 | Selected combination yields 0 items | Empty state shown (see wireframe above) — "N items in queue" reminds the user items exist elsewhere, "Clear filter" is a one-click full reset |
| 5 | User clicks "✕ Clear" or "Clear filter" | All filters (not just severity) reset — matches existing `clearAllFilters` behavior; no severity-only partial clear control, consistent with how every other filter dimension in this panel behaves today |
| 6 | A count reaches 0 for a chip (e.g. no "Low" items right now) | Chip is disabled (`disabled={count === 0}`), matching the existing Priority/Reason chip pattern — not hidden entirely, so the user can see the full taxonomy exists even when empty |

### Edge cases

- Item has `metadata["pending_approval_id"]` set but no `metadata["risk_level"]` key at all (predates this feature, or a future code path bypasses the classifier) → `SeverityBadge` renders its "Severity not recorded" state per Surface 1's edge case; this item is also included under the "Not recorded" filter chip and ranks as High for default sort.
- Item has no `pending_approval_id` at all (not an approval-pending item — e.g. idle/error/stale) → no severity badge renders at all (badge is scoped to the existing `queueItem.metadata?.["pending_approval_id"]` conditional block, unchanged footprint for non-approval review items).

---

## Surface 4 — `ApprovalRulesPanel.tsx` — Risk column (closing an existing gap)

### Wireframe — rules table with new Risk column

```
┌─ Approval Rules ──────────────────────────────────────────────────────┐
│ Pattern              │ Decision │ Risk         │ Triggers │ Actions    │
├───────────────────────┼──────────┼───────────────┼──────────┼────────────┤
│ git push --force      │ Escalate │ 🔴 CRIT       │ 12       │ [Edit][Del]│
│ rm -rf *               │ Escalate │ 🔴 CRIT       │ 8        │ [Edit][Del]│
│ npm install *          │ Allow    │ 🟡 MED        │ 45       │ [Edit][Del]│
│ *.test.ts edits        │ Allow    │ ⚪ LOW        │ 120      │ [Edit][Del]│
└─────────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | User opens the Approval Rules panel | Each row shows its already-stored `riskLevel` (previously silently threaded through `upsertRule` but never rendered — this closes that gap) via the compact `SeverityBadge` |
| 2 | User edits a rule and changes its risk level | Existing edit form behavior unchanged (out of scope — this surface only adds *display*, not a new risk-editing control beyond what already exists) |

### Edge cases

- A rule created before `riskLevel` existed on the rule schema, or with an empty string → renders "N/A" (compact "not recorded" state), same component/logic as every other surface — no separate fallback string invented for this table.

---

## Surface 5 — `ApprovalAnalyticsPanel.tsx` — Risk Level Breakdown

### Wireframe — new section, positioned near "Escalation Reasons" (existing section at line ~329)

```
┌─ Risk Level Breakdown ──────────────────────────────────────────┐
│ Level          │ Count │ Frequency                                │
├─────────────────┼───────┼──────────────────────────────────────────┤
│ 🔴 Critical     │   5   │ ████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │
│ 🟠 High         │  10   │ █████████████████░░░░░░░░░░░░░░░░░░░░░  │
│ 🟡 Medium       │  15   │ ██████████████████████████░░░░░░░░░░░░  │
│ ⚪ Low          │  10   │ █████████████████░░░░░░░░░░░░░░░░░░░░░  │
└──────────────────────────────────────────────────────────────────┘
```

### Wireframe — empty state (zero escalations in window)

```
┌─ Risk Level Breakdown ──────────────────────────────────────────┐
│                No escalations in this window.                     │
└──────────────────────────────────────────────────────────────────┘
```
(Identical copy to the existing Escalation Reasons empty state at
`ApprovalAnalyticsPanel.tsx:358` — same shared `empty` class, not a new string.)

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | User opens the Analytics panel with a time window selected (e.g. 7 days) | `GetApprovalAnalytics` response includes `riskLevelCounts`; panel renders 4 rows (Critical/High/Medium/Low — "not recorded" is not a row here since analytics reads `ClassificationAnalytics.RiskLevel`, which is always populated per decision, unlike the live queue's legacy-item gap) |
| 2 | User changes the time window | Existing window-change refetch behavior unchanged; breakdown updates with the new window's counts |
| 3 | Window has zero escalations | Shared "No escalations in this window." empty state renders instead of an empty/zeroed table |

### Edge cases

- All 4 counts are present but one is 0 (e.g. no Low-risk escalations this week) → row still renders with count `0` and a zero-width/negligible bar — rows are never hidden individually (consistent with Escalation Reasons section's existing behavior of rendering all present categories).

---

## UX Acceptance Criteria (human-testable)

### Task completion

1. **Triage speed**: A user can identify the single highest-severity item in a 40-item review queue in **1 glance, 0 clicks** — the top row after default sort is always the highest severity present, no scrolling or sorting action required.
2. **Filter to one severity level**: A user can filter `ReviewQueuePanel` to show only Critical items in **≤ 2 clicks** (1: expand filters if collapsed, 2: click the Critical chip) — filters panel state persists across the session via existing URL-key persistence (`FILTER_URL_KEYS`), so a returning user with a saved filter needs 0 clicks.
3. **Clear a filter**: A user can return to the unfiltered queue in **1 click** ("✕ Clear" or "Clear filter" button), from any filter combination, including severity.
4. **See risk on a rule**: A user can see a rule's configured risk level in `ApprovalRulesPanel` in **0 clicks** — it's a visible table column, not behind an expand/edit action.
5. **See severity trend**: A user can answer "is manual review skewing toward higher-risk items this week?" in **≤ 1 click** (select time window) once on the Analytics panel — no drill-down required for the top-level counts.

### Error / edge-case states

6. **"Severity not recorded" is never confused with "Low"**: the not-recorded state uses a visually and semantically distinct icon (⬜, neutral grey) and text ("Severity not recorded") from the Low state (⚪, blue/grey-but-distinct) in every surface that renders `SeverityBadge`. A user shown the two side-by-side can distinguish them without reading the tooltip.
7. **Severity filter zero-match state** shows exactly: "No items match the current filter." + "`N` items in queue" + a "Clear filter" button — same three elements, same copy, as every other filter dimension's zero-match state in this panel. No dead end: the "Clear filter" button always exits back to the full queue.
8. **Unclassified items surface, not hide**: an item with unrecorded severity appears **above Medium-severity items** in default sort order (fail-safe "sorts as High" behavior) — verifiable by seeding one Critical, one unrecorded, and one Medium item and confirming render order is Critical → unrecorded → Medium.
9. **No dead ends**: every state reachable via filtering, sorting, or an empty analytics window has a visible, single-click path back to an unfiltered/default view (Clear filter button, or — for the analytics empty state — simply changing the time window, which is always visible above the section).

### Accessibility

10. **Keyboard navigation**: every severity filter chip, sort dropdown, and "Clear filter"/"✕ Clear" control is reachable and operable via Tab + Enter/Space alone, with visible focus outlines — no mouse-only interaction introduced by this feature (all new controls reuse existing `<button>`/`<select>` elements from the established filter-chip pattern).
11. **Screen-reader labels**: `SeverityBadge` exposes `role="status"` and a full-word `aria-label` (e.g. `aria-label="Critical risk"`, `aria-label="Severity not recorded"`) in both full and compact variants — the compact variant's abbreviation (`CRIT`/`HIGH`/`MED`/`LOW`) and icon are both `aria-hidden="true"`, matching `ReviewQueueBadge.tsx`'s existing pattern exactly, so a screen reader announces the full word, never the abbreviation or a bare icon glyph.
12. **Colour contrast ≥ 4.5:1**: every `SeverityBadge` colour/background pairing (the new
    `criticalBg`/`criticalText` trio, and the reused `errorBg`/`errorText`, `warningBg`/
    `warningText`, `successBg`/`successText`, plus the "not recorded" pairing — which per
    `plan.md` Task 4.3.1 **reuses the existing `surfaceMuted`/`textMuted` tokens
    (`theme-contract.css.ts:8,21`), it is not a newly-introduced pairing**, corrected here
    post-validate to match plan.md, see consistency-check BLOCKER finding) meets WCAG AA
    4.5:1 contrast in **all 6 theme blocks** (light, dark, matrix, cyberpunk77, wh40k, clean).
    Only the new `critical`/`criticalBg`/`criticalText` trio needs fresh per-theme
    contrast-ratio documentation (Task 4.2.2); `surfaceMuted`/`textMuted`'s contrast is
    verified wherever those existing tokens are already used elsewhere in the app — re-confirm
    it holds for this specific badge pairing too before shipping, and re-checked by the
    existing Axe Core CI gate on PRs touching `web-app/src/`.
13. **Severity is never color-only** (WCAG 1.4.1): every rendering of severity — full badge, compact badge, table cell — pairs colour with both an icon (shape-distinct, not just hue-distinct: 🔴/⛔ vs 🟠 vs 🟡 vs ⚪ vs ⬜) and a text label or abbreviation. A user with red-green colour vision deficiency, or viewing on a greyscale/print rendering, can still distinguish all 5 states from icon shape + text alone, with colour removed entirely. This directly follows `ReviewQueueBadge.tsx`'s existing precedent, extended to a 5-state (4 levels + unknown) vocabulary instead of 2.
14. **No new colour tier collides visually with an existing one**: `RiskCritical` and `RiskHigh` render with visually distinguishable hues (not both mapped to the existing single `error`/`errorBg` pair) — verified by the Story 4.2 requirement that `critical`/`criticalBg`/`criticalText` are new, distinct tokens in all 6 themes, not a reuse of `error*`.

---

## Gaps closed post-triad (UX lens)

- **Mobile/touch**: `SeverityBadge` and filter chips reuse the existing `ReviewQueueBadge`/
  priority-chip touch-target sizing and responsive breakpoints already in place for
  `ReviewQueuePanel` — no new layout primitive is introduced, so no new mobile-specific design
  is needed beyond verifying the added 5th filter chip doesn't overflow the existing chip row
  on narrow viewports (add to Task 6.3.1's manual QA pass).
- **Loading/error states**: no new RPC is added by this feature (Story 2.1/2.2 add fields to
  existing `ListPendingApprovals`/`GetApprovalAnalytics` responses) — existing
  loading/error/retry treatment for those calls is unchanged and already covers the new field;
  a `risk_level`-shaped field simply renders as absent (→"Severity not recorded") during any
  transient fetch-error/stale-data window, which is the correct fail-safe behavior already
  specified above, not a new state to design.
- **Silent default-sort-order change**: flipping `ReviewQueuePanel`'s default from
  chronological to severity-first (Story 6.2) has no in-app notice planned. Accepted as a
  one-time, low-severity UX cost for this ship (matches the precedent of prior default-sort
  changes in this panel) rather than building one-off "what's new" UI; if user confusion
  surfaces post-ship, a toast/tooltip is a cheap follow-up, not a blocker now.
- **Cross-surface consistency (Path A vs Path B)**: both paths read `RiskLevel` from the same
  `PendingApproval`/`ApprovalMetadata` source captured once at creation (Story 1.1/1.3) — by
  construction they cannot disagree for the same approval; no additional design needed.
- **`aria-live` on filter-count changes**: add `aria-live="polite"` to the filtered-item-count
  region in `ReviewQueuePanel` (mirrors the existing `hasActiveFilter` count text) so screen
  reader users hear the new count after toggling a severity chip — fold into Task 6.3.1.
- **"N/A" ambiguity**: rename the compact abbreviation for the not-recorded state from `N/A` to
  `N/R` ("not recorded") to avoid being misread as "not applicable" — update Task 4.3.1's
  `getRiskLevelInfo()` abbreviation for the `unknown` variant.

## Summary

- **5 surfaces designed**: `SeverityBadge.tsx` (shared component), `ApprovalCard.tsx`/`ApprovalDrawer.tsx` (Path A), `ReviewQueuePanel.tsx` (Path B — badge, default sort, filter, empty state), `ApprovalRulesPanel.tsx` (Risk column), `ApprovalAnalyticsPanel.tsx` (Risk Level Breakdown section + empty state).
- **14 UX acceptance criteria** written: 5 task-completion, 4 error/edge-case, 5 accessibility.
