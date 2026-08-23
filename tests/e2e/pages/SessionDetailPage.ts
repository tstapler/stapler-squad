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

  /** The interactive xterm.js container within the active Terminal tabpanel
   * (SessionDetailView.tsx sets `aria-labelledby="tab-terminal"` on the
   * tabpanel, resolving its accessible name to "Terminal"). Scoped to
   * `[data-context="terminal"]` (XtermTerminal.tsx) rather than the tabpanel
   * itself: clicking the tabpanel's outer wrapper lands on padding/toolbar
   * area outside xterm's canvas and never focuses its hidden
   * `textarea.xterm-helper-textarea`, so keystrokes sent via `page.keyboard`
   * after that click go nowhere. Click this locator instead to reliably
   * focus xterm's input before sending keystrokes. */
  getTerminalPanel(): Locator {
    return this.page.getByRole("tabpanel", { name: /terminal/i }).locator('[data-context="terminal"]');
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
   * to announce phase transitions and the copy result. Scoped to the summary
   * panel (`data-testid="session-summary-panel"`) -- an unscoped
   * `page.getByRole("status")` also matches unrelated `role="status"` regions
   * elsewhere on the page (the nav's bulk-feedback/empty-state live regions),
   * causing a strict-mode violation once this locator is actually reached. */
  getSummaryLiveRegion(): Locator {
    return this.getSummaryPanel().getByRole("status");
  }

  // ---------------------------------------------------------------------
  // Notes panel — Info tab, NotePanel.tsx. See session-notes.spec.ts.
  // ---------------------------------------------------------------------

  getInfoTab(): Locator {
    return this.page.getByRole("tab", { name: "Info" });
  }

  getNotePanel(): Locator {
    return this.page.getByTestId("session-note-panel");
  }

  getNoteAddButton(): Locator {
    return this.page.getByRole("button", { name: "Add note" });
  }

  getNoteTextarea(): Locator {
    return this.page.getByTestId("session-note-textarea");
  }

  getNoteSaveButton(): Locator {
    return this.page.getByTestId("session-note-save-button");
  }

  getNoteRenderedBody(): Locator {
    return this.page.getByTestId("session-note-rendered");
  }
}
