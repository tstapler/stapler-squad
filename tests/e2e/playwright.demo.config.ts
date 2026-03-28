import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for demo video recording.
 * The server is started by the Go test (TestRecordDemo) before this runs.
 * No globalSetup/globalTeardown — lifecycle is managed by Go.
 */

const serverURL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const outputDir = process.env.PLAYWRIGHT_VIDEO_OUTPUT_DIR || '/tmp/demo-video-output';

export default defineConfig({
  testDir: './',

  // Only run the demo spec.
  testMatch: ['**/demo.spec.ts'],

  timeout: 60000,

  expect: {
    timeout: 10000,
  },

  fullyParallel: false,
  retries: 0,
  workers: 1,

  reporter: [['list']],

  // Test artifacts directory.
  outputDir,

  use: {
    baseURL: serverURL,

    // Always record video — that's the whole point.
    video: 'on',

    // Full HD viewport for a crisp demo.
    viewport: { width: 1440, height: 900 },

    actionTimeout: 15000,
    navigationTimeout: 20000,

    // Capture screenshots at each step for review.
    screenshot: 'on',
    trace: 'off',
  },

  projects: [
    {
      name: 'chromium',
      // Spread device for browser settings, then override viewport to ensure 1920×1080.
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
  ],
});
