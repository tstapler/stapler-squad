# Implementation Plan: fix-e2e-stale-baseline-specs (revision 2)

**Goal:** make the baseline-`fail` rows of eight specs pass or skip for the reasons observed in
[research/findings.md](../research/findings.md). Revision 1 was written without running the specs;
its fixes for AC-2, 4, 5 and 7 would not have worked.

**Rules:** test titles stay byte-identical (baseline rows are keyed by title). No `waitForTimeout`
added. Product code changes only where the test cannot be fixed without one.

| # | Change | Files | AC |
|---|---|---|---|
| 1 | Delete the spec; drop its `shard-timings.json` entry and stale comment in `e2e-video.yml` | `terminal-flickering.spec.ts` | 1 |
| 2 | `test.skip(!!process.env.CI, PERF_FLOOR_REASON)` on the three ascii tests that assert frame floors (30 FPS, 60 FPS, bouncing ball). Comment names the follow-on benchmark workflow | `terminal-stress/ascii-video.spec.ts` | 2 |
| 3 | Page: publish final progress counters when a run completes. Spec: 100KB test keeps correctness assertions everywhere and runs the frame-time ceilings only off CI, with `avgFrameTime` bound derived from the 1 fps preset; reset test stops mid-run, restarts, and requires the second run to last its full duration | `TerminalStressTest.tsx`, `large-payload.spec.ts` | 2 |
| 4 | `beforeAll` seeds a session and waits for it active by title; helper opens `/?session=<id>` and expands the toolbar, failing loudly. Update assertions to the current toolbar: keyboard toggle is primary, Paste is in the overflow row. `afterAll` deletes the session | `terminal-mobile-overflow.spec.ts` | 3 |
| 5 | Opt-in test hook: install `__e2eTriggerInputDropped` outside production **or** when `window.__E2E_HOOKS__ === true`. Spec sets it with `addInitScript` and also calls `waitForSessionsActive([title])` | `TerminalOutput.tsx`, `input-drop-badge.spec.ts` | 4 |
| 6 | Exclude closed drawers (translated down by at least their own height) from the fixed-overflow assertion. No session seeding: BottomNav already renders | `android-url-bar.spec.ts` | 5 |
| 7 | Test 5: `waitForSessionsActive([title])` before navigation; click "Redraw terminal"; same name for tests 1-4, which share the stale locator | `terminal-resize.spec.ts` | 6 |
| 8 | `waitForFlagRow` returns the rendered row; locate `role=switch`, assert `aria-checked`; failure test asserts `feature-flag-row-error` and an unchanged switch | `terminal-resync-feature-flags.spec.ts` | 7 |
| 9 | Delete flickering rows; flip rows verified by a local run to `pass`, CI-skipped ones to `skipped`. Update `meta.counts`/`specCount`; mark the triage category resolved | `next-e2e-baseline.json`, `baseline-fail-triage.json` | 8, 9 |

## Revisions from adversarial review

- **Blocker, change 3:** `stopTest` nulls `abortControllerRef`, which `runLoop` reads, so Stop never
  stops the loop. Capture `const { signal } = controller` per run; on abort the loop returns without
  touching status or metrics (Stop already set them). Guard: stop run A after the existing 2 s wait,
  start run B, and require `completed` no sooner than 4.5 s later (a surviving run A would complete it
  early). A zero counter after Reset would not catch this: 1 fps publishes only at completion.
- 100KB test keeps a loose `maxFrameTime < 5000` on CI (freeze detection); tighter bounds off CI.
- Resize test 5 asserts non-blank rows before and after, polling so it cannot pass on an empty terminal.
- Flip only rows whose fix this change makes. The four extra stress rows that already pass locally stay
  `fail`. The comparator runs on main only, so check the PR shard report for the changed specs before merge.
- Android: exclude only the two named drawers, and only while parked off-screen.
- Mobile: assert Paste is hidden before "More"; unique session title.
- Resync failure test asserts the Retry button. Update `build-coupled-specs.json` if it names these tests.
- Update the hook's comment in `TerminalOutput.tsx`. Recompute `meta.counts` from the rows.

## Departures from the acceptance criteria as written

| AC | Written | Done instead, and why |
|---|---|---|
| 1 | delete spec | Also say in the commit message that nothing replaces it |
| 2 | CI guard on perf floors | Also fixes two non-perf bugs behind the large-payload rows |
| 4 | `waitForSessionsActive` removes the timeout | Added, but the fix is the runtime hook gate |
| 5 | `beforeAll` seeds a session | Not added: BottomNav already renders; the failure is the drawer assertion |
| 7 | 2 rows | 6 rows fail from the same cause; all fixed |
| 9 | re-record | Rows edited from verified runs: Next no longer exists |

## Design notes

- **Why a runtime flag for the hook (5):** the e2e binary is the production web build, so a
  build-time gate can never be true there. The hook only calls the badge's own handler; a script able
  to set the flag can already manipulate the DOM.
- **Why `skipped` is per test, not per file (2, 3):** a file-level skip would drop passing
  correctness tests (flow control, 1MB payload, start/stop cycles).
- **Baseline edits (9):** the Next build no longer exists, so rows are edited as in #1459.

## Verification

1. Eight specs, both Chromium projects, `--retries=0`, with and without `CI=1`.
2. `bun test` for baseline-skip/compare unit tests; Jest for `TerminalOutput` and the stress page area.
3. `make ci`; `bun run lint:duplicates` in `web-app/`.
