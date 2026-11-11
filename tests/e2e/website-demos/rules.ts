// @feature website-demos
import type { Page } from '@playwright/test';

export const RULE_NAME = '[data-testid="form-name-input"]';

/**
 * Opens the approval-rule builder for a new rule. Above 640px the page has no "add" control while
 * rules exist (the FAB is mobile-only), so a search with no hits surfaces the empty-state button.
 */
export async function openNewRuleBuilder(page: Page): Promise<void> {
  const search = page.locator('input[aria-label="Search rules"]');
  await search.fill('no such rule');
  await page.locator('[data-testid="add-rule-button"]').click();
  await search.clear();
  await page.locator(RULE_NAME).first().waitFor();
}
