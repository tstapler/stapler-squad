import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { NotePanel } from "./NotePanel";

describe("NotePanel", () => {
  it("NotePanel_should_CallOnSaveWithTypedValue_When_SaveClicked", async () => {
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(<NotePanel note="" onSave={onSave} />);

    fireEvent.click(screen.getByRole("button", { name: /add note/i }));
    fireEvent.change(screen.getByTestId("session-note-textarea"), {
      target: { value: "spike — don't merge" },
    });
    fireEvent.click(screen.getByTestId("session-note-save-button"));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("spike — don't merge"));
    await waitFor(() => expect(screen.queryByTestId("session-note-textarea")).toBeNull());
  });

  it("NotePanel_should_PreserveTextareaAndShowAssertiveError_When_OnSaveRejects", async () => {
    const onSave = jest.fn().mockRejectedValue(new Error("save failed"));
    render(<NotePanel note="" onSave={onSave} />);

    fireEvent.click(screen.getByRole("button", { name: /add note/i }));
    fireEvent.change(screen.getByTestId("session-note-textarea"), {
      target: { value: "my note" },
    });
    fireEvent.click(screen.getByTestId("session-note-save-button"));

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(screen.getByTestId("session-note-textarea")).toHaveValue("my note");
    expect(screen.getByRole("alert")).toHaveAttribute("aria-live", "assertive");
    // The error must be reachable from the textarea's accessible description,
    // not just announced once via aria-live — a screen-reader user tabbing back
    // into the field needs a persistent indication it's in an error state.
    const describedBy = screen.getByTestId("session-note-textarea").getAttribute("aria-describedby");
    expect(describedBy?.split(" ")).toContain(screen.getByRole("alert").id);
  });

  it("NotePanel_should_RenderMarkdownElements_When_NoteContainsGfmSyntax", () => {
    render(
      <NotePanel note="**Blocked** — see [PR #482](https://x)" onSave={jest.fn()} />
    );
    const rendered = screen.getByTestId("session-note-rendered");
    expect(rendered.querySelector("strong")).toHaveTextContent("Blocked");
    const link = rendered.querySelector("a");
    expect(link).toHaveAttribute("href", "https://x");
  });

  it("NotePanel_should_RemapHeadingToNonPageLevelTag_When_NoteContainsH1Syntax", () => {
    render(<NotePanel note={"# Heading"} onSave={jest.fn()} />);
    const rendered = screen.getByTestId("session-note-rendered");
    // No real heading element at all — even a remapped h5/h6 would still create a
    // heading-order skip against the page's own h2 session title, so headings must
    // render as a styled non-heading element instead.
    expect(rendered.querySelector("h1,h2,h3,h4,h5,h6")).toBeNull();
    expect(rendered).toHaveTextContent("Heading");
  });

  it("NotePanel_should_RenderDetailsOpenByDefault_When_Mounted", () => {
    render(<NotePanel note="something" onSave={jest.fn()} />);
    expect(screen.getByTestId("session-note-panel")).toHaveAttribute("open");
  });

  it("renders empty state when note is empty", () => {
    render(<NotePanel note="" onSave={jest.fn()} />);
    expect(screen.getByText(/no notes yet/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add note/i })).toBeInTheDocument();
  });

  it("clicking Add note enters edit mode and focuses the textarea", () => {
    render(<NotePanel note="" onSave={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /add note/i }));
    expect(screen.getByTestId("session-note-textarea")).toHaveFocus();
  });

  it("Cancel discards the draft and reverts to the original note", () => {
    render(<NotePanel note="original" onSave={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /edit/i }));
    fireEvent.change(screen.getByTestId("session-note-textarea"), {
      target: { value: "draft text" },
    });
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(screen.queryByTestId("session-note-textarea")).toBeNull();
    expect(screen.getByTestId("session-note-rendered")).toHaveTextContent("original");
  });

  it("NotePanel_should_ReturnFocusToEditButton_When_CancelClicked", () => {
    render(<NotePanel note="original" onSave={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /edit/i }));
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(screen.getByRole("button", { name: /edit/i })).toHaveFocus();
  });

  it("NotePanel_should_ReturnFocusToAddNoteButton_When_SaveSucceeds", async () => {
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(<NotePanel note="" onSave={onSave} />);
    fireEvent.click(screen.getByRole("button", { name: /add note/i }));
    fireEvent.click(screen.getByTestId("session-note-save-button"));
    await waitFor(() => expect(screen.queryByTestId("session-note-textarea")).toBeNull());
    expect(screen.getByRole("button", { name: /add note/i })).toHaveFocus();
  });

  it("NotePanel_should_ShowLiveByteCount_When_Typing", () => {
    render(<NotePanel note="" onSave={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /add note/i }));
    fireEvent.change(screen.getByTestId("session-note-textarea"), {
      target: { value: "hello" },
    });
    expect(document.getElementById("session-note-hint")).toHaveTextContent("Markdown supported. 5/10000");
  });

  it("NotePanel_should_BlockSaveWithByteAccurateError_When_MultiByteNoteExceedsByteCapButNotCharCap", () => {
    // "あ" is 1 UTF-16 code unit (under the 10000-char textarea maxLength) but
    // 3 UTF-8 bytes — 4000 of them is 4000 chars / 12000 bytes, over the
    // backend's byte-based session.MaxNoteLength cap.
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(<NotePanel note="" onSave={onSave} />);
    fireEvent.click(screen.getByRole("button", { name: /add note/i }));
    fireEvent.change(screen.getByTestId("session-note-textarea"), {
      target: { value: "あ".repeat(4000) },
    });
    fireEvent.click(screen.getByTestId("session-note-save-button"));

    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent(/too long/i);
    expect(screen.getByTestId("session-note-textarea")).toHaveValue("あ".repeat(4000));
  });
});
