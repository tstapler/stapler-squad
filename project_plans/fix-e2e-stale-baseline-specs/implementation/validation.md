# Validation: fix-e2e-stale-baseline-specs

## Blocker Check

| # | Blocker question | Assessment |
|---|---|---|
| B1 | Does deleting `terminal-flickering.spec.ts` break any import or reference? | No other spec imports from it; `grep -r 'terminal-flickering' tests/e2e/` returns only the file itself. Safe to delete. |
| B2 | Is `createIdleSession` / `waitForSessionsActive` available in all spec files that need it? | `SessionClient` is exported from `tests/e2e/helpers/session-client.ts`; already used in `input-drop-badge.spec.ts` (createIdleSession). `waitForSessionsActive` is a public method. No new dependency required. |
| B3 | Does `process.env.CI` evaluate correctly in Playwright tests? | Yes — Playwright runs in Node; `process.env.CI` is `'true'` in GitHub Actions. Confirmed by existing specs that gate CI-only or non-CI behaviour with this variable. |
| B4 | Does the `UpdateFeatureFlag` RPC exist in test-mode? | `terminal-resync-feature-flags.spec.ts` already posts to it in `resetFlag`, which implies it is wired. The 2 failing rows are more likely a timing issue. |
| B5 | Does `BottomNav` render when no sessions exist? | Uncertain; android-url-bar task mitigates by seeding a session. |
| B6 | Will the baseline re-record step produce a clean file? | Requires a successful CI run first (or verified local run). Task 8 is blocked on Tasks 1–7 completing cleanly. |

## AC Verification Matrix

| AC | How to verify |
|---|---|
| AC-1 terminal-flickering | `git rm` committed; `grep -r '/test-terminal' tests/e2e/` returns nothing; no new `/test-terminal` route in `web-app/src/app/routes.tsx`. |
| AC-2 terminal-stress perf floors | `bun run test -- --reporter=list terminal-stress` on a CI runner shows all tests `skipped`; `process.env.CI` confirmed set. |
| AC-3 terminal-mobile-overflow | Local run: `npx playwright test terminal-mobile-overflow.spec.ts` passes all 8 tests with an isolated test server in test-mode. |
| AC-4 input-drop-badge | Local run: `npx playwright test input-drop-badge.spec.ts` passes all 10 tests (no `waitForFunction` timeout). |
| AC-5 android-url-bar | Local run on Android viewport: both tests pass; BottomNav is visible. |
| AC-6 terminal-resize | `npx playwright test terminal-resize.spec.ts` — test 5 passes (or skips if not in `chromium-dom` project); 0 fail rows. |
| AC-7 terminal-resync-feature-flags | Both tests pass; flag row visible before assertions. |
| AC-8 no new failures | `make ci` exits 0; baseline diff shows only the 47 target rows changing from `fail` to `pass`/`skip`. |
| AC-9 baseline update | `tests/e2e/baseline/next-e2e-baseline.json` committed; `git diff --stat HEAD~1 tests/e2e/baseline/` shows the file changed. |

## Risk Register

| Risk | Likelihood | Mitigation |
|---|---|---|
| BottomNav still not rendered after session seed | Low | Inspect with screenshot if still failing; fall back to seeding session in `beforeEach` and navigating directly to `/?session=<id>`. |
| terminal-resync-feature-flags still fails after waitForFlagRow | Medium | Check test-mode server logs for `UpdateFeatureFlag` RPC errors; if endpoint returns 404 in test mode, guard with explicit capability check or skip. |
| Baseline re-record diverges between runs | Low | Use same runner tier for capture and verification; commit the capture output as authoritative. |
| terminal-stress CI skip causes perf regressions to go undetected | Medium (long-term) | Accepted trade-off; follow-on item creates benchmark workflow. |
