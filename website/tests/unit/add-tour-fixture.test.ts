// Story 3.1.1 "Adding a tour is one spec plus one entry" (R13, Task 3.1.1g): a temporary
// tour .mdx + scene .ts pair, and no other file edit, shows up in expected-media output and in
// exactly one `mediatool shard-plan` shard. The pair is added to copies of the real tour
// dirs in a scratch directory, so the working tree is never written.
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { buildExpectedMedia, DEFAULT_FEATURE_MEDIA, DEFAULT_TOURS_DIR } from '../../scripts/expected-media.mjs';
import { fixtureSteps, tourMdx } from '../helpers/tour-fixture.ts';

const REPO_ROOT = join(import.meta.dirname, '..', '..', '..');
const SCENES_DIR = join(REPO_ROOT, 'tests', 'e2e', 'website-demos', 'tours');
const SHOTLISTS_DIR = join(REPO_ROOT, 'tests', 'e2e', 'website-demos', 'shotlists');
const SHARD_PLAN_GO = join(REPO_ROOT, 'tools', 'promo', 'cmd', 'mediatool', 'shardplan.go');
const ID = 'zz-add-tour';
const SHARDS = 3;

interface Shard {
	tours: string[];
}

describe.skipIf(!existsSync(SHARD_PLAN_GO))('add a tour with one spec and one mdx', () => {
	let scratch = '';
	let toursDir = '';
	let scenesDir = '';
	let mediatool = '';

	const plan = (scenes: string): Shard[] => {
		const r = spawnSync(mediatool, ['shard-plan', '-tours-dir', scenes, '-shotlists-dir', SHOTLISTS_DIR, '-shards', String(SHARDS)], { encoding: 'utf8' });
		expect(r.status, r.stderr).toBe(0);
		return (JSON.parse(r.stdout) as { include: Shard[] }).include;
	};

	beforeAll(() => {
		scratch = mkdtempSync(join(tmpdir(), 'add-tour-'));
		toursDir = join(scratch, 'tours');
		scenesDir = join(scratch, 'scenes');
		cpSync(DEFAULT_TOURS_DIR, toursDir, { recursive: true });
		cpSync(SCENES_DIR, scenesDir, { recursive: true });
		mediatool = join(scratch, 'mediatool');
		const build = spawnSync('go', ['build', '-o', mediatool, './tools/promo/cmd/mediatool'], { cwd: REPO_ROOT, encoding: 'utf8' });
		expect(build.status, build.stderr).toBe(0);
	}, 240_000);

	afterAll(() => {
		if (scratch) rmSync(scratch, { recursive: true, force: true });
	});

	it('newTourId_should_AppearInExpectedMediaAndExactlyOneShard_When_PairAddedInFixtureDir', () => {
		const options = { toursDir, featureMediaPath: DEFAULT_FEATURE_MEDIA };
		const names = () => buildExpectedMedia(options).media.map((m: { name: string }) => m.name);
		const beforeNames = names();
		const beforeTours = plan(scenesDir).flatMap((s) => s.tours);
		expect(beforeNames).not.toContain(ID);
		expect(beforeTours).not.toContain(ID);

		const steps = fixtureSteps(3).map((s, i) => ({ ...s, shot: `${ID}-${i + 1}` }));
		writeFileSync(join(toursDir, `${ID}.mdx`), tourMdx({ clip: ID, steps }));
		mkdirSync(scenesDir, { recursive: true });
		writeFileSync(join(scenesDir, `${ID}.ts`), `export const scene = { name: '${ID}' };\n`);

		const media = buildExpectedMedia(options).media as { name: string; formats?: string[] }[];
		expect(media.find((m) => m.name === ID)?.formats).toEqual(['mp4', 'webm']);
		for (const n of [1, 2, 3]) expect(names()).toContain(`${ID}-${n}`);

		const shards = plan(scenesDir);
		expect(shards.filter((s) => s.tours.includes(ID))).toHaveLength(1);
		expect(shards.flatMap((s) => s.tours).sort()).toEqual([...beforeTours, ID].sort());
	}, 60_000);
});
