// @feature up-next-tabs, unfinished-github-prs
/**
 * Up Next tab shell: default tab, URL deep links, persistence, manual-activation keyboard
 * model, scoped contrast check and 320px reflow. The isolated e2e server has no GitHub account,
 * so PR card content is covered by Jest; here the PRs panel only shows its not-connected state.
 */
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { UpNextPage } from './pages/UpNextPage';
import { deleteSeededStuckItems, enableBacklogFeatureFlag, seedStuckItem } from './pages/StuckItemsPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

// Seeded stuck items persist on the shared test server; remove them so other specs don't see them.
test.afterEach(async ({ request }) => {
  await deleteSeededStuckItems(request);
});

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

  test('upNext_should_LeavePage_When_BackAfterThreeTabSwitches', async ({ page }) => {
    const upNext = new UpNextPage(page);
    await page.goto(`${BASE_URL}/`, { waitUntil: 'domcontentloaded' });
    await upNext.goto();
    await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');

    for (const id of ['stuck', 'worktrees', 'queue'] as const) {
      await upNext.selectTab(id);
      await expect(upNext.tab(id)).toHaveAttribute('aria-selected', 'true');
    }

    // Tab changes use router.replace, so Back skips them and leaves /unfinished.
    await page.goBack({ waitUntil: 'domcontentloaded' });
    await expect(page).not.toHaveURL(/\/unfinished/);
  });

  test('upNext_should_ShowConnectBannerAndHideBadge_When_NoGitHubToken', async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on('pageerror', (err) => consoleErrors.push(err.message));

    // The isolated e2e server registers no WebSocket bridge for WatchUserPRs, so the real stream
    // fails with "WebSocket connection failed". Serve the no-token snapshot the server would send
    // instead: UserPREvent{event_type:"snapshot", auth_state:{available:false}} in a Connect envelope.
    await page.routeWebSocket(/WatchUserPRs/, (ws) => {
      ws.onMessage(() => {
        const payload = Buffer.concat([Buffer.from([0x0a, 0x08]), Buffer.from('snapshot'), Buffer.from([0x1a, 0x00])]);
        const header = Buffer.alloc(5);
        header.writeUInt32BE(payload.length, 1);
        ws.send(Buffer.concat([header, payload]));
      });
    });

    const upNext = new UpNextPage(page);
    await upNext.goto('prs');

    await expect(page.getByTestId('github-add-account-panel')).toBeVisible();
    // No count means no badge, so the accessible name carries no "need attention" suffix.
    await expect(upNext.tab('prs')).not.toHaveAccessibleName(/need attention/);
    await expect(page.getByTestId('up-next-tab-badge-prs')).toHaveCount(0);

    await upNext.selectTab('worktrees');
    await expect(upNext.tab('worktrees')).toHaveAttribute('aria-selected', 'true');
    await upNext.selectTab('queue');
    await expect(upNext.tab('queue')).toHaveAttribute('aria-selected', 'true');
    expect(consoleErrors).toEqual([]);
  });

  test('upNext_should_ShowNotFoundNoticeAndShowAllAction_When_ItemMissing', async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    await page.goto(`${BASE_URL}/unfinished?item=gone-1`, { waitUntil: 'domcontentloaded' });

    await expect(page.getByRole('tab', { name: /Stuck/ })).toHaveAttribute('aria-selected', 'true');
    const notice = page.getByRole('status').filter({ hasText: 'Item gone-1 was not found' });
    await expect(notice).toBeVisible();

    await notice.getByRole('button', { name: 'Show all stuck items' }).click();
    await expect(page).not.toHaveURL(/[?&]item=/);
    await expect(page).toHaveURL(/[?&]tab=stuck/);
    await expect(notice).toHaveCount(0);
    await expect(page.getByRole('tab', { name: /Stuck/ })).toHaveAttribute('aria-selected', 'true');
  });

  test('upNext_should_RestoreQueueAndShowTabParam_When_ClickedThenReloaded', async ({ page }) => {
    const upNext = new UpNextPage(page);
    await upNext.goto();
    await upNext.selectTab('queue');
    await expect(page).toHaveURL(/tab=queue/);

    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect(upNext.tab('queue')).toHaveAttribute('aria-selected', 'true');
    await expect(page).toHaveURL(/tab=queue/);
  });

  test('upNext_should_SelectStuckAndKeepStoredTab_When_ItemDeepLinkThenWorktreesClickedAndReloaded', async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    const title = `fix: deep link keeps stored tab seed ${Date.now()}`;
    const itemId = await seedStuckItem(request, {
      itemId: 'e2e-deeplink-1',
      title,
      reason: 'rework_cap',
      context: 'cap hit',
    });

    // Runs after resetStorage's script; the session flag keeps reloads from re-seeding.
    await page.addInitScript(() => {
      if (!sessionStorage.getItem('e2e-seeded-worktrees')) {
        sessionStorage.setItem('e2e-seeded-worktrees', '1');
        localStorage.setItem('up-next-tab', 'worktrees');
        (window as unknown as { __prsSkeletonSeen: boolean }).__prsSkeletonSeen = false;
      }
      new MutationObserver((records) => {
        for (const r of records) {
          r.addedNodes.forEach((n) => {
            if (n instanceof HTMLElement && (n.matches('[data-testid="prs-skeleton"]') || n.querySelector('[data-testid="prs-skeleton"]'))) {
              (window as unknown as { __prsSkeletonSeen: boolean }).__prsSkeletonSeen = true;
            }
          });
        }
      }).observe(document, { childList: true, subtree: true });
    });

    const upNext = new UpNextPage(page);
    await page.goto(`${BASE_URL}/unfinished?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    await expect(upNext.tab('stuck')).toHaveAttribute('aria-selected', 'true');
    const card = page.locator(`[data-testid="stuck-item"][data-item-id="${itemId}"]`);
    await expect(card).toHaveAttribute('aria-expanded', 'true');
    expect(await page.evaluate(() => (window as unknown as { __prsSkeletonSeen: boolean }).__prsSkeletonSeen)).toBe(false);
    expect(await page.evaluate(() => localStorage.getItem('up-next-tab'))).toBe('worktrees');

    await upNext.selectTab('worktrees');
    await expect(upNext.tab('worktrees')).toHaveAttribute('aria-selected', 'true');
    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect(upNext.tab('worktrees')).toHaveAttribute('aria-selected', 'true');
  });

  test('upNext_should_MoveFocusByArrowHomeEndAndSelectOnEnter_When_TabListFocused', async ({ page }) => {
    const upNext = new UpNextPage(page);
    await upNext.goto();
    await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');

    await upNext.tab('prs').focus();
    await page.keyboard.press('ArrowRight');
    await expect(upNext.tab('stuck')).toBeFocused();
    await expect(upNext.tab('prs')).toHaveAttribute('aria-selected', 'true');

    await page.keyboard.press('Enter');
    await expect(upNext.tab('stuck')).toHaveAttribute('aria-selected', 'true');
    await expect(page).toHaveURL(/[?&]tab=stuck/);
    await expect(upNext.tab('stuck')).toBeFocused();

    await page.keyboard.press('End');
    await expect(upNext.tab('queue')).toBeFocused();
    await expect(upNext.tab('stuck')).toHaveAttribute('aria-selected', 'true');

    await page.keyboard.press('Home');
    await expect(upNext.tab('prs')).toBeFocused();
    await expect(upNext.tab('stuck')).toHaveAttribute('aria-selected', 'true');
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
