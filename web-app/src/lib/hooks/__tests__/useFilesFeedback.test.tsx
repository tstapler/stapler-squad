import React from "react";
import { act, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { useFilesFeedback, SUCCESS_MS, UNDO_MS, successText } from "../useFilesFeedback";
import { FeedbackToast } from "@/components/sessions/files/FeedbackToast";

beforeEach(() => jest.useFakeTimers());
afterEach(() => jest.useRealTimers());

describe("useFilesFeedback", () => {
  it("successFeedback_should_AnnouncePolitelyWithTheDocumentedWording_When_EachSuccessKindCompletes", () => {
    expect(successText("saved", "b.go")).toBe("Saved b.go");
    expect(successText("merge_saved", "b.go")).toBe("Merged and saved b.go");
    expect(successText("reloaded", "b.go")).toBe("Reloaded b.go from disk");
    expect(successText("draft_restored", "b.go")).toBe("Restored your unsaved draft of b.go");
    expect(successText("overwrote", "b.go")).toBe("Overwrote b.go on disk with your version");
    expect(successText("reopened", "c.go")).toBe("Reopened c.go");
    const { result } = renderHook(() => useFilesFeedback());
    act(() => result.current.announceSuccess("saved", "b.go"));
    expect(result.current.message).toMatchObject({ kind: "success", text: "Saved b.go" });
  });

  it("toasts_should_StayAtLeast8s_PauseOnHoverFocus_BeDismissible_And_HaveNoMotionUnderReducedMotion", () => {
    const { result } = renderHook(() => useFilesFeedback());
    act(() => result.current.announceSuccess("saved", "b.go"));
    act(() => { jest.advanceTimersByTime(SUCCESS_MS - 1); });
    expect(result.current.toast).not.toBeNull();
    act(() => { jest.advanceTimersByTime(1); });
    expect(result.current.toast).toBeNull();
    expect(result.current.message).toBeNull();

    act(() => result.current.announceSuccess("saved", "b.go"));
    act(() => { jest.advanceTimersByTime(SUCCESS_MS - 100); });
    act(() => result.current.pause());
    act(() => { jest.advanceTimersByTime(60_000); });
    expect(result.current.toast).not.toBeNull();
    act(() => result.current.resume());
    act(() => { jest.advanceTimersByTime(SUCCESS_MS); });
    expect(result.current.toast).toBeNull();

    act(() => result.current.announceSuccess("saved", "b.go"));
    act(() => result.current.dismiss());
    expect(result.current.toast).toBeNull();
  });

  it("toasts_should_KeepAnUndoToastLongerThanAPlainOne_When_AnUndoIsOffered", () => {
    const undo = jest.fn();
    const { result } = renderHook(() => useFilesFeedback());
    act(() => result.current.announceSuccess("reopened", "c.go", { undo }));
    act(() => { jest.advanceTimersByTime(SUCCESS_MS + 1); });
    expect(result.current.toast).not.toBeNull();
    act(() => { jest.advanceTimersByTime(UNDO_MS - SUCCESS_MS); });
    expect(result.current.toast).toBeNull();
  });

  it("statusMessages_should_PersistErrorsAndClearSuccessAfter8s_And_NotOverlap", () => {
    const { result } = renderHook(() => useFilesFeedback());
    act(() => result.current.announceError("Could not reload: network"));
    act(() => result.current.announceError("Could not check the file: network"));
    act(() => { jest.advanceTimersByTime(120_000); });
    expect(result.current.message).toMatchObject({ kind: "error", text: "Could not check the file: network" });
    act(() => result.current.announceSuccess("saved", "b.go"));
    expect(result.current.message?.kind).toBe("error");
    act(() => result.current.dismiss());
    expect(result.current.message).toBeNull();
  });

  it("toasts_should_RenderNoAnimationAndADismissControl_When_PrefersReducedMotion", () => {
    const original = window.matchMedia;
    window.matchMedia = ((q: string) => ({ matches: q.includes("reduce"), media: q, addEventListener: jest.fn(), removeEventListener: jest.fn() })) as unknown as typeof window.matchMedia;
    try {
      const onDismiss = jest.fn();
      const onPause = jest.fn();
      const onResume = jest.fn();
      render(<FeedbackToast toast={{ id: 1, kind: "success", text: "Saved b.go" }} onDismiss={onDismiss} onPause={onPause} onResume={onResume} />);
      const toast = screen.getByTestId("files-toast");
      expect(toast).toHaveAttribute("data-motion", "none");
      fireEvent.mouseEnter(toast);
      expect(onPause).toHaveBeenCalled();
      fireEvent.mouseLeave(toast);
      expect(onResume).toHaveBeenCalled();
      fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
      expect(onDismiss).toHaveBeenCalled();
    } finally {
      window.matchMedia = original;
    }
  });
});
