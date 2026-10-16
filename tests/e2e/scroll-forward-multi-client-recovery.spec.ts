// @feature app-scrollback-forwarding
/**
 * UX-AC-3: a user blocked by a second connection reaches a working scroll in
 * <= 2 steps once the other connection is actually closed — (1) close the
 * other tab, (2) scroll up again. No page reload, no retry button.
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
  scrollUpUntilVisible,
  attachSecondViewer,
} from './helpers/scroll-forward-fixture';

test.describe('scroll-forward multi-client recovery (UX-AC-3)', () => {
  test.describe.configure({ timeout: 120000 });

  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('closing the second tab followed by one retry scroll should succeed with no reload or retry button', async ({
    page,
    browser,
  }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-multi-client-${Date.now()}`);
    const detailA = new SessionDetailPage(page);
    await detailA.gotoSession(session.id);
    await openAndWaitConnected(page, detailA);

    const { contextB } = await attachSecondViewer(browser, detailA, session.id);
    try {
      await scrollUpUntilVisible(page, detailA, () =>
        expect(detailA.getScrollBlockedToast()).toBeVisible({ timeout: 2500 }),
      );
      await expect(detailA.getScrollBlockedToast()).toHaveAttribute('data-blocked-reason', 'MULTIPLE_VIEWERS');
      await expect(detailA.getConnectionCountIndicator()).toHaveAttribute('data-pulse', 'true');
    } finally {
      // Step 1: close the other tab/session.
      await contextB.close();
    }

    await expect(detailA.getConnectionCountIndicator()).not.toBeAttached({ timeout: 15000 });

    // Step 2: the very next scroll gesture just works — no page.reload(), no
    // retry button.
    await scrollUpUntilVisible(page, detailA, () =>
      expect(detailA.getScrollSourceIndicator()).toBeVisible({ timeout: 2500 }),
    );
  });
});
