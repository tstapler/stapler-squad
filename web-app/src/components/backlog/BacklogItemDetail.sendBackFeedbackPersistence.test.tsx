/**
 * Task 2.2.1d (plan-feedback, Epic 2.2, CONCERN fix): verifies that
 * SendBackFeedbackBox's `actionError` state survives the parent's
 * `handleSendBackWithFeedback` catch-block `load()` re-render under REAL
 * async timing, not the synchronous mock timing every other test in this
 * suite uses.
 *
 * Why this matters: `handleSendBackWithFeedback`'s outer catch (a) calls
 * `load()`, which re-fetches the item and can flip `visible` to false the
 * instant the item's post-failure status (`ready`/`idea`) falls outside
 * `CAN_SEND_BACK_READY`, and only afterward (b) rethrows so
 * SendBackFeedbackBox's own `onSubmit` await rejects and its catch sets
 * `actionError`. (a) and (b) are two separately-scheduled async state
 * updates, not one React batch by construction — so this test drives both
 * RPC mocks with genuinely delayed Promises (setTimeout-backed, not
 * synchronously resolved) and uses a MutationObserver to catch every
 * intermediate DOM mutation between the rejection firing and the final
 * error rendering, asserting the send-back toggle/form/error region is
 * never simultaneously absent (no flash-unmount).
 */

import React from "react";
import { render, screen, fireEvent, act, waitFor } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import { makeReviewItem as makeReviewItemBase } from "./backlogItemDetailTestFixtures";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

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

// --- Real, independently-resolving Promise helpers (never synchronous) ---
function delayResolve<T>(value: T, ms = 8): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(value), ms));
}
function delayReject(error: unknown, ms = 8): Promise<never> {
  return new Promise((_resolve, reject) => setTimeout(() => reject(error), ms));
}

let getBacklogItemCallCount = 0;
// Set per-test: the status a getBacklogItem call returns once the failure
// simulation is "in effect" (i.e. every call after the initial page load).
let postFailureStatus: "idea" | "ready" = "idea";

const getBacklogItem = jest.fn();
const transitionStatus = jest.fn();
const rejectPlan = jest.fn();
const triggerTriage = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);

jest.mock("@/lib/hooks/useBacklogService", () =>
  require("./backlogItemDetailTestFixtures").useBacklogServiceMock(() => ({
    getBacklogItem,
    transitionStatus,
    rejectPlan,
    triggerTriage,
    listPipelineModes,
  }))
);

beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

// This suite's default id ("item-1") differs from the shared fixture's
// ("item-42") — none of its assertions depend on the exact value, but the
// per-call getBacklogItem mock below keys its updatedAt bump off it.
function makeReviewItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return makeReviewItemBase({ id: "item-1", ...overrides });
}

/** True only when the toggle, the form, AND the partial-failure error copy are ALL absent at once. */
function sendBackRegionFullyAbsent(): boolean {
  const toggle = screen.queryByTestId("backlog-action-send-back-feedback");
  const form = screen.queryByRole("form", { name: "Send back for re-planning" });
  const errorHeadline = screen.queryByText("Sent back, but retriage didn't start");
  return !toggle && !form && !errorHeadline;
}

/** MutationObserver-backed watcher: records every mutation at which the send-back region was fully absent. */
function watchForFlashUnmount(container: HTMLElement) {
  const violationsAt: number[] = [];
  let mutationCount = 0;
  const observer = new MutationObserver(() => {
    mutationCount += 1;
    if (sendBackRegionFullyAbsent()) violationsAt.push(mutationCount);
  });
  observer.observe(container, { childList: true, subtree: true, attributes: true, characterData: true });
  return {
    stop: () => observer.disconnect(),
    violationsAt,
  };
}

async function renderItemAndOpenForm(item: BacklogItem) {
  getBacklogItemCallCount = 0;
  getBacklogItem.mockReset().mockImplementation(() => {
    getBacklogItemCallCount += 1;
    if (getBacklogItemCallCount === 1) {
      return delayResolve(item);
    }
    // Every call after the initial page load simulates the item having
    // already moved server-side by the time it's re-fetched — a later call
    // gets a strictly-newer updatedAt so BacklogItemDetail's load() staleness
    // guard (`resultMs >= currentMs`) doesn't drop it.
    return delayResolve({
      ...item,
      status: postFailureStatus,
      planRejectionReason: "missed the mobile layout, redo with touch targets",
      updatedAt: new Date(new Date(item.updatedAt ?? 0).getTime() + getBacklogItemCallCount * 1000).toISOString(),
    });
  });
  localStorage.clear();

  const utils = render(<BacklogItemDetail itemId={item.id} />);
  await waitFor(() => expect(screen.getByTestId("backlog-action-send-back-feedback")).toBeInTheDocument());

  fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback"));
  fireEvent.change(screen.getByTestId("send-back-feedback-textarea"), {
    target: { value: "missed the mobile layout, redo with touch targets" },
  });

  return utils;
}

describe("BacklogItemDetail — actionError persists across load()'s async re-render (Task 2.2.1d)", () => {
  beforeEach(() => {
    transitionStatus.mockReset().mockImplementation(() => delayResolve({ ...makeReviewItem(), status: "ready" }));
    rejectPlan.mockReset().mockImplementation(() =>
      delayResolve({ ...makeReviewItem(), status: "ready", planRejectionReason: "missed the mobile layout, redo with touch targets" })
    );
    triggerTriage.mockReset();
  });

  it("never flash-unmounts the send-back region when triggerTriage fails after its own ready->idea CAS already committed (pre-mortem P1 #1)", async () => {
    postFailureStatus = "idea";
    triggerTriage.mockImplementation(() => delayReject(new Error("triggerTriage: artifact dir creation failed")));

    const { container } = await renderItemAndOpenForm(makeReviewItem());
    const watcher = watchForFlashUnmount(container);

    await act(async () => {
      fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));
      // Long enough for transitionStatus -> rejectPlan -> triggerTriage ->
      // the inner catch's direct getBacklogItem() -> the outer catch's
      // load() -> SendBackFeedbackBox's own catch to all settle in sequence.
      await new Promise((r) => setTimeout(r, 200));
    });

    watcher.stop();

    expect(transitionStatus).toHaveBeenCalledTimes(1);
    expect(rejectPlan).toHaveBeenCalledTimes(1);
    expect(triggerTriage).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument();
    expect(screen.getByText(/moved back to "idea"/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry send-back with this feedback" })).toBeInTheDocument();
    expect(watcher.violationsAt).toEqual([]);
  });

  it("never flash-unmounts the send-back region when triggerTriage fails with status still ready (ordinary partial failure)", async () => {
    postFailureStatus = "ready";
    triggerTriage.mockImplementation(() => delayReject(new Error("triggerTriage: headless pool nil")));

    const { container } = await renderItemAndOpenForm(makeReviewItem());
    const watcher = watchForFlashUnmount(container);

    await act(async () => {
      fireEvent.click(screen.getByTestId("backlog-action-send-back-feedback-submit"));
      await new Promise((r) => setTimeout(r, 200));
    });

    watcher.stop();

    expect(transitionStatus).toHaveBeenCalledTimes(1);
    expect(rejectPlan).toHaveBeenCalledTimes(1);
    expect(triggerTriage).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Sent back, but retriage didn't start")).toBeInTheDocument();
    expect(screen.getByText(/"Regenerate Plan with This Feedback"/)).toBeInTheDocument();
    expect(watcher.violationsAt).toEqual([]);
  });
});
