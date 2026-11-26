
import { planLeafMove } from "../paneUtils";

describe("planLeafMove", () => {
  const ids = ["a", "b", "c", "d"];

  it("moves forward as adjacent swaps and shifts the leaves in between back", () => {
    const plan = planLeafMove(ids, 0, 2, 1);
    expect(plan.swaps).toEqual([["a", "b"], ["b", "c"]]);
    expect(plan.focusIndex).toBe(0);
  });

  it("moves backward and shifts the leaves in between forward", () => {
    const plan = planLeafMove(ids, 3, 1, 2);
    expect(plan.swaps).toEqual([["d", "c"], ["c", "b"]]);
    expect(plan.focusIndex).toBe(3);
  });

  it("keeps focus on the moved content", () => {
    expect(planLeafMove(ids, 1, 3, 1).focusIndex).toBe(3);
  });

  it("leaves focus alone outside the moved range", () => {
    expect(planLeafMove(ids, 0, 1, 3).focusIndex).toBe(3);
  });

  it("does nothing for a no-op or out-of-range move", () => {
    expect(planLeafMove(ids, 2, 2, 0).swaps).toEqual([]);
    expect(planLeafMove(ids, 0, 9, 0).swaps).toEqual([]);
  });
});

import { titleFirstLine } from "../paneUtils";

describe("titleFirstLine", () => {
  it("returns the first non-empty line, trimmed", () => {
    expect(titleFirstLine("\n  Fix the bug  \nlong body")).toBe("Fix the bug");
  });
  it("returns a single-line title unchanged", () => {
    expect(titleFirstLine("Plain")).toBe("Plain");
  });
  it("falls back to the original when every line is blank", () => {
    expect(titleFirstLine("  \n ")).toBe("  \n ");
  });
});
