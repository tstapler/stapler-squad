// @feature backlog:list-page
/**
 * E2E tests for the backlog work-item concurrency queue's visible surface:
 * a "queued" item shows a distinguishable status badge in the backlog list
 * (stapler-squad-add-backlog-queue-state, Acceptance Criterion 8).
 *
 * Test data is seeded via seedQueuedItem() in ./pages/BacklogQueuePage.ts,
 * which creates a BacklogItem directly in "queued" status through the
 * `/api/debug/backlog/seed-queued` debug endpoint — registered only for
 * STAPLER_SQUAD_INSTANCE=e2e-local (server.go), never reachable in a normal
 * deploy. Driving the queue by actually filling the concurrency cap with real
 * session spawns would be slow and flaky in e2e; the dequeue mechanics (FIFO
 * order, CAS claim exclusivity, rollback on spawn failure, immediate dequeue
 * on limit raise) are covered by Go tests in server/services and session
 * (TestDequeueNextQueuedItems_*, TestDequeue_ConcurrentClaimsAreExclusive,
 * TestBacklogLifecycleListener_*_TriggersDequeue,
 * TestUpdateGlobalDefaults_RaisingLimitTriggersImmediateDequeue).
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import { seedQueuedItem } from "./pages/BacklogQueuePage";
import { enableBacklogFeatureFlag, disableBacklogFeatureFlag } from "./pages/StuckItemsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function waitForBacklogRPCsEnabled(request: APIRequestContext) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListBacklogItems`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

test.describe("backlog queue", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test("a queued item shows the queued status badge in the backlog list", async ({ page, request }) => {
    const title = `e2e queued item ${Date.now()}`;
    await seedQueuedItem(request, title);

    await page.goto(`${BASE_URL}/backlog`, { waitUntil: "domcontentloaded" });
    await page.waitForSelector('[data-testid="backlog-page"]', { timeout: 15000 });

    const row = page.getByTestId("backlog-table-row").filter({ hasText: title });
    await expect(row).toBeVisible();
    await expect(row.getByTestId("backlog-status-queued")).toBeVisible();
    await expect(row.getByTestId("backlog-status-queued")).toHaveText("Queued");
  });
});
