// @feature backlog:send-back-feedback
/**
 * E2E coverage for the send-back-with-feedback flow
 * (project_plans/plan-feedback, Epic 3.1 / Story 3.1.1) — sending a
 * `review`-status item back to `ready` with typed feedback via
 * `SendBackFeedbackBox` (web-app/src/components/backlog/detail/
 * SendBackFeedbackBox.tsx), chained through `transitionStatus` ->
 * `rejectPlan` -> `triggerTriage` (BacklogItemDetail.tsx's
 * `handleSendBackWithFeedback`).
 *
 * Seeding: RejectPlan's precondition requires a non-empty
 * `planArtifactsPath` (server/services/backlog_service_lifecycle.go), and no
 * `BacklogMutations.ts` helper sets that field. The one existing seed path
 * that writes a real plan-artifacts file and sets it on the item is the
 * `/api/debug/backlog/seed-stuck` debug endpoint with `hasPlan: true` (see
 * plan-review.spec.ts's file header for the same finding). Its
 * `statusForSeedReason` maps `reason: "abandoned_review"` to
 * `BacklogStatusReview` (the default case — server/services/
 * backlog_debug_seed_handler.go), which lands the item at exactly the
 * `review` status this story's AC calls for.
 *
 * TriggerTriage (the third call in the chain) is a real headless-triage LLM
 * call in production — this suite avoids invoking it for real, mirroring
 * plan-review.spec.ts's "Regenerate" test, by fulfilling the RPC with a
 * canned success via page.route().
 *
 * Deviations found while filling in validation.md's remaining UX acceptance
 * criteria (2, 4-12, 15, 17):
 *
 * - Criterion 4/11's steps column says the call-1-failure fallback body
 *   reads "Action failed. Please try again." — that string is never
 *   rendered. ux.md's Surface 6 section documents the correction: the real
 *   fallback (`getErrorMessage(err, "Failed to send back.")`'s literal)
 *   is "Failed to send back." (matching the headline), and code review of
 *   SendBackFeedbackBox.tsx confirms it. These tests assert only the
 *   headline, which is unambiguous either way.
 * - Criterion 15's steps column assumes a tab stop between the toggle and
 *   the textarea for "notice dismiss". SendBackFeedbackBox.tsx renders the
 *   active-session `InlineNotice` without an `onDismiss` prop, so — unlike
 *   every other InlineNotice usage in this app — it has no dismiss button
 *   at all. The tab-order test below asserts the real sequence (toggle ->
 *   textarea, already focused via the component's own open-autofocus
 *   effect -> Cancel -> Submit) instead.
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage } from "./pages/BacklogItemDetailPage";

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

/**
 * Seeds a real backlog item at status "review" with a real
 * plan_artifacts_path set on disk (see file header) via the e2e-local-only
 * debug seed endpoint.
 */
async function seedReviewItemWithPlan(request: APIRequestContext, title: string): Promise<string> {
  const res = await request.post(`${BASE_URL}/api/debug/backlog/seed-stuck`, {
    headers: { "Content-Type": "application/json" },
    data: { title, reason: "abandoned_review", hasPlan: true },
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

/** Fulfills TriggerTriage with a canned success — see file header. */
async function mockTriggerTriageSuccess(page: Page) {
  await page.route("**/api/session.v1.BacklogService/TriggerTriage", async (route) => {
    await route.fulfill({ json: { itemSession: { id: "e2e-fake-send-back-session" } } });
  });
}

/** Fulfills the given BacklogService RPC with a canned 500/internal error, for the RPC-failure UX criteria (4-6, 11, 12). */
async function mockRpcFailure(page: Page, rpcMethod: string, message: string, delayMs = 0) {
  await page.route(`**/api/session.v1.BacklogService/${rpcMethod}`, async (route) => {
    if (delayMs) await new Promise((r) => setTimeout(r, delayMs));
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ code: "internal", message }),
    });
  });
}

/**
 * Creates a real backlog item at status "in_progress" via the e2e-local-only
 * debug mutate-create endpoint — needed for UX criterion 7 (active-session
 * notice), which requires `send_back_ready` eligible (in_progress qualifies,
 * per itemActions.ts's CAN_SEND_BACK_READY) with no plan-artifacts
 * precondition to satisfy, since these tests never actually submit through
 * to a real transitionStatus/rejectPlan round trip. Mirrors
 * backlog-session-steer.spec.ts's createInProgressItem.
 */
async function createInProgressItem(request: APIRequestContext, title: string): Promise<string> {
  const res = await request.post(`${BASE_URL}/api/debug/backlog/mutate-create`, {
    headers: { "Content-Type": "application/json" },
    data: { title, status: "in_progress", priority: 3 },
  });
  if (!res.ok()) {
    throw new Error(`mutate-create failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { itemId: string };
  return body.itemId;
}

interface FakeItemSession {
  id: string;
  sessionUuid: string;
  sessionRole: string;
  endedAt?: string;
  estimatedCostUsd?: number;
}

/**
 * Intercepts GetBacklogItem for `itemId` and appends the given fabricated
 * ItemSession rows — mirrors backlog-session-steer.spec.ts's
 * injectItemSessions. A `sessionRole: "work"` entry with no `endedAt` is
 * exactly what BacklogItemDetail.tsx's `activeWorkSessionCount` counts
 * (`s.role === "work" && !s.endedAt`, mapped from the wire field
 * `sessionRole` by useBacklogService.ts's `mapItemSession`).
 *
 * The returned `injected` promise resolves once the interception has
 * actually fired for `itemId` at least once. The item detail panel becomes
 * visible (and `sendBackToggle` renders) as soon as the FIRST
 * GetBacklogItem response lands, which can race ahead of the intercepted
 * response landing and being applied to component state under load.
 * Without awaiting `injected`, `activeWorkSessionCount`-dependent
 * assertions (the InlineNotice) can flake: the toggle is already
 * interactive, but the injected session hasn't reached component state
 * yet. `await injectItemSessions(...)` itself (as before) still guarantees
 * route registration completes before navigation; callers that depend on
 * the injected sessions should additionally `await` the returned
 * `injected` promise after `openItemByTitle`.
 */
async function injectItemSessions(
  page: Page,
  itemId: string,
  sessions: FakeItemSession[]
): Promise<{ injected: Promise<void> }> {
  let resolveInjected: () => void;
  const injected = new Promise<void>((resolve) => {
    resolveInjected = resolve;
  });
  await page.route("**/api/session.v1.BacklogService/GetBacklogItem", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    if (json?.item?.id === itemId) {
      json.item.itemSessions = [
        ...(json.item.itemSessions ?? []),
        ...sessions.map((s) => ({ estimatedCostUsd: 0, ...s })),
      ];
      resolveInjected();
    }
    await route.fulfill({ response, json });
  });
  return { injected };
}

test.describe("backlog send-back-with-feedback", () => {
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
      localStorage.setItem("stapler-squad:onboarded", "true");
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test("toggle reveals the send-back form and Submit starts disabled", async ({ page, request }) => {
    const title = `send-back-feedback-toggle ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await expect(detailPage.sendBackToggle).toBeVisible();
      await expect(detailPage.sendBackTextarea).toHaveCount(0);

      await detailPage.sendBackToggle.click();

      await expect(detailPage.sendBackTextarea).toBeVisible();
      await expect(detailPage.sendBackTextarea).toHaveValue("");
      await expect(detailPage.sendBackSubmit).toBeDisabled();

      // Criterion 9: the form wrapper is a role="form" landmark named "Send
      // back for re-planning" — getByRole resolving here (strict mode)
      // proves exactly one such element exists.
      await expect(detailPage.sendBackForm).toBeVisible();
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("submitting non-empty feedback sends the item back to ready and shows Revisions requested", async ({
    page,
    request,
  }) => {
    const title = `send-back-feedback-submit ${Date.now()}`;
    const feedback = "missed the mobile case, re-check the auth approach";
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);
      await mockTriggerTriageSuccess(page);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      const urlBeforeSubmit = page.url();

      await detailPage.submitSendBackFeedback(feedback);

      // Criterion 3: success toast, and no navigation away from item detail.
      await expect(detailPage.toast).toContainText("Feedback sent — retriage started.");
      expect(page.url()).toBe(urlBeforeSubmit);

      await expect(page.getByTestId("stage-tracker")).toHaveAttribute("aria-label", "Lifecycle stage: Ready");
      await expect(detailPage.planReviewStatus).toContainText("Revisions requested");
      await expect(detailPage.planReviewStatus).toContainText(feedback);
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("Escape closes the send-back form and clears the textarea", async ({ page, request }) => {
    const title = `send-back-feedback-escape ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await detailPage.sendBackToggle.click();
      await expect(detailPage.sendBackTextarea).toBeVisible();
      await detailPage.sendBackTextarea.fill("a note that should be discarded");
      await expect(detailPage.sendBackTextarea).toHaveValue("a note that should be discarded");

      await detailPage.sendBackTextarea.press("Escape");

      await expect(detailPage.sendBackTextarea).toHaveCount(0);
      await expect(detailPage.sendBackToggle).toBeVisible();
      // Criterion 13: Escape returns focus to the toggle button.
      await expect(detailPage.sendBackToggle).toBeFocused();

      // Reopening confirms the form's state was actually cleared, not just
      // hidden — a stale value would resurface here if handleCancel had
      // failed to reset `feedback`.
      await detailPage.sendBackToggle.click();
      await expect(detailPage.sendBackTextarea).toHaveValue("");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 2
  test("Cancel button and Escape key both close the form without confirmation", async ({ page, request }) => {
    const title = `send-back-feedback-cancel-escape ${Date.now()}`;
    let itemId: string | undefined;
    let dialogFired = false;
    page.on("dialog", (dialog) => {
      dialogFired = true;
      void dialog.dismiss();
    });

    try {
      itemId = await seedReviewItemWithPlan(request, title);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await detailPage.sendBackToggle.click();
      await detailPage.sendBackTextarea.fill("cancel path text");
      await detailPage.sendBackCancel.click();
      await expect(detailPage.sendBackTextarea).toHaveCount(0);

      await detailPage.sendBackToggle.click();
      await detailPage.sendBackTextarea.fill("escape path text");
      await detailPage.sendBackTextarea.press("Escape");
      await expect(detailPage.sendBackTextarea).toHaveCount(0);

      expect(dialogFired).toBe(false);
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 4
  test("transitionStatus failure shows 'Failed to send back' with typed text preserved and Submit re-enabled", async ({
    page,
    request,
  }) => {
    const title = `send-back-feedback-call1-fail ${Date.now()}`;
    const feedback = "missed the mobile case in the original plan";
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);
      await mockRpcFailure(
        page,
        "TransitionBacklogItemStatus",
        "simulated e2e transitionStatus failure"
      );

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await detailPage.sendBackToggle.click();
      await detailPage.sendBackTextarea.fill(feedback);
      await detailPage.sendBackSubmit.click();

      // Headline is fixed by SendBackFeedbackBox.tsx's else branch; body is
      // whatever server message was returned (see file header re: the
      // stale "Action failed. Please try again." fallback text) — only the
      // headline is deterministic enough to assert here.
      await expect(detailPage.sendBackError).toContainText("Failed to send back");
      await expect(detailPage.sendBackTextarea).toHaveValue(feedback);
      await expect(detailPage.sendBackSubmit).toBeEnabled();
      await expect(detailPage.sendBackSubmit).toHaveAttribute("aria-disabled", "false");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 5
  test("rejectPlan/triggerTriage failure after transitionStatus succeeds shows 'Sent back, but retriage didn't start' pointing at PlanVerdictBox", async ({
    page,
    request,
  }) => {
    const title = `send-back-feedback-call23-fail ${Date.now()}`;
    const feedback = "re-check the auth approach for mobile";
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);
      // Only TriggerTriage is intercepted — TransitionBacklogItemStatus and
      // RejectPlan hit the real server (per the task's instructions), so the
      // item genuinely lands at "ready" with planRejectionReason set.
      await mockRpcFailure(page, "TriggerTriage", "simulated e2e triggerTriage failure");

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await detailPage.sendBackToggle.click();
      await detailPage.sendBackTextarea.fill(feedback);
      await detailPage.sendBackSubmit.click();

      await expect(detailPage.sendBackError).toContainText("Sent back, but retriage didn't start");
      await expect(page.getByTestId("stage-tracker")).toHaveAttribute("aria-label", "Lifecycle stage: Ready");
      await expect(detailPage.planReviewStatus).toContainText("Revisions requested");

      // Close the form; PlanVerdictBox's Regenerate affordance is reachable
      // without a reload since rejectPlan already committed server-side.
      await detailPage.sendBackCancel.click();
      await expect(detailPage.regeneratePlanButton).toBeVisible();
      await expect(detailPage.regeneratePlanButton).toBeEnabled();
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 6
  test("every error state exposes a clickable exit (Dismiss, Cancel, or PlanVerdictBox affordance)", async ({
    page,
    request,
  }) => {
    // Call-1 fixture: transitionStatus itself fails, nothing changed.
    const title1 = `send-back-feedback-deadend-call1 ${Date.now()}`;
    let itemId1: string | undefined;
    try {
      itemId1 = await seedReviewItemWithPlan(request, title1);
      await mockRpcFailure(page, "TransitionBacklogItemStatus", "simulated e2e call-1 failure");

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title1);

      await detailPage.sendBackToggle.click();
      await detailPage.sendBackTextarea.fill("dead-end check, call-1 failure");
      await detailPage.sendBackSubmit.click();

      await expect(detailPage.sendBackError).toContainText("Failed to send back");
      await expect(detailPage.sendBackErrorDismiss).toBeEnabled();
      await expect(detailPage.sendBackCancel).toBeEnabled();
    } finally {
      if (itemId1) await archiveItem(request, itemId1);
    }

    // Call-2/3 fixture: transitionStatus+rejectPlan succeed, triggerTriage fails.
    const title2 = `send-back-feedback-deadend-call23 ${Date.now()}`;
    let itemId2: string | undefined;
    try {
      itemId2 = await seedReviewItemWithPlan(request, title2);
      await page.unroute("**/api/session.v1.BacklogService/TransitionBacklogItemStatus");
      await mockRpcFailure(page, "TriggerTriage", "simulated e2e call-2/3 failure");

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title2);

      await detailPage.sendBackToggle.click();
      await detailPage.sendBackTextarea.fill("dead-end check, call-2/3 failure");
      await detailPage.sendBackSubmit.click();

      await expect(detailPage.sendBackError).toContainText("Sent back, but retriage didn't start");
      await expect(detailPage.sendBackErrorDismiss).toBeEnabled();
      await expect(detailPage.sendBackCancel).toBeEnabled();
      await expect(detailPage.regeneratePlanButton).toBeEnabled();
    } finally {
      if (itemId2) await archiveItem(request, itemId2);
    }
  });

  // Retry affordance for the "landed at idea" case — the single
  // most-scrutinized path in this feature (see file header's Story
  // 3.1.1/plan-feedback repair-iteration history). All three send-back RPCs
  // plus GetBacklogItem are mocked with an in-memory item state, because the
  // "ready->idea CAS already committed before the later failure" scenario
  // this exercises is an internal server race that's impractical to
  // reproduce against the real backend, and the Retry's own second
  // transitionStatus call needs the client's believed status ("idea") to
  // stay internally consistent with what the mocked server accepts —
  // letting only some calls hit the real server would desync that CAS
  // precondition (ready->ready is not a valid transition; see
  // session/domain/backlog.go's validTransitions).
  test("Retry after triggerTriage lands the item at idea resubmits the same feedback and succeeds", async ({
    page,
    request,
  }) => {
    const title = `send-back-feedback-retry-idea ${Date.now()}`;
    const feedback = "retry after landing at idea, re-check the mobile layout";
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);

      let status: "review" | "ready" | "idea" = "review";
      let triggerTriageCalls = 0;

      await page.route("**/api/session.v1.BacklogService/TransitionBacklogItemStatus", async (route) => {
        status = "ready";
        await route.fulfill({ json: { item: { id: itemId, title, status } } });
      });
      await page.route("**/api/session.v1.BacklogService/RejectPlan", async (route) => {
        await route.fulfill({ json: { item: { id: itemId, title, status, planRejectionReason: feedback } } });
      });
      await page.route("**/api/session.v1.BacklogService/TriggerTriage", async (route) => {
        triggerTriageCalls += 1;
        if (triggerTriageCalls === 1) {
          // triggerTriage's own internal ready->idea CAS commits before this
          // simulated later-step failure (pre-mortem P1 #1).
          status = "idea";
          await route.fulfill({
            status: 500,
            contentType: "application/json",
            body: JSON.stringify({ code: "internal", message: "simulated e2e triage-after-idea-CAS failure" }),
          });
        } else {
          status = "ready";
          await route.fulfill({ json: { itemSession: { id: "e2e-fake-retry-session" } } });
        }
      });
      await page.route("**/api/session.v1.BacklogService/GetBacklogItem", async (route) => {
        const response = await route.fetch();
        const json = await response.json();
        if (json?.item?.id === itemId) json.item.status = status;
        await route.fulfill({ response, json });
      });

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await detailPage.sendBackToggle.click();
      await detailPage.sendBackTextarea.fill(feedback);
      await detailPage.sendBackSubmit.click();

      await expect(detailPage.sendBackError).toContainText("Sent back, but retriage didn't start");
      await expect(detailPage.sendBackError).toContainText('moved back to "idea"');
      await expect(detailPage.sendBackErrorRetry).toBeVisible();
      await expect(detailPage.sendBackTextarea).toHaveValue(feedback);

      await detailPage.sendBackErrorRetry.click();

      await expect(detailPage.sendBackError).toHaveCount(0);
      await expect(detailPage.toast).toContainText("Feedback sent — retriage started.");
      expect(triggerTriageCalls).toBe(2);
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 7
  test("Submit stays enabled when activeWorkSessionCount > 0 and InlineNotice is shown", async ({
    page,
    request,
  }) => {
    const title = `send-back-feedback-active-session ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await createInProgressItem(request, title);
      const { injected } = await injectItemSessions(page, itemId, [
        { id: "fake-work-session-1", sessionUuid: "fake-work-session-uuid-1", sessionRole: "work" },
      ]);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);
      await injected;

      await detailPage.sendBackToggle.click();
      await expect(detailPage.sendBackActiveSessionNotice).toBeVisible();
      await expect(detailPage.sendBackSubmit).toBeDisabled();

      await detailPage.sendBackTextarea.fill("feedback with an active session in flight");
      await expect(detailPage.sendBackSubmit).toBeEnabled();
      await expect(detailPage.sendBackSubmit).toHaveAttribute("aria-disabled", "false");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 8
  test("toggle button reflects aria-expanded and is operable via Tab + Enter/Space", async ({ page, request }) => {
    const title = `send-back-feedback-keyboard-toggle ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await expect(detailPage.sendBackToggle).toHaveAttribute("aria-expanded", "false");

      await detailPage.sendBackToggle.focus();
      await expect(detailPage.sendBackToggle).toBeFocused();

      await page.keyboard.press("Enter");
      await expect(detailPage.sendBackToggle).toHaveAttribute("aria-expanded", "true");
      await expect(detailPage.sendBackTextarea).toBeVisible();

      await page.keyboard.press("Escape");
      await expect(detailPage.sendBackToggle).toHaveAttribute("aria-expanded", "false");
      await expect(detailPage.sendBackToggle).toBeFocused();

      await page.keyboard.press("Space");
      await expect(detailPage.sendBackToggle).toHaveAttribute("aria-expanded", "true");
      await expect(detailPage.sendBackTextarea).toBeVisible();
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 10
  test("textarea is labelled via a true <label htmlFor> pointing at its id", async ({ page, request }) => {
    const title = `send-back-feedback-true-label ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await detailPage.sendBackToggle.click();

      const byLabel = page.getByLabel("What should change? (required)");
      await expect(byLabel).toBeVisible();
      await expect(byLabel).toHaveAttribute("data-testid", "send-back-feedback-textarea");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 11
  test("Submit button keeps aria-busy and aria-disabled/disabled in sync across idle/pending/resolved", async ({
    page,
    request,
  }) => {
    const title = `send-back-feedback-busy-sync ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);
      // Delay the real TransitionBacklogItemStatus response to make the
      // pending frame observable, then let it fail so the form stays
      // mounted for a "resolved" frame to assert against too.
      await mockRpcFailure(page, "TransitionBacklogItemStatus", "simulated e2e busy-sync failure", 700);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await detailPage.sendBackToggle.click();

      // Idle, empty textarea.
      await expect(detailPage.sendBackSubmit).toBeDisabled();
      await expect(detailPage.sendBackSubmit).toHaveAttribute("aria-busy", "false");

      await detailPage.sendBackTextarea.fill("feedback for the busy-state check");
      await expect(detailPage.sendBackSubmit).toBeEnabled();
      await expect(detailPage.sendBackSubmit).toHaveAttribute("aria-disabled", "false");

      await detailPage.sendBackSubmit.click();

      // Pending: aria-busy and disabled/aria-disabled set together.
      await expect(detailPage.sendBackSubmit).toHaveAttribute("aria-busy", "true");
      await expect(detailPage.sendBackSubmit).toBeDisabled();
      await expect(detailPage.sendBackSubmit).toHaveAttribute("aria-disabled", "true");

      // Resolved (error — form stays mounted with the failure): both cleared.
      await expect(detailPage.sendBackSubmit).toHaveAttribute("aria-busy", "false");
      await expect(detailPage.sendBackSubmit).toBeEnabled();
      await expect(detailPage.sendBackSubmit).toHaveAttribute("aria-disabled", "false");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 12
  test("InlineError region uses role=alert and aria-live=assertive", async ({ page, request }) => {
    const title = `send-back-feedback-alert-live ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);
      await mockRpcFailure(page, "TransitionBacklogItemStatus", "simulated e2e alert-live failure");

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await detailPage.sendBackToggle.click();
      await detailPage.sendBackTextarea.fill("alert-live check");
      await detailPage.sendBackSubmit.click();

      // page.getByRole("alert") resolving to this element (via sendBackError,
      // scoped to the send-back form) already proves role=alert.
      await expect(detailPage.sendBackError).toBeVisible();
      await expect(detailPage.sendBackError).toHaveAttribute("aria-live", "assertive");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 15
  test("all interactive elements are reachable in a single forward Tab sequence matching visual order", async ({
    page,
    request,
  }) => {
    const title = `send-back-feedback-tab-order ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await createInProgressItem(request, title);
      const { injected } = await injectItemSessions(page, itemId, [
        { id: "fake-work-session-tab", sessionUuid: "fake-work-session-uuid-tab", sessionRole: "work" },
      ]);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);
      await injected;

      await detailPage.sendBackToggle.focus();
      await expect(detailPage.sendBackToggle).toBeFocused();

      await page.keyboard.press("Enter");
      await expect(detailPage.sendBackActiveSessionNotice).toBeVisible();

      // See file header: the active-session InlineNotice has no dismiss
      // button here, and the textarea auto-focuses on open — so it's
      // already the focused element rather than a further Tab stop.
      await expect(detailPage.sendBackTextarea).toBeFocused();
      // Submit is `disabled` (and so unreachable by Tab) until text is
      // typed — fill it in place, without moving focus, so the rest of
      // the sequence is actually tabbable.
      await detailPage.sendBackTextarea.fill("feedback for the tab-order check");

      await page.keyboard.press("Tab");
      await expect(detailPage.sendBackCancel).toBeFocused();

      await page.keyboard.press("Tab");
      await expect(detailPage.sendBackSubmit).toBeFocused();
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  // Criterion 17
  test("the unrelated 'Return to Triage' (send_back_idea) button is unchanged and still functions", async ({
    page,
    request,
  }) => {
    const title = `send-back-feedback-idea-unchanged ${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await seedReviewItemWithPlan(request, title);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForItemCards();

      const detailPage = new BacklogItemDetailPage(page);
      await detailPage.openItemByTitle(title);

      await expect(detailPage.sendBackIdeaButton).toBeVisible();
      await expect(detailPage.sendBackIdeaButton).toContainText("↩ Return to Triage");

      await detailPage.sendBackIdeaButton.click();

      await expect(page.getByTestId("stage-tracker")).toHaveAttribute("aria-label", "Lifecycle stage: Idea");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });
});
