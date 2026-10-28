// @feature a11y:window-switch, window:keep-alive, accessibility
/**
 * Axe sweep over the responsiveness states (UX criterion 31). Covers `/` with `window_keep_alive` ON and three
 * retained windows. The Backlog states (warm/refreshing/out-of-date/cold) and criteria 29-30 (Retry / Go to
 * Sessions controls, status-text contrast) need Stories 6.1.x UI, which does not exist yet: add them here
 * when it lands.
 */
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { WindowTabStripPage } from './pages/WindowTabStripPage';
import { flagEnabled, setFlag } from './helpers/feature-flag';

const FLAG = 'window_keep_alive';
const AXE_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

let flagBefore = false;

test.beforeAll(async ({ request }) => {
  flagBefore = await flagEnabled(request, FLAG);
  await setFlag(request, FLAG, true);
});

test.afterAll(async ({ request }) => {
  await setFlag(request, FLAG, flagBefore);
});

test.beforeEach(async ({ context }) => {
  await context.addInitScript(() => localStorage.setItem('stapler-squad:onboarded', 'true'));
});

test('axeSweep_should_ReportNoWcagAaViolations_When_3RetainedWindows', async ({ page }) => {
  await page.goto('/');
  const strip = new WindowTabStripPage(page);
  await strip.waitForLoaded();
  await strip.createWindow();
  await strip.createWindow();
  await expect(strip.tabs).toHaveCount(3);
  await expect(page.getByTestId('window-panel')).toHaveCount(3);

  const results = await new AxeBuilder({ page }).withTags(AXE_TAGS).analyze();
  const serious = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
  expect(
    serious.map((v) => `${v.id} (${v.impact}): ${v.nodes.length} node(s) e.g. ${v.nodes[0]?.target.join(' ')}`),
  ).toEqual([]);
});
