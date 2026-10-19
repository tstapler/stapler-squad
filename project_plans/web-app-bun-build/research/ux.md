# UX Research: web-app-bun-build

Scope: what could regress for browser users and for developers when Next.js is replaced by Bun.build plus a client router. No intended UI change.
Labels: VERIFIED = file opened or command run in this worktree. INFERRED = reasoned, not run.

## 1. Route-change behaviour users rely on

| Behaviour | Today | Evidence | Risk after cutover |
|---|---|---|---|
| Document title per route | Next `export const metadata` in 25 files (root `Stapler Squad Sessions`; most routes `<Name> - Stapler Squad`) | VERIFIED `web-app/src/app/**/layout.tsx`, `page.tsx` (grep) | HIGH. Bun has no metadata API. A static single `index.html` would give every route the root title. Needs a head helper, or per-route HTML. |
| Titles already inconsistent | `/backlog`, `/backlog/board` are tolerated for axe `document-title` in the sweep, so they have no title today | VERIFIED `tests/e2e/a11y-route-sweep.spec.ts` KNOWN map. Pages that are client components cannot export metadata (comment in `settings/backlog-stages/page.tsx:17`, `pipeline-modes/page.tsx:16`) | Opportunity: a title hook fixes these. Do not claim parity for them; there is no baseline. |
| Titles on client navigation | Next updates `<title>` on soft nav | INFERRED (Next behaviour) | The new router must set `document.title` on every route change, not only on first load. |
| 404 | No `not-found.tsx`, `error.tsx` or `loading.tsx` in `src/app`. Go serves root `index.html` with status 200 for any unknown path, and the directory `index.html` for known dirs | VERIFIED `find src/app` (no matches); `server/middleware/static.go:12-60` | An unknown URL renders the sessions page with 200 today. A client router `*` route would change that to a visible not-found view. That is a UX improvement, but it is a behaviour change. Decide explicitly, and keep it out of the parity claim. |
| Trailing slash | `trailingSlash: true`; three specs tolerate `/x/` or `/x` | VERIFIED `next.config.ts`; `insights-findings-panel.spec.ts:28`, `insights-session-route.spec.ts:115`, `unfinished-work.spec.ts:155` | LOW to MEDIUM. Specs use optional-slash regexes, but `unfinished-work` asserts `/settings/unfinished/?$`. Both forms must resolve and canonicalise the same way. |
| Deep links and query state | 22 `useSearchParams` call sites. Several hooks bypass the router with `window.history.replaceState` because "router.replace hard-reloads the static export" | VERIFIED `useUpNextTab.ts:67`, `UnfinishedTab.tsx:55,142`, `SessionDetailView.tsx:502` (`#file=` hash) | MEDIUM. These hacks depend on Next syncing `useSearchParams` with `history.replaceState`. React Router and TanStack do not by default. The facade must subscribe to history changes, or these hooks go stale. Add a test. |
| Back/forward | `useTrayDismissal` pushes a `ssqTray` history entry and listens to `popstate` | VERIFIED `src/lib/hooks/useTrayDismissal.ts:51-59` | MEDIUM. The router owns `history.state`. It must not overwrite the `ssqTray` marker. Spec coverage is thin (see section 4). |
| Scroll restoration | No `scrollRestoration` or route-level `scrollTo` in app code | VERIFIED grep over `web-app/src` | LOW. Next's default is scroll-to-top on push. React Router needs `<ScrollRestoration/>` or an equivalent. Decide the policy. The app uses fixed-height panes (`100dvh`), so the effect is small. INFERRED. |
| Focus on route change | No app code moves focus on navigation | VERIFIED grep. Next 15 App Router focuses the new page's first focusable segment (INFERRED from docs, not run) | MEDIUM. Behaviour differs between routers. A deliberate policy is needed (section 3). |
| Loading and transition states | No `loading.tsx`. Pages use their own `role="status"` spinners | VERIFIED grep | LOW. `next/dynamic` call sites need `React.lazy` + `Suspense` fallbacks that match today's, or a chunk fetch shows a blank pane. |
| Link prefetch | `NavLink` wrapper disables prefetch by default | VERIFIED `src/lib/navigation/NavLink.tsx` | None. The replacement must not add prefetch. |

## 2. Mobile and viewport

- VERIFIED `layout.tsx:22-27`: `viewport` = `width=device-width, initial-scale=1, maximum-scale=5, viewport-fit=cover`. Next emits this as `<meta name="viewport">`. It must be reproduced verbatim in the generated HTML. Dropping `viewport-fit=cover` silently breaks every `env(safe-area-inset-*)` use (87 matches for `env(safe-area|100dvh|visualViewport` in `src`). Dropping `maximum-scale=5` would still allow pinch zoom, but the explicit value should be kept.
- VERIFIED `tests/e2e/android-url-bar.spec.ts` covers 100dvh overflow and BottomNav. `mobile-navigation.spec.ts` covers the card-tap detail pane, and part of it skips when no sessions exist. Neither asserts the viewport meta tag. GAP: add a one-line assertion on the meta content.
- Touch targets and mobile keyboard: no router-dependent logic found. No regression expected. INFERRED.
- The mobile app connects to `https://onyx.staplerhome.internal:8444` (memory note). It loads the same embedded assets, so URL shapes and `_next/` independence matter. VERIFIED no `_next/` string in tests/e2e. The mobile app was not inspected (open question carried from requirements).

## 3. Theme and FOUC

- VERIFIED `layout.tsx:34-49`. An inline `<head>` script embeds a theme map keyed by name (`matrix`, `cyberpunk77`, `wh40k`, `clean`, `light`, `dark`) to vanilla-extract hashed class names. It reads `localStorage['stapler-theme']`, falls back to `clean`, applies a `custom:<id>` theme from `stapler-theme-custom` (class plus CSS custom properties), strips known theme classes, then adds the chosen one. The same map defaults `<html className>` to `cleanTheme` at render.
- Requirement for Bun: the HTML generator must be a build step that runs after vanilla-extract class names are known and injects the identical script into `<head>`, synchronously and before first paint, for every generated HTML file. It must not be `defer`red or moved to a module script. A module or deferred script would produce a visible flash of the `clean` theme on dark themes.
- Font classes (`jetbrainsMono.variable` etc. from `next/font/local`) are on `<html>`. The script deliberately removes only theme classes so these survive (comment `layout.tsx:46`). Replacing `next/font` with `@font-face` must keep the CSS variable names (`--font-*`) and still not be removed by the script. INFERRED for the exact variable names; the file `fonts.ts` was not read.
- ThemeContext (`src/lib/contexts/ThemeContext.tsx`) re-applies at runtime and uses the same class table (VERIFIED lines 193-228). Two sources of truth for the class names already exist. The build must keep them identical.
- Existing coverage: `tests/e2e/theme-background.spec.ts` (VERIFIED) sets `stapler-theme` before load and checks `#main-content` is non-white for dark themes on `/history` and `/logs`. It does not cover `custom:` themes, the `clean`/`light` default, or the first-paint flash.
- GAP: add a spec that, for each of the 6 themes plus one custom theme, loads the page and asserts the `<html>` class set is correct at `DOMContentLoaded`. Better, record the computed background at the first `requestAnimationFrame`.

## 4. Accessibility (WCAG 2.2 AA relevance)

- Skip link: VERIFIED `layout.tsx:67-68` and `globals.css:424-441`. `<a href="#main-content" class="skip-link">` targets `<main id="main-content">`. Hash links interact with a client router: some routers intercept `#hash` clicks. The facade must let a same-page hash through, or the skip link breaks. Also, 10 pages render their own `<main id="main-content">` inside the layout `<main>`. That gives duplicate ids and nested landmarks today (VERIFIED `account/page.tsx:326`, `history/page.tsx:307`, `logs/page.tsx:98`, etc.). This is a pre-existing defect. Do not worsen it. Do not fix it inside this migration unless the sweep shows a change.
- `aria-current`: VERIFIED only two uses (`help/page.tsx:112`, `backlog/page.tsx:869`). The main nav `aria-current` is not set in app code that I found (grep over `src` listed no layout/nav component). Next's `usePathname`-based active state, if any, must be preserved by the facade. Zero e2e specs assert `aria-current` (VERIFIED count 0). GAP.
- Route announcements: no live region announces route changes (VERIFIED grep; the `role="status"` regions are page-local). Next 15 injects an invisible route announcer (`next-route-announcer`, INFERRED from Next behaviour, not verified in the built output). If true, the Bun build loses a screen-reader route announcement silently. Recommendation: add an explicit `aria-live="polite"` announcer that reads the new `document.title`, and move focus to `#main-content` (or the page `<h1>`) after navigation. This meets WCAG 2.2 2.4.3 (focus order) and 4.1.3 (status messages).
- Axe CI (VERIFIED):
  - `.github/workflows/ux-analysis.yml` runs `accessibility.spec.ts` against `http://localhost:8543` (the Go-embedded build). Failures block the merge. Its header says "WCAG 2.1 AA" and the sweep tags are `wcag2a, wcag2aa, wcag21a, wcag21aa`. WCAG 2.2 tags (`wcag22aa`) are not run. Adding them would test target-size (2.5.8). Expect new findings. Treat as separate work, not a cutover gate.
  - `a11y-route-sweep.spec.ts` covers 29 routes with a KNOWN baseline of tolerated rules (color-contrast, document-title, etc.). The rule is "only remove entries, never add". Cutover must not need new entries. Note `/backlog` and `/backlog/board` already tolerate `document-title`.
  - Dynamic routes (`/insights/session-detail`, `/sessions/summary`, `/fanout`, `/analytics/escape`, `/login`, `/help`) are not in the sweep route list (VERIFIED against the `ROUTES` array vs 42 `page.tsx` files). GAP.
- Lighthouse CI (VERIFIED): two configs exist. `web-app/lighthouserc.json` uses `npx serve@14 out -l 3001` against `/` only and warns below perf 0.7, a11y 0.8. `tests/e2e/lighthouse.config.js` (used by the workflow) hits `localhost:8543/` and `/review-queue`, 1 run, warn only. `serve@14 out` serves with its own clean-URL logic. If the new output layout drops `dir/index.html`, this config diverges from Go's behaviour. Pick one config to keep.
- Axe and Lighthouse only run on PRs touching `web-app/src/**`. A PR that changes only the build files (`web-app/package.json`, new build script) will not trigger `ux-analysis.yml`. VERIFIED from the `paths:` filter. The cutover PR must widen the filter or run both manually.

## 5. Developer experience

Current loop (VERIFIED `package.json`, `Makefile`): `next dev --port ${PORT:-3001}`; `make web-build` (Next, development mode 54-88 s per requirements baseline).

| Concern | Next today | Bun target | Risk or action |
|---|---|---|---|
| Startup and reload | Cold compile on first page load is slow enough that the dev-mode Playwright config raises timeouts to 60 s (VERIFIED `playwright.dev-mode.config.ts`) | Spike: `.css.ts` edit to reload in 84 ms (requirements, not re-run here) | Only one component was tested (stated in the requirements). Validate full-app HMR before cutover. |
| Fast Refresh | Built in | `Bun.serve` HMR with React Fast Refresh: INFERRED from public docs; not re-verified here | Test with stateful components (the terminal, the omnibar, the session list). If state resets on every edit, the loop is worse than Next. |
| Error overlay | Next overlay with source-mapped frames, component stack | Bun dev server shows a browser error overlay (INFERRED, quality unverified) | Compare on a deliberate render error in a vanilla-extract file. Keep `ErrorBoundary` (VERIFIED in layout) unchanged. |
| Source maps | `productionBrowserSourceMaps: true` (VERIFIED) | Must emit `.map` for production if stack traces from `WebVitalsReporter`/OTel are to stay readable | Add as an explicit requirement. Size budget (`size-limit`) should exclude maps. |
| Backend proxy | `next dev` has no `/api` rewrite in `next.config.ts` (VERIFIED no `rewrites`); the dev stack runs a real Go backend on a separate port | Same | Open question in requirements is answered: no proxy exists today. Parity needs no proxy. CORS or same-origin config for ConnectRPC at the dev port must be checked. INFERRED. |
| Dev-mode e2e harness | `scripts/dev-stack/launch.ts` hard-codes `node_modules/.bin/next` (VERIFIED line 215) and waits on its output | Must launch the Bun dev server | Missing from the requirements' scope. Add it. Tests: `scripts/dev-stack/launch.test.ts`, `tests/e2e/global-setup.dev-mode.test.ts`. |
| Storybook | `@storybook/nextjs`; `.storybook/css-ts-mock-loader.js` exists (VERIFIED) | Next-free framework | Already in the requirements. UX relevance: stories are the visual regression surface for `NavLink`/`AppLink`. |

Comparable dev loops (INFERRED from public knowledge, not benchmarked here): Vite offers sub-100 ms HMR and mature overlays and source maps; Next dev is slower on cold compile but offers the best overlay and route integration; Bun's dev server is the youngest. The risk to the owner is not speed, which the spike supports, but the long tail of HMR edge cases and error-overlay quality. A one-week-style evaluation is not appropriate here; use a checklist run: edit a `.css.ts`, edit a hook, edit a context provider, introduce a type error, introduce a runtime error, and confirm each is visible and recoverable without a manual reload.

## 6. Jobs to be done

- Owner: "When I change a component, I see it in under a second without losing state" (fast iteration). "When I run `make install-service`, the build finishes in seconds and gives the same bytes tomorrow" (reproducible). "When I read the repo, I see fewer tools" (less tooling: drops webpack, `eslint-config-next`, `@storybook/nextjs`).
- Agent sessions in worktrees: each worktree currently pays a 54-88 s build and holds its own `node_modules` (a shared store was added in PRs #1093/#1095, per git log). Jobs: "build in a fresh worktree without a warm cache", "run a dev server on an isolated port without colliding with `:8543` or other worktrees" (ports 62871+ block). Constraint: `NEXTJS_SHARED_CACHE_DIR` handling in `next.config.ts` becomes unnecessary. Bun needs no shared cache if builds are ~3 s. Any new dev server must read `PORT` from env (the same convention as today).
- End users: "My bookmark, my back button and my screen reader keep working."

## 7. Existing Playwright coverage and gaps

VERIFIED by grep over `tests/e2e/*.ts` (excluding `node_modules`):

| Concern | Specs | Coverage |
|---|---|---|
| Title | `toHaveTitle` in 7 spec files / 19 call sites; all but `insights.spec.ts:18` (`/Insights/i`) match only the generic `/Stapler Squad/` | Weak. Cannot detect per-route title loss, because the root title also matches. |
| Nav click to URL | `nav-navigation.spec.ts` (Up Next, Review Queue, with and without `?session=`) | Good for 2 links. |
| Back button | `up-next-tabs.spec.ts:104`, `notification-tray-variants.spec.ts:260` (`goBack`); `goForward`: 0 | Thin. |
| Focus | `toBeFocused` in 73 files, but all are widget-level; none assert focus after route change | Gap. |
| Skip link | 0 specs target `.skip-link` (main-content hits are `#main-content` as a layout anchor in notification and theme specs) | Gap. |
| `aria-current` | 0 | Gap. |
| Scroll position | `scrollY` only in `backlog-live-updates.spec.ts` (live updates, not routing) | Gap. |
| Deep links | `backlog-deep-link-*.spec.ts` (4), `insights-session-route.spec.ts`, `backlog-item-id-deep-link.spec.ts` | Good for backlog and insights. |
| Axe | `accessibility.spec.ts`, `a11y-route-sweep.spec.ts` (29 of 42 routes) | Good baseline. |
| Theme | `theme-background.spec.ts` | Partial (section 3). |
| Mobile | `mobile-navigation.spec.ts`, `android-url-bar.spec.ts`, `notifications-responsive.spec.ts` | Reasonable. No viewport-meta assertion. |

New specs to add BEFORE cutover (run them against the Next build first to capture the baseline, then against Bun):
1. `route-titles.spec.ts`: for every static route, `toHaveTitle` equal to the exact string captured from the Next build, on both hard load and client-side navigation. Also fix or explicitly record `/backlog` titles.
2. `route-focus-and-announce.spec.ts`: after a link click, assert focus location (policy decided in planning) and that an `aria-live` region contains the new title. Skip link: Tab once, Enter, and `#main-content` receives focus and the URL hash/state behaves.
3. `history-behaviour.spec.ts`: push A to B to C, then back twice and forward once, with query params; the `ssqTray` entry does not break the stack; `?tab=` hooks update on `replaceState`.
4. `deep-link-trailing-slash.spec.ts`: each of `/x`, `/x/`, `/x?q=1`, `/x/?q=1` and a nested `settings/*` path loads the right view.
5. `theme-fouc.spec.ts`: 6 themes plus a `custom:` theme, class on `<html>` before first paint, and no `clean` flash.
6. `viewport-meta.spec.ts`: the meta content string, plus a safe-area padding check at 390x844.
7. `not-found.spec.ts`: record the current status and body for an unknown path, then assert the decided behaviour.
8. Extend the `a11y-route-sweep` `ROUTES` list with the 13 routes not covered.
9. `aria-current`: main-nav active link, in the sweep or its own spec.

## 8. Recommendations (for planning)

1. Make a head/title helper plus an announcer and focus policy part of the router facade, not an afterthought. They are the three route-change UX behaviours Next provided implicitly.
2. Keep the FOUC script synchronous in `<head>`, generated at build time from the final vanilla-extract class names. Test it per theme.
3. Preserve `trailingSlash` URL shapes for the Go fallback (`dir/index.html` or an equivalent rewrite). Do not rely on `serve@14` in Lighthouse, which may differ from Go.
4. Add the `scripts/dev-stack/launch.ts` port to scope.
5. Widen the `ux-analysis.yml` `paths:` filter so a build-only PR runs Axe.
6. Decide the 404 behaviour explicitly. Today it is the root view with 200.

## 9. Gaps in this research

- No pages were run or screenshot. All behaviour claims about Next focus, route announcer and Bun HMR are INFERRED from documentation knowledge.
- `fonts.ts`, `CockpitShell`, nav components and the mobile app were not opened.
- Dev server timings for Bun were not re-measured; the 84 ms figure is the requirements' spike claim.
