# Replacing Next.js in `web-app/`: Bun feasibility study

Status: research only. Nothing in the repo was changed except this file. No migration PR is proposed until the owner decides.

## Recommendation

**Do not move `web-app/` to Bun's bundler or test runner today.** Both spikes hit blockers inside Bun itself (section 3). **Rsbuild is the best bundler fit**: it built the same code in 6.2–6.6 s including vanilla-extract (Next: 54–116 s) using the official `@vanilla-extract/webpack-plugin`, with no custom build code. Vite also works (about 16 s) but is slower here and gains nothing over Rsbuild for this app.

For the router, **TanStack Router in SPA mode (not TanStack Start)** is the strongest candidate, because the URL state is real (22 `useSearchParams` uses, about 25 distinct keys) and typed search params are its main advantage. A hand-rolled "native React" router is not recommended (section 8). The router choice is independent of the bundler choice and is the larger piece of work.

Bun is still worth reconsidering later as a package manager or script runner. This study did not evaluate that beyond one clean `bun install` (11 s).

## 1. Confirmed: no server-side Next.js features

| Check | Result |
|---|---|
| `next.config.ts` | `output: "export"`, `trailingSlash: true`; the rest is webpack tuning (cache dir, CSS chunk consolidation) |
| `src/pages/` | does not exist |
| Route handlers (`route.ts*`), `middleware.ts*` | none |
| `getServerSideProps` / `getStaticProps` / `generateStaticParams` / `next/headers` / `next/server` / `"use server"` | none (the only hits were `revalidateWindows` in `useWindowManager.ts` and a proto comment) |
| Dynamic route segments (`[id]`) | none; all 41 `page.tsx` files are static |
| Client components | 519 files contain `"use client"` |
| Go side | `server/middleware/static.go` already falls back to `index.html` for unknown paths, so SPA routing needs no Go change |

Next features the code does use, which any replacement must cover:

- `next/navigation` (production code only, tests excluded): `useRouter` (26 uses, 66 `router.push`/`replace` calls), `useSearchParams` (22), `usePathname` (6), `redirect` (2). 63 production files import `next/navigation` or `next/link`; `next/link` is in 24 of them. `src/lib/navigation/NavLink.tsx` already wraps `next/link` for part of the app.
- `next/link` (23 files), `next/dynamic` (4), `next/font/local` (1 file, four vendored families).
- `next/dist/client/web-vitals` (an internal path) in `WebVitalsReporter.tsx`.
- `export const metadata` / `viewport` in about 10 files, and `layout.tsx` nesting.
- Per-route HTML under `trailingSlash`. The Go server serves `dir/index.html`.
- Tooling tied to Next: `@storybook/nextjs`, `eslint-config-next`, `size-limit` globs on `out/_next/...`, the `Makefile` `web-app/out` target and its webpack cache env var, and 31 matching lines across CI, Playwright and Storybook config.

## 2. Measurements

Baseline: Next 15.3.2, run in an isolated copy of `web-app/` in the session scratchpad so the live instance and the checkout were untouched. Machine: this Linux host, Node v26.8.1, Bun 1.4.3. Each number is one run unless a range is given.

| Build | Wall time | JS bytes | CSS bytes | JS gzip -9 |
|---|---|---|---|---|
| Next, dev mode (what `make` uses), cold | 88.5 s | 21,666,571 | 813,686 | not measured |
| Next, dev mode, warm | 54.0 s | same | same | n/a |
| Next, production, cold | 115.7 s | 6,349,052 (151 files) | 648,758 | 1,907,594 |
| Next, production, warm | 60.6 s | same | same | n/a |
| Vite 7 + vanilla-extract plugin, spike | 15.6 s and 16.1 s | 5,010,907 (33 files) | not measured | 1,531,427 |
| Bun.build, spike, single file | 0.3–0.6 s, plus a separate vanilla-extract prepass (first run 6.3 s cold; 2.1–2.8 s warm under Node, 1.9–2.4 s under Bun with the `eval` fix in section 3) | 5,431,639 | 626,365 | 1,658,102 |
| Bun.build, spike, split | 0.3 s, plus the same prepass | 5,429,699 (45 files) | same | 1,659,497 |

`make` builds Next in **development mode** (unminified, about 21.7 MB of JS), so the deployed bundle is far larger than the production figure.

How to read the spike numbers (they are not an apples-to-apples comparison):

- The spike entry imports all 41 pages and the root layout into one app. The `next/*` imports are replaced by small shims.
- It produces no per-route HTML, and it did not implement Next's font or metadata handling.
- Next's numbers include prerendering 41 routes.
- Bun's time excludes the vanilla-extract prepass, which runs every `.css.ts` file through `compile()` + `processVanillaFile()`: 336 files, 0 failures. My first run took 6.3 s; repeat runs with identical settings took 2.1–2.8 s under Node and 1.9–2.4 s under Bun (with the `eval` fix). Treat 6.3 s as a cold-run upper bound, not the floor. The Vite 8 and Rsbuild times (about 6 s) include vanilla-extract.
- **Runtime behaviour was not verified.** The Bun bundle reached a React render (an early load produced a `process is not defined` error, fixed with `define`). The next load crashed the browser tab, plausibly because the spike mounts all 41 pages at once with no router or backend. Do not read the bundle sizes as proof that the app works.
- Size comparison: Bun's JS is 14% below Next production; Vite's is 21% below. Treat that as indicative only, for the reasons above.

Test runner (the existing `jest.config.js` `web-app` project):

| Runner | Result |
|---|---|
| Jest 30 + ts-jest | 620 suites, 7,683 tests, 55.7 s wall; 5 suites and 12 tests fail in the scratch copy. Not investigated, so possibly a copy artifact. |
| `bun test` + happy-dom + a preload plugin standing in for `moduleNameMapper` | **Bun segfaulted** ("Segmentation fault at address 0x0") after about 90 s with 125 reported failures. The run never completed, so there is no comparable pass count. |

## 3. Findings that decide the question

1. **vanilla-extract fails under Bun because of one small dependency, not its design.** `compile()` bundles each `.css.ts` with esbuild; `processVanillaFile()` then evaluates the bundle with the `eval` npm package (0.1.8), which builds its `require` from `require-like`, which calls Node-internal `Module._load`. In Bun, `Module._load` exists but returns `undefined` (probe: `_load('path')` is an object in Node and `undefined` in Bun), so `require('@vanilla-extract/css/adapter')` is `undefined` and the destructure of `setAdapter` throws. Replacing `eval` with a 10-line `vm` + `createRequire` shim made all 336 `.css.ts` files compile under Bun with 0 failures, and the decoded JS and CSS output were identical to Node's for all 336. (An earlier version of this doc blamed vite-node; that was wrong. `compile()` uses esbuild, not vite-node, per `packages/integration/src/compile.ts` in vanilla-extract 8.0.11.) The shim was tested by overwriting the file inside the spike's `node_modules`; shipping it would need a `pnpm patch` or an override, plus an upstream fix. I did not check vanilla-extract's issue tracker.
2. **Latent CSS bug that a stricter bundler exposes.** Four files emit invalid nested `@keyframes` blocks (a `"@keyframes"` key inside `style()`): `src/app/unfinished/UnfinishedTab.css.ts`, `src/components/unfinished/UnfinishedItemDetail.css.ts`, `src/components/backlog-stuck/StuckItemsSection.css.ts`, `src/components/backlog-stuck/StuckNavBadge.css.ts`. The current production CSS contains `@keyframes{stuckBadgePulse{0%,...` inside a rule, which webpack passes through. Bun's CSS parser rejected it with "Unexpected end of input". Whether browsers ignore the animation today is inferred, not tested. Moving these to `keyframes()` is worth doing regardless of this migration.
3. **`bun test` is not a drop-in for Jest here.** Jest-style `jest.mock` and `moduleNameMapper` need replacing, jsdom needs happy-dom, and the full-suite crash is a blocker on its own. Per-file process isolation in Jest is also something Bun's single-process default does not give.
4. **Next gives little that is used, and costs a lot to remove.** About 63 files of router usage (plus 40 test files that mock `next/navigation`) and the Storybook/ESLint/size-limit coupling are the bulk of the work.
5. **"More reproducible" is not a bundler property.** The lockfile and the shared `node_modules` store drive reproducibility. Bun's `bun.lock` would not improve on `pnpm-lock.yaml`, and `docs/how-to/use-pnpm-in-web-app.md` documents why the repo is pinned to pnpm.

## 4. Bundler and test-runner alternatives

| Option | Build time (spike) | JS bytes (files) | JS gzip | vanilla-extract | Main cost |
|---|---|---|---|---|---|
| Stay on Next (production) | 60.6–115.7 s | 6,349,052 (151) | 1,907,594 | works | none; keep paying build time |
| **Rsbuild + `@vanilla-extract/webpack-plugin`** | **6.2, 6.4, 6.6 s** (3 runs) | 5,004,886 (35) | 1,472,342 | official docs path; worked first try | router replacement |
| Vite 7 + `@vanilla-extract/vite-plugin` | 15.6, 16.1 s | 5,010,907 (33) | 1,531,427 | official plugin; worked first try | router replacement |
| Bun.build | 0.3–0.6 s + 2–6 s vanilla-extract prepass | 5,429,699 (45) | 1,659,497 | custom plugin + patched `eval` package | blockers in section 3 |

Rsbuild's CSS output was 562,854 bytes. Its minifier also dropped the invalid nested `@keyframes` block silently (0 occurrences in the output), which hides the bug rather than reporting it. PR #1097 fixes the source.

Test runners, tried on the 190 of 620 test files that use no `jest.*` API (so they need no codemod):

| Runner | Result |
|---|---|
| Jest 30 (baseline, all 620 files) | 7,683 tests, 55.7 s, 12 failures in the scratch copy |
| `bun test` (all 620 files) | segfault after about 90 s, 125 failures reported |
| Rstest (190 jest-free files, jsdom, Rsbuild plugins) | 2,704 passed, 19 failed, 35 s wall. One failure class seen: a byte-array `toEqual` in a protobuf test; the other 18 were not triaged. |

The other 430 files use `jest.*`: 1,614 `jest.mock` calls in 309 files, 3,917 `jest.fn`, 254 `jest.spyOn`, 146 `jest.useFakeTimers`, 42 `jest.requireActual`. Rstest and Vitest both need those rewritten (`rs.mock`/`vi.mock`), probably with a codemod. Staying on Jest costs nothing and Jest does not depend on the bundler, so the test runner should be a separate, later decision.

## 5. Risks

| Risk | Severity |
|---|---|
| Router replacement across 63 files changes URL, history and query-param behaviour (22 `useSearchParams` uses, about 25 distinct keys) | High |
| Per-route HTML and `trailingSlash` assumptions in the Go server, Playwright specs and deep links | Medium |
| Storybook uses `@storybook/nextjs`; needs `@storybook/react-vite` | Medium |
| Jest to Vitest or Bun test: mocks for `next/*`, `jest.mock` hoisting, jscpd gate noise | Medium |
| CSS/theme FOUC script in `layout.tsx` depends on build-time VE class names | Medium |
| `size-limit` paths, `Makefile` target, CI caches, `eslint-config-next` rules | Low, mechanical |
| Dev mode today is unminified and `make` relies on it for error messages; a new build needs an equivalent switch | Low |
| Bun-specific: runtime crash in `bun test`, VE incompatibility, young ecosystem | High for Bun, none for Vite |

## 6. Staged path (if the owner wants to proceed)

1. Merge PR #1097 (the four `@keyframes` files). Done independently of any migration.
2. Add the navigation facade (section 7) and move the 63 production files and 40 test mocks onto it, under Next.
3. Spike Rsbuild properly with the real router: TanStack Router file routes, fonts, metadata, `Makefile` target, Go embed, Storybook.
4. Decide on the test runner last, using a real pass count.

## 7. Navigation facade: what it is for

A facade is one module, for example `@/lib/nav`, that exports `useRouter`, `useSearchParams`, `usePathname`, `redirect` and `Link`. Today it would just re-export the `next/*` versions. Its only job is to make later router changes a one-file edit instead of a 63-file edit plus 40 test-mock edits.

- **Why do it first:** it can ship under Next, it changes no behaviour, and it can be reverted. It separates "which router" from "which bundler".
- **When it is not worth it:** if you decide to stay on Next, the facade buys only a tidier test-mock surface. `NavLink`/`AppLink` already do this for links.
- **What goes in it:** the Next API surface the app really uses (list in section 1), not a general router abstraction. Typed search-param helpers can be added there before the TanStack switch.

## 8. Router options: React, TanStack Router, TanStack Start

| Option | Fit for this app | Notes |
|---|---|---|
| **TanStack Router, SPA** | Best | File-based routes, typed `search` params. Its bundler plugin supports Rspack/Rsbuild with `autoCodeSplitting`, per its [Rspack install guide](https://tanstack.com/router/latest/docs/framework/react/installation/with-rspack). Static output: one `index.html`; Go already falls back to it. |
| React Router 7 library mode | Good | Mature and familiar, but its typed-route features are framework-mode only, per a January 2026 comparison I could not verify against the docs. Would need the same search-param validation written by hand. |
| TanStack Start | Poor | Full-stack framework (SSR, server functions). We have no server runtime; SPA mode exists but I did not confirm the current docs and found a discussion where users could not fully disable SSR. Adds Vite/Nitro baggage for features we do not use. |
| "Native React" (own history + `useSyncExternalStore`) | Not recommended | The spike's navigation shim shows it is easy to start (about 12 lines) but it would grow to cover 41 routes, 16 nested layouts, query-string state, scroll restoration, `<Link>` active state and code splitting. That is a router, owned by us. |

Not verified: I did not build a TanStack Router app here. The route shape (41 static pages, no dynamic segments, 16 layouts) suggests a mechanical file-route port, but the actual effort is untested. Metadata (`<title>` per route) has no direct equivalent and would need a small head helper.

## 9. Decision: Bun.build with a patched evaluation step

Chosen by the owner (2026-10-19): bundle and serve with Bun, keep vanilla-extract, patch its `eval` dependency. Tests stay on Jest for now. Package install: see the next subsection; pnpm was a default, not a requirement.

Validated in the spike (scratchpad, not the repo):

| Check | Result |
|---|---|
| One-pass `Bun.build` with an on-demand vanilla-extract plugin (no prepass), all 41 pages + layout, minified, split | 2.73, 2.94, 2.98 s; JS 5,429,699 bytes in 45 files (identical size to the prepass build), CSS 626,643 bytes |
| Bun dev server (`Bun.serve` + HTML import, `development.hmr`) rendering a real `ErrorState` component with real `.css.ts` styles | Rendered with vanilla-extract class names; computed style `display: flex`, `padding: 32px` |
| Editing a `.css.ts` file while the page is open | "Reloaded in 84ms"; padding updated to 48px with no page reload (a `window` marker survived) |
| Plugin size | about 25 lines (`ve-plugin.ts`, in the spike) |

### Package install: Bun vs pnpm on the real lockfile

Measured on this host with the real `web-app/package.json` and `pnpm-lock.yaml` (scripts skipped for pnpm; Bun never runs dependency scripts), warm package caches, one run each:

| Installer | Time | Entries in `node_modules` | Unique file inodes | Size |
|---|---|---|---|---|
| `pnpm install --frozen-lockfile` | 7.5 s (12.9 s first run) | 82,011 | 65,090 | 1.3 GB |
| `bun install --frozen-lockfile` (hoisted) | 1.9 s (28.2 s with a cold Bun cache) | 76,280 | 64,715 | 1.3 GB |
| `bun install --linker isolated --frozen-lockfile` | 2.1 s | 82,256 | 64,715 | 1.3 GB |

- **Versions:** Bun read `pnpm-lock.yaml` and produced a `bun.lock`; all 103 direct dependencies resolved to the same versions as pnpm.
- **Disk and inode pressure is the same.** Both use hardlinks into a global cache, so Bun does not fix or worsen the Btrfs metadata problem behind `docs/how-to/use-bun-in-web-app.md` (formerly `use-pnpm-in-web-app.md`). The repo's shared-`node_modules` approach (`scripts/shared-node-modules.sh`) is what addresses that; it is now keyed on `bun.lock`.
- **Supply chain:** Bun never runs dependency lifecycle scripts unless listed in `trustedDependencies` (the `--ignore-scripts` help text says so). That blocks the delivery route of the 2026 TanStack attack described by Snyk, though not the malicious code itself. Update after the install migration: `@swc/core` and `esbuild` have `postinstall` scripts but both load and transform correctly with scripts not run (platform binaries come from optional dependencies), and `@vanilla-extract/css` has no lifecycle script; the three are still listed in `trustedDependencies` as a like-for-like port of `onlyBuiltDependencies`.
- **Cost of switching:** 199 `pnpm` mentions across `Makefile`, `scripts/` and `.github/`; the `only-allow pnpm` preinstall; `pnpm.onlyBuiltDependencies`; CI caches keyed on `pnpm-lock.yaml`; `tools/ci-gates`, `tests/e2e` and `tools/scanner/frontend`, which have their own pnpm installs. `bun patch` exists for the `eval` fix.
- **Not tested (at the time of this study):** the `file:` plugin dependencies after install, Storybook and Jest under a Bun-installed tree, and an `isolated` vs `hoisted` difference in tool behaviour. Since the install migration: the `file:` plugins resolve, the full Jest suite passes (627 suites) and `next build` succeeds under the isolated linker, which was chosen. Storybook is still untested.

Not validated: the real app in the dev server (only one component), React Fast Refresh across component state, source-map quality, Storybook, per-route HTML generation, and the router.

Remaining work, in dependency order:

1. **Keyframes fix** (PR #1097). Prerequisite for any strict CSS parser.
2. **Navigation facade** (section 7), shipped under Next.
3. **Patch `eval`** with `pnpm patch` (or an override) using the `vm` + `createRequire` shim, and open an upstream issue/PR with vanilla-extract.
4. **Router** (section 8): React Router 7 library mode or TanStack Router behind the facade.
5. **Build entry**: an HTML entry or a small build script producing `web-app/out/`, wiring `define` for the five `NEXT_PUBLIC_*` variables, the vendored fonts (replace `next/font/local` with `@font-face` CSS), the FOUC theme script, and per-route titles. Keep Go's SPA fallback.
6. **Tooling**: Bun install in bootstrap/Brewfile/CI; replace `next lint` (ESLint config extends `next/core-web-vitals`); move Storybook off `@storybook/nextjs`; update `size-limit` globs, `Makefile`, and CI caches.
7. **Tests**: keep Jest (it does not depend on the bundler). Decide on Rstest/Vitest separately.

Risks added by this choice: owning a build plugin and a patched dependency (pin the vanilla-extract version and re-test on upgrade), and Bun-specific bugs in a young bundler (a `bun test` segfault was seen in section 2; `Bun.build` itself has not crashed in these spikes).

## 10. Reproducing and limits

- Spikes live in the session scratchpad (`.../scratchpad/bench/`), not in the repo, so they are not committed. The preload plugin, `ve-prepass.mjs` and `build.ts` can be recreated from the descriptions above.
- The manual-instance flow from the repo's `CLAUDE.md` was not needed: nothing was click-tested against a running `stapler-squad`.
- Web searches did not turn up documented Bun support for vanilla-extract; the incompatibility above comes from running it, not from docs.
- Archaeology of vanilla-extract's integration package used a shallow clone in the scratchpad (`archaeology-vanilla-extract-*`), read-only. The Bun probes ran only against the already-installed dependency copy in the spike directory.
