// @feature up-next-tabs, unfinished-github-prs
/**
 * Up Next tab shell: default tab, URL deep links, persistence, manual-activation keyboard
 * model, scoped contrast check and 320px reflow. The isolated e2e server has no GitHub account,
 * so PR card content is covered by Jest; here the PRs panel only shows its not-connected state.
 */
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { UpNextPage } from './pages/UpNextPage';
import { enableBacklogFeatureFlag, seedStuckItem } from './pages/StuckItemsPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Up Next tabs', () => {
  test.beforeEach(async ({ page }) => {
    await new UpNextPage(page).resetStorage();
  });

  test('upNext_should_SelectPRsTabAndShowPanelInViewport_When_FirstVisit', async ({ page }) => {
    const upNext = new UpNextPage(page);
    await upNext.goto();

    await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');
    await expect(upNext.panel('prs')).toBeInViewport();
    await expect(page.getByRole('region', { name: 'GitHub Pull Requests' })).toBeVisible();
  });

  test('upNext_should_SelectStuckTab_When_ItemDeepLinkOpened', async ({ page }) => {
    await page.goto(`${BASE_URL}/unfinished?item=does-not-matter`, { waitUntil: 'domcontentloaded' });

    await expect(page.getByRole('tab', { name: /Stuck/ })).toHaveAttribute('aria-selected', 'true');
  });

  test('upNext_should_RestoreTab_When_ReloadedOrReopenedWithoutTabParam', async ({ page }) => {
    const upNext = new UpNextPage(page);
    await upNext.goto();
    await upNext.selectTab('queue');
    await expect(upNext.tab('queue')).toHaveAttribute('aria-selected', 'true');
    await expect(page).toHaveURL(/[?&]tab=queue/);

    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect(upNext.tab('queue')).toHaveAttribute('aria-selected', 'true');

    // No ?tab= in the URL: the stored tab wins over the PRs default.
    await upNext.goto();
    await expect(upNext.tab('queue')).toHaveAttribute('aria-selected', 'true');
  });

  test('upNext_should_ActivateTabOnlyOnEnter_When_ArrowKeyMovesFocus', async ({ page }) => {
    const upNext = new UpNextPage(page);
    await upNext.goto();
    await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');

    await upNext.tab('prs').focus();
    await page.keyboard.press('ArrowRight');
    await expect(upNext.tab('stuck')).toBeFocused();
    await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');
    await expect(page).not.toHaveURL(/[?&]tab=stuck/);

    await page.keyboard.press('Enter');
    await expect(upNext.tab('stuck')).toHaveAttribute('aria-selected', 'true');
    await expect(page).toHaveURL(/[?&]tab=stuck/);
  });

  test('upNext_should_ReportNoColorContrastViolationOnTabList_When_StuckBadgeSeeded', async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    await seedStuckItem(request, {
      itemId: 'e2e-tab-badge-1',
      title: 'fix: tab badge contrast seed',
      reason: 'rework_cap',
      context: 'cap hit',
    });

    const upNext = new UpNextPage(page);
    await upNext.goto();
    // The Stuck badge is part of the tab's accessible name once the count loads.
    await expect(page.getByRole('tab', { name: /Stuck, \d+ need attention/ })).toBeVisible();

    const results = await new AxeBuilder({ page })
      .include('[role="tablist"]')
      .withRules(['color-contrast'])
      .analyze();
    expect(results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`)).toEqual([]);
    expect(results.passes.some((p) => p.id === 'color-contrast')).toBe(true);
  });

  test.describe('narrow screen', () => {
    test.use({ viewport: { width: 320, height: 800 } });

    test('upNext_should_HaveNoHorizontalOverflowAt320px_When_PRsPanelNotConnected', async ({ page }) => {
      const upNext = new UpNextPage(page);
      await upNext.goto('prs');
      await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');
      await expect(upNext.panel('prs')).toBeVisible();

      // WCAG 1.4.12 text-spacing override.
      await page.addStyleTag({
        content: '* { line-height: 1.5 !important; letter-spacing: 0.12em !important; word-spacing: 0.16em !important; }',
      });

      const overflow = await page.evaluate(() => {
        const panel = document.querySelector('[data-testid="up-next-panel-prs"]') as HTMLElement;
        return {
          document: document.documentElement.scrollWidth - document.documentElement.clientWidth,
          panel: panel.scrollWidth - panel.clientWidth,
        };
      });
      expect(overflow.document).toBeLessThanOrEqual(0);
      expect(overflow.panel).toBeLessThanOrEqual(0);
    });
  });
});
