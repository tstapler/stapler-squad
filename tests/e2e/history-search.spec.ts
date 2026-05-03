// @feature history:search
import { test, expect } from '@playwright/test';
import { SessionsPage } from './pages/SessionsPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('History Search', () => {
  test('e2e:history-search - History search UI is accessible', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });
    await expect(page).toHaveTitle(/Stapler Squad/);

    const sessionsPage = new SessionsPage(page);

    // Type a search query and wait for the search input to reflect it
    await sessionsPage.searchInput.fill('payment');
    await expect(sessionsPage.searchInput).toHaveValue('payment');

    // Verify the page is still stable and showing content
    await expect(page).toHaveTitle(/Stapler Squad/);
    const bodyText = await page.locator('body').textContent();
    expect(bodyText).toBeTruthy();

    // Clear the search and wait for the input to be empty
    await sessionsPage.searchInput.clear();
    await expect(sessionsPage.searchInput).toHaveValue('');

    await expect(page).toHaveTitle(/Stapler Squad/);
  });
});
