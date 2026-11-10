// WCAG 2.5.3 / Cross-cutting 8f: accessible name equals visible text; links with
// the same name never point to different targets.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { JSDOM } from 'jsdom';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { buildWithFixtures, fixtureSteps, manifestFor, tourMdx, type FixtureBuild } from '../helpers/tour-fixture.ts';

const A = 'zz-a';
const B = 'zz-b';
const C = 'zz-c';
let build: FixtureBuild;
let pageB: Document;
let index: Document;

const stepsFor = (id: string) => fixtureSteps(3).map((s) => ({ ...s, shot: `${id}-${s.shot}` }));
const dom = (...p: string[]) => new JSDOM(readFileSync(join(build.outDir, ...p), 'utf8')).window.document;

beforeAll(() => {
	const ids = [A, B, C];
	const manifest = manifestFor({ clip: '', shots: ids.flatMap((id) => stepsFor(id).map((s) => s.shot)) });
	manifest.entries.push(...ids.map((id) => ({ name: id, kind: 'clip' as const, durationMs: 30_000 })));
	build = buildWithFixtures({
		manifest,
		transcriptClips: ids,
		tours: Object.fromEntries(ids.map((id, i) => [id, tourMdx({ title: `Tour ${id}`, clip: id, order: i + 1, steps: stepsFor(id) })])),
	});
	expect(build.status, build.output).toBe(0);
	pageB = dom('tours', B, 'index.html');
	index = dom('tours', 'index.html');
}, 120_000);
afterAll(() => build?.cleanup());

const tourLinks = (doc: Document) => [...doc.querySelectorAll('.tour-page a, .tour-card a')] as HTMLAnchorElement[];

describe('tour link names', () => {
	it('prevNextAndSeeTheTour_should_ShowFullLabelAsVisibleText_WithNoAriaLabel', () => {
		const labelled = [...pageB.querySelectorAll('[data-testid="tour-prevnext"] a'), ...index.querySelectorAll('.tour-card a')] as HTMLAnchorElement[];
		const texts = labelled.map((a) => a.textContent!.trim());
		expect(texts).toContain(`Previous tour: Tour ${A}`);
		expect(texts).toContain(`Next tour: Tour ${C}`);
		expect(texts.some((t) => t.startsWith('See the tour: '))).toBe(true);
		for (const a of labelled) expect(a.hasAttribute('aria-label')).toBe(false);
	});

	it('links_should_NotShareAnAccessibleNameAcrossDifferentTargets', () => {
		for (const doc of [pageB, index]) {
			const seen = new Map<string, string>();
			for (const a of tourLinks(doc)) {
				const name = a.getAttribute('aria-label') ?? a.textContent!.trim();
				const href = a.getAttribute('href')!;
				if (seen.has(name)) expect(seen.get(name), `"${name}"`).toBe(href);
				seen.set(name, href);
			}
		}
	});

	it('startHereStrip_should_BeDerivedFromOrderOneToThree', () => {
		const strip = index.querySelector('[data-testid="tour-start-here"]')!;
		expect(strip.querySelectorAll('a')).toHaveLength(3);
		expect(strip.querySelector('h2')?.textContent).toBe('Start here');
	});
});
