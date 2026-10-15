// Story 4.3.1 integration test: proves the 11 Troubleshooting CuratedDocs are
// reachable in the real built dist/ output -- not just listed in
// astro.config.mjs (tests/unit/sidebar-config.test.ts checks the config
// alone; a config entry pointing at a slug Astro can't route to would still
// pass that unit test but fail here).
import { access, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { beforeAll, describe, expect, it } from 'vitest';
import { extractSidebarGroupSlugs } from '../../src/lib/sidebar-config.ts';
import { runRealBuild } from '../helpers/run-build.ts';

const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const REAL_DIST_DIR = join(WEBSITE_ROOT, 'dist');
const ASTRO_CONFIG_PATH = join(WEBSITE_ROOT, 'astro.config.mjs');

describe('built site reachability', () => {
	beforeAll(() => {
		runRealBuild(WEBSITE_ROOT);
	}, 120_000);

	it('sidebarConfig_should_RenderAllCuratedDocsReachable_When_SiteBuilt', async () => {
		const configSource = await readFile(ASTRO_CONFIG_PATH, 'utf8');
		const troubleshootingSlugs = extractSidebarGroupSlugs(configSource, 'Troubleshooting');
		expect(troubleshootingSlugs).toHaveLength(11);

		// Every sidebar slug has a real built page...
		for (const slug of troubleshootingSlugs) {
			await expect(access(join(REAL_DIST_DIR, slug, 'index.html'))).resolves.toBeUndefined();
		}

		// ...and the sidebar nav rendered on any one help/ page links to all 11,
		// so a visitor reaches every page without navigating back to docs home.
		const gettingStartedHtml = await readFile(join(REAL_DIST_DIR, 'help', 'getting-started', 'index.html'), 'utf8');
		for (const slug of troubleshootingSlugs) {
			expect(gettingStartedHtml).toContain(`/stapler-squad/${slug}/`);
		}
	}, 30_000);
});
