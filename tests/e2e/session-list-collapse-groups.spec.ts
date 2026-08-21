// @feature session-list
import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
const _features = [
  FEATURE_CATALOG['session-list'],
] as const;

import { test, expect, Page } from '@playwright/test';
import { SessionClient } from './helpers/session-client';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

/**
 * E2E tests for collapsible session-list categories.
 *
 * Covers acceptance criteria:
 *   0. Toggle collapses a category's sessions
 *   1. Toggle again re-expands
 *   2. Keyboard-operable (Enter/Space), correct aria-expanded
 *   3. Collapsed state persists across reload (localStorage)
 *   4. Works identically in row and card view modes (row mode is exercised
 *      here — card view mode is not currently reachable via any app UI
 *      control, and is covered instead by a component test:
 *      web-app/src/components/sessions/__tests__/SessionList.collapse.test.tsx)
 *   5. Group header session count stays visible when collapsed
 *   6. No toggle rendered for GroupingStrategy.None
 *   8. Independent SessionList instances keep independent collapse state
 *      (covered by the same component test — split-pane is not present in
 *      this build)
 */

async function seedTwoCategorySessions(ts: number) {
  const client = new SessionClient(BASE_URL);
  const category = `e2e-collapse-backlog-${ts}`;
  const otherCategory = `e2e-collapse-working-${ts}`;
  await client.createSession({ title: `e2e-collapse-a-${ts}`, path: '/tmp', program: 'bash', category });
  await client.createSession({ title: `e2e-collapse-b-${ts}`, path: '/tmp', program: 'bash', category });
  await client.createSession({ title: `e2e-collapse-c-${ts}`, path: '/tmp', program: 'bash', category: otherCategory });
  return { category, otherCategory };
}

async function gotoHome(page: Page) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForSelector('[aria-label="Search sessions"]', { timeout: 15000 });
  // Category is the default grouping strategy, but a prior test run in the
  // same storage context may have left a different one selected.
  const groupBy = page.getByLabel('Group sessions by');
  if (await groupBy.isVisible().catch(() => false)) {
    await groupBy.selectOption('category');
  }
}

test.describe('session-list-collapse-groups', () => {
  test('clicking a category toggle collapses and re-expands its sessions (AC 0, 1, 5)', async ({ page }) => {
    const ts = Date.now();
    const { category } = await seedTwoCategorySessions(ts);
    await gotoHome(page);

    const header = page.getByRole('heading', { name: new RegExp(`${category} \\(2\\)`) });
    await expect(header).toBeVisible({ timeout: 10000 });
    const toggle = header.getByTestId('category-collapse-toggle');
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');

    const rowA = page.getByText(`e2e-collapse-a-${ts}`);
    const rowB = page.getByText(`e2e-collapse-b-${ts}`);
    await expect(rowA).toBeVisible();
    await expect(rowB).toBeVisible();

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await expect(rowA).not.toBeVisible();
    await expect(rowB).not.toBeVisible();
    // Count badge in the header stays visible while collapsed.
    await expect(header).toBeVisible();
    await expect(header).toContainText(`${category} (2)`);

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(rowA).toBeVisible();
    await expect(rowB).toBeVisible();
  });

  test('toggle is keyboard-operable with Enter and Space (AC 2)', async ({ page }) => {
    const ts = Date.now();
    const { category } = await seedTwoCategorySessions(ts);
    await gotoHome(page);

    const header = page.getByRole('heading', { name: new RegExp(`${category} \\(2\\)`) });
    const toggle = header.getByTestId('category-collapse-toggle');
    await expect(toggle).toBeVisible({ timeout: 10000 });

    await toggle.focus();
    await page.keyboard.press('Enter');
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');

    await page.keyboard.press('Space');
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
  });

  test('collapsed state persists across page reload (AC 3)', async ({ page }) => {
    const ts = Date.now();
    const { category } = await seedTwoCategorySessions(ts);
    await gotoHome(page);

    const header = page.getByRole('heading', { name: new RegExp(`${category} \\(2\\)`) });
    const toggle = header.getByTestId('category-collapse-toggle');
    await expect(toggle).toBeVisible({ timeout: 10000 });
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');

    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForSelector('[aria-label="Search sessions"]', { timeout: 15000 });

    const headerAfterReload = page.getByRole('heading', { name: new RegExp(`${category} \\(2\\)`) });
    const toggleAfterReload = headerAfterReload.getByTestId('category-collapse-toggle');
    await expect(toggleAfterReload).toBeVisible({ timeout: 10000 });
    await expect(toggleAfterReload).toHaveAttribute('aria-expanded', 'false');
    await expect(page.getByText(`e2e-collapse-a-${ts}`)).not.toBeVisible();
  });

  test('no collapse toggle is rendered when grouping strategy is None (AC 6)', async ({ page }) => {
    const ts = Date.now();
    await seedTwoCategorySessions(ts);
    await gotoHome(page);

    const groupBy = page.getByLabel('Group sessions by');
    await expect(groupBy).toBeVisible({ timeout: 10000 });
    await groupBy.selectOption('none');

    await expect(page.getByText(`e2e-collapse-a-${ts}`)).toBeVisible({ timeout: 10000 });
    await expect(page.getByTestId('category-collapse-toggle')).toHaveCount(0);
  });
});
