// @feature backlog-item-detail-lifecycle-summary, backlog-item-detail-diagnostic-panel, backlog-item-detail-pr-visibility
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
 * The "PR visibility and action reachability" block (phase 2) seeds items
 * through `/api/debug/backlog/seed-stuck` (status is derived from `reason`:
 * abandoned_review -> review, pr_ready_unmerged -> pr_pending) and moves
 * them to archived through the real ArchiveBacklogItem RPC.
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage, seedHeadlessTriageItem } from "./pages/BacklogItemDetailPage";
import { enableBacklogFeatureFlag, seedStuckItem } from "./pages/StuckItemsPage";

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

  test.beforeEach(async ({ page }) => {
    // The first-run onboarding modal intercepts clicks and aria-hides the page.
    await page.addInitScript(() => {
      try {
        window.localStorage.setItem("stapler-squad:onboarded", "true");
      } catch {
        /* ignore */
      }
    });
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

  test("Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria", async ({ page, request }) => {
    const title = `e2e headless-triage section-expand ${Date.now()}`;
    await seedHeadlessTriageItem(request, { title, status: "review" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    // Description now defaults expanded (backlog-description-prominence) —
    // Acceptance Criteria stays visible throughout every state change below,
    // proving it shares no state with Description's collapse toggle.
    const acHeading = page.getByRole("heading", { name: /Acceptance Criteria/ });
    await expect(acHeading).toBeVisible();
    await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");

    await detailPage.collapseSection("description");
    await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "false");
    await expect(acHeading).toBeVisible();

    await detailPage.expandSection("description");
    await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");
    await expect(acHeading).toBeVisible();
  });

  test.describe("PR visibility and action reachability", () => {
    const PR_NUMBER = 4242;
    const PR_URL = `https://github.com/example/repo/pull/${PR_NUMBER}`;

    async function seedAndOpen(
      page: import("@playwright/test").Page,
      request: APIRequestContext,
      opts: { reason: string; withPr: boolean; archive?: boolean }
    ) {
      const title = `e2e pr-visibility ${opts.reason} ${Date.now()}`;
      const itemId = await seedStuckItem(request, {
        itemId: title,
        title,
        reason: opts.reason,
        prNumber: opts.withPr ? PR_NUMBER : 0,
        prUrl: opts.withPr ? PR_URL : "",
      });
      if (opts.archive) {
        const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ArchiveBacklogItem`, {
          headers: { "Content-Type": "application/json" },
          data: { itemId },
        });
        expect(resp.ok()).toBeTruthy();
      }
      // Deep link rather than a table click: archived items are hidden from
      // the default list, and ?item= survives page.reload().
      await page.goto(`${BASE_URL}/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });
      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.pane.waitFor({ state: "visible", timeout: 10000 });
      return { itemId, detailPage };
    }

    test("shows a PR link in the sticky header without scrolling or expanding anything", async ({ page, request }) => {
      const { detailPage } = await seedAndOpen(page, request, { reason: "abandoned_review", withPr: true });

      const badge = detailPage.lifecycleSummary.getByTestId("lifecycle-pr-badge");
      await expect(badge).toBeVisible();
      await expect(badge).toBeInViewport();
      await expect(badge.locator("a")).toHaveAttribute("href", PR_URL);
    });

    test("hides the header PR badge for pr_pending (PullRequestSection owns the link)", async ({ page, request }) => {
      const { detailPage } = await seedAndOpen(page, request, { reason: "pr_ready_unmerged", withPr: true });

      await expect(detailPage.lifecycleSummary).toBeVisible();
      await expect(page.getByTestId("lifecycle-pr-badge")).toHaveCount(0);
    });

    test("renders no PR badge when the item has no prUrl", async ({ page, request }) => {
      const { detailPage } = await seedAndOpen(page, request, { reason: "abandoned_review", withPr: false });

      await expect(detailPage.lifecycleSummary).toBeVisible();
      await expect(page.getByTestId("lifecycle-pr-badge")).toHaveCount(0);
    });

    test("puts the actions in view at the top of the scroll area, above Acceptance Criteria", async ({ page, request }) => {
      const { detailPage } = await seedAndOpen(page, request, { reason: "abandoned_review", withPr: true });

      const firstAction = detailPage.pane.locator('[data-testid^="backlog-action-"]').first();
      await expect(firstAction).toBeVisible();
      await expect(firstAction).toBeInViewport();

      const acHeading = page.getByRole("heading", { name: /Acceptance Criteria/ });
      const actionBox = await firstAction.boundingBox();
      const acBox = await acHeading.boundingBox();
      expect(actionBox).not.toBeNull();
      expect(acBox).not.toBeNull();
      expect(actionBox!.y).toBeLessThan(acBox!.y);
    });

    test("auto-expands Version Control for an archived item with a PR, and a manual collapse survives a real reload", async ({
      page,
      request,
    }) => {
      const { detailPage } = await seedAndOpen(page, request, {
        reason: "abandoned_review",
        withPr: true,
        archive: true,
      });

      // No stored preference yet: prUrl-based default applies.
      await expect(detailPage.sectionHeader("version-control")).toHaveAttribute("aria-expanded", "true");

      await detailPage.collapseSection("version-control");
      await expect(detailPage.sectionHeader("version-control")).toHaveAttribute("aria-expanded", "false");

      // Real browser reload; the ?item= deep link reopens the same item.
      await page.reload();
      await detailPage.pane.waitFor({ state: "visible", timeout: 10000 });

      await expect(detailPage.sectionHeader("version-control")).toHaveAttribute("aria-expanded", "false");
    });
  });
});
