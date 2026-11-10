import { createMouseWheelForwarder, type WheelLike, type WheelTerminal } from "../ghosttyWheelForward";

const PIXEL = 0;
const LINE = 1;

function setup(over: Partial<WheelTerminal> = {}) {
  const sent: string[] = [];
  const term: WheelTerminal = {
    cols: 100,
    rows: 50,
    hasMouseTracking: () => true,
    cellSize: () => ({ width: 10, height: 20 }),
    gridRect: () => ({ left: 0, top: 0, width: 1000, height: 1000 }),
    ...over,
  };
  const handle = createMouseWheelForwarder(term, (d) => sent.push(d));
  const wheel = (e: Partial<WheelLike>) =>
    handle({ deltaY: 0, deltaMode: PIXEL, shiftKey: false, clientX: 0, clientY: 0, ...e });
  return { sent, wheel };
}

describe("ghostty wheel forwarding", () => {
  it("leaves the wheel to ghostty when the app has not enabled mouse tracking", () => {
    const { sent, wheel } = setup({ hasMouseTracking: () => false });
    expect(wheel({ deltaY: -100 })).toBe(false);
    expect(sent).toEqual([]);
  });

  it("sends wheel-up as an SGR button 64 report at the pointer's cell", () => {
    const { sent, wheel } = setup();
    expect(wheel({ deltaY: -20, clientX: 505, clientY: 250 })).toBe(true);
    expect(sent).toEqual(["\x1b[<64;51;13M"]);
  });

  it("sends wheel-down as button 65", () => {
    const { sent, wheel } = setup();
    wheel({ deltaY: 20, clientX: 0, clientY: 0 });
    expect(sent).toEqual(["\x1b[<65;1;1M"]);
  });

  it("lets Shift+wheel scroll the local scrollback", () => {
    const { sent, wheel } = setup();
    expect(wheel({ deltaY: -100, shiftKey: true })).toBe(false);
    expect(sent).toEqual([]);
  });

  it("accumulates sub-line trackpad deltas instead of dropping them", () => {
    const { sent, wheel } = setup();
    expect(wheel({ deltaY: -8 })).toBe(true);
    expect(wheel({ deltaY: -8 })).toBe(true);
    expect(sent).toEqual([]);
    wheel({ deltaY: -8 });
    expect(sent).toEqual(["\x1b[<64;1;1M"]);
  });

  it("reads line-mode deltas as lines", () => {
    const { sent, wheel } = setup();
    wheel({ deltaY: -2, deltaMode: LINE });
    expect(sent).toEqual(["\x1b[<64;1;1M".repeat(2)]);
  });

  it("drops the carried remainder when the app stops tracking", () => {
    let tracking = true;
    const { sent, wheel } = setup({ hasMouseTracking: () => tracking });
    wheel({ deltaY: -15 });
    tracking = false;
    wheel({ deltaY: -15 });
    tracking = true;
    wheel({ deltaY: -15 });
    expect(sent).toEqual([]);
  });

  it("falls back to cell (1,1) when the canvas cannot be measured", () => {
    const { sent, wheel } = setup({ gridRect: () => undefined });
    wheel({ deltaY: -20, clientX: 400, clientY: 400 });
    expect(sent).toEqual(["\x1b[<64;1;1M"]);
  });
});
