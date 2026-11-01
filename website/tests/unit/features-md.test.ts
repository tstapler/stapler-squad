import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import features from '../../src/content/features.json';

const FEATURES_MD_PATH = join(import.meta.dirname, '..', '..', '..', 'docs', 'features.md');

describe('docs/features.md', () => {
	it('featuresMd_should_HaveOneFeatureCardPerSection_When_Parsed', async () => {
		const contents = await readFile(FEATURES_MD_PATH, 'utf8');
		const sectionCount = contents.split('\n').filter((line) => line.startsWith('## ')).length;

		// Fails when a section is added to docs/features.md without re-running
		// `node scripts/build-features.mjs` and committing features.json.
		expect(features).toHaveLength(sectionCount);
	});

	it('featuresMd_should_NotReferenceNonexistentImage_When_Scanned', async () => {
		const contents = await readFile(FEATURES_MD_PATH, 'utf8');

		expect(contents).not.toContain('09-unfinished.png');
		expect(contents).not.toContain('Unfinished Work');
	});
});
