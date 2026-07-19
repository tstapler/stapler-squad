import { Page, Locator } from "@playwright/test";

/**
 * Page helper for the Pipeline Modes management page (/settings/pipeline-modes),
 * built in Epic 3.3 of project_plans/backlog-configurable-pipeline. Mirrors
 * BacklogSourcesSettingsPage's structure for the closest existing CRUD-settings
 * precedent in this suite.
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
}
