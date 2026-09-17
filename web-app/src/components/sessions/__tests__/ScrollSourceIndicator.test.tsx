/**
 * Tests for ScrollSourceIndicator (Story 1.4.2, Task 1.4.2a/c).
 */
import { render, screen } from "@testing-library/react";
import { ScrollSourceIndicator } from "../ScrollSourceIndicator";

describe("ScrollSourceIndicator", () => {
  it("renders 'Viewing <Program>'s own history' with role=status/aria-live=polite when visible", () => {
    render(<ScrollSourceIndicator program="Claude Code" visible />);
    const banner = screen.getByTestId("scroll-source-indicator");
    expect(banner).toHaveAttribute("role", "status");
    expect(banner).toHaveAttribute("aria-live", "polite");
    expect(banner).toHaveTextContent("Viewing Claude Code's own history");
  });

  it("marks the decorative icon aria-hidden", () => {
    render(<ScrollSourceIndicator program="Claude Code" visible />);
    const banner = screen.getByTestId("scroll-source-indicator");
    const icon = banner.querySelector('[aria-hidden="true"]');
    expect(icon).not.toBeNull();
  });

  it("unmounts (renders nothing) when visible is false", () => {
    render(<ScrollSourceIndicator program="Claude Code" visible={false} />);
    expect(screen.queryByTestId("scroll-source-indicator")).not.toBeInTheDocument();
  });

  it("uses the given program name for a non-Claude adapter", () => {
    render(<ScrollSourceIndicator program="pi" visible />);
    expect(screen.getByTestId("scroll-source-indicator")).toHaveTextContent("Viewing pi's own history");
  });
});
