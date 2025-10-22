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

// Base URL is configured in playwright.config.ts to use the test server (port 8544)
const BASE_URL = process.env.BASE_URL || 'http://localhost:8544';

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

// SKIPPED TESTS - Require backend session creation infrastructure
test.describe.skip('Advanced Review Queue Tests (Skipped)', () => {
  test('queue updates immediately on terminal input', async () => {
    // SKIPPED: Requires actual session creation, tmux, and program execution
  });

  test('keyboard navigation with [ and ] keys', async () => {
    // SKIPPED: Requires sessions in review queue
  });

  test('optimistic UI updates on acknowledgment', async () => {
    // SKIPPED: Requires sessions to acknowledge
  });

  test('WebSocket real-time updates', async () => {
    // SKIPPED: Requires active sessions generating events
  });
});
