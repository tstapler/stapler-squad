import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: enter-detection — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['session-approval-enter-detection'], // TODO: add to catalog
] as const;
/**
 * E2E tests for Enter-key detection and optimistic approval dismissal.
 *
 * These tests require a live server at http://localhost:8544 with a session
 * that has a pending approval. Because approval seeding requires complex
 * backend fixture setup (a blocking decisionCh that stays open), the approval-
 * seeding steps are skipped with test.skip until a fixture is available.
 *
 * See tests/e2e/review-queue.spec.ts for precedent on approval fixture setup.
 *
 * FR-3 note: Backend input-received events are explicitly deferred (ADR-5).
 * The frontend debounced re-fetch achieves the same observable UX result.
 */

import { test, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL ?? "http://localhost:8544";

test.describe("enter-detection", () => {
  test.skip(
    "T-E2E-001: enter-detection_should_clearApprovalDrawer_When_userPressesEnterInTerminal",
    async ({ page }) => {
      // TODO: requires approval fixture (session with active pending approval)
      // Setup: navigate to a session that has a pending approval
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

      // Open the approval drawer (badge should be visible)
      const approvalBadge = page.getByTestId("approval-nav-badge");
      await expect(approvalBadge).toBeVisible({ timeout: 5000 });

      // Click the badge to open the drawer
      await approvalBadge.click();
      const drawer = page.getByRole("complementary", { name: /pending approvals/i });
      await expect(drawer).toBeVisible();

      // Navigate to the terminal session
      const terminal = page.getByTestId("xterm-container").first();
      await terminal.click();

      // Send Enter keystroke
      await page.keyboard.press("Enter");

      // Within 100ms, the approval entry should clear from the drawer (AC-1)
      await expect(drawer.getByTestId("approval-card")).toBeHidden({ timeout: 100 });

      // Nav badge count should decrement to 0 (AC-2)
      await expect(approvalBadge).toBeHidden({ timeout: 100 });
    }
  );

  test.skip(
    "T-E2E-002: enter-detection_should_clearInTabBadge_When_userPressesEnterInTerminal",
    async ({ page }) => {
      // TODO: requires approval fixture
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

      // Verify the session row shows a "Needs Approval" badge (AC-2)
      const needsApprovalBadge = page.getByRole("status", { name: /needs approval/i }).first();
      await expect(needsApprovalBadge).toBeVisible({ timeout: 5000 });

      // Press Enter in the terminal
      const terminal = page.getByTestId("xterm-container").first();
      await terminal.click();
      await page.keyboard.press("Enter");

      // Badge should disappear within 100ms (AC-2)
      await expect(needsApprovalBadge).toBeHidden({ timeout: 100 });
    }
  );

  test.skip(
    "T-E2E-003: enter-detection_should_restoreApproval_When_stillPendingAfterRefetch",
    async ({ page }) => {
      // TODO: requires approval fixture that stays pending after refetch
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

      const approvalBadge = page.getByTestId("approval-nav-badge");
      await expect(approvalBadge).toBeVisible({ timeout: 5000 });

      // Type a non-Enter key — should NOT clear the approval (AC-5, no false positive)
      const terminal = page.getByTestId("xterm-container").first();
      await terminal.click();
      await page.keyboard.type("a");

      // Approval is still shown immediately (no flicker for non-Enter keystrokes)
      await expect(approvalBadge).toBeVisible();

      // Wait for debounce + refetch (≤500ms); approval still present since backend pending
      await page.waitForTimeout(500);
      await expect(approvalBadge).toBeVisible();
    }
  );

  test.skip(
    "T-E2E-004: enter-detection_should_notClearSessionB_When_userTypesInSessionA",
    async ({ page }) => {
      // TODO: requires two concurrent sessions with pending approvals
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

      // Both sessions have approvals — note IDs from the DOM
      const approvalBadge = page.getByTestId("approval-nav-badge");
      await expect(approvalBadge).toBeVisible({ timeout: 5000 });

      // Navigate to session A terminal and press Enter
      const sessionA = page.getByTestId("session-card").first();
      await sessionA.click();
      const terminal = page.getByTestId("xterm-container").first();
      await terminal.click();
      await page.keyboard.press("Enter");

      // Session A's approval clears within 100ms
      // Session B's approval remains visible in the drawer (AC-4)
      await page.waitForTimeout(200);
      const drawer = page.getByRole("complementary", { name: /pending approvals/i });
      // Drawer should still show session B's approval
      await expect(drawer.getByTestId("approval-card")).toBeVisible();
    }
  );

  test.skip(
    "T-E2E-005: enter-detection_should_haveNoVisibleEffect_When_noActiveNotification",
    async ({ page }) => {
      // TODO: requires a session with NO pending approvals
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

      // Ensure no approval badge is visible
      await expect(page.getByTestId("approval-nav-badge")).toBeHidden({ timeout: 5000 });

      // Press Enter in terminal — should cause no drawer to appear, no flash
      const terminal = page.getByTestId("xterm-container").first();
      await terminal.click();
      await page.keyboard.press("Enter");

      // Still no badge, no drawer (AC-5)
      await expect(page.getByTestId("approval-nav-badge")).toBeHidden();
      const drawer = page.getByRole("complementary", { name: /pending approvals/i });
      await expect(drawer).toBeHidden();
    }
  );

  test.skip(
    "T-E2E-006: enter-detection_should_notRegressApproveDenyButtons_When_featureActive",
    async ({ page }) => {
      // TODO: requires approval fixture
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

      const approvalBadge = page.getByTestId("approval-nav-badge");
      await expect(approvalBadge).toBeVisible({ timeout: 5000 });

      // Open drawer and click Approve
      await approvalBadge.click();
      const approveBtn = page.getByRole("button", { name: /approve/i }).first();
      await approveBtn.click();

      // Approval should resolve correctly (disappear from drawer) without double-clear corruption
      const drawer = page.getByRole("complementary", { name: /pending approvals/i });
      await expect(drawer.getByTestId("approval-card")).toBeHidden({ timeout: 5000 });
    }
  );

  // Smoke test: the page loads without JS errors (always runs, no fixture needed)
  test("enter-detection_smoke_should_loadPageWithoutErrors", async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });

    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    await page.waitForSelector("body", { timeout: 15000 });

    // No JS errors on page load
    expect(consoleErrors.filter(e => !e.includes("favicon"))).toHaveLength(0);
  });
});
