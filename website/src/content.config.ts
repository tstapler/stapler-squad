import { defineCollection } from 'astro:content';
import { glob } from 'astro/loaders';
import { fileURLToPath } from 'node:url';
import { docsSchema, i18nSchema } from '@astrojs/starlight/schema';
import { i18nLoader } from '@astrojs/starlight/loaders';
import { z } from 'astro/zod';
import { listDocIds, readTourSources } from './lib/tours/catalog.ts';
import { mediaMode, requireManifest, unrecordedTourIds } from './lib/tours/manifest.ts';
import { makeTourSchema } from './lib/tours/schema.ts';
import { assertDeployMedia, validateTourSources } from './lib/tours/validate.ts';

const DOCS_DIR = fileURLToPath(new URL('./content/docs/', import.meta.url));
const DOCS_PATTERN = '**/[^_]*.{md,mdx}'; // same as Starlight's docsLoader()

// Tours (ADR-001): validate every tours/*.mdx at config time, then (smoke mode)
// drop tours whose clip is unrecorded from the loader so they have no page,
// sidebar entry, search hit, card or link (Cross-cutting 6c).
const mode = mediaMode();
const tourSources = readTourSources(DOCS_DIR);
const docIds = listDocIds(DOCS_DIR);
const tourEntries = validateTourSources(DOCS_DIR, tourSources, docIds);
assertDeployMedia(tourEntries, mode);

const excludedIds =
	mode === 'smoke' && tourSources.length > 0
		? unrecordedTourIds(
				requireManifest(mode),
				tourEntries.map((t) => ({ id: t.id, clip: t.tour.clip })),
			)
		: [];
if (excludedIds.length > 0 && excludedIds.length === tourEntries.length) {
	throw new Error('TOURS_MEDIA_MODE=smoke built zero tours: the smoke clip is missing from media-manifest.json');
}
const smokeExclusions = excludedIds.map((id) => `!${id}.{md,mdx}`);

export const collections = {
	docs: defineCollection({
		// Starlight's docsLoader() takes no pattern, so this is its glob made explicit.
		loader: glob({ base: DOCS_DIR, pattern: [DOCS_PATTERN, ...smokeExclusions] }),
		schema: docsSchema({ extend: z.object({ tour: makeTourSchema(docIds).optional() }) }),
	}),
	// Task 4.4.1a: overrides Pagefind's default zero-results string (plain
	// "No results for [SEARCH_TERM]") with one that names an exit path, since
	// Pagefind's default UI renders that string as a text node (verified by
	// reading @pagefind/default-ui's source) and can't embed a clickable link.
	i18n: defineCollection({ loader: i18nLoader(), schema: i18nSchema() }),
};
