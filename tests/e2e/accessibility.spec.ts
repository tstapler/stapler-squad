import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: accessibility — mapped from @feature annotation
const _features = [
  // FEATURE_CATALOG['ui-accessibility-gate'], // TODO: add to catalog
] as const;
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
