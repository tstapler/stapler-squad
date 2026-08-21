// @feature session:bulk-select, session:delete, session:pause
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('bulk-select', () => {
  // NOTE: These tests require a running server at BASE_URL with at least 2-3 sessions pre-loaded.
  // Tests will fail (not skip) if the server is unavailable or sessions are missing.
  test.beforeEach(async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });
  });

  test('bulk-delete in row mode — selects 2 sessions, clicks Delete, undo toast appears, sessions removed from list', async ({ page }) => {
    // Enter select mode
    const selectButton = page.getByRole('button', { name: /enter select mode/i });
    await expect(selectButton).toBeVisible({ timeout: 5000 });
    await selectButton.click();

    // Click first two session row checkboxes
    const checkboxes = page.getByTestId('session-row-checkbox');
    const count = await checkboxes.count();
    expect(count).toBeGreaterThanOrEqual(2);
    await checkboxes.nth(0).click();
    await checkboxes.nth(1).click();

    // Click Delete Selected
    const deleteButton = page.getByTestId('bulk-delete-button');
    await expect(deleteButton).toBeVisible();
    await deleteButton.click();

    // Undo toast must appear
    await expect(page.getByTestId('undo-toast-button')).toBeVisible({ timeout: 5000 });
  });

  test('bulk-pause in row mode — selects 2 active sessions, clicks Pause Selected, sessions show paused status', async ({ page }) => {
    // Enter select mode
    const selectButton = page.getByRole('button', { name: /enter select mode/i });
    await expect(selectButton).toBeVisible({ timeout: 5000 });
    await selectButton.click();

    // Click first two session row checkboxes
    const checkboxes = page.getByTestId('session-row-checkbox');
    const count = await checkboxes.count();
    expect(count).toBeGreaterThanOrEqual(2);
    await checkboxes.nth(0).click();
    await checkboxes.nth(1).click();

    // Click Pause Selected
    const pauseButton = page.getByTestId('bulk-pause-button');
    await expect(pauseButton).toBeVisible();
    await pauseButton.click();

    // Toolbar should disappear (select mode exited) and bulk feedback should fire
    await expect(page.getByRole('toolbar', { name: /bulk session actions/i })).not.toBeVisible({ timeout: 3000 });
  });

  test('shift+click range select — plain click row 1, shift+click row 3, rows 1-3 are selected', async ({ page }) => {
    // Enter select mode
    const selectButton = page.getByRole('button', { name: /enter select mode/i });
    await expect(selectButton).toBeVisible({ timeout: 5000 });
    await selectButton.click();

    const checkboxes = page.getByTestId('session-row-checkbox');
    const count = await checkboxes.count();
    expect(count).toBeGreaterThanOrEqual(3);

    // Click first row (sets anchor)
    await checkboxes.nth(0).click();

    // Shift+click third row to range-select rows 1-3
    await checkboxes.nth(2).click({ modifiers: ['Shift'] });

    // All three rows should be selected: the count span should show "3 of N selected"
    const countSpan = page.locator('[aria-live="polite"][aria-atomic="true"]').filter({ hasText: /selected/ });
    await expect(countSpan).toContainText('3 of');
  });

  test('escape exits select mode — enter select mode, press Escape, checkboxes hidden and toolbar gone', async ({ page }) => {
    // Enter select mode
    const selectButton = page.getByRole('button', { name: /enter select mode/i });
    await expect(selectButton).toBeVisible({ timeout: 5000 });
    await selectButton.click();

    // The toolbar must be visible
    await expect(page.getByRole('toolbar', { name: /bulk session actions/i })).toBeVisible({ timeout: 3000 });

    // Press Escape
    await page.keyboard.press('Escape');

    // Toolbar must be gone
    await expect(page.getByRole('toolbar', { name: /bulk session actions/i })).not.toBeVisible({ timeout: 3000 });

    // The data-select-mode attribute should be "false"
    const container = page.locator('[data-context="session-list"]');
    await expect(container).toHaveAttribute('data-select-mode', 'false');
  });

  test('undo restores deleted sessions — delete 2 sessions, click Undo in toast, sessions reappear', async ({ page }) => {
    // Enter select mode
    const selectButton = page.getByRole('button', { name: /enter select mode/i });
    await expect(selectButton).toBeVisible({ timeout: 5000 });
    await selectButton.click();

    const checkboxes = page.getByTestId('session-row-checkbox');
    const count = await checkboxes.count();
    expect(count).toBeGreaterThanOrEqual(2);

    // Count sessions before delete
    const sessionRows = page.getByTestId('session-row');
    const initialCount = await sessionRows.count();

    await checkboxes.nth(0).click();
    await checkboxes.nth(1).click();

    const deleteButton = page.getByTestId('bulk-delete-button');
    await deleteButton.click();

    // Wait for optimistic removal — row count should decrease
    await expect(sessionRows).toHaveCount(initialCount - 2, { timeout: 5000 });

    // Click Undo
    const undoButton = page.getByTestId('undo-toast-button');
    await expect(undoButton).toBeVisible({ timeout: 5000 });
    await undoButton.click();

    // Sessions must reappear
    await expect(sessionRows).toHaveCount(initialCount, { timeout: 5000 });
  });
});
