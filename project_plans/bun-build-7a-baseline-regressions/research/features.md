# Edge Cases and Feature Landscape — Bun Build 7a Baseline Regressions

## R1: Route Announcer Absent

### What exists
`web-app/src/lib/nav/RouteAnnouncer.tsx` implements the Bun-build announcer as `role="status"` (polite), distinct from Next's `#__next-route-announcer__` which is `role="alert"` (assertive). It is wired unconditionally into `NavigationEffects` in `entry.tsx` alongside `ScrollPolicy` and `useFocusOnNavigate`.

### E2E specs that touch the announcer
- **`tests/e2e/route-focus-and-announce.spec.ts`** — primary spec:
  - `test.describe('Next baseline')` asserts `role="alert"` and one live region with the title after a nav click (reads from Next's built-in announcer, not `data-testid`).
  - `test.describe('Bun only')` (guarded by `test.skip(!isBunBuild())`) asserts `role="status"`, focus on `main#main-content`, clear-then-set sequencing, omnibar autofocus precedence, Back announcement, redirect de-duplication, and duplicate-`main` tolerance.
  - `watchAnnouncer()` hooks `[data-testid=route-announcer]` — this testid must be present on the Bun build.
- **`tests/e2e/not-found.spec.ts`** — Bun-only test (line 66): `expect(page.getByTestId('route-announcer')).toHaveText(NOT_FOUND_META.title)` after a client-side navigation to an unknown path.

### Other specs with incidental `aria-live`/liveRegion references (not blocked on R1)
Files: `accessibility.spec.ts`, `backlog-edit-buffering.spec.ts`, `backlog-send-back-feedback.spec.ts`, `backlog-copy-link.spec.ts`, `connection-count-indicator.spec.ts`, `bulk-select.spec.ts`, `handoff-summary.spec.ts`, `scroll-forward-live-region-announcements.spec.ts`, `notifications-needs-decision.spec.ts`, `tagging-rules-crud.spec.ts`, `multi-window.spec.ts`, `terminal-resync-*`, `insights-session-route.spec.ts`, `log-viewer.spec.ts`, `session-creation-async.spec.ts`, `session-completion-summary.spec.ts`.

Most are either scoped to `[data-testid=backlog-page]` (so absent from the route announcer selector) or already neutralised by the `build-coupled-specs.json` strategy (filtering by role/text to skip Next's always-present alert).

### Edge cases
- `accessibility.spec.ts` line 230–236 explicitly scopes `aria-live="assertive"` to `[data-testid=backlog-page]` to avoid Next's `#__next-route-announcer__`; on the Bun build that element does not exist, so the `:not()` in other specs is a no-op — confirmed neutral in `tests/e2e/baseline/build-coupled-specs.json`.
- The Bun announcer uses `data-testid="route-announcer"` while the `AppShellPage.liveRegions` locator is `[role=status],[role=alert],[aria-live]` — both match the Bun announcer's `role="status"`.

---

## R2: Theme FOUC Font Classes

### Next approach (`app/layout.tsx`)
```html
<html class="cleanTheme __jetbrainsMono-variable __rajdhani-variable __cinzel-variable __inter-variable">
```
Font variable CSS custom properties (`--font-jetbrains-mono`, etc.) are set on `<html>` by `next/font/local` at SSR time.

### Bun approach (`build/html.ts` + `web-app/index.html`)
`index.html` has no `class` attribute on `<html>`. `injectThemeBoot()` replaces `class="[^"]*"` with `class="${map.clean}"` — **the font variable classes are not injected**. They are defined via `@font-face` in `web-app/src/app/fonts.css` (loaded by `main.tsx`), but the CSS custom properties that `next/font` normally emits (`--font-*: "JetBrains Mono"`) require the variable classes on `<html>`.

### Edge case
`web-app/src/lib/contexts/ThemeContext.tsx` reads and writes the persisted custom theme class. The FOUC script (`buildThemeBootScript`) swaps `<html>`'s class to the stored theme on load — but only between theme classes, not font-variable classes. So on a hard reload with a custom theme, the font variables are also absent from the clean-theme baseline that FOUC starts from.

---

## R3: Not-Found Behavior Changed

### Next baseline (recorded in `tests/e2e/fixtures/next-baseline.json`)
```json
"notFound": { "status": 200, "finalPath": "/", "title": "Stapler Squad Sessions" }
```
Unknown paths redirect client-side to `/` (the sessions page). No dedicated not-found view.

### Bun behavior
`NotFound.tsx` renders in place (the URL stays as typed, status 200, no redirect). Title: `"Page not found - Stapler Squad"` (`NOT_FOUND_META.title` from `route-meta.ts`). The component is wired as `{ path: "*", element: spec.notFound }` in `entry.tsx`.

### Specs that read `baseline.notFound`
- `tests/e2e/not-found.spec.ts` — `test.describe('Next baseline')`: calls `measureNotFound()` from `tests/e2e/pages/BaselineProbes.ts` and compares against `baseline.notFound`. `test.describe('Bun only')` asserts the new view, its heading, path display, "Go to Sessions" link, and conditional "Go back" button.

### `measureNotFound` probe (BaselineProbes.ts)
Loads the missing path, waits up to 5 s for a URL change (catches client-side redirect), returns `{ status, finalPath, title }`.

### Edge cases
- The "Go back" button only renders when `window.history.length > 1`; the Bun-only test stubs `history.length = 1` to verify the no-back-button case.
- A client-side navigation to an unknown path (not a hard load) is also tested to confirm the route announcer carries the not-found title.

---

## R4: Tab-Switch Resets Scroll

### What the test asserts
`tests/e2e/scroll-policy.spec.ts` (the Next-baseline describe block, line 33–56): scrolls the Up Next page to a known offset, switches the active tab from `prs` to `queue` via `upNext.selectTab('queue')`, then asserts `shell.scrollTop() === before`.

### Current status — already failing on Next
`tests/e2e/baseline/bun-expected-diff.json` entry:
```json
{
  "spec": "scroll-policy.spec.ts",
  "test": "switching the Up Next tab leaves scroll and the history length unchanged",
  "reason": "Fails identically on the Next build at the same commit; not caused by the Bun migration."
}
```
Issue #1136. This is a shared baseline failure — not a Bun regression to fix in Story 7a.

### Other tab-switching scroll specs
- `tests/e2e/up-next-tabs.spec.ts` — exercises tab selection, keyboard model, Back behaviour, URL state, and badge contrast; does **not** assert `scrollTop` — safe.
- `tests/e2e/scroll-policy.spec.ts` (Bun-only, line 59–75) — `test.fail()` on POP restore; see R5 below.

### AppShellPage scroll helpers
`scrollRange()`, `scrollTop()`, `scrollTo()` in `tests/e2e/pages/AppShellPage.ts` scan `overflowY: auto|scroll` elements and pick the one with the largest range, skipping tray elements. This is how the Up Next content div is located without a fixed selector.

---

## R5: Bun POP Scroll-Restore Broken

### Root cause (per spec comment in scroll-policy.spec.ts lines 62–68)
`ScrollPolicy.tsx`'s POP branch (`src/lib/nav/ScrollPolicy.tsx`) calls `restore(saved)` with the map saved under `location.key`, but `backScrollTop` is observed as 0 on every run. Comment says: "verified with a real mouse-wheel scroll, not just a scripted scrollTop write, so this isn't a test-harness timing artifact."

### Current status
`test.fail()` is already applied to the POP-restore Bun test (line 69). The test file comment says this "Needs an issue filed and a fix outside PR 7b's file set before this can be un-marked."

### Implementation detail
`ScrollPolicy.tsx` records scroll events via `document.addEventListener("scroll", record, { capture: true })` and stores positions in `positionsByLocation: Map<string, Map<string, Position>>`. On POP it looks up `positionsByLocation.get(location.key)`. The `describe()` function identifies scrollers by `#id` or tag/sibling-index path from `body`.

The likely gap: the scroll container on Up Next / Settings is an inner element found by `AppShellPage`'s runtime scan. If the element's DOM path changes between the first render and the back-navigation re-render (because the page remounts), `find(descriptor)` may return null and `applyPosition` silently fails.

### Related unit tests
`web-app/src/lib/nav/__tests__/scroll-policy.test.tsx` — Jest-level unit tests for `ScrollPolicy`.

---

## next-baseline.json Impact Summary

Specs that read `tests/e2e/fixtures/next-baseline.json` and which properties they consume:

| Spec | Properties read |
|---|---|
| `not-found.spec.ts` | `baseline.notFound` |
| `scroll-policy.spec.ts` | `baseline.scrollPolicy` |
| `route-focus-and-announce.spec.ts` | `baseline.skipLink.*`, `baseline.historyNavAriaCurrent` |
| `route-titles.spec.ts` | `baseline.redirects` |
| `tools/capture-next-fixtures.spec.ts` | Writes the file |

Updating `next-baseline.json` would break the four consumer specs if the Bun build produces values that differ from the recorded Next values without the specs being updated in lockstep.

---

## User-Unstated Needs / Edge Cases

1. **`route-focus-and-announce.spec.ts` Bun block depends on `useFocusOnNavigate`** — the focus-on-`main` assertion (`document.activeElement === document.getElementById('main-content')`) depends on both the route announcer existing and `useFocusOnNavigate` running. If either is absent, two tests in the Bun block fail, not just one.

2. **The Bun skip-link tab-position assertion** is tied to the Next baseline value (`baseline.skipLink.tabPresses = 19`). On the Bun build the tab order may differ (no nav-before-skip-link issue), so the skip-link Bun assertions in `route-focus-and-announce.spec.ts` are currently under a `test.skip(!isBunBuild())` guard and will need the policy decided in PR 7b.

3. **`accessibility.spec.ts` AC #29** references `#__next-route-announcer__` in its comment but scopes the assertion inside `[data-testid=backlog-page]` — the Bun build's absence of that element does not affect the assertion; neutral.

4. **Font CSS custom properties** — vanilla-extract theme tokens reference `var(--font-jetbrains-mono)` etc. which are set by `next/font`'s generated CSS. On the Bun build those variables are undefined unless the font-variable classes are injected onto `<html>`. The FOUC script only swaps theme classes; it does not restore font-variable classes after the theme swap.

5. **`bun-expected-diff.json` already classifies R4 (tab-switch scroll) as a shared Next failure** at issue #1136 — fixing it is out of scope for 7a and should not be listed as a new Bun regression.
