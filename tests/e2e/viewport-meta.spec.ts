// @feature ui:viewport, accessibility
/**
 * Story 7.6a: the viewport meta tag and <html lang> are identical on every build, and the bottom
 * nav resolves env(safe-area-inset-bottom) padding. The resolved padding is 0 in desktop
 * Chromium (no inset), so the spec pins the declared padding rule via a probe element instead.
 */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const VIEWPORT_CONTENT = 'width=device-width, initial-scale=1, maximum-scale=5, viewport-fit=cover';
const ROUTES = ['/', '/history', '/settings', '/review-queue', '/rules'];

test.describe('viewport meta', () => {
  test.beforeEach(async ({ page }) => {
    await new AppShellPage(page).skipOnboarding();
  });

  for (const route of ROUTES) {
    test(`meta viewport and html lang on ${route}`, async ({ page }) => {
      await page.goto(`${BASE_URL}${route}`, { waitUntil: 'domcontentloaded' });
      await expect
        .poll(() => page.evaluate(() => document.querySelector('meta[name=viewport]')?.getAttribute('content')))
        .toBe(VIEWPORT_CONTENT);
      await expect.poll(() => page.evaluate(() => document.documentElement.lang)).toBe('en');
    });
  }

  test('bottom nav resolves env(safe-area-inset-bottom) padding at 390x844', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    const bottomNav = page.getByRole('navigation', { name: 'Bottom navigation' });
    await expect(bottomNav).toBeVisible();

    const paddingBottom = await bottomNav.evaluate((el) => getComputedStyle(el).paddingBottom);
    // Chromium reports 0px for env(safe-area-inset-bottom) without a device inset; a literal
    // `env(...)` string or NaN would mean the declaration was dropped or left unresolved.
    expect(paddingBottom).toMatch(/^\d+(\.\d+)?px$/);

    const resolvesEnv = await page.evaluate(() => {
      const probe = document.createElement('div');
      probe.style.paddingBottom = 'env(safe-area-inset-bottom)';
      document.body.appendChild(probe);
      const resolved = getComputedStyle(probe).paddingBottom;
      probe.remove();
      return resolved;
    });
    expect(resolvesEnv).toMatch(/^\d+(\.\d+)?px$/);
    test.info().annotations.push({ type: 'baseline', description: `bottom-nav padding-bottom=${paddingBottom}` });
  });
});
