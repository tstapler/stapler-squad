// @feature terminal-mobile-overflow-menu
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8543';

const MOBILE_VIEWPORT = { width: 390, height: 844 };
const DESKTOP_VIEWPORT = { width: 1280, height: 800 };

async function openTerminalView(page: import('@playwright/test').Page) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
  // Navigate to a session that has a terminal — wait for any session card to appear
  const sessionCard = page.locator('[data-testid="session-card"]').first();
  const hasSession = await sessionCard.isVisible({ timeout: 3000 }).catch(() => false);
  if (hasSession) {
    await sessionCard.click();
    await expect(page.locator('[data-testid="toolbar-toggle"]')).toBeVisible({ timeout: 5000 });
    // Ensure toolbar is expanded
    const toggle = page.locator('[data-testid="toolbar-toggle"]');
    const expanded = await toggle.getAttribute('aria-expanded');
    if (expanded === 'false') await toggle.click();
  }
}

test.describe('mobile toolbar overflow', () => {
  test('More button is visible on mobile viewport', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await openTerminalView(page);

    await expect(page.getByTestId('toolbar-more-button')).toBeVisible();
  });

  test('primary buttons are visible without opening overflow', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await openTerminalView(page);

    await expect(page.getByRole('button', { name: /show keys|hide keys/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /paste/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /image/i })).toBeVisible();
  });

  test('clicking More reveals overflow row with secondary buttons', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await openTerminalView(page);

    await expect(page.getByTestId('toolbar-overflow-row')).not.toBeVisible();
    await page.getByTestId('toolbar-more-button').click();

    await expect(page.getByTestId('toolbar-overflow-row')).toBeVisible();
    await expect(page.getByTestId('toolbar-overflow-row').getByRole('button', { name: /copy/i })).toBeVisible();
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
