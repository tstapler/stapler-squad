// Story 3.1.2: card media is declared by name in feature-media.json; the old
// heading->clip map and the file-presence-dependent output are gone.
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const SCRIPTS = join(import.meta.dirname, '..', '..', 'scripts');

describe('build-features.mjs', () => {
	it('buildFeatures_should_NotReferenceDemoClipByHeading_When_ScriptsScanned', () => {
		for (const file of readdirSync(SCRIPTS)) {
			expect(readFileSync(join(SCRIPTS, file), 'utf8'), file).not.toContain('DEMO_CLIP_BY_HEADING');
		}
	});

	it('buildFeatures_should_NotProbeTheFilesystemForClips_When_SourceRead', () => {
		const source = readFileSync(join(SCRIPTS, 'build-features.mjs'), 'utf8');
		expect(source).not.toContain('existsSync');
		expect(source).not.toContain("'clips'");
	});
});
