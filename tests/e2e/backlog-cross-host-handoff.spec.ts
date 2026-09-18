// @feature backlog:deep-link-resolve

import { test, expect } from '@playwright/test';
import { mockDeepLinkResolve, resolvePageUrl } from './pages/DeepLinkResolveMock';

/**
 * Story 5.1, Surface 3 (project_plans/backlog-deep-linking): cross-host
 * handoff UX when a resolved link's item lives on a different, live host.
 *
 * GAP: web-app/src/app/resolve/page.tsx's "handoff" branch
 * (ResolveDeepLinkInner's `resolve` callback, lines 81-84) does an instant,
 * unconditional `window.location.href = data.advertisedAddress` redirect —
 * there is no banner, no manual "Open on X now" link, no `role="status"`
 * element, and no keyboard-reachable affordance rendered at any point.
 * DeepLinkErrorBanner.tsx (the component that implements every other
 * Surface's UI) is never invoked on this path at all. Every test below is
 * written against validation.md's stated acceptance criteria and is
 * expected to fail, documenting this gap rather than papering over it with
 * a trivially-passing assertion.
 */
const OTHER_HOST = 'otherhost';
const HANDOFF_URL = `ssq://${OTHER_HOST}/backlog/v1/bl_01J0000000000000000000`;
const ADVERTISED_ADDRESS = 'https://otherhost.local:8543/backlog?item=bl_01J0000000000000000000';

test.describe('Backlog cross-host deep-link handoff', () => {
  test('crossHostLink_should_CompleteHandoffInOneClick_When_AutoRedirectBlocked', async ({ page }) => {
    test.fail(true, 'No handoff banner or manual "Open on X now" link is ever rendered — resolve/page.tsx redirects instantly via window.location.href with no intermediate UI (Gap: Surface 3 AC1)');

    await mockDeepLinkResolve(page, [{ kind: 'handoff', advertisedAddress: ADVERTISED_ADDRESS }]);
    // Block the programmatic redirect so a real "auto-redirect blocked"
    // scenario is simulated (e.g. a popup/nav blocker).
    await page.addInitScript(() => {
      Object.defineProperty(window, 'location', {
        value: { ...window.location, href: '' },
        writable: true,
      });
    });

    await page.goto(resolvePageUrl(HANDOFF_URL), { waitUntil: 'domcontentloaded' });

    const manualLink = page.getByRole('link', { name: new RegExp(`open this item on ${OTHER_HOST}`, 'i') });
    await expect(manualLink).toBeVisible();
    await manualLink.click();
  });

  test('crossHostBanner_should_NameTargetHostWithStatusRole_When_HandoffOffered', async ({ page }) => {
    test.fail(true, 'No cross-host handoff banner exists at all (Gap: Surface 3 AC2/AC3) — resolve/page.tsx redirects instantly instead of rendering any UI naming the host');

    await mockDeepLinkResolve(page, [{ kind: 'handoff', advertisedAddress: ADVERTISED_ADDRESS }]);
    await page.goto(resolvePageUrl(HANDOFF_URL), { waitUntil: 'domcontentloaded' });

    const banner = page.getByRole('status');
    await expect(banner).toContainText(OTHER_HOST);
  });

  test('crossHostBanner_should_ExposeAccessibleNameNamingHost_When_ManualLinkRendered', async ({ page }) => {
    test.fail(true, 'No manual handoff link is ever rendered (Gap: Surface 3 AC4)');

    await mockDeepLinkResolve(page, [{ kind: 'handoff', advertisedAddress: ADVERTISED_ADDRESS }]);
    await page.goto(resolvePageUrl(HANDOFF_URL), { waitUntil: 'domcontentloaded' });

    await expect(page.getByRole('link', { name: new RegExp(`open this item on ${OTHER_HOST}`, 'i') })).toBeVisible();
  });

  test('crossHostHandoff_should_BeCompletableViaKeyboardOnly_When_TabbedToManualLink', async ({ page }) => {
    test.fail(true, 'No keyboard-reachable manual handoff link exists to tab to (Gap: Surface 3 AC5)');

    await mockDeepLinkResolve(page, [{ kind: 'handoff', advertisedAddress: ADVERTISED_ADDRESS }]);
    await page.addInitScript(() => {
      Object.defineProperty(window, 'location', {
        value: { ...window.location, href: '' },
        writable: true,
      });
    });
    await page.goto(resolvePageUrl(HANDOFF_URL), { waitUntil: 'domcontentloaded' });

    const manualLink = page.getByRole('link', { name: new RegExp(`open this item on ${OTHER_HOST}`, 'i') });
    await page.keyboard.press('Tab');
    await expect(manualLink).toBeFocused();
    await page.keyboard.press('Enter');
  });
});
