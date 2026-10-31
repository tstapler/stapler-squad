// Story 3.2.1: runs buildFeaturesJson against the real README.md and
// docs/features.md on disk, not fixtures -- see tests/unit/features-json.test.ts
// for the mocked-fixture unit coverage.
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { buildFeaturesJson } from '../../scripts/build-features.mjs';

// This file lives at website/tests/integration/, so website/ is three
// directories up.
const WEBSITE_DIR = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const REPO_ROOT = join(WEBSITE_DIR, '..');
const PUBLIC_FEATURES_DIR = join(WEBSITE_DIR, 'public', 'features');

describe('buildFeaturesJson against real on-disk content', () => {
	it('buildFeaturesJson_should_ProduceCommittedCardsWithValidScreenshotPaths_When_RunAgainstRealReadmeAndFeaturesMd', () => {
		const cards = buildFeaturesJson({
			readmePath: join(REPO_ROOT, 'README.md'),
			featuresMdPath: join(REPO_ROOT, 'docs', 'features.md'),
		});

		// features.json is committed generated output; fail on any drift from its sources.
		const committed = JSON.parse(readFileSync(join(WEBSITE_DIR, 'src', 'content', 'features.json'), 'utf8'));
		expect(cards).toEqual(committed);

		const cardsWithScreenshot = cards.filter((card) => card.screenshot);
		expect(cardsWithScreenshot).toHaveLength(8);
		for (const card of cardsWithScreenshot) {
			expect(card.alt).toBeTruthy();
			const screenshotFilename = card.screenshot.replace(/^\/features\//, '');
			const diskPath = join(PUBLIC_FEATURES_DIR, screenshotFilename);
			expect(existsSync(diskPath), `expected ${diskPath} to exist on disk`).toBe(true);
		}
	});
});
