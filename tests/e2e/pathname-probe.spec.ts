// @feature ui:header-nav
// Characterises, under Next, what `usePathname()` returns across a client navigation
// (/review-queue -> /history -> back). There is no test hook for the hook's value, so it
// is observed through its one exact-comparison consumer: CockpitShell hides the floating
// "Keyboard shortcuts (?)" button only when `usePathname() === "/review-queue"`. Button
// hidden => no trailing slash; button visible on /review-queue => trailing slash.
// The result is printed (and attached) rather than asserted, so this spec records the
// behaviour Story 2.1's NormalisedPathname must encode without pinning Next internals.
//
// Recorded under Next (next.config.ts trailingSlash:true), 2026-10-19, chromium:
//   direct load /review-queue          location.pathname=/review-queue    usePathname()=/review-queue  (no slash)
//   Link click to /history             location.pathname=/history/
//   Link click back to /review-queue   location.pathname=/review-queue/  usePathname()=/review-queue/ (slash)
//   history.back() to /history         location.pathname=/history/
// So a Link navigation yields a trailing slash and a direct load does not; exact comparisons
// such as CockpitShell's `pathname !== "/review-queue"` misfire after client navigation.
import { test, expect } from '@playwright/test';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const SHORTCUTS_BUTTON = 'Keyboard shortcuts (?)';

test.describe('pathname probe', () => {
  test('records location.pathname and the usePathname() slash behaviour across nav', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 1400, height: 900 }); // desktop: the floating button only renders when not narrow
    const records: string[] = [];
    const snapshot = async (step: string) => {
      const pathname = await page.evaluate(() => window.location.pathname);
      const buttonVisible = await page.getByRole('button', { name: SHORTCUTS_BUTTON }).isVisible();
      const usePathnameHasSlash = pathname.startsWith('/review-queue') ? buttonVisible : 'n/a';
      records.push(`${step}: location.pathname=${pathname} floatingButtonVisible=${buttonVisible} usePathnameIsExactlyReviewQueue=${pathname.startsWith('/review-queue') ? !buttonVisible : 'n/a'} (slash-observed=${usePathnameHasSlash})`);
    };

    await page.goto(`${BASE_URL}/review-queue`, { waitUntil: 'domcontentloaded', timeout: 15000 });
    await expect(page.getByTestId('drawer-nav')).toBeVisible({ timeout: 15000 });
    await snapshot('direct load /review-queue');

    await page.getByTestId('drawer-nav').getByRole('link', { name: /history/i }).first().click();
    await expect(page).toHaveURL(/\/history/);
    await snapshot('after click to /history');

    // includeHidden: the drawer is aria-hidden on /history under Next (recorded finding, not asserted).
    // A Link-driven navigation (not history traversal) is where trailingSlash:true shows up.
    await page.getByTestId('drawer-nav').getByRole('link', { name: /review queue/i, includeHidden: true }).first().click();
    await expect(page).toHaveURL(/\/review-queue/);
    await snapshot('after link click to /review-queue');

    await page.goBack();
    await expect(page).toHaveURL(/\/history/);
    await snapshot('after back to /history');

    const report = records.join('\n');
    console.log(`PATHNAME PROBE\n${report}`);
    await testInfo.attach('pathname-probe', { body: report, contentType: 'text/plain' });
    expect(records).toHaveLength(4);
  });
});
