// @feature app-scrollback-forwarding
/**
 * UX-AC-11: all four new/modified text surfaces (loading pill, banner,
 * blocked toast, "no more history" line) meet >= 4.5:1 contrast in both
 * light and dark theme — explicitly including the textMuted-on-plain-
 * terminal-background usage design/ux.md's accessibility summary flags as a
 * new usage context for that token.
 */
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { SessionClient } from './helpers/session-client';
import {
  BASE_URL,
  hubGatedBeforeEach,
  hubGatedAfterEach,
  createAltScreenScrollSession,
  openAndWaitConnected,
  scrollUpUntilVisible,
  attachSecondViewer,
} from './helpers/scroll-forward-fixture';

const SURFACE_SELECTOR =
  '[data-testid="scroll-loading-pill"], [data-testid="scroll-source-indicator"], ' +
  '[data-testid="scroll-blocked-toast"], [data-testid="no-more-app-history"]';

async function scanSurfaces(page: import('@playwright/test').Page) {
  return new AxeBuilder({ page }).include(SURFACE_SELECTOR).withRules(['color-contrast']).analyze();
}

test.describe('scroll-forward contrast (UX-AC-11)', () => {
  test.describe.configure({ timeout: 120000 });
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  for (const themeName of ['light', 'dark'] as const) {
    test(`loading pill, banner, blocked toast, and no-more-history line have zero axe color-contrast violations in ${themeName} theme`, async ({
      page,
      browser,
    }) => {
      // This app switches theme via a `stapler-theme` localStorage value, not
      // prefers-color-scheme media emulation (see accessibility.spec.ts's
      // identical pattern) — must be set before first navigation.
      await page.addInitScript((name) => {
        localStorage.setItem('stapler-theme', name);
        localStorage.setItem('stapler-squad:onboarded', 'true');
      }, themeName);

      // Withhold server frames so the loading pill stays visible long enough
      // for axe to scan it (same page.routeWebSocket() technique as
      // scroll-forward-stalled-loading-cancel.spec.ts) -- registered *before*
      // navigation, not after: page.routeWebSocket only intercepts WebSocket
      // connections opened after it's registered, and the terminal's socket
      // can connect as soon as the session-detail page mounts. Registering
      // it after gotoSession (a real, reproducible bug found via direct
      // console evidence -- AppScrollbackResponse frames with real DELIVERED/
      // AT_TOP outcomes arrived and were processed even while `buffering`
      // was true) let every response through unbuffered, so the pill's
      // 150ms-show timer never got the withheld request it needs to fire for
      // -- hasMoreAppScrollbackRef flipped false after the outcome resolved
      // for real, and every further scroll gesture silently no-op'd for the
      // rest of the 20s retry window.
      //
      // Unlike scroll-forward-stalled-loading-cancel.spec.ts (which never
      // un-buffers), this test *does* flip buffering back off and expects
      // the withheld response to still arrive -- so withheld frames must be
      // queued and replayed on unbuffer, not just conditionally dropped. A
      // dropped-not-queued response left the client stuck forever with
      // isFetchingScrollbackRef true (never reset, since that only happens
      // in the response handler) -- no further scroll gesture could do
      // anything, and the banner this step waits for could never appear
      // (another real, reproducible bug found the same way as the ordering
      // one above).
      let buffering = false;
      let flushQueuedMessages: (() => void) | null = null;
      await page.routeWebSocket(/StreamTerminal/, async (ws) => {
        const server = ws.connectToServer();
        const queued: Parameters<NonNullable<Parameters<typeof server.onMessage>[0]>>[0][] = [];
        flushQueuedMessages = () => {
          for (const message of queued.splice(0)) ws.send(message);
        };
        server.onMessage((message) => {
          if (buffering) {
            queued.push(message);
            return;
          }
          ws.send(message);
        });
        ws.onMessage((message) => server.send(message));
      });
      const unbuffer = () => {
        buffering = false;
        flushQueuedMessages?.();
      };

      const client = new SessionClient(BASE_URL);
      const { session } = await createAltScreenScrollSession(client, `e2e-scroll-contrast-${themeName}-${Date.now()}`);
      const detail = new SessionDetailPage(page);
      await detail.gotoSession(session.id);
      await openAndWaitConnected(page, detail);

      buffering = true;
      const pill = detail.getScrollLoadingPill();
      await scrollUpUntilVisible(page, detail, () => expect(pill).toBeVisible({ timeout: 2500 }));
      let results = await scanSurfaces(page);
      expect(results.violations, `pill, ${themeName} theme`).toHaveLength(0);
      unbuffer();

      // DELIVERED -> banner.
      const indicator = detail.getScrollSourceIndicator();
      await expect(indicator).toBeVisible({ timeout: 10000 });
      results = await scanSurfaces(page);
      expect(results.violations, `banner, ${themeName} theme`).toHaveLength(0);

      // BLOCKED -> toast (second viewer attaches). Must run *before* the
      // AT_TOP step below: design/ux.md's Surface 2 row 2 documents that once
      // hasMoreAppScrollbackRef flips false, "no new request is sent" for the
      // rest of the session, by design ("scrolling simply stops producing new
      // content, exactly like reaching the top of a plain shell's
      // scrollback") -- only AT_TOP sets that flag, so testing BLOCKED first
      // keeps this session able to make one more real request afterward.
      // Reversing this order (found via a real, reproducible failure) isn't
      // an app bug -- the client is behaving exactly as designed -- it's an
      // invalid test scenario the app was never meant to support.
      const { contextB } = await attachSecondViewer(browser, detail, session.id);
      try {
        const toast = detail.getScrollBlockedToast();
        await scrollUpUntilVisible(page, detail, () => expect(toast).toBeVisible({ timeout: 2500 }));
        results = await scanSurfaces(page);
        expect(results.violations, `blocked toast, ${themeName} theme`).toHaveLength(0);
      } finally {
        await contextB.close();
      }

      // AT_TOP -> "no more history" line (banner stays up alongside it).
      const noMore = detail.getNoMoreAppHistory();
      await scrollUpUntilVisible(page, detail, () => expect(noMore).toBeVisible({ timeout: 2500 }));
      results = await scanSurfaces(page);
      expect(results.violations, `no-more-history line, ${themeName} theme`).toHaveLength(0);
    });
  }
});
