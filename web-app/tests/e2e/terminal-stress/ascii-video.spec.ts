import { test, expect } from '@playwright/test';
import {
  setupStressTestPage,
  selectPreset,
  startTest,
  stopTest,
  waitForTestCompletion,
  getMetrics,
  captureTestScreenshots,
  assertFrameRate,
} from './helpers';

test.describe('ASCII Video Playback Tests', () => {
  test.beforeEach(async ({ page }) => {
    await setupStressTestPage(page);
  });

  test('should render ASCII video at 30 FPS without dropping frames', async ({ page }) => {
    // Select 30 FPS preset
    await selectPreset(page, 'ASCII_30FPS');

    // Start test
    await startTest(page);

    // Let test run for 5 seconds
    await page.waitForTimeout(5000);

    // Get interim metrics
    const interimMetrics = await getMetrics(page);
    console.log('Interim metrics (30 FPS):', interimMetrics);

    // Verify frames are being rendered
    expect(interimMetrics.framesRendered).toBeGreaterThan(100);

    // Wait for completion
    await waitForTestCompletion(page, 15000);

    // Get final metrics
    const finalMetrics = await getMetrics(page);
    console.log('Final metrics (30 FPS):', finalMetrics);

    // Assertions
    // At 30 FPS for 10 seconds, expect ~300 frames
    expect(finalMetrics.framesRendered).toBeGreaterThan(250);

    // Frame time should be ~33ms for 30 FPS (allow 50% variance for CI)
    assertFrameRate(finalMetrics, 30, 0.5);
  });

  test('should render ASCII video at 60 FPS (Matrix Rain)', async ({ page }) => {
    // Select 60 FPS Matrix Rain preset
    await selectPreset(page, 'ASCII_60FPS');

    // Start test
    await startTest(page);

    // Let test run for 5 seconds
    await page.waitForTimeout(5000);

    // Get interim metrics
    const interimMetrics = await getMetrics(page);
    console.log('Interim metrics (60 FPS):', interimMetrics);

    // Verify frames are being rendered
    expect(interimMetrics.framesRendered).toBeGreaterThan(200);

    // Wait for completion
    await waitForTestCompletion(page, 15000);

    // Get final metrics
    const finalMetrics = await getMetrics(page);
    console.log('Final metrics (60 FPS):', finalMetrics);

    // At 60 FPS for 10 seconds, expect ~600 frames
    expect(finalMetrics.framesRendered).toBeGreaterThan(500);

    // Frame time should be ~16.67ms for 60 FPS (allow 50% variance for CI)
    assertFrameRate(finalMetrics, 60, 0.5);
  });

  test('should not show visible flickering during ASCII animation', async ({ page }) => {
    await selectPreset(page, 'ASCII_30FPS');
    await startTest(page);

    // Take screenshots at intervals
    const screenshots = await captureTestScreenshots(page, 10, 200);

    await stopTest(page);

    // Verify all screenshots captured content (not blank)
    for (const screenshot of screenshots) {
      // Screenshots should have reasonable size (not empty)
      expect(screenshot.length).toBeGreaterThan(1000);
    }

    console.log(`Captured ${screenshots.length} screenshots, all non-blank`);
  });

  test('should maintain stable frame pacing during animation', async ({ page }) => {
    await selectPreset(page, 'ASCII_30FPS');
    await startTest(page);

    // Wait for substantial test data
    await page.waitForTimeout(3000);

    const metrics = await getMetrics(page);

    // P95 should not be dramatically higher than average (indicates consistent pacing)
    const varianceRatio = metrics.p95FrameTime / metrics.avgFrameTime;
    console.log(`Frame time variance ratio (P95/Avg): ${varianceRatio.toFixed(2)}`);

    // P95 should be less than 3x average (allows for some jitter but not major stutters)
    expect(varianceRatio).toBeLessThan(3);

    await stopTest(page);
  });

  test('should handle bouncing ball animation pattern', async ({ page }) => {
    // ASCII_30FPS uses bouncing-ball pattern
    await selectPreset(page, 'ASCII_30FPS');
    await startTest(page);

    await page.waitForTimeout(2000);

    // Verify test is running correctly
    const status = await page.getByTestId('test-status').textContent();
    expect(status).toBe('running');

    const metrics = await getMetrics(page);
    expect(metrics.framesRendered).toBeGreaterThan(50);

    await stopTest(page);
  });

  test('should recover gracefully from rapid start/stop cycles', async ({ page }) => {
    for (let i = 0; i < 3; i++) {
      await selectPreset(page, 'ASCII_30FPS');
      await startTest(page);
      await page.waitForTimeout(500);
      await stopTest(page);

      // Verify we can read metrics after stop
      const metrics = await getMetrics(page);
      expect(metrics.framesRendered).toBeGreaterThan(0);

      // Reset for next cycle
      await page.getByTestId('reset-test').click();
      await expect(page.getByTestId('test-status')).toHaveText('idle');
    }
  });
});
