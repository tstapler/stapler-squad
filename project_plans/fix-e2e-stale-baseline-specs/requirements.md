# Requirements: fix-e2e-stale-baseline-specs

**Item ID:** e3977c3a-3ec6-4c6c-b5b4-e6f726b0f3d4  
**Title:** e2e: 47 baseline-fail rows target removed test pages or perf thresholds

## Background

After PR #1166, the recorded Next baseline (`tests/e2e/baseline/next-e2e-baseline.json`, 529
specs of class `fail`) contains 47 rows across 8 spec files that fail for environmental reasons —
removed test pages, missing runtime triggers, or hard-coded performance floors that cannot survive
shared-runner CPU budgets — not regressions in production behaviour.  Fixing them is prerequisite
to making the baseline a reliable signal.

This item is distinct from `e2e-baseline-fail-fixes` (item b21fb6da), which addressed
alias-settings label mismatches, jules-dispatch keychain errors, local-file-browser URL
assertions, dev-mode-hmr project exclusion, and route-focus skip-link assertions.  This item
covers the second wave of structural failures identified in `baseline-fail-triage.json`.

## Root Causes (per spec file)

| Spec | Rows | Root cause |
|---|---|---|
| `terminal-flickering.spec.ts` | 14 | Navigates to `/test-terminal` which does not exist. The terminal stress test page lives at `/test/terminal-stress`. |
| `terminal-stress/ascii-video.spec.ts` | 6 | Navigates to `/test/terminal-stress` correctly but asserts hard frame-count floors (`> 100 frames`, `> 500 frames`) that consistently fail under shared-runner CPU. |
| `terminal-stress/large-payload.spec.ts` | 4 | Same pattern: `framesRendered > 0` guards pass but throughput assertions fail on shared runner. |
| `terminal-mobile-overflow.spec.ts` | 8 | Waits for `toolbar-more-button` via `openTerminalView`, which requires a live session with TerminalOutput mounted; test server has no pre-seeded sessions in test-mode. |
| `input-drop-badge.spec.ts` | 10 | `waitForFunction` for `window.__e2eTriggerInputDropped` times out after 10 s. Trigger is registered only after TerminalOutput mounts; spec does not reliably land on a view with TerminalOutput active before the timeout. |
| `android-url-bar.spec.ts` | 2 | Tests viewport/layout behaviour; root cause in baseline-fail-triage.json is listed as same category (removed pages / missing triggers / perf floors) — most likely BottomNav navigation label mismatch after a nav redesign. |
| `terminal-resize.spec.ts` | 1 | `test.skip(!opened, ...)` guard should handle missing sessions but 1 row still appears; likely the skip guard is not evaluated correctly under the Next baseline runner. |
| `terminal-resync-feature-flags.spec.ts` | 2 | Feature-flag API responds but the toggle label/state assertion mismatches the actual rendered text, or the `resetFlag` beforeEach is flaky under test-mode. |

## Acceptance Criteria

1. **AC-1 terminal-flickering**: All 14 `terminal-flickering.spec.ts` baseline-fail rows are
   eliminated.  Either (a) the spec is migrated to use `/test/terminal-stress` and the
   shared stress-test harness, so its correctness assertions pass, or (b) it is deleted with a
   commit message explaining what replaced it.  No `/test-terminal` route is added back to the
   app.

2. **AC-2 terminal-stress perf floors**: The 10 `terminal-stress/ascii-video.spec.ts` and
   `terminal-stress/large-payload.spec.ts` rows no longer appear in the correctness baseline.
   Hard frame-count / fps floor assertions (`framesRendered > 100`, `framesRendered > 500`,
   `assertFrameRate(...)`) are either (a) removed from the correctness run and carried in a
   separate benchmark workflow, or (b) made robust to low-CPU runners with explicit
   `test.skip` guards keyed on `process.env.CI` with a documented justification.

3. **AC-3 terminal-mobile-overflow**: All 8 `terminal-mobile-overflow.spec.ts` rows pass.
   The spec must not depend on pre-existing live sessions; it must either seed a session via
   `SessionClient` before asserting toolbar elements, or the assertions must be guarded with
   `test.skip(!opened, ...)` just as `terminal-resize.spec.ts` does.

4. **AC-4 input-drop-badge**: All 10 `input-drop-badge.spec.ts` rows pass.  The
   `waitForTestTrigger` / `waitForFunction` must succeed within the 10 s timeout.  Root fix is
   ensuring the terminal view is fully mounted with an active session before waiting for the
   trigger; `SessionClient` must create a session and navigate to it before `waitForTestTrigger`.

5. **AC-5 android-url-bar**: Both 2 `android-url-bar.spec.ts` rows pass.  BottomNav ARIA
   label assertion is updated to match the current nav component's rendered label text, or the
   component is updated to emit the label the test expects, whichever is correct.

6. **AC-6 terminal-resize**: The 1 `terminal-resize.spec.ts` baseline-fail row is eliminated.
   The `test.skip(!opened, ...)` guard works as intended and the row transitions to `skip` or
   `pass` in the Next baseline.

7. **AC-7 terminal-resync-feature-flags**: Both 2 `terminal-resync-feature-flags.spec.ts`
   rows pass.  The toggle-label assertion matches the actual rendered label, and the `resetFlag`
   beforeEach does not leave state that causes a subsequent test to fail.

8. **AC-8 no new failures**: After this fix, `make ci` passes, `bun run lint:duplicates` in
   `web-app/` passes, and no previously-passing baseline row transitions to `fail`.

9. **AC-9 baseline update**: `tests/e2e/baseline/next-e2e-baseline.json` is updated via the
   baseline capture script to reflect the corrected spec outcomes.

## Out of Scope

- Performance benchmark infrastructure: creating a dedicated GitHub Actions workflow for
  terminal throughput benchmarks is desirable but tracked separately.
- Other `fail`-class specs in the Next baseline unrelated to the eight spec files above.
- Visual-regression snapshot regeneration (tracked in a separate item).
- The 146-row "stale-baseline-passes-on-next-without-changes" category (addressed by baseline
  re-record, tracked in issue #1186).

## Addendum 2026-11-26: re-record the baseline (criterion 9, index 8)

Review failed criterion 9 because `next-e2e-baseline.json` was edited by hand. The owner's
direction: re-record it from verified test runs, or fail the criterion with the reason.

- Rows for the eight listed spec files come from Playwright JSON reports of full CI runs on this
  branch, classified by the original rules
  (`project_plans/web-app-bun-build/spikes/s0/classify-e2e.mjs`). No row is typed or edited by hand.
- Three runs, as for the original capture.
- Rows of every other spec file stay byte-identical to `origin/main`.
- Rows of the deleted `terminal-flickering.spec.ts` are absent because no report contains them.
- A row that was `pass` on `main` and records `fail` is a regression to fix, not to commit.
- Not in scope: re-recording the whole file. It would absorb unrelated failures on `main`.
