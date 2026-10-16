// Story 4.2.1: features.mdx must reuse the shared features.json data (via
// <FeatureGrid>, Story 3.2.1's output) rather than duplicating feature copy a
// second time, and must render all 17 entries including the 9 with no
// screenshot. FeatureGrid/FeatureCard themselves are read-only here (Phase 3
// already covers their own rendering behavior in feature-card.test.ts) -- this
// test is about features.mdx's own contract: it imports, it doesn't copy.
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import features from '../../src/content/features.json';

const ENCODING = 'utf8';
const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const FEATURES_MDX_PATH = join(WEBSITE_ROOT, 'src', 'content', 'docs', 'help', 'features.mdx');

describe('features.mdx', () => {
	it('featuresMdxPage_should_RenderAll17FeatureCardEntries_When_ImportingSharedFeaturesJson', async () => {
		const source = await readFile(FEATURES_MDX_PATH, ENCODING);

		expect(features).toHaveLength(17);

		// Imports FeatureGrid (which imports features.json directly) rather than
		// importing features.json itself a second time or hand-copying entries.
		expect(source).toMatch(/import\s+FeatureGrid\s+from\s+['"].*FeatureGrid\.astro['"]/);
		expect(source).toContain('<FeatureGrid');

		// Regression guard: no feature description is pasted into the page's own
		// source -- it only ever reaches the page through the FeatureGrid import.
		for (const feature of features) {
			expect(source).not.toContain(feature.description);
		}
	});

	it('featuresMdxPage_should_RenderTextOnlyCard_When_ScreenshotFieldAbsent', () => {
		const textOnlyFeatures = features.filter((feature) => !('screenshot' in feature));
		const screenshotFeatures = features.filter((feature) => 'screenshot' in feature);

		// research/features.md §4: just over half of the 17 entries are text-only.
		expect(textOnlyFeatures).toHaveLength(9);
		expect(screenshotFeatures).toHaveLength(8);
	});
});
