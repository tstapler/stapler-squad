import { act, render } from "@testing-library/react";
import { PullRequestUndoToast, UNDO_TOAST_MS } from "./PullRequestUndoToast";

jest.mock("./PullRequestList.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));

describe("PullRequestUndoToast", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("toast_should_dismissAtOriginalDeadline_When_HostRerendersWithNewOnDismiss", () => {
    const first = jest.fn();
    const second = jest.fn();
    const props = { message: "m", hint: "h", onUndo: jest.fn(), focusUndoOnAppear: false };
    const { rerender } = render(<PullRequestUndoToast {...props} onDismiss={first} />);

    act(() => void jest.advanceTimersByTime(UNDO_TOAST_MS - 1_000));
    rerender(<PullRequestUndoToast {...props} onDismiss={second} />);
    act(() => void jest.advanceTimersByTime(999));
    expect(second).not.toHaveBeenCalled();
    act(() => void jest.advanceTimersByTime(1));

    expect(second).toHaveBeenCalledTimes(1);
    expect(second).toHaveBeenCalledWith(false);
    expect(first).not.toHaveBeenCalled();
  });
});
