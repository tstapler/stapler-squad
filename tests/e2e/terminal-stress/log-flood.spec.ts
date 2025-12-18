import { test, expect } from '@playwright/test';
import {
  setupStressTestPage,
  selectPreset,
  startTest,
  stopTest,
  waitForTestCompletion,
  getMetrics,
  assertFrameRate,
} from './helpers';

test.describe('Log Flood Tests', () => {
  test.beforeEach(async ({ page }) => {
    await setupStressTestPage(page);
  });

  test('should handle 1K lines/sec log flood', async ({ page }) => {
    await selectPreset(page, 'LOG_1K');
    await startTest(page);

    // Wait for test to complete (5 seconds)
    await waitForTestCompletion(page, 10000);

    const metrics = await getMetrics(page);
    console.log('Log flood 1K metrics:', metrics);

    // Should have processed many frames
    expect(metrics.framesRendered).toBeGreaterThan(100);

    // At 1K lines/sec for 5 seconds, expect ~500+ batch frames
    expect(metrics.bytesGenerated).toBeGreaterThan(10000);

    // Frame time should be reasonable (allow slower for log processing)
    assertFrameRate(metrics, 30, 1.0); // More lenient for log flood
  });

  test('should handle 5K lines/sec log flood', async ({ page }) => {
    await selectPreset(page, 'LOG_5K');
    await startTest(page);

    // Wait for test to complete (5 seconds)
    await waitForTestCompletion(page, 10000);

    const metrics = await getMetrics(page);
    console.log('Log flood 5K metrics:', metrics);

    // Should handle high throughput
    expect(metrics.framesRendered).toBeGreaterThan(200);
    expect(metrics.bytesGenerated).toBeGreaterThan(50000);

    // May have slower frame times but should complete
    expect(metrics.avgFrameTime).toBeLessThan(100); // Max 100ms frame time
  });

  test('should handle 10K lines/sec log flood', async ({ page }) => {
    await selectPreset(page, 'LOG_10K');
    await startTest(page);

    // Wait for test to complete (3 seconds)
    await waitForTestCompletion(page, 8000);

    const metrics = await getMetrics(page);
    console.log('Log flood 10K metrics:', metrics);

    // At 10K/sec for 3 seconds = 30K lines expected
    // Batched at 100 lines per frame = 300 frames expected
    expect(metrics.framesRendered).toBeGreaterThan(100);
    expect(metrics.bytesGenerated).toBeGreaterThan(100000);

    // Under extreme load, frame times may spike but max should be bounded
    expect(metrics.maxFrameTime).toBeLessThan(500); // Max 500ms spike
  });

  test('should maintain scrollback integrity during log flood', async ({ page }) => {
    await selectPreset(page, 'LOG_1K');
    await startTest(page);

    // Let some logs accumulate
    await page.waitForTimeout(2000);

    // Stop and check terminal state
    await stopTest(page);

    // Terminal should have content (verify via screenshot size)
    const screenshot = await page.getByTestId('terminal-container').screenshot();
    expect(screenshot.length).toBeGreaterThan(5000);

    // Verify metrics show data was processed
    const metrics = await getMetrics(page);
    expect(metrics.bytesGenerated).toBeGreaterThan(0);
  });

  test('should not freeze browser during sustained log flood', async ({ page }) => {
    await selectPreset(page, 'LOG_5K');
    await startTest(page);

    // Periodically check that the page is responsive
    for (let i = 0; i < 5; i++) {
      await page.waitForTimeout(500);

      // Page should still be responsive - we can read metrics
      const metrics = await getMetrics(page);
      expect(metrics.framesRendered).toBeGreaterThan(0);

      // Status should still be running
      const status = await page.getByTestId('test-status').textContent();
      expect(status).toBe('running');
    }

    await stopTest(page);
  });

  test('should handle colored log output correctly', async ({ page }) => {
    // LOG presets include colors by default
    await selectPreset(page, 'LOG_1K');
    await startTest(page);

    await page.waitForTimeout(1500);

    // Take screenshot to verify colors are rendering
    const screenshot = await page.getByTestId('terminal-container').screenshot();

    await stopTest(page);

    // Screenshot with colors should be larger than plain text
    expect(screenshot.length).toBeGreaterThan(10000);

    const metrics = await getMetrics(page);
    console.log('Colored log metrics:', metrics);

    // Should process efficiently even with colors
    expect(metrics.avgFrameTime).toBeLessThan(100);
  });

  test('should complete all scheduled log batches', async ({ page }) => {
    await selectPreset(page, 'LOG_1K');

    const startTime = Date.now();
    await startTest(page);
    await waitForTestCompletion(page, 10000);
    const endTime = Date.now();

    const metrics = await getMetrics(page);
    const actualDuration = endTime - startTime;

    console.log(`Test completed in ${actualDuration}ms, expected ~5000ms`);
    console.log('Final metrics:', metrics);

    // Test should complete close to expected duration (within 50% variance)
    expect(actualDuration).toBeLessThan(7500); // 5s + 50% = 7.5s max
    expect(actualDuration).toBeGreaterThan(4000); // At least 4s
  });
});
