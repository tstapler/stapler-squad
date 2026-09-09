import { render, screen } from "@testing-library/react";
import { JulesStatusBadge } from "./JulesStatusBadge";

describe("JulesStatusBadge", () => {
  it("renders a distinct icon, the visible text, and a matching aria-label for the running phase", () => {
    render(<JulesStatusBadge phase="running" />);

    const badge = screen.getByRole("img", { name: "Jules: Running" });
    expect(badge).toBeInTheDocument();
    expect(screen.getByTestId("jules-status-icon")).toBeInTheDocument();
    expect(screen.getByText("Jules: Running")).toBeInTheDocument();
  });

  it("returns null when phase is undefined", () => {
    const { container } = render(<JulesStatusBadge phase={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("keeps the running label and adds retrying text when pollHealthy is false, without switching to failed", () => {
    const eightMinutesAgo = new Date(Date.now() - 8 * 60 * 1000);
    render(<JulesStatusBadge phase="running" lastPolledAt={eightMinutesAgo} pollHealthy={false} />);

    expect(screen.getByRole("img", { name: "Jules: Running" })).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Jules: Failed" })).not.toBeInTheDocument();
    expect(screen.getByText("Last updated 8m ago, retrying…")).toBeInTheDocument();
  });

  it("announces routine transitions through the polite region and a failed transition through a separate alert region", () => {
    const { rerender } = render(<JulesStatusBadge phase="queued" />);
    rerender(<JulesStatusBadge phase="running" />);

    const politeRegion = screen.getByRole("status");
    expect(politeRegion).toHaveAttribute("aria-live", "polite");
    expect(politeRegion).toHaveTextContent("Jules session running");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    rerender(<JulesStatusBadge phase="failed" />);

    const alertRegion = screen.getByRole("alert");
    expect(alertRegion).toHaveTextContent("Jules session failed");
  });

  it("renders the reconnect-required phase with an amber icon distinct from Running and Failed, plus an Update key link, via the polite (not alert) region", () => {
    render(<JulesStatusBadge phase="reconnect-required" />);

    expect(screen.getByRole("img", { name: "Jules: Reconnect required" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite");

    const link = screen.getByRole("link", { name: "Update key" });
    expect(link).toHaveAttribute("href", "/settings/jules");
  });
});
