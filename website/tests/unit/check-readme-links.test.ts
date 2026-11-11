import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { checkReadmeLinks, extractSiteUrls, sitePath } from '../../scripts/check-readme-links.mjs';

const O = 'https://tstapler.github.io';
const B = '/stapler-squad';

function dist(pages: string[]): string {
	const dir = mkdtempSync(join(tmpdir(), 'readme-dist-'));
	for (const p of pages) {
		mkdirSync(join(dir, p), { recursive: true });
		writeFileSync(join(dir, p, 'index.html'), '<html></html>');
	}
	return dir;
}
const readme = (...paths: string[]) => paths.map((p) => `[x](${O}${B}${p})`).join('\n');

describe('check-readme-links', () => {
	it('extractSiteUrls_should_FindOnlySiteOriginUrlsOnce', () => {
		const md = `[a](${O}${B}/tours/) text ${O}${B}/tours/. [b](https://github.com/x/y) [c](${O}${B}/tours/)`;
		expect(extractSiteUrls(md)).toEqual([`${O}${B}/tours/`]);
	});

	it('sitePath_should_ReturnNull_When_UrlIsOutsideTheBase', () => {
		expect(sitePath(`${O}${B}/tours/`)).toBe('/tours/');
		expect(sitePath(`${O}${B}`)).toBe('/');
		expect(sitePath(`${O}/wrong-base/tours/`)).toBeNull();
	});

	it('passes_When_EveryLinkResolves', async () => {
		const r = await checkReadmeLinks({ readme: readme('/tours/', '/tours/a/'), distDir: dist(['tours', 'tours/a']) });
		expect(r.errors).toEqual([]);
		expect(r.checked).toBe(2);
	});

	it('fails_When_ABaseIsWrong', async () => {
		const r = await checkReadmeLinks({ readme: `[x](${O}/other/tours/)`, distDir: dist(['tours']) });
		expect(r.errors[0]).toContain('not under the site base');
	});

	it('deployMode_should_FailOnAMissingTourPage', async () => {
		const r = await checkReadmeLinks({ readme: readme('/tours/', '/tours/a/', '/tours/b/'), distDir: dist(['tours', 'tours/a']) });
		expect(r.errors).toEqual([`${O}${B}/tours/b/: no page at /tours/b/ in dist`]);
	});

	it('smokeMode_should_SkipUnbuiltTourPagesWithANotice_ButRequireTheIndexAndOneTour', async () => {
		const ok = await checkReadmeLinks({ readme: readme('/tours/', '/tours/a/', '/tours/b/'), distDir: dist(['tours', 'tours/a']), mode: 'smoke' });
		expect(ok.errors).toEqual([]);
		expect(ok.notices).toEqual([`${O}${B}/tours/b/: tour not built in this smoke build, skipped`]);

		const noIndex = await checkReadmeLinks({ readme: readme('/tours/', '/tours/a/'), distDir: dist(['tours/a']), mode: 'smoke' });
		expect(noIndex.errors).toHaveLength(1);

		const noTour = await checkReadmeLinks({ readme: readme('/tours/', '/tours/a/'), distDir: dist(['tours']), mode: 'smoke' });
		expect(noTour.errors.join()).toContain('must');
	});

	it('fails_When_ReadmeHasNoSiteUrl', async () => {
		const r = await checkReadmeLinks({ readme: 'nothing', distDir: dist([]) });
		expect(r.errors).toHaveLength(1);
	});
});
