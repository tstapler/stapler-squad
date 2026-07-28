import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
const _features = [FEATURE_CATALOG['session-update']] as const;
import { test, expect } from '@playwright/test';
import { SessionsPage } from './pages/SessionsPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

// Seeded by tests/demo/seed (see tests/demo/helpers.go): a Paused session with
// Program "claude", so this is deterministic without a beforeAll fixture.
const SEEDED_PAUSED_SESSION_TITLE = 'payment-stripe-integration';

test.describe('session-program-change', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });
  });

  test('overflow menu Change Program pre-fills the current program', async ({ page }) => {
    const sessionsPage = new SessionsPage(page);
    const card = sessionsPage.getSessionCard(SEEDED_PAUSED_SESSION_TITLE);
    await expect(card).toBeVisible({ timeout: 10000 });

    await card.getByRole('button', { name: /more session actions/i }).click();
    await page.getByRole('menuitem', { name: /change program/i }).click();

    const dialog = page.getByRole('dialog', { name: /change program/i });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('combobox')).toHaveValue('claude');
  });

  test('changing program via the overflow menu saves and updates the session list', async ({ page }) => {
    const sessionsPage = new SessionsPage(page);
    const card = sessionsPage.getSessionCard(SEEDED_PAUSED_SESSION_TITLE);
    await expect(card).toBeVisible({ timeout: 10000 });

    await card.getByRole('button', { name: /more session actions/i }).click();
    await page.getByRole('menuitem', { name: /change program/i }).click();

    const dialog = page.getByRole('dialog', { name: /change program/i });
    await expect(dialog).toBeVisible();

    const updateRequest = page.waitForRequest(
      (req) => req.url().includes('UpdateSession') && req.method() === 'POST'
    );
    await dialog.getByRole('combobox').selectOption('aider');
    await dialog.getByRole('button', { name: /^save$/i }).click();
    await updateRequest;

    await expect(dialog).not.toBeVisible();
    await expect(card.getByText('aider', { exact: true })).toBeVisible({ timeout: 10000 });
  });
});
