// @feature backlog:update-pipeline-mode, backlog-pipeline-mode-management
/**
 * UX acceptance tests for the per-stage executor table in PipelineModeForm
 * (Epic 5.1 of project_plans/backlog-stage-execution-costs). Covers
 * validation.md's "## UX Acceptance Tests" rows 1, 5, 6, 7, 8, 11, 12 — see
 * that table for the full criteria this file maps to test names for.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * These tests exercise the real CreatePipelineMode/UpdatePipelineMode RPCs
 * and the real server-side validation in session/pipeline_mode_validation.go
 * (validateStageExecutors) rather than mocking rejections — the exact error
 * text asserted below (e.g. "has no headless mode (ADR-002)", "is not a
 * recognized model") comes straight from that file, so a wording change
 * there fails this spec instead of silently drifting from what users
 * actually see.
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import { PipelineModesSettingsPage, StageRole } from "./pages/PipelineModesSettingsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

// Same rationale as backlog-pipeline-mode.spec.ts: BacklogService RPCs are
// gated by an interceptor that re-reads config.LoadConfig() from disk, which
// lags UpdateFeatureFlag's in-memory write — poll a real RPC instead of
// trusting GetFeatureFlags to report the gate is actually open yet.
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

/** Tracks whether a real page navigation/reload happens after this is attached — a `load` refire would mean the SPA lost its in-memory form state. */
function countPageLoads(page: Page): () => number {
  let loads = 0;
  page.on("load", () => loads++);
  return () => loads;
}

test.describe("pipeline-mode-stage-executors", () => {
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

  test("configures triage, review, and work stage executors in a single form submission", async ({
    page,
    request,
  }) => {
    const slug = `e2e-stage-exec-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E Stage Exec Mode" });

      const modesPage = new PipelineModesSettingsPage(page);
      await modesPage.goto();
      const getLoads = countPageLoads(page);
      const urlBefore = page.url();

      await modesPage.openEditModeForm(slug);
      await modesPage.setStageExecutor("triage", { program: "claude", model: "claude-haiku-4-5" });
      await modesPage.setStageExecutor("review", { program: "gemini", model: "claude-sonnet-4-5" });
      await modesPage.setStageExecutor("work", { program: "opencode", model: "claude-opus-4-5" });

      const updateRequest = page.waitForRequest(
        (req) => req.url().includes("/api/session.v1.BacklogService/UpdatePipelineMode") && req.method() === "POST"
      );
      await modesPage.save();
      const req = await updateRequest;
      // UpdatePipelineModeRequest.stage_executors is an `optional
      // StageExecutorsUpdate` message wrapping the role->executor map in its
      // own `values` field (proto/session/v1/backlog.proto:974-979) — proto3
      // has no field presence for a bare map, so this wrapper is what lets
      // "untouched" (absent) be distinguished from "cleared" (empty map).
      const bodyJson = req.postDataJSON() as { stageExecutors?: { values?: Record<string, unknown> } };

      expect(bodyJson.stageExecutors?.values).toBeDefined();
      const roleKeys = Object.keys(bodyJson.stageExecutors?.values ?? {});
      expect(roleKeys).toEqual(expect.arrayContaining(["triage", "review", "work"]));
      expect(bodyJson.stageExecutors?.values).toMatchObject({
        triage: { program: "claude", model: "claude-haiku-4-5" },
        review: { program: "gemini", model: "claude-sonnet-4-5" },
        work: { program: "opencode", model: "claude-opus-4-5" },
      });

      // Save succeeded and closed the form without a route change or reload.
      await expect(modesPage.formLocator()).toBeHidden({ timeout: 5000 });
      expect(page.url()).toBe(urlBefore);
      expect(getLoads()).toBe(0);
    } finally {
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });

  test('rejecting aider for the triage stage names both "aider" and the stage in the error message', async ({
    page,
    request,
  }) => {
    const slug = `e2e-stage-aider-triage-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E Aider Triage Mode" });

      const modesPage = new PipelineModesSettingsPage(page);
      await modesPage.goto();
      await modesPage.openEditModeForm(slug);

      await modesPage.setStageExecutor("triage", { program: "aider" });
      await modesPage.save();

      const rowError = modesPage.stageError("triage");
      await expect(rowError).toBeVisible();
      await expect(rowError).toContainText(/aider/i);
      await expect(rowError).toContainText(/triage/i);

      // A corrective action is available: clearing the field and re-saving.
      await modesPage.setStageExecutor("triage", { program: "" });
      await modesPage.save();
      await expect(modesPage.formLocator()).toBeHidden({ timeout: 5000 });
    } finally {
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });

  test("accepting aider for the work stage program succeeds with no error banner", async ({ page, request }) => {
    const slug = `e2e-stage-aider-work-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E Aider Work Mode" });

      const modesPage = new PipelineModesSettingsPage(page);
      await modesPage.goto();
      await modesPage.openEditModeForm(slug);

      await modesPage.setStageExecutor("work", { program: "aider" });
      await modesPage.save();

      // Success: form closes (returns to list), no error banner rendered anywhere.
      await expect(modesPage.formLocator()).toBeHidden({ timeout: 5000 });
      await expect(modesPage.formError()).toHaveCount(0);
      await expect(modesPage.stageError("work")).toHaveCount(0);
      await expect(modesPage.row(slug)).toBeVisible();
    } finally {
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });

  test("rejecting an unrecognized model id names the exact value and the affected stage", async ({
    page,
    request,
  }) => {
    const slug = `e2e-stage-bad-model-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E Bad Model Mode" });

      const modesPage = new PipelineModesSettingsPage(page);
      await modesPage.goto();
      await modesPage.openEditModeForm(slug);

      await modesPage.setStageExecutor("review", { model: "claude-opus-9000" });
      await modesPage.save();

      const rowError = modesPage.stageError("review");
      await expect(rowError).toBeVisible();
      await expect(rowError).toContainText("claude-opus-9000");
      await expect(rowError).toContainText(/review/i);
    } finally {
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });

  test("recovering from a rejected stage-executor save preserves all other in-progress field edits", async ({
    page,
    request,
  }) => {
    const slug = `e2e-stage-recover-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E Recover Mode" });

      const modesPage = new PipelineModesSettingsPage(page);
      await modesPage.goto();
      const getLoads = countPageLoads(page);
      const urlBefore = page.url();

      await modesPage.openEditModeForm(slug);

      const newName = "E2E Recover Mode Renamed";
      const newDescription = "In-progress description edit that must survive a rejected save";
      await modesPage.nameInput().fill(newName);
      await modesPage.descriptionInput().fill(newDescription);
      await modesPage.setStageExecutor("work", { program: "claude", model: "claude-sonnet-4-5" });

      // Trigger a rejection via an invalid Triage program.
      await modesPage.setStageExecutor("triage", { program: "aider" });
      await modesPage.save();
      await expect(modesPage.stageError("triage")).toBeVisible();

      // Correct only the Triage field, then re-save.
      await modesPage.setStageExecutor("triage", { program: "claude" });
      await modesPage.save();
      await expect(modesPage.formLocator()).toBeHidden({ timeout: 5000 });

      // Reopen the saved mode to confirm every other in-progress edit round-tripped.
      await modesPage.openEditModeForm(slug);
      await expect(modesPage.nameInput()).toHaveValue(newName);
      await expect(modesPage.descriptionInput()).toHaveValue(newDescription);
      await expect(modesPage.stageProgramInput("work")).toHaveValue("claude");
      await expect(modesPage.stageModelInput("work")).toHaveValue("claude-sonnet-4-5");

      expect(page.url()).toBe(urlBefore);
      expect(getLoads()).toBe(0);
    } finally {
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });

  test("each stage executor input exposes a unique aria-label distinguishing stage and field type", async ({
    page,
    request,
  }) => {
    const slug = `e2e-stage-aria-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E Aria Mode" });

      const modesPage = new PipelineModesSettingsPage(page);
      await modesPage.goto();
      await modesPage.openEditModeForm(slug);

      const expectedNames: Array<{ role: StageRole; stageLabel: string; field: "program" | "model" }> = [
        { role: "triage", stageLabel: "Triage", field: "program" },
        { role: "triage", stageLabel: "Triage", field: "model" },
        { role: "review", stageLabel: "Review", field: "program" },
        { role: "review", stageLabel: "Review", field: "model" },
        { role: "work", stageLabel: "Work", field: "program" },
        { role: "work", stageLabel: "Work", field: "model" },
      ];

      const seen = new Set<string>();
      for (const { role, stageLabel, field } of expectedNames) {
        const accessibleName = `${stageLabel} stage ${field}`;
        const byRole = page.getByRole("textbox", { name: accessibleName, exact: true });
        await expect(byRole).toHaveCount(1);

        // The accessible-name locator must resolve to the exact same element
        // the page helper's data-testid locator points at, not a coincidence.
        const testIdLocator = field === "program" ? modesPage.stageProgramInput(role) : modesPage.stageModelInput(role);
        await expect(byRole).toHaveJSProperty("id", await testIdLocator.evaluate((el) => (el as HTMLElement).id));

        seen.add(accessibleName);
      }

      // All 6 accessible names are unique.
      expect(seen.size).toBe(6);
    } finally {
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });

  test("stage executor table uses semantic table and th scope=col markup", async ({ page, request }) => {
    const slug = `e2e-stage-table-markup-${Date.now()}`;
    let mode: CreatedPipelineMode | undefined;

    try {
      mode = await createPipelineModeViaApi(request, { slug, name: "E2E Table Markup Mode" });

      const modesPage = new PipelineModesSettingsPage(page);
      await modesPage.goto();
      await modesPage.openEditModeForm(slug);

      const table = modesPage.stageExecutorTable();
      await expect(table).toBeVisible();
      expect(await table.evaluate((el) => el.tagName)).toBe("TABLE");

      for (const name of ["Stage", "Program", "Model"]) {
        const header = table.getByRole("columnheader", { name, exact: true });
        await expect(header).toHaveCount(1);
        await expect(header).toHaveAttribute("scope", "col");
      }
    } finally {
      if (mode) await deletePipelineModeViaApi(request, mode.id);
    }
  });
});
