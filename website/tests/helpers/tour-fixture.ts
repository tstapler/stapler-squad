// Builds the site with throwaway fixture tours in a hermetic scratch copy of
// website/ (real tours under src/content/docs/tours are removed from the copy, so
// their order numbers, clips and manifest rows can never collide with fixtures).
// node_modules is symlinked, not copied. The working tree is never written to.
import { spawnSync } from 'node:child_process';
import { cpSync, mkdirSync, mkdtempSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import type { MediaManifest } from '../../src/lib/tours/manifest.ts';

export const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
// Scratch lives under website/, not os.tmpdir(): Astro's normalizeFilename treats an absolute path that
// shares no ancestor beyond "/" with the site root (symlinked node_modules realpath vs /tmp on CI) as
// root-relative, so its compile-metadata lookup misses and the build fails.
const SCRATCH_DIR = '.tour-fixtures';
const COPY_SKIP = new Set(['node_modules', 'dist', '.astro', 'tests', '.vscode', SCRATCH_DIR]);

/** Copies the site sources into `dest` and drops every real tour (keeping tours/index.mdx). */
function copySite(dest: string): void {
	// Per entry, not one cpSync of the root: cpSync refuses a dest that sits inside its source.
	mkdirSync(dest, { recursive: true });
	for (const entry of readdirSync(WEBSITE_ROOT)) if (!COPY_SKIP.has(entry)) cpSync(join(WEBSITE_ROOT, entry), join(dest, entry), { recursive: true });
	symlinkSync(join(WEBSITE_ROOT, 'node_modules'), join(dest, 'node_modules'), 'dir');
	const tours = join(dest, 'src', 'content', 'docs', 'tours');
	for (const f of readdirSync(tours)) if (f !== 'index.mdx') rmSync(join(tours, f), { recursive: true, force: true });
	rmSync(join(dest, 'public', 'clips'), { recursive: true, force: true });
}

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
		caption: `Marker 1 shows thing ${i + 1}`,
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

export function manifestFor(opts: { clip?: string; shots?: string[]; durationMs?: number; commit?: string; recordedAt?: string; mobileShots?: string[]; poster?: boolean } = {}): MediaManifest {
	const { clip = FIXTURE_ID, shots = fixtureSteps().map((s) => s.shot), durationMs = 45_000, commit = 'a1b2c3d4e5f6', recordedAt = '2026-11-10T12:00:00Z', mobileShots = [], poster = true } = opts;
	return {
		commit,
		recordedAt,
		entries: [
			...(clip ? [{ name: clip, kind: 'clip' as const, formFactor: 'desktop' as const, format: 'webm', durationMs }] : []),
			...(clip && poster ? [{ name: `${clip}-poster`, kind: 'shot' as const, formFactor: 'desktop' as const, width: 1280, height: 720 }] : []),
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
	/** Removes the scratch site copy and its outDir. */
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
	mkdirSync(join(WEBSITE_ROOT, SCRATCH_DIR), { recursive: true });
	const scratch = mkdtempSync(join(WEBSITE_ROOT, SCRATCH_DIR, 'tour-fixture-'));
	const site = join(scratch, 'site');
	const outDir = join(scratch, 'dist');
	copySite(site);
	const toursDir = join(site, 'src', 'content', 'docs', 'tours');
	const clipsDir = join(site, 'public', 'clips');
	const put = (path: string, body: string) => {
		mkdirSync(join(path, '..'), { recursive: true });
		writeFileSync(path, body);
	};
	for (const [name, src] of Object.entries(tours)) put(join(toursDir, `${name}.mdx`), src);
	if (captions !== null) for (const c of transcriptClips) put(join(clipsDir, `${c}.transcript.json`), JSON.stringify(captions));
	for (const [file, bytes] of Object.entries(clipFiles)) put(join(clipsDir, file), 'x'.repeat(bytes));
	const manifestPath = join(scratch, 'media-manifest.json');
	if (manifest) writeFileSync(manifestPath, JSON.stringify(toWireManifest(manifest)));

	const { npm_config_user_agent: _ua, BASE_URL: _b, SITE: _s, ...env } = process.env;
	const result = spawnSync('bun', ['run', 'build', '--', '--outDir', outDir], {
		cwd: site,
		encoding: 'utf8',
		env: { ...env, TOURS_MEDIA_MODE: mode, MEDIA_MANIFEST_PATH: manifestPath },
	});
	return {
		outDir,
		status: result.status,
		output: `${result.stdout}\n${result.stderr}`,
		cleanup: () => {
			rmSync(scratch, { recursive: true, force: true });
		},
	};
}
