/**
 * Task 3.1.2a: fitOnShow stops polling while its anchor is hidden (ResizeObserver
 * reports 0x0) and re-arms when the observer reports a real size.
 */
import React from "react";
import { render, act } from "@testing-library/react";
import { TerminalPoolProvider, usePooledTerminal } from "../TerminalPool";

const fitSpy = jest.fn();
const focusSpy = jest.fn();

jest.mock("@/components/sessions/XtermTerminal", () => {
  const ReactLib = require("react");
  const XtermTerminal = ReactLib.forwardRef((_props: unknown, ref: unknown) => {
    ReactLib.useImperativeHandle(ref, () => ({
      terminal: { cols: 80, rows: 24 },
      focus: focusSpy,
      fit: fitSpy,
    }));
    return ReactLib.createElement("div", { "data-testid": "mock-xterm-terminal" });
  });
  return { XtermTerminal };
});

type RoCallback = (entries: unknown[]) => void;
const FRAMES = 300;

let roCallbacks: RoCallback[];
let rafQueue: Map<number, FrameRequestCallback>;
let nextRaf: number;
let rectSpy: jest.SpyInstance;
let originalRO: typeof ResizeObserver;

const rect = (w: number, h: number) =>
  ({ width: w, height: h, top: 0, left: 0, right: w, bottom: h, x: 0, y: 0, toJSON: () => ({}) }) as DOMRect;
const entry = (w: number, h: number) => [{ contentRect: { width: w, height: h } }];

function runFrames(n: number) {
  for (let i = 0; i < n; i++) {
    const pending = Array.from(rafQueue.entries());
    rafQueue.clear();
    pending.forEach(([, cb]) => cb(0));
  }
}

beforeEach(() => {
  jest.clearAllMocks();
  roCallbacks = [];
  rafQueue = new Map();
  nextRaf = 1;
  originalRO = global.ResizeObserver;
  (global as any).ResizeObserver = class {
    constructor(cb: RoCallback) { roCallbacks.push(cb); }
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  jest.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => {
    const id = nextRaf++;
    rafQueue.set(id, cb);
    return id;
  });
  jest.spyOn(window, "cancelAnimationFrame").mockImplementation((id) => void rafQueue.delete(id));
  rectSpy = jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(rect(0, 0));
});

afterEach(() => {
  (global as any).ResizeObserver = originalRO;
  jest.restoreAllMocks();
});

function Consumer() {
  const dockRef = React.useRef<HTMLDivElement | null>(null);
  usePooledTerminal("s1", dockRef, true);
  return <div ref={dockRef} />;
}

describe("TerminalPool fitOnShow hidden gating", () => {
  it("fitOnShow_should_StopRetryLoop_When_AnchorDisplayNone_And_RearmOnShow", async () => {
    render(
      <TerminalPoolProvider>
        <Consumer />
      </TerminalPoolProvider>
    );
    await act(async () => {});

    act(() => roCallbacks.forEach((cb) => cb(entry(0, 0)))); // first hidden detection
    runFrames(1);
    rectSpy.mockClear();
    act(() => runFrames(FRAMES));
    expect(rectSpy).toHaveBeenCalledTimes(0);
    expect(rafQueue.size).toBe(0);
    expect(fitSpy).not.toHaveBeenCalled();

    rectSpy.mockReturnValue(rect(800, 600));
    act(() => roCallbacks.forEach((cb) => cb(entry(800, 600)))); // shown
    act(() => runFrames(2));
    expect(fitSpy).toHaveBeenCalledTimes(1);
    expect(focusSpy).toHaveBeenCalledTimes(1);
  });
});
