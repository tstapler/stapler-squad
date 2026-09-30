// @feature app-scrollback-forwarding
/**
 * UX-AC-10: the Blocked toast's [Got it] dismiss button is keyboard-
 * reachable via Tab and activatable via Enter/Space.
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

test.describe('scroll-forward blocked toast keyboard dismissal (UX-AC-10)', () => {
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('Blocked toast dismiss button should be focusable and dismissible via Enter', async ({ page, browser }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-toast-keyboard-${Date.now()}`);
    const detailA = new SessionDetailPage(page);
    await detailA.gotoSession(session.id);
    await openAndWaitConnected(page, detailA);

    const { contextB } = await attachSecondViewer(browser, detailA, session.id);
    try {
      const toast = detailA.getScrollBlockedToast();
      await scrollUpUntilVisible(page, detailA, () => expect(toast).toBeVisible({ timeout: 2500 }));
      const dismissButton = detailA.getScrollBlockedToastDismiss();
      await expect(dismissButton).toBeVisible();

      // Reachable via keyboard focus (not only mouse) -- role="button" with
      // no explicit tabindex is natively focusable/Tab-reachable.
      await dismissButton.focus();
      await expect(dismissButton).toBeFocused();

      // Activatable via Enter.
      await page.keyboard.press('Enter');
      await expect(toast).not.toBeAttached();
    } finally {
      await contextB.close();
    }
  });

  test('Blocked toast dismiss button should be dismissible via Space', async ({ page, browser }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-toast-keyboard-space-${Date.now()}`);
    const detailA = new SessionDetailPage(page);
    await detailA.gotoSession(session.id);
    await openAndWaitConnected(page, detailA);

    const { contextB } = await attachSecondViewer(browser, detailA, session.id);
    try {
      const toast = detailA.getScrollBlockedToast();
      await scrollUpUntilVisible(page, detailA, () => expect(toast).toBeVisible({ timeout: 2500 }));
      const dismissButton = detailA.getScrollBlockedToastDismiss();

      await dismissButton.focus();
      await expect(dismissButton).toBeFocused();
      await page.keyboard.press(' ');
      await expect(toast).not.toBeAttached();
    } finally {
      await contextB.close();
    }
  });
});
