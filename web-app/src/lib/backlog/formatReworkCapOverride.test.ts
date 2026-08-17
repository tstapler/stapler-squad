import { resolveReworkCapOverride } from "./formatReworkCapOverride";

describe("resolveReworkCapOverride", () => {
  it("resolves undefined to 'unset'", () => {
    expect(resolveReworkCapOverride(undefined)).toEqual({ kind: "unset" });
  });

  it("resolves an explicit 0 to 'unlimited' (not falsy/unset)", () => {
    expect(resolveReworkCapOverride(0)).toEqual({ kind: "unlimited" });
  });

  it("resolves a positive value to 'capped' with the rounds carried through", () => {
    expect(resolveReworkCapOverride(5)).toEqual({ kind: "capped", rounds: 5 });
  });
});
