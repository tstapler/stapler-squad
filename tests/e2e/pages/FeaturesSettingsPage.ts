import type { Locator, Page } from "@playwright/test";
import { expect } from "../helpers/backlog-settings-mocks";
import { baseUrl } from '../helpers/base-url';

const BASE_URL = baseUrl();

/** Row labels as the Features page renders them (FEATURE_META in flagExtras.tsx). */
export const FLAG_LABELS = {
  shadow: "Workflow engine: shadow (compare only)",
  operator: "Enforce for operators (this UI)",
  agent: "Enforce for agents (MCP tools)",
  system: "Enforce for automation (lifecycle, sync, pollers)",
  diagnose: "Diagnose & Nudge",
  stall: "Autonomous stall nudge",
} as const;

/** DOM ids of the engine rows; the Stages banner links to these. Located by id only through the URL hash. */
export const FLAG_ROW_HASH = {
  shadow: "flag-row-backlog-workflow-engine-shadow",
  operator: "flag-row-backlog-workflow-engine-enforce",
  agent: "flag-row-backlog-workflow-engine-enforce-agent",
  system: "flag-row-backlog-workflow-engine-enforce-system",
} as const;

export type FlagKey = keyof typeof FLAG_LABELS;

/** /settings/features: flag rows, the workflow-engine group and the enforce confirmation dialog. */
export class FeaturesSettingsPage {
  readonly page: Page;
  readonly summary: Locator;
  readonly review: Locator;
  readonly loadError: Locator;
  readonly loading: Locator;
  readonly dialog: Locator;
  readonly dialogCancel: Locator;
  readonly dialogConfirm: Locator;
  readonly dialogOverlay: Locator;
  readonly evidence: Locator;
  readonly evidenceRetry: Locator;
  readonly ack: Locator;
  readonly hint: Locator;

  constructor(page: Page) {
    this.page = page;
    this.summary = page.getByTestId("workflow-engine-summary");
    this.review = page.getByTestId("workflow-engine-review");
    this.loadError = page.getByTestId("feature-flags-load-error");
    this.loading = page.getByTestId("feature-flags-loading");
    this.dialog = page.getByRole("alertdialog");
    this.dialogCancel = page.getByTestId("alert-confirm-cancel");
    this.dialogConfirm = page.getByTestId("alert-confirm-confirm");
    this.dialogOverlay = page.getByTestId("alert-confirm-overlay");
    this.evidence = page.getByTestId("feature-flag-enforce-evidence");
    this.evidenceRetry = page.getByTestId("feature-flag-enforce-evidence-retry");
    this.ack = page.getByTestId("feature-flag-enforce-ack");
    this.hint = page.getByTestId("feature-flag-enforce-hint");
  }

  async goto(hash = ""): Promise<void> {
    await this.page.goto(`${BASE_URL}/settings/features${hash ? `#${hash}` : ""}`, { waitUntil: "domcontentloaded" });
  }

  switchFor(flag: FlagKey): Locator {
    return this.page.getByRole("switch", { name: FLAG_LABELS[flag], exact: true });
  }

  row(flag: FlagKey): Locator {
    return this.page.getByTestId("feature-flag-row").filter({ has: this.switchFor(flag) });
  }

  rowStatus(flag: FlagKey): Locator {
    return this.row(flag).getByTestId("feature-flag-row-status");
  }

  rowError(flag: FlagKey): Locator {
    return this.row(flag).getByTestId("feature-flag-row-error");
  }

  revert(flag: FlagKey): Locator {
    return this.row(flag).getByTestId("feature-flag-revert");
  }

  editStagesLink(flag: FlagKey): Locator {
    return this.row(flag).getByTestId("feature-flag-edit-stages");
  }

  /** Opens the enforce dialog by clicking the class's switch. */
  async openEnforceDialog(flag: "operator" | "agent" | "system"): Promise<void> {
    await this.switchFor(flag).click();
    await this.dialog.waitFor();
  }

  /** Ticks the acknowledgement when the evidence needs one, then activates Enforce. */
  async confirmEnforce(): Promise<void> {
    if (await this.ack.count()) await this.ack.check();
    await this.dialogConfirm.click();
  }

  /** Enforces one class through the real dialog. */
  async enforce(flag: "operator" | "agent" | "system"): Promise<void> {
    await this.openEnforceDialog(flag);
    await expect(this.evidence).not.toHaveAttribute("aria-busy", "true");
    await this.confirmEnforce();
    await this.dialog.waitFor({ state: "detached" });
  }
}
