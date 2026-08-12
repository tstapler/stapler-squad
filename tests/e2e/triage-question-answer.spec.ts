// @feature backlog:trigger-triage, triage-question-answer
/**
 * E2E coverage for Gap 1 (project_plans/backlog-operator-feedback-loop, Epic
 * 1) — answering a triage clarifying question inline (no retyping the whole
 * feedback), which composes a "Q:.../A:..." string and fires a re-triage via
 * TriggerTriage.
 *
 * A real triage run (real headless Claude subprocess call) is slow/flaky/
 * costly to run in CI — following the established convention in this suite
 * (see backlog-pipeline-mode.spec.ts's file-level comment and
 * crashed-session-ux.spec.ts), this spec creates a real "idea"-status
 * backlog item via the API (skipTriage: true, so CreateBacklogItem never
 * kicks off a real triage run) and intercepts the real GetBacklogItem
 * response to inject a fabricated completed-triage ItemSession — role
 * "triage", endedAt set, TriageResult with a mix of AC suggestions and
 * "question"-rationale suggestions (TriageDiffSection.tsx filters
 * questionSuggestions by rationale === "question"; note it only renders the
 * diff section at all when at least one non-question suggestion exists —
 * see TriageReviewPanel.tsx's hasSuggestions gate). TriggerTriage itself is
 * separately intercepted so answering a question never triggers a real LLM
 * call.
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

async function createIdeaItem(request: APIRequestContext, title: string): Promise<string> {
  const res = await request.post(`${BASE_URL}/api/session.v1.BacklogService/CreateBacklogItem`, {
    headers: { "Content-Type": "application/json" },
    data: { title, priority: 3, repoPath: "", skipTriage: true },
  });
  if (!res.ok()) {
    throw new Error(`CreateBacklogItem failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { item?: { id: string } };
  if (!body.item?.id) throw new Error("CreateBacklogItem did not return an item id");
  return body.item.id;
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

/**
 * Intercepts the real GetBacklogItem response for `itemId` and injects a
 * completed triage ItemSession carrying `questions` (rendered as
 * "question"-rationale TriageSuggestion entries, in order — their index
 * within TriageDiffSection's filtered questionSuggestions array is what
 * `triage-question-answer-toggle-<i>` keys off) plus one ordinary AC
 * suggestion so TriageReviewPanel's hasSuggestions gate lets the diff
 * section (and therefore the questions block) render at all.
 */
async function injectCompletedTriageWithQuestions(page: Page, itemId: string, questions: string[]) {
  await page.route("**/api/session.v1.BacklogService/GetBacklogItem", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    if (json?.item?.id === itemId) {
      json.item.itemSessions = [
        ...(json.item.itemSessions ?? []),
        {
          id: "e2e-fake-triage-session-entity",
          sessionUuid: "e2e-fake-triage-session-uuid",
          sessionRole: "triage",
          endedAt: new Date().toISOString(),
          triageResult: {
            summary: "Found 1 suggestion and " + questions.length + " clarifying question(s).",
            suggestions: [
              { text: "Add a regression test for the new path", rationale: "coverage" },
              ...questions.map((q) => ({ text: q, rationale: "question" })),
            ],
            clarifyingQuestions: [],
            tasks: [],
            iteration: 1,
            feedback: "",
          },
          estimatedCostUsd: 0,
        },
      ];
    }
    await route.fulfill({ response, json });
  });
}

test.describe("triage-question-answer", () => {
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
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test("answers a triage question without retyping and triggers re-triage", async ({ page, request }) => {
    const itemTitle = `triage-answer-${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await createIdeaItem(request, itemTitle);
      await injectCompletedTriageWithQuestions(page, itemId, ["What database should this use?"]);

      // TriggerTriage would otherwise spawn a real headless Claude call —
      // fulfill a canned success response so answering a question stays fast
      // and deterministic (see file header).
      const triggerTriageRequest = page.waitForRequest(
        (req) =>
          req.url().includes("/api/session.v1.BacklogService/TriggerTriage") && req.method() === "POST"
      );
      await page.route("**/api/session.v1.BacklogService/TriggerTriage", async (route) => {
        await route.fulfill({ json: { itemSession: { id: "e2e-fake-retriage-session" } } });
      });

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForPageLoad();
      await backlogPage.openItemDetail(itemTitle);
      await expect(backlogPage.getItemDetailPane()).toBeVisible();

      const toggle = page.getByTestId("triage-question-answer-toggle-0");
      // The injected GetBacklogItem response (route.fetch() + mutate +
      // route.fulfill()) adds a full extra network round trip to the
      // detail pane's first load — under load this occasionally exceeds
      // the default 5s assertion timeout even though the element does
      // arrive; a longer timeout here avoids a flaky false negative
      // without masking a real absence.
      await expect(toggle).toBeVisible({ timeout: 10000 });
      await toggle.click();

      const input = page.getByTestId("triage-question-answer-input-0");
      await input.fill("Postgres");

      const submit = page.getByTestId("triage-question-answer-submit-0");
      await expect(submit).toBeEnabled();
      await submit.click();

      const request1 = await triggerTriageRequest;
      const body = request1.postDataJSON() as { itemId?: string; feedback?: string };
      expect(body.itemId).toBe(itemId);
      expect(body.feedback).toContain("What database should this use?");
      expect(body.feedback).toContain("Postgres");

      await expect(page.getByText("✓ Answered: Postgres")).toBeVisible();
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("keeps the answer visible and surfaces an error when re-triage is already in flight", async ({
    page,
    request,
  }) => {
    const itemTitle = `triage-answer-inflight-${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await createIdeaItem(request, itemTitle);
      await injectCompletedTriageWithQuestions(page, itemId, ["Should this support offline mode?"]);

      // Simulates "a re-triage is already in flight" by having the real
      // TriggerTriage call this answer submission drives fail server-side —
      // the exact branch TriageDiffSection's handleSubmit catch block
      // exists for (form stays open, draft text preserved, inline error
      // shown) regardless of which precondition made the call fail.
      await page.route("**/api/session.v1.BacklogService/TriggerTriage", async (route) => {
        await route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify({
            code: "failed_precondition",
            message: "a triage run is already in progress for this item",
          }),
        });
      });

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForPageLoad();
      await backlogPage.openItemDetail(itemTitle);
      await expect(backlogPage.getItemDetailPane()).toBeVisible();

      await page.getByTestId("triage-question-answer-toggle-0").click();
      const input = page.getByTestId("triage-question-answer-input-0");
      await input.fill("Yes, offline-first for the mobile client.");
      await page.getByTestId("triage-question-answer-submit-0").click();

      const errorBanner = page.getByTestId("triage-error-banner");
      await expect(errorBanner).toBeVisible();
      await expect(errorBanner).toHaveAttribute("role", "alert");

      // The form never closed — input-0 is still rendered with the typed
      // draft intact (a silent no-op / lost answer would instead have
      // closed the form or cleared the draft).
      await expect(input).toBeVisible();
      await expect(input).toHaveValue("Yes, offline-first for the mobile client.");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("returns focus to the Answer toggle when Cancel is pressed", async ({ page, request }) => {
    const itemTitle = `triage-answer-cancel-${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await createIdeaItem(request, itemTitle);
      await injectCompletedTriageWithQuestions(page, itemId, ["What database should this use?"]);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForPageLoad();
      await backlogPage.openItemDetail(itemTitle);
      await expect(backlogPage.getItemDetailPane()).toBeVisible();

      await page.getByTestId("triage-question-answer-toggle-0").click();
      await page.getByTestId("triage-question-answer-input-0").fill("partial draft");
      await page.getByTestId("triage-question-answer-cancel-0").click();

      await expect(page.getByTestId("triage-question-answer-toggle-0")).toBeVisible();
      const activeTestId = await page.evaluate(() => document.activeElement?.getAttribute("data-testid"));
      expect(activeTestId).toBe("triage-question-answer-toggle-0");
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });

  test("keeps Submit disabled until an answer is typed", async ({ page, request }) => {
    const itemTitle = `triage-answer-empty-${Date.now()}`;
    let itemId: string | undefined;

    try {
      itemId = await createIdeaItem(request, itemTitle);
      await injectCompletedTriageWithQuestions(page, itemId, ["What database should this use?"]);

      let triggerTriageCalled = false;
      await page.route("**/api/session.v1.BacklogService/TriggerTriage", async (route) => {
        triggerTriageCalled = true;
        await route.fulfill({ json: { itemSession: { id: "e2e-fake-retriage-session" } } });
      });

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForPageLoad();
      await backlogPage.openItemDetail(itemTitle);
      await expect(backlogPage.getItemDetailPane()).toBeVisible();

      await page.getByTestId("triage-question-answer-toggle-0").click();
      const submit = page.getByTestId("triage-question-answer-submit-0");
      await expect(submit).toHaveAttribute("aria-disabled", "true");
      // A real `disabled` DOM attribute (not just aria-disabled) backs this
      // button — Playwright refuses to click a genuinely disabled element,
      // which is itself proof a real user cannot fire the RPC from here.
      await expect(submit).toBeDisabled();

      expect(triggerTriageCalled).toBe(false);
    } finally {
      if (itemId) await archiveItem(request, itemId);
    }
  });
});
