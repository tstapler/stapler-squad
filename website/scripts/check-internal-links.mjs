#!/usr/bin/env node
// BrokenLinkCheck (Task 4.4.1b): walks every <a href> in the built site and
// confirms each internal link resolves to a real file in dist/ -- Starlight
// renders every CuratedDoc and the landing page as plain <a> tags, so a
// stale/typo'd slug (e.g. a cross-link to a doc that got excluded or renamed)
// would otherwise only surface as a live 404 after deploy.
//
// Run standalone: node website/scripts/check-internal-links.mjs
// (wired as a step in .github/workflows/deploy-website.yml, between "Build
// site" and "Check demo-asset health")

import { access, readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
export const DEFAULT_DIST_DIR = path.join(__dirname, '..', 'dist');
// Must match astro.config.mjs's `site`/`base` -- there is no runtime config
// introspection here, same tradeoff check-asset-health.mjs makes for its own
// hardcoded release-download base URL.
export const SITE_ORIGIN = 'https://tstapler.github.io';
export const SITE_BASE = '/stapler-squad';

async function findHtmlFiles(dir) {
	let entries;
	try {
		entries = await readdir(dir, { withFileTypes: true });
	} catch {
		return [];
	}

	const files = [];
	for (const entry of entries) {
		const fullPath = path.join(dir, entry.name);
		if (entry.isDirectory()) {
			files.push(...(await findHtmlFiles(fullPath)));
		} else if (entry.isFile() && entry.name.endsWith('.html')) {
			files.push(fullPath);
		}
	}
	return files;
}

// Matches `<a ...href="...">` tags only -- not <link>/<script> src attributes,
// which point at build-generated assets (CSS bundles, sitemap, favicon) with
// their own hashing/existence guarantees unrelated to CuratedDoc content links.
const ANCHOR_HREF_PATTERN = /<a\b[^>]*\shref="([^"]*)"/g;

export function extractAnchorHrefs(html) {
	const hrefs = [];
	for (const match of html.matchAll(ANCHOR_HREF_PATTERN)) {
		hrefs.push(match[1]);
	}
	return hrefs;
}

// Returns true for a link this checker should validate against dist/ --
// false for same-page anchors, external origins, and non-http(s) schemes.
export function isInternalLink(href, { base = SITE_BASE, origin = SITE_ORIGIN } = {}) {
	if (!href || href.startsWith('#')) return false;
	if (/^(mailto|tel|javascript):/i.test(href)) return false;
	if (href.startsWith('//')) return false; // protocol-relative -> external
	if (/^https?:\/\//i.test(href)) {
		// Only in-scope if it's a fully-qualified self-link back to this site's
		// own origin+base, checked as a real prefix match. A substring check
		// (e.g. `href.includes(base)`) misclassifies any third-party URL whose
		// path happens to contain "/stapler-squad" -- e.g. the GitHub social
		// link `https://github.com/tstapler/stapler-squad` itself -- as internal.
		return href.startsWith(`${origin}${base}`);
	}
	return true;
}

// Strips the query string and fragment, then maps an internal href to the
// dist/ file it should resolve to, trying Starlight's directory-with-index
// convention first, then a flat .html file, then an exact match.
export async function resolveInternalLink(href, { distDir = DEFAULT_DIST_DIR, base = SITE_BASE, origin = SITE_ORIGIN } = {}) {
	const withoutHash = href.split('#')[0].split('?')[0];
	let pathname = withoutHash;
	const fullOriginPrefix = `${origin}${base}`;
	if (pathname.startsWith(fullOriginPrefix)) {
		pathname = pathname.slice(fullOriginPrefix.length);
	} else if (pathname.startsWith(base)) {
		pathname = pathname.slice(base.length);
	}
	const trimmed = pathname.replace(/^\/+/, '').replace(/\/+$/, '');

	const candidates = trimmed === ''
		? [path.join(distDir, 'index.html')]
		: [
			path.join(distDir, trimmed, 'index.html'),
			path.join(distDir, `${trimmed}.html`),
			path.join(distDir, trimmed),
		];

	for (const candidate of candidates) {
		try {
			await access(candidate);
			return { resolved: true, path: candidate };
		} catch {
			// try next candidate
		}
	}
	return { resolved: false, path: null };
}

export async function checkInternalLinks({ distDir = DEFAULT_DIST_DIR, base = SITE_BASE } = {}) {
	const htmlFiles = await findHtmlFiles(distDir);
	const failures = [];
	let checked = 0;

	for (const file of htmlFiles) {
		const html = await readFile(file, 'utf8');
		const hrefs = extractAnchorHrefs(html);
		for (const href of hrefs) {
			if (!isInternalLink(href, { base })) continue;
			checked += 1;
			const { resolved } = await resolveInternalLink(href, { distDir, base });
			if (!resolved) {
				failures.push({ href, sourceFile: path.relative(distDir, file) });
			}
		}
	}

	return { failures, checked };
}

async function main() {
	const { failures, checked } = await checkInternalLinks();

	for (const failure of failures) {
		console.error(`[check-internal-links] FAIL ${failure.href} (linked from ${failure.sourceFile})`);
	}

	if (failures.length > 0) {
		console.error(
			`[check-internal-links] ${failures.length} of ${checked} internal link(s) are broken`,
		);
		process.exitCode = 1;
		return;
	}

	console.log(`[check-internal-links] ${checked} internal link(s) OK`);
}

// Only run when invoked directly (node check-internal-links.mjs), not when
// imported by tests.
if (import.meta.url === `file://${process.argv[1]}`) {
	await main();
}
