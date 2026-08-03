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
}
