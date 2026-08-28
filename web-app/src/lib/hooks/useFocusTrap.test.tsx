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

// MutableSetHarness lets a test disable a specific button AFTER the trap is
// already active, to regression-test that getFocusable() is recomputed on
// every Tab press rather than cached once at activation (useFocusTrap.ts's
// doc comment on getFocusable) — none of the harnesses above mutate the
// focusable set post-activation, so they'd pass unchanged whether or not
// that fix were present.
function MutableSetHarness({
  isActive,
  firstDisabled,
  thirdDisabled,
}: {
  isActive: boolean;
  firstDisabled?: boolean;
  thirdDisabled?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  useFocusTrap(containerRef, isActive);

  return (
    <div ref={containerRef} data-testid="container">
      <button data-testid="first" disabled={firstDisabled}>
        First
      </button>
      <button data-testid="second">Second</button>
      <button data-testid="third" disabled={thirdDisabled}>
        Third
      </button>
    </div>
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

  it("useFocusTrap_should_ExcludeDisabledElement_When_ComputingForwardWrapBoundary", () => {
    const { getByTestId, rerender } = render(<MutableSetHarness isActive={true} />);
    expect(document.activeElement).toBe(getByTestId("first"));

    // Disable "third" — the trap's current last focusable element — AFTER
    // activation (e.g. a dialog's Send button disabling mid-flight). A
    // snapshot cached once at activation would still treat "third" as the
    // wrap target and never fire (document.activeElement can never equal a
    // reference the test moves focus away from), silently breaking the
    // forward wrap instead of skipping the disabled element.
    rerender(<MutableSetHarness isActive={true} thirdDisabled={true} />);

    // "second" is now the effective last focusable element.
    getByTestId("second").focus();
    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(getByTestId("first"));
  });

  it("useFocusTrap_should_ExcludeDisabledElement_When_ComputingBackwardWrapBoundary", () => {
    const { getByTestId, rerender } = render(<MutableSetHarness isActive={true} />);
    expect(document.activeElement).toBe(getByTestId("first"));

    // Disable "first" — the element the trap auto-focused on activation —
    // AFTER activation. "second" becomes the new first focusable element.
    rerender(<MutableSetHarness isActive={true} firstDisabled={true} />);

    getByTestId("second").focus();
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(getByTestId("third"));
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
