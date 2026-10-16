/**
 * Epic 6.3 (backlog-event-driven-updates): filtered-list exit transition.
 *
 * design/ux.md §7: when a live status change causes an item to stop matching
 * the active list filter, the row should briefly fade out (~200ms) instead of
 * vanishing in the same render the filter re-evaluates — but only for a
 * genuinely live, one-at-a-time departure. A bulk resnapshot on reconnect (no
 * `liveVersion` advance, see backlogItemsSlice.ts) removes instantly, and an
 * item that flaps back into the filter before its exit timer fires must
 * settle back to a normal in-place row rather than finish unmounting.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import BacklogPage from "../page";
import { itemFixture, mockUseWatchBacklogItems } from "./backlogPageTestFixtures";

jest.mock("next/navigation", () => require("./backlogPageTestFixtures").nextNavigationMock());
jest.mock("@/lib/analytics", () => require("./backlogPageTestFixtures").analyticsMock());
jest.mock("@/lib/analytics/usePageView", () => require("./backlogPageTestFixtures").usePageViewMock());
jest.mock("@/components/backlog/BacklogItemDetail", () => require("./backlogPageTestFixtures").backlogItemDetailMock());
jest.mock("@/components/backlog/BacklogItemForm", () => require("./backlogPageTestFixtures").backlogItemFormMock());
jest.mock("@/components/backlog/BacklogEmptyState", () => require("./backlogPageTestFixtures").backlogEmptyStateMock());
jest.mock("@/components/backlog/VaguenessPromptModal", () => require("./backlogPageTestFixtures").vaguenessPromptModalMock());
jest.mock("@/components/backlog/BacklogTourModal", () => require("./backlogPageTestFixtures").backlogTourModalMock());
jest.mock("@/components/backlog/GitHubIssuePicker", () => require("./backlogPageTestFixtures").gitHubIssuePickerMock());
jest.mock("@connectrpc/connect", () => require("./backlogPageTestFixtures").connectMock());
jest.mock("@connectrpc/connect-web", () => require("./backlogPageTestFixtures").connectWebMock());
jest.mock("@/lib/config", () => require("./backlogPageTestFixtures").configMock());
jest.mock("@/lib/store", () => require("./backlogPageTestFixtures").storeMock());
jest.mock("@/lib/hooks/useBacklogService", () => require("./backlogPageTestFixtures").useBacklogServiceMockFactory());
jest.mock("@/lib/hooks/useWatchBacklogItems", () => require("./backlogPageTestFixtures").useWatchBacklogItemsMock());

describe("BacklogPage filtered-list exit transition (Epic 6.3)", () => {
  beforeEach(() => {
    localStorage.clear();
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("BacklogPage_should_FadeRowOutThenRemove_When_ItemLeavesFilterViaGenuineLiveUpdate", () => {
    const v1 = itemFixture({});
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(<BacklogPage />);
    fireEvent.click(screen.getByTestId("backlog-filter-status-in_progress"));
    expect(screen.getByText("Fix retry loop in triage")).toBeInTheDocument();

    // Live event: status flips to "review" (no longer matches the in_progress
    // filter) and liveVersion advances -- a genuine live delta.
    const v2 = itemFixture({ status: "review", liveVersion: 2 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogPage />);

    // The row is still present immediately after, marked as exiting rather
    // than gone -- reads as "moved," not "vanished."
    const row = screen.getByTestId("backlog-table-row");
    expect(row).toHaveAttribute("data-exiting", "true");
    expect(row).toHaveAttribute("aria-hidden", "true");

    // After the ~200ms fade completes, it's actually removed from the DOM.
    act(() => {
      jest.advanceTimersByTime(250);
    });
    expect(screen.queryByTestId("backlog-table-row")).not.toBeInTheDocument();
  });

  it("BacklogPage_should_RemoveRowInstantly_When_DepartureIsFromBulkResnapshotNotLiveEvent", () => {
    const v1 = itemFixture({});
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(<BacklogPage />);
    fireEvent.click(screen.getByTestId("backlog-filter-status-in_progress"));
    expect(screen.getByText("Fix retry loop in triage")).toBeInTheDocument();

    // Resnapshot-style correction: status changes but liveVersion does NOT
    // advance (mirrors backlogItemsSlice.ts only bumping liveVersion for
    // is_snapshot: false events) -- must remove instantly, no fade.
    const v2 = itemFixture({ status: "review" });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogPage />);

    expect(screen.queryByTestId("backlog-table-row")).not.toBeInTheDocument();
  });

  it("BacklogPage_should_SettleInPlace_When_ItemFlapsBackIntoFilterDuringExitTransition", () => {
    const v1 = itemFixture({});
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(<BacklogPage />);
    fireEvent.click(screen.getByTestId("backlog-filter-status-in_progress"));
    expect(screen.getByText("Fix retry loop in triage")).toBeInTheDocument();

    // Leaves the filter via a genuine live change -> starts exiting.
    const v2 = itemFixture({ status: "review", liveVersion: 2 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogPage />);
    expect(screen.getByTestId("backlog-table-row")).toHaveAttribute("data-exiting", "true");

    // Flaps back into the filter before the ~200ms transition completes.
    const v3 = itemFixture({ status: "in_progress", liveVersion: 3 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v3], connectionState: "live" });
    rerender(<BacklogPage />);

    // Settles back to a normal in-place row -- the exit is cancelled, not
    // finished (the DOM node is never fully removed in between).
    const row = screen.getByTestId("backlog-table-row");
    expect(row).not.toHaveAttribute("data-exiting");

    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(screen.getByText("Fix retry loop in triage")).toBeInTheDocument();
  });
});
