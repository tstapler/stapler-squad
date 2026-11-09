/**
 * The list (table) view must surface the same status widgets as the board
 * cards: verdict, duplicate, dependency, claim, and provenance chips, with
 * the stuck/claims data fetched once per page rather than per row.
 */

import "./backlogPageBaseMocks";
import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import BacklogPage from "../page";
import { itemFixture, mockUseWatchBacklogItems, mockListForeignClaims } from "./backlogPageTestFixtures";

describe("BacklogPage list row widgets", () => {
  afterEach(() => {
    jest.clearAllMocks();
    mockListForeignClaims.mockResolvedValue([]);
  });

  it("BacklogPage_should_ShowDuplicateBadgeAndDependencyChip_When_ItemHasThem", () => {
    mockUseWatchBacklogItems.mockReturnValue({
      items: [itemFixture({ duplicatePending: true, duplicateRef: "other-1", blockedBy: ["item-2"] })],
      connectionState: "live",
    });
    render(<BacklogPage />);
    const widgets = within(screen.getByTestId("backlog-row-widgets"));
    expect(widgets.getByTestId("duplicate-pending-badge")).toBeInTheDocument();
    expect(widgets.getByTestId("dependency-chip")).toBeInTheDocument();
  });

  it("BacklogPage_should_RenderEmptyWidgetsContainer_When_ItemHasNoSignals", () => {
    mockUseWatchBacklogItems.mockReturnValue({ items: [itemFixture()], connectionState: "live" });
    render(<BacklogPage />);
    expect(screen.getByTestId("backlog-row-widgets")).toBeEmptyDOMElement();
  });

  it("BacklogPage_should_ShowClaimChip_When_ForeignClaimMatchesItemUrl", async () => {
    mockListForeignClaims.mockResolvedValue([
      { externalUrl: "https://github.com/o/r/issues/1", claimingHostId: "host-b", itemDeepLink: "ssq://x", disputed: false },
    ]);
    mockUseWatchBacklogItems.mockReturnValue({
      items: [itemFixture({ externalUrl: "https://github.com/o/r/issues/1", externalId: "1" })],
      connectionState: "live",
    });
    render(<BacklogPage />);
    await waitFor(() => expect(screen.getByTestId("claim-chip")).toBeInTheDocument());
    expect(mockListForeignClaims).toHaveBeenCalledTimes(1);
  });
});
