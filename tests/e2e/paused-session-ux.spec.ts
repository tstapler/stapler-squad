import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: paused-session-ux — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['session-pause'], // TODO: add to catalog
  // FEATURE_CATALOG['session-resume'], // TODO: add to catalog
  // FEATURE_CATALOG['paused-session-overlay'], // TODO: add to catalog
  // FEATURE_CATALOG['paused-session-row-distinction'], // TODO: add to catalog
] as const;
// Paused Session UX tests — overlay visibility, Resume button, and row visual distinction.
// These tests verify that paused sessions are clearly communicated to users and actionable.
//
// TODO: wire a beforeAll fixture that creates + pauses a session via the RPC so these tests
// run deterministically in CI instead of requiring a pre-existing paused session.
// Track: implement fixture using CreateSession + UpdateSession(status=PAUSED) RPCs.

import { test, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

test.describe("paused-session-ux", () => {
  test.fixme(
    "paused-session-ux_should_showOverlay_When_pausedSessionOpened",
    // Requires a paused session fixture (beforeAll via CreateSession + UpdateSession RPC).
    async ({ page }) => {
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

      const pausedRowLocator = page.locator('[data-testid="session-row"][data-paused="true"]');
      const pausedCardLocator = page.locator('[data-testid="session-card"][data-paused="true"]');

      const hasPausedRow = await pausedRowLocator.count() > 0;
      const hasPausedCard = await pausedCardLocator.count() > 0;
      expect(hasPausedRow || hasPausedCard, "no paused session fixture — see TODO above").toBe(true);

      if (hasPausedRow) {
        await pausedRowLocator.first().click();
      } else {
        await pausedCardLocator.first().click();
      }

      await expect(page.getByRole("status", { name: "Session is paused" })).toBeVisible({ timeout: 5000 });
      await expect(page.getByRole("button", { name: "Resume this session" })).toBeVisible();
    }
  );

  test.fixme(
    "paused-session-ux_should_resumeSession_When_overlayResumeClicked",
    // Requires a paused session fixture.
    async ({ page }) => {
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

      const pausedRowLocator = page.locator('[data-testid="session-row"][data-paused="true"]');
      const pausedCardLocator = page.locator('[data-testid="session-card"][data-paused="true"]');

      const hasPausedRow = await pausedRowLocator.count() > 0;
      const hasPausedCard = await pausedCardLocator.count() > 0;
      expect(hasPausedRow || hasPausedCard, "no paused session fixture — see TODO above").toBe(true);

      if (hasPausedRow) {
        await pausedRowLocator.first().click();
      } else {
        await pausedCardLocator.first().click();
      }

      await expect(page.getByRole("status", { name: "Session is paused" })).toBeVisible({ timeout: 5000 });
      await page.getByRole("button", { name: "Resume this session" }).click();
      await expect(page.getByRole("status", { name: "Session is paused" })).not.toBeVisible({ timeout: 10000 });
    }
  );

  test.fixme(
    "paused-session-ux_should_dimRow_When_sessionIsPaused",
    // Requires a paused session fixture.
    async ({ page }) => {
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
      await page.waitForSelector('[data-testid="session-row"]', { timeout: 10000 });

      const pausedRow = page.locator('[data-testid="session-row"][data-paused="true"]');
      expect(await pausedRow.count(), "no paused session fixture — see TODO above").toBeGreaterThan(0);
      await expect(pausedRow.first()).toBeVisible();
    }
  );

  test.fixme(
    "paused-session-ux_should_showResumeButton_When_rowIsPaused",
    // Requires a paused session fixture.
    async ({ page }) => {
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
      await page.waitForSelector('[data-testid="session-row"]', { timeout: 10000 });

      const pausedRow = page.locator('[data-testid="session-row"][data-paused="true"]');
      expect(await pausedRow.count(), "no paused session fixture — see TODO above").toBeGreaterThan(0);

      const resumeButton = pausedRow.first().getByRole("button", { name: /resume/i });
      await expect(resumeButton).toBeVisible();
    }
  );
});
