// Story 4.4.1: BrokenLinkCheck walks a built dist/ tree and validates every
// internal <a href> resolves to a real file. Runs against small fixture dist/
// trees (mkdtemp'd), not the real site -- see
// tests/integration/check-internal-links.integration.test.ts for the real-build run.
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { checkInternalLinks } from '../../scripts/check-internal-links.mjs';

const INDEX_HTML = 'index.html';
const HELP_DIR = 'help';
const GETTING_STARTED_SLUG = 'getting-started';
const DEBUG_WITH_LOGS_SLUG = 'debug-with-logs';

async function writeFixtureSite(dir: string, { withBrokenLink }: { withBrokenLink: boolean }) {
	await mkdir(join(dir, HELP_DIR, GETTING_STARTED_SLUG), { recursive: true });
	await mkdir(join(dir, HELP_DIR, DEBUG_WITH_LOGS_SLUG), { recursive: true });

	const brokenLinkHtml = withBrokenLink
		? '<a href="/stapler-squad/help/does-not-exist/">missing</a>'
		: '';

	await writeFile(
		join(dir, INDEX_HTML),
		`<html><body>
			<a href="/stapler-squad/help/getting-started/">Getting Started</a>
			<a href="https://github.com/tstapler/stapler-squad">GitHub</a>
			<a href="#top">Top</a>
		</body></html>`,
	);
	await writeFile(
		join(dir, HELP_DIR, GETTING_STARTED_SLUG, INDEX_HTML),
		`<html><body>
			<a href="/stapler-squad/help/debug-with-logs/">Debugging logs</a>
			${brokenLinkHtml}
		</body></html>`,
	);
	await writeFile(join(dir, HELP_DIR, DEBUG_WITH_LOGS_SLUG, INDEX_HTML), '<html><body>Logs doc</body></html>');
}

describe('checkInternalLinks', () => {
	let dir: string;

	beforeEach(async () => {
		dir = await mkdtemp(join(tmpdir(), 'check-internal-links-test-'));
	});

	afterEach(async () => {
		await rm(dir, { recursive: true, force: true });
	});

	it('checkInternalLinks_should_PassSilently_When_AllLinksResolveToRealRoutes', async () => {
		await writeFixtureSite(dir, { withBrokenLink: false });

		const { failures } = await checkInternalLinks({ distDir: dir });

		expect(failures).toEqual([]);
	});

	it('checkInternalLinks_should_ExitNonZeroAndNameBrokenLink_When_LinkPointsToMissingSlug', async () => {
		await writeFixtureSite(dir, { withBrokenLink: true });

		const { failures } = await checkInternalLinks({ distDir: dir });

		expect(failures).toHaveLength(1);
		expect(failures[0].href).toBe('/stapler-squad/help/does-not-exist/');
		expect(failures[0].sourceFile).toContain(GETTING_STARTED_SLUG);
	});
});
