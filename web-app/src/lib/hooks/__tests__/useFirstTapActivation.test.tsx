import React, { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { useFirstTapActivation } from "../useFirstTapActivation";

// jsdom has no PointerEvent; a MouseEvent subclass carries clientX/Y and pointerId.
class TestPointerEvent extends MouseEvent {
  pointerId: number;
  constructor(type: string, init: MouseEventInit & { pointerId?: number } = {}) {
    super(type, init);
    this.pointerId = init.pointerId ?? 1;
  }
}
beforeAll(() => { (window as unknown as { PointerEvent: unknown }).PointerEvent = TestPointerEvent; });

function Harness({ onActivate }: { onActivate: (key: string) => void }) {
  const tap = useFirstTapActivation(onActivate);
  const [reversed, setReversed] = useState(false);
  const keys = reversed ? ["row3", "row2", "row1"] : ["row1", "row2", "row3"];
  return (
    <div>
      <input aria-label="query" />
      <button onClick={() => setReversed(true)}>relayout</button>
      <div data-testid="list" {...tap}>
        {keys.map((k) => <div key={k} data-row-key={k} data-testid={k}>{k}</div>)}
      </div>
    </div>
  );
}

const down = (el: Element, x = 10, y = 10) => fireEvent.pointerDown(el, { clientX: x, clientY: y, pointerId: 1 });
const up = (el: Element, x = 10, y = 10) => fireEvent.pointerUp(el, { clientX: x, clientY: y, pointerId: 1 });

describe("useFirstTapActivation", () => {
  it("firstTap_should_ActivateCapturedRowOnce_And_IgnoreScroll_When_KeyboardOpen", () => {
    const onActivate = jest.fn();
    render(<Harness onActivate={onActivate} />);
    const input = screen.getByLabelText("query");
    input.focus();

    // One tap with the keyboard up: blur on down, activate on up, and the synthesized click adds nothing.
    down(screen.getByTestId("row3"));
    expect(document.activeElement).not.toBe(input);
    up(screen.getByTestId("row3"));
    fireEvent.click(screen.getByTestId("row3"));
    expect(onActivate).toHaveBeenCalledTimes(1);
    expect(onActivate).toHaveBeenCalledWith("row3");

    // More than 10 px between down and up is a scroll: nothing opens, but the keyboard still dismisses.
    input.focus();
    onActivate.mockClear();
    down(screen.getByTestId("row2"), 10, 10);
    fireEvent.pointerMove(screen.getByTestId("row2"), { clientX: 10, clientY: 26, pointerId: 1 });
    up(screen.getByTestId("row2"), 10, 26);
    expect(onActivate).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(input);
  });

  it("firstTap_should_NotReHitTestAfterRelayout_When_ViewportMovesBetweenDownAndUp", () => {
    const onActivate = jest.fn();
    render(<Harness onActivate={onActivate} />);
    down(screen.getByTestId("row1"));
    fireEvent.click(screen.getByText("relayout"));
    // After the relayout the pointer lands on what is now a different row; the captured one still wins.
    up(screen.getByTestId("row3"));
    expect(onActivate).toHaveBeenCalledTimes(1);
    expect(onActivate).toHaveBeenCalledWith("row1");
  });

  it("firstTap_should_ActivateOnPlainClick_When_NoPointerSequence (assistive technology)", () => {
    const onActivate = jest.fn();
    render(<Harness onActivate={onActivate} />);
    fireEvent.click(screen.getByTestId("row2"));
    expect(onActivate).toHaveBeenCalledWith("row2");
  });

  it("firstTap_should_IgnoreAPointerThatCancels_When_SystemTakesOverTheGesture", () => {
    const onActivate = jest.fn();
    render(<Harness onActivate={onActivate} />);
    down(screen.getByTestId("row1"));
    fireEvent.pointerCancel(screen.getByTestId("row1"), { pointerId: 1 });
    up(screen.getByTestId("row1"));
    expect(onActivate).not.toHaveBeenCalled();
  });
});
