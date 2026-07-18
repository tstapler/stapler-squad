import { SESSION_TYPES } from "./OmnibarCreationPanel";

describe("SESSION_TYPES hint copy", () => {
  it("gives every session type a non-empty scenario-based description", () => {
    for (const type of SESSION_TYPES) {
      expect(type.description).toMatch(/^Use this when/);
    }
  });

  it("gives every session type a distinct description", () => {
    const descriptions = SESSION_TYPES.map((t) => t.description);
    expect(new Set(descriptions).size).toBe(descriptions.length);
  });
});
