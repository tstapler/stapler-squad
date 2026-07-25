// @feature backlog:create-pipeline-mode, backlog:update-pipeline-mode, backlog:delete-pipeline-mode, backlog:get-pipeline-mode, backlog:list-pipeline-modes, backlog-pipeline-mode-selector, backlog-pipeline-mode-management
/**
 * E2E tests for the PipelineMode feature (project_plans/backlog-configurable-pipeline,
 * Epic 4.3): the /settings/pipeline-modes management CRUD page (Epic 3.3), the
 * per-item mode selector radio group (Epic 3.2), and the "what ran" read-only
 * surface in BacklogItemDetail (Epic 3.4).
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * Scope note on scenario 3 ("what ran" surface): populating a real
 * ItemSession.pipelineModeSnapshot requires either TriggerTriage (a real
 * headless Claude subprocess call — see backlog_service_triage.go's
 * synchronous ItemSession-create-then-async-LLM-call flow) or SpawnSessionFromItem
 * (a real tmux/git-worktree work session). No existing spec in this suite
 * exercises either RPC — even backlog.spec.ts's "Trigger Triage" coverage only
 * asserts the button's disabled state, never actually clicks it (see its
 * `e2e:backlog-triage-gate-disabled` test) — because both are slow/flaky/costly
 * to run in CI. Consistent with that established convention, this spec instead
 * intercepts the real GetBacklogItem response for a real, API-created backlog
 * item and injects one fabricated ItemSession record with the fields
 * TriggerTriage/SpawnSessionFromItem would themselves have set
 * (pipelineModeSnapshot from item.PipelineMode — see backlog_service_triage.go
 * lines ~375 and ~789). This exercises the real BacklogItemDetail component's
 * real rendering logic (resolvePipelineModeDisplay) in a real browser against
 * the real, already-tested PipelineMode CRUD/selection data — it verifies the
 * frontend "what ran" surface, not the backend snapshot-write path (which has
 * its own Go unit test coverage per Story 3.4.1 / backlog_service_triage_test.go).
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { PipelineModesSettingsPage } from "./pages/PipelineModesSettingsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

// Same rationale as backlog-sources-settings.spec.ts: BacklogService RPCs are
// gated by an interceptor that re-reads config.LoadConfig() from disk, which
// lags UpdateFeatureFlag's in-memory write — poll a real RPC instead of trusting
// GetFeatureFlags to report the gate is actually open yet.
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

interface CreatedPipelineMode {
  id: string;
  slug: string;
  name: string;
}

async function createPipelineModeViaApi(
  request: APIRequestContext,
  opts: { slug: string; name: string; description?: string }
): Promise<CreatedPipelineMode> {
  const res = await request.post(`${BASE_URL}/api/session.v1.BacklogService/CreatePipelineMode`, {
    headers: { "Content-Type": "application/json" },
    data: {
      slug: opts.slug,
      name: opts.name,
      description: opts.description ?? "",
      enabled: true,
    },
  });
  if (!res.ok()) {
    throw new Error(`CreatePipelineMode failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { item: CreatedPipelineMode };
  return body.item;
}

async function deletePipelineModeViaApi(request: APIRequestContext, id: string) {
  await request
    .post(`${BASE_URL}/api/session.v1.BacklogService/DeletePipelineMode`, {
      headers: { "Content-Type": "application/json" },
      data: { id },
    })
    .catch(() => {
      // Best-effort cleanup — do not fail the test on cleanup errors.
    });
}

async function archiveBacklogItemViaApi(request: APIRequestContext, id: string) {
  await request
    .post(`${BASE_URL}/api/session.v1.BacklogService/ArchiveBacklogItem`, {
      headers: { "Content-Type": "application/json" },
      data: { id },
    })
    .catch(() => {
      // Best-effort cleanup — do not fail the test on cleanup errors.
    });
}

/**
 * Intercepts the real GetBacklogItem response for `itemId` and injects one
 * fabricated ItemSession record carrying `pipelineModeSnapshot`, so
 * BacklogItemDetail's "what ran" Pipeline group has something to render
 * without requiring a real TriggerTriage/SpawnSessionFromItem call. See the
 * file-level doc comment for the full rationale.
 */
async function injectFakeSessionWithPipelineSnapshot(page: Page, itemId: string, modeSlug: string) {
  await page.route("**/api/session.v1.BacklogService/GetBacklogItem", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    if (json?.item?.id === itemId) {
      json.item.itemSessions = [
        ...(json.item.itemSessions ?? []),
        {
          id: "e2e-fake-session-entity",
          sessionUuid: "e2e-fake-session-uuid",
          sessionRole: "work",
          pipelineModeSnapshot: modeSlug,
          // Left empty deliberately: resolvePipelineModeDisplay() in
          // BacklogItemDetail.tsx treats an empty snapshot hash as "no drift
          // comparison" (pre-feature/default session), which keeps this
          // scenario focused on "the mode name renders", not drift detection.
          pipelineModeSnapshotHash: "",
          estimatedCostUsd: 0,
        },
      ];
    }
    await route.fulfill({ response, json });
  });
}

test.describe("backlog-pipeline-mode", () => {
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
    // Pre-seed the first-visit tour as dismissed, same as backlog.spec.ts, so it
    // doesn't intercept clicks on the backlog page for the selection scenario.
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test("AC-01 create a pipeline mode via /settings/pipeline-modes and see it in the list", async ({ page }) => {
    const slug = `e2e-quick-${Date.now()}`;
    const modesPage = new PipelineModesSettingsPage(page);
    await modesPage.goto();

    await modesPage.createMode({
      slug,
      name: "E2E Quick Fix",
      description: "Fast, low-ceremony pipeline for small fixes",
      templateFields: {
        triagePromptTemplate: "This is a quick-fix item — skip deep analysis for {{item_id}}.",
        reviewPromptTemplate: "Quick review — only check the smallest correct change.",
      },
    });

    const row = modesPage.row(slug);
    await expect(row).toBeVisible();
    await expect(row).toContainText("E2E Quick Fix");
  });

  test("AC-02 select a pipeline mode on a backlog item and see the selection persist", async ({ page, request }) => {
    const slug = `e2e-select-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;
    let itemId: string | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E Select Mode" });

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForPageLoad();

      const itemTitle = `pipeline-mode-select-${Date.now()}`;
      await backlogPage.openNewItemForm();
      await backlogPage.fillNewItemForm(itemTitle, { repoPath: "/tmp/e2e-pipeline-mode-test" });

      const option = backlogPage.getPipelineModeOption(slug);
      await expect(option).toBeVisible();
      await backlogPage.selectPipelineMode(slug);
      await expect(option).toHaveAttribute("aria-checked", "true");

      await backlogPage.submitNewItemForm();
      await page.waitForSelector('[data-testid="backlog-form-modal"]', { state: "hidden", timeout: 5000 });

      // Capture the created item's id for cleanup, and confirm the selection
      // round-tripped through CreateBacklogItem by re-opening the item in edit
      // mode (BacklogItemForm is reused there with initialValues=item), where
      // the same testid should now render pre-selected.
      const row = backlogPage.getItemCard(itemTitle);
      await expect(row).toBeVisible();
      await backlogPage.openItemDetail(itemTitle);
      await expect(backlogPage.getItemDetailPane()).toBeVisible();

      const editButton = page.locator('[data-testid="backlog-detail-edit"]');
      await editButton.click();
      const editedOption = backlogPage.getPipelineModeOption(slug);
      await expect(editedOption).toHaveAttribute("aria-checked", "true");

      // Recover the item id for cleanup via ListBacklogItems (detail pane itself
      // doesn't expose a raw id testid).
      const listRes = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListBacklogItems`, {
        headers: { "Content-Type": "application/json" },
        data: {},
      });
      const listBody = (await listRes.json()) as { items?: Array<{ id: string; title: string }> };
      itemId = listBody.items?.find((i) => i.title === itemTitle)?.id;
    } finally {
      if (itemId) await archiveBacklogItemViaApi(request, itemId);
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });

  test("AC-03 'what ran' surface shows the pipeline mode name for a session snapshot", async ({ page, request }) => {
    const slug = `e2e-whatran-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;
    let itemId: string | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E What Ran Mode" });

      const itemTitle = `pipeline-mode-what-ran-${Date.now()}`;
      const createRes = await request.post(`${BASE_URL}/api/session.v1.BacklogService/CreateBacklogItem`, {
        headers: { "Content-Type": "application/json" },
        data: { title: itemTitle, priority: 3, repoPath: "", skipTriage: true, pipelineMode: slug },
      });
      const createBody = (await createRes.json()) as { item?: { id: string } };
      itemId = createBody.item?.id;
      if (!itemId) throw new Error("CreateBacklogItem did not return an item id");

      await injectFakeSessionWithPipelineSnapshot(page, itemId, slug);

      const backlogPage = new BacklogPage(page);
      await backlogPage.goto();
      await backlogPage.waitForPageLoad();
      await backlogPage.openItemDetail(itemTitle);
      await expect(backlogPage.getItemDetailPane()).toBeVisible();

      const pipelineGroup = page.getByRole("group", { name: "Pipeline" });
      await expect(pipelineGroup).toBeVisible();
      await expect(pipelineGroup).toContainText("E2E What Ran Mode");
    } finally {
      if (itemId) await archiveBacklogItemViaApi(request, itemId);
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });
});
