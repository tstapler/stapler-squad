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

import "./backlogPageBaseMocks";
import React from "react";
import { render, screen, act } from "@testing-library/react";
import BacklogPage from "../page";
import { itemFixture, mockUseWatchBacklogItems } from "./backlogPageTestFixtures";

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
