// Story 4.1.1: getting-started.md must cover README's three install paths
// (Homebrew, manual install script, build from source) 1:1, with no 4th
// path invented. Same drift-guard pattern as hero.test.ts/features-md.test.ts:
// read the real README.md and compare, don't hardcode expected strings twice.
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
	extractBuildFromSourceCommand,
	extractInstallCommand,
	extractManualInstallCommand,
} from '../../src/lib/readme-copy.ts';

const ENCODING = 'utf8';
// kibitzer:ignore replace-magic-literal -- '..' is the literal parent-directory
// path segment used to walk up from this test file to website/ and repo root.
const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const REPO_ROOT = join(WEBSITE_ROOT, '..');
const README_PATH = join(REPO_ROOT, 'README.md');
const GETTING_STARTED_PATH = join(WEBSITE_ROOT, 'src', 'content', 'docs', 'help', 'getting-started.md');

const EXPECTED_SUBSECTIONS = ['Homebrew', 'Manual install script', 'Build from source'];

describe('getting-started.md', () => {
	it('gettingStartedMd_should_Contain3InstallSubsections_When_Parsed', async () => {
		const readme = await readFile(README_PATH, ENCODING);
		const gettingStarted = await readFile(GETTING_STARTED_PATH, ENCODING);

		for (const heading of EXPECTED_SUBSECTIONS) {
			expect(gettingStarted).toContain(`## ${heading}`);
		}

		expect(gettingStarted).toContain(extractInstallCommand(readme));
		expect(gettingStarted).toContain(extractManualInstallCommand(readme));
		expect(gettingStarted).toContain(extractBuildFromSourceCommand(readme));

		// Each subsection has its own copyable command block -- 3 headings, >=3 fenced blocks.
		const bashBlockCount = (gettingStarted.match(/```bash/g) ?? []).length;
		expect(bashBlockCount).toBeGreaterThanOrEqual(EXPECTED_SUBSECTIONS.length);
	});

	it('gettingStartedMd_should_NotContainInstallPathAbsentFromReadme_When_ComparedAgainstSource', async () => {
		const gettingStarted = await readFile(GETTING_STARTED_PATH, ENCODING);

		// Regression guard: exactly the 3 README-sourced subsections, never a
		// 4th invented install path (e.g. "## Docker", "## npm install", ...).
		const h2Headings = [...gettingStarted.matchAll(/^## (.+)$/gm)].map((match) => match[1].trim());
		expect(h2Headings).toEqual(EXPECTED_SUBSECTIONS);
	});
});
