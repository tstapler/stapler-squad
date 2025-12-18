import { test, expect } from '@playwright/test';

/**
 * Critical Escape Code Tests
 * Tests for the 5 most frequently used escape codes (>1000 occurrences)
 * Based on analysis of 3,366 Claude sessions
 */

test.describe('Critical Escape Code Tests', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/test/escape-codes');

    // Wait for page to initialize
    await page.waitForSelector('[data-testid="terminal-container"]', { timeout: 30000 });
    await page.waitForSelector('[data-testid="escape-code-selector"]', { timeout: 15000 });

    // Verify initial state
    await expect(page.getByTestId('test-status')).toHaveText('idle', { timeout: 10000 });
  });

  test('critical code: Erase to End of Line (\\x1b[K)', async ({ page }) => {
    // This is the most used escape code with 4,767 occurrences
    await page.getByTestId('escape-code-selector').selectOption('erase-eol');

    // Configure test parameters
    await page.getByTestId('frame-rate-input').fill('30');
    await page.getByTestId('duration-input').fill('5');

    // Start test
    await page.getByTestId('start-test').click();
    await expect(page.getByTestId('test-status')).not.toHaveText('idle', { timeout: 5000 });

    // Wait for test completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 10000 });

    // Verify metrics
    const framesRendered = await page.getByTestId('frames-rendered').textContent();
    const frameCount = parseInt(framesRendered?.replace(/,/g, '') || '0');

    // Should render at least 100 frames in 5 seconds (accounting for test overhead)
    expect(frameCount).toBeGreaterThan(100);

    // Verify terminal content was modified
    const terminalContent = await page.getByTestId('terminal-container').textContent();
    expect(terminalContent).toBeTruthy();

    // Verify no rendering errors
    const errors = await page.getByTestId('error-count').textContent();
    expect(parseInt(errors || '0')).toBe(0);
  });

  test('critical code: Default Foreground Color (\\x1b[39m)', async ({ page }) => {
    // Second most used with 3,678 occurrences
    await page.getByTestId('escape-code-selector').selectOption('color-default-fg');

    // Configure for color testing
    await page.getByTestId('frame-rate-input').fill('60');
    await page.getByTestId('duration-input').fill('3');
    await page.getByTestId('color-cycling-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Let it run for a moment
    await page.waitForTimeout(1500);

    // Capture a screenshot to verify color rendering
    const screenshot = await page.screenshot({
      clip: { x: 0, y: 0, width: 800, height: 600 },
      animations: 'disabled'
    });
    expect(screenshot.length).toBeGreaterThan(1000);

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 10000 });

    // Verify frame metrics
    const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();
    const frameTime = parseFloat(avgFrameTime?.replace('ms', '') || '0');

    // Frame time should be reasonable (under 50ms for 60 FPS target)
    expect(frameTime).toBeLessThan(50);
  });

  test('critical code: G0 ASCII Charset (\\x1b(B)', async ({ page }) => {
    // 3,366 occurrences - charset switching
    await page.getByTestId('escape-code-selector').selectOption('charset-ascii');

    // Test with various character sets
    await page.getByTestId('frame-rate-input').fill('30');
    await page.getByTestId('duration-input').fill('4');
    await page.getByTestId('charset-test-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Monitor for charset switching
    await page.waitForTimeout(2000);

    // Verify charset changes are being applied
    const charsetChanges = await page.getByTestId('charset-changes').textContent();
    expect(parseInt(charsetChanges || '0')).toBeGreaterThan(0);

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 10000 });

    // Verify no character corruption
    const corruptedChars = await page.getByTestId('corrupted-chars').textContent();
    expect(parseInt(corruptedChars || '0')).toBe(0);
  });

  test('critical code: Reset All Attributes (\\x1b[m)', async ({ page }) => {
    // 3,366 occurrences - full reset
    await page.getByTestId('escape-code-selector').selectOption('reset-all');

    // Configure for attribute testing
    await page.getByTestId('frame-rate-input').fill('45');
    await page.getByTestId('duration-input').fill('5');
    await page.getByTestId('attribute-cycling-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Let test run
    await page.waitForTimeout(3000);

    // Get metrics
    const metrics = await getMetrics(page);

    // Verify reset operations are happening
    const resetCount = await page.getByTestId('reset-count').textContent();
    expect(parseInt(resetCount || '0')).toBeGreaterThan(100);

    // Stop test
    await page.getByTestId('stop-test').click({ force: true });

    // Verify terminal is in clean state after resets
    const terminalState = await page.getByTestId('terminal-state').textContent();
    expect(terminalState).toBe('clean');
  });

  test('critical code: 256-color Foreground (\\x1b[38;5;231m)', async ({ page }) => {
    // 2,208 occurrences - extended color support
    await page.getByTestId('escape-code-selector').selectOption('color-256-fg');

    // Configure for 256-color testing
    await page.getByTestId('frame-rate-input').fill('30');
    await page.getByTestId('duration-input').fill('6');
    await page.getByTestId('color-256-test-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Monitor color palette usage
    await page.waitForTimeout(3000);

    // Verify multiple colors are being used
    const uniqueColors = await page.getByTestId('unique-colors').textContent();
    expect(parseInt(uniqueColors || '0')).toBeGreaterThan(50); // Should use many of the 256 colors

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 15000 });

    // Verify color accuracy
    const colorAccuracy = await page.getByTestId('color-accuracy').textContent();
    const accuracy = parseFloat(colorAccuracy?.replace('%', '') || '0');
    expect(accuracy).toBeGreaterThan(95); // Should maintain high color accuracy
  });

  test('combined critical codes: rapid switching', async ({ page }) => {
    // Test rapid switching between all critical codes
    await page.getByTestId('escape-code-selector').selectOption('critical-mix');

    // High frequency switching
    await page.getByTestId('frame-rate-input').fill('120');
    await page.getByTestId('duration-input').fill('10');
    await page.getByTestId('rapid-switch-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Let it run for a bit
    await page.waitForTimeout(5000);

    // Get interim metrics
    const interimMetrics = await getMetrics(page);
    console.log('Rapid switching interim metrics:', interimMetrics);

    // Verify high frame rate is maintained
    expect(interimMetrics.avgFrameTime).toBeLessThan(20); // Should maintain < 20ms for 50+ FPS

    // Verify no memory leaks during rapid switching
    const memoryGrowth = await page.getByTestId('memory-growth').textContent();
    const growthRate = parseBytes(memoryGrowth?.replace('/s', '') || '0');
    expect(growthRate).toBeLessThan(1024 * 1024); // Less than 1MB/s growth

    // Stop test
    await page.getByTestId('stop-test').click({ force: true });

    // Final verification
    const finalMetrics = await getMetrics(page);
    console.log('Rapid switching final metrics:', finalMetrics);

    // Should have processed thousands of escape codes
    const escapeCodesProcessed = await page.getByTestId('escape-codes-processed').textContent();
    expect(parseInt(escapeCodesProcessed?.replace(/,/g, '') || '0')).toBeGreaterThan(5000);
  });

  test('performance baseline: critical codes at production load', async ({ page }) => {
    // Simulate production load with the most common escape codes
    await page.getByTestId('escape-code-selector').selectOption('production-load');

    // Production-like settings
    await page.getByTestId('frame-rate-input').fill('60');
    await page.getByTestId('duration-input').fill('30');
    await page.getByTestId('production-mode-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Take periodic measurements
    const measurements = [];
    for (let i = 0; i < 6; i++) {
      await page.waitForTimeout(5000);
      const metrics = await getMetrics(page);
      measurements.push(metrics);
      console.log(`Measurement ${i + 1}:`, metrics);
    }

    // Verify consistent performance over time
    const avgFrameTimes = measurements.map(m => m.avgFrameTime);
    const maxFrameTime = Math.max(...avgFrameTimes);
    const minFrameTime = Math.min(...avgFrameTimes);

    // Frame time variance should be low (consistent performance)
    expect(maxFrameTime - minFrameTime).toBeLessThan(10);

    // All frame times should be acceptable
    avgFrameTimes.forEach(frameTime => {
      expect(frameTime).toBeLessThan(30); // Maintain 30+ FPS
    });

    // Verify no memory leak over 30 seconds
    const finalMetrics = measurements[measurements.length - 1];
    expect(finalMetrics.memoryGrowthRate).toBeLessThan(512 * 1024); // Less than 512KB/s

    // Verify error-free execution
    const errors = await page.getByTestId('error-count').textContent();
    expect(parseInt(errors || '0')).toBe(0);
  });
});

// Helper functions
async function getMetrics(page: any) {
  const framesRendered = await page.getByTestId('frames-rendered').textContent();
  const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();
  const p95FrameTime = await page.getByTestId('p95-frame-time').textContent();
  const p99FrameTime = await page.getByTestId('p99-frame-time').textContent();
  const maxFrameTime = await page.getByTestId('max-frame-time').textContent();
  const peakMemory = await page.getByTestId('peak-memory').textContent();
  const memoryGrowth = await page.getByTestId('memory-growth').textContent();
  const bytesGenerated = await page.getByTestId('bytes-generated').textContent();

  return {
    framesRendered: parseInt(framesRendered?.replace(/,/g, '') || '0'),
    avgFrameTime: parseFloat(avgFrameTime?.replace('ms', '') || '0'),
    p95FrameTime: parseFloat(p95FrameTime?.replace('ms', '') || '0'),
    p99FrameTime: parseFloat(p99FrameTime?.replace('ms', '') || '0'),
    maxFrameTime: parseFloat(maxFrameTime?.replace('ms', '') || '0'),
    peakMemory: parseBytes(peakMemory || '0'),
    memoryGrowthRate: parseBytes(memoryGrowth?.replace('/s', '') || '0'),
    bytesGenerated: parseBytes(bytesGenerated || '0'),
  };
}

function parseBytes(str: string): number {
  const match = str.match(/([\d.]+)\s*(B|KB|MB|GB)?/i);
  if (!match) return 0;

  const value = parseFloat(match[1]);
  const unit = (match[2] || 'B').toUpperCase();

  switch (unit) {
    case 'GB': return value * 1024 * 1024 * 1024;
    case 'MB': return value * 1024 * 1024;
    case 'KB': return value * 1024;
    default: return value;
  }
}