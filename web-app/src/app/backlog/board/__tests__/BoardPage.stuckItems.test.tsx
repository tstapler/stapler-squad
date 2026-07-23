/**
 * Story 5.1.1 (plan.md Task 5.1.1a): useStuckBacklogItems() must be called
 * ONCE at board/page.tsx level, not once per card — otherwise N cards means
 * N independent 60s polls, wasting requests and risking cards updating out
 * of sync with each other on the same screen. This verifies the single-call
 * contract and that the resolved items are correctly distributed to the
 * matching card by itemId.
 */

import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import BacklogBoardPage from "../page";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
}));

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Some backlog item",
    status: "idea",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

function makeStuckItem(overrides: Partial<StuckBacklogItem> = {}): StuckBacklogItem {
  return {
    itemId: "item-2",
    title: "Second item",
    status: "in_progress",
    reason: StuckReason.STALE_WORK,
    firstDetectedAt: timestampFromDate(new Date(Date.now() - 60 * 60 * 1000)),
    lastCheckedAt: timestampFromDate(new Date(Date.now() - 30 * 1000)),
    prNumber: 0,
    prUrl: "",
    context: "",
    ...overrides,
  } as StuckBacklogItem;
}

jest.mock("@/lib/hooks/useBacklogService", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogService");
  return {
    ...actual,
    useBacklogService: () => ({
      transitionStatus: jest.fn(),
      triggerTriage: jest.fn(),
      spawnSessionFromItem: jest.fn(),
      cancelTriage: jest.fn(),
    }),
  };
});

// BacklogBoard (Epic 5.2, backlog-event-driven-updates) now sources its
// items from the live useWatchBacklogItems stream/store directly instead of
// a `listBacklogItems`-fed `items` prop — mock the hook so this suite can
// still feed it fixture items without a real Redux store/ConnectRPC client.
const mockUseWatchBacklogItems = jest.fn();
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: (...args: unknown[]) => mockUseWatchBacklogItems(...args),
}));

const useStuckBacklogItemsMock = jest.fn();

jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: (...args: unknown[]) => useStuckBacklogItemsMock(...args),
}));

describe("BacklogBoardPage — single useStuckBacklogItems() call (Story 5.1.1)", () => {
  beforeEach(() => {
    mockUseWatchBacklogItems.mockReset();
    useStuckBacklogItemsMock.mockReset();
  });

  it("BoardPage_should_CallUseStuckBacklogItemsOnceAndDistributeResolvedItemsPerCard_When_MultipleCardsRender", async () => {
    const items = [
      makeItem({ id: "item-1", title: "First item", status: "idea" }),
      makeItem({ id: "item-2", title: "Second item", status: "idea" }),
    ];
    mockUseWatchBacklogItems.mockReturnValue({ items, connectionState: "live" });
    useStuckBacklogItemsMock.mockReturnValue({
      items: [makeStuckItem({ itemId: "item-2" })],
      isLoading: false,
      error: null,
    });

    render(<BacklogBoardPage />);

    await waitFor(() => expect(screen.getAllByTestId("backlog-item-card")).toHaveLength(2));

    const callsFor2Cards = useStuckBacklogItemsMock.mock.calls.length;

    const cards = screen.getAllByTestId("backlog-item-card");
    const stuckCard = cards.find((c) => c.getAttribute("data-item-id") === "item-2")!;
    const clearCard = cards.find((c) => c.getAttribute("data-item-id") === "item-1")!;

    expect(stuckCard.querySelector('[data-testid="blocker-chip"]')).toBeInTheDocument();
    expect(clearCard.querySelector('[data-testid="blocker-chip"]')).not.toBeInTheDocument();

    // Prove the "not N independent polls" contract: re-render the same page
    // component with 5 cards instead of 2 and assert the hook's call count
    // (which only grows with BacklogBoardPage's own re-renders, driven by its
    // own load-state transitions) does NOT scale with the number of cards —
    // i.e. the hook is invoked once per page render, never once per card.
    useStuckBacklogItemsMock.mockClear();
    mockUseWatchBacklogItems.mockReturnValue({
      items: [
        makeItem({ id: "item-1", title: "First item", status: "idea" }),
        makeItem({ id: "item-2", title: "Second item", status: "idea" }),
        makeItem({ id: "item-3", title: "Third item", status: "idea" }),
        makeItem({ id: "item-4", title: "Fourth item", status: "idea" }),
        makeItem({ id: "item-5", title: "Fifth item", status: "idea" }),
      ],
      connectionState: "live",
    });

    render(<BacklogBoardPage />);

    await waitFor(() => expect(screen.getAllByTestId("backlog-item-card")).toHaveLength(7));

    const callsFor5Cards = useStuckBacklogItemsMock.mock.calls.length;
    expect(callsFor5Cards).toBe(callsFor2Cards);
  });
});
