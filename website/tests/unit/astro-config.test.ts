// Story 1.1.1: astro.config.mjs must set `base`/`site` correctly -- the
// single most likely "works locally, breaks live" bug class per
// research/pitfalls.md §2c. These tests load the real, built config object
// (not a fixture) so a future edit to astro.config.mjs is caught here before
// it ever reaches a live deploy.
import { describe, expect, it } from 'vitest';
import astroConfig from '../../astro.config.mjs';

const EXPECTED_BASE = '/stapler-squad';
const EXPECTED_SITE = 'https://tstapler.github.io';

/**
 * The invariant `deployWorkflow`/BasePath correctness depends on: every
 * config object this site builds with must declare a non-empty `base`. No
 * linter integration exists for this repo's Astro config, so this validates
 * the invariant directly rather than shelling out to one.
 */
function assertHasBasePath(config: { base?: string }): void {
	if (!config.base) {
		throw new Error('astro config is missing a "base" value -- Pages subpath URLs will be wrong');
	}
}

describe('astro.config.mjs', () => {
	it('astroConfig_should_SetBasePathAndSite_When_ConfigLoaded', () => {
		expect(astroConfig.base).toBe(EXPECTED_BASE);
		expect(astroConfig.site).toBe(EXPECTED_SITE);
	});

	it('astroConfig_should_FailLint_When_BasePathMissing', () => {
		// Happy path: the real config has `base` set, so the check passes.
		expect(() => assertHasBasePath(astroConfig)).not.toThrow();

		// Error path: a config missing `base` must be caught by the same
		// check, not discovered only once the site is live.
		expect(() => assertHasBasePath({ site: EXPECTED_SITE })).toThrow(/missing a "base" value/);
	});
});
