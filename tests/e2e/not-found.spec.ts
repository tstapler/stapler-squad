// @feature ui:header-nav
/** An unknown path renders the dedicated "Page not found" view (app/NotFound.tsx) with status 200. */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { NOT_FOUND_META } from '../../web-app/src/app/route-meta';
import { baseUrl } from './helpers/base-url';

const MISSING_PATH = '/nonexistent';

test.describe('not found (UX G3)', () => {
  test('shows the not-found view: heading, path, home link, status 200, no redirect, no active nav', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.skipOnboarding();
    await page.setViewportSize({ width: 1400, height: 900 });
    const response = await page.goto(`${baseUrl()}${MISSING_PATH}`, {
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
    await page.goto(`${baseUrl()}${MISSING_PATH}`, { waitUntil: 'domcontentloaded' });
    await expect(page.getByRole('button', { name: /go back/i })).toHaveCount(0);

    await expect(shell.drawerNav).toBeVisible();
    await shell.navLink(/history/i).click({ force: true });
    await expect(page).toHaveTitle('History - Stapler Squad');
  });
});
