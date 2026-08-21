// @feature backlog:manual-override, backlog:update-item, backlog:transition-status
/**
 * E2E tests for the manual operator escape hatch on a backlog item detail
 * page: forcing a status transition with a required reason, and manually
 * linking an already-existing PR to an item stuck in review (the
 * out-of-band-worktree case — no item_sessions link, so report_pr_created
 * was never callable). See the ticket this closes:
 * https://github.com/tstapler/stapler-squad/issues/335
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage } from "./pages/BacklogItemDetailPage";
import { createBacklogItemDirect, enableBacklogFeatureFlag } from "./pages/BacklogMutations";

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

test.describe("backlog manual override", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  // Pre-seed both first-visit tours (the backlog-specific one and the
  // app-wide worktree-diagram one, web-app/src/components/onboarding/
  // useOnboarding.ts) as already dismissed so neither modal's overlay
  // intercepts clicks on the manual-override controls.
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  });

  test("operator can force a valid status transition with a required reason", async ({ page, request }) => {
    const title = `e2e manual override status ${Date.now()}`;
    await createBacklogItemDirect(request, { title, status: "review" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    await detailPage.expandSection("manual-override");

    const submit = page.getByTestId("manual-override-status-submit");
    await expect(submit).toBeDisabled();

    // The status <select> is populated from the server's own
    // allowed_transitions for this item — never a client-side re-encoding
    // of the transition graph.
    await page.getByTestId("manual-override-status-select").selectOption("in_progress");
    await expect(submit).toBeDisabled();

    await page
      .getByTestId("manual-override-reason-textarea")
      .fill("reviewer session zombied (#334) — unsticking manually");
    await expect(submit).toBeEnabled();

    await submit.click();

    // Reflects the server's real post-transition state, not an optimistic
    // guess — StageTracker's aria-label is derived straight from item.status.
    await expect(page.getByTestId("stage-tracker")).toHaveAttribute(
      "aria-label",
      "Lifecycle stage: In Progress"
    );
  });

  // Regression for the empty Force-status dropdown bug: ListBacklogItems'
  // summary DTO previously omitted allowedTransitions entirely (unlike
  // GetBacklogItem, which was never affected). Asserting directly on the
  // ListBacklogItems response pins the actual DTO this bug lived in — the
  // item detail panel's own GetBacklogItem fetch on mount would mask a
  // regression here, since that RPC always returns allowedTransitions.
  test("ListBacklogItems response carries allowedTransitions for an idea item", async ({ request }) => {
    const title = `e2e manual override idea-status ${Date.now()}`;
    await createBacklogItemDirect(request, { title, status: "idea" });

    const listRes = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListBacklogItems`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    const body = (await listRes.json()) as { items?: Array<{ title?: string; allowedTransitions?: string[] }> };
    const item = (body.items ?? []).find((i) => i.title === title);
    expect(item).toBeDefined();
    expect(item?.allowedTransitions).toEqual(expect.arrayContaining(["archived", "ready", "refining"]));
  });

  test("operator can force-archive an idea item via the manual override dropdown", async ({ page, request }) => {
    const title = `e2e manual override idea-status ${Date.now()}`;
    await createBacklogItemDirect(request, { title, status: "idea" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    await detailPage.expandSection("manual-override");

    const select = page.getByTestId("manual-override-status-select");
    await expect(select.locator("option")).toHaveText(["Select a status…", "archived", "ready", "refining"]);

    await select.selectOption("archived");
    await page
      .getByTestId("manual-override-reason-textarea")
      .fill("duplicate of an existing item — archiving via manual override");
    await page.getByTestId("manual-override-status-submit").click();

    await expect(page.getByTestId("stage-tracker")).toHaveAttribute("aria-label", "Lifecycle stage: Archived");
  });

  test("operator can link an existing PR to an item stuck in review with no live session", async ({
    page,
    request,
  }) => {
    const title = `e2e manual override pr-link ${Date.now()}`;
    await createBacklogItemDirect(request, { title, status: "review" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    await detailPage.expandSection("manual-override");

    await page
      .getByTestId("manual-override-pr-url-input")
      .fill("https://github.com/tstapler/stapler-squad/pull/320");
    await page.getByTestId("manual-override-pr-number-input").fill("320");

    const submit = page.getByTestId("manual-override-pr-submit");
    await expect(submit).toBeEnabled();
    await submit.click();

    // SetBacklogItemPRAndTransition moves review -> pr_pending atomically
    // with the PR fields — both must be visible together.
    await expect(page.getByTestId("stage-modifier-badge")).toHaveText("PR pending");
    await expect(
      page.getByRole("link", { name: "https://github.com/tstapler/stapler-squad/pull/320" })
    ).toBeVisible();
  });
});
