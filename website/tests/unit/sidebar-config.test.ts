// Story 4.3.1: all 11 `include: true` CuratedDocs from Story 2.1.2 must be
// listed under the sidebar's "Troubleshooting" group, and every listed slug
// must have a matching file on disk (no drift between config and content).
import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { extractSidebarGroupSlugs, findSlugsWithNoMatchingDoc } from '../../src/lib/sidebar-config.ts';

const ENCODING = 'utf8';
const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const ASTRO_CONFIG_PATH = join(WEBSITE_ROOT, 'astro.config.mjs');
const HELP_DOCS_DIR = join(WEBSITE_ROOT, 'src', 'content', 'docs', 'help');
const DOCS_ROOT_DIR = join(WEBSITE_ROOT, 'src', 'content', 'docs');

// Pages that belong to the "Getting Started"/"Features" sidebar groups, not
// "Troubleshooting" -- excluded from the on-disk CuratedDoc count below.
const NON_TROUBLESHOOTING_HELP_PAGES = new Set(['getting-started.md', 'features.mdx']);

async function realCuratedDocSlugs(): Promise<string[]> {
	const entries = await readdir(HELP_DOCS_DIR);
	return entries
		.filter((entry) => !NON_TROUBLESHOOTING_HELP_PAGES.has(entry))
		.map((entry) => `help/${entry.replace(/\.mdx?$/, '')}`)
		.sort();
}

describe('astro.config.mjs sidebar', () => {
	it('sidebarConfig_should_List11TroubleshootingEntries_When_AstroConfigLoaded', async () => {
		const configSource = await readFile(ASTRO_CONFIG_PATH, ENCODING);
		const troubleshootingSlugs = extractSidebarGroupSlugs(configSource, 'Troubleshooting');

		expect(troubleshootingSlugs).toHaveLength(11);
		expect([...troubleshootingSlugs].sort()).toEqual(await realCuratedDocSlugs());
	});

	it('sidebarConfig_should_FailValidation_When_SlugHasNoMatchingCuratedDoc', async () => {
		const realSlugs = await realCuratedDocSlugs();

		// Happy path: the real config's slugs are all backed by real files.
		expect(await findSlugsWithNoMatchingDoc(realSlugs, DOCS_ROOT_DIR)).toEqual([]);

		// Error path: a slug pointing at a file that was never generated (e.g.
		// an excluded/renamed CuratedDoc) is caught, not silently ignored.
		const slugsWithDrift = [...realSlugs, 'help/does-not-exist'];
		const missing = await findSlugsWithNoMatchingDoc(slugsWithDrift, DOCS_ROOT_DIR);
		expect(missing).toEqual(['help/does-not-exist']);
	});
});
