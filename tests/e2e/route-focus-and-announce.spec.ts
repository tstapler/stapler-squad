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
import { isBunBuild } from './helpers/web-build';
import { AppShellPage } from './pages/AppShellPage';
import baseline from './fixtures/next-baseline.json';

const HISTORY_TITLE = 'History - Stapler Squad';
const SETTINGS_TITLE = 'Settings - Stapler Squad';
const LOGS_TITLE = 'Logs - Stapler Squad';

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

/** Records every announcer-region text change, keyed by its own clear-then-set sequencing. */
async function watchAnnouncer(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    (window as unknown as { __ann: string[] }).__ann = [];
    const el = document.querySelector('[data-testid=route-announcer]')!;
    new MutationObserver(() => (window as unknown as { __ann: string[] }).__ann.push(el.textContent ?? '')).observe(el, {
      childList: true,
      characterData: true,
      subtree: true,
    });
  });
}

async function announcerLog(page: import('@playwright/test').Page): Promise<string[]> {
  return page.evaluate(() => (window as unknown as { __ann: string[] }).__ann);
}

test.describe('route focus and announcements (Bun only)', () => {
  test.skip(!isBunBuild(), "Next's own announcer (role=alert) is asserted by the Next-baseline block above");

  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1400, height: 900 });
    await new AppShellPage(page).skipOnboarding();
  });

  test('the announcer is role=status, not role=alert, and focus lands on the layout main', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/review-queue');
    await expect(shell.drawerNav).toBeVisible();

    await shell.navLink(/history/i).click({ force: true });
    await expect(page).toHaveTitle(HISTORY_TITLE);

    const announcer = shell.liveRegions.filter({ hasText: HISTORY_TITLE }).first();
    await expect(announcer).toHaveAttribute('role', 'status');
    await expect
      .poll(() => page.evaluate(() => document.activeElement === document.getElementById('main-content')))
      .toBe(true);
  });

  test('two navigations with the same title are both announced (clear then set each time)', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/settings');
    await expect(shell.drawerNav).toBeVisible();
    await watchAnnouncer(page);

    await shell.clientNavigate('/settings/pipeline-modes');
    await expect.poll(() => announcerLog(page)).toEqual([SETTINGS_TITLE, '']);
    await shell.clientNavigate('/settings/remotes');
    await expect.poll(() => announcerLog(page)).toEqual([SETTINGS_TITLE, '', SETTINGS_TITLE, '']);
  });

  test('a page that autofocuses its own input (the omnibar) keeps that focus', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/logs');
    await expect(shell.drawerNav).toBeVisible();

    // ?new=true (useOmnibarQueryParamLaunch) opens the omnibar and focuses its input outside <main>;
    // useFocusOnNavigate must not steal it back.
    await shell.clientNavigate('/?new=true');
    const input = page.getByRole('combobox', { name: /session source input/i });
    await expect(input).toBeFocused();
  });

  test('browser Back announces the title and refocuses the layout main', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/logs');
    await expect(shell.drawerNav).toBeVisible();
    await shell.navLink(/history/i).click({ force: true });
    await expect(page).toHaveTitle(HISTORY_TITLE);

    await watchAnnouncer(page);
    await page.goBack();
    await expect(page).toHaveTitle(LOGS_TITLE);
    await expect.poll(() => announcerLog(page)).toEqual([LOGS_TITLE, '']);
    await expect
      .poll(() => page.evaluate(() => document.activeElement === document.getElementById('main-content')))
      .toBe(true);
  });

  test('a loader redirect (/config) announces exactly once, not once per hop', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/history');
    await expect(shell.drawerNav).toBeVisible();
    await watchAnnouncer(page);

    await shell.clientNavigate('/config');
    await expect(page).toHaveURL(/\/settings\?tab=config-files/);
    await expect.poll(() => announcerLog(page)).toEqual([SETTINGS_TITLE, '']);
  });

  test('/history: duplicate main#main-content nodes are tolerated, but the first is focused', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.goto('/logs');
    await expect(shell.drawerNav).toBeVisible();
    await shell.navLink(/history/i).click({ force: true });
    await expect(page).toHaveTitle(HISTORY_TITLE);

    const mains = page.locator('main#main-content');
    expect(await mains.count()).toBeGreaterThanOrEqual(1);
    await expect
      .poll(() => page.evaluate(() => document.activeElement === document.querySelectorAll('main#main-content')[0]))
      .toBe(true);
  });
});
