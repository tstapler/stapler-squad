// Standalone config for scripts/record-website-demos.sh: no global server (the
// script starts a seeded one), one chromium worker so clips never share a server
// at the same time, and no retries (a retried clip would record twice; the recorder retries a
// flaky scene itself, on a fresh context). Mobile shots use their own 390x844 touch context
// (website-demos/shots.ts), so there is a single desktop project here.
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: '.',
  testMatch: 'website-demos.spec.ts',
  workers: 1,
  retries: 0,
  timeout: 240_000,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    viewport: { width: 1280, height: 720 },
    // Fail on the step that is stuck instead of hanging to the test timeout.
    actionTimeout: 30_000,
    navigationTimeout: 60_000,
  },
});
