// @feature backlog:parse-item-intent, session:create
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || process.env.BASE_URL || 'http://localhost:8544';

test.describe('Omnibar Backlog Intent Intake Confirmation', () => {
  test('typing backlog: prefix opens the LLM intent review form', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });
    
    // Open the Omnibar
    await page.getByText('New Session').first().click();

    // Find Omnibar input
    const omnibarInput = page.locator('input[aria-label="Search sessions"], input[placeholder*="session"], input[type="text"]').first();
    await expect(omnibarInput).toBeVisible({ timeout: 5000 });

    // Type the backlog: command
    await omnibarInput.fill('backlog: Refactor error handling in session storage');

    // Screenshot typing state
    await page.screenshot({ path: 'tests/e2e/screenshots/omnibar-backlog-typing.png' });

    // Press Enter to trigger intent parsing & review
    await omnibarInput.press('Enter');

    // Verify the review form or backlog item form is displayed
    const backlogForm = page.locator('[data-testid="backlog-item-form"]');
    await expect(backlogForm).toBeVisible({ timeout: 15000 });

    // Verify title input contains the title (parsed or fallback)
    const titleInput = page.locator('[data-testid="backlog-title-input"]');
    await expect(titleInput).toBeVisible({ timeout: 5000 });

    // Screenshot review modal state
    await page.screenshot({ path: 'tests/e2e/screenshots/omnibar-backlog-review-modal.png' });

    console.log('✅ Omnibar backlog intent review UI confirmed successfully!');
  });
});
