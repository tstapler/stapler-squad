# Pitfalls and Risks — bun-build-7a-baseline-regressions

## R1 — Route announcer: `__next-route-announcer__` absent

**What the test expects**

`route-focus-and-announce.spec.ts` line 34–36 (Next-baseline block):
```ts
await expect(shell.liveRegions.filter({ hasText: HISTORY_TITLE })).toHaveCount(1);
const announcer = shell.liveRegions.filter({ hasText: HISTORY_TITLE }).first();
await expect(announcer).toHaveAttribute('role', 'alert');
```
`liveRegions` matches `[role=status],[role=alert],[aria-live]`. The test expects exactly one live region carrying the title, with `role=alert` — that is Next's own `__next-route-announcer__` element.

**The structural danger**

The Next-baseline describe block has **no `test.skip(!isBunBuild())` guard**. It runs on both builds. On the Bun build, the custom `RouteAnnouncer` uses `role=status`, not `role=alert`. So `await expect(announcer).toHaveAttribute('role', 'alert')` fails on every Bun run. This is the regression: after PRs 4/5a/6a the CI runner may be using the Bun build, which means the "Next baseline" assertions fire against Bun and fail.

**Fix pitfall**

Adding `test.skip(isBunBuild(), '…')` to the Next-baseline block is the minimal fix, but it requires understanding which CI job runs against which build. If there is a single shared test server, the Next-baseline tests can never run on Bun. Removing the block entirely and relying only on the Bun-only block is safer but loses the recorded Next behavior as regression protection.

The `watchAnnouncer` helper at line 67 uses `document.querySelector('[data-testid=route-announcer]')` — this is the Bun custom element, not Next's own `__next-route-announcer__`. Any test in the Next-baseline block that calls `watchAnnouncer` would silently return an empty log on Next (no element found, no mutations observed).

---

## R2 — Theme FOUC: font-variable-class count test

**What the test checks**

`theme-fouc.spec.ts` lines 90–93 (Next-baseline block, "font variable classes survive every theme"):
```ts
const sizes = new Set(Array.from(fonts.values()).map((c) => c.length));
expect(sizes.size, 'class count is identical across themes').toBe(1);
```
It loads each of the 6 themes, captures `document.documentElement.classList` at `DOMContentLoaded`, and asserts every theme produces a list with the **same number** of classes. The assumption is that switching themes only swaps one hashed class; the four `next/font` variable classes (`jetbrainsMono.variable`, etc.) are static on the `<html>` element from `layout.tsx` and must survive theme switching.

**Root cause of failure**

`buildThemeBootScript` (in `theme-boot-script.ts`) constructs `allThemeClasses` as the space-joined values of the theme map, then removes ALL of them from `<html>` before adding the selected theme class:
```js
var themeClasses=<json>.split(' ');
themeClasses.forEach(function(c){if(c)document.documentElement.classList.remove(c);});
document.documentElement.classList.add(cls);
```
If a vanilla-extract theme class hash **collides** with a `next/font` variable class (extremely unlikely but possible if the class-name generation changed), removing that class removes a font variable. More likely: a PR changed `layout.tsx`'s static `className` on `<html>` to include a class that is also in the theme map, so it is removed by the boot script and causes `classes.length` to differ by theme.

Check the `className` on `<html>` in `layout.tsx` (currently `${cleanTheme} ${jetbrainsMono.variable} ${rajdhani.variable} ${cinzel.variable} ${inter.variable}`). If any of these class strings became empty string after a refactor (e.g., a next/font variable returns `""` during SSR), the count would vary.

**Fix pitfall**

The test asserts an exact count relationship. If the Bun build genuinely omits `next/font` classes (it uses `--font-*` CSS custom properties via `fonts.css` instead), the class count will legitimately differ between builds. The "Bun only" describe block already has its own FOUC test (`--font-* variables resolve under every theme`) that does not check class counts. Do **not** alter the class-count assertion to unify Next and Bun behavior; keep them in separate describe blocks.

---

## R3 — Not-found baseline changed

**What the test checks**

`not-found.spec.ts` line 21: `expect(observed).toEqual(baseline.notFound)` where `baseline.notFound` is:
```json
{ "status": 200, "finalPath": "/", "title": "Stapler Squad Sessions" }
```
`measureNotFound` navigates to `/nonexistent`, waits for a client redirect to settle (5 s timeout), and returns status, `pathname`, and `title`.

**Risk in updating `next-baseline.json`**

`next-baseline.json` is a single file imported by three spec files (`route-focus-and-announce.spec.ts`, `not-found.spec.ts`, `scroll-policy.spec.ts`). Changing `notFound.*` is safe for the other two since they read different top-level keys (`skipLink`, `historyNavAriaCurrent`, `scrollPolicy`). However, the value must be re-recorded from a **running Next build**, not inferred. If the redirect target changed (e.g., `/` now lands on a different route because a redirect was added in a PR), or the `Stapler Squad Sessions` title changed in `layout.tsx`'s `metadata`, the baseline entry must match the current Next behavior exactly.

The `NOT_FOUND_META` import from `../../web-app/src/app/route-meta` in `not-found.spec.ts` is used only in the Bun-only describe block (line 14 import, line 49 use). Changing `route-meta.ts` to add a not-found entry for the Bun build does not affect the Next baseline assertion — it reads from `next-baseline.json`.

---

## R4 — Tab-switch resets scroll (both builds)

**What the test checks**

`scroll-policy.spec.ts` lines 41–55: scrolls the `/unfinished?tab=prs` page to `SCROLL_OFFSET` (40 px), calls `upNext.selectTab('queue')`, then asserts `shell.scrollTop() === before`.

**`selectTab` does not re-mount the component**

`UpNextPage.selectTab` → `this.tab(id).click()` → Radix UI `Tabs.Trigger` `onValueChange` → `useUpNextTab.setTab` → `replaceSearch('?tab=queue')`. Radix UI `Tabs.Content` keeps all panels in the DOM (hidden with the `hidden` attribute). No component unmounts.

**Root cause: measurement-based scroller detection is fragile across tab panels**

`AppShellPage.SCROLLER` finds the element with the largest `scrollHeight - clientHeight`. Before clicking the queue tab, the PRs panel is active and the scroller it finds (e.g., a scroll region inside the PRs panel or the main layout) has a certain `scrollTop`. After clicking Queue, the Queue panel is revealed (hidden attribute removed) and its `Tabs.Content` may have a larger scroll range than the PRs panel, becoming the new dominant scroller — which starts at `scrollTop = 0`.

So `shell.scrollTop()` after the click returns 0 for the *newly dominant* scroll container, not the one scrolled before the click.

**Pitfall in fixing `ScrollPolicy.tsx`**

`useUpNextTab.setTab` calls `replaceSearch` (not `router.replace`). In the Bun/react-router build, `replaceSearch` calls `history.replaceState` directly, bypassing react-router. But react-router still picks up `replaceState` calls via its own history listener and issues a `REPLACE` navigation. This causes `ScrollPolicy.tsx`'s `useLayoutEffect` to fire with `navigationType === 'REPLACE'` and a **new** `location.key`. Even though the pathname check (`if (pathname === previousPathname.current) return`) prevents the full scroll-reset, `currentKey.current` is updated. Any scroll recorded before the tab switch is now stored under the old key; `restore` on Back would look up the new key and find nothing.

Do not change the `pathname === previousPathname.current` guard to also check search params — doing so would prevent scroll reset on genuine search-param-only navigations that do change the visible content.

---

## R5 — Bun POP scroll-restore fails

**Existing disposition**

The test is already marked `test.fail()` with an explanatory comment. It fails every run — this is the known-gap marker, not a newly broken test.

**Root cause (already documented in the test)**

`ScrollPolicy.tsx`'s `restore()` is called with `positionsByLocation.get(location.key)`. On Back, `location.key` is the key for the old entry. If scroll was recorded correctly, `saved` is non-null. `restore()` calls `applyPosition(descriptor, position)` for each entry. `applyPosition` calls `find(descriptor)` which, for an element without an `id`, walks the sibling-index path `tag:N>tag:M>…`.

The scroller on `/settings/features` is an inner `<main>` element (confirmed by `LONG_PAGE = '/settings/features'` and the comment about "the inner `<main>` on Settings"). If this main element has an `id`, `describe()` returns `#main-content` and `find()` works reliably. If it does not have an `id` or its index position changes after re-render on POP, `find()` returns `null` and `applyPosition` returns `false` — the restore silently does nothing.

**Pitfall in fixing the descriptor approach**

`describe()` uses the tag/sibling-index walk only as fallback when `element.id` is empty. Adding `data-testid` as a second candidate (before the walk) is the least-invasive fix. However:
- `find()` would need a complementary `querySelector('[data-testid=X]')` branch.
- All scroll containers that lack an `id` would need a `data-testid` added in production markup — an HTML change, not just a test change.
- The `scrollers` ref stores raw element references. Between saving and restoring (across a POP), the element is a new instance created by re-render; `find()` must re-query the DOM, which is what the descriptor approach is for. Any descriptor that is non-unique (duplicate `id`s, duplicate `data-testid`s) would restore the wrong element.

The test comment confirms this is a "known gap… needs an issue filed and a fix outside PR 7b's file set". Do not fix it as part of PR 7a; keep `test.fail()` as the gap marker.

---

## Cross-cutting risk: `next-baseline.json` as shared fixture

All three baseline spec files import the same JSON. Any structural change (rename a key, change a nested field) requires searching all three importers. The `scrollPolicy.scrolledTo` field (currently `1200`) is used in `scroll-policy.spec.ts` as the expected scroll target AND in `baseline.scrollPolicy` deep-equality. If the settings page height changes (making 1200 px unreachable), both the `scrolledTo` check and the `forwardNavScrollTop`/`backScrollTop` checks would fail simultaneously. Keep the `SCROLL_TARGET = 1200` constant and the fixture value in sync.
