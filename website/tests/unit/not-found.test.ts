// Story 4.4.2: src/pages/404.astro must render the site's own chrome (not a
// bare page), via Starlight's own <StarlightPage> wrapper.
//
// <StarlightPage> depends on Astro.locals.starlightRoute, populated by
// middleware that resolves the page against the synced "docs" content
// collection (same constraint hero.test.ts documents for Hero.astro) --
// confirmed by spiking a bare `experimental_AstroContainer` render, which
// throws because the collection isn't synced in that isolated environment.
// So the first test reads 404.astro's raw source (fast, no build needed);
// the second builds the real site once and checks the rendered dist/404.html,
// including that its asset/link URLs are base-path-absolute rather than
// relative -- the actual risk for "does this still render right when a static
// host serves it for a deeply nested unmatched path" (a relative URL would
// resolve against the wrong directory at a different depth; an absolute one
// resolves identically regardless of which path 404'd).
import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { beforeAll, describe, expect, it } from 'vitest';
import { extractAnchorHrefs, isInternalLink } from '../../scripts/check-internal-links.mjs';
import { runRealBuild } from '../helpers/run-build.ts';

const ENCODING = 'utf8';
const WEBSITE_ROOT = join(import.meta.dirname, '..', '..');
const NOT_FOUND_SOURCE_PATH = join(WEBSITE_ROOT, 'src', 'pages', '404.astro');
const REAL_DIST_404_PATH = join(WEBSITE_ROOT, 'dist', '404.html');

describe('404.astro', () => {
	beforeAll(() => {
		runRealBuild(WEBSITE_ROOT);
	}, 120_000);

	it('notFoundPage_should_RenderSiteHeaderNavAndDocsHomeLink_When_Rendered', async () => {
		const source = await readFile(NOT_FOUND_SOURCE_PATH, ENCODING);

		// Site chrome comes from Starlight's own Page.astro, not a hand-built lookalike.
		expect(source).toMatch(/import\s+StarlightPage\s+from\s+['"]@astrojs\/starlight\/components\/StarlightPage\.astro['"]/);
		expect(source).toContain("title: 'Page not found'");
		expect(source).toContain('Go to the docs home');
	});

	it('notFoundPage_should_RenderConsistently_When_ReachedViaDeeplyNestedUnmatchedPath', async () => {
		const html = await readFile(REAL_DIST_404_PATH, ENCODING);

		expect(html).toContain('Page not found');
		expect(html).toContain('Go to the docs home');
		// Site header (search box, GitHub social link) renders -- not GitHub
		// Pages' bare default 404 HTML.
		expect(html).toContain('github.com/tstapler/stapler-squad');

		// Every internal <a> link is base-path-absolute (the same scoping
		// check-internal-links.mjs uses). A relative URL (e.g. "../help/") would
		// resolve against whatever deep path 404'd, not the site root, and break
		// when GitHub Pages serves this one static file for an arbitrarily
		// nested unmatched path.
		const internalHrefs = extractAnchorHrefs(html).filter((href) => isInternalLink(href));
		expect(internalHrefs.length).toBeGreaterThan(0);
		for (const href of internalHrefs) {
			expect(href.startsWith('/stapler-squad/'), `expected an absolute base-path URL, got "${href}"`).toBe(true);
		}
	});
});
