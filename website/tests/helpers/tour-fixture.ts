// Builds the real site with a throwaway fixture tour (Epic 3.2 owns real tour
// content). Fixture files use the `zz-fixture` prefix, are written into the
// source tree only for the duration of the build, and are always removed. The
// manifest lives in a scratch dir and is passed via MEDIA_MANIFEST_PATH, so
// nothing under public/ besides the zz-fixture clip files is touched.
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { MediaManifest } from '../../src/lib/tours/manifest.ts';

export const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const TOURS_DIR = join(WEBSITE_ROOT, 'src', 'content', 'docs', 'tours');
const CLIPS_DIR = join(WEBSITE_ROOT, 'public', 'clips');

export const FIXTURE_ID = 'zz-fixture';

export interface FixtureStep {
	text: string;
	shot: string;
	alt: string;
	caption: string;
	narration: string;
	why?: string;
}

export function fixtureSteps(count = 3, overrides: Partial<FixtureStep> = {}): FixtureStep[] {
	return Array.from({ length: count }, (_, i) => ({
		text: `Do thing ${i + 1}`,
		shot: `${FIXTURE_ID}-${i + 1}`,
		alt: `Screenshot of step ${i + 1}`,
		caption: `Caption ${i + 1}`,
		narration: `On screen, thing ${i + 1} happens`,
		...overrides,
	}));
}

export function tourMdx(opts: { id?: string; clip?: string; helpDoc?: string; order?: number; steps?: unknown[]; phoneFigure?: string; title?: string } = {}): string {
	const { title = 'Fixture tour', clip = FIXTURE_ID, helpDoc = 'help/getting-started', order = 1, steps = fixtureSteps(), phoneFigure } = opts;
	const frontmatter = {
		title,
		description: 'Fixture tour for tests',
		tour: {
			title,
			outcome: 'See the fixture work end to end',
			clip,
			features: ['dashboard'],
			helpDoc,
			order,
			category: 'Visibility',
			audience: 'Anyone',
			durationMin: 2,
			...(phoneFigure ? { phoneFigure } : {}),
			steps,
		},
	};
	return `---\n${JSON.stringify(frontmatter, null, 2)}\n---\n`;
}

export function manifestFor(opts: { clip?: string; shots?: string[]; durationMs?: number; commit?: string; recordedAt?: string; mobileShots?: string[] } = {}): MediaManifest {
	const { clip = FIXTURE_ID, shots = fixtureSteps().map((s) => s.shot), durationMs = 45_000, commit = 'a1b2c3d4e5f6', recordedAt = '2026-11-10T12:00:00Z', mobileShots = [] } = opts;
	return {
		commit,
		recordedAt,
		entries: [
			...(clip ? [{ name: clip, kind: 'clip' as const, formFactor: 'desktop' as const, format: 'webm', durationMs }] : []),
			...shots.map((name) => ({ name, kind: 'shot' as const, formFactor: 'desktop' as const, width: 1280, height: 720 })),
			...mobileShots.map((name) => ({ name, kind: 'shot' as const, formFactor: 'mobile' as const, width: 390, height: 844 })),
		],
	};
}

/** The recorder/Go shape on disk (one row per rendition), so builds exercise the real parser. */
export function toWireManifest(m: MediaManifest) {
	const base = { status: 'recorded', bytes: 1000, commit: m.commit, themeId: 'grimdark' };
	return {
		version: 1,
		commit: m.commit,
		startedAt: m.recordedAt,
		finishedAt: m.recordedAt,
		entries: m.entries.flatMap((e) =>
			e.kind === 'clip'
				? (['webm', 'mp4'] as const).map((format) => ({ ...base, name: e.name, format, src: `clips/${e.name}.${format}`, durationMs: e.durationMs, formFactor: e.formFactor }))
				: [{ ...base, name: e.name, format: 'webp', src: `shots/${e.name}.webp`, formFactor: e.formFactor, width: e.width, height: e.height }],
		),
	};
}

export interface FixtureBuild {
	outDir: string;
	status: number | null;
	output: string;
	/** Removes the scratch outDir (source fixtures are already gone after the build). */
	cleanup: () => void;
}

/** Runs `astro build` into a scratch outDir. `tours` maps file name (no ext) to mdx source. */
export function buildWithFixtures(opts: {
	mode?: 'deploy' | 'smoke';
	manifest?: MediaManifest | null;
	tours?: Record<string, string>;
	captions?: unknown;
	/** Clips that get a transcript file (default: [clipName]). */
	transcriptClips?: string[];
	clipName?: string;
	/** public/clips files to create (name -> byte size), e.g. stub renditions. */
	clipFiles?: Record<string, number>;
}): FixtureBuild {
	const { mode = 'deploy', manifest = manifestFor(), tours = { [FIXTURE_ID]: tourMdx() }, captions = [{ tMs: 1000, text: 'Caption one' }, { tMs: 9000, text: 'Caption two' }], clipName = FIXTURE_ID, transcriptClips = [clipName], clipFiles = {} } = opts;
	const scratch = mkdtempSync(join(tmpdir(), 'tour-fixture-'));
	const outDir = join(scratch, 'dist');
	const written: string[] = [];
	const put = (path: string, body: string) => {
		mkdirSync(join(path, '..'), { recursive: true });
		writeFileSync(path, body);
		written.push(path);
	};
	for (const [name, src] of Object.entries(tours)) put(join(TOURS_DIR, `${name}.mdx`), src);
	if (captions !== null) for (const c of transcriptClips) put(join(CLIPS_DIR, `${c}.transcript.json`), JSON.stringify(captions));
	for (const [file, bytes] of Object.entries(clipFiles)) put(join(CLIPS_DIR, file), 'x'.repeat(bytes));
	const manifestPath = join(scratch, 'media-manifest.json');
	if (manifest) writeFileSync(manifestPath, JSON.stringify(toWireManifest(manifest)));

	const { npm_config_user_agent: _ua, BASE_URL: _b, SITE: _s, ...env } = process.env;
	const result = spawnSync('bun', ['run', 'build', '--', '--outDir', outDir], {
		cwd: WEBSITE_ROOT,
		encoding: 'utf8',
		env: { ...env, TOURS_MEDIA_MODE: mode, MEDIA_MANIFEST_PATH: manifestPath },
	});
	// Sources are only needed during the build; remove them now so builds can't leak into each other.
	for (const f of written) rmSync(f, { force: true });
	try {
		rmdirSync(CLIPS_DIR);
	} catch {
		// not empty: real clips live here too
	}
	return {
		outDir,
		status: result.status,
		output: `${result.stdout}\n${result.stderr}`,
		cleanup: () => {
			rmSync(scratch, { recursive: true, force: true });
		},
	};
}
