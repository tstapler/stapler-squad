// Phase 6, Story 6.1.1: a real Lighthouse CI run (not a mocked one) against
// the built dist/, both the desktop config (lighthouserc.json) and the
// mobile-emulation config (lighthouserc.mobile.json), asserting every
// category score is actually >= 0.90 -- not just that `lhci autorun`'s own
// exit code was 0 (which the error-level assertions in both configs already
// guarantee, but reading the real numbers back is stronger evidence per
// this repo's "run it, don't read it" convention).
//
// Heavy (spawns a real build, a real `astro preview` server, and a real
// headless Chrome per config) -- excluded from the default `bun run test` run
// the same way every other *.integration.test.ts file here is (see
// vitest.config.ts's `test:integration` script).
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { execFileSync } from 'node:child_process';
import { existsSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { runRealBuild } from '../helpers/run-build';

const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const CATEGORIES = ['performance', 'accessibility', 'best-practices'] as const;
const MIN_SCORE = 0.9;

/**
 * Local/sandboxed-dev fallback only: a real CI runner (GitHub Actions'
 * ubuntu-22.04 image) ships Chrome, so lhci's own chrome-launcher finds it
 * without this. This environment doesn't have a system Chrome, but does
 * have Playwright's own downloaded Chromium (installed for
 * tests/a11y.spec.ts) -- reuse it rather than requiring a second browser
 * download for the exact same check.
 */
function findFallbackChromePath(): string | undefined {
	if (process.env.CHROME_PATH) return process.env.CHROME_PATH;
	const cacheDir = join(homedir(), '.cache', 'ms-playwright');
	if (!existsSync(cacheDir)) return undefined;
	const candidates = readdirSync(cacheDir)
		.filter((name) => /^chromium-\d+$/.test(name))
		.sort()
		.reverse();
	for (const dir of candidates) {
		const exe = join(cacheDir, dir, 'chrome-linux64', 'chrome');
		if (existsSync(exe)) return exe;
	}
	return undefined;
}

function runLhciAndReadScores(configFile: string, outputDir: string): Array<Record<string, number>> {
	const outputPath = join(WEBSITE_ROOT, outputDir);
	rmSync(outputPath, { recursive: true, force: true });

	const chromePath = findFallbackChromePath();
	execFileSync(
		'npx',
		[
			'lhci',
			'autorun',
			`--config=./${configFile}`,
			'--collect.settings.chromeFlags=--no-sandbox --headless=new',
		],
		{
			cwd: WEBSITE_ROOT,
			env: chromePath ? { ...process.env, CHROME_PATH: chromePath } : process.env,
			stdio: 'inherit',
		},
	);

	const reportFiles = readdirSync(outputPath).filter((name) => name.endsWith('.report.json'));
	expect(reportFiles.length).toBeGreaterThan(0);

	return reportFiles.map((name) => {
		const report = JSON.parse(readFileSync(join(outputPath, name), 'utf8'));
		return {
			performance: report.categories.performance.score,
			accessibility: report.categories.accessibility.score,
			'best-practices': report.categories['best-practices'].score,
		};
	});
}

describe('lighthouseCi against real built dist/', () => {
	beforeAll(() => {
		runRealBuild(WEBSITE_ROOT);
	}, 120_000);

	afterAll(() => {
		// `astro preview` daemonizes itself (see playwright.config.ts's comment
		// on this), so lhci's own startServerCommand-spawned process doesn't
		// stop on its own -- tear it down explicitly, same as the manual
		// desktop/mobile runs this test automates.
		try {
			execFileSync('bun', ['run', 'astro', '--', 'preview', 'stop'], { cwd: WEBSITE_ROOT, stdio: 'ignore' });
		} catch {
			// No server was running -- nothing to stop.
		}
		rmSync(join(WEBSITE_ROOT, '.lighthouseci'), { recursive: true, force: true });
		rmSync(join(WEBSITE_ROOT, '.lighthouseci-mobile'), { recursive: true, force: true });
	});

	it('lighthouseCi_should_PassBothDesktopAndMobilePasses_When_RunAgainstBuiltDist', () => {
		const desktopScores = runLhciAndReadScores('lighthouserc.json', '.lighthouseci');
		const mobileScores = runLhciAndReadScores('lighthouserc.mobile.json', '.lighthouseci-mobile');

		for (const scores of [...desktopScores, ...mobileScores]) {
			for (const category of CATEGORIES) {
				expect(scores[category]).toBeGreaterThanOrEqual(MIN_SCORE);
			}
		}
	}, 180_000);
});
