# Stack Research: nav-redesign

**Date**: 2026-06-24
**Feature**: Navigation redesign — grouping, mobile completeness, settings consolidation

---

## 1. vanilla-extract Patterns in Nav Components

### DrawerNav.css.ts
- Uses `recipe()` from `@vanilla-extract/recipes` for stateful variants (open/closed drawer, active item, label visibility, badge position).
- Uses `style()` for static, one-off elements (navList, navIcon, drawerDivider, toggleButton).
- Token references: `vars.color.*`, `vars.space.*`, `vars.radii.*`, `vars.fontSize.*`, `vars.fontWeight.*`, `zIndex.raised` — all from `@/styles/theme.css` (re-exported from `theme-contract.css`).
- Media query breakpoint for hiding the entire drawer on mobile: `(max-width: 768px) { display: none }` — hardcoded, not a breakpoint token.
- WCAG 2.1 AA touch target floor enforced inline: `minHeight: "44px"` on `navItem` and `toggleButton`.

### BottomNav.css.ts
- No `recipe()` — uses plain `style()` for every rule, then composes active state by string-concatenating two class names in the TSX (`${styles.navItem} ${isActive ? styles.navItemActive : ""}`).
- More sheet slide-up animation is a CSS `transform: translateY(...)` transition toggled by adding/removing `moreSheetOpen` class.
- `zIndex` values (`zIndex.bottomNav`, `zIndex.bottomNavMoreBackdrop`, `zIndex.bottomNavMoreSheet`) all come from the `zIndex` export in `theme.css` — named slots, not magic numbers.
- Mobile-only visibility: `(min-width: 900px) { display: none }` — note different breakpoint (900px) from DrawerNav (768px).
- Dynamic `--bottom-nav-height` CSS custom property is set at runtime via `ResizeObserver` + `document.documentElement.style.setProperty`.

### Navigation.css.ts
- Older pattern: plain `style()` only, no `recipe()`. Active state is a separate exported `active` style composed via string concatenation in TSX.
- Uses hardcoded `zIndex: 50` (not a theme token) — flagged in CSS arch rules as an anti-pattern.
- This component (`Navigation.tsx`) is a legacy top-bar nav; it is NOT driven by `nav-pages.ts`. It hardcodes its own items and only covers 3 routes (Sessions, Review Queue, Backlog).

### Common pattern summary
- DrawerNav is the most modern: full `recipe()` with typed variants.
- BottomNav uses `style()` + class concatenation — easy to extend with new section headers using new `style()` exports without restructuring.
- Adding group section headers to either component only requires new `style()` (or `recipe()`) exports for a `sectionHeader` and optional `sectionDivider` style — no external library needed.

---

## 2. NavPage Interface — Current Structure and Minimal Extension for Grouping

### Current interface (`web-app/src/lib/nav-pages.ts`)

```typescript
export interface NavPage {
  href: string;
  label: string;
  shortLabel?: string;
  icon: LucideIcon;
  mobileNav?: boolean;      // false = excluded from BottomNav entirely
  headerNav?: boolean;      // false = hidden from always-visible header row
  bottomNavPrimary?: boolean; // true = in primary bar; absent = in More sheet
  featureFlag?: string;
}
```

### Problems with current approach
- `mobileNav: false` is the only mechanism for mobile exclusion — it silences items completely rather than reorganizing them.
- No group/category concept — consumers (`DrawerNav`, `BottomNav`) iterate `NAV_PAGES` as a flat list.
- The derived constants (`MOBILE_NAV_PAGES`, `BOTTOM_NAV_PRIMARY`, `BOTTOM_NAV_MORE`) are all flat `Array.filter()` results.

### Minimal sufficient extension for grouping

Add a `group` field to `NavPage`:

```typescript
export type NavGroup = "core" | "work" | "analytics" | "settings" | "system";

export interface NavPage {
  // ... existing fields ...
  group?: NavGroup;   // NEW: logical grouping for rendering section headers
}
```

Derive grouped views with a utility:

```typescript
export function groupNavPages(pages: NavPage[]): Map<NavGroup | "ungrouped", NavPage[]> { ... }
```

Consumers iterate the map to render section headers between groups. This is purely additive — no breaking changes to the existing flat exports.

**Settings consolidation**: The three fragmented items (`Settings`, `Config Files`, `Features`) can be collapsed to a single entry pointing to `routes.settings` (with the settings page handling tabs internally). Removing the two redundant entries from `NAV_PAGES` and updating the settings page to use query params or internal tabs suffices — no new routes needed.

**Mobile completeness**: Change `mobileNav: false` → `group: "system"` on the hidden items (Settings, Logs, Errors, Help, Insights, Escape Analytics, Files). The More sheet then renders the "system" group rather than silently excluding them.

---

## 3. Third-Party Navigation Libraries in package.json

No dedicated navigation component library is installed. Radix UI primitives present:

| Package | Version | Use in nav? |
|---|---|---|
| `@radix-ui/react-dialog` | ^1.1.15 | Used elsewhere (modals), not in nav |
| `@radix-ui/react-slot` | ^1.2.4 | Used elsewhere |
| `@radix-ui/react-tabs` | ^1.1.13 | Used in settings page |
| `@radix-ui/react-tooltip` | ^1.2.8 | Used elsewhere |

**There is no `@radix-ui/react-navigation-menu`, `vaul`, or any other nav-specific library.** All nav components are bespoke.

Implication: the redesign must be fully custom. No Radix `NavigationMenu` or `Accordion` primitive can be dropped in without a new dependency. The existing pattern (custom `style()`/`recipe()` + plain React state) should be continued.

---

## 4. React Patterns in DrawerNav / BottomNav

### DrawerNav.tsx
- **Context-driven**: consumes `useNavigation()` from `NavigationContext` for collapse state (`isDrawerOpen`, `toggleDrawer`).
- **No local state**: all drawer open/close state lives in `NavigationProvider` (localStorage-persisted, breakpoint-aware auto-close).
- **Flat map**: `NAV_PAGES.map(page => <li>...)` — a single pass over the flat array. To add groups, this map needs to become a two-level render: iterate groups, then items per group.
- **Badge injection by href**: three badges (ReviewQueue, Unfinished, Notifications) are injected via `if (page.href === routes.XYZ)` conditionals inside the map. This pattern is brittle but must be preserved for the existing badges; a `badgeComponent?: React.ComponentType` field on `NavPage` would be a cleaner future refactor.
- **No compound component pattern** — just a flat function component importing styles directly.

### BottomNav.tsx
- **Multiple contexts**: consumes `useOmnibar`, `useAuth`, `useNotifications`, `useFeatureFlags`, `useHandedness` — it aggregates more concerns than DrawerNav.
- **Local state for More sheet**: `useState(false)` for `moreOpen` — closed on route change and Escape key. This `moreSheet` slide-up is a sheet-style bottom drawer, not a popover.
- **Feature flag filtering inline**: `filterByFlag(pages)` is a local function wrapping `useFeatureFlags`. A group-aware refactor should preserve this filtering before grouping.
- **Hard-coded special items**: Notifications (badge), New Session button, and Account link are NOT in `NAV_PAGES` — they are rendered directly. Any redesign must account for these three special slots.
- **No compound component pattern** — single function component with `renderPrimaryItem` as a local helper.

### Key constraints for the redesign
1. DrawerNav and BottomNav are both `"use client"` components — no server component restrictions apply.
2. The `NavigationContext` only manages drawer collapse state; it does not need to change for grouping.
3. Feature flag filtering must happen before grouping; the group utility should accept pre-filtered arrays.
4. The More sheet in BottomNav is a custom slide-up sheet (not a dialog/portal) — it uses `position: fixed` relative to the viewport with `--bottom-nav-height` for positioning. Adding section headers inside it is straightforward via new `style()` exports.

---

## Summary

1. **vanilla-extract pattern**: DrawerNav uses `recipe()` (typed variants), BottomNav uses plain `style()` + class concatenation. Both reference `vars.*` and `zIndex.*` tokens from `theme-contract.css`. New section header styles fit naturally as additional `style()` exports — no new library needed.

2. **Minimal NavPage extension**: Add `group?: NavGroup` to `NavPage` and a `groupNavPages()` utility. Change `mobileNav: false` to an appropriate group membership on currently-hidden items. Consolidate the three Settings-related entries into one. All existing derived constants remain backward-compatible.

3. **No nav library installed**: Only `@radix-ui/react-dialog`, `react-tabs`, `react-tooltip`, and `react-slot` are present — no nav-specific Radix primitive. The redesign must use custom components; the existing bespoke pattern should be extended rather than replaced.
