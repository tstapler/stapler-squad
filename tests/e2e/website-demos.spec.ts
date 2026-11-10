// @feature website-demos
// Records every selected scene (legacy clips plus tours/*.ts, found by glob) to
// <WEBSITE_DEMO_OUT>/<name>.webm and merges this shard's manifest to media-manifest.json.
// Run through scripts/record-website-demos.sh, which seeds and starts an isolated server and
// exports TEST_SERVER_URL, WEBSITE_DEMO_OUT, DEMO_REF_TIME and (with --only) WEBSITE_DEMO_ONLY.
// Excluded from the regular e2e projects (playwright.config.ts) and skipped unless
// WEBSITE_DEMO_OUT is set.
import { test } from '@playwright/test';
import * as path from 'path';
import { CLIPS } from './website-demos/clips';
import { shardStart, writeShardManifest } from './website-demos/manifest';
import { defaultEnv, recordClip } from './website-demos/recorder';
import { discoverScenes, selectScenes } from './website-demos/scene';

const BASE_URL = process.env.TEST_SERVER_URL;
const RECORDING = Boolean(process.env.WEBSITE_DEMO_OUT);
const SKIP_REASON = 'run via scripts/record-website-demos.sh (sets TEST_SERVER_URL and WEBSITE_DEMO_OUT)';

// Loaded at collection time so each scene is its own test: one failing scene does not hide the rest.
const scenes = RECORDING
  ? selectScenes([...CLIPS, ...discoverScenes(path.join(__dirname, 'website-demos', 'tours'))], process.env.WEBSITE_DEMO_ONLY)
  : CLIPS;

for (const scene of scenes) {
  test(`record ${scene.name}`, async ({ browser }) => {
    test.skip(!BASE_URL || !RECORDING, SKIP_REASON);
    await recordClip(browser, BASE_URL!, scene);
  });
}

test.afterAll(() => {
  if (!RECORDING) return;
  const env = defaultEnv();
  writeShardManifest(env.outDir, env.commit, shardStart(env.outDir));
});
