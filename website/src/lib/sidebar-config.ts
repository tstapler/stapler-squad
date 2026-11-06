// Story 4.3.1: astro.config.mjs's Troubleshooting sidebar group is hand-maintained
// (an explicit slug list, not `autogenerate`, since help/ also holds
// getting-started.md and features.mdx -- see astro.config.mjs's comment). These
// helpers read it back out of the raw config source (same "parse the real file,
// don't re-assert a hardcoded copy" approach as src/lib/readme-copy.ts) so a test
// catches drift between the sidebar list and the files actually on disk.
import { stat } from 'node:fs/promises';
import { join } from 'node:path';

/** Extracts the `slug: '...'` values listed under one top-level sidebar group's `items` array. */
export function extractSidebarGroupSlugs(configSource: string, groupLabel: string): string[] {
	const groupPattern = new RegExp(`label:\\s*['"]${groupLabel}['"][\\s\\S]*?items:\\s*\\[([\\s\\S]*?)\\]`);
	const match = configSource.match(groupPattern);
	if (!match) {
		throw new Error(`Could not find sidebar group "${groupLabel}" in astro.config.mjs`);
	}

	const slugPattern = /slug:\s*['"]([^'"]+)['"]/g;
	return [...match[1].matchAll(slugPattern)].map((slugMatch) => slugMatch[1]);
}

/**
 * Returns the subset of `slugs` (collection-relative, e.g. "help/debug-with-logs")
 * that has no matching `.md`/`.mdx` file under `docsDir` -- empty when every
 * sidebar entry is backed by a real CuratedDoc.
 */
export async function findSlugsWithNoMatchingDoc(slugs: string[], docsDir: string): Promise<string[]> {
	const missing: string[] = [];
	for (const slug of slugs) {
		const candidates = [join(docsDir, `${slug}.md`), join(docsDir, `${slug}.mdx`)];
		let found = false;
		for (const candidate of candidates) {
			try {
				await stat(candidate);
				found = true;
				break;
			} catch {
				// try next candidate
			}
		}
		if (!found) missing.push(slug);
	}
	return missing;
}
