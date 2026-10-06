// @feature app-scrollback-forwarding
/**
 * UX-AC-1 (project_plans/app-scrollback-forwarding/implementation/validation.md):
 * a user whose scroll-up succeeds sees the outcome within one scroll gesture
 * -- no additional click/tap is required to reveal what happened.
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
} from './helpers/scroll-forward-fixture';

test.describe('scroll-forward outcome visible (UX-AC-1)', () => {
  test.beforeEach(hubGatedBeforeEach);
  test.afterEach(hubGatedAfterEach);

  test('scroll-up gesture should reveal the outcome affordance with no additional click', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const { session, programPath } = await createAltScreenScrollSession(
      client,
      `e2e-scroll-outcome-visible-${Date.now()}`,
    );
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await openAndWaitConnected(page, detail);

    // The single scroll gesture is the only interaction — no follow-up click.
    // (Retried via scrollUpUntilVisible, not a single one-shot gesture: see
    // its doc comment for the real server-side settling race this guards.)
    const indicator = detail.getScrollSourceIndicator();
    await scrollUpUntilVisible(page, detail, () => expect(indicator).toBeVisible({ timeout: 2500 }));
    // The banner has a decorative aria-hidden icon span alongside the text
    // span (ScrollSourceIndicator.tsx) -- assert the substantive copy via
    // containment rather than an exact toHaveText that would also have to
    // account for the icon glyph.
    await expect(indicator).toContainText(`Viewing ${programPath}'s own history`);
    await expect(indicator).toHaveAttribute('role', 'status');
    await expect(indicator).toHaveAttribute('aria-live', 'polite');
  });
});
