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

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const RESYNC_FLAG = 'terminal:resync-stagger';
const RESYNC_FLAG_LABEL = 'Terminal resync: stagger bursts';

async function gotoFeaturesPage(page: import('@playwright/test').Page) {
  await page.goto(`${BASE_URL}/settings/features`, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForSelector('h1', { timeout: 10000 });
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
    const row = page.locator('div').filter({ hasText: RESYNC_FLAG_LABEL }).last();
    await expect(row).toBeVisible({ timeout: 10000 });
    expect(await page.getByText(RESYNC_FLAG, { exact: true }).count()).toBe(0);

    // Step 2: click the toggle for that flag.
    const toggleButton = page.getByRole('button', { name: `Enable ${RESYNC_FLAG_LABEL}` });
    await expect(toggleButton).toBeVisible({ timeout: 10000 });
    await expect(toggleButton).toHaveAttribute('aria-pressed', 'false');
    await toggleButton.click();

    // Toggling flips the accessible name/state — this is the "two steps"
    // (navigate+locate, then click) the row name documents.
    const disableButton = page.getByRole('button', { name: `Disable ${RESYNC_FLAG_LABEL}` });
    await expect(disableButton).toBeVisible({ timeout: 10000 });
    await expect(disableButton).toHaveAttribute('aria-pressed', 'true');
  });

  // Row 11: toggling reflects the new state in the UI (badge + aria-pressed)
  // without a page reload, and a reload preserves the persisted value.
  test('featureFlagsPage_should_ReflectToggleOrShowError_When_FlagToggled', async ({ page }) => {
    await gotoFeaturesPage(page);

    const toggleButton = page.getByRole('button', { name: `Enable ${RESYNC_FLAG_LABEL}` });
    await expect(toggleButton).toBeVisible({ timeout: 10000 });

    const row = page.locator('div').filter({ hasText: RESYNC_FLAG_LABEL }).last();
    await expect(row.getByText('Off', { exact: true })).toBeVisible();

    await toggleButton.click();

    // Badge flips to "On" and no error is shown.
    await expect(row.getByText('On', { exact: true })).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('alert')).toHaveCount(0);

    // Reload — the toggled state must persist (server-backed, not local-only state).
    await gotoFeaturesPage(page);
    const disableButton = page.getByRole('button', { name: `Disable ${RESYNC_FLAG_LABEL}` });
    await expect(disableButton).toBeVisible({ timeout: 10000 });
    await expect(disableButton).toHaveAttribute('aria-pressed', 'true');
  });

  // Row 10: when the UpdateFeatureFlag RPC fails, the page must show an
  // actionable alert (role="alert") and let the operator refresh, rather than
  // silently failing or leaving the toggle in an inconsistent state.
  test('featureFlagsPage_should_ShowActionableAlertAndAllowRefresh_When_SetFlagRpcFails', async ({ page }) => {
    await page.route('**/api/session.v1.SessionService/UpdateFeatureFlag', (route) => {
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"code":"internal","message":"forced failure"}' });
    });

    await gotoFeaturesPage(page);

    const toggleButton = page.getByRole('button', { name: `Enable ${RESYNC_FLAG_LABEL}` });
    await expect(toggleButton).toBeVisible({ timeout: 10000 });
    await toggleButton.click();

    // FeatureFlagsContext.setFlag catches the error and surfaces role="alert"
    // text ending in "Please refresh." (page.tsx's errorMessage paragraph).
    // Next.js's own route-announcer (#__next-route-announcer__) is also
    // role="alert" and always present, so scope to the visible one with text.
    const alert = page.getByRole('alert').filter({ hasText: 'refresh' });
    await expect(alert).toBeVisible({ timeout: 10000 });
    await expect(alert).toContainText('refresh', { ignoreCase: true });

    // The toggle must not have optimistically flipped to "enabled" — the
    // failed RPC's response never arrived, so local state is unchanged.
    await expect(toggleButton).toHaveAttribute('aria-pressed', 'false');

    // Unroute and confirm a normal refresh recovers (the "allow refresh" half
    // of the row's name).
    await page.unroute('**/api/session.v1.SessionService/UpdateFeatureFlag');
    await gotoFeaturesPage(page);
    await expect(page.getByRole('alert').filter({ hasText: 'refresh' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: `Enable ${RESYNC_FLAG_LABEL}` })).toBeVisible({ timeout: 10000 });
  });
});
