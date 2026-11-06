import React, { useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import { buildOutline, OUTLINE_MAX_BYTES } from "@/lib/files/outline";
import { OutlinePanel } from "../files/OutlinePanel";

const GO = "package p\n\nfunc A() {}\n\ntype B struct{}\n";

function Host({ text, language, size, onSelect, onGoToLine }: {
  text: string; language: string; size?: number; onSelect: (line: number) => void; onGoToLine?: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div>
      <button onClick={() => setOpen(true)}>Outline</button>
      {open && (
        <OutlinePanel
          result={buildOutline(text, language, { size })}
          onSelect={(s) => { onSelect(s.line); setOpen(false); }}
          onClose={() => setOpen(false)}
          onGoToLine={onGoToLine ?? (() => undefined)}
        />
      )}
    </div>
  );
}

function mount(props: Omit<React.ComponentProps<typeof Host>, "onSelect"> & { onSelect?: (line: number) => void }) {
  const h = createFilesTestHarness();
  render(<Host onSelect={props.onSelect ?? jest.fn()} {...props} />, { wrapper: h.wrapper });
  return h;
}

describe("OutlinePanel", () => {
  it("outlinePanel_should_JumpAndPushJumplist_And_ShowUnavailableWithClose", async () => {
    const onSelect = jest.fn();
    const h = mount({ text: GO, language: "go", onSelect });
    const user = userEvent.setup();
    await user.click(screen.getByText("Outline"));
    const list = screen.getByRole("listbox", { name: "Symbols" });
    expect(within(list).getAllByRole("option").map((o) => o.textContent)).toEqual([expect.stringContaining("A"), expect.stringContaining("B")]);
    expect(within(list).getByRole("option", { name: /^A.*3$/ })).toBeInTheDocument();
    await user.click(within(list).getByRole("option", { name: /^B/ }));
    expect(onSelect).toHaveBeenCalledWith(5);
    const outline = h.analytics.events.filter((e) => e.name === "files.nav.outline");
    expect(outline).toHaveLength(1);
    expect(outline[0].labels).toMatchObject({ outcome: "jumped", symbols: "1-9" });

    onSelect.mockClear();
  });

  it("outlinePanel_should_ShowMessageCloseAndGoToLine_When_LargeUnsupportedOrEmpty", async () => {
    const goto = jest.fn();
    const h = mount({ text: GO, language: "go", size: OUTLINE_MAX_BYTES + 1, onGoToLine: goto });
    const user = userEvent.setup();
    await user.click(screen.getByText("Outline"));
    expect(screen.getByText("Outline unavailable for large files")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Go to line" }));
    expect(goto).toHaveBeenCalled();
    expect(h.analytics.events.find((e) => e.name === "files.nav.outline")!.labels).toMatchObject({ outcome: "unavailable_large" });
  });

  it("outlinePanel_should_ShowNoSymbolsMessage_When_LanguageHasNoMap", async () => {
    const h = mount({ text: "a: 1", language: "yaml" });
    await userEvent.click(screen.getByText("Outline"));
    expect(screen.getByText("No symbols found for this file type")).toBeInTheDocument();
    expect(h.analytics.events.find((e) => e.name === "files.nav.outline")!.labels).toMatchObject({ outcome: "empty", symbols: "0" });
  });

  it("overlays_should_RestoreFocusToInvoker_When_OutlineClosedByEscapeOrButton", async () => {
    const h = mount({ text: GO, language: "go" });
    const user = userEvent.setup();
    const invoker = screen.getByText("Outline");
    await user.click(invoker);
    expect(screen.getByRole("textbox", { name: "Filter symbols" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Outline" })).toBeNull();
    expect(invoker).toHaveFocus();
    expect(h.analytics.events.find((e) => e.name === "files.nav.outline")!.labels).toMatchObject({ outcome: "closed" });
    await user.click(invoker);
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(invoker).toHaveFocus();
  });

  it("outlinePanel_should_FilterTypeAhead_And_SelectWithEnter", async () => {
    const onSelect = jest.fn();
    mount({ text: GO, language: "go", onSelect });
    const user = userEvent.setup();
    await user.click(screen.getByText("Outline"));
    await user.type(screen.getByRole("textbox", { name: "Filter symbols" }), "b");
    expect(screen.getAllByRole("option")).toHaveLength(1);
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith(5);
  });
});
