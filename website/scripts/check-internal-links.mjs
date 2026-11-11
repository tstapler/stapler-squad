#!/usr/bin/env node
// BrokenLinkCheck (Task 4.4.1b): walks every <a href> in the built site and
// confirms each internal link resolves to a real file in dist/ -- Starlight
// renders every CuratedDoc and the landing page as plain <a> tags, so a
// stale/typo'd slug (e.g. a cross-link to a doc that got excluded or renamed)
// would otherwise only surface as a live 404 after deploy.
//
// It also checks media (img/video/source) references, so a missing poster or
// clip fails here too.
//
// Run standalone: node website/scripts/check-internal-links.mjs
// (wired as a step in .github/workflows/deploy-website.yml, between "Build
// site" and "Check demo-asset health")

import { access, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { findHtmlFiles } from '../src/lib/release-asset.ts';
import { SITE_BASE, SITE_ORIGIN } from '../src/site-config.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
export const DEFAULT_DIST_DIR = path.join(__dirname, '..', 'dist');
// Re-exported for transform-markdown.mjs, which imports SITE_BASE from this
// module rather than site-config.mjs directly.
export { SITE_BASE, SITE_ORIGIN };

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

// Media references (Story 3.1.3): <img src>, <video src|poster>, <source src>.
// A tour page whose poster or clip file is missing from dist/ must fail here,
// not 404 after deploy. data: URIs and external origins are skipped by
// isInternalLink; only attribute values on these three tags are read.
const MEDIA_TAG_PATTERN = /<(img|video|source)\b[^>]*>/gi;
const MEDIA_ATTRS = { img: ['src'], video: ['src', 'poster'], source: ['src'] };

export function extractMediaRefs(html) {
	const refs = [];
	for (const [tag, name] of html.matchAll(MEDIA_TAG_PATTERN)) {
		for (const attr of MEDIA_ATTRS[name.toLowerCase()]) {
			const value = tag.match(new RegExp(`\\s${attr}="([^"]*)"`, 'i'));
			if (value && value[1]) refs.push(value[1]);
		}
	}
	return refs;
}

// Returns true for a link this checker should validate against dist/ --
// false for same-page anchors, external origins, and non-http(s) schemes.
export function isInternalLink(href, { base = SITE_BASE, origin = SITE_ORIGIN } = {}) {
	if (!href || href.startsWith('#')) return false;
	if (/^(mailto|tel|javascript|data|blob):/i.test(href)) return false;
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
		for (const href of [...hrefs, ...extractMediaRefs(html)]) {
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
	// Optional first argument: a dist/ directory other than the site's own.
	const { failures, checked } = await checkInternalLinks(process.argv[2] ? { distDir: path.resolve(process.argv[2]) } : {});

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
