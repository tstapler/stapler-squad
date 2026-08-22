// @feature connection-count-indicator
import { test, expect } from '@playwright/test';
import { SessionsPage } from './pages/SessionsPage';
import { SessionDetailPage } from './pages/SessionDetailPage';

/**
 * ConnectionCountIndicator (Epic 4.2, Story 4.2.2) — Surface 1 of
 * project_plans/terminal-multi-connection-streaming/design/ux.md.
 *
 * ConnectionCountIndicator.test.tsx (Jest) already covers everything that
 * can be verified from a single rendered component in isolation: render
 * gating on count, role/aria attributes, icon aria-hidden-ness, tooltip
 * content branching on `sizeMismatch`, debounce/coalescing of rapid count
 * changes, and the departure-announcement/unmount timing — all driven by
 * feeding synthetic `count` props directly to the component.
 *
 * What Jest cannot cover, because it never renders two real connections:
 * whether the indicator actually appears when a *second real browser
 * connection* attaches to the same session's StreamHub, and disappears (via
 * the departure state) when that connection actually goes away. This spec
 * covers exactly that gap, using two independent Playwright browser
 * contexts (separate cookie/localStorage jars, like two real tabs) against
 * the same session.
 *
 * `connection_count` is only populated on the PathHubOwned stream path
 * (STAPLER_SQUAD_USE_STREAM_HUB) — see terminal-hub-path.spec.ts's file
 * header for why this repo's shared e2e server can't yet isolate a single
 * hub-owned session per-test (Story 3.1.1's per-session override hasn't
 * landed). This spec gracefully skips unless an operator explicitly opts in:
 *
 *   STAPLER_SQUAD_USE_STREAM_HUB=true npx playwright test connection-count-indicator.spec.ts
 */

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const HUB_PATH_ENABLED = process.env.STAPLER_SQUAD_USE_STREAM_HUB === 'true';
const ONBOARDED_KEY = 'stapler-squad:onboarded';

async function dismissOnboarding(page: import('@playwright/test').Page) {
  // useOnboarding.ts's 800ms timer otherwise pops a full-viewport onboarding
  // modal mid-test on a fresh (empty-localStorage) context — see
  // repo-path-picker-parity.spec.ts / session-completion-summary.spec.ts.
  await page.addInitScript((key) => {
    try {
      window.localStorage.setItem(key, 'true');
    } catch {
      /* ignore */
    }
  }, ONBOARDED_KEY);
}

test.describe('connection count indicator (multi-connection)', () => {
  test.beforeEach(() => {
    test.skip(
      !HUB_PATH_ENABLED,
      'STAPLER_SQUAD_USE_STREAM_HUB is not set for this run — connection_count is only ' +
        'populated on the PathHubOwned stream path. Re-run with ' +
        'STAPLER_SQUAD_USE_STREAM_HUB=true to exercise this spec.',
    );
  });

  test('indicator appears when a second tab attaches, and is accessible per design/ux.md UX-AC-2/4/6/8', async ({
    page,
    browser,
  }) => {
    await dismissOnboarding(page);

    const sessionsPage = new SessionsPage(page);
    const detailA = new SessionDetailPage(page);

    await sessionsPage.goto();
    await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });
    const sessionId = await sessionsPage.createBashSession('e2e-conn-count');

    await detailA.getTerminalTab().click();
    await expect(detailA.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    // Single-tab case: zero added visual surface (UX-AC-1).
    await expect(detailA.getConnectionCountIndicator()).not.toBeAttached();

    // Second, fully independent browser context — a separate cookie/
    // localStorage jar, the closest Playwright equivalent to a second real
    // browser tab/window attaching to the same session.
    const contextB = await browser.newContext();
    try {
      const pageB = await contextB.newPage();
      await dismissOnboarding(pageB);
      const detailB = new SessionDetailPage(pageB);
      await detailB.gotoSession(sessionId);

      await detailB.getTerminalTab().click();
      await expect(detailB.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
      await expect(pageB.getByText('Connected')).toBeVisible({ timeout: 15000 });

      // Operator (tab A) can tell another connection attached in 0 extra
      // steps (UX-AC-2) — the indicator is near the terminal chrome, no
      // navigation required.
      const indicatorA = detailA.getConnectionCountIndicator();
      await expect(indicatorA).toBeVisible({ timeout: 15000 });
      await expect(indicatorA).toHaveAttribute('role', 'status');
      await expect(indicatorA).toHaveAttribute('aria-live', 'polite');
      await expect(indicatorA).toHaveAttribute('aria-label', '2 connections active');

      // Never role="alert" — this is not an error state (UX-AC-4).
      await expect(page.getByRole('alert', { name: /connections? active/i })).toHaveCount(0);

      // Icon glyph is aria-hidden; visible/announced text carries all
      // meaning (UX-AC-6).
      await expect(indicatorA.locator('[aria-hidden="true"]')).toBeAttached();
      await expect(indicatorA).toContainText('2');

      // Keyboard-navigable: reachable via focus, tooltip revealable via
      // keyboard focus, not hover-only (UX-AC-3/8).
      await indicatorA.focus();
      const tooltipA = detailA.getConnectionCountTooltip();
      await expect(tooltipA).toBeVisible();
      await expect(tooltipA).toHaveText('2 connections active');
      await indicatorA.blur();
      await expect(tooltipA).not.toBeAttached();

      // Also reachable via hover (mouse path), same content.
      await indicatorA.hover();
      await expect(tooltipA).toBeVisible();
      await expect(tooltipA).toHaveText('2 connections active');
    } finally {
      await contextB.close();
    }
  });

  test('indicator announces departure and then disappears when the second tab closes (design/ux.md Step 5)', async ({
    page,
    browser,
  }) => {
    await dismissOnboarding(page);

    const sessionsPage = new SessionsPage(page);
    const detailA = new SessionDetailPage(page);

    await sessionsPage.goto();
    await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });
    const sessionId = await sessionsPage.createBashSession('e2e-conn-count-departure');

    await detailA.getTerminalTab().click();
    await expect(detailA.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    const contextB = await browser.newContext();
    const pageB = await contextB.newPage();
    await dismissOnboarding(pageB);
    const detailB = new SessionDetailPage(pageB);
    await detailB.gotoSession(sessionId);
    await detailB.getTerminalTab().click();
    await expect(detailB.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });
    await expect(pageB.getByText('Connected')).toBeVisible({ timeout: 15000 });

    const indicatorA = detailA.getConnectionCountIndicator();
    await expect(indicatorA).toHaveAttribute('aria-label', '2 connections active', { timeout: 15000 });

    // Close the second tab — hub's SubscriberCount() goes 2 -> 1.
    await contextB.close();

    // A screen-reader user who was told "2 connections active" must not go
    // silent without a signal: the live region updates its text to a
    // departure announcement before the node unmounts (a content mutation
    // within a still-present live region is reliably announced, unlike a
    // node's disappearance in the same commit).
    await expect(indicatorA).toHaveAttribute('aria-label', '1 connection active', { timeout: 15000 });
    await expect(indicatorA).toHaveAttribute('role', 'status');
    await expect(indicatorA).toHaveAttribute('aria-live', 'polite');

    // Then, once the departure announcement has been held long enough to be
    // read, the indicator actually unmounts — back to the single-tab,
    // zero-added-visual-surface state (UX-AC-1).
    await expect(indicatorA).not.toBeAttached({ timeout: 5000 });
  });
});
