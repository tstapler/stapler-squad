// @feature app-scrollback-forwarding
/**
 * UX-AC-2: a user viewing forwarded history can return to the live session
 * view with no new "exit history mode" button — none exists anywhere in the
 * DOM at any point in this flow.
 *
 * TerminalOutput.tsx clears the ScrollSourceIndicator banner only when a
 * subsequent *normal* (non-app-scrollback) output frame arrives (Task
 * 1.4.2b) — there is no explicit client->server "return to live" RPC for an
 * alt-screen session (the wheel listener only handles deltaY < 0). The
 * fixture script's Ctrl+L handler stands in for the live agent producing new
 * output, the only real mechanism that clears the banner.
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
  scrollDownOnTerminal,
} from './helpers/scroll-forward-fixture';

test.describe('scroll-forward return to live (UX-AC-2)', () => {
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('scrolling down from forwarded history should return to the live tail without an exit button', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-return-live-${Date.now()}`);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    await scrollUpUntilVisible(page, detail, () =>
      expect(detail.getScrollSourceIndicator()).toBeVisible({ timeout: 2500 }),
    );

    // No new "exit history mode" button/testid exists anywhere in the DOM
    // while forwarded history is showing.
    await expect(page.getByTestId('exit-history')).toHaveCount(0);
    await expect(page.getByRole('button', { name: /exit history/i })).toHaveCount(0);

    // The live agent resumes producing output (Ctrl+L trigger) — the banner
    // must clear on its own, no button click required.
    await detail.getTerminalPanel().click();
    await page.keyboard.press('Control+l');
    await expect(detail.getScrollSourceIndicator()).not.toBeAttached({ timeout: 10000 });

    // The user's natural next action — scrolling toward the tail — completes
    // the return-to-live flow without error and without needing any new UI.
    await scrollDownOnTerminal(page, detail);
    await expect(page.getByTestId('exit-history')).toHaveCount(0);
    await expect(detail.getScrollSourceIndicator()).not.toBeAttached();
  });
});
