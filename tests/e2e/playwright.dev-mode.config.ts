import { defineConfig, devices } from '@playwright/test';

/**
 * Opt-in Playwright configuration for the dev-mode harness (Epic 5.1,
 * plan.md Story 5.1.1). Clone of playwright.config.ts, but global-setup/
 * global-teardown start a full two-process DevStack (real Go backend +
 * real `next dev`, independently OS-assigned ports) via
 * scripts/dev-stack/launch.ts's startDevStack(), instead of the single,
 * statically-ported backend the default suite uses.
 *
 * timeout/expect.timeout are raised relative to the default config to
 * accommodate a cold `next dev` compile (first page load under `next dev`
 * is materially slower than the pre-built, statically-exported frontend
 * the default suite serves).
 *
 * Run via: `npm run test:e2e:dev-mode` (web-app/package.json) — separate
 * from the default `test:e2e` run per requirements' explicit non-goal of
 * migrating the whole suite to dev-mode.
 */

export default defineConfig({
  globalSetup: './global-setup.dev-mode.ts',
  globalTeardown: './global-teardown.dev-mode.ts',
  testDir: './',

  // Test timeout (individual test) — raised from 30000 to accommodate a
  // cold `next dev` compile on first navigation.
  timeout: 60000,

  // Expect timeout for assertions — raised from 5000 for the same reason.
  expect: {
    timeout: 15000,
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
    ['html', { outputFolder: './playwright-report-dev-mode' }],
    ['allure-playwright', { outputFolder: 'allure-results' }],
  ],

  // Global test setup
  use: {
    // Base URL for tests — dynamically assigned by global-setup.dev-mode.ts
    // (the DevStack's frontend/next-dev URL); override with TEST_SERVER_URL.
    baseURL: process.env.TEST_SERVER_URL || 'http://localhost:8544',

    // Browser trace on failure
    trace: 'on-first-retry',

    // Screenshot on failure
    screenshot: 'only-on-failure',

    // Video: always-on when RECORD_FEATURES=true, otherwise retain on failure
    video: process.env.RECORD_FEATURES === 'true' ? 'on' : 'retain-on-failure',

    // Test results output directory
    outputDir: 'test-results-dev-mode/',

    // Test timeout
    actionTimeout: 10000,

    // Navigation timeout
    navigationTimeout: 15000,
  },

  // Snapshot path template for visual regression tests
  snapshotPathTemplate: 'tests/snapshots/{projectName}/{testFilePath}/{arg}{ext}',

  // Test projects (browsers) — identical to playwright.config.ts. Only
  // backlog.dev-mode.spec.ts (Epic 5.2) is expected to run under this
  // config in practice, but the project list itself is unchanged so this
  // stays a faithful clone rather than a narrower, hand-pruned copy.
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
      // DOM-renderer project — see playwright.config.ts for the full
      // rationale (disables WebGL so xterm.js falls back to its DOM
      // renderer, for tests that assert on rendered terminal content).
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
