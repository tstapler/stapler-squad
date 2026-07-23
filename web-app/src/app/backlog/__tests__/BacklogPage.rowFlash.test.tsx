/**
 * Sweep fix (backlog-event-driven-updates Phase 5 compliance sweep,
 * 2026-07-22): ux.md UX AC #6 ("An item's status badge/label updates within
 * ~2 seconds of a server-side transition, with a background flash that fades
 * within ~1 second") was implemented for BacklogItemCard.tsx (used by the
 * Kanban board) but never wired into the /backlog list page's table rows —
 * the page only tracked `item.liveVersion` for the Epic 6.3 exit transition.
 * This covers the row-level `.justChanged`-equivalent flash added to close
 * that gap.
 */

import React from "react";
import { render, screen, act } from "@testing-library/react";
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

describe("BacklogPage list row flash (sweep fix, ux.md UX AC #6)", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("BacklogPage_should_FlashRow_When_ItemUpdatesLiveWhileStillMatchingTheFilter", () => {
    const v1 = itemFixture({});
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(<BacklogPage />);
    const row = screen.getByTestId("backlog-table-row");
    expect(row.className).not.toMatch(/tableRowJustChanged/);

    // Genuine live update (liveVersion advances) that still matches every
    // active filter (none set) -- row stays in place but should flash.
    const v2 = itemFixture({ status: "review", liveVersion: 2 });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogPage />);

    expect(screen.getByTestId("backlog-table-row").className).toMatch(/tableRowJustChanged/);

    act(() => {
      jest.advanceTimersByTime(300);
    });
    expect(screen.getByTestId("backlog-table-row").className).not.toMatch(/tableRowJustChanged/);
  });

  it("BacklogPage_should_NotFlashRow_When_UpdateIsASnapshotNotAGenuineLiveEvent", () => {
    const v1 = itemFixture({});
    mockUseWatchBacklogItems.mockReturnValue({ items: [v1], connectionState: "live" });

    const { rerender } = render(<BacklogPage />);

    // Resnapshot-style correction: fields change but liveVersion does not
    // advance (mirrors backlogItemsSlice.ts only bumping liveVersion for
    // is_snapshot: false events) -- must never flash.
    const v2 = itemFixture({ status: "review" });
    mockUseWatchBacklogItems.mockReturnValue({ items: [v2], connectionState: "live" });
    rerender(<BacklogPage />);

    expect(screen.getByTestId("backlog-table-row").className).not.toMatch(/tableRowJustChanged/);
  });
});
