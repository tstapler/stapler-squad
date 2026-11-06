import React, { useReducer } from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { bufferReducer, initialBufferState, type BufferAction, type BufferState } from "@/lib/files/bufferReducer";
import { TabSwitcherSheet } from "../files/TabSwitcherSheet";

const load = (path: string, pinned: boolean): BufferAction[] => [
  { type: "open", path, sessionId: "s1", pinned },
  { type: "markLoaded", path, token: "v1:1:1:x", readOnly: "none" },
];
// a.go pinned, b.go preview (the next open may replace it), c.go dirty
const mixed = (): BufferState =>
  [...load("a.go", true), ...load("pkg/b.go", false), ...load("c.go", true), { type: "markDirty", path: "c.go", dirty: true } as BufferAction]
    .reduce(bufferReducer, initialBufferState(10));

function Sheet({ initial, refusal = null, spies }: { initial: BufferState; refusal?: string | null; spies?: { copy: jest.Mock } }) {
  const [state, dispatch] = useReducer(bufferReducer, initial);
  return (
    <TabSwitcherSheet
      buffers={state.buffers}
      activeIndex={state.activeIndex}
      refusal={refusal}
      onSelect={(path) => dispatch({ type: "activate", path })}
      onClose={(path) => dispatch({ type: "close", path })}
      onPin={(path) => dispatch({ type: "pin", path })}
      onUnpin={(path) => dispatch({ type: "unpin", path })}
      onCloseOthers={(keep) => state.buffers.filter((b) => b.path !== keep).forEach((b) => dispatch({ type: "close", path: b.path }))}
      onCopyPath={spies?.copy ?? jest.fn()}
      onDismiss={jest.fn()}
      onGoToFiles={jest.fn()}
    />
  );
}

const rowButton = (name: RegExp | string) => screen.getByRole("button", { name });

describe("TabSwitcherSheet", () => {
  it("tabSwitcherSheet_should_PinUnpinFromRowMenu_And_NameStateInAccessibleName_And_HighlightEligibleClosesWhenRefused", async () => {
    const user = userEvent.setup();
    const copy = jest.fn();
    const { rerender } = render(<Sheet initial={mixed()} spies={{ copy }} />);

    // State is in the accessible name as words, with a glyph for pinned and a dot for unsaved.
    expect(rowButton("a.go, pinned")).toBeInTheDocument();
    expect(rowButton("b.go, preview")).toBeInTheDocument();
    expect(rowButton("c.go, unsaved")).toBeInTheDocument();
    expect(within(screen.getByTestId("tab-sheet-row-a.go")).getByText("📌")).toBeInTheDocument();

    // Unpin from the row actions menu turns the pinned row into a preview row.
    await user.click(rowButton("Actions for a.go"));
    await user.click(screen.getByRole("menuitem", { name: "Unpin" }));
    expect(rowButton("a.go, preview")).toBeInTheDocument();

    // And Pin turns a preview row into a pinned one.
    await user.click(rowButton("Actions for b.go"));
    await user.click(screen.getByRole("menuitem", { name: "Pin" }));
    expect(rowButton("b.go, pinned")).toBeInTheDocument();

    // Copy path goes through the row menu too.
    await user.click(rowButton("Actions for c.go"));
    await user.click(screen.getByRole("menuitem", { name: "Copy path" }));
    expect(copy).toHaveBeenCalledWith("c.go");

    // Refusal: the message is pinned at the top and only the clean rows' Close is highlighted.
    rerender(<Sheet initial={mixed()} refusal="Too many tabs open - close one" />);
    expect(screen.getByRole("note")).toHaveTextContent("Too many tabs open - close one");
    expect(screen.getByRole("button", { name: "Close a.go, frees a slot" })).toHaveAttribute("data-highlight", "true");
    expect(screen.getByRole("button", { name: "Close c.go" })).not.toHaveAttribute("data-highlight");
  });

  it("tabSwitcherSheet_should_OpenTheSameMenuOnLongPress_And_SuppressTheTapThatEndsIt", () => {
    jest.useFakeTimers();
    try {
      const onSelect = jest.fn();
      render(
        <TabSwitcherSheet
          buffers={mixed().buffers}
          activeIndex={0}
          refusal={null}
          onSelect={onSelect}
          onClose={jest.fn()} onPin={jest.fn()} onUnpin={jest.fn()} onCloseOthers={jest.fn()} onCopyPath={jest.fn()}
          onDismiss={jest.fn()} onGoToFiles={jest.fn()}
        />,
      );
      const main = rowButton("b.go, preview");
      fireEvent.pointerDown(main);
      act(() => { jest.advanceTimersByTime(500); });
      expect(screen.getByRole("menu", { name: "b.go actions" })).toBeInTheDocument();
      fireEvent.pointerUp(main);
      fireEvent.click(main);
      expect(onSelect).not.toHaveBeenCalled();
    } finally {
      jest.useRealTimers();
    }
  });

  it("tabSwitcherSheet_should_KeepEveryTouchTarget44px", () => {
    render(<Sheet initial={mixed()} />);
    const sheet = screen.getByTestId("tab-switcher-sheet");
    for (const row of within(sheet).getAllByRole("listitem")) {
      expect(row).toHaveStyle({ minHeight: "44px" });
      for (const b of within(row).getAllByRole("button")) expect(b).toHaveStyle({ minHeight: "44px" });
    }
  });
});
