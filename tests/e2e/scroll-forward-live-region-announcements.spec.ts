// @feature app-scrollback-forwarding
/**
 * UX-AC-9 (automated part ONLY): every status surface is reachable by a
 * screen reader via role="status" + aria-live="polite", and each outcome
 * transition (DELIVERED -> AT_TOP -> BLOCKED) announces exactly once, not
 * zero times and not repeatedly.
 *
 * The manual VoiceOver/NVDA smoke test validation.md's UX-AC-9 row also
 * calls for is a separate, non-automatable step (Playwright cannot drive an
 * actual screen reader) and is NOT covered by this file — see this repo's
 * report for that carve-out.
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

declare global {
  interface Window {
    __scrollForwardStatusMounts?: string[];
  }
}

async function startStatusMountObserver(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    window.__scrollForwardStatusMounts = [];
    const observer = new MutationObserver((mutations) => {
      for (const m of mutations) {
        m.addedNodes.forEach((node) => {
          if (node.nodeType !== 1) return;
          const el = node as Element;
          const testid = el.getAttribute('data-testid');
          if (testid && el.getAttribute('role') === 'status') {
            window.__scrollForwardStatusMounts!.push(testid);
          }
        });
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  });
}

async function readStatusMounts(page: import('@playwright/test').Page): Promise<string[]> {
  return page.evaluate(() => window.__scrollForwardStatusMounts ?? []);
}

test.describe('scroll-forward live-region announcements (UX-AC-9, automated)', () => {
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('each outcome transition should announce exactly once via a role="status" aria-live="polite" region', async ({
    page,
    browser,
  }) => {
    const client = new SessionClient(BASE_URL);
    const { session } = await createAltScreenScrollSession(client, `e2e-scroll-live-region-${Date.now()}`);
    const detailA = new SessionDetailPage(page);
    await detailA.gotoSession(session.id);
    await openAndWaitConnected(page, detailA);
    await startStatusMountObserver(page);

    // Transition 1: DELIVERED.
    const indicator = detailA.getScrollSourceIndicator();
    await scrollUpUntilVisible(page, detailA, () => expect(indicator).toBeVisible({ timeout: 2500 }));
    await expect(indicator).toHaveAttribute('role', 'status');
    await expect(indicator).toHaveAttribute('aria-live', 'polite');

    // Transition 2: BLOCKED (second viewer attaches). Must run *before* the
    // AT_TOP transition below -- see scroll-forward-contrast.spec.ts's
    // identical-shape comment: AT_TOP sets hasMoreAppScrollbackRef false,
    // and design/ux.md's Surface 2 row 2 documents that once it's false "no
    // new request is sent" for the rest of the session, by design. Testing
    // BLOCKED first keeps this session able to make one more real request
    // afterward for the AT_TOP transition.
    const { contextB } = await attachSecondViewer(browser, detailA, session.id);
    try {
      const toast = detailA.getScrollBlockedToast();
      await scrollUpUntilVisible(page, detailA, () => expect(toast).toBeVisible({ timeout: 2500 }));
      await expect(toast).toHaveAttribute('role', 'status');
      await expect(toast).toHaveAttribute('aria-live', 'polite');
    } finally {
      await contextB.close();
    }

    // Transition 3: AT_TOP (byte-identical redraw).
    const noMore = detailA.getNoMoreAppHistory();
    await scrollUpUntilVisible(page, detailA, () => expect(noMore).toBeVisible({ timeout: 2500 }));
    await expect(noMore).toHaveAttribute('role', 'status');
    await expect(noMore).toHaveAttribute('aria-live', 'polite');

    const mounts = await readStatusMounts(page);
    const countOf = (testid: string) => mounts.filter((t) => t === testid).length;
    expect(countOf('scroll-source-indicator'), `mounts: ${mounts.join(',')}`).toBe(1);
    expect(countOf('no-more-app-history'), `mounts: ${mounts.join(',')}`).toBe(1);
    expect(countOf('scroll-blocked-toast'), `mounts: ${mounts.join(',')}`).toBe(1);
  });
});
