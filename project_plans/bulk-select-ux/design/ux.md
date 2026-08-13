# UX Design: Bulk Session Selection

**Date**: 2026-06-23
**Feature**: Bulk session selection — row mode parity, hover-reveal, keyboard shortcuts, undo-on-delete
**Surfaces**: 7
**UX Acceptance Criteria**: 29

---

## Surface Inventory

1. Session list row — idle state (no selection active)
2. Session list row — select mode active
3. Session list card — idle and select mode
4. BulkActions toolbar — count display, action buttons, state variants
5. Undo toast notification (bottom-center, via NotificationContext)
6. Master "Select All" control (list header)
7. Mobile / touch form factor — all surfaces

---

## Surface 1: Session Row — Idle State (No Selection Active)

### Layout

Row view uses a CSS grid. The checkbox column is always reserved (24 px) to prevent layout shift when entering select mode. The checkbox is visually hidden at rest and revealed on hover.

```
IDLE ROW (no hover, no selection):

┌─────────────────────────────────────────────────────────────────────────┐
│ [  ] ● claude-code  /projects/foo    running   2h 14m   [▶] [⏸] [✕]  │
│  ^                                                                       │
│  24px reserved, invisible                                               │
└─────────────────────────────────────────────────────────────────────────┘

IDLE ROW (hover, cursor over row):

┌─────────────────────────────────────────────────────────────────────────┐
│ [☐] ● claude-code  /projects/foo    running   2h 14m   [▶] [⏸] [✕]  │
│  ^                                                                       │
│  ghost checkbox appears (opacity ~60%, border only)                     │
└─────────────────────────────────────────────────────────────────────────┘
```

**Grid columns** (left to right):
- 24px — checkbox cell (always-reserved; visibility controlled by CSS)
- 8px  — status dot column
- 1fr  — session name + path
- auto — status label ("running", "paused", "stopped")
- auto — elapsed time
- 32px — actions (play/pause)
- auto — delete button

**Active / current session variant** — the session open in the terminal panel:

```
┌──────────────────────────────────────────────────────────────────────────┐
│▌[☐] ● claude-code  /projects/foo    running   2h 14m   [▶] [⏸] [✕]  │
│^                                                                          │
│3px left border in status-green (#22c55e or var(--success))              │
│Bold session name. aria-current="true" on the row element.               │
└──────────────────────────────────────────────────────────────────────────┘
```

### Interaction Flow

| User action | System response |
|---|---|
| Cursor enters row | Checkbox ghost fades in (CSS transition, ~100ms). No React state change. |
| Cursor leaves row (no selection) | Checkbox ghost fades out. |
| User clicks checkbox (first selection) | `selectMode` becomes `true`. All row checkboxes become persistently visible. BulkActions toolbar slides in from bottom. `lastAnchorId` is set to this session ID. |
| User clicks row body (not checkbox) | Navigates to session terminal — same as today. |

### Edge Cases

- Row is both active (aria-current) and selected: shows left border AND background tint AND filled checkbox. Three independent visual indicators, none shared.
- Row is off-screen (virtualized, not in DOM): no visual state needed. On scroll-back-into-view, the row is re-mounted with `isSelected` prop from the parent Set — no stale state.
- Row with a very long session name: name truncates with ellipsis. Checkbox cell does not shrink (fixed 24px).

---

## Surface 2: Session Row — Select Mode Active

### Layout

Once `selectMode=true` (at least one session selected), `data-select-mode="true"` is set on the list container. All checkboxes become persistently visible via CSS cascade — no per-row prop change needed.

```
SELECT MODE — unselected row:

┌─────────────────────────────────────────────────────────────────────────┐
│ [☐] ● aider        /projects/bar    stopped   0h 45m   [▶] [⏸] [✕]  │
│  ^                                                                       │
│  checkbox visible, unchecked, pointer-events enabled                    │
└─────────────────────────────────────────────────────────────────────────┘

SELECT MODE — selected row:

┌─────────────────────────────────────────────────────────────────────────┐
│ [☑] ● aider        /projects/bar    stopped   0h 45m   [▶] [⏸] [✕]  │
│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│
│ background: --session-selected-bg (indigo at 8% opacity, NOT status    │
│ green and NOT the active-session blue/green border color)              │
└─────────────────────────────────────────────────────────────────────────┘

SELECT MODE — Shift+click in progress (range preview):

  Row 1  [☑] ● session-alpha   (anchor — plain-clicked first)
  Row 2  [☑] ● session-beta    (in range)
  Row 3  [☑] ● session-gamma   (Shift+clicked — range endpoint)
  Row 4  [☐] ● session-delta   (outside range — unselected)

Range is computed from flatItems order. Group headers in between are skipped.
```

### Interaction Flow

| User action | System response |
|---|---|
| Click unselected row checkbox | Adds session to selection. `lastAnchorId` updates to this session. |
| Click selected row checkbox | Removes session from selection. If last session deselected, `selectMode` remains `true` until Escape. |
| Shift+click any row | Computes range from `lastAnchorId` to clicked row (inclusive, flat order, headers skipped). Replaces current selection with range. `lastAnchorId` does NOT change. |
| Click row body (non-checkbox area) in select mode | Navigates to session (existing behavior). Does NOT change selection. |
| Press `X` with row keyboard-focused | Toggles selection on focused row. Enters select mode if first selection. |

### Edge Cases

- Shift+click before any anchor: treat as plain click (anchor is null — no range possible).
- Shift+click across group headers: range includes all sessions between endpoints in `flatItems` order, regardless of group boundaries.
- Shift+click on already-selected anchor: selects only the anchor itself (range of one).
- Filter applied after selection: `activeSelection = selectedSessions ∩ filteredSessions`. The displayed count and bulk-action targets use `activeSelection`. Sessions hidden by the filter stay in `selectedSessions` in memory but do not appear in the count or operations.

---

## Surface 3: Session Card — Idle and Select Mode

Card view already supports selection; this section documents the design to ensure parity and highlight the key visual distinction between active and selected states.

```
CARD — IDLE (no selection active):

┌──────────────────────────────────────────────────┐
│ [☐ ghost on hover]  claude-code                 │
│ /projects/foo                                    │
│ Status: running   2h 14m                        │
│ [▶ Resume] [⏸ Pause] [✕ Delete]                │
└──────────────────────────────────────────────────┘

CARD — SELECTED:

┌──────────────────────────────────────────────────┐
│ [☑]  claude-code                                │
│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│
│ /projects/foo                                    │
│ Status: running   2h 14m                        │
│ [▶ Resume] [⏸ Pause] [✕ Delete]                │
└──────────────────────────────────────────────────┘
  background: --session-selected-bg
  checkbox: top-left, filled blue

CARD — ACTIVE (currently open in terminal):

┌──────────────────────────────────────────────────┐
│▌[☐]  claude-code                                │
│ /projects/foo                                    │
│ Status: running   2h 14m                        │
│ [▶ Resume] [⏸ Pause] [✕ Delete]                │
└──────────────────────────────────────────────────┘
  3px left border in --success color
  aria-current="true"
  NO background tint (not selected for bulk)

CARD — ACTIVE + SELECTED:

┌──────────────────────────────────────────────────┐
│▌[☑]  claude-code                                │
│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│
│ /projects/foo                                    │
│ Status: running   2h 14m                        │
│ [▶ Resume] [⏸ Pause] [✕ Delete]                │
└──────────────────────────────────────────────────┘
  Both indicators present simultaneously.
  Left border: status-green (aria-current)
  Background tint: --session-selected-bg (aria-selected)
```

### Interaction Flow

Identical to Surface 2 for row mode. The card's checkbox is positioned top-left; clicking it follows the same anchor/range/toggle model.

---

## Surface 4: BulkActions Toolbar

### Layout

The BulkActions toolbar is a floating bar docked at the **bottom** of the session list view. It appears when `selectMode=true` and disappears (slides out) when `selectMode=false`.

```
BULK ACTIONS — PARTIAL SELECTION (3 of 12):

┌──────────────────────────────────────────────────────────────────────────────┐
│  [☑ indeterminate]  3 sessions selected       [Pause 3] [Delete 3] [✕ Clear]│
│     ^                     ^                       ^           ^           ^  │
│  master checkbox    aria-live count          context-     destructive   Escape│
│  (indeterminate:    (polite announcement)    aware btn    action btn    hint  │
│  some not all)                                                                │
└──────────────────────────────────────────────────────────────────────────────┘

BULK ACTIONS — ALL SELECTED (12 of 12):

┌──────────────────────────────────────────────────────────────────────────────┐
│  [☑ checked]  12 sessions selected (⌘A)    [Pause 12] [Delete 12] [✕ Clear] │
│                                                                               │
└──────────────────────────────────────────────────────────────────────────────┘

BULK ACTIONS — MIXED STATUS (3 paused + 2 running selected):

┌──────────────────────────────────────────────────────────────────────────────┐
│  [☑ indeterminate]  5 sessions selected      [Resume 3] [Pause 2] [Delete 5]│
│                                                  ^           ^               │
│                                             context-aware: shows Resume and  │
│                                             Pause separately for mixed state │
└──────────────────────────────────────────────────────────────────────────────┘

BULK ACTIONS — ALL PAUSED (5 paused selected):

┌──────────────────────────────────────────────────────────────────────────────┐
│  [☑ indeterminate]  5 sessions selected      [Resume All 5]  [Delete 5]  [✕]│
│                                                   ^                          │
│                                        "Pause" button de-emphasized/hidden   │
│                                        since all selected are already paused │
└──────────────────────────────────────────────────────────────────────────────┘

KEYBOARD HINTS (appended to action labels):
- "Select All" button shows "(⌘A)" on macOS, "(Ctrl+A)" on other platforms
- "Clear / Cancel" button shows "(Esc)"
```

**Master checkbox behavior:**
- 0 selected: unchecked (toolbar not shown)
- Some selected (1 to N-1): indeterminate (dash icon). Set via `el.indeterminate = true` on the DOM node — NOT via `aria-checked="mixed"` on the native checkbox.
- All selected: checked.
- Clicking indeterminate: selects all (transition to all-selected state).
- Clicking checked (all selected): deselects all (transition to 0 selected — toolbar hides).

**Count display:**
- Shows `activeSelection.size` (intersection of `selectedSessions` and current `filteredSessions`).
- Has `aria-live="polite"` and `aria-atomic="true"` for screen reader announcements.
- Format: `"N session selected"` (singular) / `"N sessions selected"` (plural).

**Context-aware action buttons:**

| Selection status mix | Buttons shown |
|---|---|
| All paused | [Resume All N] [Delete N] |
| All running | [Pause All N] [Delete N] |
| All stopped | [Delete N] (pause/resume not applicable) |
| Mixed paused + running | [Resume X] [Pause Y] [Delete N] |
| Mixed with stopped | [Resume X] [Pause Y] [Delete N] |

"Delete" is always shown and always styled as the destructive action (distinct color, e.g. `--error` token).

### Interaction Flow

| User action | System response |
|---|---|
| Click master checkbox (indeterminate) | `handleSelectAll()` — all `filteredSessions` added to `selectedSessions`. |
| Click master checkbox (all-selected) | `handleClearSelection()` — `selectedSessions` cleared, `selectMode` becomes `false`. |
| Click "Delete N" | Optimistic removal of `activeSelection` sessions from displayed list. Undo toast shown. No RPC fired yet. |
| Click "Pause N" or "Resume N" | Fires per-session pause/resume RPCs in parallel. Selection clears. Toolbar hides. |
| Click "Clear" / "Cancel" | `handleClearSelection()` — same as Escape. |

### Edge Cases

- Count becomes 0 because a filter removed all selected items: toolbar stays visible but count shows "0 sessions selected" and action buttons are disabled. Clear button remains enabled so user can exit select mode.
- User triggers bulk action while a previous undo window is pending: previous pending delete is flushed immediately (RPCs fire), then the new action proceeds.
- All sessions deleted from the list (0 sessions remaining): toolbar hides, `selectMode` becomes `false`.

---

## Surface 5: Undo Toast Notification

### Layout

The undo toast appears at the **bottom-center** of the viewport after a bulk delete. It is non-blocking — the user can continue interacting with the session list while the toast is visible.

```
UNDO TOAST (5-second window):

                    ┌──────────────────────────────────────────┐
                    │  🗑  Deleted 5 sessions      [Undo]  [✕] │
                    └──────────────────────────────────────────┘
                                     ^
                           bottom-center of viewport
                           above any fixed footer / nav

UNDO TOAST — timing bar (visual countdown):

                    ┌──────────────────────────────────────────┐
                    │  🗑  Deleted 5 sessions      [Undo]  [✕] │
                    ├████████████████████░░░░░░░░░░░░░░░░░░░░░│  <- optional
                    └──────────────────────────────────────────┘
                    thin progress bar depleting over 5s (optional
                    enhancement — helps users know how long they have)

UNDO TOAST — undo clicked (toast dismisses, sessions reappear):

                    Sessions animate back into their original positions.
                    No RPC has been called.

UNDO TOAST — timer expired (toast auto-dismisses):

                    RPCs fire for each pending session via Promise.allSettled.
                    Sessions are permanently deleted from server.

UNDO TOAST — partial delete failure (some RPCs failed):

                    ┌──────────────────────────────────────────────────────────────┐
                    │  ⚠  3 deleted, 2 failed — failed sessions are back in list  │
                    └──────────────────────────────────────────────────────────────┘
                    Error feedback shown for 5 seconds. No undo button.
                    Failed sessions reappear in the list immediately.
                    Select mode is NOT re-entered.

UNDO TOAST — full delete failure (all RPCs failed):

                    ┌──────────────────────────────────────────────────────────────┐
                    │  ⚠  All 5 deletes failed — sessions are back in the list    │
                    └──────────────────────────────────────────────────────────────┘
                    Error feedback shown for 5 seconds. No undo button.
                    All sessions reappear in the list. Select mode is NOT re-entered.
```

**Positioning rationale**: The app has a fixed left sidebar. Bottom-left overlaps the sidebar. Top-right is too far from where destructive actions are taken. Bottom-center is the established pattern (Gmail, Material Design snackbar guidelines).

**Stacking behavior**: Only one undo toast at a time. A second bulk delete while a toast is active:
1. Flushes the first pending delete immediately (fires all queued RPCs).
2. Dismisses the first toast.
3. Shows a new toast for the second operation.

**Toast message format**: `"Deleted N session"` (N=1) / `"Deleted N sessions"` (N>1).

**Accessibility**:
- Toast container has `role="status"` and `aria-live="polite"` so screen readers announce it without interrupting the user.
- "Undo" button has `aria-label="Undo the last bulk delete"`.
- "Dismiss" (✕) button has `aria-label="Dismiss notification"`.
- Toast stays visible for a minimum of 5 seconds to meet WCAG 2.1 timing guidance for actions on toasts.
- **WCAG 2.4.3 — Undo button receives focus on mount**: When the toast mounts with `notificationType === "undo"`, the Undo button MUST receive programmatic focus via `undoButtonRef.current?.focus()` inside a `useEffect` (equivalent to `autoFocus`, but without the scroll-into-view side-effect). Without this, keyboard-only users land on the "Select" header button after the BulkActions toolbar unmounts and must Tab through the entire session list to reach the Undo button — making it practically unreachable within the 5-second window (WCAG 2.4.3 violation).
- **WCAG 2.4.3 — Focus return on toast dismissal**: When the toast is dismissed by any path (Undo clicked, ✕ clicked, or 5-second timer expiry), focus MUST be returned to the "Select" header button (`selectButtonRef.current?.focus()`). This is the element that held focus immediately before the toast mount (it received focus when the BulkActions toolbar unmounted per the bulk-delete auto-exit path in Task 2.2.1-D).

### Interaction Flow

| State | User action | System response |
|---|---|---|
| Toast visible | Click "Undo" | `clearTimeout(timer)`. Sessions restored to list immediately. Toast dismissed. No RPCs fired. |
| Toast visible | Click "✕" dismiss | Toast dismissed. Pending deletes flushed immediately (RPCs fire now, not after 5s). **Implementation note**: the dismiss handler must call `flushPendingDeletes()` before `removeNotification()` — calling only `removeNotification()` would silently skip the RPCs. |
| Toast visible | Do nothing | After 5s, timer fires. RPCs called via `Promise.allSettled` for all pending IDs. Toast auto-dismisses. |
| Toast visible | Trigger another bulk delete | Previous pending deletes flushed. New undo window starts. New toast shows. |
| Toast visible | Navigate away / unmount | `useEffect` cleanup fires `flushPendingDeletes()`. RPCs fire on unmount. |
| Toast visible | Close browser tab | Pending deletes may be lost — documented known limitation (no `beforeunload` handler). |
| Timer expires | Some RPCs fail (partial failure) | Failed sessions reappear in list. Error shown: "N deleted, M failed — failed sessions are back in the list" (5 seconds). No undo toast. Select mode stays exited. |
| Timer expires | All RPCs fail (full failure) | All sessions reappear in list. Error shown: "All M deletes failed — sessions are back in the list" (5 seconds). No undo toast. Select mode stays exited. |

### Edge Cases

- Bulk delete of 0 sessions (shouldn't be reachable via UI but guard in code): no toast shown.
- Session was already deleted server-side between bulk-delete and undo-window expiry: `DeleteSession` RPC returns 404. Log and continue — no user-visible error (session is already gone).
- Undo clicked after component unmount: `undoFn` closure has captured `pendingDeleteRef`; if `pendingDeleteRef.current` is null (flush already happened), undo is a no-op. Guard: `if (!pendingDeleteRef.current) return;`.

---

## Surface 6: Master "Select All" Control

The master checkbox is located in the BulkActions toolbar (see Surface 4). This section documents the full select-all flow including the "select filtered vs. all" scope decision.

```
MASTER CHECKBOX STATES:

[○]  — unchecked: no sessions selected (toolbar not visible)
[—]  — indeterminate: some sessions selected (1 to N-1)
[☑]  — checked: all filtered sessions selected

INTERACTION (clicking indeterminate):

  Before: 3 of 12 sessions selected → [—]
  Click  → "Select all 12" → [☑], all 12 in selectedSessions
  User sees count "12 sessions selected"

INTERACTION (clicking checked):

  Before: 12 of 12 sessions selected → [☑]
  Click  → Clear selection → selectMode=false, toolbar hides

SELECT ALL WITH ACTIVE FILTER:

  User has filtered to "stopped" status: 4 sessions visible
  Cmd+A or clicking master checkbox:
    → Selects all 4 visible (filtered) sessions
    → Count: "4 sessions selected"
    → No "select all N on server" second-step banner needed
       (single flat virtualized list, no server pagination)
```

### Keyboard Shortcut: Cmd/Ctrl+A

- Fires `handleSelectAll()` — sets `selectedSessions` to the full set of `filteredSessions` IDs.
- Guard: does not fire when focus is inside `<input>` or `<textarea>` or `contentEditable` element.
- Enters select mode if not already active.
- Works from anywhere in the session list container (document-level listener, guarded by `selectMode` active).

---

## Surface 7: Mobile / Touch Form Factor

Touch devices have no hover event, so the hover-reveal checkbox pattern must adapt.

### Design Decision: Persistent Checkboxes in Select Mode

On touch devices, checkboxes are never hover-revealed. Instead:

- **Browse mode** (no selection active): No visible checkboxes. A "Select" button appears in the header/toolbar as the entry point.
- **Select mode active**: Checkboxes become persistently visible on all rows, identical to desktop select mode. Touch targets are sized to 44×44px minimum (WCAG 2.5.5, Apple HIG).

The "Select" button is always visible on touch devices; on desktop it is present but the hover-reveal provides an additional entry point.

```
MOBILE — BROWSE MODE:

┌──────────────────────────────────────────────────────────┐
│  Sessions                          [Select] [Filter] [+] │  <- header
├──────────────────────────────────────────────────────────┤
│  claude-code  /projects/foo    running  2h 14m      [⋯]  │
│  aider        /projects/bar    stopped  0h 45m      [⋯]  │
│  claude-code  /projects/baz    paused   1h 02m      [⋯]  │
└──────────────────────────────────────────────────────────┘
  No checkboxes visible. User taps "Select" to enter select mode.

MOBILE — SELECT MODE:

┌──────────────────────────────────────────────────────────┐
│  [✕ Cancel]  3 selected          [Pause 3] [Delete 3]   │  <- BulkActions
├──────────────────────────────────────────────────────────┤
│  [☑]  claude-code  /projects/foo    running   2h 14m    │  <- selected
│  [☐]  aider        /projects/bar    stopped   0h 45m    │
│  [☑]  claude-code  /projects/baz    paused    1h 02m    │  <- selected
│  [☐]  claude-code  /projects/qux    running   0h 30m    │
└──────────────────────────────────────────────────────────┘
  All checkboxes visible and tappable. 44px touch targets.
  BulkActions toolbar moves to TOP on mobile (closer to thumb reach on small screens).
  "Cancel" replaces the "Select" button in the header area.
```

**Touch target sizing**: The checkbox cell in row mode is 24px wide on desktop. On touch (detected via `@media (pointer: coarse)`), the checkbox cell expands to 44px and the checkbox input itself gets `width: 44px; height: 44px` via CSS to meet the 44×44px minimum (WCAG 2.5.5, Apple HIG). **Implementation note**: use `@media (pointer: coarse)` for hit-target expansion and `@media (hover: none)` for hiding hover-reveal behavior — both rules are required and serve different purposes; do not collapse them into a single selector.

**No touch-and-hold / long-press**: Long-press to enter select mode is explicitly out of scope for this iteration (per requirements). Entry is via the "Select" button only on touch.

**Shift+click on touch**: Not applicable — no Shift key on mobile keyboards. Range select is not available on touch in this iteration.

**Undo toast on mobile**: Same bottom-center positioning. Ensure toast does not overlap the mobile bottom navigation bar or system gesture area. Apply `padding-bottom: env(safe-area-inset-bottom)` to the toast container.

---

## Keyboard Shortcut Behavior Summary

| Shortcut | Precondition | Effect |
|---|---|---|
| Click checkbox | Any | Enters select mode (if first), toggles session. Sets anchor. |
| Shift+click checkbox | Anchor exists | Replaces selection with range from anchor to clicked row. |
| `X` (key) | Row has keyboard focus | Toggles selection on focused row. Enters select mode if first. |
| `Cmd+A` / `Ctrl+A` | List has focus, no input focused | Selects all filtered sessions. Enters select mode if needed. |
| `Escape` | Select mode active, no modal open | Clears selection, exits select mode. **Focus returns to the "Select" header button** (WCAG 2.4.3). |
| `Escape` | Modal is open | Closes modal only. Select mode unaffected. (`stopPropagation` in modal.) |
| Cancel / Clear Selection button | Select mode active | Exits select mode. **Focus returns to the "Select" header button** (WCAG 2.4.3). |

**Escape precedence rule** (Sarah Higley's model): Each layer calls `event.stopPropagation()` after handling Escape. The innermost open layer handles Escape first and stops propagation. The SessionList Escape handler only fires for events that bubble past any open modal.

**Focus management on select-mode exit (WCAG 2.4.3 Focus Order)**: When the BulkActions toolbar unmounts (select mode exits), any focus that was inside the toolbar is lost. To comply with WCAG 2.4.3, focus MUST be programmatically returned to the "Select" button in the session list header via `selectButtonRef.current?.focus()`. This applies to ALL exit paths:
1. Escape key → `handleClearSelection()` → `selectButtonRef.current?.focus()`
2. "Cancel" or "Clear Selection" button → `handleClearSelection()` → `selectButtonRef.current?.focus()`
3. Bulk delete completing (select mode auto-exits) → `handleClearSelection()` → `selectButtonRef.current?.focus()`

The `handleClearSelection` function owns this focus return centrally — it must not be duplicated at individual call sites.

---

## Visual Design Tokens

All tokens must be added to `web-app/src/app/globals.css` before use in components. No hardcoded hex values in component files.

| Token name | Value | Usage |
|---|---|---|
| `--session-selected-bg` | `rgba(99, 102, 241, 0.08)` | Row/card background when selected for bulk action |
| `--session-selected-border` | (not used — background tint is sufficient) | N/A |
| (existing) `--success` | green (already defined) | Active/current session left border |
| (existing) `--primary` | blue/indigo (already defined) | Filled checkbox, BulkActions count accent |
| (existing) `--error` | red (already defined) | Delete button destructive styling |
| (existing) `--error-text` | red text (already defined) | Delete button label |

**Critical rule**: `--session-selected-bg` MUST NOT be the same color as the active session border (`--success`). The two indicators must be categorically distinguishable — different hue, different indicator type (background vs. border).

---

## ARIA Annotation Map

| Element | Role | ARIA attributes |
|---|---|---|
| Session list container | (implicit list) | `aria-multiselectable="true"` when `selectMode=true` |
| Session row (row mode) | `role="row"` | `aria-selected={isSelected}`, `aria-current="true"` when active |
| Session card | `role="listitem"` | `aria-selected={isSelected}`, `aria-current="true"` when active |
| Row checkbox input | `type="checkbox"` (native) | `aria-label="Select session {name}"`, `tabIndex={selectMode ? 0 : -1}` |
| Checkbox cell wrapper | `div` | `aria-hidden={!selectMode}` (hides from AT when not in select mode) |
| Master checkbox | `type="checkbox"` (native) | `aria-label="Select all sessions"`. `el.indeterminate = true` set imperatively — do NOT use `aria-checked="mixed"` on a native checkbox. |
| Selection count span | `span` | `aria-live="polite"`, `aria-atomic="true"` |
| Undo toast container | `div` | `role="status"`, `aria-live="polite"` |
| Undo button | `button` | `aria-label="Undo the last bulk delete"` |
| Dismiss (✕) button | `button` | `aria-label="Dismiss notification"` |
| BulkActions toolbar | `div` | `role="toolbar"`, `aria-label="Bulk session actions"` |

---

## UX Acceptance Criteria

### Selection — Entry and Exit

**AC-01** User can enter select mode by clicking the checkbox on any session row (row mode) without first clicking a "Select" mode button. Checkboxes must be hover-revealed before any selection is active.

**AC-02** User can enter select mode in card view identically to row view — clicking the card's checkbox enters select mode and selects that card.

**AC-03** Pressing `Escape` while select mode is active and no modal is open clears all selected sessions and exits select mode. The BulkActions toolbar disappears. Focus returns to the "Select" button in the session list header (WCAG 2.4.3 Focus Order). Clicking "Cancel" or "Clear Selection" in the toolbar produces the same result, including the focus return.

**AC-04** Pressing `Escape` while a modal (e.g., a confirmation dialog) is open closes only the modal. Select mode and the current selection remain unchanged after the modal closes.

**AC-05** On mobile (touch), checkboxes are NOT visible in browse mode. A "Select" button in the header is the only entry point. After tapping "Select", all row checkboxes become visible and tappable.

**AC-06** On mobile, once select mode is active, all checkboxes are persistently visible (not hover-reveal). No hover event is required to reveal a checkbox.

### Selection — Checkboxes and State

**AC-07** User can select 10 sessions and delete them in 4 or fewer interactions: (1) hover and click first checkbox, (2) Shift+click the 10th session, (3) click "Delete N" in the toolbar, (4) optionally click "Undo" or let timer expire.

**AC-08** Shift+click selects a contiguous range including both the anchor endpoint (the last non-Shift-clicked row) and the Shift-clicked endpoint. The anchor is not reset by a Shift+click.

**AC-09** A second Shift+click (with the same anchor) contracts or extends the range from the anchor to the new Shift-click target. Sessions previously in the range but outside the new range are deselected.

**AC-10** Clicking any row without Shift resets the anchor to that row. Any previous Shift+click range is replaced, not unioned.

**AC-11** Shift+click across group boundaries selects all sessions (not group headers) between the two endpoints in flat `flatItems` order.

**AC-12** Pressing `Cmd+A` (macOS) or `Ctrl+A` (other platforms) while the session list has focus selects all currently filtered (visible) sessions. `Cmd+A` inside a text input does NOT trigger this behavior.

**AC-13** The master checkbox in BulkActions shows an indeterminate state (dash icon, not checked, not unchecked) when some-but-not-all filtered sessions are selected. Clicking the indeterminate master checkbox selects all filtered sessions.

**AC-14** Pressing the `X` key while a session row has keyboard focus toggles selection on that row and enters select mode if not already active. (Desktop only — no keyboard on mobile.)

### Selection — Visual Feedback

**AC-15** A selected session row has a distinct background tint (`--session-selected-bg`) applied. The tint is visually distinguishable from: (a) an unselected row, (b) the active/current session row, and (c) the hover state.

**AC-16** The active (currently-viewed) session and bulk-selected sessions use different visual indicators. The active session shows a left border in the status color. Selected-for-bulk sessions show a background tint. Neither indicator color is reused between the two states.

**AC-17** The BulkActions toolbar shows a selection count that reflects only the sessions visible under the current filter (active selection = `selectedSessions ∩ filteredSessions`). If a filter reduces 5 selected sessions to 3 visible, the toolbar shows "3 sessions selected", not "5 sessions selected".

**AC-18** The selection count in BulkActions is announced by screen readers when it changes (`aria-live="polite"`).

**AC-19** When select mode is not active, the checkbox in each session row has `tabIndex={-1}` and is not reachable by keyboard Tab navigation.

### Layout — No Layout Shift

**AC-20** Entering select mode does not cause any horizontal layout shift in the session list. The 24px checkbox column is reserved at all times (whether `selectMode` is true or false), so no grid column is added or removed when select mode is toggled.

### Undo — Bulk Delete

**AC-21** After clicking "Delete N sessions" in the BulkActions toolbar, the deleted sessions disappear from the list immediately (within one render cycle, < 16ms). No `DeleteSession` RPC has been called at the moment of disappearance.

**AC-22** The undo toast appears within 200ms of the bulk delete action being triggered. Toast text reads "Deleted N session(s)" with a visible "Undo" button.

**AC-23** Clicking "Undo" in the toast within 5 seconds restores all deleted sessions to their original positions in the list. No `DeleteSession` RPC is called at any point during this flow.

**AC-24** If the user does not click "Undo" within 5 seconds, the `DeleteSession` RPC is called for each pending session after the timer expires. The toast auto-dismisses.

**AC-25** Only one undo toast is visible at a time. If the user triggers a second bulk delete while a previous undo toast is active, the previous pending deletes are flushed immediately (RPCs fire), and a new undo toast appears for the second operation.

**AC-26** If the user closes the browser tab during the 5-second undo window, pending deletes are lost — no `DeleteSession` RPCs are fired. This is an accepted known limitation for this developer tool. No `beforeunload` handler, `navigator.sendBeacon`, or synchronous XHR is used; do not introduce these patterns. What DOES happen: navigating away within the app (without closing the tab) triggers the `useEffect` cleanup, which calls `flushPendingDeletes()` and fires the RPCs before the component unmounts. Only a hard tab close (which skips component unmount) silently drops the pending deletes.

### Context-Aware Toolbar

**AC-27** When all selected sessions are in the "paused" state, the BulkActions toolbar prominently shows "Resume All N" and does not show a "Pause" button (pausing an already-paused session is a no-op). When all are "running", "Pause All N" is shown and "Resume" is de-emphasized. In mixed states, both are shown with counts ("Resume X" / "Pause Y").

### Accessibility

**AC-28** All interactive elements in the selection UI meet WCAG 2.1 AA criteria:
- Checkboxes are reachable by keyboard Tab when `selectMode=true` (`tabIndex={0}`)
- `aria-selected` is set on each row element reflecting its selection state
- `aria-multiselectable="true"` is on the list container when `selectMode=true`
- The indeterminate master checkbox sets `el.indeterminate = true` via DOM property — `aria-checked="mixed"` is NOT set on the native checkbox element
- Touch targets on mobile are at least 44×44px
- Contrast ratio of `--session-selected-bg` against the row background meets WCAG 2.1 SC 1.4.11 (3:1 minimum for non-text contrast)
- **WCAG 2.4.3 Focus Order**: when the BulkActions toolbar unmounts on select-mode exit (via Escape, Cancel/Clear, or bulk-delete auto-exit), focus is programmatically returned to the "Select" button in the session list header via `selectButtonRef.current?.focus()`. Focus MUST NOT be silently dropped to `document.body`.

**AC-28b** (WCAG 2.4.3 — Undo toast keyboard access) Given a bulk delete has fired and the undo toast is visible, a keyboard-only user can reach the Undo button within 3 Tab presses OR the Undo button receives focus automatically on toast mount (0 Tab presses required) and can activate it before the 5-second timer expires. Specifically: the `NotificationToast` component MUST call `undoButtonRef.current?.focus()` in a `useEffect` on mount when `notificationType === "undo"`. Given the toast is dismissed by any path (Undo, ✕, or timer expiry), focus MUST return to the "Select" header button.

**AC-29** Partial or full `DeleteSession` RPC failure is surfaced to the user:
- **Partial failure** (some RPCs fail): failed sessions reappear in the list; error feedback "N deleted, M failed — failed sessions are back in the list" is shown for 5 seconds. Select mode remains exited. No undo toast is shown on the failure path.
- **Full failure** (all RPCs fail): all sessions reappear in the list; error feedback "All M deletes failed — sessions are back in the list" is shown for 5 seconds. Select mode remains exited.

---

## Interaction Flow Diagrams

### Flow 1: First Selection to Bulk Delete with Undo

```
User hovers session row
        │
        ▼
Ghost checkbox appears (CSS, no React state)
        │
User clicks checkbox
        │
        ▼
selectMode=true | session added to selectedSessions | lastAnchorId=sessionId
        │
        ▼
BulkActions toolbar slides in from bottom
Checkboxes become persistently visible on all rows
        │
User Shift+clicks another session (row N)
        │
        ▼
Range computed: anchor→N in flatItems order
selectedSessions replaced with range Set
BulkActions count updates
        │
User clicks "Delete 5" in BulkActions
        │
        ▼
5 sessions removed from displayed list (optimistic)
selectedSessions cleared, selectMode=false
BulkActions toolbar hides
Undo toast appears at bottom-center within 200ms
5-second timer starts
        │
   ┌────┴────────────────────────────┐
   │                                 │
User clicks Undo               Timer expires (5s)
   │                                 │
   ▼                                 ▼
5 sessions reappear         DeleteSession RPC ×5 (parallel)
No RPC called               Sessions permanently deleted
Toast dismissed             Toast auto-dismissed
```

### Flow 2: Escape Precedence

```
User is in select mode with 3 sessions selected
        │
User opens a bulk-action confirmation modal
        │
Modal receives focus
        │
User presses Escape
        │
        ▼
Modal keydown handler fires first
Modal closes
e.stopPropagation() called
        │
Event does NOT bubble to SessionList handler
        │
selectMode remains true
selectedSessions unchanged (3 still selected)
```

### Flow 3: Mobile Touch Entry

```
User on mobile device opens session list
        │
No checkboxes visible (hover-reveal does not apply)
"Select" button visible in header
        │
User taps "Select"
        │
        ▼
selectMode=true
All row checkboxes appear persistently (44px touch targets)
BulkActions toolbar appears at top of list
        │
User taps session checkboxes to select
        │
User taps "Delete N" in toolbar
        │
        ▼
[same pending-delete → undo toast flow as desktop]
```

---

## Summary

| Metric | Value |
|---|---|
| Surfaces designed | 7 |
| UX acceptance criteria | 30 |
| Keyboard shortcuts documented | 7 |
| Interaction flow diagrams | 3 |
| ARIA annotation entries | 11 |
| Visual design tokens | 3 new, 3 existing reused |
| Mobile / touch adaptations | 4 (persistent checkboxes, "Select" button entry, 44px touch targets, bottom nav inset) |
