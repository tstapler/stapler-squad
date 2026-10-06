import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
const _features = [FEATURE_CATALOG['review-queue-list'], FEATURE_CATALOG['review-queue-acknowledge']] as const;
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
import { dismissOnboardingIfPresent } from './pages/OnboardingPage';

// Base URL falls back to the test server port; playwright.config.ts sets baseURL
const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Review Queue Smoke Tests', () => {
  test('review queue page loads successfully', async ({ page }) => {
    await page.goto(`${BASE_URL}/review-queue`);
    await dismissOnboardingIfPresent(page);
    await page.waitForSelector('[data-testid="review-queue"]', { timeout: 5000 });

    // Verify page elements are present
    await expect(page.locator('[data-testid="review-queue"]')).toBeVisible();
    await expect(page.locator('[data-testid="review-queue"] [data-testid="review-queue-badge"]')).toBeVisible();
  });

  test('review queue badge is visible', async ({ page }) => {
    await page.goto(`${BASE_URL}/review-queue`);
    await dismissOnboardingIfPresent(page);

    const badge = page.locator('[data-testid="review-queue"] [data-testid="review-queue-badge"]');
    await expect(badge).toBeVisible();

    // Badge should show a number (even if 0)
    const text = await badge.textContent();
    expect(text?.trim()).toMatch(/^\d+$/);
  });

  test('review queue panel renders without errors', async ({ page }) => {
    await page.goto(`${BASE_URL}/review-queue`);
    await dismissOnboardingIfPresent(page);
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
  // /sessions/new client-side redirects to `/?new=true`, which opens the
  // Omnibar in discovery mode (OmnibarContext's openOmnibar() with no input
  // defaults to discovery, not creation — only the Ctrl+Shift+K shortcut
  // opens directly into creation mode). Typing a path into the session
  // source input triggers LocalPathDetector, which switches the Omnibar
  // into its creation panel (OmnibarCreationPanel.tsx). The old multi-step
  // SessionWizard these tests originally targeted is @deprecated and no
  // longer rendered — see SessionWizard.tsx's docblock.
  test('session creation opens the omnibar with a session source input', async ({ page }) => {
    await page.goto(`${BASE_URL}/sessions/new`);
    await dismissOnboardingIfPresent(page);

    await expect(page.locator('input[aria-label="Session source input"]')).toBeVisible({ timeout: 10000 });
  });

  test('session creation form has required fields', async ({ page }) => {
    await page.goto(`${BASE_URL}/sessions/new`);
    await dismissOnboardingIfPresent(page);

    // Typing a local path detects it and switches the omnibar from
    // discovery into creation mode, revealing the session-type/name/path fields.
    await page.locator('input[aria-label="Session source input"]').fill('/tmp');

    await expect(page.getByRole('radiogroup', { name: 'Session Type' })).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('textbox', { name: 'Session Name' })).toBeVisible();

    await page.getByRole('radio', { name: 'Existing folder' }).click();
    await expect(page.getByLabel('Working Directory')).toBeVisible();

    // .first(): two "Create Session" buttons render simultaneously — the
    // OmnibarCreationPanel footer submit button and a second, independently
    // wired submit button in Omnibar.tsx's shortcuts bar (both guarded by
    // the same !isDiscoveryMode condition). That's a pre-existing UI
    // duplication, not something this test should mask silently — worth its
    // own follow-up to collapse the two submit paths into one.
    await expect(page.getByRole('button', { name: 'Create Session' }).first()).toBeVisible();

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
    await dismissOnboardingIfPresent(page);
    await page.waitForSelector('[data-testid="review-queue"]', { timeout: 5000 });

    // This sentinel confirms the ReviewQueuePanel rendered without errors and the
    // loading state resolved. Its presence is required for acknowledge tests to proceed.
    await expect(page.locator('[data-testid="review-queue-loaded"]')).toBeAttached({ timeout: 10000 });
  });

  test('when queue has items, each carries acknowledge data-testid', async ({ page }) => {
    // +feature: ui:review-queue
    await page.goto(`${BASE_URL}/review-queue`);
    await dismissOnboardingIfPresent(page);
    await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000, state: 'attached' });

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
    await dismissOnboardingIfPresent(page);
    await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000, state: 'attached' });

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
