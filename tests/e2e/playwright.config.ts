import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for terminal flickering tests
 * Tests run against production server at localhost:8543
 */

export default defineConfig({
  testDir: './',

  // Test timeout (individual test)
  timeout: 30000,

  // Expect timeout for assertions
  expect: {
    timeout: 5000,
  },

  // Run tests sequentially
  fullyParallel: false,

  // Retry failed tests once
  retries: 1,

  // Number of parallel workers
  workers: 1,

  // Reporter configuration
  reporter: [
    ['list'], // Console output
    ['html', { outputFolder: './playwright-report' }],
  ],

  // Global test setup
  use: {
    // Base URL for tests (production server)
    baseURL: 'http://localhost:8543',

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
  ],
});
