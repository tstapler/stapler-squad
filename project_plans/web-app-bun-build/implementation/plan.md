# Implementation Plan: web-app-bun-build

**Feature**: Replace Next.js 15.3.2 (static export) in `web-app/` with `Bun.build` plus the Bun dev server, a client router (React Router 7.18.4 behind a `src/lib/nav` facade) and Bun-based tooling, producing the same `web-app/out/` contract that the Go binary embeds. Staged, dual-build, each PR independently shippable and revertible.
**Date**: 2026-10-19
**Status**: Ready for implementation (Epic 0 spike gates run first and can redirect the plan; see Spike Gates). Triad review (product, UX, engineering): READY TO BUILD, gaps folded; see [triad-review.md](triad-review.md) and the Repair log. External wall-clock blockers (PR #1100 unmerged; Bun #45730/#33520 and vanilla-extract#1664 open upstream) are named in the plan, not plan defects
**ADRs**: [ADR-001 router](../decisions/ADR-001-router-react-router-7-behind-nav-facade.md) · [ADR-002 Storybook](../decisions/ADR-002-storybook-keep-on-react-webpack5.md) · [ADR-003 asset prefix](../decisions/ADR-003-keep-next-static-asset-prefix-until-cleanup.md) · [ADR-004 minification as its own PR 8b, explicit `NODE_ENV`](../decisions/ADR-004-ship-minified-at-cutover.md) · [ADR-005 budgets](../decisions/ADR-005-size-limit-budget-shape.md) · [ADR-006 dual-build seam](../decisions/ADR-006-dual-build-seam-web-build-next-bun.md) · [ADR-007 bundler and Vite 8 fallback gates](../decisions/ADR-007-bundler-bun-build-with-vite-8-fallback-gates.md) · [ADR-008 vanilla-extract `eval` patch](../decisions/ADR-008-vanilla-extract-eval-bun-patch.md)

**ADR location and promotion**: the eight ADRs live in `project_plans/web-app-bun-build/decisions/` and are not copied into `docs/adr/` by default. `docs/adr/` already holds `ADR-001-gemini-exit-code-contract.md`, `ADR-002-workspace-host-registry-gossip.md` and two files numbered ADR-023, so a copy would collide. If the owner wants them promoted, Story 9.3a renumbers each to the next free number (`ls docs/adr | sort | tail`) and records the mapping (project ADR-00N to `docs/adr/ADR-0MM`) in each file's header; ADR amendments before then (Story 6.4's ADR-005 line) edit the project copy.

**Inputs**: `project_plans/web-app-bun-build/requirements.md` (Complexity 4), `research/{stack,features,architecture,pitfalls,ux,build-vs-buy}.md`, `docs/explanation/bun-migration-feasibility.md`. Evidence labels in those files (VERIFIED / INFERRED / UNVERIFIED) carry over; anything marked UNVERIFIED there is either closed by a spike gate or listed under Unresolved Questions.

**Scope**: full. 10 epics (phases 0-9), delivered as 18 independently revertible PRs: 0, 1, 2c, 2d, 2a, 2b, 2e, 3, 4, 5a, 5b, 6a, 6b, 7a, 7b, 8, 8b, 9 (PR 4, the router, precedes PR 5, the build, as in requirements.md and feasibility section 9; the splits and the Epic 7 ordering are explained below; the optional Story 9.4 is a further PR outside the 18). Story IDs are stable across the repairs (new stories and tasks are appended; see the Repair log at the end).

---

## Plan-level decisions and ordering rationale

1. **Spike gates first (Epic 0).** Every UNVERIFIED item in the research that can invalidate the Bun choice is a gate with pass/fail numbers. Nothing after Epic 0 starts until the gate record (Story 0.6) exists. Epics 1 and 7 (bundler-independent, Next-only work) may start in parallel with Epic 0 because they cannot be invalidated by it.
2. **PR order: router implementation (PR 4) lands before the Bun build (PR 5a)**, as in requirements.md and feasibility section 9. The Bun build can only be exercised end to end once an entry with a router exists, and the router implementation is dead code until a bundler selects `impl.router.tsx`, so landing it first is safe, jest-testable with `MemoryRouter`, and lets PR 5 be the first PR where the real app runs in a browser under Bun. Building first would have forced PR 5 to ship an unexercisable build.
3. **Epic 7 is split in two and the order is exact.** **PR 7a ("Next baselines")** contains the Next-side halves of Stories 7.1-7.9 and 7.11-7.13, plus the whole of Story 7.14 (the route smoke spec, formerly Task 7.10d, moved so that Story 7.10 lies wholly in PR 7b): fixtures captured from the Next build, assertions that hold on Next, the triage of Next-quirk specs and `parity-diff` self-test. It depends only on PR 1 (it needs `RouteMeta` for the 42-path list) and lands **before PR 2a**, while Next is the only build. **PR 7b ("Bun assertions")** contains every assertion that can only pass on the Bun build (D5 focus move, `role=status` announcer, not-found view, boot-script and `<html lang>` checks, scroll policy, unauthenticated `/login` asset run, the `e2e-bun` job and its expected-diff list: Stories 7.2b, 7.5b, 7.7b, 7.10, 7.12, 7.13b) and lands **after PR 5a** (it needs the Makefile `WEB_BUILD` seam and a Bun `out/`), before PR 8. Bun-only assertions are gated on one helper, `tests/e2e/helpers/web-build.ts` (`webBuildMode()`, a one-line wrapper over `parseWebBuildMode(env)`). The parser has a single owner, Story 5.7 (PR 5a), in `scripts/dev-stack/web-build-mode.ts`; `launch.ts`, `test-server.ts` and the helper only import it. Story 9.3b deletes both. Nothing in 7b can be validated earlier, so nothing in 7a asserts it.
4. **Characterisation before change.** Epic 1 pins today's behaviour (pathname/trailing slash, guard layouts, raw History sites, Go static/auth behaviour) under Next before Epic 2 touches 108 files (65 production importers + 43 test files).
5. **Two new Go-side tests precede any layout change** (`static.go` has no test file; `auth.go` exemption is the highest-severity coupling).
6. **Bun-side code is tested by a Jest project that shells out to a pinned `bun` (Story 3.0).** Jest runs in Node, where the `eval` bug and `Bun.build` do not exist, and `bun test` is forbidden (segfault; requirements). So tests that must observe Bun behaviour are split in two files: `X.test.ts` (collected by the `build` Jest project, runs in Node) calls `runInBun("<name>.bun.ts")`, and the `*.bun.ts` script (never collected by Jest; plain `bun <file>` execution, short-lived, JSON on stdout) does the Bun-only work. Pure-logic tests (`verify-output` on fixture trees, `check-dependency-screen`, `budgets`) run directly in Node. **Test-location rule for every story below:** Bun-side tests live under `web-app/build/__tests__/**` and `web-app/build/plugins/__tests__/**` (`*.test.ts`, with `bun/*.bun.ts` helpers) and script tests under `web-app/scripts/__tests__/**` (`*.test.mjs`); all are collected by the `build` project defined in Story 3.0. A story that names any other location for a Bun-side test is wrong.
7. **Every added-scope item has a finite acceptance criterion and an escape hatch** (nothing added beyond the requirements may stall the cutover): D5 focus move (single boolean `FOCUS_ON_NAVIGATE`, not a cutover gate), the 13-route Axe sweep (Story 7.8: only Bun-only regressions block), the ESLint flat-config conversion (Story 6.2: classified-diff tolerance and a time-box; PR 6a does not gate PR 8), the Lighthouse consolidation (Story 6.5: one config, existing warn-only thresholds). The owner pursues full scope; these rules bound it, they do not cut it.
8. **Counts are derived, not hard-coded.** Numbers such as "42 routes", "336 `.css.ts`", "63 importers", "25 title exports" were true on 2026-10-19 or off by a few (re-counted 2026-10-19: 337 `.css.ts` files; 65 production importers of `next/(navigation|link|dynamic)` by regex; 26 files in `src/app` with `metadata`/`generateMetadata` (13 `layout.tsx` + 13 `page.tsx`); 42 `page.tsx`; 152 e2e specs). Every test derives its expected count from the filesystem or `ast-grep`; the numbers in this plan are orientation only.
9. **Rollback is a ladder with an expiry, not "one line" forever** (see Migration Plan): flip the default (valid until PR 9 merges), revert PR 8, and after PR 9 only a forward fix. Every restart that realises a rollback needs the Story 8.5 precondition (`--tmux-keep-server` deployed), enforced by an `install-service` preflight rather than by a document.
10. **One required check, tested workflow edits (pre-mortem failure mode 3; redesigned in iteration 4, review R2-B1/C3/C4).** Branch protection does not exist on `main` today (`gh api repos/tstapler/stapler-squad/branches/main/protection/required_status_checks` returns 404 "Branch not protected"; `rulesets` and `rules/branches/main` return `[]`, checked 2026-10-19), so naming `web-build-gate` is a **first-time protection setup** by the owner after PR 5a merges, not an addition to an existing list. The gate is the aggregate job `web-build-gate`, and **every job named here lives in one workflow file, `.github/workflows/build.yml`** (`needs:` cannot cross workflow files; `ux-analysis.yml` and `e2e-video.yml` cannot be needed from it, and `build.yml` has no Playwright job today). The jobs are separate jobs, not matrix legs: `needs.<job_id>.result` is one string per job (a matrix leg has no result of its own), matrix legs cannot `needs` each other, and the default `fail-fast: true` would cancel the Next leg when the Bun leg fails. They are `web-build-smoke-next` (the existing `web-build-smoke` job at `build.yml:123`, renamed) and `web-build-smoke-bun` (PR 5a), `e2e-next` and `e2e-bun` (new jobs, PR 7b; `e2e-bun` has `needs: e2e-next` and downloads the Next Playwright JSON report as an artifact of the same run). `web-build-gate` has `if: always()` and `needs:` every one of those jobs that exists plus `detect-changes` (a job joins the `needs:` list in the PR that creates it), and runs one step, `web-app/scripts/ci/web-build-gate.mjs`, over `toJSON(needs)` and `vars.WEB_BUILD_GATE_MEMBERS`. **Rule**: a member whose result is `failure` or `cancelled` fails the gate; `success` and `skipped` pass (a job skipped by its `if:` on an irrelevant PR counts as passing, `build.yml:24-30`); non-members are listed in the step summary and never fail the gate; a name in the variable that is not in `needs` fails the gate (typo guard); an unset or empty variable falls back to the script's default `detect-changes,web-build-smoke-next,e2e-next`. **Membership has one mechanism**, the repository Actions variable `WEB_BUILD_GATE_MEMBERS` (comma-separated job ids): a Bun job joins with `gh variable set WEB_BUILD_GATE_MEMBERS --body "..."` after 10 consecutive green runs on `main` or an explicit owner flip recorded in `cutover-gate-results.md`, and leaves by the same command. Neither is a workflow edit, a PR or a branch-protection change, and the `needs:` list is never edited to join or leave (Stories 5.11, 7.10, 8.2, 8.5 and the runbook use only this mechanism). Before joining, a Bun job runs, uploads its results and appears in the step summary but cannot block; it is not `continue-on-error`. Any `strategy.matrix` added to `build.yml` later must set `fail-fast: false` (asserted by Task 5.11c). Every PR that edits a workflow file (Stories 5.11, 6.3, 6.5, 6.10, 7.10, 8.2) first runs green via `workflow_dispatch` on a throwaway branch (the PR body links the run) and `actionlint` runs in `make ready` (Task 5.11c). `workflow_dispatch` already exists in `build.yml:20` and `lint.yml:24`, and is missing from `benchmark.yml` and `ux-analysis.yml`, so the one-line trigger commit (GitHub dispatches only workflows that exist on the default branch; Task 5.11c re-verifies) covers those two files only. A `workflow_dispatch` run reports no pull-request status, so it proves the YAML parses and the jobs pass but **cannot reproduce the deadlock P1-3 describes** (a required context never reported on a PR): Task 5.11d therefore adds one PR-event proof on a scratch repository with protection enabled.
11. **Minification is its own PR; `NODE_ENV` is never a profile switch.** The cutover (PR 8) ships Bun unminified, parity with today's deployed bundle; PR 8b turns minification on after the first Soak window (ADR-004, F4). `build.ts` sets `NODE_ENV="production"` on every invocation; only `dev.ts` sets `"development"`; the only profile switch is `WEB_MINIFY`.
12. **The router entry stays behind the facade.** `createBrowserRouter`, `RouterProvider` and `Outlet` live in `src/lib/nav/entry.tsx` (Task 4.2d), inside the one directory the `react-router` ban exempts; only `src/main.tsx` imports `createAppRouter`/`AppRouterProvider` (the entry-only lint pattern allows `src/main.tsx` and tests, nothing else); `routes.tsx` imports no router API, only facade types and `RouteMeta`. The 8-name facade and the strict ban are unchanged (ADR-001 amendment).
13. **Task boundary and the codemod exemption (triad review, engineering).** A task is one worker run that hand-edits at most **3-5 source files** (its tests count as one more file); a task that would touch more is split. Exemption: a task that applies one mechanical transform across many files is sized by its **script**, not by its file count, if and only if all of the following hold: (a) the transform is a committed script (`ast-grep` rule or `git rm` manifest; the binary is `ast-grep`, never `sg`, which is shadow-utils on Ubuntu runners; Task 2.2d provisions it) and the PR is reviewed as the script, its fixture tests and 5 sampled diffs, not as N file diffs; (b) the task names its file set as a derived glob or list (below), never "all usages"; (c) a verification command with an expected empty or exact output is run and pasted; (d) the script has a liveness test (an input fixture where it must change something, so a no-op codemod fails); (e) running it twice leaves an empty second diff; (f) residual hand edits after the script are at most 5 files, else they go to the follow-on task (2.2c, 2.4b, 2.5b). The exempt tasks are exactly these five:

    - **2.2b**: files `web-app/src/**/*.test.{ts,tsx}` containing `jest.mock("next/navigation"` or `jest.mock("next/link"` (43 on 2026-10-19: 40 mock `next/navigation` and 3 mock only `next/link`, namely `BackgroundActivity.test.tsx`, `NotificationItem.test.tsx` and `__tests__/NotificationItem.hidden.test.tsx`, so the list is the union of two queries, `ast-grep run -p 'jest.mock("next/navigation", $$$)'` and `-p 'jest.mock("next/link", $$$)'`, each run once per `-l ts` and `-l tsx` because the CLI takes one `--lang` per run; the 5 tests that also `import ... from "next/navigation"`, `settings/backlog-stages/page.test.tsx`, `useUpNextTab.test.ts`, `layout/__tests__/Header.test.tsx`, `layout/__tests__/BottomNav.test.tsx` and `hooks/__tests__/useFilterState.test.ts`, are all in this set and get their import rewritten too); script `web-app/scripts/codemods/next-nav-to-facade-tests.yml`; verify: `rg -l "jest\.mock\(['\"]next/(navigation|link)['\"]" web-app/src` prints nothing (43 files before) and `rg -l "from ['\"]next/(navigation|link)" web-app/src --glob '*.test.*' --glob '**/__tests__/**'` prints nothing (5 files before); the 8 files with `jest.mock("next/dynamic"` (`SessionDetail.embedded`, `SessionDetailView.focus`, `.restarted-from`, `.summary-tab`, `.trigger-attribution`, `.backlog-origin-badge`, `.note-error` and `SessionDetail.readonly`) are deliberately outside this task and stay valid until Task 9.1b removes the Next lazy seam (`lazy/impl.next.tsx` keeps `next/dynamic`, Story 2.7), so a regex that matches `next/dynamic` here is wrong; the failing-file list of `npx jest --no-coverage` equals the Story 0.1 control, and Task 2.2a's 5-file mutation check passes.
    - **2.3a**: non-test `web-app/src/app/**/*.{ts,tsx}` importing `next/navigation` or `next/link` (27 files on 2026-10-19; test files in this directory are in 2.2b's set, not here); script `web-app/scripts/codemods/next-nav-to-facade.yml`; verify: `rg -l "from ['\"]next/(navigation|link)" web-app/src/app --glob '!**/*.test.*' --glob '!**/__tests__/**'` prints nothing (27 files before).
    - **2.4a**: non-test `web-app/src/components/**/*.{ts,tsx}` with the same imports (26 files on 2026-10-19); same rule file; verify: the same `rg` over `web-app/src/components` with the same two `--glob '!...'` exclusions prints nothing (26 files before).
    - **2.5a**: non-test `web-app/src/**/*.{ts,tsx}` importing `next/navigation`, `next/link` or `next/dynamic` outside `src/app`, `src/components`, `src/lib/nav` and `src/lib/lazy` (the `src/lib/**` files and strays: 9 files for `navigation|link` on 2026-10-19, so 27 + 26 + 9 = 62, and 65 once `next/dynamic` is included); same rule file; verify: the repo-wide `rg` of Story 2.5 prints nothing once PR 2c has merged. Globs in these commands are anchored with a leading `**/` (`--glob '!**/src/lib/nav/**'`): ripgrep anchors a glob that contains a slash at the working directory, so the unanchored form `!src/lib/nav/**` run from the repository root silently excludes nothing (VERIFIED 2026-10-19 with `!src/lib/hooks/**`, which left the count at 62, versus 59 with `!**/src/lib/hooks/**`).
    - **9.1a**: `git rm` manifest (`web-app/next.config.ts`, `web-app/next-env.d.ts`, the 16 `web-app/src/app/**/layout.tsx`, `web-app/src/app/fonts.ts`) plus script `web-app/scripts/codemods/remove-next-metadata.yml` over `web-app/src/app/**/page.tsx` that (i) removes `export const metadata`/`viewport` and `generateMetadata` and (ii) deletes the `import type { Metadata[, Viewport] } from "next"` line that becomes unused (on 2026-10-19: 13 `page.tsx` files export `metadata`, 11 of them with that type import; the 16 `layout.tsx` files, 13 of which export `metadata`, go with the manifest; 26 is the count over all of `src/app`, 13 layouts + 13 pages, not over `page.tsx` alone); manifest committed in the PR body; verify: `rg "export const (metadata|viewport)|generateMetadata" web-app/src/app` prints nothing (26 files before); `rg "from ['\"]next" web-app/src --glob '!**/lib/nav/impl.next.tsx' --glob '!**/lib/lazy/impl.next.tsx'` prints nothing (the two excluded files are the Next-side seams that Task 9.1b deletes; the unrestricted `rg "from ['\"]next" web-app/src` is the Story 9.1 PR-level check and is empty only after 9.1b); and `git diff --stat --diff-filter=D` lists exactly the manifest. Three sites outside the script must already be gone when this runs: `src/components/telemetry/WebVitalsReporter.tsx:4` (`next/dist/client/web-vitals`, Story 2.8, PR 2d), the `next/dynamic` call sites (Story 2.7, PR 2c) and `next/font/local` in `fonts.ts` (in the manifest). The script's liveness fixture contains a page with `import type { Metadata } from "next"` plus `export const metadata`, and fails if the import line survives.

    The codemod scripts and their `in/out` fixture pairs live under `web-app/scripts/codemods/` and `web-app/scripts/codemods/__fixtures__/`; their liveness tests are in `web-app/scripts/__tests__/codemods.test.mjs`, run from PR 2a by `node --test` through a `test:codemods` package script (the `build` Jest project does not exist until PR 3; Task 2.2d), and also collected by the `build` project from PR 3.

---

## Domain Glossary
*(Ubiquitous language. Names are used verbatim in code, tests, scripts and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `WebBuildMode` | The value of `WEB_BUILD`: `next` or `bun` | Union type `"next" \| "bun"`; single parser `parseWebBuildMode(env)` owned by Story 5.7; Makefile var; deleted at cleanup. Not the minify switch (that is `WEB_MINIFY`) |
| `ModeStamp` | File `web-app/.web-build-mode` holding the current `WebBuildMode`; rewritten only on change | Prerequisite of `web-app/out`; never inside `out/` |
| `OutContract` | `web-app/out/` with `index.html` at the root plus assets and `public/**`; the only interface Go sees | Both modes write it |
| `OutTmpDir` | `web-app/.out-tmp-<pid>`; Bun build target before atomic rename to `out/` | Per-process, avoids concurrent-worktree clobbering |
| `AssetPrefix` | URL prefix of hashed build assets; `/_next/static/` through cutover | ADR-003; `ExemptPrefix` must cover it |
| `ExemptPrefix` | Entry of `exemptPrefixes` in `server/middleware/auth.go` that bypasses login | `/auth/ /login /health /_next/ /favicon` |
| `NavFacade` | The module `src/lib/nav` (public API in `index.ts`) | Only import path for navigation outside `lib/nav` |
| `NavImpl` | One of `impl.next.tsx`, `impl.router.tsx`, `impl.memory.tsx` (the single in-memory fake shared by Storybook and Jest); all satisfy `impl.types.ts`. The lazy seam follows the same pattern with `lazy/impl.next.tsx` and `lazy/impl.bun.tsx`. Hooks and the `Link`/`Redirect` components are members of the impl (hence `.tsx`); there are no shared `Link.tsx`/`Redirect.tsx` | Selected by build-time alias of `@/lib/nav/impl` |
| `NavContract` | The shared jest suite run against every `NavImpl` | Story 4.7 |
| `RouterEntry` | `src/lib/nav/entry.tsx`: `createAppRouter(spec)` and `AppRouterProvider`, the only place `createBrowserRouter`/`RouterProvider`/`Outlet` are used. Not one of the 8 facade names and not re-exported by `index.ts` | Task 4.2d; imported only by `src/main.tsx` and tests |
| `BuildGate` | The aggregate CI job `web-build-gate`; the only web-build check in branch protection | Plan-level decision 10; Story 5.11 |
| `RoutePath` | Union of the 42 route path string literals | Enforced only where it is the key type: `RouteTable`, `RouteMeta`, `FeatureGuard` config. `push`/`replace`/`Link href` accept `string` (call sites append query strings and ids); `RoutePath` does not restrict them |
| `RouteDef` | Sum type: `{load}` \| `{redirect, keepQuery}`. Titles and guard names are NOT stored here: `routes.tsx` looks them up from `RouteMeta` by `RoutePath` | Table value in `src/app/routes.tsx` |
| `RouteTable` | `Record<RoutePath, RouteDef>` in `src/app/routes.tsx` | Element `app`; consumes `lib/nav/paths.ts` |
| `RouteMeta` | Pure data (path, title, guard name; `NOT_FOUND_META` is exported separately for the title of the not-found view) in `src/app/route-meta.ts`; the single title and guard source for the Bun build | Created in Epic 1 under Next; drift-tested against `metadata` exports; key set equal to `RoutePath` and `routeTable` (asserted) |
| `FeatureGuard` | One component (`src/app/FeatureGuard.tsx`) that renders `null` while flags load and `replace("/")` when the flag is off | Introduced in PR 1 (Story 1.7) and consumed by the `backlog`, `settings/backlog-sources`, `changelog` Next layouts; Story 4.3 only wires it into the table |
| `RootShell` | `src/app/RootShell.tsx`: provider and chrome tree extracted from `layout.tsx` | Shared by both builds |
| `NormalisedPathname` | `usePathname()` result with the trailing slash stripped (except `/`) | `lib/nav/normalize.ts` |
| `ReplaceSearch` | Facade helper that updates the query string with replace semantics and without clobbering `history.state` | Replaces the 4 raw `history.replaceState/pushState` sites |
| `RouteAnnouncer` | Polite `role="status"` live region announcing `document.title` after a navigation (reads `document.title`; never imports `route-meta`) | Not `role="alert"`, so e2e alert counts do not change |
| `FocusPolicy` | After a pathname change, focus moves to the layout `main` (tabindex -1) through the `mainRef` that `RootShell` receives from `RouterEntry`, never through a DOM query; never on search-only change, hash change or first load | Decision D5 |
| `LazyBoundary` | The module `src/lib/lazy`; its factory export is `lazyComponent(loader, {loading})`: `React.lazy` + `Suspense` + loading + reload-once on chunk error | Replaces `next/dynamic` |
| `StaleChunkReload` | One-shot `location.reload()` when a hashed chunk import fails | Guarded by a sessionStorage flag to avoid loops |
| `ThemeMap` | Build-time map of 6 theme names to vanilla-extract class names (matrix, cyberpunk77, wh40k, clean, light, dark) | Generated from the same plugin cache as the bundle |
| `ThemeBoot` | Inline synchronous script in `<head>` applying the stored theme before first paint | Generated at build; `<!--THEME_BOOT-->` placeholder |
| `VePlugin` | `build/plugins/vanilla-extract.ts`: Bun plugin compiling `.css.ts` via `compile()` + `processVanillaFile()` | One factory shared by build, dev and theme-map |
| `EvalPatch` | `patches/eval@0.1.8.patch` replacing `eval.js` with a `vm` + `createRequire` shim | ADR-008 |
| `PatchCanary` | Tiny `.css.ts` evaluated by the plugin in `make ready`/CI to prove `EvalPatch` is applied | Fails naming `patches/` |
| `BuildStats` | JSON record per build: duration per stage, JS/CSS bytes, chunk count, Initial JS, peak RSS, mode, minify | `web-app/.build/stats.json`, never in `out/` |
| `Budgets` | `web-app/build/budgets.json`: Total JS, Initial JS, Largest chunk | ADR-005 |
| `InitialJs` | Bytes of the JS closure referenced by `index.html` (`<script src>` + `modulepreload`) | New metric |
| `BuildTestProject` | Jest project `build` (node env) collecting `web-app/build/**/__tests__/**/*.test.ts` and `web-app/scripts/__tests__/**/*.test.mjs`; Bun-only assertions run through `runInBun(...)` against a pinned `bun` | Story 3.0 |
| `E2eBaseline` | `tests/e2e/baseline/next-e2e-baseline.json`: per-spec pass/fail/flaky outcome of the full Playwright suite on the Next build (3 runs) | Story 0.1; the cutover gate (Story 8.1) compares against it |
| `ThemeBootScript` | Pure string builder `buildThemeBootScript(map: ThemeMap): string` in `web-app/src/styles/theme-boot-script.ts`, imported by Next `layout.tsx` and by `build/html.ts`; imports nothing from `theme.css.ts` | Story 5.4 |
| `SpikeGate` | A pre-implementation experiment S0-S4 with numeric pass/fail criteria | Epic 0 |
| `GateRecord` | `project_plans/web-app-bun-build/research/spike-results.md`: measured outcomes and the go/no-go decision | Story 0.6 |
| `DualBuildPeriod` | From PR 2a through the cleanup PR, when both builds must stay green | Ends with Epic 9 |
| `Soak` | Window between cutover and cleanup in which the owner's instance runs the Bun build: at least 14 calendar days and 3 distinct dev sessions after PR 8, then at least 7 days after PR 8b; stop-loss 45 days after PR 8 | Wall-clock blocker (Story 8.4) |
| `ParityDiff` | Computed-style/DOM diff of selected routes between the Next and Bun builds | Story 7.9 |
| `ManualInstance` | Isolated stapler-squad instance on the 62871/62872 block with its own tmux socket: 62871 is its HTTP `PORT` (the dev server's `NEXT_PUBLIC_API_URL` target), 62872 its TLS `--remote-port`. The Bun dev server uses spare port 62875 | Only target for manual or e2e checks; never :8543 |

---

## Pattern Decisions

**Alternatives explored (Step 0.5).**
- *A. Big-bang swap* (replace Next, router and tooling in one PR). Strength: fewest total changes. Weakness: no revert path short of hundreds of files; failures are not attributable.
- *B. Strangler / branch-by-abstraction behind a facade with a dual build* (chosen). Strength: every PR is shippable and revertible, router and bundler risk are separated, tests become router-independent. Weakness: two builds to keep green for a bounded period.
- *C. Switch bundler to Vite 8 (or Rsbuild) now, Bun later.* Strength: official VE plugin, no patch. Weakness: gives up the owner's chosen 2x speed margin; but it is the pre-agreed exit (ADR-007), and 80% of this plan is bundler-independent, so choosing B does not foreclose C.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Navigation (`src/lib/nav`) | Facade + Adapter, bound at build time | GoF; Feathers "branch by abstraction" | Direct router calls in 65 files (regex count) | Router decision decoupled from bundler; 43 test mocks collapse to one |
| Per-build `NavImpl` / `LazyBoundary` impl selection | Strategy selected by module alias (compile-time) | GoF Strategy | Runtime `if (isBun)` | Dead code elimination; no Next code in the Bun bundle |
| Route table | Table-driven dispatch with `satisfies Record<RoutePath, RouteDef>` | Fowler (Table-Driven Methods), type-driven-design | File-system routes or TanStack codegen | 42 flat routes; compile error on a missing route; no codegen dependency |
| `RoutePath` | Closed string-literal union (newtype-like) | type-driven-design | Plain `string` | Cannot navigate to a non-route |
| `RouteDef` | Sum type (`load` vs `redirect`) | type-driven-design | Optional-field bag | Exhaustive handling; redirect-only pages cannot also have a loader |
| `WebBuildMode` | Union type with exhaustive `switch` | type-driven-design | Free-form env string | Unknown mode fails loudly |
| `build/build.ts` | Template Method: ordered named stages, each unit-testable and timed | GoF; PoEAA (Transaction Script is fine here) | One 300-line script | Stages are testable and reported in `BuildStats` |
| `VePlugin` compile results | Memoisation keyed by file path plus a content hash of the file and of its VE dependency closure (not mtime: coarse after checkout and CI restore), size-bounded; full-reload fallback on `*.css.ts` change if S2 shows Bun does not re-request dependants | PoEAA (Identity Map analogue) | Recompile every request | Correct dev invalidation (F7) and faster rebuilds |
| `.out-tmp` then rename | Atomic swap (write-then-rename) | Pragmatic Programmer (crash-only) | Build in place | Failed or concurrent builds never leave a half-written `out/` |
| Legacy behaviour pinning | Characterisation tests | Feathers | Rewrite then hope | Router swap needs a behavioural net |
| Next-to-Bun cutover | Strangler Fig + feature flag (`WEB_BUILD`) | Fowler | Big bang | Bounded dual period with one-line rollback |
| Next-only imports | Anti-corruption layer + lint ban + resolver ban | DDD | Shimming `next/*` in prod (as the spike did) | Shims hide leftover imports (F1) |
| `RootShell` | Extract Class / Extract Function (Refactor-first) | Fowler (Refactoring) | Duplicate provider trees per build | Drift between builds |
| Output verification | Fitness function in the build (`verify-output.ts`) | Evolutionary architecture | Manual checks | Fails the build on missing assets, `process.env` leaks, budget overrun |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| Router/navigation: 65 prod (regex count; 63 by the earlier `ast-grep` count) + 43 test files import `next/*`; only `NavLink` abstracts links | Framework coupling welded to the bundler decision | **Isolate via seam** (`src/lib/nav`) preceded by **Refactor-first** (Epic 1: `RootShell`, `RouteMeta`, characterisation tests) | `research/architecture.md` §10: code is healthy apart from coupling; a broad rewrite would be gold-plating |
| `app/layout.tsx` mixes `<html>`, FOUC, fonts, metadata and the provider tree | Two concerns in one file; the provider tree must be shared | **Refactor-first** (Story 1.1) | The Bun entry needs the same tree without the Next shell |
| Titles scattered across 25 `metadata` exports | No single source; duplicated in e2e | **Refactor-first** (Story 1.2 `RouteMeta` + drift test) | Becomes the title source for the router build |
| 4 raw `history.replaceState/pushState` sites (`useUpNextTab.ts:69`, `UnfinishedTab.tsx:144`, `SessionDetailView.tsx:502`, `useTrayDismissal.ts:51`; lines 67, 55 and 142 are comments, re-verified 2026-10-19) | Bypass the router; stale `useSearchParams` under RR; `null` state wipes router state | **Refactor-first** (Story 2.6 `ReplaceSearch` helper) | RR would break them silently |
| `server/middleware/static.go` has no test file; masks missing assets as HTML 200 | No regression net on the Go side | **Refactor-first** (Stories 1.5, 1.6) | Cutover safety depends on it |
| `web-app/.size-limit.json` duplicated in `web-app/package.json` | Two sources of truth, both Next-named; today's 5 MB Total JS gate (`benchmark.yml:295`) is unverified against the production build | **Refactor-first** (Story 6.4 deletes one; Task 0.4d measures whether the 5 MB gate passes today, so the move to 6,349,052 B is recorded as a relaxation or a fix) | ADR-005 |
| `scripts/shared-node-modules.sh` hash ignores patches | Stale shared tree after a patch change (F11) | **Extend as-is** + hash input and test (Story 3.3) | Adding an input does not add a violation; the script and its `.test.sh` already model `file:` deps |
| `.eslintrc.json` extends `next/core-web-vitals`; `next lint` exit-code coupling | Next-specific rule set and gate semantics | **Refactor-first** (Stories 6.1-6.3) | Required to delete `eslint-config-next` |
| `next.config.ts` webpack hacks (shared cache dir, CSS chunk consolidation, `extensionAlias`) | Next-only | **Isolate via seam** (stay under `WEB_BUILD=next`, deleted in Epic 9) | Not worth touching; removed with Next |
| `src/app/routes.tsx`, `NotFound.tsx`, `FeatureGuard.tsx`, `RootShell.tsx` live inside Next's `app/` directory during the dual period | Type-checked and linted by the Next build; Next ignores non-route files | **Extend as-is**; stated here, not moved to `src/router/` (moving would add a `boundaries` element for a 2-PR lifetime) | Harmless; Story 4.3 asserts Next's route list is unchanged by their presence |
| `OmnibarContext` importing a `sessions` component; ADR-010 cross-domain baseline | Orthogonal layering debt | **Extend as-is** (not touched) | Unrelated to this migration (`research/architecture.md` §10) |
| 10 pages render a nested `<main id="main-content">` (duplicate ids) | Pre-existing a11y defect | **Extend as-is**; no new instance. `FocusPolicy` targets the layout `<main>` through a `mainRef` that `RootShell` receives from the router entry (no DOM query, so never a page's nested one; Story 4.5) | Fixing it is outside parity; do not worsen it (`research/ux.md` §4) |
| `test/*` and `debug/*` pages ship in production | Questionable surface | **Extend as-is** (decision D2) | Parity; owner may prune later |

---

## Migration Plan
*(No schema or data change. This is the migration of build artifacts and tooling.)*

- **Unit of migration**: eighteen PRs (plus the optional Story 9.4 PR), each independently shippable. Contract between web and Go: `web-app/out/` with `index.html` at its root; unchanged throughout.
- **PR decomposition (every PR stays green under Next, and each states its own revert)**

| PR | Stories | Change | Rollback of this PR alone |
|----|---------|--------|---------------------------|
| 0 | 0.1-0.7 | Spike scripts, committed spike (`spikes/bun-spike/`), feasibility doc, `GateRecord`, `E2eBaseline` | Delete the files; no product change |
| 1 | 1.1-1.7 | `RootShell`, `RouteMeta`, `FeatureGuard` (consumed by the three Next layouts), characterisation tests, Go tests, Go 404 for asset-prefix misses | Revert the PR; the 404 change is one hunk in `static.go`; layouts return to their bespoke guards |
| 7a | 7.1-7.9 (Next halves, incl. 7.9b), 7.11a-b, 7.12 (baseline), 7.13a, 7.14 (route smoke spec) | Next-baselined e2e specs, fixtures, triage of Next-quirk specs | Revert; specs are additive |
| 2c | 2.7 | `LazyBoundary` (Next impl = `next/dynamic`), 4 files migrated | Revert; independent of the facade |
| 2d | 2.8 | `web-vitals` package swap | Revert; independent |
| 2a | 2.1, 2.2 | Facade core, `impl.next.tsx`, `impl.memory.tsx`, global jest stub (back-compat: mocks both module ids), 43 test files, `ast-grep`/`rg` provisioning (Task 2.2d) | Revert; production code still imports `next/*` directly |
| 2b | 2.3-2.6, 2.9a-c | Production migration to the facade, `ReplaceSearch`, lint ban at `warn`, NavLink collapse | Revert 2b alone: code returns to direct `next/*` imports; 2a's facade stays unused |
| 2e | 2.9d | Ban flips to `error` one merge cycle after 2b; back-compat jest mock removed | Revert; ban returns to `warn` |
| 3 | 3.0-3.5 | `build` Jest project, `bun patch`, pins, canary, shared-hash change | Revert; Next build ignores the patch (verified in Story 3.2); nothing depends on the `build` project except PRs 4-5 tests |
| 4 | 4.1-4.7 | `impl.router.tsx`, `routes.tsx`, `main.tsx`, announcer (dead code until PR 5a) | Revert; nothing selects it |
| 5a | 5.1-5.5, 5.7, 5.9-5.11 | `build/`, Makefile `WEB_BUILD` seam (default `next`), `verify-output`, CI jobs `web-build-smoke-next` and `web-build-smoke-bun` and the aggregate `web-build-gate` | Revert, or never set `WEB_BUILD=bun`; default stays `next` |
| 5b | 5.6, 5.8 | Dev server, dev-stack and Playwright dev-mode harness | Revert; `next dev` path untouched |
| 6a | 6.1-6.3 | ESLint: replace `next/core-web-vitals`, flat config, warning gate without `next lint`. Functionally independent of the Bun build, but Task 6.3b edits `Makefile:1021` after Task 5.7b has rewritten the same file, so 6a is merged **after 5a** (a textual-ordering dependency, not a behavioural one) | Revert the PR; `eslint-config-next` is still installed, so the eslintrc path returns whole. Does not gate PR 8 |
| 6b | 6.4-6.10 | size-limit/budgets, other workflows, Storybook, docs, Go built-asset walk, jscpd/registry, supply-chain checks | Revert; each story is a separate commit, so partial reverts are possible by commit, not only by file |
| 7b | 7.2b, 7.2c, 7.5b, 7.7b, 7.10 (the whole story: a-c, e), 7.11c, 7.12 (Bun assertions), 7.13b | Bun-only assertions, new `e2e-next` and `e2e-bun` jobs in `build.yml` with expected-diff list | Revert; additive (first remove the Bun jobs from `WEB_BUILD_GATE_MEMBERS` with `gh variable set`, no workflow edit, Story 8.5; the PR's `needs:` additions revert with it) |
| 8 | 8.1-8.5 | Default flips to `bun` in Makefile and CI, **unminified** (`WEB_MINIFY=0` explicit in the Makefile); `install-service` preflight | Flip the default back to `next` (valid until PR 9 merges) |
| 8b | 8.6 | Makefile drops `WEB_MINIFY=0` (minified default), after the first Soak window and a passing minified gate | Revert PR 8b: Makefile `WEB_MINIFY ?= 0` back, the CI default of Task 8.6a back, then restart the service (ladder item 1b) |
| 9 | 9.1-9.3 | Delete Next code and deps | **Not revertible once later PRs merge**: forward fix only (see ladder) |

- **Dependency order of the PRs**: 0 -> 1 -> {7a, 2c, 2d} -> 2a -> 2b -> 2e; 3 -> 4 (needs 2b) -> 5a -> {5b, 6b, 7b}; 6a after 2e and after 5a (no behavioural dependency on the Bun build; it must follow 5a only because Tasks 5.7b and 6.3b both edit the `Makefile` lint line, see the Makefile chain in Dependency Visualization; **which order wins**: 6a before PR 8 is preferred, but 6a does not gate PR 8, so if 6a stalls PR 8 merges first and Task 6.3b owns the rebase over PR 8's Makefile hunks, which are disjoint from its own: `WEB_BUILD ?=`, `WEB_MINIFY` and `install-service` versus the `lint` line near `Makefile:1021`); 8 needs 5a, 5b, 6b, 7a, 7b; 8b needs 8 + the first Soak window; 9 needs 8b + the second Soak window + 6a. "Independently revertible" means: reverting a PR is safe at the moment it is the newest merged PR that depends on it. PRs 4-9 depend on the facade (2a/2b), so 2a/2b cannot be reverted while they are merged; this is stated, not hidden.
- **Zero-downtime**: the Go binary embeds the UI; the build flip is a normal release. Stale browser tabs after a restart are covered by the Go 404 plus `StaleChunkReload` pair (Stories 1.6 and 2.7).
- **Rollback ladder (honest form)**: (1) flip the `WEB_BUILD` default and rebuild: valid from PR 8 until PR 9 merges; (1b) after PR 8b, revert PR 8b alone: restore `WEB_MINIFY ?= 0` in the `Makefile`, restore the CI default that Task 8.6a changed (the same PR, so a revert of the PR covers both), keep the `verify-output` assertion consistent (`stats.minifyExplicit === true` and `stats.minify === false` once the line is back), then restart the service (`make install-service`, planned window, Story 8.5 preflight); CI budgets stay enforced on the separate minified build, so after this revert budgets are checked on minified bytes while the deployed bytes are unminified; (2) revert PR 8: same window as (1); (3) **after PR 9 merges there is no rollback to Next other than a forward fix**: the tag `web-dual-build-last` restores a tree with both builds only by discarding every commit merged after PR 9 (new pages without `metadata`, deleted `layout.tsx` conflicts), so it is an emergency measure, not a rollback; the rollback window therefore expires when PR 9 merges, and the Soak (Story 8.4) exists to make PR 9 safe. **Every realised rollback or flip needs a service restart**: `make install-service` kills the tmux server and all live sessions unless the deployed unit passes `--tmux-keep-server` (CLAUDE.md WARNING; `docs/explanation/tmux-keep-server-on-restart.md`). Story 8.5 makes that a precondition, enforces it with an `install-service` preflight, and writes the runbook; required-check names do not change at the flip because only the aggregate `web-build-gate` is required (decision 10).

## Observability Plan
- **Build metrics (CI)**: `build/build.ts` writes `web-app/.build/stats.json` with: `mode`, `minify`, `durationMs` (total and per stage: guard, clean, theme-map, bundle, html, public, verify, swap), `jsBytes`, `cssBytes`, `chunkCount`, `initialJsBytes`, `largestChunkBytes`, `gzipJsBytes`, `peakRssKb` (CI wraps with `/usr/bin/time -v`), Bun version. CI appends a table to `$GITHUB_STEP_SUMMARY` and uploads `stats.json` as an artifact; `.github/workflows/ci-duration-trend.yml` is extended with the build series if it accepts extra series (UNVERIFIED; else a small `build-metrics` step in `build.yml`). The Next-mode job emits the same fields (duration, bytes, chunk count) for a like-for-like before/after table.
- **Logs**: each build stage logs `[web-build] <stage> <ms>`; failures print the failing file (VE plugin throws with the `.css.ts` path) and exit non-zero. The dev server logs plugin recompiles with durations.
- **Budget alerts**: `verify-output.ts` fails the build on Total JS / Initial JS / Largest chunk overrun (ADR-005). No oncall alerts; failures surface as CI failures (requirements).
- **Runtime telemetry continuity**: `WebVitalsReporter` keeps emitting the same fields via the `web-vitals` package; `web_vital.fid` stops (FID removed upstream), INP continues; checked in Story 2.8. `StaleChunkReload` increments an existing telemetry counter/`performance.mark` so reloads are visible (no new alert); asserted in Task 2.7e.
- **Soak observation** (Epic 8): the owner's instance runs `WEB_BUILD=bun`; the runbook lists what to look at (browser console errors, `Unexpected token <`, login page asset 200s) and how to revert.

## Risk Control
- **Feature flag**: `WEB_BUILD` (Makefile and CI variable). Default `next` until PR 8; `bun` afterwards; variable deleted in PR 9. `WEB_MINIFY=0` is the cutover default (set explicitly in the Makefile until PR 8b) and the debug-build switch afterwards; `WEB_MINIFY` is the only profile switch (`NODE_ENV` is always `production` in `build.ts`). Dev server: `WEB_DEV_HMR=0` disables HMR if #40248 bites.
- **Rollback procedure**: flip the `WEB_BUILD` default and rebuild, then restart the service during a planned window (`make install-service`; never to try changes: use a ManualInstance). Precondition: the deployed unit passes `--tmux-keep-server` (Story 8.5), otherwise the restart kills every live session. Valid until PR 9 merges; afterwards forward-fix only (Migration Plan ladder).
- **Staged rollout**: PRs 0-7b do not change what ships. PR 8 ships the Bun build (unminified) to the owner's instance first (soak), then CI default; PR 8b turns minification on after the first Soak window; PR 9 only after the second Soak window ends clean.
- **Safety nets**: lint ban on `next/*` outside the facade; resolver ban on `next*` in the Bun build; `verify-output.ts`; `PatchCanary`; Go asset-exemption test; both builds run in CI until cutover behind the single `web-build-gate` check; `build.ts` runs `verify-eval-patch` as its first stage; e2e on a ManualInstance with `--remote-access`.
- **Supply chain**: exact pins (Bun, `react-router`, `web-vitals`, `@vanilla-extract/*`, `eval`), committed `bun.lock`, `--frozen-lockfile`, `minimumReleaseAge`, minimal `trustedDependencies`, `oven-sh/setup-bun` pinned by SHA, no `pull_request_target` checkouts; every new dependency gets an `npm pack` inspection (no `github:` optionalDependencies, no unexpected large root JS).

## Spike Gates (Epic 0): pass/fail criteria and Vite 8 trigger conditions

| Gate | Question | PASS criteria (all) | FAIL consequence |
|------|----------|--------------------|------------------|
| S0 | Does Next, Jest, ESLint and Storybook still work on a Bun-installed tree (PR #1100)? Which packages need `trustedDependencies`? | `WEB_BUILD=next make web-build` exits 0 and JS bytes are within 2% of the pnpm development-mode build (21,666,571 B); Jest: identical set of failing files to a pnpm run of the same commit; `storybook build` exits 0; `eslint` loads the `file:` plugins; a VE compile of 3 files succeeds with only `esbuild` and `@vanilla-extract/css` trusted (or the minimal set found); **the full Playwright suite (152 specs) has been run 3 times on the Next build on a ManualInstance and committed as `E2eBaseline`** (each spec classified pass / fail / flaky); **`spikes/s0-rerun.sh` (Task 0.1d) re-runs the numeric S0 checks in one command and is re-run on the actual merged PR #1100 `main` before PR 3 starts, and again whenever #1100's head changes between gate sign-off and its merge; the `GateRecord` names the #1100 commit each S0 result was taken on** | Dual build not viable (ADR-006): Epics 8 and 9 collapse into one cutover PR with a revert plan; Next checks move to a pnpm worktree |
| S1 | Does an HTML-entry `Bun.build` work with splitting, plugins, `publicPath:"/"`, the `_next/static` naming prefix (**entry, chunk and asset all set**) and woff2? Is Bun CSS bug #45730 hitting VE output? Does the theme placeholder survive Bun's HTML minifier? | HTML entry + `splitting` + VE plugin + `define` builds the real `src/` entry with `naming:{entry,chunk,asset}` all under `_next/static/`, **and `out/index.html` exists at the root after the `html` stage** (VERIFIED on Bun 1.4.3: an HTML entry is emitted at `out/_next/static/chunks/index-<hash>.html`, so the stage either moves the post-processed emitted HTML to `out/index.html` (A') or the build uses a script entry plus `html.ts` (B); no stray second `*.html`); **a walk over every file in `out/` finds each under an `auth.go` exempt prefix or on the explicit public-file allowlist** (the earlier spike `build2.ts` set `entry:"[name].[ext]"`, `chunk:"chunks/..."`, `asset:"assets/..."`: none of those is exempt, so it would have locked out `/login`); every asset URL in `index.html` starts with `/_next/static/`; the `THEME_BOOT` placeholder survives `minify:true` in the emitted HTML (candidate forms tried in order: `<!--THEME_BOOT-->`, `<script id="theme-boot"></script>`, `<meta name="theme-boot">`; the surviving form is recorded); a nested deep link `/backlog/board` served by the real Go binary (ManualInstance) returns JS with a JS content type for each asset; woff2 `url()` resolved (external or copied under `_next/static/media`), no data-URI inlining of the 194,364 B of fonts; `jsx: preserve` in tsconfig does not break the transform; `src/gen/**` `.js`->`.ts` imports resolve; an import of `next/navigation` throws from the resolver; invalid CSS throws with `throw:true`; **CSS census**: for each of the 6 theme classes the **resolved custom-property map** (name -> computed value at `:root` with that class, read in a browser) is equal between Bun and Next CSS, and rule counts are reported but only a difference that is not explained by minifier rule-merging fails (a reordering bug leaves counts equal, a legitimate merge changes them), on at least one pinnable Bun version (test 1.4.3 and 1.3.11) | Script entry + `html.ts` design B if only HTML-entry fails; Trigger T6 (Vite 8) if the census fails on all pinnable versions |
| S2 | Does the REAL app work in `bun dev`, including React Fast Refresh? | With a prototype router entry (shared with S4) against a ManualInstance on 62871/62872: all 42 routes load, zero console errors; editing a component on 3 stateful pages (session terminal pane, omnibar, session list) preserves a React state marker (Fast Refresh) in all 3; editing `src/styles/theme-contract.css.ts` with a page open updates dependants without a manual reload (otherwise the mtime memo in Story 5.1 is mandatory, recorded); `public/` files served; API calls succeed cross-origin with `NEXT_PUBLIC_API_URL` and the host bound to `localhost`; a type error and a runtime error each show an overlay and recover; `madge` reports the import-cycle count (for #40248); after 200 edits RSS stays under 1.5 GB with no crash | **Amber** (live reload works, state not preserved on some page): the record lists which pages lose state and the measured time to a full reload, and the owner decides; `madge` cycle count is **informational**, never a pass line, but the cycles that lie on the HMR path of a page that lost state are listed; **Red** (does not render all routes, or hot reload broken): Trigger T1 |
| S3 | All 42 routes lazy: sizes, CSS ordering, memory | Entry uses `RouteTable` with `import()` per route; at least 42 route chunks; xterm, codemirror, monaco, recharts, react-markdown are not in the Initial JS; minified total JS <= 6,349,052 B; Initial JS compared with the Next per-route first-load JS measured in Task 0.4d (ceiling rule in Story 5.9; a miss is AMBER with numbers, not RED); the 6 `dynamic()` call sites (3 files; a 4th importer, `test/terminal-stress`, only has an unused import) produce lazy chunks; build wall <= 10 s (warm median of n >= 5; cold reported, Story 0.4); peak RSS <= 2 GB single and 3 concurrent builds complete with no OOM; ParityDiff on 5 routes (`/`, `/settings`, `/backlog`, `/review-queue`, `/history`) x 2 viewports **x 6 theme classes** (plus forced `:hover`/`:focus-visible` on the first 5 focusable elements per route) shows zero unexplained computed-style differences, where an allow-list entry must carry a linked reason; **and (route-order-dependent CSS, #45730 / T4, pre-mortem P1-1)** ParityDiff over **all 42 routes cold** at 1 viewport x 2 themes (`clean` and `dark`), **and** an in-session navigation-sequence walk (3 fixed + 3 seeded-random orderings of all 42 routes, client navigation only, no reload; the seeds are recorded) in which the computed-style snapshot of the final route equals the same route cold-loaded on the same Bun build (order independence, no Next needed) and equals Next's, **and** a census of the ordered rule selectors of the shared (non-lazy) CSS per theme class equal to Next's modulo explained rule merges. Any unexplained delta fires T4 | Trigger T2 (size or splitting) or T4 (CSS ordering) |
| S4 | Does React Router 7.18.4 (data router) behave as the facade needs? | `/x/` matches `/x`; `document.title` updates from `<title>` rendered in the root on client navigation (else `useEffect` fallback adopted); scroll container identified (window vs `<main>`) and `ScrollRestoration` verdict recorded; search-param setter push vs replace matrix per call site type preserved; raw `history.replaceState` is NOT observed by `useSearchParams` (expected; confirms Story 2.6) and `ReplaceSearch` via `navigate({replace:true})` is observed; `history.state` retains the `ssqTray` marker through RR navigation; `FeatureGuard` blank-vs-redirect parity with the 3 layouts; tarball has no `optionalDependencies`, no `github:` source, no install scripts, version not on the banned list | Router decision returns to the owner (ADR-001); Epic 4 does not start |

**Vite 8 fallback triggers** (ADR-007): T1 S2 red; T2 S3 size/splitting; T3 bundler crash or miscompile in two or more CI runs (watched through the Soak); T4 S3 CSS ordering unfixable in one repair wave; T5 a vanilla-extract minor upgrade cannot be made green in one repair wave; T6 S1 census fails on every pinnable Bun version. A fired trigger is reported to the owner with the numbers; the plan then switches Stories 5.1-5.6 to a Vite config and drops Epic 3.

---

## Flagged choices (owner decisions; defaults applied so work can proceed)

| ID | Choice | Default in this plan | ADR | If the owner chooses otherwise |
|----|--------|---------------------|-----|-------------------------------|
| F1 | Router (**owner-confirmed 2026-10-19**) | React Router **7.18.4**, exact pin, behind `src/lib/nav`. TanStack Router only at an audited exact pin >= 1.170.0 (never 1.169.5 / 1.169.8); not recommended | ADR-001 | Rewrite `impl.router.tsx`, re-run S4 against it, re-plan Epic 4; Epics 1-3 unchanged |
| F2 | Storybook (**owner-confirmed 2026-10-19**) | Keep on `react-webpack5`; delete unused `@storybook/nextjs`; CI `storybook build` job | ADR-002 | Delete Storybook entirely (Story 6.6 variant) |
| F3 | Asset prefix | Keep `/_next/static/` through the dual period; rename later with `auth.go` + `static.go` + Go test in one PR (Story 9.4) | ADR-003 | Rename at cutover: fold Story 9.4 into PR 8 (loses the one-line rollback) |
| F4 | Minification timing | The cutover (PR 8) ships Bun unminified, parity with today's deployment; minify is switched on by its own revertible PR 8b after the first Soak window. Size budgets are enforced on a minified CI build from PR 5a on | ADR-004 | Fold Story 8.6 into PR 8 (minified at cutover, the original default; loses attribution of minifier vs bundler bugs) |
| F5 | Budget shape | Total JS + Initial JS + Largest chunk in `budgets.json`; `size-limit` only for Total JS | ADR-005 | Total JS only (loses first-load guard) |
| F6 | Dual-build seam | `WEB_BUILD=next|bun`, both write `out/`, ModeStamp, per-build alias | ADR-006 | If S0 fails: single cutover PR |
| F7 | Bundler fallback gates | Vite 8 + official VE plugin on triggers T1-T6 | ADR-007 | Rsbuild instead of Vite 8 if Vite 8 re-measured on the real app misses 10 s |

**Secondary decisions (defaults applied; no ADR, reversible in one story)**

| ID | Decision | Default | Where |
|----|----------|---------|-------|
| D1 | Unknown-path behaviour | Render a small not-found view without redirecting; Go still answers 200 (no Go change). Today an unknown path renders the sessions page. Recorded as a deliberate behaviour change, excluded from the parity claim. Content criteria (heading, links, title, announcement) are in Story 4.3c; title comes from `RouteMeta`'s `NOT_FOUND_META` | Stories 4.3, 7.7 |
| D2 | `test/*` and `debug/*` pages in production | Keep (parity); tests reference some | Story 4.3 |
| D3 | Ship `.map` files in the embedded dist | Keep parity (linked maps shipped; `sourcemap:"linked"` is set in Story 5.5 and excluded from the JS budgets in Story 5.9); the binary-size delta is measured by `scripts/measure-web-build.sh` and reported in Story 8.3 (not a `BuildStats` field) | Stories 5.5, 5.9, 8.3 |
| D4 | Link `manifest.json` | No (nothing links it today); follow-up, not parity | Story 5.3 |
| D5 | Focus on route change | Move focus to the layout `#main-content` after a pathname change, unless the user or the page already placed focus in an input (edge cases in Story 4.5). Added scope, so bounded: one boolean `FOCUS_ON_NAVIGATE` in `lib/nav/policy.ts`; if Story 7.2b still shows an unresolved conflict after one repair wave, the default becomes `false` and the finding is recorded. **Not a cutover gate** | Stories 4.5, 7.2 |
| D6 | `bun dev` backend reachability | Cross-origin via `NEXT_PUBLIC_API_URL` and `STAPLER_SQUAD_EXTRA_ORIGINS` (parity with `next dev`); same-origin proxy deferred | Story 5.6 |
| D7 | `Total JS` ceiling | Set from S3 numbers, hard cap 6,349,052 B | Story 5.9 |
| D8 | Focus ring on the programmatically focused layout `main` | `outline: none` on `main#main-content[tabindex=-1]` only, matching Next's no-focus behaviour; justification and tests in Story 4.5 (UX-58) | Stories 4.5, 7.2a |

---

## Owner handoff and sign-off points

*(Where the owner is asked for a decision and what each one blocks; the calendar blockers themselves are listed once under "Wall-clock blockers".)*

| Point | When | Decision | What waits on it | What does not wait |
|---|---|---|---|---|
| **F1 router sign-off, relative to gate S4** | Story 0.6, **after** gate S4 reports and **before** Epic 4 starts. It is not asked earlier: S4 is the evidence (matching, titles, search params, foreign `history.state`, tarball checks) the owner signs against | F1 CONFIRMED by owner 2026-10-19 (React Router 7.18.4); S4 PASS is still required. Original options: confirm React Router 7.18.4, or choose TanStack at an audited pin >= 1.170.0 | Epic 4 (PR 4) and everything after it, including PR 5a's router entry; S4 PASS **and** the F1 sign-off must both be in the `GateRecord` | Epics 1, 2 and 3 and PR 7a are router-independent (the facade sits in front of either router), so they proceed while S4 runs and while the owner decides |
| If F1 flips to TanStack | Story 0.6 | Re-run S4 against the chosen pin; rewrite `impl.router.tsx` (Epic 4 re-planned) | Epic 4 only | Epics 1-3 unchanged |
| F2-F7, D1-D7 and the 11 accepted scope additions | Story 0.6 (one reply) | Accept, decline or bound each; declining removes the named story or flips its flag | Only the story named in each row | Everything else |
| Budget relaxation (6,349,052 B vs today's 5 MB gate) | Story 0.6, from Task 0.4d | Accept the relaxation or set the lower ceiling | Story 5.9 | Everything before PR 5a |
| Amber S2 | After gate S2 | Accept live reload without state-preserving Fast Refresh, or fire T1 | Epic 5's dev server (Story 5.6) | Build stories |
| Cutover | Before PR 8 | Written decision on a missed 5x ratio (Story 8.3), `--tmux-keep-server` record (Story 8.5), flip approval | PR 8 | PR 5a-7b |

---

## Effort estimate
*(Bands and weights INFERRED per ESTIMATION.md; recalibrate after the first implementer run. No human-time units. CU per row rounded half up; phase and grand totals computed from raw tokens.)*

| Story | Tasks | Classes | Raw tokens | Est. CU (x1.1; 12% output) | Wall-clock blocker |
|---|---|---|---|---|---|
| 0.1 Gate S0, Next/Jest/ESLint/Storybook on a Bun-installed tree, plus the Playwright `E2eBaseline` on Next and the one-command S0 re-run | 6 | M+S+S+XS+S+XS | 335k | 369k | PR #1100 merge (owner/other session); 3x152-spec run machine time |
| 0.2 Gate S1, HTML-entry `Bun.build` (HTML relocated to `out/index.html`), `naming.entry/chunk/asset`, assets, defines, CSS census, theme-placeholder survival, remote-auth `/login` feasibility | 4 | M+M+S+S | 370k | 407k |  |
| 0.3 Gate S2, the real app in `bun dev` with Fast Refresh | 3 | M+S+M | 315k | 347k | Owner call if amber |
| 0.4 Gate S3, 42 lazy routes: size, CSS ordering (6 themes, 42 routes cold, navigation-sequence walk, rule-order census), memory, Next first-load baseline, baseline timing protocol (n>=5, cold/warm, `next dev` reload, footprint) | 6 | S+S+M+S+M+S | 480k | 528k |  |
| 0.5 Gate S4, React Router 7.18.4 behaviour spike, plus the built-Next announcer politeness check | 4 | S+S+XS+XS | 150k | 165k |  |
| 0.6 Gate record and owner sign-off, plus the value quantification | 3 | S+S+S | 165k | 182k | Owner sign-off on gate record |
| 0.7 Commit the spike and the feasibility doc (runs first in PR 0) | 2 | S+XS | 75k | 83k |  |
| 1.1 Extract `RootShell` from `app/layout.tsx` | 2 | S+S | 110k | 121k |  |
| 1.2 `RouteMeta` table with a drift test, and contributor guidance that ships with it | 3 | S+S+S | 165k | 182k |  |
| 1.3 Characterise pathname and trailing-slash behaviour | 2 | S+S | 110k | 121k |  |
| 1.4 Characterise feature-flag guards and the raw History sites | 2 | S+M | 185k | 204k |  |
| 1.5 Go characterisation tests for `static.go` and the auth exemption | 2 | S+XS | 75k | 83k |  |
| 1.6 404 for missing assets under the asset prefix | 1 | S | 55k | 61k |  |
| 1.7 `FeatureGuard` consumed by the three Next layouts | 2 | S+S | 110k | 121k |  |
| 2.1 Facade core, contract types, `impl.next.tsx`, typed search-param helpers | 4 | S+S+S+S | 220k | 242k |  |
| 2.2 Global jest stub (back-compat) and 43 test files onto `@/lib/nav`, plus `ast-grep`/`rg` provisioning | 4 | S+M+S+S | 295k | 325k | |
| 2.3 Migrate production `src/app/**` to the facade | 2 | S+S | 110k | 121k |  |
| 2.4 Migrate production `src/components/**` to the facade | 2 | M+S | 185k | 204k |  |
| 2.5 Migrate production `src/lib/**` and remaining files to the facade | 2 | S+S | 110k | 121k |  |
| 2.6 `ReplaceSearch` and hash helper replace the 4 raw History sites | 3 | S+M+XS | 205k | 226k |  |
| 2.7 `LazyBoundary` replaces `next/dynamic` (stale-chunk safety, controlled-input-safe dirty tracking, telemetry counter) | 5 | S+S+S+S+XS | 240k | 264k |  |
| 2.8 `web-vitals` package replaces `next/dist/client/web-vitals` | 2 | XS+S | 75k | 83k |  |
| 2.9 Lint ban (warn then error), `NavLink` collapse, stories on `impl.memory`, facade guard rails for concurrent PRs | 5 | XS+S+S+XS+S | 205k | 226k | One merge cycle between 2b and 2e |
| 3.0 `build` Jest project, `runInBun`, `.gitignore` negation, `.bun-version`, tsconfig exclude (Bun-side test runner), no-live-port and no-`bun test` scans | 5 | M+S+S+S+S | 350k | 385k | PR #1100 merged (needs pinned `bun`) |
| 3.1 Direct, exact-pinned vanilla-extract dependencies | 1 | S | 55k | 61k |  |
| 3.2 Author and commit `patches/eval@0.1.8.patch` | 2 | S+M | 185k | 204k | PR #1100 layout known |
| 3.3 Shared `node_modules` hash covers lockfile and patches | 2 | XS+S | 75k | 83k | Lockfile location resolved |
| 3.4 `PatchCanary` (runs in Bun, with a negative test) and `verify-eval-patch` | 3 | S+S+S | 165k | 182k |  |
| 3.5 Upstream tracking | 1 | XS | 20k | 22k | Upstream response (non-blocking) |
| 4.1 Pin `react-router@7.18.4` and add a supply-chain gate | 2 | S+S | 110k | 121k |  |
| 4.2 `impl.router.tsx`, the React Router implementation of the contract, and the `entry.tsx` router entry | 4 | M+M+S+S | 370k | 407k |  |
| 4.3 `RouteTable` (42 routes), `RoutePending`, guards, redirects, not-found view | 4 | M+M+S+S | 370k | 407k |  |
| 4.4 `main.tsx`: pathless shell layout route with errorElement inside the shell | 3 | M+S+S | 240k | 264k |  |
| 4.5 `RouteAnnouncer`, `FocusPolicy` and skip-link safety (edge cases) | 3 | S+S+S | 165k | 182k |  |
| 4.6 Scroll policy and History interplay | 2 | S+S | 110k | 121k |  |
| 4.7 `NavContract` suite across implementations | 1 | S | 55k | 61k |  |
| 5.1 `VePlugin` with content-hash invalidation and full-reload fallback | 3 | M+M+S | 315k | 347k |  |
| 5.2 Build-seam resolver plugin | 2 | S+S | 110k | 121k |  |
| 5.3 HTML shell, viewport, fonts, blocking theme CSS link | 2 | S+S | 110k | 121k |  |
| 5.4 `ThemeMap` and `ThemeBoot` from the same VE cache and `ThemeBootScript` | 3 | S+S+S | 165k | 182k |  |
| 5.5 `build/build.ts`, the stage pipeline (naming.entry, stage-named errors, explicit `NODE_ENV`, patch guard stage, optional offline fixture); stages split over Tasks 5.5b, 5.5e, 5.5f | 6 | M+S+S+S+S+S | 405k | 446k |  |
| 5.6 Dev server (overlay, port-in-use, bind) | 4 | S+M+S+S | 295k | 325k |  |
| 5.7 Makefile seam, script inventory, tool pins, both tsconfigs type-check both impls, the single `parseWebBuildMode` | 5 | S+S+XS+S+XS | 205k | 226k |  |
| 5.8 Dev stack and Playwright dev-mode harness | 3 | S+S+XS | 130k | 143k |  |
| 5.9 `verify-output.ts`, `public-allowlist.json`, `budgets.json` (Next-relative ceilings) and `BuildStats` | 3 | M+M+S | 315k | 347k |  |
| 5.10 Differential CSS gates in CI (oracle, census, rule-order census) | 3 | S+S+S | 165k | 182k |  |
| 5.11 CI jobs `web-build-smoke-next` and `web-build-smoke-bun`, aggregate `web-build-gate` with variable-driven membership and a fixture-tested script, workflow-edit protocol, `actionlint`, PR-event proof on a scratch repo | 4 | S+S+S+S | 220k | 242k | CI queue; owner's first-time branch-protection setup naming the one required check `web-build-gate` |
| 6.1 Inventory and replace `next/core-web-vitals` | 2 | S+S | 110k | 121k |  |
| 6.2 Flat config conversion (classified-diff tolerance, time-boxed) | 2 | S+M | 185k | 204k |  |
| 6.3 Warning gate semantics without `next lint` | 2 | S+S | 110k | 121k | CI run |
| 6.4 `size-limit` and budgets per ADR-005 | 2 | XS+S | 75k | 83k |  |
| 6.5 Remaining CI workflows | 4 | S+S+S+XS | 185k | 204k | CI run |
| 6.6 Storybook per ADR-002 | 2 | S+XS | 75k | 83k |  |
| 6.7 Docs, CLAUDE.md and ADR amendments, docs-grep test | 2 | S+XS | 75k | 83k |  |
| 6.8 Go walk of every `out/` file against the auth exemption | 3 | S+S+XS | 130k | 143k |  |
| 6.9 jscpd, feature registry, scanner and ignore files | 1 | S | 55k | 61k |  |
| 6.10 Install and supply-chain hardening verification | 1 | S | 55k | 61k | CI run |
| 7.1 `route-titles.spec.ts` (auto-retrying `toHaveTitle`) | 2 | S+S | 110k | 121k |  |
| 7.2 `route-focus-and-announce.spec.ts`, `aria-current`, edge cases (7.2b Bun-only), stale-chunk omnibar spec (7.2c Bun-only) | 3 | S+S+S | 165k | 182k |  |
| 7.3 `history-behaviour.spec.ts` | 1 | M | 130k | 143k |  |
| 7.4 `deep-link-trailing-slash.spec.ts`, `?window=`, `/login` navigation | 2 | S+S | 110k | 121k |  |
| 7.5 `theme-fouc.spec.ts`, blocking CSS link, storage-throws (7.5b Bun-only) | 2 | M+S | 185k | 204k |  |
| 7.6 `viewport-meta.spec.ts` and `<html lang>` | 1 | XS | 20k | 22k |  |
| 7.7 `not-found.spec.ts` (7.7b Bun-only content criteria) | 1 | S | 55k | 61k |  |
| 7.8 Axe sweep gaps (incl. WCAG 2.2 AA tags) and Lighthouse routes (bounded); WebKit mobile checks (report-only) | 2 | S+S | 110k | 121k |  |
| 7.9 `ParityDiff` harness: 6 themes, resolved properties, reasoned allow-list, 42 routes cold, navigation-sequence walk | 2 | M+M | 260k | 286k |  |
| 7.10 Run e2e in both modes (all PR 7b) as new `e2e-next` and `e2e-bun` jobs in `build.yml`; the Bun job reports and joins `web-build-gate` through `WEB_BUILD_GATE_MEMBERS` after 10 green runs on `main`; `e2e-baseline-compare.ts` with fail-case tests; route coverage manifest | 5 | S+S+M+S+S | 350k | 385k | CI queue; 10 consecutive green `main` runs (or owner flip) before the Bun job can block |
| 7.11 Triage of Next-quirk specs against the `E2eBaseline` | 3 | S+M+XS | 205k | 226k |  |
| 7.12 `scroll-policy.spec.ts` | 1 | S | 55k | 61k |  |
| 7.13 `login-static-assets.spec.ts`, unauthenticated `/login` request log | 2 | S+S | 110k | 121k |  |
| 7.14 Generated route smoke spec over all 42 deep links (PR 7a; formerly Task 7.10d) | 1 | S | 55k | 61k |  |
| 8.1 Pre-cutover gate run on a ManualInstance (baseline compare, login assets) | 3 | M+S+XS | 205k | 226k | Owner-run ManualInstance session |
| 8.2 Flip defaults and record gate membership | 2 | XS+XS | 40k | 44k | Owner approves flip; planned service restart |
| 8.3 Measured before/after table, 5x measurement script, results-doc completeness test | 3 | S+S+XS | 130k | 143k |  |
| 8.4 Soak runbook, exit criteria, default lengths, PR 9 deadline | 2 | XS+XS | 40k | 44k | Soak windows (owner instance): >= 14 days + 3 sessions, then >= 7 days |
| 8.5 Restart-safety precondition (enforced by an `install-service` preflight), rollback runbook with a tested `gh api` command, rollback rehearsal | 5 | S+S+S+S+S | 275k | 303k | Owner confirms deployed unit flags (runs before 8.2) |
| 8.6 Enable minification (PR 8b): minified full-suite gate and flip | 2 | XS+M | 150k | 165k | First Soak window passed; second Soak window follows |
| 9.1 Delete Next application code and aliases | 3 | M+S+S | 240k | 264k | Soak exit |
| 9.2 Delete Next dependencies, scripts and CI/Makefile branches, no-`next`-dependency test | 3 | S+M+XS | 205k | 226k | Soak exit |
| 9.3 Final docs grep (incl. `tests/`), `WEB_BUILD` residue removal, ADR status test and closure | 3 | S+S+XS | 130k | 143k | Soak exit |
| 9.4 Optional follow-up PR, rename the asset prefix (ADR-003) | 2 | S+S | 110k | 121k | Soak exit + owner choice |

| Phase (PR) | Stories | Tasks | Raw tokens | CU before verification |
|---|---|---|---|---|
| 0 | 7 | 28 | 1,890k | 2,079k |
| 1 | 7 | 14 | 810k | 891k |
| 2 (2a-2e) | 9 | 29 | 1,645k | 1,810k |
| 3 | 6 | 14 | 850k | 935k |
| 4 | 7 | 19 | 1,420k | 1,562k |
| 5 (5a, 5b) | 11 | 38 | 2,435k | 2,679k |
| 6 (6a, 6b) | 10 | 21 | 1,055k | 1,161k |
| 7 (7a, 7b) | 14 | 28 | 1,920k | 2,112k |
| 8 (8, 8b) | 6 | 17 | 840k | 924k |
| 9 | 4 | 11 | 685k | 754k |

**Totals (base plan, happy path)**: 81 stories, 219 tasks, 13.550M raw tokens, **14.91M CU before verification** (14.905M); with the verification multiplier x1.75 (range x1.5-2: per-story spec and quality review, `6-verify` layers, CI reruns): **26.08M CU (range 22.4-29.8M)**. Size band: **XL (> 15M)**, as before; requirements.md says XL with the same figures. The first repair added 7 stories and 34 tasks (+2.14M raw, +2.35M CU before verification, +4.1M CU verified) over the reviewed 72/144/18.1M plan; the iteration-2 repair added 3 tasks and enlarged Tasks 0.2b and 7.10 (+0.245M raw); the iteration-3 repair (Phase 4) added 1 story (8.6) and 27 tasks (+1.395M raw, +1.53M CU before verification, +2.68M CU verified; 24 of the 27 tasks are XS or S) and phase rows were recomputed from raw; the triad-gap folding added 1 story (7.14, a move of Task 7.10d) and 8 net tasks (+0.22M raw, +0.242M CU before verification, +0.42M CU verified): Tasks 0.1e, 0.1f, 0.4f, 0.5d, 0.6c, 5.5e, 5.5f, 7.8b, with Task 0.1c re-classed M to S and Task 5.5b M to S. The iteration-4 repair (Phase 4 re-review) added 3 S tasks and no story: Tasks 2.2d (`ast-grep`/`rg` provisioning and the `test:codemods` script), 5.11d (PR-event proof on a scratch repo) and 7.10f (the new `e2e-next`/`e2e-bun` jobs), +165k raw, +181.5k CU before verification, +0.318M CU verified; the other iteration-4 changes were folded into existing tasks without re-classing. Planning overhead already spent is not included. Informational only, not a scope gate.

**Estimate caveat (triad review).** The factors are flat across row types: x1.1 CU per raw token and x1.75 verification on every row. Spike rows (all of Phase 0: 1.89M raw) and docs-only rows (Stories 6.7, 8.4: 0.115M raw) carry no `6-verify` code-review layers and no CI repair waves, so x1.75 over-states them; they are left at the flat factor so the headline stays conservative. Sensitivity (INFERRED): at x1.25 on those rows the verified total falls by (1,890k + 115k) x 1.1 x 0.5 = about 1.10M CU, to about 25.0M. Recalibrate both factors after the first implementer run, per the basis note below.

**Contingency lines (named, outside the base estimate; paths that only run if a gate fires or a gap is closed)**

| ID | Contingency | Trigger | Tasks (classes) | Raw | CU before verification | CU x1.75 |
|---|---|---|---|---|---|---|
| C1 | S0 failure path: no dual build. Epics 8 and 9 collapse into one cutover PR; Next checks move to a pnpm control worktree; adds a revert plan and a pre-merge parity run | S0 FAIL (ADR-006) | M+M+S | 315k | 347k | 606k |
| C2 | Vite 8 + official VE plugin fallback: Vite config, HTML/theme boot, dev server, `verify-output`/stats adaptation, CI job, oracle/census adaptation. Gross new work only: the avoided Epic 3 (3.1-3.5) and Stories 5.1, 5.2, 5.6 are credited back but not netted here (sunk spend on already-merged stories stays) | T1-T6 (ADR-007) | M+S+S+S+S+S+M | 535k | 589k | 1,030k |
| C3 | macOS evidence: S1 census, `bun patch` behaviour and a Bun build on a macOS host, recorded in the `GateRecord` (otherwise "unverified on macOS" is stated, Story 0.6) | Owner wants macOS verified before cutover | S+S | 110k | 121k | 212k |

Worst case (C1, C2 and C3 all fire; C1 and C2 are not mutually exclusive): +1,056k CU before verification, +1,848k CU verified. **Base plus all contingencies: 15.96M CU before verification (14.905M + 1.056M = 15.961M), 27.93M CU verified (26.0838M + 1.848M = 27.9318M).** (Earlier bases: 12.682M / 22.19M after iteration 1, 12.947M / 22.657M after iteration 2, 14.4815M / 25.3426M after iteration 3, 14.7235M / 25.7661M after the triad folding.)

**Critical path in agent waves** (a wave = one fan-out of parallel worker runs, each followed by one verify/repair wave): Epic 0: 4 (0.7 first; S0+S1 next; shared prototype entry; S2 || S3 || S4; record). Epics 1 and 7a run in parallel with Epic 0 where they cannot be invalidated by it: 3 and 2 waves. Epic 2: 4 (2c || 2d first; then 2.1; then 2.2 || 2.3 || 2.4 || 2.5; then 2.6 and 2.9a-c; 2e one merge cycle later). Epic 3: 3 (3.0 first; overlaps Epic 2; serialises on `package.json`). Epic 4: 5 (4.1, 4.2, then 4.3 || 4.5 || 4.6, 4.4, 4.7). Epic 5: 5 (5.1 || 5.2 || 5.3 || 5.6, then 5.4, 5.5, then 5.7 || 5.8 || 5.9, then 5.10 || 5.11). Epic 6: 6a chain of 3 (starts after 5a merges because of the Makefile chain; no behavioural dependency), 6b 2 waves. Epic 7b: 2. Epic 8: 4 (8.5 first, 8.1 || 8.3, 8.2, then 8b after the first Soak window). Epic 9: 3. About 34 build waves end to end before the Soak. Shared-file serialisation points: `Makefile`, `.github/workflows/build.yml`, `web-app/package.json`, `tsconfig.json`, `web-app/jest.config.js` (see Dependency Visualization).

**Overrun checkpoint**: after each phase merges, compare actual spend (price-weighted, using the same weights) with the phase row above multiplied by 1.75. If cumulative spend on completed phases is more than 25% over the table value, pause and report the delta and its cause (wrong estimate vs real surprise). **Abort criteria (pre-mortem failure mode 9)**: if cumulative spend after Phase 5 exceeds 1.5x the table's cumulative value, the owner chooses in writing between (a) continue, (b) switch to the Vite 8 fallback (ADR-007, contingency C2), (c) stop and keep the independently valuable PRs (Epics 1, 2c, 2d, which are worth keeping under Next, and 7a). The report names which spend is real surprise and which is estimate error. Beyond the abort criterion this is informational, not an invitation to cut.

**Wall-clock blockers (the only calendar time), with owners**
- PR #1100 (pnpm to Bun) must merge first: blocks Story 0.1 and everything after Epic 0 (owner: Tyler / the session holding #1100). If #1100's head changes shape before it merges, `spikes/s0-rerun.sh` (Task 0.1d) re-runs S0 on the merged `main` before PR 3 and the `GateRecord` notes the commit. Stories 3.0, 3.2, 3.3, 4.1 and 5.11 additionally wait for the rebase on it.
- Machine time for the three full Playwright runs on Next (Task 0.1c): runs unattended on a ManualInstance. The same applies to the local first Bun run of the suite before PR 7b opens (Task 7.11c).
- Owner sign-off on the gate record and on flagged choices F1-F7 and the budget relaxation, if any (Story 0.6): owner: Tyler.
- Owner call on an amber S2 result (live reload without state-preserving Fast Refresh): owner: Tyler.
- CI-only verification of workflow edits (Stories 5.11, 6.3, 6.5, 6.10, 7.10, 8.2): a green `workflow_dispatch` run on a throwaway branch per PR (decision 10), plus the one PR-event proof on a scratch repository with protection enabled (Task 5.11d), because a dispatch run cannot show a required check going unreported; workflows cannot run locally.
- 10 consecutive green runs of a Bun job (`web-build-smoke-bun`, `e2e-bun`) on `main` before its name is added to `WEB_BUILD_GATE_MEMBERS` (or an explicit owner flip recorded in `cutover-gate-results.md`): CI calendar time, not agent time.
- Branch protection is a first-time setup, not an edit: `main` has none today (`gh api .../branches/main/protection/required_status_checks` returns 404, rulesets empty, 2026-10-19). One time, when PR 5a merges and after the scratch-repo proof of Task 5.11d: the owner requires `web-build-gate` as the single web-build check. Nothing is renamed at the flip and nothing is removed on a rollback, because gate membership is the repository variable `WEB_BUILD_GATE_MEMBERS` (decision 10). Owner: Tyler; settings are not in git.
- Confirming the deployed unit passes `--tmux-keep-server` (Story 8.5), then an owner-run ManualInstance session for the pre-cutover run and the planned service restart for the flip (8.1, 8.2): owner: Tyler.
- Soak windows (8.4): at least 14 calendar days and 3 distinct dev sessions after PR 8, then at least 7 days after PR 8b; PR 9 is scheduled on a dated reminder and the owner decides in writing between finishing and reverting to Next if PR 9 has not merged 45 days after PR 8 (stop-loss).
- Upstream response on vanilla-extract#1664 (Story 3.5): not blocking.

_Basis: bands and weights INFERRED (ESTIMATION.md); per-task token classes XS 20k / S 55k / M 130k; CU = 1.1 x raw assumes 12% output share with about half of input cached. Recalibrate after the first implementer run._

---

## Unresolved Questions
- [ ] Where does `bun.lock` live after PR #1100 (workspace root vs `web-app/`)? `patchedDependencies` itself goes in the **root** `package.json` in every layout: Bun reads it from the root manifest only and warns on a workspace member that declares it (per `research/stack.md` line 77, citing the Bun docs; the spike wrote it to the root manifest; UNVERIFIED on the merged layout until Story 3.2 runs `bun patch --commit` there) — blocks Story 3.3's lockfile re-key — owner: PR #1100 author / Story 3.2 implementer reads the merged PR.
- [ ] Does Next 15.3.2 build, Jest 30 and Storybook run on a Bun-installed tree? — blocks the dual-build design (Epic 5 onward) — owner: Story 0.1 (S0).
- [ ] Does the `feature-flag` guard `RequireFeatureFlag` render `null` while loading and redirect to `/` when off, like the two bespoke layouts? Flag name of `settings/backlog-sources/layout.tsx` was not read — blocks Story 4.3 — owner: Story 1.4 (characterisation) answers it.
- [ ] Does Next's client `<Link>` navigation append a trailing slash to `usePathname()` under `trailingSlash:true`? — blocks the `NormalisedPathname` rule in Story 2.1 — owner: Story 1.3.
- [ ] Is `window` or `<main>` the scroll container; is `ScrollRestoration` usable? — blocks Story 4.6 — owner: S4.
- [ ] Does any external client (mobile app, notifications, Slack) rely on trailing-slash URLs or `_next/` paths? No mobile source exists in this repo; server-built links have no trailing slash (`research/features.md` §2) — blocks D1 sign-off, not implementation — owner: owner.
- [ ] Does `tools/scanner/frontend/src` key on `page.tsx` paths? (`research/features.md` §4 says UNVERIFIED) — blocks Story 6.9 — owner: Story 6.9 reads it.
- [ ] ESLint format: convert to flat config (Story 6.2) or stay on eslintrc under ESLint 9 until later. Default: convert; depends on the `file:` plugins loading under flat config (S0) — owner: Story 6.1.
- [ ] Rebase gate: Stories 3.0, 3.2, 3.3, 4.1 and 5.11 must not start until the branch is rebased on the merged PR #1100 and the `bun.lock` / `patchedDependencies` location is known (Story 3.3 depends on this answer) — owner: PR #1100 author.
- [ ] Bun version pin: 1.4.3 is in the #45730 range; S1 decides between 1.4.3 and 1.3.11 (or later fix) — blocks Story 5.7 — owner: S1.
- [ ] Confirmation of flagged choices F1-F7 and secondary decisions D1-D7 — each blocks only the story named in its row — owner: Tyler.
- [ ] Does `ci-duration-trend.yml` accept extra series? — Story 5.9 falls back to a step summary if not — owner: Story 5.9.
- [ ] Re-read of TanStack's companion hardening post — only if F1 flips to TanStack — owner: Tyler.

## Dependency Visualization

```
 PR 0   Epic 0  0.7 (commit spike) ► S0 S1 S2 S3 S4 ──► 0.6 GateRecord ──(owner sign-off)──────────────┐
          │ (E2eBaseline from 0.1c is the cutover reference)                                           │
          ├────────────► PR 1  RootShell, RouteMeta, FeatureGuard, characterisation, Go tests          │
          │                │                                                                           │
          │                ├──► PR 7a  Next-baselined specs + route smoke (7.14) + triage (before PR 2a) │
          │                ├──► PR 2c  LazyBoundary  ──┐        (independent of the facade)            │
          │                ├──► PR 2d  web-vitals    ──┤                                               │
          │                ▼                           ▼                                               │
          │             PR 2a  Facade core + test mocks (needs 7a) ─► PR 2b  prod migration, ban=warn   │
          │                                                              │ (one merge cycle)           │
          │                                                              ▼                            │
          │                                                           PR 2e  ban=error                 │
          │             PR 3  Story 3.0 (.gitignore, .bun-version, runner) ► eval patch ► canary       │
          │                │                          │                                                │
          │                └──────────┬───────────────┘ (needs 2b for 4)                               │
          │                           ▼                                                                │
          │                        PR 4  Router impl + entry (needs S4, 2b, 3.0)                       │
          │                           ▼                                                                │
          │                        PR 5a  Bun build, Makefile seam, verify-output, CI smoke-next/-bun jobs + web-build-gate │
          │                          │    │     │                                                      │
          │                          │    │     └──► PR 6b  size-limit, CI, Storybook, docs, Go walk   │
          │                          │    └────────► PR 7b  Bun-only specs, e2e-next/e2e-bun jobs (Bun reports; joins gate via variable after 10 green) │
          │                          └─────────────► PR 5b  dev server, dev-stack                      │
          │       PR 6a  ESLint (after 2e and 5a (Makefile chain); gates PR 9, not PR 8; if stalled, 6.3b rebases)│
          └──────────────────────────────────────────────────┴──► PR 8  Cutover, unminified (needs 5a 5b 6b 7a 7b; Story 8.5 first)
                                                                       │ Soak 1 (>= 14 days, 3 sessions) → PR 8b enables minification (one-line revert) → Soak 2 (>= 7 days); stop-loss 45 days after PR 8
                                                                       ▼
                                                              PR 9  Remove Next (needs 8b + Soak 2 + 6a); optional asset-prefix rename

 Intra-epic hot spots (serialise on shared files):
   Makefile: 5.7 → 6.3 → 8.5d → 8.2 → 8.6 → 9.2 (5.7 is PR 5a, 6.3 is PR 6a: hence 6a merges after 5a; 6a vs PR 8 is a preferred order only, 6.3b owns the rebase if PR 8 goes first)        .github/workflows/build.yml: 5.11 → 7.10 → 6.5 → 8.2 → 9.2
   web-app/package.json: 2.8 → 3.1 → 4.1 → 6.1 → 6.4 → 9.1     tsconfig.json: 5.7 → 9.1
   web-app/jest.config.js: 3.0 → 2.2 (global stub) is independent; 3.0 is the only owner of the `build` project
   .gitignore: 3.0 (`!/web-app/build/`) → 5.7 → 6.9      public-allowlist.json: created 5.9 (PR 5a) → consumed 6.8 (6b), 7.13b (7b)
   scripts/e2e-baseline-compare.ts: created 7.10 (7b) → used by 8.1
 Parallel fan-out inside a wave: 2.3 ∥ 2.4 ∥ 2.5 ; 5.1 ∥ 5.3 ∥ 5.6 ; 6.1 ∥ 6.4 ∥ 6.6 ∥ 6.8 ; 7.1…7.9 ; 7.11 before 7.10
```

---
## Phase 0: Spike gates (PR 0)

Spike code is throwaway and lives under `project_plans/web-app-bun-build/spikes/` (scripts only; no change to `web-app/`). **Story 0.7 runs first**: the earlier proof-of-concept (`bench/bun-spike/` with `ve-plugin.ts`, the shims, `build2.ts`) exists only in a session scratchpad, so it is committed before any story ports from it. Results are committed to `research/spike-results.md` (the `GateRecord`). All browser or server runs use a `ManualInstance` (ports 62871/62872, `STAPLER_SQUAD_INSTANCE=claude-manual-test`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`, binary under `~/.stapler-squad/manual-builds/manual-1/`); never `:8543`, never `make install-service`.

### Epic 0.1: Remove the UNVERIFIED items that can invalidate the plan
**Goal**: Numeric pass/fail answers for S0-S4 and a signed gate decision before any product code changes.

#### Story 0.1: Gate S0, Next/Jest/ESLint/Storybook on a Bun-installed tree
**As the** owner, **I want** to know whether the Next build still works after PR #1100, **so that** the dual-build period is real, not assumed.
**Acceptance Criteria**:
- Next production-path build works on a Bun-installed tree.
  - *Given* a clean worktree at the PR #1100 head with `bun install --frozen-lockfile` done, *When* the existing `make web-build` (development-mode Next build) runs, *Then* it exits 0 and `web-app/out` JS bytes are within 2% of 21,666,571 B.
- Jest has no new failures.
  - *Given* the same tree and a pnpm-installed control worktree of the same commit, *When* `npx jest --no-coverage` runs in both, *Then* the failing-file lists are identical (recorded with counts, baseline 620 files / 7,683 tests).
- Required lifecycle scripts are known.
  - *Given* `trustedDependencies` empty, *When* a VE compile of `src/styles/theme.css.ts` and `storybook build` and `eslint src/lib/nav` run, *Then* the minimal set of packages that must be trusted (candidates `esbuild`, `@vanilla-extract/css`, `@swc/core`) is listed with the failing symptom for each omission.
- Playwright baseline on Next exists (the cutover reference, B2).
  - *Given* a Next build embedded into a ManualInstance binary (`WEB_BUILD` unset, Next default), *When* the full suite `cd tests/e2e && npm test` (152 specs, count derived by `ls tests/e2e/*.spec.ts`) runs 3 times with the Playwright JSON reporter, *Then* `tests/e2e/baseline/next-e2e-baseline.json` is committed with, per spec file and test title, **every attempt of every run** (`runs[i].attempts[k] = {retry, status}` from the reporter's `results[].retry` and `results[].status`, not only the final status: `playwright.config.ts` has `retries: 1`, which would otherwise turn a first-fail-then-pass into a plain `pass`), and a classification: `pass` = passed on attempt 0 in all 3 runs; `flaky` = any run needed a retry, or final outcomes differ across runs; `fail` = final status failed in all 3 runs. The file also holds the Next and Playwright versions, the commit SHA and the run dates, and per spec a `routesVisited` list (pathnames navigated to, recorded by a fixture hook in the baseline run only; the input of the coverage manifest in Task 7.10e); `tests/e2e/baseline/README.md` is not created (the JSON has a `meta` key explaining itself).
  - *Gate rule that consumes it* (implemented and unit-tested in Story 7.10, Task 7.10c; defined here so the baseline carries the data it needs): a spec the baseline classes `pass` (attempt 0 in all 3 Next runs) **fails** on Bun when it fails or needs a retry in **at least 1 of 3** Bun runs, unless the `e2e-next` job of the same run also shows that failure in at least 1 of its own runs (then it is a shared flake, reported and judged as the Next job is); a spec the baseline classes `flaky` fails on Bun when its final status is failed in at least 2 of 3 runs (1 of 3 is a **warning** printed with the failure text and listed in the step summary, dispositioned in Story 8.1 triage) and must pass in at least 1 of 3 Bun runs (compared by pass-rate, so it is not wholly masked; pre-mortem failure mode 6: a timing regression in a focus or announcer spec must not hide behind a 2-of-3 rule); a spec the baseline classes `fail` is excluded from the gate (accepted masking, stated here) but reported when it passes on Bun; a spec that is `pass` on Next and fails on Bun is a gate failure unless listed in `bun-expected-diff.json`. Specs or titles absent from the baseline (the baseline is a snapshot at PR 0; concurrent PRs add and rename specs) are judged against the **`e2e-next` job of the same run**: the comparator takes that job's Playwright JSON report (`--next-results`, uploaded as an artifact by `e2e-next` and downloaded by `e2e-bun`, both in `build.yml`) and a spec failing on Bun but passing there is a gate failure; failing on both is treated as a baseline-class `fail` and reported; if the Next report is missing the comparator fails (never skips).
- Next-build quirks are inventoried.
  - *Given* the baseline run, *When* the specs are grepped for build-mode coupling (`reactStrictMode`, `__next-route-announcer__`, `NEXT_BUILD_MODE`, dev-mode comments, e.g. `tests/e2e/insights-findings-panel.spec.ts:88-95`: the comment at 88-92 says the dev-mode Next build double-invokes the mount effect, and the error response is queued twice at lines 94-95; `jules-status-badge.spec.ts:47`; the `#__next-route-announcer__` exclusions in 5 specs), *Then* the list is recorded in `next-e2e-baseline.json` under `buildCoupledSpecs` for Story 7.11.
**Files**: `project_plans/web-app-bun-build/spikes/s0.sh` (new), `project_plans/web-app-bun-build/spikes/s0-rerun.sh` (new), `project_plans/web-app-bun-build/research/spike-results.md` (new), `tests/e2e/baseline/next-e2e-baseline.json` (new)

##### Task 0.1a: Control run on pnpm and Bun trees (M)
- Create two worktrees, run build, Jest, `storybook build`, `eslint`; store outputs in the scratchpad; diff failure lists.
- Files: `spikes/s0.sh`
##### Task 0.1b: Lifecycle-script and `file:` plugin matrix (S)
- Toggle `trustedDependencies`; confirm `eslint-plugin-analytics` and `eslint-plugin-rpc-lifecycle` (`file:` deps) load; record `bun install` linker (hoisted vs isolated) outcome.
- Files: `spikes/s0.sh`, `spike-results.md`
##### Task 0.1c: Playwright baseline x3 on a Next ManualInstance, capturing every attempt (S; machine time)
- Uses the existing `tests/e2e/global-setup.ts` isolation (dynamic free port, PID-scoped temp dir); never `:8543`. Output: three raw Playwright JSON reports (kept in the scratchpad, not committed) with `results[].retry` and `results[].status` per attempt. Split from the former single M task (triad review: it bundled the run, the classifier and the inventory).
##### Task 0.1d: `spikes/s0-rerun.sh`, the one-command S0 re-run (XS)
- Re-runs the numeric S0 checks (Next build bytes within 2%, Jest failing-file list vs the pnpm control, `storybook build`, `eslint` plugin load, VE compile of 3 files) and prints the #1100 commit it ran on. Run on the merged #1100 `main` before PR 3 starts and whenever #1100's head changes before it merges; the commit is written into the `GateRecord` (pre-mortem failure mode 9).
##### Task 0.1e: Classifier, `routesVisited` fixture hook and `next-e2e-baseline.json` (S)
- A script that reads the three reports and writes `tests/e2e/baseline/next-e2e-baseline.json` with `runs[i].attempts[k]`, the pass / flaky / fail classification of this story and the `meta` key; the baseline-run-only fixture hook that records `routesVisited` per spec. Unit-tested on fixture reports (a first-fail-then-pass attempt must classify as `flaky`, not `pass`).
##### Task 0.1f: Inventory the build-coupled specs into `buildCoupledSpecs` (XS)
- The grep of this story's last AC, written under `buildCoupledSpecs` in the baseline JSON for Story 7.11.

#### Story 0.2: Gate S1, HTML-entry `Bun.build`, assets, defines, CSS census
**As the** owner, **I want** the single-HTML entry design proven with the real plugin set, **so that** the build architecture is not discovered during Epic 5.
**Acceptance Criteria**:
- HTML location (adversarial re-review B3, VERIFIED by running Bun 1.4.3: with `entrypoints:["index.html"]` and `naming.entry` under `_next/static/chunks/`, Bun writes the emitted HTML to `out/_next/static/chunks/index-<hash>.html` and there is **no `out/index.html`**; `naming.entry` cannot carry both the JS entry pattern and the HTML location, while the Go handler reads `index.html` at the FS root, `server/middleware/static.go:21-40`). The spike therefore proves one of two layouts and records which: **A'** the `html` stage finds the emitted `.html` in `result.outputs`, post-processes it (theme boot injection) and writes it to `out/index.html`, then deletes the original; or **B** (the stated fallback) the entry is the script (`src/main.tsx`) and `build/html.ts` generates `out/index.html` from a template at build time. Either way the walk below runs **after** that step.
  - *Given* the chosen layout built over the real `src/`, *When* the build finishes, *Then* `out/index.html` exists, no other `*.html` exists under `out/`, and every asset URL in it starts with `/_next/static/`.
- Output layout and deep links.
  - *Given* `Bun.build({entrypoints:["index.html"], splitting:true, minify:true, publicPath:"/", naming:{entry:"_next/static/chunks/[name]-[hash].[ext]", chunk:"_next/static/chunks/[name]-[hash].[ext]", asset:"_next/static/media/[name]-[hash].[ext]"}, plugins:[ve]})` over the real `src/` (`naming.entry` is set explicitly: the earlier spike `build2.ts` set `entry:"[name].[ext]"`, `chunk:"chunks/..."`, `asset:"assets/..."`, none of which is under an exempt prefix, and an unset `entry` leaves the entry bundle's location to a Bun default that a Bun upgrade may change), *When* the output is embedded in a ManualInstance binary and `GET /backlog/board` then every script and stylesheet URL from the returned HTML is requested unauthenticated, *Then* each returns 200 with a `javascript` or `css` content type and none is HTML; *and when* every file under `out/` (including dynamic-import chunks, CSS `url()` fonts, `.map` files and `public/` assets) is checked against `auth.go:95-101`'s `exemptPrefixes` (`/auth/ /login /health /_next/ /favicon`), *Then* each is under an exempt prefix or on a written allowlist of public files the unauthenticated `/login` page is shown not to request (decided empirically from the request log of that page).
- Unauthenticated `/login` asset check is achievable (feeds Story 7.13; adversarial re-review B3 feasibility).
  - *Given* remote auth exists only on the remote TLS listener (`main.go` ~1528 `server.EnsureNetworkTLSCerts`, ~1637 `middleware.Auth(sessions, ...)` inside `startRemoteAccess`; the local listener's token validator at `local_auth.go:53` and `middleware.Auth` returning `next` when `AuthRequires` is false, `auth.go:60-66`) and `tests/e2e` contains no remote-access or `ignoreHTTPSErrors` code today, *When* the ManualInstance is started with `--remote-access --remote-port 62872` (CA/hostname as the live instance's private CA flow requires), *Then* Task 0.2b records, with the commands run: (1) whether the remote listener starts under the ManualInstance env, (2) a Playwright context with `ignoreHTTPSErrors: true` (or the CA trusted) loads `/login` unauthenticated, (3) the recorded request list. If any step is infeasible in the e2e harness, the **recorded fallback is a Go test** (`server/middleware/login_assets_test.go`) that serves the real built `out/` (`middleware.StaticHandler` over `os.DirFS("web-app/out")`) wrapped in `middleware.Auth` with an always-reject validator via `httptest`, fetches `/login`, parses the HTML for script/link/preload URLs (plus CSS `url()` references transitively) and asserts each returns 200 with a non-HTML content type for a JS/CSS/font URL; Story 7.13 then uses that Go test in place of the Playwright server fixture. The decision (Playwright path or Go fallback) is written to the GateRecord.
- Fonts.
  - *Given* the 8 vendored woff2 files (194,364 B), *When* the build runs, *Then* `out/` contains them under `_next/static/media/` and no `data:font/woff2` string appears in the CSS.
- Defines and strictness.
  - *Given* `NEXT_PUBLIC_API_URL` unset and the other 4 `NEXT_PUBLIC_*` set to sentinels, *When* the bundle is grepped, *Then* it contains no `process.env` text, and a page load logs no `process is not defined`; `process.env.NODE_ENV` is `"production"`. *And* importing `next/navigation` fails the build, *and* an invalid CSS rule fails it with `throw:true`.
- CSS census (#45730).
  - *Given* the Next production CSS and the Bun CSS for Bun 1.4.3 and 1.3.11, *When* rules and custom properties are counted for each of the 6 theme classes (`matrix`, `cyberpunk77`, `wh40k`, `clean`, `light`, `dark`), *Then* the resolved custom-property maps (name -> computed value, read in a browser) are equal on at least one pinnable Bun version (the passing version is recorded as the pin); rule counts are reported, and a count difference fails only if it is not explained by rule merging.
- Theme placeholder survives minification.
  - *Given* `index.html` containing the candidate placeholder forms `<!--THEME_BOOT-->`, `<script id="theme-boot"></script>` and `<meta name="theme-boot">`, *When* built with `minify:true`, *Then* the emitted HTML still contains each form that survives, and the first surviving form (in that order) is recorded as the placeholder Story 5.4 must use.
**Files**: `spikes/s1/build.ts`, `spikes/s1/census.ts` (new), `spike-results.md`

##### Task 0.2a: Spike build script over real `src/` with plugin and defines (M)
- Port `bench/bun-spike/ve-plugin.ts` and the eval shim via a local `bun patch`; test with `STAPLER_SQUAD_SHARED_NODE_MODULES=0`.
##### Task 0.2b: Serve through the real Go binary, fetch deep link assets, prove the remote-auth `/login` check is achievable (M)
- Build binary into `manual-builds/manual-1`, copy spike output into `server/web/dist` of a scratch checkout (not the worktree), curl nested paths.
- Start it with `--remote-access --remote-port 62872`; drive `/login` from Playwright with TLS errors ignored or the CA trusted; record the request list; if not workable, write the Go-test fallback described in this story's AC and record the choice for Story 7.13.
##### Task 0.2c: CSS census and font strategy (S)
- Script that parses both CSS files with `postcss` (or the Bun CSS parser) and counts rules/custom properties per theme class, then loads both builds in a browser and compares the resolved custom-property map per theme class; run on two Bun versions.
##### Task 0.2d: Walk `out/` against the exempt prefixes (after the HTML move/generation); placeholder-survival check (S)
- Script `spikes/s1/exempt-walk.ts` (becomes the model for Story 6.8) and the three-placeholder minifier check.

#### Story 0.3: Gate S2, the real app in `bun dev` with Fast Refresh
**As the** owner, **I want** the whole app, not one component, rendered and hot-reloaded by the Bun dev server, **so that** the daily loop is genuinely faster.
**Acceptance Criteria**:
- All routes render.
  - *Given* a ManualInstance backend on `PORT=62871` and `bun dev` on `localhost:62875` (a spare port of the CLAUDE.md manual dev block; 62873 is instance #2's `PORT`) with `NEXT_PUBLIC_API_URL=http://localhost:62871`, *When* a Playwright script visits each of the 42 `RoutePath` values, *Then* every page renders its `<h1>` or main landmark and the console has zero errors.
- Fast Refresh keeps state.
  - *Given* the session terminal pane, the omnibar and the session list each holding typed or selected state, *When* a comment-only edit changes the component file, *Then* the visible state is preserved and no full reload occurs (a `window.__marker` survives) on all 3.
- Shared token invalidation.
  - *Given* `/settings` open, *When* a colour value in `src/styles/theme-contract.css.ts` changes, *Then* the computed colour on a dependent element changes within 2 s with no manual reload (if it does not, the content-hash memo of Story 5.1 is recorded as mandatory, and if dependants are still not re-requested the full-reload fallback is recorded as the chosen behaviour).
- Dev errors and memory.
  - *Given* a deliberate TypeScript error and a deliberate render-time throw, *When* each is introduced then removed, *Then* an overlay appears and clears; and after 200 scripted edits RSS is under 1.5 GB with no crash. *And* the `madge` import-cycle count is recorded as informational (no pass line); if a page loses state under Fast Refresh, the record lists that page, the cycles on its import path and the measured time to a full reload, which is the data the owner needs for an amber decision.
**Files**: `spikes/s2/dev.ts`, `spikes/s2/routes.spec.ts`, `spikes/s2/prototype-entry.tsx` (new), `spike-results.md`

##### Task 0.3a: Prototype router entry reusable by S3/S4 (M)
- Minimal `createBrowserRouter` over 42 lazy routes, `RootShell` stub imported from the current providers; kept in `spikes/` only.
##### Task 0.3b: Dev server with `[serve.static]` plugin, `public/` routes, `localhost` bind (S)
##### Task 0.3c: Fast Refresh, token invalidation, overlay and memory scripts; `madge` cycle census (M)

#### Story 0.4: Gate S3, 42 lazy routes: size, CSS ordering, memory
**As the** owner, **I want** the production-equivalent bundle measured, **so that** the "no larger than Next" and "10 s" metrics are known before commitment.
**Acceptance Criteria**:
- Size and splitting.
  - *Given* the S3 prototype built minified, *When* `BuildStats` are computed, *Then* total JS <= 6,349,052 B, at least 42 route chunks exist, and `@xterm`, `monaco-editor`, `codemirror`, `recharts` and `react-markdown` modules are absent from the Initial JS closure of `index.html`.
- Speed and memory.
  - *Given* 3 concurrent builds on this host, *When* run under `/usr/bin/time -v`, *Then* each exits 0 in <= 10 s wall and peak RSS <= 2 GB. The <= 10 s target is a **warm** figure: the median of n >= 5 consecutive runs with build caches present (the VE memo and `web-app/.build`; for Next, `web-app/.next/cache` and the shared webpack cache directory `NEXTJS_SHARED_CACHE_DIR`, `~/.stapler-squad/nextjs-webpack-cache`, set by `Makefile:226`); the **cold** figure (n >= 5, each run after deleting all of those caches, including the shared webpack directory; the OS file cache is not dropped, a stated limitation) is reported alongside, is not a pass line, and a cold median above 10 s is recorded, not failed.
- Baseline timings have a protocol, not a single run (triad review, product; the requirements baseline of 54-88 s and 60.6-115.7 s is n=1 and does not say which runs were cold).
  - *Given* Task 0.4f's script `spikes/s3/baseline-timings.sh`, *When* it runs on this host, *Then* it reports, for each of `make web-build` (Next, development-mode), the production `next build` and `bun web-app/build/build.ts` (prototype), n >= 5 runs per mode per temperature (cold, warm), as median and min-max range, with the host's load average at start; these medians replace the single-run baselines as the denominators of the success metrics and of Story 8.3's 5x ratio. Next's incremental caches make a warm Next build faster than a cold one and so shrink the Bun/Next ratio (review R2-C8d), so for Next the script reports the warm median twice, once as-is (caches present) and once with `web-app/.next/cache` and `NEXTJS_SHARED_CACHE_DIR` cleared before every run, and records which caches existed at the start of each run.
- Dev-loop baseline (the 84 ms figure needs a before number).
  - *Given* `next dev` on a ManualInstance backend (`PORT=62871`, `NEXT_PUBLIC_API_URL`) and a page open, *When* a comment-only edit is made to a component and, separately, a `.css.ts` edit, *Then* Task 0.4f records the time from file write to the DOM update (Playwright `MutationObserver`/computed-style poll, n >= 5, median and range) and the `next dev` cold start to first 200 on `/`; the Bun figures of Story 0.3 are compared with these, not with the spike's lone 84 ms.
- Footprint baseline (the "leaner" goal needs a metric).
  - *Given* the pre-migration tree on the Bun-installed layout (PR #1100), *When* measured, *Then* Task 0.4f records the installed package count (`bun pm ls --all | wc -l` in `web-app/`, UNVERIFIED until run; the exact command used is recorded), `du -sk web-app/node_modules`, direct `dependencies` + `devDependencies` counts from `web-app/package.json`, and `bun install --frozen-lockfile` wall time with an empty cache directory (cold) and with a populated one (warm), n >= 5 each, as the "before" rows of Story 8.3's table.
- CSS ordering.
  - *Given* the Next production build and the Bun build served from two ManualInstances, *When* computed styles of the elements with `data-testid` on `/`, `/settings`, `/backlog`, `/review-queue`, `/history` at 390x844 and 1280x800 are diffed, *Then* the diff is empty or every difference is an allow-list entry carrying a linked reason (an issue/ADR URL or a quoted CSS rule), repeated under each of the 6 theme classes and with forced `:hover` and `:focus-visible` on the first 5 focusable elements of each route (ordering bugs, T4, show on non-default themes and interaction states).
- Route-order dependence (pre-mortem P1-1; INFERRED: Bun orders per-chunk CSS by load order, so a sequence nobody tested can differ).
  - *Given* the Bun build, *When* ParityDiff runs over all 42 `RoutePath` values cold at 1280x800 under `clean` and `dark`, and an in-session walk visits all 42 routes by client navigation (no reload) in 3 fixed orderings (ascending, descending, `RouteMeta` declaration order) and 3 seeded-random orderings (seeds recorded in the result), *Then* for every walk the computed-style snapshot of the final route equals that same route cold-loaded on the same Bun build and equals Next's, and the ordered rule-selector list of the shared (non-lazy) CSS file per theme class equals Next's modulo explained rule merging; any unexplained delta fires T4.
- Next baselines for the budgets (adversarial review).
  - *Given* the Next production build (not the development-mode build used for `out/`), *When* per-route first-load JS is taken from the build output and today's `npx size-limit` (`benchmark.yml:295`, Total JS 5 MB) is run against the build that workflow produces, *Then* the record states each route's Next first-load JS, the largest of them, and whether the 5 MB gate passes today; the Total JS ceiling of 6,349,052 B is thereby labelled "relaxation" or "fix" with numbers, and Story 5.9's Initial JS ceiling is derived from them.
**Files**: `spikes/s3/measure.ts`, `spikes/s3/parity-diff.ts`, `spikes/s3/nav-walk.ts`, `spikes/s3/next-baseline.ts`, `spikes/s3/baseline-timings.sh` (new), `spike-results.md`

##### Task 0.4a: Size, chunk and Initial JS measurement (S)
##### Task 0.4b: Concurrent build memory run (S)
##### Task 0.4c: Computed-style diff harness over 6 themes and interaction states, and the all-42-routes cold run (reused as Story 7.9) (M)
##### Task 0.4d: Next per-route first-load JS and the status of today's 5 MB size-limit gate (S)
##### Task 0.4e: Navigation-sequence walk (3 fixed + 3 seeded-random orderings) and the rule-selector-order census per theme (M)
##### Task 0.4f: Baseline timing protocol: build n >= 5 cold and warm, `next dev` reload and start, footprint (package count, `node_modules` size, install time) (S)

#### Story 0.5: Gate S4, React Router 7.18.4 behaviour spike
**As the** owner, **I want** the router's behaviour pinned on the exact version, **so that** the facade design in Epic 4 rests on measurements.
**Acceptance Criteria**:
- Matching and titles.
  - *Given* the S4 prototype, *When* `/settings/` and `/settings` are loaded, *Then* both render Settings; *and* after a client navigation from `/` to `/history`, `document.title` equals `History - Stapler Squad` via a root-rendered `<title>` (else the `useEffect` fallback is recorded).
- Search params and History.
  - *Given* `/unfinished?tab=a`, *When* `window.history.replaceState(history.state, "", "?tab=b")` runs, *Then* `useSearchParams()` still reports `a` (documented), *and when* the facade-style `navigate("?tab=b",{replace:true})` runs, *then* it reports `b` and `history.length` is unchanged.
- Foreign state.
  - *Given* `history.state` containing `{ssqTray:true}`, *When* an RR `navigate` push and `back()` occur, *Then* the marker survives or the exact loss is recorded for Story 4.6.
- Supply chain.
  - *Given* `npm pack react-router@7.18.4`, *When* `package.json` is read, *Then* it has no `optionalDependencies`, no `github:` URL and no install script, and 7.18.4 is not in the banned list.
- Next's announcer politeness is measured, not assumed (triad review, UX; requirements.md accepted scope addition 3 calls it assertive; that is INFERRED, not read from the built output).
  - *Given* the built Next `out/` (the Task 0.1c build) and a page loaded on a ManualInstance, *When* the script reads the `#__next-route-announcer__` element after a client navigation (its `aria-live`, `role` and the bundled Next client source that creates it, grepped under `out/_next/static/chunks/`), *Then* the observed `aria-live` value and `role` are written to the `GateRecord`; if it is `polite`/`status` rather than assertive, the "assertive" wording in `requirements.md` accepted scope addition 3 and `design/ux.md` S3/UX-08 is corrected in the same PR (the Bun announcer stays polite either way).
**Files**: `spikes/s4/router.spec.ts`, `spikes/s4/next-announcer.ts` (new), `spike-results.md`

##### Task 0.5a: Matching, titles, scroll container, guard parity (S)
##### Task 0.5b: History/search-param/foreign-state matrix (S)
##### Task 0.5c: Tarball inspection and banned-version list (XS)
##### Task 0.5d: Read the built Next announcer's `aria-live` and `role` and record them (XS)

#### Story 0.6: Gate record and owner sign-off
**As the** owner, **I want** one table of measured results against the criteria, **so that** I can accept the plan, or trigger the Vite 8 exit, on evidence.
**Acceptance Criteria**:
- Record complete.
  - *Given* S0-S4 results, *When* `research/spike-results.md` is read, *Then* each gate shows PASS / AMBER / RED, the measured values, the chosen Bun pin, the design (A or B) for HTML generation, and which of T1-T6 (if any) fired, and the PR #1100 commit each S0 result was taken on.
- Platform coverage is explicit.
  - *Given* that S0-S4 ran on this Linux host only, *When* the record is read, *Then* it states "macOS: unverified" for the S1 census and `bun patch` behaviour unless contingency C3 was run, with the reason (Bun CSS-bug exposure and versions can differ per platform).
- Budget relaxation is an owner decision.
  - *Given* Task 0.4d's finding, *When* the Total JS ceiling is 6,349,052 B and today's gate is 5 MB, *Then* the record carries the owner's accept/decline of the relaxation (declining sets the ceiling to the lower number and is a Story 5.9 input).
- Accepted scope additions are signed off.
  - *Given* the "Accepted scope additions" list in `requirements.md` (11 items: not-found view, focus move, polite announcer, `RoutePending`, new-version banner, Go 404 under `/_next/`, flat-config conversion, Lighthouse consolidation, Axe sweep gaps, minification as PR 8b, FID series ending), *When* the owner reviews the record, *Then* each is accepted, declined or bounded further and the reply is quoted; a declined item removes the named story or flips its flag (for example `FOCUS_ON_NAVIGATE=false`).
- Router sign-off is ordered after S4 (triad review, product).
  - *Given* gate S4 has reported, *When* the owner replies in this story, *Then* the reply confirms F1 (React Router 7.18.4) or chooses TanStack at an audited pin; Epic 4 starts only when S4 is PASS and this reply is quoted in the `GateRecord`; Epics 1-3 and PR 7a do not wait for it (see "Owner handoff and sign-off points").
- Value is quantified, not argued from build seconds alone (triad review, product).
  - *Given* Task 0.6c's script `spikes/s0/usage-counts.sh`, *When* it runs, *Then* the record states, with the exact commands and the window used (at least the last 28 days): merged commits on `main` per week (`git log origin/main --since='28 days ago' --oneline | wc -l`), `build.yml` runs per week and their median duration (`gh run list --workflow build.yml --limit 500 --json createdAt,startedAt,updatedAt,conclusion`), and the count of agent worktrees (`git worktree list | wc -l`, as a lower bound on concurrent build users); and states the expected saving in two cost units, never in human time: (1) CI runner-minutes per week saved (runs per week x the median Next build step minus the Bun step) and (2) agent build invocations per week x seconds saved (wall-clock only; token cost is unchanged apart from fewer polling turns, so no CU saving is claimed). Provisional values are in `requirements.md` ("Value and ICE score"); this task replaces them with the dated run.
- Plan reconciled.
  - *Given* a fired trigger or an amber, *When* the record is committed, *Then* `plan.md` carries an "Amendments" note naming the stories replaced or re-scoped, and the owner's reply is quoted.
**Files**: `project_plans/web-app-bun-build/research/spike-results.md`, `project_plans/web-app-bun-build/implementation/plan.md`, `project_plans/web-app-bun-build/spikes/s0/usage-counts.sh` (new)

##### Task 0.6a: Write the record and trigger evaluation (S)
##### Task 0.6b: Plan amendments if any gate is not PASS (S)
##### Task 0.6c: Value quantification: usage counts from git/gh and the expected saving in cost units (S)

#### Story 0.7: Commit the spike and the feasibility doc (runs first)
**As the** owner, **I want** the proof artifacts in the repo, **so that** the plan's evidence and port sources survive a scratchpad cleanup.
**Acceptance Criteria**:
- Spike is in git.
  - *Given* the session scratchpad `bench/bun-spike/` (`ve-plugin.ts`, `shims/`, `entry.tsx`, `build2.ts`, the oracle script), *When* it is copied to `project_plans/web-app-bun-build/spikes/bun-spike/` with a `NOTES.md` stating which naming options it used (`entry:"[name].[ext]"`, `chunk:"chunks/..."`, `asset:"assets/..."`) and that those are not exempt-prefix-safe, *Then* `git ls-files project_plans/web-app-bun-build/spikes/bun-spike | wc -l` is >= 5 and Task 0.2a ports from the committed path.
- Feasibility doc is in git.
  - *Given* the untracked `docs/explanation/bun-migration-feasibility.md`, *When* committed in PR 0 as it stands (with its one-line status note at the top saying `Bun.build` was chosen and the initial Rsbuild recommendation is superseded), *Then* `git ls-files docs/explanation/bun-migration-feasibility.md` lists it, and plan references to it resolve.
**Files**: `project_plans/web-app-bun-build/spikes/bun-spike/*` (new), `docs/explanation/bun-migration-feasibility.md`
##### Task 0.7a: Copy the spike (S)
##### Task 0.7b: Commit the feasibility doc (XS)

---

## Phase 1: Characterise and extract under Next (PR 1)

### Epic 1.1: Pin today's behaviour and extract the shared shell
**Goal**: Behaviour-neutral refactor under Next that removes the two Next-specific leaks (provider tree in `layout.tsx`, titles in `metadata`) and builds the behavioural net for the router swap.

#### Story 1.1: Extract `RootShell` from `app/layout.tsx`
**As a** developer, **I want** the provider and chrome tree in one component, **so that** Next and Bun entries mount the identical tree.
**Acceptance Criteria**:
- Tree is shared and unchanged.
  - *Given* `src/app/layout.tsx` today, *When* the `ViewportProvider > ErrorBoundary > AuthProvider > Providers > CockpitShell > skip-link, <main id="main-content">, TmuxVersionMismatchBanner, ForkPressureStatusBanner, {children}, TrayErrorBoundary > NotificationPanel` tree moves to `src/app/RootShell.tsx`, *Then* a render test of `<RootShell>child</RootShell>` finds the skip link `href="#main-content"`, the `main#main-content` containing `child`, and the notification panel, and `layout.tsx` contains only `<html>`, `<head>`, FOUC, fonts, metadata, `<body><RootShell>`.
- No visible change.
  - *Given* the Next build before and after, *When* `tests/e2e/theme-background.spec.ts` and `tests/e2e/smoke.spec.ts` run, *Then* both pass unchanged.
**Files**: `web-app/src/app/layout.tsx`, `web-app/src/app/RootShell.tsx` (new), `web-app/src/app/RootShell.test.tsx` (new)

##### Task 1.1a: Test first, RootShell render test (S)
##### Task 1.1b: Move tree, keep `Providers.tsx` branches intact (S)

#### Story 1.2: `RouteMeta` table with a drift test
**As a** developer, **I want** page titles and guards in one data table, **so that** the router build and the e2e title specs share one source.
**Acceptance Criteria**:
- Table matches reality.
  - *Given* the 42 `page.tsx` files (count derived from the filesystem) and the 12 title layouts, *When* the drift test parses every `export const metadata`/`generateMetadata` title with the TypeScript compiler API, *Then* `RouteMeta[path].title` equals the effective title (nearest layout or page) for all 42 paths, e.g. `/settings/jules` is `Jules - Settings - Stapler Squad`, `/changelog` inherits `Stapler Squad Sessions`.
- Not-found title is data too.
  - *Given* decision D1, *When* the table is read, *Then* it exports `NOT_FOUND_META` (`title: "Page not found - Stapler Squad"`, default pending owner wording) so Story 4.3c's view takes its title from the table rather than a literal.
- Guards recorded.
  - *Given* `backlog/layout.tsx`, `settings/backlog-sources/layout.tsx`, `changelog/layout.tsx`, *When* the table is read, *Then* those three paths carry `guard` names and the others do not.
- Failing tests say what to do, and guidance ships with PR 1 (pre-mortem failure mode 5).
  - *Given* a `page.tsx` added without a `RouteMeta` row, or a `metadata` title that differs from its row, *When* the drift or key-set test fails, *Then* the message names the fix ("add the row to `web-app/src/app/route-meta.ts`"); *and* PR 1 adds a `CLAUDE.md` note on `RouteMeta` and a glob-scoped `.claude/rules/nav-facade.md` (matching `web-app/src/**`) stating the route-meta rule now and the facade rule from PR 2b (Task 2.9e), so autonomous sessions meet the rule before the lint does.
**Files**: `web-app/src/app/route-meta.ts` (new), `web-app/src/app/route-meta.test.ts` (new), `CLAUDE.md`, `.claude/rules/nav-facade.md` (new)

##### Task 1.2a: Test first, parse-and-compare test (S)
##### Task 1.2b: Generate the table from the test's own parse once, then hand-verify (S)
##### Task 1.2c: Fix-instruction messages, `CLAUDE.md` note and the `.claude/rules/nav-facade.md` stub (S)

#### Story 1.3: Characterise pathname and trailing-slash behaviour
**As a** developer, **I want** the exact `usePathname()` values pinned under Next, **so that** `NormalisedPathname` encodes real behaviour.
**Acceptance Criteria**:
- Probe result recorded.
  - *Given* the Next build on a ManualInstance, *When* a Playwright probe loads `/review-queue`, clicks the nav link to `/history` and back, *Then* it records the `usePathname()` value (via a test hook) and `location.pathname` at each step into `docs/reference/` or the test as a comment (slash or no slash).
- Strict comparisons pinned.
  - *Given* mocked pathnames `/review-queue` and `/review-queue/`, *When* `CockpitShell`, `Navigation`, `Header`, `DrawerNav` and `BottomNav` render, *Then* the active-state outcome for each is asserted for both inputs (these tests will go red for the slash case on purpose until Story 2.1 normalises; they are written as the target behaviour and the slash case is `it.failing` with a link to Story 2.1).
**Files**: `web-app/src/components/layout/__tests__/pathname-characterisation.test.tsx` (new), `tests/e2e/pathname-probe.spec.ts` (new)

##### Task 1.3a: Playwright probe on Next (S)
##### Task 1.3b: Jest active-state matrix (S)

#### Story 1.4: Characterise feature-flag guards and the raw History sites
**As a** developer, **I want** the guard and History semantics asserted before they move, **so that** drift is a red test, not a surprise.
**Acceptance Criteria**:
- Guards.
  - *Given* flags still loading, *When* `backlog/layout.tsx` renders, *Then* it renders `null`; *Given* `flags.backlog === false`, *When* flags finish loading, *Then* `router.replace("/")` is called once. Same for `settings/backlog-sources` (its flag name is recorded here) and `changelog` via `RequireFeatureFlag` (blank-vs-redirect behaviour recorded).
- Raw History sites.
  - *Given* `useUpNextTab` (`replaceState` call at `useUpNextTab.ts:69`) at `?tab=a`, *When* the tab setter runs, *Then* `window.history.replaceState` receives the existing `history.state` and `?tab=b`; *Given* `useTrayDismissal` opening the tray, *Then* `pushState` receives a state containing `ssqTray:true` and a `popstate` closes it; *Given* `SessionDetailView` selecting file `a/b.go`, *Then* the URL hash is `#file=a%2Fb.go` (exact encoding recorded).
**Files**: `web-app/src/app/backlog/__tests__/layout.test.tsx`, `web-app/src/lib/hooks/__tests__/useUpNextTab.history.test.ts`, `web-app/src/lib/hooks/__tests__/useTrayDismissal.history.test.ts` (new; exact neighbours confirmed by `ast-grep`)

##### Task 1.4a: Guard layout tests (S)
##### Task 1.4b: History-site tests for the 4 call sites (M)

#### Story 1.5: Go characterisation tests for `static.go` and the auth exemption
**As the** owner, **I want** the Go side covered before it matters, **so that** a lockout or caching regression has a test to fail.
**Acceptance Criteria**:
- Static behaviour.
  - *Given* an in-memory `fstest.MapFS` with `index.html`, `rules/index.html`, `_next/static/chunks/a-1.js`, *When* the handler serves `/rules`, `/nonexistent`, `/_next/static/chunks/a-1.js`, *Then* it returns 200 `rules/index.html` (no redirect), 200 root `index.html`, and 200 JS with `Cache-Control: public, max-age=31536000, immutable`; `index.html` carries `no-cache`.
- Exemption.
  - *Given* remote auth enabled and no session, *When* `isExempt` is evaluated for `/login`, `/_next/static/chunks/a-1.js`, `/auth/x`, `/health`, `/favicon.ico`, and for `/assets/a.js`, *Then* the first five are exempt and `/assets/a.js` is not.
**Files**: `server/middleware/static_test.go` (new), `server/middleware/auth_test.go`

##### Task 1.5a: Test first, `static_test.go` (S)
##### Task 1.5b: Extend `auth_test.go` table (XS)

#### Story 1.6: 404 for missing assets under the asset prefix
**As a** user with a stale tab, **I want** a missing hashed chunk to 404, **so that** the browser reports a loading error instead of parsing HTML as JavaScript.
**Acceptance Criteria**:
- Misses under `/_next/` are 404.
  - *Given* the embedded FS without `/_next/static/chunks/old-9.js`, *When* it is requested, *Then* status is 404 (not 200 `index.html`).
- Other fallbacks unchanged.
  - *Given* `/backlog/board` (no file), *When* requested, *Then* status is 200 root `index.html` as before.
**Files**: `server/middleware/static.go`, `server/middleware/static_test.go`

##### Task 1.6a: Test first, then 404 branch (S)

#### Story 1.7: `FeatureGuard` consumed by the three Next layouts
**As a** developer, **I want** one feature-flag guard used by both builds, **so that** guard behaviour cannot drift between the Next layouts and the router table (architecture review).
**Acceptance Criteria**:
- One implementation.
  - *Given* the characterised semantics from Story 1.4 (render `null` while flags load; `replace("/")` once when the flag is off), *When* `src/app/FeatureGuard.tsx` is added taking `{flag: FlagName, children}` and `backlog/layout.tsx`, `settings/backlog-sources/layout.tsx` and `changelog/layout.tsx` are rewritten to render `<FeatureGuard flag=...>{children}</FeatureGuard>`, *Then* the three Story 1.4 layout tests pass unchanged in intent and `rg "router.replace\(\"/\"\)" src/app/*/layout.tsx` finds no bespoke guard left.
- Guard names are table-driven.
  - *Given* `RouteMeta`'s `guard` field (Story 1.2), *When* the drift test runs, *Then* the set of layouts using `FeatureGuard` equals the set of `RouteMeta` entries with a `guard`, and each flag name matches.
**Files**: `web-app/src/app/FeatureGuard.tsx` (new), `web-app/src/app/FeatureGuard.test.tsx` (new), the three layouts above
##### Task 1.7a: Test first, FeatureGuard contract plus layout equivalence (S)
##### Task 1.7b: Implement and rewrite the three layouts (S)

---
## Phase 2: Navigation facade (PRs 2c, 2d, 2a, 2b, 2e)

**PR split (architecture review; every split point is green under Next).** Merge order: **2c** (Story 2.7, `LazyBoundary`) and **2d** (Story 2.8, `web-vitals`) first, because neither depends on the facade; then **2a** (Stories 2.1, 2.2: facade core, `impl.next.tsx`, `impl.memory.tsx`, global jest stub, 43 test files); then **2b** (Stories 2.3-2.6 and 2.9a-c: production migration, `ReplaceSearch`, lint ban at `warn`, `NavLink` collapse, stories); then **2e** (Task 2.9d: ban to `error`, back-compat mock removed) one merge cycle after 2b. Story 2.5's final grep needs 2.7 merged (2c) for the `next/dynamic` count to reach 0.

**Rebase protocol for concurrent PRs (this repo merges autonomous PRs continuously).** Each of 2a, 2b and 2e publishes the codemod command in its PR body (`ast-grep` rule file committed at `web-app/scripts/codemods/next-nav-to-facade.yml`) so in-flight branches can re-run it; the global jest stub in 2a mocks both `next/navigation` (as today) and `@/lib/nav`, so a branch still adding `jest.mock("next/navigation")` keeps passing until 2e; the ban is `warn` in 2b so such branches show a warning, and becomes `error` only in 2e.

### Epic 2.1: `src/lib/nav` and the 108-file migration (65 production importers + 43 test files), under Next
**Goal**: Router-independent app code. Everything keeps building and behaving exactly as before, on Next, because `impl.next.tsx` re-exports `next/navigation`. The facade is derived from real usage (`useRouter` push/replace/back/forward, `useSearchParams`, `usePathname`, `Link`, `Redirect`), not a generic router API. `router.refresh/prefetch/useParams/notFound` are not exported (zero production uses).

#### Story 2.1: Facade core, contract types and `impl.next.tsx`
**As a** developer, **I want** one navigation module with a typed contract, **so that** the bundler and router choices are independent.
**Acceptance Criteria**:
- Contract is complete and typed.
  - *Given* `impl.types.ts`, *When* `tsc --noEmit` runs, *Then* `impl.next.tsx` and `impl.memory.tsx` both type-check against it (each impl file ends with a `satisfies NavImpl` assertion), and the exported names are exactly `useRouter`, `useSearchParams`, `usePathname`, `Link`, `Redirect`, `useReplaceSearch`, `replaceHash`, `pushTrayEntry` plus the pure search-param helpers of Task 2.1d. `Link` and `Redirect` are **impl members** (`.tsx` files; there are no shared `Link.tsx`/`Redirect.tsx`); `replaceHash` and `pushTrayEntry` stay in `impl.types.ts` so raw History cannot be reached around the facade. The router entry (`createAppRouter`, `AppRouterProvider`) is deliberately not among the 8 names: it lives in `src/lib/nav/entry.tsx` (Task 4.2d), is not re-exported by `index.ts` and is not part of `NavImpl`.
- Router-neutral options.
  - *Given* the Next-specific `{ scroll: false }`, *When* the facade options are defined, *Then* `impl.types.ts` names it `{ resetScroll: false }` (mapped to `{scroll:false}` by `impl.next.tsx` and to `preventScrollReset:true` by `impl.router.tsx`), and the 4 call sites are migrated to it by the codemod.
- `RoutePath` is not oversold.
  - *Given* `push`/`replace`/`Link href` signatures, *Then* they accept `string` (call sites append query strings and ids); `RoutePath` is enforced only as the key type of `RouteTable`/`RouteMeta` and by `paths.ts` constants, and the PR body and glossary say so.
- Pathname is normalised.
  - *Given* Next returning `/review-queue/`, *When* `usePathname()` from the facade is called, *Then* it returns `/review-queue`; *Given* `/`, *Then* `/`. (The `it.failing` cases from Story 1.3 turn green and lose `.failing`.)
- Build-time impl selection.
  - *Given* `next.config.ts` with a webpack alias `@/lib/nav/impl` -> `impl.next.tsx` and a `tsconfig` path for `tsc`, *When* `make web-build` runs, *Then* it succeeds and the bundle contains no `impl.router`.
**Files**: `web-app/src/lib/nav/{index.ts,impl.types.ts,impl.next.tsx,impl.memory.tsx,paths.ts,normalize.ts,search-params.ts,testing.tsx}` (new; `testing.tsx` is a thin `renderWithNav` helper over `impl.memory.tsx`, not a second fake), `web-app/src/lib/routes.ts` (re-export from `paths.ts`), `web-app/next.config.ts`, `web-app/tsconfig.json`

##### Task 2.1a: Test first, normalise and contract-shape tests (S)
##### Task 2.1b: Types, `impl.next.tsx` with `Link` (no `prefetch` prop) and `Redirect`, `impl.memory.tsx` (S)
##### Task 2.1c: Webpack alias and tsconfig path; `RoutePath` union from `routes.ts` constants (S)
##### Task 2.1d: Typed search-param helpers (ADR-001 item 3) (S)
- `lib/nav/search-params.ts`: pure parsers (`stringParam`, `intParam`, `enumParam`) over `URLSearchParams`, no router import; adopted by `useUpNextTab` and one further call site named in the PR, with tests. The remaining query keys are not mass-migrated.

#### Story 2.2: Global jest stub and 43 test files onto `@/lib/nav`
**As a** developer, **I want** tests to mock the facade, **so that** they pass identically under any `NavImpl`.
**Acceptance Criteria**:
- Single mock surface.
  - *Given* `jest.setup.js:107` mocking `next/navigation`, *When* it additionally mocks `@/lib/nav` with the same four hooks and shapes (both module ids stay mocked, backward compatible, until PR 2e), *Then* `npx jest --no-coverage` has the same failing-file list as the Story 0.1 control.
- Codemod is complete and not vacuous.
  - *Given* the 43 files containing `jest.mock("next/navigation"` or `jest.mock("next/link"` (the union of the two `ast-grep` queries of decision 13: 40 + 3 link-only), *When* the `ast-grep` codemod runs, *Then* `rg -l "jest\.mock\(['\"]next/(navigation|link)['\"]" web-app/src` prints nothing and `rg -l "from ['\"]next/(navigation|link)" web-app/src --glob '*.test.*' --glob '**/__tests__/**'` prints nothing, while the 8 files that `jest.mock("next/dynamic"` are untouched (that mock stays valid until Task 9.1b; the earlier regex `jest\.mock\(['\"]next/` also matched them and could never print nothing), and a sampled 5 files fail when their mocked `push` expectation is deliberately inverted (proves the mock is live).
- Tooling is provisioned (review R2-C1).
  - *Given* a CI runner and a developer host, *When* Task 2.2d has landed, *Then* `ast-grep --version` equals the pinned `@ast-grep/cli` version in both, no codemod script, test or command in this plan invokes `sg`, and the codemod liveness tests run in CI from this PR.
**Files**: `web-app/jest.setup.js`, 43 `*.test.tsx` files (list produced by the two `ast-grep` queries of decision 13), `web-app/src/lib/nav/testing.tsx`, `web-app/package.json` (`@ast-grep/cli`, `test:codemods`), `Makefile`, `.github/actions/prepare/action.yml`, `web-app/scripts/__tests__/codemods.test.mjs`

##### Task 2.2a: Test first, mutation check on 5 sampled files (S)
##### Task 2.2b: Codemod run, `next/link` mocks render a plain `<a>` (M; codemod exemption, decision 13)
- File set, script and verification command: decision 13, row 2.2b. Fixture-tested script; reviewed as script plus 5 sampled diffs.
##### Task 2.2c: Update global stub (back-compat for both ids; delegates to `impl.memory.tsx`), fix leftover failures (S)
##### Task 2.2d: Provision `ast-grep` and `rg`, and the `test:codemods` script (S; do this first in the story, Task 2.2b needs it)
- `@ast-grep/cli` is a pinned devDependency of `web-app`, invoked as `ast-grep` (through `node_modules/.bin`), never `sg`: on Ubuntu `sg` is the shadow-utils "switch group" command, and the local `sg` is `~/.cargo/bin/sg` only because cargo installed it (`Makefile:896`, `actor-lint`, already assumes it). `make ensure-tools` (`Makefile:66`) and `make install-tools` (`Makefile:808`) gain an `ast-grep` and a `ripgrep` check; the CI `prepare` action installs `ripgrep` explicitly (UNVERIFIED that the runner image ships it).
- Committed tests (codemod liveness, docs-grep and the others in this plan) use Node file APIs and the `ast-grep` binary from `node_modules`, so CI correctness never depends on `rg`; the `rg` lines in this plan are the implementer's pasted verification commands.
- `web-app/scripts/__tests__/codemods.test.mjs` runs under `node --test` through a `test:codemods` package script wired into the CI Jest step and `make test-affected-web`, because the `build` Jest project does not exist until PR 3; Story 3.0 then also collects it. A test in the same file fails if any file under `web-app/scripts/codemods/` invokes `sg` instead of `ast-grep`.

#### Story 2.3: Migrate production `src/app/**` to the facade
**As a** developer, **I want** `app/` pages and layouts off `next/*`, **so that** the router swap touches no page.
**Acceptance Criteria**:
- Imports moved.
  - *Given* `rg -l "from ['\"]next/(navigation|link)" web-app/src/app --glob '!**/*.test.*' --glob '!**/__tests__/**'` (27 files before; the one test file there, `settings/backlog-stages/page.test.tsx`, belongs to Task 2.2b), *When* the codemod runs, *Then* the result is empty and `config/page.tsx` and `settings/defaults/page.tsx` use `<Redirect to="/settings?tab=config-files" />` and `<Redirect to="/settings" />` respectively.
- Behaviour kept.
  - *Given* `/config?x=1`, *When* the page renders, *Then* it redirects to `/settings?tab=config-files` and drops `x` (current behaviour, asserted).
**Files**: all non-test `web-app/src/app/**` files with `next/*` imports (listed by `ast-grep`), `web-app/src/app/config/page.tsx`, `web-app/src/app/settings/defaults/page.tsx`

##### Task 2.3a: Codemod `src/app` (S; codemod exemption, decision 13)
- Files: non-test `web-app/src/app/**/*.{ts,tsx}` importing `next/(navigation|link)`; verification: decision 13, row 2.3a.
##### Task 2.3b: Convert the 2 `redirect()` pages; add the `/config` query-drop test (S)

#### Story 2.4: Migrate production `src/components/**` to the facade
**As a** developer, **I want** components off `next/*`, **so that** no component is router-specific.
**Acceptance Criteria**:
- *Given* `rg -l "from ['\"]next/(navigation|link)" web-app/src/components --glob '!**/*.test.*' --glob '!**/__tests__/**'` (26 files before; the two layout tests, `Header.test.tsx` and `BottomNav.test.tsx`, belong to Task 2.2b), *When* the codemod runs, *Then* the result is empty, `AppLink` re-exports facade `Link`, and the `components` jest project passes with the Story 0.1 failing-file list.
  - *Given* `Navigation.tsx` rendering with pathname `/history`, *When* it renders, *Then* the History item has the active style/`aria-current` it has today (characterisation from Story 1.3).
**Files**: non-test `web-app/src/components/**` (listed by `ast-grep`), `web-app/src/components/ui/AppLink.tsx`

##### Task 2.4a: Codemod `src/components` (M; codemod exemption, decision 13)
- Files: non-test `web-app/src/components/**/*.{ts,tsx}`; verification: decision 13, row 2.4a. Residual hand edits go to Task 2.4b.
##### Task 2.4b: Fix type errors and snapshot churn (S)

#### Story 2.5: Migrate production `src/lib/**` and remaining files to the facade
**As a** developer, **I want** hooks and utilities off `next/*`, **so that** the lint ban can go to `error`.
**Acceptance Criteria**:
- *Given* `rg -l "from ['\"]next/(navigation|link|dynamic)" web-app/src --glob '!**/*.test.*' --glob '!**/__tests__/**' --glob '!**/src/lib/nav/**' --glob '!**/src/lib/lazy/**'` (the globs are anchored with `**/`; the unanchored `!src/lib/nav/**` excludes nothing from the repository root), *When* Stories 2.3-2.5 and 2.7 are complete, *Then* the result is empty (the prior count, 65 by this regex on 2026-10-19, is stated in the PR body).
- *Given* `useWindowUrlSync` calling `router.replace(url, { scroll: false })`, *When* it runs through the facade, *Then* the call site passes `{resetScroll:false}`, `impl.next.tsx` forwards `{scroll:false}` to Next, and the call count in its test is unchanged.
**Files**: `web-app/src/lib/**` (listed by `ast-grep`), `web-app/src/lib/window/useWindowUrlSync.ts`, `web-app/src/lib/hooks/*`

##### Task 2.5a: Codemod `src/lib` and strays (S; codemod exemption, decision 13)
- Files: `web-app/src/lib/**` and strays outside `app`, `components`, `lib/nav`, `lib/lazy`; verification: decision 13, row 2.5a.
##### Task 2.5b: `resetScroll` option parity for the 4 `{scroll:false}` sites (S)

#### Story 2.6: `ReplaceSearch` and hash helper replace the 4 raw History sites
**As a** user, **I want** tab and file selections to keep working after the router swap, **so that** URL state does not silently go stale.
**Acceptance Criteria**:
- Sites migrated.
  - *Given* the calls at `useUpNextTab.ts:69`, `UnfinishedTab.tsx:144`, `SessionDetailView.tsx:502` (and 504), `useTrayDismissal.ts:51` (Task 2.6a re-greps `replaceState|pushState` to catch any site added since), *When* migrated, *Then* no production file outside `lib/nav` calls `history.replaceState` or `history.pushState` except through `useReplaceSearch` / `replaceHash` / `pushTrayEntry`, verified by an ESLint `no-restricted-syntax` selector on `window.history.*State`.
- State preserved.
  - *Given* `history.state = {idx:3, ssqTray:true}`, *When* `useReplaceSearch()("?tab=b")` runs under `impl.next.tsx`, *Then* `history.state` is unchanged apart from the URL and `useSearchParams()` returns `tab=b` (the old behaviour, now behind the facade).
- Story 1.4 tests still pass untouched in intent.
**Files**: `web-app/src/lib/nav/{impl.types.ts,impl.next.tsx,index.ts}`, `web-app/src/lib/hooks/useUpNextTab.ts`, `web-app/src/app/unfinished/UnfinishedTab.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`, `web-app/src/lib/hooks/useTrayDismissal.ts`

##### Task 2.6a: Test first, helper contract tests incl. foreign-state preservation (S)
##### Task 2.6b: `impl.next.tsx` implementations (raw `replaceState`, as today) and 4 call-site edits (M)
##### Task 2.6c: ESLint selector banning raw History calls outside `lib/nav` (XS)

#### Story 2.7: `LazyBoundary` replaces `next/dynamic`
**As a** developer, **I want** one lazy-loading helper with a stale-chunk recovery, **so that** code splitting does not depend on Next.
**Acceptance Criteria**:
- Same loading behaviour.
  - *Given* the 6 `dynamic()` call sites in 3 files (`InsightsDashboard.tsx` x4, `SessionDetail.tsx`, `SessionDetailView.tsx`; `test/terminal-stress/page.tsx` has only an unused `next/dynamic` import, which is removed, not migrated; counted by `grep -rnE 'dynamic\(' web-app/src` on 2026-10-19), *When* migrated to `lazyComponent(() => import(...), {loading})`, *Then* each shows its skeleton, then the component, and no site passes `ssr`.
- Stale chunk recovery.
  - *Given* a dynamic import rejecting with a chunk-load error, *When* the lazy component mounts for the first time in a session, *Then* `location.reload()` is called once and a flag prevents a second reload; on the second failure the `ErrorBoundary` renders.
- Flag lifecycle (adversarial review).
  - *Given* the reload flag set before a reload, *When* the first route renders successfully after boot, *Then* the flag is cleared, so a second deploy later in the same long-lived tab auto-recovers once more; the flag value is the build id (`document.querySelector('script[src*="/_next/static/"]')` entry URL hash), so a flag from an older build never suppresses recovery.
- Storage unavailable (UX G6).
  - *Given* `sessionStorage` throwing on access (private mode, blocked site data), *When* a chunk-load error occurs, *Then* the helper falls back to a module-level in-memory flag (still at most one reload per page lifetime) and never throws from the recovery path.
- Unsaved input is not silently discarded (UX G6; pre-mortem P1-2).
  - *Given* React DOM keeps a controlled input's `defaultValue` in step with its `value` (INFERRED, so `value !== defaultValue` is always false for this app's controlled inputs: omnibar, prompt boxes, forms), *When* the lazy helper is first used, *Then* it installs one document-level capture `input` listener that records each edited field in a `dirty` set held as `Set<WeakRef<Element>>` plus a `WeakSet<Element>` for once-only membership (cleared on form `submit` and when the field's value becomes empty); every evaluation first prunes the set, dropping any entry whose `deref()` is gone or whose node is `!isConnected`, so a field that was edited and then unmounted by a route change (the exact case this guard exists for) cannot keep automatic recovery off for the rest of the page lifetime and the set never retains detached DOM nodes (review R2-C6); `defaultValue` is never consulted.
  - *Given* a chunk-load error, *When* the pruned dirty set still holds a connected field, or any visible text-like field (`input` of type text/email/url/tel/password/number, `textarea`, `[contenteditable]`) has a non-empty value, excluding search and filter boxes (`type="search"`, `role="searchbox"`, `[data-draft-ignore]`), *Then* no automatic reload happens and a non-blocking banner (`role="status"`, text "A new version is available. Reload when ready.", with a Reload button) is shown instead; the automatic reload applies only when no such field exists.
  - *Given* a controlled input that is typed into and then unmounted (a route change replaces it) before a chunk-load error, *When* the error occurs, *Then* the pruned set is empty, no banner shows, exactly one automatic reload happens (test in Task 2.7d, using a test-only `dirtyCount()` export of `stale-chunk.ts`), and the same test asserts the unmounted node is no longer reachable from the set.
  - *Given* the Jest test in Task 2.7a, *Then* it renders a **real React controlled input** (`<input value={v} onChange={...}/>` driven by `userEvent.type`, not a bare DOM node and not an uncontrolled fixture, which would pass vacuously) and asserts the banner appears and `location.reload` is not called; a negative case with an empty field asserts one reload.
  - *Given* the Bun build, *When* the Playwright spec of Task 7.2c types into the real omnibar (a React controlled input), makes the next route chunk return 404 through `page.route`, and navigates, *Then* a `window.__marker` set before navigation survives (no reload) and the banner is visible.
- Reloads are observable (REQ-N4, validation A10).
  - *Given* the stale-chunk recovery path, *When* it reloads or shows the banner, *Then* it increments the existing telemetry counter (or `performance.mark('stale-chunk-reload')` when no counter exists) once per event, asserted in Task 2.7e.
- Seam.
  - *Given* `src/lib/lazy/impl` aliased like `nav/impl`, *When* built under Next, *Then* `impl.next.tsx` uses `next/dynamic` (still the Next build's behaviour) and `impl.bun.tsx` uses `React.lazy` + `Suspense`.
  - *Given* `web-app/next.config.ts` with a webpack alias `@/lib/lazy/impl` -> `impl.next.tsx` and a matching `tsconfig` path (the Task 2.1c mechanism; this is why `next.config.ts` and `tsconfig.json` are in Files), *When* `make web-build` runs, *Then* it succeeds and the bundle contains neither `impl.bun` nor `stale-chunk`.
**Files**: `web-app/src/lib/lazy/{index.ts,impl.types.ts,impl.next.tsx,impl.bun.tsx,stale-chunk.ts}` (new; the dirty-set listener and recovery logic live in `stale-chunk.ts`, used by `impl.bun.tsx` only), the 4 files above (3 migrated, 1 import removed), `web-app/next.config.ts`, `web-app/tsconfig.json`

##### Task 2.7a: Test first, chunk-error reload-once tests with `impl.bun.tsx`, including the real-controlled-input case (S)
##### Task 2.7b: Implementations and alias (S)
##### Task 2.7c: Migrate the 4 files (S)
##### Task 2.7d: Flag lifecycle, storage fallback and the dirty-set unsaved-input banner with tests, including the edited-then-unmounted case (S)
##### Task 2.7e: Telemetry counter for stale-chunk reloads with a test (XS)

#### Story 2.8: `web-vitals` package replaces `next/dist/client/web-vitals`
**As the** owner, **I want** telemetry to keep flowing without a Next-internal import, **so that** removing Next does not blind the dashboards.
**Acceptance Criteria**:
- Same payload.
  - *Given* `WebVitalsReporter.tsx` using `onCLS`, `onFCP`, `onINP`, `onLCP`, `onTTFB` from `web-vitals@6.2.3` (exact pin, `npm pack` inspected), *When* a mocked LCP metric `{name:"LCP", value:1200, rating:"good", delta:1200, id:"v1"}` fires, *Then* the reporter emits the same fields and `performance.mark('web-vital:LCP')` as today.
- FID note.
  - *Given* the comment listing FID, *When* edited, *Then* it states INP replaces FID and the PR body names the `web_vital.fid` series as ending; a repo grep for `web_vital.fid` in dashboards/docs is attached.
**Files**: `web-app/src/components/telemetry/WebVitalsReporter.tsx`, its test, `web-app/package.json`

##### Task 2.8a: Test first, mocked-metric test (XS)
##### Task 2.8b: Swap, pin, dashboard grep (S)

#### Story 2.9: Lint ban, `NavLink` collapse, stories on the facade
**As a** developer, **I want** the boundary enforced, **so that** no `next/*` import can return outside the facade.
**Acceptance Criteria**:
- Ban at `error`.
  - *Given* (no exclusion is added for the router entry, which lives in `src/lib/nav/entry.tsx`, ADR-001 amendment) `.eslintrc.json` with `no-restricted-imports` at severity `warn` in PR 2b and `error` from PR 2e (Task 2.9d), for `next`, `next/*`, `react-router`, `react-router-dom` (excluding `src/lib/nav/**`, `src/lib/lazy/**`, `*.test.*`, `*.spec.*`, and type-only `next` imports in `src/app/**` for `Metadata`/`Viewport` until Story 9.1), *When* a file imports `next/link`, *Then* ESLint reports the rule at the current severity with the message pointing at `@/lib/nav` and the codemod command.
- Old wrappers gone.
  - *Given* `lib/navigation/NavLink.tsx` and the 24 `next/link` importers, *When* the codemod finishes, *Then* `lib/navigation/` is deleted and `AppLink.tsx` is a one-line re-export of facade `Link`.
- Stories do not need Next.
  - *Given* `AppLink.stories.tsx` and `Navigation.stories.tsx`, *When* Storybook builds with the alias `@/lib/nav/impl` -> `impl.memory.tsx` (the same in-memory fake the Jest stub delegates to; there is no second fake), *Then* `storybook build` exits 0 and neither story imports `next/*`.
**Files**: `web-app/.eslintrc.json`, `web-app/src/lib/navigation/NavLink.tsx` (delete), `web-app/src/components/ui/AppLink.tsx`, `web-app/src/lib/nav/impl.memory.tsx` (new), `web-app/.storybook/main.ts`, the two stories

##### Task 2.9a: Test first, ESLint RuleTester case for the ban (XS)
##### Task 2.9b: Rule, NavLink deletion, `AppLink` re-export (S)
##### Task 2.9c: Storybook alias to `impl.memory.tsx` + two stories (S)
##### Task 2.9d: Flip the ban to `error` and remove the `next/navigation` back-compat mock, one merge cycle after PR 2b (XS)
##### Task 2.9e: Guard rails for concurrent PRs (S)
- Extends `.claude/rules/nav-facade.md` (from Task 1.2c) with the facade rule and the codemod command; the lint message carries the same command; a CI grep fails any new `eslint-disable.*no-restricted-imports` or `jest.mock("next/` outside an allowlist; a merge-queue freeze window for `web-app/src/**` PRs is set while PR 2b is open instead of relying on the back-compat stub alone (pre-mortem failure mode 5).

---

## Phase 3: `bun patch` of `eval` and direct vanilla-extract deps (PR 3)

### Epic 3.1: Make vanilla-extract evaluate under Bun, safely
**Goal**: A runner for Bun-only tests (Story 3.0), then a carried, tested, pinned patch (ADR-008) that does not disturb the Next build. Precondition for 3.0, 3.2 and 3.3: the branch is rebased on the merged PR #1100 (Unresolved Questions: rebase gate).

#### Story 3.0: `build` Jest project and `runInBun` helper (the Bun-side test runner; first story of PR 3)
**As a** developer, **I want** tests of Bun-only behaviour to be collected and run by the standard Jest run, **so that** the Phase 3-5 acceptance criteria are executable and cannot pass vacuously under Node (adversarial B1).
**Acceptance Criteria**:
- Runner exists and collects the right files.
  - *Given* `web-app/jest.config.js` with its existing projects (`src`, `eslint-plugin-analytics`, `scripts/dev-stack`, `tests/e2e`), *When* a project named `build` is added (`testEnvironment: "node"`, `rootDir: "<web-app>"`, `testMatch: ["<rootDir>/build/**/__tests__/**/*.test.ts", "<rootDir>/scripts/__tests__/**/*.test.mjs"]`, `preset: "ts-jest"` and the ts-jest `transform` block copied from the `dev-stack` project (`web-app/jest.config.js` lines 66-84; NOT `eslint-plugin-analytics`, whose `transform: {}` at line 64 is plain JS and cannot parse `.ts`), `roots: ["<rootDir>/build", "<rootDir>/scripts/__tests__"]`, `*.bun.ts` and `fixtures/` excluded by the `testMatch`), *Then* `npx jest --selectProjects build --listTests` lists a seeded `build/__tests__/runner-smoke.test.ts` and a seeded `scripts/__tests__/runner-smoke.test.mjs` (if ESM `.mjs` collection under Jest needs `--experimental-vm-modules`, the `.mjs` script tests are instead run by `node --test` through a second `package.json` script, `test:scripts`, wired into the same CI step and `make` target; the choice is recorded in this story's PR and `docs/how-to/build-web-app.md`), and `npm test` (`web-app/package.json`: `jest`), `make test-affected-web` and the CI Jest job all collect them with no extra flag.
- Bun-side assertions really run in Bun.
  - *Given* `build/__tests__/helpers/run-in-bun.ts` exporting `runInBun(script: string, args?: string[]): {status, json, stderr}`, *When* it runs `bun <script>` (a `*.bun.ts` under `build/__tests__/bun/`, `cwd` = `web-app`), *Then* it first asserts `bun --version` equals the version in `.bun-version` (a mismatch fails with a message naming `make ensure-tools`), runs with a 60 s timeout, and parses one JSON object from stdout; a seeded `bun/runtime.bun.ts` returns `{runtime: typeof Bun !== "undefined"}` and the smoke test asserts `true`, proving the code ran under Bun and not Node.
- Missing Bun fails, it does not skip.
  - *Given* `bun` absent from `PATH`, *When* the `build` project runs, *Then* the suite fails with a clear message (never `test.skip`), both locally and in CI.
- Every `*.bun.ts` is driven through `runInBun` (adversarial re-review: the test-location rule was a convention only).
  - *Given* `build/__tests__/bun-coverage.test.ts` (a plain Node test in the `build` project) that globs `build/**/__tests__/bun/*.bun.ts` and `build/**/__tests__/**/*.test.ts`, *When* it runs, *Then* it fails naming any `*.bun.ts` that no `*.test.ts` references through an import of `run-in-bun` plus the script's file name, and any `*.test.ts` that mentions a `.bun.ts` without importing `run-in-bun`; a fixture pair (an orphan `.bun.ts` and a test that skips `runInBun`) proves the check can fail.
- New files under `web-app/build/` are tracked, not ignored (adversarial N1).
  - *Given* `.gitignore:95` is `build/`, which ignores every directory named `build` including `web-app/build/` (VERIFIED: `git check-ignore -v web-app/build/build.ts` reports `.gitignore:95:build/` and exits 0 today), *When* the line `!/web-app/build/` is added directly after `build/` (a root-anchored negation of the parent directory; git applies it because the parent directory itself is re-included, so nested paths follow; VERIFIED in a scratch repo with `build/` + `!/web-app/build/`: `web-app/build/build.ts`, `web-app/build/__tests__/a.test.ts`, `web-app/build/__tests__/bun/x.bun.ts`, `web-app/build/plugins/p.ts` and `web-app/build/.out-tmp/q` all exit 1 from `git check-ignore`, while `other/build/z` is still ignored, exit 0), *Then* `git check-ignore -v web-app/build/build.ts web-app/build/__tests__/runner-smoke.test.ts web-app/build/__tests__/bun/runtime.bun.ts web-app/build/plugins/p.ts` exits 1 (none ignored) and `git add --dry-run web-app/build/__tests__/runner-smoke.test.ts` lists the file; a Jest test `build/__tests__/gitignore.test.ts` runs `git check-ignore` on those paths and fails if any exits 0. Decision: negate rather than rename the directory to `web-app/bun-build/`, because every later story, the dependency diagram, three ADRs and the Makefile prerequisites name `web-app/build/`; the negation is one line and is test-guarded. (The Story 5.7 and 6.9 additions `/web-app/.out-tmp-*`, `/web-app/.build/`, `/web-app/.web-build-mode` stay as written.)
- Version pin exists where this story needs it.
  - *Given* the `runInBun` version check reads `.bun-version` and the CI step takes its version from it, *When* this story lands, *Then* `.bun-version` (repo root, one line, e.g. `1.4.3`, final value fixed by the S1 GateRecord) exists in this story's diff and `.tool-versions` names the same version (Story 5.7 then only verifies, and no longer creates it); `ls .bun-version` succeeds in a clean checkout.
- Next's type-check does not see Bun-only code (INFERRED: `web-app/tsconfig.json` `include` has `**/*.ts` and no `@types/bun` is planned, so `next build` would type-check `build/**/*.ts` and `*.bun.ts`, which use `Bun.*`; `next.config.ts` has no `ignoreBuildErrors`).
  - *Given* `build/**` added to the base `tsconfig.json` `exclude` (alongside `node_modules` and `src/gen/session/v1/session_connect.ts`), *When* the seeded `build/__tests__/bun/runtime.bun.ts` (which references the `Bun` global) exists and `WEB_BUILD=next make web-build` (or `npx next build` with the type-check step) runs, *Then* the build exits 0 with no `Bun`/`bun` type error; the implementer first runs `next build` WITHOUT the exclude and records whether it fails (turning INFERRED into VERIFIED either way), and Story 5.7's `tsconfig.bun.json` (with `@types/bun`) later type-checks `build/**` for the Bun side.
- Wiring.
  - *Given* the CI Jest job and `make ready`, *When* they run, *Then* Bun is installed first (`oven-sh/setup-bun` pinned by SHA, version from `.bun-version`), and `make test-web-build` (new; `cd web-app && npx jest --selectProjects build`) is a prerequisite of `make ready`. `make test` (`Makefile:612`, the Go suite) is not changed.
- Two repo-wide scans fail the suite (validation A5, A6).
  - *Given* `web-app/scripts/__tests__/no-live-port.test.mjs`, *When* it scans `project_plans/web-app-bun-build/spikes/`, `web-app/build/`, `web-app/scripts/`, `scripts/dev-stack/` and `tests/e2e/helpers/` for the literal `8543` outside an allowlist of lines that document the live port (each with a reason), *Then* any other hit fails naming file and line (REQ-C1: harness and spike scripts never target the live instance); *and given* `web-app/scripts/__tests__/no-bun-test.test.mjs`, *When* it scans the `package.json` scripts, the Makefile, `.github/workflows/*.yml` and `web-app/scripts` for `bun test`, *Then* any hit fails (REQ-C3: Bun is not the test runner; a full-suite `bun test` run segfaulted). Each scan has a fixture proving it can fail.
- Rule for the rest of the plan.
  - *Given* the Test-location rule in Plan-level decision 6, *When* Stories 3.4, 4.1, 5.1-5.6, 5.9 and 6.4 are implemented, *Then* each of their test files lives where this project collects it.
**Files**: `.gitignore` (`!/web-app/build/` after line 95), `.bun-version` (new), `.tool-versions`, `web-app/tsconfig.json` (`exclude` += `build/**`), `web-app/jest.config.js`, `web-app/build/__tests__/helpers/run-in-bun.ts` (new), `web-app/build/__tests__/bun/runtime.bun.ts` (new), `web-app/build/__tests__/runner-smoke.test.ts` (new), `web-app/build/__tests__/bun-coverage.test.ts` (new), `web-app/build/__tests__/gitignore.test.ts` (new), `web-app/scripts/__tests__/runner-smoke.test.mjs` (new), `web-app/scripts/__tests__/no-live-port.test.mjs` (new), `web-app/scripts/__tests__/no-bun-test.test.mjs` (new), `web-app/package.json`, `Makefile`, `.github/workflows/build.yml`
##### Task 3.0a: Test first, the two smoke tests, then the `build` project in `jest.config.js` (M)
##### Task 3.0b: `runInBun` with pin check, timeout and JSON contract (S)
##### Task 3.0c: Makefile target, CI Bun install step, failure-not-skip test (S)
##### Task 3.0d: `.gitignore` negation + `git check-ignore` test, `.bun-version`, tsconfig `exclude` with the `next build` type-check run, `bun-coverage` check (S) — do this task first in the story: nothing else in Phase 3-6 is trackable until the negation lands
##### Task 3.0e: `no-live-port` and `no-bun-test` scans with failing fixtures (S)

#### Story 3.1: Direct, exact-pinned vanilla-extract dependencies
**As a** developer, **I want** the packages the build plugin imports to be direct dependencies, **so that** versions are explicit.
**Acceptance Criteria**:
- *Given* `@vanilla-extract/integration` only arriving through `@vanilla-extract/next-plugin`, *When* it is added as a devDependency at `8.0.11` exact, `@vanilla-extract/css` at an exact pin (the version this story resolves, minimum 1.21.3), `eval` pinned at `0.1.8` via `overrides`, *Then* `bun install --frozen-lockfile` succeeds and `WEB_BUILD=next make web-build` output has the same class-name set for 3 sampled `.css.ts` files.
- *Given* `@vanilla-extract/next-plugin` still in use by Next, *When* `css` moves to the new version, *Then* the Next build and Jest pass (else the bump is split and documented).
**Files**: `web-app/package.json`, `bun.lock` (location per PR #1100)

##### Task 3.1a: Add and pin; run Next build and Jest (S)

#### Story 3.2: Author and commit `patches/eval@0.1.8.patch`
**As a** developer, **I want** the `eval` shim carried by Bun, **so that** `.css.ts` files evaluate under Bun.
**Acceptance Criteria**:
- Patch applies from a clean install.
  - *Given* `STAPLER_SQUAD_SHARED_NODE_MODULES=0`, `bun patch eval@0.1.8` immediately followed by the `vm` + `createRequire` shim written to `eval.js` and `bun patch --commit`, *When* `rm -rf node_modules && bun install --frozen-lockfile` runs, *Then* `node_modules/eval/eval.js` matches the shim and `stat -c %h` on it is 1 (not hardlinked to the global cache).
- Next unaffected.
  - *Given* the patch applied, *When* `WEB_BUILD=next make web-build` and the Jest `web-app` project run, *Then* both succeed with unchanged output class names for the 3 sampled files.
- Oracle.
  - *Given* a Node subprocess evaluating every `.css.ts` file (the set is globbed, 337 on 2026-10-19), *When* compared with Bun evaluation through the patched `eval`, *Then* JS and CSS outputs are byte-identical for all of them (the spike result, reproduced in the repo as a script that ports `spikes/bun-spike/` from Story 0.7).
**Files**: `patches/eval@0.1.8.patch` (new), root `package.json` (`patchedDependencies`: Bun reads it from the root manifest only, see Unresolved Questions), `web-app/build/oracle/ve-oracle.ts` (new)

##### Task 3.2a: Author the patch per runbook; document runbook in `docs/how-to/patch-vanilla-extract-eval.md` (S)
##### Task 3.2b: Oracle script (Node vs Bun, every globbed `.css.ts` file) (M)

#### Story 3.3: Shared `node_modules` hash covers lockfile and patches
**As a** developer working in many worktrees, **I want** a patch change to invalidate the shared tree, **so that** no worktree serves an unpatched or foreign-patched `eval`.
**Acceptance Criteria**:
- *Given* two worktrees whose only difference is `patches/eval@0.1.8.patch`, *When* the real `scripts/shared-node-modules.sh` (never a copy of its logic) computes the hash, *Then* the hashes differ; *Given* identical inputs, *Then* they are equal.
- *Given* the script's `.test.sh` and Makefile test target, *When* run, *Then* they pass including the new case, and the script keys on `bun.lock` (not `pnpm-lock.yaml`) and `node_modules/.bin` presence.
**Files**: `scripts/shared-node-modules.sh`, `scripts/shared-node-modules.test.sh` (shell tests, not Bun-side; they stay under `scripts/`). Depends on the answered lockfile-location question (Unresolved Questions); this story does not start before it is resolved.

##### Task 3.3a: Test first, hash-differs test (XS)
##### Task 3.3b: Hash inputs and lockfile re-key (S)

#### Story 3.4: `PatchCanary` and `verify-eval-patch` in `make ready` and CI
**As a** developer, **I want** a loud failure when the patch is absent, **so that** I never debug `setAdapter of undefined`.
**Acceptance Criteria**:
- The canary runs under Bun and can fail, asserting on exit status, not on error text.
  - *Given* the `build` project of Story 3.0 and an unpatched copy of `eval@0.1.8`'s `eval.js` committed at `build/__tests__/fixtures/eval-unpatched/eval.js`, *When* the Jest test `ve-eval-patch.test.ts` calls `runInBun("ve-eval-patch.bun.ts", ["--eval-dir", "<fixture>"])`, *Then* the Bun script exits non-zero (it catches the failure, prints `{ok:false, error}` as its JSON line and exits 1) and the Jest assertion is `status !== 0 && json.ok === false`; the same probe against the real, patched `node_modules/eval` exits 0 with `ok:true` (the **negative test**). The error text is deliberately NOT asserted: VERIFIED (re-review, `npm pack eval@0.1.8`, Bun 1.4.3) that the probe snippet `var p=require('path'); module.exports={sep:p.sep}` returns `{"sep":"/"}` under `node` and throws `undefined is not an object (evaluating 'p.sep')` under `bun`, which is not the `setAdapter of undefined` message seen in a real vanilla-extract run, and the wording differs between Bun versions. A Node-only run cannot reproduce the failure, so it could not have caught the missing patch.
- Marker script.
  - *Given* the patch removed from `node_modules/eval/eval.js`, *When* `bun run verify:eval-patch` (`scripts/verify-eval-patch.mjs`, executed by Bun) runs, *Then* it exits 1 with a message naming `patches/eval@0.1.8.patch` and issue #33520.
- Functional canary.
  - *Given* the patch present, *When* the canary `.css.ts` (a `style()` and a `createTheme()` with a custom property) is evaluated inside `bun` by `@vanilla-extract/integration`'s `compile()` + `processVanillaFile()` (the calls `VePlugin` wraps; `VePlugin` does not exist until Story 5.1, which re-runs this canary through the plugin) (Bun-side script, result returned as JSON), *Then* it returns a non-empty class name and CSS containing the custom property.
- Every build path is guarded, not only `make ready` (pre-mortem failure mode 7).
  - *Given* `build/build.ts`, *When* it starts, *Then* its first stage, `guard`, runs the `verify-eval-patch` marker check and exits non-zero naming `patches/eval@0.1.8.patch` before any other work, so the release workflow, `make install-service` on a new host and a worktree on a stale shared `node_modules` are all covered (Story 5.5 lists `guard` among its stages).
- Gate wiring.
  - *Given* `make ready`, *When* run, *Then* it invokes both checks before the lint steps and records a sentinel in `.cache/` keyed on `patches/**` and `bun.lock`.
**Files**: `web-app/scripts/verify-eval-patch.mjs` (new), `web-app/build/__tests__/ve-eval-patch.test.ts` and `web-app/build/__tests__/bun/ve-eval-patch.bun.ts` (new; collected by the Story 3.0 project), `web-app/build/__tests__/fixtures/eval-unpatched/eval.js` (new), `Makefile`, `.github/workflows/lint.yml`

##### Task 3.4a: Test first, negative (unpatched fixture fails) and positive (patched passes) cases (S)
##### Task 3.4b: Marker script, Makefile and lint.yml wiring (S)
##### Task 3.4c: Functional canary through `compile()` + `processVanillaFile()` under Bun (S)

#### Story 3.5: Upstream tracking
**As the** owner, **I want** the fix offered upstream, **so that** the patch can be deleted later.
**Acceptance Criteria**:
- *Given* vanilla-extract#1664, *When* a comment with the `vm`/`createRequire` replacement diff is posted and an issue is filed against Bun for `Module._load` returning `undefined`, *Then* both URLs are recorded in `docs/how-to/patch-vanilla-extract-eval.md` with a "delete patch when" condition.
**Files**: `docs/how-to/patch-vanilla-extract-eval.md`

##### Task 3.5a: Post upstream comment/issue and record links (XS)

---
## Phase 4: Router implementation and app entry (PR 4; dead code until PR 5a selects it)

### Epic 4.1: `impl.router.tsx`, `RouteTable`, `main.tsx`
**Goal**: Everything the Bun build needs at runtime, tested with `MemoryRouter`/`createMemoryRouter` in jest. Nothing in the Next build imports it. Precondition: gate S4 PASS and ADR-001 confirmed.

#### Story 4.1: Pin `react-router@7.18.4` and add a supply-chain gate
**As the** owner, **I want** the router dependency pinned and screened, **so that** a repeat of the May 2026 TanStack incident cannot ride in through a range.
**Acceptance Criteria**:
- Exact pin and screen.
  - *Given* `react-router` added at `7.18.4` (no caret), *When* `node web-app/scripts/check-dependency-screen.mjs` runs, *Then* it exits 0 and prints "no github: sources, no optionalDependencies, no install scripts" for `react-router` and its dependencies in `bun.lock`.
- Banned versions rejected.
  - *Given* a fixture lockfile containing `@tanstack/react-router@1.169.5`, *When* the script runs on it, *Then* it exits 1 naming the version and the 2026-05-11 incident.
- Install policy.
  - *Given* `bunfig.toml` `[install] minimumReleaseAge = 259200`, *When* `bun install --frozen-lockfile` runs, *Then* it succeeds with the committed lockfile.
**Files**: `web-app/package.json`, `bun.lock`, `web-app/bunfig.toml` (new or extended from PR #1100), `web-app/scripts/check-dependency-screen.mjs` (new), `web-app/scripts/__tests__/check-dependency-screen.test.mjs` (new; collected by the `build` project of Story 3.0, pure Node logic)

##### Task 4.1a: Test first, fixture-based screen tests (S)
##### Task 4.1b: Screen script, pin, bunfig (S)

#### Story 4.2: `impl.router.tsx`, the React Router implementation of the contract
**As a** developer, **I want** the facade backed by React Router, **so that** the Bun build navigates without Next.
**Acceptance Criteria**:
- Hooks keep their shapes.
  - *Given* a `createMemoryRouter` at `/unfinished?tab=a&item=7`, *When* `useSearchParams()` is read, *Then* it returns a read-only `URLSearchParams` with `tab=a`, `item=7`, and the same object identity across re-renders with unchanged URL (no effect re-fire in `useOmnibarQueryParamLaunch`).
  - *Given* `/review-queue/`, *When* `usePathname()` is read, *Then* `/review-queue`.
- Push vs replace preserved.
  - *Given* `useRouter().replace("/?new=true")` and `.push("/history")`, *When* each runs, *Then* history length is unchanged for replace and +1 for push; `replace(url,{resetScroll:false})` maps to `preventScrollReset:true`.
- Link behaviour.
  - *Given* `<Link href="#main-content">` (skip link) and `<Link href="/rules">`, *When* clicked, *Then* the hash link is left to the browser (no router navigation), and `/rules` navigates without a full reload.
- Redirect and ReplaceSearch.
  - *Given* `<Redirect to="/settings?tab=config-files"/>`, *When* mounted, *Then* the location is replaced (one history entry). *Given* `history.state={ssqTray:true}`, *When* `useReplaceSearch()("?tab=b")` runs, *Then* `useSearchParams()` returns `tab=b` and the marker is still present.
- Router entry stays behind the facade (consistency B2; ADR-001 amendment).
  - *Given* `web-app/src/lib/nav/entry.tsx` exporting `createAppRouter(spec)` and `AppRouterProvider`, where `spec` is `{routes, shell: ({children, mainRef}) => ReactNode, pending, errorElement: ({error, retry}) => ReactNode, notFound, titleFor}` expressed only in app-level types, *When* `src/main.tsx` and `routes.tsx` are linted, *Then* neither imports `react-router` (the ban at `error` passes with no new exclusion), and a second `no-restricted-imports` pattern allows `@/lib/nav/entry` only from `src/main.tsx` and tests (`routes.tsx` therefore does not import the entry: it supplies the `routes` table that `main.tsx` passes to `createAppRouter`; decision 12). Two checks replace assumption (review R2-C5): Task 4.2d runs `eslint src/main.tsx src/lib/nav/entry.tsx src/app/routes.tsx` and asserts exit 0 (the new `src/main.tsx` matches no `boundaries` element, so only a run shows it is clean), and a unit test asserts that the runtime export set of `@/lib/nav` equals an explicit allow-list (the 8 facade names plus the Task 2.1d helpers `stringParam`, `intParam`, `enumParam`) and contains none of `createAppRouter`, `AppRouterProvider`, `createBrowserRouter`, `RouterProvider`, `Outlet`; the `src/lib/nav/**` exemption in `no-restricted-imports` covers any file later dropped into that directory, and this test is what stops such a file from leaking through `index.ts`. `entry.tsx` builds the pathless layout route (`<Shell mainRef={...}><Outlet/></Shell>`), the pathless `errorElement` child, per-route lazy loading under `Suspense` with `pending`, the `*` route, and mounts the announcer and focus hook inside the shell. `index.ts` does not re-export it, so the Next build never bundles it.
**Files**: `web-app/src/lib/nav/impl.router.tsx` (new; exports the hooks plus `Link` and `Redirect`, with a `satisfies NavImpl` assertion), `web-app/src/lib/nav/entry.tsx` (new), `web-app/src/lib/nav/index.ts`, `web-app/.eslintrc.json` (entry-only import pattern)

##### Task 4.2a: Test first, shared NavContract cases for the router impl (M)
##### Task 4.2b: Implement hooks, `Link`, `Redirect` (M)
##### Task 4.2c: `ReplaceSearch`, `replaceHash`, `pushTrayEntry` preserving foreign `history.state` (S)
##### Task 4.2d: `lib/nav/entry.tsx` (`createAppRouter`, `AppRouterProvider`), the entry-only import pattern, the eslint run over `main.tsx`/`entry.tsx`/`routes.tsx` and the `@/lib/nav` export-set test, with tests over `createMemoryRouter` (S)

#### Story 4.3: `RouteTable` with 42 routes, guards, redirects and not-found
**As a** user, **I want** every URL to resolve as today, **so that** bookmarks and server-built deep links keep working.
**Acceptance Criteria**:
- Exhaustive table, one source of truth.
  - *Given* `RoutePath` (42 literals incl. `/` and `/fanout`, count derived from `find src/app -name page.tsx`), *When* `routes.tsx` declares `const routeTable = {...} satisfies Record<RoutePath, RouteDef>` with `RouteDef = {load} | {redirect, keepQuery}`, *Then* removing any entry fails `tsc`, and a test asserts that the key sets of `RoutePath`, `RouteMeta` and `routeTable` are equal and equal to the `find src/app -name page.tsx` output. `routes.tsx` stores no title and no guard: it looks both up from `RouteMeta[path]` (architecture review: three title sources collapse to `metadata` for Next and `RouteMeta` for Bun, with the Story 1.2 drift test between them).
- Titles and lazy loading.
  - *Given* `RouteMeta`, *When* the router renders `/settings/jules`, *Then* the page chunk is requested via `import()` and `document.title` is `Jules - Settings - Stapler Squad`.
- Route-level pending UI (UX G1).
  - *Given* a route whose `import()` has not resolved, *When* it stays pending for more than 150 ms, *Then* the shell (header, nav, notification panel) stays mounted and a `RoutePending` skeleton (reusing the existing page skeleton primitive; `aria-busy="true"` on the layout `<main>`) renders inside `<main>`; for pending under 150 ms nothing flashes. A bare `React.lazy` that blanks the main pane fails this criterion. Policy is one `Suspense` boundary per route element, inside the shell (built by `createAppRouter` from the `pending` element).
- Redirects.
  - *Given* `/config?x=1`, *Then* location becomes `/settings?tab=config-files` (query dropped); `/settings/defaults` becomes `/settings`; `/sessions/new` keeps its existing `router.replace("/?new=true")` page logic.
- Guards.
  - *Given* `RouteMeta.guard` for the three guarded paths, *When* `routes.tsx` wraps those routes in the `FeatureGuard` introduced in PR 1 (Story 1.7; not re-implemented here), *Then* `flags.backlog === false` after load sends `/backlog/board` to `/` (replace) after rendering `null` while loading, and `/changelog` and `/settings/backlog-sources` behave per Story 1.4.
- Unknown path (D1) and the not-found view (UX G3).
  - *Given* `/nonexistent`, *When* rendered, *Then* the not-found view is shown inside the shell, the URL is unchanged and no redirect occurs; the view has an `<h1>` "Page not found", the unknown path echoed in text, a link to `/` ("Go to Sessions") and a "Go back" button calling `history.back()`, hidden when `history.length` is 1 so that only the home link shows (design/ux.md S8); `document.title` is `NOT_FOUND_META.title` (Story 1.2) and the `RouteAnnouncer` announces it (Story 4.5); status stays 200 (Go unchanged).
- `RoutePending` respects reduced motion and does not double-announce (triad review, UX; UX-60).
  - *Given* `emulateMedia({reducedMotion: "reduce"})` and a route whose chunk is held pending beyond 150 ms, *When* the skeleton renders, *Then* every element of it has computed `animation-name: none` and no `transition` (any shimmer or pulse of the reused skeleton primitive is wrapped in `@media (prefers-reduced-motion: no-preference)` or disabled under `reduce`; if the primitive has no animation the assertion passes vacuously and the test says so). *Given* the same delayed navigation with a `MutationObserver` on every `[role=status], [role=alert], [aria-live]` node, *Then* the skeleton contains none of them (it signals busyness with `aria-busy="true"` on the layout `<main>` only) and exactly one live-region text change occurs, the `RouteAnnouncer`'s title announcement after the route commits, so a screen reader hears the navigation once, not "loading" and then the title.
- Next is unaffected.
  - *Given* `routes.tsx`, `NotFound.tsx`, `FeatureGuard.tsx` living in `src/app/`, *When* `make web-build` (Next) runs, *Then* the exported route list is the same 42 paths (Next ignores non-`page.tsx` files).
**Files**: `web-app/src/app/routes.tsx` (new), `web-app/src/lib/nav/paths.ts`, `web-app/src/app/NotFound.tsx` (new), `web-app/src/app/RoutePending.tsx` (new), `web-app/src/app/routes.test.tsx` (new)

##### Task 4.3a: Test first, exhaustiveness + key-set equality + titles + redirects + guards (M)
##### Task 4.3b: Route table; `FeatureGuard` wiring from `RouteMeta` (M)
##### Task 4.3c: `NotFound` view with the content criteria above (S)
##### Task 4.3d: `RoutePending` and the Suspense policy (S)

#### Story 4.4: `main.tsx` mounts `RootShell` inside the router
**As a** developer, **I want** the entry to mirror Next's persistent root layout, **so that** providers, terminals and Redux state survive navigation, including when a page fails.
**Acceptance Criteria**:
- Route structure keeps the shell on failure (architecture review; UX G2).
  - *Given* `main.tsx` calling `createAppRouter({shell, errorElement: RouteError, pending: RoutePending, routes, notFound, titleFor})`, which builds (inside `lib/nav/entry.tsx`, the only file that touches the router API) the tree `[{ element: <Shell><Outlet/></Shell>, children: [{ errorElement, children: [...42 routes + "*"] }] }]`, i.e. a **pathless layout route** that renders the shell with **no** `errorElement`, and a **pathless child** carrying the `errorElement`, *When* a route's lazy `import()` rejects (after the `StaleChunkReload` second failure), *Then* `AuthProvider`, `CockpitShell`, the skip link, the nav and the notification panel are still present and the existing `ErrorBoundary` UI renders inside `<main>` with a **Retry** button (a full `location.reload()` when the error is a chunk-load error, because a rejected dynamic-import promise is memoised and a re-render would not re-request it; otherwise the boundary's existing `onRetry`; the new-version banner of Story 2.7 is a different control with its own Reload button; design/ux.md S11); the test asserts the skip link and nav exist after the failure.
- Persistence.
  - *Given* `TerminalPoolProvider` mounted in `RootShell`, *When* navigating `/` to `/history` and back, *Then* the provider instance is the same (mount effect ran once), and `usePathname` consumers (`PageViewTracker`, `NavigationProvider`) work because `RootShell` is inside `RouterProvider`.
- StrictMode.
  - *Given* dev mode, *When* a page effect with a fire-once ref mounts, *Then* it runs twice under `<React.StrictMode>` exactly as under `reactStrictMode:true`.
- Pending UI.
  - *Given* Story 4.3's `RoutePending`, *Then* it renders inside the pathless child, so a slow chunk shows the skeleton in `<main>` with the shell intact (UX G1).
- Title.
  - *Given* the root renders `<title>{matchTitle}</title>` from `RouteMeta`, *When* navigating between routes, *Then* `document.title` follows `RouteMeta` on every client navigation (or the `useEffect` fallback chosen in S4).
- Search-only change keeps the title (UX-05).
  - *Given* a `?tab=` change on `/unfinished`, *Then* `document.title` is unchanged.
**Files**: `web-app/src/main.tsx` (new), `web-app/src/lib/nav/entry.tsx`, `web-app/src/app/routes.tsx`, `web-app/src/app/RootShell.tsx`, `web-app/src/app/RouteError.tsx` (new), `web-app/src/main.test.tsx` (new)

##### Task 4.4a: Test first, persistence, error-inside-shell and pending tests (M)
##### Task 4.4b: `main.tsx` calling `createAppRouter`, title (S)
##### Task 4.4c: `RouteError` wrapping the existing `ErrorBoundary` UI with Retry (S)

#### Story 4.5: `RouteAnnouncer`, `FocusPolicy` and skip-link safety
**As a** screen-reader or keyboard user, **I want** route changes announced and focus moved, **so that** the swap does not silently remove what Next's announcer gave me.
**Acceptance Criteria**:
- Announcement.
  - *Given* a navigation from `/` to `/history`, *When* it completes, *Then* a `role="status"` `aria-live="polite"` element's text becomes `History - Stapler Squad`; it is not `role="alert"`; it does not announce on first load or search-only changes.
- Announcer edge cases (UX G4).
  - *Given* two consecutive navigations whose titles are identical (for example `/` -> `/history` -> `/` -> `/history`... or a route re-entered with a different param), *When* the second completes, *Then* the live region is cleared and then set (clear-then-set in consecutive tasks), so assistive tech re-announces it. *Given* a browser Back/Forward (`popstate`), *Then* the title is announced like any other navigation. *Given* a `<Redirect>` (for example `/config` -> `/settings`), *Then* exactly one announcement occurs, for the final location.
- Decoupled from the route table (architecture review).
  - *Given* `RouteAnnouncer.tsx`, *Then* it reads `document.title` after the navigation commit (the title is set before the announcer reads it, ordering verified by S4's result) and imports nothing from `src/app/**` (an ESLint `boundaries` check in lint passes); a `titleFor` prop supplied by `app` is the permitted alternative.
- Focus (D5, bounded by `FOCUS_ON_NAVIGATE`).
  - *Given* focus on the History nav link, *When* the pathname changes, *Then* the **layout** `<main id="main-content" tabindex="-1">` receives focus (UX G5: ten pages render a nested `<main id="main-content">`, so the target is the `mainRef` that `createAppRouter` passes to the shell and `RootShell` attaches to the layout `<main>`; there is no DOM query, so a page's nested one is never chosen; the hook is `useFocusOnNavigate(mainRef)`); *Given* only `?tab=` or the hash changes (UX-07), *Then* focus does not move and nothing is announced; *Given* an element with `autofocus`, or any input/textarea/select/contenteditable already focused inside `<main>` after the route commits, *Then* focus is NOT stolen (checked after the commit, so a page that autofocuses its own field wins); *Given* the first load, *Then* focus does not move.
- Skip link.
  - *Given* `<a href="#main-content" class="skip-link">`, *When* Tab then Enter, *Then* `#main-content` has focus and no router navigation happened.
- No visible focus ring on the programmatically focused layout `main` (triad review, UX; decision D8, UX-58).
  - *Decision.* The layout `main#main-content[tabindex="-1"]` gets `outline: none` for `:focus` and `:focus-visible`, scoped by the one class `RootShell` puts on that element (never a global `:focus` rule). Justification: (1) Next focuses nothing on navigation, so today no ring appears; a pane-sized ring after every click would be a new visible regression in a migration whose claim is parity; (2) the element is not interactive and not in the Tab order (`tabindex=-1`), so WCAG 2.4.7 (Focus Visible) is about the controls inside it, which keep their rings; (3) the move is for reading position and assistive-technology context, and the change is conveyed by the title and the polite announcer, not by a ring; (4) the cost is that a sighted keyboard user loses a ring on the skip-link target; Next has the same behaviour today (Task 7.2a records the computed outline on `main` after the skip link on Next to prove it), so parity holds.
  - *Given* a navigation that focuses the layout `main`, *When* the computed style is read, *Then* `outline-style` is `none` (or `outline-width` is `0px`) and no `box-shadow` is applied as a substitute; *Given* the stylesheet, *Then* the only rule removing the outline targets that class (a Jest test over the `RootShell` style, and a Playwright check that a nav link focused by Tab keeps a non-`none` outline); *Given* ParityDiff's forced `:focus-visible` on the first 5 focusable elements per route (Story 7.9), *Then* those still match Next, so no other ring regressed.
**Files**: `web-app/src/lib/nav/RouteAnnouncer.tsx` (new), `web-app/src/lib/nav/useFocusOnNavigate.ts` (new), `web-app/src/lib/nav/policy.ts` (new, `FOCUS_ON_NAVIGATE`), `web-app/src/app/RootShell.tsx`

##### Task 4.5a: Test first (S)
##### Task 4.5b: Announcer (clear-then-set, `document.title` read) mounted by `createAppRouter` inside the shell (S)
##### Task 4.5c: Focus hook with the autofocus/occupied-focus exemptions and the `mainRef` target (S)

#### Story 4.6: Scroll policy and History interplay
**As a** user, **I want** back/forward and scroll to behave as before, **so that** navigation feels the same.
**Acceptance Criteria**:
- Scroll.
  - *Given* S4's recorded scroll container, *When* navigating forward to a new pathname, *Then* the container scrolls to top; *When* going back, *Then* position is restored; *When* only search params change (`router.replace(url,{resetScroll:false})`), *Then* position is unchanged.
- Tray entry.
  - *Given* the mobile notification tray opened (`pushTrayEntry`), *When* the browser Back is pressed, *Then* the tray closes and the route does not change; the RR `popstate` handler does not push or drop the marker entry.
**Files**: `web-app/src/lib/nav/impl.router.tsx`, `web-app/src/lib/nav/ScrollPolicy.tsx` (new), `web-app/src/lib/hooks/useTrayDismissal.ts` (consumes `pushTrayEntry`)

##### Task 4.6a: Test first (S)
##### Task 4.6b: Implement per S4 verdict (S)

#### Story 4.7: `NavContract` suite across implementations
**As a** developer, **I want** one suite run against every `NavImpl`, **so that** drift between `impl.next.tsx` and `impl.router.tsx` is a red test.
**Acceptance Criteria**:
- *Given* `navContract(impl)` covering normalised pathname, stable search params, push vs replace, `resetScroll:false`, redirect, `ReplaceSearch`, *When* run for `impl.next.tsx`, `impl.router.tsx` and `impl.memory.tsx`, *Then* all three pass the same ~20 cases. For `impl.next.tsx` the test imports the **real** wrapper (`jest.requireActual` of the file, bypassing the global `@/lib/nav` stub; an assertion in the test fails if the stub is in the import graph) and mocks only the external `next/navigation` module; this proves the wrapper's own logic (normalisation, option mapping, `ReplaceSearch`), not Next itself. Parity with real Next behaviour is evidenced only by the Next-baselined e2e specs (Stories 7.1-7.4), and the story says so in its header comment.
**Files**: `web-app/src/lib/nav/__tests__/nav-contract.ts` (new), `web-app/src/lib/nav/__tests__/impl.*.test.tsx` (new)

##### Task 4.7a: Extract shared cases from 4.2a into the contract (S)

---

## Phase 5: Bun build, dev server and the `WEB_BUILD` seam (PRs 5a and 5b)

**PR split.** **5a** = Stories 5.1-5.5, 5.7, 5.9-5.11 (`build/`, Makefile seam, `verify-output`, CI `web-build-smoke-next`/`web-build-smoke-bun` and `web-build-gate`); **5b** = Stories 5.6 and 5.8 (dev server, dev-stack, Playwright dev-mode harness). 5b depends on 5a's plugin and `package.json` seam; `next dev` stays untouched, so reverting 5b alone is clean. The `web-build-smoke-bun` job runs and reports from its first appearance in 5a (it is deterministic and carries the lockout walk) and is not `continue-on-error`, but only the aggregate `web-build-gate` is ever required; `web-build-smoke-bun` joins that gate by being added to the repository variable `WEB_BUILD_GATE_MEMBERS` after 10 consecutive green runs on `main` or a recorded owner flip (decision 10).

### Epic 5.1: `web-app/build/` and `make web-build` in Bun mode
**Goal**: `WEB_BUILD=bun make web-build` writes a verified `web-app/out/`; the default stays `next`. Preconditions: gates S1-S3 PASS (or design B), Epics 3 and 4 merged.

#### Story 5.1: `VePlugin` with dependency-aware invalidation
**As a** developer, **I want** one vanilla-extract Bun plugin, **so that** build, dev and theme-map share identifiers and the cache.
**Acceptance Criteria**:
- Correct output.
  - *Given* a `.css.ts` using `style`, `createTheme`, `globalStyle` and `keyframes`, *When* the plugin loads it with `identOption:"short"` (prod) and `"debug"` (dev), *Then* the returned JS exports the class map and the virtual CSS contains the rules.
- Invalidation (F7).
  - *Given* `Foo.css.ts` importing `tokens.css.ts`, *When* the **content hash** of `tokens.css.ts` changes (key = path + hash of the file and of its VE dependency closure; mtime and size are not the key, since mtime is coarse after checkout and CI restore) and `Foo.css.ts` is loaded again, *Then* `compile` re-runs (spied) and the new value appears; *Given* only the mtime changes (touch), *Then* `compile` is not re-run; *Given* nothing changed, *Then* `compile` is not re-run.
- Dependants of an edited token file (architecture review).
  - *Given* Bun's `onLoad` has no `addWatchFile` and a memo only makes a *re-requested* dependant correct, *When* S2's result (Story 0.3) shows Bun does not re-request dependants of an edited `*.css.ts`, *Then* the dev server (Story 5.6) performs a full page reload on any `*.css.ts` change, and this fallback is the documented behaviour instead of "memo mandatory".
- Bounded cache.
  - *Given* the dev server running, *When* more than 2,000 distinct `.css.ts` results are cached (the repo has about 337 today), *Then* least-recently-used entries are evicted, which keeps the S2 1.5 GB RSS criterion attainable.
- Canary through the plugin.
  - *Given* the Story 3.4 canary, *When* it is loaded through the plugin (Bun-side script), *Then* it returns the same non-empty class name and CSS.
- Fail loudly.
  - *Given* a `.css.ts` that throws, *When* loaded, *Then* the build throws an error containing the file path and no CSS is emitted for it.
**Files**: `web-app/build/plugins/vanilla-extract.ts` (new), `web-app/build/plugins/__tests__/vanilla-extract.test.ts` and `web-app/build/plugins/__tests__/bun/vanilla-extract.bun.ts` (new; the `.test.ts` runs in Node and drives the Bun-side script through `runInBun`, collected by Story 3.0's project)

##### Task 5.1a: Test first, using fixtures under `build/plugins/__tests__/fixtures/` (M)
##### Task 5.1b: Port spike plugin from `spikes/bun-spike/` (without the keyframes stripper), content-hash memo, factory (M)
##### Task 5.1c: Bounded LRU cache and the full-reload fallback flag consumed by Story 5.6 (S)

#### Story 5.2: Build-seam resolver plugin
**As a** developer, **I want** the Bun build to select the Bun impls and to refuse `next`, **so that** a half-migrated import cannot ship.
**Acceptance Criteria**:
- Alias.
  - *Given* an import of `@/lib/nav/impl` and `@/lib/lazy/impl`, *When* resolved in the Bun build, *Then* they resolve to `impl.router.tsx` and `impl.bun.tsx`.
- Ban.
  - *Given* a fixture file importing `next/navigation`, *When* bundled, *Then* the build fails naming the importer file (F1); `next` is also refused when imported by a dependency.
- Resolution parity.
  - *Given* `src/gen/**` importing `./x.js` that exists as `x.ts`, *When* bundled, *Then* it resolves.
**Files**: `web-app/build/plugins/build-seam.ts` (new), `web-app/build/plugins/__tests__/build-seam.test.ts` and `bun/build-seam.bun.ts` (new; `Bun.build` runs in the Bun-side script)

##### Task 5.2a: Test first (S)
##### Task 5.2b: Implement `onResolve` alias and ban (S)

#### Story 5.3: HTML shell, viewport, fonts
**As a** mobile user, **I want** the viewport and fonts exactly as today, **so that** safe-area insets and typography do not change.
**Acceptance Criteria**:
- Head content.
  - *Given* `web-app/index.html`, *When* the build runs, *Then* the emitted HTML has `<html lang="en">`, `<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=5, viewport-fit=cover">`, default `<title>Stapler Squad Sessions</title>`, the root `description` meta, and no `rel="manifest"` (D4).
- Fonts.
  - *Given* `src/app/fonts.css` (plain CSS) with `@font-face` for JetBrains Mono (normal and italic), Rajdhani 400/500/600/700, Cinzel `400 700` (one block), Inter, `font-display: swap`, and `:root{--font-jetbrains-mono:...;--font-rajdhani:...;--font-cinzel:...;--font-inter:...}`, *When* built, *Then* the 4 variable names are defined, woff2 URLs point into `/_next/static/media/`, and a `<link rel="preload" as="font" crossorigin>` exists for the primary UI font.
- Fallbacks.
  - *Given* each family, *Then* a `local()`/system fallback stack is present to limit layout shift.
- Theme CSS blocks first paint (UX G10).
  - *Given* the emitted HTML, *Then* the theme/global stylesheet is a render-blocking `<link rel="stylesheet">` in `<head>` (not injected by a module script), so `ThemeBoot` classes always have CSS behind them at first paint.
**Files**: `web-app/index.html` (new), `web-app/src/app/fonts.css` (new), `web-app/src/app/fonts.ts` (kept for Next), `web-app/build/__tests__/html-shell.test.ts` and `bun/html-shell.bun.ts` (new; the build runs in Bun, the HTML/CSS parse in the Node test)

##### Task 5.3a: Test first, parse emitted HTML/CSS (S)
##### Task 5.3b: `index.html`, `fonts.css`, wire into entry (S)

#### Story 5.4: `ThemeMap` and `ThemeBoot`, generated from the same VE cache
**As a** user of a dark theme, **I want** no flash of the clean theme, **so that** first paint is correct.
**Acceptance Criteria**:
- Map generation.
  - *Given* `src/styles/theme.css.ts`, *When* `theme-map.ts` runs with the build's plugin instance, *Then* it yields 6 name-to-class entries (`matrix`, `cyberpunk77`, `wh40k`, `clean`, `light`, `dark`) and `cleanTheme`.
- Drift guard.
  - *Given* the emitted boot script and CSS, *When* `verify-output` runs, *Then* every class string in the boot script appears in the emitted CSS, else the build fails.
- One script builder, three consumers (architecture review).
  - *Given* `web-app/src/styles/theme-boot-script.ts` exporting `buildThemeBootScript(map: ThemeMap): string` (a pure string builder: it imports nothing from `theme.css.ts`, so `build/` never has to evaluate VE outside the plugin), *When* Next `layout.tsx` (given the map built from `theme.css.ts` exports) and `build/html.ts` (given `ThemeMap` from `theme-map.ts`) both call it, *Then* both inline byte-identical script text for the same map; the dev `theme-boot.ts` module script (accepts a flash) calls the same function.
- Placeholder survives minification.
  - *Given* the placeholder form recorded by Task 0.2d (default order: `<!--THEME_BOOT-->`, else `<script id="theme-boot"></script>`, else `<meta name="theme-boot">`), *When* the HTML is built with `minify:true` and post-processed by `html.ts`, *Then* the placeholder is replaced exactly once and none remains; a test fails if the minifier strips it.
- Storage unavailable (UX G10).
  - *Given* `localStorage` throwing on access, *When* the boot script runs, *Then* it catches, applies the `clean` class and does not throw (page usable).
- Behaviour.
  - *Given* `localStorage['stapler-theme']='matrix'`, *When* the built page is parsed with the boot script run synchronously in `<head>` (not deferred, not a module), *Then* `<html>` has the `matrix` class and font classes are untouched; *Given* `stapler-theme-custom` with `custom:42`, *Then* its class and CSS variables are applied; *Given* a stale class in storage, *Then* it degrades to `clean`.
**Files**: `web-app/build/theme-map.ts` (new), `web-app/build/html.ts` (new), `web-app/src/styles/theme-boot-script.ts` (new), `web-app/src/app/layout.tsx`, `web-app/build/__tests__/theme-boot.test.ts` (new; jsdom boot-script run in Node plus a Bun-side `bun/theme-map.bun.ts`), `web-app/src/styles/theme.css.ts` (read only)

##### Task 5.4a: Test first, jsdom run of the boot script (S)
##### Task 5.4b: `theme-map.ts` (shared plugin instance) and placeholder injection (S)
##### Task 5.4c: Extract the boot script text from `layout.tsx` into `theme-boot-script.ts`, used by Next `layout.tsx`, `html.ts` and the dev module (S)

#### Story 5.5: `build/build.ts`, the stage pipeline
**As a** developer, **I want** one command producing a verified `out/`, **so that** `make web-build` is fast and deterministic.
**Acceptance Criteria**:
- Defines.
  - *Given* env with `NEXT_PUBLIC_API_URL` unset and `NEXT_PUBLIC_RECONNECT_V2=1`, `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING`, `NEXT_PUBLIC_OTEL_ENABLED`, `NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` set, *When* built, *Then* all 5 appear as inlined literals (`undefined` for unset), `process.env.NODE_ENV` is `"production"` for **every** `build.ts` invocation, with or without `WEB_MINIFY=0` (only `dev.ts` sets `"development"`; ADR-004 Decision 1: today's "development-mode" Next build already runs with `NODE_ENV=production`, so `Providers.tsx` picks `HttpAnalyticsProvider`, and the Bun cutover build must too), and no `process.env` token remains in `out/**/*.js`. The var list is derived from `rg -oh "NEXT_PUBLIC_[A-Z0-9_]+" src | sort -u` in a unit test, so a sixth var fails the test.
- HTML lands at the root (adversarial re-review B3).
  - *Given* the layout chosen by Story 0.2 (A': the `html` stage locates the emitted `.html` in `result.outputs`, post-processes it and writes `out/index.html`, then deletes the original under `_next/static/chunks/`; or B: script entry plus `html.ts`), *When* built, *Then* `out/index.html` exists, it is the only `*.html` under `out/`, and a stage test that mocks the bundle output with the HTML at `_next/static/chunks/index-<hash>.html` asserts the move.
- Atomic output.
  - *Given* a build that fails at the bundle stage, *When* it exits, *Then* the previous `web-app/out/` is intact and `.out-tmp-*` is removed; two concurrent builds with different PIDs both succeed.
- Public assets.
  - *Given* `public/push-sw.js`, `push-sw-logic.js`, `manifest.json`, `icons/`, `docs/*.md`, `logo.svg`, *When* built, *Then* all exist at their exact root URLs with unhashed names.
- Toggles and minify default (ADR-004; validation A3).
  - *Given* `WEB_MINIFY=0`, *When* built, *Then* output is unminified and `process.env.NODE_ENV` is still `"production"`; *Given* `WEB_MINIFY` unset, *Then* the code default is minified (`stats.minify === true`, asserted in Task 5.5a, and a `verify-output` rule fails a `minify:false` build unless `WEB_MINIFY=0` was set explicitly, recorded as `stats.minifyExplicit`). There is no `WEB_BUILD_MODE` and no `--mode` flag: `WEB_MINIFY` alone replaces the old `NEXT_BUILD_MODE` distinction (the Makefile passes `WEB_MINIFY=0` until PR 8b), which also removes the name collision with the `WebBuildMode` glossary term.
- Patch guard is the first stage (pre-mortem failure mode 7).
  - *Given* the patch absent or drifted, *When* `build.ts` starts, *Then* stage `guard` (Story 3.4's `verify-eval-patch`) fails first, naming `patches/eval@0.1.8.patch`.
- Offline build (validation A7; optional).
  - *Given* the fixture app and the build run with the network unreachable (for example under `unshare -n`), *When* built, *Then* it succeeds (fonts are vendored, REQ-C7); if Bun's offline behaviour cannot be isolated on the runner this criterion is dropped and the reason recorded.
- Output locations are pinned (B3).
  - *Given* the bundle stage's `Bun.build` call, *Then* it sets all three of `naming.entry`, `naming.chunk` and `naming.asset` explicitly, each under `_next/static/` (entry and chunk under `_next/static/chunks/`, assets under `_next/static/media/`); a stage unit test fails if any is unset, and `verify-output` (Story 5.9) fails the build if a file lies outside an exempt prefix or the public allowlist, so a Bun upgrade changing a default cannot reintroduce the lockout silently.
- Failures name the file and the stage (UX G7).
  - *Given* a build that fails, *When* it exits non-zero, *Then* the message contains the failing stage name (`guard | clean | theme-map | bundle | html | public | verify | swap`) and, where one exists, the failing source file (for a `.css.ts` error the plugin's path; for a resolver ban the importer).
- Speed (metric).
  - *Given* this host, *When* `bun web-app/build/build.ts` runs warm, *Then* wall time <= 10 s (target ~3 s); CI enforces it through `maxWallMs` (Story 5.9).
**Files**: `web-app/build/build.ts` (new), `web-app/build/__tests__/build.test.ts` and `bun/build.bun.ts` (new; the fixture-app build runs in the Bun-side script, assertions in the Node test), `web-app/build/stages/*.ts` (new)

##### Task 5.5a: Test first, defines/atomic/public tests against a tiny fixture app (M)
##### Task 5.5b: Stage runner (ordered named stages, per-stage timing, stage-named errors) and the `guard`, `clean` and `swap` stages (S)
- Files: `web-app/build/build.ts`, `web-app/build/stages/{guard,clean,swap}.ts`, `web-app/build/stages/runner.ts`. Split from the former single M task that bundled seven stages (triad review); the bundle-side stages are Tasks 5.5e and 5.5f, which depend on this runner.
##### Task 5.5c: Stats emission (stub for 5.9) and the env switches (S)
##### Task 5.5d: Optional offline fixture build (S)
##### Task 5.5e: `theme-map` and `bundle` stages: `naming.entry/chunk/asset` under `_next/static/`, `publicPath:"/"`, `sourcemap:"linked"`, `throw:true`, defines (S)
- Files: `web-app/build/stages/{theme-map,bundle}.ts`. Wires the plugins of Stories 5.1, 5.2 and 5.4; adds no plugin logic.
##### Task 5.5f: `html` stage (A': move the emitted HTML to `out/index.html`; or B: generate it) and `public` stage (S)
- Files: `web-app/build/stages/{html,public}.ts`. Layout A' or B is the one recorded by Story 0.2.

#### Story 5.6: Dev server
**As the** owner, **I want** `bun dev` to hot-reload the real app, **so that** the daily loop is under a second.
**Acceptance Criteria**:
- Serves like prod.
  - *Given* `web-app/build/dev.ts` using `Bun.serve({routes:{"/*": index, ...publicRoutes}, development:{hmr:true, console:true}})` and `bunfig.toml [serve.static] plugins` pointing at the shared plugin module, *When* started with `PORT=62875` (spare port of the CLAUDE.md manual dev block; the Story 6.7 docs add the row), *Then* `GET /backlog?item=x` returns the shell, `GET /push-sw.js` returns the file, and the server is bound to `localhost` only.
- Reload.
  - *Given* a page open, *When* a `.css.ts` and a shared token file are edited, *Then* styles update with no full reload (S2 criteria encoded as a Playwright dev-mode spec).
- Escape hatches.
  - *Given* `WEB_DEV_HMR=0`, *Then* HMR is off and live reload still works; *Given* the process crashes, *Then* `scripts/dev-stack` restarts it (Story 5.8).
- Theme gap documented.
  - *Given* dev mode, *Then* `theme-boot.ts` runs as the first module script and a short flash is documented in `docs/how-to/web-dev-server.md`.
- Dev errors are visible (UX G7).
  - *Given* a deliberate TypeScript/syntax error and a deliberate render-time throw, *When* each is introduced then removed, *Then* an overlay (Bun's `development.console`/error overlay or the app's `ErrorBoundary`) shows the error with the file name and clears when fixed (this is the Playwright encoding of S2's overlay criterion).
- Overlay and build-failure behaviour are asserted in CI, not by hand (triad review, UX; UX-53, UX-55).
  - *Given* the `web-build-smoke-bun` job (Story 5.11), *When* PR 5b merges, *Then* a CI step starts `bun web-app/build/dev.ts` on the spare port and runs `tests/e2e/dev-mode-hmr.spec.ts` (introduce then remove a type error and a render throw; the overlay appears with the file name and clears; `ErrorBoundary` still handles the render throw), and the stage-failure test of Story 5.5 (`build.test.ts`, run by `make test-web-build`) asserts UX-55; both are required-green in that job and neither is a manual check. The step is a workflow edit, so decision 10's protocol applies.
- Port in use (UX G7).
  - *Given* `PORT` already bound, *When* `dev.ts` starts, *Then* it exits non-zero with a message naming the port and the process holding it if discoverable (`ss -ltnp` hint), never a raw stack trace.
- Fallback consumption.
  - *Given* Story 5.1's full-reload flag, *When* a `*.css.ts` file changes and S2 recorded that dependants are not re-requested, *Then* the dev server triggers a full reload; otherwise HMR applies.
**Files**: `web-app/build/dev.ts` (new), `web-app/build/__tests__/dev-fetch.test.ts` and `bun/dev-fetch.bun.ts` (new; Bun-side `fetch` handler checks), `web-app/bunfig.toml`, `web-app/src/theme-boot.ts` (new), `web-app/package.json` (see Story 5.7's script inventory for the `dev` script in dual mode), `docs/how-to/web-dev-server.md` (new)

##### Task 5.6a: Test first, `fetch` handler unit tests for public routes and bind host (S)
##### Task 5.6b: `dev.ts`, bunfig plugin entry, HMR flag, `PORT` honoured (M)
##### Task 5.6c: Playwright dev-mode HMR spec using S2's scripts (S)
##### Task 5.6d: Overlay and port-in-use handling with tests (S)

#### Story 5.7: Makefile seam, tool pins, tsconfig
**As a** developer, **I want** `WEB_BUILD` to select the build safely, **so that** flipping modes never reuses stale output.
**Acceptance Criteria**:
- Stamp.
  - *Given* `make web-build` ran in `next` mode, *When* run again with `WEB_BUILD=bun` without any file change, *Then* `web-app/out` is rebuilt (stamp changed) and `server/web/dist` is recopied; running again unchanged is a no-op.
- Mode-aware guards.
  - *Given* `WEB_BUILD=bun`, *Then* the `test -d web-app/node_modules/next` guard is skipped and `web-app/build/** web-app/bunfig.toml patches/**` are prerequisites (replacing `next.config.ts`).
- Pins.
  - *Given* `.tool-versions` and `.bun-version` (created in Story 3.0 with a provisional value) and `packageManager`, *Then* all three name the Bun version chosen in the GateRecord (this story edits `.bun-version` if the GateRecord pin differs); `make ensure-tools` installs it.
- tsc covers both impls.
  - *Given* `tsconfig.json` (alias `@/lib/nav/impl` -> `impl.next.tsx`, `@/lib/lazy/impl` -> `impl.next.tsx`) and `tsconfig.bun.json` (extends the base with `jsx: "react-jsx"`, `types: ["bun"]` via `@types/bun`, and `include` covering `build/**` and the Bun entry, which the base `tsconfig.json` excludes since Story 3.0; aliases -> `impl.router.tsx` and `impl.bun.tsx`), *When* `tsc -p` runs for each, *Then* both pass **and each config also includes the impl files it does not select** (via `include`), so the unselected impl is still type-checked against `impl.types.ts` (the `satisfies NavImpl` assertions in each impl file make a mismatch a compile error); a test seeds a signature mismatch in the unselected impl and asserts `tsc -p` fails.
  - *Given* Jest, *Then* `jest.config.js` needs no `moduleNameMapper` entry for `@/lib/nav/impl` for most tests (the global stub covers them) but the contract tests import impl files by relative path; Task 5.7c records this in the PR.
- Cutover default is unminified.
  - *Given* `Makefile`, *Then* `WEB_MINIFY ?= 0` is set explicitly with a comment pointing at Story 8.6 (which deletes it), and `NODE_ENV` is not a Makefile variable.
- One `WebBuildMode` parser, one owner (consistency C8).
  - *Given* `scripts/dev-stack/web-build-mode.ts` exporting `parseWebBuildMode(env): WebBuildMode`, *When* tested in the `scripts/dev-stack` Jest project, *Then* `next`, `bun` and unset parse and anything else throws naming the allowed values; this story is its only creator, and Stories 5.8 and 7.10 import it.
- Script inventory in dual mode (adversarial review).
  - *Given* the targets and scripts that spawn or build the web app, *Then* each is listed with its Next-mode and Bun-mode behaviour in `docs/how-to/build-web-app.md`, and none is left calling `next` under `WEB_BUILD=bun`: `web-app/package.json` `dev` (line 15, `next dev --port ${PORT:-3001} --hostname localhost`), `build` (line 16, `next build`) and `start` (line 17, `next start`); `Makefile` `web-build` (line 238), `restart-web` (line 247), `restart-web-profile` (line 258), `web-dev` (line 269), `preview` (line 540), `dev-stack` (line 562, text only; behaviour is Story 5.8) and the `web-app/out` rule (line 218). `package.json` gets mode-aware scripts (`dev:next`, `dev:bun`, `dev` dispatching on `WEB_BUILD`, same for `build`/`start`).
**Files**: `scripts/dev-stack/web-build-mode.ts` (new), `Makefile`, `.tool-versions`, `.bun-version` (created in Story 3.0; edited here only if the GateRecord pin differs), `web-app/package.json` (scripts), `web-app/tsconfig.json`, `web-app/tsconfig.bun.json` (new), `docs/how-to/build-web-app.md`, `.gitignore` (`web-app/.out-tmp-*`, `web-app/.build/`, `web-app/.web-build-mode`)

##### Task 5.7a: Test first, a `make -n` based test script for the stamp logic (S)
##### Task 5.7b: Makefile edits (retry wrapper only in next mode) and pins (S)
##### Task 5.7c: tsconfig variant including both impls, plus the mismatch test (XS)
##### Task 5.7d: Script inventory and mode-aware `package.json` scripts / Makefile targets (S)
##### Task 5.7e: `parseWebBuildMode` with its test (XS)

#### Story 5.8: Dev stack and Playwright dev-mode harness
**As an** agent running the dev stack, **I want** it to start the Bun dev server, **so that** `make dev-stack` still works.
**Acceptance Criteria**:
- *Given* `scripts/dev-stack/launch.ts` spawning `next dev`, *When* `WEB_BUILD=bun`, *Then* it spawns `bun web-app/build/dev.ts` with `PORT=<free port>` and `NEXT_PUBLIC_API_URL`, the backend still gets `STAPLER_SQUAD_EXTRA_ORIGINS=http://localhost:<port>`, and readiness (any HTTP response) is detected; its unit tests (`launch.test.ts`, `ports.test.ts`) pass with a mocked Bun command.
- *Given* `tests/e2e/playwright.dev-mode.config.ts` and `global-setup.dev-mode.ts`, *Then* they use the same command, and `web-app/playwright.config.ts` `webServer.command` no longer hard-codes `npx next dev` in Bun mode (and its use is confirmed or the file is removed). `launch.ts`, `test-server.ts` and `tests/e2e/helpers/web-build.ts` import the single `parseWebBuildMode(env)` created in Story 5.7 (no second parser).
**Files**: `scripts/dev-stack/launch.ts`, `scripts/dev-stack/launch.test.ts`, `tests/e2e/global-setup.dev-mode.ts`, `tests/e2e/playwright.dev-mode.config.ts`, `web-app/playwright.config.ts`, `Makefile` (`dev-stack` text)

##### Task 5.8a: Test first, update `launch.test.ts` expectations (S)
##### Task 5.8b: Implement mode switch and supervisor restart (S)
##### Task 5.8c: Playwright configs (XS)

#### Story 5.9: `verify-output.ts`, `budgets.json` and `BuildStats`
**As the** owner, **I want** the build to fail on structural errors and budget overruns, **so that** a bad bundle never reaches `server/web/dist`.
**Acceptance Criteria**:
- Structure.
  - *Given* a built `out/`, *When* `verify-output` runs, *Then* it passes only if `index.html` exists, every asset URL in it exists on disk under `/_next/static/`, **every file under `out/` is under a prefix in `exemptPrefixes` or on the public allowlist `server/middleware/testdata/public-allowlist.json`** (the same walk as Story 6.8, run here so a bad bundle never reaches `server/web/dist`; this story CREATES the allowlist, see below), no `*.js` contains an import of `next`, the ThemeBoot classes exist in the CSS, and `.build/stats.json` is not inside `out/`; **and every URL string beginning with `/` found in the JS closure of the `/login` route's own entry (string-literal `import()`, `fetch`, `src` and `href` values) resolves under an exempt prefix or the allowlist** (pre-mortem failure mode 10: a chunk requested only by the remote TLS WebAuthn flow must not fall outside `exemptPrefixes`). A `process.env` text hit in `*.js` is a **warning with an allowlist** (guarded third-party references, for example inside `@bufbuild/protobuf`, are expected possible); the hard gate for that class is the runtime check in a Playwright smoke (no `process is not defined`, Story 5.5/8.1).
- The public allowlist exists before its first consumer (adversarial re-review: it was created in Story 6.8, PR 6b, but needed by this story in PR 5a and by Story 7.13b in PR 7b).
  - *Given* `server/middleware/testdata/public-allowlist.json` (a JSON array of `{path, reason}`, created here with one reasoned entry per `web-app/public/` file that is neither under an exempt prefix nor requested by `/login`: `push-sw.js`, `push-sw-logic.js`, `manifest.json`, `icons/*`, `logo.svg`, `docs/*.md`, derived from `ls web-app/public` in a unit test so a new public file without an entry fails), *When* `verify-output` runs on a fixture tree containing `push-sw.js`, *Then* it passes with the entry and fails without it, and an entry with an empty `reason` fails the schema check (the schema check is a plain Node function shared with Story 6.8's Go test through the JSON shape, and is tested in `verify-output.test.ts`). Stories 6.8 and 7.13b consume the file and do not create it.
- Budgets (ADR-005).
  - *Given* `budgets.json` with Total JS, Initial JS, Largest chunk set from the S3 record (Total JS cap 6,349,052 B; Initial JS ceiling = the measured Bun value x 1.05, which must not exceed the largest Next per-route first-load JS from Task 0.4d x 1.10; if it does, the story records AMBER with both numbers and the owner picks, it does not stall; each number carries a `source` field naming the measurement), *When* a fixture output exceeds each in turn, *Then* the build fails naming the budget, bytes and limit.
- Stats.
  - *Given* a build, *Then* `stats.json` contains `mode`, `minify`, per-stage `durationMs`, `jsBytes`, `cssBytes`, `chunkCount`, `initialJsBytes`, `largestChunkBytes`, `gzipJsBytes`, `bunVersion`, and (when run under `/usr/bin/time -v` wrapper) `peakRssKb`; CI writes a Markdown table to `$GITHUB_STEP_SUMMARY`.
- Wall-time gate (validation A1; REQ-M1).
  - *Given* `budgets.json` carrying `maxWallMs: 10000`, *When* a fixture `stats.json` has total `durationMs` above it, *Then* `verify-output` (and the CI summary step) fails naming the stat, the value and the limit, and a fixture below it passes; CI retries the build once before failing (host load) and always prints the number.
- Byte budgets apply to the minified build (ADR-004).
  - *Given* the unminified cutover default, *Then* the byte budgets are enforced on a second, minified build made in the same `web-build-smoke-bun` job (`WEB_MINIFY=1`, written outside the embedded `out/`), and the unminified build's sizes are recorded in `stats.json` with the structural checks only.
**Files**: `server/middleware/testdata/public-allowlist.json` (new, created here), `web-app/build/verify-output.ts` (new), `web-app/build/budgets.json` (new), `web-app/build/stats.ts` (new), `web-app/build/__tests__/verify-output.test.ts` (new; pure Node logic over fixture `out/` trees, collected by Story 3.0's project), `.github/workflows/build.yml`

##### Task 5.9a: Test first, fixture outputs, allowlist schema and `ls web-app/public` coverage test (M)
##### Task 5.9b: Verifier, budgets, stats (M)
##### Task 5.9c: Step-summary writer and `ci-duration-trend.yml` series (S)

#### Story 5.10: Differential CSS gates in CI
**As the** owner, **I want** the VE output proven equal to Node's and the Bun CSS compared with Next's, **so that** Bun bundler regressions (#45730) are caught on every Bun bump.
**Acceptance Criteria**:
- Oracle.
  - *Given* `build/oracle/ve-oracle.ts` (from Story 3.2), *When* run in CI nightly and on changes to `patches/**`, `bun.lock`, `.bun-version` or `@vanilla-extract/*`, *Then* every `.css.ts` file (globbed; 337 on 2026-10-19) matches Node byte for byte.
- Census.
  - *Given* a Next build and a Bun build of the same commit, *When* `css-census.ts` compares the resolved custom-property map per theme class (and reports rule counts), *Then* the maps are equal; a Bun version bump that changes a resolved value fails the job, while a pure rule-count change explained by merging only warns.
- Rule-selector order (pre-mortem P1-1).
  - *Given* the shared (non-lazy) CSS file of each build, *When* `css-census.ts` also extracts the ordered rule-selector list per theme class, *Then* the Bun list equals Next's modulo merges that the census explains with a merged-selector map; an unexplained reordering fails the job (a reordering leaves counts equal, so counts alone cannot catch it).
**Files**: `web-app/build/oracle/ve-oracle.ts`, `web-app/build/oracle/css-census.ts` (new), `.github/workflows/build.yml`

##### Task 5.10a: Test first (S)
##### Task 5.10b: CI job wiring (S)
##### Task 5.10c: Rule-selector-order census (promoted from Task 0.4e) (S)

#### Story 5.11: CI jobs `web-build-smoke-next` and `web-build-smoke-bun`, aggregate `web-build-gate`
**As the** owner, **I want** both builds exercised in CI from PR 5a on behind one required check, **so that** drift between them is caught immediately (F2) and a red Bun job can never block merges before it has earned membership.
**Acceptance Criteria**:
- Two separate jobs, no matrix (review R2-B1; decision 10).
  - *Given* `.github/workflows/build.yml`, where `web-build-smoke` is today one job (`build.yml:123`, `make web-build` plus a proto-binding check, `needs: detect-changes`, `if: always() && (...)`), *When* PR 5a lands, *Then* that job is renamed `web-build-smoke-next` with unchanged behaviour and `web-build-smoke-bun` is added beside it with the same `needs`/`if:` guard; the Bun job builds with `WEB_BUILD=bun`, runs `verify-output`, builds both outputs (default unminified plus the minified size build of Story 5.9) and uploads stats; `oven-sh/setup-bun` is pinned by commit SHA and the Bun version is read from `.bun-version`. No `strategy.matrix` is used (a matrix leg has no `needs.<job>.result` of its own, legs cannot `needs` each other, and `fail-fast` would cancel the Next leg when Bun fails). Neither job name is ever named in branch protection.
- Single stable required check (pre-mortem P1-3).
  - *Given* the job `web-build-gate` in the same file, with `if: always()`, `needs:` [`detect-changes`, `web-build-smoke-next`, `web-build-smoke-bun`] (Task 7.10f appends `e2e-next` and `e2e-bun` in the PR that creates them) and one step running `node web-app/scripts/ci/web-build-gate.mjs` with `NEEDS_JSON: ${{ toJSON(needs) }}` and `MEMBERS: ${{ vars.WEB_BUILD_GATE_MEMBERS }}`, *When* a member's result is `failure` or `cancelled`, *Then* the gate fails; `success` and `skipped` pass; non-members are printed to the step summary and never fail the gate; a name in `MEMBERS` that is not in `needs` fails the gate; an unset or empty variable means the default `detect-changes,web-build-smoke-next,e2e-next` (names not yet present in `needs` are ignored only for this default, never for an explicitly set variable).
  - *Given* membership, *Then* it has one mechanism: `gh variable set WEB_BUILD_GATE_MEMBERS --body "<comma list>"`. A Bun job joins after 10 consecutive green runs on `main` or an owner flip recorded in `cutover-gate-results.md`, and leaves by the same command; no workflow edit, PR or branch-protection change is part of joining or leaving. The owner names `web-build-gate`, and only it, as a required check once PR 5a merges: branch protection does not exist on `main` today (`gh api repos/tstapler/stapler-squad/branches/main/protection/required_status_checks` returns 404 "Branch not protected", rulesets empty, 2026-10-19), so this is a first-time setup after the proof of Task 5.11d.
- The gate can fail (review R2-B1; Task 5.11b).
  - *Given* `web-app/scripts/__tests__/web-build-gate.test.mjs` running the script as a child process over fixture `needs` JSON and member lists, *Then* exit status is non-zero for: a member Next job `failure`; a member Next job `cancelled`; a member Bun job `failure` (variable includes it); `detect-changes` `failure` (a default member, so a broken relevance check cannot turn the gate green); a member name absent from `needs`; and zero for: a non-member Bun job `failure` (printed in the summary); a member job `skipped` with the others `success`; all jobs `skipped` (an irrelevant PR); an unset variable with Next and `detect-changes` green and Bun red. A gate that always exits 0 therefore fails five of these cases. A second test parses `build.yml` and fails if `web-build-gate` lacks `if: always()`, has `continue-on-error`, or if any `strategy.matrix` in the file lacks `fail-fast: false`.
- Workflow edits are tested before they merge.
  - *Given* a PR that edits a workflow file, *When* it is opened, *Then* its body links a green `workflow_dispatch` run on a throwaway branch and `actionlint` passes in `make ready`, so a syntax error or a misnamed `needs:` fails locally. `workflow_dispatch` is already in `build.yml:20` and `lint.yml:24`; `benchmark.yml` and `ux-analysis.yml` have none (VERIFIED by grep 2026-10-19), so the one-line trigger commit, merged first because GitHub dispatches only workflows already on the default branch (Task 5.11c re-verifies), covers those two files only.
- A dispatch run cannot prove the required-check behaviour (review R2-C4).
  - *Given* a scratch repository with a reduced copy of the workflow (`detect-changes`, two dummy smoke jobs, the real `web-build-gate` script) and branch protection requiring `web-build-gate`, *When* draft PRs are opened from a throwaway branch, *Then* `gh pr checks` shows the `web-build-gate` context reported and green on (a) a relevant PR, and (b) an irrelevant PR where `detect-changes` skips the smoke jobs, and red with merge blocked on (c) a PR whose member job fails; the three outputs are pasted into `docs/how-to/rollback-web-build.md` (Task 5.11d).
- *Given* the cache steps, *Then* the `node_modules` cache key includes `hashFiles('bun.lock','patches/**')` with no `restore-keys` fallback, and no build-output cache exists for the Bun job.
**Files**: `.github/workflows/build.yml`, `.github/actions/prepare/action.yml`, `web-app/scripts/ci/web-build-gate.mjs` (new), `web-app/scripts/__tests__/web-build-gate.test.mjs` (new), `.github/workflows/benchmark.yml` and `.github/workflows/ux-analysis.yml` (the `workflow_dispatch` trigger only), `docs/how-to/rollback-web-build.md`

##### Task 5.11a: Rename to `web-build-smoke-next`, add `web-build-smoke-bun`, cache keys (S)
##### Task 5.11b: `web-build-gate` job, the `web-build-gate.mjs` script and its fixture tests (member, non-member, skipped, cancelled, failure, unknown member, default members) (S)
##### Task 5.11c: `actionlint` in `make ready`, the `workflow_dispatch` trigger commit for `benchmark.yml` and `ux-analysis.yml`, the throwaway-branch protocol, and the `fail-fast: false` / `if: always()` assertions (S)
##### Task 5.11d: PR-event proof on a scratch repository with branch protection enabled; paste the three `gh pr checks` outputs (S)

---
## Phase 6: Tooling (PRs 6a and 6b)

**PR split.** **6a** = Stories 6.1-6.3 (ESLint: replace `next/core-web-vitals`, flat config, warning gate): no behavioural dependency on the Bun build (it is merged after 5a only because Tasks 5.7b and 6.3b both edit the `Makefile` lint line; the previous "independent" claim contradicted the Makefile chain 5.7 -> 6.3), high risk (`boundaries` and the two `file:` plugins), has its own rollback (revert the PR; `eslint-config-next` stays installed). It lands after 2e, **gates PR 9 but not PR 8**, so a stalled lint conversion cannot hold the cutover. **6b** = Stories 6.4-6.10 (size-limit/budgets, other workflows, Storybook, docs, Go built-asset walk, jscpd/registry, supply chain), delivered as one commit per story so a partial revert is possible by commit.

### Epic 6.1: Lint, budgets, CI, Storybook, docs, Go guard
**Goal**: Every tool that keyed on Next, `.next`, `next lint`, `pnpm-lock.yaml` or `_next/static/chunks/framework-*` works in both modes. `eslint-config-next` stays installed until PR 9 (rollback of the lint path stays possible).

#### Story 6.1: Inventory and replace `next/core-web-vitals`
**As a** developer, **I want** the rules that actually fire preserved without `eslint-config-next`, **so that** hook-dependency and a11y lint do not silently vanish.
**Acceptance Criteria**:
- Inventory recorded.
  - *Given* `eslint --print-config src/app/page.tsx` before the change, *When* the effective rule set from `next/core-web-vitals` is listed, *Then* a table of rules and their severity (react recommended, `react-hooks/rules-of-hooks` error, `react-hooks/exhaustive-deps` warn, 6 `jsx-a11y` warnings, `import/no-anonymous-default-export` warn, `@next/next/*`) is committed to `docs/reference/eslint-rules-from-next.md`.
- Equivalence.
  - *Given* a fixture file with one violation per retained rule, *When* ESLint runs with the new `extends`, *Then* the same 9 rule ids report at the same severities as before; `@next/next/*` ids no longer exist.
- Custom rules untouched.
  - *Given* `boundaries`, `analytics`, `rpc-lifecycle` and the `no-restricted-syntax` selectors, *When* the repo lints, *Then* the set of reported files for rule ids outside the swap is unchanged.
**Files**: `web-app/.eslintrc.json`, `web-app/package.json`, `web-app/src/__fixtures__/eslint-rules/*` (new), `docs/reference/eslint-rules-from-next.md` (new)

##### Task 6.1a: Test first, fixtures and RuleTester run (S)
##### Task 6.1b: Replace `extends` with explicit plugins; pin `eslint-plugin-react`, `-react-hooks`, `-jsx-a11y` exactly (S)

#### Story 6.2: Flat config conversion
**As a** developer, **I want** one `eslint.config.mjs`, **so that** the repo is not left on the deprecated format after `next lint` is gone.
**Acceptance Criteria**:
- *Given* the eslintrc content (boundaries elements, `file:` plugins, the large `no-restricted-syntax` block, the overrides for `**/*.css.ts` and `src/components/**/*.tsx`), *When* converted to flat config on ESLint 9 (`@eslint/js`, `typescript-eslint`, `globals`; no `FlatCompat` in the final state), *Then* `eslint .` reports the same (file, rule id, severity) multiset as before the conversion (diffed by script).
- *Given* `ESLINT_USE_FLAT_CONFIG` unset, *Then* editors and CI pick it up with no env var.
- Finite acceptance (added scope, bounded).
  - *Given* ~1,450 pre-existing warnings (lint.yml comment) and a parser change, *When* the before/after (file, rule id, severity) multisets are diffed, *Then* the story passes if the diff is empty **or** every differing line is classified by the diff script into one of three named classes (parser difference, rule renamed by the plugin upgrade, config file newly linted) and the total differing lines are <= 2% of the baseline total; the classified diff is committed. If this is not reached after one repair wave, the conversion is abandoned, the repo stays on eslintrc via `ESLINT_USE_FLAT_CONFIG=false` under ESLint 9 for the retained period, and a follow-up issue is filed; Story 6.1 and 6.3 still ship. This story never blocks PR 8.
- Boundary rule still fires.
  - *Given* a fixture file under `src/lib/` importing from `src/app/`, *When* ESLint runs with the converted config, *Then* `boundaries` reports it (the `lib` -> `app` disallow the plan leans on).
**Files**: `web-app/eslint.config.mjs` (new), `web-app/.eslintrc.json` (delete), `web-app/package.json`

##### Task 6.2a: Test first, baseline report snapshot script and diff (S)
##### Task 6.2b: Convert and re-diff (M)

#### Story 6.3: Warning gate semantics without `next lint`
**As the** owner, **I want** the "any warning fails the changed-files gate" behaviour preserved, **so that** the lint gate does not weaken.
**Acceptance Criteria**:
- *Given* `.github/workflows/lint.yml` building `--file` args for `npx next lint`, *When* replaced by `eslint --max-warnings 0 <changed files>`, *Then* a PR touching a file with one `exhaustive-deps` warning fails the job and a PR touching clean files passes; the comment that says "`next lint` exits 1 on ANY warning" is rewritten.
- *Given* `web-app/scripts/lint-affected.sh:50`, `Makefile:1021` and `package.json` `lint`, *Then* all use `eslint` (the whole-repo run keeps its existing warning tolerance, which is stated in the script header).
**Files**: `.github/workflows/lint.yml`, `web-app/scripts/lint-affected.sh`, `Makefile`, `web-app/package.json`

##### Task 6.3a: Test first, shell test with two fixture files (S)
##### Task 6.3b: Edit four call sites and comments (S)

#### Story 6.4: `size-limit` and budgets per ADR-005
**As the** owner, **I want** budgets that match Bun's output, **so that** the check cannot pass vacuously.
**Acceptance Criteria**:
- *Given* `web-app/.size-limit.json` and the duplicate `size-limit` key in `web-app/package.json`, *When* the `package.json` key is deleted and `web-app/.size-limit.json` (the survivor, as ADR-003 and Story 9.4 already name it) globs `out/_next/static/**/*.js` (ignoring maps) with its limit read from `budgets.json`, *Then* `npx size-limit` passes on a good build (the minified size build of Story 5.9; the unminified cutover output is not measured against the Total JS limit).
- *Given* a deliberately wrong glob matching zero files, *When* `size-limit` runs, *Then* the job fails (test asserts failure, not pass).
- *Given* Initial JS and Largest chunk budgets, *Then* they are enforced by `verify-output` (Story 5.9) and the PR comment (if any) shows all three numbers.
- Relaxation is recorded, not implied.
  - *Given* Task 0.4d's result for today's 5 MB gate, *When* `budgets.json` sets Total JS at 6,349,052 B, *Then* ADR-005 (in `project_plans/web-app-bun-build/decisions/`) gains an "Amendments" line stating whether this is a relaxation of a passing gate or the correction of an already-failing one, with both numbers and the owner's Story 0.6 decision.
**Files**: `web-app/.size-limit.json` (survives; the `package.json` key is deleted), `web-app/build/budgets.json`, `web-app/scripts/__tests__/size-limit-zero-match.test.mjs` (new; collected by Story 3.0's project), `project_plans/web-app-bun-build/decisions/ADR-005-size-limit-budget-shape.md` (amendment; the ADRs are not in `docs/adr/`, see the header note)

##### Task 6.4a: Test first, zero-match failure test (XS)
##### Task 6.4b: Delete duplicate, re-glob, wire to `budgets.json` (S)

#### Story 6.5: Remaining CI workflows
**As the** owner, **I want** all workflows mode-aware, **so that** no job silently builds the wrong thing.
**Acceptance Criteria**:
- Benchmarks and caches.
  - *Given* `benchmark.yml` (lines ~285, 345, 531, 609, 620) running `pnpm run build` and caching `.next/cache`, *When* updated, *Then* they run `bun run build` for `WEB_BUILD` and keep `.next/cache` only for the `next` mode; `.github/actions/prepare/action.yml:59-66` likewise.
- UX analysis trigger.
  - *Given* `ux-analysis.yml` `paths:` containing only `web-app/src/**`, *When* widened to `web-app/build/**`, `web-app/package.json`, `bun.lock`, `patches/**`, `web-app/index.html`, `web-app/bunfig.toml`, `Makefile`, `server/web/**`, `server/middleware/static.go`, `server/middleware/auth.go`, *Then* a PR changing only `web-app/build/build.ts` triggers Axe and Lighthouse.
- Lighthouse config (added scope, bounded).
  - *Given* two configs (`web-app/lighthouserc.json` using `npx serve@14 out`, and `tests/e2e/lighthouse.config.js` against the Go server), *When* one is chosen, *Then* the Go-served one is kept, `serve@14` usage is removed, and `/` and `/review-queue` plus one nested deep link (`/settings/jules`) are audited. Acceptance is finite: the single config runs once in CI and produces a report; thresholds stay as today (warn when score < 70, per CLAUDE.md); no new blocking assertion is introduced. `ux-analysis.yml`'s path widening is accepted when the `yq` path assertion passes.
- Release and Docker paths (pre-mortem failure mode 7).
  - *Given* `rg "web-build|next build|pnpm run build" .github/workflows Dockerfile*` hits outside the files above, *Then* each build path calls the mode-aware target, so `build.ts`'s `guard` stage applies on every path; none calls `next` directly under `WEB_BUILD=bun`.
- Scaffolding guard.
  - *Given* `backlog-scaffolding-guard.yml:53` and `session/git/scaffolding.go:35`, *Then* the `web-app/.next/` ignore is kept until PR 9 and `web-app/.out-tmp-*`, `web-app/.build/` are added.
**Files**: `.github/workflows/benchmark.yml`, `.github/workflows/ux-analysis.yml`, `.github/workflows/backlog-scaffolding-guard.yml`, `.github/actions/prepare/action.yml`, `tests/e2e/lighthouse.config.js`, `web-app/lighthouserc.json`, `session/git/scaffolding.go`, `session/git/scaffolding_test.go`

##### Task 6.5a: Test first, `actionlint` clean + `yq` path assertions; a green `workflow_dispatch` run on a throwaway branch is linked in the PR (decision 10) (S)
##### Task 6.5b: benchmark/prepare edits (S)
##### Task 6.5c: ux-analysis paths and Lighthouse config (S)
##### Task 6.5d: Scaffolding ignore additions and Go test (XS)

#### Story 6.6: Storybook per ADR-002
**As a** developer, **I want** `@storybook/nextjs` gone and Storybook still building, **so that** one more Next package leaves the tree.
**Acceptance Criteria**:
- *Given* `package.json:136`, *When* `@storybook/nextjs` is removed, *Then* `rg "storybook/nextjs" web-app` hits only the explanatory comments (updated) and `npm run build-storybook` exits 0.
- *Given* `.github/workflows/build.yml`, *When* a non-blocking `storybook-build` job runs `storybook build` on PRs touching `web-app/src/**/*.stories.tsx`, `.storybook/**`, `web-app/src/lib/nav/**`, *Then* it passes on this branch.
- Variant if the owner picks "delete Storybook" for F2: `.storybook/`, 50 stories, scripts, and `@chromatic-com/storybook` are removed and no job is added.
**Files**: `web-app/package.json`, `web-app/.storybook/main.ts`, `.github/workflows/build.yml`

##### Task 6.6a: Remove dependency, fix comments, run build (S)
##### Task 6.6b: CI job (XS)

#### Story 6.7: Docs, CLAUDE.md and ADR amendments
**As a** future contributor, **I want** the repo's guidance to describe the new build, **so that** nobody follows a stale `next dev` instruction.
**Acceptance Criteria**:
- *Given* `rg -n "next dev|next build|NEXT_BUILD_MODE|\.next/|Next\.js static export" CLAUDE.md docs server/web/embed.go .claude/commands Makefile`, *When* updated for dual mode, *Then* each hit either describes `WEB_BUILD` or is explicitly marked "Next mode (removed in PR 9)".
- *Given* `docs/adr/009-vanilla-extract-type-safe-css.md:31` ("compatible with Next.js 15 App Router") and `docs/adr/010-frontend-modularity.md:40-44` (RSC boundary), *Then* each gains an "Amended by project web-app-bun-build" note.
- *Given* a new `docs/how-to/build-web-app.md` and a `CLAUDE.md` Reference Documents Index row, *Then* they document `WEB_BUILD`, `WEB_MINIFY`, `WEB_DEV_HMR`, ports, the patch runbook link, and the debug-build recipe.
- *Given* the CLAUDE.md "Manual dev port block", *Then* it gains the Bun dev server row (62875) next to 62871 (instance #1 `PORT`, HTTP), 62872 (instance #1 `--remote-port`, TLS) and 62873/62874 (instance #2), and the contributor guidance first written in PR 1 and PR 2b (Tasks 1.2c, 2.9e) is extended here, not first introduced.
- Docs checks are tests (validation A9; REQ-S5).
  - *Given* `web-app/scripts/__tests__/docs-grep.test.mjs`, *When* it runs the `rg` of the first bullet, *Then* it fails on any hit not marked "Next mode (removed in PR 9)" or describing `WEB_BUILD`, with a fixture proving it can fail.
**Files**: `CLAUDE.md`, `docs/how-to/build-web-app.md` (new), `docs/adr/009-vanilla-extract-type-safe-css.md`, `docs/adr/010-frontend-modularity.md`, `server/web/embed.go`, `.claude/commands/perf/make-it-faster.md`

##### Task 6.7a: Docs edits and new how-to (S)
##### Task 6.7b: `docs-grep.test.mjs` with a failing fixture (XS)

#### Story 6.8: Go test binding every built file to the auth exemption
**As the** owner, **I want** a lockout to be impossible to merge, **so that** the login page can always load everything it needs (F4/R1).
**Acceptance Criteria**:
- Every file, not just `index.html` (adversarial B3).
  - *Given* a built `web-app/out/` (locally the test skips with a clear message if `out/` is absent; **in CI it fails**: when `CI` or `REQUIRE_BUILT_OUT=1` is set, which the `web-build-smoke-bun` job sets explicitly, a missing or empty `out/` is a test failure, so a mis-wired step cannot skip silently; a unit test of the small `builtOutDir(t, require bool)` helper covers both branches; `web-build-smoke-bun` provides the artifact), *When* the Go test walks **every file** under `out/` (including dynamic-import chunks, CSS `url()` fonts, `.map` files and `public/` assets), *Then* the URL path of each file satisfies `isExempt` (prefixes `/auth/ /login /health /_next/ /favicon`, `server/middleware/auth.go:95-101`) or is on `server/middleware/testdata/public-allowlist.json` (created in Story 5.9, consumed here), whose entries are `{path, reason}` and name only files the unauthenticated `/login` page is shown (Story 7.13) not to request; every file under `/_next/` also gets the immutable cache header from the static handler.
- Parsed references are checked too.
  - *Given* `index.html` and every emitted `*.css`, *When* `src`/`href`/`url()` references and string-literal `import()` specifiers in `*.js` that resolve to files are extracted, *Then* each referenced URL exists in the walk (no reference to a file outside `out/`).
- Negative cases.
  - *Given* a fixture `out/` tree containing `assets/a.js`, or an entry bundle at `/entry.js`, or a `.woff2` under `/media/`, *Then* the test fails naming the file; *Given* the same tree with the file added to the allowlist without a reason, *Then* it fails.
**Files**: `server/middleware/built_assets_test.go` (new), `server/middleware/testdata/public-allowlist.json` (read; created in Story 5.9), `server/web/embed.go` (comment: no longer "Next.js static export")
##### Task 6.8a: Test first, walk plus fixtures (S)
##### Task 6.8b: Reference extraction from HTML/CSS/JS and the allowlist schema check (S)
##### Task 6.8c: CI wiring: run after `web-build-smoke-bun` with the artifact, `REQUIRE_BUILT_OUT=1` set (XS)

#### Story 6.9: jscpd, feature registry, scanner and ignore files
**As a** developer, **I want** repo gates to keep working with the new files, **so that** `make ready` stays green.
**Acceptance Criteria**:
- *Given* the new router/plugin test boilerplate, *When* `pnpm`-free `bun run lint:duplicates` runs, *Then* the percentage is <= 0.14% (shared helpers extracted if not; `build/` code lives outside `src/` so it is not scanned).
- *Given* `tools/scanner/frontend/src` (read in this story), *When* `make registry-generate` runs after moving no files, *Then* `git diff docs/registry` is empty, and if the scanner keys on `page.tsx` paths, the finding is recorded and handled.
- *Given* `.gitignore`, *Then* it lists `/web-app/.out-tmp-*`, `/web-app/.build/`, `/web-app/.web-build-mode`, and still carries the `!/web-app/build/` negation added in Story 3.0 (`git check-ignore web-app/build/build.ts` exits 1).
**Files**: `web-app/.jscpd.json`, `tools/scanner/frontend/src/*`, `.gitignore`

##### Task 6.9a: Run gates, fix or extract (S)

#### Story 6.10: Install and supply-chain hardening verification
**As the** owner, **I want** the install posture proven, **so that** a fresh clone and CI resolve identical, screened trees.
**Acceptance Criteria**:
- *Given* `bun install --frozen-lockfile` in CI, *When* run twice after `bun add`-then-`bun install` (workaround for #44713), *Then* the lockfile is stable.
- *Given* `trustedDependencies` from gate S0, *Then* it lists only packages that need scripts and a CI step fails if `bun.lock` contains any `github:` source or any package from the banned-version list.
- *Given* all workflows, *Then* `rg "pull_request_target" .github` returns no hit that checks out PR code, and `oven-sh/setup-bun` is pinned by SHA.
**Files**: `web-app/bunfig.toml`, `web-app/package.json`, `.github/workflows/*.yml`, `web-app/scripts/check-dependency-screen.mjs`

##### Task 6.10a: Wire screen script into CI and verify pins (S)

---

## Phase 7: UX pre-cutover specs (PRs 7a and 7b)

All specs follow the repo conventions: first line `// @feature ...`, no `waitForTimeout`, locators by `data-testid` or ARIA role, page helpers under `tests/e2e/pages/`. Title assertions use auto-retrying `await expect(page).toHaveTitle(...)` (never a one-shot `page.title()` read), because on Bun the title is set by React after navigation. **Ordering is exact (see Plan-level decision 3):** the "a" halves of each story below are in **PR 7a** (Next baselines: land right after PR 1, before PR 2a, while Next is the only build, so fixtures are captured from Next); the "b" halves, which assert behaviour that exists only on the Bun build, are in **PR 7b** (land after PR 5a, before PR 8). Where Next's behaviour differs from the decided policy, the "a" half records the Next value to `tests/e2e/fixtures/` and the "b" half asserts the policy through `webBuildMode() === "bun"` from the single helper `tests/e2e/helpers/web-build.ts`.

### Epic 7.1: Specs that make the router swap safe
**Goal**: Close the coverage gaps listed in `research/ux.md` §7 before the cutover, and make the Playwright baseline of Story 0.1 actionable.

#### Story 7.1: `route-titles.spec.ts`
**As a** user with many tabs, **I want** each page to keep its title, **so that** I can tell tabs apart.
**Acceptance Criteria**:
- *Given* `tests/e2e/fixtures/route-titles.json` captured from the Next build (all `page.tsx` paths; `/backlog` and `/backlog/board` recorded as the root title they have today), *When* each static route is hard loaded and then reached by client-side click navigation, *Then* `await expect(page).toHaveTitle(fixture)` (auto-retrying) equals the fixture string exactly, e.g. `/settings/jules` -> `Jules - Settings - Stapler Squad`. Lands in 7a; runs unchanged on Bun.
- Auto-retrying title assertions everywhere (triad review, UX).
  - *Given* every spec under `tests/e2e` (not only this one) and every page-object helper, *When* grepped for one-shot title reads (`page.title()`, `document.title` inside a bare `evaluate` followed by `expect(...).toBe`), *Then* none remains: each is converted to `await expect(page).toHaveTitle(...)` (count derived from the grep and stated in the PR body), and a CI grep step in the e2e conventions check fails on a new one (Task 7.1b). On Bun the title is set by React after navigation, so a one-shot read is flaky by construction.
**Files**: `tests/e2e/route-titles.spec.ts`, `tests/e2e/fixtures/route-titles.json`, `tests/e2e/pages/` helper, the e2e convention checker (title-read grep)
##### Task 7.1a: Capture script writing the fixture from the Next build (S)
##### Task 7.1b: Spec, conversion of one-shot title reads in existing specs, and the CI grep guard (S)

#### Story 7.2: `route-focus-and-announce.spec.ts` and `aria-current`
**As a** keyboard or screen-reader user, **I want** navigation announced and focus placed, **so that** I am not stranded.
**Acceptance Criteria**:
- 7.2a (PR 7a, Next): *Given* a click on the History nav link, *When* navigation completes, *Then* a live region (`[role=status],[role=alert],[aria-live]`, excluding unrelated page regions) contains `History - Stapler Squad`; *Given* the skip link, *When* Tab then Enter, *Then* `#main-content` is focused and the URL path is unchanged; *Given* the main nav on `/history`, *Then* the History item exposes `aria-current="page"` if it does today (baseline recorded; if absent today, the spec records "absent", does not assert it, and a follow-up issue is filed); *Given* the skip link activated on Next, *Then* the computed `outline-style` of `main#main-content` is recorded to the fixture (the Next value D8 matches).
- 7.2b (PR 7b, Bun only): *Given* the same click, *Then* the region is `role=status` (not `alert`), and focus is on the layout `main#main-content`; *Given* two navigations to pages with equal titles in sequence, *Then* the region text is announced twice (a `MutationObserver` records clear-then-set); *Given* a page that autofocuses an input (for example the omnibar route), *Then* focus stays on that input; *Given* browser Back or Forward (popstate), *Then* the title is announced and focus is on the layout `main#main-content` (UX-12); *Given* the terminal session page or an omnibar-open navigation, *Then* the page keeps the focus it grabs once the route settles, even if it grabs it asynchronously after data loads (pre-mortem failure mode 11; if any doubt remains `FOCUS_ON_NAVIGATE` is `false` for those two routes); *Given* `/config` (redirect), *Then* exactly one announcement is recorded; *Given* `/history`, *Then* `document.querySelectorAll("main#main-content")` may contain duplicates (pre-existing) but the focused element is the first (UX G4, G5). If an unresolved conflict remains after one repair wave, `FOCUS_ON_NAVIGATE` is set `false` and the spec asserts the policy as `false` (D5 is not a cutover gate).
- 7.2c (PR 7b, Bun only; Story 2.7's guard): *Given* the real omnibar (a React controlled input) holding typed text, *When* `page.route` makes the next route chunk return 404 and the user navigates, *Then* a `window.__marker` set before navigation survives (no reload), the "A new version is available. Reload when ready." banner is visible, and the typed text is intact; *Given* an empty page with no text-like fields, *Then* exactly one automatic reload happens.
**Files**: `tests/e2e/route-focus-and-announce.spec.ts`, `tests/e2e/stale-chunk-guard.spec.ts` (new, 7.2c)
##### Task 7.2a: Spec with baseline recording (S)
##### Task 7.2b: Bun-only assertions incl. edge cases (S)
##### Task 7.2c: Stale-chunk omnibar spec with a 404 chunk (S)

#### Story 7.3: `history-behaviour.spec.ts`
**As a** user, **I want** Back and Forward to behave, **so that** I can retrace my steps.
**Acceptance Criteria**:
- *Given* navigation `/` -> `/history?sessionId=a` -> `/logs`, *When* Back twice then Forward once, *Then* the URL after the second Back is `/` and after the Forward is `/history?sessionId=a`, exactly as the Next baseline.
- *Given* the mobile viewport and the notification tray opened (`ssqTray` entry), *When* Back is pressed, *Then* the tray closes and the URL is unchanged.
- *Given* `/unfinished?tab=a`, *When* the tab control selects `b`, *Then* the URL becomes `?tab=b` without adding a history entry and the active tab is `b` (the 4 raw-History behaviours).
Lands in 7a; runs unchanged on Bun.
**Files**: `tests/e2e/history-behaviour.spec.ts`
##### Task 7.3a: Spec (M)

#### Story 7.4: `deep-link-trailing-slash.spec.ts`
**As a** recipient of a notification or Slack link, **I want** it to open the right view, **so that** links keep working.
**Acceptance Criteria**:
- 7.4a (PR 7a): *Given* `/rules`, `/rules/`, `/rules?prefill=x`, `/rules/?prefill=x`, `/settings/jules/`, *When* hard loaded, *Then* each shows the right page; URLs the server builds (`/?session=ID&tab=terminal&notification=N`, `/backlog?item=ID`) open the session pane / backlog item; `page.url()` for `/rules` equals `${BASE_URL}/rules` (existing expectation). *Given* a push-click handoff string `/?session=abc&window=w1`, *Then* `PushClickHandoff` navigation lands with both params intact.
- 7.4b (UX G9; the spec runs on both builds, assertions identical): *Given* `/?window=w1` and a client navigation to `/history`, *When* the page changes, *Then* `?window=w1` is preserved wherever `useWindowUrlSync` preserves it today (baseline recorded on Next; same under Bun); *Given* the login flow, *When* the app navigates to `/login` (a hard navigation today, outside the router), *Then* the full document loads from the server (a request for `/login` is observed) and the SPA fallback does not render the previous page.
**Files**: `tests/e2e/deep-link-trailing-slash.spec.ts`
##### Task 7.4a: Spec (S)
##### Task 7.4b: `?window=` preservation and `/login` hard-navigation cases (S)

#### Story 7.5: `theme-fouc.spec.ts`
**As a** dark-theme user, **I want** no flash of the light theme, **so that** pages do not flicker.
**Acceptance Criteria**:
- 7.5a (PR 7a, Next): *Given* `stapler-theme` set to each of `matrix`, `cyberpunk77`, `wh40k`, `clean`, `light`, `dark` before load, *When* the page loads, *Then* at `DOMContentLoaded` `<html>` has that theme's class and the first `requestAnimationFrame` background is not the clean-theme background (for the dark ones). *Given* a `custom:` theme in `stapler-theme-custom`, *Then* its class and CSS variables are applied before first paint; *Given* a stale class in storage, *Then* it degrades to `clean` and the page is usable. *Given* font classes on `<html>` under Next, *Then* the theme script does not remove them.
- 7.5b (PR 7b, Bun, UX G10): *Given* `--font-*` variables under Bun, *Then* the boot script does not remove them; *Given* the emitted document, *Then* the theme stylesheet is a `<link rel="stylesheet">` present in `<head>` before any module script (render-blocking); *Given* `localStorage` access throwing (`Object.defineProperty(window, "localStorage", {get(){throw new Error()}})` via `addInitScript`), *Then* the page renders with the `clean` theme and no uncaught error.
**Files**: `tests/e2e/theme-fouc.spec.ts`
##### Task 7.5a: Spec (M)
##### Task 7.5b: Bun-only head-order and storage-throws cases (S)

#### Story 7.6: `viewport-meta.spec.ts`
**As a** mobile user, **I want** safe-area insets honoured, **so that** content is not under the notch.
**Acceptance Criteria**:
- *Given* any route, *Then* `meta[name=viewport]` content equals `width=device-width, initial-scale=1, maximum-scale=5, viewport-fit=cover` and `document.documentElement.lang` equals `en` (`<html lang="en">`, as Story 5.3 emits and `layout.tsx` has today); *Given* 390x844, *Then* a bottom-nav element's computed bottom padding resolves `env(safe-area-inset-bottom)` as today (value recorded). Lands in 7a; runs unchanged on Bun.
**Files**: `tests/e2e/viewport-meta.spec.ts`
##### Task 7.6a: Spec (XS)

#### Story 7.7: `not-found.spec.ts`
**As a** user mistyping a URL, **I want** a clear result, **so that** I know the page does not exist.
**Acceptance Criteria**:
- 7.7a (PR 7a, Next): *Given* `/nonexistent`, *When* loaded on Next, *Then* status 200 and the rendered body are recorded to a fixture (today's behaviour: the sessions page).
- 7.7b (PR 7b, Bun, UX G3): *Given* `/nonexistent`, *Then* the not-found view shows an `<h1>` "Page not found", the path text, a link to `/` and a Back button; `await expect(page).toHaveTitle(NOT_FOUND_META.title)`; the announcer region contains that title; the URL is unchanged; status 200; no redirect; no nav item is active (no active class and no `aria-current`; UX-17); the nav still works from the view; "Go back" is shown only when `history.length` > 1.
**Files**: `tests/e2e/not-found.spec.ts`
##### Task 7.7a: Spec, baseline half (7.7a) and Bun-only half (7.7b, PR 7b) (S)

#### Story 7.8: Axe sweep gaps and Lighthouse routes
**As a** user relying on assistive tech, **I want** all routes covered, **so that** regressions are caught everywhere.
**Acceptance Criteria**:
- *Given* `a11y-route-sweep.spec.ts` `ROUTES` (29 of 42 on 2026-10-19), *When* the missing routes (`/insights/session-detail`, `/sessions/summary`, `/fanout`, `/analytics/escape`, `/login`, `/help`, and the others found by diffing against the `page.tsx` paths) are added, *Then* the sweep runs on Next and on Bun. **Bounded**: a violation that also occurs on the Next build is added to the existing `KNOWN` list with a linked issue (this is the only permitted `KNOWN` addition; it does not block); a violation that occurs only on Bun blocks. The story is therefore finite and never gates the cutover on pre-existing a11y debt.
- WCAG 2.2 AA is part of the sweep (triad review, UX; UX-59; replaces the earlier "tags not enabled, follow-up" line).
  - *Given* `AxeBuilder.withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])` (the installed `@axe-core/playwright`/`axe-core` version is checked first to ship the `wcag22aa` tag, axe-core >= 4.5; if it does not, the story bumps the pinned version in the same PR or records the gap), *When* the sweep runs over all routes on Next and on Bun, *Then* a violation found on both builds (including every `wcag22aa` rule that is new to the project, for example target size) is **report-only**: it goes to the existing `KNOWN` list with a linked issue and does not block; a violation found only on Bun blocks. The set of tags run is asserted by a unit test of the sweep's config so the 2.2 tag cannot be dropped silently.
- WebKit / iOS Safari-equivalent checks, report-only if not feasible (triad review, UX; UX-61).
  - *Given* a Playwright `webkit` project (installed through `npx playwright install webkit`; availability on the Linux CI runner and on the dev host is checked first) with the iPhone 13/14 device descriptor at 390x844, *When* the mobile cases run on both builds, *Then* the following are asserted and compared with Next: (1) the `ssqTray` history marker: open the notification tray, press the browser Back (and, where WebKit exposes it, the edge-swipe equivalent through `page.goBack()`), the tray closes and the URL is unchanged; (2) `100dvh` panes: with the virtual keyboard simulated by shrinking the visual viewport (`page.setViewportSize` height reduction while an input is focused), a pane sized with `100dvh` does not overflow the visible area and the focused input stays within the visual viewport. If the WebKit project cannot be installed or run (no browser build for the host, flaky emulation of the keyboard), the case is **report-only**: it is skipped with a printed reason, the reason is recorded in `cutover-gate-results.md`, and a manual checklist item on a real iPhone (tray Back, keyboard over the omnibar) is added to Story 8.1; it never blocks the cutover and never fails the suite for a missing browser.
- Keyboard operability (UX-50; Axe cannot prove it).
  - *Given* a short manual checklist (Tab to the skip link and activate it; reach every nav item; open and close the notification tray with the keyboard; reach the not-found view's links; operate the ErrorBoundary Retry), *When* it is run once on each build, *Then* every control is reachable and operable; the checklist and its result live in `research/cutover-gate-results.md` (Story 8.1), not in a new document.
Lands in 7a for the Next run; the Bun run is part of Story 7.10's matrix.
**Files**: `tests/e2e/a11y-route-sweep.spec.ts`, `tests/e2e/mobile-webkit.spec.ts` (new, 7.8b), `tests/e2e/playwright.config.ts` (webkit project)
##### Task 7.8a: Extend `ROUTES`, add the WCAG 2.2 AA tags, classify findings as Next-also / Bun-only (S)
##### Task 7.8b: WebKit mobile project: `ssqTray` Back and `100dvh` with the virtual keyboard, report-only where infeasible (S)

#### Story 7.9: `ParityDiff` harness as a repeatable check
**As the** owner, **I want** the Next-vs-Bun computed-style comparison runnable on demand, **so that** CSS ordering regressions (T4) are caught at cutover and after Bun bumps.
**Acceptance Criteria**:
- *Given* the S3 harness promoted to `tests/e2e/tools/parity-diff.ts`, *When* run with two base URLs, *Then* it outputs a JSON diff for 5 routes x 2 viewports x **6 theme classes** (forced `:hover` and `:focus-visible` on the first 5 focusable elements per route) comparing computed styles and the resolved custom-property map of `:root`, and exits non-zero on any difference not in `tests/e2e/tools/parity-allow.json`, where each allow-list entry is `{route, selector, property, reason}` and `reason` must be a URL or a quoted CSS rule (an entry without one fails the run).
- *Given* the promoted S3 harness (pre-mortem P1-1), *When* run against a Bun build, *Then* it also runs all 42 routes cold at 1 viewport x 2 themes and the in-session navigation-sequence walk (3 fixed + 3 seeded-random orderings, seed printed, no reload), comparing the final route's computed styles with its cold load on the same build and with Next, and exits non-zero on any difference not in the allow-list.
- *Given* a seeded CSS difference, *Then* the self-test fails; *given* an allow-list entry with no `reason`, *then* the self-test fails.
Lands in 7a (harness and self-test); its Bun run is Story 8.1.
**Files**: `tests/e2e/tools/parity-diff.ts`, `tests/e2e/tools/nav-walk.ts` (new), `tests/e2e/tools/parity-allow.json`
##### Task 7.9a: Promote harness, add 6 themes, resolved properties, reasoned allow-list, self-tests (M)
##### Task 7.9b: Promote the 42-route cold run and the navigation-sequence walk, with a seeded-order self-test (M)

#### Story 7.10: Run e2e in both modes
**As the** owner, **I want** the whole suite exercised against both builds before cutover, **so that** parity is proven by the existing specs too, with a gate that can actually fail (adversarial B2).
**Acceptance Criteria**:
- *Given* `tests/e2e/helpers/test-server.ts` building the binary via `make`, *When* `WEB_BUILD` is set, *Then* the global setup builds that mode, and `.github/workflows/build.yml` (the same file as Story 5.11: `needs:` cannot cross workflow files, and `build.yml` has no Playwright job today, e2e runs live in `ux-analysis.yml` and `e2e-video.yml`) gains two new jobs, not a matrix: `e2e-next` (`WEB_BUILD=next`, full suite, uploads its Playwright JSON report as an artifact with `if: always()`) and `e2e-bun` (`needs: e2e-next`, `if: always() && needs.e2e-next.result != 'skipped' && needs.e2e-next.result != 'cancelled'` so it still runs when Next had failures, `WEB_BUILD=bun`, downloads that artifact from the same run, then runs the comparator); Task 7.10f creates both and appends them to `web-build-gate`'s `needs:`.
- `e2e-bun` runs and reports from its first appearance (PR 7b) and joins `web-build-gate` only when its name is added to the repository variable `WEB_BUILD_GATE_MEMBERS` after 10 consecutive green runs on `main` or a recorded owner flip (decision 10, Story 5.11); it is not `continue-on-error`, and joining is not a workflow edit. It runs `scripts/e2e-baseline-compare.ts` (created in this story, Task 7.10c, so that Story 8.1 and CI use the same script) against `tests/e2e/baseline/next-e2e-baseline.json` (Task 0.1e), `tests/e2e/baseline/bun-expected-diff.json` (Story 7.11) and the Playwright JSON report of the `e2e-next` job of the same run, applying the rule defined in Story 0.1 (baseline `pass` fails on Bun at >= 1 of 3 unless the same-run `e2e-next` job shows it too; baseline `flaky` fails at >= 2 of 3, warns at 1 of 3 and must pass >= 1 of 3; baseline `fail` excluded but reported; specs absent from the baseline judged against the same-run `e2e-next` report, missing report fails). The expected-diff list may only shrink (the comparator fails if an entry is added without a linked issue, or a listed spec now passes and is not removed).
- The comparator can fail (adversarial re-review: no AC proved it).
  - *Given* `tests/e2e/baseline/e2e-baseline-compare.test.ts` (collected by the existing `e2e-dev-mode` Jest project) with fixtures under `tests/e2e/baseline/__fixtures__/`, *When* it runs the script as a child process over each fixture, *Then* exit status is non-zero for: (1) a **new failure** (spec `pass` in the baseline, failing 3/3 on Bun, not listed); (2) **flaky -> fail** (baseline `flaky`, failing 3/3 on Bun); (3) a **missing spec** (absent from the baseline, failing on Bun, passing in the same-run `e2e-next` report) and a missing Next report; (4) an `bun-expected-diff.json` entry with no `issue` URL; (5) a listed spec that now passes and was not removed, and (6) a baseline-`pass` spec failing 1 of 3 on Bun while `e2e-next` is clean; and exit status is zero for: a baseline `fail` spec failing on Bun, a missing spec failing on both builds (reported as baseline-class), a baseline-`flaky` spec failing 1 of 3 on Bun (prints a warning with the failure text), a baseline-`pass` spec failing 1 of 3 on Bun and also in the same-run `e2e-next` report (shared flake, reported) and a listed expected-diff spec that still fails. A comparator that always exits 0 therefore fails seven of these cases.
- First-run ordering (adversarial re-review): `e2e-bun` reports from PR 7b's first CI run (and can block once its name is in `WEB_BUILD_GATE_MEMBERS`), so `bun-expected-diff.json` must exist and be accurate **before PR 7b opens**: the implementer runs the suite 3 times locally on a Bun build in a ManualInstance (Task 7.11c), runs the comparator, and commits the resulting file in PR 7b; otherwise PR 7b's own cell is red and the 10-green-run clock cannot start.
- Route coverage cannot hide in excluded specs (pre-mortem failure mode 6).
  - *Given* `scripts/route-coverage-manifest.ts` (Task 7.10e), *When* it maps each `RoutePath` to the specs that visited it (from `routesVisited` in `next-e2e-baseline.json`, Task 0.1e, plus the generated smoke spec of Story 7.14, which ships in PR 7a), *Then* it fails if any route's only coverage is baseline-`fail`, baseline-`flaky` or listed in `bun-expected-diff.json`. (The smoke spec itself moved to Story 7.14 so that this story lies wholly in PR 7b; the previous Task 7.10d is now Task 7.14a, and the letter d is not reused in 7.10.)
**Files**: `scripts/route-coverage-manifest.ts` (new, 7.10e), `scripts/e2e-baseline-compare.ts` (new), `tests/e2e/baseline/e2e-baseline-compare.test.ts` and `tests/e2e/baseline/__fixtures__/*` (new), `tests/e2e/helpers/test-server.ts`, `tests/e2e/global-setup.ts`, `tests/e2e/helpers/web-build.ts` (new), `tests/e2e/baseline/bun-expected-diff.json`, `.github/workflows/build.yml` (new jobs `e2e-next` and `e2e-bun`; `e2e-bun` has `needs: e2e-next` and downloads its JSON report artifact; `web-build-gate` `needs:` gains both)
##### Task 7.10a: Env plumbing and the single `WebBuildMode` parser (S)
##### Task 7.10b: The `e2e-bun` step that downloads the `e2e-next` report artifact and runs the comparator (S)
##### Task 7.10c: `scripts/e2e-baseline-compare.ts` and its fail-case unit tests (M)
##### Task 7.10e: Route coverage manifest and its failure test (S)
##### Task 7.10f: Create the `e2e-next` and `e2e-bun` jobs in `build.yml` (`needs: e2e-next`, the `if:` guards, `if: always()` report upload, `WEB_BUILD` per job), append both to `web-build-gate`'s `needs:`, and extend the gate fixture test with `e2e-next` cancelled (red) and `e2e-bun` failing as a non-member (green) (S)

#### Story 7.11: Triage of Next-quirk specs against the `E2eBaseline`
**As the** owner, **I want** specs that encode Next build quirks classified before they can fail the Bun gate for the wrong reason, **so that** the cutover gate is meaningful (adversarial B2).
**Acceptance Criteria**:
- Inventory and budget.
  - *Given* the `buildCoupledSpecs` list from Task 0.1f (at least `tests/e2e/insights-findings-panel.spec.ts:88-95`, `jules-status-badge.spec.ts:47` and the five specs excluding `#__next-route-announcer__`), *When* each is triaged, *Then* it ends in exactly one state: (a) made build-neutral (for example the insights spec queues responses by count-agnostic mock instead of twice, so it passes under StrictMode double-mount on Next and single-mount on Bun), or (b) recorded in `bun-expected-diff.json` with a linked issue and the reason. The work is capped at **12 specs**; if more than 12 are found, the remainder go to (b) with issues and the gate proceeds.
- Evidence and timing.
  - *Given* a (a)-class fix, *Then* the spec passes 3 of 3 runs on the Next build (lands in PR 7a) and, when PR 7b first runs the suite on a Bun build, on Bun as well. Story 7.11 deliberately straddles the two PRs (Tasks 7.11a-b in PR 7a, Task 7.11c in PR 7b); Task 7.11c is a first-class task, with its own heading below, because state (b) can only be decided from a Bun run, so `bun-expected-diff.json` is created in PR 7b by Task 7.11c from a **local first Bun run on a ManualInstance before PR 7b opens** (3 runs, then Story 7.10's comparator), and PR 7a ships only the inventory and the (a) rewrites.
**Files**: `tests/e2e/insights-findings-panel.spec.ts`, `tests/e2e/jules-status-badge.spec.ts`, the other listed specs, `tests/e2e/baseline/bun-expected-diff.json` (new)
##### Task 7.11a: Inventory classification and issues (S)
##### Task 7.11b: Build-neutral rewrites, verified on Next 3/3 (M)
##### Task 7.11c: Create `bun-expected-diff.json` from a local first Bun run before PR 7b opens (XS; lands in PR 7b)
- Run the suite 3 times on a Bun build in a ManualInstance, run `scripts/e2e-baseline-compare.ts`, write each unexplained failure as an entry with a linked issue; the file is committed in PR 7b so `e2e-bun` is green on its first CI run.

#### Story 7.12: `scroll-policy.spec.ts` (UX G8)
**As a** user, **I want** scroll position to behave as before, **so that** navigation feels the same.
**Acceptance Criteria**:
- *Given* the scroll container recorded by S4 (window or `<main>`), *When* navigating forward to a new pathname, *Then* the container is at top; *When* going Back, *Then* the previous position is restored (within 2 px); *When* only the search params change through `router.replace(url,{resetScroll:false})` (for example the Up Next tab), *Then* the position is unchanged; *Given* hash-only navigation `#main-content`, *Then* the browser handles it without a router navigation. Baselined on Next in 7a (the Next values recorded to a fixture), asserted on Bun in 7b.
**Files**: `tests/e2e/scroll-policy.spec.ts`
##### Task 7.12a: Spec (S)

#### Story 7.13: `login-static-assets.spec.ts` (lockout guard, adversarial B3)
**As the** owner, **I want** the unauthenticated `/login` page proven to load every asset it needs, **so that** a Bun naming default can never lock users out.
**Acceptance Criteria**:
- 7.13a (PR 7a, Next): *Given* a server started with remote auth enabled and no session (a `remoteAuth` option in `tests/e2e/helpers/test-server.ts` that starts the binary with `--remote-access` and a fresh credential store; **feasibility is proven first by Task 0.2b**: remote auth is attached only to the remote TLS listener, so the Playwright context needs `ignoreHTTPSErrors` or the CA trusted; if 0.2b recorded the Playwright path as infeasible, 7.13a/7.13b are implemented as the Go test `server/middleware/login_assets_test.go` that serves the real built `out/` through `middleware.StaticHandler` wrapped in `middleware.Auth` with an always-reject validator over `httptest`, and the request log becomes the parsed-reference list of `/login`), *When* a fresh browser context loads `/login`, *Then* every network request is recorded (`page.on("request"/"response")`, including dynamic-import chunks, fonts, images, source maps if fetched), and the test asserts zero responses with a redirect (3xx) to `/login` for a static asset, zero non-2xx responses, and records the list of requested paths to `server/middleware/testdata/login-requests.json` (the input for Story 6.8's allowlist).
- 7.13b (PR 7b, Bun): the same run against the Bun build asserts the same, and additionally that every requested path is under an exempt prefix or on `public-allowlist.json` (created in Story 5.9). It is also a step of the `e2e-bun` job and part of Story 8.1, not a one-off.
**Files**: `tests/e2e/login-static-assets.spec.ts`, `tests/e2e/helpers/test-server.ts`, `server/middleware/testdata/login-requests.json`
##### Task 7.13a: Server fixture with remote auth, request-log spec (S)
##### Task 7.13b: Bun assertions and CI step (S)

#### Story 7.14: Generated route smoke spec over all 42 deep links (PR 7a)
**As the** owner, **I want** every route loaded once with and without a trailing slash on both builds, **so that** a route cannot be covered only by specs the baseline excludes (pre-mortem failure mode 6). Moved here from Task 7.10d so that Story 7.10 lies wholly in PR 7b; the spec needs only PR 1 (`RouteMeta`) and lands with PR 7a.
**Acceptance Criteria**:
- *Given* `tests/e2e/route-smoke.spec.ts` generated from `RouteMeta` (key set derived, not hard-coded), *When* it visits all 42 deep links with and without a trailing slash on the Next build (PR 7a) and on both builds afterwards (Story 7.10's matrix), *Then* it fails on any console error, page error, failed request, `Unexpected token <` or missing `main`; console errors that also occur on Next are allow-listed one by one with an issue link, and the allow-list is captured on Next before the facade migration starts.
**Files**: `tests/e2e/route-smoke.spec.ts` (new)
##### Task 7.14a: Generated route smoke spec over all 42 deep links, with and without trailing slash (S; formerly Task 7.10d)

---

## Phase 8: Cutover (PR 8)

### Epic 8.1: Flip the default after evidence
**Goal**: Bun becomes the default build; Next remains one flip (plus a planned restart) away until PR 9 merges. Story 8.5 (restart-safety precondition and rollback runbook) runs before Story 8.2.

#### Story 8.1: Pre-cutover gate run on a ManualInstance
**As the** owner, **I want** the cutover decision backed by a full run, **so that** the flip is boring.
**Acceptance Criteria**:
- Full suite against the recorded baseline.
  - *Given* `WEB_BUILD=bun make web-build` (the cutover configuration: unminified, `WEB_MINIFY=0` from the Makefile) embedded into a binary run as a ManualInstance with `--remote-access` (`PORT=62871`, `--remote-port 62872`), *When* the full Playwright suite (every spec under `tests/e2e`, count derived, plus Stories 7.1-7.13) runs 3 times, *Then* the set of failing tests is a subset of the `fail`/`flaky` entries of `tests/e2e/baseline/next-e2e-baseline.json` (Task 0.1e) union `tests/e2e/baseline/bun-expected-diff.json` (Story 7.11), with the comparison produced by `scripts/e2e-baseline-compare.ts`, the same script CI uses (created and unit-tested in Story 7.10; the baseline is the committed `E2eBaseline`, not a recollection); a spec that failed on Next and passes on Bun is reported, not hidden.
- Lockout guard (B3) is a **hard cutover gate** (triad review, UX; ux.md section 5: the login-asset lockout has no in-app exit).
  - It cannot be waived, downgraded to report-only or deferred by the owner's 5x decision, the Axe `KNOWN` list or any other bounded item; PR 8 does not merge without this evidence in `cutover-gate-results.md`.
  - *Given* remote auth enabled and no session, *When* the unauthenticated `/login` Playwright run of Story 7.13 executes against this build, *Then* it records every request (dynamic chunks, CSS `url()` fonts, images, maps) with zero redirects to `/login` and zero non-2xx; *and* the Go walk of Story 6.8 passes against the same `out/`; *and* `curl -s -o /dev/null -w "%{http_code} %{content_type}"` of 3 sampled non-`index.html` assets (one chunk, one font, one `public/` file used by the login page) returns 200 with the right type unauthenticated, run against both the local listener and the remote TLS listener (`https://localhost:62872`, private CA or `-k`), because remote-only users are the ones a lockout hits (pre-mortem failure mode 10).
- Passkey ceremony is checked by hand (validation A8; REQ-N2).
  - *Given* the ManualInstance with `--remote-access`, *When* the owner opens `/login` on the remote listener, signs in with a passkey and follows a `setup_token` link, *Then* both succeed; the result is recorded in `cutover-gate-results.md` (the automated checks prove asset loading, not the WebAuthn ceremony).
- Keyboard checklist (UX-50).
  - *Given* the checklist of Story 7.8, *When* it is run on the Bun build, *Then* every control is reachable and operable; recorded in `cutover-gate-results.md`.
- Screen-reader checklist item (triad review, UX; UX-62).
  - *Given* the ManualInstance on the Bun build and one screen reader (NVDA on Windows/Linux-hosted browser, or VoiceOver on macOS/iOS, whichever the owner has; the one used is recorded), *When* the owner navigates from `/` to `/history` with the nav link and then Back, *Then* the title is spoken once per navigation (no "loading" announcement, no double announcement), focus lands on the main region, and the not-found view is announced by its title; the result and the Next-baseline comparison (Task 0.5d recorded Next's announcer politeness) are written to `cutover-gate-results.md`. If no screen reader is available the item is recorded as "not run" with the reason; it is a recorded manual check, not a mechanical gate, because Axe and the live-region assertions cannot prove what a screen reader speaks.
- Mobile Safari items not covered by the WebKit project (UX-61).
  - *Given* Story 7.8's WebKit cases ran report-only or were skipped, *Then* a real-iPhone (or Safari simulator) pass of the tray Back/edge-swipe and the virtual keyboard over the omnibar (`100dvh`) is recorded in `cutover-gate-results.md`; a failure is filed as an issue and does not block the cutover.
- Dev and build error handling are CI-asserted, not manual (triad review, UX; UX-53, UX-55).
  - *Given* Task 5.6c's dev-mode CI step (`dev-mode-hmr.spec.ts` for the overlay, run in the `web-build-smoke-bun` job) and the `build.test.ts` stage-failure test (exit status, stage and file named, `out/` intact, `.out-tmp-*` removed), *Then* both are green in the `web-build-smoke-bun` job before this gate is run; this story only cites those runs and adds no manual overlay or build-failure check.
- Minification is not a gate of this story (ADR-004).
  - *Given* the cutover ships unminified, *Then* this story runs one informational minified pass (`WEB_MINIFY=1`, one run of the suite) and records it as the input to Story 8.6, where the name-dependence review and the 3-run minified gate live.
- Parity.
  - *Given* ParityDiff and the census (Stories 7.9, 5.10) run on the Bun build, *Then* all pass: the 5-route x 6-theme matrix, all 42 routes cold, the navigation-sequence walks (3 fixed + 3 seeded-random; seeds in the record) and the rule-order census (pre-mortem P1-1); any unexplained delta fires T4.
- Bounded added-scope items.
  - *Given* Stories 4.5 (D5), 6.2 (flat config) and 7.8 (Axe) are added scope, *Then* none of them is a precondition of this gate except the Bun-only regressions defined in 7.8.
**Files**: `project_plans/web-app-bun-build/research/cutover-gate-results.md` (new), `scripts/e2e-baseline-compare.ts` (used here; created in Story 7.10)
##### Task 8.1a: Run, triage, fix-forward list (M)
##### Task 8.1b: Informational minified pass; name-dependence grep inventory handed to Story 8.6 (S)
##### Task 8.1c: Run the Story 7.10 comparator on the 3 runs and record login-asset evidence (XS)

#### Story 8.2: Flip defaults
**As the** owner, **I want** one small PR flipping the default, **so that** rollback is small and well defined.
**Acceptance Criteria**:
- *Given* `Makefile` `WEB_BUILD ?= next`, *When* changed to `bun` (with `WEB_MINIFY ?= 0` unchanged until Story 8.6), and the `WEB_BUILD`/`WEB_MINIFY` defaults of the CI jobs updated (no matrix exists; `web-build-smoke-next` stays as the rollback canary until PR 9), *Then* `make web-build` produces an unminified Bun build, `make install-service` documentation notes the planned-restart warning, and `WEB_BUILD=next make web-build` still builds Next.
- Required checks never change (pre-mortem P1-3).
  - *Given* that only the aggregate `web-build-gate` is required (decision 10) and the job names do not change, *When* the CI defaults change, *Then* the PR description states that no required-check name changed, Task 8.2b records the value of `WEB_BUILD_GATE_MEMBERS` at the flip (whether the Bun jobs are members and the run history or owner flip that justified it; read back with `gh variable get WEB_BUILD_GATE_MEMBERS`), and the inverse steps for a rollback are in `docs/how-to/rollback-web-build.md` (Story 8.5).
- Preconditions are recorded before merge.
  - *Given* `cutover-gate-results.md`, *Then* PR 8 does not merge without (a) the passing `install-service` preflight of Task 8.5d, (b) the same-host `make web-build` ratio of Story 8.3 and, if it missed 5x, the owner's written decision, and (c) a green `workflow_dispatch` run of the edited workflow on a throwaway branch.
- *Given* the PR, *Then* it touches only Makefile, workflow defaults, docs and nothing under `src/`.
**Files**: `Makefile`, `.github/workflows/build.yml`, `docs/how-to/build-web-app.md`, `docs/how-to/rollback-web-build.md`
##### Task 8.2a: Flip the defaults (XS)
##### Task 8.2b: Record the value of `WEB_BUILD_GATE_MEMBERS` at the flip (XS)

#### Story 8.3: Measured before/after table
**As the** owner, **I want** the success metrics measured the same way as the baseline, **so that** the result is evidence, not assertion.
**Acceptance Criteria**:
- The 5x metric gates PR 8 (consistency C13; validation A2; REQ-M2).
  - *Given* `scripts/measure-web-build.sh` (new), which runs `make web-build` for `WEB_BUILD=next` (the baseline is `make web-build` exactly as the Makefile runs it today: `pnpm run build` with `NEXT_BUILD_MODE=development`, `Makefile:226`, a `next build` with development-mode settings; it is neither `next dev` nor the production-mode Next build, which Story 0.4 measures separately) and `WEB_BUILD=bun` back to back on one host, n >= 5 runs per mode for each of cold and warm (cold = build caches deleted, warm = caches present; the Story 0.4 protocol, where Next's caches are `web-app/.next/cache` and the shared webpack directory `NEXTJS_SHARED_CACHE_DIR`), and prints the ratio of the **warm medians** (the gate), the cold medians, the min-max ranges and both times, plus the warm ratio recomputed with both Next cache directories cleared before every Next run (Next's incremental cache makes warm Next faster and so shrinks the ratio, review R2-C8d; this second ratio is reported beside the gate for the owner's decision, not a second gate), *When* it runs before PR 8 merges, *Then* the Bun end-to-end time (including the Go embed copy) is at most one fifth of the same-session Next time (the absolute 10.8 s derived from the 54.0 s warm baseline is also printed but is host-dependent, so the ratio is the gate). A miss does not block mechanically: the numbers (with per-stage times from `stats.json`) go to the owner, who chooses in writing between accept, investigate and a Vite 8 evaluation (ADR-007), and PR 8 does not merge without that record in `cutover-gate-results.md`.
- Results table.
  - *Given* the baseline method (Story 0.4's protocol: n >= 5 per mode, cold and warm, median and range, the host's load average noted), *When* measured, *Then* `docs/explanation/bun-migration-results.md` shows: production-equivalent build <= 10 s (warm median; the cold median beside it); the ratio above; the dev-loop reload time of `next dev` (Task 0.4f's before number) against `bun dev`, same method; **footprint rows, before and after** (the "leaner footprint" goal): direct dependency count (`dependencies` + `devDependencies` in `web-app/package.json`), installed package count, `du -sk web-app/node_modules`, and `bun install --frozen-lockfile` wall time cold and warm (n >= 5), where "before" is the Task 0.4f record and "after" is measured on a throwaway worktree with the Story 9.2 dependency removals applied (labelled "scratch projection"; Task 9.2a re-measures on the real tree and updates the row); total JS <= 6,349,052 B on the minified size build and the unminified cutover JS bytes; dev-server `.css.ts` reload time; Jest with no new failures against the Story 0.1 control; binary size delta (from the `go build` output, D3); or any miss stated with its number. Footprint rows have no pass line: a row that does not shrink is stated, not failed. Story 8.6 appends the minified-default numbers after PR 8b.
- Completeness is tested (validation A4; REQ-S6).
  - *Given* `web-app/scripts/__tests__/results-doc.test.mjs`, *When* it parses the results doc, *Then* it fails if a success-metric row of requirements.md or a footprint row is missing, a row has no number, or a miss has no stated reason.
**Files**: `docs/explanation/bun-migration-results.md` (new), `scripts/measure-web-build.sh` (new), `web-app/scripts/__tests__/results-doc.test.mjs` (new)
##### Task 8.3a: Measure and write (S)
##### Task 8.3b: `scripts/measure-web-build.sh` and the ratio check (S)
##### Task 8.3c: `results-doc.test.mjs` with a failing fixture (XS)

#### Story 8.4: Soak runbook and exit criteria
**As the** owner, **I want** an explicit soak, **so that** Next is deleted only on evidence.
**Acceptance Criteria**:
- *Given* `docs/how-to/build-web-app.md` "Soak" section, *Then* it lists what to watch (console `Unexpected token <`, login asset 200s, chunk-reload counter, HMR crashes), the revert command, T3 counting rule (two CI bundler crashes), and exit criteria: the owner's instance ran the Bun build for at least 14 calendar days and 3 distinct dev sessions after PR 8 with zero reverts and no open P1 web-build issue (then PR 8b), and for at least 7 further days after PR 8b (then PR 9). The owner may lengthen, never shorten, these defaults and records the reason in `cutover-gate-results.md`.
- PR 9 has a deadline (pre-mortem failure mode 9).
  - *Given* PR 8 merged on date D, *Then* dated reminders for D+21 days (earliest PR 9) and D+45 days exist (calendar entry or scheduled agent; the link is recorded in `cutover-gate-results.md`); if PR 9 has not merged by D+45 the owner decides in writing between finishing it and reverting to Next (stop-loss), because the dual-build tax (two CI cells, aliases, conditionals in tests) must not become permanent. Epics 1, 2c, 2d and 7a stay valuable under Next either way.
**Files**: `docs/how-to/build-web-app.md`, `project_plans/web-app-bun-build/research/cutover-gate-results.md`
##### Task 8.4a: Write the section (XS)
##### Task 8.4b: Dated PR 9 reminders and the stop-loss note (XS)

#### Story 8.5: Restart-safety precondition and rollback runbook (runs before Story 8.2)
**As the** owner, **I want** the rollback to be executable without destroying live sessions, **so that** "one-line flip" is true in practice (adversarial concern).
**Acceptance Criteria**:
- Precondition checked.
  - *Given* the deployed service, *When* `systemctl --user cat stapler-squad | grep -- --tmux-keep-server` (Linux) or `grep -- --tmux-keep-server ~/Library/LaunchAgents/*stapler-squad*.plist` (macOS) runs, *Then* it matches, or the story records the explicit decision to schedule the flip and any rollback accepting loss of every live tmux session (see CLAUDE.md WARNING and `docs/explanation/tmux-keep-server-on-restart.md`); the result is written into `cutover-gate-results.md`. Story 8.2 does not merge without this record.
- Runbook.
  - *Given* `docs/how-to/rollback-web-build.md`, *Then* it lists, in order: (1) confirm `--tmux-keep-server`; (2) flip `WEB_BUILD ?=` back to `next` (valid only until PR 9 merges) or revert PR 8; (3) `make install-service` in a planned window; (4) if the Bun jobs had joined `web-build-gate`, remove them with `gh variable set WEB_BUILD_GATE_MEMBERS --body "detect-changes,web-build-smoke-next,e2e-next"` and read it back with `gh variable get WEB_BUILD_GATE_MEMBERS` (a repository variable: no workflow edit, no PR, no branch-protection change, because required-check names never change; the same command is the only way a Bun job joins); only if the gate itself must go, run the tested `gh api` command of Task 8.5c; (5) post-checks (console error watch, login assets 200); and has a separate PR 8b section listing the complete revert set (Makefile `WEB_MINIFY ?= 0` line, the CI default of Task 8.6a, the `minifyExplicit` assertion, the service restart under the same `--tmux-keep-server` precondition), and states plainly that after PR 9 merges there is no rollback to Next, only a forward fix, and that the `web-dual-build-last` tag restores both builds only by discarding every later commit.
- The precondition is enforced, not just documented (pre-mortem failure mode 8).
  - *Given* `make install-service`, *When* the deployed unit lacks `--tmux-keep-server` (the same two greps as above) and `ACCEPT_SESSION_LOSS=1` is not set, *Then* the target exits non-zero before building, with a message naming `docs/explanation/tmux-keep-server-on-restart.md`; with the flag present, or with `ACCEPT_SESSION_LOSS=1`, it proceeds. A `make -n`-based shell test with fake unit files covers the three cases. This replaces "or record the decision to accept loss" as the only escape.
- Rollback is rehearsed once.
  - *Given* a ManualInstance on its private tmux socket, *When* the flip is rehearsed before PR 8 (flip the default, rebuild, restart the ManualInstance with `--tmux-keep-server`), *Then* its sessions survive and the steps and timings are written into the runbook.
- The required-check removal command is tested.
  - *Given* the runbook's command `printf '["web-build-gate"]' | gh api -X DELETE repos/<owner>/<repo>/branches/main/protection/required_status_checks/contexts --input -` (`--input -` reads the body from stdin; the earlier `--input <(echo ...)` form is a shell process substitution that is not portable; UNVERIFIED until Task 8.5c runs it), *When* it is run against a scratch repository with a protected branch (the only place it can run: `main` of this repository has no protection today, so the real-repo flow is the first-time setup of Story 5.11 and this command is only ever the removal path), *Then* the command as run and a read-back (`gh api repos/<owner>/<repo>/branches/main/protection/required_status_checks --jq .contexts`) showing the check gone are pasted into the runbook.
**Files**: `Makefile` (`install-service` preflight), `scripts/install-service-preflight.test.sh` (new), `docs/how-to/rollback-web-build.md` (new), `project_plans/web-app-bun-build/research/cutover-gate-results.md`
##### Task 8.5a: Check the deployed unit and record the result (S)
##### Task 8.5b: Write the runbook (S)
##### Task 8.5c: Run the `gh api` required-check removal on a scratch repo and paste the read-back (S)
##### Task 8.5d: `install-service` preflight with a shell test (S)
##### Task 8.5e: Rehearse the rollback on a ManualInstance (S)

#### Story 8.6: Enable minification (PR 8b)
**As the** owner, **I want** minification switched on as its own revertible step after the bundler has soaked, **so that** a production-only minifier bug is attributable (pre-mortem failure mode 4; ADR-004).
**Acceptance Criteria**:
- Minified gate.
  - *Given* the first Soak window passed (Story 8.4 exit criteria), *When* `WEB_BUILD=bun WEB_MINIFY=1 make web-build` is embedded into a ManualInstance with `--remote-access` and the full Playwright suite runs 3 times through `scripts/e2e-baseline-compare.ts`, plus the Axe sweep, *Then* the failing set is a subset of the allowed baseline set exactly as in Story 8.1.
- Name dependence.
  - *Given* `rg "constructor\.name|\.name ===|Function\.name" web-app/src` hits are reviewed and any spec that failed only minified is rerun with `WEB_MINIFY=0`, *Then* every minified-only difference is fixed, or has a linked issue whose body carries an owner-written accept note ("accepted by <owner> on <date>: <reason>") copied into `cutover-gate-results.md`; a difference with neither fails Task 8.6b, and PR 8b does not open (a bare issue link is not a waiver).
- The flip is one line.
  - *Given* the Makefile line `WEB_MINIFY ?= 0` deleted, *When* the cutover build runs, *Then* `stats.minify === true` and `stats.minifyExplicit === false` (validation A3), the byte budgets hold on the deployed output, and reverting PR 8b is the whole rollback.
- Revert set is complete (review R2-C7).
  - *Given* PR 8b reverted, *Then* all four pieces are back: the Makefile line `WEB_MINIFY ?= 0`; the CI default that Task 8.6a changed (same PR, so the revert covers it); `verify-output`'s assertion in its explicit-flag path (`stats.minify === false`, `stats.minifyExplicit === true`, the state the restored line produces); and the deployed service restarted with `make install-service`, so the Story 8.5 `--tmux-keep-server` preflight applies. CI keeps enforcing the byte budgets on its separate minified build, so after a revert budgets are checked on minified bytes while the deployed bytes are unminified; the runbook says so.
- Results appended.
  - *Given* `docs/explanation/bun-migration-results.md`, *Then* it gains the minified-default numbers (total JS bytes, build time, binary size) and the completeness test of Task 8.3c still passes.
**Files**: `Makefile`, `docs/explanation/bun-migration-results.md`, `project_plans/web-app-bun-build/research/cutover-gate-results.md`
##### Task 8.6a: Delete the Makefile `WEB_MINIFY=0` line and update CI defaults (XS)
##### Task 8.6b: Minified suite gate, name-dependence review, results append (M)

---

## Phase 9: Remove Next (PR 9, after the Soak)

### Epic 9.1: Delete the dual-build path
**Goal**: No `next` dependency left in `web-app/package.json`, no dead code, tagged restore point. **This PR is the end of the rollback window**: once it and any later PR merge, restoring Next means a forward fix; `web-dual-build-last` is an emergency tree, not a revert (reverting the range discards every commit merged since and conflicts with the deleted `layout.tsx` files).

#### Story 9.1: Delete Next application code and aliases
**As a** developer, **I want** only one build, **so that** the code tells the truth.
**Acceptance Criteria**:
- *Given* tag `web-dual-build-last` created on the last commit with both builds, *When* the PR deletes `web-app/next.config.ts`, `next-env.d.ts`, all 16 `layout.tsx`, `export const metadata/viewport` in pages, `src/app/fonts.ts`, `impl.next.tsx`, `lazy/impl.next.tsx`, the webpack aliases, and `RootShell`'s Next caller, *Then* `rg "from ['\"]next" web-app/src` returns nothing and `tsc` passes for the single tsconfig (`jsx: react-jsx`, no `next` plugin).
- *Given* `RouteMeta`, *Then* it is merged into `routes.tsx` data and its drift test is replaced by a title-table test against the e2e fixture.
**Files**: the files above; `web-app/tsconfig.json`; `web-app/src/lib/nav/index.ts`
##### Task 9.1a: Tag, delete layouts/metadata/config (M; mass-delete exemption, decision 13)
- Deletion manifest and metadata-removal script: decision 13, row 9.1a; the PR body pastes the manifest and the three verification outputs.
##### Task 9.1b: Collapse alias seams into direct imports (S)
##### Task 9.1c: Fold `RouteMeta` into `routes.tsx` (S)

#### Story 9.2: Delete Next dependencies, scripts and CI/Makefile branches
**As the** owner, **I want** the footprint gone, **so that** the original goal is met.
**Acceptance Criteria**:
- *Given* `web-app/package.json`, *When* `next`, `eslint-config-next`, `@next/eslint-plugin-next`, `@vanilla-extract/next-plugin` (and `@storybook/nextjs` if still present) are removed, *Then* `rg '"next"' web-app/package.json` is empty and `bun install --frozen-lockfile`, `make ready`, Jest and the e2e suite pass.
- *Given* `Makefile`, workflows and `.github/actions/prepare`, *When* `WEB_BUILD`, `NEXT_BUILD_MODE`, `NEXTJS_SHARED_CACHE_DIR`, `.next/cache` steps, the `web-app/.web-build-mode` stamp, and the `web-build-smoke-next` and `e2e-next` jobs are deleted (with their entries in `web-build-gate`'s `needs:`), *Then* the Bun build is the only build and `.gitignore` drops `/web-app/.next`. Ordering: run `gh variable set WEB_BUILD_GATE_MEMBERS --body "detect-changes,web-build-smoke-bun,e2e-bun"` **before** merging, because the gate fails on a member name that is absent from `needs:`.
**Files**: `web-app/package.json`, `bun.lock`, `Makefile`, `.github/workflows/*.yml`, `.github/actions/prepare/action.yml`, `.gitignore`
##### Task 9.2a: Dependency removal and lockfile (S)
##### Task 9.2b: Makefile/CI branch removal (M)
##### Task 9.2c: `no-next-dependency.test.mjs` added by this PR so the dependency cannot return (XS)
- Validation A11 / REQ-M6: asserts `web-app/package.json` has no `next`, `eslint-config-next`, `@next/*`, `@storybook/nextjs` or `@vanilla-extract/next-plugin`, and that no file under `web-app/src` imports from `next`; it has a seeded failing fixture.

#### Story 9.3: Final docs grep and closure
**As a** future contributor, **I want** no stale Next guidance and no build-mode residue in tests, **so that** nobody reintroduces either.
**Acceptance Criteria**:
- Docs.
  - *Given* `rg -n "next dev|next build|WEB_BUILD|Next\.js" CLAUDE.md docs .claude project_plans README.md tests`, *Then* every remaining hit is historical (ADR text or the migration results doc) and marked as such; ADR-001..008 in `project_plans/web-app-bun-build/decisions/` move to Accepted and stay there; promotion into `docs/adr/` is optional and, if the owner asks for it, each file is renumbered to the next free number (the directory already has `ADR-001-gemini-exit-code-contract.md`, `ADR-002-workspace-host-registry-gossip.md` and two ADR-023 files) with the mapping recorded in its header; the memory/project notes are updated.
- Test residue (architecture review).
  - *Given* the conditionals added by Phase 7, *When* Task 9.3b runs, *Then* `rg "WEB_BUILD|webBuildMode" tests/ scripts/dev-stack` returns nothing: `tests/e2e/helpers/web-build.ts` is deleted, the `process.env.WEB_BUILD` use in `tests/e2e/helpers/test-server.ts` and `scripts/dev-stack/launch.ts` is removed, the `=== "bun"` branches in Stories 7.2b/7.5b/7.7b/7.12/7.13b become plain assertions, the Next-baseline fixtures in `tests/e2e/fixtures/` that only recorded Next values are deleted or converted to plain assertions, and `docs/how-to/web-dev-server.md` no longer mentions `WEB_BUILD`; the e2e suite and Jest pass.
**Files**: docs, `CLAUDE.md`, ADRs, `tests/e2e/**`, `scripts/dev-stack/launch.ts`
##### Task 9.3a: Grep (now including `tests/`), edit, set ADR statuses, optional ADR promotion with renumbering (S)
##### Task 9.3b: Remove `WEB_BUILD` conditionals and fixtures from tests, delete `web-build.ts` and `scripts/dev-stack/web-build-mode.ts` (S)
##### Task 9.3c: `adr-status.test.mjs`: fails if any `decisions/ADR-*.md` still says `Status: Proposed` (XS; validation A9, REQ-S7)

#### Story 9.4: Optional follow-up PR, rename the asset prefix (ADR-003)
**As the** owner, **I want** to drop the `_next` name, **so that** URLs stop referencing a removed framework.
**Acceptance Criteria**:
- *Given* `exemptPrefixes` in `auth.go:95-101` and the immutable rule at `static.go:76-78`, *When* both change to `/assets/` in one PR together with `build.ts` naming, `web-app/.size-limit.json` globs, `built_assets_test.go` and docs, *Then* the Go test enumerating built asset URLs passes, an unauthenticated `/login` still loads all assets, and `grep -rn "_next" server web-app tests` returns nothing but historical docs.
**Files**: `server/middleware/auth.go`, `server/middleware/static.go`, `server/middleware/built_assets_test.go`, `web-app/build/build.ts`, `web-app/.size-limit.json`
##### Task 9.4a: Test first, enumeration test with new prefix (S)
##### Task 9.4b: Atomic rename across Go + build (S)

---
## Risk coverage matrix

Every risk in `research/pitfalls.md` (R1-R13), `research/architecture.md` §11 (F1-F17), the UX pre-cutover list (`research/ux.md` §7) and the binding inputs maps to at least one story.

| Risk / requirement | Covered by |
|---|---|
| R1 Go `/_next/` auth exemption and immutable cache | 1.5, 1.6, 0.2 (`out/` walk), 5.5 (`naming.entry`), 5.9, 6.8 (every file), 7.13 (unauthenticated `/login` request log), 8.1, ADR-003, 9.4 |
| R2 Bun CSS bug #45730 | 0.2 (census, two Bun versions), 5.7 (pin), 5.10 (CI diff on every bump), 8.1 |
| R3 `bun patch` not applied (#33520) | 3.2, 3.4, 5.11 (cache keys), 8.1 |
| R4 Router: scroll, announcer, `useSearchParams`, trailing slash | 1.3, 1.4, 2.1, 2.6, 4.2-4.6, 7.1-7.4 |
| R5 Supply chain (TanStack, Bun install) | 4.1, 2.8, 6.10, ADR-001 |
| R6 Theme FOUC script, font classes, `<html className>` | 5.3, 5.4, 7.5 |
| R7 `NEXT_PUBLIC_*` and `NODE_ENV` inlining | 0.2, 5.5, 8.1 |
| R8 Dev-server fidelity, HMR cycles, memory | 0.3, 5.6, 8.4, ADR-007 T1 |
| R9 Storybook, ESLint, Jest assumptions | 2.2, 2.9, 6.1, 6.2, 6.6 |
| R10 CI caches, `size-limit`, Lighthouse | 5.11, 6.4, 6.5, 9.2 |
| R11 Bun runtime crashes | 5.5 (short-lived build, no `bun test`), 5.6/5.8 (supervised dev server), T3 |
| R12 Strict CSS hiding old bugs | 0.2 (`throw:true`), 5.5, 8.1 |
| R13 jscpd threshold | 2.2 (shared mock), 6.9 |
| F1 Partial migration | 2.9 (lint ban), 5.2 (resolver ban), 9.1 |
| F2 Dual-build drift | 5.11, 7.10, 7.11, ADR-006 |
| F3 Stale-mode artifact | 5.7 (ModeStamp) |
| F4 Login lockout | 1.5, 0.2, 5.5, 5.9, 6.8, 7.13, 8.1 |
| F5 HTML-as-JS from SPA fallback | 1.6, 2.7 (StaleChunkReload) |
| F6 Theme mismatch | 5.4 (guard in `verify-output`), 5.9 |
| F7 Dev invalidation gap | 0.3, 5.1 |
| F8 Minification regression | 8.1, ADR-004 |
| F9 Router drift | 1.3, 1.4, 4.7, 7.1-7.4 |
| F10 Stale-tab skew | accepted; mitigated by 1.6 and 2.7 |
| F11 Shared `node_modules` poisoning | 3.2 (runbook, `SHARED_NODE_MODULES=0`), 3.3 |
| F12 CI cache poisoning | 5.11, 6.10 |
| F13 Next on a Bun tree | 0.1 (S0), ADR-006 fallback |
| F14 Worktree concurrency / memory | 0.4, 5.5 (OutTmpDir) |
| F15 HMR not as good as spiked | 0.3, ADR-007 amber/T1 |
| F16 Bundler bug | ADR-007 triggers, 8.4 |
| F17 Source maps in the binary | D3, 5.9, 8.3 |
| UX: per-route titles | 1.2, 4.3, 4.4, 7.1 |
| UX: focus and route announcer | 4.5, 7.2 |
| UX: history / query params, 4 raw History sites | 2.6, 4.2, 4.6, 7.3 |
| UX: trailing-slash deep links | 1.3, 7.4 |
| UX: theme FOUC for 6 themes + custom | 5.4, 7.5 |
| UX: viewport meta | 5.3, 7.6 |
| UX: unknown-path behaviour | 4.3, 7.7, D1 |
| UX: Axe sweep gaps (13 routes) | 7.8 |
| UX: Lighthouse config pick | 6.5 |
| UX: `ux-analysis.yml` path filter widening | 6.5 |
| 5 `NEXT_PUBLIC_*` + `NODE_ENV` defines | 5.5 |
| FOUC script from the same VE cache | 5.4 |
| VE plugin dependency invalidation | 5.1 |
| Patch-applied canary and CI check (Bun-side, with a negative test) | 3.0, 3.4 |
| Bun-side tests have a runner (adversarial B1) | 3.0 and the test-location rule (Plan-level decision 6); 3.4, 4.1, 5.1-5.6, 5.9, 6.4 |
| Playwright baseline on Next and Next-quirk triage (adversarial B2) | 0.1c, 7.10, 7.11, 8.1 |
| Spike and feasibility doc committed | 0.7 |
| Rollback cost: `--tmux-keep-server`, required-check names, post-PR-9 expiry | 8.2, 8.5, 9.1, Migration Plan ladder |
| Pre-mortem P1-1: route-order-dependent CSS (#45730 / T4) | S3 gate row, 0.4 (Task 0.4e), 5.10 (Task 5.10c), 7.9 (Task 7.9b), 8.1 |
| Pre-mortem P1-2: stale-chunk guard blind to controlled inputs | 2.7 (dirty-set listener, Tasks 2.7a/2.7e), 7.2 (Task 7.2c) |
| Pre-mortem P1-3: CI-only workflow edits, required-check deadlock | Plan-level decision 10, 5.11 (Tasks 5.11b-d: gate script and fixtures, `actionlint`, PR-event proof on a scratch repo), 7.10 (Task 7.10f), 8.2, 8.5 (Task 8.5c) |
| Pre-mortem 4 (bundler and minifier change together), 5, 6, 7, 8, 9, 10, 11 | 8.6 and ADR-004; 1.2 (Task 1.2c) and 2.9 (Task 2.9e); 0.1, 7.10 (Tasks 7.10d-e); 3.4, 5.5; 8.5 (Tasks 8.5d-e); 8.4, 0.1d, overrun checkpoint; 5.9, 8.1; 7.2b |
| Validation A1-A12 | 5.9 (A1), 8.3 (A2, A4), 5.5 and 8.6 (A3), 3.0 (A5, A6), 5.5 Task 5.5d (A7, optional), 8.1 (A8), 6.7 and 9.3 (A9), 2.7 (A10), 9.2 (A11), 7.2b, 7.7b, 7.8 (A12) |
| Route-level pending UI, error inside the shell, not-found content (UX G1-G3) | 4.3, 4.4, 7.7 |
| Announcer and focus edge cases, nested `main#main-content` (UX G4-G5) | 4.5, 7.2 |
| Stale-chunk storage fallback and unsaved input (UX G6) | 2.7 |
| Dev overlay, build error naming, port in use (UX G7) | 5.5, 5.6 |
| Scroll policy coverage (UX G8) | 4.6, 7.12 |
| `?window=` preservation and `/login` navigation (UX G9) | 7.4 |
| Theme CSS blocking link, boot script when storage throws (UX G10) | 5.3, 5.4, 7.5 |
| Bun pin and Bun CSS diff against Next | 5.7, 5.10 |
| `web-vitals` swap and FID note | 2.8 |
| `scripts/dev-stack/launch.ts`, Playwright dev-mode config | 5.8 |
| `web-app/.size-limit.json` and `web-app/package.json` duplicate | 6.4 |
| `lint.yml` reliance on `next lint` exit code | 6.3 |
| `.eslintrc.json` `next/core-web-vitals` replacement | 6.1, 6.2 |
| CI `.next/cache` steps | 6.5, 9.2 |
| Go middleware coupling to `/_next/` | 1.5, 1.6, 6.8, 9.4 |

---

## Amendments
*(Filled by Story 0.6 if a spike gate is not PASS; empty at plan approval.)*

### PR 4 amendments (gate S4 findings; `research/spike-results.md` on `docs/gate-s4-react-router`)

1. **44 routes, not 42.** `find src/app -name page.tsx` and `ROUTE_META` both give 44. `RoutePath` (`src/lib/nav/paths.ts`) is now the 44-literal `ROUTE_PATHS` tuple; `ROUTE_META` is `Record<RoutePath, ...>` and `routeTable` `satisfies Record<RoutePath, RouteDef>`, so a missing key fails `tsc`. Wherever Stories 4.3 and 7.14 say 42, read 44.
2. **Story 4.3: guarded paths are four, not three.** `/backlog`, `/backlog/board` (guard `backlog`), `/changelog`, `/settings/backlog-sources`. `FeatureGuard` delegates to `RequireFeatureFlag`; `routes.tsx` wraps by `RouteMeta.guard` and stores no flag of its own.
3. **Story 4.2 and 4.3: redirects.** `<Redirect>` passes `replace` (RR's `<Navigate>` pushes by default; gate S4 row 4), and `RouteDef` redirects are route `loader`s that throw `redirect()`, so the redirecting URL never commits (no extra history entry, one announcement). `useRouter().replace` always passes `{replace:true}`; any call site that meant "replace" must say so.
4. **Story 4.2: `useReplaceSearch`.** It navigates with `{replace:true}` and then re-merges foreign top-level `history.state` keys (an RR replace rewrites state to `{usr,key,idx}` and drops `ssqTray`; gate S4 row 6). Raw `replaceState` is not observed by RR, so only the router impl's version updates `useSearchParams` (gate S4 row 5).
5. **Story 4.2: `usePathname`** normalises the trailing slash itself (RR keeps `/x/` in `pathname`; gate S4 row 1). Next client navigations produce `/backlog/`, RR keeps `/backlog`; the e2e baselines in 7a/7b must not assert the slash.
6. **Story 4.4: title.** The title is a root-rendered `<title>` fed by `titleFor`; no `useEffect` fallback (gate S4 row 2). `index.html` (Story 5.4) must ship **no** static `<title>`: a static one is not replaced and leaves two `<title>` elements.
7. **Story 4.6 is rewritten (replaces the single-container ACs of Stories 4.6 and 7.12).** Gate S4 row 3: the window never scrolls and `<ScrollRestoration>` only manages `window`, so it is not used. Scrolling happens in per-page inner elements (16 of 44 routes at 1280 px on an empty instance, a lower bound) and in a shell `div` at 390 px. `ScrollPolicy` therefore records `scrollTop` of every element that fires a scroll event, keyed by `location.key` and an element descriptor (id, else tag/sibling-index path), and:
   - *Given* a forward navigation to a new normalised pathname, *Then* `window` and every scroller seen so far are reset to 0, unless the navigation carried `preventScrollReset`.
   - *Given* Back or Forward to an entry with recorded positions and a different pathname, *Then* each recorded scroller found by descriptor is restored, retried every 50 ms for up to 1.5 s while a lazy page grows the content, and the retry stops at the first wheel, touch or key input.
   - *Given* a search-only or hash-only change, *Then* nothing moves.
   - *Given* the tray entry (`pushTrayEntry`), *Then* Back closes the tray, the route and RR's render count do not change, and `ssqTray` is on the earlier entry again after Forward (jsdom test over `createBrowserRouter`).
   Story 7.12's "the scroll container recorded by S4 (window or `<main>`)" becomes "the inner scrollers listed by the S4 probe"; its "within 2 px" is a new behaviour, not parity (Next's own Back behaviour for inner scrollers was not measured).
8. **Story 4.5: announcer.** Next's built announcer is `aria-live="assertive"` / `role="alert"` (VERIFIED by S4); the Bun announcer stays polite. To make "exactly one announcement" hold for page-level redirects, the announcer waits 100 ms after a pathname change before reading `document.title`, and clears the text 1 s after announcing.
9. **Story 4.1: supply-chain gate placement.** `check-dependency-screen.test.mjs` is a `node:test` file in `web-app/scripts/__tests__/` (the path in the story's Files line, run by `bun run test:codemods`), not a `build`-project Jest test. Install scripts are not in `bun.lock`, so the script reads them from installed manifests (the isolated store's `name@version[+hash]` directories) and says so by skipping packages that are not installed.
10. **Story 4.7: file names.** The contract is `__tests__/nav-contract.tsx` (it contains JSX). Data-router suites import `__tests__/fetch-globals.ts` first: jest-environment-jsdom has no `Request`/`Response`/`Headers`, and Node's `Request` rejects a jsdom `AbortSignal`, so all five are borrowed from the outer realm.
11. **`impl.memory` `Link`** now leaves same-page `#hash` hrefs to the browser, matching the other two implementations (found by the contract suite).
12. **Not done in PR 4, still owed:** fonts and the theme-class FOUC script that `layout.tsx` emits (Story 5.4 / PR 5a), and the Playwright checks for the focus ring and the reduced-motion skeleton (Stories 7.x; Jest covers the source rules only).

---

## Repair log
*(Phase 3 repair, 2026-10-19. Inputs: `adversarial-review.md` (BLOCKED), `architecture-review.md` (CONCERNS), the UX gap list G1-G10. Story IDs are stable; added stories are 0.7, 1.7, 3.0, 7.11, 7.12, 7.13, 8.5; added tasks are named in each story. Totals after repair: 79 stories, 178 tasks, 12.68M CU before verification, 22.2M CU verified; contingencies C1-C3 add 1.06M / 1.85M. Files changed: this plan, ADR-001/002 (impl file names), ADR-001/005/006 (amendments), `design/ux.md` (gap-resolution table).)*

### Adversarial blockers
| ID | Resolution | Stories |
|---|---|---|
| B1 | `build` Jest project shelling out to a pinned `bun` via `runInBun`; test-location rule; PatchCanary runs under Bun with a negative test against a committed unpatched fixture | 3.0 (new), 3.4 (rewritten, Task 3.4c), Plan-level decision 6; test paths re-pointed in 4.1, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.9, 6.4 |
| B2 | Playwright `E2eBaseline` x3 on Next, build-coupled spec inventory (insights spec lines verified: comment 88-92, doubled mock 94-95), triage story with a 12-spec cap, Bun e2e cell required from first appearance with a shrink-only expected-diff list, 8.1 compares against the committed baseline | 0.1 (Task 0.1c), 7.10, 7.11 (new), 8.1 |
| B3 | Walk of every `out/` file against `auth.go` exempt prefixes (spike, `verify-output`, Go test) with a reasoned allowlist; unauthenticated `/login` Playwright request log; `naming.entry` set in 0.2 and pinned in 5.5 (verified: spike `build2.ts` used `entry:"[name].[ext]"`, `chunk:"chunks/..."`, `asset:"assets/..."`, none exempt) | 0.2 (Task 0.2d), 5.5, 5.9, 6.8 (rewritten), 7.13 (new), 8.1 |

### Adversarial concerns
| Concern | Resolution | Stories |
|---|---|---|
| Epic 7 ordering | Split into 7a (Next baselines, before PR 2a) and 7b (Bun-only assertions, after PR 5a); single `webBuildMode()` helper | Plan-level decision 3, Phase 7 intro, 7.2b, 7.5b, 7.7b, 7.10, 7.12, 7.13b |
| Rollback cost (`--tmux-keep-server`) | Precondition and runbook story; required-check names | 8.5 (new), 8.2 (Task 8.2b), Migration Plan ladder, Risk Control |
| Revertibility after PR 2 / PR 9 | Per-PR rollback table; ladder with explicit expiry; tag is an emergency tree | Migration Plan, 9.1 goal, ADR-006 amendment |
| Weak parity gates | 6 themes, resolved custom-property maps, interaction states, reasoned allow-list; census compares resolved maps | 0.2, 0.4, 7.9, 5.10, S1/S3 gate rows |
| Budgets loosened, no Next baseline | Task 0.4d (Next first-load per route, status of 5 MB gate); Initial JS ceiling rule; owner decision; ADR-005 amendment | 0.4, 0.6, 5.9, 6.4 |
| Spike not in repo | Spike and feasibility doc committed first | 0.7 (new) |
| Added scope unbounded | Finite criteria: D5 `FOCUS_ON_NAVIGATE` (non-gating), Axe sweep Next-also vs Bun-only, flat config classified-diff tolerance (<= 2%, one repair wave, 6a does not gate PR 8), Lighthouse one-config warn-only | Plan-level decision 7, D5 row, 4.5, 6.2, 6.5, 7.8 |
| PR 2 races with concurrent PRs | Rebase protocol: codemod published, back-compat jest stub, ban `warn` then `error` in 2e | Phase 2 intro, 2.2, 2.9 (Task 2.9d) |
| Recorded-only S2/S3 numbers | `madge` informational with data pack for amber; Initial JS tied to Next baseline | S2/S3 gate rows, 0.3, 0.4 |
| StaleChunkReload flag never cleared | Cleared after first successful route render; keyed by build id | 2.7 |
| Single-platform evidence | Explicit "macOS unverified" line; contingency C3 | 0.6, Effort estimate (C3) |
| Fallbacks not costed | Contingencies C1, C2, C3 as named lines outside the base | Effort estimate |
| Dev vs prod scripts in dual mode | Script inventory with line numbers (`package.json:15-17`; Makefile 218, 238, 247, 258, 269, 540, 562) | 5.7 (Task 5.7d) |

### Adversarial minors
Counts derived, not hard-coded (337 `.css.ts`, 65 importers, 26 metadata files, 42 pages, 152 specs): Plan-level decision 8, 3.2, 5.10, 2.5, 4.3. Cited lines corrected (`useUpNextTab.ts:69`, `UnfinishedTab.tsx:144`): Tech Debt Disposition, 1.4, 2.6. `.bun-version` marked new, #1100 rebase gate: 5.7, Unresolved Questions, 3.x header. `process.env` grep demoted to warning with allowlist: 5.9. `routes.tsx`/`NotFound.tsx`/`FeatureGuard.tsx` live in `src/app/`: stated in Tech Debt Disposition, asserted in 4.3. PR 5/6 splits: see architecture item 3. **Not fixed**: the requirements.md text ("41 routes", "Large") because this repair may not edit that file; the plan states the correct values.

### Architecture concerns
| # | Resolution | Stories |
|---|---|---|
| 1 Root `errorElement` drops shell | Pathless shell layout route, `errorElement` on a pathless child; AC that skip link and nav survive a chunk failure (also UX G2) | 4.4 (Task 4.4c) |
| 2 Single source of truth | `RouteDef` drops title and guard (looked up from `RouteMeta`); key-set equality test; `FeatureGuard` introduced under Next and consumed by the three layouts before the Bun path | 1.7 (new), 1.2, 4.3 |
| 3 PR decomposition | PR 2 -> 2c, 2d, 2a, 2b, 2e; PR 5 -> 5a, 5b; PR 6 -> 6a (lint), 6b; each with its own rollback; dependency diagram, PR table, cross-references renumbered | Migration Plan, Phase 2/5/6 intros, Dependency Visualization |
| 4 VE cache key | Content hash, bounded LRU, full-reload fallback for token-file edits | 5.1 (Task 5.1c), 5.6, 0.3 |
| 5 FOUC | `theme-boot-script.ts` pure builder; three placeholder candidates tested against Bun's minifier in the spike, exact form recorded | 0.2 (Task 0.2d), 5.4, 5.3 |
| 6 Facade | `resetScroll`; `RoutePath` claim narrowed (glossary, 2.1, ADR-001); typed search-param helpers scheduled; one in-memory impl (`impl.memory.tsx`); NavContract tests the real Next wrapper and states its limits; impl files are `.tsx`, `Link`/`Redirect` are impl members | 2.1 (Task 2.1d), 2.2, 2.5, 2.9, 4.2, 4.7, ADR-001/002 |
| 7 WEB_BUILD residue; branch protection | Task 9.3b removes conditionals, helper and fixtures, grep extended to `tests/`; required-check names in the 8.2/8.5 runbook | 9.3, 8.2, 8.5, 7.10 |
| 8 tsc coverage of the unselected impl | Each tsconfig includes both impls; `satisfies NavImpl`; mismatch test; Jest `moduleNameMapper` decision | 5.7 |
| 9 Announcer coupling | Reads `document.title`, no `app` import, boundary lint check | 4.5 |
| Nitpicks | 3.3 depends on lockfile location (3.3, Unresolved Questions); single `WebBuildMode` parser (5.8, 7.10); D1 kept as deliberate change (D1 row); 6.2 asserts the `lib` -> `app` boundary still fires (6.2) | as listed |

### UX gaps
| Gap | Stories |
|---|---|
| G1 route pending UI | 4.3 (Task 4.3d), 4.4 |
| G2 error inside shell | 4.4 |
| G3 not-found content | 1.2, 4.3c, 7.7b |
| G4 announcer / focus edge cases | 4.5, 7.2b |
| G5 nested `main#main-content` | 4.5, 7.2b, Tech Debt Disposition |
| G6 storage unavailable, unsaved input | 2.7 (Task 2.7d) |
| G7 overlay, stage-named build errors, port in use | 5.5, 5.6 (Task 5.6d) |
| G8 scroll e2e | 7.12 (new) |
| G9 `?window=`, `/login` hard navigation | 7.4b |
| G10 blocking theme `<link>`, storage-throws boot | 5.3, 5.4, 7.5b |
Also: Story 7.6 asserts `<html lang="en">`; title specs use auto-retrying `toHaveTitle` (Phase 7 intro, 7.1).

### Iteration 2 repair (re-review N1 and new concerns)
| Item | Resolution | Stories |
|---|---|---|
| N1 BLOCKER `.gitignore:95 build/` ignores `web-app/build/` | Chose negation `!/web-app/build/` after line 95 over renaming the directory (a rename touches every later story, three ADRs and Makefile prerequisites). VERIFIED in a scratch repo with `build/` + `!/web-app/build/`: `web-app/build/build.ts`, `__tests__/a.test.ts`, `__tests__/bun/x.bun.ts`, `plugins/p.ts`, `.out-tmp/q` all exit 1 from `git check-ignore`; `other/build/z` still exits 0; without the negation `git check-ignore -v web-app/build/build.ts` reports `.gitignore:95:build/` (run in this worktree). AC plus a `gitignore.test.ts` guard in Story 3.0; `.bun-version` moved from Story 5.7 to 3.0; `build/**` added to the `tsconfig.json` `exclude` with a `next build` type-check run (INFERRED until run) | 3.0 (new Task 3.0d), 5.7, 6.9 |
| Story 3.0 transform reference | Copy the `dev-stack` ts-jest project (`jest.config.js` 66-84), not `eslint-plugin-analytics` (`transform: {}`); `roots` stated; `bun-coverage.test.ts` fails on any `*.bun.ts` not driven through `runInBun` (with a seeded failing fixture) | 3.0 |
| Story 3.4 probe | Asserts exit status and `json.ok === false`, not error text (unpatched eval under Bun throws `undefined is not an object (evaluating 'p.sep')`) | 3.4 |
| B2 gaps | Baseline stores every attempt (`retry`, `status`); classification and gate rule defined (fails = final failure in >= 2 of 3 Bun runs; 1 of 3 = warning; baseline `flaky` needs >= 1 of 3 passes; baseline `fail` excluded but reported; specs absent from the baseline judged against the same-commit Next cell, missing report fails); `scripts/e2e-baseline-compare.ts` moved from 8.1 to 7.10 (Task 7.10c) with six fail-case fixtures and four pass-case fixtures; `bun-expected-diff.json` needs a local first Bun run before PR 7b opens; Task 7.11c is now a `#####` heading | 0.1, 7.10, 7.11, 8.1 |
| B3 HTML location | Spike must prove A' (move the emitted `_next/static/chunks/index-<hash>.html` to `out/index.html` in the `html` stage and delete the original) or B (script entry + `html.ts`); `out/index.html` AC added to 0.2 and the S1 gate row, an HTML-lands-at-root AC and mocked-output stage test added to 5.5; walk runs after the move | 0.2, S1, 5.5 |
| B3 remote-auth feasibility | Task 0.2b (now M) proves the remote TLS `/login` path from Playwright (`ignoreHTTPSErrors`/CA) with a Go-test fallback (`login_assets_test.go`, real `out/` through `middleware.Auth` with an always-reject validator over `httptest`); 7.13 uses whichever 0.2b records | 0.2, 7.13 |
| B3 allowlist ordering | `public-allowlist.json` now created in Story 5.9 (PR 5a, first consumer) with a `{path, reason}` schema check and an `ls web-app/public` coverage test; 6.8 and 7.13b consume it; 6.8's skip-if-absent becomes a failure when `CI` or `REQUIRE_BUILT_OUT=1` (set by `web-build-smoke`) | 5.9, 6.8, 7.13 |
| Smaller | Story 3.0 added to the rebase-gate lists (Wall-clock blockers, Unresolved Questions); contingency totals restated (see below) | Effort estimate |

Totals after iteration 2: 79 stories, 181 tasks (3.0d, 7.10c, 7.11c added), 11.77M raw, 12.95M CU before verification, 22.7M verified (range 19.4-25.9M). Contingency arithmetic: on the previous base the corrected figures are 12.682M + 1.056M = 13.74M and 22.19M + 1.848M = 24.04M; on the new base they are 14.00M and 24.51M. Not edited by this repair: `adversarial-review.md` checklist boxes, `requirements.md`.

### Iteration 3 repair (Phase 4: consistency.md, pre-mortem.md, validation.md)
*(2026-10-19. Story IDs unchanged; new: Story 8.6 (PR 8b) and 27 tasks. Totals after this repair: 80 stories, 208 tasks, 13.165M raw, 14.48M CU before verification, 25.3M CU verified (range 21.7-29.0M); contingencies C1-C3 unchanged (+1.056M / +1.848M), so base plus all is 15.54M / 27.19M. 18 PRs (8b added) plus the optional Story 9.4 PR. Files changed: this plan, requirements.md, design/ux.md, ADR-001, ADR-004 (rewritten, file name kept), ADR-008, and one status line at the top of docs/explanation/bun-migration-feasibility.md.)*

| Finding | Resolution | Stories changed |
|---|---|---|
| Consistency B1 (`NODE_ENV` contradiction) | `NODE_ENV="production"` for every `build.ts` invocation, only `dev.ts` sets `"development"`; `WEB_BUILD_MODE` and `--mode` dropped, `WEB_MINIFY` is the only profile switch; Story 5.5 test rewritten; ADR-004 Decision 1 aligned | 5.5, ADR-004, decision 11 |
| Consistency B2 (lint ban vs router entry) | Chose strict ban plus `src/lib/nav/entry.tsx` (`createAppRouter`, `AppRouterProvider`) inside the already-exempt directory over widening exclusions: the facade stays 8 names, router types stay in one directory, and the Next build never bundles the entry. An entry-only import pattern allows it from `src/main.tsx` and tests | 2.1, 2.9, 4.2 (Task 4.2d), 4.4, 4.5, glossary `RouterEntry`, ADR-001 amendment, decision 12 |
| C1 stale size/lineage | requirements.md numbers updated; stale clauses at old plan.md:302 and :1647 removed | Effort totals, Repair log, requirements.md |
| C2 added scope | "Accepted scope additions" list (11 items) in requirements.md; Out of Scope and the zero-regression metric reworded; sign-off AC added | 0.6, requirements.md |
| C3 name collision | `WEB_BUILD_MODE` removed (see B1); glossary `WebBuildMode` says it is not the minify switch | 5.5, glossary |
| C4 | "secondary decision S3" corrected to D3 | ADR-004 |
| C5 focus target | `mainRef` supplied by `createAppRouter` to `RootShell`; no DOM query | 4.5, glossary `FocusPolicy`, Tech Debt row, ux.md S2 edge case |
| C6 Reload vs Retry; stale G6 row | Label is **Retry** (full reload for chunk-load errors, else `onRetry`); the banner keeps its own Reload; "Accepted risk (G6)" row replaced by the banner behaviour | 4.4, ux.md (S11, UX-39, UX-50, exit table, G6) |
| C7 ports | Backend is `PORT=62871`; Bun dev server moved to spare 62875 (not instance #2's 62873); Story 6.7 adds the CLAUDE.md port-block row | 0.3, 5.6, 6.7, glossary `ManualInstance`, ux.md S15/UX-51 |
| C8 `WebBuildMode` parser owner | Story 5.7 creates `parseWebBuildMode(env)` in `scripts/dev-stack/web-build-mode.ts`; 5.8 and 7.10a import it; `webBuildMode()` is a one-line wrapper | 5.7 (Task 5.7e), 5.8, 7.10, decision 3 |
| C9 `dynamic` site count | Counted with `grep -rnE 'dynamic\('`: 6 call sites in 3 files, plus one unused import in `test/terminal-stress/page.tsx` (removed, not migrated); S3, 2.7 and UX-42 unified | S3 row, 2.7, ux.md UX-42 |
| C10 ADR home | ADRs stay in `project_plans/.../decisions/`; promotion into `docs/adr/` (which has ADR-001/002 with other titles and two ADR-023 files) renumbers to the next free number with a header mapping | header note, 6.4, 9.3 |
| C11 `patchedDependencies` | Root `package.json` only (per `research/stack.md:77`, citing Bun docs); the open question is only the lockfile location | 3.2, Unresolved Questions |
| C12 feasibility doc | One-line status note at its top (Bun.build chosen, Rsbuild recommendation superseded, counts predate recounts) | 0.7 |
| C13 5x not gated | PR 8 is gated on a same-host back-to-back `make web-build` ratio of at least 5x (ratio, not the host-dependent 10.8 s); a miss goes to the owner, whose written decision is a precondition of PR 8 | 8.3 (Task 8.3b), 8.2 |
| Pre-mortem P1-1 route-order CSS | 42 routes cold (2 themes), 3 fixed + 3 seeded-random in-session walks compared with cold load on the same build and with Next, rule-selector-order census per theme | S3 row, 0.4 (Task 0.4e), 5.10 (Task 5.10c), 7.9 (Task 7.9b), 8.1 |
| Pre-mortem P1-2 stale-chunk guard | Document-level `input` listener and dirty set replace `value !== defaultValue`; Jest test with a real controlled React input; Playwright omnibar spec with a 404 chunk; telemetry counter; ux.md wording fixed | 2.7 (Task 2.7e), 7.2 (Task 7.2c), ux.md |
| Pre-mortem P1-3 required-check deadlock | Single required `web-build-gate`; matrix cells never named; Bun cells join after 10 green runs on `main` or an owner flip; `workflow_dispatch` run on a throwaway branch per workflow PR; `actionlint` in `make ready`; tested `gh api` removal command | decision 10, 5.11 (Tasks 5.11b-c), 6.5, 7.10, 8.2, 8.5 (Task 8.5c) |
| Pre-mortem 4 (minifier and bundler together) | Cutover unminified; minify is PR 8b (Story 8.6) after the first Soak; budgets enforced on a minified CI build; ADR-004 and requirements metric reconciled (the owner-visible default F4 changes, with the original as the alternative) | 5.5, 5.9, 8.1, 8.2, 8.6 (new), F4, ADR-004, requirements.md |
| Pre-mortem 5 (dual-build drift) | Guidance, rule file and fix-instruction messages move to PR 1 and PR 2b; CI grep and merge freeze | 1.2 (Task 1.2c), 2.9 (Task 2.9e), 6.7 |
| Pre-mortem 6 (baseline masking) | Baseline-`pass` specs fail at 1 of 3 unless Next shows it too; generated route smoke spec; route coverage manifest from `routesVisited`; comparator fixtures 7 fail + 5 pass | 0.1, 7.10 (Tasks 7.10d-e) |
| Pre-mortem 7 (patch drift) | `guard` stage first in `build.ts`; oracle also on `.bun-version`; release/Docker paths audited; shared-hash test uses the real script | 3.3, 3.4, 5.5, 5.10, 6.5 |
| Pre-mortem 8 (rollback kills tmux) | `install-service` preflight (exit non-zero without `--tmux-keep-server` unless `ACCEPT_SESSION_LOSS=1`), recorded before PR 8, rollback rehearsed on a ManualInstance | 8.5 (Tasks 8.5d-e), 8.2 |
| Pre-mortem 9 (organisational) | Soak defaults (>= 14 days and 3 sessions, then >= 7 days), dated PR 9 reminders, 45-day stop-loss, S0 one-command re-run on merged #1100, overrun abort criteria | 8.4 (Task 8.4b), 0.1 (Task 0.1d), Effort estimate, wall-clock blockers |
| Pre-mortem 10, 11 | `verify-output` checks URL strings in the `/login` closure; `curl` also on the remote listener; terminal and omnibar keep their own focus | 5.9, 8.1, 7.2b |
| Validation A1 | `maxWallMs: 10000` check with failing fixture | 5.9 |
| A2 | `scripts/measure-web-build.sh` and the ratio gate (see C13) | 8.3 |
| A3 | Minify default asserted (`stats.minify`, `minifyExplicit`) | 5.5, 8.6 |
| A4 | `results-doc.test.mjs` | 8.3 (Task 8.3c) |
| A5, A6 | `no-live-port` and `no-bun-test` scans with failing fixtures | 3.0 (Task 3.0e) |
| A7 | Offline fixture build, marked optional with a drop rule | 5.5 (Task 5.5d) |
| A8 | Manual passkey sign-in and `setup_token` step | 8.1 |
| A9 | `docs-grep.test.mjs`, `adr-status.test.mjs` | 6.7 (Task 6.7b), 9.3 (Task 9.3c) |
| A10 | Stale-chunk counter AC and test | 2.7 (Task 2.7e) |
| A11 | `no-next-dependency.test.mjs` | 9.2 (Task 9.2c) |
| A12 | UX-12 focus on popstate, UX-17 no active item, UX-50 keyboard checklist | 7.2b, 7.7b, 7.8, 8.1 |
| Nitpicks fixed | N1 (requirements counts), N2 (UX-05 in 4.4, UX-07 in 4.5, UX-12, UX-17, UX-50), N3 (ux.md copy adopted in 4.3), N4 (UX-49 "five specs plus one page object"), N5 ("Task 0.4d", "Task 0.2d"), N6 (D3 row), N7 (18 PRs plus optional 9.4), N8 (108), N9 (ADR-008 path), N10 (`.size-limit.json` survives; `.gitignore` additions owned by 5.7, verified by 6.9), N11 (ADR-008 and 3.1 wording), N12 (glossary), N13 (decision 2), N14 (8.3), N15 (glossary lazy impls and `lazyComponent`) | as listed |
| Not fixed | N16 (ADR-006 "through PR 7" wording: ADR-006 left untouched, the plan's rollback ladder is explicit); `validation.md` and `pre-mortem.md`/`consistency.md` (outside the editable set; `validation.md` UX-51/UX-57 rows still cite port 62873 and its counts still describe the pre-repair test inventory, so Phase 5 follows this plan's ports and a validation refresh is advisable); the `adversarial-review.md` checklist boxes; the `workflow_dispatch`-on-default-branch rule and the `gh api` removal command are stated from GitHub documentation and stay UNVERIFIED until Tasks 5.11c and 8.5c run them | |

Cost-class check (recounted): the 27 added tasks are 9 XS (0.1d, 2.7e, 5.7e, 6.7b, 8.3c, 8.4b, 8.6a, 9.2c, 9.3c), 15 S and 3 M (0.4e, 7.9b, 8.6b): 9 x 20k + 15 x 55k + 3 x 130k = 1,395k raw, matching the Effort estimate rows.

### Triad gap folding
*(2026-10-19, `pm:triad-review`; lens results verbatim in `triad-review.md`, verdict READY TO BUILD. Story IDs unchanged except one new story, 7.14, which receives the former Task 7.10d. Totals after this folding: 81 stories, 216 tasks, 13.385M raw, 14.72M CU before verification, 25.77M CU verified (range 22.1-29.4M); contingencies C1-C3 unchanged, so base plus all is 15.78M / 27.61M. 18 PRs plus the optional Story 9.4 PR, unchanged. Files changed: this plan, `requirements.md`, `design/ux.md`, `implementation/validation.md`, `implementation/consistency.md` (counts only), ADR-003 (one path), `implementation/triad-review.md` (new).)*

| Lens | Finding | Resolution | Where |
|---|---|---|---|
| Product | No `next dev` reload baseline | Task 0.4f measures write-to-DOM time and cold start on `next dev`, n >= 5, median and range; Story 0.3's Bun figures are compared with it, not with the lone 84 ms | 0.4 (Task 0.4f), 8.3 |
| Product | Baselines n=1, cold vs warm unspecified | Protocol: n >= 5 per mode per temperature, median and range; the <= 10 s target is the **warm** median, cold reported without a pass line; Story 8.3's 5x ratio uses warm medians | S3 row, 0.4, 8.3 |
| Product | "Leaner footprint" has no metric | Dependency count, installed package count, `node_modules` size, install time (cold/warm) as before (Task 0.4f) and after (scratch projection in 8.3, re-measured in 9.2a) rows, no pass line | 0.4, 8.3, 9.2 |
| Product | Value argued from build seconds only | Task 0.6c counts merged commits, `build.yml` runs and worktrees per week from git/gh and states the saving in runner-minutes and agent wall-clock (no CU saving claimed); provisional figures and an ICE one-liner in `requirements.md` ("Value and ICE score"), with no downscale recommendation | 0.6 (Task 0.6c), requirements.md |
| Product | Answered Open Questions still open | Seven marked RESOLVED with the answer and owner story; router stays open until Story 0.6 | requirements.md |
| Product | F1 sign-off point vs gate S4 | New section "Owner handoff and sign-off points": F1 is signed in Story 0.6 after S4 reports and before Epic 4; Epics 1-3 and PR 7a do not wait; AC added to Story 0.6 | Plan section, 0.6 |
| UX | `wcag22aa` tags | Story 7.8 runs `wcag2a/2aa/21a/21aa/22aa`; both-builds violations report-only via `KNOWN` with an issue, Bun-only blocks; config asserted by a unit test; axe version check first (UX-59) | 7.8 (Task 7.8a), ux.md |
| UX | Focus ring on programmatic `main` focus | Decision D8: `outline: none` on the layout `main#main-content[tabindex=-1]` only, matching Next, justified in four points; computed-style AC, rule-scope test, Next value recorded in 7.2a (UX-58) | 4.5, 7.2a, D8, ux.md |
| UX | Next announcer politeness INFERRED; screen-reader check | Task 0.5d reads the built Next output into the `GateRecord` and corrects the "assertive" wording if wrong; Story 8.1 gains an NVDA/VoiceOver checklist item (recorded manual check, not a mechanical gate) (UX-62) | 0.5, 8.1, ux.md |
| UX | `RoutePending` reduced motion, double announce | AC in 4.3: `reduced-motion` computed `animation-name: none`; no live region in the skeleton and exactly one live-region change per navigation (UX-60) | 4.3 (Task 4.3d), ux.md |
| UX | iOS Safari checks | Task 7.8b: Playwright WebKit project for the `ssqTray` Back marker and `100dvh` with the virtual keyboard; report-only with a printed reason when infeasible; real-iPhone item in 8.1 (UX-61) | 7.8 (Task 7.8b), 8.1, ux.md |
| UX | 8.1 unauthenticated fetch | Stated as a hard cutover gate that no other decision can waive | 8.1 |
| UX | UX-53 and UX-55 manual | CI-asserted: the dev-mode overlay spec runs in the Bun cell (Task 5.6c) and the stage-failure test runs in `make test-web-build`; 8.1 only cites them | 5.6, 5.5, 8.1 |
| UX | Auto-retrying `toHaveTitle` everywhere | Every one-shot title read in `tests/e2e` converted and a CI grep guard added | 7.1 (Task 7.1b) |
| Eng | 3-5 file boundary and codemod exemption | Plan-level decision 13: boundary, six exemption conditions, the five exempt tasks (2.2b, 2.3a, 2.4a, 2.5a, 9.1a) each with a derived file set, script path and verification command; headings marked | decision 13, 2.2b, 2.3a, 2.4a, 2.5a, 9.1a |
| Eng | Task 5.5b bundles seven stages | Split into 5.5b (runner, `guard`, `clean`, `swap`; S), 5.5e (`theme-map`, `bundle`; S), 5.5f (`html`, `public`; S) | 5.5 |
| Eng | Task 0.1c bundles run, classifier, inventory | Split into 0.1c (run x3; S), 0.1e (classifier, `routesVisited` hook, JSON; S), 0.1f (inventory; XS); cross-references moved | 0.1 and Story 7.10/7.11 text |
| Eng | 6a "independent" vs Makefile chain 5.7 -> 6.3 | Claim removed: 6a has no behavioural dependency but merges after 5a because both edit the `Makefile` lint line; PR table, dependency order, diagram, waves and PR split text aligned (the dependency was kept, not broken: breaking it would need a second Makefile hunk in 5a) | Migration Plan, Phase 6 intro, diagram |
| Eng | Story 7.10 straddles 7a/7b | Route smoke spec moved to new Story 7.14 (PR 7a); Story 7.10 is wholly PR 7b; PR table and decision 3 updated | 7.10, 7.14 |
| Eng | 7.11c not a proper heading | Verified already a `#####` task heading (since iteration 2); Story 7.11 now states that it deliberately straddles (7.11a-b in 7a, 7.11c in 7b) and why | 7.11 |
| Eng | `validation.md` and `consistency.md` stale | `validation.md`: header, inputs line (81 stories, 216 tasks, 18 PRs), five new UX rows (UX-58..UX-62), the A13 amendment row for this folding, counts recomputed; `consistency.md`: counts section refreshed (counts only) | validation.md, consistency.md |
| Eng | `.size-limit.json` paths | `web-app/.size-limit.json` everywhere in the plan and ADR-003 (history in older Repair-log rows left as written) | 6.4, 9.4, Tech Debt, R-matrix, ADR-003 |
| Eng | Flat 1.1 CU / x1.75 on spike and docs rows | Noted, not changed: "Estimate caveat" paragraph with the sensitivity (about 24.7M if those rows used x1.25); headline stays conservative | Effort estimate |

**Cost-class check (recounted):** 8 added tasks: 6 S (0.1e, 0.4f, 0.6c, 5.5e, 5.5f, 7.8b) and 2 XS (0.1f, 0.5d) = 6 x 55k + 2 x 20k = 370k; two re-classes (Task 0.1c M to S, Task 5.5b M to S) = -150k; net +220k raw, matching the Effort estimate rows (Phase 0 +130k, Phase 5 +35k, Phase 7 +55k = +220k with Story 0.4 +55k, 0.5 +20k, 0.6 +55k, 0.1 +0, 5.5 +35k, 7.8 +55k, 7.10 -55k, 7.14 +55k).

**Not fixed, with reasons:** (1) the wave count in "Critical path" is unchanged: the 5.5 and 0.1 splits are sequential inside their stories and add no wave. (2) `wcag22aa` availability in the pinned axe-core, the `bun pm ls --all` count command, WebKit availability on the CI runner and the GitHub-documented behaviours already marked UNVERIFIED stay UNVERIFIED until the tasks that run them execute (Task 7.8a, 0.4f, 7.8b). (3) Story 7.11 still straddles PR 7a/7b by design (state (b) of the triage can only be decided from a Bun run); only Story 7.10 was changed. (4) The flat estimate factors were not re-derived (no data to recalibrate before the first implementer run). (5) `adversarial-review.md` checklist boxes and `pre-mortem.md` untouched.

### Iteration 4 repair (Phase 4 re-review: `adversarial-review.md`, "Re-review (iteration 2: Phase 4 additions)")
*(2026-10-19. Story IDs unchanged; no story added; 3 S tasks added: 2.2d, 5.11d, 7.10f. Totals after this repair: 81 stories, 219 tasks, 13.550M raw, 14.905M CU before verification, 26.08M CU verified (range 22.4-29.8M); contingencies C1-C3 unchanged, so base plus all is 15.96M / 27.93M. Recounted by script: 81 `#### Story`, 219 `##### Task`, every Effort row's classes reproduce its raw figure, per-story task counts equal the heading counts, phase rows sum to 13,550k. Files changed: this plan, `decisions/ADR-006`, `validation.md`. Not edited, outside the editable set: `consistency.md` (its counts section still says 216 tasks / 13,385k) and `adversarial-review.md` (its checkboxes).)*

| Finding | Resolution | Where |
|---|---|---|
| R2-B1 gate cannot be built with one matrix job | Separate jobs, no matrix: `web-build-smoke-next` (the existing job at `build.yml:123`, renamed), `web-build-smoke-bun`, `e2e-next`, `e2e-bun` (`needs: e2e-next`, downloads its JSON report artifact), all in `.github/workflows/build.yml` (the only file where `needs:` can reach them; `build.yml` has no Playwright job today). `web-build-gate`: `if: always()`, `needs:` every existing job plus `detect-changes`, one step running `web-build-gate.mjs` over `toJSON(needs)` and `vars.WEB_BUILD_GATE_MEMBERS`; member `failure`/`cancelled` fails, `success`/`skipped` pass, non-members never fail, unknown member name fails, empty variable means the default `detect-changes,web-build-smoke-next,e2e-next`. Task 5.11b fixture cases: member / non-member / skipped / cancelled / failure / unknown member / default; a second test asserts `if: always()`, no `continue-on-error` and `fail-fast: false` on any matrix | decision 10, 5.11 (rewritten, Tasks 5.11a-d), 7.10 (Tasks 7.10b, 7.10f), 8.2, 8.5, PR table, diagram, ADR-006 |
| R2-B2 Decision 13 regex matches `next/dynamic` mocks | 2.2b regex is `jest\.mock\(['\"]next/(navigation|link)['\"]`; the 8 `jest.mock("next/dynamic"` files are named as deliberately kept until Task 9.1b; 2.3a/2.4a/2.5a exclude `*.test.*` and `__tests__` and the 5 tests that import from `next/navigation` are named as belonging to 2.2b's set (all 5 also contain a `jest.mock` of nav/link, so none is orphaned) | decision 13, Stories 2.2-2.5 |
| R2-C1 `sg` not provisioned | New Task 2.2d: pinned `@ast-grep/cli`, invoked as `ast-grep` (never `sg`), `make ensure-tools`/`install-tools` checks, `ripgrep` installed explicitly in the CI `prepare` action, committed tests use Node APIs, a test fails on any `sg` invocation. Also found: codemod liveness tests were assigned to the `build` Jest project, which does not exist until PR 3, although the codemods run in PR 2a; they now run under `node --test` from PR 2a (`test:codemods`) and are also collected by `build` from PR 3. Also found: the file-set query in decision 13 (`jest.mock("next/navigation", ...)` alone) missed 3 link-only test files; the set is the union of two queries (43) | 2.2d, decision 13, 2.2 |
| R2-C2 9.1a counts | 26 = all of `src/app` (13 layouts + 13 pages); `page.tsx` alone = 13, 11 with the `Metadata` type import; the script must also delete that import line (liveness fixture added); `WebVitalsReporter.tsx:4`, `next/dynamic` sites and `fonts.ts` listed as outside-the-script sites; the unrestricted `rg "from ['\"]next"` is the PR-level check after Task 9.1b, the 9.1a command excludes the two Next-side seams | decision 13, plan-level decision 8 |
| R2-C3 membership "variable" vs "workflow edit" | One mechanism everywhere: repository variable `WEB_BUILD_GATE_MEMBERS` set with `gh variable set`; 5.11, 7b rollback row, Story 7.10, 8.2 (Task 8.2b), 8.5 runbook step 4, 9.2 (set the variable before merging), wall-clock blockers | as listed |
| R2-C4 branch protection | Reworded as first-time setup (404 re-run today, see below); Task 5.11d adds the PR-event proof on a scratch repo (relevant PR, irrelevant PR, failing member); `workflow_dispatch` trigger commit limited to `benchmark.yml` and `ux-analysis.yml`; runbook command is `printf '["web-build-gate"]' | gh api -X DELETE ... --input -` | decision 10, 5.11, 8.5, wall-clock blockers |
| R2-C5 router entry | Decision 12 now says only `src/main.tsx` imports the entry (`routes.tsx` imports no router API); Task 4.2d adds `eslint src/main.tsx src/lib/nav/entry.tsx src/app/routes.tsx` exit 0 and an export-set test for `@/lib/nav`. The review asked for "exactly 8 names"; Story 2.1 already allows the Task 2.1d helpers (`stringParam`, `intParam`, `enumParam`) at runtime, so the test asserts the 8 names plus those three, and absence of `createAppRouter`, `AppRouterProvider`, `createBrowserRouter`, `RouterProvider`, `Outlet` | decision 12, 4.2 |
| R2-C6 stale dirty set | `Set<WeakRef<Element>>` + `WeakSet`, pruned on every evaluation (`deref()` gone or `!isConnected`); edited-then-unmounted case in Task 2.7d | 2.7 |
| R2-C7 PR 8b rollback | Ladder item (1b) lists Makefile line, CI default (Task 8.6a), `minifyExplicit` path, service restart, budgets-on-minified note; PR 8b table row; Story 8.6 gets a "revert set is complete" AC and a waiver rule (a difference needs a fix or an issue with an owner-written accept note copied into `cutover-gate-results.md`, else Task 8.6b fails); 6a-versus-PR 8 order stated: preferred order only, Task 6.3b owns the rebase | Migration Plan, diagram, 8.6, 8.5 |
| R2-C8 smaller | (a) row 9 cell restored; (b) Task 0.1d moved before 0.1e; (c) Story 8.3: `make web-build` is `pnpm run build` with `NEXT_BUILD_MODE=development` (`Makefile:226`), so "development-mode" is accurate for the target as run and the review's "production build" suggestion was **not** applied; the wording now says exactly what runs and that the production-mode Next build is measured separately in Story 0.4; (d) warm baseline: caches named (`.next/cache` and `NEXTJS_SHARED_CACHE_DIR`), warm ratio printed as-is and with both caches cleared, gate stays on the as-is warm ratio; (e) Story 2.7 `next.config.ts` now has an AC (webpack alias for `@/lib/lazy/impl`) and `tsconfig.json` joins Files | 0.1, 0.4, 8.3, 2.7 |

**Verification commands run on today's tree (read-only, worktree `sdd-web-app-bun-build`, 2026-10-19):**

| Command | Result | Matches the AC? |
|---|---|---|
| `rg -l "jest\.mock\(['\"]next/" web-app/src` (old 2.2b regex) | 51 files | No: cannot print nothing (43 + 8 `next/dynamic`); replaced |
| `rg -l "jest\.mock\(['\"]next/(navigation\|link)['\"]" web-app/src` (new 2.2b) | 43 files before the codemod | Yes (empty after) |
| `rg -l "jest\.mock\(['\"]next/dynamic['\"]" web-app/src` | 8 files | Kept on purpose |
| `rg -l "from ['\"]next/(navigation\|link)" web-app/src --glob '*.test.*' --glob '**/__tests__/**'` | 5 files, each also has a nav/link `jest.mock` | Yes, all in 2.2b's set |
| `ast-grep run -p 'jest.mock("next/navigation", $$$)'` per `-l ts`/`-l tsx`, union with the `next/link` query | 43 | Yes; the `next/navigation` query alone gives 40 (the other 3 files mock only `next/link`), so the single-query file set was wrong |
| 2.3a: `rg -l "from ['\"]next/(navigation\|link)" web-app/src/app --glob '!**/*.test.*' --glob '!**/__tests__/**'` | 27 files before | Yes (empty after) |
| 2.4a: same over `web-app/src/components` | 26 files before | Yes |
| Without the test globs: `src/app` 28 (+1 test, `backlog-stages/page.test.tsx`), `src/components` 28 (+2 tests, `Header.test.tsx`, `BottomNav.test.tsx`) | the unrestricted command is non-empty by construction, as the review found | Fixed by the globs |
| 2.5: Story 2.5 command with `**/`-anchored globs | 65 before (27 + 26 + 9 for `navigation\|link`, plus `next/dynamic` imports) | Yes (empty after 2.3-2.5 and 2.7) |
| Glob anchoring: `--glob '!src/lib/hooks/**'` vs `'!**/src/lib/hooks/**'` from the repo root | 62 vs 59 files | The unanchored form excludes nothing; Story 2.5's original globs were no-ops; fixed |
| 9.1a: `rg -l "export const (metadata\|viewport)\|generateMetadata" web-app/src/app` | 26 files (13 `page.tsx`, 13 `layout.tsx`) | Yes (empty after the manifest and script) |
| `find web-app/src/app -name layout.tsx` / `-name page.tsx` | 16 / 42 | Yes |
| `rg -l "from ['\"]next" web-app/src --glob '!**/lib/nav/impl.next.tsx' --glob '!**/lib/lazy/impl.next.tsx'` | 96 files today | Empty only after PRs 2a-2d and 9.1a; simulated post-state (minus manifest layouts, `fonts.ts`, tests, nav/link/dynamic, `Metadata` type imports) leaves exactly one line: `WebVitalsReporter.tsx:4` (Story 2.8), now listed |
| `rg -l "import type \{[^}]*\} from ['\"]next['\"]"` | 24 files (13 layouts, 11 pages) | The script must delete the 11 page imports |
| `gh api repos/tstapler/stapler-squad/branches/main/protection/required_status_checks` | 404 "Branch not protected" | Confirms first-time setup wording |
| `grep -n workflow_dispatch .github/workflows/*.yml` | present in `build.yml:20` and `lint.yml:24`, absent from `benchmark.yml` and `ux-analysis.yml` | Confirms the two-file trigger commit |
| `which sg ast-grep rg` | `~/.cargo/bin/sg`, `~/.cargo/bin/ast-grep`, `rg` present locally | CI image contents UNVERIFIED; Task 2.2d provisions explicitly |

Not runnable on today's tree: the codemod outputs themselves (empty-after), the `git diff --stat --diff-filter=D` manifest check and the idempotence and liveness conditions (they need the scripts, which are the tasks' deliverables); the gate script and `actionlint`; the PR-event proof (needs a scratch repository). These stay UNVERIFIED until Tasks 2.2b, 5.11b-d and 9.1a run.

**Cost-class check (recounted):** 3 added tasks, all S: 2.2d, 5.11d, 7.10f = 3 x 55k = 165k raw, matching the Effort rows (Story 2.2 240k to 295k, 5.11 165k to 220k, 7.10 295k to 350k; Phase 2 +55k, Phase 5 +55k, Phase 7 +55k). CU x1.1 = 181.5k; x1.75 = 317.6k.

**Not fixed, with reasons:** (1) the Critical-path wave count is unchanged: Task 2.2d runs in the first wave of Epic 2 beside 2c and 2d, 5.11d follows 5.11b in the Epic 5 final wave, 7.10f is inside the two Epic 7b waves. (2) `consistency.md` counts and the checkboxes in `adversarial-review.md` are outside the editable set. (3) The review's suggestion to call the Next baseline a "production build" was not applied because `Makefile:226` shows `make web-build` runs with `NEXT_BUILD_MODE=development` (see R2-C8c). (4) CI runner contents (`ripgrep`, `ast-grep`) and the GitHub semantics of skipped needed jobs under `always()` stay as the review labelled them (DOCS/UNVERIFIED) until Tasks 2.2d and 5.11d run.
