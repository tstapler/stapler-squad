/**
 * Task 5.2d (notification-tray-and-hidden-session-gate), T-RO-21: characterization
 * of today's behaviour, written before the hidden-session read-only guard. The
 * backlog composer steers a live review session through the widened
 * `updateSession(sessionId, { steerMessage })` RPC and confirms with a toast.
 * The session's hidden visibility is server-side state; the UI sees only the
 * linked review row, so this pins the call the guard must keep allowing (O7).
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BacklogItemDetail } from "../BacklogItemDetail";
import { makeReviewItem } from "../backlogItemDetailTestFixtures";

jest.mock("../SessionMonitor", () => require("../backlogItemDetailTestFixtures").sessionMonitorMock());
jest.mock("../GateVerdictBox", () => require("../backlogItemDetailTestFixtures").gateVerdictBoxMock());
jest.mock("../TriageReviewPanel", () => require("../backlogItemDetailTestFixtures").triageReviewPanelMock());
jest.mock("../TriageLoadingIndicator", () => require("../backlogItemDetailTestFixtures").triageLoadingIndicatorMock());

jest.mock("@/lib/hooks/useSessionRepoPaths", () => require("../backlogItemDetailTestFixtures").useSessionRepoPathsMock());
jest.mock("@/lib/hooks/usePathCompletions", () => require("../backlogItemDetailTestFixtures").usePathCompletionsMock());
jest.mock("@/lib/analytics", () => require("../backlogItemDetailTestFixtures").analyticsMock());
jest.mock("@/lib/hooks/useWatchBacklogItems", () => require("../backlogItemDetailTestFixtures").useWatchBacklogItemsMock());
jest.mock("@/lib/store", () => require("../backlogItemDetailTestFixtures").storeMock());
jest.mock("@connectrpc/connect", () => require("../backlogItemDetailTestFixtures").connectMockWithActual());
jest.mock("@connectrpc/connect-web", () => require("../backlogItemDetailTestFixtures").connectWebMock());
jest.mock("@/lib/hooks/useStuckBacklogItems", () => require("../backlogItemDetailTestFixtures").useStuckBacklogItemsMock());

const updateSession = jest.fn();
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession: jest.fn(), updateSession }),
}));

const showActionToast = jest.fn().mockReturnValue("toast-id");
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ showActionToast }),
}));

const getBacklogItem = jest.fn();
jest.mock("@/lib/hooks/useBacklogService", () =>
  require("../backlogItemDetailTestFixtures").useBacklogServiceMock(() => ({
    getBacklogItem,
    listPipelineModes: jest.fn().mockResolvedValue([]),
  }))
);

// Pre-existing jest/vanilla-extract mock limitation — see BacklogItemDetail.test.tsx.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

describe("BacklogItemDetail steer characterization (Task 5.2d)", () => {
  it("backlog_steer_should_call_updateSession_with_steerMessage_and_show_sent_toast_when_hidden_review_session_is_steerable", async () => {
    const sessionId = "review-live-1";
    const item = makeReviewItem({
      linkedSessions: [
        { entityId: "entity-review-1", sessionId, role: "review", estimatedCostUsd: 0 },
      ],
    });
    getBacklogItem.mockReset().mockResolvedValue(item);
    updateSession.mockReset().mockResolvedValue({ id: sessionId });
    showActionToast.mockClear();
    localStorage.clear();

    render(<BacklogItemDetail itemId={item.id} />);

    const toggle = await screen.findByTestId(`session-steer-toggle-${sessionId}`);
    fireEvent.click(toggle);
    fireEvent.change(screen.getByTestId(`session-steer-input-${sessionId}`), {
      target: { value: "re-check the acceptance criteria" },
    });
    fireEvent.click(screen.getByTestId(`session-steer-submit-${sessionId}`));

    await waitFor(() =>
      expect(updateSession).toHaveBeenCalledWith(sessionId, { steerMessage: "re-check the acceptance criteria" })
    );
    await waitFor(() =>
      expect(showActionToast).toHaveBeenCalledWith("Steering message sent.", "success", `${sessionId}:steer`)
    );
  });
});
