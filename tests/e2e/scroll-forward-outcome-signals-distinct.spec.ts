// @feature app-scrollback-forwarding
/**
 * UX-AC-6: every ScrollForwardOutcome except NO_CAPABILITY produces a
 * distinguishable signal; NO_CAPABILITY matches today's pre-existing (flag-
 * off) baseline exactly, per design/ux.md Surface 5's explicit scoping.
 */
import { test, expect } from '@playwright/test';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { SessionClient } from './helpers/session-client';
import {
  BASE_URL,
  hubGatedBeforeEach,
  hubGatedAfterEach,
  dismissOnboarding,
  createAltScreenScrollSession,
  openAndWaitConnected,
  scrollUpOnTerminal,
  scrollUpUntilVisible,
  attachSecondViewer,
} from './helpers/scroll-forward-fixture';

async function assertNoAppScrollbackSurfaces(page: import('@playwright/test').Page, detail: SessionDetailPage) {
  await expect(detail.getScrollSourceIndicator()).not.toBeAttached();
  await expect(detail.getScrollBlockedToast()).not.toBeAttached();
  await expect(detail.getNoMoreAppHistory()).not.toBeAttached();
}

test.describe('scroll-forward outcome signals — DELIVERED/AT_TOP/BLOCKED (UX-AC-6)', () => {
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('DELIVERED renders the ScrollSourceIndicator and nothing else', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-signal-delivered-${Date.now()}`);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    await scrollUpUntilVisible(page, detail, () =>
      expect(detail.getScrollSourceIndicator()).toBeVisible({ timeout: 2500 }),
    );

    await expect(detail.getScrollBlockedToast()).not.toBeAttached();
    await expect(detail.getNoMoreAppHistory()).not.toBeAttached();
  });

  test('AT_TOP renders the "no more history" line once the redraw stops changing', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-signal-attop-${Date.now()}`);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    // First scroll -> DELIVERED (fixture's before->after redraw).
    await scrollUpUntilVisible(page, detail, () =>
      expect(detail.getScrollSourceIndicator()).toBeVisible({ timeout: 2500 }),
    );

    // Second scroll -> the fixture redraws byte-identical "after" content
    // again, which session/instance_scroll_forward.go computes as AT_TOP.
    await scrollUpUntilVisible(page, detail, () =>
      expect(detail.getNoMoreAppHistory()).toBeVisible({ timeout: 2500 }),
    );
    await expect(detail.getNoMoreAppHistory()).toHaveText('No more history available');
    // The banner stays up alongside AT_TOP (design/ux.md Surface 2: "does
    // not flicker off between pages").
    await expect(detail.getScrollSourceIndicator()).toBeVisible();
    await expect(detail.getScrollBlockedToast()).not.toBeAttached();
  });

  test('BLOCKED renders the blocked toast and nothing else', async ({ page, browser }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-signal-blocked-${Date.now()}`);
    const detailA = new SessionDetailPage(page);
    await detailA.gotoSession(session.id);
    await openAndWaitConnected(page, detailA);

    const { contextB } = await attachSecondViewer(browser, detailA, session.id);
    try {
      await scrollUpUntilVisible(page, detailA, () =>
        expect(detailA.getScrollBlockedToast()).toBeVisible({ timeout: 2500 }),
      );
      await expect(detailA.getScrollSourceIndicator()).not.toBeAttached();
      await expect(detailA.getNoMoreAppHistory()).not.toBeAttached();
    } finally {
      await contextB.close();
    }
  });
});

test.describe('scroll-forward outcome signals — NO_CAPABILITY baseline (UX-AC-6)', () => {
  // NO_CAPABILITY is not reachable in practice with the flag on (AppScrollGate
  // refuses to attempt forwarding when resolveScrollAdapter returns nil) --
  // design/ux.md Surface 5 scopes this criterion to matching the flag-off
  // baseline instead, which needs no hub path or seeding at all.
  test.beforeEach(async ({ page }) => {
    await dismissOnboarding(page);
  });

  test('flag-off scroll-up is a silent no-op, matching NO_CAPABILITY’s defensive baseline', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-signal-nocap-${Date.now()}`);
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    await scrollUpOnTerminal(page, detail);

    // No app-scrollback surface renders at all -- byte-for-byte the same as
    // today's baseline for an unsupported/flag-off program.
    await assertNoAppScrollbackSurfaces(page, detail);
    await expect(detail.getScrollLoadingPill()).not.toBeAttached();
  });
});
