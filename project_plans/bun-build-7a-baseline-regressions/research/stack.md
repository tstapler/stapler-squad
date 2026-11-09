# Technology Stack Research: bun-build-7a-baseline-regressions

## 1. Key Library Versions

Source: `web-app/package.json`

| Library | Version |
|---|---|
| Next.js | 15.3.2 |
| React | ^19.0.0 |
| react-router | 7.18.4 |
| @playwright/test | ^1.57.0 |
| Bun (packageManager) | 1.4.3 |

## 2. R1 — `__next-route-announcer__` Absence

The spec `route-focus-and-announce.spec.ts` asserts that on Next, the live-region text is in an element with `role="alert"` — Next's own `__next-route-announcer__` element. The custom `RouteAnnouncer` in `src/lib/nav/RouteAnnouncer.tsx` uses `role="status"` and `aria-live="polite"`, which is explicitly described in its docstring as a *replacement* (ADR-001): "Polite live region that speaks `document.title` after each pathname change (Next's own announcer is assertive; ADR-001)."

Next 15 still ships its own `__next-route-announcer__` (role=alert, assertive). The test checks `shell.liveRegions.filter({ hasText: HISTORY_TITLE })` for count 1, asserting the `role="alert"` variant. If the custom `RouteAnnouncer` renders *alongside* Next's built-in announcer, there would be two, not one. If the custom announcer rendered instead of Next's (i.e., Next's was removed), count would be zero for the role=alert check. The regression most likely means the custom `RouteAnnouncer` (data-testid="route-announcer") is now being rendered in the Next build path when it should not be, or Next's built-in announcer is not firing. The Bun block in the same spec skips on Next and looks for `data-testid="route-announcer"` — the custom one.

**Known behavior:** Next 13+ App Router removed server-side rendering of `__next-route-announcer__` and made it purely client-side. It still exists in Next 15. No version-change removal is expected; the regression is most likely from a change to how `RouteAnnouncer` is mounted in the layout.

## 3. R2 — Theme FOUC font-variable-class test failures on Next

The spec `theme-fouc.spec.ts` line 88–93 asserts that `shared.length >= 4` font variable classes are present on `<html>` under every theme. The test comment on line 89 reads: "Exactly one class differs between themes; the rest (next/font variables) are shared."

On Next, fonts are injected via `next/font/local` as CSS variable classes on `<html>`. The `buildThemeBootScript` in `src/styles/theme-boot-script.ts` builds an IIFE that removes all theme classes from `<html>`, then adds the correct one. The `allThemeClasses` value passed to the script is `Object.values(map).join(" ")` — only the VE hashed theme class names, not `next/font` variable classes. So the boot script should not remove font classes.

The failure direction likely involves either:
- The `allThemeClasses` variable in the injected script accidentally containing font variable class names (if they appear in the VE class map), causing them to be stripped; or
- The test fixture for Next's `loadWithStorage` calling `page.goto` without first navigating away, meaning `next/font` classes never appear.

The `src/theme-boot.ts` file also exists separately from `src/styles/theme-boot-script.ts` — these may be duplicated entry points, with one potentially including font class names in the class removal list.

## 4. R3 — not-found.tsx convention file

The Next.js App Router convention file is `not-found.tsx` (all lowercase). The app has `NotFound.tsx` (PascalCase) — a React component used by the Bun router build — but **no `src/app/not-found.tsx` convention file**.

Without `src/app/not-found.tsx`, Next's App Router falls through to its own 404 shell, which in this app resolves to the sessions page at `/` (SPA catch-all). The baseline records: `{ "status": 200, "finalPath": "/", "title": "Stapler Squad Sessions" }`.

The spec `not-found.spec.ts` line 22 says the test `expect(observed).toEqual(baseline.notFound)` — it is asserting the *existing* Next baseline behavior (200 + redirect to `/`), not a not-found page. If the baseline changed, it means some intervening PR introduced a `not-found.tsx` or changed the root layout catch-all behavior.

## 5. R4/R5 — Scroll Policy

`src/lib/nav/ScrollPolicy.tsx` uses a `describe`/`find` approach for element-based scroll restoration:

- **`describe(element)`**: Returns `#id` if the element has an ID, else a path of `tagname:sibling-index` segments joined by `>`, walking up to `<body>`.
- **`find(descriptor)`**: Reverses it — `getElementById` for `#id` descriptors, or walks `body.children[index]` for path descriptors.
- **POP handling (line 108–110)**: `if (navigationType === "POP")` — looks up `positionsByLocation.get(location.key)` and calls `restore(saved)`. The spec comment at line 63–68 confirms the known gap: "ScrollPolicy.tsx's POP branch calls `restore()` with the position saved under the current `location.key`, but observed `backScrollTop` is 0" — the test is marked `test.fail()`.

**R4 (tab-switch resets scroll on both builds)**: `ScrollPolicy.tsx` line 113 calls `window.scrollTo(0, 0)` and resets `scrollers.current` on any `PUSH` navigation where the pathname changed. The Up Next tab switch uses `history.replaceState` (not router navigation), so `navigationType` should be `POP` or not fire at all — but if it fires as `PUSH` with a different pathname (e.g., `?tab=queue` adds a search param that triggers a pathname comparison issue), `normalize.ts` may need checking.

**R5 (Bun POP restore fails)**: The location key from react-router 7 may differ from what was recorded. The `positionsByLocation` map stores positions under `location.key` at record time; on POP, `location.key` is the key of the destination entry. If the keys are not stable across navigation (e.g., react-router 7.18 generates new keys on hard load), `positionsByLocation.get(location.key)` returns `undefined` and `restore` is never called.

## 6. `buildThemeBootScript` Details

Source: `src/styles/theme-boot-script.ts`

- Takes a `ThemeMap` (`Record<ThemeName, string>` — mapping theme name → VE hashed class).
- Generates an inline IIFE that:
  1. Reads `localStorage.getItem('stapler-theme')`.
  2. Looks up the hashed class from the map.
  3. Handles `custom:` prefix themes (reads `stapler-theme-custom` JSON, validates `cls` and `props`).
  4. Removes **all** theme classes (`allThemeClasses = Object.values(map).join(" ")`).
  5. Adds the resolved class.
  6. Catches errors and falls back to `map.clean`.

The script only removes classes from `Object.values(map)` — theme classes only, not font variable classes. However, if the Bun build passes a different map (one that includes font classes from its VE extraction pass), that is where the FOUC font-stripping would come from.

## 7. `not-found.tsx` Conclusion

There is no `src/app/not-found.tsx` (Next.js App Router convention file) in this repo. The `NotFound.tsx` component is a Bun-router-specific component, rendered when the Bun router catches an unmatched path. The baseline records Next's current behavior (200 + "/" redirect). If the baseline test fails, a route was added or changed in a way that alters this fall-through behavior.
