# Architecture Findings: Responsive Nav & Action Bars

**Date:** 2026-04-13
**Scope:** CSS approach selection for responsive toolbars in React/Next.js with CSS Modules + vanilla-extract

---

## Summary

**Recommendation: Hybrid Approach A + C** — Extend `ActionBar` with a `compact` prop for 80% of toolbars, plus targeted CSS-only media query fixes for Header/WorkspaceSwitcher edge cases.

Container queries (Approach B) are **blocked** by the Safari 15.4 browser target.

---

## Browser Support: CSS Container Queries

| Browser | Minimum Version | Support | Notes |
|---------|----------------|---------|-------|
| Chrome | 106+ | ✅ Full | |
| Chrome | < 106 | ❌ No | |
| Firefox | 110+ | ✅ Full | |
| Firefox | < 110 | ❌ No | |
| Safari (macOS) | 16.0+ | ✅ Full | macOS 13+ |
| **Safari (iOS) 15.4** | **< 16.0** | **❌ No** | **PROJECT BLOCKER** |
| Safari (iOS) | 16.0+ | ✅ Full | iOS 16+ |
| Edge | 106+ | ✅ Full | Chromium-based |

**Global support: ~93.99%** — but iOS Safari 15.4 is an explicit project target per requirements.md.

---

## Approach A: Extend ActionBar with `compact` Prop

**Feasibility:** HIGH | **Browser compatibility:** 100% | **Implementation effort:** LOW

Extend the existing `ActionBar.tsx` with a `compact` boolean prop that applies a `@media (max-width: 1024px)` rule in `ActionBar.module.css`:
- Reduces `gap` by 30–50%
- Reduces padding
- Adds an optional `.compactLabel` class that hides text labels at mid-widths (opt-in per toolbar)

```tsx
// Usage
<ActionBar compact>
  <button className={styles.action}>
    <Icon /> <span className={styles.label}>New Session</span>
  </button>
</ActionBar>
```

**Pros:**
- Zero new dependencies; extends proven existing pattern
- Consistent behavior across all toolbars
- Simple API; backwards compatible
- Already follows the pattern in `Header.module.css` (hamburger at 768px)

**Cons:**
- Requires prop threading for compact mode
- Some toolbars need selective label hiding, others need full reflow — one prop may not cover all cases
- Header WorkspaceSwitcher overflow still needs surgical per-component CSS

---

## Approach B: Container Queries (CSS `@container`)

**Feasibility:** BLOCKED | **Browser compatibility:** FAIL (Safari 15.4)

Container queries let each component respond to its container's width rather than the viewport. They work seamlessly in CSS Modules and vanilla-extract (`.css.ts`) — no technical restriction.

**Why blocked:** Project targets iOS Safari 15.4+. Container query support requires Safari 16.0+. This is a 1-major-version gap but it's an explicit requirement blocker with no acceptable polyfill path.

**Revisit when:** Drop iOS Safari 15.4 support (i.e., require iOS 16+).

---

## Approach C: CSS-Only Per-Component Media Queries

**Feasibility:** HIGH | **Browser compatibility:** 100% | **Implementation effort:** MEDIUM

Add `@media (max-width: 1024px)` and `@media (max-width: 768px)` breakpoints directly to each component's `.module.css` file. No component API changes needed.

**Pros:**
- Works across all target browsers
- Flexible per-component customization
- Minimal code changes; proven approach (already in Header.module.css)

**Cons:**
- CSS duplication across multiple `.module.css` files
- No shared pattern — each toolbar looks different at mid-widths
- Doesn't scale well as new toolbars are added

---

## Recommendation: Hybrid A + C

### Primary (Approach A): Extend ActionBar for Standardized Toolbars

Add `compact` prop to `ActionBar.tsx`; migrate these to use it:
- `SessionList` filter bar
- `HistoryFilterBar`
- Logs toolbar (DensityToggle, ExportButton, LiveTailToggle, TimeRangePicker row)
- Header `.actions` row (New Session button text label hiding)

### Secondary (Approach C): Targeted CSS Fixes for Edge Cases

Header + WorkspaceSwitcher need surgical per-component CSS:
1. Add `@media (768px–1024px)` breakpoint to `Header.module.css` — compress `.actions`, hide WorkspaceSwitcher text label
2. Add `max-width` constraint to `WorkspaceSwitcher.module.css` to prevent overflow at narrow widths
3. Test intermediate widths (800px, 900px, 1024px) and add component-specific tweaks as needed

### Implementation Phases

1. **Phase 1:** Extend ActionBar + fix Header/WorkspaceSwitcher (primary complaint per requirements)
2. **Phase 2:** Migrate SessionList, HistoryFilterBar to enhanced ActionBar
3. **Phase 3:** Add component-specific `.module.css` media queries for any remaining toolbars post-testing

### Why This Works

- **Consistency:** 80% of toolbars get standardized ActionBar behavior
- **Flexibility:** Edge cases (WorkspaceSwitcher, Header) get custom CSS
- **100% browser compatibility:** No container query dependency
- **Zero new dependencies:** Extends existing code
- **Maintainability:** Shared pattern reduces duplication; custom behavior is opt-in

---

## Sources

- MDN: CSS Container Queries — https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_containment/Container_queries
- caniuse: CSS Container Queries — https://caniuse.com/css-container-queries
- Current ActionBar implementation: `web-app/src/components/ui/ActionBar.tsx` + `ActionBar.module.css`
