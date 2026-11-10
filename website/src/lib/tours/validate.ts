// Build-time validation of tour sources, run from content.config.ts. Throws one
// aggregated error naming every offending file and Zod field path, so a bad
// tour fails `bun run build` before any page renders.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseFrontmatter, type TourSource } from './catalog.ts';
import { checkOrderSet, makeTourSchema, type TourEntry } from './schema.ts';
import { clipDurationMs, hasClip, requireManifest, type MediaMode } from './manifest.ts';

export const CLIP_MAX_DURATION_MS = 60_000;

export function validateTourSources(
	docsDir: string,
	sources: readonly TourSource[],
	docIds: ReadonlySet<string>,
): TourEntry[] {
	const schema = makeTourSchema(docIds);
	const errors: string[] = [];
	const entries: TourEntry[] = [];
	for (const s of sources) {
		const raw = parseFrontmatter(readFileSync(join(docsDir, s.file), 'utf8')).tour;
		if (raw === undefined) {
			errors.push(`${s.file}: missing required "tour" frontmatter object`);
			continue;
		}
		const parsed = schema.safeParse(raw);
		if (!parsed.success) {
			for (const issue of parsed.error.issues) {
				errors.push(`${s.file}: tour.${issue.path.join('.')}: ${issue.message}`);
			}
			continue;
		}
		entries.push({ id: s.id, tour: parsed.data });
	}
	errors.push(...checkOrderSet(entries));
	if (errors.length > 0) throw new Error(`Invalid tours:\n  ${errors.join('\n  ')}`);
	return entries;
}

/**
 * Deploy mode: every tour clip must be in the manifest (and within the length
 * cap when the manifest records a duration), and the manifest must exist.
 * Only called when at least one tour exists.
 */
export function assertDeployMedia(entries: readonly TourEntry[], mode: MediaMode, manifestPath?: string): void {
	if (mode !== 'deploy' || entries.length === 0) return;
	const manifest = requireManifest(mode, manifestPath);
	const errors: string[] = [];
	for (const { id, tour } of entries) {
		if (!hasClip(manifest, tour.clip)) errors.push(`${id}: clip "${tour.clip}" is absent from media-manifest.json`);
		const d = clipDurationMs(manifest, tour.clip);
		if (d !== undefined && d > CLIP_MAX_DURATION_MS) {
			errors.push(`${id}: clip "${tour.clip}" is ${d} ms, over the ${CLIP_MAX_DURATION_MS} ms cap`);
		}
	}
	if (errors.length > 0) throw new Error(`Tour media check failed (TOURS_MEDIA_MODE=deploy):\n  ${errors.join('\n  ')}`);
}
