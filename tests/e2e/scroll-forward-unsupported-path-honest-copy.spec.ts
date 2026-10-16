// @feature app-scrollback-forwarding
/**
 * UX-AC-8: the one real, acknowledged limitation (UNSUPPORTED_STREAMING_PATH
 * — a solo user on PathLegacyPerConnection, unconditionally Blocked) tells
 * the truth: no false "retry" affordance, no false "another viewer" claim.
 *
 * A session is pinned onto PathLegacyPerConnection via
 * forceLegacyStreamingPath — config.EffectiveStreamHubEnabled's global
 * default is hub-owned, so this can't be assumed unconditionally.
 */
import { test, expect } from '@playwright/test';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { SessionClient } from './helpers/session-client';
import {
  BASE_URL,
  createAltScreenScrollSession,
  forceLegacyStreamingPath,
  openAndWaitConnected,
  scrollUpUntilVisible,
  soloPathBeforeEach,
  soloPathAfterEach,
} from './helpers/scroll-forward-fixture';

test.describe('scroll-forward unsupported-path honest copy (UX-AC-8)', () => {
  test.beforeEach(soloPathBeforeEach);
  test.afterEach(soloPathAfterEach);

  test('UNSUPPORTED_STREAMING_PATH toast should contain no retry affordance or false viewer claim', async ({
    page,
    request,
  }) => {
    const client = new SessionClient(BASE_URL);
    const title = `e2e-scroll-honest-copy-${Date.now()}`;
    const { session } = await createAltScreenScrollSession(client, title);
    await forceLegacyStreamingPath(request, title);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    const toast = detail.getScrollBlockedToast();
    await scrollUpUntilVisible(page, detail, () => expect(toast).toBeVisible({ timeout: 2500 }));
    await expect(toast).toHaveAttribute('data-blocked-reason', 'UNSUPPORTED_STREAMING_PATH');

    const text = (await toast.textContent()) ?? '';
    expect(text).not.toMatch(/retry|try again/i);
    expect(text).not.toMatch(/viewer/i);

    // The one action offered is dismissal ("Got it"), never a false retry
    // button — the "Got it" dismiss button is the toast's only <button>.
    await expect(toast.getByRole('button')).toHaveCount(1);
    await expect(detail.getScrollBlockedToastDismiss()).toHaveText('Got it');
  });
});
