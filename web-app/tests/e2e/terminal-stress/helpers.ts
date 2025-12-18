/**
 * Test helpers for terminal stress tests
 */

import { Page, expect } from '@playwright/test';

export interface StressTestMetrics {
  framesRendered: number;
  avgFrameTime: number;
  p95FrameTime: number;
  p99FrameTime: number;
  maxFrameTime: number;
  peakMemory: number;
  memoryGrowthRate: number;
  bytesGenerated: number;
  elapsed: number;
}

/**
 * Navigate to stress test page and wait for terminal to initialize
 */
export async function setupStressTestPage(page: Page): Promise<void> {
  await page.goto('/test/terminal-stress');
  // Wait for terminal container with longer timeout (parallel tests compete for resources)
  await page.waitForSelector('[data-testid="terminal-container"]', { timeout: 30000 });
  // Wait for start button to be ready
  await page.waitForSelector('[data-testid="start-test"]', { timeout: 15000 });

  // Verify page is ready - use waitFor for better reliability
  await expect(page.getByTestId('test-status')).toHaveText('idle', { timeout: 10000 });
}

/**
 * Get the current renderer type (webgl or canvas)
 */
export async function getRendererType(page: Page): Promise<'webgl' | 'canvas' | 'unknown'> {
  // Try from exposed window variable first (most reliable)
  const fromWindow = await page.evaluate(() => (window as any).stressTestRendererType);
  if (fromWindow) return fromWindow;

  // Fall back to reading from DOM
  const text = await page.getByTestId('renderer-type').textContent();
  return (text?.toLowerCase() as 'webgl' | 'canvas') || 'unknown';
}

/**
 * Assert that WebGL renderer is active
 */
export async function assertWebGLActive(page: Page): Promise<void> {
  const renderer = await getRendererType(page);
  expect(renderer).toBe('webgl');
}

/**
 * Select a test preset
 */
export async function selectPreset(page: Page, preset: string): Promise<void> {
  const selector = page.getByTestId('preset-selector');
  await selector.selectOption(preset);
}

/**
 * Start the stress test
 */
export async function startTest(page: Page): Promise<void> {
  await page.getByTestId('start-test').click();
  // Wait for test to start or complete (may complete very quickly for some tests)
  await expect(page.getByTestId('test-status')).not.toHaveText('idle', { timeout: 5000 });
}

/**
 * Stop the stress test
 * Note: Uses force: true because WebGL canvas updates cause element instability
 */
export async function stopTest(page: Page): Promise<void> {
  const stopButton = page.getByTestId('stop-test');
  const status = await page.getByTestId('test-status').textContent();

  // Only try to stop if still running
  if (status === 'running') {
    // Use force: true to click even during canvas animation
    await stopButton.click({ force: true });
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 5000 });
  }
}

/**
 * Wait for test to complete naturally
 */
export async function waitForTestCompletion(page: Page, timeoutMs: number = 60000): Promise<void> {
  await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: timeoutMs });
}

/**
 * Get current metrics from the page
 */
export async function getMetrics(page: Page): Promise<StressTestMetrics> {
  const framesRendered = await page.getByTestId('frames-rendered').textContent();
  const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();
  const p95FrameTime = await page.getByTestId('p95-frame-time').textContent();
  const p99FrameTime = await page.getByTestId('p99-frame-time').textContent();
  const maxFrameTime = await page.getByTestId('max-frame-time').textContent();
  const peakMemory = await page.getByTestId('peak-memory').textContent();
  const memoryGrowth = await page.getByTestId('memory-growth').textContent();
  const bytesGenerated = await page.getByTestId('bytes-generated').textContent();
  const elapsed = await page.getByTestId('progress-elapsed').textContent();

  return {
    framesRendered: parseInt(framesRendered?.replace(/,/g, '') || '0'),
    avgFrameTime: parseFloat(avgFrameTime?.replace('ms', '') || '0'),
    p95FrameTime: parseFloat(p95FrameTime?.replace('ms', '') || '0'),
    p99FrameTime: parseFloat(p99FrameTime?.replace('ms', '') || '0'),
    maxFrameTime: parseFloat(maxFrameTime?.replace('ms', '') || '0'),
    peakMemory: parseBytes(peakMemory || '0'),
    memoryGrowthRate: parseBytes(memoryGrowth?.replace('/s', '') || '0'),
    bytesGenerated: parseBytes(bytesGenerated || '0'),
    elapsed: parseTime(elapsed || '0'),
  };
}

/**
 * Parse bytes from formatted string (e.g., "1.5 MB" -> 1572864)
 */
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

/**
 * Parse time from formatted string (e.g., "5.2s" -> 5200)
 */
function parseTime(str: string): number {
  const match = str.match(/([\d.]+)(ms|s)?/i);
  if (!match) return 0;

  const value = parseFloat(match[1]);
  const unit = (match[2] || 'ms').toLowerCase();

  return unit === 's' ? value * 1000 : value;
}

/**
 * Take screenshots at regular intervals during test
 * Note: Uses page.screenshot() instead of element.screenshot() because
 * xterm.js with WebGL causes continuous canvas updates that prevent
 * element stability. Page screenshots don't have this limitation.
 */
export async function captureTestScreenshots(
  page: Page,
  count: number,
  intervalMs: number
): Promise<Buffer[]> {
  const screenshots: Buffer[] = [];

  for (let i = 0; i < count; i++) {
    // Use page screenshot - element screenshots wait for stability which
    // never occurs with continuously updating WebGL canvas
    // Set timeout to avoid hanging on font loading
    const screenshot = await page.screenshot({
      animations: 'disabled',
      timeout: 5000,
    });
    screenshots.push(screenshot);
    await page.waitForTimeout(intervalMs);
  }

  return screenshots;
}

/**
 * Assert frame rate is within acceptable bounds
 */
export function assertFrameRate(metrics: StressTestMetrics, targetFps: number, tolerance: number = 0.3): void {
  const targetFrameTime = 1000 / targetFps;
  const maxAcceptableFrameTime = targetFrameTime * (1 + tolerance);

  expect(metrics.avgFrameTime).toBeLessThan(maxAcceptableFrameTime);
  expect(metrics.p95FrameTime).toBeLessThan(maxAcceptableFrameTime * 1.5);
}

/**
 * Assert no memory leaks (growth rate under threshold)
 */
export function assertNoMemoryLeak(metrics: StressTestMetrics, maxGrowthBytesPerSec: number = 1024 * 1024): void {
  expect(metrics.memoryGrowthRate).toBeLessThan(maxGrowthBytesPerSec);
}

/**
 * Assert minimum frame count achieved
 */
export function assertFrameCount(metrics: StressTestMetrics, minFrames: number): void {
  expect(metrics.framesRendered).toBeGreaterThanOrEqual(minFrames);
}

/**
 * Get memory snapshots at intervals (for Chrome DevTools Protocol)
 */
export async function collectMemorySnapshots(
  page: Page,
  durationMs: number,
  intervalMs: number
): Promise<number[]> {
  const snapshots: number[] = [];
  const startTime = Date.now();

  while (Date.now() - startTime < durationMs) {
    const memory = await page.evaluate(() => {
      return (performance as any).memory?.usedJSHeapSize || 0;
    });
    snapshots.push(memory);
    await page.waitForTimeout(intervalMs);
  }

  return snapshots;
}

/**
 * Analyze memory trend for leak detection
 */
export function analyzeMemoryTrend(snapshots: number[]): {
  startMemory: number;
  endMemory: number;
  peakMemory: number;
  growthRate: number;
  isLeak: boolean;
} {
  if (snapshots.length < 2) {
    return {
      startMemory: snapshots[0] || 0,
      endMemory: snapshots[0] || 0,
      peakMemory: snapshots[0] || 0,
      growthRate: 0,
      isLeak: false,
    };
  }

  const startMemory = snapshots[0];
  const endMemory = snapshots[snapshots.length - 1];
  const peakMemory = Math.max(...snapshots);
  const totalGrowth = endMemory - startMemory;
  const durationSec = snapshots.length * 0.5; // Assuming 500ms intervals
  const growthRate = totalGrowth / durationSec;

  // Consider it a leak if memory consistently grows more than 1MB/sec
  const isLeak = growthRate > 1024 * 1024;

  return {
    startMemory,
    endMemory,
    peakMemory,
    growthRate,
    isLeak,
  };
}
