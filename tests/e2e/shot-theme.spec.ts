// @feature ui:theme, website-demos
/**
 * R9 integration: the context the website recorder builds (initContext) renders Grimdark, for
 * both desktop and phone form factors. Verifies the html class differs from the default theme's
 * and the body background is Grimdark's token, the same facts assertGrimdark gates every shot on.
 */
import { test, expect, type Browser, type Page } from '@playwright/test';
import { baseUrl } from './helpers/base-url';
import { THEME_BACKGROUNDS, THEME_ID, assertGrimdark, readThemeFacts } from './website-demos/assertions';
import { initContext } from './website-demos/recorder';

const BASE_URL = baseUrl();
const VIEWPORTS = [
  { name: 'desktop', size: { width: 1280, height: 720 } },
  { name: 'phone', size: { width: 390, height: 844 } },
] as const;

async function loadWithTheme(browser: Browser, theme: string, size: { width: number; height: number }) {
  const context = await browser.newContext({ baseURL: BASE_URL, viewport: size });
  await initContext(context, theme);
  const page = await context.newPage();
  await page.goto('/');
  await page.getByRole('main').first().waitFor();
  const htmlClass = await page.evaluate(() => document.documentElement.className);
  return { context, page, htmlClass };
}

async function expectGrimdark(page: Page) {
  await assertGrimdark(page);
  expect((await readThemeFacts(page)).background).toBe(THEME_BACKGROUNDS[THEME_ID]);
}

test.describe('recorded shots are Grimdark', () => {
  for (const { name, size } of VIEWPORTS) {
    test(`${name} recorder context renders the grimdark html class and body background`, async ({ browser }) => {
      const clean = await loadWithTheme(browser, 'clean', size);
      const grim = await loadWithTheme(browser, THEME_ID, size);
      try {
        expect(grim.htmlClass, 'html class differs from the clean theme').not.toBe(clean.htmlClass);
        await expectGrimdark(grim.page);
      } finally {
        await clean.context.close();
        await grim.context.close();
      }
    });
  }
});
