import * as fs from 'fs';
import { defineConfig, devices, webkit } from '@playwright/test';

/**
 * WebKit (iPhone) project for mobile-webkit.spec.ts. Report-only by construction: when the
 * browser build is not installed (npx playwright install webkit) the project is left out and the
 * reason is printed, so a missing WebKit never fails the suite (plan Story 7.8b).
 */
function webkitMobileProjects() {
  let installed = false;
  try {
    installed = fs.existsSync(webkit.executablePath());
  } catch {
    installed = false;
  }
  if (!installed) {
    console.warn('[playwright.config] webkit-mobile skipped: WebKit is not installed (npx playwright install webkit)');
    return [];
  }
  return [{ name: 'webkit-mobile', use: { ...devices['iPhone 13'] }, testMatch: '**/mobile-webkit.spec.ts' }];
}

/**
 * Playwright configuration for terminal flickering tests
 * Tests spin up a dedicated isolated backend on port 8544 via global-setup.ts.
 * Set TEST_SERVER_URL env var to override the target (e.g. for CI with a pre-started server).
 */

// Files viewer mobile specs (and the lazy-doc/transport spikes) run in these projects as well as `chromium`.
// Task 0.1.4b decision: WebKit ran green in the Playwright Noble image but is not yet proven on the CI runner, so
// `mobile-chromium` (Pixel-class, 390x844, touch) is the gating mobile project and `mobile-webkit` is informational,
// opt-in with PW_WEBKIT=1 (CI needs `npx playwright install webkit --with-deps` first).
// Story 1.3.5c: the chord spec also runs on desktop Firefox and WebKit, informational only (real accelerator
// delivery is the owner's manual check, Story 2.5.2). CI installs the browsers with `npx playwright install
// firefox webkit --with-deps` and runs these two projects with `continue-on-error`; Chromium stays gating.
const SHORTCUT_SPECS = ['**/files-shortcuts.spec.ts'];
const MOBILE_SPECS = ['**/files-mobile*.spec.ts', '**/files-large.spec.ts', '**/files-lazydoc-spike.spec.ts', '**/files-transport-spike.spec.ts'];

/**
 * Frontend-responsiveness perf project (Story 1.1.3). Registered only when explicitly selected
 * (`--project=perf` or PW_PERF=1): Playwright runs every registered project when none is named, and a
 * 5-run, 4x-throttled, 20-session scenario cannot share a worker with other specs (global 30 s timeout,
 * polluted timings).
 */
function perfProjects() {
  const selected = process.env.PW_PERF === '1' || /--project[= ]perf(\s|$)/.test(process.argv.join(' '));
  if (!selected) return [];
  return [
    {
      name: 'perf',
      testMatch: '**/perf/**/*.spec.ts',
      timeout: 600000,
      retries: 0,
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1600, height: 900 },
        // 4x CPU throttle on a 20-session fixture blows past the global 10 s / 15 s action and navigation budgets.
        actionTimeout: 90000,
        navigationTimeout: 180000,
        launchOptions: { args: ['--use-gl=swiftshader', '--disable-gpu-sandbox'] },
      },
    },
  ];
}

export default defineConfig({
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  testDir: './',
  // Playwright's default testMatch picks up ANY *.test.ts too, which would sweep in the
  // Jest-only unit tests under tests/e2e/baseline/ (describe/test as ts-jest globals, not
  // imported from @playwright/test -- Playwright fails loading them outright). Every real e2e
  // spec in this directory is named *.spec.ts (confirmed: no running suite needs the default
  // pattern), so this is a narrowing, not a behavior change for any spec that currently runs.
  testMatch: '**/*.spec.ts',

  // Test timeout (individual test)
  timeout: 30000,

  // Expect timeout for assertions
  expect: {
    timeout: 5000,
  },

  // --shard balances by individual test, not whole file; workers:1 below is unchanged.
  fullyParallel: true,

  // Retry failed tests once
  retries: 1,

  // Number of parallel workers
  workers: 1,

  // Reporter configuration
  reporter: [
    ['list'], // Console output
    ['html', { outputFolder: './playwright-report' }],
    ['allure-playwright', { outputFolder: 'allure-results' }],
    // scripts/e2e-baseline-compare.ts's input (Task 0.1e/7.10a): e2e-next/e2e-bun (build.yml)
    // and the local baseline/expected-diff procedures (Stories 0.1, 7.11, 8.1) all read this.
    ['json', { outputFile: process.env.PLAYWRIGHT_JSON_REPORT_PATH || './playwright-report/report.json' }],
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
      testIgnore: ['**/mobile-webkit.spec.ts', '**/perf/**'],
      use: {
        ...devices['Desktop Chrome'],
        launchOptions: {
          // SwiftShader: software WebGL so terminal canvas renders in headless CI
          args: ['--use-gl=swiftshader', '--disable-gpu-sandbox'],
        },
      },
    },
    {
      name: 'desktop-firefox',
      testMatch: SHORTCUT_SPECS,
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'desktop-webkit',
      testMatch: SHORTCUT_SPECS,
      use: { ...devices['Desktop Safari'] },
    },
    {
      name: 'mobile-chromium',
      testMatch: MOBILE_SPECS,
      use: { ...devices['Pixel 7'], viewport: { width: 390, height: 844 } },
    },
    ...(process.env.PW_WEBKIT === '1'
      ? [
          {
            name: 'mobile-webkit',
            testMatch: MOBILE_SPECS,
            use: { ...devices['iPhone 14'] },
          },
        ]
      : []),
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
      testIgnore: ['**/mobile-webkit.spec.ts', '**/perf/**'],
      use: {
        ...devices['Desktop Chrome'],
        launchOptions: {
          args: ['--disable-webgl', '--disable-3d-apis', '--disable-gpu-sandbox'],
        },
      },
    },
    ...perfProjects(),
    ...webkitMobileProjects(),
  ],
});
