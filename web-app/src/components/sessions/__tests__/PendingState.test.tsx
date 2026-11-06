import React from "react";
import { act, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fakeClock } from "@/lib/files/__tests__/fixtures/fakeClock";
import {
  PENDING_MS, WaitState, failureClass, reloadFailedBanner, usePendingDelay, type WaitKind,
} from "../files/PendingState";
import { BannerStack } from "../files/BannerStack";

const WAITS: { kind: WaitKind; pending: string; failure: string; exits: string[] }[] = [
  { kind: "open", pending: "Opening b.go...", failure: "Could not open b.go: not found", exits: ["Retry", "Close tab", "Reveal in tree"] },
  { kind: "tree", pending: "Loading files...", failure: "Could not load files: network error", exits: ["Retry"] },
  { kind: "restore", pending: "Restoring 3 tabs", failure: "Could not restore: network error", exits: ["Retry", "Close tab"] },
  { kind: "outline", pending: "Reading symbols...", failure: "Could not read symbols", exits: ["Close"] },
  { kind: "stat", pending: "Checking the file on disk...", failure: "Could not check the file: network error", exits: ["Retry", "Keep editing"] },
  { kind: "merge", pending: "Loading merge view...", failure: "Merge view unavailable - try again", exits: ["Retry"] },
];

describe("usePendingDelay", () => {
  it("pendingDelay_should_FireOnlyAfter150ms_And_ResetWhenWaitEnds", () => {
    const { clock, advance } = fakeClock();
    const { result, rerender } = renderHook(({ on }) => usePendingDelay(on, PENDING_MS, clock), { initialProps: { on: true } });
    expect(result.current).toBe(false);
    act(() => advance(149));
    expect(result.current).toBe(false);
    act(() => advance(1));
    expect(result.current).toBe(true);
    rerender({ on: false });
    expect(result.current).toBe(false);
    rerender({ on: true });
    act(() => advance(100));
    rerender({ on: false });
    act(() => advance(500));
    expect(result.current).toBe(false);
  });
});

describe("WaitState", () => {
  it("pendingStates_should_ShowAfter150msWithExitOnFailure_When_OpenTreeRestoreOutlineStatMergeLoad", async () => {
    for (const w of WAITS) {
      const onAction = jest.fn();
      const exits = w.exits.map((label) => ({ label, onClick: () => onAction(label) }));
      const pending = render(<WaitState kind={w.kind} name="b.go" count={3} />);
      const region = screen.getByRole("status");
      expect(region).not.toHaveAttribute("aria-busy"); // busy on a live region can hold its announcement
      expect(region).toHaveTextContent(w.pending);
      pending.unmount();

      const failure = render(<WaitState kind={w.kind} name="b.go" count={3} failure={{ reason: w.kind === "open" ? "not_found" : "rpc_error", actions: exits }} />);
      expect(screen.getByRole("alert")).toHaveTextContent(w.failure);
      for (const label of w.exits) {
        await userEvent.click(screen.getByRole("button", { name: label }));
        expect(onAction).toHaveBeenCalledWith(label);
      }
      failure.unmount();
    }
  });

  it("pendingStates_should_LeaveADirtyBufferUntouched_And_OfferRetryCopyKeep_When_ReloadFails", async () => {
    const retry = jest.fn();
    const copy = jest.fn();
    const keep = jest.fn();
    const dirty = reloadFailedBanner({ path: "b.go", reason: "rpc_error", dirty: true, seq: 1, onRetry: retry, onCopy: copy, onKeep: keep });
    expect(dirty.kind).toBe("reload_failed_dirty");
    render(<BannerStack banners={[dirty]} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Could not reload: network error");
    for (const name of ["Retry", "Copy my version", "Keep editing"]) await userEvent.click(screen.getByRole("button", { name }));
    expect([retry, copy, keep].map((f) => f.mock.calls.length)).toEqual([1, 1, 1]);
    expect(reloadFailedBanner({ path: "b.go", reason: "not_found", dirty: false, seq: 2, onRetry: retry, onCopy: copy, onKeep: keep }).kind).toBe("reload_failed");
  });

  it("pendingStates_should_NameEveryFailureClassWithoutPathsOrErrorText", () => {
    expect(failureClass("not_found")).toBe("not found");
    expect(failureClass("too_large")).toBe("too large");
    expect(failureClass("binary")).toBe("binary file");
    expect(failureClass("encoding")).toBe("unsupported encoding");
    expect(failureClass("worktree_unavailable")).toBe("worktree unavailable");
    expect(failureClass("rpc_error")).toBe("network error");
  });
});
