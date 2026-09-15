/**
 * Task 2.2.1 (plan-feedback, Epic 2.2, Story 2.2.1): call-order/args/toast
 * coverage for `handleSendBackWithFeedback`, closing the gap left by
 * `BacklogItemDetail.sendBackFeedbackPersistence.test.tsx` (Task 2.2.1d),
 * which only asserts DOM-presence/no-flash-unmount timing, not the exact
 * `transitionStatus`/`rejectPlan`/`triggerTriage` call order, arguments, or
 * toast copy that plan.md's Story 2.2.1 acceptance criteria spell out.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

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
const transitionStatus = jest.fn();
const rejectPlan = jest.fn();
const triggerTriage = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus,
    triggerTriage,
    rejectPlan,
    cancelTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    unarchiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

// Pre-existing jest/vanilla-extract mock limitation — see BacklogItemDetail.test.tsx.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: "session-entity-1",
    sessionId: "session-1",
    role: "work",
    estimatedCostUsd: 0,
    pipelineModeSnapshot: "",
    pipelineModeSnapshotHash: "",
    ...overrides,
  };
}

const T1 = timestampFromDate(new Date("2026-07-12T14:02:00.000Z"));
const FEEDBACK = "missed the mobile layout, redo with touch targets";

function makeReviewItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-42",
    title: "Fix mobile layout",
    description: "desc",
    status: "review",
    priority: 3,
    repoPath: "/tmp/repo",
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    autoApprovePlan: false,
    planApproved: false,
    acCriteria: [{ index: 0, text: "AC 1", status: "done" }],
    linkedSessions: [makeSession()],
    notes: "",
    createdAt: "2026-07-12T14:02:00.000Z",
    updatedAt: "2026-07-12T14:02:00.000Z",
    updatedAtRaw: T1,
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

async function renderItemAndOpenForm(item: BacklogItem) {
  getBacklogItem.mockReset().mockResolvedValue(item);
  localStorage.clear();

  render(<BacklogItemDetail itemId={item.id} />);
  await waitFor(() => expect(screen.getByTestId("backlog-action-send-back-feedback")).toBeInTheDocument());

  fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback"));
  fireEvent.change(screen.getByTestId("send-back-feedback-textarea"), {
    target: { value: FEEDBACK },
  });
}

function submit() {
  fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));
}

describe("BacklogItemDetail — handleSendBackWithFeedback (Story 2.2.1)", () => {
  beforeEach(() => {
    transitionStatus.mockReset();
    rejectPlan.mockReset();
    triggerTriage.mockReset();
    showActionToast.mockClear();
  });

  it("submits feedback through transitionStatus, rejectPlan, and triggerTriage in order, shows a success toast, and reloads", async () => {
    const item = makeReviewItem();
    transitionStatus.mockResolvedValue({ ...item, status: "ready" });
    rejectPlan.mockResolvedValue({ ...item, status: "ready", planRejectionReason: FEEDBACK });
    triggerTriage.mockResolvedValue({ itemSessionId: "session-x" });

    await renderItemAndOpenForm(item);
    const initialGetBacklogItemCalls = getBacklogItem.mock.calls.length;
    submit();

    await waitFor(() => expect(triggerTriage).toHaveBeenCalledTimes(1));

    expect(transitionStatus).toHaveBeenCalledWith("item-42", "ready", {
      expectedStatus: "review",
      expectedUpdatedAt: T1,
      overrideReason: FEEDBACK,
    });
    expect(rejectPlan).toHaveBeenCalledWith("item-42", FEEDBACK);
    expect(triggerTriage).toHaveBeenCalledWith("item-42", FEEDBACK);

    // Call order: transitionStatus -> rejectPlan -> triggerTriage.
    const [transitionOrder] = transitionStatus.mock.invocationCallOrder;
    const [rejectOrder] = rejectPlan.mock.invocationCallOrder;
    const [triageOrder] = triggerTriage.mock.invocationCallOrder;
    expect(transitionOrder).toBeLessThan(rejectOrder);
    expect(rejectOrder).toBeLessThan(triageOrder);

    await waitFor(() =>
      expect(showActionToast).toHaveBeenCalledWith("Feedback sent — retriage started.", "success", "item-42:send_back_ready")
    );
    // load() re-fetches the item via getBacklogItem — one additional call
    // beyond whatever the initial render already issued.
    await waitFor(() => expect(getBacklogItem.mock.calls.length).toBeGreaterThan(initialGetBacklogItemCalls));
  });

  it("does not call rejectPlan or triggerTriage when transitionStatus rejects", async () => {
    const item = makeReviewItem();
    transitionStatus.mockRejectedValue(new Error("transitionStatus: precondition failed"));

    await renderItemAndOpenForm(item);
    submit();

    await waitFor(() => expect(showActionToast).toHaveBeenCalled());

    expect(transitionStatus).toHaveBeenCalledTimes(1);
    expect(rejectPlan).not.toHaveBeenCalled();
    expect(triggerTriage).not.toHaveBeenCalled();
    // getErrorMessage (web-app/src/lib/utils/connectError.ts) prefers the
    // real Error#message over the "Failed to send back." fallback, which
    // this branch keeps (unlike the partial-failure branches below) since
    // nothing changed server-side.
    expect(showActionToast).toHaveBeenCalledWith(
      "transitionStatus: precondition failed",
      "error",
      "item-42:send_back_ready"
    );
  });

  it("reloads before showing the error toast when triggerTriage fails after transitionStatus and rejectPlan succeed", async () => {
    const item = makeReviewItem();
    transitionStatus.mockResolvedValue({ ...item, status: "ready" });
    rejectPlan.mockResolvedValue({ ...item, status: "ready", planRejectionReason: FEEDBACK });
    triggerTriage.mockRejectedValue(new Error("triggerTriage: headless pool nil"));
    // Called both directly (inside handleSendBackWithFeedback's triage catch,
    // to learn the real post-failure status) and via load() in the outer
    // catch — return "ready" throughout since this scenario's CAS never ran.
    getBacklogItem.mockResolvedValue({ ...item, status: "ready" });

    await renderItemAndOpenForm(item);
    submit();

    await waitFor(() =>
      expect(showActionToast).toHaveBeenCalledWith(
        "Send-back needs attention — see details below.",
        "error",
        "item-42:send_back_ready"
      )
    );

    expect(transitionStatus).toHaveBeenCalledTimes(1);
    expect(rejectPlan).toHaveBeenCalledTimes(1);
    expect(triggerTriage).toHaveBeenCalledTimes(1);

    const lastGetBacklogItemOrder =
      getBacklogItem.mock.invocationCallOrder[getBacklogItem.mock.invocationCallOrder.length - 1];
    const errorToastCallIndex = showActionToast.mock.calls.findIndex(([, type]) => type === "error");
    const errorToastOrder = showActionToast.mock.invocationCallOrder[errorToastCallIndex];
    expect(lastGetBacklogItemOrder).toBeLessThan(errorToastOrder);
  });

  it("reloads before showing the error toast when rejectPlan fails after transitionStatus succeeds", async () => {
    const item = makeReviewItem();
    transitionStatus.mockResolvedValue({ ...item, status: "ready" });
    rejectPlan.mockRejectedValue(new Error("rejectPlan: no plan artifacts path"));
    getBacklogItem.mockResolvedValue({ ...item, status: "ready" });

    await renderItemAndOpenForm(item);
    submit();

    await waitFor(() =>
      expect(showActionToast).toHaveBeenCalledWith(
        "Send-back needs attention — see details below.",
        "error",
        "item-42:send_back_ready"
      )
    );

    expect(transitionStatus).toHaveBeenCalledTimes(1);
    expect(rejectPlan).toHaveBeenCalledTimes(1);
    expect(triggerTriage).not.toHaveBeenCalled();

    const lastGetBacklogItemOrder =
      getBacklogItem.mock.invocationCallOrder[getBacklogItem.mock.invocationCallOrder.length - 1];
    const errorToastCallIndex = showActionToast.mock.calls.findIndex(([, type]) => type === "error");
    const errorToastOrder = showActionToast.mock.invocationCallOrder[errorToastCallIndex];
    expect(lastGetBacklogItemOrder).toBeLessThan(errorToastOrder);
  });
});
