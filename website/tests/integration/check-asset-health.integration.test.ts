// Integration test: runs the real Astro build, scans the real dist/ output,
// and (since no DemoClip is embedded in site content yet -- Phase 3.3 hasn't
// landed) also seeds one extra fixture page referencing a not-yet-published
// clip so the genuine HEAD request this test issues against
// github.com/tstapler/stapler-squad/releases/download/... has something to
// exercise. No fetch mocking anywhere in this file -- real network call.
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
		// Sanity: the real site build currently embeds zero release-asset URLs
		// (Phase 3.3 demo embeds haven't landed), so scanning it alone proves
		// nothing by itself -- hence the fixture below for the genuine network
		// assertion.
		const realBuildResults = await checkAssetHealth({ distDir: REAL_DIST_DIR });
		expect(realBuildResults).toEqual([]);

		const fixtureResults = await checkAssetHealth({ distDir: fixtureDir });

		expect(fixtureResults).toHaveLength(1);
		const [result] = fixtureResults;
		expect(result.ok).toBe(false);
		expect(result.url).toContain('does-not-exist-yet.gif');
		expect(result.reason).toContain('404');
	}, 30_000);
});
