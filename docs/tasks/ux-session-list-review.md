# UX Review: Session List — Stapler Squad

**Source**: UX audit of session list views, pane management, and mobile experience.
**Date**: 2026-06-13
**Scope**: `web-app/src/components/sessions/`, `web-app/src/components/pane/`

---

## Critical Issues

### Task C-1: Show persistent navigation back to session list on mobile

**File**: `web-app/src/components/pane/MobilePaneTabStrip.tsx` line 24

**Problem**: `MobilePaneTabStrip` renders `null` when `leaves.length <= 1`. First-time users who open a session detail pane have no visible path back to the session list — the system back button is the only escape hatch. Violates Nielsen H3 (User control and freedom) and H1 (Visibility of system status).

**Acceptance Criteria**:
- [ ] On mobile, when a session detail pane is open (even as the only pane), a persistent back affordance is visible
- [ ] Acceptable implementations: (a) show the tab strip whenever a detail pane is open regardless of leaf count, or (b) add a persistent back arrow in `PaneHeader` on mobile when the current pane is session detail
- [ ] The back affordance navigates the user to the session list without requiring a system back gesture
- [ ] No regression on desktop — desktop pane behavior is unchanged

---

### Task C-2: Give "Needs Approval" sessions a distinct status dot color in row mode

**File**: `web-app/src/components/sessions/SessionRow.css.ts` lines 55–68

**Problem**: Both `needs-approval` and `paused` states map to `vars.color.statusDot.paused` in row mode. Users cannot distinguish sessions needing immediate input from merely suspended ones. Violates Nielsen H5 (Error prevention) and POUR Perceivable.

**Acceptance Criteria**:
- [ ] `needs-approval` status dot in row mode uses a distinct color from `paused` — amber tone recommended, matching the pulsing glow animation used in card mode (`statusNeedsApproval`)
- [ ] Optionally add a CSS pulse animation to the `needs-approval` dot in row mode for consistency with card mode
- [ ] Color is sourced from `vars.color.*` theme tokens (no hardcoded hex values per CSS architecture rules)
- [ ] Both row mode and card mode convey "needs attention" using visually consistent treatment

---

## High Severity

### Task H-1: Show active filter summary on mobile when session list is filtered

**File**: `web-app/src/components/sessions/SessionList.tsx` lines 622–636

**Problem**: Filter controls are collapsed by default on mobile. A persisted filter can silently reduce the visible session list with no visible explanation to the user.

**Acceptance Criteria**:
- [ ] When any filter is active and the filter panel is collapsed on mobile, an inline summary is shown below the search input
- [ ] Summary lists the active filter dimensions (e.g., "Filtered by: Status, Tag")
- [ ] Summary includes a one-tap "Clear filters" affordance
- [ ] Summary is not shown when no filters are active or on desktop where filter panel is always visible

---

### Task H-2: Simplify grouping strategy selector and add no-effect hint

**File**: `web-app/src/components/sessions/SessionList.tsx` lines 718–731

**Problem**: Plain `<select>` with 10 strategies, no descriptions, no relevance feedback, no visual preview. Cognitive overload for new users.

**Acceptance Criteria**:
- [ ] Surface 3–4 most-used strategies directly in the main control bar (recommended: None, Category, Tag, Status)
- [ ] Remaining strategies (Workflow, Session Type, Path, Branch, Program) are accessible via a "More..." disclosure or secondary selector
- [ ] When the selected strategy produces only one group, show a hint: "All sessions share the same [strategy name] — grouping has no effect"
- [ ] Existing behavior is preserved; only the presentation layer changes

---

### Task H-3: Add hover tooltip for Alt+Click split-pane affordance on session cards

**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 260–271

**Problem**: `e.altKey && onOpenInNewPane` triggers split-pane opening. No tooltip, no hover hint, no visual affordance. Feature is entirely undiscoverable.

**Acceptance Criteria**:
- [ ] A tooltip appears on hover over a session card: "Alt+Click to open side by side"
- [ ] Tooltip contains a `<kbd>Alt</kbd>` chip to visually distinguish the keyboard modifier
- [ ] Tooltip does not appear on touch devices (where Alt+Click is not applicable)
- [ ] Tooltip does not interfere with existing card click or drag interactions

---

### Task H-4: Make row action buttons visible on touch devices

**File**: `web-app/src/components/sessions/SessionRow.css.ts` lines 118–135

**Problem**: The `actions` style uses `opacity: 0`, revealing buttons only on hover. Touch devices lose the hover state immediately on touch, making action buttons permanently invisible.

**Acceptance Criteria**:
- [ ] Action buttons in row mode are always visible on touch devices
- [ ] Implementation uses `@media (hover: none) { opacity: 1 }` in the `.css.ts` styles (not inline styles)
- [ ] Desktop hover-reveal behavior is preserved — buttons remain hidden until hover on non-touch devices
- [ ] No layout shift occurs when buttons become permanently visible on touch

---

### Task H-5: Improve "Select" button label and affordance for bulk actions

**File**: `web-app/src/components/sessions/SessionList.tsx` lines 601–607

**Problem**: Plain "Select" text button provides no context about what selection enables. No icon, no tooltip, no hint about bulk actions.

**Acceptance Criteria**:
- [ ] Button label is "Select…" (with ellipsis to indicate a mode change)
- [ ] Button has `title="Select multiple sessions for bulk actions"`
- [ ] Button includes a checkbox icon alongside the label
- [ ] When selection mode is active, bulk action affordances (delete, tag, pause selected) are visible

---

## Medium Severity

### Task M-1: Make "Edit Tags" button always visible on touch devices in card mode

**File**: `web-app/src/components/sessions/SessionCard.css.ts` (editTagsButton style)

**Problem**: `editTagsButton` uses `opacity: 0`, revealing only on hover. Inaccessible on touch devices.

**Acceptance Criteria**:
- [ ] `@media (hover: none) { opacity: 1 }` added to `editTagsButton` style
- [ ] Desktop hover-reveal behavior is preserved
- [ ] Change is made in `SessionCard.css.ts` using vanilla-extract (no inline styles)

---

### Task M-2: Add explicit rename button and remove covert title-click rename trigger

**File**: `web-app/src/components/sessions/SessionCard.tsx` and `SessionRow.tsx`

**Problem**: Clicking the `<h3>` title silently enters inline rename mode. No visible affordance, no `role="button"` on the element. Violates WCAG 4.1.2 (Name, Role, Value).

**Acceptance Criteria**:
- [ ] A dedicated pencil icon button is placed beside the session title to trigger rename mode
- [ ] The rename button has `aria-label="Rename session"` and appropriate `role="button"`
- [ ] Clicking the title text directly no longer enters rename mode (or if retained, the element has `role="button"` and `tabIndex={0}` with keyboard support)
- [ ] Rename mode UX (input field, confirm/cancel) is unchanged

---

### Task M-3: Standardize paused session visual treatment across card and row modes

**File**: `web-app/src/components/sessions/SessionCard.css.ts`, `SessionRow.css.ts`

**Problem**: Card mode uses `opacity: 0.75` on the whole card for paused sessions. Row mode uses a left border accent. Inconsistent treatment across view modes confuses users who switch between them.

**Acceptance Criteria**:
- [ ] Paused session visual treatment is consistent between card and row view modes
- [ ] Preferred approach: use left border accent treatment in both modes (opacity dimming can fail WCAG AA contrast)
- [ ] If opacity dimming is kept, verify it meets WCAG AA contrast ratios for all text inside the dimmed card
- [ ] The chosen treatment is documented in a comment in both `.css.ts` files referencing this task

---

### Task M-4: Show "X of Y" session count when filters are active

**File**: `web-app/src/components/sessions/SessionList.tsx` (header count display)

**Problem**: Header shows `Sessions (3)` with no indication that filters are active and that 44 other sessions exist.

**Acceptance Criteria**:
- [ ] When `filteredSessions.length !== sessions.length`, the header shows "Sessions (3 of 47)"
- [ ] When no filters are active, the header shows "Sessions (47)" (existing behavior)
- [ ] The total count excludes no sessions — it reflects the true unfiltered count

---

### Task M-5: Add swipe-to-dismiss and drag indicator to mobile bottom sheet

**File**: `web-app/src/components/pane/PaneTilingContainer.tsx` lines 271–309

**Problem**: Mobile bottom-sheet picker has no swipe-down dismiss gesture and no drag indicator pill. Unconventional for mobile sheet patterns.

**Acceptance Criteria**:
- [ ] A 36×4px rounded drag indicator pill is rendered at the top of the bottom sheet
- [ ] Swipe-down gesture dismisses the sheet using the same `touchstart`/`touchmove` pattern as `useTerminalGestures`
- [ ] Dismiss threshold is consistent with existing gesture patterns in the codebase
- [ ] Sheet can still be dismissed by tapping outside or using an explicit close button (existing fallback preserved)

---

### Task M-6: Hide or constrain column picker on mobile in row mode

**File**: `web-app/src/components/sessions/SessionList.tsx` (ColumnPicker render condition)

**Problem**: Column picker is shown whenever `viewMode === "row"` with no mobile suppression. Most columns are unusable on narrow viewports.

**Acceptance Criteria**:
- [ ] On narrow viewports (mobile), the column picker is either hidden or limited to mobile-relevant columns
- [ ] If hidden: a sensible default column set is applied automatically for mobile row mode
- [ ] If limited: only columns that render usably at mobile width are offered
- [ ] Desktop column picker behavior is unchanged

---

## Low Severity

### Task L-1: Add visible label to "+" new session button on desktop

**File**: `web-app/src/components/sessions/SessionList.tsx` (new session button)

**Problem**: Button uses a "+" symbol only. `aria-label` exists but there is no visible text label. Ambiguous on mobile and for new users on desktop.

**Acceptance Criteria**:
- [ ] On desktop viewports, button shows "New session" text alongside the icon
- [ ] On mobile viewports, icon-only display is acceptable, with `aria-label="New session"` present
- [ ] Breakpoint behavior uses a CSS media query in the `.css.ts` file (not inline styles or JS viewport checks)

---

### Task L-2: Extract inline styles from project rename form to vanilla-extract

**File**: `web-app/src/components/sessions/SessionList.tsx` lines 852–875

**Problem**: Rename form uses `style={{ ... }}` with hardcoded `var(--css-token)` strings. Violates CSS architecture rules (ADR-009) — `var()` strings must not appear in `.tsx` files; use `vars.*` in `.css.ts` instead.

**Acceptance Criteria**:
- [ ] All inline `style={{ ... }}` props removed from the rename form elements
- [ ] Equivalent styles extracted to `SessionList.css.ts` using vanilla-extract `style()` or `recipe()`
- [ ] No raw `var(--token)` strings remain in the `.tsx` file; all token references use `vars.*` from the theme contract
- [ ] Visual appearance of the rename form is unchanged

---

### Task L-3: Add collapsible toggle to group headers for large inactive groups

**File**: `web-app/src/components/sessions/SessionList.tsx` (group header rendering, virtualizer `flatItems`)

**Problem**: Groups cannot be collapsed. Large "Stopped" / "Hibernated" groups clutter the session list and push active sessions out of view.

**Acceptance Criteria**:
- [ ] Group headers include a collapse/expand toggle (chevron icon)
- [ ] Collapsed groups hide their session rows; only the group header remains visible
- [ ] Collapsed state is persisted in component state (not required to survive page reload)
- [ ] The `flatItems` virtualizer model correctly omits collapsed group rows from the render list
- [ ] Expand/collapse is keyboard accessible

---

### Task L-4: Use "Tap" vs "Click" instruction based on input device

**File**: `web-app/src/components/pane/PaneSplitRenderer.tsx` line 273

**Problem**: Hardcoded "Click a session to open it here" instruction is incorrect on touch devices where the interaction is a tap.

**Acceptance Criteria**:
- [ ] On touch devices, instruction reads "Tap a session to open it here"
- [ ] On pointer devices, instruction reads "Click a session to open it here"
- [ ] Implementation uses a CSS `@media (hover: none)` rule or a lightweight viewport/pointer hook — not a JS user-agent sniff
- [ ] Instruction text is not hardcoded as an inline string if it can be derived from a shared constant

---

## Feature Plan: Workspace / Named Pane Sets

This section captures the findings and two-phase implementation plan from the workspace concept assessment. No tasks are created here until the approach is validated.

### Current State

The following capabilities already exist and can be composed:

- Project grouping API: `listProjects`, `createProject`, `assignSessionsToProject` in `server/services/`
- Serializable pane tree state in `web-app/src/components/pane/`
- `RESET_LAYOUT` action in the pane reducer
- `SPLIT_AND_ASSIGN_SESSION` action chain for opening multiple sessions as panes

### What Is Missing

- Named layout serialization (save/restore pane tree state by name)
- Workspace switcher UI (tab bar above `PaneTilingContainer`)
- Multiple named layout storage (localStorage for Phase 1; server-side for multi-device)

### Phase 1 Task: Project-aware pane sets (low effort)

**File**: `web-app/src/components/sessions/SessionList.tsx`, pane reducer

**Description**: When a user selects a Project group, offer "Open all N sessions as panes." Chains existing `SPLIT_AND_ASSIGN_SESSION` actions. No new persistence model required.

**Acceptance Criteria**:
- [ ] Project group headers include an "Open as panes" button (visible when group has 2–6 sessions)
- [ ] Clicking "Open as panes" dispatches `SPLIT_AND_ASSIGN_SESSION` for each session in the group
- [ ] Layout is created using the existing tiling logic; no new state shape is introduced
- [ ] Button is absent or disabled when the group has 1 or more than 6 sessions (to avoid unusable micro-panes)

### Phase 2 Task: Named layout save and restore (medium effort)

**Files**: `web-app/src/components/pane/PaneHeader.tsx`, `PaneTilingContainer.tsx`, pane reducer, `localStorage`

**Description**: "Save as workspace..." saves the current pane tree to localStorage under a user-defined name. A workspace switcher in `PaneHeader` of the session-list pane restores a named layout. Covers 90% of the use case without server persistence.

**Acceptance Criteria**:
- [ ] "Save as workspace..." button in `PaneHeader` opens a name input prompt
- [ ] Named layout (serialized pane tree) is saved to `localStorage` under a namespaced key
- [ ] Workspace switcher in `PaneHeader` lists saved workspaces and restores pane tree on selection
- [ ] Restoring a workspace dispatches a layout-replace action to the pane reducer
- [ ] Saved workspaces survive page reload (localStorage persistence)
- [ ] No server-side changes required in Phase 2; server persistence is deferred to a potential Phase 3

**Note**: Full server-side workspaces are only warranted for multi-device sync scenarios and are out of scope for Phases 1 and 2.
