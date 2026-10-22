# UX Preservation Design: web-app-bun-build

Date: 2026-10-19. Phase 3 UX design artifact.
Inputs: `requirements.md`, `research/ux.md`, `research/features.md`, `implementation/plan.md` (Epic 7 = Phase 7, plus Stories 1.2-1.6, 2.6-2.7, 4.2-4.6, 5.3-5.6, 5.8, 6.5, 8.1).

## 0. Principle

This migration intends no visual change. The design goal is a behavioural diff of zero for everything below, except two decisions the plan already records as deliberate and excluded from the parity claim:

- D1: an unknown path shows a not-found view (today it renders the sessions page).
- D5 and the announcer: route changes move focus and are announced politely. Next did the announcing implicitly (an `aria-live="assertive"` region, INFERRED in `research/features.md` section 4); the plan replaces it with `role=status` / `aria-live=polite`. This is a deliberate change of politeness level.

Everything else is baselined on the Next build first (Phase 7), then must pass unchanged on the Bun build. Evidence labels follow the research files: VERIFIED = opened or run; INFERRED = reasoned.

## 1. Surfaces

| ID | Surface | Type | Owner stories |
|----|---------|------|---------------|
| S1 | Document title (hard load and client navigation) | Browser chrome, tab strip | 1.2, 4.3, 4.4, 7.1 |
| S2 | Focus placement after navigation | Keyboard / AT | 4.5, 7.2 |
| S3 | Route announcer (live region) | Screen reader | 4.5, 7.2 |
| S4 | Skip link | Keyboard | 4.2, 4.5, 7.2 |
| S5 | Main nav active state and `aria-current` | Desktop header, drawer, mobile bottom nav | 1.3, 2.4, 7.2 |
| S6 | Back / forward and the history stack | Browser | 2.6, 4.2, 4.6, 7.3 |
| S7 | Deep links, query-param state, redirects, feature-flag guards | URL | 1.3, 1.4, 4.2, 4.3, 7.4 |
| S8 | Unknown-path / not-found view | Page | 4.3, 7.7 (view content: gap G3) |
| S9 | Theme first paint (6 themes plus custom) | Page load | 5.3, 5.4, 7.5 |
| S10 | Viewport, safe areas, mobile and desktop layout | Mobile | 5.3, 7.6 |
| S11 | Route loading, lazy-chunk failure, stale tab | Loading / error states | 2.7, 1.6, 4.4 (route-level pending: gap G1) |
| S12 | Fonts (`@font-face`, CSS variables) | Typography | 5.3 |
| S13 | Scroll policy | Page | 4.6 (no e2e: gap G8) |
| S14 | Login and auth hard navigation (asset prefix) | Auth flow | 1.5, 1.6, 6.8, 8.1 |
| S15 | Dev server: HMR, error overlay (developer) | Dev surface | 0.3, 5.6, 5.8 |
| S16 | Build failure output (developer, CI, agents) | CLI output | 5.5, 5.9 (message content: gap G7) |

Interactive surfaces (S1-S11, S14) get the full treatment. S12, S13 are condensed. S15 and S16 are developer surfaces with a representative sample and bullet criteria.

---

## 2. Interactive surfaces

### S1. Document title

Layout (browser tab):

```
[ favicon ] History - Stapler Squad   x      <- tab text = RouteMeta[path].title
```

Flow:

1. Hard load of `/settings/jules`: static shell `<title>Stapler Squad Sessions</title>` is present before JS (axe `document-title` passes on first paint). After the route chunk mounts, title becomes `Jules - Settings - Stapler Squad`.
2. Client click History -> `document.title` = `History - Stapler Squad` in the same commit as the route render.
3. Route with no own title (`/backlog`, `/files`, `/help`, `/resolve`, `/changelog`, `/`): title is the root string `Stapler Squad Sessions`. `/settings/*` without own title: `Settings - Stapler Squad`. This is today's behaviour (VERIFIED `research/features.md` section 1), including `/backlog` and `/backlog/board` having no distinct title. Do not claim to fix them.
4. Search-only change (`?tab=b`): title unchanged.

Error handling:

| Failure | User sees | Exit |
|---------|-----------|------|
| Title lookup misses (route not in `RouteMeta`) | Root title | None needed; `routes.test` asserts all 42 keys |
| Route chunk fails to load | Previous title stays, then ErrorBoundary (S11) | Retry button |
| Unknown path | `Page not found - Stapler Squad` (see G3) | Nav / home link (S8) |

### S2 and S3. Focus and route announcement

```
nav link clicked --> route renders --> focus -> main#main-content (tabindex=-1)
                                   --> <div role=status aria-live=polite class=sr-only>
                                         "History - Stapler Squad"</div>
```

Flow: user activates a nav link (mouse or keyboard) -> pathname changes -> after commit, focus moves to the layout `main#main-content` and the live region text is set to the new `document.title`. No move and no announcement on first load, on search-only or hash-only changes, or when the pathname is unchanged.

Edge cases:

- Duplicate ids: 10 pages render their own `<main id="main-content">` inside the layout `<main>` (VERIFIED, pre-existing). `getElementById` returns the first, the layout main. The focus hook must target the layout element by ref, not by a fresh query (decision: `RootShell` receives a `mainRef` from the router entry and attaches it to the layout `<main>`; Story 4.5). Do not fix the duplication here.
- Identical titles on consecutive navigations (`/backlog` -> `/backlog/board`, both root title): setting the same text does not re-announce. The announcer clears then sets (next frame) so AT re-reads it.
- Focus must not steal from a field the page just autofocused (omnibar, search): if `document.activeElement` is inside `#main-content` and is not the clicked link, leave it.
- Redirect routes (`/config`, `/settings/defaults`, `/sessions/new`): announce once, for the final path, not per hop.
- Browser Back/Forward (popstate) is a pathname change: same focus and announcement behaviour as a link click.
- Politeness: `role=status`, never `role=alert`. Next's own announcer politeness is **INFERRED** (assumed assertive); plan Task 0.5d reads the built Next output and records the real value, and a screen-reader item (NVDA or VoiceOver) in Story 8.1 checks what is spoken (UX-62).
- Focus ring (decision D8, UX-58): the layout `main#main-content[tabindex=-1]` has `outline: none`, scoped to that one element, because Next focuses nothing on navigation (no ring today), the element is not interactive or in the Tab order, and the change is conveyed by the title and announcer. Controls inside the page keep their rings.

Error handling: if `#main-content` is missing (guard still returning `null`, S7), the hook retries on the next commit that renders it, then gives up silently; the page is still reachable by Tab. No error state is shown to the user.

### S4. Skip link

```
[Skip to main content]   <- first Tab stop, visible on focus only (globals.css .skip-link)
Header ...
main#main-content
```

Flow: Tab (first stop) -> link appears -> Enter -> focus lands in `#main-content`, URL path and search are unchanged, no router navigation, no announcement. A `#main-content` hash in the address bar is acceptable only if it is what happens today (baseline in 7.2).

Error handling: if the router intercepts the hash click and navigates, the page would re-render and lose focus. Facade `Link` must leave same-page hash links to the browser (4.2 covers this). Exit: Escape or Tab continues normally.

### S5. Active nav state and `aria-current`

Today only two `aria-current` uses exist in app code (`help/page.tsx:112`, `backlog/page.tsx:869`); main nav `aria-current` is not found (VERIFIED grep, `research/ux.md` section 4). Preserve visual active state; do not invent `aria-current` and call it parity.

Flow: on `/history` the History item shows the active style in Header, DrawerNav and BottomNav. On `/` only the Home item is active (strict equality `=== routes.home`). On `/review-queue/` (trailing slash) the result equals `/review-queue`: `usePathname` must be normalised (Story 1.3 pins Next's behaviour; 4.2 normalises).

Error handling: unknown path (S8): no nav item is active; nav is still operable.

### S6. Back, forward and the history stack

Flow (baseline for 7.3): `/` -> `/history?sessionId=a` -> `/logs`; Back x2 -> `/`; Forward x1 -> `/history?sessionId=a`. Search-only updates via `ReplaceSearch` replace the current entry (no new entry). The mobile notification tray adds an entry with `ssqTray:true`; Back closes the tray and does not change the route; the router's popstate handler must not drop or re-push that marker. `SessionDetailView`'s `#file=` update must preserve `history.state` (today it passes `null`, which would wipe the router's `idx`/`key`; the facade `replaceHash` preserves foreign state).

Error handling and exit: the user can always leave via Back or any nav link. If router state is lost (foreign `history.state` replaced), React Router falls back to a full location read; the spec in 7.3 detects this as a wrong URL after Back.

### S7. Deep links, query params, redirects, guards

Contract (VERIFIED `research/features.md` section 2): server-built links have no trailing slash before `?`: `/?session=ID&tab=terminal&notification=N`, `/?session=ID`, `/backlog?item=ID`. Hard loads of `/rules`, `/rules/`, `/rules?prefill=x`, `/rules/?prefill=x`, `/settings/jules/` all show the right page, and `page.url()` for `/rules` stays `${BASE_URL}/rules`.

Redirects:

```
/config?x=1        -> /settings?tab=config-files   (query dropped, replace)
/settings/defaults -> /settings                    (replace)
/sessions/new      -> /?new=true | /?duplicate=<id> (page effect, replace)
```

Guards (backlog, settings/backlog-sources, changelog): while flags load the main pane is blank; if the flag is off, `replace("/")`.

Error handling:

| Case | User sees | Exit |
|------|-----------|------|
| Flag off | Lands on `/` | Normal nav |
| Flags never finish loading | Blank main pane (as today) | Nav stays usable; reload |
| Deep link to a missing session / item id | Existing in-page not-found state (unchanged code) | Nav |
| `/resolve?url=ssq://...` | Existing page, then `window.location.href` to another host | Cancel with Back before redirect |
| Push-click handoff string `/?session=abc&window=w1` | Opens session pane with both params | n/a |

`?window=<id>` must survive `replace` calls that rebuild from `searchParams.toString()` (multi-window identity).

### S8. Unknown-path / not-found view (D1, deliberate change)

```
+-------------------------------------------------+
| [Header / nav, fully operable]                  |
|                                                 |
|   <h1>Page not found</h1>                       |
|   The page "/nonexistent" does not exist.       |
|   [ Go to Sessions ]  [ Go back ]               |
+-------------------------------------------------+
```

Flow: hard load or client navigation to an unknown path -> view renders in the shell, URL unchanged (no redirect), Go status stays 200 (no Go change). Title `Page not found - Stapler Squad`; announced and focused like any route (S2, S3). "Go to Sessions" navigates to `/`; "Go back" calls history back (hidden if `history.length` is 1; then only the home link shows).

Error handling: none beyond exit paths above; the view has two exits plus the global nav. The view must not set any redirect (breaks `waitForURL` expectations). Escape-hatch for test paths: `/test/*` and `/debug/*` stay routable (D2).

### S9. Theme first paint

Flow at load: synchronous inline `<script>` in `<head>` (not deferred, not `type=module`) reads `localStorage['stapler-theme']` -> removes known theme classes -> adds the chosen one; font classes/variables on `<html>` are untouched. The theme CSS must be a render-blocking `<link rel=stylesheet>` in `<head>` so the themed background paints before any JS runs. Verify this for the Bun build: if the CSS were injected by JS, dark themes would flash white.

Matrix to check: `matrix`, `cyberpunk77`, `wh40k`, `clean`, `light`, `dark`, plus `custom:<id>` (class and CSS variables from `stapler-theme-custom`). No stored theme -> `clean`.

Error handling:

| Failure | Result | Exit |
|---------|--------|------|
| `localStorage` throws (private window) | Boot script try/catch, falls back to `clean`; page usable | ThemeContext re-applies at runtime |
| Stale class in storage after a deploy changed hashes | Degrades to `clean` | User picks theme in settings |
| Custom theme row missing | `clean` | Settings |
| Dev server (S15) | Brief flash (module-script boot, documented in `docs/how-to/web-dev-server.md`) | Accepted; dev only |

### S10. Viewport, safe areas, mobile and desktop

Shell must emit exactly: `<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=5, viewport-fit=cover">` and `<html lang="en">`. Without `viewport-fit=cover`, all `env(safe-area-inset-*)` uses (87 matches, VERIFIED) silently resolve to 0.

Flows to hold identical at 390x844 and 1280x800: BottomNav visible on mobile and clear of the home indicator; session card tap opens the detail pane; `100dvh` panes do not overflow with the URL bar (android-url-bar spec); pinch zoom still allowed (maximum-scale=5). Touch targets and mobile keyboard behaviour are not router-dependent; no change expected (INFERRED). iOS Safari is the engine most likely to differ (edge-swipe Back and the `ssqTray` history marker; `100dvh` with the virtual keyboard): a Playwright WebKit project checks both, report-only when the browser or keyboard emulation is unavailable, plus a real-iPhone item in Story 8.1 (UX-61).

Error handling: none (static). Failure shows up as the e2e assertion in 7.6.

### S11. Route loading, lazy-chunk failure, stale tab

Today there is no `loading.tsx` / `error.tsx` (VERIFIED). Pages use their own `role="status"` spinners; `next/dynamic` sites show `loading` skeletons.

```
click link --> [old page stays or shell + skeleton] --> new page
                  \--> chunk fails --> reload once (flag) --> fails again --> ErrorBoundary (Retry button)
```

Flow: navigating to a lazy route: header, nav and providers (terminal pool, auth) persist (Story 4.4); the main pane shows a skeleton or keeps the old page until the chunk arrives (policy: see G1). The `RoutePending` skeleton does not animate under `prefers-reduced-motion: reduce` and contains no live region, so the only announcement is the title after the route commits (UX-60). A 404 on a hashed chunk (Go returns 404 under `/_next/` after Story 1.6) triggers one `location.reload()` guarded by a `sessionStorage` flag; a second failure renders the root `ErrorBoundary` with its Retry button (a full `location.reload()` when the failure is a chunk-load error, because a rejected dynamic-import promise is memoised and re-rendering would not re-request it; otherwise the boundary's existing `onRetry`). The "Reload" button below belongs to the new-version banner, a different control.

Error handling:

| Failure | User sees | Exit |
|---------|-----------|------|
| Stale tab, chunk 404 | One automatic reload, then the new build | n/a |
| Reload fails again (offline) | ErrorBoundary with Retry | Retry button; nav links in the shell remain if the boundary is inside the shell (confirm; G2) |
| `sessionStorage` blocked | Flag cannot be set; must not loop | Treat as "already reloaded" and show ErrorBoundary |
| Chunk 404 while a field holds text (omnibar text, form draft) | No automatic reload; a non-blocking banner "A new version is available. Reload when ready." | Reload button on the banner, when ready (Story 2.7). Dirty tracking uses a document-level `input` listener, not `value !== defaultValue`, which is always false for React controlled inputs (INFERRED; the Jest test with a real controlled input in Story 2.7 is the proof) |

### S14. Login and auth hard navigation

Flow: the ConnectRPC interceptor does `window.location.href = '/login'` on `Unauthenticated`. With remote access enabled and no session, every asset on the login page (JS, CSS, fonts under `/_next/static/`) is served unauthenticated; otherwise the login page is blank and nobody can sign in (highest-severity item in `research/features.md` section 3). `/login?setup_token=` and WebAuthn must work.

Error handling: if assets redirect to HTML the page is dead with no recovery. Mitigation is test-only: Go `isExempt` table (Story 1.5, 6.8) and the unauthenticated asset fetch in 8.1.

---

## 3. Condensed surfaces

### S12. Fonts

```css
@font-face { font-family: "JetBrains Mono"; src: url(/_next/static/media/...woff2); font-display: swap; }
:root { --font-jetbrains-mono: ...; --font-rajdhani: ...; --font-cinzel: ...; --font-inter: ...; }
```

- The four variables exist at `:root` and are not removed by the theme script.
- Text is never invisible while fonts load (`font-display: swap`) and each family has a system fallback.
- Next preloads 8 font files; the plan preloads only the primary UI font (5.3). Small CLS risk accepted, INFERRED.
- Fonts load from `/_next/static/media/` (auth-exempt prefix).

### S13. Scroll policy

- Forward navigation to a new pathname: scroll container goes to top. Back: position restored. Search-only `replace(url, {scroll:false})`: position unchanged.
- The app is fixed-height (`100dvh`) panes, so the scrolling element is the pane, not the window; identify it in S4's spike record (Story 4.6 depends on it).

### S15. Dev server (developer surface)

Representative sample:

```
$ PORT=62875 bun web-app/build/dev.ts
dev server http://localhost:62875  (HMR on)
[error] src/components/Foo.css.ts:12  Unexpected token   <- also shown as overlay in browser
```

- Binds `localhost` only (CORS accepts only `localhost`/`127.0.0.1` ports; `STAPLER_SQUAD_EXTRA_ORIGINS`).
- A `.css.ts` or React edit updates without a full reload; stateful components (terminal, omnibar, session list) keep state on a comment-only edit.
- A type or runtime error shows an overlay in the browser that clears when fixed, with no manual reload; the app's `ErrorBoundary` stays unchanged for render errors.
- A deep link such as `/backlog?item=x` serves the shell. `WEB_DEV_HMR=0` turns HMR off but keeps live reload.
- If the dev process crashes, `scripts/dev-stack` restarts it (5.8). Port in use: the process exits non-zero with the port in the message (G7).

### S16. Build failure output (developer / CI / agent surface)

Representative sample:

```
$ bun web-app/build/build.ts
[build] css-ts  src/components/Foo.css.ts: invalid CSS near "@keyframes" (line 12)
[build] FAILED at stage: bundle (2.1 s). web-app/out left unchanged.
exit 1
```

- Exits non-zero on bundle, CSS, verify or budget failure.
- The message names the failing file and stage; budget failures name the budget, bytes and limit (5.9 states this).
- Previous `web-app/out/` stays intact and `.out-tmp-*` is removed.
- Warnings from Bun's CSS parser are errors (the `@keyframes` class of bug).
- Unattended agent sessions can read the failing path from stderr without opening logs.

---

## 4. UX acceptance criteria

Format: Given / When / Then; owner = plan story. "Gap" = not covered by the current plan.

### Titles (S1)

- UX-01. Given any of the 42 static routes, When hard loaded, Then `document.title` equals `RouteMeta[path].title` captured from the Next build. Owner: 1.2, 7.1.
- UX-02. Given any route reached by clicking a nav link, When the route renders, Then `document.title` equals the same string in the same commit (no stale previous title). Owner: 4.4, 7.1.
- UX-03. Given `/backlog` and `/backlog/board`, Then title is the root string (parity with today; not claimed as a fix). Owner: 7.1.
- UX-04. Given a hard load before JS runs, Then the shell title is non-empty (axe `document-title` passes). Owner: 5.3.
- UX-05. Given a search-only change, Then the title does not change. Owner: 4.4.

### Focus and announcer (S2, S3, S4)

- UX-06. Given focus on a nav link, When the pathname changes, Then `#main-content` (tabindex -1, the layout element) receives focus. Owner: 4.5, 7.2.
- UX-07. Given only `?tab=` changes, a hash change, or first page load, Then focus does not move and nothing is announced. Owner: 4.5.
- UX-08. Given a navigation to `/history`, Then one `role=status` `aria-live=polite` region contains `History - Stapler Squad` and it is not `role=alert`. Owner: 4.5, 7.2.
- UX-09. Given two consecutive navigations with identical titles (`/backlog` then `/backlog/board`), Then the announcer text is cleared and re-set so AT re-reads it. Owner: none (G4).
- UX-10. Given a field autofocused by the new page, When navigation completes, Then focus is not stolen from it. Owner: none (G4).
- UX-11. Given a page that renders its own nested `main#main-content` (10 pages), When navigating to it, Then focus goes to the layout main and no duplicate-id regression beyond today's. Owner: none (G5).
- UX-12. Given browser Back or Forward changes the pathname, Then UX-06 and UX-08 apply. Owner: 4.5, 7.2b (focus and announcement both asserted on popstate; G4).
- UX-13. Given a redirect route (`/config`), Then one announcement for the final path. Owner: none (G4).
- UX-14. Given the first Tab on any page, Then the skip link shows; Enter focuses `#main-content` with no path change and no router navigation. Owner: 4.2, 4.5, 7.2.

### Nav (S5)

- UX-15. Given `/review-queue` and `/review-queue/`, Then the active-nav outcome in Header, DrawerNav, BottomNav and CockpitShell is identical. Owner: 1.3, 4.2.
- UX-16. Given `/history`, Then the History item has the active style; `aria-current` is asserted only if it exists on the Next baseline; if absent, a follow-up issue exists. Owner: 7.2.
- UX-17. Given an unknown path, Then no nav item is active and all items are operable. Owner: 4.3, 7.7b (asserts no active nav item or `aria-current` on the not-found view; G3).

### History and deep links (S6, S7)

- UX-18. Given `/` -> `/history?sessionId=a` -> `/logs`, When Back x2 then Forward x1, Then URLs are `/` then `/history?sessionId=a`. Owner: 7.3.
- UX-19. Given the mobile tray open, When Back is pressed, Then the tray closes and the route and URL are unchanged. Owner: 4.6, 7.3.
- UX-20. Given `/unfinished?tab=a`, When tab `b` is selected, Then URL is `?tab=b`, no new history entry, active tab is `b`. Owner: 2.6, 7.3.
- UX-21. Given `SessionDetailView` selecting `a/b.go`, Then hash is `#file=a%2Fb.go` and `history.state` keeps its router keys. Owner: 1.4, 2.6.
- UX-22. Given `/rules`, `/rules/`, `/rules?prefill=x`, `/rules/?prefill=x`, `/settings/jules/`, Then each loads the right page and `page.url()` for `/rules` is `${BASE_URL}/rules`. Owner: 7.4.
- UX-23. Given server links `/?session=ID&tab=terminal&notification=N` and `/backlog?item=ID`, When opened cold, Then the session pane / backlog item opens. Owner: 7.4.
- UX-24. Given the push-click string `/?session=abc&window=w1`, Then both params survive. Owner: 7.4.
- UX-25. Given `/config?x=1`, Then location is `/settings?tab=config-files` (replace, query dropped); `/settings/defaults` -> `/settings`. Owner: 4.3.
- UX-26. Given `flags.backlog === false`, When `/backlog/board` loads, Then it renders blank while loading, then replaces to `/` once. Owner: 1.4, 4.3.
- UX-27. Given a `replace` call on a URL carrying `?window=<id>`, Then `window` is preserved. Owner: 7.10 (full e2e run only; no dedicated AC; G9).

### Not-found (S8)

- UX-28. Given `/nonexistent`, When loaded, Then the not-found view shows, URL unchanged, status 200, no redirect. Owner: 4.3, 7.7.
- UX-29. Given the not-found view, Then it has an `h1`, a link to `/` and a Back control, the title `Page not found - Stapler Squad`, and it is announced and focused like any route. Owner: none (G3).

### Theme, viewport, fonts (S9, S10, S12)

- UX-30. Given `stapler-theme` is each of the 6 values, When the page loads, Then at `DOMContentLoaded` `<html>` has that class and, for dark themes, the first-frame background is not the clean background. Owner: 5.4, 7.5.
- UX-31. Given `custom:<id>`, Then its class and CSS variables apply before first paint; given a stale class, Then `clean` and the page is usable. Owner: 5.4, 7.5.
- UX-32. Given the boot script, Then it is inline, synchronous, in `<head>`, and does not remove font classes or `--font-*` variables. Owner: 5.4, 7.5.
- UX-33. Given the built HTML, Then the theme CSS is a blocking `<link rel=stylesheet>` in `<head>`, not injected by JS, so no white flash on dark themes. Owner: 7.5 (rAF background check) partially; explicit link assertion: G10.
- UX-34. Given `localStorage` throws, Then the boot script falls back to `clean` without an exception. Owner: none (G10).
- UX-35. Given any route, Then the viewport meta equals `width=device-width, initial-scale=1, maximum-scale=5, viewport-fit=cover` and `<html lang="en">`. Owner: 5.3, 7.6.
- UX-36. Given 390x844, Then bottom-nav bottom padding resolves `env(safe-area-inset-bottom)` as recorded on Next. Owner: 7.6.
- UX-37. Given 390x844 and 1280x800 on 5 routes, Then ParityDiff reports no unexplained computed-style difference. Owner: 7.9.
- UX-38. Given fonts, Then the four `--font-*` variables are defined, `font-display: swap` is set, system fallbacks exist, and woff2 URLs are under `/_next/static/media/`. Owner: 5.3.

### Loading, errors, auth (S11, S14)

- UX-39. Given a lazy chunk 404, When the component first mounts, Then one reload occurs; on the second failure the `ErrorBoundary` shows with a Retry button. Owner: 2.7, 4.4.
- UX-40. Given `sessionStorage` throws, Then no reload loop occurs (ErrorBoundary shown). Owner: none (G6).
- UX-41. Given a navigation to a lazy route, Then header, nav and providers persist and the main pane shows a skeleton or the old page, never a blank shell. Owner: none (G1).
- UX-42. Given `InsightsDashboard` x4, `SessionDetail`, `SessionDetailView` dynamic sites (6 `dynamic()` call sites in 3 files; `test/terminal-stress/page.tsx` only carries an unused `next/dynamic` import, removed rather than migrated), Then each shows the same skeleton as today, then the component. Owner: 2.7.
- UX-43. Given remote auth on and no session, When `/login` and every asset URL in its HTML are fetched, Then all return 200 with JS/CSS. Owner: 1.5, 6.8, 8.1.
- UX-44. Given an `Unauthenticated` RPC error, Then the hard navigation to `/login` loads. Owner: 8.1 (gate run), no dedicated spec (G9).

### Scroll (S13)

- UX-45. Given forward navigation, Then the scroll container is at top; given Back, position restored; given search-only `replace`, unchanged. Owner: 4.6 (jest only; e2e gap G8).

### Accessibility checks (cross-cutting)

- UX-46. Given the 13 routes missing from `a11y-route-sweep`, When added, Then the sweep passes with no new `KNOWN` entries. Owner: 7.8.
- UX-47. Given a PR touching only `web-app/build/**`, Then Axe and Lighthouse still run (`ux-analysis.yml` paths widened). Owner: 6.5.
- UX-48. Given Lighthouse, Then `/`, `/review-queue` and `/settings/jules` are audited against the Go-served build, not `serve@14`. Owner: 6.5.
- UX-49. Given the new announcer, Then axe finds no new violation from it (the five specs plus one page object excluding `#__next-route-announcer__` stay harmless; the exclusion is removed with Next in PR 9). Owner: 7.2, 7.8 (exclusion cleanup is optional and unowned).
- UX-50. Given keyboard-only use, Then nav, skip link, tray, not-found view and ErrorBoundary Retry are reachable and operable; text contrast is unchanged (axe `color-contrast` baseline in `KNOWN`, no additions). Owner: 7.8 (Axe cannot prove operability: the manual keyboard checklist in Story 7.8, run in Story 8.1, does).

### Developer experience (S15, S16)

- UX-51. Given the dev server on `PORT=62875`, Then it binds `localhost` only and `GET /backlog?item=x` returns the shell. Owner: 5.6.
- UX-52. Given a `.css.ts` edit, Then styles update with no full reload; given a comment-only edit to the terminal, omnibar and session list components, Then their state survives. Owner: 0.3, 5.6.
- UX-53. Given a deliberate type error and a deliberate render throw, Then an overlay appears and clears on fix, and `ErrorBoundary` still handles the render throw. Owner: 0.3 (spike) and 5.6 (CI-asserted by `dev-mode-hmr.spec.ts` in the Bun cell; not manual; G7).
- UX-54. Given a dev process crash, Then `scripts/dev-stack` restarts it and readiness is re-detected. Owner: 5.8.
- UX-55. Given a build failure at any stage, Then it exits non-zero, names the failing file and stage, leaves `web-app/out` intact and removes `.out-tmp-*`. Owner: 5.5 (atomic output; asserted in CI by `build.test.ts` through `make test-web-build`, not manual); file-naming message: G7.
- UX-56. Given a budget overrun, Then the build fails naming the budget, bytes and limit. Owner: 5.9.
- UX-57. Given a port already in use, Then `dev.ts` exits non-zero with the port in the message. Owner: none (G7).

### Triad review additions

- UX-58. Given the layout `main#main-content[tabindex=-1]` focused after a pathname change, Then its computed outline is `none` with no substitute `box-shadow`, the only rule removing it is scoped to that element, and other controls keep their focus rings (ParityDiff forced `:focus-visible` still matches Next). Owner: 4.5 (decision D8), 7.2a, 7.9.
- UX-59. Given the Axe sweep, Then it runs `wcag2a`, `wcag2aa`, `wcag21a`, `wcag21aa` and `wcag22aa` over all routes on both builds; a violation also present on Next is report-only (`KNOWN` with an issue), a Bun-only violation blocks. Owner: 7.8.
- UX-60. Given a route held pending beyond 150 ms, Then the `RoutePending` skeleton has no animation under `prefers-reduced-motion: reduce`, contains no `role=status`, `role=alert` or `aria-live`, and exactly one live-region change occurs for the navigation. Owner: 4.3 (Task 4.3d).
- UX-61. Given an iPhone-sized WebKit context, Then the `ssqTray` entry closes on Back with the URL unchanged and a `100dvh` pane does not overflow with the virtual keyboard; report-only (with a printed reason) when WebKit or keyboard emulation is unavailable, plus a real-iPhone item recorded in Story 8.1. Owner: 7.8 (Task 7.8b), 8.1.
- UX-62. Given the built Next output, Then its announcer's real `aria-live`/`role` is recorded (Task 0.5d), and given one screen reader (NVDA or VoiceOver) on the Bun build, Then the title is spoken once per navigation, focus lands on the main region and the not-found view is announced; recorded manually in `cutover-gate-results.md`. Owner: 0.5 (Task 0.5d), 8.1.

Totals: 62 acceptance criteria, 16 surfaces.

---

## 5. Exit-path audit

Every state has an exit:

| State | Exit |
|-------|------|
| Not-found view | Home link, Back control, global nav |
| ErrorBoundary after chunk failure | Retry button |
| Guard blank pane (flags slow) | Global nav; reload |
| Tray open | Back (closes it), tray close control |
| Dev overlay | Fix the error (auto-clear) or dismiss |
| Build failure | Fix the file named in the message; previous `out/` still usable |
| Login page without auth assets | No in-app exit; prevented by Go test and the 8.1 unauthenticated fetch (test-only) |

Unresolved UX blockers (flow with no exit path or missing error state): none that block planning. Two items need a confirmation rather than a new story: that the ErrorBoundary for a failed route sits inside the shell so nav stays usable (G2), and the login-asset lockout (S14) which has no in-app exit and depends on the Go tests, so 8.1's unauthenticated fetch must stay a hard cutover gate.

## 6. Gaps against `implementation/plan.md`

| Gap | UX criterion without an owner | Suggested owner |
|-----|-------------------------------|-----------------|
| G1 | UX-41: route-level pending UI. Plan 4.3 says routes load via `import()` but states no fallback, and Story 4.4 only covers `errorElement`. A bare `React.lazy` route would blank the main pane during chunk fetch; `lazy` route objects keep the old page. Pick a policy. | Add an AC to 4.3 or 4.4 |
| G2 | S11: confirm the failed-route ErrorBoundary renders inside the shell | 4.4 (add one assertion) |
| G3 | UX-29 and UX-17: not-found view content (heading, home/back links, title, announcement). 4.3c is "XS" with no AC; `RouteMeta` has 42 keys and no entry for the not-found title | 4.3c, 7.7 |
| G4 | UX-09, UX-10, UX-12, UX-13: announcer clear-then-set, no focus steal, popstate, redirect de-dup | 4.5 and 7.2 |
| G5 | UX-11: focus target when a page renders its own nested `main#main-content` (10 pages) | 4.5 (use the layout ref) and a 7.2 case on `/history` |
| G6 | UX-40: `sessionStorage` unavailable in StaleChunkReload; plus unsaved-draft protection (banner instead of automatic reload) | 2.7 |
| G7 | UX-53, UX-55 (message content), UX-57: dev error overlay AC in 5.6; build error message names file and stage; port-in-use message | 5.5, 5.6 |
| G8 | UX-45: no Playwright coverage of scroll (jest only, `research/ux.md` section 7 lists scroll as a gap) | New case in 7.3 or a 7.x spec |
| G9 | UX-27, UX-44: `?window=` preservation and the `/login` hard navigation have only the generic 7.10 full-suite run | 7.4 |
| G10 | UX-33, UX-34: assert theme CSS is a blocking `<link>` in the emitted head; boot script survives a throwing `localStorage` | 5.4, 7.5 |

Notes for the plan owner:

- `research/ux.md` section 7 items 1-9 are all present as Stories 7.1-7.8 (item 9, `aria-current`, folds into 7.2). Coverage of those nine is complete; the gaps above are second-order cases the research did not list.
- Story 7.2 correctly refuses to assert `aria-current` unless it exists on Next today. Keep that.
- Story 7.1 hard-load title checks rely on `toHaveTitle` auto-retry; on Bun the title is the root string until the route chunk mounts, so any spec that reads `document.title` once after `domcontentloaded` will be flaky. Use the auto-retrying matcher everywhere.
- The static `lang="en"` and `description` meta are covered in 5.3 but not asserted in 7.6; add `lang` to 7.6.

## Plan-gap resolution (Phase 3 repair, 2026-10-19)

Owners assigned in `implementation/plan.md` (the "Owner: none" criteria above now have one; the "accepted risk" for G6 is replaced by the banner criterion):

| Gap | Owning stories | Criterion location |
|---|---|---|
| G1 route-level pending UI | 4.3 (`RoutePending`, 150 ms rule), 4.4 | UX-41 |
| G2 failed-route error inside the shell | 4.4 (pathless layout route + pathless `errorElement` child) | S11 |
| G3 not-found content | 4.3c, 7.7b, 1.2 (`NOT_FOUND_META`) | UX-29 |
| G4 announcer edge cases | 4.5, 7.2b | UX-09, 10, 12, 13 |
| G5 nested `main#main-content` | 4.5, 7.2b | UX-11 |
| G6 storage unavailable, unsaved input | 2.7 (Task 2.7d) | UX-40 |
| G7 dev overlay, build error naming, port in use | 5.5, 5.6 (Task 5.6d) | UX-53, 55, 57 |
| G8 scroll e2e | 7.12 | UX-45 |
| G9 `?window=`, `/login` hard navigation | 7.4b | UX-27, 44 |
| G10 blocking theme CSS link, storage throws | 5.3, 5.4, 7.5b | UX-33, 34 |
| Triad: focus ring on programmatic `main` focus | 4.5 (D8), 7.2a, 7.9 | UX-58 |
| Triad: WCAG 2.2 AA axe tags | 7.8 (Task 7.8a) | UX-59 |
| Triad: `RoutePending` reduced motion, no double announce | 4.3 (Task 4.3d) | UX-60 |
| Triad: iOS Safari / WebKit checks | 7.8 (Task 7.8b), 8.1 | UX-61 |
| Triad: Next announcer politeness INFERRED, screen-reader item | 0.5 (Task 0.5d), 8.1 | UX-62 |
| Triad: UX-53 and UX-55 asserted in CI, not manually | 5.5, 5.6 (Task 5.6c) | UX-53, 55 |
