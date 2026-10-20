// @feature ui:header-nav
/**
 * Story 7.7a (Next baseline): today a path that matches no route answers 200 and the app lands on
 * the sessions page at `/`; there is no not-found view. fixtures/next-baseline.json records the
 * observed status, final path and title. The Bun half (7.7b, PR 7b) asserts the decided
 * "Page not found" view through the web-build helper.
 */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { measureNotFound } from './pages/BaselineProbes';
import baseline from './fixtures/next-baseline.json';

test.describe('not found (Next baseline)', () => {
  test('an unknown path answers 200 and ends on the recorded page', async ({ page }) => {
    await new AppShellPage(page).skipOnboarding();
    const observed = await measureNotFound(page);
    expect(observed).toEqual(baseline.notFound);
  });

  test('the nav still works from the page an unknown path lands on', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.skipOnboarding();
    await page.setViewportSize({ width: 1400, height: 900 });
    await measureNotFound(page);
    await expect(shell.drawerNav).toBeVisible();
    await shell.navLink(/history/i).click({ force: true });
    await expect(page).toHaveTitle('History - Stapler Squad');
  });
});
