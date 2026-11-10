// @feature website-demos
// Record-time media manifest: one JSON per shard, merged by `mediatool`. Field names and
// the sort order mirror tools/promo/internal/manifest (MediaEntry, Merge). After post-processing,
// `mediatool refresh` re-measures the shipped files and adds the mp4 rows.
import * as crypto from 'crypto';
import * as fs from 'fs';
import * as path from 'path';
import { THEME_ID } from './assertions';

export type Format = 'webm' | 'mp4' | 'webp' | 'png';
export type Status = 'recorded' | 'failed' | 'skipped';
export type Seeded = 'db' | 'rpc' | 'route';
export type FormFactor = 'desktop' | 'mobile';

export interface MediaEntry {
  name: string;
  format: Format;
  status: Status;
  src?: string;
  poster?: string;
  source?: string;
  bytes: number;
  sha256?: string;
  commit: string;
  themeId: string;
  attempts?: number;
  seeded?: Seeded;
  durationMs?: number;
  formFactor?: FormFactor;
}

export interface MediaManifest {
  version: number;
  commit: string;
  startedAt: string;
  finishedAt: string;
  entries: MediaEntry[];
}

export const MANIFEST_VERSION = 1;
export const MANIFEST_FILE = 'media-manifest.json';
const PARTS_DIR = '.manifest-parts';

export function sha256Of(file: string): string {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

/** Size and hash of a finished file; a missing file yields zero bytes so a failed row stays writable. */
export function fileFacts(file: string): { bytes: number; sha256?: string } {
  if (!fs.existsSync(file)) return { bytes: 0 };
  return { bytes: fs.statSync(file).size, sha256: sha256Of(file) };
}

export function failedEntry(name: string, format: Format, commit: string, attempts: number): MediaEntry {
  return { name, format, status: 'failed', bytes: 0, commit, themeId: THEME_ID, attempts };
}

/** Each scene writes its own part, so Playwright tests never contend over one file. */
export function writePart(outDir: string, id: string, entries: MediaEntry[]): void {
  const dir = path.join(outDir, PARTS_DIR);
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, `${id}.json`), JSON.stringify(entries, null, 2));
}

export function readParts(outDir: string): MediaEntry[] {
  const dir = path.join(outDir, PARTS_DIR);
  if (!fs.existsSync(dir)) return [];
  return fs
    .readdirSync(dir)
    .filter((f) => f.endsWith('.json'))
    .sort()
    .flatMap((f) => JSON.parse(fs.readFileSync(path.join(dir, f), 'utf8')) as MediaEntry[]);
}

/** Sorted by (name, format) and rejecting duplicates, like the Go Merge. */
export function mergeEntries(entries: MediaEntry[]): MediaEntry[] {
  const seen = new Set<string>();
  for (const e of entries) {
    const key = `${e.name}\0${e.format}`;
    if (seen.has(key)) throw new Error(`duplicate manifest entry ${e.name} (${e.format})`);
    seen.add(key);
  }
  return [...entries].sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : a.format < b.format ? -1 : a.format > b.format ? 1 : 0));
}

export function buildManifest(commit: string, startedAt: Date, finishedAt: Date, entries: MediaEntry[]): MediaManifest {
  return { version: MANIFEST_VERSION, commit, startedAt: startedAt.toISOString(), finishedAt: finishedAt.toISOString(), entries: mergeEntries(entries) };
}

/** First call stamps the shard start; a Playwright worker restart then keeps the true start for the wall-clock budget. */
export function shardStart(outDir: string): Date {
  const file = path.join(outDir, '.started-at');
  if (fs.existsSync(file)) return new Date(fs.readFileSync(file, 'utf8').trim());
  const now = new Date();
  fs.mkdirSync(outDir, { recursive: true });
  fs.writeFileSync(file, now.toISOString());
  return now;
}

/** Merges this shard's parts into <outDir>/media-manifest.json. */
export function writeShardManifest(outDir: string, commit: string, startedAt: Date, finishedAt: Date = new Date()): MediaManifest {
  const manifest = buildManifest(commit, startedAt, finishedAt, readParts(outDir));
  fs.mkdirSync(outDir, { recursive: true });
  fs.writeFileSync(path.join(outDir, MANIFEST_FILE), JSON.stringify(manifest, null, 2));
  return manifest;
}
