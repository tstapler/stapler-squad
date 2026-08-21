# Features Findings: Toolbar & Action Bar Audit

**Date:** 2026-04-13
**Scope:** All toolbar, filter bar, and action bar components in the Stapler Squad web app

---

## Summary Table

| Component | Type | Overflow Behavior | Breakpoints | Min-Width Constraints | Touch Targets | Risk Level |
|-----------|------|-------------------|-------------|----------------------|---------------|------------|
| `ActionBar.tsx` | Shared flex component | flex-wrap: wrap (default) / overflow-x: auto @ ≤640px | ✓ @640px | flex-shrink: 0 on scroll variant | ✓ 44px+ | LOW |
| `Header.tsx` | Top nav layout | No overflow handling on desktop | ✓ @768px only | gap: 2rem (desktop), 0.75rem (mobile) | ✓ 44px buttons @ mobile | **CRITICAL** |
| `WorkspaceSwitcher.tsx` | Dropdown selector | Dropdown: position absolute; no scroll on parent | Limited (trigger max-width: 160px) | min-width: 220px dropdown | ✓ 44px+ | **HIGH** |
| `SessionList.tsx` | Filter bar + list | flex-wrap: wrap (desktop) / flex-direction: column @≤768px | ✓ @768px | search min-width: 250px (desktop) | ✓ 44px+ | **HIGH** |
| `HistoryFilterBar.tsx` | Filter toolbar | flex-wrap: wrap (parent) | ✓ @768px | .select min-width: 140px | ✓ 44px implicit | MEDIUM |
| `DensityToggle.tsx` | Icon button group | overflow: hidden (container) | None | Fixed 32px per button | ❌ **32px** (below 44px min) | MEDIUM |
| `ExportButton.tsx` | Dropdown menu | Dropdown: position absolute, overflow: hidden | None | min-width: 180px dropdown | ✓ 44px implicit | HIGH |
| `LiveTailToggle.tsx` | Control group | Container flex + absolute dropdown | None | min-width: 140px dropdown | ✓ 44px+ | HIGH |
| `TimeRangePicker.tsx` | Dropdown picker | Dropdown: position absolute, overflow: hidden | None | min-width: 200px dropdown | ✓ 44px+ | HIGH |
| `MultiSelect.tsx` | Logs toolbar filter | Dropdown: overflow hidden, options: max-height 200px, overflow-y auto | None | min-width: 160px dropdown | ✓ 44px+ | HIGH |
| `BulkActions.tsx` | Persistent action bar | flex-direction: column @≤768px | ✓ @768px | gap: 1rem / 0.75rem | ✓ 44px min-height | ✅ GOOD |
| `SessionLogsTab.tsx` | Logs view toolbar | flex-wrap: wrap + overflow: auto (.tableWrapper) | None on toolbar | search min-width: 200px | ✓ 44px+ | **CRITICAL** |

---

## Critical Issues

### 1. Header.tsx — No Intermediate Breakpoint
**Files:** `Header.module.css`

`.container` uses `gap: 2rem` with no `overflow-x: auto` or flex-wrap. Nav is centered with `justify-content: center`. On tablets (~800–1100px) the centered nav links compete with the `.actions` area. Gap only collapses at 768px (from 2rem to 0.75rem) — no mid-breakpoint exists.

```css
/* Header.module.css */
.container {
  max-width: 1400px;
  display: flex;
  gap: 2rem;  /* No wrap, no scroll */
}

@media (max-width: 768px) {
  .container { gap: 0.75rem; }
  /* Large jump — nothing between full desktop and hamburger */
}
```

**Fix direction:** Add `@media (768px–1024px)` breakpoint that hides text labels in `.actions` while keeping icon buttons visible.

---

### 2. SessionLogsTab.tsx Toolbar — No Responsive Stacking
**Files:** `SessionLogsTab.module.css`

Toolbar uses `flex-wrap: wrap` but no media query. `.searchInput` has `min-width: 200px` (62.5% of 320px viewport). Multiple absolute-positioned dropdown components (MultiSelect, LiveTailToggle, TimeRangePicker) follow with no viewport-edge protection.

```css
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  /* No media query for mobile stacking */
}

.searchInput {
  min-width: 200px;  /* 62.5% of 320px phone */
}
```

**Fix direction:** Add `@media (max-width: 768px)` that stacks toolbar vertically; migrate to `ActionBar` with `scroll` prop.

---

## High-Priority Issues

### 3. WorkspaceSwitcher — Dropdown Can Overflow Viewport
**Files:** `WorkspaceSwitcher.module.css`

`.trigger` has `max-width: 160px` (50% of 320px viewport). Dropdown `min-width: 220px` positioned `right: 0` — on narrow phones it will overflow the left edge.

```css
.dropdown {
  min-width: 220px;
  max-width: 320px;
  position: absolute;
  right: 0;  /* No viewport protection */
}
```

**Fix direction:** Add `max-width: min(320px, calc(100vw - 1rem))` to `.dropdown`; constrain trigger width at mid-breakpoints.

---

### 4. ActionBar Breakpoint Mismatch (640px vs 768px)
**Files:** `ActionBar.module.css`, `Header.module.css`, `SessionList.module.css`

ActionBar's scroll mode activates at `max-width: 640px`. Header and SessionList respond at `@media (max-width: 768px)`. At 650–768px, ActionBar wraps while SessionList toolbar still tries to flex-fit items — inconsistent UX.

**Fix direction:** Align ActionBar's breakpoint to 768px, or add a `breakpoint` prop.

---

### 5. Absolute-Positioned Dropdowns — No Viewport Guard
**Files:** `ExportButton.module.css`, `LiveTailToggle.module.css`, `TimeRangePicker.module.css`, `MultiSelect.module.css`

All four use `position: absolute` with either `left: 0` or `right: 0`, with `min-width` ranging from 140px to 200px. No `max-width: calc(100vw - Xpx)` or JS viewport-edge detection.

| Component | Position anchor | min-width | Overflow direction |
|-----------|----------------|-----------|-------------------|
| ExportButton | `right: 0` | 180px | Left overflow on narrow viewports |
| LiveTailToggle | `right: 0` | 140px | Left overflow on narrow viewports |
| TimeRangePicker | `left: 0` | 200px | Right overflow on narrow viewports |
| MultiSelect | `left: 0` | 160px | Right overflow on narrow viewports |

**Fix direction:** Add `max-width: min(Npx, calc(100vw - 1rem))` and `right: max(0px, calc(Npx - 100vw))` or switch to a positioned-with-flip utility.

---

### 6. SessionList Filter Bar — `display: contents` Fragility
**Files:** `SessionList.module.css`

Desktop layout uses `display: contents` on `.filterControls`, making its children participate directly in the parent flex container. This is fragile and fails predictably when the parent context changes (e.g., if ActionBar wraps it).

```css
.filterControls {
  display: contents;  /* Fragile — children float into parent flex */
}
/* Mobile override */
.filterControlsOpen {
  display: flex;
}
```

Additionally, `search min-width: 250px` on desktop has no override until 768px. At 600px tablet: 250px search + multiple 120–140px selects + gaps = overflow.

**Fix direction:** Replace `display: contents` with standard flex; remove `min-width: 250px` or add 480px–768px override.

---

## Medium-Priority Issues

### 7. DensityToggle — Touch Target Below 44px Minimum
**File:** `DensityToggle.module.css`

```css
.option {
  width: 32px;
  height: 32px;  /* WCAG 2.5.5 requires 44px minimum */
}
```

**Fix direction:** Increase to `min-width: 44px; min-height: 44px` with padding to keep visual size.

---

### 8. WorkspaceSwitcher Trigger — Hardcoded `max-width: 160px`
**File:** `WorkspaceSwitcher.module.css`

Trigger truncates at 160px at all viewport widths. On 320px phone, the trigger consumes 50% of viewport. No responsive override exists.

**Fix direction:** Add `@media (max-width: 768px) { .trigger { max-width: 120px; } }`.

---

### 9. HistoryFilterBar — 5 Selects at 140px Each
**File:** `HistoryFilterBar.module.css`

5 selects × 140px = 700px + gaps → overflows any viewport under 800px. Mobile override at 768px reduces to 120px each but the math still doesn't work at 500–768px.

**Fix direction:** Add `@media (max-width: 640px)` breakpoint with `min-width: 100px` or use ActionBar scroll mode.

---

## Components Using ActionBar (vs. Ad-hoc)

| Component | Uses ActionBar? | Notes |
|-----------|----------------|-------|
| `BulkActions.tsx` | ❌ Ad-hoc flex | Has its own responsive CSS; works well |
| `SessionList` filter bar | ❌ Ad-hoc flex | `display: contents` pattern, fragile |
| `HistoryFilterBar` | ❌ Ad-hoc flex | flex-wrap only, no scroll mode |
| `SessionLogsTab` toolbar | ❌ Ad-hoc flex | No responsive handling |
| `Header` `.actions` | ❌ Ad-hoc flex | No mid-breakpoint |

**Finding: No production toolbar currently uses the `ActionBar` component.** ActionBar exists as a shared component but is unused by the main toolbar areas.

---

## Responsive Breakpoints In Use

| Width | Components That Respond |
|-------|------------------------|
| 640px | ActionBar (scroll mode only) |
| 768px | Header, SessionList, BulkActions, HistoryFilterBar |
| No breakpoint | ExportButton, LiveTailToggle, TimeRangePicker, MultiSelect, DensityToggle, SessionLogsTab toolbar |

**Gap risk:** The 640–768px range has mismatched behavior. The 480–640px range is entirely unresponsive for most toolbars.

---

## Safe / Well-Implemented Patterns

**`BulkActions.tsx`** — Properly responsive with mobile stacking at 768px; full-width buttons on mobile. ✅

**`ActionBar.tsx` scroll variant** — Correct `-webkit-overflow-scrolling: touch`, `scrollbar-width: none`, `flex-shrink: 0` on children. ✅

**`SessionLogsTab` `.tableWrapper`** — Uses `flex: 1; overflow: auto; min-height: 0` to prevent flex content overflow. ✅
