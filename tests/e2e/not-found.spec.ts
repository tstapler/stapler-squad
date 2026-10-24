// @feature ui:header-nav
/**
 * Story 7.7a (Next baseline): today a path that matches no route answers 200 and the app lands on
 * the sessions page at `/`; there is no not-found view. fixtures/next-baseline.json records the
 * observed status, final path and title. The Bun half (7.7b, PR 7b) asserts the decided
 * "Page not found" view through the web-build helper.
 */
import { test, expect } from '@playwright/test';
import { isBunBuild } from './helpers/web-build';
import { AppShellPage } from './pages/AppShellPage';
import { measureNotFound } from './pages/BaselineProbes';
import baseline from './fixtures/next-baseline.json';
import { NOT_FOUND_META } from '../../web-app/src/app/route-meta';

const MISSING_PATH = '/nonexistent';

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

test.describe('not found (Bun only, UX G3)', () => {
  test.skip(!isBunBuild(), 'The dedicated not-found view (app/NotFound.tsx) only renders on the Bun router build');

  test('shows the not-found view: heading, path, home link, status 200, no redirect, no active nav', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.skipOnboarding();
    await page.setViewportSize({ width: 1400, height: 900 });
    const response = await page.goto(`${(process.env.TEST_SERVER_URL || 'http://localhost:8544')}${MISSING_PATH}`, {
      waitUntil: 'domcontentloaded',
    });

    expect(response?.status()).toBe(200);
    await expect(page).toHaveURL(new RegExp(`${MISSING_PATH}$`));
    await expect(page).toHaveTitle(NOT_FOUND_META.title);
    await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible();
    await expect(page.getByText(MISSING_PATH, { exact: false })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Go to Sessions' })).toBeVisible();

    const navLinks = shell.drawerNav.getByRole('link', { includeHidden: true });
    expect(await navLinks.evaluateAll((all) => all.some((a) => a.getAttribute('aria-current') || a.hasAttribute('data-active')))).toBe(false);
  });

  test('reached by a client-side navigation, the announcer carries the not-found title', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.skipOnboarding();
    // The RouteAnnouncer only reacts to a pathname *change*, so a hard load (no previous route)
    // never announces; drive the transition from another route instead, as a real nav-miss would.
    await shell.goto('/history');
    await expect(shell.drawerNav).toBeVisible();
    await shell.clientNavigate(MISSING_PATH);
    await expect(page).toHaveTitle(NOT_FOUND_META.title);
    await expect(page.getByTestId('route-announcer')).toHaveText(NOT_FOUND_META.title);
  });

  test('the nav still works and "Go back" shows only when there is history to go back to', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.skipOnboarding();
    await page.setViewportSize({ width: 1400, height: 900 });

    // A fresh context's history.length is still >1 (the initial about:blank entry counts), so stub
    // it to assert the single-entry case the AC describes.
    await page.addInitScript(() => Object.defineProperty(window.history, 'length', { get: () => 1 }));
    await page.goto(`${(process.env.TEST_SERVER_URL || 'http://localhost:8544')}${MISSING_PATH}`, { waitUntil: 'domcontentloaded' });
    await expect(page.getByRole('button', { name: /go back/i })).toHaveCount(0);

    await expect(shell.drawerNav).toBeVisible();
    await shell.navLink(/history/i).click({ force: true });
    await expect(page).toHaveTitle('History - Stapler Squad');
  });
});
