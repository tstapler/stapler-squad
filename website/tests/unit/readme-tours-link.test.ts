// Story 3.3.2: README points at the tours (no embedded binary media), using the same origin and
// base as astro.config.mjs, and its Start-here links are the tours with order 1-3.
import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import yaml from 'js-yaml';
import { describe, expect, it } from 'vitest';
import { extractSiteUrls, sitePath } from '../../scripts/check-readme-links.mjs';
import { SITE_BASE, SITE_ORIGIN } from '../../src/site-config.mjs';

const ROOT = join(import.meta.dirname, '..', '..');
const README = readFileSync(join(ROOT, '..', 'README.md'), 'utf8');
const DOCS = join(ROOT, 'src', 'content', 'docs');
const TOURS = join(DOCS, 'tours');

const tours = readdirSync(TOURS)
	.filter((f) => f.endsWith('.mdx') && f !== 'index.mdx')
	.map((f) => ({ id: f.replace(/\.mdx$/, ''), tour: (yaml.load(readFileSync(join(TOURS, f), 'utf8').match(/^---\r?\n([\s\S]*?)\r?\n---/)![1]) as { tour: { order: number; outcome: string } }).tour }));

describe('README tours section', () => {
	it('embedsNoBinaryMedia', () => {
		expect(README).not.toMatch(/assets\/demo/);
		expect(README).not.toMatch(/assets\/screenshot/);
		expect(README).not.toMatch(/<video/);
	});

	it('linksTheToursIndexOnTheSameOriginAndBaseAsAstroConfig', () => {
		expect(README).toContain(`${SITE_ORIGIN}${SITE_BASE}/tours/)`);
	});

	it('hasThreeStartHereLinksInOrderWithTheTourOutcome', () => {
		const startHere = tours.filter((t) => t.tour.order <= 3).sort((a, b) => a.tour.order - b.tour.order);
		expect(startHere).toHaveLength(3);
		const bullets = README.split('\n').filter((l) => l.startsWith('- [') && l.includes(`${SITE_BASE}/tours/`) && !l.includes('[All tours]'));
		expect(bullets).toHaveLength(3);
		startHere.forEach((t, i) => {
			expect(bullets[i]).toContain(`${SITE_ORIGIN}${SITE_BASE}/tours/${t.id}/)`);
			expect(bullets[i]).toContain(t.tour.outcome);
		});
	});

	it('everyReadmeSiteUrlHasASourcePage', () => {
		for (const url of extractSiteUrls(README)) {
			const p = sitePath(url);
			expect(p, url).not.toBeNull();
			const rel = p!.replace(/^\/|\/$/g, '');
			const found = rel === '' || ['.mdx', '.md'].some((ext) => existsSync(join(DOCS, rel + ext)) || existsSync(join(DOCS, rel, `index${ext}`)));
			// help/* pages other than features are synced from docs/ at build time, so only tours and the features page are guaranteed here.
			if (rel.startsWith('tours') || rel === 'help/features') expect(found, url).toBe(true);
		}
	});
});
