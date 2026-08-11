// @feature backlog
import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';

// Dev-mode parity check for REQ-6 (project_plans/isolated-dev-stacks): proves the
// Playwright dev-mode harness (playwright.dev-mode.config.ts, Epic 5.1) actually
// serves the app end-to-end — a real browser, a real `next dev` origin, and a real
// ConnectRPC call across the dynamic-port boundary to a dynamically-ported backend.
//
// This intentionally mirrors tests/e2e/backlog.spec.ts's seeding approach (API-driven
// item creation, same as its "Triage" suite) and locator/page-object usage
// (BacklogPage.getTableRows()) rather than adding new coverage — it is a parity
// check, not new test surface. Unlike backlog.spec.ts, this file never hardcodes a
// baseURL/port: `page` and `request` resolve relative paths against Playwright's
// config-driven `use.baseURL`, which playwright.dev-mode.config.ts sets from
// process.env.TEST_SERVER_URL (the dynamically-assigned frontend origin), not the
// static-exported build on port 8544.
test.describe('backlog-dev-mode', () => {
  // Enable the backlog feature flag before the test runs, and restore it to disabled
  // afterwards — same as backlog.spec.ts's beforeAll/afterAll. The flag defaults to
  // off, so without this the layout guard redirects away from /backlog and the
  // waitForSelector('[data-testid="backlog-page"]') assertion below would fail.
  test.beforeAll(async ({ request }) => {
    await request.post('/api/session.v1.SessionService/UpdateFeatureFlag', {
      headers: { 'Content-Type': 'application/json' },
      data: { name: 'backlog', enabled: true },
    });
  });

  test.afterAll(async ({ request }) => {
    await request.post('/api/session.v1.SessionService/UpdateFeatureFlag', {
      headers: { 'Content-Type': 'application/json' },
      data: { name: 'backlog', enabled: false },
    });
  });

  test('should display a seeded backlog item when navigating to the backlog page', async ({ page, request }) => {
    const backlogPage = new BacklogPage(page);
    const itemTitle = `Dev Mode Parity Item ${Date.now()}`;

    // Seed a backlog item via the API directly — the same mechanism backlog.spec.ts's
    // "Triage" suite uses (request.post to CreateBacklogItem) — so this test does not
    // depend on the table/empty-state UI being in any particular starting state.
    const createRes = await request.post('/api/session.v1.BacklogService/CreateBacklogItem', {
      headers: { 'Content-Type': 'application/json' },
      data: { title: itemTitle, priority: 3, repoPath: '', skipTriage: true },
    });
    const body = (await createRes.json()) as { item?: { id: string } };
    const createdItemId = body.item?.id;

    try {
      // Pre-seed the first-visit tour as already dismissed, matching backlog.spec.ts's
      // beforeEach, so the tour modal doesn't pop up and block the assertion below.
      await page.addInitScript(() => {
        localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
      });

      // Navigate against playwright.dev-mode.config.ts's dynamically-assigned frontend
      // origin (config-driven baseURL) — no hardcoded host/port here.
      await page.goto('/backlog', { waitUntil: 'domcontentloaded' });
      await page.waitForSelector('[data-testid="backlog-page"]', { timeout: 15000 });

      // Same backlog-item-visible locator backlog.spec.ts uses throughout.
      const itemRow = backlogPage.getTableRows().filter({ hasText: itemTitle });
      await expect(itemRow.first()).toBeVisible();
    } finally {
      // Best-effort cleanup so the seeded item does not pollute other dev-mode runs.
      if (createdItemId) {
        await request
          .post('/api/session.v1.BacklogService/ArchiveBacklogItem', {
            headers: { 'Content-Type': 'application/json' },
            data: { id: createdItemId },
          })
          .catch(() => {});
      }
    }
  });
});
