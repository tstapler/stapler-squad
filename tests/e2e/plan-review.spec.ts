// @feature backlog:reject-plan, backlog-plan-verdict-box
/**
 * E2E coverage for Gap 3 (project_plans/backlog-operator-feedback-loop,
 * Epics 3-4) — Approve and Request Changes visible side by side
 * (PlanVerdictBox.tsx / ActionsSection.tsx), rejecting a plan updates the
 * card in place, and the follow-up Regenerate action.
 *
 * Seeding: no existing RPC/debug endpoint creates a "has a real plan
 * artifact on disk, gated on plan approval" item directly for a "ready"
 * status item. The `/api/debug/backlog/seed-stuck` debug endpoint (e2e-local
 * only — see backlog_debug_seed_handler.go) with `hasPlan: true` and
 * `reason: "plan_not_approved"` is the one existing seed path that writes a
 * real plan_artifacts_path file and sets it on a real BacklogItem row — its
 * `statusForSeedReason` maps that reason to "queued", not "ready". Per
 * itemActions.ts's `getAvailableActions`, "queued" is gated on plan approval
 * exactly like "ready" (both add `approve_plan` when `hasPlan &&
 * isGatedOnPlanApproval`), and BacklogItemDetail.tsx renders PlanVerdictBox
 * for status "queued" the same as "ready" — so this seed path exercises the
 * same AC3/AC4/AC5 behavior the plan intended, just via "queued" instead of
 * "ready" (the illustrative status in plan.md/validation.md, not a literal
 * requirement — the acceptance criterion is "Approve and Request Changes
 * visible together", not a specific status string).
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function waitForBacklogRPCsEnabled(request: APIRequestContext) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListPipelineModes`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

/** Seeds a real backlog item with a real plan_artifacts_path via the e2e-local debug seed endpoint (see file header). */
async function seedItemWithPlan(request: APIRequestContext, title: string): Promise<string> {
  const res = await request.post(`${BASE_URL}/api/debug/backlog/seed-stuck`, {
    headers: { "Content-Type": "application/json" },
    data: { title, reason: "plan_not_approved", hasPlan: true },
  });
  if (!res.ok()) {
    throw new Error(`seed-stuck failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { itemId: string };
  return body.itemId;
}

async function archiveItem(request: APIRequestContext, itemId: string) {
  await request
    .post(`${BASE_URL}/api/session.v1.BacklogService/ArchiveBacklogItem`, {
      headers: { "Content-Type": "application/json" },
      data: { id: itemId },
    })
    .catch(() => {
      // Best-effort cleanup — do not fail the test on cleanup errors.
    });
}

async function openDetail(page: Page, itemTitle: string): Promise<BacklogPage> {
  const backlogPage = new BacklogPage(page);
  await backlogPage.goto();
  await backlogPage.waitForPageLoad();
  await backlogPage.openItemDetail(itemTitle);
  await expect(backlogPage.getItemDetailPane()).toBeVisible();
  return backlogPage;
}

function planReviewStatus(page: Page) {
  return page.getByRole("status", { name: "Plan review status" });
}

test.describe("plan-review", () => {
  test.beforeAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { "Content-Type": "application/json" },
      data: { name: "backlog", enabled: true },
    });
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { "Content-Type": "application/json" },
      data: { name: "backlog", enabled: false },
    });
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      // Two separate onboarding modals can intercept pointer events on first
      // load: the app-wide "One place for all your AI coding sessions" tour
      // and the backlog-specific "How backlog items work" tour. Both must be
      // suppressed (see backlog-plan-approval-flicker.spec.ts).
      localStorage.setItem("stapler-squad:onboarded", "true");
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test("shows Approve and Request Changes together and updates status after rejecting", async ({
    page,
    request,
  }) => {
    const itemTitle = `plan-review-approve-reject-${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedItemWithPlan(request, itemTitle);
      await openDetail(page, itemTitle);

      const approveBtn = page.getByTestId("backlog-action-approve-plan");
      const rejectToggle = page.getByTestId("backlog-action-reject-plan");
      await expect(approveBtn).toBeVisible();
      await expect(rejectToggle).toBeVisible();

      await rejectToggle.click();
      const reasonInput = page.getByTestId("plan-reject-reason");
      await reasonInput.fill("Please also handle the mobile navigation case.");
      await page.getByTestId("backlog-action-reject-plan-submit").click();

      await expect(planReviewStatus(page)).toContainText("Revisions requested");
      // The reject form's own textarea (data-testid="plan-reject-reason")
      // closes asynchronously right after the same successful submit that
      // updates the status text above — a getByText search for the reason
      // can transiently match both the persisted <p> in the status card AND
      // the still-open textarea's value (Playwright's getByText matches a
      // <textarea>'s current value too), a strict-mode violation. Wait for
      // the form to actually close first so only the persisted paragraph
      // remains, then assert on the now-unambiguous text.
      await expect(page.getByTestId("plan-reject-reason")).toHaveCount(0);
      await expect(page.getByText("Please also handle the mobile navigation case.")).toBeVisible();
      await expect(page.getByTestId("backlog-action-regenerate-plan")).toBeVisible();
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("keeps the Request Changes submit button disabled until a reason is typed", async ({
    page,
    request,
  }) => {
    const itemTitle = `plan-review-empty-reason-${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedItemWithPlan(request, itemTitle);
      await openDetail(page, itemTitle);

      await page.getByTestId("backlog-action-reject-plan").click();
      const submit = page.getByTestId("backlog-action-reject-plan-submit");
      const reasonInput = page.getByTestId("plan-reject-reason");

      await expect(submit).toHaveAttribute("aria-disabled", "true");
      await expect(submit).toBeDisabled();

      await reasonInput.fill("   ");
      await expect(submit).toHaveAttribute("aria-disabled", "true");
      await expect(submit).toBeDisabled();

      await reasonInput.fill("A real reason");
      await expect(submit).not.toHaveAttribute("aria-disabled", "true");
      await expect(submit).toBeEnabled();
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("keeps the typed reason visible and shows an error toast when RejectPlan fails", async ({
    page,
    request,
  }) => {
    const itemTitle = `plan-review-reject-fails-${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedItemWithPlan(request, itemTitle);
      await openDetail(page, itemTitle);

      await page.route("**/api/session.v1.BacklogService/RejectPlan", async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ code: "internal", message: "failed to reject plan: simulated e2e failure" }),
        });
      });

      await page.getByTestId("backlog-action-reject-plan").click();
      const reasonInput = page.getByTestId("plan-reject-reason");
      await reasonInput.fill("A reason that must not be lost");
      await page.getByTestId("backlog-action-reject-plan-submit").click();

      // PlanVerdictBox's own local catch renders an InlineError (role=alert)
      // in addition to BacklogItemDetail's action toast — assert the former,
      // a deterministic message independent of toast auto-dismiss timing.
      await expect(page.getByRole("alert").filter({ hasText: "Action failed" })).toBeVisible();

      // The reason textarea is still mounted (form never closed) and still
      // holds the typed text — a failed submit never silently discards it.
      await expect(reasonInput).toBeVisible();
      await expect(reasonInput).toHaveValue("A reason that must not be lost");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("surfaces a working Regenerate action after changes are requested", async ({ page, request }) => {
    const itemTitle = `plan-review-regenerate-${Date.now()}`;
    let itemId: string | undefined;
    const rejectionReason = "Handle the mobile case in the plan.";

    try {
      itemId = await seedItemWithPlan(request, itemTitle);
      await openDetail(page, itemTitle);

      // Real RejectPlan round trip — this item really has planArtifactsPath
      // set on disk (see file header), so the real handler succeeds.
      await page.getByTestId("backlog-action-reject-plan").click();
      await page.getByTestId("plan-reject-reason").fill(rejectionReason);
      await page.getByTestId("backlog-action-reject-plan-submit").click();
      await expect(planReviewStatus(page)).toContainText("Revisions requested");

      // Regenerating calls TriggerTriage(id, planRejectionReason) — a real
      // headless triage LLM call, which this suite avoids (see
      // triage-question-answer.spec.ts's file header for the same
      // rationale). Fulfill a canned success, then have the *next*
      // GetBacklogItem response reflect what a completed regeneration would
      // have produced (rejection reason cleared, still not approved) —
      // simulating "the regeneration landed" the same way
      // backlog-pipeline-mode.spec.ts fabricates ItemSession fields that
      // only a real TriggerTriage/SpawnSessionFromItem call would set.
      let regenerated = false;
      await page.route("**/api/session.v1.BacklogService/TriggerTriage", async (route) => {
        regenerated = true;
        await route.fulfill({ json: { itemSession: { id: "e2e-fake-regen-session" } } });
      });
      await page.route("**/api/session.v1.BacklogService/GetBacklogItem", async (route) => {
        const response = await route.fetch();
        const json = await response.json();
        if (regenerated && json?.item?.id === itemId) {
          json.item.planRejectionReason = "";
          json.item.planApproved = false;
        }
        await route.fulfill({ response, json });
      });

      const triggerTriageRequest = page.waitForRequest(
        (req) =>
          req.url().includes("/api/session.v1.BacklogService/TriggerTriage") && req.method() === "POST"
      );
      await page.getByTestId("backlog-action-regenerate-plan").click();

      const req = await triggerTriageRequest;
      const body = req.postDataJSON() as { itemId?: string; feedback?: string };
      expect(body.itemId).toBe(itemId);
      expect(body.feedback).toBe(rejectionReason);

      await expect(planReviewStatus(page)).toContainText("Pending review");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });
});
