import { create } from "@bufbuild/protobuf";
import { SessionSchema, type Session } from "@/gen/session/v1/types_pb";
import { compareSessionsByCost } from "../sessionCostSort";

function makeSession(id: string): Session {
  return create(SessionSchema, { id, title: id });
}

describe("compareSessionsByCost", () => {
  it("compareSessionsByCost_should_sortHigherCostFirst_When_sortDirDesc", () => {
    const cheap = makeSession("cheap");
    const expensive = makeSession("expensive");
    const costById = new Map([
      ["cheap", 0.1],
      ["expensive", 5.0],
    ]);

    expect(compareSessionsByCost(expensive, cheap, costById, "desc")).toBeLessThan(0);
    expect(compareSessionsByCost(cheap, expensive, costById, "desc")).toBeGreaterThan(0);
  });

  it("compareSessionsByCost_should_sortLowerCostFirst_When_sortDirAsc", () => {
    const cheap = makeSession("cheap");
    const expensive = makeSession("expensive");
    const costById = new Map([
      ["cheap", 0.1],
      ["expensive", 5.0],
    ]);

    expect(compareSessionsByCost(cheap, expensive, costById, "asc")).toBeLessThan(0);
    expect(compareSessionsByCost(expensive, cheap, costById, "asc")).toBeGreaterThan(0);
  });

  it("compareSessionsByCost_should_sortMissingCostLast_When_sortDirDesc", () => {
    const priced = makeSession("priced");
    const missing = makeSession("missing");
    const costById = new Map([["priced", 1.0]]);

    expect(compareSessionsByCost(missing, priced, costById, "desc")).toBeGreaterThan(0);
    expect(compareSessionsByCost(priced, missing, costById, "desc")).toBeLessThan(0);
  });

  it("compareSessionsByCost_should_sortMissingCostLast_When_sortDirAsc", () => {
    // The bug class this test guards against: a sentinel fed into a generic
    // comparator inverts position on direction flip. Missing cost must stay
    // last on BOTH directions, not just desc.
    const priced = makeSession("priced");
    const missing = makeSession("missing");
    const costById = new Map([["priced", 1.0]]);

    expect(compareSessionsByCost(missing, priced, costById, "asc")).toBeGreaterThan(0);
    expect(compareSessionsByCost(priced, missing, costById, "asc")).toBeLessThan(0);
  });

  it("compareSessionsByCost_should_returnZero_When_bothSessionsMissingCost", () => {
    const a = makeSession("a");
    const b = makeSession("b");
    const costById = new Map<string, number>();

    expect(compareSessionsByCost(a, b, costById, "desc")).toBe(0);
    expect(compareSessionsByCost(a, b, costById, "asc")).toBe(0);
  });
});
