# Web-app Bun build: gate record (GateRecord)

Gates S0 and S1 only. S2 to S4, the owner sign-offs (Story 0.6) and the trigger decision are **not** in this file yet.
Measured 2026-10-21 on Linux x86_64 (Manjaro), Bun 1.4.3 (also 1.3.11 for S1), Node 26.8.1, `main` at `b9258fe35`.
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
- Fast Refresh, dev server (S2), 42-route sizes and CSS ordering (S3), React Router (S4).
