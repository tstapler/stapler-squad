import { describe, it, expect } from 'vitest';
import { readFileSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { transformMarkdown } from '../../scripts/transform-markdown.mjs';
import { syncContent } from '../../scripts/sync-content.mjs';

const WEBSITE_DIR = dirname(dirname(fileURLToPath(import.meta.url)));
const REPO_ROOT = join(WEBSITE_DIR, '..', '..');
const HELP_DOCS_DIR = join(WEBSITE_DIR, 'src', 'content', 'docs', 'help');

describe('sync-content.mjs against real on-disk content', () => {
	it('syncContent_should_StripProjectPlansLinks_When_TransformingRealHowToFile', () => {
		// use-up-next-pr-tabs-and-ask-to-fix.md is the real docs/how-to/ file that
		// actually carries project_plans/ references (backtick-wrapped ADR paths,
		// not Markdown link syntax) among this epic's 11 included files.
		const sourcePath = 'docs/how-to/use-up-next-pr-tabs-and-ask-to-fix.md';
		const markdown = readFileSync(join(REPO_ROOT, sourcePath), 'utf8');
		expect(markdown).toContain('project_plans/');

		const { content } = transformMarkdown(markdown, {
			sourcePath,
			sourceDir: 'docs/how-to',
		});

		expect(content).not.toContain('project_plans/');
		expect(content).not.toContain('CLAUDE.md');
	});

	it('syncContent_should_Produce11CuratedDocsAndSkipRunNilSafety_When_RunAgainstFullManifest', () => {
		const results = syncContent({ check: true });

		expect(results).toHaveLength(11);
		const skippedPath = join(HELP_DOCS_DIR, 'run-nil-safety-analysis.md');
		expect(existsSync(skippedPath)).toBe(false);
		expect(results.some((r) => r.outputPath === skippedPath)).toBe(false);
	});
});
