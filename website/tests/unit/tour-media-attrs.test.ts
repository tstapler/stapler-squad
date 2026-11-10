// R12, Cross-cutting 7e: lazy media attributes and webm-then-mp4 sources on a built tour page.
import { readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { JSDOM } from 'jsdom';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { buildWithFixtures, FIXTURE_ID, fixtureSteps, manifestFor, tourMdx, type FixtureBuild } from '../helpers/tour-fixture.ts';

const CLIP_BUDGET_BYTES = 3 * 1024 * 1024;
let build: FixtureBuild;
let doc: Document;

beforeAll(() => {
	build = buildWithFixtures({
		manifest: manifestFor({ mobileShots: ['zz-fixture-phone'] }),
		tours: { [FIXTURE_ID]: tourMdx({ phoneFigure: 'zz-fixture-phone' }) },
		clipFiles: { [`${FIXTURE_ID}.webm`]: 100, [`${FIXTURE_ID}.mp4`]: 100 },
	});
	expect(build.status, build.output).toBe(0);
	doc = new JSDOM(readFileSync(join(build.outDir, 'tours', FIXTURE_ID, 'index.html'), 'utf8')).window.document;
}, 120_000);
afterAll(() => build?.cleanup());

describe('built tour page media attributes', () => {
	it('video_should_HavePreloadNonePosterControlsAndNoAutoplay', () => {
		const videos = [...doc.querySelectorAll('video')];
		expect(videos.length).toBeGreaterThan(0);
		for (const v of videos) {
			expect(v.getAttribute('preload')).toBe('none');
			expect(v.getAttribute('poster')).toBeTruthy();
			expect(v.hasAttribute('controls')).toBe(true);
			expect(v.hasAttribute('autoplay')).toBe(false);
		}
	});

	it('video_should_ListWebmSourceThenMp4_And_BothFilesExistUnderBudget', () => {
		for (const v of doc.querySelectorAll('video')) {
			const sources = [...v.querySelectorAll('source')];
			expect(sources.map((s) => s.getAttribute('type'))).toEqual(['video/webm', 'video/mp4']);
			for (const s of sources) {
				const file = s.getAttribute('src')!.replace('/stapler-squad/', '');
				expect(statSync(join(build.outDir, file)).size).toBeLessThan(CLIP_BUDGET_BYTES);
			}
		}
	});

	it('images_should_BeLazyWithExplicitSize_And_FiguresReserveTheirBox', () => {
		const imgs = [...doc.querySelectorAll('img')].filter((i) => i.closest('.tour-page'));
		expect(imgs.length).toBeGreaterThanOrEqual(fixtureSteps().length + 1);
		for (const img of imgs) {
			expect(img.getAttribute('loading')).toBe('lazy');
			expect(Number(img.getAttribute('width'))).toBeGreaterThan(0);
			expect(Number(img.getAttribute('height'))).toBeGreaterThan(0);
			expect(img.getAttribute('alt')).toBeTruthy();
			expect(img.closest('figure')?.getAttribute('style')).toContain('aspect-ratio');
		}
	});

	it('phoneFigure_should_LinkToFullSizeShot_WithAriaLabel', () => {
		const link = doc.querySelector('[data-testid="tour-phone-figure"] a')!;
		expect(link.getAttribute('aria-label')).toMatch(/^Open full-size screenshot: /);
		expect(link.getAttribute('href')).toContain('zz-fixture-phone.mobile.webp');
	});
});
