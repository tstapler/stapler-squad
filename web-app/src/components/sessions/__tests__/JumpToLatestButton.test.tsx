// @feature terminal-jump-to-latest
import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { readFileSync } from "fs";
import { join } from "path";
import { JumpToLatestButton, type JumpTerminal, type JumpToLatestButtonProps } from "../JumpToLatestButton";
import { createNetPagesUpTracker, type NetPagesUpTracker } from "@/lib/terminal/scrollPosition";
import { PAGE_DOWN_BYTES, PAGE_RATE_LIMIT_MS } from "@/lib/terminal/scrollRouting";

const ANNOUNCEMENT_DEBOUNCE_MS = 2000;

function makeTerminal(overrides: { rows?: number; viewportY?: number; baseY?: number; length?: number } = {}) {
  const listeners: Array<() => void> = [];
  const buffer = {
    viewportY: overrides.viewportY ?? 10,
    baseY: overrides.baseY ?? 100,
    length: overrides.length ?? 124,
  };
  const terminal = {
    rows: overrides.rows ?? 24,
    buffer: { active: buffer },
    scrollToBottom: jest.fn(),
    focus: jest.fn(),
    onScroll: (cb: () => void) => {
      listeners.push(cb);
      return { dispose: jest.fn() };
    },
    onWriteParsed: (cb: () => void) => {
      listeners.push(cb);
      return { dispose: jest.fn() };
    },
  };
  return { terminal, buffer, fire: () => act(() => listeners.forEach((l) => l())) };
}

interface Harness {
  props: JumpToLatestButtonProps;
  tracker: NetPagesUpTracker;
  sendData: jest.Mock;
  term: ReturnType<typeof makeTerminal>;
}

function harness(over: Partial<JumpToLatestButtonProps> = {}, termOverrides = {}): Harness {
  const term = makeTerminal(termOverrides);
  const tracker = createNetPagesUpTracker();
  const sendData = jest.fn();
  const props: JumpToLatestButtonProps = {
    terminal: term.terminal as unknown as JumpTerminal,
    route: "xterm-local",
    netPagesUp: tracker,
    connectionEpoch: 0,
    sendData,
    ...over,
  };
  return { props, tracker, sendData, term };
}

const button = () => screen.queryByRole("button");
const pageUp = (t: NetPagesUpTracker, n: number) => act(() => { for (let i = 0; i < n; i++) t.pageUp(); });

beforeEach(() => jest.useFakeTimers());
afterEach(() => {
  jest.useRealTimers();
  jest.restoreAllMocks();
});

describe("visibility", () => {
  it("jumpToLatest_should_ShowFixedLabelPerRoute_When_AwayFromLive", () => {
    const local = harness();
    const { unmount } = render(<JumpToLatestButton {...local.props} />);
    expect(button()?.textContent).toContain("Jump to latest");
    unmount();

    const tui = harness({ route: "tui-pgkeys" });
    render(<JumpToLatestButton {...tui.props} />);
    expect(button()).toBeNull(); // no pages sent yet
    pageUp(tui.tracker, 2);
    expect(button()?.textContent).toContain("Page down to latest");
  });

  it("jumpToLatest_should_BeHiddenInTui_When_EstimateInvalid", () => {
    const h = harness({ route: "tui-pgkeys" });
    render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 2);
    expect(button()).not.toBeNull();
    act(() => h.tracker.invalidate("keystroke"));
    expect(button()).toBeNull();
  });

  it("jumpToLatest_should_BeHidden_When_FewerThanFiveRows", () => {
    const four = harness({}, { rows: 4 });
    const { unmount } = render(<JumpToLatestButton {...four.props} />);
    expect(button()).toBeNull();
    unmount();
    const five = harness({}, { rows: 5 });
    render(<JumpToLatestButton {...five.props} />);
    expect(button()).not.toBeNull();
  });

  it("jumpToLatest_should_BeHidden_When_BufferEmptyOrAtLive", () => {
    const empty = harness({}, { viewportY: 0, baseY: 0, length: 0 });
    const { unmount } = render(<JumpToLatestButton {...empty.props} />);
    expect(button()).toBeNull();
    unmount();

    const live = harness({}, { viewportY: 100, baseY: 100 });
    render(<JumpToLatestButton {...live.props} />);
    expect(button()).toBeNull();
    live.term.buffer.viewportY = 40; // user scrolls up
    live.term.fire();
    expect(button()).not.toBeNull();
  });
});

describe("tap", () => {
  it("jumpToLatest_should_NotFocusTerminal_When_Tapped", () => {
    const h = harness();
    render(<JumpToLatestButton {...h.props} />);
    const notPrevented = fireEvent.mouseDown(button()!);
    expect(notPrevented).toBe(false); // preventDefault keeps focus (and the keyboard) where it was
    fireEvent.click(button()!);
    expect(h.term.terminal.scrollToBottom).toHaveBeenCalledTimes(1);
    expect(h.term.terminal.focus).not.toHaveBeenCalled();
    expect(h.sendData).not.toHaveBeenCalled();
  });

  it("jumpToLatest_should_SendAtMostFivePgDn_When_Tapped", () => {
    const h = harness({ route: "tui-pgkeys" });
    render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 5);
    fireEvent.click(button()!);

    expect(h.sendData).toHaveBeenCalledTimes(1); // first key immediately
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS - 1); });
    expect(h.sendData).toHaveBeenCalledTimes(1);
    act(() => { jest.advanceTimersByTime(1); });
    expect(h.sendData).toHaveBeenCalledTimes(2);
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 10); });
    expect(h.sendData).toHaveBeenCalledTimes(5);
    expect(h.sendData.mock.calls.every((c) => c[0] === PAGE_DOWN_BYTES)).toBe(true);
    expect(h.tracker.getState()).toEqual({ pages: 0, valid: true });
    expect(h.term.terminal.scrollToBottom).not.toHaveBeenCalled();
    expect(button()).toBeNull();
  });

  it("jumpToLatest_should_StopSending_When_UnmountedMidSequence", () => {
    const h = harness({ route: "tui-pgkeys" });
    const { unmount } = render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 3);
    fireEvent.click(button()!);
    unmount();
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 5); });
    expect(h.sendData).toHaveBeenCalledTimes(1);
  });
});

describe("queued page keys", () => {
  const tuiHarness = (over: Partial<JumpToLatestButtonProps> = {}) => harness({ route: "tui-pgkeys", ...over });

  it("jumpToLatest_should_ClampToFivePgDn_When_EstimateExceedsLimit", () => {
    const tracker = createNetPagesUpTracker(50); // a tracker that tolerates >5 pages, so only the click clamp bounds the sequence
    const h = tuiHarness({ netPagesUp: tracker });
    render(<JumpToLatestButton {...h.props} />);
    pageUp(tracker, 9);
    fireEvent.click(button()!);
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 20); });
    expect(h.sendData).toHaveBeenCalledTimes(5);
  });

  it("jumpToLatest_should_CancelQueuedKeys_When_EstimateInvalidatedMidSequence", () => {
    const h = tuiHarness();
    render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 5);
    fireEvent.click(button()!);
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS); });
    expect(h.sendData).toHaveBeenCalledTimes(2);
    act(() => { h.tracker.invalidate("keystroke"); });
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 10); });
    expect(h.sendData).toHaveBeenCalledTimes(2);
    expect(jest.getTimerCount()).toBe(0);
  });

  it("jumpToLatest_should_CancelQueuedKeys_When_ConnectionEpochChangesMidSequence", () => {
    const h = tuiHarness();
    const { rerender } = render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 5);
    fireEvent.click(button()!);
    rerender(<JumpToLatestButton {...h.props} connectionEpoch={1} />);
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 10); });
    expect(h.sendData).toHaveBeenCalledTimes(1);
  });

  it("jumpToLatest_should_CancelQueuedKeys_When_RouteFlipsMidSequence", () => {
    const h = tuiHarness();
    const { rerender } = render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 5);
    fireEvent.click(button()!);
    rerender(<JumpToLatestButton {...h.props} route="xterm-local" />);
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 10); });
    expect(h.sendData).toHaveBeenCalledTimes(1);
  });

  it("jumpToLatest_should_LeaveNoPendingTimers_When_SequenceCompletes", () => {
    const h = tuiHarness();
    render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 5);
    fireEvent.click(button()!);
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 10); });
    expect(h.sendData).toHaveBeenCalledTimes(5);
    expect(jest.getTimerCount()).toBe(0);
  });

  it("jumpToLatest_should_DoNothing_When_ChunkedPasteInFlightAtTap", () => {
    const h = tuiHarness({ isInputBusy: () => true });
    render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 3);
    fireEvent.click(button()!);
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 10); });
    expect(h.sendData).not.toHaveBeenCalled();
    expect(h.tracker.getState()).toEqual({ pages: 3, valid: true }); // estimate untouched
  });

  it("jumpToLatest_should_SkipQueuedKeys_When_PasteStartsMidSequence", () => {
    let busy = false;
    const h = tuiHarness({ isInputBusy: () => busy });
    render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 4);
    fireEvent.click(button()!);
    busy = true;
    act(() => { jest.advanceTimersByTime(PAGE_RATE_LIMIT_MS * 10); });
    expect(h.sendData).toHaveBeenCalledTimes(1);
  });

  it("jumpToLatest_should_StillScrollLocalBuffer_When_ChunkedPasteInFlight", () => {
    const h = harness({ isInputBusy: () => true });
    render(<JumpToLatestButton {...h.props} />);
    fireEvent.click(button()!);
    expect(h.term.terminal.scrollToBottom).toHaveBeenCalledTimes(1);
  });
});

describe("output while scrolled", () => {
  function renderWithTick() {
    const h = harness({ outputTick: 0 });
    const utils = render(<JumpToLatestButton {...h.props} />);
    const tick = (n: number) => utils.rerender(<JumpToLatestButton {...h.props} outputTick={n} />);
    return { h, tick };
  }

  it("jumpToLatest_should_HoldPosition_When_OutputArrivesWhileScrolledUp", () => {
    const { h, tick } = renderWithTick();
    expect(screen.queryByTestId("jump-new-output-dot")).toBeNull();
    tick(1);
    expect(h.term.terminal.scrollToBottom).not.toHaveBeenCalled();
    expect(h.term.buffer.viewportY).toBe(10);
    expect(button()?.textContent).toContain("Jump to latest"); // label never changes
    expect(screen.getByTestId("jump-new-output-dot")).toBeInTheDocument();
    expect(button()?.textContent).toContain("new output");
  });

  it("jumpToLatest_should_AnnounceOncePerEpisode", () => {
    const { h, tick } = renderWithTick();
    const region = screen.getByRole("status");
    const wait = (ms: number) => act(() => { jest.advanceTimersByTime(ms); });
    // Each announcement is an empty -> text transition; count them at every step.
    let announcements = 0;
    let previous = region.textContent;
    const step = () => {
      if (previous === "" && region.textContent === "New output below") announcements++;
      previous = region.textContent;
    };

    tick(1);
    wait(ANNOUNCEMENT_DEBOUNCE_MS - 1);
    step();
    expect(region.textContent).toBe("");
    wait(1);
    step();
    expect(region.textContent).toBe("New output below");

    tick(2);
    step();
    tick(3);
    wait(ANNOUNCEMENT_DEBOUNCE_MS * 2);
    step();
    expect(region.textContent).toBe("New output below");
    expect(announcements).toBe(1); // more output in the same episode: no second announcement

    // Back at the bottom re-arms; a second scrolled-away episode announces again.
    h.term.buffer.viewportY = 100;
    h.term.fire();
    step();
    expect(region.textContent).toBe("");
    h.term.buffer.viewportY = 20;
    h.term.fire();
    tick(4);
    wait(ANNOUNCEMENT_DEBOUNCE_MS);
    step();
    expect(announcements).toBe(2);
  });
});

describe("placement", () => {
  const rect = (top: number, bottom: number) => ({ top, bottom, left: 0, right: 100 });

  function mockButtonRect(r: ReturnType<typeof rect>) {
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ ...r, x: r.left, y: r.top, width: 100, height: r.bottom - r.top, toJSON: () => ({}) });
  }
  const corner = () => button()!.getAttribute("data-corner");

  it("jumpToLatest_should_MoveTopRight_When_CursorRowIntersects", () => {
    mockButtonRect(rect(300, 344));
    const h = harness({ getCursorRowRect: () => rect(310, 330) });
    render(<JumpToLatestButton {...h.props} />);
    expect(corner()).toBe("top-right");
  });

  it("jumpToLatest_should_StayBottomRight_When_CursorRowClear", () => {
    mockButtonRect(rect(300, 344));
    const h = harness({ getCursorRowRect: () => rect(20, 40) });
    render(<JumpToLatestButton {...h.props} />);
    expect(corner()).toBe("bottom-right");
  });

  it("jumpToLatest_should_StayTopRight_When_OverlapPersistsAcrossReEvaluations", () => {
    mockButtonRect(rect(300, 344));
    const h = harness({ getCursorRowRect: () => rect(310, 330) });
    const { rerender } = render(<JumpToLatestButton {...h.props} />);
    expect(corner()).toBe("top-right");
    for (let i = 0; i < 2; i++) {
      rerender(<JumpToLatestButton {...h.props} gestureActive />);
      rerender(<JumpToLatestButton {...h.props} gestureActive={false} />);
      expect(corner()).toBe("top-right");
    }
  });

  it("jumpToLatest_should_KeepCorner_When_GestureInProgress", () => {
    mockButtonRect(rect(300, 344));
    let cursor = rect(20, 40);
    const h = harness({ getCursorRowRect: () => cursor });
    const { rerender } = render(<JumpToLatestButton {...h.props} />);
    expect(corner()).toBe("bottom-right");

    cursor = rect(310, 330); // cursor moves under the button mid-drag
    rerender(<JumpToLatestButton {...h.props} gestureActive />);
    h.term.buffer.viewportY = 11;
    h.term.fire();
    expect(corner()).toBe("bottom-right"); // never relocates mid-gesture

    rerender(<JumpToLatestButton {...h.props} gestureActive={false} />);
    expect(corner()).toBe("top-right"); // re-evaluated once the gesture ends
  });
});

describe("reconnect", () => {
  it("jumpToLatest_should_ResetEstimate_When_ConnectionEpochChanges", () => {
    const h = harness({ route: "tui-pgkeys" });
    const { rerender } = render(<JumpToLatestButton {...h.props} />);
    pageUp(h.tracker, 3);
    expect(button()).not.toBeNull();
    rerender(<JumpToLatestButton {...h.props} connectionEpoch={1} />);
    expect(button()).toBeNull();
    expect(h.tracker.getState()).toEqual({ pages: 0, valid: false, reason: "reconnect" });
  });
});

describe("styles", () => {
  it("jumpToLatest_should_HaveNoTransition_When_ReducedMotion", () => {
    // jest maps *.css.ts to a style mock, so assert on the source (same approach as XtermTerminal.overscroll.test.ts).
    const src = readFileSync(join(__dirname, "../JumpToLatestButton.css.ts"), "utf8");
    const media = src.match(/prefers-reduced-motion:\s*reduce[^}]*\}/);
    expect(media).not.toBeNull();
    expect(media![0]).toMatch(/transition:\s*"none"/);
    expect(media![0]).toMatch(/animation:\s*"none"/);
  });

  it("jumpToLatest_should_Be44pxTarget", () => {
    const src = readFileSync(join(__dirname, "../JumpToLatestButton.css.ts"), "utf8");
    expect(src).toMatch(/minHeight:\s*"44px"/);
    expect(src).toMatch(/minWidth:\s*"44px"/);
  });
});
