// Story 3.2.1: runs buildFeaturesJson against the real README.md and
// docs/features.md on disk, not fixtures -- see tests/unit/features-json.test.ts
// for the mocked-fixture unit coverage.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { buildFeaturesJson } from '../../scripts/build-features.mjs';

// This file lives at website/tests/integration/, so website/ is three
// directories up.
const WEBSITE_DIR = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const REPO_ROOT = join(WEBSITE_DIR, '..');

describe('buildFeaturesJson against real on-disk content', () => {
	it('buildFeaturesJson_should_ProduceCommittedCardsWithValidScreenshotPaths_When_RunAgainstRealReadmeAndFeaturesMd', () => {
		const cards = buildFeaturesJson({
			readmePath: join(REPO_ROOT, 'README.md'),
			featuresMdPath: join(REPO_ROOT, 'docs', 'features.md'),
			mediaPath: join(WEBSITE_DIR, 'src', 'content', 'feature-media.json'),
			requireMedia: true,
		});

		// features.json is committed generated output and now identical in git and
		// in the deployed build (media is declared by name); fail on any drift.
		const committed = JSON.parse(readFileSync(join(WEBSITE_DIR, 'src', 'content', 'features.json'), 'utf8'));
		expect(cards).toEqual(committed);

		// Story 3.2.5: every card has a shot, named by the recorder's manifest (shots/<name>.webp);
		// the files themselves are recorded in CI and checked by `mediatool fresh`, not committed.
		for (const card of cards) {
			expect(card.alt, card.heading).toBeTruthy();
			expect(card.screenshot, card.heading).toMatch(/^\/shots\/[a-z0-9-]+\.webp$/);
		}
	});
});
