import { SESSION_TYPES, isAutoApproveSupported } from "./OmnibarCreationPanel";

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

describe("isAutoApproveSupported", () => {
  it("returns true for claude", () => {
    expect(isAutoApproveSupported("claude")).toBe(true);
  });

  it("returns true for aider with extra args, matched on basename", () => {
    expect(isAutoApproveSupported("aider --model ollama_chat/gemma3:1b")).toBe(true);
  });

  it("returns false for an unsupported agent", () => {
    expect(isAutoApproveSupported("codex")).toBe(false);
  });

  it("returns true for an empty program (System default resolves to claude)", () => {
    expect(isAutoApproveSupported("")).toBe(true);
  });
});
