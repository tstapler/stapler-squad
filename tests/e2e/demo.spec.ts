import { test, expect } from '@playwright/test';
import * as path from 'path';
import * as fs from 'fs';

/**
 * Demo recording spec — "Mission Control for AI Agents"
 *
 * Narrative:
 *   You have 6 Claude/Aider agents running across your microservices.
 *   Stapler Squad is your mission control: see everything, act on what needs
 *   attention, never lose context.
 *
 * Pre-seeded sessions (6 total):
 *   payment-stripe-integration  Running   Backend   Payments/API/Priority
 *   fix-api-timeout             NeedsApproval Backend Bug/API/Urgent
 *   auth-refactor               Paused    Backend   Auth/Security
 *   dashboard-redesign          Running   Frontend  React/UX/Priority
 *   k8s-autoscaling             Ready     Infrastructure DevOps/Kubernetes
 *   payment-email-notifications Running   Backend   Payments/Notifications (aider)
 */

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const SNAP_DIR = process.env.PLAYWRIGHT_VIDEO_OUTPUT_DIR || '/tmp/demo-video-output';

function snap(page: import('@playwright/test').Page, name: string) {
  fs.mkdirSync(SNAP_DIR, { recursive: true });
  return page.screenshot({ path: path.join(SNAP_DIR, name) });
}

/**
 * Shows a floating scene label in the bottom-left of the viewport.
 * Slides in, holds for `holdMs`, then slides out.
 */
async function showSceneLabel(
  page: import('@playwright/test').Page,
  label: string,
  holdMs = 2000,
) {
  await page.evaluate(({ label }) => {
    const existing = document.getElementById('demo-scene-label');
    if (existing) existing.remove();

    const el = document.createElement('div');
    el.id = 'demo-scene-label';
    el.textContent = label;
    el.style.cssText = `
      position: fixed;
      bottom: 32px;
      left: 32px;
      background: rgba(15, 23, 42, 0.92);
      color: #f8fafc;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
      font-size: 18px;
      font-weight: 600;
      letter-spacing: 0.01em;
      padding: 10px 20px;
      border-radius: 8px;
      border-left: 3px solid #3b82f6;
      box-shadow: 0 4px 24px rgba(0,0,0,0.4);
      pointer-events: none;
      z-index: 2147483645;
      opacity: 0;
      transform: translateY(12px);
      transition: opacity 0.25s ease, transform 0.25s ease;
    `;
    document.body.appendChild(el);
    // Trigger entrance
    requestAnimationFrame(() => {
      el.style.opacity = '1';
      el.style.transform = 'translateY(0)';
    });
  }, { label });

  await page.waitForTimeout(holdMs);

  await page.evaluate(() => {
    const el = document.getElementById('demo-scene-label');
    if (el) {
      el.style.opacity = '0';
      el.style.transform = 'translateY(12px)';
      setTimeout(() => el.remove(), 280);
    }
  });
}

/** Type text character-by-character to produce a real-time filter effect. */
async function humanType(
  locator: import('@playwright/test').Locator,
  text: string,
  charDelayMs = 110,
) {
  await locator.click();
  for (const char of text) {
    await locator.press(char);
    await locator.page().waitForTimeout(charDelayMs);
  }
}

/**
 * Injects a visible cursor dot, click-ripple effect, and notification suppression
 * into every page load via addInitScript (survives navigations).
 * Uses DOMContentLoaded to ensure body exists before appending elements.
 */
async function injectClickHighlighter(page: import('@playwright/test').Page) {
  await page.addInitScript(() => {
    function setup() {
      // Suppress notification toasts so they never appear in the recording.
      const suppressStyle = document.createElement('style');
      suppressStyle.textContent = `
        [class*="NotificationToast"],
        [class*="notification-toast"],
        [class*="notificationToast"] { display: none !important; }
      `;
      (document.head || document.documentElement).appendChild(suppressStyle);

      const style = document.createElement('style');
      style.textContent = `
        #demo-cursor {
          position: fixed;
          width: 28px;
          height: 28px;
          background: rgba(59, 130, 246, 0.80);
          border: 3px solid rgba(255, 255, 255, 0.95);
          border-radius: 50%;
          pointer-events: none;
          z-index: 2147483647;
          transform: translate(-50%, -50%);
          box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.35), 0 3px 10px rgba(0,0,0,0.35);
          transition: background 0.1s ease, transform 0.08s ease;
        }
        #demo-cursor.pressing {
          background: rgba(239, 68, 68, 0.88);
          transform: translate(-50%, -50%) scale(0.85);
        }
        @keyframes demoRipple {
          from { transform: translate(-50%, -50%) scale(0.3); opacity: 0.9; }
          to   { transform: translate(-50%, -50%) scale(2.6); opacity: 0; }
        }
        .demo-ripple {
          position: fixed;
          width: 56px;
          height: 56px;
          border: 3px solid rgba(59, 130, 246, 0.9);
          border-radius: 50%;
          pointer-events: none;
          z-index: 2147483646;
          animation: demoRipple 0.5s ease-out forwards;
        }
      `;
      (document.head || document.documentElement).appendChild(style);

      const cursor = document.createElement('div');
      cursor.id = 'demo-cursor';
      document.body.appendChild(cursor);

      document.addEventListener('mousemove', (e) => {
        cursor.style.left = e.clientX + 'px';
        cursor.style.top  = e.clientY + 'px';
      }, { passive: true });

      document.addEventListener('mousedown', (e) => {
        cursor.classList.add('pressing');
        const ripple = document.createElement('div');
        ripple.className = 'demo-ripple';
        ripple.style.left = e.clientX + 'px';
        ripple.style.top  = e.clientY + 'px';
        document.body.appendChild(ripple);
        setTimeout(() => ripple.remove(), 500);
      }, { passive: true });

      document.addEventListener('mouseup', () => {
        cursor.classList.remove('pressing');
      }, { passive: true });
    }

    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', setup);
    } else {
      setup();
    }
  });
}

test('Demo Flow', async ({ page }) => {

  // Inject cursor + click-ripple visualiser before first navigation.
  await injectClickHighlighter(page);

  // ── Scene 1: Dashboard — show all 6 sessions at once ─────────────────────
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await expect(page).toHaveTitle(/Stapler Squad/);
  await page.waitForSelector('input[placeholder*="Search"]', { timeout: 10000 });
  await page.waitForTimeout(1000);

  // Remove any notification toasts before snapshotting the clean dashboard.
  await page.evaluate(() => {
    document.querySelectorAll('[class*="NotificationToast"]').forEach(el => el.remove());
  });
  await page.waitForTimeout(300);

  await showSceneLabel(page, '🚀 Mission Control — 6 agents, one dashboard');
  await snap(page, '01-dashboard.png');
  await page.waitForTimeout(1000);

  // ── Scene 2: Search — "payment" filters 6 → 2 in real time ───────────────
  const searchInput = page.locator('input[placeholder*="Search"]').first();
  await humanType(searchInput, 'payment');
  await page.waitForTimeout(800);
  await showSceneLabel(page, '🔍 Instant search across all sessions');
  await snap(page, '02-search-payment.png');
  await page.waitForTimeout(400);

  // Clear search to restore all sessions.
  await searchInput.clear();
  await page.waitForTimeout(700);

  // ── Scene 3: Filter to "Needs Approval" — triage workflow ────────────────
  const statusFilter = page.locator('select[aria-label="Filter by status"]').first();
  if (await statusFilter.isVisible({ timeout: 2000 })) {
    // Use the option value from the select — inspect the actual value in the DOM.
    // Status values: NeedsApproval = 4 (see session/instance.go).
    // The select likely uses numeric string values matching the proto Status enum.
    // Try label first; fall back to index if not found.
    // SessionStatus.NEEDS_APPROVAL = 5 in the proto enum (see types_pb.ts).
    await statusFilter.selectOption({ value: '5' });
    await page.waitForTimeout(800);
    await showSceneLabel(page, '⚠️  Filter by status — spot agents that need attention');
    await snap(page, '03-needs-approval.png');
    await page.waitForTimeout(400);

    // Reset filter.
    await statusFilter.selectOption({ index: 0 });
    await page.waitForTimeout(500);
  }

  // ── Scene 4: Group by Tag — multi-dimensional organization ────────────────
  const groupBy = page.locator('select[aria-label="Group sessions by"]').first();
  if (await groupBy.isVisible({ timeout: 2000 })) {
    await groupBy.selectOption({ value: 'tag' });   // GroupingStrategy.Tag = "tag"
    await page.waitForTimeout(1000);
    await showSceneLabel(page, '🏷️  Group by tag — multi-dimensional organization');
    await snap(page, '04-group-by-tag.png');
    await page.waitForTimeout(500);

    // Reset to category.
    await groupBy.selectOption({ value: 'category' });
    await page.waitForTimeout(600);
  }

  // ── Scene 5: Review Queue — structured triage ─────────────────────────────
  await page.locator('a[href*="review-queue"]').first().click();
  await page.waitForTimeout(1200);
  await showSceneLabel(page, '📋 Review Queue — structured agent triage');
  await snap(page, '05-review-queue.png');
  await page.waitForTimeout(800);

  // ── Scene 6: Back to sessions — final hero shot ───────────────────────────
  await page.locator('a[href="/"]').first().click();
  await page.waitForTimeout(1000);
  await showSceneLabel(page, '✨ Stapler Squad — mission control for AI agents');
  await snap(page, '06-final.png');
  await page.waitForTimeout(2500); // Clean ending frame for the video loop.
});
