/**
 * End-to-end tests for review queue functionality
 *
 * NOTE: Advanced tests are skipped because they require complex backend setup.
 * Session creation requires tmux sessions, git worktrees, and program execution
 * which is not suitable for E2E testing without mock infrastructure.
 *
 * Current tests focus on UI smoke testing.
 *
 * Prerequisites:
 * - Test server started automatically by global-setup.ts on port 8544
 * - Test server uses isolated data directory (not production data)
 */

import { test, expect } from '@playwright/test';

// Base URL falls back to the production server port; playwright.config.ts sets baseURL
const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8543';

test.describe('Review Queue Smoke Tests', () => {
  test('review queue page loads successfully', async ({ page }) => {
    await page.goto(`${BASE_URL}/review-queue`);
    await page.waitForSelector('[data-testid="review-queue"]', { timeout: 5000 });

    // Verify page elements are present
    await expect(page.locator('[data-testid="review-queue"]')).toBeVisible();
    await expect(page.locator('[data-testid="review-queue-badge"]')).toBeVisible();
  });

  test('review queue badge is visible', async ({ page }) => {
    await page.goto(`${BASE_URL}/review-queue`);

    const badge = page.locator('[data-testid="review-queue-badge"]');
    await expect(badge).toBeVisible();

    // Badge should show a number (even if 0)
    const text = await badge.textContent();
    expect(text).toMatch(/^\d+$/);
  });

  test('review queue panel renders without errors', async ({ page }) => {
    await page.goto(`${BASE_URL}/review-queue`);
    await page.waitForSelector('[data-testid="review-queue"]', { timeout: 5000 });

    // Verify the review queue panel is fully rendered
    const reviewQueue = page.locator('[data-testid="review-queue"]');
    await expect(reviewQueue).toBeVisible();

    // Should have at least the empty state or session items
    const hasContent = await page.locator('[data-testid="review-queue"] > *').count();
    expect(hasContent).toBeGreaterThan(0);
  });
});

test.describe('Session Creation Flow (UI Only)', () => {
  test('session creation wizard has all steps', async ({ page }) => {
    await page.goto(`${BASE_URL}/sessions/new`);

    // Verify wizard steps are present (using more specific selectors to avoid multiple matches)
    await expect(page.locator('.Wizard_stepLabel__dIAKY', { hasText: 'Basic Info' })).toBeVisible();
    await expect(page.locator('.Wizard_stepLabel__dIAKY', { hasText: 'Repository' })).toBeVisible();
    await expect(page.locator('.Wizard_stepLabel__dIAKY', { hasText: 'Configuration' })).toBeVisible();
    await expect(page.locator('.Wizard_stepLabel__dIAKY', { hasText: 'Review' })).toBeVisible();
  });

  test('session creation form has required test IDs', async ({ page }) => {
    await page.goto(`${BASE_URL}/sessions/new`);

    // Step 1: Basic Info
    await expect(page.locator('[data-testid="session-title"]')).toBeVisible();

    // Navigate to step 2
    await page.fill('[data-testid="session-title"]', 'test-session');
    await page.click('button:has-text("Next")');

    // Step 2: Repository
    await expect(page.locator('[data-testid="session-path"]')).toBeVisible();

    // Navigate to step 3
    await page.fill('[data-testid="session-path"]', '/tmp');
    await page.click('button:has-text("Next")');

    // Step 3: Configuration
    await expect(page.locator('[data-testid="auto-yes-checkbox"]')).toBeVisible();

    // Navigate to step 4
    await page.click('button:has-text("Next")');

    // Step 4: Review
    await expect(page.locator('[data-testid="create-session-button"]')).toBeVisible();

    // Note: We don't actually create the session as it requires backend setup
  });
});

/**
 * Verify the review queue acknowledge flow structure.
 *
 * These tests run against a live test server and verify that:
 * 1. All acknowledge-related UI elements carry the correct data-testid attributes
 * 2. When the queue is non-empty, the Skip button can be activated and the item disappears
 *
 * Tests that require real sessions (tmux + active Claude process) remain in the skipped block.
 */
test.describe('Review Queue Acknowledge Flow — UI Contract', () => {
  test('review-queue-loaded sentinel is present after page renders', async ({ page }) => {
    // +feature: ui:review-queue
    await page.goto(`${BASE_URL}/review-queue`);
    await page.waitForSelector('[data-testid="review-queue"]', { timeout: 5000 });

    // This sentinel confirms the ReviewQueuePanel rendered without errors and the
    // loading state resolved. Its presence is required for acknowledge tests to proceed.
    await expect(page.locator('[data-testid="review-queue-loaded"]')).toBeAttached({ timeout: 10000 });
  });

  test('when queue has items, each carries acknowledge data-testid', async ({ page }) => {
    // +feature: ui:review-queue
    await page.goto(`${BASE_URL}/review-queue`);
    await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000 });

    const items = await page.locator('[data-testid^="review-item-"]').all();

    // If there happen to be items in the test server queue, verify each carries
    // the correct acknowledge button data-testid so selectors are stable.
    for (const item of items) {
      const sessionId = (await item.getAttribute('data-testid'))?.replace('review-item-', '') ?? '';
      expect(sessionId).toBeTruthy();

      // Each non-approval item must have an acknowledge button
      const ackButton = page.locator(`[data-testid="acknowledge-${sessionId}"]`);
      const approveButton = page.locator(`[data-testid="approve-${sessionId}"]`);

      // At least one of acknowledge (skip) or approve button must be present
      const ackCount = await ackButton.count();
      const approveCount = await approveButton.count();
      expect(ackCount + approveCount).toBeGreaterThan(0);
    }
  });

  test('acknowledge button removes item from DOM (optimistic UI)', async ({ page }) => {
    // +feature: ui:review-queue
    await page.goto(`${BASE_URL}/review-queue`);
    await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000 });

    const items = await page.locator('[data-testid^="review-item-"]').all();

    if (items.length === 0) {
      test.skip(); // No sessions in test queue — skip rather than fail
    }

    // Pick the first item that has an acknowledge (Skip) button
    let targetSessionId: string | null = null;
    for (const item of items) {
      const sessionId = (await item.getAttribute('data-testid'))?.replace('review-item-', '') ?? '';
      const ackButton = page.locator(`[data-testid="acknowledge-${sessionId}"]`);
      if (await ackButton.count() > 0) {
        targetSessionId = sessionId;
        break;
      }
    }

    if (!targetSessionId) {
      test.skip(); // Only approval items exist — skip
    }

    const beforeCount = await page.locator('[data-testid^="review-item-"]').count();

    await page.click(`[data-testid="acknowledge-${targetSessionId}"]`);

    // Optimistic removal: item should disappear from DOM without a page reload
    await expect(page.locator(`[data-testid="review-item-${targetSessionId}"]`)).not.toBeAttached({ timeout: 3000 });

    // Queue should have one fewer item
    const afterCount = await page.locator('[data-testid^="review-item-"]').count();
    expect(afterCount).toBe(beforeCount - 1);
  });
});

// SKIPPED TESTS - Require backend session creation infrastructure
test.describe.skip('Advanced Review Queue Tests (Require Backend)', () => {
  test('queue updates immediately on terminal input', async () => {
    // SKIPPED: Requires actual session creation, tmux, and program execution
  });

  test('keyboard navigation with [ and ] keys', async () => {
    // SKIPPED: Requires sessions in review queue
  });

  test('WebSocket real-time updates', async () => {
    // SKIPPED: Requires active sessions generating events
  });
});
