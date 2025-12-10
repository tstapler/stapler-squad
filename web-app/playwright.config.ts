import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for terminal stress tests.
 *
 * Usage:
 *   npm run test:e2e              # Run all E2E tests
 *   npm run test:stress           # Run only terminal stress tests
 *   npx playwright test           # Direct invocation
 *
 * The webServer will automatically start on an available port.
 *
 * @see https://playwright.dev/docs/test-configuration
 */

// Use a fixed port for consistency between webServer and tests
// Can be overridden with TEST_PORT environment variable
const TEST_PORT = process.env.TEST_PORT || '3333';

export default defineConfig({
  testDir: './tests/e2e',
  /* Run tests in files in parallel */
  fullyParallel: true,
  /* Fail the build on CI if you accidentally left test.only in the source code. */
  forbidOnly: !!process.env.CI,
  /* Retry on CI only */
  retries: process.env.CI ? 2 : 0,
  /* Opt out of parallel tests on CI. */
  workers: process.env.CI ? 1 : undefined,
  /* Reporter to use. See https://playwright.dev/docs/test-reporters */
  reporter: 'html',
  /* Shared settings for all the projects below. See https://playwright.dev/docs/api/class-testoptions. */
  use: {
    /* Base URL set dynamically based on test port */
    baseURL: `http://localhost:${TEST_PORT}`,

    /* Collect trace when retrying the failed test. See https://playwright.dev/docs/trace-viewer */
    trace: 'on-first-retry',

    /* Take screenshot on failure */
    screenshot: 'only-on-failure',

    /* Record video for all tests - enables watching terminal animations */
    video: 'on',

    /* Video settings for terminal stress tests */
    contextOptions: {
      recordVideo: {
        dir: './test-results/videos',
        size: { width: 1280, height: 720 },
      },
    },
  },

  /* Configure projects for major browsers */
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        // Enable performance.memory API for memory tests
        launchOptions: {
          args: ['--enable-precise-memory-info'],
        },
      },
    },
  ],

  /* Run your local dev server before starting the tests */
  /* Use webpack mode (not turbopack) for protobuf .js import compatibility */
  webServer: {
    command: `npx next dev --port ${TEST_PORT}`,
    url: `http://localhost:${TEST_PORT}`,
    reuseExistingServer: !process.env.CI,
    timeout: 120 * 1000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
