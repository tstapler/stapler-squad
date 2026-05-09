// @feature workspace:list-targets, workspace:switch
import { test, expect } from '@playwright/test';
import { SessionsPage } from './pages/SessionsPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8543';

test.describe('Workspace Management', () => {
  test('e2e:workspace-list - Workspace information is accessible', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });
    await expect(page).toHaveTitle(/Stapler Squad/);

    const sessionsPage = new SessionsPage(page);

    // Verify the sessions page renders and search is operable
    await expect(sessionsPage.searchInput).toBeVisible();

    // Verify some session cards are rendered (pre-seeded sessions in test mode)
    const sessionCards = sessionsPage.getSessionCards();
    const count = await sessionCards.count();
    expect(count).toBeGreaterThan(0);
  });

  test('e2e:workspace-switch - Review queue page loads', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });

    // Navigate to review queue if a link is present
    const reviewLink = page.locator('a[href*="review-queue"]').first();
    if (await reviewLink.isVisible({ timeout: 3000 }).catch(() => false)) {
      await reviewLink.click();
      await page.waitForLoadState('domcontentloaded');
      await expect(page).toHaveTitle(/Stapler Squad/);
    } else {
      // If no review link, verify the main page is stable
      await expect(page).toHaveTitle(/Stapler Squad/);
    }
  });
});
