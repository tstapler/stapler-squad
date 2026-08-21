# Nav Redesign — Architecture Research

## Files Read

- `web-app/src/lib/nav-pages.ts` — single source of truth
- `web-app/src/components/layout/DrawerNav.tsx` — desktop sidebar
- `web-app/src/components/layout/BottomNav.tsx` — mobile bottom bar
- `web-app/src/components/layout/Header.tsx` — legacy top bar (wraps BottomNav)
- `web-app/src/components/ui/Navigation.tsx` — minimal legacy nav (3 items only, hardcoded)
- `web-app/src/lib/contexts/NavigationContext.tsx` — drawer open/close state only
- `web-app/src/components/layout/__tests__/BottomNav.test.tsx` — test surface to protect

---

## Current State

`nav-pages.ts` exports a flat `NAV_PAGES: NavPage[]` array of 16 items, plus four pre-filtered derived arrays:

| Derived export | How computed |
|---|---|
| `MOBILE_NAV_PAGES` | `mobileNav !== false` |
| `HEADER_NAV_PAGES` | `headerNav !== false` |
| `BOTTOM_NAV_PRIMARY` | `bottomNavPrimary && mobileNav !== false && href !== notifications` |
| `BOTTOM_NAV_MORE` | `mobileNav !== false && !bottomNavPrimary` |

Each component consumes the array that matches its context:

- **DrawerNav** iterates `NAV_PAGES` directly (all 16 items, flat list)
- **BottomNav** uses `BOTTOM_NAV_PRIMARY` (4 items) and `BOTTOM_NAV_MORE` (sheet), then applies `featureFlag` filtering at render time via `useFeatureFlags()`
- **Header** uses `NAV_PAGES` directly, applies `featureFlag` filtering inline, uses `headerNav === false` to visually demote items to hamburger-only
- **Navigation** (`/ui/Navigation.tsx`) is fully independent — hardcodes 3 items, does not use `nav-pages.ts` at all

---

## Files That Import from nav-pages.ts

Only **three** files import from `nav-pages.ts`:

1. `web-app/src/components/layout/DrawerNav.tsx` — imports `NAV_PAGES`
2. `web-app/src/components/layout/BottomNav.tsx` — imports `BOTTOM_NAV_PRIMARY`, `BOTTOM_NAV_MORE`, `NavPage` (type)
3. `web-app/src/components/layout/Header.tsx` — imports `NAV_PAGES`

`Navigation.tsx` (`/ui/`) does **not** import from `nav-pages.ts`. It has its own local array.

---

## NavigationContext Analysis

`NavigationContext` only manages drawer open/close state and its localStorage persistence. It has no awareness of nav items, grouping, or filtering. **Grouping does not need to be context-aware.** The context is stable and does not need changes regardless of which option is chosen.

---

## Architectural Options

### Option A: `group` field on NavPage (recommended)

Add a `group` field to the `NavPage` interface:

```ts
export type NavGroup = "primary" | "automation" | "insights" | "settings" | "system";

export interface NavPage {
  // ... existing fields ...
  group?: NavGroup;  // undefined = ungrouped / treat as primary
}
```

Components that want grouped rendering call a shared helper:

```ts
export function groupNavPages(pages: NavPage[]): Map<NavGroup, NavPage[]> {
  const map = new Map<NavGroup, NavPage[]>();
  for (const page of pages) {
    const g = page.group ?? "primary";
    if (!map.has(g)) map.set(g, []);
    map.get(g)!.push(page);
  }
  return map;
}
```

**Pros:**
- Zero change to the existing derived arrays (`BOTTOM_NAV_PRIMARY`, `MOBILE_NAV_PAGES`, etc.) — they still work by filter predicate
- `featureFlag` filtering composes with grouping (filter first, then group)
- DrawerNav can opt-in to grouped rendering; BottomNav can ignore `group` entirely since its primary/more split is driven by `bottomNavPrimary`
- Minimal change surface — only `nav-pages.ts` interface + data, plus opt-in rendering in DrawerNav and BottomNav More sheet
- Backwards compatible — existing tests don't break

**Cons:**
- Group label strings (display names) have to live somewhere — either a separate map or a parallel `NAV_GROUP_LABELS: Record<NavGroup, string>` export

### Option B: `NAV_GROUPS` nested structure

Replace the flat array with:

```ts
export interface NavGroupDef {
  id: NavGroup;
  label: string;
  items: NavPage[];
}
export const NAV_GROUPS: NavGroupDef[] = [ ... ];
// Derived flat array for components that need it:
export const NAV_PAGES = NAV_GROUPS.flatMap((g) => g.items);
```

**Pros:**
- Group label strings are co-located with items
- Intent is explicit — no inference needed

**Cons:**
- Breaks all three existing import sites: `DrawerNav`, `BottomNav`, `Header` all iterate `NAV_PAGES` directly; they must each be updated
- The derived filter arrays (`BOTTOM_NAV_PRIMARY`, `MOBILE_NAV_PAGES`) must be regenerated from `NAV_GROUPS.flatMap()` — easy to do but adds refactor surface
- BottomNav's `featureFlag` filtering at render time (`filterByFlag`) would need to work on the nested structure, or stay as-is on a flat derived array
- BottomNav More sheet groups would need its own "slice" of `NAV_GROUPS` — the primary/more split is independent of the group split, creating a two-dimensional slicing problem

### Option C: Separate group registry (not recommended)

Keep `NAV_PAGES` flat and add:

```ts
export const NAV_GROUP_REGISTRY: Record<NavGroup, string[]> = {
  primary:    [routes.home, routes.backlog, routes.unfinished, routes.reviewQueue, routes.notifications],
  automation: [routes.workflows, routes.rules],
  insights:   [routes.insights, routes.escapeAnalytics],
  settings:   [routes.settings, routes.settingsFeatures, routes.files],
  system:     [routes.logs, routes.errors, routes.help],
};
```

**Pros:** Zero change to `NavPage` interface or existing data.

**Cons:**
- Two sources of truth for nav items — an href listed in `NAV_PAGES` but missing from the registry silently omits from all groups
- Groups must be computed at call sites by cross-referencing hrefs — verbose and error-prone
- Hardcoded href strings in the registry diverge from `routes.*` if routes change
- No type safety between registry keys and `NavPage` hrefs

---

## Recommendation: Option A

Option A is the minimum viable change that supports grouping with the least breakage:

1. Add `group?: NavGroup` to `NavPage` interface
2. Add group assignments to the 16 entries in `NAV_PAGES`
3. Add `NAV_GROUP_LABELS: Record<NavGroup, string>` for display names
4. Add `groupNavPages()` utility function (exported from `nav-pages.ts`)
5. Update `DrawerNav` to render group headers between sections
6. Optionally update BottomNav More sheet to render group headers (low priority)

The existing derived filter arrays (`BOTTOM_NAV_PRIMARY`, `BOTTOM_NAV_MORE`, etc.) remain unchanged. `featureFlag` filtering in `BottomNav` and `Header` continues to work as-is.

---

## Proposed Group Structure

Based on the current 16 items:

| Group | Items |
|---|---|
| `primary` | Sessions, Backlog, Unfinished, Review Queue, Notifications |
| `automation` | Workflows, Rules |
| `insights` | Insights, History, Escape Analytics |
| `settings` | Settings, Config Files, Features, Files |
| `system` | Logs, Errors, Help |

---

## Badge / Group Header Interaction

Badges (ReviewQueueNavBadge, UnfinishedNavBadge, NotificationsNavBadge) are **item-level** in all three existing components — they are rendered via `page.href === routes.X` guards inside the item render loop. This pattern must be preserved.

**Recommendation: keep badges item-level.** Group headers should not show aggregate badges. Reasons:

1. The badge components make their own async data fetches (counts come from contexts); a group-level badge would require either duplicating that logic or introducing new aggregate selectors
2. Collapsed DrawerNav already hides labels — a group-level badge on a collapsed section header is ambiguous UX
3. The BottomNav primary bar does not render group headers at all, so the item-level badge pattern is the only design that works across all three nav surfaces

If a group-level "dot" indicator is ever needed (e.g., to show a group contains items with activity), the cleanest approach is a derived boolean computed from the same badge contexts, passed as a prop to a `NavGroupHeader` component.

---

## Minimal Change Surface Summary

| File | Change required? | What changes |
|---|---|---|
| `web-app/src/lib/nav-pages.ts` | Yes (core) | Add `NavGroup` type, `group?` field on `NavPage`, group assignments on all 16 items, `NAV_GROUP_LABELS`, `groupNavPages()` helper |
| `web-app/src/components/layout/DrawerNav.tsx` | Yes | Replace flat `NAV_PAGES.map()` with `groupNavPages()` iteration; add group header rendering |
| `web-app/src/components/layout/DrawerNav.css.ts` | Yes | Add styles for group header labels |
| `web-app/src/components/layout/BottomNav.tsx` | Optional | More sheet can optionally render group headers; primary bar unchanged |
| `web-app/src/components/layout/Header.tsx` | Optional | Hamburger menu can optionally render group headers; header nav row unchanged |
| `web-app/src/components/ui/Navigation.tsx` | No | Fully independent; does not use `nav-pages.ts` |
| `web-app/src/lib/contexts/NavigationContext.tsx` | No | Context only manages drawer open/close |
| Test files | Minimal | `BottomNav.test.tsx` tests primary items by label — no group headers in primary bar, so no changes needed unless More sheet gets group headers |

---

## Key Constraints

1. `featureFlag` filtering must happen **after** grouping is computed (or the `groupNavPages()` helper must accept a pre-filtered array) — BottomNav currently filters at render time with `useFeatureFlags()`, which is correct and should remain
2. `DrawerNav` uses `NAV_PAGES` (all items, no mobile-only filter) — when adding groups, it should iterate `NAV_PAGES` through `groupNavPages()` rather than iterating `MOBILE_NAV_PAGES`, to preserve the existing "all items visible on desktop" behavior
3. The DrawerNav collapsed state hides labels — group header labels must also hide when collapsed (same `visible: isDrawerOpen` variant used on `navLabel`)
