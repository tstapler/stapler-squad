/**
 * E2E Tests for ANSI Escape Code Test Harness
 * Tests that all production escape codes can be rendered
 */

import { test, expect } from '@playwright/test';

test.describe('Escape Code Test Harness', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/test/escape-codes');
  });

  test('loads the test harness page', async ({ page }) => {
    // Verify page title
    await expect(page.getByRole('heading', { name: 'ANSI Escape Code Test Harness' })).toBeVisible();

    // Verify library stats are displayed (may show 77+ codes)
    await expect(page.getByText('production occurrences')).toBeVisible();
  });

  test('displays all escape code categories', async ({ page }) => {
    // Check category filter has all categories
    const categorySelect = page.locator('select').first();
    await expect(categorySelect).toBeVisible();

    // Verify categories are present in dropdown (options exist but may not be visible until opened)
    await expect(categorySelect.locator('option', { hasText: 'SGR' })).toBeAttached();
    await expect(categorySelect.locator('option', { hasText: 'Cursor' })).toBeAttached();
    await expect(categorySelect.locator('option', { hasText: 'Erase' })).toBeAttached();
  });

  test('displays critical priority codes', async ({ page }) => {
    // Filter by critical priority
    const prioritySelect = page.locator('select').nth(1);
    await prioritySelect.selectOption('critical');

    // Should show critical codes - use getByText with exact match
    await expect(page.getByText('Erase to End of Line').first()).toBeVisible();
    await expect(page.getByText('Default Foreground').first()).toBeVisible();
  });

  test('can apply quick presets', async ({ page }) => {
    // Click Critical Only preset
    await page.getByRole('button', { name: 'Critical Only' }).click();

    // Verify preset is applied - check that frame rate selector exists
    const frameRateSelect = page.locator('select').filter({ hasText: '30 FPS' });
    await expect(frameRateSelect).toBeVisible();
  });

  test('can start and stop test', async ({ page }) => {
    // Click Critical Only preset to select some codes
    await page.getByRole('button', { name: 'Critical Only' }).click();

    // Start test
    await page.getByRole('button', { name: 'Start Test' }).click();

    // Verify test is running
    await expect(page.getByRole('button', { name: 'Stop Test' })).toBeVisible();

    // Wait for some frames to be generated
    await page.waitForTimeout(500);

    // Stop test
    await page.getByRole('button', { name: 'Stop Test' }).click();

    // Verify test stopped
    await expect(page.getByRole('button', { name: 'Start Test' })).toBeVisible();
  });

  test('category filter changes displayed codes', async ({ page }) => {
    // Filter by SGR category
    const categorySelect = page.locator('select').first();
    await categorySelect.selectOption('SGR');

    // Should show SGR codes - look for at least one
    await expect(page.getByText('Bold').first()).toBeVisible();

    // Filter by Cursor category
    await categorySelect.selectOption('Cursor');

    // Should show Cursor codes
    await expect(page.getByText('Cursor Position').first()).toBeVisible();
  });
});

test.describe('Critical Escape Codes Display', () => {
  test('displays top critical codes', async ({ page }) => {
    await page.goto('/test/escape-codes');

    // Filter to show critical codes
    const prioritySelect = page.locator('select').nth(1);
    await prioritySelect.selectOption('critical');

    // Verify critical codes are listed
    await expect(page.getByText('Erase to End of Line')).toBeVisible();
    await expect(page.getByText('4767 occurrences')).toBeVisible();

    await expect(page.getByText('Default Foreground')).toBeVisible();
    await expect(page.getByText('3678 occurrences')).toBeVisible();

    await expect(page.getByText('Designate G0 character set')).toBeVisible();
  });
});
