import { useRef } from "react";
import { render, fireEvent } from "@testing-library/react";
import { useFocusTrap } from "./useFocusTrap";

function TrapHarness({
  isActive,
  withTrigger,
  empty,
  hideTrigger,
}: {
  isActive: boolean;
  withTrigger?: boolean;
  empty?: boolean;
  hideTrigger?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  useFocusTrap(containerRef, isActive, withTrigger ? triggerRef : undefined);

  return (
    <>
      {!hideTrigger && (
        <button ref={triggerRef} data-testid="trigger">
          Open
        </button>
      )}
      <div ref={containerRef} data-testid="container">
        {!empty && (
          <>
            <button data-testid="first">First</button>
            <button data-testid="second">Second</button>
          </>
        )}
      </div>
    </>
  );
}

describe("useFocusTrap", () => {
  it("useFocusTrap_should_moveFocusToFirstFocusable_When_activated", () => {
    const { getByTestId } = render(<TrapHarness isActive={true} />);
    expect(document.activeElement).toBe(getByTestId("first"));
  });

  it("useFocusTrap_should_wrapForward_When_tabbingPastLastFocusable", () => {
    const { getByTestId } = render(<TrapHarness isActive={true} />);
    getByTestId("second").focus();
    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(getByTestId("first"));
  });

  it("useFocusTrap_should_wrapBackward_When_shiftTabbingBeforeFirstFocusable", () => {
    const { getByTestId } = render(<TrapHarness isActive={true} />);
    getByTestId("first").focus();
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(getByTestId("second"));
  });

  it("useFocusTrap_should_preventDefaultTab_When_noFocusableElementsExist", () => {
    render(<TrapHarness isActive={true} empty={true} />);
    const event = fireEvent.keyDown(document, { key: "Tab", cancelable: true });
    expect(event).toBe(false); // fireEvent returns false when preventDefault was called
  });

  it("useFocusTrap_should_restoreFocusToTrigger_When_deactivated", () => {
    const { getByTestId, rerender } = render(
      <TrapHarness isActive={true} withTrigger={true} />
    );
    rerender(<TrapHarness isActive={false} withTrigger={true} />);
    expect(document.activeElement).toBe(getByTestId("trigger"));
  });

  it("useFocusTrap_should_noop_When_triggerRefNotProvided", () => {
    const { rerender } = render(<TrapHarness isActive={true} withTrigger={false} />);
    expect(() => rerender(<TrapHarness isActive={false} withTrigger={false} />)).not.toThrow();
  });

  it("useFocusTrap_should_noop_When_triggerElementDetachedFromDom", () => {
    const { rerender } = render(<TrapHarness isActive={true} withTrigger={true} />);
    // Unmount the trigger through React (not a raw DOM .remove(), which would
    // fight React's own reconciler for ownership of the node) so the effect's
    // already-captured triggerEl reference becomes a detached node by the time
    // cleanup runs.
    rerender(<TrapHarness isActive={true} withTrigger={true} hideTrigger={true} />);
    expect(() =>
      rerender(<TrapHarness isActive={false} withTrigger={true} hideTrigger={true} />)
    ).not.toThrow();
  });
});
