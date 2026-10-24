# Web-app Bun build: gate record (GateRecord)

Gates S0, S1, S3 and S4. S2, the owner sign-offs (Story 0.6) and the trigger decision are **not** in this file yet.
S0 and S1 measured 2026-10-21 on Linux x86_64 (Manjaro), Bun 1.4.3 (also 1.3.11 for S1), Node 26.8.1, `main` at `b9258fe35`.
S4 measured the same day at `main` `b9258fe35`; S3 measured 2026-10-21 at `main` `c7d385890` (after PR 4 React Router, PR 5a Bun build pipeline, PR 6a flat ESLint config merged), Bun 1.4.3, Node 26.8.1.
macOS: unverified (S1 census, `bun patch` behaviour, lifecycle scripts).
Host load average was 6 to 11 throughout (other agent sessions), so wall times are indicative only.

Commits each S0 result was taken on:
- **#1100 boundary pair**: pnpm control `9acc37d33` (parent of #1100) against Bun `69d1281e3` (#1100 merge). `git diff --stat 69d1281e3^ 69d1281e3 -- web-app/src` is empty, so only the package manager differs.
- **main**: `b9258fe35`, Bun only. No pnpm control exists for main: `pnpm-lock.yaml` was deleted by #1100 and `web-app/package.json` has since gained dependencies (`web-vitals`, `@ast-grep/cli`, `fast-check`, `@vanilla-extract/integration`, a VE bump), so the old lockfile cannot reproduce main.

Re-run: `project_plans/web-app-bun-build/spikes/s0-rerun.sh [web-app]` (prints the commit; see header for env knobs).

## Verdict table

| Gate | Criterion | Verdict | Evidence |
|---|---|---|---|
| S0 | Next dev-mode build exits 0 on a Bun tree, JS within 2% | **PASS** at the #1100 boundary; **UNVERIFIABLE** on main | table A |
| S0 | Jest: same failing-file set as pnpm | **PASS** | table B |
| S0 | `storybook build` exits 0 | **RED** as configured (isolated linker); passes with `linker = "hoisted"` | section S0-storybook |
| S0 | `eslint` loads the `file:` plugins | **PASS** | section S0-eslint |
| S0 | VE compile of 3 files with only the minimal trusted set | **PASS** with an empty set | section S0-trust |
| S0 | Playwright suite x3 on Next committed as `E2eBaseline` | **NOT MET**: not run to completion, no baseline file | section S0-e2e |
| S0 | `s0-rerun.sh` exists and runs | **PASS** (script written and run once; it reports the three findings above) | section S0-rerun |
| S1 | HTML entry + splitting + VE plugin + define builds the real `src/`, naming under `_next/static/` | **PASS** on 1.4.3, **RED** on 1.3.11 | section S1-build |
| S1 | `out/index.html` at root, no stray `*.html` (layout A') | **PASS** | section S1-build |
| S1 | Every `out/` file under an `auth.go` exempt prefix or allowlisted | **PASS** (spike output only; `public/` not included) | section S1-auth |
| S1 | Asset URLs in `index.html` start with `/_next/static/` | **PASS** 1.4.3, **RED** 1.3.11 | section S1-build |
| S1 | `THEME_BOOT` placeholder survives `minify:true` | **PASS**, first form `<!--THEME_BOOT-->` | section S1-build |
| S1 | Deep link served by the real Go binary, JS content type per asset | **PASS** | section S1-auth |
| S1 | woff2 resolved, no data-URI inlining of the 194,364 B | **RED by default, PASS with mitigation** | section S1-fonts |
| S1 | `jsx: preserve`, `src/gen` `.js` to `.ts`, `next/*` import throws, invalid CSS throws | **PASS** | section S1-build |
| S1 | Defines: no `process.env` text, no `process is not defined` | **AMBER**: 2 guarded third-party matches remain; no runtime error | section S1-defines |
| S1 | CSS census: resolved custom-property map equal per theme | **PASS with explained differences** on 1.4.3; not run on 1.3.11 | section S1-census |
| S1 | Unauthenticated `/login` assets feasible (Story 7.13) | **PASS**: Playwright path works, Go fallback not needed | section S1-auth |

Triggers: T6 (S1 census fails on every pinnable Bun) did **not** fire: 1.4.3 passes. S0's FAIL consequence ("dual build not viable") is not met by the Storybook finding alone, because a verified one-line mitigation exists, but that is the owner's call. Pin candidate: **Bun 1.4.3**; 1.3.11 mis-writes asset URLs.

## S0

### Table A: Next development-mode build (`NEXT_BUILD_MODE=development bun run build`, the command `make web-build` runs; `WEB_BUILD` does not exist yet, so this is the only mode)

Private webpack cache per tree, `out` and `.next` removed first. JS bytes = sum of `*.js` under `out/` (excluding `.map`).

| Tree | Exit | JS bytes | vs 21,666,571 (plan baseline) | vs pnpm control | Wall |
|---|---|---|---|---|---|
| pnpm, `9acc37d33` | 0 | 21,589,745 | -0.35% | n/a | 67.5 s |
| Bun, `69d1281e3` (#1100) | 0 | 21,485,779 | -0.84% | **-0.48%** (limit 2%) | 59.5 s |
| Bun, main `b9258fe35` | 0 | 22,634,446 | +4.47% | no control | 69.6 s |
| Bun, main, empty `trustedDependencies` | 0 | 22,634,436 | +4.47% | no control | 92.2 s |

Reading: the pnpm control reproduces the plan's 21,666,571 within 0.35%, so the byte-count method matches. The #1100 boundary passes by a wide margin. The +4.47% on main is **not** attributable to Bun: no same-commit pnpm control is constructible (see top). The plan's constant is stale for main; refresh it from a pnpm build of the commit where the dual build starts, or set the tolerance against the #1100-boundary pair. Run-to-run noise on the same tree is 10 bytes (row 3 versus row 4).

### Table B: Jest (`jest --no-coverage --json`, `CI=true`)

Control trees were extracted with `git archive` (web-app, scripts, tests, docs, project_plans) and `src/gen` regenerated with `buf generate` using only the local `protoc-gen-es` plugin; the `gen` directories of the two control trees are identical.

| Tree | Suites | Tests | Failing files | Failing tests |
|---|---|---|---|---|
| pnpm, `9acc37d33` | 628 | 7,801 | 0 | 0 |
| Bun, `69d1281e3` | 628 | 7,801 | 0 | 0 |
| Bun, main `b9258fe35`, quiet host | 760 | 8,903 | 0 | 0 (2 pending) |

Failing-file lists are identical (both empty); `jestcmp.mjs`-style diff of the two lists is empty. The plan's "620 files / 7,683 tests" baseline is out of date: the counts above are what the same suite reports now.
Caveat: a second main run while an e2e run was loading the host (`s0-rerun.sh`) failed `src/lib/files/__tests__/inputProfile.test.ts` and `transportCompressionSpike.test.ts`. Both passed in the quiet run. Not diagnosed; INFERRED load sensitivity. Treat as a flake to chase before the Soak, not a Bun regression.
An earlier attempt on extracted trees without `docs/` failed 2 to 6 suites that read repo files outside `web-app/`; that was an extraction artifact, fixed by extracting `docs/`.

### S0-trust: lifecycle scripts

`bun install --frozen-lockfile` **ignores** `package.json`'s `trustedDependencies` in favour of the list stored in `bun.lock` (a "no trust" test with `--frozen-lockfile` was a false negative). The valid test removes `node_modules`, empties the list and runs a plain `bun install`.

- Packages with install scripts in the tree: `@ast-grep/cli`, `@swc/core`, `core-js-pure`, `esbuild` (x2), `sharp` (x2), `unrs-resolver`. `bun pm untrusted` with an empty list flags only `@ast-grep/cli` and `@swc/core`; `esbuild` and `sharp` are in Bun's default-trusted list.
- With an empty list on Linux x64: VE compile of `theme.css.ts`, `theme-contract.css.ts`, `layout.css.ts` succeeds (`spikes/ve-compile3.ts`), `@swc/core` transforms TypeScript, `ast-grep --version` prints 0.51.0, `next lint --dir src/lib/nav` passes, and the Next development build exits 0 (Table A, last row).
- `@vanilla-extract/css` has no install script at all, so its `trustedDependencies` entry has no effect.
- **Minimal set found on this host: empty.** Symptom per omission: none observed for `@swc/core` or `@ast-grep/cli` (both ship their native binary through platform optional dependencies; the postinstall is a fallback). Not verified on macOS or with a clean Bun cache (the platform packages came from the global cache).

### S0-storybook: RED

`storybook build` fails on the Bun trees (#1100 and main) with `Error: Cannot find module 'babel-loader'`, and passes on the pnpm control (rc 0, 14 s).
Root cause (CONFIRMED by the experiments below): `web-app/.storybook/main.ts` calls `require.resolve("babel-loader")`, but `babel-loader` is only a transitive dependency (of `@storybook/nextjs`), and `bunfig.toml` sets `linker = "isolated"`, which does not expose transitive packages at the top level.
- Adding `babel-loader@9.2.1` as a devDependency moves the failure to `Cannot find package '@babel/preset-env'` (more phantom dependencies follow).
- `linker = "hoisted"` in `bunfig.toml` with the same lockfile: `storybook build` exit 0 in 14 s.
Not run with the hoisted linker: Next build, Jest, S1. The choice (hoisted linker, or declare the missing devDependencies) is the owner's; Epic 6 owns Storybook.

### S0-eslint

`next lint --dir src/lib/nav`: "No ESLint warnings or errors" on main (isolated linker) and with an empty trust list. `eslint --print-config src/lib/nav/index.ts` lists `analytics/require-on-click`, `analytics/require-omnibar-dispatch`, `analytics/require-page-analytics`, `analytics/require-rpc-analytics` and `rpc-lifecycle/require-abort-signal`, so both `file:` plugins loaded. Plain `eslint` (v9.36.0) needs `ESLINT_USE_FLAT_CONFIG=false` for the `.eslintrc.json`; `next lint` sets it itself.

### S0-e2e: not met (honest account)

Not committed: `tests/e2e/baseline/next-e2e-baseline.json`. The tooling exists (`spikes/s0/classify-e2e.mjs`, 6 fixture tests pass, including first-fail-then-pass classified `flaky`). `routesVisited` was not collected (needs a fixture hook; not written).

What was run and found:
1. `playwright test` with no file filter aborts: Playwright's default `testMatch` collects the Jest unit test `tests/e2e/global-setup.dev-mode.test.ts` (`ReferenceError: jest is not defined`). Workaround used: pass the top-level `*.spec.ts` files explicitly (177 files today, not 152; 2,285 tests across all projects).
2. `CLAUDE.md` says the e2e server gets a dynamic port, but 126 spec files hardcode `process.env.BASE_URL ?? "http://localhost:8544"` and `global-setup.ts` exports only `TEST_SERVER_URL`. With a dynamic port they fail with `ERR_CONNECTION_REFUSED`. Workaround used: `TEST_SERVER_PORT=62880` with `TEST_SERVER_URL` and `BASE_URL` both `http://localhost:62880` (inside the permitted port range), `TEST_SERVER_BINARY` = a binary embedding the main Next build, `--test-mode` isolated dir. The live instance on :8543 was not touched.
3. One run was started and stopped by me after 189 attempts (about 16 minutes): about 12 attempts per minute, so about 3 hours per full run and about 9 hours for three. Not done. The partial run had no JSON report (the reporter writes at the end) and the host was loaded by my other measurements, so it is **not** a baseline. Partial log tally: 132 passed, 37 failed in `chromium`, 4 skipped, and all 16 attempts in the four `visual-*` projects failed within 3 ms (not diagnosed; probably missing snapshots or fixtures).
4. Known Next failures already recorded by #1117 (`tests/e2e/baseline/build-coupled-specs.json`, `failingOnNextAtCapture`): 4 tests. Build-coupled spec inventory is in that file (8 specs); a fresh grep for `reactStrictMode|__next-route-announcer__|NEXT_BUILD_MODE|dev-mode` finds `accessibility`, `backlog.dev-mode`, `insights-session-route`, `multi-window`, `terminal-resync-banner`, `terminal-resync-feature-flags`, all in that list or the dev-mode config.

To finish: run `e2e-run.sh`-equivalent three times (about 9 hours of machine time on a quiet host), then `node spikes/s0/classify-e2e.mjs`.

### S0-rerun

`spikes/s0-rerun.sh` runs: Next build bytes against a baseline (2% tolerance), Jest failing-file list against a control, `storybook build`, ESLint plugin load, VE compile of 3 files; prints the commit; exit status is the failure count. Run on main it reported: build FAIL (4.47% against the stale constant), jest FAIL (the two load-sensitive files above), storybook FAIL (babel-loader), eslint PASS, ve PASS. It must be re-run on the merged #1100 `main` before PR 3 and whenever #1100's head changes; the first two FAILs need a pnpm-built baseline number and a quiet host to be meaningful.

## S1

Spike code: `spikes/s1/{build.ts,exempt-walk.ts,served-check.ts,browser.mjs,census-compare.mjs}`. Entry is a **generated spike entry** (`web-app/.s1-entry/`, not committed), not the future `RouteTable`: it renders the real `RootShell` over 44 lazily imported `page.tsx` files, aliasing `@/lib/nav/impl` to `impl.memory.tsx` and `@/lib/lazy/impl` to `impl.bun.tsx` (both already on main). The VE plugin is the one from `spikes/bun-spike/ve-plugin.ts` over the patched `eval` from #1122.

### S1-build (Bun 1.4.3)

`Bun.build({entrypoints:[index.html], splitting, minify:true, publicPath:"/", throw:true, naming:{entry,chunk under _next/static/chunks/[name]-[hash].[ext], asset under _next/static/media/...}, define, plugins:[seams, vanillaExtract]})`:
- Build wall 1.1 s (two runs), 3.2 s under load. Output: 319 files, 132 JS (5,572,067 B), 46 CSS (2,930,905 B), 8 woff2, 1 html. This is a size indication only: S3 owns the size gate.
- Layout **A'** verified: Bun emits `out/_next/static/chunks/index-<hash>.html`; moving it to `out/index.html` leaves exactly one `*.html`.
- `index.html` references exactly `/_next/static/chunks/index-<hash>.css` and `.js`.
- Placeholders: all three candidate forms (`<!--THEME_BOOT-->`, `<script id="theme-boot"></script>`, `<meta name="theme-boot">`) are byte-identical in the emitted HTML with `minify:true`. Bun did not minify the HTML whitespace at all. Recorded form: `<!--THEME_BOOT-->`.
- `jsx: "preserve"` in `web-app/tsconfig.json` did not break the transform (the app rendered: `#root` had 7 children in a browser). `src/gen` `.js` to `.ts` imports resolved.
- Probes: an import of `next/navigation` fails the build with `forbidden import of next/navigation from .../main.tsx` (resolver `onResolve` throws); an invalid CSS file fails the build with `throw:true` (`Invalid selector. Empty selector is not allowed`).
- `tsconfig.json` `paths` maps `@/lib/lazy/impl` and `@/lib/nav/impl` to the Next implementations; the plugin's `onResolve` takes priority, which the build relies on.
- **Bun 1.3.11: RED.** Files are written under `_next/static/chunks/` but the HTML references `/index-89hkqxfw.js` and `/index-wty68qej.css` (prefix omitted), so the page fails in a browser (`Expected a JavaScript-or-Wasm module script but the server responded with a MIME type of "text/html"`). Chunk splitting also differs (215 JS files against 132). 1.4.3 is the only pinnable version tested.

### S1-fonts

The 8 woff2 files total 194,364 B (`ls -l`; sum of 8 files).
- Default: Bun's CSS bundler inlines all 8 as `data:font/woff2;base64,` (8 matches; CSS 3,189,912 B against 2,930,905 B after the fix, a 259,007 B difference, consistent with base64 of 194,364 B). `loader: {".woff2": "file"}` had no effect on CSS `url()`.
- Mitigation that works: write `@font-face` with `url("/_next/static/media/<name>.woff2")`, pass `external: ["/_next/*"]` and copy the fonts into `out/_next/static/media/` in a stage. Result: 8 files, 194,364 B, 0 `data:font` matches. Cost: the copied names carry no content hash (cache busting is Story 5.x's problem).
- `next/font/local` has no Bun equivalent, so `src/app/fonts.ts` is not usable on Bun; the census below shows the effect.

### S1-auth

`auth.go` exempt prefixes: `/auth/ /login /health /_next/ /favicon`.
- `exempt-walk.ts` on the spike output: 319 files, 318 under `/_next/`, 1 allowlisted (`/index.html`, served by `StaticFileServer` as the SPA fallback), 0 violations. For contrast the current Next `out/` has 409 files and 110 outside the exempt prefixes (per-route `index.html`/`index.txt`), so today's output already relies on SPA fallback and auth redirect for those. `public/` (manifest, icons, `push-sw*.js`, `docs/`) is not in the spike output; the allowlist for it is still open and is Story 5.x/6.8's.
- Real Go binary (built from main plus the spike `out/` as `server/web/dist`), ManualInstance on 62873 and remote listener on 62874 (`--remote-access`, `STAPLER_SQUAD_INSTANCE`, `STAPLER_SQUAD_TMUX_SOCKET` set): `GET /backlog/board` returns 200 `text/html` on the local listener; all 318 files requested unauthenticated return 200 with no HTML content type (`text/javascript` 132, `text/css` 46, `font/woff2` 8, `text/plain` 132 for `.map`). On the **remote** (authenticated) listener the deep link returns 302 to `/login` as designed and the same 318 assets return 200 unauthenticated.
- Story 7.13 feasibility: the remote listener starts under the ManualInstance environment, and Playwright with `ignoreHTTPSErrors:true` loads `/login` unauthenticated (`browser.mjs login`). Spike build: 26 requests, all static ones under `/_next/`, the rest `/api/...` (401). Current Next build: 618 requests (the app re-renders `/login` repeatedly), static ones under `/_next/`. `tests/e2e/helpers/test-server.ts` already has a `remoteAuth` option. **Decision: Playwright path; the Go-test fallback is not needed.**

### S1-defines

With `NEXT_PUBLIC_API_URL` undefined and the other four set to sentinels: `process.env.NODE_ENV` is `"production"`, sentinels are present (`sentinel-*` found in the otel chunk), and the page logged no `process is not defined`. The literal criterion "no `process.env` text" is **not** met: 2 matches remain, both third-party and guarded by `typeof process` (`@bufbuild/protobuf`: `process.env.BUF_BIGINT_DISABLE`; a debug check `process.env.LOG`). Verdict AMBER on the literal wording; no functional problem seen.

### S1-census

`browser.mjs census` loads `/` from the Next build (Next binary, local listener) and from the Bun spike build, sets each of the 6 theme classes, and compares every custom property that appears in any stylesheet, resolved by the browser (colours, shadows and durations canonicalised through the computed style, because Bun's CSS minifier rewrites notation: `#00cc33` to `#0c3`, `100ms` to `.1s`, `rgba(0,0,0,.85)` to `#000000d9`).
- Theme class names are identical in both builds (for example `matrix` = `_1b1axoi2`), as are the custom-property names.
- Common properties: 236 per theme. **`light` and `dark`: 0 differences. `matrix`, `cyberpunk77`, `wh40k`, `clean`: 3 differences each**, all three being the font-family tokens (for example `'jetbrainsMono','jetbrainsMonoFallback'` on Next against `'Monaco',monospace` on Bun). Cause: those values come from `next/font/local` variables that the spike entry does not provide. EXPLAINED by the font strategy, not by a CSS ordering bug.
- Only on Next: `--font-jetbrains-mono`, `--font-rajdhani`, `--font-cinzel`, `--font-inter` (4, same cause).
- Rule counts on `/`: Next 6,673, Bun 6,570 (-103, -1.5%); the two builds load different lazy CSS for `/`, so this is reported, not gated. Rule *order* is S3's census and was not examined.
- Bun 1.3.11: not run (the page does not load, see S1-build).
- Rendered text length of `/` is 823 characters on both, an equality, not a proof of parity.

## Not measured

- 3 full Playwright runs on Next and `next-e2e-baseline.json` (S0).
- A pnpm control for main (S0 bytes and Jest at main).
- macOS anything; clean-cache behaviour of lifecycle scripts.
- The S0 FAIL consequence decision, the Bun pin, T1 to T6 evaluation, Story 0.6 sign-offs, the `Amendments` section of `plan.md` (the plan file is not on `main`; it lives on the planning branch).
- S1 with the hoisted linker; S1 `public/` allowlist; Bun HTML-entry behaviour with the real `RouteTable`/router entry (does not exist yet).
- Fast Refresh, dev server (S2), 42-route sizes and CSS ordering (S3). Gate S4 (React Router) is recorded below.

## Gate S4: React Router 7.18.4 behaviour (plan Story 0.5, Task 0.3a)

Run date: 2026-10-21. Host: Linux (Manjaro) only; **macOS: unverified**. Base: `origin/main` `b9258fe35`. Bun 1.4.3, Chromium 145.0.7632.6 (Playwright cache build 1208, driven by `playwright-core@1.56.1`), React 19.1.1, `react-router@7.18.4` (exact, `bun.lock` committed). Ports used: 62871 (ManualInstance), 62875 (prototype server). `:8543` untouched.

Sources, all under `project_plans/web-app-bun-build/spikes/`:

| File | Role |
|---|---|
| `s2/prototype-entry.tsx` | Task 0.3a shared prototype: `createBrowserRouter` over one lazy route per `ROUTE_META` row (44), a `RootShell` stub, three guard layouts, `<title>` in the root. Reusable by S3. |
| `s4/build.ts`, `s4/serve.ts` | Bundle with `Bun.build`; static server with the Go handler's SPA semantics (`server/middleware/static.go`: `path.Clean`, existing file wins, else `index.html`). |
| `s4/router.spec.ts` -> `s4/results.json` | RR-side measurements (sections 1-7). `bun router.spec.ts`. |
| `s4/next-announcer.ts` -> `s4/next-results.json` | Real built Next app in a ManualInstance: announcer, scroll container, guard behaviour. |
| `s4/tarball.mjs` -> `s4/tarball-result.json` | Tarball inspection. |
| `s4/probe.ts` | Identifies one shell-level scroller (see scroll row). |

### Verdicts

Overall S4: **7 PASS, 1 AMBER** (scroll). The AMBER is not a router defect: it changes the design of Story 4.6/4.6b (see row 3). Owner sign-off on F1 is not recorded here.

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1 | `/x/` matches `/x` | PASS | 44 `ROUTE_META` paths, each cold-loaded plain and with a trailing slash (88 loads): 88/88 render the matching route, 0 mismatches, 0 page errors. `/settings` and `/settings/` both render `<h1>Settings</h1>`. A client navigation to `href="/settings/"` renders Settings; RR's `location.pathname` stays `/settings/` (not stripped), so `usePathname` must keep using `normalizePathname`. `/nope/` hits the catch-all. Note: the plan says 42 routes; `ROUTE_META` has 44 rows. |
| 2 | `document.title` on client nav | PASS | Root-rendered `<title>{titleFor(pathname)}</title>` (React 19 hoisting): `/` -> `/history` gives `History - Stapler Squad`. Walking all 44 paths by `router.navigate`: 0 title mismatches vs `ROUTE_META`. A layout effect and a post-commit `useEffect` (the announcer's position) both already see the new title (recorded `["Stapler Squad Sessions","History - Stapler Squad"]` / `["History - Stapler Squad"]`), so the `useEffect` fallback is not needed and the announcer-after-title ordering holds. Trailing-slash client nav gives the same title. Observation for Story 5.4: a static `<title>` in `index.html` is **not replaced**; the head ends with two `<title>` elements (React's first, the static one second) and `document.title` returns the first. Ship no static `<title>` or accept the duplicate. |
| 3 | Scroll container and `ScrollRestoration` verdict | **AMBER** | Real app (built Next in the ManualInstance, 44 routes x 1280x800 and 390x844 = 88 loads): the **window never scrolls** (`scrollHeight - innerHeight` = 0 in 88/88; with a 6000 px child injected, `window.scrollTo(0,400)` moved 0 px in 88/88); `body` `overflow:hidden`, `#main-content` `overflow:hidden` in 88/88. So the container is **neither window nor `<main>`**: scrolling happens in per-page inner elements. At 1280 wide, 16 of 44 routes already overflow in a page-level element with empty data (for example `/history`, `/logs` (virtuoso), `/unfinished`, `/rules`, `/config`, `/settings`, `/help`, `/changelog`; `/spike-files` is a CodeMirror scroller); the other 28 routes did not overflow on an empty instance, so the table is a lower bound. At 390 wide a shell-level `div` outside `#main-content` (`overflowY:auto`, 758 px high, 1026 px content, `/history`) exists on all 44 routes. Prototype with RR 7.18.4: `<ScrollRestoration/>` with window scroll resets to 0 on push, restores 1200 on back, and `preventScrollReset` keeps 1200 (works); without it RR does not touch scroll (forward stays 1200). With an inner scroller (same element across routes) `ScrollRestoration` does nothing: forward leaves 1200, back leaves the last value (300) rather than restoring 1200. **Verdict: `ScrollRestoration` is unusable for this app** because it only manages `window`, which never scrolls here; Task 4.6b needs its own per-page scroll reset and restore (keyed by location key), and plan Story 4.6/7b ACs that assume one "container" must be rewritten. Not measured: Next's own behaviour for inner scrollers on back (so the "restored within 2 px" AC may be a new behaviour, not parity). |
| 4 | Push vs replace matrix | PASS | `history.length` delta per call: `navigate(to)` +1; `navigate(to,{replace:true})` 0; `<Link>` +1; `<Link replace>` 0; `<Link>` to the **current** URL 0 (RR auto-replaces); `navigate(currentUrl)` +1 (duplicate entry); `navigate("?tab=b")` +1; `navigate("?tab=b",{replace:true})` 0; `navigate("#x")` +1; `setSearchParams(p)` +1; `setSearchParams(p,{replace:true})` 0; `<Navigate to>` default +2 (the redirect itself is pushed), `<Navigate to replace>` +1 (replaces the redirect entry); `navigate(-1)` traverses, length unchanged. Facade consequences: `router.push/replace` map 1:1; `Redirect` must pass `replace`; search-param setters used by Next-era call sites that mean "replace" must pass `{replace:true}` explicitly (RR defaults to push). Not measured: Next's own deltas for the same calls (the matrix is RR-side only). |
| 5 | Raw `history.replaceState` not observed; `ReplaceSearch` observed | PASS | `/unfinished?tab=a`, raw `replaceState(history.state,"","?tab=b")`: `location.search` = `?tab=b`, `useSearchParams` still `a`, RR `router.state.location.search` still `?tab=a`, `history.length` delta 0. Facade-style `navigate("?tab=b",{replace:true})` (fresh load): `useSearchParams` = `b`, delta 0. Run after the raw write: also `b`, delta 0. This confirms Story 2.6 (the four raw sites must move to facade helpers). |
| 6 | `history.state` keeps the `ssqTray` marker | PASS, with one recorded loss | RR initial state is `{idx:0}`; after marking the entry (`{idx:0,ssqTray:true}`), `navigate("/history")` writes `{usr,key,idx:1}` on the new entry and `back()` restores `{idx:0,ssqTray:true}`: **retained**. Clean tray sequence (raw `pushState({...state,ssqTray:true})` as `pushTrayEntryInHistory` does): length +1, RR location unchanged, RR render count unchanged (1 -> 1), `back()` -> marker absent on the earlier entry, `forward()` -> marker present, an RR push on top of the tray entry and `back()` -> marker present. **Loss:** an RR `navigate(...,{replace:true})` on a marked entry rewrites the whole state to `{usr,key,idx}` and drops `ssqTray`. Re-merging `{...history.state, ssqTray:true}` right after the navigate restores it (measured). Story 4.6 input: the router impl's `useReplaceSearch` (and any replace) must re-merge foreign top-level keys. A raw tray push duplicates RR's `key`/`idx` on the new entry; no misbehaviour observed in these sequences. |
| 7 | `FeatureGuard` blank-vs-redirect parity, 3 layouts | PASS | Covers `/backlog`, `/backlog/board`, `/changelog`, `/settings/backlog-sources` (3 layouts) x flag on/off x client-nav/cold-load = 16 cases on each side. Next side is the **real app** with the `GetFeatureFlags` response delayed 800 ms and the flag forced; RR side is the prototype guard copying `RequireFeatureFlag`'s logic with a stub store. Both: cold load is blank (`#main-content` text length 0 at +450 ms) until the flag store loads; flag on renders the page; flag off ends on `/` after exactly one `replace`, cold-load `history.length` 2 -> 2 (entry replaced), client-nav delta +1 (the guarded entry is replaced, so Back is not trapped). One difference: Next's client navigations produce trailing-slash URLs (`/backlog/`), RR keeps `/backlog`. Caveat: RR side uses a prototype, not the real `RequireFeatureFlag`/context. |
| 8 | Tarball and supply chain | PASS | `react-router@7.18.4` (published 2026-09-15, npm provenance attestation present): no `optionalDependencies`; dependencies `cookie ^1.0.1`, `set-cookie-parser ^2.6.0`; no `github:`/git/http dependency source; `scripts` are `build`, `watch`, `typecheck` only (no `preinstall`/`install`/`postinstall`); not deprecated. Resolved transitives `cookie@1.1.1` and `set-cookie-parser@2.7.2` also pass; `cookie` ships a `prepare` script, which registry installs do not run (`bun pm untrusted` in the spike reports 0 untrusted dependencies with scripts). `bun.lock` has 0 `github:` entries. Banned list: the plan defines none yet (Story 4.1 builds the screen); the only named bad versions are TanStack `1.169.5`/`1.169.8` (`research/stack.md`), and 7.18.4 is on no such list. GitHub advisory DB (`gh api "advisories?ecosystem=npm&affects=<pkg>@<ver>"`): 0 advisories for `react-router@7.18.4`, `cookie@1.1.1`, `set-cookie-parser@2.7.2` (the same query returns advisories for `react-router@7.5.0` and `cookie@0.4.0`, so it is not vacuous). `bun audit` and the npm bulk-advisory endpoint timed out and were not used. |

### Built Next announcer politeness (Task 0.5d)

Read from the real built Next app in the ManualInstance (not inferred): `<next-route-announcer>` hosts a shadow root containing `<div aria-live="assertive" id="__next-route-announcer__" role="alert">`; after a client navigation to `/history` its text becomes `History - Stapler Squad` (one mutation, `document.title` already equal). The client source that creates it is in `/_next/static/chunks/6737-8a2e11fcf4e11202.js` (`ANNOUNCER_ID = '__next-route-announcer__'`). **Next is assertive/`role=alert`**, so the "assertive" wording in `requirements.md` accepted scope addition 3 and `design/ux.md` is correct; no wording correction is needed, only the INFERRED label can become VERIFIED. The Bun announcer stays polite/`role=status` as planned.

### Limits of this run

- Linux only; the ManualInstance binary (`~/.stapler-squad/manual-builds/manual-1/stapler-squad`, mtime 2026-10-20 08:07 -0700) embeds a Next build whose source commit is not recorded, so the Next measurements are of that build, not of `b9258fe35`. Scroll rows used an empty instance (no sessions), so data-dependent overflow is under-counted.
- The RR prototype was served by `s4/serve.ts` (same SPA fallback rules as the Go handler), not through the Go binary.
- Prototype pages are stubs; real page effects (data fetching, focus, `useSearchParams` consumers) were not exercised.
- Not measured: Next-side push/replace deltas, Next-side tray-marker behaviour, Next's scroll behaviour for inner scrollers.
- `bun audit` / npm advisory endpoint unavailable (timeouts); advisory screening relied on the GitHub advisory DB.

## Gate S2: `bun dev` with the real app, including React Fast Refresh

Run date: 2026-10-22. Supersedes this file's earlier "Not measured: Fast Refresh, dev server (S2)..." line (the "Not measured" section above is left as originally written per this update's append-only scope). Host: Linux (Manjaro) only; **macOS: unverified**. Base: this branch (`feat/web-bun-dev-server`, PR 5a `49005ecbb` + PR 6a `c7d385890` merged, plus Story 5.6/5.8's own `web-app/build/dev.ts`, `dev-server.ts`, `build/plugins/dev.ts`, `src/theme-boot.ts`). Bun 1.4.3. Backend: a ManualInstance built from this branch (`STAPLER_SQUAD_INSTANCE=claude-s2-62875`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-s2-62875`, `PORT=62876`, backlog feature flag enabled). Dev server: `PORT=62875 NEXT_PUBLIC_API_URL=http://127.0.0.1:62876/api bun web-app/build/dev.ts`. Chromium via Playwright (`tests/e2e/node_modules/playwright-core@1.56.1`). Ports: 62875/62876 only; `:8543` untouched. Driver scripts: `project_plans/web-app-bun-build/spikes/s2/s2.mjs` (route sweep) and `.../spikes/s2/hmr.mjs` (fastrefresh/csstoken/overlay/memory) — both throwaway, not part of the shipped build.

### Verdict table

| Criterion | Verdict | Evidence |
|---|---|---|
| All 42+ routes load, zero console errors | **AMBER** | section S2-routes: 44/44 `ROUTE_META` paths render (landmark present, HTTP 200, 0 uncaught page exceptions); 43/44 have no console error beyond one pre-existing, non-Bun Redux serializability warning; 1/44 (`/notifications`) has one additional pre-existing 404. Neither residual is Bun- or dev-server-specific (app/backend-data-shape issues, reproduced on every route load regardless of bundler) |
| Fast Refresh keeps state (3 stateful pages) | **AMBER** | section S2-fastrefresh: 1 of 3 preserves state (terminal-stress preset `<select>`); 2 of 3 lose it (SessionList's filters-panel/show-archived toggle, Omnibar's input text). In all 3, Bun live-swaps the module with **zero full page reloads** — this is "live reload without state-preserving Fast Refresh" exactly, not a crash or a full-reload fallback |
| theme-contract.css.ts / shared-token-file edit updates dependants without manual reload, else full-reload fallback recorded | **PASS** | section S2-csstoken: Bun's own dev HMR re-requests and live-swaps the compiled CSS of an edited `.css.ts` into the already-rendered page, confirmed in under 150ms with zero full reloads (standalone `bun build/dev.ts`); the content-hash memo (Story 5.1) is sufficient and the full-reload fallback is **not** needed by default (`VeCompiler.fullReloadOnCssTsChange` flipped from `true` to `false`). One spontaneous reload was observed in the same check run through the full dev-stack (real backend + router) whose trigger is not yet isolated — recorded, does not change the verdict since both of the AC's actual requirements (2s budget, no *manual* reload) hold either way |
| public/ files served | **PASS** | `push-sw.js`, nested `icons/*.svg` served at root paths with correct content type (`build/__tests__/dev-fetch.test.ts`, and manually via `curl` against the real server) |
| Cross-origin API calls succeed with NEXT_PUBLIC_API_URL | **PASS** | section S2-routes: `ListSessions`/`WatchSessions`/etc. from `http://localhost:62875` to `http://127.0.0.1:62876/api` succeed (CORS preflight passes once `STAPLER_SQUAD_EXTRA_ORIGINS` names the frontend origin by exact host:port) |
| Type-error and runtime-error overlays appear and clear | **PASS** | section S2-overlay: a syntax error shows Bun's native overlay naming the file and clears on revert; a render-time throw is caught by the app's `ErrorBoundary` ("Something went wrong") and the app recovers after reload |
| `madge` import-cycle count | **informational** | 12 cycles repo-wide (`madge --circular --ts-config tsconfig.json src/main.tsx`, 1332 files). `Omnibar.tsx` (a page that lost Fast Refresh state) sits on 2 of them (`Omnibar.tsx -> OmnibarCreationPanel.tsx`, `Omnibar.tsx -> lib/omnibar/parseSlashCommand.ts`); `SessionList.tsx` (also lost state) sits on 0 — cycles do not fully explain the state loss |
| 200 edits keep RSS under 1.5 GB, no crash | **PASS** | section S2-memory: dev server RSS 291.5 MB before -> 290.1 MB after 200 rapid edits to one file (effectively flat), process alive throughout, 0 crashes |

**Overall: AMBER**, driven by the Fast Refresh criterion. This matches the plan's own pre-built "Amber S2" row (`plan.md`'s Flagged-choices table: "Accept live reload without state-preserving Fast Refresh, or fire T1"); T1 (Vite 8 fallback) fires only on an S2 **red**, which this is not — routes render, 0 uncaught exceptions, live reload works, and the CSS-dependant criterion is a clean PASS. Owner call on whether AMBER is acceptable for cutover (owner: Tyler, per plan).

### S2-routes

44 `ROUTE_META` paths loaded cold via `s2.mjs routes` (not 42; same discrepancy S4 already recorded). All 44: HTTP 200, a `<h1>`/`main`/`[role=main]` landmark present, 0 `pageerror` events. Console-error census across all 44 (`distinct.mjs` tally, deduped by message prefix):
- 44/44: `A non-serializable value was detected in {an action,the state}, in the path: payload.averageAgeSeconds / reviewQueue.reviewQueue.averageAgeSeconds. Value: 0n` (Redux Toolkit's dev-only serializability middleware flagging a proto `int64` field; `src/lib/store/reviewQueueSlice.ts`, `src/gen/session/v1/types_pb.ts`). INFERRED not Bun-specific: this is a frontend/backend data-shape interaction (a BigInt proto field reaching the Redux store) independent of the bundler; not verified side-by-side against `next dev` in this run (time-boxed), but the mechanism (Redux Toolkit's `isSerializable` check) does not consult `WEB_BUILD` or the bundler at all.
- 1/44 (`/notifications`): one additional `Failed to load resource: 404 @ .../SessionService/GetSession` — a stale/seeded session id referenced by notification data on a fresh instance with no real sessions; also not Bun-specific.
- Before the two fixes below (see S2-fixes), the same sweep additionally showed, on every route: a CORS preflight failure (wrong `STAPLER_SQUAD_EXTRA_ORIGINS` host literal in the probe's own setup, not a server defect — fixed by using the exact `localhost`/`[::1]` form the browser sends), a 405 on `/api/client-logs` (no route registered for it), and, on `/test/terminal-stress` only, `Failed to resolve dynamic import '...@xterm/xterm/css/xterm.css'` from a module-scope `require()` CSS import. All three are recorded in S2-fixes below as PRE-EXISTING-ISH gaps this PR closes, not residual S2 findings.

### S2-fastrefresh

Method: open a page, set a real piece of component-local `useState` via the UI, append a comment-only line to the component's source file via `fs.writeFileSync` (not a value change — isolates "does the HMR module swap preserve state" from "does the new code also work"), wait 2s, re-read the state. `page.on("load")` counted across the whole run (0 throughout all 3 cases: no full navigation ever occurred).

| Page | State probed | File edited | Preserved? | Full reload? |
|---|---|---|---|---|
| Session list (`/`) | "Show Archived" checkbox + filters panel open | `src/components/sessions/SessionList.tsx` | **No** — toggle unreachable after edit (panel closed, `filtersOpen` reset) | No |
| Omnibar (global overlay, `Ctrl+K`) | search input text (`s2-hmr-canary`) | `src/components/sessions/Omnibar.tsx` | **No** — input value empty after edit | No |
| `/test/terminal-stress` preset selector (embeds a real `@xterm/xterm` `Terminal` instance — substituted for "session terminal pane": creating a live tmux-backed session was out of scope for this probe, recorded here rather than silently claimed) | `<select>` value (`ASCII_60FPS`) | `src/app/test/terminal-stress/page.tsx` | **Yes** — value unchanged after edit | No |

Bun's own dev-server log corroborates: `Reloaded in Nms: <file>` for each edit, never a full-page request. Omnibar.tsx is on 2 of the repo's 12 `madge --circular` cycles (see verdict table); SessionList.tsx is on 0 — the state loss there has no cycle explanation found in this run.

### S2-csstoken

The plan's AC names `src/styles/theme-contract.css.ts`, but that file is `createThemeContract` placeholders only (no literal values — its own header comment says so); the literal per-theme colour values live in `src/styles/theme.css.ts` (also what `build/config.ts`'s `themeFile` points at). Both were tested:
- `theme-contract.css.ts` + a comment-only append: Bun logs `Reloaded in 6ms: src/styles/theme-contract.css.ts`, 0 full reloads. Nothing to visibly verify (no literal value in the file).
- `theme.css.ts`: `cleanTheme.color.textPrimary` changed from `#e2e8f0` to `#ff00ff`. `getComputedStyle()` on `.page`'s element (`src/app/page.css.ts`'s `color: vars.color.textPrimary`) read the new value (`rgb(255, 0, 255)`) **100ms** after the edit, with 0 full reloads (Bun logs `Reloaded in 21ms: src/styles/theme.css.ts`).

This directly contradicts the plan's pessimistic default assumption ("Bun's `onLoad` has no `addWatchFile`... a memo only makes a *re-requested* dependant correct") for this bundler/version: Bun 1.4.3's `development.hmr` **does** re-request and live-swap a changed `.css.ts`'s compiled CSS into already-open pages. Consequence: `VeCompiler.fullReloadOnCssTsChange` (`build/plugins/vanilla-extract.ts`) defaults to `false` (was `true`), and `build/dev.ts`'s SSE-based full-reload broadcaster now only engages when `WEB_DEV_HMR=0` (HMR fully off).

Re-run through the full dev-stack (`tests/e2e/dev-mode-hmr.spec.ts`, a real backend + `scripts/dev-stack/launch.ts`-spawned `bun build/dev.ts`, `/settings`'s `<h1>`): the colour update is reproduced (within 2s, no manual reload — gate S2's literal wording), but one spontaneous `page.on('load')` event was observed around the edit in that fuller environment, where the standalone measurement above saw none. It is not this app's own reload code (`dev-server.ts`'s SSE hub is never created while HMR is on, per the flip above, confirmed by code inspection) and was not isolated further inside this PR's time budget — the spec records the count as a Playwright annotation (`reload-count`) rather than failing on it, since the AC's two actual requirements (2s budget, no *manual* reload) both hold. Flagged for follow-up, not blocking S2's AMBER-not-RED verdict.

### S2-overlay

Both on `/test/terminal-stress`, introduced then reverted:
- **Type/syntax error** (an unterminated string literal): Bun's native overlay (the `bun-hmr` custom element) appears, names the file (`src/app/test/terminal-stress/page.tsx`) and the error (`Unterminated string literal`), and clears within 1.2s of the revert.
- **Runtime throw** (`throw new Error("s2-runtime-canary")` at the top of the render): caught by the app's own `ErrorBoundary` ("Something went wrong" / "Error Details" / "Retry"), not by Bun's overlay — an allowed outcome per the AC's wording ("Bun's overlay **or** the app's ErrorBoundary"). After a manual reload following the revert, the page recovers (the preset selector is back).

### S2-memory

200 edits to `src/app/test/terminal-stress/page.tsx` (`fs.writeFileSync`, ~60ms apart) while the page was open. Dev server process RSS (`/proc/<pid>/status`, `VmRSS`): 291.5 MB before -> 290.1 MB after (no growth; within host-noise). Process alive throughout (`kill -0` equivalent via a repeat `/proc` read), 0 crashes, 0 restarts needed.

### S2-fixes (code changes made to reach the above, not residual findings)

- **CORS/client-logs/xterm (routing and app bugs, not dev-server design gaps)**: `STAPLER_SQUAD_EXTRA_ORIGINS` must name the exact `host:port` the browser sends (`localhost` and `[::1]` are different origins to the backend's CORS check) — a probe-setup correction, not a server fix. `/api/client-logs` had no route on the dev server at all (the app's `clientLogShipper.ts` posts same-origin `/api/client-logs`, same as production where the Go binary serves both); `build/dev-server.ts` now forwards it to `NEXT_PUBLIC_API_URL` when set (`clientLogsHandler`), else answers 204 so it never shows as a 405 in the console. `src/app/test/terminal-stress/page.tsx` imported `@xterm/xterm/css/xterm.css` via a runtime `require()` inside a `typeof window !== 'undefined'` guard (an SSR-avoidance pattern Bun's bundler cannot resolve for a CSS specifier at runtime); moved to a static top-level `import` (matching `XtermTerminal.tsx`'s existing pattern) — this bug also affects the production Bun build (Story 5.2+), since the page is in the real route table.
- **Port-in-use detection**: `Bun.serve` binds `SO_REUSEPORT`-style instead of throwing `EADDRINUSE` when another *Bun* listener already holds the port, so `assertPortFree` (`dev-server.ts`) probes with a real TCP connect before binding, rather than relying on the bind call to fail.

### Limits of this run

- macOS unverified (Fast Refresh, Bun's CSS HMR behaviour, and the overlay are all plausibly platform-independent, but not run there).
- The Redux `averageAgeSeconds` warning was not verified side-by-side against `next dev` in this run (time-boxed); it is INFERRED non-Bun-specific from the mechanism (a dev-only Redux Toolkit middleware), not confirmed by a parallel measurement.
- Fast Refresh was probed via a comment-only source edit on 3 pages, not via every possible edit shape (e.g. a hook added/removed, a prop type change) or every page in the route table; `SessionList.tsx`'s state loss has no `madge`-visible cycle explanation and was not root-caused further (out of scope for this PR — Bun's own React-refresh integration, not something `web-app/build/` controls).
- "Session terminal pane" was approximated by `/test/terminal-stress` (a real `@xterm/xterm` `Terminal` instance, no live backend session) rather than a tmux-backed session, to stay inside the PR's time budget; this is recorded explicitly rather than claimed as the literal page.
- `madge` was run via `bunx madge` (not added as a `web-app` devDependency); re-running needs network access to resolve the package the first time.

## Gate S3: 42 lazy routes — size, CSS ordering, memory (plan Story 0.4)

Run date: 2026-10-21. Base: `origin/main` `c7d385890` (PR 4 React Router, PR 5a Bun build pipeline, PR 6a flat ESLint config all merged — superseding the S3 prototype scaffolding used by earlier spikes; this gate measures the **real** `web-app/src/main.tsx` entry built by the **real** `web-app/build/build.ts`, not a generated stand-in). Host: Linux (Manjaro), 24 CPUs, 61 GB RAM; **macOS: unverified**. Bun 1.4.3, Node 26.8.1. Host load average ranged 1.7–30 across the run (other agent sessions sharing the host), noted per measurement below; wall-clock numbers are indicative, not canonical. Backend: a ManualInstance on port 62873 (`STAPLER_SQUAD_INSTANCE=s3-gate`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-s3-gate`), empty (no sessions). `:8543` untouched throughout.

Builds measured: `web-app/out` after `bun run build:next` (Next production export, not the `NEXT_BUILD_MODE=development` build `make web-build` uses) and `web-app/out` after `WEB_MINIFY=1 bun build/build.ts` (the real pipeline, PR 5a). Both served statically on ports 62877 (Next) and 62878 (Bun) by `spikes/s3/serve.ts`, which also proxies `/api/*` and `/auth/*` to the ManualInstance (`/api` WebSocket upgrades are **not** proxied — see Limits).

Sources, all under `project_plans/web-app-bun-build/spikes/s3/`: `measure.ts` (BuildStats + Next manifest sizes), `route-loads.mjs` (per-route browser-measured JS/CSS over all 44 `RoutePath`s), `parity-diff.mjs` (`cold`/`matrix`/`walk`/`census` modes — ParityDiff, the 44-route cold run, the navigation-sequence walk, and a secondary CSS census), `css-rules.mjs` (CSSOM rule extraction used by `parity-diff.mjs census`), `walk-detail.mjs` (untruncated final-route diff, used to pin the navigation-walk finding below), `probe-css.mjs`, `probe-tour.mjs`, `probe-terminal-stress.mjs` (targeted repros), `dev-loop.mjs` (Task 0.4f dev-loop baseline), `baseline-timings.sh` + `timed.py` (Task 0.4b/0.4f build-timing protocol; `/usr/bin/time -v` is absent on this host, so `timed.py` uses Python's `resource.getrusage` instead), `serve.ts`, `pw.mjs`, `routes.json`. The official `web-app/build/oracle/census-cli.ts` (shipped by PR 5a) is used as the primary CSS-ordering gate; `parity-diff.mjs census` is a secondary, browser-resolved cross-check.

### Verdict table

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1a | Total JS <= 6,349,052 B (minified) | **PASS** | `.build/stats.json`: `jsBytes: 5680778`. Reproduced identically across 3 separate full builds (first run, a run after `bun install` picked up PR 6a's new devDependencies, and a final confirmation run before this write-up): `5,680,778` B every time. |
| 1b | >= 42 route chunks | **PASS** | `measure.ts`: `distinctRouteChunks: 42`, `routesWithoutChunk: []` — all 42 `routeTable` `load` entries (the 2 `redirect` entries, `/config` and `/settings/defaults`, need none) resolve to a distinct chunk via their page module's position in the sourcemap graph. |
| 1c | `@xterm`, `monaco-editor`, `codemirror`, `recharts` absent from Initial JS | **PASS** | `measure.ts` `forbiddenModulesInInitialChunks`: all 0. |
| 1d | `react-markdown` absent from Initial JS | **RED** | `forbiddenModulesInInitialChunks.react-markdown: 1`. Root cause (§ below): a static (non-lazy) import chain reaches the always-rendered app shell. |
| 2 | 6 `dynamic()`/`lazyComponent()` call sites produce lazy chunks | **PASS** | `measure.ts` `lazySites`: `DailySpendChart`, `ModelBreakdownChart`, `ModelOverTimeChart`, `StageCostChart` (all from `InsightsDashboard.tsx`), `SessionDetailView` (`SessionDetail.tsx`), `TerminalOutput` (`SessionDetailView.tsx`) — each resolves to its own chunk, `inInitial: false` for all 6. |
| 3 | Build wall <= 10 s (warm median, n >= 5); 3 concurrent builds, peak RSS <= 2 GB | **PASS** | 5 serial runs of `WEB_MINIFY=1 bun build/build.ts`: wall 1.30–1.39 s (median 1.33 s), peak RSS 736–798 MB. 3 rounds of 3 concurrent builds (9 builds total): wall 1.57–1.78 s, peak RSS 746–822 MB, all exit 0. No persistent build cache exists (S1's finding), so cold and warm are the same measurement here. |
| 4 | Baseline timing protocol exists and runs (n >= 5 cold/warm, Next dev-mode/prod/Bun, load avg recorded) | **PASS** | `baseline-timings.sh` ported to call `bun build/build.ts` (PR 5a) in place of the retired spike bundler; see § Baseline timings. |
| 5 | Dev-loop baseline (`next dev` cold start, edit-to-DOM, n >= 5) | **PASS** | `dev-loop.mjs`; see § Dev-loop baseline. Bun's own dev server (`dev:bun`) is not yet built (`PR 5b`; `package.json`: `"echo 'The Bun dev server lands in PR 5b...'; exit 1"`), so only the `next dev` "before" number is recorded, as the plan's Task 0.4f scopes it. |
| 6 | Footprint baseline (package counts, `node_modules` size, install cold/warm) | **PASS**, n reduced | See § Footprint baseline; host load during the install runs was 27–30 (other agent sessions), so only 1 cold + 1 warm run were taken, not 5. |
| 7 | CSS census: resolved custom-property map and cascade-winner parity | **PASS** | `bun build/oracle/census-cli.ts --next <next-out> --bun <out>`: exit 0, `themeDiffs: []`, `cascadeDiffs: []` (4,606 of 6,257 common selectors moved — reported, not failed, per the tool's own design). |
| 8 | ParityDiff, 5 routes x 2 viewports x 6 themes, computed styles + interaction states | **AMBER** | Core (computed styles, resolved custom-property map) is clean once the allow-listed `background-position` notation diff is excluded. A real, deterministic, unallow-listed divergence was found in the focus/hover walk around the backlog tour/onboarding overlay (§ below) — not yet in the allow-list, no linked reason exists for it. |
| 9 | All 42 routes cold, 1 viewport x 2 themes (`clean`, `dark`), zero unexplained diffs | **RED** | 48 unallow-listed prop diffs: 34 font-family (explained by the font strategy, § below — a candidate allow-list addition, not a defect) and **14 `pointer-events` diffs on `/backlog` alone (7 elements x 2 themes), the same backlog-tour-modal bug as §8/§10, now shown present on a plain cold load — not only after interaction-state probing or a 44-route walk**. Plus 124 structure diffs: **36 are the `/test/terminal-stress` route crash** (§ below) and 88 are `route-announcer` (present only in the Bun DOM — the announcer region itself, not its content, differs in count because Next's own `<next-route-announcer>` lives in a separate shadow root this scan does not see). The font-family and announcer-presence diffs are candidate allow-list entries; the pointer-events diff and the terminal-stress crash are not. |
| 10 | Navigation-sequence walk (3 fixed + 3 seeded-random orderings, all 42 routes, no reload): final route's computed styles equal the same route cold-loaded on the same Bun build | **RED** | Reproduced on 2 of 2 orderings tested in detail (ascending, descending): after visiting all 44 routes with no reload, the final route shows `pointer-events: none` on dozens of chrome elements (and, in the descending case, on the route's own pane/window elements) that read `pointer-events: auto` on a fresh cold load of the same route. § below. Root cause not identified within this gate's scope. |
| 11 | Next per-route first-load JS, largest, and today's 5 MB `size-limit` gate status | **PASS** (fully measured) | § Next baseline. Today's gate **passes**: brotli Total JS 1,828,027 B against the 5,000,000 B limit. |

### 1d, react-markdown in the Initial JS: root cause

`measure.ts`'s sourcemap walk of the entry chunk finds 136 `react-markdown`/`micromark`/`mdast`-family sources. Tracing the one non-`node_modules` match (`src/components/backlog/BacklogItemForm.tsx`, which imports `react-markdown` directly, not lazily) up the import graph: `src/app/Providers.tsx` renders `<Omnibar>` at the app-shell level (every route, not behind a route boundary) -> `src/components/sessions/Omnibar.tsx` renders `<BacklogComposeWidget>` -> `src/components/backlog/BacklogComposeWidget.tsx` imports `BacklogItemForm` -> `BacklogItemForm.tsx` imports `react-markdown` at module scope. None of these four imports go through `lazyComponent()`/`dynamic()`, so Bun's bundler correctly puts the whole chain in the entry chunk — this is an application-architecture gap (the Omnibar's "compose a backlog item" quick action pulling in a markdown editor unconditionally), not a Bun bundler defect, and it predates this plan (Next's `next/dynamic` has the same non-lazy import today; Next's bundle just doesn't have a comparable "Initial JS must exclude X" gate to catch it). Fix is a Story 2.x/5.x task: wrap `BacklogItemForm`'s markdown-dependent subtree in `lazyComponent()`, or lazy-load `BacklogComposeWidget` itself from `Omnibar.tsx`.

### 8, Interaction-state ParityDiff: backlog tour/onboarding overlay

The matrix run (`parity-diff.mjs matrix`, 5 routes x 2 viewports x 6 themes x 5 focus + 5 hover states = 60 configs) found two related, deterministic (not timing-flaky — confirmed with `probe-tour.mjs`, 4 repeats x 4 wait times, 0/16 variance within a build) differences, both on `/backlog` and `/`:

- `backlog-tour-modal`'s inline `style` attribute is `pointer-events: none` on every Next load and `pointer-events: auto` on every Bun load (and the same for `backlog-lifecycle-node-*`/`backlog-lifecycle-diagram` inside it). **This is not interaction-state-specific**: the plain `/backlog` cold-load run (§9, row 9 of the verdict table) shows the identical 7-element, 2-theme diff with no focus/hover probing at all — the matrix run's contribution is confirming it is unaffected by theme, viewport or interaction state, not discovering it.
- The 4th tab-stop's accessible name differs: `"Skip onboarding"` (Next) vs. `"Skip tour"` (Bun) — i.e. the two builds land on a **different button** at the same point in the focus order, which cascades into further `outline-color`/`outline-width`/`outline-offset` diffs on that element and the adjacent one (both from the standard focus ring, which differs because a different element has it).

Not root-caused within this gate's scope (two different first-run onboarding components appear to exist — `BacklogTourModal` ("Skip tour") and an "onboarding" modal ("Skip onboarding") — and something about which one mounts, or their relative DOM order, differs between the two builds on a fresh `localStorage`). Flagged for Story 4.x/5.10 investigation; not currently in `parity-allow.json`-equivalent form because no reason has been established to link.

### 9, `/test/terminal-stress`: a genuine Bun bundler cross-chunk-import bug

Cold-loading `/test/terminal-stress` on the Bun build throws `ReferenceError: __commonJS is not defined` at `page-<hash>.js:1`, caught by React Router's `errorElement` (`"Something went wrong"`); the Next build renders the page normally (`<h1>Terminal Streaming Stress Test</h1>`). Reproduces identically minified and unminified (`probe-terminal-stress.mjs`; confirmed with `WEB_MINIFY=0 bun build/build.ts`, unminified name `__commonJS`, minified name `Xe`).

Root cause, read from the emitted chunk (`page-<hash>.js`'s first line): the page's static (non-lazy) top-level `import { Terminal } from "@xterm/xterm"` etc. — guarded only by `typeof window !== "undefined"`, the pattern `src/app/test/terminal-stress/page.tsx` uses instead of `lazyComponent()` — pulls in `@xterm/xterm/css/xterm.css` as a CommonJS-wrapped side-effect module (`var require_xterm2 = __commonJS(function(exports, module) {});`, called unconditionally inside the `if` block). Every **other** chunk that calls `__commonJS(...)` correctly imports it, either defining it directly (the shared runtime chunk) or importing it from that chunk — `index-h6gcce2d.js`, `index-za9r61x4.js`, `index-2przg7j7.js`, `index-bxh8j64r.js`, `index-qpjarfj4.js`, `XtermTerminal-<hash>.js`, `page-fy7xe0v0.js`, `SessionDetailView-<hash>.js`, and `TerminalOutput-<hash>.js` (the last two serving the **lazily-loaded**, correctly-working `@xterm/xterm` usage behind `SessionDetailView` -> `TerminalOutput` -> `XtermTerminal`) all import `__commonJS` where they use it. Only `page-rx9s40np.js` (the `/test/terminal-stress` route chunk) omits it from its import statement while still calling it — Bun's splitter drops this one cross-chunk import. This matches the plan's own note that this route is a "4th importer" that "only has an unused import" and was deliberately not migrated to `lazyComponent()`; this bug is evidence for why not, not a reason this spike introduced.

This single route accounts for all 36 of the non-font, non-announcer structural diffs in the all-44-routes cold run (18 `data-testid` elements present on Next and absent on Bun x 2 themes).

### 10, Navigation-sequence walk: pointer-events get stuck after extended SPA navigation

`parity-diff.mjs walk`'s own aggregate scoring under-reports this (its `final.vsBunColdDiffs` read 3–7 per ordering; not reconciled within this gate's scope — `walk-detail.mjs`, an untruncated per-key diff of the same walk, is what actually surfaced it and is the citable evidence below). Procedure: on the Bun build, `history.pushState` + a synthetic `popstate` dispatch through all 44 `RoutePath`s (ascending and descending `RouteMeta` order, no reload), then snapshot every `data-testid` element's computed style and diff against a **fresh cold load of the final route only**, on the same build:

- **Ascending** (final route `/workflows`): 33 elements read `pointer-events: none` when walked vs. `auto` cold — all in the persistent app shell (`drawer-nav`, the whole notification `tray-*` subtree, both `announcer-*` regions, `route-announcer`). Plus 2 small `opacity` diffs on notification badges (0.94 vs 0.90-ish; consistent with mid-pulse-animation sampling, not investigated further).
- **Descending** (final route `/`): 58 elements, now **also** including the route's own content (`window-tab`, `window-add-button`, `pane-leaf`, `pane-header`, `pane-view-toggle-btn`) — the whole UI reads non-interactive, not just the persistent chrome.

Both are 100% reproducible (re-run twice each, identical element sets). Not root-caused: no global `pointer-events: none` CSS rule exists in the source (`globalEffects.css.ts`'s `html::before` scanline overlay is permanently `pointer-events: none` and unrelated, its own ::before pseudo-element); this is an inline style, meaning some component's JS sets it, most plausibly a "navigation pending"/overlay-lock flag (`src/lib/nav/entry.tsx`'s `PendingFallback` sets/clears `aria-busy` on `<main>` per navigation) whose cleanup assumes the full-page remount Next's router always did between navigations — an assumption PR 4's persistent `RootShell` (which "sits inside the router... and outlives every navigation") breaks. This is squarely pre-mortem P1-1's concern, realised as an interactivity bug rather than a literal CSS-rule-order bug — the static CSS itself is fine (census-cli.ts PASS), but the **rendered, navigated-through** app becomes unusable. Flagged as the clearest T4 trigger-candidate in this gate.

#### Root-cause follow-up (2026-10-22): confirmed false positive, not a Bun/router/RootShell bug

CDP-level instrumentation (`DOMDebugger.setDOMBreakpoint(type: "attribute-modified")` on `document.body`, Playwright's `newCDPSession`, since monkey-patching `CSSStyleDeclaration.prototype`'s accessor and `setProperty`/`cssText` never intercepted the write — Chromium's inline-style bindings don't route through a JS-overridable prototype path) caught the exact call site: `@radix-ui/react-dismissable-layer`'s mount effect, `ownerDocument.body.style.pointerEvents = "none"` (`disableOutsidePointerEvents` branch), the standard mechanism `Dialog.Content` uses to block background clicks while a modal is open.

The modal is `OnboardingModal` (`src/components/onboarding/OnboardingModal.tsx`, via `useOnboarding.ts`): it auto-opens **800ms after initial mount** on any session lacking the `stapler-squad:onboarded` localStorage key — true of every fresh Playwright context, including this gate's own `walk`/`walk-detail.mjs` scripts, which (unlike `tests/e2e/terminal-stress/helpers.ts`'s `setupStressTestPage`, added in PR #1141 for the identical reason) never seeded that key. A 44-route walk at ~120ms/hop comfortably crosses the 800ms threshold before the final snapshot; the walk never dismisses the tour (no Escape/Skip/Get-Started), so the modal — and the body lock it correctly owns while open — is still there when the snapshot runs. The **cold-load baseline**, sampled ~300ms after load, is taken *before* the timer fires, so it never sees the tour and reads `pointer-events: auto`. That timing mismatch between the two sides of the comparison, not anything about route order, Bun's CSS bundling (#45730), or `RootShell` persistence, explains 100% of the diff.

Confirmed, not inferred: with `stapler-squad:onboarded` seeded via `page.addInitScript` before `goto` (`parity-diff.mjs`'s `newPage`, and `walk-detail.mjs`, both patched below), the identical 44-route walk — both orderings, same Bun build, same ManualInstance — now shows **zero** `pointer-events`/structural diffs on `drawer-nav`, the `tray-*` subtree, `announcer-*`, `route-announcer`, or route content; the only residual diffs are the 2 pre-existing notification-badge `opacity` readings already flagged above as animation-phase sampling noise, unchanged by this fix. Separately, with the tour open, pressing Escape confirmed `DismissableLayer`'s cleanup restores `body`'s style attribute to `""` correctly (`dialogCount` 1→0, `bodyStyle` `"pointer-events: none;"` → `""`) — the lock is not actually "stuck" for a real user who interacts with the tour even once; it only reads as permanently stuck to a walk script that never does.

**Verdict: T4 does not fire.** This is a gap in the gate's own repro tooling (no onboarding suppression), not a defect in the Bun build, the React Router entry, or `RootShell`'s persistence. Fixed in `parity-diff.mjs` (`newPage`'s existing `addInitScript` now also sets `stapler-squad:onboarded`) and `walk-detail.mjs` (both pages now call a `suppressOnboarding` helper before `goto`), mirroring the precedent in `tests/e2e/terminal-stress/helpers.ts`.

### 11, Next baseline and the budget-relaxation figure

Today's `web-app/.size-limit.json`/`package.json` `size-limit` gate, run exactly as `benchmark.yml` runs it (`bunx size-limit` against the Next production export `out/`): **passes**, Total JS bundle 1,828,027 B (brotli) against the 5,000,000 B limit, Framework chunk 49,388 B against 650,000 B, Main app chunk 29,333 B against 400,000 B — all 3 rows pass with wide margin.

Next's own `next build` output (`out/`, the production export, not the `NEXT_BUILD_MODE=development` build): "First Load JS shared by all" 104 kB; largest per-route "First Load JS" is `/backlog` and `/` at 546–547 kB. **These are brotli/gzip-scale figures, not raw bytes** (confirmed: they track my own gzip-byte measurements below to within a few percent, not my raw-byte measurements, which are 4x larger) — a unit a future reader could easily conflate with the plan's raw-byte `Total JS`/`Initial JS` budgets. Independently measured, in both units, over all 44 routes (`route-loads.mjs`, actual browser network bytes; `measure.ts`, Next's `app-build-manifest.json` static analysis — the two methods agree to within a few percent):

| | Bun | Next | Bun vs Next |
|---|---:|---:|---:|
| Total JS (raw, all chunks) | 5,680,778 B | 6,757,063 B (chunks dir only), 6,759,298 B (all static) | Bun **16.0% smaller** |
| Total JS (gzip, all chunks) | 1,829,011 B | 2,115,825 B | Bun **13.6% smaller** |
| Max per-route JS (raw, browser-measured) | 2,629,977 B (`/backlog`) | 2,626,012 B (`/backlog`) | within 0.2% |
| Max per-route JS (gzip, browser-measured) | 804,437 B (`/backlog`) | 790,526 B (`/backlog`) | Bun 1.8% larger |
| Initial/entry JS (raw) | 1,494,183 B | n/a (no single "entry"; Next's own largest per-route "First Load JS" is 547 kB brotli-scale) | — |

So: the plan's 6,349,052 B `Total JS` ceiling is **not a relaxation of today's enforced gate** (that gate measures brotli bytes and passes today at 1,828,027 B — the two numbers are not directly comparable, being different units and different measurement scopes) — it is a **new raw-byte metric** that did not exist before, set from Bun's own measured raw total (5,680,778 B) with headroom. Compared on the same (raw) basis, Bun's build is smaller than Next's today, in total (16.0%) and roughly tied per-route (within 0.2–1.8% either direction). Story 5.9's "Initial JS ceiling = Bun value x 1.05, must not exceed largest Next first-load x 1.10" rule: `1,494,183 x 1.05 = 1,568,892` (matches `budgets.json`) against the Next-side comparator — if the comparator is Next's own printed "largest First Load JS" taken literally (547 kB, brotli-scale) x 1.10 = ~601,700 B, Bun's raw 1,568,892 B figure would read as **over** that number, which is comparing raw bytes against a compressed-size estimate and would be a false RED; if the comparator is read in the same (raw) units as Bun's figure — Next's largest per-route raw first-load, 2,626,012 B (browser-measured) or 2,103,356 B (manifest-derived) x 1.10 — Bun's 1,568,892 B is comfortably under. **This unit mismatch is itself the finding**: Story 5.9/ADR-005 must pin which of these four Next-side numbers (brotli-scale printed, gzip-byte, raw-byte-manifest, raw-byte-browser) is "the largest Next per-route first-load JS" before the rule in `budgets.json`'s comment and in `ADR-005` can be applied without ambiguity; this gate could not resolve it because the plan text does not say which.

### Baseline timings (Task 0.4b/0.4f)

`baseline-timings.sh` (ported from the retired spike bundler to `bun build/build.ts`, PR 5a) protocol: n >= 5 per mode per temperature, median and min-max range, host load average at each phase's start.

| Mode | Temp | n | Wall median | Wall range | Peak RSS median | Peak RSS max |
|---|---|---:|---:|---:|---:|---:|
| Bun build (real pipeline) | cold = warm (no persistent cache) | 5 | 1.33 s | 1.30–1.39 s | 772 MB | 798 MB |
| Bun build, 3 concurrent x 3 rounds | warm | 9 | 1.69 s | 1.57–1.78 s | 791 MB | 822 MB |
| Next production export (`bun run build:next`) | cold | 5 | 70.31 s | 66.46–101.31 s | 4,437 MB | 4,565 MB |
| Next production export | warm | 5 | 39.66 s | 37.25–53.09 s | 2,569 MB | 2,757 MB |
| Next dev-mode build (`make web-build`'s command) | cold | 5 | 63.46 s | 60.16–70.65 s | 4,319 MB | 4,377 MB |
| Next dev-mode build | warm | 5 | 36.92 s | 36.06–37.25 s | 2,438 MB | 2,651 MB |

The 4 Next rows were measured once, at `main` `b9258fe35` (before PR 4/5a/6a), host load 3.7–16.1 across the run. They were **not** re-measured to the full n >= 5 protocol after the merge: Next's build path is architecturally insulated from PR 4 (the nav-facade ESLint rule bans `react-router*` imports outside `src/main.tsx`/`src/app/routes.tsx`/tests, so `next build`'s webpack bundle cannot see PR 4's new code) and from PR 5a/6a (neither touches `next.config.ts` or anything Next's webpack graph reads); a spot-check `bun run build:next` run post-merge completed in a comparable ~70 s wall, consistent with no detected regression, but was not independently timed to the n >= 5 protocol. The Bun rows are post-merge, from the real pipeline. Bun/Next ratio (warm, raw wall): about **30x** faster (1.33 s vs 39.66 s against the production export; vs 36.92 s against the dev-mode build `make web-build` actually runs) — both far inside Story 8.3's 5x gate, with wide margin for host-load noise.

### Dev-loop baseline (Task 0.4f)

`next dev` (`PORT=62876`, `NEXT_PUBLIC_API_URL` pointed at the ManualInstance), n=5, host load 1.7–9.9:

| Metric | Median | Range |
|---|---:|---:|
| Cold start to first 200 on `/` | 23.5 s | one run; a second independent run earlier in this gate measured 13.0 s — high variance, both reported |
| Comment-only edit (`history/page.tsx`'s `<h1>`) to DOM update | 566 ms | 489–1,108 ms |
| `.css.ts` token edit (`theme.css.ts`'s `cleanTheme.color.cardBackground`) to computed-style change | 10,676 ms | 7,651–14,926 ms |

Bun's own dev server does not exist yet (`dev:bun` errors by design, PR 5b), so there is no Bun-side comparison in this gate; these are the "before" numbers Story 0.3/5.x's dev-loop work will be compared against. The `.css.ts` edit path (~8–15 s) is far from the plan's "within 2 s" target for the eventual Bun dev server — expected, since this measures `next dev`'s own HMR path, not anything Bun-specific.

### Footprint baseline (Task 0.4f)

Measured against `main` `c7d385890` (post-PR-6a, which added `eslint-plugin-react`/`-react-hooks`/`-jsx-a11y`/etc. as devDependencies):

| Metric | Value |
|---|---:|
| Direct `dependencies` | 65 |
| Direct `devDependencies` | 59 |
| `bun pm ls --all \| wc -l` | 1,902 |
| `du -sk web-app/node_modules` | 1,416,080 KB (~1.35 GB) |
| `bun install --frozen-lockfile`, cold cache | 32.68 s (n=1) |
| `bun install --frozen-lockfile`, warm cache | 0.34 s (n=1) |

Host load was 27–30 during the install runs (other agent sessions), so only 1 run per temperature was taken, not the n >= 5 protocol; both numbers are plausible but noisy. These are the "before" row for Story 8.3's table; Story 9.2's dependency removals have not landed, so there is no "after" yet.

### Not measured / limits of this S3 run

- macOS anything.
- The `walk` mode's own aggregate `vsBunCold` scoring under-counts the navigation-walk finding (§10) for a reason not diagnosed in this gate's scope; `walk-detail.mjs`'s untruncated, independently-written comparison is the citable evidence instead. Fixing `parity-diff.mjs`'s aggregate path is flagged, not done.
- The root cause of the backlog-tour/onboarding focus-order divergence (§8) and the navigation-walk pointer-events lock (§10) are both identified down to "some stateful component/effect," not to the specific line of code.
- `parity-diff.mjs walk` ran all 3 fixed orderings (ascending, descending, `RouteMeta` declaration order) and 3 seeded-random orderings (seeds `20261021`, `1337`, `424242`) per the plan, but only 2 of the 6 (ascending, descending) were re-verified in full untruncated detail via `walk-detail.mjs`. The other 4 orderings' aggregate (under-counted) `final.vsBunColdDiffs` scores were also nonzero (7, 7, 7, 7 for routemeta-declaration and the 3 random seeds respectively) but not independently confirmed with the untruncated tool.
- Footprint and dev-loop cold-start numbers are n=1/n=1 and n=5-with-one-outlier respectively, below the plan's n >= 5 protocol, because of host load from concurrent agent sessions on this machine.
- `WEB_BUILD=bun make web-build` (the actual Makefile target, as opposed to `bun build/build.ts` invoked directly) was not separately timed; `build/build.ts` is the only moving part `WEB_BUILD=bun` adds over the already-measured `make web-build`'s existing `proto-gen`/`ensure-tools` steps, so no material difference is expected, but it was not measured.
