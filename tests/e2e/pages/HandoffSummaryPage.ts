import { Page, Locator, Route } from "@playwright/test";
import { SessionDetailPage } from "./SessionDetailPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/**
 * JSON shape of `session.v1.HandoffSummaryProto` as it appears over the wire
 * (ConnectRPC JSON codec) -- `google.protobuf.Timestamp` fields serialize as
 * RFC3339 strings (confirmed against approval-ci-block.spec.ts's
 * `createdAt: new Date().toISOString()` precedent), and the status enum
 * serializes as its full `HANDOFF_SUMMARY_STATUS_*` name (confirmed against
 * crashed-session-ux.spec.ts's `SESSION_STATUS_CRASHED` precedent).
 */
export interface HandoffSummaryFixture {
  sessionId?: string;
  sessionTitle?: string;
  status?:
    | "HANDOFF_SUMMARY_STATUS_UNSPECIFIED"
    | "HANDOFF_SUMMARY_STATUS_PENDING"
    | "HANDOFF_SUMMARY_STATUS_GENERATING"
    | "HANDOFF_SUMMARY_STATUS_READY"
    | "HANDOFF_SUMMARY_STATUS_ERROR";
  activeTask?: string;
  summaryText?: string;
  middleMessagesSummarized?: number;
  errorMessage?: string;
  errorStage?: string;
  generatedAt?: string;
  generationStartedAt?: string;
}

export function handoffSummaryFixture(overrides: HandoffSummaryFixture = {}): Record<string, unknown> {
  const now = new Date().toISOString();
  return {
    sessionId: overrides.sessionId ?? "",
    sessionTitle: overrides.sessionTitle ?? "",
    status: overrides.status ?? "HANDOFF_SUMMARY_STATUS_READY",
    activeTask: overrides.activeTask ?? "",
    summaryText: overrides.summaryText ?? "",
    middleMessagesSummarized: overrides.middleMessagesSummarized ?? 0,
    errorMessage: overrides.errorMessage ?? "",
    errorStage: overrides.errorStage ?? "",
    generatedAt: overrides.generatedAt ?? now,
    generationStartedAt: overrides.generationStartedAt ?? now,
  };
}

/**
 * Page object for the `HandoffSummarySection`/`RestartWithSummaryButton`
 * restart-with-summary flow (context-compression, Info tab). Wraps
 * `SessionDetailPage` for navigation rather than duplicating it, per this
 * repo's page-helper convention.
 *
 * Route-interception helpers here follow crashed-session-ux.spec.ts's
 * `page.route()` + `route.fetch()`/`route.fulfill()` pattern: there is no
 * backend test-mode hook to force a real `PoolClient`/RPC failure, so every
 * GENERATING/READY/ERROR state exercised by these tests is injected at the
 * HTTP layer rather than produced by a real LLM call.
 */
export class HandoffSummaryPage {
  readonly page: Page;
  readonly detail: SessionDetailPage;

  constructor(page: Page) {
    this.page = page;
    this.detail = new SessionDetailPage(page);
  }

  async gotoInfoTab(sessionId: string) {
    await this.page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
    await this.detail.gotoSession(sessionId);
    await this.detail.getInfoTab().click();
  }

  // -- Locators -----------------------------------------------------------

  getSectionHeader(): Locator {
    return this.page.getByTestId("collapsible-header-handoff-summary");
  }

  getList(): Locator {
    return this.page.getByRole("list", { name: "Handoff summary" });
  }

  getListItem(): Locator {
    return this.getList().getByRole("listitem");
  }

  getEmptyText(): Locator {
    return this.page.getByText("No handoff summary generated for this session.");
  }

  getPreviewToggle(): Locator {
    return this.getListItem().getByText("Preview full handoff text");
  }

  /** The full summary text revealed by expanding the preview `<details>`. */
  getPreviewText(summaryText: string): Locator {
    return this.getListItem().getByText(summaryText);
  }

  getActionButton(): Locator {
    return this.page.getByTestId("restart-with-summary-button");
  }

  getRetryButton(): Locator {
    return this.page.getByTestId("restart-with-summary-retry");
  }

  getErrorContainer(): Locator {
    return this.page.getByTestId("restart-with-summary-error");
  }

  getErrorMessage(): Locator {
    return this.page.getByTestId("restart-with-summary-error-message");
  }

  getErrorDetailsToggle(): Locator {
    return this.getErrorContainer().getByText("Details");
  }

  /** The raw `error_message` text node behind the `<details>` disclosure. */
  getErrorRawDetail(rawText: string): Locator {
    return this.getErrorContainer().getByText(rawText);
  }

  getRestartError(): Locator {
    return this.page.getByTestId("restart-with-summary-restart-error");
  }

  getAriaLiveRegions(): Locator {
    return this.getList().locator('[aria-live="polite"]');
  }

  // -- Route interception ---------------------------------------------------

  /**
   * Intercepts both `GetHandoffSummary` and `TriggerHandoffSummary` with a
   * single mutable fixture: `GetHandoffSummary` always returns whatever
   * `fixture` currently holds; `TriggerHandoffSummary` replaces it with
   * `onTrigger()`'s result (or leaves it unchanged if omitted) before
   * returning it. This lets a test simulate "no row yet, then a real click
   * flips it to GENERATING/READY/ERROR" without waiting on a real backend
   * or a real LLM call -- both of `HandoffSummarySection`'s and
   * `RestartWithSummaryButton`'s independent `useHandoffSummary` polls
   * observe the same mutable state.
   */
  async mockHandoffSummary(
    initial: Record<string, unknown> | null,
    onTrigger?: () => Record<string, unknown>
  ): Promise<{ set: (fixture: Record<string, unknown> | null) => void }> {
    let current = initial;
    await this.page.route(
      "**/api/session.v1.HandoffSummaryService/GetHandoffSummary",
      async (route: Route) => {
        await route.fulfill({ contentType: "application/json", body: JSON.stringify({ summary: current ?? undefined }) });
      }
    );
    await this.page.route(
      "**/api/session.v1.HandoffSummaryService/TriggerHandoffSummary",
      async (route: Route) => {
        if (onTrigger) current = onTrigger();
        await route.fulfill({ contentType: "application/json", body: JSON.stringify({ summary: current ?? undefined }) });
      }
    );
    // Route handlers persist across page.goto() navigations, so a single
    // registration can be reused across multiple session navigations within
    // the same test by calling `set()` before each `gotoInfoTab()` --
    // re-calling `mockHandoffSummary()` itself would stack a second set of
    // handlers rather than replacing the first.
    return { set: (fixture) => { current = fixture; } };
  }

  /** Forces `CreateSession` to reject with a ConnectRPC error envelope. */
  async mockCreateSessionError(status: number, code: string, message: string) {
    await this.page.route(
      "**/api/session.v1.SessionService/CreateSession",
      async (route: Route) => {
        await route.fulfill({
          status,
          contentType: "application/json",
          body: JSON.stringify({ code, message }),
        });
      }
    );
  }

  /** Forces `CreateSession` to succeed with a synthetic session id, without touching the real backend. */
  async mockCreateSessionSuccess(newSessionId: string) {
    await this.page.route(
      "**/api/session.v1.SessionService/CreateSession",
      async (route: Route) => {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            session: {
              id: newSessionId,
              title: `restarted-${newSessionId}`,
              status: "SESSION_STATUS_RUNNING",
              sessionType: "SESSION_TYPE_DIRECTORY",
            },
          }),
        });
      }
    );
  }
}

export { BASE_URL };
