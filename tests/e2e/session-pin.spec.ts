// @feature session:pin, session:unpin
import { test, expect } from '@playwright/test';
import { SessionsPage } from './pages/SessionsPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

// Seeded by tests/demo/seed: a Paused, non-archived session.
const SEEDED_SESSION_TITLE = 'payment-stripe-integration';

test.describe('session-pin', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });
  });

  test('pin moves the session into the Pinned section, survives reload, and unpin restores it', async ({ page }) => {
    const sessionsPage = new SessionsPage(page);
    const card = sessionsPage.getSessionCard(SEEDED_SESSION_TITLE);
    await expect(card).toBeVisible({ timeout: 10000 });

    await card.getByRole('button', { name: /more session actions/i }).click();
    const pin = page.getByTestId('session-pin-toggle');
    await expect(pin).toHaveAttribute('aria-checked', 'false');
    await pin.click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByTestId('pinned-section-header')).toBeVisible();

    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect(page.getByTestId('pinned-section-header')).toBeVisible({ timeout: 15000 });

    await sessionsPage.getSessionCard(SEEDED_SESSION_TITLE)
      .getByRole('button', { name: /more session actions/i }).click();
    const unpin = page.getByTestId('session-pin-toggle');
    await expect(unpin).toHaveAttribute('aria-checked', 'true');
    await unpin.click();
    await expect(page.getByTestId('pinned-section-header')).toHaveCount(0);
  });

  test('rolls back the optimistic pin when the RPC fails', async ({ page }) => {
    await page.route('**/PinSession', (route) => route.fulfill({ status: 500, body: '{}' }));
    const sessionsPage = new SessionsPage(page);
    const card = sessionsPage.getSessionCard(SEEDED_SESSION_TITLE);
    await expect(card).toBeVisible({ timeout: 10000 });

    await card.getByRole('button', { name: /more session actions/i }).click();
    await page.getByTestId('session-pin-toggle').click();

    await expect(page.getByTestId('pinned-section-header')).toHaveCount(0);
  });
});
