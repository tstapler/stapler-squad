import type { Terminal as GhosttyWebTerminal } from "ghostty-web";
import { attachGhosttyTouch, LONG_PRESS_MS } from "../ghosttyTouch";

function setup(uri?: string) {
  const el = document.createElement("div");
  const canvas = document.createElement("canvas");
  el.appendChild(canvas);
  document.body.appendChild(el);
  const term = {
    element: el,
    hasSelection: () => false,
    clearSelection: jest.fn(),
    getScrollbackLength: () => 0,
    getViewportY: () => 0,
    renderer: { charWidth: 10, charHeight: 20 },
    linkDetector: { getLinkAt: jest.fn().mockResolvedValue(uri ? { text: uri } : undefined) },
  } as unknown as GhosttyWebTerminal;
  const events: string[] = [];
  for (const t of ["mousedown", "mousemove", "dblclick", "wheel"]) canvas.addEventListener(t, () => events.push(t));
  document.addEventListener("mouseup", () => events.push("mouseup"));
  const touch = (type: string, x = 5, y = 5) => {
    const e = new Event(type, { bubbles: true, cancelable: true }) as Event & Record<string, unknown>;
    const pt = { clientX: x, clientY: y };
    e.touches = type === "touchend" || type === "touchcancel" ? [] : [pt];
    e.changedTouches = [pt];
    canvas.dispatchEvent(e);
  };
  return { term, touch, events };
}

describe("attachGhosttyTouch", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("reports the link under a tap", async () => {
    const { term, touch } = setup("https://example.com");
    const onLink = jest.fn();
    attachGhosttyTouch(term, { onLink });
    touch("touchstart");
    touch("touchend");
    await Promise.resolve();
    await Promise.resolve();
    expect(onLink).toHaveBeenCalledWith("https://example.com", 5, 5);
  });

  it("long-press with no movement replays mousedown, mouseup, dblclick (word select)", () => {
    const { term, touch, events } = setup();
    attachGhosttyTouch(term, { onLink: jest.fn() });
    touch("touchstart");
    jest.advanceTimersByTime(LONG_PRESS_MS);
    touch("touchend");
    expect(events).toEqual(["mousedown", "mouseup", "dblclick"]);
  });

  it("long-press then drag replays mousemove and does not word-select", () => {
    const { term, touch, events } = setup();
    attachGhosttyTouch(term, { onLink: jest.fn() });
    touch("touchstart");
    jest.advanceTimersByTime(LONG_PRESS_MS);
    touch("touchmove", 60, 5);
    touch("touchend", 60, 5);
    expect(events).toEqual(["mousedown", "mousemove", "mouseup"]);
  });

  it("a drag scrolls via wheel events with finger-down as negative delta", () => {
    const { term, touch, events } = setup();
    attachGhosttyTouch(term, { onLink: jest.fn() });
    const deltas: number[] = [];
    term.element?.querySelector("canvas")?.addEventListener("wheel", (e) => deltas.push((e as WheelEvent).deltaY));
    touch("touchstart", 5, 100);
    touch("touchmove", 5, 130);
    touch("touchmove", 5, 150);
    touch("touchend", 5, 150);
    expect(deltas).toEqual([-30, -20]);
    expect(events.filter((e) => e === "wheel")).toHaveLength(2);
  });

  it("a drag before the long-press fires is not a tap or a selection", async () => {
    const { term, touch, events } = setup("https://example.com");
    const onLink = jest.fn();
    attachGhosttyTouch(term, { onLink });
    touch("touchstart");
    touch("touchmove", 5, 80);
    jest.advanceTimersByTime(LONG_PRESS_MS);
    touch("touchend", 5, 80);
    await Promise.resolve();
    expect(events.filter((e) => e !== "wheel")).toEqual([]);
    expect(onLink).not.toHaveBeenCalled();
  });
});
