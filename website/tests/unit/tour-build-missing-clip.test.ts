// Cross-cutting 6c: the two media modes. Deploy fails on missing media; smoke
// does not build an unrecorded tour at all.
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { afterAll, describe, expect, it } from 'vitest';
import { buildWithFixtures, fixtureSteps, manifestFor, tourMdx, type FixtureBuild } from '../helpers/tour-fixture.ts';

const builds: FixtureBuild[] = [];
const build = (opts: Parameters<typeof buildWithFixtures>[0]) => {
	const b = buildWithFixtures(opts);
	builds.push(b);
	return b;
};
afterAll(() => builds.forEach((b) => b.cleanup()));
const read = (b: FixtureBuild, ...p: string[]) => readFileSync(join(b.outDir, ...p), 'utf8');

describe('TOURS_MEDIA_MODE=deploy', () => {
	it('build_should_FailNamingClip_When_ClipAbsentFromManifest', () => {
		const b = build({ manifest: manifestFor({ clip: '' }) });
		expect(b.status).not.toBe(0);
		expect(b.output).toContain('clip "zz-fixture" is absent from media-manifest.json');
	}, 120_000);

	it('build_should_FailNamingPoster_When_PosterShotAbsentFromManifest', () => {
		const b = build({ manifest: manifestFor({ poster: false }) });
		expect(b.status).not.toBe(0);
		expect(b.output).toContain('poster shot "zz-fixture-poster" is absent from media-manifest.json');
	}, 120_000);

	it('build_should_FailNamingShot_When_StepShotAbsentFromManifest', () => {
		const b = build({ manifest: manifestFor({ shots: ['zz-fixture-1', 'zz-fixture-2'] }) });
		expect(b.status).not.toBe(0);
		expect(b.output).toContain('tour shot "zz-fixture-3"');
	}, 120_000);

	it('build_should_FailNamingFile_When_ManifestMissing', () => {
		const b = build({ manifest: null });
		expect(b.status).not.toBe(0);
		expect(b.output).toContain('media-manifest.json not found');
	}, 120_000);

	it('build_should_FailNamingCap_When_ClipLongerThanSixtySeconds', () => {
		const b = build({ manifest: manifestFor({ durationMs: 61_000 }) });
		expect(b.status).not.toBe(0);
		expect(b.output).toContain('over the 60000 ms cap');
	}, 120_000);
});

describe('TOURS_MEDIA_MODE=smoke', () => {
	const SMOKE = 'zz-smoke';
	const UNREC = 'zz-unrecorded';
	const steps = fixtureSteps(3);
	const tours = {
		[SMOKE]: tourMdx({ clip: SMOKE, order: 1 }),
		[UNREC]: tourMdx({ clip: UNREC, order: 2, steps: fixtureSteps(3).map((s) => ({ ...s, shot: `${UNREC}-${s.shot}` })) }),
	};
	// The recorded tour has only its first two shots: the third must render no figure.
	const manifest = manifestFor({ clip: SMOKE, shots: [steps[0].shot, steps[1].shot] });

	it('smoke_should_NotBuildUnrecordedTour_And_LeaveNoTraceOfIt', () => {
		const b = build({ mode: 'smoke', manifest, tours, clipName: SMOKE });
		expect(b.status, b.output).toBe(0);
		expect(existsSync(join(b.outDir, 'tours', UNREC))).toBe(false);
		expect(existsSync(join(b.outDir, 'tours', SMOKE, 'index.html'))).toBe(true);
		const index = read(b, 'tours', 'index.html');
		const page = read(b, 'tours', SMOKE, 'index.html');
		for (const html of [index, page, read(b, 'index.html')]) {
			expect(html).not.toContain(UNREC);
			expect(html).not.toMatch(/coming soon/i);
		}
		// Only tour: no prev, next becomes "Back to all tours".
		expect(page).not.toContain('Previous tour:');
		expect(page).toContain('Back to all tours');
		// Built tour: the unrecorded step shot renders no figure, text stays.
		expect(page.match(/data-testid="tour-shot"/g)).toHaveLength(2);
		expect(page).toContain('Do thing 3');
	}, 120_000);

	it('smoke_should_FailTheBuild_When_NoTourIsRecorded', () => {
		const b = build({ mode: 'smoke', manifest: manifestFor({ clip: 'other-clip' }), tours, clipName: SMOKE });
		expect(b.status).not.toBe(0);
		expect(b.output).toContain('built zero tours');
	}, 120_000);

	it('metaRow_should_OmitWatchLabel_When_ManifestHasNoDuration', () => {
		const m = manifestFor({ clip: SMOKE, shots: steps.map((s) => s.shot) });
		delete m.entries[0].durationMs;
		const b = build({ mode: 'smoke', manifest: m, tours: { [SMOKE]: tours[SMOKE] }, clipName: SMOKE });
		expect(b.status, b.output).toBe(0);
		expect(read(b, 'tours', SMOKE, 'index.html')).not.toContain('Watch ');
	}, 120_000);
});
