import React from "react";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StatusBar, readOnlyChipText } from "../files/StatusBar";
import { useFilesFeedback } from "@/lib/hooks/useFilesFeedback";

const base = { line: 1, col: 1, eol: "CRLF" as const, encoding: "UTF-8", language: "Markdown", readOnly: "none" as const, dirty: false };

describe("StatusBar", () => {
  it("statusBar_should_ShowModeLnColEolEncodingLanguageReasonDirtyOffline_And_ChipsAreButtons", async () => {
    render(<StatusBar {...base} readOnly="truncated" readOnlyDetail="first 1 MB of 3.2 MB" dirty offline message={null} />);
    expect(screen.getByText("Ln 1, Col 1")).toBeInTheDocument();
    expect(screen.getByText("CRLF")).toBeInTheDocument();
    expect(screen.getByText("UTF-8")).toBeInTheDocument();
    expect(screen.getByText("Markdown")).toBeInTheDocument();
    expect(screen.getByText(/Modified/)).toBeInTheDocument();
    expect(screen.getByText("Offline")).toBeInTheDocument();
    const chip = screen.getByRole("button", { name: "Read-only: showing first 1 MB of 3.2 MB" });
    chip.focus();
    await userEvent.keyboard("{Enter}");
    expect(chip).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("note")).toHaveTextContent(/cannot be edited/i);
    await userEvent.keyboard("{Escape}");
    expect(chip).toHaveAttribute("aria-expanded", "false");
    expect(chip).toHaveFocus();
  });

  it("statusBar_should_OmitTheReasonChip_When_BufferIsEditable", () => {
    render(<StatusBar {...base} message={null} />);
    expect(screen.queryByText(/Read-only/)).toBeNull();
    expect(screen.queryByText(/Modified/)).toBeNull();
    expect(screen.queryByText("Offline")).toBeNull();
  });

  it("statusBar_should_NameEveryReadOnlyReason_When_Rendered", () => {
    for (const reason of ["flag_off", "binary", "truncated", "too_large", "non_utf8", "mixed_eol", "symlink", "worktree_missing", "hidden_session", "git_internal"] as const) {
      expect(readOnlyChipText(reason)).toMatch(/^Read-only: \S/);
    }
  });

  describe("message region", () => {
    function Harness() {
      const f = useFilesFeedback();
      return (
        <div>
          <button onClick={() => f.announceSuccess("saved", "b.go")}>ok</button>
          <button onClick={() => f.announceError("Could not save: permission")}>fail</button>
          <StatusBar {...base} message={f.message} onDismissMessage={f.dismiss} onPause={f.pause} onResume={f.resume} />
        </div>
      );
    }

    beforeEach(() => jest.useFakeTimers());
    afterEach(() => jest.useRealTimers());

    it("statusMessages_should_PersistErrorsAndClearSuccessAfter8s_And_NotOverlap", () => {
      render(<Harness />);
      const region = screen.getByTestId("files-status-message");
      expect(region).toHaveAttribute("aria-live", "polite");
      act(() => screen.getByText("ok").click());
      expect(region).toHaveTextContent("Saved b.go");
      act(() => { jest.advanceTimersByTime(8000); });
      expect(region).toHaveTextContent("");
      act(() => screen.getByText("fail").click());
      act(() => { jest.advanceTimersByTime(60_000); });
      expect(region).toHaveTextContent("Could not save: permission");
      act(() => screen.getByRole("button", { name: "Dismiss message" }).click());
      expect(region).toHaveTextContent("");
    });
  });
});
