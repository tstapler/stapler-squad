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

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

jest.mock("@/lib/analytics/usePageView", () => ({
  usePageView: () => {},
}));

jest.mock("@/components/backlog/BacklogItemDetail", () => ({
  BacklogItemDetail: () => null,
}));
jest.mock("@/components/backlog/BacklogItemForm", () => ({
  BacklogItemForm: () => null,
}));
jest.mock("@/components/backlog/BacklogEmptyState", () => ({
  BacklogEmptyState: () => null,
  FilterZeroState: () => null,
  FooterNudge: () => null,
}));
jest.mock("@/components/backlog/VaguenessPromptModal", () => ({
  VaguenessPromptModal: () => null,
}));
jest.mock("@/components/backlog/BacklogTourModal", () => ({
  BacklogTourModal: () => null,
}));
jest.mock("@/components/backlog/GitHubIssuePicker", () => ({
  GitHubIssuePicker: () => null,
}));

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getBacklogItem: jest.fn() }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => jest.fn(),
}));

jest.mock("@/lib/store", () => ({
  useAppDispatch: () => jest.fn(),
}));

jest.mock("@/lib/hooks/useBacklogService", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogService");
  return {
    ...actual,
    useBacklogService: () => ({
      createBacklogItem: jest.fn(),
      importGitHubIssue: jest.fn(),
      triggerTriage: jest.fn(),
    }),
  };
});

const mockUseWatchBacklogItems = jest.fn();
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => mockUseWatchBacklogItems(),
}));

function itemFixture(overrides: Record<string, unknown>) {
  return {
    id: "item-1",
    title: "Fix retry loop in triage",
    status: "in_progress",
    priority: 3,
    acCriteria: [],
    liveVersion: 1,
    ...overrides,
  } as any;
}

describe("BacklogPage filtered-list exit transition (Epic 6.3)", () => {
  beforeEach(() => {
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
