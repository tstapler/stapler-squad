import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
const _features = [FEATURE_CATALOG['backlog-get-item']] as const;
// @feature backlog:item-detail, backlog:board-page

import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';
import { createBacklogItemDirect, enableBacklogFeatureFlag, disableBacklogFeatureFlag } from './pages/BacklogMutations';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Backlog item ID + deep link', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  });

  test('detail view shows the item ID as visible, selectable text', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-id-visible-${Date.now()}` });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const idText = page.getByTestId('backlog-item-id');
    await expect(idText).toBeVisible();
    await expect(idText).toHaveText(itemId);
    await expect(idText).toHaveCSS('user-select', 'text');
  });

  test('copies the item ID to the clipboard with a reverting confirmation state', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-id-copy-${Date.now()}` });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const copyButton = page.getByTestId('copy-item-id-button');
    await copyButton.click();
    await expect(copyButton).toHaveText(/Copied/);

    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toBe(itemId);

    await expect(copyButton).not.toHaveText(/Copied/, { timeout: 3000 });
  });

  test('copies a shareable deep link to the clipboard', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-link-copy-${Date.now()}` });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const copyLinkButton = page.getByTestId('copy-item-link-button');
    await copyLinkButton.click();
    await expect(copyLinkButton).toHaveText(/Copied/);

    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toBe(`${BASE_URL}/backlog?item=${itemId}`);
  });

  test('visiting /backlog?item=<id> opens the matching item detail pane directly', async ({ page, request }) => {
    const title = `e2e-direct-open-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const pane = page.getByTestId('backlog-item-detail');
    await expect(pane).toBeVisible();
    await expect(pane).toContainText(title);
    await expect(page.getByTestId('backlog-item-id')).toHaveText(itemId);
  });

  test('visiting /backlog/board?item=<id> restores the detail pane on the board itself', async ({ page, request }) => {
    const title = `e2e-board-deep-link-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title });

    await page.goto(`/backlog/board?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    await expect(page).toHaveURL(/\/backlog\/board/);
    const pane = page.getByTestId('backlog-item-detail');
    await expect(pane).toBeVisible();
    await expect(pane).toContainText(title);
    await expect(page.getByTestId('backlog-item-id')).toHaveText(itemId);
  });

  test('an invalid item ID shows the not-found state on both /backlog and /backlog/board without crashing', async ({ page }) => {
    const missingId = '00000000-0000-0000-0000-000000000000';

    await page.goto(`/backlog?item=${missingId}`, { waitUntil: 'domcontentloaded' });
    await expect(page.getByTestId('backlog-item-detail')).toContainText('Item not found.');

    await page.goto(`/backlog/board?item=${missingId}`, { waitUntil: 'domcontentloaded' });
    await expect(page.getByTestId('backlog-item-detail')).toContainText('Item not found.');
  });

  test('a malformed item ID shows the not-found state without crashing', async ({ page }) => {
    await page.goto('/backlog?item=not-a-uuid', { waitUntil: 'domcontentloaded' });
    await expect(page.getByTestId('backlog-item-detail')).toContainText('Item not found.');
  });

  test('copy buttons are 44px+ touch targets on a mobile viewport', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-touch-target-${Date.now()}` });

    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    for (const testId of ['copy-item-id-button', 'copy-item-link-button']) {
      const box = await page.getByTestId(testId).boundingBox();
      expect(box, `${testId} not found`).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
  });

  test('the board page still opens via the list page deep link after a board round-trip (regression: no full-page redirect)', async ({ page, request }) => {
    const title = `e2e-board-click-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title });

    const backlogPage = new BacklogPage(page);
    await page.goto('/backlog/board', { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('[data-testid="backlog-board"]', { timeout: 15000 });

    const card = page.locator(`[data-item-id="${itemId}"]`).first();
    await card.click();

    await expect(page).toHaveURL(new RegExp(`/backlog/board/?\\?item=${itemId}`));
    await expect(page.getByTestId('backlog-item-detail')).toBeVisible();

    await backlogPage.closeItemDetail();
    await expect(page).toHaveURL(/\/backlog\/board\/?$/);
    await expect(page.getByTestId('backlog-item-detail')).not.toBeVisible();
  });
});
