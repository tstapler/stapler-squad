// Records one website demo clip: a fresh browser context with video on, the
// caller's scenes, then the finished video moved to <outDir>/<name>.webm. The
// stable file name is the contract with the website (DemoClip name in
// website/scripts/build-features.mjs), unlike Playwright's per-test directories.
import type { Browser, Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

export const VIEWPORT = { width: 1280, height: 720 } as const;
export const OUT_DIR = path.resolve(process.env.WEBSITE_DEMO_OUT ?? 'website-demo-output');

/** Handed to a clip's scenes so it can mark the end of page loading. */
export interface ClipControl {
  /** Call once the first real content is on screen; everything before it is trimmed. */
  ready(): void;
}

export type Scenes = (page: Page, clip: ClipControl) => Promise<void>;

export interface ClipSpec {
  name: string;
  title: string;
  /** App theme the clip is recorded in; defaults to the site's own, wh40k. */
  theme?: string;
  scenes: Scenes;
}

export async function recordClip(browser: Browser, baseURL: string, clip: ClipSpec): Promise<string> {
  const { name, title, theme = 'wh40k', scenes } = clip;
  const rawDir = path.join(OUT_DIR, '.raw', name);
  fs.rmSync(rawDir, { recursive: true, force: true });
  fs.mkdirSync(rawDir, { recursive: true });

  const context = await browser.newContext({
    baseURL,
    viewport: VIEWPORT,
    recordVideo: { dir: rawDir, size: VIEWPORT },
  });
  await context.addInitScript((appTheme) => {
    try {
      // Keep tours and toasts out of the footage.
      localStorage.setItem('stapler-squad:onboarded', 'true');
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
      localStorage.setItem('ssq.notifications.quietMode', 'true');
      localStorage.setItem('stapler-theme', appTheme);
    } catch {
      // about:blank has no localStorage; the real pages run this again.
    }
  }, theme);
  const page = await context.newPage();
  const startedAt = Date.now();
  let readyAt = startedAt;
  let video;
  try {
    video = page.video();
    await scenes(page, { ready: () => (readyAt = Date.now()) });
  } finally {
    await context.close(); // finalizes the video file
  }

  const out = path.join(OUT_DIR, `${name}.webm`);
  fs.mkdirSync(OUT_DIR, { recursive: true });
  await video!.saveAs(out);
  // Read by scripts/record-website-demos.sh: the title card text, and the measured (not guessed) load time to trim.
  fs.writeFileSync(path.join(OUT_DIR, `${name}.meta.json`), JSON.stringify({ title, trimStartMs: readyAt - startedAt }));
  fs.rmSync(rawDir, { recursive: true, force: true });
  return out;
}

/** Bottom-left caption that slides in, holds, and slides out. */
export async function caption(page: Page, text: string, holdMs = 2200): Promise<void> {
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
  await page.waitForTimeout(holdMs);
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
