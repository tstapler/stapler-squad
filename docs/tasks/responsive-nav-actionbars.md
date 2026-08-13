# Implementation Plan: Responsive Nav & Action Bars

Status: Ready for Implementation
Created: 2026-04-13
Requirements: `project_plans/responsive-nav-actionbars/requirements.md`
Research: `project_plans/responsive-nav-actionbars/research/`
ADR: `project_plans/responsive-nav-actionbars/decisions/ADR-001-hybrid-responsive-approach.md`

---

## Overview

Fix all toolbar, action bar, and navigation overflow/clipping issues in the Stapler Squad web UI across viewport widths from 320px to 2560px. The approach is a hybrid of extending ActionBar with a `compact` prop (for consistent toolbar responsiveness) and targeted CSS media query fixes (for Header and WorkspaceSwitcher edge cases). See ADR-001.

## Architecture Decision

**Hybrid A+C** per ADR-001:
- **A**: Extend `ActionBar` with `compact` prop; apply to SessionList filters, HistoryFilterBar, logs toolbar.
- **C**: Targeted CSS-only breakpoint fixes for Header.module.css (1024px intermediate) and WorkspaceSwitcher.module.css (dropdown viewport guard).

Container queries (Approach B) rejected due to iOS Safari 15.4 lacking support.

## Aligned Breakpoints

All components will use this consistent breakpoint ladder:

| Token | Width | Behavior |
|-------|-------|----------|
| `--breakpoint-sm` | 640px | ActionBar scroll mode activates |
| `--breakpoint-md` | 768px | Mobile layout: hamburger nav, stacked filters, 44px touch targets |
| `--breakpoint-lg` | 1024px | Compact mode: hidden labels, reduced gaps, compressed actions |

These tokens already exist in `globals.css` (lines 64-68).

---

## Story 1: Fix Header Intermediate Breakpoint + WorkspaceSwitcher Overflow

**Goal**: Eliminate the 768-1024px dead zone where header actions compete with nav links, and prevent WorkspaceSwitcher dropdown from overflowing the viewport on any device.

### Task 1.1: Add 1024px Intermediate Breakpoint to Header

**Size**: Small (2h)
**Files**:
- `web-app/src/components/layout/Header.module.css`

**Changes**:

Add a `@media (max-width: 1024px)` block between the existing desktop styles and the 768px mobile block:

```css
/* Intermediate breakpoint: compress header for tablets/narrow windows */
@media (max-width: 1024px) {
  .container {
    gap: 1rem; /* down from 2rem — prevents actions from competing with nav */
  }

  .navLink {
    padding: 0.5rem 0.5rem; /* narrower horizontal padding */
    font-size: 0.8125rem;
  }

  .newSessionLabel {
    display: none; /* hide "New Session" text, keep + icon */
  }

  .newSessionButton {
    padding: 0.5rem 0.625rem;
  }

  .actions {
    gap: 0.5rem; /* down from 0.75rem */
  }

  .subtitle {
    display: none; /* hide "Session Manager" earlier */
  }
}
```

Add explicit overflow control to the header to prevent silent clipping:

```css
.header {
  /* existing styles... */
  overflow: hidden; /* prevent body overflow-x:hidden from masking header overflow */
}
```

Exception: the mobile nav dropdown is `position: absolute` and needs to escape `overflow: hidden`. Since the header already has `isolation: isolate`, change the mobile `.nav` from `position: absolute` to use a portal pattern OR move the overflow guard to `.container` instead:

```css
.container {
  /* existing styles... */
  overflow: hidden; /* guard on container, not header — allows nav dropdown to escape */
}
```

**Acceptance Criteria**:
- At 900px width: all header elements visible, no overlap between nav and actions.
- At 1024px: "New Session" shows only the + icon; "Session Manager" subtitle hidden.
- At 768px: existing hamburger behavior unchanged.
- At 1200px+: full desktop layout, no regressions.
- Resize from 320px to 2560px produces no layout jumps.

### Task 1.2: Fix WorkspaceSwitcher Dropdown Viewport Overflow

**Size**: Small (2h)
**Files**:
- `web-app/src/components/layout/WorkspaceSwitcher.module.css`

**Changes**:

Constrain the dropdown to the viewport:

```css
.dropdown {
  /* existing styles... */
  max-width: min(320px, calc(100vw - 1rem)); /* prevent overflow on phones */
}
```

Constrain the trigger button for narrow widths:

```css
@media (max-width: 768px) {
  .trigger {
    max-width: 120px; /* down from 160px — saves space for icon buttons */
  }
}
```

Ensure merge button is always visible on touch devices (hover-reveal is inaccessible):

```css
@media (max-width: 768px) {
  .mergeButton {
    opacity: 1; /* always visible on touch; no hover */
  }
}
```

**Acceptance Criteria**:
- At 320px: dropdown does not extend beyond viewport right edge.
- At 375px (iPhone SE): trigger button does not crowd notification/debug/help buttons.
- Dropdown scrolls internally if workspace list is long.
- Merge button visible without hover on mobile.

### Task 1.3: Smooth Header Height Transition and Add Overflow Guard

**Size**: Small (1h)
**Files**:
- `web-app/src/app/globals.css`

**Changes**:

The current setup has a binary jump from `--header-height: 4rem` (desktop) to `3.5rem` (mobile at 768px). This causes an 8px content shift. Fix by making the transition gradual:

```css
/* Replace the existing mobile header height override */
@media (max-width: 1024px) {
  :root {
    --header-height: 3.5rem; /* transition earlier at 1024px, not 768px */
  }
}
```

This aligns header height reduction with the new 1024px breakpoint, so the height change and the nav compression happen at the same point instead of at different widths.

**Acceptance Criteria**:
- Resizing across 1024px: single visual change (compact mode + height reduction happen together).
- No visible content "jump" at 768px anymore.
- Below 768px: header height remains 3.5rem (no further change).

### Task 1.1/1.2/1.3 Dependency

All three tasks are independent and can run in parallel. They touch different CSS files with no overlapping selectors.

```
Task 1.1 (Header.module.css)     ─┐
Task 1.2 (WorkspaceSwitcher.module.css) ─┼─ all parallel
Task 1.3 (globals.css)           ─┘
```

---

## Story 2: Apply ActionBar Consistently to SessionList + HistoryFilterBar

**Goal**: Replace ad-hoc flex rows in SessionList and HistoryFilterBar with the shared ActionBar component, adding a `compact` prop to ActionBar for the 1024px breakpoint.

### Task 2.1: Add `compact` Prop to ActionBar

**Size**: Small (2h)
**Files**:
- `web-app/src/components/ui/ActionBar.tsx`
- `web-app/src/components/ui/ActionBar.module.css`

**Changes to ActionBar.tsx**:

Add `compact` to the props interface and wire it to a CSS class:

```tsx
interface ActionBarProps {
  children: React.ReactNode;
  gap?: "sm" | "md" | "lg";
  justify?: "start" | "end" | "between" | "center";
  scroll?: boolean;
  compact?: boolean; // NEW: reduces gap and enables scroll at 1024px
  className?: string;
}

// In the component:
const classes = [
  styles.actionBar,
  gapClass[gap],
  justifyClass[justify],
  scroll ? styles.scroll : undefined,
  compact ? styles.compact : undefined,
  className,
]
  .filter(Boolean)
  .join(" ");
```

**Changes to ActionBar.module.css**:

Add a `.compact` variant that activates at the 1024px breakpoint:

```css
/* Compact variant: tighter gap and scroll at tablet widths */
@media (max-width: 1024px) {
  .compact {
    gap: 0.25rem;
  }
}

@media (max-width: 768px) {
  .compact {
    flex-wrap: nowrap;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: none;
  }

  .compact::-webkit-scrollbar {
    display: none;
  }

  .compact > * {
    flex-shrink: 0;
  }
}
```

Note: The existing `.scroll` class activates at 640px. The `.compact` class activates scroll at 768px (earlier) and reduces gap at 1024px. They can coexist; a toolbar can use both `scroll` and `compact` if it wants all three breakpoints.

**Acceptance Criteria**:
- `<ActionBar compact>` at 1024px: gap visibly tighter.
- `<ActionBar compact>` at 768px: horizontal scroll, no wrapping, no shrinking.
- `<ActionBar scroll>` behavior at 640px: unchanged (no regression).
- `<ActionBar>` with neither prop: unchanged (no regression).

### Task 2.2: Wrap SessionList Filter Bar in ActionBar

**Size**: Medium (3h)
**Files**:
- `web-app/src/components/sessions/SessionList.tsx`
- `web-app/src/components/sessions/SessionList.module.css`

**Changes to SessionList.tsx**:

Import ActionBar and wrap the filter controls:

```tsx
import { ActionBar } from "@/components/ui/ActionBar";
```

Replace the `<div className={styles.filterControls}>` wrapper (lines 362-465) with ActionBar:

```tsx
{/* Desktop: ActionBar wraps all filter controls */}
<ActionBar
  scroll
  compact
  gap="sm"
  className={`${styles.filterControls} ${filtersOpen ? styles.filterControlsOpen : ""}`}
>
  {/* ...existing select elements and checkbox... */}
</ActionBar>
```

The search input + filter toggle button in `.filterTopRow` remain outside ActionBar (they have special mobile layout behavior).

**Changes to SessionList.module.css**:

Remove `display: contents` from `.filterControls` on desktop. ActionBar provides its own flex layout, so the `display: contents` trick is no longer needed:

```css
.filterControls {
  /* Remove: display: contents; */
  /* ActionBar handles layout now */
}
```

Keep the mobile override that hides/shows on toggle:

```css
@media (max-width: 768px) {
  .filterControls {
    display: none; /* hidden by default on mobile */
  }

  .filterControlsOpen {
    display: flex; /* shown when filter toggle is open */
  }
}
```

This requires overriding ActionBar's flex on mobile to stack vertically. Add to SessionList.module.css:

```css
@media (max-width: 768px) {
  .filterControlsOpen {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
  }
}
```

**Acceptance Criteria**:
- Desktop (1200px+): filters in a wrapping flex row, same as before.
- Tablet (800-1024px): filters compress with smaller gaps via ActionBar `compact`.
- Mobile (768px): filter toggle button shows/hides filter panel; filters stack vertically.
- Phone (375px): filters scroll horizontally if panel is open and items don't fit.
- Search input still spans full width and is always visible.
- Mobile filter toggle still works (shows active filter dot indicator).

### Task 2.3: Wrap HistoryFilterBar in ActionBar

**Size**: Small (2h)
**Files**:
- `web-app/src/components/history/HistoryFilterBar.tsx`
- `web-app/src/components/history/HistoryFilterBar.module.css`

**Changes to HistoryFilterBar.tsx**:

Import and wrap the `.filters` div:

```tsx
import { ActionBar } from "@/components/ui/ActionBar";

// Replace: <div className={styles.filters}>
// With:
<ActionBar scroll compact gap="sm" className={styles.filters}>
```

**Changes to HistoryFilterBar.module.css**:

Remove the flex layout from `.filters` since ActionBar provides it:

```css
.filters {
  /* Remove: display: flex; gap: 10px; flex-wrap: wrap; align-items: center; */
  /* ActionBar handles layout */
}
```

Keep the mobile override:

```css
@media (max-width: 768px) {
  .filters {
    gap: 8px;
  }

  .select {
    min-width: 120px;
    font-size: 12px;
    min-height: 44px; /* touch target */
  }

  .sortOrderButton {
    min-width: 44px;
    min-height: 44px; /* touch target */
  }
}
```

**Acceptance Criteria**:
- Desktop: filters in a wrapping row (no visual change).
- Tablet (800-1024px): tighter gaps, everything still reachable.
- Mobile (768px): filters scroll horizontally.
- All selects and buttons meet 44px touch target on mobile.
- Search mode toggle still functional.

### Task 2.1/2.2/2.3 Dependencies

Task 2.1 must complete first (ActionBar compact prop). Tasks 2.2 and 2.3 can then run in parallel.

```
Task 2.1 (ActionBar.tsx/css)
  ├── Task 2.2 (SessionList) ─ parallel
  └── Task 2.3 (HistoryFilterBar) ─ parallel
```

---

## Story 3: Fix Absolute-Positioned Dropdown Viewport Guards in Logs Toolbar

**Goal**: Prevent dropdowns in ExportButton, TimeRangePicker, MultiSelect, and LiveTailToggle from rendering off-screen on narrow viewports, and wrap the logs page toolbars in ActionBar.

### Task 3.1: Add Viewport Guards to Dropdown Positioning

**Size**: Small (2h)
**Files**:
- `web-app/src/components/logs/ExportButton.module.css`
- `web-app/src/components/logs/TimeRangePicker.module.css`
- `web-app/src/components/logs/MultiSelect.module.css`
- `web-app/src/components/logs/LiveTailToggle.module.css`

**Changes**:

All four components have `.dropdown` with `position: absolute` and either `left: 0` or `right: 0`. On phones, these can extend beyond the viewport. Add viewport guards:

**ExportButton.module.css** (dropdown uses `right: 0`):
```css
@media (max-width: 768px) {
  .dropdown {
    right: auto;
    left: 50%;
    transform: translateX(-50%);
    max-width: calc(100vw - 2rem);
  }
}
```

**TimeRangePicker.module.css** (dropdown uses `left: 0`, `min-width: 200px`):
```css
@media (max-width: 768px) {
  .dropdown {
    left: auto;
    right: 0;
    max-width: calc(100vw - 2rem);
    min-width: min(200px, calc(100vw - 2rem));
  }
}
```

**MultiSelect.module.css** (dropdown uses `left: 0`, `min-width: 160px`):
```css
@media (max-width: 768px) {
  .dropdown {
    left: auto;
    right: 0;
    max-width: calc(100vw - 2rem);
    min-width: min(160px, calc(100vw - 2rem));
  }
}
```

**LiveTailToggle.module.css** (dropdown uses `right: 0`, `min-width: 140px`):
```css
@media (max-width: 768px) {
  .dropdown {
    max-width: calc(100vw - 2rem);
    min-width: min(140px, calc(100vw - 2rem));
  }

  .pauseButton,
  .settingsButton {
    width: 44px;
    height: 44px;
  }
}
```

**Acceptance Criteria**:
- At 375px: open each dropdown and verify it does not extend past either viewport edge.
- Dropdown content is scrollable if it exceeds available height.
- All interactive elements in dropdowns meet 44px touch target.
- Click-outside-to-close still functions correctly.

### Task 3.2: Wrap Logs Page Toolbars in ActionBar

**Size**: Medium (3h)
**Files**:
- `web-app/src/app/logs/page.tsx`
- `web-app/src/app/logs/page.module.css`

**Changes to page.tsx**:

Import ActionBar and wrap both toolbar areas:

```tsx
import { ActionBar } from "@/components/ui/ActionBar";
```

Wrap `.headerActions` (line 328):
```tsx
<ActionBar scroll compact gap="md" className={styles.headerActions}>
  <LiveTailToggle ... />
  <TimeRangePicker ... />
  <span className={styles.timezone}>...</span>
  <button className={styles.refreshButton}>...</button>
  <ExportButton ... />
</ActionBar>
```

Wrap `.filters` (line 357):
```tsx
<ActionBar scroll compact gap="md" className={styles.filters}>
  <div className={styles.filterGroup}>...</div>
  <MultiSelect ... />
  <div className={styles.filterGroup}>...</div>
  <div className={styles.filterGroup}>...</div>
</ActionBar>
```

**Changes to page.module.css**:

Remove the flex layout from `.headerActions` and `.filters` since ActionBar provides it:

```css
.headerActions {
  /* Remove: display: flex; align-items: center; gap: 1rem; */
  /* ActionBar handles layout */
}

.filters {
  /* Remove: display: flex; gap: 1rem; flex-wrap: wrap; */
  /* Keep visual styling: */
  margin-bottom: 1.5rem;
  padding: 1rem;
  background-color: #1a1a1a;
  border-radius: 6px;
  border: 1px solid #333;
}
```

Add responsive override for stacking header on mobile:

```css
@media (max-width: 768px) {
  .header {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.75rem;
  }

  .header h1 {
    font-size: 1.25rem;
  }
}
```

**Acceptance Criteria**:
- Desktop (1200px+): logs toolbar looks identical to current.
- Tablet (800-1024px): toolbar items compress with smaller gaps.
- Mobile (768px): header title stacks above action bar; action bar scrolls horizontally.
- Filter bar scrolls horizontally on mobile instead of overflowing.
- All existing keyboard shortcuts still work.

### Task 3.1/3.2 Dependencies

Tasks 3.1 and 3.2 are independent. Task 3.2 depends on Task 2.1 (ActionBar compact prop).

```
Task 2.1 (ActionBar.tsx/css) ──── Task 3.2 (logs page.tsx/css)
                                                              parallel with
Task 3.1 (4 dropdown CSS files) ─────────────── (independent)
```

---

## Full Dependency Graph

```
Story 1 (all parallel, no deps):
  Task 1.1 (Header.module.css)
  Task 1.2 (WorkspaceSwitcher.module.css)
  Task 1.3 (globals.css)

Story 2 (sequential then parallel):
  Task 2.1 (ActionBar.tsx + .module.css)
    ├── Task 2.2 (SessionList.tsx + .module.css)
    └── Task 2.3 (HistoryFilterBar.tsx + .module.css)

Story 3 (partially parallel):
  Task 3.1 (4 dropdown .module.css files) ── independent
  Task 2.1 → Task 3.2 (logs page.tsx + page.module.css)
```

**Critical path**: Task 2.1 -> Task 3.2 (or Task 2.2/2.3).
**Maximum parallelism**: Tasks 1.1, 1.2, 1.3, 2.1, 3.1 can all start immediately. After 2.1 completes, 2.2, 2.3, and 3.2 can run in parallel.

---

## Files Changed Per Task

| Task | Files Modified | Files Created |
|------|---------------|---------------|
| 1.1 | `web-app/src/components/layout/Header.module.css` | none |
| 1.2 | `web-app/src/components/layout/WorkspaceSwitcher.module.css` | none |
| 1.3 | `web-app/src/app/globals.css` | none |
| 2.1 | `web-app/src/components/ui/ActionBar.tsx`, `web-app/src/components/ui/ActionBar.module.css` | none |
| 2.2 | `web-app/src/components/sessions/SessionList.tsx`, `web-app/src/components/sessions/SessionList.module.css` | none |
| 2.3 | `web-app/src/components/history/HistoryFilterBar.tsx`, `web-app/src/components/history/HistoryFilterBar.module.css` | none |
| 3.1 | `web-app/src/components/logs/ExportButton.module.css`, `web-app/src/components/logs/TimeRangePicker.module.css`, `web-app/src/components/logs/MultiSelect.module.css`, `web-app/src/components/logs/LiveTailToggle.module.css` | none |
| 3.2 | `web-app/src/app/logs/page.tsx`, `web-app/src/app/logs/page.module.css` | none |

**Total**: 14 files modified, 0 files created.

---

## Verification Strategy

Each task should be verified by:

1. **Visual resize test**: Drag browser width from 320px to 2560px at each breakpoint (640, 768, 1024, 1280). No overflow, no clipping, no layout jumps.
2. **Touch target audit**: At 375px, verify all interactive elements are at least 44x44px using browser DevTools element inspector.
3. **Dropdown bounds check**: Open every dropdown at 375px and verify it stays within viewport bounds.
4. **Mobile navigation test**: At 375px, verify hamburger menu opens/closes, mobile filter toggle works, and all controls are reachable.
5. **Desktop regression check**: At 1440px, verify no visual changes to any toolbar or navigation element.
6. **Dark mode check**: Verify all changes work correctly in both light and dark mode.
7. **Keyboard navigation**: Tab through all interactive elements and verify focus visibility and correct behavior.

**Automated**: `make quick-check` to verify build + tests + lint pass after each task.

---

## Known Issues

### KI-1: Header Overflow Hidden vs Mobile Nav Dropdown [SEVERITY: Medium]

**Description**: Adding `overflow: hidden` to `.header` or `.container` in Header.module.css to prevent silent overflow clipping will also clip the mobile nav dropdown, which uses `position: absolute; top: 100%` to extend below the header.

**Mitigation**:
- Apply `overflow: hidden` to `.container` (not `.header`) since `.header` already has `isolation: isolate` for the absolute-positioned nav.
- The mobile nav is a child of `.container` but positioned absolutely relative to `.container` (which has `position: relative` at 768px). Verify the nav dropdown still renders correctly by testing at 768px after adding the overflow guard.
- If clipping occurs, fall back to `overflow: clip` (CSS Overflow Level 4) which clips without creating a scroll container, or use `overflow-x: hidden` only.

**Files Affected**: `web-app/src/components/layout/Header.module.css`

**Prevention**: Test hamburger menu open/close immediately after adding the overflow rule.

### KI-2: Header Height Binary Jump Causes Content Shift [SEVERITY: Medium]

**Description**: `--header-height` changes from `4rem` to `3.5rem` at a breakpoint. If the content below uses `margin-top: var(--header-height)` or `top: var(--header-height)`, this causes an 8px visual jump as the user resizes across the breakpoint.

**Mitigation**:
- Move the height change to 1024px to align with the compact breakpoint (Task 1.3), so all visual changes happen simultaneously instead of at different widths.
- This makes the jump a deliberate, single-point transition rather than an unexpected intermediate state.

**Files Affected**: `web-app/src/app/globals.css`

**Prevention**: Test resize across 1024px boundary specifically and verify content below header does not shift independently.

### KI-3: `display: contents` Removal in SessionList Breaks Mobile Filter Toggle [SEVERITY: High]

**Description**: SessionList's `.filterTopRow` and `.filterControls` use `display: contents` on desktop so their children participate in the parent `.filters` flex container. Wrapping filter controls in ActionBar replaces `display: contents` with ActionBar's own flex layout. The mobile filter toggle (which hides/shows `.filterControls` via `display: none`/`display: flex`) may stop working if ActionBar's flex conflicts with the visibility toggle.

**Mitigation**:
- Keep the `.filterControls` class on the ActionBar wrapper so the mobile CSS `display: none` / `.filterControlsOpen display: flex` overrides still apply.
- ActionBar's own `display: flex` will be overridden by the `.filterControls { display: none }` mobile rule because the latter is in a `@media` block with higher specificity when both classes are on the same element.
- Test the mobile filter toggle immediately after wrapping in ActionBar.

**Files Affected**: `web-app/src/components/sessions/SessionList.tsx`, `web-app/src/components/sessions/SessionList.module.css`

**Prevention**: Write a manual test sequence: at 375px, tap "Filters" toggle, verify controls appear, tap again, verify they hide.

### KI-4: Inconsistent Breakpoints Between ActionBar `scroll` and `compact` [SEVERITY: Low]

**Description**: ActionBar's existing `scroll` prop fires at 640px. The new `compact` prop fires scroll at 768px. A toolbar with both props would see: scroll at 768px (compact) which then changes to the existing scroll at 640px. Since both apply `flex-wrap: nowrap; overflow-x: auto`, the actual visible behavior would be the same, but the CSS specificity order matters.

**Mitigation**:
- In ActionBar.module.css, ensure `.compact` media queries come after `.scroll` media queries so that when both are present, the wider breakpoint (768px from compact) takes precedence at larger widths and the 640px scroll block is redundant but harmless.
- Alternatively, if a toolbar passes `compact`, it does not need `scroll` (compact subsumes scroll at a wider breakpoint). Document this in ActionBar's JSDoc.

**Files Affected**: `web-app/src/components/ui/ActionBar.module.css`

**Prevention**: Use `compact` without `scroll` for toolbars that need the 768px scroll. Use `scroll` alone for toolbars that only need 640px scroll.

### KI-5: WorkspaceSwitcher Dropdown `right: 0` Overflow on Phones [SEVERITY: Medium]

**Description**: WorkspaceSwitcher dropdown has `right: 0; min-width: 220px`. At 320px viewport, this puts the dropdown's left edge at `320 - 220 = 100px` from left, but the trigger button may be positioned further right, causing the dropdown to extend beyond the left edge of the viewport.

**Mitigation**:
- Replace fixed `min-width: 220px` with `min-width: min(220px, calc(100vw - 1rem))` so the dropdown width is capped to viewport width minus margin.
- Add `max-width: min(320px, calc(100vw - 1rem))` to the `.dropdown` rule.
- Test at 320px by opening the workspace switcher.

**Files Affected**: `web-app/src/components/layout/WorkspaceSwitcher.module.css`

**Prevention**: Test with Chrome DevTools responsive mode at 320px, 375px, and 414px.

### KI-6: Absolute-Positioned Log Dropdowns Overflow Off-Screen [SEVERITY: Medium]

**Description**: ExportButton, TimeRangePicker, MultiSelect, and LiveTailToggle all use `position: absolute` dropdowns with either `left: 0` or `right: 0`. On phones (375px), dropdowns positioned with `left: 0` extend past the right viewport edge if their `min-width` exceeds available space; dropdowns with `right: 0` may extend past the left edge.

**Mitigation**:
- Add `@media (max-width: 768px)` blocks to each dropdown CSS with `max-width: calc(100vw - 2rem)` and flip alignment where needed (Task 3.1).
- Use `min-width: min(Npx, calc(100vw - 2rem))` pattern for constrained minimum widths.

**Files Affected**: `web-app/src/components/logs/ExportButton.module.css`, `TimeRangePicker.module.css`, `MultiSelect.module.css`, `LiveTailToggle.module.css`

**Prevention**: Open each dropdown at 375px in Chrome DevTools and visually verify bounds.

### KI-7: Body `overflow-x: hidden` Masks Real Overflow Bugs [SEVERITY: Low]

**Description**: `globals.css` sets `html, body { overflow-x: hidden }`. This silently clips any horizontal overflow rather than showing a scrollbar. During development, this makes it hard to detect when a component overflows — the content is simply cut off without visible indication.

**Mitigation**:
- During development/testing of these changes, temporarily remove `overflow-x: hidden` from body to reveal any hidden overflow.
- After all tasks are complete and verified, the `overflow-x: hidden` can remain as a safety net for production.
- This is not something to fix in this feature — it is a pre-existing condition. Document it as a known development workflow caveat.

**Files Affected**: `web-app/src/app/globals.css` (observation only, no change)

**Prevention**: Use browser DevTools "Show layout shift regions" and "Highlight all paint" to detect overflow during testing.
