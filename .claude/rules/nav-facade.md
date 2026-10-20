---
globs:
  - "web-app/src/**"
---

# Every Page Needs a RouteMeta Row (and, later, Navigation Goes Through `@/lib/nav`)

**Route metadata (in force now).** Every `web-app/src/app/**/page.tsx` needs a row in `web-app/src/app/route-meta.ts`, whose `title` equals the page's effective Next title (the page's own `metadata.title`, else the nearest layout's, else the root's) and whose `guard` names the feature flag if a `FeatureGuard` layout covers it. `route-meta.test.ts` parses the sources and fails on any mismatch, and its message says to fix the row.

**Feature-flag guards.** Gate a route subtree with `<FeatureGuard flag="...">` (`web-app/src/app/FeatureGuard.tsx`) in its `layout.tsx`, and add the flag to `GuardFlag` in `route-meta.ts`. Do not write a bespoke `router.replace("/")` guard in a layout.

**Navigation facade (from the facade PR onward).** Import `useRouter`, `usePathname`, `useSearchParams`, `Link` and `Redirect` from `@/lib/nav`, never from `next/navigation` or `next/link`; do not call `window.history.pushState`/`replaceState` outside `src/lib/nav`. This is not enforced yet, so existing imports stay until the migration lands.

## Why

The build is moving off Next, so titles and guards live in data that both builds read, and routing sits behind one module so the router choice stays swappable. Without the drift test a new page silently ships with the wrong tab title in the other build.
