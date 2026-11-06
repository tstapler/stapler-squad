import React from "react";
import { render, screen, within } from "@testing-library/react";
import { KeyboardShortcutOverlay } from "../KeyboardShortcutOverlay";
import { DiffPane } from "@/components/sessions/DiffPane";

jest.mock("@/components/sessions/files/engine/LezerHighlighter", () => ({
  LezerHighlighter: class {
    tokenize() {
      return Promise.resolve([]);
    }
  },
}));

describe("KeyboardShortcutOverlay Diff section", () => {
  it("shortcutOverlay_should_ListDiffShortcuts_When_DiffPaneMounted", () => {
    render(<DiffPane content="" added={0} removed={0} />);
    render(<KeyboardShortcutOverlay isOpen onClose={jest.fn()} />);
    const diff = screen.getByRole("region", { name: "Diff View" });
    expect(within(diff).getByText("Next change")).toBeInTheDocument();
    expect(within(diff).getByText("Previous change")).toBeInTheDocument();
    expect(within(diff).getByText("Toggle split / unified view")).toBeInTheDocument();
  });
});
