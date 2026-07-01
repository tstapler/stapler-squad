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

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Accessibility (WCAG 2.1 AA)', () => {
  // Axe scans are CPU-heavy; give each test 2 minutes to avoid browser-crash flakes.
  test.setTimeout(120_000);

  test('IT-5.1: Main page has no critical or serious accessibility violations', async ({ page }) => {
    // Disable animations so Axe sees final rendered state, not mid-animation opacity
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.goto(BASE_URL, { waitUntil: 'load' });

    // Wait for network to settle so the app is fully rendered before scanning.
    // This prevents mid-render browser crashes when axe fires under SwiftShader.
    await page.waitForLoadState('networkidle', { timeout: 30_000 });

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
    await page.waitForLoadState('networkidle', { timeout: 30_000 });

    const results = await new AxeBuilder({ page })
      .exclude('pre, [class*="terminal"], [class*="Terminal"]')
      .analyze();

    const criticalViolations = results.violations.filter(
      v => v.impact === 'critical' || v.impact === 'serious',
    );

    expect(criticalViolations).toHaveLength(0);
  });
});
