# UX Design: Board/Kanban View Toggle for Sessions

Source inputs: `project_plans/board-kanban-view/requirements.md`,
`project_plans/board-kanban-view/research/ux.md`,
`project_plans/board-kanban-view/implementation/plan.md`. This document is the
pre-implementation UX design artifact — wireframes, interaction flows, and
human-testable acceptance criteria for every user-facing surface the plan introduces.

Component names below match the plan exactly: `SessionBoard`, `BoardColumn`, `BoardCard`,
`BoardSwimlane`, `MoveToMenu`, `BulkActions`. Columns: **Running / Needs Review / Paused /
Complete** (`BoardColumnKey`: `running` / `needs_review` / `paused` / `complete`).

---

## Surface 1 — List/Board Toggle Control

### Wireframe

```
┌─ Dashboard Header ──────────────────────────────────────────────────────┐
│  [Search sessions...        ]   Group by: [Status ▾]   ( List | Board ) │
│                                                          ^^^^^^^^^^^^^^  │
│                                                          segmented ctrl │
└───────────────────────────────────────────────────────────────────────┘
```

Segmented control, two buttons, current mode visually pressed/filled:

```
( [●List]  [ Board] )      →  press "Board" or key `b`  →      ( [ List]  [●Board] )
```

### Interaction flow

1. User is in List view with `searchQuery="auth-fix"`, a status filter, and 2 selected
   rows.
2. User clicks "Board" (or presses `b` with focus outside any text input).
3. System: no RPC refetch. `SessionBoard` mounts, consuming the *same*
   `useFilteredGroupedSessions` output List was already using. Search box, filter
   selector, and selection state are visually still present and unchanged.
4. Board renders with the currently-filtered/selected sessions bucketed into columns.
5. Pressing `b` again (or clicking "List") returns to List with the same state intact.

### Error / edge cases

| Case | Behavior |
|---|---|
| `b` pressed while a text input (search box, omnibar, rename field) is focused | No toggle. The character is typed normally into the field. |
| `b` pressed with zero sessions loaded (initial load spinner) | Toggle still flips the mode; the board renders its own loading state (below), not a broken column set. |
| User is mid-drag when `b` is pressed | Not reachable in practice (drag captures pointer focus), but defensively: cancel the drag (treat as `{type:"cancelled"}`, card returns to origin) before switching views, so no in-flight optimistic state leaks into the unmounted board. |

### Loading state (added 2026-08-07, UX-lens triad review gap)

Session data is fetched once via the shared `watchSessions()` stream both List and Board
consume — Board's loading state is only visible on true first paint (no sessions in the
Redux store yet), not on every view switch. `SessionBoard` renders all 4 `BoardColumn`
shells immediately (same structure as Surface 8's empty state) with each column body
showing a skeleton/shimmer placeholder (reuse `SessionList`'s existing loading treatment —
grep for its skeleton component before building a new one) instead of the "No sessions
here" empty-state copy, so a user never mistakes "still loading" for "genuinely empty."
Loading resolves to either populated columns or Surface 8's empty state, never a
stuck/ambiguous middle state.

### Toggle control accessible state (added 2026-08-07, UX-lens triad review gap)

The List/Board segmented control (Surface 1) uses `role="group"` on the wrapper with each
option as a `<button aria-pressed={isActive}>` — the same pattern used for other pressed
toggles already in this codebase (confirm the existing convention via
`grep -rn "aria-pressed" web-app/src/components/` at implementation time and match it,
rather than introducing a second toggle-affordance pattern). Screen readers announce each
button's state as "List, pressed" / "Board, not pressed" (or vice versa), not just "List
button" with no state.

### Screen-reader announcement on view switch (added 2026-08-07, UX-lens triad review gap)

Switching List↔Board (click or `b`) fires one live-region announcement — "Board view,
showing N sessions" / "List view, showing N sessions" — using the same live region Surface
3/10 already wire for drag outcomes (one shared region, not a second one), so a screen
reader user gets confirmation the view actually changed, not just silent DOM replacement.

---

## Surface 2 — Board With 4 Default Columns

### Wireframe

```
┌─ Running (3) ─────┐ ┌─ Needs Review (1) ─┐ ┌─ Paused (2) ───────┐ ┌─ Complete (5) ─────┐
│ ┌────────────────┐ │ │ ┌────────────────┐ │ │ ┌────────────────┐ │ │ ┌────────────────┐ │
│ │ ⠿ fix-login-bug│ │ │ │ ⠿ add-oauth    │ │ │ │ ⠿ refactor-css │ │ │ │ ⠿ update-deps  │ │
│ │   feature/login│ │ │ │   PR #482      │ │ │ │   paused 2h ago│ │ │ │   merged ✓     │ │
│ └────────────────┘ │ │ └────────────────┘ │ │ └────────────────┘ │ │ └────────────────┘ │
│ ┌────────────────┐ │ │                    │ │ ┌────────────────┐ │ │ ┌────────────────┐ │
│ │ ⠿ payment-retry│ │ │                    │ │ │ ⠿ hibernated-wt│ │ │ │ ⠿ old-spike    │ │
│ └────────────────┘ │ │                    │ │ └────────────────┘ │ │ └────────────────┘ │
│ ┌────────────────┐ │ │                    │ │                    │ │        ⋮ (3 more) │
│ │ ⠿ creating...  │ │ │                    │ │                    │ │                    │
│ └────────────────┘ │ │                    │ │                    │ │                    │
└────────────────────┘ └────────────────────┘ └────────────────────┘ └────────────────────┘
```

- `⠿` = drag-handle affordance (also the `MoveToMenu` trigger's neighbor, see Surface 4).
- Column header = `<section aria-label="Running column">` per plan Task 2.1.1b; count
  badge is `aria-label="3 sessions"`, not just visible text.
- "Needs Review" gets the strongest visual accent (color-coded header, per the emotional
  JTBD in `research/ux.md` §5 — this is the column with review-anxiety weight) without
  dropping contrast below AA on the other three.
- `CREATING`/`RESTORING` sessions render inside "Running" with the existing transient
  chip (no 5th column).

### Interaction flow

1. Board mounts → sessions bucket into columns via `getBoardColumnKey`.
2. User scans left-to-right: Running → Needs Review → Paused → Complete — matches the
   natural "in-flight → blocked-on-me → parked → done" reading order.
3. Hovering a card reveals secondary actions (pause/stop/open) consistent with the
   existing `SessionCard` — the board wraps `SessionCard`, it doesn't fork it.

### Error / edge cases

Covered by dedicated surfaces below (empty column: Surface 8; many cards: Surface 9).

---

## Surface 3 — Drag Interaction (Pointer/Mouse)

### Flow diagram

```
[Resting]──pointerdown on ⠿──▶[Grabbed]──drag over column──▶[Hovering target]
                                   │                                │
                                   │ drop on invalid target         │ drop on valid target
                                   ▼                                ▼
                          [Rejected: illegal]              [Optimistic move]
                                   │                                │
                          card bounces back            card renders in new column,
                          + toast + SR announce         pending style, RPC in flight
                                                                    │
                                                     ┌──────────────┴───────────────┐
                                                     ▼                              ▼
                                            RPC succeeds                   RPC fails/rejects
                                          card settles, focus         card reconciles to actual
                                          stays on card, SR           server column, distinct
                                          announces "moved"           toast + SR announce
```

### Interaction flow (happy path)

1. User presses pointer down on a card's drag handle (`⠿`, `aria-label="Drag {title} to
   move"`).
2. Card lifts (visual: shadow/scale), cursor becomes `grabbing`.
3. User drags across column boundaries; the column currently under the pointer
   highlights as a valid or invalid drop target (border/background change) based on
   `isLegalBoardDrag(from, to)` computed live, so the user gets a preview before
   releasing — this is a lower-cost signal than only finding out after drop.
4. User releases over a valid column → card snaps into that column immediately
   (optimistic), shown with a pending/dimmed treatment until `updateSession` resolves.
5. RPC resolves 200 → pending treatment clears, live region announces "Fix login bug
   moved to Paused," focus remains on the card in its new column.

### Error / edge cases

| Case | What the user sees | What the system does |
|---|---|---|
| Drop on illegal target (e.g. Complete → Needs Review) | Card animates back to origin column; toast: "Can't move a completed session to Needs Review." | No RPC fired at all (client-side `isLegalBoardDrag` short-circuit). Live region announces the rejection. |
| Drop is legal client-side but server rejects (`FailedPrecondition`) | Card briefly shows pending state, then re-renders in its **actual current** column (may differ from both origin and intended target) with a highlight flash; toast: "Session already changed state — showing its current status." | Optimistic override cleared; column re-derives from authoritative `sessions` prop, not the stale pre-drag value (mirrors `.claude/rules/go-double-checked-locking.md`'s discipline). |
| Network failure during the mutation | Card reverts to origin column; toast: "Couldn't update session — network error. Try again." (visually distinct from the illegal/rejected toasts — different icon/color) | No state change persisted; user can retry the same drag or use `MoveToMenu`. |
| Another viewer/process changes the session mid-drag (`watchSessions` push arrives while dragging) | Card's rendered column does not change until the user's own drag resolves (frozen at drag-start snapshot) — no yanking a card out from under the cursor. | `inFlightDragSessionId` suppresses live recompute for that one session; once `onDragEnd` fires, reconciliation uses the true current state, same as the two rows above. |
| User presses `Escape` mid-drag | Card returns to origin column, no toast (this is a user-initiated cancel, not a rejection — don't announce it as an error). | `{type:"cancelled"}` outcome; no RPC. |
| Drag released outside any column (e.g. onto the page background) | Same as `Escape` — card returns to origin, no error framing. | `event.over === null` → treated as cancelled, not rejected. |

---

## Surface 4 — Non-Drag "Move to..." Fallback Menu (WCAG 2.1 SC 2.5.7)

This is not optional polish — it is the Level AA conformance path, and per the research
doc it's built as *one* control serving touch, keyboard, and screen-reader users, not
three separate paths.

### Wireframe

```
┌────────────────────┐
│ ⠿  fix-login-bug   │  ⋮  ←  "Move to..." trigger (kebab or dedicated button)
│    feature/login   │
└────────────────────┘
        tap/Enter on the trigger
                │
                ▼
        ┌───────────────────────┐
        │ Move "fix-login-bug"  │
        │ ───────────────────── │
        │ ○ Paused              │
        │ ○ Complete            │
        └───────────────────────┘
        (only legal targets from
         legalBoardTransitions[currentColumn]
         are listed)
```

For a Needs-Review card, the menu shows "Running" (mapped internally to the
`ResolveApproval`-approve path, not a raw status write — same semantics as dragging it
out).

### Interaction flow — keyboard

1. `Tab` reaches the card's `MoveToMenu` trigger button (`aria-label="Move {title} to
   another column"`, `aria-haspopup="menu"`).
2. `Enter`/`Space` opens the menu; focus moves into the menu, first item focused.
3. Arrow keys move between listed columns; `Enter` selects.
4. Selection calls the same `attemptColumnMove` function a drag-and-drop would have
   called (Task 4.1.1a) — one shared code path, so behavior (optimistic update, RPC,
   rejection handling, live-region announcement) is identical to Surface 3's outcomes.
5. On success, focus returns to the trigger button, now inside the card's new column
   location (not lost to `<body>`).
6. `Escape` at any point in the open menu closes it with no action taken.

### Interaction flow — touch

1. Tap the trigger (kebab/"Move to..." affordance) — same target, ≥44×44px hit area.
2. Tap a listed column — same `attemptColumnMove` path, same feedback.
3. Tap outside the menu, or the trigger again, to dismiss without acting.

### Error / edge cases

| Case | Behavior |
|---|---|
| Card is in "Complete" (`legalBoardTransitions["complete"] = []`) | Menu still opens but shows "No moves available from Complete" rather than an empty, silent menu — avoids a dead-end where the control appears broken. |
| Selecting a target and the RPC fails | Identical toast/live-region treatment to Surface 3's rejection rows — same shared function, same outcome types. |
| Menu open, user presses `b` (view-toggle shortcut) | Menu's own `Escape`/outside-click handling takes priority; `b` should not fire the global view-toggle while a menu has focus (treat the open menu as a focused non-text-input control that still intercepts single-letter global shortcuts — verify at implementation against the existing `useKeyboard` ignore-list mechanism). |

---

## Surface 5 — Swimlane-Grouping-as-Rows Mode

### Wireframe

```
Group by: [Branch ▾]

┌─ feature/login ──────────────────────────────────────────────────────────┐
│ Running (1)        Needs Review (0)     Paused (1)         Complete (0)  │
│ ┌────────────┐     (empty)              ┌────────────┐                   │
│ │fix-login   │                          │old-attempt │                   │
│ └────────────┘                          └────────────┘                   │
└────────────────────────────────────────────────────────────────────────┘
┌─ main ────────────────────────────────────────────────────────────────────┐
│ Running (0)        Needs Review (0)     Paused (0)         Complete (1)  │
│ (empty)                                                    ┌────────────┐│
│                                                              │hotfix-123  ││
│                                                              └────────────┘│
└────────────────────────────────────────────────────────────────────────┘
```

Status remains the **column** axis at all times; the grouping strategy (branch, tag,
category, etc.) becomes horizontal **row** dividers — a 2D grid, not a replacement of
columns. This preserves "drag = status change" regardless of which swimlane axis is
active (see plan's Pattern Decision "Swimlane axis vs. status columns").

### Interaction flow

1. User has been in default (status-only) board view.
2. User picks "Branch" from the existing grouping-strategy selector (same control List
   view already uses).
3. Board re-renders as N `BoardSwimlane` rows, each with its own 4-column set and its
   own per-column counts.
4. Dragging a card within its row behaves exactly like Surface 3 (mutates status only).
   Dragging is not supported *between* rows (there is no "move to a different branch"
   drag gesture — row membership is derived from session data, not draggable).
5. Switching grouping strategy back to "None"/"Status" collapses back to the single-row
   default board.

### Error / edge cases

| Case | Behavior |
|---|---|
| A grouping value with zero sessions in every column (e.g. an empty branch bucket somehow surfaced) | Row is not rendered at all — `groupSessions` only returns rows with ≥1 member, consistent with List view's existing grouping behavior. |
| `Tag` grouping, session has multiple tags | Same session's card renders once per matching tag row (duplicate DOM nodes, same session ID) — dragging any instance mutates the one underlying session's status; both instances reflect the new column on next render. This is called out explicitly so a user isn't confused into thinking they have two sessions. |
| Very many swimlane rows (e.g. 20+ branches) | Rows scroll vertically (page-level scroll), each row's columns scroll horizontally independently — avoid a single mega-grid that requires both-axis scrolling simultaneously, which is disorienting. Flag to implementation/testing as a scroll-interaction case worth a manual check. |

---

## Surface 6 — Search-Across-Columns

### Wireframe

```
Search: [ login                              ]

┌─ Running (1) ──────┐ ┌─ Needs Review (0) ──┐ ┌─ Paused (1) ────────┐ ┌─ Complete (0) ─────┐
│ ┌────────────────┐ │ │ (empty — no matches)│ │ ┌────────────────┐  │ │ (empty — no matches)│
│ │fix-login-bug   │ │ │                      │ │ │old-login-flow  │  │ │                     │
│ └────────────────┘ │ │                      │ │ └────────────────┘  │ │                     │
└────────────────────┘ └──────────────────────┘ └─────────────────────┘ └─────────────────────┘
```

### Interaction flow

1. User types into the existing instant-search box (same box used in List view — it is
   not duplicated per view).
2. On every keystroke, `filteredSessions` (from the shared `useFilteredGroupedSessions`
   hook) updates; `SessionBoard` re-buckets from that filtered set, not the raw session
   list.
3. Every column's count badge reflects only the filtered subset — a column that had 5
   cards may now show "1" or "0".
4. Clearing the search restores all columns to their unfiltered counts instantly (no
   RPC refetch — this is client-side filtering over already-loaded data).

### Error / edge cases

| Case | Behavior |
|---|---|
| Search matches zero sessions across the whole board | All 4 columns show their individual empty-state message (Surface 8's treatment), not a single board-wide "no results" message replacing the columns — keeps the column structure visible so the user can immediately see search is filtering, not that the board is broken. |
| Search query cleared while a card was mid-drag | Not reachable (search box and drag are mutually exclusive input focus); no special handling needed beyond the existing `b`-during-menu consideration. |

---

## Surface 7 — Bulk-Select-Across-Columns

### Wireframe

```
☑ fix-login-bug (Running)         ☑ add-oauth (Needs Review)        ☐ refactor-css (Paused)

┌───────────────────────────────────────────────────────────────────────────┐
│  2 selected     [ Pause ]   [ Stop ]   [ Clear selection ]                │
└───────────────────────────────────────────────────────────────────────────┘
                        ^ BulkActions bar, same component as List view
```

### Interaction flow

1. User enters select mode (same affordance as List view — e.g. long-press, checkbox
   toggle, or a "Select" mode button already present).
2. Checkboxes appear on each `BoardCard` (threaded through to the wrapped `SessionCard`,
   which already supports `isSelected`/`onToggleSelect`).
3. User checks one card in "Running" and one in "Needs Review".
4. The same `BulkActions` bar List view uses appears, `selectedCount={2}`, independent of
   which columns those 2 selections live in.
5. Clicking "Pause" fires the bulk action across both selected sessions regardless of
   column.
6. **Dragging a selected card**: if the user drags a card that is part of the active
   selection, the whole selection moves together (one RPC call per selected session,
   same target status) — matches Trello/Linear precedent, not a silent single-card move
   that would contradict what "selected" visually implies.

### Error / edge cases

| Case | Behavior |
|---|---|
| User drags a card that is *not* part of the current selection, while a selection exists elsewhere | Only the dragged card moves — the existing selection is left untouched (don't silently expand it). |
| Bulk action partially fails (e.g. 1 of 2 selected sessions rejects the transition) | Per-session outcome is reported — the session that succeeded moves/updates, the one that failed shows its own rejection toast/highlight, not a single opaque "bulk action failed" message that hides which one worked. |
| Selecting cards, then switching to List view | Selection persists (same underlying `selectedSessions` state, not duplicated per view) — consistent with the toggle's "no state loss" contract in Surface 1. |
| Selecting a card that's rendered twice due to `Tag` swimlane multi-membership | Selecting either DOM instance selects the one underlying session; both instances show the checked state (same session ID drives both). |

---

## Surface 8 — Empty Column State

### Wireframe

```
┌─ Complete (0) ─────┐
│                     │
│    ┌───────────┐    │
│    │   ∅        │    │
│    └───────────┘    │
│   No sessions here  │
│                     │
└────────────────────┘
```

### Interaction flow

1. A column with zero matching sessions still renders its full shell (header, count
   badge showing "0") — it does not collapse to zero width or disappear.
2. The body shows a short, low-emphasis empty-state message ("No sessions here" or
   similar), reusing the existing `SessionListEmptyState` visual language rather than a
   new pattern.
3. The column remains a valid drop target — dragging a legal card into an empty column
   works identically to a populated one (the empty-state message is replaced by the
   card, not overlaid).

### Error / edge cases

| Case | Behavior |
|---|---|
| All 4 columns are empty (new workspace, no sessions yet) | Board renders 4 empty-shell columns side by side (not a single board-wide "no sessions at all" takeover) — consistent with Surface 6's "keep structure visible" principle, and gives the user an immediate mental map of where sessions *will* appear once created. |
| Column becomes empty as a result of a drag (last card dragged out) | Column transitions from populated to its empty-state treatment smoothly (no flash of a completely blank column mid-transition). |

---

## Surface 9 — Column With Many Cards (Virtualized)

### Wireframe

```
┌─ Complete (247) ───┐
│ ┌────────────────┐ │  ← only visible cards are
│ │ card 1         │ │    actually mounted in the DOM
│ └────────────────┘ │    (per-column virtualization,
│ ┌────────────────┐ │    not board-wide)
│ │ card 2         │ │
│ └────────────────┘ │
│ ┌────────────────┐ │
│ │ card 3         │ │
│ └────────────────┘ │
│         ⋮           │  ← scroll thumb indicates
│   (scrollbar)        │    244 more below
└────────────────────┘
```

### Interaction flow

1. User scrolls within a single column's card list (`overflowY: auto` on that column
   only — not the whole board).
2. Cards mount/unmount as they enter/leave the virtualization window; count badge always
   shows the true total ("247"), never the visible-window count.
3. Dragging a card from a long column works the same as any other column — drop targets
   are computed per-column, not requiring every card in the list to be mounted
   simultaneously (per-column virtualization avoids the cross-column collision-detection
   rough edge the plan flags for board-wide virtualizers).

### Error / edge cases

| Case | Behavior |
|---|---|
| User attempts to drag a card that is currently scrolled out of view in another column, into a long column | Not applicable — you can only drag a card that's currently rendered/visible; there is no "drag from off-screen" gesture. `MoveToMenu` remains available as an alternative that doesn't require the target column to be scrolled into view first (Surface 4). |
| Column has 1000+ cards (extreme case) | Virtualization keeps render cost bounded regardless of count; no artificial cap is applied (an arbitrary cap would silently hide sessions, which the research doc explicitly flags as unacceptable). |
| Scrolling a column while a live push adds a new card to it | New card is inserted at its correct sorted position; if the user is scrolled away from that position, no forced scroll-jump — the count badge updates immediately as the visible signal that something changed. |

---

## Surface 10 — Error/Rejected-Drag State (Consolidated Reference)

This surface consolidates the three distinct rejection/failure flavors introduced across
Surfaces 3–4, since they must be visually and semantically distinguishable from each
other (a user who sees "my drag failed" needs to know *why* to know what to do next).

| Outcome | Trigger | Visual treatment | Toast copy (example) | Live-region announcement | Recovery action |
|---|---|---|---|---|---|
| `rejected_illegal` | Client-side `isLegalBoardDrag` returns false | Card bounces back to origin instantly, no pending state | "Can't move a completed session to Needs Review." | "Can't move {title} to {column}." | None needed — user picks a valid target instead (highlighted live during drag, per Surface 3). |
| `rejected_by_server` | RPC returns `FailedPrecondition` (state changed underneath the drag) | Card briefly pends, then re-renders in its **actual current** column with a highlight flash | "Session already changed state — showing its current status." | "{title} already changed — now shown as {actual column}." | None needed — the board is now showing truth; user can re-attempt if they still want the original move. |
| `network_error` | RPC call itself fails (timeout, offline, 5xx) | Card reverts to origin column, pending state clears | "Couldn't update session — network error. Try again." | "Couldn't move {title} — network error." | Retry the drag or `MoveToMenu` action; toast optionally includes a "Retry" affordance. |
| `cancelled` | User-initiated: `Escape`, drop outside any column | Card returns to origin, silently | (none) | (none) | N/A — this is not an error, no messaging needed. |

**Design principle carried through all three real error rows**: never leave the user
unsure whether their action "sort of happened." Every non-cancelled outcome ends in one
of exactly two states — moved, or visibly and explainably not moved — with no
intermediate ambiguous state left on screen after the toast/announcement fires.

---

## Surface 11 — Mobile/Touch Layout (<768px)

### Wireframe

```
┌ 375px viewport ───────────────┐
│ [Search...]      (List|Board) │
│ Group by: [Status ▾]          │
│                                │
│ ┌─ Running (3) ──────────────┐│ ← one column ≈ full viewport width,
│ │ ┌────────────────────────┐ ││   horizontally swipeable, scroll-snap
│ │ │ ⠿  fix-login-bug    ⋮  │ ││   between columns
│ │ └────────────────────────┘ ││
│ │ ┌────────────────────────┐ ││
│ │ │ ⠿  payment-retry    ⋮  │ ││
│ │ └────────────────────────┘ ││
│ └────────────────────────────┘│
│      ● ○ ○ ○   ← column position dots (Running/NeedsReview/Paused/Complete)
└────────────────────────────────┘
        swipe left/right to change column
```

- The `⋮` "Move to..." trigger (Surface 4) is the **primary documented interaction** at
  this width — drag remains technically available via `TouchSensor` (200ms hold +
  8px tolerance, per the plan's touch-activation constraint) but is not the expected
  path, since unreliable drag-on-touch is exactly the risk AC10 calls out.
- `touch-action: none` is scoped to the drag handle only (`⠿`), never the card body —
  a user's vertical scroll gesture on the card itself must never be hijacked into a drag
  attempt.

### Interaction flow

1. User opens the dashboard on a phone, toggles to Board (tap or the toggle control —
   `b` shortcut is desktop-only in practice since it needs a hardware keyboard, but
   nothing disables it on a mobile browser with a keyboard).
2. One column is shown at a time, sized to the viewport; horizontal swipe (native
   scroll, `scroll-snap-type: x mandatory`) moves between Running/Needs
   Review/Paused/Complete.
3. To move a session, the user taps the card's `⋮`/"Move to..." trigger (Surface 4's
   touch flow) rather than attempting a drag — larger hit target, no risk of an
   accidental drag from a scroll gesture.
4. If the user does attempt a long-press drag, it works (TouchSensor is wired), but the
   UI does not require it and does not visually suggest it's the only way (the
   `MoveToMenu` trigger is always visible on every card at this width, not hidden behind
   a hover state that doesn't exist on touch).

### Error / edge cases

| Case | Behavior |
|---|---|
| User attempts a drag but lifts before the 200ms activation delay | No drag starts; treated as a tap — if it landed on the drag handle specifically, nothing happens (handle isn't a tap target for anything else); if elsewhere on the card, normal card-tap behavior (open detail) applies. |
| User swipes vertically inside a column (scrolling cards) vs. horizontally (changing column) | Vertical scroll stays within the column's card list; horizontal scroll/swipe moves between columns — the two gestures must not conflict (this is a manual QA checkpoint, not something ASCII can fully specify). |
| Very narrow viewport (<375px, e.g. older/small phones) | Column width still ≈ viewport width (not a fixed px value that could overflow) — uses the existing `calc(100vw - 2 * spacing)` pattern from the plan rather than a hardcoded `320px`. |
| Rotating device orientation mid-scroll | Column re-snaps to the nearest column boundary on resize, doesn't leave the view mid-column. |

---

## UX Acceptance Criteria

Each item below is written to be verifiable by a human tester without reading source
code — a click count, an exact visible string, or an observable state.

### Toggle & persistence

1. From the dashboard in List view with a search query and an active status filter set,
   clicking "Board" (1 click) or pressing `b` switches to Board view in under 300ms
   perceived latency, with the same search query still visible in the search box and the
   same filter still applied (verify by comparing visible card counts before/after — they
   must match the filtered result, not the unfiltered total).
2. Reloading the browser after switching to Board view restores Board view (not List) on
   the same workspace, with no visible flash of List view before Board renders.
3. Switching to a different workspace (via the workspace switcher) and back does not
   carry over the first workspace's view-mode choice — each workspace's List/Board
   preference is independent (test: set workspace A to Board, switch to workspace B,
   confirm B opens in List by default if never set there).
4. Pressing `b` while typing in the search box inserts the letter "b" into the search
   text and does **not** change the view mode.

### Column rendering

5. All 4 columns (Running, Needs Review, Paused, Complete) are visible (or reachable via
   one swipe/scroll on mobile) at all times, in that fixed left-to-right order, even when
   one or more is empty.
6. Every column header's count badge is readable via screen reader as "N sessions" (not
   just a bare number) — verify with VoiceOver/NVDA/a screen-reader emulator.
7. A session with status `PAUSED` and a session with status `HIBERNATED` both appear in
   the "Paused" column, visually distinguished from each other only by their existing
   status chip (not by being in different columns).

### Drag interaction

8. Dragging a card from Running to Paused completes in one continuous pointer gesture
   (press, drag, release) with no intermediate confirmation dialog, and the card is
   visibly present in the Paused column within 1 second of release (optimistic).
9. Dragging a card to an illegal target column (e.g. Complete → Needs Review) results in
   the card visibly returning to its original column within 1 second, accompanied by a
   toast message that names the specific reason (not a generic "Error" or "Something
   went wrong").
10. During any drag, the column currently under the pointer shows a visibly different
    treatment (highlight/border) depending on whether dropping there would succeed or be
    rejected — a tester can distinguish "this will work" from "this won't" before
    releasing.
11. No card can be dropped into a state where it visually appears in two columns at once
    or in neither column, at any point during or after a drag (including on a forced
    network failure — simulate via devtools offline mode).

### Non-drag fallback (WCAG 2.1 SC 2.5.7)

12. Every visible card has a non-drag "Move to..." control reachable by `Tab` alone (no
    mouse), operable with `Enter`/`Space` to open and arrow keys + `Enter` to select,
    with zero drag gesture required to move a session between any two valid columns.
13. The "Move to..." menu for a card in "Complete" either lists no destinations with an
    explicit "No moves available" message, or is absent entirely — it never opens to a
    blank/empty dropdown with no explanatory text.
14. Completing a move via "Move to..." produces the identical end state (card in target
    column, toast/announcement on success or failure) as completing the same move via
    drag — a tester performing the same logical move both ways sees no difference in
    outcome.
15. After a successful "Move to..." action, keyboard focus is on the card (or its
    trigger) in its new column location — pressing `Tab` immediately after moves to the
    *next* element in that new context, not back to page top or a lost/`body` focus
    state (verify visually via focus ring, or via a screen reader's focus announcement).

### Screen reader / accessibility

16. A screen reader user is announced, via a live region, each of: pick-up ("grabbed"),
    a successful move ("{title} moved to {column}"), an illegal rejection ("Can't move
    {title} to {column}"), and a server-side rejection with different wording than the
    illegal case — a tester using a screen reader can distinguish all three announcement
    types by text content alone, without seeing the screen.
17. Every column header text and every status/count badge meets 4.5:1 contrast against
    its background in both light and dark theme (verify with a contrast-checker browser
    extension against the shipped CSS, not just the design mockup).
18. Every interactive control on a card (drag handle, "Move to..." trigger, checkbox in
    select mode) has a visible focus indicator when reached via `Tab`, distinguishable
    from the resting state without relying on color alone.

### Swimlanes / grouping

19. Selecting "Branch" as the grouping strategy while in Board view adds one horizontal
    row per branch, each still showing all 4 status columns — a tester can confirm by
    counting: (number of rows) × 4 = (number of column-shells visible), and status
    columns are never replaced or hidden by the grouping change.
20. A session with two tags, when grouped by Tag, is visible in both matching tag rows
    simultaneously — a tester can locate the same session title in two different rows at
    once.

### Search & bulk-select

21. Typing a search query that matches sessions in only 2 of the 4 columns causes the
    other 2 columns to show their empty-state message (not disappear), and every visible
    column's count badge reflects only the filtered subset — verified by counting visible
    cards against the badge number in each column.
22. Selecting one card in Running and one card in Needs Review shows a bulk-action bar
    reading "2 selected" (exact count, not "2+" or a stale number), and clicking a bulk
    action (e.g. Pause) results in both sessions changing state, regardless of their
    starting columns.
23. Switching from Board to List view with 2 cards selected preserves the same 2-item
    selection in List view (verified via the same bulk-action bar/count reappearing
    there unchanged).

### Empty / high-volume columns

24. A column with zero sessions still shows its header and count badge reading "0" and a
    visible "No sessions" (or equivalent) message in its body — it is never a blank area
    indistinguishable from a loading/broken state.
25. A column with 100+ sessions scrolls smoothly (no visible frame stutter on a mid-range
    device scroll test) and its count badge always shows the true total, not the number
    of currently-rendered/visible cards.

### Mobile / touch

26. On a 375px-wide viewport, only one column is fully visible at a time, and a
    horizontal swipe moves cleanly to the adjacent column with a visible snap (no
    settling at an off-column, partial-scroll position).
27. On a touch device, every card's "Move to..." trigger is directly tappable (no
    hover-reveal required) and meets a minimum 44×44px touch target (verify by
    measurement in devtools' touch/mobile emulation).
28. A vertical swipe/scroll gesture starting on a card's body (not its drag handle)
    scrolls the column's card list and does not initiate a drag.
29. A tester can complete a full Running → Complete move on a touch-only device (no
    mouse, no keyboard) using only tap gestures, without ever successfully or
    unsuccessfully attempting a drag.

### Loading, toggle state, view-switch announcement (added 2026-08-07)

31. On first load before any session data has arrived, Board view shows all 4 column
    shells with skeleton/shimmer placeholders in each body — never the "No sessions here"
    empty-state copy before the initial fetch resolves, and never a blank/broken layout.
32. Each segmented-control button (List, Board) exposes its pressed state via
    `aria-pressed`, verifiable with a screen reader announcing "pressed"/"not pressed" (or
    equivalent) for the active option.
33. Switching between List and Board (click or `b`) triggers one live-region announcement
    naming the new view and the visible session count — a screen reader user can confirm
    the switch happened without visually inspecting the page.

### No dead ends

30. At every step described above — toggle, drag, fallback menu, search, bulk-select,
    swimlane switch — a tester can always return to a known-good state (List view, or
    Board view with all sessions visible and no stuck pending/error state) using only
    visible, discoverable controls, with no case requiring a page reload to recover
    (network-failure reverts, illegal-drag bounce-backs, and cancelled drags all
    self-resolve without a refresh).
