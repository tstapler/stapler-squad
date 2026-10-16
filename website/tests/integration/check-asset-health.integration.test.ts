// Integration test: runs the real Astro build, scans the real dist/ output,
// and also seeds one extra fixture page referencing a different
// not-yet-published clip, so the genuine HEAD request this test issues
// against github.com/tstapler/stapler-squad/releases/download/... has two
// independent cases to exercise. No fetch mocking anywhere in this file --
// real network call.
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { releaseAssetUrl } from '../../src/lib/release-asset.ts';
import { checkAssetHealth } from '../../scripts/check-asset-health.mjs';
import { runRealBuild } from '../helpers/run-build.ts';

const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const REAL_DIST_DIR = join(WEBSITE_ROOT, 'dist');

describe('checkAssetHealth against real build output', () => {
	let fixtureDir: string;

	beforeAll(async () => {
		runRealBuild(WEBSITE_ROOT);

		fixtureDir = await mkdtemp(join(tmpdir(), 'check-asset-health-integration-'));
		const unpublishedUrl = releaseAssetUrl({
			tag: 'website-demo-gifs-latest',
			name: 'does-not-exist-yet',
			ext: 'gif',
		});
		await writeFile(join(fixtureDir, 'fixture.html'), `<html><body><img src="${unpublishedUrl}"></body></html>`);
	}, 120_000);

	afterAll(async () => {
		if (fixtureDir) {
			await rm(fixtureDir, { recursive: true, force: true });
		}
	});

	it('checkAssetHealth_should_ScanBuiltDistAndValidateRealReleaseUrls_When_RunAgainstDistOutput', async () => {
		// Story 3.3.1's second AC: the real site build now embeds exactly one
		// release-asset URL -- the "Live Terminal Streaming" FeatureCard's
		// terminal-streaming.webm DemoClip. It isn't published yet (Phase 5's
		// demo-recording workflow is a separate, already-deferred task), so this
		// genuine HEAD request is expected to 404 until that clip ships.
		const realBuildResults = await checkAssetHealth({ distDir: REAL_DIST_DIR });
		expect(realBuildResults).toHaveLength(1);
		const [realResult] = realBuildResults;
		expect(realResult.ok).toBe(false);
		expect(realResult.url).toContain('terminal-streaming.webm');
		expect(realResult.reason).toContain('404');

		const fixtureResults = await checkAssetHealth({ distDir: fixtureDir });

		expect(fixtureResults).toHaveLength(1);
		const [result] = fixtureResults;
		expect(result.ok).toBe(false);
		expect(result.url).toContain('does-not-exist-yet.gif');
		expect(result.reason).toContain('404');
	}, 30_000);
});
