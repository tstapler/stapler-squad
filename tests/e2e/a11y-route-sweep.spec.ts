// @feature accessibility
// Axe sweep over every statically-routable app page. Fails on serious/critical
// WCAG 2.1 A/AA violations except those listed in KNOWN (a baseline of
// pre-existing debt: only remove entries, never add; some, e.g. color-contrast on
// /sessions/new, depend on render timing so staleness is not asserted).
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

const ROUTES = [
  '/', '/account', '/backlog', '/backlog/board', '/config', '/errors', '/files', '/help',
  '/history', '/insights', '/logs', '/notifications', '/review-queue', '/rules',
  '/settings', '/settings/backlog-sources', '/settings/backlog-stages', '/settings/defaults',
  '/settings/features', '/settings/jules', '/settings/pipeline-modes', '/settings/remotes',
  '/settings/tagging-classifier', '/settings/unfinished', '/triggers', '/unfinished',
  '/workflows', '/sessions/new', '/sessions/import',
];

/** route -> axe rule ids tolerated on it today (pre-existing, filed in audit-findings). */
const KNOWN: Record<string, string[]> = {
  '/account': ['color-contrast'],
  '/backlog': ['document-title'],
  '/backlog/board': ['document-title'],
  '/config': ['color-contrast'],
  '/help': ['color-contrast'],
  '/history': ['color-contrast'],
  '/logs': ['aria-required-children', 'aria-required-parent', 'color-contrast', 'scrollable-region-focusable'],
  '/rules': ['color-contrast', 'scrollable-region-focusable'],
  '/sessions/new': ['color-contrast', 'nested-interactive'],
  '/settings': ['color-contrast'],
  '/settings/backlog-stages': ['color-contrast'],
  '/settings/defaults': ['color-contrast'],
  '/settings/features': ['color-contrast'],
  '/settings/jules': ['color-contrast'],
  '/settings/pipeline-modes': ['color-contrast'],
  '/settings/remotes': ['color-contrast'],
  '/settings/tagging-classifier': ['color-contrast'],
  '/unfinished': ['color-contrast', 'nested-interactive'],
  '/workflows': ['color-contrast'],
};

test.describe('Accessibility route sweep (WCAG 2.1 AA)', () => {
  test.setTimeout(120_000);
  for (const route of ROUTES) {
    test(`no serious/critical violations on ${route}`, async ({ page }) => {
      await page.goto(`${BASE_URL}${route}`, { waitUntil: 'load' });
      await page.locator('body').waitFor();
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
      const tolerated = new Set(KNOWN[route] ?? []);
      const serious = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
      const unexpected = serious.filter((v) => !tolerated.has(v.id));
      expect(
        unexpected.map((v) => `${v.id} (${v.impact}): ${v.nodes.length} node(s) e.g. ${v.nodes[0]?.target.join(' ')}`),
      ).toEqual([]);
    });
  }
});
