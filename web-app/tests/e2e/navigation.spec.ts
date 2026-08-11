// @feature ui:header-nav, ui:bottom-nav
import { test, expect } from '@playwright/test';

/**
 * Navigation E2E tests — driven by the shared NAV_PAGES constant.
 *
 * The "pop" pattern: every page in NAV_PAGES is added to a Set, then
 * removed as it is verified. An assertion at the end of each suite
 * ensures nothing was skipped. Adding a new NavPage without updating
 * the test is intentionally impossible — the Set will be non-empty.
 *
 * Desktop viewport (1280×800): Header nav visible, BottomNav hidden.
 * Mobile viewport (390×844): BottomNav visible, Header hidden.
 *
 * Requires a running dev server (`make restart-web`).
 */

// Inline the shared nav page definitions so the test file has no
// server-side import dependency. Keep in sync with web-app/src/lib/nav-pages.ts.
const NAV_PAGES = [
  { href: '/',             label: 'Sessions',     shortLabel: undefined,  mobileNav: true },
  { href: '/unfinished',   label: 'Up Next',      shortLabel: undefined,  mobileNav: true },
  { href: '/review-queue', label: 'Review Queue', shortLabel: 'Review',   mobileNav: true },
  { href: '/rules',        label: 'Rules',        shortLabel: undefined,  mobileNav: true },
  { href: '/logs',         label: 'Logs',         shortLabel: undefined,  mobileNav: false },
  { href: '/history',      label: 'History',      shortLabel: undefined,  mobileNav: true },
  { href: '/config',       label: 'Config',       shortLabel: undefined,  mobileNav: true },
  { href: '/settings',     label: 'Settings',     shortLabel: undefined,  mobileNav: false },
] as const;

const MOBILE_PAGES = NAV_PAGES.filter((p) => p.mobileNav);
const ALL_PAGES    = NAV_PAGES;

test.describe('Header navigation (desktop)', () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test('every NAV_PAGE is reachable via the header', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');

    const remaining = new Set(ALL_PAGES.map((p) => p.href));

    for (const navPage of ALL_PAGES) {
      const link = page.locator(`header nav a[href="${navPage.href}/"], header nav a[href="${navPage.href}"]`).first();
      await expect(link).toBeVisible({ timeout: 3000 });
      await link.click();
      await expect(page).toHaveURL(new RegExp(navPage.href === '/' ? '^http[^?]+/$' : navPage.href));
      remaining.delete(navPage.href);
    }

    // All pages must have been popped — fail loudly if any were skipped
    expect([...remaining], 'Some NAV_PAGES were not covered by header nav').toHaveLength(0);
  });

  test('header is visible on desktop', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await expect(page.locator('header')).toBeVisible();
  });

  test('active link has aria-current="page" on home', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await expect(page.locator('header nav a[href="/"]').first()).toHaveAttribute('aria-current', 'page');
  });

  test('New Session button opens omnibar', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Create new session (⌘K)').click();
    await expect(page.getByLabel('Session source input')).toBeVisible({ timeout: 5000 });
  });
});

test.describe('BottomNav (mobile)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('every mobile NAV_PAGE is reachable via BottomNav', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');

    const remaining = new Set(MOBILE_PAGES.map((p) => p.href));
    const nav = page.locator('nav[aria-label="Bottom navigation"]');

    for (const navPage of MOBILE_PAGES) {
      const link = nav.locator(`a[href="${navPage.href}/"], a[href="${navPage.href}"]`).first();
      await expect(link).toBeVisible({ timeout: 3000 });
      await link.click();
      await expect(page).toHaveURL(new RegExp(navPage.href === '/' ? '^http[^?]+/$' : navPage.href));
      remaining.delete(navPage.href);
    }

    // All mobile pages must have been popped
    expect([...remaining], 'Some MOBILE_NAV_PAGES were not covered by BottomNav').toHaveLength(0);
  });

  test('desktop-only pages are absent from BottomNav', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    const nav = page.locator('nav[aria-label="Bottom navigation"]');
    const desktopOnly = NAV_PAGES.filter((p) => !p.mobileNav);

    for (const navPage of desktopOnly) {
      const link = nav.locator(`a[href="${navPage.href}/"], a[href="${navPage.href}"]`);
      await expect(link).toHaveCount(0);
    }
  });

  test('header is hidden on mobile', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await expect(page.locator('header')).toBeHidden();
  });

  test('New session button opens omnibar', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: 'New session' }).click();
    await expect(page.getByLabel('Session source input')).toBeVisible({ timeout: 5000 });
  });

  test('active nav item has aria-current="page" on home', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    const nav = page.locator('nav[aria-label="Bottom navigation"]');
    await expect(nav.locator('a[href="/"]').first()).toHaveAttribute('aria-current', 'page');
  });
});
