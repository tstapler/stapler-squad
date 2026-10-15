// ReleaseAssetRef (ADR-002): the single typed place that constructs demo-asset
// URLs under the project's GitHub Releases download convention, so a renamed
// tag/filename is a one-place edit instead of scattered string interpolation.

import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';

const RELEASE_DOWNLOAD_BASE = 'https://github.com/tstapler/stapler-squad/releases/download';

export interface ReleaseAssetRef {
	tag: string;
	name: string;
	ext: string;
}

export function releaseAssetUrl({ tag, name, ext }: ReleaseAssetRef): string {
	return `${RELEASE_DOWNLOAD_BASE}/${tag}/${name}.${ext}`;
}

// Matches any releaseAssetUrl()-shaped URL embedded in rendered HTML.
const RELEASE_ASSET_URL_PATTERN = /https:\/\/github\.com\/tstapler\/stapler-squad\/releases\/download\/[\w.-]+\/[\w.-]+/g;

async function findHtmlFiles(dir: string): Promise<string[]> {
	let entries;
	try {
		entries = await readdir(dir, { withFileTypes: true });
	} catch {
		// Build output directory doesn't exist yet (e.g. pnpm run build hasn't
		// run) -- nothing to scan.
		return [];
	}

	const files: string[] = [];
	for (const entry of entries) {
		const fullPath = join(dir, entry.name);
		if (entry.isDirectory()) {
			files.push(...(await findHtmlFiles(fullPath)));
		} else if (entry.isFile() && entry.name.endsWith('.html')) {
			files.push(fullPath);
		}
	}
	return files;
}

// Scans built website/dist/**/*.html for releases/download/ URLs, so
// AssetHealthCheck has no second, hand-maintained list of which clips are
// referenced -- the rendered HTML is the one source of truth.
export async function extractReleaseAssetUrls(distDir: string): Promise<string[]> {
	const htmlFiles = await findHtmlFiles(distDir);
	const found = new Set<string>();
	for (const file of htmlFiles) {
		const contents = await readFile(file, 'utf8');
		for (const match of contents.matchAll(RELEASE_ASSET_URL_PATTERN)) {
			found.add(match[0]);
		}
	}
	return [...found].sort();
}
