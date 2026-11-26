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
| 5 | Server-gated test hook: `/api/server-info` sends `e2e_hooks` only on the `e2e-local` instance, and `useE2eInputDropTrigger` installs `__e2eTriggerInputDropped` only then (replaces the earlier page-set window flag). Spec also calls `waitForSessionsActive([title])` | `server.go`, `useE2eInputDropTrigger.ts`, `input-drop-badge.spec.ts` | 4 |
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

- **Why a server-reported flag for the hook (5):** the e2e binary is the production web build, so a
  build-time gate would need a separate web build in every e2e job, and `make` does not rebuild when
  an environment variable changes. The server already gates its debug endpoints on `e2e-local`.
- **Why `skipped` is per test, not per file (2, 3):** a file-level skip would drop passing
  correctness tests (flow control, 1MB payload, start/stop cycles).
- **Baseline edits (9):** the Next build no longer exists, so rows are edited as in #1459.

## Verification

1. Eight specs, both Chromium projects, `--retries=0`, with and without `CI=1`.
2. `bun test` for baseline-skip/compare unit tests; Jest for `TerminalOutput` and the stress page area.
3. `make ci`; `bun run lint:duplicates` in `web-app/`.

## Addendum 2026-11-26: re-record the baseline rows (criterion index 8)

Complexity 1: no new types, no product code, no migration.

1. Write `project_plans/fix-e2e-stale-baseline-specs/rerecord-baseline.mjs`. It imports
   `buildBaseline` from `../web-app-bun-build/spikes/s0/classify-e2e.mjs`. Inputs: the baseline from
   `origin/main` (not this branch's hand-edited copy), three `report.json` files, their run URLs and
   the captured commit.
   - Replaces every row of the eight spec files with rows built from the reports. A (spec, project,
     title) in no report gets no row (deleted or renamed tests).
   - Carries `routesVisited` over from the `main` row with the same key (null for new keys).
   - Leaves every other row as it is; recomputes `meta.specCount` and `meta.counts`; records
     `meta.rerecorded = {commit, specFiles, runDates}`. The original `meta` fields stay.
   - Refuses to write when: fewer than three reports; a report has no rows for one of the seven
     surviving files; a report was cut off (a row of those files with zero attempts).
2. Push the branch, then `gh workflow run build.yml --ref <branch>` three times, one after another
   (same concurrency group). Download each `e2e-bun-report`. All three must run the same commit.
3. Run the script. Check:
   - no row that is `pass` on `main` is `fail` in the result (a `flaky` one is listed in the PR);
   - the comparator, per report, has no `fail` finding in the eight spec files;
   - `bun test tests/e2e/scripts tests/e2e/baseline` passes;
   - rows outside the eight files are identical to `origin/main`.
   A listed row that records `fail` in the three runs is investigated and fixed (then recaptured),
   not edited.
4. If CI cannot produce three usable reports, restore `origin/main`'s baseline rows for these files
   only on the owner's say; mark index 8 `fail` with the reason.

Risks: a CI run takes about 30 minutes; shards that exhaust their budget leave tests unreached
(caught by the refusal above); `main` may move the baseline file before merge (re-run the script on
the new `main` copy with the same reports).

### Adversarial review of the addendum: CONCERNS, all six findings adopted

1. The script asserts classes for each row that is `fail` on `main` (55 rows in the eight files; the
   item's 47 excludes 4 resync and 4 stress rows): flickering absent; input-drop 10, mobile 8,
   resync 6, android 2, resize 1 and large-payload `should handle 100KB output` /
   `should show frame generation timing` `pass`; ascii frame floors 6 `skipped`; the 4
   `visible flickering` / `corrupt terminal state` rows are recorded as observed.
2. The comparator gets all three reports in one invocation.
3. Reports with non-empty `errors` are refused; each run's `headSha` and `event` are checked.
4. `tools/bzl/e2erun/main.go` tolerance of `skipped` rows is checked before commit.
5. A new key inherits its file's `routesVisited`.
6. A dispatch ending red from the comparator is acceptable if it uploaded a report. The hand-edited
   baseline is replaced by `origin/main`'s copy before the first run.
