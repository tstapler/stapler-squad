import {
  clampTop,
  computeBounds,
  describeY,
  movedPastThreshold,
  parseBellPos,
  stepY,
  topToY,
  yToTop,
} from "@/lib/utils/bellPosition";

const bounds = computeBounds({ viewportHeight: 800, safeTop: 20, safeBottom: 10, bottomInset: 0, handleHeight: 88 });

describe("bellPosition", () => {
  it("bounds keep an 8px margin and the handle height inside the viewport", () => {
    expect(bounds).toEqual({ minTop: 28, maxTop: 694 });
  });

  it("bounds never invert on a tiny viewport", () => {
    const b = computeBounds({ viewportHeight: 50, safeTop: 0, safeBottom: 0, bottomInset: 0, handleHeight: 88 });
    expect(b.maxTop).toBe(b.minTop);
  });

  it("clamps, and round-trips y through top", () => {
    expect(clampTop(-50, bounds)).toBe(28);
    expect(clampTop(5000, bounds)).toBe(694);
    expect(topToY(yToTop(0.25, bounds), bounds)).toBeCloseTo(0.25);
  });

  it("uses a larger threshold for touch than mouse", () => {
    expect(movedPastThreshold(0, 6, "mouse")).toBe(true);
    expect(movedPastThreshold(0, 6, "touch")).toBe(false);
    expect(movedPastThreshold(0, 10, "touch")).toBe(true);
    expect(movedPastThreshold(3, 4, "mouse")).toBe(false);
  });

  it("steps and clamps keyboard moves, large with shift", () => {
    expect(stepY(0.5, "down", false)).toBeCloseTo(0.55);
    expect(stepY(0.5, "up", true)).toBeCloseTo(0.3);
    expect(stepY(0.02, "up", false)).toBe(0);
    expect(stepY(0.99, "down", true)).toBe(1);
  });

  it("names the third of the screen", () => {
    expect([0, 0.5, 1].map(describeY)).toEqual(["top", "middle", "bottom"]);
  });

  it.each([null, "", "nope", "{}", '{"v":2,"edge":"left","y":0.5}', '{"v":1,"edge":"top","y":0.5}', '{"v":1,"edge":"left","y":"x"}'])(
    "treats %p as junk",
    (raw) => {
      expect(parseBellPos(raw)).toBeNull();
    },
  );

  it("parses a valid record and clamps y", () => {
    expect(parseBellPos('{"v":1,"edge":"left","y":3}')).toEqual({ v: 1, edge: "left", y: 1 });
  });
});
