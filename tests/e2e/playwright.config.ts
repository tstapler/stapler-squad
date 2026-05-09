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
    ['allure-playwright', { outputFolder: 'allure-results' }],
  ],

  // Global test setup
  use: {
    // Base URL for tests (production server)
    baseURL: 'http://localhost:8543',

    // Browser trace on failure
    trace: 'on-first-retry',

    // Screenshot on failure
    screenshot: 'only-on-failure',

    // Video: always-on when RECORD_FEATURES=true, otherwise retain on failure
    video: process.env.RECORD_FEATURES === 'true' ? 'on' : 'retain-on-failure',

    // Test results output directory
    outputDir: 'test-results/',

    // Test timeout
    actionTimeout: 10000,

    // Navigation timeout
    navigationTimeout: 15000,
  },

  // Test projects (browsers)
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        launchOptions: {
          // SwiftShader: software WebGL so terminal canvas renders in headless CI
          args: ['--use-gl=swiftshader', '--disable-gpu-sandbox'],
        },
      },
    },
    {
      // DOM-renderer project: disables WebGL entirely so xterm.js falls back to
      // its built-in DOM renderer.  Text content appears in real .xterm-rows > div
      // spans, making terminal output directly readable from the browser without
      // relying on tmux capture-pane.  Use this project for any test that needs to
      // assert on rendered terminal content via the DOM (e.g. countRenderedRows,
      // reading text from .xterm-rows).
      //
      // How it works: XtermTerminal.tsx guards WebglAddon behind
      //   if (typeof WebGL2RenderingContext !== 'undefined')
      // --disable-webgl makes that check false → no addon loads → DOM renderer.
      name: 'chromium-dom',
      use: {
        ...devices['Desktop Chrome'],
        launchOptions: {
          args: ['--disable-webgl', '--disable-3d-apis', '--disable-gpu-sandbox'],
        },
      },
    },
  ],
});
