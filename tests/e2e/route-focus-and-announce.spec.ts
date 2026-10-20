// @feature ui:header-nav, accessibility
/**
 * Story 7.2a (Next baseline): what a keyboard or screen-reader user gets on navigation.
 * Premises of the plan that do not hold on Next, recorded in fixtures/next-baseline.json:
 *  - The skip link is not the first Tab stop (the nav precedes it), so the spec Tabs to it.
 *  - Activating it sets the `#main-content` hash but does not focus <main> (it has no tabindex),
 *    and the computed outline-style of main#main-content is `none`.
 * The Bun half (7.2b) asserts the decided policy through the web-build helper in PR 7b.
 */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import baseline from './fixtures/next-baseline.json';

const HISTORY_TITLE = 'History - Stapler Squad';

test.describe('route focus and announcements (Next baseline)', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1400, height: 900 });
    await new AppShellPage(page).skipOnboarding();
  });

  test('clicking the History nav link announces the new title in a live region', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/review-queue');
    await expect(shell.drawerNav).toBeVisible();

    await shell.navLink(/history/i).click({ force: true });
    await expect(page).toHaveTitle(HISTORY_TITLE);

    // Next's own announcer is the only live region that carries the title.
    await expect(shell.liveRegions.filter({ hasText: HISTORY_TITLE })).toHaveCount(1);
    const announcer = shell.liveRegions.filter({ hasText: HISTORY_TITLE }).first();
    await expect(announcer).toHaveAttribute('role', 'alert');
  });

  test('skip link: Tab reaches it, Enter keeps the path and sets the hash', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/history');
    await expect(shell.drawerNav).toBeVisible();

    const tabPresses = await shell.tabToSkipLink();
    expect(tabPresses, 'skip link tab position on Next').toBe(baseline.skipLink.tabPresses);

    const outcome = await shell.activateSkipLink();
    expect(outcome.path).toBe('/history');
    expect(outcome.hash).toBe(baseline.skipLink.hash);
    expect(outcome.mainFocused, 'Next does not move focus into <main>').toBe(baseline.skipLink.mainFocused);
    expect(outcome.outlineStyle).toBe(baseline.skipLink.outlineStyle);
  });

  test('the History nav item exposes aria-current="page" on /history', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/history');
    await expect(shell.drawerNav).toBeVisible();
    await expect(shell.navLink(/history/i)).toHaveAttribute('aria-current', baseline.historyNavAriaCurrent as string);
  });
});
