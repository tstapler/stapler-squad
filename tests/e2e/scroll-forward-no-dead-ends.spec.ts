// @feature app-scrollback-forwarding
/**
 * UX-AC-7: every stopping point (Blocked toast, "no more history" line,
 * stalled-loading pill) exposes a concrete next action — no state requires
 * page.reload().
 */
import { test, expect } from '@playwright/test';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { SessionClient } from './helpers/session-client';
import {
  BASE_URL,
  hubGatedBeforeEach,
  hubGatedAfterEach,
  createAltScreenScrollSession,
  openAndWaitConnected,
  scrollDownOnTerminal,
  scrollUpUntilVisible,
  attachSecondViewer,
} from './helpers/scroll-forward-fixture';

test.describe('scroll-forward no dead ends (UX-AC-7)', () => {
  test.describe.configure({ timeout: 120000 });
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('Blocked toast exposes an actionable next step, no reload required', async ({ page, browser }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-deadend-blocked-${Date.now()}`);
    const detailA = new SessionDetailPage(page);
    await detailA.gotoSession(session.id);
    await openAndWaitConnected(page, detailA);

    const { contextB } = await attachSecondViewer(browser, detailA, session.id);
    try {
      const toast = detailA.getScrollBlockedToast();
      await scrollUpUntilVisible(page, detailA, () => expect(toast).toBeVisible({ timeout: 2500 }));
      // The toast itself names the concrete next action.
      await expect(toast).toContainText(/close other tabs or sessions/i);
    } finally {
      await contextB.close();
    }

    // The action described in the toast — closing the other tab — then
    // actually works, with no reload.
    await scrollUpUntilVisible(page, detailA, () =>
      expect(detailA.getScrollSourceIndicator()).toBeVisible({ timeout: 2500 }),
    );
  });

  test('"no more history" state still allows scrolling back down to live, no reload required', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-deadend-attop-${Date.now()}`);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    await scrollUpUntilVisible(page, detail, () =>
      expect(detail.getScrollSourceIndicator()).toBeVisible({ timeout: 2500 }),
    ); // DELIVERED
    await scrollUpUntilVisible(page, detail, () =>
      expect(detail.getNoMoreAppHistory()).toBeVisible({ timeout: 2500 }),
    ); // AT_TOP (byte-identical redraw)

    // Scrolling back down is the documented next action -- must not throw,
    // hang, or require any special affordance.
    await scrollDownOnTerminal(page, detail);
    await expect(detail.getTerminalPanel()).toBeVisible();
  });

  test('stalled-loading pill exposes a clickable Cancel, no reload required', async ({ page }) => {
    let buffering = false;
    await page.routeWebSocket(/StreamTerminal/, async (ws) => {
      const server = ws.connectToServer();
      server.onMessage((message) => {
        if (buffering) return;
        ws.send(message);
      });
      ws.onMessage((message) => server.send(message));
    });

    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-deadend-stalled-${Date.now()}`);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    buffering = true;
    await scrollUpUntilVisible(page, detail, () =>
      expect(detail.getScrollLoadingPill()).toBeVisible({ timeout: 2500 }),
    );

    const cancelButton = detail.getScrollLoadingPillCancel();
    await expect(cancelButton).toBeVisible({ timeout: 9000 });
    await expect(cancelButton).toBeEnabled();
    await cancelButton.click();
    await expect(detail.getScrollLoadingPill()).not.toBeAttached();

    buffering = false;
  });
});
