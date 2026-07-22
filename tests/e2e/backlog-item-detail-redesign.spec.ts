// @feature backlog-item-detail-lifecycle-summary, backlog-item-detail-diagnostic-panel
/**
 * E2E tests for the redesigned backlog item detail panel
 * (project_plans/backlog-item-detail-ux, Epic 6.1 / Story 6.1.2) — proves
 * the two success metrics validation.md's Happy Path Scenario names:
 * the Lifecycle Summary is visible with zero prior clicks, and every
 * session row (including synthetic ones) is inspectable, never a dead
 * link.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * Test data is seeded via `seedHeadlessTriageItem()`
 * (./pages/BacklogItemDetailPage.ts), which creates a real BacklogItem plus
 * one linked `headless-triage-*` ItemSession through the
 * `/api/debug/backlog/seed-headless-triage-session` debug endpoint
 * (server/services/backlog_debug_seed_handler.go, Task 6.1.2a) — registered
 * only when STAPLER_SQUAD_INSTANCE=e2e-local, never reachable in a normal
 * deploy.
 *
 * NOTE: this spec was written and type-checked (`npx tsc --noEmit -p
 * tests/e2e`) but was NOT run against a live ./stapler-squad instance in
 * this environment — no e2e-local server was running here. It has not been
 * verified to pass; treat it as unexecuted until a real run confirms it.
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage, seedHeadlessTriageItem } from "./pages/BacklogItemDetailPage";
import { enableBacklogFeatureFlag } from "./pages/StuckItemsPage";

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

test.describe("backlog item detail redesign", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  test("shows the Lifecycle Summary with zero prior clicks", async ({ page, request }) => {
    const title = `e2e headless-triage lifecycle ${Date.now()}`;
    await seedHeadlessTriageItem(request, { title, status: "review" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    // Zero clicks past opening the item — no section expanded yet.
    await expect(detailPage.lifecycleSummary).toBeVisible();
  });

  test("reveals the synthetic session row's TriageReviewPanel readOnly summary when expanded", async ({
    page,
    request,
  }) => {
    const title = `e2e headless-triage diagnostic ${Date.now()}`;
    const { sessionId } = await seedHeadlessTriageItem(request, { title, status: "review" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    // Sessions section defaults expanded (SessionsSection.tsx), so the
    // synthetic row's own header is reachable without an extra click on
    // the Sessions section itself.
    const row = detailPage.syntheticSessionRow(sessionId);
    await expect(row).toBeVisible();
    await expect(row).toHaveAttribute("aria-expanded", "false");

    await row.click();
    await expect(row).toHaveAttribute("aria-expanded", "true");

    await expect(detailPage.sessionDiagnosticPanel()).toBeVisible();
    await expect(detailPage.triageReviewPanel()).toBeVisible();
  });

  test("expands a top-level section from its own default-collapsed state", async ({ page, request }) => {
    const title = `e2e headless-triage section-expand ${Date.now()}`;
    await seedHeadlessTriageItem(request, { title, status: "review" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    // DescriptionSection defaults collapsed for every item (Story 3.1.3).
    await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "false");
    await detailPage.expandSection("description");
    await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");
  });
});
