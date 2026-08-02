import React from "react";
import { render, act } from "@testing-library/react";
import { InputDropBadge } from "./InputDropBadge";
import type { DroppedInputEvent } from "@/lib/hooks/useTerminalStream";

function makeEvent(count: number, at: number): DroppedInputEvent {
  return { count, at };
}

// The pill's visible text and the srOnly LiveRegion's text are identical
// content living in two separate elements — scope queries to the visible
// pill (the element carrying `title`) to avoid ambiguous-match errors, and
// to the `[role="alert"]` element for announcement-content assertions.
function pillEl(container: HTMLElement): HTMLElement | null {
  return container.querySelector("div[title]");
}

function alertEl(container: HTMLElement): HTMLElement | null {
  return container.querySelector('[role="alert"]');
}

function alertText(container: HTMLElement): string {
  const el = alertEl(container);
  expect(el).not.toBeNull();
  return el?.textContent ?? "";
}

describe("InputDropBadge", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    act(() => {
      jest.runOnlyPendingTimers();
    });
    jest.useRealTimers();
  });

  // Task 4.2.3.1 — basic render contract, matching GitHubBadge's `return
  // null` idiom (AC-VIS-5 / Surface D).
  it("InputDropBadge_should_renderNull_When_droppedInputEventIsNull", () => {
    const { container } = render(<InputDropBadge droppedInputEvent={null} />);
    expect(container.firstChild).toBeNull();
  });

  it("InputDropBadge_should_renderPillWithDropCount_When_droppedInputEventIsSet", () => {
    const { container } = render(<InputDropBadge droppedInputEvent={makeEvent(1, 1000)} />);
    expect(pillEl(container)?.textContent).toMatch(/1 input event not sent/i);
  });

  // AC-VIS-2 — no color-only signal: an aria-hidden icon AND visible text
  // both present.
  it("InputDropBadge_should_pairAriaHiddenIconWithVisibleText_When_droppedInputEventIsSet", () => {
    const { container } = render(<InputDropBadge droppedInputEvent={makeEvent(1, 1000)} />);
    const icon = container.querySelector('svg[aria-hidden="true"]');
    expect(icon).not.toBeNull();
    const pill = pillEl(container);
    expect(pill).not.toBeNull();
    expect(pill?.textContent).toMatch(/1 input event not sent/i);
    expect(pill).toBeVisible();
  });

  // AC-KBD-1/AC-KBD-2 — no focus theft, not focusable.
  it("does not set focus/tabIndex on mount", () => {
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();
    expect(document.activeElement).toBe(input);

    const { container } = render(<InputDropBadge droppedInputEvent={makeEvent(1, 1000)} />);

    expect(document.activeElement).toBe(input);
    const pill = pillEl(container);
    expect(pill).not.toBeNull();
    expect(pill).not.toHaveAttribute("tabIndex");
    document.body.removeChild(input);
  });

  it("InputDropBadge_should_notMoveFocus_When_BadgeAppearsWhileTerminalFocused", () => {
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();
    const before = document.activeElement;

    render(<InputDropBadge droppedInputEvent={makeEvent(1, 1000)} />);

    expect(document.activeElement).toBe(before);
    document.body.removeChild(input);
  });

  it("InputDropBadge_should_haveNoFocusableTabIndex_When_Rendered", () => {
    const { container } = render(<InputDropBadge droppedInputEvent={makeEvent(1, 1000)} />);
    const pill = pillEl(container);
    expect(pill?.tagName).not.toBe("BUTTON");
    expect(pill?.tagName).not.toBe("A");
    expect(pill).not.toHaveAttribute("tabIndex");
  });

  // AC-SR-1 — assertive, role=alert, aria-atomic, all on the same element.
  it("InputDropBadge_should_renderRoleAlertWithAssertiveAtomicLiveRegion_When_droppedInputEventIsSet", () => {
    const { container } = render(<InputDropBadge droppedInputEvent={makeEvent(1, 1000)} />);
    const region = alertEl(container);
    expect(region).not.toBeNull();
    expect(region).toHaveAttribute("aria-live", "assertive");
    expect(region).toHaveAttribute("aria-atomic", "true");
  });

  // AC-VIS-3 — auto-dismiss within ~4s with no further drops (Surface B).
  it("InputDropBadge_should_autoDismissWithinFourSeconds_When_NoFurtherDropsOccur", () => {
    const { container } = render(<InputDropBadge droppedInputEvent={makeEvent(1, 1000)} />);
    expect(container.firstChild).not.toBeNull();

    act(() => {
      jest.advanceTimersByTime(4001);
    });

    expect(container.firstChild).toBeNull();
  });

  // AC-VIS-4 — no stacking: exactly one instance visible across 3 rapid drops.
  it("InputDropBadge_should_renderExactlyOneInstance_When_ThreeDropsOccurInQuickSuccession", () => {
    const t0 = 1000;
    const { container, rerender } = render(<InputDropBadge droppedInputEvent={makeEvent(1, t0)} />);
    expect(container.querySelectorAll('[role="alert"]').length).toBe(1);

    act(() => { jest.advanceTimersByTime(800); });
    rerender(<InputDropBadge droppedInputEvent={makeEvent(2, t0 + 800)} />);
    expect(container.querySelectorAll('[role="alert"]').length).toBe(1);

    act(() => { jest.advanceTimersByTime(800); });
    rerender(<InputDropBadge droppedInputEvent={makeEvent(1, t0 + 1600)} />);
    expect(container.querySelectorAll('[role="alert"]').length).toBe(1);
  });

  // Task 4.2.3.2 / AC-SR-2 — coalesced running-total wording, accumulated
  // across successive drops within the same episode.
  it("InputDropBadge_should_announceRunningTotal_When_MultipleDropsCoalesceInSameEpisode", () => {
    const t0 = 2000;
    const { container, rerender } = render(<InputDropBadge droppedInputEvent={makeEvent(1, t0)} />);
    expect(pillEl(container)?.textContent).toMatch(/1 input event not sent/i);

    rerender(<InputDropBadge droppedInputEvent={makeEvent(2, t0 + 800)} />);
    expect(pillEl(container)?.textContent).toMatch(/3 input events not sent/i);

    rerender(<InputDropBadge droppedInputEvent={makeEvent(1, t0 + 1600)} />);
    expect(pillEl(container)?.textContent).toMatch(/4 input events not sent/i);
  });

  it("InputDropBadge_should_announceAccumulatedRunningTotal_When_DropsCoalesceInSameEpisode", () => {
    const t0 = 3000;
    const { container, rerender } = render(<InputDropBadge droppedInputEvent={makeEvent(1, t0)} />);
    expect(alertEl(container)?.textContent).toMatch(/1 input event not sent/i);

    rerender(<InputDropBadge droppedInputEvent={makeEvent(2, t0 + 800)} />);
    expect(alertEl(container)?.textContent).toMatch(/3 input events not sent/i);
    expect(alertEl(container)?.textContent).not.toMatch(/^2 input events/i);

    rerender(<InputDropBadge droppedInputEvent={makeEvent(1, t0 + 1600)} />);
    expect(alertEl(container)?.textContent).toMatch(/4 input events not sent/i);
  });

  // AC-SR-3 — announcement count bounded exactly by content changes: N rapid
  // updates within the dwell window fire exactly N announcements (not fewer,
  // not more). We observe this via the LiveRegion's rendered text changing
  // exactly N times, since InputDropBadge calls announce() exactly once per
  // distinct `at` while visible.
  it("InputDropBadge_should_fireOneAnnouncementPerContentChange_When_NRapidDropsOccurWithinDwellWindow", () => {
    const t0 = 4000;
    const seenMessages = new Set<string>();
    const { container, rerender } = render(<InputDropBadge droppedInputEvent={makeEvent(1, t0)} />);
    seenMessages.add(alertText(container));

    const N = 5;
    for (let i = 1; i < N; i++) {
      act(() => { jest.advanceTimersByTime(200); });
      rerender(<InputDropBadge droppedInputEvent={makeEvent(1, t0 + i * 200)} />);
      seenMessages.add(alertText(container));
    }

    // Each of the N distinct occurrences produced a distinct running total,
    // hence a distinct announced message — exactly N, never fewer or more.
    expect(seenMessages.size).toBe(N);
  });

  // AC-SR-4 — no duplicate "all clear": droppedInputEvent -> null must not
  // change the pill/announcement (InputDropBadge never announces reconnect
  // success itself; that's ConnectionIndicator's job, out of scope here).
  it("InputDropBadge_should_notAnnounce_When_droppedInputEventTransitionsBackToNull", () => {
    const t0 = 5000;
    const { container, rerender } = render(<InputDropBadge droppedInputEvent={makeEvent(1, t0)} />);
    const before = alertText(container);

    rerender(<InputDropBadge droppedInputEvent={null} />);

    // The badge/announcement is still showing (its own dwell timer, not the
    // prop, governs dismissal) and its content is unchanged by the
    // transition to null.
    expect(alertEl(container)?.textContent).toBe(before);
  });

  // AC-RESOLVE-2 — unmount safety: pending dwell timer cleared in cleanup.
  it("InputDropBadge_should_clearPendingTimer_When_UnmountedBeforeDwellTimerFires", () => {
    const errorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    const clearSpy = jest.spyOn(global, "clearTimeout");

    const { unmount } = render(<InputDropBadge droppedInputEvent={makeEvent(1, 6000)} />);

    unmount();
    clearSpy.mockClear();

    act(() => {
      jest.advanceTimersByTime(5000);
    });

    // No React "state update on an unmounted component" warning fired.
    const stateUpdateWarnings = errorSpy.mock.calls.filter((args) =>
      String(args[0]).includes("unmounted component")
    );
    expect(stateUpdateWarnings).toHaveLength(0);

    errorSpy.mockRestore();
    clearSpy.mockRestore();
  });

  // AC-RESOLVE-3 — fresh mount starts clean; no state leakage across remounts.
  it("InputDropBadge_should_resetRunningTotal_When_RemountedForSameSession", () => {
    const t0 = 7000;
    const { unmount } = render(<InputDropBadge droppedInputEvent={makeEvent(1, t0)} />);
    unmount();

    const { container } = render(<InputDropBadge droppedInputEvent={makeEvent(1, t0 + 5000)} />);
    expect(pillEl(container)?.textContent).toMatch(/1 input event not sent/i);
    expect(pillEl(container)?.textContent).not.toMatch(/4 input events/i);
  });

  // AC-KBD-3 — no focus theft or scroll on update/auto-dismiss.
  it("InputDropBadge_should_notMoveFocusOrScroll_When_CountUpdatesOrAutoDismisses", () => {
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();
    const scrollSpy = jest.fn();
    window.scrollTo = scrollSpy as unknown as typeof window.scrollTo;

    const t0 = 8000;
    const { rerender } = render(<InputDropBadge droppedInputEvent={makeEvent(1, t0)} />);
    expect(document.activeElement).toBe(input);

    rerender(<InputDropBadge droppedInputEvent={makeEvent(2, t0 + 800)} />);
    expect(document.activeElement).toBe(input);

    act(() => { jest.advanceTimersByTime(4001); });
    expect(document.activeElement).toBe(input);
    expect(scrollSpy).not.toHaveBeenCalled();

    document.body.removeChild(input);
  });
});
