// @feature notification-tray
/**
 * The desktop tray handle can be dragged vertically along the right edge, the
 * position survives a reload, a plain click still toggles the tray, and Reset
 * restores the default.
 */
import { test, expect } from '@playwright/test';
import { TrayHandlePage } from './pages/NotificationTrayHandle';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';
const DRAG_PX = 160;

test.use(profiles.V1.use);

test.describe('notification tray handle drag', () => {
  test.beforeEach(async ({ context, page }) => {
    await context.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  });

  test('drag_should_move_the_handle_persist_across_reload_and_not_open_the_tray', async ({ page }) => {
    const bell = new TrayHandlePage(page);
    await expect(bell.handle).toBeVisible();
    const before = await bell.top();

    await bell.dragBy(DRAG_PX);

    await expect.poll(() => bell.top()).toBeGreaterThan(before + DRAG_PX / 2);
    await expect(bell.handle).toHaveAttribute('aria-expanded', 'false');

    const moved = await bell.top();
    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect.poll(() => bell.top()).toBeCloseTo(moved, -1);
  });

  test('click_without_movement_should_open_the_tray', async ({ page }) => {
    const bell = new TrayHandlePage(page);
    await bell.handle.click();
    await expect(bell.handle).toHaveAttribute('aria-expanded', 'true');
  });

  test('reset_should_restore_the_default_position', async ({ page }) => {
    const bell = new TrayHandlePage(page);
    const original = await bell.top();
    await bell.dragBy(DRAG_PX);
    await expect.poll(() => bell.top()).toBeGreaterThan(original + DRAG_PX / 2);

    await bell.handle.click();
    await bell.openSettings();
    await bell.resetButton.click();

    await expect.poll(() => bell.top()).toBeCloseTo(original, -1);
  });
});
