/**
 * Tests for InputDropBadge (Story 2.3, Task 2.3.4).
 *
 * Covers:
 *  - role="alert" / aria-live="assertive" on the live region
 *  - singular vs. plural badge text
 *  - auto-dismiss after DEFAULT_TOAST_MS
 *  - (e) two consecutive identical-count episodes each still produce a
 *    distinct underlying text-node mutation (adversarial-review.md's
 *    dedup concern)
 *  - (f) appearing does not move document.activeElement (design/ux.md §3.1)
 */
import { render, screen, act } from "@testing-library/react";
import { InputDropBadge } from "../InputDropBadge";
import { DEFAULT_TOAST_MS } from "@/lib/notification-policy";

describe("InputDropBadge", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("renders with role=\"alert\" and aria-live=\"assertive\"", () => {
    render(<InputDropBadge count={1} episodeSeq={1} />);
    const region = screen.getByRole("alert");
    expect(region).toHaveAttribute("aria-live", "assertive");
  });

  it("shows singular text for count === 1", () => {
    render(<InputDropBadge count={1} episodeSeq={1} />);
    expect(screen.getByTestId("input-drop-badge")).toHaveTextContent(
      "1 keystroke dropped — reconnecting"
    );
  });

  it("shows plural text with the exact count for count > 1", () => {
    render(<InputDropBadge count={3} episodeSeq={1} />);
    expect(screen.getByTestId("input-drop-badge")).toHaveTextContent(
      "3 keystrokes dropped — reconnecting"
    );
  });

  it("auto-dismisses the visual badge after DEFAULT_TOAST_MS", () => {
    render(<InputDropBadge count={1} episodeSeq={1} />);
    expect(screen.getByTestId("input-drop-badge")).toBeInTheDocument();

    act(() => {
      jest.advanceTimersByTime(DEFAULT_TOAST_MS + 100);
    });

    expect(screen.queryByTestId("input-drop-badge")).not.toBeInTheDocument();
  });

  it("does not require any timeout that exceeds ~8.5s to dismiss", () => {
    render(<InputDropBadge count={2} episodeSeq={1} />);

    act(() => {
      jest.advanceTimersByTime(DEFAULT_TOAST_MS - 500);
    });
    // Still visible just before the deadline.
    expect(screen.getByTestId("input-drop-badge")).toBeInTheDocument();

    act(() => {
      jest.advanceTimersByTime(1000);
    });
    expect(screen.queryByTestId("input-drop-badge")).not.toBeInTheDocument();
  });

  it("two consecutive episodes with an identical count each produce a distinct live-region text mutation", () => {
    const { rerender } = render(<InputDropBadge count={1} episodeSeq={1} />);
    const firstText = screen.getByRole("alert").textContent ?? "";

    rerender(<InputDropBadge count={1} episodeSeq={2} />);
    const secondText = screen.getByRole("alert").textContent ?? "";

    // The underlying text node must differ between the two announcements...
    expect(secondText).not.toBe(firstText);
    // ...even though the human-readable count is the same.
    expect(firstText.replace(/​/g, "")).toBe(secondText.replace(/​/g, ""));
  });

  it("does not move document.activeElement when it appears", () => {
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();
    expect(document.activeElement).toBe(input);

    render(<InputDropBadge count={1} episodeSeq={1} />);

    expect(document.activeElement).toBe(input);
    document.body.removeChild(input);
  });

  it("defensively no-ops on count <= 0 (no badge, no announcement)", () => {
    render(<InputDropBadge count={0} episodeSeq={1} />);
    expect(screen.queryByRole("alert")).toHaveTextContent("");
  });
});
