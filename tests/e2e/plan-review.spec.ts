// @feature backlog:reject-plan, backlog:get-plan-artifact-content, backlog-plan-verdict-box, backlog-plan-content-viewer
/**
 * E2E tests for the plan-approval-ux feature: the plan-review status card
 * (PlanVerdictBox), the reject-with-reason flow, and in-app markdown
 * rendering of the plan document (PlanArtifactsSection).
 *
 * Scope note: populating a real plan_artifacts_path requires a real
 * TriggerTriage call (a real headless Claude subprocess — slow/flaky/costly
 * in CI, per plan-gate.spec.ts's own established convention). Consistent
 * with backlog-pipeline-mode.spec.ts's "what ran" surface tests, this spec
 * instead intercepts GetBacklogItem/GetPlanArtifactContent/RejectPlan for a
 * real, API-created backlog item to exercise the real frontend components
 * against fabricated-but-realistic plan data.
 */
import { test, expect, Page } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const FAKE_PLAN_PATH = '/tmp/e2e-fake-plan-dir';
const FAKE_PLAN_CONTENT = '# Fake Plan\n\nDo the thing.\n';

/**
 * Intercepts GetBacklogItem to report `plan_artifacts_path` (never set by a
 * real triage in this spec), GetPlanArtifactContent to serve fabricated
 * plan.md content, and RejectPlan to record a rejection reason in the same
 * in-memory state subsequent GetBacklogItem calls reflect — so the reject →
 * "Changes requested" → reason-visible flow round-trips realistically
 * without a real plan_artifacts_path on disk.
 */
async function mockPlanRoutes(page: Page, itemId: string) {
  let rejectionReason = '';

  await page.route('**/api/session.v1.BacklogService/GetBacklogItem', async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    if (json?.item?.id === itemId) {
      json.item.planArtifactsPath = FAKE_PLAN_PATH;
      json.item.planRejectionReason = rejectionReason;
    }
    await route.fulfill({ response, json });
  });

  await page.route('**/api/session.v1.BacklogService/GetPlanArtifactContent', async (route) => {
    const req = route.request().postDataJSON() as { itemId?: string };
    if (req.itemId === itemId) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          content: FAKE_PLAN_CONTENT,
          truncated: false,
          sizeBytes: String(FAKE_PLAN_CONTENT.length),
          modifiedAtUnixMs: '1000',
        }),
      });
      return;
    }
    await route.continue();
  });

  await page.route('**/api/session.v1.BacklogService/RejectPlan', async (route) => {
    const req = route.request().postDataJSON() as { itemId?: string; reason?: string };
    if (req.itemId === itemId) {
      rejectionReason = req.reason ?? '';
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          item: { id: itemId, planApproved: false, planRejectionReason: rejectionReason },
        }),
      });
      return;
    }
    await route.continue();
  });
}

test.describe('plan-review flow', () => {
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

  test('e2e:plan-verdict-box-reject-flow - user can see pending review, reject with a reason, and see Changes requested with the reason and a Regenerate button', async ({
    page,
    request,
  }) => {
    const itemTitle = `plan-review-test-${Date.now()}`;

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

    await mockPlanRoutes(page, createdItemId!);

    await page.goto(`${BASE_URL}/backlog`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('[data-testid="backlog-page"]', { timeout: 15000 });

    const backlogPage = new BacklogPage(page);
    await backlogPage.openItemDetail(itemTitle);
    await expect(backlogPage.getItemDetailPane()).toBeVisible();

    await expect(page.getByText('Pending review')).toBeVisible();

    await page.locator('[data-testid="backlog-action-reject-plan"]').click();
    await page.locator('[data-testid="plan-reject-reason"]').fill('missing caching plan');
    await page.locator('[data-testid="backlog-action-reject-plan-submit"]').click();

    await expect(page.locator('[role="status"]').getByText('Changes requested')).toBeVisible();
    await expect(page.getByTestId('plan-rejection-reason')).toHaveText('missing caching plan');
    await expect(page.locator('[data-testid="backlog-action-regenerate-plan"]')).toBeVisible();
  });

  test('e2e:plan-content-viewer-renders-markdown - plan.md content renders as formatted markdown, not the raw path', async ({
    page,
    request,
  }) => {
    const itemTitle = `plan-content-test-${Date.now()}`;

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

    await mockPlanRoutes(page, createdItemId!);

    await page.goto(`${BASE_URL}/backlog`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('[data-testid="backlog-page"]', { timeout: 15000 });

    const backlogPage = new BacklogPage(page);
    await backlogPage.openItemDetail(itemTitle);
    await expect(backlogPage.getItemDetailPane()).toBeVisible();

    const rendered = page.getByTestId('backlog-plan-content-rendered');
    await expect(rendered).toBeVisible();
    await expect(rendered.locator('h1')).toHaveText('Fake Plan');
    await expect(rendered).not.toHaveText(FAKE_PLAN_PATH);
  });
});
