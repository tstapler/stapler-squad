// @feature get-feature-flags, update-feature-flag
/**
 * E2E tests for the Feature Flags admin page (Surface 3 of
 * project_plans/terminal-resync-reliability), covering validation.md rows 9-11:
 *   - featureFlagsPage_should_ToggleResyncFlagInTwoStepsWithReadableLabel_When_OperatorNavigatesAndClicks
 *   - featureFlagsPage_should_ShowActionableAlertAndAllowRefresh_When_SetFlagRpcFails
 *   - featureFlagsPage_should_ReflectToggleOrShowError_When_FlagToggled
 *
 * Page implementation: web-app/src/app/settings/features/page.tsx.
 * FEATURE_META already maps every `terminal:resync-*` flag to a readable label,
 * so "readable label" assertions check the rendered label text rather than the
 * raw flag name.
 */

import { test, expect } from '@playwright/test';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const RESYNC_FLAG = 'terminal:resync-stagger';
const RESYNC_FLAG_LABEL = 'Terminal resync: stagger bursts';

async function gotoFeaturesPage(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/settings/features`, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForSelector('h1', { timeout: 10000 });
}

/** Waits for the flag's row to render, and returns it with its switch. */
async function waitForFlagRow(page: import('@playwright/test').Page) {
  const row = page.getByTestId('feature-flag-row').filter({
    has: page.getByRole('switch', { name: RESYNC_FLAG_LABEL }),
  });
  await expect(row).toBeVisible({ timeout: 10000 });
  return { row, toggle: row.getByRole('switch', { name: RESYNC_FLAG_LABEL }) };
}

// Reset the flag to a known state (off) before/after each test so tests don't
// depend on ordering or leak state to other spec files sharing the isolated
// test server.
async function resetFlag(request: import('@playwright/test').APIRequestContext) {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { 'Content-Type': 'application/json' },
    data: { name: RESYNC_FLAG, enabled: false },
  });
}

test.describe('terminal-resync-feature-flags', () => {
  test.beforeEach(async ({ request }) => {
    await resetFlag(request);
  });

  test.afterEach(async ({ request }) => {
    await resetFlag(request);
  });

  // Row 9: operator navigates to the page, finds the flag by its readable
  // label, and toggles it via a two-step interaction (find row, click toggle).
  test('featureFlagsPage_should_ToggleResyncFlagInTwoStepsWithReadableLabel_When_OperatorNavigatesAndClicks', async ({ page }) => {
    await gotoFeaturesPage(page);

    // Step 1: locate the flag row by its readable label (not the raw flag name).
    const { toggle } = await waitForFlagRow(page);
    const nameTexts = await page.getByTestId('feature-flag-name').allTextContents();
    expect(nameTexts.some((text) => text.trim().startsWith(RESYNC_FLAG))).toBe(false);

    // Step 2: click the toggle for that flag.
    await expect(toggle).toHaveAttribute('aria-checked', 'false');
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-checked', 'true', { timeout: 10000 });
  });

  // Row 11: toggling reflects the new state in the UI (badge + aria-pressed)
  // without a page reload, and a reload preserves the persisted value.
  test('featureFlagsPage_should_ReflectToggleOrShowError_When_FlagToggled', async ({ page }) => {
    await gotoFeaturesPage(page);

    const { row, toggle } = await waitForFlagRow(page);
    await expect(row.getByTestId('feature-flag-status')).toHaveText('Off');

    await toggle.click();

    // Badge flips to "On" and no error is shown.
    await expect(row.getByTestId('feature-flag-status')).toHaveText('On', { timeout: 10000 });
    // Scoped to this page's errors: unrelated host alerts (e.g. fork pressure) also use role=alert.
    await expect(page.getByTestId('feature-flag-row-error')).toHaveCount(0);
    await expect(page.getByTestId('feature-flags-load-error')).toHaveCount(0);

    // Reload — the toggled state must persist (server-backed, not local-only state).
    await gotoFeaturesPage(page);
    await expect((await waitForFlagRow(page)).toggle).toHaveAttribute('aria-checked', 'true');
  });

  // Row 10: when the UpdateFeatureFlag RPC fails, the page must show an
  // actionable alert (role="alert") and let the operator refresh, rather than
  // silently failing or leaving the toggle in an inconsistent state.
  test('featureFlagsPage_should_ShowActionableAlertAndAllowRefresh_When_SetFlagRpcFails', async ({ page }) => {
    await page.route('**/api/session.v1.SessionService/UpdateFeatureFlag', (route) => {
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"code":"internal","message":"forced failure"}' });
    });

    await gotoFeaturesPage(page);

    const { row, toggle } = await waitForFlagRow(page);
    await toggle.click();

    // The row surfaces the failure as role="alert" with a Retry action.
    const alert = row.getByTestId('feature-flag-row-error');
    await expect(alert).toBeVisible({ timeout: 10000 });
    await expect(alert).toContainText(`Couldn't change ${RESYNC_FLAG_LABEL}`);
    await expect(alert.getByRole('button', { name: 'Retry' })).toBeVisible();

    // The toggle must not have optimistically flipped to "enabled".
    await expect(toggle).toHaveAttribute('aria-checked', 'false');

    // Unroute and confirm a normal refresh recovers (the "allow refresh" half
    // of the row's name).
    await page.unroute('**/api/session.v1.SessionService/UpdateFeatureFlag');
    await gotoFeaturesPage(page);
    await expect((await waitForFlagRow(page)).toggle).toHaveAttribute('aria-checked', 'false');
    await expect(page.getByTestId('feature-flag-row-error')).toHaveCount(0);
  });
});
