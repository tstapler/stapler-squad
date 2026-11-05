import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import { BacklogItemDetail } from "./BacklogItemDetail";
import { makeReviewItem } from "./backlogItemDetailTestFixtures";

jest.mock("./SessionMonitor", () => require("./backlogItemDetailTestFixtures").sessionMonitorMock());
jest.mock("./GateVerdictBox", () => require("./backlogItemDetailTestFixtures").gateVerdictBoxMock());
jest.mock("./TriageReviewPanel", () => require("./backlogItemDetailTestFixtures").triageReviewPanelMock());
jest.mock("./TriageLoadingIndicator", () => require("./backlogItemDetailTestFixtures").triageLoadingIndicatorMock());

jest.mock("@/lib/hooks/useSessionRepoPaths", () => require("./backlogItemDetailTestFixtures").useSessionRepoPathsMock());
jest.mock("@/lib/hooks/usePathCompletions", () => require("./backlogItemDetailTestFixtures").usePathCompletionsMock());
jest.mock("@/lib/hooks/useSessionService", () => require("./backlogItemDetailTestFixtures").useSessionServiceMock());
jest.mock("@/lib/analytics", () => require("./backlogItemDetailTestFixtures").analyticsMock());
jest.mock("@/lib/hooks/useWatchBacklogItems", () => require("./backlogItemDetailTestFixtures").useWatchBacklogItemsMock());
jest.mock("@/lib/store", () => require("./backlogItemDetailTestFixtures").storeMock());
jest.mock("@connectrpc/connect", () => require("./backlogItemDetailTestFixtures").connectMockWithActual());
jest.mock("@connectrpc/connect-web", () => require("./backlogItemDetailTestFixtures").connectWebMock());
jest.mock("@/lib/hooks/useStuckBacklogItems", () => require("./backlogItemDetailTestFixtures").useStuckBacklogItemsMock());

// useNotifications() falls back to a no-op (including a stubbed
// showActionToast) when rendered outside a NotificationProvider, which
// every other BacklogItemDetail.*.test.tsx file relies on implicitly — but
// this file needs to assert the exact toast text/type, so it mocks the
// module directly instead, spying on showActionToast.
const showActionToast = jest.fn().mockReturnValue("toast-id");
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ showActionToast }),
}));


const getBacklogItem = jest.fn();
const associatePullRequest = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);

jest.mock("@/lib/hooks/useBacklogService", () =>
  require("./backlogItemDetailTestFixtures").useBacklogServiceMock(() => ({
    getBacklogItem,
    transitionStatus: jest.fn(),
    rejectPlan: jest.fn(),
    triggerTriage: jest.fn(),
    listPipelineModes,
    associatePullRequest,
  }))
);

beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

const PR_URL = "https://github.com/acme/app/pull/55";

async function linkPrFromManualOverrides() {
  getBacklogItem.mockReset().mockResolvedValue(makeReviewItem());
  localStorage.clear();
  render(<BacklogItemDetail itemId="item-42" />);
  fireEvent.click(await screen.findByRole("button", { name: /Manual Overrides/ }));
  fireEvent.change(await screen.findByTestId("manual-override-pr-url-input"), { target: { value: PR_URL } });
  fireEvent.change(screen.getByTestId("manual-override-pr-number-input"), { target: { value: "55" } });
  fireEvent.click(screen.getByTestId("manual-override-pr-submit"));
}

describe("BacklogItemDetail — manual PR link toast is worded from the server's pr_association", () => {
  beforeEach(() => {
    associatePullRequest.mockReset();
    showActionToast.mockClear();
  });

  it("claims the move to pr_pending only for LINKED_AND_MOVED", async () => {
    associatePullRequest.mockResolvedValue({ item: makeReviewItem({ status: "pr_pending" }), association: "linked_and_moved" });
    await linkPrFromManualOverrides();
    await waitFor(() =>
      expect(showActionToast).toHaveBeenCalledWith("PR #55 linked — item moved to pr_pending.", "success", "item-42:manual_associate_pr")
    );
    expect(associatePullRequest).toHaveBeenCalledWith("item-42", expect.objectContaining({ prUrl: PR_URL, prNumber: 55 }));
  });

  it("says it is already tracking the PR, with no move claim, for ALREADY_TRACKED", async () => {
    associatePullRequest.mockResolvedValue({ item: makeReviewItem({ status: "pr_pending" }), association: "already_tracked" });
    await linkPrFromManualOverrides();
    await waitFor(() =>
      expect(showActionToast).toHaveBeenCalledWith("Already tracking PR #55 — nothing changed.", "success", "item-42:manual_associate_pr")
    );
    expect(showActionToast).not.toHaveBeenCalledWith(expect.stringMatching(/moved/), expect.anything(), expect.anything());
  });

  it("shows the server's FailedPrecondition message as an error toast", async () => {
    associatePullRequest.mockRejectedValue(new ConnectError("item is in status ready; PR can only be linked in review or pr_pending", Code.FailedPrecondition));
    await linkPrFromManualOverrides();
    await waitFor(() =>
      expect(showActionToast).toHaveBeenCalledWith(
        "item is in status ready; PR can only be linked in review or pr_pending",
        "error",
        "item-42:manual_associate_pr"
      )
    );
    expect(showActionToast).not.toHaveBeenCalledWith(expect.stringMatching(/linked/), "success", expect.anything());
  });
});
