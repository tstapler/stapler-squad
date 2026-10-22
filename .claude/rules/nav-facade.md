---
globs:
  - "web-app/src/**"
---

# Every Page Needs a RouteMeta Row (and, later, Navigation Goes Through `@/lib/nav`)

**Route metadata (in force now).** Every `web-app/src/app/**/page.tsx` needs a row in `web-app/src/app/route-meta.ts`, whose `title` equals the page's effective Next title (the page's own `metadata.title`, else the nearest layout's, else the root's) and whose `guard` names the feature flag if a `FeatureGuard` layout covers it. `route-meta.test.ts` parses the sources and fails on any mismatch, and its message says to fix the row.

**Feature-flag guards.** Gate a route subtree with `<FeatureGuard flag="...">` (`web-app/src/app/FeatureGuard.tsx`) in its `layout.tsx`, and add the flag to `GuardFlag` in `route-meta.ts`. Do not write a bespoke `router.replace("/")` guard in a layout.

**Navigation facade (in force).** Import `useRouter`, `usePathname`, `useSearchParams`, `Link` and `Redirect` from `@/lib/nav`, never from `next/navigation` or `next/link`; do not call `window.history.pushState`/`replaceState` outside `src/lib/nav` (use `useReplaceSearch`, `replaceHash`, `pushTrayEntry`, or `pushHistoryMarker(key, value)` for a keyed entry). Pass `{ resetScroll: false }`, not Next's `{ scroll: false }`. ESLint errors on violations (`no-restricted-imports` for `next`, `next/*`, `react-router*` and `no-restricted-properties` for `pushState`/`replaceState`; `src/app/fonts.ts` is the one disabled line, until Story 9.1); rewrite old imports with `node web-app/scripts/codemods/next-nav-to-facade.mjs <file-or-dir>`. In Jest, mock `@/lib/nav` (spread `jest.requireActual("@/lib/nav/impl.memory")` so `Link` and the other members stay defined), not `next/navigation`.

## Why

The build is moving off Next, so titles and guards live in data that both builds read, and routing sits behind one module so the router choice stays swappable. Without the drift test a new page silently ships with the wrong tab title in the other build.
