import { render, screen } from "@testing-library/react";
import BacklogFlowPage from "../page";

jest.mock("@/lib/nav", () => jest.requireActual("@/lib/nav/impl.memory"));
jest.mock("@/lib/analytics/usePageView", () => ({ usePageView: () => {} }));
jest.mock("../FlowPage", () => ({
  FlowPage: () => {
    throw new Error("render failure");
  },
}));

describe("BacklogFlowPage", () => {
  it("BacklogFlowPage_should_OfferBackToBacklogLink_When_PageThrows", () => {
    jest.spyOn(console, "error").mockImplementation(() => {});
    render(<BacklogFlowPage />);

    expect(screen.getByRole("link", { name: "Back to Backlog" })).toHaveAttribute("href", "/backlog");
  });
});
