/**
 * Tests for BacklogItemPanel — Epic 5.4 (backlog-event-driven-updates,
 * Story 5.4.1), the 4th `useWatchBacklogItems` consumer, embedded inside
 * `SessionDetail`/`SessionDetailView` for a session's linked backlog item.
 *
 * Covers:
 *  1. Initial load via getBacklogItem (unchanged first-paint behavior).
 *  2. Live updates: a store-level change to the linked item (via
 *     selectBacklogItemById) is reflected without re-fetching.
 *  3. Task 5.4.1c: BacklogItemArchivedEvent/BacklogItemRemovedEvent for the
 *     linked item replaces the "View full item" action with a terminal-state
 *     InlineNotice, mirroring BacklogItemDetail's Task 5.3.1c mechanism.
 */

import React from "react";
import { render, screen, waitFor, act } from "@testing-library/react";
import { BacklogItemPanel } from "./BacklogItemPanel";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

const getBacklogItem = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  // mapBacklogItem is a real (unmocked) named export — the panel's live-update
  // effect calls it to convert the raw proto item read off the mocked store
  // below into the domain shape this component renders.
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({ getBacklogItem }),
}));

// Epic 5.4: BacklogItemPanel subscribes via useWatchBacklogItems (unfiltered,
// just to keep the shared store connected) + a Redux selector (Task 5.4.1b),
// and opens its own lightweight raw watch stream for archive/removal
// terminal-state detection (Task 5.4.1c). Both are controllable per-test via
// the module-scope `mock*` holders below, reset in `beforeEach`.
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => ({ items: [], connectionState: "live" }),
}));

let mockLiveItemsMap: Record<string, unknown> = {};
jest.mock("@/lib/store", () => ({
  useAppSelector: (selector: (state: unknown) => unknown) =>
    selector({ backlogItems: { items: mockLiveItemsMap } }),
}));

// Raw events the terminal-state watch stream (Task 5.4.1c) yields on its next
// connection — set before render() (the stream is opened once, on mount).
let mockTerminalStreamEvents: Array<{ event: { case: string; value: { itemId: string } } }> = [];
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    watchBacklogItems: () =>
      (async function* () {
        for (const e of mockTerminalStreamEvents) {
          yield e;
        }
      })(),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => jest.fn(),
}));

// The jest styleMock for `.css.ts` files wraps every export (including plain
// `style()` string exports) in a callable proxy function, which triggers a
// benign "Invalid value for prop className" React warning. Pre-existing
// jest/vanilla-extract mock limitation — see BacklogItemDetail.test.tsx and
// RadioGroup.test.tsx, which silence it the same way.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  getBacklogItem.mockReset();
  mockLiveItemsMap = {};
  mockTerminalStreamEvents = [];
  window.localStorage.clear();
});

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Fix retry loop in triage runner",
    status: "review",
    priority: 2,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [{ index: 0, text: "Do the thing", status: "pending" }],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

/** Raw proto-shaped item, as it lives in backlogItemsSlice's normalized store. */
function makeRawItem(overrides: Record<string, unknown> = {}) {
  return {
    id: "item-1",
    title: "Fix retry loop in triage runner",
    status: "in_progress",
    priority: 2,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePr: false,
    planApproved: false,
    acceptanceCriteria: [],
    itemSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

async function renderOpenPanel() {
  window.localStorage.setItem("backlog-panel-session-1", "open");
  const result = render(
    <BacklogItemPanel backlogItemId="item-1" sessionId="session-1" />
  );
  await waitFor(() => expect(screen.getByTestId("backlog-panel-title")).toBeInTheDocument());
  return result;
}

describe("BacklogItemPanel — initial load", () => {
  it("fetches the linked item via getBacklogItem and renders it", async () => {
    getBacklogItem.mockResolvedValue(makeItem());

    await renderOpenPanel();

    expect(getBacklogItem).toHaveBeenCalledWith("item-1");
    expect(screen.getByTestId("backlog-panel-title")).toHaveTextContent(
      "Fix retry loop in triage runner"
    );
    expect(screen.getByTestId("backlog-panel-view-full")).toBeInTheDocument();
  });
});

describe("BacklogItemPanel — live updates (Task 5.4.1b)", () => {
  it("reflects a status change applied to the store without re-fetching", async () => {
    getBacklogItem.mockResolvedValue(makeItem({ status: "review" }));

    const { rerender } = await renderOpenPanel();
    expect(screen.getByTestId("backlog-panel")).toHaveTextContent("review");

    // Simulate a live event landing in the normalized store (as
    // useWatchBacklogItems.ts's dispatch(upsertItem(...)) would do).
    mockLiveItemsMap = { "item-1": makeRawItem({ status: "done" }) };
    // Re-render the same component instance to pick up the new selector
    // value the mocked useAppSelector returns.
    rerender(<BacklogItemPanel backlogItemId="item-1" sessionId="session-1" />);

    await waitFor(() =>
      expect(screen.getByTestId("backlog-panel")).toHaveTextContent("done")
    );
    // Only the initial mount fetch happened — the live update did not trigger
    // a second getBacklogItem call for the already-open panel.
    expect(getBacklogItem).toHaveBeenCalledTimes(1);
  });
});

describe("BacklogItemPanel — verdict display (sweep fix, Story 5.4.1 AC / ux.md AC #14)", () => {
  it("renders a verdict badge and summary when the linked item has a recorded gate verdict", async () => {
    getBacklogItem.mockResolvedValue(
      makeItem({ status: "review", gateVerdict: "FAIL", gateVerdictSummary: "2 of 5 criteria did not pass" })
    );

    await renderOpenPanel();

    await waitFor(() => expect(screen.getByTestId("backlog-panel-verdict")).toBeInTheDocument());
    expect(screen.getByTestId("backlog-panel-verdict")).toHaveTextContent("FAIL");
    expect(screen.getByTestId("backlog-panel-verdict")).toHaveTextContent(
      "2 of 5 criteria did not pass"
    );
  });

  it("does not render a verdict badge when no verdict has been recorded", async () => {
    getBacklogItem.mockResolvedValue(makeItem({ status: "in_progress" }));

    await renderOpenPanel();

    expect(screen.queryByTestId("backlog-panel-verdict")).not.toBeInTheDocument();
  });

  it("does not render a verdict badge while a verdict is only PENDING", async () => {
    getBacklogItem.mockResolvedValue(makeItem({ status: "review", gateVerdict: "PENDING" }));

    await renderOpenPanel();

    expect(screen.queryByTestId("backlog-panel-verdict")).not.toBeInTheDocument();
  });
});

describe("BacklogItemPanel — terminal state (Task 5.4.1c)", () => {
  it("replaces the View full item action with an InlineNotice when the linked item is archived", async () => {
    getBacklogItem.mockResolvedValue(makeItem());
    mockTerminalStreamEvents = [
      { event: { case: "itemArchived", value: { itemId: "item-1" } } },
    ];

    await renderOpenPanel();

    await waitFor(() =>
      expect(screen.getByTestId("backlog-panel-terminal-notice")).toBeInTheDocument()
    );
    expect(screen.getByTestId("backlog-panel-terminal-notice")).toHaveTextContent(
      "This item was archived elsewhere."
    );
    expect(screen.queryByTestId("backlog-panel-view-full")).not.toBeInTheDocument();
  });

  it("replaces the View full item action with an InlineNotice when the linked item is removed", async () => {
    getBacklogItem.mockResolvedValue(makeItem());
    mockTerminalStreamEvents = [
      { event: { case: "itemRemoved", value: { itemId: "item-1" } } },
    ];

    await renderOpenPanel();

    await waitFor(() =>
      expect(screen.getByTestId("backlog-panel-terminal-notice")).toBeInTheDocument()
    );
    expect(screen.getByTestId("backlog-panel-terminal-notice")).toHaveTextContent(
      "This item was removed elsewhere."
    );
    expect(screen.queryByTestId("backlog-panel-view-full")).not.toBeInTheDocument();
  });

  it("ignores archive/removed events for a different item", async () => {
    getBacklogItem.mockResolvedValue(makeItem());
    mockTerminalStreamEvents = [
      { event: { case: "itemArchived", value: { itemId: "some-other-item" } } },
    ];

    await renderOpenPanel();
    // Give the async stream a tick to (not) apply anything.
    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.queryByTestId("backlog-panel-terminal-notice")).not.toBeInTheDocument();
    expect(screen.getByTestId("backlog-panel-view-full")).toBeInTheDocument();
  });
});
