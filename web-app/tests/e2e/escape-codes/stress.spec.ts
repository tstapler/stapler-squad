import { test, expect, Page } from '@playwright/test';

/**
 * Escape Code Stress Tests
 * Performance and stress testing for escape code handling
 * Tests high frame rates, memory leaks, and rapid code switching
 */

test.describe('Escape Code Stress Tests', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/test/escape-codes');

    // Wait for initialization
    await page.waitForSelector('[data-testid="terminal-container"]', { timeout: 30000 });
    await page.waitForSelector('[data-testid="stress-mode-selector"]', { timeout: 15000 });

    // Verify initial state
    await expect(page.getByTestId('test-status')).toHaveText('idle', { timeout: 10000 });

    // Enable performance monitoring
    await page.getByTestId('enable-perf-monitoring').check();
  });

  test('stress test: 120 FPS for 10 seconds', async ({ page }) => {
    test.setTimeout(60000); // Increase timeout for stress test

    // Configure for high frame rate
    await page.getByTestId('stress-mode-selector').selectOption('high-fps');
    await page.getByTestId('frame-rate-input').fill('120');
    await page.getByTestId('duration-input').fill('10');

    // Use critical escape codes for realistic load
    await page.getByTestId('escape-code-selector').selectOption('critical-mix');

    // Start test
    await page.getByTestId('start-test').click();

    // Take performance snapshots every 2 seconds
    const performanceSnapshots = [];
    for (let i = 0; i < 5; i++) {
      await page.waitForTimeout(2000);

      const snapshot = await capturePerformanceSnapshot(page);
      performanceSnapshots.push(snapshot);
      console.log(`Snapshot ${i + 1}:`, snapshot);
    }

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 15000 });

    // Get final metrics
    const finalMetrics = await getDetailedMetrics(page);
    console.log('Final metrics (120 FPS):', finalMetrics);

    // Performance assertions
    expect(finalMetrics.framesRendered).toBeGreaterThan(800); // Should achieve high frame count
    expect(finalMetrics.avgFrameTime).toBeLessThan(15); // < 15ms for 60+ FPS effective
    expect(finalMetrics.p95FrameTime).toBeLessThan(25); // P95 under 25ms
    expect(finalMetrics.droppedFrames).toBeLessThan(50); // Less than 50 dropped frames

    // Verify performance didn't degrade over time
    const firstSnapshot = performanceSnapshots[0];
    const lastSnapshot = performanceSnapshots[performanceSnapshots.length - 1];
    const degradation = lastSnapshot.avgFrameTime - firstSnapshot.avgFrameTime;

    expect(degradation).toBeLessThan(5); // Frame time shouldn't increase by more than 5ms
  });

  test('memory leak detection: 30 seconds continuous operation', async ({ page }) => {
    test.setTimeout(90000); // Extended timeout for long test

    // Configure for memory leak detection
    await page.getByTestId('stress-mode-selector').selectOption('memory-leak-detection');
    await page.getByTestId('frame-rate-input').fill('60');
    await page.getByTestId('duration-input').fill('30');

    // Use comprehensive escape codes to stress memory
    await page.getByTestId('escape-code-selector').selectOption('all-codes');
    await page.getByTestId('memory-stress-checkbox').check();

    // Collect baseline memory
    const baselineMemory = await collectMemoryMetrics(page);
    console.log('Baseline memory:', formatBytes(baselineMemory));

    // Start test
    await page.getByTestId('start-test').click();

    // Collect memory snapshots every 5 seconds
    const memorySnapshots = [];
    for (let i = 0; i < 6; i++) {
      await page.waitForTimeout(5000);

      const memory = await collectMemoryMetrics(page);
      memorySnapshots.push(memory);
      console.log(`Memory at ${(i + 1) * 5}s:`, formatBytes(memory));
    }

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 35000 });

    // Analyze memory trend
    const memoryTrend = analyzeMemoryTrend(memorySnapshots);
    console.log('Memory analysis:', memoryTrend);

    // Memory leak assertions
    expect(memoryTrend.isLeak).toBe(false);
    expect(memoryTrend.growthRate).toBeLessThan(1024 * 1024); // Less than 1MB/s growth

    // Peak memory should be reasonable
    expect(memoryTrend.peakMemory).toBeLessThan(baselineMemory + 50 * 1024 * 1024); // Less than 50MB growth

    // Verify garbage collection is working
    const gcEvents = await page.getByTestId('gc-events').textContent();
    expect(parseInt(gcEvents || '0')).toBeGreaterThan(0); // GC should have run
  });

  test('rapid code switching: 1000 switches per second', async ({ page }) => {
    test.setTimeout(60000);

    // Configure for rapid switching
    await page.getByTestId('stress-mode-selector').selectOption('rapid-switching');
    await page.getByTestId('switch-rate-input').fill('1000');
    await page.getByTestId('duration-input').fill('10');

    // Enable all escape code categories for maximum variety
    await page.getByTestId('all-categories-checkbox').check();
    await page.getByTestId('random-switching-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Monitor switching performance
    await page.waitForTimeout(5000);

    // Get mid-test metrics
    const midMetrics = await getDetailedMetrics(page);
    console.log('Mid-test metrics (rapid switching):', midMetrics);

    // Verify switches are happening at expected rate
    const switchCount = await page.getByTestId('switch-count').textContent();
    expect(parseInt(switchCount?.replace(/,/g, '') || '0')).toBeGreaterThan(4000); // 4000+ switches in 5 seconds

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 15000 });

    // Final metrics
    const finalMetrics = await getDetailedMetrics(page);
    console.log('Final metrics (rapid switching):', finalMetrics);

    // Performance should remain stable despite rapid switching
    expect(finalMetrics.avgFrameTime).toBeLessThan(30); // Maintain 30+ FPS
    expect(finalMetrics.errorCount).toBe(0); // No errors during switching

    // Verify total switches
    const totalSwitches = await page.getByTestId('total-switches').textContent();
    expect(parseInt(totalSwitches?.replace(/,/g, '') || '0')).toBeGreaterThan(8000); // 8000+ total switches
  });

  test('buffer overflow protection: massive data burst', async ({ page }) => {
    // Configure for buffer overflow testing
    await page.getByTestId('stress-mode-selector').selectOption('buffer-overflow');
    await page.getByTestId('burst-size-input').fill('10000'); // 10K codes per burst
    await page.getByTestId('burst-count-input').fill('10');

    // Start test
    await page.getByTestId('start-test').click();

    // Monitor for buffer issues
    await page.waitForTimeout(5000);

    // Check for buffer overflows
    const bufferOverflows = await page.getByTestId('buffer-overflows').textContent();
    expect(parseInt(bufferOverflows || '0')).toBe(0); // No buffer overflows

    // Check for data corruption
    const dataCorruption = await page.getByTestId('data-corruption').textContent();
    expect(parseInt(dataCorruption || '0')).toBe(0); // No data corruption

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 30000 });

    // Verify all data was processed
    const processedCodes = await page.getByTestId('escape-codes-processed').textContent();
    expect(parseInt(processedCodes?.replace(/,/g, '') || '0')).toBeGreaterThanOrEqual(100000); // All 100K codes processed
  });

  test('concurrent streams: multiple terminals with different escape codes', async ({ page }) => {
    test.setTimeout(60000);

    // Configure for concurrent terminal testing
    await page.getByTestId('stress-mode-selector').selectOption('concurrent-terminals');
    await page.getByTestId('terminal-count-input').fill('4'); // 4 concurrent terminals
    await page.getByTestId('duration-input').fill('15');

    // Each terminal uses different escape code sets
    await page.getByTestId('terminal-1-codes').selectOption('cursor-movement');
    await page.getByTestId('terminal-2-codes').selectOption('colors-graphics');
    await page.getByTestId('terminal-3-codes').selectOption('text-attributes');
    await page.getByTestId('terminal-4-codes').selectOption('screen-control');

    // Start test
    await page.getByTestId('start-test').click();

    // Monitor each terminal
    await page.waitForTimeout(5000);

    // Verify all terminals are active
    for (let i = 1; i <= 4; i++) {
      const terminalActive = await page.getByTestId(`terminal-${i}-active`).textContent();
      expect(terminalActive).toBe('true');

      const terminalFrames = await page.getByTestId(`terminal-${i}-frames`).textContent();
      expect(parseInt(terminalFrames || '0')).toBeGreaterThan(100);
    }

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 20000 });

    // Verify no cross-terminal interference
    const interferenceDetected = await page.getByTestId('interference-detected').textContent();
    expect(interferenceDetected).toBe('false');

    // Total performance across all terminals
    const totalFrames = await page.getByTestId('total-frames-all-terminals').textContent();
    expect(parseInt(totalFrames?.replace(/,/g, '') || '0')).toBeGreaterThan(2000);
  });

  test('recovery test: error injection and recovery', async ({ page }) => {
    // Configure for error recovery testing
    await page.getByTestId('stress-mode-selector').selectOption('error-recovery');
    await page.getByTestId('error-injection-rate').fill('5'); // 5% error rate
    await page.getByTestId('duration-input').fill('20');

    // Enable recovery mechanisms
    await page.getByTestId('auto-recovery-checkbox').check();
    await page.getByTestId('fallback-mode-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Monitor recovery events
    await page.waitForTimeout(10000);

    // Check recovery metrics
    const errorsInjected = await page.getByTestId('errors-injected').textContent();
    const errorsRecovered = await page.getByTestId('errors-recovered').textContent();
    const errorsFailed = await page.getByTestId('errors-failed').textContent();

    const injected = parseInt(errorsInjected || '0');
    const recovered = parseInt(errorsRecovered || '0');
    const failed = parseInt(errorsFailed || '0');

    // Most errors should be recovered
    expect(recovered).toBeGreaterThan(injected * 0.95); // 95%+ recovery rate
    expect(failed).toBeLessThan(injected * 0.05); // Less than 5% failure rate

    // Wait for completion
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 25000 });

    // Terminal should still be functional after errors
    const terminalFunctional = await page.getByTestId('terminal-functional').textContent();
    expect(terminalFunctional).toBe('true');
  });

  test('performance degradation: long-running stability test', async ({ page }) => {
    test.setTimeout(120000); // 2 minute timeout

    // Configure for long-running test
    await page.getByTestId('stress-mode-selector').selectOption('long-running');
    await page.getByTestId('frame-rate-input').fill('60');
    await page.getByTestId('duration-input').fill('60'); // 1 minute

    // Use realistic production load
    await page.getByTestId('escape-code-selector').selectOption('production-load');
    await page.getByTestId('production-mode-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Collect performance metrics every 10 seconds
    const performanceOverTime = [];
    for (let i = 0; i < 6; i++) {
      await page.waitForTimeout(10000);

      const metrics = await getDetailedMetrics(page);
      performanceOverTime.push({
        time: (i + 1) * 10,
        ...metrics
      });
      console.log(`Performance at ${(i + 1) * 10}s:`, metrics);
    }

    // Analyze performance degradation
    const firstMinuteAvg = performanceOverTime[0].avgFrameTime;
    const lastMinuteAvg = performanceOverTime[performanceOverTime.length - 1].avgFrameTime;
    const degradationPercent = ((lastMinuteAvg - firstMinuteAvg) / firstMinuteAvg) * 100;

    console.log(`Performance degradation: ${degradationPercent.toFixed(2)}%`);

    // Performance should not degrade significantly
    expect(degradationPercent).toBeLessThan(20); // Less than 20% degradation

    // Frame time should remain acceptable throughout
    performanceOverTime.forEach(metric => {
      expect(metric.avgFrameTime).toBeLessThan(50); // Maintain 20+ FPS minimum
    });

    // No memory leaks over extended period
    const memoryGrowth = performanceOverTime[performanceOverTime.length - 1].memoryUsed - performanceOverTime[0].memoryUsed;
    expect(memoryGrowth).toBeLessThan(100 * 1024 * 1024); // Less than 100MB growth
  });
});

// Helper functions
async function capturePerformanceSnapshot(page: Page) {
  const framesRendered = await page.getByTestId('frames-rendered').textContent();
  const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();
  const p95FrameTime = await page.getByTestId('p95-frame-time').textContent();
  const cpuUsage = await page.getByTestId('cpu-usage').textContent();
  const memoryUsed = await page.getByTestId('memory-used').textContent();

  return {
    framesRendered: parseInt(framesRendered?.replace(/,/g, '') || '0'),
    avgFrameTime: parseFloat(avgFrameTime?.replace('ms', '') || '0'),
    p95FrameTime: parseFloat(p95FrameTime?.replace('ms', '') || '0'),
    cpuUsage: parseFloat(cpuUsage?.replace('%', '') || '0'),
    memoryUsed: parseBytes(memoryUsed || '0'),
  };
}

async function getDetailedMetrics(page: Page) {
  const snapshot = await capturePerformanceSnapshot(page);

  const p99FrameTime = await page.getByTestId('p99-frame-time').textContent();
  const maxFrameTime = await page.getByTestId('max-frame-time').textContent();
  const droppedFrames = await page.getByTestId('dropped-frames').textContent();
  const errorCount = await page.getByTestId('error-count').textContent();

  return {
    ...snapshot,
    p99FrameTime: parseFloat(p99FrameTime?.replace('ms', '') || '0'),
    maxFrameTime: parseFloat(maxFrameTime?.replace('ms', '') || '0'),
    droppedFrames: parseInt(droppedFrames?.replace(/,/g, '') || '0'),
    errorCount: parseInt(errorCount || '0'),
  };
}

async function collectMemoryMetrics(page: Page): Promise<number> {
  const memory = await page.evaluate(() => {
    return (performance as any).memory?.usedJSHeapSize || 0;
  });
  return memory;
}

function analyzeMemoryTrend(snapshots: number[]): {
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
  const durationSec = snapshots.length * 5; // 5 second intervals
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

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}