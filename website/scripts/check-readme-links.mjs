#!/usr/bin/env node
// README link check (Task 3.3.2c): every README URL under the site origin must resolve to a page in
// dist/. check-internal-links.mjs scans only dist, so it cannot see README. A URL on the origin but
// outside SITE_BASE fails, which is how a wrong base is caught.
//
// TOURS_MEDIA_MODE=smoke builds only the recorded tour, so README links to tour pages that were not
// built are skipped with a notice; /tours/ and at least one tour must still resolve. In deploy mode
// (the default) every link must resolve.
//
// Usage: node scripts/check-readme-links.mjs [distDir] [readmePath]

import { access, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { SITE_BASE, SITE_ORIGIN } from '../src/site-config.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_DIST = path.join(__dirname, '..', 'dist');
const DEFAULT_README = path.join(__dirname, '..', '..', 'README.md');
const TOUR_PAGE = /^\/tours\/[^/]+\/$/;

/** Every `http(s)://<origin>...` URL in the Markdown, de-duplicated, trailing punctuation removed. */
export function extractSiteUrls(markdown, origin = SITE_ORIGIN) {
	const escaped = origin.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	const urls = markdown.match(new RegExp(`${escaped}[^\\s)<>"']*`, 'g')) ?? [];
	return [...new Set(urls.map((u) => u.replace(/[.,;:]+$/, '')))];
}

/** The site-relative path ("/tours/") of a URL, or null when it is on the origin but outside the base. */
export function sitePath(url, { origin = SITE_ORIGIN, base = SITE_BASE } = {}) {
	const rest = url.slice(origin.length).split(/[?#]/)[0];
	if (rest === base || rest === `${base}/`) return '/';
	return rest.startsWith(`${base}/`) ? rest.slice(base.length) : null;
}

async function exists(file) {
	try {
		await access(file);
		return true;
	} catch {
		return false;
	}
}

async function resolvesInDist(distDir, sitePathname) {
	const rel = sitePathname.replace(/^\//, '');
	const candidates = sitePathname.endsWith('/') ? [path.join(distDir, rel, 'index.html')] : [path.join(distDir, rel), path.join(distDir, rel, 'index.html'), path.join(distDir, `${rel}.html`)];
	for (const c of candidates) if (await exists(c)) return true;
	return false;
}

/** @returns {Promise<{errors: string[], notices: string[], checked: number}>} */
export async function checkReadmeLinks({ readme, distDir = DEFAULT_DIST, mode = 'deploy', origin = SITE_ORIGIN, base = SITE_BASE }) {
	const errors = [];
	const notices = [];
	const urls = extractSiteUrls(readme, origin);
	if (urls.length === 0) errors.push(`README.md has no URL under ${origin}`);
	let tourPagesResolved = 0;
	for (const url of urls) {
		const p = sitePath(url, { origin, base });
		if (p === null) {
			errors.push(`${url}: not under the site base ${base}/`);
			continue;
		}
		const ok = await resolvesInDist(distDir, p);
		if (ok && TOUR_PAGE.test(p)) tourPagesResolved++;
		if (ok) continue;
		if (mode === 'smoke' && TOUR_PAGE.test(p)) notices.push(`${url}: tour not built in this smoke build, skipped`);
		else errors.push(`${url}: no page at ${p} in dist`);
	}
	if (mode === 'smoke' && tourPagesResolved === 0 && urls.some((u) => TOUR_PAGE.test(sitePath(u, { origin, base }) ?? ''))) {
		errors.push('smoke build: no README tour link resolves, but the recorded tour must');
	}
	return { errors, notices, checked: urls.length };
}

async function main() {
	const [distDir = DEFAULT_DIST, readmePath = DEFAULT_README] = process.argv.slice(2);
	const mode = process.env.TOURS_MEDIA_MODE === 'smoke' ? 'smoke' : 'deploy';
	const { errors, notices, checked } = await checkReadmeLinks({ readme: await readFile(readmePath, 'utf8'), distDir, mode });
	for (const n of notices) console.log(`notice: ${n}`);
	if (errors.length > 0) {
		for (const e of errors) console.error(`README link check: ${e}`);
		process.exitCode = 1;
		return;
	}
	console.log(`README link check: ${checked} URL(s) resolve in ${distDir} (${mode})`);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	await main();
}
