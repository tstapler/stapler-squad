import React, { useRef, useState } from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { AlertConfirmDialog } from "./AlertConfirmDialog";

beforeAll(() => {
  // Benign vanilla-extract jest className warning (see features/page.test.tsx).
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => jest.restoreAllMocks());

type OpenerKind = "keyboard" | "click" | "non-focusing";

/** Opener plus dialog; "non-focusing" passes returnFocusRef and never takes focus, like Safari's click. */
function Harness({
  kind,
  onConfirm = () => {},
  confirmDisabled = false,
}: {
  kind: OpenerKind;
  onConfirm?: () => void;
  confirmDisabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const switchRef = useRef<HTMLButtonElement>(null);
  return (
    <div>
      <button ref={switchRef} data-testid="opener" onClick={() => setOpen(true)}>
        Opener
      </button>
      {open && (
        <AlertConfirmDialog
          heading="Enforce operators?"
          confirmLabel="Enforce"
          onConfirm={() => {
            onConfirm();
            setOpen(false);
          }}
          onCancel={() => setOpen(false)}
          confirmDisabled={confirmDisabled}
          confirmDescribedById="hint"
          returnFocusRef={kind === "non-focusing" ? switchRef : undefined}
        >
          <p>Body text</p>
          <span id="hint">Tick the acknowledgement to enable Enforce.</span>
        </AlertConfirmDialog>
      )}
    </div>
  );
}

function openWith(kind: OpenerKind) {
  const opener = screen.getByTestId("opener");
  if (kind === "non-focusing") {
    // Focus stays on body, as when Safari does not focus a clicked button.
    (document.activeElement as HTMLElement | null)?.blur();
  } else {
    opener.focus();
  }
  fireEvent.click(opener);
  return opener;
}

const closers: Array<[string, () => void]> = [
  ["Cancel", () => fireEvent.click(screen.getByTestId("alert-confirm-cancel"))],
  ["Escape", () => fireEvent.keyDown(document, { key: "Escape" })],
  ["overlay click", () => fireEvent.click(screen.getByTestId("alert-confirm-overlay"))],
  ["Confirm", () => fireEvent.click(screen.getByTestId("alert-confirm-confirm"))],
];

describe("AlertConfirmDialog", () => {
  it("focuses Cancel first, traps Tab, cancels on Escape and on overlay click (not on an inside click), returns focus after Cancel, Escape, overlay click and Confirm for keyboard, click and non-focusing openers via returnFocusRef, and honors confirmDisabled with confirmDescribedById", () => {
    // role and aria wiring, initial focus, Tab trap, DOM order
    const first = render(<Harness kind="keyboard" />);
    openWith("keyboard");
    const dialog = screen.getByRole("alertdialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveAccessibleName("Enforce operators?");
    expect(dialog).toHaveAccessibleDescription(/Body text/);
    const cancel = screen.getByTestId("alert-confirm-cancel");
    const confirm = screen.getByTestId("alert-confirm-confirm");
    expect(cancel).toHaveFocus();
    expect(cancel.compareDocumentPosition(confirm) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    confirm.focus();
    fireEvent.keyDown(document, { key: "Tab" });
    expect(cancel).toHaveFocus();
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(confirm).toHaveFocus();

    // an inside click does not cancel
    fireEvent.click(screen.getByText("Body text"));
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    first.unmount();

    // focus returns to the opener after every closer, for every opener kind
    for (const kind of ["keyboard", "click", "non-focusing"] as OpenerKind[]) {
      for (const [, close] of closers) {
        const view = render(<Harness kind={kind} />);
        const opener = openWith(kind);
        expect(screen.getByRole("alertdialog")).toBeInTheDocument();
        act(() => close());
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        expect(opener).toHaveFocus();
        view.unmount();
      }
    }

    // confirmDisabled: aria-disabled, hint wiring, activation is a no-op
    const onConfirm = jest.fn();
    render(<Harness kind="keyboard" onConfirm={onConfirm} confirmDisabled />);
    openWith("keyboard");
    const disabled = screen.getByTestId("alert-confirm-confirm");
    expect(disabled).toHaveAttribute("aria-disabled", "true");
    expect(disabled).toHaveAccessibleDescription("Tick the acknowledgement to enable Enforce.");
    fireEvent.click(disabled);
    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });
});
