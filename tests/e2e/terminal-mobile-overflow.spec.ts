import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: terminal-mobile-overflow — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['terminal-mobile-overflow-menu'], // TODO: add to catalog
] as const;
import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();

const MOBILE_VIEWPORT = { width: 390, height: 844 };
const DESKTOP_VIEWPORT = { width: 1280, height: 800 };

const client = new SessionClient(BASE_URL);
let sessionId = '';

async function openTerminalView(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/?session=${sessionId}`, { waitUntil: 'domcontentloaded', timeout: 10000 });
  const toggle = page.getByTestId('toolbar-toggle');
  await expect(toggle).toBeVisible({ timeout: 8000 });
  // Ensure toolbar is expanded
  if ((await toggle.getAttribute('aria-expanded')) === 'false') await toggle.click();
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
}

test.describe('mobile toolbar overflow', () => {
  test.beforeAll(async () => {
    const title = `e2e-mobile-overflow-${Date.now()}`;
    sessionId = (await client.createIdleSession(title, '/tmp')).id;
    await client.waitForSessionsActive([title]);
  });

  test.afterAll(async () => {
    if (sessionId) await client.deleteSession(sessionId, true).catch(() => {});
  });

  test('More button is visible on mobile viewport', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await openTerminalView(page);

    await expect(page.getByTestId('toolbar-more-button')).toBeVisible();
  });

  test('primary buttons are visible without opening overflow', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await openTerminalView(page);

    await expect(page.getByRole('button', { name: /(show|hide) mobile keyboard/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /redraw terminal/i })).toBeVisible();
    // Paste lives in the overflow row on mobile
    await expect(page.getByRole('button', { name: /paste from clipboard/i })).not.toBeVisible();
  });

  test('clicking More reveals overflow row with secondary buttons', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await openTerminalView(page);

    await expect(page.getByTestId('toolbar-overflow-row')).not.toBeVisible();
    await page.getByTestId('toolbar-more-button').click();

    await expect(page.getByTestId('toolbar-overflow-row')).toBeVisible();
    await expect(page.getByTestId('toolbar-overflow-row').getByRole('button', { name: /copy/i })).toBeVisible();
    await expect(page.getByTestId('toolbar-overflow-row').getByRole('button', { name: /paste/i })).toBeVisible();
    await expect(page.getByTestId('toolbar-overflow-row').getByRole('button', { name: /bottom/i })).toBeVisible();
    await expect(page.getByTestId('toolbar-overflow-row').getByRole('button', { name: /clear/i })).toBeVisible();
  });

  test('clicking Less hides the overflow row', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await openTerminalView(page);

    await page.getByTestId('toolbar-more-button').click();
    await expect(page.getByTestId('toolbar-overflow-row')).toBeVisible();

    await page.getByTestId('toolbar-more-button').click();
    await expect(page.getByTestId('toolbar-overflow-row')).not.toBeVisible();
  });

  test('secondary buttons are hidden in toolbar on mobile', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await openTerminalView(page);

    // The secondaryGroup in the toolbar row should be hidden on mobile
    const secondary = page.getByTestId('toolbar-secondary');
    await expect(secondary).not.toBeVisible();
  });

  test('overflow row is hidden on desktop viewport', async ({ page }) => {
    await page.setViewportSize(DESKTOP_VIEWPORT);
    await openTerminalView(page);

    // On desktop the More button doesn't exist and the overflow row never renders
    await expect(page.getByTestId('toolbar-more-button')).not.toBeVisible();
    await expect(page.getByTestId('toolbar-overflow-row')).not.toBeAttached();
  });
});
