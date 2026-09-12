import { Page, Locator, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

export interface TaggingRuleFormInput {
  matchField?: "name" | "branch" | "path" | "program";
  pattern: string;
  outputTag: string;
  name: string;
  priority?: number;
}

/**
 * Page object for the "Tagging Rules" tab (design/ux.md Surfaces 4/5/6) — a source-filter
 * tab alongside the existing approval-rule tabs on `/rules`
 * (web-app/src/components/sessions/ApprovalRulesPanel.tsx renders `TaggingRulesPanel` when
 * it's active), plus its create/edit form
 * (web-app/src/components/rules/TaggingRuleBuilderForm.tsx).
 *
 * Locators use `data-testid`/ARIA roles only, per `e2e-test-conventions`. Table rows use
 * `getByRole("row")` (an HTML `<table>` row's implicit ARIA role) rather than any CSS
 * selector.
 */
export class TaggingRulesPage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  async goto() {
    await this.page.goto(`${BASE_URL}/rules`, { waitUntil: "domcontentloaded" });
  }

  getTab(): Locator {
    return this.page.getByTestId("tagging-rules-tab");
  }

  getPanel(): Locator {
    return this.page.getByTestId("tagging-rules-panel");
  }

  async openTab() {
    await this.getTab().click();
    await expect(this.getPanel()).toBeVisible();
  }

  getEmptyState(): Locator {
    return this.page.getByTestId("tagging-rules-empty-state");
  }

  getAddButton(): Locator {
    return this.page.getByTestId("add-tagging-rule-button");
  }

  getNameInput(): Locator {
    return this.page.getByTestId("tagging-rule-name-input");
  }

  getMatchFieldSelect(): Locator {
    return this.page.getByTestId("tagging-rule-match-field-select");
  }

  getPatternInput(): Locator {
    return this.page.getByTestId("tagging-rule-pattern-input");
  }

  getPatternError(): Locator {
    return this.page.getByTestId("tagging-rule-pattern-error");
  }

  getOutputTagInput(): Locator {
    return this.page.getByTestId("tagging-rule-output-tag-input");
  }

  getSaveButton(): Locator {
    return this.page.getByRole("button", { name: /^Save Rule|Saving…$/ });
  }

  getCancelButton(): Locator {
    return this.page.getByRole("button", { name: "Cancel" });
  }

  /** A rule's table row, located by its rendered name. */
  getRow(name: string): Locator {
    return this.page.getByRole("row").filter({ hasText: name });
  }

  getEditButton(name: string): Locator {
    return this.page.getByRole("button", { name: `Edit tagging rule ${name}` });
  }

  getDeleteButton(name: string): Locator {
    return this.page.getByRole("button", { name: `Delete tagging rule ${name}` });
  }

  /** Matches whichever of "Enable"/"Disable tagging rule" is currently rendered. */
  getToggleButton(name: string): Locator {
    return this.getRow(name).getByRole("button", { name: /^(Enable|Disable) tagging rule$/ });
  }

  /**
   * The "Fires (7d)" column header — carries the reused approval-rule tooltip verbatim.
   * Uses `data-testid` rather than `getByRole("columnheader", ...)`: the table's CSS
   * strips the `<th>`'s implicit ARIA role (confirmed via Playwright's accessibility
   * snapshot — it resolves to a plain "cell"), so a role locator would never match.
   */
  getFireCountHeader(): Locator {
    return this.page.getByTestId("tagging-rule-fire-count-header");
  }

  /** Fills out and saves the rule builder form. Does not open the form or the tab first. */
  async fillAndSave(input: TaggingRuleFormInput) {
    if (input.matchField) {
      await this.getMatchFieldSelect().selectOption(input.matchField);
    }
    await this.getPatternInput().fill(input.pattern);
    await this.getOutputTagInput().fill(input.outputTag);
    await this.getNameInput().fill(input.name);
    if (input.priority !== undefined) {
      await this.page.locator('input[type="number"]').fill(String(input.priority));
    }
    await this.getSaveButton().click();
  }
}
