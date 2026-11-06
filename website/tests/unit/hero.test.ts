// Story 3.1.1: hero tagline + install CTA copy must be lifted from README.md,
// not reworded (design/ux.md §2's "don't invent new copy" instruction).
//
// index.mdx's hero.tagline is Starlight frontmatter rendered by its Hero.astro
// component via `Astro.locals.starlightRoute`, which only exists inside a real
// route render -- not available to a bare `experimental_AstroContainer`
// render of the page in isolation. So the tagline check reads index.mdx's raw
// text directly (same pattern as features-md.test.ts's regression guards)
// rather than attempting a full-page container render. InstallCommand.astro
// itself has no such dependency, so its check is a real container render.
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { experimental_AstroContainer as AstroContainer } from 'astro/container';
import InstallCommand from '../../src/components/InstallCommand.astro';
import { extractValueProp, extractInstallCommand } from '../../src/lib/readme-copy.ts';

const ENCODING = 'utf8';
// kibitzer:ignore replace-magic-literal -- '..' is the literal parent-directory
// path segment used to walk up from this test file to website/ and repo root.
const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const REPO_ROOT = join(WEBSITE_ROOT, '..');
const README_PATH = join(REPO_ROOT, 'README.md');
const INDEX_MDX_PATH = join(WEBSITE_ROOT, 'src', 'content', 'docs', 'index.mdx');

describe('SplashPage hero', () => {
	it('splashPageHero_should_ContainReadmeValuePropVerbatim_When_Rendered', async () => {
		const readme = await readFile(README_PATH, ENCODING);
		const indexMdx = await readFile(INDEX_MDX_PATH, ENCODING);
		const valueProp = extractValueProp(readme);

		expect(indexMdx).toContain(valueProp);
	});

	it('installCommandBlock_should_NotDriftFromReadme_When_CommandTextCompared', async () => {
		const readme = await readFile(README_PATH, ENCODING);
		const indexMdx = await readFile(INDEX_MDX_PATH, ENCODING);
		const expectedCommand = extractInstallCommand(readme);

		const attrMatch = indexMdx.match(/<InstallCommand command="([^"]+)"/);
		expect(attrMatch, 'index.mdx must pass a literal command="..." prop to <InstallCommand>').not.toBeNull();
		const wiredCommand = attrMatch![1];
		expect(wiredCommand).toBe(expectedCommand);

		const container = await AstroContainer.create();
		const html = await container.renderToString(InstallCommand, { props: { command: wiredCommand } });

		// Astro HTML-escapes `&` inside text content, so compare against the
		// same escaping rather than the raw command string.
		expect(html).toContain(expectedCommand.replace(/&/g, '&amp;'));
	});
});
