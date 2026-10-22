# Research: Stack (libraries, versions, patterns) for a Bun.build SPA of web-app

Date: 2026-10-19. Host: Linux, Bun 1.4.3 (installed), npm registry queried the same day.
Labels: **VERIFIED** = I ran it or opened the source this session. **INFERRED** = reasoned, not run.
Scratch experiments lived in the session scratchpad (`.../scratchpad/stackres/`), not in the repo.

## 0. Corrections to earlier assumptions (read first)

1. **Storybook is already off `@storybook/nextjs`.** VERIFIED: `web-app/.storybook/main.ts` uses `@storybook/react-webpack5` 8.6.18 with `useSWC`, a babel-loader rule and a `.css.ts` mock loader (`.storybook/css-ts-mock-loader.js`). `@storybook/nextjs` appears only in `package.json` devDependencies (`grep -rln "storybook/nextjs"` hits `package.json`, `main.ts` comment, mock loader comment). Removing it is a one-line dependency delete, not a migration.
2. **The legacy ESLint config is `.eslintrc.json` run via `next lint`.** VERIFIED (`web-app/.eslintrc.json`, `.github/workflows/lint.yml:335-381`). CI uses `npx next lint --file ...` and relies on `next lint` exiting 1 on any warning. Replacing it means replacing that exit-code behaviour (`eslint --max-warnings 0`) too.
3. **React in the lockfile is 19.1.1** (`pnpm-lock.yaml`: `react@19.1.1`). This matters for React Router 8 (section 4).
4. **`@vanilla-extract/integration` 8.0.11 depends on `@vanilla-extract/css ^1.21.3`** (VERIFIED, `npm view`), but `web-app` pins `^1.20.1` (lockfile may resolve to 1.20.x). The build plugin needs `@vanilla-extract/integration` as a direct, pinned devDependency (today it only arrives through `@vanilla-extract/next-plugin`), and `@vanilla-extract/css` should move to >=1.21.3 in the same change.

## 1. Bun version pinning

- Installed and tested: **Bun 1.4.3** (`bun --version`; `npm view bun dist-tags.latest` = 1.4.3; `@types/bun` 1.4.3). VERIFIED.
- Pin in three places so local, CI and worktree sessions agree: `package.json` `"packageManager": "bun@1.4.3"` (and `engines.bun`), `.bun-version` (read by `oven-sh/setup-bun@v2`'s `bun-version-file`), and the Brewfile/bootstrap entry. INFERRED (setup-bun input names not re-checked this session). The in-flight pnpm to Bun PR likely already chooses the pin; align with it rather than diverge.
- Treat a Bun bump as a build-affecting change: the bundler, CSS parser and HTML pipeline are all inside the runtime. Re-run the size and e2e gate on every bump.

## 2. `Bun.build` details needed

Source: https://bun.com/docs/bundler and https://bun.com/docs/bundler/fullstack (opened), plus experiments below.

### 2.1 Entry: HTML entry vs script entry

- **HTML entry works with plugins and code splitting.** VERIFIED: `Bun.build({ entrypoints: ["./src/index.html"], outdir, splitting: true, minify: true, sourcemap: "linked", target: "browser", publicPath: "/", define, plugins: [vanillaExtract()] })` built in 58-62 ms for a small app and produced `index.html` with a hashed `<script type="module" crossorigin>`, a hashed `<link rel="stylesheet">` and a `<link rel="modulepreload">` for the shared chunk; a dynamic `import()` became its own chunk (`lazy-*.js`). `<title>` and other head tags in the HTML were preserved.
- Consequence: a **single HTML entry is the right default** (one `index.html`, Go's SPA fallback in `server/middleware/static.go` already serves it). Per-route HTML is not needed for the router options below because they are client-side routers.
- **`naming.entry` does not give a stable JS name when the entry is HTML**: the entry JS came out as `assets/index-<hash>.js` (hashed). Fine for the Go embed; matters only for `size-limit` globs (section 9) and for any Playwright/Lighthouse config that referenced `_next/static/chunks/*` (open question in requirements).
- Script entry (`entrypoints: ["./entry.tsx"]`, as in the spike `build2.ts`) gives a stable `[name].[ext]` entry but then **we own `index.html` generation** (theme-FOUC script, `<title>`, viewport meta, CSS link, modulepreload). HTML entry is less code. The one thing HTML entry cannot do is inject the build-time `themeMap` into the inline FOUC script (it needs the class strings exported by `theme.css.ts`). Options: (a) a post-build step that reads the compiled theme class names and string-replaces a placeholder in `index.html`; (b) move the FOUC logic to read class names from a tiny generated `theme-map.js` loaded synchronously with `<script src>` (adds a request, but fully static); (c) generate `index.html` ourselves from a template using the same VE `compile()` results. This is a plan-phase decision; (a) is the smallest.

### 2.2 Options table (from the docs page, VERIFIED reading; those marked * also exercised)

| Option | Use here |
|---|---|
| `splitting: true`* | Needed for `next/dynamic` replacements (`React.lazy` + `import()`) and route-level lazy loading. `modulePreload` (default true) adds `<link rel=modulepreload>` to the HTML. `minChunkSize` (default 0) can fold tiny chunks; the spike had 45 chunks, tune later. |
| `minify: true`* | Whitespace + identifiers + syntax. Needed because today's deployed bundle is unminified. Keep a `WEB_BUILD_MODE=development` switch that sets `minify: false` and `sourcemap: "inline"` to replace `NEXT_BUILD_MODE=development`. |
| `sourcemap: "linked"`* | Writes `*.js.map` and a `sourceMappingURL` comment; requires `outdir`. Production Next currently ships `productionBrowserSourceMaps: true`, so keep `linked` for parity. Maps are large (787 KB map for a 176 KB bundle in the toy), so exclude `*.map` from size-limit and decide whether the Go embed should include them (embed size). |
| `define`* | String-literal map. Must cover every `process.env.X` the client reads. Used keys in `src/` (VERIFIED by grep): `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_RECONNECT_V*` (several suffixes in the grep output were truncated by my regex; enumerate exactly with `grep -rhoE "NEXT_PUBLIC_[A-Z0-9_]+" src | sort -u` in the plan phase), `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING`, `NEXT_PUBLIC_OTEL_ENABLED`, `NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`. Plus `process.env.NODE_ENV`. A `define` of `"process.env.NEXT_PUBLIC_X": '"hi"'` was inlined (VERIFIED). Unset values: define as `"undefined"` as the spike did. Alternative: `env: "NEXT_PUBLIC_*"` (docs: inlines matching env vars at build time). I did not test `env`; `define` is explicit and testable, so prefer it. Dev server: `[serve.static] env = "NEXT_PUBLIC_*"` (docs) or the same `define` through `Bun.build` if we build ourselves. |
| `naming`* | Object form `{entry, chunk, asset}` with tokens `[dir] [name] [ext] [hash]`. Use `chunk: "assets/[name]-[hash].[ext]"`, `asset: "assets/[name]-[hash].[ext]"` to give a single stable directory (replaces `_next/static/`). |
| `publicPath`* | `"/"` yields absolute `/assets/...` URLs in HTML. Correct for Go serving at the root; keep `trailingSlash`-style deep links working because the URLs are root-absolute. |
| `loader` | Map of extension to loader. **See 2.4: it did not change CSS `url()` handling.** |
| `external`* | Works for absolute font URLs in CSS (2.4). |
| `metafile` | `true`/path/`{json, markdown}` per docs (not run). Candidate for emitting the CI build-size observability record the requirements ask for; confirm shape in the plan phase. |
| `drop`, `banner`, `footer`, `packages`, `root`, `files` | Not needed. `drop: ["console"]` exists if we ever want `removeConsole`. |

### 2.3 Plugins: `Bun.build` vs `bunfig.toml [serve.static]`

- `Bun.build({ plugins })` takes `BunPlugin` objects directly. VERIFIED working with the HTML entry (the VE plugin transformed `.css.ts` files inside an HTML-entry build).
- `Bun.serve({ routes: { "/*": index } })` bundles the imported HTML itself; its plugins come only from `bunfig.toml`: `[serve.static] plugins = ["./devtest/plugins.ts"]` (docs; the spike's `bunfig.toml.dev` + `devtest/plugins.ts` do exactly this and the spike reports the HMR result: 84 ms `.css.ts` reload). The docs say plugin entries there are a module path or package name; the module's default export is the plugin object. The docs also note plugins work in the `Bun.build()` JS API but "not yet" in the CLI (`bun build` command): **do not use the `bun build` CLI**; use a `build.ts` script run with `bun run`.
- Plan consequence: one shared plugin module (`web-app/build/plugins.ts`) imported by both `build.ts` (production) and referenced from `bunfig.toml [serve.static]` (dev). Watch for drift: two entry points, one plugin set.
- `development: { hmr: true, console: true }` on `Bun.serve` gives HMR + browser console echo. `development: true` re-bundles the HTML per request and disables minification (docs).
- **SPA fallback in dev:** the docs page does not show a catch-all route; the spike used `routes: { "/*": index }` which VERIFIED serves the HTML for any path (the spike devtest ran). API proxy to the Go backend (ConnectRPC `/api`) is **not covered by the docs**: implement in `fetch(req)` fallback with `fetch(goOrigin + url.pathname, ...)`, streaming bodies and WebSocket upgrade (terminal streaming uses ConnectRPC streaming; WS proxying needs `server.upgrade` handling). INFERRED, unvalidated. This is the largest dev-server unknown after Fast Refresh.

### 2.4 Assets: woff2, svg, CSS (experiments VERIFIED)

- **woff2 referenced by `url()` in a plain CSS file linked from the HTML is inlined as a `data:font/woff2;base64,...` URL**, even for a 40 KB file, in Bun 1.4.3. Setting `loader: { ".woff2": "file" }` did **not** change that for CSS `url()`. The repo's vendored fonts total 194,364 B (`src/app/fonts/*/*.woff2`), about 259 KB as base64 inside CSS, which then blocks first render and is re-downloaded whenever the CSS changes.
- **Absolute `url("/fonts/x.woff2")` fails to resolve** (`Could not resolve: "/fonts/f.woff2"`) unless marked external. With `external: ["/fonts/*"]` the URL is left as-is (VERIFIED). So the clean pattern is: put fonts in a static dir copied verbatim to `out/fonts/` (the build script copies `public/` and `src/app/fonts` to `out/`), write `@font-face { src: url("/fonts/...") }` in `globals.css`, and pass `external: ["/fonts/*"]`. Alternative: accept inlining (smaller moving parts, +259 KB CSS) and measure FCP.
- **`url()` relative paths inside a vanilla-extract virtual CSS file fail to resolve** (`globalFontFace(..., { src: "url(./ve.woff2)" })` gives `Could not resolve`) with the spike's plugin as written (`resolveDir` returned for the `ve-css` namespace did not make relative `url()` resolve). The app does not currently declare fonts in `.css.ts` (fonts come from `next/font/local`), so this only constrains where `@font-face` goes: plain `globals.css`, not `globalFontFace`. Worth one more look if any `.css.ts` references `url(...)`; `grep -rn "url(" src --include=*.css.ts` in plan phase.
- `.svg`: the docs say unrecognized extensions are emitted as hashed assets and the import resolves to the path (and `logo.svg` in `public/` is referenced by URL, not imported). INFERRED that imported SVGs behave the same; `public/` files (`logo.svg`, `manifest.json`, `push-sw.js`, `icons/`) must be copied to `out/` by the build script (Next did this automatically). Service worker `push-sw.js` must keep its exact URL (no hash).
- `.css`: bundled into one hashed CSS per entry chunk graph (docs: "bundled into a single .css file"). The spike produced 626 KB CSS. Next's `next.config.ts` CSS consolidation hack (single `styles` chunk) is therefore unnecessary. Bun's CSS parser is strict: it rejected the invalid nested `@keyframes` (fixed in #1097) per the feasibility doc, so expect other strict-parse failures to surface at build time (a feature, with a cost).

### 2.5 Dev vs production switch

- Use one `build.ts` taking `--mode=development|production` (replaces `NEXT_BUILD_MODE`). Development: `minify: false`, `sourcemap: "inline"` or `linked`, `define NODE_ENV="development"` (React dev build gives readable errors, which the current `make` target uses development mode for). Production: `NODE_ENV="production"`, minified. The size budget applies to production only.

## 3. `bun patch` for the vanilla-extract `eval` dependency

Source: https://bun.com/docs/install/patch (opened) and a hands-on run (VERIFIED, Bun 1.4.3, project with `@vanilla-extract/integration@8.0.11`).

- Dependency chain: `@vanilla-extract/integration@8.0.11` depends on `eval@0.1.8` (VERIFIED `npm view ... dependencies`; `eval` is a **transitive** dependency of the repo).
- Flow that worked:
  1. `bun patch eval` (prepares `node_modules/eval`).
  2. Overwrite `node_modules/eval/eval.js` with the 14-line `vm` + `createRequire` shim (`eval-shim.cjs` from the spike; it keeps the original CommonJS export `module.exports = function (content, filename, scope, includeGlobals)`).
  3. `bun patch --commit node_modules/eval` wrote `patches/eval@0.1.8.patch`, added `"patchedDependencies": { "eval@0.1.8": "patches/eval@0.1.8.patch" }` to the **root** `package.json`, and recorded it in `bun.lock` (`grep patch bun.lock`).
  4. `rm -rf node_modules && bun install --frozen-lockfile` re-applied the patch (`diff node_modules/eval/eval.js eval-shim.cjs` identical).
- **Transitive packages patch fine** (the docs page was silent; the run shows it works) and the key is `name@version`, so a vanilla-extract bump that moves `eval` to another version silently drops the patch (the entry would no longer match); add a CI check that `node_modules/eval/eval.js` contains the shim marker (e.g. `createRequire`), or assert in `build.ts` that `require("eval")` returns a function that can `require("node:path")`.
- **Docs: Bun reads `patchedDependencies` from the root `package.json` only** and warns when a workspace package declares it (docs). If web-app is a workspace member of a Bun workspace after the package-manager PR, the entry belongs in the workspace root, not `web-app/package.json`. If web-app remains its own root, it stays in `web-app/package.json`.
- **Gotcha I hit (VERIFIED):** on the first attempt I wrote the shim into `node_modules/eval/eval.js` and `bun patch --commit` reported "No changes detected" while the **global Bun cache** (`~/.bun/install/cache/eval@0.1.8@@@1/eval.js`) had been overwritten through a hardlink (`stat` showed link count 2 before `bun patch`; 1 after). I removed that cache entry and reinstalled; the original file was restored (`diff` against `eval.js.orig` clean) and the second, correct attempt left the cache untouched. Rule for the runbook: always run `bun patch <pkg>` immediately before editing, then check `stat -c %h` is 1, never edit `node_modules` of a hardlink-installed package directly. Hardlinked installs are the default here (`bun install` backend; feasibility doc section 9 says both pnpm and Bun hardlink into a cache).
- **Aliasing via plugin is not a substitute.** The spike found `Bun.plugin` `onResolve` did not intercept a CJS `require` inside `node_modules` (feasibility doc / task brief). I did not re-test that this session. The patch is required.
- Alternatives to consider and reject: (a) vendor `eval` under `build/vendor/eval.cjs` and use `package.json` `overrides`/`resolutions` to point `eval` at a `file:` copy (Bun supports `overrides`; not tested; cleaner diff than a binary-ish patch but still pinned). (b) Skip `processVanillaFile`'s `eval` by running the compile in Node (`node ve-prepass.mjs`): removes Bun from the CSS path but re-adds a Node dependency and a second process; the requirements chose the patch. Upstream: `eval@0.1.8` is the dependency; the vanilla-extract fix would be replacing `eval` in `packages/integration/src/processVanillaFile.ts`; no issue search was done (feasibility doc also did not). Open one at https://github.com/vanilla-extract-css/vanilla-extract/issues when shipping.
- Verified end to end: with the patch active, the VE plugin (`ve-plugin.ts` from the spike, using `compile`, `processVanillaFile`, `getSourceFromVirtualCssFile`, `virtualCssFileFilter`, `cssFileFilter`) built a `.css.ts` file inside a `Bun.build` run (output CSS contained the class).

## 4. Router candidates (exact versions, registry state 2026-10-19)

All VERIFIED by `npm view` unless noted.

### 4.1 React Router
- `react-router` dist-tags: **latest = 8.4.0** (published 2026-09-15; 8.0.0 published 2026-06-17), **version-7 = 7.18.4** (published 2026-09-15), version-6 = 6.30.6.
- **8.4.0 requires `react`/`react-dom` `>=19.2.7` and Node `>=22.22.0`** (peerDependencies/engines). The repo's lockfile has **React 19.1.1**, so adopting 8.x forces a React bump. 8.4.0 dependencies: `cookie-es`, `@remix-run/route-pattern`.
- **7.18.4 peers `react >=18`, Node `>=20`**, dependencies `cookie`, `set-cookie-parser`. No postinstall scripts, no `optionalDependencies` (VERIFIED, empty).
- Library modes: Declarative (`<BrowserRouter>`), Data (`createBrowserRouter` + `<RouterProvider>` from `react-router/dom`). Install page (https://reactrouter.com/start/library/installation, https://reactrouter.com/start/data/installation) shows any bundler works; package is `react-router` (not `react-router-dom`) in 8.x docs. I did not verify `ScrollRestoration`, `useSearchParams`, or route `lazy` in these pages (the fetched content did not cover them); `useSearchParams` and `ScrollRestoration` are long-standing in 7.x INFERRED.
- Recommendation (INFERRED): **pin `react-router@7.18.4` exactly** (works on current React 19.1.1, no peer bump, API surface the facade needs is stable) unless the owner wants to take a React 19.2.7 bump in the same project; then 8.4.0 is the supported line. Decide in the ADR. Either way pin exactly (no caret) and keep `bun.lock` committed.
- Search params: React Router has no typed search params. The facade (`@/lib/nav`) should expose typed helpers (`useQueryParam('key', parser)`) built on `useSearchParams`; 22 uses / ~25 keys is the cost either way.

### 4.2 TanStack Router
- Latest `@tanstack/react-router` = **1.170.45** (2026-10-16). Peers `react >=18`. Dependencies: `isbot ^5.1.22`, `@tanstack/history 1.162.5`, `@tanstack/react-store ^0.11.2`, `@tanstack/router-core 1.171.38`. Provenance attestation present (`dist.attestations.provenance` SLSA v1). No `optionalDependencies` entry on the latest 1.170.45 / `router-core` 1.171.38 / `router-cli` / `router-generator` / `history` / `react-store` (VERIFIED empty).
- **May 2026 compromise** (GHSA-g7cv-rxg3-hmpx, opened https://github.com/TanStack/router/security/advisories/GHSA-g7cv-rxg3-hmpx; also https://security.snyk.io/vuln/SNYK-JS-TANSTACKREACTROUTER-16640208): on 2026-05-11 84 malicious versions across 42 `@tanstack/*` packages were published (registry times for react-router: 1.169.5 at 19:20:42Z and 1.169.8 at 19:26:17Z, VERIFIED). Malicious packages carry an `optionalDependencies` entry `@tanstack/setup` pointing at a GitHub commit (`github:tanstack/router#79ac49ee...`) and a ~2.3 MB `router_init.js`. Publish went through the legit OIDC trusted publisher, so **provenance alone did not protect** consumers at the time.
  - The advisory lists `1.169.9` as patched for `react-router`/`router-core`. **`npm view @tanstack/react-router@1.169.9` returns E404 today** (VERIFIED), and neither `1.169.9` nor `1.169.4` exists in the registry; the registry now jumps `1.169.2` (2026-05-05) -> `1.169.5` (2026-05-11 malicious) -> `1.170.0` (2026-05-15). So the advisory's "patched" version never appeared as such; the safe lines are **versions published before 2026-05-11 19:00 UTC (<= 1.169.2) or the post-incident 1.170.x series**. The malicious versions may be unpublished or still present; do not assume.
  - Other affected packages relevant here and their advisory "patched" versions (VERIFIED from the advisory page): `@tanstack/router-plugin` 1.167.42, `@tanstack/router-cli` 1.166.50, `@tanstack/router-generator` 1.166.49, `@tanstack/router-core` 1.169.9, `@tanstack/history` 1.161.13. Current registry latest (VERIFIED): router-plugin 1.168.47, router-cli 1.167.45, router-generator 1.167.45, router-core 1.171.38, history 1.162.5 (all later than the advisory's patched numbers).
  - **If TanStack is chosen: pin exact versions** (e.g. `@tanstack/react-router@1.170.45`, `@tanstack/router-core@1.171.38`, `@tanstack/history@1.162.5`), check each tarball's `package.json` for `@tanstack/setup`, run installs with scripts disabled (Bun does by default), and review `bun.lock` for any `github:` source. I would not pick a version below 1.170.0 (published inside/adjacent to the incident week) without reading its tarball.
- **No Bun plugin.** The TanStack file-routing plugin docs (https://tanstack.com/router/latest/docs/framework/react/routing/file-based-routing) list Vite, Rspack/Rsbuild, Webpack and Esbuild installation guides and no Bun integration. `@tanstack/router-plugin` peers: `vite`, `webpack`, `@rsbuild/core`, `vite-plugin-solid`, `@tanstack/react-router ^1.170.45` (VERIFIED). Under `Bun.build` the options are the `@tanstack/router-cli` (generate `routeTree.gen.ts` as a pre-step; the fetched page did not confirm CLI behaviour, INFERRED) or code-based routes. Auto code splitting (a main selling point) comes from that bundler plugin and would be lost or hand-written (`lazyRouteComponent`).
- Net (INFERRED): React Router 7.18.4 behind the facade is the lower-risk pairing with Bun.build: no bundler plugin needed, no 2026 supply-chain event, React 19.1.1 compatible. TanStack's typed search params is real value but needs a pre-generation step, three new packages with a compromise history, and a hand-rolled code-split story under Bun. If typed search params matter most, write them in the facade over `useSearchParams` with Zod-less small parsers (the repo already has no router-level schema library).

## 5. `web-vitals` replacement for `next/dist/client/web-vitals`

- Only one consumer: `src/components/telemetry/WebVitalsReporter.tsx` (VERIFIED by grep) calls `useReportWebVitals(cb)` and reads `metric.name`, `value`, `rating`, `delta`, `id`.
- Replace with the `web-vitals` package: latest **6.2.3** (npm), exports `onCLS`, `onFCP`, `onINP`, `onLCP`, `onTTFB` (the readme references `onLCP, onINP, onCLS`; export map includes `./onCLS.js` etc., VERIFIED). Metric objects have the same field names (`name`, `value`, `rating`, `delta`, `id`), so the callback body is unchanged; replace `useReportWebVitals(cb)` with a `useEffect` that registers `onCLS(cb); onFCP(cb); onINP(cb); onLCP(cb); onTTFB(cb);` once.
- **FID:** the reporter's comment lists FID. `web-vitals` removed `onFID` in v4+ (INFERRED from the 6.x export surface, which I did not see listing FID); INP replaces it. Drop FID from the comment; expect the `web_vital.fid` series to end. Next's wrapper (15.3.2) reported what it bundled (web-vitals 4.x). Check that dashboards do not depend on `web_vital.fid`.
- Types: `import type { Metric } from "web-vitals"`.
- Pin `web-vitals@6.2.3` exactly (`engines` not checked); size ~ small (unpacked 1.19 MB includes sources; shipped is a few KB, INFERRED). Any Jest mock of `next/dist/client/web-vitals` (grep in `__tests__`) moves to `web-vitals`.

## 6. Storybook without `@storybook/nextjs`

- Current state is already Next-free (section 0): `@storybook/react-webpack5` 8.6.18, addons essentials/themes/a11y, `@chromatic-com/storybook` 3.2.7, with `.css.ts` mocked by a stub loader (components render without styles).
- Option A (do now, near-zero risk): delete `@storybook/nextjs` from `devDependencies`; keep react-webpack5 8.6.18. **No Bun runtime concern** (Storybook CLI runs on Node; `bunx --bun` not needed; INFERRED, not run).
- Option B: `@storybook/react-vite` — **8.6.18 peers `vite ^4 || ^5 || ^6`; 10.6.2 peers `vite ^5 - ^8`, `storybook ^10.6.2`** (VERIFIED). The official `@vanilla-extract/vite-plugin` 5.2.6 peers `vite ^5 - ^8` (VERIFIED), which would give Storybook **real VE styling** instead of the stub, but it adds Vite to a repo that is otherwise removing a bundler and doubles the CSS pipeline (Bun plugin for the app, Vite plugin for Storybook). A major Storybook upgrade 8 -> 10 is a separate project (addon-essentials was folded into core in 9+; INFERRED, not checked).
- Recommendation: Option A in the migration; defer B (and the 8 -> 10 upgrade) as a separate improvement.
- Storybook also needs the `@` alias; already present in `main.ts` (`"@": path.resolve(__dirname, "../src")`).

## 7. ESLint without `eslint-config-next`

Source: https://nextjs.org/docs/app/api-reference/config/eslint (opened) and the packed `eslint-config-next@15.3.2` `index.js` / `core-web-vitals.js` (VERIFIED, unpacked from npm).

What `next/core-web-vitals` (the repo's `extends`) actually turns on:
- `plugin:react/recommended`, `plugin:react-hooks/recommended` (react-hooks ^5: `rules-of-hooks` error, `exhaustive-deps` warn), `plugin:@next/next/recommended` + `core-web-vitals` (upgrades `no-html-link-for-pages`, `no-sync-scripts` to errors).
- `import/no-anonymous-default-export: warn`; `react/no-unknown-property`, `react/react-in-jsx-scope`, `react/prop-types`, `react/jsx-no-target-blank`: off.
- jsx-a11y (warn): `alt-text` (img), `aria-props`, `aria-proptypes`, `aria-unsupported-elements`, `role-has-required-aria-props`, `role-supports-aria-props`.
- Parser: `@typescript-eslint/parser` for ts/tsx; env browser+node; settings `react.version: detect`; import resolver node + typescript.

What is Next-specific and worth zero in a Vite-less SPA: all `@next/next/*` rules except possibly `no-img-element` (the SPA has no `next/image` to prefer, so it would only nag), `no-html-link-for-pages` (replace by the router facade lint: forbid `<a href="/...">` internal navigation if wanted, or a `no-restricted-syntax` rule), `no-head-element`, `no-sync-scripts` (the FOUC inline script is intentionally sync). So reproducing core-web-vitals means: **react (recommended), react-hooks (recommended), jsx-a11y (the six rules above), import/no-anonymous-default-export**; the `@next/next` plugin can be dropped.

Replacement plan (flat config, `eslint.config.mjs`):
- ESLint **9** is installed (`"eslint": "^9"`; latest npm is 10.13.0). Flat config is the default in 9. `eslint-plugin-react-hooks` latest **7.1.1** peers eslint through ^10 (VERIFIED), `eslint-plugin-jsx-a11y` latest **6.10.2** peers eslint `^3 - ^9` (VERIFIED: so **stay on ESLint 9**; jsx-a11y does not list 10), `typescript-eslint` latest **8.71.2**. Note `eslint-plugin-react-hooks` 7 adds React Compiler rules to its `recommended` preset (INFERRED from 6/7 release notes, not reopened); pin the six legacy rules explicitly (`rules-of-hooks: error`, `exhaustive-deps: warn`) rather than adopting `recommended` blindly to avoid a wall of new warnings (repo already has "~1450 pre-existing warnings" per `lint.yml`).
- Convert `.eslintrc.json` to flat: keep `eslint-plugin-boundaries` ^6.0.2 (supports flat), the two `file:` plugins (`eslint-plugin-analytics`, `eslint-plugin-rpc-lifecycle`; plain objects, flat-compatible with `plugins: { analytics, "rpc-lifecycle": ... }` — INFERRED, check they export a plain `rules` object), the large `no-restricted-syntax` block, and the overrides (`**/*.css.ts`, `src/components/**/*.tsx`, etc.). Using `FlatCompat` from `@eslint/eslintrc` (3.3.7) is a stopgap, not recommended.
- `next lint` exits 1 on any warning (comment in `lint.yml`); `eslint` exits 1 only on errors. Preserve CI behaviour with `eslint --max-warnings 0 <changed files>` in `lint.yml` (the current CI step builds `--file` args from changed files; use plain file args). `package.json` `lint` script becomes `eslint . && ...`; `scripts/lint-affected.sh` needs the same change (not read this session).
- Dev dependencies to add: `eslint-plugin-react`, `eslint-plugin-react-hooks`, `eslint-plugin-jsx-a11y`, `eslint-plugin-import` (only if keeping `no-anonymous-default-export`; otherwise drop), `typescript-eslint`, `@eslint/js`, `globals`. Remove: `eslint-config-next`, `@next/eslint-plugin-next`. Versions above are npm latest; pin exactly.

## 8. Fonts

- Source today: `src/app/fonts.ts` calls `localFont` four times (JetBrains Mono variable normal+italic, Rajdhani 400/500/600/700, Cinzel variable at 400 and 700 pointing at one file, Inter), exposing `--font-jetbrains-mono`, `--font-rajdhani`, `--font-cinzel`, `--font-inter` CSS variables, and applying `.variable` classes on `<html>` (`layout.tsx`).
- Replacement: write `@font-face` blocks in `src/app/globals.css` (or a new `fonts.css`) with the same `font-family` names and weights, `font-display: swap`, and define the four CSS variables on `:root` (e.g. `--font-rajdhani: "Rajdhani", ui-sans-serif, ...`). Next also generated size-adjusted fallbacks; the replacement should add an explicit `local()` or system fallback stack to limit CLS (INFERRED).
- URL strategy: see 2.4 — serve from `out/fonts/` with `external: ["/fonts/*"]` (VERIFIED works) or accept data-URI inlining (194 KB of woff2 -> ~259 KB CSS). Preloading: add `<link rel="preload" as="font" type="font/woff2" crossorigin href="/fonts/...">` for the primary UI font in `index.html`.
- `Cinzel` is the same file listed for weights 400 and 700: collapse into one `@font-face` with `font-weight: 400 700`.

## 9. `size-limit` for a non-Next output layout

- Current `.size-limit.json` and `package.json["size-limit"]` (both exist; the JSON file and `package.json` key duplicate each other — drop one) point at `out/_next/static/chunks/**/*.js`, `framework-*.js`, `main-*.js`. These names are Next-only.
- Bun HTML-entry output: `out/assets/*.js` with hashed names derived from the entry/chunk names (VERIFIED naming above: `index-<hash>.js`, `lazy-<hash>.js`). There is **no framework chunk** and no stable `main-*` name. Replace with: (1) `Total JS`: `out/assets/**/*.js`, ignore `**/*.map`, limit stays `5 MB` (the requirements say total JS must be <= Next production 6.35 MB and spike measured 5.43 MB; 5 MB is not met by the spike, so set the new budget from the real measured number plus headroom — a plan decision, not mine); (2) `Entry chunk`: path derived from `out/index.html` (small script to read the `<script src>` and feed it to `size-limit`) or glob `out/assets/index-*.js`; (3) an optional React chunk requires `splitting` to isolate React, which Bun does by shared-chunk analysis, not by name; skip the framework check or compute it from the largest shared chunk (INFERRED).
- `@size-limit/file` 14.2.0 and `size-limit` 14.2.0 are the latest (VERIFIED; repo has ^11, `14.2.0` engines `node ^22.19 || ^24.5 || >=26`); staying on 11 is fine and avoids a Node floor bump. `gzip: false` is current; a gzip/brotli budget is the better metric for shipped bytes, consider adding.
- Observability requirement (bytes, chunk count, duration): emit from `build.ts` (`BuildOutput.outputs`: `kind`, `path`, `size`; shown in my run) to `$GITHUB_STEP_SUMMARY`/a JSON file; `metafile: true` is the alternative.

## 10. Supply-chain checklist for the new dependency set

- New or newly direct: `@vanilla-extract/integration` (8.0.11, direct), `web-vitals` 6.2.3, `react-router` 7.18.4 (or 8.4.0 + React bump), ESLint plugins (section 7), maybe `@types/bun` 1.4.3. Remove: `next`, `@vanilla-extract/next-plugin`, `eslint-config-next`, `@storybook/nextjs`.
- Pin exact versions; rely on `bun.lock` + `bun install --frozen-lockfile`. Bun does not run dependency lifecycle scripts unless listed in `trustedDependencies` (feasibility doc; consistent with Bun docs), which the repo currently expresses as `pnpm.onlyBuiltDependencies` (`@swc/core`, `esbuild`, `@vanilla-extract/css`): map to `trustedDependencies` only for what actually needs a postinstall (esbuild's native binary arrives via optional platform packages, not a script; `@swc/core` is only for ts-jest/Storybook; untested).
- Before adding any package, check for the TanStack-style signature: `optionalDependencies` pointing to `github:` and unexpected large root JS files (`npm pack`, do not install).

## 11. Open items I could not settle

- Dev-server `/api` proxy and WebSocket upgrade through `Bun.serve` (not in docs, not tested).
- Fast Refresh with stateful components under Bun HMR (spike tested one component).
- Whether `external` and `publicPath` interact correctly with Bun's HMR dev bundler; I only tested `Bun.build`.
- ESLint plugin behaviour of the two `file:` plugins under flat config; `scripts/lint-affected.sh` content.
- Upstream issue for `eval`; I did not search vanilla-extract's tracker.
- React Router 7 vs 8: confirm `ScrollRestoration`/`useSearchParams`/route `lazy` in the chosen version's docs when writing the ADR.

## Sources
- https://bun.com/docs/bundler
- https://bun.com/docs/bundler/fullstack
- https://bun.com/docs/install/patch
- https://reactrouter.com/start/library/installation
- https://reactrouter.com/start/data/installation
- https://github.com/TanStack/router/security/advisories/GHSA-g7cv-rxg3-hmpx
- https://security.snyk.io/vuln/SNYK-JS-TANSTACKREACTROUTER-16640208
- https://tanstack.com/router/latest/docs/framework/react/routing/file-based-routing
- https://nextjs.org/docs/app/api-reference/config/eslint
- npm registry (`npm view`) for every version and peer-dependency statement above, 2026-10-19
