// @feature accessibility
// Axe sweep over every statically-routable app page. Fails on serious/critical
// WCAG 2.2 A/AA violations except those listed in KNOWN (a baseline of
// pre-existing debt: remove entries freely; add one only for a violation that also occurs on the
// Next build, with a linked issue (Story 7.8a); some, e.g. color-contrast on
// /sessions/new, depend on render timing so staleness is not asserted).
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import axe from 'axe-core';
import { ROUTE_META } from '../../web-app/src/app/route-meta';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();

/** Every page.tsx route, derived so a new route cannot escape the sweep (Story 7.8a). */
const ROUTES = Object.keys(ROUTE_META);

export const AXE_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

/** route -> axe rule ids tolerated on it today (pre-existing, filed in audit-findings). */
// Entries marked #1112 were added by the 7.8a route extension; all occur on the Next build.
const KNOWN: Record<string, string[]> = {
  '/changelog': ['color-contrast', 'definition-list'], // #1112
  '/account': ['color-contrast'],
  '/help': ['color-contrast'],
  '/history': ['color-contrast'],
  '/logs': ['aria-required-children', 'aria-required-parent', 'color-contrast', 'scrollable-region-focusable'],
  '/rules': ['color-contrast', 'scrollable-region-focusable'],
  '/sessions/new': ['color-contrast', 'nested-interactive'],
  // Throwaway files-viewer Phase 0 spike pages (hostile CodeMirror attributes); red on origin/main too.
  '/spike-files': ['aria-input-field-name', 'scrollable-region-focusable'],
  '/spike-lazydoc': ['aria-input-field-name', 'scrollable-region-focusable'],
  '/settings/jules': ['color-contrast'],
  '/settings/remotes': ['color-contrast'],
  '/settings/tagging-classifier': ['color-contrast'],
  '/unfinished': ['color-contrast', 'nested-interactive'],
  '/workflows': ['color-contrast'],
};

/** Optional: sweep under one theme (stapler-theme key) instead of the default. */
const THEME = process.env.A11Y_SWEEP_THEME;

test.describe('Accessibility route sweep (WCAG 2.2 AA)', () => {
  test('sweep config keeps the WCAG 2.2 AA tag and the installed axe-core ships it', () => {
    expect(AXE_TAGS).toContain('wcag22aa');
    expect(axe.getRules(['wcag22aa']).length).toBeGreaterThan(0);
  });

  test.setTimeout(120_000);
  for (const route of ROUTES) {
    test(`no serious/critical violations on ${route}`, async ({ page }) => {
      if (THEME) await page.addInitScript((t) => localStorage.setItem('stapler-theme', t), THEME);
      await page.goto(`${BASE_URL}${route}`, { waitUntil: 'load' });
      await page.locator('body').waitFor();
      const results = await new AxeBuilder({ page }).withTags(AXE_TAGS).analyze();
      const tolerated = new Set(KNOWN[route] ?? []);
      const serious = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
      const unexpected = serious.filter((v) => !tolerated.has(v.id));
      expect(
        unexpected.map((v) => `${v.id} (${v.impact}): ${v.nodes.length} node(s) e.g. ${v.nodes[0]?.target.join(' ')}`),
      ).toEqual([]);
    });
  }
});
