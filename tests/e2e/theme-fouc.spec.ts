// @feature ui:theme, accessibility
/**
 * Story 7.5a (Next baseline): the inline theme script applies the stored theme class to <html>
 * before first paint, degrades to `clean` on unknown values, applies `custom:` themes, and never
 * removes the font variable classes. Theme class names are hashed, so each is identified by being
 * distinct from `clean` (the default with no storage) rather than by name.
 */
import { test, expect, type Page } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const THEMES = ['matrix', 'cyberpunk77', 'wh40k', 'clean', 'light', 'dark'] as const;
const DARK_THEMES = ['matrix', 'cyberpunk77', 'wh40k', 'dark'] as const;

interface FirstPaintFacts {
  classes: string[];
  style: string;
  /** Resolved theme tokens (every custom property declared in the stylesheets), joined. */
  tokens: string;
}

/** Seeds localStorage before any page script, then records <html> at DOMContentLoaded and the first frame. */
async function loadWithStorage(page: Page, storage: Record<string, string>): Promise<FirstPaintFacts> {
  await new AppShellPage(page).skipOnboarding();
  await page.addInitScript((entries) => {
    for (const [key, value] of Object.entries(entries)) localStorage.setItem(key, value);
    const w = window as unknown as { __paint?: unknown };
    document.addEventListener('DOMContentLoaded', () => {
      const html = document.documentElement;
      const classes = Array.from(html.classList);
      const style = html.getAttribute('style') ?? '';
      requestAnimationFrame(() => {
        const names = new Set<string>();
        for (const sheet of Array.from(document.styleSheets)) {
          try {
            for (const rule of Array.from(sheet.cssRules)) {
              for (const name of Array.from((rule as CSSStyleRule).style ?? [])) if (name.startsWith('--')) names.add(name);
            }
          } catch {
            // cross-origin sheet
          }
        }
        const computed = getComputedStyle(html);
        const tokens = Array.from(names).sort().map((n) => computed.getPropertyValue(n).trim()).join('|');
        w.__paint = { classes, style, tokens };
      });
    });
  }, storage);
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => (window as unknown as { __paint?: unknown }).__paint !== undefined);
  return page.evaluate(() => (window as unknown as { __paint: FirstPaintFacts }).__paint);
}

test.describe('theme FOUC prevention (Next baseline)', () => {
  test('each stored theme is applied at DOMContentLoaded and dark themes resolve their own tokens on the first frame', async ({ browser }) => {
    const byTheme = new Map<string, FirstPaintFacts>();
    for (const theme of THEMES) {
      const context = await browser.newContext();
      const page = await context.newPage();
      byTheme.set(theme, await loadWithStorage(page, { 'stapler-theme': theme }));
      await context.close();
    }
    const clean = byTheme.get('clean')!;

    const themeClassOf = (facts: FirstPaintFacts) => facts.classes.filter((c) => !clean.classes.includes(c) || facts === clean);
    const distinct = new Set(THEMES.map((t) => byTheme.get(t)!.classes.join(' ')));
    expect(distinct.size, 'every theme yields a distinct <html> class list').toBe(THEMES.length);

    for (const theme of THEMES.filter((t) => t !== 'clean')) {
      expect(themeClassOf(byTheme.get(theme)!).length, `${theme} adds its own class`).toBeGreaterThan(0);
    }
    // <body>'s own background is theme-independent on Next (globals.css --background), so the
    // first-frame check reads the theme tokens instead: a late theme would still show clean's.
    for (const theme of DARK_THEMES) {
      expect(byTheme.get(theme)!.tokens, `${theme} first-frame theme tokens`).not.toBe(clean.tokens);
    }
  });

  test('font variable classes survive every theme', async ({ browser }) => {
    const fonts = new Map<string, string[]>();
    for (const theme of THEMES) {
      const context = await browser.newContext();
      const facts = await loadWithStorage(await context.newPage(), { 'stapler-theme': theme });
      await context.close();
      fonts.set(theme, facts.classes);
    }
    // Exactly one class differs between themes; the rest (next/font variables) are shared.
    const sizes = new Set(Array.from(fonts.values()).map((c) => c.length));
    expect(sizes.size, 'class count is identical across themes').toBe(1);
    const shared = fonts.get('clean')!.filter((c) => Array.from(fonts.values()).every((list) => list.includes(c)));
    expect(shared.length, 'font variable classes present under every theme').toBeGreaterThanOrEqual(4);
  });

  test('a custom: theme applies its class and CSS variables before first paint', async ({ page }) => {
    const custom = { id: 'custom:e2e', cls: 'e2e-custom-theme', props: { '--e2e-probe': '42px' } };
    const facts = await loadWithStorage(page, {
      'stapler-theme': 'custom:e2e',
      'stapler-theme-custom': JSON.stringify(custom),
    });
    expect(facts.classes).toContain('e2e-custom-theme');
    expect(facts.style).toContain('--e2e-probe');
    expect(await page.evaluate(() => document.documentElement.style.getPropertyValue('--e2e-probe'))).toBe('42px');
  });

  test('an unknown stored theme degrades to clean and the page stays usable', async ({ browser }) => {
    const cleanContext = await browser.newContext();
    const clean = await loadWithStorage(await cleanContext.newPage(), {});
    await cleanContext.close();

    const context = await browser.newContext();
    const page = await context.newPage();
    const stale = await loadWithStorage(page, { 'stapler-theme': 'removed-theme-from-an-old-build' });
    expect(stale.classes.join(' ')).toBe(clean.classes.join(' '));
    await expect(new AppShellPage(page).main).toBeVisible();
    await context.close();
  });
});
