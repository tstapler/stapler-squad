// @feature window:create, window:switch, window:close, window:rename
/**
 * UX-acceptance sweep for the multi-window feature — see
 * project_plans/multi-window/implementation/validation.md's "Requirement -> Test
 * Mapping" table (rows for Surfaces 1-10) for the exact behaviors each test below
 * proves. Cross-tab/URL-sync coverage lives in multi-window-cross-tab.spec.ts (REQ-10);
 * these tests are single-tab, single-window-strip interaction checks.
 */
import { test, expect } from '@playwright/test';
import { WindowTabStripPage } from './pages/WindowTabStripPage';

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
    await expect(page.getByRole('dialog')).toHaveCount(0);
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
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('close last window auto-recreates blank window with no confirmation dialog', async ({ page }) => {
    await page.goto('/');
    const strip = new WindowTabStripPage(page);
    await strip.waitForLoaded();
    await expect(strip.tabs).toHaveCount(1);
    const originalId = WindowTabStripPage.windowIdFromUrl(page.url());

    // With only one window, WindowTabStrip only renders a "×" once 2+ windows
    // exist (see WindowTabStripPage's header comment) — page.tsx's own
    // single-window close affordance is exercised via the close button once
    // it's the *result* of the auto-recreation invariant below, so drive this
    // directly through the page object's closeTab helper name used by the
    // component. If no close control renders for a lone window, this reflects
    // the same "close" action as clicking a window's own tab's × once the app
    // exposes it — assert on the invariant this test is actually about: at no
    // point does closing the sole window produce a confirmation dialog, and
    // the strip always shows exactly one tab, but that tab is a *different*,
    // blank window afterward.
    await strip.closeTab('Window 1');

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(strip.tabs).toHaveCount(1);
    const newId = WindowTabStripPage.windowIdFromUrl(page.url());
    expect(newId).not.toBe(originalId);
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
    await expect(page.getByRole('alert')).toHaveCount(0);
    await expect(page.getByRole('dialog')).toHaveCount(0);

    // Renaming "Window 2" to the exact name of "Window 1" is allowed silently.
    await strip.renameTab('Window 2', 'Window 1');
    await expect(strip.tabs).toHaveCount(2);
    await expect(strip.tabs.filter({ hasText: 'Window 1' })).toHaveCount(2);
    await expect(page.getByRole('alert')).toHaveCount(0);
    await expect(page.getByRole('dialog')).toHaveCount(0);
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

    // Tab into the strip — focus should land on the sole tabindex=0 tab.
    await page.keyboard.press('Tab');
    await expect(strip.tabs.filter({ hasText: /^Window 3$/ })).toHaveAttribute('tabindex', '0');
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

    await page.reload();
    await strip.waitForLoaded();
    await strip.createWindow();
    await expect(strip.tabs).toHaveCount(3);
    await expect(page.getByTestId('window-onboarding-hint')).toHaveCount(0);
  });
});
