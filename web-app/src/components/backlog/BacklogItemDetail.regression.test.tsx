/**
 * Regression test for stapler-squad#146: acceptance criteria disappear while
 * editing a backlog item during triage, because the loading-state guard
 * unmounts <BacklogItemForm> on every 5s triage poll.
 *
 * Verified this fails on the pre-fix code (view collapses to "Loading…", the
 * item content asserted below disappears) and passes with the fix.
 */

import React from "react";
import { render, screen, act, fireEvent } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

// Heavy children pull their own hooks/timers; stub them out so this test is
// focused on BacklogItemDetail's own render behavior during a background refresh.
jest.mock("./SessionMonitor", () => ({ SessionMonitor: () => null }));
jest.mock("./GateVerdictBox", () => ({ GateVerdictBox: () => null }));
jest.mock("./TriageReviewPanel", () => ({ TriageReviewPanel: () => null }));
jest.mock("./TriageLoadingIndicator", () => ({ TriageLoadingIndicator: () => null }));

// The edit-mode branch renders BacklogItemForm → RepoPathInput, which uses
// useSessionRepoPaths (Redux) and usePathCompletions (RPC). Stub both so this
// test doesn't need a Redux store or ConnectRPC transport.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

// useSessionService pulls in useAnalytics, which requires an
// AnalyticsContextProvider we don't want to stand up for this focused test.
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession: jest.fn() }),
}));

// BacklogItemDetail itself also calls useAnalytics() directly (for the
// session-delete tracking event) — mock it the same way. Without this,
// render() throws "useAnalytics must be used within an
// AnalyticsContextProvider" since no provider is mounted in this test.
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

// Epic 5.3 (backlog-event-driven-updates): BacklogItemDetail now also
// subscribes via useWatchBacklogItems + a Redux selector, and opens its own
// lightweight raw watch stream for archive/removal terminal-state detection
// (Task 5.3.1b/5.3.1c). The raw terminal stream is stubbed inert (no events)
// for every test in this file. The store selector mock is controllable via
// `mockLiveItemsMap` (module-scope, reset in beforeEach) — the first test
// below uses it to simulate the live update that now replaces the deleted
// 5s poll.
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => ({ items: [], connectionState: "live" }),
}));
let mockLiveItemsMap: Record<string, unknown> = {};
jest.mock("@/lib/store", () => ({
  useAppSelector: (selector: (state: unknown) => unknown) =>
    selector({ backlogItems: { items: mockLiveItemsMap } }),
}));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    watchBacklogItems: () => (async function* () {})(),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

// BacklogItemDetail calls useStuckBacklogItems() once and passes the
// resolved StuckBacklogItem down to LifecycleSummary as a prop — stub it so
// this suite never attempts a real ConnectRPC call.
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => ({ items: [], isLoading: false, error: null }),
}));


const getBacklogItem = jest.fn();
// Epic 3.4: BacklogItemDetail now fetches the mode list on mount for the
// "what ran" surface, via a `useEffect` keyed on `listPipelineModes`. This
// must be a stable (module-scope) reference like `getBacklogItem` above —
// declaring it inline inside the mock factory below would recreate a new
// jest.fn() on every render (the factory re-runs each time
// `useBacklogService()` is called), making the effect's dependency change
// every render and re-fire in an infinite loop (each resolution calls
// `setPipelineModes`, forcing a re-render, forcing the effect to refire).
const listPipelineModes = jest.fn().mockResolvedValue([]);

jest.mock("@/lib/hooks/useBacklogService", () => ({
  // mapBacklogItem is a real (unmocked) named export required by
  // BacklogItemDetail.tsx's live-update effect (Task 5.3.1b) to convert the
  // raw proto item read off the mocked store above into the domain shape
  // this component renders.
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    archiveBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

const baseItem: BacklogItem = {
  id: "item-1",
  title: "Jira hygiene checker",
  description: "desc",
  status: "idea",
  priority: 3,
  repoPath: "/tmp/repo",
  skipPlanning: false,
  skipReviewGate: false,
  autoSpawnSession: false,
  autoCreatePR: false,
  planApproved: false,
  // triageStatus "running" is what enables the 5s background poll.
  triageStatus: "running",
  acCriteria: [{ index: 0, text: "Define the watermelon signal", status: "pending" }],
  linkedSessions: [],
  notes: "",
  createdAt: "2026-07-01T00:00:00Z",
  updatedAt: "2026-07-01T00:00:00Z",
  statusEvents: [],
  progressNotes: [],
  totalEstimatedCostUsd: 0,
};

function makeReviewItem(id: string, overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    ...baseItem,
    id,
    title: `Review item ${id}`,
    status: "review",
    triageStatus: undefined,
    gateVerdict: "PENDING",
    ...overrides,
  };
}

/**
 * Test harness mirroring the fix in web-app/src/app/backlog/page.tsx:
 * `<BacklogItemDetail key={itemId} itemId={itemId} .../>` — the `key` is
 * what forces a full remount when the selected item changes (Story 3.1.1,
 * Task 3.1.1a).
 */
function Harness({ itemId }: { itemId: string }) {
  return <BacklogItemDetail key={itemId} itemId={itemId} />;
}

describe("BacklogItemDetail — background refresh must not unmount the view", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    getBacklogItem.mockReset();
    mockLiveItemsMap = {};
    // Story 3.1.4's useSectionExpandState persists collapse state to
    // localStorage keyed by itemId — clear between tests so one test's
    // expand/collapse interactions never leak into the next.
    localStorage.clear();
  });
  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  // Epic 5.3 (Story 5.3.1) deletes the 5s `shouldPoll` interval this test
  // used to exercise directly (`BacklogItemDetail.tsx:245` pre-Epic-5.3) —
  // live updates now arrive via the store selector instead. The original
  // stapler-squad#146 guarantee ("a background refresh must not unmount the
  // view / lose in-flight content") is re-anchored to that new mechanism
  // here rather than dropped.
  it("keeps item content visible when a live update arrives for the open item", async () => {
    getBacklogItem.mockResolvedValue(baseItem);

    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);

    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.getByText("Jira hygiene checker")).toBeInTheDocument();
    expect(screen.getByText("Define the watermelon signal")).toBeInTheDocument();

    // Simulate a live BacklogItemUpdatedEvent for this item landing in the
    // shared store — this is what now keeps the panel fresh instead of the
    // deleted poll.
    mockLiveItemsMap = {
      "item-1": {
        id: "item-1",
        title: "Jira hygiene checker",
        status: "idea",
        priority: 3,
        acceptanceCriteria: [{ index: 0, text: "Define the watermelon signal", status: "pending" }],
      },
    };

    await act(async () => {
      rerender(<BacklogItemDetail itemId="item-1" />);
      await Promise.resolve();
    });

    // With the old bug, a background refresh briefly set `loading` and the
    // view collapsed to "Loading…", losing the content below. The new live
    // path never touches `loading` at all, so this is structurally
    // impossible to regress via this mechanism — assert the content simply
    // stays mounted and visible.
    expect(screen.getByText("Jira hygiene checker")).toBeInTheDocument();
    expect(screen.getByText("Define the watermelon signal")).toBeInTheDocument();
    // The initial single-item fetch is not re-issued by the live update path.
    expect(getBacklogItem).toHaveBeenCalledTimes(1);
  });

  it("suspends the triage poll entirely while the edit form is open", async () => {
    getBacklogItem.mockResolvedValue(baseItem);

    render(<BacklogItemDetail itemId="item-1" />);

    await act(async () => {
      await Promise.resolve();
    });
    expect(getBacklogItem).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Edit item" }));
    expect(screen.getByTestId("backlog-repo-path-input")).toBeInTheDocument();

    // Type an unsaved acceptance criterion — this is the actual data the
    // original bug destroyed. Asserting it survives (not just the fetch call
    // count) is the real behavioral concern, not an implementation detail.
    fireEvent.click(screen.getByTestId("backlog-add-criterion"));
    fireEvent.change(screen.getByTestId("backlog-criterion-text-1"), {
      target: { value: "Encode Fix Version" },
    });

    // The poll interval would normally fire here — it must not while editing.
    await act(async () => {
      jest.advanceTimersByTime(10_000);
      await Promise.resolve();
    });

    expect(getBacklogItem).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("backlog-criterion-text-1")).toHaveValue("Encode Fix Version");
  });
});

describe("BacklogItemDetail — Story 3.1.1: itemId state-reset fix", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    getBacklogItem.mockReset();
    // Story 3.1.4's useSectionExpandState persists collapse state to
    // localStorage keyed by itemId — clear between tests so one test's
    // expand/collapse interactions never leak into the next.
    localStorage.clear();
  });
  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  it("BacklogItemDetail_should_RemountAndCloseManualReviewForm_When_KeyedItemIdChangesFromAToB", async () => {
    const itemA = makeReviewItem("itm_a");
    const itemB = makeReviewItem("itm_b");
    getBacklogItem.mockImplementation((id: string) =>
      Promise.resolve(id === "itm_a" ? itemA : itemB)
    );

    const { rerender } = render(<Harness itemId="itm_a" />);
    await act(async () => {
      await Promise.resolve();
    });

    fireEvent.click(screen.getByTestId("backlog-action-manual-review"));
    expect(screen.getByTestId("manual-review-form")).toBeInTheDocument();

    rerender(<Harness itemId="itm_b" />);
    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.queryByTestId("manual-review-form")).not.toBeInTheDocument();
  });

  it("BacklogItemDetail_should_PreserveManualReviewFormOpen_When_SameItemIdRerendersWithFreshPollData", async () => {
    let pollCount = 0;
    getBacklogItem.mockImplementation(() => {
      pollCount += 1;
      return Promise.resolve(makeReviewItem("itm_a", { updatedAt: `2026-07-01T00:0${pollCount}:00Z` }));
    });

    const { rerender } = render(<Harness itemId="itm_a" />);
    await act(async () => {
      await Promise.resolve();
    });

    fireEvent.click(screen.getByTestId("backlog-action-manual-review"));
    expect(screen.getByTestId("manual-review-form")).toBeInTheDocument();

    // Same itemId → same `key` → React keeps the existing instance mounted
    // (proving the remount fix doesn't over-trigger on unrelated
    // re-renders). Note: since Story 3.1.3, polling is itself suspended
    // while the manual-review form is open (Task 3.1.3c) — the poll tick
    // below is a no-op for that reason, which only reinforces the form
    // staying open; the parent-level rerender is what this test is
    // actually proving doesn't reset local component state.
    rerender(<Harness itemId="itm_a" />);
    await act(async () => {
      jest.advanceTimersByTime(5_000);
      await Promise.resolve();
    });

    expect(pollCount).toBe(1);
    expect(screen.getByTestId("manual-review-form")).toBeInTheDocument();
  });
});
