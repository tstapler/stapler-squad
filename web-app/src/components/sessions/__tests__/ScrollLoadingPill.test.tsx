/**
 * Tests for ScrollLoadingPill (Story 1.4.5, Task 1.4.5a).
 */
import { render, screen, fireEvent } from "@testing-library/react";
import { ScrollLoadingPill } from "../ScrollLoadingPill";

describe("ScrollLoadingPill", () => {
  it("renders nothing when not visible", () => {
    render(<ScrollLoadingPill visible={false} stalled={false} onCancel={jest.fn()} />);
    expect(screen.queryByTestId("scroll-loading-pill")).not.toBeInTheDocument();
  });

  it("renders role=status + aria-live=polite when visible", () => {
    render(<ScrollLoadingPill visible stalled={false} onCancel={jest.fn()} />);
    const pill = screen.getByTestId("scroll-loading-pill");
    expect(pill).toHaveAttribute("role", "status");
    expect(pill).toHaveAttribute("aria-live", "polite");
  });

  it("shows generic 'Loading more…' copy with no program name", () => {
    render(<ScrollLoadingPill visible stalled={false} onCancel={jest.fn()} />);
    expect(screen.getByTestId("scroll-loading-pill")).toHaveTextContent("Loading more…");
  });

  it("shows program-specific copy when program is known", () => {
    render(<ScrollLoadingPill visible stalled={false} program="Claude Code" onCancel={jest.fn()} />);
    expect(screen.getByTestId("scroll-loading-pill")).toHaveTextContent("Loading Claude Code's history…");
  });

  it("does not show a Cancel button before stalled", () => {
    render(<ScrollLoadingPill visible stalled={false} onCancel={jest.fn()} />);
    expect(screen.queryByTestId("scroll-loading-pill-cancel")).not.toBeInTheDocument();
  });

  it("shows stalled copy and a keyboard-focusable Cancel button once stalled", () => {
    render(<ScrollLoadingPill visible stalled program="Claude Code" onCancel={jest.fn()} />);
    expect(screen.getByTestId("scroll-loading-pill")).toHaveTextContent("Still trying to load Claude Code's history…");
    const cancel = screen.getByTestId("scroll-loading-pill-cancel");
    expect(cancel.tagName).toBe("BUTTON");
  });

  it("calls onCancel when the Cancel button is activated", () => {
    const onCancel = jest.fn();
    render(<ScrollLoadingPill visible stalled onCancel={onCancel} />);
    fireEvent.click(screen.getByTestId("scroll-loading-pill-cancel"));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
