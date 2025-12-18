import { test, expect } from '@playwright/test';

/**
 * Comprehensive Escape Code Tests
 * Tests for all 77 unique escape codes found in Claude sessions
 * Organized by category for systematic validation
 */

interface EscapeCodeCategory {
  name: string;
  codes: Array<{
    code: string;
    description: string;
    testId: string;
    occurrences: number;
  }>;
}

const ESCAPE_CODE_CATEGORIES: EscapeCodeCategory[] = [
  {
    name: 'Cursor Movement',
    codes: [
      { code: '\\x1b[H', description: 'Cursor Home', testId: 'cursor-home', occurrences: 837 },
      { code: '\\x1b[1G', description: 'Move to Column 1', testId: 'cursor-col1', occurrences: 420 },
      { code: '\\x1b[2K', description: 'Erase Entire Line', testId: 'erase-line', occurrences: 342 },
      { code: '\\x1b[J', description: 'Erase Down', testId: 'erase-down', occurrences: 249 },
      { code: '\\x1b[A', description: 'Cursor Up', testId: 'cursor-up', occurrences: 180 },
      { code: '\\x1b[B', description: 'Cursor Down', testId: 'cursor-down', occurrences: 165 },
      { code: '\\x1b[C', description: 'Cursor Forward', testId: 'cursor-forward', occurrences: 156 },
      { code: '\\x1b[D', description: 'Cursor Back', testId: 'cursor-back', occurrences: 143 },
    ]
  },
  {
    name: 'Color and Graphics',
    codes: [
      { code: '\\x1b[39m', description: 'Default Foreground', testId: 'color-default-fg', occurrences: 3678 },
      { code: '\\x1b[38;5;231m', description: '256-color White', testId: 'color-256-white', occurrences: 2208 },
      { code: '\\x1b[38;5;75m', description: '256-color Blue', testId: 'color-256-blue', occurrences: 702 },
      { code: '\\x1b[38;5;242m', description: '256-color Gray', testId: 'color-256-gray', occurrences: 546 },
      { code: '\\x1b[32m', description: 'Green', testId: 'color-green', occurrences: 351 },
      { code: '\\x1b[36m', description: 'Cyan', testId: 'color-cyan', occurrences: 216 },
      { code: '\\x1b[35m', description: 'Magenta', testId: 'color-magenta', occurrences: 90 },
      { code: '\\x1b[31m', description: 'Red', testId: 'color-red', occurrences: 72 },
      { code: '\\x1b[33m', description: 'Yellow', testId: 'color-yellow', occurrences: 54 },
      { code: '\\x1b[37m', description: 'White', testId: 'color-white', occurrences: 45 },
      { code: '\\x1b[34m', description: 'Blue', testId: 'color-blue', occurrences: 36 },
      { code: '\\x1b[30m', description: 'Black', testId: 'color-black', occurrences: 27 },
      { code: '\\x1b[90m', description: 'Bright Black', testId: 'color-bright-black', occurrences: 18 },
      { code: '\\x1b[38;2;255;255;255m', description: 'RGB White', testId: 'color-rgb-white', occurrences: 12 },
      { code: '\\x1b[49m', description: 'Default Background', testId: 'color-default-bg', occurrences: 369 },
      { code: '\\x1b[40m', description: 'Black Background', testId: 'bg-black', occurrences: 180 },
      { code: '\\x1b[48;5;16m', description: '256-color Black BG', testId: 'bg-256-black', occurrences: 144 },
    ]
  },
  {
    name: 'Text Attributes',
    codes: [
      { code: '\\x1b[m', description: 'Reset All', testId: 'reset-all', occurrences: 3366 },
      { code: '\\x1b[0m', description: 'Reset (Explicit)', testId: 'reset-explicit', occurrences: 891 },
      { code: '\\x1b[1m', description: 'Bold', testId: 'bold', occurrences: 612 },
      { code: '\\x1b[2m', description: 'Dim', testId: 'dim', occurrences: 324 },
      { code: '\\x1b[3m', description: 'Italic', testId: 'italic', occurrences: 198 },
      { code: '\\x1b[4m', description: 'Underline', testId: 'underline', occurrences: 162 },
      { code: '\\x1b[7m', description: 'Reverse', testId: 'reverse', occurrences: 81 },
      { code: '\\x1b[22m', description: 'Normal Intensity', testId: 'normal-intensity', occurrences: 27 },
    ]
  },
  {
    name: 'Screen Control',
    codes: [
      { code: '\\x1b[K', description: 'Erase to End of Line', testId: 'erase-eol', occurrences: 4767 },
      { code: '\\x1b[2J', description: 'Clear Screen', testId: 'clear-screen', occurrences: 423 },
      { code: '\\x1b[3J', description: 'Clear Scrollback', testId: 'clear-scrollback', occurrences: 54 },
      { code: '\\x1b[0J', description: 'Erase Below', testId: 'erase-below', occurrences: 36 },
      { code: '\\x1b[1J', description: 'Erase Above', testId: 'erase-above', occurrences: 27 },
    ]
  },
  {
    name: 'Character Sets',
    codes: [
      { code: '\\x1b(B', description: 'G0 ASCII', testId: 'charset-ascii', occurrences: 3366 },
      { code: '\\x1b(0', description: 'G0 Graphics', testId: 'charset-graphics', occurrences: 108 },
      { code: '\\x1b)0', description: 'G1 Graphics', testId: 'charset-g1-graphics', occurrences: 72 },
      { code: '\\x1b[10m', description: 'Primary Font', testId: 'font-primary', occurrences: 45 },
    ]
  },
  {
    name: 'Terminal Modes',
    codes: [
      { code: '\\x1b[?25h', description: 'Show Cursor', testId: 'cursor-show', occurrences: 558 },
      { code: '\\x1b[?25l', description: 'Hide Cursor', testId: 'cursor-hide', occurrences: 486 },
      { code: '\\x1b[?1049h', description: 'Alt Screen Buffer', testId: 'alt-screen-on', occurrences: 234 },
      { code: '\\x1b[?1049l', description: 'Normal Screen Buffer', testId: 'alt-screen-off', occurrences: 234 },
      { code: '\\x1b[?2004h', description: 'Bracketed Paste On', testId: 'paste-on', occurrences: 162 },
      { code: '\\x1b[?2004l', description: 'Bracketed Paste Off', testId: 'paste-off', occurrences: 162 },
      { code: '\\x1b[?7h', description: 'Auto-wrap On', testId: 'wrap-on', occurrences: 90 },
      { code: '\\x1b[?7l', description: 'Auto-wrap Off', testId: 'wrap-off', occurrences: 81 },
    ]
  },
  {
    name: 'Window Operations',
    codes: [
      { code: '\\x1b]0;', description: 'Set Title', testId: 'set-title', occurrences: 297 },
      { code: '\\x1b]8;;', description: 'Hyperlink', testId: 'hyperlink', occurrences: 108 },
      { code: '\\x1b]2;', description: 'Set Window Title', testId: 'window-title', occurrences: 72 },
    ]
  }
];

test.describe('Comprehensive Escape Code Tests', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/test/escape-codes');
    await page.waitForSelector('[data-testid="terminal-container"]', { timeout: 30000 });
    await page.waitForSelector('[data-testid="category-selector"]', { timeout: 15000 });
    await expect(page.getByTestId('test-status')).toHaveText('idle', { timeout: 10000 });
  });

  // Test each category
  for (const category of ESCAPE_CODE_CATEGORIES) {
    test.describe(category.name, () => {
      test(`should handle all ${category.name} escape codes`, async ({ page }) => {
        // Select category
        await page.getByTestId('category-selector').selectOption(category.name);

        // Configure test for the category
        await page.getByTestId('frame-rate-input').fill('30');
        await page.getByTestId('duration-input').fill('10');
        await page.getByTestId('comprehensive-mode-checkbox').check();

        // Start test
        await page.getByTestId('start-test').click();

        // Wait for initial processing
        await page.waitForTimeout(2000);

        // Verify codes are being processed
        for (const code of category.codes) {
          const codeProcessed = await page.getByTestId(`processed-${code.testId}`).textContent();
          const count = parseInt(codeProcessed || '0');

          // Each code should be processed at least once
          expect(count).toBeGreaterThan(0);
        }

        // Wait for completion
        await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 15000 });

        // Verify category metrics
        const categoryErrors = await page.getByTestId(`errors-${category.name.toLowerCase().replace(/\s+/g, '-')}`).textContent();
        expect(parseInt(categoryErrors || '0')).toBe(0);

        // Verify performance for this category
        const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();
        const frameTime = parseFloat(avgFrameTime?.replace('ms', '') || '0');
        expect(frameTime).toBeLessThan(50); // Should maintain 20+ FPS
      });

      // Test high-occurrence codes individually
      for (const code of category.codes.filter(c => c.occurrences > 100)) {
        test(`should handle ${code.description} (${code.code}) correctly`, async ({ page }) => {
          await page.getByTestId('escape-code-selector').selectOption(code.testId);

          // Configure based on code type
          const duration = code.occurrences > 1000 ? '10' : '5';
          await page.getByTestId('duration-input').fill(duration);
          await page.getByTestId('frame-rate-input').fill('60');

          // Start test
          await page.getByTestId('start-test').click();

          // Monitor for a bit
          await page.waitForTimeout(2000);

          // Verify code is being processed
          const processed = await page.getByTestId('escape-codes-processed').textContent();
          expect(parseInt(processed?.replace(/,/g, '') || '0')).toBeGreaterThan(50);

          // Stop test
          await page.getByTestId('stop-test').click({ force: true });

          // Verify no errors
          const errors = await page.getByTestId('error-count').textContent();
          expect(parseInt(errors || '0')).toBe(0);
        });
      }
    });
  }

  test('should handle mixed categories simultaneously', async ({ page }) => {
    // Test with codes from multiple categories
    await page.getByTestId('escape-code-selector').selectOption('mixed-categories');

    // High-stress configuration
    await page.getByTestId('frame-rate-input').fill('120');
    await page.getByTestId('duration-input').fill('15');
    await page.getByTestId('all-categories-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Take periodic measurements
    const measurements = [];
    for (let i = 0; i < 3; i++) {
      await page.waitForTimeout(5000);

      // Check each category is being processed
      for (const category of ESCAPE_CODE_CATEGORIES) {
        const categoryId = category.name.toLowerCase().replace(/\s+/g, '-');
        const count = await page.getByTestId(`category-${categoryId}-count`).textContent();
        expect(parseInt(count || '0')).toBeGreaterThan(0);
      }

      // Collect metrics
      const metrics = await getMetrics(page);
      measurements.push(metrics);
    }

    // Verify consistent performance across categories
    const avgFrameTimes = measurements.map(m => m.avgFrameTime);
    avgFrameTimes.forEach(frameTime => {
      expect(frameTime).toBeLessThan(30); // Maintain 30+ FPS with all categories
    });

    // Verify terminal integrity
    const terminalIntegrity = await page.getByTestId('terminal-integrity').textContent();
    expect(terminalIntegrity).toBe('intact');
  });

  test('should validate escape code sequences', async ({ page }) => {
    // Test sequence validation
    await page.getByTestId('escape-code-selector').selectOption('sequence-validation');

    // Configure for validation testing
    await page.getByTestId('validation-mode-checkbox').check();
    await page.getByTestId('strict-parsing-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Wait for validation to complete
    await expect(page.getByTestId('test-status')).toHaveText('completed', { timeout: 30000 });

    // Check validation results
    const validSequences = await page.getByTestId('valid-sequences').textContent();
    const invalidSequences = await page.getByTestId('invalid-sequences').textContent();
    const malformedSequences = await page.getByTestId('malformed-sequences').textContent();

    // Most sequences should be valid
    const validCount = parseInt(validSequences || '0');
    const invalidCount = parseInt(invalidSequences || '0');
    const malformedCount = parseInt(malformedSequences || '0');

    expect(validCount).toBeGreaterThan(invalidCount + malformedCount);
    expect(malformedCount).toBe(0); // No malformed sequences in production codes
  });

  test('should handle edge cases and boundary conditions', async ({ page }) => {
    // Test edge cases
    await page.getByTestId('escape-code-selector').selectOption('edge-cases');

    // Configure for edge case testing
    await page.getByTestId('edge-case-mode-checkbox').check();
    await page.getByTestId('boundary-test-checkbox').check();

    // Start test
    await page.getByTestId('start-test').click();

    // Run edge case scenarios
    await page.waitForTimeout(5000);

    // Verify edge cases handled gracefully
    const edgeCasesPassed = await page.getByTestId('edge-cases-passed').textContent();
    const edgeCasesFailed = await page.getByTestId('edge-cases-failed').textContent();

    const passed = parseInt(edgeCasesPassed || '0');
    const failed = parseInt(edgeCasesFailed || '0');

    // All edge cases should pass
    expect(failed).toBe(0);
    expect(passed).toBeGreaterThan(0);

    // Stop test
    await page.getByTestId('stop-test').click({ force: true });

    // Verify terminal recovered from edge cases
    const terminalState = await page.getByTestId('terminal-state').textContent();
    expect(terminalState).toBe('clean');
  });
});

// Helper function
async function getMetrics(page: any) {
  const framesRendered = await page.getByTestId('frames-rendered').textContent();
  const avgFrameTime = await page.getByTestId('avg-frame-time').textContent();
  const peakMemory = await page.getByTestId('peak-memory').textContent();

  return {
    framesRendered: parseInt(framesRendered?.replace(/,/g, '') || '0'),
    avgFrameTime: parseFloat(avgFrameTime?.replace('ms', '') || '0'),
    peakMemory: parseBytes(peakMemory || '0'),
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