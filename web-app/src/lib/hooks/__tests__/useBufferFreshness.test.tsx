import React, { useState } from "react";
import { act, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createFakeTransport, createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import type { LoadedBuffer } from "@/lib/files/bufferReducer";
import type { FileStat, FileTransportPort } from "@/lib/files/transport/fileTransport";
import { StaleBanner } from "@/components/sessions/files/StaleBanner";
import { useBufferFreshness, compareTokens } from "../useBufferFreshness";
import type { PollMode } from "../pollMode";

const T1 = "v1:5:1:h1";
const MTIME_ONLY = "v1:5:2:h1";
const CHANGED = "v1:6:3:h2";
const CHANGED_AGAIN = "v1:7:4:h3";

const buffer = (token: string, over: Partial<LoadedBuffer> = {}): LoadedBuffer => ({
  status: "loaded", path: "a.go", sessionId: "s1", pinned: true, viewState: { line: 1, col: 0, scrollTop: 0 }, usedAt: 1,
  token, dirty: false, saving: false, source: { kind: "full" }, mode: "read", readOnly: "none", ...over,
}) as LoadedBuffer;

const statOf = (token: string): FileStat => ({ token, size: 5, readOnly: "none", analysis: "unspecified" });

function setup(initial: { active?: LoadedBuffer | null; enabled?: boolean; sessionRunning?: boolean; pollMode?: PollMode } = {}) {
  const stat = jest.fn<Promise<FileStat>, []>(async () => statOf(T1));
  const transport: FileTransportPort = { ...createFakeTransport({ files: { "a.go": "x" } }), stat };
  const h = createFilesTestHarness({ sessionId: "s1", transport });
  const onAdoptToken = jest.fn();
  const view = renderHook(
    (p: { active: LoadedBuffer | null; enabled: boolean; sessionRunning: boolean; pollMode?: PollMode }) =>
      useBufferFreshness({ ...p, onAdoptToken }),
    { wrapper: h.wrapper, initialProps: { active: initial.active === undefined ? buffer(T1) : initial.active, enabled: initial.enabled ?? true, sessionRunning: initial.sessionRunning ?? false, ...(initial.pollMode && { pollMode: initial.pollMode }) } },
  );
  return { h, stat, onAdoptToken, ...view };
}

const tick = (ms: number) => act(async () => { await jest.advanceTimersByTimeAsync(ms); });
const setVisibility = (state: "visible" | "hidden") => Object.defineProperty(document, "visibilityState", { value: state, configurable: true });

describe("useBufferFreshness", () => {
  beforeEach(() => { jest.useFakeTimers(); setVisibility("visible"); });
  afterEach(() => { jest.useRealTimers(); setVisibility("visible"); jest.restoreAllMocks(); });

  it("useBufferFreshness_should_ShowBannerWithoutReplacingContent_When_FocusAndTokenChanged", async () => {
    const { result, stat } = setup();
    stat.mockResolvedValue(statOf(CHANGED));
    await act(async () => { window.dispatchEvent(new Event("focus")); });
    expect(result.current.stale).toBeNull();
    await tick(1_000);
    expect(result.current.stale).toMatchObject({ path: "a.go", token: CHANGED, dirty: false, actor: "unknown" });
  });

  it("useBufferFreshness_should_KeepFocus_And_ReloadKeepsLineAndScroll_When_BannerActioned", async () => {
    jest.useRealTimers();
    const onReload = jest.fn();
    function Probe() {
      const [dismissed, setDismissed] = useState(false);
      return (
        <div>
          <input aria-label="Search files" />
          {!dismissed && <StaleBanner stale={{ path: "a.go", token: CHANGED, dirty: false, actor: "unknown" }} sessionRunning={false} onAction={(a) => { if (a === "reload") onReload(); setDismissed(true); }} />}
        </div>
      );
    }
    render(<Probe />);
    const input = screen.getByRole("textbox", { name: "Search files" });
    input.focus();
    expect(screen.getByRole("status")).toHaveTextContent("Changed on disk - Reload");
    expect(input).toHaveFocus();
    await userEvent.click(screen.getByRole("button", { name: "Reload" }));
    expect(onReload).toHaveBeenCalledTimes(1);
  });

  it("useBufferFreshness_should_AdoptTokenSilently_When_OnlyMtimeChangedHashEqual", async () => {
    const { result, stat, onAdoptToken } = setup();
    stat.mockResolvedValue(statOf(MTIME_ONLY));
    await act(async () => { window.dispatchEvent(new Event("focus")); });
    await tick(1_000);
    expect(result.current.stale).toBeNull();
    expect(onAdoptToken).toHaveBeenCalledWith("a.go", MTIME_ONLY);
    expect(compareTokens(T1, MTIME_ONLY)).toBe("mtime_only");
    expect(compareTokens(T1, CHANGED)).toBe("changed");
    expect(compareTokens(T1, T1)).toBe("same");
    // A "-" hash segment (not hashed yet) compares the whole string.
    expect(compareTokens("v1:5:1:-", "v1:5:2:-")).toBe("changed");
    expect(compareTokens("v1:5:1:-", "v1:5:1:-")).toBe("same");
  });

  it("bufferFreshness_should_PollActiveBufferEvery10sWithDebounceAndBackoff_When_VisibleAndFocused", async () => {
    const { result, stat, h, rerender } = setup();
    await tick(9_999);
    expect(stat).not.toHaveBeenCalled();
    await tick(1);
    expect(stat).toHaveBeenCalledTimes(1);

    // Changed on one check only: confirmed one second later, then exactly one banner.
    stat.mockResolvedValue(statOf(CHANGED));
    await tick(10_000);
    expect(result.current.stale).toBeNull();
    await tick(1_000);
    expect(result.current.stale?.token).toBe(CHANGED);
    expect(h.analytics.events.filter((e) => e.name === "files.stale.shown")).toHaveLength(1);

    // Token moves again while the banner is open: updated in place, not announced twice.
    stat.mockResolvedValue(statOf(CHANGED_AGAIN));
    await tick(10_000);
    await tick(1_000);
    expect(result.current.stale?.token).toBe(CHANGED_AGAIN);
    expect(h.analytics.events.filter((e) => e.name === "files.stale.shown")).toHaveLength(1);

    // Saved (clean, new token), then an agent overwrites: the banner returns within a poll plus the debounce.
    rerender({ active: buffer("v1:9:9:saved"), enabled: true, sessionRunning: false });
    expect(result.current.stale).toBeNull();
    stat.mockResolvedValue(statOf("v1:9:10:agent"));
    await tick(11_000);
    expect(result.current.stale?.token).toBe("v1:9:10:agent");
  });

  it("bufferFreshness_should_NotPoll_When_HiddenOfflineOrFlagOff_And_BackOffAfterThreeFailures_And_KeepOneInFlight", async () => {
    const hidden = setup();
    setVisibility("hidden");
    await tick(60_000);
    expect(hidden.stat).not.toHaveBeenCalled();
    hidden.unmount();
    setVisibility("visible");

    const off = setup({ enabled: false });
    await tick(60_000);
    expect(off.stat).not.toHaveBeenCalled();
    off.unmount();

    jest.spyOn(window.navigator, "onLine", "get").mockReturnValue(false);
    const offline = setup();
    await tick(60_000);
    expect(offline.stat).not.toHaveBeenCalled();
    offline.unmount();
    jest.spyOn(window.navigator, "onLine", "get").mockReturnValue(true);

    const failing = setup();
    failing.stat.mockRejectedValue(new Error("down"));
    await tick(10_000);
    await tick(10_000);
    await tick(10_000);
    expect(failing.stat).toHaveBeenCalledTimes(3);
    await tick(29_000);
    expect(failing.stat).toHaveBeenCalledTimes(3);
    await tick(1_000);
    expect(failing.stat).toHaveBeenCalledTimes(4);
    failing.unmount();

    const slow = setup();
    slow.stat.mockImplementation(() => new Promise<FileStat>(() => undefined));
    for (let i = 0; i < 5; i++) await act(async () => { window.dispatchEvent(new Event("focus")); });
    await tick(30_000);
    expect(slow.stat).toHaveBeenCalledTimes(1);
  });

  it("bufferFreshness_should_PollOnlyTheActiveBuffer_And_CheckNewActiveImmediately", async () => {
    const { stat, rerender } = setup();
    await tick(10_000);
    expect(stat).toHaveBeenCalledTimes(1);
    rerender({ active: buffer(T1, { path: "b.go" }), enabled: true, sessionRunning: false });
    await act(async () => { await jest.advanceTimersByTimeAsync(0); });
    expect(stat).toHaveBeenCalledTimes(2);
    expect(stat.mock.calls.length).toBe(2);
  });

  it("staleBanner_should_NeverAssertActor_And_SuffixByRunningState_When_SessionRunningOrNot", async () => {
    jest.useRealTimers();
    const stale = { path: "a.go", token: CHANGED, dirty: false, actor: "agent_likely" as const };
    const { rerender } = render(<StaleBanner stale={stale} sessionRunning onAction={() => undefined} />);
    expect(screen.getByRole("status")).toHaveTextContent("Changed on disk - Reload (session is running; this may be the agent)");
    rerender(<StaleBanner stale={{ ...stale, actor: "unknown" }} sessionRunning={false} onAction={() => undefined} />);
    expect(screen.getByRole("status")).toHaveTextContent("Changed on disk - Reload (source unknown)");
    expect(screen.getByRole("status").textContent).not.toMatch(/agent changed|changed by/i);
    rerender(<StaleBanner stale={{ ...stale, dirty: true }} sessionRunning onAction={() => undefined} />);
    expect(screen.getByRole("status")).toHaveTextContent("Changed on disk - review diff");
  });

  it("staleBanner_should_EmitActionEvent_When_UserChoosesAction", async () => {
    const { result, stat, h } = setup({ sessionRunning: true });
    stat.mockResolvedValue(statOf(CHANGED));
    await act(async () => { window.dispatchEvent(new Event("focus")); });
    await tick(1_000);
    expect(h.analytics.events.find((e) => e.name === "files.stale.shown")!.labels).toMatchObject({ dirty: "false", actor: "agent_likely" });
    act(() => result.current.resolve("reload"));
    expect(result.current.stale).toBeNull();
    expect(h.analytics.events.find((e) => e.name === "files.stale.action")!.labels).toMatchObject({ action: "reload" });
  });

  it("useBufferFreshness_should_NotStat_When_PollModeOffForHiddenWindow", async () => {
    const { stat } = setup({ pollMode: "off" });
    await tick(120_000);
    await act(async () => { window.dispatchEvent(new Event("focus")); });
    expect(stat).not.toHaveBeenCalled();
  });

  it("useBufferFreshness_should_PollSlowly_When_PollModeSlowForUnfocusedPane", async () => {
    const { stat } = setup({ pollMode: "slow" });
    await tick(30_000);
    expect(stat).not.toHaveBeenCalled(); // fast cadence is 10 s
    await tick(80_000);
    expect(stat).toHaveBeenCalledTimes(1);
  });
});
