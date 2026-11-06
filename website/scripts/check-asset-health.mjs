#!/usr/bin/env node
// Verifies every ReleaseAssetRef URL in the built site serves the exact bytes
// recorded in asset-health-manifest.json, not just a 200 -- a --clobber
// re-publish can leave a URL 200-ing with silently different content. See
// ADR-002 and project_plans/stapler-squad-website/implementation/pre-mortem.md
// Failure #5.

import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { extractReleaseAssetUrls } from '../src/lib/release-asset.ts';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
export const DEFAULT_DIST_DIR = path.join(__dirname, '..', 'dist');
export const DEFAULT_MANIFEST_PATH = path.join(__dirname, 'asset-health-manifest.json');

export function assetNameFromUrl(url) {
	return url.split('/').pop();
}

export async function loadManifest(manifestPath = DEFAULT_MANIFEST_PATH) {
	const raw = await readFile(manifestPath, 'utf8');
	return JSON.parse(raw);
}

function isHeadOk(status) {
	return status >= 200 && status < 400;
}

// Checks a single ReleaseAssetRef URL: HEAD first, then (only when the HEAD
// check is clean AND a manifest entry exists) GET + SHA256 content-identity
// comparison. headFn/getFn are injectable so tests can mock the network.
export async function checkUrl(url, manifest, { headFn = fetch, getFn = fetch } = {}) {
	const headResponse = await headFn(url, { method: 'HEAD' });
	if (!isHeadOk(headResponse.status)) {
		return { url, ok: false, reason: `HEAD request returned ${headResponse.status}` };
	}

	const name = assetNameFromUrl(url);
	const expectedHash = manifest[name];
	if (!expectedHash) {
		// Not-yet-published clip is a different failure than wrong-content
		// (plan.md Task 2.2.1b) -- report it through the same failure shape
		// as a HEAD-check failure rather than fabricating a hash comparison
		// against nothing.
		return { url, ok: false, reason: `no asset-health-manifest.json entry for "${name}" (not yet published?)` };
	}

	const getResponse = await getFn(url);
	const buffer = Buffer.from(await getResponse.arrayBuffer());
	const actualHash = createHash('sha256').update(buffer).digest('hex');
	if (actualHash !== expectedHash) {
		return {
			url,
			ok: false,
			reason: `content hash mismatch for "${name}": expected ${expectedHash}, got ${actualHash}`,
			expectedHash,
			actualHash,
		};
	}

	return { url, ok: true };
}

export async function checkAssetHealth({
	distDir = DEFAULT_DIST_DIR,
	manifestPath = DEFAULT_MANIFEST_PATH,
	headFn,
	getFn,
} = {}) {
	const urls = await extractReleaseAssetUrls(distDir);
	const manifest = await loadManifest(manifestPath);
	return Promise.all(urls.map((url) => checkUrl(url, manifest, { headFn, getFn })));
}

async function main() {
	const results = await checkAssetHealth();
	const failures = results.filter((result) => !result.ok);

	for (const failure of failures) {
		console.error(`[check-asset-health] FAIL ${failure.url}: ${failure.reason}`);
	}

	if (failures.length > 0) {
		console.error(
			`[check-asset-health] ${failures.length} of ${results.length} referenced release asset(s) failed health check`,
		);
		process.exitCode = 1;
		return;
	}

	console.log(`[check-asset-health] ${results.length} referenced release asset(s) OK`);
}

// Only run when invoked directly (node check-asset-health.mjs), not when
// imported by tests.
if (import.meta.url === `file://${process.argv[1]}`) {
	await main();
}
