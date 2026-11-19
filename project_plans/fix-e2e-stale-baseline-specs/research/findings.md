# Research Findings: fix-e2e-stale-baseline-specs

## Q1: terminal-flickering.spec.ts (14 rows)

7 tests in one `describe` block; all share a `beforeEach` that navigates to `/test-terminal` and
waits for `[data-testid="terminal-container"]` (10 s timeout → 404 → timeout = baseline fail).

Unique assertions against non-existent testids: `changed-lines`, `unchanged-lines`,
`total-updates`, window globals `window.testTerminal`, `window.testStateApplicator`,
`window.testMetrics`.  These do not map to the `/test/terminal-stress` page (which uses
`preset-selector`, `progress-elapsed`, `p95-frame-time`, `bytes-generated`).

**Recommendation:** Delete the spec.  The incremental-rendering invariants it verified (only
changed lines updated, 60fps frame budget, StateApplicator not causing full re-renders) are
appropriate for Jest unit tests, not for a test page that was intentionally removed.  Confirm
no other spec navigates to `/test-terminal`.

## Q2: Hard perf floors in terminal-stress specs (10 rows)

`ascii-video.spec.ts` hard floors:
- `framesRendered > 100` at 5 s interim (30fps test)
- `framesRendered > 250` at final (30fps, 10 s)
- `assertFrameRate(30, 0.5)` → avgFrameTime < 50ms, p95 < 75ms
- `framesRendered > 200` at 5 s interim (60fps test)
- `framesRendered > 500` at final (60fps, 10 s)
- `assertFrameRate(60, 0.5)` → avgFrameTime < 25ms, p95 < 37.5ms (hardest: fails on slow runners)
- `p95FrameTime / avgFrameTime < 3` (jitter ratio)

`large-payload.spec.ts` floors:
- `avgFrameTime < 500ms` (100KB payload)
- `maxFrameTime < 2000ms` (100KB)
- `maxFrameTime < 5000ms` (1MB payload)
- `memoryGrowthRate < 5MB/s`

The 60fps assertions are the most fragile. `> 500` frames-in-10s = 50fps minimum — plausible on
a loaded CI runner.

**Recommendation:** Add `test.skip(!!process.env.CI, 'perf-floor: run in benchmark workflow only')`
at the describe-block level for each of the two files.  This removes them from the correctness
gate immediately.  Longer-term, move to a dedicated benchmark GHA workflow.

## Q3: input-drop-badge.spec.ts flow (10 rows)

All 5 tests follow: `SessionClient.createIdleSession` → `openTerminalView(page, session.id)` →
`waitForTestTrigger` (polls `window.__e2eTriggerInputDropped` for 10 s).

**Missing step:** `waitForSessionsActive` is not called between `createIdleSession` and
`openTerminalView`.  When the page loads and the session is not yet `ACTIVE`, TerminalOutput
either renders in a loading/disconnected state or never mounts, so the trigger is never
registered and `waitForFunction` times out at 10 s.

**Fix:** Call `await client.waitForSessionsActive([session.id])` (or equivalent polling) before
`openTerminalView`.  Alternatively, increase the `waitForFunction` timeout to 20 s and add a
`waitUntil: 'networkidle'` to the `page.goto`, but the former is more reliable.

## Q4: android-url-bar.spec.ts (2 rows)

ARIA label `'Bottom navigation'` matches the component.  Root cause is likely that `BottomNav`
is a mobile-only layout element hidden at `width: 412` when no sessions exist (empty test
server), or the component isn't rendered at all on the landing page without sessions.

**Fix options:**
- Seed a session via `SessionClient.createIdleSession` before `page.goto` so the main app UI
  is populated and BottomNav is rendered.
- Or, assert `toBeVisible({ timeout: 15000 })` and add a `waitForSessionsActive` guard so
  the test survives slow startup.

## Q5: terminal-resync-feature-flags.spec.ts (2 rows)

`RESYNC_FLAG_LABEL = 'Terminal resync: stagger bursts'` matches `flagExtras.tsx`.  No label
mismatch.  The 2 failing rows are likely caused by `resetFlag` in `beforeEach` making a
ConnectRPC call before the server is fully ready, or a race where the POST succeeds but the
page load race means the initial fetch of flags returns before the reset completes.

**Fix:** Add a `waitForSettingsPageReady` check after navigation — ensure the flag row is
rendered before asserting its state.  The spec already calls `gotoFeaturesPage` which waits for
`h1`; adding a check for the flag row to be rendered before assertions should suffice.

## Q6: terminal-resize.spec.ts (1 row)

5 tests total.  Tests 1–4 use `test.skip(!opened, ...)` and are safe.  Test 5 ("no duplicate
lines after resize") uses `test.skip(!testInfo.project.name.includes('dom'), ...)` and creates
its own session via `createIdleSession`.  It runs only in `chromium-dom` project.  It fails when
the `expect(page.getByText('Connected')).toBeVisible({ timeout: 20000 })` assertion races with
slow session startup.

**Fix:** Call `await client.waitForSessionsActive([session.id])` before the page navigation,
same as the fix for `input-drop-badge`.

## Q7: session-client.ts API

`createIdleSession(title, path)` → returns session object.  `waitForSessionsActive([id])` polls
until all given IDs reach `SESSION_STATUS_ACTIVE`.  Used in several other specs; the pattern is
well-established.  `input-drop-badge.spec.ts` and `terminal-resize.spec.ts` test 5 omit this
call, which is the gap.
