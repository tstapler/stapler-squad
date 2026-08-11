/**
 * Epic 6.4 (backlog-event-driven-updates): Kanban board column-transition
 * fade/entry.
 *
 * design/ux.md §7 + UX Acceptance Criterion #8: when a live status change
 * moves an item between board columns, the card should briefly fade out of
 * its origin column while the freshly mounted card in its destination column
 * force-flashes ("just changed") -- both driven by the same event, so a human
 * tester perceives it as "moved" rather than "one card disappeared, an
 * unrelated card appeared." Only a genuinely live, one-at-a-time transition
 * animates (gated on `item.liveVersion` advancing, mirroring Epic 6.3's list
 * exit-fade in BacklogPage.exitTransition.test.tsx) -- a bulk resnapshot on
 * reconnect must move instantly, and `prefers-reduced-motion: reduce`
 * collapses both the exit and entry effects to instant.
 */

import React from "react";
import { act, render, screen, within } from "@testing-library/react";
import { BacklogBoard } from "./BacklogBoard";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";

jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: jest.fn(),
}));
const mockUseWatchBacklogItems = useWatchBacklogItems as jest.Mock;

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Fix retry loop in triage",
    status: "in_progress",
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
    liveVersion: 1,
    ...overrides,
  } as BacklogItem;
}

function inProgressColumn() {
  return screen.getByTestId("backlog-column-in_progress");
}
function reviewColumn() {
  return screen.getByTestId("backlog-column-review");
}

describe("BacklogBoard column transition (Epic 6.4)", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("BacklogBoard_should_FadeCardOutOfOriginAndFlashIntoDestination_When_StatusChangesViaGenuineLiveUpdate", () => {
    const v1 = makeItem({ status: "in_progress", liveVersion: 1 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(
      <BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />
    );
    expect(inProgressColumn()).toHaveTextContent("Fix retry loop in triage");

    // Live event: status flips to "review" and liveVersion advances -- a
    // genuine live delta, moving the item to a new column.
    const v2 = makeItem({ status: "review", liveVersion: 2 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />);

    // Immediately after: the card is still present in the origin column,
    // marked as exiting (reads as "moved," not "vanished") ...
    const exitingCard = screen.getByTestId("backlog-card-exiting");
    expect(exitingCard).toHaveAttribute("aria-hidden", "true");
    expect(inProgressColumn()).toContainElement(exitingCard);

    // ... and has already appeared in the destination column, flashed.
    const destinationCard = within(reviewColumn()).getByTestId("backlog-item-card");
    expect(destinationCard.className).toMatch(/justChanged/);

    // After the ~200ms fade completes, the origin column's copy is removed.
    act(() => {
      jest.advanceTimersByTime(250);
    });
    expect(screen.queryByTestId("backlog-card-exiting")).not.toBeInTheDocument();
    expect(inProgressColumn()).not.toHaveTextContent("Fix retry loop in triage");
  });

  it("BacklogBoard_should_MoveCardInstantly_When_ColumnChangeIsFromBulkResnapshotNotLiveEvent", () => {
    const v1 = makeItem({ status: "in_progress", liveVersion: 1 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(
      <BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />
    );
    expect(inProgressColumn()).toHaveTextContent("Fix retry loop in triage");

    // Resnapshot-style correction: status changes but liveVersion does NOT
    // advance (mirrors backlogItemsSlice.ts only bumping liveVersion for
    // is_snapshot: false events) -- must move instantly, no fade/flash.
    const v2 = makeItem({ status: "review" });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />);

    expect(screen.queryByTestId("backlog-card-exiting")).not.toBeInTheDocument();
    expect(inProgressColumn()).not.toHaveTextContent("Fix retry loop in triage");
    const destinationCard = within(reviewColumn()).getByTestId("backlog-item-card");
    expect(destinationCard.className).not.toMatch(/justChanged/);
  });

  it("BacklogBoard_should_CollapseToInstantMove_When_ReducedMotionIsPreferred", () => {
    jest.spyOn(window, "matchMedia").mockImplementation((query: string) => ({
      matches: query.includes("prefers-reduced-motion: reduce"),
      media: query,
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })) as unknown as typeof window.matchMedia;

    const v1 = makeItem({ status: "in_progress", liveVersion: 1 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(
      <BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />
    );
    expect(inProgressColumn()).toHaveTextContent("Fix retry loop in triage");

    const v2 = makeItem({ status: "review", liveVersion: 2 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />);

    // The exiting card's timer duration collapses to 0ms under reduced
    // motion, so it's removed on the very next tick rather than lingering
    // for the full ~200ms fade.
    act(() => {
      jest.advanceTimersByTime(0);
    });
    expect(screen.queryByTestId("backlog-card-exiting")).not.toBeInTheDocument();
    expect(inProgressColumn()).not.toHaveTextContent("Fix retry loop in triage");
  });

  it("BacklogBoard_should_SettleInDestinationColumn_When_ItemFlapsBackToOriginDuringExitTransition", () => {
    const v1 = makeItem({ status: "in_progress", liveVersion: 1 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(
      <BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />
    );

    // Leaves in_progress via a genuine live change -> starts exiting there.
    const v2 = makeItem({ status: "review", liveVersion: 2 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />);
    expect(screen.getByTestId("backlog-card-exiting")).toBeInTheDocument();

    // Flaps back to in_progress before the ~200ms exit transition completes.
    const v3 = makeItem({ status: "in_progress", liveVersion: 3 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v3], connectionState: "live" });
    rerender(<BacklogBoard onAction={jest.fn()} onItemClick={jest.fn()} />);

    // Settles back in_progress -- the pending exit is cancelled, not
    // finished (no lingering exiting duplicate in the review column).
    expect(screen.queryByTestId("backlog-card-exiting")).not.toBeInTheDocument();

    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(inProgressColumn()).toHaveTextContent("Fix retry loop in triage");
    expect(reviewColumn()).not.toHaveTextContent("Fix retry loop in triage");
  });
});
