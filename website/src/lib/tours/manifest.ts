// Reads media-manifest.json (the recorder's record-time truth) and applies the
// two media modes of Cross-cutting 6c.
//
// Wire shape (one row per rendition, owned by tools/promo/internal/manifest.MediaManifest and
// emitted by tests/e2e/website-demos/manifest.ts; example: tools/promo/schema/media-manifest.example.json):
//   { version, commit, startedAt, finishedAt, entries: [{ name, format: webm|mp4|webp|png,
//     status, src, poster?, durationMs?, formFactor?, width?, height?, ... }] }
// `parseManifest` normalizes it for the components: `recordedAt` is `finishedAt`, only
// `status: recorded` rows count, a webm row is a `clip`, an image row is a `shot` (mp4 rows add
// nothing: the clip is its webm row). Shipped files under public/: clips/<name>.webm|.mp4,
// clips/<name>.transcript.json (sidecar by convention, no manifest field), shots/<name>.webp
// (mobile shots are named for themselves, e.g. dashboard-mobile) and shots/<name>-poster.webp.
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export type MediaMode = 'deploy' | 'smoke';

export interface MediaEntry {
	name: string;
	kind: 'clip' | 'shot';
	formFactor?: 'desktop' | 'mobile';
	format?: string;
	durationMs?: number;
	width?: number;
	height?: number;
}

export interface MediaManifest {
	commit: string;
	recordedAt: string;
	entries: MediaEntry[];
}

interface WireEntry {
	name: string;
	format: string;
	status: string;
	durationMs?: number;
	formFactor?: string;
	width?: number;
	height?: number;
}

/** Normalizes the Go-shaped wire manifest; throws on a structurally invalid one. */
export function parseManifest(raw: unknown, where = MEDIA_MANIFEST_FILE): MediaManifest {
	const m = raw as { commit?: unknown; finishedAt?: unknown; entries?: unknown } | null;
	if (typeof m?.commit !== 'string' || typeof m?.finishedAt !== 'string' || !Array.isArray(m?.entries)) {
		throw new Error(`${where} must have string commit, string finishedAt and an entries array`);
	}
	const entries: MediaEntry[] = [];
	for (const e of m.entries as WireEntry[]) {
		if (e.status !== 'recorded') continue;
		if (e.format === 'mp4') continue;
		const formFactor = e.formFactor === 'mobile' ? 'mobile' : e.formFactor === 'desktop' ? 'desktop' : undefined;
		const clip = e.format === 'webm';
		entries.push({
			name: e.name,
			kind: clip ? 'clip' : 'shot',
			format: e.format,
			...(formFactor ? { formFactor } : {}),
			...(typeof e.durationMs === 'number' ? { durationMs: e.durationMs } : {}),
			...(!clip && typeof e.width === 'number' ? { width: e.width } : {}),
			...(!clip && typeof e.height === 'number' ? { height: e.height } : {}),
		});
	}
	return { commit: m.commit, recordedAt: m.finishedAt, entries };
}

export const MEDIA_MANIFEST_FILE = 'media-manifest.json';
const WEBSITE_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..');
export const DEFAULT_MANIFEST_PATH = join(WEBSITE_ROOT, 'public', MEDIA_MANIFEST_FILE);

export function mediaMode(env: Record<string, string | undefined> = process.env): MediaMode {
	const raw = env.TOURS_MEDIA_MODE ?? 'deploy';
	if (raw !== 'deploy' && raw !== 'smoke') {
		throw new Error(`TOURS_MEDIA_MODE must be "deploy" or "smoke", got "${raw}"`);
	}
	return raw;
}

/** Returns null when the file is absent; throws on a present-but-malformed file. */
export function readManifest(path: string = process.env.MEDIA_MANIFEST_PATH ?? DEFAULT_MANIFEST_PATH): MediaManifest | null {
	if (!existsSync(path)) return null;
	return parseManifest(JSON.parse(readFileSync(path, 'utf8')), `${MEDIA_MANIFEST_FILE} at ${path}`);
}

/** Deploy mode requires the manifest; smoke treats an absent one as nothing recorded. */
export function requireManifest(mode: MediaMode, path?: string): MediaManifest {
	const manifest = readManifest(path);
	if (manifest) return manifest;
	if (mode === 'deploy') {
		throw new Error(`${MEDIA_MANIFEST_FILE} not found${path ? ` at ${path}` : ''}: deploy builds need the recorded media manifest`);
	}
	return { commit: '', recordedAt: '', entries: [] };
}

export function hasClip(manifest: MediaManifest, name: string): boolean {
	return manifest.entries.some((e) => e.kind === 'clip' && e.name === name);
}

export function hasShot(manifest: MediaManifest, name: string, formFactor: 'desktop' | 'mobile' = 'desktop'): boolean {
	return manifest.entries.some((e) => e.kind === 'shot' && e.name === name && (e.formFactor ?? 'desktop') === formFactor);
}

export function clipDurationMs(manifest: MediaManifest, name: string): number | undefined {
	return manifest.entries.find((e) => e.kind === 'clip' && e.name === name && typeof e.durationMs === 'number')?.durationMs;
}

export function shotSize(manifest: MediaManifest, name: string, formFactor: 'desktop' | 'mobile' = 'desktop'): { width: number; height: number } {
	const e = manifest.entries.find((x) => x.kind === 'shot' && x.name === name && (x.formFactor ?? 'desktop') === formFactor);
	const fallback = formFactor === 'mobile' ? { width: 390, height: 844 } : { width: 1280, height: 720 };
	return { width: e?.width ?? fallback.width, height: e?.height ?? fallback.height };
}

/** Smoke mode: tour ids (without `tours/`) whose clip is absent are excluded from the docs loader. */
export function unrecordedTourIds(manifest: MediaManifest, tours: ReadonlyArray<{ id: string; clip: string }>): string[] {
	return tours.filter((t) => !hasClip(manifest, t.clip)).map((t) => t.id);
}

export const clipUrls = (base: string, name: string) => ({
	webm: `${base}/clips/${name}.webm`,
	mp4: `${base}/clips/${name}.mp4`,
	poster: `${base}/shots/${name}-poster.webp`,
});
export const shotUrl = (base: string, name: string) => `${base}/shots/${name}.webp`;

/** "Recorded 2026-11-10 (build <small>a1b2c3d</small>)" parts (Cross-cutting 8g). */
export function stampParts(manifest: MediaManifest): { date: string; sha: string } {
	return { date: manifest.recordedAt.slice(0, 10), sha: manifest.commit.slice(0, 7) };
}

/** "M:SS" for the meta row; undefined when the manifest has no duration. */
export function formatWatch(durationMs: number | undefined): string | undefined {
	if (durationMs === undefined) return undefined;
	const total = Math.round(durationMs / 1000);
	return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
}
