import { computeRangeIds } from "./rangeSelect";

const makeItems = (ids: string[]) =>
  ids.map(id => ({ kind: "session" as const, session: { id } }));

describe("computeRangeIds", () => {
  it("returns range between anchor and target inclusive", () => {
    const items = makeItems(["a", "b", "c", "d", "e"]);
    expect(computeRangeIds("a", "c", items)).toEqual(["a", "b", "c"]);
  });
  it("works in reverse order (target before anchor)", () => {
    const items = makeItems(["a", "b", "c", "d"]);
    expect(computeRangeIds("c", "a", items)).toEqual(["a", "b", "c"]);
  });
  it("falls back to single-select when anchor not in flatItems", () => {
    const items = makeItems(["b", "c", "d"]);
    expect(computeRangeIds("a", "c", items)).toEqual(["c"]);
  });
  it("returns [targetId] when anchor was filtered out but other items remain", () => {
    // Simulates: anchor 'a' was visible, user applied a filter, now only b/c/d are visible
    const items = makeItems(["b", "c", "d"]);
    const result = computeRangeIds("a", "c", items);
    // anchor not found → falls back to single-select of the target
    expect(result).toEqual(["c"]);
  });
  it("skips header items (non-session kind)", () => {
    const items = [
      { kind: "header" as const },
      { kind: "session" as const, session: { id: "a" } },
      { kind: "header" as const },
      { kind: "session" as const, session: { id: "b" } },
    ] as Array<{ kind: string; session?: { id: string } }>;
    expect(computeRangeIds("a", "b", items)).toEqual(["a", "b"]);
  });
});
