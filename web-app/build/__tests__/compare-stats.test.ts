import { compareToBaseline, type GrowthTolerances } from "../compare-stats";

const TOL: GrowthTolerances = { jsBytes: 100, initialJsBytes: 10, largestChunkBytes: 10, largestLazyChunkBytes: 50 };
const base = { jsBytes: 1000, initialJsBytes: 500, largestChunkBytes: 500, largestLazyChunkBytes: 300 };

describe("compareToBaseline", () => {
  it("passes when nothing grew past its tolerance, including a shrink", () => {
    expect(compareToBaseline({ ...base, jsBytes: 1100, initialJsBytes: 400 }, base, TOL)).toEqual([]);
  });

  it("fails only the metric that grew, naming before and after", () => {
    const errs = compareToBaseline({ ...base, initialJsBytes: 511 }, base, TOL);
    expect(errs).toEqual(["growth: Initial JS grew 11 B vs main (500 -> 511), over the 10 B tolerance"]);
  });

  it("never fails a PR because main itself is large", () => {
    const big = { jsBytes: 9e9, initialJsBytes: 9e9, largestChunkBytes: 9e9, largestLazyChunkBytes: 9e9 };
    expect(compareToBaseline(big, big, TOL)).toEqual([]);
  });

  it("skips a metric the baseline predates", () => {
    const { largestLazyChunkBytes: _omit, ...old } = base;
    expect(compareToBaseline({ ...base, largestLazyChunkBytes: 99999 }, old, TOL)).toEqual([]);
  });
});
