// R19, WCAG 1.2.1/1.2.3: the transcript lists every step narration then every caption, in order.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { JSDOM } from 'jsdom';
import { afterAll, describe, expect, it } from 'vitest';
import { buildWithFixtures, FIXTURE_ID, fixtureSteps, type FixtureBuild } from '../helpers/tour-fixture.ts';

const builds: FixtureBuild[] = [];
afterAll(() => builds.forEach((b) => b.cleanup()));

describe('tour transcript', () => {
	it('transcript_should_ContainNarrationThenCaptions_InOrder_When_Rendered', () => {
		const captions = [
			{ tMs: 1000, text: 'First caption' },
			{ tMs: 20000, text: 'Second caption' },
		];
		const b = buildWithFixtures({ captions });
		builds.push(b);
		expect(b.status, b.output).toBe(0);
		const doc = new JSDOM(readFileSync(join(b.outDir, 'tours', FIXTURE_ID, 'index.html'), 'utf8')).window.document;
		const details = doc.querySelector('details[data-testid="tour-transcript"]')!;
		expect(details.querySelector('summary')?.textContent).toBe('Transcript');
		const text = details.textContent!;
		const expected = [...fixtureSteps().map((s) => s.narration), ...captions.map((c) => c.text)];
		let at = -1;
		for (const piece of expected) {
			const next = text.indexOf(piece, at + 1);
			expect(next, `"${piece}" in order`).toBeGreaterThan(at);
			at = next;
		}
	}, 120_000);

	it('build_should_Fail_When_TranscriptJsonMissing', () => {
		const b = buildWithFixtures({ captions: null });
		builds.push(b);
		expect(b.status).not.toBe(0);
		expect(b.output).toContain('transcript for clip "zz-fixture" not found');
	}, 120_000);

	it('build_should_Fail_When_TranscriptJsonEmpty', () => {
		const b = buildWithFixtures({ captions: [] });
		builds.push(b);
		expect(b.status).not.toBe(0);
		expect(b.output).toContain('is empty');
	}, 120_000);
});
