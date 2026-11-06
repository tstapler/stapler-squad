# Requirements: web-app-bun-build

**Date**: 2026-10-19
**Type**: migration
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement
`web-app/` is a static SPA built with Next.js 15.3.2 (`output: "export"`), with no SSR, API routes, middleware or server-data functions (verified; see `docs/explanation/bun-migration-feasibility.md` §1). Next.js still costs the owner a slow build (production 60.6–115.7 s; the `make` target builds in development mode at 54–88 s), a large dependency/tooling footprint (webpack, `@storybook/nextjs`, `eslint-config-next`, `next/*` router APIs in 65 production files (regex count, 2026-10-19) plus 43 test files that mock them), and a webpack-specific build config. The owner wants a faster, leaner build and dev loop on Bun.

## Baseline
- Build: `make web-build` → `NEXT_BUILD_MODE=development next build` (development mode, unminified: 21.7 MB JS) via `web-app/out`, copied to `server/web/dist` and embedded by Go (`server/web/embed.go`); Go serves `index.html` as SPA fallback (`server/middleware/static.go`).
- Measured on this host (isolated copy, one run each): Next production cold 115.7 s / warm 60.6 s, JS 6,349,052 B in 151 files (1,907,594 B gzip); development-mode cold 88.5 s / warm 54.0 s.
- Dev loop: `next dev` on `${PORT:-3001}`.
- Tests: Jest 30 + ts-jest, 620 files, 7,683 tests, 55.7 s.
- Package manager: pnpm 10.27.0 (a separate in-flight PR moves every project to Bun; this project builds on it).

## Users / Consumers
- The owner and agent-spawned worktree sessions (build/dev loop, `make install-service`).
- The Go binary (embeds `web-app/out`).
- CI (`build.yml`, `lint.yml`, Playwright e2e, Lighthouse/Axe UX analysis, `size-limit`, Storybook).
- Browser users on desktop and mobile (existing requirement: both form factors must keep working).

## Success Metrics
Compared to the baseline above, measured on the same host with the same method:
- Production-equivalent build ≤ 10 s wall (spike: ~2.8 s for bundling alone, excluding HTML/route/asset work).
- `make web-build` end-to-end (including the Go embed copy) at least 5× faster than the 54–88 s baseline.
- Total shipped JS no larger than the Next production figure (6.35 MB; spike 5.43 MB), measured on the minified build, and the deployed bundle minified. Sequencing (ADR-004 amendment): the cutover PR ships unminified, parity with today's deployed 21.7 MB bundle; minification follows as its own revertible PR (8b) after the first Soak window, so "deployed bundle minified" is met at PR 8b, while the 6.35 MB budget is enforced in CI on a minified build from PR 5a on.
- Dev server with working hot reload for `.css.ts` and React components (spike: `.css.ts` edit → 84 ms reload without full page reload).
- Zero functional regression: all 42 routes render and navigate, deep links and query-string state behave as before, existing Playwright e2e and the Jest suite pass with no new failures against the pre-migration results. The only behaviour differences allowed are the "Accepted scope additions" below.
- No `next` dependency left in `web-app/package.json`.

## Value and ICE score
*(Added by the triad review. Provisional, from one command run on 2026-10-19; Story 0.6, Task 0.6c replaces them with a dated, at-least-28-day run. Informational: it does not recommend any downscale, and the owner has chosen full scope.)*
- Build seconds are not the whole value. Usage, measured by `git log origin/main --since='28 days ago' --oneline | wc -l` (299 commits, about 75 per week), `gh run list --workflow build.yml --limit 100` (100 runs between 2026-10-17T20:17Z and 2026-10-19T08:46Z, about 2.7 per hour, a window too short to extrapolate) and `git worktree list | wc -l` (76 worktrees present). Each build run and each agent worktree session pays the web build, so the saving scales with those counts.
- Expected saving is stated in two cost units, never in human time: CI runner-minutes per week (runs per week x (Next build step minus Bun build step)) and agent build invocations per week x seconds saved (wall-clock only; token cost is unchanged apart from fewer polling turns, so no CU saving is claimed). The numbers are filled by Task 0.6c.
- **ICE (impact x confidence x ease, each 1-10): 6 x 7 x 3 = 126 of 1000.** Impact 6: no user-facing change, the gain is build and dev-loop speed and a smaller dependency footprint across many runs. Confidence 7: spike measured about 2.8 s bundling and an 84 ms `.css.ts` reload, end-to-end 10.8 s not yet measured. Ease 3: XL scope (about 25.8M CU, 18 PRs, a Soak). Provisional.

## Size
XL (> 15M CU; plan.md estimates about 25.8M CU with the verification multiplier, range 22.1–29.4M, plus contingency lines C1–C3 outside the base) — informational only per ESTIMATION.md. Full scope is the default and not a question for the owner.

**Wall-clock blockers**:
- Owner decision: router — DECIDED 2026-10-19: React Router 7 (ADR-001). Gate S4 still validates it technically before Epic 4.
- The pnpm → Bun package-manager PR (worktree session in flight) must merge first.
- CI-only verification (workflows cannot be run locally).
- Upstream fix to vanilla-extract's `eval` dependency (nice-to-have; the repo carries a local `bun patch` meanwhile).
- Soak window after cutover before deleting the Next.js build path.

## Constraints
- Do not disturb the live instance on :8543; manual testing uses an isolated instance (`CLAUDE.md` "Manual/interactive testing").
- vanilla-extract stays (336 `.css.ts` files); its `eval` dependency needs a patch because `Module._load` returns `undefined` under Bun (spike-verified: 336/336 files evaluate, output identical to Node).
- Jest stays for now; Bun is not the test runner (a full-suite `bun test` run segfaulted).
- Go side keeps SPA fallback; URLs and `trailingSlash` behaviour must stay compatible with existing deep links, Playwright specs and the mobile app.
- Supply-chain posture: pin versions; no new dependency without a recent-compromise check (TanStack Router had a May 2026 incident: `@tanstack/react-router` 1.169.5 and 1.169.8).
- Repo gates: conventional commits, `make ready` (lint, jscpd 0.14% threshold), e2e conventions, feature registry markers (`// +feature:`), no force push, PRs ready-for-review.
- Fonts are vendored woff2 (no network at build time); the theme-FOUC inline script depends on build-time vanilla-extract class names.

## Non-functional Requirements
- **Performance SLO**: build and dev-reload targets above; no regression in Lighthouse CI (warn < 70) or `size-limit` budgets (Total JS 5 MB / framework 650 KB / main 400 KB today; budgets to be re-expressed for the new output layout).
- **Scalability**: not applicable (static assets).
- **Security classification**: internal; auth flows (WebAuthn on `/login`) must keep working.
- **Data residency**: no special requirements.

## Scope
### In Scope
- Replace Next.js build/dev with a Bun-based build (`Bun.build`) and dev server (HMR), producing the same `web-app/out` contract for the Go embed.
- vanilla-extract Bun plugin + patched `eval` (`bun patch`).
- Navigation facade (`@/lib/nav`) and a client-side router replacing `next/navigation` and `next/link` (router chosen in planning).
- Replacements for `next/dynamic`, `next/font/local` (→ `@font-face`), `export const metadata`/`viewport` (per-route titles), `useReportWebVitals`, `process.env.NEXT_PUBLIC_*` defines, HTML entry generation, theme-FOUC script.
- ESLint without `eslint-config-next` (keep custom rules/boundaries), Storybook off `@storybook/nextjs`, `size-limit`, Makefile, Go embed path, CI workflows, Playwright config, docs and CLAUDE.md references.
- Measured before/after table.
- ADRs for the bundler, router and vanilla-extract patch decisions.

### Out of Scope
- Moving tests to Bun test, Vitest or Rstest.
- Changing the CSS framework (vanilla-extract stays).
- Server-side rendering or any Node runtime at serve time.
- UI redesign or behaviour changes beyond route/title parity and the "Accepted scope additions" below.
- The pnpm → Bun package manager switch itself (separate PR in flight; this project depends on it).

## Rabbit Holes
- Router replacement: 22 `useSearchParams` uses with ~25 distinct query keys, 66 `router.push/replace` calls, 16 nested layouts, scroll restoration, active-link state.
- Per-route HTML + `trailingSlash` + Go fallback + Playwright `baseURL`/deep-link expectations.
- Storybook without Next (`@storybook/nextjs` → a react-vite or react-webpack5 framework, which brings its own bundler).
- Dev-server fidelity vs production: React Fast Refresh across stateful components, source maps, Bun dev-server plugin config (`bunfig.toml [serve.static]`).
- Invalid CSS previously hidden by webpack/Rspack (the keyframes bug, fixed in #1097) — other strict-parser failures may surface.
- Bun bundler maturity: code splitting of `next/dynamic` call sites, CSS chunking order, tree-shaking of `process.env.NODE_ENV` branches.
- jscpd threshold and the feature-registry scanner may key on `next`-specific paths.
- `src/gen/**` protobuf output and `proto-gen` ordering in the Makefile.

## Alternatives Considered
- Stay on Next (rejected: build time and footprint).
- Vite 8 + Rolldown with the official vanilla-extract plugin: 5.75–6.25 s, no custom build code. Strong fallback.
- Rsbuild + `@vanilla-extract/webpack-plugin`: 6.2–6.6 s. Equal speed; also viable.
- Bun.build with a patched `eval`: ~2.8 s one-pass, HMR validated on one component. **Chosen by owner.**
- Router: React Router 7 library mode behind a facade (leading), TanStack Router (typed search params, but 2026 supply-chain incident), TanStack Start (rejected, no server), hand-rolled router (rejected), small libraries such as `type-router-react` (single-maintainer).
- Different CSS-in-TS library (Panda, StyleX, Pigment): rejected; no problem to solve.

## Feasibility Risks
- Bun-specific bugs in a young bundler (a `bun test` segfault was seen; `Bun.build` has not crashed in spikes).
- Owning a build plugin and a patched dependency; vanilla-extract upgrades need re-testing.
- Full-app behaviour under the Bun dev server is unvalidated (only one component was tested).
- CI changes cannot be run locally.
- The router decision is still open.

## Observability Requirements
- Build: emit and record build duration, output JS/CSS bytes, and chunk count in CI (existing `size-limit` and benchmark workflows are the home for this).
- Runtime: existing browser telemetry (`WebVitalsReporter`, OpenTelemetry web SDK) must keep working with the Next-internal web-vitals import replaced (`web-vitals` library or `PerformanceObserver`).
- No new oncall alerts; failures surface as CI failures.

## Risk Control
- Keep the Next build path working and selectable (e.g. `WEB_BUILD=next|bun` in the Makefile) until the Bun path passes e2e and a soak period on the owner's instance; then remove Next in a final cleanup PR.
- Rollback: revert the cutover commit (Makefile default flip); no data or API changes are involved.
- Cutover verification uses an isolated second instance (ports 62871/62872 block), never the live service.
- Soak default: at least 14 calendar days and 3 distinct dev sessions after PR 8, a second window of at least 7 days after PR 8b, and a written stop-loss decision if PR 9 has not merged 45 days after PR 8 (plan Story 8.4).
- Staged PRs in the order of `docs/explanation/bun-migration-feasibility.md` §9: facade → `eval` patch → router → Bun build entry → tooling → cleanup.

## Accepted scope additions
*(Each is a deliberate difference from today's behaviour or an addition to the original scope, with its bound. Defaults are applied so work can proceed; the owner signs off the list in plan Story 0.6. Declining one removes the named story or flips its flag; none is a cutover gate except where stated.)*
1. **Not-found view (D1)**: an unknown path renders a "Page not found" view inside the shell instead of today's sessions page; Go still answers 200 (Stories 1.2, 4.3c, 7.7b).
2. **Focus move on route change (D5)**: focus moves to the layout `main` after a pathname change; one boolean `FOCUS_ON_NAVIGATE`, not a cutover gate; the focused `main` shows no focus ring, matching Next (D8) (Stories 4.5, 7.2b).
3. **Polite route announcer**: a `role="status"` `aria-live="polite"` region replaces Next's route announcer (assertive, INFERRED until Task 0.5d reads the built Next output); not `role="alert"` (Story 4.5).
4. **`RoutePending` skeleton** while a lazy route chunk loads; the shell stays mounted (Stories 4.3d, 4.4).
5. **"A new version is available" banner** instead of an automatic reload when a stale-chunk error occurs while a field holds text (Story 2.7).
6. **Go 404 for missing assets under `/_next/`** instead of the HTML fallback (Story 1.6).
7. **ESLint flat-config conversion** off `eslint-config-next`, time-boxed and tolerance-bounded; does not gate PR 8 (Stories 6.1-6.3).
8. **Lighthouse config consolidation** to the Go-served config; warn-only thresholds unchanged (Story 6.5).
9. **Axe sweep extended** from 29 to all routes, with Next-also findings added to `KNOWN` (Story 7.8); WCAG 2.2 AA axe tags are included, report-only where Next also violates.
10. **Minification as its own PR (8b)** after the cutover (ADR-004); the 6.35 MB budget is enforced on a minified CI build from PR 5a.
11. **`web_vital.fid` series ends** (FID was removed upstream; INP continues) (Story 2.8).

## Open Questions
*(Marked against "Phase 2 Resolutions" below and the plan; RESOLVED means answered by research or a decision already taken, with the owner story that carries the evidence. Only the router is still open.)*
- **RESOLVED 2026-10-19 (owner: React Router 7; S4 evidence gate remains)**: Router: React Router 7 library mode vs TanStack Router. Chosen: React Router 7.18.4 behind the `src/lib/nav` facade (F1, ADR-001); the owner signs off in Story 0.6 after gate S4 reports and before Epic 4 starts (plan "Owner handoff and sign-off points").
- **RESOLVED**: Storybook: which Next-free framework. It already runs on `@storybook/react-webpack5`; `@storybook/nextjs` is an unused leftover; keep and delete the leftover (ADR-002, Story 6.6).
- **RESOLVED**: Is `next lint`'s rule set (`next/core-web-vitals`) worth reproducing? It reduces to react + react-hooks + 6 jsx-a11y warnings + `import/no-anonymous-default-export`; reproduce those, drop `@next/next/*` (Story 6.1).
- **RESOLVED**: HTML generation: one `index.html` with SPA routing and Go's SPA fallback; the emitted-HTML location is settled by gate S1 (layout A' or B, Stories 0.2, 5.5).
- **RESOLVED**: Where do per-route `<title>` and `viewport` live: titles in `RouteMeta` (Story 1.2), viewport and `lang` static in `index.html` (Story 5.3).
- **RESOLVED, one sign-off left**: Does anything depend on `_next/` asset paths or `trailingSlash` URLs? Asset paths stay under `/_next/static/` (ADR-003, `auth.go`/`static.go` coupling); specs and Lighthouse are inventoried (Stories 0.1, 7.11); the mobile app lives outside this repo, so the owner's D1 sign-off in Story 0.6 closes it.
- **RESOLVED**: How is the vanilla-extract `eval` patch carried and tested: `bun patch` of `eval@0.1.8`, `PatchCanary`, oracle and upstream tracking (ADR-008, Epic 3).
- **RESOLVED**: Does the Bun dev server need the Go backend proxied: no, cross-origin via `NEXT_PUBLIC_API_URL` and `STAPLER_SQUAD_EXTRA_ORIGINS`, as `next dev` does today (decision D6, Story 5.6).

## Phase 2 Resolutions (added after research)
Answered by research (see research/*.md): Storybook already runs on `@storybook/react-webpack5` (`@storybook/nextjs` is an unused leftover, 50 stories, no CI/Chromatic); `next/core-web-vitals` reduces to react + react-hooks + 6 jsx-a11y warnings + `import/no-anonymous-default-export`; HTML = one hand-written `index.html` with Go's SPA fallback; `next dev` has no backend proxy (cross-origin via `NEXT_PUBLIC_API_URL` + `STAPLER_SQUAD_EXTRA_ORIGINS`); `bun patch` of `eval` is the primary fix (upstream issue vanilla-extract#1664); asset paths must stay under `/_next/static/` (Go `auth.go:95-101`, `static.go:76`) unless both Go files change together.
Corrections to counts: 42 `page.tsx` files (new: `fanout`), 43 test files mock `next/navigation`/`next/link`, only 4 of 16 layouts do more than export a title (root + feature-flag guards on backlog, changelog, settings/backlog-sources).
*(unresolved after Phase 2 research — owner decisions, planned as flagged choices)*: router (RESOLVED 2026-10-19: React Router 7.18.4 pinned exactly); Storybook (RESOLVED 2026-10-19: drop `@storybook/nextjs`, keep react-webpack5); when to rename the `/_next/` asset prefix; minification timing (own PR 8b after cutover, ADR-004); `size-limit` budget shape (framework/main chunk globs have no Bun equivalent).
Base for this work: PR #1100 (pnpm → Bun package manager; open) and #1097 (keyframes fix; merged).
