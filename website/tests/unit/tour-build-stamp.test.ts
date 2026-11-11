// Cross-cutting 8g: one build-stamp format, on tour pages and the /tours/ index.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { buildWithFixtures, FIXTURE_ID, manifestFor, type FixtureBuild } from '../helpers/tour-fixture.ts';

let build: FixtureBuild;
beforeAll(() => {
	build = buildWithFixtures({ manifest: manifestFor({ commit: 'a1b2c3d9999', recordedAt: '2026-11-10T08:00:00Z' }) });
	expect(build.status, build.output).toBe(0);
}, 120_000);
afterAll(() => build?.cleanup());

describe('build stamp', () => {
	for (const [label, parts] of [['tour page', ['tours', FIXTURE_ID, 'index.html']], ['tours index', ['tours', 'index.html']]] as const) {
		it(`stamp_should_UseDateFirstFormatWithShaInSmall_On_${label.replace(' ', '_')}`, () => {
			const html = readFileSync(join(build.outDir, ...parts), 'utf8');
			expect(html).toContain('Recorded 2026-11-10 (build <small>a1b2c3d</small>)');
			expect(html).not.toContain('Recorded from build');
			expect(html).toContain("Recorded with sample data, not a real team's work.");
		});
	}

	it('tourPage_should_NotShowRecordedFromBuild_Anywhere_InDist', () => {
		for (const parts of [['index.html'], ['tours', FIXTURE_ID, 'index.html']]) {
			expect(readFileSync(join(build.outDir, ...parts), 'utf8')).not.toContain('Recorded from build');
		}
	});
});
