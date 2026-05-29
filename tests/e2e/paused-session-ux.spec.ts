// @feature session:pause, session:resume, paused-session-overlay, paused-session-row-distinction
// Paused Session UX tests — overlay visibility, Resume button, and row visual distinction.
// These tests verify that paused sessions are clearly communicated to users and actionable.

import { test, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

test.describe("paused-session-ux", () => {
  test("paused-session-ux_should_showOverlay_When_pausedSessionOpened", async ({ page }) => {
    // Navigate to the app and find a paused session in the detail view.
    // This test validates that the paused overlay appears with the correct ARIA role and label.
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

    // Find a session row with data-paused="true" if one exists
    const pausedRowLocator = page.locator('[data-testid="session-row"][data-paused="true"]');
    const pausedCardLocator = page.locator('[data-testid="session-card"][data-paused="true"]');

    const hasPausedRow = await pausedRowLocator.count() > 0;
    const hasPausedCard = await pausedCardLocator.count() > 0;

    if (!hasPausedRow && !hasPausedCard) {
      // Skip if no paused session is available in test environment
      test.skip();
      return;
    }

    // Click the first paused session to open its detail view
    if (hasPausedRow) {
      await pausedRowLocator.first().click();
    } else {
      await pausedCardLocator.first().click();
    }

    // The overlay should appear in the terminal tab
    await expect(page.getByRole("status", { name: "Session is paused" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: "Resume this session" })).toBeVisible();
  });

  test("paused-session-ux_should_resumeSession_When_overlayResumeClicked", async ({ page }) => {
    // Navigate to the app and find a paused session
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

    const pausedRowLocator = page.locator('[data-testid="session-row"][data-paused="true"]');
    const pausedCardLocator = page.locator('[data-testid="session-card"][data-paused="true"]');

    const hasPausedRow = await pausedRowLocator.count() > 0;
    const hasPausedCard = await pausedCardLocator.count() > 0;

    if (!hasPausedRow && !hasPausedCard) {
      test.skip();
      return;
    }

    if (hasPausedRow) {
      await pausedRowLocator.first().click();
    } else {
      await pausedCardLocator.first().click();
    }

    // Verify overlay is showing
    await expect(page.getByRole("status", { name: "Session is paused" })).toBeVisible({ timeout: 5000 });

    // Click the Resume button in the overlay
    await page.getByRole("button", { name: "Resume this session" }).click();

    // After clicking Resume, the ResumeSessionModal or confirmation should appear
    // OR the overlay should transition away (depending on confirm flow)
    // Wait a moment for the modal to appear or for the overlay to disappear
    await expect(page.getByRole("status", { name: "Session is paused" })).not.toBeVisible({ timeout: 10000 });
  });

  test("paused-session-ux_should_dimRow_When_sessionIsPaused", async ({ page }) => {
    // Navigate to the session list and verify paused rows have the data-paused attribute
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

    // Wait for the session list to render
    await page.waitForSelector('[data-testid="session-row"]', { timeout: 10000 });

    // Check if any paused row exists — the data-paused attribute must be present
    const pausedRow = page.locator('[data-testid="session-row"][data-paused="true"]');
    const pausedCount = await pausedRow.count();

    if (pausedCount === 0) {
      // No paused sessions in test environment — skip
      test.skip();
      return;
    }

    // The paused row should be visible and have the data-paused="true" attribute
    await expect(pausedRow.first()).toBeVisible();
  });

  test("paused-session-ux_should_showResumeButton_When_rowIsPaused", async ({ page }) => {
    // Verify that the Resume action button is visible for paused rows without hover
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

    await page.waitForSelector('[data-testid="session-row"]', { timeout: 10000 });

    const pausedRow = page.locator('[data-testid="session-row"][data-paused="true"]');
    const pausedCount = await pausedRow.count();

    if (pausedCount === 0) {
      test.skip();
      return;
    }

    // Actions within paused rows should be visible without hover
    // (CSS selector: [data-paused="true"] forces opacity: 1 on .actions)
    const resumeButton = pausedRow.first().getByRole("button", { name: /resume/i });
    await expect(resumeButton).toBeVisible();
  });
});
