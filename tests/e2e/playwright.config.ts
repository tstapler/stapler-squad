import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for terminal flickering tests
 * Tests spin up a dedicated isolated backend on port 8544 via global-setup.ts.
 * Set TEST_SERVER_URL env var to override the target (e.g. for CI with a pre-started server).
 */

export default defineConfig({
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
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
    // Base URL for tests — dynamically assigned by global-setup; override with TEST_SERVER_URL
    baseURL: process.env.TEST_SERVER_URL || 'http://localhost:8544',

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

  // Snapshot path template for visual regression tests
  snapshotPathTemplate: 'tests/snapshots/{projectName}/{testFilePath}/{arg}{ext}',

  // Test projects (browsers)
  projects: [
    // Visual regression projects — one per theme
    {
      name: 'visual-matrix',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'tests/e2e/fixtures/matrix-theme.json',
        viewport: { width: 1280, height: 800 },
      },
      testMatch: '**/visual-regression.spec.ts',
    },
    {
      name: 'visual-cyberpunk77',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'tests/e2e/fixtures/cyberpunk77-theme.json',
        viewport: { width: 1280, height: 800 },
      },
      testMatch: '**/visual-regression.spec.ts',
    },
    {
      name: 'visual-wh40k',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'tests/e2e/fixtures/wh40k-theme.json',
        viewport: { width: 1280, height: 800 },
      },
      testMatch: '**/visual-regression.spec.ts',
    },
    {
      name: 'visual-clean',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'tests/e2e/fixtures/clean-theme.json',
        viewport: { width: 1280, height: 800 },
      },
      testMatch: '**/visual-regression.spec.ts',
    },
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
