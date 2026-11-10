// @feature website-demos
// What a tour scene file exports, and how scene files are found. Adding a tour is one
// tours/<id>.ts here plus one tours/<id>.mdx on the website; nothing else is registered.
import type { Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import type { Landing } from './assertions';
import type { FormFactor, Seeded } from './manifest';

/** Handed to a scene so it can mark the end of page loading. */
export interface ClipControl {
  /**
   * Call once the first real content is on screen. Everything before it is trimmed;
   * the Grimdark, dialog and landing assertions run here, so a wrong theme or a
   * redirected page fails before any footage is kept.
   */
  ready(): Promise<void>;
}

export type Scenes = (page: Page, clip: ClipControl) => Promise<void>;

export interface Annotation {
  /** Element the numbered badge points at. */
  selector: string;
  /** Marker number shown in the badge, matching the step caption. */
  n: number;
}

export interface ShotSpec {
  /** File stem and manifest name, e.g. 'dashboard-mobile'. */
  name: string;
  /** Route to open, e.g. '/review-queue'. */
  route: string;
  /** Landmark that proves the real page rendered (not a redirect target). */
  landmark: string;
  formFactor?: FormFactor;
  /** Extra selectors to mask (version and build stamps are always masked). */
  mask?: string[];
  annotations?: Annotation[];
  /** Brings the page to the state worth capturing (open a panel, filter a list). */
  prepare?: (page: Page) => Promise<void>;
}

export interface ClipSpec {
  /** Clip file stem and tour id. */
  name: string;
  /** Title-card text. */
  title: string;
  /** App theme the clip is recorded in; media is Grimdark only, so this exists for fixtures. */
  theme?: string;
  scenes: Scenes;
  /** Checked at ready(): the page the clip is meant to show. */
  landing?: Landing;
  /** Feature flags the scene needs on; enabled and read back before recording. */
  flags?: string[];
  /** Strings the tour steps quote; each must appear in the page text. */
  quotes?: string[];
  /** How the data on screen was seeded; 'route' marks a route-fixture fallback. */
  seeded?: Seeded;
  shots?: ShotSpec[];
}

export type Scene = ClipSpec;

/**
 * Stills with no clip: the feature-card shots and the mobile set. Found by glob in
 * shotlists/<name>.ts (file name = list name), recorded like a scene by `--only <name>`.
 */
export interface ShotListSpec {
  name: string;
  /** Feature flags the shots need on; enabled and read back before capturing. */
  flags?: string[];
  seeded?: Seeded;
  shots: ShotSpec[];
}

/** Files in a tours directory that are scenes: *.ts, minus tests and `_`-prefixed helpers. */
export function sceneFiles(dir: string): string[] {
  if (!fs.existsSync(dir)) return [];
  return fs
    .readdirSync(dir)
    .filter((f) => f.endsWith('.ts') && !f.endsWith('.test.ts') && !f.endsWith('.d.ts') && !f.startsWith('_'))
    .sort();
}

/** A scene module must export `scene` named after its file, so id, file and manifest name agree. */
export function validateScene(file: string, mod: { scene?: Scene }): Scene {
  const id = path.basename(file, '.ts');
  if (!mod.scene) throw new Error(`${file}: must export \`scene\``);
  if (mod.scene.name !== id) throw new Error(`${file}: scene.name is "${mod.scene.name}" but the file is "${id}"`);
  return mod.scene;
}

/** Synchronous so a spec file can register one test per scene while Playwright collects. */
export function discoverScenes(dir: string): Scene[] {
  // eslint-disable-next-line @typescript-eslint/no-require-imports -- collection is synchronous; ESM import() is async
  return sceneFiles(dir).map((f) => validateScene(f, require(path.join(dir, f))));
}

/** `only` is the comma-separated list from `record-website-demos.sh --only`; unknown ids fail loudly. */
export function selectScenes<T extends { name: string }>(all: T[], only: string | undefined): T[] {
  const wanted = (only ?? '').split(',').map((s) => s.trim()).filter(Boolean);
  if (wanted.length === 0) return all;
  const unknown = wanted.filter((w) => !all.some((s) => s.name === w));
  if (unknown.length > 0) throw new Error(`--only names no known scene: ${unknown.join(', ')}`);
  return all.filter((s) => wanted.includes(s.name));
}

/** A shotlist module must export `shotlist` named after its file, mirroring validateScene. */
export function validateShotList(file: string, mod: { shotlist?: ShotListSpec }): ShotListSpec {
  const id = path.basename(file, '.ts');
  if (!mod.shotlist) throw new Error(`${file}: must export \`shotlist\``);
  if (mod.shotlist.name !== id) throw new Error(`${file}: shotlist.name is "${mod.shotlist.name}" but the file is "${id}"`);
  if (mod.shotlist.shots.length === 0) throw new Error(`${file}: shotlist has no shots`);
  return mod.shotlist;
}

export function discoverShotLists(dir: string): ShotListSpec[] {
  // eslint-disable-next-line @typescript-eslint/no-require-imports -- collection is synchronous; ESM import() is async
  return sceneFiles(dir).map((f) => validateShotList(f, require(path.join(dir, f))));
}
