import { test, expect } from '@playwright/test';
import {
  setupStressTestPage,
  selectPreset,
  startTest,
  stopTest,
  waitForTestCompletion,
  getMetrics,
} from './helpers';

test.describe('Large Payload Tests', () => {
  test.beforeEach(async ({ page }) => {
    await setupStressTestPage(page);
  });

  test('should handle 100KB payload without freezing', async ({ page }) => {
    await selectPreset(page, 'PAYLOAD_100KB');
    await startTest(page);

    // Wait for test to complete (5 payloads over 5 seconds)
    await waitForTestCompletion(page, 10000);

    const metrics = await getMetrics(page);
    console.log('100KB payload metrics:', metrics);

    // Should have processed payloads
    expect(metrics.framesRendered).toBeGreaterThan(0);
    expect(metrics.bytesGenerated).toBeGreaterThan(100 * 1024);

    // Frame times may be longer for large writes
    expect(metrics.avgFrameTime).toBeLessThan(500); // Max 500ms per payload
    expect(metrics.maxFrameTime).toBeLessThan(2000); // Max 2s spike
  });

  test('should handle 1MB payload', async ({ page }) => {
    // Increase test timeout for large payload
    test.setTimeout(60000);

    await selectPreset(page, 'PAYLOAD_1MB');
    await startTest(page);

    // Wait for test to complete (3 payloads over 3 seconds)
    await waitForTestCompletion(page, 30000);

    const metrics = await getMetrics(page);
    console.log('1MB payload metrics:', metrics);

    // Should have processed at least some payloads
    expect(metrics.framesRendered).toBeGreaterThan(0);

    // 1MB payloads are intensive - allow longer frame times
    expect(metrics.maxFrameTime).toBeLessThan(5000); // Max 5s per payload
  });

  test('should not corrupt terminal state with large payloads', async ({ page }) => {
    await selectPreset(page, 'PAYLOAD_100KB');
    await startTest(page);

    // Wait for first payload
    await page.waitForTimeout(2000);

    // Terminal should have content
    const screenshot1 = await page.getByTestId('terminal-container').screenshot();
    expect(screenshot1.length).toBeGreaterThan(5000);

    // Wait for more payloads
    await waitForTestCompletion(page, 10000);

    // Terminal should still have valid content
    const screenshot2 = await page.getByTestId('terminal-container').screenshot();
    expect(screenshot2.length).toBeGreaterThan(5000);

    const metrics = await getMetrics(page);
    console.log('Final metrics:', metrics);
  });

  test('should remain responsive during large payload processing', async ({ page }) => {
    await selectPreset(page, 'PAYLOAD_100KB');
    await startTest(page);

    // During payload processing, UI should remain responsive
    for (let i = 0; i < 5; i++) {
      await page.waitForTimeout(1000);

      // Can still interact with page
      const status = await page.getByTestId('test-status').textContent();
      expect(['running', 'completed']).toContain(status);

      // Metrics should be updating
      const metrics = await getMetrics(page);
      console.log(`Check ${i + 1}: frames=${metrics.framesRendered}, bytes=${metrics.bytesGenerated}`);
    }

    await stopTest(page);
  });

  test('should handle payload with flow control active', async ({ page }) => {
    // 100KB payloads may trigger flow control (HIGH_WATERMARK = 100KB)
    await selectPreset(page, 'PAYLOAD_100KB');
    await startTest(page);

    await waitForTestCompletion(page, 15000);

    const metrics = await getMetrics(page);
    console.log('Flow control test metrics:', metrics);

    // Even with flow control, should eventually complete
    expect(metrics.framesRendered).toBeGreaterThan(0);

    // Check for memory stability
    expect(metrics.memoryGrowthRate).toBeLessThan(5 * 1024 * 1024); // Less than 5MB/s growth
  });

  test('should successfully reset after large payload test', async ({ page }) => {
    await selectPreset(page, 'PAYLOAD_100KB');
    await startTest(page);
    await page.waitForTimeout(2000);
    await stopTest(page);

    // Get metrics before reset
    const metricsBeforeReset = await getMetrics(page);
    expect(metricsBeforeReset.framesRendered).toBeGreaterThan(0);

    // Reset
    await page.getByTestId('reset-test').click();
    await expect(page.getByTestId('test-status')).toHaveText('idle');

    // Metrics should be zeroed
    const metricsAfterReset = await getMetrics(page);
    expect(metricsAfterReset.framesRendered).toBe(0);

    // Should be able to start new test
    await startTest(page);
    await page.waitForTimeout(1000);

    const newMetrics = await getMetrics(page);
    expect(newMetrics.framesRendered).toBeGreaterThan(0);

    await stopTest(page);
  });
});
