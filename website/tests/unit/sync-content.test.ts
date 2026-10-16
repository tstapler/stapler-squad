import { describe, it, expect, afterEach } from 'vitest';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { syncContent } from '../../scripts/sync-content.mjs';

const ENCODING = 'utf8';
const HOW_TO_DIR = join('docs', 'how-to');
const HELP_DIR = join('website', 'src', 'content', 'docs', 'help');

/**
 * Builds an isolated fixture repo: <root>/docs/how-to/<name> for the
 * SourceDoc and <root>/website/src/content/docs/help/ as the sync target,
 * matching sync-content.mjs's real repo-root-relative layout assumptions.
 */
function makeFixtureRepo() {
	const root = mkdtempSync(join(tmpdir(), 'sync-content-test-'));
	mkdirSync(join(root, HOW_TO_DIR), { recursive: true });
	mkdirSync(join(root, HELP_DIR), { recursive: true });
	return root;
}

function writeManifest(root: string, entries: unknown[]) {
	const manifestPath = join(root, 'content-manifest.json');
	writeFileSync(manifestPath, JSON.stringify(entries, null, 2), ENCODING);
	return manifestPath;
}

function writeSourceDoc(root: string, name: string, content: string) {
	writeFileSync(join(root, HOW_TO_DIR, name), content, ENCODING);
}

/** Sets up the "one valid include:true entry" fixture shared by the happy-path test. */
function setUpValidEntryFixture(root: string) {
	writeSourceDoc(
		root,
		'debug-with-logs.md',
		'# Debug with Logs\n\nThis fixture doc explains how to debug with logs.\n',
	);
	return writeManifest(root, [
		{ source: 'docs/how-to/debug-with-logs.md', include: true, slug: 'help/debug-with-logs' },
	]);
}

describe('sync-content.mjs', () => {
	const cleanupDirs: string[] = [];

	afterEach(() => {
		for (const dir of cleanupDirs.splice(0)) {
			rmSync(dir, { recursive: true, force: true });
		}
	});

	it('syncContent_should_CreateCuratedDocWithDerivedTitle_When_ManifestEntryIncludesValidSource', () => {
		const root = makeFixtureRepo();
		cleanupDirs.push(root);
		const manifestPath = setUpValidEntryFixture(root);

		const results = syncContent({ manifestPath, repoRoot: root });

		expect(results).toHaveLength(1);
		const outputPath = join(root, HELP_DIR, 'debug-with-logs.md');
		expect(results[0].outputPath).toBe(outputPath);
		const written = readFileSync(outputPath, ENCODING);
		expect(written).toContain("title: 'Debug with Logs'");
		expect(written).toMatch(/description: '.+'/);
	});

	it('syncContent_should_ExitNonZeroAndPrintPath_When_SourceDocMissing', () => {
		const root = makeFixtureRepo();
		cleanupDirs.push(root);
		const manifestPath = writeManifest(root, [
			{ source: 'docs/how-to/does-not-exist.md', include: true, slug: 'help/x' },
		]);

		expect(() => syncContent({ manifestPath, repoRoot: root })).toThrowError(/does-not-exist\.md/);
	});
});
