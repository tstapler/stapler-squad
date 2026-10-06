import { Page, Locator } from "@playwright/test";

// The 3 stage roles PipelineModeForm.tsx's StageExecutorTable renders as rows
// (session.StageRole's consts) — mirrored here rather than imported since
// page helpers have no access to the web-app's TS module graph.
export type StageRole = "triage" | "review" | "work";

/**
 * Page helper for the Pipeline Modes management page (/settings/pipeline-modes),
 * built in Epic 3.3 of project_plans/backlog-configurable-pipeline. Mirrors
 * BacklogSourcesSettingsPage's structure for the closest existing CRUD-settings
 * precedent in this suite.
 *
 * The stage-executor accessors below (Epic 5.1 of
 * project_plans/backlog-stage-execution-costs) target the exact
 * `data-testid`s PipelineModeForm.tsx and PipelineModeForm.test.tsx already
 * use — see `pipeline-mode-stage-${role}-{program,model}` and
 * `pipeline-mode-stage-${role}-error`.
 */
export class PipelineModesSettingsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto(
      (process.env.TEST_SERVER_URL ?? "http://localhost:8544") + "/settings/pipeline-modes",
      { waitUntil: "domcontentloaded", timeout: 15000 }
    );
    await this.page.waitForSelector('[data-testid="pipeline-mode-new"]', { timeout: 10000 });
  }

  async openNewModeForm() {
    await this.page.getByTestId("pipeline-mode-new").click();
    await this.page.waitForSelector('[data-testid="pipeline-mode-form"]', { timeout: 5000 });
  }

  /**
   * Fills and submits the create form. `templateFields` keys must match the
   * `f.key` values in PipelineModeForm.tsx's CONTENT_FIELDS (e.g.
   * "triagePromptTemplate", "reviewPromptTemplate") — only the fields present
   * in the map are filled, the rest are left blank.
   */
  async createMode(opts: {
    slug: string;
    name: string;
    description?: string;
    templateFields?: Record<string, string>;
  }) {
    await this.openNewModeForm();
    await this.page.getByTestId("pipeline-mode-slug").fill(opts.slug);
    await this.page.getByTestId("pipeline-mode-name").fill(opts.name);
    if (opts.description) {
      await this.page.getByTestId("pipeline-mode-description").fill(opts.description);
    }
    for (const [key, value] of Object.entries(opts.templateFields ?? {})) {
      await this.page.getByTestId(`pipeline-mode-field-${key}`).fill(value);
    }
    await this.page.getByTestId("pipeline-mode-submit").click();
  }

  row(slug: string): Locator {
    return this.page.getByTestId(`pipeline-mode-row-${slug}`);
  }

  /** Opens the edit form for an existing mode's row, via its Edit button. */
  async openEditModeForm(slug: string) {
    await this.page.getByTestId(`pipeline-mode-edit-${slug}`).click();
    await this.page.waitForSelector('[data-testid="pipeline-mode-form"]', { timeout: 5000 });
  }

  nameInput(): Locator {
    return this.page.getByTestId("pipeline-mode-name");
  }

  descriptionInput(): Locator {
    return this.page.getByTestId("pipeline-mode-description");
  }

  stageProgramInput(role: StageRole): Locator {
    return this.page.getByTestId(`pipeline-mode-stage-${role}-program`);
  }

  stageModelInput(role: StageRole): Locator {
    return this.page.getByTestId(`pipeline-mode-stage-${role}-model`);
  }

  /** Fills a stage row's Program and/or Model field, leaving the other untouched when omitted. */
  async setStageExecutor(role: StageRole, values: { program?: string; model?: string }) {
    if (values.program !== undefined) {
      await this.stageProgramInput(role).fill(values.program);
    }
    if (values.model !== undefined) {
      await this.stageModelInput(role).fill(values.model);
    }
  }

  stageError(role: StageRole): Locator {
    return this.page.getByTestId(`pipeline-mode-stage-${role}-error`);
  }

  formError(): Locator {
    return this.page.getByTestId("pipeline-mode-error");
  }

  forceUnknownModelCheckbox(): Locator {
    return this.page.getByTestId("pipeline-mode-force-unknown-model");
  }

  stageExecutorTable(): Locator {
    return this.page.getByTestId("pipeline-mode-stage-executor-table");
  }

  formLocator(): Locator {
    return this.page.getByTestId("pipeline-mode-form");
  }

  async save() {
    await this.page.getByTestId("pipeline-mode-submit").click();
  }
}
