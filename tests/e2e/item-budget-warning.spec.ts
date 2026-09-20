// @feature backlog:item-budget-warning, insights-dashboard

/**
 * E2E tests for project_plans/backlog-stage-execution-costs's per-item soft
 * budget warning (Epic 5.3, design/ux.md Surface D2 — ItemBudgetWarning.tsx).
 * See implementation/validation.md's UX Acceptance Tests table, rows 4, 10, 18.
 */

import { test, expect, type Page } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';
import {
  createBacklogItemDirect,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from './pages/BacklogMutations';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Item budget warning (design/ux.md Surface D2)', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
      localStorage.setItem('stapler-squad:onboarded', 'true');
      // Pin a consistent theme across every page load in a test — the FOUC
      // script otherwise falls back to prefers-color-scheme, which can differ
      // between this run's backlog-detail load and its /insights load and
      // make the "shared warning color token" test below compare two
      // different themes' token values instead of the same one.
      localStorage.setItem('stapler-theme', 'light');
    });
  });

  // Opens an existing item's detail pane and switches it into edit mode —
  // mirrors backlog-edit-buffering.spec.ts's identical local helper.
  async function openInEditMode(page: Page, title: string): Promise<BacklogPage> {
    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.openItemDetail(title);
    await page.getByTestId('backlog-detail-edit').click();
    await expect(page.getByTestId('backlog-item-form')).toBeVisible();
    return backlogPage;
  }

  test('setting the budget threshold input alone opts into the per-item warning with no separate toggle', async ({ page, request }) => {
    const title = `e2e-budget-warning-opt-in-${Date.now()}`;
    await createBacklogItemDirect(request, { title });

    await openInEditMode(page, title);

    // No threshold configured yet — ItemBudgetWarning renders nothing at all
    // (verified absent, not present-but-hidden — see the next test).
    await expect(page.getByTestId('item-budget-warning')).toHaveCount(0);

    // The ONLY control this test interacts with is the threshold input
    // itself, proving AC4's "opt in via <=1 form field edit, no separate
    // enable toggle" — there is no separate checkbox/switch anywhere in this
    // form for the budget feature; the input's own presence/absence of a
    // value IS the opt-in/opt-out state (BacklogItemForm.tsx's
    // costBudgetThresholdTouchedRef gating).
    await page.getByTestId('backlog-cost-budget-threshold-input').fill('0');
    await page.getByTestId('backlog-form-submit').click();

    // Save succeeded — form closes back to the read-only detail view.
    await expect(page.getByTestId('backlog-item-form')).toHaveCount(0);

    // A threshold of 0 against a freshly created item's real
    // totalEstimatedCostUsd (0 — no ItemSessions yet) crosses
    // ItemBudgetWarning's strict `totalCostUsd < thresholdUsd` guard
    // immediately (0 < 0 is false), so the banner renders without needing to
    // fabricate real session cost data — demonstrating the warning slot goes
    // from entirely absent to visible purely from the single threshold-field
    // edit above.
    await expect(page.getByTestId('item-budget-warning')).toBeVisible({ timeout: 10000 });
  });

  test('shows no budget warning UI when no threshold is configured for the item', async ({ page, request }) => {
    const title = `e2e-budget-warning-no-threshold-${Date.now()}`;
    await createBacklogItemDirect(request, { title });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.openItemDetail(title);

    // Entirely absent from the DOM — ItemBudgetWarning returns null when
    // thresholdUsd is undefined, rather than rendering a hidden element, so
    // this must be a zero count, not merely "not visible".
    await expect(page.getByTestId('item-budget-warning')).toHaveCount(0);
  });

  test('global and per-item budget warnings share consistent warning styling while remaining distinctly labeled', async ({ page, request }) => {
    // --- Per-item warning (Surface D2, ItemBudgetWarning.tsx) ---
    const title = `e2e-budget-warning-shared-style-${Date.now()}`;
    await createBacklogItemDirect(request, { title });
    await openInEditMode(page, title);
    await page.getByTestId('backlog-cost-budget-threshold-input').fill('0');
    await page.getByTestId('backlog-form-submit').click();
    await expect(page.getByTestId('backlog-item-form')).toHaveCount(0);

    const itemBanner = page.getByTestId('item-budget-warning');
    await expect(itemBanner).toBeVisible({ timeout: 10000 });
    await expect(itemBanner).toContainText(/spent/i);

    const itemColors = await itemBanner.evaluate((el) => {
      const cs = getComputedStyle(el);
      const spans = el.querySelectorAll('span');
      const textSpan = spans[spans.length - 1] as HTMLElement | undefined;
      return {
        background: cs.backgroundColor,
        textColor: textSpan ? getComputedStyle(textSpan).color : null,
      };
    });
    // Captured now, before navigating away to /insights below — the banner's
    // own page won't exist anymore once that navigation happens.
    const itemBannerText = (await itemBanner.textContent()) ?? '';

    // --- Global monthly warning (Surface D1, pre-existing ProjectedCostCard) ---
    // Fixture-driven rather than relying on real accumulated usage: useProjectedCost
    // needs >=7 days of current-month daily buckets before it produces a
    // projection at all (useProjectedCost.ts), and InsightsDashboard only renders
    // ProjectedCostCard when summary.sessions is non-empty — a mocked
    // GetInsightsSummary response guarantees both deterministically.
    const now = new Date();
    const year = now.getUTCFullYear();
    const month = now.getUTCMonth();
    const dailyBuckets = Array.from({ length: 7 }, (_, i) => ({
      date: new Date(Date.UTC(year, month, i + 1)).toISOString(),
      totalInputTokens: '1000',
      totalOutputTokens: '500',
      cacheReadTokens: '0',
      estimatedCostUsd: 500,
      sessionCount: 1,
    }));

    await page.route('**/api/session.v1.InsightsService/GetInsightsSummary', async (route) => {
      await route.fulfill({
        json: {
          sessions: [
            {
              sessionId: 's-budget-warning-global',
              conversationId: 'c-budget-warning-global',
              primaryModel: 'claude-sonnet-4-6',
              totalInputTokens: '7000',
              totalOutputTokens: '3500',
              cacheReadTokens: '0',
              estimatedCostUsd: 3500,
              cacheHitRate: 0,
              messageCount: 10,
            },
          ],
          totalCostUsd: 3500,
          totalInputTokens: '7000',
          totalOutputTokens: '3500',
          totalCacheReadTokens: '0',
          overallCacheHitRate: 0,
          daily: dailyBuckets,
        },
      });
    });
    await page.addInitScript(() => {
      // Well below the ~$14-15k/month the fixture above projects — guarantees
      // isOverBudget regardless of the exact number of days in the current month.
      localStorage.setItem('insights_budget_threshold_usd', '50');
    });

    await page.goto(`${BASE_URL}/insights`, { waitUntil: 'domcontentloaded' });

    await expect(page.getByText('Projected this month')).toBeVisible({ timeout: 15000 });
    const globalWarningText = page.getByText('Over budget!', { exact: true });
    await expect(globalWarningText).toBeVisible({ timeout: 10000 });

    const globalColors = await globalWarningText.evaluate((el) => {
      const cs = getComputedStyle(el);
      const parent = el.parentElement;
      return {
        textColor: cs.color,
        background: parent ? getComputedStyle(parent).backgroundColor : null,
      };
    });

    // Same warning color tokens (vars.color.warningBg / vars.color.warningText —
    // see ItemBudgetWarning.css.ts's and ProjectedCostCard.css.ts's shared-token
    // comments) even though these are two genuinely separate components (AC18).
    expect(itemColors.background).toBe(globalColors.background);
    expect(itemColors.textColor).toBe(globalColors.textColor);

    // Distinctly labeled: the per-item banner talks about THIS item's spend
    // ("... spent, threshold ..."); the global card's wording never appears in it.
    expect(itemBannerText).not.toMatch(/projected this month/i);
    expect(itemBannerText).not.toMatch(/over budget!/i);
  });
});
