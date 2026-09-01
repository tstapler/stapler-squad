import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
const _features = [FEATURE_CATALOG['backlog-get-item']] as const;
// @feature backlog:item-detail, backlog:board-page

import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';
import {
  createBacklogItemDirect,
  createBacklogItemDirectWithPublicId,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from './pages/BacklogMutations';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Backlog item ID + deep link', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ context, page }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    // Suppress both the backlog-specific tour and the general app-wide
    // onboarding modal (web-app/src/components/onboarding/OnboardingModal.tsx)
    // so neither ever intercepts clicks on the header's copy buttons.
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
      localStorage.setItem('stapler-squad:onboarded', 'true');
    });
  });

  test('detail view shows the item ID as visible, selectable text', async ({ page, request }) => {
    const { itemId, publicId } = await createBacklogItemDirectWithPublicId(request, { title: `e2e-id-visible-${Date.now()}` });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const idText = page.getByTestId('backlog-item-id');
    await expect(idText).toBeVisible();
    // Displays publicId (bl_...), not the raw UUID -- Story 2.3's
    // public_id-first display (BacklogItemDetail.tsx).
    await expect(idText).toHaveText(publicId);
    await expect(idText).toHaveCSS('user-select', 'text');
  });

  test('copies the item ID to the clipboard with a reverting confirmation state', async ({ page, request }) => {
    const { itemId, publicId } = await createBacklogItemDirectWithPublicId(request, { title: `e2e-id-copy-${Date.now()}` });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const copyButton = page.getByTestId('copy-item-id-button');
    // Wait for the item's publicId to actually be the one rendered before
    // clicking: the initial render can briefly show the fallback item.id
    // until the fetch carrying publicId lands, and the copy handler closes
    // over whatever item is current at click time.
    await expect(page.getByTestId('backlog-item-id')).toHaveText(publicId);
    await copyButton.click();
    await expect(copyButton).toHaveText(/Copied/);

    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toBe(publicId);

    await expect(copyButton).not.toHaveText(/Copied/, { timeout: 3000 });
  });

  test('copies a shareable deep link to the clipboard', async ({ page, request }) => {
    const { itemId, publicId } = await createBacklogItemDirectWithPublicId(request, { title: `e2e-link-copy-${Date.now()}` });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const copyLinkButton = page.getByTestId('copy-item-link-button');
    // See the "copies the item ID" test above for why this wait matters.
    await expect(page.getByTestId('backlog-item-id')).toHaveText(publicId);
    await copyLinkButton.click();
    await expect(copyLinkButton).toHaveText(/Copied/);

    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    // Story 2.3: Copy Link now builds an ssq:// deep link, not the old
    // ?item=<uuid> web URL.
    const host = new URL(BASE_URL).host;
    expect(clipboardText).toBe(`ssq://${host}/backlog/v1/${publicId}`);
  });

  test('clicking Copy ID then Copy Link within the confirmation window keeps the second button confirmed (no stale-timer race)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-copy-race-${Date.now()}` });

    // Uses page.clock (not waitForTimeout, per e2e-test-conventions.md) to
    // deterministically control the two copy buttons' 1.5s confirmation
    // timers rather than racing against real wall-clock time.
    await page.clock.install();
    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const copyIdButton = page.getByTestId('copy-item-id-button');
    const copyLinkButton = page.getByTestId('copy-item-link-button');
    await expect(copyIdButton).toBeVisible();

    await copyIdButton.click();
    await expect(copyIdButton).toHaveText(/Copied/);

    await page.clock.fastForward(500);
    await copyLinkButton.click();
    await expect(copyLinkButton).toHaveText(/Copied/);

    // At t=1500ms from the FIRST click, its old timer used to fire and clear
    // copiedField to null — dropping Copy Link's confirmation ~500ms early.
    await page.clock.fastForward(1000);
    await expect(copyLinkButton).toHaveText(/Copied/);
    await expect(copyIdButton).not.toHaveText(/Copied/);

    await page.clock.fastForward(600);
    await expect(copyLinkButton).not.toHaveText(/Copied/);
  });

  test('visiting /backlog?item=<id> opens the matching item detail pane directly', async ({ page, request }) => {
    const title = `e2e-direct-open-${Date.now()}`;
    const { itemId, publicId } = await createBacklogItemDirectWithPublicId(request, { title });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    const pane = page.getByTestId('backlog-item-detail');
    await expect(pane).toBeVisible();
    await expect(pane).toContainText(title);
    await expect(page.getByTestId('backlog-item-id')).toHaveText(publicId);
  });

  test('visiting /backlog/board?item=<id> restores the detail pane on the board itself', async ({ page, request }) => {
    const title = `e2e-board-deep-link-${Date.now()}`;
    const { itemId, publicId } = await createBacklogItemDirectWithPublicId(request, { title });

    await page.goto(`/backlog/board?item=${itemId}`, { waitUntil: 'domcontentloaded' });
    await expect(page).toHaveURL(/\/backlog\/board/);
    const pane = page.getByTestId('backlog-item-detail');
    await expect(pane).toBeVisible();
    await expect(pane).toContainText(title);
    await expect(page.getByTestId('backlog-item-id')).toHaveText(publicId);
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
