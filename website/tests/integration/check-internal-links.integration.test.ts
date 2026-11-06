// Integration test: runs the real Astro build (Phases 3 + 4 content included)
// and scans the real dist/ output for broken internal links. No fixture --
// this is the "does the real site actually pass" counterpart to
// tests/unit/check-internal-links.test.ts's fixture-based unit coverage.
import { join } from 'node:path';
import { beforeAll, describe, expect, it } from 'vitest';
import { checkInternalLinks } from '../../scripts/check-internal-links.mjs';
import { runRealBuild } from '../helpers/run-build.ts';

const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const REAL_DIST_DIR = join(WEBSITE_ROOT, 'dist');

describe('checkInternalLinks against real build output', () => {
	beforeAll(() => {
		runRealBuild(WEBSITE_ROOT);
	}, 120_000);

	it('checkInternalLinks_should_ScanRealBuiltDistOutput_When_RunAgainstPhase3And4Content', async () => {
		const { failures, checked } = await checkInternalLinks({ distDir: REAL_DIST_DIR });

		expect(failures).toEqual([]);
		expect(checked).toBeGreaterThan(0);
	}, 30_000);
});
