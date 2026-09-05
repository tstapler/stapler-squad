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

  // ---------------------------------------------------------------------
  // Connection-count indicator — Epic 4.2, Story 4.2.2
  // (ConnectionCountIndicator.tsx, rendered by TerminalOutput.tsx only when
  // connectionCount > 1). See connection-count-indicator.spec.ts.
  // ---------------------------------------------------------------------

  getConnectionCountIndicator(): Locator {
    return this.page.getByTestId("connection-count-indicator");
  }

  getConnectionCountTooltip(): Locator {
    return this.page.getByTestId("connection-count-tooltip");
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

  // ---------------------------------------------------------------------
  // Insights session drill-down — route vs. modal (project_plans/insights-cost-intelligence,
  // design/ux.md B3). Unlike the sections above (the main workspace pane's
  // <SessionDetail>), these cover `SessionDetailContent` as rendered by
  // `/insights/session-detail?sessionId=` (SessionDetailPageClient.tsx) and
  // by the dashboard's quick-peek modal (SessionDetailDrawer.tsx) — both
  // render the same component, so one set of locators/assertions serves
  // both surfaces. Added here per this project's page-object reuse
  // convention rather than forking a second "session detail" page object.
  // ---------------------------------------------------------------------

  /** Navigates directly to the deep-linkable route (cold navigation, no prior client-side history). */
  async gotoInsightsSessionRoute(sessionId: string) {
    // Skip the first-run onboarding tour modal — see InsightsPage.goto()'s
    // identical addInitScript for why this is needed on every fresh
    // navigation, not just the dashboard's.
    await this.page.addInitScript(() => {
      try {
        window.localStorage.setItem("stapler-squad:onboarded", "true");
      } catch {
        /* ignore */
      }
    });
    await this.page.goto(`${BASE_URL}/insights/session-detail?sessionId=${encodeURIComponent(sessionId)}`, {
      waitUntil: "domcontentloaded",
    });
  }

  /** The route/modal's `<h1>`-equivalent heading — receives focus on route mount (design/ux.md B3). */
  getInsightsHeading(): Locator {
    return this.page.getByRole("heading", { level: 1 });
  }

  /** Present in every route state (found, not-found, error) — the "no dead ends" guarantee. */
  getInsightsBackToDashboardLink(): Locator {
    return this.page.getByRole("link", { name: /Back to dashboard/i });
  }

  getInsightsSessionNotFound(): Locator {
    return this.page.getByTestId("session-not-found");
  }

  /** SessionDetailDrawer's dialog container (role="dialog"). */
  getInsightsModal(): Locator {
    return this.page.getByRole("dialog", { name: /session details/i });
  }

  getInsightsModalCloseButton(): Locator {
    return this.page.getByRole("button", { name: /close session details/i });
  }

  /** Navigates from the modal (quick-peek) to the deep-linkable route. */
  getInsightsOpenFullPageLink(): Locator {
    return this.page.getByRole("link", { name: /Open full page/i });
  }

  /** SessionDetailContent's "Tools Breakdown" table (shared verbatim by the modal and the route). */
  getInsightsToolsBreakdownTable(): Locator {
    return this.page.getByTestId("tools-breakdown-table");
  }
}
