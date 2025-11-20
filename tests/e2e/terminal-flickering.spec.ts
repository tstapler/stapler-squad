import { test, expect } from '@playwright/test';

/**
 * Terminal Flickering Test
 *
 * Verifies that the StateApplicator's incremental rendering and RAF batching
 * eliminate flickering during rapid ANSI updates (like Claude's pulsing status bar).
 *
 * Test strategy:
 * 1. Load test page with simulated rapid updates
 * 2. Start test (50 updates/sec batched to 60fps)
 * 3. Verify frame timing stays within 60fps target (~16.67ms)
 * 4. Verify only changed lines are updated (not full screen clears)
 * 5. Take screenshots to verify no visible flickering
 */

test.describe('Terminal Flickering Fix', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to test page (using production server at 8543)
    await page.goto('http://localhost:8543/test-terminal');

    // Wait for terminal to initialize
    await page.waitForSelector('[data-testid="terminal-container"]', { timeout: 10000 });
  });

  test('should render test page correctly', async ({ page }) => {
    // Verify page elements exist
    await expect(page.getByRole('heading', { name: 'Terminal Flickering Test' })).toBeVisible();
    await expect(page.getByTestId('start-test')).toBeVisible();
    await expect(page.getByTestId('stop-test')).toBeVisible();
    await expect(page.getByTestId('reset-test')).toBeVisible();
    await expect(page.getByTestId('metrics-display')).toBeVisible();
    await expect(page.getByTestId('terminal-container')).toBeVisible();
  });

  test('should maintain 60fps with incremental rendering', async ({ page }) => {
    // Start test
    await page.getByTestId('start-test').click();

    // Wait for test to run (let it accumulate some frames)
    await page.waitForTimeout(2000); // 2 seconds

    // Get metrics
    const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();
    const framesRendered = await page.getByTestId('frames-rendered').textContent();
    const changedLines = await page.getByTestId('changed-lines').textContent();
    const unchangedLines = await page.getByTestId('unchanged-lines').textContent();

    // Parse metrics
    const avgFrameTimeMs = parseFloat(avgFrameTime?.replace('ms', '') || '0');
    const framesCount = parseInt(framesRendered || '0');

    console.log('Performance Metrics:');
    console.log(`  Avg Frame Time: ${avgFrameTimeMs}ms`);
    console.log(`  Frames Rendered: ${framesCount}`);
    console.log(`  Changed Lines: ${changedLines}`);
    console.log(`  Unchanged Lines: ${unchangedLines}`);

    // Assertions

    // 1. Frame time should be within 60fps target (allow some variance for CI)
    // Target: 16.67ms (60fps), Max acceptable: 40ms (25fps)
    expect(avgFrameTimeMs).toBeLessThan(40);
    expect(avgFrameTimeMs).toBeGreaterThan(0); // Sanity check

    // 2. Should have rendered multiple frames
    expect(framesCount).toBeGreaterThan(50); // At least 1 second worth at 50 updates/sec

    // 3. Only 1 line should change per update (status bar)
    expect(changedLines).toBe('1');

    // 4. 23 lines should remain unchanged
    expect(unchangedLines).toBe('23');

    // Stop test
    await page.getByTestId('stop-test').click();
  });

  test('should not show visible flickering during rapid updates', async ({ page }) => {
    // Start test
    await page.getByTestId('start-test').click();

    // Take screenshots during rapid updates to check for blank frames
    const screenshots: Buffer[] = [];

    for (let i = 0; i < 5; i++) {
      await page.waitForTimeout(200); // Space screenshots out
      const screenshot = await page.getByTestId('terminal-container').screenshot();
      screenshots.push(screenshot);
    }

    // Stop test
    await page.getByTestId('stop-test').click();

    // Verify screenshots - none should be completely blank
    // (A blank frame would indicate the terminal.clear() is causing visible flicker)
    for (const screenshot of screenshots) {
      // Screenshots should be reasonably sized (not empty)
      expect(screenshot.length).toBeGreaterThan(1000); // Arbitrary threshold
    }

    // Note: More sophisticated flicker detection would compare pixel differences
    // between consecutive frames, but this basic check catches major issues
  });

  test('should batch rapid updates with RAF', async ({ page }) => {
    // Start test
    await page.getByTestId('start-test').click();

    // Wait for first frame to ensure test is fully started
    await page.waitForTimeout(100);

    // Let test run for longer to reduce timing variance (2 seconds instead of 1)
    await page.waitForTimeout(2000);

    // Get frame count
    const framesRendered = await page.getByTestId('frames-rendered').textContent();
    const framesCount = parseInt(framesRendered || '0');

    // Test sends 50 updates/sec, RAF batches to ~50-60fps (browser dependent)
    // In 2 seconds we expect:
    //   - Minimum: 80 frames (40fps * 2sec - allows for startup delay)
    //   - Maximum: 140 frames (70fps * 2sec - allows for high refresh rate displays)
    //
    // More lenient threshold reduces flakiness from timing variance
    expect(framesCount).toBeGreaterThan(80); // Reasonable minimum
    expect(framesCount).toBeLessThan(140); // Not rendering excessively

    // Verify we're getting consistent frame pacing
    const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();
    const avgFrameTimeMs = parseFloat(avgFrameTime?.replace('ms', '') || '0');
    expect(avgFrameTimeMs).toBeLessThan(50); // Should be well under 20fps threshold

    await page.getByTestId('stop-test').click();
  });

  test('should reset state correctly', async ({ page }) => {
    // Start test
    await page.getByTestId('start-test').click();

    // Wait a bit
    await page.waitForTimeout(500);

    // Stop test
    await page.getByTestId('stop-test').click();

    // Verify we have metrics
    const framesBeforeReset = await page.getByTestId('frames-rendered').textContent();
    expect(parseInt(framesBeforeReset || '0')).toBeGreaterThan(0);

    // Reset
    await page.getByTestId('reset-test').click();

    // Verify metrics are reset
    await expect(page.getByTestId('frames-rendered')).toHaveText('0');
    await expect(page.getByTestId('avg-frame-time')).toHaveText('0.00ms');
    await expect(page.getByTestId('total-updates')).toHaveText('0');
    await expect(page.getByTestId('test-status')).toHaveText('Ready');
  });

  test('should expose test API for external verification', async ({ page }) => {
    // Verify test utilities are exposed on window
    const hasTestTerminal = await page.evaluate(() => {
      return typeof (window as any).testTerminal !== 'undefined';
    });

    const hasTestStateApplicator = await page.evaluate(() => {
      return typeof (window as any).testStateApplicator !== 'undefined';
    });

    const hasTestMetrics = await page.evaluate(() => {
      return typeof (window as any).testMetrics !== 'undefined';
    });

    expect(hasTestTerminal).toBe(true);
    expect(hasTestStateApplicator).toBe(true);
    expect(hasTestMetrics).toBe(true);
  });

  test('should complete full test cycle automatically', async ({ page }) => {
    // Start test
    await page.getByTestId('start-test').click();

    // Test should auto-complete after 300 updates (~5 seconds at 60fps)
    // Wait up to 10 seconds for completion
    await expect(page.getByTestId('test-status')).toHaveText('Completed', { timeout: 10000 });

    // Verify final metrics
    const framesRendered = await page.getByTestId('frames-rendered').textContent();
    const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();

    console.log('Final Metrics:');
    console.log(`  Frames Rendered: ${framesRendered}`);
    console.log(`  Avg Frame Time: ${avgFrameTime}`);

    // Should have rendered close to 300 frames
    const framesCount = parseInt(framesRendered || '0');
    expect(framesCount).toBeGreaterThanOrEqual(290); // Allow small variance
    expect(framesCount).toBeLessThanOrEqual(310);

    // Frame time should still be good
    const avgFrameTimeMs = parseFloat(avgFrameTime?.replace('ms', '') || '0');
    expect(avgFrameTimeMs).toBeLessThan(40); // Within 60fps target
  });
});
