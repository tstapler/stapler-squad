// Recursive HTML file discovery shared by the dist link checks. The release-URL
// helpers that used to live here were retired with demo-publish.yml.

import { readdir } from 'node:fs/promises';
import { join } from 'node:path';

export async function findHtmlFiles(dir: string): Promise<string[]> {
	let entries;
	try {
		entries = await readdir(dir, { withFileTypes: true });
	} catch {
		// Build output directory doesn't exist yet (e.g. bun run build hasn't
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
