// Reads media-manifest.json (the recorder's record-time truth) and applies the
// two media modes of Cross-cutting 6c. The manifest shape here is the
// consumer-side contract the recorder (Epic 2) must emit:
//   { commit, recordedAt, entries: [{ name, kind: 'clip'|'shot', formFactor,
//     format, durationMs?, width?, height? }] }
// File naming under public/: clips/<name>.webm|.mp4, clips/<name>.poster.webp,
// clips/<name>.transcript.json, shots/<name>.webp, shots/<name>.mobile.webp.
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
	const parsed = JSON.parse(readFileSync(path, 'utf8'));
	if (typeof parsed?.commit !== 'string' || typeof parsed?.recordedAt !== 'string' || !Array.isArray(parsed?.entries)) {
		throw new Error(`${MEDIA_MANIFEST_FILE} at ${path} must have string commit, string recordedAt and an entries array`);
	}
	return parsed as MediaManifest;
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
	poster: `${base}/clips/${name}.poster.webp`,
});
export const shotUrl = (base: string, name: string, formFactor: 'desktop' | 'mobile' = 'desktop') =>
	`${base}/shots/${name}${formFactor === 'mobile' ? '.mobile' : ''}.webp`;

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
