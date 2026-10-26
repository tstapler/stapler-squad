// @feature session:create, backlog-list-items
import { test, expect, Browser, Page, CDPSession } from '@playwright/test';
import { execFileSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import { PerfPage } from '../pages/PerfPage';
import { perfFixtureShape, perfInitScript, seedPerfFixture, SeededFixture, tmuxPaneDims } from './fixture';
import { summarizeProfile, CpuProfile } from './cpuProfile';

/**
 * Frontend-responsiveness perf spec (Story 1.1.3). Run only via `--project=perf` against the isolated
 * global-setup instance. Env: PERF_RUNS (kept runs, default 5), PERF_SESSIONS / PERF_BACKLOG (fallback fixture
 * sizes), PERF_SCENARIOS (comma list), PERF_WRITE_BASELINE=1 (write baseline.json instead of asserting budgets).
 */
const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const KEPT_RUNS = Number(process.env.PERF_RUNS ?? 5);
const LOAD_WINDOW_MS = 16000;
const SWITCHES = 7;
const SWITCH_WARMUP = 2;
const ECHO_ROUND_TRIPS = Number(process.env.PERF_ECHOES ?? 30);
const BASELINE_PATH = path.join(__dirname, 'baseline.json');
const THROTTLES = [4, 1] as const;
type Throttle = (typeof THROTTLES)[number];

/** PERF_KEEP_ALIVE=on|off pins `window_keep_alive` for the run; every number in the report is for that state. */
const KEEP_ALIVE = process.env.PERF_KEEP_ALIVE as 'on' | 'off' | undefined;
const RETAINED_SWITCH_MEDIAN_MS = 150; // OPEN-DECISION-throttle-gate: recorded as annotations, not a hard fail
const RETAINED_LONG_TASK_MAX_MS = 50;
const COLD_LONG_TASK_MAX_MS = 100;

const wanted = (name: string) => !process.env.PERF_SCENARIOS || process.env.PERF_SCENARIOS.split(',').includes(name);

// ---- statistics ----
function median(xs: number[]): number {
  const s = [...xs].sort((a, b) => a - b);
  const m = Math.floor(s.length / 2);
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
}
function mad(xs: number[]): number {
  const m = median(xs);
  return median(xs.map((x) => Math.abs(x - m)));
}
function percentile(xs: number[], p: number): number {
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.min(s.length - 1, Math.ceil(p * s.length) - 1)];
}

interface RunResult {
  [metric: string]: number | null;
}
interface Aggregate {
  median: number | null;
  mad: number | null;
  runs: (number | null)[];
}
interface ScenarioReport {
  throttle: Throttle;
  keptRuns: number;
  metrics: Record<string, Aggregate>;
  requestsByRpc: Record<string, Aggregate>;
}

function aggregate(runs: (number | null)[]): Aggregate {
  const nums = runs.filter((x): x is number => x !== null);
  return nums.length ? { median: median(nums), mad: mad(nums), runs } : { median: null, mad: null, runs };
}

const rpcName = (url: string) => {
  const m = url.match(/\/api\/[\w.]+\/([A-Za-z0-9]+)(?:\?|$)/);
  return m ? m[1] : null;
};

// ---- harness ----
let fixture: SeededFixture;
const report: { scenarios: Record<string, ScenarioReport>; cpu: Record<string, unknown> } = { scenarios: {}, cpu: {} };

async function openPage(browser: Browser, throttle: Throttle) {
  const context = await browser.newContext({ viewport: { width: 1600, height: 900 }, baseURL: BASE_URL });
  const page = await context.newPage();
  page.setDefaultTimeout(90000);
  page.setDefaultNavigationTimeout(180000);
  await page.addInitScript(perfInitScript(fixture.layout));
  const perf = new PerfPage(page);
  await perf.installObservers();
  const cdp = await context.newCDPSession(page);
  await perf.setCpuThrottle(cdp, throttle);
  return { context, page, perf, cdp };
}

interface Ctx {
  page: Page;
  perf: PerfPage;
  cdp: CDPSession;
  runIndex: number;
}

/**
 * Runs `body` `keptRuns` times (plus `extraRuns` leading warmup runs that are dropped) on a fresh page per
 * throttle condition. `body` returns the run's metrics; the harness adds long-task, LayoutCount, request and
 * gauge metrics measured around it.
 */
async function runScenario(
  browser: Browser,
  name: string,
  throttle: Throttle,
  body: (c: Ctx, reqs: string[]) => Promise<RunResult>,
  opts: { extraRuns?: number; prepare?: (c: Ctx) => Promise<void>; resetBodyCounters?: boolean } = {}
): Promise<ScenarioReport> {
  const { context, page, perf, cdp } = await openPage(browser, throttle);
  const reqs: string[] = [];
  page.on('request', (r) => {
    const n = rpcName(r.url());
    if (n) reqs.push(n);
  });
  const perRun: RunResult[] = [];
  const perRunReqs: Record<string, number>[] = [];
  try {
    for (let i = 0; i < KEPT_RUNS + (opts.extraRuns ?? 0); i++) {
      const c: Ctx = { page, perf, cdp, runIndex: i };
      await opts.prepare?.(c);
      reqs.length = 0;
      await perf.resetCounters().catch(() => undefined);
      const layout0 = await perf.layoutCount(cdp);
      const t0 = Date.now();
      const out = await body(c, reqs);
      const wallMs = Date.now() - t0;
      const counters = await perf.readCounters();
      const layout1 = await perf.layoutCount(cdp);
      if (i < (opts.extraRuns ?? 0)) continue;
      perRun.push({
        wallMs,
        longTaskCount: counters.longTasks.length,
        longTaskSumMs: Math.round(counters.longTasks.reduce((a, b) => a + b, 0)),
        longTaskMaxMs: Math.round(Math.max(0, ...counters.longTasks)),
        layoutCountDelta: layout1 - layout0,
        xtermMounts: counters.xtermMounts,
        peakInFlight: counters.gauge?.peakInFlight ?? null,
        queuedOver1s: counters.gauge?.queuedOver1s ?? null,
        queuedOver1sCritical: counters.gauge?.queuedOver1sCritical ?? null,
        queuedOver1sDeferred: counters.gauge?.queuedOver1sDeferred ?? null,
        eventP75Ms: counters.eventDurations.length ? percentile(counters.eventDurations, 0.75) : null,
        ...out,
      });
      const byRpc: Record<string, number> = {};
      for (const n of reqs) byRpc[n] = (byRpc[n] ?? 0) + 1;
      perRunReqs.push(byRpc);
    }
  } finally {
    await context.close();
  }
  const keys = [...new Set(perRun.flatMap((r) => Object.keys(r)))];
  const rpcs = [...new Set(perRunReqs.flatMap((r) => Object.keys(r)))];
  const scenario: ScenarioReport = {
    throttle,
    keptRuns: perRun.length,
    metrics: Object.fromEntries(keys.map((k) => [k, aggregate(perRun.map((r) => r[k] ?? null))])),
    requestsByRpc: Object.fromEntries(rpcs.map((k) => [k, aggregate(perRunReqs.map((r) => r[k] ?? 0))])),
  };
  report.scenarios[`${name}@${throttle}x`] = scenario;
  console.log(`PERF_SCENARIO ${name}@${throttle}x ${JSON.stringify(scenario)}`);
  return scenario;
}

async function loadShell({ page, perf }: Ctx, url: string) {
  await page.goto(url);
  await perf.waitForShell();
  await perf.dismissOverlays();
}

// ---- per-method request budgets (Story 1.1.3) ----
const loadBudgets = (durationS: number): Record<string, number> => ({
  GetFeatureFlags: 1,
  ListProgramsConfig: 1,
  LogClientEvents: Math.ceil(durationS / 5) + 1,
  GetFileTransportCapabilities: 1,
  ListWorkflows: 1,
});

function readBaseline(): { expectedFail?: string[] } | null {
  try {
    return JSON.parse(fs.readFileSync(BASELINE_PATH, 'utf8'));
  } catch {
    return null;
  }
}

function budgetViolations(s: ScenarioReport, durationS: number): string[] {
  return Object.entries(loadBudgets(durationS))
    .filter(([rpc, max]) => (s.requestsByRpc[rpc]?.median ?? 0) > max)
    .map(([rpc]) => `${rpc}<=${loadBudgets(durationS)[rpc]}`);
}

test.describe('frontend responsiveness (perf)', () => {
  test.beforeAll(async () => {
    test.setTimeout(900000);
    if (KEEP_ALIVE) {
      const resp = await fetch(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'window_keep_alive', enabled: KEEP_ALIVE === 'on' }),
      });
      if (!resp.ok) throw new Error(`UpdateFeatureFlag window_keep_alive failed: HTTP ${resp.status}`);
    }
    fixture = await seedPerfFixture(BASE_URL, {
      sessionCount: process.env.PERF_SESSIONS ? Number(process.env.PERF_SESSIONS) : undefined,
      backlogCount: process.env.PERF_BACKLOG ? Number(process.env.PERF_BACKLOG) : undefined,
    });
  });

  test.afterAll(async () => {
    const out = {
      metadata: {
        capturedAt: new Date().toISOString(),
        commit: safeGit(['rev-parse', 'HEAD']),
        fixture: {
          shape: perfFixtureShape,
          actualSessions: fixture?.sessionTitles.length,
          actualBacklogItems: process.env.PERF_BACKLOG ? Number(process.env.PERF_BACKLOG) : perfFixtureShape.backlogItems,
          seedMs: fixture?.seedMs,
        },
        windowKeepAlive: KEEP_ALIVE ?? 'server-default',
        keptRuns: KEPT_RUNS,
        throttles: THROTTLES,
        viewport: '1600x900',
        host: { loadAvg1m: require('os').loadavg()[0], cpus: require('os').cpus().length },
      },
      expectedFail: Object.entries(report.scenarios)
        .filter(([k]) => k.startsWith('load@'))
        .flatMap(([, s]) => budgetViolations(s, LOAD_WINDOW_MS / 1000)),
      ...report,
    };
    fs.mkdirSync('test-results', { recursive: true });
    fs.writeFileSync('test-results/perf-results.json', JSON.stringify(out, null, 2));
    if (process.env.PERF_WRITE_BASELINE === '1') fs.writeFileSync(BASELINE_PATH, JSON.stringify(out, null, 2) + '\n');
  });

  for (const throttle of THROTTLES) {
    test(`load / ${throttle}x`, async ({ browser }) => {
      test.skip(!wanted('load'));
      const s = await runScenario(browser, 'load', throttle, async (c) => {
        await loadShell(c, '/');
        await c.page.waitForFunction((ms) => performance.now() > ms, LOAD_WINDOW_MS, { polling: 'raf', timeout: 120000 });
        return {};
      });
      assertBudgets(s);
      assertCriticalNeverQueued(s);
    });

    test(`switch window retained ${throttle}x`, async ({ browser }) => {
      test.skip(!wanted('switchRetained'));
      const s = await runScenario(browser, 'switchRetained', throttle, async (c) => {
        const ms: number[] = [];
        const highlight: boolean[] = [];
        for (let i = 0; i < SWITCHES; i++) {
          if (i === SWITCH_WARMUP) await c.perf.resetCounters();
          const d = await c.perf.switchWindow(i % 2 === 0 ? 1 : 0);
          if (i >= SWITCH_WARMUP && d !== null) ms.push(d);
          if (i >= SWITCH_WARMUP) highlight.push(await c.perf.lastHighlightFirstFrame());
        }
        return { switchMsMedian: median(ms), highlightFirstFrame: Number(highlight.every(Boolean)) };
      }, { prepare: (c) => loadShell(c, '/') });
      assertCriticalNeverQueued(s);
      annotateTarget('retained switch median', s.metrics.switchMsMedian?.median, RETAINED_SWITCH_MEDIAN_MS);
      annotateTarget('retained long-task max', s.metrics.longTaskMaxMs?.median, RETAINED_LONG_TASK_MAX_MS);
    });

    test(`switch window cold ${throttle}x`, async ({ browser }) => {
      test.skip(!wanted('switchCold'));
      const s = await runScenario(browser, 'switchCold', throttle, async (c) => {
        // First visit of w2..w4, then w1 again: w1 has been LRU-evicted once K = 3 other windows were visited.
        const first = await c.perf.switchWindow(1);
        await c.perf.switchWindow(2);
        await c.perf.switchWindow(3);
        const evicted = await c.perf.switchWindow(0);
        return { coldFirstVisitMs: first, coldEvictedRevisitMs: evicted };
      }, { prepare: (c) => loadShell(c, '/') });
      // Cold switches are out of the 150 ms / 50 ms retained numbers but not out of the 100 ms task ceiling.
      annotateTarget('cold long-task max', s.metrics.longTaskMaxMs?.median, COLD_LONG_TASK_MAX_MS);
    });

    test(`switch window tmux pane dims ${throttle}x`, async ({ browser }) => {
      test.skip(!wanted('switchTmuxDims') || throttle !== 1);
      const names = fixture.sessionTitles.map((t) => `staplersquad_${t.replace(/[^A-Za-z0-9_-]/g, '_')}`);
      const { context, page, perf, cdp } = await openPage(browser, throttle);
      try {
        await loadShell({ page, perf, cdp, runIndex: 0 }, '/');
        const before = await Promise.all(names.map((n) => tmuxPaneDims(n)));
        // w2..w4 first visits (cold), then w1 again (evicted at K = 3), then w2 again (retained).
        for (const index of [1, 2, 3, 0, 1]) {
          await perf.switchWindow(index);
          const after = await Promise.all(names.map((n) => tmuxPaneDims(n)));
          expect(after, `tmux pane dims after switching to window ${index + 1} (keep-alive ${KEEP_ALIVE ?? 'server-default'})`).toEqual(before);
          for (const dims of after) expect(dims).not.toMatch(/^(0x|1x1$)|x0$/);
        }
      } finally {
        await context.close();
      }
    });

    test(`switch window leader key ${throttle}x`, async ({ browser }) => {
      test.skip(!wanted('switchLeader'));
      await runScenario(browser, 'switchLeader', throttle, async (c) => {
        const before = await c.perf.measureCount('ssq:window-switch');
        await c.page.keyboard.press('Alt+w');
        await c.page.keyboard.press('2');
        await c.perf.waitForMeasureGrowth('ssq:window-switch', before);
        const ms = await c.perf.lastMeasureMs('ssq:window-switch');
        await c.perf.switchWindow(0);
        return { leaderSwitchMs: ms };
      }, { prepare: (c) => loadShell(c, '/') });
    });

    test(`route / -> /backlog -> / ${throttle}x`, async ({ browser }) => {
      test.skip(!wanted('routeRoundTrip'));
      await runScenario(browser, 'routeRoundTrip', throttle, async (c) => {
        const to: number[] = [];
        const back: number[] = [];
        for (let i = 0; i < 4; i++) {
          await c.perf.spaNavigate('/backlog');
          to.push((await c.perf.lastMeasureMs('ssq:route-change')) ?? NaN);
          await c.perf.spaNavigate('/');
          back.push((await c.perf.lastMeasureMs('ssq:route-change')) ?? NaN);
        }
        return { toBacklogMedianMs: median(to), backToSessionsMedianMs: median(back) };
      }, { prepare: (c) => loadShell(c, '/') });
    });

    for (const mode of ['cold', 'warm'] as const) {
      test(`hard load /backlog ${mode} ${throttle}x`, async ({ browser }) => {
        test.skip(!wanted('backlogHardLoad'));
        await runScenario(browser, `backlogHardLoad-${mode}`, throttle, async (c) => {
          if (mode === 'cold') {
            await c.page.goto('/');
            await c.page.evaluate(() => {
              sessionStorage.clear();
              const keep = ['ssq:perf', 'cockpit.windowLayout', 'cockpit.lastFocusedWindowId'];
              Object.keys(localStorage).filter((k) => !keep.includes(k) && !k.startsWith('ssq-')).forEach((k) => localStorage.removeItem(k));
            });
          }
          const t0 = Date.now();
          await c.page.goto('/backlog');
          await c.page.getByTestId('backlog-page').waitFor({ timeout: 120000 });
          await c.page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
          return { backlogReadyMs: Date.now() - t0 };
        });
      });
    }

    test(`terminal echo ${throttle}x`, async ({ browser }) => {
      test.skip(!wanted('terminalEcho'));
      await runScenario(browser, 'terminalEcho', throttle, async (c) => {
        const frames: { t: number; text: string }[] = [];
        c.page.on('websocket', (ws) =>
          ws.on('framereceived', ({ payload }) => frames.push({ t: performance.now(), text: payload.toString() }))
        );
        const rtt: number[] = [];
        for (let n = 0; n < ECHO_ROUND_TRIPS; n++) {
          // Window-switch workload runs between round trips; each echo lands on the focused terminal of the active window.
          await c.perf.switchWindow(n % 2);
          const input = c.page.getByRole('textbox', { name: 'Terminal input' }).locator('visible=true').first();
          await input.focus();
          // `m''ark` makes the typed echo differ from the command's output, so only the output matches `__ssq_mark_<n>`.
          await input.pressSequentially(`echo __ssq_m''ark_${n}`);
          const from = frames.length;
          const t0 = performance.now();
          await c.page.keyboard.press('Enter');
          await expect
            .poll(() => frames.slice(from).find((f) => f.text.includes(`__ssq_mark_${n}`))?.t ?? null, { timeout: 30000 })
            .not.toBeNull();
          rtt.push(frames.slice(from).find((f) => f.text.includes(`__ssq_mark_${n}`))!.t - t0);
        }
        return { echoP50Ms: percentile(rtt, 0.5), echoP95Ms: percentile(rtt, 0.95) };
      }, { prepare: (c) => loadShell(c, '/') });
    });
  }

  // CDP Profiler pass: separate from the timing pass, unthrottled, one run each.
  test('criticalCalls_should_HaveZeroQueuedOver1s_When_LoadScenarioRuns', async () => {
    const loads = Object.entries(report.scenarios).filter(([k]) => k.startsWith('load@'));
    test.skip(loads.length === 0, 'load scenario did not run in this invocation');
    for (const [, s] of loads) assertCriticalNeverQueued(s);
  });

  test('cpu profile: window switch and /backlog load', async ({ browser }) => {
    test.skip(!wanted('cpu'));
    const { context, page, perf, cdp } = await openPage(browser, 1);
    try {
      await loadShell({ page, perf, cdp, runIndex: 0 }, '/');
      await cdp.send('Profiler.enable');
      const prof = async (label: string, fn: () => Promise<void>) => {
        await cdp.send('Profiler.start');
        await fn();
        const { profile } = await cdp.send('Profiler.stop');
        report.cpu[label] = summarizeProfile(profile as unknown as CpuProfile);
        console.log(`PERF_CPU ${label} ${JSON.stringify(report.cpu[label])}`);
      };
      await prof('switchX4', async () => {
        for (let i = 0; i < 4; i++) await perf.switchWindow(i % 2);
      });
      await prof('loadBacklog', async () => {
        await page.goto('/backlog');
        await page.getByTestId('backlog-page').waitFor({ timeout: 120000 });
      });
      expect(Object.keys(report.cpu)).toHaveLength(2);
    } finally {
      await context.close();
    }
  });
});

function annotateTarget(label: string, actual: number | null | undefined, max: number) {
  if (actual == null || actual <= max) return;
  test.info().annotations.push({ type: 'over-target', description: `${label}: ${actual} ms > ${max} ms (keep-alive ${KEEP_ALIVE ?? 'server-default'})` });
}

/** Story 2.1.9: deferral must keep every `critical` call out of the browser's 6-connection queue (> 1 s). */
function assertCriticalNeverQueued(s: ScenarioReport) {
  if (process.env.PERF_WRITE_BASELINE === '1') return;
  const worst = Math.max(0, ...(s.metrics.queuedOver1sCritical?.runs ?? []).map((n) => n ?? 0));
  expect(worst, 'critical calls with requestStart - fetchStart > 1000 ms').toBe(0);
}

function assertBudgets(s: ScenarioReport) {
  const violations = budgetViolations(s, LOAD_WINDOW_MS / 1000);
  if (process.env.PERF_WRITE_BASELINE === '1') return; // baseline capture records violations as expectedFail
  const known = new Set(readBaseline()?.expectedFail ?? []);
  const unexpected = violations.filter((v) => !known.has(v));
  const expectedFails = violations.filter((v) => known.has(v));
  if (expectedFails.length) test.info().annotations.push({ type: 'expected-fail', description: expectedFails.join(', ') });
  expect(unexpected, `request budgets exceeded: ${unexpected.join(', ')}`).toEqual([]);
}

function safeGit(args: string[]): string | null {
  try {
    return execFileSync('git', args, { encoding: 'utf8' }).trim();
  } catch {
    return null;
  }
}
