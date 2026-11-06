// @feature notification:push-click-handoff, session:deep-link

import { test, expect } from '@playwright/test';

/**
 * Story 5.5 / validation TP-3. The service worker posts {type:"notification-click", url} to an
 * open window; the page must route in-app without a document load, so terminals stay mounted.
 * A real push click (focus + postMessage from push-sw.js) is device-gated; this spec drives the
 * page half by dispatching the same message on navigator.serviceWorker. The SW half is covered by
 * web-app/src/lib/hooks/__tests__/pushClickHandoff.test.ts.
 */
test.describe('Push click handoff', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:onboarded', 'true');
    });
  });

  test('tp3_should_route_without_load_event', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForLoadState('load');
    // The app stamps ?window=<id> after mount; wait so that router.replace cannot race the handoff.
    await expect(page).toHaveURL(/[?&]window=/);

    await page.evaluate(() => {
      (window as unknown as { __handoffMarker: string }).__handoffMarker = 'kept';
    });
    let loads = 0;
    page.on('load', () => {
      loads += 1;
    });

    const target = '/?session=handoff-missing&tab=terminal&notification=n1';
    await page.evaluate((url) => {
      navigator.serviceWorker.dispatchEvent(
        new MessageEvent('message', { data: { type: 'notification-click', url } }),
      );
    }, target);

    await expect(page).toHaveURL(/session=handoff-missing/);
    await expect(page).toHaveURL(/notification=n1/);
    expect(loads).toBe(0);
    expect(
      await page.evaluate(() => (window as unknown as { __handoffMarker?: string }).__handoffMarker),
    ).toBe('kept');
  });

  test('foreign_origin_message_is_ignored', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForLoadState('load');
    const before = page.url();

    await page.evaluate(() => {
      navigator.serviceWorker.dispatchEvent(
        new MessageEvent('message', {
          data: { type: 'notification-click', url: 'https://evil.example/?session=x' },
        }),
      );
    });

    await expect(page).toHaveURL(before);
  });
});
