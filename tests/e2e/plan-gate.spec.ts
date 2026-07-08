// @feature backlog:transition-status, backlog:spawn-session
import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Plan Gate', () => {
  test.beforeAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { 'Content-Type': 'application/json' },
      data: { name: 'backlog', enabled: true },
    });
  });

  test.afterAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { 'Content-Type': 'application/json' },
      data: { name: 'backlog', enabled: false },
    });
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
    });
  });

  let createdItemId: string | undefined;

  test.afterEach(async ({ request }) => {
    if (!createdItemId) return;
    try {
      await request.post(`${BASE_URL}/api/session.v1.BacklogService/ArchiveBacklogItem`, {
        headers: { 'Content-Type': 'application/json' },
        data: { id: createdItemId },
      });
    } catch {
      // Best-effort cleanup — do not fail the test on cleanup errors.
    }
    createdItemId = undefined;
  });

  test('e2e:plan-gate-blocks-spawn-until-approved-or-skipped - Spawn Session is disabled for a ready item until the plan is approved or skipped', async ({ page, request }) => {
    const itemTitle = `plan-gate-test-${Date.now()}`;

    // Create a "ready" item directly in the "ready" status via skip_triage, then
    // transition idea -> ready is not needed since triage is skipped entirely.
    const createRes = await request.post(`${BASE_URL}/api/session.v1.BacklogService/CreateBacklogItem`, {
      headers: { 'Content-Type': 'application/json' },
      data: {
        title: itemTitle,
        priority: 3,
        repoPath: '',
        skipTriage: true,
        acceptanceCriteria: [{ text: 'seed criterion' }],
      },
    });
    const body = (await createRes.json()) as { item?: { id: string } };
    createdItemId = body.item?.id;
    expect(createdItemId).toBeTruthy();

    await request.post(`${BASE_URL}/api/session.v1.BacklogService/TransitionBacklogItemStatus`, {
      headers: { 'Content-Type': 'application/json' },
      data: { itemId: createdItemId, targetStatus: 'ready' },
    });

    await page.goto(`${BASE_URL}/backlog`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('[data-testid="backlog-page"]', { timeout: 15000 });

    const backlogPage = new BacklogPage(page);
    await backlogPage.openItemDetail(itemTitle);
    await expect(backlogPage.getItemDetailPane()).toBeVisible();

    // Neither skip_planning nor plan_approved is set — Spawn Session must be blocked.
    const spawnBtn = page.locator('[data-testid="backlog-action-spawn-session"]');
    await expect(spawnBtn).toBeVisible();
    await expect(spawnBtn).toHaveAttribute('aria-disabled', 'true');
    await expect(spawnBtn).toHaveAttribute('title', 'Approve the plan or enable skip_planning to spawn a session');

    // Enabling skip_planning lifts the gate without requiring plan approval.
    await request.post(`${BASE_URL}/api/session.v1.BacklogService/UpdateBacklogItem`, {
      headers: { 'Content-Type': 'application/json' },
      data: { itemId: createdItemId, skipPlanning: true },
    });

    await backlogPage.closeItemDetail();
    await backlogPage.openItemDetail(itemTitle);
    await expect(backlogPage.getItemDetailPane()).toBeVisible();

    const spawnBtnAfterSkip = page.locator('[data-testid="backlog-action-spawn-session"]');
    await expect(spawnBtnAfterSkip).not.toHaveAttribute('aria-disabled', 'true');
  });
});
