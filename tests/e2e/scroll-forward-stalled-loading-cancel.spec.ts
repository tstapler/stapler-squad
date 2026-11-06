// @feature app-scrollback-forwarding
/**
 * UX-AC-5: the stalled-loading case offers an explicit Cancel action after
 * 8s — no state leaves the user staring at an indefinite spinner.
 *
 * design/ux.md Surface 1 step 5 is explicit that the 8s-stalled case is a
 * genuine *network* stall (no server response at all), not the app-level
 * `deadlineExceeded` outcome (waitForRedrawQuiescence's own ~2s poll deadline
 * always returns SOME outcome). The fixture script can't simulate "the
 * server never responds" on its own — that requires withholding the
 * AppScrollbackResponse frame at the WebSocket level, the same
 * page.routeWebSocket() proxy-and-withhold technique
 * terminal-resync-banner.spec.ts already uses for its own stall scenario.
 *
 * Unlike the other specs in this suite, this one does NOT need
 * STAPLER_SQUAD_USE_STREAM_HUB: withholding happens at the WebSocket layer
 * before any server response — real or BLOCKED — ever reaches the client, so
 * the pill's 150ms-show/8s-stall timers (pure client-side state driven by
 * sending the request, not by what the server would have decided) exercise
 * identically on either stream path. Runs unconditionally in both e2e passes.
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

test.describe('scroll-forward stalled loading cancel (UX-AC-5)', () => {
  test.beforeEach(soloPathBeforeEach);
  test.afterEach(soloPathAfterEach);

  test('a stalled forward request should surface a Cancel action after the timeout window', async ({
    page,
    request,
  }) => {
    let buffering = false;
    await page.routeWebSocket(/StreamTerminal/, async (ws) => {
      const server = ws.connectToServer();
      server.onMessage((message) => {
        if (buffering) return; // withhold server->client frames while buffering
        ws.send(message);
      });
      ws.onMessage((message) => server.send(message));
    });

    const client = new SessionClient(BASE_URL);
    const title = `e2e-scroll-stalled-${Date.now()}`;
    const { session } = await createAltScreenScrollSession(client, title);
    await forceLegacyStreamingPath(request, title);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    // Start withholding, then fire the scroll -- its AppScrollbackResponse
    // will never arrive, forcing the real 150ms-show / 8s-stall timers to
    // run for real.
    buffering = true;
    const pill = detail.getScrollLoadingPill();
    await scrollUpUntilVisible(page, detail, () => expect(pill).toBeVisible({ timeout: 2500 }));
    // Polling assertion (not waitForTimeout) across the real 8s stall window.
    await expect(pill).toHaveText(/Still trying/, { timeout: 9000 });

    const cancelButton = detail.getScrollLoadingPillCancel();
    await expect(cancelButton).toBeVisible();
    await cancelButton.click();

    // Cancel resets local state and clears the pill without cancelling the
    // in-flight request (Task 1.4.5b) — the view returns to live.
    await expect(pill).not.toBeAttached();

    buffering = false;
  });
});
