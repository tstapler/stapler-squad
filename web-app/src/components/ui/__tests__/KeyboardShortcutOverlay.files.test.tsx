import React from "react";
import { render, screen, within, fireEvent, waitFor } from "@testing-library/react";
import { KeyboardShortcutOverlay } from "../KeyboardShortcutOverlay";
import { FILES_CHORDS, chordSignature } from "@/lib/hooks/filesChords";

const setKeyboard = (value: unknown) => Object.defineProperty(navigator, "keyboard", { value, configurable: true });

afterEach(() => { delete (navigator as unknown as { keyboard?: unknown }).keyboard; });

describe("KeyboardShortcutOverlay Files section", () => {
  it("shortcutOverlay_should_ListEveryFilesChordWithUsLabelsAndNote_When_LayoutApiAbsent", () => {
    render(<KeyboardShortcutOverlay isOpen onClose={jest.fn()} />);
    const files = screen.getByRole("region", { name: "Files" });
    for (const c of FILES_CHORDS) expect(within(files).getByText(c.label)).toBeInTheDocument();
    expect(within(files).getByText("Alt+[")).toBeInTheDocument();
    expect(within(files).getByText("(the key at the US [ position)", { selector: "span" })).toBeInTheDocument();
    expect(within(files).getByText("Esc, then Tab")).toBeInTheDocument();
    expect(within(files).getByText("Ctrl+Shift+F")).toBeInTheDocument();
    expect(FILES_CHORDS.map(chordSignature)).toContain("Alt+BracketLeft");
  });

  it("shortcutOverlay_should_ShowTheLayoutResolvedKey_When_GetLayoutMapExists", async () => {
    setKeyboard({ getLayoutMap: () => Promise.resolve(new Map([["BracketLeft", "ü"], ["BracketRight", "+"], ["KeyW", "w"]])) });
    render(<KeyboardShortcutOverlay isOpen onClose={jest.fn()} />);
    await waitFor(() => expect(screen.getByText("Alt+Ü")).toBeInTheDocument());
    expect(screen.queryByText(/the key at the US/)).toBeNull();
  });

  it("shortcutOverlay_should_FilterFilesRows_When_SearchTyped", () => {
    render(<KeyboardShortcutOverlay isOpen onClose={jest.fn()} />);
    fireEvent.change(screen.getByRole("searchbox", { name: "Search keyboard shortcuts" }), { target: { value: "jumplist" } });
    const files = screen.getByRole("region", { name: "Files" });
    expect(within(files).getAllByText(/Jumplist/)).toHaveLength(2);
    expect(within(files).queryByText("Quick open")).toBeNull();
  });
});
