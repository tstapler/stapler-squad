// @feature website-demos
// Records every clip in website-demos/clips.ts to <WEBSITE_DEMO_OUT>/<name>.webm.
// Run through scripts/record-website-demos.sh, which seeds and starts an isolated
// server and exports TEST_SERVER_URL; not part of the regular e2e run.
import { test } from '@playwright/test';
import { CLIPS } from './website-demos/clips';
import { recordClip } from './website-demos/recorder';

const BASE_URL = process.env.TEST_SERVER_URL;
test.describe.configure({ mode: 'serial' });

for (const clip of CLIPS) {
  test(`record ${clip.name}`, async ({ browser }) => {
    test.skip(!BASE_URL, 'TEST_SERVER_URL not set; use scripts/record-website-demos.sh');
    await recordClip(browser, BASE_URL!, clip);
  });
}
