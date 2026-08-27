import { useRef, RefObject } from "react";
import { render, fireEvent, act } from "@testing-library/react";
import { useFocusTrap } from "./useFocusTrap";

// Mirrors the real call sites (ReviewChangesModal, BacklogFileBrowserModal):
// the trigger is a button that lives OUTSIDE the trapped/unmounted subtree
// (portaled elsewhere in the DOM), not a sibling that unmounts along with it.
function TrapHarness({
  isActive,
  triggerRef,
  empty = false,
}: {
  isActive: boolean;
  triggerRef?: RefObject<HTMLElement | null>;
  empty?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  useFocusTrap(containerRef, isActive, triggerRef);

  return (
    <div ref={containerRef} data-testid="container">
      {!empty && (
        <>
          <button data-testid="first">First</button>
          <div aria-hidden="true">
            <button data-testid="hidden">Hidden</button>
          </div>
          <button data-testid="last">Last</button>
        </>
      )}
    </div>
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
  it("useFocusTrap_should_RestoreFocusToTrigger_When_UnmountedWithTriggerRefSupplied", () => {
    const trigger = document.createElement("button");
    document.body.appendChild(trigger);
    const triggerRef = { current: trigger as HTMLElement | null };

    const { unmount } = render(<TrapHarness isActive triggerRef={triggerRef} />);

    act(() => {
      unmount();
    });

    expect(document.activeElement).toBe(trigger);
    trigger.remove();
  });

  it("useFocusTrap_should_NotThrowAndLeaveFocusUnchanged_When_NoTriggerRefSupplied", () => {
    const { unmount, getByTestId } = render(<TrapHarness isActive />);
    const first = getByTestId("first");
    expect(document.activeElement).toBe(first);

    expect(() => {
      act(() => {
        unmount();
      });
    }).not.toThrow();

    // No trigger to restore to — jsdom (like real browsers) drops focus to
    // <body> once the focused node is removed. Asserting body specifically
    // (not just "!== first") catches a future regression that refocuses some
    // other stray element instead of leaving focus alone.
    expect(document.activeElement).toBe(document.body);
  });

  it("useFocusTrap_should_NotThrow_When_TriggerElementRemovedFromDomBeforeUnmount", () => {
    const trigger = document.createElement("button");
    document.body.appendChild(trigger);
    const triggerRef = { current: trigger as HTMLElement | null };

    const { unmount } = render(<TrapHarness isActive triggerRef={triggerRef} />);

    // Trigger is removed from the DOM while the trap is still active — e.g.
    // the row that opened the modal was filtered/deleted mid-review.
    trigger.remove();

    expect(() => {
      act(() => {
        unmount();
      });
    }).not.toThrow();

    // .focus() on a detached node is a documented DOM no-op — focus falls
    // back to <body>, not left dangling on some other stray element.
    expect(document.activeElement).toBe(document.body);
  });

  it("useFocusTrap_should_MoveFocusToFirstFocusable_When_Activated", () => {
    const { getByTestId } = render(<TrapHarness isActive />);
    expect(document.activeElement).toBe(getByTestId("first"));
  });

  it("useFocusTrap_should_WrapForward_When_TabPressedOnLastFocusable", () => {
    const { getByTestId } = render(<TrapHarness isActive />);
    const first = getByTestId("first");
    const last = getByTestId("last");

    last.focus();
    expect(document.activeElement).toBe(last);

    fireEvent.keyDown(document, { key: "Tab" });

    expect(document.activeElement).toBe(first);
  });

  it("useFocusTrap_should_WrapBackward_When_ShiftTabPressedOnFirstFocusable", () => {
    const { getByTestId } = render(<TrapHarness isActive />);
    const first = getByTestId("first");
    const last = getByTestId("last");

    expect(document.activeElement).toBe(first);

    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });

    expect(document.activeElement).toBe(last);
  });

  it("useFocusTrap_should_ExcludeAriaHiddenSubtree_When_CyclingFocusableElements", () => {
    const { getByTestId } = render(<TrapHarness isActive />);
    const last = getByTestId("last");

    last.focus();
    fireEvent.keyDown(document, { key: "Tab" });

    // Wraps straight to "first", never landing on the aria-hidden "hidden" button.
    expect(document.activeElement).toBe(getByTestId("first"));
    expect(document.activeElement).not.toBe(getByTestId("hidden"));
  });

  it("useFocusTrap_should_NotThrow_When_TabPressedWithZeroFocusableChildren", () => {
    render(<TrapHarness isActive empty />);

    expect(() => {
      fireEvent.keyDown(document, { key: "Tab" });
    }).not.toThrow();
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
});
