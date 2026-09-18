import { isAutoApproveSupported, isApprovalExtensionSupported } from "./autoApprove";

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

describe("isApprovalExtensionSupported", () => {
  it("returns true for pi", () => {
    expect(isApprovalExtensionSupported("pi")).toBe(true);
  });

  it("returns true for pi with extra args, matched on basename", () => {
    expect(isApprovalExtensionSupported("pi --some-flag")).toBe(true);
  });

  it("returns false for claude (enforced via its own hook, not this extension)", () => {
    expect(isApprovalExtensionSupported("claude")).toBe(false);
  });

  it("returns false for opencode (out of scope for this extension)", () => {
    expect(isApprovalExtensionSupported("opencode")).toBe(false);
  });

  it("returns false for an empty program", () => {
    expect(isApprovalExtensionSupported("")).toBe(false);
  });
});
