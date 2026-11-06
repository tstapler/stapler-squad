# Research: Features the new build must preserve (web-app-bun-build)

Scope: the feature surface Next.js provides implicitly, plus edge cases and failure modes of a migration to Bun.build + a client router.
Labels: **VERIFIED** = file opened or command run (cited). **INFERRED** = reasoned from code or general knowledge, not tested. Paths are relative to the worktree root; `web-app/` paths are under `web-app/`.

## 0. Corrections to the premises

| Premise | Finding |
|---|---|
| "41 routes" | **42** `page.tsx` files at this HEAD (`find src/app -name page.tsx`). The new one is `src/app/fanout/page.tsx` (#1096). VERIFIED. |
| "16 nested layouts" | 16 `layout.tsx` files, but only **4 do work**: root; `backlog/layout.tsx` and `settings/backlog-sources/layout.tsx` (client-side feature-flag gate that calls `router.replace("/")` when `flags.backlog` is off); `changelog/layout.tsx` (`RequireFeatureFlag`). The other 12 are `<>{children}</>` wrappers that only export `metadata`. VERIFIED (file dumps). Layout nesting is therefore not a router rabbit hole. Titles and gates are the real content. |
| "Storybook uses `@storybook/nextjs`" | `.storybook/main.ts` already uses `@storybook/react-webpack5` with babel-loader and a `css-ts-mock-loader`. `@storybook/nextjs` is only a leftover devDependency (`package.json:136`). Only `AppLink.stories.tsx` and `Navigation.stories.tsx` import `next/*`. VERIFIED. Storybook is not blocked by the migration. |
| "Dev server proxies to the Go backend" | **No proxy exists.** `next.config.ts` has no rewrites (static export forbids them). Dev is **cross-origin**: the frontend calls the backend directly via `NEXT_PUBLIC_API_URL`, and the backend allows the dev origin via `STAPLER_SQUAD_EXTRA_ORIGINS` (`main.go:465-476`, `scripts/dev-stack/launch.ts:206-221`). `bun dev` must replicate that, or add a proxy as a deliberate improvement. VERIFIED. |
| "Makefile builds in development mode" | Only minification is off (`next.config.ts` `isDevelopmentBuild`). `next build` still compiles with `process.env.NODE_ENV === "production"`. The deployed bundle therefore takes the production branches, e.g. `Providers.tsx` picks `HttpAnalyticsProvider` over `ConsoleAnalyticsProvider`. INFERRED from Next behaviour. See §6 for the Bun consequence, which is VERIFIED. |

## 1. Route inventory (VERIFIED, `find`/`grep` over `src/app`)

42 static routes, no dynamic segments, no `generateStaticParams`, no `error.tsx`, `loading.tsx`, `not-found.tsx` or `template.tsx`, and no `icon`, `favicon`, `robots` or `manifest` route files.

Routes: `/`, `/account`, `/analytics/escape`, `/backlog`, `/backlog/board`, `/changelog`, `/config`*, `/debug/escape-codes`, `/errors`, `/fanout`, `/files`, `/help`, `/history`, `/insights`, `/insights/session-detail`, `/login`, `/logs`, `/notifications`, `/resolve`, `/review-queue`, `/rules`, `/sessions/import`, `/sessions/new`*, `/sessions/summary`, `/settings`, `/settings/{background-models,backlog-sources,backlog-stages,defaults*,features,jules,llm-backends,pipeline-modes,remotes,tagging-classifier,unfinished}`, `/test/{escape-codes,layout-overlap,terminal-stress}`, `/triggers`, `/unfinished`, `/workflows`.

\* Redirect-only pages:
- `/settings/defaults` calls `redirect("/settings")` (`src/app/settings/defaults/page.tsx`).
- `/config` calls `redirect("/settings?tab=config-files")`. It drops any incoming query string (`src/app/config/page.tsx`).
- `/sessions/new` uses `router.replace("/?new=true")` or `"/?duplicate=<id>"` in a `useEffect`.

Dynamic-ID routes are emulated with query params because there are no `[id]` segments: `/insights/session-detail?sessionId=`, `/sessions/summary?sessionId=`, `/fanout?groupId=`. `src/app/insights/session-detail/page.tsx:23` says so explicitly.

`test/*` and `debug/*` pages ship in production. Keep them or drop them deliberately. `tests/e2e` may reference them (`test/layout-overlap`, `terminal-stress`), so check before dropping.

### Titles and metadata

Root: `title "Stapler Squad Sessions"`, `description`, and `viewport {width: device-width, initialScale: 1, maximumScale: 5, viewportFit: "cover"}` (`src/app/layout.tsx:17-27`).

Own titles, all in the form `<Name> - Stapler Squad`:
- Via layout: account, config, errors, history, rules, logs, workflows, triggers, fanout ("Compare Variants"), login ("Sign In"), review-queue, settings.
- Via page: analytics/escape, insights, notifications, sessions/import, unfinished ("Up Next"), and settings/{backlog-sources, jules, unfinished, tagging-classifier, background-models, llm-backends}.
  - These six read "<X> - Settings - Stapler Squad".

Everything else inherits:
- The parent title, which is "Settings - Stapler Squad" for `settings/*` pages without their own title.
- The root title otherwise, e.g. backlog, backlog/board, changelog, files, help, resolve, `/`.

`settings/backlog-stages/page.tsx:17` and `pipeline-modes/page.tsx:16` document that they are `"use client"` and omit metadata.

The built output in the main checkout confirms the prerendered `<title>` per route: `/` "Stapler Squad Sessions", `/settings/` "Settings - Stapler Squad", `/login/` "Sign In - Stapler Squad" (`server/web/dist/*/index.html`). That checkout's dist may differ from HEAD, but it contains `fanout/`.

Constraints the new build must meet:
- e2e asserts titles in about 20 places: `toHaveTitle(/Stapler Squad/)` in smoke, session-lifecycle, history-search, backlog and workspace-management specs, and `toHaveTitle(/Insights/i)` in `insights.spec.ts:18`.
- The axe sweep (`tests/e2e/a11y-route-sweep.spec.ts`) fails serious violations such as `document-title` on 29 routes. A title must exist on first paint, so keep the root `<title>` static in the HTML shell and update it per route.
- Next's prerendered HTML carries the per-route title before JS runs. A single-`index.html` SPA only gets the root title until JS runs. Fine for users; matters only for no-JS crawlers (none) and for specs that read the title immediately after `goto` with `waitUntil: 'domcontentloaded'`. Those use `toHaveTitle`, which auto-retries, so the risk is low (INFERRED).
- The `description` meta is not asserted anywhere I found (not exhaustively searched).

## 2. URL and query-param contracts (VERIFIED by grep of `searchParams.get`, `routes.ts`, e2e `goto`)

All keys read through `useSearchParams` (production code):

| Route | Keys | Source |
|---|---|---|
| `/` | `session`, `tab`, `newPane`, `notification`; launch params `new`, `pr`, `duplicate`, `worktree`, `branch`, `title`; plus `window` | `sessionDeepLink.ts:11-16`, `useOmnibarQueryParamLaunch.ts:20-25`, `lib/window/useWindowUrlSync.ts:54` |
| `/backlog`, `/backlog/board`, `/unfinished` | `item` | `backlog/page.tsx:261`, `backlog/board/page.tsx:40`, `UnfinishedTab.tsx:54` |
| `/unfinished` | `tab`, `item` | `lib/hooks/useUpNextTab.ts:28` |
| `/settings` | `tab` (including `config-files` from the `/config` redirect) | `settings/page.tsx:31` |
| `/settings/backlog-stages` | `editStage` | `:70` |
| `/history` | `sessionId`, `messageIndex` | `history/page.tsx:118,122` |
| `/review-queue` | `session`, plus filter keys via `useFilterState(FILTER_URL_KEYS)` | `:133`, `ReviewQueuePanel.tsx:368` |
| `/insights` | `preset`, `from`, `to` | `InsightsDashboard.tsx:87-89` |
| `/insights/session-detail`, `/sessions/summary` | `sessionId` | |
| `/fanout` | `groupId` | `fanout/page.tsx:131` |
| `/resolve` | `url` (an `ssq://` link) | `resolve/page.tsx:56` |
| `/login` | `setup_token` | `login/page.tsx:19` |
| `/rules` | `prefill` | `rules/page.tsx:16` |
| `/help` | `doc` | `help/page.tsx:23` |
| `/files` | `path` | `LocalFileBrowser.tsx:302` |
| `/test/layout-overlap` | `mode` | |

Read through `window.location.search` directly, so the router does not matter for these: `ApprovalRulesPanel.tsx:126` and `SessionCard.tsx:203` (`debug=1`).

Other keys seen in code that is not in this table: `useFilterState` consumers' keys, and a possible `window` read in `lib/telemetry/clientLogShipper.ts:166`.

Key facts for the router choice:
- **Multi-window state lives in the URL.** `?window=<id>` is the per-tab window identity (ADR-001 in `useWindowUrlSync.ts`), and it self-heals via `router.replace`. The router must preserve unrelated params across `replace`; the code rebuilds from `searchParams.toString()`.
- **Raw History API usage bypasses the router in four places**, which is the largest hidden risk:
  - `UnfinishedTab.tsx:144` and `useUpNextTab.ts:69`: `window.history.replaceState(window.history.state, "", "?...")`. Comments say `router.replace` "hard-reloads the static export" and that Next 15 syncs `useSearchParams` with native `replaceState`, though "not always" (`UnfinishedTab.tsx:55`). React Router and TanStack Router do **not** observe raw `replaceState`/`pushState`, so `useSearchParams` would go stale. Fix: route these through the router's navigate with `replace: true`. The "dismissedItemId" workaround state can then go. INFERRED for the library behaviour.
  - `SessionDetailView.tsx:495-504`: `#file=<path>` hash and `history.replaceState(null, "", ...)`. This passes `null` state, which wipes the router's `history.state`. React Router stores `idx` and `key` in `history.state`, so wiping it can break back/forward position tracking (INFERRED). Use `state` passthrough, or a router-aware hash update.
  - `lib/hooks/useTrayDismissal.ts:51`: `pushState({...history.state, ssqTray: true}, "")` plus a `popstate` listener, to give the mobile sheet a back-button entry. The router's own `popstate` handler also fires. This is a likely interaction bug and needs an explicit test on mobile (INFERRED).
- **`router.replace(url, { scroll: false })`** is used in 4 files (`page.tsx`, `useOmnibarQueryParamLaunch.ts`, `useWindowUrlSync.ts`, plus one more). The facade needs an equivalent `scroll` option, or the router must not scroll on search-only changes.
- Server-generated deep links the router must keep resolving:
  - `server/push/subscriber.go:344`: `/?session=<id>&tab=terminal` (with `&notification=<id>`).
  - `server/services/slack_notifier.go:281,326`: `/?session=<id>`.
  - `session/backlog_lifecycle_pr.go:378-381`: `<baseURL>/backlog?item=<id>`.
  - **None has a trailing slash before `?`.** `routes.ts` builds URLs the same way (`/unfinished?item=`).
- Push-click handoff: `public/push-sw-logic.js` normalises the URL, then `PushClickHandoff.tsx` calls `router.push(url)` with a path-plus-query string. It must keep accepting `/?session=...` strings. `tests/e2e/push-click-handoff.spec.ts:23,40,41` assert `window=`, `session=`, `notification=`.
- `/resolve?url=ssq://...` is the cross-host handoff landing page, and `window.location.href = data.advertisedAddress` sends the user to another host (`resolve/page.tsx:82`).
- The ConnectRPC auth interceptor hard-navigates `window.location.href = '/login'` on `Unauthenticated` (`lib/config.ts:44-48`). A full-page load of `/login` must therefore work from the Go server.

### Trailing slash: what actually happens today

- `next.config.ts` sets `trailingSlash: true`, so the export emits `dir/index.html` for every route plus `404.html`, `404/index.html` and per-route `index.txt` (RSC payload). Count in the main checkout's `server/web/dist`: **44 HTML files** (42 pages, `404.html`, `404/index.html`). VERIFIED (`find server/web/dist -name '*.html'`).
- Go (`server/middleware/static.go`) calls `fs.Stat(path.Clean(url))`:
  - `/rules` resolves to the directory `rules/` and serves `rules/index.html` with status 200, **with no redirect to `/rules/`**.
  - Any path that does not exist, such as `/nonexistent` or a deleted chunk, returns **root `index.html` with status 200**.
  - `out/404.html` is **never served** by Go.
  - Result: the address bar shows no trailing slash after a hard load. Specs assert exactly this: `tagging-rules-crud.spec.ts:56,175` expect `page.url()` to equal `${BASE_URL}/rules`, and `backlog-deep-link-backcompat.spec.ts:55` expects `/backlog?item=<id>$`. Others tolerate both with `\/?$` (`import-external-session.spec.ts:11`, `backlog-deep-link-errors.spec.ts:54`, `unfinished-work.spec.ts:156`).
  - Components compare `pathname` with strict equality: `CockpitShell.tsx:34` (`pathname !== "/review-queue"`), `Header.tsx:85,110` and `DrawerNav.tsx:60` (`=== routes.home`), `BottomNav.tsx:80,172`. `startsWith` is used for the other nav items.
  - Whether Next's client-side `<Link>` navigation appends a slash under `trailingSlash: true` (which would break the strict comparisons after in-app navigation) is **UNVERIFIED**. No test I read distinguishes it. A 10-minute probe of the current build on an isolated instance would settle it. The target is no trailing slash everywhere, matching `routes.ts` and the e2e specs.
- Consequence for the Bun build: a single root `index.html` already works with Go's existing fallback. Per-route HTML is not required for compatibility. It is only needed if per-route `<title>` must exist before JS runs.

## 3. Go server, embed and auth contracts that assume Next (VERIFIED)

1. **`server/middleware/auth.go:95-101`: `exemptPrefixes` = `/auth/`, `/login`, `/health`, `/_next/`, `/favicon`.** Remote or passkey auth redirects every other unauthenticated GET to `/login` with 302. A Bun build whose assets live anywhere other than `/_next/` (e.g. `/assets/`, `/chunk-*.js`) would make the **login page's own JS, CSS and fonts redirect to HTML**, so nobody can sign in. This is the highest-severity item. Options: keep emitting under `/_next/static/` (cheapest, which also keeps `static.go`'s immutable-cache rule), or change both Go files together with tests. `isExempt` is unit-tested in `auth_test.go` (confirm and extend).
2. `server/middleware/static.go:76`: `Cache-Control: public, max-age=31536000, immutable` only for `/_next/`. Everything else gets Go's default (embedded files have a zero ModTime, so no `Last-Modified`). A different asset directory would silently lose long-term caching. Content-hashed filenames are mandatory under that header (Bun's default `[name]-[hash]` qualifies).
3. **There is no `static_test.go`.** `ls server/middleware | grep static` finds only `static.go`. Add tests for the fallback, directory-index and cache-header behaviour before changing the layout, or the cutover has no regression net on the Go side.
4. Stale-chunk failure mode (exists today, worsens if chunk names change per build): after a deploy, an open tab requests an old hashed chunk. Go returns **`index.html` with 200** for the missing file, so the browser throws a syntax error instead of a clean 404. The new build should use `<script type="module">` and dynamic imports, which fail loudly, and consider letting `static.go` return 404 for paths with a file extension under the asset prefix (INFERRED improvement).
5. Gzip/zstd is applied by `server/middleware/gzip.go` (chain `otelhttp -> logging -> CORS -> gzip -> [auth] -> mux`, `server/server.go:1566`). So size budgets are measured raw, and nothing is pre-compressed on disk.
6. `server/web/embed.go`: `//go:embed all:dist`, and `server/web/dist` is a copy of `web-app/out` (`Makefile:229-235`). The contract is simply "everything in `out/`, with `index.html` at the root". Any output layout works as long as the Makefile copies it.
7. Absolute asset URLs are mandatory (`/_next/...` today). With the root fallback serving the same `index.html` for `/backlog/board`, a relative `./assets/x.js` would 404 into HTML on any nested deep link. Set Bun's `publicPath: "/"` explicitly.
8. No CSP header is set by the app server (only `sandbox` on file downloads, `file_service.go:789`), so the inline FOUC script is not blocked. No `X-Frame-Options` set.

## 4. Test, CI and tooling assumptions about Next output

e2e (`tests/e2e`, 152 `*.spec.ts`; root-level Playwright, global-setup spawns the Go binary with `--test-mode`):
- **No spec references `_next/` asset paths or `page.route('**/_next/**')`.** `grep "_next"` finds only the string `#__next-route-announcer__`. VERIFIED.
- **`#__next-route-announcer__` is excluded in 6 files**: `accessibility.spec.ts:230`, `multi-window.spec.ts:16-26`, `terminal-resync-banner.spec.ts:171,240`, `terminal-resync-feature-flags.spec.ts:114`, `insights-session-route.spec.ts:40`, `pages/EscapeAnalyticsGlobalViewPage.ts:39-43`. Those exclusions still pass when the element is absent. But Next injects an `aria-live="assertive"` region that announces the new page title on client navigation. Dropping it **removes a real screen-reader navigation announcement** (a11y regression, not caught by any test). The new router needs a polite live region that announces `document.title` after navigation, plus the focus handling below. INFERRED for Next's behaviour (the announcer is created at runtime; `server/web/dist/index.html` has 0 occurrences, which is consistent with that).
- e2e also pins: 29 routes in the axe sweep (`a11y-route-sweep.spec.ts`), `toHaveURL` patterns (§2), `toHaveTitle` (§1), `page.waitForURL(`${BASE_URL}/`)` after feature-flag redirects (`backlog.spec.ts:638-645`, `backlog-sources-settings.spec.ts:131-132`), which depend on the `router.replace("/")` in the gating layouts.
- **`playwright.dev-mode.config.ts` plus `global-setup.dev-mode.ts`** start a two-process DevStack (Go backend plus `next dev`) via `scripts/dev-stack/launch.ts` (`nextBin dev --port N --hostname localhost`, env `NEXT_PUBLIC_API_URL`, readiness = any HTTP response). `make dev-stack` and `npm run test:e2e:dev-mode` depend on it, so it needs a Bun equivalent (or a documented removal). `launch.test.ts` and `ports.test.ts` mock `next dev`.
- `web-app/playwright.config.ts` (legacy, port 3333) is separate from the root config and has a `webServer` entry that starts a dev server. Check whether it is still used before porting.
- Lighthouse: `web-app/lighthouserc.json` uses `npx serve@14 out -l 3001` and audits only `http://localhost:3001/`. It needs only a root `index.html`, but `serve` does no SPA fallback, so it only works for `/`. `ux-analysis.yml` runs `lhci autorun --config=lighthouse.config.js` against a real Go server. Neither depends on `_next`.
- `size-limit` (`package.json:108-127`) globs `out/_next/static/chunks/**/*.js`, `framework-*.js`, `main-*.js`. Those three budgets (5 MB, 650 KB, 400 KB) must be re-expressed for Bun's naming. A "framework" chunk does not exist unless you create one via splitting. `benchmark.yml` (≈lines 285-295, 345, 531, 609-620) runs `pnpm run build` directly (not `make`) and measures a **production** Next build, while the deployed one is the unminified dev build.
- CI caches `web-app/.next/cache` in `.github/actions/prepare/action.yml:62-66`, `build.yml:180-184`, and four times in `benchmark.yml` (285, 345, 531, 609). These can be deleted or retargeted.
- `.github/workflows/lint.yml:276-379` and `web-app/scripts/lint-affected.sh:50` run `next lint`, which exits 1 on any warning. A replacement must reproduce that (warnings as failures) or the gate silently weakens. `Makefile:1021` runs `npx next lint`.
- `.eslintrc.json` extends `next/core-web-vitals` and has custom rules (boundaries, analytics, rpc-lifecycle, 100vh bans). Only the `next/core-web-vitals` part goes away.
- `tsconfig.json` has `"plugins": [{"name": "next"}]`, `jsx: "preserve"`, and includes `next-env.d.ts` and `.next/types/**/*.ts`. Bun's JSX transform ignores `preserve` for bundling, but `tsc` and the IDE need `jsx: react-jsx`.
- `.gitignore` lists `/web-app/.next`, `/web-app/out/`. `backlog-scaffolding-guard.yml:53` greps for `web-app/.next/`.
- Jest: `jest.config.js` (ts-jest, jsdom) maps `.css.ts` to `styleMock.js`, so Jest is independent of the bundler. 40 test files mock `next/navigation`/`next/link`; those mocks move to the facade.
- `web-app/.jscpd.json` ignores only `**/fonts/**`; no `next`-specific path keys. `tools/scanner/frontend` contains only `package.json`, `src/` and `tsconfig.json`; I did not read `src/`, so whether the feature-registry scanner keys on `src/app/**/page.tsx` is **UNVERIFIED** (grep one hop deeper before finalising).
- `.env.local.example` documents `NEXT_PUBLIC_RECONNECT_V2`. `next dev` auto-loads `.env.local`; `Bun.serve` loads `.env` files but only inlines vars selected via the build `env` option (see §6).

## 5. Service worker, PWA, manifest, mobile app

- `public/push-sw.js` (+ `push-sw-logic.js`) is registered at `/push-sw.js` with root scope (`lib/hooks/usePushNotifications.ts:114,172`). It must be copied verbatim to `out/` root, and its `importScripts` path must still resolve. It handles `notificationclick` and hands the URL to an open window (focus, then `postMessage`) or `openWindow`. It has no cache layer; a comment says caching is handled by a separate `cache-sw.js`, which does not exist in `public/`. VERIFIED (`ls public`).
- `public/manifest.json` has `start_url: "/"`, `display: standalone`, 8 icons under `/icons/`. **Nothing links it**: `server/web/dist/index.html` has no `rel="manifest"` and no source file adds one. So installability is not wired today (VERIFIED by grep of `src` and dist). Do not "preserve" a link that does not exist. Note it as a possible follow-up, not a parity requirement. The auth exempt list also does not cover `/manifest.json`, `/icons/` or `/push-sw.js` (not exempt, only matters for unauthenticated pages).
- Static public assets referenced by the app: `/docs/*.md` (11 files, loaded by the help page `lib/docs/docLoader`), `/icons/*`, `/logo.svg`.
- `index.txt` / RSC payload files disappear. Nothing external should read them. `ls` shows `index.txt` at dist root; no Go or test code references it (INFERRED from earlier `_next` grep, which does not cover `index.txt` — low risk).
- Mobile app: there is **no mobile client source in this repo** (`ls` of the repo root). The only mobile-facing contracts found are the deep-link URLs in §2 and the server address recorded in memory (`https://onyx.staplerhome.internal:8444`, a direct connection). Mobile web (Android Chrome and iOS) is exercised by the `ViewportProvider`, `--viewport-height`, `android-url-bar.spec.ts` and ESLint bans on `100vh`/`100dvh` (`.eslintrc.json`). The build must keep the viewport meta tag exactly, including `viewport-fit=cover`. UNVERIFIED whether an external app depends on trailing-slash URLs; none of the repo's URL builders produce them.

## 6. Build-time features Next provides implicitly (each needs an explicit replacement)

| Next feature | Where used | Replacement requirement |
|---|---|---|
| `next/font/local` x4 families | `src/app/fonts.ts`; 8 woff2 files, 240 KB, in `src/app/fonts/` | `@font-face` CSS with `font-display: swap` (Next's default), `<link rel=preload as=font crossorigin>` for the files (dist shows 8 preloads), and a **`.variable` class equivalent** that sets `--font-jetbrains-mono`, `--font-rajdhani`, `--font-cinzel`, `--font-inter` on `<html>` (consumed by `styles/theme.css.ts:389-626`, `globals.css`, `FileContentViewer.tsx`). Next also generates size-adjusted fallback fonts to avoid CLS; losing them is a small CLS regression (INFERRED). The font files must be hashed and copied under the asset prefix that auth exempts (§3). |
| `export const metadata`/`viewport` | §1 | Static shell title, per-route title helper, viewport meta in the HTML shell. |
| Root layout `<html className=...>` | `layout.tsx:75-79` | `<html lang="en" class="${cleanTheme} ${font vars}">` in the HTML shell. `cleanTheme` is a build-time vanilla-extract class hash, so the shell must be generated after VE evaluation, or the class must be set by the FOUC script. |
| FOUC theme script | `layout.tsx:36-60` | Inline `<script>` in `<head>`, using a map of 6 VE theme classes (`matrix, cyberpunk77, wh40k, clean, light, dark`) baked from `@/styles/theme.css`. It reads `localStorage['stapler-theme']` and `stapler-theme-custom` (`custom:` themes set CSS vars and a class) and removes all known theme classes before adding one. `ThemeContext.tsx:22,40,225-228` uses the same keys and classes, so any change to class naming must be consistent. No CSP blocks inline script. The shell generator must import the same VE module (and so run after the VE plugin), or the map will be stale. |
| `NEXT_PUBLIC_*` inlining | `NEXT_PUBLIC_API_URL` (3 reads: `lib/config.ts:16` etc.), `NEXT_PUBLIC_RECONNECT_V2` (7 reads, 4 files), `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING`, `NEXT_PUBLIC_OTEL_ENABLED`, `NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (5 vars), plus `process.env.NODE_ENV` (9 reads) | **VERIFIED with a probe on Bun 1.4.3** (scratchpad `bunprobe/b.ts`): (a) `Bun.build` with `env: "NEXT_PUBLIC_*"` inlines only vars that are **set at build time**; unset ones stay as the literal `process.env.X`, which throws `ReferenceError: process is not defined` in the browser. This matches the spike's earlier `process is not defined` error. Every one of the 5 vars must be put in `define` explicitly (as `"undefined"` or a JSON string), not left to the env prefix. (b) `process.env.NODE_ENV` is **inlined as `"development"` by default**, even when `minify: true`. Without `define: {"process.env.NODE_ENV": '"production"'}` the deployed bundle would flip `Providers.tsx` to `ConsoleAnalyticsProvider`, silently disabling HTTP analytics, and enable dev branches in the 9 reads. The "dev vs prod build" switch must toggle minify and sourcemaps, never NODE_ENV. |
| `next/dynamic` x4 files, 7 sites, all `ssr: false` with a `loading` skeleton | `SessionDetail.tsx:24`, `SessionDetailView.tsx:289`, `InsightsDashboard.tsx:46-58` (4 charts), `test/terminal-stress/page.tsx` | `React.lazy` + `Suspense fallback` (no SSR to disable). Bun splitting must produce real separate chunks for the `import()` calls (recharts, xterm in `TerminalOutput`). Check chunk count and size against budgets. |
| `useReportWebVitals` | `components/telemetry/WebVitalsReporter.tsx:5` imports an **internal** path `next/dist/client/web-vitals` | `web-vitals` is not a direct dependency (`package.json` has no entry). Add a pinned version or use `PerformanceObserver`. Metrics emitted: CLS (scaled x10000), INP, LCP, FCP, FID, TTFB, with `performance.mark('web-vital:<name>')`. `metric.rating`, `delta` and `id` are used, which the `web-vitals` library provides natively. |
| Error handling | No `error.tsx`. `ErrorBoundary` wraps the app in the root layout (reload button calls `window.location.reload()`), `TrayErrorBoundary` wraps the notification panel | Replace with the router's error element only if per-route boundaries are wanted. Parity = keep the existing boundaries. |
| 404 page | Next emits `404.html` ("404: This page could not be found."), but **Go never serves it** (§2). A hard load of an unknown path gets root `index.html`, and then Next's client router renders its built-in not-found (INFERRED) | Add an explicit catch-all route that renders a small not-found view. Must not redirect (redirecting would break `waitForURL` expectations). |
| Scroll restoration | `scroll: false` on 4 replace sites; no `scrollRestoration` or manual `scrollTo` in `src` (`FileTree.tsx:696` is a tree widget) | Next resets scroll to top on forward navigation and restores on back (INFERRED). React Router needs `<ScrollRestoration />` or equivalent, and must **not** scroll on search-only changes. Test with `/backlog?item=` selection. |
| Focus management on navigation | Skip link `<a href="#main-content">` in `layout.tsx:67`; `inert` on `#main-content` while modals are open (`useTrayDismissal.ts:70-74`). Next moves focus/announces on route change (INFERRED) | Add a post-navigation focus move to `#main-content` or `h1`, plus the live-region announcement (§4). Without it, keyboard and screen-reader users stay focused on the clicked link in an unrendered view. |
| `next/link` | 24 files via `NavLink` (`lib/navigation/NavLink.tsx`, re-exported as `AppLink`), sets `prefetch={false}` | Facade `Link`. The `prefetch` prop becomes a no-op (the router has no route-level prefetch for static chunks unless added). |
| `next/navigation` | 44 import sites (non-test); `usePathname` x6, `redirect` x2, `useRouter` x26, `router.push/replace` x66; **no** `router.back/refresh/prefetch`, no `useParams`, `notFound`, `next/image`, `next/script`, `next/head` | Small, enumerated surface. `from "next"` (24) is type-only (`Metadata`/`Viewport`/`NextConfig`). |
| `useSearchParams` + `<Suspense>` | 21 files contain `<Suspense` (Next static export requires it) | Harmless to keep. |
| Server-component `redirect()` | 2 pages | `<Navigate replace>`; note `/config` drops the incoming query string. |
| `reactStrictMode: true` | `next.config.ts` | Wrap in `<StrictMode>` or effects will not double-run in dev; hooks with fire-once refs (e.g. `hiddenSessionFallbackRef`, `lastUrlRoutedRef` in `page.tsx`) are designed with this in mind (INFERRED). |
| webpack `extensionAlias` | `.js` -> `.ts/.tsx` for generated protobuf imports (`next.config.ts:36-40`) | Bun resolves `./x.js` to `./x.ts` natively (INFERRED; confirm during the spike with the real `src/gen/**` output, since the spike already bundled 41 pages). |
| `optimizePackageImports: ['@/components','@/lib']` and CSS chunk consolidation (`splitChunks.styles`) | `next.config.ts:22-23,66-86` | Bun tree-shakes barrels itself; CSS ordering across split chunks is a **cascade-order risk** with vanilla-extract + global CSS (`globals.css`, `globalEffects.css`). Verify the order of `globals.css` vs `.css.ts` output is unchanged. |
| Source maps | `productionBrowserSourceMaps: true` | Emit `.map` files (Bun `sourcemap: "linked"`); are they embedded in Go? They add to the 52 MB dist. Decide whether to ship them. |
| `retry-with-backoff.sh -n 3` around the build | `Makefile:226` | Probably to absorb flaky Next builds; a deterministic build can drop it. |

## 7. Dev-mode behaviours `bun dev` must replicate

- Port `${PORT:-3001}`, host `localhost` (`package.json:15`); DevStack allocates an arbitrary frontend port and sets `NEXT_PUBLIC_API_URL` to the backend (`launch.ts:215-221`) and `STAPLER_SQUAD_EXTRA_ORIGINS=http://localhost:<frontendPort>` on the backend (§0).
- CORS origin validation accepts only `http(s)://localhost:<port>` or `127.0.0.1:<port>` and only when `STAPLER_SQUAD_INSTANCE` is also set (`main.go:465-476`, `parseExtraOrigins`). So `bun dev` must bind **`localhost`/`127.0.0.1`**, not `0.0.0.0` or a LAN name, or CORS fails.
- Cookie auth (`cs_auth`) and WebSockets (`watch-ws-transport.ts`, `BrowserTab.tsx`, `CDPViewer.tsx`) across origins: cross-origin cookies need `SameSite`/`credentials` handling; today dev with passkey auth enabled is likely already awkward. Using a Bun dev-server proxy for `/api`, `/auth` and WS would make dev same-origin and is a candidate improvement (not parity).
- SPA fallback in dev: deep links such as `/backlog?item=x` must serve the shell (Bun's HTML route `"/*"` handling).
- HMR: `.css.ts` edits and React Fast Refresh (spike validated one component only). Error overlay and source maps in dev.
- Unminified dev build and readable stack traces, as `make` provides today.
- `DevStack` readiness treats "any HTTP response" as ready.
- `tests/e2e/helpers/test-server.ts` builds the Go binary and so depends on `make server/web/dist` ordering (the comments at `:162-177` say a stale frontend is prevented via the Make dependency chain).

## 8. Unstated user needs (INFERRED from repo history and practices)

1. **Stable deep links**: every URL builder in §2 and the e2e `goto` shapes must keep working, including cold loads on nested paths (needs absolute `publicPath`).
2. **Fast reload**: the owner measured 54-88 s builds; the target is dev reload < 100 ms (spike: 84 ms CSS).
3. **Source maps** in dev and a decision for prod.
4. **Multi-worktree friendliness**: the repo shares `node_modules` across worktrees and the webpack cache (`NEXTJS_SHARED_CACHE_DIR`) to avoid inode pressure. The Bun build must not write large caches into the worktree, and must not require a per-worktree install step beyond what `shared-node-modules.sh` already does.
5. **Agent-friendly failure output**: sessions run builds unattended; the build must exit non-zero with the failing file, including on invalid CSS (the `@keyframes` bug that webpack silently passed).
6. **No live-instance disturbance**: manual testing must use the 62871/62872 block with an isolated tmux socket (CLAUDE.md).
7. Unminified-versus-minified switch for debugging in the deployed binary.

## 9. Edge cases and failure modes to design for

1. Auth-exempt prefix mismatch (§3.1): login page unusable under remote auth. **Highest severity; trivially testable with a Go unit test and an e2e under `--remote-access`.**
2. `process.env.*` leaks: unset `NEXT_PUBLIC_*` throwing `ReferenceError` in the browser; default `NODE_ENV=development` (§6, VERIFIED).
3. Raw History API (`replaceState`/`pushState`) not observed by the new router (§2). Four sites, plus `useWindowUrlSync` and `useFilterState` rely on exact `searchParams` re-render timing.
4. Strict `pathname ===` comparisons break if the new router yields trailing slashes (§2).
5. Stale-chunk 200-HTML masking (§3.4); no cache header on a non-`/_next/` prefix (§3.2).
6. Feature-flag gating layouts: `backlog/layout.tsx` returns `null` while `isLoading`, then redirects to `/` if the flag is off. With a router, keep the same loading-then-redirect behaviour or the e2e `waitForURL('/')` tests fail (`backlog.spec.ts:638`).
7. Strict-mode double effects with fire-once refs (`useOmnibarQueryParamLaunch`, `page.tsx` deep-link effects). The omnibar launch hook calls `router.replace("/")` to clear its params, and any router that makes `searchParams` identity change on every render re-triggers it (the hook lists `searchParams` in deps; INFERRED).
8. `window.location.reload()` at `ErrorBoundary.tsx:44`, `WorkspaceSwitcher.tsx:80`, `ConnectionIndicator.tsx:100`, `useDatabase.ts:129`: fine with any bundler.
9. CSS order and invalid CSS (the 4 `@keyframes` files fixed in #1097). Other strict-parser failures may remain; run Bun's CSS parser over all 336 `.css.ts` outputs and treat warnings as errors in the build.
10. vanilla-extract class hashes are build-dependent, so the FOUC map, `className` hydration and any `localStorage`-stored custom-theme class (`stapler-theme-custom` stores `cls` — see `layout.tsx:56`) can go stale across deploys. Custom-theme rows already guard with `c.id===t`; check that a stale `cls` degrades to `clean` (INFERRED).
11. Test files for Bun/VE (`bun patch` of `eval`): the Jest suite never exercises it; add a build-time smoke test (evaluate N `.css.ts`, compare class map with the Node result, as the spike did).
12. `jest.mock("next/navigation")` in 40 files; if the facade is introduced under Next first (feasibility §6), those mocks must move to `@/lib/nav` in one codemod, or half the suite will pass vacuously against the real hook.
13. The feature-registry markers (`// +feature:` in the first 10 lines of React files) must survive any file moves (e.g. `resolve/page.tsx:2`).

## 10. Comparable migrations and known gotchas (general knowledge; INFERRED, not freshly sourced in this session)

- **Next static export -> Vite SPA (React Router or TanStack Router)** is the most common path. Recurring problems: (1) `process.env.NEXT_PUBLIC_*` replacement (Vite uses `import.meta.env` and `VITE_` unless `define` is added); (2) `trailingSlash`/`dir/index.html` output turning into one `index.html` plus host rewrite rules, with the stale-chunk-returns-HTML problem; (3) the lost route announcer and focus management; (4) `useSearchParams` semantics differ (Next returns a read-only snapshot; React Router's setter does functional updates); (5) font handling; (6) `<Link>` active state and prefetch.
- **Bun.build specifics** (this session's probes, Bun 1.4.3): default `NODE_ENV=development`; env-prefix inlining skips unset vars (VERIFIED above). Bun's HTML-import dev server with HMR is documented to support React Fast Refresh, but it was validated on one component only.
- Bun plugin ecosystem is thin. Expect to hand-write: the vanilla-extract plugin (spike: ~25 lines), CSS handling for `.css.ts` output and global ordering, HTML shell generation, hashing/copy of `public/`, and a manifest of emitted files for `size-limit`.
- Router supply-chain: TanStack Router had a May 2026 incident (`@tanstack/react-router` 1.169.5 and 1.169.8) per `requirements.md`; React Router 7 library mode behind a facade avoids that and the search-param typing gap can be closed with small Zod-free parsers in the facade.

## 11. Parity checklist for planning (derived)

Must-have (blocks cutover): auth-exempt asset prefix, absolute `publicPath`, NODE_ENV and 5 `NEXT_PUBLIC_*` defines, theme FOUC shell, font-face plus variable classes, per-route titles, 42 routes plus catch-all not-found, redirect pages, feature-flag gate layouts, all query-param contracts, `scroll:false` semantic, History-API call sites, `public/` copy (push SW, docs, icons, logo, manifest), `size-limit` globs, Makefile and embed path, `bun dev` with CORS-compatible origin, dev-stack and dev-mode e2e harness, web-vitals replacement.

Should-have (a11y and fidelity): route-change announcement, focus management, scroll restoration, size-adjusted font fallback, unminified debug switch, `next lint` warning-as-error parity, Go `static_test.go`.

Decisions to surface to the owner: ship per-route HTML or one shell; keep the `/_next/` prefix for assets (cheapest, avoids Go changes) or rename with Go changes and tests; whether to link `manifest.json`; whether to proxy `/api` in `bun dev`; whether `test/*` and `debug/*` pages ship in production; whether to ship source maps.
