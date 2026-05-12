// @feature ui:theme-background
// Regression: cockpitRoot lacked backgroundColor, so pages like /history showed
// white backgrounds even under dark themes. The fix sets backgroundColor:
// vars.color.background on cockpitRoot in layout.css.ts.

import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

const DARK_THEMES = ['matrix', 'dark', 'cyberpunk77', 'wh40k', 'clean'] as const;

// White in any browser-reported format
const WHITE = 'rgb(255, 255, 255)';

test.describe('theme-background', () => {
  for (const theme of DARK_THEMES) {
    test(`theme-background > /history has non-white background under ${theme} theme`, async ({ page }) => {
      // Set the theme in localStorage before the page loads so the FOUC script picks it up
      await page.addInitScript((t) => {
        localStorage.setItem('stapler-theme', t);
      }, theme);

      await page.goto(`${BASE_URL}/history`, { waitUntil: 'domcontentloaded', timeout: 15000 });
      await page.waitForSelector('#main-content', { timeout: 10000 });

      const bg = await page.evaluate(() => {
        const el = document.getElementById('main-content');
        return el ? window.getComputedStyle(el).backgroundColor : 'not-found';
      });

      // Without vars.color.background on cockpitRoot, main-content inherits from
      // body { background: var(--background) } which is always #fff regardless of theme.
      expect(bg, `Expected non-white background under ${theme} theme, got ${bg}`).not.toBe(WHITE);
    });
  }

  test('theme-background > /logs has non-white background under dark theme', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-theme', 'dark');
    });

    await page.goto(`${BASE_URL}/logs`, { waitUntil: 'domcontentloaded', timeout: 15000 });
    await page.waitForSelector('body', { timeout: 10000 });

    const bg = await page.evaluate(() => {
      const el = document.getElementById('main-content');
      return el ? window.getComputedStyle(el).backgroundColor : 'not-found';
    });

    expect(bg, `Expected non-white background on /logs under dark theme, got ${bg}`).not.toBe(WHITE);
  });
});
