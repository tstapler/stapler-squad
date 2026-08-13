import { isAutoApproveSupported } from "./autoApprove";

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
