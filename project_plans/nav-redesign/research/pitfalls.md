# Nav Redesign — Pitfalls & Risks

Research date: 2026-06-24
Stack: Next.js App Router, React, TypeScript, vanilla-extract

---

## 1. Consumer Map — Files That Import nav-pages Exports

| Export | Consumers |
|---|---|
| `NAV_PAGES` | `DrawerNav.tsx`, `Header.tsx` |
| `BOTTOM_NAV_PRIMARY` | `BottomNav.tsx` |
| `BOTTOM_NAV_MORE` | `BottomNav.tsx` |
| `MOBILE_NAV_PAGES` | `nav-pages.ts` (derived, no external consumer currently) |
| `HEADER_NAV_PAGES` | `nav-pages.ts` (derived, no external consumer currently) |
| `NavPage` (type) | `BottomNav.tsx` |

**Key observation:** `MOBILE_NAV_PAGES` and `HEADER_NAV_PAGES` are currently computed in `nav-pages.ts` but are not imported anywhere outside the file itself. If a redesign adds grouped exports and removes these derived arrays, nothing will break — but they are dead exports today that may cause confusion.

---

## 2. Test Breakage Risk

### 2a. BottomNav.test.tsx — hardcoded PRIMARY_ITEMS constant

`BottomNav.test.tsx` (line 77) hardcodes:
```typescript
const PRIMARY_ITEMS = [
  { href: "/", label: "Sessions" },
  { href: "/unfinished", label: "Unfinished" },
  { href: "/review-queue", label: "Review" },
] as const;
```
This list is **missing Backlog** (which is feature-flagged) and relies on the current `shortLabel` values ("Review" not "Review Queue"). Any change to:
- which items are `bottomNavPrimary: true`
- `shortLabel` values
- the order of primary items

...will break the assertion at line 104 (`remaining.size === 0`). The test iterates `PRIMARY_ITEMS` checking `getByText(item.label)`, so adding a group label text could also create false positives.

**The test also does NOT mock `FeatureFlagsContext`** (no `jest.mock("@/lib/contexts/FeatureFlagsContext", ...)` in the file). BottomNav calls `useFeatureFlags()` directly. Jest relies on a module that calls `createClient(SessionService, createConnectTransport(...))` — if that path throws in the JSDOM environment, the test will break silently or render an empty `morePages`. This gap is masked today because the backlog item is feature-flagged off by default, but any redesign that adds more feature-flagged items to the primary bar will surface this as a test reliability issue.

### 2b. Header.test.tsx — missing mocks for FeatureFlagsContext, MemoryNavBadge, NotificationsNavBadge

`Header.test.tsx` mocks 13 modules but is missing:
- `@/lib/contexts/FeatureFlagsContext` — `Header` calls `useFeatureFlags()` directly (line 17–32 of Header.tsx). The context's default value is `{ flags: {}, isLoading: true, ... }` (safe fallback), so tests pass today. But if `Header` gains any must-render path gated on a flag, a missing mock will silently hide items.
- `@/components/ui/NotificationsNavBadge` — rendered directly inside the nav loop in `Header.tsx` (line 95–96). No mock means it renders live and can throw if its own context dependencies aren't satisfied. Currently it does not throw because `useNotifications` is mocked, but this is a fragile dependency chain.
- `@/components/sessions/MemoryNavBadge` — rendered in `Header.tsx` line 115. No mock; same risk.

### 2c. Navigation.test.tsx — feature flag mock is coarse

`Navigation.test.tsx` mocks `useFeatureFlag` (singular) at the module level. `Navigation.tsx` is a legacy component (not in the main layout path — it's a standalone `<nav>` component). If the redesign retires it or moves its feature-flag gating to `useFeatureFlags` (plural), the mock will stop working.

### 2d. DrawerNav — zero unit tests

There is no `DrawerNav.test.tsx`. `CockpitShell.test.tsx` only mocks DrawerNav away entirely (`DrawerNav: () => <nav ... />`). Any restructuring of DrawerNav (adding group headers, section dividers) has zero test coverage to protect against regressions.

---

## 3. Badge Regression Risks

### ReviewQueueNavBadge — two render paths, only one is generic

In `BottomNav.tsx` the `renderPrimaryItem` function has a hardcoded `href === routes.reviewQueue` branch (lines 88–94) that renders `<ReviewQueueNavBadge inline={true} />` alongside the icon. Every other primary item just renders the icon. If a grouped redesign moves Review Queue into a different position or changes how items are rendered (e.g., via a shared render function that doesn't have this special case), the badge will silently disappear.

In `DrawerNav.tsx` the badge is also hardcoded as a href check (lines 55–58). Same risk.

In `Header.tsx` the same pattern exists (lines 91–98) — a ternary that checks for `routes.reviewQueue`, `routes.unfinished`, and `routes.notifications`. All three render badge components inline. A refactoring that moves badge rendering into the `NavPage` object (e.g., a `badge?: ReactNode` field) would fix this, but the current three-way special-case must be preserved or migrated atomically.

### Notifications badge — fully custom-rendered, excluded from BOTTOM_NAV_MORE

Notifications is marked `bottomNavPrimary: true` specifically to keep it out of the More sheet (see comment in `nav-pages.ts` line 44). The actual rendering uses custom HTML (Bell icon + unread count span) with no `renderPrimaryItem` call — it's hardcoded after the `primaryPages.map()` call (lines 163–177). If the redesign changes how "primary" is defined, or if `routes.notifications` ever appears in `morePages`, it will render twice.

---

## 4. Feature Flag Interaction — Empty Group Header Risk

The current `BottomNav.tsx` filters pages by flag **after** fetching the flat list:
```typescript
const filterByFlag = (pages: NavPage[]) =>
  pages.filter((p) => !p.featureFlag || flags[p.featureFlag]);
const primaryPages = filterByFlag(BOTTOM_NAV_PRIMARY);
const morePages = filterByFlag(BOTTOM_NAV_MORE);
```

Currently the only feature-flagged item is `backlog` in `BOTTOM_NAV_PRIMARY`. If the redesign introduces groups where a group header is rendered separately from its items (e.g., a "Work" group containing Backlog + Unfinished), and Backlog is the only item in that group, the group header `<h3>Work</h3>` will still render when the flag is off — resulting in an orphan header with no items beneath it.

**Concrete risk:** Any group structure must check whether the group has any visible (flag-passing) items before rendering the group header. Vanilla-extract CSS alone cannot fix this — it requires runtime JavaScript logic.

The `DrawerNav.tsx` iterates `NAV_PAGES` directly and does not filter by `featureFlag` at all (line 34). This means feature-flagged items render in the DrawerNav regardless of the flag state. This is a pre-existing bug; the redesign should fix it or risk surfacing it more visibly with group headers.

---

## 5. Active State Detection

### 5a. Query-param href breaks `pathname.startsWith()`

The Config Files entry has:
```typescript
href: routes.settings + "?tab=config-files"   // = "/settings?tab=config-files"
```

Next.js `usePathname()` returns only the path segment without the query string. So `pathname?.startsWith("/settings?tab=config-files")` is **always false** — the Config Files item is never highlighted as active, even when the user is on that tab.

The same issue affects the `isMoreActive` computation in `BottomNav.tsx` (line 70). When a user visits `/settings?tab=config-files`, `isMoreActive` returns `false` (the More button won't light up), because the More sheet contains Config Files with a query-param href that never matches.

**Fix required before or during redesign:** Either use `href: routes.settings` with a `matchHref?` field for active-state logic, or strip query params from the stored href and accept that Config Files / Features / Settings all light up together.

### 5b. `/settings` prefix collision

Both Settings (`href: "/settings"`, `mobileNav: false`) and Features (`href: "/settings/features"`) start with `/settings`. `pathname.startsWith("/settings")` is true for both — so any item that uses `/settings` as its href will appear active whenever the user is on any settings sub-page. This is currently masked because Settings has `mobileNav: false` (excluded from mobile), but consolidating settings items into a group could expose it.

---

## 6. vanilla-extract Build-Time Constraints

vanilla-extract generates CSS at build time — all style tokens must be static. There are no runtime constraints that block adding group header styles, but two rules apply:

1. **No inline style overrides for layout**: Per the CSS architecture rules (`.claude/rules/css-architecture.md`), `style={{ flexDirection: ... }}` and similar inline styles beat the CSS cascade. Group header styles must go in a `.css.ts` file, not as inline `style` props.

2. **New CSS variables need `globals.css` first**: If group headers need a divider color or label color not already in the theme contract, the token must be added to `web-app/src/styles/theme-contract.css.ts` before use. The `lint:css` CI step fails on undefined vars.

3. **`zIndex` numbers must be named**: The CSS architecture doc prohibits hardcoded `zIndex` integers. The More sheet already uses `zIndex.bottomNavMoreSheet` and `zIndex.bottomNavMoreBackdrop`. Any new overlay for a grouped popout or accordion must add a named slot to `theme-contract.css.ts`.

---

## 7. Mobile Safe-Area / `--bottom-nav-height` Risk

`BottomNav.tsx` uses a `ResizeObserver` to measure `nav.offsetHeight` and sets `--bottom-nav-height` on `document.documentElement` (lines 54–68). The `moreSheet` CSS positions itself at `bottom: var(--bottom-nav-height, 72px)`.

The ResizeObserver watches only the `<nav>` bar itself — not the More sheet. Adding more items to the More sheet does not affect `--bottom-nav-height` and is safe.

However, if the redesign adds an **accordion section inside the primary bar** (e.g., expandable groups within the nav bar itself), the bar's `offsetHeight` will change when the accordion opens. The ResizeObserver will fire and update `--bottom-nav-height`, which will cause the More sheet to jump. The solution is to keep the bottom nav bar a fixed height and put all variable-height content in the More sheet or a separate overlay.

The More sheet's `bottom` fallback value is hardcoded to `72px`. If the nav bar height changes substantially in a redesign (e.g., two rows), this fallback will be wrong on first render before the ResizeObserver fires.

---

## 8. More Sheet — Items with Zero Test Coverage

The following items from `BOTTOM_NAV_MORE` have no test in any `*.test.tsx` file:
- `Workflows` (`/workflows`)
- `Rules` (`/rules`)
- `History` (`/history`)
- `Config Files` (`/settings?tab=config-files`)
- `Features` (`/settings/features`)

None of these are exercised in `BottomNav.test.tsx`, `Header.test.tsx`, or `Navigation.test.tsx`. There is no test that:
- Opens the More sheet and checks that these items render
- Checks their `href` values
- Verifies their active states

The handedness toggle button and Account link (rendered directly in the More sheet, not from `BOTTOM_NAV_MORE`) also have zero test coverage.

---

## 9. Summary of Highest-Risk Items

1. **BottomNav.test.tsx `PRIMARY_ITEMS` hardcode + missing FeatureFlagsContext mock** — any change to which items are primary, their labels, or adding more feature-flagged items will break the test silently or with a confusing error.

2. **Badge special-casing is scattered across 3 files** — `ReviewQueueNavBadge`, `UnfinishedNavBadge`, and `NotificationsNavBadge` are each rendered via inline `href === routes.X` checks in DrawerNav, Header, and BottomNav. These must all be migrated together or any one will silently drop a badge.

3. **Config Files href contains a query string that is always undetectable by `pathname.startsWith()`** — the item is permanently broken for active-state and isMoreActive detection. This is a pre-existing bug that the redesign must fix when consolidating Settings/Config Files/Features.
