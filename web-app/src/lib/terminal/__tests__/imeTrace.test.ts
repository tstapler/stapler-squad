import { attachImeTrace, imeTraceEnabled } from "../imeTrace";

describe("imeTrace", () => {
  it("is opt-in via ?imeTrace=1 only (not on by default for Android)", () => {
    expect(imeTraceEnabled("?imeTrace=1")).toBe(true);
    expect(imeTraceEnabled("?imeTrace=0")).toBe(false);
    expect(imeTraceEnabled("")).toBe(false);
  });

  it("ships the recorded input-path events as one [ime-trace] warning on dispose", () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    const root = document.createElement("div");
    const trace = attachImeTrace(root, "ghostty");
    root.dispatchEvent(new KeyboardEvent("keydown", { key: "p", keyCode: 229, bubbles: true }));
    root.dispatchEvent(new CompositionEvent("compositionupdate", { data: "hunter2", bubbles: true }));
    root.dispatchEvent(new InputEvent("beforeinput", { data: "s3cret", inputType: "insertText", bubbles: true }));
    trace.note("onData len=2");
    trace.dispose();
    expect(warn).toHaveBeenCalledTimes(1);
    const msg = String(warn.mock.calls[0][0]);
    expect(msg).toContain("[ime-trace] renderer=ghostty");
    expect(msg).toContain("keydown kc=229");
    expect(msg).toContain("compositionupdate len=7");
    expect(msg).toContain("beforeinput type=insertText len=6");
    expect(msg).toContain("onData len=2");
    // Privacy: typed content must never be recorded.
    expect(msg).not.toContain("hunter2");
    expect(msg).not.toContain("s3cret");
    expect(msg).not.toMatch(/key=/);
    warn.mockRestore();
  });

  it("keeps only the last 50 events", () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    const trace = attachImeTrace(document.createElement("div"), "xterm");
    for (let i = 0; i < 80; i++) trace.note(`n${i}`);
    trace.dispose();
    const msg = String(warn.mock.calls[0][0]);
    expect(msg).toContain("n79");
    expect(msg).not.toContain('"+0 n0"');
    expect(msg).not.toContain(" n29\"");
    warn.mockRestore();
  });
});
