// @feature app-scrollback-forwarding
/**
 * REQ-3b (validation.md's Requirement -> Test Mapping): AppScrollGate has no
 * user-identity parameter at all (only subscriberCount int, see
 * session/scroll_gate.go), so it cannot and does not distinguish "two
 * different people" from "one person, two tabs" — this is the committed
 * Phase-1 behavior (pre-mortem P1 #5 — accepted limitation, not a gap), not
 * a bug this spec is trying to catch. This repo has no multi-account concept
 * to simulate a genuinely different user with, so "same user, two tabs" is
 * simulated the same way every other multi-client spec in this suite already
 * does — two independent BrowserContexts against the same session.
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

test.describe('scroll-forward same-user two tabs (REQ-3b)', () => {
  test.describe.configure({ timeout: 120000 });
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('two BrowserContexts authenticated as the same user should still block scroll-forwarding while both are open', async ({
    page,
    browser,
  }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-same-user-two-tabs-${Date.now()}`);
    const detailA = new SessionDetailPage(page);
    await detailA.gotoSession(session.id);
    await openAndWaitConnected(page, detailA);

    // Context B — same "user" in every sense this app tracks (no auth/
    // account concept to differentiate), simulating one person on a second
    // tab/device rather than a second distinct person.
    const { contextB } = await attachSecondViewer(browser, detailA, session.id);
    try {
      // Identical MULTIPLE_VIEWERS blocked toast as the two-distinct-viewers
      // case (UX-AC-4's Case 1) — the gate has no way to tell these apart.
      const toast = detailA.getScrollBlockedToast();
      await scrollUpUntilVisible(page, detailA, () => expect(toast).toBeVisible({ timeout: 2500 }));
      await expect(toast).toHaveAttribute('data-blocked-reason', 'MULTIPLE_VIEWERS');
      await expect(detailA.getConnectionCountIndicator()).toHaveAttribute('data-pulse', 'true');
    } finally {
      // Closing the second tab...
      await contextB.close();
    }

    await expect(detailA.getConnectionCountIndicator()).not.toBeAttached({ timeout: 15000 });

    // ...and retrying from context A succeeds, identical to UX-AC-3.
    await scrollUpUntilVisible(page, detailA, () =>
      expect(detailA.getScrollSourceIndicator()).toBeVisible({ timeout: 2500 }),
    );
  });
});
