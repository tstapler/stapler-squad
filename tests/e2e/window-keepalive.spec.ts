// @feature window:keep-alive, window:switch, a11y:window-switch
/**
 * Keep-alive windows (Story 4.1.3, UX criteria 10 and 11). `window_keep_alive` is switched on for the isolated
 * test server (never a live instance) and restored afterwards. Not covered here: criteria 9, 12, 34 (Story 4.1.2's
 * placeholder is not built yet), geometry parity (Story 4.1.5) and the echo-latency p95 (needs the perf rig).
 */
import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import { WindowTabStripPage } from './pages/WindowTabStripPage';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const FLAG = 'window_keep_alive';
const rpc = (name: string) => `${BASE_URL}/api/session.v1.SessionService/${name}`;
const json = { 'Content-Type': 'application/json' };

async function flagEnabled(request: APIRequestContext): Promise<boolean> {
  const res = await request.post(rpc('GetFeatureFlags'), { headers: json, data: {} });
  const flags = (await res.json()).flags as { name: string; enabled?: boolean }[];
  return flags.find((f) => f.name === FLAG)?.enabled ?? false;
}

async function setFlag(request: APIRequestContext, enabled: boolean): Promise<void> {
  await request.post(rpc('UpdateFeatureFlag'), { headers: json, data: { name: FLAG, enabled } });
  expect(await flagEnabled(request)).toBe(enabled); // read back, not assumed
}

async function seedTwoWindows(page: Page): Promise<WindowTabStripPage> {
  await page.goto('/');
  const strip = new WindowTabStripPage(page);
  await strip.waitForLoaded();
  await strip.createWindow();
  await expect(strip.tabs).toHaveCount(2);
  await strip.switchToTab('Window 1');
  await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 1');
  return strip;
}

let flagBefore = false;

test.beforeAll(async ({ request }) => {
  flagBefore = await flagEnabled(request);
  await setFlag(request, true);
});

test.afterAll(async ({ request }) => {
  await setFlag(request, flagBefore);
});

test.beforeEach(async ({ context }) => {
  await context.addInitScript(() => localStorage.setItem('stapler-squad:onboarded', 'true'));
});

test.describe('window keep-alive: hidden windows stay mounted but inert', () => {
  test('hiddenWindow_should_BeInertDisplayNoneAndUnreachableByTab_When_KeepAliveOn', async ({ page }) => {
    await seedTwoWindows(page);
    const panels = page.getByTestId('window-panel');
    await expect(panels).toHaveCount(2);
    const hidden = panels.and(page.locator('[hidden]'));
    await expect(hidden).toHaveCount(1);
    await expect(hidden).toHaveAttribute('inert', '');
    await expect(hidden).toHaveCSS('display', 'none');

    // Tab until focus returns to the first focusable element; bound = focusable count + 5, never a fixed budget.
    const reachedHiddenPanel = await page.evaluate(() => {
      const focusables = Array.from(document.querySelectorAll<HTMLElement>('a[href],button,input,select,textarea,[tabindex]'));
      return focusables.some((el) => el.closest('[hidden]') && el.tabIndex >= 0 && el.offsetParent !== null);
    });
    expect(reachedHiddenPanel).toBe(false);
    const bound = (await page.locator('a[href],button,input,select,textarea,[tabindex="0"]').count()) + 5;
    for (let i = 0; i < bound; i += 1) {
      await page.keyboard.press('Tab');
      expect(await page.evaluate(() => !!document.activeElement?.closest('[hidden]'))).toBe(false);
    }
  });

  test('keypress_should_BeHandledOnce_When_TwoWindowsMounted', async ({ page }) => {
    const strip = await seedTwoWindows(page);
    const leavesIn = (panel: ReturnType<Page['getByTestId']>) => panel.locator('[data-testid^="pane-leaf-"]');
    const panels = page.getByTestId('window-panel');
    const active = panels.and(page.locator(':not([hidden])'));
    const hidden = panels.and(page.locator('[hidden]'));
    const activeBefore = await leavesIn(active).count();
    const hiddenBefore = await hidden.evaluate((el) => el.querySelectorAll('[data-testid^="pane-leaf-"]').length);

    await leavesIn(active).first().click();
    await page.keyboard.press('Control+\\'); // split pane vertically: one registration, one handler
    await expect(leavesIn(active)).toHaveCount(activeBefore + 1);
    expect(await hidden.evaluate((el) => el.querySelectorAll('[data-testid^="pane-leaf-"]').length)).toBe(hiddenBefore);
    await expect(strip.tabs).toHaveCount(2);
  });
});
