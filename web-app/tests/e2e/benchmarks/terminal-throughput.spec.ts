/**
 * Terminal throughput benchmark.
 *
 * Measures how fast xterm.js can render terminal output delivered via the
 * stress test page. Uses performance.mark/measure inside a single
 * page.evaluate() call to avoid Playwright IPC overhead on timing boundaries.
 *
 * Output: web-app/benchmark-results.json (customBiggerIsBetter format)
 *
 * Design notes:
 * - First 2 runs are discarded as warmup (cold-start React + xterm.js init overhead).
 * - Uses page.waitForFunction() for settling instead of page.waitForTimeout().
 * - All timing is done inside a single evaluate() call to eliminate IPC overhead.
 *
 * Note on renderer: GitHub Actions' headless Chromium disables GPU acceleration,
 * so xterm.js falls back from WebGL to the canvas renderer in CI. Baselines
 * therefore reflect canvas-renderer throughput. Local runs with a GPU may use
 * WebGL and produce higher numbers — baselines are renderer-specific and not
 * cross-comparable between environments.
 *
 * @see ADR-003: Frontend Performance Measurement Strategy
 */

import { test } from '@playwright/test';
import * as path from 'path';
import {
  setupStressTestPage,
  selectPreset,
  startTest,
  waitForTestCompletion,
  getMetrics,
} from '../terminal-stress/helpers';
import { writeBenchmarkResults, computeStats } from './output-benchmark-results';

const RESULTS_PATH = path.resolve(__dirname, '../../../benchmark-results.json');
const WARMUP_RUNS = 2;
const TOTAL_RUNS = 10;
// 100KB payload: a realistic terminal burst from a Claude Code session
const PRESET = 'PAYLOAD_100KB';
const PAYLOAD_BYTES = 100 * 1024;

test.describe('Terminal Throughput Benchmark', () => {
  // Increase timeout: each run takes ~5–15s depending on rendering speed
  test.setTimeout(300_000);

  test('measure xterm.js render throughput over 10 runs', async ({ page }) => {
    const throughputSamples: number[] = [];

    for (let run = 0; run < TOTAL_RUNS; run++) {
      // Navigate fresh for each run to get a clean xterm.js state.
      // This avoids accumulated state from prior runs affecting timing.
      await setupStressTestPage(page);
      await selectPreset(page, PRESET);

      // Capture start time immediately before triggering the test
      const startTime = await page.evaluate(() => {
        performance.mark('bench-start');
        return performance.now();
      });

      await startTest(page);

      // Wait for the test to complete (all payloads rendered)
      await waitForTestCompletion(page, 30_000);

      // Capture end time and compute duration in a single evaluate() call
      // to avoid Playwright IPC overhead on the timing boundary.
      const durationMs = await page.evaluate((start: number) => {
        performance.mark('bench-end');
        performance.measure('bench-run', 'bench-start', 'bench-end');
        const entries = performance.getEntriesByName('bench-run');
        if (entries.length > 0) {
          return entries[entries.length - 1].duration;
        }
        // Fallback: use mark timestamps
        return performance.now() - start;
      }, startTime);

      const metrics = await getMetrics(page);
      const bytesRendered = metrics.bytesGenerated;
      const bytesPerSec =
        durationMs > 0 ? (bytesRendered / durationMs) * 1000 : 0;

      throughputSamples.push(bytesPerSec);

      console.log(
        `Run ${run + 1}/${TOTAL_RUNS}: ${(bytesRendered / 1024).toFixed(1)}KB in ` +
          `${durationMs.toFixed(0)}ms = ${(bytesPerSec / 1024).toFixed(0)} KB/s` +
          (run < WARMUP_RUNS ? ' [warmup, discarded]' : ''),
      );
    }

    const stats = computeStats(throughputSamples, WARMUP_RUNS);
    console.log('\n=== Throughput Stats (after warmup) ===');
    console.log(`  Mean:   ${(stats.mean / 1024).toFixed(0)} KB/s`);
    console.log(`  P50:    ${(stats.p50 / 1024).toFixed(0)} KB/s`);
    console.log(`  P95:    ${(stats.p95 / 1024).toFixed(0)} KB/s`);
    console.log(`  Min:    ${(stats.min / 1024).toFixed(0)} KB/s`);
    console.log(`  Max:    ${(stats.max / 1024).toFixed(0)} KB/s`);
    console.log(
      `  CV:     ${(stats.cv * 100).toFixed(1)}% (target: <20% for stable CI)`,
    );

    // Write results for github-action-benchmark (customBiggerIsBetter)
    writeBenchmarkResults(RESULTS_PATH, [
      {
        name: 'terminal-throughput-mean',
        unit: 'bytes/sec',
        value: Math.round(stats.mean),
        extra: `p50=${(stats.p50 / 1024).toFixed(0)}KB/s p95=${(stats.p95 / 1024).toFixed(0)}KB/s cv=${(stats.cv * 100).toFixed(1)}% payload=${PAYLOAD_BYTES / 1024}KB runs=${TOTAL_RUNS - WARMUP_RUNS}`,
      },
      {
        name: 'terminal-throughput-p50',
        unit: 'bytes/sec',
        value: Math.round(stats.p50),
      },
    ]);

    console.log(`\n✅ Results written to ${RESULTS_PATH}`);
  });
});
