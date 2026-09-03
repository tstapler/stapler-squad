/**
 * Regression coverage for the 2026-08-03 fix
 * (docs/tasks/backlog-feature-improvement.md, item be676dab): a backlog item
 * whose most recent triage session ended with no usable result
 * (`triageResult: null`) used to render NOTHING when the item had already
 * advanced past `idea` (e.g. `queued` via the WIP cap) — no summary, no error
 * state, no retry affordance, just the stuck badge and a bare "Delete"
 * action. `mapBacklogItem` (useBacklogService.ts) already correctly derives
 * `triageStatus: "failed"` for this shape regardless of item status; the bug
 * was that the failed-triage banner's render condition was hardcoded to
 * `item.status === "idea"` only. This suite verifies the generalized
 * condition renders the banner for a queued item too, and that retrying
 * resets the item to idea before re-triggering triage (TriggerTriage only
 * ever accepts idea/ready — see AutoRespawnTriage's identical server-side
 * handling in server/services/backlog_service_triage.go).
 */

import React from "react";
import { render, screen, act, fireEvent, within } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, BacklogItemStatus } from "@/lib/hooks/useBacklogService";

jest.mock("./SessionMonitor", () => ({ SessionMonitor: () => null }));
jest.mock("./GateVerdictBox", () => ({ GateVerdictBox: () => null }));
jest.mock("./TriageReviewPanel", () => ({ TriageReviewPanel: () => null }));
jest.mock("./TriageLoadingIndicator", () => ({ TriageLoadingIndicator: () => null }));
jest.mock("./ReviewChangesModal", () => ({ ReviewChangesModal: () => null }));
jest.mock("./BacklogFileBrowserModal", () => ({ BacklogFileBrowserModal: () => null }));

jest.mock("@/lib/hooks/useVcsStatus", () => ({
  useVcsStatus: () => ({ data: null, loading: false, error: null, refetch: jest.fn() }),
}));
jest.mock("@/lib/hooks/useBacklogItemShipStatus", () => ({
  useBacklogItemShipStatus: () => ({ data: null, loading: false, refetch: jest.fn() }),
}));
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => ({ items: [], isLoading: false, error: null }),
}));
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({ useSessionRepoPaths: () => [] }));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession: jest.fn() }),
}));
jest.mock("@/lib/analytics", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => ({ items: [], connectionState: "live" }),
}));
jest.mock("@/lib/store", () => ({
  useAppSelector: (selector: (state: unknown) => unknown) => selector({ backlogItems: { items: {} } }),
}));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    watchBacklogItems: () =>
      (async function* () {
        /* no events */
      })(),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

const getBacklogItem = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);
const transitionStatus = jest.fn().mockResolvedValue(true);
const triggerTriage = jest.fn().mockResolvedValue(undefined);

jest.mock("@/lib/hooks/useBacklogService", () => ({
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus,
    triggerTriage,
    cancelTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

// vanilla-extract .css.ts jest mock wraps every export in a callable proxy,
// which triggers a benign "Invalid value for prop className" warning —
// pre-existing limitation, silenced the same way BacklogItemDetail.test.tsx does.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  transitionStatus.mockClear().mockResolvedValue(true);
  triggerTriage.mockClear().mockResolvedValue(undefined);
});

function makeFailedTriageItem(status: BacklogItemStatus): BacklogItem {
  return {
    id: "item-1",
    title: "Two-way linkage + status/label sync",
    description: "desc",
    status,
    priority: 2,
    repoPath: "/tmp/repo",
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    planArtifactsPath: undefined,
    acCriteria: [],
    // A single triage-role session that ended with no summary — mapBacklogItem
    // derives triageStatus: "failed" from exactly this shape, regardless of
    // item.status (see useBacklogService.ts's mapBacklogItem).
    linkedSessions: [
      {
        entityId: "session-entity-1",
        sessionId: "headless-triage-1",
        role: "triage",
        startedAt: "2026-08-02T18:00:34Z",
        endedAt: "2026-08-03T02:52:54Z",
        estimatedCostUsd: 0,
      },
    ],
    notes: "",
    createdAt: "2026-08-02T18:00:00Z",
    updatedAt: "2026-08-03T02:52:54Z",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    // getBacklogItem returns the already-mapped BacklogItem shape (mapBacklogItem
    // runs inside the real hook, not on this mock's return value) — triageStatus
    // and triageResult must be set explicitly here to match what mapBacklogItem
    // would have derived from the ended, summary-less session above.
    triageStatus: "failed",
    triageResult: undefined,
  };
}

async function renderItem(status: BacklogItemStatus) {
  getBacklogItem.mockReset().mockResolvedValue(makeFailedTriageItem(status));
  render(<BacklogItemDetail itemId="item-1" />);
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("BacklogItemDetail — triage-produced-no-usable-result banner", () => {
  it("renders the failed-triage banner with a retry action for a queued item gated on plan approval (be676dab shape)", async () => {
    await renderItem("queued");

    // A queued, gated, no-plan item now also exposes a distinct "Retry Triage"
    // button in the Actions section (getAvailableActions) alongside this
    // banner's own inline retry — both fixes for the same be676dab dead end,
    // so scope this query to the banner itself to disambiguate.
    const banner = await screen.findByRole("alert");
    expect(banner).toHaveTextContent(/ended without producing a usable plan/i);
    expect(within(banner).getByRole("button", { name: /retry/i })).toBeInTheDocument();
  });

  it("still renders the failed-triage banner for an idea-status item (no regression)", async () => {
    await renderItem("idea");

    const banner = await screen.findByRole("alert");
    expect(banner).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /retry/i })).toBeInTheDocument();
  });

  it("does not render the banner for a ready-status item (deliberately out of scope — see itemActions.ts)", async () => {
    await renderItem("ready");

    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("resets the queued item to idea before re-triggering triage when Retry is clicked", async () => {
    await renderItem("queued");

    const banner = await screen.findByRole("alert");
    const retryButton = within(banner).getByRole("button", { name: /retry/i });
    await act(async () => {
      fireEvent.click(retryButton);
      await Promise.resolve();
    });

    expect(transitionStatus).toHaveBeenCalledWith("item-1", "idea");
    expect(triggerTriage).toHaveBeenCalledWith("item-1");
    // Reset must happen before the retriggered call, not merely both being called.
    const resetOrder = transitionStatus.mock.invocationCallOrder[0];
    const triageOrder = triggerTriage.mock.invocationCallOrder[0];
    expect(resetOrder).toBeLessThan(triageOrder);
  });

  it("does NOT reset status before re-triggering triage for an idea-status item", async () => {
    await renderItem("idea");

    const retryButton = await screen.findByRole("button", { name: /retry/i });
    await act(async () => {
      fireEvent.click(retryButton);
      await Promise.resolve();
    });

    expect(transitionStatus).not.toHaveBeenCalled();
    expect(triggerTriage).toHaveBeenCalledWith("item-1");
  });

  // MINOR finding from PR #322 review: InlineError (unmodified, pre-existing)
  // has no built-in disabled/loading state, so a double-click on its Retry
  // button used to be able to fire two concurrent retriggerTriageCore calls —
  // worse for a queued item than the pre-existing idea-only exposure, since
  // each call does its own transitionStatus(id,"idea") + triggerTriage(id).
  // BacklogItemDetail now threads the same actionLoading guard
  // TriageLoadingIndicator's onCancel already used, swapping onRetry to a
  // no-op while a retry is in flight.
  it("does not dispatch a second retry while the first is still in flight (double-click guard)", async () => {
    await renderItem("queued");

    const banner = await screen.findByRole("alert");
    const retryButton = within(banner).getByRole("button", { name: /retry/i });

    fireEvent.click(retryButton);
    fireEvent.click(retryButton);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(transitionStatus).toHaveBeenCalledTimes(1);
    expect(triggerTriage).toHaveBeenCalledTimes(1);
  });
});
