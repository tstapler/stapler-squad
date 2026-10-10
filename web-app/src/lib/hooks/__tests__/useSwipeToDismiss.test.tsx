import React, { useRef } from "react";
import { act, render, screen } from "@testing-library/react";
import { useSwipeToDismiss } from "@/lib/hooks/useSwipeToDismiss";

const onDismiss = jest.fn();

function Row({ disabled = false }: { disabled?: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  const view = useSwipeToDismiss(ref, { onDismiss, disabled });
  return (
    <div ref={ref} data-testid="row" data-offset={view.offset} data-revealed={String(view.revealed)}>
      <button>inner</button>
    </div>
  );
}

function touch(target: Element, type: string, x: number, y: number, t: number) {
  const event = new Event(type, { bubbles: true, cancelable: true });
  const point = { clientX: x, clientY: y };
  Object.defineProperty(event, "touches", { value: type === "touchend" ? [] : [point] });
  Object.defineProperty(event, "changedTouches", { value: [point] });
  Object.defineProperty(event, "timeStamp", { value: t });
  act(() => {
    target.dispatchEvent(event);
  });
}

beforeEach(() => {
  onDismiss.mockClear();
  jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    width: 360, height: 80, top: 0, left: 0, right: 360, bottom: 80, x: 0, y: 0, toJSON: () => ({}),
  } as DOMRect);
});
afterEach(() => jest.restoreAllMocks());

describe("useSwipeToDismiss", () => {
  it("swipe_hook_should_dismiss_once_at_90px_200ms_and_cancel_on_60px_vertical_drift", () => {
    render(<Row />);
    const row = screen.getByTestId("row");

    touch(row, "touchstart", 100, 100, 0);
    touch(row, "touchmove", 140, 102, 60);
    touch(row, "touchmove", 190, 104, 150);
    expect(row).toHaveAttribute("data-offset", "90");
    expect(row).toHaveAttribute("data-revealed", "true");
    touch(row, "touchend", 190, 104, 200);
    expect(onDismiss).toHaveBeenCalledTimes(1);

    onDismiss.mockClear();
    touch(row, "touchstart", 100, 100, 1_000);
    touch(row, "touchmove", 140, 130, 1_050);
    touch(row, "touchmove", 190, 160, 1_100);
    touch(row, "touchend", 190, 160, 1_150);
    expect(onDismiss).not.toHaveBeenCalled();
    expect(row).toHaveAttribute("data-offset", "0");
  });

  it("swipe_hook_should_ignore_a_touch_that_starts_within_30px_of_either_screen_edge", () => {
    render(<Row />);
    const row = screen.getByTestId("row");
    const edgeX = [10, window.innerWidth - 10];
    for (const x0 of edgeX) {
      touch(row, "touchstart", x0, 100, 0);
      touch(row, "touchmove", x0 + 60, 102, 60);
      touch(row, "touchmove", x0 + 120, 104, 120);
      touch(row, "touchend", x0 + 120, 104, 150);
      expect(row).toHaveAttribute("data-offset", "0");
    }
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("swipe_hook_should_not_reach_document_touchmove_spy_when_swipe_starts_on_row", () => {
    render(<Row />);
    const row = screen.getByTestId("row");
    const documentSpy = jest.fn();
    document.addEventListener("touchmove", documentSpy);

    touch(row, "touchstart", 100, 100, 0);
    touch(row.querySelector("button")!, "touchmove", 140, 101, 60); // starts on a child, bubbles to the row
    touch(row, "touchend", 140, 101, 100);

    expect(documentSpy).not.toHaveBeenCalled();
    document.removeEventListener("touchmove", documentSpy);
  });

  it("does not stop touches that never started on the row", () => {
    render(<Row />);
    const documentSpy = jest.fn();
    document.addEventListener("touchmove", documentSpy);
    touch(screen.getByTestId("row"), "touchmove", 10, 10, 5);
    expect(documentSpy).toHaveBeenCalledTimes(1);
    document.removeEventListener("touchmove", documentSpy);
  });

  it("ignores the gesture while disabled", () => {
    render(<Row disabled />);
    const row = screen.getByTestId("row");
    touch(row, "touchstart", 100, 100, 0);
    touch(row, "touchmove", 190, 101, 100);
    touch(row, "touchend", 190, 101, 150);
    expect(onDismiss).not.toHaveBeenCalled();
  });
});
