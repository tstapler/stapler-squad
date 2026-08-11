# UX Design: Description Prominence in Backlog Item Detail

**Item**: `5b0e1d57-5244-4847-9b61-029b147f6aab` — "Backlog description should be more prominent."
**Scope**: default-state flip only — Description's effective `defaultExpanded`
(threaded into the `CollapsibleGroup`'s controlled `value` prop, via
`openSectionKeys`, which is itself built from
`useSectionExpandState(itemId, "description", false)` in
`BacklogItemDetail.tsx`) changes from `false` to `true`. (Not an uncontrolled
`defaultValue` — `BacklogItemDetail.tsx` passes `value={openSectionKeys}` plus
`onValueChange`, so the group is fully controlled; see plan.md's Alternatives
Considered section and `Collapsible.tsx:139-157`.) No new component, no new
visual treatment, no markup/ARIA change. This doc is intentionally short —
proportional to a one-flag fix.

## 1. Surface affected

One surface, two content states, one persistence interaction:

- **Backlog item detail view** (`BacklogItemDetail.tsx`) — the Description
  section rendered by `DescriptionSection.tsx` inside the secondary-info
  `CollapsibleGroup`.
  - **Has content**: markdown description body.
  - **Empty**: `<p>No description.</p>` (existing empty-state markup,
    unchanged).
- **Interaction with per-item stored preference**: `useSectionExpandState`
  reads/writes `localStorage["backlog-detail-section-<itemId>-description"]`.
  The default only governs items/users with *no* stored entry yet.

## 2. Before / after

**Before** — Description collapsed by default, Acceptance Criteria always open:

```
┌ Item Title ──────────────────────────────────┐
│ Status · Priority · Tags                      │
├────────────────────────────────────────────────┤
│ Acceptance Criteria                            │  ← always visible
│  ▸ No acceptance criteria defined.             │
├────────────────────────────────────────────────┤
│ ▶ Description                                  │  ← collapsed, 1 click to read
├────────────────────────────────────────────────┤
│ ▶ Comments                                     │
└────────────────────────────────────────────────┘
```

**After** — Description open by default, same as Acceptance Criteria:

```
┌ Item Title ──────────────────────────────────┐
│ Status · Priority · Tags                      │
├────────────────────────────────────────────────┤
│ Acceptance Criteria                            │  ← unchanged, always visible
│  ▸ No acceptance criteria defined.             │
├────────────────────────────────────────────────┤
│ ▼ Description                                  │  ← open by default, 0 clicks
│   Fix the thing so that widgets reconcile...   │
├────────────────────────────────────────────────┤
│ ▶ Comments                                     │
└────────────────────────────────────────────────┘
```

Only the Description chevron/open-state and the presence of its content
region change. No other section, layout, spacing, or visual token changes.

## 3. Interaction flow

| Step | User does | System does |
|---|---|---|
| 1 | Opens a backlog item's detail view | Reads `localStorage["backlog-detail-section-<itemId>-description"]` |
| 2a | (no prior visit to this item) | No stored entry found → falls back to new default `true` → Description renders open, content visible with 0 clicks |
| 2b | (previously manually collapsed Description on this item) | Stored entry `false` found → renders collapsed, per existing persistence contract — unchanged from today |
| 3 | Optionally clicks the Description header to collapse or expand | Radix Accordion toggles open state; `useSectionExpandState` writes the new boolean to that item's localStorage key |
| 4 | Leaves and returns to the same item later | Stored entry (from step 3, if any) is read again and takes precedence over the default, on every subsequent visit |

No new state machine, no new persistence key, no change to how the toggle
itself behaves — only the value used when step 1 finds nothing stored.

## 4. Edge cases

- **Empty description** (`item.description` falsy): still expands by
  default, per research/ux.md §4 — showing "No description." immediately is
  itself informative (mirrors Acceptance Criteria's existing empty-state
  pattern) and avoids a content-dependent branching rule. No new empty-state
  UI is introduced; the existing `<p>No description.</p>` markup is
  unaffected.
- **Non-regression — user previously collapsed manually**: a user who
  toggled Description closed on item X before this change keeps seeing it
  closed on item X after the change ships, because `useSectionExpandState`'s
  stored value always wins over the default. This is expected per
  requirements.md's constraints, not a bug to "fix" by migrating old state.
- **Other sections unaffected**: Acceptance Criteria's always-visible
  behavior, and every other `CollapsibleGroup` section's own stored/default
  state, are untouched — this is a single default-value change scoped to
  the `"description"` section key only.

## 5. UX acceptance criteria

1. Opening any backlog item's detail view for the first time (no stored
   `backlog-detail-section-<itemId>-description` entry) shows the
   Description section already expanded, with its content visible, in 0
   clicks — for both non-empty and empty (`No description.`) descriptions.
2. A user who has never toggled Description on a given item sees it open on
   every fresh visit to that item, until they manually collapse it.
3. A user who previously collapsed Description on item X manually still sees
   it collapsed on revisiting item X after this change ships (no migration,
   no forced re-expand).
4. Collapsing Description remains a single click/tap on its header, from
   either state — no dead end, no added confirmation step.
5. Re-expanding a manually-collapsed Description remains a single
   click/tap, and the choice persists across reloads/navigations away and
   back, exactly as it does today for a manually-collapsed state.
6. Acceptance Criteria's visibility and layout are pixel-identical before
   and after — this change touches no sibling section's rendering.
7. No layout shift/flash on initial mount: Description's open/closed state
   is determined before first paint from the (default-or-stored) value —
   there is no visible "closed then snaps open" transition on load.
8. **Accessibility — keyboard**: Description's trigger is reachable and
   toggleable via the same keyboard interaction (Tab to header, Enter/Space
   to toggle) as every other `CollapsibleGroup` section, both when open and
   closed by default — no new keyboard trap or altered tab order.
9. **Accessibility — screen reader**: Description's trigger exposes
   `aria-expanded="true"` on first render for a no-stored-preference item
   (vs. `"false"` today), and the content region is announced/reachable
   immediately without requiring an extra activation — verified via Radix's
   existing automatic `aria-expanded`/`aria-controls` wiring (no new ARIA
   attributes are hand-authored by this change).
10. No new visual elements or tokens are introduced, so no new contrast or
    styling checks apply beyond confirming the existing open-state
    chevron/content styling (already used for Comments/other sections when
    open) renders correctly for Description too.

## Sources

Design rationale carried forward from `project_plans/backlog-description-prominence/research/ux.md`
(comparable-product convention, JTBD, accessibility read) — not re-derived here.
