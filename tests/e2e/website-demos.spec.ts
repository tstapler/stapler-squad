// @feature website-demos
// Records every clip in website-demos/clips.ts to <WEBSITE_DEMO_OUT>/<name>.webm.
// Run through scripts/record-website-demos.sh, which seeds and starts an isolated
// server and exports TEST_SERVER_URL and WEBSITE_DEMO_OUT. Excluded from the regular
// e2e projects (playwright.config.ts) and skipped unless WEBSITE_DEMO_OUT is set.
import { test } from '@playwright/test';
import { CLIPS } from './website-demos/clips';
import { recordClip } from './website-demos/recorder';

const BASE_URL = process.env.TEST_SERVER_URL;
const RECORDING = Boolean(process.env.WEBSITE_DEMO_OUT);

for (const clip of CLIPS) {
  test(`record ${clip.name}`, async ({ browser }) => {
    test.skip(!BASE_URL || !RECORDING, 'run via scripts/record-website-demos.sh (sets TEST_SERVER_URL and WEBSITE_DEMO_OUT)');
    await recordClip(browser, BASE_URL!, clip);
  });
}
