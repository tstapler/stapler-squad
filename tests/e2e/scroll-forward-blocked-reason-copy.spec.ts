// @feature app-scrollback-forwarding
/**
 * UX-AC-4: the Blocked toast shows the message matching its actual
 * ScrollBlockedReason, never a one-size-fits-all string.
 *
 * Case 1 (MULTIPLE_VIEWERS, hub-path-gated): a real second subscriber is
 * attached — toast copy interpolates the session's program and the
 * ConnectionCountIndicator pulses.
 * Case 2 (UNSUPPORTED_STREAMING_PATH): a solo tab pinned onto
 * PathLegacyPerConnection via forceLegacyStreamingPath (config.
 * EffectiveStreamHubEnabled's global default is hub-owned, not legacy, so
 * this can no longer be assumed unconditionally) — every scroll is BLOCKED
 * regardless of viewer count (session/scroll_gate.go passes a -1
 * subscriberCount sentinel on that path) — fixed copy, no program
 * interpolation, no badge pulse.
 */
import { test, expect } from '@playwright/test';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { SessionClient } from './helpers/session-client';
import {
  BASE_URL,
  hubGatedBeforeEach,
  hubGatedAfterEach,
  createAltScreenScrollSession,
  forceLegacyStreamingPath,
  openAndWaitConnected,
  scrollUpUntilVisible,
  attachSecondViewer,
  soloPathBeforeEach,
  soloPathAfterEach,
} from './helpers/scroll-forward-fixture';

test.describe('scroll-forward blocked reason copy — MULTIPLE_VIEWERS (UX-AC-4 case 1)', () => {
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('MULTIPLE_VIEWERS renders the exact reason-accurate toast copy and pulses the connection badge', async ({
    page,
    browser,
  }) => {
    const client = new SessionClient(BASE_URL);
    const { session, programPath } = await createAltScreenScrollSession(
      client,
      `e2e-scroll-blocked-copy-mv-${Date.now()}`,
    );
    const detailA = new SessionDetailPage(page);
    await detailA.gotoSession(session.id);
    await openAndWaitConnected(page, detailA);

    const { contextB } = await attachSecondViewer(browser, detailA, session.id);
    try {
      const toast = detailA.getScrollBlockedToast();
      await scrollUpUntilVisible(page, detailA, () => expect(toast).toBeVisible({ timeout: 2500 }));
      await expect(toast).toHaveAttribute('data-blocked-reason', 'MULTIPLE_VIEWERS');
      // TerminalOutput.tsx's blockedToastCopy — the message span is the
      // toast's first (and only) <span>, distinct from the "Got it" button.
      await expect(toast.locator('span')).toHaveText(
        `⚠ Can't browse ${programPath}'s history right now — another viewer is connected. ` +
          `Close other tabs or sessions viewing this session to enable it.`,
      );
      await expect(detailA.getConnectionCountIndicator()).toHaveAttribute('data-pulse', 'true');
    } finally {
      await contextB.close();
    }
  });
});

test.describe('scroll-forward blocked reason copy — UNSUPPORTED_STREAMING_PATH (UX-AC-4 case 2)', () => {
  test.beforeEach(soloPathBeforeEach);
  test.afterEach(soloPathAfterEach);

  test('UNSUPPORTED_STREAMING_PATH renders fixed copy with no viewer claim and no badge pulse', async ({
    page,
    request,
  }) => {
    const client = new SessionClient(BASE_URL);
    const title = `e2e-scroll-blocked-copy-usp-${Date.now()}`;
    const { session } = await createAltScreenScrollSession(client, title);
    await forceLegacyStreamingPath(request, title);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    // Solo tab: the connection-count indicator never renders at all
    // (TerminalOutput.tsx only shows it once connectionCount > 1).
    await expect(detail.getConnectionCountIndicator()).not.toBeAttached();

    const toast = detail.getScrollBlockedToast();
    await scrollUpUntilVisible(page, detail, () => expect(toast).toBeVisible({ timeout: 2500 }));
    await expect(toast).toHaveAttribute('data-blocked-reason', 'UNSUPPORTED_STREAMING_PATH');
    await expect(toast.locator('span')).toHaveText("⚠ Scroll-forwarding isn't available for this session yet");
    await expect(detail.getConnectionCountIndicator()).not.toBeAttached();
  });
});
