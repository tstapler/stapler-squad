// Stories 3.2.5 and 3.2.6: every feature card has a shot, every shot the recorder takes has a
// consumer, and the phone-figure shots are exactly the ones the tours reference.
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import yaml from 'js-yaml';
import { buildExpectedMedia } from '../../scripts/expected-media.mjs';
import features from '../../src/content/features.json';
import featureMedia from '../../src/content/feature-media.json';
import { CARD_SHOTS } from '../../../tests/e2e/website-demos/shotlists/cards.ts';
import { MOBILE_SHOTS, flagsEnabled as mobileFlags } from '../../../tests/e2e/website-demos/shotlists/mobile.ts';

const TOURS_DIR = join(import.meta.dirname, '..', '..', 'src', 'content', 'docs', 'tours');
const SCENES_DIR = join(import.meta.dirname, '..', '..', '..', 'tests', 'e2e', 'website-demos', 'tours');

interface Tour {
	id: string;
	phoneFigure?: string;
	shots: string[];
	features: string[];
}
const tours: Tour[] = readdirSync(TOURS_DIR)
	.filter((f) => f.endsWith('.mdx') && f !== 'index.mdx')
	.map((f) => {
		const fm = yaml.load(readFileSync(join(TOURS_DIR, f), 'utf8').match(/^---\r?\n([\s\S]*?)\r?\n---/)![1]) as { tour: { phoneFigure?: string; features: string[]; steps: { shot: string }[] } };
		return { id: f.replace(/\.mdx$/, ''), phoneFigure: fm.tour.phoneFigure, shots: fm.tour.steps.map((s) => s.shot), features: fm.tour.features };
	});

/** Shot names a tour scene file declares in its `shots:` array. */
function sceneShotNames(id: string): string[] {
	const src = readFileSync(join(SCENES_DIR, `${id}.ts`), 'utf8');
	const block = src.slice(src.indexOf('shots: ['));
	return [...block.matchAll(/name: '([^']+)',\s*route:/g)].map((m) => m[1]);
}

describe('feature card shots (Story 3.2.5)', () => {
	const media = featureMedia as Record<string, { shot: string; alt: string; tour?: string }>;

	it('every card in features.json has a feature-media.json entry with shot and alt', () => {
		expect(features.length).toBeGreaterThan(0);
		for (const card of features) {
			expect(media[card.heading], `${card.heading} has no feature-media.json entry`).toBeDefined();
			expect(media[card.heading].shot).toBeTruthy();
			expect(media[card.heading].alt.length).toBeGreaterThanOrEqual(20);
		}
		expect(Object.keys(media).sort()).toEqual(features.map((c) => c.heading).sort());
	});

	it('cards.ts records exactly the shot each card names, once', () => {
		const names = CARD_SHOTS.map((c) => c.spec.name);
		expect(new Set(names).size).toBe(names.length);
		for (const { heading, spec } of CARD_SHOTS) {
			expect(media[heading]?.shot, heading).toBe(spec.name);
		}
		expect(CARD_SHOTS.map((c) => c.heading).sort()).toEqual(features.map((c) => c.heading).sort());
	});

	it('every card tour link names a tour that exists', () => {
		const ids = new Set(tours.map((t) => `tours/${t.id}`).concat(tours.map((t) => t.id)));
		for (const [heading, entry] of Object.entries(media)) {
			if (entry.tour) expect(ids.has(entry.tour), `${heading} -> ${entry.tour}`).toBe(true);
		}
	});

	it('card shots use the desktop viewport', () => {
		for (const { spec } of CARD_SHOTS) expect(spec.formFactor ?? 'desktop').toBe('desktop');
	});
});

describe('mobile shot set (Story 3.2.6)', () => {
	const PHONE = {
		'dashboard-lifecycle': 'terminal-mobile',
		'organise-at-scale': 'dashboard-mobile',
		'approval-review': 'review-queue-mobile',
		'backlog-pipeline': 'backlog-mobile',
	} as const;

	it('exactly the four named tours set phoneFigure, to the named shots', () => {
		const withFigure = Object.fromEntries(tours.filter((t) => t.phoneFigure).map((t) => [t.id, t.phoneFigure]));
		expect(withFigure).toEqual(PHONE);
	});

	it('mobile.ts produces every phone figure, as 390x844 touch shots, once each', () => {
		const produced = MOBILE_SHOTS.map((s) => s.name).sort();
		expect(produced).toEqual(Object.values(PHONE).sort());
		for (const s of MOBILE_SHOTS) expect(s.formFactor, s.name).toBe('mobile');
	});

	it('each mobile shot is referenced by exactly one tour', () => {
		for (const name of MOBILE_SHOTS.map((s) => s.name)) {
			expect(tours.filter((t) => t.phoneFigure === name), name).toHaveLength(1);
		}
	});

	it('the backlog flag is enabled for the shotlist because backlog-mobile needs it', () => {
		expect(mobileFlags).toContain('backlog');
	});
});

describe('no orphan media (Story 3.2.5 consumer equality)', () => {
	it('every tour step shot is produced by its tour scene, and every scene shot is referenced', () => {
		for (const t of tours) {
			expect(sceneShotNames(t.id).sort(), t.id).toEqual([...t.shots].sort());
		}
	});

	it('everything the site consumes has a producer in the recorder (expected-media vs scenes, shotlists, legacy clips)', () => {
		const legacyClips = [...readFileSync(join(SCENES_DIR, '..', 'clips.ts'), 'utf8').matchAll(/^    name: '([^']+)',$/gm)].map((m) => m[1]);
		const produced = new Set<string>([
			...tours.flatMap((t) => [t.id, ...sceneShotNames(t.id)]),
			...legacyClips,
			...CARD_SHOTS.map((c) => c.spec.name),
			...MOBILE_SHOTS.map((s) => s.name),
		]);
		const expected = buildExpectedMedia().media.map((m) => m.name);
		expect(expected.filter((n) => !produced.has(n))).toEqual([]);
		expect(legacyClips.length).toBeGreaterThan(0);
	});
});
