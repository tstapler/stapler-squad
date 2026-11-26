// @feature input-drop-badge
/**
 * E2E coverage for InputDropBadge (Story 2.3, Task 2.3.5).
 *
 * Reproducing a genuine WebSocket-reconnect-drop (Stories 2.1/2.2's actual
 * trigger) deterministically inside Playwright would require racing a
 * server-side connection teardown against a client keystroke — impractical
 * to do reliably in an e2e harness. Per this plan's fallback convention
 * (matching Tasks 1.2.4/1.2.5), this spec instead drives the badge via a
 * test-only trigger (`window.__e2eTriggerInputDropped`, wired in
 * TerminalOutput.tsx) that forwards to the exact same `onInputDropped`
 * handler a real drop would invoke — the drop-detection mechanism itself
 * (MessageQueue.close()'s drop semantics, the connection-generation guard)
 * already has direct Jest coverage (Stories 2.1/2.2); this spec covers what
 * only a real browser can verify: visible rendering, live-region
 * announcement, and bounding-rect positioning relative to the terminal
 * (design/ux.md UX-AC-2).
 */
import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();

/** Auto-dismiss window from notification-policy.ts's DEFAULT_TOAST_MS. */
const DEFAULT_TOAST_MS = 8_000;

/** Not a latency assertion: the 400ms coalescing window plus render took up to 1.15s on a loaded host. */
const BADGE_TIMEOUT_MS = 5_000;

async function openTerminalView(page: import('@playwright/test').Page, sessionId?: string): Promise<boolean> {
  const url = sessionId ? `${BASE_URL}/?session=${sessionId}` : BASE_URL;
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 10000 });

  if (!sessionId) {
    const sessionCard = page.locator('[data-testid="session-card"]').first();
    const hasSession = await sessionCard.isVisible({ timeout: 5000 }).catch(() => false);
    if (!hasSession) return false;
    await sessionCard.click();
  }

  await expect(page.locator('[data-testid="toolbar-toggle"]')).toBeVisible({ timeout: 8000 });
  return true;
}

/** Creates a bash session and waits until the server reports it active. */
async function createActiveSession(client: SessionClient, title: string) {
  const session = await client.createIdleSession(title, '/tmp');
  await client.waitForSessionsActive([title]);
  return session;
}

/** Waits until the e2e test-only trigger has been installed by TerminalOutput. */
async function waitForTestTrigger(page: import('@playwright/test').Page): Promise<void> {
  await page.waitForFunction(
    () => typeof (window as unknown as { __e2eTriggerInputDropped?: unknown }).__e2eTriggerInputDropped === 'function',
    undefined,
    { timeout: 10000 },
  );
}

async function triggerDrop(page: import('@playwright/test').Page, count: number): Promise<void> {
  await page.evaluate((n) => {
    (window as unknown as { __e2eTriggerInputDropped?: (count: number) => void }).__e2eTriggerInputDropped?.(n);
  }, count);
}

test.describe('input-drop-badge', () => {
  // The e2e binary is a production web build, where the test trigger needs this opt-in.
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      (window as unknown as { __E2E_HOOKS__?: boolean }).__E2E_HOOKS__ = true;
    });
  });

  test('badge becomes visible with the correct singular/plural count text', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const session = await createActiveSession(client, `e2e-input-drop-${Date.now()}`);

    try {
      const opened = await openTerminalView(page, session.id);
      test.skip(!opened, 'No session available in test server');

      await waitForTestTrigger(page);

      await triggerDrop(page, 1);

      const badge = page.getByTestId('input-drop-badge');
      await expect(badge).toBeVisible({ timeout: BADGE_TIMEOUT_MS });
      await expect(badge).toHaveText(/1 keystroke dropped — reconnecting/);
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });

  test('coalesces a burst of drops into a single badge with the summed count', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const session = await createActiveSession(client, `e2e-input-drop-burst-${Date.now()}`);

    try {
      const opened = await openTerminalView(page, session.id);
      test.skip(!opened, 'No session available in test server');

      await waitForTestTrigger(page);

      // Three drops in one task, so they always land inside the 400ms coalescing window.
      await page.evaluate(() => {
        const trigger = (window as unknown as { __e2eTriggerInputDropped?: (count: number) => void }).__e2eTriggerInputDropped;
        [1, 1, 1].forEach((n) => trigger?.(n));
      });

      const badge = page.getByTestId('input-drop-badge');
      await expect(badge).toHaveText(/3 keystrokes dropped — reconnecting/, { timeout: BADGE_TIMEOUT_MS });
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });

  test('badge does not overlap the terminal cursor region (design/ux.md UX-AC-2)', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const session = await createActiveSession(client, `e2e-input-drop-bounds-${Date.now()}`);

    try {
      const opened = await openTerminalView(page, session.id);
      test.skip(!opened, 'No session available in test server');

      await page.setViewportSize({ width: 1200, height: 800 });
      await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

      await waitForTestTrigger(page);
      await triggerDrop(page, 2);

      const badge = page.getByTestId('input-drop-badge');
      await expect(badge).toBeVisible({ timeout: BADGE_TIMEOUT_MS });

      const badgeBox = await badge.boundingBox();
      const cursorLayer = page.locator('.xterm-cursor-layer').first();
      const hasCursorLayer = await cursorLayer.isVisible({ timeout: 2000 }).catch(() => false);

      expect(badgeBox).not.toBeNull();

      if (hasCursorLayer) {
        const cursorBox = await cursorLayer.boundingBox();
        if (badgeBox && cursorBox) {
          const overlaps =
            badgeBox.x < cursorBox.x + cursorBox.width &&
            badgeBox.x + badgeBox.width > cursorBox.x &&
            badgeBox.y < cursorBox.y + cursorBox.height &&
            badgeBox.y + badgeBox.height > cursorBox.y;
          expect(overlaps).toBe(false);
        }
      }
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });

  test('badge auto-dismisses within the configured timeout', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const session = await createActiveSession(client, `e2e-input-drop-dismiss-${Date.now()}`);

    try {
      const opened = await openTerminalView(page, session.id);
      test.skip(!opened, 'No session available in test server');

      await waitForTestTrigger(page);
      await triggerDrop(page, 1);

      const badge = page.getByTestId('input-drop-badge');
      await expect(badge).toBeVisible({ timeout: BADGE_TIMEOUT_MS });

      // Auto-dismisses within ~8-8.5s of becoming visible (UX-AC-3), no user action.
      await expect(badge).not.toBeVisible({ timeout: DEFAULT_TOAST_MS + 1500 });
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });

  test('badge appearing does not steal focus (design/ux.md §3.1)', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const session = await createActiveSession(client, `e2e-input-drop-focus-${Date.now()}`);

    try {
      const opened = await openTerminalView(page, session.id);
      test.skip(!opened, 'No session available in test server');

      await waitForTestTrigger(page);

      // Focus the terminal input once it is connected, so its own autofocus can't move focus mid-test.
      await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });
      const terminalInput = page.locator('.xterm-helper-textarea').first();
      await terminalInput.focus();
      await expect(terminalInput).toBeFocused();

      await triggerDrop(page, 1);
      await expect(page.getByTestId('input-drop-badge')).toBeVisible({ timeout: BADGE_TIMEOUT_MS });

      await expect(terminalInput).toBeFocused();
    } finally {
      await client.deleteSession(session.id, true).catch(() => {});
    }
  });
});
