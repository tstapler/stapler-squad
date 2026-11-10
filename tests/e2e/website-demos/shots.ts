// @feature website-demos
// Still captures for tour steps: deterministic (reduced motion, no animations, fonts
// loaded), masked where the page prints a version or build stamp, desktop or mobile.
import type { Browser, BrowserContext, Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { SceneAssertionError, THEME_ID, assertGrimdark, assertLanding } from './assertions';
import { type FormFactor, type MediaEntry, type Seeded, fileFacts } from './manifest';
import type { ShotSpec } from './scene';

export const DESKTOP_VIEWPORT = { width: 1280, height: 720 } as const;
export const MOBILE_VIEWPORT = { width: 390, height: 844 } as const;

/** Pages print these; masking keeps a shot identical across builds. */
export const DEFAULT_MASKS: readonly string[] = ['[data-testid="build-provenance"]', '[data-testid="tymuxd-version"]'];

/** Grimdark gold on near-black; same pair the caption overlay uses. Contrast is unit-tested. */
export const BADGE_COLORS = { fill: '#c0a020', text: '#0c0a08' } as const;

export function viewportFor(formFactor: FormFactor = 'desktop') {
  return formFactor === 'mobile' ? MOBILE_VIEWPORT : DESKTOP_VIEWPORT;
}

export function shotSrc(name: string): string {
  return `shots/${name}.png`;
}

/** Device pixel ratio for shots; the viewport (and so the layout) is unchanged by it. */
export function shotScale(env: NodeJS.ProcessEnv = process.env): number {
  const n = Number(env.WEBSITE_DEMO_DPR ?? '1');
  return Number.isFinite(n) && n >= 1 && n <= 3 ? n : 1;
}

export interface ContextOptions {
  baseURL: string;
  formFactor?: FormFactor;
  video?: { dir: string };
}

/** A fresh context in the shot's form factor. Mobile is touch with a 390x844 viewport. */
export async function newShotContext(browser: Browser, opts: ContextOptions): Promise<BrowserContext> {
  const ff = opts.formFactor ?? 'desktop';
  const viewport = viewportFor(ff);
  return browser.newContext({
    baseURL: opts.baseURL,
    viewport,
    deviceScaleFactor: shotScale(),
    isMobile: ff === 'mobile',
    hasTouch: ff === 'mobile',
    reducedMotion: 'reduce',
    ...(opts.video ? { recordVideo: { dir: opts.video.dir, size: viewport } } : {}),
  });
}

async function fontsReady(page: Page): Promise<void> {
  const ok = await page.evaluate(async () => {
    await document.fonts.ready;
    const family = getComputedStyle(document.body).fontFamily.split(',')[0].trim();
    return document.fonts.check(`16px ${family}`);
  });
  if (!ok) throw new SceneAssertionError('body font did not finish loading before the shot');
}

async function drawBadges(page: Page, spec: ShotSpec): Promise<void> {
  if (!spec.annotations?.length) return;
  await page.evaluate(
    ({ items, colors }) => {
      for (const { selector, n } of items) {
        const target = document.querySelector(selector);
        if (!target) throw new Error(`annotation target not found: ${selector}`);
        const r = target.getBoundingClientRect();
        const badge = document.createElement('div');
        badge.setAttribute('data-demo-badge', String(n));
        badge.textContent = String(n);
        badge.style.cssText = `position:fixed;left:${Math.max(4, r.left - 12)}px;top:${Math.max(4, r.top - 12)}px;width:28px;height:28px;
          border-radius:50%;background:${colors.fill};color:${colors.text};font:700 16px/28px system-ui,sans-serif;
          text-align:center;box-shadow:0 0 0 2px ${colors.text};z-index:2147483646;pointer-events:none`;
        document.body.appendChild(badge);
      }
    },
    { items: spec.annotations, colors: BADGE_COLORS },
  );
}

export interface ShotContext {
  outDir: string;
  commit: string;
  seeded?: Seeded;
}

/** Captures the page as it is now (navigation, landing check and Grimdark check belong to the caller). */
export async function captureShot(page: Page, spec: ShotSpec, ctx: ShotContext): Promise<MediaEntry> {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await fontsReady(page);
  await spec.prepare?.(page);
  await drawBadges(page, spec);
  const ff = spec.formFactor ?? 'desktop';
  const src = shotSrc(spec.name);
  const file = path.join(ctx.outDir, src);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  await page.screenshot({
    path: file,
    type: 'png',
    animations: 'disabled',
    caret: 'hide',
    mask: [...DEFAULT_MASKS, ...(spec.mask ?? [])].map((s) => page.locator(s)),
  });
  return { name: spec.name, format: 'png', status: 'recorded', src, ...fileFacts(file), commit: ctx.commit, themeId: THEME_ID, seeded: ctx.seeded, formFactor: ff, ...pixelSize(ff) };
}

function pixelSize(ff: FormFactor): { width: number; height: number } {
  const { width, height } = viewportFor(ff);
  const scale = shotScale();
  return { width: width * scale, height: height * scale };
}

/** Opens the shot's route in its own context, asserts landing and theme, then captures. */
export async function recordShot(browser: Browser, spec: ShotSpec, ctx: ShotContext & { baseURL: string; init: (c: BrowserContext) => Promise<void> }): Promise<MediaEntry> {
  const context = await newShotContext(browser, { baseURL: ctx.baseURL, formFactor: spec.formFactor });
  try {
    await ctx.init(context);
    const page = await context.newPage();
    await page.goto(spec.route);
    await assertLanding(page, { path: spec.route.split('?')[0], landmark: spec.landmark });
    await assertGrimdark(page);
    return await captureShot(page, spec, ctx);
  } finally {
    await context.close();
  }
}
