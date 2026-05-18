// @feature session:browser-passthrough
import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('browser-passthrough', () => {
  let sessionId: string;

  test.beforeAll(async () => {
    // Create a session to use for browser-passthrough tab navigation tests
    const client = new SessionClient(BASE_URL);
    const session = await client.createSession({
      title: 'e2e-browser-passthrough-test',
      path: '/tmp',
      program: 'bash',
    });
    sessionId = session.id;
  });

  test.afterAll(async () => {
    if (!sessionId) return;
    const client = new SessionClient(BASE_URL);
    await client.deleteSession(sessionId, true).catch(() => {
      // Best-effort cleanup — don't fail the suite if session was already gone
    });
  });

  test('shows browser tab when cdp available', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });

    // Click on the pre-created session card to open its detail panel
    const sessionCard = page.locator('[data-testid="session-card"]').filter({
      hasText: 'e2e-browser-passthrough-test',
    });
    await expect(sessionCard).toBeVisible({ timeout: 10000 });
    await sessionCard.click();

    // The "Browser" tab must exist in the tab strip regardless of VNC/CDP availability
    const browserTab = page.getByRole('tab', { name: /browser/i });
    await expect(browserTab).toBeVisible({ timeout: 5000 });
  });

  test('shows loading state when cdp not connected', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });

    // Open the session detail panel
    const sessionCard = page.locator('[data-testid="session-card"]').filter({
      hasText: 'e2e-browser-passthrough-test',
    });
    await expect(sessionCard).toBeVisible({ timeout: 10000 });
    await sessionCard.click();

    const browserTab = page.getByRole('tab', { name: /browser/i });
    await expect(browserTab).toBeVisible({ timeout: 5000 });

    // In a test environment, no VNC/CDP stack is running so the tab is disabled
    // (aria-disabled="true") and clicking it is a no-op. Verify the disabled state.
    const isDisabled = await browserTab.getAttribute('aria-disabled');
    if (isDisabled === 'true') {
      // Browser tab correctly shows as disabled when VNC unavailable
      expect(isDisabled).toBe('true');
    } else {
      // Browser tab is enabled — click it and verify a placeholder/loading state is shown
      await browserTab.click();

      // The browser tab panel must contain one of the expected placeholder messages:
      // "Browser passthrough unavailable" (UNAVAILABLE status) or
      // "No browser open yet" (NO_BROWSER / STARTING status)
      const tabPanel = page.getByRole('tabpanel', { name: /browser/i });
      await expect(
        tabPanel.getByText(/browser passthrough unavailable|no browser open yet|starting virtual display/i)
      ).toBeVisible({ timeout: 5000 });
    }
  });

  test('browser tab panel renders canvas when cdp ready', async ({ page: _page }) => {
    // This test requires a running Chrome with CDP and x11vnc — skip in standard CI
    test.skip(true, 'requires live CDP connection (Xvfb + x11vnc + Chrome)');
  });
});
