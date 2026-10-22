# Pitfalls: web-app Next.js static export -> Bun.build + Bun dev server

Phase 2 research (Pitfalls). Date 2026-10-19. Labels: **VERIFIED** = file opened or command run (cited); **INFERRED** = reasoning from evidence; **UNVERIFIED** = secondary source only. Repo paths are relative to the worktree root. Spike Bun version is 1.4.3 (`docs/explanation/bun-migration-feasibility.md:36`).

## 1. Ranked risks (likelihood x impact)

| # | Risk | L | I | Mitigation |
|---|---|---|---|---|
| R1 | Go server keys on the `/_next/` prefix for **auth exemption** and **immutable caching**. A new asset dir (e.g. `/assets/`) gets 302'd to `/login` for unauthenticated users, so the login page cannot load its own JS/CSS. | High | Critical | Either emit assets under `out/_next/static/` (keep the prefix) or change `server/middleware/auth.go` exemptPrefixes and `static.go` cache rule in the **same PR** as the build switch, with a Go test. See 2.1. |
| R2 | Bun 1.4.x CSS bundler regression #45730: drops an earlier rule when two same-selector rules contain only custom properties and other rules sit between them. vanilla-extract `createTheme` emits custom-property-only rules; the spike used 1.4.3, the affected range. | Med | High | Diff built CSS against the Next production CSS (rule/custom-property count per theme class); pin Bun to a version where the diff is clean; keep a CSS-parity check in CI. See 3.1. |
| R3 | `bun patch` for `eval` silently not applied (#33520) or lost on a lockfile change. Symptom is a cryptic `setAdapter` TypeError (same as vanilla-extract#1664) or, worse, a partial build. | Med | High | Postinstall-independent assertion: a `scripts/verify-eval-patch` step (checks the patched marker in `node_modules/eval`) run in Makefile and CI before the build; pin vanilla-extract and `eval` exactly. See 3.2. |
| R4 | Router swap: loss of scroll restoration, route-change a11y announcer, `useSearchParams` semantics (22 uses, ~25 keys), trailing-slash URLs asserted in e2e. | High | High | Facade first (under Next); parity test matrix per query key; add an aria-live announcer; keep `trailingSlash` normalisation. See 4. |
| R5 | Supply chain: TanStack Router May 2026 compromise (react-router 1.169.5, 1.169.8) and Bun-specific install behaviour (lockfile rewrite, linker). | Med (if TanStack chosen) | Critical | Prefer React Router 7 library mode; if TanStack, require >=1.169.9, exact pin, `minimumReleaseAge`, no lifecycle scripts. See 5. |
| R6 | theme-FOUC inline script and `<html className>` need build-time vanilla-extract class names and the four `next/font` CSS variable classes. A pure one-pass `Bun.build` has no HTML templating step that knows them. | High | Med-High | Compute theme map at runtime in a tiny first-script module (or post-build HTML step reading the build output); replace `variable:` classes with plain `:root` custom properties in `@font-face` CSS. See 2.3. |
| R7 | `NEXT_PUBLIC_*` and `process.env.NODE_ENV` inlining: Bun `define` limits (#16025 index access, #19508 `env: "disable"` still inlines NODE_ENV). | Med | Med | Static-grep gate: only `process.env.NAME` dot access allowed; explicit `define` map; assert no `process.env` string remains in output. See 2.4. |
| R8 | Dev-server fidelity: HMR import-cycle bug (#40248), only one component validated, Bun.build native memory leak history (#34053, closed) for long-lived dev. | Med | Med | Full-app dev smoke (all 41 routes load, no console errors) as a gate; keep `WEB_BUILD=next` dev path until soak ends. See 3.3. |
| R9 | Storybook, ESLint and Jest assumptions: `.eslintrc.json` extends `next/core-web-vitals`; `package.json` still lists `@storybook/nextjs` (config already uses react-webpack5); jest mocks `next/*` in ~40 files. | High | Low-Med | See 6. Mechanical but wide. |
| R10 | CI caches/artifacts keyed to `.next/cache`, `pnpm-lock.yaml`; `size-limit` globs `out/_next/static/chunks/**`; Lighthouse uses `npx serve@14 out`. | High | Low (loud failures) | Enumerated in 2.2; update in the tooling PR. |
| R11 | Bun runtime crashes (JSC GC segfaults #41857, #34476; `bun test` segfault seen in spike). | Low for `Bun.build`, Med for long dev server | Med | Don't use `bun test`; run build as a short-lived process; retry wrapper already exists (`scripts/retry-with-backoff.sh`). |
| R12 | Invalid CSS hidden by webpack (keyframes bug, #1097) - other strict-parser failures may remain. | Med | Low-Med | Run Bun CSS build with `throw:true` over whole app; fix each source file; do not suppress. |
| R13 | jscpd (0.14% threshold) rises from new router/plugin/test boilerplate. | Med | Low | Put shared mocks in a helper; run `pnpm run lint:duplicates` per PR. See 2.2. |

## 2. Repo-specific findings (all VERIFIED by grep/read unless noted)

### 2.1 Go server and `/_next/` (R1)
- `server/middleware/static.go:76-78`: `strings.HasPrefix(cleanPath, "/_next/")` is the only path that gets `Cache-Control: public, max-age=31536000, immutable`; everything else gets nothing, and `.html` gets `no-cache`. A Bun output dir named differently loses long-lived caching (perf regression, not a failure).
- `server/middleware/auth.go:94-100`: `exemptPrefixes` = `/auth/`, `/login`, `/health`, `/_next/`, `/favicon`. Assets outside `/_next/` require a session, so the login page (WebAuthn) breaks for logged-out users. This is the highest-impact, easiest-to-miss coupling. Also note `/login` is a prefix: `/login/` assets are fine.
- `static.go:40-60`: directory requests serve `dir/index.html`, else fall back to root `index.html`. A single-`index.html` SPA works via the fallback, but any unknown path returns 200 + index (no real 404); see 2.5.
- Hashed bundle filenames from `Bun.build` are fine for `immutable`; the HTML must stay `no-cache` (it is).

### 2.2 Everything else keyed on Next paths
| Location | Coupling | Action |
|---|---|---|
| `Makefile:217-227` | target `web-app/out`, depends on `web-app/next.config.ts`, guards `web-app/node_modules/next`, `NEXT_BUILD_MODE`, `NEXTJS_SHARED_CACHE_DIR`, `pnpm run build` | Re-key prerequisite file and guard; drop webpack cache dir |
| `Makefile:553`, `scripts/dev-stack/launch.ts:11,247,549`, `tests/e2e/global-setup.dev-mode.ts:10,45` | `next dev` frontend child and readiness log text | Replace with Bun dev server; readiness check ("any HTTP response") is compatible |
| `Makefile:1021` | `npx next lint` | ESLint CLI |
| `web-app/package.json:15-18, 109-125` | scripts + `size-limit` config (also duplicated in `web-app/.size-limit.json`: two copies) | Fix both; delete one |
| `web-app/playwright.config.ts:82` | `npx next dev --port` webServer | Bun dev command |
| `web-app/lighthouserc.json` | `npx serve@14 out -l 3001`, URL `/` | Works if `out/index.html` exists. `serve` does clean-URL/trailing-slash handling differently from Go - not a faithful test of deep links |
| `.github/actions/prepare/action.yml:62,77`, `.github/workflows/build.yml:180`, `benchmark.yml:285,345,531,609,620` | cache path `web-app/.next/cache`; `cp -r web-app/out` | Replace cache with Bun install cache + build output hash |
| `.github/workflows/lint.yml:276-379` | `npx next lint [--file ...]`; comments rely on "`next lint` exits 1 on ANY warning" | ESLint with `--max-warnings 0` to preserve gate semantics |
| `.github/workflows/backlog-scaffolding-guard.yml:53`, `session/git/scaffolding.go:35`, `scaffolding_test.go:18` | ignore `web-app/.next/` | Keep until Next path removed |
| `tools/scanner/frontend/src/component-scanner.ts:13` | excludes `.next` dir; scans `web-app/src` for `// +feature:` markers (275 files carry one) | Scanner keys on markers in `src/`, not `page.tsx` or `.next`: low risk. If route files are renamed/moved, markers move with them; run `make registry-generate` and commit diffs |
| `.eslintrc.json` (`boundaries/elements` `src/app/**`) | layer rule keyed on `src/app/` | If routes leave `src/app/`, update the element patterns or boundaries stop applying silently |
| `tests/e2e/*` | `#__next-route-announcer__` excluded in `accessibility.spec.ts:230`, `EscapeAnalyticsGlobalViewPage.ts:39-43`, `terminal-resync-banner.spec.ts:171,240`; trailing-slash URL assertions in `unfinished-work.spec.ts:155`, `insights-findings-panel.spec.ts:28`; dev-mode build assumptions `insights-findings-panel.spec.ts:88-89`; `jules-status-badge.spec.ts:47` comments on static-export behaviour | Re-triage each; announcer exclusion becomes dead but harmless; add the new announcer to a11y expectations |
| `web-app/.jscpd.json` | `minLines 20 / minTokens 200 / threshold 0.14`, ignores `**/fonts/**` | See R13; new generated build code in `src` would count, keep it in `web-app/scripts/` or outside `src` |
| `web-app/.size-limit.json` | three budgets on `out/_next/static/chunks/framework-*.js` and `main-*.js`; Bun has no such named chunks | Re-express as total JS + largest chunk + CSS; `framework-*`/`main-*` globs would match **zero files**; confirm `size-limit` fails (not passes) on zero matches |
| `.gitignore:47,50,91` | `/web-app/.next`, `/web-app/out/` | Add any new output dir |
| `.claude/commands/perf/make-it-faster.md:540,626,638` | `.next/static/chunks` | Doc only |
| `package.json` `preinstall: npx only-allow pnpm`, `pnpm.onlyBuiltDependencies` (`@swc/core`, `esbuild`, `@vanilla-extract/css`) | pnpm-only | Handled by the package-manager PR; the equivalent Bun key is `trustedDependencies` (see 5.2) |
| `docs/` | many references (not grepped exhaustively) | Include a docs grep in the cleanup PR |

### 2.3 theme-FOUC script, fonts, per-route metadata (R6)
- `web-app/src/app/layout.tsx:12-60`: the inline script embeds `JSON.stringify` of six vanilla-extract theme class names plus `cleanTheme`, `jetbrainsMono.variable` etc. as `<html className>`; `export const metadata`/`viewport` provide title, description, `viewport-fit=cover` (needed for iOS safe-area; losing it is a mobile regression). 20 files export `metadata`/`generateMetadata`/`viewport` (grep).
- `web-app/src/app/fonts.ts:1-30`: `next/font/local` with `variable:` custom property names (`--font-jetbrains-mono`, ...). Replace with `@font-face` + `:root{--font-...}`; verify the same custom property names are consumed by `.css.ts` files (not checked here). `next/font` also adds `size-adjust` fallback fonts and preload `<link>`s; dropping them can cause layout shift (INFERRED: standard `next/font` behaviour).
- Because FOUC prevention must run before first paint, an external `<script defer>` or a module script is too late; it has to be inline or a blocking classic script. If theme class names are only known after the JS bundle is built, generate the HTML in a post-build step that imports the built theme module, or hardcode names via a second small vanilla-extract entry.

### 2.4 Env inlining (R7)
- grep: `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_RECONNECT_V`(2), `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING`, `NEXT_PUBLIC_OTEL_ENABLED`, `NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` appear in 50+ places across src, tests and scripts; `process.env.NODE_ENV` in ~8. No `process.env[...]` dynamic access found in non-test src (grep returned none), which is the pattern Bun `define` handles poorly (#16025, open).
- Bun docs say `define` values must be JSON strings/identifiers/property paths and that `env` prefix matching supports only a trailing `*` (https://bun.com/docs/bundler). `NEXT_PUBLIC_*` inlining is therefore **opt-in** (`env: "NEXT_PUBLIC_*"` or explicit `define`); forgetting it leaves a runtime `process is not defined` (already seen in the spike, feasibility doc line 56).
- #19508 (open): `Bun.build({env:"disable"})` still inlines `process.env.NODE_ENV`. Do not rely on `env:"disable"` to leave NODE_ENV runtime-switchable; always set it explicitly (`"production"` for release, `"development"` for the dev-mode build `make` uses today).
- Next replaces `NEXT_PUBLIC_*` from `.env*` files automatically; `web-app/.env.local.example` exists. Bun's bundler does not load `.env` into `define` by default for `Bun.build` JS API unless `env` is set (docs say only the CLI default is `disable`; INFERRED for the API).

### 2.5 Implicit Next behaviours being dropped (item f)
| Behaviour | Consequence | Mitigation |
|---|---|---|
| Per-route HTML (`trailingSlash:true` gives `/sessions/index.html`) | One `index.html` + Go fallback works, but unknown URLs return 200, not 404; Lighthouse/e2e specs asserting trailing slash change | Router `NotFound` route; normalise trailing slash in the router; keep e2e assertions or relax them deliberately |
| Prefetch of `<Link>` routes and chunks | Cold navigation latency for lazily split routes | `React.lazy` + `<link rel=modulepreload>` or hover prefetch; measure |
| Scroll restoration | Next static export restores on back/forward; React Router library mode requires the data router for `<ScrollRestoration>` (INFERRED from RR docs, not re-verified today) | Choose `createBrowserRouter` form, or implement history scroll save |
| Route announcer (`#__next-route-announcer__`, aria-live) | Screen-reader route change announcements vanish silently; existing a11y e2e only *excludes* it | Add own announcer; add e2e test |
| Image optimisation | None used (`next/image`, `next/script`, `next/head` have zero hits) | n/a |
| Font optimisation | See 2.3 | |
| Tree-shaking by `process.env.NODE_ENV` | Dev-only React code (and the `reactStrictMode` double-render) differ; bundle larger if not defined | Define NODE_ENV; set `reactStrictMode` behaviour explicitly via `<StrictMode>` (it is on today: `next.config.ts` `reactStrictMode: true`) - removing it changes effect double-invocation in dev only |
| `optimizePackageImports: ['@/components','@/lib']` barrel handling | Bun tree-shakes ESM barrels but side-effect-ful modules (`.css.ts` imports) can stay in | Check `sideEffects` and measure bundle |
| webpack `extensionAlias` for `.js` -> `.ts` (protobuf output) | `src/gen/**` imports with `.js` suffix must resolve under Bun | Bun resolves `.js`->`.ts` natively (INFERRED, spike bundled it); keep in CI build as proof |
| Source maps: `productionBrowserSourceMaps: true` | Maps shipped today | Keep `sourcemap:"linked"`; note #37184 (open): `BuildArtifact.sourcemap` mis-points at a CSS asset in single-entry builds |
| `removeConsole: false` in dev builds / unminified `make` build | New build needs equivalent dev/prod switch | `WEB_BUILD_MODE` env mapped to `minify` |

## 3. Bun bundler/dev-server pitfalls (oven-sh/bun issues, queried with `gh api search/issues` on 2026-10-19)

### 3.1 CSS
- **#45730 (open, VERIFIED text)**: 1.4.0-1.4.4-canary regression, 1.3.11 fine. First `.tag{--a:1px}` rule dropped when a later non-adjacent rule has the same selector and only custom properties. https://github.com/oven-sh/bun/issues/45730. Relevance to vanilla-extract: INFERRED (theme contract output uses custom properties; not tested). Run a before/after custom-property census on the built CSS.
- Docs: "CSS is bundled into a single `.css` file"; `--css-chunking` only for multiple entrypoints (https://bun.com/docs/bundler). Today's Next config forces CSS consolidation for preload warnings, so a single file is consistent, but it means no per-route CSS splitting (CSS size 626,365 B in spike vs 648,758 B Next; fine).
- CSS order across dynamic imports: esbuild's own release notes state compile-time CSS order can differ from runtime JS evaluation order for dynamic imports (https://github.com/evanw/esbuild/releases/tag/v0.12.0), and Bun's bundler follows esbuild's model (INFERRED). With vanilla-extract, order matters only for same-specificity overrides across components; mitigate by a visual regression check (Playwright screenshot of several routes, before/after).
- #20508 (closed 2026-07-24): dynamic imports of JS that reference CSS became invalid CSS imports. Fixed, but confirms this area was buggy; keep the `next/dynamic` call sites (4) under test.
- #24770 (open): `@supports` handling; #19663 (open): `composes` subpath imports. Neither obviously used; grep before relying.
- Plugin `onLoad` for CSS: the docs do not say whether virtual CSS from an `onLoad` plugin (what the vanilla-extract plugin returns) is supported in all code paths. The spike worked for the happy path. UNVERIFIED for the dev-server path with `#19951` (open): "fullstack dev server (bake) + onResolve plugin breaks" - directly relevant to a `onResolve`-based virtual `.vanilla.css` module in the dev server. Test `.css.ts` resolution through `bunfig.toml [serve.static]` explicitly, including a cold start and an edit of a *dependency* of a `.css.ts` file (the plugin must invalidate).

### 3.2 `bun patch` / `eval`
- vanilla-extract#1664 (open): identical failure ("`require('@vanilla-extract/css/adapter')` is `undefined`") under `bun --bun` in the Next plugin, no maintainer fix; so upstream is not imminent. https://github.com/vanilla-extract-css/vanilla-extract/issues/1664
- oven-sh/bun#33520 (open): `patchedDependencies` silently not applied on CI with a warm global cache. https://github.com/oven-sh/bun/issues/33520 -> verify-step mitigation (R3).
- The `eval` package: patched dep means every vanilla-extract upgrade can change the import chain; pin exact versions and add a CI test that compiles 3 representative `.css.ts` files.
- Alternative designs that avoid patching: run the vanilla-extract prepass under Node (`node` child process, ~2 s) and feed generated CSS to `Bun.build`; or use Vite 8/Rsbuild (the owner chose Bun; recorded as fallback in requirements).
- No GitHub issue exists in vanilla-extract for generic non-Vite/non-esbuild bundler support beyond the official webpack/rollup/esbuild/vite/next plugins (searched `bun`, `rspack`, `rolldown`; results above). Treat the Bun plugin as unsupported-by-upstream code owned by this repo.

### 3.3 Dev server and HMR
- #40248 (open, 1.4.0): HMR hands `null` to a module reading an import cycle at top level during evaluation; works with `hmr:false`. This repo is large with barrels (`@/components`, `@/lib`), so cycles are likely. Provide `hmr:false` escape hatch; run a cycle check (e.g. `madge`) before cutover.
- #19129 (closed 2026-07-25): `React.lazy` under the fullstack dev server misbehaved; fixed, but re-test `next/dynamic` replacements in dev specifically.
- #44463 (open): TC39 decorators fail in HMR runtime; grep showed no need in this app (INFERRED, not grepped for decorators).
- #34053 (closed): `Bun.build()` leaked native memory per call, hurting watch/dev processes; if the design wraps `Bun.build` in a custom watch loop, re-check on the pinned version.
- #45238 (open): `Bun.serve` with `development:false` never reports why an HTML route failed to bundle - build errors may be opaque in production-mode serve; always run `Bun.build` directly in CI with `throw:true`.
- #37576 (closed 2026-09-26): splitting + identical shared chunk content -> "Multiple files share the same output path". Fixed; an example of splitting edge cases. Run the real 41-route split build in CI.
- Code splitting requires `format:"esm"` and, with hashed filenames, a correct `publicPath` (https://haruska.com/til/bun-code-splitting-with-rails-propshaft, https://bun.com/docs/bundler); the Go server serves from `/`, so `publicPath:"/"` must be explicit or chunks 404 on nested routes (`/settings/jules/` resolves relative `./chunk.js` against the wrong directory).

### 3.4 Crash reports
- JSC heap corruption/SIGSEGV in GC parallel marking: #41857 (open, 2026-10-16), #34476 (Linux arm64, long-running server). The spike's `bun test` segfault is consistent. Short-lived `Bun.build` is the lowest-exposure use; a long-lived dev server is the highest. Mitigation: supervisor auto-restart in `scripts/dev-stack/launch.ts`; fall back to `WEB_BUILD=next` dev.
- #17723 (open): moving Node->Bun spiked CPU/memory in a container - relevant only if CI containers are tight.

## 4. Router replacement pitfalls (item c)
- **Search params**: Next's `useSearchParams` is read-only `ReadonlyURLSearchParams`; mutation is via `router.replace(url)` (66 `push/replace` calls). React Router's `useSearchParams` returns `[params, setParams]` and *its setter creates a new history entry unless `{replace:true}`*; a mechanical port changes back-button behaviour. The facade must preserve `push` vs `replace` per call site.
- **Basepath / trailing slash**: with `trailingSlash:true` all in-app URLs have a trailing slash today. RR7 does not add one; links built as `/sessions` vs `/sessions/` mismatch route matching only if `caseSensitive`/strict is set, but external deep links (mobile app, notifications, Slack) may use either. Keep a normalising redirect.
- **`redirect()`** from `next/navigation` (2 uses) throws during render; the RR equivalent is `<Navigate replace>` or loader `redirect`; library mode (no data router) has no loaders.
- **16 nested layouts**: file-system nesting does not exist in RR/TanStack unless a plugin is used. TanStack's file-based plugin adds a build dependency (`@tanstack/router-plugin`, in the compromised set at 1.167.38/1.167.41); with Bun you would have to hand-write the route tree or run its CLI generator.
- **Query-string state across navigation**: `useOmnibarQueryParamLaunch.ts` and `sessionDeepLink.ts` in `src/app/` read URL params at mount; Next static export reads them after hydration, a SPA router may have them on first render - ordering differences can double-fire effects.
- **Tests**: 40 test files mock `next/navigation`; the facade should provide a single test helper (also helps jscpd).
- **Typed routes** (TanStack's selling point) need the generated route tree; not available in RR library mode (feasibility doc line 128 itself says "could not verify").

## 5. Supply chain and Bun install (items a, e)

### 5.1 TanStack May 2026 compromise (VERIFIED from secondary sources; primary postmortem not opened)
- Affected: all `@tanstack/*` listed versions from 1.169.5 up to (excluding) 1.169.9, notably `@tanstack/react-router` and `@tanstack/router-core` **1.169.5 and 1.169.8**; 84 malicious versions across 42 packages, published 2026-05-11 ~19:20-19:26 UTC; also `@tanstack/react-start` 1.167.68/.71 and `@tanstack/router-plugin` 1.167.38/.41. Fixed: 1.169.9. Sources: https://advisories.gitlab.com/npm/@tanstack/react-router/CVE-2026-45321/ , https://semgrep.dev/blog/2026/tanstack-router-packages-hit-by-coordinated-supply-chain-attack , https://security.snyk.io/vuln/SNYK-JS-TANSTACKREACTROUTER-16640208
- Release path: `pull_request_target` "pwn request" -> Actions cache poisoning -> OIDC token read from runner memory -> publish via the project's legitimate trusted-publisher workflow. The malicious versions carried **valid SLSA provenance**, so `npm audit signatures`/provenance checks do not detect them (StepSecurity: https://www.stepsecurity.io/blog/mini-shai-hulud-is-back-a-self-spreading-supply-chain-attack-hits-the-npm-ecosystem).
- Delivery at install time (StepSecurity): each package gained an `optionalDependencies` entry `@tanstack/setup` -> `github:tanstack/router#79ac49ee...` (attacker-fork commit) whose `prepare` script (`bun run tanstack_runner.js && exit 1`) ran when resolved from a git URL; the `exit 1` made the optional dep fail quietly. Worm payload: credential theft, self-propagation, persistence via user systemd units/LaunchAgents, dead-man's switch (Semgrep). Indicators: `router_init.js`, `@tanstack/setup` in `node_modules`/lockfile.
- Whether Bun's default would have blocked it: Bun does not run dependency lifecycle scripts unless the package is in `trustedDependencies` (feasibility doc line 159; Bun docs). Whether that includes `prepare` of a git dependency is **UNVERIFIED**; the Bun docs page I fetched is silent. Treat as unproven; test with a harmless local git dep fixture before claiming protection.
- Because the attack hit pinned-by-range consumers on a fresh install on 2026-05-11, **exact pins + committed lockfile + frozen installs** are what protect later installs; `minimumReleaseAge` would have blocked the same-day publish.

### 5.2 Hardening checklist for the Bun install
1. `bunfig.toml` `[install] minimumReleaseAge = 259200` (seconds; default null; `minimumReleaseAgeExcludes` for exceptions) - https://bun.com/docs/runtime/bunfig (VERIFIED text).
2. `bun install --frozen-lockfile` in CI; note #44713 (open): `bun add` can write a `bun.lock` that the next `bun install` rewrites, making `--frozen-lockfile` fail - run `bun install` once after `bun add` before committing.
3. `trustedDependencies` minimal: the pnpm list was `@swc/core`, `esbuild`, `@vanilla-extract/css` (`web-app/package.json:7-11`). Whether each still needs scripts under Bun was not tested (feasibility doc line 159). If `@swc/core` is only used by Jest/Storybook, a missing postinstall breaks those silently later.
4. Linker: `hoisted` is Bun's default for existing projects (docs); the spike found hoisted and isolated both worked (feasibility doc lines 153-155) but Storybook/Jest under each were not tested. `isolated` surfaces phantom-dependency imports (a real hazard in a 100+ dep app with ESLint plugins loaded via `file:`).
5. `file:` plugin dependencies (`eslint-plugin-analytics`, `eslint-plugin-rpc-lifecycle`) after Bun install: not tested.
6. Ensure no CI workflow uses `pull_request_target` with checkout of PR code, and pin `oven-sh/setup-bun` (and all actions) to commit SHAs (`gha-pin-actions` skill exists). `rg setup-bun .github` returned nothing, so Bun is not yet in CI; the package-manager PR owns this.
7. Pin Bun itself: `bun-version` in setup-bun and a `.bun-version`/`packageManager`-style field; Bun 1.3.11 -> 1.4.0 changed CSS output (#45730), so floating "latest" is a risk.
8. Shared node_modules store (`scripts/shared-node-modules.sh`, with `.test.sh` and a Makefile test) is keyed on `pnpm-lock.yaml` and `.modules.yaml`; it must be re-keyed on `bun.lock` and `node_modules/.bin` presence. Also `bun.lock` (text, JSONC) merge conflicts and `patchedDependencies` entries must go through the same shared-store check (R3).
9. Prefer React Router 7 for this project: it removes the one dependency with a known 2026 compromise from the plan (decision for the owner; RR itself must also be exact-pinned and checked against advisories at pin time).

## 6. Tooling replacement pitfalls (item d)
- **Storybook**: `web-app/.storybook/main.ts` already uses `@storybook/react-webpack5` with a comment that `@storybook/nextjs` is incompatible with Next 15 (VERIFIED, lines 1-12), yet `package.json:136` still lists `@storybook/nextjs`. Removal may be nearly free; verify nothing imports from it (`grep -rn storybook/nextjs`). Storybook stays on webpack, so vanilla-extract in Storybook uses `css-ts-mock-loader.js` (VERIFIED file exists) - the Bun plugin is not exercised there; no new risk, but also no coverage of the Bun plugin by Storybook.
- **ESLint**: `.eslintrc.json` is `extends: ["next/core-web-vitals"]` plus custom plugins. `next/core-web-vitals` brings `react-hooks/rules-of-hooks`, `react-hooks/exhaustive-deps`, `jsx-a11y` subset, `@next/next/*` rules (including `no-html-link-for-pages`, `no-img-element`). Replacing with `eslint-plugin-react-hooks` + `eslint-plugin-jsx-a11y` + `eslint-plugin-react` keeps the useful rules; the `@next/next/*` rules become moot. Before deleting, list which `next/*` rules actually fire today (answers requirements Open Question 3). Also `next lint` is deprecated upstream in Next 15.5+/16 (UNVERIFIED here), so the swap is overdue anyway. `lint.yml` file-scoped logic must be reworked to `eslint --max-warnings 0 <files>`.
- **next/font**: see 2.3.
- **`useReportWebVitals`**: `next/dist/client/web-vitals` is an internal path (feasibility doc line 29) - replace with the `web-vitals` package; metric names/attribution may differ (INP/FID), so the OTel dashboard series may shift. Confirm consumers of `WebVitalsReporter` output before swapping.
- **Jest**: ts-jest + `moduleNameMapper` may reference `next/jest` or `.next`; `web-app/jest.config.js` was not opened. Check it does not call `next/jest` (INFERRED risk: `next/jest` also handles CSS/image mocks and `.css.ts` transforms; if used, replacement needs explicit mappers).
- **size-limit**: see 2.2.

## 7. What to design against (summary)
1. Decide asset URL layout first (R1) and make it a tested contract: Go middleware test for auth exemption + cache header on the new prefix.
2. Add a build verification script run by `make web-build` and CI: assert (a) eval patch present, (b) no `process.env` or `next/` strings in output, (c) `out/index.html` exists and references existing hashed files, (d) CSS custom-property census equals Next's, (e) JS bytes <= 6.35 MB.
3. Keep `WEB_BUILD=next|bun` until the Bun path passes the full Playwright suite on an isolated instance (ports 62871/62872) and a soak; the Next path also provides the CSS/DOM baseline for parity diffs.
4. Pin: Bun, vanilla-extract, `eval`, router (exact), `minimumReleaseAge` on.
5. Facade-first router migration preserving push vs replace semantics, with a11y announcer and scroll restoration tests.

## 8. Gaps (not verified)
- TanStack's own postmortem (tanstack.com) and GHSA were not opened; delivery details come from StepSecurity/Semgrep.
- Whether Bun skips `prepare` for git dependencies: unverified.
- Whether #45730 affects vanilla-extract output: not tested.
- Bun dev server with `.css.ts` plugin across the whole app, `[serve.static]` plugin config: not tested (docs page did not cover it).
- `jest.config.js`, `.env` loading, `web-app/src/lib/**` env readers' exact semantics, and docs/ references were not exhaustively read.
- React Router scroll-restoration constraint is from memory of RR docs, not re-fetched.
