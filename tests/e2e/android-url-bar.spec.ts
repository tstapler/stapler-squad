// @feature android-url-bar-overflow
/**
 * Regression tests for content hidden behind the Android Chrome URL bar.
 *
 * Chrome on Android shows a ~66 px URL bar when the page is at the top. Elements
 * sized with 100vh or 100lvh overflow into that bar; 100dvh and
 * var(--viewport-height, 100dvh) shrink correctly with it.
 *
 * ESLint and stylelint enforce the var(--viewport-height) pattern at author time.
 * These tests verify the runtime behaviour: critical chrome (BottomNav) stays inside
 * the reduced viewport and the page produces no horizontal overflow.
 */
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

// Pixel 6 Pro in portrait. The second viewport simulates the URL bar visible,
// which reduces usable height by ~66 px (the bar's CSS-pixel height on this device).
const ANDROID_FULL     = { width: 412, height: 915 };
const ANDROID_URL_BAR  = { width: 412, height: 849 };

test.describe('android-url-bar-overflow', () => {
  test('BottomNav stays within viewport when URL bar is visible', async ({ page }) => {
    await page.setViewportSize(ANDROID_URL_BAR);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10_000 });

    const bottomNav = page.getByRole('navigation', { name: 'Bottom navigation' });
    await expect(bottomNav).toBeVisible();

    const box = await bottomNav.boundingBox();
    expect(box).not.toBeNull();
    // Bottom edge must not exceed the reduced viewport (would be hidden behind the URL bar)
    expect(box!.y + box!.height).toBeLessThanOrEqual(ANDROID_URL_BAR.height);
  });

  test('page has no horizontal overflow on Android viewport with URL bar', async ({ page }) => {
    await page.setViewportSize(ANDROID_URL_BAR);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10_000 });

    const hasHorizontalOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    );
    expect(hasHorizontalOverflow).toBe(false);
  });

  test('BottomNav adapts when viewport shrinks from full to URL-bar height', async ({ page }) => {
    await page.setViewportSize(ANDROID_FULL);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10_000 });
    await expect(page.getByRole('navigation', { name: 'Bottom navigation' })).toBeVisible();

    // Simulate the URL bar appearing (viewport height drops)
    await page.setViewportSize(ANDROID_URL_BAR);

    const box = await page.getByRole('navigation', { name: 'Bottom navigation' }).boundingBox();
    expect(box).not.toBeNull();
    expect(box!.y + box!.height).toBeLessThanOrEqual(ANDROID_URL_BAR.height);
  });

  test('no fixed-position element overflows viewport bottom on Android', async ({ page }) => {
    await page.setViewportSize(ANDROID_URL_BAR);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10_000 });

    const overflowingFixed = await page.evaluate((viewportHeight) => {
      const fixed = Array.from(document.querySelectorAll('*')).filter((el) => {
        const style = window.getComputedStyle(el);
        return style.position === 'fixed';
      });
      return fixed
        .map((el) => {
          const r = el.getBoundingClientRect();
          return { tag: el.tagName, id: (el as HTMLElement).dataset.testid ?? '', bottom: r.bottom };
        })
        .filter((el) => el.bottom > viewportHeight);
    }, ANDROID_URL_BAR.height);

    expect(overflowingFixed).toHaveLength(0);
  });
});
