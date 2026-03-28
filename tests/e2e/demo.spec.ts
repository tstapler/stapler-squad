import { test, expect } from '@playwright/test';
import * as path from 'path';
import * as fs from 'fs';

/**
 * Demo recording spec — "Mission Control for AI Agents"
 *
 * Narrative:
 *   You have 6 Claude/Aider agents running across your microservices.
 *   Stapler Squad is your mission control: see everything, act on what needs
 *   attention, automate the predictable, and never lose context.
 *
 * Pre-seeded sessions (6 total):
 *   payment-stripe-integration  Paused        Backend        Payments/API/Priority
 *   fix-api-timeout             NeedsApproval Backend        Bug/API/Urgent
 *   auth-refactor               Paused        Backend        Auth/Security
 *   dashboard-redesign          Paused        Frontend       React/UX/Priority
 *   k8s-autoscaling             Ready         Infrastructure DevOps/Kubernetes
 *   payment-email-notifications Paused        Backend        Payments/Notifications (aider)
 *
 * Note: No Running sessions — poller reclassifies Running sessions without
 * a backing tmux process. Paused/Ready/NeedsApproval are stable in test mode.
 *
 * Scenes:
 *   01 - Dashboard             overview of all 6 agents
 *   02 - Search                "payment" filters 6 → 2 in real time
 *   03 - NeedsApproval filter  triage workflow
 *   04 - Group by Tag          multi-dimensional organisation
 *   05 - Review Queue          structured triage
 *   06 - Rules page            built-in auto-approval rules
 *   07 - Add custom rule       creating a new "Allow git log" rule
 *   08 - Final hero            back to sessions dashboard
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
  // Wait for the session list search input specifically (not the notification panel search).
  await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });
  await page.waitForTimeout(1000);

  // Remove any notification toasts before snapshotting the clean dashboard.
  await page.evaluate(() => {
    document.querySelectorAll('[class*="NotificationToast"]').forEach(el => el.remove());
  });
  await page.waitForTimeout(300);

  // Park the cursor in the bottom-right so it doesn't appear as an artifact
  // floating mid-content on the opening static shot.
  await page.mouse.move(1380, 860);
  await page.waitForTimeout(150);

  await showSceneLabel(page, '🚀 Mission Control — 6 agents, one dashboard', 2500);
  await snap(page, '01-dashboard.png');
  await page.waitForTimeout(1000);

  // ── Scene 2: Search — "payment" filters 6 → 2 in real time ───────────────
  const searchInput = page.locator('input[aria-label="Search sessions"]').first();
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

  // ── Scene 6: Rules page — auto-approval rules ────────────────────────────
  await page.locator('a[href*="rules"]').first().click();
  await page.waitForTimeout(1500);
  // Switch to the Built-in tab to show the pre-seeded protective rules.
  const builtInTab = page.getByRole('button', { name: /built.in/i }).first();
  if (await builtInTab.isVisible({ timeout: 3000 })) {
    await builtInTab.click();
    await page.waitForTimeout(700);
  }
  // Curate the display: show only the first 8 rules to keep the scene readable.
  // The 42-rule table is too dense at demo viewport size — 8 rows communicate
  // the system clearly without overwhelming the viewer.
  await page.evaluate(() => {
    const tables = document.querySelectorAll('table');
    tables.forEach(table => {
      const rows = table.querySelectorAll('tbody tr');
      rows.forEach((row, i) => {
        if (i >= 8) (row as HTMLElement).style.display = 'none';
      });
    });
    // Also handle list-based rule items in case the component isn't a table.
    const listItems = document.querySelectorAll('[class*="rule"][class*="row"], [class*="ruleRow"], [class*="RuleRow"]');
    listItems.forEach((item, i) => {
      if (i >= 8) (item as HTMLElement).style.display = 'none';
    });
  });
  await page.waitForTimeout(300);
  await showSceneLabel(page, '🛡️ Auto-approval rules — automate the predictable');
  await snap(page, '06-rules.png');
  await page.waitForTimeout(1000);

  // ── Scene 7: Add a custom rule ────────────────────────────────────────────
  // Switch back to All / Custom tab and open the new-rule form.
  const allTab = page.getByRole('button', { name: /^all/i }).first();
  if (await allTab.isVisible({ timeout: 2000 })) {
    await allTab.click();
    await page.waitForTimeout(400);
  }
  const addRuleButton = page.getByRole('button', { name: /add custom rule/i }).first();
  if (await addRuleButton.isVisible({ timeout: 3000 })) {
    await addRuleButton.click();
    await page.waitForTimeout(600);

    // Scroll the form into the center of the viewport so the command pattern
    // field — the hero detail of the scene — is clearly visible and not
    // competing with the rules table above or analytics section below.
    const ruleForm = page.locator('form, [class*="add"][class*="rule"], [class*="addRule"], [class*="AddRule"]').last();
    if (await ruleForm.isVisible({ timeout: 2000 })) {
      await ruleForm.scrollIntoViewIfNeeded();
      await page.waitForTimeout(300);
    }

    // Fill in the form to show a realistic "Allow git log" rule.
    const nameInput = page.locator('input[placeholder*="Allow git log"]').first();
    if (await nameInput.isVisible({ timeout: 2000 })) {
      await humanType(nameInput, 'Allow git log', 90);
    }

    const toolInput = page.locator('input[placeholder*="Bash"]').first();
    if (await toolInput.isVisible({ timeout: 2000 })) {
      await humanType(toolInput, 'Bash', 90);
    }

    // Use "^git log" (with caret) to distinguish from the Name field whose
    // placeholder also contains "git log" but without the leading caret.
    const cmdInput = page.locator('input[placeholder*="^git log"]').first();
    if (await cmdInput.isVisible({ timeout: 2000 })) {
      await humanType(cmdInput, '^git log', 90);
    }

    await page.waitForTimeout(600);
    await showSceneLabel(page, '⚙️ Custom rules — teach Claude what\'s always safe');
    await snap(page, '07-add-rule.png');
    await page.waitForTimeout(1000);

    // Cancel — don't persist in the demo.
    const cancelButton = page.getByRole('button', { name: /cancel/i }).first();
    if (await cancelButton.isVisible({ timeout: 2000 })) {
      await cancelButton.click();
      await page.waitForTimeout(400);
    }
  }

  // ── Scene 8: Back to sessions — final hero shot ───────────────────────────
  // This scene completes the narrative loop: we started with a pending approval
  // (fix-api-timeout). Now we show it resolved — injecting a DOM update that
  // flips the "NEEDS APPROVAL" badge to "READY" to signal work was handled.
  await page.locator('a[href="/"]').first().click();
  await page.waitForTimeout(1000);

  // Scroll down enough to reveal the fix-api-timeout card (3rd in Backend
  // group) so the badge flip is visible in the final frame.
  await page.evaluate(() => window.scrollBy({ top: 350, behavior: 'smooth' }));
  await page.waitForTimeout(600);

  // Flip any "NEEDS APPROVAL" badge to "READY" to close the narrative arc —
  // the approval was reviewed, the agent is unblocked.
  await page.evaluate(() => {
    document.querySelectorAll('*').forEach(el => {
      if (el.children.length === 0 && el.textContent?.trim().toUpperCase().includes('NEEDS APPROVAL')) {
        const badge = el as HTMLElement;
        badge.textContent = 'READY';
        badge.style.setProperty('color', '#16a34a', 'important');
        badge.style.setProperty('background', '#dcfce7', 'important');
        badge.style.setProperty('border-color', '#22c55e', 'important');
      }
    });
  });
  await page.waitForTimeout(400);

  // Hover the (now-READY) fix-api-timeout card to signal active interaction.
  const urgentCard = page.locator('[class*="card"], [class*="Card"], [class*="session"]').nth(2);
  if (await urgentCard.isVisible({ timeout: 2000 })) {
    await urgentCard.hover();
    await page.waitForTimeout(300);
  }

  await showSceneLabel(page, '✨ Stapler Squad — every agent tracked, every approval handled', 3000);
  await snap(page, '08-final.png');
  await page.waitForTimeout(2500); // Clean ending frame for the video loop.
});
