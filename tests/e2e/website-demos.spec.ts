// @feature website-demos
// Records every selected scene (legacy clips plus tours/*.ts) and shotlist (shotlists/*.ts), both found by glob, to
// <WEBSITE_DEMO_OUT>/<name>.webm and merges this shard's manifest to media-manifest.json.
// Run through scripts/record-website-demos.sh, which seeds and starts an isolated server and
// exports TEST_SERVER_URL, WEBSITE_DEMO_OUT, DEMO_REF_TIME and (with --only) WEBSITE_DEMO_ONLY.
// Excluded from the regular e2e projects (playwright.config.ts) and skipped unless
// WEBSITE_DEMO_OUT is set.
import { test } from '@playwright/test';
import * as path from 'path';
import { CLIPS } from './website-demos/clips';
import { shardStart, writeShardManifest } from './website-demos/manifest';
import { defaultEnv, recordClip, recordShotList } from './website-demos/recorder';
import { discoverScenes, discoverShotLists, selectScenes } from './website-demos/scene';

const BASE_URL = process.env.TEST_SERVER_URL;
const RECORDING = Boolean(process.env.WEBSITE_DEMO_OUT);
const SKIP_REASON = 'run via scripts/record-website-demos.sh (sets TEST_SERVER_URL and WEBSITE_DEMO_OUT)';

// Loaded at collection time so each scene is its own test: one failing scene does not hide the rest.
const DEMOS = path.join(__dirname, 'website-demos');
const ONLY = process.env.WEBSITE_DEMO_ONLY;
const allScenes = RECORDING ? [...CLIPS, ...discoverScenes(path.join(DEMOS, 'tours'))] : CLIPS;
const allLists = RECORDING ? discoverShotLists(path.join(DEMOS, 'shotlists')) : [];
// --only names are checked against every recordable id, so an unknown one fails loudly.
const wanted = RECORDING ? new Set(selectScenes([...allScenes, ...allLists], ONLY).map((s) => s.name)) : new Set<string>();
const scenes = RECORDING ? allScenes.filter((s) => wanted.has(s.name)) : CLIPS;
const selectedLists = allLists.filter((l) => wanted.has(l.name));

for (const scene of scenes) {
  test(`record ${scene.name}`, async ({ browser }) => {
    test.skip(!BASE_URL || !RECORDING, SKIP_REASON);
    await recordClip(browser, BASE_URL!, scene);
  });
}

for (const list of selectedLists) {
  test(`record shots ${list.name}`, async ({ browser }) => {
    test.skip(!BASE_URL || !RECORDING, SKIP_REASON);
    await recordShotList(browser, BASE_URL!, list);
  });
}

test.afterAll(() => {
  if (!RECORDING) return;
  const env = defaultEnv();
  writeShardManifest(env.outDir, env.commit, shardStart(env.outDir));
});
