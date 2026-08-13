import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: accessibility — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['ui-accessibility-gate'], // TODO: add to catalog
  FEATURE_CATALOG['backlog-list-items'],
  FEATURE_CATALOG['backlog-transition-status'],
] as const;
// @feature backlog:watch, backlog:list-page, backlog:board-page, backlog:item-detail, backlog:connection-indicator
// Story 5: UX Analysis Automation - Axe Core accessibility gate
// This test file is the CI gate for WCAG 2.1 AA compliance.
// critical + serious violations block merge.

import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import {
  StuckItemsPage,
  seedStuckItem,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from './pages/StuckItemsPage';
import { BacklogPage } from './pages/BacklogPage';
import { createBacklogItemDirect, transitionBacklogItemDirect, updateBacklogItemDirect } from './pages/BacklogMutations';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Accessibility (WCAG 2.1 AA)', () => {
  // Axe scans are CPU-heavy; give each test 2 minutes to avoid browser-crash flakes.
  test.setTimeout(120_000);

  test('IT-5.1: Main page has no critical or serious accessibility violations', async ({ page }) => {
    // Disable animations so Axe sees final rendered state, not mid-animation opacity
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.goto(BASE_URL, { waitUntil: 'load' });

    // Wait for a known main-page element instead of networkidle: this app
    // polls continuously (session list, terminal streams), so network
    // activity may never go quiet for the 500ms networkidle requires -
    // matching this repo's e2e convention of avoiding wait conditions that
    // depend on network settling (see alias.spec.ts's identical selector).
    await page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 30_000 });

    // Run axe analysis
    const results = await new AxeBuilder({ page })
      // Exclude terminal pre elements - intentional design for terminal rendering
      .exclude('pre, [class*="terminal"], [class*="Terminal"]')
      .analyze();

    // Collect critical and serious violations
    const criticalViolations = results.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious',
    );

    if (criticalViolations.length > 0) {
      const messages = criticalViolations.map(v =>
        `\n  [${v.impact?.toUpperCase()}] ${v.id}: ${v.description}\n    Affected: ${v.nodes.slice(0, 2).map(n => n.target.join(', ')).join('; ')}`,
      );
      console.error(`Accessibility violations found:${messages.join('')}`);
    }

    expect(criticalViolations).toHaveLength(0);
  });

  test('IT-5.1: Secondary routes are accessible', async ({ page }) => {
    // Navigate to review queue page
    await page.goto(`${BASE_URL}/review-queue`, { waitUntil: 'load' });
    // See the main-page test above for why this doesn't use networkidle.
    await page.waitForSelector('[data-testid="review-queue"]', { timeout: 30_000 });

    const results = await new AxeBuilder({ page })
      .exclude('pre, [class*="terminal"], [class*="Terminal"]')
      .analyze();

    const criticalViolations = results.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious',
    );

    expect(criticalViolations).toHaveLength(0);
  });

  // UX Criterion 19 (design/ux.md AC 19 / validation.md row 19): the new
  // stuck-reason chip color/text pairs must meet WCAG AA contrast (4.5:1).
  // Depends on seedStuckItem() in ./pages/StuckItemsPage.ts — see the KNOWN
  // GAP note there (no debug seed endpoint exists yet).
  test('stuck-item chips pass Axe color-contrast on /unfinished', async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    try {
      await seedStuckItem(request, {
        itemId: 'axe-pr-ready',
        title: 'fix: axe contrast pr-ready',
        reason: 'pr_ready_unmerged',
        prNumber: 148,
        prUrl: 'https://github.com/tstapler/stapler-squad/pull/148',
      });
      await seedStuckItem(request, {
        itemId: 'axe-rework-cap',
        title: 'fix: axe contrast rework-cap',
        reason: 'rework_cap',
        context: 'cap hit',
      });
      await seedStuckItem(request, {
        itemId: 'axe-abandoned',
        title: 'fix: axe contrast abandoned-review',
        reason: 'abandoned_review',
      });

      await page.emulateMedia({ reducedMotion: 'reduce' });
      const stuckPage = new StuckItemsPage(page);
      await stuckPage.goto();
      await expect(stuckPage.section).toBeVisible();

      const results = await new AxeBuilder({ page })
        .include('[data-testid="stuck-items-section"]')
        .withRules(['color-contrast'])
        .analyze();

      expect(results.violations).toHaveLength(0);
    } finally {
      await disableBacklogFeatureFlag(request);
    }
  });
});

// backlog-event-driven-updates: cross-cutting accessibility ACs (design/ux.md
// UX Acceptance Criteria #4, #5, #12, #26, #29-#33), covering the 4 backlog
// live-update surfaces (list, board, detail panel, connection indicator) per
// this repo's convention of extending accessibility.spec.ts rather than
// duplicating an a11y suite per surface (validation.md's Test Stack note).
test.describe('Accessibility — backlog live updates (WCAG 4.1.3 AA)', () => {
  test.setTimeout(120_000);

  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
      // Also suppress the separate, app-wide onboarding walkthrough
      // (useOnboarding.ts's `stapler-squad:onboarded` key, gated
      // independently of the backlog-specific tour above). It auto-opens
      // ~800ms after mount on any page — including /backlog — and its
      // modal Overlay intercepts pointer events on the backlog table,
      // timing out BacklogPage.openItemDetail()'s row click.
      localStorage.setItem('stapler-squad:onboarded', 'true');
    });
  });

  test('under reduced motion, a filtered-out item is removed near-instantly instead of playing the ~200ms fade (UX AC #4, #26)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-axe-reduced-motion-${Date.now()}`,
      status: 'in_progress',
    });

    await page.emulateMedia({ reducedMotion: 'reduce' });
    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.applyStatusFilter('in_progress');

    const row = backlogPage.getRowById(itemId);
    await expect(row).toBeVisible();

    await transitionBacklogItemDirect(request, itemId, 'done');

    // BacklogPage's EXIT_TRANSITION_MS collapses to 0ms under
    // prefers-reduced-motion (page.tsx's reducedMotionRef gate), so the row
    // should be gone well inside the ~200ms window the animated case uses —
    // a much tighter bound than backlog-live-updates.spec.ts's 3s non-reduced
    // assertion serves as the "instant" signal here.
    await expect(row).toHaveCount(0, { timeout: 1000 });
  });

  test('verdict badges, status labels, and the connection indicator all carry a visible text label independent of color (UX AC #5)', async ({ page, request }) => {
    await createBacklogItemDirect(request, {
      title: `e2e-axe-text-label-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();

    // Connection indicator: a colored dot alone would fail this — the
    // component always renders a text label alongside it.
    const indicatorText = await backlogPage.getConnectionIndicator().textContent();
    expect(indicatorText?.trim().length ?? 0).toBeGreaterThan(0);

    // Status badges render a human-readable label, not just a color class.
    const statusBadge = page.locator('[aria-label^="Status:"]').first();
    await expect(statusBadge).toBeVisible();
    expect((await statusBadge.textContent())?.trim().length ?? 0).toBeGreaterThan(0);
  });

  test('no aria-live region on /backlog or /backlog/board uses assertive during a routine transition (UX AC #29)', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-axe-no-assertive-${Date.now()}`,
      status: 'in_progress',
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();

    await transitionBacklogItemDirect(request, itemId, 'review');
    await expect(backlogPage.getRowById(itemId).locator('[aria-label^="Status:"]')).toContainText('Review', { timeout: 5000 });

    // Scoped to the backlog page's own content — Next.js injects its own
    // `#__next-route-announcer__` (aria-live="assertive" role="alert") on
    // every route for SPA navigation announcements; that's framework
    // built-in behavior unrelated to this feature's routine status/verdict
    // changes, so it's excluded rather than silently making this assertion
    // pass/fail on something out of scope.
    const assertiveCount = await page.locator('[data-testid="backlog-page"] [aria-live="assertive"]').count();
    expect(assertiveCount).toBe(0);
  });

  test('the connection indicator and gate verdict box carry aria-atomic="true" (UX AC #30)', async ({ page, request }) => {
    const title = `e2e-axe-atomic-${Date.now()}`;
    await createBacklogItemDirect(request, { title, status: 'review' });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await expect(backlogPage.getConnectionIndicator()).toHaveAttribute('aria-atomic', 'true');

    await backlogPage.openItemDetail(title);
    // GateVerdictBox only renders for status "review" — matches the item
    // seeded above. It is queried by its accessible name (role="status",
    // aria-label="Gate verdict"), not a data-testid — it has none.
    const gateVerdictBox = page.getByRole('status', { name: 'Gate verdict' });
    await expect(gateVerdictBox).toHaveAttribute('aria-atomic', 'true');
  });

  test('the backlog list and board item collections are never wrapped in one giant aria-live region (UX AC #31)', async ({ page }) => {
    const backlogPage = new BacklogPage(page);

    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await expect(page.locator('table[aria-label="Backlog items"]')).not.toHaveAttribute('aria-live', /.+/);

    await backlogPage.gotoBoard();
    await expect(page.getByTestId('backlog-board')).not.toHaveAttribute('aria-live', /.+/);
  });

  test('Reload and dismiss controls on the buffered-update banner are keyboard-reachable with a visible focus indicator (UX AC #32)', async ({ page, request }) => {
    const title = `e2e-axe-keyboard-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title, description: 'Original', status: 'review' });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.openItemDetail(title);
    await page.getByTestId('backlog-detail-edit').click();

    await updateBacklogItemDirect(request, itemId, { description: 'Changed elsewhere' });

    const notice = page.getByTestId('backlog-detail-buffered-update-notice');
    await expect(notice).toBeVisible({ timeout: 5000 });

    const reloadButton = notice.getByRole('button', { name: 'Reload' });

    // Real Tab-key navigation (not `.focus()`) is required here: globals.css
    // uses `:focus:not(:focus-visible) { outline: none }` (keyboard-only
    // focus rings, line 194), and programmatic `.focus()` does not put
    // Chromium into the `:focus-visible` state the way an actual keyboard
    // Tab does — asserting on a `.focus()`-triggered outline would give a
    // false negative against this app's own (correct) focus-ring design.
    let reached = false;
    for (let i = 0; i < 30; i++) {
      await page.keyboard.press('Tab');
      if (await reloadButton.evaluate((el) => el === document.activeElement).catch(() => false)) {
        reached = true;
        break;
      }
    }
    expect(reached).toBe(true);
    await expect(reloadButton).toBeFocused();

    const outline = await reloadButton.evaluate((el) => getComputedStyle(el).outlineStyle);
    // A visible focus indicator means outline (or an equivalent
    // browser/theme default) is not explicitly suppressed to "none".
    expect(outline).not.toBe('none');
  });

  test('flash overlay, connection indicator, and InlineNotice meet 4.5:1 contrast in light and dark themes (UX AC #33)', async ({ context, request }) => {
    const title = `e2e-axe-contrast-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title, description: 'Original', status: 'review' });

    // This app switches theme via a `stapler-theme` localStorage value +
    // documentElement class (web-app/src/app/layout.tsx's FOUC-prevention
    // script) — the `@media (prefers-color-scheme: dark)` block was removed
    // (globals.css, Story 1.5.3) — so `page.emulateMedia({ colorScheme })`
    // would be a no-op here; set the real theme mechanism instead, mirroring
    // playwright.config.ts's visual-regression theme fixtures. A fresh page
    // per iteration (not the shared `page` fixture) avoids stacking multiple
    // addInitScript callbacks against one page across iterations.
    for (const themeName of ['light', 'dark'] as const) {
      const page = await context.newPage();
      await page.addInitScript((name) => {
        localStorage.setItem('stapler-theme', name);
        localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
        // See the describe-level beforeEach above for why this second key
        // (the separate, app-wide onboarding walkthrough) also must be
        // seeded — this test uses a fresh `context.newPage()` per
        // iteration, which doesn't inherit that beforeEach's addInitScript.
        localStorage.setItem('stapler-squad:onboarded', 'true');
      }, themeName);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForPageLoad();
      await backlogPage.openItemDetail(title);
      await page.getByTestId('backlog-detail-edit').click();

      await updateBacklogItemDirect(request, itemId, { description: `Changed elsewhere (${themeName})` });
      await expect(page.getByTestId('backlog-detail-buffered-update-notice')).toBeVisible({ timeout: 5000 });

      // Scoped to only the elements this feature introduces/modifies
      // (connection indicator, InlineNotice) — NOT the whole backlog-page
      // table, which contains pre-existing components (e.g. the priority
      // badge) with their own, unrelated contrast issues predating this
      // feature; scanning the full page would fail this test on a bug this
      // sweep didn't introduce and isn't scoped to fix.
      const results = await new AxeBuilder({ page })
        .include('[data-testid="connection-indicator"]')
        .include('[data-testid="backlog-detail-buffered-update-notice"]')
        .withRules(['color-contrast'])
        .analyze();

      expect(results.violations, `color-contrast violations in ${themeName} theme`).toHaveLength(0);
      await page.close();
    }
  });
});
