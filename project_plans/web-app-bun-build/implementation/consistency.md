# Cross-artifact consistency: web-app-bun-build (Phase 4)

Date: 2026-10-19. Inputs read in full: requirements.md, implementation/plan.md (1,690 lines at the time; 1,977 after the triad folding), design/ux.md, ADR-001..008 (with Amendments), docs/explanation/bun-migration-feasibility.md.
Result: 2 BLOCKER, 13 CONCERN, 16 NITPICK. No requirement in `Scope -> In Scope` lacks a story. Every UX-01..UX-57 has an owning story that exists.

## Counts recomputed (VERIFIED by command)
*(Refreshed after the triad gap folding; counts only. The findings below are as of the Phase 4 pass and were resolved in the plan's Repair log, "Iteration 3 repair".)*
- plan.md: 81 `#### Story` and 216 `##### Task` (`grep -c '^#### Story'`, `grep -c '^##### Task'`), 10 phases (0-9); per-story task counts equal the Effort table column and each row's raw tokens equal the sum of its classes (checked by script: 0 mismatches over 81 rows); phase rows equal heading counts (Phase 0: 7/28, 1: 7/14, 2: 9/28, 3: 6/14, 4: 7/19, 5: 11/37, 6: 10/21, 7: 14/27, 8: 6/17, 9: 4/11).
- Raw 13,550k; x1.1 = 14,723.5k CU ("14.72M"); x1.75 = 25.77M; range x1.5-2 = 22.1-29.4M. Contingencies 1,056k / 1,848k; base+all = 15.78M / 27.61M.
- 18 PRs listed in the header and Migration Plan table (0,1,2c,2d,2a,2b,2e,3,4,5a,5b,6a,6b,7a,7b,8,8b,9); optional Story 9.4 is a further PR not in the 18.
- Repo: 42 `page.tsx`, 16 `layout.tsx`, 43 test files mocking `next/(navigation|link)`, 65 production importers (regex), 337 `.css.ts`, 152 e2e specs, 29/42 routes in the a11y sweep (13 missing = UX).
- ux.md: UX-01..UX-62 (62), S1..S16 (16). validation.md rows: 90 Unit, 70 Integration, 1 Migration, 62 UX (223 total).

## BLOCKER
B1. NODE_ENV rule contradicts between ADR-004 and Story 5.5.
- ADR-004 Decision 1 (line 12): `build.ts` always sets NODE_ENV explicitly; the dev/prod switch toggles minify and sourcemaps only, never NODE_ENV (today's `NEXT_BUILD_MODE=development` build is NODE_ENV=production, so `Providers.tsx` picks `HttpAnalyticsProvider`).
- plan.md:1059 (Story 5.5 Defines): NODE_ENV is "production" "and 'development' only with `--mode=development`"; plan.md:1067: `WEB_BUILD_MODE=development` replaces `NEXT_BUILD_MODE`. Literal implementation makes `make web-build` (which uses the development mode today) ship NODE_ENV=development.
- Fix: edit Story 5.5 so NODE_ENV is "production" for every `build.ts` invocation (including `WEB_MINIFY=0` and any development profile); the profile only toggles minify/sourcemaps; only `dev.ts` sets "development". Change the test at 5.5 accordingly.

B2. The lint ban forbids imports the router entry must make.
- ADR-001 Decision 1 and plan.md:750 (Story 2.9): `react-router` is banned (error from PR 2e) everywhere except `src/lib/nav/**`, `src/lib/lazy/**` and tests; plan.md:636 fixes the facade to exactly 8 names.
- plan.md:920 (Story 4.4) and 4.3: `src/main.tsx` uses `createBrowserRouter`, `Outlet`, `RouterProvider` (and `RouteError`, `routes.tsx` need RR types). PR 4 lands after 2e, so its own lint fails.
- Fix: either add `src/main.tsx` (and the named router-entry files) to the 2.9 exclusions and ADR-001 item 1, or have `impl.router.tsx` export a `createAppRouter(routeTable, shell)`/`AppRouterProvider` so the entry stays facade-only (then add those names to the 2.1 contract list).

## CONCERN
C1. Stale size/lineage text. requirements.md:33 says ~22.2M CU, range 19.0-25.4M; plan has 22.7M, 19.4-25.9M. plan.md:302 says "requirements.md still says Large" and plan.md:1647 says it has "41 routes"; requirements now says XL and 42. Fix: update requirements.md:33; delete the two stale plan clauses.
C2. Added scope against Out of Scope. requirements.md "Out of Scope: UI redesign or behaviour changes beyond route/title parity" and metric "Zero functional regression" versus D1 not-found view, D5 focus move, announcer politeness change (assertive to polite), `RoutePending`, the "new version available" banner (Story 2.7), Go 404 under `/_next/` (Story 1.6), flat-config conversion (6.2), Lighthouse consolidation (6.5), Axe sweep gaps (7.8). The plan flags D1/D5 as owner decisions but requirements.md never records them. Fix: add an "Accepted scope additions" list to requirements.md, signed off in Story 0.6. (Not a BLOCKER only because the plan marks each as a pending owner decision with a bound.)
C3. `WEB_BUILD_MODE` (plan.md:1067; research/stack.md, pitfalls.md) collides with the glossary term `WebBuildMode` = value of `WEB_BUILD` (plan.md:33). Fix: rename the profile switch (for example `WEB_BUILD_PROFILE`) or drop it in favour of `WEB_MINIFY`.
C4. ADR-004 item 4 says "secondary decision S3"; the plan's secondary decisions are D1-D7 and S3 is spike gate S3. Fix: "D3".
C5. Focus target. ux.md:77 requires the focus hook to target the layout `main` "by ref, not by a fresh query"; plan.md:945 and the Tech Debt row use `document.querySelector("main#main-content")`. Fix: pick the ref (RootShell passes it) in 4.5.
C6. Error control and stale UX text. UX S11/UX-39 say the ErrorBoundary has a "Reload button"; Story 4.4 says "Retry that re-requests the chunk"; existing `ErrorBoundary.tsx` exposes `onRetry`. ux.md:195 still says the lost-draft case is an "Accepted risk (G6)" though Story 2.7 now shows a banner instead. Fix: align the label and drop the stale row.
C7. Ports. plan.md:170 and :455 (Story 0.3) use ManualInstance backend `http://localhost:62872`; per CLAUDE.md 62871 is `PORT` (HTTP) and 62872 is the TLS `--remote-port`, and Stories 0.2b/8.1 and the glossary agree. The dev server on 62873 (plan.md:1084, ux.md:229, UX-51) is the CLAUDE.md "instance #2 PORT". Fix: backend 62871; assign dev-server a spare port (62875+) or add a dev-server row to the port block in Story 6.7.
C8. `WebBuildMode` parser has two owners and a PR-order problem. Story 5.8 (PR 5b, plan.md:1129) says `launch.ts`, `test-server.ts` and `tests/e2e/helpers/web-build.ts` share `parseWebBuildMode(env)`; Story 7.10a (PR 7b) creates the "single WebBuildMode parser" and plan decision 3 calls it `webBuildMode()`. 5b and 7b are parallel after 5a. Fix: one creator (5.8) and one name; 7.10a consumes it.
C9. `dynamic` site counts. Gate S3 (plan.md:171) says "4 `dynamic` sites"; Story 2.7 says 7 sites in 4 files; UX-42 lists 6 (omits `test/terminal-stress`). Fix: "all `dynamic` sites (7 in 4 files)" in S3 and UX-42.
C10. ADR home. Story 6.4 Files (plan.md:1228) says "`docs/adr` ADR-005 amendment", but ADRs live in `project_plans/web-app-bun-build/decisions/`; `docs/adr/` already has `ADR-001-gemini-exit-code-contract.md` and `ADR-002-workspace-host-registry-gossip.md`, so promotion would collide. Story 9.3 only "sets ADR statuses". Fix: state the location and a renumbering rule.
C11. `patchedDependencies` location. Unresolved Questions say Bun reads it from the root manifest only; Story 3.2 Files say "root or `web-app/package.json`". Fix: root only (or the PR #1100 layout).
C12. The committed feasibility doc contradicts the chosen design. Its header Recommendation says "Do not move `web-app/` to Bun's bundler... Rsbuild is the best fit"; section 9 and requirements/ADR-007 record Bun.build as chosen. Story 0.7 commits it "unchanged". It also says 41 pages and 336 `.css.ts`. Fix: allow a one-line status note at the top of the doc pointing to section 9 and ADR-007 in Story 0.7.
C13. Success metrics not gated. requirements: build <= 10 s and `make web-build` >= 5x faster. Story 5.5 gates only `build.ts` warm time; the end-to-end 5x (10.8 s at the best baseline) appears only in Story 8.3 as "any miss stated", with no remedy path. Fix: say whether a miss blocks PR 8 or goes to the owner.

## NITPICK
N1. requirements.md:8 "40 test files" (43 at :122, plan, ADR-001) and "63 production files" (plan 65 by regex; explained in plan decision 8).
N2. UX criteria with thin plan ACs: UX-05 (search-only title) has no AC in 4.4; UX-07 (hash change) not in 4.5; UX-12 (focus on popstate) only announcement asserted in 7.2b; UX-17 (no active nav item) not in 7.7b; UX-50 (keyboard-only reachability) has no AC in 7.8, which is Axe-only.
N3. Not-found copy: ux.md "Go to Sessions", hidden Back when `history.length` is 1; Story 4.3c "Go to sessions", Back always shown.
N4. UX-49 "six specs" exclude `#__next-route-announcer__`; plan says 5; grep finds 5 specs plus 1 page-object file.
N5. "Story 0.4d" and "Story 0.2d" are Tasks.
N6. D3 row cites Stories 5.4/8.3; sourcemaps are set in 5.5 and excluded in 5.9; 8.3 reports binary size but `BuildStats` (5.9) has no such field.
N7. "17 PRs" excludes the optional Story 9.4 PR.
N8. Epic 2 heading "106-file migration" versus 65 + 43 = 108.
N9. ADR-008 `scripts/verify-eval-patch` versus plan `web-app/scripts/verify-eval-patch.mjs`.
N10. `.gitignore` additions are owned by both 5.7 and 6.9 (and 6.5d for the scaffolding guard); size-limit survivor (`.size-limit.json` or `package.json` key) is unspecified in 6.4 but named `.size-limit.json` in 9.4 and ADR-003.
N11. ADR-008 and Story 3.1: `@vanilla-extract/css` ">=1.21.3 exact" is self-contradictory.
N12. Glossary says `RouteMeta` has "a `NOT_FOUND_META` title entry"; Story 1.2 exports it separately (needed for the 4.3 key-set equality).
N13. Plan decision 2 says PR 4/5 were swapped "relative to the brief", but requirements.md and feasibility section 9 already order router before build.
N14. Story 8.3 "Jest unchanged (7,683 tests)" while the plan adds tests; say "no new failures".
N15. Impl naming asymmetry: nav uses `impl.router.tsx`, lazy uses `impl.bun.tsx`; the glossary `NavImpl` omits lazy impls; `LazyBoundary` vs `lazyComponent`.
N16. ADR-006 "through PR 7" is correct but easy to misread against "default `next` until PR 8".
