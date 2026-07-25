# UX Research: Bulk Selection in List/Table Views

**Date**: 2026-06-23
**Context**: Stapler Squad session list — adding bulk select to row view (virtualized list, React + TypeScript + vanilla-extract). Users are developers managing many parallel AI coding sessions (Claude Code, Aider, etc.).

---

## 1. Industry Comparison

### Gmail

**Checkbox visibility**: Hover-reveal. The sender avatar transforms into a checkbox on hover — a "dual-purpose" affordance that conserves column space. After one item is checked, all visible checkboxes become persistently visible for the duration of the selection session.

**Indeterminate state**: The master checkbox at the top-left shows a dash (indeterminate) when some-but-not-all page-visible items are selected. Clicking indeterminate selects all visible items; clicking checked deselects all.

**Select-all banner (two-step pattern)**: Checking the master checkbox selects all 50 items visible on the page. A blue banner appears inline: "All 50 conversations on this page are selected. Select all [N] conversations in Inbox." The second step expands to the server-side full set. This friction is intentional — prevents accidental mass operations. For Stapler Squad's single flat virtualized list with no pagination, the one-step version ("Select all X sessions") is sufficient.

**Shift+click**: The anchor point is the most recent non-shift click. Shift+clicking extends or contracts the range from that fixed anchor. A new plain click resets the anchor. This is the standard OS file-select model (Windows Explorer, macOS Finder list view).

**Selected row visual treatment**: Yellow/gold background tint. The toolbar swaps navigation controls for contextual bulk actions (Archive, Delete, Mark as read/unread, Move to, Labels, Snooze).

**Undo pattern**: Action fires immediately. Toast appears at bottom center: "Conversation moved to Trash — Undo." Duration: approximately 5 seconds. Non-blocking — users can continue working. Only one undo toast at a time; a new action replaces the previous toast and commits the prior action.

---

### Linear (Issue Tracker)

**Checkbox visibility**: Hover-revealed by default, keeping the list visually clean. However, Linear compensates with first-class keyboard selection: `X` selects the focused issue without needing to hover or use the mouse.

**Keyboard model**: `X` toggles selection on focused item. `Shift+X` extends range. `Shift+↑/↓` extends range one item at a time. `Cmd/Ctrl+A` selects all in the current list/board.

**Bulk action toolbar**: A floating bar appears at the **bottom** of the view after selection. Quick actions (priority, status) appear directly in the bar. Complex actions route through `Cmd/Ctrl+K` (command bar) or right-click.

**Escape**: `Esc` exits selection if no text field is focused. When a text field has focus, `Esc` clears that input first, then exits selection on the second press.

**Key insight**: Linear's pattern proves that hover-reveal checkboxes are acceptable when keyboard selection (`X`) is first-class. The visual reveal is a secondary affordance; keyboard users never need to hover.

---

### GitHub Issues

**Checkbox visibility**: Persistent — the checkbox column is always visible in the classic Issues list. No hover-reveal required. This reflects GitHub's assumption that bulk operations are a primary, not incidental, workflow on the Issues page.

**Select-all**: Master checkbox at top of the column. Checks all visible items on the current page.

**Bulk action toolbar**: Appears at top of list once items are selected. Actions available: Label, Milestone, Assignee, Mark as, Close issue, Reopen issue.

**No undo**: GitHub uses permanent/immediate actions with no undo toast. Relies on confirmation-on-close workflows instead.

**GitHub Projects (table view)**: `Cmd+Click` for non-contiguous selection. `Shift+Click` or `Shift+Arrow` for range select. Bulk cell editing via drag (spreadsheet-style) in table view.

**Key insight**: Persistent checkboxes work well in task-management contexts where the list is primarily a worklist, not an information display. GitHub Issues is closer to a todo list than an information radiator.

---

### Notion Database Tables

**Checkbox visibility**: Hover-reveal. Hovering any row reveals a row-select checkbox at the left edge. Distinct from any "Checkbox" property column in the database schema. The "Select All" checkbox appears on hover of the Name column header.

**Bulk action bar**: Docked at top of the database view (not floating). Shows editable properties inline. A "..." overflow menu reveals additional fields. Available actions: Edit properties, Delete, Duplicate, Move to.

**Scope limitation**: Checkboxes and bulk edit exist only in Table view. Board, Gallery, Calendar views have no bulk-selection UI.

**Keyboard**: `Shift+Click` and `Cmd/Ctrl+Click` for multi-select.

**Key insight**: Notion's docked top bar trades positional proximity (bar is far from where selections are being made) for stability (doesn't obscure content). Linear's bottom-floating bar is more ergonomically proximate to list content.

---

### VS Code Explorer Pane

**Checkbox visibility**: None — no checkbox UI at all. Selection is shown purely through row background highlight.

**Multi-select**: `Ctrl/Cmd+Click` toggles individual file selection (non-contiguous). `Shift+Click` selects contiguous range. Both patterns match OS file manager conventions.

**Keyboard**: Arrow keys move focus without changing selection. No `Cmd+A` equivalent for the Explorer. Space does not toggle selection — it opens the file. Enter opens the file.

**Bulk actions**: Surface through right-click context menu only (no floating toolbar). Actions: Delete, drag-and-drop, "Compare Selected" (exactly 2 files).

**Key insight**: VS Code proves that a rich bulk-select interaction model does not require checkboxes at all. However, this only works when the selection affordance is well-understood convention (file managers). In a new tool context, checkboxes reduce the learning curve significantly.

---

## 2. Hover-Reveal vs. Persistent Checkboxes

### The Tradeoff

| Dimension | Hover-Reveal | Persistent |
|---|---|---|
| Visual density | Preserves text column width; affordance is hidden at rest | Adds ~40px left column; reduces information density |
| Discoverability | Requires hover to discover — fails for new users | Immediately obvious intent |
| Touch screens | Breaks completely — no hover event exists | Works on all input types |
| Keyboard-only users | Problematic unless keyboard shortcut exists | Standard Tab/Space navigability |
| Scan-and-select speed | Slower — requires hover-before-click | Faster — can scan and check without hovering |
| Cognitive load at rest | Lower — visual noise reduced while browsing | Higher — checkboxes compete with content |

### When each approach works

**Hover-reveal is appropriate when:**
- The list is primarily an information display ("read mode") with selection as an incidental workflow
- A compensating keyboard shortcut (like Linear's `X`) exists for keyboard users
- The list is dense with text and checkbox columns would meaningfully reduce readability
- Users are expected to use the product heavily and learn the hover affordance

**Persistent is appropriate when:**
- The list is primarily a worklist ("task mode") where selection is a frequent, expected workflow
- Users include keyboard-only users without workarounds
- The product is used on touch devices
- Discoverability is a priority (power tools, enterprise apps)

### Recommended approach for Stapler Squad

**Hybrid — state-dependent visibility:**

1. **Browse mode** (no items selected): Checkbox appears as a ghost/low-opacity indicator on row hover only. Before any selection has been made, the density of the list is preserved. This is consistent with how Linear handles it.

2. **Select mode** (at least one item selected): All checkboxes become persistently visible for all rows, whether hovered or not. Users can now scan and check quickly. This avoids the frustrating "lose the checkbox" problem where moving the mouse to check another item makes the first one disappear.

3. **Keyboard shortcut as first-class path**: Add a shortcut (e.g., `X` when a row has keyboard focus) to toggle selection without requiring the hover-reveal. This makes the feature fully keyboard-accessible even while checkboxes are hover-hidden.

This pattern (hover-reveal before first selection, persistent after) is confirmed by Eleken's bulk-action UX research as the right balance for developer tool density.

---

## 3. Keyboard Model Best Practices

### Shift+Click Anchor Point

**The standard**: The anchor is the most recent **non-shift click**. It stays fixed through subsequent Shift+clicks. Shift+clicking replaces the previous range (it does not extend additively).

Concrete behavior:
- Click row 3 → anchor = row 3
- Shift+click row 7 → selects rows 3–7 (anchor stays at 3)
- Shift+click row 5 → selects rows 3–5 (range contracts; anchor still at 3)
- Click row 10 (no shift) → anchor moves to row 10; any previous range is cleared

This is the behavior of Windows Explorer (List View), macOS Finder (List/Column view), Gmail, Linear, and virtually every OS file manager. It is the expected mental model.

**Implementation note for Stapler Squad**: Store `anchorIndex` in a `useRef` (not state). Update it only on non-shift clicks. On Shift+click, compute the range as `[min(anchorIndex, currentIndex), max(anchorIndex, currentIndex)]`, replace (not union) the existing selection with this range, then call `onToggleSelect` for any items entering or leaving the range.

**Edge case — virtualized list with groups**: Shift+click range must operate on the flat `flatItems` array (the virtualizer's ordering), not per-group arrays. A range spanning two groups should include all items in between, inclusive.

### Cmd/Ctrl+A — Select All

**Scope**: Scoped to the session list container when it has focus. Must not fire when focus is in a text input.

**Guard implementation**:
```typescript
// In the keydown handler on the SessionList container:
if ((e.metaKey || e.ctrlKey) && e.key === 'a') {
  const active = document.activeElement;
  if (active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement) {
    return; // let the input handle its own Cmd+A
  }
  e.preventDefault();
  selectAll();
}
```

**Behavior**: Selects all currently filtered/visible sessions (the flat virtualized list items, not the server-side total). Enters select mode if not already in it.

**Existing keydown handler**: The `SessionList` already handles `G` for grouping. Add `Cmd/Ctrl+A` and `Escape` to the same handler.

### Escape Key

**Precedence rule** (from ARIA best practices and Sarah Higley's "Escaping 101"): Each component that handles Escape must call `event.stopPropagation()` after handling it. The innermost-open layer handles Escape first.

**Concrete precedence for Stapler Squad:**
1. Modal/dialog open → Escape closes modal, calls `stopPropagation()` → select mode unaffected
2. No modal, select mode active → Escape exits select mode, clears selection
3. No modal, no select mode → Escape is a no-op

**Implementation**: The modal's `keydown` handler calls `event.stopPropagation()`. The `SessionList`'s Escape handler runs only for events that bubble past the modal layer.

### Arrow Keys + Space (ARIA Grid Pattern)

**When required**: Only mandatory if the component uses `role="grid"` or `role="listbox"`. If rows are `role="row"` within a plain `role="list"`, Tab+Space to reach and toggle checkboxes is sufficient for WCAG 2.1 SC 2.1.1 compliance.

**Recommendation for Stapler Squad's virtualized list**: Do not implement full ARIA grid keyboard navigation for this iteration. The virtualized list makes roving-tabindex patterns complex. Instead:
- Add `role="row"` and `aria-selected` to each row
- Add `aria-multiselectable="true"` to the list container
- Ensure checkboxes are Tab-reachable within each row
- Shift+Click and Cmd+A keyboard shortcuts cover power-user range selection

Full Arrow+Space navigation can be a follow-on.

### Tab Behavior in Select Mode

Tab must behave normally — it does not move between checkboxes row-by-row. The checkbox inside each row is reached when Tab focus enters that row. Do not trap Tab within the list; focus traps are reserved for modal dialogs only.

---

## 4. Undo for Destructive Bulk Actions

### Toast vs. Confirmation Modal

**Decision heuristic:**

| Situation | Pattern |
|---|---|
| Action is reversible (undo/restore is possible) | Undo toast — no confirmation needed |
| Action is irreversible AND involves many items (10+) | Confirmation modal with count |
| Action is irreversible AND one item | Inline confirmation or modal |
| Action is recoverable but feels heavyweight | Undo toast preferred — avoids confirmation fatigue |

**NN/G explicit guidance**: An undo mechanism is preferable to a confirmation dialog. Confirmations add friction and users habituate to clicking "OK" without reading ("confirmation fatigue"). Undo respects user agency after they've seen the consequence.

**For Stapler Squad bulk delete**: Use the client-side pending-delete pattern (see below) to achieve undo without a restore RPC. If that is descoped, fall back to a confirmation modal with count: "Delete 5 sessions permanently? This cannot be undone." Use a red destructive button and a visible cancel path.

### The Gmail Undo Pattern

- Action fires **immediately** — items disappear from the list at once
- Toast appears at **bottom center** of viewport: "Deleted 5 conversations — Undo"
- Duration: ~5 seconds
- Non-blocking — the user can continue selecting, scrolling, etc.
- Only **one toast at a time** — a second action commits the prior deletion and shows a new toast
- Toast includes a dismiss X on some versions

### Visual State During the Undo Window

Three patterns used in the wild:

**A. Immediate removal** (Gmail, Notion): Items disappear immediately. Clean, fast. Relies entirely on the undo toast as recovery path. Risk: user misses toast, data is gone. Mitigated by making the toast prominent and high-contrast.

**B. Greyed-out / dimmed**: Items become semi-transparent with a strikethrough label "Deleting…". Communicates pending state. Downside: visual clutter, ambiguous state ("are they deleted or not?"). Avoid.

**C. Slide-out animation** (Linear and some task apps): Items animate out over 300–500ms. Clean middle ground — communicates "something is happening" without leaving ambiguous zombie rows. Undo during the animation cancels it in-place.

**Recommendation for Stapler Squad**: Use **immediate removal (Pattern A)** with client-side pending-delete. Sessions vanish from the list immediately but no RPC is fired yet. On undo click, restore sessions to local state instantly. On timer expiry, fire the batch deletes. This matches user expectations from Gmail and other familiar apps.

### Client-Side Pending-Delete Pattern (No Restore RPC Required)

This pattern achieves full undo UX with zero backend changes:

1. User clicks "Delete N sessions"
2. Optimistically remove sessions from the displayed list (remove from React state)
3. Store deleted session objects in a `pendingDeletion` ref with a 5-second timer ID
4. Show undo toast: "Deleted 5 sessions — Undo"
5. If user clicks Undo → call `clearTimeout(timerId)`, restore sessions from ref to list state, dismiss toast
6. If timer fires → call `DeleteSession` RPC for each session in the pending set in parallel; clear the ref

**Caveats**:
- If the browser tab closes during the undo window, the deletes are lost. Acceptable for a developer session management tool — sessions will be recreated as needed.
- If the user navigates away or the component unmounts, flush the pending deletes immediately (fire all RPCs on cleanup).
- Stacking: if user bulk-deletes again while a previous undo is live, flush the previous pending deletes immediately (fire their RPCs) before starting the new pending-delete window.

### Toast Positioning

**Standard**: Bottom-left or bottom-center for toasts with action buttons. Top-right for passive notification toasts (background events, completed tasks).

**For Stapler Squad specifically**: The app has a fixed left sidebar. Bottom-left will overlap the navigation. **Bottom-center** is the correct choice. Avoid top-center (associated with error banners) and top-right (too far from where destructive actions are taken in the list).

**Duration**: 5 seconds minimum for a toast with an action button. Accessibility guidance: toasts with actions should stay visible long enough for users with cognitive or motor disabilities to reach and click them (5–7 seconds is the established range).

**Stacking behavior**: Only one undo toast at a time. A second bulk action commits the previous pending deletion and shows a new toast.

---

## 5. Selection Feedback

### Selection Count Placement

**Industry patterns**:
- Gmail: count shown in the toolbar replacing navigation controls ("50 selected")
- Linear: count shown in the floating bottom action bar
- GitHub Issues: count shown in the bulk-action dropdown trigger at the top of the list
- PatternFly / Hashicorp Helios design systems: count in the contextual action bar, not as a standalone label

**Recommendation for Stapler Squad**: Display count in the floating bottom bulk-action bar. The `BulkActions` component that already exists should show the count as part of its label (e.g., "5 sessions selected" or a badge). Do not add a second count display in the header — splitting count information across two non-adjacent locations creates cognitive overhead.

**Header checkbox count exception**: The master "Select All" checkbox in the header may include a tooltip or `aria-label` describing the total count, but should not duplicate the count visually if the bottom bar is visible.

### Indeterminate Checkbox State

**WCAG/ARIA requirements**:
- Use the native HTML `<input type="checkbox">` indeterminate property (set via JavaScript: `el.indeterminate = true`). This exposes as "mixed" to assistive technology through platform accessibility APIs automatically.
- Do NOT add `aria-checked="mixed"` to a native checkbox — the HTML state overrides ARIA, causing double-announcement bugs.
- The visual indicator (dash/minus icon) must be distinguishable from both checked and unchecked at a minimum 3:1 contrast ratio (WCAG 2.1 SC 1.4.11, Non-text Contrast).

**Behavior**: Clicking an indeterminate master checkbox should select all (not deselect). Second click deselects all. This matches Gmail and the HTML spec behavior.

**Implementation**:
```typescript
// In a useEffect after render, set the indeterminate DOM property:
const checkboxRef = useRef<HTMLInputElement>(null);
useEffect(() => {
  if (checkboxRef.current) {
    checkboxRef.current.indeterminate = someSelected && !allSelected;
  }
}, [someSelected, allSelected]);
```

### Selected Row Visual Treatment

**Industry approaches**:
- Gmail: Yellow/gold background tint on the row
- Linear: Blue left border accent + light blue background fill
- GitHub Issues: Light blue/grey background tint on the checkbox column
- Notion table: Subtle blue row background + persistent blue checkbox fill

**Recommendation**: Row background tint (using a new CSS custom property `--session-selected-bg` in `globals.css` or a vanilla-extract token) plus the checked checkbox visual. Do not rely on border accent alone — it fails in high-contrast mode and is insufficiently visible in dense lists.

**Avoid blue**: If the active/current session already uses blue highlighting (common for "you are here" indicators), use a different hue for selection — e.g., a neutral grey or a light teal. Blue-on-blue creates ambiguity between "selected for bulk action" and "currently active."

### Selected-in-Select-Mode vs. Current (Active) Session

This is the critical distinction for Stapler Squad: one session is "active" (the one the user is currently working in, shown in the terminal panel) and zero-to-many sessions may be "selected" for bulk action.

**Use different colors AND different indicators:**

| State | Visual Treatment | ARIA |
|---|---|---|
| Active/current session | Left border in a status-related color (e.g., bright green for running) + bold session name | `aria-current="true"` |
| Selected for bulk action | Light neutral background tint + filled checkbox | `aria-selected="true"` |
| Active AND selected | Both: border + background tint + filled checkbox | Both `aria-current` and `aria-selected` |
| Hover (no other state) | Subtle background change + ghost checkbox reveals | — |

**Key rule**: The active session indicator and the bulk-selection indicator must never share the same color or the same indicator type. Users need to distinguish "I'm working in this session" from "I've marked this for deletion."

### "Select All in Filter" vs. "Select All Globally"

For Stapler Squad's flat virtualized list without server-side pagination: single-step is sufficient. "Select all X sessions" where X is the count of the currently filtered list. No second-step "select all N on server" banner needed unless pagination is added.

---

## 6. Jobs-to-Be-Done Analysis

### Primary Functional Job

**"When I finish a wave of AI-assisted work, I need to rapidly clear stopped/completed sessions from the list so I can regain a clear mental model of what is still active."**

The core frustration is not efficiency in the mechanical sense — it is that a list full of stopped sessions creates ongoing cognitive load: every time the user glances at it, they must mentally filter out noise. Bulk delete closes the "needs action" loop on many items at once. This is qualitatively different from single-item delete repeated N times, even if the total time were the same.

### Secondary Functional Jobs

- "When I need to step away from my computer, I need to pause all running sessions at once so they don't consume resources or make unexpected changes."
- "When I return to a project, I need to resume a set of related paused sessions simultaneously so I can context-switch efficiently."
- "When I want to focus on one area, I need to pause all sessions in unrelated workspaces without navigating to each one."

### Emotional Jobs

**Control** (primary): A developer managing 30+ sessions in various states experiences cognitive overload. The bulk-select capability provides the sensation of mastery — "I can impose order on this list." Without it, the list becomes something users dread looking at, not a useful tool.

**Completion**: Deleting 20 stopped sessions at once triggers a stronger "done" signal than repeating single deletions. The satisfying cleared state ("0 stopped sessions") reinforces the tool as productive.

**Safety**: The undo toast after bulk delete eliminates the fear of accidental mass destruction. Without undo, users will hesitate on bulk delete — or avoid it entirely and tolerate the messy list. Undo is not just a UX nicety; it is a prerequisite for users to actually trust and use the bulk delete feature.

### Social Job

Primarily internal self-image ("I am organized, I work efficiently"). There is no meaningful social/peer visibility for session list state — this is a solo productivity tool, not a team dashboard. Social job is minor.

### Struggling Moments (Specific, Observable)

1. **End-of-work-session cleanup**: 15 stopped sessions in the list after a long coding session. User must click each one, confirm delete, wait for RPC. Total: ~3-5 minutes of pure busywork. Many users give up and leave the list cluttered indefinitely.

2. **Before a meeting/break**: User wants to pause all running sessions quickly. Currently must pause each one — potentially losing track of which were running vs. paused. Risk of context loss on return.

3. **Cleanup anxiety spiral**: The accumulated stopped sessions make the list hard to parse (active sessions hidden among noise). Cleaning up is painful. Users avoid the list view and therefore miss useful information. The lack of bulk operations degrades the entire tool's utility.

4. **Accidental single delete**: No undo currently. A user who fat-fingers a session delete has no recovery path. This makes users more cautious about all deletes, including intentional ones. Undo on bulk delete has a secondary benefit: it also signals that the tool is generally safe to use.

### What Would Make Bulk Select Feel "Magical" vs. "Adequate"

**Adequate**: Checkboxes appear, user clicks sessions, toolbar shows count, clicks Delete, sessions are gone.

**Magical — status-aware bulk actions**: When the user selects 5 paused sessions, the bulk action toolbar prominently shows "Resume All" and de-emphasizes "Pause" (which would be a no-op). When 3 running + 2 paused sessions are selected, the toolbar shows both with context ("Resume 2, Pause 3"). The toolbar is smart about mixed states.

**Magical — smart "Clear completed" action**: A "Delete all stopped" or "Delete all completed" quick-action button above the list that doesn't require individual selection — analogous to "Archive all in this category" in Gmail. One gesture clears the noise. With undo, this becomes a genuinely powerful cleanup tool.

**Magical — keyboard-driven flow**: Shift+click selects the exact expected range. Pressing `D` (or `Delete` key) while sessions are selected opens a contextual command or immediately triggers delete with undo toast — no mousing to the toolbar required.

---

## Summary Recommendations for Stapler Squad

### Checkbox Visibility
Use hybrid hover-reveal + state-persistent: ghost checkbox on row hover before any selection; all checkboxes become permanently visible once select mode is entered. Add keyboard shortcut `X` to toggle selection on the focused row without requiring hover.

### Shift+Click
Implement sticky anchor model: `anchorIndex` stored in a `useRef`, updated only on non-shift clicks. Shift+click replaces the range from anchor to current index. Operate on the flat virtualized list order, not per-group arrays.

### Keyboard Shortcuts
- `X`: Toggle selection on focused row (enter select mode if first selection)
- `Shift+Click`: Range select from anchor
- `Cmd/Ctrl+A`: Select all filtered sessions (guard against input focus)
- `Escape`: Exit select mode + clear selection (lower precedence than modal close via `stopPropagation`)

### Undo
Implement client-side pending-delete: remove sessions from display immediately, hold DELETE RPCs for 5 seconds, show undo toast at bottom-center. On undo, restore to list. On timeout, fire RPCs. On component unmount, flush immediately. Only one pending-delete window at a time.

### Visual Feedback
- Selection count in the floating bottom `BulkActions` bar (not duplicated in header)
- Selected rows: light neutral background tint + filled checkbox
- Active session: distinct color/indicator (`aria-current`, left border) — must NOT share color with bulk-selection highlight
- Indeterminate master checkbox: set via `el.indeterminate = true` (no `aria-checked="mixed"` on native checkbox)

### Contextual Toolbar Intelligence
Make the toolbar state-aware: when all selected sessions are paused, emphasize "Resume All"; when all are running, emphasize "Pause All"; in mixed state, show both. This transforms the toolbar from a generic set of buttons into a genuinely helpful tool.

---

## Sources

- Nielsen Norman Group: "Bulk Actions: 3 Design Guidelines" (video), "Confirmation Dialogs Can Prevent User Errors"
- W3C WAI-ARIA Authoring Practices Guide: Listbox Pattern, Keyboard Interface, Grid Pattern
- Sarah Higley: "Escaping 101" — Escape key precedence model
- Eleken: "Bulk action UX: 8 design guidelines with examples for SaaS"
- PatternFly: Bulk Selection pattern documentation
- Material Design: Snackbar guidelines
- Hashicorp Helios Design System: Table selection patterns
- Atlassian AUI: aria-current vs aria-selected distinction
- WCAG 2.1 SC 1.4.11 (Non-text Contrast), SC 2.1.1 (Keyboard)
- Direct observation: Gmail, Linear, GitHub Issues, Notion, VS Code Explorer
