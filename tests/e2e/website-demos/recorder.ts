// @feature website-demos
// Records one scene: a fresh context with video on, a frozen clock, the scene's steps,
// capture-time assertions, then the finished video, transcript, DOM dump, shots and
// manifest rows under OUT_DIR. The stable file name <name>.webm is the contract with the
// website (website/scripts/build-features.mjs), unlike Playwright's per-test directories.
import type { Browser, BrowserContext, Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { SceneAssertionError, THEME_ID, assertGrimdark, assertLanding, assertNoDialog, dumpDomText, missingQuotes } from './assertions';
import { hold } from './hold';
import { type MediaEntry, failedEntry, fileFacts, writePart } from './manifest';
import { NoRetryError, RetriesExhaustedError, withRetry } from './retry';
import type { ClipControl, ClipSpec, Scenes, ShotSpec } from './scene';
import { DESKTOP_VIEWPORT, newShotContext, recordShot } from './shots';
import { CaptionLog, attachCaptionLog, captionLogOf } from './transcript';

export type { ClipControl, ClipSpec, Scenes } from './scene';
export const VIEWPORT = DESKTOP_VIEWPORT;
export const OUT_DIR = path.resolve(process.env.WEBSITE_DEMO_OUT ?? 'website-demo-output');
/** scripts/demo-post-process.sh prepends a 1.5 s title card, so transcript times shift by it. */
export const TITLE_CARD_MS = 1500;

export interface RecordEnv {
  outDir: string;
  commit: string;
  /** Reference instant the seeder used (DEMO_REF_TIME); the page clock is fixed to it. */
  refTime?: string;
  /** Upper bound on clip length, from tools/promo/budgets.json. */
  clipMaxDurationMs: number;
  /** Injectable for tests. */
  fetchFn?: typeof fetch;
}

/** The single source of the clip cap is tools/promo/budgets.json (Cross-cutting 8h). */
export function readClipMaxDurationMs(budgetsFile = path.resolve(__dirname, '../../../tools/promo/budgets.json')): number {
  const v = (JSON.parse(fs.readFileSync(budgetsFile, 'utf8')) as { clipMaxDurationMs?: number }).clipMaxDurationMs;
  if (!Number.isInteger(v) || (v as number) <= 0) throw new Error(`${budgetsFile}: clipMaxDurationMs missing`);
  return v as number;
}

export function defaultEnv(env: NodeJS.ProcessEnv = process.env): RecordEnv {
  return {
    outDir: OUT_DIR,
    commit: env.GITHUB_SHA ?? env.PROMO_COMMIT ?? 'unknown',
    refTime: env.DEMO_REF_TIME,
    clipMaxDurationMs: readClipMaxDurationMs(),
  };
}

const flagRpc = (base: string, name: string) => `${base}/api/session.v1.SessionService/${name}`;

/** Enables feature flags on the isolated server and reads them back; a silently ignored write must fail. */
export async function ensureFlags(baseURL: string, flags: readonly string[], fetchFn: typeof fetch = fetch): Promise<void> {
  const post = (name: string, data: unknown) =>
    fetchFn(flagRpc(baseURL, name), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
  for (const flag of flags) {
    const res = await post('UpdateFeatureFlag', { name: flag, enabled: true });
    if (!res.ok) throw new Error(`UpdateFeatureFlag ${flag}: HTTP ${res.status}`);
  }
  if (flags.length === 0) return;
  const listed = (await (await post('GetFeatureFlags', {})).json()) as { flags?: { name: string; enabled?: boolean }[] };
  for (const flag of flags) {
    if (!listed.flags?.find((f) => f.name === flag)?.enabled) throw new Error(`feature flag ${flag} did not turn on`);
  }
}

/** Keeps tours and toasts out of the footage, selects the theme and freezes time. */
export async function initContext(context: BrowserContext, theme: string, refTime?: string): Promise<void> {
  // setFixedTime (not install): timers keep running so animations and polling still work.
  if (refTime) await context.clock.setFixedTime(new Date(refTime));
  await context.addInitScript((appTheme) => {
    try {
      localStorage.setItem('stapler-squad:onboarded', 'true');
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
      localStorage.setItem('ssq.notifications.quietMode', 'true');
      localStorage.setItem('stapler-theme', appTheme);
    } catch {
      // about:blank has no localStorage; the real pages run this again.
    }
  }, theme);
}

interface AttemptResult {
  rows: MediaEntry[];
  title: string;
}

async function saveClipOutputs(spec: ClipSpec, env: RecordEnv, log: CaptionLog, trimStartMs: number, video: ReturnType<Page['video']>): Promise<void> {
  const out = path.join(env.outDir, `${spec.name}.webm`);
  fs.mkdirSync(env.outDir, { recursive: true });
  await video!.saveAs(out);
  // Read by scripts/record-website-demos.sh: title card text and the measured load time to trim.
  fs.writeFileSync(path.join(env.outDir, `${spec.name}.meta.json`), JSON.stringify({ title: spec.title, trimStartMs }));
  fs.writeFileSync(path.join(env.outDir, `${spec.name}.transcript.json`), JSON.stringify(log.toJSON(), null, 2));
}

const posterSrc = (name: string) => `shots/${name}-poster.png`;

/** The first real frame, taken at ready() before the trim point so it never lands in the footage. */
async function capturePoster(page: Page, name: string, outDir: string): Promise<string> {
  const file = path.join(outDir, posterSrc(name));
  fs.mkdirSync(path.dirname(file), { recursive: true });
  await page.screenshot({ path: file, type: 'png', animations: 'disabled', caret: 'hide' });
  return file;
}

async function recordAttempt(browser: Browser, baseURL: string, spec: ClipSpec, env: RecordEnv, attempt: number): Promise<AttemptResult> {
  const theme = spec.theme ?? THEME_ID;
  const rawDir = path.join(env.outDir, '.raw', spec.name);
  fs.rmSync(rawDir, { recursive: true, force: true });
  fs.mkdirSync(rawDir, { recursive: true });
  if (spec.flags?.length) await ensureFlags(baseURL, spec.flags, env.fetchFn);

  const context = await newShotContext(browser, { baseURL, video: { dir: rawDir } });
  await initContext(context, theme, env.refTime);
  const page = await context.newPage();
  const startedAt = Date.now();
  let readyAt = startedAt;
  let readied = false;
  const log = new CaptionLog(Date.now, TITLE_CARD_MS);
  attachCaptionLog(page, log);
  const rows: MediaEntry[] = [];
  const control: ClipControl = {
    ready: async () => {
      if (spec.landing) await assertLanding(page, spec.landing);
      await assertGrimdark(page);
      await assertNoDialog(page);
      posterFile = await capturePoster(page, spec.name, env.outDir);
      readied = true;
      readyAt = Date.now();
      log.markReady();
    },
  };

  let posterFile = '';
  let domText = '';
  let durationMs = 0;
  let video: ReturnType<Page['video']>;
  try {
    video = page.video();
    await spec.scenes(page, control);
    if (!readied) throw new SceneAssertionError(`${spec.name}: scene never called clip.ready()`);
    await assertNoDialog(page);
    domText = await dumpDomText(page, env.outDir, spec.name);
    durationMs = log.elapsedMs();
  } finally {
    await context.close(); // finalizes the video file
  }

  const missing = missingQuotes(domText, spec.quotes ?? []);
  if (missing.length > 0) throw new SceneAssertionError(`${spec.name}: step quote(s) not on screen: ${missing.map((q) => JSON.stringify(q)).join(', ')}`);
  if (log.toJSON().length === 0) throw new SceneAssertionError(`${spec.name}: no captions, so no transcript (R19)`);
  if (durationMs > env.clipMaxDurationMs) throw new NoRetryError(`${spec.name}: clip is ${durationMs} ms, over the ${env.clipMaxDurationMs} ms cap`);

  await saveClipOutputs(spec, env, log, readyAt - startedAt, video);
  fs.rmSync(rawDir, { recursive: true, force: true });

  const webm = path.join(env.outDir, `${spec.name}.webm`);
  rows.push({
    name: spec.name,
    format: 'webm',
    status: 'recorded',
    src: `clips/${spec.name}.webm`,
    poster: posterSrc(spec.name),
    ...fileFacts(webm),
    commit: env.commit,
    themeId: theme,
    attempts: attempt,
    seeded: spec.seeded,
    durationMs,
    formFactor: 'desktop',
  });
  rows.push({
    name: `${spec.name}-poster`,
    format: 'png',
    status: 'recorded',
    src: posterSrc(spec.name),
    ...fileFacts(posterFile),
    commit: env.commit,
    themeId: theme,
    attempts: attempt,
    seeded: spec.seeded,
    formFactor: 'desktop',
  });
  for (const shot of spec.shots ?? []) {
    rows.push(await recordShotRow(browser, baseURL, shot, spec, env, attempt));
  }
  return { rows, title: spec.title };
}

async function recordShotRow(browser: Browser, baseURL: string, shot: ShotSpec, spec: ClipSpec, env: RecordEnv, attempt: number): Promise<MediaEntry> {
  const row = await recordShot(browser, shot, {
    baseURL,
    outDir: env.outDir,
    commit: env.commit,
    seeded: spec.seeded,
    init: (c) => initContext(c, spec.theme ?? THEME_ID, env.refTime),
  });
  return { ...row, attempts: attempt };
}

/** Between attempts, confirm the server still answers; a dead server aborts instead of recording garbage twice. */
async function serverStillUp(baseURL: string, fetchFn: typeof fetch): Promise<void> {
  const res = await fetchFn(baseURL + '/');
  if (!res.ok) throw new Error(`server returned HTTP ${res.status} between attempts`);
}

/**
 * Records one scene with up to two attempts. Writes the shard's manifest part either way:
 * recorded rows on success, a failed row otherwise. A theme failure also leaves a
 * `.fatal-<name>.txt` marker so the script fails the whole shard, whatever freshnessEnforce says.
 * Throws on failure so the Playwright test (and the shard) fails.
 */
export async function recordClip(browser: Browser, baseURL: string, clip: ClipSpec, env: RecordEnv = defaultEnv()): Promise<string> {
  const fetchFn = env.fetchFn ?? fetch;
  try {
    const { value } = await withRetry((n) => recordAttempt(browser, baseURL, clip, env, n), {
      between: () => serverStillUp(baseURL, fetchFn),
    });
    writePart(env.outDir, clip.name, value.rows);
    return path.join(env.outDir, `${clip.name}.webm`);
  } catch (error) {
    const attempts = error instanceof RetriesExhaustedError ? error.attempts : 1;
    writePart(env.outDir, clip.name, [failedEntry(clip.name, 'webm', env.commit, attempts)]);
    if (error instanceof NoRetryError) {
      fs.mkdirSync(env.outDir, { recursive: true });
      fs.writeFileSync(path.join(env.outDir, `.fatal-${clip.name}.txt`), error.message);
    }
    throw error;
  }
}

/** Bottom-left caption that slides in, holds, and slides out. Also logged for the transcript. */
export async function caption(page: Page, text: string, holdMs = 2200): Promise<void> {
  captionLogOf(page)?.add(text);
  await page.evaluate((label) => {
    document.getElementById('demo-caption')?.remove();
    const el = document.createElement('div');
    el.id = 'demo-caption';
    el.textContent = label;
    el.style.cssText = `position:fixed;bottom:32px;left:32px;max-width:60%;padding:10px 20px;border-radius:8px;
      background:rgba(12,10,8,.92);color:#e4d6b8;border-left:3px solid #c0a020;
      font:600 18px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;box-shadow:0 4px 24px rgba(0,0,0,.4);
      pointer-events:none;z-index:2147483645;opacity:0;transform:translateY(12px);
      transition:opacity .25s ease,transform .25s ease`;
    document.body.appendChild(el);
    requestAnimationFrame(() => {
      el.style.opacity = '1';
      el.style.transform = 'translateY(0)';
    });
  }, text);
  await hold(page, holdMs);
  await page.evaluate(() => {
    const el = document.getElementById('demo-caption');
    if (el) {
      el.style.opacity = '0';
      setTimeout(() => el.remove(), 280);
    }
  });
}

/** Types one character at a time so filters visibly update as the text grows. */
export async function typeSlowly(page: Page, selector: string, text: string, delay = 110): Promise<void> {
  const input = page.locator(selector).first();
  await input.click();
  await input.pressSequentially(text, { delay });
}
