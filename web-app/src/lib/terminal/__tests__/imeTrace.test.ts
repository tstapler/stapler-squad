import { attachImeTrace, imeTraceEnabled } from "../imeTrace";

describe("imeTrace", () => {
  it("enables on Android or ?imeTrace", () => {
    expect(imeTraceEnabled("Mozilla/5.0 (Linux; Android 10)", "")).toBe(true);
    expect(imeTraceEnabled("Mozilla/5.0 (Macintosh)", "?imeTrace=1")).toBe(true);
    expect(imeTraceEnabled("Mozilla/5.0 (Macintosh)", "")).toBe(false);
  });

  it("ships the recorded input-path events as one [ime-trace] warning on dispose", () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    const root = document.createElement("div");
    const trace = attachImeTrace(root, "ghostty");
    root.dispatchEvent(new KeyboardEvent("keydown", { key: "Unidentified", keyCode: 229, bubbles: true }));
    root.dispatchEvent(new CompositionEvent("compositionupdate", { data: "hi", bubbles: true }));
    trace.note("onData len=2");
    trace.dispose();
    expect(warn).toHaveBeenCalledTimes(1);
    const msg = String(warn.mock.calls[0][0]);
    expect(msg).toContain("[ime-trace] renderer=ghostty");
    expect(msg).toContain("keydown key=Unidentified code=229");
    expect(msg).toContain("compositionupdate data=\\\"hi\\\"");
    expect(msg).toContain("onData len=2");
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
