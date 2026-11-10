// Story 3.3.1: the landing page's hero source, tours link, Start-here strip and per-card tour links,
// on a hermetic build with fixture tours named like the real ones the cards point at.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { JSDOM } from 'jsdom';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { buildWithFixtures, fixtureSteps, manifestFor, tourMdx, type FixtureBuild } from '../helpers/tour-fixture.ts';

const BUILT = ['dashboard-lifecycle', 'approval-review', 'organise-at-scale', 'zz-order-four'];
let build: FixtureBuild;
let landing: Document;
let features: Document;

const stepsFor = (id: string) => fixtureSteps(3).map((s) => ({ ...s, shot: `${id}-${s.shot}` }));
const dom = (...p: string[]) => new JSDOM(readFileSync(join(build.outDir, ...p), 'utf8')).window.document;

beforeAll(() => {
	const manifest = manifestFor({ clip: '', shots: BUILT.flatMap((id) => stepsFor(id).map((s) => s.shot)) });
	manifest.entries.push(...BUILT.map((id) => ({ name: id, kind: 'clip' as const, durationMs: 45_000 })));
	manifest.entries.push(...BUILT.map((id) => ({ name: `${id}-poster`, kind: 'shot' as const, width: 1280, height: 720 })));
	build = buildWithFixtures({
		manifest,
		transcriptClips: BUILT,
		// order 4 must not enter the Start-here strip; 'fanout-compare' (a card's tour) is deliberately not built
		tours: Object.fromEntries(BUILT.map((id, i) => [id, tourMdx({ title: `Tour ${id}`, clip: id, order: i + 1, steps: stepsFor(id) })])),
	});
	expect(build.status, build.output).toBe(0);
	landing = dom('index.html');
	features = dom('help', 'features', 'index.html');
}, 120_000);
afterAll(() => build?.cleanup());

describe('landing page', () => {
	it('hero_should_PlayTheRecordedClipAsWebmThenMp4WithAShotPoster', () => {
		const video = landing.querySelector('[data-testid="hero-video"]')!;
		const sources = [...video.querySelectorAll('source')].map((s) => [s.getAttribute('src'), s.getAttribute('type')]);
		expect(sources).toEqual([
			['/stapler-squad/clips/dashboard-lifecycle.webm', 'video/webm'],
			['/stapler-squad/clips/dashboard-lifecycle.mp4', 'video/mp4'],
		]);
		expect(video.getAttribute('poster')).toBe('/stapler-squad/shots/dashboard-lifecycle-poster.webp');
		expect(landing.documentElement.innerHTML).not.toContain('demo.webm');
	});

	it('seeTheTours_should_LinkToTheToursIndexBesideTheHeroCtas', () => {
		const link = [...landing.querySelectorAll('a')].find((a) => a.textContent!.trim() === 'See the tours')!;
		expect(link.getAttribute('href')).toBe('/stapler-squad/tours/');
	});

	it('startHere_should_ListToursOrderOneToThreeOnly', () => {
		const strip = landing.querySelector('[data-testid="tour-start-here"]')!;
		const hrefs = [...strip.querySelectorAll('a')].map((a) => a.getAttribute('href'));
		expect(hrefs).toEqual(BUILT.slice(0, 3).map((id) => `/stapler-squad/tours/${id}/`));
	});

	it('cards_should_LinkToTheirTourOnlyWhenThatTourIsBuilt', () => {
		const cardLink = (heading: string) =>
			[...features.querySelectorAll('[data-testid="feature-card"]')]
				.find((c) => c.querySelector('h4')!.textContent === heading)!
				.querySelector('[data-testid="feature-card-tour"]');
		expect(cardLink('Dashboard')?.textContent?.trim()).toBe('See the tour: Tour dashboard-lifecycle');
		expect(cardLink('Dashboard')?.getAttribute('href')).toBe('/stapler-squad/tours/dashboard-lifecycle/');
		expect(cardLink('Review Queue')?.getAttribute('href')).toBe('/stapler-squad/tours/approval-review/');
		// fanout-compare is not built here: no link, and no trace of the tour.
		expect(cardLink('Parallel Fan-Out')).toBeNull();
		expect(features.documentElement.innerHTML).not.toContain('fanout-compare');
	});

	it('featuresPage_should_StateTheCardCountFromFeaturesJson', async () => {
		const features19 = (await import('../../src/content/features.json')).default.length;
		expect(features.querySelector('main')!.textContent).toContain(`All ${features19} features`);
	});
});
