import { APIRequestContext, Page, Locator } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/**
 * Page object for the redesigned backlog item detail panel
 * (project_plans/backlog-item-detail-ux, Epic 6.1 / Story 6.1.2) — the
 * always-visible Lifecycle Summary header, the collapsible sibling sections
 * built in Epic 3 (CollapsibleSection headers, one per section, sharing a
 * page-level CollapsibleGroup per Task 3.1.4i), and the per-row diagnostic
 * panel for synthetic sessions wired in Epic 4.
 */
export class BacklogItemDetailPage {
  readonly page: Page;
  readonly pane: Locator;
  readonly lifecycleSummary: Locator;
  readonly pipelineBadge: Locator;

  constructor(page: Page) {
    this.page = page;
    this.pane = page.getByTestId("backlog-item-detail");
    this.lifecycleSummary = page.getByTestId("lifecycle-summary");
    this.pipelineBadge = page.getByTestId("lifecycle-pipeline-badge");
  }

  /**
   * A top-level section's Collapsible header — an
   * `Accordion.Trigger`-backed `<button aria-expanded="true|false">`
   * (web-app/src/components/ui/Collapsible.tsx), located by its
   * `data-testid="collapsible-header-<sectionKey>"` (never by CSS class,
   * per .claude/rules/e2e-test-conventions.md). `sectionKey` matches the
   * `sectionKey` prop each extracted section passes, e.g. "sessions",
   * "version-control", "description".
   */
  sectionHeader(sectionKey: string): Locator {
    return this.page.getByTestId(`collapsible-header-${sectionKey}`);
  }

  async isSectionExpanded(sectionKey: string): Promise<boolean> {
    const expanded = await this.sectionHeader(sectionKey).getAttribute("aria-expanded");
    return expanded === "true";
  }

  async expandSection(sectionKey: string) {
    const header = this.sectionHeader(sectionKey);
    if ((await header.getAttribute("aria-expanded")) !== "true") {
      await header.click();
    }
  }

  async collapseSection(sectionKey: string) {
    const header = this.sectionHeader(sectionKey);
    if ((await header.getAttribute("aria-expanded")) === "true") {
      await header.click();
    }
  }

  /**
   * Locates a synthetic session row's own Collapsible header inside the
   * Sessions section by its accessible name (the session id text rendered
   * in the trigger, per SessionsSection.tsx) rather than by the row's
   * internal `sectionKey` (`session-<entityId>`), whose id is only known
   * at seed time and isn't guaranteed stable across a real DB round-trip.
   */
  syntheticSessionRow(sessionId: string): Locator {
    return this.page.getByRole("button", { name: new RegExp(sessionId) });
  }

  /**
   * The read-only diagnostic panel revealed inside an expanded synthetic
   * session row — `SessionDiagnosticPanel` (data-testid
   * "session-diagnostic-panel"), wrapping either `TriageReviewPanel
   * readOnly` (data-testid "triage-review-panel") or `GateVerdictBox
   * readOnly` depending on which field is populated (Story 4.1.2).
   */
  sessionDiagnosticPanel(): Locator {
    return this.page.getByTestId("session-diagnostic-panel");
  }

  triageReviewPanel(): Locator {
    return this.page.getByTestId("triage-review-panel");
  }

  relatedWorkInput(): Locator {
    return this.page.getByTestId("triage-related-work-input");
  }

  relatedWorkResults(): Locator {
    return this.page.getByTestId("triage-related-work-results");
  }

  async openItemByTitle(title: string) {
    const row = this.page.getByTestId("backlog-table-row").filter({ hasText: title });
    await row.first().click();
    await this.pane.waitFor({ state: "visible", timeout: 5000 });
  }
}

/**
 * Seeds a backlog item with one linked `headless-triage-*` ItemSession
 * fixture (role "triage", `TriageResult` populated) so the redesigned
 * detail view's Synthetic Session diagnostic display (Epic 4) can be
 * exercised end-to-end without waiting on a real headless triage LLM call.
 *
 * Backed by the `/api/debug/backlog/seed-headless-triage-session` handler
 * (server/services/backlog_debug_seed_handler.go, Task 6.1.2a) — no
 * existing e2e fixture/seed path covered this case (checked BacklogPage.ts
 * and the pre-existing seed-stuck/seed-queued handlers first, per plan.md's
 * Unresolved Question #3), so this is a new minimal endpoint following the
 * exact pattern of `seedStuckItem` (StuckItemsPage.ts) /
 * `handleSeedQueued`. Registered ONLY when STAPLER_SQUAD_INSTANCE=e2e-local,
 * never reachable in a normal deploy.
 */
export async function seedHeadlessTriageItem(
  request: APIRequestContext,
  opts: { title: string; status?: string; summary?: string; ended?: boolean }
): Promise<{ itemId: string; sessionId: string }> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/seed-headless-triage-session`, {
    headers: { "Content-Type": "application/json" },
    data: {
      title: opts.title,
      status: opts.status ?? "review",
      summary: opts.summary ?? "",
      ended: opts.ended ?? false,
    },
  });
  if (!resp.ok()) {
    throw new Error(
      `seedHeadlessTriageItem failed (${resp.status()}): ${await resp.text().catch(() => "")}`
    );
  }
  const body = (await resp.json()) as { itemId: string; sessionId: string };
  return body;
}
