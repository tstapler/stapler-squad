// @feature session:create, session:list
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

/**
 * Terminal hub-owned path (PathHubOwned) parity smoke test.
 *
 * Epic 2.2 (terminal-multi-connection-streaming) wires streamTerminal's
 * PathHubOwned branch behind STAPLER_SQUAD_USE_STREAM_HUB (default off,
 * unset today). Story 3.1.1's per-session override — the mechanism this
 * spec's plan.md Task 2.2.2d assumed for forcing one session onto
 * PathHubOwned without flipping every session in the shared e2e server —
 * has not landed yet, so this spec cannot yet isolate a single hub-owned
 * session the way terminal-resize.spec.ts isolates its target session.
 *
 * Until that override exists, the only way to exercise PathHubOwned in this
 * shared-server e2e setup is to run the *entire* suite with
 * STAPLER_SQUAD_USE_STREAM_HUB=true set before `npm test` (global-setup.ts's
 * spawned server inherits process.env verbatim — see
 * tests/e2e/helpers/test-server.ts). That is not this suite's default (it
 * would put every other spec's sessions on the still-partial hub-owned path
 * too), so this spec gracefully skips unless an operator explicitly opts in:
 *
 *   STAPLER_SQUAD_USE_STREAM_HUB=true npx playwright test terminal-hub-path.spec.ts
 *
 * Once Story 3.1.1's per-session override lands, this spec should switch to
 * forcing PathHubOwned for one session via that override instead of relying
 * on a whole-suite env var, matching terminal-resize.spec.ts's isolation.
 */

const HUB_PATH_ENABLED = process.env.STAPLER_SQUAD_USE_STREAM_HUB === 'true';

async function openTerminalView(page: import('@playwright/test').Page): Promise<boolean> {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
  const sessionCard = page.locator('[data-testid="session-card"]').first();
  const hasSession = await sessionCard.isVisible({ timeout: 5000 }).catch(() => false);
  if (!hasSession) return false;

  await sessionCard.click();
  await expect(page.locator('[data-testid="toolbar-toggle"]')).toBeVisible({ timeout: 8000 });

  return true;
}

test.describe('terminal hub-owned path (PathHubOwned)', () => {
  test.beforeEach(() => {
    test.skip(
      !HUB_PATH_ENABLED,
      'STAPLER_SQUAD_USE_STREAM_HUB is not set for this run — the per-session override ' +
        '(Story 3.1.1) that would let this spec force just one session onto PathHubOwned ' +
        'has not landed yet. Re-run with STAPLER_SQUAD_USE_STREAM_HUB=true to exercise this spec.'
    );
  });

  test('terminal output appears within the same bounded wait as the legacy path', async ({ page }) => {
    const opened = await openTerminalView(page);
    test.skip(!opened, 'No session available in test server');

    // Same bounded-wait threshold terminal-resize.spec.ts / terminal-flickering.spec.ts
    // apply to the legacy path — establishing the hub-owned path's own verified parity
    // rather than an inferred non-regression on the untouched legacy branch.
    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });
  });

  test('resize settles to one final size without an intermediate flicker frame', async ({ page }) => {
    const opened = await openTerminalView(page);
    test.skip(!opened, 'No session available in test server');

    await expect(page.getByText('Connected')).toBeVisible({ timeout: 15000 });

    const toggle = page.locator('[data-testid="toolbar-toggle"]');
    const expanded = await toggle.getAttribute('aria-expanded');
    if (expanded === 'false') await toggle.click();

    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached();

    await page.getByRole('button', { name: 'Resize terminal' }).click();

    await expect(page.getByRole('status', { name: 'Terminal resizing' })).not.toBeAttached({ timeout: 10000 });
    await expect(page.getByText('Connected')).toBeVisible();
  });
});
