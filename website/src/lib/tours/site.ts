// Render-time (astro:content) access to the built tour set and the manifest.
import { getCollection } from 'astro:content';
import { fileURLToPath } from 'node:url';
import { readTourSources } from './catalog.ts';
import { mediaMode, readManifest, requireManifest, type MediaManifest, type MediaMode } from './manifest.ts';
import { isTourId, type TourEntry } from './schema.ts';

export const PUBLIC_DIR = fileURLToPath(new URL('../../../public/', import.meta.url));

export const basePath = (): string => import.meta.env.BASE_URL.replace(/\/$/, '');

export interface TourContext {
	mode: MediaMode;
	manifest: MediaManifest;
}

const DOCS_DIR = fileURLToPath(new URL('../../content/docs/', import.meta.url));

/**
 * Deploy mode needs the manifest only once a tour exists; before any tour
 * content lands, the empty /tours/ index must still build.
 */
export function tourContext(): TourContext {
	const mode = mediaMode();
	if (mode === 'deploy' && readTourSources(DOCS_DIR).length === 0) {
		return { mode, manifest: readManifest() ?? { commit: '', recordedAt: '', entries: [] } };
	}
	return { mode, manifest: requireManifest(mode) };
}

/** Tours present in the docs collection (smoke-excluded ones are already absent). */
export async function builtTours(): Promise<TourEntry[]> {
	const docs = await getCollection('docs');
	return docs.filter((d) => isTourId(d.id) && d.data.tour).map((d) => ({ id: d.id, tour: d.data.tour! }));
}
