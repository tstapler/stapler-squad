import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for claude-squad end-to-end tests
 *
 * Run tests with:
 *   npx playwright test
 *   npx playwright test --headed  (see browser)
 *   npx playwright test --debug   (debug mode)
 */

export default defineConfig({
  testDir: './',

  // Global setup and teardown (test server with isolated data)
  globalSetup: require.resolve('./global-setup'),
  globalTeardown: require.resolve('./global-teardown'),

  // Test timeout (individual test)
  timeout: 30000,

  // Expect timeout for assertions
  expect: {
    timeout: 5000,
  },

  // Run tests in parallel
  fullyParallel: false, // Sequential for queue state consistency

  // Fail on first test failure (faster feedback)
  fullyParallel: false,

  // Retry failed tests once
  retries: process.env.CI ? 2 : 1,

  // Number of parallel workers
  workers: process.env.CI ? 1 : 1, // Sequential for state consistency

  // Reporter configuration
  reporter: [
    ['list'], // Console output
    ['html', { outputFolder: '../playwright-report' }], // HTML report
    ['json', { outputFile: '../test-results.json' }], // JSON for CI
  ],

  // Global test setup
  use: {
    // Base URL for tests (use test server port for isolated testing)
    baseURL: process.env.BASE_URL || 'http://localhost:8544',

    // Browser trace on failure
    trace: 'on-first-retry',

    // Screenshot on failure
    screenshot: 'only-on-failure',

    // Video on failure
    video: 'retain-on-failure',

    // Test timeout
    actionTimeout: 10000,

    // Navigation timeout
    navigationTimeout: 15000,
  },

  // Test projects (browsers)
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },

    // Uncomment for multi-browser testing
    // {
    //   name: 'firefox',
    //   use: { ...devices['Desktop Firefox'] },
    // },
    // {
    //   name: 'webkit',
    //   use: { ...devices['Desktop Safari'] },
    // },
  ],

  // Web server configuration removed - using globalSetup instead for better control
  // The test server is started in globalSetup with isolated data directory
});
