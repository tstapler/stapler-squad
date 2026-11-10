// @feature website-demos
// Capture-time assertions: what is on screen must be what the site promises.
// Theme failures are NoRetryError: no retry, and the shard fails whatever freshnessEnforce says.
import type { Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { NoRetryError } from './retry';

/** The only theme promo media is recorded in; mirrors tools/promo/internal/manifest.ThemeID. */
export const THEME_ID = 'grimdark';

/**
 * Body background per built-in theme, from web-app/src/styles/theme.css.ts
 * (kept in step by __tests__/assertions.test.ts). Class names are hashed, so the
 * rendered background is the observable proof of the active theme.
 */
export const THEME_BACKGROUNDS: Readonly<Record<string, string>> = {
  light: 'rgb(255, 255, 255)',
  dark: 'rgb(10, 10, 10)',
  matrix: 'rgb(0, 0, 0)',
  cyberpunk77: 'rgb(13, 13, 26)',
  grimdark: 'rgb(12, 10, 8)',
  clean: 'rgb(15, 17, 23)',
};

export class ThemeAssertionError extends NoRetryError {}
export class SceneAssertionError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'SceneAssertionError';
  }
}

export interface ThemeFacts {
  /** localStorage['stapler-theme'] as the page sees it. */
  stored: string | null;
  /** Computed body background (falls back to <html> when body is transparent). */
  background: string;
}

function themeForBackground(background: string): string | undefined {
  return Object.entries(THEME_BACKGROUNDS).find(([, bg]) => bg === background)?.[0];
}

/** Pure verdict, so the logic is testable without a browser. Returns null when the theme is right. */
export function themeVerdict(facts: ThemeFacts): string | null {
  if (facts.background === THEME_BACKGROUNDS[THEME_ID]) return null;
  const got = themeForBackground(facts.background) ?? `unknown (background ${facts.background})`;
  return `expected theme class ${THEME_ID}, got ${got}`;
}

export async function readThemeFacts(page: Page): Promise<ThemeFacts> {
  return page.evaluate(() => {
    const transparent = (c: string) => c === 'rgba(0, 0, 0, 0)' || c === 'transparent';
    const body = getComputedStyle(document.body).backgroundColor;
    let stored: string | null = null;
    try {
      stored = localStorage.getItem('stapler-theme');
    } catch {
      // storage blocked
    }
    return { stored, background: transparent(body) ? getComputedStyle(document.documentElement).backgroundColor : body };
  });
}

/** Throws unless the app actually rendered Grimdark. Cannot be softened: callers must not catch it. */
export async function assertGrimdark(page: Page): Promise<void> {
  const verdict = themeVerdict(await readThemeFacts(page));
  if (verdict) throw new ThemeAssertionError(verdict);
}

export interface Landing {
  /** Expected final pathname, e.g. '/backlog/board'. */
  path: string;
  /** Selector or role-based locator that exists only on the real page, not on a redirect target. */
  landmark: string;
}

export function pathMatches(url: string, expected: string): boolean {
  try {
    return new URL(url, 'http://placeholder').pathname.replace(/\/$/, '') === expected.replace(/\/$/, '');
  } catch {
    return false;
  }
}

/** A flag-gated page that redirected must fail the clip, not record the redirect target. */
export async function assertLanding(page: Page, landing: Landing): Promise<void> {
  if (!pathMatches(page.url(), landing.path)) {
    throw new SceneAssertionError(`expected final path ${landing.path}, got ${new URL(page.url()).pathname} (feature flag off or redirect?)`);
  }
  await page.locator(landing.landmark).first().waitFor({ state: 'visible', timeout: 15_000 });
}

export async function assertNoDialog(page: Page): Promise<void> {
  const open = await page.locator('[role=dialog]').count();
  if (open > 0) throw new SceneAssertionError(`${open} [role=dialog] element(s) open at capture; dismiss them in the scene`);
}

/** Quotes quoted by the tour's steps must appear on screen (P2-3). Returns the missing ones. */
export function missingQuotes(domText: string, quotes: readonly string[]): string[] {
  return quotes.filter((q) => !domText.includes(q));
}

/** Writes body.innerText to <outDir>/dom-text/<name>.txt for termcheck -mode=media-text and the leak scan. */
export async function dumpDomText(page: Page, outDir: string, name: string): Promise<string> {
  const text = await page.evaluate(() => document.body.innerText);
  const dir = path.join(outDir, 'dom-text');
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, `${name}.txt`), text);
  return text;
}
