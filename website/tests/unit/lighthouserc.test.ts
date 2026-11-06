// Phase 6, Story 6.1.1: the site's own Lighthouse CI config must be higher
// (and genuinely independent -- not a copy-paste) than tests/e2e/lighthouse.config.js's
// warn-only 0.70/0.80 thresholds, per research/ux.md §3's "copying web-app/'s
// config verbatim would under-enforce" note. Two files (desktop + mobile
// emulation passes) are checked identically.
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';

const require = createRequire(import.meta.url);
const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const REPO_ROOT = join(WEBSITE_ROOT, '..');

const CATEGORIES = ['performance', 'accessibility', 'best-practices'] as const;
const DESKTOP_CONFIG = 'lighthouserc.json';
const MOBILE_CONFIG = 'lighthouserc.mobile.json';

function loadAssertions(path: string): Record<string, unknown> {
	const config = JSON.parse(readFileSync(path, 'utf8'));
	return config.ci.assert.assertions;
}

describe('website/lighthouserc.json and lighthouserc.mobile.json', () => {
	for (const file of [DESKTOP_CONFIG, MOBILE_CONFIG]) {
		it(`lighthouseRc_should_SetMinScore0_9WithErrorAssertion_When_ConfigParsed (${file})`, () => {
			const assertions = loadAssertions(join(WEBSITE_ROOT, file));

			for (const category of CATEGORIES) {
				expect(assertions[`categories:${category}`]).toEqual(['error', { minScore: 0.9 }]);
			}
		});
	}

	it('lighthouseRc_should_BeStricterThanWebAppThresholds_When_ConfigsCompared', () => {
		const websiteAssertions = loadAssertions(join(WEBSITE_ROOT, DESKTOP_CONFIG));
		const webAppAssertions: Record<string, unknown> = require(
			join(REPO_ROOT, 'tests', 'e2e', 'lighthouse.config.js'),
		).ci.assert.assertions;

		// The actual invariant: website/'s thresholds must be a strictly higher
		// bar than whatever web-app/ currently enforces, and web-app/'s severity
		// must still be advisory (warn), not blocking (error). Asserting the
		// *relationship* rather than web-app's literal numbers means a
		// legitimate, unrelated change to tests/e2e/lighthouse.config.js doesn't
		// break this website test with no visible connection between the two.
		for (const category of CATEGORIES) {
			const key = `categories:${category}`;
			const [websiteSeverity, { minScore: websiteMinScore }] = websiteAssertions[key] as [
				string,
				{ minScore: number },
			];
			const [webAppSeverity, { minScore: webAppMinScore }] = webAppAssertions[key] as [
				string,
				{ minScore: number },
			];

			expect(webAppSeverity).toBe('warn');
			expect(websiteMinScore).toBeGreaterThan(webAppMinScore);
		}
	});
});
