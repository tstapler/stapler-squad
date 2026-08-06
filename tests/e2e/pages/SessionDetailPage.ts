import { Page, Locator } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/**
 * Page object for the main workspace's session detail view
 * (web-app/src/components/pane/PaneSplitRenderer.tsx renders <SessionDetail>
 * for a pane whose `viewKind` isn't "session-list").
 *
 * Added for project_plans/backlog-event-driven-updates Surface 4
 * (`BacklogItemPanel` embedded in `SessionDetail`) — see
 * session-detail-backlog-panel.spec.ts's file header for the confirmed
 * wiring gap this page object documents: no real call site threads a
 * `backlogItemId` prop into `SessionDetail`/`SessionDetailView` today, so
 * `getBacklogPanel()` will never find a match against the live app no
 * matter which session is open, until that wiring is added.
 */
export class SessionDetailPage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  async gotoSession(sessionId: string) {
    await this.page.goto(`${BASE_URL}/?session=${sessionId}`, { waitUntil: "domcontentloaded" });
  }

  getPane(): Locator {
    return this.page.locator('[data-context="cockpit"]');
  }

  getBacklogPanel(): Locator {
    return this.page.getByTestId("backlog-panel");
  }

  getBacklogPanelToggle(): Locator {
    return this.page.getByTestId("backlog-panel-toggle");
  }

  getBacklogPanelTerminalNotice(): Locator {
    return this.page.getByTestId("backlog-panel-terminal-notice");
  }

  getConnectionIndicators(): Locator {
    return this.page.getByTestId(/connection-indicator|live-indicator/);
  }

  getCIStatusBadge(): Locator {
    return this.page.getByTestId("ci-status-badge");
  }

  // ---------------------------------------------------------------------
  // Terminal tab — added for session-completion-summary.spec.ts, which
  // types `exit` into a plain-shell one-off session's terminal to end it
  // naturally (EventExited) and trigger session-summary generation.
  // ---------------------------------------------------------------------

  getTerminalTab(): Locator {
    return this.page.getByRole("tab", { name: /terminal/i });
  }

  /** The active tab's `role="tabpanel"` region — SessionDetailView.tsx sets
   * `aria-labelledby="tab-terminal"` on it, which resolves its accessible
   * name to the Terminal tab's label ("Terminal"). Click this to focus
   * xterm's hidden input before sending keystrokes via `page.keyboard`. */
  getTerminalPanel(): Locator {
    return this.page.getByRole("tabpanel", { name: /terminal/i });
  }

  /** Terminal toolbar toggle — visible once the terminal has attached and
   * is ready for input (established readiness signal, see terminal-resize.spec.ts). */
  getTerminalToolbarToggle(): Locator {
    return this.page.getByTestId("toolbar-toggle");
  }

  // ---------------------------------------------------------------------
  // Summary tab — SessionDetailView.tsx gates it on `isSessionTerminal`
  // (status === STOPPED); SessionSummaryPanel.tsx renders the content once
  // generation reaches READY.
  // ---------------------------------------------------------------------

  getSummaryTab(): Locator {
    return this.page.getByRole("tab", { name: "Summary" });
  }

  getSummaryPanel(): Locator {
    return this.page.getByTestId("session-summary-panel");
  }

  getSummaryMarkdownBody(): Locator {
    return this.page.getByTestId("summary-markdown-body");
  }

  getSummaryCopyButton(): Locator {
    return this.page.getByRole("button", { name: "Copy summary as Markdown" });
  }

  /** Shared `aria-live="polite"` status region SessionSummaryPanel.tsx uses
   * to announce phase transitions and the copy result. */
  getSummaryLiveRegion(): Locator {
    return this.page.getByRole("status");
  }
}
