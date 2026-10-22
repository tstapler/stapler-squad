# Architecture Research: bun-build-7a-baseline-regressions

## Scope

Five regressions/gaps after PRs 4/5a/6a merged into the Bun migration. This document
answers each architectural question from the triage brief and maps the root cause for
each regression to the relevant seam(s) in the codebase.

---

## Q1: Does `RootShell.tsx`'s `tabIndex=-1` on `<main>` affect Next's announcer?

`RootShell.tsx` (line 25) renders:
```tsx
<main id="main-content" ref={mainRef} tabIndex={-1} className={mainContent}>
```

`tabIndex=-1` makes `<main>` programmatically focusable (so `useFocusOnNavigate` can call
`main.focus()`) but removes it from the Tab order. This is the correct pattern for a skip-link
target.

**Effect on Next's announcer**: none direct. Next 15 App Router's `__next-route-announcer__`
is inserted into `<body>` by the Next runtime after any client-side navigation via `<Link>` or
`router.push`. The announcer does NOT depend on `<main>` focus or tab-index.

However: `DrawerNav.tsx` (line 4) imports `Link` from `@/lib/nav`. On the Next build,
`@/lib/nav/impl` is aliased to `impl.next.tsx`, which exports `NextLink` wrapped with
`prefetch={false}`. Nav drawer links **are** `NextLink` components on Next — they are NOT
plain `<a>` tags. A click on them triggers Next's client-side router, which fires the announcer.

**Confirmed by**: `measureScrollPolicy` in `BaselineProbes.ts` uses the same
`navLink(/history/i).click({ force: true })` and the scroll-policy baseline passes (navigation
completes), so the click does trigger client-side navigation. R1's failure is specific to
the announcer not appearing, not to navigation not happening.

**Root cause of R1**: Next 15 does navigate, but `__next-route-announcer__` is not injected
(or its text is not set). Likely cause: the `<title>` element changed. In `layout.tsx`,
`metadata.title` sets the static title "Stapler Squad Sessions". Page-level metadata (e.g.,
`metadata.title = "History - Stapler Squad"`) is required for the announcer to carry the
per-page title. If PR 4/5a/6a removed per-page metadata exports from history/review-queue
pages, or if Next's announcer now reads `document.title` at a different time, the announcer
fires with the wrong text (or empty text), making the count 0 in the test's `hasText` filter.

The `RouteAnnouncer` in `entry.tsx` (`role="status"`, `data-testid="route-announcer"`) is
**only** rendered on the Bun build — it is inside `NavigationEffects` which is only called
from `AppLayout` inside `createAppRouter` (Bun-only entry point `main.tsx`). It is absent
from the Next DOM, so there is no conflict between the two announcers.

**Fix approach for R1**: Verify each page (history, settings, etc.) still exports correct
`metadata` with a unique `title`. If Next's announcer fires but with the wrong text, update
the `metadata.title` exports. If the announcer does not fire at all, investigate whether
Playwright `{ force: true }` dispatches a click that Next's router event-delegation intercepts
(it should, since it bubbles to document, but confirm against Next 15.3.2).

---

## Q2: Does the nav drawer use `NextLink` or plain `<a>` tags?

`DrawerNav.tsx` line 4: `import { Link, usePathname } from "@/lib/nav";`
`@/lib/nav/index.ts` re-exports from `@/lib/nav/impl`.
`@/lib/nav/impl` is aliased per build:
- Next: `impl.next.tsx` → `NextLink` wrapper with `prefetch={false}`
- Bun: `impl.router.tsx` → React Router `Link`

The drawer uses the **nav facade `Link`** — it is `NextLink` on Next and React Router `Link`
on Bun. No plain `<a>` tags.

---

## Q3: What does `buildThemeBootScript` do — does it strip font variable classes?

Source: `src/styles/theme-boot-script.ts` line 11:
```ts
const allThemeClasses = Object.values(map).join(" ");
```
`map` is a `ThemeMap` (`Record<ThemeName, string>`) containing only VE-hashed theme class names
(e.g., `_matrixTheme_abc123`). The script removes only these names from `<html>`, then adds the
resolved theme class.

`next/font/local` variable classes (e.g., `__className_xyz789`) are **not** in `allThemeClasses`
and are not removed. The boot script is safe for font classes by design.

**Root cause of R2**: `layout.tsx` line 41 sets `<html>` with 5 classes: `cleanTheme` + 4
`next/font/local` variable classes. After the boot script, any theme has exactly 5 classes (1
theme + 4 font vars). The test expects class count identical across themes and ≥4 shared
(font) classes. This should pass with current code.

If R2 is failing, check these two paths:
1. PR 6a may have changed the Next build pipeline (ESLint/tsconfig) so the boot script is no
   longer inlined as a `<script>` tag in `<head>` — if `dangerouslySetInnerHTML` was removed
   or the `<head>` structure changed, the script runs after first paint, making `loadWithStorage`
   see the pre-script classes.
2. PR 4/5a may have removed one of the font imports from `layout.tsx` (e.g., removed `cinzel`
   or `inter`), reducing the font class count below 4. Confirm the current import list in
   `layout.tsx` still includes all four: `jetbrainsMono`, `rajdhani`, `cinzel`, `inter`.

The custom-theme test (`facts.style` missing `--e2e-probe`): The boot script branch at line 12
writes `style.setProperty(k, c.props[k])` for custom themes. This branch only fires if
`c.cls` matches `/^\S+$/` and `c.props` exists. The test fixture passes `cls: 'e2e-custom-theme'`
(valid) and `props: { '--e2e-probe': '42px' }` (valid). The custom theme branch should work.
If this assertion is failing, the boot script is not executing at all (script-inlining regressed).

---

## Q4: Is there a `not-found.tsx` in `web-app/src/app/`?

No. `find web-app/src/app -name "not-found.tsx"` returns nothing. The App Router convention
file does not exist; unmatched Next routes fall through to the SPA and the app redirects
client-side to `/`.

`NOT_FOUND_META` in `route-meta.ts` (`title: "Page not found - Stapler Squad"`) is used by
the **Bun** router's `titleFor(pathname)` fallback — it does not affect Next.

**R3 disposition**: If the baseline test is failing (observed title ≠ "Stapler Squad Sessions"
or finalPath ≠ "/"), something changed the Next not-found fall-through behavior in PRs 4/5a/6a.
Candidates: a redirect route was added or removed; or a new `app/[...slug]/page.tsx` catch-all
was added that renders something other than the sessions page. Since no `not-found.tsx` exists,
this is most likely a route-table change — check whether any route that previously caught
`/nonexistent` and redirected to `/` is still in place.

If the change is intentional (owner wants Next to also show the NotFound view), add
`src/app/not-found.tsx` importing `NotFound` from `@/app/NotFound`, and update
`tests/e2e/fixtures/next-baseline.json` to `{ status: 200, finalPath: "/nonexistent", title: "Page not found - Stapler Squad" }`.

---

## Q5: R4 — Tab-switch resets scroll: root cause

`useUpNextTab.setTab` calls `replaceSearch` → `replaceSearchInHistory` → `history.replaceState`.
This does NOT fire a `popstate` event and does NOT trigger `useNavigationType`/`useLocation` in
react-router. `ScrollPolicy` only activates on route navigation events. Tab switching is NOT a
route navigation, so `ScrollPolicy` does not reset scroll.

The scroll reset observed by the test comes from a different source:
`AppShellPage.SCROLLER` finds the element with the **largest scrollable overflow** at the time
of measurement. Before the tab switch, the 'prs' panel is active and has the most content, so
the SCROLLER finds it at `scrollTop = SCROLL_OFFSET`. After switching to 'queue', Radix UI makes
the 'queue' panel visible (`Tabs.Content` with `tabIndex={0}`) and hides the 'prs' panel. The
SCROLLER now finds the 'queue' panel (or a different element) as the new largest scrollable
element — that element's `scrollTop` is 0 (freshly visible). The test sees 0 and fails.

`UpNextTabs.Tabs.Content` uses `key={tab}` (each panel's own stable string, e.g. "prs"), not
`key={value}` (the active tab). Panels are NOT remounting on tab switch. The scroll reset is
a measurement artifact, not a React state issue.

Both builds fail because SCROLLER's dynamic detection is affected by the same DOM change
regardless of whether ScrollPolicy is present.

**Fix**: Add a stable `data-testid="up-next-scroll-container"` to the outer `div.container` in
`UnfinishedTab.tsx` (or use the `UpNextTabs`'s root element). Update `AppShellPage` to use this
stable selector when computing scroll for the `/unfinished` page, OR give the whole tab region
a single persistent scroll host that wraps `Tabs.Root` and is the CSS scroll container.

The simplest path: give the outer container in `UnfinishedTab.tsx` (the `styles.container` div)
`id="up-next-scroll-container"`. The SCROLLER will prefer `#id` lookup and find the same element
regardless of which tab is active.

---

## Q6: R5 — Bun POP scroll-restore: `describe`/`find` path stability after Suspense remount

`ScrollPolicy.describe(element)` returns `#${id}` if the element has an `id`, else a path of
`tagname:sibling-index` segments from the body root. `find(descriptor)` uses `getElementById`
for `#id` descriptors, or walks `body.children[index]` for path descriptors.

`PageBoundary` in `entry.tsx` (line 111):
```tsx
<Suspense key={route.path} fallback={<PendingFallback>...</PendingFallback>}>
  <Page />
</Suspense>
```

`key={route.path}` is stable within a route (always `"/settings/features"` for that page), so
the Suspense boundary itself doesn't remount. However, `Page` is a lazy component — on the
initial load React renders the fallback until the chunk loads, then renders the page. On Back
(POP), React Router keeps the page component in memory (it was already loaded), so no Suspense
fallback fires and the DOM is committed synchronously. The `useLayoutEffect` in `ScrollPolicy`
runs after commit, so `find()` should see the DOM.

The real issue: the scroll container on `/settings/features` has no `id`. The `describe()`
path is `main:0>div:0>...` (tag:sibling-index), which is correct for the first render. But
`positionsByLocation.get(location.key)` may return `undefined` if the key under which scroll
was recorded differs from the key React Router surfaces on POP.

React Router 7 assigns a `location.key` of `"default"` to the initial entry and a random UUID
to each subsequent push. On POP, the key is the UUID that was assigned when the entry was first
pushed. `ScrollPolicy.currentKey.current` is updated to track the current key. The `record`
handler stores positions under `currentKey.current`. On POP, `location.key` is the original
entry's key — this should match the recorded key.

The more likely root cause (confirmed by the spec comment "verified with a real mouse-wheel
scroll"): `describe(element)` captures the scroller's structural path at record time. On POP,
React re-renders the page inside the same Suspense tree (no key change for settled lazy pages),
but if the settings page uses conditional rendering that changes sibling indices between loads
(e.g., a loading banner that appears/disappears), the path changes and `find()` returns null.

**Fix**: Add `id="settings-features-scroll-container"` (or similar) to the scroll host on
`/settings/features`. `describe()` will return `"#settings-features-scroll-container"` and
`find()` will use `getElementById`, which is remount-safe. More broadly: any page whose scroll
should be restored by `ScrollPolicy` on POP needs a stable `id` on its scroll container.

---

## Architecture Summary

| Regression | Seam | Root Cause | Fix |
|---|---|---|---|
| R1 — announcer | Next runtime / page metadata | `__next-route-announcer__` fires but `document.title` is wrong, OR per-page `metadata` exports were removed in PRs 4/5a/6a | Restore per-page `metadata.title` exports; verify baseline title strings |
| R2 — font FOUC | `layout.tsx` + boot script | Boot script not inlined (script tag missing after PR 6a build-config change) OR font import removed from `layout.tsx` | Verify `<head>` still has `dangerouslySetInnerHTML` script; confirm all 4 font `variable` imports remain in `layout.tsx` |
| R3 — not-found | `next-baseline.json` fixture | Either intentional (a new convention file was added) or unintentional route-table change | Decision gate: if intentional, add `app/not-found.tsx` + update fixture; if not, find and revert the route change |
| R4 — tab scroll | `AppShellPage.SCROLLER` heuristic | SCROLLER measures a different DOM element after tab switch; no remount, just visibility change | Add stable `id` or `data-testid` to outer `UnfinishedTab` container; SCROLLER prefers `#id` |
| R5 — POP restore | `ScrollPolicy.describe()`/`find()` | Scroll container on `/settings/features` has no `id`; structural path may change between record and restore | Add stable `id` to settings-features scroll container |

---

## Tech Debt Disposition: Extend as-is

The architecture itself is sound. The nav facade, ScrollPolicy, RouteAnnouncer split, and
theme boot script are correctly designed:
- Nav facade correctly isolates Next vs Bun routing behind a compile-time alias.
- `RouteAnnouncer` is Bun-only and does not pollute the Next DOM.
- `buildThemeBootScript` correctly limits class removal to VE theme names.
- `ScrollPolicy`'s `describe`/`find` heuristic is the right approach; it just needs IDs added
  to the scroll containers of pages that need POP restore.

None of the five regressions require architectural refactoring. All five are isolated,
mechanical fixes:
- R1/R2/R3: verify and restore config/metadata that regressed in PR 4/5a/6a.
- R4: add a stable `id` or `data-testid` to the UpNext outer scroll container.
- R5: add a stable `id` to the settings/features scroll container.

**Disposition: Extend as-is** — no seam isolation or refactor-first work needed.
