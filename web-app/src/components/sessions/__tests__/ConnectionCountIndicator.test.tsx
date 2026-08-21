/**
 * Tests for ConnectionCountIndicator (Epic 4.2, Story 4.2.2, Task 4.2.2d).
 *
 * Covers:
 *  - does not render when count <= 1 or undefined (plan.md Story 4.2.1 AC2 /
 *    Story 4.2.2 AC1, design/ux.md UX-AC-1)
 *  - role="status" + aria-live="polite" (never role="alert") when visible
 *    (Story 4.2.2 AC1/AC2, UX-AC-4)
 *  - icon is aria-hidden, visible/announced text carries the count
 *    (UX-AC-6)
 *  - changes-only: mounting already at count > 1 does not itself require a
 *    fresh DOM mutation once settled (Story 4.2.2 AC1)
 *  - rapid oscillation is coalesced into a single settled value
 *    (design/ux.md's "Rapid count oscillation" edge case, UX-AC-11)
 *  - tooltip revealed on hover and on keyboard focus (UX-AC-3, UX-AC-8),
 *    and only carries the resize-mismatch sentence when sizeMismatch is true
 *    (UX-AC-10 — never speculative)
 */
import { render, screen, act, fireEvent } from "@testing-library/react";
import { ConnectionCountIndicator } from "../ConnectionCountIndicator";

describe("ConnectionCountIndicator", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("does not render when count is undefined", () => {
    render(<ConnectionCountIndicator count={undefined} />);
    expect(screen.queryByTestId("connection-count-indicator")).not.toBeInTheDocument();
  });

  it("does not render when count is 1", () => {
    render(<ConnectionCountIndicator count={1} />);
    expect(screen.queryByTestId("connection-count-indicator")).not.toBeInTheDocument();
  });

  it("does not render when count is 0", () => {
    render(<ConnectionCountIndicator count={0} />);
    expect(screen.queryByTestId("connection-count-indicator")).not.toBeInTheDocument();
  });

  it("renders with role=\"status\" and aria-live=\"polite\" when count > 1", () => {
    render(<ConnectionCountIndicator count={2} />);
    const indicator = screen.getByTestId("connection-count-indicator");
    expect(indicator).toHaveAttribute("role", "status");
    expect(indicator).toHaveAttribute("aria-live", "polite");
  });

  it("never uses role=\"alert\"", () => {
    render(<ConnectionCountIndicator count={2} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("aria-label carries the count in plain language", () => {
    render(<ConnectionCountIndicator count={3} />);
    expect(screen.getByTestId("connection-count-indicator")).toHaveAttribute(
      "aria-label",
      "3 connections active"
    );
  });

  it("uses singular wording for exactly one other connection beyond the viewer (count === 2 still reads plural 'connections')", () => {
    // count itself is the total subscriber count, so 2 is "2 connections active" (plural).
    render(<ConnectionCountIndicator count={2} />);
    expect(screen.getByTestId("connection-count-indicator")).toHaveAttribute(
      "aria-label",
      "2 connections active"
    );
  });

  it("icon glyph is aria-hidden; visible text carries the meaning", () => {
    render(<ConnectionCountIndicator count={2} />);
    const indicator = screen.getByTestId("connection-count-indicator");
    const icon = indicator.querySelector("[aria-hidden='true']");
    expect(icon).not.toBeNull();
    expect(indicator).toHaveTextContent("2");
  });

  it("is keyboard-reachable (tabIndex=0)", () => {
    render(<ConnectionCountIndicator count={2} />);
    expect(screen.getByTestId("connection-count-indicator")).toHaveAttribute("tabIndex", "0");
  });

  it("reveals the tooltip on hover", () => {
    render(<ConnectionCountIndicator count={2} />);
    const indicator = screen.getByTestId("connection-count-indicator");
    expect(screen.queryByTestId("connection-count-tooltip")).not.toBeInTheDocument();

    fireEvent.mouseEnter(indicator);
    expect(screen.getByTestId("connection-count-tooltip")).toBeInTheDocument();

    fireEvent.mouseLeave(indicator);
    expect(screen.queryByTestId("connection-count-tooltip")).not.toBeInTheDocument();
  });

  it("reveals the tooltip on keyboard focus, not hover-only", () => {
    render(<ConnectionCountIndicator count={2} />);
    const indicator = screen.getByTestId("connection-count-indicator");

    fireEvent.focus(indicator);
    expect(screen.getByTestId("connection-count-tooltip")).toBeInTheDocument();

    fireEvent.blur(indicator);
    expect(screen.queryByTestId("connection-count-tooltip")).not.toBeInTheDocument();
  });

  it("tooltip shows only the plain count when there is no resize mismatch", () => {
    render(<ConnectionCountIndicator count={2} sizeMismatch={false} />);
    fireEvent.mouseEnter(screen.getByTestId("connection-count-indicator"));
    expect(screen.getByTestId("connection-count-tooltip")).toHaveTextContent("2 connections active");
    expect(screen.getByTestId("connection-count-tooltip")).not.toHaveTextContent(/different size/);
  });

  it("tooltip includes the resize-mismatch sentence only when sizeMismatch is true", () => {
    render(<ConnectionCountIndicator count={2} sizeMismatch={true} />);
    fireEvent.mouseEnter(screen.getByTestId("connection-count-indicator"));
    expect(screen.getByTestId("connection-count-tooltip")).toHaveTextContent(
      "Another connection has this session open at a different size."
    );
  });

  it("never mentions raw internals like hub, transport, or subscriber", () => {
    render(<ConnectionCountIndicator count={2} sizeMismatch={true} />);
    fireEvent.mouseEnter(screen.getByTestId("connection-count-indicator"));
    const text = screen.getByTestId("connection-count-indicator").textContent ?? "";
    expect(text).not.toMatch(/hub|transport|subscriber/i);
  });

  it("settles immediately on first render — no coalescing delay for the initial paint", () => {
    // Mounting already at count=2 (e.g. page reload while a second tab is
    // attached) must render synchronously; there is no artificial "changes-
    // only" grace period on mount itself.
    render(<ConnectionCountIndicator count={2} />);
    expect(screen.getByTestId("connection-count-indicator")).toBeInTheDocument();
  });

  it("coalesces a rapid burst of count changes into a single settled value", () => {
    const { rerender, queryByTestId } = render(<ConnectionCountIndicator count={2} />);
    expect(queryByTestId("connection-count-indicator")).toBeInTheDocument();

    // Flap 2 -> 1 -> 2 -> 3 within the coalesce window; only the final value
    // (3) should ever actually settle and be reflected in the DOM.
    rerender(<ConnectionCountIndicator count={1} />);
    act(() => {
      jest.advanceTimersByTime(100);
    });
    rerender(<ConnectionCountIndicator count={2} />);
    act(() => {
      jest.advanceTimersByTime(100);
    });
    rerender(<ConnectionCountIndicator count={3} />);

    // Still showing the pre-flap value — nothing has settled yet.
    expect(queryByTestId("connection-count-indicator")).toHaveAttribute("aria-label", "2 connections active");

    act(() => {
      jest.advanceTimersByTime(600);
    });

    expect(queryByTestId("connection-count-indicator")).toHaveAttribute("aria-label", "3 connections active");
  });

  it("unmounts once a settled flap resolves back down to 1", () => {
    const { rerender, queryByTestId } = render(<ConnectionCountIndicator count={2} />);
    expect(queryByTestId("connection-count-indicator")).toBeInTheDocument();

    rerender(<ConnectionCountIndicator count={1} />);
    act(() => {
      jest.advanceTimersByTime(600);
    });

    expect(queryByTestId("connection-count-indicator")).not.toBeInTheDocument();
  });
});
