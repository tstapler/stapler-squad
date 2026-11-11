// What the built site should contain, derived from the same inputs the build
// used (tours/*.mdx frontmatter + media-manifest.json + TOURS_MEDIA_MODE), so
// the e2e assertions parametrize themselves for deploy and smoke builds.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseFrontmatter, readTourSources } from '../../src/lib/tours/catalog.ts';
import { hasClip, mediaMode, readManifest } from '../../src/lib/tours/manifest.ts';
import { START_HERE_MAX_ORDER } from '../../src/lib/tours/nav.ts';

const DOCS_DIR = join(import.meta.dirname, '..', '..', 'src', 'content', 'docs');

export interface ExpectedTour {
	slug: string;
	title: string;
	order: number;
	category: string;
}

export interface TourExpectations {
	/** Tour sources on disk, built or not. */
	sources: number;
	built: ExpectedTour[];
	startHere: ExpectedTour[];
	categories: string[];
}

export function loadTourExpectations(): TourExpectations {
	const sources = readTourSources(DOCS_DIR);
	const mode = mediaMode();
	const manifest = readManifest();
	const built = sources
		.filter((s) => mode === 'deploy' || (manifest && s.clip && hasClip(manifest, s.clip)))
		.map((s) => {
			const t = parseFrontmatter(readFileSync(join(DOCS_DIR, s.file), 'utf8')).tour as ExpectedTour & { title: string };
			return { slug: s.id.replace(/^tours\//, ''), title: t.title, order: t.order, category: t.category };
		})
		.sort((a, b) => a.order - b.order);
	return {
		sources: sources.length,
		built,
		startHere: built.filter((t) => t.order <= START_HERE_MAX_ORDER),
		categories: [...new Set(built.map((t) => t.category))],
	};
}
