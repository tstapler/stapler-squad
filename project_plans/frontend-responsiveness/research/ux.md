# UX + Measurement Research: frontend-responsiveness

Labels: VERIFIED = file opened or command run in this worktree this session; INFERRED = reasoning or recalled industry guidance, not checked against a source here (no web fetch was done; external thresholds below are from memory and should be re-checked before being cited in an ADR).

## 1. Perceived-performance UX

### 1.1 Thresholds (INFERRED, widely published)
- Response/RAIL: input -> visible response within 100 ms feels instant; keep each main-thread task under 50 ms so input is never blocked more than that; animation frames 16 ms.
- INP (Core Web Vitals): good <= 200 ms, needs improvement 200-500 ms, poor > 500 ms at p75. For a keyboard-driven localhost tool, the project's own target (switch < 150 ms, long tasks < 50 ms, requirements.md) is stricter than INP "good" and is the right bar.
- Loading feedback: nothing needed < ~100 ms; skeleton or progress after ~300-1000 ms; after ~1 s show something stateful ("still loading", retry) rather than an indefinite skeleton.

### 1.2 Comparable patterns (INFERRED)
| Product | Pattern | Takeaway for ssq |
|---|---|---|
| tmux | Window switch is a visibility swap of already-live panes; zero load | Keep-alive of recent windows (bounded) is the faithful model; the user's mental model is tmux-style (leader Alt+W, VERIFIED in `web-app/src/lib/window/useWindowShortcuts.ts`) |
| VS Code | Editor groups/tabs keep terminals and editors alive when hidden; webviews can retain context | Same; hidden surfaces stop painting/measuring |
| Linear | Local-first cache; navigation renders from cache instantly, revalidates in background; prefetch on hover/focus | Stale-while-revalidate for Backlog list; prefetch on link hover/keyboard focus |
| Browser tab strip | Switch is instant; the page you return to is exactly as left | Scroll position and focus must be restored per window |

### 1.3 Recommendation by surface
1. **Window switch**: switch the active-window indicator synchronously (optimistic: the tab highlight updates in the same frame as the click/keypress), then reveal the target window's retained DOM. If the target was evicted (beyond the keep-alive bound), render the pane chrome immediately and mount xterm lazily; do not block the tab highlight on terminal mount. Skeleton is wrong for a retained window; at most a pane-level placeholder for an evicted one.
2. **Sessions <-> Backlog**: render the Backlog shell from the last cached list (stale-while-revalidate) and show a subtle "refreshing" affordance only if revalidation exceeds ~1 s. Use route/chunk prefetch on nav-link hover/focus. Skeletons only on a true cold first load (no cache).
3. **Backlog hard load**: skeleton rows sized like real cards (avoids CLS), progressive rendering of the first screenful, defer off-screen cards (virtualize or `content-visibility: auto`).
4. **RPC stalls > 1 s**: after 1 s show inline "Still loading..." with retry; after the transport timeout show an error state with retry, never an infinite skeleton. Keep stale data visible with an "out of date" badge if present.

### 1.4 Existing building blocks (VERIFIED)
- `web-app/src/app/RoutePending.tsx`: route-chunk skeleton (3 `Skeleton` blocks), deliberately silent; comment says busyness is `aria-busy` on the layout `main` and the announcer speaks once. It has no stall/timeout/error escalation: gap for the >1 s case.
- `web-app/src/components/sessions/SessionListSkeleton.tsx` exists and is used by `SessionList.tsx` (grep hit). Pattern for the Backlog skeleton.
- `aria-busy` loading pattern exists in settings pages (`app/settings/backlog-quota/page.tsx:334`, tests assert it); grep found no `aria-busy` in `app/page.tsx` or `SessionList.tsx` (the `main`-level `aria-busy` the RoutePending comment mentions was not located in my greps; INFERRED it lives in the layout shell, not confirmed).

### 1.5 Accessibility requirements
- **Route change focus** (VERIFIED, `lib/nav/useFocusOnNavigate.ts`): on pathname change, focus moves to layout `main` unless focus is already on an input-like element inside it. `FOCUS_ON_NAVIGATE = true` (`lib/nav/policy.ts:2`).
- **Announcements** (VERIFIED, `lib/nav/RouteAnnouncer.tsx`): polite `role="status" aria-live="polite" aria-atomic` region speaking `document.title` ANNOUNCE_DELAY_MS=100 after each pathname change, cleared after 1000 ms.
- **Gap for window switching** (VERIFIED by absence): both hooks key off `usePathnameChange`. A window switch within `/` does not change pathname (windows are URL-synced via `useWindowUrlSync.ts`, mechanism not read), so a window switch likely produces no announcement and no focus move. INFERRED: needs verifying; if confirmed, requirements are:
  - Announce "Window 2 of 3: <name>" through the same polite region (do not add a second live region).
  - Move focus to the target window's active pane/terminal after the switch; with keep-alive, hidden windows must be `inert`/`display:none` (not just visually hidden) so they leave the tab order and a11y tree, and `aria-hidden` content must contain no focusable elements.
  - Hidden terminals must not double-handle keys (already a requirement-level constraint in requirements.md).
  - Keyboard path must stay the primary path: leader Alt+W then 1-9/n/p/,/o (VERIFIED, `useWindowShortcuts.ts:39-44`). The 3 s leader timeout and capture-phase catch-all disarm are existing behavior to preserve. Latency from keypress to visible switch is the metric to measure for this path, not mouse click.
- Respect `prefers-reduced-motion` for any switch transition; the recommended default is no transition at all (instant).
- Loading/error states: skeleton gets `aria-busy`; stall and error messages go in the polite region or `role="alert"` only for hard failures.

## 2. Measurement plan

### 2.1 What exists (VERIFIED)
- **Ad hoc profiling scripts** `/tmp/ssqprof/prof.cjs` and `cpu.cjs` (outside the repo, will be lost on /tmp clear):
  - `prof.cjs`: Playwright chromium with explicit headless-shell `executablePath`, hard-coded manual instance `http://localhost:62871`, `addInitScript` installing a `PerformanceObserver({entryTypes:["longtask"]})` and a `MutationObserver` counting added `.xterm` nodes (remount counter), wrapper `stat(label, fn)` that resets counters, times wall-clock, waits `networkidle`, prints long-task count/sum, xterm mounts and request count. Scenarios: load `/`, create window, switch tab0<->tab1 x3, SPA nav to `/backlog` and back x2, hard load `/backlog`.
  - `cpu.cjs`: CDP `Profiler.start/stop` per scenario, aggregates self-time per function from `profile.samples`/`timeDeltas`, prints top 14 (this is how `getBoundingClientRect` self-time is attributable).
  - Weaknesses: absolute paths and port, `networkidle` as the end condition (wall-clock includes the 500 ms/idle wait, so "ms=" overstates switch latency), `force: true` clicks bypass actionability, sleeps (`waitForTimeout`), dismissal of overlays by DOM removal, dev-state data not seeded (the 18.8 s `/` load is state-dependent), no thresholds/exit code.
- **Repo benchmarks**: `web-app/tests/e2e/benchmarks/{rpc-latency,terminal-throughput}.spec.ts` plus `output-benchmark-results.ts` (VERIFIED listing). `rpc-latency` measures `ListSessions` TTFB/total via `performance.now()` inside `page.evaluate`, discards 2 warmup runs of 12, writes `web-app/e2e-latency-results.json` in customSmallerIsBetter format; cites ADR-003 "Frontend Performance Measurement Strategy" (ADR file not located by `ls docs/adr | grep`; INFERRED it uses another name).
- **Lighthouse**: `tests/e2e/lighthouse.config.js` (desktop preset, 3 URLs against :8543, `categories:performance` warn at 0.7, numberOfRuns 1, upload to temporary-public-storage). Run in `.github/workflows/ux-analysis.yml` (continue-on-error, non-blocking) and in `benchmark.yml` job `lighthouse` ("Advisory", `bunx @lhci/cli autorun`). Lighthouse only measures cold page load, so it cannot see window switches or SPA navigation.
- **CI benchmark plumbing** (`benchmark.yml`, VERIFIED by grep): Go tier-1 with benchstat baseline in cache and `tools/ci-gates/benchmark-regression.js`; `e2e-latency` job restores/saves a baseline via the `bench-e2e-latency-` cache key and checks the results file exists. This is the closest existing home for a frontend perf budget.
- **Client vitals**: `web-app/src/components/telemetry/WebVitalsReporter.tsx` (VERIFIED) uses `web-vitals` 6.2.3 (`onCLS/onFCP/onINP/onLCP/onTTFB`), writes `performance.mark("web-vital:<NAME>", {detail})`, and sends `web_vital.*` analytics events with the active session id. INP is already collected in production; but INP reports only the worst interaction per page lifetime, so it does not attribute to "window switch".
- No `performance.measure` or `longtask` usage in `web-app/src` or `tests` per the greps I managed to run (my glob-style grep command errored on `--include`, so INFERRED; re-run with the Grep tool before relying on it).

### 2.2 Proposed metrics (all measured by the same harness before and after)
Map directly to requirements.md success metrics:
| Metric | How | Source |
|---|---|---|
| Window switch time-to-interactive | App emits `performance.mark("ssq:window-switch:start")` on the action (click/keydown handler) and `...:end` after the next committed paint of the target window (double `requestAnimationFrame` after the state commit), then `performance.measure("ssq:window-switch", ...)`. Test reads `performance.getEntriesByName`. | new, client |
| Route change duration | Same pattern around `usePathnameChange` (start on link click/prefetch trigger, end on first paint of the destination's content, not the skeleton) | new, client |
| Long tasks during scenario | `PerformanceObserver longtask` (exists in prof.cjs); report count, sum, max. Note: only tasks > 50 ms; for finer data use CDP tracing | harness |
| Long Animation Frames | `PerformanceObserver({type:"long-animation-frame"})` (Chromium 123+, INFERRED availability) gives script attribution without DevTools; good for pinpointing `getBoundingClientRect` callers | harness |
| `getBoundingClientRect` self-time | CDP `Profiler` aggregation from cpu.cjs, run headless without DevTools open | harness |
| Forced-layout count | CDP `Performance.getMetrics` (`LayoutCount`, `RecalcStyleCount`, `LayoutDuration`, `RecalcStyleDuration`) delta per scenario; cheap, deterministic-ish proxy for the 1,646 `UpdateLayoutTree` calls | harness |
| Request pressure | Playwright `page.on("request"/"requestfinished")` with `request.timing()`: per-scenario count of calls by RPC name, max `blocked`-equivalent (`requestStart - startTime`), peak concurrent in-flight per origin (compute from start/finish timestamps), duplicate count of `GetFeatureFlags` | harness; replicates HAR metrics without manual export |
| xterm remounts | MutationObserver counter (exists) | harness |
| Dev-only in-flight gauge | Wrap the shared transport fetch with a counter exposed on `window.__ssqPerf` only when a dev/test flag is set (requirements.md Observability) | new, client |

Keep `window.__ssqPerf` (marks + counters) behind a build/runtime flag so production carries only the marks. Prefer the standard User Timing API over a custom bus so DevTools, Lighthouse user-timing audits, and Playwright all see the same data.

### 2.3 Harness design (isolated, repeatable)
1. Move the scripts into the repo as a Playwright spec, e.g. `web-app/tests/e2e/perf/` next to `benchmarks/`, using `@playwright/test` fixtures rather than raw `chromium.launch` with a hard-coded executable path (the path in prof.cjs is machine-specific).
2. Server: reuse `tests/e2e/global-setup.ts`-style isolated instance (random port, temp dir, `--test-mode --tmux-keep-server`, per CLAUDE.md) so it never touches :8543. For local developer runs, the CLAUDE.md manual-instance recipe with `STAPLER_SQUAD_TMUX_SOCKET` and ports 62871+ (VERIFIED in CLAUDE.md; prof.cjs hard-codes 62871).
3. Seed deterministic state: N sessions (e.g. 20), 3 windows with terminals, M backlog items (e.g. 200). The baseline numbers in requirements.md (18.8 s `/` load) come from a dev-state instance and are not reproducible without a fixture; define one fixture and re-baseline on it before changing code.
4. Throttling for CI stability: CDP `Emulation.setCPUThrottlingRate` (e.g. 4x) so regressions rise above runner noise; and optionally `Network.emulateNetworkConditions` off (localhost). Run each scenario >= 7 times, discard the first 2 (warmup, same as rpc-latency), assert on the median and report p90.
5. Replace `networkidle` and `waitForTimeout` with explicit end markers (the `ssq:*:end` mark) to measure the interaction instead of the quiet period. This also satisfies the repo's e2e rule banning `waitForTimeout` (VERIFIED in CLAUDE.md "Conventions (enforced in CI)"); locators must use `data-testid`/ARIA (the existing scripts use `a[href=...]` and `[class*="card"]`, which would violate that).
6. Use Chromium headless-shell consistently; keep DevTools closed (requirements.md notes DevTools inflated the exported trace). CPU profiler use perturbs timing, so run the CDP Profiler pass separately from the timing pass.
7. Spec header `// @feature ...` per convention.

### 2.4 Budgets and CI placement
- Budgets (derive from requirements.md; set initial CI thresholds with headroom after the first re-baseline, then ratchet):
  - window switch median <= 150 ms; no long task > 100 ms during switch; long-task sum < 50 ms per switch (target) / looser in CI.
  - `/backlog` hard load long-task total < 1 s.
  - Per scenario: zero requests with queue time > 1 s; <= 1 `GetFeatureFlags`; duplicate-RPC count per method <= configured cap.
  - Forced layout: `LayoutCount` delta and `getBoundingClientRect` self-time vs. stored baseline (-80%).
- Gate type: absolute budgets for the user-felt metrics (switch ms, long-task max, duplicate RPC count, peak in-flight) because they are deterministic enough and map to the success metrics; relative-to-baseline (benchstat-style, existing `benchmark-regression.js` pattern, cache key `bench-e2e-latency-`) for time-based numbers prone to runner variance.
- Where: add a job beside `e2e-latency` in `.github/workflows/benchmark.yml` (it already builds the frontend, restores a baseline cache, and checks result files; VERIFIED by grep, job bodies not read in full). Start advisory (PR comment, mirroring the Go tier-1 comment step) for the soak period, then make the deterministic subset (duplicate-RPC count, peak concurrent in-flight, long task max) blocking. Lighthouse remains advisory (cold load only).
- Unit-level guard cheap to run in `make test`: a Jest test that mounts the shared transport/flag provider and asserts one `GetFeatureFlags` call for N consumers; and a lint/ESLint `no-restricted-imports` rule banning direct `createConnectTransport` imports outside the shared module (37 files import it today per requirements.md; the repo already uses `no-restricted-imports` for nav, VERIFIED in `.claude/rules/nav-facade.md`). This is the structural regression guard for the request fan-out, stronger than a timing budget.
- Production: keep `WebVitalsReporter` INP and add the `ssq:window-switch` / `ssq:route-change` measures as analytics `performance` events so real-use regressions show up without CI.

## 3. Risks and open points
- Window switch has no pathname change, so existing focus/announce hooks do not cover it (INFERRED, verify in `useWindowUrlSync.ts` / `useWindowManager.ts`).
- Keep-alive plus a11y: hidden windows must be `inert`; verify with the existing Axe route sweep (`tests/e2e/a11y-route-sweep.spec.ts`, exists, not read).
- Measurement noise on shared CI runners: use CPU throttling, medians, and counts over milliseconds where possible.
- Baselines in requirements.md came from a dev-state instance and DevTools-attached trace; re-baseline on the seeded fixture before claiming any win.
