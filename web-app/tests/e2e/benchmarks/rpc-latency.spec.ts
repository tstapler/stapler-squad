/**
 * E2E RPC latency benchmark.
 *
 * Measures the full request path from browser → Go backend:
 *   - TTFB (time to first byte): server processing time
 *   - Total RPC time: full request/response round trip
 *
 * Timing is measured inside page.evaluate() using performance.now() to avoid
 * Playwright IPC overhead on the timing boundaries.
 *
 * Output: web-app/e2e-latency-results.json (customSmallerIsBetter format)
 *
 * Prerequisites:
 *   - Go backend server must be running on localhost:8543
 *   - Frontend server must be running (managed by playwright.config.ts webServer)
 *
 * Design notes:
 * - First 2 samples discarded as warmup (connection pool cold-start).
 * - Backend URL defaults to localhost:8543; override with BACKEND_URL env var.
 *
 * @see ADR-003: Frontend Performance Measurement Strategy
 */

import { test, expect } from '@playwright/test';
import * as path from 'path';
import { writeBenchmarkResults, computeStats } from './output-benchmark-results';

const E2E_RESULTS_PATH = path.resolve(
  __dirname,
  '../../../e2e-latency-results.json',
);
// ConnectRPC endpoint on the Go backend. Override with BACKEND_URL env var in CI.
const BACKEND_URL = process.env['BACKEND_URL'] ?? 'http://localhost:8543';
const LIST_SESSIONS_PATH = '/api/session.v1.SessionService/ListSessions';
const WARMUP_RUNS = 2;
const TOTAL_RUNS = 10;

test.describe('RPC Latency Benchmark', () => {
  test.setTimeout(120_000);

  test('measure ListSessions RPC latency over 10 samples', async ({ page }) => {
    // Navigate to any page to establish the browser context
    await page.goto('/');

    const ttfbSamples: number[] = [];
    const totalSamples: number[] = [];

    for (let run = 0; run < TOTAL_RUNS; run++) {
      // Measure timing inside the page using performance.now() to avoid
      // Playwright IPC overhead on the timing boundaries.
      const { ttfb, total } = await page.evaluate(
        async ({ url }: { url: string }) => {
          const start = performance.now();
          const response = await fetch(url, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Connect-Protocol-Version': '1',
            },
            body: JSON.stringify({}),
          });
          const ttfb = performance.now() - start;
          await response.json();
          const total = performance.now() - start;
          return { ttfb, total };
        },
        { url: `${BACKEND_URL}${LIST_SESSIONS_PATH}` },
      );

      // Guard against invalid values
      if (ttfb >= 0 && total > 0) {
        ttfbSamples.push(ttfb);
        totalSamples.push(total);
      }

      console.log(
        `Run ${run + 1}/${TOTAL_RUNS}: TTFB=${ttfb.toFixed(1)}ms total=${total.toFixed(1)}ms` +
          (run < WARMUP_RUNS ? ' [warmup, discarded]' : ''),
      );
    }

    // Expect at least enough valid samples after warmup
    const validSamples = totalSamples.length;
    expect(validSamples).toBeGreaterThanOrEqual(TOTAL_RUNS - WARMUP_RUNS);

    const ttfbStats = computeStats(ttfbSamples, WARMUP_RUNS);
    const totalStats = computeStats(totalSamples, WARMUP_RUNS);

    console.log('\n=== RPC Latency Stats (after warmup) ===');
    console.log(`  TTFB  mean: ${ttfbStats.mean.toFixed(1)}ms  p95: ${ttfbStats.p95.toFixed(1)}ms  cv: ${(ttfbStats.cv * 100).toFixed(1)}%`);
    console.log(`  Total mean: ${totalStats.mean.toFixed(1)}ms  p95: ${totalStats.p95.toFixed(1)}ms  cv: ${(totalStats.cv * 100).toFixed(1)}%`);

    // Write results for CI baseline comparison (customSmallerIsBetter)
    writeBenchmarkResults(E2E_RESULTS_PATH, [
      {
        name: 'list-sessions-ttfb-mean',
        unit: 'ms',
        value: parseFloat(ttfbStats.mean.toFixed(2)),
        extra: `p95=${ttfbStats.p95.toFixed(1)}ms min=${ttfbStats.min.toFixed(1)}ms max=${ttfbStats.max.toFixed(1)}ms cv=${(ttfbStats.cv * 100).toFixed(1)}%`,
      },
      {
        name: 'list-sessions-total-mean',
        unit: 'ms',
        value: parseFloat(totalStats.mean.toFixed(2)),
        extra: `p95=${totalStats.p95.toFixed(1)}ms min=${totalStats.min.toFixed(1)}ms max=${totalStats.max.toFixed(1)}ms`,
      },
    ]);

    console.log(`\n✅ Results written to ${E2E_RESULTS_PATH}`);
  });
});
