# Architecture research: web-app Bun build + routing layer

Phase 2 (Architecture dimension) for `project_plans/web-app-bun-build/`. Complexity 4.
Labels: **VERIFIED** = I opened the file or ran the command this session. **INFERRED** = reasoned from verified facts, not tested. **UNVERIFIED** = needs a spike before planning relies on it.
Paths are relative to the repo root unless prefixed `spike:` (= `/home/tstapler/.cache/ssq-e2e-tmp/claude-1000/-home-tstapler-Programming-stapler-squad/3967ab3e-3f96-43f9-8669-c617ee1faa4a/scratchpad/bench/bun-spike/`).

## 0. Corrections to the prior docs (found while reading code)

| Prior claim | Finding |
|---|---|
| "16 nested layouts" are a router rabbit hole (requirements.md, feasibility §1) | **VERIFIED** by reading all 16 `layout.tsx` files: 1 root, **2 feature-flag guards** (`src/app/backlog/layout.tsx`, `src/app/settings/backlog-sources/layout.tsx`, ~20 lines each, `useFeatureFlags` + `router.replace("/")`), **1** `RequireFeatureFlag` wrapper (`changelog/layout.tsx:1-7`), and **12 pure metadata pass-throughs** (`account config errors fanout history login logs review-queue rules settings triggers workflows`; each is `export const metadata = {title}` + `<>{children}</>`). No layout renders chrome. The "nesting" is really a flat route table with a `title` and an optional flag guard. |
| "41 routes" | `find src/app -name page.tsx` = **42** (includes `/`). The spike's `entry.tsx` lists 41 plus the root. Use 42 in acceptance criteria. |
| Storybook "moves off `@storybook/nextjs`" is a rabbit hole (requirements.md "Rabbit Holes", feasibility §5) | **VERIFIED**: `web-app/.storybook/main.ts:1,20` already uses `@storybook/react-webpack5` (its header comment explains `@storybook/nextjs` was abandoned). `@storybook/nextjs` is still in `package.json:136` as an unused devDependency. Removal is a one-line dependency delete, not a framework migration. `.storybook/css-ts-mock-loader.js` already stubs `.css.ts`, so Storybook never touches the vanilla-extract/Bun plugin. |
| Pathname semantics unchanged by a router swap | **INFERRED, needs a test**: `next.config.ts:22` sets `trailingSlash: true`, so Next's `usePathname()` plausibly returns `/review-queue/`. `CockpitShell.tsx:34` does `pathname !== "/review-queue"` and `Navigation.tsx:45` does `pathname === item.href`; both would be latent no-ops today if so. The facade must normalise pathnames (strip trailing slash except `/`) and a characterisation test must pin the current behaviour before the swap. |
| Next provides the route announcer | **VERIFIED**: six e2e specs exclude `#__next-route-announcer__` from `[role="alert"]` queries (`tests/e2e/multi-window.spec.ts:26`, `insights-session-route.spec.ts:40`, ...). React Router has no announcer, so the swap silently removes a screen-reader navigation announcement unless we add one. Add a `RouteAnnouncer` (polite `role="status"`, not `role="alert"`, so the e2e alert counts do not change). |
| `router.refresh/prefetch` must be supported | **VERIFIED** by grep: zero production uses. The facade omits them. Only `useRouter` (27 importers), `useSearchParams` (23), `usePathname` (6), `redirect` (2 files: `app/config/page.tsx:5`, `app/settings/defaults/page.tsx:5`). `useParams` is only in the global jest mock; there are no dynamic segments. |

Baseline facts reused without re-deriving: static-export contract and 63-file router footprint (`docs/explanation/bun-migration-feasibility.md` §1), spike numbers (§2, §9), `eval`/`Module._load` root cause (§3.1). I did not re-run the spike benchmarks.

## 1. Build pipeline: stages and where they live

Current chain (**VERIFIED**, `Makefile:181,210-238,570-585`):

```
ensure-tools -> node_modules/.modules.yaml (scripts/shared-node-modules.sh)
             -> proto-gen (buf generate proto -> web-app/src/gen, stamp .proto-gen.stamp)
             -> web-app/out (next build)  -> server/web/dist (rm -rf + cp -r)
             -> go build (//go:embed all:dist, server/web/embed.go:8)
```

Target chain. Only the `web-app/out` recipe body changes; every other edge and every consumer of `web-app/out` stays:

```
ensure-tools (adds bun to .tool-versions; today: golang, nodejs, buf only)
 -> node_modules (shared-node-modules.sh, re-keyed on bun.lock + patches/**; see §11)
 -> proto-gen            (unchanged; it already depends on node_modules for protoc-gen-es)
 -> web-app/out          (WEB_BUILD=bun: `bun web-app/build/build.ts`)
 -> server/web/dist      (unchanged cp)
 -> go build             (unchanged embed)
```

`build/build.ts` is the single Bun entry. Its internal stages, each a small function so they can be unit-tested and timed:

| # | Stage | Notes |
|---|---|---|
| 1 | **clean** | Build into `web-app/.out-tmp-$$`, never into `out/` directly (see §11 worktree/rollback). |
| 2 | **theme-map** | Evaluate `src/styles/theme.css.ts` exports for the FOUC script (§3.4). |
| 3 | **bundle** | `Bun.build` with VE plugin, `splitting`, `minify` (default on; `WEB_MINIFY=0` for debug), `sourcemap: "linked"`, `publicPath: "/"`, `define` for the five `NEXT_PUBLIC_*` vars. |
| 4 | **html** | Produce `index.html` (§2). |
| 5 | **public** | Copy `web-app/public/**` to the output root. Next did this implicitly. `push-sw.js` (service worker) and `manifest.json` must be at the root path. **VERIFIED** they live in `web-app/public/`. |
| 6 | **manifest** | Write build stats (duration, JS/CSS bytes, chunk count, initial-JS bytes) to `web-app/.build/stats.json`, **not** into `out/`. `server/web/embed.go:8` uses `all:dist`, so anything in `out/` is embedded and served publicly. |
| 7 | **swap** | `rm -rf out && mv .out-tmp out` (rename is atomic; a failed build leaves the previous `out/` intact). |

Placement: `web-app/build/{build.ts,dev.ts,html.ts,theme-map.ts,plugins/vanilla-extract.ts}` plus `web-app/bunfig.toml`. These are outside every `eslint-plugin-boundaries` element (elements only cover `src/**`), so no lint-model change. The Makefile prerequisite list replaces `web-app/next.config.ts` with `web-app/build/** web-app/bunfig.toml web-app/patches/**`.

Define semantics: Next inlines `NEXT_PUBLIC_*` from the build-time environment. `build.ts` must do the same (`define: {"process.env.NEXT_PUBLIC_API_URL": JSON.stringify(process.env.NEXT_PUBLIC_API_URL)}` and `"undefined"` literal only when unset). The spike hard-coded `undefined` for all five (`spike: build2.ts`), which is wrong for `scripts/dev-stack/launch.ts:221`, which sets `NEXT_PUBLIC_API_URL`. Keep the `NEXT_PUBLIC_` names through the dual-build period; renaming is a cleanup-PR item.

## 2. HTML entry and route strategy

**Recommendation: one `index.html`, no per-route HTML.**

Why (all VERIFIED unless noted):
- `server/middleware/static.go:20-38` already serves root `index.html` with 200 for any path that does not exist in the embedded FS. With a single `index.html`, `/settings/`, `/backlog/board`, etc. do not exist on disk, so every deep link takes that branch. Zero Go change needed for routing.
- 42 static pages, no `[id]`, no SSR. Per-route HTML under CSR is 42 identical shells; nothing distinguishes them but `<title>`, which React 19 can set client-side (§5).
- Dropping per-route HTML removes the `trailingSlash` coupling between Next, Go and the e2e specs. The specs are already tolerant: `/\/settings\/unfinished\/?$/` (`tests/e2e/unfinished-work.spec.ts:156`), `/\/insights\/session-detail\/?\?sessionId=/` (`insights-findings-panel.spec.ts:30`); the exact-match ones are for `${BASE_URL}/` (root), which is unchanged. Canonical form becomes no trailing slash; `/x/` still resolves because Go falls back and React Router ignores a trailing slash when matching (**INFERRED**, RR documented behaviour; confirm in the router spike).
- Lighthouse CI serves `out/` with `npx serve@14 out -l 3001` and only requests `/` (`web-app/lighthouserc.json`), so it works unchanged. If a deep-link URL is ever added there, `serve` needs `-s`.

Traps to design around:
1. **Absolute asset URLs.** With one HTML served at arbitrary depths, assets must be `publicPath: "/"`. A relative `./chunks/x.js` breaks at `/backlog/board`. The spike never served a nested path, so this is untested (**UNVERIFIED**).
2. **Unauthenticated asset prefix.** `server/middleware/auth.go:98-102` exempts `/auth/ /login /health /_next/ /favicon` from login. Any other asset prefix means the `/login` page cannot load its own JS, and the user is locked out. **Recommendation: emit assets under `/_next/static/...` during the dual-build period** (Bun's `naming` option allows it: `chunk: "_next/static/chunks/[name]-[hash].[ext]"`). That keeps `auth.go:99`, the `static.go:82` immutable-cache branch and the `.size-limit.json` globs valid with no Go change in the cutover PR. Rename to `/assets/` in the cleanup PR as one atomic Go + build change (binary and UI ship together, so no skew except stale browser tabs, §11 F10).
3. **Go masks missing assets as HTML.** `static.go:23-38` returns `index.html` 200 for a missing `/_next/static/chunks/foo.js`, so a stale tab fetching a deleted hashed chunk gets HTML parsed as JS ("Unexpected token <"). Small Go hardening: 404 for misses under the asset prefix. Pair it with a one-shot reload on dynamic-import failure (`vite:preloadError` equivalent; hand-rolled in the lazy helper, §6).
4. **Route/public collisions.** `public/docs` and `public/icons` exist; a route named `/docs` would hit `static.go:41-58` directory handling. No current route collides (**VERIFIED** against the 42 page paths).

### 2.1 HTML generation: two designs, spike-gated

- **A (preferred): HTML as the Bun entrypoint.** `web-app/index.html` with `<script type="module" src="./src/main.tsx">`. `Bun.build({entrypoints:["index.html"]})` rewrites hashed asset URLs, and the same file is the dev server's HTML import. This makes dev and prod share one template. **UNVERIFIED**: the spike used `entry.tsx` for build and an HTML import only for dev (`spike: devtest/index.html`). Gate: spike that HTML-entry + `splitting` + plugins + `publicPath:"/"` + `naming` prefix work together on Bun 1.4.3.
- **B (fallback): custom `html.ts`.** Entry is `main.tsx`; `html.ts` renders the template from `result.outputs`. More code, full control over preload tags and the asset prefix. Dev then needs its own `Bun.serve` `fetch` that calls the same `html.ts`.

### 2.2 `<head>`, `<html>` content that currently lives in `app/layout.tsx`

`app/layout.tsx` (**VERIFIED**, lines 13-83) renders `<html lang className={clean theme + 4 font vars} suppressHydrationWarning>`, `<head>` with the FOUC script, `metadata`, `viewport`. In a CSR build:
- `lang`, `viewport` (`width=device-width, initial-scale=1, maximum-scale=5, viewport-fit=cover`) and the default title/description go into `index.html` statically.
- `suppressHydrationWarning` becomes moot (no hydration).
- Font-variable classes on `<html>` (`jetbrainsMono.variable`, ...) are produced by `next/font/local` (`app/fonts.ts`, four families, vendored woff2). Replace with `src/app/fonts.css.ts` using VE `fontFace()` plus `globalStyle(":root", {vars})` defining `--font-jetbrains-mono`, `--font-rajdhani`, `--font-cinzel`, `--font-inter`. Consumers already use `var(--font-*)`; no class needed on `<html>`. Woff2 `url()` resolution through the VE virtual-CSS namespace needs the plugin's `resolveDir` to be right (`ve-plugin.ts` sets it) (**INFERRED**; spike did not exercise fonts).
- `cleanTheme` class on `<html>` is a build-time VE class: it comes from the FOUC step.

### 2.3 Theme-FOUC script

The script embeds a `themeMap` of build-time VE class names (`layout.tsx:41-44`). Options:
- **Prod (recommended):** inline `<script>` generated by `theme-map.ts`, injected at a `<!--THEME_BOOT-->` placeholder (design A: string replace on the output HTML; design B: in `html.ts`). `theme-map.ts` obtains the six class names from **the same plugin instance and compile cache** as the main bundle by building a throwaway `target:"bun"` entry that re-exports `src/styles/theme.css.ts`. Identifiers are deterministic per file scope, so a second independent evaluation would also match, but sharing the cache removes the question.
- **Guard test (cheap, catches drift):** assert every class in the emitted boot script appears in the emitted CSS; run in the build's post-step and in CI.
- **Dev:** Bun's HTML import cannot template inline scripts. Use `<script type="module" src="./src/theme-boot.ts">` first in `<head>`. Module scripts defer, so a brief theme flash is possible in dev only. This is a deliberate dev/prod parity gap; document it.

## 3. Navigation facade `@/lib/nav` and the boundaries model

### 3.1 What exists

- `src/lib/navigation/NavLink.tsx` (**VERIFIED**) wraps `next/link`, forces `prefetch={false}`, and lives in `lib/` on purpose: `shared -> ui` is disallowed by `.eslintrc.json` (`boundaries/dependencies`), `shared -> lib` is allowed.
- `src/components/ui/AppLink.tsx` is a one-line re-export of `NavLink` as `AppLink`.
- `src/lib/routes.ts:5` already holds `export const routes = {...}` path constants (**VERIFIED**; `DrawerNav.tsx:60` uses `routes.home`).
- The global jest stub is `jest.setup.js:107` (`jest.mock("next/navigation", ..., {virtual: true})`); **43** test files mock `next/navigation` or `next/link` directly (grep `jest.mock(['"]next/(navigation|link)` returned 43 test files; feasibility doc says 40).

### 3.2 Facade shape

```
src/lib/nav/
  index.ts          public API (the only import path allowed outside this dir)
  impl.types.ts     the contract (types only)
  impl.router.ts    react-router-backed implementation  (Bun build)
  impl.next.ts      next/navigation-backed implementation (Next build, deleted at cleanup)
  Link.tsx          was lib/navigation/NavLink.tsx (prefetch prop dropped)
  paths.ts          route path constants + RoutePath union (absorbs lib/routes.ts or re-exports it)
  normalize.ts      pathname normalisation (trailing slash)
  Redirect.tsx      <Redirect to replace/> (replaces next's redirect())
  RouteAnnouncer.tsx
  testing.tsx       mock helpers for jest
```

Exports, derived from the actual import counts above (not a generic router API): `useRouter(): {push, replace, back, forward}`, `useSearchParams(): URLSearchParams`, `usePathname(): string` (normalised), `Link`, `Redirect`. Optional additive helper later: `useSearchParam(key)` / `useQueryState` for the ~25 distinct keys, which is where TanStack's typed `search` was attractive; building it on the facade gets that benefit without that router.
Not exported: `redirect()`. Both call sites are redirect-only pages; in the route table they become `{path:"/config", redirect:"/settings?tab=config-files"}` and `{path:"/settings/defaults", redirect:"/settings"}`, so the facade needs no throwing `redirect`.

`AppLink` and `NavLink` keep their import paths as thin re-exports of `lib/nav` `Link` until the 24 `next/link` importers are moved, then are deleted (one codemod PR).

### 3.3 Where it sits in the boundaries lint model

Element map (**VERIFIED**, `.eslintrc.json`): `app -> everything`; `lib -> ui, sessions, gen` only; `ui/shared -> lib, gen`; `gen -> nothing`. Consequences:
- `src/lib/nav/**` is already element `lib`, importable from every other element. No new element and no `boundaries/dependencies` change.
- `lib/nav` **must not import `app`** (not in `lib`'s allow list). So the route table cannot live in `lib/nav`. Put it in `src/app/routes.tsx` (element `app`, allowed to import pages and layout). `lib/nav/paths.ts` is pure data, and `app/routes.tsx` consumes it with `satisfies Record<RoutePath, RouteDef>` so a missing route is a compile error (the inversion that keeps `lib` free of `app`).
- Enforcement uses **`no-restricted-imports`**, not the boundaries plugin (it classifies repo paths, and externals need extra config): ban `next`, `next/*`, `react-router`, `react-router-dom` everywhere except `src/lib/nav/**` and `*.test.*`/`*.spec.*`. Add as `error` in the same PR that finishes the codemod; there is no baseline mechanism, so the codemod PR must be complete.
- The `boundaries/ignore` list already excludes tests; the nav mock helper in `testing.tsx` is only imported from tests.
- Note on a latent stale thing: ADR-010's Step 1 ("RSC boundary", `docs/adr/010-frontend-modularity.md:40-44`) and ADR-009's constraint "compatible with Next.js 15 App Router" (`docs/adr/009-vanilla-extract-type-safe-css.md:31`) become moot or amended; the new bundler ADR should say so explicitly.

### 3.4 Tests

Move the global stub from `next/navigation` to `@/lib/nav` (same four hooks, same shapes), and codemod the 43 files (`jest.mock("next/navigation"` -> `jest.mock("@/lib/nav"`, `next/link` mocks -> render a plain `<a>`). Because tests mock the facade, they are independent of which `impl.*` is active; that independence is what allows the dual-build period. Use `ast-grep` for the codemod and keep it mechanical.

## 4. Router integration with the layouts and root shell

Router choice stays the owner's. The architecture below is router-agnostic via the facade; it is written for React Router 7 library mode because it is the leading recommendation. **TanStack Router is not a supply-chain-safe default given the May 2026 incident (requirements.md), and its typed-search benefit is obtainable through the facade helper above.** One caveat on RR: the feasibility doc §8 says RR7's typed routes are framework-mode only and that claim is **UNVERIFIED**; irrelevant if typing lives in `lib/nav/paths.ts`.

### 4.1 Route table

```ts
// src/app/routes.tsx  (element: app)
const routes: Record<RoutePath, RouteDef> = {
  "/settings/backlog-sources": { load: () => import("./settings/backlog-sources/page"), title: "Settings - Stapler Squad", guard: "backlog" },
  "/config": { redirect: "/settings?tab=config-files" },
  ...
};
```

- 42 entries (two of them, `/config` and `/settings/defaults`, are redirect-only pages). `title` copied from the 12 metadata layouts (child pages inherit the nearest layout's title in Next; the table makes that explicit).
- Guards (3): `backlog` (layout covers `/backlog` and `/backlog/board`), the `settings/backlog-sources` layout (same hook pattern; its flag name not read, UNVERIFIED), and `changelog`. Implement as a `<RequireFeatureFlag>` wrapper, which already exists in `FeatureFlagsContext`; the two bespoke layouts become the same wrapper. Note the two bespoke guards `router.replace("/")` after load and render `null` while loading, whereas `RequireFeatureFlag` behaviour must be checked so the swap does not change redirect-vs-blank semantics (**UNVERIFIED**; read `FeatureFlagsContext` in planning).
- `createBrowserRouter` (data router) with per-route `lazy`/`handle:{title}`. It is preferable to `<BrowserRouter>` because route-level `lazy` is the code-splitting seam (§6) and `handle` carries titles.

### 4.2 Root layout and providers

Extract the provider/chrome tree out of `app/layout.tsx` into `src/app/RootShell.tsx` **before** the router work, under Next (refactor-first, §10). Shape:

```
RootShell = ViewportProvider > ErrorBoundary > AuthProvider > Providers > CockpitShell
              > <a skip-link/> <main id="main-content"> TmuxVersionMismatchBanner ForkPressureStatusBanner {children}
              > TrayErrorBoundary > NotificationPanel
```

- Next build: `layout.tsx` = `<html>`/`<head>`/FOUC/fonts/metadata + `<body><RootShell>{children}</RootShell></body>`.
- Bun build: root route element = `<RootShell><Outlet/></RootShell>`, mounted **inside** `RouterProvider` because `PageViewTracker`, `NavigationProvider` etc. call `usePathname`. The root element persists across navigations, so `TerminalPoolProvider` (`Providers.tsx`, comment: "must live above PaneSplitRenderer's key=... remount boundary") and Redux/context state survive route changes exactly as under Next's persistent root layout.
- Sharing `RootShell` is what prevents provider-tree drift between the two builds in the dual period.
- `reactStrictMode: true` (`next.config.ts:13`) -> wrap in `<React.StrictMode>` in `main.tsx` so dev keeps double-effect checking (a no-op in production builds).
- Root `errorElement` = existing `ErrorBoundary` UI; data routers otherwise show RR's default error page when a `lazy` import rejects.
- `Providers.tsx` branches on `process.env.NODE_ENV === "production"` (HttpAnalyticsProvider vs Console). The Bun `define` must set `NODE_ENV` correctly in both prod and dev; the deployed build today is production-flavoured (unminified only), so this does not change.

### 4.3 Titles and metadata

React 19 hoists `<title>` rendered anywhere to `<head>` (**INFERRED** from React 19 docs; `react@^19` per spike `package.json`). Root shell renders `<title>{useMatches().at(-1)?.handle?.title ?? DEFAULT}</title>`. No head library. Verify in the router spike; fallback is a 6-line `useEffect(() => { document.title = ... })`.

### 4.4 Other Next-provided behaviours to replace (checklist for planning)

| Next feature | Replacement | Status |
|---|---|---|
| `next/dynamic` (6 `dynamic(` call sites: `InsightsDashboard.tsx` x4, `SessionDetail.tsx`, `SessionDetailView.tsx`; there may be more `ssr:false` options) | `lib/utils/lazy.ts` wrapping `React.lazy` + `Suspense` + `loading` option + chunk-error reload-once (spike shim: `spike: shims/dynamic.tsx`, 6 lines) | trivial |
| `useReportWebVitals` from `next/dist/client/web-vitals` (`WebVitalsReporter.tsx:4`) | `web-vitals` package (new dep: needs the recent-compromise check) or `PerformanceObserver` | small |
| Route announcer | `RouteAnnouncer` (see §0) | small |
| Scroll-to-top on navigation | **UNVERIFIED**: no `scrollTo`/`ScrollRestoration` in `app/` or `components/layout`. Check whether the scroll container is `window` or `<main>`; RR `<ScrollRestoration/>` only handles `window`. |
| `<Link prefetch>` | already forced `false` in NavLink; drop the prop |
| `eslint-config-next` (`next/core-web-vitals`) | see §9 |

## 5. Code-splitting boundaries

The spike's 45 JS files came from splitting only at the `dynamic()` sites, **because the spike statically imported all 42 pages into one entry** (`spike: entry.tsx`). So the spike shows nothing about route-level splitting, and the initial bundle was far from what we would ship. Target boundaries:

1. **Route level**: every route `load: () => import(page)` -> one chunk per page (Next did the same per `page.tsx`).
2. **Heavy-dependency isolation**, expected to fall out of (1) but worth asserting in a bundle-budget test: `@xterm/*` (terminal), `codemirror` + `@codemirror/lang-*` (9 languages), `@monaco-editor/react` + `monaco-editor` (only in `app/config/ConfigPageContent.tsx`, **VERIFIED**; make sure the Monaco worker/loader assets are not pulled into the initial chunk), `recharts`, `react-markdown`/`remark-gfm`, `react-virtuoso`, `react-arborist`.
3. **Existing `dynamic()` sites** keep their splits via the `lazy` helper.
4. **Initial JS** = entry + root shell + `Providers` tree + the first route. This is the number the new `size-limit` budget should guard.

Budget re-expression (`.size-limit.json` has `framework-*.js` and `main-*.js`, which Bun does not emit; **VERIFIED** file contents): (a) Total JS: glob `out/_next/static/**/*.js` <= 5 MB (unchanged, works if prefix kept); (b) Initial JS: computed from the HTML's `<script src>` + `modulepreload` links (`size-limit` supports explicit `path` arrays, or compute in `build.ts` stats and assert in a test); (c) Largest single chunk. Treat (b) as new, with a baseline from the first real build.

Risks: Bun's chunk naming/dedup quality across 42 lazy entries and **CSS chunk ordering** are untested (**UNVERIFIED**). `next.config.ts:48-69` currently forces all CSS into one `styles` chunk to avoid preload warnings and cascade surprises; VE output is mostly single-class so order matters only for `globalStyle`/theme blocks, but it has to be checked (check: diff computed styles on 5 representative routes between Next and Bun builds).

## 6. vanilla-extract Bun plugin and `bun patch`

### 6.1 Spike plugin (VERIFIED, read in full)

`spike: ve-plugin.ts` (~25 lines): `onResolve(virtualCssFileFilter)` -> `ve-css` namespace; `onLoad` for the namespace returns the virtual CSS as `loader:"css"` with `resolveDir`; `onLoad(cssFileFilter)` calls `compile({filePath, cwd, identOption})` then `processVanillaFile({source, filePath, identOption})` and returns JS. `identOption` = `"debug"` in dev, `"short"` in prod. `spike: bunfig.toml.dev` wires it through `[serve.static] plugins = [...]` for the dev server. It also carries a nested-`@keyframes` stripper that is no longer needed: PR #1097 is merged in this branch (`git log`: `a097988cf fix(web-app): use keyframes() instead of invalid nested @keyframes (#1097)`). Remove the stripper; keep the failure loud.

### 6.2 What the production plugin needs beyond the spike

| Gap | Design |
|---|---|
| **Dev dependency invalidation (INFERRED, important)** | `compile()` bundles a `.css.ts` and its imports (e.g. `theme-contract.css.ts`, shared tokens) with esbuild *inside* the plugin, so Bun's module graph does not know `Foo.css.ts` depends on `theme.css.ts`. The spike only edited the `.css.ts` that was itself imported (84 ms reload). Editing a shared token file would not invalidate dependents. Fix: memoise `{result, watchFiles, mtimes}` per file and re-run `compile` when any `watchFiles` entry's mtime changed (a `statSync` loop; no new watcher). Also gives free rebuild speed. |
| Cold compile cost | 336 files, ~2-2.8 s warm / 6.3 s cold from the spike (§2 of the feasibility doc). Prod build calls `compile` per file inside `onLoad`; the spike's one-pass build2 timing (2.7-3.0 s) already includes it. Peak RSS unmeasured; measure with `/usr/bin/time -v` (the repo had an OOM incident, memory index "backlog WIP limit"). |
| Identifier stability | Same plugin instance feeds the FOUC `theme-map` (§2.3). |
| Plugin instance sharing | One `createVanillaExtractPlugin()` factory used by `build.ts`, `dev.ts`/bunfig, and `theme-map.ts`; a single source of truth for `identOption`. |
| Fail loudly | Throw on any VE error, with the file path. Do not catch and emit empty CSS. |
| Strict CSS | Bun's CSS parser rejects invalid CSS that webpack/Rspack silently passed (the keyframes case). Run the full build early; any further failures are pre-existing bugs to fix at source, not suppress. |

### 6.3 The `eval` patch

Root cause (feasibility §3.1, spike-verified): `@vanilla-extract/integration` evaluates the esbuild bundle through the `eval` npm package (0.1.8) -> `require-like` -> `Module._load`, which returns `undefined` under Bun. `spike: eval-shim.cjs` (10 lines, `vm.Script` + `createRequire`) fixes it: 336/336 files, output identical to Node. `spike: eval.js.orig` shows the spike simply overwrote `node_modules/eval/eval.js`.

Mechanics:
1. `bun patch eval@0.1.8` -> replace the body of `eval.js` with the shim -> `bun patch --commit` -> `web-app/patches/eval@0.1.8.patch` plus `patchedDependencies` in `web-app/package.json`.
2. The build script runs inside Bun, and `integration` requires `eval` with Node-style resolution at runtime. **Bun.build plugins cannot intercept that** (`onResolve` only affects what the bundler resolves, not what the build script's own process loads). That is why a patch (or override) is required, and a plugin-level alias will not work (**INFERRED**; one-line spike check).
3. **Authoring hazard:** `bun patch` edits files under `node_modules`. In this repo `web-app/node_modules` is a symlink to `~/.stapler-squad/shared-node-modules/<hash>` shared by every worktree with the same hash (**VERIFIED**, `scripts/shared-node-modules.sh` header). Author the patch with `STAPLER_SQUAD_SHARED_NODE_MODULES=0` so the shared tree is not mutated for other worktrees. Also: `compute_hash` hashes `pnpm-lock.yaml` plus `file:` deps only (**VERIFIED**, lines 61-73 of the script); it must also hash `patches/**` or a worktree that changes the patch keeps serving a stale shared tree (the exact bug class the script header already documents for `file:` deps).
4. **Alternatives, in preference order:** (a) `bun patch` (owner's choice); (b) an `overrides` entry pointing `eval` at a ~10-line local package under `web-app/vendor/eval-shim` (more transparent than a diff, survives lockfile churn, needs the same shared-hash treatment); (c) escape hatch: run only the VE compile step in a Node subprocess (the feasibility doc measured the Node prepass at 2.1-2.8 s), requiring Node at build time, which `.tool-versions` already provides.
5. **Drift guard:** a smoke test (`build/__tests__/ve-eval-patch.test.ts`, run in `make ready` and CI) that evaluates a tiny canary `.css.ts` through the plugin and fails with a message pointing at `patches/`. Without it, a VE upgrade that changes the `eval` version silently drops the patch and the failure surfaces as a cryptic `setAdapter of undefined`. Pin `@vanilla-extract/integration` and `eval`; upstream the fix (nice-to-have).
6. Bun does not run lifecycle scripts unless `trustedDependencies` lists the package. `compile()` needs a working `esbuild` binary; whether `esbuild` and `@vanilla-extract/css` need trusting is **UNVERIFIED** (feasibility §9 says "not tested"). Spike gate.

## 7. Dev vs prod parity and `bun dev`

| Aspect | Dev (`bun dev`) | Prod (`build.ts`) | Parity risk |
|---|---|---|---|
| Entry | `Bun.serve({routes:{"/*": index}, development:{hmr:true, console:true}})` (spike `devtest/server.ts`) | same `index.html`, design A | low if design A works |
| VE idents | `debug` | `short` | e2e conventions already ban CSS class selectors (`CLAUDE.md` e2e conventions), so low |
| Minify | off | on (new; today's deployed bundle is unminified, `NEXT_BUILD_MODE=development`) | **medium**: minification is a behaviour change vs what ships today. Look for `constructor.name`/`Function.name` reliance (**INFERRED**, not searched); consider `keepNames` if Bun supports it (**UNVERIFIED**). `WEB_MINIFY=0` retains the old debuggable output for incident work. |
| Theme boot | module script (flash possible) | inline | accepted gap |
| Static `public/` | must be served explicitly: Bun HTML routes do not serve it. Generate per-file routes from `readdirSync("public")` (`/push-sw.js`, `/manifest.json`, icons). | copied at build | low |
| Fast Refresh | **UNVERIFIED**: the spike validated CSS reload only, not React state-preserving refresh | n/a | gate in spike S2 |
| StrictMode | on | on (no-op in prod) | none |
| Source maps | inline/linked | linked | `.map` files are embedded in the Go binary today (`productionBrowserSourceMaps: true`, `next.config.ts:11`); keep parity or drop deliberately (binary size) |

### 7.1 Reaching the Go backend

Today: `next dev` on another origin; `getApiBaseUrl()` returns `process.env.NEXT_PUBLIC_API_URL` when set (`src/lib/config.ts:15-17`, **VERIFIED**); `scripts/dev-stack/launch.ts:215-223` spawns `next dev` with `NEXT_PUBLIC_API_URL`, and the backend gets `STAPLER_SQUAD_EXTRA_ORIGINS=http://localhost:<frontendPort>` for CORS (`launch.ts:205`). So it is cross-origin by design.

Recommendation: **keep that model** (Option A). `bun dev` reads `NEXT_PUBLIC_API_URL` through `define`; only the spawn command in `launch.ts` and the `webServer.command` in `web-app/playwright.config.ts:82` change. Same cookies/WebAuthn behaviour as today, nothing new to debug.
Rejected for now: (B) same-origin proxy in `Bun.serve` `fetch` for `/api`, `/auth`, `/health`: nicer but must also proxy WebSocket upgrades (`src/lib/transport/watch-ws-transport.ts`) and streaming ConnectRPC bodies; real code with new failure modes. Can be a later improvement.
Port discipline: the dev server port must come from the manual block (62871+) or `findFreePort()`, never a fixed 3001 default for agent sessions (`web-app/package.json` dev script defaults `${PORT:-3001}`); keep `PORT` honoured.

## 8. Dual-build cutover seam (`WEB_BUILD=next|bun`)

**Contract between web and Go: the directory `web-app/out/` containing `index.html` + assets + `public/**`.** Go sees nothing else (`server/web/embed.go`, `static.go`, `auth.go`). Everything below protects that contract.

```make
WEB_BUILD ?= next          # flipped to bun in the cutover PR; deleted in cleanup
web-app/.web-build-mode: FORCE    # rewritten only when the value changed (cmp-then-mv)
web-app/out: ... web-app/.web-build-mode
	@$(WEB_BUILD_CMD_$(WEB_BUILD))
```

Design points:
1. **Mode stamp.** Without it, flipping `WEB_BUILD` changes no mtime and `make` reuses the other mode's `out/` and `server/web/dist` (stale-mode bug). The stamp lives outside `out/` (so it is not embedded).
2. **Single output dir.** Both modes write `web-app/out`; the Bun path builds in a temp dir and swaps (§1). All consumers (size-limit, lighthouse, e2e, `cp -r` to `dist`, `.gitignore`) stay unchanged.
3. **Source compiles under both.** Differences are confined to: the entry (`layout.tsx` vs `main.tsx`+`routes.tsx`) and `lib/nav/impl.*`. Selection: `next.config.ts` webpack `resolve.alias["@/lib/nav/impl"] -> impl.next.ts`; Bun plugin `onResolve` -> `impl.router.ts`; `tsc` checks both against `impl.types.ts`. `RootShell` and every page are shared.
4. **Unmigrated-import guard.** The spike *shimmed* `next/*` (`spike: build2.ts`), which would hide leftover imports. The production Bun build must **fail** on any resolution of `next` or `next/*` (plugin `onResolve` throws). Together with the `no-restricted-imports` error, a half-migrated file cannot ship.
5. **CI matrix** until cutover: `web-build-smoke` (`.github/workflows/build.yml:123-187`) and the Playwright e2e job run for `WEB_BUILD in {next, bun}`; jest runs once (bundler-independent). The `Cache Next.js build cache` steps (`build.yml:177-183`, `.github/actions/prepare/action.yml:59-66`, four more in `benchmark.yml`) stay for `next`, and no cache is added for `bun` (no persistent build cache to poison).
6. **Rollback** = flip the Makefile default back (and CI default). Stays one-line as long as Next code and deps remain; the cleanup PR (delete `next`, `eslint-config-next`, `@storybook/nextjs`, `layout.tsx` html shell, `impl.next.ts`, aliases) happens only after the soak window. Tag the last commit that has both builds.
7. **Makefile guard** `test -d web-app/node_modules/next` (`Makefile:221`) must become mode-aware (it would fail or lie in bun mode).

## 9. Tooling edges that touch this architecture (not owned by this dimension, listed so planning sees them)

- ESLint: `.eslintrc.json` `"extends": ["next/core-web-vitals"]` (**VERIFIED**). Dropping `eslint-config-next` also drops `react-hooks/*` and `@next/next/*` rules; keep `eslint-plugin-react-hooks` explicitly or hook-dependency lint silently disappears. `lint.yml:277-379` and `package.json` use `next lint`, which exits non-zero on any warning; replacing it with raw `eslint` changes that warning behaviour, so decide the `--max-warnings` policy deliberately.
- Playwright: `web-app/playwright.config.ts:82` `npx next dev`; `tests/e2e/global-setup.dev-mode.ts`, `playwright.dev-mode.config.ts`, `scripts/dev-stack/launch.ts:215` all spawn `next dev`; `Makefile` `dev-stack` target text. Main e2e (`tests/e2e/global-setup.ts`) runs the Go binary with embedded `dist`, so it exercises the built output and must be run in both modes during the dual period.
- `.github/workflows/lint.yml`, `ux-analysis.yml` (runs `./stapler-squad`, so it exercises the embedded build), Lighthouse (`npx serve@14 out`).
- Docs/CLAUDE.md/`server/web/embed.go:12` comment referencing "Next.js static export".

## 10. Tech-debt disposition: router / navigation area

**Disposition: Isolate via seam, preceded by a small Refactor-first step.**

- *Why not Extend as-is:* 63 production files and 43 test files import `next/*` directly; swapping the router in place would touch ~106 files in one change with no behavioural net, and the one existing abstraction (`NavLink`) covers only links. Extending as-is means the router decision and the bundler decision are welded together.
- *Why not a broad Refactor-first:* the code is not unhealthy beyond the coupling. 12 of 16 layouts are pass-through metadata, router API usage is narrow (4 hooks + link), `routes.ts` and `NavLink` already point the right way. Rewriting pages first would be gold-plating.
- *Why Isolate via seam:* the facade (§3) is the seam; the `impl.next.ts`/`impl.router.ts` pair makes the swap a one-file change per build mode, keeps the Next build green and revertible while the codemod lands, and lets tests become router-independent.
- *The small Refactor-first step* (under Next, behaviour-neutral, own PR): extract `RootShell` from `layout.tsx` (§4.2), pull layout `metadata` titles into a `routes`-style table, and add characterisation tests for pathname/trailing-slash and for the 2 flag-guard layouts. These remove the two things that would otherwise leak Next specifics into the new entry.
- *Debt deliberately not paid:* `OmnibarContext` importing a `sessions` component (`project_plans/frontend-architecture-audit/research/state-management.md:45`) and the cross-domain imports from ADR-010's baseline stay out of scope; they are orthogonal.

## 11. Failure modes specific to the migration (Complexity 4)

Severity: H/M/L = likelihood x impact as I judge it. Detection = what would catch it before users do.

| # | Failure mode | Trigger | Detection | Mitigation / rollback |
|---|---|---|---|---|
| F1 H | **Partial migration** (files left on `next/*`) | codemod misses dynamic import, `require`, storybook/test-utils | Bun build throws on any `next*` resolution (§8.4); `no-restricted-imports` error | Both guards in the same PR as the codemod; never shim `next/*` in production plugins |
| F2 H | **Dual-build drift** (code works in one mode only) | feature PR lands during the dual period using a Next-only API (e.g. `next/image`, `metadata`) | CI matrix builds + e2e in both modes; `no-restricted-imports` blocks new `next/*` | Shorten the dual period; freeze `layout.tsx` metadata additions |
| F3 H | **Stale-mode artifact** (Makefile reuses other mode's `out`/`dist`) | flip `WEB_BUILD` without a file change | mode stamp (§8.1); CI always cold | Stamp file + FORCE rule |
| F4 H | **Login lockout from asset prefix** | assets emitted outside an `auth.go` exempt prefix | an e2e/`curl` test: unauthenticated GET of `/login` and its script URL returns 200 with JS | keep `/_next/` prefix until cleanup; add a Go test enumerating built asset URLs against `isExempt` |
| F5 M | **HTML-as-JS from SPA fallback** after deploy/restart | tab holds old `index.html`, requests deleted hashed chunk; Go returns index.html 200 | console `Unexpected token <`; Go test for 404 under asset prefix | Go 404 on asset-prefix miss + reload-once on chunk error |
| F6 M | **Theme/FOUC mismatch** | VE identifiers in boot script differ from emitted CSS | post-build guard test (§2.3) | shared plugin cache; run the guard in `make ready` |
| F7 M | **Dev invalidation gap** (token edits do not refresh dependents) | `.css.ts` imports a shared `.css.ts`/`.ts` | manual: edit `theme-contract.css.ts` with page open | mtime memo (§6.2); fallback: `bun --watch` full reload |
| F8 M | **Minification regression** (first minified deploy) | name-dependent code, side-effectful module order | full e2e on the minified build; `WEB_MINIFY=0` is the diagnostic | ship minified only after the e2e pass; retain the env switch |
| F9 M | **Router behaviour drift**: trailing slash, scroll, `replace` vs `push` semantics, announcer, guard blank-vs-redirect | swap | characterisation tests written under Next *before* the swap (§10); 24+ e2e `toHaveURL` assertions | the Refactor-first step produces them |
| F10 L | Stale-tab skew across a binary upgrade | any release | F5 handling | accepted, same as Next today |
| F11 H | **Shared `node_modules` poisoning** | `compute_hash` omits `patches/**`; or `bun patch` authored in the shared tree | a worktree sees an unpatched or foreign-patched VE (`setAdapter of undefined`) | add `patches/**` + `bun.lock` to the hash; author patches with `STAPLER_SQUAD_SHARED_NODE_MODULES=0`; the canary test (§6.3.5) |
| F12 M | **CI cache poisoning / staleness** | `restore-keys` fallbacks (`build.yml:182`, `prepare/action.yml:64-66`) restore a cache from a different lockfile; a cache of `node_modules` restored without the patch | cache keys include `bun.lock` + `patches/**` hash; no `restore-keys` prefix fallback for `node_modules`; no build-output cache for the Bun path; caches from `main` only | pin `oven-sh/setup-bun` by SHA (repo uses `gha-pin-actions` convention, `build.yml:150` pins `pnpm/action-setup` by SHA) and pin the Bun version in `.tool-versions` |
| F13 M | **Next build on a Bun-installed tree** (needed for the dual period) | pnpm -> Bun PR merges first, and `next build`, Storybook, Jest were "not tested" on a Bun-installed `node_modules` (feasibility §9) | first spike gate S0: `WEB_BUILD=next make web-build` + `jest` on the Bun tree | if it fails, the dual-build period is not viable and cutover must be a single PR with a revert plan |
| F14 M | **Worktree concurrency** | N agent worktrees running `make web-build` at once (the repo already hit an OOM on 2026-07-12 per memory) | peak RSS and wall time under 2-3 concurrent builds | build into per-process temp dir + atomic rename (§1); no shared writable cache unless content-hashed and atomically written (Next's shared webpack cache precedent, `next.config.ts:36-55`); measure Bun+VE peak RSS; dev ports from the 62871 block |
| F15 M | **HMR/Fast Refresh not as good as spiked** | stateful components (terminal, panes) | spike S2 on 3 stateful pages | fall back to full-page live reload; do not block cutover on HMR if build wins hold |
| F16 L | **Bun bundler bug / crash** | young bundler | CI | escape hatches: Vite 8 + official VE plugin (5.75-6.25 s) or Rsbuild, both already benchmarked; the facade/RootShell/route-table work is bundler-independent, so it is not wasted |
| F17 L | **Source maps embedded in binary** grow `stapler-squad` | Bun maps are bigger/smaller than Next | binary size diff in CI | decide explicitly; keep parity first |

Cross-cutting rollback ladder: (1) Makefile default flip; (2) revert the cutover PR; (3) after cleanup, revert the cleanup commit range (restorable because the tagged commit has both builds). No data/API changes exist, so there is nothing to migrate back.

## 12. Spike gates the plan should schedule first (cheapest way to remove UNVERIFIED items)

| Gate | Question | Ends |
|---|---|---|
| S0 | Does `next build`, `jest` and Storybook still work on a Bun-installed tree? Do `esbuild`/`@vanilla-extract/css` need `trustedDependencies`? | decides if dual-build is viable (F13) |
| S1 | HTML-entry `Bun.build` + `splitting` + plugins + `publicPath:"/"` + `naming` prefix + woff2 `url()`; load a nested deep link | design A vs B (§2.1) |
| S2 | Real app in `bun dev`: Fast Refresh on a stateful page, token-file edit invalidation, public/ serving, API via `NEXT_PUBLIC_API_URL` against an isolated instance (62871 block) | §7 |
| S3 | All 42 routes lazy + CSS ordering diff + initial-JS baseline + peak RSS | §5, F14 |
| S4 | Router spike: trailing-slash matching, `<title>` hoisting, scroll container, guard semantics | §4 |

## 13. Open items for the owner

1. Router (RR7 library mode recommended behind the facade; TanStack not recommended on supply-chain grounds plus the facade covers typed search).
2. Keep `/_next/static/` as the asset prefix through soak, rename in cleanup (recommended), or rename at cutover.
3. Ship minified at cutover (requirements say yes) or after one soak cycle on unminified Bun output.
4. Is the `Total JS 5 MB` budget enough, or should the initial-JS budget be the gate now that routes are lazy?
