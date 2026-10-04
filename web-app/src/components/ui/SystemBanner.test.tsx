// @feature ui:system-banner
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SystemBanner, SystemBannerStack, type SystemBannerProps } from "./SystemBanner";

jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

describe("SystemBanner", () => {
  it("calls onDismiss when the dismiss control is clicked", () => {
    const onDismiss = jest.fn();
    render(<SystemBanner id="x" severity="warning" message="hi" onDismiss={onDismiss} />);
    fireEvent.click(screen.getByLabelText("Dismiss"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("renders no pager controls for a single item", () => {
    render(<SystemBanner id="x" severity="warning" message="hi" pager={{ index: 0, total: 1, onPrev: jest.fn(), onNext: jest.fn() }} />);
    expect(screen.queryByLabelText("Next notification")).not.toBeInTheDocument();
  });
});

describe("SystemBannerStack", () => {
  function items(): SystemBannerProps[] {
    return [
      { id: "a", severity: "warning", message: "message a", testId: "banner-a" },
      { id: "b", severity: "error", message: "message b", testId: "banner-b" },
    ];
  }

  it("renders nothing for an empty list", () => {
    const { container } = render(<SystemBannerStack items={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows one item at a time and cycles with next/prev, wrapping around", () => {
    render(<SystemBannerStack items={items()} />);

    expect(screen.getByTestId("banner-a")).toBeInTheDocument();
    expect(screen.queryByTestId("banner-b")).not.toBeInTheDocument();
    expect(screen.getByText("1 / 2")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Next notification"));
    expect(screen.getByTestId("banner-b")).toBeInTheDocument();
    expect(screen.queryByTestId("banner-a")).not.toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Next notification"));
    expect(screen.getByTestId("banner-a")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Previous notification"));
    expect(screen.getByTestId("banner-b")).toBeInTheDocument();
  });
});
