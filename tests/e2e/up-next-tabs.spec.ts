// @feature unfinished-github-prs
/**
 * Up Next PRs panel narrow-screen reflow (WCAG 1.4.10 at 320 CSS px / 400% zoom).
 * Runs against the not-connected / empty panel, which needs no GitHub data; real PR cards are
 * covered by the Jest class-token checks and the manual pass in plan Task 6.1.1d.
 */
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Up Next PRs panel reflow', () => {
  test.use({ viewport: { width: 320, height: 640 } });

  test('has no horizontal overflow at 320px', async ({ page }) => {
    await page.goto(`${BASE_URL}/unfinished?tab=prs`, { waitUntil: 'domcontentloaded' });
    await expect(page.getByRole('tab', { name: /^PRs/ })).toHaveAttribute('aria-selected', 'true');
    await expect(page.locator('#github-prs-list')).toBeVisible();

    const overflow = await page.evaluate(() => {
      const panel = document.querySelector('#github-prs-list') as HTMLElement;
      return {
        document: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        panel: panel.scrollWidth - panel.clientWidth,
      };
    });
    expect(overflow.document).toBeLessThanOrEqual(0);
    expect(overflow.panel).toBeLessThanOrEqual(0);
  });
});
