import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: nav-navigation — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['ui-header-nav'], // TODO: add to catalog
] as const;
// Regression tests for nav link navigation from the sessions page.
// Bug: window.history.replaceState in the nav click handler was intercepted by
// Next.js's patched router, causing navigation to "/" instead of the target route.

import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('nav-navigation', () => {
  test('nav-navigation > Unfinished link navigates from sessions page', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const link = page.getByRole('link', { name: /unfinished/i }).first();
    await expect(link).toBeVisible({ timeout: 10000 });
    await link.click();

    await expect(page).toHaveURL(/\/unfinished/, { timeout: 5000 });
  });

  test('nav-navigation > Review Queue link navigates from sessions page', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const link = page.getByRole('link', { name: /review queue/i }).first();
    await expect(link).toBeVisible({ timeout: 10000 });
    await link.click();

    await expect(page).toHaveURL(/\/review-queue/, { timeout: 5000 });
  });

  test('nav-navigation > Unfinished link navigates when session param is in URL', async ({ page }) => {
    // Navigate with a fake ?session= param to reproduce the original bug scenario.
    // The session ID doesn't need to be valid — we just need the param present.
    await page.goto(`${BASE_URL}/?session=nonexistent-session-id`, {
      waitUntil: 'domcontentloaded',
      timeout: 15000,
    });

    const link = page.getByRole('link', { name: /unfinished/i }).first();
    await expect(link).toBeVisible({ timeout: 10000 });
    await link.click();

    await expect(page).toHaveURL(/\/unfinished/, { timeout: 5000 });
  });

  test('nav-navigation > Review Queue link navigates when session param is in URL', async ({ page }) => {
    await page.goto(`${BASE_URL}/?session=nonexistent-session-id`, {
      waitUntil: 'domcontentloaded',
      timeout: 15000,
    });

    const link = page.getByRole('link', { name: /review queue/i }).first();
    await expect(link).toBeVisible({ timeout: 10000 });
    await link.click();

    await expect(page).toHaveURL(/\/review-queue/, { timeout: 5000 });
  });

  test('nav-navigation > Sessions link navigates back from unfinished page', async ({ page }) => {
    await page.goto(`${BASE_URL}/unfinished`, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const link = page.getByRole('link', { name: /^sessions$/i }).first();
    await expect(link).toBeVisible({ timeout: 10000 });
    await link.click();

    await expect(page).toHaveURL(/^\/?$|^\/?[?#]/, { timeout: 5000 });
  });
});
