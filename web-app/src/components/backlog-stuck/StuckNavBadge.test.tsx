import React from "react";
import { render, screen } from "@testing-library/react";

const mockUseStuckBacklogItems = jest.fn();

jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => mockUseStuckBacklogItems(),
}));

import { StuckNavBadge } from "./StuckNavBadge";

describe("StuckNavBadge", () => {
  beforeEach(() => {
    mockUseStuckBacklogItems.mockReset();
  });

  describe("StuckNavBadge_should_showLoadingAffordance_When_NoFetchHasSucceededYet", () => {
    it("renders a neutral loading placeholder before the first successful fetch, never a bare 0", () => {
      mockUseStuckBacklogItems.mockReturnValue({ items: [], lastFetched: null });
      render(<StuckNavBadge />);
      expect(screen.getByTestId("stuck-nav-badge-loading")).toBeInTheDocument();
      expect(screen.queryByTestId("stuck-nav-badge")).not.toBeInTheDocument();
      expect(screen.queryByText("0")).not.toBeInTheDocument();
    });
  });

  describe("StuckNavBadge_should_haveFullPhraseAriaLabelAndHideAtZero_When_Rendered", () => {
    it("shows the count with a full-phrase aria-label once fetched", () => {
      mockUseStuckBacklogItems.mockReturnValue({
        items: [{}, {}, {}, {}, {}],
        lastFetched: new Date(),
      });
      render(<StuckNavBadge />);
      const badge = screen.getByTestId("stuck-nav-badge");
      expect(badge).toHaveTextContent("5");
      expect(badge).toHaveAttribute("aria-label", "5 items stuck");
    });

    it("uses singular phrasing for a count of 1", () => {
      mockUseStuckBacklogItems.mockReturnValue({ items: [{}], lastFetched: new Date() });
      render(<StuckNavBadge />);
      expect(screen.getByTestId("stuck-nav-badge")).toHaveAttribute(
        "aria-label",
        "1 item stuck"
      );
    });

    it("is not rendered at all when count is 0 after a successful fetch", () => {
      mockUseStuckBacklogItems.mockReturnValue({ items: [], lastFetched: new Date() });
      const { container } = render(<StuckNavBadge />);
      expect(container.firstChild).toBeNull();
    });
  });
});
