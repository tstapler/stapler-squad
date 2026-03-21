import { test, expect } from '@playwright/test';

// Base URL is configured in playwright.config.ts to use the test server (port 8544)
const BASE_URL = 'http://localhost:8544';

test.describe('Smoke Tests', () => {
  test('review queue page loads successfully', async ({ page }) => {
    // Navigate and wait for DOM content (WebSocket keeps network busy, so don't wait for networkidle)
    await page.goto(`${BASE_URL}/review-queue`, {
      waitUntil: 'domcontentloaded',
      timeout: 10000
    });

    // Check page title
    await expect(page).toHaveTitle(/Stapler Squad/);

    // Check that review queue component is present with longer timeout for hydration
    await expect(page.locator('[data-testid="review-queue"]')).toBeVisible({ timeout: 10000 });

    // Verify panel title is present using role-based locator (more specific than text match)
    await expect(page.getByRole('heading', { name: 'Review Queue' })).toBeVisible();

    // Verify filters are rendered (always present regardless of content)
    await expect(page.locator('text=Priority:')).toBeVisible();
    await expect(page.locator('text=Reason:')).toBeVisible();

    console.log('✅ Review queue page loaded successfully');
  });

  test('home page loads successfully', async ({ page }) => {
    await page.goto(BASE_URL);

    await expect(page).toHaveTitle(/Stapler Squad/);

    console.log('✅ Home page loaded successfully');
  });

  test('navigation header is present', async ({ page }) => {
    await page.goto(BASE_URL);

    // Check for navigation links
    await expect(page.getByText('Sessions')).toBeVisible();
    await expect(page.getByText('Review Queue')).toBeVisible();
    await expect(page.getByText('New Session')).toBeVisible();

    console.log('✅ Navigation header is present');
  });
});
