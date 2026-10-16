import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const FEATURES_MD_PATH = join(import.meta.dirname, '..', '..', '..', 'docs', 'features.md');

describe('docs/features.md', () => {
	it('featuresMd_should_Have17Sections_When_Parsed', async () => {
		const contents = await readFile(FEATURES_MD_PATH, 'utf8');
		const sectionCount = contents.split('\n').filter((line) => line.startsWith('## ')).length;

		expect(sectionCount).toBe(17);
	});

	it('featuresMd_should_NotReferenceNonexistentImage_When_Scanned', async () => {
		const contents = await readFile(FEATURES_MD_PATH, 'utf8');

		expect(contents).not.toContain('09-unfinished.png');
		expect(contents).not.toContain('Unfinished Work');
	});
});
