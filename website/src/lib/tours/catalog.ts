// Config-time discovery of tour sources and docs ids, used by content.config.ts
// (before the content layer exists) to validate helpDoc and to compute the
// smoke-mode loader exclusion. Reads files directly; no astro:content.
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import yaml from 'js-yaml';
import { TOURS_DIR, isTourId } from './schema.ts';

const DOC_EXT = /\.(md|mdx)$/;

function walk(dir: string): string[] {
	if (!existsSync(dir)) return [];
	return readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
		e.isDirectory() ? walk(join(dir, e.name)) : [join(dir, e.name)],
	);
}

/** Starlight docs collection ids for every doc file (`help/x.md` -> `help/x`, `a/index.mdx` -> `a`). */
export function listDocIds(docsDir: string): Set<string> {
	const ids = new Set<string>();
	for (const file of walk(docsDir)) {
		if (!DOC_EXT.test(file)) continue;
		const rel = relative(docsDir, file).split('\\').join('/').replace(DOC_EXT, '');
		ids.add(rel.replace(/\/index$/, ''));
	}
	return ids;
}

export function parseFrontmatter(source: string): Record<string, unknown> {
	const match = source.match(/^---\r?\n([\s\S]*?)\r?\n---/);
	if (!match) return {};
	const data = yaml.load(match[1]);
	return data && typeof data === 'object' ? (data as Record<string, unknown>) : {};
}

export interface TourSource {
	/** Docs collection id, e.g. `tours/dashboard-lifecycle`. */
	id: string;
	/** Path relative to the docs dir, e.g. `tours/dashboard-lifecycle.mdx`. */
	file: string;
	clip?: string;
}

/** Every tours/*.md(x) except the index, with its raw `tour.clip` (unvalidated). */
export function readTourSources(docsDir: string): TourSource[] {
	return walk(join(docsDir, TOURS_DIR))
		.filter((f) => DOC_EXT.test(f))
		.map((f) => {
			const file = relative(docsDir, f).split('\\').join('/');
			const id = file.replace(DOC_EXT, '');
			const tour = parseFrontmatter(readFileSync(f, 'utf8')).tour as { clip?: unknown } | undefined;
			return { id, file, clip: typeof tour?.clip === 'string' ? tour.clip : undefined };
		})
		.filter((s) => isTourId(s.id));
}
