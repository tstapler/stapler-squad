import { isMouseTracking } from "../mouseTracking";
import type { Terminal } from "@xterm/xterm";

function makeTerminal(mouseTrackingMode: string | undefined): Terminal {
  return {
    modes: mouseTrackingMode !== undefined ? { mouseTrackingMode } : undefined,
  } as unknown as Terminal;
}

describe("isMouseTracking", () => {
  it("returns false when mode is 'none'", () => {
    expect(isMouseTracking(makeTerminal("none"))).toBe(false);
  });

  it("returns false when modes is undefined", () => {
    expect(isMouseTracking(makeTerminal(undefined))).toBe(false);
  });

  it("returns true when mode is 'any'", () => {
    expect(isMouseTracking(makeTerminal("any"))).toBe(true);
  });

  it("returns true when mode is 'x10'", () => {
    expect(isMouseTracking(makeTerminal("x10"))).toBe(true);
  });

  it("returns true when mode is 'normal'", () => {
    expect(isMouseTracking(makeTerminal("normal"))).toBe(true);
  });
});
