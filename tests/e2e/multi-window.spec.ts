// @feature window:create, window:switch, window:close, window:rename
/**
 * UX-acceptance sweep for the multi-window feature — see
 * project_plans/multi-window/implementation/validation.md's "Requirement -> Test
 * Mapping" table (rows for Surfaces 1-10) for the exact behaviors each test below
 * proves. Cross-tab/URL-sync coverage lives in multi-window-cross-tab.spec.ts (REQ-10);
 * these tests are single-tab, single-window-strip interaction checks.
 */
import { test, expect, Page } from '@playwright/test';
import { WindowTabStripPage } from './pages/WindowTabStripPage';

/**
 * The app always mounts a NotificationPanel (role="dialog", see
 * NotificationPanel.tsx) and can show a background NotificationToast
 * (role="alert", data-testid="toast") from unrelated seeded test-server
 * activity; Next.js itself also always mounts a route-change announcer
 * (id="__next-route-announcer__", role="alert"). None of these are specific
 * to window actions, so "no dialog/alert appeared as a result of this
 * action" must exclude all three rather than assert a literal zero count
 * page-wide.
 */
function nonWindowDialogs(page: Page) {
  return page.locator('[role="dialog"]:not([aria-label="Notification Panel"])');
}
function nonToastAlerts(page: Page) {
  return page.locator('[role="alert"]:not([data-testid="toast"]):not(#__next-route-announcer__)');
}

test.describe('multi-window UX acceptance', () => {
  test.beforeEach(async ({ context }) => {
    // Suppress the generic onboarding modal (useOnboarding.ts shows it 800ms
    // after a fresh load with no `stapler-squad:onboarded` key) — same
    // convention as multi-window-cross-tab.spec.ts. This is unrelated to the
    // window-specific onboarding hint under test in the last test below.
    await context.addInitScript(() => {
      localStorage.setItem('stapler-squad:onboarded', 'true');
    });
  });

  test('create window lands in new empty window with one click', async ({ page }) => {
    await page.goto('/');
    const strip = new WindowTabStripPage(page);
    await strip.waitForLoaded();
    await expect(strip.getTab('Window 1')).toBeVisible();

    await strip.createWindow();

    // New tab is active immediately (page.tsx wires "+" to switchToWindow(createWindow())).
    await expect(strip.tabs).toHaveCount(2);
    const activeTab = strip.getActiveTab();
    await expect(activeTab).toHaveAttribute('title', 'Window 2');
    await expect(activeTab).toHaveAttribute('aria-selected', 'true');

    // The new window's pane content is a fresh, single session-list leaf —
    // not a copy of Window 1's layout.
    await expect(page.getByTestId('session-list-scroll')).toBeVisible();

    // Zero dialogs render as part of window creation.
    await expect(nonWindowDialogs(page)).toHaveCount(0);
  });

  test('Alt+W then digit switches window with no mouse interaction', async ({ page }) => {
    await page.goto('/');
    const strip = new WindowTabStripPage(page);
    await strip.waitForLoaded();

    // Create windows up to 9 (Window 1 already exists from the fresh load).
    for (let i = 2; i <= 9; i++) {
      await strip.createWindow();
    }
    await expect(strip.tabs).toHaveCount(9);

    // Arm the leader (Alt+w) then send the digit "3" — both via the keyboard only.
    await page.keyboard.press('Alt+w');
    await page.keyboard.press('3');

    const thirdTab = strip.getTabByIndex(2);
    await expect(thirdTab).toHaveAttribute('aria-selected', 'true');
    await expect(thirdTab).toHaveAttribute('title', 'Window 3');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 3');

    // Its pane content is visible — the switch actually rendered the target window.
    await expect(page.getByTestId('session-list-scroll')).toBeVisible();
  });

  test('rename window commits on Enter with no confirmation step', async ({ page }) => {
    await page.goto('/');
    const strip = new WindowTabStripPage(page);
    await strip.waitForLoaded();
    await expect(strip.getTab('Window 1')).toBeVisible();

    await strip.renameTab('Window 1', 'Research');

    // Accessible name/text updates immediately — no confirm dialog anywhere.
    await expect(strip.getTab('Research')).toBeVisible();
    await expect(strip.getTab('Window 1')).toHaveCount(0);
    await expect(nonWindowDialogs(page)).toHaveCount(0);
  });

  test('close last window auto-recreates blank window with no confirmation dialog', async ({ page }) => {
    await page.goto('/');
    const strip = new WindowTabStripPage(page);
    await strip.waitForLoaded();
    await expect(strip.tabs).toHaveCount(1);
    const originalId = WindowTabStripPage.windowIdFromUrl(page.url());

    // With only one window, WindowTabStrip never renders a "×" (see
    // WindowTabStripPage's header comment) — close it via the tab's own
    // Delete-key handler (WindowTabButton's onKeyDown) instead of a button
    // that doesn't exist yet for a lone window.
    await strip.getTab('Window 1').focus();
    await page.keyboard.press('Delete');

    await expect(nonWindowDialogs(page)).toHaveCount(0);
    await expect(strip.tabs).toHaveCount(1);
    // useWindowUrlSync's router.replace to the replacement window's id lands
    // in a later effect tick than the click/keydown itself — poll rather
    // than read page.url() synchronously.
    await expect
      .poll(() => WindowTabStripPage.windowIdFromUrl(page.url()))
      .not.toBe(originalId);
    await expect(page.getByTestId('session-list-scroll')).toBeVisible();
  });

  test('renaming to empty reverts silently and renaming to a duplicate name is allowed silently', async ({
    page,
  }) => {
    await page.goto('/');
    const strip = new WindowTabStripPage(page);
    await strip.waitForLoaded();
    await expect(strip.getTab('Window 1')).toBeVisible();
    await strip.createWindow();
    await expect(strip.tabs).toHaveCount(2);
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 2');

    // Enter rename mode on "Window 2", clear the field, and blur — no Enter.
    await strip.getTab('Window 2').dblclick();
    const input = strip.tabList.getByRole('textbox');
    await expect(input).toBeVisible();
    await input.fill('');
    await input.blur();

    // Original name persists; no error/alert renders.
    await expect(strip.getTab('Window 2')).toBeVisible();
    await expect(nonToastAlerts(page)).toHaveCount(0);
    await expect(nonWindowDialogs(page)).toHaveCount(0);

    // Renaming "Window 2" to the exact name of "Window 1" is allowed silently.
    await strip.renameTab('Window 2', 'Window 1');
    await expect(strip.tabs).toHaveCount(2);
    await expect(strip.tabs.filter({ hasText: 'Window 1' })).toHaveCount(2);
    await expect(nonToastAlerts(page)).toHaveCount(0);
    await expect(nonWindowDialogs(page)).toHaveCount(0);
  });

  test('window tab strip is keyboard navigable via roving tabindex and reaches the new-window button next', async ({
    page,
  }) => {
    await page.goto('/');
    const strip = new WindowTabStripPage(page);
    await strip.waitForLoaded();
    await strip.createWindow();
    await strip.createWindow();
    await expect(strip.tabs).toHaveCount(3);

    // Focus the sole tabindex=0 tab directly — a bare page-level Tab press
    // would first land on layout.tsx's "Skip to main content" link (and any
    // other page chrome ahead of the strip in DOM order), which isn't what
    // this test is about.
    await strip.getTab('Window 3').focus();
    await expect(strip.getTab('Window 3')).toHaveAttribute('tabindex', '0');
    const zeroTabIndexTabs = strip.tabList.locator('[role="tab"][tabindex="0"]');
    await expect(zeroTabIndexTabs).toHaveCount(1);

    // ArrowRight moves focus to the next tab and activates it immediately (no extra Enter/click).
    await page.keyboard.press('ArrowRight');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 1');
    await expect(strip.tabList.locator('[role="tab"][tabindex="0"]')).toHaveCount(1);

    // Home jumps to and activates the first tab.
    await page.keyboard.press('Home');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 1');

    // End jumps to and activates the last tab.
    await page.keyboard.press('End');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 3');

    // Tabbing again leaves the strip and lands on the "+" (New window) button.
    await page.keyboard.press('Tab');
    await expect(page.getByRole('button', { name: 'New window' })).toBeFocused();
  });

  test('onboarding hint appears once when second window is created and never reappears after dismissal', async ({
    page,
  }) => {
    await page.goto('/');
    const strip = new WindowTabStripPage(page);
    await strip.waitForLoaded();
    await expect(strip.tabs).toHaveCount(1);

    await strip.createWindow();
    await expect(strip.tabs).toHaveCount(2);

    await expect(page.getByTestId('window-onboarding-hint')).toBeVisible();
    await page.getByRole('button', { name: 'Got it' }).click();
    await expect(page.getByTestId('window-onboarding-hint')).toHaveCount(0);
    // The 2-window layout must actually land in localStorage before reload,
    // or the reload can observe a stale 1-window snapshot (same debounced-save
    // race waitForPersistedWindowCount exists for elsewhere in this page object).
    await strip.waitForPersistedWindowCount(2);

    await page.reload();
    await strip.waitForLoaded();
    await strip.createWindow();
    await expect(strip.tabs).toHaveCount(3);
    await expect(page.getByTestId('window-onboarding-hint')).toHaveCount(0);
  });
});
