import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: backlog edit-mode buffering — mapped from @feature annotation
const _features = [
  FEATURE_CATALOG['backlog-update-item'],
  FEATURE_CATALOG['backlog-get-item'],
] as const;
// @feature backlog:watch, backlog:item-detail, backlog:inline-notice

/**
 * E2E tests for project_plans/backlog-event-driven-updates Story 5.3.2 /
 * Surface 6 (edit-mode buffered update + warn-before-overwrite) —
 * design/ux.md UX Acceptance Criteria #21, #22, #23, #24, #26.
 *
 * See backlog-live-updates.spec.ts's file header for the debug mutate
 * endpoint pattern used to simulate a background field change arriving
 * while the item is open for editing, without a second real browser context.
 */

import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';
import {
  createBacklogItemDirect,
  updateBacklogItemDirect,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from './pages/BacklogMutations';

test.describe('Backlog edit-mode buffered updates', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
    });
  });

  async function openInEditMode(page: import('@playwright/test').Page, title: string) {
    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.openItemDetail(title);
    await page.getByTestId('backlog-detail-edit').click();
    await expect(page.getByTestId('backlog-item-form')).toBeVisible();
    return backlogPage;
  }

  test('typing into the title field is unaffected by a background description change arriving live (UX AC #21)', async ({ page, request }) => {
    const title = `e2e-edit-buffer-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title, description: 'Original description', status: 'review' });

    await openInEditMode(page, title);

    const titleInput = page.getByTestId('backlog-title-input');
    await titleInput.fill('An in-progress edit that must not be clobbered');

    await updateBacklogItemDirect(request, itemId, { description: 'Changed elsewhere while editing' });

    await expect(page.getByTestId('backlog-detail-buffered-update-notice')).toBeVisible({ timeout: 5000 });
    await expect(titleInput).toHaveValue('An in-progress edit that must not be clobbered');
  });

  test('the buffered-update banner uses informational styling and does not block form interaction (UX AC #22)', async ({ page, request }) => {
    const title = `e2e-edit-buffer-style-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title, description: 'Original description', status: 'review' });

    await openInEditMode(page, title);
    await updateBacklogItemDirect(request, itemId, { description: 'Changed elsewhere' });

    const notice = page.getByTestId('backlog-detail-buffered-update-notice');
    await expect(notice).toBeVisible({ timeout: 5000 });

    // InlineNotice is role="status"/aria-live="polite" — NOT InlineError's
    // role="alert"/aria-live="assertive" family.
    await expect(notice).toHaveAttribute('role', 'status');
    await expect(notice).toHaveAttribute('aria-live', 'polite');

    // Rest of the form remains interactive while the banner is visible.
    await expect(page.getByTestId('backlog-title-input')).toBeEditable();
    await expect(page.getByTestId('backlog-form-submit')).toBeEnabled();
    await expect(page.getByTestId('backlog-form-cancel')).toBeEnabled();
  });

  test('clicking Reload applies the buffered update and clears the banner in the same action (UX AC #23, #24)', async ({ page, request }) => {
    const title = `e2e-edit-buffer-reload-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title, description: 'Original description', status: 'review' });

    await openInEditMode(page, title);

    // A second buffered event while the banner is already showing must not
    // stack a second banner instance — only the most recent (second)
    // event's data is what Reload applies.
    await updateBacklogItemDirect(request, itemId, { description: 'First background change' });
    const notice = page.getByTestId('backlog-detail-buffered-update-notice');
    await expect(notice).toBeVisible({ timeout: 5000 });

    await updateBacklogItemDirect(request, itemId, { description: 'Second background change' });
    await expect(notice).toHaveCount(1);

    // Reload is always available whenever the banner is visible.
    const reloadButton = notice.getByRole('button', { name: 'Reload' });
    await expect(reloadButton).toBeVisible();
    await expect(reloadButton).toBeEnabled();
    await reloadButton.click();

    await expect(notice).toHaveCount(0);
    await expect(page.getByTestId('backlog-description-input')).toHaveValue('Second background change');
  });

  test('clicking Save while a buffered update is pending warns before overwriting the concurrent change (UX AC #26)', async ({ page, request }) => {
    const title = `e2e-edit-buffer-save-conflict-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title, description: 'Original description', status: 'review' });

    await openInEditMode(page, title);

    const titleInput = page.getByTestId('backlog-title-input');
    await titleInput.fill('My unsaved title edit');

    await updateBacklogItemDirect(request, itemId, { description: 'Changed elsewhere' });
    await expect(page.getByTestId('backlog-detail-buffered-update-notice')).toBeVisible({ timeout: 5000 });

    await page.getByTestId('backlog-form-submit').click();

    // Save does not go through immediately — a confirm-style warning
    // appears instead, offering "Save Anyway" / "Reload".
    const conflictNotice = page.getByTestId('backlog-detail-save-conflict-notice');
    await expect(conflictNotice).toBeVisible({ timeout: 5000 });
    await expect(conflictNotice).toContainText('overwrite a change made elsewhere');

    const saveAnywayButton = conflictNotice.getByRole('button', { name: 'Save Anyway' });
    await expect(saveAnywayButton).toBeVisible();
    await expect(conflictNotice.getByRole('button', { name: 'Reload' })).toBeVisible();

    await saveAnywayButton.click();

    // Proceeds with the original save, exiting edit mode.
    await expect(page.getByTestId('backlog-item-form')).toHaveCount(0, { timeout: 5000 });
    await expect(page.getByTestId('backlog-detail-edit')).toBeVisible();
  });
});
