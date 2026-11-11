// Story 4.4.1: BrokenLinkCheck walks a built dist/ tree and validates every
// internal <a href> resolves to a real file. Runs against small fixture dist/
// trees (mkdtemp'd), not the real site -- see
// tests/integration/check-internal-links.integration.test.ts for the real-build run.
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { spawnSync } from 'node:child_process';
import { checkInternalLinks, extractMediaRefs } from '../../scripts/check-internal-links.mjs';

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

describe('media reference checking (Story 3.1.3)', () => {
	const SCRIPT = join(import.meta.dirname, '..', '..', 'scripts', 'check-internal-links.mjs');
	let dir: string;

	beforeEach(async () => {
		dir = await mkdtemp(join(tmpdir(), 'check-media-links-test-'));
		await mkdir(join(dir, 'tours', 'x'), { recursive: true });
		await mkdir(join(dir, 'shots'), { recursive: true });
		await writeFile(join(dir, 'shots', 'present.webp'), 'x');
	});

	afterEach(async () => {
		await rm(dir, { recursive: true, force: true });
	});

	it('extractMediaRefs_should_ReadImgVideoAndSourceAttributes_When_HtmlHasThem', () => {
		const html =
			'<img src="/a.webp" alt=""><video poster="/p.webp" src="/v.webm"><source src="/s.mp4" type="video/mp4"></video><a href="/no">x</a>';
		expect(extractMediaRefs(html)).toEqual(['/a.webp', '/v.webm', '/p.webp', '/s.mp4']);
	});

	it('checkInternalLinks_should_NameMissingPoster_When_VideoPosterAbsentFromDist', async () => {
		await writeFile(
			join(dir, 'tours', 'x', 'index.html'),
			'<video poster="/stapler-squad/shots/missing.webp"><source src="/stapler-squad/shots/present.webp"></video>',
		);
		const { failures } = await checkInternalLinks({ distDir: dir });
		expect(failures.map((f) => f.href)).toEqual(['/stapler-squad/shots/missing.webp']);
	});

	it('cli_should_ExitOneNamingPoster_When_TourPosterMissing', async () => {
		await writeFile(join(dir, 'tours', 'x', 'index.html'), '<video poster="/stapler-squad/shots/missing.webp"></video>');
		const result = spawnSync('node', [SCRIPT, dir], { encoding: 'utf8' });
		expect(result.status).toBe(1);
		expect(result.stderr).toContain('/stapler-squad/shots/missing.webp');
	});

	it('checkInternalLinks_should_IgnoreExternalAndDataUris_When_MediaIsNotLocal', async () => {
		await writeFile(
			join(dir, 'tours', 'x', 'index.html'),
			'<img src="https://example.com/a.png"><img src="data:image/gif;base64,AAAA">',
		);
		expect((await checkInternalLinks({ distDir: dir })).failures).toEqual([]);
	});
});
