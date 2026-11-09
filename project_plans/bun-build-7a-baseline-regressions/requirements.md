# Requirements: web-app Bun migration — PR 7a baseline regressions (bun-build-7a-baseline-regressions)

## Background

PR 7a (#1117, merged 2026-10-20) pinned Next.js behavior as a baseline for the ongoing Bun
router migration. After PRs 4 (#1128), 5a (#1131), and 6a (#1132) merged on 2026-10-21, four
of those baseline tests now fail, and one known Bun-only gap was documented with `test.fail()`.
This item is the cleanup step: root-cause and fix the regressions, decide on the not-found
unification question, and land PR 7b's Bun-only assertions on a green baseline.

---

## In-scope regressions and gaps

### R1 — Next route announcer absent (`route-focus-and-announce.spec.ts`)

**Symptom**: `shell.liveRegions.filter({ hasText: HISTORY_TITLE })` finds no element on
the Next build after a nav-link click. The test expects exactly one `role=alert` region
containing the page title (Next's `__next-route-announcer__`).

**Hypothesis**: Something in PR 4's `RootShell.tsx` or nav-facade changes prevents Next's
client-side router from running (and therefore from injecting `__next-route-announcer__`)
when `navLink.click({ force: true })` is called. Possible causes: the drawer nav links are
no longer `NextLink` components after the nav-facade refactor; or the `{ force: true }` path
through Playwright bypasses some condition that Next requires to detect navigation.

**Acceptance criterion**: On the Next build, a nav-link click from `/review-queue` to `/history`
produces exactly one live region containing the History page title with `role="alert"`.

---

### R2 — Theme FOUC: font-variable-class and custom-theme assertions fail on Next (`theme-fouc.spec.ts`)

**Symptom**: Two tests fail on the Next build:
- "font variable classes survive every theme": the class count is no longer identical across
  themes (or fewer than 4 shared classes remain).
- "a custom: theme applies its class and CSS variables before first paint": `facts.style`
  does not contain `--e2e-probe`, meaning the inline boot script no longer writes inline
  style props for custom themes.

**Hypothesis**: The `buildThemeBootScript` function was updated (or the `layout.tsx` HTML
class list was changed) during PRs 4/5a/6a so that the boot script now strips all classes
from `<html>` before applying the theme, or the custom-theme branch no longer writes
`style` attributes. May also be an unintentional side-effect of the ESLint/build config
changes in PR 6a.

**Acceptance criterion**: On the Next build, `html` carries the same number of CSS classes
for every theme (difference of ≤1 between any two themes), at least 4 font-variable classes
are present under every theme, and a `custom:` theme writes its CSS properties to
`html.style` before first paint.

---

### R3 — Not-found baseline changed (`not-found.spec.ts`)

**Symptom**: The Next baseline test `baseline.notFound` expects `{ status: 200, finalPath: "/",
title: "Stapler Squad Sessions" }` but now visiting `/nonexistent` on the Next build shows the
`NotFound.tsx` "Page not found" view (URL stays `/nonexistent`, title "Page not found - Stapler Squad").

**Hypothesis / decision gate**: This may be intentional — PR 4 added `NotFound.tsx` and PR 5a/6a
may have wired the Next build to also render it for unmatched routes (e.g., via a `not-found.tsx`
convention file added to the Next app directory). If intentional, `fixtures/next-baseline.json`'s
`notFound` key needs updating, and the "nav still works" assertion in the Next baseline block
needs reviewing against the new landing page. If unintentional (a Next routing regression),
the root cause must be fixed instead.

**Acceptance criterion**: Owner confirms whether unification is intentional. If yes, update
`fixtures/next-baseline.json` to match the new behavior and ensure the "nav still works" test
passes against the not-found view. If no, restore the old fall-through behavior.

---

### R4 — Tab-switch resets scroll on both builds (`scroll-policy.spec.ts`)

**Symptom**: "switching the Up Next tab leaves scroll and the history length unchanged" fails
on **both** the Next and Bun builds. `scrollTop` resets to 0 after `upNext.selectTab('queue')`
instead of staying at `before`.

**Hypothesis**: The UpNextPage tab-switching mechanism (or the `UpNextPage.selectTab` page
object helper) causes a re-mount of the scroll container — likely via a `key` prop on the
tab content panel that changes when the active tab changes, or a React state change that
unmounts/remounts the scroller. Because the scroller remounts, its `scrollTop` resets to 0.
This is a React rendering issue, not a router/scroll-policy issue (the scroll policy's
pathname-guard correctly no-ops on search-only changes, and `history.replaceState` in
`replaceSearchInHistory` doesn't fire a `popstate` event).

**Acceptance criterion**: Switching tabs in the Up Next panel from any tab to any other tab
leaves the scroll container's `scrollTop` within 2 px of its value before the switch, and
`history.length` is unchanged.

---

### R5 — Bun POP scroll-restore does not work (`scroll-policy.spec.ts`, Bun only, `test.fail()`)

**Symptom**: `ScrollPolicy.tsx`'s POP branch calls `restore()` with the saved positions for
`location.key`, but `backScrollTop` is always 0. Currently documented with `test.fail()` in
the Bun-only block.

**Hypothesis**: The `describe()` / `find()` path-based element selector does not survive
page remounts. React Router's `<Suspense key={route.path}>` unmounts and remounts the entire
page subtree on each navigation, so the tag:sibling-index path from `body` to the scroll
container changes between the time the scroll is recorded (on `/settings/features`) and the
time `restore()` tries to re-find the element (after navigating back). Adding an `id` or
`data-testid` to the scroller, or switching to an ID-based descriptor, would make `find()`
succeed after remount.

**Acceptance criterion**: On the Bun build, `page.goBack()` from `/history` to
`/settings/features` restores `scrollTop` to within 2 px of the value recorded before the
forward navigation. The `test.fail()` annotation is removed and the assertion passes green.

---

## Out of scope

- PR 7b's own Bun-only assertions (those are correct and should stay `test.skip(!isBunBuild())`).
- Accessibility improvements beyond restoring the pre-regression baseline.
- Any Next.js version upgrade.
- Scroll restore for the Bun build on paths not covered by `measureScrollPolicy`.

---

## Files likely to change

| File | Why |
|---|---|
| `tests/e2e/fixtures/next-baseline.json` | Update `notFound` (R3), potentially `skipLink.tabPresses` if R1 fix changes tab order |
| `web-app/src/lib/nav/ScrollPolicy.tsx` | Fix element descriptor for remount-safe lookup (R5) |
| `web-app/src/styles/theme-boot-script.ts` or `web-app/src/app/layout.tsx` | Fix font-class stripping (R2) |
| UpNextPage tab panel component (TBD path) | Fix re-mount causing scroll reset (R4) |
| `web-app/src/app/not-found.tsx` (new file, if R3 is intentional) | Wire NotFound into Next App Router |
| `tests/e2e/scroll-policy.spec.ts` | Remove `test.fail()` once R5 is fixed |

---

## Constraints

- Do not modify any source file that PR 7b currently asserts (it asserts the Bun-only behavior
  already in place; regressions are on the Next-baseline side).
- All changes must leave `make ci` green on the Next build before landing.
- PR 7b's own `test.skip(!isBunBuild())` guards must remain intact.
