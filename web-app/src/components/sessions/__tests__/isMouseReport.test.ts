import { isMouseReport } from "../XtermTerminal";

describe("isMouseReport", () => {
  it.each(["\x1b[<35;51;21M", "\x1b[<0;51;21m", "\x1b[M #R", "\x1b[<0;1;1M\x1b[<0;1;1m"])("matches mouse report %j", (s) => {
    expect(isMouseReport(s)).toBe(true);
  });

  it.each(["a", "\x1b[A", "ls\r", "\x1b[<35;51;21Mx", "\x1b[200~paste"])("passes through %j", (s) => {
    expect(isMouseReport(s)).toBe(false);
  });
});

describe("trackSgrMouseEncoding", () => {
  it("follows ?1006h / ?1006l from the real xterm parser and ignores other modes", async () => {
    const { Terminal } = await import("@xterm/xterm");
    const { trackSgrMouseEncoding } = await import("../XtermTerminal");
    const term = new Terminal();
    const flag = { current: false };
    trackSgrMouseEncoding(term, flag);
    const write = (d: string) => new Promise<void>((r) => term.write(d, r));
    await write("\x1b[?1000h");
    expect(flag.current).toBe(false);
    await write("\x1b[?1000;1006h");
    expect(flag.current).toBe(true);
    await write("\x1b[?1006l");
    expect(flag.current).toBe(false);
  });
});

describe("trackSgrMouseEncoding reset", () => {
  it("clears the flag on RIS and DECSTR", async () => {
    const { Terminal } = await import("@xterm/xterm");
    const { trackSgrMouseEncoding } = await import("../XtermTerminal");
    const term = new Terminal();
    const flag = { current: false };
    trackSgrMouseEncoding(term, flag);
    const write = (d: string) => new Promise<void>((r) => term.write(d, r));
    await write("\x1b[?1006h");
    await write("\x1bc");
    expect(flag.current).toBe(false);
    await write("\x1b[?1006h");
    await write("\x1b[!p");
    expect(flag.current).toBe(false);
  });
});
