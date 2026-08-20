// @feature backlog:deep-link-resolve

import { test, expect } from '@playwright/test';
import { mockDeepLinkResolve, resolvePageUrl } from './pages/DeepLinkResolveMock';

/**
 * Story 5.1, Surfaces 4-9 (project_plans/backlog-deep-linking): the `/resolve`
 * page's error/edge-case UX, rendered via DeepLinkErrorBanner.tsx. Unlike
 * the cross-host handoff surface (backlog-cross-host-handoff.spec.ts, which
 * is a documented gap), every reason covered here IS implemented — see
 * DeepLinkErrorBanner.tsx's `copyFor()` switch. Responses are mocked via
 * page.route (DeepLinkResolveMock.ts) so each failure kind is deterministic
 * without needing a real second host or registry entry.
 */
const OTHER_HOST = 'otherhost';
const SOME_ITEM_URL = `ssq://myhost/backlog/v1/bl_01J0000000000000000000`;
const REMOTE_ITEM_URL = `ssq://${OTHER_HOST}/backlog/v1/bl_01J0000000000000000000`;

test.describe('Backlog deep-link resolution errors', () => {
  test('deletedItemLink_should_ShowAlertBannerWithBoardEscapeHatch_When_ItemNoLongerExists', async ({ page }) => {
    await mockDeepLinkResolve(page, [{ kind: 'not-found', reason: 'deleted' }]);
    await page.goto(resolvePageUrl(SOME_ITEM_URL), { waitUntil: 'domcontentloaded' });

    const banner = page.getByRole('alert');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('This backlog item no longer exists');

    await page.getByRole('button', { name: 'Go to backlog board' }).click();
    await expect(page).toHaveURL(/\/backlog\/board$/);
  });

  test('unreachableHostLink_should_ShowStatusBannerWithLastSeenAndRetry_When_LivenessCheckTimesOut', async ({ page }) => {
    await mockDeepLinkResolve(page, [
      { kind: 'unreachable', reason: 'unreachable', lastSeenAt: '2h ago' },
      { kind: 'unreachable', reason: 'unreachable', lastSeenAt: '2h ago' },
    ]);
    await page.goto(resolvePageUrl(REMOTE_ITEM_URL), { waitUntil: 'domcontentloaded' });

    const banner = page.getByRole('status');
    await expect(banner).toContainText(OTHER_HOST);
    await expect(banner).toContainText('Last seen');

    // Retry re-runs the check (a second, distinguishable request fires).
    let secondRequestSeen = false;
    page.on('request', (req) => {
      if (req.url().includes('/api/deep-link/resolve')) secondRequestSeen = true;
    });
    await page.getByRole('button', { name: 'Retry' }).click();
    await expect.poll(() => secondRequestSeen).toBe(true);
  });

  test('unregisteredHostLink_should_ShowStatusBannerWithNoAddressAction_When_HostnameUnknown', async ({ page }) => {
    await mockDeepLinkResolve(page, [{ kind: 'unreachable', reason: 'not-registered' }]);
    await page.goto(resolvePageUrl(REMOTE_ITEM_URL), { waitUntil: 'domcontentloaded' });

    const banner = page.getByRole('status');
    await expect(banner).toContainText(OTHER_HOST);
    await expect(page.getByRole('button', { name: 'Copy host address' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible();
  });

  test('malformedLink_should_ShowAlertBannerWithoutRawURLOrStackTrace_When_LinkTruncated', async ({ page }) => {
    const truncatedRaw = 'ssq://myhost/backlog/v1/bl_01J000'; // deliberately cut off
    await mockDeepLinkResolve(page, [{ kind: 'invalid', reason: 'malformed' }]);
    await page.goto(resolvePageUrl(truncatedRaw), { waitUntil: 'domcontentloaded' });

    const banner = page.getByRole('alert');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("This link isn't valid");

    const bannerText = (await banner.textContent()) ?? '';
    expect(bannerText).not.toContain(truncatedRaw);
    expect(bannerText.toLowerCase()).not.toContain('at resolvedeeplinkinner'); // no stack trace leakage
  });

  test('versionMismatchLink_should_ShowAlertBannerNamingVersionGap_When_LinkVersionUnsupported', async ({ page }) => {
    const unsupportedVersionUrl = 'ssq://myhost/backlog/v99/bl_01J0000000000000000000';
    await mockDeepLinkResolve(page, [{ kind: 'invalid', reason: 'version-mismatch' }]);
    await page.goto(resolvePageUrl(unsupportedVersionUrl), { waitUntil: 'domcontentloaded' });

    const banner = page.getByRole('alert');
    await expect(banner).toContainText('needs a newer version of stapler-squad');

    const bannerText = (await banner.textContent()) ?? '';
    expect(bannerText).not.toContain('v99');
  });

  test('retryAction_should_NotAccumulateDuplicateBanners_When_ClickedRepeatedlyOnFailure', async ({ page }) => {
    await mockDeepLinkResolve(page, [
      { kind: 'unreachable', reason: 'unreachable', lastSeenAt: '1h ago' },
      { kind: 'unreachable', reason: 'unreachable', lastSeenAt: '1h ago' },
      { kind: 'unreachable', reason: 'unreachable', lastSeenAt: '1h ago' },
      { kind: 'unreachable', reason: 'unreachable', lastSeenAt: '1h ago' },
    ]);
    await page.goto(resolvePageUrl(REMOTE_ITEM_URL), { waitUntil: 'domcontentloaded' });

    const banner = page.getByTestId('deep-link-error-banner');
    await expect(banner).toHaveCount(1);

    const retryButton = page.getByRole('button', { name: 'Retry' });
    for (let i = 0; i < 3; i += 1) {
      await retryButton.click();
      await expect(banner).toHaveCount(1);
    }
  });
});
