// @feature backlog:create-epic-integration-child, backlog:transition-status
/**
 * Epic integration child: epic created -> siblings done -> integration child auto-created ->
 * epic's done transition blocked -> integration check passes -> epic closes.
 * The planning hook is driven through the CreateEpicIntegrationChild RPC (the Epic items feature
 * will call the same entry point).
 */
import { test, expect, type APIRequestContext } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage } from "./pages/BacklogItemDetailPage";
import {
  createBacklogItemDirect,
  deleteBacklogItemsByTitlePrefix,
  enableBacklogFeatureFlag,
  transitionBacklogItemDirect,
} from "./pages/BacklogMutations";
import { setFlag } from "./helpers/feature-flag";
import { baseUrl } from "./helpers/base-url";

const BASE_URL = baseUrl();
const PREFIX = "e2e epic integration";
const json = { "Content-Type": "application/json" };
const backlogRpc = (name: string) => `${BASE_URL}/api/session.v1.BacklogService/${name}`;

async function getItem(request: APIRequestContext, itemId: string) {
  const res = await request.post(backlogRpc("GetBacklogItem"), { headers: json, data: { itemId } });
  expect(res.ok()).toBeTruthy();
  return (await res.json()).item as { id: string; status: string; isIntegrationChild?: boolean; integrationChildId?: string };
}

async function createIntegrationChild(request: APIRequestContext, epicItemId: string, siblingItemIds: string[]) {
  return request.post(backlogRpc("CreateEpicIntegrationChild"), { headers: json, data: { epicItemId, siblingItemIds } });
}

test.describe("epic integration child", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await setFlag(request, "epic_integration_child_enabled", true);
  });

  test.afterAll(async ({ request }) => {
    await deleteBacklogItemsByTitlePrefix(request, [PREFIX, "Integration check: " + PREFIX]);
    await setFlag(request, "epic_integration_child_enabled", false);
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  });

  test("epic closes only after its auto-created integration child passes", async ({ page, request }) => {
    const stamp = Date.now();
    const epicTitle = `${PREFIX} ${stamp}`;
    const epicId = await createBacklogItemDirect(request, {
      title: epicTitle,
      status: "review",
      acCriteria: ["it works", "e2e: true"],
    });
    const siblingA = await createBacklogItemDirect(request, { title: `${PREFIX} slice-a ${stamp}`, status: "done" });
    const siblingB = await createBacklogItemDirect(request, { title: `${PREFIX} slice-b ${stamp}`, status: "done" });

    // Planning completes: exactly one integration child, idempotent on retry.
    const created = await createIntegrationChild(request, epicId, [siblingA, siblingB]);
    expect(created.ok(), await created.text()).toBeTruthy();
    const child = (await created.json()).item as { id: string; isIntegrationChild: boolean };
    expect(child.isIntegrationChild).toBe(true);
    const again = await createIntegrationChild(request, epicId, [siblingA, siblingB]);
    expect(((await again.json()).item as { id: string }).id).toBe(child.id);
    expect((await getItem(request, epicId)).integrationChildId).toBe(child.id);

    // UI: the child carries the badge, the epic shows the awaiting state.
    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();
    await expect(
      page.getByTestId("backlog-table-row").filter({ hasText: `Integration check: ${epicTitle}` }).getByTestId("integration-check-badge"),
    ).toBeVisible();
    const detail = new BacklogItemDetailPage(page);
    await detail.openItemByTitle(epicTitle);
    await expect(page.getByTestId("backlog-integration-check")).toContainText("Awaiting Integration Check");

    // The epic cannot close while the check is pending.
    const blocked = await request.post(`${BASE_URL}/api/debug/backlog/mutate-transition`, {
      headers: json,
      data: { itemId: epicId, targetStatus: "done" },
    });
    expect(blocked.status()).toBe(500);
    expect(await blocked.text()).toContain("integration check is still pending");
    expect((await getItem(request, epicId)).status).toBe("review");

    // Integration check passes -> the epic closes.
    await transitionBacklogItemDirect(request, child.id, "done");
    await transitionBacklogItemDirect(request, epicId, "done");
    expect((await getItem(request, epicId)).status).toBe("done");
  });

  test("an archived integration child does not let the epic close", async ({ request }) => {
    const stamp = Date.now();
    const epicId = await createBacklogItemDirect(request, {
      title: `${PREFIX} archived ${stamp}`,
      status: "review",
      acCriteria: ["e2e: true"],
    });
    const sibling = await createBacklogItemDirect(request, { title: `${PREFIX} archived-slice ${stamp}`, status: "done" });
    const created = await createIntegrationChild(request, epicId, [sibling]);
    const childId = ((await created.json()).item as { id: string }).id;

    await transitionBacklogItemDirect(request, childId, "archived");
    expect((await getItem(request, childId)).status).toBe("archived");
    const res = await request.post(`${BASE_URL}/api/debug/backlog/mutate-transition`, {
      headers: json,
      data: { itemId: epicId, targetStatus: "done" },
    });
    expect(res.status()).toBe(500);
    expect(await res.text()).toContain("integration check is still pending");
  });
});
